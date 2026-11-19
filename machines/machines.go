// Package machines is the Machines page: the machines in one directory, each
// drawn and shown as text, with the problems Corbie finds in them. The
// Toolkits page shows the same directory from the tools up. Both only read.
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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie-ui/controller"
	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// Config is what the Machines page is given: the directory of machines, the
// controller and its store when the UI acts as the controller, which give
// where each secret and value is set, how often the page looks for a
// change, and the shell every page shares.
type Config struct {
	Dir        string
	Controller *identity.Identity
	Store      store.Store
	// Runs says the UI offers the Runs page, to which a machine's location
	// links.
	Runs bool
	// List is every run whose status the UI may read, as last read, and a
	// signal of each change: a machine's location counts its runs from it.
	// None where the UI has no store.
	List List
	// Client is the client the UI acts as: its running runs are counted in
	// the states and stages of a machine's drawing, and it reads the
	// definition a run started with. None where it acts as no client.
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
	dir   *dir
	index *indexed
}

//go:embed machines.js toolkits.js shared.js
var files embed.FS

// script serves one of the pages' scripts.
func script(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		data, _ := files.ReadFile(name)
		w.Write(data)
	}
}

// Pages is the Machines and Toolkits pages.
type Pages struct {
	http.Handler
	h *handler
}

