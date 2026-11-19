package runs

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// A narrowing is what the address of the Runs page holds: the location
// prefix and the labels it narrows to, and the window of the runs that
// ended.
type narrowing struct {
	At, Window string
	Labels     []string
}

// A window is a length of time the ended runs are listed for.
type window struct {
	Name string
	Long time.Duration
}

var windows = []window{{"1h", time.Hour}, {"6h", 6 * time.Hour}, {"24h", 24 * time.Hour}, {"7d", 7 * 24 * time.Hour}}

func narrowingOf(q url.Values) narrowing {
	n := narrowing{At: q.Get("at"), Window: q.Get("window"), Labels: strings.Fields(q.Get("label"))}
	if !slices.ContainsFunc(windows, func(w window) bool { return w.Name == n.Window }) {
		n.Window = "6h"
	}
	return n
}

func (n narrowing) long() time.Duration {
	for _, w := range windows {
		if w.Name == n.Window {
			return w.Long
		}
	}
	return 6 * time.Hour
}

// with gives the address of the page with one thing changed.
func (n narrowing) with(change func(*narrowing)) string {
	change(&n)
	q := url.Values{}
	set := func(k, v, none string) {
		if v != none {
			q.Set(k, v)
		}
	}
	set("at", n.At, "")
	set("label", strings.Join(n.Labels, " "), "")
	set("window", n.Window, "6h")
	if len(q) == 0 {
		return "/runs"
	}
	return "/runs?" + q.Encode()
}

// LabelText is the labels narrowed to, as they are typed.
func (n narrowing) LabelText() string { return strings.Join(n.Labels, " ") }

func (n narrowing) query() string {
	if a := n.with(func(*narrowing) {}); strings.Contains(a, "?") {
		return a[strings.Index(a, "?"):]
	}
	return ""
}

// keeps says whether a run is one the narrowing leaves: one at the prefix
// or below it, with every label, where a label with a value must match it
// and one with none need only be there, as `corbie ls --label` reads it.
func (n narrowing) keeps(r *client.Run) bool {
	if n.At != "" && !under(r.Location, n.At) {
		return false
	}
	for _, l := range n.Labels {
		name, want, valued := strings.Cut(l, "=")
		got, ok := r.Labels[name]
		if !ok || (valued && got != want) {
			return false
		}
	}
	return true
}

// under says whether a location is a prefix or below it.
func under(location, prefix string) bool {
	return location == prefix || strings.HasPrefix(location, prefix+"/")
}

// listView is the Runs page as it is drawn.
type listView struct {
	Narrowing                     narrowing
	Every                         choice
	Clients                       []clientPlaces
	Here                          *placeHead
	Busy, Idle, Singletons, Ended section
}

type choice struct {
	Name, Href string
	Current    bool
}

// A section is one panel of lines: its part's ID, its name, and the windows
// its head offers.
type section struct {
	ID, Name string
	Lines    []rootLine
	Windows  []choice
}

// clientPlaces is the locations of one client's trees, as Locations lists
// them: the trees the UI knows, then the locations in trees it does not.
type clientPlaces struct {
	Name  string
	Trees []treeView
}

// A treeView is a tree and its locations; a tree with no prefix is the
// locations whose tree the UI does not know, each written in full.
type treeView struct {
	Prefix, Href string
	Current      bool
	Places       []place
}

// A place is one location as Locations lists it: its name from its tree's
// prefix, its lifetime where it is known, and the status of a singleton's
// run or how many of its runs are running.
type place struct {
	Name, Location, Href, Lifetime, Status, Says string
	Current                                      bool
}

// placeHead is the head of a page narrowed to one location, and what it
// offers: Cancel in the UI's client's own tree, Send at a keyed location,
// Start at a transient location of its own tree.
type placeHead struct {
	Location, Lifetime, Tree, Client string
	Cancel                           *page.Call
	Send, Start                      bool
}

// fixed is what a run has from its start, which never changes: its title,
// the prefix of its tree, its lifetime, whether a call started it, the job
// of its parent's it is, whether a machine above it or it was made while
// running, and the directory last submitted at its tree. The UI reads it
// from the record of a run in a tree of its client.
type fixed struct {
	Title, Tree, Lifetime, Job, Dir string
	Called, Made                    bool
}

