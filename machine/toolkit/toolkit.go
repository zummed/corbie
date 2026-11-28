// Package toolkit is the machine that answers requests for tools: an open
// gives its instructions and its entries, and a call runs one tool. Its
// tools are written in its definition, or one job answers every request.
package toolkit

import (
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/zummed/corbie/machine"
)

// Toolkit is the full form of the fields of a definition that only a
// toolkit has. It has Tools, or Job.
type Toolkit struct {
	// Instructions are the text an open gives, filled from the input.
	Instructions string           `json:"instructions,omitzero"`
	Tools        map[string]*Tool `json:"tools,omitzero"`
	// Job answers every request, in place of Instructions and Tools.
	Job *machine.JobSpec `json:"job,omitzero"`
	// Resume is how long a resume waits for work to end; ten minutes if it
	// says nothing.
	Resume machine.Duration `json:"resume,omitzero"`
}

// DefaultResume is how long a resume waits for work to end by default, and
// defaultWait how long it waits between two asks where the answer gives no
// time.
const (
	DefaultResume = 10 * time.Minute
	defaultWait   = 5 * time.Second
)

// waitTimer is the name of the timer a resume waits on between two asks.
const waitTimer = "wait"

// state is what a resume that waits remembers between steps.
type state struct {
	// State is the state the job last gave, which the next ask gives back.
	State any `json:"state,omitzero"`
	// Until is when the resume stops waiting.
	Until time.Time `json:"until,omitzero"`
}

// Tool is a named job a call runs, with its description and the shape of
// its arguments, a JSON Schema of an object. Written machine: or call: it
// names a machine, whose description it takes, and its arguments are that
// machine's input without the names the tool fixes.
type Tool struct {
	Description string           `json:"description"`
	Args        map[string]any   `json:"args,omitzero"`
	Job         *machine.JobSpec `json:"job"`
	// Machine is the file of the machine the tool names, and Call its
	// location. Job runs it with the input the tool fixes.
	Machine string `json:"machine,omitzero"`
	Call    string `json:"call,omitzero"`
}

// named says the tool names a machine, by its file or its location.
func (t *Tool) named() bool { return t.Machine != "" || t.Call != "" }

// What a request asks, as its do: says.
const (
	Open   = "open"
	Call   = "call"
	Resume = "resume"
	Cancel = "cancel"
)

// The kinds of entry an open gives.
const (
	KindTool    = "tool"
	KindToolkit = "toolkit"
)

// The tool a toolkit with files: has, and its script.
const (
	readTool   = "read"
	readScript = "corbie:read.js"
	readSource = `return require(input.files[input.path])
`
)

// answerJob is the name of the job that answers every request.
const answerJob = "job"

// shortForm is the short form of a tool's arguments. A blob there is the
// blob itself, as in a machine's input, and a name the shape does not give
// is refused.
var shortForm = machine.ShortForm{Blob: machine.InputForm.Blob, Closed: true}

//go:embed manual.txt
var manual string

type toolkitType struct{}

func init() { machine.Register(toolkitType{}) }

func (toolkitType) Name() string { return "toolkit" }

// Limits adds how long a resume waits to the limits every type takes.
func (toolkitType) Limits() []machine.Field {
	return []machine.Field{{Name: "resume", Kind: machine.KindDuration, Description: "how long a resume waits for work to end before it answers working again; ten minutes by default"}}
}

