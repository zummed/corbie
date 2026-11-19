package runs

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// runParts gives the parts of a run's page that follow the run: Steps only
// for a run in a tree of the UI's client. The drawing and the details
// follow the row selected, which the page asks for.
func runParts(v runView) RunParts {
	r := v.Run
	parts := RunParts{
		Page: PagePart{ID: r.ID, Mine: v.Mine},
		Summary: SummaryPart{ID: r.ID, Machine: r.Machine, Type: r.Type, Status: r.Status, Failure: r.Failure,
			Title: r.Title, Location: r.Location, Address: v.Address, At: v.At, Defined: v.Defined,
			Started: v.Started, Ended: v.Ended, Took: v.Took, Cancel: v.Cancel},
		Path:     v.Path,
		Waits:    WaitsPart{ID: r.ID, Ended: r.HasEnded(), Waits: v.Waits, Reply: v.Reply, Send: v.Send, Labels: v.Labels},
		Output:   OutputPart{Ended: r.HasEnded(), Refused: r.Refused, Layer: r.Layer, Fix: r.Fix, Error: r.Error, Output: v.Output, Used: v.Used, Files: v.Files},
		Children: v.Children,
	}
	if v.Mine {
		parts.Steps = &StepsPart{Ended: r.HasEnded(), Rows: v.Rows, Count: v.Count, Dated: v.Dated}
	}
	return parts
}

// RunParts are the parts of a run's page. Steps and Shown are a run's in
// a tree of the UI's client alone.
type RunParts struct {
	Page     PagePart    `json:"page"`
	Summary  SummaryPart `json:"summary"`
	Path     []link      `json:"path"`
	Waits    WaitsPart   `json:"waits"`
	Output   OutputPart  `json:"output"`
	Children []child     `json:"children"`
	Steps    *StepsPart  `json:"steps,omitzero"`
	Shown    *shown      `json:"shown,omitzero"`
}

// PagePart is the run a page is of, and whether it is in a tree of the
// UI's client.
type PagePart struct {
	ID   string
	Mine bool
}

// SummaryPart is the head of a run's page.
type SummaryPart struct {
	ID, Machine, Type, Status, Failure, Title, Location, Address, At string
	Defined                                                          *definedAt
	Started, Ended, Took                                             string
	Cancel                                                           *page.Call
}

// WaitsPart is what a run waits on, what can be sent to it, and its labels.
type WaitsPart struct {
	ID          string
	Ended       bool
	Waits       []wait
	Reply, Send bool
	Labels      []label
}

// OutputPart is how a run ended, its output, what it used and its files.
type OutputPart struct {
	Ended                      bool
	Refused, Layer, Fix, Error string
	Output, Used               string
	Files                      []file
}

// StepsPart is a run's Steps.
type StepsPart struct {
	Ended bool
	Rows  []stepRow
	Count int
	Dated bool
}

// runView is a run's page as it is drawn.
type runView struct {
	Run  *client.Run
	Name string
	// Address is the run's full address, and At the page narrowed to its
	// location.
	Address, At          string
	Mine                 bool
	Started, Ended, Took string
	Path                 []link
	Waits                []wait
	Labels               []label
	Rows                 []stepRow
	Count                int
	Dated                bool
	Output               string
	Used                 string
	Files                []file
	Children             []child
	Cancel               *page.Call
	Send, Reply          bool
	Shown                shown
	Defined              *definedAt
}

// A link is a run by its machine, as the path, the waits and the steps
// show it, with its title and the tooltip of its name. Sent says what of
// the run before it in a path started it: a send, a call, or, where the UI
// cannot read which, a send or a call.
type link struct {
	ID, Machine, Title, Tip, Type, Sent string
	This                                bool
}

// shown is a run as of one row of Steps: its drawing as it was at the
// row's step, and the row's details, with the entry of the record it is
// from. It has no drawing where the row's step cannot be read.
type shown struct {
	Row           *stepRow
	Defined       *definedAt
	Page          string
	Type          string
	Time          string
	Drawing       *drawing.Scene
	Lines         []line
	Input, Output string
}

// A wait is one thing a run waits on, in words: a ticket with its name,
// and its value where the run is in a tree of the UI's client, or a running child
// with its run.
type wait struct {
	Text, Ticket, Value string
	Run                 *link
}

