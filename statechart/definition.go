// Package statechart is the machine that is always in some state and moves
// between states when events arrive.
package statechart

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // time zones are known to the binary itself

	"github.com/zummed/corbie/machine"
)

// Chart is the full form of a statechart's own part of a definition.
type Chart struct {
	Initial string            `json:"initial,omitzero"`
	States  map[string]*State `json:"states"`
}

// State is one state in full form.
type State struct {
	Initial string                      `json:"initial,omitzero"`
	States  map[string]*State           `json:"states,omitzero"`
	Job     *machine.JobSpec            `json:"job,omitzero"`
	Jobs    map[string]*machine.JobSpec `json:"jobs,omitzero"`
	Ticket  string                      `json:"ticket,omitzero"`
	Timers  map[string]string           `json:"timers,omitzero"` // the time of each, by its name
	Entry   []Send                      `json:"entry,omitzero"`
	Exit    []Send                      `json:"exit,omitzero"`
	// On is where to go on each event, by its name: done, failed, a
	// timer, a ticket's answer, or any other event.
	On map[string][]Transition `json:"on,omitzero"`
	// End is done or failed in a final state: how it ends the state that
	// holds it, or the chart.
	End    string `json:"end,omitzero"`
	Output any    `json:"output,omitzero"`
}

// Transition is one candidate of a rule.
type Transition struct {
	Go   string `json:"go,omitzero"`
	When string `json:"when,omitzero"`
	Send []Send `json:"send,omitzero"`
}

// Send is the chart's one action: a named event to the parent or a child.
type Send struct {
	To    string `json:"to"`
	Event string `json:"event"`
	Data  any    `json:"data,omitzero"`
}

// names are what an expression of a chart reads.
var names = []string{"event", "input", "states", "tickets"}

type chartType struct{}

func init() { machine.Register(chartType{}) }

func (chartType) Name() string { return "statechart" }

// What a chart's definition holds of its own. A state may hold states, so
// its shape holds itself.
var (
	sendShape = &machine.Shape{Name: "send", What: "a send", Fields: []machine.Field{
		{Name: "to", Kind: machine.KindWord, Like: "parent", Does: "who it is sent to: parent, or a state or a named job of the chart"},
		{Name: "event", Kind: machine.KindWord, Like: "review_wanted", Does: "the event's name"},
		{Name: "data", Kind: machine.KindValue, Like: `{ ticket: "${tickets.verdict}" }`, Does: "what the event carries"},
	}}
	sends = machine.Field{Name: "send", Kind: machine.KindValue, Of: sendShape, Like: "{ to: parent, event: review_wanted }",
		Does: "an event to send: one, or a list"}
	actionShape     = &machine.Shape{Name: "action", What: "what a state does on entering or leaving", Fields: []machine.Field{sends}}
	transitionShape = &machine.Shape{Name: "transition", What: "a transition", Fields: []machine.Field{
		{Name: "go", Kind: machine.KindWord, Like: "approved", Does: "the state to go to; with none, it stays and only sends"},
		{Name: "when", Kind: machine.KindWord, Like: "${event.data.ok}", Does: "a condition, one expression: the transition is taken only if it holds"},
		sends,
	}}
	stateShape = &machine.Shape{Name: "state", What: "a state"}
	shape      = &machine.Shape{Name: "statechart", What: "a statechart", Fields: []machine.Field{
		{Name: "initial", Kind: machine.KindWord, Like: "drafting", Does: "the state it starts in"},
		{Name: "states", Kind: machine.KindNamed, Of: stateShape, Does: "its states"},
	}}
)

