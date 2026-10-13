//go:build linux

package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
)

// This runs on Linux only: the memory a job's process really holds is read
// from /proc, and on no other system is the limit enforced from outside.
func Test_run_memory_limit_enforced_from_outside(t *testing.T) {
	w := newWorld(t)
	limited := func(j *instance.Job) { j.Memory, j.Timeout = 16<<20, machine.Duration(8*time.Second) }
	// A script that holds values past the limit, though not far past it,
	// is interrupted from inside.
	w.job(w.ctl, "script", `const held = []; while (held.length < 24) held.push("x".repeat(1 << 20) + held.length); for (;;) {}`, limited)
	pass(t, w.runner("all"))
	if got := w.results()["script"]; got == nil || got.Status != machine.StatusError || !strings.Contains(got.Error.Error, "the script went past its memory limit of 16777216 bytes") {
		t.Fatalf("a script that made values past its limit: %+v", got)
	}
	// A process that holds memory no script made, where nothing inside it
	// is counting, is killed from outside.
	file := filepath.Join(t.TempDir(), "job")
	w.job(w.ctl, "process", `return "ran"`, limited)
	r := w.runner("all")
	r.Command = []string{os.Args[0], "job-that-holds-memory", file}
	end := passing(r)
	defer end()
	pid := numbered(t, file)
	soon(t, "the job to be answered", func() bool { return w.results()["process"] != nil })
	if got := w.results()["process"]; got.Status != machine.StatusError || !strings.Contains(got.Error.Error, "the job went past its memory limit of 16777216 bytes") {
		t.Fatalf("a process that held memory past its job's limit: %+v", got)
	}
	soon(t, "the job's process to be killed", func() bool { return ended(pid) })
}