// String is a wait as one line of words.
func (w wait) String() string {
	if w.Run != nil {
		return w.Text + " " + w.Run.Machine
	}
	return w.Text
}

// inWords gives waits as the Runs page writes them, in one line.
func inWords(waits []wait) string {
	var words []string
	for _, w := range waits {
		words = append(words, w.String())
	}
	return strings.Join(words, ", ")
}

type label struct{ Name, Value string }

type file struct {
	Path, Type, Size, Href string
}

// A child is a run another started, as its parent's page lists it.
type child struct {
	ID, Status, Failure, Name, Key, Tip, Machine, Type, When, Took string
	Ended                                                          bool
}

// record is what a run's page has read of its record, as it is read.
type record struct {
	mu      sync.Mutex
	entries []client.Entry
	changed chan struct{}
}

func newRecord() *record { return &record{changed: make(chan struct{})} }

func (r *record) add(e client.Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *record) all() []client.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.entries)
}

func (r *record) Changed() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changed
}

// reading is what a run's page has read of a run in a tree of the UI's client,
// kept while the page is open: its latest step, read again when the run
// takes another.
type reading struct {
	step *instance.Step
}

// read reads a run's latest step. A run with no step yet, or one sealed to
// a key the client no longer has, has none.
func (h *handler) read(ctx context.Context, id string, number int, r *reading) error {
	if r.step != nil && r.step.Number >= number {
		return nil
	}
	step, err := instance.Latest(ctx, h.Client.Store, h.Client.Controller, h.Client.ID, id, number)
	if errors.Is(err, identity.ErrNotForUs) {
		return nil
	}
	if err != nil || step == nil {
		return err
	}
	r.step = step
	return nil
}

// kept is what the run pages have read that never changes: each step of a
// run, and the machine a run was started from with the machines it names.
// It is read again from the store when it grows past a few hundred.
type kept struct {
	mu       sync.Mutex
	steps    map[string]*instance.Step
	machines map[string]*definition
}

// A definition is the machine a run was started from, and the machines it
// names, by the name its jobs give them.
type definition struct {
	def      *machine.Definition
	machines map[string]drawing.Machine
}

// step reads step number of a run, once. A step sealed to a key the
// client no longer has is none.
func (h *handler) step(ctx context.Context, id string, number int) (*instance.Step, error) {
	key := id + "/" + strconv.Itoa(number)
	h.kept.mu.Lock()
	step := h.kept.steps[key]
	h.kept.mu.Unlock()
	if step != nil {
		return step, nil
	}
	step, err := instance.ReadStep(ctx, h.Client.Store, h.Client.Controller, h.Client.ID, id, number)
	if errors.Is(err, identity.ErrNotForUs) {
		return nil, nil
	}
	if err != nil || step == nil {
		return nil, err
	}
	h.kept.mu.Lock()
	defer h.kept.mu.Unlock()
	if h.kept.steps == nil || len(h.kept.steps) > 500 {
		h.kept.steps = map[string]*instance.Step{}
	}
	h.kept.steps[key] = step
	return step, nil
}

// definitionOf reads the machine a run was started from, and the machines
// it names, once.
func (h *handler) definitionOf(ctx context.Context, id string, step *instance.Step) (*definition, error) {
	h.kept.mu.Lock()
	d := h.kept.machines[id]
	h.kept.mu.Unlock()
	if d != nil {
		return d, nil
	}
	ref := step.State.Definition
	def, err := machine.Load(ctx, h.Client.Store, ref)
	if err != nil {
		return nil, err
	}
	d = &definition{def: def, machines: map[string]drawing.Machine{}}
	for name, at := range def.Machines {
		child, err := machine.Load(ctx, h.Client.Store, ref.Beside(at))
		if err != nil {
			return nil, err
		}
		d.machines[name] = drawing.Machine{Type: child.Type, Name: child.Name}
	}
	h.kept.mu.Lock()
	defer h.kept.mu.Unlock()
	if h.kept.machines == nil || len(h.kept.machines) > 200 {
		h.kept.machines = map[string]*definition{}
	}
	h.kept.machines[id] = d
	return d, nil
}

