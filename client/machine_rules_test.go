package client_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
)

// A permission's scope may be filled from the machine's input and from
// nothing else, in every type of machine: what a script gave back, an item,
// an event or what a model asked for never chooses a scope.
func TestPermissionScopeFilledFromAnythingButTheInputIsRefused(t *testing.T) {
	const chart = `
		statechart: c
		initial: a
		states:
		  a: { job: ./add.js, on: { done: b } }
		  b: { job: { script: ./add.js, allow: ["ENTRY"] }, on: { done: c } }
		  c: { end: done }
	`
	for _, c := range []struct{ def, entry, place string }{
		{`
			flow: f
			stages:
			  a: { job: ./add.js }
			  b: { job: { script: ./add.js, allow: ["ENTRY"] } }
		`, "fs.read:${stages.a.output.dir}/**", "stages.b.job.allow[0]"},
		{`
			flow: f
			stages:
			  b: { each: [/etc], job: { script: ./add.js, allow: ["ENTRY"] } }
		`, "fs.read:${item}/**", "stages.b.job.allow[0]"},
		{chart, "fs.read:${states.a.output.dir}/**", "states.b.job.allow[0]"},
		{chart, "net:${event.data.host}", "states.b.job.allow[0]"},
		{strings.Replace(agentDef("http://127.0.0.1:1", ""), `input: { n: "${args.n}" }`, `input: { n: "${args.n}" }, allow: ["ENTRY"]`, 1),
			"fs.read:${args.n}/**", "tools.add.job.allow[0]"},
	} {
		_, err := machineParse(strings.Replace(c.def, "ENTRY", c.entry, 1), scripts)
		if err == nil {
			t.Errorf("%s passed the check", c.entry)
			continue
		}
		for _, want := range []string{c.place, `"` + c.entry + `"`, "from the machine's input and its values, and from nothing else"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s was refused without %q:\n%v", c.entry, want, err)
			}
		}
	}
	flowRefused(t, `
		flow: f
		input: { dir: any }
		stages:
		  a: { job: { script: ./add.js, deny: ["fs.read:${input.dir}/**"] } }
	`, scripts, "a deny entry is written out in full")
}

// A tool's scope filled from the agent's input passes, and is filled from
// it: the tool reads the directory the agent was started with.
func TestAgentToolScopeIsFilledFromTheAgentsInput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("the note"), 0o600)
	m := newModel(t, calls("c1", "look", `{"name": "note.txt"}`), says("done"))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err := l.Run(ctx, draft(t, `
		agent: reader
		input: { dir: any }
		model: { name: test-model, service: anthropic, address: `+m.URL+`, secret: TEST_KEY }
		prompt: Read the note.
		tools:
		  look:
		    does: Read a file of the directory
		    args: { name: string }
		    job: { script: ./look.js, input: { path: "${input.dir}/${args.name}" }, allow: ["fs.read:${input.dir}/**"] }
	`, map[string]string{"look.js": `return fs.read(input.path)`}), client.Start{
		Input: map[string]any{"dir": dir}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "the note") {
		t.Fatalf("the tool's result was shown as: %s", shown)
	}
}

// A definition a script made may mark a job for approval and have no ask of
// its own. Whether an ask applies is settled where it is started: under a
// machine with an ask it runs, and with no ask anywhere above it fails at
// its start, saying so.
func TestMadeDefinitionsApprovalsAreCheckedWhereItIsStarted(t *testing.T) {
	files := map[string]string{
		"make.js": `
			return machines.define(
				"flow: made\nstages:\n  held: { job: { script: ./step.js, approve: true } }\noutput: ${stages.held.output}\n",
				{ files: { "./step.js": "return 7" } })
		`,
		"ask.js": `return { approved: input.jobs.map(r => r.name) }`,
	}
	const maker = `
		flow: maker
		ASK
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}" } }
		output: ${stages.go.output}
	`
	l := local(t)
	r := run(t, l, strings.Replace(maker, "ASK", "ask: ./ask.js", 1), files, nil)
	check(t, r.Status, r.Output, "done", `7`)
	r = run(t, l, strings.Replace(maker, "ASK", "", 1), files, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "go: ") || !strings.Contains(r.Error, "no ask applies") {
		t.Fatalf("with no ask above it the made definition's run ended %s: %s", r.Status, r.Error)
	}
}

