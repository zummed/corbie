package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// A request is one thing asked of the bucket.
type request struct {
	method string
	name   string // the value's name, or none for a listing
	query  url.Values
	header map[string]string
	from   string // for a copy, the name of the value to copy
	body   []byte
	twice  bool // sending it twice is the same as sending it once
}

// An answer is what the bucket said to a request.
type answer struct {
	status int
	header http.Header
	body   []byte
}

// send sends a request until the bucket answers it, for a few seconds with
// growing waits, and then reports the store as out of reach.
//
// A request the bucket turned away is sent again whatever it is. One the
// bucket may have acted on is sent again only where twice is the same as
// once; otherwise the caller is told the outcome is not known, since a
// second try could meet the first one's own value.
func (b *bucket) send(ctx context.Context, r request) (answer, error) {
	if err := ctx.Err(); err != nil {
		return answer{}, err
	}
	sum := sha256.Sum256(r.body)
	hash := hex.EncodeToString(sum[:])
	wait := b.wait
	var err error
	for try := range bucketTries {
		if try > 0 {
			select {
			case <-ctx.Done():
				return answer{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
			case <-time.After(wait):
			}
			wait *= 2
		}
		var a answer
		var mayHaveActed bool
		if a, mayHaveActed, err = b.once(ctx, r, hash); err == nil {
			return a, nil
		}
		if mayHaveActed && !r.twice {
			return answer{}, fmt.Errorf("%w: %w", ErrUnknown, err)
		}
	}
	return answer{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
}

// once sends a request one time. Of a try the bucket gave no answer to, it
// says whether the bucket may have acted on the request all the same.
func (b *bucket) once(ctx context.Context, r request, hash string) (a answer, mayHaveActed bool, err error) {
	path := b.base
	if r.name != "" {
		path += "/" + b.prefix + r.name
	}
	if path == "" {
		path = "/"
	}
	target := b.origin + escapePath(path)
	if len(r.query) > 0 {
		target += "?" + strings.ReplaceAll(r.query.Encode(), "+", "%20")
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, bytes.NewReader(r.body))
	if err != nil {
		return answer{}, false, withoutAddress(err)
	}
	req.Header.Set("X-Amz-Content-Sha256", hash)
	for name, value := range r.header {
		req.Header.Set(name, value)
	}
	if r.from != "" {
		req.Header.Set("X-Amz-Copy-Source", escapePath("/"+b.name+"/"+b.prefix+r.from))
	}
	b.key.sign(req, b.region, time.Now())

	resp, err := b.client.Do(req)
	if err != nil {
		// A request that never found the bucket was never sent.
		var op *net.OpError
		neverSent := errors.As(err, &op) && op.Op == "dial"
		return answer{}, !neverSent, withoutAddress(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return answer{}, true, err
	}
	a = answer{resp.StatusCode, resp.Header, body}
	switch {
	// Turned away: slow down, or a write to the same name is under way.
	case a.status == http.StatusServiceUnavailable, a.status == http.StatusTooManyRequests, a.status == http.StatusConflict:
		return answer{}, false, a.unexpected()
	// Something went wrong at the bucket, or between here and it.
	case a.status >= 500:
		return answer{}, true, a.unexpected()
	}
	return a, false, nil
}

// unexpected is the error for an answer that is none of the answers the
// request has. It shows the bucket's code for what went wrong and nothing
// else of what the bucket said, which can quote the key.
func (a answer) unexpected() error {
	return fmt.Errorf("store: the bucket answered %s", strings.TrimSpace(fmt.Sprint(a.status, " ", errorCode(a.body))))
}

// errorCode is the code in an answer that says what went wrong, and nothing
// for any other answer.
func errorCode(body []byte) string {
	var doc struct {
		XMLName xml.Name `xml:"Error"`
		Code    string
	}
	if xml.Unmarshal(body, &doc) != nil {
		return ""
	}
	return doc.Code
}

// withoutAddress takes the request's address off an error.
func withoutAddress(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// escape writes a string the way a signed request must: every byte but
// letters, digits and "-_.~" as a percent and two digits.
func escape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// escapePath escapes each part of a path and keeps its slashes.
func escapePath(path string) string {
	return strings.ReplaceAll(escape(path), "%2F", "/")
}
