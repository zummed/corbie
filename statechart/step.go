package statechart

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"slices"
	"time"

	"github.com/zummed/corbie/machine"
)

// state is everything a chart remembers between steps. It does not grow
// with use: one output for each state, and what the active states wait on.
type state struct {
	// Active is the active states, outermost first.
	Active  []string             `json:"active"`
	Outputs map[string]any       `json:"outputs,omitzero"`
	Keys    map[string]string    `json:"keys,omitzero"`   // by waiting state
	Timers  map[string]time.Time `json:"timers,omitzero"` // by timer name
	// Partial holds, for a state with several named jobs, the outputs of
	// those that are done.
	Partial map[string]map[string]any `json:"partial,omitzero"`
}

// maxMoves is how many transitions one step may make before its chain is
// taken not to come to rest.
const maxMoves = 500

// stepper is one step being taken.
type stepper struct {
	chart *Chart
	all   map[string]*node
	jobs  map[string]string // each job's state
	in    *machine.StepIn
	st    *state
	out   *machine.StepOut
	eval  *machine.Evaluator
	event machine.Event
	// fresh names what this step made: an event that arrived for an earlier
	// job, timer or key of the same name is not for these.
	fresh    map[string]bool
	finished []string // final states entered and not yet acted on
	moves    int
	keys     int
	ended    bool
}

func (chartType) Step(in *machine.StepIn) (*machine.StepOut, error) {
	var chart Chart
	if err := json.Unmarshal(in.Definition.Body, &chart); err != nil {
		return nil, err
	}
	s := &stepper{chart: &chart, in: in, st: &state{}, out: &machine.StepOut{}, jobs: map[string]string{},
		eval: machine.NewEvaluator(in.Time, in.Seed), fresh: map[string]bool{}}
	s.all = chart.index(func(string, string, ...any) {})
	for name, n := range s.all {
		if n.state.Job != nil {
			s.jobs[name] = name
		}
		for job := range n.state.Jobs {
			s.jobs[job] = name
		}
	}
	if in.State != nil {
		if err := json.Unmarshal(in.State, s.st); err != nil {
			return nil, err
		}
	}
	for i, event := range in.Events {
		if s.ended {
			break
		}
		s.event = event
		if err := s.handle(i, event); err != nil {
			return nil, err
		}
		if err := s.settle(); err != nil {
			return nil, err
		}
	}
	s.out.State = s.st
	return s.out, nil
}

func (s *stepper) request(r machine.Request) { s.out.Requests = append(s.out.Requests, r) }

func (s *stepper) active(name string) bool { return slices.Contains(s.st.Active, name) }

// handle gives one event to the chart.
func (s *stepper) handle(i int, event machine.Event) error {
	switch event.Name {
	case machine.Start:
		return s.enter(first(s.chart.Initial, s.chart.States))
	case machine.Done, machine.Failed:
		name, ok := s.jobs[event.From]
		if !ok || !s.active(name) || s.fresh["job "+event.From] {
			return nil // the state was left, or left and entered again
		}
		if event.Name == machine.Failed {
			return s.failed(name, event)
		}
		n := s.all[name].state
		if n.Jobs != nil {
			// Done when all its jobs are done; its output is each one's by name.
			if s.st.Partial == nil {
				s.st.Partial = map[string]map[string]any{}
			}
			if s.st.Partial[name] == nil {
				s.st.Partial[name] = map[string]any{}
			}
			s.st.Partial[name][event.From] = event.Data
			if len(s.st.Partial[name]) < len(n.Jobs) {
				return nil
			}
			event = machine.Event{Name: machine.Done, From: name, Data: s.st.Partial[name]}
			delete(s.st.Partial, name)
			s.event = event
		}
		s.setOutput(name, event.Data)
		_, err := s.offer(name, func(st *State) []Transition { return st.Done })
		return err
	case machine.Timer:
		at, ok := s.st.Timers[event.From]
		due, _ := event.Data.(string)
		if !ok || s.fresh["timer "+event.From] || at.UTC().Format(time.RFC3339Nano) != due {
			return nil // its state was left
		}
		var name string
		var index int
		if _, err := fmt.Sscanf(event.From, "%d.", &index); err != nil {
			return nil
		}
		name = event.From[len(fmt.Sprintf("%d.", index)):]
		n, ok := s.all[name]
		if !ok || index >= len(n.state.After) {
			return nil
		}
		delete(s.st.Timers, event.From)
		_, err := s.take(name, n.place+".after."+n.state.After[index].At, n.state.After[index].Rules)
		return err
	}
	// Any other event: an answer to a key goes to the state whose key it
	// carries, and anything else to the innermost active state; from there
	// it is offered outward.
	from := ""
	if len(s.st.Active) > 0 {
		from = s.st.Active[len(s.st.Active)-1]
	}
	if _, isKey := s.st.Keys[event.From]; isKey && s.all[event.From] != nil && s.all[event.From].state.Wait == event.Name {
		if s.fresh["key "+event.From] {
			return nil
		}
		from = event.From
	}
	taken, err := s.offer(from, func(st *State) []Transition { return st.On[event.Name] })
	if err == nil && !taken {
		s.out.Ignored = append(s.out.Ignored, i)
	}
	return err
}

