package client_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
)

// A flow that takes its stages from a file. Whatever the mapping writes stands
// before a file's, a file's own before what it includes, and an earlier file's
// before a later one's.
func TestIncludeGivesAFilesFields(t *testing.T) {
	stage := func(n int) string { return fmt.Sprintf("{ job: { script: ./add.js, input: { n: %d } } }", n) }
	r := run(t, local(t), `
		flow: sums
		stages:
		  include: [./first.yaml, ./second.yaml]
		  own: `+stage(1)+`
		output: { own: "${stages.own.output.n}", a: "${stages.a.output.n}", b: "${stages.b.output.n}", c: "${stages.c.output.n}" }
	`, map[string]string{
		"add.js":      scripts["add.js"],
		"first.yaml":  "include: ./inner.yaml\na: " + stage(10) + "\nown: " + stage(1000) + "\n",
		"inner.yaml":  "a: " + stage(2000) + "\nb: " + stage(20) + "\n",
		"second.yaml": "b: " + stage(3000) + "\nc: " + stage(30) + "\n",
	}, nil)
	check(t, r.Status, r.Output, "done", `{"own": 2, "a": 11, "b": 21, "c": 31}`)
}

// include: works in any mapping of fields and any mapping of names, in every
// type of machine, and the stored definition holds no include:.
func TestIncludeWorksInEveryMapping(t *testing.T) {
	files := map[string]string{
		"add.js":     scripts["add.js"],
		"job.yaml":   "script: ./add.js\ninput: { n: 1 }\n",
		"tools.yaml": "add: { description: Add, job: { include: ./job.yaml } }\n",
		"base.yaml":  "model: { include: ./model.yaml }\nprompt: hi\n",
		"model.yaml": "name: m\nservice: anthropic\naddress: http://127.0.0.1:1\nsecret: KEY\n",
		"to.yaml":    "to: parent\nevent: seen\n",
		"waits.yaml": "first: { job: { include: ./job.yaml }, on: { done: { include: ./go.yaml } } }\nlast: { end: done }\n",
		"go.yaml":    "go: last\nsend: { include: ./to.yaml }\n",
	}
	for name, def := range map[string]string{
		"a flow":       "flow: f\nstages:\n  one: { job: { include: ./job.yaml } }\n",
		"a chart":      "statechart: c\ninitial: first\nstates: { include: ./waits.yaml }\n",
		"an agent":     "agent: a\ninclude: ./base.yaml\ntools: { include: ./tools.yaml }\n",
		"in a toolkit": "agent: a\ninclude: ./base.yaml\ntoolkits:\n  p: { description: P, tools: { include: ./tools.yaml } }\n",
		"in place":     "flow: f\nstages:\n  one: { job: { machine: { agent: a, include: ./base.yaml, tools: { include: ./tools.yaml } } } }\n",
	} {
		d, err := machineParse(def, files)
		if err != nil {
			t.Errorf("%s was refused: %v", name, err)
			continue
		}
		if strings.Contains(string(d.Definition.Body), "include") {
			t.Errorf("%s stored an include: %s", name, d.Definition.Body)
		}
	}
}

