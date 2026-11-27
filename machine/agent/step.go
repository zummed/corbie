package agent

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/toolkit"
)

// state is everything an agent remembers between steps. The conversation
// is kept as its turns, each written once, as a blob, by the job that made
// the turn; nothing here is rewritten as the conversation grows.
type state struct {
	// Turns are the blobs that hold each turn's part of the conversation.
	Turns []any `json:"turns,omitzero"`
	// Turn is how many turns have been taken, and Limit how many may be.
	Turn  int `json:"turn"`
	Limit int `json:"limit"`
	// Waiting says what the agent waits for: a turn or its tools.
	Waiting string `json:"waiting,omitzero"`
	// Calls are the calls of the current turn, in the order the model made
	// them.
	Calls []*call `json:"calls,omitzero"`
	// Next is what the next turn adds to the conversation.
	Next []any `json:"next,omitzero"`
	// Tokens counts the tokens the agent's own turns used.
	Tokens map[string]float64 `json:"tokens,omitzero"`
	// Handles are the blobs the model has been shown, by the short name
	// each was shown under. A name always means the same blob.
	Handles map[string]any `json:"handles,omitzero"`
	// Items is how many items each of those blobs has; less than none for
	// one whose type has no items, and nothing for one not yet counted.
	Items map[string]float64 `json:"items,omitzero"`
	// Fresh are the handles named since the last turn: blobs the model has
	// not been shown yet, each of which is given a preview first.
	Fresh []string `json:"fresh,omitzero"`
	// Previews are the previews being made before the next turn.
	Previews []*preview `json:"previews,omitzero"`
	// Wrong says what was wrong with the last reply, if it did not fit.
	Wrong string `json:"wrong,omitzero"`
	// Opening are the opens of the toolkits that are opened before the
	// first turn.
	Opening []*opening `json:"opening,omitzero"`
	// Opened are the toolkits that are open, each a list of entries, by the
	// list's name: an entry's name, or a list's name and the name of a
	// toolkit in it.
	Opened map[string]*kit `json:"opened,omitzero"`
	// Unoffered are the entries whose when: did not hold when the run
	// started: the model is never shown them.
	Unoffered []string `json:"unoffered,omitzero"`
	// Toolkits is the name of the blob that holds the list of the agent's
	// own toolkits, where that list is too large to show.
	Toolkits string `json:"toolkits,omitzero"`
	// Tasks are the calls of toolkits' tools that have not ended, by the
	// name the model knows each by.
	Tasks map[string]*task `json:"tasks,omitzero"`
}

// task is a call of a toolkit's tool that ended working, or with requests
// for input: what a resume needs to carry it on.
type task struct {
	Entry string   `json:"entry"`
	Path  []string `json:"path,omitzero"`
	Tool  string   `json:"tool"`
	// Args are the arguments the model gave the call, and Sent the same
	// with each blob's name turned into the blob, where they hold one.
	Args any `json:"args,omitzero"`
	Sent any `json:"sent,omitzero"`
	// State is what the toolkit last gave, which a resume gives back.
	State any `json:"state,omitzero"`
	// Requests are what the call asks for, where it asks for input.
	Requests any `json:"requests,omitzero"`
}

// opening is one open of a toolkit: the list it makes, the agent's entry
// whose job opens it, and the names that lead to it inside that entry's
// toolkit.
type opening struct {
	Job   string   `json:"job,omitzero"`
	List  string   `json:"list"`
	Entry string   `json:"entry"`
	Path  []string `json:"path,omitzero"`
	Done  bool     `json:"done,omitzero"`
}

// kit is an open toolkit: the entries its open gave. Every job for it is
// the job of the agent's entry it came from.
type kit struct {
	Entry   string   `json:"entry"`
	Path    []string `json:"path,omitzero"`
	Entries []*entry `json:"entries,omitzero"`
	// Stored is the blob that holds the list's entries, where the list is
	// too large to show. Entries are then the tools the model has opened.
	Stored any `json:"stored,omitzero"`
	// Blob is the name of the blob that holds the names of such a list,
	// where they are too large to show too: the model searches it.
	Blob string `json:"blob,omitzero"`
}

// entry is one item of an open toolkit's list.
type entry struct {
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitzero"`
	Args        map[string]any `json:"args,omitzero"`
}

// preview is one job that looks at something before the model is shown it:
// a blob it has not seen, or a tool's result too large to show, which the
// job stores as a blob.
type preview struct {
	Job    string `json:"job"`
	Handle string `json:"handle,omitzero"` // the blob it is of
	Call   int    `json:"call,omitzero"`   // or the call whose result it stores, counted from one
	List   string `json:"list,omitzero"`   // or the list it stores
	Done   bool   `json:"done,omitzero"`
	Shown  any    `json:"shown,omitzero"` // the first few items
}

// showLimit is the most of a tool's result, as JSON, that is shown to the
// model in place by default. A larger one is stored as a blob.
const showLimit = 16 << 10

// show gives the most that is shown to the model in place.
func (s *stepper) show() int {
	if s.a.Show > 0 {
		return int(s.a.Show)
	}
	return showLimit
}

// call is one call the model made in a turn.
type call struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	// Open is the toolkit the call opens, for a call of open.
	Open *opening `json:"open,omitzero"`
	// Task is the task the call carries on, for a call of resume.
	Task string `json:"task,omitzero"`
	// Find is the entry the call opens from a list that is stored, while
	// its job reads the entry.
	Find *finding `json:"find,omitzero"`
	Job  string   `json:"job,omitzero"` // the job that carries it out; none if it ran nothing
	Args any      `json:"args,omitzero"`
	// Sent are the arguments a toolkit was sent, where a blob's name in
	// them was turned into the blob.
	Sent any  `json:"sent,omitzero"`
	Done bool `json:"done,omitzero"`
	// What goes back to the model: a result, an error or a denial.
	Result any    `json:"result,omitzero"`
	Error  string `json:"error,omitzero"`
	// Large says the result is too large to show, and is to be stored.
	Large bool `json:"large,omitzero"`
}

// finding is an open from a list that is stored: the list, the entry's
// name, and what the model said, which a hook of the toolkit's open reads.
type finding struct {
	List string `json:"list"`
	Name string `json:"name"`
	Said string `json:"said,omitzero"`
}

// stepper is one step being taken.
type stepper struct {
	a    *Agent
	in   *machine.StepIn
	st   *state
	out  *machine.StepOut
	eval *machine.Evaluator
	// unread says, by the entry's name, why an entry written call: with a
	// location in full has no description and no arguments.
	unread map[string][]string
}

