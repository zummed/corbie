package machine

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"testing"
)

// astray is a machine type whose reader and whose shape do not agree, in
// each of the three ways they can fail to.
type astray struct{ probe }

func (astray) Name() string { return "astray" }

func (astray) Shape() *Shape {
	return &Shape{Name: "astray", What: "a machine gone astray", Fields: []Field{
		{Name: "count", Kind: KindNumber, Does: "a number, which the reader reads as text"},
		{Name: "unread", Kind: KindWord, Does: "a field the reader never asks for"},
	}}
}

func (astray) Fill(p *Parser, f *Fields) any {
	f.Text("count")
	f.Text("unnamed")
	f.Of("count")
	return &probeBody{}
}

func (astray) Check(*Checker, *Definition)          {}
func (astray) Show(*Definition, jsontext.Value) any { return nil }

func init() { Register(astray{}) }

// A definition is read by its type's shape and by nothing else: a reader
// that reads a field the shape does not name, reads one as another kind
// than the shape gives it, or leaves one the shape names unread, is a
// fault of Corbie's that the first definition read brings out.
func Test_mach_reader_reads_exactly_what_its_shape_names(t *testing.T) {
	got := problems(t, "astray: a\n", nil)
	for _, want := range []string{
		`unnamed: Corbie reads a field "unnamed" here that its own account of a machine gone astray does not name`,
		`count: Corbie reads the field "count" here as another kind of value`,
		`names a field "unread" that it does not read`,
		`count: Corbie reads mappings in the field "count" here, and its own account of a machine gone astray gives them no shape`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the fault is not said: %q in\n%s", want, got)
		}
	}
	// One whose reader and shape agree is read, and says nothing of either.
	if _, err := Parse([]byte("probe: p\nvalue: 1\nwork:\n  a: { script: ./a.js, timeout: 5s }\n"), files(map[string]string{"a.js": ""})); err != nil {
		t.Fatal(err)
	}
}

// What a definition of a type may hold is given whole: its first line and
// what every type takes, the type's own, and each mapping's shape once, by
// its name, so that a shape that holds itself is still finite.
func Test_mach_a_type_is_described_by_what_its_reader_reads(t *testing.T) {
	d, ok := Describe("probe")
	if !ok {
		t.Fatal("the probe is not described")
	}
	var names []string
	for _, shape := range []string{"definition", "probe"} {
		for _, f := range d.Shapes[shape].Fields {
			names = append(names, f.Name)
		}
	}
	if got := strings.Join(names, " "); got != "probe input keep limits allow deny ask_about ask work value" {
		t.Errorf("a probe's definition holds: %s", got)
	}
	if d.Shapes["job"] != JobShape || d.Shapes["limits"] == nil || len(d.Shapes) != 4 {
		t.Errorf("its shapes are %v", d.Shapes)
	}
	sent, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`{"name":"work","kind":"named","does":"its jobs, by name","of":"job"}`, `{"name":"ask","kind":"job",`, `"like":"256MB"`} {
		if !strings.Contains(string(sent), want) {
			t.Errorf("a client is not given %s in\n%s", want, sent)
		}
	}
	if _, ok := Describe("nothing"); ok {
		t.Error("a type that is not there was described")
	}
}