// A rootLine is one root run: the chain of running runs from it to the
// deepest, what the deepest waits on, and the rows it unfolds to.
type rootLine struct {
	ID, Status, Failure, Name, Tip, Machine, Type, When, Took string
	// Key is the run's key after its title, at a keyed location.
	Key string
	// Titled says the name is the root's title, which is cut at its end;
	// an address is cut in its middle.
	Titled bool
	Chain  []link
	folded
	Shown, Waits string
	Ended        bool
	Under        int
	Rows         []row
	// More is how many of the runs it started are not rows, and are
	// listed on its page.
	More int
}

// folded is a chain as a line shows it: a chain of more than three is its
// first, an ellipsis holding the whole chain, and its last.
type folded struct {
	Front, Back []link
	Between     string
}

func fold(chain []link) folded {
	if len(chain) <= 3 {
		return folded{Front: chain}
	}
	var names []string
	for _, c := range chain {
		names = append(names, c.Machine)
	}
	return folded{Front: chain[:1], Between: strings.Join(names, " › "), Back: chain[len(chain)-1:]}
}

// A row is one run a root started, as its line unfolds: its name, the chain
// below it, what it ended with or what the deepest waits on, how many runs
// are under it, when it started and how long it took. The root's own row,
// where it runs, comes first.
type row struct {
	ID, Status, Failure, Name, Key, Tip, Machine, Type string
	Own                                                bool
	folded
	Shown, Says, When, Took string
	Under                   int
}

// most is how many runs a root started are rows of its line.
const most = 20

// nameOf is how the UI names a run wherever it lists it: by its title, by
// its address, written from the prefix at, where it has none, and by its
// machine where it has neither.
func nameOf(r *client.Run, at string) string { return cmp.Or(r.Title, addressOf(r, at), r.Machine) }

// keyOf gives the key a run's name is followed by: #key for a run at a
// keyed location named by its title, whose name does not give its key.
func keyOf(r *client.Run) string {
	if r.Title == "" || r.Key == "" {
		return ""
	}
	return "#" + r.Key
}

// addressOf gives a run's address as the command line writes it: its
// location, with #key at a keyed location, written from the prefix at. A
// location the prefix names in full keeps its last name. A child that
// machine: started has no address of its own.
func addressOf(r *client.Run, at string) string {
	if r.Location == "" || r.Parent != "" {
		return ""
	}
	loc := r.Location
	switch {
	case at != "" && strings.HasPrefix(loc, at+"/"):
		loc = loc[len(at)+1:]
	case at != "" && loc == at:
		loc = path.Base(loc)
	}
	if r.Key != "" {
		loc += "#" + r.Key
	}
	return loc
}

// tipOf is the tooltip of a run's name: its title where it has one, its
// full address, or a child's location, and its ID.
func tipOf(r *client.Run) string {
	var tip []string
	if r.Title != "" {
		tip = append(tip, r.Title)
	}
	if a := cmp.Or(addressOf(r, ""), r.Location); a != "" {
		tip = append(tip, a)
	}
	return strings.Join(append(tip, r.ID), "\n")
}

