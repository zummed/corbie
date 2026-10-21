package machine

import (
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
	Depth   *int           `json:"depth,omitzero"`   // how many levels of machines may be below it; eight if not said
	Jobs    int            `json:"jobs,omitzero"`    // how many jobs an instance may start
	Allow   []string       `json:"allow,omitzero"`
	Deny    []string       `json:"deny,omitzero"`
	// AskAbout lists the permissions that must be asked about.
	AskAbout []string `json:"ask_about,omitzero"`
	Ask      *JobSpec `json:"ask,omitzero"`
	// Body is the type's own part.
	Body jsontext.Value `json:"body"`
	// Scripts and Machines are the files and child definitions the
	// definition names, each a blob beside the stored definition.
	Scripts  map[string]blob.Ref `json:"scripts,omitzero"`
	Machines map[string]blob.Ref `json:"machines,omitzero"`
}

// The defaults of a definition that says nothing.
const (
	DefaultKeep    = Duration(7 * 24 * 3600e9)
	DefaultDepth   = 8
	DefaultTimeout = Duration(5 * 60e9)
)

// A JobSpec is a job as a definition writes it, its values not yet filled
// in.
type JobSpec struct {
	Script   string   `json:"script,omitzero"`  // a path, or exactly one expression that gives a stored script
	Files    []string `json:"files,omitzero"`   // the files the script loads
	Machine  string   `json:"machine,omitzero"` // a name in Machines, or exactly one expression that gives a stored definition
	Input    any      `json:"input,omitzero"`
	Allow    []string `json:"allow,omitzero"`
	Deny     []string `json:"deny,omitzero"`
	Timeout  string   `json:"timeout,omitzero"` // a length of time, or an expression that gives one
	Attempts int      `json:"attempts,omitzero"`
	Memory   int64    `json:"memory,omitzero"`
	Tags     []string `json:"tags,omitzero"`
	Approve  bool     `json:"approve,omitzero"`
	// Alone says the job has only its own permissions.
	Alone bool `json:"alone,omitzero"`
}

// IsMachine says whether the job is a machine.
func (s *JobSpec) IsMachine() bool { return s.Machine != "" }

