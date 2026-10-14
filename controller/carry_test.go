package controller_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// sent opens what the controller left in a runner's inbox under a name.
func (w *world) sent(r *identity.Identity, at string) (*instance.Message, identity.Value) {
	w.t.Helper()
	data, mark, err := w.owner.Read(ctx, at)
	if err != nil || mark == store.None {
		w.t.Fatalf("nothing is at %s (%v)", at, err)
	}
	var msg instance.Message
	value, err := identity.Read(data, at, w.ctl.Card, r, &msg)
	if err != nil {
		w.t.Fatalf("%s: %v", at, err)
	}
	return &msg, value
}

// again leaves a run's start in the inbox once more, so that the step that
// used it is carried out again by the next pass.
func (w *world) again(run string) {
	w.t.Helper()
	w.leave(w.c1, run[strings.IndexByte(run, '/')+1:], &instance.Message{Kind: instance.KindStart, Instance: run,
		Start: &instance.Start{Definition: w.latest(run).State.Definition}})
}

// result leaves a job's result in the controller's inbox, as its runner.
func (w *world) result(r *identity.Identity, run, job string, result instance.Result) {
	w.t.Helper()
	result.Job = job
	w.leave(r, store.Stamp(w.clock())+"-"+job, &instance.Message{Kind: instance.KindResult, Instance: run, Result: &result})
}

func Test_ctl_names_of_what_a_step_asks_for_fixed_by_entry(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	// One step asks for a script, a child, an event to the child, a timer, a
	// timeout and a receipt.
	run := w.submit(`
		statechart: asks
		limits: { timeout: 2h }
		initial: s
		states:
		  s:
		    jobs:
		      job: { script: ./work.js, attempts: 2 }
		      kid:
		        machine:
		          statechart: kid
		          initial: k
		          states:
		            k: { on: { hello: k } }
		    entry: { send: { to: kid, event: hello } }
		    timers: { an_hour: 1h }
		    on: { an_hour: late }
		  late: { end: done }
	`, map[string]string{"work.js": "return 1"}, nil)
	w.noted()
	w.pass() // the first step, carried out; the child has not taken its start
	entry := w.latest(run)
	kinds := map[string]int{}
	var asked []string // the name of each thing the entry asked for
	flat := strings.Replace(run, "/", ".", 1)
	for _, ask := range entry.Asks {
		kinds[ask.Kind]++
		switch ask.Kind {
		case instance.AskScript:
			// The name a job is sent under: its instance and the job's name,
			// which carries the attempt from the second on.
			if ask.Message != flat+".job" || ask.Name != "job" {
				t.Fatalf("the message is named %q for the job %q", ask.Message, ask.Name)
			}
			asked = append(asked, store.Message("r1", "ctl", ask.Message), store.Wake("ctl", ask.At, run, ask.What))
		case instance.AskChild, instance.AskEvent:
			// A message's name: the instance it is sent for, the step, and
			// its place among what the step sends.
			if !strings.HasPrefix(ask.Message, flat+".00000001.") {
				t.Fatalf("a message of the step is named %q", ask.Message)
			}
			asked = append(asked, store.Message("ctl", "ctl", ask.Message))
		case instance.AskWake:
			asked = append(asked, store.Wake("ctl", ask.At, run, ask.What))
		case instance.AskReceipt:
			asked = append(asked, store.Receipt("ctl", ask.Sender, ask.Message[strings.LastIndexByte(ask.Message, '/')+1:]))
		}
	}
	if kinds[instance.AskScript] != 1 || kinds[instance.AskChild] != 1 || kinds[instance.AskEvent] != 1 || kinds[instance.AskWake] != 3 || kinds[instance.AskReceipt] != 1 {
		t.Fatalf("the step asked for %v", kinds)
	}
	wrote := map[string]bool{}
	for _, op := range w.noted() {
		if op.Act == store.ActAdd && op.Done {
			wrote[op.Name] = true
		}
	}
	for _, name := range asked {
		if !wrote[name] {
			t.Fatalf("%s, which the entry names, was not written", name)
		}
	}
	// The entry is carried out again, half an hour later, by a controller
	// started afresh: every write lands on the name the first one had, and
	// is answered "already there".
	w.tick(30 * time.Minute)
	w.afresh(w.ctrl.Store)
	w.ctrl.SideBySide = 1 // the entry is carried out before the child takes anything
	w.again(run)
	w.pass()
	found := map[string]bool{}
	for _, op := range w.noted() {
		if op.Act == store.ActAdd && !op.Done {
			found[op.Name] = true
		}
	}
	for _, name := range asked {
		if !found[name] {
			t.Fatalf("carried out again, the entry did not find %s already there", name)
		}
	}
	// The second attempt at the job has a name of its own, and the same
	// request.
	first, _ := w.sent(r1, store.Message("r1", "ctl", flat+".job"))
	w.result(r1, run, "job", instance.Result{Attempt: 1, Status: machine.StatusError, Error: &machine.Error{Status: machine.StatusError, Error: "no"}})
	w.settle()
	w.tick(2 * time.Second)
	w.settle()
	second, _ := w.sent(r1, store.Message("r1", "ctl", flat+".job@2"))
	if second.Job.Attempt != 2 || second.Job.Request != first.Job.Request || first.Job.Request != run+"/job" {
		t.Fatalf("the second attempt is %d of the request %q; the first was of %q", second.Job.Attempt, second.Job.Request, first.Job.Request)
	}
}

