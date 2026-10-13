package client_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var scripts = map[string]string{
	"add.js":  `return { n: input.n + 1 }`,
	"echo.js": `return input`,
	"fail.js": `throw new Error("told to fail")`,
	"emit.js": `
		for (const x of input.items) emit("item", x)
		return { count: input.items.length }
	`,
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func check(t *testing.T, status string, output any, wantStatus, want string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("the run ended %s, want %s", status, wantStatus)
	}
	var w any
	json.Unmarshal([]byte(want), &w)
	if !reflect.DeepEqual(output, w) {
		t.Fatalf("the output is\n  %s\nwant\n  %s", jsonOf(output), want)
	}
}

func TestFlowOrderFollowsData(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: order
		input: [n]
		stages:
		  last:
		    job: { script: ./add.js, input: { n: "${stages.third.output.n}" } }
		  first:
		    job: { script: ./add.js, input: { n: "${input.n}" } }
		  third:
		    follows: [second]
		    job: { script: ./add.js, input: { n: "${stages['second'].output.n}" } }
		  second:
		    job: { script: ./add.js, input: { n: "${stages.first.output.n}" } }
		output: ${stages.last.output.n}
	`, scripts, map[string]any{"n": 0})
	check(t, r.Status, r.Output, "done", `4`)
}

func TestFlowSkipSpreads(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: skips
		input: [go]
		stages:
		  gate:
		    when: input.go
		    job: { script: ./add.js, input: { n: 1 } }
		  reads_gate:
		    job: { script: ./add.js, input: { n: "${stages.gate.output.n}" } }
		  after_gate:
		    follows: [gate]
		    job: { script: ./echo.js, input: { ran: true } }
		  reads_reader:
		    job: { script: ./add.js, input: { n: "${stages.reads_gate.output.n}" } }
		  notices_skip:
		    when: stages.gate.skipped
		    job: { script: ./echo.js, input: { saw_skip: "${stages.gate.skipped}" } }
		  independent:
		    job: { script: ./echo.js, input: { ran: true } }
		output:
		  gate: ${stages.gate.skipped}
		  reads_gate: ${stages.reads_gate.skipped}
		  after_gate: ${stages.after_gate.skipped}
		  reads_reader: ${stages.reads_reader.skipped}
		  notices_skip: ${stages.notices_skip.output}
		  independent: ${stages.independent.output}
	`, scripts, map[string]any{"go": false})
	check(t, r.Status, r.Output, "done", `{"gate":true,"reads_gate":true,"after_gate":true,"reads_reader":true,
		"notices_skip":{"saw_skip":true},"independent":{"ran":true}}`)
}

func TestFlowFailureHandledByAStageThatReadsIt(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: handled
		stages:
		  bad:
		    job: { script: ./fail.js }
		  cleanup:
		    when: stages.bad.failed
		    job: { script: ./echo.js, input: { error: "${stages.bad.error}" } }
		  needs_bad:
		    job: { script: ./echo.js, input: { got: "${stages.bad.output}" } }
		output:
		  cleanup: ${stages.cleanup.output}
		  needs_bad_skipped: ${stages.needs_bad.skipped}
	`, scripts, nil)
	check(t, r.Status, r.Output, "done", `{"cleanup":{"error":"Error: told to fail at line 1, column 7"},"needs_bad_skipped":true}`)
}

func TestFlowFailureNobodyReadsFailsTheFlow(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: unhandled
		stages:
		  bad:
		    job: { script: ./fail.js }
		  other:
		    job: { script: ./echo.js, input: { got: "${stages.bad.output}" } }
	`, scripts, nil)
	if r.Status != "failed" || r.Failure != "error" || !strings.HasPrefix(r.Error, "bad: Error: told to fail") {
		t.Fatalf("the flow ended %s/%s: %q", r.Status, r.Failure, r.Error)
	}
}

