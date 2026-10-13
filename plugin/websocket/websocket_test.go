package websocket

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

func newJob(t *testing.T, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}},
		Scratch: t.TempDir(),
		Secrets: map[string]string{"TOKEN": "s3cret-token"},
	}
	t.Cleanup(job.End)
	return job
}

// serve starts a server on this machine and hands each connection to
// handle, giving the server's address.
func serve(t *testing.T, options *websocket.AcceptOptions, handle func(context.Context, *websocket.Conn, *http.Request)) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, options)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handle(r.Context(), conn, r)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func echo(ctx context.Context, conn *websocket.Conn, _ *http.Request) {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		conn.Write(ctx, kind, data)
	}
}

func connectTo(job *plugin.Job, address string, opt plugin.Options) (any, error) {
	module, _ := plugin.Find("websocket")
	return module.Calls["connect"].Invoke(job, "websocket", "connect", address, opt)
}

func open(t *testing.T, job *plugin.Job, address string, opt plugin.Options) *plugin.Object {
	t.Helper()
	ws, err := connectTo(job, address, opt)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return ws.(*plugin.Object)
}

func call(job *plugin.Job, ws *plugin.Object, name string, first any) (any, error) {
	return ws.Calls[name].Invoke(job, "object", name, first, nil)
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func Test_websocket_text_and_bytes_each_way(t *testing.T) {
	job := newJob(t, "all")
	ws := open(t, job, serve(t, nil, echo), nil)
	for _, message := range []any{"hello", plugin.Bytes{0, 1, 2, 0xff}, plugin.NewBlob("", []byte("in a blob"))} {
		if _, err := call(job, ws, "send", message); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := call(job, ws, "receive", nil); err != nil || got != "hello" {
		t.Fatalf("receive gave %v, %v", got, err)
	}
	if got, err := call(job, ws, "receive", nil); err != nil || !bytes.Equal(got.(plugin.Bytes), []byte{0, 1, 2, 0xff}) {
		t.Fatalf("receive gave %v, %v", got, err)
	}
	if got, err := call(job, ws, "receive", nil); err != nil || string(got.(plugin.Bytes)) != "in a blob" {
		t.Fatalf("receive gave %v, %v", got, err)
	}
	if _, err := call(job, ws, "send", map[string]any{"not": "a message"}); code(err) != plugin.BadInput {
		t.Fatalf("sending a map gave %v", err)
	}
	_, err := call(job, ws, "receive", map[string]any{"timeuot": 1.0})
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"timeuot"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
}

func Test_websocket_headers_and_secrets(t *testing.T) {
	address := serve(t, nil, func(ctx context.Context, conn *websocket.Conn, r *http.Request) {
		conn.Write(ctx, websocket.MessageText, []byte(r.Header.Get("Authorization")+" "+r.Header.Get("X-Plain")))
		echo(ctx, conn, r)
	})
	bearer, _ := plugin.Join("Bearer ", plugin.NewSecret("TOKEN"))
	headers := map[string]any{"Authorization": bearer, "X-Plain": "as written"}
	job := newJob(t, "all")
	ws := open(t, job, address, plugin.Options{"headers": headers})
	if got, err := call(job, ws, "receive", nil); err != nil || got != "Bearer s3cret-token as written" {
		t.Fatalf("the server was sent %v, %v", got, err)
	}
	if _, err := call(job, ws, "send", plugin.NewSecret("TOKEN")); err != nil {
		t.Fatal(err)
	}
	if got, err := call(job, ws, "receive", nil); err != nil || got != "s3cret-token" {
		t.Fatalf("receive gave %v, %v", got, err)
	}

	// Over a plain connection a secret bound only to the host does not go.
	host := strings.TrimPrefix(address, "ws://")
	_, err := connectTo(newJob(t, "net:"+host, "secret:TOKEN@net:"+host), address, plugin.Options{"headers": headers})
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || refusal.What != "secret:TOKEN@net.unencrypted:"+host {
		t.Fatalf("a secret bound to a host, over a plain connection, gave %v", err)
	}
}

// An address goes out as it was written: what is escaped in it already
// stays escaped, also beside something that has to be escaped on the way.
func TestAddressGoesOutAsWritten(t *testing.T) {
	asked := make(chan string, 1)
	address := serve(t, nil, func(ctx context.Context, conn *websocket.Conn, r *http.Request) {
		asked <- r.RequestURI
		echo(ctx, conn, r)
	})
	job := newJob(t, "network")
	open(t, job, address+"/a%2Fb/x y/é?q=%2F é", nil)
	if got, want := <-asked, "/a%2Fb/x%20y/%C3%A9?q=%2F%20%C3%A9"; got != want {
		t.Fatalf("the address arrived as %s, and was written to arrive as %s", got, want)
	}
}

func Test_websocket_close(t *testing.T) {
	closed := make(chan error, 1)
	address := serve(t, nil, func(ctx context.Context, conn *websocket.Conn, _ *http.Request) {
		_, _, err := conn.Read(ctx)
		closed <- err
	})
	job := newJob(t, "all")
	ws := open(t, job, address, nil)
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-closed; websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("the other end saw %v, not the connection closed", err)
	}
	if _, err := call(job, ws, "send", "more"); code(err) != plugin.BadInput {
		t.Fatalf("a call after close gave %v", err)
	}
	if _, err := call(job, ws, "receive", nil); code(err) != plugin.BadInput {
		t.Fatalf("a call after close gave %v", err)
	}
}

func Test_websocket_other_side_closes(t *testing.T) {
	job := newJob(t, "all")
	politely := serve(t, nil, func(ctx context.Context, conn *websocket.Conn, _ *http.Request) {
		conn.Write(ctx, websocket.MessageText, []byte("last"))
		conn.Close(websocket.StatusNormalClosure, "")
	})
	ws := open(t, job, politely, nil)
	for _, want := range []any{"last", nil, nil} {
		if got, err := call(job, ws, "receive", nil); err != nil || got != want {
			t.Fatalf("receive gave %v, %v, not %v", got, err, want)
		}
	}
	broken := serve(t, nil, func(context.Context, *websocket.Conn, *http.Request) {})
	ws = open(t, job, broken, nil)
	if _, err := call(job, ws, "receive", nil); code(err) != plugin.Refused {
		t.Fatalf("a connection that broke gave %v", err)
	}
}

func Test_websocket_kept_alive_while_the_script_works(t *testing.T) {
	pingEvery = 20 * time.Millisecond
	t.Cleanup(func() { pingEvery = 30 * time.Second })
	var asked atomic.Int32
	answers := make(chan error, 8)
	options := &websocket.AcceptOptions{OnPingReceived: func(context.Context, []byte) bool {
		asked.Add(1)
		return true
	}}
	address := serve(t, options, func(ctx context.Context, conn *websocket.Conn, _ *http.Request) {
		ctx = conn.CloseRead(ctx)
		for _, text := range []string{"one", "two", "three"} {
			conn.Write(ctx, websocket.MessageText, []byte(text))
		}
		// The server asks, again and again, whether the client is there.
		for range cap(answers) {
			ping, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			answers <- conn.Ping(ping)
			cancel()
			time.Sleep(30 * time.Millisecond)
		}
		<-ctx.Done()
	})
	job := newJob(t, "all")
	ws := open(t, job, address, nil)
	// The script does other things, and makes no call on the connection.
	for range cap(answers) {
		if err := <-answers; err != nil {
			t.Fatalf("the server's ping was not answered: %v", err)
		}
	}
	if asked.Load() == 0 {
		t.Fatal("the other side was never pinged")
	}
	for _, want := range []string{"one", "two", "three"} {
		if got, err := call(job, ws, "receive", nil); err != nil || got != want {
			t.Fatalf("receive gave %v, %v, not %q", got, err, want)
		}
	}
	// It is still open: a wait for more runs out of time, and is not told
	// the connection ended.
	if _, err := call(job, ws, "receive", map[string]any{"timeout": "50ms"}); code(err) != plugin.TimedOut {
		t.Fatalf("receive on the open connection gave %v", err)
	}
}

func Test_plug_every_wait_has_a_deadline(t *testing.T) {
	wait = 100 * time.Millisecond
	t.Cleanup(func() { wait = 30 * time.Second })
	job := newJob(t, "all")
	ws := open(t, job, serve(t, nil, echo), nil)
	if _, err := call(job, ws, "receive", nil); code(err) != plugin.TimedOut {
		t.Fatalf("receive, with no time given, when nothing arrives gave %v", err)
	}
	// The wait ended and not the connection.
	if _, err := call(job, ws, "send", "after"); err != nil {
		t.Fatal(err)
	}
	if got, err := call(job, ws, "receive", nil); err != nil || got != "after" {
		t.Fatalf("receive gave %v, %v", got, err)
	}

	// A server that takes the connection and never answers.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go io.Copy(io.Discard, conn)
		}
	}()
	if _, err := connectTo(job, "ws://"+l.Addr().String(), nil); code(err) != plugin.TimedOut {
		t.Fatalf("connect, with no time given, to a server that says nothing gave %v", err)
	}
}

