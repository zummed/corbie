package machine

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A Definition is a machine's definition in its one full form, which is
// what is stored and run. A written definition is filled out into it.
type Definition struct {
	Type    string         `json:"type"`
	Name    string         `json:"name"`
	Input   map[string]any `json:"input,omitzero"`   // the shape of its input
	Keep    Duration       `json:"keep,omitzero"`    // how long its instances are kept
	Timeout string         `json:"timeout,omitzero"` // how long an instance may run: a length of time, or an expression
	Title   string         `json:"title,omitzero"`   // what a run is called: text, or text with expressions
	Depth   *int           `json:"depth,omitzero"`   // how many levels of machines may be below it; sixteen if not said
	Jobs    int            `json:"jobs,omitzero"`    // how many jobs an instance may start
	Allow   []string       `json:"allow,omitzero"`
	Deny    []string       `json:"deny,omitzero"`
	// Approve lists the permissions a job may use only after the ask
	// approves it.
	Approve []string `json:"approve,omitzero"`
	Ask     *JobSpec `json:"ask,omitzero"`
	// Description says what the machine is for: plain text, with nothing
	// filled in.
	Description string `json:"description,omitzero"`
	// Meta is data Corbie keeps with the definition and never reads: no
	// expression in it is filled in.
	Meta any `json:"meta,omitzero"`
	// Lifetime says how runs start at the file's location, and Update what a
	// changed file does to those already running. Runs is how many runs of a
	// keyed file may be running at once, with no limit at 0. A machine
	// written in place or made while running has none of them.
	Lifetime string `json:"lifetime,omitzero"`
	Update   string `json:"update,omitzero"`
	Runs     int    `json:"runs,omitzero"`
	// Receive says who may send to the file's runs or call it, by event
	// name, with "" for every event. With none, its own tree and its tree's
	// client may.
	Receive map[string][]string `json:"receive,omitzero"`
	// Read names the clients, beside its tree's, that read the records of
	// the file's runs, each as client:<name>.
	Read []string `json:"read,omitzero"`
	// Body is the type's own part.
	Body jsontext.Value `json:"body"`
	// Scripts and Machines are the files and child definitions the
	// definition names, each a blob beside the stored definition.
	Scripts  map[string]blob.Ref `json:"scripts,omitzero"`
	Machines map[string]blob.Ref `json:"machines,omitzero"`
	// Lines gives the file and the line each value is written at, by its
	// place, such as model.name or allow[1]: base.yaml:3.
	Lines map[string]string `json:"lines,omitzero"`
}

// How runs start at a location, and what a changed file does to them.
const (
	LifetimeSingleton = "singleton"
	LifetimeKeyed     = "keyed"
	LifetimeTransient = "transient"
	UpdateRestart     = "restart"
	UpdateDrain       = "drain"
)

// The defaults of a definition that says nothing.
const (
	DefaultKeep    = Duration(7 * 24 * 3600e9)
	DefaultDepth   = 16
	DefaultTimeout = Duration(5 * 60e9)
)

// A JobSpec is a job as a definition writes it, its values not yet filled
// in.
type JobSpec struct {
	Script  string       `json:"script,omitzero"`  // a path, or exactly one expression that gives a stored script
	Files   []string     `json:"files,omitzero"`   // the files the script loads
	Machine string       `json:"machine,omitzero"` // a name in Machines, or exactly one expression that gives a stored definition
	Send    *SendSpec    `json:"send,omitzero"`    // an event sent to a run by its address
	Call    *AddressSpec `json:"call,omitzero"`    // a location whose run the job starts and waits for
	Input   any          `json:"input,omitzero"`
	Allow   []string     `json:"allow,omitzero"`
	Deny    []string     `json:"deny,omitzero"`
	Timeout string       `json:"timeout,omitzero"` // a length of time, or an expression that gives one
	Restart *Restart     `json:"restart,omitzero"`
	Memory  int64        `json:"memory,omitzero"`
	Tags    []string     `json:"tags,omitzero"`
	Approve bool         `json:"approve,omitzero"`
	// Alone says the job has only its own permissions.
	Alone bool `json:"alone,omitzero"`
}

