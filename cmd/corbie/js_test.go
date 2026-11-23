package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The command builds for js/wasm, and there corbie up runs a controller, a
// runner and a gateway in the one process: a job runs beside its runner,
// the gateway listens on
// no port and answers the code that started the program. A job with a
// memory: runs, and a call the gateway cannot read leaves the program up.
// Node stands in for a browser.
func Test_role_js_wasm_runs_a_job_and_answers_its_hosts_code(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH, and it runs the build for js/wasm here")
	}
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	program := filepath.Join(dir, "corbie.wasm")
	build := exec.Command("go", "build", "-tags", "noui", "-o", program, ".")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the command does not build for js/wasm: %v\n%s", err, out)
	}
	work := filepath.Join(dir, "work")
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(work, "double.yaml"), "flow: double\nstages:\n  twice: { job: { script: ./double.js, memory: 64MB, input: \"${input}\" } }\noutput: ${stages.twice.output}\n")
	write(filepath.Join(work, "double.js"), "return { doubled: input.n * 2 }\n")
	// A machine made while running: the controller takes its steps apart from itself.
	write(filepath.Join(work, "made.yaml"), "flow: made\nstages:\n  make: { job: { script: ./make.js } }\n  run: { job: { machine: \"${stages.make.output}\" } }\noutput: ${stages.run.output}\n")
	write(filepath.Join(work, "make.js"), "return machines.define(\"statechart: inner\\ninitial: a\\nstates:\\n  a: { end: done, output: made }\\n\")\n")
	token := filepath.Join(dir, "token")
	write(token, "token-of-the-test\n")

	run := exec.Command(node, "testdata/js_wasm.cjs", filepath.Join(strings.TrimSpace(string(root)), "lib", "wasm", "wasm_exec.js"), program, work, filepath.Join(dir, "config"), token)
	var failed strings.Builder
	run.Stderr = &failed
	out, err := run.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, failed.String())
	}
	var got struct {
		Said   string
		Short  string
		Stayed bool
		Who    struct {
			Status int
			Body   string
		}
		Started struct {
			Status int
			Body   string
		}
		Run struct {
			Status int
			Body   struct {
				Status string
				Error  string
				Output struct{ Doubled int }
			}
		}
		Made struct {
			Status int
			Body   struct {
				Status string
				Error  string
				Output string
			}
		}
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(got.Said, "answering the requests of the code that started it") {
		t.Errorf("the gateway did not say that it listens on no port:\n%s", got.Said)
	}
	if !strings.Contains(got.Short, "corbieGateway.fetch takes") {
		t.Errorf("a call with three arguments gave %q", got.Short)
	}
	if !got.Stayed {
		t.Error("the function that stops a request did something after the answer was whole")
	}
	if got.Who.Status != 200 || !strings.Contains(got.Who.Body, `"client"`) {
		t.Errorf("GET /client was answered %d %s", got.Who.Status, got.Who.Body)
	}
	if got.Run.Status != 200 || got.Run.Body.Status != "done" || got.Run.Body.Output.Doubled != 42 {
		t.Errorf("the run was answered %d: status %q, error %q, output %+v; the start was answered %d %s", got.Run.Status, got.Run.Body.Status, got.Run.Body.Error, got.Run.Body.Output, got.Started.Status, got.Started.Body)
	}
	if got.Made.Status != 200 || got.Made.Body.Status != "done" || got.Made.Body.Output != "made" {
		t.Errorf("the run of a machine made while running was answered %d: status %q, error %q, output %q", got.Made.Status, got.Made.Body.Status, got.Made.Body.Error, got.Made.Body.Output)
	}
}