// Shape is what a toolkit's definition holds of its own.
func (toolkitType) Shape() *machine.Shape {
	tool := &machine.Shape{Name: "tool", What: "a tool", Fields: []machine.Field{
		{Name: "description", Kind: machine.KindText, Description: "what the tool is for, for the model to choose by"},
		{Name: "args", Kind: machine.KindTypes, Description: "the shape of its arguments: each name with the type of its value, or a JSON Schema with type: object"},
		{Name: "job", Kind: machine.KindJob, Description: "the job a call of the tool starts; it reads input and args"},
		{Name: "machine", Kind: machine.KindWord, Path: true,
			Description: "the file of a machine, in place of description, args and job: the description is that machine's, and the arguments are its input"},
		{Name: "call", Kind: machine.KindWord, Description: "the location of a machine in the same tree, in place of machine:"},
		{Name: "input", Kind: machine.KindValue,
			Description: "of a tool with machine: or call:, the names of the machine's input the tool fixes; it reads the toolkit's input"},
		{Name: "approve", Kind: machine.KindFlag, Description: "of a tool with machine: or call:, marks the tool's job for approval, so it starts only after a hook before it"},
		{Name: "meta", Kind: machine.KindValue, Description: "of a tool with machine: or call:, data of any kind, which hooks read as job.meta and nothing else reads"},
	}}
	return &machine.Shape{Name: "toolkit", What: "a toolkit", Fields: []machine.Field{
		{Name: "instructions", Kind: machine.KindText, Description: "the text an open gives; it reads the toolkit's input"},
		{Name: "tools", Kind: machine.KindNamed, Of: tool, Description: "the toolkit's tools, each a named job"},
		{Name: "files", Kind: machine.KindList, Path: true,
			Description: "text files a caller reads with the tool read, which the toolkit then has: a skill's files beside its SKILL.md"},
		{Name: "job", Kind: machine.KindJob,
			Description: "one job that answers every request, in place of instructions and tools; it is given the toolkit's input"},
	}}
}

// Fill reads a toolkit as written and gives its full form.
func (toolkitType) Fill(p *machine.Parser, f *machine.Fields) any {
	t := &Toolkit{Instructions: f.Text("instructions"), Resume: p.Limits().Duration("resume")}
	if job, ok := f.Value("job"); ok {
		t.Job = p.Job("job", job)
	}
	// With files:, the toolkit has a tool that reads one of them: a script
	// of Corbie's own, which is sent the files and nothing else.
	files := map[string]any{}
	var sent []string
	for _, written := range f.Strings("files") {
		clean := p.CleanPath(written)
		p.File("files", clean)
		files[strings.TrimPrefix(written, "./")] = clean
		sent = append(sent, clean)
	}
	if len(files) > 0 {
		p.Builtin(readScript, readSource)
		paths := []any{}
		for _, name := range sortedNames(files) {
			paths = append(paths, name)
		}
		t.Tools = map[string]*Tool{readTool: {Description: "Read one of the toolkit's files.",
			Args: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "enum": paths}},
				"required": []any{"path"}, "additionalProperties": false},
			Job: &machine.JobSpec{Script: readScript, Files: sent, Alone: true, Input: map[string]any{"path": "${args.path}", "files": files}}}}
	}
	tools, order := f.Map("tools")
	for _, name := range order {
		place := "tools." + name
		if _, reads := t.Tools[name]; reads {
			p.Problem(place, "the tool %q has the name of the tool a toolkit with files: has for reading them", name)
			continue
		}
		tf := p.Fields(place, tools[name], f.Of("tools"))
		if tf == nil {
			continue
		}
		var tool *Tool
		if tf.Has("machine") || tf.Has("call") {
			tool = fillNamed(p, tf, place)
		} else {
			for _, field := range []string{"input", "approve", "meta"} {
				if tf.Has(field) {
					p.Problem(place+"."+field, "%s: belongs to a tool with machine: or call:; a tool with job: writes its input, approve and meta in the job", field)
				}
			}
			tf.Value("input")
			tf.Bool("approve")
			tf.Value("meta")
			tool = &Tool{Description: tf.Text("description")}
			args, _ := tf.Map("args")
			full, err := shortForm.Full(args)
			if err != nil {
				p.Problem(place+".args", "%v", err)
				full = args
			}
			tool.Args = full
			if job, ok := tf.Value("job"); ok {
				tool.Job = p.Job(place+".job", job)
			}
		}
		tf.Done()
		if t.Tools == nil {
			t.Tools = map[string]*Tool{}
		}
		t.Tools[name] = tool
	}
	return t
}

