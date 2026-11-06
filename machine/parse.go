package machine

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zummed/corbie/blob"
)

// A Parser fills a written definition out into its full form. Everything
// wrong is gathered and reported together.
type Parser struct {
	problems Problems
	noted    map[Problem]bool // every problem in problems, each once
	load     func(path string) ([]byte, error)
	dir      string           // where the definition's paths start from
	top      string           // the outermost definition's file, which a problem does not name
	source   string           // the machine file being read, which a problem names
	chain    []string         // every file being read, outermost first
	includes *int             // how many files have been included
	reads    map[string]*read // the files included, by their paths from the context
	at       map[string]*node // each value written out, by its place
	draft    *Draft
	inline   int
	limits   *Fields // the definition's limits, for a type to read its own
	nearer   bool    // the jobs being read go to an ask nearer than the definition's
	// inPlace says the definition being read is written in place, and made
	// that it was made while a machine runs: neither is a file at a
	// location.
	inPlace, made bool
}

// Parse reads a written definition, fills it out, and checks it. load gives
// the contents of a file the definition names, by its path. A definition
// that holds a job for approval and has no ask for it to go to is refused.
func Parse(src []byte, load func(path string) ([]byte, error)) (*Draft, error) {
	return parse(src, "", load, true)
}

// ParseIn is Parse for a definition kept in a file of the context, by its
// path from the context. load gives a file by its path from the context.
func ParseIn(file string, src []byte, load func(path string) ([]byte, error)) (*Draft, error) {
	return parse(src, file, load, true)
}

// ParseMade is Parse for a definition made while a machine runs. Whether
// its approvals have an ask to go to is left open: that is settled by what
// is above it when a job is held.
func ParseMade(src []byte, load func(path string) ([]byte, error)) (*Draft, error) {
	return parse(src, "", load, false)
}

func parse(src []byte, file string, load func(path string) ([]byte, error), root bool) (*Draft, error) {
	p := &Parser{load: load, dir: path.Dir(file), top: file, source: file, noted: map[Problem]bool{}, includes: new(int), reads: map[string]*read{}, made: !root}
	if file == "" {
		p.dir = ""
	} else {
		p.chain = []string{file}
	}
	draft, needsAsk := p.parse(src, "")
	if draft != nil && needsAsk && root {
		p.Problem("", "a job is marked for approval, or a permission is listed to be asked about, and there is no ask for it to go to: give the definition an ask")
	}
	if len(p.problems) > 0 {
		return nil, p.problems
	}
	return draft, nil
}

// Problem notes one thing wrong, at a place in the file being read.
func (p *Parser) Problem(place, format string, args ...any) {
	p.note(Problem{File: p.fileAt(place), Place: place, Text: fmt.Sprintf(format, args...)})
}

// problemIn notes one thing wrong, at a place in a file.
func (p *Parser) problemIn(file, place, format string, args ...any) {
	if file == p.top {
		file = ""
	}
	p.note(Problem{File: file, Place: place, Text: fmt.Sprintf(format, args...)})
}

func (p *Parser) note(problem Problem) {
	if !p.noted[problem] {
		p.noted[problem] = true
		p.problems = append(p.problems, problem)
	}
}

// parse reads one written definition. needsAsk says whether it, or a child
// that takes its ask from it, holds a job for approval with no ask to go to.
func (p *Parser) parse(src []byte, place string) (draft *Draft, needsAsk bool) {
	n, first, err := readYAML(src, p.source)
	if err != nil {
		p.Problem(place, "%v", err)
		return nil, false
	}
	if t, ok := TypeOf(first); ok {
		n = p.mapping(n, whole(t).field, "", p.chain)
	}
	p.at = map[string]*node{}
	p.index(n, "")
	return p.fill(plain(n).(map[string]any), first, place)
}

