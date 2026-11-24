package gateway_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
)

// The test binary stands in for the process a runner starts for a job.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	os.Exit(m.Run())
}

// served is a gateway in front of every role in this process.
func served(t *testing.T) (*client.Local, caller) {
	t.Helper()
	local, err := client.NewLocal(context.Background(), permission.Layer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	local.Runner.Command = []string{os.Args[0], runner.JobArg}
	t.Cleanup(local.Close)
	return local, serve(t, local.Client)
}

// serve puts a gateway in front of a client.
func serve(t *testing.T, c *client.Client) caller {
	t.Helper()
	server := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"the-token": c}, Interval: 10 * time.Millisecond})
	t.Cleanup(server.Close)
	return caller{t: t, url: server.URL, token: "the-token", client: c}
}

// alone is a gateway whose controller is not running: what is sent to it
// is never taken up.
func alone(t *testing.T) (*client.Client, caller) {
	t.Helper()
	_, c, at := aloneOn(t)
	return c, at
}

// aloneOn is alone, with the store it is on.
func aloneOn(t *testing.T) (*store.Memory, *client.Client, caller) {
	t.Helper()
	mem := store.NewMemory()
	ctl, err := identity.New("controller", identity.Controller)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := identity.New("client", identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUp(context.Background(), mem.As(store.Party{Kind: store.Owner})); err != nil {
		t.Fatal(err)
	}
	c := client.New(cli, ctl.Card, mem.As(store.Party{Kind: store.Client, Name: "client", Controller: "controller"}))
	return mem, c, serve(t, c)
}

// A query parameter the route does not take is refused and named, on every
// route, and so is a status no run can have: a listing asked for with a
// mistake in it is not answered as though it had none.
func TestGatewayRefusesAParameterItDoesNotKnow(t *testing.T) {
	_, c := alone(t)
	const run = "/runs/client/20200101T000000.000Z-0000000000"
	for request, names := range map[string]string{
		"GET /runs?al":                    `\"al\"`,
		"GET /runs?all&watching":          `\"watching\"`,
		"GET /runs?status=nonsense":       "status is running, done, failed, cancelled",
		"POST /runs?dry":                  `\"dry\"`,
		"GET " + run + "?waits":           `\"waits\"`,
		"GET " + run + "/record?from=3":   `\"from\"`,
		"GET " + run + "/definition?wait": `\"wait\"`,
		"POST " + run + "/cancel?life=1m": `\"life\"`,
	} {
		method, path, _ := strings.Cut(request, " ")
		if status, _, text := c.do(method, path, ""); status != http.StatusBadRequest || !strings.Contains(text, names) {
			t.Errorf("%s was answered %d %s", request, status, text)
		}
	}
	// An entry of a start's deny list that is no permission is refused as
	// one of its allow list is.
	for _, list := range []string{"allow", "deny"} {
		if status, _, text := c.upload(form(c, map[string]any{"at": "/door", list: []string{"nonsense"}})...); status != http.StatusBadRequest || !strings.Contains(text, "is not a permission") {
			t.Errorf("a start whose %s list holds no permission was answered %d %s", list, status, text)
		}
	}
	// Every parameter a listing takes is taken.
	if status, _, text := c.do("GET", "/runs?all&status=done&parent=client/20200101T000000.000Z-0000000000&label=team=ops", ""); status != 200 || strings.TrimSpace(text) != "[]" {
		t.Errorf("a listing narrowed every way was answered %d %s", status, text)
	}
}

// start starts a run through the gateway and gives its ID.
func (c caller) start(request map[string]any) string {
	c.t.Helper()
	status, started, text := c.upload(form(c, request)...)
	id, _ := started["id"].(string)
	if status != 200 || id == "" {
		c.t.Fatalf("a start was answered %d %s", status, text)
	}
	return id
}

// events reads a stream to its end and gives each event's name and data.
func (c caller) events(path string) (names []string, data []map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		c.t.Fatalf("the stream %s was answered %d", path, resp.StatusCode)
	}
	lines := bufio.NewScanner(resp.Body)
	lines.Buffer(nil, 1<<20)
	for lines.Scan() {
		if name, ok := strings.CutPrefix(lines.Text(), "event: "); ok {
			names = append(names, name)
		}
		if text, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
			var v map[string]any
			if err := json.Unmarshal([]byte(text), &v); err != nil {
				c.t.Fatalf("the stream carried %q: %v", text, err)
			}
			data = append(data, v)
		}
	}
	return names, data
}