func init() {
	stateShape.Fields = []machine.Field{
		{Name: "initial", Kind: machine.KindWord, Like: "drafting", Does: "of the states it holds, the one it starts in"},
		{Name: "states", Kind: machine.KindNamed, Of: stateShape, Does: "states of its own"},
		{Name: "job", Kind: machine.KindJob, Like: "{ script: ./draft.js }", Does: "a script or a machine, started on entry and cancelled on leaving"},
		{Name: "jobs", Kind: machine.KindNamed, Of: machine.JobShape, Does: "several named jobs; the state is done when all are, and its output is each one's by name"},
		{Name: "ticket", Kind: machine.KindWord, Like: "verdict", Does: "the name of a ticket the state issues while it is active, read as ${tickets.name}; its answer is an event of that name"},
		{Name: "timers", Kind: machine.KindNamed, Like: "{ too_long: 48h }",
			Does: "timers started on entry and cancelled on leaving, each a length of time (90s, 48h, 7d) or a time of day; each is an event of its name when it is due"},
		{Name: "entry", Kind: machine.KindFields, Of: actionShape, Like: "{ send: { to: parent, event: review_wanted } }", Does: "what it does on being entered: it sends"},
		{Name: "exit", Kind: machine.KindFields, Of: actionShape, Like: "{ send: { to: parent, event: left } }", Does: "what it does on being left: it sends"},
		{Name: "on", Kind: machine.KindNamed, Of: transitionShape, Like: "{ done: waiting, failed: gave_up }",
			Does: "where to go on each event, by its name: a state's name, one transition, or a list. done and failed are the state's jobs ending, and any other is a timer, a ticket's answer, an event from the parent or a child, or a value a job emitted"},
		{Name: "end", Kind: machine.KindWord, Like: "done", Values: []string{machine.Done, machine.Failed},
			Does: "makes it a final state, which ends the state that holds it, or the chart, as done or failed"},
		{Name: "output", Kind: machine.KindValue, Like: "${states.drafting.output}", Does: "of a final state: the result"},
	}
}

func (chartType) Shape() *machine.Shape { return shape }

// Fill reads a chart as written and gives its full form.
func (chartType) Fill(p *machine.Parser, f *machine.Fields) any {
	chart := &Chart{Initial: f.Text("initial")}
	states, order := f.Map("states")
	chart.States = fillStates(p, "states", states, order)
	return chart
}

func fillStates(p *machine.Parser, place string, written map[string]any, order []string) map[string]*State {
	states := map[string]*State{}
	for _, name := range order {
		states[name] = fillState(p, place+"."+name, written[name])
	}
	return states
}

func fillState(p *machine.Parser, place string, written any) *State {
	s := &State{}
	f := p.Fields(place, written, stateShape)
	if f == nil {
		return s
	}
	s.Initial = f.Text("initial")
	inner, order := f.Map("states")
	s.States = fillStates(p, place+".states", inner, order)
	if len(s.States) == 0 {
		s.States = nil
	}
	if job, ok := f.Value("job"); ok {
		s.Job = p.Job(place+".job", job)
	}
	if jobs, order := f.Map("jobs"); jobs != nil {
		s.Jobs = map[string]*machine.JobSpec{}
		for _, name := range order {
			s.Jobs[name] = p.Job(place+".jobs."+name, jobs[name])
		}
	}
	s.Ticket = f.Text("ticket")
	if timers, order := f.Map("timers"); timers != nil {
		s.Timers = map[string]string{}
		for _, name := range order {
			at, ok := timers[name].(string)
			if !ok {
				p.Problem(place+".timers."+name, "a timer's time is text: a length of time or a time of day")
			}
			s.Timers[name] = at
		}
	}
	s.Entry = fillActions(p, f, "entry")
	s.Exit = fillActions(p, f, "exit")
	if on, order := f.Map("on"); on != nil {
		s.On = map[string][]Transition{}
		for _, event := range order {
			s.On[event] = fillRule(p, place+".on."+event, on[event], f.Of("on"))
		}
	}
	s.End = f.Text("end")
	s.Output, _ = f.Value("output")
	f.Done()
	return s
}

// fillRule reads a rule: a bare state name, one transition, or a list. A
// transition written out is read by the shape its field gives it.
func fillRule(p *machine.Parser, place string, written any, transition *machine.Shape) []Transition {
	list, ok := written.([]any)
	if !ok {
		list = []any{written}
	}
	var rule []Transition
	for i, item := range list {
		at := place
		if ok {
			at = fmt.Sprintf("%s[%d]", place, i)
		}
		if to, isName := item.(string); isName {
			rule = append(rule, Transition{Go: to})
			continue
		}
		f := p.Fields(at, item, transition)
		if f == nil {
			continue
		}
		t := Transition{Go: f.Text("go"), When: f.Cond("when")}
		t.Send = fillSends(p, at+".send", f, "send")
		f.Done()
		rule = append(rule, t)
	}
	return rule
}

