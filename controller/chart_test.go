package controller_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

const door = `
	statechart: door
	initial: closed
	states:
	  closed:
	    on: { open: opened }
	  opened:
	    ticket: code
	    entry: { send: { to: parent, event: want_code, data: { ticket: "${tickets.code}" } } }
	    timers: { too_long: 10s }
	    on:
	      code:
	        - { go: unlocked, when: "${event.data == 42}" }
	        - { go: closed }
	      too_long: timed_out
	  unlocked: { end: done, output: { opened: true, by: "${event.name}", said: "${input.greeting}" } }
	  timed_out: { end: failed, output: late }
`

// One run going through the store: started, sent an event, waiting on a
// ticket and a timer, answered, ended.
func TestRunThroughTheStore(t *testing.T) {
	w := newWorld(t)
	run := w.submit(door, nil, map[string]any{"greeting": "hello"})

	// Sent and not yet taken up: a message and the definition, no head.
	if w.head(run) != nil {
		t.Fatal("a run has a head before the controller has taken its start")
	}
	w.settle()
	head := w.head(run)
	if head == nil || head.Status != instance.Running || head.Step != 1 || head.Machine != "door" || head.Type != "statechart" {
		t.Fatalf("after the start the head is %+v", head)
	}
	if got := w.list(store.Inbox("ctl")); len(got) != 0 {
		t.Fatalf("the start is still in the inbox: %v", got)
	}

	// An event the chart has a rule for.
	message, err := w.client.Send(ctx, run, "open", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(message); r == nil || !r.Taken || r.Acted == nil || !*r.Acted {
		t.Fatalf("the receipt for open is %+v", r)
	}
	head = w.head(run)
	if head.Step != 2 || head.Timers != 1 || !reflect.DeepEqual(head.Tickets, []string{"code"}) {
		t.Fatalf("waiting, the head is %+v", head)
	}
	step := w.latest(run)
	var ticket string
	for _, ask := range step.Asks {
		if ask.Kind == instance.AskEvent && ask.Event.Name == "want_code" {
			ticket, _ = ask.Event.Data.(map[string]any)["ticket"].(string)
		}
	}
	if ticket == "" || step.State.Tickets["opened"].Value != ticket {
		t.Fatalf("the ticket the chart sent up is %q, its state holds %+v", ticket, step.State.Tickets)
	}
	if wakes := w.wakes(); len(wakes) != 1 {
		t.Fatalf("the wakes are %v", wakes)
	}

	// An event it has no rule for is dropped, and the receipt says so.
	message, _ = w.client.Send(ctx, run, "open", nil, 0)
	w.settle()
	if r := w.receipt(message); r == nil || !r.Taken || r.Acted == nil || *r.Acted {
		t.Fatalf("the receipt for an event with no rule is %+v", r)
	}

	// A wrong ticket is refused; the right one is taken, once.
	wrong, _ := w.client.Reply(ctx, run, "not-the-ticket", 42, 0)
	right, _ := w.client.Reply(ctx, run, ticket, 42, 0)
	again, _ := w.client.Reply(ctx, run, ticket, 7, 0)
	w.settle()
	if r := w.receipt(wrong); r == nil || r.Taken {
		t.Fatalf("the receipt for a wrong ticket is %+v", r)
	}
	if r := w.receipt(right); r == nil || !r.Taken {
		t.Fatalf("the receipt for the answer is %+v", r)
	}
	if r := w.receipt(again); r == nil || r.Taken {
		t.Fatalf("the receipt for a second answer is %+v", r)
	}

	head = w.head(run)
	if head.Status != instance.Done || head.Timers != 0 || len(head.Tickets) != 0 || head.Ended.IsZero() {
		t.Fatalf("at the end the head is %+v", head)
	}
	if got := w.list(store.Running("ctl")); len(got) != 0 {
		t.Fatalf("an ended run is still among the running: %v", got)
	}
	want := map[string]any{"opened": true, "by": "code", "said": "hello"}
	if got := w.latest(run).State.Output; !reflect.DeepEqual(got, want) {
		t.Fatalf("the output is %v, want %v", got, want)
	}
	// Nothing is left waiting.
	if got := append(w.list(store.Inbox("ctl")), w.wakes()...); len(got) != 0 {
		t.Fatalf("left behind: %v", got)
	}
}

func TestTimerEndsTheWait(t *testing.T) {
	w := newWorld(t)
	run := w.submit(door, nil, map[string]any{"greeting": "hi"})
	w.settle()
	w.client.Send(ctx, run, "open", nil, 0)
	w.settle()

	// A pass before the timer's time leaves it.
	w.tick(5 * time.Second)
	w.settle()
	if head := w.head(run); head.Status != instance.Running || head.Step != 2 {
		t.Fatalf("before its time the timer moved the chart: %+v", head)
	}
	w.tick(6 * time.Second)
	w.settle()
	head := w.head(run)
	if head.Status != instance.Failed || head.Failure != "error" {
		t.Fatalf("after the timer the head is %+v", head)
	}
	last := w.latest(run)
	if last.State.Output != "late" || last.State.Error == nil {
		t.Fatalf("a failed final state gave %v, %v", last.State.Output, last.State.Error)
	}
	// An answer after the end is refused.
	message, _ := w.client.Reply(ctx, run, "whatever", 1, 0)
	w.settle()
	if r := w.receipt(message); r == nil || r.Taken {
		t.Fatalf("an answer to an ended run got %+v", r)
	}
	if n := len(w.steps(run)); n != 3 {
		t.Fatalf("the run has %d steps, want 3", n)
	}
}