// Everything a job names is read from the directory of the file it is written
// in: a script, its files, a machine, a machine written in place and what it
// names, a job written as a path, an ask, a list and a turn.
func TestIncludedPathsAreReadFromTheFilesDirectory(t *testing.T) {
	d := draft(t, `
		agent: a
		include: ./kit/base.yaml
		tools: { include: ./kit/tools.yaml }
	`, map[string]string{
		"kit/base.yaml": `
			model: { name: m, service: anthropic, address: "http://127.0.0.1:1", secret: KEY }
			prompt: hi
			ask: ./ask.js
			turn: ./turn.js
		`,
		"kit/tools.yaml": `
			short: { description: S, job: ./lib/short.js }
			long: { description: L, job: { script: ./lib/long.js, files: [./lib/one.js, top.js] } }
			named: { description: N, job: { machine: ./lib/child.yaml } }
			placed:
			  description: P
			  job:
			    machine:
			      flow: placed
			      stages: { one: { job: ./lib/placed.js }, two: { job: { machine: ./lib/child.yaml } } }
			listed: { list: ./lib/list.js, job: ./lib/list.js }
		`,
		"kit/ask.js":         "return {}",
		"kit/turn.js":        "return {}",
		"kit/lib/short.js":   "return 1",
		"kit/lib/long.js":    "return 1",
		"kit/lib/one.js":     "return 1",
		"top.js":             "return 1",
		"kit/lib/placed.js":  "return 1",
		"kit/lib/list.js":    "return []",
		"kit/lib/child.yaml": "flow: child\nstages: { one: { job: ./beside.js } }\n",
		"kit/lib/beside.js":  "return 1",
	})
	for _, name := range []string{"./kit/ask.js", "./kit/turn.js", "./kit/lib/short.js", "./kit/lib/long.js", "./kit/lib/one.js", "./kit/lib/list.js"} {
		if _, ok := d.Definition.Scripts[name]; !ok {
			t.Errorf("the definition does not name %s: %v", name, d.Definition.Scripts)
		}
	}
	if _, ok := d.Definition.Scripts["../top.js"]; ok {
		t.Errorf("../top.js was read from the top file: %v", d.Definition.Scripts)
	}
	if _, ok := d.Definition.Scripts["./top.js"]; !ok {
		t.Errorf("the definition does not name ./top.js: %v", d.Definition.Scripts)
	}
	if _, ok := d.Definition.Machines["./kit/lib/child.yaml"]; !ok {
		t.Errorf("the definition's machines are %v", d.Definition.Machines)
	}
}

// A file included by a file is read from where it is, and a machine a job
// names is read from the file that writes it.
func TestIncludedPathsMoveThroughEveryFile(t *testing.T) {
	d := draft(t, `
		statechart: c
		initial: a
		states: { include: ./one/states.yaml }
	`, map[string]string{
		"one/states.yaml": "include: ./two/more.yaml\n",
		"one/two/more.yaml": `
			a:
			  job: run.js
			  on: { done: b }
			b:
			  jobs: { named: ./named.js, child: { machine: ./child.yaml } }
			  on: { done: { go: z } }
			z: { end: done }
		`,
		"run.js":             "return 1",
		"one/two/named.js":   "return 1",
		"one/two/child.yaml": "flow: child\nstages: { one: { job: ./k.js } }\n",
		"one/two/k.js":       "return 1",
	})
	for _, name := range []string{"./run.js", "./one/two/named.js"} {
		if _, ok := d.Definition.Scripts[name]; !ok {
			t.Errorf("the definition does not name %s: %v", name, d.Definition.Scripts)
		}
	}
	if _, ok := d.Definition.Machines["./one/two/child.yaml"]; !ok {
		t.Errorf("the definition's machines are %v", d.Definition.Machines)
	}
}