// fillNamed reads a tool written with machine: or call:. Nothing else of a
// tool is written for it: its description is the machine's, its arguments
// are the machine's input without the names the tool fixes, and its job
// runs the machine. The machine is read with the definition, so it is a
// file, or a location of the same tree.
func fillNamed(p *machine.Parser, tf *machine.Fields, place string) *Tool {
	for _, field := range []string{"description", "args", "job"} {
		if tf.Has(field) {
			p.Problem(place+"."+field, "a tool with machine: or call: has no %s: the description and arguments come from the machine it names", field)
		}
	}
	tf.Text("description")
	tf.Map("args")
	tf.Value("job")
	tool := &Tool{}
	var target *machine.Definition
	tool.Job, tool.Machine, tool.Call, target = p.Named(tf, place, "a tool")
	at, the := ".machine", "the machine "+tool.Machine
	if tool.Call != "" {
		at, the = ".call", "the machine at "+tool.Call
	}
	if strings.HasPrefix(tool.Call, "/") {
		p.Problem(place+at, "a toolkit's tool names a location of its own tree, written from the tree's prefix, and %s is written in full", tool.Call)
	}
	if target == nil {
		return tool
	}
	tool.Description = target.Description
	switch {
	case target.Type == "toolkit":
		p.Problem(place+at, "%s is a toolkit: a toolkit's tool names a machine that is no toolkit", the)
	case target.Description == "":
		p.Problem(place+at, "%s has no description:, which is the tool's", the)
	}
	if tool.Call != "" && target.Lifetime != machine.LifetimeTransient {
		p.Problem(place+at, "%s is %s: a call starts a run only of a transient location", the, target.Lifetime)
	}
	fixed, _ := tool.Job.Input.(map[string]any)
	var wrong []machine.Wrong
	tool.Args, wrong = machine.Recorded{Input: target.Input}.Args(at, the, fixed)
	for _, w := range wrong {
		p.Problem(place+w.At, "%s", w.Text)
	}
	return tool
}

// Check reports what is wrong with a toolkit in full form.
func (toolkitType) Check(c *machine.Checker, def *machine.Definition) {
	var t Toolkit
	if !c.DecodeBody(def, &t) {
		return
	}
	if def.Description == "" {
		c.Problem("description", "a toolkit says what it is for, for a model to choose by")
	}
	if def.Lifetime == machine.LifetimeSingleton {
		c.Problem("lifetime", "a toolkit is given a request when it starts, so it cannot be a singleton")
	}
	c.Value("instructions", t.Instructions, "input")
	switch {
	case t.Job != nil && (len(t.Tools) > 0 || t.Instructions != ""):
		c.Problem("job", "a toolkit has job:, which answers every request, or instructions: and tools:, and not both")
	case t.Job != nil:
		c.Job("job", t.Job, "input")
	case len(t.Tools) == 0:
		c.Problem("tools", "a toolkit has tools:, or one job: that answers every request")
	}
	for _, name := range sortedNames(t.Tools) {
		tool, place := t.Tools[name], "tools."+name
		c.Name(place, name)
		if tool == nil || tool.Job == nil {
			c.Problem(place, "a tool needs a job")
			continue
		}
		if tool.named() {
			// What the machine gives was checked as it was read, and one
			// that could not be read has its problem.
			if tool.Args != nil {
				if _, err := machine.CompileShape(tool.Args); err != nil {
					c.Problem(place, "%v", err)
				}
			}
			c.Job(place, tool.Job, "input")
			continue
		}
		if tool.Description == "" {
			c.Problem(place+".description", "a tool says what it is for, for the model to choose by")
		} else if machine.HasExpr(tool.Description) {
			c.Problem(place+".description", "a description is plain text, with no expression")
		}
		if _, err := machine.CompileShape(tool.Args); err != nil {
			c.Problem(place+".args", "%v", err)
		}
		c.Job(place+".job", tool.Job, "args", "input")
		// A job that reads an argument the shape does not name would
		// always find it missing.
		properties, _ := tool.Args["properties"].(map[string]any)
		for _, read := range machine.ReadsIn(append([]any{tool.Job.Input}, tool.Job.Addressed()...), "args") {
			if _, named := properties[read.Name]; !named {
				c.Problem(place+".job", "it reads \"args.%s\", which the tool's args do not name", read.Name)
			}
		}
	}
}

