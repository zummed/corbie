package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

// own are the headers a request to a server carries that are the
// plugin's to write and not a script's.
var own = []string{"Accept", "Content-Type", "MCP-Protocol-Version", "Mcp-Method", "Mcp-Name", "Mcp-Session-Id"}

// connection is the connection a web address is reached over: its host and
// port, and whether it is encrypted.
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

// address reads a web address that the script or a server gave.
func address(text string) (*url.URL, error) {
	to, err := url.Parse(plugin.EscapedAddress(text))
	if err != nil || to.Host == "" || (to.Scheme != "http" && to.Scheme != "https") {
		return nil, plugin.Errorf(plugin.BadInput, "%q is not an address starting https:// or http://", text)
	}
	if to.User != nil {
		return nil, plugin.Errorf(plugin.BadInput, "an address may not hold a user name or a password: send a sign-in as a header or as client")
	}
	return to, nil
}

func connect(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	text, err := plugin.Text(first, "the server's address, starting https:// or http://")
	if err != nil {
		return nil, err
	}
	to, err := address(text)
	if err != nil {
		return nil, err
	}
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	given, err := opt.Map("headers")
	if err != nil {
		return nil, err
	}
	over, encrypted := connection(to)
	headers, err := job.Headers(given, over, encrypted, own...)
	if err != nil {
		return nil, err
	}
	for name := range headers {
		if strings.HasPrefix(strings.ToLower(name), "mcp-param-") {
			return nil, plugin.Errorf(plugin.BadInput, "the header %s carries a tool's argument, which the plugin writes", name)
		}
	}
	signIn, err := clientOf(opt)
	if err != nil {
		return nil, err
	}
	if signIn != nil && headers.Get("Authorization") != "" {
		return nil, plugin.Errorf(plugin.BadInput, "a sign-in is given as a header or as client, not both")
	}
	w := &web{job: job, to: to, headers: headers, signIn: signIn}
	s := &server{way: w, name: to.Host, web: true, limit: limit}
	job.OnEnd(func() { s.close() })
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	if err := s.agree(ctx); err != nil {
		s.close()
		return nil, s.failure(ctx, err)
	}
	return s.object(), nil
}

// web is the way to a server over the web: each message is a request of
// its own.
type web struct {
	job     *plugin.Job
	to      *url.URL
	headers http.Header // the script's, with any secret in them revealed
	signIn  *client
	// token is what the sign-in gave, which goes to this server and to no
	// other.
	token string
	// session is what a server of an earlier version names the
	// conversation by, from its handshake on.
	session string
}

func (w *web) ask(ctx context.Context, m *message, headers map[string]string) (*message, error) {
	return w.post(ctx, m, headers, false)
}

func (w *web) tell(ctx context.Context, m *message, headers map[string]string) error {
	_, err := w.post(ctx, m, headers, false)
	return err
}

// do sends a request on a connection of its own, and gives the answer's
// status, headers and body.
func (w *web) do(req *http.Request) (int, http.Header, io.ReadCloser, error) {
	job := w.job
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil, // nothing of the machine's own settings is used
			// A connection is the outcome of one check, so each request opens
			// its own and none is kept for another.
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return job.Dial(ctx, address)
			},
		},
		// A redirect would carry the headers to a place they were not
		// revealed for.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, resp.Body, nil
}

// close ends the conversation with a server of an earlier version, which
// may be keeping it.
func (w *web) close() {
	if w.session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(w.job.Context, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "DELETE", w.to.String(), nil)
	if err != nil {
		return
	}
	w.write(req, nil)
	if _, _, body, err := w.do(req); err == nil {
		body.Close()
	}
}

// write gives a request the headers every request to the server has.
func (w *web) write(req *http.Request, headers map[string]string) {
	req.Header = w.headers.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header["Accept"] = []string{"application/json, text/event-stream"}
	req.Header["Content-Type"] = []string{"application/json"}
	for name, value := range headers {
		req.Header[name] = []string{value}
	}
	if w.session != "" {
		req.Header["Mcp-Session-Id"] = []string{w.session}
	}
	if w.token != "" {
		req.Header["Authorization"] = []string{"Bearer " + w.token}
	}
}

func (w *web) post(ctx context.Context, m *message, headers map[string]string, signedIn bool) (*message, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", w.to.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	w.write(req, headers)
	status, header, got, err := w.do(req)
	if err != nil {
		return nil, err
	}
	defer got.Close()
	if session := header.Get("Mcp-Session-Id"); session != "" {
		w.session = session
	}
	read := io.LimitReader(got, plugin.MaxWhole)
	switch {
	case status == 401 && w.signIn != nil && !signedIn:
		if w.token, err = w.signIn.token(ctx, w, header.Get("WWW-Authenticate")); err != nil {
			return nil, err
		}
		return w.post(ctx, m, headers, true)
	case status == 401 || status == 403:
		return nil, plugin.Errorf(plugin.Refused, "the server %s refused the request (%d): %s", w.to.Host, status, challenged(header.Get("WWW-Authenticate")))
	case status >= 300 && status < 400:
		return nil, plugin.Errorf(plugin.Refused, "the server %s answered with a redirect to %s, and the plugin follows none", w.to.Host, header.Get("Location"))
	case m.ID == nil && status >= 200 && status < 300:
		return nil, nil
	}
	kind := strings.TrimSpace(strings.Split(header.Get("Content-Type"), ";")[0])
	if status >= 200 && status < 300 && kind == "text/event-stream" {
		return w.events(read, m)
	}
	whole, err := io.ReadAll(read)
	if err != nil {
		return nil, err
	}
	var answer message
	if json.Unmarshal(whole, &answer) == nil && (answer.Result != nil || answer.Error != nil) {
		return &answer, nil
	}
	if status >= 200 && status < 300 {
		return nil, plugin.Errorf(plugin.Failed, "the server %s answered with something that is not JSON-RPC: %s", w.to.Host, cut(whole))
	}
	// An answer with no JSON-RPC error in it is one no server of latest
	// gives.
	return nil, &unmodern{fmt.Sprintf("%d %s", status, cut(whole))}
}

// events reads an answer sent as server-sent events: notifications about
// the request, then its answer.
func (w *web) events(body io.Reader, m *message) (*message, error) {
	lines := bufio.NewReader(body)
	var data []string
	for {
		line, err := lines.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(after, " "))
		}
		if (line == "" || err != nil) && len(data) > 0 {
			var answer message
			if json.Unmarshal([]byte(strings.Join(data, "\n")), &answer) == nil && answer.Method == "" && fmt.Sprint(answer.ID) == fmt.Sprint(m.ID) {
				return &answer, nil
			}
			data = nil
		}
		if err != nil {
			return nil, plugin.Errorf(plugin.Failed, "the server %s ended its answer without one", w.to.Host)
		}
	}
}

// challenged says what a refusal of a sign-in said, from its
// WWW-Authenticate header.
func challenged(header string) string {
	said := challenge(header)
	switch {
	case said["error_description"] != "":
		return said["error_description"]
	case said["error"] != "":
		return said["error"]
	}
	return "it wants a sign-in, given as a header or as client"
}

// cut gives the start of a text, for an error to show.
func cut(text []byte) string {
	if len(text) > 200 {
		return string(text[:200]) + "…"
	}
	return string(text)
}
