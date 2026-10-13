package controller_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// events tells the names of a step's events, in order.
func events(step *instance.Step) string {
	var names []string
	for _, e := range step.Events {
		names = append(names, e.Name)
	}
	return strings.Join(names, " ")
}

func Test_ctl_start_taken_alone_before_other_messages(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	// Events are waiting with the start, before any pass.
	w.client.Send(ctx, run, "tick", nil, 0)
	w.client.Send(ctx, run, "tick", nil, 0)
	w.settle()
	steps := w.steps(run)
	if len(steps) != 2 || events(steps[0]) != "start" || events(steps[1]) != "tick tick" {
		t.Fatalf("the steps took %q and then %q", events(steps[0]), events(steps[len(steps)-1]))
	}
}

func Test_ctl_events_on_one_link_arrive_in_order_sent(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		statechart: ordered
		initial: s
		states:
		  s: { on: { a: s, b: s, c: s } }
	`, nil, nil)
	w.settle()
	for _, name := range []string{"a", "b", "c", "b", "a"} {
		w.client.Send(ctx, run, name, nil, 0)
	}
	w.settle()
	if got := events(w.latest(run)); got != "a b c b a" {
		t.Fatalf("the events arrived as %q", got)
	}
}

func Test_ctl_cancel_or_deadline_beats_events_in_one_step(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	event, _ := w.client.Send(ctx, run, "tick", nil, 0) // sorts ahead of the cancel
	w.client.Cancel(ctx, run, 0)
	w.settle()
	steps := w.steps(run)
	last := steps[len(steps)-1]
	if len(steps) != 2 || last.State.Status != instance.Cancelled || len(last.Events) != 1 {
		t.Fatalf("after an event and a cancel there are %d steps, the last %s with %q", len(steps), last.State.Status, events(last))
	}
	if r := w.receipt(event); r == nil || r.Taken {
		t.Fatalf("the event that sorted ahead of the cancel got %+v", r)
	}
}

func Test_ctl_cancel_reaches_child_not_yet_started(t *testing.T) {
	w := newWorld(t)
	run := w.submit(family, nil, nil)
	w.pass() // the parent's first step: the child's start is in the inbox
	if got := w.list(store.Inbox("ctl")); len(got) != 1 {
		t.Fatalf("after the parent's first step the inbox holds %v", got)
	}
	w.client.Cancel(ctx, run, 0)
	// Take the parent's cancel before the child has taken its start.
	w.settle()
	var child string
	for _, at := range w.list(store.Finished("ctl")) {
		if inst := store.InstanceOf("ctl", at); inst != run {
			child = inst
		}
	}
	if child == "" || w.head(child).Status != instance.Cancelled || w.head(run).Status != instance.Cancelled {
		t.Fatalf("the child (%q) was not cancelled with its parent", child)
	}
	if left := append(w.list(store.Inbox("ctl")), w.list(store.Running("ctl"))...); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func Test_ctl_first_step_without_head_is_finished(t *testing.T) {
	want, _ := play(t, 1<<40)
	// Stopped straight after writing the first step, before any head.
	if got, _ := play(t, 1); got != want {
		t.Fatalf("stopped after the first step's entry, the run went otherwise:\n%s", got)
	}
}

func Test_ctl_unfinished_step_finished_before_next_is_written(t *testing.T) {
	want, _ := play(t, 1<<40)
	for _, n := range []int64{2, 3, 4, 5, 6, 7} {
		if got, _ := play(t, n); got != want {
			t.Fatalf("stopped partway through carrying out a step (after %d writes), the run went otherwise:\n%s", n, got)
		}
	}
}

func Test_ctl_message_past_its_time_named_by_step_still_finishes_it(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	inner := w.ctrl.Store
	s := &stopping{Store: inner}
	s.left.Store(1) // the step's entry, and nothing after
	w.ctrl = controller.New(w.ctl, s)
	w.ctrl.Now = w.clock
	w.ctrl.Pass(ctx)
	// Away for longer than the start is good for.
	w.tick(2 * time.Hour)
	w.ctrl = controller.New(w.ctl, inner)
	w.ctrl.Now = w.clock
	w.settle()
	if head := w.head(run); head == nil || head.Status != instance.Running || head.Step != 1 {
		t.Fatalf("the unfinished first step was not finished: %+v", head)
	}
	if r := w.receipt(run[strings.IndexByte(run, '/')+1:]); r == nil || !r.Taken {
		t.Fatalf("the start's receipt is %+v", r)
	}
}

func Test_ctl_message_to_self_outlives_absent_controller(t *testing.T) {
	w := newWorld(t)
	run := w.submit(family, nil, nil)
	w.pass() // the child's start is written
	w.tick(72 * time.Hour)
	for range 6 {
		w.settle()
		w.tick(time.Second)
	}
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("after three days away the run is %+v", head)
	}
}

func Test_ctl_wake_for_something_no_longer_waited_on_does_nothing(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	// Wakes nobody set: for a timer, a job and an attempt it never had.
	for _, what := range []string{"timer.0.waiting~1", "due.work~1", "again.work~1", "nonsense"} {
		w.owner.Write(ctx, store.Wake("ctl", w.clock().Add(-time.Second), run, what), []byte("\n"), store.OnlyIfNew)
	}
	w.settle()
	if n := len(w.steps(run)); n != 1 {
		t.Fatalf("a wake for nothing led to a step: %d steps", n)
	}
	if left := w.wakes(); len(left) != 0 {
		t.Fatalf("wakes for nothing were left: %v", left)
	}
}

func Test_ctl_message_for_removed_instance_dropped(t *testing.T) {
	w := newWorld(t)
	gone := "c1/20200101T000000.000Z-0000000000"
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	w.leave(r1, "20261010T120000.000000Z-late", &instance.Message{Kind: instance.KindResult, Instance: gone,
		Result: &instance.Result{Job: "x", Status: instance.Done}})
	w.owner.Write(ctx, store.Wake("ctl", w.clock().Add(-time.Second), gone, "timer.x~1"), []byte("\n"), store.OnlyIfNew)
	message, _ := w.client.Send(ctx, gone, "tick", nil, 0)
	w.settle()
	if left := append(w.list(store.Inbox("ctl")), w.wakes()...); len(left) != 0 {
		t.Fatalf("messages and wakes for an instance that is gone were left: %v", left)
	}
	if r := w.receipt(message); r == nil || r.Taken || !strings.Contains(r.Refused, "no such run") {
		t.Fatalf("the client's message for a run that is gone got %+v", r)
	}
}

func Test_ctl_refusal_receipt_written_before_message_deleted(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	w.client.Send(ctx, run, "stop", nil, 0)
	w.settle()
	w.mu.Lock()
	w.ops = nil
	w.mu.Unlock()
	message, _ := w.client.Send(ctx, run, "tick", nil, 0) // for a run that has ended
	w.settle()
	receiptAt, deleteAt := -1, -1
	w.mu.Lock()
	for i, op := range w.ops {
		if op.Done && op.Act == store.ActAdd && strings.HasSuffix(op.Name, "/receipts/c1/"+message) {
			receiptAt = i
		}
		if op.Done && op.Act == store.ActDelete && strings.HasSuffix(op.Name, "/inbox/c1/"+message) {
			deleteAt = i
		}
	}
	w.mu.Unlock()
	if receiptAt < 0 || deleteAt < receiptAt {
		t.Fatalf("the receipt was written at %d and the message deleted at %d", receiptAt, deleteAt)
	}
}

func Test_ctl_repeated_message_recognised_by_fresh_controller(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	message, _ := w.client.Send(ctx, run, "tick", nil, 0)
	w.settle()
	// A controller started afresh, which remembers nothing, is given the
	// same message again; and the same start again.
	w.ctrl = controller.New(w.ctl, w.ctrl.Store)
	w.ctrl.Now = w.clock
	w.leave(w.c1, message, &instance.Message{Kind: instance.KindEvent, Instance: run, Event: &machine.Event{Name: "tick"}})
	w.leave(w.c1, run[strings.IndexByte(run, '/')+1:], &instance.Message{Kind: instance.KindStart, Instance: run,
		Start: &instance.Start{Definition: w.latest(run).State.Definition, Seed: []byte("another")}})
	w.settle()
	if head := w.head(run); head.Step != 2 {
		t.Fatalf("a repeated message led to another step: %+v", head)
	}
	if r := w.receipt(message); r == nil || !r.Taken {
		t.Fatalf("the receipt no longer reads as it did: %+v", r)
	}
}

// failingLists is a store that cannot list the trusted parties.
type failingLists struct {
	store.Store
	fail atomic.Bool
}

func (f *failingLists) List(ctx context.Context, under string) ([]string, error) {
	if f.fail.Load() && strings.HasSuffix(under, "/trust") {
		return nil, fmt.Errorf("%w: turned away", store.ErrUnreachable)
	}
	return f.Store.List(ctx, under)
}

func Test_ctl_unreadable_runner_ends_pass_not_run(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	f := &failingLists{Store: w.ctrl.Store}
	f.fail.Store(true)
	w.ctrl = controller.New(w.ctl, f)
	w.ctrl.Now = w.clock
	run := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
	if _, err := w.ctrl.Pass(ctx); err == nil {
		t.Fatal("a pass that could not read the runners ended as if nothing was wrong")
	}
	if w.head(run) != nil || len(w.steps(run)) != 0 {
		t.Fatal("something was written for a step that could not reach the store")
	}
	// The store answers again: the run goes on as it would have.
	f.fail.Store(false)
	w.settle()
	if head := w.head(run); head == nil || head.Status != instance.Running || head.Jobs != 1 {
		t.Fatalf("after the store answered again the run is %+v", head)
	}
	if jobs := w.list(store.Inbox("r1")); len(jobs) != 1 {
		t.Fatalf("the job was not sent: %v", jobs)
	}
}

func Test_ctl_step_that_cannot_reach_store_is_not_a_failed_step(t *testing.T) {
	want, _ := play(t, 1<<40)
	for n := int64(0); n < 12; n++ {
		got, _ := play(t, n)
		if strings.Contains(got, "failed") || got != want {
			t.Fatalf("a store out of reach after %d writes left the run otherwise:\n%s", n, got)
		}
	}
}

// counting counts how many requests a store has open at once.
type counting struct {
	store.Store
	open, most atomic.Int64
}

func (c *counting) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	n := c.open.Add(1)
	defer c.open.Add(-1)
	for {
		most := c.most.Load()
		if n <= most || c.most.CompareAndSwap(most, n) {
			break
		}
	}
	time.Sleep(50 * time.Microsecond)
	return c.Store.Write(ctx, name, value, how)
}

func Test_ctl_thousand_jobs_sent_in_bounded_batches(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	c := &counting{Store: w.ctrl.Store}
	w.ctrl = controller.New(w.ctl, c)
	w.ctrl.Now = w.clock
	// The flow type is not in this package's tests: a chart with many
	// named jobs asks for them all in one step.
	var runs strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&runs, "\t      r%d: ./work.js\n", i)
	}
	run := w.submit("\tstatechart: many\n\tinitial: s\n\tstates:\n\t  s:\n\t    jobs:\n"+runs.String(), map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	if jobs := w.list(store.Inbox("r1")); len(jobs) != 1000 {
		t.Fatalf("%d jobs were sent", len(jobs))
	}
	if head := w.head(run); head.Jobs != 1000 {
		t.Fatalf("the head counts %d jobs", head.Jobs)
	}
	if most := c.most.Load(); most > int64(w.ctrl.SideBySide) || most < 2 {
		t.Fatalf("a thousand jobs were written %d at a time; the bound is %d", most, w.ctrl.SideBySide)
	}
	// Carried out again, nothing is sent twice.
	before := w.list("r1")
	w.settle()
	if after := w.list("r1"); len(after) != len(before) {
		t.Fatalf("a second pass changed what the runner was sent: %d then %d", len(before), len(after))
	}
}

func Test_mach_undelivered_event_is_named_in_record(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		statechart: lonely
		initial: a
		states:
		  a:
		    on: { poke: { send: { to: b, event: hello } } }
		  b:
		    job: { machine: { statechart: kid, states: { k: { final: true } } } }
	`, nil, nil)
	w.settle()
	w.client.Send(ctx, run, "poke", nil, 0)
	w.settle()
	found := false
	for _, ask := range w.latest(run).Asks {
		if ask.Kind == instance.AskEvent && ask.Undelivered && ask.Event.Name == "hello" && ask.Name == "b" {
			found = true
		}
	}
	if !found || w.head(run).Status != instance.Running {
		t.Fatalf("an event for a child that is not running: noted %v, the chart is %s", found, w.head(run).Status)
	}
}

