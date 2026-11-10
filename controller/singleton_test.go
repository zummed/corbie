package controller_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
)

// tally is a singleton that keeps what it is sent under set as the output
// of kept.
const tally = `
	statechart: tally
	lifetime: singleton
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

// A singleton started again is given no input, whatever the run it replaces
// held.
func Test_ctl_singleton_started_again_is_given_no_input(t *testing.T) {
	w := newWorld(t)
	first := w.deploy(tally)
	w.set(first, map[string]any{"n": 1.0})
	second := w.deploy(titled(tally, "two"))
	if w.head(first).Status != instance.Cancelled {
		t.Fatalf("the run replaced is %s", w.head(first).Status)
	}
	if got := w.latest(second).State.Input; got != nil {
		t.Fatalf("the run started again was given %v", got)
	}
}

// A start taken in place of a start still waiting is the one run started.
func Test_ctl_start_taken_in_place_of_one_not_yet_taken(t *testing.T) {
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
}

// Only the controller starts a run in place of others: a client's start
// that names runs it replaces is refused.
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
