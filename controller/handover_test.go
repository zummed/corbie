package controller_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// tally is a singleton that keeps what it is sent under set as the output
// of kept, and hands that over.
const tally = `
	statechart: tally
	lifetime: singleton
	handover: kept
	initial: idle
	states:
	  idle: { on: { set: kept } }
	  kept:
	    initial: k
	    states:
	      k: { end: done, output: "${event.data}" }
	    on: { done: idle }
`

// titled gives a definition with a title, so that a submit finds it changed.
func titled(src, title string) string {
	return strings.Replace(src, "initial: idle", "title: "+title+"\n\tinitial: idle", 1)
}

// deploy submits a singleton at /s and runs the controller until the run it
// starts, or starts again, has taken its first step. It gives that run.
func (w *world) deploy(src string) string {
	w.t.Helper()
	run := w.submitted(src)
	if w.head(run) == nil || w.head(run).Status != instance.Running {
		w.t.Fatalf("the run %s at /s is %+v", run, w.head(run))
	}
	return run
}

// submitted submits a singleton at /s, runs the controller for a while, and
// gives the run the submit starts.
func (w *world) submitted(src string) string {
	w.t.Helper()
	message, err := w.client.Submit(ctx, w.tree("/s", "", map[string]string{"/s": src}, nil), 0)
	if err != nil {
		w.t.Fatal(err)
	}
	for range 6 {
		w.settle()
		w.tick(time.Second)
	}
	return w.receipt(message).Tree.Locations[0].Run
}

// set sends a run set with some data, and runs the controller.
func (w *world) set(run string, data any) {
	w.t.Helper()
	if _, err := w.client.Send(ctx, w.to(run), "set", data, 0); err != nil {
		w.t.Fatal(err)
	}
	w.settle()
}

// A singleton started again is given what the run it replaces hands over:
// the latest output of the state its handover: names, or, where that state
// gave none, what that run was given.
func Test_ctl_handover_hands_over_what_the_run_was_given(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"n": 1.0})
	second := w.deploy(titled(tally, "two"))
	if got := w.latest(second).State.Input; !reflect.DeepEqual(got, map[string]any{"n": 1.0}) {
		t.Fatalf("the second run was given %v", got)
	}
	third := w.deploy(titled(tally, "three"))
	if got := w.latest(third).State.Input; !reflect.DeepEqual(got, map[string]any{"n": 1.0}) {
		t.Fatalf("the third run, in place of one that kept nothing, was given %v", got)
	}
	if notes := w.keyNotes(third); len(notes) != 0 {
		t.Fatalf("the third run's record says %v", notes)
	}
}

// A run started, not started again, is given no input: a singleton
// cancelled and then submitted starts empty.
func Test_ctl_cancelled_singleton_starts_empty(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"n": 1.0})
	if _, err := w.client.Cancel(ctx, first, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	second := w.deploy(titled(tally, "two"))
	if got := w.latest(second).State.Input; got != nil {
		t.Fatalf("the run started after a cancel was given %v", got)
	}
}

// What is handed over is named by the definition of the run replaced: a
// chart that gains handover: hands nothing over on that submit, and one
// that loses it hands over on that submit for the last time.
func Test_ctl_handover_is_named_by_the_run_replaced(t *testing.T) {
	w := newWorld(t)
	without := strings.Replace(tally, "\thandover: kept\n", "", 1)
	first := w.deploy(without)
	w.set(first, map[string]any{"n": 1.0})
	second := w.deploy(tally)
	if got := w.latest(second).State.Input; got != nil {
		t.Fatalf("the run in place of one with no handover: was given %v", got)
	}
	w.set(second, map[string]any{"n": 2.0})
	third := w.deploy(without)
	if got := w.latest(third).State.Input; !reflect.DeepEqual(got, map[string]any{"n": 2.0}) {
		t.Fatalf("the run in place of one with handover: was given %v", got)
	}
	fourth := w.deploy(titled(without, "four"))
	if got := w.latest(fourth).State.Input; got != nil {
		t.Fatalf("the run in place of one that lost handover: was given %v", got)
	}
}

// The new file's input: checks what the run is handed: a value that does
// not fit fails the run at its first step, with the value in its record,
// and the next submit starts the singleton with no input.
func Test_ctl_handover_is_checked_by_the_new_input(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"n": 1.0})
	second := w.submitted(strings.Replace(tally, "initial: idle", "input:\n\t  n: string?\n\tinitial: idle", 1))
	if head := w.head(second); head == nil || head.Status != instance.Failed {
		t.Fatalf("the run handed a value its input: does not fit is %+v", head)
	}
	if got := w.latest(second).State.Input; !reflect.DeepEqual(got, map[string]any{"n": 1.0}) {
		t.Fatalf("the failed run's record holds %v", got)
	}
	third := w.deploy(titled(tally, "three"))
	if got := w.latest(third).State.Input; got != nil {
		t.Fatalf("the run in place of one that failed was given %v", got)
	}
}

