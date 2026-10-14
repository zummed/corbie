// Package flow is the machine whose stages run once each, in an order that
// follows from the data they pass.
package flow

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
)

// Flow is the full form of a flow's own part of a definition.
type Flow struct {
	Stages map[string]*Stage `json:"stages"`
	Output any               `json:"output,omitzero"`
	// Emit names the stage whose emitted items the flow emits to its own
	// parent as they arrive.
	Emit string `json:"emit,omitzero"`
}

// Stage is one stage in full form.
type Stage struct {
	Job     *machine.JobSpec `json:"job"`
	When    string           `json:"when,omitzero"`
	Follows []string         `json:"follows,omitzero"`
	// Each makes the stage run once for every item: a list, or an
	// expression that gives one.
	Each any `json:"each,omitzero"`
	// Emits names another stage, whose emitted items are this stage's
	// items, each started as it arrives.
	Emits string `json:"emits,omitzero"`
	// Blob is a blob whose items are this stage's: a job of the stage's own
	// name reads it and emits them, and each starts as it arrives.
	Blob any `json:"blob,omitzero"`
	// Serial has the items go one at a time, in order.
	Serial bool `json:"serial,omitzero"`
}

func (s *Stage) perItem() bool { return s.Each != nil || s.Emits != "" || s.Blob != nil }

type flowType struct{}

func init() { machine.Register(flowType{}) }

func (flowType) Name() string { return "flow" }

// itemsScript is the script of the job that reads a stage's blob and emits
// its items.
const (
	itemsScript = "corbie:items.js"
	itemsSource = `let n = 0
for (const item of blobs.items(input.blob)) {
  emit("item", item)
  n++
}
return { count: n }
`
)

// shape is what a flow's definition holds of its own.
var shape = &machine.Shape{Name: "flow", What: "a flow", Fields: []machine.Field{
	{Name: "stages", Kind: machine.KindNamed, Does: "its stages, each run once, in the order that follows from what they read of each other",
		Of: &machine.Shape{Name: "stage", What: "a stage", Fields: []machine.Field{
			{Name: "when", Kind: machine.KindWord, Like: "${stages.summarise.failed}", Does: "a condition, one expression: the stage runs only if it holds"},
			{Name: "follows", Kind: machine.KindList, Like: "[fetch]", Does: "stages it runs after, though it reads nothing of them"},
			{Name: "job", Kind: machine.KindJob, Like: "{ script: ./fetch.js }", Does: "what the stage does: a script, or a machine"},
			{Name: "each", Kind: machine.KindValue, Like: "${input.urls}",
				Does: "runs the stage once for every item, read as ${item}: { items: … }, { emitted: … } or { blob: … }, or a list or an expression that gives one",
				Of: &machine.Shape{Name: "each", What: "where a stage's items come from", Fields: []machine.Field{
					{Name: "items", Kind: machine.KindValue, Like: "${input.urls}", Does: "a list, or an expression that gives one"},
					{Name: "emitted", Kind: machine.KindWord, Like: "fetch", Does: "a stage whose emitted items the stage follows, each as it arrives"},
					{Name: "blob", Kind: machine.KindValue, Like: "${input.file}", Does: "the blob whose items the stage follows: lines, rows, entries"},
					{Name: "serial", Kind: machine.KindFlag, Like: "true", Does: "the items go one after another, in order"},
				}}},
		}}},
	{Name: "output", Kind: machine.KindValue, Like: "${stages.summarise.output}", Does: "the flow's result"},
	{Name: "emit", Kind: machine.KindWord, Like: "fetch", Does: "a stage whose emitted items the flow emits to its own parent as they arrive"},
}}

func (flowType) Shape() *machine.Shape { return shape }