func Test_mach_key_not_derivable_from_public_names(t *testing.T) {
	w := newWorld(t)
	run := w.submit(door, nil, map[string]any{"greeting": "x"})
	w.settle()
	w.client.Send(ctx, run, "open", nil, 0)
	w.settle()
	var key string
	for _, k := range w.latest(run).State.Keys {
		key = k.Value
	}
	if len(key) < 20 {
		t.Fatalf("the key is %q", key)
	}
	for _, name := range w.list("") {
		raw, _, _ := w.owner.Read(ctx, name)
		if strings.Contains(name, key) || strings.Contains(string(raw), key) {
			t.Fatalf("the key shows in %s, which whoever runs the store can read", name)
		}
	}
	// Two runs of one chart get different keys.
	other := w.submit(door, nil, map[string]any{"greeting": "x"})
	w.settle()
	w.client.Send(ctx, other, "open", nil, 0)
	w.settle()
	for _, k := range w.latest(other).State.Keys {
		if k.Value == key {
			t.Fatal("two runs of one chart got the same key")
		}
	}
}

func Test_mach_emit_delivered_twice_is_taken_once(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.submit(`
		statechart: listens
		initial: working
		states:
		  working:
		    job: { script: ./work.js }
		    on: { page: { send: { to: parent, event: saw } } }
	`, map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	note := &instance.Message{Kind: instance.KindEmit, Instance: run, Emit: &instance.EmitNote{Job: "working", Name: "page", Index: 1, Data: "p1"}}
	w.leave(r1, "20261010T120001.000000Z-a", note)
	w.settle()
	// The message about it arrives again, under another name.
	w.leave(r1, "20261010T120002.000000Z-b", note)
	w.settle()
	saw := 0
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskEvent && ask.Event.Name == "saw" {
				saw++
			}
		}
	}
	if saw != 1 {
		t.Fatalf("an emitted value delivered twice was taken %d times", saw)
	}
}

