package gateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
)

// tree sends a submit: an upload form whose part submit is JSON, with a
// part for each file.
func (c caller) tree(submit string, files map[string]string, extra ...part) (int, map[string]any, string) {
	c.t.Helper()
	parts := []part{{name: "submit", contents: submit}}
	for path, text := range files {
		parts = append(parts, part{name: "file:" + path, filename: path, contents: text})
	}
	return c.uploadTo("/trees", append(parts, extra...)...)
}

// json sends a request whose body is JSON.
func (c caller) json(method, path, body string) (int, map[string]any, string) {
	c.t.Helper()
	return c.send(method, path, "application/json", strings.NewReader(body))
}

// house is a directory of machines: a singleton, a flow with a script, a
// file that is no machine, and a listed file the flow includes.
var house = map[string]string{
	"devices/boiler.yaml": "statechart: boiler\nlifetime: singleton\ninitial: idle\nstates:\n  idle:\n    on: { stop: off }\n  off: { end: done }\n",
	"greet.yaml":          "include: shared/base.yaml\nflow: greet\nstages:\n  say: { job: { script: ./greet.js, input: { name: \"${input.name}\" } } }\noutput: { text: \"${stages.say.output}\", from: \"${run.context}\" }\n",
	"greet.js":            `return "hello " + input.name`,
	"bad.yaml":            "flow: bad\nstages: 3\n",
	".corbieignore":       "shared\n",
	"shared/base.yaml":    "input: { name: any }\n",
}