// fillActions reads what a state does on entering or leaving: it sends.
func fillActions(p *machine.Parser, f *machine.Fields, name string) []Send {
	inner := f.Fields(name)
	if inner == nil {
		return nil
	}
	sends := fillSends(p, inner.Place()+".send", inner, "send")
	inner.Done()
	return sends
}

// fillSends reads one send or a list of them.
func fillSends(p *machine.Parser, place string, f *machine.Fields, name string) []Send {
	v, ok := f.Value(name)
	if !ok {
		return nil
	}
	list, isList := v.([]any)
	if !isList {
		list = []any{v}
	}
	var sends []Send
	for i, item := range list {
		at := place
		if isList {
			at = fmt.Sprintf("%s[%d]", place, i)
		}
		sf := p.Fields(at, item, f.Of(name))
		if sf == nil {
			continue
		}
		send := Send{To: sf.Text("to"), Event: sf.Text("event")}
		send.Data, _ = sf.Value("data")
		sf.Done()
		sends = append(sends, send)
	}
	return sends
}

// node is a state with where it sits.
type node struct {
	name   string
	state  *State
	parent string // the state that holds it; "" at the top
	place  string
}

// index lists every state of a chart by its name. A name used twice is a
// problem, given to report.
func (c *Chart) index(report func(place, format string, args ...any)) map[string]*node {
	all := map[string]*node{}
	var walk func(states map[string]*State, parent, place string)
	walk = func(states map[string]*State, parent, place string) {
		for _, name := range sortedNames(states) {
			at := place + "." + name
			if _, dup := all[name]; dup {
				report(at, "there are two states named %q: a state's name is used once in a chart", name)
				continue
			}
			all[name] = &node{name: name, state: states[name], parent: parent, place: at}
			if states[name] != nil {
				walk(states[name].States, name, at+".states")
			}
		}
	}
	walk(c.States, "", "states")
	return all
}

