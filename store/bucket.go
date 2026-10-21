package store

import (
	"cmp"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// A bucket store keeps every value as one object in a bucket of S3, or of a
// service that takes the same requests. Each of the three ways of writing is
// one request that carries its condition, and the mark of a value is the tag
// the bucket gives it.
//
// Its address is s3://bucket/path?region=…&endpoint=…. The path is what
// every name is put under, and the endpoint is for a service other than S3.
// The party's key is never in the address.
type bucket struct {
	client *http.Client
	key    bucketKey
	region string
	name   string // the bucket's own name
	origin string // the scheme and host every request goes to
	base   string // the path every key is under: none where the host names the bucket
	prefix string // what every key begins with: none, or "some/path/"

	wait time.Duration // before a second try, and twice as long before each one after
	part int64         // a value this long or longer is sent in parts of this size
}

const (
	// Tries at one request go on for a little over six seconds: seven of
	// them, with waits of 0.1s, 0.2s and so on up to 3.2s in between.
	bucketTries = 7
	bucketWait  = 100 * time.Millisecond

	// A connection that has sat unused this long is not used again.
	bucketIdle = 3 * time.Second

	// No more than this many requests are with the bucket at once.
	bucketConns = 32

	// A value this long or longer is sent in parts of this size. A bucket
	// takes parts of five megabytes and up, and ten thousand of them.
	bucketPart = 16 << 20

	// How long giving up an upload may take after its caller has gone.
	bucketGiveUp = 10 * time.Second
)

func openBucket(address string) (Store, error) {
	key, err := bucketKeyHere()
	if err != nil {
		return nil, err
	}
	return newBucket(address, key)
}

// newBucket opens the bucket an address names with a key. Nothing is sent
// until something is read or written. No error here repeats the address,
// which may wrongly have a key in it.
func newBucket(address string, key bucketKey) (*bucket, error) {
	notAnAddress := errors.New("store: a bucket's address reads s3://bucket/path?region=…")
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "s3" || u.Host == "" {
		return nil, notAnAddress
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, notAnAddress
	}
	for name := range q {
		if name != "region" && name != "endpoint" {
			return nil, errors.New("store: a bucket's address takes a region and an endpoint and nothing else")
		}
	}
	b := &bucket{key: key, name: u.Host, wait: bucketWait, part: bucketPart}
	b.region = cmp.Or(q.Get("region"), os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"))
	if b.region == "" {
		return nil, errors.New("store: a bucket's address says its region: s3://bucket?region=…")
	}
	if prefix := strings.Trim(u.Path, "/"); prefix != "" {
		b.prefix = prefix + "/"
	}
	b.origin = "https://" + b.name + ".s3." + b.region + ".amazonaws.com"
	holdsKey := u.User != nil
	if endpoint := q.Get("endpoint"); endpoint != "" {
		e, err := url.Parse(endpoint)
		if err != nil || e.Host == "" || (e.Scheme != "http" && e.Scheme != "https") {
			return nil, errors.New("store: a bucket's endpoint reads https://host:port")
		}
		holdsKey = holdsKey || e.User != nil
		b.origin = e.Scheme + "://" + e.Host
		b.base = "/" + b.name
	}
	if holdsKey {
		return nil, errors.New("store: a bucket's address never holds a key: the key comes from the environment")
	}
	b.client = &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			// A bucket that takes a request and then says nothing has not
			// answered.
			ResponseHeaderTimeout: 30 * time.Second,
			// A value comes back as the bytes that were written.
			DisableCompression:  true,
			MaxConnsPerHost:     bucketConns,
			MaxIdleConnsPerHost: bucketConns,
			IdleConnTimeout:     bucketIdle,
		},
		// The bucket's answer is taken as it comes, never followed elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return b, nil
}

func (b *bucket) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := checkName(name); err != nil {
		return nil, None, err
	}
	a, err := b.send(ctx, request{method: http.MethodGet, name: name, twice: true})
	switch {
	case err != nil:
		return nil, None, err
	case a.status == http.StatusOK:
		return a.body, Mark(a.header.Get("ETag")), nil
	case a.status == http.StatusNotFound, a.status == http.StatusForbidden:
		return nil, None, nil
	}
	return nil, None, a.unexpected()
}

