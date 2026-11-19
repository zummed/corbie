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

	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
)

// A toolkit is a group of tools under an agent's `toolkits:`, as the
// Toolkits page shows it. File is the file it is included from, where one is
// written, Instructions the first line of its `instructions:`, or
// InstructionsFile the file they are included from, Opening says opening it
// needs approval, and Broken that its file has problems.
type toolkit struct {
	ID, Name, Description, File    string
	Instructions, InstructionsFile string
	Opening, Broken                bool
	Tools                          []toolkitTool
	Offered                        []offer
	key                            string
}

// A toolkitTool is a tool of a toolkit: its job's kind, script, machine,
// call or send, the file that job names or of the machine at the location
// it calls or sends to, the location a call names, the event a send sends
// and the address it sends it to, the names of the input a call fixes, its
// `when:`, and whether the job needs approval.
type toolkitTool struct {
	Name, Description, Job, File, Call, Event, To, When string
	Fixed                                               []string
	Approve                                             bool
}

// An offer is an agent that offers a toolkit, with the `when:` of its entry,
// the machines that start it, and for an agent in a file of its own, how its
// location's runs start.
type offer struct {
	Name, File, When, Lifetime, Started string
	Starts                              []start
}

// A start is a machine that starts an agent.
type start struct {
	Name, File, Type string
}

// toolkitsView is the Toolkits page as it is drawn, its one part
// "toolkits". It lists the project's problems as the Machines page does,
// and shows the toolkit chosen with the problems found in its file, or its
// file's text. Dir is the directory, cut short, and DirFull the whole of it.
type toolkitsView struct {
	Dir      string
	DirFull  string
	Toolkits []toolkit
	Open     *toolkit
	Problems []problem
	Found    []problem
	ShowText bool
	Text     []codeLine
	Unread   string
}

// A node is one file of a machine as the toolkits are read: the type and name of
// the machine in it.
type node struct {
	name, kind string
}

// toolkitIndex is what the machines in a directory say of toolkits: above
// gives the files of the machines that run or call the machine in a file.
type toolkitIndex struct {
	ix       *index
	byFile   map[string]*node
	above    map[string][]string
	toolkits map[string]*toolkit
}

