package gateway_test

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// several is every role in this process, with a way to enrol a client under
// a ceiling and to put a gateway in front of the clients its tokens name.
type several struct {
	t     *testing.T
	local *client.Local
}

func newSeveral(t *testing.T) several {
	t.Helper()
	local, err := client.NewLocal(context.Background(), permission.Layer{Allow: []string{"all"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	local.Runner.Command = []string{os.Args[0], runner.JobArg}
	t.Cleanup(local.Close)
	return several{t: t, local: local}
}

func (s several) enrol(name string, allow ...string) *client.Client {
	s.t.Helper()
	id, err := identity.New(name, identity.Client)
	if err != nil {
		s.t.Fatal(err)
	}
	ctl := s.local.Controller
	if err := identity.WriteTrust(context.Background(), ctl.Store, ctl.ID, identity.Trust{Card: id.Card, Ceiling: permission.Layer{Allow: allow}}); err != nil {
		s.t.Fatal(err)
	}
	return client.New(id, ctl.ID.Card, s.local.Memory.As(store.Party{Kind: store.Client, Name: name, Controller: ctl.ID.Name}))
}

// serve gives a caller for each token of a gateway. submits is the client
// that submits the trees the callers start runs in.
func (s several) serve(submits *client.Client, callers map[string]*client.Client) func(token string) caller {
	s.t.Helper()
	server := httptest.NewServer(&gateway.Gateway{Callers: callers, Interval: 10 * time.Millisecond})
	s.t.Cleanup(server.Close)
	return func(token string) caller { return caller{t: s.t, url: server.URL, token: token, client: submits} }
}

// A token names one of the gateway's clients, and a request is sent as that
// client: a run started with a token is its client's, another token of the
// same client reads and cancels it, and a token of another client does
// neither. The client's ceiling limits the run, whatever the request says.
func Test_client_gateway_token_names_its_client(t *testing.T) {
	s := newSeveral(t)
	editor, viewer := s.enrol("editor", "at:/**", "network"), s.enrol("viewer")
	as := s.serve(editor, map[string]*client.Client{"first": editor, "second": editor.As(""), "views": viewer})
	first, second, views := as("first"), as("second"), as("views")

	at := first.place(door, nil)
	id := first.start(map[string]any{"at": at})
	if !strings.HasPrefix(id, "editor/") {
		t.Fatalf("the run started with the editor's token is %s", id)
	}
	for range 200 {
		if _, run, _ := first.do("GET", "/runs/"+id, ""); run["status"] == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status, run, text := first.do("GET", "/runs/"+id, ""); status != 200 || run["status"] != "running" {
		t.Fatalf("the editor's token was shown %d %s", status, text)
	}
	// Another token of the same client.
	if status, text := second.listed("/runs", id); status != 200 || !strings.Contains(text, id) {
		t.Fatalf("the editor's other token lists %d %s", status, text)
	}
	// A token of another client: the run is not one of its own. It is shown
	// what the head says and no more, and its cancel does nothing.
	if status, run, text := views.do("GET", "/runs/"+id, ""); status != 200 || run["status"] != "running" || run["input"] != nil {
		t.Fatalf("the viewer's token was shown %d %s", status, text)
	}
	if status, _, text := views.do("GET", "/runs/"+id+"/record", ""); status != 404 || !strings.Contains(text, "in a tree of the client editor") {
		t.Fatalf("the viewer's token was shown the record %d %s", status, text)
	}
	if status, receipt, text := views.do("POST", "/runs/"+id+"/cancel", "{}"); status != 200 || receipt["taken"] != false || !strings.Contains(text, "only that client cancels it") {
		t.Fatalf("the viewer's cancel was answered %d %s", status, text)
	}
	if status, _, text := views.do("GET", "/runs", ""); status != 200 || strings.Contains(text, id) {
		t.Fatalf("the viewer's token lists %d %s", status, text)
	}
	// Its ceiling has no send:, so its start in the editor's tree is denied.
	called := views.start(map[string]any{"at": at})
	if _, run, text := views.do("GET", "/runs/"+called+"?wait", ""); !strings.HasPrefix(called, "viewer/") || run["failure"] != "denied" || run["layer"] != `the ceiling of the client "viewer"` {
		t.Fatalf("the viewer's start in the editor's tree ended %s", text)
	}
	if status, run, text := first.do("GET", "/runs/"+id, ""); status != 200 || run["status"] != "running" {
		t.Fatalf("after the viewer's requests the run is %d %s", status, text)
	}
	if status, receipt, text := second.do("POST", "/runs/"+id+"/cancel", "{}"); status != 200 || receipt["taken"] != true {
		t.Fatalf("the editor's other token cancelled %d %s", status, text)
	}
	if _, run, text := first.do("GET", "/runs/"+id+"?wait", ""); run["status"] != "cancelled" {
		t.Fatalf("the cancelled run is %s", text)
	}

	// The editor's ceiling is network. A definition that allows disk is
	// denied when its job writes a file.
	path := t.TempDir() + "/a.txt"
	written := first.start(map[string]any{"definition": `
flow: writes
input: { path: any }
stages:
  write:
    job:
      script: ./write.js
      input: { path: "${input.path}" }
      allow: [disk]
      timeout: 30s
`, "files": map[string]string{"./write.js": `fs.write(input.path, { text: "x" })`}, "input": map[string]any{"path": path}})
	if _, run, text := first.do("GET", "/runs/"+written+"?wait", ""); run["status"] != "failed" || run["failure"] != "denied" || !strings.Contains(text, "ceiling") {
		t.Fatalf("the run that wrote a file ended %s", text)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the job wrote the file its client's ceiling does not allow")
	}
}

// The gateway writes the principal a token names into every message it sends
// for the token's requests: the start and each event name their own principal
// in the record, and a listing names none. A request names no principal.
func Test_client_gateway_writes_the_tokens_principal(t *testing.T) {
	s := newSeveral(t)
	editor := s.enrol("editor", "at:/**")
	as := s.serve(editor, map[string]*client.Client{
		"alices": editor.As("alice@example.com"), "bobs": editor.As("bob@example.com"), "nobodys": editor})
	alice, bob, nobody := as("alices"), as("bobs"), as("nobodys")

	at := alice.place(`
statechart: door
initial: closed
states:
  closed:
    on: { knock: closed, open: opened }
  opened: { end: done }
`, nil)
	id := alice.start(map[string]any{"at": at})
	for _, sent := range []struct {
		by    caller
		event string
	}{{bob, "knock"}, {nobody, "knock"}, {alice, "open"}} {
		if status, receipt, text := sent.by.do("POST", "/runs/"+id+"/events", `{"event": "`+sent.event+`"}`); status != 200 || receipt["taken"] != true {
			t.Fatalf("%s was answered %d %s", sent.event, status, text)
		}
	}
	status, record, text := nobody.do("GET", "/runs/"+id+"/record", "")
	if status != 200 {
		t.Fatalf("the record was answered %d %s", status, text)
	}
	// Each event of the record, with the principal of its cause.
	principals := map[string][]string{}
	for _, entry := range record["entries"].([]any) {
		events, _ := entry.(map[string]any)["events"].([]any)
		for _, e := range events {
			event := e.(map[string]any)
			cause, _ := event["cause"].(map[string]any)
			if cause["client"] != "editor" || cause["message"] == nil {
				t.Errorf("the event %v names the cause %v", event["name"], cause)
			}
			principal, _ := cause["principal"].(string)
			principals[event["name"].(string)] = append(principals[event["name"].(string)], principal)
		}
	}
	knocks := principals["knock"]
	if len(knocks) != 2 || knocks[0] != "bob@example.com" || knocks[1] != "" {
		t.Errorf("the knocks name the principals %q", knocks)
	}
	if got := principals["open"]; len(got) != 1 || got[0] != "alice@example.com" {
		t.Errorf("open names the principals %q", got)
	}
	// The run's cause is its start's, and a listing, which reads heads,
	// names no principal.
	run, _ := record["run"].(map[string]any)
	if cause, _ := run["cause"].(map[string]any); cause["principal"] != "alice@example.com" || cause["client"] != "editor" {
		t.Errorf("the run's cause is %v", run["cause"])
	}
	if status, listed := bob.listed("/runs?all", id); status != 200 || !strings.Contains(listed, id) || strings.Contains(listed, "example.com") {
		t.Errorf("the listing is %d %s", status, listed)
	}
	// A caller asks which client and principal its token names.
	for token, want := range map[caller]string{alice: `{"client":"editor","principal":"alice@example.com"}`, nobody: `{"client":"editor"}`} {
		if status, _, text := token.do("GET", "/client", ""); status != 200 || strings.TrimSpace(text) != want {
			t.Errorf("GET /client was answered %d %s", status, text)
		}
	}
	// A request names no principal.
	for route, body := range map[string]string{"/runs": `{"at": "` + at + `", "principal": "mallory@example.com"}`,
		"/runs/" + id + "/events": `{"event": "knock", "principal": "mallory@example.com"}`} {
		if status, _, text := bob.send("POST", route, "application/json", strings.NewReader(body)); status != 400 || !strings.Contains(text, "principal") {
			t.Errorf("a request to %s that names a principal was answered %d %s", route, status, text)
		}
	}
}