// IsMachine says whether the job is a machine.
func (s *JobSpec) IsMachine() bool { return s.Machine != "" }

// IsSend says whether the job sends an event, and IsCall whether it calls a
// location.
func (s *JobSpec) IsSend() bool { return s.Send != nil }
func (s *JobSpec) IsCall() bool { return s.Call != nil }

// A SendSpec is an event a job sends, as a definition writes it.
type SendSpec struct {
	To    AddressSpec `json:"to"`
	Event string      `json:"event"`
	Data  any         `json:"data,omitzero"`
}

// An AddressSpec is an address as a definition writes it: a location with a
// key or a run, each text or text with expressions, or exactly one
// expression that gives a whole address.
type AddressSpec struct {
	At   string `json:"at,omitzero"`
	Key  string `json:"key,omitzero"`
	Run  string `json:"run,omitzero"`
	Expr string `json:"expr,omitzero"`
}

// Fill gives the address with its values filled in, and checked.
func (s *AddressSpec) Fill(e *Evaluator, scope Scope, place string) (*Address, error) {
	a := &Address{}
	if s.Expr != "" {
		v, err := e.Text(s.Expr, scope, place)
		if err != nil {
			return nil, err
		}
		m, ok := v.(map[string]any)
		var key string
		known := 0
		for name, field := range map[string]*string{"at": &a.At, "key": &key, "run": &a.Run, "client": &a.Client} {
			if _, has := m[name]; has {
				known++
			}
			if text, isText := m[name].(string); isText {
				*field = text
			} else if m[name] != nil {
				ok = false
			}
		}
		if known != len(m) {
			ok = false // a field no address has
		}
		if _, isText := m["key"].(string); isText {
			a.Key = &key
		}
		if !ok || len(m) == 0 {
			return nil, fmt.Errorf("%s: %s gives %s, and an address is { at }, { at, key }, { at, run } or { client }", place, s.Expr, describe(v))
		}
	} else {
		for _, f := range []struct {
			written string
			into    *string
			name    string
		}{{s.At, &a.At, "at"}, {s.Run, &a.Run, "run"}} {
			if f.written == "" {
				continue
			}
			text, err := e.Key(f.written, scope, place+"."+f.name)
			if err != nil {
				return nil, err
			}
			*f.into = text
		}
		if s.Key != "" {
			key, err := e.Key(s.Key, scope, place+".key")
			if err != nil {
				return nil, err
			}
			a.Key = &key
		}
	}
	if err := a.Check(); err != nil {
		return nil, fmt.Errorf("%s: %w", place, err)
	}
	return a, nil
}

// Fill gives the job with its values filled in from a scope.
func (s *JobSpec) Fill(e *Evaluator, scope Scope, place, name string) (*Job, error) {
	job := &Job{Name: name, Files: s.Files, Deny: s.Deny, Restart: s.Restart,
		Memory: s.Memory, Tags: s.Tags, Approve: s.Approve, Alone: s.Alone}
	var err error
	if job.Input, err = e.Fill(s.Input, scope, place+".input"); err != nil {
		return nil, err
	}
	if job.Timeout, err = e.Duration(s.Timeout, scope, place+".timeout"); err != nil {
		return nil, err
	}
	if s.Send != nil {
		to, err := s.Send.To.Fill(e, scope, place+".send.to")
		if err != nil {
			return nil, err
		}
		data, err := e.Fill(s.Send.Data, scope, place+".send.data")
		if err != nil {
			return nil, err
		}
		job.Send = &Send{At: to, Event: s.Send.Event, Data: data}
	}
	if s.Call != nil {
		if job.Call, err = s.Call.Fill(e, scope, place+".call"); err != nil {
			return nil, err
		}
	}
	for i, entry := range s.Allow {
		filled, err := e.Entry(entry, Scope{"input": scope["input"]}, fmt.Sprintf("%s.allow[%d]", place, i))
		if err != nil {
			return nil, err
		}
		job.Allow = append(job.Allow, filled)
	}
	from, which := s.Script, "script"
	if s.IsMachine() {
		from, which = s.Machine, "machine"
	}
	if !HasExpr(from) {
		job.Script, job.Machine = s.Script, s.Machine
		return job, nil
	}
	stored, err := e.Text(from, scope, place+"."+which)
	if err != nil {
		return nil, err
	}
	if _, ok := blob.AsRef(stored); !ok || isDefinition(stored) != s.IsMachine() {
		return nil, fmt.Errorf("%s.%s: %s did not give a stored %s", place, which, from, which)
	}
	job.Stored = stored
	return job, nil
}