// A tool's result too large to show that is one plain text is stored as
// the text it is, and the model is given its handle: its items are the
// text's lines.
func TestLargeTextResultIsStoredAsText(t *testing.T) {
	m := newModel(t, calls("c1", "lines", `{"n": 3000}`), calls("c2", "blob_read", `{"blob": "blob1", "from": 2999}`), says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  lines:
		    does: Give many lines
		    args: { n: number }
		    job: { script: ./lines.js, input: { n: "${args.n}" } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"],
		"lines.js": `return Array.from({ length: input.n }, (_, i) => "line number " + i).join("\n")`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := lastShown(m.Calls[1])
	if len(shown) > 4000 || !strings.Contains(shown, `\"blob\":\"blob1\"`) || !strings.Contains(shown, `\"items\":3000`) ||
		!strings.Contains(shown, `\"preview\":[\"line number 0\",\"line number 1\"`) || !strings.Contains(shown, `text/plain`) {
		t.Fatalf("a text of three thousand lines was shown as: %s", shown)
	}
	if read := lastShown(m.Calls[2]); !strings.Contains(read, `line number 2999`) {
		t.Fatalf("reading the end of the stored text gave: %s", read)
	}
}

// A stage that is written wrongly is reported as that, once: it is still a
// stage, and the flow is not also said to have none.
func TestBadlyWrittenStageIsNotAlsoAFlowWithNoStages(t *testing.T) {
	_, err := machineParse(`
		flow: bare
		stages:
		  a: ./add.js
	`, scripts)
	if err == nil || !strings.Contains(err.Error(), "stages.a: this should be a mapping of fields, and is text") ||
		strings.Contains(err.Error(), "at least one stage") {
		t.Fatalf("a stage written as a bare path: %v", err)
	}
	flowRefused(t, "flow: none\nstages: {}\n", nil, "a flow needs at least one stage")
}

// The scripts of a flow whose emitting stage cannot end before one of the
// items it emitted has been started: it waits for the mark an item's job
// leaves. A flow that held the items back until the stage ended would
// never get there, and the stage says so.
var emitsAndWaits = map[string]string{
	"emit.js": `
		for (const x of input.items) emit("item", x)
		const until = Date.now() + 20000
		while (!fs.exists(input.dir + "/started")) {
			if (Date.now() > until) throw new Error("no item was started while the emitting stage was still running")
		}
		return { count: input.items.length }
	`,
	"mark.js": `
		fs.write(input.dir + "/started", { text: "x" })
		return input.n + 1
	`,
}

// A stage that follows another stage's emitted items starts each as it is
// emitted, without waiting for the emitting stage to end.
func TestFlowItemsStartAsTheyAreEmitted(t *testing.T) {
	l := local(t, "disk")
	r := run(t, l, `
		flow: follows
		input: { dir: any }
		allow: ["fs.read:${input.dir}/**", "fs.write:${input.dir}/**"]
		stages:
		  source:
		    job: { script: ./emit.js, input: { dir: "${input.dir}", items: [1, 2, 3] } }
		  per_item:
		    each: { emitted: source }
		    job: { script: ./mark.js, input: { dir: "${input.dir}", n: "${item}" } }
		output:
		  count: ${stages.source.output.count}
		  items: ${stages.per_item.output.map(r => r.output)}
	`, emitsAndWaits, map[string]any{"dir": t.TempDir()})
	if r.Status != "done" {
		t.Fatalf("the flow ended %s: %s", r.Status, r.Error)
	}
	check(t, r.Status, r.Output, "done", `{"count": 3, "items": [2, 3, 4]}`)
}

// A flow that names a stage with emit: sends that stage's emitted items up
// to its parent as they arrive: the parent's stage that follows the child
// starts on the first before the child has ended, and runs once for each.
func TestFlowPassesANamedStagesItemsUpAsTheyArrive(t *testing.T) {
	l := local(t, "disk")
	r := run(t, l, `
		flow: parent
		input: { dir: any }
		allow: ["fs.read:${input.dir}/**", "fs.write:${input.dir}/**"]
		stages:
		  kid:
		    job:
		      input: { dir: "${input.dir}" }
		      machine:
		        flow: kid
		        input: { dir: any }
		        allow: ["fs.read:${input.dir}/**"]
		        emit: source
		        stages:
		          source:
		            job: { script: ./emit.js, input: { dir: "${input.dir}", items: [1, 2, 3] } }
		        output: ${stages.source.output.count}
		  per_item:
		    each: { emitted: kid }
		    job: { script: ./mark.js, input: { dir: "${input.dir}", n: "${item}" } }
		output:
		  count: ${stages.kid.output}
		  items: ${stages.per_item.output.map(r => r.output)}
	`, emitsAndWaits, map[string]any{"dir": t.TempDir()})
	if r.Status != "done" {
		t.Fatalf("the flow ended %s: %s", r.Status, r.Error)
	}
	check(t, r.Status, r.Output, "done", `{"count": 3, "items": [2, 3, 4]}`)
}

// The controller checks a definition itself, with the same code as the
// client: one the client's check never saw is refused when it is started,
// in the words the client's check would have used.
func TestControllerRefusesWhatTheClientLetThroughInTheSameWords(t *testing.T) {
	const def = `
		flow: unchecked
		stages:
		  a: { job: { script: ./add.js, input: { n: "READS" } } }
	`
	_, err := machineParse(strings.Replace(def, "READS", "${stages.missing.output}", 1), scripts)
	if err == nil {
		t.Fatal("the client's check passed a stage that reads one that is not there")
	}
	// A draft the client's check passed, altered afterwards.
	d := draft(t, strings.Replace(def, "READS", "${input.n}", 1), scripts)
	d.Definition.Body = []byte(strings.Replace(string(d.Definition.Body), "${input.n}", "${stages.missing.output}", 1))
	if problems, _ := machine.Check(d.Definition); len(problems) == 0 {
		t.Fatal("the altered definition is not one the check refuses")
	}
	l := local(t)
	id, startErr := l.Run(ctx, d, client.Start{})
	if startErr != nil {
		t.Fatal(startErr)
	}
	r := wait(t, l, id)
	if r.Status != "failed" || !strings.Contains(r.Error, "the definition did not pass its check") || !strings.Contains(r.Error, err.Error()) {
		t.Fatalf("the run ended %s: %s\nthe client's check says:\n%v", r.Status, r.Error, err)
	}
	runs, listErr := l.List(ctx, client.Filter{})
	if listErr != nil || len(runs) != 0 {
		t.Fatalf("a refused definition left %d runs behind (%v)", len(runs), listErr)
	}
}

// A relative path in a child's own file is read from that file's
// directory.
func TestPathIsReadFromTheDirectoryOfItsFile(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: parent
		stages:
		  kid: { job: { machine: ./kids/kid.yaml } }
		output: ${stages.kid.output}
	`, map[string]string{
		"kids/kid.yaml": "flow: kid\nstages:\n  it: { job: ./kid.js }\noutput: ${stages.it.output}\n",
		"kids/kid.js":   `return "the kid's own"`,
		"kid.js":        `return "the parent's"`,
	}, nil)
	if r.Status != "done" || r.Output != "the kid's own" {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// A job's timeout and a run's timeout may come from input. Each is filled
// in when the job is filled or the run starts, and a filled value that is
// not a length of time fails the run, naming its place.
func TestTimeoutComesFromInput(t *testing.T) {
	l := local(t)
	job := `
		flow: one
		stages:
		  it: { job: { script: ./it.js, timeout: "${input.t}" } }
	`
	loops := map[string]string{"it.js": "for (;;) {}"}
	started := time.Now()
	wantError(t, run(t, l, job, loops, map[string]any{"t": "2s"}), "error", "time limit of 2s")
	if took := time.Since(started); took > 15*time.Second {
		t.Fatalf("a job with a timeout of 2s from input was stopped after %v", took)
	}
	r := run(t, l, job, loops, map[string]any{"t": "soon"})
	if r.Status != "failed" || !strings.Contains(r.Error, `stages.it.job.timeout: "soon" is not a length of time`) {
		t.Fatalf("a timeout of soon from input ended %s/%s: %s", r.Status, r.Failure, r.Error)
	}
	r = run(t, l, job, loops, map[string]any{"t": 5})
	if r.Status != "failed" || !strings.Contains(r.Error, "stages.it.job.timeout: ${input.t} is a number, not a length of time") {
		t.Fatalf("a timeout of 5 from input ended %s/%s: %s", r.Status, r.Failure, r.Error)
	}

	limited := `
		flow: one
		limits: { timeout: "${input.t}" }
		stages:
		  it: { job: { script: ./it.js, timeout: 20s } }
	`
	started = time.Now()
	r = run(t, l, limited, loops, map[string]any{"t": "1s"})
	if r.Status != "failed" || r.Failure != "timeout" || time.Since(started) > 15*time.Second {
		t.Fatalf("a run with a timeout of 1s from input ended %s/%s after %v: %s", r.Status, r.Failure, time.Since(started), r.Error)
	}
	r = run(t, l, limited, loops, map[string]any{"t": "soon"})
	if r.Status != "failed" || !strings.Contains(r.Error, `limits.timeout: "soon" is not a length of time`) {
		t.Fatalf("a run timeout of soon from input ended %s/%s: %s", r.Status, r.Failure, r.Error)
	}

	for _, c := range []struct{ def, holds string }{
		{strings.Replace(job, "${input.t}", "${nothing.here}", 1), `stages.it.job.timeout: in ${nothing.here}`},
		{strings.Replace(job, `"${input.t}"`, "soon", 1), `stages.it.job.timeout: "soon" is not a length of time`},
		{strings.Replace(limited, "${input.t}", "${stages.it.output}", 1), "limits.timeout: in ${stages.it.output}"},
		{strings.Replace(limited, `"${input.t}"`, "soon", 1), `limits.timeout: "soon" is not a length of time`},
	} {
		if _, err := machineParse(c.def, loops); err == nil || !strings.Contains(err.Error(), c.holds) {
			t.Errorf("want a refusal holding %q, got: %v", c.holds, err)
		}
	}
}

// A program using Corbie as a library lists a definition's jobs, each with
// its place. An agent's jobs include those of the tools for working with
// blobs that every agent has.
func TestJobsAreListedWithTheirPlaces(t *testing.T) {
	for _, c := range []struct {
		def  string
		want []string
	}{
		{`
			flow: two
			stages:
			  a: { job: ./add.js }
			  b: { job: { script: ./add.js, input: { n: "${stages.a.output}" } } }
		`, []string{"stages.a.job", "stages.b.job"}},
		{agentDef("http://127.0.0.1:1", ""), []string{"turn", "tools.add.job", "tools.blob_code.job", "tools.blob_read.job", "tools.blob_search.job"}},
	} {
		var places []string
		for _, job := range machine.Jobs(draft(t, c.def, scripts).Definition) {
			if job.Job == nil {
				t.Errorf("the job at %s is empty", job.Place)
			}
			places = append(places, job.Place)
		}
		if !slices.Equal(places, c.want) {
			t.Errorf("the jobs are at %v, not %v", places, c.want)
		}
	}
}

// A definition's input: is a shape. Its short form takes names it does not
// give, and a blob there is the blob itself; an input that does not fit
// fails the run, naming each place.
func TestInputIsAShape(t *testing.T) {
	l := local(t)
	echo := map[string]string{"echo.js": "return input"}
	def := `
		flow: one
		input: { n: number, report: "blob?" }
		stages:
		  it: { job: { script: ./echo.js, input: "${input}" } }
		output: ${stages.it.output}
	`
	if r := run(t, l, def, echo, map[string]any{"n": 1, "more": true}); r.Status != "done" {
		t.Fatalf("an input with a name its shape does not give ended %s: %s", r.Status, r.Error)
	}
	file := client.InputFile{Name: "report.txt", Contents: strings.NewReader("a report")}
	if r := run(t, l, def, echo, map[string]any{"n": 1, "report": file}); r.Status != "done" {
		t.Fatalf("an input with a file as its blob ended %s: %s", r.Status, r.Error)
	}
	r := run(t, l, def, echo, map[string]any{"n": "one", "report": "a report"})
	for _, holds := range []string{"at /n:", "at /report:"} {
		if r.Status != "failed" || !strings.Contains(r.Error, holds) {
			t.Errorf("an input that does not fit ended %s with %q; want an error holding %q", r.Status, r.Error, holds)
		}
	}
}

// A default is filled in only where properties and items lead from the top
// of the input's shape, and must fit its own place.
func TestMisplacedDefaultIsRefused(t *testing.T) {
	for _, c := range []struct{ input, holds string }{
		{`{ type: object, anyOf: [{ properties: { n: { default: 1 } } }] }`, "at #/anyOf/0/properties/n: a default is filled in only where"},
		{`{ type: object, properties: { n: { $ref: "#/$defs/n" } }, $defs: { n: { default: 1 } } }`, "at #/$defs/n: a default is filled in only where"},
		{`{ type: object, properties: { n: { type: number, default: two } } }`, "at #/properties/n: the default does not fit its own place"},
	} {
		def := "flow: one\ninput: " + c.input + "\nstages:\n  it: { job: ./echo.js }\n"
		if _, err := machineParse(def, map[string]string{"echo.js": "return input"}); err == nil || !strings.Contains(err.Error(), c.holds) {
			t.Errorf("want a refusal holding %q, got: %v", c.holds, err)
		}
	}
	good := "flow: one\ninput: { type: object, properties: { n: { type: number, default: 2 }, all: { type: array, items: { properties: { m: { default: [] } } } } } }\nstages:\n  it: { job: ./echo.js }\n"
	if _, err := machineParse(good, map[string]string{"echo.js": "return input"}); err != nil {
		t.Errorf("defaults under properties and items are refused: %v", err)
	}
}

// A definition of any type, and a machine written in place, may carry
// meta: with any value. It is kept with the stored definition as written,
// an expression in it is neither checked nor filled in, and the run goes
// on as if it were not there.
func TestMetaIsKeptAndNeverRead(t *testing.T) {
	const meta = `meta: { on: { at: "02:00 Europe/London" }, says: "${not an expression", n: [1, two] }`
	flow := `
		flow: backup-check
		` + meta + `
		stages:
		  check: { job: { script: ./add.js, input: { n: 1 } } }
		  inner:
		    job:
		      machine:
		        flow: inner
		        meta: "${input.anything}"
		        stages: { s: { job: { script: ./add.js, input: { n: 2 } } } }
		        output: ${stages.s.output}
		output: ${stages.check.output}
	`
	d := draft(t, flow, scripts)
	if kept := jsonOf(d.Definition.Meta); kept != `{"n":[1,"two"],"on":{"at":"02:00 Europe/London"},"says":"${not an expression"}` {
		t.Fatalf("the stored definition keeps meta as %s", kept)
	}
	for _, child := range d.Children {
		if child.Definition.Meta != "${input.anything}" {
			t.Fatalf("the machine written in place keeps meta as %v", child.Definition.Meta)
		}
	}
	r := run(t, local(t, "all"), flow, scripts, nil)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
	for _, def := range []string{
		"statechart: s\n" + meta + "\ninitial: a\nstates: { a: { end: done } }\n",
		"agent: a\n" + meta + "\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\n",
	} {
		d, err := machineParse(def, nil)
		if err != nil || d.Definition.Meta == nil {
			t.Errorf("a definition with meta: was refused or lost it: %v\n%s", err, def)
		}
	}
}

// A definition of any type, and a machine written in place, may give
// description:, plain text kept with the stored definition as written. An
// expression in it is refused.
func Test_mach_description_is_kept_as_written(t *testing.T) {
	const description = "description: Adds one to n, then tells nobody."
	flow := `
		flow: adder
		` + description + `
		stages:
		  inner:
		    job:
		      machine:
		        flow: inner
		        description: The machine written in place.
		        stages: { s: { job: { script: ./add.js, input: { n: 2 } } } }
		        output: ${stages.s.output}
		output: ${stages.inner.output}
	`
	d := draft(t, flow, scripts)
	if d.Definition.Description != "Adds one to n, then tells nobody." {
		t.Fatalf("the stored definition keeps the description as %q", d.Definition.Description)
	}
	for _, child := range d.Children {
		if child.Definition.Description != "The machine written in place." {
			t.Fatalf("the machine written in place keeps the description as %q", child.Definition.Description)
		}
	}
	r := run(t, local(t, "all"), flow, scripts, nil)
	check(t, r.Status, r.Output, "done", `{"n": 3}`)
	for _, def := range []string{
		"statechart: s\n" + description + "\ninitial: a\nstates: { a: { end: done } }\n",
		"agent: a\n" + description + "\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\n",
	} {
		d, err := machineParse(def, nil)
		if err != nil || d.Definition.Description != "Adds one to n, then tells nobody." {
			t.Errorf("a definition with description: was refused or lost it: %v\n%s", err, def)
		}
	}
	_, err := machineParse("flow: f\ndescription: Adds ${input.n}.\nstages:\n  a: { job: ./add.js }\n", scripts)
	if err == nil || !strings.Contains(err.Error(), "description: a description is plain text, with no expression") {
		t.Fatalf("a description with an expression was not refused at description: %v", err)
	}
}

// approve: at the top of a definition is a list of permissions, and in a job
// true or false. Each written in the other's place is refused, and the
// refusal tells the two apart.
func Test_mach_approve_is_a_list_on_a_definition_and_a_flag_on_a_job(t *testing.T) {
	for def, want := range map[string]string{
		"flow: f\nask: ./ask.js\napprove: true\nstages:\n  a: { job: ./add.js }\n":                              "approve: at the top of a definition, this is a list of the permissions a job may use only after a yes",
		"flow: f\nask: ./ask.js\nstages:\n  a: { job: { script: ./add.js, approve: [exec] } }\n":                "stages.a.job.approve: in a job, this is true or false",
		"flow: f\nask: ./ask.js\napprove: [exec]\nstages:\n  a: { job: { script: ./add.js, approve: true } }\n": "",
	} {
		_, err := machineParse(def, map[string]string{"add.js": scripts["add.js"], "ask.js": "return { approved: [] }"})
		switch {
		case want == "" && err != nil:
			t.Errorf("%s was refused: %v", def, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s was not refused with %q: %v", def, want, err)
		}
	}
}

// Every field named when, in every shape of every type, is a condition: one
// expression, written ${…}.
func Test_mach_every_when_is_a_condition(t *testing.T) {
	found := 0
	for _, name := range machine.Types() {
		d, _ := machine.Describe(name)
		for _, shape := range d.Shapes {
			for _, f := range shape.Fields {
				if f.Name != "when" {
					continue
				}
				found++
				if _, ok := machine.OneExpr(f.Like); f.Kind != machine.KindWord || !ok || !strings.HasPrefix(f.Does, "a condition, one expression") {
					t.Errorf("%s's %s has a when: that is not a condition: %+v", name, shape.Name, f)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no shape has a when:")
	}
}

// A definition of any type may give title:, filled in from the input and
// the run when the run starts, as one line of at most 200 characters. Only
// the client that started the run is shown it; a viewer, who reads the
// head, is not.
func TestTitleIsFilledInWhenTheRunStarts(t *testing.T) {
	l := local(t, "all")
	flow := `
		flow: release
		title: "${input.channel}: ${input.said}"
		stages:
		  check: { job: { script: ./add.js, input: { n: 1 } } }
		  inner:
		    job:
		      machine:
		        flow: inner
		        title: "inner ${input.n}"
		        stages: { s: { job: { script: ./add.js, input: { n: 2 } } } }
		      input: { n: 3 }
		output: ${stages.check.output}
	`
	r := run(t, l, flow, scripts, map[string]any{"channel": "#testing", "said": "give me\n\na release card\r\n\tfor\u2028 4.2 "})
	if r.Status != "done" || r.Title != "#testing: give me a release card for 4.2" {
		t.Fatalf("the run ended %s with the title %q", r.Status, r.Title)
	}
	children, err := l.List(ctx, client.Filter{Parent: r.ID, Finished: true})
	if err != nil || len(children) != 1 {
		t.Fatalf("the run's children are %v: %v", children, err)
	}
	if child, err := l.Show(ctx, children[0].ID); err != nil || child.Title != "inner 3" {
		t.Fatalf("the child's title is %q: %v", child.Title, err)
	}
	if children[0].Title != "" {
		t.Errorf("a listing, which reads heads, gives the title %q", children[0].Title)
	}
	viewer := client.New(nil, l.Client.Controller, l.Client.Store)
	if seen, err := viewer.Show(ctx, r.ID); err != nil || seen.Title != "" {
		t.Errorf("a viewer is shown the title %q: %v", seen.Title, err)
	}

	sender := `
		statechart: sender
		allow: ["send:keyed"]
		initial: a
		states:
		  a:
		    job: ./echo.js
		    on: { done: { go: z, send: { to: { at: keyed, key: keyed-1 }, event: hello, data: { x: "a\n\n b" } } } }
		  z: { end: done }
	`
	keyedSrc := "statechart: keyed\nlifetime: keyed\ntitle: \"keyed ${input.x}\"\ninitial: a\nstates: { a: { on: { hello: b } }, b: { end: done } }\n"
	files := map[string]string{"echo.js": scripts["echo.js"]}
	message, err := l.Submit(ctx, &client.Tree{Prefix: "/titles", Machines: []*client.Machine{
		{Path: "sender.yaml", Location: "/titles/sender", Draft: draft(t, sender, files)},
		{Path: "keyed.yaml", Location: "/titles/keyed", Draft: draft(t, keyedSrc, nil)}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := l.WaitReceipt(ctx, message, 0, 20*time.Millisecond); err != nil || !r.Taken {
		t.Fatalf("the tree's receipt is %+v: %v", r, err)
	}
	if _, err := l.Start(ctx, client.Start{At: "/titles/sender"}); err != nil {
		t.Fatal(err)
	}
	keyed := waitForFinished(t, l, "keyed")
	if got, err := l.Show(ctx, keyed); err != nil || got.Title != "keyed a b" || got.Location != "/titles/keyed" || got.Key != "keyed-1" {
		t.Errorf("a run a send started is shown as %+v: %v", got, err)
	}

	long := run(t, l, "flow: long\ntitle: \"${'x'.repeat(300)}é\"\nstages: { s: { job: ./echo.js } }\n", scripts, nil)
	if len([]rune(long.Title)) != 200 || long.Title != strings.Repeat("x", 200) {
		t.Errorf("a title of 301 characters is shown as %d: %q", len([]rune(long.Title)), long.Title)
	}
	for _, def := range []string{
		"statechart: s\ntitle: \"door ${input.n}\"\ninitial: a\nstates: { a: { end: done } }\n",
		"agent: a\ntitle: hello\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\n",
	} {
		if d, err := machineParse(def, nil); err != nil || d.Definition.Title == "" {
			t.Errorf("a definition with title: was refused or lost it: %v\n%s", err, def)
		}
	}
	if _, err := machineParse("flow: f\ntitle: \"${stages.s.output}\"\nstages: { s: { job: ./echo.js } }\n", scripts); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("a title that reads stages is not refused at its place: %v", err)
	}
	failed := run(t, l, "flow: f\ntitle: \"${input.x.y}\"\nstages: { s: { job: ./echo.js } }\n", scripts, map[string]any{})
	if failed.Status != "failed" || !strings.Contains(failed.Error, "title: ${input.x.y}") {
		t.Errorf("a title that cannot be filled in ended %s with %q", failed.Status, failed.Error)
	}
}

// growModel is the model of each agent the growing definitions write.
const growModel = "model: { name: test-model, service: anthropic, address: http://127.0.0.1:1, secret: K }"

// inPlaceAgent is a job running an agent written in place, with one more
// field of its own.
func inPlaceAgent(field string) string {
	return "{ machine: { agent: inner, " + growModel + ", prompt: hi, " + field + " } }"
}

// No file a definition reads leads back to itself: a machine file or an
// included file, with machines written in place between. Each is refused,
// naming the chain, at once.
func Test_mach_file_leading_back_to_itself_is_refused(t *testing.T) {
	agent := "agent: a\n" + growModel + "\nprompt: hi\n"
	for _, c := range []struct {
		name, def string
		files     map[string]string
		want      string
	}{
		{"a file that includes itself", agent + "include: ./t.yaml\n",
			map[string]string{"t.yaml": "include: ./t.yaml\n"},
			"include: the definition leads back to itself: t.yaml -> t.yaml"},
		{"two files that include each other", agent + "tools: { include: ./a.yaml }\n",
			map[string]string{"a.yaml": "include: ./b.yaml\n", "b.yaml": "include: ./a.yaml\n"},
			"tools.include: the definition leads back to itself: a.yaml -> b.yaml -> a.yaml"},
		{"a file included below itself", agent + "tools: { include: ./t.yaml }\n",
			map[string]string{"t.yaml": "t: { does: x, job: " + inPlaceAgent("tools: { include: ./t.yaml }") + " }\n"},
			"the definition leads back to itself: t.yaml -> t.yaml"},
		{"a part's file", agent + "open:\n  s: { include: ./s.yaml }\n",
			map[string]string{"s.yaml": "does: Loop.\ntools:\n  t: { does: x, job: " + inPlaceAgent("open: { s: { include: ./s.yaml } }") + " }\n"},
			"the definition leads back to itself: s.yaml -> s.yaml"},
		{"a machine file", "flow: f\nstages:\n  one: { job: { machine: ./f.yaml } }\n",
			map[string]string{"f.yaml": "flow: g\nstages:\n  one: { job: { machine: { flow: inner, stages: { two: { job: { machine: ./f.yaml } } } } } }\n"},
			"f.yaml: stages.two.job.machine: the definition leads back to itself: f.yaml -> f.yaml"},
		{"two machine files", "flow: f\nstages:\n  one: { job: { machine: ./a.yaml } }\n",
			map[string]string{"a.yaml": "flow: a\nstages:\n  one: { job: { machine: ./b.yaml } }\n", "b.yaml": "flow: b\nstages:\n  one: { job: { machine: ./a.yaml } }\n"},
			"b.yaml: stages.one.job.machine: the definition leads back to itself: a.yaml -> b.yaml -> a.yaml"},
		{"a type only files leading back could give", "include: ./a.yaml\n",
			map[string]string{"a.yaml": "include: ./b.yaml\n", "b.yaml": "include: ./a.yaml\n"},
			"b.yaml: include: the definition leads back to itself: a.yaml -> b.yaml -> a.yaml"},
		{"a type from a file that includes itself", "include: ./a.yaml\n",
			map[string]string{"a.yaml": "flow: a\nstages: { s: { job: ./s.js } }\ninclude: ./a.yaml\n", "s.js": "return 1"},
			"a.yaml: include: the definition leads back to itself: a.yaml -> a.yaml"},
		{"an included file and a machine file", agent + "open:\n  s: { include: ./s.yaml }\n",
			map[string]string{"s.yaml": "does: Loop.\ntools:\n  t: { does: x, job: { machine: ./b.yaml } }\n", "b.yaml": agent + "open:\n  s: { include: ./s.yaml }\n"},
			"the definition leads back to itself: s.yaml -> b.yaml -> s.yaml"},
	} {
		began := time.Now()
		_, err := machineParse(c.def, c.files)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want a refusal holding %q, got: %v", c.name, c.want, err)
		}
		if took := time.Since(began); took > time.Second {
			t.Errorf("%s: refusing it took %s", c.name, took)
		}
	}
}

// A machine a script defines that leads back to itself fails the job that
// defined it, saying so, and the runner goes on to run the next job.
func Test_mach_made_definition_leading_back_to_itself_fails_its_job(t *testing.T) {
	files := map[string]string{"make.js": `
		return machines.define("agent: a\n` + growModel + `\nprompt: hi\nopen:\n  s: { include: ./s.yaml }\n",
			{ files: { "./s.yaml": "does: Loop.\ntools:\n  t: { does: x, job: ` + inPlaceAgent("open: { s: { include: ./s.yaml } }") + ` }\n" } })
	`}
	l := local(t)
	r := run(t, l, "flow: maker\nstages:\n  make: { job: ./make.js }\n", files, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "make: ") || !strings.Contains(r.Error, "the definition leads back to itself: s.yaml -> s.yaml") {
		t.Fatalf("the job that defined a machine leading back to itself ended %s: %s", r.Status, r.Error)
	}
	r = run(t, l, "flow: next\nstages:\n  add: { job: { script: ./add.js, input: { n: 1 } } }\noutput: ${stages.add.output}\n", map[string]string{"add.js": scripts["add.js"]}, nil)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
}

// definingFails runs each script, which defines a machine, and checks that
// its job fails saying what is wrong, at once; then the same runner runs
// the next job to done.
func definingFails(t *testing.T, cases map[string]struct{ script, want string }) {
	t.Helper()
	l := local(t)
	for name, c := range cases {
		began := time.Now()
		r := run(t, l, "flow: maker\nstages:\n  make: { job: ./make.js }\n", map[string]string{"make.js": c.script}, nil)
		if r.Status != "failed" || !strings.Contains(r.Error, "make: ") || !strings.Contains(r.Error, c.want) {
			t.Errorf("%s: the job that defined it ended %s: %.500s", name, r.Status, r.Error)
		}
		if took := time.Since(began); took > 5*time.Second {
			t.Errorf("%s: the run took %s", name, took)
		}
	}
	r := run(t, l, "flow: next\nstages:\n  add: { job: { script: ./add.js, input: { n: 1 } } }\noutput: ${stages.add.output}\n", map[string]string{"add.js": scripts["add.js"]}, nil)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
}

// A machine a script defines that is past 4 MiB as it is stored fails the job
// that defined it, at once, and the runner goes on to run the next job. So
// does one a client sends, before anything is stored.
func Test_mach_made_definition_past_the_most_fails_its_job(t *testing.T) {
	definingFails(t, map[string]struct{ script, want string }{
		"30,000 stages each naming a script of its own": {`
			const stages = [], files = {}
			for (let i = 0; i < 30000; i++) {
				stages.push("  s" + i + ": { job: ./f" + i + ".js }\n")
				files["./f" + i + ".js"] = ""
			}
			return machines.define("flow: f\nstages:\n" + stages.join(""), { files })
		`, "the machine as it is stored is"},
		"a few lines of aliases": {`
			let text = "flow: f\nmeta:\n  a0: &a0 [ x ]\n"
			for (let i = 1; i < 30; i++) text += "  a" + i + ": &a" + i + " [" + ("*a" + (i - 1) + ",").repeat(9) + "]\n"
			return machines.define(text + "stages:\n  a: { job: ./add.js }\n", { files: { "./add.js": "return 1" } })
		`, "document contains excessive aliasing"},
	})
	big := "flow: f\nmeta: " + strconv.Quote(strings.Repeat("x", 5<<20)) + "\nstages:\n  a: { job: ./add.js }\n"
	if _, err := machineParse(big, map[string]string{"add.js": scripts["add.js"]}); err == nil || !strings.Contains(err.Error(), "and a machine may be at most 4 MiB") {
		t.Errorf("a client's definition past 4 MiB: %.300v", err)
	}
}

// aliasBomb is the fields of a mapping whose aliases make 135,795 values
// from 230 bytes. A file that adds meta: *b4 makes 258,017.
const aliasBomb = "b0: &b0 [1,1,1,1,1,1,1,1,1,1]\nb1: &b1 [*b0,*b0,*b0,*b0,*b0,*b0,*b0,*b0,*b0,*b0]\nb2: &b2 [*b1,*b1,*b1,*b1,*b1,*b1,*b1,*b1,*b1,*b1]\n" +
	"b3: &b3 [*b2,*b2,*b2,*b2,*b2,*b2,*b2,*b2,*b2,*b2]\nb4: &b4 [*b3,*b3,*b3,*b3,*b3,*b3,*b3,*b3,*b3,*b3]\n"

// A run reads its own ID in every expression of every type of machine, and
// a permission's scope stays filled from the input and from nothing else.
func Test_mach_run_is_read_in_every_type(t *testing.T) {
	l := local(t)
	echo := map[string]string{"echo.js": `return input`}
	for name, def := range map[string]string{
		"a flow's output": `
			flow: f
			stages:
			  a: { job: { script: ./echo.js, input: { id: "${run.id}" } } }
			output: { id: "${stages.a.output.id}", same: "${run.id}" }
		`,
		"a statechart's job and final state": `
			statechart: s
			initial: a
			states:
			  a: { job: { script: ./echo.js, input: { id: "${run.id}" } }, on: { done: b } }
			  b: { end: done, output: { id: "${states.a.output.id}", same: "${run.id}" } }
		`,
		"an agent's prompt": `
			agent: g
			model: { name: scripted, service: anthropic }
			turn: ./turn.js
			prompt: "${run.id}"
			reply: { id: string, same: string }
		`,
		"a timeout read from the run": `
			flow: f
			limits: { timeout: "${run.id ? '1m' : '1s'}" }
			stages:
			  a: { job: { script: ./echo.js, input: { id: "${run.id}" }, timeout: "${run.id ? '1m' : '1s'}" } }
			output: { id: "${stages.a.output.id}", same: "${run.id}" }
		`,
	} {
		files := map[string]string{"echo.js": echo["echo.js"], "turn.js": `
			const said = require("agents").conversation(input)
			const id = said[said.length - 1].content
			const answer = { role: "assistant", content: JSON.stringify({ id, same: id }), calls: [] }
			return { turn: blobs.make([answer], { type: "application/json" }), text: answer.content, calls: [], stop: "end", usage: { input: 1, output: 1 } }
		`}
		r := run(t, l, def, files, nil)
		out, _ := r.Output.(map[string]any)
		if reply, ok := out["reply"].(map[string]any); ok {
			out = reply
		}
		if r.Status != "done" || out["id"] != r.ID || out["same"] != r.ID {
			t.Fatalf("%s: the run %s ended %s and read %v: %s", name, r.ID, r.Status, r.Output, r.Error)
		}
	}
	// Each type reads the run's location and its tree where it reads its ID,
	// and they are the run's.
	for name, def := range map[string]string{
		"a flow's output": `
			flow: f
			stages:
			  a: { job: { script: ./turn.js } }
			output: "${run.location} ${run.tree}"
		`,
		"a statechart's final state": `
			statechart: s
			initial: b
			states:
			  b: { end: done, output: "${run.location} ${run.tree}" }
		`,
		"an agent's prompt": `
			agent: g
			model: { name: scripted, service: anthropic }
			turn: ./turn.js
			prompt: "${run.location} ${run.tree}"
		`,
		"a timeout": `
			flow: f
			limits: { timeout: "${run.location && run.tree ? '1m' : '1s'}" }
			stages:
			  a: { job: { script: ./turn.js, timeout: "${run.location && run.tree ? '1m' : '1s'}" } }
		`,
	} {
		if _, err := machineParse(def, map[string]string{"turn.js": "return {}"}); err != nil {
			t.Errorf("%s does not read run.location and run.tree: %v", name, err)
		}
	}
	located := machine.NewEvaluator(time.Now(), []byte("seed"), machine.Run{ID: "r1", Location: "/home/devices/boiler", Tree: "/home"})
	if got, err := located.Text("${run.location}", machine.Scope{"input": nil}, "output"); err != nil || got != "/home/devices/boiler" {
		t.Errorf("run.location gives %v (%v)", got, err)
	}
	if got, err := located.Text("${run.tree}", machine.Scope{"input": nil}, "output"); err != nil || got != "/home" {
		t.Errorf("run.tree gives %v (%v)", got, err)
	}
	_, err := machineParse(`
		flow: f
		stages:
		  a: { job: { script: ./echo.js, allow: ["fs.read:${run.id}/**"] } }
	`, echo)
	if err == nil || !strings.Contains(err.Error(), "from the machine's input and its values, and from nothing else") {
		t.Fatalf("a permission's scope read the run: %v", err)
	}
}

// A definition gives one type, written anywhere in it or given by a file it
// includes, and the field named for it gives the name.
func Test_mach_type_is_one_field_anywhere(t *testing.T) {
	post := map[string]string{"m.yaml": "flow: post\nstages: { s: { job: ./s.js } }\n", "s.js": "return 1", "b.yaml": "statechart: b\n"}
	for def, want := range map[string]string{
		"stages: { s: { job: ./s.js } }\nflow: late\n": "flow late",
		"include: ./m.yaml\n":                          "flow post",
		"flow: mine\ninclude: ./m.yaml\n":              "flow mine",
	} {
		d, err := machineParse(def, post)
		if err != nil {
			t.Fatalf("%q: %v", def, err)
		}
		if got := d.Definition.Type + " " + d.Definition.Name; got != want {
			t.Fatalf("%q is a %s", def, got)
		}
	}
	for def, want := range map[string]string{
		"stages: { s: { job: ./s.js } }\n": "it gives no type: a machine writes one of agent:, flow:, statechart: with its name, or takes it from a file it includes",
		"flow: a\ninclude: ./b.yaml\n":     "it gives more than one type, flow: a at line 1, statechart: b at b.yaml:1: a machine has one",
		"include: [./m.yaml, ./b.yaml]\n":  "it gives more than one type, flow: post at m.yaml:1, statechart: b at b.yaml:1: a machine has one",
	} {
		if _, err := machineParse(def, post); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want %q, got: %v", def, want, err)
		}
	}
}
