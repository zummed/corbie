package drawing

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/statechart"
)

// chart is a statechart as its graph is read: every state by its name, the
// state that holds each, the states each holds, and the state each level
// starts in. A send that names a machine is a state, by a name no state can
// have.
type chart struct {
	states map[string]*statechart.State
	parent map[string]string
	start  map[string]string
	list   []State
	sends  []rule
	all    []rule
	walker *walker
}

// A rule is one transition: from a state, on an event, to a state.
type rule struct {
	from, event, to, when, timer string
	send                         bool
}

func chartOf(c *statechart.Chart, run *Run, w *walker) *Chart {
	ch := &chart{states: map[string]*statechart.State{}, parent: map[string]string{}, start: map[string]string{"": c.Initial}, walker: w}
	ch.index("", "states.", c.States)
	ch.all = ch.ruleList()
	marks := ch.marks(run)
	out := &Chart{Start: c.Initial, States: ch.list, Rules: []Rule{}}
	for i := range out.States {
		out.States[i].Mark = marks.of(out.States[i].Name)
	}
	for _, r := range ch.all {
		out.Rules = append(out.Rules, Rule{From: r.from, Event: r.event, To: r.to, When: r.when, Timer: r.timer, Send: r.send, Taken: marks.taken[r]})
	}
	return out
}

// index lists the states of a level by name, each before the states it
// holds, and the sends of each after those. place is where the definition
// writes the level's states, such as states.working.states.
func (ch *chart) index(holder, place string, states map[string]*statechart.State) {
	for _, name := range sortedKeys(states) {
		s := states[name]
		ch.states[name] = s
		ch.parent[name] = holder
		st := State{Card: ch.cardOf(name, s), Name: name, Parent: holder, Final: final(s), Fails: final(s) && s.End == "failed"}
		st.Card.Line = lineOf(ch.walker.lines, place+name)
		ch.list = append(ch.list, st)
		if s == nil {
			continue
		}
		if len(s.States) > 0 {
			ch.start[name] = s.Initial
			ch.list[len(ch.list)-1].Start = s.Initial
			ch.index(name, place+name+".states.", s.States)
		}
		ch.sent(holder, name, "entry", s.Entry)
		ch.sent(holder, name, "exit", s.Exit)
		for _, event := range sortedKeys(s.On) {
			for _, t := range s.On[event] {
				ch.sent(holder, name, event, t.Send)
			}
		}
	}
}

// sent gives each send to an address a card below the state that sends it,
// named by the address.
func (ch *chart) sent(holder, name, event string, sends []statechart.Send) {
	for _, send := range sends {
		if send.Address == nil {
			continue
		}
		key := fmt.Sprintf("%s.%s.%d", name, event, len(ch.sends))
		ch.states[key] = &statechart.State{}
		ch.parent[key] = holder
		ch.list = append(ch.list, State{Card: Card{Name: Address(send.Address), Icon: "statechart", Lines: []string{"send " + send.Event}}, Name: key, Parent: holder, Send: true})
		ch.sends = append(ch.sends, rule{from: name, event: event, to: key, send: true})
	}
}

// Address writes an address as the command line does: the location, with
// #key or a run's ID after it, or the one expression that gives it.
func Address(a *machine.AddressSpec) string {
	switch {
	case a.Expr != "":
		return a.Expr
	case a.Key != "":
		return a.At + "#" + a.Key
	case a.Run != "":
		return a.At + " run " + a.Run
	}
	return a.At
}

// cardOf gives what a state's card says.
func (ch *chart) cardOf(name string, s *statechart.State) Card {
	c := Card{Name: name}
	if s == nil {
		return c
	}
	c.Narrow = final(s)
	ch.walker.describe(&c, name, s.Job)
	if len(s.Jobs) > 0 {
		c.Icon = "script"
		c.Lines = append(c.Lines, fit("jobs: "+strings.Join(sortedKeys(s.Jobs), ", ")))
	}
	c.Lines = append(c.Lines, tags(s)...)
	return c
}