func (agentType) Step(in *machine.StepIn) (*machine.StepOut, error) {
	var a Agent
	if err := json.Unmarshal(in.Definition.Body, &a); err != nil {
		return nil, err
	}
	s := &stepper{a: &a, in: in, st: &state{}, out: &machine.StepOut{}, eval: machine.NewEvaluator(in.Time, in.Seed, in.Run)}
	s.unread = s.recorded()
	if in.State != nil {
		if err := json.Unmarshal(in.State, s.st); err != nil {
			return nil, err
		}
	}
	for i, event := range in.Events {
		acted, err := s.arrive(event)
		if err != nil {
			return nil, err
		}
		if !acted {
			s.out.Ignored = append(s.out.Ignored, i)
		}
	}
	s.out.State = s.st
	return s.out, nil
}

func (s *stepper) request(r machine.Request) { s.out.Requests = append(s.out.Requests, r) }

// arrive takes one event. An agent acts on the results of its turns, its
// tools, and on nothing else.
func (s *stepper) arrive(event machine.Event) (bool, error) {
	switch {
	case event.Name == machine.Start:
		return true, s.begin()
	case event.From == turnJob && s.st.Waiting == turnJob:
		if event.Name == machine.Failed {
			// A turn that fails fails the agent.
			s.fail(event, "turn")
			return true, nil
		}
		if event.Name == machine.Done {
			return true, s.replied(event.Data)
		}
	case s.st.Waiting == "tools" && (event.Name == machine.Done || event.Name == machine.Failed):
		for _, c := range s.st.Calls {
			if c.Job != event.From || c.Done {
				continue
			}
			c.Done = true
			if c.Find != nil && event.Name == machine.Done {
				if err := s.read(c, event.Data); err != nil {
					return true, err
				}
			} else if event.Name == machine.Done {
				s.done(c, event.Data)
			} else {
				// A tool that fails goes back to the model as an error,
				// with whatever output it still gave.
				data, _ := event.Data.(map[string]any)
				c.Error = fmt.Sprint(data["error"])
				if data["status"] == machine.StatusDenied {
					c.Error = "denied: " + c.Error
				} else if c.Task != "" {
					// A resume that fails for any other reason ends its task.
					delete(s.st.Tasks, c.Task)
				}
				if f := s.find(c.Tool); f != nil && f.kit == nil && c.Open == nil && c.Find == nil {
					s.done(c, data["output"])
					c.Large = false
				}
			}
			return true, s.settled()
		}
	case s.st.Waiting == previewJob && (event.Name == machine.Done || event.Name == machine.Failed):
		for _, o := range s.st.Opening {
			if o.Job != event.From || o.Done {
				continue
			}
			o.Done = true
			// A toolkit that fails to open before the first turn fails the
			// agent, as a turn does.
			if event.Name == machine.Failed {
				s.fail(event, place(o.Entry))
				return true, nil
			}
			instructions, err := s.take(o, event.Data)
			if err != nil {
				s.end(&machine.Error{Status: machine.StatusError, Error: place(o.Entry) + ": " + err.Error()})
				return true, nil
			}
			if instructions != "" {
				s.st.Next = append([]any{map[string]any{"role": "user",
					"content": fmt.Sprintf("(the toolkit %s is open; its instructions follow)\n%s", o.List, instructions)}}, s.st.Next...)
			}
			if s.large(s.listed(s.st.Opened[o.List].Entries)) {
				s.list(&preview{List: o.List})
			} else if inside := s.toolkitsIn(o.List); len(inside) > 0 {
				s.st.Next = append(s.st.Next, map[string]any{"role": "user", "content": map[string]any{"list": o.List, "toolkits": inside, "how": howToOpen(o.List)}})
			}
			return true, s.ready()
		}
		for _, p := range s.st.Previews {
			if p.Job != event.From || p.Done {
				continue
			}
			p.Done = true
			s.previewed(p, event)
			return true, s.ready()
		}
	}
	return false, nil
}

// goesOn takes a toolkit's answer to a call or a resume. With a result the
// call has ended, and its task with it. With working, or with requests for
// input, the call is a task: the model is shown its name, and carries it on
// with resume.
func (s *stepper) goesOn(c *call, t *task, output any) (result any, ended bool) {
	answer, _ := output.(map[string]any)
	if result, ended = answer["result"]; ended {
		delete(s.st.Tasks, c.Task)
		return result, true
	}
	state, kept := answer["state"]
	working, works := answer["working"]
	requests, asks := answer["requests"]
	if t == nil || !kept || !works && !asks {
		c.Error = "the toolkit's answer holds no result"
		return nil, false
	}
	name := c.Task
	for n := len(s.st.Tasks) + 1; name == ""; n++ {
		// A name is never one a task of the conversation has.
		if _, used := s.st.Tasks[fmt.Sprintf("task%d", n)]; !used {
			name = fmt.Sprintf("task%d", n)
		}
	}
	if s.st.Tasks == nil {
		s.st.Tasks = map[string]*task{}
	}
	t.State, t.Requests = state, requests
	s.st.Tasks[name] = t
	if asks {
		c.Result = s.shown(map[string]any{"task": name, "requests": requests})
	} else {
		c.Result = s.shown(map[string]any{"task": name, "working": working})
	}
	return nil, false
}

// done takes the output of a call's job. An open gives the toolkit's
// instructions, and a call of a toolkit's tool the result in its answer.
func (s *stepper) done(c *call, output any) {
	f := s.find(c.Tool)
	switch {
	case c.Open != nil:
		instructions, err := s.take(c.Open, output)
		if err != nil {
			c.Error = err.Error()
			return
		}
		if instructions == "" {
			instructions = "Opened " + c.Open.List + "."
		}
		// What opening a toolkit gives is shown to the model whole, with the
		// toolkits its list holds. A list too large to show is stored first.
		c.Result = instructions
		c.Large = s.large(s.listed(s.st.Opened[c.Open.List].Entries))
		if inside := s.toolkitsIn(c.Open.List); len(inside) > 0 && !c.Large {
			c.Result = map[string]any{"instructions": instructions, "list": c.Open.List, "toolkits": inside, "how": howToOpen(c.Open.List)}
		}
		return
	case c.Task != "":
		var ended bool
		if output, ended = s.goesOn(c, s.st.Tasks[c.Task], output); !ended {
			return
		}
	case f != nil && f.kit != nil:
		var ended bool
		if output, ended = s.goesOn(c, &task{Entry: f.entry, Path: f.kit.Path, Tool: f.name, Args: c.Args, Sent: c.Sent}, output); !ended {
			return
		}
	case f != nil && f.tool.Agent:
		// A tool that is an agent is shown as what that agent replied: its
		// conversation and its count of tokens are in the record, and are
		// not the caller's to read through.
		result, _ := output.(map[string]any)
		output = result["reply"]
	}
	c.Result = s.shown(output)
	text, _ := json.Marshal(c.Result)
	c.Large = len(text) > s.show()
}

