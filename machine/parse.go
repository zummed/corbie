package machine

import (
	"encoding/json/v2"
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
	load     func(path string) ([]byte, error)
	dir      string   // where the definition's paths start from
	in       string   // the directory of the file being read, from dir
	chain    []string // the machine files being read, outermost first
	draft    *Draft
	inline   int
	limits   *Fields // the definition's limits, for a type to read its own
}

// Parse reads a written definition, fills it out, and checks it. load gives
// the contents of a file the definition names, by its path. A definition
// that holds a job for approval and has no ask for it to go to is refused.
func Parse(src []byte, load func(path string) ([]byte, error)) (*Draft, error) {
	return parse(src, load, true)
}

// ParseMade is Parse for a definition made while a machine runs. Whether
// its approvals have an ask to go to is left open: it is settled when the
// definition is started, under whatever ask is above it then.
func ParseMade(src []byte, load func(path string) ([]byte, error)) (*Draft, error) {
	return parse(src, load, false)
}

func parse(src []byte, load func(path string) ([]byte, error), root bool) (*Draft, error) {
	p := &Parser{load: load}
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
	problem := Problem{Place: place, Text: fmt.Sprintf(format, args...)}
	if len(p.chain) > 0 {
		problem.File = p.chain[len(p.chain)-1]
	}
	if !slices.Contains(p.problems, problem) {
		p.problems = append(p.problems, problem)
	}
}

// parse reads one written definition. needsAsk says whether it, or a child
// that takes its ask from it, holds a job for approval with no ask to go to.
func (p *Parser) parse(src []byte, place string) (draft *Draft, needsAsk bool) {
	written, first, err := readYAML(src)
	if err != nil {
		p.Problem(place, "%v", err)
		return nil, false
	}
	return p.fill(written, first, place)
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
	}
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
	// The whole is checked whatever the filling-out found, so that
	// everything wrong is reported together.
	problems, approves := Check(def)
	for _, problem := range problems {
		p.Problem(problem.Place, "%s", problem.Text)
	}
	// A child with no ask of its own takes its parent's. A machine run as
	// an ask takes none from above it.
	for name, child := range p.draft.Children {
		if def.Ask != nil && def.Ask.Machine == name {
			if child.needsAsk {
				p.Problem("ask", "the machine run as the ask marks a job for approval, or lists a permission to be asked about, and has no ask of its own: a machine run as an ask takes no ask from above it")
			}
			continue
		}
		approves = approves || child.needsAsk
	}
	p.draft.needsAsk = approves && def.Ask == nil
	return p.draft, p.draft.needsAsk
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
	s.Script = p.path(f.Text("script"))
	for _, file := range f.Strings("files") {
		s.Files = append(s.Files, p.path(file))
	}
	machine, hasMachine := f.Value("machine")
	s.Input, _ = f.Value("input")
	s.Allow = f.Strings("allow")
	s.Deny = f.Strings("deny")
	s.Timeout = f.text("timeout", KindDuration)
	s.Attempts = f.Int("attempts")
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
		switch m := machine.(type) {
		case string:
			s.Machine = p.path(m)
			if !HasExpr(m) {
				p.machineFile(place+".machine", s.Machine)
			}
		case map[string]any:
			// A machine written in place.
			p.inline++
			s.Machine = fmt.Sprintf("in-place-%d", p.inline)
			first := ""
			for _, name := range Types() {
				if _, ok := m[name]; ok {
					first = name
				}
			}
			p.child(place+".machine", s.Machine, func(sub *Parser) (*Draft, bool) {
				return sub.fill(m, first, place+".machine")
			}, path.Join(p.dir, p.in), "")
		default:
			p.Problem(place+".machine", "a machine is the path of a definition, or a definition written in place")
		}
	}
	return s
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

// Load reads a file by its path from the directory of the file being read.
func (p *Parser) Load(name string) ([]byte, error) {
	if !relative(name) {
		return nil, fmt.Errorf("the path starts with ./ or ../")
	}
	return p.load(path.Join(p.dir, p.in, name))
}

// In fills what is written in another file, named by its path from the
// directory of the file being read: a path written there is read from
// that file's directory.
func (p *Parser) In(file string, fill func()) {
	outer := p.in
	p.in = path.Dir(path.Join(p.in, file))
	defer func() { p.in = outer }()
	fill()
}

// path gives a relative path written in the file being read as a clean
// path from the directory of the definition, starting with ./ or ../.
// Anything else is given as it is.
func (p *Parser) path(name string) string {
	if !relative(name) || HasExpr(name) {
		return name
	}
	joined := path.Join(p.in, name)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return joined
	}
	return "./" + joined
}

func relative(name string) bool {
	return strings.HasPrefix(name, "./") || strings.HasPrefix(name, "../")
}