// shownAt gives a run as of one of its rows: its drawing, from its record
// cut at the row's step and the state that step left, and the row's
// details, whose record holds the row's step and the values its run's jobs
// emitted since the step before.
func (h *handler) shownAt(ctx context.Context, page string, r *stepRow, entries []client.Entry, byID map[string]*client.Run, day time.Time) (shown, error) {
	s := shown{Row: r, Page: page, Type: r.Run.Type, Time: r.Time, Input: indentOf(r.In), Output: indentOf(r.Out)}
	cut, own := cutAt(entries, r)
	for _, e := range own {
		s.Lines = append(s.Lines, lines(e, r.Run.Machine, day)...)
	}
	step, err := h.step(ctx, r.Run.ID, r.Step)
	if err != nil || step == nil {
		return s, err
	}
	d, err := h.definitionOf(ctx, r.Run.ID, step)
	if err != nil {
		return s, err
	}
	s.Type = d.def.Type
	if s.Defined, err = h.defined(ctx, r.Run.ID); err != nil {
		return s, err
	}
	var st *instance.State
	if step.State.Status == instance.Running {
		st = &step.State
	}
	marks := marksOf(r.Run.ID, cut, step.State.Status, st, byID)
	marks.Machines = d.machines
	scene, err := drawing.Draw(d.def, marks)
	if err != nil {
		return s, err
	}
	s.Drawing = &scene
	return s, nil
}

// cutAt gives the steps of a row's run up to the row's step, and the
// entries of the row's details: the values the run's jobs emitted since the
// step before, then the step.
func cutAt(entries []client.Entry, r *stepRow) (cut, own []client.Entry) {
	var emits []client.Entry
	for _, e := range entries {
		switch {
		case e.ID != r.Run.ID || e.Step > r.Step:
		case e.Step == 0:
			emits = append(emits, e)
		case e.Step < r.Step:
			cut, emits = append(cut, e), nil
		default:
			cut, own, emits = append(cut, e), append(emits, e), nil
		}
	}
	return cut, own
}

// newest gives the row that happened last, the latest of those at one
// time in the order rows are listed.
func newest(rows []stepRow) *stepRow {
	var last *stepRow
	for i := range rows {
		r := &rows[i]
		if last == nil || !r.At.Before(last.At) {
			last = r
		}
		if r.Group != nil {
			if n := newest(r.Group.Rows); n != nil && !n.At.Before(last.At) {
				last = n
			}
		}
	}
	return last
}

// rowAt finds a row by its key.
func rowAt(rows []stepRow, key string) *stepRow {
	for i := range rows {
		if rows[i].Key == key {
			return &rows[i]
		}
		if g := rows[i].Group; g != nil {
			if r := rowAt(g.Rows, key); r != nil {
				return r
			}
		}
	}
	return nil
}

// dated says whether any row happened on another date than day's, and so
// shows its date.
func dated(rows []stepRow, day time.Time) bool {
	for _, r := range rows {
		if page.Dated(r.At, day) {
			return true
		}
		if r.Group != nil && dated(r.Group.Rows, day) {
			return true
		}
	}
	return false
}

// countRows gives how many rows there are, at every depth.
func countRows(rows []stepRow) int {
	n := len(rows)
	for _, r := range rows {
		if r.Group != nil {
			n += countRows(r.Group.Rows)
		}
	}
	return n
}

// mine says whether a run is in a tree of the UI's client, and so whether
// the UI may read it all.
func (h *handler) mine(id string) bool {
	return h.Client != nil && store.InstanceClient(id) == h.Client.ID.Name
}

