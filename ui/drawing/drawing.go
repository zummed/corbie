// Package drawing gives the graph of a machine from its definition in full
// form, with what a run shows of its state marked on it, and the browser
// files that lay the graph out and draw it as SVG.
package drawing

import (
	"embed"
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
	"github.com/zummed/corbie/machine/flow"
	"github.com/zummed/corbie/machine/statechart"
)

//go:embed layout.js drawing.js
var files embed.FS

// Files serves the area's browser files: layout.js, which lays a graph out,
// and drawing.js, which draws it.
func Files() http.Handler {
	sub, _ := fs.Sub(files, ".")
	return http.FileServerFS(sub)
}

// A Run is what a run shows of its state as of one step, to be marked on
// its machine's drawing.
type Run struct {
	// Steps are the steps of its record, in order, up to the step it is
	// drawn as of: the latest.
	Steps []Step
	// Going are the jobs going after the latest step, and Held those of
	// them held for a hook. A job is named as the definition names what
	// asked for it, and a hook's job as HookJob names it.
	Going, Held []string
	// Children are the runs its jobs started, by the job's name.
	Children map[string]Child
	// In are the graphs of runs its jobs started, by the job's name, each
	// drawn inside that job's card with its own marks.
	In map[string]Scene
	// Machines are the machines its definition names, by the name its jobs
	// give them.
	Machines map[string]Machine
	// Ended says the run had ended by the latest step: the states it ended
	// in are no longer lit as current. Failed says it ended with status
	// failed.
	Ended, Failed bool
}

// A Step is one step of a run's record: the run's state after it, the
// events it took, the jobs whose failure it took, and the jobs it asked
// for, by name.
type Step struct {
	Shown  machine.Shown
	Events []string
	Failed []string
	Asked  []string
}

// A Child is a run a job started: its ID, its machine and the machine's
// type.
type Child struct{ ID, Machine, Type string }

// A Machine is a machine a definition names: its type and its name.
type Machine struct{ Type, Name string }

// latest gives the run's state after the latest step.
func (r *Run) latest() machine.Shown {
	if r == nil || len(r.Steps) == 0 {
		return machine.Shown{}
	}
	return r.Steps[len(r.Steps)-1].Shown
}

// asked says whether any step asked for a job.
func (r *Run) asked(name string) bool {
	return r != nil && slices.ContainsFunc(r.Steps, func(s Step) bool { return slices.Contains(s.Asked, name) })
}

// failedAt says the run failed at a job: its last step took the job's
// failure, and the run ended with status failed.
func (r *Run) failedAt(name string) bool {
	return r != nil && r.Failed && len(r.Steps) > 0 && slices.Contains(r.Steps[len(r.Steps)-1].Failed, name)
}

// A Scene is the graph of a machine, which the browser lays out and draws:
// its type and name, and the graph of its type. A mark is how a run's state
// marks a card: now, held, done, failed, cancelled, skipped or never, and
// nothing where no run is drawn.
type Scene struct {
	Type, Name string
	Flow       *Flow     `json:",omitzero"`
	Chart      *Chart    `json:",omitzero"`
	Agent      *Agent    `json:",omitzero"`
	Decision   *Decision `json:",omitzero"`
}

// A Card is what a card says: the icon of what its job is, its name, a line
// for each fact about its job, its tooltip, its link, the machine unfolded
// inside it, whether it is narrow, as a final state is, and where its file
// writes it, as file:line, where the definition says.
type Card struct {
	Icon, Name string
	Lines      []string
	Title      string
	Href       string
	In         *Scene `json:",omitzero"`
	Narrow     bool
	Line       string
}

// A Flow is its stages, by name.
type Flow struct {
	Stages []Stage
}

// A Stage is a stage's card, the stages it waits for and why it waits for
// each, by the stage's name, whether it has `when:`, whether it is drawn as
// a stack for `each:`, `emits:` or `blob:`, and its mark.
type Stage struct {
	Card  Card
	After []string
	Why   map[string]string
	When  bool
	Stack bool
	Mark  string
}

// A Chart is a statechart: the state it starts in, every state, a holder
// before the states it holds, and every transition.
type Chart struct {
	Start  string
	States []State
	Rules  []Rule
}

