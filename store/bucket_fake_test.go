package store

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The kinds of request Corbie makes of a bucket. The fake takes these and
// refuses every other.
const (
	fRead            = "read"
	fReadPart        = "read part"
	fList            = "list"
	fOverwrite       = "overwrite"
	fNew             = "write only-if-new"
	fUnchanged       = "write only-if-unchanged"
	fDelete          = "delete"
	fDeleteUnchanged = "delete only-if-unchanged"
	fCopy            = "copy only-if-new"
	fStart           = "upload start"
	fPart            = "upload part"
	fEnd             = "upload end only-if-new"
	fGiveUp          = "upload given up"
)

const (
	fakeBucket = "bkt"
	fakeRegion = "test-1"
)

// fakeS3 stands in for a bucket, in this process. It answers as S3 does to
// the requests Corbie makes, fails the test on any other, and checks that
// every request is signed by a key it knows. A key can be held to a party's
// row of the table.
type fakeS3 struct {
	t   *testing.T
	srv *httptest.Server

	mu      sync.Mutex
	prefix  string // what the store's address puts every name under
	objects map[string][]byte
	uploads map[string]*fakeUpload
	keys    map[string]fakeKey

	// Switches.
	page     int            // a listing gives at most this many names at a time
	turnAway int            // this many requests more are answered with status, and not acted on
	status   int            // 503 is "slow down"
	only     string         // where set, only requests of this kind are turned away
	lose     map[string]int // by kind: this many more are acted on and their answers dropped
	sour     int            // this many copies and upload ends more fail after S3 has said all is well
	hold     time.Duration  // every request is kept this long

	// What it saw.
	seen           map[string]int // requests by kind, turned away or not
	at             []time.Time    // when each request came
	conns          int            // connections opened
	busy, mostBusy int            // requests here at once
}

type fakeKey struct {
	secret string
	token  string // what goes with a key lent for a while
	party  Party
}

type fakeUpload struct {
	key   string
	parts map[int][]byte
}

type reply struct {
	status int
	etag   string
	body   []byte
}

func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{t: t, prefix: "pre/fix/", page: 1000, status: http.StatusServiceUnavailable,
		objects: map[string][]byte{}, uploads: map[string]*fakeUpload{}, keys: map[string]fakeKey{},
		lose: map[string]int{}, seen: map[string]int{}}
	f.srv = httptest.NewUnstartedServer(f)
	f.srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			f.set(func() { f.conns++ })
		}
	}
	f.srv.Start()
	t.Cleanup(f.srv.Close)
	return f
}

// address is the store's address: it holds no key.
func (f *fakeS3) address() string {
	return "s3://" + fakeBucket + "/" + f.prefix + "?region=" + fakeRegion + "&endpoint=" + f.srv.URL
}

// newKey makes a key the fake knows, held to the party's row.
func (f *fakeS3) newKey(p Party) bucketKey {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("AKIDFAKE%04d", len(f.keys))
	key := bucketKey{id: id, secret: "sEcReT/of+" + id}
	f.keys[id] = fakeKey{secret: key.secret, party: p}
	return key
}

// open opens the store with a new key held to the party's row, with waits
// between tries short enough for a test.
func (f *fakeS3) open(p Party) *bucket {
	f.t.Helper()
	b, err := newBucket(f.address(), f.newKey(p))
	if err != nil {
		f.t.Fatal(err)
	}
	b.wait = time.Millisecond / 4
	return b
}

func (f *fakeS3) as(p Party) Store { return f.open(p) }

// set changes switches, or looks at what was seen, with the fake held still.
func (f *fakeS3) set(change func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change()
}

// took gives the requests seen since it was last asked, by kind.
func (f *fakeS3) took() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := f.seen
	f.seen = map[string]int{}
	f.at = nil
	return seen
}

// value gives what is under a name, without a request.
func (f *fakeS3) value(name string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.objects[f.prefix+name]
	return string(value), ok
}

func etag(value []byte) string {
	return fmt.Sprintf(`"%x"`, md5.Sum(value))
}

func fail(status int, code string) reply {
	return reply{status: status, body: []byte("<Error><Code>" + code + "</Code><Message>no</Message></Error>")}
}