func sortedNames[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Check reports what is wrong with a chart in full form.
func (chartType) Check(c *machine.Checker, def *machine.Definition) {
	var chart Chart
	if !c.DecodeBody(def, &chart) {
		return
	}
	all := chart.index(c.Problem)
	if len(chart.States) == 0 {
		c.Problem("states", "a chart needs at least one state")
		return
	}
	checkInitial(c, "", chart.Initial, chart.States)
	// The children a send may be addressed to: the job of a state, or a
	// named job, that is a machine.
	children, scripts, jobState := map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, name := range sortedNames(all) {
		s := all[name].state
		if s == nil {
			continue
		}
		note := func(job string, spec *machine.JobSpec) {
			if spec == nil {
				return
			}
			jobState[job] = name
			if spec.IsMachine() {
				children[job] = true
			} else {
				scripts[job] = true
			}
		}
		note(name, s.Job)
		for _, job := range sortedNames(s.Jobs) {
			if _, clash := all[job]; clash {
				c.Problem(all[name].place+".jobs."+job, "the job %q has the name of a state", job)
			}
			if other, dup := jobState[job]; dup && other != name {
				c.Problem(all[name].place+".jobs."+job, "there are two jobs named %q", job)
			}
			note(job, s.Jobs[job])
		}
	}
	// An event has a name, where it came from and its data, and nothing
	// else for an expression to read.
	eventParts := func(place string, reads []machine.Read) {
		for _, r := range reads {
			if r.Name != "name" && r.Name != "from" && r.Name != "data" {
				c.Problem(place, "\"event.%s\" reads a part an event does not have: an event has name, from and data (a job's output is event.data, and its error event.data.error)", r.Name)
			}
		}
	}
	issued := map[string]string{} // each ticket's name, by the state that issues it
	for _, name := range sortedNames(all) {
		n := all[name]
		s := n.state
		if s == nil {
			c.Problem(n.place, "the state is empty")
			continue
		}
		c.Name(n.place, name)
		eventParts(n.place+".output", machine.ReadsIn(s.Output, "event"))
		for _, spec := range append([]*machine.JobSpec{s.Job}, jobsOf(s)...) {
			if spec != nil {
				eventParts(n.place+".job", machine.ReadsIn(spec.Input, "event"))
			}
		}
		for _, send := range append(slices.Clone(s.Entry), s.Exit...) {
			eventParts(n.place, machine.ReadsIn(send.Data, "event"))
		}
		for _, rule := range rulesOf(s) {
			for _, t := range rule {
				if t.When != "" {
					eventParts(n.place, machine.ReadsOf(t.When, "event"))
				}
				for _, send := range t.Send {
					eventParts(n.place, machine.ReadsIn(send.Data, "event"))
				}
			}
		}
		checkInitial(c, n.place, s.Initial, s.States)
		holds := len(s.States) > 0
		switch {
		case holds && (s.Job != nil || s.Jobs != nil):
			c.Problem(n.place, "a state that holds states has no job of its own")
		case s.Job != nil && s.Jobs != nil:
			c.Problem(n.place, "a state has one job or several named jobs, not both")
		}
		switch {
		case s.End != "" && s.End != machine.Done && s.End != machine.Failed:
			c.Problem(n.place+".end", "%q: a final state ends as done or as failed", s.End)
		case s.End != "" && (holds || s.Job != nil || s.Jobs != nil || s.Ticket != "" || s.Timers != nil ||
			s.On != nil || s.Entry != nil || s.Exit != nil):
			c.Problem(n.place, "a final state only ends: it has end: and may give an output, and nothing else")
		case s.End == "" && s.Output != nil:
			c.Problem(n.place, "only a final state gives an output: give it end: done or end: failed")
		}
		c.Job(n.place+".job", s.Job, names...)
		for _, job := range sortedNames(s.Jobs) {
			c.Name(n.place+".jobs."+job, job)
			c.Job(n.place+".jobs."+job, s.Jobs[job], names...)
		}
		if s.Ticket != "" {
			c.Name(n.place+".ticket", s.Ticket)
			if reserved(s.Ticket) {
				c.Problem(n.place+".ticket", "a ticket cannot be named %q, which is the name of an event of its own", s.Ticket)
			}
			if other, dup := issued[s.Ticket]; dup {
				c.Problem(n.place+".ticket", "the state %q issues a ticket named %q too: a ticket's name is used once in a statechart", other, s.Ticket)
			}
			issued[s.Ticket] = name
		}
		for _, timer := range sortedNames(s.Timers) {
			at := n.place + ".timers." + timer
			c.Name(at, timer)
			switch {
			case reserved(timer):
				c.Problem(at, "a timer cannot be named %q, which is the name of an event of its own", timer)
			case ticketOf(all, timer) != "":
				c.Problem(at, "%q is the name of the ticket of the state %q, and a timer cannot take a ticket's name", timer, ticketOf(all, timer))
			case !handled(all, name, timer):
				c.Problem(at, "nothing goes anywhere when the timer is due: give it a rule under on:, in this state or one that holds it")
			}
			if machine.HasExpr(s.Timers[timer]) {
				c.Value(at, s.Timers[timer], names...)
			} else if _, err := parseAt(s.Timers[timer]); err != nil {
				c.Problem(at, "%v", err)
			}
		}
		c.Value(n.place+".output", s.Output, names...)
		sends := func(place string, list []Send, leaving bool) {
			for i, send := range list {
				at := fmt.Sprintf("%s[%d]", place, i)
				if machine.HasExpr(send.Event) || send.Event == "" {
					c.Problem(at+".event", "%q: the event a send sends has a name, written out", send.Event)
				}
				c.Value(at+".data", send.Data, names...)
				switch {
				case send.To == machine.Parent:
				case scripts[send.To]:
					c.Problem(at+".to", "%q runs a script, and only a machine can be sent an event", send.To)
				case !children[send.To]:
					c.Problem(at+".to", "%q is not the parent or a child: a send goes to parent, or to the state or named job that started a machine", send.To)
				case leaving && jobState[send.To] == name:
					c.Problem(at+".to", "a send on leaving a state cannot go to that state's own child, which is already being stopped")
				}
			}
		}
		sends(n.place+".entry.send", s.Entry, false)
		sends(n.place+".exit.send", s.Exit, true)
		rule := func(place string, list []Transition) {
			for i, t := range list {
				at := fmt.Sprintf("%s[%d]", place, i)
				if t.Go != "" {
					if _, ok := all[t.Go]; !ok {
						c.Problem(at+".go", "there is no state named %q", t.Go)
					}
				} else if len(t.Send) == 0 {
					c.Problem(at, "a transition names a state to go to, or sends, or both")
				}
				if t.When != "" {
					c.Cond(at+".when", t.When, names...)
				}
				sends(at+".send", t.Send, false)
			}
		}
		for _, event := range sortedNames(s.On) {
			if machine.HasExpr(event) || event == machine.Start || event == machine.Timer {
				c.Problem(n.place+".on."+event, "%q cannot be named under on: it is an event's own name, written out, and start is not one a state is given", event)
			}
			rule(n.place+".on."+event, s.On[event])
			if what, taken, owners := ownersOf(all, event); len(owners) > 0 && !slices.ContainsFunc(owners, func(o string) bool { return within(all, name, o) }) {
				quoted := make([]string, len(owners))
				for i, o := range owners {
					quoted[i] = strconv.Quote(o)
				}
				at, of := strings.Join(quoted, " or "), "the state "
				if len(owners) > 1 {
					of = "the states "
				}
				c.Problem(n.place+".on."+event, "the state %q can never receive %q. %q is %s of %s%s, and only %s arrives under that name, at %s or a state that holds it. "+
					"Put the rule there, or give the event another name", name, event, event, what, of, at, taken, at)
			}
		}
	}
}

// reserved says whether a name is an event's that a timer or a ticket cannot
// take.
func reserved(name string) bool {
	return slices.Contains([]string{machine.Start, machine.Done, machine.Failed, machine.Timer}, name)
}

// ownersOf gives, for a name that is a ticket's or a timer's, which it is,
// what alone arrives under the name, and the states that issue or start it.
func ownersOf(all map[string]*node, name string) (what, taken string, owners []string) {
	var tickets, timers []string
	for _, state := range sortedNames(all) {
		n := all[state].state
		if n == nil {
			continue
		}
		if n.Ticket == name {
			tickets = append(tickets, state)
		} else if _, ok := n.Timers[name]; ok {
			timers = append(timers, state)
		}
	}
	if len(tickets) > 0 {
		return "the ticket", "its answer", tickets
	}
	return "a timer", "the timer itself", timers
}

// ticketOf gives the state that issues a ticket of a name, or nothing.
func ticketOf(all map[string]*node, name string) string {
	for _, state := range sortedNames(all) {
		if n := all[state].state; n != nil && n.Ticket == name {
			return state
		}
	}
	return ""
}

// within says whether a state is another, or is held by it at any depth.
func within(all map[string]*node, holder, name string) bool {
	for ; name != ""; name = all[name].parent {
		if name == holder {
			return true
		}
	}
	return false
}

// handled says whether a state, or one that holds it, has a rule for an
// event.
func handled(all map[string]*node, name, event string) bool {
	for ; name != ""; name = all[name].parent {
		if _, ok := all[name].state.On[event]; ok {
			return true
		}
	}
	return false
}

func checkInitial(c *machine.Checker, place, initial string, states map[string]*State) {
	at := strings.TrimPrefix(place+".initial", ".")
	switch {
	case len(states) == 0 && initial != "":
		c.Problem(at, "initial names a state among those held here, and none are held")
	case len(states) > 1 && initial == "":
		c.Problem(at, "there is more than one state here, so initial has to name the one entered first")
	case initial != "" && states[initial] == nil:
		c.Problem(at, "initial names %q, which is not one of the states held here", initial)
	}
}

// first is the state entered first among those a chart or a state holds.
func first(initial string, states map[string]*State) string {
	if initial != "" {
		return initial
	}
	for name := range states {
		return name
	}
	return ""
}

// when is a time an after rule is taken at: a length of time from entry, or
// a time of day, with a weekday and a zone if it gives them.
type when struct {
	length  time.Duration
	clock   bool
	hour    int
	minute  int
	weekday int // -1 for any day
	zone    *time.Location
}

var weekdays = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6}

