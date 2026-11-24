package runs

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"
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
			Started: v.Started, Ended: v.Ended, Took: v.Took, Cancel: v.Cancel, Why: v.Why, Differs: v.Differs},
		Path:     v.Path,
		Waits:    WaitsPart{ID: r.ID, Ended: r.HasEnded(), Waits: v.Waits, Reply: v.Reply, Send: v.Send, Labels: v.Labels, Timeout: v.Timeout},
		Output:   OutputPart{Ended: r.HasEnded(), Refused: r.Refused, Layer: r.Layer, Fix: r.Fix, Error: r.Error, Output: v.Output, Used: v.Used, Files: v.Files},
		Children: v.Children,
	}
	if v.Mine {
		parts.Steps = &StepsPart{Ended: r.HasEnded(), Rows: v.Rows, Count: v.Count, Dated: v.Dated, Follows: v.Follows}
		parts.Shown = v.Shown
	}
	return parts
}

// RunParts are the parts of a run's page. Steps and Shown, the drawing and
// details of the newest row, are a run's in a tree of the UI's client
// alone.
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

// SummaryPart is the head of a run's page: the run, why it is where it is,
// and how the definition now recorded at its location differs from the one
// it started with.
type SummaryPart struct {
	ID, Machine, Type, Status, Failure, Title, Location, Address, At string
	Defined                                                          *definedAt
	Started, Ended, Took                                             string
	Cancel                                                           *page.Call
	Why                                                              *why
	Differs                                                          []string
}

// A why is why a run is where it is, in one line: what it is doing, since
// when where it waits, and on what, the row it is at, and for a failure
// what refused it and what would fix it.
type why struct {
	Lead  string    // "waiting", or "failed (denied)"
	Since time.Time `json:",omitzero"` // the page says how long ago
	Rest  string    // "on job fetch on runner-a in child review"
	Row   string    // the key of the row it is at
	Facts []fact
}

// WaitsPart is what a run waits on, what can be sent to it, its labels,
// and when it passes its own timeout.
type WaitsPart struct {
	ID          string
	Ended       bool
	Waits       []wait
	Reply, Send bool
	Labels      []label
	Timeout     string
}

// OutputPart is how a run ended, its output, what it used and its files.
type OutputPart struct {
	Ended                      bool
	Refused, Layer, Fix, Error string
	Output, Used               string
	Files                      []file
}

// StepsPart is a run's Steps, and the key of the row the page selects
// while it follows the run: the newest, or a failed run's deepest failure.
type StepsPart struct {
	Ended   bool
	Rows    []stepRow
	Count   int
	Dated   bool
	Follows string
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
	Shown                *shown
	Defined              *definedAt
	Why                  *why
	Differs              []string
	Timeout              string
	Follows              string
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
	// Logs are what the row's job logged, all of it, as it grows.
	Logs []logLine
}

// A logLine is one value a job emitted while it ran, as its run's record
// holds it: when, which value, and the value in words.
type logLine struct {
	Time, Name, Text string
}

// logsOf gives what a job of a run emitted, from the run's record, by the
// job's full name: one attempt of one ask.
func logsOf(entries []client.Entry, id, full string, day time.Time) []logLine {
	var out []logLine
	for _, e := range entries {
		if e.ID != id || e.Step != 0 || e.Job != full {
			continue
		}
		text, _ := e.Data.(string)
		if text == "" {
			data, _ := json.Marshal(e.Data, json.Deterministic(true))
			text = clip(string(data))
		}
		if e.Name == instance.EmitProgress {
			if p, ok := e.Data.(map[string]any); ok {
				text = fmt.Sprint(p["done"])
				if total, ok := p["total"]; ok {
					text += " of " + fmt.Sprint(total)
				}
			}
		}
		out = append(out, logLine{Time: page.Clock(e.Time, day), Name: e.Name, Text: text})
	}
	return out
}

// A wait is one thing a run waits on, in words: a ticket with its name,
// and its value where the run is in a tree of the UI's client, a running
// child with its run, or a job with what is known of it.
type wait struct {
	Text, Ticket, Value string
	Run                 *link
	Job                 *going
}