func isDefinition(v any) bool {
	ref, ok := blob.AsRef(v)
	return ok && ref.Type == blob.Definition
}

// A Problem is one thing wrong with a definition: in which file, where, and
// what. File is the path of the child definition's file, from the
// directory of the definition, and empty for the definition itself.
type Problem struct {
	File  string `json:"file,omitzero"`
	Place string `json:"place"`
	Text  string `json:"text"`
}

func (p Problem) String() string {
	var parts []string
	for _, part := range []string{p.File, p.Place, p.Text} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return oneLine(strings.Join(parts, ": "))
}

// Problems is everything wrong with a definition, reported together.
type Problems []Problem

func (ps Problems) Error() string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = p.String()
	}
	return strings.Join(lines, "\n")
}

// A Checker gathers what is wrong with a definition in full form. The
// client and the controller check with this same code.
type Checker struct {
	problems Problems
	def      *Definition
	// approves is set when a job of the definition may be held for approval.
	approves bool
	// jobs is every job the check visits, with its place.
	jobs []PlacedJob
	// values is every value an expression of the definition reads, by name.
	values map[string]bool
	// sends is every send and call the check visits, with its place.
	sends []PlacedSend
}

// A PlacedJob is a job of a definition and its place, such as
// stages.a.job.
type PlacedJob struct {
	Place string
	Job   *JobSpec
}

// Problem notes one thing wrong.
func (c *Checker) Problem(place, format string, args ...any) {
	c.problems = append(c.problems, Problem{Place: place, Text: fmt.Sprintf(format, args...)})
}

var nameForm = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Name checks a name: of a machine, a stage, a state, a tool, a job. An
// expression in a name is refused.
func (c *Checker) Name(place, name string) {
	switch {
	case HasExpr(name):
		c.Problem(place, "%q is a name, and a name cannot be an expression", name)
	case !nameForm.MatchString(name):
		c.Problem(place, "%q is not a name: use letters, digits, '_' and '-', starting with a letter", name)
	case name == "ask":
		c.Problem(place, "the name \"ask\" is taken by the definition's ask")
	}
}

// Approves notes that the definition holds something for approval other
// than a job marked for it, which needs an ask as such a job does.
func (c *Checker) Approves() { c.approves = true }

// Asked checks jobs that go to an ask of their own when they are held, an
// ask nearer than the definition's: they need no ask from the definition
// or above it. With no ask of their own they go to the definition's.
func (c *Checker) Asked(ask *JobSpec, check func()) {
	before := c.approves
	check()
	if ask != nil {
		c.approves = before
	}
}

// reading adds to the names a place gives what every expression reads.
func reading(names []string) []string { return append(slices.Clip(names), RunName, ValuesName) }

// text checks the expressions in a text, which may read the names, and
// notes the values they read.
func (c *Checker) text(place, text string, names []string) {
	if err := CheckText(text, names); err != nil {
		c.Problem(place, "%v", err)
		return
	}
	read, err := valuesRead(text)
	c.noteValues(place, read, err)
}

