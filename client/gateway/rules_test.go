package gateway_test

import (
	"bufio"
	"context"
	"encoding/json"
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

	_ "github.com/zummed/corbie/flow"
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
	server := httptest.NewServer(&gateway.Gateway{Client: c, Token: "the-token", Interval: 10 * time.Millisecond})
	t.Cleanup(server.Close)
	return caller{t: t, url: server.URL, token: "the-token"}
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
		if status, _, text := c.upload(form(c, map[string]any{"definition": door, list: []string{"nonsense"}})...); status != http.StatusBadRequest || !strings.Contains(text, "is not a permission") {
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

// A definition sent to the gateway may name only the files sent with it,
// and a path that is a file and a directory both is refused.
func Test_client_gateway_definition_cannot_read_gateway_disk(t *testing.T) {
	c0, c := alone(t)
	// A file on the gateway's own disk, beside where it runs and by its
	// whole path.
	dir := t.TempDir()
	secret := dir + "/secret.js"
	if err := os.WriteFile(secret, []byte(`return "the gateway's own"`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	sent := func(script string, files map[string]string) (int, string) {
		status, _, text := c.upload(form(c, map[string]any{"files": files,
			"definition": "flow: reads\nstages:\n  it: { job: { script: " + script + " } }\n"})...)
		return status, text
	}
	for _, script := range []string{"./secret.js", "../" + dir[strings.LastIndexByte(dir, '/')+1:] + "/secret.js", secret, "./sub/../secret.js"} {
		if status, text := sent(script, map[string]string{"./other.js": "return 1"}); status != 400 || !strings.Contains(text, "secret.js") {
			t.Errorf("a definition naming %s, which was not sent with it, was answered %d %s", script, status, text)
		}
	}
	// A file and a directory of one name, and one file twice.
	for _, files := range []map[string]string{
		{"./a": "return 1", "./a/b.js": "return 2"},
		{"./a": "return 1", "./a/b/c.js": "return 2"},
		{"./a": "return 1", "a": "return 2"},
	} {
		if status, text := sent("./a", files); status != 400 || !strings.Contains(text, "./a") {
			t.Errorf("an upload holding %v was answered %d %s", files, status, text)
		}
	}
	if status, text := sent("./a", map[string]string{"./a": "return 1", "./b/c.js": "return 2"}); status != 200 {
		t.Fatalf("a definition naming a file sent with it was answered %d %s", status, text)
	}
	// Nothing was started by any of the refused.
	if names, _ := c0.Store.List(context.Background(), store.Records("controller")+"/client"); len(names) != 2 {
		t.Fatalf("the refused requests left %d values in the store: %v", len(names), names)
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
	var outputs, starts []string
	for _, parts := range [][]part{
		{{name: "definition", contents: sees}},
		{{name: "definition", contents: sees}, {name: "start", contents: `{"input": null}`}},
		{{name: "start", contents: `{"input": null, "labels": null}`}, {name: "definition", contents: sees}},
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
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, _, text := one.do("GET", "/runs", ""); strings.Contains(text, id) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run was never listed")
		}
	}
	alike(id)
	if status, receipt, text := one.do("POST", "/runs/"+id+"/events", `{"event": "open", "data": {"who": "one"}}`); status != 200 || receipt["acted"] != true {
		t.Fatalf("the event was answered %d %s", status, text)
	}
	if _, run, text := two.do("GET", "/runs/"+id+"?wait", ""); run["status"] != "done" {
		t.Fatalf("the other gateway answered %s", text)
	}
	// The head follows the record by a moment: both list the run as ended.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, _, text := one.do("GET", "/runs?all&status=done", ""); strings.Contains(text, id) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run was never listed as done")
		}
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
	id := c.start(map[string]any{"definition": door})
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
		id := at.start(map[string]any{"definition": door})
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