// going is what a run's state says of a job going: its name, the runner it
// was sent to, which attempt it is, when it was sent, when its result is
// due, when it restarts and how many times it has, and what it has logged.
type going struct {
	Name     string
	Runner   string
	Attempt  int
	Began    time.Time `json:",omitzero"` // the page says how long ago
	Sent     string
	Due      string
	Again    string
	Restarts int
	Logs     []logLine
	full     string // the job's name with its ask's and attempt's numbers
}

// goingOf gives what is known of a job going, its times as the page writes
// them at at.
func goingOf(j *instance.Going, at time.Time) *going {
	g := &going{Name: jobOf(j.Name), Runner: j.Runner, Attempt: j.Attempt, Began: j.Began, Restarts: j.Restarts, full: j.Full()}
	when := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return page.When(t, at)
	}
	g.Sent, g.Due, g.Again = when(j.Began), when(j.Due), when(j.Again)
	return g
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

// shownAt gives a run as of one of its rows: its drawing, and the row's
// details, whose record holds the row's step and the values its run's jobs
// emitted since the step before. The drawing is of the page's run with the
// cards down to the row's run unfolded, each run on the way as of its latest
// step at or before the row's, the row's run as of the row's step. A row of
// a run the page's run did not start is its run's drawing alone. entries
// hold the record of every run drawn.
func (h *handler) shownAt(ctx context.Context, page string, r *stepRow, entries []client.Entry, byID map[string]*client.Run, day time.Time) (shown, error) {
	s := shown{Row: r, Page: page, Type: r.Run.Type, Time: r.Time, Input: indentOf(r.In), Output: indentOf(r.Out)}
	if r.Full != "" {
		s.Logs = logsOf(entries, r.Run.ID, r.Full, day)
	}
	_, own := cutAt(entries, r)
	for _, e := range own {
		s.Lines = append(s.Lines, lines(e, r.Run.Machine, day)...)
	}
	var err error
	if s.Defined, err = h.defined(ctx, r.Run.ID); err != nil {
		return s, err
	}
	chain := chainDown(page, r.Run.ID, byID)
	var inner *drawing.Scene
	for i := len(chain) - 1; i >= 0; i-- {
		id, number := chain[i], r.Step
		if id != r.Run.ID {
			number = stepAt(entries, id, r.At)
		}
		scene, kind, err := h.sceneAt(ctx, id, number, entries, byID, chainNext(chain, i), inner)
		if err != nil || scene == nil {
			return s, err
		}
		if id == r.Run.ID {
			s.Type = kind
		}
		inner = scene
	}
	s.Drawing = inner
	return s, nil
}

// chainDown gives the runs from the page's run down to a run it started,
// by parent, or the run alone where the page's run did not start it.
func chainDown(page, id string, byID map[string]*client.Run) []string {
	chain := []string{id}
	for r := byID[id]; r != nil && r.ID != page; r = byID[r.Parent] {
		if r.Parent == "" {
			return []string{id}
		}
		chain = append([]string{r.Parent}, chain...)
	}
	return chain
}

// chainNext gives the run after the i'th in a chain, or none at its end.
func chainNext(chain []string, i int) string {
	if i+1 < len(chain) {
		return chain[i+1]
	}
	return ""
}

// stepAt gives a run's latest step at or before a time, or its first.
func stepAt(entries []client.Entry, id string, at time.Time) int {
	n := 1
	for _, e := range entries {
		if e.ID == id && e.Step > 0 && !e.Time.After(at) {
			n = max(n, e.Step)
		}
	}
	return n
}

