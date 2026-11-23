package machines

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"
)

// A toolkit is a group of tools under an agent's `toolkits:`, as the
// Toolkits page shows it. Tree is the prefix of its tree, File the file its
// tools were submitted in, where that is not its agent's own, Instructions
// the first line of its `instructions:`, or InstructionsFile the file they
// were included from, and Opening says opening it needs approval.
type toolkit struct {
	ID, Tree, Name, Description, File string
	Instructions, InstructionsFile    string
	Opening                           bool
	Tools                             []toolkitTool
	Offered                           []offer
	key                               string
}

// A toolkitTool is a tool of a toolkit: its job's kind, script, machine,
// call or send, the file a script or a machine job names, the location a
// call names, the event a send sends and the address it sends it to, the
// location of the machine recorded where the job calls, sends or runs, the
// names of the input a call fixes, its `when:`, and whether the job needs
// approval.
type toolkitTool struct {
	Name, Description, Job, File, At, Call, Event, To, When string
	Fixed                                                   []string
	Approve                                                 bool
}

// An offer is an agent that offers a toolkit: its name, its file, the
// location of the machine recorded from that file, the `when:` of its
// entry, the machines that start it, and for an agent at a location, how
// its location's runs start.
type offer struct {
	Name, File, At, When, Lifetime, Started string
	Starts                                  []start
}

// A start is a machine that starts an agent: its name, its file, the
// location it is recorded at, and its type.
type start struct {
	Name, File, At, Type string
}

// A node is one file of a machine as the toolkits are read: the type and name of
// the machine in it.
type node struct {
	name, kind string
}

// toolkitIndex is what the machines of one tree say of toolkits: files
// gives the machine recorded from each file, and above the files of the
// machines that run or call the machine in a file.
type toolkitIndex struct {
	ix       *index
	tree     string
	files    map[string]*recorded
	byFile   map[string]*node
	above    map[string][]string
	toolkits map[string]*toolkit
}

// toolkitsOf reads every toolkit an agent of the machines recorded offers,
// tree by tree, and through them the machines they name and call.
func toolkitsOf(ix *index) []toolkit {
	var toolkits []toolkit
	for _, t := range ix.Trees {
		px := &toolkitIndex{ix: ix, tree: t.Prefix, files: map[string]*recorded{}, byFile: map[string]*node{}, above: map[string][]string{}, toolkits: map[string]*toolkit{}}
		for _, e := range t.Entries {
			m := ix.machines[e.Location]
			px.files[m.File] = m
		}
		for _, file := range sortedKeys(px.files) {
			px.add(file, px.files[file].Draft)
		}
		for _, key := range sortedKeys(px.toolkits) {
			p := px.toolkits[key]
			for i := range p.Offered {
				o := &p.Offered[i]
				o.Starts = px.startsOf(o.File)
				if m := px.files[o.File]; m != nil && px.byFile[o.File].kind == "agent" {
					o.Lifetime, o.Started = m.Draft.Definition.Lifetime, startedBy(m.Draft.Definition.Lifetime, m.Location)
				}
			}
			slices.SortFunc(p.Offered, func(a, b offer) int { return strings.Compare(a.File+a.Name, b.File+b.Name) })
			toolkits = append(toolkits, *p)
		}
	}
	slices.SortStableFunc(toolkits, func(a, b toolkit) int {
		return cmp.Or(strings.Compare(a.Tree, b.Tree), strings.Compare(a.Name+a.key, b.Name+b.key))
	})
	uses := map[string]int{}
	for i := range toolkits {
		p := &toolkits[i]
		uses[p.Name]++
		p.ID = p.Name
		if uses[p.Name] > 1 {
			p.ID = fmt.Sprintf("%s-%d", p.Name, uses[p.Name])
		}
	}
	return toolkits
}

// at gives the location of the machine recorded from a file of the tree,
// or nothing where the file has none.
func (px *toolkitIndex) at(file string) string {
	if m := px.files[file]; m != nil {
		return m.Location
	}
	return ""
}

// add reads a machine in a file once, and what it names.
func (px *toolkitIndex) add(file string, d *machine.Draft) {
	if px.byFile[file] != nil {
		return
	}
	px.byFile[file] = &node{name: d.Definition.Name, kind: d.Definition.Type}
	px.read(file, d)
}

// read reads a machine, in its own file or written in place in that file: the
// toolkits of an agent, the machines it names, and those its calls name.
func (px *toolkitIndex) read(file string, d *machine.Draft) {
	if d.Definition.Type == "agent" {
		var a agent.Agent
		if json.Unmarshal(d.Definition.Body, &a) == nil {
			px.offers(file, &a, d)
		}
	}
	for _, j := range machine.Jobs(d.Definition) {
		if j.Job.Call == nil || j.Job.Call.Expr != "" {
			continue
		}
		called := px.ix.machines[px.ix.locationOf(px.tree, j.Job.Call.At)]
		if called != nil && called.Tree == px.tree && !slices.Contains(px.above[called.File], file) {
			px.above[called.File] = append(px.above[called.File], file)
		}
	}
	for _, name := range sortedKeys(d.Children) {
		if strings.HasPrefix(name, "in-place-") {
			px.read(file, d.Children[name])
			continue
		}
		child := path.Clean(name)
		if !slices.Contains(px.above[child], file) {
			px.above[child] = append(px.above[child], file)
		}
		px.add(child, d.Children[name])
	}
}