// preview asks for a look at what the model is about to be shown for the
// first time: each blob it has not seen, and each result too large to
// show. The next turn waits for them as it waits for tools.
func (s *stepper) preview() error {
	for _, name := range s.st.Fresh {
		s.look(&preview{Handle: name}, map[string]any{"blob": s.st.Handles[name]})
	}
	s.st.Fresh = nil
	for i, c := range s.st.Calls {
		switch {
		case c.Large && c.Open != nil:
			// A list too large to show is stored, and the model searches it.
			s.list(&preview{Call: i + 1, List: c.Open.List})
		case c.Large:
			s.look(&preview{Call: i + 1}, map[string]any{"value": c.Result})
		}
	}
	s.st.Waiting = previewJob
	return s.ready()
}

// name gives a preview job its name, and keeps it.
func (s *stepper) name(p *preview) {
	p.Job = previewJob
	if n := len(s.st.Previews); n > 0 {
		p.Job = fmt.Sprintf("%s.%d", previewJob, n)
	}
	s.st.Previews = append(s.st.Previews, p)
}

// look asks for a preview job.
func (s *stepper) look(p *preview, input map[string]any) {
	s.name(p)
	s.request(machine.Request{Job: &machine.Job{Name: p.Job, Script: previewScript, Alone: true, Input: input}})
}

// list asks for the job that stores a toolkit's list: its entries as one
// blob, and their names for the model to choose by.
func (s *stepper) list(p *preview) {
	s.name(p)
	s.request(machine.Request{Job: &machine.Job{Name: p.Job, Script: listScript, Alone: true,
		Input: map[string]any{"entries": s.listed(s.st.Opened[p.List].Entries), "limit": float64(s.show())}}})
}

// listed gives a list's entries as they are stored.
func (s *stepper) listed(entries []*entry) []any {
	out := []any{}
	for _, e := range entries {
		item := map[string]any{"kind": e.Kind, "name": e.Name, "description": e.Description}
		if e.Kind == toolkit.KindTool {
			item["args"] = e.Args
		}
		out = append(out, item)
	}
	return out
}

// large says a list is too large to show: the limit is that of a tool's
// result.
func (s *stepper) large(list []any) bool {
	text, _ := json.Marshal(list)
	return len(text) > s.show()
}

// howToSearch tells the model what to do with a list whose names are a
// blob, and howToChoose with one whose names it is shown.
func howToSearch(list string) string {
	return fmt.Sprintf("search this blob with the blob tools, then call open with { list: %q, name: <an entry's name> }", list)
}

func howToChoose(list string) string {
	return fmt.Sprintf("call open with { list: %q, name: <an entry's name> }: a tool is offered from your next turn", list)
}

// stored takes what a job stored of a list. The agent's own toolkits are
// one blob, which the model searches. A toolkit's list is its entries as a
// blob the agent keeps, and their names: shown to the model, or a blob it
// searches. The model opens an entry by its name. A list that could not be
// stored is shown as a short one is.
func (s *stepper) stored(p *preview, event machine.Event, found map[string]any) {
	var c *call
	if p.Call > 0 {
		c = s.st.Calls[p.Call-1]
		c.Large = false
	}
	if event.Name != machine.Done {
		return
	}
	told := map[string]any{"list": p.List}
	if _, searched := blob.AsRef(found["blob"]); searched {
		handle, _ := s.shown(found["blob"]).(map[string]any)
		p.Handle, _ = handle["blob"].(string)
		s.st.Fresh = slices.DeleteFunc(s.st.Fresh, func(name string) bool { return name == p.Handle })
		told["entries"], told["how"] = handle, howToSearch(p.List)
	} else if names, shown := found["names"].([]any); shown && p.List != toolkitsList {
		told["entries"], told["how"] = names, howToChoose(p.List)
	} else {
		return
	}
	if p.List == toolkitsList {
		s.st.Toolkits = p.Handle
	} else {
		entries, kept := blob.AsRef(found["entries"])
		if !kept {
			return
		}
		k := s.st.Opened[p.List]
		k.Stored, k.Blob, k.Entries = entries.Value(), p.Handle, nil
	}
	if c != nil {
		told["instructions"] = c.Result
		c.Result = told
		return
	}
	s.st.Next = append(s.st.Next, map[string]any{"role": "user", "content": told})
}

// previewed takes what a preview job found. One that failed leaves its
// blob with no preview; a result that could not be stored is an error to
// the model, since it cannot be shown.
func (s *stepper) previewed(p *preview, event machine.Event) {
	found, _ := event.Data.(map[string]any)
	if p.List != "" {
		s.stored(p, event, found)
	} else if p.Call > 0 {
		c := s.st.Calls[p.Call-1]
		c.Large = false
		if _, stored := blob.AsRef(found["blob"]); event.Name != machine.Done || !stored {
			c.Result, c.Error = nil, "the result was too large to show, and could not be stored as a blob"
			return
		}
		handle, _ := s.shown(found["blob"]).(map[string]any)
		c.Result = handle
		p.Handle, _ = handle["blob"].(string)
		// This one has its preview already.
		s.st.Fresh = slices.DeleteFunc(s.st.Fresh, func(name string) bool { return name == p.Handle })
	}
	if event.Name != machine.Done {
		return
	}
	if n, ok := found["items"].(float64); ok {
		if s.st.Items == nil {
			s.st.Items = map[string]float64{}
		}
		s.st.Items[p.Handle] = n
	}
	// A preview is made from a blob's contents, which may hold another
	// blob's reference: it is shown as any value is.
	p.Shown = s.shown(found["preview"])
}

// ready starts the next turn once every preview is made. What the turn
// adds to the conversation shows each blob as its handle, and a blob shown
// for the first time with its preview.
func (s *stepper) ready() error {
	for _, p := range s.st.Previews {
		if !p.Done {
			return nil
		}
	}
	for _, o := range s.st.Opening {
		if !o.Done {
			return nil
		}
	}
	s.st.Opening = nil
	if len(s.st.Calls) > 0 {
		var results []any
		for _, c := range s.st.Calls {
			result := map[string]any{"id": c.ID, "name": c.Tool}
			if c.Result != nil {
				result["result"] = c.Result
			}
			if c.Error != "" {
				result["error"] = c.Error
			}
			results = append(results, result)
		}
		s.st.Next = []any{map[string]any{"role": "tool", "results": results}}
	}
	first := map[string]any{}
	for _, p := range s.st.Previews {
		if p.Shown != nil {
			first[p.Handle] = p.Shown
		}
	}
	s.st.Next, _ = s.handles(s.st.Next, first).([]any)
	s.st.Previews = nil
	return s.turn()
}