// listOf gives the Runs page from the runs, the latest step of each run in
// a tree of the UI's client, mine, that the page shows, and what is fixed of
// each of those runs.
func listOf(runs []*client.Run, stepOf func(*client.Run) *instance.Step, facts map[string]fixed, n narrowing, at time.Time, mine string) listView {
	v := listView{Narrowing: n,
		Busy:       section{ID: "busy", Name: "Busy"},
		Idle:       section{ID: "idle", Name: "Idle"},
		Singletons: section{ID: "singletons", Name: "Singletons"},
		Ended:      section{ID: "ended", Name: "Ended"},
	}
	byID := map[string]*client.Run{}
	under := map[string][]*client.Run{}
	for _, r := range runs {
		byID[r.ID] = r
		under[r.Parent] = append(under[r.Parent], r)
	}
	for _, kids := range under {
		slices.SortFunc(kids, func(a, b *client.Run) int { return strings.Compare(a.ID, b.ID) })
	}
	v.Every = choice{Name: "every location", Href: n.with(func(m *narrowing) { m.At = "" }), Current: n.At == ""}
	v.Clients, v.Here = placesOf(runs, facts, n, mine)
	for _, w := range windows {
		v.Ended.Windows = append(v.Ended.Windows, choice{Name: w.Name, Href: n.with(func(m *narrowing) { m.Window = w.Name }), Current: n.Window == w.Name})
	}
	from := at.Add(-n.long())
	var roots, ended []*client.Run
	for _, r := range runs {
		parent := byID[r.Parent]
		switch {
		case !n.keeps(r):
		case !r.HasEnded() && (parent == nil || parent.HasEnded()):
			roots = append(roots, r)
		case r.HasEnded() && parent == nil && !r.Ended.Before(from):
			ended = append(ended, r)
		}
	}
	slices.SortFunc(roots, func(a, b *client.Run) int { return strings.Compare(b.ID, a.ID) })
	slices.SortFunc(ended, func(a, b *client.Run) int { return b.Ended.Compare(a.Ended) })

	l := lister{byID: byID, under: under, stepOf: stepOf, from: from, at: at, prefix: n.At}
	for _, r := range roots {
		line := l.line(r)
		line.Chain = descent(r, under, running)
		line.folded = fold(line.Chain)
		line.Shown, line.Waits = l.waits(byID[line.Chain[len(line.Chain)-1].ID])
		switch {
		case facts[r.ID].Lifetime == machine.LifetimeSingleton:
			v.Singletons.Lines = append(v.Singletons.Lines, line)
		case r.Status == instance.Waiting || r.Jobs > 0 || len(r.Tickets) > 0 || len(running(under[r.ID])) > 0:
			v.Busy.Lines = append(v.Busy.Lines, line)
		default:
			v.Idle.Lines = append(v.Idle.Lines, line)
		}
	}
	for _, r := range ended {
		line := l.line(r)
		line.When = page.When(r.Ended, at)
		v.Ended.Lines = append(v.Ended.Lines, line)
	}
	return v
}

// placesOf gives the locations at which the list holds a run, by the client
// whose tree holds each, and the head of a page narrowed to one of them.
func placesOf(runs []*client.Run, facts map[string]fixed, n narrowing, mine string) ([]clientPlaces, *placeHead) {
	type found struct {
		client, tree, lifetime string
		keyed                  bool
		running                int
		newest                 *client.Run
	}
	at := map[string]*found{}
	for _, r := range runs {
		if r.Location == "" {
			continue
		}
		f := at[r.Location]
		if f == nil {
			f = &found{client: store.InstanceClient(r.ID)}
			at[r.Location] = f
		}
		known := facts[r.ID]
		f.tree, f.lifetime = cmp.Or(f.tree, known.Tree), cmp.Or(f.lifetime, known.Lifetime)
		f.keyed = f.keyed || r.Key != ""
		if r.Parent == "" {
			if !r.HasEnded() {
				f.running++
			}
			if f.newest == nil || r.Started.After(f.newest.Started) {
				f.newest = r
			}
		}
	}
	trees := map[string]map[string][]place{} // by client, then by tree
	var here *placeHead
	for _, loc := range slices.Sorted(maps.Keys(at)) {
		f := at[loc]
		if f.lifetime == "" && f.keyed {
			f.lifetime = machine.LifetimeKeyed
		}
		p := place{Name: loc, Location: loc, Lifetime: f.lifetime, Href: n.with(func(m *narrowing) { m.At = loc }), Current: n.At == loc}
		if f.tree != "" {
			p.Name = strings.TrimPrefix(loc, f.tree+"/")
			if loc == f.tree {
				p.Name = path.Base(loc)
			}
		}
		switch {
		case f.lifetime == machine.LifetimeSingleton && f.newest != nil:
			p.Status = f.newest.Status
		case f.running > 0:
			p.Says = fmt.Sprintf("%d running", f.running)
		}
		if trees[f.client] == nil {
			trees[f.client] = map[string][]place{}
		}
		trees[f.client][f.tree] = append(trees[f.client][f.tree], p)
		if p.Current {
			here = &placeHead{Location: loc, Lifetime: f.lifetime, Tree: f.tree, Client: f.client}
			own := mine != "" && f.client == mine
			if own {
				cancel := page.Call{Words: []string{"run", "cancel"}, Args: []string{loc}}
				here.Cancel = &cancel
			}
			here.Send = mine != "" && f.lifetime == machine.LifetimeKeyed
			here.Start = own && f.lifetime == machine.LifetimeTransient
		}
	}
	var out []clientPlaces
	for _, c := range slices.Sorted(maps.Keys(trees)) {
		cp := clientPlaces{Name: c}
		// The trees the UI knows come first, by prefix; the locations of
		// trees it does not know, under no prefix, last.
		prefixes := slices.Sorted(maps.Keys(trees[c]))
		if len(prefixes) > 0 && prefixes[0] == "" {
			prefixes = append(prefixes[1:], "")
		}
		for _, prefix := range prefixes {
			t := treeView{Prefix: prefix, Places: trees[c][prefix]}
			if prefix != "" {
				t.Href, t.Current = n.with(func(m *narrowing) { m.At = prefix }), n.At == prefix
			}
			cp.Trees = append(cp.Trees, t)
		}
		out = append(out, cp)
	}
	return out, here
}