// A State is a state's card, the state that holds it, the state it starts
// in where it holds states, whether it is final and ends failed, and its
// mark. A send to an address is a state of its own, below the state that
// sends it, with a name no state can have.
type State struct {
	Card   Card
	Name   string
	Parent string
	Start  string
	Final  bool
	Fails  bool
	Send   bool
	Mark   string
}

// A Rule is one transition: from a state, on an event, to a state, with its
// condition and its timer's time, whether it is a send's, and whether the
// step drawn took it.
type Rule struct {
	From, Event, To, When, Timer string
	Send, Taken                  bool
}

// An Agent is its model with its mark and the turn it is on, how many turns
// it has, what it can call, its hooks, and where its file writes the model.
type Agent struct {
	Model, Mark string
	ModelLine   string
	Turn, Turns int
	Toolkits    []Toolkit
	Hooks       []Hook
}

// A Hook is a hook's card with its mark. When says it has a condition.
type Hook struct {
	Card Card
	When bool
	Mark string
}

// HookJob is the name a run's marks give the jobs of a hook: one no job of
// a definition has.
func HookJob(hook string) string { return "hook " + hook }

// A Toolkit is one group of what the model can call: a toolkit, or the
// tools in no toolkit, which has no name. File is the file the toolkit is
// included from, where that is not the agent's own, and Folded says the
// run drawn has not asked for its tools.
type Toolkit struct {
	Name, File, When string
	Toolkit, Folded  bool
	Tools            []Tool
}

// A Tool is a tool's card, whether it has `when:`, and its mark.
type Tool struct {
	Card Card
	When bool
	Mark string
}

// A Decision is its model with its mark, where its file writes the model,
// and its answers.
type Decision struct {
	Model, Mark string
	ModelLine   string
	Answers     Card
}

// Draw gives the graph of a definition, with a run's state marked on it
// when run is not nil.
func Draw(def *machine.Definition, run *Run) (Scene, error) {
	w := &walker{}
	if run != nil {
		w.children, w.machines, w.in = run.Children, run.Machines, run.In
	}
	return sceneOf(def, run, w)
}

// Unfold says how a drawing's cards unfold. A card's path is the names of
// the cards down to it, from the machine drawn, joined by /.
type Unfold struct {
	// File is the file of the machine drawn.
	File string
	// Open holds the paths of the cards unfolded.
	Open map[string]bool
	// Href gives the address of a card's link.
	Href func(path string) string
	// Called gives the address of the machine at a location a call writes
	// out, or none where there is no machine there.
	Called func(at string) string
}

// Unfolded gives the graph of a machine and the machines it names, with a link on
// each card whose job names a machine, and the machines of the cards
// unfolded inside them.
func Unfolded(draft *machine.Draft, u Unfold) (Scene, error) {
	return sceneOf(draft.Definition, nil, &walker{unfold: u, draft: draft, file: u.File})
}

func sceneOf(def *machine.Definition, run *Run, w *walker) (Scene, error) {
	s := Scene{Type: def.Type, Name: def.Name}
	w.lines = def.Lines
	switch def.Type {
	case "flow":
		var f flow.Flow
		if err := json.Unmarshal(def.Body, &f); err != nil {
			return s, err
		}
		s.Flow = flowOf(&f, run, w)
	case "statechart":
		var c statechart.Chart
		if err := json.Unmarshal(def.Body, &c); err != nil {
			return s, err
		}
		s.Chart = chartOf(&c, run, w)
	case "agent":
		var a agent.Agent
		if err := json.Unmarshal(def.Body, &a); err != nil {
			return s, err
		}
		s.Agent = agentOf(def, &a, run, w)
	case "decision":
		var dec agent.Decision
		if err := json.Unmarshal(def.Body, &dec); err != nil {
			return s, err
		}
		s.Decision = decisionOf(&dec, run)
		s.Decision.ModelLine = lineOf(def.Lines, "model")
		s.Decision.Answers.Line = lineOf(def.Lines, "questions")
	default:
		return s, fmt.Errorf("a machine of type %q has no drawing", def.Type)
	}
	return s, nil
}

