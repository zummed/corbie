package controller_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// teller is the transient flow at /t/tell. Its first stage sends said to a
// chat at /t/room, and its second to /t/lost, where no singleton runs.
const teller = `
	flow: tell
	allow: ["send:**"]
	stages:
	  tell: { job: { send: { to: { at: room, key: "chat:1" }, event: said, data: { text: hello } } } }
	  lost: { job: { send: { to: { at: lost }, event: said, data: { text: lost } } } }
	output: { told: true }
`

// A send job ends done, with no output, once its event is written; one that
// is not delivered is noted in the record and ends done all the same.
func Test_ctl_send_job_is_done_once_written(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{"/t/tell": teller})
	w.noted()
	run := w.startAt(w.client, "/t/tell", client.Start{})
	w.settle()
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("the flow is %s: %+v", head.Status, w.latest(run).State.Error)
	}
	rooms := w.runsOf("talk")
	if len(rooms) != 1 || !slices.Equal(w.told(rooms[0]), []string{"hello"}) {
		t.Fatalf("the rooms are %v", rooms)
	}
	var sent, lost *instance.Ask
	for _, ask := range w.sentBy(run) {
		switch ask.Name {
		case "tell":
			sent = &ask
		case "lost":
			lost = &ask
		}
	}
	if sent == nil || sent.To != rooms[0] || sent.Undelivered {
		t.Fatalf("the record names the send %+v", sent)
	}
	if lost == nil || !lost.Undelivered || !strings.Contains(lost.Text, "no run of the singleton at /t/lost is running") {
		t.Fatalf("the record names the lost send %+v", lost)
	}
	done := map[string]bool{}
	for _, step := range w.steps(run) {
		for _, event := range step.Events {
			if event.Name == machine.Done && event.Data == nil {
				done[event.From] = true
			}
		}
	}
	if !done["tell"] || !done["lost"] {
		t.Fatalf("the stages ended done with no output: %v", done)
	}
	// The event is written before the job's result.
	event, result := -1, -1
	for i, op := range w.noted() {
		switch {
		case op.Act != store.ActAdd || !op.Done:
		case op.Name == store.Message("ctl", "ctl", sent.Message):
			event = i
		case op.Name == store.Message("ctl", "ctl", sent.Message+".done"):
			result = i
		}
	}
	if event < 0 || result < event {
		t.Fatalf("the event was written at %d and the result at %d", event, result)
	}
}

// busy is a keyed chat that keeps what is said while it works, until it is
// told free.
const busy = `
	statechart: talk
	lifetime: keyed
	initial: open
	states:
	  open: { on: { said: working } }
	  working:
	    defer: [said]
	    on: { free: open }
`

// Events a cancelled chart kept under defer: are noted as undelivered in
// the record of each run that sent one.
func Test_ctl_kept_events_of_a_cancelled_chart_are_noted(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/room": busy})
	for _, text := range []string{"one", "two", "three"} {
		w.say(sender, "say", 1, text)
		w.settle()
	}
	room := w.runsOf("talk")[0]
	if _, err := w.client.Cancel(ctx, room, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if head := w.head(room); head.Status != instance.Cancelled {
		t.Fatalf("the chat is %s", head.Status)
	}
	var kept []string
	for _, note := range w.keyNotes(sender) {
		if strings.Contains(note, "was cancelled while it kept the event under defer:") {
			kept = append(kept, note)
		}
	}
	if len(kept) != 2 || !strings.Contains(kept[0], "the event said sent to the run "+room+" at /t/room") {
		t.Fatalf("the sender's record says %v", w.keyNotes(sender))
	}
}

// desk is a singleton that keeps rang while it works.
const desk = `
	statechart: desk
	lifetime: singleton
	initial: open
	states:
	  open: { on: { rang: working } }
	  working:
	    defer: [rang]
	    on: { free: open }
`

// A singleton whose file changed is cancelled and started again on the new
// file, and the events it kept are noted to their senders.
func Test_ctl_restart_starts_a_singleton_again_and_notes_kept_events(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/desk": desk})
	old := w.runsOf("desk")[0]
	for range 3 {
		w.tell(sender, "ring", map[string]any{"at": "desk"})
		w.settle()
	}
	changed := strings.Replace(desk, "initial: open", "title: changed\n\tinitial: open", 1)
	r := w.submitTree(w.client, w.tree("/t", "/srv/t", map[string]string{"/t/router": router, "/t/room": talk, "/t/desk": changed}, nil))
	var did instance.Recorded
	for _, done := range r.Tree.Locations {
		if done.Location == "/t/desk" {
			did = done
		}
	}
	if did.What != "restarted" || !slices.Equal(r.Tree.Cancelled, []string{old}) {
		t.Fatalf("the submit did %+v, cancelling %v", did, r.Tree.Cancelled)
	}
	w.settle()
	if runs := w.at("/t/desk"); runs[old] != instance.Cancelled || runs[did.Run] != instance.Running || w.latest(did.Run).State.Title != "changed" {
		t.Fatalf("the runs at /t/desk are %v", runs)
	}
	var kept []string
	for _, note := range w.keyNotes(sender) {
		if strings.Contains(note, "kept the event under defer:") {
			kept = append(kept, note)
		}
	}
	if len(kept) != 2 {
		t.Fatalf("the sender's record says %v", w.keyNotes(sender))
	}
}