// Data stays as it is written. A value under input:, whatever it looks like,
// is no path and no include:, and neither is one in meta: or the shape of an
// input, and a tool may be called anything.
func TestIncludeLeavesDataAlone(t *testing.T) {
	d := draft(t, `
		agent: a
		include: ./kit/base.yaml
		tools: { include: ./kit/tools.yaml }
	`, map[string]string{
		"kit/base.yaml": `
			input: { include: string }
			model: { name: m, service: anthropic, address: "http://127.0.0.1:1", secret: KEY }
			prompt: ./not-a-path.txt
			meta: { include: ./nothing.yaml, machine: ./nothing.yaml }
		`,
		"kit/tools.yaml": `
			script:
			  description: A tool named as a field is
			  args: { include: string }
			  job:
			    script: ./run.js
			    input: { script: ./x.js, files: [./y.js], machine: ./z.yaml, include: ./nothing.yaml, job: ./w.js }
		`,
		"kit/run.js": "return 1",
	})
	body := string(d.Definition.Body)
	for _, want := range []string{`"script":"./x.js"`, `"files":["./y.js"]`, `"machine":"./z.yaml"`, `"include":"./nothing.yaml"`, `"job":"./w.js"`, `"./not-a-path.txt"`, `"./kit/run.js"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the stored definition does not hold %s: %s", want, body)
		}
	}
	if meta := fmt.Sprint(d.Definition.Meta); !strings.Contains(meta, "./nothing.yaml") {
		t.Errorf("meta is %s", meta)
	}
}

// An included file is read as a definition is: a number is a number, a value
// YAML would read as a date stays the text written, and an empty file is
// refused, as an empty definition is.
func Test_mach_included_file_is_read_as_a_definition_is(t *testing.T) {
	def := "flow: f\nstages: { include: ./stages.yaml }\n"
	d := draft(t, def, map[string]string{
		"add.js":      scripts["add.js"],
		"stages.yaml": "one:\n  job: { script: ./add.js, restart: { limit: 2 }, input: { since: 2026-01-02 } }\n",
	})
	for _, want := range []string{`"limit":2`, `"since":"2026-01-02"`} {
		if !strings.Contains(string(d.Definition.Body), want) {
			t.Errorf("the stored definition does not hold %s: %s", want, d.Definition.Body)
		}
	}
	if _, err := machineParse(def, map[string]string{"stages.yaml": ""}); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("an empty file gave: %v", err)
	}
	// A file that is a list is included into a list.
	d = draft(t, "flow: f\nstages: { one: { job: ./add.js } }\nallow: [{ include: ./l.yaml }]\n", map[string]string{"add.js": scripts["add.js"], "l.yaml": "- net:example.com\n"})
	if !strings.Contains(jsonOf(d.Definition), `"allow":["net:example.com"]`) {
		t.Errorf("a list file gave %s", jsonOf(d.Definition))
	}
}

// What is wrong with an include: is said at the include:, and what is wrong in
// an included file names that file.
func Test_mach_include_problems_name_the_file(t *testing.T) {
	for name, c := range map[string]struct {
		def   string
		files map[string]string
		want  string
	}{
		"climbing":      {"flow: f\ninclude: ../stages.yaml\n", nil, `include: "../stages.yaml": a path is read from the context, or from this file's directory with ./, and has no ../`},
		"not text":      {"flow: f\ninclude: [3]\n", nil, "include: include: names a file or a list of them, and this is a number"},
		"missing":       {"flow: f\ninclude: ./no.yaml\n", nil, `include: "./no.yaml" cannot be read: no such file`},
		"not a mapping": {"flow: f\ninclude: ./l.yaml\n", map[string]string{"l.yaml": "- a\n"}, `include: "./l.yaml" is a list, and only a mapping is merged into a mapping`},
		"a field that does not belong": {"flow: f\nstages: { include: ./s.yaml }\n", map[string]string{"s.yaml": "one: { colour: red, job: ./a.js }\n", "a.js": "return 1"},
			`s.yaml: stages.one.colour: "colour" is not a field that belongs here`},
		"a machine in place with no type": {"flow: f\nstages: { one: { job: { machine: { include: ./m.yaml } } } }\n", map[string]string{"m.yaml": "stages: { one: { job: ./a.js } }\n", "a.js": "return 1"},
			"stages.one.job.machine: it gives no type: a machine writes one of agent:, flow:, statechart: with its name, or takes it from a file it includes\n"},
		"a file below a file": {"flow: f\nstages: { include: ./a/s.yaml }\n", map[string]string{"a/s.yaml": "include: ./t.yaml\n", "a/t.yaml": "one: { colour: red, job: ./a.js }\n", "a/a.js": "return 1"},
			`a/t.yaml: stages.one.colour: "colour" is not a field that belongs here`},
	} {
		if _, err := machineParse(c.def, c.files); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got: %v", name, c.want, err)
		}
	}
	// What the definition writes itself names no included file.
	_, err := machineParse("flow: f\nstages:\n  include: ./s.yaml\n  mine: { colour: red, job: ./a.js }\n",
		map[string]string{"s.yaml": "one: { job: ./a.js }\n", "a.js": "return 1"})
	if err == nil || strings.Contains(err.Error(), "s.yaml") || !strings.Contains(err.Error(), "stages.mine.colour") {
		t.Errorf("a problem in what the definition writes gave: %v", err)
	}
}