// Fill reads a flow as written and gives its full form.
func (flowType) Fill(p *machine.Parser, f *machine.Fields) any {
	flow := &Flow{Stages: map[string]*Stage{}}
	stages, order := f.Map("stages")
	for _, name := range order {
		place := "stages." + name
		sf := p.Fields(place, stages[name], f.Of("stages"))
		if sf == nil {
			// What is wrong with it has been said. It is still a stage: the
			// flow is not also said to have none.
			flow.Stages[name] = &Stage{}
			continue
		}
		stage := &Stage{When: sf.Cond("when"), Follows: sf.Strings("follows")}
		if job, ok := sf.Value("job"); ok {
			stage.Job = p.Job(place+".job", job)
		}
		if each, ok := sf.Value("each"); ok {
			switch v := each.(type) {
			case string:
				// Short for { items: … }.
				if machine.HasExpr(v) {
					stage.Each = v
				} else {
					p.Problem(place+".each", "%q is not a list: if %q is a stage whose emitted values this stage follows, write \"each: { emitted: %s }\"", v, v, v)
				}
			case []any:
				stage.Each = v
			case map[string]any:
				bf := p.Fields(place+".each", v, sf.Of("each"))
				items, isItems := bf.Value("items")
				emitted := bf.Text("emitted")
				from, isBlob := bf.Value("blob")
				stage.Serial = bf.Bool("serial")
				bf.Done()
				sources := 0
				for _, given := range []bool{isItems, emitted != "", isBlob} {
					if given {
						sources++
					}
				}
				switch {
				case sources != 1:
					p.Problem(place+".each", "each: { … } holds one of items:, emitted: and blob:")
				case isItems:
					if text, ok := items.(string); ok && !machine.HasExpr(text) {
						p.Problem(place+".each.items", "%q is not a list: if %q is a stage whose emitted values this stage follows, write \"each: { emitted: %s }\"", text, text, text)
					}
					stage.Each = items
				case emitted != "":
					stage.Emits = emitted
				default:
					p.Builtin(itemsScript, itemsSource)
					stage.Blob = from
				}
			default:
				p.Problem(place+".each", "each takes { items: … }, { emitted: … } or { blob: … }, or a list or an expression that gives one")
			}
		}
		sf.Done()
		flow.Stages[name] = stage
	}
	flow.Output, _ = f.Value("output")
	flow.Emit = f.Text("emit")
	return flow
}

// The parts of a stage an expression may read.
var parts = []string{"output", "failed", "error", "skipped", "emitted"}

// needs is what one stage needs of the others, found from its data.
type needs struct {
	// after are the stages it runs after: those it reads, those it follows
	// and the one whose emits it follows.
	after []string
	// failure are the stages whose failure its condition reads: the
	// failures it handles.
	failure []string
	// skipped are the stages its own condition asks about being skipped.
	skipped []string
	// output are the stages whose output it reads.
	output []string
}

// reads gathers what a stage's expressions read of the other stages.
func (s *Stage) reads() (all []machine.Read, when []machine.Read) {
	if s.Job != nil {
		all = append(all, machine.ReadsIn(s.Job.Input, "stages")...)
		all = append(all, machine.Reads(s.Job.Machine, "stages")...)
		all = append(all, machine.Reads(s.Job.Script, "stages")...)
	}
	all = append(all, machine.ReadsIn(s.Each, "stages")...)
	all = append(all, machine.ReadsIn(s.Blob, "stages")...)
	if s.When != "" {
		when = machine.ReadsOf(s.When, "stages")
		all = append(all, when...)
	}
	return all, when
}

func (s *Stage) needs() needs {
	var n needs
	add := func(list *[]string, name string) {
		if !slices.Contains(*list, name) {
			*list = append(*list, name)
		}
	}
	all, when := s.reads()
	for _, r := range all {
		add(&n.after, r.Name)
		switch r.Part {
		case "failed", "error", "skipped":
		default:
			add(&n.output, r.Name)
		}
	}
	for _, r := range when {
		switch r.Part {
		case "failed", "error":
			add(&n.failure, r.Name)
		case "skipped":
			add(&n.skipped, r.Name)
		}
	}
	for _, name := range s.Follows {
		add(&n.after, name)
		add(&n.output, name)
	}
	if s.Emits != "" {
		add(&n.after, s.Emits)
	}
	return n
}