// fill fills out one definition, read into plain values, whose type is the
// name its first line gives.
func (p *Parser) fill(written map[string]any, first, place string) (draft *Draft, needsAsk bool) {
	t, ok := TypeOf(first)
	if !ok {
		p.Problem(place, "the first line gives %q, which is not a type of machine: a definition begins with its type and its name, such as \"flow: digest\" (the types are: %s)",
			first, strings.Join(Types(), ", "))
		return nil, false
	}
	outer := p.draft
	p.draft = &Draft{Definition: &Definition{Type: t.Name(), Scripts: map[string]blob.Ref{}, Machines: map[string]blob.Ref{}},
		Files: map[string][]byte{}, Children: map[string]*Draft{}}
	defer func() { p.draft = outer }()
	def := p.draft.Definition

	p.draft.written = p.at[""]
	def.Lines = p.lines()
	f := p.Fields("", written, definitionShape(t))
	def.Name = f.Text(t.Name())
	if input, _ := f.Map("input"); input != nil {
		if full, err := InputForm.Full(input); err != nil {
			p.Problem("input", "%v", err)
		} else {
			def.Input = full
		}
	}
	def.Keep = f.Duration("keep")
	limits := f.Fields("limits")
	def.Timeout = limits.text("timeout", KindDuration)
	if limits.Has("depth") {
		depth := limits.Int("depth")
		def.Depth = &depth
	}
	def.Jobs = limits.Int("jobs")
	def.Allow = f.Strings("allow")
	def.Deny = f.Strings("deny")
	def.AskAbout = f.Strings("ask_about")
	if ask, ok := f.Value("ask"); ok {
		def.Ask = p.Job("ask", ask)
		p.Ask("ask", def.Ask)
	}
	def.Title = f.Text("title")
	def.Meta, _ = f.Value("meta")
	p.location(def, f, limits)
	// What is left is the type's own, read by the type's own shape. A
	// field neither names is one that does not belong.
	f.read()
	outerLimits := p.limits
	p.limits = limits
	own := p.Fields("", f.Rest(), t.Shape())
	body := t.Fill(p, own)
	own.Done()
	limits.Done()
	p.limits = outerLimits
	var err error
	def.Body, err = json.Marshal(body, json.Deterministic(true))
	if err != nil {
		p.Problem("", "%v", err)
	}
	// A step reads a stored definition up to its limit, and one past it
	// could be stored and never read.
	if p.draft.size, err = p.draft.storedSize(); err != nil {
		p.Problem("", "%v", err)
	}
	if p.draft.size > blob.StepLimit {
		p.Problem(place, "the machine as it is stored is %d bytes, and a machine may be at most %d MiB", p.draft.size, blob.StepLimit>>20)
		return nil, false
	}
	// The whole is checked whatever the filling-out found, so that
	// everything wrong is reported together.
	problems, approves := Check(def)
	for _, problem := range problems {
		p.Problem(problem.Place, "%s", problem.Text)
	}
	// A child with no ask of its own takes its parent's. A machine run as
	// an ask takes none from above it.
	for name, child := range p.draft.Children {
		if place, isAsk := p.draft.asks[name]; isAsk {
			if child.needsAsk {
				p.Problem(place, "the machine run as the ask marks a job for approval, or lists a permission to be asked about, and has no ask of its own: a machine run as an ask takes no ask from above it")
			}
			continue
		}
		approves = approves || child.needsAsk && !p.draft.nearer[name]
	}
	p.draft.needsAsk = approves && def.Ask == nil
	return p.draft, p.draft.needsAsk
}

// location reads what a file says of the runs at its location. A machine
// written in place or made while running is no file at a location, and
// says none of it.
func (p *Parser) location(def *Definition, f, limits *Fields) {
	def.Lifetime = f.Text("lifetime")
	def.Update = f.Text("update")
	def.Runs = limits.Int("runs")
	if receive, ok := f.take("receive", KindList); ok {
		def.Receive = p.receive("receive", receive)
	}
	if p.inPlace || p.made {
		for _, name := range []string{"lifetime", "update", "receive"} {
			if f.Has(name) {
				p.Problem(name, "%s belongs to a file at a location, and a machine written in place or made while running has none", name)
			}
		}
		if limits.Has("runs") {
			p.Problem("limits.runs", "runs belongs to a file at a location, and a machine written in place or made while running has none")
		}
		def.Lifetime, def.Update, def.Runs, def.Receive = "", "", 0, nil
		return
	}
	if limits.Has("runs") && def.Runs < 1 {
		p.Problem("limits.runs", "runs is at least 1")
	}
	if def.Lifetime == "" {
		def.Lifetime = LifetimeTransient
	}
	if def.Update == "" {
		def.Update = UpdateDrain
		if def.Lifetime == LifetimeSingleton {
			def.Update = UpdateRestart
		}
	}
}

