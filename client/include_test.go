package client_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
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
		"tools.yaml": "add: { does: Add, job: { include: ./job.yaml } }\n",
		"base.yaml":  "model: { include: ./model.yaml }\nprompt: hi\n",
		"model.yaml": "name: m\nservice: anthropic\naddress: http://127.0.0.1:1\nsecret: KEY\n",
		"to.yaml":    "to: parent\nevent: seen\n",
		"waits.yaml": "first: { job: { include: ./job.yaml }, on: { done: { include: ./go.yaml } } }\nlast: { end: done }\n",
		"go.yaml":    "go: last\nsend: { include: ./to.yaml }\n",
	}
	for name, def := range map[string]string{
		"a flow":    "flow: f\nstages:\n  one: { job: { include: ./job.yaml } }\n",
		"a chart":   "statechart: c\ninitial: first\nstates: { include: ./waits.yaml }\n",
		"an agent":  "agent: a\ninclude: ./base.yaml\ntools: { include: ./tools.yaml }\n",
		"in a part": "agent: a\ninclude: ./base.yaml\nopen:\n  p: { does: P, tools: { include: ./tools.yaml } }\n",
		"in place":  "flow: f\nstages:\n  one: { job: { machine: { agent: a, include: ./base.yaml, tools: { include: ./tools.yaml } } } }\n",
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
			short: { does: S, job: ./lib/short.js }
			long: { does: L, job: { script: ./lib/long.js, files: [./lib/one.js, top.js] } }
			named: { does: N, job: { machine: ./lib/child.yaml } }
			placed:
			  does: P
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

// A file included by a file is read from where it is, and a statechart's send
// names its machine from the file that writes it.
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
			  on: { done: { go: b, send: { to: { key: k, machine: ./keyed.yaml }, event: e } } }
			b:
			  jobs: { named: ./named.js }
			  on: { done: { go: z } }
			z: { end: done }
		`,
		"run.js":             "return 1",
		"one/two/named.js":   "return 1",
		"one/two/keyed.yaml": "flow: keyed\nstages: { one: { job: ./k.js } }\n",
		"one/two/k.js":       "return 1",
	})
	for _, name := range []string{"./run.js", "./one/two/named.js"} {
		if _, ok := d.Definition.Scripts[name]; !ok {
			t.Errorf("the definition does not name %s: %v", name, d.Definition.Scripts)
		}
	}
	if _, ok := d.Definition.Machines["./one/two/keyed.yaml"]; !ok {
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
			  does: A tool named as a field is
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
		"not a mapping": {"flow: f\ninclude: ./l.yaml\n", map[string]string{"l.yaml": "- a\n"}, `include: "./l.yaml" cannot be read: it should be a mapping`},
		"a field that does not belong": {"flow: f\nstages: { include: ./s.yaml }\n", map[string]string{"s.yaml": "one: { colour: red, job: ./a.js }\n", "a.js": "return 1"},
			`s.yaml: stages.one.colour: "colour" is not a field that belongs here`},
		"a machine in place with no type": {"flow: f\nstages: { one: { job: { machine: { include: ./m.yaml } } } }\n", map[string]string{"m.yaml": "flow: inner\nstages: { one: { job: ./a.js } }\n", "a.js": "return 1"},
			"stages.one.job.machine: a machine written in place writes its type and its name itself"},
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
	if err == nil || !strings.Contains(err.Error(), "more than 1000 files are included") {
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