// handles gives a value with each handle in it saying what the model
// needs to know how to read the blob: how many items it has, roughly
// what reading it would cost, and, the first time, its preview.
func (s *stepper) handles(v any, first map[string]any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v)+3)
		for k, item := range v {
			out[k] = s.handles(item, first)
		}
		name, ok := s.handle(v)
		if !ok {
			return out
		}
		size, _ := v["size"].(float64)
		items, counted := s.st.Items[name]
		switch {
		case counted && items >= 0:
			// Text costs about a token for every four bytes.
			out["items"] = items
			out["cost"] = fmt.Sprintf("about %.0f tokens to read whole", size/4)
			if items > 0 {
				out["cost"] = fmt.Sprintf("about %.0f tokens to read whole, %.0f an item", size/4, max(size/4/items, 1))
			}
		case counted:
			out["cost"] = "it has no items: it is not text, a table or a list"
		}
		if shown, ok := first[name]; ok {
			out["preview"] = shown
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = s.handles(item, first)
		}
		return out
	}
	return v
}

// handle says whether a value is a handle the model was shown, and gives
// its name.
func (s *stepper) handle(v map[string]any) (string, bool) {
	name, ok := v["blob"].(string)
	_, known := s.st.Handles[name]
	_, typed := v["type"]
	_, sized := v["size"]
	return name, ok && known && typed && sized
}

func (s *stepper) turns() int {
	if s.a.Turns > 0 {
		return s.a.Turns
	}
	return DefaultTurns
}

// begin starts the agent: the conversation so far, if it was given one,
// and then its prompt.
func (s *stepper) begin() error {
	s.st.Limit = s.turns()
	scope := machine.Scope{"input": s.in.Input}
	so, err := s.eval.Fill(s.a.Conversation, scope, "conversation")
	if err != nil {
		return err
	}
	carried, _ := so.(map[string]any)
	if so != nil {
		// A conversation carried on brings its turns and the blobs they
		// name, so each name goes on meaning the blob it meant.
		turns, isTurns := carried["turns"].([]any)
		blobs, isBlobs := carried["blobs"].(map[string]any)
		for _, held := range blobs {
			if _, ok := blob.AsRef(held); !ok {
				isBlobs = false
			}
		}
		if !isTurns || !isBlobs {
			return fmt.Errorf("conversation: it should be the conversation of an agent's result, { turns, blobs }")
		}
		s.st.Turns, s.st.Handles = turns, maps.Clone(blobs)
	}
	prompt, err := s.eval.Fill(s.a.Prompt, scope, "prompt")
	if err != nil {
		return err
	}
	if prompt != nil {
		s.st.Next = []any{map[string]any{"role": "user", "content": s.shown(prompt)}}
	}
	if err := s.offer(scope); err != nil {
		return err
	}
	if err := s.readable(); err != nil {
		return err
	}
	// A conversation carried on brings the tasks still working. It is input,
	// so a task is kept only where its entry is a toolkit that is offered.
	if tasks, ok := carried["tasks"].(map[string]any); ok {
		for name, written := range tasks {
			var t *task
			if text, _ := json.Marshal(written); json.Unmarshal(text, &t) != nil || t == nil {
				continue
			}
			if tool, ok := s.a.Tools[t.Entry]; ok && tool.Toolkit && s.offers(t.Entry) {
				if s.st.Tasks == nil {
					s.st.Tasks = map[string]*task{}
				}
				s.st.Tasks[name] = t
			}
		}
	}
	// It brings the toolkits that were open too, each as the model was
	// shown it. One is kept only where a call of open with no hook would
	// open it: its entry is a toolkit that is offered and is not marked
	// approve: true.
	if lists, ok := carried["opened"].(map[string]any); ok {
		for list, written := range lists {
			var k *kit
			if text, _ := json.Marshal(written); json.Unmarshal(text, &k) != nil || k == nil {
				continue
			}
			tool, ok := s.a.Tools[k.Entry]
			_, shown := s.st.Handles[k.Blob]
			_, stored := blob.AsRef(k.Stored)
			if !ok || !tool.Toolkit || tool.Job.Approve || !s.offers(k.Entry) || k.Blob != "" && !shown || k.Stored != nil && !stored {
				continue
			}
			k.Entries = slices.DeleteFunc(k.Entries, func(e *entry) bool { return e == nil })
			if s.st.Opened == nil {
				s.st.Opened = map[string]*kit{}
			}
			s.st.Opened[list] = k
		}
	}
	// The list of the agent's own toolkits is stored where it is too large
	// to show.
	var own []*entry
	for _, name := range sortedNames(s.a.Tools) {
		if tool := s.a.Tools[name]; tool.Toolkit && s.offers(name) {
			own = append(own, &entry{Kind: toolkit.KindToolkit, Name: name, Description: tool.Description})
		}
	}
	if list := s.listed(own); s.large(list) {
		s.look(&preview{List: toolkitsList}, map[string]any{"value": list})
	} else if can := s.toolkitsIn(toolkitsList); len(can) > 0 {
		// The model is told which toolkits it can open before its prompt.
		s.st.Next = append([]any{map[string]any{"role": "user", "content": map[string]any{"list": toolkitsList, "toolkits": can, "how": howToOpen(toolkitsList)}}}, s.st.Next...)
	}
	// The toolkits with open: true are opened once, now; the first turn
	// waits for them as it waits for its previews.
	for _, name := range sortedNames(s.a.Tools) {
		tool := s.a.Tools[name]
		if _, open := s.st.Opened[name]; open || !tool.Toolkit || !tool.Open || !s.offers(name) {
			continue
		}
		o := &opening{Job: openTool, List: name, Entry: name}
		if n := len(s.st.Opening); n > 0 {
			o.Job = fmt.Sprintf("%s.%d", openTool, n)
		}
		job, err := s.ask(o.Entry, o.Job, map[string]any{"do": toolkit.Open}, map[string]any{"toolkit": name, "open": true})
		if err != nil {
			return err
		}
		s.st.Opening = append(s.st.Opening, o)
		s.request(machine.Request{Job: job})
	}
	return s.preview()
}

// offer checks the when: of each entry once, on the agent's input, and
// keeps those that do not hold. A when: is checked once, so that every step
// offers the model the same tools.
func (s *stepper) offer(scope machine.Scope) error {
	for _, name := range sortedNames(s.a.Tools) {
		tool := s.a.Tools[name]
		if tool.When == "" {
			continue
		}
		ok, err := s.eval.Cond(tool.When, scope, place(name)+".when")
		if err != nil {
			return err
		}
		if !ok {
			s.st.Unoffered = append(s.st.Unoffered, name)
		}
	}
	return nil
}