// receive reads who may send to a file's runs: a list for every event, or a
// mapping from an event's name to a list.
func (p *Parser) receive(place string, written any) map[string][]string {
	list := func(place string, v any) []string {
		if text, ok := v.(string); ok {
			return []string{text}
		}
		items, ok := v.([]any)
		if !ok {
			p.Problem(place, "this should be a list of locations and client:<name>, and is %s", describe(v))
			return nil
		}
		out := []string{}
		for i, item := range items {
			text, ok := item.(string)
			if !ok {
				p.Problem(fmt.Sprintf("%s[%d]", place, i), "this should be text, and is %s", describe(item))
				continue
			}
			out = append(out, text)
		}
		return out
	}
	m, ok := written.(map[string]any)
	if !ok {
		return map[string][]string{"": list(place, written)}
	}
	out := map[string][]string{}
	for _, event := range sortedKeys(m) {
		if event == "" {
			p.Problem(place, "an event's name is not empty")
			continue
		}
		out[event] = list(placeOf(place, event), m[event])
	}
	return out
}

// Address reads an address as it is written: a mapping, or exactly one
// expression that gives one.
func (p *Parser) Address(place string, written any) AddressSpec {
	if text, ok := written.(string); ok {
		if !IsOneExpr(text) {
			p.Problem(place, "%q: an address is { at }, { at, key } or { at, run }, or exactly one expression that gives one", text)
		}
		return AddressSpec{Expr: text}
	}
	f := p.Fields(place, written, AddressShape)
	if f == nil {
		return AddressSpec{}
	}
	defer f.Done()
	return AddressSpec{At: f.Text("at"), Key: f.Text("key"), Run: f.Text("run")}
}

// Ask notes a job the definition holds as an ask, at its place: a machine
// run as it takes no ask from above it.
func (p *Parser) Ask(place string, s *JobSpec) {
	if s == nil || s.Machine == "" {
		return
	}
	if p.draft.asks == nil {
		p.draft.asks = map[string]string{}
	}
	p.draft.asks[s.Machine] = place
}

// Nearer reads jobs that go to an ask nearer than the definition's. A
// machine that only such jobs run takes that ask as the nearest above it,
// and needs none from the definition.
func (p *Parser) Nearer(read func()) {
	outer := p.nearer
	p.nearer = true
	read()
	p.nearer = outer
}

// runs notes a machine a job runs, and whether every job that runs it goes
// to a nearer ask.
func (p *Parser) runs(name string) {
	if name == "" {
		return
	}
	if p.draft.nearer == nil {
		p.draft.nearer = map[string]bool{}
	}
	nearer, seen := p.draft.nearer[name]
	p.draft.nearer[name] = p.nearer && (nearer || !seen)
}

// Limits gives the definition's limits, for a type to read those it adds.
func (p *Parser) Limits() *Fields { return p.limits }

// Job reads a job as it is written: a mapping, or just the path of a
// script. It loads the script and its files, or the machine.
func (p *Parser) Job(place string, written any) *JobSpec {
	s := &JobSpec{}
	if text, ok := written.(string); ok {
		written = map[string]any{"script": text}
	}
	f := p.Fields(place, written, JobShape)
	if f == nil {
		return s
	}
	s.Script = p.cleanPath(f.Text("script"))
	for _, file := range f.Strings("files") {
		s.Files = append(s.Files, p.cleanPath(file))
	}
	machine, hasMachine := f.Value("machine")
	if sf := f.Fields("send"); sf != nil {
		to, _ := sf.Value("to")
		s.Send = &SendSpec{To: p.Address(placeOf(place, "send.to"), to), Event: sf.Text("event")}
		s.Send.Data, _ = sf.Value("data")
		sf.Done()
	}
	if cf := f.Fields("call"); cf != nil {
		s.Call = &AddressSpec{At: cf.Text("at")}
		cf.Done()
	}
	s.Key = f.Text("key")
	s.Input, _ = f.Value("input")
	s.Allow = f.Strings("allow")
	s.Deny = f.Strings("deny")
	s.Timeout = f.text("timeout", KindDuration)
	s.Restart = p.restart(f.at("restart"), f.Fields("restart"))
	s.Memory = f.Size("memory")
	s.Tags = f.Strings("tags")
	s.Approve = f.Bool("approve")
	f.Done()

	if s.Script != "" && !HasExpr(s.Script) {
		for _, file := range append([]string{s.Script}, s.Files...) {
			p.file(place, file)
		}
	}
	if hasMachine {
		s.Machine = p.Machine(place+".machine", machine)
	}
	return s
}