// kind says which of Corbie's requests this is, or "" for any other.
func (f *fakeS3) kind(r *http.Request) string {
	q := r.URL.Query()
	only := func(names ...string) bool {
		return len(q) == len(names) && !slices.ContainsFunc(names, func(name string) bool { return !q.Has(name) })
	}
	get, put := r.Method == http.MethodGet, r.Method == http.MethodPut
	post, del := r.Method == http.MethodPost, r.Method == http.MethodDelete
	ifNew, ifSame := r.Header.Get("If-None-Match") == "*", r.Header.Get("If-Match") != ""
	source := r.Header.Get("X-Amz-Copy-Source") != ""
	switch {
	case r.URL.Path == "/"+fakeBucket:
		if get && q.Get("list-type") == "2" && q.Get("encoding-type") == "url" &&
			(only("list-type", "encoding-type", "prefix") || only("list-type", "encoding-type", "prefix", "continuation-token")) {
			return fList
		}
	case !strings.HasPrefix(r.URL.Path, "/"+fakeBucket+"/"):
	case get && only() && r.Header.Get("Range") != "":
		return fReadPart
	case get && only():
		return fRead
	case put && only() && source && ifNew:
		return fCopy
	case put && only() && !source && ifNew:
		return fNew
	case put && only() && !source && ifSame:
		return fUnchanged
	case put && only() && !source:
		return fOverwrite
	case put && only("partNumber", "uploadId"):
		return fPart
	case post && only("uploads"):
		return fStart
	case post && only("uploadId") && ifNew:
		return fEnd
	case del && only("uploadId"):
		return fGiveUp
	case del && only() && ifSame:
		return fDeleteUnchanged
	case del && only():
		return fDelete
	}
	return ""
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	kind := f.kind(r)

	f.mu.Lock()
	f.seen[kind]++
	f.at = append(f.at, time.Now())
	f.busy++
	f.mostBusy = max(f.mostBusy, f.busy)
	hold := f.hold
	f.mu.Unlock()

	time.Sleep(hold)

	f.mu.Lock()
	rep, lost := f.answer(kind, r, body)
	f.busy--
	f.mu.Unlock()

	if lost {
		panic(http.ErrAbortHandler) // the connection closes with nothing said
	}
	if rep.etag != "" {
		w.Header().Set("ETag", rep.etag)
	}
	w.WriteHeader(rep.status)
	w.Write(rep.body)
}

// answer acts on a request and gives the answer, and whether to drop it.
func (f *fakeS3) answer(kind string, r *http.Request, body []byte) (reply, bool) {
	if kind == "" {
		f.t.Errorf("a request Corbie has no need of: %s %s %v", r.Method, r.URL, r.Header)
		return fail(http.StatusNotImplemented, "NotImplemented"), false
	}
	// As S3 does, an answer that says what is wrong can quote the key and
	// the token it was given.
	quoting := func(status int, code string) reply {
		id := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential="), "/")[0]
		return reply{status: status, body: []byte("<Error><Code>" + code + "</Code><AWSAccessKeyId>" + id +
			"</AWSAccessKeyId><Token-0>" + xmlText(r.Header.Get("X-Amz-Security-Token")) + "</Token-0></Error>")}
	}
	key, code := f.signedBy(r, body)
	if code != "" {
		return quoting(http.StatusForbidden, code), false
	}
	if f.turnAway > 0 && (f.only == "" || f.only == kind) {
		f.turnAway--
		return quoting(f.status, map[int]string{503: "SlowDown", 500: "InternalError"}[f.status]), false
	}
	name := strings.TrimPrefix(r.URL.Path, "/"+fakeBucket+"/")
	if !f.allowed(key.party, kind, r, name) {
		return fail(http.StatusForbidden, "AccessDenied"), false
	}
	if f.sour > 0 && (kind == fCopy || kind == fEnd) {
		f.sour--
		return reply{status: http.StatusOK, body: []byte("<Error><Code>InternalError</Code></Error>")}, false
	}
	rep := f.act(kind, r, name, body)
	if f.lose[kind] > 0 {
		f.lose[kind]--
		return rep, true
	}
	return rep, false
}