// blobs gives the path of every blob shown in a value.
func blobs(v any) []string {
	var paths []string
	switch v := v.(type) {
	case map[string]any:
		if path, ok := v["$blob"].(string); ok {
			return []string{path}
		}
		for _, inner := range v {
			paths = append(paths, blobs(inner)...)
		}
	case []any:
		for _, inner := range v {
			paths = append(paths, blobs(inner)...)
		}
	}
	return paths
}

const makes = `
flow: makes
stages:
  make: { job: { script: ./make.js } }
output: { report: "${stages.make.output.report}" }
`

// A path shown in an entry of a run's stream is fetched as a path in its
// result is.
func Test_client_gateway_blob_in_stream_entry_can_be_fetched(t *testing.T) {
	_, c := served(t)
	id := c.start(map[string]any{"definition": makes, "files": map[string]string{"./make.js": `
		emit("draft", { early: blobs.make("an early draft", { type: "text/markdown" }) })
		return { report: blobs.make("the report") }`}})
	names, entries := c.events("/runs/" + id + "/record?watch")
	if len(names) == 0 || names[len(names)-1] != "end" || entries[len(entries)-1]["status"] != "done" {
		t.Fatalf("the stream ended %v with %v", names, entries[len(entries)-1])
	}
	fetched := map[string]string{}
	for i, entry := range entries {
		for _, path := range blobs(entry) {
			from := id
			if names[i] == "entry" {
				from = entry["id"].(string)
			}
			req, _ := http.NewRequest("GET", c.url+"/runs/"+from+"/files/"+path, nil)
			req.Header.Set("Authorization", "Bearer "+c.token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			text, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Errorf("the path %q, shown in the stream's %s, was answered %d %s", path, names[i], resp.StatusCode, text)
				continue
			}
			fetched[string(text)] = resp.Header.Get("Content-Type")
			if names[i] == "entry" && !strings.Contains(path, "/") {
				t.Errorf("an entry shows the path %q, which says nothing of where in the record it is", path)
			}
		}
	}
	// The blob a job emitted is in no result: only its entry shows it.
	if fetched["an early draft"] != "text/markdown" || fetched["the report"] != "text/plain" {
		t.Fatalf("the paths of the stream fetched %v", fetched)
	}
	for _, path := range []string{"nothing", "steps/1/events[0].data.nothing", "emits/make/7/values[0].data.early"} {
		if status, _, text := c.do("GET", "/runs/"+id+"/files/"+path, ""); status != 404 {
			t.Errorf("the path %q, which names no file, was answered %d %s", path, status, text)
		}
	}
}