// Machine reads a machine a definition runs, as it is written: the path of a
// definition, or a definition written in place. It loads it, and gives the
// name the definition has for it.
func (p *Parser) Machine(place string, written any) (name string) {
	defer func() { p.runs(name) }()
	switch m := written.(type) {
	case string:
		name = p.cleanPath(m)
		if !HasExpr(m) {
			p.machineFile(place, name)
		}
	case map[string]any:
		// A machine written in place.
		p.inline++
		name = fmt.Sprintf("in-place-%d", p.inline)
		first := ""
		for _, kind := range Types() {
			if _, ok := m[kind]; ok {
				first = kind
			}
		}
		if first == "" {
			p.Problem(place, "a machine written in place writes its type and its name itself, such as flow: digest (the types are: %s)", strings.Join(Types(), ", "))
			return name
		}
		p.child(place, name, func(sub *Parser) (*Draft, bool) {
			sub.at, sub.inPlace = below(p.at, place), true
			return sub.fill(m, first, place)
		}, p.dir, "")
	default:
		p.Problem(place, "a machine is the path of a definition, or a definition written in place")
	}
	return name
}

// Builtin adds a script that comes with Corbie to the definition's files,
// under a name no path can have.
func (p *Parser) Builtin(name, source string) {
	p.draft.Files[name] = []byte(source)
	p.draft.Definition.Scripts[name] = blob.Ref{}
}

// Given gives a loader of the files handed over with a definition as text,
// each under the path the definition names it by.
func Given(files map[string]string) func(string) ([]byte, error) {
	given := map[string]string{}
	for name, text := range files {
		given[path.Clean(name)] = text
	}
	return func(name string) ([]byte, error) {
		if text, ok := given[name]; ok {
			return []byte(text), nil
		}
		return nil, fmt.Errorf("it was not given with the definition")
	}
}

// MachineType gives the type of a machine the definition names, as a job
// names it, or nothing for one that is not known until it runs.
func (p *Parser) MachineType(name string) string {
	if child := p.draft.Children[name]; child != nil && child.Definition != nil {
		return child.Definition.Type
	}
	return ""
}

// cleanPath gives a path as a definition keeps it: from the context, starting
// with ./, however it was written. A path that leaves the context, an
// expression and one of Corbie's own scripts are given as they are.
func (p *Parser) cleanPath(name string) string {
	switch {
	case name == "", HasExpr(name), strings.HasPrefix(name, "corbie:"), !inContext(name):
		return name
	case fromFile(name):
		return "./" + path.Join(p.dir, name)
	}
	return "./" + path.Clean(name)
}

// fromFile says a path is read from the directory of the file it is in.
func fromFile(name string) bool { return strings.HasPrefix(name, "./") }

// inContext says a path can stay in the context: one that climbs out of a
// directory, or starts at the top of the disk, cannot.
func inContext(name string) bool {
	return !path.IsAbs(name) && !slices.Contains(strings.Split(name, "/"), "..")
}

// kept gives the path from the context of a path as a definition keeps it,
// which is what load is given.
func kept(name string) string { return strings.TrimPrefix(name, "./") }

// outside is what is said of a path that leaves the context.
const outside = "a path is read from the context, or from this file's directory with ./, and has no ../"

