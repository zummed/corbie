package role_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/role"
)

// requests stands in for the part of a serverless host an HTTP server asks
// for its calls: it hands out each event as a call that may run for the time
// given, and keeps what each call was answered with. read, when given, is
// told of each part of an answer as it arrives.
type requests struct {
	mu      sync.Mutex
	events  []string
	limits  []time.Duration
	asked   int
	answers map[string]string // by call: the whole answer
	modes   map[string]string // by call: how the answer said it is sent
	read    func(call, part string)
	done    func()
}

func (h *requests) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/invocation/next"):
		h.mu.Lock()
		if h.asked >= len(h.events) {
			h.mu.Unlock()
			<-r.Context().Done() // no more calls
			return
		}
		event, limit := h.events[h.asked], h.limits[h.asked]
		h.asked++
		w.Header().Set("Lambda-Runtime-Aws-Request-Id", "call-"+strconv.Itoa(h.asked))
		h.mu.Unlock()
		w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.FormatInt(time.Now().Add(limit).UnixMilli(), 10))
		io.WriteString(w, event)
	case strings.HasSuffix(r.URL.Path, "/response"):
		call := strings.Split(r.URL.Path, "/")[4]
		var whole bytes.Buffer
		part := make([]byte, 4096)
		for {
			n, err := r.Body.Read(part)
			whole.Write(part[:n])
			if h.read != nil && n > 0 {
				h.read(call, string(part[:n]))
			}
			if err != nil {
				break
			}
		}
		h.mu.Lock()
		h.answers[call] = whole.String()
		h.modes[call] = r.Header.Get("Lambda-Runtime-Function-Response-Mode") + " " + strings.Join(r.TransferEncoding, ",") + " " + r.Header.Get("Content-Type")
		last := len(h.answers) == len(h.events)
		h.mu.Unlock()
		if last && h.done != nil {
			h.done()
		}
	}
}

// answered gives the status and headers a call was answered with, and the
// body: the two are apart by eight zero bytes.
func (h *requests) answered(t *testing.T, call string) (first, body string) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	first, body, apart := strings.Cut(h.answers[call], strings.Repeat("\x00", 8))
	if !apart {
		t.Fatalf("the answer to %s has no eight zero bytes between its headers and its body: %q", call, h.answers[call])
	}
	return first, body
}

// asked is a request as AWS Lambda gives one that came to a function's URL.
func asked(method, path, query, body string, encoded bool, headers map[string]string) string {
	var held []string
	for name, value := range headers {
		held = append(held, fmt.Sprintf("%q: %q", name, value))
	}
	return fmt.Sprintf(`{"version": "2.0", "rawPath": %q, "rawQueryString": %q, "headers": {%s}, "body": %q, "isBase64Encoded": %v,
		"requestContext": {"http": {"method": %q, "path": %q}}}`, path, query, strings.Join(held, ", "), body, encoded, method, path)
}

