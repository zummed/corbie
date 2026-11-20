package page

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func Test_page_header_offers_the_parties_pages(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, Shell{Title: "Runs", Current: "Runs", Offered: []string{"Runs", "Machines"}, Parties: []string{"laptop-client"}}, struct{}{})
	body := w.Body.String()
	for _, want := range []string{`<a href="/runs" aria-current="page">Runs</a>`, `<a href="/machines">Machines</a>`, `<code>laptop-client</code>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the header does not hold %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "/controller") {
		t.Errorf("the header offers the Controller page to a UI that is not the controller")
	}
	w = httptest.NewRecorder()
	Write(w, Shell{Title: "Runs", Current: "Runs", Offered: []string{"Runs"}}, struct{}{})
	if !strings.Contains(w.Body.String(), "a viewer") {
		t.Errorf("a UI with no party does not say it is a viewer")
	}
}

// parts is a page whose parts change when it is told to, or fail to be
// read.
type parts struct {
	mu      sync.Mutex
	shown   map[string]any
	err     error
	changed chan struct{}
}

func (p *parts) set(shown map[string]any, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shown, p.err = shown, err
	close(p.changed)
	p.changed = make(chan struct{})
}

func (p *parts) read(context.Context) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.shown, p.err
}

func (p *parts) Changed() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.changed
}

func follow(t *testing.T, p *parts) (*websocket.Conn, func() map[string]any) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Stream(w, r, time.Hour, p.Changed, p.read)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn, func() map[string]any {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		return m
	}
}

func Test_page_follows_over_a_websocket(t *testing.T) {
	p := &parts{shown: map[string]any{"now": map[string]int{"runs": 1}}, changed: make(chan struct{})}
	_, next := follow(t, p)
	if m := next(); m["type"] != "part" || m["id"] != "now" || fmt.Sprint(m["data"]) != "map[runs:1]" {
		t.Errorf("the page was sent %v over its WebSocket", m)
	}
}

func Test_page_stream_sends_every_part_then_what_changed(t *testing.T) {
	p := &parts{shown: map[string]any{"a": "one", "b": "two"}, changed: make(chan struct{})}
	_, next := follow(t, p)
	first := map[any]any{}
	for range 2 {
		m := next()
		first[m["id"]] = m["data"]
	}
	if first["a"] != "one" || first["b"] != "two" {
		t.Fatalf("the page was not sent every part as it connected: %v", first)
	}
	p.set(map[string]any{"a": "one", "b": "three"}, nil)
	if m := next(); m["id"] != "b" || m["data"] != "three" {
		t.Errorf("after b changed, the page was sent %v", m)
	}
	p.set(map[string]any{"a": "one", "b": "three"}, errors.New("the store cannot be read"))
	if m := next(); m["type"] != "store" || m["store"] != "lost" {
		t.Errorf("a page was not told the store is lost, but sent %v", m)
	}
	p.set(map[string]any{"a": "four", "b": "three"}, nil)
	if m := next(); m["store"] != "live" {
		t.Errorf("a page was not told the store is read again, but sent %v", m)
	}
	if m := next(); m["id"] != "a" || m["data"] != "four" {
		t.Errorf("after the store was read again, the page was sent %v", m)
	}
}