// restart reads a job's restart policy.
func (p *Parser) restart(place string, f *Fields) *Restart {
	if f == nil {
		return nil
	}
	r := &Restart{Delay: DefaultDelay, Longest: DefaultLongest}
	switch when := f.Text("when"); when {
	case "", RestartFailure:
	case RestartAlways:
		r.Always = true
	default:
		p.Problem(placeOf(place, "when"), "%q: when is %s or %s", when, RestartFailure, RestartAlways)
	}
	if limit, ok := f.Value("limit"); ok {
		var err error
		if r.Limit, r.Within, err = parseLimit(limit); err != nil {
			p.Problem(placeOf(place, "limit"), "%v", err)
		}
	}
	if delay := f.Text("delay"); delay != "" {
		var err error
		if r.Delay, r.Longest, err = parseDelay(delay); err != nil {
			p.Problem(placeOf(place, "delay"), "%v", err)
		}
	}
	f.Done()
	return r
}

// parseLimit reads a limit: a number, or a number in a length of time.
func parseLimit(written any) (int, Duration, error) {
	if n, ok := written.(float64); ok && n == math.Trunc(n) && n >= 1 && n <= math.MaxInt32 {
		return int(n), 0, nil
	}
	text, ok := written.(string)
	if !ok {
		return 0, 0, fmt.Errorf("a limit is a number of at least 1, such as 3, or a number in a length of time, such as 5 in 1h")
	}
	count, span, ok := strings.Cut(text, " in ")
	n, err := strconv.Atoi(strings.TrimSpace(count))
	if !ok || err != nil || n < 1 {
		return 0, 0, fmt.Errorf("%q: a limit is a number of at least 1, such as 3, or a number in a length of time, such as 5 in 1h", text)
	}
	within, err := ParseDuration(strings.TrimSpace(span))
	if err != nil {
		return 0, 0, err
	}
	if within <= 0 {
		return 0, 0, fmt.Errorf("%q: the length of time of a limit is more than nothing", text)
	}
	return n, within, nil
}

// parseDelay reads a delay: a length of time, or two with to between them.
func parseDelay(text string) (first, longest Duration, err error) {
	from, to, grows := strings.Cut(text, " to ")
	if !grows {
		to = from
	}
	if first, err = ParseDuration(strings.TrimSpace(from)); err != nil {
		return 0, 0, err
	}
	if longest, err = ParseDuration(strings.TrimSpace(to)); err != nil {
		return 0, 0, err
	}
	if first <= 0 || longest < first {
		return 0, 0, fmt.Errorf("%q: a delay is more than nothing, and doubles to a longer one", text)
	}
	return first, longest, nil
}

// file loads a script or a file a script loads.
func (p *Parser) file(place, name string) {
	if _, ok := p.draft.Files[name]; ok {
		return
	}
	if strings.HasPrefix(name, "corbie:") {
		return // one of Corbie's own, added by the type that uses it
	}
	if !fromFile(name) || !inContext(name) {
		p.draft.Definition.Scripts[name] = blob.Ref{}
		p.Problem(place, "%q: %s", name, outside)
		return
	}
	// Named once, it is in the definition's table, read or not: what is
	// wrong with it is said here and not again by the check.
	p.draft.Definition.Scripts[name] = blob.Ref{}
	if contents, ok := p.read(place, name, kept(name)); ok {
		p.draft.Files[name] = contents
	}
}

// machineFile loads a child definition kept in its own file.
func (p *Parser) machineFile(place, name string) {
	if _, ok := p.draft.Children[name]; ok {
		return
	}
	if !fromFile(name) || !inContext(name) {
		p.draft.Definition.Machines[name] = blob.Ref{}
		p.Problem(place, "%q: %s", name, outside)
		return
	}
	full := kept(name)
	src, ok := p.open(place, name, full)
	if !ok {
		p.draft.Definition.Machines[name] = blob.Ref{}
		return
	}
	p.child(place, name, func(sub *Parser) (*Draft, bool) { return sub.parse(src, "") }, path.Dir(full), full)
}

// open reads a file of the definition, by its full path, that is filled
// in as part of it. A file already being read would lead back to itself.
func (p *Parser) open(place, name, full string) ([]byte, bool) {
	if chain := p.reading(place); slices.Contains(chain, full) {
		p.Problem(place, "the definition leads back to itself: %s -> %s", strings.Join(chain, " -> "), full)
		return nil, false
	}
	return p.read(place, name, full)
}

