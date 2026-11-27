package toolkit

import (
	"encoding/json/v2"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
)

const notes = `toolkit: notes
description: Notes kept on shelves.
tools:
  add_note:
    description: Add a note.
    args: { text: string }
    job: { script: ./add.js, input: { text: "${args.text}" } }
`

const server = `toolkit: server
description: A server of tools.
job: { script: ./add.js }
`

func parse(src string) (*machine.Definition, error) {
	draft, err := machine.Parse([]byte(src), machine.Given(map[string]string{"add.js": "return 1"}))
	if err != nil {
		return nil, err
	}
	return draft.Definition, nil
}

// step takes one step of a toolkit given a request and events.
func step(t *testing.T, src string, request any, events ...machine.Event) *machine.StepOut {
	t.Helper()
	def, err := parse(src)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{}
	if request != nil {
		input["request"] = request
	}
	out, err := toolkitType{}.Step(&machine.StepIn{Definition: def, Events: events, Input: input, Time: time.Unix(0, 0), Seed: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestToolkitDefinitionIsChecked(t *testing.T) {
	for _, c := range []struct{ change, with, problem string }{
		{"description: Notes kept on shelves.\n", "", "description: a toolkit says what it is for"},
		{"tools:", "job: { script: ./add.js }\ntools:", "job: a toolkit has job:, which answers every request, or instructions: and tools:, and not both"},
		{"toolkit: notes\n", "toolkit: notes\nlifetime: singleton\n", "lifetime: a toolkit is given a request when it starts"},
		{"    description: Add a note.\n", "", "tools.add_note.description: a tool says what it is for"},
		{"${args.text}", "${args.shelf}", `it reads "args.shelf", which the tool's args do not name`},
		{"    job: { script: ./add.js, input: { text: \"${args.text}\" } }\n", "", "tools.add_note: a tool needs a job"},
	} {
		_, err := parse(strings.Replace(notes, c.change, c.with, 1))
		if err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("with %q in place of %q the problem is %v, want one holding %q", c.with, c.change, err, c.problem)
		}
	}
	if _, err := parse("toolkit: empty\ndescription: Nothing.\n"); err == nil || !strings.Contains(err.Error(), "a toolkit has tools:, or one job:") {
		t.Errorf("a toolkit with no tools and no job gave %v", err)
	}
	for _, src := range []string{notes, server} {
		if _, err := parse(src); err != nil {
			t.Errorf("a toolkit as it should be written was refused: %v", err)
		}
	}
}

func TestToolkitRefusesARequestItCannotRead(t *testing.T) {
	for _, c := range []struct {
		request any
		problem string
	}{
		{nil, "a toolkit is given request: in its input"},
		{map[string]any{"do": "list"}, "request.do: a request's do is open, call, resume or cancel"},
		{map[string]any{"do": "open", "at": "/elsewhere"}, "request.at: a request holds do, toolkit, tool, args, state and responses"},
		{map[string]any{"do": "call"}, "request.tool: a call names its tool"},
		{map[string]any{"do": "resume", "tool": "add_note"}, "request.state: a resume gives the state its call ended with"},
		{map[string]any{"do": "open", "toolkit": "inner"}, "request.toolkit: this is a list of the names"},
	} {
		for _, src := range []string{notes, server} {
			out := step(t, src, c.request, machine.Event{Name: machine.Start})
			if len(out.Requests) != 1 || out.Requests[0].End == nil || out.Requests[0].End.Error == nil ||
				!strings.Contains(out.Requests[0].End.Error.Error, c.problem) {
				got, _ := json.Marshal(out.Requests)
				t.Errorf("the request %v gave %s, want a failure holding %q", c.request, got, c.problem)
			}
		}
	}
}

// The job that answers a call carries the call for hooks to read, and is
// given the toolkit's input.
func TestToolkitJobIsGivenTheRequest(t *testing.T) {
	request := map[string]any{"do": "call", "tool": "find", "args": map[string]any{"text": "milk"}}
	out := step(t, server, request, machine.Event{Name: machine.Start})
	if len(out.Requests) != 1 || out.Requests[0].Job == nil {
		t.Fatalf("a call asked for %+v", out.Requests)
	}
	got, _ := json.Marshal(map[string]any{"input": out.Requests[0].Job.Input, "tool": out.Requests[0].Job.Tool}, json.Deterministic(true))
	want := `{"input":{"request":{"args":{"text":"milk"},"do":"call","tool":"find"}},"tool":{"args":{"text":"milk"},"name":"find","toolkit":"server"}}`
	if string(got) != want {
		t.Fatalf("the job has\n%s\nwant\n%s", got, want)
	}
	if out := step(t, server, map[string]any{"do": "open"}, machine.Event{Name: machine.Start}); out.Requests[0].Job.Tool != nil {
		t.Fatalf("the job of an open carries the tool %v", out.Requests[0].Job.Tool)
	}
}

func Test_toolkit_entry_keeps_only_its_four_fields(t *testing.T) {
	answer := map[string]any{"instructions": "Ask.", "at": "/elsewhere", "entries": []any{
		map[string]any{"kind": "tool", "name": "find", "description": "Find a note.", "args": map[string]any{"type": "object"},
			"job": map[string]any{"call": "/elsewhere"}, "call": "/elsewhere", "approve": false},
		map[string]any{"kind": "toolkit", "name": "review", "description": "How to review.", "args": map[string]any{"type": "object"}, "input": 1},
	}}
	out := step(t, server, map[string]any{"do": "open"}, machine.Event{Name: machine.Done, From: answerJob, Data: answer})
	if len(out.Requests) != 1 || out.Requests[0].End == nil || out.Requests[0].End.Error != nil {
		t.Fatalf("the answer gave %+v", out.Requests)
	}
	got, _ := json.Marshal(out.Requests[0].End.Output, json.Deterministic(true))
	want := `{"entries":[{"args":{"type":"object"},"description":"Find a note.","kind":"tool","name":"find"},` +
		`{"description":"How to review.","kind":"toolkit","name":"review"}],"instructions":"Ask."}`
	if string(got) != want {
		t.Fatalf("the open gives\n%s\nwant\n%s", got, want)
	}
	for _, c := range []struct {
		entries []any
		problem string
	}{
		{[]any{map[string]any{"kind": "job", "name": "find"}}, "entries[0].kind: an entry's kind is tool or toolkit"},
		{[]any{map[string]any{"kind": "tool"}}, "entries[0].name: an entry has a name"},
		{[]any{map[string]any{"kind": "tool", "name": "a"}, map[string]any{"kind": "toolkit", "name": "a"}}, `entries[1].name: two entries are named "a"`},
	} {
		out := step(t, server, map[string]any{"do": "open"}, machine.Event{Name: machine.Done, From: answerJob, Data: map[string]any{"entries": c.entries}})
		if end := out.Requests[0].End; end == nil || end.Error == nil || !strings.Contains(end.Error.Error, c.problem) {
			t.Errorf("the entries %v gave %+v, want a failure holding %q", c.entries, end, c.problem)
		}
	}
}

// A shape a job gives for a tool's arguments is kept as data: one that
// would not compile, or would cost much to compile, ends the step as any
// other does.
func Test_toolkit_shape_from_a_job_is_not_compiled(t *testing.T) {
	huge := map[string]any{"type": 4, "pattern": strings.Repeat("(a{1000})", 3000)}
	entries := []any{map[string]any{"kind": "tool", "name": "a", "args": map[string]any{"allOf": []any{huge, huge, huge, huge}}}}
	out := step(t, server, map[string]any{"do": "open"}, machine.Event{Name: machine.Done, From: answerJob, Data: map[string]any{"entries": entries}})
	end := out.Requests[0].End
	if end == nil || end.Error != nil {
		t.Fatalf("the open ended %+v", end)
	}
	got, _ := json.Marshal(end.Output)
	if !strings.Contains(string(got), `"type":4`) {
		t.Fatalf("the shape was not kept as it was given: %.200s", got)
	}
}

// A resume waits a second at the least between two asks, and a wait that
// would end after its limit ends it.
func TestToolkitResumeWaitIsHeldToItsLimit(t *testing.T) {
	def, err := parse(server)
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"do": "resume", "tool": "find", "state": map[string]any{}}
	answers := func(wait float64) machine.Request {
		t.Helper()
		st, _ := json.Marshal(state{Until: time.Unix(600, 0)})
		out, err := toolkitType{}.Step(&machine.StepIn{Definition: def, State: st, Input: map[string]any{"request": request}, Time: time.Unix(0, 0), Seed: []byte("s"),
			Events: []machine.Event{{Name: machine.Done, Data: map[string]any{"working": map[string]any{"wait": wait}, "state": map[string]any{}}}}})
		if err != nil || len(out.Requests) != 1 {
			t.Fatalf("a wait of %v gave %+v, %v", wait, out, err)
		}
		return out.Requests[0]
	}
	if r := answers(0.001); r.Timer == nil || !r.Timer.At.Equal(time.Unix(1, 0)) {
		t.Errorf("a wait of a millisecond asked for %+v", r)
	}
	for _, long := range []float64{601, 1e12, 1e300} {
		if r := answers(long); r.End == nil {
			t.Errorf("a wait of %v seconds asked for %+v", long, r)
		}
	}
}

// orders is a toolkit at kits/orders whose tools name machines of its tree.
const orders = `toolkit: orders
description: The orders app.
input: { team: string }
tools:
  read_file: { call: files/read, input: { project: "${input.team}/orders" } }
  change_file: { machine: ./change.yaml, approve: true, meta: { channel: "#orders-team" } }
`

// tree is the tree the toolkit orders is in.
var tree = map[string]string{
	"files/read.yaml":  "flow: read\ndescription: Read one file of a project.\ninput: { project: string, path: string, ref: \"string?\" }\nstages:\n  read: { job: { script: ./add.js } }\n",
	"files/add.js":     "return 1",
	"kits/change.yaml": "flow: change\ndescription: Change one file.\ninput: { path: string, text: string }\nstages:\n  change: { job: { script: ./add.js } }\n",
	"kits/add.js":      "return 1",
	"kits/plain.yaml":  "flow: plain\nstages:\n  one: { job: { script: ./add.js } }\n",
	"kits/keyed.yaml":  "statechart: keyed\ndescription: Kept.\nlifetime: keyed\ninitial: a\nstates:\n  a: { end: done }\n",
	"kits/notes.yaml":  notes,
	"kits/all.yaml":    "flow: all\ndescription: Any of these.\ninput: { type: object, anyOf: [{ required: [a] }, { required: [b] }] }\nstages:\n  one: { job: { script: ./add.js } }\n",
}

// parseOrders reads the toolkit orders, as written, at kits/orders in its tree.
func parseOrders(src string) (*machine.Definition, error) {
	load := func(name string) ([]byte, error) {
		text, ok := tree[name]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(text), nil
	}
	draft, err := machine.ParseAt("kits/orders.yaml", []byte(src), load, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	return draft.Definition, nil
}

// A tool written machine: or call: takes its description and its arguments
// from the machine it names, without the names it fixes, and its job runs
// that machine with the fixed input over the arguments.
func TestToolkitToolNamesAMachine(t *testing.T) {
	def, err := parseOrders(orders)
	if err != nil {
		t.Fatal(err)
	}
	var kit Toolkit
	if err := json.Unmarshal(def.Body, &kit); err != nil {
		t.Fatal(err)
	}
	read, change := kit.Tools["read_file"], kit.Tools["change_file"]
	if read.Description != "Read one file of a project." || change.Description != "Change one file." {
		t.Errorf("the descriptions are %q and %q", read.Description, change.Description)
	}
	if want := map[string]any{"type": "object", "additionalProperties": false, "required": []any{"path"},
		"properties": map[string]any{"path": map[string]any{"type": "string"}, "ref": map[string]any{"type": "string"}}}; !reflect.DeepEqual(read.Args, want) {
		t.Errorf("read_file's arguments are %v", read.Args)
	}
	if read.Job.Call == nil || read.Job.Call.At != "files/read" || len(read.Job.Allow) != 1 || read.Job.Allow[0] != "send:files/read" {
		t.Errorf("read_file's job is %+v", read.Job)
	}
	if change.Job.Machine == "" || !change.Job.Approve || len(change.Job.Allow) != 0 {
		t.Errorf("change_file's job is %+v", change.Job)
	}
	input := map[string]any{"team": "group", "request": map[string]any{"do": "call", "tool": "read_file", "args": map[string]any{"path": "a.txt", "ref": "main"}}}
	out, err := toolkitType{}.Step(&machine.StepIn{Definition: def, Events: []machine.Event{{Name: machine.Start}}, Input: input, Time: time.Unix(0, 0), Seed: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Requests) != 1 || out.Requests[0].Job == nil {
		t.Fatalf("the call asked for %+v", out.Requests)
	}
	job := out.Requests[0].Job
	if want := map[string]any{"path": "a.txt", "ref": "main", "project": "group/orders"}; !reflect.DeepEqual(job.Input, want) {
		t.Errorf("the machine is given %v", job.Input)
	}
	if want := map[string]any{"toolkit": "orders", "name": "read_file", "args": map[string]any{"path": "a.txt", "ref": "main"}}; !reflect.DeepEqual(job.Tool, want) {
		t.Errorf("job.tool is %v", job.Tool)
	}
}

// What a tool that names a machine is refused for, each at its place.
func TestToolkitToolThatNamesAMachineIsChecked(t *testing.T) {
	for _, c := range []struct{ tool, problem string }{
		{"{ call: files/read, description: Reads. }", "tools.t.description: a tool with machine: or call: has no description"},
		{"{ call: files/read, machine: ./change.yaml }", "tools.t: a tool names its machine by its file with machine:, or by its location with call:, and not both"},
		{"{ call: /other/files/read }", "tools.t.call: a toolkit's tool names a location of its own tree"},
		{"{ call: files/none }", "tools.t.call: there is no machine at files/none"},
		{"{ call: kits/notes }", "tools.t.call: the machine at kits/notes is a toolkit"},
		{"{ machine: ./notes.yaml }", "tools.t.machine: the machine ./kits/notes.yaml is a toolkit"},
		{"{ call: kits/plain }", "tools.t.call: the machine at kits/plain has no description:"},
		{"{ call: kits/keyed }", "tools.t.call: the machine at kits/keyed is keyed"},
		{"{ call: kits/all }", "tools.t.call: the input of the machine at kits/all is written with anyOf"},
		{"{ call: files/read, input: { branch: main } }", `tools.t.input.branch: the machine at files/read takes no input named "branch"`},
		{"{ call: files/read, input: { project: \"${args.path}\" } }", "tools.t.input.project: in ${args.path}: it reads \"args\""},
		{"{ description: Adds., args: {}, job: { script: ./add.js }, approve: true }", "tools.t.approve: approve: belongs to a tool with machine: or call:"},
	} {
		_, err := parseOrders("toolkit: orders\ndescription: The orders app.\ntools:\n  t: " + c.tool + "\n")
		if err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("the tool %s gave %v, want a problem holding %q", c.tool, err, c.problem)
		}
	}
	// A machine made while running has no tree, so it names no location.
	if _, err := machine.ParseMade([]byte("toolkit: orders\ndescription: The orders app.\ntools:\n  t: { call: files/read }\n"), machine.Given(nil)); err == nil || !strings.Contains(err.Error(), "tools.t.call: a machine made while running has no tree, and calls no location") {
		t.Errorf("a machine made while running gave %v", err)
	}
	// A file read alone has no tree, so it names no location.
	if _, err := parse("toolkit: orders\ndescription: The orders app.\ntools:\n  t: { call: files/read }\n"); err == nil || !strings.Contains(err.Error(), "tools.t.call: the machine at files/read is read from the tree this file is in") {
		t.Errorf("a file read alone gave %v", err)
	}
}