// noteValues notes the values an expression reads, each by the name it is
// written with.
func (c *Checker) noteValues(place string, read []string, err error) {
	if err != nil {
		c.Problem(place, "%v", err)
		return
	}
	if c.values == nil {
		c.values = map[string]bool{}
	}
	for _, name := range read {
		c.values[name] = true
	}
}

// Value checks the expressions in a value, which may read the names.
func (c *Checker) Value(place string, value any, names ...string) {
	switch v := value.(type) {
	case string:
		c.text(place, v, reading(names))
	case map[string]any:
		for _, k := range sortedKeys(v) {
			c.Value(place+"."+k, v[k], names...)
		}
	case []any:
		for i, item := range v {
			c.Value(fmt.Sprintf("%s[%d]", place, i), item, names...)
		}
	}
}

// Duration checks a length of time: written out, or with expressions,
// which may read the names.
func (c *Checker) Duration(place, text string, names ...string) {
	switch {
	case HasExpr(text):
		c.Value(place, text, names...)
	case text != "":
		if _, err := ParseDuration(text); err != nil {
			c.Problem(place, "%v", err)
		}
	}
}

// Cond checks a condition, which is a bare expression.
func (c *Checker) Cond(place, src string, names ...string) {
	if err := CheckExpr(src, reading(names)); err != nil {
		c.Problem(place, "%s: %v", shown(src), err)
		return
	}
	read, err := valuesReadOf(src)
	c.noteValues(place, read, err)
}

// Job checks a job, whose expressions may read the names.
func (c *Checker) Job(place string, s *JobSpec, names ...string) {
	if s == nil {
		return
	}
	c.jobs = append(c.jobs, PlacedJob{Place: place, Job: s})
	kinds := 0
	for _, is := range []bool{s.Script != "", s.Machine != "", s.IsSend(), s.IsCall()} {
		if is {
			kinds++
		}
	}
	switch {
	case kinds > 1:
		c.Problem(place, "a job is one of a script, a machine, a send or a call")
	case kinds == 0:
		c.Problem(place, "a job needs a script, a machine, a send or a call")
	case s.IsSend():
		if s.Input != nil || s.Timeout != "" || s.Restart != nil || s.Memory != 0 || len(s.Tags) > 0 || len(s.Files) > 0 {
			c.Problem(place, "a send takes to, event and data, and approve, allow and deny: it has no input, timeout, restart, memory, tags or files")
		}
		c.sends = append(c.sends, PlacedSend{Place: place + ".send.to", To: &s.Send.To, Event: s.Send.Event, Job: s})
		c.Address(place+".send.to", &s.Send.To, names...)
		switch e := s.Send.Event; {
		case e == "" || HasExpr(e):
			c.Problem(place+".send.event", "%q: the event's name is written out", e)
		case Reserved(e):
			c.Problem(place+".send.event", "%q: no machine can send an event named %s, %s, %s or %s", e, Start, Done, Failed, Timer)
		}
		c.Value(place+".send.data", s.Send.Data, names...)
	case s.IsCall():
		if s.Memory != 0 || len(s.Tags) > 0 || len(s.Files) > 0 {
			c.Problem(place, "memory, tags and files belong to a script; a call has none")
		}
		c.sends = append(c.sends, PlacedSend{Place: place + ".call", To: s.Call, Job: s})
		c.Address(place+".call", s.Call, names...)
		if s.Call.Key != "" || s.Call.Run != "" || s.Call.Expr != "" {
			c.Problem(place+".call", "a call names a location with at, and nothing else")
		}
	case s.IsMachine():
		if s.Memory != 0 || len(s.Tags) > 0 || len(s.Files) > 0 {
			c.Problem(place, "memory, tags and files belong to a script; a machine has none")
		}
		c.named(place+".machine", s.Machine, "machine", c.def.Machines, names)
	default:
		c.named(place+".script", s.Script, "script", c.def.Scripts, names)
		for i, file := range s.Files {
			c.named(fmt.Sprintf("%s.files[%d]", place, i), file, "file", c.def.Scripts, nil)
		}
	}
	c.Value(place+".input", s.Input, names...)
	c.Duration(place+".timeout", s.Timeout, names...)
	c.permissions(place, s.Allow, s.Deny)
	for i, tag := range s.Tags {
		if HasExpr(tag) || tag == "" {
			c.Problem(fmt.Sprintf("%s.tags[%d]", place, i), "%q is a tag, and a tag is written out", tag)
		}
	}
	if s.Memory < 0 {
		c.Problem(place, "memory cannot be less than nothing")
	}
	if r := s.Restart; r != nil && (r.Limit < 0 || r.Within < 0 || r.Delay <= 0 || r.Longest < r.Delay || r.Within > 0 && r.Limit == 0) {
		c.Problem(place+".restart", "a restart's limit and its time cannot be less than nothing, its time needs a limit, and its delay is more than nothing and doubles to a longer one")
	}
	c.approves = c.approves || s.Approve
}

