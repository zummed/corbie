package controller_test

import (
	"strings"
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

// What the controller writes to a run for a step that asked for something
// names that step: the note that a send was not delivered, and the failure
// of a call no run took.
func Test_inst_note_and_failed_call_name_the_step(t *testing.T) {
	w := newWorld(t)
	child := strings.Replace(talk, "\tlifetime: keyed\n", "", 1)
	sender := w.roomsWith(map[string]string{"/t/parent": `
		statechart: parent
		initial: a
		states:
		  a: { job: { machine: ./child.yaml } }
	`}, map[string]string{"child.yaml": child})
	parent := w.startAt(w.client, "/t/parent", client.Start{})
	w.settle()
	w.tell(sender, "poke", map[string]any{"at": "router", "run": w.child(parent), "text": "wrong"})
	w.settle()
	sending := 0
	for _, s := range w.steps(sender) {
		for _, ask := range s.Asks {
			if ask.Kind == instance.AskEvent && ask.Location == "/t/router" {
				sending = s.Number
			}
		}
	}
	if cause, _ := w.causeOf(sender, instance.KindNote); sending == 0 || cause != (instance.Cause{Run: sender, Step: sending}) {
		t.Fatalf("the note names %+v, not step %d of the sender", cause, sending)
	}

	w = newWorld(t)
	run := w.calls(map[string]string{"/t/lookup": strings.Replace(lookup, "statechart: lookup", "statechart: lookup\n\treceive: [nobody]", 1)}, nil)
	calling, failed := 0, instance.Cause{}
	for _, s := range w.steps(run) {
		for _, ask := range s.Asks {
			if ask.Kind == instance.AskCall {
				calling = s.Number
			}
		}
		for _, e := range s.Events {
			if strings.HasSuffix(e.Source, ".failed") {
				failed = e.Cause
			}
		}
	}
	if calling == 0 || failed != (instance.Cause{Run: run, Step: calling}) {
		t.Fatalf("the failed call names %+v, not step %d of the caller", failed, calling)
	}
}

// still is a singleton that waits.
const still = `
	statechart: still
	lifetime: singleton
	initial: w
	states:
	  w:
	    on: { go: over }
	  over: { end: done }
`

// A client's message that names a person leaves the person in the cause of
// what the message does: the start of a run, an event to it, an event sent
// to a key, a call into another client's tree and the singleton a submit
// starts. A message that names none leaves none.
func Test_inst_cause_names_the_person(t *testing.T) {
	w := newWorld(t)
	alice, bob := w.client.As("alice@example.com"), w.client.As("bob@example.com")
	files := map[string]string{"/t/front": front, "/t/helper": helper, "/t/room": talk, "/t/still": still,
		"/t/lookup": strings.Replace(waiter, "initial: w", "receive: [client:c2]\n\tinitial: w", 1)}
	if r := w.submitTree(alice, w.tree("/t", "/srv/t", files, nil)); !r.Taken {
		t.Fatalf("the tree was refused: %s", r.Refused)
	}
	w.settle()
	// The singleton the submit started.
	for run := range w.at("/t/still") {
		if cause, _ := w.causeOf(run, machine.Start); cause.Client != "c1" || cause.Person != "alice@example.com" {
			t.Errorf("the singleton's start names %+v", cause)
		}
	}
	if len(w.at("/t/still")) != 1 {
		t.Fatalf("the submit started %v at /t/still", w.at("/t/still"))
	}
	// A start, and an event to the run.
	run := w.startAt(alice, "/t/front", client.Start{})
	w.settle()
	if _, err := bob.Send(ctx, w.to(run), "go", "x", time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if cause, _ := w.causeOf(run, machine.Start); cause.Client != "c1" || cause.Message == "" || cause.Person != "alice@example.com" {
		t.Errorf("the start names %+v", cause)
	}
	if cause, _ := w.causeOf(run, "go"); cause.Client != "c1" || cause.Person != "bob@example.com" {
		t.Errorf("go names %+v", cause)
	}
	// What the run asked for names the run's step, and no person.
	if called := w.called(run); len(called) != 1 {
		t.Fatalf("the desk called %v", called)
	} else if cause, _ := w.causeOf(called[0], machine.Start); cause.Run != run || cause.Person != "" {
		t.Errorf("the called run's start names %+v", cause)
	}
	// An event sent to a key.
	key := "b"
	if _, err := bob.Send(ctx, machine.Address{At: "/t/room", Key: &key}, "said", map[string]any{"text": "hi"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	found := false
	for room := range w.at("/t/room") {
		if head := w.head(room); head.Key == key {
			found = true
			if cause, _ := w.causeOf(room, "said"); cause.Client != "c1" || cause.Person != "bob@example.com" {
				t.Errorf("the event sent to a key names %+v", cause)
			}
		}
	}
	if !found {
		t.Fatalf("no run has the key %s: %v", key, w.at("/t/room"))
	}
	// A call into another client's tree.
	carol := w.other("c2", "send:/t/**").As("carol@example.com")
	id, err := carol.Start(ctx, client.Start{At: "/t/lookup"})
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	r, err := carol.Receipt(ctx, strings.SplitN(id, "/", 2)[1])
	if err != nil || r == nil || r.Run == "" {
		t.Fatalf("the call's receipt is %+v, %v", r, err)
	}
	if cause, _ := w.causeOf(r.Run, machine.Start); cause.Client != "c2" || cause.Person != "carol@example.com" {
		t.Errorf("the called run's start names %+v", cause)
	}
	// A message that names no person, and a cancel that names one.
	plain := w.startAt(w.client, "/t/front", client.Start{})
	w.settle()
	if cause, _ := w.causeOf(plain, machine.Start); cause.Client != "c1" || cause.Person != "" {
		t.Errorf("a start with no person names %+v", cause)
	}
	if _, err := bob.Cancel(ctx, plain, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	steps := w.steps(plain)
	last := steps[len(steps)-1]
	if w.head(plain).Status != instance.Cancelled || len(last.Events) != 1 || last.Events[0].Cause.Client != "c1" || last.Events[0].Cause.Person != "bob@example.com" {
		t.Errorf("the cancel left the run %s, and its last step took %+v", w.head(plain).Status, last.Events)
	}
}

// A head is sealed to nobody, and names no person: the person of the
// message that started a run is in the run's record alone.
func Test_inst_head_names_no_person(t *testing.T) {
	w := newWorld(t)
	alice := w.client.As("alice@example.com")
	if r := w.submitTree(alice, w.tree("/t", "/srv/t", map[string]string{"/t/still": still, "/t/lookup": waiter}, nil)); !r.Taken {
		t.Fatalf("the tree was refused: %s", r.Refused)
	}
	run := w.startAt(alice, "/t/lookup", client.Start{})
	w.settle()
	runs := []string{run}
	for single := range w.at("/t/still") {
		runs = append(runs, single)
	}
	if len(runs) != 2 {
		t.Fatalf("the runs are %v", runs)
	}
	for _, run := range runs {
		if cause, _ := w.causeOf(run, machine.Start); cause.Person != "alice@example.com" {
			t.Errorf("the record of %s names %+v", run, cause)
		}
		head := w.head(run)
		if head.Cause.Client != "c1" || head.Cause.Message == "" || head.Cause.Person != "" {
			t.Errorf("the head of %s names %+v", run, head.Cause)
		}
	}
}
