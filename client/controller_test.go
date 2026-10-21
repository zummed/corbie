package client_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/store"
)

// The controller makes the attempts a job asks for and gives the machine
// only the final outcome; each attempt after the first is made a little
// later.
func TestAttempts(t *testing.T) {
	l := local(t)
	started := time.Now()
	r := run(t, l, `
		flow: retry
		stages:
		  flaky:
		    job: { script: ./flaky.js, attempts: 3 }
		  never:
		    job: { script: ./never.js, attempts: 2 }
		  note:
		    when: ${stages.never.failed}
		    job: { script: ./echo.js, input: { why: "${stages.never.error}" } }
		output: { flaky: "${stages.flaky.output}", never: "${stages.note.output.why}" }
	`, map[string]string{
		"flaky.js": `if (job.attempt < 2) throw new Error("not yet"); return { attempt: job.attempt, request: job.request }`,
		"never.js": `throw new Error("attempt " + job.attempt)`,
		"echo.js":  scripts["echo.js"],
	}, nil)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	flaky := out["flaky"].(map[string]any)
	if flaky["attempt"] != 2.0 || !strings.HasSuffix(flaky["request"].(string), "/flaky") {
		t.Fatalf("the second attempt gave %v", flaky)
	}
	if !strings.Contains(out["never"].(string), "attempt 2") {
		t.Fatalf("the machine was given %v, not the final outcome", out["never"])
	}
	if took := time.Since(started); took < 900*time.Millisecond {
		t.Fatalf("the second attempt was made at once (%v)", took)
	}
}

// Values a failed attempt emitted stand, and the same values from a later
// attempt are not delivered twice.
func Test_ctl_later_attempt_emits_not_delivered_twice(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: emits-twice
		stages:
		  source:
		    job: { script: ./source.js, attempts: 2 }
		  each_one:
		    each: { emitted: source }
		    job: { script: ./echo.js, input: { item: "${item}" } }
		output: ${stages.each_one.output.map(r => r.output.item)}
	`, map[string]string{
		"source.js": `
			emit("item", "a"); emit("item", "b")
			if (job.attempt === 1) throw new Error("after two")
			emit("item", "c")
			return 3
		`,
		"echo.js": scripts["echo.js"],
	}, nil)
	check(t, r.Status, r.Output, "done", `["a", "b", "c"]`)
}

func TestLimits(t *testing.T) {
	l := local(t)
	// A definition may limit how many jobs its instance starts.
	r := run(t, l, `
		flow: limited
		limits: { jobs: 3 }
		stages:
		  a: { job: { script: ./add.js, input: { n: 0 } } }
		  b: { job: { script: ./add.js, input: { n: "${stages.a.output.n}" } } }
		  c: { job: { script: ./add.js, input: { n: "${stages.b.output.n}" } } }
		  d: { job: { script: ./add.js, input: { n: "${stages.c.output.n}" }, attempts: 3 } }
	`, scripts, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "d: job limit reached") {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	// A root may have only so many levels of machines below it.
	nested := `
		flow: top
		limits: { depth: 1 }
		stages:
		  one:
		    job:
		      machine:
		        flow: middle
		        stages:
		          two:
		            job:
		              machine:
		                flow: bottom
		                stages:
		                  three: { job: { script: ./add.js, input: { n: 1 } } }
	`
	r = run(t, l, nested, scripts, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "depth limit reached") {
		t.Fatalf("two levels under a root that allows one: %s: %s", r.Status, r.Error)
	}
	if r = run(t, l, strings.Replace(nested, "depth: 1", "depth: 2", 1), scripts, nil); r.Status != "done" {
		t.Fatalf("two levels under a root that allows two: %s: %s", r.Status, r.Error)
	}
}

// A job whose tags no runner has fails before anything is sent.
func TestTagsNoRunnerHas(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: tagged
		stages:
		  it: { job: { script: ./add.js, input: { n: 1 }, tags: [gpu, eu] } }
	`, scripts, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "no runner has the tags the job names: gpu, eu") {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
}

