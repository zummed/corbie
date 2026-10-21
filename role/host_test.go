package role_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// until waits for something the host's copies bring about.
func (h *host) until(what string, holds func() bool) {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	give := time.AfterFunc(5*time.Second, func() {
		h.mu.Lock()
		h.change.Broadcast()
		h.mu.Unlock()
	})
	defer give.Stop()
	for began := time.Now(); !holds(); {
		if time.Since(began) > 5*time.Second {
			h.t.Fatalf("it never came about: %s", what)
		}
		h.change.Wait()
	}
}

func Test_role_standin_host_freezes_kills_and_counts(t *testing.T) {
	h := newHost(t)
	owner := h.mem.As(store.Party{Kind: store.Owner})
	there := func(name string) bool {
		_, mark, err := owner.Read(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		return mark != store.None
	}
	write := func(s store.Store, name string) error {
		_, err := s.Write(context.Background(), name, []byte("x"), store.OnlyIfNew)
		return err
	}

	// A copy for every call, each made from nothing, with a disk of its
	// own, and told how long it may run.
	made := 0
	plain := func(c *copyOf) role.Pass {
		made++
		s := c.store(store.Owner)
		return func(ctx context.Context) (time.Time, error) {
			if _, told := ctx.Deadline(); !told {
				t.Error("a copy was not told how long it may run")
			}
			if entries, err := os.ReadDir(c.disk()); err != nil || len(entries) != 0 {
				t.Errorf("a new copy's disk holds %v (%v)", entries, err)
			}
			if err := os.WriteFile(filepath.Join(c.disk(), "mine"), []byte("x"), 0o600); err != nil {
				t.Error(err)
			}
			return time.Time{}, write(s, fmt.Sprintf("plain/%d", made))
		}
	}
	a, b := h.call("plain", plain), h.call("plain", plain)
	if made != 2 || a.disk() == b.disk() || !there("plain/1") || !there("plain/2") {
		t.Fatalf("two calls made %d copies, with the disks %s and %s", made, a.disk(), b.disk())
	}

	// A copy that leaves work behind when it answers is frozen: the work
	// does not go on, the copy is counted, and thawed it goes on from where
	// it was.
	release, landed := make(chan struct{}), make(chan error, 1)
	lingers := func(c *copyOf) role.Pass {
		s := c.store(store.Owner)
		return func(ctx context.Context) (time.Time, error) {
			go func() {
				<-release
				landed <- write(s, "left/behind")
			}()
			return time.Time{}, write(s, "left/before")
		}
	}
	c := h.call("lingers", lingers)
	if h.worked() != 0 {
		t.Fatal("a copy that has made no request since it answered is counted as working on")
	}
	close(release)
	h.until("the frozen copy's request is held", func() bool { return c.held == 1 })
	if !there("left/before") || there("left/behind") || h.worked() != 1 {
		t.Fatalf("a copy that works on after answering: its write landed: %v; copies counted: %d", there("left/behind"), h.worked())
	}
	c.thaw()
	if err := <-landed; err != nil || !there("left/behind") {
		t.Fatalf("thawed, the copy's write did not land (%v)", err)
	}
	h.call("plain", plain)
	if h.worked() != 1 || h.killed() != 0 {
		t.Fatalf("after one copy that worked on and four that did not, %d are counted, and %d killed", h.worked(), h.killed())
	}

	// A copy that goes on after its time is up is killed at its limit: what
	// it had in hand is done, and nothing after it.
	var told error
	heedless := func(c *copyOf) role.Pass {
		s := c.store(store.Owner)
		return func(ctx context.Context) (time.Time, error) {
			for n := 1; ; n++ {
				if err := write(s, fmt.Sprintf("heedless/%03d", n)); err != nil {
					told = ctx.Err()
					return time.Time{}, err
				}
			}
		}
	}
	h.each = func(c *copyOf, _ string, _ bool) {
		if c.writes == 5 {
			c.up()
		}
	}
	c = h.call("heedless", heedless)
	wrote, _ := store.Names(owner.List(ctx, "heedless"))
	if !c.killed || h.killed() != 1 || len(wrote) != 5+grace || told == nil {
		t.Fatalf("a copy that went on after its time was up: killed: %v, having written %d values; its pass was told: %v", c.killed, len(wrote), told)
	}
	// One that stops when it is told is not.
	heeds := func(c *copyOf) role.Pass {
		s := c.store(store.Owner)
		return func(ctx context.Context) (time.Time, error) {
			for n := 1; ctx.Err() == nil; n++ {
				if err := write(s, fmt.Sprintf("heeds/%03d", n)); err != nil {
					return time.Time{}, err
				}
			}
			return time.Time{}, ctx.Err()
		}
	}
	c = h.call("heeds", heeds)
	wrote, _ = store.Names(owner.List(ctx, "heeds"))
	if c.killed || h.killed() != 1 || len(wrote) != 5 {
		t.Fatalf("a copy that stopped when its time was up: killed: %v, having written %d values", c.killed, len(wrote))
	}

	// Killed at a write, a copy has not made it.
	h.each = func(c *copyOf, name string, _ bool) {
		if strings.HasSuffix(name, "/003") {
			c.kill()
		}
	}
	c = h.call("heedless", heedless)
	if !c.killed || there("heedless/"+fmt.Sprintf("%03d", 5+grace+1)) {
		t.Fatal("a copy the host killed went on")
	}
	wrote, _ = store.Names(owner.List(ctx, "heedless"))
	if len(wrote) != 5+grace {
		t.Fatalf("a copy killed at a write that finds its values already there: %d values", len(wrote))
	}

	// Frozen at a write in the middle of its pass, a copy has not made it,
	// and makes it when it is thawed.
	h.each = func(c *copyOf, name string, _ bool) {
		if name == "heeds/007" {
			c.freeze()
		}
	}
	slow := func(c *copyOf) role.Pass {
		s := c.store(store.Owner)
		return func(ctx context.Context) (time.Time, error) {
			return time.Time{}, write(s, "heeds/007")
		}
	}
	c = h.call("heeds", slow)
	if c.answered || there("heeds/007") {
		t.Fatal("a copy frozen at a write made it")
	}
	c.thaw()
	c.wait()
	if !c.answered || !there("heeds/007") || c.err != nil {
		t.Fatalf("thawed, the copy did not finish its pass (%v)", c.err)
	}
}

// every tries a point of the work, and the next, and so on, several at a
// time, until one finds that the work never reached its point. try says
// whether the point was reached and, if anything went wrong there, what.
func every(t *testing.T, least int, try func(k int) (reached bool, wrong string)) {
	t.Helper()
	type came struct {
		reached bool
		wrong   string
	}
	for from := 1; ; from += 16 {
		results := make([]came, 16)
		var wg sync.WaitGroup
		for i := range results {
			wg.Go(func() { results[i].reached, results[i].wrong = try(from + i) })
		}
		wg.Wait()
		for i, r := range results {
			switch {
			case t.Failed():
				return
			case r.wrong != "":
				t.Fatal(r.wrong)
			case !r.reached && from+i <= least:
				t.Fatalf("the work has only %d points at which to stop it", from+i-1)
			case !r.reached:
				return
			}
		}
	}
}

// unbroken does the work with nothing done to any copy, and tells what
// came of it.
func unbroken(t *testing.T) string {
	t.Helper()
	w := newWorld(t)
	w.submit(everything, map[string]string{"work.js": "return 1"}, nil)
	w.work(4, w.play)
	want := w.outcome()
	for _, part := range []string{"parent: done", "child: done", "script flaky,", "script flaky@2", "timer", "again", "output: map[flaky:map[attempt:2] kid:map[from:child]]", "left waiting: 0"} {
		if !strings.Contains(want, part) {
			t.Fatalf("the work, with nothing done to any copy, did not end as it should (no %q):\n%s", part, want)
		}
	}
	return want
}

func Test_role_kill_controller_pass_at_every_write(t *testing.T) {
	want := unbroken(t)
	// The copy that makes the k-th of all the controller's writes is killed
	// there, and fresh copies carry on.
	every(t, 40, func(k int) (bool, string) {
		w := newWorld(t)
		w.submit(everything, map[string]string{"work.js": "return 1"}, nil)
		w.h.each = nth("ctl", k, func(c *copyOf) { c.kill() })
		w.work(4, w.play)
		if got := w.outcome(); got != want {
			return true, fmt.Sprintf("killed at write %d, the store after fresh passes is not the one an unbroken pass leaves.\nkilled:\n%s\nunbroken:\n%s", k, got, want)
		}
		if w.h.worked() != 0 {
			return true, fmt.Sprintf("%d copies worked on after answering", w.h.worked())
		}
		return w.h.killed() == 1, ""
	})
}

func Test_role_copy_frozen_at_every_write_and_thawed_harms_nothing(t *testing.T) {
	want := unbroken(t)
	var anew atomic.Int64
	every(t, 40, func(k int) (bool, string) {
		w := newWorld(t)
		w.submit(everything, map[string]string{"work.js": "return 1"}, nil)
		// The copy that makes the k-th of the controller's writes is frozen
		// there, and other copies do all the work, to its end.
		var frozen *copyOf
		w.h.each = nth("ctl", k, func(c *copyOf) {
			c.freeze()
			frozen = c
		})
		w.work(4, w.play)
		if frozen == nil {
			return false, ""
		}
		if got := w.outcome(); got != want {
			return true, fmt.Sprintf("with a copy frozen at write %d, the others left the store otherwise.\nfrozen:\n%s\nunbroken:\n%s", k, got, want)
		}
		// Thawed, it goes on from where it was with what it held before.
		w.noted()
		frozen.thaw()
		frozen.wait()
		for _, op := range w.noted() {
			if op.Party.Kind != store.Controller || op.Act == store.ActRead || op.Act == store.ActList {
				continue
			}
			if op.Act != store.ActDelete && op.How == store.Overwrite {
				return true, fmt.Sprintf("thawed at write %d, the copy wrote %s whatever was there", k, op.Name)
			}
			if strings.Contains(op.Name, "/records/") && op.Act != store.ActDelete && op.Done {
				return true, fmt.Sprintf("thawed at write %d, the copy changed the record: %s", k, op.Name)
			}
			if op.Act == store.ActAdd && op.Done {
				anew.Add(1)
			}
		}
		if left := w.list(store.Running("ctl")); len(left) != 0 {
			return true, fmt.Sprintf("thawed at write %d, the copy left a head among the running for a run that has ended: %v", k, left)
		}
		// Nothing of any instance has changed, and what it wrote anew is
		// dropped by the passes that follow, once its time is past.
		w.h.tick(12 * time.Minute)
		w.work(2, w.play)
		if got := w.outcome(); got != want {
			return true, fmt.Sprintf("a copy frozen at write %d and thawed after the work was done left the store otherwise.\nthawed:\n%s\nunbroken:\n%s", k, got, want)
		}
		if w.h.killed() != 0 || w.h.worked() != 0 {
			return true, fmt.Sprintf("the host killed %d copies, and %d worked on after answering", w.h.killed(), w.h.worked())
		}
		return true, ""
	})
	t.Logf("over every write, thawed copies of the controller wrote %d things anew, all of which later passes dropped", anew.Load())

	// A copy of the runner frozen at the k-th write of its pass, and thawed
	// after other copies have had the job before them: at once, or when the
	// job's due time has long passed. Its script runs once, whoever runs it.
	for _, late := range []bool{false, true} {
		every(t, 3, func(k int) (bool, string) {
			w, run, tally := oneJobSent(t, "1m")
			var frozen *copyOf
			w.h.each = nth("r1", k, func(c *copyOf) {
				c.freeze()
				frozen = c
			})
			real := func() { w.h.call("r1", w.runner) }
			real()
			if frozen == nil {
				return false, ""
			}
			marked := len(w.list(store.TakenList("r1"))) == 1
			answered := len(w.list(store.Inbox("ctl")+"/r1")) == 1
			w.work(2, real)
			if late {
				w.h.tick(8 * time.Minute)
				w.work(2, real)
			}
			frozen.thaw()
			frozen.wait()
			w.work(2, real)
			w.h.tick(8 * time.Minute)
			w.work(2, real)
			how := fmt.Sprintf("a runner's copy frozen at write %d of its pass (the job marked: %v, its result written: %v) and thawed (after the job's due time: %v)", k, marked, answered, late)
			steps := w.steps(run)
			if head := w.head(run); head == nil || head.Status != "done" || len(steps) == 0 {
				return true, fmt.Sprintf("%s left the run as %+v", how, head)
			}
			// It held the job when it was frozen, and no other copy could do
			// the work: thawed past the due time, its result comes too late.
			want := "ok"
			if late && marked && !answered {
				want = "timeout"
			}
			if output := steps[len(steps)-1].State.Output; output != want || ran(tally) != 1 {
				return true, fmt.Sprintf("%s: the machine was given %v and the script ran %d times", how, output, ran(tally))
			}
			if left := append(w.waiting(), w.list(store.TakenList("r1"))...); len(left) != 0 || w.h.killed() != 0 || w.h.worked() != 0 {
				return true, fmt.Sprintf("%s: this is left: %v", how, left)
			}
			return true, ""
		})
	}
}

func Test_role_host_limit_at_every_point_kills_nothing(t *testing.T) {
	want := unbroken(t)
	// The time of the copy that makes the k-th of the controller's requests
	// is up there. It finishes what it has in hand and answers; the calls
	// that follow take up what it had not reached.
	every(t, 100, func(k int) (bool, string) {
		w := newWorld(t)
		w.submit(everything, map[string]string{"work.js": "return 1"}, nil)
		seen, reached := 0, false
		w.h.each = func(c *copyOf, _ string, _ bool) {
			if c.party != "ctl" {
				return
			}
			if seen++; seen == k {
				c.up()
				reached = true
			}
		}
		w.work(4, w.play)
		if w.h.killed() != 0 || w.h.worked() != 0 {
			return true, fmt.Sprintf("with its time up at request %d, %d copies went on and were killed, and %d worked on after answering", k, w.h.killed(), w.h.worked())
		}
		if got := w.outcome(); got != want {
			return true, fmt.Sprintf("with a copy's time up at request %d, the work ended otherwise.\ncut short:\n%s\nnever cut short:\n%s", k, got, want)
		}
		return reached, ""
	})
}

func Test_role_minute_call_takes_waiting_messages(t *testing.T) {
	w := newWorld(t)
	const waits = `
		statechart: waits
		initial: waiting
		states:
		  waiting:
		    timers: { too_long: 30s }
		    on:
		      too_long: late
		      tick: { send: { to: parent, event: ticked } }
		  late:
		    on: { tick: { send: { to: parent, event: ticked } } }
	`
	first := w.submit(waits, nil, nil)
	w.h.call("ctl", w.controller) // the call the bucket made for the start
	if head := w.head(first); head == nil || head.Timers != 1 {
		t.Fatalf("the first run is %+v", head)
	}
	// A start and an event are written, and the bucket's notice of each is
	// lost: no call comes for them. The timer's time passes too.
	second := w.submit(strings.Replace(waits, "timers: { too_long: 30s }", "", 1), nil, nil)
	w.client.Send(ctx, first, "tick", nil, 0)
	w.h.tick(time.Minute)
	w.h.call("ctl", w.controller) // the call once a minute
	if head := w.head(second); head == nil || head.Step != 1 {
		t.Fatalf("the call once a minute did not take the start that was waiting: %+v", head)
	}
	steps := w.steps(first)
	var given []string
	for _, event := range steps[len(steps)-1].Events {
		given = append(given, event.Name)
	}
	if len(steps) != 2 || strings.Join(given, " ") != "tick timer" {
		t.Fatalf("the call once a minute gave the waiting run %v in %d steps", given, len(steps))
	}
	if left := w.waiting(); len(left) != 0 || w.h.killed() != 0 {
		t.Fatalf("left waiting after the call once a minute: %v", left)
	}
}

// oneJob is a run of one job, whose script notes on the test's disk each
// time it runs, and emits a value.
const oneJob = `
	statechart: one-job
	input: { dir: any }
	initial: working
	states:
	  working:
	    job: { script: ./work.js, input: { tally: "${input.dir}/tally" }, timeout: TIMEOUT, allow: ["fs.write:${input.dir}/**"] }
	    on:
	      done: finished
	      failed: broken
	  finished: { end: done, output: "${states.working.output}" }
	  broken: { end: done, output: "${event.data.status}" }
`

const tallies = `
	fs.write(input.tally, { text: "ran\n", append: true })
	emit("ran", 1)
	return "ok"
`

// oneJobSent starts a run of one job and has the controller send the job.
func oneJobSent(t *testing.T, timeout string) (w *world, run, tally string) {
	t.Helper()
	w = newWorld(t)
	dir := t.TempDir()
	tally = filepath.Join(dir, "tally")
	run = w.submit(strings.Replace(oneJob, "TIMEOUT", timeout, 1), map[string]string{"work.js": tallies}, map[string]any{"dir": dir})
	w.h.call("ctl", w.controller)
	if jobs := w.list(store.Inbox("r1")); len(jobs) != 1 {
		t.Errorf("the jobs sent are %v", jobs)
	}
	return w, run, tally
}

// ran says how many times the script has run.
func ran(tally string) int {
	noted, _ := os.ReadFile(tally)
	return strings.Count(string(noted), "ran\n")
}

func Test_role_kill_runner_pass_at_every_write(t *testing.T) {
	every(t, 3, func(k int) (bool, string) {
		w, run, tally := oneJobSent(t, "1m")
		// The runner's copy is killed at the k-th write of its pass.
		w.h.each = nth("r1", k, func(c *copyOf) { c.kill() })
		killed := w.h.call("r1", w.runner).killed
		w.h.each = nil
		marked := len(w.list(store.TakenList("r1"))) == 1
		answered := len(w.list(store.Inbox("ctl")+"/r1")) == 1
		emitted := len(w.list(store.Emits("ctl", run)))
		// Fresh passes; and then the job's due time passes, and with it the
		// time for which the job is good.
		real := func() { w.h.call("r1", w.runner) }
		w.work(2, real)
		before := ran(tally)
		w.h.tick(8 * time.Minute)
		w.work(2, real)
		steps := w.steps(run)
		if head := w.head(run); head == nil || head.Status != "done" || len(steps) == 0 {
			return killed, fmt.Sprintf("killed at write %d, the run is %+v", k, head)
		}
		how := fmt.Sprintf("killed at write %d of its pass (the job marked: %v, its result written: %v)", k, marked, answered)
		output, times := steps[len(steps)-1].State.Output, ran(tally)
		switch {
		case !killed, answered, !marked:
			// The job was never taken, or its result was written: its script
			// ran once, by whichever copy, and the machine has the result.
			if output != "ok" || times != 1 {
				return killed, fmt.Sprintf("%s, the machine was given %v and the script ran %d times", how, output, times)
			}
		default:
			// The job was in the hands of the copy that was killed: no copy
			// runs it again, the machine sees a timeout at its due time, and
			// what it had emitted stands.
			if output != "timeout" || times > 1 || before != times || len(w.list(store.Emits("ctl", run))) != emitted {
				return killed, fmt.Sprintf("%s, the machine was given %v and the script ran %d times", how, output, times)
			}
		}
		if left := append(w.waiting(), w.list(store.TakenList("r1"))...); len(left) != 0 {
			return killed, fmt.Sprintf("%s, once the job's due time has passed this is left: %v", how, left)
		}
		return killed, ""
	})
}

func Test_role_runner_pass_waits_for_its_jobs(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		statechart: slow
		initial: working
		states:
		  working:
		    job: { script: ./work.js }
		    on: { done: over }
		  over: { end: done, output: "${states.working.output}" }
	`, map[string]string{"work.js": `const until = Date.now() + 200; while (Date.now() < until) {}; return "took its time"`}, nil)
	w.h.call("ctl", w.controller)
	c := w.h.call("r1", w.runner)
	// When the pass has answered, the job it took is done: its result is
	// written and the job is gone from the inbox.
	if results, jobs := w.list(store.Inbox("ctl")+"/r1"), w.list(store.Inbox("r1")); c.err != nil || len(results) != 1 || len(jobs) != 0 {
		t.Fatalf("the runner's pass answered (%v) with %d results written and %d jobs in its inbox", c.err, len(results), len(jobs))
	}
	w.h.call("ctl", w.controller)
	if steps := w.steps(run); w.head(run).Status != "done" || steps[len(steps)-1].State.Output != "took its time" {
		t.Fatalf("the run is %+v", w.head(run))
	}
	if w.h.worked() != 0 || w.h.killed() != 0 {
		t.Fatalf("%d copies worked on after answering, and %d were killed", w.h.worked(), w.h.killed())
	}
}

func Test_role_runner_never_holds_a_job_at_the_host_limit(t *testing.T) {
	// A job whose script may run for twenty seconds, and a call that the
	// host lets run for four seconds, for eight, and so on up to a minute.
	const timeout = 20 * time.Second
	took := 0
	for limit := 4 * time.Second; limit <= time.Minute; limit += 4 * time.Second {
		w, _, tally := oneJobSent(t, "20s")
		w.h.limit = limit
		c := w.h.call("r1", w.runner)
		jobs, results := w.list(store.Inbox("r1")), w.list(store.Inbox("ctl")+"/r1")
		taken := ran(tally) > 0
		if c.err != nil || c.killed || !c.answered {
			t.Fatalf("with %s from the host, the runner's pass ended %v", limit, c.err)
		}
		switch {
		case taken && limit < timeout+role.HostMargin:
			// Had its script run to its limit, the copy would have been killed
			// with the job in its hands, or with no time to answer it.
			t.Fatalf("with %s from the host, the runner ran a job that may run for %s", limit, timeout)
		case taken:
			took++
			if ran(tally) != 1 || len(jobs) != 0 || len(results) != 1 {
				t.Fatalf("with %s from the host, the job that was taken ran %d times and %d jobs are left", limit, ran(tally), len(jobs))
			}
		case limit >= timeout+3*role.HostMargin:
			t.Fatalf("with %s from the host, the runner did not run a job that may run for %s", limit, timeout)
		default:
			// A job no call of this length could be sure to finish is not
			// run: it is answered at once as failed.
			if len(jobs) != 0 || len(results) != 1 {
				t.Fatalf("with %s from the host, the job that could not fit left %d jobs in the inbox and %d results", limit, len(jobs), len(results))
			}
		}
	}
	if took == 0 {
		t.Fatal("the runner never took the job")
	}
}

// left gives the goroutines that are in a controller's or a runner's code.
func left() []string {
	buf := make([]byte, 4<<20)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, "/corbie/controller.") || strings.Contains(stack, "/corbie/runner.") {
			found = append(found, stack)
		}
	}
	return found
}

func Test_role_nothing_runs_after_the_pass_answers(t *testing.T) {
	w := newWorld(t)
	w.submit(everything, map[string]string{"work.js": `
		if (job.name.startsWith("flaky") && job.attempt < 2) throw new Error("not the first time")
		emit("ran", job.name)
		return { attempt: job.attempt }
	`}, nil)
	// When a copy has answered, nothing of its pass is left: no part of the
	// controller or the runner is still at work in it, or waiting to be.
	answered := func(party string, made func(*copyOf) role.Pass) int {
		t.Helper()
		before := w.changed()
		c := w.h.call(party, made)
		if c.err != nil {
			t.Fatalf("a pass of %s ended %v", party, c.err)
		}
		began := time.Now()
		for len(left()) > 0 {
			if time.Since(began) > 3*time.Second {
				t.Fatalf("a copy of %s has answered, and this of its pass goes on:\n%s", party, strings.Join(left(), "\n\n"))
			}
			time.Sleep(time.Millisecond)
		}
		return w.changed() - before
	}
	for range 4 {
		for answered("ctl", w.controller)+answered("r1", w.runner) > 0 {
		}
		w.h.tick(time.Second)
	}
	if got := w.outcome(); !strings.Contains(got, "parent: done") || !strings.Contains(got, "left waiting: 0") {
		t.Fatalf("the work ended as\n%s", got)
	}
	// And no copy made a request of the store after it had answered.
	if w.h.worked() != 0 || w.h.killed() != 0 {
		t.Fatalf("%d copies worked on after answering, and %d were killed", w.h.worked(), w.h.killed())
	}
}

// away is a store that is out of reach for as long as it is told to be.
type away struct {
	store.Store
	out atomic.Bool
}

func (a *away) List(ctx context.Context, under string) ([]store.Listed, error) {
	if a.out.Load() {
		return nil, fmt.Errorf("%w: no answer", store.ErrUnreachable)
	}
	return a.Store.List(ctx, under)
}

func Test_role_loop_survives_store_out_of_reach(t *testing.T) {
	w := newWorld(t)
	run := w.submit(everything, map[string]string{"work.js": "return 1"}, nil)
	s := &away{Store: w.h.mem.As(store.Party{Kind: store.Controller, Name: "ctl"})}
	s.out.Store(true)
	made := controller.New(w.ctl, s)
	made.Now = w.h.clock
	loop, stop := context.WithCancel(ctx)
	reported, over := make(chan error, 1000), make(chan struct{})
	go func() {
		defer close(over)
		role.Run(loop, time.Now, func(name string, err error) {
			select {
			case reported <- err:
			default:
			}
		}, role.Role{Name: "controller", Pass: made.Pass, Interval: time.Millisecond})
	}()
	defer func() {
		stop()
		<-over
	}()
	// Pass after pass finds the store out of reach; each is reported, and
	// another follows after the interval.
	for n := range 3 {
		select {
		case err := <-reported:
			if !errors.Is(err, store.ErrUnreachable) {
				t.Fatalf("a pass that could not reach the store was reported as %v", err)
			}
		case <-over:
			t.Fatalf("the loop stopped after %d passes that could not reach the store", n)
		case <-time.After(5 * time.Second):
			t.Fatalf("after %d passes that could not reach the store, no other was made", n)
		}
	}
	if w.head(run) != nil {
		t.Fatal("a store out of reach was written to")
	}
	// The store answers again, and the same loop takes up what was waiting.
	s.out.Store(false)
	for began := time.Now(); w.head(run) == nil; time.Sleep(time.Millisecond) {
		if time.Since(began) > 5*time.Second {
			t.Fatal("when the store answered again, the loop did not take the start that was waiting")
		}
	}
}
