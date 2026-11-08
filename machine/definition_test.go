package machine

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/store"
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
		{Name: "work", Kind: KindNamed, Of: JobShape, Description: "its jobs, by name"},
		{Name: "value", Kind: KindValue, Description: "a value of any kind"},
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
	eval := NewEvaluator(in.Time, in.Seed, in.Run)
	for _, name := range sortedKeys(body.Jobs) {
		job, err := body.Jobs[name].Fill(eval, Scope{"input": in.Input}, "work."+name, name)
		if err != nil {
			return nil, err
		}
		out.Requests = append(out.Requests, Request{Job: job})
	}
	return out, nil
}

func (probe) Show(jsontext.Value) Shown { return Shown{} }
func (probe) Manual() string            { return "probe: a machine type for tests." }

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

// A definition gives exactly one type, as a field written anywhere in it or
// in a file it includes, and the type's field gives the name.
func TestTypeIsOneFieldAnywhere(t *testing.T) {
	inc := map[string]string{
		"m.yaml":       "probe: post\nvalue: 1\n",
		"deep.yaml":    "include: ./m.yaml\n",
		"kit.yaml":     "parts:\n  p: { probe: inner }\n",
		"other.yaml":   "listener: b\n",
		"self.yaml":    "include: ./self.yaml\n",
		"nothing.yaml": "value: 1\n",
	}
	for src, name := range map[string]string{
		"probe: p\nvalue: 1\n":             "p",
		"value: 1\nprobe: p\n":             "p",
		"include: ./m.yaml\n":              "post",
		"include: ./deep.yaml\n":           "post",
		"include: ./kit.yaml#parts.p\n":    "inner",
		"probe: mine\ninclude: ./m.yaml\n": "mine",
		"include: ./m.yaml\nprobe: mine\n": "mine",
	} {
		d, err := Parse([]byte(src), files(inc))
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if d.Definition.Type != "probe" || d.Definition.Name != name {
			t.Fatalf("%q is a %s named %q", src, d.Definition.Type, d.Definition.Name)
		}
	}
	for src, want := range map[string]string{
		"widget: p\n":                                       "it gives no type: a machine writes one of ",
		"include: ./nothing.yaml\n":                         "it gives no type",
		"probe: a\nlistener: b\n":                           "it gives more than one type, probe: a at line 1, listener: b at line 2: a machine has one",
		"probe: a\ninclude: ./other.yaml\n":                 "it gives more than one type, probe: a at line 1, listener: b at other.yaml:1: a machine has one",
		"include: ./gone.yaml\n":                            `include: "./gone.yaml" cannot be read: no such file`,
		"include: ./self.yaml\n":                            "leads back to itself",
		"probe: p\nwork:\n  a: { machine: { value: 1 } }\n": "work.a.machine: it gives no type",
		"probe: p\nwork:\n  a: { machine: { include: ./m.yaml, listener: b } }\n": "work.a.machine: it gives more than one type, listener: b at line 3, probe: post at m.yaml:1",
	} {
		got := problems(t, src, inc)
		if !strings.Contains(got, want) {
			t.Fatalf("%q: %s", src, got)
		}
		if strings.Contains(want, "cannot be read") || strings.Contains(want, "leads back") {
			if strings.Contains(got, "no type") {
				t.Fatalf("%q: an include that cannot be read is said alone: %s", src, got)
			}
		}
	}
}