// A walker is what a drawing knows of the machines its jobs name. On the
// Machines page it is a machine's draft, its file and the path of the card
// it is drawn in. On a run's page it is the machines its definition names,
// the runs its jobs started, and the graphs of those drawn inside its cards.
type walker struct {
	unfold   Unfold
	draft    *machine.Draft
	file     string
	machines map[string]Machine
	children map[string]Child
	in       map[string]Scene
	path     string
	lines    map[string]string
}

// open gives the link of a card whose job names a machine in a draft, and
// the machine's graph when the card is unfolded.
func (w *walker) open(at, job string) (string, *Scene) {
	at = strings.TrimPrefix(w.path+"/"+at, "/")
	href := w.unfold.Href(at)
	if !w.unfold.Open[at] {
		return href, nil
	}
	sub := &walker{unfold: w.unfold, draft: w.draft.Children[job], file: path.Clean(job), path: at}
	if inPlace(job) {
		sub.file = w.file
	}
	s, err := sceneOf(sub.draft.Definition, nil, sub)
	if err != nil {
		return href, nil
	}
	return href, &s
}

// inPlace says a name in a definition's table of machines is that of a
// machine written in place.
func inPlace(job string) bool { return strings.HasPrefix(job, "in-place-") }

// ToolkitFile gives the file a toolkit of an agent is included from, as
// Corbie writes out the agent: the file its tools are written in, or its
// `description:` where it has no tools, when that is not own, the agent's
// own file. A toolkit written in place gives nothing.
func ToolkitFile(def *machine.Definition, toolkit, own string) string {
	at := "toolkits." + toolkit + "."
	file := fileOf(def.Lines[at+"description"])
	for _, place := range slices.Sorted(maps.Keys(def.Lines)) {
		if strings.HasPrefix(place, at+"tools.") {
			file = fileOf(def.Lines[place])
			break
		}
	}
	if file == own {
		return ""
	}
	return file
}

// fileOf gives the file of a value's file and line, as base.yaml:3.
func fileOf(line string) string {
	if i := strings.LastIndexByte(line, ':'); i >= 0 {
		return line[:i]
	}
	return line
}

// describe writes what a card's job is into it: its icon, its lines, a line
// where it needs approval, where the job names a machine, the link to it,
// and the job's limits in its tooltip. at is the name a run gives the
// card's job, and its path on the Machines page.
func (w *walker) describe(c *Card, at string, job *machine.JobSpec) {
	if job == nil {
		return
	}
	w.describeJob(c, at, job)
	if s, ok := w.in[at]; ok {
		c.In = &s
	}
	if job.Approve {
		c.Lines = append(c.Lines, "approve: true")
	}
	c.Title = strings.Join(slices.DeleteFunc(append([]string{c.Title}, limits(job)...), func(l string) bool { return l == "" }), "\n")
}

// limits gives what bounds a job, as a definition writes it: its timeout,
// its restart policy, with its limit and its delay where they are not
// Corbie's defaults, the tags of the runner it needs, and what it is
// allowed.
func limits(job *machine.JobSpec) []string {
	var out []string
	if job.Timeout != "" {
		out = append(out, "timeout: "+job.Timeout)
	}
	if r := job.Restart; r != nil {
		after := "failure"
		if r.Always {
			after = "always"
		}
		fields := []string{"after: " + after}
		switch {
		case r.Limit > 0 && r.Within > 0:
			fields = append(fields, "limit: "+strconv.Itoa(r.Limit)+" in "+short(time.Duration(r.Within)))
		case r.Limit > 0:
			fields = append(fields, "limit: "+strconv.Itoa(r.Limit))
		}
		switch {
		case r.Delay == machine.DefaultDelay && r.Longest == machine.DefaultLongest:
		case r.Delay == r.Longest:
			fields = append(fields, "delay: "+short(time.Duration(r.Delay)))
		default:
			fields = append(fields, "delay: "+short(time.Duration(r.Delay))+" "+short(time.Duration(r.Longest)))
		}
		out = append(out, "restart: { "+strings.Join(fields, ", ")+" }")
	}
	if len(job.Tags) > 0 {
		out = append(out, "tags: "+strings.Join(job.Tags, ", "))
	}
	if len(job.Allow) > 0 {
		out = append(out, "allow: "+strings.Join(job.Allow, ", "))
	}
	return out
}

