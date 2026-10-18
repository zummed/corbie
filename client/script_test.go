package client_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/plugin"
)

// script runs one script as a job and gives the run.
func script(t *testing.T, l *client.Local, source string, extra string) *client.Run {
	t.Helper()
	return run(t, l, `
		flow: one
		stages:
		  it: { job: { script: ./it.js, files: [./lib.js], `+extra+` } }
		output: ${stages.it.output}
	`, map[string]string{"it.js": source, "lib.js": "exports.two = 2"}, nil)
}

func wantError(t *testing.T, r *client.Run, failure, holds string) {
	t.Helper()
	if r.Status != "failed" || r.Failure != failure || !strings.Contains(r.Error, holds) {
		t.Fatalf("the run ended %s/%s with %q; want %s with an error holding %q", r.Status, r.Failure, r.Error, failure, holds)
	}
}

func Test_run_script_cannot_close_its_function(t *testing.T) {
	l := local(t)
	r := script(t, l, "return 1 }); (function () { emit('outside', 1)", "timeout: 5s")
	wantError(t, r, "error", "not the body of one function")
}

func Test_run_error_position_matches_script_as_written(t *testing.T) {
	l := local(t)
	wantError(t, script(t, l, "throw new Error('first line')", "timeout: 5s"), "error", "first line at line 1, column 7")
	wantError(t, script(t, l, "const a = 1\n\n  throw new Error('third line')", "timeout: 5s"), "error", "third line at line 3, column 9")
	wantError(t, script(t, l, "const a = 1\nconst b = = 2", "timeout: 5s"), "error", "line 2")
}

func Test_run_require_reads_only_listed_files(t *testing.T) {
	l := local(t, "all")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "on-disk.js"), []byte("exports.x = 1"), 0o644)
	if r := script(t, l, `return require("./lib.js").two`, "timeout: 5s"); r.Output != 2.0 {
		t.Fatalf("a listed file: %v %s", r.Output, r.Error)
	}
	wantError(t, script(t, l, `return require("./other.js")`, "timeout: 5s"), "error", "not among the files the job lists")
	wantError(t, script(t, l, `return require("../`+filepath.Base(dir)+`/on-disk.js")`, "timeout: 5s"), "error", "not among the files the job lists")
	wantError(t, script(t, l, `return require("no-such-plugin")`, "timeout: 5s"), "error", `does not have the plugin "no-such-plugin"`)
}

func Test_run_plugin_cannot_take_a_scripts_own_name(t *testing.T) {
	for _, name := range []string{"input", "job", "emit", "log", "progress", "require"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("a module was registered under the name %s", name)
				}
			}()
			plugin.Register(&plugin.Module{Name: name})
		}()
	}
}

func Test_run_value_nested_too_deep_is_an_error(t *testing.T) {
	l := local(t)
	r := script(t, l, "let v = {}; let top = v; for (let i = 0; i < 5000; i++) { v.next = {}; v = v.next }\nreturn top", "timeout: 10s")
	wantError(t, r, "error", "nested too deep")
	// A value that holds itself is the same.
	wantError(t, script(t, l, "const a = {}; a.self = a; return a", "timeout: 10s"), "error", "nested too deep")
}

func Test_run_enormous_sparse_value_stopped_at_time_limit(t *testing.T) {
	l := local(t)
	started := time.Now()
	r := script(t, l, "const a = []; a.length = 4000000000; return a", "timeout: 2s")
	if r.Status != "failed" || time.Since(started) > 20*time.Second {
		t.Fatalf("an enormous sparse value: %s after %v (%s)", r.Status, time.Since(started), r.Error)
	}
}

func Test_run_time_limit_stops_script_stuck_in_a_call(t *testing.T) {
	l := local(t, "all")
	started := time.Now()
	// A program that outlasts the script's time limit, inside a call.
	r := script(t, l, `return exec.run("sleep", { args: ["30"], timeout: 60 })`, `timeout: 1s, allow: ["exec:sleep"]`)
	// The job ran out of time; the flow, which had no stage to handle that,
	// fails with an error that says so.
	wantError(t, r, "error", "time limit")
	if took := time.Since(started); took > 15*time.Second {
		t.Fatalf("a script stuck in a call was stopped only after %v", took)
	}
	// An endless loop is stopped too.
	wantError(t, script(t, l, "for (;;) {}", "timeout: 1s"), "error", "time limit")
}

