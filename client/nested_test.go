package client_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// years makes a directory holding 2025/a and 2026/a.
func years(t *testing.T) string {
	dir := t.TempDir()
	for _, name := range []string{"2025", "2026"} {
		os.MkdirAll(filepath.Join(dir, name), 0o700)
		os.WriteFile(filepath.Join(dir, name, "a"), []byte(name), 0o600)
	}
	return dir
}

// A child the definition names runs with what its own definition allows,
// unless the job that starts it carries a list.
func TestParentNarrowsOnlyWithAList(t *testing.T) {
	dir := years(t)
	files := map[string]string{"read.js": `return fs.read(input.path)`}
	const named = `
		flow: parent
		input: { dir: any, year: any }
		ABOVE
		stages:
		  kid:
		    job:
		      LIST
		      input: { dir: "${input.dir}", year: "${input.year}" }
		      machine:
		        flow: child
		        input: { dir: any, year: any }
		        allow: ["fs.read:${input.dir}/**"]
		        stages:
		          read: { job: { script: ./read.js, input: { path: "${input.dir}/${input.year}/a" } } }
		        output: ${stages.read.output}
		output: ${stages.kid.output}
	`
	with := func(above, list string) string {
		return strings.NewReplacer("ABOVE", above, "LIST", list).Replace(named)
	}
	l := local(t, "disk", "env")

	// With no list on the job, the child has what its own definition says.
	r := run(t, l, with("", ""), files, map[string]any{"dir": dir, "year": "2025"})
	check(t, r.Status, r.Output, "done", `"2025"`)

	// The lists written for the whole of the parent are for its own
	// scripts, and do not narrow the child.
	r = run(t, l, with(`allow: ["env:HOME"]`, ""), files, map[string]any{"dir": dir, "year": "2025"})
	check(t, r.Status, r.Output, "done", `"2025"`)

	// With a list, the child is held to it, and the refusal names the job.
	list := `allow: ["fs.read:${input.dir}/2026/**"]`
	r = run(t, l, with("", list), files, map[string]any{"dir": dir, "year": "2026"})
	check(t, r.Status, r.Output, "done", `"2026"`)
	r = run(t, l, with("", list), files, map[string]any{"dir": dir, "year": "2025"})
	if r.Failure != "denied" || r.Refused != "fs.read:"+dir+"/2025/a" || !strings.Contains(r.Layer, "the job \"kid\" in the definition of \"parent\"") {
		t.Fatalf("outside the job's list: %s/%s refused %s by %s", r.Status, r.Failure, r.Refused, r.Layer)
	}
}

// A machine a script made is allowed nothing by a job with no list,
// whatever its own definition says, and what the job lists otherwise.
func Test_perm_made_machine_gets_only_its_jobs_list(t *testing.T) {
	dir := years(t)
	files := map[string]string{"make.js": `
		return machines.define(
			"flow: made\ninput: { dir: any }\nallow: ['fs.read:" + input.dir + "/**']\nstages:\n" +
			"  read: { job: { script: ./read.js, input: { path: '" + input.dir + "/2026/a' } } }\n" +
			"output: ${stages.read.output}\n",
			{ files: { "./read.js": "return fs.read(input.path)" } })
	`}
	l := local(t, "disk")
	const made = `
		flow: maker
		input: { dir: any }
		stages:
		  make: { job: { script: ./make.js, input: { dir: "${input.dir}" } } }
		  go:
		    job:
		      LIST
		      machine: ${stages.make.output}
		      input: { dir: "${input.dir}" }
		output: ${stages.go.output}
	`
	r := run(t, l, strings.Replace(made, "LIST", "", 1), files, map[string]any{"dir": dir})
	if r.Failure != "denied" || r.Refused != "fs.read:"+dir+"/2026/a" || !strings.Contains(r.Layer, "the job \"go\" in the definition of \"maker\"") {
		t.Fatalf("a made machine with no list: %s/%s refused %s by %s: %s", r.Status, r.Failure, r.Refused, r.Layer, r.Error)
	}
	r = run(t, l, strings.Replace(made, "LIST", `allow: ["fs.read:${input.dir}/**"]`, 1), files, map[string]any{"dir": dir})
	check(t, r.Status, r.Output, "done", `"2026"`)
}

// A script can store a definition with no machines.define at all, as a blob
// of a definition's type, and a job's process could store anything. Nothing
// but the controller's check stands between it and a run: it is checked
// when it is started, and narrowed as any machine made while running is.
func Test_perm_definition_stored_as_a_blob_is_checked_and_narrowed(t *testing.T) {
	dir := years(t)
	files := map[string]string{"make.js": `
		const stored = machines.define(
			"flow: made\nstages:\n  read: { job: { script: ./read.js, input: { path: '" + input.dir + "/2026/a' } } }\n" +
			"output: ${stages.read.output}\n",
			{ files: { "./read.js": "return fs.read(input.path)" } })
		const def = JSON.parse(blobs.read(stored))
		def.allow = ["all"]
		if (input.broken) def.timeout = "${nothing.here}"
		return blobs.make(JSON.stringify(def), { type: "application/x-corbie-definition" })
	`}
	const maker = `
		flow: maker
		input: { dir: any, broken: any }
		stages:
		  make: { job: { script: ./make.js, input: { dir: "${input.dir}", broken: "${input.broken}" } } }
		  go: { job: { machine: "${stages.make.output}" } }
		output: ${stages.go.output}
	`
	l := local(t, "disk")
	r := run(t, l, maker, files, map[string]any{"dir": dir, "broken": false})
	if r.Failure != "denied" || r.Refused != "fs.read:"+dir+"/2026/a" || !strings.Contains(r.Layer, "the job \"go\" in the definition of \"maker\"") {
		t.Fatalf("a definition that allows all, stored as a blob: %s/%s refused %s by %s: %s", r.Status, r.Failure, r.Refused, r.Layer, r.Error)
	}
	r = run(t, l, maker, files, map[string]any{"dir": dir, "broken": true})
	if r.Status != "failed" || !strings.Contains(r.Error, "the definition did not pass its check") || !strings.Contains(r.Error, "nothing") {
		t.Fatalf("a broken definition stored as a blob: %s/%s: %s", r.Status, r.Failure, r.Error)
	}
}
