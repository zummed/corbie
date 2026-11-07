package controller_test

import (
	"slices"
	"strings"
	"testing"
	"time"

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
	if lost == nil || !lost.Undelivered || !strings.Contains(lost.Text, "nothing is recorded at /t/lost") {
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

// A send job whose event is too large to write fails, saying so, and
// nothing reaches the receiver.
func Test_ctl_send_job_too_large_to_write_fails(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{"/t/tell": `
		flow: tell
		allow: ["send:**"]
		stages:
		  big: { job: { send: { to: { at: room, key: "chat:1" }, event: said, data: { text: "${'x'.repeat(300000)}" } } } }
	`})
	run := w.startAt(w.client, "/t/tell", client.Start{})
	w.settle()
	last := w.latest(run)
	if head := w.head(run); head.Status != instance.Failed || last.State.Error == nil ||
		!strings.Contains(last.State.Error.Error, "the event said could not be sent: it is too large") {
		t.Fatalf("the flow is %s with %+v", head.Status, last.State.Error)
	}
	if rooms := w.runsOf("talk"); len(rooms) != 0 {
		t.Fatalf("the event started the rooms %v", rooms)
	}
}

// A flow's approve: [send:…] holds a send job to that location for its
// ask: the event is written once the ask approves the job, and never when it
// approves nothing.
func Test_ctl_approve_send_holds_a_send_job(t *testing.T) {
	const tell = `
		flow: tell
		allow: ["send:**"]
		approve: ["send:/t/room"]
		ask: ./ask.js
		stages:
		  tell: { job: { send: { to: { at: room, key: "chat:1" }, event: said, data: { text: hello } } } }
	`
	for _, approve := range []bool{true, false} {
		w := newWorld(t)
		r1 := w.newRunner()
		w.roomsWith(map[string]string{"/t/tell": tell}, map[string]string{"ask.js": "return input"})
		run := w.startAt(w.client, "/t/tell", client.Start{})
		w.settle()
		var holds int
		for _, step := range w.steps(run) {
			for _, ask := range step.Asks {
				if ask.Kind == instance.AskHold && ask.Job.IsSend() {
					holds++
				}
			}
		}
		if holds != 1 || len(w.sentBy(run)) != 0 || len(w.runsOf("talk")) != 0 {
			t.Fatalf("the send job was held %d times and sent %v", holds, w.sentBy(run))
		}
		approved := []any{}
		if approve {
			approved = append(approved, "tell")
		}
		w.result(r1, run, instance.AskName, instance.Result{Status: instance.Done, Output: map[string]any{"approved": approved}})
		w.settle()
		rooms := w.runsOf("talk")
		if !approve {
			if head := w.head(run); len(w.sentBy(run)) != 0 || len(rooms) != 0 || head.Status != instance.Failed {
				t.Fatalf("not approved, the job sent %v, reached %v and the flow is %s", w.sentBy(run), rooms, head.Status)
			}
			continue
		}
		if head := w.head(run); head.Status != instance.Done {
			t.Fatalf("after the ask the flow is %s: %+v", head.Status, w.latest(run).State.Error)
		}
		if len(rooms) != 1 || !slices.Equal(w.told(rooms[0]), []string{"hello"}) {
			t.Fatalf("after the ask the rooms are %v", rooms)
		}
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

// An event a parent sent down that its cancelled child chart kept is noted
// as undelivered in the parent's record.
func Test_ctl_kept_events_a_parent_sent_are_noted_to_it(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
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
		            defer: [poke]
		            on: { go: j }
		          j: { on: { poke: j } }
		    on: { poke: { send: { to: s, event: poke } }, failed: t, done: t }
		  t: { on: { never: t } }
	`, nil, nil)
	w.settle()
	kid := w.below(run)
	w.client.Send(ctx, w.to(run), "poke", nil, 0)
	w.settle()
	if _, err := w.client.Cancel(ctx, kid, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if head := w.head(kid); head.Status != instance.Cancelled {
		t.Fatalf("the child is %s", head.Status)
	}
	notes := w.keyNotes(run)
	if len(notes) != 1 || !strings.Contains(notes[0], "the event poke sent to the run "+kid) ||
		!strings.Contains(notes[0], "was cancelled while it kept the event under defer:") {
		t.Fatalf("the parent's record says %v", notes)
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

// A send job's event that the receiver refuses is noted in the flow's
// record, though the flow ended once the event was written: the note is
// written after its end, and the flow stays done.
func Test_ctl_undelivered_note_reaches_an_ended_sender(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{
		"/t/poke": `
			flow: poke
			allow: ["send:**"]
			stages:
			  poke: { job: { send: { to: { at: picky, key: k1 }, event: said, data: { text: hi } } } }
		`,
		"/t/picky": strings.Replace(talk, "statechart: talk", "statechart: picky\n\treceive: [client:nobody]", 1),
	})
	run := w.startAt(w.client, "/t/poke", client.Start{})
	w.settle()
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("the flow is %s", head.Status)
	}
	notes := w.keyNotes(run)
	if len(notes) != 1 || !strings.Contains(notes[0], "the event said sent to /t/picky#k1 was not delivered: /t/picky does not receive said") {
		t.Fatalf("the ended flow's record says %v", notes)
	}
	steps := w.steps(run)
	if last := steps[len(steps)-1]; last.State.Status != instance.Done {
		t.Fatalf("the note's step left the flow %s", last.State.Status)
	}
}

// An ended run whose record has a note after its end takes what arrives
// later as any ended run does: a cancel and an event are refused, a second
// note is written after the first, and the head follows the latest step.
func Test_ctl_ended_run_with_a_note_takes_what_comes_later(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{
		"/t/poke": `
			flow: poke
			allow: ["send:**"]
			stages:
			  poke: { job: { send: { to: { at: picky, key: k1 }, event: said, data: { text: hi } } } }
		`,
		"/t/picky": strings.Replace(talk, "statechart: talk", "statechart: picky\n\treceive: [client:nobody]", 1),
	})
	run := w.startAt(w.client, "/t/poke", client.Start{})
	w.settle()
	if notes := w.keyNotes(run); len(notes) != 1 {
		t.Fatalf("the ended flow's record says %v", notes)
	}
	if _, err := w.client.Cancel(ctx, run, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	w.put(w.ctl, "second.note", &instance.Message{Kind: instance.KindNote, Instance: run, From: run, Note: "a second note"}, w.ctl.Card, time.Time{})
	w.settle()
	if _, err := w.client.Send(ctx, machine.Address{Run: run}, "late", nil, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	notes := w.keyNotes(run)
	if len(notes) != 2 || notes[1] != "a second note" {
		t.Fatalf("the ended flow's record says %v", notes)
	}
	steps := w.steps(run)
	last := steps[len(steps)-1]
	if head := w.head(run); head.Status != instance.Done || head.Step != last.Number || last.State.Status != instance.Done {
		t.Fatalf("the head is at step %d and %s; the latest step is %d and %s", head.Step, head.Status, last.Number, last.State.Status)
	}
}

// A held call or send job is shown to the ask as what it is: the location a
// call starts, or where a send goes, its event and its data.
func Test_ctl_ask_is_told_a_held_call_or_send(t *testing.T) {
	const tell = `
		flow: tell
		allow: ["send:**"]
		approve: ["send:**"]
		ask: ./ask.js
		stages:
		  c: { job: { call: { at: lookup }, input: { n: 5 } } }
		  s: { job: { send: { to: { at: room, key: "chat:1" }, event: said, data: { text: hello } } } }
	`
	w := newWorld(t)
	r1 := w.newRunner()
	w.roomsWith(map[string]string{"/t/tell": tell, "/t/lookup": lookup}, map[string]string{"ask.js": "return input"})
	w.startAt(w.client, "/t/tell", client.Start{})
	w.settle()
	var jobs []any
	for _, msg := range w.inboxOf(r1) {
		if in, _ := msg.Job.Input.(map[string]any); in["jobs"] != nil {
			jobs, _ = in["jobs"].([]any)
		}
	}
	got := map[string]map[string]any{}
	for _, job := range jobs {
		j := job.(map[string]any)
		got[j["name"].(string)] = j
	}
	call, _ := got["c"]["call"].(map[string]any)
	send, _ := got["s"]["send"].(map[string]any)
	to, _ := send["to"].(map[string]any)
	if call["at"] != "/t/lookup" || got["c"]["script"] != nil || to["at"] != "/t/room" || to["key"] != "chat:1" || send["event"] != "said" || got["s"]["script"] != nil {
		t.Fatalf("the ask was told %v", jobs)
	}
}