func Test_ctl_job_sent_again_keeps_name_and_due_time(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	jobs := w.list(store.Inbox("r1"))
	if len(jobs) != 1 {
		t.Fatalf("the jobs sent are %v", jobs)
	}
	first, firstValue := w.sent(r1, jobs[0])
	// The runner takes the job out of its inbox; twenty seconds later the
	// step that asked for it is carried out again.
	w.owner.Delete(ctx, jobs[0], store.Overwrite)
	w.tick(20 * time.Second)
	w.afresh(w.ctrl.Store)
	w.again(run)
	w.settle()
	if again := w.list(store.Inbox("r1")); !slices.Equal(again, jobs) {
		t.Fatalf("sent again, the job is at %v and was at %v", again, jobs)
	}
	second, secondValue := w.sent(r1, jobs[0])
	if !second.Job.Due.Equal(first.Job.Due) || !secondValue.Until.Equal(firstValue.Until) {
		t.Fatalf("sent again, the job is due at %s and good until %s; it was due at %s and good until %s",
			second.Job.Due, secondValue.Until, first.Job.Due, firstValue.Until)
	}
	if second.Job.Name != first.Job.Name || second.Job.Attempt != first.Job.Attempt || second.Job.Request != first.Job.Request {
		t.Fatalf("sent again, the job is %+v and was %+v", second.Job, first.Job)
	}
	if wakes := w.wakes(); len(wakes) != 1 {
		t.Fatalf("the wakes for its due time are %v", wakes)
	}
}