// Address checks an address, whose expressions may read the names. A
// location written out is checked here, and one with expressions when it
// is filled in.
func (c *Checker) Address(place string, s *AddressSpec, names ...string) {
	if s.Expr != "" {
		if !IsOneExpr(s.Expr) {
			c.Problem(place, "%q: an address is { at }, { at, key } or { at, run }, or exactly one expression that gives one", s.Expr)
			return
		}
		c.Value(place, s.Expr, names...)
		return
	}
	switch {
	case s.At == "":
		c.Problem(place, "an address names its location with at")
	case HasExpr(s.At):
		c.Value(place+".at", s.At, names...)
	default:
		if err := CheckLocation(s.At); err != nil {
			c.Problem(place+".at", "%v", err)
		}
	}
	if s.Key != "" && s.Run != "" {
		c.Problem(place, "an address names a key or a run, not both")
	}
	c.Value(place+".key", s.Key, names...)
	c.Value(place+".run", s.Run, names...)
	if s.Key != "" && !HasExpr(s.Key) {
		if err := CheckKey(s.Key); err != nil {
			c.Problem(place+".key", "%v", err)
		}
	}
	if s.Run != "" && !HasExpr(s.Run) {
		if err := store.CheckInstance(s.Run); err != nil {
			c.Problem(place+".run", "%v", err)
		}
	}
}

// named checks the path of a script or the name of a machine: a name of the
// definition's, or exactly one expression.
func (c *Checker) named(place, name, what string, table map[string]blob.Ref, names []string) {
	if HasExpr(name) {
		if names == nil || !IsOneExpr(name) {
			c.Problem(place, "%q: the %s of a job is a name; it may be exactly one expression that gives a stored one, and nothing else", name, what)
		} else {
			c.text(place, name, reading(names))
		}
		return
	}
	if _, ok := table[name]; !ok {
		c.Problem(place, "the %s %q is not among those the definition holds", what, name)
	}
}

// permissions checks the lists of a layer written in a definition, wherever
// in the definition it is. A permission's kind and a secret's name are
// written out, a scope may be filled from the machine's input and its values
// and from nothing else, and a deny entry holds no expression. What a script gave
// back or a model asked for never chooses a scope.
func (c *Checker) permissions(place string, allow, deny []string) {
	for i, entry := range allow {
		at := fmt.Sprintf("%s.allow[%d]", place, i)
		if err := permission.CheckWritten(entry); err != nil {
			c.Problem(at, "%v", err)
		} else if err := CheckText(entry, []string{"input", ValuesName}); err != nil {
			c.Problem(at, "%q: a permission's scope is filled from the machine's input and its values, and from nothing else: %v", entry, err)
		} else {
			c.text(at, entry, []string{"input", ValuesName})
		}
	}
	for i, entry := range deny {
		at := fmt.Sprintf("%s.deny[%d]", place, i)
		if HasExpr(entry) {
			c.Problem(at, "%q: a deny entry is written out in full, with no expression", entry)
		} else if err := permission.CheckWritten(entry); err != nil {
			c.Problem(at, "%v", err)
		}
	}
}