// parseAt reads a time as an after rule writes it: 48h, 18:00, mon 09:00,
// 09:00 Europe/London, mon 09:00 Europe/London.
func parseAt(text string) (when, error) {
	bad := fmt.Errorf("%q is not a length of time or a time of day (write it like 48h, 18:00, \"mon 09:00\" or \"09:00 Europe/London\")", text)
	if !strings.Contains(text, ":") {
		d, err := machine.ParseDuration(text)
		if err != nil {
			return when{}, bad
		}
		return when{length: time.Duration(d)}, nil
	}
	w := when{clock: true, weekday: -1, zone: time.UTC}
	parts := strings.Fields(text)
	if day, ok := weekdays[strings.ToLower(parts[0])]; ok && len(parts) > 1 {
		w.weekday, parts = day, parts[1:]
	}
	if len(parts) == 0 || len(parts) > 2 {
		return when{}, bad
	}
	if _, err := fmt.Sscanf(parts[0], "%d:%d", &w.hour, &w.minute); err != nil || len(parts[0]) != 5 ||
		w.hour < 0 || w.hour > 23 || w.minute < 0 || w.minute > 59 {
		return when{}, bad
	}
	if len(parts) == 2 {
		if parts[1] == "Local" || parts[1] == "local" {
			return when{}, fmt.Errorf("%q: a zone has a name, such as Europe/London; \"wherever the controller runs\" is not one", text)
		}
		zone, err := time.LoadLocation(parts[1])
		if err != nil {
			return when{}, fmt.Errorf("%q: %q is not a time zone", text, parts[1])
		}
		w.zone = zone
	}
	return w, nil
}