func Test_ctl_cancel_good_until_jobs_due_time(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.submit(strings.Replace(oneJob, "timeout: 1m", "timeout: 2h", 1), map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	jobs := w.list(store.Inbox("r1"))
	if len(jobs) != 1 {
		t.Fatalf("the jobs sent are %v", jobs)
	}
	job, _ := w.sent(r1, jobs[0])
	w.client.Cancel(ctx, run, 0)
	w.settle()
	cancel, value := w.sent(r1, jobs[0]+".cancel")
	if cancel.Kind != instance.KindCancel || !strings.HasSuffix(jobs[0], "/"+cancel.Cancel) {
		t.Fatalf("the cancel is %+v", cancel)
	}
	// The job waits in the inbox for a free place: an hour and a half on,
	// the cancel is still good, as it is until the job's result is due.
	if !value.Until.Equal(job.Job.Due) || !value.Good(w.clock().Add(90*time.Minute)) {
		t.Fatalf("the cancel is good until %s, and the job's result is due at %s", value.Until, job.Job.Due)
	}
}

func Test_ctl_blob_copied_up_before_parent_is_told(t *testing.T) {
	const handsUp = `
		statechart: above
		initial: working
		states:
		  working:
		    job:
		      machine:
		        statechart: below
		        initial: working
		        states:
		          working:
		            job: { script: ./work.js }
		            on: { done: over }
		          over: { end: done, output: { file: "${states.working.output}" } }
		    on: { done: over }
		  over: { end: done, output: "${states.working.output}" }
	`
	// begin takes a run to where the child's job has returned a file that
	// lies in the child's record.
	begin := func() (w *world, parent, child string, file blob.Ref) {
		w = newWorld(t)
		r1 := w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{}, nil)
		parent = w.submit(handsUp, map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		child = w.below(parent)
		file = w.blob(child, w.clock(), "made below")
		w.result(r1, child, "working", instance.Result{Attempt: 1, Status: instance.Done, Output: file.Value()})
		return w, parent, child, file
	}
	told := func(w *world) bool {
		for _, name := range w.list(store.Inbox("ctl")) {
			if strings.HasSuffix(name, ".result") {
				return true
			}
		}
		return false
	}
	w, parent, _, file := begin()
	copied := store.Blob("ctl", parent, file.At[strings.LastIndexByte(file.At, '/')+1:])
	w.noted()
	w.pass()
	copyAt, toldAt := -1, -1
	for i, op := range w.noted() {
		if op.Act != store.ActAdd || !op.Done {
			continue
		}
		if op.Name == copied {
			copyAt = i
		}
		if strings.HasSuffix(op.Name, ".result") && strings.HasPrefix(op.Name, store.Inbox("ctl")+"/ctl/") {
			toldAt = i
		}
	}
	if copyAt < 0 || toldAt < copyAt {
		t.Fatalf("the file was in the parent's record at %d and the parent told at %d", copyAt, toldAt)
	}
	w.settle()
	out, _ := w.latest(parent).State.Output.(map[string]any)
	ref, ok := blob.AsRef(out["file"])
	if w.head(parent).Status != instance.Done || !ok || ref.At != copied {
		t.Fatalf("the parent ended %s with %v", w.head(parent).Status, out)
	}
	if contents, err := blob.ReadAll(ctx, w.owner, ref, 1<<20); err != nil || string(contents) != "made below" {
		t.Fatalf("the file the parent was handed reads %q (%v)", contents, err)
	}
	// Stopped at any point, the parent is never told of a file that is not
	// in its record.
	for k := int64(0); ; k++ {
		w, parent, _, file := begin()
		copied := store.Blob("ctl", parent, file.At[strings.LastIndexByte(file.At, '/')+1:])
		inner := w.ctrl.Store
		s := &stopping{Store: inner}
		s.left.Store(k)
		w.afresh(s)
		w.ctrl.Pass(ctx)
		if !s.stopped.Load() {
			if k < 4 {
				t.Fatalf("handing the file up took only %d writes", k)
			}
			break
		}
		if _, mark, _ := w.owner.Read(ctx, copied); told(w) && mark == store.None {
			t.Fatalf("stopped after %d writes, the parent has been told and the file is not in its record", k)
		}
	}
}

// sweep stops a controller after each number of writes in turn, from a
// point a test brings a world to, lets a controller started afresh carry
// on, and gives each world to the test. It ends when a number of writes is
// reached at which the controller was never stopped.
func sweep(t *testing.T, begin func() *world, each func(k int64, w *world, atStop []string)) {
	t.Helper()
	for k := int64(0); ; k++ {
		w := begin()
		inner := w.ctrl.Store
		s := &stopping{Store: inner}
		s.left.Store(k)
		w.afresh(s)
		for range 6 {
			if _, err := w.ctrl.Pass(ctx); err != nil {
				break
			}
		}
		if !s.stopped.Load() {
			if k < 4 {
				t.Fatalf("the work took only %d writes", k)
			}
			return
		}
		atStop := w.list("")
		w.afresh(inner)
		w.settle()
		each(k, w, atStop)
	}
}

func Test_ctl_stopped_between_last_events_and_ending_parent_sees_events_first(t *testing.T) {
	var parent string
	begin := func() *world {
		w := newWorld(t)
		parent = w.submit(`
			statechart: above
			initial: working
			states:
			  working:
			    job:
			      machine:
			        statechart: below
			        initial: waiting
			        states:
			          waiting:
			            on: { finish: { go: over, send: { to: parent, event: bye, data: last words } } }
			          over: { end: done, output: ended }
			    on:
			      done: over
			      finish: { send: { to: working, event: finish } }
			      bye: { send: { to: parent, event: heard } }
			  over: { end: done }
		`, nil, nil)
		w.settle()
		w.client.Send(ctx, parent, "finish", nil, 0)
		return w
	}
	// What the parent was given, step by step.
	given := func(w *world) string {
		var all []string
		for _, step := range w.steps(parent) {
			all = append(all, events(step))
		}
		return strings.Join(all, " | ")
	}
	w := begin()
	w.settle()
	if got := given(w); got != "start | finish | bye done" || w.head(parent).Status != instance.Done {
		t.Fatalf("never stopped, the parent was given %q", got)
	}
	between := 0
	sweep(t, begin, func(k int64, w *world, atStop []string) {
		// Was it stopped with the child's last event written and its ending
		// not?
		event, ending := "", ""
		for _, name := range atStop {
			if strings.HasPrefix(name, store.Inbox("ctl")+"/ctl/") && strings.HasSuffix(name, ".emit") {
				event = name
			}
			if strings.HasPrefix(name, store.Inbox("ctl")+"/ctl/") && strings.HasSuffix(name, ".result") {
				ending = name
			}
		}
		if event != "" && ending == "" {
			between++
		}
		if event != "" && ending != "" && event > ending {
			t.Fatalf("the event's name %s sorts after the ending's %s", event, ending)
		}
		// The parent was given the event before the ending: in one step, or
		// in a step of its own when it was found waiting alone.
		if got := given(w); got != "start | finish | bye done" && got != "start | finish | bye | done" {
			t.Fatalf("stopped after %d writes, the parent was given %q", k, got)
		}
		if w.head(parent).Status != instance.Done {
			t.Fatalf("stopped after %d writes, the parent is %s", k, w.head(parent).Status)
		}
	})
	if between == 0 {
		t.Fatal("the controller was never stopped between the last event and the ending")
	}
}

func Test_ctl_stopped_after_attempt_failed_next_attempt_still_made(t *testing.T) {
	var run string
	var r1 *identity.Identity
	begin := func() *world {
		w := newWorld(t)
		r1 = w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{}, nil)
		run = w.submit(strings.Replace(oneJob, "timeout: 1m", "timeout: 1m, attempts: 2", 1), map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		w.result(r1, run, "working", instance.Result{Attempt: 1, Status: machine.StatusError, Error: &machine.Error{Status: machine.StatusError, Error: "not this time"}})
		return w
	}
	second := func(w *world) string {
		return store.Message("r1", "ctl", store.JobMessage(run, "working@2"))
	}
	stops := 0
	sweep(t, begin, func(k int64, w *world, _ []string) {
		stops++
		// The wait before the second attempt passes.
		w.tick(5 * time.Second)
		w.settle()
		if _, mark, _ := w.owner.Read(ctx, second(w)); mark == store.None {
			t.Fatalf("stopped after %d writes of the step that took the failure, the second attempt was never made: the run is %+v, with %v waiting",
				k, w.head(run), append(w.list(store.Inbox("ctl")), w.wakes()...))
		}
		if head := w.head(run); head.Status != instance.Running || head.Jobs != 1 {
			t.Fatalf("stopped after %d writes, the run is %+v", k, head)
		}
	})
	if stops < 4 {
		t.Fatalf("the controller was stopped at %d points", stops)
	}
}

func Test_ctl_job_that_cannot_be_sent_fails_though_controller_stops(t *testing.T) {
	for job, want := range map[string]string{
		// Decided when the step is taken: no runner has the tag.
		`{ script: ./work.js, tags: [gpu] }`: "no runner has the tags the job names: gpu",
		// Found when the entry is carried out: the secret is not set.
		`{ script: ./work.js, allow: ["secret:NONE@net:api.example"] }`: `the secret "NONE" is not set`,
	} {
		var id string
		begin := func() *world {
			w := newWorld(t)
			w.enrol(w.party("r1", identity.Runner), permission.Layer{Allow: []string{"all"}}, nil)
			id = w.submit(fmt.Sprintf(`
				statechart: unsent
				initial: s
				states:
				  s:
				    job: %s
				    on: { failed: told }
				  told: { end: done, output: "${event.data.error}" }
			`, job), map[string]string{"work.js": "return 1"}, nil)
			return w
		}
		stops := 0
		sweep(t, begin, func(k int64, w *world, _ []string) {
			stops++
			head, last := w.head(id), w.latest(id)
			if head == nil || head.Status != instance.Done || !strings.Contains(fmt.Sprint(last.State.Output), want) {
				t.Fatalf("%s: stopped after %d writes, the machine was never told its job failed: the run is %+v with %v",
					job, k, head, last.State.Output)
			}
			if left := append(append(w.list(store.Inbox("ctl")), w.wakes()...), w.list(store.Inbox("r1"))...); len(left) != 0 {
				t.Fatalf("%s: stopped after %d writes, left waiting: %v", job, k, left)
			}
		})
		if stops < 4 {
			t.Fatalf("%s: the controller was stopped at %d points", job, stops)
		}
	}
}

// unsent is a store through which no job can be sent: a runner's inbox is
// out of reach.
type unsent struct{ store.Store }

func (u unsent) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	if !strings.HasPrefix(name, "ctl/") && strings.Contains(name, "/inbox/") {
		return false, fmt.Errorf("%w: %s", store.ErrUnreachable, name)
	}
	return u.Store.Write(ctx, name, value, how)
}