// file loads a script or a file a script loads.
func (p *Parser) file(place, name string) {
	if _, ok := p.draft.Files[name]; ok {
		return
	}
	if strings.HasPrefix(name, "corbie:") {
		return // one of Corbie's own, added by the type that uses it
	}
	if !relative(name) {
		p.draft.Definition.Scripts[name] = blob.Ref{}
		p.Problem(place, "%q: the path of a script starts with ./ or ../", name)
		return
	}
	// Named once, it is in the definition's table, read or not: what is
	// wrong with it is said here and not again by the check.
	p.draft.Definition.Scripts[name] = blob.Ref{}
	contents, err := p.load(path.Join(p.dir, name))
	if err != nil {
		p.Problem(place, "%q cannot be read: %v", name, err)
		return
	}
	p.draft.Files[name] = contents
}

// machineFile loads a child definition kept in its own file.
func (p *Parser) machineFile(place, name string) {
	if _, ok := p.draft.Children[name]; ok {
		return
	}
	full := path.Join(p.dir, name)
	for _, above := range p.chain {
		if above == full {
			p.Problem(place, "the definition leads back to itself: %s -> %s", strings.Join(p.chain, " -> "), full)
			p.draft.Definition.Machines[name] = blob.Ref{}
			return
		}
	}
	src, err := p.load(full)
	if err != nil {
		p.draft.Definition.Machines[name] = blob.Ref{}
		p.Problem(place, "%q cannot be read: %v", name, err)
		return
	}
	p.child(place, name, func(sub *Parser) (*Draft, bool) { return sub.parse(src, "") }, path.Dir(full), full)
}

func (p *Parser) child(place, name string, read func(*Parser) (*Draft, bool), dir, file string) {
	sub := &Parser{load: p.load, dir: dir, chain: p.chain, problems: p.problems, inline: p.inline * 100}
	if file != "" {
		sub.chain = append(append([]string{}, p.chain...), file)
	}
	child, _ := read(sub)
	p.problems = sub.problems
	p.draft.Definition.Machines[name] = blob.Ref{}
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

func (f *Fields) at(name string) string {
	if f.place == "" {
		return name
	}
	return f.place + "." + name
}

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

// readYAML reads a written definition into plain values: mappings, lists,
// text, numbers, true and false. It gives the top mapping and the name on
// its first line.
func readYAML(src []byte) (map[string]any, string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		text := oneLine(strings.TrimPrefix(err.Error(), "yaml: "))
		if strings.Contains(string(src), "${") {
			text += ` (if a value starts with ${ or holds one beside a colon or a brace, put it in quotes: "${…}")`
		}
		return nil, "", fmt.Errorf("this is not YAML that can be read: %s", text)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, "", fmt.Errorf("the definition is empty")
	}
	top := doc.Content[0]
	budget := 200000
	v, err := fromYAML(top, &budget, map[*yaml.Node]bool{})
	if err != nil {
		return nil, "", err
	}
	m, ok := v.(map[string]any)
	if !ok || len(top.Content) == 0 {
		return nil, "", fmt.Errorf("a definition is a mapping that begins with its type and its name")
	}
	return m, top.Content[0].Value, nil
}

// fromYAML turns a node into a plain value. Aliases may not grow a
// definition far beyond what was written, nor contain themselves.
func fromYAML(n *yaml.Node, budget *int, inside map[*yaml.Node]bool) (any, error) {
	if *budget--; *budget < 0 {
		return nil, fmt.Errorf("the definition's aliases make it far larger than what is written")
	}
	switch n.Kind {
	case yaml.AliasNode:
		if inside[n.Alias] {
			return nil, fmt.Errorf("line %d: an alias contains itself", n.Line)
		}
		inside[n.Alias] = true
		defer delete(inside, n.Alias)
		return fromYAML(n.Alias, budget, inside)
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: the name of a field is plain text", k.Line)
			}
			if _, dup := m[k.Value]; dup {
				return nil, fmt.Errorf("line %d: the field %q is written twice", k.Line, k.Value)
			}
			v, err := fromYAML(n.Content[i+1], budget, inside)
			if err != nil {
				return nil, err
			}
			m[k.Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			v, err := fromYAML(item, budget, inside)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.ScalarNode:
		// A plain value YAML would read as a date stays the text written.
		switch n.ShortTag() {
		case "!!null":
			return nil, nil
		case "!!bool":
			return n.Value == "true" || n.Value == "True" || n.Value == "TRUE", nil
		case "!!int", "!!float":
			var f float64
			if err := n.Decode(&f); err == nil {
				return f, nil
			}
		}
		return n.Value, nil
	}
	return nil, fmt.Errorf("line %d: this part of the definition cannot be read", n.Line)
}
