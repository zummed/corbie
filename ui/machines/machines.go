// Package machines is the Machines page: the machines the controller
// recorded, by tree, each drawn and as its definition. It only reads the
// store.
package machines

import (
	"cmp"
	"context"
	"embed"
	"encoding/json/v2"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"
	"go.yaml.in/yaml/v3"
)

// Config is what the Machines page is given: the controller and its store,
// which give the trees, the definition recorded at each location and where
// each secret and value is set, how often the page looks for a change, and
// the shell every page shares.
type Config struct {
	Controller *identity.Identity
	Store      store.Store
	// List is every run whose status the UI may read, as last read, and a
	// signal of each change: a machine's location counts its runs from it.
	List List
	// Client is the client the UI acts as: its running runs are counted in
	// the states and stages of a machine's drawing. None where it acts as
	// no client.
	Client *client.Client
	// Latest gives a run's latest step, which the Runs page reads too.
	Latest   func(ctx context.Context, c *client.Client, run *client.Run) (*instance.Step, error)
	Interval time.Duration
	Shell    page.Shell
}

// A List is every run whose status the UI may read, as last read, and a
// signal of each change.
type List interface {
	Runs() ([]*client.Run, error)
	Changed() <-chan struct{}
}

type handler struct {
	Config
	index *indexed
	// differ keeps how two stored definitions differ, by their references.
	mu     sync.Mutex
	differ map[string][]string
}

//go:embed machines.js shared.js
var files embed.FS

// script serves one of the pages' scripts.
func script(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		data, _ := files.ReadFile(name)
		w.Write(data)
	}
}

// Pages is the Machines page.
type Pages struct {
	http.Handler
	h *handler
}