// next is when the time is due for a state entered at a moment: a length
// counts from entry, and a time of day is its next occurrence strictly
// after entry.
func (w when) next(entered time.Time) time.Time {
	if !w.clock {
		return entered.Add(w.length)
	}
	local := entered.In(w.zone)
	for day := 0; day <= 8; day++ {
		y, m, d := local.Date()
		at := time.Date(y, m, d+day, w.hour, w.minute, 0, 0, w.zone)
		if w.weekday >= 0 && int(at.Weekday()) != w.weekday {
			continue
		}
		if h, min, _ := at.Clock(); h != w.hour || min != w.minute {
			// The zone's clock skips this time on this day: it is due at
			// the moment the clock jumps past it.
			at = jump(time.Date(y, m, d+day, 0, 0, 0, 0, w.zone), w.zone)
		}
		if at.After(entered) {
			return at.UTC()
		}
	}
	return entered.Add(24 * time.Hour)
}

// jump finds the moment within the day after from at which the zone's
// clock changes.
func jump(from time.Time, zone *time.Location) time.Time {
	_, start := from.Zone()
	lo, hi := from, from.Add(30*time.Hour)
	for hi.Sub(lo) > time.Second {
		mid := lo.Add(hi.Sub(lo) / 2)
		if _, offset := mid.In(zone).Zone(); offset == start {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi.Truncate(time.Second)
}

// jobsOf gives a state's named jobs.
func jobsOf(s *State) []*machine.JobSpec {
	var jobs []*machine.JobSpec
	for _, name := range sortedNames(s.Jobs) {
		jobs = append(jobs, s.Jobs[name])
	}
	return jobs
}

// rulesOf gives every rule a state has.
func rulesOf(s *State) [][]Transition {
	var rules [][]Transition
	for _, name := range sortedNames(s.On) {
		rules = append(rules, s.On[name])
	}
	return rules
}