func (ch *chart) ruleList() []rule {
	var rules []rule
	for _, name := range sortedKeys(ch.states) {
		s := ch.states[name]
		if s == nil {
			continue
		}
		for _, event := range sortedKeys(s.On) {
			for _, t := range s.On[event] {
				if t.Go == "" {
					continue
				}
				rules = append(rules, rule{from: name, event: event, to: t.Go, when: t.When, timer: s.Timers[event]})
			}
		}
	}
	return append(rules, ch.sends...)
}

func final(s *statechart.State) bool { return s != nil && s.End != "" }

// tags are what a state waits on, written in it: its ticket and its timers.
func tags(s *statechart.State) []string {
	if s == nil || len(s.States) > 0 {
		return nil
	}
	var lines []string
	if s.Ticket != "" {
		lines = append(lines, fit("ticket "+s.Ticket))
	}
	for _, name := range sortedKeys(s.Timers) {
		lines = append(lines, fit("timer "+name))
	}
	return lines
}

// chartMarks are a run's states, as the drawing marks them: a state the run
// is in whose job is held for a hook is marked held.
type chartMarks struct {
	run      *Run
	now, was map[string]bool // in now, and in at any step
	taken    map[rule]bool   // the rules the latest step took
}

// marks follows a run through its steps: from the states it was in before
// each step, by the events the step took, to the states it was in after.
// A state passed through within one step is one it was in.
func (ch *chart) marks(run *Run) chartMarks {
	m := chartMarks{run: run, now: map[string]bool{}, was: map[string]bool{}, taken: map[rule]bool{}}
	if run == nil || len(run.Steps) == 0 {
		return m
	}
	in := ch.with(ch.entering(ch.start[""])...)
	maps.Copy(m.was, in)
	for _, step := range run.Steps {
		after := ch.with(step.Shown.States...)
		m.taken = map[rule]bool{}
		for _, r := range ch.took(in, step.Events, after) {
			m.taken[r] = true
			maps.Copy(m.was, ch.with(ch.entering(r.to)...))
		}
		maps.Copy(m.was, after)
		in = after
	}
	m.now = in
	return m
}

// took gives the rules a step took, an event at a time, from the states the
// run was in before it. Where an event has rules to several states, the one
// to a state the run is in after the step is the one taken; a rule with a
// condition that leads to none of those is not known to be taken.
func (ch *chart) took(in map[string]bool, events []string, after map[string]bool) []rule {
	var taken []rule
	for _, e := range events {
		var found []rule
		for _, r := range ch.all {
			if !r.send && r.event == e && in[r.from] {
				found = append(found, r)
			}
		}
		i := slices.IndexFunc(found, func(r rule) bool { return after[r.to] })
		if i < 0 && len(found) == 1 && found[0].when == "" {
			i = 0
		}
		if i < 0 {
			continue
		}
		taken = append(taken, found[i])
		in = ch.with(ch.entering(found[i].to)...)
	}
	return taken
}

// with gives states with every state that holds them.
func (ch *chart) with(names ...string) map[string]bool {
	in := map[string]bool{}
	for _, n := range names {
		for ; n != "" && !in[n]; n = ch.parent[n] {
			in[n] = true
		}
	}
	return in
}

// entering gives a state with the state it starts in, and the state that
// one starts in, down to a state that holds none.
func (ch *chart) entering(name string) []string {
	var names []string
	for ; name != "" && !slices.Contains(names, name); name = ch.start[name] {
		names = append(names, name)
	}
	return names
}

func (m chartMarks) of(name string) string {
	switch {
	case m.run == nil:
		return ""
	case m.now[name] && !m.run.Ended && slices.Contains(m.run.Held, name):
		return "held"
	case m.now[name] && !m.run.Ended:
		return "now"
	case m.now[name] && m.run.Failed:
		return "failed"
	case m.was[name]:
		return "done"
	}
	return "never"
}