// Check reports what is wrong with a flow in full form.
func (flowType) Check(c *machine.Checker, def *machine.Definition) {
	var flow Flow
	if !c.DecodeBody(def, &flow) {
		return
	}
	if len(flow.Stages) == 0 {
		c.Problem("stages", "a flow needs at least one stage")
		return
	}
	names := sortedNames(flow.Stages)
	known := func(place, name string) {
		if name == "*" {
			c.Problem(place, "a stage is read by its name, written out; this reads one chosen by a value")
		} else if _, ok := flow.Stages[name]; !ok {
			c.Problem(place, "there is no stage named %q", name)
		}
	}
	after := map[string][]string{}
	for _, name := range names {
		stage, place := flow.Stages[name], "stages."+name
		if stage == nil {
			c.Problem(place, "the stage is empty")
			continue
		}
		c.Name(place, name)
		scope := []string{"input", "stages"}
		if stage.perItem() {
			scope = append(scope, "item")
		}
		if stage.Job == nil {
			c.Problem(place, "a stage needs a job")
		}
		c.Job(place+".job", stage.Job, scope...)
		if stage.When != "" {
			c.Cond(place+".when", stage.When, "input", "stages")
		}
		if stage.Each != nil && stage.Emits != "" {
			c.Problem(place+".each", "a stage's items are a list or what another stage emits, not both")
		}
		c.Value(place+".each", stage.Each, "input", "stages")
		c.Value(place+".each.blob", stage.Blob, "input", "stages")
		if stage.Emits != "" {
			known(place+".each", stage.Emits)
			if stage.Emits == name {
				c.Problem(place+".each", "a stage cannot follow its own emits")
			}
		}
		for i, other := range stage.Follows {
			known(fmt.Sprintf("%s.follows[%d]", place, i), other)
		}
		all, _ := stage.reads()
		for _, r := range all {
			known(place, r.Name)
			if r.Part != "" && !slices.Contains(parts, r.Part) {
				c.Problem(place, "\"stages.%s.%s\" reads a part a stage does not have: a stage has output, failed, error, skipped and emitted", r.Name, r.Part)
			}
		}
		after[name] = stage.needs().after
	}
	c.Value("output", flow.Output, "input", "stages")
	for _, r := range machine.ReadsIn(flow.Output, "stages") {
		known("output", r.Name)
	}
	if flow.Emit != "" {
		known("emit", flow.Emit)
	}
	// A flow with a cycle is refused.
	state := map[string]int{}
	var trail []string
	var visit func(name string) bool
	visit = func(name string) bool {
		switch state[name] {
		case 1:
			at := slices.Index(trail, name)
			circle := []string{}
			for _, stage := range append(slices.Clone(trail[at:]), name) {
				circle = append(circle, strconv.Quote(stage))
			}
			c.Problem("stages", "the stages wait on each other in a circle: %s", strings.Join(circle, " -> "))
			return true
		case 2:
			return false
		}
		state[name] = 1
		trail = append(trail, name)
		for _, next := range after[name] {
			if _, ok := flow.Stages[next]; ok && visit(next) {
				return true
			}
		}
		trail = trail[:len(trail)-1]
		state[name] = 2
		return false
	}
	for _, name := range names {
		if visit(name) {
			break
		}
	}
}