// New gives the Machines page.
func New(c Config) *Pages {
	h := &handler{Config: c, index: &indexed{store: c.Store, ctl: c.Controller}, differ: map[string][]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /machines", h.show)
	mux.HandleFunc("GET /machines/stream", h.stream)
	mux.HandleFunc("GET /machines/view", h.viewPart)
	mux.HandleFunc("GET /machines/script.js", script("machines.js"))
	mux.HandleFunc("GET /machines/shared.js", script("shared.js"))
	return &Pages{Handler: mux, h: h}
}

// Href gives the address on the Machines page of the machine recorded at a
// location, with the cards named in cards unfolded one inside another. It
// gives none where no machine is recorded there.
func (p *Pages) Href(ctx context.Context, location string, cards []string) (string, error) {
	ix, err := p.h.index.current(ctx)
	if err != nil || ix.machines[location] == nil {
		return "", err
	}
	open := map[string]bool{}
	for i := range cards {
		open[strings.Join(cards[:i+1], "/")] = true
	}
	return machineHref(location, open), nil
}

// machineHref gives the address of the machine at a location, with the
// cards at the paths in open unfolded.
func machineHref(location string, open map[string]bool) string {
	q := url.Values{"at": {location}}
	for _, p := range sortedKeys(open) {
		if open[p] {
			q.Add("open", p)
		}
	}
	return "/machines?" + q.Encode()
}

// view is the machine chosen on the Machines page, as the page draws it:
// its part "view". Open is its location, View the tab chosen, drawing or
// definition, and Stamp the mark of the trees it was read at.
type view struct {
	*index  `json:"-"`
	Open    string
	Kind    string
	First   string
	Stamp   string
	View    string
	Text    []codeLine
	Place   *place
	Uses    []use
	Drawing *drawing.Scene
}

// A place is what a machine is for, where it lives and how its runs start,
// as its definition says: its description, its location, its lifetime, what a
// changed file does to its runs, how many may run at once, who may send to
// it and call it, the locations it may send to and call, and the
// permissions that mark a job for approval.
type place struct {
	Description, Location, Lifetime, Started, Update, Updated string
	Runs                                                      int
	Receives, Sends, Approve                                  []string
}

// A use is a secret or a value a machine uses, and where it is set for the
// machine's location.
type use struct {
	Name, Kind, State string
}

func (h *handler) show(w http.ResponseWriter, r *http.Request) {
	v, status, err := h.machinesPage(r.Context(), r.URL.Query())
	if err != nil {
		page.Error(w, status, err.Error())
		return
	}
	counted, err := h.counted(r.Context(), v.index)
	if err != nil {
		page.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s := h.Shell
	s.Title, s.Current, s.Stream, s.Script = "Machines", "Machines", "/machines/stream", "/machines/script.js"
	page.Write(w, s, MachinesParts{List: v.index, Runs: counted, View: &v})
}

// viewPart answers the part "view" of the Machines page at the address its
// query gives, as JSON: the page asks for it when it shows another tab or
// another card unfolded in place, or when a submit changed the machine.
func (h *handler) viewPart(w http.ResponseWriter, r *http.Request) {
	v, status, err := h.machinesPage(r.Context(), r.URL.Query())
	if err != nil {
		page.Error(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.MarshalWrite(w, v, json.Deterministic(true))
}

// MachinesParts are the parts of the Machines page: the list, the runs at
// each location, and the machine chosen as its tab shows it, which the page
// has as it loads and asks for at /machines/view.
type MachinesParts struct {
	List *index                `json:"list"`
	Runs map[string]atLocation `json:"runs"`
	View *view                 `json:"view,omitzero"`
}

// machinesPage reads the machine the address chooses, by default the first,
// as the tab the address chooses shows it: its drawing, with the cards the
// address names unfolded, or its definition. It gives the status of the
// answer where it cannot.
func (h *handler) machinesPage(ctx context.Context, q url.Values) (view, int, error) {
	ix, err := h.index.current(ctx)
	if err != nil {
		return view{}, http.StatusInternalServerError, err
	}
	v := view{index: ix, Open: q.Get("at"), View: q.Get("view"), Stamp: ix.Stamp}
	for _, t := range ix.Trees {
		if v.Open == "" && len(t.Entries) > 0 {
			v.Open = t.Entries[0].Location
		}
	}
	if v.Open == "" {
		return v, http.StatusOK, nil
	}
	m := ix.machines[v.Open]
	if m == nil {
		return v, http.StatusNotFound, fmt.Errorf("no machine is recorded at %s", v.Open)
	}
	def := m.Draft.Definition
	v.Kind, v.First, v.Place = def.Type, typeAndName(def.Type, def.Name), placeOf(m)
	if v.Uses, err = h.usesOf(ctx, m); err != nil {
		return v, http.StatusInternalServerError, err
	}
	if v.View == "definition" {
		v.Text, err = definitionOf(def)
		if err != nil {
			return v, http.StatusInternalServerError, err
		}
		return v, http.StatusOK, nil
	}
	v.View = "drawing"
	open := map[string]bool{}
	for _, at := range q["open"] {
		open[at] = true
	}
	u := drawing.Unfold{File: m.File, Open: open, Href: hrefs(v.Open, open), Called: func(at string) string { return ix.called(m.Tree, at) }}
	scene, err := drawing.Unfolded(m.Draft, u)
	if err != nil {
		return v, http.StatusInternalServerError, err
	}
	v.Drawing = &scene
	return v, http.StatusOK, nil
}

// placeOf gives where a machine lives and how its runs start.
func placeOf(m *recorded) *place {
	def := m.Draft.Definition
	p := &place{Description: def.Description, Location: m.Location, Lifetime: def.Lifetime, Update: def.Update, Runs: def.Runs, Approve: def.Approve}
	p.Started = startedBy(def.Lifetime, m.Location)
	p.Updated = "a changed file leaves its runs to finish on the definition they started with"
	if def.Update == machine.UpdateRestart {
		p.Updated = "a changed file cancels its runs"
	}
	for _, event := range sortedKeys(def.Receive) {
		from := cmp.Or(strings.Join(def.Receive[event], ", "), "none")
		if event == "" {
			p.Receives = append(p.Receives, "every event from "+from)
		} else {
			p.Receives = append(p.Receives, event+" from "+from)
		}
	}
	tools := calledBy(def)
	for _, entry := range def.Allow {
		if to, ok := strings.CutPrefix(entry, "send:"); ok {
			p.Sends = append(p.Sends, sendsTo(to)+byTools(tools[to]))
			delete(tools, to)
		} else if entry == "all" {
			p.Sends = append(p.Sends, "all (every location)")
		}
	}
	for _, at := range sortedKeys(tools) {
		p.Sends = append(p.Sends, at+byTools(tools[at]))
	}
	return p
}

// calledBy gives the location each tool of an agent written `call:` calls,
// which the tool may send to, with the tools that call it.
func calledBy(def *machine.Definition) map[string][]string {
	var a agent.Agent
	if def.Type != "agent" || json.Unmarshal(def.Body, &a) != nil {
		return nil
	}
	tools := map[string][]string{}
	for _, name := range sortedKeys(a.Tools) {
		if at := a.Tools[name].Call; at != "" {
			tools[at] = append(tools[at], name)
		}
	}
	return tools
}

// byTools gives the note on a location the tools named call, or nothing
// where none does.
func byTools(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return " (by the tool " + names[0] + ")"
	}
	return " (by the tools " + strings.Join(names, ", ") + ")"
}

// sendsTo gives a send: entry as written, with what a pattern of every
// location means.
func sendsTo(to string) string {
	switch to {
	case "**":
		return "** (every location in its tree)"
	case "/**":
		return "/** (every location)"
	}
	return to
}

// usesOf gives the secrets and the values a machine uses, as Corbie reads
// them from its definition, and where each is set: at the machine's
// location or the nearest location above it.
func (h *handler) usesOf(ctx context.Context, m *recorded) ([]use, error) {
	var out []use
	for _, name := range secretsOf(m.Draft) {
		out = append(out, use{Name: name, Kind: "secret"})
	}
	for _, name := range m.Draft.Values() {
		out = append(out, use{Name: name, Kind: "value"})
	}
	if len(out) == 0 {
		return out, nil
	}
	secrets, err := secret.List(ctx, h.Store, h.Controller.Name)
	if err != nil {
		return nil, err
	}
	values, err := secret.ListValues(ctx, h.Store, h.Controller)
	if err != nil {
		return nil, err
	}
	for i, u := range out {
		set := secrets
		if u.Kind == "value" {
			set = values
		}
		out[i].State = "not set at " + m.Location + " or above"
		if at := nearest(set, u.Name, m.Location); at != "" {
			out[i].State = "set at " + at
		}
	}
	return out, nil
}

// secretsOf gives the names of the secrets a draft's jobs and its child
// definitions name, in order.
func secretsOf(d *machine.Draft) []string {
	all := map[string]bool{}
	var add func(d *machine.Draft)
	add = func(d *machine.Draft) {
		allow := slices.Clone(d.Definition.Allow)
		for _, job := range machine.Jobs(d.Definition) {
			allow = append(allow, job.Job.Allow...)
		}
		for _, name := range secret.Named(allow) {
			all[name] = true
		}
		for _, child := range d.Children {
			add(child)
		}
	}
	add(d)
	return sortedKeys(all)
}

// nearest gives the location nearest to a location, at it or above it,
// where a name is set, or nothing.
func nearest(set []secret.Entry, name, location string) string {
	best := ""
	for _, e := range set {
		above := e.At == "/" || e.At == location || strings.HasPrefix(location, e.At+"/")
		if e.Name == name && above && len(e.At) > len(best) {
			best = e.At
		}
	}
	return best
}

// hrefs gives the address of a card's link: the page with the card unfolded,
// or, when it is, with it and what is inside it folded.
func hrefs(location string, open map[string]bool) func(string) string {
	return func(at string) string {
		next := map[string]bool{at: !open[at]}
		for p := range open {
			if p != at && !(open[at] && strings.HasPrefix(p, at+"/")) {
				next[p] = open[p]
			}
		}
		return machineHref(location, next)
	}
}

// stream sends the page the list as the store now holds it, whatever
// machine the page shows, and the runs at each location, at once when the
// list of runs changes.
func (h *handler) stream(w http.ResponseWriter, r *http.Request) {
	never := make(chan struct{})
	changed := func() <-chan struct{} { return never }
	if h.List != nil {
		changed = h.List.Changed
	}
	page.Stream(w, r, h.Interval, changed, func(ctx context.Context) (any, error) {
		ix, err := h.index.current(ctx)
		if err != nil {
			return nil, err
		}
		counted, err := h.counted(ctx, ix)
		if err != nil {
			return nil, err
		}
		return MachinesParts{List: ix, Runs: counted}, nil
	})
}

// A codeLine is one line of a definition as text, in spans, and the file
// and line its value was submitted from, as file:line.
type codeLine struct {
	Spans []span
	From  string
}

// A span is part of a line: a key, an expression, a comment, or plain.
type span struct {
	Class, Text string
}

var (
	keyAt   = regexp.MustCompile(`^(\s*(?:-\s+)?)([A-Za-z_][\w.-]*)(:)(\s|$)`)
	exprIn  = regexp.MustCompile(`\$\{[^}]*\}`)
	comment = regexp.MustCompile(`(^|\s)#.*$`)
)

// colourLine gives a line's keys, expressions and comment set apart.
func colourLine(line string) codeLine {
	var l codeLine
	rest, note := line, ""
	if at := comment.FindStringIndex(line); at != nil {
		if line[at[0]] != '#' {
			at[0]++
		}
		rest, note = line[:at[0]], line[at[0]:]
	}
	if m := keyAt.FindStringSubmatchIndex(rest); m != nil {
		l.Spans = append(l.Spans, span{Text: rest[:m[3]]}, span{Class: "k", Text: rest[m[4]:m[5]]})
		rest = rest[m[5]:]
	}
	for {
		at := exprIn.FindStringIndex(rest)
		if at == nil {
			break
		}
		l.Spans = append(l.Spans, span{Text: rest[:at[0]]}, span{Class: "x", Text: rest[at[0]:at[1]]})
		rest = rest[at[1]:]
	}
	l.Spans = append(l.Spans, span{Text: rest})
	if note != "" {
		l.Spans = append(l.Spans, span{Class: "c", Text: note})
	}
	return l
}

// commentAt is the column at which a value's file and line are written.
const commentAt = 45

// definitionOf gives a definition as it is recorded: its type and name, then
// its fields by name, the type's own among them, each value with the file
// and line it was submitted from.
func definitionOf(def *machine.Definition) ([]codeLine, error) {
	d := *def
	d.Scripts, d.Machines, d.Lines = nil, nil, nil
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	body, _ := fields["body"].(map[string]any)
	for _, name := range []string{"type", "name", "body"} {
		delete(fields, name)
	}
	maps.Copy(fields, body)
	w := &writing{lines: def.Lines}
	w.scalar(def.Type+":", def.Name, def.Type, "")
	w.mapping(fields, "", "", "")
	return w.out, nil
}

// A writing writes a definition's values, a line at a time.
type writing struct {
	lines map[string]string
	out   []codeLine
}

// line writes one line, with the file and line of the value at a place.
// A value a short form was filled out to has the line of the short form:
// the nearest place above its own that has one.
func (w *writing) line(text, place string) {
	l := colourLine(text)
	for at := place; at != ""; {
		if from := w.lines[at]; from != "" {
			pad := strings.Repeat(" ", max(1, commentAt-len([]rune(text))))
			l.Spans = append(l.Spans, span{Text: pad}, span{Class: "c", Text: "# " + from})
			l.From = from
			break
		}
		at = at[:max(strings.LastIndexAny(at, ".["), 0)]
	}
	w.out = append(w.out, l)
}

// mapping writes a mapping's fields by name at an indent. The first is
// written after first, which stands in for the indent.
func (w *writing) mapping(m map[string]any, place, indent, first string) {
	for i, k := range sortedKeys(m) {
		lead, at := indent, k
		if i == 0 {
			lead = first
		}
		if place != "" {
			at = place + "." + k
		}
		w.value(lead+k+":", m[k], at, indent)
	}
}

// value writes a value after what leads it, at an indent.
func (w *writing) value(lead string, v any, place, indent string) {
	switch v := v.(type) {
	case map[string]any:
		if len(v) == 0 {
			w.line(lead+" {}", place)
			return
		}
		w.line(lead, "")
		w.mapping(v, place, indent+"  ", indent+"  ")
	case []any:
		if len(v) == 0 {
			w.line(lead+" []", place)
			return
		}
		w.line(lead, "")
		for i, item := range v {
			at := fmt.Sprintf("%s[%d]", place, i)
			if m, ok := item.(map[string]any); ok && len(m) > 0 {
				w.mapping(m, at, indent+"    ", indent+"  - ")
			} else {
				w.value(indent+"  -", item, at, indent+"  ")
			}
		}
	default:
		w.scalar(lead, v, place, indent)
	}
}

// scalar writes a value that is no mapping or list as YAML writes it, a
// text of several lines as a block under its first line.
func (w *writing) scalar(lead string, v any, place, indent string) {
	text, _ := yaml.Marshal(v)
	head, rest, _ := strings.Cut(strings.TrimSuffix(string(text), "\n"), "\n")
	w.line(lead+" "+head, place)
	if rest == "" {
		return
	}
	for _, line := range strings.Split(rest, "\n") {
		if line = strings.TrimPrefix(line, "    "); line != "" {
			line = indent + "  " + line
		}
		w.out = append(w.out, codeLine{Spans: []span{{Text: line}}})
	}
}