// sceneAt gives a run's drawing as of one of its steps, marked from its
// record up to that step and the state the step left, with a child's
// drawing inside the card of the job that started it, and the type of the
// run's machine. A step sealed to a key the client no longer has gives no
// drawing.
func (h *handler) sceneAt(ctx context.Context, id string, number int, entries []client.Entry, byID map[string]*client.Run, child string, inner *drawing.Scene) (*drawing.Scene, string, error) {
	step, err := h.step(ctx, id, number)
	if err != nil || step == nil {
		return nil, "", err
	}
	d, err := h.definitionOf(ctx, id, step)
	if err != nil {
		return nil, "", err
	}
	var upTo []client.Entry
	for _, e := range entries {
		if e.ID == id && e.Step > 0 && e.Step <= number {
			upTo = append(upTo, e)
		}
	}
	var st *instance.State
	if step.State.Status == instance.Running {
		st = &step.State
	}
	marks := marksOf(id, upTo, step.State.Status, st, byID)
	marks.Machines = d.machines
	for name, c := range marks.Children {
		if c.ID == child && inner != nil {
			marks.In = map[string]drawing.Scene{name: *inner}
		}
	}
	scene, err := drawing.Draw(d.def, marks)
	return &scene, d.def.Type, err
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
	for _, w := range v.Waits {
		if w.Job != nil {
			w.Job.Logs = last(logsOf(entries, id, w.Job.full, run.Started), 20)
		}
	}
	if st != nil && !st.TimeoutAt.IsZero() {
		v.Timeout = page.When(st.TimeoutAt, at)
	}
	v.Rows = rowsOf(id, entries, byID, h.mine, at, run.Started)
	v.Count, v.Dated = countRows(v.Rows), dated(v.Rows, run.Started)
	if v.Why, err = h.whyOf(ctx, run, st, v.Rows, byID, under, at); err != nil {
		return v, err
	}
	switch {
	case v.Why != nil && run.Status == instance.Failed && v.Why.Row != "":
		v.Follows = v.Why.Row
	case newest(v.Rows) != nil:
		v.Follows = newest(v.Rows).Key
	}
	if v.Differs, err = h.differs(ctx, id); err != nil {
		return v, err
	}
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

// last gives the last n of a list.
func last[T any](list []T, n int) []T {
	return list[max(0, len(list)-n):]
}

// whyOf gives why a run is where it is: for a run that failed, the deepest
// row that failed, and for a running run, what the deepest run running under
// it waits on, and since when.
func (h *handler) whyOf(ctx context.Context, run *client.Run, st *instance.State, rows []stepRow, byID map[string]*client.Run, under map[string][]*client.Run, at time.Time) (*why, error) {
	switch {
	case run.Status == instance.Failed:
		w := &why{Lead: run.Status}
		if run.Failure != "" {
			w.Lead += " (" + run.Failure + ")"
		}
		row := deepestFailure(rows)
		if row == nil {
			if run.Error != "" {
				w.Facts = []fact{{Name: "error", Text: run.Error}}
			}
			return w, nil
		}
		w.Row = row.Key
		if row.Job != "" {
			w.Rest = "at " + jobWord(row.Run.Type, row.Job)
		}
		if row.Run.ID != run.ID {
			w.Rest = strings.TrimSpace(w.Rest + " in " + row.Run.Type + " " + row.Run.Machine)
		}
		for _, f := range row.Facts {
			switch f.Name {
			case "error", "refused", "by", "fix":
				w.Facts = append(w.Facts, f)
			}
		}
		// A job that started a child gives its timeout, and how the child
		// ended.
		if started := startedBy(rows, row); started != nil {
			for _, f := range started.Facts {
				if f.Name == "timeout" {
					w.Facts = append(w.Facts, f)
				}
			}
			g := started.Group
			child := g.Run.Type + " " + g.Run.Machine + " " + g.Status
			if g.Failure != "" {
				child += " (" + g.Failure + ")"
			}
			w.Facts = append(w.Facts, fact{Name: "child", Text: child, Run: &g.Run})
		}
		if run.Refused != "" && !slices.ContainsFunc(w.Facts, func(f fact) bool { return f.Name == "refused" }) {
			w.Facts = append(w.Facts, fact{Name: "refused", Text: run.Refused}, fact{Name: "by", Text: run.Layer}, fact{Name: "fix", Text: run.Fix})
		}
		return w, nil
	case run.HasEnded():
		return nil, nil
	}
	chain := descent(run, under, running)
	deep, r := chain[len(chain)-1], run
	if deep.ID != run.ID {
		r = byID[deep.ID]
		step, err := h.Steps.Latest(ctx, h.Client, r)
		if err != nil {
			return nil, err
		}
		st = nil
		if step != nil {
			st = &step.State
		}
	}
	w := &why{Lead: r.Status, Row: newestOf(rows, deep.ID)}
	waits := waitsOf(r, st, nil, at)
	pick := -1
	for _, rank := range []func(wait) bool{
		func(x wait) bool { return x.Job != nil && x.Job.Runner != "" },
		func(x wait) bool { return x.Job != nil },
		func(x wait) bool { return x.Ticket != "" },
		func(wait) bool { return true },
	} {
		if pick = slices.IndexFunc(waits, rank); pick >= 0 {
			break
		}
	}
	switch {
	case pick >= 0:
		x := waits[pick]
		w.Lead, w.Rest = "waiting", "on "+x.Text
		if x.Job != nil {
			w.Since = x.Job.Began
			if x.Job.Runner != "" {
				w.Rest += " on " + x.Job.Runner
			}
		}
	case r.Status == instance.Running && st != nil && r.Type == "statechart":
		// A statechart with nothing going waits for an event in its states.
		w.Lead, w.Rest = "waiting", "on an event in "+shownOf(&instance.Step{State: *st})
	}
	if deep.ID != run.ID {
		w.Rest = strings.TrimSpace(w.Rest + " in " + deep.Type + " " + deep.Machine)
	}
	return w, nil
}

// jobWord names a job as its machine's type does: a flow's stage, a
// statechart's state.
func jobWord(kind, job string) string {
	switch {
	case job == "":
		return ""
	case kind == "flow":
		return "stage " + job
	case kind == "statechart":
		return "state " + job
	}
	return "job " + job
}

// deepestFailure gives the row that failed deepest in the runs under a
// run: a job's failure before a run's end at any depth, as it is what
// failed the run above it; then the deepest; then the latest.
func deepestFailure(rows []stepRow) *stepRow {
	var found *stepRow
	before := func(r *stepRow) bool {
		switch {
		case found == nil:
			return true
		case (r.Job != "") != (found.Job != ""):
			return r.Job != ""
		case r.Depth != found.Depth:
			return r.Depth > found.Depth
		}
		return !r.At.Before(found.At)
	}
	for i := range rows {
		r := &rows[i]
		if strings.Contains(r.Kind, "failed") && before(r) {
			found = r
		}
		if r.Group != nil {
			if g := deepestFailure(r.Group.Rows); g != nil && before(g) {
				found = g
			}
		}
	}
	return found
}

// startedBy gives the row at which a row's job started a child, where it
// did: the row of the same job and run before it that holds a group.
func startedBy(rows []stepRow, row *stepRow) *stepRow {
	var found *stepRow
	var walk func([]stepRow)
	walk = func(rows []stepRow) {
		for i := range rows {
			r := &rows[i]
			if r.Group != nil && r.Run.ID == row.Run.ID && r.Job == row.Job && !r.At.After(row.At) {
				found = r
			}
			if r.Group != nil {
				walk(r.Group.Rows)
			}
		}
	}
	walk(rows)
	return found
}

// newestOf gives the key of a run's newest row among rows, or none.
func newestOf(rows []stepRow, id string) string {
	var found *stepRow
	var walk func([]stepRow)
	walk = func(rows []stepRow) {
		for i := range rows {
			r := &rows[i]
			if r.Run.ID == id && (found == nil || !r.At.Before(found.At)) {
				found = r
			}
			if r.Group != nil {
				walk(r.Group.Rows)
			}
		}
	}
	walk(rows)
	if found == nil {
		return ""
	}
	return found.Key
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
				waits = append(waits, wait{Text: "held " + jobOf(j.Name), Job: goingOf(j, at)})
			case j.Call != nil:
				to := j.Call.At
				if j.Call.Key != nil {
					to += "#" + *j.Call.Key
				}
				waits = append(waits, wait{Text: "call " + to, Job: goingOf(j, at)})
			case j.Runner != "" || !j.Again.IsZero():
				waits = append(waits, wait{Text: "job " + jobOf(j.Name), Job: goingOf(j, at)})
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
			if a.Job != nil && a.Job.Hook != "" {
				name = drawing.HookJob(a.Job.Hook)
			}
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
			if j.Hook != "" {
				name = drawing.HookJob(j.Hook)
			}
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
