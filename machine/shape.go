package machine

import (
	"encoding/json/v2"
	"slices"
)

// A Shape is what one mapping of a definition may hold: its fields, in the
// order a person writes them. It is said once. The reader of definitions
// reads a mapping by its shape, and can read no field the shape does not
// name; and a client is given the same shape, to offer the fields to a
// person. So what is read and what is offered cannot come apart.
type Shape struct {
	// Name is what the mapping is called where a field holds one: model,
	// tool, job. What is the same as it stands in a sentence: "a model".
	Name   string  `json:"name"`
	What   string  `json:"what"`
	Fields []Field `json:"fields"`
	// Sorts is there where the mapping is one of several sorts of thing,
	// which hold different fields: a model of one service or of another.
	Sorts *Sorts `json:"sorts,omitzero"`
}

// Sorts says a mapping is one of several sorts, told apart by one of its
// fields, and which of the mapping's fields each sort holds. A field a sort
// does not hold is one that is not written for it, and is not offered.
type Sorts struct {
	// By is the field that says which sort a mapping is, and Fields gives,
	// for each value of it, the names of the fields that sort holds.
	By     string              `json:"by"`
	Fields map[string][]string `json:"fields"`
	// Which says which sort a mapping is where the field that says so is
	// not written, from what is: a model's service follows from its name.
	Which func(written map[string]any) string `json:"-"`
}

// Sort says which sort of thing a written mapping of the shape is: the
// value of the field that says so, or what follows from the rest. It is
// nothing for a shape that has no sorts, and for a mapping that says
// nothing of its sort.
func (s *Shape) Sort(written map[string]any) string {
	if s == nil || s.Sorts == nil {
		return ""
	}
	if sort, ok := written[s.Sorts.By].(string); ok && sort != "" {
		return sort
	}
	if s.Sorts.Which == nil {
		return ""
	}
	return s.Sorts.Which(written)
}

// A Field is one field of a mapping: its name, the kind of value it takes,
// and what it is for.
type Field struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	// Does says what the field is for, in a line.
	Does string `json:"does"`
	// Like is the field as it is written, by example: what stands after
	// its name.
	Like string `json:"like,omitzero"`
	// Of is the shape of the mappings the field holds: of the one mapping
	// of a field that is fields, of each named thing of one that is named,
	// and, of a value, of a mapping written there where a shorter form
	// could be. A job holds the shape of a job, and needs none said.
	Of *Shape `json:"-"`
	// Values are the values it takes, where they are known. With Open any
	// other may be written too: these are then the known ones, and no
	// more than that.
	Values []string `json:"values,omitzero"`
	Open   bool     `json:"open,omitzero"`
	// By names a field beside this one that its values depend on, and
	// ValuesBy gives them for each value of that field, with those for
	// where that field is not written under "".
	By       string              `json:"by,omitzero"`
	ValuesBy map[string][]string `json:"values_by,omitzero"`
}

// MarshalJSON gives a field as a client is given it, with the shape it
// holds named and not written out: a shape may hold itself.
func (f Field) MarshalJSON() ([]byte, error) {
	type plain Field
	of := ""
	if s := f.Holds(); s != nil {
		of = s.Name
	}
	return json.Marshal(struct {
		plain
		Of string `json:"of,omitzero"`
	}{plain(f), of})
}

// Kind is the kind of value a field takes.
type Kind string

const (
	// KindWord is a name or a short value, written out on one line.
	KindWord Kind = "word"
	// KindText is text of any length.
	KindText Kind = "text"
	// KindNumber is a whole number, and KindFlag true or false.
	KindNumber Kind = "number"
	KindFlag   Kind = "flag"
	// KindDuration is a length of time, written like 30s, and KindSize a
	// size in bytes, written like 256MB.
	KindDuration Kind = "duration"
	KindSize     Kind = "size"
	// KindList is a list of texts; one text alone is a list of one.
	KindList Kind = "list"
	// KindFields is a mapping with a shape of its own, and KindNamed a
	// mapping of names, each of which holds a thing of one shape.
	KindFields Kind = "fields"
	KindNamed  Kind = "named"
	// KindTypes is a mapping of names, each with the type of its value.
	KindTypes Kind = "types"
	// KindJob is a job: the path of a script, or a mapping written as a
	// job is.
	KindJob Kind = "job"
	// KindValue is anything: text, a number, a list, a mapping. Its reader
	// says what it takes.
	KindValue Kind = "value"
)

// Holds gives the shape of the mappings a field holds: the one it names,
// and for a job the shape of a job.
func (f *Field) Holds() *Shape {
	if f.Kind == KindJob {
		return JobShape
	}
	return f.Of
}