// A lister gives the lines of the Runs page from the runs under each run
// and the latest steps of the client's own.
type lister struct {
	byID   map[string]*client.Run
	under  map[string][]*client.Run
	stepOf func(*client.Run) *instance.Step
	from   time.Time
	at     time.Time
	// prefix is the location prefix the page is narrowed to, from which
	// addresses are written.
	prefix string
}

// line gives a root's line, with the rows it unfolds to: its own row where
// it runs, then the runs it started that run or ended in the window,
// newest first, the first twenty of them.
func (l lister) line(r *client.Run) rootLine {
	line := rootLine{ID: r.ID, Status: r.Status, Failure: r.Failure, Name: nameOf(r, l.prefix), Key: keyOf(r), Tip: tipOf(r), Titled: r.Title != "", Machine: r.Machine, Type: r.Type, Ended: r.HasEnded(), Under: l.count(r)}
	if r.HasEnded() {
		line.Took = took(r.Ended.Sub(r.Started))
	} else {
		line.When = page.When(r.Started, l.at)
		own := row{ID: r.ID, Status: r.Status, Failure: r.Failure, Name: r.Machine, Tip: tipOf(r), Machine: r.Machine, Type: r.Type, Own: true, When: line.When}
		own.Shown, own.Says = l.waits(r)
		line.Rows = append(line.Rows, own)
	}
	kids, shown := l.under[r.ID], 0
	for i := len(kids) - 1; i >= 0; i-- {
		switch k := kids[i]; {
		case k.HasEnded() && k.Ended.Before(l.from):
		case shown == most:
			line.More++
		default:
			line.Rows = append(line.Rows, l.row(k))
			shown++
		}
	}
	return line
}

// row gives a run a root started: a running one with the chain of running
// runs below it and what the deepest waits on, an ended one with the chain
// of the newest run at each level below it and what it ended with.
func (l lister) row(r *client.Run) row {
	w := row{ID: r.ID, Status: r.Status, Failure: r.Failure, Name: nameOf(r, l.prefix), Key: keyOf(r), Tip: tipOf(r), Machine: r.Machine, Type: r.Type, Under: l.count(r), When: page.When(r.Started, l.at)}
	if !r.HasEnded() {
		chain := descent(r, l.under, running)
		w.folded = fold(chain)
		w.Shown, w.Says = l.waits(l.byID[chain[len(chain)-1].ID])
		return w
	}
	w.folded = fold(descent(r, l.under, all))
	w.Took = took(r.Ended.Sub(r.Started))
	w.Says = endOf(l.stepOf(r))
	return w
}

// waits gives the states a running run is in and what it waits on, from
// its latest step where it is in a tree of the UI's client.
func (l lister) waits(r *client.Run) (shown, says string) {
	step := l.stepOf(r)
	return shownOf(step), inWords(waitsOf(r, stateOf(step), running(l.under[r.ID]), l.at))
}

// count gives how many runs are under a run.
func (l lister) count(r *client.Run) int {
	n := 0
	for _, k := range l.under[r.ID] {
		n += 1 + l.count(k)
	}
	return n
}