// A request is what a run of a toolkit is asked, as its input's request:
// gives it.
type request struct {
	Do   string
	Tool string
	Args any
	Said string // what the caller says of the call, which a hook reads
}

// requestOf reads the request from a run's input, and says what is wrong
// with it.
func requestOf(input any) (*request, error) {
	whole, _ := input.(map[string]any)
	written, ok := whole["request"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("a toolkit is given request: in its input, as { do: open } or { do: call, tool, args }")
	}
	for _, name := range sortedNames(written) {
		if !slices.Contains([]string{"do", "toolkit", "tool", "args", "state", "responses", "said"}, name) {
			return nil, fmt.Errorf("request.%s: a request holds do, toolkit, tool, args, state, responses and said", name)
		}
	}
	r := &request{Args: written["args"]}
	r.Do, _ = written["do"].(string)
	r.Tool, _ = written["tool"].(string)
	switch {
	case !slices.Contains([]string{Open, Call, Resume, Cancel}, r.Do):
		return nil, fmt.Errorf("request.do: a request's do is %s, %s, %s or %s", Open, Call, Resume, Cancel)
	case r.Do != Open && r.Tool == "":
		return nil, fmt.Errorf("request.tool: a %s names its tool", r.Do)
	case (r.Do == Resume || r.Do == Cancel) && written["state"] == nil:
		return nil, fmt.Errorf("request.state: a %s gives the state its call ended with", r.Do)
	}
	if said, written := written["said"]; written {
		text, isText := said.(string)
		if !isText || r.Do == Open {
			return nil, fmt.Errorf("request.said: this is a text, what the caller says of a call, a resume or a cancel")
		}
		r.Said = text
	}
	if path, written := written["toolkit"]; written {
		names, isList := path.([]any)
		for _, name := range names {
			if text, _ := name.(string); text == "" {
				isList = false
			}
		}
		if !isList {
			return nil, fmt.Errorf("request.toolkit: this is a list of the names that lead to a toolkit inside this one")
		}
	}
	if r.Args == nil {
		r.Args = map[string]any{}
	}
	return r, nil
}