// jobsAt gives, for each runner that was sent anything, how many jobs.
func (w *world) jobsAt() map[string]int {
	at := map[string]int{}
	for _, name := range w.list("") {
		if runner, _, ok := strings.Cut(name, "/inbox/ctl/"); ok && runner != "ctl" {
			at[runner]++
		}
	}
	return at
}

func Test_ctl_attempt_goes_to_one_runner_when_runners_change(t *testing.T) {
	w := newWorld(t)
	w.enrol(w.party("r1", identity.Runner), permission.Layer{}, nil)
	const runs = 8
	var ids []string
	for range runs {
		ids = append(ids, w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil))
	}
	// Every run's first step is written, with the one runner there is, and
	// no job can be sent.
	inner := w.ctrl.Store
	for range runs + 1 {
		w.afresh(unsent{inner})
		w.ctrl.Pass(ctx)
	}
	for _, id := range ids {
		if step := w.latest(id); step == nil || step.State.Jobs["working"].Runner != "r1" {
			t.Fatalf("the first step of %s is %+v", id, step)
		}
	}
	if at := w.jobsAt(); len(at) != 0 {
		t.Fatalf("jobs were sent: %v", at)
	}
	// Seven more runners qualify by the time the entries are carried out.
	for i := 2; i <= 8; i++ {
		w.enrol(w.party(fmt.Sprintf("r%d", i), identity.Runner), permission.Layer{}, nil)
	}
	w.afresh(inner)
	w.settle()
	if at := w.jobsAt(); len(at) != 1 || at["r1"] != runs {
		t.Fatalf("with more runners by the time the entries were carried out, the jobs went to %v", at)
	}
	// Carried out again, with the runners there are now, nothing is sent
	// anywhere else.
	for _, id := range ids {
		w.again(id)
	}
	w.afresh(inner)
	w.settle()
	if at := w.jobsAt(); len(at) != 1 || at["r1"] != runs {
		t.Fatalf("carried out again, the jobs are at %v", at)
	}
}