// read reads a file, by its full path.
func (p *Parser) read(place, name, full string) ([]byte, bool) {
	contents, err := p.load(full)
	if err != nil {
		p.Problem(place, "%q cannot be read: %v", name, err)
		return nil, false
	}
	return contents, true
}

func (p *Parser) child(place, name string, read func(*Parser) (*Draft, bool), dir, file string) {
	p.draft.Definition.Machines[name] = blob.Ref{}
	chain := p.reading(place)
	sub := &Parser{load: p.load, dir: dir, top: p.top, source: p.source, chain: chain, includes: p.includes, reads: p.reads,
		problems: p.problems, noted: p.noted, inline: p.inline * 100, made: p.made}
	if file != "" {
		sub.source, sub.chain = file, append(chain, file)
	}
	child, _ := read(sub)
	p.problems = sub.problems
	if child != nil {
		p.draft.Children[name] = child
	}
}

// Fields reads the fields of a mapping one at a time, by the mapping's
// shape: it reads no field the shape does not name, and any field left
// when it is done is one that does not belong.
type Fields struct {
	p     *Parser
	place string
	shape *Shape
	m     map[string]any
	taken map[string]bool
	asked map[string]bool // the fields the reader has asked for, written or not
}

// Fields starts reading a mapping of a shape. A value that is not a
// mapping is a problem, and gives nil.
func (p *Parser) Fields(place string, v any, shape *Shape) *Fields {
	m, ok := v.(map[string]any)
	if !ok {
		p.Problem(place, "this should be a mapping of fields, and is %s", describe(v))
		return nil
	}
	return &Fields{p: p, place: place, shape: shape, m: m, taken: map[string]bool{}, asked: map[string]bool{}}
}

// named gives the shape's field of a name, for a reader that reads it as
// one of some kinds, or as whatever it is. A reader that reads a field the
// shape does not name, or reads it as a kind the shape does not give it,
// is a fault in Corbie and not in the definition, and is said as one: a
// shape that did not hold its reader would be no account of what is read.
func (f *Fields) named(name string, as ...Kind) *Field {
	f.asked[name] = true
	field := f.shape.field(name)
	switch {
	case field == nil:
		f.p.Problem(f.at(name), "Corbie reads a field %q here that its own account of %s does not name: this is a fault in Corbie", name, f.shape.What)
	case !field.Kind.reads(as):
		f.p.Problem(f.at(name), "Corbie reads the field %q here as another kind of value than its own account of %s gives it: this is a fault in Corbie", name, f.shape.What)
	}
	return field
}

// Fields starts reading the mapping a field holds, by the shape the field
// gives it. A field that is not written gives nil.
func (f *Fields) Fields(name string) *Fields {
	if f == nil {
		return nil
	}
	field := f.named(name, KindFields)
	v, ok := f.m[name]
	f.taken[name] = true
	if !ok || field == nil {
		return nil
	}
	return f.p.Fields(f.at(name), v, field.Holds())
}

// Of gives the shape of the mappings a field holds: of each named thing,
// or of a mapping written where a shorter form could be.
func (f *Fields) Of(name string) *Shape {
	if f == nil {
		return nil
	}
	field := f.named(name)
	if field == nil {
		return nil
	}
	if field.Holds() == nil {
		f.p.Problem(f.at(name), "Corbie reads mappings in the field %q here, and its own account of %s gives them no shape: this is a fault in Corbie", name, f.shape.What)
	}
	return field.Holds()
}

func (f *Fields) at(name string) string { return placeOf(f.place, name) }

// Place is where in the definition the mapping is.
func (f *Fields) Place() string { return f.place }

// Value takes a field that may hold anything: a value, or a job as it is
// written.
func (f *Fields) Value(name string) (any, bool) {
	return f.take(name, KindValue, KindJob)
}

// take takes a field that the shape gives one of some kinds.
func (f *Fields) take(name string, as ...Kind) (any, bool) {
	if f == nil {
		return nil, false
	}
	f.named(name, as...)
	v, ok := f.m[name]
	f.taken[name] = true
	return v, ok
}

