package gateway_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/permission"

	_ "github.com/zummed/corbie/statechart"
)

const door = `
statechart: door
initial: closed
states:
  closed:
    on: { open: opened }
  opened: { end: done, output: { by: "${event.data.who}" } }
`

type caller struct {
	t     *testing.T
	url   string
	token string
}

func (c caller) do(method, path, body string) (int, map[string]any, string) {
	c.t.Helper()
	return c.send(method, path, "", strings.NewReader(body))
}

func (c caller) send(method, path, kind string, body io.Reader) (int, map[string]any, string) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.url+path, body)
	req.Header.Set("Authorization", "Bearer "+c.token)
	if kind != "" {
		req.Header.Set("Content-Type", kind)
	}
	// An answer that does not come is a failure, not a test that hangs.
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var v map[string]any
	json.Unmarshal(data, &v)
	return resp.StatusCode, v, string(data)
}

// A part of an upload form: a field, or with a file's name a file.
type part struct{ name, filename, contents string }

// upload sends a start: an upload form of these parts.
func (c caller) upload(parts ...part) (int, map[string]any, string) {
	c.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, p := range parts {
		var w io.Writer
		var err error
		if p.filename == "" {
			w, err = form.CreateFormField(p.name)
		} else {
			w, err = form.CreateFormFile(p.name, p.filename)
		}
		if err != nil {
			c.t.Fatal(err)
		}
		io.WriteString(w, p.contents)
	}
	form.Close()
	return c.send("POST", "/runs", form.FormDataContentType(), &body)
}

// form gives the parts of a start that is a definition, the files beside
// it by their paths under "files", and anything else a start carries.
func form(c caller, request map[string]any) []part {
	c.t.Helper()
	var parts []part
	rest := map[string]any{}
	for name, value := range request {
		switch name {
		case "definition":
			parts = append(parts, part{name: "definition", contents: value.(string)})
		case "files":
			for at, text := range value.(map[string]string) {
				parts = append(parts, part{name: "file:" + at, filename: "a-file", contents: text})
			}
		default:
			rest[name] = value
		}
	}
	if len(rest) > 0 {
		data, err := json.Marshal(rest)
		if err != nil {
			c.t.Fatal(err)
		}
		parts = append(parts, part{name: "start", contents: string(data)})
	}
	return parts
}