// offers notes the toolkits of an agent. Two agents that include one file
// offer one toolkit, whichever path and name each gives it. Toolkits in no
// file are one where their names, their `description:` and their tools are
// the same.
func (px *toolkitIndex) offers(file string, a *agent.Agent, d *machine.Draft) {
	for _, name := range sortedKeys(a.Toolkits) {
		var tools []string
		for _, tool := range sortedKeys(a.Tools) {
			if a.Tools[tool].Toolkit == name {
				tools = append(tools, tool)
			}
		}
		included := drawing.ToolkitFile(d.Definition, name, file)
		key := included
		if key == "" {
			key = name + "\x00" + a.Toolkits[name].Description + "\x00" + strings.Join(tools, " ")
		}
		p := px.toolkits[key]
		if p == nil {
			p = &toolkit{Tree: px.tree, Name: name, Description: a.Toolkits[name].Description, File: included, key: key}
			p.Instructions, p.InstructionsFile = instructionsOf(d.Definition, name, a.Toolkits[name].Instructions, cmp.Or(included, file))
			for _, tool := range tools {
				p.Tools = append(p.Tools, px.toolOf(tool, a.Tools[tool], d))
			}
			px.toolkits[key] = p
		}
		p.Opening = p.Opening || a.Toolkits[name].Approve
		if !slices.ContainsFunc(p.Offered, func(o offer) bool { return o.File == file && o.Name == d.Definition.Name }) {
			p.Offered = append(p.Offered, offer{Name: d.Definition.Name, File: file, At: px.at(file), When: a.Toolkits[name].When})
		}
	}
}

// instructionsOf gives a toolkit's instructions as the page shows them: the
// file they are included from, where that is not the toolkit's own, or
// their first line.
func instructionsOf(def *machine.Definition, name, text, own string) (string, string) {
	if text == "" {
		return "", ""
	}
	if file := fileOf(def.Lines["toolkits."+name+".instructions"]); file != "" && file != own {
		return "", file
	}
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return first, ""
}

// fileOf gives the file of a value's file and line, as base.yaml:3.
func fileOf(line string) string {
	if i := strings.LastIndexByte(line, ':'); i >= 0 {
		return line[:i]
	}
	return line
}

// toolOf gives a tool as the page shows it: a call's or a send's location
// is that of the machine recorded where it calls or sends to. A call is
// written `call:` or as a job's `call:`.
func (px *toolkitIndex) toolOf(name string, t *agent.Tool, d *machine.Draft) toolkitTool {
	out := toolkitTool{Name: name, Description: t.Description, When: t.When}
	if t.Job == nil {
		return out
	}
	out.Approve = t.Job.Approve
	called := t.Call
	if called == "" && t.Job.Call != nil {
		called = cmp.Or(t.Job.Call.At, t.Job.Call.Expr)
	}
	if called != "" {
		fixed, _ := t.Job.Input.(map[string]any)
		out.Job, out.Call, out.At, out.Fixed = "call", called, px.ix.locationOf(px.tree, called), sortedKeys(fixed)
		return out
	}
	if s := t.Job.Send; s != nil {
		out.Job, out.Event, out.To = "send", s.Event, drawing.Address(&s.To)
		if s.To.Expr == "" {
			out.At = px.ix.locationOf(px.tree, s.To.At)
		}
		return out
	}
	if t.Job.IsMachine() {
		out.Job = "machine"
		if d.Children[t.Job.Machine] != nil && !strings.HasPrefix(t.Job.Machine, "in-place-") {
			out.File = path.Clean(t.Job.Machine)
			out.At = px.at(out.File)
		}
		return out
	}
	out.Job, out.File = "script", writtenFrom(t.Job.Script)
	return out
}

// writtenFrom gives the path of a script an agent written out names, from the
// directory, or nothing for one of Corbie's own scripts or one an expression
// gives.
func writtenFrom(name string) string {
	if name == "" || strings.HasPrefix(name, "corbie:") || machine.HasExpr(name) {
		return ""
	}
	return path.Clean(name)
}

// locationOf gives the location a definition of a tree writes out, read
// from the tree's prefix or from / where it begins with /, where a machine
// is recorded there, or nothing.
func (ix *index) locationOf(tree, at string) string {
	if at == "" || machine.HasExpr(at) {
		return ""
	}
	if !strings.HasPrefix(at, "/") {
		at = path.Join(tree, at)
	}
	if ix.machines[at] == nil {
		return ""
	}
	return at
}