func Test_role_two_copies_send_one_job_to_one_runner(t *testing.T) {
	// Two copies take the same step at the same moment. The first read the
	// runners when there was one; before its step is written, seven more
	// are enrolled and the second copy takes the step and carries it out.
	elsewhere := 0
	for round := range 8 {
		w := newWorld(t)
		w.enrol(w.party("r1", identity.Runner), permission.Layer{}, nil)
		run := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
		other := controller.New(w.ctl, w.ctrl.Store)
		other.Now = w.clock
		w.afresh(&meanwhile{Store: w.ctrl.Store,
			at: func(name string, how store.How) bool { return name == store.Step("ctl", run, 1) },
			then: func() {
				for i := 2; i <= 8; i++ {
					w.enrol(w.party(fmt.Sprintf("r%d", i), identity.Runner), permission.Layer{}, nil)
				}
				if _, err := other.Pass(ctx); err != nil {
					t.Error(err)
				}
			}})
		w.noted()
		w.pass()
		lost := false
		for _, op := range w.noted() {
			lost = lost || op.Name == store.Step("ctl", run, 1) && op.Act == store.ActAdd && !op.Done
		}
		if !lost {
			t.Fatalf("round %d: the first copy's step was not answered \"already there\"", round)
		}
		at := w.jobsAt()
		chosen := w.latest(run).State.Jobs["working"].Runner
		if len(at) != 1 || at[chosen] != 1 {
			t.Fatalf("round %d: the entry says the job goes to %s, and jobs are at %v", round, chosen, at)
		}
		if chosen != "r1" {
			elsewhere++
		}
		if head := w.head(run); head.Step != 1 || head.Jobs != 1 {
			t.Fatalf("round %d: the head is %+v", round, head)
		}
	}
	if elsewhere == 0 {
		t.Fatal("in eight rounds the second copy never chose another runner than the first would have")
	}
}

