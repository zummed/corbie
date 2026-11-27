package toolkit

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
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