// Check gives everything wrong with a definition in full form. The second
// result says whether any job in it is held for approval, which needs an
// ask in the definition or above it.
func Check(def *Definition) (Problems, bool) {
	c := check(def)
	return c.problems, c.approves || len(def.Approve) > 0
}

// Jobs gives every job of a definition with its place, in the order the
// check visits them.
func Jobs(def *Definition) []PlacedJob {
	return check(def).jobs
}

// Values gives the names of the values the expressions of a definition read,
// in order. Its child definitions are not read.
func Values(def *Definition) []string {
	return sortedKeys(check(def).values)
}

// Values gives the names of the values the expressions of a draft and of
// every child definition in it read, in order.
func (d *Draft) Values() []string {
	all := map[string]bool{}
	var add func(d *Draft)
	add = func(d *Draft) {
		for _, name := range Values(d.Definition) {
			all[name] = true
		}
		for _, child := range d.Children {
			add(child)
		}
	}
	add(d)
	return sortedKeys(all)
}

// Addressed gives what a send or a call job writes beside its input: the
// address and the event's data, each of which may hold expressions.
func (s *JobSpec) Addressed() []any {
	var written []any
	if s.Send != nil {
		written = append(written, s.Send.To.At, s.Send.To.Key, s.Send.To.Run, s.Send.To.Expr, s.Send.Data)
	}
	if s.Call != nil {
		written = append(written, s.Call.At)
	}
	return written
}

// Values gives the names of the values a job's expressions read, in order.
func (s *JobSpec) Values() []string {
	all := map[string]bool{}
	written := []any{s.Script, s.Machine, s.Input, s.Timeout}
	for _, entry := range s.Allow {
		written = append(written, entry)
	}
	written = append(written, s.Addressed()...)
	for _, read := range ReadsIn(written, ValuesName) {
		if read.Name != "*" {
			all[read.Name] = true
		}
	}
	return sortedKeys(all)
}

func check(def *Definition) *Checker {
	c := &Checker{def: def}
	t, ok := TypeOf(def.Type)
	if !ok {
		c.Problem("", "%q is not a type of machine (the types are: %s)", def.Type, strings.Join(Types(), ", "))
		return c
	}
	if HasExpr(def.Name) || !nameForm.MatchString(def.Name) {
		c.Problem(def.Type, "%q is not a name for a machine", def.Name)
	}
	if def.Input != nil {
		if schema, err := CompileShape(def.Input); err != nil {
			c.Problem("input", "%v", err)
		} else {
			for _, problem := range misplacedDefaults(schema) {
				c.Problem("input", "%s", problem)
			}
			if def.Lifetime == LifetimeSingleton {
				if err := Fits(schema, withDefaults(schema, map[string]any{})); err != nil {
					c.Problem("input", "a singleton is given no input, and this input needs some: %v", err)
				}
			}
		}
	}
	c.Duration("limits.timeout", def.Timeout, "input")
	c.Value("title", def.Title, "input")
	if (def.Depth != nil && *def.Depth < 0) || def.Jobs < 0 {
		c.Problem("", "depth and jobs cannot be less than nothing")
	}
	c.location(def)
	c.permissions("", def.Allow, def.Deny)
	if HasExpr(def.Description) {
		c.Problem("description", "a description is plain text, with no expression")
	}
	for i, entry := range def.Approve {
		at := fmt.Sprintf("approve[%d]", i)
		if HasExpr(entry) {
			c.Problem(at, "%q: a permission that needs a yes is written out in full, with no expression", entry)
		} else if err := permission.CheckWritten(entry); err != nil {
			c.Problem(at, "%v", err)
		}
	}
	c.Job("ask", def.Ask, "input")
	c.approves = false // the ask's own job is never itself held by this definition's marks
	t.Check(c, def)
	return c
}