// Step takes a toolkit's step. At its start it answers an open itself, or
// asks for the job the request needs, and that job's end is the toolkit's.
// A resume whose job answers working waits on a timer and asks again, until
// the work ends or the resume has waited as long as it may.
func (toolkitType) Step(in *machine.StepIn) (*machine.StepOut, error) {
	var t Toolkit
	if err := json.Unmarshal(in.Definition.Body, &t); err != nil {
		return nil, err
	}
	st := &state{}
	if in.State != nil {
		if err := json.Unmarshal(in.State, st); err != nil {
			return nil, err
		}
	}
	out := &machine.StepOut{State: st}
	end := func(e *machine.Ending) { out.Requests = append(out.Requests, machine.Request{End: e}) }
	fail := func(err error) {
		end(&machine.Ending{Error: &machine.Error{Status: machine.StatusError, Error: err.Error()}})
	}
	// ask starts the job a request needs, or ends the run with the answer
	// the toolkit gives itself.
	ask := func(input any) {
		r, err := requestOf(input)
		if err != nil {
			fail(err)
			return
		}
		answer, job, err := t.start(in, r, input)
		switch {
		case err != nil:
			fail(err)
		case job != nil:
			if r.Do != Open {
				tool := map[string]any{"toolkit": in.Definition.Name, "name": r.Tool, "args": r.Args}
				if r.Said != "" {
					tool["said"] = r.Said
				}
				job.Tool = tool
			}
			out.Requests = append(out.Requests, machine.Request{Job: job})
		default:
			end(&machine.Ending{Output: answer})
		}
	}
	for i, event := range in.Events {
		switch event.Name {
		case machine.Start:
			limit := time.Duration(t.Resume)
			if limit == 0 {
				limit = DefaultResume
			}
			st.Until = in.Time.Add(limit)
			ask(in.Input)
		case machine.Done:
			r, err := requestOf(in.Input)
			if err != nil {
				return nil, err
			}
			answer, err := t.answer(r.Do, event.Data)
			if err != nil {
				fail(err)
				continue
			}
			// A resume waits while the work goes on, and the time fits.
			given, _ := answer.(map[string]any)
			working, goesOn := given["working"]
			wait := defaultWait
			said, _ := working.(map[string]any)
			if seconds, ok := said["wait"].(float64); ok && seconds > 0 {
				// At least a second, and no more than ends after the limit.
				most := st.Until.Sub(in.Time).Seconds() + 1
				wait = time.Duration(min(max(seconds, 1), most) * float64(time.Second))
			}
			if r.Do != Resume || !goesOn || in.Time.Add(wait).After(st.Until) {
				end(&machine.Ending{Output: answer})
				continue
			}
			st.State = given["state"]
			out.Requests = append(out.Requests, machine.Request{Timer: &machine.TimerAt{Name: waitTimer, At: in.Time.Add(wait)}})
		case machine.Timer:
			// The next ask gives back the state the last answer gave.
			whole, _ := in.Input.(map[string]any)
			asked, _ := whole["request"].(map[string]any)
			input, request := maps.Clone(whole), maps.Clone(asked)
			request["state"] = st.State
			input["request"] = request
			ask(input)
		case machine.Failed:
			end(&machine.Ending{Error: machine.Unhandled(event.From, event.Data)})
		default:
			out.Ignored = append(out.Ignored, i)
		}
	}
	return out, nil
}

// start gives the answer to a request the toolkit answers itself, or the
// job that answers it. input is the run's input, with the request in it.
func (t *Toolkit) start(in *machine.StepIn, r *request, input any) (any, *machine.Job, error) {
	eval := machine.NewEvaluator(in.Time, in.Seed, in.Run)
	scope := machine.Scope{"input": input}
	if t.Job != nil {
		job, err := t.Job.Fill(eval, scope, "job", answerJob)
		if err != nil {
			return nil, nil, err
		}
		if t.Job.Input == nil {
			job.Input = input
		}
		return nil, job, nil
	}
	whole, _ := input.(map[string]any)
	if _, inside := whole["request"].(map[string]any)["toolkit"]; inside {
		return nil, nil, fmt.Errorf("request.toolkit: the toolkit %s has no toolkit inside it", in.Definition.Name)
	}
	if r.Do == Open {
		instructions, err := eval.Fill(t.Instructions, scope, "instructions")
		if err != nil {
			return nil, nil, err
		}
		entries := []any{}
		for _, name := range sortedNames(t.Tools) {
			entries = append(entries, map[string]any{"kind": KindTool, "name": name,
				"description": t.Tools[name].Description, "args": t.Tools[name].Args})
		}
		return map[string]any{"instructions": instructions, "entries": entries}, nil, nil
	}
	tool, ok := t.Tools[r.Tool]
	if !ok {
		return nil, nil, fmt.Errorf("the toolkit %s has no tool %q (it has: %s)", in.Definition.Name, r.Tool, strings.Join(sortedNames(t.Tools), ", "))
	}
	schema, err := machine.CompileShape(tool.Args)
	if err != nil {
		return nil, nil, err
	}
	if err := machine.Fits(schema, r.Args); err != nil {
		return nil, nil, fmt.Errorf("the arguments do not fit the tool %q: %v", r.Tool, err)
	}
	if tool.named() {
		// A tool that names a machine gives it the arguments as they were
		// given, with the input the tool fixes over them.
		job, err := tool.Job.Fill(eval, scope, "tools."+r.Tool, r.Tool)
		if err != nil {
			return nil, nil, err
		}
		input := map[string]any{}
		given, _ := r.Args.(map[string]any)
		maps.Copy(input, given)
		fixed, _ := job.Input.(map[string]any)
		maps.Copy(input, fixed)
		job.Input = input
		return nil, job, nil
	}
	scope["args"] = r.Args
	job, err := tool.Job.Fill(eval, scope, "tools."+r.Tool+".job", r.Tool)
	return nil, job, err
}

