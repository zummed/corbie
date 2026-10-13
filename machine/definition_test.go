package machine

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"
	"testing"
)

// probe is a machine type for these tests: it has jobs and a value, and
// its step asks for every job once. A job may read the input and an item.
type probe struct{}

type probeBody struct {
	Jobs  map[string]*JobSpec `json:"jobs,omitzero"`
	Value any                 `json:"value,omitzero"`
}

func (probe) Name() string { return "probe" }

func (probe) Shape() *Shape {
	return &Shape{Name: "probe", What: "a probe", Fields: []Field{
		{Name: "work", Kind: KindNamed, Of: JobShape, Does: "its jobs, by name"},
		{Name: "value", Kind: KindValue, Does: "a value of any kind"},
	}}
}

func (probe) Fill(p *Parser, f *Fields) any {
	body := &probeBody{Jobs: map[string]*JobSpec{}}
	jobs, order := f.Map("work")
	for _, name := range order {
		body.Jobs[name] = p.Job("work."+name, jobs[name])
	}
	body.Value, _ = f.Value("value")
	return body
}

func (probe) Check(c *Checker, def *Definition) {
	var body probeBody
	if !c.DecodeBody(def, &body) {
		return
	}
	for _, name := range sortedKeys(body.Jobs) {
		c.Name("work."+name, name)
		c.Job("work."+name, body.Jobs[name], "input", "item")
	}
	c.Value("value", body.Value, "input")
}

func (probe) Step(in *StepIn) (*StepOut, error) {
	var body probeBody
	json.Unmarshal(in.Definition.Body, &body)
	out := &StepOut{State: map[string]any{}}
	eval := NewEvaluator(in.Time, in.Seed)
	for _, name := range sortedKeys(body.Jobs) {
		job, err := body.Jobs[name].Fill(eval, Scope{"input": in.Input}, "work."+name, name)
		if err != nil {
			return nil, err
		}
		out.Requests = append(out.Requests, Request{Job: job})
	}
	return out, nil
}

func (probe) Show(*Definition, jsontext.Value) any { return nil }
func (probe) Manual() string                       { return "probe: a machine type for tests." }

func init() { Register(probe{}) }

func files(contents map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		if text, ok := contents[path]; ok {
			return []byte(text), nil
		}
		return nil, fmt.Errorf("no such file")
	}
}

func problems(t *testing.T, src string, contents map[string]string) string {
	t.Helper()
	_, err := Parse([]byte(src), files(contents))
	if err == nil {
		t.Fatalf("the definition passed:\n%s", src)
	}
	return err.Error()
}

func TestFirstLineGivesTypeAndName(t *testing.T) {
	if _, err := Parse([]byte("probe: p\nvalue: 1\n"), files(nil)); err != nil {
		t.Fatal(err)
	}
	if got := problems(t, "widget: p\n", nil); !strings.Contains(got, `"widget"`) || !strings.Contains(got, "first line") {
		t.Fatalf("an unknown type: %s", got)
	}
	if got := problems(t, "value: 1\nprobe: p\n", nil); !strings.Contains(got, "first line") {
		t.Fatalf("the type not on the first line: %s", got)
	}
}

