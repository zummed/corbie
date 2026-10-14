package controller_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// leave writes a message into the controller's inbox as a party.
func (w *world) leave(as *identity.Identity, name string, msg *instance.Message) {
	w.t.Helper()
	at := store.Message("ctl", as.Name, name)
	data, err := as.Write(at, w.clock(), w.clock().Add(time.Hour), msg, w.ctl.Card)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, at, data, store.OnlyIfNew); err != nil {
		w.t.Fatal(err)
	}
}

const oneJob = `
	statechart: one-job
	initial: working
	states:
	  working:
	    job: { script: ./work.js, timeout: 1m }
	    on:
	      done: finished
	      failed: broken
	  finished: { end: done, output: "${states.working.output}" }
	  broken: { end: done, output: "${event.data.status}" }
`

// A message that does not carry the part its kind says it does is no
// message: it is deleted, and the pass goes on.
func TestMessageWithoutItsPartIsDropped(t *testing.T) {
	w := newWorld(t)
	run := w.submit(door, nil, map[string]any{"greeting": "x"})
	w.settle()
	for i, kind := range []string{instance.KindResult, instance.KindEmit, instance.KindAnswer, instance.KindEvent, instance.KindStart, "nonsense"} {
		w.leave(w.c1, "20991231T000000.00000"+string(rune('0'+i))+"Z-bad", &instance.Message{Kind: kind, Instance: run})
	}
	w.settle()
	if got := w.list(store.Inbox("ctl")); len(got) != 0 {
		t.Fatalf("malformed messages were left in the inbox: %v", got)
	}
	if head := w.head(run); head.Status != instance.Running || head.Step != 1 {
		t.Fatalf("a malformed message moved the run: %+v", head)
	}
}

// A result from a runner the job was not sent to is dropped, and the job's
// due time still ends the wait.
func TestWrongRunnersResultDoesNotCancelTheDueTime(t *testing.T) {
	w := newWorld(t)
	r1, r2 := w.party("r1", identity.Runner), w.party("r2", identity.Runner)
	w.enrol(r1, permission.Layer{}, []string{"main"})
	w.enrol(r2, permission.Layer{}, nil)
	run := w.submit(strings.Replace(oneJob, "timeout: 1m", "timeout: 1m, tags: [main]", 1), map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	if jobs := w.list(store.Inbox("r1")); len(jobs) != 1 {
		t.Fatalf("the job was not sent to r1: %v", jobs)
	}
	// r2, which was sent no such job, writes a well-formed result for it.
	w.leave(r2, "20261010T120001.000000Z-forged", &instance.Message{Kind: instance.KindResult, Instance: run,
		Result: &instance.Result{Job: "working", Attempt: 1, Status: instance.Done, Output: "forged"}})
	w.tick(2 * time.Minute)
	w.settle()
	head := w.head(run)
	last := w.latest(run)
	if head.Status != instance.Done || last.State.Output != machine.StatusTimeout {
		t.Fatalf("after the due time the run is %s with %v", head.Status, last.State.Output)
	}
	// A cancel went to the runner the job was sent to.
	found := false
	for _, name := range w.list(store.Inbox("r1")) {
		found = found || strings.HasSuffix(name, ".cancel")
	}
	if !found {
		t.Fatal("no cancel was left for the runner whose job timed out")
	}
}

// A result that has arrived wins over its own due time, however late the
// controller is in looking.
func Test_ctl_waiting_result_beats_its_due_wake(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	w.leave(r1, "20261010T120030.000000Z-result", &instance.Message{Kind: instance.KindResult, Instance: run,
		Result: &instance.Result{Job: "working", Attempt: 1, Status: instance.Done, Output: "in time"}})
	w.tick(time.Hour) // the controller looks an hour later: both are waiting
	w.settle()
	if last := w.latest(run); last.State.Status != instance.Done || last.State.Output != "in time" {
		t.Fatalf("the run ended %s with %v", last.State.Status, last.State.Output)
	}
	if n := len(w.steps(run)); n != 2 {
		t.Fatalf("the record holds %d steps: a timeout was written beside the result", n)
	}
}

// A timer set again for the same moment as the one it replaces is still
// delivered.
func Test_ctl_wake_set_again_survives_delete_of_used_one(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		statechart: daily
		initial: waiting
		states:
		  waiting:
		    timers: { evening: "18:00" }
		    on:
		      evening: working
		      again: waiting
		  working: { end: done, output: fired }
	`, nil, nil)
	w.settle()
	// Leave the state and enter it again before its time: the timer is
	// cleared and set again for the very same moment.
	w.tick(time.Hour)
	w.client.Send(ctx, run, "again", nil, 0)
	w.settle()
	if head := w.head(run); head.Step != 2 || head.Timers != 1 {
		t.Fatalf("after entering again the head is %+v", head)
	}
	w.tick(6 * time.Hour) // past 18:00
	w.settle()
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("the timer set again was never delivered: %+v", head)
	}
	if left := w.wakes(); len(left) != 0 {
		t.Fatalf("wakes left behind: %v", left)
	}
}

// What a step refuses is answered only once the step's entry is the one
// that was written: the receipt follows the entry.
func Test_ctl_loser_of_step_carries_out_winners_entry(t *testing.T) {
	w := newWorld(t)
	run := w.submit(door, nil, map[string]any{"greeting": "x"})
	w.settle()
	w.client.Send(ctx, run, "open", nil, 0)
	w.settle()
	var ticket string
	for _, k := range w.latest(run).State.Tickets {
		ticket = k.Value
	}
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{}, nil)
	other := w.clientOf(c2)
	first, _ := w.client.Reply(ctx, run, ticket, 42, 0)
	second, _ := other.Reply(ctx, run, ticket, 42, 0)
	w.mu.Lock()
	w.ops = nil
	w.mu.Unlock()
	w.settle()
	if r := w.receipt(first); r == nil || !r.Taken {
		t.Fatalf("the first answer's receipt: %+v", r)
	}
	if r, _ := other.Receipt(ctx, second); r == nil || r.Taken || !strings.Contains(r.Refused, "already answered") {
		t.Fatalf("the second answer's receipt: %+v", r)
	}
	stepAt, receiptAt := -1, -1
	w.mu.Lock()
	for i, op := range w.ops {
		if op.Act != store.ActAdd || !op.Done {
			continue
		}
		if strings.Contains(op.Name, "/steps/") && stepAt < 0 {
			stepAt = i
		}
		if strings.Contains(op.Name, "/receipts/c2/") {
			receiptAt = i
		}
	}
	w.mu.Unlock()
	if stepAt < 0 || receiptAt < stepAt {
		t.Fatalf("the refusal's receipt was written at %d, the step at %d", receiptAt, stepAt)
	}
}

// A client's event sent again after it was taken gets the same answer and
// no second step.
func TestEventSentTwiceIsTakenOnce(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	message, _ := w.client.Send(ctx, run, "tick", nil, 0)
	w.settle()
	if head := w.head(run); head.Step != 2 {
		t.Fatalf("the event was not taken: %+v", head)
	}
	// The very same message is written again.
	w.leave(w.c1, message, &instance.Message{Kind: instance.KindEvent, Instance: run, Event: &machine.Event{Name: "tick"}})
	w.settle()
	if head := w.head(run); head.Step != 2 {
		t.Fatalf("the event sent again led to a second step: %+v", head)
	}
	if got := w.list(store.Inbox("ctl")); len(got) != 0 {
		t.Fatalf("the repeated message was left in the inbox: %v", got)
	}
}