// runOf reads what a run's page shows: everything for a run in a tree of
// the UI's client, and what the list says of any other.
func (h *handler) runOf(ctx context.Context, id string, entries []client.Entry, r *reading) (runView, error) {
	at := time.Now()
	v := runView{Mine: h.mine(id)}
	c := h.Lister
	if v.Mine {
		c = h.Client
	}
	run, err := c.Show(ctx, id)
	if err != nil {
		return v, err
	}
	v.Run, v.Name, v.Address = run, nameOf(run, ""), cmp.Or(addressOf(run, ""), run.Location)
	if run.Location != "" {
		v.At = narrowing{At: run.Location, Window: "6h"}.with(func(*narrowing) {})
	}
	v.Started = page.When(run.Started, at)
	if run.HasEnded() {
		v.Ended, v.Took = page.When(run.Ended, at), took(run.Ended.Sub(run.Started))
	}
	for _, name := range slices.Sorted(maps.Keys(run.Labels)) {
		v.Labels = append(v.Labels, label{Name: name, Value: run.Labels[name]})
	}
	runs, facts, _ := h.titled(ctx)
	byID := map[string]*client.Run{}
	under := map[string][]*client.Run{}
	for _, r := range runs {
		byID[r.ID] = r
		under[r.Parent] = append(under[r.Parent], r)
	}
	v.Path = pathOf(run, byID, under, facts)
	for _, r := range under[id] {
		v.Children = append(v.Children, childOf(r, at))
	}
	slices.SortFunc(v.Children, func(a, b child) int { return strings.Compare(a.ID, b.ID) })

	v.Reply = h.Client != nil && !run.HasEnded() && !v.Mine
	v.Send = h.Client != nil && sends(run)
	switch {
	case !run.HasEnded() && v.Mine:
		cancel := cancelOf(id)
		v.Cancel = &cancel
	case !run.HasEnded() && h.Controller:
		v.Cancel = &page.Call{Words: []string{"controller", "cancel"}, Args: []string{id}}
	}
	if !v.Mine {
		v.Waits = waitsOf(run, nil, running(under[id]), at)
		return v, nil
	}

	if err := h.read(ctx, id, run.Step, r); err != nil {
		return v, err
	}
	if v.Defined, err = h.defined(ctx, id); err != nil {
		return v, err
	}
	var st *instance.State
	if r.step != nil && !run.HasEnded() {
		st = &r.step.State
	}
	v.Waits = waitsOf(run, st, running(under[id]), at)
	v.Rows = rowsOf(id, entries, byID, h.mine, at, run.Started)
	v.Count, v.Dated = countRows(v.Rows), dated(v.Rows, run.Started)
	if run.Output != nil {
		data, _ := json.Marshal(run.Output, json.Deterministic(true))
		v.Output = indent(data)
	}
	v.Used = strings.Join(run.Used, " ")
	if !run.HasEnded() && len(run.Tickets) > 0 {
		tickets, err := h.Client.Tickets(ctx, id)
		if err != nil {
			return v, err
		}
		for i, w := range v.Waits {
			if w.Ticket != "" {
				v.Waits[i].Value = tickets[w.Ticket]
			}
		}
	}
	if run.HasEnded() {
		files, err := h.Client.Files(ctx, id)
		if err != nil {
			return v, err
		}
		for _, p := range slices.Sorted(maps.Keys(files)) {
			f := files[p]
			v.Files = append(v.Files, file{Path: p, Type: f.Type, Size: size(f.Size), Href: "/run/" + id + "/file?path=" + url.QueryEscape(p)})
		}
	}
	return v, nil
}

// sends says whether a run is offered Send: only a running statechart is.
func sends(run *client.Run) bool { return !run.HasEnded() && run.Type == "statechart" }

// pathOf gives the runs from a run's root down to it, by parent, and on
// from it to the deepest running run; before the root, the run whose send
// or call started it.
func pathOf(run *client.Run, byID map[string]*client.Run, under map[string][]*client.Run, facts map[string]fixed) []link {
	path := chainOf(run, under)
	path[0].This = true
	seen := map[string]bool{run.ID: true}
	root := run
	for up := byID[run.Parent]; up != nil && !seen[up.ID]; up = byID[up.Parent] {
		seen[up.ID] = true
		root = up
		path = slices.Insert(path, 0, linkTo(up))
	}
	if by := byID[root.Cause.Run]; by != nil && !seen[by.ID] {
		path[0].Sent = "a send or a call"
		if f, ok := facts[root.ID]; ok {
			path[0].Sent = map[bool]string{false: "a send", true: "a call"}[f.Called]
		}
		path = slices.Insert(path, 0, linkTo(by))
	}
	return path
}

// chainOf gives a run and the running runs below it down to the deepest,
// by the newest running child at each level.
func chainOf(r *client.Run, under map[string][]*client.Run) []link {
	return descent(r, under, running)
}

// running gives the runs among kids that have not ended.
func running(kids []*client.Run) []*client.Run {
	return slices.DeleteFunc(slices.Clone(kids), (*client.Run).HasEnded)
}