func TestFlowEach(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: each
		input: [numbers]
		stages:
		  bump:
		    each: ${input.numbers}
		    job: { script: ./maybe.js, input: { n: "${item}" } }
		  gather:
		    job: { script: ./echo.js, input: { all: "${stages.bump.output}" } }
		output: ${stages.gather.output.all}
	`, map[string]string{
		"maybe.js": `
			if (input.n === 2) throw new Error("two is refused")
			return input.n * 10
		`,
		"echo.js": scripts["echo.js"],
	}, map[string]any{"numbers": []any{1, 2, 3}})
	check(t, r.Status, r.Output, "done", `[{"item":1,"output":10},{"item":2,"error":"Error: two is refused at line 1, column 26"},{"item":3,"output":30}]`)
}

func TestFlowEachFollowsEmits(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: follows
		stages:
		  source:
		    job: { script: ./emit.js, input: { items: [{ n: 1 }, { n: 2 }, { n: 3 }] } }
		  per_item:
		    each: source
		    one_at_a_time: true
		    job: { script: ./add.js, input: { n: "${item.n}" } }
		output:
		  count: ${stages.source.output.count}
		  items: ${stages.per_item.output.map(r => r.output.n)}
	`, scripts, nil)
	check(t, r.Status, r.Output, "done", `{"count":3,"items":[2,3,4]}`)
}

func TestFlowEachOverABlob(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: lines
		stages:
		  make:
		    job: { script: ./make.js }
		  shout:
		    each: { blob: "${stages.make.output.text}" }
		    job: { script: ./upper.js, input: { line: "${item}" } }
		output: ${stages.shout.output.map(r => r.output)}
	`, map[string]string{
		"make.js":  `return { text: blobs.make("one\ntwo\nthree\n") }`,
		"upper.js": `return input.line.toUpperCase()`,
	}, nil)
	check(t, r.Status, r.Output, "done", `["ONE","TWO","THREE"]`)
}

func TestFlowRunsAChildWrittenInPlace(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: parent
		input: [numbers]
		stages:
		  kids:
		    each: ${input.numbers}
		    job:
		      input: { n: "${item}" }
		      machine:
		        flow: kid
		        input: [n]
		        stages:
		          one: { job: { script: ./add.js, input: { n: "${input.n}" } } }
		          two: { job: { script: ./add.js, input: { n: "${stages.one.output.n}" } } }
		        output: ${stages.two.output.n}
		output: ${stages.kids.output.map(r => r.output)}
	`, scripts, map[string]any{"numbers": []any{1, 10}})
	check(t, r.Status, r.Output, "done", `[3,12]`)
}

func TestFilesAndPermissions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in.txt"), []byte("hello\n"), 0o600)
	const def = `
		flow: files
		input: [dir]
		allow: ["fs.read:${input.dir}/**"]
		stages:
		  copy:
		    job:
		      script: ./copy.js
		      input: { dir: "${input.dir}" }
		      allow: ["fs.write:${input.dir}/out/**"]
		output: ${stages.copy.output}
	`
	files := map[string]string{"copy.js": `
		const text = fs.read(input.dir + "/in.txt")
		fs.write(input.dir + "/out/copy.txt", { text: text.toUpperCase() })
		fs.write("scratch.txt", { text: "x" })
		return fs.list(input.dir + "/out").map(e => e.name)
	`}
	// With nothing allowed by the ceiling, the read is refused, and the
	// refusal says what, which layer and the fix.
	l := local(t)
	r := run(t, l, def, files, map[string]any{"dir": dir})
	if r.Status != "failed" || r.Failure != "denied" || r.Refused != "fs.read:"+dir+"/in.txt" ||
		!strings.Contains(r.Layer, "ceiling of the client") || !strings.Contains(r.Layer, "ceiling of the runner") ||
		!strings.Contains(r.Fix, "corbie controller enrol client --allow fs.read:"+dir+"/in.txt") {
		t.Fatalf("with no ceiling: %s/%s\n refused: %s\n layer: %s\n fix: %s", r.Status, r.Failure, r.Refused, r.Layer, r.Fix)
	}
	// With disk allowed, the read and the first write pass; the write to
	// the scratch directory has no entry and is refused by the definition.
	l = local(t, "disk")
	r = run(t, l, def, files, map[string]any{"dir": dir})
	if r.Failure != "denied" || !strings.HasPrefix(r.Refused, "fs.write:") || !strings.Contains(r.Layer, "the job copy in the definition of files") {
		t.Fatalf("the scratch write: %s/%s refused %s by %s", r.Status, r.Failure, r.Refused, r.Layer)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "out", "copy.txt")); string(got) != "HELLO\n" {
		t.Fatalf("the copy holds %q", got)
	}
}