// recorded fills in each entry written call: with a location in full from
// what the run read of the machine there as it started, and gives what is
// wrong with each that cannot be filled in, by the entry's name.
func (s *stepper) recorded() map[string][]string {
	unread := map[string][]string{}
	for name, tool := range s.a.Tools {
		if !strings.HasPrefix(tool.Call, "/") {
			continue
		}
		for _, w := range tool.read(s.in.Run.Recorded[tool.Call]) {
			unread[name] = append(unread[name], place(name)+w.At+": "+w.Text)
		}
	}
	return unread
}

// readable says what is wrong with each entry that is offered and calls a
// location in full whose machine could not be read.
func (s *stepper) readable() error {
	var wrong []string
	for _, name := range sortedNames(s.unread) {
		if s.offers(name) {
			wrong = append(wrong, s.unread[name]...)
		}
	}
	if len(wrong) > 0 {
		return errors.New(strings.Join(wrong, "; "))
	}
	return nil
}

// offers says whether an entry of the definition is offered: its when:
// held when the run started.
func (s *stepper) offers(name string) bool { return !slices.Contains(s.st.Unoffered, name) }

// shownAs gives the name the model is shown for a tool of a list, and the
// name of the list a toolkit in a list makes: the list's name and its own,
// joined by _. Model services take a tool's name in letters, digits, _
// and -.
func shownAs(list, name string) string {
	return list + "_" + strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '_'
	}, name)
}

// ask gives the job that puts a request to the toolkit an entry names: the
// entry's job, whose input is the input the entry fixes and the request.
func (s *stepper) ask(name, jobName string, request, shown map[string]any) (*machine.Job, error) {
	job, err := s.a.Tools[name].Job.Fill(s.eval, machine.Scope{"input": s.in.Input}, place(name), jobName)
	if err != nil {
		return nil, err
	}
	input := map[string]any{}
	fixed, _ := job.Input.(map[string]any)
	maps.Copy(input, fixed)
	input["request"] = request
	job.Input, job.Tool = input, shown
	return job, nil
}

// openable gives the toolkits the model can open, each as the open that
// would open it: the agent's entries that name a toolkit and are offered,
// in the list toolkits, and the toolkits in each open list. One that is
// open is left out.
func (s *stepper) openable() map[string]*opening {
	out := map[string]*opening{}
	for name, tool := range s.a.Tools {
		if _, open := s.st.Opened[name]; tool.Toolkit && !open && s.offers(name) {
			out[name] = &opening{List: name, Entry: name}
		}
	}
	for list, k := range s.st.Opened {
		for _, e := range k.Entries {
			as := shownAs(list, e.Name)
			if _, open := s.st.Opened[as]; e.Kind == toolkit.KindToolkit && !open {
				out[as] = &opening{List: as, Entry: k.Entry, Path: append(slices.Clone(k.Path), e.Name)}
			}
		}
	}
	return out
}

// within gives the list a toolkit is opened from, the name it has there
// and its description.
func (s *stepper) within(o *opening) (list, name, description string) {
	if len(o.Path) == 0 {
		return toolkitsList, o.Entry, s.a.Tools[o.Entry].Description
	}
	name = o.Path[len(o.Path)-1]
	for list, k := range s.st.Opened {
		for _, e := range k.Entries {
			if k.Entry == o.Entry && e.Kind == toolkit.KindToolkit && shownAs(list, e.Name) == o.List {
				return list, name, e.Description
			}
		}
	}
	return "", name, ""
}

// opener gives the tool that opens a toolkit, while there is a toolkit or
// a tool of a long list to open. Its description is the same at every
// turn: the conversation names what can be opened.
func (s *stepper) opener() map[string]any {
	stored := s.st.Toolkits != ""
	for _, k := range s.st.Opened {
		stored = stored || k.Stored != nil
	}
	if len(s.openable()) == 0 && !stored {
		return nil
	}
	text := func() any { return map[string]any{"type": "string"} }
	return map[string]any{"name": openTool, "description": openDescription,
		"args": map[string]any{"type": "object", "properties": map[string]any{"list": text(), "name": text()},
			"required": []any{"list", "name"}, "additionalProperties": false}}
}

// howToOpen tells the model what to do with a list of toolkits it is shown.
func howToOpen(list string) string {
	return fmt.Sprintf("call open with { list: %q, name: <a toolkit's name> }", list)
}

// toolkitsIn gives the toolkits of a list that the model can open, each
// with its name and its description, as the model is shown them.
func (s *stepper) toolkitsIn(list string) []any {
	can := s.openable()
	out := []any{}
	for _, as := range sortedNames(can) {
		if in, name, description := s.within(can[as]); in == list {
			out = append(out, map[string]any{"name": name, "description": description})
		}
	}
	return out
}

// listNamed gives the list a call of open names: by its name, or by the
// name of the blob that holds it.
func (s *stepper) listNamed(args any) (list, name string) {
	given, _ := args.(map[string]any)
	list, _ = given["list"].(string)
	name, _ = given["name"].(string)
	if list != "" && list == s.st.Toolkits {
		return toolkitsList, name
	}
	for named, k := range s.st.Opened {
		if list != "" && list == k.Blob {
			return named, name
		}
	}
	return list, name
}

// chosen gives the open a call of open asks for, or says what is wrong
// with the call.
func (s *stepper) chosen(args any) (*opening, string) {
	list, name := s.listNamed(args)
	can := s.openable()
	for _, as := range sortedNames(can) {
		if in, named, _ := s.within(can[as]); in == list && named == name {
			return can[as], ""
		}
	}
	if _, open := s.st.Opened[list]; !open && list != toolkitsList {
		return nil, fmt.Sprintf("there is no list named %q: a list is %s, or a toolkit that is open", list, toolkitsList)
	}
	return nil, fmt.Sprintf("there is no toolkit named %q to open in the list %q", name, list)
}

// taken says what is wrong with the name a tool would be shown to the model
// under, or nothing.
func (s *stepper) taken(name, shown string) string {
	_, own := s.a.Tools[shown]
	switch {
	case len(shown) > 64:
		return fmt.Sprintf("the tool %q would be shown to the model as %q, which is longer than 64 characters", name, shown)
	case own || shown == openTool || shown == resumeTool || s.has(shown):
		return fmt.Sprintf("the tool %q would be shown to the model as %q, which another tool has", name, shown)
	}
	return ""
}

// asEntry reads one entry of a list, or gives nothing for what is not one.
// A blob among a tool's arguments is the name the model was shown.
func asEntry(item any) *entry {
	written, _ := item.(map[string]any)
	e := &entry{}
	e.Kind, _ = written["kind"].(string)
	e.Name, _ = written["name"].(string)
	e.Description, _ = written["description"].(string)
	e.Args, _ = asArg(written["args"]).(map[string]any)
	if e.Name == "" || e.Kind != toolkit.KindTool && e.Kind != toolkit.KindToolkit {
		return nil
	}
	return e
}