func sortedNames[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Where a stage stands.
const (
	pending = ""
	running = "running"
	done    = "done"
	failed  = "failed"
	skipped = "skipped"
)

// state is everything a flow remembers between steps.
type state struct {
	Stages map[string]*stageState `json:"stages"`
}

// stageState is where one stage stands and what came of it.
type stageState struct {
	Status string `json:"status,omitzero"`
	Output any    `json:"output,omitzero"`
	Error  any    `json:"error,omitzero"` // the error's text
	Denial any    `json:"denial,omitzero"`
	// Emitted are the items the stage's job has sent up.
	Emitted []any `json:"emitted,omitzero"`
	// Items are the items of a stage that runs once for each.
	Items []*itemState `json:"items,omitzero"`
	// Listed says the stage's list of items has been worked out.
	Listed bool `json:"listed,omitzero"`
	// Reading is where the job that reads a stage's blob stands.
	Reading string `json:"reading,omitzero"`
}

type itemState struct {
	Item   any    `json:"item"`
	Status string `json:"status,omitzero"`
	Output any    `json:"output,omitzero"`
	Error  any    `json:"error,omitzero"`
	Denial any    `json:"denial,omitzero"`
}

func (s *stageState) settled() bool {
	return s.Status == done || s.Status == failed || s.Status == skipped
}

// stepper is one step being taken.
type stepper struct {
	flow *Flow
	in   *machine.StepIn
	st   *state
	out  *machine.StepOut
	eval *machine.Evaluator
	need map[string]needs
}

func (flowType) Step(in *machine.StepIn) (*machine.StepOut, error) {
	var flow Flow
	if err := json.Unmarshal(in.Definition.Body, &flow); err != nil {
		return nil, err
	}
	s := &stepper{flow: &flow, in: in, st: &state{Stages: map[string]*stageState{}}, out: &machine.StepOut{},
		eval: machine.NewEvaluator(in.Time, in.Seed), need: map[string]needs{}}
	if in.State != nil {
		if err := json.Unmarshal(in.State, s.st); err != nil {
			return nil, err
		}
	}
	for name, stage := range flow.Stages {
		if s.st.Stages[name] == nil {
			s.st.Stages[name] = &stageState{}
		}
		s.need[name] = stage.needs()
	}
	for i, event := range in.Events {
		if !s.arrive(event) {
			s.out.Ignored = append(s.out.Ignored, i)
		}
	}
	if err := s.advance(); err != nil {
		return nil, err
	}
	s.out.State = s.st
	return s.out, nil
}

func (s *stepper) request(r machine.Request) { s.out.Requests = append(s.out.Requests, r) }

// arrive takes one event: the result of a stage's job or of one item's, or
// a value a job sent up. A flow acts on nothing else.
func (s *stepper) arrive(event machine.Event) bool {
	if event.Name == machine.Start {
		return true
	}
	name, index, isItem := strings.Cut(event.From, ".")
	stage, ok := s.st.Stages[name]
	if !ok || event.From == machine.Parent || event.From == "client" {
		return false
	}
	if !isItem && s.flow.Stages[name].Blob != nil {
		return s.read(name, stage, event)
	}
	var item *itemState
	if isItem {
		var i int
		if _, err := fmt.Sscanf(index, "%d", &i); err != nil || i < 0 || i >= len(stage.Items) {
			return false
		}
		item = stage.Items[i]
	}
	switch event.Name {
	case machine.Done:
		if item != nil {
			item.Status, item.Output = done, event.Data
		} else {
			stage.Status, stage.Output = done, event.Data
		}
	case machine.Failed:
		data, _ := event.Data.(map[string]any)
		if item != nil {
			item.Status, item.Error = failed, data["error"]
			if data["status"] == machine.StatusDenied {
				item.Denial = data
			}
		} else {
			stage.Status, stage.Error, stage.Output = failed, data["error"], data["output"]
			if data["status"] == machine.StatusDenied {
				stage.Denial = data
			}
		}
	case machine.Timer:
		return false
	default:
		// A value the job sent up: one of the stage's emitted items.
		stage.Emitted = append(stage.Emitted, event.Data)
		if s.flow.Emit == name {
			s.request(machine.Request{Send: &machine.Send{To: machine.Parent, Event: event.Name, Data: event.Data}})
		}
	}
	return true
}

// read takes an event of the job that reads a stage's blob: each item it
// emits is one of the stage's, its end says no more will come, and its
// failure fails the stage.
func (s *stepper) read(name string, st *stageState, event machine.Event) bool {
	switch event.Name {
	case machine.Done:
		st.Reading = done
	case machine.Failed:
		data, _ := event.Data.(map[string]any)
		st.Reading, st.Status, st.Error = failed, failed, data["error"]
		if data["status"] == machine.StatusDenied {
			st.Denial = data
		}
		for i, item := range st.Items {
			if item.Status == running {
				s.request(machine.Request{Cancel: fmt.Sprintf("%s.%d", name, i)})
			}
		}
	case machine.Timer:
		return false
	default:
		st.Items = append(st.Items, &itemState{Item: event.Data})
	}
	return true
}

// advance starts every stage that can now start, settles every stage that
// has now settled, and ends the flow when all have, or when a failure
// turns out to be one nobody handles.
func (s *stepper) advance() error {
	names := sortedNames(s.flow.Stages)
	for changed := true; changed; {
		changed = false
		for _, name := range names {
			moved, err := s.move(name)
			if err != nil {
				return err
			}
			changed = changed || moved
		}
	}
	// A failure counts as handled only when a stage whose condition reads
	// it actually runs. If none reads it, the flow fails the moment it
	// arrives; if one does and is then skipped, the flow fails then.
	for _, name := range names {
		st := s.st.Stages[name]
		if st.Status != failed && !s.itemFailed(name) {
			continue
		}
		handled, waiting := false, false
		for _, reader := range names {
			reads := s.need[reader].failure
			if st.Status != failed {
				reads = s.need[reader].output // an item's failure is read with the stage's output
			}
			if !slices.Contains(reads, name) {
				continue
			}
			switch s.st.Stages[reader].Status {
			case pending:
				waiting = true
			case skipped:
			default:
				handled = true
			}
		}
		// The flow's own output is a reader too.
		for _, r := range machine.ReadsIn(s.flow.Output, "stages") {
			if r.Name == name && (st.Status != failed || r.Part == "failed" || r.Part == "error") {
				handled = true
			}
		}
		if handled || waiting {
			continue
		}
		s.fail(name, st)
		return nil
	}
	for _, name := range names {
		if !s.st.Stages[name].settled() {
			return nil
		}
	}
	output, err := s.eval.Fill(s.flow.Output, s.scope(nil), "output")
	if err != nil {
		return err
	}
	s.request(machine.Request{End: &machine.Ending{Output: output}})
	return nil
}

// itemFailed says whether a per-item stage has an item that failed: the
// failure arrives with the item, without waiting for the rest.
func (s *stepper) itemFailed(name string) bool {
	st := s.st.Stages[name]
	if st.Status != done && st.Status != running {
		return false
	}
	for _, item := range st.Items {
		if item.Status == failed {
			return true
		}
	}
	return false
}

// fail fails the flow with the error of a stage nobody handled the failure
// of: the error names the stage and then gives the stage's own.
func (s *stepper) fail(name string, st *stageState) {
	text := st.Error
	denial := st.Denial
	for i, item := range st.Items {
		if item.Status == failed && st.Status != failed {
			name, text, denial = fmt.Sprintf("%s item %d", name, i), item.Error, item.Denial
			break
		}
	}
	e := machine.Unhandled(name, denial)
	e.Error = fmt.Sprintf("%s: %v", name, text)
	// A job asked for in the step that fails the flow would only be
	// cancelled: none is started.
	s.out.Requests = slices.DeleteFunc(s.out.Requests, func(r machine.Request) bool { return r.Job != nil })
	s.request(machine.Request{End: &machine.Ending{Error: e}})
}

// move takes one stage as far as it can go in this step, and says whether
// anything changed.
func (s *stepper) move(name string) (bool, error) {
	stage, st := s.flow.Stages[name], s.st.Stages[name]
	place := "stages." + name
	switch st.Status {
	case done, failed, skipped:
		return false, nil
	case running:
		if !stage.perItem() {
			return false, nil
		}
		return s.items(name, stage, st)
	}
	// Pending: it waits for the stages it runs after.
	need := s.need[name]
	for _, other := range need.after {
		ot := s.st.Stages[other]
		if ot == nil {
			continue
		}
		if !ot.settled() && !(other == stage.Emits && ot.Status == running) {
			return false, nil
		}
	}
	for _, other := range need.after {
		ot := s.st.Stages[other]
		switch {
		case ot == nil:
		case ot.Status == skipped && !slices.Contains(need.skipped, other):
			// A skip spreads along the arrows, unless the stage's own
			// condition asks whether that stage was skipped.
			st.Status = skipped
			return true, nil
		case ot.Status == failed && !slices.Contains(need.failure, other) && (slices.Contains(need.output, other) || other == stage.Emits):
			// It depends on a failed stage for its output and does not read
			// the failure.
			if other != stage.Emits {
				st.Status = skipped
				return true, nil
			}
		}
	}
	if stage.When != "" {
		ok, err := s.eval.Cond(stage.When, s.scope(nil), place+".when")
		if err != nil {
			return false, err
		}
		if !ok {
			st.Status = skipped
			return true, nil
		}
	}
	st.Status = running
	if stage.Blob != nil {
		reader := &machine.JobSpec{Script: itemsScript, Input: map[string]any{"blob": stage.Blob}}
		job, err := reader.Fill(s.eval, s.scope(nil), place+".each.blob", name)
		if err != nil {
			return false, err
		}
		job.Wants = []string{"*"}
		st.Reading = running
		s.request(machine.Request{Job: job})
	}
	if stage.perItem() {
		_, err := s.items(name, stage, st)
		return true, err
	}
	job, err := stage.Job.Fill(s.eval, s.scope(nil), place+".job", name)
	if err != nil {
		return false, err
	}
	job.Wants = s.wants(name)
	s.request(machine.Request{Job: job})
	return true, nil
}

// wants says which emitted values of a stage's job the flow acts on: all
// of them, if a stage follows them or reads them, or the flow emits them
// to its parent.
func (s *stepper) wants(name string) []string {
	if s.flow.Emit == name {
		return []string{"*"}
	}
	for _, stage := range s.flow.Stages {
		if stage.Emits == name {
			return []string{"*"}
		}
		all, _ := stage.reads()
		for _, r := range all {
			if r.Name == name && r.Part == "emitted" {
				return []string{"*"}
			}
		}
	}
	for _, r := range machine.ReadsIn(s.flow.Output, "stages") {
		if r.Name == name && r.Part == "emitted" {
			return []string{"*"}
		}
	}
	return nil
}

// items takes a per-item stage as far as it can go: its list is worked
// out, items are started, and when every item has settled the stage has.
func (s *stepper) items(name string, stage *Stage, st *stageState) (bool, error) {
	place := "stages." + name
	changed := false
	finished := true // no more items will come
	switch {
	case stage.Emits != "":
		source := s.st.Stages[stage.Emits]
		for len(st.Items) < len(source.Emitted) {
			st.Items = append(st.Items, &itemState{Item: source.Emitted[len(st.Items)]})
			changed = true
		}
		finished = source.settled()
	case stage.Blob != nil:
		finished = st.Reading == done
	case !st.Listed:
		value, err := s.eval.Fill(stage.Each, s.scope(nil), place+".each")
		if err != nil {
			return false, err
		}
		list, ok := value.([]any)
		if !ok {
			what := "a value that is not a list"
			if _, isBlob := blob.AsRef(value); isBlob {
				what = "a blob (to run once for each item of a blob, write each: { blob: … })"
			}
			return false, fmt.Errorf("%s.each: each needs a list, and was given %s", place, what)
		}
		for _, item := range list {
			st.Items = append(st.Items, &itemState{Item: item})
		}
		st.Listed, changed = true, true
	}
	going := 0
	for _, item := range st.Items {
		if item.Status == running {
			going++
		}
	}
	for i, item := range st.Items {
		if item.Status != pending || (stage.Serial && going > 0) {
			continue
		}
		scope := s.scope(item.Item)
		job, err := stage.Job.Fill(s.eval, scope, place+".job", fmt.Sprintf("%s.%d", name, i))
		if err != nil {
			return false, err
		}
		item.Status = running
		going++
		changed = true
		s.request(machine.Request{Job: job})
	}
	if !finished {
		return changed, nil
	}
	output := make([]any, len(st.Items))
	for i, item := range st.Items {
		switch item.Status {
		case done:
			output[i] = map[string]any{"item": item.Item, "output": item.Output}
		case failed:
			output[i] = map[string]any{"item": item.Item, "error": item.Error}
		default:
			return changed, nil
		}
	}
	st.Status, st.Output = done, output
	// If the stage whose emits it followed failed, the items that started
	// have finished and the stage stands as done with what it has.
	return true, nil
}

// scope is what a stage's expressions read: the flow's input, every
// stage's output, failure and skip, and for a per-item stage its item.
func (s *stepper) scope(item any) machine.Scope {
	stages := make(map[string]any, len(s.st.Stages))
	for name, st := range s.st.Stages {
		entry := map[string]any{"failed": st.Status == failed, "skipped": st.Status == skipped}
		if st.Output != nil {
			entry["output"] = st.Output
		}
		if st.Error != nil {
			entry["error"] = st.Error
		}
		if st.Emitted != nil {
			entry["emitted"] = st.Emitted
		} else {
			entry["emitted"] = []any{}
		}
		stages[name] = entry
	}
	return machine.Scope{"input": s.in.Input, "stages": stages, "item": item}
}

// Show gives what a watcher is shown of a flow's state: where each stage
// stands.
func (flowType) Show(def *machine.Definition, raw jsontext.Value) any {
	var st state
	if raw == nil || json.Unmarshal(raw, &st) != nil {
		return nil
	}
	shown := map[string]any{}
	for name, stage := range st.Stages {
		status := stage.Status
		if status == pending {
			status = instance.Waiting
		}
		shown[name] = status
	}
	return map[string]any{"stages": shown}
}

func (flowType) Manual() string { return manual }
