package agent

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/machine"
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
}

// preview is one job that looks at something before the model is shown it:
// a blob it has not seen, or a tool's result too large to show, which the
// job stores as a blob.
type preview struct {
	Job    string `json:"job"`
	Handle string `json:"handle,omitzero"` // the blob it is of
	Call   int    `json:"call,omitzero"`   // or the call whose result it stores, counted from one
	Done   bool   `json:"done,omitzero"`
	Shown  any    `json:"shown,omitzero"` // the first few items
}

// showLimit is the most of a tool's result, as JSON, that is shown to the
// model in place. A larger one is stored as a blob.
const showLimit = 32 << 10

// call is one call the model made in a turn.
type call struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	Job  string `json:"job,omitzero"` // the job that carries it out; none if it ran nothing
	Args any    `json:"args,omitzero"`
	Done bool   `json:"done,omitzero"`
	// What goes back to the model: a result, an error or a denial.
	Result any    `json:"result,omitzero"`
	Error  string `json:"error,omitzero"`
	// Large says the result is too large to show, and is to be stored.
	Large bool `json:"large,omitzero"`
}

// stepper is one step being taken.
type stepper struct {
	a    *Agent
	in   *machine.StepIn
	st   *state
	out  *machine.StepOut
	eval *machine.Evaluator
}

func (agentType) Step(in *machine.StepIn) (*machine.StepOut, error) {
	var a Agent
	if err := json.Unmarshal(in.Definition.Body, &a); err != nil {
		return nil, err
	}
	s := &stepper{a: &a, in: in, st: &state{}, out: &machine.StepOut{}, eval: machine.NewEvaluator(in.Time, in.Seed)}
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
// tools and its ask, and on nothing else.
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
			if event.Name == machine.Done {
				c.Result = s.shown(s.said(c, event.Data))
				text, _ := json.Marshal(c.Result)
				c.Large = len(text) > showLimit
			} else {
				// A tool that fails goes back to the model as an error,
				// with whatever output it still gave.
				data, _ := event.Data.(map[string]any)
				c.Error = fmt.Sprint(data["error"])
				if data["status"] == machine.StatusDenied {
					c.Error = "denied: " + c.Error
				}
				c.Result = s.shown(s.said(c, data["output"]))
			}
			return true, s.settled()
		}
	case s.st.Waiting == previewJob && (event.Name == machine.Done || event.Name == machine.Failed):
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

// said gives what the model is shown of a tool's output. A tool that is an
// agent is shown as what that agent replied: its conversation and its count
// of tokens are in the record, and are not the caller's to read through.
func (s *stepper) said(c *call, output any) any {
	if tool := s.a.Tools[c.Tool]; tool == nil || !tool.Agent {
		return output
	}
	result, _ := output.(map[string]any)
	return result["reply"]
}

// preview asks for a look at what the model is about to be shown for the
// first time: each blob it has not seen, and each result too large to
// show. The next turn waits for them as it waits for tools.
func (s *stepper) preview() error {
	ask := func(p *preview, input map[string]any) {
		p.Job = previewJob
		if n := len(s.st.Previews); n > 0 {
			p.Job = fmt.Sprintf("%s.%d", previewJob, n)
		}
		s.st.Previews = append(s.st.Previews, p)
		s.request(machine.Request{Job: &machine.Job{Name: p.Job, Script: previewScript, Alone: true, Input: input}})
	}
	for _, name := range s.st.Fresh {
		ask(&preview{Handle: name}, map[string]any{"blob": s.st.Handles[name]})
	}
	s.st.Fresh = nil
	for i, c := range s.st.Calls {
		if c.Large {
			ask(&preview{Call: i + 1}, map[string]any{"value": c.Result})
		}
	}
	s.st.Waiting = previewJob
	return s.ready()
}

// previewed takes what a preview job found. One that failed leaves its
// blob with no preview; a result that could not be stored is an error to
// the model, since it cannot be shown.
func (s *stepper) previewed(p *preview, event machine.Event) {
	found, _ := event.Data.(map[string]any)
	if p.Call > 0 {
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
	if so != nil {
		// A conversation carried on brings its turns and the blobs they
		// name, so each name goes on meaning the blob it meant.
		carried, _ := so.(map[string]any)
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
	return s.preview()
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
	var tools []any
	for _, name := range sortedNames(s.a.Tools) {
		// The model sees a tool's name, what it does and what arguments it
		// takes; never how it is run or what it is permitted.
		tools = append(tools, map[string]any{"name": name, "does": s.a.Tools[name].Does, "args": s.a.Tools[name].Args})
	}
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
		"model":  map[string]any{"name": s.a.Model.Name, "service": s.a.Model.serviceOf(), "address": s.a.Model.addressOf(), "secret": s.a.Model.Secret, "effort": s.a.Model.Effort, "reply_tokens": float64(s.a.Model.ReplyTokens), "own": own},
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
		for _, name := range []string{"input", "output"} {
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
	for _, c := range calls {
		made, _ := c.(map[string]any)
		one := &call{Args: made["args"]}
		one.ID, _ = made["id"].(string)
		one.Tool, _ = made["name"].(string)
		s.st.Calls = append(s.st.Calls, one)
		tool, ok := s.a.Tools[one.Tool]
		if !ok {
			one.Done, one.Error = true, fmt.Sprintf("there is no tool named %q", one.Tool)
			continue
		}
		args, err := s.fit(tool.Args, one.Args)
		if err != nil {
			one.Done, one.Error = true, "the arguments do not fit the tool: "+err.Error()
			continue
		}
		// Each call's job is named after its tool; a tool called more than
		// once in a turn adds its place.
		seen[one.Tool]++
		one.Job = one.Tool
		if seen[one.Tool] > 1 {
			one.Job = fmt.Sprintf("%s.%d", one.Tool, seen[one.Tool]-1)
		}
		scope["args"] = args
		job, err := tool.Job.Fill(s.eval, scope, "tools."+one.Tool+".job", one.Job)
		if err != nil {
			return err
		}
		// The ask is shown the arguments, what the tool does and what the
		// model said.
		job.Shown = map[string]any{"does": tool.Does, "args": one.Args, "said": text}
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
// conversation, as its turns and the blobs they name, and a count of the
// tokens its own turns used.
func (s *stepper) result(reply any) map[string]any {
	turns, blobs := s.st.Turns, s.st.Handles
	if turns == nil {
		turns = []any{}
	}
	if blobs == nil {
		blobs = map[string]any{}
	}
	out := map[string]any{"conversation": map[string]any{"turns": turns, "blobs": blobs},
		"tokens": map[string]any{"input": s.st.Tokens["input"], "output": s.st.Tokens["output"]}}
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
	schema, err := compiled(shape)
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(data); err != nil {
		return nil, wrong(err)
	}
	out, err := s.blobs(shape, shape, data, 0)
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