// Files that each include the next twice would grow past what anyone could
// hold, and leading back to themselves is not what they do. They are refused
// at once.
func Test_mach_includes_are_limited(t *testing.T) {
	files := map[string]string{"a.js": "return 1", "f40.yaml": "one: { job: ./a.js }\n"}
	for i := 0; i < 40; i++ {
		files[fmt.Sprintf("f%d.yaml", i)] = fmt.Sprintf("a: { job: { machine: { flow: i, stages: { include: ./f%d.yaml } } } }\nb: { job: { machine: { flow: i, stages: { include: ./f%d.yaml } } } }\n", i+1, i+1)
	}
	began := time.Now()
	_, err := machineParse("flow: f\nstages: { include: ./f0.yaml }\n", files)
	if err == nil || !strings.Contains(err.Error(), "more than 1000 includes are written out") {
		t.Errorf("got: %v", err)
	}
	if took := time.Since(began); took > 5*time.Second {
		t.Errorf("refusing it took %s", took)
	}
}

// A machine a script makes may include the files it gives.
func TestIncludeInAMachineMadeWhileRunning(t *testing.T) {
	r := run(t, local(t), "flow: maker\nstages:\n  make: { job: ./make.js }\n  run: { job: { machine: \"${stages.make.output}\" } }\noutput: ${stages.run.output}\n", map[string]string{"make.js": `
		return machines.define("flow: made\nstages: { include: ./kit/s.yaml }\noutput: ${stages.one.output}\n",
			{ files: { "./kit/s.yaml": "one: { job: ./run.js }\n", "./kit/run.js": "return 7" } })
	`}, nil)
	check(t, r.Status, r.Output, "done", "7")
}

// family is a definition that includes a file whole, a part of it, and a
// text, in a mapping, a list and a text.
var family = map[string]string{
	"family.yaml": `agent: family
include: ./base.yaml
model: { effort: high }
system:
  - { include: ./style.md }
  - You look after one house.
allow:
  - { include: ./base.yaml#allow }
  - net:devices.lan
`,
	"base.yaml": `model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.said}
allow: ["fs.read:${input.dir}/**"]
`,
	"style.md": "Answer briefly.\n",
}