func (b *bucket) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if length <= 0 {
		return nil, nil // a bucket takes a range of nothing for the whole value
	}
	a, err := b.send(ctx, request{method: http.MethodGet, name: name, twice: true,
		header: map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", from, from+length-1)}})
	switch {
	case err != nil:
		return nil, err
	case a.status == http.StatusPartialContent:
		return a.body, nil
	case a.status == http.StatusNotFound, a.status == http.StatusForbidden,
		a.status == http.StatusRequestedRangeNotSatisfiable: // the value ends before from
		return nil, nil
	}
	return nil, a.unexpected()
}

func (b *bucket) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	return b.change(ctx, request{method: http.MethodPut, name: name, body: value}, how)
}

func (b *bucket) Delete(ctx context.Context, name string, how How) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	if how.kind != onlyIfUnchanged {
		how = Overwrite
	}
	return b.change(ctx, request{method: http.MethodDelete, name: name}, how)
}

// Copy asks the bucket to copy the value itself. A value is never copied
// onto itself: a bucket counts that as replacing it.
func (b *bucket) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := errors.Join(checkName(from), checkName(to)); err != nil {
		return false, err
	}
	if from == to {
		return false, nil
	}
	return b.change(ctx, request{method: http.MethodPut, name: to, from: from}, OnlyIfNew)
}

// change sends a write, a copy or a delete in the way how says, as one
// request that carries its condition, and gives the bucket's answer to it.
// Nothing is read or listed first.
func (b *bucket) change(ctx context.Context, r request, how How) (bool, error) {
	switch how.kind {
	case overwrite:
		r.twice = true
	case onlyIfNew:
		r.header = map[string]string{"If-None-Match": "*"}
	case onlyIfUnchanged:
		if how.mark == None {
			// Nothing was read, so nothing can be unchanged; and a bucket
			// takes a condition that names no mark for no condition.
			return false, nil
		}
		r.header = map[string]string{"If-Match": string(how.mark)}
	}
	deleting := r.method == http.MethodDelete
	a, err := b.send(ctx, r)
	switch {
	case err != nil:
		return false, err
	case a.status == http.StatusPreconditionFailed:
		return false, nil
	case a.status == http.StatusForbidden:
		return false, fmt.Errorf("%w to %s %s", ErrRefused, actOf(how, deleting), r.name)
	case a.status == http.StatusNotFound && deleting:
		return true, nil
	case a.status == http.StatusNotFound && r.from != "":
		return false, notThere(r.from)
	case a.status == http.StatusNotFound && how.kind == onlyIfUnchanged:
		return false, nil // deleted since it was read
	// A bucket can answer that all is well and then say in the answer's
	// body that it is not.
	case a.status/100 == 2 && errorCode(a.body) == "":
		return true, nil
	}
	return false, a.unexpected()
}

// WriteFrom sends a value shorter than one part as one write, and a longer
// one in parts that the bucket puts under the name together at the end.
func (b *bucket) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	first, err := io.ReadAll(io.LimitReader(r, b.part))
	if err != nil {
		return false, err
	}
	if int64(len(first)) < b.part {
		return b.Write(ctx, name, first, OnlyIfNew)
	}
	return b.writeParts(ctx, name, first, r)
}