// endOf gives what a run ended with, from its last step: its error, or
// the last event it took, as its record writes it.
func endOf(step *instance.Step) string {
	switch {
	case step == nil:
		return ""
	case step.State.Error != nil:
		return step.State.Error.Error
	case len(step.Events) == 0:
		return ""
	}
	e := step.Events[len(step.Events)-1]
	if from := cmp.Or(senderOf(e.Event), e.From); from != "" {
		return e.Name + " from " + from
	}
	return e.Name
}

// descent gives a run and, at each level below it, the newest of the runs
// kids leaves.
func descent(r *client.Run, under map[string][]*client.Run, kids func([]*client.Run) []*client.Run) []link {
	var chain []link
	for r != nil {
		chain = append(chain, linkTo(r))
		left := kids(under[r.ID])
		r = nil
		if len(left) > 0 {
			r = left[len(left)-1]
		}
	}
	return chain
}

// all gives every run of kids.
func all(kids []*client.Run) []*client.Run { return kids }

// shownOf gives the states or running stages a run is in, or the turn an
// agent is on, from the run's state after its latest step.
func shownOf(step *instance.Step) string {
	if step == nil {
		return ""
	}
	t, ok := machine.TypeOf(step.State.Type)
	if !ok {
		return ""
	}
	shown := t.Show(step.State.Inner)
	names := slices.Clone(shown.States)
	for _, name := range slices.Sorted(maps.Keys(shown.Stages)) {
		if shown.Stages[name] == instance.Running {
			names = append(names, name)
		}
	}
	if shown.Turn > 0 {
		names = append(names, "turn "+strconv.Itoa(shown.Turn))
	}
	return strings.Join(names, ", ")
}

// stateOf gives the state of a step, where there is one.
func stateOf(step *instance.Step) *instance.State {
	if step == nil {
		return nil
	}
	return &step.State
}

// counted is a thing and how many of it there are, where there are more
// than one.
func counted(n int, what string) string {
	if n == 1 {
		return what
	}
	return plural(n, what)
}

// cancelOf is the command that cancels a run in a tree of the UI's client.
func cancelOf(id string) page.Call {
	return page.Call{Words: []string{"run", "cancel"}, Args: []string{id}}
}

// atAct runs the command of an action on a location: Cancel every run
// there, Send to a key, or Start a run. It answers with what the command
// printed: the receipt, or each run cancelled, or the error.
func (h *handler) atAct(w http.ResponseWriter, r *http.Request) {
	at := r.FormValue("at")
	if store.CheckLocation(at) != nil {
		page.Error(w, http.StatusBadRequest, "not a location: "+at)
		return
	}
	if h.Client == nil {
		page.Error(w, http.StatusForbidden, "only a client acts on a location: start the UI as the client")
		return
	}
	var call page.Call
	switch r.PathValue("act") {
	case "cancel":
		call = page.Call{Words: []string{"run", "cancel"}, Args: []string{at}}
	case "send":
		data, err := jsonOf(r.FormValue("data"))
		if err != nil {
			page.Error(w, http.StatusBadRequest, "the data is not JSON: "+err.Error())
			return
		}
		call = page.Call{Words: []string{"run", "send"}, Args: []string{at + "#" + r.FormValue("key"), r.FormValue("event")}}
		if data != "" {
			call.Args = append(call.Args, data)
		}
	case "start":
		input, err := jsonOf(r.FormValue("input"))
		if err != nil {
			page.Error(w, http.StatusBadRequest, "the input is not JSON: "+err.Error())
			return
		}
		call = page.Call{Words: []string{"run", "start"}, Args: []string{at}}
		if input != "" {
			call.Flags = []string{"--input=" + input}
		}
	default:
		page.Error(w, http.StatusNotFound, "there is no such action")
		return
	}
	answer := h.Command(r.Context(), call)
	if answer.Status != 0 {
		page.Error(w, http.StatusConflict, refusal(answer))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, receipt(answer))
}

// steps keeps the latest step of each run of the UI's client that the
// Runs page shows, by the step of its head it was read at: a step never
// changes once it is written.
type steps struct {
	mu   sync.Mutex
	read map[string]stepRead
}

type stepRead struct {
	at   int
	step *instance.Step
}

// A stepReader reads the steps one drawing of the Runs page shows. The
// page is drawn by one reader at a time.
type stepReader struct {
	ctx  context.Context
	c    *client.Client
	s    *steps
	runs []*client.Run
	kept map[string]stepRead
	errs []error
}