// location checks what a file says of the runs at its location.
func (c *Checker) location(def *Definition) {
	switch def.Lifetime {
	case "", LifetimeSingleton, LifetimeTransient:
	case LifetimeKeyed:
		if !Receives(def) {
			c.Problem("lifetime", "a %s takes no events sent to it, so it cannot be keyed: only a statechart can", def.Type)
		}
	default:
		c.Problem("lifetime", "%q: lifetime is %s, %s or %s", def.Lifetime, LifetimeSingleton, LifetimeKeyed, LifetimeTransient)
	}
	switch def.Update {
	case "", UpdateRestart:
	case UpdateDrain:
		if def.Lifetime == LifetimeSingleton {
			c.Problem("update", "a singleton always restarts, and cannot drain")
		}
	default:
		c.Problem("update", "%q: update is %s or %s", def.Update, UpdateRestart, UpdateDrain)
	}
	switch {
	case def.Runs < 0:
		c.Problem("limits.runs", "runs is at least 1")
	case def.Runs > 0 && def.Lifetime != LifetimeKeyed:
		c.Problem("limits.runs", "runs limits the runs of a keyed file, and this file is not keyed")
	}
	for i, entry := range def.Read {
		at := fmt.Sprintf("read[%d]", i)
		name, ok := strings.CutPrefix(entry, "client:")
		switch {
		case HasExpr(entry):
			c.Problem(at, "%q: a read: entry is written out, with no expression", entry)
		case !ok:
			c.Problem(at, "%q: read: names clients, as client:wall, since only a client reads a record", entry)
		case name == "":
			c.Problem(at, "client: names a client, as client:wall")
		}
	}
	for _, event := range sortedKeys(def.Receive) {
		place := "receive"
		if event != "" {
			place = "receive." + event
			switch {
			case HasExpr(event):
				c.Problem(place, "%q: an event's name is written out", event)
			case Reserved(event):
				c.Problem(place, "%q: no run is sent an event named %s, %s, %s or %s", event, Start, Done, Failed, Timer)
			}
		}
		for i, entry := range def.Receive[event] {
			at := fmt.Sprintf("%s[%d]", place, i)
			if HasExpr(entry) {
				c.Problem(at, "%q: a receive: entry is written out, with no expression", entry)
			} else if name, ok := strings.CutPrefix(entry, "client:"); ok {
				if name == "" {
					c.Problem(at, "client: names a client, as client:panel")
				}
			} else if err := CheckPattern(entry); err != nil {
				c.Problem(at, "%v", err)
			}
		}
	}
}

// DecodeBody reads a definition's body into a type's own form, strictly: a
// field the type does not know is a problem.
func (c *Checker) DecodeBody(def *Definition, into any) bool {
	if err := json.Unmarshal(def.Body, into, json.RejectUnknownMembers(true)); err != nil {
		c.Problem("", "the stored definition is not one this type can read: %v", err)
		return false
	}
	return true
}

// Decode reads a stored definition, strictly.
func Decode(contents []byte) (*Definition, error) {
	var def Definition
	if err := json.Unmarshal(contents, &def, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("the stored definition cannot be read: %w", err)
	}
	return &def, nil
}

// ErrDefinition is a stored definition that cannot be used: it is not
// there, is not a definition, or cannot be read.
var ErrDefinition = errors.New("the stored definition cannot be used")

// Load reads a stored definition from the store, as plain JSON: nothing in
// it is compiled or evaluated. An error that is not ErrDefinition is the
// store's.
func Load(ctx context.Context, s store.Store, ref blob.Ref) (*Definition, error) {
	if ref.Type != blob.Definition {
		return nil, fmt.Errorf("%w: it is not a stored definition", ErrDefinition)
	}
	contents, err := blob.ReadAll(ctx, s, ref, blob.StepLimit)
	if blob.Unusable(err) {
		return nil, fmt.Errorf("%w: %w", ErrDefinition, err)
	}
	if err != nil {
		return nil, err
	}
	def, err := Decode(contents)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDefinition, err)
	}
	return def, nil
}