// A run whose whole output is one blob shows it with the path file, and the
// gateway gives its bytes at that path, to the client of the tree and to a
// client that called the run. The route with no path is the list.
func Test_client_gateway_whole_output_blob_is_fetched_by_its_path(t *testing.T) {
	local, owner := served(t)
	ctx := context.Background()
	at := owner.place("flow: whole\nreceive: [client:gw]\nstages:\n  make: { job: { script: ./make.js } }\n",
		map[string]string{"./make.js": `return blobs.make("all of it", { type: "text/markdown" })`})
	gw, err := identity.New("gw", identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.WriteTrust(ctx, local.Controller.Store, local.Controller.ID, identity.Trust{Card: gw.Card, Ceiling: permission.Layer{Allow: []string{"send:/**"}}}); err != nil {
		t.Fatal(err)
	}
	called := serve(t, client.New(gw, local.Controller.ID.Card, local.Memory.As(store.Party{Kind: store.Client, Name: "gw", Controller: local.Controller.ID.Name})))
	for who, c := range map[string]caller{"the tree's client": owner, "a client that calls": called} {
		status, started, text := c.send("POST", "/runs", "application/json", strings.NewReader(`{"at": "`+at+`"}`))
		id, _ := started["id"].(string)
		if status != 200 || id == "" {
			t.Fatalf("the start by %s was answered %d %s", who, status, text)
		}
		_, run, text := c.do("GET", "/runs/"+id+"?wait", "")
		if out, _ := run["output"].(map[string]any); run["status"] != "done" || out["$blob"] != "file" {
			t.Fatalf("the run of %s ended %s", who, text)
		}
		if status, kind, text := c.fetch(id, "file"); status != 200 || text != "all of it" || kind != "text/markdown" {
			t.Errorf("%s asked for the file and was answered %d %s %q", who, status, kind, text)
		}
		for _, route := range []string{"/files", "/files/"} {
			if status, files, text := c.do("GET", "/runs/"+id+route, ""); status != 200 || len(files) != 1 || files["file"] == nil {
				t.Errorf("%s asked for %s and was answered %d %s", who, route, status, text)
			}
		}
	}
}

// An input of null in a start is no input: the machine sees what it sees
// when nothing is sent.
func Test_client_gateway_null_input_is_no_input(t *testing.T) {
	_, c := served(t)
	const sees = `
statechart: sees
initial: over
states:
  over: { end: done, output: { given: "${input === undefined ? 'nothing' : JSON.stringify(input)}" } }
`
	at := c.place(sees, nil)
	var outputs, starts []string
	for _, parts := range [][]part{
		{{name: "start", contents: `{"at": "` + at + `"}`}},
		{{name: "start", contents: `{"at": "` + at + `", "input": null}`}},
		{{name: "start", contents: `{"input": null, "labels": null, "at": "` + at + `"}`}},
	} {
		status, started, text := c.upload(parts...)
		id, _ := started["id"].(string)
		if status != 200 || id == "" {
			t.Fatalf("a start was answered %d %s", status, text)
		}
		_, run, text := c.do("GET", "/runs/"+id+"?wait", "")
		if run["status"] != "done" {
			t.Fatalf("the run ended %s", text)
		}
		output, _ := json.Marshal(run["output"])
		outputs = append(outputs, string(output))
		// What arrived at the machine, as its first step shows it.
		_, entries := c.events("/runs/" + id + "/record?watch")
		start, _ := json.Marshal(entries[0]["events"].([]any)[0].(map[string]any)["data"])
		starts = append(starts, string(start))
	}
	if outputs[1] != outputs[0] || outputs[2] != outputs[0] || starts[1] != starts[0] || starts[2] != starts[0] {
		t.Fatalf("with no input and with an input of null, the machine gave %v and was started with %v", outputs, starts)
	}
	// An input that is something is passed on.
	id := c.start(map[string]any{"definition": sees, "input": map[string]any{"n": 1}})
	if _, run, text := c.do("GET", "/runs/"+id+"?wait", ""); !strings.Contains(text, `{\"n\":1}`) || run["status"] != "done" {
		t.Fatalf("with an input, the run ended %s", text)
	}
}

func quoted(text string) string {
	data, _ := json.Marshal(text)
	return string(data)
}

// Two gateways on one config answer alike for one run: each reads the
// store afresh.
func Test_client_gateway_two_copies_answer_alike(t *testing.T) {
	local, one := served(t)
	two := serve(t, client.New(local.ID, local.Client.Controller, local.Store))
	id := one.start(map[string]any{"definition": door, "labels": map[string]string{"team": "ops"}})
	alike := func(want string) {
		t.Helper()
		for _, path := range []string{"/runs/" + id, "/runs?all", "/runs/" + id + "/definition"} {
			_, _, first := one.do("GET", path, "")
			_, _, second := two.do("GET", path, "")
			if first != second || !strings.Contains(first, want) && !strings.HasSuffix(path, "/definition") {
				t.Fatalf("for %s one gateway answered\n  %s and the other\n  %s", path, first, second)
			}
		}
	}
	// Both have answered for the run while it runs; then it ends by an
	// event sent through one of them.
	if _, _, text := two.do("GET", "/runs/"+id+"?wait=50ms", ""); !strings.Contains(text, `"status":"running"`) {
		t.Fatalf("before the event the run is %s", text)
	}
	if _, text := one.listed("/runs", id); !strings.Contains(text, id) {
		t.Fatal("the run was never listed")
	}
	alike(id)
	if status, receipt, text := one.do("POST", "/runs/"+id+"/events", `{"event": "open", "data": {"who": "one"}}`); status != 200 || receipt["acted"] != true {
		t.Fatalf("the event was answered %d %s", status, text)
	}
	if _, run, text := two.do("GET", "/runs/"+id+"?wait", ""); run["status"] != "done" {
		t.Fatalf("the other gateway answered %s", text)
	}
	// The head follows the record by a moment: both list the run as ended.
	if _, text := one.listed("/runs?all&status=done", id); !strings.Contains(text, id) {
		t.Fatal("the run was never listed as done")
	}
	alike(id)
	// A stream from either carries the same entries and the same end.
	_, first := one.events("/runs/" + id + "/record?watch")
	_, second := two.events("/runs/" + id + "/record?watch")
	if a, b := quotedAny(first), quotedAny(second); a != b || len(first) < 3 {
		t.Fatalf("one gateway's stream was\n  %s and the other's\n  %s", a, b)
	}
}

func quotedAny(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// An ID that is no run is not found, by the answer's status and at once,
// on every route that names a run; a run started a moment ago is waiting,
// and never not found.
func Test_client_gateway_unknown_run_is_not_found(t *testing.T) {
	_, c := alone(t)
	started := time.Now()
	for _, id := range []string{
		"client/20200101T000000.000Z-0000000000",
		"client/" + store.Stamp(time.Now()) + "-0000000000",
		"client/" + store.Stamp(time.Now().Add(time.Hour)) + "-0000000000",
		"other/" + store.Stamp(time.Now()) + "-0000000000",
		"client/not-a-run",
	} {
		for route, method := range map[string]string{"": "GET", "?wait": "GET", "/record": "GET", "/record?watch": "GET", "/definition": "GET", "/files/report": "GET",
			"/files/steps/1/events[0].data": "GET", "/cancel": "POST", "/events": "POST", "/keys": "POST"} {
			if status, _, text := c.do(method, "/runs/"+id+route, `{}`); status != 404 || !strings.Contains(text, "no such run") {
				t.Errorf("%s /runs/%s%s was answered %d %s", method, id, route, status, text)
			}
		}
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("the answers for runs that do not exist took %v", took)
	}
	// With no controller running, a run just started is waiting.
	id := c.start(map[string]any{"at": "/door"})
	for _, route := range []string{"", "?wait=20ms"} {
		if status, run, text := c.do("GET", "/runs/"+id+route, ""); status != 200 || run["status"] != "waiting" || run["id"] != id {
			t.Errorf("GET /runs/%s%s for a run just started was answered %d %s", id, route, status, text)
		}
	}
	// Its record so far is empty, and is answered without staying.
	if status, _, text := c.do("GET", "/runs/"+id+"/record", ""); status != 200 || !strings.Contains(text, `"entries":[]`) || !strings.Contains(text, `"status":"waiting"`) {
		t.Errorf("GET /runs/%s/record for a run just started was answered %d %s", id, status, text)
	}
	for _, route := range []string{"/definition", "/files/report"} {
		if status, _, text := c.do("GET", "/runs/"+id+route, ""); status != 409 || !strings.Contains(text, "waiting") {
			t.Errorf("GET /runs/%s%s for a run just started was answered %d %s", id, route, status, text)
		}
	}
	// Its stream opens, and stays open for it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", c.url+"/runs/"+id+"/record?watch", nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("the stream of a run just started was answered %d", resp.StatusCode)
	}
}

// A wait that is cut short, by its own limit or by the caller going away,
// answers with the run as it stands, through the package and through the
// gateway alike.
func Test_client_wait_cut_short_answers_as_it_stands(t *testing.T) {
	// One run waiting to start, with no controller; one running, which
	// waits for an event.
	unsent, waiting := alone(t)
	local, running := served(t)
	for want, at := range map[string]struct {
		c *client.Client
		caller
	}{"waiting": {unsent, waiting}, "running": {local.Client, running}} {
		// With no controller, nothing takes a submit: the start names a
		// location that is not there, and is never taken up.
		request := map[string]any{"at": "/door"}
		if want == "running" {
			request = map[string]any{"definition": door}
		}
		id := at.start(request)
		if want == "running" {
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
				if run, err := at.c.Show(context.Background(), id); err == nil && run.Status == "running" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("the run was never taken up")
				}
			}
		}
		// Through the package: by the wait's own limit, and by a caller
		// that goes away.
		limited, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		gone, leave := context.WithCancel(context.Background())
		time.AfterFunc(30*time.Millisecond, leave)
		for how, ctx := range map[string]context.Context{"its own limit": limited, "its caller going away": gone} {
			run, err := at.c.Wait(ctx, id, 5*time.Millisecond)
			if err != nil || run == nil || run.Status != want || run.ID != id {
				t.Errorf("a wait on a %s run, cut short by %s, gave %+v, %v", want, how, run, err)
			}
			run, err = at.c.Follow(ctx, id, 5*time.Millisecond, func(client.Entry) {})
			if err != nil || run == nil || run.Status != want {
				t.Errorf("a follow of a %s run, cut short by %s, gave %+v, %v", want, how, run, err)
			}
		}
		cancel()
		// Through the gateway, by the limit the wait is given.
		began := time.Now()
		status, run, text := at.do("GET", "/runs/"+id+"?wait=30ms", "")
		if status != 200 || run["status"] != want || run["id"] != id || run["error"] != nil {
			t.Errorf("GET ?wait=30ms on a %s run was answered %d %s", want, status, text)
		}
		if took := time.Since(began); took < 30*time.Millisecond || took > 5*time.Second {
			t.Errorf("a wait limited to 30ms answered after %v", took)
		}
		if status, _, text := at.do("GET", "/runs/"+id+"?wait=soon", ""); status != 400 {
			t.Errorf("a wait with a limit that is no length of time was answered %d %s", status, text)
		}
	}
}