// short writes a length of time as a definition does: 1h, 1h30m, 5s, with
// no units of none.
func short(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

// lineOf gives where a definition writes the value at a place, such as
// stages.find, as file:line: the first line of the values under it in the
// file most of them are in, the file named first where two have as many,
// or nothing where it writes none. Corbie gives the line of each value
// that is no map or list, so it is the line of the first such value, not
// of the place's own key.
func lineOf(lines map[string]string, place string) string {
	type at struct {
		values, first int
	}
	files := map[string]*at{}
	for k, v := range lines {
		if k != place && !strings.HasPrefix(k, place+".") && !strings.HasPrefix(k, place+"[") {
			continue
		}
		file, line := "", v
		if i := strings.LastIndexByte(v, ':'); i >= 0 {
			file, line = v[:i], v[i+1:]
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		f := files[file]
		if f == nil {
			f = &at{first: n}
			files[file] = f
		}
		f.values++
		f.first = min(f.first, n)
	}
	if len(files) == 0 {
		return ""
	}
	names := slices.Sorted(maps.Keys(files))
	best := names[0]
	for _, file := range names[1:] {
		if files[file].values > files[best].values {
			best = file
		}
	}
	f := files[best]
	if best == "" {
		return strconv.Itoa(f.first)
	}
	return best + ":" + strconv.Itoa(f.first)
}

// describeJob writes a card's icon, its line and its link. The machine a
// job names is known by its draft, or by the definition a run was started
// from; one that is neither is known only by how the definition names it.
// On a run's page the card links to the run its job started, and on the
// Machines page a call's card to the machine at the location it writes out.
func (w *walker) describeJob(c *Card, at string, job *machine.JobSpec) {
	switch {
	case job.Script != "":
		c.Icon, c.Lines = "script", []string{fit("script: " + job.Script)}
		return
	case job.Send != nil:
		c.Icon, c.Lines = "statechart", []string{fit("send: " + job.Send.Event), fit("to: " + Address(&job.Send.To))}
		return
	case job.Call != nil:
		c.Icon, c.Lines = "file", []string{fit("call: " + Address(job.Call))}
		if ch, ok := w.children[at]; ok {
			c.Icon, c.Lines, c.Href = ch.Type, append(c.Lines, fit(ch.Type+": "+ch.Machine)), "/run/"+ch.ID
		} else if w.unfold.Called != nil && job.Call.Expr == "" && !machine.HasExpr(job.Call.At) {
			c.Href = w.unfold.Called(job.Call.At)
		}
		return
	case job.Machine == "":
		return
	}
	c.Icon, c.Lines = "file", []string{fit("machine: " + job.Machine)}
	if inPlace(job.Machine) {
		c.Lines = []string{"machine: written in place"}
	}
	if m, ok := w.machines[job.Machine]; ok {
		c.Icon, c.Lines = m.Type, []string{fit(m.Type + ": " + m.Name)}
	}
	if ch, ok := w.children[at]; ok {
		c.Icon, c.Lines, c.Href = ch.Type, []string{fit(ch.Type + ": " + ch.Machine)}, "/run/"+ch.ID
		return
	}
	if w.draft == nil || w.draft.Children[job.Machine] == nil {
		return
	}
	def := w.draft.Children[job.Machine].Definition
	c.Icon, c.Lines = def.Type, []string{fit(def.Type + ": " + def.Name)}
	c.Href, c.In = w.open(at, job.Machine)
}

// longest is how many letters a card's line has room for.
const longest = 26

// fit gives a line short enough for a card: where a path is too long, its
// file's name, and the whole path is in the file's text.
func fit(line string) string {
	if len([]rune(line)) <= longest {
		return line
	}
	field, value, ok := strings.Cut(line, ": ")
	if ok && strings.Contains(value, "/") {
		line = field + ": …/" + path.Base(value)
	}
	return clip(line, longest)
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// titleOf gives a tooltip of lines: those given, and `when:` where there is
// a condition.
func titleOf(first, when string) string {
	lines := []string{}
	if first != "" {
		lines = append(lines, first)
	}
	if when != "" {
		lines = append(lines, "when: "+when)
	}
	return strings.Join(lines, "\n")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