func parseFamily(t *testing.T, files map[string]string) *machine.Draft {
	t.Helper()
	d, err := machine.ParseIn("family.yaml", []byte(files["family.yaml"]), machine.Given(files))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// The definition is written out before it is stored: mappings merged at every
// depth, lists and texts given their included items, and each value kept with
// its file and line.
func Test_mach_written_out_keeps_file_and_line(t *testing.T) {
	d := parseFamily(t, family)
	want := `agent: family                                # family.yaml:1
model:
  name: claude-sonnet-5-5                    # base.yaml:1
  secret: ANTHROPIC_KEY                      # base.yaml:1
  effort: high                               # family.yaml:3
prompt: ${input.said}                        # base.yaml:2
allow:
  - fs.read:${input.dir}/**                  # base.yaml:3
  - net:devices.lan                          # family.yaml:9
system: |-                                   # style.md:1
  Answer briefly.

  You look after one house.
`
	if got := string(d.WrittenOut()); got != want {
		t.Errorf("written out:\n%s\nwant:\n%s", got, want)
	}
	for place, at := range map[string]string{"model.name": "base.yaml:1", "model.effort": "family.yaml:3", "allow[0]": "base.yaml:3", "allow[1]": "family.yaml:9", "system": "style.md:1"} {
		if got := d.Definition.Lines[place]; got != at {
			t.Errorf("%s is kept at %q, not %q", place, got, at)
		}
	}
	stored := jsonOf(d.Definition)
	if !strings.Contains(stored, `"lines":{`) || strings.Contains(stored, "include") {
		t.Errorf("the stored definition is %s", stored)
	}
	// A machine kept in its own file is printed as its path.
	d = draft(t, "flow: f\nstages:\n  one: { job: { machine: ./kid.yaml } }\n", map[string]string{"kid.yaml": "flow: kid\nstages: { one: { job: ./a.js } }\n", "a.js": "return 1"})
	if got := string(d.WrittenOut()); !strings.Contains(got, "machine: ./kid.yaml") || strings.Contains(got, "flow: kid") {
		t.Errorf("a machine file is printed as:\n%s", got)
	}
}

// In a mapping, include: merges at every depth. What the including file writes
// wins, a list or a text it writes replaces the included one whole, a value of
// another kind wins whole, and a file reached by two routes counts once.
func Test_mach_include_merges_at_every_depth(t *testing.T) {
	files := map[string]string{
		"family.yaml": `agent: family
include: [./a.yaml, ./b.yaml]
model: { effort: high }
tools:
  include: ./tools.yaml
  read: { description: Read one shelf }
limits: { turns: 3 }
`,
		"a.yaml":     "include: ./base.yaml\nlimits: { jobs: 5 }\n",
		"b.yaml":     "include: ./base.yaml\nprompt: from b\n",
		"base.yaml":  "model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }\nprompt: ${input.said}\nlimits: { turns: 9, timeout: 1h }\n",
		"tools.yaml": "read: { description: Read, job: ./read.js }\nwrite: { description: Write, job: ./write.js }\n",
		"read.js":    "return 1",
		"write.js":   "return 1",
	}
	d := parseFamily(t, files)
	out := string(d.WrittenOut())
	for _, want := range []string{"effort: high", "name: claude-sonnet-5-5", "prompt: ${input.said}", "description: Read one shelf", "job: read.js", "description: Write", "turns: 3", "jobs: 5", "timeout: 1h"} {
		if !strings.Contains(out, want) {
			t.Errorf("the written out definition does not hold %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "from b") || strings.Contains(out, "turns: 9") {
		t.Errorf("an included value won over one written before it:\n%s", out)
	}
	// A mistake in a file reached by two routes is said once.
	files["base.yaml"] += "colour: red\n"
	_, err := machine.ParseIn("family.yaml", []byte(files["family.yaml"]), machine.Given(files))
	if err == nil || strings.Count(err.Error(), "base.yaml: colour") != 1 {
		t.Errorf("a mistake reached twice is said as: %v", err)
	}
	// A machine written in place takes its type from a file it includes.
	d = draft(t, "flow: f\nstages: { one: { job: { machine: { include: ./m.yaml } } } }\n", map[string]string{"m.yaml": "flow: inner\nstages: { one: { job: ./a.js } }\n", "a.js": "return 1"})
	if len(d.Children) != 1 {
		t.Errorf("the machine in place gave %v", d.Children)
	}
	// One tool is taken from a file by its path, and no other.
	d = draft(t, "agent: one\nmodel: { name: m, service: anthropic, secret: K }\nprompt: hi\ntools:\n  read: { include: ./tools.yaml#read }\n", map[string]string{
		"tools.yaml": "read: { description: Read, job: ./read.js }\nwrite: { description: Write, job: ./write.js }\n", "read.js": "return 1", "write.js": "return 1"})
	if out := string(d.WrittenOut()); !strings.Contains(out, "description: Read") || strings.Contains(out, "write") {
		t.Errorf("one tool taken by its path gave:\n%s", out)
	}
	// A value of another kind than the included one wins whole.
	d = draft(t, "flow: f\ninclude: ./base.yaml\nstages:\n  one: { job: ./a.js }\n", map[string]string{"base.yaml": "stages: two\nkeep: 1d\n", "a.js": "return 1"})
	if !strings.Contains(jsonOf(d.Definition), `"keep":"24h0m0s"`) || strings.Contains(string(d.WrittenOut()), "two") {
		t.Errorf("the definition is %s", d.WrittenOut())
	}
}

// Inside a list, an item { include: <file> } is replaced by the file's list or
// value, and { include: <file>#<path> } by the value at that path. Data is
// left as it is written.
func Test_mach_include_splices_into_a_list(t *testing.T) {
	d := draft(t, `
		flow: f
		allow:
		  - { include: ./perms.yaml }
		  - net:c.example
		  - { include: ./base.yaml#extra.allow }
		deny: [{ include: ./base.yaml#deny }]
		stages:
		  include: ./base.yaml#stages
		  two: { job: { include: ./base.yaml#stages.one.job, input: [{ include: ./perms.yaml }] } }
		meta: [{ include: ./perms.yaml }]
	`, map[string]string{
		"perms.yaml": "- net:a.example\n- { include: ./more.yaml }\n",
		"more.yaml":  "[net:b.example]\n",
		"base.yaml":  "extra: { allow: net:d.example }\ndeny: exec\nstages:\n  one: { job: { script: ./a.js } }\n",
		"a.js":       "return 1",
	})
	stored := jsonOf(d.Definition)
	for _, want := range []string{`"allow":["net:a.example","net:b.example","net:c.example","net:d.example"]`, `"deny":["exec"]`,
		`"meta":[{"include":"./perms.yaml"}]`, `"input":[{"include":"./perms.yaml"}]`, `"two":{`} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored definition does not hold %s: %s", want, stored)
		}
	}
	for name, c := range map[string]struct{ def, want string }{
		"nothing at the path":   {"flow: f\nstages: { one: { job: ./a.js } }\nallow: [{ include: ./base.yaml#nothing }]\n", `allow[0].include: "./base.yaml" has nothing at nothing`},
		"a path into a mapping": {"flow: f\nstages: { include: ./base.yaml#deny }\n", `stages.include: "./base.yaml#deny" is text, and only a mapping is merged into a mapping`},
		"a list into a mapping": {"flow: f\nstages: { include: ./list.yaml }\n", `stages.include: "./list.yaml" is a list, and only a mapping is merged into a mapping`},
	} {
		_, err := machineParse(c.def, map[string]string{"base.yaml": "deny: exec\n", "list.yaml": "- a\n", "a.js": "return 1"})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got: %v", name, c.want, err)
		}
	}
}