// waitsOf gives what a running run waits on: each ticket, each running
// child, each job by its name, and each timer with when it is due. With no
// state, as for a run in another client's tree, its jobs and timers are
// only counted.
func waitsOf(run *client.Run, st *instance.State, kids []*client.Run, at time.Time) []wait {
	if run.HasEnded() {
		return nil
	}
	var waits []wait
	for _, t := range run.Tickets {
		waits = append(waits, wait{Text: "ticket " + t, Ticket: t})
	}
	for _, r := range kids {
		kid := linkTo(r)
		waits = append(waits, wait{Text: "child", Run: &kid})
	}
	if st == nil {
		if run.Jobs > 0 {
			waits = append(waits, wait{Text: counted(run.Jobs, "job")})
		}
		if run.Timers > 0 {
			waits = append(waits, wait{Text: counted(run.Timers, "timer")})
		}
	} else {
		for _, name := range slices.Sorted(maps.Keys(st.Jobs)) {
			switch j := st.Jobs[name]; {
			case j.Held:
				waits = append(waits, wait{Text: "held " + jobOf(j.Name)})
			case j.Runner != "":
				waits = append(waits, wait{Text: "job " + jobOf(j.Name)})
			}
		}
		timers := slices.SortedFunc(maps.Keys(st.Timers), func(a, b string) int {
			return cmp.Or(st.Timers[a].At.Compare(st.Timers[b].At), strings.Compare(a, b))
		})
		for _, name := range timers {
			waits = append(waits, wait{Text: "timer " + jobOf(name) + " due " + page.When(st.Timers[name].At, at)})
		}
	}
	return waits
}

// marksOf gives what a run shows of its state, for its drawing: from the
// steps of its own record up to one step, the status that step left, and
// the jobs going after it.
func marksOf(id string, entries []client.Entry, status string, st *instance.State, byID map[string]*client.Run) *drawing.Run {
	m := &drawing.Run{Ended: status != instance.Running && status != instance.Waiting, Failed: status == instance.Failed, Children: map[string]drawing.Child{}}
	for _, e := range entries {
		if e.ID != id || e.Step == 0 {
			continue
		}
		s := drawing.Step{Shown: e.Shown}
		for _, event := range e.Events {
			s.Events = append(s.Events, event.Name)
			if event.Name == machine.Failed && event.From != "" {
				s.Failed = append(s.Failed, jobOf(event.From))
			}
		}
		for _, a := range e.Asks {
			if a.Kind != instance.AskScript && a.Kind != instance.AskChild && a.Kind != instance.AskCall {
				continue
			}
			name := jobOf(a.Name)
			s.Asked = append(s.Asked, name)
			if r := byID[cmp.Or(a.Child, callee(a))]; r != nil {
				m.Children[name] = drawing.Child{ID: r.ID, Machine: r.Machine, Type: r.Type}
			}
		}
		m.Steps = append(m.Steps, s)
	}
	if st != nil {
		for _, key := range slices.Sorted(maps.Keys(st.Jobs)) {
			j := st.Jobs[key]
			name := jobOf(j.Name)
			m.Going = append(m.Going, name)
			if j.Held {
				m.Held = append(m.Held, name)
			}
		}
	}
	return m
}

// jobOf gives the name of what asked for a job or set a timer, as the
// definition writes it: without what tells its calls, attempts, items and
// states apart.
func jobOf(name string) string {
	if i := strings.IndexAny(name, "#@~[."); i > 0 {
		return name[:i]
	}
	return name
}

// callee gives the run a call job started, where the ask is a call's.
func callee(a instance.Ask) string {
	if a.Kind == instance.AskCall {
		return a.Run
	}
	return ""
}

func childOf(r *client.Run, at time.Time) child {
	c := child{ID: r.ID, Status: r.Status, Failure: r.Failure, Name: nameOf(r, ""), Key: keyOf(r), Tip: tipOf(r), Machine: r.Machine, Type: r.Type, Ended: r.HasEnded()}
	c.When = page.When(r.Started, at)
	if r.HasEnded() {
		c.When, c.Took = page.When(r.Ended, at), took(r.Ended.Sub(r.Started))
	}
	return c
}

// indentOf gives a value as indented JSON, or nothing for none.
func indentOf(v any) string {
	if v == nil {
		return ""
	}
	data, _ := json.Marshal(v, json.Deterministic(true))
	return indent(data)
}

func indent(data []byte) string {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return string(data)
	}
	out, err := json.Marshal(v, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return string(data)
	}
	return string(out)
}

func size(n int64) string {
	switch {
	case n < 1<<10:
		return plural(int(n), "byte")
	case n < 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + " KB"
	}
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
}