// answer gives the toolkit's answer from what its job gave. A tool's job
// gives its result, or { working, state } or { requests, state } in place
// of one. A job that answers every request gives the answer itself.
func (t *Toolkit) answer(do string, data any) (any, error) {
	if t.Job != nil {
		return answered(do, data)
	}
	given, _ := data.(map[string]any)
	_, state := given["state"]
	_, working := given["working"]
	_, requests := given["requests"]
	if do != Cancel && len(given) == 2 && state && (working || requests) {
		return given, nil
	}
	return map[string]any{"result": data}, nil
}

// answered gives the answer a toolkit's job gave to a request, holding
// only what an answer holds, or says what is wrong with it.
func answered(do string, data any) (any, error) {
	given, _ := data.(map[string]any)
	has := func(names ...string) bool {
		for _, name := range names {
			if _, ok := given[name]; !ok {
				return false
			}
		}
		return len(given) == len(names)
	}
	switch {
	case do == Open:
		return opened(given)
	case has("result"), do != Cancel && (has("working", "state") || has("requests", "state")):
		return given, nil
	case do == Cancel:
		return nil, fmt.Errorf("the toolkit's job answers a cancel with { result }")
	}
	return nil, fmt.Errorf("the toolkit's job answers a %s with { result }, { working, state } or { requests, state }", do)
}

// opened gives the answer to an open: the instructions, and each entry
// with its kind, its name, its description and, for a tool, the shape of
// its arguments. Anything else an entry holds is dropped.
func opened(given map[string]any) (any, error) {
	instructions, isText := given["instructions"].(string)
	list, isList := given["entries"].([]any)
	if (!isText && given["instructions"] != nil) || !isList {
		return nil, fmt.Errorf("the toolkit's job answers an open with { instructions, entries }: a text and a list")
	}
	entries := []any{}
	seen := map[string]bool{}
	for i, item := range list {
		entry, _ := item.(map[string]any)
		kind, _ := entry["kind"].(string)
		name, _ := entry["name"].(string)
		description, _ := entry["description"].(string)
		switch {
		case kind != KindTool && kind != KindToolkit:
			return nil, fmt.Errorf("entries[%d].kind: an entry's kind is %s or %s", i, KindTool, KindToolkit)
		case name == "":
			return nil, fmt.Errorf("entries[%d].name: an entry has a name", i)
		case seen[name]:
			return nil, fmt.Errorf("entries[%d].name: two entries are named %q", i, name)
		}
		seen[name] = true
		kept := map[string]any{"kind": kind, "name": name, "description": description}
		if kind == KindTool {
			args, _ := entry["args"].(map[string]any)
			if args == nil {
				args = map[string]any{"type": "object"}
			}
			kept["args"] = args
		}
		entries = append(entries, kept)
	}
	return map[string]any{"instructions": instructions, "entries": entries}, nil
}

func sortedNames[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Show gives nothing: a toolkit has at most one job, which its record
// shows.
func (toolkitType) Show(jsontext.Value) machine.Shown { return machine.Shown{} }

func (toolkitType) Manual() string { return manual }