// Everything wrong is reported together, each with its place, and a
// mistake in a child's file with that file.
func TestEverythingWrongIsReportedTogether(t *testing.T) {
	got := problems(t, `probe: p
colour: blue
value: "${input.a +}"
work:
  one: { script: ./missing.js }
  two: { machine: ./child.yaml, memory: 1GB }
  three: { machine: ./kids/kid.yaml }
`, map[string]string{"child.yaml": "probe: child\n", "kids/kid.yaml": "probe: kid\nvalue: \"${input.a +}\"\n"})
	for _, want := range []string{"colour", "value", "missing.js"} {
		if !strings.Contains(got, want) {
			t.Errorf("the report says nothing of %s:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n") < 3 {
		t.Errorf("four mistakes were reported as:\n%s", got)
	}
	if !strings.Contains(got, "\nkids/kid.yaml: value: in ${input.a +}") {
		t.Errorf("the child's mistake was reported as:\n%s", got)
	}

	_, err := Parse([]byte("probe: p\nwork:\n  kid: { machine: ./kids/kid.yaml }\n"), files(map[string]string{"kids/kid.yaml": "probe: kid\nvalue: \"${input.a +}\"\n"}))
	var found Problems
	if !errors.As(err, &found) || len(found) != 1 {
		t.Fatalf("the child's mistake was reported as: %v", err)
	}
	if found[0].File != "kids/kid.yaml" || found[0].Place != "value" {
		t.Errorf("the child's mistake is in file %q at %q", found[0].File, found[0].Place)
	}
	written, err := json.Marshal(found[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `"file":"kids/kid.yaml"`) {
		t.Errorf("the child's mistake is written as %s", written)
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

// An alias that contains itself is refused, and so is a document whose
// aliases make far more than it writes, at once. Aliases as people write
// them are read.
func Test_mach_alias_growth_is_refused(t *testing.T) {
	bomb := func(levels int) string {
		text := "probe: p\nvalue:\n  l0: &l0 [x,x,x,x,x,x,x,x,x,x]\n"
		for i := 1; i < levels; i++ {
			text += fmt.Sprintf("  l%d: &l%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*l%d,", i-1), 10), ","))
		}
		return text
	}
	if _, err := Parse([]byte(bomb(2)), files(nil)); err != nil {
		t.Errorf("two levels of aliases: %v", err)
	}
	began := time.Now()
	for _, levels := range []int{7, 30} {
		if got := problems(t, bomb(levels), nil); !strings.Contains(got, "document contains excessive aliasing") {
			t.Errorf("%d levels of aliases: %s", levels, got)
		}
	}
	if _, err := Written([]byte(bomb(30))); err == nil || !strings.Contains(err.Error(), "document contains excessive aliasing") {
		t.Errorf("30 levels of aliases shown to a person: %v", err)
	}
	if took := time.Since(began); took > time.Second {
		t.Errorf("refusing them took %s", took)
	}
	for _, src := range []string{"probe: p\nvalue: &a [1, *a]\n", "probe: p\nvalue: &a { x: *a }\n"} {
		if got := problems(t, src, nil); !strings.Contains(got, "value contains itself") {
			t.Errorf("%q: %s", src, got)
		}
	}
}

// What a definition holds is counted as it is stored: the size counted for
// each machine is the size of the definition Store writes, its files and
// child definitions named by their references.
func Test_mach_definition_counted_as_stored(t *testing.T) {
	contents := map[string]string{"a.js": "return 1", "b.js": strings.Repeat("x", 3000), "lib/c.js": "", "kid.yaml": "probe: kid\nwork:\n  one: ./a.js\n"}
	draft, err := Parse([]byte("probe: p\nmeta: { owner: alice }\nwork:\n  a: { script: ./a.js, files: [./lib/c.js] }\n  b: ./b.js\n  kid: { machine: ./kid.yaml }\n  here: { machine: { probe: here, work: { one: ./b.js } } }\n"), files(contents))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := draft.Store(context.Background(), st, "ctl", store.NewInstance("c", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var sizes func(d *Draft, ref blob.Ref)
	sizes = func(d *Draft, ref blob.Ref) {
		if ref.Size != int64(d.size) {
			t.Errorf("%s: stored %d bytes, counted %d", d.Definition.Name, ref.Size, d.size)
		}
		def, err := Load(context.Background(), st, ref)
		if err != nil {
			t.Fatal(err)
		}
		for name, child := range d.Children {
			sizes(child, ref.Beside(def.Machines[name]))
		}
	}
	sizes(draft, ref)
	if len(draft.Children) != 2 {
		t.Errorf("the draft has %d children", len(draft.Children))
	}

	// 30,000 jobs, each naming an empty script of its own, are a text of
	// 0.6 MiB whose stored form, with a reference for each script, is past
	// 4 MiB: refused before anything is stored.
	var src strings.Builder
	scripts := map[string]string{}
	src.WriteString("probe: p\nwork:\n")
	for i := range 30000 {
		fmt.Fprintf(&src, "  j%d: ./s%d.js\n", i, i)
		scripts[fmt.Sprintf("s%d.js", i)] = ""
	}
	began := time.Now()
	if got := problems(t, src.String(), scripts); !strings.Contains(got, "the machine as it is stored is") {
		t.Errorf("30,000 scripts: %.500s", got)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("refusing 30,000 scripts took %s", took)
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
	for _, want := range []string{"work.a.allow[0]", `"fs.read:${item}/**"`, "from the machine's input and its values, and from nothing else", `reads "item"`} {
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
		"probe: p\napprove: [exec]\nwork:\n  a: { script: ./a.js }\n",
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
  job: { script: ./a.js, files: [./lib.js], input: { n: "${input.n}" }, allow: [exec:git], timeout: 90s, restart: { limit: 2 }, memory: 256MB, tags: [gpu] }
  kid: { machine: ./child.yaml, input: { n: 1 }, timeout: 1h, restart: { limit: 1 } }
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
	if job.Script != "./a.js" || job.Memory != 256<<20 || job.Timeout != "90s" || job.Restart == nil || job.Restart.Limit != 2 || len(job.Files) != 1 {
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
	if got := problems(t, "probe: p\nwork:\n  a: { script: /etc/passwd }\n", contents); !strings.Contains(got, "has no ../") {
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

// A file that is not YAML names the line its problem is on, whether the
// YAML reader's parser or its scanner found it.
func Test_mach_not_yaml_names_its_line(t *testing.T) {
	for src, line := range map[string]string{
		"probe: p\n# c\n# c\ninput: { days: number? }\n": "line 4:",
		"probe: p\nmeta: [1, 2\n":                        "line 2:",
		"probe: p\nmeta:\n\t- x\n":                       "line 3:",
	} {
		if got := problems(t, src, nil); !strings.Contains(got, "this is not YAML that can be read: "+line) {
			t.Errorf("%q gave:\n%s", src, got)
		}
	}
}