// read takes the entry a job read from a stored list, for a call of open.
// A tool is offered from the next turn. A toolkit is opened as any is, by
// the job of the agent's entry.
func (s *stepper) read(c *call, output any) error {
	f := c.Find
	c.Find = nil
	k := s.st.Opened[f.List]
	e := asEntry(output)
	if e == nil || k == nil {
		c.Error = fmt.Sprintf("there is no entry named %q in the list %q", f.Name, f.List)
		return nil
	}
	shown := shownAs(f.List, e.Name)
	if e.Kind == toolkit.KindTool {
		if c.Error = s.taken(e.Name, shown); c.Error == "" {
			k.Entries = append(k.Entries, e)
			c.Result = "Opened " + shown + ": it is offered from your next turn."
		}
		return nil
	}
	if _, open := s.st.Opened[shown]; open {
		c.Error = fmt.Sprintf("the toolkit %q is open", shown)
		return nil
	}
	o := &opening{List: shown, Entry: k.Entry, Path: append(slices.Clone(k.Path), e.Name)}
	tool := map[string]any{"toolkit": o.Entry, "open": true, "said": f.Said, "path": o.Path}
	job, err := s.ask(o.Entry, c.Job, map[string]any{"do": toolkit.Open, "toolkit": o.Path}, tool)
	if err != nil {
		return err
	}
	c.Open, c.Done = o, false
	s.request(machine.Request{Job: job})
	return nil
}

// take keeps the entries an open gave, as the list the open makes, and
// gives the toolkit's instructions, or says what is wrong with the answer.
// A tool is shown to the model under the list's name and its own.
func (s *stepper) take(o *opening, answer any) (string, error) {
	given, _ := answer.(map[string]any)
	items, ok := given["entries"].([]any)
	if !ok {
		return "", fmt.Errorf("the toolkit gave no answer to an open: it answers { instructions, entries }")
	}
	k := &kit{Entry: o.Entry, Path: o.Path}
	seen := map[string]bool{}
	for _, item := range items {
		e := asEntry(item)
		if e == nil {
			return "", fmt.Errorf("the toolkit gave no answer to an open: an entry is { kind, name, description, args }")
		}
		shown := shownAs(o.List, e.Name)
		if e.Kind == toolkit.KindTool {
			if wrong := s.taken(e.Name, shown); wrong != "" {
				return "", errors.New(wrong)
			}
			if seen[shown] {
				return "", fmt.Errorf("the tool %q would be shown to the model as %q, which another tool has", e.Name, shown)
			}
			seen[shown] = true
		}
		k.Entries = append(k.Entries, e)
	}
	if s.st.Opened == nil {
		s.st.Opened = map[string]*kit{}
	}
	s.st.Opened[o.List] = k
	instructions, _ := given["instructions"].(string)
	return instructions, nil
}

// has says a tool of an open toolkit has a name the model is shown,
// whether or not the model is offered it yet.
func (s *stepper) has(name string) bool {
	for list, k := range s.st.Opened {
		for _, e := range k.Entries {
			if e.Kind == toolkit.KindTool && shownAs(list, e.Name) == name {
				return true
			}
		}
	}
	return false
}

// found is a tool the model knows by a name: an entry of the definition,
// or a tool of an open toolkit, with the list it is in and its own name.
type found struct {
	entry       string
	tool        *Tool
	description string
	args        map[string]any
	kit         *kit
	name        string
}

// find gives the tool the model knows by a name: an entry that is offered
// and is one tool, or a tool of an open toolkit.
func (s *stepper) find(name string) *found {
	if tool, ok := s.a.Tools[name]; ok {
		if tool.Toolkit || !s.offers(name) {
			return nil
		}
		return &found{entry: name, tool: tool, description: tool.Description, args: tool.Args}
	}
	for _, list := range sortedNames(s.st.Opened) {
		k := s.st.Opened[list]
		for _, e := range k.Entries {
			if e.Kind == toolkit.KindTool && shownAs(list, e.Name) == name {
				return &found{entry: k.Entry, tool: s.a.Tools[k.Entry], description: e.Description, args: e.Args, kit: k, name: e.Name}
			}
		}
	}
	return nil
}

// offered gives the tools the model is offered, each with its name, its
// description and what arguments it takes: the entries that are one tool,
// the tools of each open toolkit, and the tool that opens a toolkit while
// one is left to open. The model never sees how a tool is run or what it
// is permitted.
func (s *stepper) offered() []any {
	names := slices.Collect(maps.Keys(s.a.Tools))
	for list, k := range s.st.Opened {
		for _, e := range k.Entries {
			if e.Kind == toolkit.KindTool {
				names = append(names, shownAs(list, e.Name))
			}
		}
	}
	slices.Sort(names)
	var tools []any
	for _, name := range names {
		if f := s.find(name); f != nil {
			tools = append(tools, map[string]any{"name": name, "description": f.description, "args": f.args})
		}
	}
	if opener := s.opener(); opener != nil {
		tools = append(tools, opener)
	}
	if len(s.st.Tasks) > 0 {
		tools = append(tools, map[string]any{"name": resumeTool, "description": resumeDescription,
			"args": map[string]any{"type": "object", "properties": map[string]any{"task": map[string]any{"type": "string"},
				"responses": map[string]any{"type": "object"}, "cancel": map[string]any{"type": "boolean"}},
				"required": []any{"task"}, "additionalProperties": false}})
	}
	return tools
}

// turn asks for the next turn: one call to the model, made by a job. At
// the limit the agent fails; its error carries the conversation, which
// another agent can carry on.
func (s *stepper) turn() error {
	if s.st.Turn >= s.st.Limit {
		text := fmt.Sprintf("turn limit reached: the agent used its %d turns", s.st.Limit)
		if s.st.Wrong != "" {
			text += "; its last reply did not fit: " + s.st.Wrong
		}
		s.end(&machine.Error{Status: machine.StatusError, Error: text})
		return nil
	}
	scope := machine.Scope{"input": s.in.Input}
	system, err := s.eval.Fill(s.a.System, scope, "system")
	if err != nil {
		return err
	}
	job, err := s.a.Turn.Fill(s.eval, scope, "turn", turnJob)
	if err != nil {
		return err
	}
	tools := s.offered()
	own := make([]any, len(s.a.Model.Own))
	for i, tool := range s.a.Model.Own {
		own[i] = tool
	}
	left := s.st.Limit - s.st.Turn
	add := s.st.Next
	if left <= 3 {
		add = append(add, map[string]any{"role": "user", "content": fmt.Sprintf("(%d turns remain: finish with what you have)", left)})
	}
	given := map[string]any{
		"model":  map[string]any{"name": s.a.Model.Name, "service": s.a.Model.serviceOf(), "address": s.a.Model.addressOf(), "secret": s.a.Model.Secret, "effort": s.a.Model.Effort, "reply_tokens": float64(s.a.Model.ReplyTokens), "own": own, "tool_changes": s.a.Model.ToolChanges},
		"system": system, "tools": tools, "turns": s.st.Turns, "add": add, "shape": s.a.Reply,
		// The blobs the model has been shown, by name: those its service
		// takes as content are sent to it as they are.
		"blobs": s.st.Handles,
	}
	if own, ok := job.Input.(map[string]any); ok {
		for k, v := range own {
			given[k] = v
		}
	}
	job.Input = given
	s.st.Turn++
	s.st.Waiting, s.st.Next, s.st.Calls = turnJob, nil, nil
	s.request(machine.Request{Job: job})
	return nil
}