// toolkitsOf reads every toolkit an agent in the directory offers, and
// through them the machines they name and call. A machine with
// problems offers none, since Corbie does not write it out.
func toolkitsOf(ix *index) []toolkit {
	px := &toolkitIndex{ix: ix, byFile: map[string]*node{}, above: map[string][]string{}, toolkits: map[string]*toolkit{}}
	for _, file := range sortedKeys(ix.machines) {
		if m := ix.machines[file]; m.Draft != nil {
			px.add(file, m.Draft)
		}
	}
	var toolkits []toolkit
	for _, p := range px.toolkits {
		for i := range p.Offered {
			o := &p.Offered[i]
			o.Starts = px.startsOf(o.File)
			if m := ix.machines[o.File]; m != nil && m.Draft != nil && px.byFile[o.File].kind == "agent" {
				o.Lifetime, o.Started = m.Draft.Definition.Lifetime, startedBy(m.Draft.Definition.Lifetime, m.Location)
			}
		}
		slices.SortFunc(p.Offered, func(a, b offer) int { return strings.Compare(a.File+a.Name, b.File+b.Name) })
		toolkits = append(toolkits, *p)
	}
	for i := range toolkits {
		toolkits[i].Broken = toolkits[i].File != "" && slices.ContainsFunc(ix.Problems, func(p problem) bool { return p.File == toolkits[i].File })
	}
	slices.SortFunc(toolkits, func(a, b toolkit) int { return strings.Compare(a.Name+a.key, b.Name+b.key) })
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

// toolkitsView gives the Toolkits page with a toolkit chosen, by default the
// first.
func (ix *index) toolkitsView(open string) toolkitsView {
	v := toolkitsView{Toolkits: ix.toolkits, Problems: ix.Problems, Unread: ix.Unread}
	for i := range v.Toolkits {
		if v.Toolkits[i].ID == open || (open == "" && i == 0) {
			v.Open = &v.Toolkits[i]
		}
	}
	if v.Open != nil && v.Open.File != "" {
		v.Found = ix.at(v.Open.File)
	}
	return v
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
		if called := px.ix.fileAt(j.Job.Call.At); called != "" && !slices.Contains(px.above[called], file) {
			px.above[called] = append(px.above[called], file)
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
			p = &toolkit{Name: name, Description: a.Toolkits[name].Description, File: included, key: key}
			p.Instructions, p.InstructionsFile = instructionsOf(d.Definition, name, a.Toolkits[name].Instructions, cmp.Or(included, file))
			for _, tool := range tools {
				p.Tools = append(p.Tools, px.ix.toolOf(tool, a.Tools[tool], d))
			}
			px.toolkits[key] = p
		}
		p.Opening = p.Opening || a.Toolkits[name].Approve
		if !slices.ContainsFunc(p.Offered, func(o offer) bool { return o.File == file && o.Name == d.Definition.Name }) {
			p.Offered = append(p.Offered, offer{Name: d.Definition.Name, File: file, When: a.Toolkits[name].When})
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

// toolOf gives a tool as the page shows it, its paths written from the
// directory: a call's or a send's file is that of the machine at the
// location it calls or sends to. A call is written `call:` or as a job's
// `call:`.
func (ix *index) toolOf(name string, t *agent.Tool, d *machine.Draft) toolkitTool {
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
		out.Job, out.Call, out.File, out.Fixed = "call", called, ix.fileAt(called), sortedKeys(fixed)
		return out
	}
	if s := t.Job.Send; s != nil {
		out.Job, out.Event, out.To = "send", s.Event, drawing.Address(&s.To)
		if s.To.Expr == "" {
			out.File = ix.fileAt(s.To.At)
		}
		return out
	}
	if t.Job.IsMachine() {
		out.Job = "machine"
		if d.Children[t.Job.Machine] != nil && !strings.HasPrefix(t.Job.Machine, "in-place-") {
			out.File = path.Clean(t.Job.Machine)
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

// fileAt gives the file of the machine at a location a definition writes
// out, from the tree's prefix or from / where it begins with /, or nothing
// where the directory has no machine there.
func (ix *index) fileAt(at string) string {
	if at == "" || machine.HasExpr(at) {
		return ""
	}
	location := at
	if !strings.HasPrefix(at, "/") {
		location = path.Join(ix.Prefix, at)
	}
	return ix.located[location]
}

// called gives the address on the Machines page of the machine at a
// location a call writes out, or none.
func (ix *index) called(at string) string {
	if file := ix.fileAt(at); file != "" {
		return "/machines?" + url.Values{"file": {file}}.Encode()
	}
	return ""
}

// startsOf gives the machines that start the agent in a file: for an agent
// in a file of its own, each machine that runs that file with `machine:` or
// calls its location; for one written in place, the machine it is written
// in.
func (px *toolkitIndex) startsOf(file string) []start {
	if h := px.byFile[file]; h.kind != "agent" {
		return []start{{Name: h.name, File: file, Type: h.kind}}
	}
	var starts []start
	for _, above := range px.above[file] {
		h := px.byFile[above]
		starts = append(starts, start{Name: h.name, File: above, Type: h.kind})
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

// toolkitsPage reads the Toolkits page as an address asks for it: the
// toolkit chosen, and its file's text where the address says view=text.
func (h *handler) toolkitsPage(ctx context.Context, q url.Values) (toolkitsView, error) {
	ix, err := h.tree(ctx)
	if err != nil {
		return toolkitsView{}, err
	}
	v := ix.toolkitsView(q.Get("toolkit"))
	v.Dir, v.DirFull = short(shortly(h.Dir)), shortly(h.Dir)
	if v.Open != nil && v.Open.File != "" && q.Get("view") == "text" {
		src, _, err := h.dir.read(v.Open.File)
		if err != nil {
			return v, err
		}
		v.ShowText, v.Text = true, colour(src, v.Open.File, v.Found)
	}
	return v, nil
}

// showToolkits is the Toolkits page.
func (h *handler) showToolkits(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, err := h.toolkitsPage(r.Context(), q)
	if err != nil {
		page.Error(w, http.StatusNotFound, err.Error())
		return
	}
	id := q.Get("toolkit")
	if id != "" && v.Open == nil {
		page.Error(w, http.StatusNotFound, "there is no toolkit "+id)
		return
	}
	stream := url.Values{}
	if v.Open != nil {
		stream.Set("toolkit", v.Open.ID)
	}
	if v.ShowText {
		stream.Set("view", "text")
	}
	s := h.Shell
	s.Title, s.Current, s.Stream, s.Script = "Toolkits", "Toolkits", "/toolkits/stream?"+stream.Encode(), "/toolkits/script.js"
	page.Write(w, s, ToolkitsParts{Toolkits: v})
}

// streamToolkits sends the page the toolkits as the directory now holds them.
func (h *handler) streamToolkits(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	never := make(chan struct{})
	page.Stream(w, r, h.Interval, func() <-chan struct{} { return never }, func(ctx context.Context) (any, error) {
		v, err := h.toolkitsPage(ctx, q)
		if err != nil {
			return nil, err
		}
		return ToolkitsParts{Toolkits: v}, nil
	})
}

// ToolkitsParts are the parts of the Toolkits page: its one part, the
// toolkits and the one chosen.
type ToolkitsParts struct {
	Toolkits toolkitsView `json:"toolkits"`
}