// reader reads steps of the runs listed, as one drawing of the page needs.
func (s *steps) reader(ctx context.Context, c *client.Client, runs []*client.Run) *stepReader {
	s.mu.Lock()
	return &stepReader{ctx: ctx, c: c, s: s, runs: runs, kept: map[string]stepRead{}}
}

// of gives the latest step of a run the client started, read again only
// where its head has moved on, and nothing for another client's run.
func (r *stepReader) of(run *client.Run) *instance.Step {
	if r.c == nil || r.c.ID == nil || store.InstanceClient(run.ID) != r.c.ID.Name {
		return nil
	}
	got, ok := r.kept[run.ID]
	if !ok {
		got, ok = r.s.read[run.ID]
	}
	if !ok || got.at != run.Step {
		step, err := instance.Latest(r.ctx, r.c.Store, r.c.Controller, r.c.ID, run.ID, run.Step)
		if err != nil && !errors.Is(err, identity.ErrNotForUs) {
			r.errs = append(r.errs, err)
			return nil
		}
		got = stepRead{at: run.Step, step: step}
	}
	r.kept[run.ID] = got
	return got.step
}

// done keeps the steps of the runs still listed, forgets the rest, and
// gives what could not be read.
func (r *stepReader) done() error {
	for _, run := range r.runs {
		if got, ok := r.s.read[run.ID]; ok && r.kept[run.ID] == (stepRead{}) {
			r.kept[run.ID] = got
		}
	}
	r.s.read = r.kept
	r.s.mu.Unlock()
	return errors.Join(r.errs...)
}

// titles keeps what is fixed of each run in a tree of the UI's client
// that the list holds: it is filled in at the run's first step and never
// changes.
type titles struct {
	mu   sync.Mutex
	read map[string]titleRead
}

// A titleRead is what is fixed of a run, and the step of its head it was
// looked for at. Kept says it was read, and is the run's for good.
type titleRead struct {
	at    int
	fixed fixed
	kept  bool
}

// of gives the runs of a listing, each run of the client's a copy with its
// title, and what is fixed of each, read once for each run. What is fixed
// of runs the listing no longer holds is forgotten.
func (t *titles) of(ctx context.Context, c *client.Client, runs []*client.Run) ([]*client.Run, map[string]fixed, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	kept := map[string]titleRead{}
	facts := map[string]fixed{}
	var errs []error
	var out []*client.Run
	for _, run := range runs {
		if c == nil || c.ID == nil || store.InstanceClient(run.ID) != c.ID.Name {
			out = append(out, run)
			continue
		}
		got, ok := t.read[run.ID]
		if !ok || (!got.kept && got.at != run.Step) {
			f, found, err := fixedOf(ctx, c, run)
			if err != nil {
				errs = append(errs, err)
				out = append(out, run)
				continue
			}
			got = titleRead{at: run.Step, fixed: f, kept: found}
		}
		kept[run.ID] = got
		facts[run.ID] = got.fixed
		titled := *run
		titled.Title = got.fixed.Title
		out = append(out, &titled)
	}
	t.read = kept
	return out, facts, errors.Join(errs...)
}

// fixedOf reads what is fixed of a run from the step its head gives, or
// from its latest step where that one is not there. A run with no step yet
// has not been given its title; one sealed to a key the client no longer
// has shows none.
func fixedOf(ctx context.Context, c *client.Client, run *client.Run) (f fixed, found bool, err error) {
	step, err := instance.ReadStep(ctx, c.Store, c.Controller, c.ID, run.ID, run.Step)
	if err == nil && step == nil {
		step, err = instance.Current(ctx, c.Store, c.Controller, c.ID, run.ID)
	}
	switch {
	case errors.Is(err, identity.ErrNotForUs):
		return f, true, nil
	case err != nil || step == nil:
		return f, false, err
	}
	st := &step.State
	return fixed{Title: st.Title, Tree: st.Tree, Lifetime: st.Lifetime, Job: st.Above.Job, Dir: st.Above.Context,
		Called: st.Above.CallJob != "" || st.Above.CallReceipt != nil, Made: st.Above.Made}, true, nil
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func took(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
