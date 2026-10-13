package client_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
)

// blobsOf counts the blobs in a run's record.
func blobsOf(t *testing.T, l *client.Local, r *client.Run) int {
	t.Helper()
	names, err := l.Memory.As(ownerOf()).List(ctx, "controller/records/"+r.ID+"/blobs")
	if err != nil {
		t.Fatal(err)
	}
	return len(names)
}

func Test_run_source_map_named_in_script_is_never_read(t *testing.T) {
	l := local(t, "all")
	// A map that says every place in a script is line 50 of another file.
	named := filepath.Join(t.TempDir(), "there.map")
	if err := os.WriteFile(named, []byte(`{"version":3,"file":"script","sources":["elsewhere.js"],"names":[],"mappings":"AAiDA,gxTAAA;AAAA,gxTAAA;AAAA,gxTAAA;AAAA,gxTAAA"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	naming := "\n//# sourceMappingURL=file://" + named
	// Named in the script, in a file beside it, and in code the script
	// makes as it runs, which starts two lines down: an error's place is
	// where it was written each time.
	for name, at := range map[string]struct{ source, place string }{
		"in the script":    {`throw new Error("as written")` + naming, "line 1,"},
		"in a file":        {`require("./lib.js").fail()`, "line 1,"},
		"in code it makes": {`new Function("throw new Error('as written')` + strings.ReplaceAll(naming, "\n", `\n`) + `")()`, "line 3,"},
	} {
		r := run(t, l, `
			flow: one
			stages:
			  it: { job: { script: ./it.js, files: [./lib.js], timeout: 5s } }
		`, map[string]string{"it.js": at.source, "lib.js": `exports.fail = function () { throw new Error("as written") }` + naming}, nil)
		if r.Status != "failed" || !strings.Contains(r.Error, "as written at "+at.place) {
			t.Errorf("a map named %s: the run ended %s with %q", name, r.Status, r.Error)
		}
	}
}

func Test_run_file_made_a_blob_is_read_in_place(t *testing.T) {
	l := local(t, "all")
	lies := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(lies, []byte(strings.Repeat("0123456789", 100000)), 0o644); err != nil {
		t.Fatal(err)
	}
	none := script(t, l, `return 1`, `timeout: 10s, allow: ["all"]`)
	// A megabyte made a blob, read in part, and written out to another
	// file: none of it goes to the store.
	r := run(t, l, `
		flow: one
		input: [lies]
		stages:
		  it: { job: { script: ./it.js, files: [./lib.js], input: { lies: "${input.lies}" }, timeout: 10s, allow: ["all"] } }
		output: ${stages.it.output}
	`, map[string]string{"lib.js": "exports.two = 2", "it.js": `
		const made = blobs.file(input.lies)
		fs.write("copy.txt", { blob: made })
		return { type: made.type, size: made.size, part: blobs.read(made, { from: 999995, length: 5 }), copy: fs.stat("copy.txt").size }
	`}, map[string]any{"lies": lies})
	check(t, r.Status, r.Output, "done", `{"type": "text/plain", "size": 1000000, "part": "56789", "copy": 1000000}`)
	if got, want := blobsOf(t, l, r), blobsOf(t, l, none); got != want {
		t.Fatalf("the record holds %d blobs, and that of a run that made none holds %d", got, want)
	}
}

func Test_blob_used_only_inside_a_job_never_reaches_the_store(t *testing.T) {
	l := local(t, "all")
	const made = `const made = blobs.make("x".repeat(100000))` + "\n"
	count := func(source string) int {
		r := script(t, l, source, `timeout: 10s, allow: ["all"]`)
		if r.Status != "done" {
			t.Fatalf("the run ended %s: %s", r.Status, r.Error)
		}
		return blobsOf(t, l, r)
	}
	none := count(`return 100000`)
	// Read, measured and written to a file inside the job.
	if got := count(made + `fs.write("out.txt", { blob: made }); return blobs.read(made).length + fs.stat("out.txt").size`); got != none {
		t.Errorf("a blob used only inside its job: the record holds %d blobs, and %d without it", got, none)
	}
	// It is sent when its reference leaves, in an emit or in the result,
	// and once if it leaves twice.
	for name, source := range map[string]string{
		"in an emit":    made + `emit("made", made); return 1`,
		"in the result": made + `return { made }`,
		"in both":       made + `emit("made", made); return { made }`,
	} {
		if got := count(source); got != none+1 {
			t.Errorf("a blob that leaves %s: the record holds %d blobs, and %d without it", name, got, none)
		}
	}
}

func Test_run_file_changed_after_opening_fails_the_read(t *testing.T) {
	l := local(t, "all")
	r := script(t, l, `
		const checked = "the file that was checked"
		const open = function (name) { fs.write(name, { text: checked }); return blobs.file(name) }
		const read = function (blob) { try { return blobs.read(blob) } catch (e) { return "refused: " + e.message } }

		const cut = open("cut.txt")
		fs.write("cut.txt", { text: "now shorter" })

		const grown = open("grown.txt")
		fs.write("grown.txt", { text: ", and more", append: true })

		const replaced = open("replaced.txt")
		fs.write("another.txt", { text: "another file put in place" })
		fs.move("another.txt", { to: "replaced.txt" })

		const linked = open("linked.txt")
		fs.write("elsewhere.txt", { text: "what a link would lead to" })
		fs.remove("linked.txt")
		exec.run("ln", { args: ["-s", "elsewhere.txt", "linked.txt"] })

		return { cut: read(cut), grown: read(grown), replaced: read(replaced), linked: read(linked),
		         now: [fs.read("replaced.txt"), fs.read("linked.txt")] }
	`, `timeout: 10s, allow: ["all"]`)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	// Cut short, the read fails; grown, it is read to the size it had.
	if cut, _ := out["cut"].(string); !strings.HasPrefix(cut, "refused: ") || !strings.Contains(cut, "changed after it was opened") {
		t.Errorf("a file cut short after it was opened was read as %q", out["cut"])
	}
	if out["grown"] != "the file that was checked" {
		t.Errorf("a file that grew after it was opened was read as %q", out["grown"])
	}
	// With another file or a link in its place, what is read is never the
	// other file: it is the file that was checked, or nothing.
	check(t, r.Status, out["now"], "done", `["another file put in place", "what a link would lead to"]`)
	for _, name := range []string{"replaced", "linked"} {
		if got, _ := out[name].(string); got != "the file that was checked" && !strings.HasPrefix(got, "refused: ") {
			t.Errorf("a file %s after it was opened was read as %q", name, got)
		}
	}
}

func Test_run_opened_file_held_until_job_ends(t *testing.T) {
	l := local(t, "all")
	// The file is removed as soon as it is made a blob, and others are
	// made after it. The blob is read then, and again when it leaves the
	// job in the result.
	r := script(t, l, `
		fs.write("checked.txt", { text: "the file that was checked" })
		const made = blobs.file("checked.txt")
		fs.remove("checked.txt")
		for (let i = 0; i < 20; i++) fs.write("next-" + i + ".txt", { text: "a file made after it was gone" })
		return { there: fs.exists("checked.txt"), read: blobs.read(made), made }
	`, `timeout: 10s, allow: ["all"]`)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	if out["there"] != false || out["read"] != "the file that was checked" {
		t.Fatalf("with the file gone, the blob was read as %q (the file is there: %v)", out["read"], out["there"])
	}
	contents, _, err := l.Fetch(ctx, r.ID, "made")
	if err != nil {
		t.Fatal(err)
	}
	if left, _ := io.ReadAll(contents); string(left) != "the file that was checked" {
		t.Fatalf("what left the job is %q", left)
	}
}

func Test_run_stopping_one_program_leaves_the_others(t *testing.T) {
	l := local(t, "all")
	// One program goes on by itself; a second is stopped at the time its
	// call gave it. The first is still there, and the script goes on.
	r := script(t, l, `
		const other = exec.run("sh", { args: ["-c", "sleep 600 >/dev/null 2>&1 & echo $!"] }).out.trim()
		let stopped = "it ended by itself"
		try { exec.run("sleep", { args: ["600"], timeout: 0.2 }) } catch (e) { stopped = e.code }
		return { stopped, other: exec.run("sh", { args: ["-c", "kill -0 " + other + " && echo is still there"] }).out.trim() }
	`, `timeout: 20s, allow: ["all"]`)
	check(t, r.Status, r.Output, "done", `{"stopped": "timeout", "other": "is still there"}`)
}

func Test_run_program_that_cannot_start_is_in_the_calls_answer(t *testing.T) {
	l := local(t, "all")
	r := script(t, l, `
		const tried = function (program, options) {
		  try { return { status: exec.run(program, options).status } } catch (e) { return { code: e.code } }
		}
		fs.write("not-a-program", { text: "nothing a system can start", mode: 0o755 })
		return {
		  missing: tried("no-such-program-anywhere"),
		  unstartable: tried("./not-a-program"),
		  bad: tried("sh", { args: ["-c", "exit 3"] }),
		  good: tried("sh", { args: ["-c", "exit 0"] }),
		  slow: tried("sleep", { args: ["600"], timeout: 0.2 }),
		}
	`, `timeout: 20s, allow: ["all"]`)
	check(t, r.Status, r.Output, "done", `{"missing": {"code": "not_found"}, "unstartable": {"code": "not_found"},
		"bad": {"status": 3}, "good": {"status": 0}, "slow": {"code": "timeout"}}`)
}