// failed gives a job's failure to its state and outward. A failure nobody
// takes fails the chart, whose error names the job and then gives the
// job's error.
func (s *stepper) failed(name string, event machine.Event) error {
	taken, err := s.offer(name, func(st *State) []Transition { return st.Failed })
	if err != nil || taken {
		return err
	}
	e := machine.Unhandled(event.From, event.Data)
	s.end(&machine.Ending{Error: e})
	return nil
}

// offer gives the current event to a state and, where it has no rule that
// is taken, to the state that holds it, and so on outward.
func (s *stepper) offer(name string, rules func(*State) []Transition) (bool, error) {
	for name != "" {
		n := s.all[name]
		if taken, err := s.take(name, n.place, rules(n.state)); err != nil || taken {
			return taken, err
		}
		name = n.parent
	}
	return false, nil
}

// take takes the first transition of a rule whose condition holds.
func (s *stepper) take(holder, place string, rule []Transition) (bool, error) {
	for i, t := range rule {
		if t.When != "" {
			ok, err := s.eval.Cond(t.When, s.scope(holder), fmt.Sprintf("%s[%d].when", place, i))
			if err != nil {
				return false, err
			}
			if !ok {
				continue
			}
		}
		return true, s.move(holder, fmt.Sprintf("%s[%d]", place, i), t)
	}
	return false, nil
}

// move makes a transition: leave, send, enter. One that names no state
// stays and only sends. A move to a state inside the holder does not leave
// the holder; any other move leaves everything up to the first state that
// holds both, the holder included.
func (s *stepper) move(holder, place string, t Transition) error {
	if s.moves++; s.moves > maxMoves {
		s.end(&machine.Ending{Error: &machine.Error{Status: machine.StatusError,
			Error: fmt.Sprintf("the chart does not come to rest: one event set off more than %d transitions", maxMoves)}})
		return nil
	}
	if t.To == "" {
		return s.send(holder, place+".send", t.Send)
	}
	// The state that stays: the holder for a move inward, else the nearest
	// state that holds both the holder and the target.
	stays := holder
	if !s.inside(t.To, holder) {
		stays = s.all[holder].parent
		for stays != "" && !s.inside(t.To, stays) {
			stays = s.all[stays].parent
		}
	}
	keep := 0
	if stays != "" {
		keep = slices.Index(s.st.Active, stays) + 1
	}
	for i := len(s.st.Active) - 1; i >= keep; i-- {
		if err := s.leave(s.st.Active[i]); err != nil {
			return err
		}
	}
	s.st.Active = s.st.Active[:keep]
	if err := s.send(holder, place+".send", t.Send); err != nil {
		return err
	}
	// Enter from just below what stays, down to the target.
	var path []string
	for name := t.To; name != stays; name = s.all[name].parent {
		path = append(path, name)
	}
	slices.Reverse(path)
	for _, name := range path[:len(path)-1] {
		if err := s.enterOne(name); err != nil {
			return err
		}
	}
	return s.enter(t.To)
}

// inside says whether a state is held, at any depth, by another.
func (s *stepper) inside(name, holder string) bool {
	for p := s.all[name].parent; p != ""; p = s.all[p].parent {
		if p == holder {
			return true
		}
	}
	return false
}

// enter enters a state and, where it holds states, the one entered first
// inside it, and so on down.
func (s *stepper) enter(name string) error {
	for name != "" && !s.ended {
		if err := s.enterOne(name); err != nil {
			return err
		}
		n := s.all[name].state
		name = first(n.Initial, n.States)
	}
	return nil
}

// enterOne enters one state: its key exists first, then its entry sends
// are made, its timers start and its jobs are asked for.
func (s *stepper) enterOne(name string) error {
	n := s.all[name]
	st := n.state
	s.st.Active = append(s.st.Active, name)
	if st.Wait != "" {
		s.keys++
		mac := hmac.New(sha256.New, s.in.Seed)
		fmt.Fprintf(mac, "key\x00%s\x00%d", name, s.keys)
		// In hex: a value a person can write as it is after a command.
		value := hex.EncodeToString(mac.Sum(nil)[:18])
		if s.st.Keys == nil {
			s.st.Keys = map[string]string{}
		}
		s.st.Keys[name] = value
		s.fresh["key "+name] = true
		s.request(machine.Request{Key: &machine.KeyNamed{ID: name, Name: st.Wait, Value: value}})
	}
	if err := s.send(name, n.place+".entry.send", st.Entry); err != nil {
		return err
	}
	for i, after := range st.After {
		w, err := parseAt(after.At)
		if err != nil {
			return err
		}
		timer := fmt.Sprintf("%d.%s", i, name)
		at := w.next(s.in.Time)
		if s.st.Timers == nil {
			s.st.Timers = map[string]time.Time{}
		}
		s.st.Timers[timer] = at
		s.fresh["timer "+timer] = true
		s.request(machine.Request{Timer: &machine.TimerAt{Name: timer, At: at}})
	}
	if st.Job != nil {
		if err := s.job(name, n.place+".job", name, st.Job); err != nil {
			return err
		}
	}
	for _, job := range sortedNames(st.Jobs) {
		if err := s.job(name, n.place+".jobs."+job, job, st.Jobs[job]); err != nil {
			return err
		}
	}
	if st.Final {
		s.finished = append(s.finished, name)
	}
	return nil
}