func flowRefused(t *testing.T, src string, files map[string]string, want string) {
	t.Helper()
	_, err := machineParse(src, files)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want a refusal holding %q, got: %v", want, err)
	}
}

func Test_flow_reference_to_missing_stage_is_refused(t *testing.T) {
	flowRefused(t, `
		flow: bad
		stages:
		  a: { job: { script: ./add.js, input: { n: "${stages.missing.output}" } } }
	`, scripts, `no stage named "missing"`)
	flowRefused(t, `
		flow: bad
		stages:
		  a: { job: { script: ./add.js, input: { n: 1 } } }
		output: ${stages["nowhere"].output}
	`, scripts, `no stage named "nowhere"`)
	// A flow with a cycle is refused, and the refusal names both stages.
	flowRefused(t, `
		flow: circle
		stages:
		  a: { job: { script: ./add.js, input: { n: "${stages.b.output.n}" } } }
		  b: { follows: [a], job: { script: ./add.js, input: { n: 1 } } }
	`, scripts, "a -> b -> a")
	flowRefused(t, `
		flow: norun
		stages:
		  a: { when: input.x }
	`, scripts, "a stage needs a job")
}

func Test_flow_output_counts_as_reader_of_item_failures(t *testing.T) {
	l := local(t)
	def := `
		flow: items
		stages:
		  each_one:
		    each: [1, 2]
		    job: { script: ./maybe.js, input: { n: "${item}" } }
		OUTPUT
	`
	files := map[string]string{"maybe.js": `if (input.n === 2) throw new Error("two"); return input.n`}
	// The flow's own output reads the list: an item's failure is in it.
	r := run(t, l, strings.Replace(def, "OUTPUT", "output: ${stages.each_one.output.length}", 1), files, nil)
	check(t, r.Status, r.Output, "done", `2`)
	// Nothing reads the list: the item's failure fails the flow.
	r = run(t, l, strings.Replace(def, "OUTPUT", "", 1), files, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "each_one item 1") {
		t.Fatalf("with nothing reading the list the flow ended %s: %s", r.Status, r.Error)
	}
}

func Test_flow_each_refuses_what_is_not_a_list(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: notalist
		input: [items]
		stages:
		  each_one:
		    each: ${input.items}
		    job: { script: ./add.js, input: { n: "${item}" } }
	`, scripts, map[string]any{"items": "just one text"})
	if r.Status != "failed" || !strings.Contains(r.Error, "each needs a list") {
		t.Fatalf("each over a text ended %s: %s", r.Status, r.Error)
	}
}

func Test_flow_following_stage_waits_for_other_inputs(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: waits
		stages:
		  source:
		    job: { script: ./emit.js, input: { items: [1, 2] } }
		  slow:
		    job: { script: ./slow.js }
		  per_item:
		    each: source
		    job: { script: ./echo.js, input: { item: "${item}", ready: "${stages.slow.output}" } }
		output: ${stages.per_item.output.map(r => r.output.ready)}
	`, map[string]string{"emit.js": scripts["emit.js"], "echo.js": scripts["echo.js"],
		"slow.js": `const until = Date.now() + 300; while (Date.now() < until) {}; return "ready"`}, nil)
	check(t, r.Status, r.Output, "done", `["ready", "ready"]`)
}