// A field whose value may be a list of what it holds, such as a state's
// send:, splices an included item or list into its list, and receive: takes
// included entries in both its forms.
func TestIncludeSplicesIntoAListOfThings(t *testing.T) {
	d := draft(t, `
		statechart: s
		initial: a
		states:
		  a:
		    on: { go: b }
		    entry:
		      send:
		        - { to: parent, event: one }
		        - { include: ./sends.yaml }
		  b: { end: done }
		receive:
		  set: [{ include: ./who.yaml }, client:panel]
	`, map[string]string{
		"sends.yaml": "- { to: parent, event: two }\n- { to: parent, event: three }\n",
		"who.yaml":   "[thermostats/*, /work/**]\n",
	})
	stored := jsonOf(d.Definition)
	for _, want := range []string{`"event":"one"`, `"event":"two"`, `"event":"three"`, `"receive":{"set":["thermostats/*","/work/**","client:panel"]}`} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored definition does not hold %s: %s", want, stored)
		}
	}
	if strings.Contains(stored, "include") {
		t.Errorf("the stored definition holds an include: %s", stored)
	}
	d = draft(t, "statechart: s\ninitial: a\nstates: { a: { end: done } }\nreceive: { include: ./by-event.yaml }\n",
		map[string]string{"by-event.yaml": "set: [client:panel]\nstop: [{ include: ./who.yaml }]\n", "who.yaml": "- client:admin\n"})
	if got := jsonOf(d.Definition); !strings.Contains(got, `"receive":{"set":["client:panel"],"stop":["client:admin"]}`) {
		t.Errorf("receive: by event from a file is stored as %s", got)
	}
}

// A definition read from a file in a directory reads the files it includes
// from that directory, keeps their lines by their paths, and names its own
// file where an include leads back to it.
func TestIncludeFromAFileInADirectory(t *testing.T) {
	files := map[string]string{
		"house/boiler.yaml": "flow: boiler\ninclude: ./base.yaml\nstages: { one: { job: ./a.js } }\n",
		"house/base.yaml":   "keep: 1d\n",
		"house/a.js":        "return 1",
	}
	d, err := machine.ParseIn("house/boiler.yaml", []byte(files["house/boiler.yaml"]), machine.Given(files))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Definition.Lines["keep"]; got != "house/base.yaml:1" {
		t.Errorf("keep is kept at %q", got)
	}
	if got := d.Definition.Lines["flow"]; got != "house/boiler.yaml:1" {
		t.Errorf("flow is kept at %q", got)
	}
	files["house/base.yaml"] = "include: ./boiler.yaml\n"
	_, err = machine.ParseIn("house/boiler.yaml", []byte(files["house/boiler.yaml"]), machine.Given(files))
	if err == nil || !strings.Contains(err.Error(), "the definition leads back to itself: house/boiler.yaml -> house/base.yaml -> house/boiler.yaml") {
		t.Errorf("an include that leads back to the file gave: %v", err)
	}
}