// Fill gives the job with its values filled in from a scope.
func (s *JobSpec) Fill(e *Evaluator, scope Scope, place, name string) (*Job, error) {
	job := &Job{Name: name, Files: s.Files, Deny: s.Deny, Attempts: s.Attempts,
		Memory: s.Memory, Tags: s.Tags, Approve: s.Approve, Alone: s.Alone}
	var err error
	if job.Input, err = e.Fill(s.Input, scope, place+".input"); err != nil {
		return nil, err
	}
	if job.Timeout, err = e.Duration(s.Timeout, scope, place+".timeout"); err != nil {
		return nil, err
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

// Value checks the expressions in a value, which may read the names.
func (c *Checker) Value(place string, value any, names ...string) {
	switch v := value.(type) {
	case string:
		if err := CheckText(v, names); err != nil {
			c.Problem(place, "%v", err)
		}
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
	if err := CheckExpr(src, names); err != nil {
		c.Problem(place, "%s: %v", oneLine(src), err)
	}
}

// Job checks a job, whose expressions may read the names.
func (c *Checker) Job(place string, s *JobSpec, names ...string) {
	if s == nil {
		return
	}
	c.jobs = append(c.jobs, PlacedJob{Place: place, Job: s})
	switch {
	case s.Script != "" && s.Machine != "":
		c.Problem(place, "a job is a script or a machine, not both")
	case s.Script == "" && s.Machine == "":
		c.Problem(place, "a job needs a script or a machine")
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
	if s.Attempts < 0 || s.Memory < 0 {
		c.Problem(place, "attempts and memory cannot be less than nothing")
	}
	c.approves = c.approves || s.Approve
}

// named checks the path of a script or the name of a machine: a name of the
// definition's, or exactly one expression.
func (c *Checker) named(place, name, what string, table map[string]blob.Ref, names []string) {
	if HasExpr(name) {
		if names == nil || !IsOneExpr(name) {
			c.Problem(place, "%q: the %s of a job is a name; it may be exactly one expression that gives a stored one, and nothing else", name, what)
		} else if err := CheckText(name, names); err != nil {
			c.Problem(place, "%v", err)
		}
		return
	}
	if _, ok := table[name]; !ok {
		c.Problem(place, "the %s %q is not among those the definition holds", what, name)
	}
}

// permissions checks the lists of a layer written in a definition, wherever
// in the definition it is. A permission's kind and a secret's name are
// written out, a scope may be filled from the machine's input and from
// nothing else, and a deny entry holds no expression. What a script gave
// back or a model asked for never chooses a scope.
func (c *Checker) permissions(place string, allow, deny []string) {
	for i, entry := range allow {
		at := fmt.Sprintf("%s.allow[%d]", place, i)
		if err := permission.CheckWritten(entry); err != nil {
			c.Problem(at, "%v", err)
		} else if err := CheckText(entry, []string{"input"}); err != nil {
			c.Problem(at, "%q: a permission's scope is filled from the machine's input and from nothing else: %v", entry, err)
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
	return c.problems, c.approves || len(def.AskAbout) > 0
}

// Jobs gives every job of a definition with its place, in the order the
// check visits them.
func Jobs(def *Definition) []PlacedJob {
	return check(def).jobs
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
		}
	}
	c.Duration("limits.timeout", def.Timeout, "input")
	if (def.Depth != nil && *def.Depth < 0) || def.Jobs < 0 {
		c.Problem("", "depth and jobs cannot be less than nothing")
	}
	c.permissions("", def.Allow, def.Deny)
	for i, entry := range def.AskAbout {
		at := fmt.Sprintf("ask_about[%d]", i)
		if HasExpr(entry) {
			c.Problem(at, "%q: a permission to be asked about is written out in full, with no expression", entry)
		} else if err := permission.CheckWritten(entry); err != nil {
			c.Problem(at, "%v", err)
		}
	}
	c.Job("ask", def.Ask, "input")
	c.approves = false // the ask's own job is never itself held by this definition's marks
	t.Check(c, def)
	return c
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

// Load reads a stored definition from the store. An error that is not
// ErrDefinition is the store's.
func Load(ctx context.Context, s store.Store, ref blob.Ref) (*Definition, error) {
	if ref.Type != blob.Definition {
		return nil, fmt.Errorf("%w: it is not a stored definition", ErrDefinition)
	}
	contents, err := blob.ReadAll(ctx, s, ref, blob.StepLimit)
	switch {
	case blob.Unusable(err):
		return nil, fmt.Errorf("%w: %w", ErrDefinition, err)
	case err != nil:
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
}

// Store writes a draft into an instance's record: its files and its child
// definitions side by side, and then the definition itself, which names
// them. It gives the stored definition's reference. What bounds how many
// requests are open at once is a store from store.Bounded.
func (d *Draft) Store(ctx context.Context, s store.Store, ctl, instance string) (blob.Ref, error) {
	paths, names := sortedKeys(d.Files), sortedKeys(d.Children)
	refs := make([]blob.Ref, len(paths)+len(names))
	err := store.Each(ctx, store.SideBySide, len(refs), func(i int) (err error) {
		if i < len(paths) {
			refs[i], err = blob.Write(ctx, s, store.Blob(ctl, instance, blob.NewName()), blob.TypeOf(paths[i]), blob.NewKey(),
				strings.NewReader(string(d.Files[paths[i]])))
		} else {
			refs[i], err = d.Children[names[i-len(paths)]].Store(ctx, s, ctl, instance)
		}
		return err
	})
	if err != nil {
		return blob.Ref{}, err
	}
	def := *d.Definition
	def.Scripts, def.Machines = map[string]blob.Ref{}, map[string]blob.Ref{}
	for i, path := range paths {
		def.Scripts[path] = refs[i].Local()
	}
	for i, name := range names {
		def.Machines[name] = refs[len(paths)+i].Local()
	}
	contents, err := json.Marshal(&def, json.Deterministic(true))
	if err != nil {
		return blob.Ref{}, err
	}
	return blob.Write(ctx, s, store.Blob(ctl, instance, blob.NewName()), blob.Definition, blob.NewKey(),
		strings.NewReader(string(contents)))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
