package machine

import "slices"

// A Shape is what one mapping of a definition may hold: its fields, in the
// order a person writes them. It is said once. The reader of definitions
// reads a mapping by its shape, and can read no field the shape does not
// name.
type Shape struct {
	// Name is what the mapping is called where a field holds one: model,
	// tool, job. What is the same as it stands in a sentence: "a model".
	Name   string
	What   string
	Fields []Field
	// Sorts is there where the mapping is one of several sorts of thing,
	// which hold different fields: a model of one service or of another.
	Sorts *Sorts
}

// Sorts says a mapping is one of several sorts, told apart by one of its
// fields, and which of the mapping's fields each sort holds. A field a sort
// does not hold is one that is not written for it.
type Sorts struct {
	// By is the field that says which sort a mapping is, and Fields gives,
	// for each value of it, the names of the fields that sort holds.
	By     string
	Fields map[string][]string
	// Which says which sort a mapping is where the field that says so is
	// not written, from what is: a model's service follows from its name.
	Which func(written map[string]any) string
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
	Name string
	Kind Kind
	// Description says what the field is for, in a line.
	Description string
	// Of is the shape of the mappings the field holds: of the one mapping
	// of a field that is fields, of each named thing of one that is named,
	// and, of a value, of a mapping written there where a shorter form
	// could be. A job holds the shape of a job, and needs none said.
	Of *Shape
	// Path says what the field holds is a path, or a list of them, read
	// from the file it is written in; a mapping there is a machine.
	Path bool
	// Many says a value of the field may be a list of what it holds, each
	// read by its shape: a list there is no data.
	Many bool
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
	// KindList is a list of texts. Where its reader takes one, a mapping
	// of names, each with such a list, gives a list for each name.
	KindList Kind = "list"
	// KindFields is a mapping with a shape of its own, and KindNamed a
	// mapping of names, each of which holds a thing of one shape.
	KindFields Kind = "fields"
	KindNamed  Kind = "named"
	// KindTypes is a shape: a JSON Schema of an object, or the short form,
	// a mapping of names, each with the type of its value.
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

// entry gives a field that stands for one named thing of a named field.
func (f *Field) entry() *Field {
	if f.Of == JobShape {
		return &Field{Kind: KindJob}
	}
	return &Field{Kind: KindFields, Of: f.Of}
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

// hookShape is the shape of a hook.
var hookShape = &Shape{Name: "hook", What: "a hook", Fields: []Field{
	{Name: "before", Kind: KindJob, Description: "a job that runs before a job starts: { ok: true } lets the job go on, with the input it gives if it gives one, and anything else denies the job"},
	{Name: "after", Kind: KindJob, Description: "a job that runs after a job has ended done: { ok: true } gives the machine the job's output, or the output it gives, and anything else fails the job"},
	{Name: "when", Kind: KindWord, Description: "a condition, one expression, on the job: the hook runs for the jobs it holds for; every job, if not given"},
}}

// The fields every type of machine takes, beside its type and its name.
var shared = []Field{
	{Name: "description", Kind: KindText, Description: "what the machine is for, in plain text that Corbie keeps with the definition and fills nothing into"},
	{Name: "input", Kind: KindTypes, Description: "the shape of its input: each name with the type of its value, or a JSON Schema with type: object"},
	{Name: "keep", Kind: KindDuration, Description: "how long its instances are kept after they end (a week)"},
	{Name: "allow", Kind: KindList, Description: "permissions for every script the definition runs"},
	{Name: "deny", Kind: KindList, Description: "what no script the definition runs may do"},
	{Name: "approve", Kind: KindList, Description: "permissions that mark a job for approval: a job that declares one starts only after a hook before it"},
	{Name: "hooks", Kind: KindNamed, Of: hookShape, Description: "jobs that run before or after the jobs of the machine and of the machines below it, by name"},
	{Name: "title", Kind: KindText, Description: "what a run is called, filled in from its input when it starts; only the client of its tree sees it"},
	{Name: "meta", Kind: KindValue, Description: "data of any kind, which Corbie keeps with the definition and never reads"},
	{Name: "lifetime", Kind: KindWord,
		Description: "how runs start at its location: singleton, one started by the submit; keyed, one for each key a send names; transient, the default, a new one for each call, start or send"},
	{Name: "update", Kind: KindWord,
		Description: "what a changed file does to the runs already running at its location: restart cancels them; drain, the default but for a singleton, lets them finish"},
	{Name: "receive", Kind: KindList,
		Description: "who may send to its runs or call it: locations and client:<name>, for every event or by event name; with none, its own tree and its tree's client"},
	{Name: "read", Kind: KindList,
		Description: "the clients, beside its tree's, that read its runs' records, as client:<name>"},
}

// The limits every type of machine takes. A type may add its own.
var limits = []Field{
	{Name: "timeout", Kind: KindDuration, Description: "how long an instance may run (for ever, if not given)"},
	{Name: "depth", Kind: KindNumber, Description: "how many levels of machines may be below a root (0: none)"},
	{Name: "jobs", Kind: KindNumber, Description: "how many jobs an instance may start"},
	{Name: "runs", Kind: KindNumber, Description: "of a keyed file: how many of its runs may be running at once (no limit, if not given)"},
}

// AddressShape is the shape of an address: the run a send goes to.
var AddressShape = &Shape{Name: "address", What: "an address", Fields: []Field{
	{Name: "at", Kind: KindWord, Description: "the location: read from the tree's prefix, or a full path from /; alone, the run of a singleton, or a new run of a transient location"},
	{Name: "key", Kind: KindWord, Description: "the key, at a keyed location: its run, started if none is running"},
	{Name: "run", Kind: KindWord, Description: "one run at the location, by its ID"},
}}

// CallShape is the shape of a call: the location whose run a job starts.
var CallShape = &Shape{Name: "call", What: "a call", Fields: []Field{
	{Name: "at", Kind: KindWord, Description: "the location, which is transient: read from the tree's prefix, or a full path from /"},
}}

// SendShape is the shape of the event a job sends.
var SendShape = &Shape{Name: "sent", What: "an event a job sends", Fields: []Field{
	{Name: "to", Kind: KindValue, Of: AddressShape, Description: "the run it goes to: an address, or one expression that gives one, such as ${event.from}"},
	{Name: "event", Kind: KindWord, Description: "the event's name"},
	{Name: "data", Kind: KindValue, Description: "what the event carries"},
}}

// restartShape is the shape of a job's restart policy.
var restartShape = &Shape{Name: "restart", What: "a job's restart policy", Fields: []Field{
	{Name: "after", Kind: KindWord, Description: "failure, after an error or a timeout, which is the default, or always, after the job ends well too"},
	{Name: "limit", Kind: KindValue, Description: "how many restarts: a number, or a number in a length of time; no limit if not given"},
	{Name: "delay", Kind: KindWord, Description: "the wait before a restart: two lengths of time double from the first to the second, and one is the same each time; 1s to 5m if not given"},
}}

// JobShape is the shape of a job, wherever one is written.
var JobShape = &Shape{Name: "job", What: "a job", Fields: []Field{
	{Name: "script", Kind: KindWord, Path: true, Description: "a script, which a runner executes: a file beside the definition"},
	{Name: "files", Kind: KindList, Path: true, Description: "files the script loads with require"},
	{Name: "machine", Kind: KindValue, Path: true, Description: "a machine, which the controller starts as a child: a definition beside this one, or written in place"},
	{Name: "send", Kind: KindFields, Of: SendShape, Description: "an event sent to a run by its address; the job ends done once it is sent"},
	{Name: "call", Kind: KindFields, Of: CallShape, Description: "a location whose run is started with the input; the job waits, and its output is that run's result"},
	{Name: "input", Kind: KindValue, Description: "what the script, the machine or the called run is given"},
	{Name: "allow", Kind: KindList, Description: "what the job may do, with the definition's allow; for a machine, it narrows everything the child does"},
	{Name: "deny", Kind: KindList, Description: "what the job may not do"},
	{Name: "timeout", Kind: KindDuration, Description: "how long it may take, five minutes by default; for a machine, the child's timeout"},
	{Name: "restart", Kind: KindFields, Description: "what happens when the job ends", Of: restartShape},
	{Name: "memory", Kind: KindSize, Description: "the most memory the job may use"},
	{Name: "tags", Kind: KindList, Description: "chooses runners: one that has every tag"},
	{Name: "approve", Kind: KindFlag, Description: "marks the job for approval: it starts only after a hook before it"},
	{Name: "meta", Kind: KindValue, Description: "data of any kind, which hooks read as job.meta and nothing else reads"},
}}

// definitionShape is the shape of a definition before its type's own
// fields: the field that gives its type and its name, and what every type
// takes.
func definitionShape(t Type) *Shape {
	first := Field{Name: t.Name(), Kind: KindWord, Description: "its name: the field named for the type gives the type and the name"}
	own := slices.Clone(limits)
	if l, ok := t.(Limiter); ok {
		own = append(own, l.Limits()...)
	}
	limit := Field{Name: "limits", Kind: KindFields, Description: "how much an instance may use",
		Of: &Shape{Name: "limits", What: "a definition's limits", Fields: own}}
	fields := append([]Field{first}, shared[:3]...)
	fields = append(fields, limit)
	return &Shape{Name: "definition", What: "a definition", Fields: append(fields, shared[3:]...)}
}

// A TypeShape is everything a definition of one type may hold: each
// mapping's shape, by its name. The definition itself is two of them
// together, as it is read: "definition" is its type and its name and what
// every type takes, and the shape named as the type is the type's own.
type TypeShape struct {
	Type   string
	Shapes map[string]*Shape
}

// Describe gives what a definition of a type may hold, as its reader reads it.
func Describe(name string) (*TypeShape, bool) {
	t, ok := TypeOf(name)
	if !ok {
		return nil, false
	}
	d := &TypeShape{Type: name, Shapes: map[string]*Shape{}}
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
