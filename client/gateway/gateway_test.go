package gateway_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"

	_ "github.com/zummed/corbie/machine/statechart"
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
	// client is the gateway's client, which submits the trees the gateway
	// starts runs in.
	client *client.Client
}

func (c caller) do(method, path, body string) (int, map[string]any, string) {
	c.t.Helper()
	return c.send(method, path, "", strings.NewReader(body))
}

// listed gives a listing once it holds the run, or as it is after 5
// seconds. A listing reads heads, and a head is written a moment after the
// record and the receipt.
func (c caller) listed(path, id string) (int, string) {
	c.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		status, _, text := c.do("GET", path, "")
		if status != 200 || strings.Contains(text, id) || time.Now().After(deadline) {
			return status, text
		}
	}
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
	return c.uploadTo("/runs", parts...)
}

// uploadTo sends an upload form of these parts to a route.
func (c caller) uploadTo(route string, parts ...part) (int, map[string]any, string) {
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
	return c.send("POST", route, form.FormDataContentType(), &body)
}

// placed counts the trees the tests submit, each at a prefix of its own.
var placed atomic.Int64

// place submits a definition and the files beside it as a tree of one file,
// through the gateway's client, and gives its location once the submit is
// taken.
func (c caller) place(src string, files map[string]string) string {
	c.t.Helper()
	draft, err := machine.Parse([]byte(src), machine.Given(files))
	if err != nil {
		c.t.Fatalf("the definition does not pass:\n%v", err)
	}
	name := draft.Definition.Name
	at := fmt.Sprintf("/%s-%d", name, placed.Add(1))
	ctx := context.Background()
	message, err := c.client.Submit(ctx, &client.Tree{Prefix: at, Machines: []*client.Machine{{Path: name + ".yaml", Location: at, Draft: draft}}}, 0)
	if err != nil {
		c.t.Fatal(err)
	}
	receipt, err := c.client.WaitReceipt(ctx, message, time.Minute, 10*time.Millisecond)
	if err != nil || !receipt.Taken {
		c.t.Fatalf("the submit of %s has the receipt %+v: %v", at, receipt, err)
	}
	return at
}

// form gives the parts of a start: the start as JSON, with its location
// "at", or a definition under "definition" and the files beside it by
// their paths under "files", which are placed first and started at their
// location.
func form(c caller, request map[string]any) []part {
	c.t.Helper()
	rest := map[string]any{}
	for name, value := range request {
		if name != "definition" && name != "files" {
			rest[name] = value
		}
	}
	if src, given := request["definition"].(string); given {
		files, _ := request["files"].(map[string]string)
		rest["at"] = c.place(src, files)
	}
	data, err := json.Marshal(rest)
	if err != nil {
		c.t.Fatal(err)
	}
	return []part{{name: "start", contents: string(data)}}
}

func TestGateway(t *testing.T) {
	local, err := client.NewLocal(context.Background(), permission.Layer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	server := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"the-token": local.Client}})
	defer server.Close()
	c := caller{t: t, url: server.URL, token: "the-token", client: local.Client}

	// A request without the right token is refused and does nothing.
	if status, _, _ := (caller{t: t, url: server.URL, token: "the-toke"}).do("GET", "/runs", ""); status != http.StatusUnauthorized {
		t.Fatalf("a wrong token was answered %d", status)
	}
	// A field the gateway does not know is refused, and the answer names
	// it: in what a start carries, among the parts of its form, and in what
	// an event carries.
	if status, _, text := c.upload(part{name: "start", contents: `{"at": "/door", "labels": {"a": "b"}, "ceiling": ["all"]}`}); status != 400 || !strings.Contains(text, "ceiling") {
		t.Fatalf("an unknown field was answered %d %s", status, text)
	}
	if status, _, text := c.upload(part{name: "start", contents: `{"at": "/door"}`}, part{name: "ceiling", contents: "all"}); status != 400 || !strings.Contains(text, `a part named \"ceiling\"`) {
		t.Fatalf("an unknown part was answered %d %s", status, text)
	}
	// A start names the location it starts a run at.
	if status, _, text := c.upload(part{name: "start", contents: `{"input": {}}`}); status != 400 || !strings.Contains(text, "at") {
		t.Fatalf("a start with no location was answered %d %s", status, text)
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

	// A start where nothing is recorded is answered with its refusal, and
	// with ?wait as well.
	for _, query := range []string{"", "?wait"} {
		status, refused, text := c.uploadTo("/runs"+query, form(c, map[string]any{"at": "/nowhere"})...)
		if status != 200 || refused["taken"] != false || !strings.Contains(text, "nothing is recorded at /nowhere") || refused["id"] == nil {
			t.Fatalf("a start%s at /nowhere was answered %d %s", query, status, text)
		}
	}
	status, started, text := c.upload(form(c, map[string]any{"definition": door, "labels": map[string]string{"team": "ops"}, "input": nil})...)
	id, _ := started["id"].(string)
	if status != 200 || id == "" || started["taken"] != true {
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
	if status, text := c.listed("/runs?all&label=team=ops", id); status != 200 || !strings.Contains(text, id) {
		t.Fatalf("the listing was answered %d %s", status, text)
	}
	if status, _, text := c.do("GET", "/runs?all&label=team=ops&label=team=web", ""); status != 400 || !strings.Contains(text, `team`) {
		t.Fatalf("a listing narrowed to one label name twice was answered %d %s", status, text)
	}
	// A listing narrowed to a location, and to a key there, gives the runs
	// and their IDs.
	at, _ := run["location"].(string)
	if status, text := c.listed("/runs?all&at="+at, id); status != 200 || at == "" || !strings.Contains(text, id) {
		t.Fatalf("the listing at %q was answered %d %s", at, status, text)
	}
	for _, query := range []string{"?all&at=/elsewhere", "?all&at=" + at + "&key=alice"} {
		if status, _, text := c.do("GET", "/runs"+query, ""); status != 200 || strings.TrimSpace(text) != "[]" {
			t.Fatalf("the listing %s was answered %d %s", query, status, text)
		}
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
	server := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"the-token": local.Client}})
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
	none := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"": local.Client}})
	defer none.Close()
	for _, shown := range []string{"", "the-token"} {
		if status, _, text := (caller{t: t, url: none.URL, token: shown}).do("GET", "/runs", ""); status != http.StatusUnauthorized {
			t.Errorf("a gateway with no token answered %q with %d %s", shown, status, text)
		}
	}
}