// A submit sent to the gateway records a tree as corbie run submit records a
// directory: each file at its location, a singleton started, a listed file
// with no location, and a file with a problem named with what is wrong. The
// directory the gateway read them from is gone when it answers.
func Test_client_gateway_submits_a_tree(t *testing.T) {
	// The directory for temporary files is a link, as it is on some systems.
	temp := filepath.Join(t.TempDir(), "tmp")
	if err := os.Symlink(t.TempDir(), temp); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", temp)
	s := newSeveral(t)
	editor, viewer := s.enrol("editor", "all", "at:/**"), s.enrol("viewer")
	as := s.serve(editor, map[string]*client.Client{"edits": editor.As("alice@example.com"), "views": viewer})
	edits, views := as("edits"), as("views")

	status, answer, text := edits.tree(`{"at": "/house", "directory": "/srv/house"}`, house)
	if status != 200 || answer["taken"] != true {
		t.Fatalf("the submit was answered %d %s", status, text)
	}
	what := map[string]string{}
	single := ""
	for _, row := range answer["tree"].(map[string]any)["locations"].([]any) {
		r := row.(map[string]any)
		what[r["location"].(string)] = r["what"].(string)
		if run, _ := r["run"].(string); run != "" {
			single = run
		}
	}
	if len(what) != 3 || what["/house/devices/boiler"] != "started" || what["/house/greet"] != "recorded" || what["/house/bad"] != "problem" || !strings.HasPrefix(single, "editor/") {
		t.Fatalf("the submit recorded %v and started %q: %s", what, single, text)
	}
	if problems, _ := answer["problems"].(map[string]any); len(problems) != 1 || !strings.Contains(problems["/house/bad"].(string), "stages") || strings.Contains(text, "corbie-tree-") {
		t.Fatalf("the submit's problems are %v", answer["problems"])
	}
	// The singleton's start names the principal the token was given to.
	var started map[string]any
	for range 500 {
		if _, started, _ = edits.do("GET", "/runs/"+single, ""); started["status"] == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cause, _ := started["cause"].(map[string]any); cause["principal"] != "alice@example.com" {
		t.Fatalf("the singleton is %v", started)
	}
	// A run of the flow reads its script and its included file, and the
	// directory the submit named.
	id := edits.start(map[string]any{"at": "/house/greet", "input": map[string]any{"name": "x"}})
	if _, run, shown := edits.do("GET", "/runs/"+id+"?wait", ""); run["status"] != "done" || quotedAny(run["output"]) != `{"from":"/srv/house","text":"hello x"}` {
		t.Fatalf("the run of the submitted flow ended %s", shown)
	}
	// A tree of one file.
	if status, answer, text := edits.tree(`{"at": "/one", "file": "greet.yaml"}`, house); status != 200 || answer["taken"] != true || !strings.Contains(text, `"location":"/one"`) {
		t.Fatalf("the submit of one file was answered %d %s", status, text)
	}
	// A file the directory lacks is named from the directory.
	if _, _, text := edits.tree(`{"at": "/lost"}`, map[string]string{"lost.yaml": "flow: lost\nstages:\n  say: { job: { script: ./absent.js } }\n"}); !strings.Contains(text, "absent.js") || strings.Contains(text, "corbie-tree-") {
		t.Errorf("the submit of a machine whose script is absent was answered %s", text)
	}
	// A client whose ceiling has no at: is refused by the controller.
	if status, answer, text := views.tree(`{"at": "/theirs"}`, house); status != 200 || answer["taken"] != false || !strings.Contains(text, "at:/theirs") {
		t.Fatalf("the viewer's submit was answered %d %s", status, text)
	}
	// What is no submit is refused, naming what is wrong, and sends nothing.
	// A file of the gateway's computer is none of the directory's.
	outside := filepath.Join(t.TempDir(), "out.js")
	if err := os.WriteFile(outside, []byte("return 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for want, send := range map[string]func() (int, map[string]any, string){
		"names the prefix": func() (int, map[string]any, string) { return edits.tree(`{"directory": "/srv/house"}`, house) },
		"definition": func() (int, map[string]any, string) {
			return edits.tree(`{"at": "/other"}`, house, part{name: "definition", contents: "flow: x"})
		},
		"not a path inside the directory": func() (int, map[string]any, string) {
			return edits.tree(`{"at": "/other"}`, map[string]string{"../out.yaml": "flow: x\n"})
		},
		`../out.js\" is not a path inside the directory`: func() (int, map[string]any, string) {
			return edits.tree(`{"at": "/other", "file": "../out.js"}`, house)
		},
		outside + `\" is not a path inside the directory`: func() (int, map[string]any, string) {
			return edits.tree(`{"at": "/other", "file": "`+outside+`"}`, house)
		},
		`../absent.yaml\" is not a path inside the directory`: func() (int, map[string]any, string) {
			return edits.tree(`{"at": "/other", "file": "../absent.yaml"}`, house)
		},
		"no part named submit": func() (int, map[string]any, string) {
			return edits.uploadTo("/trees", part{name: "file:a.yaml", filename: "a.yaml", contents: "flow: a\n"})
		},
		"secrets": func() (int, map[string]any, string) { return edits.tree(`{"at": "/other", "secrets": {}}`, house) },
	} {
		if status, _, text := send(); status != 400 || !strings.Contains(text, want) {
			t.Errorf("a submit that should be refused for %s was answered %d %s", want, status, text)
		}
	}
	if _, _, listed := edits.do("GET", "/runs?all&at=/other", ""); strings.TrimSpace(listed) != "[]" {
		t.Errorf("a refused submit left the runs %s", listed)
	}
	// A removal sent to the gateway removes the tree as corbie run remove
	// does, with what is set in it, and only for the tree's client.
	if status, _, text := edits.json("POST", "/secrets", `{"act": "set", "at": "/house/greet", "name": "K", "value": "v"}`); status != 200 {
		t.Fatalf("the set was answered %d %s", status, text)
	}
	if status, answer, text := views.json("POST", "/trees/remove", `{"at": "/house"}`); status != 200 || answer["taken"] != false {
		t.Errorf("the viewer's removal was answered %d %s", status, text)
	}
	if status, answer, text := edits.json("POST", "/trees/remove", `{"at": "/house"}`); status != 200 || answer["taken"] != true ||
		!strings.Contains(text, `"removed":["secret K at /house/greet"]`) || !strings.Contains(text, `"location":"/house/devices/boiler"`) {
		t.Errorf("the removal was answered %d %s", status, text)
	}
	// Nothing of a submit's files remains on the gateway's computer.
	if left, err := os.ReadDir(temp); err != nil || len(left) != 0 {
		t.Errorf("the gateway left %v in its temporary directory (%v)", left, err)
	}
}

// A client sets, lists and removes its secrets and values through the
// gateway. A list gives a value and never a secret, and a client that may
// submit nowhere sets nothing.
func Test_client_gateway_sets_secrets_and_values(t *testing.T) {
	s := newSeveral(t)
	editor, viewer := s.enrol("editor", "all", "at:/**"), s.enrol("viewer")
	as := s.serve(editor, map[string]*client.Client{"edits": editor, "views": viewer})
	edits, views := as("edits"), as("views")

	for _, of := range []string{"/secrets", "/values"} {
		if status, receipt, text := edits.json("POST", of, `{"act": "set", "at": "/house", "name": "COLOUR", "value": "blue-9731"}`); status != 200 || receipt["taken"] != true || strings.Contains(text, "blue-9731") {
			t.Fatalf("a set at %s was answered %d %s", of, status, text)
		}
		status, receipt, text := edits.json("POST", of, `{"act": "list"}`)
		listed, _ := receipt["listed"].([]any)
		if status != 200 || len(listed) != 1 {
			t.Fatalf("a list at %s was answered %d %s", of, status, text)
		}
		row := listed[0].(map[string]any)
		if row["name"] != "COLOUR" || row["at"] != "/house" || (of == "/values") != (row["value"] == "blue-9731") || (of == "/secrets" && strings.Contains(text, "blue-9731")) {
			t.Errorf("the list at %s is %s", of, text)
		}
		if status, receipt, text := views.json("POST", of, `{"act": "set", "at": "/house", "name": "COLOUR", "value": "red"}`); status != 200 || receipt["taken"] != false || receipt["refused"] == nil {
			t.Errorf("the viewer's set at %s was answered %d %s", of, status, text)
		}
		for body, want := range map[string]string{`{"act": "keep"}`: "act is set, remove or list", `{"act": "list", "secret": "x"}`: "secret", ``: "act is set, remove or list"} {
			if status, _, text := edits.json("POST", of, body); status != 400 || !strings.Contains(text, want) {
				t.Errorf("%s given %s was answered %d %s", of, body, status, text)
			}
		}
		if status, receipt, text := edits.json("POST", of, `{"act": "remove", "at": "/house", "name": "COLOUR"}`); status != 200 || receipt["taken"] != true {
			t.Fatalf("a remove at %s was answered %d %s", of, status, text)
		}
		if _, receipt, text := edits.json("POST", of, `{"act": "list"}`); receipt["taken"] != true || receipt["listed"] != nil {
			t.Errorf("after the remove the list at %s is %s", of, text)
		}
	}
	// The value a run reads is the one set through the gateway.
	at := edits.place("flow: reads\nstages:\n  say: { job: { script: ./say.js } }\noutput: \"${values.COLOUR}\"\n", map[string]string{"./say.js": "return 1"})
	if _, receipt, text := edits.json("POST", "/values", `{"act": "set", "at": "`+at+`", "name": "COLOUR", "value": "green"}`); receipt["taken"] != true {
		t.Fatalf("a set at the flow's location was answered %s", text)
	}
	id := edits.start(map[string]any{"at": at})
	if _, run, shown := edits.do("GET", "/runs/"+id+"?wait", ""); run["output"] != "green" {
		t.Errorf("the run that reads the value ended %s", shown)
	}
}

// An event is sent to a location, or to a key at one, and the receipt names
// the run that took it. A run is sent an event by its own route.
func Test_client_gateway_sends_to_an_address(t *testing.T) {
	_, c := served(t)
	at := c.place("statechart: talk\nlifetime: keyed\ninitial: open\nstates:\n  open:\n    on: { said: open, bye: over }\n  over: { end: done }\n", nil)
	status, receipt, text := c.json("POST", "/events", `{"to": "`+at+`#alice", "event": "said", "data": {"text": "hi"}}`)
	run, _ := receipt["run"].(string)
	if status != 200 || receipt["taken"] != true || run == "" {
		t.Fatalf("an event to a key was answered %d %s", status, text)
	}
	if _, listed := c.listed("/runs?at="+at+"&key=alice", run); !strings.Contains(listed, run) {
		t.Errorf("the runs with the key are %s", listed)
	}
	if status, again, text := c.json("POST", "/events", `{"to": "`+at+`#alice", "event": "bye"}`); status != 200 || again["run"] != run || again["acted"] != true {
		t.Errorf("a second event to the key was answered %d %s", status, text)
	}
	for body, want := range map[string]string{
		`{"to": "` + run + `", "event": "said"}`:    "/runs/<id>/events",
		`{"event": "said"}`:                         "to is a location",
		`{"to": "` + at + `", "event": "", "x": 1}`: "unknown object member",
	} {
		if status, _, text := c.json("POST", "/events", body); status != 400 || !strings.Contains(text, want) {
			t.Errorf("an event given %s was answered %d %s", body, status, text)
		}
	}
	// A key that cannot be one is the controller's to refuse.
	if status, receipt, text := c.json("POST", "/events", `{"to": "`+at+`#", "event": "said"}`); status != 200 || receipt["taken"] != false || !strings.Contains(text, "a key is not empty") {
		t.Errorf("an event to an empty key was answered %d %s", status, text)
	}
	if status, _, text := c.do("GET", "/events", ""); status != 404 {
		t.Errorf("GET /events was answered %d %s", status, text)
	}
}

// A run's tickets, one step of its record and the files of its result each
// have a route, and give what the package gives.
func Test_client_gateway_shows_tickets_steps_and_files(t *testing.T) {
	_, c := served(t)
	id := c.start(map[string]any{"definition": `
statechart: door
initial: making
states:
  making:
    job: { script: ./make.js }
    on: { done: closed }
  closed:
    ticket: knock
    on: { knock: opened }
  opened: { end: done, output: { report: "${states.making.output.report}", said: "${event.data}" } }
`, "files": map[string]string{"./make.js": `return { report: blobs.make("the report") }`}})
	var tickets map[string]any
	for range 500 {
		if _, tickets, _ = c.do("GET", "/runs/"+id+"/tickets", ""); tickets["knock"] != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ticket, _ := tickets["knock"].(string)
	if ticket == "" {
		t.Fatalf("the run's tickets are %v", tickets)
	}
	if status, receipt, text := c.json("POST", "/runs/"+id+"/tickets", `{"ticket": "`+ticket+`", "value": "come in"}`); status != 200 || receipt["taken"] != true {
		t.Fatalf("the answer was given %d %s", status, text)
	}
	_, run, shown := c.do("GET", "/runs/"+id+"?wait", "")
	if run["status"] != "done" {
		t.Fatalf("the run ended %s", shown)
	}
	// A step is the entry the record gives under that number.
	_, record, _ := c.do("GET", "/runs/"+id+"/record", "")
	var first map[string]any
	for _, e := range record["entries"].([]any) {
		if entry := e.(map[string]any); entry["step"] == 1.0 && entry["id"] == id {
			first = entry
		}
	}
	status, step, text := c.do("GET", "/runs/"+id+"/steps/1", "")
	want, _ := json.Marshal(first)
	if got, _ := json.Marshal(step); status != 200 || first == nil || string(got) != string(want) {
		t.Errorf("step 1 was answered %d %s, and the record has %s", status, text, want)
	}
	// A step read by its number has the outputs so far as well.
	_, step, text = c.do("GET", "/runs/"+id+"/steps/2", "")
	state, _ := step["state"].(map[string]any)
	outputs, _ := state["outputs"].(map[string]any)
	made, _ := outputs["making"].(map[string]any)
	if report, _ := made["report"].(map[string]any); report["$blob"] == nil {
		t.Errorf("step 2 was answered %s", text)
	}
	for _, none := range []string{"/steps/99", "/steps/0", "/steps/one", "/steps"} {
		if status, _, text := c.do("GET", "/runs/"+id+none, ""); status != 404 {
			t.Errorf("%s was answered %d %s", none, status, text)
		}
	}
	// The files of the result, each by the path it is fetched at.
	status, files, text := c.do("GET", "/runs/"+id+"/files", "")
	file, _ := files["report"].(map[string]any)
	if status != 200 || len(files) != 1 || file["size"] != 10.0 || file["type"] == nil {
		t.Fatalf("the files were answered %d %s", status, text)
	}
	if status, _, text := c.do("GET", "/runs/"+id+"/files/report", ""); status != 200 || text != "the report" {
		t.Errorf("the file was answered %d %s", status, text)
	}
	// A ticket's value is for those who read the record.
	if status, _, text := c.do("GET", "/runs/"+id+"/tickets", ""); status != 200 || strings.TrimSpace(text) != "{}" {
		t.Errorf("the tickets of a run that has ended were answered %d %s", status, text)
	}
}