// A gateway whose client does not own a location's tree starts a run there
// as a call. The run is answered from the call's result: its status and its
// output, whose files are fetched as any run's are, and nothing of its
// record.
func Test_client_gateway_call_is_answered_from_its_result(t *testing.T) {
	local, owner := served(t)
	ctx := context.Background()
	gw, err := identity.New("gw", identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.WriteTrust(ctx, local.Controller.Store, local.Controller.ID, identity.Trust{Card: gw.Card, Ceiling: permission.Layer{Allow: []string{"send:/**"}}}); err != nil {
		t.Fatal(err)
	}
	c := serve(t, client.New(gw, local.Controller.ID.Card, local.Memory.As(store.Party{Kind: store.Client, Name: "gw", Controller: local.Controller.ID.Name})))
	at := owner.place(`
statechart: echo
receive: [client:gw]
initial: over
states:
  over: { end: done, output: "${input}" }
`, nil)
	status, started, text := c.upload(part{name: "start", contents: `{"at": "` + at + `", "input": {"n": 1}}`},
		part{name: "input:file", filename: "a.txt", contents: "called"})
	id, _ := started["id"].(string)
	if status != 200 || id == "" {
		t.Fatalf("the call was answered %d %s", status, text)
	}
	status, run, text := c.do("GET", "/runs/"+id+"?wait", "")
	if status != 200 || run["status"] != "done" || run["called"] == nil || run["output"].(map[string]any)["n"] != 1.0 {
		t.Fatalf("the call was answered %d %s", status, text)
	}
	if status, _, text := c.do("GET", "/runs/"+id+"/files/file", ""); status != 200 || text != "called" {
		t.Fatalf("the call's file was answered %d %s", status, text)
	}
	if status, record, text := c.do("GET", "/runs/"+id+"/record", ""); status != 200 || len(record["entries"].([]any)) != 0 {
		t.Fatalf("the record of a call was answered %d %s", status, text)
	}
	// Its definition and a cancel are more than its result, and refused as
	// that, whether the run is done or not.
	for route, method := range map[string]string{"/definition": "GET", "/cancel": "POST"} {
		if status, _, text := c.do(method, "/runs/"+id+route, "{}"); status != 403 || !strings.Contains(text, "a call into a tree of another client") {
			t.Errorf("%s /runs/<call>%s was answered %d %s", method, route, status, text)
		}
	}
}

// A start with no file is JSON: the request's body is what the part start
// of an upload form holds.
func TestStartIsJSONWhenItHasNoFile(t *testing.T) {
	_, c := served(t)
	at := c.place("statechart: sees\ninitial: over\nstates:\n  over: { end: done, output: \"${input.n}\" }\n", nil)
	status, started, text := c.send("POST", "/runs", "application/json", strings.NewReader(`{"at": "`+at+`", "input": {"n": 7}}`))
	id, _ := started["id"].(string)
	if status != 200 || id == "" {
		t.Fatalf("a start that is JSON was answered %d %s", status, text)
	}
	if _, run, text := c.do("GET", "/runs/"+id+"?wait", ""); run["status"] != "done" || run["output"] != float64(7) {
		t.Fatalf("the run ended %s", text)
	}
	if status, _, text := c.send("POST", "/runs", "application/json", strings.NewReader(`{"at": "`+at+`", "definition": "x"}`)); status != 400 || !strings.Contains(text, "definition") {
		t.Fatalf("a start that is JSON with a definition was answered %d %s", status, text)
	}
}

// failing is a store whose listings fail once it is told to.
type failing struct {
	store.Store
	fail chan struct{}
}

func (f failing) List(ctx context.Context, under string) ([]store.Listed, error) {
	select {
	case <-f.fail:
		return nil, io.ErrUnexpectedEOF
	default:
		return f.Store.List(ctx, under)
	}
}

// A stream the gateway stops because a look failed carries a last message
// named error that gives the failure.
func TestStreamSaysWhyItStopped(t *testing.T) {
	mem := store.NewMemory()
	ctl, _ := identity.New("controller", identity.Controller)
	cli, _ := identity.New("client", identity.Client)
	if err := store.SetUp(context.Background(), mem.As(store.Party{Kind: store.Owner})); err != nil {
		t.Fatal(err)
	}
	f := failing{mem.As(store.Party{Kind: store.Client, Name: "client", Controller: "controller"}), make(chan struct{})}
	c := serve(t, client.New(cli, ctl.Card, f))
	req, _ := http.NewRequest("GET", c.url+"/runs?watch", nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewReader(resp.Body)
	if first, err := lines.ReadString('\n'); err != nil || !strings.HasPrefix(first, "event: run") {
		t.Fatalf("the stream began %q, %v", first, err)
	}
	close(f.fail)
	rest, _ := io.ReadAll(lines)
	if !strings.Contains(string(rest), "event: error\ndata: {\"error\":") || !strings.Contains(string(rest), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("the stream ended %q", rest)
	}
}

// A start with ?wait is answered with the run when it has ended. A blob of
// its output is read by its path, as for any run, or with ?files has its
// bytes in the answer.
func Test_client_gateway_start_waits_and_gives_files(t *testing.T) {
	local, err := client.NewLocal(context.Background(), permission.Layer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	server := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"the-token": local.Client}})
	defer server.Close()
	c := caller{t: t, url: server.URL, token: "the-token", client: local.Client}
	at := c.place("flow: file\nstages:\n  make: { job: { script: ./make.js } }\noutput: ${stages.make.output}\n",
		map[string]string{"make.js": `return { n: 1, file: blobs.make("zebra", { type: "text/plain" }) }`})
	slow := c.place("statechart: slow\ninitial: waiting\nstates:\n  waiting: { ticket: word }\n", nil)
	start := func(query, at string) (int, map[string]any, string) {
		return c.send("POST", "/runs"+query, "application/json", strings.NewReader(`{"at": "`+at+`"}`))
	}

	status, run, text := start("?wait", at)
	output, _ := run["output"].(map[string]any)
	file, _ := output["file"].(map[string]any)
	if status != 200 || run["status"] != "done" || output["n"] != float64(1) || file["$blob"] == nil {
		t.Fatalf("a start with wait was answered %d %s", status, text)
	}
	id, _ := run["id"].(string)
	if status, _, text := c.do("GET", "/runs/"+id+"/files/"+file["$blob"].(string), ""); status != 200 || text != "zebra" {
		t.Errorf("the blob read by its path was answered %d %q", status, text)
	}
	// With files, each blob of the output has its bytes, and the rest of the output is as it was.
	b64 := func(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }
	status, run, text = start("?wait&files", at)
	output, _ = run["output"].(map[string]any)
	file, _ = output["file"].(map[string]any)
	if status != 200 || run["status"] != "done" || output["n"] != float64(1) || file["data"] != b64("zebra") || file["type"] != "text/plain" || file["size"] != float64(5) || file["$blob"] != "file" {
		t.Fatalf("a start with wait and files was answered %d %s", status, text)
	}
	if _, run, text := c.do("GET", "/runs/"+id+"?files", ""); !strings.Contains(text, b64("zebra")) || run["status"] != "done" {
		t.Errorf("GET with files was answered %s", text)
	}
	// An output that is itself a blob.
	whole := c.place("flow: whole\nstages:\n  make: { job: { script: ./whole.js } }\noutput: ${stages.make.output}\n",
		map[string]string{"whole.js": `return blobs.make("zebra")`})
	if _, run, text := start("?wait&files", whole); fmt.Sprint(run["output"].(map[string]any)["data"]) != b64("zebra") {
		t.Errorf("a run whose output is a blob was answered %s", text)
	}
	// A blob in a list, the same blob at two places, a blob written out
	// inside a text, and values that only look like blobs: one that names a
	// blob's path, one with data of its own, and one that holds a blob.
	mixed := c.place("flow: mixed\nstages:\n  make: { job: { script: ./mixed.js } }\noutput:\n  made: ${stages.make.output}\n  text: \"see ${stages.make.output.file} end\"\n",
		map[string]string{"mixed.js": `const b = blobs.make("ab")
			return { file: blobs.make("zebra"), list: [b, { again: b }],
				looks: { "$blob": "made.file" }, more: { "$blob": "made.file", type: "x", size: 1, data: "mine" }, holds: { "$blob": "x", inner: blobs.make("cd") } }`})
	status, run, text = start("?wait&files", mixed)
	output, _ = run["output"].(map[string]any)
	made, _ := output["made"].(map[string]any)
	at2 := func(v any, keys ...any) any {
		for _, k := range keys {
			switch k := k.(type) {
			case string:
				m, _ := v.(map[string]any)
				v = m[k]
			case int:
				l, _ := v.([]any)
				if k >= len(l) {
					return nil
				}
				v = l[k]
			}
		}
		return v
	}
	if status != 200 || run["status"] != "done" || at2(made, "file", "data") != b64("zebra") || at2(made, "list", 0, "data") != b64("ab") || at2(made, "list", 1, "again", "data") != b64("ab") ||
		at2(made, "looks", "data") != nil || at2(made, "looks", "$blob") != "made.file" || at2(made, "more", "data") != "mine" || at2(made, "holds", "inner", "data") != b64("cd") || at2(made, "holds", "$blob") != "x" ||
		!strings.Contains(fmt.Sprint(output["text"]), b64("zebra")) || !strings.HasPrefix(fmt.Sprint(output["text"]), "see ") {
		t.Errorf("a run with blobs and values like blobs was answered %d %s", status, text)
	}
	// The blobs are given up to a size together: 5 + 2 + 2 + 2 + 5 bytes here.
	// Past it the answer is an error that names the run and how each blob is read.
	mixedID, _ := run["id"].(string)
	small := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"the-token": local.Client}, MaxFiles: 15})
	defer small.Close()
	exact := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"the-token": local.Client}, MaxFiles: 16})
	defer exact.Close()
	status, run, text = caller{t: t, url: small.URL, token: "the-token"}.do("GET", "/runs/"+mixedID+"?files", "")
	if status != 413 || run["id"] != mixedID || !strings.Contains(text, "at most 15 bytes") || !strings.Contains(text, "/runs/"+mixedID+"/files/") || strings.Contains(text, b64("zebra")) {
		t.Errorf("blobs of 16 bytes, with at most 15 given, were answered %d %s", status, text)
	}
	if status, _, text := (caller{t: t, url: exact.URL, token: "the-token"}).do("GET", "/runs/"+mixedID+"?files", ""); status != 200 || !strings.Contains(text, b64("zebra")) {
		t.Errorf("blobs of 16 bytes, with at most 16 given, were answered %d %s", status, text)
	}
	// On a start past the size, the error names the run that was started.
	status, run, text = caller{t: t, url: small.URL, token: "the-token"}.send("POST", "/runs?wait&files", "application/json", strings.NewReader(`{"at": "`+mixed+`"}`))
	startedID, _ := run["id"].(string)
	if status != 413 || startedID == "" || startedID == mixedID || !strings.Contains(text, "/runs/"+startedID+"/files/") {
		t.Errorf("a start with files past the size was answered %d %s", status, text)
	}
	if status, run, text := c.do("GET", "/runs/"+startedID, ""); status != 200 || run["status"] != "done" {
		t.Errorf("the run a start past the size named was answered %d %s", status, text)
	}
	// A size no sum can hold is past the limit like any other: here a text
	// names one blob twice with a size of 2^62, and the real blob is not read.
	key := strings.Repeat("A", 43)
	huge := c.place("flow: huge\nstages:\n  make: { job: { script: ./huge.js } }\noutput: ${stages.make.output}\n",
		map[string]string{"huge.js": `const big = JSON.stringify({ "$blob": "controller/nope", type: "t", size: 4611686018427387904, key: "` + key + `" })
			return { a: blobs.make("zebra"), y: big, z: big }`})
	if status, run, text := (caller{t: t, url: exact.URL, token: "the-token"}).send("POST", "/runs?wait&files", "application/json", strings.NewReader(`{"at": "`+huge+`"}`)); status != 413 || run["id"] == nil || strings.Contains(text, b64("zebra")) {
		t.Errorf("an output that names blobs of 2^62 bytes twice was answered %d %s", status, text)
	}
	// A blob that cannot be read is an error that names the run and the
	// blob's path: here a text names a blob the store does not have.
	gone := c.place("flow: gone\nstages:\n  make: { job: { script: ./gone.js } }\noutput: ${stages.make.output}\n",
		map[string]string{"gone.js": `return { note: JSON.stringify({ "$blob": "controller/nope", type: "t", size: 3, key: "` + key + `" }), file: blobs.make("zebra") }`})
	status, run, text = start("?wait&files", gone)
	if status != 500 || run["id"] == nil || !strings.Contains(text, "note#1") || strings.Contains(text, b64("zebra")) {
		t.Errorf("a run with a blob that is not in the store was answered %d %s", status, text)
	}
	if status, run, text := c.do("GET", "/runs/"+run["id"].(string), ""); status != 200 || run["status"] != "done" {
		t.Errorf("the run with a blob that is not in the store was answered %d %s", status, text)
	}
	// Through another gateway, the limit and the error are that gateway's.
	far := client.Remote(small.URL, "the-token")
	if output, err := far.OutputWithBytes(context.Background(), mixedID, 1<<30); !errors.Is(err, client.ErrFilesTooLarge) || output != nil {
		t.Errorf("through a gateway that gives 15 bytes, the client was given %v, %v", output, err)
	}
	far = client.Remote(exact.URL, "the-token")
	if output, err := far.OutputWithBytes(context.Background(), mixedID, 1); err != nil || !strings.Contains(fmt.Sprint(output), b64("zebra")) {
		t.Errorf("through a gateway that gives 16 bytes, the client was given %v, %v", output, err)
	}
	// files needs wait on a start: alone it is answered 400, and starts no run.
	_, _, listedBefore := c.do("GET", "/runs?all", "")
	if status, _, text := start("?files", at); status != 400 || !strings.Contains(text, "wait") {
		t.Errorf("a start with files and no wait was answered %d %s", status, text)
	}
	if _, _, after := c.do("GET", "/runs?all", ""); strings.Count(after, `"id"`) != strings.Count(listedBefore, `"id"`) {
		t.Errorf("a start with files and no wait started a run")
	}
	// A wait's own limit answers with the run as it stands, with files too.
	if status, run, text := start("?wait=200ms&files", slow); status != 200 || run["status"] != "running" {
		t.Errorf("a start with a wait of 200ms and files was answered %d %s", status, text)
	}
	began := time.Now()
	if status, run, text := start("?wait=200ms", slow); status != 200 || run["status"] != "running" || time.Since(began) > 5*time.Second {
		t.Errorf("a start with a wait of 200ms was answered %d %s after %s", status, text, time.Since(began))
	}
	// A wait that cannot be read is refused, and nothing is started.
	_, _, listed := c.do("GET", "/runs?all", "")
	if status, _, text := start("?wait=abc", at); status != 400 || !strings.Contains(text, "wait") {
		t.Errorf("a start with a wait that is no length of time was answered %d %s", status, text)
	}
	if status, _, text := c.do("GET", "/runs/"+id+"?wait=abc", ""); status != 400 || !strings.Contains(text, "wait") {
		t.Errorf("a wait that is no length of time was answered %d %s", status, text)
	}
	time.Sleep(300 * time.Millisecond)
	if _, _, after := c.do("GET", "/runs?all", ""); strings.Count(after, `"id"`) != strings.Count(listed, `"id"`) {
		t.Errorf("a start that was refused started a run: the list went from %s to %s", listed, after)
	}
	// A wait of no time answers at once with the run as it stands.
	began = time.Now()
	status, run, text = start("?wait=0s", slow)
	if status != 200 || run["status"] == "done" || time.Since(began) > 5*time.Second {
		t.Errorf("a start with a wait of 0s was answered %d %s after %s", status, text, time.Since(began))
	}
	began = time.Now()
	if status, run, text := c.do("GET", "/runs/"+run["id"].(string)+"?wait=0s", ""); status != 200 || run["status"] == "done" || time.Since(began) > 5*time.Second {
		t.Errorf("a wait of 0s was answered %d %s after %s", status, text, time.Since(began))
	}
	// With no wait, a start is answered with the run's ID alone.
	if status, run, text := start("", at); status != 200 || run["id"] == nil || run["status"] != nil {
		t.Errorf("a start was answered %d %s", status, text)
	}
	if status, _, text := start("?watch", at); status != 400 || !strings.Contains(text, "watch") {
		t.Errorf("a start with a parameter it does not take was answered %d %s", status, text)
	}
}