// Has says whether the field is written, without taking it.
func (f *Fields) Has(name string) bool {
	if f == nil {
		return false
	}
	f.named(name)
	_, ok := f.m[name]
	return ok
}

// Text takes a field that is text.
func (f *Fields) Text(name string) string {
	return f.text(name, KindWord, KindText)
}

func (f *Fields) text(name string, as ...Kind) string {
	v, ok := f.take(name, as...)
	if !ok {
		return ""
	}
	switch v := v.(type) {
	case string:
		return v
	case float64, bool:
		return fmt.Sprint(v)
	}
	f.p.Problem(f.at(name), "this should be text, and is %s", describe(v))
	return ""
}

// Cond takes a field that is a condition: exactly one expression, written
// ${…}. It gives the expression's source.
func (f *Fields) Cond(name string) string {
	text := f.text(name, KindWord)
	if text == "" {
		return ""
	}
	src, ok := OneExpr(text)
	if !ok {
		f.p.Problem(f.at(name), "%q: a condition is one expression, written ${…}", text)
	}
	return src
}

// Int takes a field that is a whole number.
func (f *Fields) Int(name string) int {
	v, ok := f.take(name, KindNumber)
	if !ok {
		return 0
	}
	if n, ok := v.(float64); ok && n == math.Trunc(n) {
		return int(n)
	}
	f.p.Problem(f.at(name), "this should be a whole number, and is %s", describe(v))
	return 0
}

// Bool takes a field that is true or false.
func (f *Fields) Bool(name string) bool {
	v, ok := f.take(name, KindFlag)
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		f.p.Problem(f.at(name), "this should be true or false, and is %s", describe(v))
	}
	return b
}

// Duration takes a field that is a length of time.
func (f *Fields) Duration(name string) Duration {
	text := f.text(name, KindDuration)
	if text == "" {
		return 0
	}
	d, err := ParseDuration(text)
	if err != nil {
		f.p.Problem(f.at(name), "%v", err)
	}
	return d
}

// Size takes a field that is a size in bytes, written like 256MB.
func (f *Fields) Size(name string) int64 {
	text := f.text(name, KindSize)
	if text == "" {
		return 0
	}
	units := []struct {
		suffix string
		bytes  int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}, {"", 1}}
	for _, u := range units {
		if number, ok := strings.CutSuffix(text, u.suffix); ok {
			if n, err := strconv.ParseFloat(strings.TrimSpace(number), 64); err == nil && n >= 0 {
				return int64(n * float64(u.bytes))
			}
		}
	}
	f.p.Problem(f.at(name), "%q is not a size (write it like 256MB)", text)
	return 0
}

// Strings takes a field that is a list of texts; one text alone is a list
// of one.
func (f *Fields) Strings(name string) []string {
	v, ok := f.take(name, KindList)
	if !ok || v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return []string{s}
	}
	list, ok := v.([]any)
	if !ok {
		f.p.Problem(f.at(name), "this should be a list, and is %s", describe(v))
		return nil
	}
	var out []string
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			f.p.Problem(fmt.Sprintf("%s[%d]", f.at(name), i), "this should be text, and is %s", describe(item))
			continue
		}
		out = append(out, s)
	}
	return out
}

// Map takes a field that is a mapping, and gives its names in order.
func (f *Fields) Map(name string) (map[string]any, []string) {
	v, ok := f.take(name, KindNamed, KindTypes)
	if !ok || v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		f.p.Problem(f.at(name), "this should be a mapping, and is %s", describe(v))
		return nil, nil
	}
	return m, sortedKeys(m)
}

// Rest gives the fields not yet taken, for a type to read.
func (f *Fields) Rest() map[string]any {
	rest := map[string]any{}
	if f == nil {
		return rest
	}
	for k, v := range f.m {
		if !f.taken[k] {
			rest[k] = v
		}
	}
	return rest
}

// Done reports every field not taken as one that is not known here.
func (f *Fields) Done() {
	if f == nil {
		return
	}
	for _, k := range sortedKeys(f.m) {
		if !f.taken[k] {
			f.p.Problem(f.at(k), "%q is not a field that belongs here", k)
		}
	}
	f.read()
}

