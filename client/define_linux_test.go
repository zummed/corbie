//go:build linux

package client_test

import (
	"context"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
)

// costlyDefine is a script that defines a machine that takes seconds to
// read: thousands of tools in files of tools, each with a long pattern.
const costlyDefine = `
	const files = { "./add.js": "return 1" }, names = []
	for (let f = 0; f < 5; f++) {
		const tools = []
		for (let i = 0; i < 2000; i++) {
			tools.push("t" + f + "_" + i + ": { does: x, job: ./add.js, args: { type: object, properties: { a: { type: string, pattern: \"^a{990}" + i + "$\" } } } }\n")
		}
		names.push("./t" + f + ".yaml")
		files[names[f]] = tools.join("")
	}
	return machines.define("agent: a\n` + growModel + `\nprompt: hi\ntools: { include: [" + names.join(",") + "] }\n", { files })
`

// spent gives the processor time this process has used. It holds the
// runner, and a job's process is another.
func spent(t *testing.T) time.Duration {
	t.Helper()
	var used syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &used); err != nil {
		t.Fatal(err)
	}
	return time.Duration(used.Utime.Nano() + used.Stime.Nano())
}

// A define is read in its job's process. Past the job's time limit the job
// is killed and fails, however long the rest of the read would take. None of
// that time is the runner's, which runs another job meanwhile and goes on
// running them after.
func Test_run_define_stops_at_its_jobs_time_limit(t *testing.T) {
	l := local(t)
	next := "flow: next\nstages:\n  add: { job: { script: ./add.js, input: { n: 1 } } }\noutput: ${stages.add.output}\n"
	before := spent(t)
	id, err := l.Run(ctx, draft(t, "flow: maker\nstages:\n  make: { job: { script: ./make.js, timeout: 100ms } }\n", map[string]string{"make.js": costlyDefine}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	meanwhile := make(chan *client.Run, 1)
	go func() { meanwhile <- run(t, l, next, map[string]string{"add.js": scripts["add.js"]}, nil) }()
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	made, err := l.Wait(waitCtx, id, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if made.Status != "failed" || !strings.Contains(made.Error, "the script ran past its time limit of 100ms") {
		t.Fatalf("the job that defined it ended %s: %.300s", made.Status, made.Error)
	}
	r := <-meanwhile
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
	if used := spent(t) - before; used > 1500*time.Millisecond {
		t.Fatalf("the runner's own process used %s while the job defined a machine", used)
	}
	r = run(t, l, next, map[string]string{"add.js": scripts["add.js"]}, nil)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
}

// A define of a definition that takes more than its
// job's memory: fails the job, and the runner goes on running jobs. Read
// anywhere else, it would have finished.
func Test_run_define_past_its_jobs_memory_is_killed(t *testing.T) {
	l := local(t)
	r := run(t, l, "flow: maker\nstages:\n  make: { job: { script: ./make.js, memory: 16MB } }\n", map[string]string{"make.js": costlyDefine}, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "memory limit of 16777216 bytes") {
		t.Fatalf("the job that defined it ended %s: %.300s", r.Status, r.Error)
	}
	r = run(t, l, "flow: next\nstages:\n  add: { job: { script: ./add.js, input: { n: 1 } } }\noutput: ${stages.add.output}\n", map[string]string{"add.js": scripts["add.js"]}, nil)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
}