// A job that cannot be built, whether it is a script or a child, fails,
// and the machine is told as it is of any failure: nothing is sent, and
// nothing is left waiting.
func TestJobThatCannotBeBuiltFails(t *testing.T) {
	w := newWorld(t)
	w.enrol(w.party("r1", identity.Runner), permission.Layer{Allow: []string{"all"}}, nil)
	const chart = `
		statechart: unbuilt
		initial: s
		states:
		  s:
		    job: JOB
		    done: end
		  end: { final: true }
	`
	for job, want := range map[string]string{
		`{ script: ./work.js, allow: ["secret:NONE@net:api.example"] }`:                                    "the secret NONE is not set",
		`{ machine: { statechart: kid, states: { k: { final: true } } }, input: "${'x'.repeat(300000)}" }`: "pass large values as blobs",
	} {
		id := w.submit(strings.Replace(chart, "JOB", job, 1), map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		last := w.latest(id)
		if head := w.head(id); head.Status != instance.Failed || last.State.Error == nil || !strings.Contains(last.State.Error.Error, want) {
			t.Fatalf("%s\n is %s with %v, want %q; step %d, events %s", job, head.Status, last.State.Error, want, last.Number, events(last))
		}
		if jobs := w.list(store.Inbox("r1")); len(jobs) != 0 {
			t.Fatalf("%d jobs were sent", len(jobs))
		}
	}
}

func Test_perm_layers_travel_apart(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{Allow: []string{"all"}}, nil)
	_, err := w.client.Start(ctx, w.draft(`
		statechart: parent
		initial: s
		states:
		  s:
		    job:
		      allow: ["fs.read:/srv/reports/**"]
		      machine:
		        statechart: kid
		        initial: k
		        states:
		          k:
		            job: { script: ./work.js, allow: ["fs.read:/srv/**"], deny: ["fs.read:/srv/private/**"] }
		            done: end
		          end: { final: true }
		    done: end
		  end: { final: true }
	`, map[string]string{"work.js": "return 1"}), client.Start{Allow: &permission.Layer{Allow: []string{"disk"}}})
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	jobs := w.list(store.Inbox("r1"))
	if len(jobs) != 1 {
		t.Fatalf("%d jobs were sent", len(jobs))
	}
	data, _, _ := w.owner.Read(ctx, jobs[0])
	value, err := identity.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var msg instance.Message
	if err := value.Open(r1, &msg); err != nil {
		t.Fatal(err)
	}
	// Each layer is there as it was written, under its own name, in the
	// order they were laid down; none has been narrowed by another.
	var got []string
	for _, layer := range msg.Job.Layers {
		got = append(got, fmt.Sprintf("%s: allow %v deny %v", layer.Name, layer.Allow, layer.Deny))
	}
	want := []string{
		"the job k in the definition of kid: allow [fs.read:/srv/**] deny [fs.read:/srv/private/**]",
		"the list the run was started with: allow [disk] deny []",
		"the job s in the definition of parent: allow [fs.read:/srv/reports/**] deny []",
		"the ceiling of the client c1: allow [all] deny []",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the job carries\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// A controller secret is kept sealed, and the value set last is the one a
// job that names it is sent. One that is not set fails the job before
// anything is sent.
func TestControllerSecret(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{Allow: []string{"all"}}, nil)
	uses := func() (failure string, sent []string) {
		run := w.submit(`
			statechart: uses
			initial: s
			states:
			  s:
			    job: { script: ./work.js, allow: ["secret:K@net:api.example"], attempts: 3 }
			    done: end
			  end: { final: true }
		`, map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		if e := w.latest(run).State.Error; e != nil {
			failure = e.Error
		}
		for _, name := range w.list(store.Inbox("r1")) {
			data, _, _ := w.owner.Read(ctx, name)
			var msg instance.Message
			if _, err := identity.Read(data, name, w.ctl.Card, r1, &msg); err != nil {
				t.Fatal(err)
			}
			sent = append(sent, msg.Job.Secrets["K"])
			w.owner.Delete(ctx, name, store.Overwrite)
		}
		return failure, sent
	}
	if failure, sent := uses(); !strings.Contains(failure, "the secret K is not set") || len(sent) != 0 {
		t.Fatalf("with the secret not set: %q, %d jobs sent", failure, len(sent))
	}
	for _, value := range []string{"the first value", "the second"} {
		if err := secret.Set(ctx, w.ctrl.Store, w.ctl, "K", value); err != nil {
			t.Fatal(err)
		}
		if failure, sent := uses(); failure != "" || !slices.Equal(sent, []string{value}) {
			t.Fatalf("with the secret set to %q: %q, and the job was sent %q", value, failure, sent)
		}
	}
	kept, _, _ := w.owner.Read(ctx, store.Secret("ctl", "K"))
	var value string
	if _, err := identity.Read(kept, store.Secret("ctl", "K"), w.ctl.Card, r1, &value); bytes.Contains(kept, []byte("the second")) || err == nil {
		t.Fatalf("the secret is kept in the clear, or a runner's key opens it (%v)", err)
	}
	if err := secret.Remove(ctx, w.ctrl.Store, w.ctl, "K"); err != nil {
		t.Fatal(err)
	}
	if failure, sent := uses(); !strings.Contains(failure, "the secret K is not set") || len(sent) != 0 {
		t.Fatalf("with the secret removed: %q, %d jobs sent", failure, len(sent))
	}
}

func Test_ctl_wake_time_fixed_by_entry(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		statechart: timed
		keep: 1d
		deadline: 2h
		initial: waiting
		states:
		  waiting:
		    after: { 1h: over }
		    on: { tick: { send: { to: parent, event: ticked } } }
		  over: { final: true }
	`, nil, nil)
	w.settle()
	first := w.list(store.Wakes("ctl"))
	// The first step asked for three wakes: its timer, its deadline, and
	// the one for its history.
	asked := 0
	for _, ask := range w.latest(run).Asks {
		if ask.Kind == instance.AskWake {
			asked++
			if _, mark, _ := w.owner.Read(ctx, store.Wake("ctl", ask.At, run, ask.What)); mark == store.None {
				t.Fatalf("the wake %s is not under the time its entry gives, %s", ask.What, ask.At)
			}
		}
	}
	if asked != 3 || len(first) != 4 {
		t.Fatalf("the step asked for %d wakes and the store holds %v", asked, first)
	}
	// The step is carried out again ten minutes later: its start is in the
	// inbox again, so a controller started afresh finishes it again.
	w.tick(10 * time.Minute)
	w.afresh(w.ctrl.Store)
	w.leave(w.c1, run[strings.IndexByte(run, '/')+1:], &instance.Message{Kind: instance.KindStart, Instance: run,
		Start: &instance.Start{Definition: w.latest(run).State.Definition}})
	w.settle()
	if again := w.list(store.Wakes("ctl")); !slices.Equal(again, first) {
		t.Fatalf("carried out again later, the wakes are\n%v\nand were\n%v", again, first)
	}

	// The wake for its history comes due, and is acted on twice, hours
	// apart: the next one is set once.
	trims := func() (names []string) {
		for _, name := range w.list(store.Wakes("ctl")) {
			if strings.HasSuffix(name, "/trim") {
				names = append(names, name)
			}
		}
		return names
	}
	w = newWorld(t)
	run = w.submit(kept, nil, nil)
	w.settle()
	due := trims()
	w.tick(24*time.Hour + time.Minute)
	w.settle()
	next := trims()
	if len(due) != 1 || len(next) != 1 || next[0] == due[0] {
		t.Fatalf("the wake for its history was %v and after a day is %v", due, next)
	}
	w.owner.Write(ctx, due[0], []byte("\n"), store.OnlyIfNew) // as if stopped before it was deleted
	w.tick(3 * time.Hour)
	w.afresh(w.ctrl.Store)
	w.settle()
	if again := trims(); !slices.Equal(again, next) {
		t.Fatalf("acted on a second time three hours later, the wakes for its history are %v, and were %v", again, next)
	}
}

// meanwhile is a store in which something else happens just before one
// write lands: another copy does its work between this copy's look and its
// write.
type meanwhile struct {
	store.Store
	at   func(name string, how store.How) bool
	then func()
	once sync.Once
}

func (m *meanwhile) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	if m.at(name, how) {
		m.once.Do(m.then)
	}
	return m.Store.Write(ctx, name, value, how)
}

func Test_ctl_head_write_that_finds_a_change_reads_again(t *testing.T) {
	// One copy has written step 2 and is about to bring the head to it.
	// Before its write lands, another copy brings the head to step 2 and
	// then to step 3.
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	other := controller.New(w.ctl, w.ctrl.Store)
	other.Now = w.clock
	slow := &meanwhile{Store: w.ctrl.Store,
		at: func(name string, how store.How) bool {
			return name == store.RunningHead("ctl", run) && how != store.OnlyIfNew
		},
		then: func() {
			w.client.Send(ctx, run, "tick", nil, 0)
			for range 3 {
				if _, err := other.Pass(ctx); err != nil {
					t.Error(err)
				}
			}
			if head := w.head(run); head.Step != 3 {
				t.Errorf("the other copy brought the head to %+v", head)
			}
		}}
	w.afresh(slow)
	w.client.Send(ctx, run, "tick", nil, 0)
	w.noted()
	w.pass()
	if head := w.head(run); head.Step != 3 {
		t.Fatalf("the copy that was behind put the head back to step %d", head.Step)
	}
	changed := 0
	for _, op := range w.noted() {
		if op.Name == store.RunningHead("ctl", run) && op.Act == store.ActRewrite && !op.Done {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("the slow copy's write of the head was answered \"changed\" %d times", changed)
	}

	// A copy is about to write the first head of a new instance. Before its
	// write lands, another copy takes the instance to its end: its head is
	// among the finished and gone from the running.
	w = newWorld(t)
	run = w.submit(kept, nil, nil)
	other = controller.New(w.ctl, w.ctrl.Store)
	other.Now = w.clock
	slow = &meanwhile{Store: w.ctrl.Store,
		at: func(name string, how store.How) bool { return name == store.RunningHead("ctl", run) },
		then: func() {
			w.client.Cancel(ctx, run, 0)
			for range 3 {
				if _, err := other.Pass(ctx); err != nil {
					t.Error(err)
				}
			}
			if head := w.head(run); head == nil || head.Status != instance.Cancelled || len(w.list(store.Running("ctl"))) != 0 {
				t.Errorf("the other copy left the head as %+v", head)
			}
		}}
	w.afresh(slow)
	w.pass()
	if left := w.list(store.Running("ctl")); len(left) != 0 {
		t.Fatalf("a head written after the instance had ended is left among the running: %v", left)
	}
	if head := w.head(run); head.Status != instance.Cancelled || head.Step != 2 {
		t.Fatalf("the head is %+v", head)
	}
}

// A job is sent the value of a secret it names only where every layer
// the controller knows of lets that secret travel at all. Where one does
// not, by having no entry for it or by denying it everywhere, nothing is
// sent and the job is denied: the machine is told which layer refused and
// what would fix it, as for any refusal.
func TestSecretALayerDoesNotAllowIsNotSentAndItsJobIsDenied(t *testing.T) {
	const alone = `
		statechart: uses
		initial: s
		states:
		  s:
		    job: { script: ./work.js, allow: ["net:api.example", "SECRET"]DENY }
		    done: end
		    failed: refused
		  end: { final: true }
		  refused: { final: true }
	`
	const below = `
		statechart: parent
		initial: s
		states:
		  s:
		    job:
		      allow: ["network"]
		      machine:
		        statechart: uses
		        initial: s
		        states:
		          s:
		            job: { script: ./work.js, allow: ["net:api.example", "SECRET"] }
		            done: end
		            failed: refused
		          end: { final: true }
		          refused: { final: true }
	`
	all := []string{"all"}
	groups := []string{"network", "disk", "exec", "env"}
	for _, c := range []struct {
		name            string
		chart           string
		entry, deny     string   // the job's own entry for the secret, and its deny list
		ceiling, denied []string // the client's ceiling
		start           []string // the list the run is started with
		layer, fix      string   // the layer that refuses and what it says to do; none where the value is sent
	}{
		{name: "every layer allows it", chart: alone, entry: "secret:K@net:api.example", ceiling: []string{"network", "secret:K@*"}},
		{name: "a ceiling of all", chart: alone, entry: "secret:K@*", ceiling: all},
		{name: "a layer covers it by a pattern", chart: alone, entry: "secret:K@net:api.example", ceiling: []string{"network", "secret:*@net:*.example"}},
		{name: "a layer denies it to one place only", chart: alone, entry: "secret:K@*", ceiling: all, denied: []string{"secret:K@net:other.example"}},
		{name: "the ceiling has no entry for it", chart: alone, entry: "secret:K@net:api.example", ceiling: []string{"network"},
			layer: "the ceiling of the client c1", fix: "the controller's owner runs: corbie controller enrol c1 --allow secret:K@net:api.example"},
		{name: "a ceiling of the four groups", chart: alone, entry: "secret:K@*", ceiling: groups,
			layer: "the ceiling of the client c1", fix: "the controller's owner runs: corbie controller enrol c1 --allow secret:K@*"},
		{name: "the start's list has no entry for it", chart: alone, entry: "secret:K@net:api.example", ceiling: all, start: []string{"network"},
			layer: "the list the run was started with", fix: "start the run with secret:K@net:api.example in its list as well (corbie submit --allow)"},
		{name: "a parent's job has no entry for it", chart: below, entry: "secret:K@net:api.example", ceiling: all,
			layer: "the job s in the definition of parent", fix: "add the line secret:K@net:api.example to the allow list of the job s in the definition of parent"},
		{name: "the ceiling denies it everywhere", chart: alone, entry: "secret:K@net:api.example", ceiling: all, denied: []string{"secret:K@*"},
			layer: "the ceiling of the client c1", fix: "take the deny entry secret:K@* out of the ceiling of the client c1"},
		{name: "the ceiling denies everything", chart: alone, entry: "secret:K@net:api.example", ceiling: all, denied: all,
			layer: "the ceiling of the client c1", fix: "take the deny entry all out of the ceiling of the client c1"},
		{name: "the job's own deny list stops it everywhere", chart: alone, entry: "secret:K@net:api.example", deny: `, deny: ["secret:*@*"]`, ceiling: all,
			layer: "the job s in the definition of uses", fix: "take the deny entry secret:*@* out of the job s in the definition of uses"},
		{name: "two layers refuse", chart: alone, entry: "secret:K@net:api.example", ceiling: []string{"network"}, start: []string{"all"}, deny: `, deny: ["all"]`,
			layer: "the job s in the definition of uses and by the ceiling of the client c1",
			fix:   "take the deny entry all out of the job s in the definition of uses; and the controller's owner runs: corbie controller enrol c1 --allow secret:K@net:api.example"},
	} {
		w := newWorld(t)
		r1 := w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{Allow: all}, nil)
		w.enrol(w.c1, permission.Layer{Allow: c.ceiling, Deny: c.denied}, nil)
		if err := secret.Set(ctx, w.ctrl.Store, w.ctl, "K", "the value"); err != nil {
			t.Fatal(err)
		}
		start := client.Start{}
		if c.start != nil {
			start.Allow = &permission.Layer{Allow: c.start}
		}
		chart := strings.NewReplacer("SECRET", c.entry, "DENY", c.deny).Replace(c.chart)
		run, err := w.client.Start(ctx, w.draft(chart, map[string]string{"work.js": "return 1"}), start)
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		if c.chart == below {
			run = w.below(run)
		}
		var sent []string
		for _, name := range w.list(store.Inbox("r1")) {
			msg, _ := w.sent(r1, name)
			sent = append(sent, msg.Job.Secrets["K"])
		}
		if c.layer == "" {
			if !slices.Equal(sent, []string{"the value"}) || w.head(run).Status != instance.Running {
				t.Errorf("%s: the run is %s, and its jobs were sent %q", c.name, w.head(run).Status, sent)
			}
			continue
		}
		if len(sent) != 0 {
			t.Errorf("%s: a job was sent, with %q for the secret", c.name, sent)
		}
		// The machine took the refusal as the failure of its job.
		var told map[string]any
		for _, step := range w.steps(run) {
			for _, e := range step.Events {
				if e.Name == machine.Failed && e.From == "s" {
					told, _ = e.Data.(map[string]any)
				}
			}
		}
		want := map[string]any{"status": machine.StatusDenied, "refused": c.entry, "layer": c.layer, "fix": c.fix,
			"error": c.entry + " was refused by " + c.layer + ". To allow it: " + c.fix}
		if fmt.Sprint(told) != fmt.Sprint(want) || w.head(run).Status != instance.Done {
			t.Errorf("%s: the run is %s, and the machine was told\n  %v\nwant\n  %v", c.name, w.head(run).Status, told, want)
		}
	}
}