// replied takes a turn's reply: the tools the model picked are run, all at
// once; or, with none, its reply is the agent's result.
func (s *stepper) replied(data any) error {
	reply, _ := data.(map[string]any)
	ref, ok := blob.AsRef(reply["turn"])
	if !ok {
		// The conversation is kept as its turns: a turn that gives back no
		// part of it would lose it without a word.
		s.end(&machine.Error{Status: machine.StatusError,
			Error: "turn: the script that made the turn gave back no turn: it returns { turn, text, calls, stop, usage }, where turn is a blob of this turn's part of the conversation"})
		return nil
	}
	s.st.Turns = append(s.st.Turns, ref.Value())
	if usage, ok := reply["usage"].(map[string]any); ok {
		if s.st.Tokens == nil {
			s.st.Tokens = map[string]float64{}
		}
		for _, name := range []string{"input", "output", "cached"} {
			n, _ := usage[name].(float64)
			s.st.Tokens[name] += n
		}
	}
	text, _ := reply["text"].(string)
	calls, _ := reply["calls"].([]any)
	// A reply cut short in the middle of asking for a tool runs no tool.
	if reply["stop"] == "length" {
		s.st.Wrong = "it was cut short by the model's length limit"
		s.st.Next = []any{map[string]any{"role": "user", "content": "(your reply was cut short by the length limit; give it again, shorter)"}}
		return s.turn()
	}
	if len(calls) == 0 {
		return s.finish(text)
	}
	s.st.Waiting = "tools"
	scope := machine.Scope{"input": s.in.Input}
	seen := map[string]int{}
	resumed := map[string]bool{}
	for _, c := range calls {
		made, _ := c.(map[string]any)
		one := &call{Args: made["args"]}
		one.ID, _ = made["id"].(string)
		one.Tool, _ = made["name"].(string)
		s.st.Calls = append(s.st.Calls, one)
		// Each call's job is named after its tool; a tool called more than
		// once in a turn adds its place.
		named := func() string {
			seen[one.Tool]++
			if seen[one.Tool] > 1 {
				return fmt.Sprintf("%s.%d", one.Tool, seen[one.Tool]-1)
			}
			return one.Tool
		}
		// A hook reads the call as job.tool: the toolkit's entry, the tool's
		// own name, the arguments and what the model said.
		through := func(o *kit, shown map[string]any) map[string]any {
			shown["toolkit"], shown["said"] = o.Entry, text
			if len(o.Path) > 0 {
				shown["path"] = o.Path
			}
			return shown
		}
		if list, name := s.listNamed(one.Args); one.Tool == openTool && s.st.Opened[list] != nil && s.st.Opened[list].Stored != nil {
			// An entry of a list that is stored is read from it, by a job.
			k := s.st.Opened[list]
			if slices.ContainsFunc(k.Entries, func(e *entry) bool { return e.Name == name }) {
				one.Done, one.Result = true, "Opened "+shownAs(list, name)+": it is offered from your next turn."
				continue
			}
			one.Job, one.Find = named(), &finding{List: list, Name: name, Said: text}
			s.request(machine.Request{Job: &machine.Job{Name: one.Job, Script: entryScript, Alone: true,
				Input: map[string]any{"entries": k.Stored, "name": name}}})
			continue
		}
		if one.Tool == openTool {
			o, wrong := s.chosen(one.Args)
			if o == nil {
				one.Done, one.Error = true, wrong
				continue
			}
			request := map[string]any{"do": toolkit.Open}
			if len(o.Path) > 0 {
				request["toolkit"] = o.Path
			}
			one.Job, one.Open = named(), o
			job, err := s.ask(o.Entry, one.Job, request, through(&kit{Entry: o.Entry, Path: o.Path}, map[string]any{"open": true}))
			if err != nil {
				return err
			}
			s.request(machine.Request{Job: job})
			continue
		}
		if one.Tool == resumeTool {
			given, _ := one.Args.(map[string]any)
			name, _ := given["task"].(string)
			t := s.st.Tasks[name]
			switch {
			case t == nil:
				one.Done, one.Error = true, fmt.Sprintf("there is no task named %q", name)
				continue
			case resumed[name]:
				one.Done, one.Error = true, fmt.Sprintf("another call of this turn resumes the task %q", name)
				continue
			}
			resumed[name] = true
			// A resume is the call carried on: its job is the entry's, with
			// the call's tool and arguments, so a hook that held the call
			// holds the resume.
			args := cmp.Or(t.Sent, t.Args)
			if args == nil {
				args = map[string]any{}
			}
			request := map[string]any{"do": toolkit.Resume, "tool": t.Tool, "args": args, "state": t.State}
			shown := map[string]any{"name": t.Tool, "args": t.Args, "resume": name}
			if t.Requests != nil {
				shown["requests"] = t.Requests
			}
			if cancel, _ := given["cancel"].(bool); cancel {
				request["do"], shown["cancel"] = toolkit.Cancel, true
			} else if responses, given := given["responses"]; given {
				request["responses"], shown["responses"] = responses, responses
			}
			if len(t.Path) > 0 {
				request["toolkit"] = t.Path
			}
			one.Job, one.Task = named(), name
			job, err := s.ask(t.Entry, one.Job, request, through(&kit{Entry: t.Entry, Path: t.Path}, shown))
			if err != nil {
				return err
			}
			s.request(machine.Request{Job: job})
			continue
		}
		f := s.find(one.Tool)
		if f == nil {
			one.Done, one.Error = true, fmt.Sprintf("there is no tool named %q", one.Tool)
			continue
		}
		if f.kit != nil {
			// The toolkit checks the arguments, which go to it as the model
			// gave them, with each blob's name turned into the blob.
			given := one.Args
			if given == nil {
				given = map[string]any{}
			}
			args, err := s.blobs(f.args, given)
			if err != nil {
				one.Done, one.Error = true, "the arguments do not fit the tool: "+err.Error()
				continue
			}
			if !reflect.DeepEqual(args, given) {
				one.Sent = args
			}
			request := map[string]any{"do": toolkit.Call, "tool": f.name, "args": args}
			if len(f.kit.Path) > 0 {
				request["toolkit"] = f.kit.Path
			}
			one.Job = named()
			job, err := s.ask(f.entry, one.Job, request, through(f.kit, map[string]any{"name": f.name, "args": given}))
			if err != nil {
				return err
			}
			s.request(machine.Request{Job: job})
			continue
		}
		args, err := s.fit(f.args, one.Args)
		if err != nil {
			one.Done, one.Error = true, "the arguments do not fit the tool: "+err.Error()
			continue
		}
		one.Job = named()
		scope["args"] = args
		at := place(one.Tool)
		if !f.tool.named() {
			at += ".job"
		}
		job, err := f.tool.Job.Fill(s.eval, scope, at, one.Job)
		if err != nil {
			return err
		}
		// An entry that names a machine gives it the model's arguments as
		// they were given, with the input the entry fixes over them.
		if f.tool.named() {
			input := map[string]any{}
			maps.Copy(input, args)
			fixed, _ := job.Input.(map[string]any)
			maps.Copy(input, fixed)
			job.Input = input
		}
		job.Tool = map[string]any{"name": one.Tool, "args": one.Args, "said": text}
		s.request(machine.Request{Job: job})
	}
	return s.settled()
}