// A Draft is a definition filled out and checked, with the contents of the
// files it names and the drafts of its child definitions, ready to be
// stored.
type Draft struct {
	Definition *Definition
	Files      map[string][]byte
	Children   map[string]*Draft
	// needsAsk says a job in it is held for approval and it has no ask.
	needsAsk bool
	// asks are the machines it runs as an ask, each with its place.
	asks map[string]string
	// nearer says, of each machine it runs, whether only jobs that go to
	// an ask nearer than its own run it.
	nearer map[string]bool
	// size is the size of the definition as it is stored.
	size int
	// written is the definition as it is written out, with what it
	// includes in place.
	written *node
}

// Store writes a draft into an instance's record, and gives the stored
// definition's reference. What bounds how many requests are open at once is
// a store from store.Bounded.
func (d *Draft) Store(ctx context.Context, s store.Store, ctl, instance string) (blob.Ref, error) {
	return d.Write(ctx, func(typ string, contents []byte) (blob.Ref, error) {
		return blob.Write(ctx, s, store.Blob(ctl, instance, blob.NewName()), typ, blob.NewKey(), bytes.NewReader(contents))
	})
}

// Write writes a draft a blob at a time: its files and its child
// definitions side by side, and then the definition itself, which names
// them. write stores one blob of a type and gives its reference.
func (d *Draft) Write(ctx context.Context, write func(typ string, contents []byte) (blob.Ref, error)) (blob.Ref, error) {
	paths, names := sortedKeys(d.Files), sortedKeys(d.Children)
	refs := make([]blob.Ref, len(paths)+len(names))
	err := store.Each(ctx, store.SideBySide, len(refs), func(i int) (err error) {
		if i < len(paths) {
			refs[i], err = write(blob.TypeOf(paths[i]), d.Files[paths[i]])
		} else {
			refs[i], err = d.Children[names[i-len(paths)]].Write(ctx, write)
		}
		return err
	})
	if err != nil {
		return blob.Ref{}, err
	}
	contents, err := d.stored(refs)
	if err != nil {
		return blob.Ref{}, err
	}
	return write(blob.Definition, contents)
}

// stored gives the definition as it is stored, naming its files and then
// its child definitions, each in the order of its name, by refs.
func (d *Draft) stored(refs []blob.Ref) ([]byte, error) {
	paths, names := sortedKeys(d.Files), sortedKeys(d.Children)
	def := *d.Definition
	def.Scripts, def.Machines = map[string]blob.Ref{}, map[string]blob.Ref{}
	for i, path := range paths {
		def.Scripts[path] = refs[i].Local()
	}
	for i, name := range names {
		def.Machines[name] = refs[len(paths)+i].Local()
	}
	return json.Marshal(&def, json.Deterministic(true))
}

// storedSize gives the size of the definition as Store stores it, before
// anything is stored: each reference is one as long as Store makes, of a
// file or a child definition of the size it has.
func (d *Draft) storedSize() (int, error) {
	paths, names := sortedKeys(d.Files), sortedKeys(d.Children)
	refs := make([]blob.Ref, 0, len(paths)+len(names))
	for _, path := range paths {
		refs = append(refs, blob.Ref{At: blob.NewName(), Type: blob.TypeOf(path), Size: int64(len(d.Files[path])), Key: blob.NewKey()})
	}
	for _, name := range names {
		refs = append(refs, blob.Ref{At: blob.NewName(), Type: blob.Definition, Size: int64(d.Children[name].size), Key: blob.NewKey()})
	}
	contents, err := d.stored(refs)
	return len(contents), err
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
