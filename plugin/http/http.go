// Package http is the plugin for web requests.
package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	nethttp "net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

// maxRedirects is how many redirects one call follows.
const maxRedirects = 20

// wait is how long the whole of a request may take unless it says.
var wait = 10 * time.Second

var options = []string{"headers", "query", "body", "json", "form", "upload", "timeout", "redirects", "as", "trust"}

func init() {
	// Only request takes its method as an option: each other call is one.
	calls := map[string]*plugin.Call{"request": {Options: append([]string{"method"}, options...), Do: request("")}}
	for _, method := range []string{"get", "head", "post", "put", "patch", "delete"} {
		calls[method] = &plugin.Call{Options: options, Do: request(strings.ToUpper(method))}
	}
	plugin.Register(&plugin.Module{Name: "http", Page: page, Calls: calls})
}

func request(named string) func(*plugin.Job, any, plugin.Options) (any, error) {
	return func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
		method := named
		if method == "" {
			m, err := opt.Text("method", "GET")
			if err != nil {
				return nil, err
			}
			method = strings.ToUpper(m)
		}
		return do(job, method, first, opt)
	}
}

// connection is the connection a web address is reached over: its host and
// port, and whether it is encrypted. Over https it always is: nothing
// falls back to a plain connection.
func connection(u *url.URL) (address string, encrypted bool) {
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port), u.Scheme == "https"
}