// Everything wrong is reported together, each with its place.
func TestEverythingWrongIsReportedTogether(t *testing.T) {
	got := problems(t, `probe: p
colour: blue
value: "${input.a +}"
work:
  one: { script: ./missing.js }
  two: { machine: ./child.yaml, memory: 1GB }
`, map[string]string{"child.yaml": "probe: child\n"})
	for _, want := range []string{"colour", "value", "missing.js"} {
		if !strings.Contains(got, want) {
			t.Errorf("the report says nothing of %s:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n") < 2 {
		t.Errorf("three mistakes were reported as:\n%s", got)
	}
}

func Test_mach_unknown_or_repeated_field_is_refused(t *testing.T) {
	if got := problems(t, "probe: p\nvalue: 1\nvalue: 2\n", nil); !strings.Contains(got, "written twice") {
		t.Fatalf("a repeated field: %s", got)
	}
	if got := problems(t, "probe: p\nValue: 1\n", nil); !strings.Contains(got, `"Value" is not a field`) {
		t.Fatalf("a name in the wrong case: %s", got)
	}
	if got := problems(t, "probe: p\nwork:\n  a: { script: ./a.js, retries: 3 }\n", map[string]string{"a.js": ""}); !strings.Contains(got, "retries") {
		t.Fatalf("an unknown field of a job: %s", got)
	}
	// A stored definition is held to the same: the controller does not
	// take a client's word for what it stores.
	for _, stored := range []string{
		`{"type":"probe","name":"p","body":{},"colour":"blue"}`,
		`{"type":"probe","name":"p","name":"q","body":{}}`,
		`{"type":"probe","Name":"p","body":{}}`,
	} {
		if _, err := Decode([]byte(stored)); err == nil {
			t.Errorf("a stored definition was read: %s", stored)
		}
	}
	def, err := Decode([]byte(`{"type":"probe","name":"p","body":{"jobs":{},"extra":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := Check(def); len(got) == 0 {
		t.Error("an unknown field in a stored definition's body passed its check")
	}
}

func Test_mach_problem_is_one_line(t *testing.T) {
	got := problems(t, "probe: p\nvalue: \"a long prompt\\nof several lines\\nthen ${input.a +} more\"\n", nil)
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "value") {
			t.Fatalf("a problem was broken across lines:\n%s", got)
		}
	}
}

func Test_mach_alias_growth_is_refused(t *testing.T) {
	bomb := "probe: p\nvalue:\n  a: &a [x,x,x,x,x,x,x,x,x,x]\n  b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a,*a]\n  c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b,*b]\n  d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c,*c]\n  e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d,*d]\n  f: [*e,*e,*e,*e,*e,*e,*e,*e,*e,*e]\n"
	if got := problems(t, bomb, nil); !strings.Contains(got, "alias") {
		t.Fatalf("aliases that grow a definition: %s", got)
	}
}

func Test_mach_unquoted_expression_gets_quoting_hint(t *testing.T) {
	if got := problems(t, "probe: p\nwork:\n  a: { script: ./a.js, input: { n: ${input.n} } }\n", nil); !strings.Contains(got, "put it in quotes") {
		t.Fatalf("an unquoted expression: %s", got)
	}
}

func Test_mach_date_like_value_stays_text(t *testing.T) {
	draft, err := Parse([]byte("probe: p\nvalue: { day: 2026-10-01, time: 12:30:00, n: 3, yes: true, none: null, text: hello }\n"), files(nil))
	if err != nil {
		t.Fatal(err)
	}
	stored := string(draft.Definition.Body)
	for _, want := range []string{`"day":"2026-10-01"`, `"time":"12:30:00"`, `"n":3`, `"yes":true`, `"text":"hello"`} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored form %s has no %s", stored, want)
		}
	}
}

func Test_mach_stored_expression_reads_back_as_written(t *testing.T) {
	draft, err := Parse([]byte("probe: p\nvalue: \"${input.a && input.b < 3 ? '<ok>' : 'no'}\"\n"), files(nil))
	if err != nil {
		t.Fatal(err)
	}
	if stored := string(draft.Definition.Body); !strings.Contains(stored, "input.a && input.b < 3 ? '<ok>'") {
		t.Fatalf("the stored form changed the expression: %s", stored)
	}
}

func Test_mach_check_compiles_every_expression(t *testing.T) {
	for src, want := range map[string]string{
		"probe: p\nvalue: \"${input.a +}\"\n":              "value",
		"probe: p\nvalue: { deep: [1, \"${let = 3}\"] }\n": "value.deep[1]",
		"probe: p\nvalue: \"${require('fs')}\"\n":          `reads "require"`,
		"probe: p\nvalue: \"${stages.a}\"\n":               `reads "stages"`,
		"probe: p\nvalue: \"text ${input.a\"\n":            "never closed",
	} {
		if got := problems(t, src, nil); !strings.Contains(got, want) {
			t.Errorf("%q: %s", src, got)
		}
	}
}

func Test_mach_definition_including_itself_is_refused(t *testing.T) {
	contents := map[string]string{
		"a.yaml": "probe: a\nwork:\n  kid: { machine: ./b.yaml }\n",
		"b.yaml": "probe: b\nwork:\n  kid: { machine: ./a.yaml }\n",
	}
	got := problems(t, "probe: root\nwork:\n  kid: { machine: ./a.yaml }\n", contents)
	if !strings.Contains(got, "leads back to itself") || !strings.Contains(got, "a.yaml") || !strings.Contains(got, "b.yaml") {
		t.Fatalf("a definition that includes itself: %s", got)
	}
}

func TestNamesAreNotValues(t *testing.T) {
	js := map[string]string{"a.js": ""}
	for src, want := range map[string]string{
		"probe: \"${input.x}\"\n":                                                      "not a name",
		"probe: p\nwork:\n  \"${input.x}\": ./a.js\n":                                  "a name cannot be an expression",
		"probe: p\nwork:\n  a: { script: ./a.js, tags: [\"${input.x}\"] }\n":           "a tag is written out",
		"probe: p\nwork:\n  a: { script: ./a.js, deny: [\"net:${input.x}\"] }\n":       "written out in full",
		"probe: p\nwork:\n  a: { script: ./a.js, allow: [\"${input.x}:y\"] }\n":        "is not a permission",
		"probe: p\nwork:\n  a: { script: ./a.js, allow: [\"secret:${input.x}@*\"] }\n": "a secret's name is written out",
		"probe: p\nwork:\n  a: { script: \"./${input.x}.js\" }\n":                      "exactly one expression",
		"probe: p\nwork:\n  ask: ./a.js\n":                                             "taken by the definition's ask",
	} {
		if got := problems(t, src, js); !strings.Contains(got, want) {
			t.Errorf("%q: %s", src, got)
		}
	}
	// A machine or a script may be exactly one expression.
	if _, err := Parse([]byte("probe: p\nwork:\n  a: { machine: \"${input.definition}\" }\n  b: { script: \"${input.script}\" }\n"), files(nil)); err != nil {
		t.Fatal(err)
	}
}

// A permission's scope is filled from the machine's input. Anything else a
// job may read, it may read in its input and not in its permissions.
func TestPermissionScopeReadsOnlyTheInput(t *testing.T) {
	js := map[string]string{"a.js": ""}
	const def = "probe: p\nwork:\n  a: { script: ./a.js, input: { n: \"${item}\" }, allow: [\"fs.read:${WHAT}/**\"] }\n"
	if _, err := Parse([]byte(strings.Replace(def, "WHAT", "input.dir", 1)), files(js)); err != nil {
		t.Fatalf("a scope filled from the input: %v", err)
	}
	got := problems(t, strings.Replace(def, "WHAT", "item", 1), js)
	for _, want := range []string{"work.a.allow[0]", `"fs.read:${item}/**"`, "from the machine's input and from nothing else", `reads "item"`} {
		if !strings.Contains(got, want) {
			t.Errorf("a scope filled from an item was reported without %q:\n%s", want, got)
		}
	}
	// The same holds where the scope is filled: it is given the input alone.
	spec := &JobSpec{Script: "./a.js", Allow: []string{"fs.read:${item}/**"}}
	if job, err := spec.Fill(eval(t), Scope{"input": map[string]any{}, "item": "/etc"}, "work.a", "a"); err == nil {
		t.Fatalf("a scope was filled from an item: %v", job.Allow)
	}
	spec.Allow = []string{"fs.read:${input.dir}/**"}
	job, err := spec.Fill(eval(t), Scope{"input": map[string]any{"dir": "/srv"}, "item": "/etc"}, "work.a", "a")
	if err != nil || len(job.Allow) != 1 || job.Allow[0] != "fs.read:/srv/**" {
		t.Fatalf("a scope filled from the input is %v (%v)", job, err)
	}
}

// A definition made while a machine runs may hold a job for approval with
// no ask of its own: whether one applies is settled where it is started.
// Everything else wrong with it is still found.
func TestParseMadeLeavesTheAskOpen(t *testing.T) {
	js := map[string]string{"a.js": "", "held.yaml": "probe: held\nwork:\n  a: { script: ./a.js, approve: true }\n"}
	for _, src := range []string{
		"probe: p\nwork:\n  a: { script: ./a.js, approve: true }\n",
		"probe: p\nask_about: [exec]\nwork:\n  a: { script: ./a.js }\n",
		"probe: p\nwork:\n  kid: { machine: ./held.yaml }\n",
	} {
		if got := problems(t, src, js); !strings.Contains(got, "give the definition an ask") {
			t.Errorf("%q, written and sent by a client: %s", src, got)
		}
		if _, err := ParseMade([]byte(src), files(js)); err != nil {
			t.Errorf("%q, made while a machine runs: %v", src, err)
		}
	}
	for src, want := range map[string]string{
		"probe: p\nwork:\n  a: { script: ./missing.js, approve: true }\n": "missing.js",
		"probe: p\nask: { machine: ./held.yaml }\n":                       "takes no ask from above it",
	} {
		_, err := ParseMade([]byte(src), files(js))
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "give the definition an ask") {
			t.Errorf("%q, made while a machine runs: %v", src, err)
		}
	}
}

func TestJobsAreWrittenTheSameForBoth(t *testing.T) {
	contents := map[string]string{"a.js": "", "lib.js": "", "child.yaml": "probe: child\n"}
	draft, err := Parse([]byte(`probe: p
allow: ["net:${input.host}"]
work:
  job: { script: ./a.js, files: [./lib.js], input: { n: "${input.n}" }, allow: [exec:git], timeout: 90s, attempts: 3, memory: 256MB, tags: [gpu] }
  kid: { machine: ./child.yaml, input: { n: 1 }, timeout: 1h, attempts: 2 }
  short: ./a.js
  inline:
    machine:
      probe: within
      value: 3
`), files(contents))
	if err != nil {
		t.Fatal(err)
	}
	var body probeBody
	json.Unmarshal(draft.Definition.Body, &body)
	job := body.Jobs["job"]
	if job.Script != "./a.js" || job.Memory != 256<<20 || job.Timeout.String() != "1m30s" || job.Attempts != 3 || len(job.Files) != 1 {
		t.Fatalf("the job is %+v", job)
	}
	if body.Jobs["short"].Script != "./a.js" || !body.Jobs["kid"].IsMachine() {
		t.Fatal("the short form of a job, or a machine job, was not filled out")
	}
	if len(draft.Files) != 2 || len(draft.Children) != 2 {
		t.Fatalf("the draft holds %d files and %d child definitions", len(draft.Files), len(draft.Children))
	}
	if got := problems(t, "probe: p\nwork:\n  kid: { machine: ./child.yaml, memory: 1GB }\n", contents); !strings.Contains(got, "belong to a script") {
		t.Fatalf("a machine job with memory: %s", got)
	}
	if got := problems(t, "probe: p\nwork:\n  a: { script: /etc/passwd }\n", contents); !strings.Contains(got, "starts with ./") {
		t.Fatalf("a script path outside the set: %s", got)
	}
}

func Test_mach_step_taken_twice_makes_the_same_names(t *testing.T) {
	draft, err := Parse([]byte("probe: p\nwork:\n  a: { script: ./a.js, input: { r: \"${Math.random()}\", at: \"${Date.now()}\" } }\n"), files(map[string]string{"a.js": ""}))
	if err != nil {
		t.Fatal(err)
	}
	take := func(seed string) string {
		out, err := probe{}.Step(&StepIn{Definition: draft.Definition, Time: stepTime, Seed: []byte(seed), Input: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(out.Requests, json.Deterministic(true))
		return string(data)
	}
	if take("s") != take("s") {
		t.Fatal("the same step taken twice asked for different things")
	}
	if take("s") == take("other") {
		t.Fatal("another step got the same random numbers")
	}
}

func TestDurations(t *testing.T) {
	for text, want := range map[string]string{"30s": "30s", "5m": "5m0s", "48h": "48h0m0s", "7d": "168h0m0s", "1.5d": "36h0m0s"} {
		if d, err := ParseDuration(text); err != nil || d.String() != want {
			t.Errorf("%s is %v (%v), want %s", text, d, err, want)
		}
	}
	for _, text := range []string{"", "soon", "5", "-1h", "1w"} {
		if _, err := ParseDuration(text); err == nil {
			t.Errorf("%q passed as a length of time", text)
		}
	}
}