// An event too large to be a message, sent up to a parent or down to a
// child, fails the job it belongs to with a reason: the parent is told as
// it is of any failure, the child is stopped, and nothing is left waiting.
func TestEventTooLargeToSendFailsItsJob(t *testing.T) {
	const big = `{ send: { to: WHERE, event: big, data: "${'x'.repeat(300000)}" } }`
	for direction, chart := range map[string]string{
		"up": `
			statechart: parent
			initial: s
			states:
			  s:
			    job:
			      machine:
			        statechart: kid
			        initial: k
			        states:
			          k:
			            on: { poke: ` + strings.Replace(big, "WHERE", "parent", 1) + ` }
			    on: { poke: { send: { to: s, event: poke } } }
		`,
		"down": `
			statechart: parent
			initial: s
			states:
			  s:
			    job:
			      machine:
			        statechart: kid
			        initial: k
			        states:
			          k:
			            on: { big: k }
			    on: { poke: ` + strings.Replace(big, "WHERE", "s", 1) + ` }
		`,
	} {
		w := newWorld(t)
		run := w.submit(chart, nil, nil)
		w.settle()
		kid := w.below(run)
		if w.head(run).Status != instance.Running || w.head(kid).Status != instance.Running {
			t.Fatalf("%s: before the event the two are %s and %s", direction, w.head(run).Status, w.head(kid).Status)
		}
		w.client.Send(ctx, run, "poke", nil, 0)
		w.settle()
		last := w.latest(run)
		if head := w.head(run); head.Status != instance.Failed || last.State.Error == nil ||
			!strings.Contains(last.State.Error.Error, "the event big was too large to send") ||
			!strings.Contains(last.State.Error.Error, "pass large values as blobs") {
			t.Fatalf("%s: the parent is %s with %+v; its last step took %q", direction, head.Status, last.State.Error, events(last))
		}
		if head := w.head(kid); head.Status != instance.Cancelled {
			t.Fatalf("%s: the child is %s", direction, head.Status)
		}
		if left := append(w.list(store.Inbox("ctl")), w.wakes()...); len(left) != 0 {
			t.Fatalf("%s: this is left waiting: %v", direction, left)
		}
	}
}