func TestGateway(t *testing.T) {
	local, err := client.NewLocal(context.Background(), permission.Layer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	server := httptest.NewServer(&gateway.Gateway{Client: local.Client, Token: "the-token"})
	defer server.Close()
	c := caller{t: t, url: server.URL, token: "the-token"}

	// A request without the right token is refused and does nothing.
	if status, _, _ := (caller{t: t, url: server.URL, token: "the-toke"}).do("GET", "/runs", ""); status != http.StatusUnauthorized {
		t.Fatalf("a wrong token was answered %d", status)
	}
	// A field the gateway does not know is refused, and the answer names
	// it: in what a start carries, among the parts of its form, and in what
	// an event carries.
	if status, _, text := c.upload(part{name: "definition", contents: door}, part{name: "start", contents: `{"labels": {"a": "b"}, "ceiling": ["all"]}`}); status != 400 || !strings.Contains(text, "ceiling") {
		t.Fatalf("an unknown field was answered %d %s", status, text)
	}
	if status, _, text := c.upload(part{name: "definition", contents: door}, part{name: "ceiling", contents: "all"}); status != 400 || !strings.Contains(text, `a part named \"ceiling\"`) {
		t.Fatalf("an unknown part was answered %d %s", status, text)
	}
	// A definition with mistakes is refused with everything wrong.
	if status, _, text := c.upload(part{name: "definition", contents: "statechart: bad\nstates: {a: {on: {go: nowhere}}}"}); status != 400 || !strings.Contains(text, "nowhere") {
		t.Fatalf("a bad definition was answered %d %s", status, text)
	}
	// Nothing was started by any of them.
	if status, _, text := c.do("GET", "/runs?all", ""); status != 200 || strings.TrimSpace(text) != "[]" {
		t.Fatalf("after three refused starts the listing was answered %d %s", status, text)
	}
	// An ID that is no run is not found, at once, on every route.
	for _, path := range []string{"", "/record", "/record?watch", "/definition", "/cancel"} {
		method := "GET"
		if path == "/cancel" {
			method = "POST"
		}
		if status, _, _ := c.do(method, "/runs/client/20200101T000000.000Z-0000000000"+path, ""); status != 404 {
			t.Fatalf("no such run%s was answered %d", path, status)
		}
	}

	status, started, text := c.upload(form(c, map[string]any{"definition": door, "labels": map[string]string{"team": "ops"}, "input": nil})...)
	id, _ := started["id"].(string)
	if status != 200 || id == "" {
		t.Fatalf("a start was answered %d %s", status, text)
	}
	// Follow its record as a stream, in the background.
	stream := make(chan string, 1)
	go func() {
		req, _ := http.NewRequest("GET", server.URL+"/runs/"+id+"/record?watch", nil)
		req.Header.Set("Authorization", "Bearer the-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			stream <- err.Error()
			return
		}
		defer resp.Body.Close()
		var all strings.Builder
		lines := bufio.NewScanner(resp.Body)
		for lines.Scan() {
			all.WriteString(lines.Text() + "\n")
		}
		stream <- all.String()
	}()
	// An event the chart has a rule for: the answer is its receipt.
	if status, _, text := c.do("POST", "/runs/"+id+"/events", `{"event": "open", "date": {"who": "caller"}}`); status != 400 || !strings.Contains(text, "date") {
		t.Fatalf("an event with an unknown field was answered %d %s", status, text)
	}
	if status, receipt, text := c.do("POST", "/runs/"+id+"/events", `{"event": "open", "data": {"who": "caller"}}`); status != 200 || receipt["taken"] != true || receipt["acted"] != true {
		t.Fatalf("an event was answered %d %s", status, text)
	}
	status, run, text := c.do("GET", "/runs/"+id+"?wait", "")
	if status != 200 || run["status"] != "done" || run["id"] != id || run["output"].(map[string]any)["by"] != "caller" {
		t.Fatalf("the run was answered %d %s", status, text)
	}
	// The stream ends by saying how the run ended.
	got := <-stream
	if !strings.Contains(got, "event: entry") || !strings.Contains(got, "event: end") || !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("the stream of the record was:\n%s", got)
	}
	// The record so far is what the stream gave, without staying.
	status, record, text := c.do("GET", "/runs/"+id+"/record", "")
	if entries, _ := record["entries"].([]any); status != 200 || len(entries) != strings.Count(got, "event: entry") || !strings.Contains(text, `"run":{`) || !strings.Contains(text, `"status":"done"`) {
		t.Fatalf("the record so far was answered %d %s, and the stream was:\n%s", status, text, got)
	}
	if status, _, text := c.do("GET", "/runs?all&label=team=ops", ""); status != 200 || !strings.Contains(text, id) {
		t.Fatalf("the listing was answered %d %s", status, text)
	}
	if status, def, _ := c.do("GET", "/runs/"+id+"/definition", ""); status != 200 || def["name"] != "door" {
		t.Fatalf("the definition was answered %d %v", status, def)
	}
}

// Only the whole token is taken: one that is cut short, carried on, wrong in
// one letter or not there is refused, and a gateway that was given no token
// takes none.
func TestOnlyTheWholeTokenIsTaken(t *testing.T) {
	local, err := client.NewLocal(context.Background(), permission.Layer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	server := httptest.NewServer(&gateway.Gateway{Client: local.Client, Token: "the-token"})
	defer server.Close()
	for _, wrong := range []string{"", "t", "the-toke", "the-tokens", "the-tokem", "The-token", "the-token the-token"} {
		if status, _, text := (caller{t: t, url: server.URL, token: wrong}).do("GET", "/runs", ""); status != http.StatusUnauthorized {
			t.Errorf("the token %q was answered %d %s", wrong, status, text)
		}
	}
	if status, _, text := (caller{t: t, url: server.URL, token: "the-token"}).do("GET", "/runs", ""); status != 200 {
		t.Fatalf("the right token was answered %d %s", status, text)
	}
	// The token is shown as a bearer's: the right one shown bare, or under
	// another word, is refused.
	for _, shown := range []string{"the-token", "Basic the-token", "Bearer  the-token"} {
		req, _ := http.NewRequest("GET", server.URL+"/runs", nil)
		req.Header.Set("Authorization", shown)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization: %s was answered %d", shown, resp.StatusCode)
		}
	}
	none := httptest.NewServer(&gateway.Gateway{Client: local.Client})
	defer none.Close()
	for _, shown := range []string{"", "the-token"} {
		if status, _, text := (caller{t: t, url: none.URL, token: shown}).do("GET", "/runs", ""); status != http.StatusUnauthorized {
			t.Errorf("a gateway with no token answered %q with %d %s", shown, status, text)
		}
	}
}