var authorization = regexp.MustCompile(`^AWS4-HMAC-SHA256 Credential=([^/]+)/\d{8}/([^/]+)/s3/aws4_request, SignedHeaders=([a-z0-9;-]+), Signature=([0-9a-f]{64})$`)

// signedBy gives the key that signed a request, or S3's code for why the
// request is not taken.
func (f *fakeS3) signedBy(r *http.Request, body []byte) (fakeKey, string) {
	m := authorization.FindStringSubmatch(r.Header.Get("Authorization"))
	if m == nil {
		f.t.Errorf("a request not signed: %s %s", r.Method, r.URL)
		return fakeKey{}, "AccessDenied"
	}
	key, ok := f.keys[m[1]]
	if !ok {
		return fakeKey{}, "InvalidAccessKeyId"
	}
	sum := sha256.Sum256(body)
	names := strings.Split(m[3], ";")
	for name := range r.Header {
		// Every condition a request carries is under its signature.
		if name = strings.ToLower(name); (strings.HasPrefix(name, "if-") || strings.HasPrefix(name, "x-amz-") || name == "range") &&
			!slices.Contains(names, name) {
			f.t.Errorf("%s %s: the header %s is not signed", r.Method, r.URL, name)
		}
	}
	if r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) || m[2] != fakeRegion ||
		signature(key.secret, m[2], r, names) != m[4] {
		f.t.Errorf("a request wrongly signed: %s %s", r.Method, r.URL)
		return fakeKey{}, "SignatureDoesNotMatch"
	}
	if r.Header.Get("X-Amz-Security-Token") != key.token {
		return fakeKey{}, "InvalidToken"
	}
	return key, ""
}

// allowed says whether the key's party may make this request.
func (f *fakeS3) allowed(p Party, kind string, r *http.Request, key string) bool {
	name := strings.TrimPrefix(key, f.prefix)
	switch kind {
	case fRead, fReadPart:
		return p.Allows(ActRead, name)
	case fList:
		under := strings.TrimPrefix(r.URL.Query().Get("prefix"), f.prefix)
		return p.Allows(ActList, strings.TrimSuffix(under, "/"))
	case fNew, fEnd:
		return p.Allows(ActAdd, name)
	case fCopy:
		return p.Allows(ActAdd, name) && p.Allows(ActRead, strings.TrimPrefix(f.source(r), f.prefix))
	case fOverwrite, fUnchanged:
		return p.Allows(ActRewrite, name)
	case fDelete, fDeleteUnchanged:
		return p.Allows(ActDelete, name)
	}
	// The steps of an upload that put nothing under the name.
	return p.Allows(ActAdd, name) || p.Allows(ActRewrite, name)
}

// source is the key a copy is to be made of.
func (f *fakeS3) source(r *http.Request) string {
	from, err := url.PathUnescape(r.Header.Get("X-Amz-Copy-Source"))
	if err != nil {
		f.t.Errorf("a copy's source is not escaped: %v", err)
	}
	return strings.TrimPrefix(from, "/"+fakeBucket+"/")
}

