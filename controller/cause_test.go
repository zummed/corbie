package controller_test

import (
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
)

// front is the transient file at /t/desk: told go, it calls /t/helper and
// then sends what it was given to the run of key a at /t/room.
const front = `
	statechart: front
	allow: ["send:**"]
	initial: idle
	states:
	  idle:
	    on: { go: asking }
	  asking:
	    job: { call: { at: helper }, input: "${event.data}" }
	    on:
	      done: { go: idle, send: { to: { at: room, key: a }, event: said, data: { text: hi } } }
`

// helper is the transient file at /t/helper: it runs a child that ends at
// once, and ends with it.
const helper = `
	statechart: helper
	initial: a
	states:
	  a:
	    job:
	      machine:
	        statechart: kid
	        initial: k
	        states:
	          k: { end: done }
	    on: { done: over }
	  over: { end: done }
`

// causeOf gives the cause of the first event of a name that a run took.
func (w *world) causeOf(run, event string) (instance.Cause, int) {
	w.t.Helper()
	for _, step := range w.steps(run) {
		for _, e := range step.Events {
			if e.Name == event {
				return e.Cause, step.Number
			}
		}
	}
	w.t.Fatalf("the run %s took no %s", run, event)
	return instance.Cause{}, 0
}

// Every event a step takes from another run names the run and the step that
// sent it, and one from a client names the client and its message. A client's
// message is followed through a call, a child and a send to its end, and the
// head of each run the request started names that cause.
func Test_inst_event_names_its_cause(t *testing.T) {
	w := newWorld(t)
	if r := w.submitTree(w.client, w.tree("/t", "/srv/t", map[string]string{"/t/front": front, "/t/helper": helper, "/t/room": talk}, nil)); !r.Taken {
		t.Fatalf("the tree was refused: %s", r.Refused)
	}
	run := w.startAt(w.client, "/t/front", client.Start{})
	w.settle()
	message, err := w.client.Send(ctx, w.to(run), "go", "x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	w.settle()

	// The client's message.
	if cause, _ := w.causeOf(run, machine.Start); cause.Client != "c1" || cause.Message == "" || cause.Run != "" {
		t.Fatalf("the desk's start names %+v", cause)
	}
	took, step := w.causeOf(run, "go")
	if took.Client != "c1" || took.Message != message {
		t.Fatalf("the desk took go from %+v, not the message %s", took, message)
	}
	// The call.
	called := w.called(run)
	if len(called) != 1 {
		t.Fatalf("the desk called %v", called)
	}
	if cause, _ := w.causeOf(called[0], machine.Start); cause != (instance.Cause{Run: run, Step: step}) {
		t.Fatalf("the called run's start names %+v, not step %d of the desk", cause, step)
	}
	if head := w.head(called[0]); head.Cause != (instance.Cause{Run: run, Step: step}) {
		t.Fatalf("the called run's head names %+v", head.Cause)
	}
	// The child, and its end.
	var kid string
	var asking int
	for _, s := range w.steps(called[0]) {
		for _, ask := range s.Asks {
			if ask.Kind == instance.AskChild {
				kid, asking = ask.Child, s.Number
			}
		}
	}
	if cause, _ := w.causeOf(kid, machine.Start); cause != (instance.Cause{Run: called[0], Step: asking}) {
		t.Fatalf("the child's start names %+v", cause)
	}
	if cause, _ := w.causeOf(called[0], machine.Done); cause != (instance.Cause{Run: kid, Step: w.head(kid).Step}) {
		t.Fatalf("the called run took the child's end from %+v", cause)
	}
	// The call's end, and the send it led to.
	ended, sending := w.causeOf(run, machine.Done)
	if ended != (instance.Cause{Run: called[0], Step: w.head(called[0]).Step}) {
		t.Fatalf("the desk took the call's end from %+v", ended)
	}
	rooms := w.runsOf("talk")
	if len(rooms) != 1 {
		t.Fatalf("the rooms are %v", rooms)
	}
	if cause, _ := w.causeOf(rooms[0], "said"); cause != (instance.Cause{Run: run, Step: sending}) {
		t.Fatalf("the room took said from %+v, not step %d of the desk", cause, sending)
	}
	// What the run's own job sent names no cause: its name is in from.
	for _, s := range w.steps(kid) {
		for _, e := range s.Events {
			if e.Name != machine.Start && e.Cause != (instance.Cause{}) {
				t.Fatalf("the child's %s names %+v", e.Name, e.Cause)
			}
		}
	}
}

// A cause a client writes in its message is not taken: the controller says
// what sent an event.
func Test_inst_client_cannot_write_its_cause(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.leave(w.c1, "20261010T120000.000000Z-forged", &instance.Message{Kind: instance.KindSend, To: &machine.Address{Run: sender},
		Cause: &instance.Cause{Run: "c1/20261010T115900.000Z-0123456789", Step: 9},
		Event: &machine.Event{Name: "stop"}})
	w.settle()
	if cause, _ := w.causeOf(sender, "stop"); cause.Run != "" || cause.Client != "c1" || cause.Message != "20261010T120000.000000Z-forged" {
		t.Fatalf("the run took stop from %+v", cause)
	}
}