// On a serverless host each call is one HTTP request: the handler is given
// its method, path, query, headers and body, what it writes is the answer,
// sent as it is written, and its time ends before the host's limit.
func Test_role_serverless_host_calls_an_http_server_for_each_request(t *testing.T) {
	serving, stop := context.WithCancel(context.Background())
	defer stop()
	sawFirst := make(chan struct{})
	host := &requests{
		events: []string{
			asked("POST", "/things/one", "a=1&b=two", base64.StdEncoding.EncodeToString([]byte("the body\x00\xff")), true, map[string]string{"authorization": "Bearer t", "content-type": "application/octet-stream", "host": "fn.example.com"}),
			asked("GET", "/slow", "", "", false, nil),
			asked("GET", "/waits", "", "", false, nil),
			`{"Records": [{"s3": {}}]}`,
		},
		limits:  []time.Duration{30 * time.Second, 30 * time.Second, 600 * time.Millisecond, 30 * time.Second},
		answers: map[string]string{}, modes: map[string]string{},
		done: stop,
	}
	var once sync.Once
	host.read = func(call, part string) {
		if call == "call-2" && strings.Contains(part, "first") {
			once.Do(func() { close(sawFirst) })
		}
	}
	server := httptest.NewServer(host)
	defer server.Close()
	t.Setenv("AWS_LAMBDA_RUNTIME_API", strings.TrimPrefix(server.URL, "http://"))
	defer time.AfterFunc(20*time.Second, stop).Stop()

	var given struct{ method, path, query, auth, kind, host, body string }
	var waited time.Duration
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/things/one":
			body, _ := io.ReadAll(r.Body)
			given.method, given.path, given.query, given.body = r.Method, r.URL.Path, r.URL.RawQuery, string(body)
			given.auth, given.kind, given.host = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.Host
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "11")
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"ok":true}`)
		case "/slow":
			// The second part is written only once the host has the first.
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "first\n")
			w.(http.Flusher).Flush()
			select {
			case <-sawFirst:
				io.WriteString(w, "second\n")
			case <-time.After(10 * time.Second):
				io.WriteString(w, "the host was not given the first part while the answer was being written\n")
			}
		case "/waits":
			began := time.Now()
			<-r.Context().Done()
			waited = time.Since(began)
			io.WriteString(w, "as it stands")
		}
	})
	var reported []string
	if err := role.Answer(serving, func(err error) { reported = append(reported, err.Error()) }, handler); err != nil {
		t.Fatal(err)
	}

	if given.method != "POST" || given.path != "/things/one" || given.query != "a=1&b=two" || given.body != "the body\x00\xff" ||
		given.auth != "Bearer t" || given.kind != "application/octet-stream" || given.host != "fn.example.com" {
		t.Errorf("the handler was given %+v", given)
	}
	first, body := host.answered(t, "call-1")
	if !strings.Contains(first, `"statusCode":201`) || !strings.Contains(first, `"Content-Type":"application/json"`) || strings.Contains(first, "Content-Length") || body != `{"ok":true}` {
		t.Errorf("the first call was answered %s and %q", first, body)
	}
	if mode := host.modes["call-1"]; mode != "streaming chunked application/vnd.awslambda.http-integration-response" {
		t.Errorf("the answer said it is sent as %q", mode)
	}
	if first, body := host.answered(t, "call-2"); !strings.Contains(first, `"statusCode":200`) || body != "first\nsecond\n" {
		t.Errorf("the second call was answered %s and %q", first, body)
	}
	// A request's time ends before the host's limit, and it is answered.
	if _, body := host.answered(t, "call-3"); body != "as it stands" || waited > 550*time.Millisecond {
		t.Errorf("the third call was answered %q after %s, of the 600ms the host gave", body, waited)
	}
	// A call that is no HTTP request is refused, and told to the owner.
	if first, body := host.answered(t, "call-4"); !strings.Contains(first, `"statusCode":400`) || !strings.Contains(body, "function URL") {
		t.Errorf("a call that is no HTTP request was answered %s and %q", first, body)
	}
	if len(reported) != 1 || !strings.Contains(reported[0], "not an HTTP request") {
		t.Errorf("the owner was told %q", reported)
	}
}

// A gateway on a serverless host is a function of its own: it answers each
// request the host gives it as the client its token names, takes its tokens
// from a file or an issuer, and is started beside no other role.
func Test_role_serverless_host_calls_a_gateway_for_each_request(t *testing.T) {
	held := &bucket{values: map[string][]byte{}}
	served := httptest.NewServer(held)
	defer served.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "the-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "the-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	config := setUp(t, "s3://bkt/main?region=test-1&endpoint="+served.URL, func() bool { return held.has("/inbox/") })
	tokens := filepath.Join(t.TempDir(), "tokens")
	os.WriteFile(tokens, []byte("alices host-client alice@example.com\n"), 0o600)
	// A serverless host unpacks a package readable by everyone.
	os.Chmod(filepath.Join(config, "client", "identity"), 0o644)
	t.Setenv("TMPDIR", t.TempDir())

	serving, stop := context.WithCancel(ctx)
	host := &requests{
		events: []string{
			asked("GET", "/client", "", "", false, map[string]string{"authorization": "Bearer alices"}),
			asked("GET", "/client", "", "", false, map[string]string{"cookie": "token=alices"}),
			asked("OPTIONS", "/runs", "", "", false, map[string]string{"origin": "https://app.example.com"}),
			asked("GET", "/runs", "watch", "", false, map[string]string{"authorization": "Bearer alices"}),
		},
		limits:  []time.Duration{30 * time.Second, 30 * time.Second, 30 * time.Second, 2 * time.Second},
		answers: map[string]string{}, modes: map[string]string{},
		done: stop,
	}
	server := httptest.NewServer(host)
	defer server.Close()
	t.Setenv("AWS_LAMBDA_RUNTIME_API", strings.TrimPrefix(server.URL, "http://"))
	defer time.AfterFunc(30*time.Second, stop).Stop()

	// With no token file and no issuer each copy would make its own token.
	if status, said := corbie(serving, "start", "gateway", "--config", config); status == 0 || !strings.Contains(said, "each copy of the function would make a token of its own") {
		t.Fatalf("with no token file, corbie start gateway ended %d: %s", status, said)
	}
	// Beside a controller it is refused: the host calls each for another thing.
	if status, said := corbie(serving, "start", "controller", "gateway", "--token-file", tokens, "--config", config); status == 0 || !strings.Contains(said, "a gateway is a function of its own") {
		t.Fatalf("beside a controller, corbie start gateway ended %d: %s", status, said)
	}
	status, said := corbie(serving, "start", "gateway", "--client", config, "--token-file", tokens, "--config", config)
	if status != 0 || !strings.Contains(said, "gateway for the client host-client, answering the requests to this function's URL") {
		t.Fatalf("corbie start gateway ended %d: %s", status, said)
	}
	if first, body := host.answered(t, "call-1"); !strings.Contains(first, `"statusCode":200`) || strings.TrimSpace(body) != `{"client":"host-client","principal":"alice@example.com"}` {
		t.Errorf("a request with a token was answered %s and %q", first, body)
	}
	if first, body := host.answered(t, "call-2"); !strings.Contains(first, `"statusCode":401`) || !strings.Contains(body, "does not show a token") {
		t.Errorf("a request with a cookie and no token was answered %s and %q", first, body)
	}
	if first, _ := host.answered(t, "call-3"); !strings.Contains(first, `"statusCode":204`) || !strings.Contains(first, `"Access-Control-Allow-Origin":"*"`) {
		t.Errorf("a browser's question before a call was answered %s", first)
	}
	// A stream is sent, and ends before the host's limit.
	if first, body := host.answered(t, "call-4"); !strings.Contains(first, `"Content-Type":"text/event-stream"`) || !strings.Contains(body, `"listed":true`) {
		t.Errorf("a stream was answered %s and %q", first, body)
	}
}

// corbie up on a serverless host is a whole Corbie in each copy of the
// function: a controller, a runner and a gateway on a store of the copy's
// own. The host calls for each request of the gateway, and a start that
// waits is answered with the run, its blobs with their bytes.
func Test_role_serverless_host_runs_a_whole_corbie(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.yaml"), []byte("flow: hello\nstages:\n  greet: { job: { script: ./greet.js, input: \"${input}\" } }\noutput: ${stages.greet.output}\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "greet.js"), []byte("return { said: \"hello, \" + input.name, file: blobs.make(\"zebra\", { type: \"text/plain\" }) }\n"), 0o600)
	tokens := filepath.Join(t.TempDir(), "tokens")
	os.WriteFile(tokens, []byte("the-token\n"), 0o600)
	t.Setenv("TMPDIR", t.TempDir())
	t.Chdir(t.TempDir())

	serving, stop := context.WithCancel(ctx)
	start := base64.StdEncoding.EncodeToString([]byte(`{"at": "/work/hello", "input": {"name": "host"}}`))
	headers := map[string]string{"authorization": "Bearer the-token", "content-type": "application/json"}
	host := &requests{
		events: []string{
			asked("POST", "/runs", "wait", start, true, headers),
		},
		limits:  []time.Duration{60 * time.Second},
		answers: map[string]string{}, modes: map[string]string{},
		done: stop,
	}
	server := httptest.NewServer(host)
	defer server.Close()
	t.Setenv("AWS_LAMBDA_RUNTIME_API", strings.TrimPrefix(server.URL, "http://"))
	defer time.AfterFunc(60*time.Second, stop).Stop()

	// A store named there would be one store for copies that each set it up.
	if status, said := corbie(serving, "up", "--in", dir, "--store", t.TempDir(), "gateway", "--token-file", tokens); status == 0 || !strings.Contains(said, "takes no --store") {
		t.Fatalf("corbie up --store ended %d: %s", status, said)
	}
	status, said := corbie(serving, "up", "--in", dir, "--at", "/work", "gateway", "--token-file", tokens)
	if status != 0 || !strings.Contains(said, "/work/hello") || !strings.Contains(said, "answering the requests to this function's URL") ||
		!strings.Contains(said, "which is this copy's alone") {
		t.Fatalf("corbie up ended %d: %s", status, said)
	}
	first, body := host.answered(t, "call-1")
	if !strings.Contains(first, `"statusCode":200`) || !strings.Contains(body, `"status":"done"`) || !strings.Contains(body, `"said":"hello, host"`) {
		t.Errorf("a start that waits was answered %s and %s", first, body)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("TMPDIR"), ".corbie", "store", "version")); err != nil {
		t.Errorf("the store is not in a directory of the copy's own: %v", err)
	}
}