// act does what a request asks.
func (f *fakeS3) act(kind string, r *http.Request, key string, body []byte) reply {
	q := r.URL.Query()
	old, there := f.objects[key]
	match := r.Header.Get("If-Match")
	put := func(value []byte) reply {
		f.objects[key] = value
		return reply{status: http.StatusOK, etag: etag(value)}
	}
	switch kind {
	case fList:
		return f.list(q)

	case fRead:
		if !there {
			return fail(http.StatusNotFound, "NoSuchKey")
		}
		return reply{http.StatusOK, etag(old), old}

	case fReadPart:
		var from, to int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &from, &to); err != nil || from < 0 || to < from {
			// S3 takes a range it cannot read for none, and gives the whole value.
			f.t.Errorf("a range no bucket reads: %q", r.Header.Get("Range"))
			return fail(http.StatusBadRequest, "InvalidArgument")
		}
		switch {
		case !there:
			return fail(http.StatusNotFound, "NoSuchKey")
		case from >= len(old):
			return fail(http.StatusRequestedRangeNotSatisfiable, "InvalidRange")
		}
		return reply{http.StatusPartialContent, etag(old), old[from:min(to+1, len(old))]}

	case fOverwrite:
		return put(body)

	case fNew:
		if there {
			return fail(http.StatusPreconditionFailed, "PreconditionFailed")
		}
		return put(body)

	case fUnchanged:
		switch {
		case !there:
			return fail(http.StatusNotFound, "NoSuchKey")
		case etag(old) != match:
			return fail(http.StatusPreconditionFailed, "PreconditionFailed")
		}
		return put(body)

	case fDeleteUnchanged:
		// S3 counts a conditional delete of nothing as done.
		if there && etag(old) != match {
			return fail(http.StatusPreconditionFailed, "PreconditionFailed")
		}
		fallthrough
	case fDelete:
		delete(f.objects, key)
		return reply{status: http.StatusNoContent}

	case fCopy:
		from := f.source(r)
		value, ok := f.objects[from]
		switch {
		case from == key:
			return fail(http.StatusBadRequest, "InvalidRequest")
		case !ok:
			return fail(http.StatusNotFound, "NoSuchKey")
		case there:
			return fail(http.StatusPreconditionFailed, "PreconditionFailed")
		}
		put(value)
		return reply{status: http.StatusOK, body: []byte("<CopyObjectResult><ETag>" + xmlText(etag(value)) + "</ETag></CopyObjectResult>")}

	case fStart:
		id := fmt.Sprintf("upload %d/+", len(f.at))
		f.uploads[id] = &fakeUpload{key: key, parts: map[int][]byte{}}
		return reply{status: http.StatusOK, body: []byte("<InitiateMultipartUploadResult><UploadId>" + id + "</UploadId></InitiateMultipartUploadResult>")}
	}

	// The rest are steps of an upload that has started.
	id := q.Get("uploadId")
	up := f.uploads[id]
	if up == nil || up.key != key {
		return fail(http.StatusNotFound, "NoSuchUpload")
	}
	switch kind {
	case fPart:
		n, err := strconv.Atoi(q.Get("partNumber"))
		if err != nil || n < 1 || n > 10000 {
			return fail(http.StatusBadRequest, "InvalidArgument")
		}
		up.parts[n] = body
		return reply{status: http.StatusOK, etag: etag(body)}

	case fEnd:
		var whole struct{ Part []uploadedPart }
		if err := xml.Unmarshal(body, &whole); err != nil {
			return fail(http.StatusBadRequest, "MalformedXML")
		}
		var value []byte
		for i, part := range whole.Part {
			sent, ok := up.parts[part.PartNumber]
			if !ok || part.PartNumber != i+1 || part.ETag != etag(sent) {
				return fail(http.StatusBadRequest, "InvalidPart")
			}
			value = append(value, sent...)
		}
		if there {
			return fail(http.StatusPreconditionFailed, "PreconditionFailed")
		}
		delete(f.uploads, id)
		put(value)
		return reply{status: http.StatusOK, body: []byte("<CompleteMultipartUploadResult></CompleteMultipartUploadResult>")}
	}
	delete(f.uploads, id) // fGiveUp
	return reply{status: http.StatusNoContent}
}

// list gives one page of a listing: the names in order, escaped as S3
// escapes them, and where to go on from if there are more.
func (f *fakeS3) list(q url.Values) reply {
	var keys []string
	for key := range f.objects {
		if strings.HasPrefix(key, q.Get("prefix")) && key > q.Get("continuation-token") {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	var page struct {
		XMLName               xml.Name `xml:"ListBucketResult"`
		IsTruncated           bool
		Contents              []struct{ Key string }
		NextContinuationToken string `xml:",omitempty"`
	}
	if len(keys) > f.page {
		keys = keys[:f.page]
		page.IsTruncated = true
		page.NextContinuationToken = keys[len(keys)-1]
	}
	for _, key := range keys {
		key = strings.ReplaceAll(url.QueryEscape(key), "%2F", "/")
		page.Contents = append(page.Contents, struct{ Key string }{key})
	}
	body, err := xml.Marshal(page)
	if err != nil {
		f.t.Error(err)
	}
	return reply{status: http.StatusOK, body: body}
}

func xmlText(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