// A run whose handover: gives nothing hands over null, and the new run's
// record has a note naming it.
func Test_ctl_handover_of_nothing_is_noted(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	second := w.deploy(titled(tally, "two"))
	if got := w.latest(second).State.Input; got != nil {
		t.Fatalf("the second run was given %v", got)
	}
	if notes := w.keyNotes(second); len(notes) != 1 || !strings.Contains(notes[0], "the run "+first+" handed over nothing") {
		t.Fatalf("the second run's record says %v", notes)
	}
}

// The blobs a value handed over names are copied into the new run's record.
func Test_ctl_handover_copies_named_blobs(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	ref, err := blob.Write(ctx, w.owner, store.Blob("ctl", first, blob.NewName()), blob.TypeOf("seen.txt"), blob.NewKey(), strings.NewReader("a, b"))
	if err != nil {
		t.Fatal(err)
	}
	w.set(first, map[string]any{"seen": ref.Value()})
	second := w.deploy(titled(tally, "two"))
	input, _ := w.latest(second).State.Input.(map[string]any)
	copied, ok := blob.AsRef(input["seen"])
	if !ok || !strings.HasPrefix(copied.At, store.Blobs("ctl", second)+"/") {
		t.Fatalf("the second run was given %v", input)
	}
	if contents, err := blob.ReadAll(ctx, w.owner, copied, 100); err != nil || string(contents) != "a, b" {
		t.Fatalf("the copy holds %q, %v", contents, err)
	}
}

// A start taken in place of a start still waiting takes the runs that start
// replaces, so what they hand over is not lost with it.
func Test_ctl_handover_survives_a_start_taken_in_place_of_one_not_yet_taken(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"n": 1.0})
	// No start is taken for a while.
	inner := w.ctrl.Store
	w.afresh(&hiding{Store: inner, hide: func(name string) bool { return strings.HasSuffix(name, ".start") }})
	var messages []string
	for _, title := range []string{"two", "three"} {
		message, err := w.client.Submit(ctx, w.tree("/s", "", map[string]string{"/s": titled(tally, title)}, nil), 0)
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
		for range 4 {
			w.settle()
			w.tick(time.Second)
		}
	}
	left, third := w.receipt(messages[0]).Tree.Locations[0].Run, w.receipt(messages[1]).Tree.Locations[0].Run
	if w.head(first).Status != instance.Cancelled || w.head(left) != nil || w.head(third) != nil {
		t.Fatalf("with no start taken the runs at /s are %v", w.at("/s"))
	}
	w.afresh(inner)
	for range 4 {
		w.settle()
		w.tick(time.Second)
	}
	if w.head(left) != nil || w.head(third) == nil {
		t.Fatalf("the runs at /s are %v", w.at("/s"))
	}
	if got := w.latest(third).State.Input; !reflect.DeepEqual(got, map[string]any{"n": 1.0}) {
		t.Fatalf("the run started in place of a start not yet taken was given %v", got)
	}
}

// A run replaced whose record is gone hands over nothing, and the new run's
// record has a note naming it.
func Test_ctl_handover_from_a_record_gone_is_noted(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"n": 1.0})
	inner := w.ctrl.Store
	w.afresh(&hiding{Store: inner, hide: func(name string) bool { return strings.HasSuffix(name, ".start") }})
	second := w.submitted(titled(tally, "two"))
	if w.head(first).Status != instance.Cancelled || w.head(second) != nil {
		t.Fatalf("with no start taken the runs at /s are %v", w.at("/s"))
	}
	for _, name := range w.list(store.Record("ctl", first)) {
		if _, err := w.owner.Delete(ctx, name, store.Overwrite); err != nil {
			t.Fatal(err)
		}
	}
	w.afresh(inner)
	for range 4 {
		w.settle()
		w.tick(time.Second)
	}
	if got := w.latest(second).State.Input; got != nil {
		t.Fatalf("the run in place of one whose record is gone was given %v", got)
	}
	if notes := w.keyNotes(second); len(notes) != 1 || !strings.Contains(notes[0], "no record can be read of the run "+first) {
		t.Fatalf("the second run's record says %v", notes)
	}
}

// Only the controller starts a run in place of others: a client's start
// that names runs it replaces is refused, and hands it nothing.
func Test_ctl_client_start_replaces_no_run(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"secret": "c1's"})
	if _, err := w.client.Cancel(ctx, first, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	c2id := w.party("c2", identity.Client)
	w.enrol(c2id, permission.Layer{Allow: []string{"all"}}, nil)
	at, _ := w.place(w.clientOf(c2id), `
	statechart: probe
	initial: waiting
	states:
	  waiting: { on: { never: waiting } }
	`, nil)
	run, msg := w.start("c2", at)
	msg.Start.Replaces = []string{first}
	w.put(c2id, "x-start", msg, w.ctl.Card, w.clock().Add(time.Hour))
	w.settle()
	if w.head(run) != nil {
		t.Fatalf("a client's start naming %s was taken: %+v", first, w.latest(run).State.Input)
	}
	r, err := w.clientOf(c2id).Receipt(ctx, "x-start")
	if err != nil || r == nil || !strings.Contains(r.Refused, "a client's start replaces no run") {
		t.Fatalf("the start's receipt is %+v, %v", r, err)
	}
}