// field gives the field of a name, or nil.
func (s *Shape) field(name string) *Field {
	if s == nil {
		return nil
	}
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

// The fields every type of machine takes, after the first line.
var shared = []Field{
	{Name: "input", Kind: KindList, Like: "[urls, depth]", Does: "the inputs it needs; a start without one fails"},
	{Name: "keep", Kind: KindDuration, Like: "7d", Does: "how long its instances are kept after they end (a week)"},
	{Name: "allow", Kind: KindList, Like: "[net:example.com]", Does: "permissions for every script the definition runs"},
	{Name: "deny", Kind: KindList, Like: "[exec]", Does: "what no script the definition runs may do"},
	{Name: "ask_about", Kind: KindList, Like: "[exec]", Does: "permissions that must be asked about before a job starts"},
	{Name: "ask", Kind: KindJob, Like: "{ machine: ./ask.yaml }", Does: "the job that approves when a job needs approval"},
}

// The limits every type of machine takes. A type may add its own.
var limits = []Field{
	{Name: "timeout", Kind: KindDuration, Like: "1h", Does: "how long an instance may run (for ever, if not given)"},
	{Name: "depth", Kind: KindNumber, Like: "8", Does: "how many levels of machines may be below a root (0: none)"},
	{Name: "jobs", Kind: KindNumber, Like: "100", Does: "how many jobs an instance may start"},
}

// JobShape is the shape of a job, wherever one is written.
var JobShape = &Shape{Name: "job", What: "a job", Fields: []Field{
	{Name: "script", Kind: KindWord, Like: "./fetch.js", Does: "a script, which a runner executes: a file beside the definition"},
	{Name: "files", Kind: KindList, Like: "[./lib.js]", Does: "files the script loads with require"},
	{Name: "machine", Kind: KindValue, Like: "./child.yaml", Does: "a machine, which the controller starts as a child: a definition beside this one, or written in place"},
	{Name: "input", Kind: KindValue, Like: `{ url: "${item}" }`, Does: "what the script or the machine is given"},
	{Name: "allow", Kind: KindList, Like: "[net:example.com]", Does: "what the job may do, with the definition's allow; for a machine, it narrows everything the child does"},
	{Name: "deny", Kind: KindList, Like: "[exec]", Does: "what the job may not do"},
	{Name: "timeout", Kind: KindDuration, Like: "5m", Does: "how long it may take, five minutes by default; for a machine, the child's timeout"},
	{Name: "attempts", Kind: KindNumber, Like: "3", Does: "how many times it is tried on an error or a timeout"},
	{Name: "memory", Kind: KindSize, Like: "256MB", Does: "the most memory the job may use"},
	{Name: "tags", Kind: KindList, Like: "[gpu]", Does: "chooses runners: one that has every tag"},
	{Name: "approve", Kind: KindFlag, Like: "true", Does: "wait for the definition's ask to approve it"},
}}

// definitionShape is the shape of a definition before its type's own
// fields: its first line, and what every type takes.
func definitionShape(t Type) *Shape {
	first := Field{Name: t.Name(), Kind: KindWord, Like: "digest", Does: "its name: the first line gives the type and the name"}
	own := slices.Clone(limits)
	if l, ok := t.(Limiter); ok {
		own = append(own, l.Limits()...)
	}
	limit := Field{Name: "limits", Kind: KindFields, Like: "{ timeout: 1h, jobs: 100 }", Does: "how much an instance may use",
		Of: &Shape{Name: "limits", What: "a definition's limits", Fields: own}}
	fields := append([]Field{first}, shared[:2]...)
	fields = append(fields, limit)
	return &Shape{Name: "definition", What: "a definition", Fields: append(fields, shared[2:]...)}
}

// A Description is everything a definition of one type may hold: each
// mapping's shape, by its name. The definition itself is two of them
// together, as it is read: "definition" is its first line and what every
// type takes, and the shape named as the type is the type's own.
type Description struct {
	Type   string            `json:"type"`
	Shapes map[string]*Shape `json:"shapes"`
}

// Describe gives what a definition of a type may hold, as its reader reads it.
func Describe(name string) (*Description, bool) {
	t, ok := TypeOf(name)
	if !ok {
		return nil, false
	}
	d := &Description{Type: name, Shapes: map[string]*Shape{}}
	var add func(s *Shape)
	add = func(s *Shape) {
		if d.Shapes[s.Name] != nil {
			return
		}
		d.Shapes[s.Name] = s
		for _, f := range s.Fields {
			if inner := f.Holds(); inner != nil {
				add(inner)
			}
		}
	}
	add(definitionShape(t))
	add(t.Shape())
	return d, true
}

// reads says whether a field of a kind may be read as one of some kinds. A
// value is whatever its reader takes it for, so it may be read as any.
func (k Kind) reads(as []Kind) bool {
	return len(as) == 0 || k == KindValue || slices.Contains(as, k)
}

// Written reads a definition's text into the plain values its reader
// reads it as: mappings, lists, text, numbers, true and false. It is for a
// client that shows a person what a definition has written, field by
// field, as it will be read.
func Written(src []byte) (map[string]any, error) {
	written, _, err := readYAML(src)
	return written, err
}