func Test_plug_left_open_closed_at_job_end(t *testing.T) {
	closed := make(chan error, 1)
	address := serve(t, nil, func(ctx context.Context, conn *websocket.Conn, _ *http.Request) {
		_, _, err := conn.Read(ctx)
		closed <- err
	})
	job := newJob(t, "all")
	open(t, job, address, nil)
	job.End()
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("the other end read a message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the connection outlived the job")
	}
}

func Test_websocket_refused_without_permission(t *testing.T) {
	var seen atomic.Bool
	address := serve(t, nil, func(context.Context, *websocket.Conn, *http.Request) { seen.Store(true) })
	_, err := connectTo(newJob(t), address, nil)
	var refusal *permission.Refusal
	if want := "net:" + strings.TrimPrefix(address, "ws://"); !errors.As(err, &refusal) || refusal.What != want {
		t.Fatalf("connect gave %v, not a refusal naming %s", err, want)
	}
	time.Sleep(50 * time.Millisecond)
	if seen.Load() {
		t.Fatal("the server saw a connection")
	}
}

func Test_plug_wrapped_library_makes_no_lookup(t *testing.T) {
	// A proxy the machine is set to use is not used, and a name reaches
	// the check as it was written, not looked up.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	for address, want := range map[string]string{
		"ws://feed.corbie.invalid:8080/v1": "net:feed.corbie.invalid:8080",
		"ws://feed.corbie.invalid/v1":      "net:feed.corbie.invalid:80",
		"wss://Feed.Corbie.invalid/v1":     "net:feed.corbie.invalid:443",
	} {
		_, err := connectTo(newJob(t), address, nil)
		var refusal *permission.Refusal
		if !errors.As(err, &refusal) || refusal.What != want {
			t.Fatalf("connecting to %s gave %v, not a refusal naming %s", address, err, want)
		}
	}
}