func (s *stepper) job(holder, place, name string, spec *machine.JobSpec) error {
	job, err := spec.Fill(s.eval, s.scope(holder), place, name)
	if err != nil {
		return err
	}
	job.Wants = s.wants()
	s.fresh["job "+name] = true
	s.request(machine.Request{Job: job})
	return nil
}

// wants names every event the chart has a rule for under on:, which are
// the emitted values it acts on.
func (s *stepper) wants() []string {
	seen := map[string]bool{}
	for _, n := range s.all {
		for event := range n.state.On {
			seen[event] = true
		}
	}
	return sortedNames(seen)
}

// leave leaves one state: its leaving sends are made, its jobs and timers
// are cancelled and its key ends.
func (s *stepper) leave(name string) error {
	n := s.all[name]
	st := n.state
	if err := s.send(name, n.place+".exit.send", st.Exit); err != nil {
		return err
	}
	if st.Job != nil {
		s.request(machine.Request{Cancel: name})
	}
	for _, job := range sortedNames(st.Jobs) {
		s.request(machine.Request{Cancel: job})
	}
	for i := range st.After {
		timer := fmt.Sprintf("%d.%s", i, name)
		if _, ok := s.st.Timers[timer]; ok {
			delete(s.st.Timers, timer)
			s.request(machine.Request{Clear: timer})
		}
	}
	if _, ok := s.st.Keys[name]; ok {
		delete(s.st.Keys, name)
		s.request(machine.Request{Drop: name})
	}
	delete(s.st.Partial, name)
	return nil
}

// send makes sends, their data filled in as they are made.
func (s *stepper) send(holder, place string, sends []Send) error {
	for i, send := range sends {
		data, err := s.eval.Fill(send.Data, s.scope(holder), fmt.Sprintf("%s[%d].data", place, i))
		if err != nil {
			return err
		}
		s.request(machine.Request{Send: &machine.Send{To: send.To, Event: send.Event, Data: data}})
	}
	return nil
}

// settle acts on the final states entered: innermost first, each once. A
// final state ends the state that holds it, one at the top ends the chart,
// and one marked as a failure fails the chart from any depth.
func (s *stepper) settle() error {
	for len(s.finished) > 0 && !s.ended {
		name := s.finished[0]
		s.finished = s.finished[1:]
		if !s.active(name) {
			continue
		}
		n := s.all[name]
		output, err := s.eval.Fill(n.state.Output, s.scope(name), n.place+".output")
		if err != nil {
			return err
		}
		switch {
		case n.state.Fails:
			s.end(&machine.Ending{Error: &machine.Error{Status: machine.StatusError,
				Error: fmt.Sprintf("the chart reached %s, a final state marked as a failure", name), Output: output}})
		case n.parent == "":
			s.end(&machine.Ending{Output: output})
		default:
			s.setOutput(n.parent, output)
			s.event = machine.Event{Name: machine.Done, From: n.parent, Data: output}
			if _, err := s.offer(n.parent, func(st *State) []Transition { return st.Done }); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *stepper) end(ending *machine.Ending) {
	s.ended = true
	s.request(machine.Request{End: ending})
}

func (s *stepper) setOutput(name string, output any) {
	if s.st.Outputs == nil {
		s.st.Outputs = map[string]any{}
	}
	if output == nil {
		delete(s.st.Outputs, name)
		return
	}
	s.st.Outputs[name] = output
}

// scope is what an expression in a state reads: the chart's input, each
// state's latest output, the event that caused the current move, and the
// keys of the state and of those that hold it, its own first.
func (s *stepper) scope(holder string) machine.Scope {
	states := make(map[string]any, len(s.all))
	for name := range s.all {
		entry := map[string]any{}
		if output, ok := s.st.Outputs[name]; ok {
			entry["output"] = output
		}
		states[name] = entry
	}
	keys := map[string]any{}
	for name := holder; name != ""; name = s.all[name].parent {
		if value, ok := s.st.Keys[name]; ok {
			if _, nearer := keys[s.all[name].state.Wait]; !nearer {
				keys[s.all[name].state.Wait] = value
			}
		}
	}
	event := map[string]any{"name": s.event.Name, "from": s.event.From, "data": s.event.Data}
	return machine.Scope{"input": s.in.Input, "states": states, "event": event, "keys": keys}
}

// Show gives what a watcher is shown of a chart's state: the states it is
// in and the names of the keys it waits on.
func (chartType) Show(def *machine.Definition, raw jsontext.Value) any {
	var st state
	if raw == nil || json.Unmarshal(raw, &st) != nil {
		return nil
	}
	return map[string]any{"states": st.Active}
}

func (chartType) Manual() string { return manual }