// settled goes on once every call of this turn has settled: to the
// previews of what the results hold, and then to the next turn, where the
// results are shown to the model in the order the calls were made.
func (s *stepper) settled() error {
	for _, c := range s.st.Calls {
		if !c.Done {
			return nil
		}
	}
	return s.preview()
}

// finish ends the agent with the model's last reply: as text, or as data
// in the shape the definition gives. A reply that does not fit is told to
// the model, which takes another turn.
func (s *stepper) finish(text string) error {
	var reply any = text
	if s.a.Reply != nil {
		data, err := shaped(text)
		if err == nil {
			_, err = s.fit(s.a.Reply, data)
		}
		if err != nil {
			s.st.Wrong = err.Error()
			s.st.Next = []any{map[string]any{"role": "user", "content": fmt.Sprintf(
				"(your reply does not fit what is wanted: %v. Reply with JSON only, fitting this JSON Schema: %s)", err, shapeText(s.a.Reply))}}
			return s.turn()
		}
		reply = data
	}
	s.st.Waiting = ""
	s.request(machine.Request{End: &machine.Ending{Output: s.result(reply)}})
	return nil
}

// result is what an agent's result, or its error, always carries: the
// conversation, as its turns and the blobs they name, and a count of the tokens its own turns used.
func (s *stepper) result(reply any) map[string]any {
	turns, blobs := s.st.Turns, s.st.Handles
	if turns == nil {
		turns = []any{}
	}
	if blobs == nil {
		blobs = map[string]any{}
	}
	conversation := map[string]any{"turns": turns, "blobs": blobs}
	if len(s.st.Tasks) > 0 {
		conversation["tasks"] = s.st.Tasks
	}
	if len(s.st.Opened) > 0 {
		conversation["opened"] = s.st.Opened
	}
	out := map[string]any{"conversation": conversation,
		"tokens": map[string]any{"input": s.st.Tokens["input"], "output": s.st.Tokens["output"], "cached": s.st.Tokens["cached"]}}
	if reply != nil {
		out["reply"] = s.real(reply)
	}
	return out
}

func (s *stepper) end(e *machine.Error) {
	e.Output = s.result(nil)
	s.st.Waiting = ""
	s.request(machine.Request{End: &machine.Ending{Error: e}})
}

// fail fails the agent with the failure of one of its own jobs.
func (s *stepper) fail(event machine.Event, what string) {
	s.end(machine.Unhandled(what, event.Data))
}

// shaped reads a reply that should be data: JSON, which may be wrapped in
// one fenced block. Data followed by anything else does not fit.
func shaped(text string) (any, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if end := strings.LastIndex(text, "```"); end > 3 && strings.TrimSpace(text[end+3:]) == "" {
			text = text[3:end]
			if nl := strings.IndexByte(text, '\n'); nl >= 0 && !strings.ContainsAny(text[:nl], "{[\"") {
				text = text[nl+1:]
			}
		}
	}
	var data any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &data); err != nil {
		return nil, fmt.Errorf("it is not JSON alone")
	}
	return data, nil
}

func shapeText(shape map[string]any) string {
	text, _ := json.Marshal(shape, json.Deterministic(true))
	return string(text)
}

// fit checks data against a shape and gives it with each handle's name,
// where the shape marks a blob, turned into the blob's reference. A name
// is turned into a reference nowhere else: ordinary text from a model may
// happen to contain one.
func (s *stepper) fit(shape map[string]any, data any) (map[string]any, error) {
	if data == nil {
		data = map[string]any{}
	}
	schema, err := machine.CompileShape(shape)
	if err != nil {
		return nil, err
	}
	if err := machine.Fits(schema, data); err != nil {
		return nil, err
	}
	out, err := s.blobs(shape, data)
	if err != nil {
		return nil, err
	}
	given, _ := out.(map[string]any)
	return given, nil
}

// shown gives a value as the model is shown it: each blob is a handle, a
// short name with its type and size, and no key reaches the model. A
// reference written out inside a text is shown the same way.
func (s *stepper) shown(v any) any {
	shown, _ := blob.Shown(v, func(_ string, ref blob.Ref) (any, error) {
		name := ""
		for existing, held := range s.st.Handles {
			if r, _ := blob.AsRef(held); r.At == ref.At {
				name = existing
			}
		}
		if name == "" {
			// A name is never one the conversation has used.
			for n := len(s.st.Handles) + 1; ; n++ {
				if _, used := s.st.Handles[fmt.Sprintf("blob%d", n)]; !used {
					name = fmt.Sprintf("blob%d", n)
					break
				}
			}
			if s.st.Handles == nil {
				s.st.Handles = map[string]any{}
			}
			s.st.Handles[name] = ref.Value()
			s.st.Fresh = append(s.st.Fresh, name)
		}
		return map[string]any{"blob": name, "type": ref.Type, "size": float64(ref.Size)}, nil
	})
	return shown
}

// real gives a value with each handle in it turned back into the blob's
// reference, for the agent's own result.
func (s *stepper) real(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if name, ok := s.handle(v); ok {
			return s.st.Handles[name]
		}
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = s.real(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = s.real(item)
		}
		return out
	}
	return v
}
