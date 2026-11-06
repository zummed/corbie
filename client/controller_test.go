package client_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/store"
)

// The controller makes the restarts a job asks for and gives the machine
// only the final outcome; each attempt after the first is made a little
// later.
func TestRestartAfterAFailure(t *testing.T) {
	l := local(t)
	started := time.Now()
	r := run(t, l, `
		flow: retry
		stages:
		  flaky:
		    job: { script: ./flaky.js, restart: { limit: 2 } }
		  never:
		    job: { script: ./never.js, restart: { limit: 1 } }
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

// Values an attempt emitted stand, and every value of the next attempt is
// delivered, so a job that hears something new after a restart loses nothing.
func Test_ctl_a_restart_delivers_all_it_emits(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: emits-twice
		stages:
		  source:
		    job: { script: ./source.js, restart: { limit: 1 } }
		  each_one:
		    each: { emitted: source }
		    job: { script: ./echo.js, input: { item: "${item}" } }
		output: ${stages.each_one.output.map(r => r.output.item)}
	`, map[string]string{
		"source.js": `
			emit("item", "a" + job.attempt); emit("item", "b" + job.attempt)
			if (job.attempt === 1) throw new Error("after two")
			emit("item", "c" + job.attempt)
			return 3
		`,
		"echo.js": scripts["echo.js"],
	}, nil)
	check(t, r.Status, r.Output, "done", `["a1", "b1", "a2", "b2", "c2"]`)
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
		  d: { job: { script: ./add.js, input: { n: "${stages.c.output.n}" }, restart: { limit: 2 } } }
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
	id, err := l.Run(ctx, draft(t, strings.Replace(slowFamily, "DEADLINE", "", 1),
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
	id, err := l.Run(ctx, draft(t, strings.Replace(slowFamily, "DEADLINE", "limits: { timeout: 1s }", 1),
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

// The record names an attempt's end as that attempt, and not the one after it.
func Test_ctl_the_record_names_each_attempt(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: named
		stages:
		  flaky:
		    job: { script: ./never.js, restart: { limit: 1, delay: 1ms } }
		  note:
		    when: ${stages.flaky.failed}
		    job: { script: ./never.js, restart: { when: always, limit: 1, delay: 1ms }, input: { fine: true } }
	`, map[string]string{"never.js": `if (input && input.fine) return 1; throw new Error("no")`}, nil)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	var seen []string
	if _, err := l.Record(ctx, r.ID, func(e client.Entry) {
		for _, event := range e.Events {
			if event.Name == "failed" || event.Name == "done" {
				seen = append(seen, event.Name+":"+event.From)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(seen, " "), "failed:flaky failed:flaky done:note done:note"; got != want {
		t.Fatalf("the record names %s, and not %s", got, want)
	}
}

// A job with no restart: runs once, whatever happens to it.
func Test_ctl_a_job_with_no_restart_runs_once(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: once
		stages:
		  never:
		    job: { script: ./never.js }
		  note:
		    when: ${stages.never.failed}
		    job: { script: ./echo.js, input: { why: "${stages.never.error}" } }
		output: ${stages.note.output.why}
	`, map[string]string{"never.js": `throw new Error("attempt " + job.attempt)`, "echo.js": scripts["echo.js"]}, nil)
	if r.Status != "done" || !strings.Contains(r.Output.(string), "attempt 1") {
		t.Fatalf("the run ended %s with %v", r.Status, r.Output)
	}
}

// A policy with no limit goes on until the job ends well, and a job that ends
// well under when: failure is final.
func Test_ctl_no_limit_goes_on_until_it_works(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: until
		stages:
		  flaky:
		    job: { script: ./flaky.js, restart: { delay: 1ms } }
		output: ${stages.flaky.output}
	`, map[string]string{"flaky.js": `if (job.attempt < 12) throw new Error("not yet"); return job.attempt`}, nil)
	if r.Status != "done" || r.Output != 12.0 {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// when: always runs a job that ended well again, as a new job with a new request
// name. The limit's last run gives the machine its outcome, and the record shows
// the rest.
func Test_ctl_when_always_runs_again_after_it_ends_well(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: again
		stages:
		  count:
		    job: { script: ./count.js, restart: { when: always, limit: 2, delay: 1ms } }
		output: ${stages.count.output}
	`, map[string]string{"count.js": `return job.request`}, nil)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	if got := r.Output.(string); !strings.HasSuffix(got, "/count#3") {
		t.Fatalf("the last run had the request name %s", got)
	}
}

// A limit in a length of time counts only the restarts in it, so a job that fails
// rarely is never out of them.
func Test_ctl_a_limit_in_a_length_of_time_forgets_old_restarts(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: window
		stages:
		  slow:
		    job: { script: ./fail.js, restart: { limit: 1 in 200ms, delay: 250ms } }
		output: ${stages.slow.output}
	`, map[string]string{"fail.js": `if (job.attempt < 5) throw new Error("not yet"); return job.attempt`}, nil)
	if r.Status != "done" || r.Output != 5.0 {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
	r = run(t, l, `
		flow: window
		stages:
		  fast:
		    job: { script: ./fail.js, restart: { limit: 1 in 1h, delay: 1ms } }
		output: ${stages.fast.output}
	`, map[string]string{"fail.js": `if (job.attempt < 5) throw new Error("attempt " + job.attempt); return job.attempt`}, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "attempt 2") {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
}

// A restart after a job ended well is a new job, so it counts against the limit
// of jobs, and that limit ends it.
func Test_ctl_a_new_job_counts_against_the_limit_of_jobs(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: limited
		limits: { jobs: 3 }
		stages:
		  count:
		    job: { script: ./count.js, restart: { when: always, delay: 1ms } }
	`, map[string]string{"count.js": `return 1`}, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "job limit reached") {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
}

// A restart after a failure is the same job, and does not count.
func Test_ctl_a_restart_after_a_failure_is_the_same_job(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: limited
		limits: { jobs: 1 }
		stages:
		  flaky:
		    job: { script: ./flaky.js, restart: { limit: 4, delay: 1ms } }
		output: ${stages.flaky.output}
	`, map[string]string{"flaky.js": `if (job.attempt < 4) throw new Error("not yet"); return job.attempt`}, nil)
	if r.Status != "done" || r.Output != 4.0 {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// A yes covers a job until it ends well: a failure is not asked about again, and
// a job that ended well is.
func Test_ctl_a_restart_is_held_only_after_a_job_ended_well(t *testing.T) {
	l := local(t)
	askFile := `flow: ask
stages:
  yes: { job: { script: ./yes.js, input: { jobs: "${input.jobs}" } } }
output: ${stages.yes.output}
`
	yes := `return { approve: input.jobs.map((j) => j.name) }`
	failing := run(t, l, `
		flow: held
		ask: { machine: ./ask.yaml }
		stages:
		  flaky:
		    job: { script: ./flaky.js, approve: true, restart: { limit: 2, delay: 1ms } }
	`, map[string]string{"ask.yaml": askFile, "yes.js": yes, "flaky.js": `if (job.attempt < 3) throw new Error("not yet"); return 1`}, nil)
	if failing.Status != "done" {
		t.Fatalf("the run ended %s: %s", failing.Status, failing.Error)
	}
	if got := countAsks(t, l, failing.ID); got != 1 {
		t.Fatalf("a failure was asked about again: %d asks", got)
	}
	well := run(t, l, `
		flow: held
		ask: { machine: ./ask.yaml }
		stages:
		  count:
		    job: { script: ./one.js, approve: true, restart: { when: always, limit: 2, delay: 1ms } }
	`, map[string]string{"ask.yaml": askFile, "yes.js": yes, "one.js": `return 1`}, nil)
	if well.Status != "done" {
		t.Fatalf("the run ended %s: %s", well.Status, well.Error)
	}
	if got := countAsks(t, l, well.ID); got != 3 {
		t.Fatalf("a job that ended well was asked about %d times", got)
	}
}

// countAsks gives how many times a run's ask was started.
func countAsks(t *testing.T, l *client.Local, id string) int {
	t.Helper()
	runs, err := l.List(ctx, client.Filter{Finished: true, Parent: id})
	if err != nil {
		t.Fatal(err)
	}
	asks := 0
	for _, run := range runs {
		if run.Machine == "ask" {
			asks++
		}
	}
	return asks
}

// A cancel is never restarted, so a child cancelled by hand stays stopped.
func Test_ctl_a_cancelled_job_is_not_restarted(t *testing.T) {
	l := local(t)
	id, err := l.Run(ctx, draft(t, `
		flow: parent
		stages:
		  kid:
		    job: { machine: ./kid.yaml, restart: { when: always, delay: 1ms } }
		  after:
		    when: ${stages.kid.failed}
		    job: { script: ./one.js }
		output: { stopped: "${stages.kid.failed}" }
	`, map[string]string{
		"kid.yaml": "statechart: kid\ninitial: waiting\nstates:\n  waiting: { on: { end: over } }\n  over: { end: done }\n",
		"one.js":   "return 1",
	}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	child := ""
	for deadline := time.Now().Add(10 * time.Second); child == "" && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		runs, err := l.List(ctx, client.Filter{Parent: id})
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			if run.Machine == "kid" {
				child = run.ID
			}
		}
	}
	if child == "" {
		t.Fatal("no child started")
	}
	if _, err := l.Cancel(ctx, child, 0); err != nil {
		t.Fatal(err)
	}
	r, err := l.Wait(ctx, id, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "done" || r.Output.(map[string]any)["stopped"] != true {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// An ask that ends well is final, whatever its policy says, so a job held for it
// is not left waiting.
func Test_ctl_an_ask_ends_when_it_ends_well(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: asked
		ask: { script: ./yes.js, restart: { when: always, delay: 1ms } }
		stages:
		  held: { job: { script: ./one.js, approve: true } }
		output: ${stages.held.output}
	`, map[string]string{"yes.js": `return { approve: input.jobs.map((j) => j.name) }`, "one.js": "return 1"}, nil)
	if r.Status != "done" || r.Output != 1.0 {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}