// New gives the Machines and Toolkits pages.
func New(c Config) (*Pages, error) {
	d, err := newDir(c.Dir)
	if err != nil {
		return nil, err
	}
	h := &handler{Config: c, dir: d, index: &indexed{dir: d}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /machines", h.show)
	mux.HandleFunc("GET /machines/stream", h.stream)
	mux.HandleFunc("GET /machines/view", h.viewPart)
	mux.HandleFunc("GET /toolkits", h.showToolkits)
	mux.HandleFunc("GET /toolkits/stream", h.streamToolkits)
	mux.HandleFunc("GET /toolkits/view", h.toolkitPart)
	mux.HandleFunc("GET /machines/script.js", script("machines.js"))
	mux.HandleFunc("GET /toolkits/script.js", script("toolkits.js"))
	mux.HandleFunc("GET /machines/shared.js", script("shared.js"))
	return &Pages{Handler: mux, h: h}, nil
}

// Href gives the address on the Machines page of the machine at a location
// of a tree submitted from dir, with the cards named in cards unfolded one
// inside another, and the machine's file. It gives none where the page does
// not show that machine: a tree submitted from another directory, or a
// location the directory, at the prefix the page reads it at, has no
// machine at.
func (p *Pages) Href(ctx context.Context, location, dir string, cards []string) (href, file string, err error) {
	if dir != p.h.dir.real {
		return "", "", nil
	}
	ix, err := p.h.tree(ctx)
	if err != nil {
		return "", "", err
	}
	for _, m := range ix.machines {
		if m.Location != location {
			continue
		}
		q := url.Values{"file": {m.Path}}
		for i := range cards {
			q.Add("open", strings.Join(cards[:i+1], "/"))
		}
		return "/machines?" + q.Encode(), m.Path, nil
	}
	return "", "", nil
}

// view is the file chosen on the Machines page, as the page draws it: its
// part "view". View is the tab chosen: drawing, text or written. File is the
// file chosen where it is no machine and is listed in .corbieignore or
// included. Stamp is the mark of the directory it was read at. Facts is
// what the file's head says it is.
type view struct {
	*index     `json:"-"`
	Open       string
	Kind       string
	First      string
	Facts      string
	Stamp      string
	View       string
	Drawable   bool
	Text       []codeLine
	Found      []problem
	Place      *place
	Uses       []use
	File       *entry
	Included   []string
	Controller bool
	Runs       bool
	Drawing    *drawing.Scene
}

// list is the list of the Machines page, as the page draws it: its part
// "files". Dir is the directory, cut short, and DirFull the whole of it.
// The page reads the file it shows again when the list's Stamp differs
// from the view's.
type list struct {
	Dir, DirFull string
	*index
}

// A place is what a machine is for, where it lives and how its runs start,
// as its file says: its description, its location, its lifetime, what a
// changed file does to its runs, how many may run at once, who may send to
// it and call it, the locations it may send to and call, and the
// permissions a job uses only after its ask approves.
type place struct {
	Description, Location, Lifetime, Started, Update, Updated string
	Runs                                                      int
	Receives, Sends, Approve                                  []string
}

// A use is a secret or a value a machine uses, and, acting as the
// controller, where it is set for the machine's location.
type use struct {
	Name, Kind, State string
}

// tree gives the index of the directory, read at the prefix of the tree
// the controller recorded from it, where the UI acts as the controller and
// it recorded one; at the prefix a submit gives by default otherwise.
func (h *handler) tree(ctx context.Context) (*index, error) {
	if h.Controller == nil {
		return h.index.current(""), nil
	}
	trees, err := controller.Trees(ctx, h.Store, h.Controller)
	if err != nil {
		return nil, err
	}
	at := ""
	for _, t := range trees {
		if t.Directory == h.dir.real {
			at = t.Prefix
			break
		}
	}
	return h.index.current(at), nil
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
	page.Write(w, s, MachinesParts{Files: h.listPart(v.index), Runs: counted, View: &v})
}

// viewPart answers the part "view" of the Machines page at the address its
// query gives, as JSON: the page asks for it when it shows another tab or
// another card unfolded in place, or when the file changed on disk.
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
// each location, and the file chosen as its tab shows it, which the page
// has as it loads and asks for at /machines/view.
type MachinesParts struct {
	Files list                  `json:"files"`
	Runs  map[string]atLocation `json:"runs"`
	View  *view                 `json:"view,omitzero"`
}

// listPart is the part that lists the directory.
func (h *handler) listPart(ix *index) list {
	return list{Dir: short(shortly(h.Dir)), DirFull: shortly(h.Dir), index: ix}
}

// machinesPage reads the file the address chooses, by default the first
// machine, as the tab the address chooses shows it: its drawing, with the
// cards the address names unfolded, its text, or it written out. It gives
// the status of the answer where it cannot.
func (h *handler) machinesPage(ctx context.Context, q url.Values) (view, int, error) {
	ix, err := h.tree(ctx)
	if err != nil {
		return view{}, http.StatusInternalServerError, err
	}
	v := view{index: ix, Open: q.Get("file"), View: q.Get("view"), Stamp: ix.Stamp, Controller: h.Controller != nil, Runs: h.Runs}
	if v.Open == "" && len(ix.Groups) > 0 {
		v.Open = ix.Groups[0].Entries[0].Path
	}
	if v.Open == "" {
		return v, http.StatusOK, nil
	}
	v.Open = path.Clean(v.Open)
	src, err := h.opened(&v)
	if err != nil {
		return v, http.StatusNotFound, err
	}
	m := ix.machines[v.Open]
	v.Drawable = m != nil && m.Draft != nil
	if v.Drawable {
		v.Place = placeOf(m)
		if v.Uses, err = h.usesOf(ctx, m); err != nil {
			return v, http.StatusInternalServerError, err
		}
	}
	v.File = ix.file(v.Open)
	v.Included = ix.included[v.Open]
	v.Facts = cmp.Or(v.First, "a file")
	if v.File != nil && v.File.Listed {
		v.Facts = "listed in .corbieignore"
	}
	switch {
	case v.Drawable && v.View == "written":
		v.Text = writtenOut(m.Draft.WrittenOut())
	case v.Drawable && v.View != "text":
		v.View = "drawing"
		open := map[string]bool{}
		for _, at := range q["open"] {
			open[at] = true
		}
		u := drawing.Unfold{File: v.Open, Open: open, Href: hrefs(v.Open, open), Called: ix.called}
		scene, err := drawing.Unfolded(m.Draft, u)
		if err != nil {
			return v, http.StatusInternalServerError, err
		}
		v.Drawing = &scene
	default:
		v.View, v.Text = "text", colour(src, v.Open, v.Found)
	}
	return v, http.StatusOK, nil
}

// opened reads the file chosen, its type and name, and the problems found
// in it.
func (h *handler) opened(v *view) ([]byte, error) {
	src, _, err := h.dir.read(v.Open)
	if err != nil {
		return nil, err
	}
	v.Found = v.index.at(v.Open)
	if m := v.index.machines[v.Open]; m != nil {
		var name string
		v.Kind, name = typeIn(src)
		if m.Draft != nil {
			v.Kind, name = m.Draft.Definition.Type, m.Draft.Definition.Name
		}
		v.First = cmp.Or(typeAndName(v.Kind, name), "a machine")
	}
	return src, nil
}

// placeOf gives where a machine lives and how its runs start.
func placeOf(m *client.Machine) *place {
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
// them from it written out, and, acting as the controller, where each is
// set: at the machine's location or the nearest location above it.
func (h *handler) usesOf(ctx context.Context, m *client.Machine) ([]use, error) {
	var out []use
	for _, name := range secretsOf(m.Draft) {
		out = append(out, use{Name: name, Kind: "secret"})
	}
	for _, name := range m.Draft.Values() {
		out = append(out, use{Name: name, Kind: "value"})
	}
	if h.Controller == nil || len(out) == 0 {
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
		if d.Definition.Ask != nil {
			allow = append(allow, d.Definition.Ask.Allow...)
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
func hrefs(file string, open map[string]bool) func(string) string {
	return func(at string) string {
		q := url.Values{"file": {file}}
		for _, p := range slices.Sorted(maps.Keys(open)) {
			if !open[at] || (p != at && !strings.HasPrefix(p, at+"/")) {
				q.Add("open", p)
			}
		}
		if !open[at] {
			q.Add("open", at)
		}
		return "/machines?" + q.Encode()
	}
}

// stream sends the page the list as the directory now holds it, whatever
// file the page shows, and the runs at each location, at once when the list
// of runs changes.
func (h *handler) stream(w http.ResponseWriter, r *http.Request) {
	never := make(chan struct{})
	changed := func() <-chan struct{} { return never }
	if h.List != nil {
		changed = h.List.Changed
	}
	page.Stream(w, r, h.Interval, changed, func(ctx context.Context) (any, error) {
		ix, err := h.tree(ctx)
		if err != nil {
			return nil, err
		}
		counted, err := h.counted(ctx, ix)
		if err != nil {
			return nil, err
		}
		return MachinesParts{Files: h.listPart(ix), Runs: counted}, nil
	})
}

// A codeLine is one line of a file as the text view shows it, in spans,
// its anchor, and whether a problem is found at it.
type codeLine struct {
	Spans []span
	ID    string
	Bad   bool
}

// A span is part of a line: a key, an expression, a comment, or plain,
// and where it links to.
type span struct {
	Class, Text, Href string
}

var (
	keyAt   = regexp.MustCompile(`^(\s*(?:-\s+)?)([A-Za-z_][\w.-]*)(:)(\s|$)`)
	exprIn  = regexp.MustCompile(`\$\{[^}]*\}`)
	comment = regexp.MustCompile(`(^|\s)#.*$`)
	lineAt  = regexp.MustCompile(`^(.*\S)(\s+)# (\S+):(\d+)$`)
)

// colour gives a file's lines, its keys, expressions and comments set apart,
// and the lines of the problems found in it marked.
func colour(src []byte, file string, found []problem) []codeLine {
	bad := map[int]bool{}
	for _, p := range found {
		if p.File == file {
			bad[p.Line] = true
		}
	}
	var out []codeLine
	for i, line := range strings.Split(strings.TrimSuffix(string(src), "\n"), "\n") {
		l := colourLine(line)
		l.ID, l.Bad = fmt.Sprintf("L%d", i+1), bad[i+1]
		out = append(out, l)
	}
	return out
}

// writtenOut gives a machine written out, each value's file and line a
// link to that line of that file.
func writtenOut(text []byte) []codeLine {
	var out []codeLine
	for _, line := range strings.Split(strings.TrimSuffix(string(text), "\n"), "\n") {
		m := lineAt.FindStringSubmatch(line)
		if m == nil {
			out = append(out, colourLine(line))
			continue
		}
		l := colourLine(m[1])
		href := "/machines?" + url.Values{"file": {m[3]}, "view": {"text"}}.Encode() + "#L" + m[4]
		l.Spans = append(l.Spans, span{Text: m[2]}, span{Class: "c", Text: "# " + m[3] + ":" + m[4], Href: href})
		out = append(out, l)
	}
	return out
}

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

// shortly writes a directory as a person types it: under the home
// directory, from ~.
func shortly(dir string) string {
	if home, err := os.UserHomeDir(); err == nil && (dir == home || strings.HasPrefix(dir, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(dir, home)
	}
	return dir
}

// short cuts a long path in the middle, keeping its start and its end.
func short(p string) string {
	if r := []rune(p); len(r) > 34 {
		return string(r[:10]) + "…" + string(r[len(r)-23:])
	}
	return p
}
