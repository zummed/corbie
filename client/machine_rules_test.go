package client_test

import (
	"os"
	"path/filepath"
	"slices"
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
		for _, want := range []string{c.place, `"` + c.entry + `"`, "from the machine's input and from nothing else"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s was refused without %q:\n%v", c.entry, want, err)
			}
		}
	}
	flowRefused(t, `
		flow: f
		input: [dir]
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
	id, err := l.Start(ctx, draft(t, `
		agent: reader
		input: [dir]
		model: { name: test-model, service: anthropic, address: `+m.URL+`, secret: TEST_KEY }
		prompt: Read the note.
		tools:
		  look:
		    does: Read a file of the directory
		    args: { name: string }
		    job: { script: ./look.js, input: { path: "${input.dir}/${args.name}" }, allow: ["fs.read:${input.dir}/**"] }
	`, map[string]string{"look.js": `return fs.read(input.path)`}), client.Start{
		Input: map[string]any{"dir": dir}, Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
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
		"ask.js": `return { approve: input.jobs.map(r => r.name) }`,
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
		input: [dir]
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
		input: [dir]
		allow: ["fs.read:${input.dir}/**", "fs.write:${input.dir}/**"]
		stages:
		  kid:
		    job:
		      input: { dir: "${input.dir}" }
		      machine:
		        flow: kid
		        input: [dir]
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
	id, startErr := l.Start(ctx, d, client.Start{})
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