// called gives the address on the Machines page of the machine recorded at
// a location a call of a tree writes out, or none.
func (ix *index) called(tree, at string) string {
	if location := ix.locationOf(tree, at); location != "" {
		return machineHref(location, nil)
	}
	return ""
}

// startsOf gives the machines that start the agent in a file: for an agent
// in a file of its own, each machine that runs that file with `machine:` or
// calls its location; for one written in place, the machine it is written
// in.
func (px *toolkitIndex) startsOf(file string) []start {
	if h := px.byFile[file]; h.kind != "agent" {
		return []start{{Name: h.name, File: file, At: px.at(file), Type: h.kind}}
	}
	var starts []start
	for _, above := range px.above[file] {
		h := px.byFile[above]
		starts = append(starts, start{Name: h.name, File: above, At: px.at(above), Type: h.kind})
	}
	slices.SortFunc(starts, func(a, b start) int { return strings.Compare(a.File, b.File) })
	return starts
}

// startedBy says how the runs at a location with a lifetime start.
func startedBy(lifetime, location string) string {
	switch lifetime {
	case machine.LifetimeSingleton:
		return "the submit starts its one run"
	case machine.LifetimeKeyed:
		return "a send to a key starts the run for that key"
	}
	return "a call, a send, or corbie run start " + location + ", starts each run"
}

// ToolkitsParts are the parts of the Toolkits page: the list, and the
// toolkit chosen, which the page has as it loads and asks for at
// /toolkits/view.
type ToolkitsParts struct {
	List    toolkitList    `json:"list"`
	Toolkit *toolkitChosen `json:"toolkit,omitzero"`
}

// toolkitList is the list of the Toolkits page: the toolkits of each tree,
// and the mark of the trees they were read at.
type toolkitList struct {
	Trees []toolkitTree
	Stamp string
}

// A toolkitTree is the toolkits the agents of one tree offer.
type toolkitTree struct {
	Prefix   string
	Toolkits []toolkit
}

// toolkitChosen is the toolkit chosen, and the mark of the trees it was
// read at.
type toolkitChosen struct {
	Open  *toolkit
	Stamp string
}

// listOfToolkits gives the list of the Toolkits page.
func (ix *index) listOfToolkits() toolkitList {
	l := toolkitList{Stamp: ix.Stamp}
	for _, k := range ix.toolkits {
		if n := len(l.Trees); n == 0 || l.Trees[n-1].Prefix != k.Tree {
			l.Trees = append(l.Trees, toolkitTree{Prefix: k.Tree})
		}
		t := &l.Trees[len(l.Trees)-1]
		t.Toolkits = append(t.Toolkits, k)
	}
	return l
}

// chosenToolkit reads the Toolkits page at an address: the toolkit it
// chooses, by default the first. It gives the status of the answer where
// it cannot.
func (h *handler) chosenToolkit(ctx context.Context, q url.Values) (*index, *toolkitChosen, int, error) {
	ix, err := h.index.current(ctx)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, err
	}
	open, v := q.Get("toolkit"), &toolkitChosen{Stamp: ix.Stamp}
	for i := range ix.toolkits {
		if ix.toolkits[i].ID == open || (open == "" && i == 0) {
			v.Open = &ix.toolkits[i]
		}
	}
	if open != "" && v.Open == nil {
		return ix, v, http.StatusNotFound, fmt.Errorf("there is no toolkit %s", open)
	}
	return ix, v, http.StatusOK, nil
}

// showToolkits is the Toolkits page.
func (h *handler) showToolkits(w http.ResponseWriter, r *http.Request) {
	ix, v, status, err := h.chosenToolkit(r.Context(), r.URL.Query())
	if err != nil {
		page.Error(w, status, err.Error())
		return
	}
	s := h.Shell
	s.Title, s.Current, s.Stream, s.Script = "Toolkits", "Toolkits", "/toolkits/stream", "/toolkits/script.js"
	page.Write(w, s, ToolkitsParts{List: ix.listOfToolkits(), Toolkit: v})
}

// toolkitPart answers the part "toolkit" of the Toolkits page at the
// address its query gives, as JSON: the page asks for it when it shows
// another toolkit in place, or when a submit changed a tree.
func (h *handler) toolkitPart(w http.ResponseWriter, r *http.Request) {
	_, v, status, err := h.chosenToolkit(r.Context(), r.URL.Query())
	if err != nil {
		page.Error(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.MarshalWrite(w, v, json.Deterministic(true))
}

// streamToolkits sends the page the list of toolkits as the store now
// holds it, whatever toolkit the page shows.
func (h *handler) streamToolkits(w http.ResponseWriter, r *http.Request) {
	never := make(chan struct{})
	page.Stream(w, r, h.Interval, func() <-chan struct{} { return never }, func(ctx context.Context) (any, error) {
		ix, err := h.index.current(ctx)
		if err != nil {
			return nil, err
		}
		return ToolkitsParts{List: ix.listOfToolkits()}, nil
	})
}