// A text may be a list of pieces, each a text or { include: … }, joined by one
// empty line, and { include: <file> } alone is a text of one piece.
func Test_mach_text_is_joined_from_pieces(t *testing.T) {
	files := map[string]string{"intro.md": "Be brief.\n\nBe kind.\n", "base.yaml": "words: { closing: Thank you. }\n",
		"trail.md": "Line one.\n\n\n\n", "lead.md": "\n\nLead.\n"}
	for def, want := range map[string]string{
		"flow: f\nstages: { one: { job: ./a.js } }\ntitle: [{ include: ./trail.md }, { include: ./lead.md }, plain]\n":                       `"title":"Line one.\n\nLead.\n\nplain"`,
		"flow: f\nstages: { one: { job: ./a.js } }\ntitle:\n  - |\n    Block one.\n  - |\n    Block two.\n  - plain\n":                       `"title":"Block one.\n\nBlock two.\n\nplain"`,
		"flow: f\nstages: { one: { job: ./a.js } }\ntitle: [{ include: ./intro.md }, Then go on., { include: ./base.yaml#words.closing }]\n": `"title":"Be brief.\n\nBe kind.\n\nThen go on.\n\nThank you."`,
		"flow: f\nstages: { one: { job: ./a.js } }\ntitle: { include: ./intro.md }\n":                                                        `"title":"Be brief.\n\nBe kind."`,
	} {
		files["a.js"] = "return 1"
		d := draft(t, def, files)
		if got := jsonOf(d.Definition); !strings.Contains(got, want) {
			t.Errorf("%s gave %s", def, got)
		}
	}
	_, err := machineParse("flow: f\nstages: { one: { job: ./a.js } }\ntitle: [3]\n", map[string]string{"a.js": "return 1"})
	if err == nil || !strings.Contains(err.Error(), "title[0]: a piece of a text is text or { include: <file> }") {
		t.Errorf("a piece that is a number gave: %v", err)
	}
}

// ${…} in an included text is an expression, checked with the definition and
// filled in at run time, and ${'${'} is a literal ${.
func Test_mach_included_text_holds_expressions(t *testing.T) {
	_, err := machineParse("flow: f\nstages: { one: { job: ./a.js } }\ntitle: { include: ./t.md }\n", map[string]string{"t.md": "On ${input.topic +}", "a.js": "return 1"})
	if err == nil || !strings.Contains(err.Error(), "title: in ${input.topic +}") {
		t.Errorf("a mistake in an included text gave: %v", err)
	}
	r := run(t, local(t), "flow: f\ntitle: { include: ./t.md }\nstages: { one: { job: { script: ./a.js, input: \"${'${'}x}\" } } }\noutput: ${stages.one.output}\n",
		map[string]string{"t.md": "On ${input.topic}", "a.js": "return input"}, map[string]any{"topic": "bees"})
	check(t, r.Status, r.Output, "done", `"${x}"`)
	if r.Title != "On bees" {
		t.Errorf("the title is %q", r.Title)
	}
}

// What a definition runs has a hash: moving a line changes nothing, and
// changing a piece it includes changes it.
func Test_mach_hash_leaves_out_lines(t *testing.T) {
	before := parseFamily(t, family).Hash()
	moved := map[string]string{"family.yaml": family["family.yaml"], "base.yaml": "# a comment\n\n" + family["base.yaml"], "style.md": family["style.md"]}
	if parseFamily(t, moved).Hash() != before {
		t.Error("moving lines changed the hash")
	}
	moved["style.md"] = "Answer at length.\n"
	if parseFamily(t, moved).Hash() == before {
		t.Error("changing a piece left the hash as it was")
	}
}