// An output too large to send is sent in blobs: a long text in it as a
// blob of text, whose items are its lines, and a large list as one blob of
// JSON, whose items are its entries. A text of 64 KiB or less stays text.
func TestOutputTooLargeIsSentInBlobs(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: large
		stages:
		  text: { job: { script: ./text.js } }
		  list: { job: { script: ./list.js } }
		  long: { job: { script: ./texts.js, input: { size: 65537 } } }
		  edge: { job: { script: ./texts.js, input: { size: 65536 } } }
		  count: { job: { script: ./count.js, input: { text: "${stages.text.output.text}", list: "${stages.list.output}", edge: "${stages.edge.output}" } } }
		output: { title: "${stages.text.output.title}", text: "${stages.text.output.text}", list: "${stages.list.output}",
		          long: "${stages.long.output}", edge: "${stages.edge.output}", count: "${stages.count.output}" }
	`, map[string]string{
		"text.js":  `return { title: "t", text: "a line\n".repeat(60000) }`,
		"list.js":  `return Array.from({ length: 50000 }, (_, n) => ({ n }))`,
		"texts.js": `return Array.from({ length: 5 }, (_, n) => String(n).repeat(input.size))`,
		"count.js": `
			let lines = 0, entries = 0
			for (const _ of blobs.items(input.text)) lines++
			for (const _ of blobs.items(input.list)) entries++
			const edge = Array.from(blobs.items(input.edge))
			return { lines, entries, edge: edge.map((text) => text.length + ":" + text[0]) }
		`,
	}, nil)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	text, _ := out["text"].(map[string]any)
	list, _ := out["list"].(map[string]any)
	edge, _ := out["edge"].(map[string]any)
	if out["title"] != "t" || text["type"] != "text/plain" || text["size"] != 420000.0 || text["key"] != nil ||
		list["type"] != "application/json" || edge["type"] != "application/json" {
		t.Fatalf("the output was %v", jsonOf(out))
	}
	for i, each := range out["long"].([]any) {
		if blob, _ := each.(map[string]any); blob["type"] != "text/plain" || blob["size"] != 65537.0 {
			t.Errorf("the long text %d was %v", i, blob)
		}
	}
	// A map of many small values has no long text or list, and is too
	// large to send.
	wantError(t, script(t, l, `return Object.fromEntries(Array.from({ length: 50000 }, (_, n) => ["k" + n, n]))`, "timeout: 10s"),
		"error", "too large to send even with its long texts and lists in blobs")
	count := out["count"].(map[string]any)
	if count["lines"] != 60000.0 || count["entries"] != 50000.0 ||
		jsonOf(count["edge"]) != `["65536:0","65536:1","65536:2","65536:3","65536:4"]` {
		t.Fatalf("the blobs had %v", jsonOf(count))
	}
}

// A long text, and a long list, holding a secret's value are each stored
// as a blob with the secret's name in its place, as they are when sent.
func TestOutputInABlobIsMasked(t *testing.T) {
	const value = "s3cret-value-of-k"
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(r.Header.Get("X-K"))) }))
	t.Cleanup(echo.Close)
	host := strings.TrimPrefix(echo.URL, "http://")
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, `
		flow: masked
		stages:
		  echo: { job: { script: ./echo.js, input: { url: "`+echo.URL+`" }, allow: ["net:`+host+`", "secret:K@net.unencrypted:`+host+`"] } }
		  read: { job: { script: ./read.js, input: { text: "${stages.echo.output.text}", rows: "${stages.echo.output.rows}" } } }
		output: ${stages.read.output}
	`, map[string]string{
		"echo.js": `
			const said = http.get(input.url, { headers: { "X-K": secrets.get("K") } }).body
			return { text: "x".repeat(300000) + said, rows: Array.from({ length: 5000 }, (_, n) => ({ n, said })) }
		`,
		"read.js": `
			const text = blobs.read(input.text, { as: "text" })
			const rows = blobs.read(input.rows, { as: "text" })
			return { value: text.includes("` + value + `") || rows.includes("` + value + `"), tail: text.slice(-12), size: text.length, row: JSON.parse(rows)[7] }
		`,
	}), client.Start{Secrets: map[string]string{"K": value}})
	if err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	if r.Status != "done" || jsonOf(r.Output) != `{"row":{"n":7,"said":"[secret K]"},"size":300010,"tail":"xx[secret K]","value":false}` {
		t.Fatalf("the run ended %s (%s) with %v", r.Status, r.Error, jsonOf(r.Output))
	}
}

func Test_run_large_output_holding_blobs_is_not_put_in_a_blob(t *testing.T) {
	l := local(t)
	r := script(t, l, `return Array.from({ length: 2000 }, (_, n) => blobs.make("blob " + n))`, "timeout: 30s")
	wantError(t, r, "error", "too large to send even with its long texts and lists in blobs; a list that holds blobs stays a list")
	// A long text beside a blob is still stored as a blob of its own.
	r = script(t, l, `return { file: blobs.make("a file"), text: "x".repeat(300000) }`, "timeout: 10s")
	if out, _ := r.Output.(map[string]any); r.Status != "done" || out["text"].(map[string]any)["type"] != "text/plain" {
		t.Fatalf("a long text beside a blob ended %s (%s): %v", r.Status, r.Error, jsonOf(r.Output))
	}
}

func Test_run_script_cannot_swap_what_corbie_uses_of_the_language(t *testing.T) {
	l := local(t)
	r := script(t, l, `
		JSON.stringify = () => '"swapped"'
		JSON.parse = () => "swapped"
		Object.keys = () => []
		Array.prototype.map = () => ["swapped"]
		return { real: [1, 2], text: "kept" }
	`, "timeout: 5s")
	check(t, r.Status, r.Output, "done", `{"real": [1, 2], "text": "kept"}`)
}

func Test_run_plugin_values_show_only_listed_calls(t *testing.T) {
	l := local(t)
	r := script(t, l, `
		const b = blobs.make("some text")
		return { keys: Object.keys(b).sort(), json: JSON.stringify(b), symbols: Object.getOwnPropertySymbols(b).length,
		         inside: Object.getOwnPropertySymbols(b).map(s => Object.keys(b[s] || {}).length) }
	`, "timeout: 5s")
	check(t, r.Status, r.Output, "done", `{"keys": ["size", "type"], "json": "{\"type\":\"text/plain\",\"size\":9}", "symbols": 0, "inside": []}`)
}

func Test_run_emit_entries_numbered_from_one_without_gaps(t *testing.T) {
	l := local(t)
	id, err := l.Start(ctx, draft(t, `
		flow: emits
		stages:
		  it: { job: { script: ./it.js } }
	`, map[string]string{"it.js": `
		for (let i = 0; i < 100; i++) emit("n", i)
		log("done emitting")
		progress(1, 1)
		return 1
	`}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	var values []float64
	var names []string
	done, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		if e.Step == 0 {
			names = append(names, e.Name)
			if n, ok := e.Data.(float64); ok && e.Name == "n" {
				values = append(values, n)
			}
		}
	})
	if err != nil || done.Status != "done" {
		t.Fatalf("following: %v %+v", err, done)
	}
	// Every emit was shown, in the order the job made them.
	if len(values) != 100 || len(names) != 102 || names[100] != "log" || names[101] != "progress" {
		t.Fatalf("%d values and %d emits were shown", len(values), len(names))
	}
	for i, n := range values {
		if n != float64(i) {
			t.Fatalf("value %d is %v", i, n)
		}
	}
	// Written a few at a time, numbered from one with none left out.
	entries, _ := l.Memory.As(ownerOf()).List(ctx, "controller/records/"+id+"/emits")
	if len(entries) < 2 || len(entries) > 20 {
		t.Fatalf("102 emits were written as %d entries", len(entries))
	}
	for i, name := range entries {
		if !strings.HasSuffix(name, "/it/"+pad8(i+1)) {
			t.Fatalf("entry %d is named %s", i+1, name)
		}
	}
}

func Test_run_stray_print_cannot_corrupt_result(t *testing.T) {
	l := local(t, "all")
	// A program the job starts prints to the job's own output and error.
	r := script(t, l, `
		exec.run("sh", { args: ["-c", "echo not-a-result; echo also-not >&2"] })
		return "the real result"
	`, `timeout: 10s, allow: ["exec:sh"]`)
	if r.Status != "done" || r.Output != "the real result" {
		t.Fatalf("the run ended %s with %v (%s)", r.Status, r.Output, r.Error)
	}
}

func TestScratchDirectoryIsMadeAndRemoved(t *testing.T) {
	l := local(t, "all")
	r := script(t, l, `
		fs.write("notes/a.txt", { text: "kept for the job" })
		return { read: fs.read("notes/a.txt"), where: exec.run("pwd").out.trim() }
	`, `timeout: 10s, allow: ["fs.read:**", "fs.write:**", "exec:pwd"]`)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	where, _ := out["where"].(string)
	if out["read"] != "kept for the job" || !filepath.IsAbs(where) {
		t.Fatalf("the scratch directory: %v", out)
	}
	if _, err := os.Stat(where); !os.IsNotExist(err) {
		t.Fatalf("the scratch directory %s is still there after the job", where)
	}
}

func TestAJobStartsWithNothingOfTheRunners(t *testing.T) {
	t.Setenv("CORBIE_TEST_MARKER", "the-runners-own")
	l := local(t, "all")
	r := script(t, l, `
		return { env: exec.run("env").out, allowed: env.get("CORBIE_TEST_MARKER"), missing: env.get("NO_SUCH_VARIABLE", { otherwise: "none" }) }
	`, `timeout: 10s, allow: ["exec:env", "env:*"]`)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	if strings.Contains(out["env"].(string), "the-runners-own") {
		t.Fatalf("a started program was given the runner's variables: %s", out["env"])
	}
	// A variable the job is allowed is read through the way out for it.
	if out["allowed"] != "the-runners-own" || out["missing"] != "none" {
		t.Fatalf("env.get gave %v and %v", out["allowed"], out["missing"])
	}
}