// read reports a field the shape names that the reader never asked for.
// Such a field would be offered to a person and then refused, so it is a
// fault in Corbie, found the first time any definition is read.
func (f *Fields) read() {
	for _, field := range f.shape.Fields {
		if !f.asked[field.Name] {
			f.p.Problem(f.place, "Corbie's own account of %s names a field %q that it does not read: this is a fault in Corbie", f.shape.What, field.Name)
		}
	}
}

func describe(v any) string {
	switch v.(type) {
	case nil:
		return "empty"
	case string:
		return "text"
	case float64:
		return "a number"
	case bool:
		return "true or false"
	case []any:
		return "a list"
	case map[string]any:
		return "a mapping"
	}
	return fmt.Sprintf("%T", v)
}

// readYAML reads a written definition, from a file, into its values: mappings,
// lists, text, numbers, true and false. It gives the top mapping and the name
// on its first line.
func readYAML(src []byte, file string) (*node, string, error) {
	n, top, err := nodeYAML(src, file)
	switch {
	case err != nil:
		return nil, "", err
	case top == nil:
		return nil, "", fmt.Errorf("the definition is empty")
	case !n.isMap() || len(n.keys) == 0:
		return nil, "", fmt.Errorf("a definition is a mapping that begins with its type and its name")
	}
	return n, n.keys[0], nil
}

// nodeYAML reads one YAML document of a file into its values, as a
// definition is read: a date stays the text written. It gives its top node,
// or nothing for an empty document. The YAML reader refuses an alias
// that contains itself, or a document whose aliases make far more than it
// writes, only when it decodes: a tree of nodes holds each alias once. So a
// document with an alias is decoded first. Only then, since decoding compares
// every two names of a mapping. A type that does not fit is no fault here, and
// is said of the values.
func nodeYAML(src []byte, file string) (*node, *yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		text := oneLine(strings.TrimPrefix(err.Error(), "yaml: "))
		if strings.Contains(string(src), "${") {
			text += ` (if a value starts with ${ or holds one beside a colon or a brace, put it in quotes: "${…}")`
		}
		return nil, nil, fmt.Errorf("this is not YAML that can be read: %s", text)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil, nil
	}
	top := doc.Content[0]
	if aliased(top) {
		var decoded any
		var wrongType *yaml.TypeError
		if err := top.Decode(&decoded); err != nil && !errors.As(err, &wrongType) {
			return nil, nil, fmt.Errorf("this is not YAML that can be read: %s", oneLine(strings.TrimPrefix(err.Error(), "yaml: ")))
		}
	}
	n, err := fromYAML(top, file)
	return n, top, err
}

// aliased says whether a node is an alias or holds one.
func aliased(n *yaml.Node) bool {
	return n.Kind == yaml.AliasNode || slices.ContainsFunc(n.Content, aliased)
}

// fromYAML turns a YAML node of a file into a value.
func fromYAML(n *yaml.Node, file string) (*node, error) {
	out := &node{file: file, line: n.Line}
	switch n.Kind {
	case yaml.AliasNode:
		return fromYAML(n.Alias, file)
	case yaml.MappingNode:
		out.m = map[string]*node{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: the name of a field is plain text", k.Line)
			}
			if _, dup := out.m[k.Value]; dup {
				return nil, fmt.Errorf("line %d: the field %q is written twice", k.Line, k.Value)
			}
			v, err := fromYAML(n.Content[i+1], file)
			if err != nil {
				return nil, err
			}
			out.set(k.Value, v)
		}
		return out, nil
	case yaml.SequenceNode:
		out.list, out.items = true, make([]*node, 0, len(n.Content))
		for _, item := range n.Content {
			v, err := fromYAML(item, file)
			if err != nil {
				return nil, err
			}
			out.items = append(out.items, v)
		}
		return out, nil
	case yaml.ScalarNode:
		// A plain value YAML would read as a date stays the text written.
		out.value = n.Value
		switch n.ShortTag() {
		case "!!null":
			out.value = nil
		case "!!bool":
			out.value = n.Value == "true" || n.Value == "True" || n.Value == "TRUE"
		case "!!int", "!!float":
			var f float64
			if err := n.Decode(&f); err == nil {
				out.value = f
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("line %d: this part of the definition cannot be read", n.Line)
}