func do(job *plugin.Job, method string, first any, opt plugin.Options) (any, error) {
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	follow, err := opt.Bool("redirects", true)
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "text")
	if err != nil {
		return nil, err
	}
	headers, err := opt.Map("headers")
	if err != nil {
		return nil, err
	}
	// given says whether the script wrote a header, in whatever letters.
	given := func(name string) bool {
		for written := range headers {
			if strings.EqualFold(written, name) {
				return true
			}
		}
		return false
	}
	query, err := opt.Map("query")
	if err != nil {
		return nil, err
	}
	// The limit covers the whole of the request, the body included.
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()

	transport := &nethttp.Transport{
		Proxy: nil, // nothing of the machine's own settings is used
		// A connection is the outcome of one check, so each request opens
		// its own and none is kept for another.
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return job.Dial(ctx, address)
		},
		ForceAttemptHTTP2: false,
		// A script that says how the answer may be packed, or asks for a
		// part of it, is sent that header alone and unpacks the answer
		// itself.
		DisableCompression: given("Accept-Encoding") || given("Range"),
	}
	if opt.Has("trust") {
		// Only the certificates the script gives are trusted, at every
		// place the call goes.
		pem, _, err := job.Contents(opt["trust"])
		if err != nil {
			return nil, err
		}
		pool, err := plugin.Certificates(pem)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	}
	client := &nethttp.Client{Transport: transport, CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error {
		return nethttp.ErrUseLastResponse // each redirect is followed here, one at a time
	}}

	address := first
	for hop := 0; ; hop++ {
		// The address is revealed afresh for each place: a secret in it is
		// taken only for a host it is bound to.
		target, err := addressOf(job, address, query)
		if err != nil {
			return nil, err
		}
		query = nil
		over, encrypted := connection(target)
		reveal := func(v any) (string, error) { return job.RevealOver(v, over, encrypted) }
		var body []byte
		var large *plugin.Blob // a body that is a blob is sent as it is read
		var parts io.Reader    // and so is an upload
		var length int64
		contentType := ""
		switch {
		case opt.Has("upload"):
			fields, ferr := opt.Map("upload")
			if ferr != nil {
				return nil, ferr
			}
			parts, length, contentType, err = upload(job, fields, reveal)
		case opt.Has("json"):
			// The JSON is written here, text included, so here is where
			// what goes into it is escaped.
			body, err = plugin.JSON(opt["json"])
			contentType = "application/json"
		case opt.Has("form"):
			fields, ferr := opt.Map("form")
			if ferr != nil {
				return nil, ferr
			}
			values := url.Values{}
			for name, value := range fields {
				text, rerr := reveal(plugin.TextOf(value))
				if rerr != nil {
					return nil, rerr
				}
				values.Set(name, text)
			}
			body, contentType = []byte(values.Encode()), "application/x-www-form-urlencoded"
		case opt.Has("body"):
			if s, ok := opt["body"].(*plugin.Secret); ok {
				// A body with a secret in it is sent as the script wrote
				// it: whatever type the request states, nothing here
				// escapes the value.
				text, rerr := reveal(s)
				if rerr != nil {
					return nil, rerr
				}
				body = []byte(text)
			} else if b, ok := opt["body"].(*plugin.Blob); ok {
				large = b
			} else {
				body, _, err = job.Contents(opt["body"])
			}
		}
		if err != nil {
			return nil, err
		}
		if (len(body) > 0 || large != nil || parts != nil) && (method == "GET" || method == "HEAD") {
			return nil, plugin.Errorf(plugin.BadInput, "a request that only fetches (%s) carries no body", method)
		}
		var sending io.Reader = bytes.NewReader(body)
		if large != nil {
			sending, length = large.Reader(job), large.Size
			if contentType == "" {
				contentType = large.Type
			}
		}
		if parts != nil {
			sending = parts
		}
		req, err := nethttp.NewRequestWithContext(ctx, method, target.String(), sending)
		if err != nil {
			return nil, plugin.Errorf(plugin.BadInput, "the request cannot be made")
		}
		if large != nil || parts != nil {
			req.ContentLength = length
		}
		// A script's headers are sent as written, and what is added here
		// is added only where the script wrote none.
		if req.Header, err = job.Headers(headers, over, encrypted); err != nil {
			return nil, err
		}
		if _, ours := req.Header["User-Agent"]; !given("User-Agent") {
			req.Header["User-Agent"] = []string{"corbie"}
		} else if !ours {
			// The script's own is in other letters. It goes as written,
			// and this keeps a second from being put beside it.
			req.Header["User-Agent"] = nil
		}
		if contentType != "" && !given("Content-Type") {
			req.Header["Content-Type"] = []string{contentType}
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, failure(err, target, limit)
		}
		if follow && resp.StatusCode >= 300 && resp.StatusCode < 400 && resp.Header.Get("Location") != "" {
			resp.Body.Close()
			next, err := target.Parse(plugin.EscapedAddress(resp.Header.Get("Location")))
			if err != nil || (next.Scheme != "http" && next.Scheme != "https") {
				return nil, plugin.Errorf(plugin.Refused, "the server redirected to something that is not a web address")
			}
			if hop >= maxRedirects {
				return nil, plugin.Errorf(plugin.Refused, "the request was redirected more than %d times", maxRedirects)
			}
			if resp.StatusCode == 303 || ((resp.StatusCode == 301 || resp.StatusCode == 302) && method == "POST") {
				method = "GET"
				delete(opt, "body")
				delete(opt, "json")
				delete(opt, "form")
				delete(opt, "upload")
			}
			address = next.String()
			continue
		}
		defer resp.Body.Close()
		out := map[string]any{"status": float64(resp.StatusCode), "url": target.String(), "headers": headersOf(resp.Header)}
		contentTypeOf := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		// A body going into a blob is passed on as it arrives, whatever its
		// size; one handed to the script is held whole, and has a largest
		// size.
		out["body"], err = job.Give(form, contentTypeOf, "the body", func(w io.Writer) error {
			if _, err := io.Copy(w, resp.Body); err != nil && !errors.Is(err, io.ErrShortWrite) {
				return failure(err, target, limit)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	}
}

// addressOf reads the address a call was given: text, or a handle that
// holds a secret. What cannot go out as it is is escaped, which a request
// needs, and nothing else in the address is changed. One that holds a user
// name or a password is refused, and no error repeats an address.
func addressOf(job *plugin.Job, address any, query map[string]any) (*url.URL, error) {
	text, isText := address.(string)
	var shape *url.URL
	if !isText {
		handle, ok := address.(*plugin.Secret)
		if !ok {
			return nil, plugin.Errorf(plugin.BadInput, "the call takes a web address first")
		}
		// To learn where it goes, the address is read with no secret in
		// it; then it is revealed for that place.
		probe, err := url.Parse(plugin.EscapedAddress(handle.Shape()))
		if err != nil || probe.Host == "" {
			return nil, plugin.Errorf(plugin.BadInput, "the address cannot be read, or holds a secret in its host")
		}
		over, encrypted := connection(probe)
		if text, err = job.RevealOver(handle, over, encrypted); err != nil {
			return nil, err
		}
		shape = probe
	}
	u, err := url.Parse(plugin.EscapedAddress(text))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, plugin.Errorf(plugin.BadInput, "the call takes a web address, starting http:// or https://, first")
	}
	// The secret was allowed for the place the address named without it.
	if shape != nil && (u.Scheme != shape.Scheme || u.Host != shape.Host || (u.User != nil) != (shape.User != nil)) {
		return nil, plugin.Errorf(plugin.BadInput, "a secret may not stand in the part of an address that says where the request goes: its scheme, host, port or user")
	}
	if u.User != nil {
		return nil, plugin.Errorf(plugin.BadInput, "an address may not hold a user name or a password: send a sign-in as a header")
	}
	// What the query option names is added after what the address already
	// asks, which stays as it was written.
	names := make([]string, 0, len(query))
	for name := range query {
		names = append(names, name)
	}
	sort.Strings(names)
	over, encrypted := connection(u)
	for _, name := range names {
		value, err := job.RevealOver(plugin.TextOf(query[name]), over, encrypted)
		if err != nil {
			return nil, err
		}
		if u.RawQuery != "" {
			u.RawQuery += "&"
		}
		u.RawQuery += url.QueryEscape(name) + "=" + url.QueryEscape(value)
	}
	return u, nil
}

func headersOf(h nethttp.Header) map[string]any {
	out := map[string]any{}
	for name, values := range h {
		out[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	return out
}

// failure gives a failed request the one shape of error. A refusal passes
// through as it is.
func failure(err error, target *url.URL, limit time.Duration) error {
	if refusal := plugin.Refusal(err); refusal != nil {
		return refusal
	}
	var known *plugin.Error
	if errors.As(err, &known) {
		return known
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return plugin.Errorf(plugin.TimedOut, "%s did not answer within %s", target.Host, limit)
	}
	return plugin.Errorf(plugin.Refused, "the request to %s failed: %v", target.Host, unwrap(err))
}

func unwrap(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}

const page = `http: this plugin sends HTTP requests.

  http.get(url, options)        and head, post, put, patch, delete
  http.request(url, options)    with { method }

  gives { status, headers, body, url }

method is the HTTP method of http.request; default "GET".

Options:
  headers     a map, sent as written
  query       a map, added to the end of the URL's query
  body        text, bytes or a blob, sent as it is
  json        data, sent as JSON
  form        a map, sent as a URL-encoded form
  upload      a map, sent as multipart/form-data; a value is text, or a file: bytes, a blob
              or { data, name, type }
  as          the form of body: "text", "json", "bytes" or "blob"; default "text"
  timeout     for the whole request: seconds, or text such as "30s"; default 10s
  redirects   false returns a redirect and does not follow it; default true
  trust       certificates in PEM, the only ones then trusted, at every host; one that
              cannot be read is an error

Returns:
  status    the status code; 404 and 500 are not errors
  headers   a map, with names in lower case
  body      the body, in the form of as
  url       the URL that answered, after redirects

A handle to a secret (` + "`corbie manual secrets`" + `) may be a value of headers,
query, form or upload, the whole body, or the path or query of url.

- Corbie does not escape a secret in body: the script escapes it for the text
  around it with secrets.encoded.
- A handle in json is an error.
- A request with a secret follows a redirect only to a host that the secret
  may go to.
- Corbie percent-encodes spaces and non-ASCII characters in url, and characters
  that a path cannot contain, such as " < and >. It changes nothing else: %2F
  stays %2F.
- A user name or a password in url is an error.
- Corbie adds User-Agent: corbie, and a Content-Type for json, form, upload and
  a blob, unless headers has them.
- Host, Content-Length, Transfer-Encoding, Trailer and Connection in headers
  are an error: Corbie writes them from url and the body.
- When headers has Accept-Encoding or Range, Corbie does not decompress body.
- Corbie follows at most 20 redirects, and checks each host as it checks the
  first.
- body as text, json or bytes is at most 64 MiB. A blob, received or sent, may
  be any size (` + "`corbie manual blobs`" + `).

Errors:
  refused     no connection, a certificate that is not trusted, a redirect that is not to
              http or https, or a 21st redirect
  timeout     no whole response within timeout
  not_found   the host's name does not resolve
  bad_input   a call written wrongly, such as a body on GET or HEAD, or a body over 64 MiB
              that is not a blob

Needs these permissions:
  net:host                           each host, those of redirects included
  secret:NAME@net:host               a secret sent to an https URL
  secret:NAME@net.unencrypted:host   a secret sent to an http URL

This script sends a secret in a header.

    allow: [net:api.github.com, "secret:GITHUB_TOKEN@net:api.github.com"]

    const r = http.get("https://api.github.com/user", {
      headers: { Authorization: secrets.text` + "`" + `Bearer ${secrets.get("GITHUB_TOKEN")}` + "`" + ` },
      as: "json",
    })
    if (r.status !== 200) throw new Error("GitHub said " + r.status)
    return r.body.login                 // gives "octocat"

These calls use the other methods.

    allow: [net:example.com]

    const orders = "https://example.com/orders"
    http.post(orders, { json: { item: "tea" }, as: "json" }).body    // gives { id: 1, item: "tea" }
    http.put(orders + "/1", { json: { item: "cake" } }).status       // gives 200
    http.patch(orders + "/1", { form: { item: "tea" } }).status      // gives 200
    http.head(orders + "/1").status                                  // gives 200
    http.delete(orders + "/1").status                                // gives 204
    http.request(orders, { method: "OPTIONS" }).status               // gives 204
`