const slowFamily = `
	flow: parent
	DEADLINE
	stages:
	  kid:
	    job:
	      machine:
	        flow: child
	        stages:
	          slow: { job: { script: ./slow.js, allow: ["exec:sleep"] } }
`

// When a machine is cancelled, every job and every machine below it is
// cancelled too, and a job is stopped where it runs.
func TestCancelReachesEverythingBelow(t *testing.T) {
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, strings.Replace(slowFamily, "DEADLINE", "", 1),
		map[string]string{"slow.js": `exec.run("sleep", { args: ["30"], timeout: 60 }); return "finished"`}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	child := waitFor(t, l, func(runs []*client.Run) string {
		for _, r := range runs {
			if r.Machine == "child" && r.Jobs == 1 {
				return r.ID
			}
		}
		return ""
	})
	time.Sleep(300 * time.Millisecond) // let the job be taken
	started := time.Now()
	if _, err := l.Cancel(ctx, id, 0); err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "cancelled" {
		t.Fatalf("the root ended %s", r.Status)
	}
	if r := wait(t, l, child); r.Status != "cancelled" {
		t.Fatalf("the child ended %s", r.Status)
	}
	// The job is stopped long before its thirty seconds.
	waitFor(t, l, func([]*client.Run) string {
		names, _ := store.Names(l.Memory.As(ownerOf()).List(ctx, "runner/inbox"))
		if len(names) == 0 {
			return "empty"
		}
		return ""
	})
	if took := time.Since(started); took > 15*time.Second {
		t.Fatalf("the job went on for %v after the cancel", took)
	}
}

// A machine whose timeout has passed fails as out of time, and
// everything below it is cancelled.
func TestTimeout(t *testing.T) {
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, strings.Replace(slowFamily, "DEADLINE", "limits: { timeout: 1s }", 1),
		map[string]string{"slow.js": `exec.run("sleep", { args: ["30"], timeout: 60 }); return "finished"`}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	if r.Status != "failed" || r.Failure != "timeout" || !strings.Contains(r.Error, "timeout passed") {
		t.Fatalf("the root ended %s/%s: %s", r.Status, r.Failure, r.Error)
	}
	child := waitFor(t, l, func(runs []*client.Run) string {
		all, _ := l.List(ctx, client.Filter{Finished: true, Parent: id})
		for _, r := range all {
			if r.Status == "cancelled" {
				return r.ID
			}
		}
		return ""
	})
	if child == "" {
		t.Fatal("the child was not cancelled")
	}
}

// waitFor looks at the running runs until a test says it has what it
// waits for.
func waitFor(t *testing.T, l *client.Local, found func([]*client.Run) string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := l.List(ctx, client.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if got := found(runs); got != "" {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("what the test waits for never came")
	return ""
}

// Events between a parent and a child arrive once each and in the order
// they were sent, and one sent in the step that starts the child waits for
// the child.
func Test_mach_event_sent_before_child_starts_is_kept(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		statechart: talker
		initial: talking
		states:
		  talking:
		    entry: { send: [{ to: talking, event: one }, { to: talking, event: two }, { to: nobody_home, event: lost }] }
		    job:
		      machine:
		        statechart: listener
		        initial: first
		        states:
		          first: { on: { one: second, two: wrong } }
		          second: { on: { two: third } }
		          third: { end: done, output: in order }
		          wrong: { end: done, output: out of order }
		    on: { done: finished }
		  nobody_home:
		    job: { machine: { statechart: unused, states: { only: { end: done } } } }
		  finished: { end: done, output: "${states.talking.output}" }
	`, nil, nil)
	check(t, r.Status, r.Output, "done", `"in order"`)
}

// A state's own job that emits a value under the state's ticket's name does
// not answer the ticket: only an answer given with the ticket does.
func TestJobCannotAnswerItsStatesTicket(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		statechart: own_job
		initial: waiting
		states:
		  waiting:
		    ticket: verdict
		    job: ./say.js
		    on: { verdict: answered, done: not_answered }
		  answered: { end: done, output: answered }
		  not_answered: { end: done, output: not answered }
	`, map[string]string{"say.js": `emit("verdict", "the job answered")
return 1`}, nil)
	check(t, r.Status, r.Output, "done", `"not answered"`)
}