// writeParts sends a value in parts, the first of which is already read.
// Only the last request puts anything under the name, and it carries the
// condition. An upload that does not get that far is given up, which leaves
// no value.
func (b *bucket) writeParts(ctx context.Context, name string, part []byte, rest io.Reader) (done bool, err error) {
	a, err := b.step(ctx, request{method: http.MethodPost, name: name, query: url.Values{"uploads": {""}}})
	if err != nil {
		return false, err
	}
	var upload struct{ UploadId string }
	if err := xml.Unmarshal(a.body, &upload); err != nil {
		return false, err
	}
	id := []string{upload.UploadId}
	defer func() {
		if !done {
			// The caller may be gone already, and gets the first error
			// whatever becomes of this: an upload left behind holds no value.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bucketGiveUp)
			defer cancel()
			b.send(ctx, request{method: http.MethodDelete, name: name, query: url.Values{"uploadId": id}, twice: true})
		}
	}()
	var whole struct {
		XMLName xml.Name `xml:"CompleteMultipartUpload"`
		Part    []uploadedPart
	}
	for n := 1; len(part) > 0; n++ {
		a, err := b.step(ctx, request{method: http.MethodPut, name: name, body: part,
			query: url.Values{"uploadId": id, "partNumber": {strconv.Itoa(n)}}})
		if err != nil {
			return false, err
		}
		whole.Part = append(whole.Part, uploadedPart{n, a.header.Get("ETag")})
		if part, err = io.ReadAll(io.LimitReader(rest, b.part)); err != nil {
			return false, err
		}
	}
	body, err := xml.Marshal(whole)
	if err != nil {
		return false, err
	}
	return b.change(ctx, request{method: http.MethodPost, name: name, query: url.Values{"uploadId": id}, body: body}, OnlyIfNew)
}

type uploadedPart struct {
	PartNumber int
	ETag       string
}

// step sends one request of an upload that puts nothing under the name, so
// that sending it twice does no harm.
func (b *bucket) step(ctx context.Context, r request) (answer, error) {
	r.twice = true
	a, err := b.send(ctx, r)
	switch {
	case err != nil:
		return a, err
	case a.status == http.StatusForbidden:
		return a, fmt.Errorf("%w to %s %s", ErrRefused, ActAdd, r.name)
	case a.status != http.StatusOK:
		return a, a.unexpected()
	}
	return a, nil
}

// List reads the listing to its end: a bucket gives at most a thousand
// names at a time, in the order they sort, each with its ETag, which is the
// mark a read gives.
func (b *bucket) List(ctx context.Context, under string) ([]Listed, error) {
	return b.list(ctx, under, 0)
}

// First reads one page of a listing, which is a thousand names at most.
func (b *bucket) First(ctx context.Context, under string, n int) ([]Listed, error) {
	listed, err := b.list(ctx, under, 1)
	return listed[:max(min(len(listed), n, firstMost), 0)], err
}

// list reads a listing to its end, or for a number of pages.
func (b *bucket) list(ctx context.Context, under string, pages int) ([]Listed, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	prefix := b.prefix
	if under != "" {
		prefix += under + "/"
	}
	// Names come back escaped, so that no name can spoil the listing.
	query := url.Values{"list-type": {"2"}, "encoding-type": {"url"}, "prefix": {prefix}}
	var listed []Listed
	for {
		a, err := b.send(ctx, request{method: http.MethodGet, query: query, twice: true})
		switch {
		case err != nil:
			return nil, err
		case a.status == http.StatusForbidden:
			return nil, fmt.Errorf("%w to %s %s", ErrRefused, ActList, under)
		case a.status != http.StatusOK:
			return nil, a.unexpected()
		}
		var page struct {
			Contents              []struct{ Key, ETag string }
			IsTruncated           bool
			NextContinuationToken string
		}
		if err := xml.Unmarshal(a.body, &page); err != nil {
			return nil, err
		}
		for _, c := range page.Contents {
			key, err := url.QueryUnescape(c.Key)
			if err != nil {
				return nil, err
			}
			listed = append(listed, Listed{strings.TrimPrefix(key, b.prefix), Mark(c.ETag)})
		}
		if pages--; !page.IsTruncated || pages == 0 {
			return listed, nil
		}
		query.Set("continuation-token", page.NextContinuationToken)
	}
}

// Notice gives a channel that is never sent to: a bucket does not tell.
func (b *bucket) Notice(string) (<-chan struct{}, func()) {
	return make(chan struct{}), func() {}
}
