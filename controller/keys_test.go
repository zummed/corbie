package controller_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// talk is a run a key names. It ends when it is told bye.
const talk = `
	statechart: talk
	initial: open
	states:
	  open:
	    on:
	      said:
	        - { go: over, when: "${event.data.text == 'bye'}" }
	        - { go: open }
	  over: { end: done, output: "${run.key}" }
`

// router sends what it is told to the run of a chat.
const router = `
	statechart: router
	initial: ready
	states:
	  ready:
	    on:
	      say: { send: { to: { machine: ./talk.yaml, key: "chat:${event.data.chat}" }, event: said, data: "${event.data}" } }
	      tell: { send: { to: { key: "chat:${event.data.chat}" }, event: said, data: "${event.data}" } }
	      stop: gone
	  gone: { end: done }
`

var routed = map[string]string{"talk.yaml": talk}

// say has a client send a router an event.
func (w *world) say(run, event string, chat int, text string) {
	w.t.Helper()
	if _, err := w.client.Send(ctx, run, event, map[string]any{"chat": float64(chat), "text": text}, time.Hour); err != nil {
		w.t.Fatal(err)
	}
}

// runsOf gives the runs of a machine, running or ended, sorted by what each
// was told.
func (w *world) runsOf(name string) []string {
	w.t.Helper()
	var runs []string
	for _, at := range append(w.list(store.Running("ctl")), w.list(store.Finished("ctl"))...) {
		inst := store.InstanceOf("ctl", at)
		if head := w.head(inst); head != nil && head.Machine == name && !slices.Contains(runs, inst) {
			runs = append(runs, inst)
		}
	}
	slices.SortFunc(runs, func(a, b string) int {
		return strings.Compare(strings.Join(w.told(a), " "), strings.Join(w.told(b), " "))
	})
	return runs
}

// told gives the text of each event of the name said a run was given, in
// the order it was given them.
func (w *world) told(run string) []string {
	w.t.Helper()
	var texts []string
	for _, step := range w.steps(run) {
		for _, event := range step.Events {
			if data, ok := event.Data.(map[string]any); ok && event.Name == "said" {
				texts = append(texts, fmt.Sprint(data["text"]))
			}
		}
	}
	return texts
}

// entry reads a key's entry.
func (w *world) entry(client, key string) *instance.KeyFile {
	w.t.Helper()
	entry, _, err := instance.ReadKey(ctx, w.owner, w.ctl.Card, w.ctl, store.ClientKey("ctl", client, key), key)
	if err != nil {
		w.t.Fatal(err)
	}
	return entry
}

// write signs a key's entry, as a controller stopped at the right moment
// leaves it.
func (w *world) writeEntry(client, key, run, start string) {
	w.t.Helper()
	at := store.ClientKey("ctl", client, key)
	data, err := w.ctl.Write(at, w.now, time.Time{}, instance.KeyFile{Key: key, Run: run, Start: start}, w.ctl.Card, w.c1.Card)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, at, data, store.Overwrite); err != nil {
		w.t.Fatal(err)
	}
}

// keyNotes gives what a run's record says of its sends to keys that failed.
func (w *world) keyNotes(run string) []string {
	w.t.Helper()
	var notes []string
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			switch {
			case ask.Kind == instance.AskNote:
				notes = append(notes, ask.Text)
			case ask.Kind == instance.AskEvent && ask.Undelivered && ask.Name == "" && ask.Text != "":
				notes = append(notes, ask.Text)
			case ask.Kind == instance.AskEvent && ask.Undelivered && ask.Name == "":
				notes = append(notes, "undelivered "+ask.Key)
			}
		}
	}
	return notes
}

// A send with a machine starts a run with the key when no run has it, gives
// it the event's data as its input and then the event, and joins that run
// after. The run is a root of the sender's client that records its sender.
func Test_chart_send_to_a_key_starts_a_run_then_delivers(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "say", 1, "two")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
	run := runs[0]
	if got := w.told(run); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("the run was told %v", got)
	}
	head, st := w.head(run), w.latest(run).State
	if head.Key != "chat:1" || head.Parent != "" || head.StartedBy != sender || store.InstanceClient(run) != "c1" {
		t.Fatalf("the head is %+v", head)
	}
	if st.Above.Parent != "" || st.Above.StartedBy != sender || st.Above.Depth != 1 {
		t.Fatalf("it runs under %+v", st.Above)
	}
	if in, _ := st.Input.(map[string]any); in["text"] != "one" {
		t.Fatalf("its input is %v", st.Input)
	}
	if entry := w.entry("c1", "chat:1"); entry == nil || entry.Run != run {
		t.Fatalf("the entry is %+v", entry)
	}
	for _, step := range w.steps(run) {
		for _, event := range step.Events {
			if event.Name == "said" && event.From != sender {
				t.Fatalf("the event is from %q", event.From)
			}
		}
	}
}

// A send to a key with no machine reaches the run with the key, and one that
// names no run is written in the sender's record and fails nothing.
func Test_chart_send_to_a_key_no_run_has_is_undelivered(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "tell", 1, "nobody hears this")
	w.settle()
	if got := w.keyNotes(sender); !slices.Equal(got, []string{"undelivered chat:1"}) || w.head(sender).Status != instance.Running {
		t.Fatalf("the record says %v, and the run is %s", got, w.head(sender).Status)
	}
	if got := w.runsOf("talk"); len(got) != 0 {
		t.Fatalf("the send started %v", got)
	}
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "tell", 1, "two")
	w.settle()
	if got := w.told(w.runsOf("talk")[0]); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("the run was told %v", got)
	}
}

// A key is free again once its run ends.
func Test_ctl_key_is_free_once_its_run_ends(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "bye")
	w.settle()
	if entry := w.entry("c1", "chat:1"); entry != nil {
		t.Fatalf("the entry of a run that ended is %+v", entry)
	}
	w.say(sender, "say", 1, "again")
	w.settle()
	if runs := w.runsOf("talk"); len(runs) != 2 {
		t.Fatalf("the runs are %v", runs)
	}
}

// A key is any text: it is in no name in the store, however it is written.
func Test_store_a_key_is_in_no_name(t *testing.T) {
	w := newWorld(t)
	key := "../a/b:c d/" + strings.Repeat("é", 200)
	w.holding(w.client, key)
	found := w.entry("c1", key)
	if found == nil {
		t.Fatal("the key has no entry")
	}
	if head := w.head(found.Run); head.Key != key {
		t.Fatalf("the head says %q", head.Key)
	}
	sum := sha256.Sum256([]byte(key))
	if name := store.ClientKey("ctl", "c1", key); name != "ctl/keys/c1/"+hex.EncodeToString(sum[:]) {
		t.Fatalf("the key's entry is under %s", name)
	}
	for _, name := range w.list("ctl") {
		if strings.Contains(name, "é") {
			t.Fatalf("a name holds the key: %s", name)
		}
	}
}

// A client's keys name only that client's runs.
func Test_ctl_a_key_names_only_runs_of_its_client(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"all"}}, nil)
	theirs := w.holding(w.clientOf(c2), "chat:1")
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "tell", 1, "not for them")
	w.say(sender, "say", 1, "mine")
	w.settle()
	if got := w.told(theirs); len(got) != 0 {
		t.Fatalf("another client's run was told %v", got)
	}
	if got := w.keyNotes(sender); !slices.Equal(got, []string{"undelivered chat:1"}) {
		t.Fatalf("the record says %v", got)
	}
	var mine []string
	for _, run := range w.runsOf("talk") {
		if run != theirs {
			mine = append(mine, run)
		}
	}
	if len(mine) != 1 || store.InstanceClient(mine[0]) != "c1" {
		t.Fatalf("the runs are %v", mine)
	}
}

// waits is a machine that waits for an event.
const waits = `
	statechart: waits
	initial: open
	states:
	  open: { on: { finish: over } }
	  over: { end: done }
`

// holding gives a run of waits that holds a key of a client: the child of
// that client's flow, whose job gives it the key.
func (w *world) holding(c *client.Client, key string) string {
	w.t.Helper()
	at, _ := w.place(c, `
		flow: holds
		stages:
		  a: { job: { machine: ./waits.yaml, key: "${input.key}" } }
	`, map[string]string{"waits.yaml": waits})
	parent := w.startAt(c, at, client.Start{Input: map[string]any{"key": key}})
	w.settle()
	return w.child(parent)
}

// A job with a key whose key is in use fails, with a reason. A run with the
// key from a job is an ordinary child, cancelled with its job.
func Test_ctl_job_with_a_key_in_use_fails(t *testing.T) {
	w := newWorld(t)
	files := map[string]string{"waits.yaml": waits}
	flow := `
		flow: asks
		stages:
		  a: { job: { machine: ./waits.yaml, key: "${input.key}" } }
	`
	first := w.submit(flow, files, map[string]any{"key": "k"})
	w.settle()
	second := w.submit(flow, files, map[string]any{"key": "k"})
	w.settle()
	st := w.latest(second).State
	if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "in use") {
		t.Fatalf("the second run is %s: %+v", st.Status, st.Error)
	}
	child := w.child(first)
	if head := w.head(child); head.Key != "k" || head.Parent != first {
		t.Fatalf("the child's head is %+v", head)
	}
	if _, err := w.client.Cancel(ctx, first, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if head := w.head(child); head.Status != instance.Cancelled {
		t.Fatalf("the child is %s", head.Status)
	}
	if entry := w.entry("c1", "k"); entry != nil {
		t.Fatalf("the entry of a child that ended is %+v", entry)
	}
}

// A key past the bound, or empty, fails the send or the job with a reason in
// the sender's record, and nothing is written to the store. The sender goes
// on: a key can come from outside.
func Test_mach_key_past_its_bound_writes_nothing(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(`
		statechart: s
		initial: ready
		states:
		  ready:
		    on:
		      go: { send: { to: { machine: ./waits.yaml, key: "${event.data}" }, event: x } }
	`, map[string]string{"waits.yaml": waits}, nil)
	w.settle()
	before := w.list("ctl/keys")
	for _, key := range []any{strings.Repeat("a", 257), "", nil, map[string]any{"a": 1.0}} {
		if _, err := w.client.Send(ctx, sender, "go", key, time.Hour); err != nil {
			t.Fatal(err)
		}
		w.settle()
	}
	notes := w.keyNotes(sender)
	if len(notes) != 4 || !strings.Contains(notes[0], "at most 256") || !strings.Contains(notes[1], "not empty") {
		t.Fatalf("the record says %v", notes)
	}
	if got := w.list("ctl/keys"); !slices.Equal(got, before) || len(w.runsOf("waits")) != 0 || w.head(sender).Status != instance.Running {
		t.Fatalf("the keys are %v and the runs %v", got, w.runsOf("waits"))
	}
	if _, err := w.client.Send(ctx, sender, "go", strings.Repeat("é", 256), time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if got := w.runsOf("waits"); len(got) != 1 {
		t.Fatalf("256 characters started %v", got)
	}

	flow := `
		flow: f
		stages:
		  a: { job: { machine: ./waits.yaml, key: "${input.key}" } }
	`
	run := w.submit(flow, map[string]string{"waits.yaml": waits}, map[string]any{"key": ""})
	w.settle()
	if st := w.latest(run).State; st.Status != instance.Failed || !strings.Contains(st.Error.Error, "not empty") {
		t.Fatalf("a job with an empty key: %s %+v", st.Status, st.Error)
	}
}

// A run's data that does not fit its machine's input fails the send in the
// sender's record. The key stays free.
func Test_ctl_send_whose_data_does_not_fit_fails_in_the_senders_record(t *testing.T) {
	w := newWorld(t)
	strict := strings.Replace(talk, "initial: open", "input: { count: number }\n\tinitial: open", 1)
	sender := w.submit(router, map[string]string{"talk.yaml": strict}, nil)
	w.settle()
	w.say(sender, "say", 1, "no count in this")
	w.settle()
	notes := w.keyNotes(sender)
	if len(notes) != 1 || !strings.Contains(notes[0], "chat:1") || !strings.Contains(notes[0], "count") {
		t.Fatalf("the record says %v", notes)
	}
	if entry := w.entry("c1", "chat:1"); entry != nil {
		t.Fatalf("the key is held by %+v", entry)
	}
	if left := w.list(store.Inbox("ctl")); len(left) != 0 {
		t.Fatalf("left waiting: %v", left)
	}
}

// Whatever its type, a run a send starts has the event's data as its input,
// and an event sent on to a type that does not act on it is ignored.
func Test_ctl_send_starts_a_machine_of_any_type(t *testing.T) {
	w := newWorld(t)
	w.newRunner()
	sender := w.submit(router, map[string]string{"talk.yaml": `
		flow: talk
		stages:
		  a: { job: { script: ./step.js, input: "${input}" } }
	`, "step.js": "return input"}, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "say", 1, "two")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
	st := w.latest(runs[0]).State
	if in, _ := st.Input.(map[string]any); in["text"] != "one" || st.Status != instance.Running {
		t.Fatalf("the flow has the input %v and is %s", st.Input, st.Status)
	}
	if got := w.told(runs[0]); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("the flow was given %v", got)
	}
}

// Stopped after any single write, with a run ending as events are sent to its
// key, every event reaches a run once and in the order it was sent.
func Test_ctl_key_stopped_after_every_write(t *testing.T) {
	want, _ := playKeys(t, 1<<40)
	if want != "[one bye] [two three]" {
		t.Fatalf("a run never stopped was told %s", want)
	}
	for n := int64(0); ; n++ {
		got, stopped := playKeys(t, n)
		if !stopped {
			if n < 20 {
				t.Fatalf("the workload took only %d writes", n)
			}
			return
		}
		if got != want {
			t.Fatalf("stopped after %d writes the runs were told %s, not %s", n, got, want)
		}
	}
}

func playKeys(t *testing.T, stopAfter int64) (string, bool) {
	t.Helper()
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	// Three in one step of the sender: the first ends the run the other two
	// are sent to.
	w.say(sender, "say", 1, "bye")
	w.say(sender, "say", 1, "two")
	w.say(sender, "say", 1, "three")
	inner := w.ctrl.Store
	s := &stopping{Store: inner}
	s.left.Store(stopAfter)
	w.ctrl = controller.New(w.ctl, s)
	w.ctrl.Now = w.clock
	for range 12 {
		if _, err := w.ctrl.Pass(ctx); err != nil {
			break
		}
	}
	w.afresh(inner)
	w.settle()
	var told []string
	for _, run := range w.runsOf("talk") {
		told = append(told, fmt.Sprint(w.told(run)))
	}
	if left := w.list(store.Inbox("ctl")); len(left) != 0 {
		t.Fatalf("left waiting: %v", left)
	}
	if entry := w.entry("c1", "chat:1"); entry == nil || w.head(entry.Run).Status != instance.Running {
		t.Fatalf("the key's entry is %+v", entry)
	}
	return strings.Join(told, " "), s.stopped.Load()
}

// Two senders race to start one key: one run starts, and each event reaches
// it. The loser's copy of the definition stays until housekeeping.
func Test_ctl_two_senders_race_for_one_key(t *testing.T) {
	w := newWorld(t)
	first := w.submit(router, routed, nil)
	second := w.submit(router, routed, nil)
	w.settle()
	w.say(first, "say", 1, "a")
	w.say(second, "say", 1, "b")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
	got := w.told(runs[0])
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("the run was told %v", got)
	}
}

// Two senders race, stopped after any single write: one run starts, and each
// event is given to it once.
func Test_ctl_two_senders_stopped_after_every_write(t *testing.T) {
	for n := int64(0); ; n++ {
		w := newWorld(t)
		first := w.submit(router, routed, nil)
		second := w.submit(router, routed, nil)
		w.settle()
		w.say(first, "say", 1, "a")
		w.say(second, "say", 1, "b")
		inner := w.ctrl.Store
		s := &stopping{Store: inner}
		s.left.Store(n)
		w.ctrl = controller.New(w.ctl, s)
		w.ctrl.Now = w.clock
		for range 12 {
			if _, err := w.ctrl.Pass(ctx); err != nil {
				break
			}
		}
		w.afresh(inner)
		w.settle()
		runs := w.runsOf("talk")
		if len(runs) != 1 {
			t.Fatalf("stopped after %d writes the runs are %v", n, runs)
		}
		got := w.told(runs[0])
		slices.Sort(got)
		if !slices.Equal(got, []string{"a", "b"}) {
			t.Fatalf("stopped after %d writes the run was told %v", n, got)
		}
		if !s.stopped.Load() {
			if n < 10 {
				t.Fatalf("the workload took only %d writes", n)
			}
			return
		}
	}
}

// Two controllers at once race the same way.
func Test_ctl_two_copies_race_for_one_key(t *testing.T) {
	for range 5 {
		w := newWorld(t)
		first := w.submit(router, routed, nil)
		second := w.submit(router, routed, nil)
		w.settle()
		w.say(first, "say", 1, "a")
		w.say(second, "say", 1, "b")
		other := controller.New(w.ctl, w.ctrl.Store)
		other.Now = w.clock
		done := make(chan error)
		go func() { _, err := other.Pass(ctx); done <- err }()
		if _, err := w.ctrl.Pass(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		w.settle()
		runs := w.runsOf("talk")
		if len(runs) != 1 {
			t.Fatalf("the runs are %v", runs)
		}
		got := w.told(runs[0])
		slices.Sort(got)
		if !slices.Equal(got, []string{"a", "b"}) {
			t.Fatalf("the run was told %v", got)
		}
	}
}

// An event sent with a machine to a run that ended before it took the event
// starts a run. One sent with no machine is noted as undelivered in the
// sender's record.
func Test_ctl_event_for_a_run_that_ended_starts_anew_or_is_noted(t *testing.T) {
	for _, c := range []struct {
		send string
		runs int
	}{{"say", 2}, {"tell", 1}} {
		w := newWorld(t)
		sender := w.submit(router, routed, nil)
		w.settle()
		w.say(sender, "say", 1, "one")
		w.settle()
		w.say(sender, "say", 1, "bye")
		w.say(sender, c.send, 1, "late")
		w.settle()
		if runs := w.runsOf("talk"); len(runs) != c.runs {
			t.Fatalf("%s: the runs are %v", c.send, runs)
		}
		if notes := w.keyNotes(sender); c.send == "tell" && (len(notes) != 1 || !strings.Contains(notes[0], "said")) {
			t.Fatalf("the record says %v", notes)
		}
	}
}

// An event with no machine for a run that has ended goes to the run that has
// the key by then.
func Test_ctl_event_with_no_machine_goes_to_the_run_that_has_the_key_now(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "say", 1, "bye")
	w.say(sender, "say", 1, "two")
	w.say(sender, "tell", 1, "three")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 2 || !slices.Equal(w.told(runs[1]), []string{"two", "three"}) {
		t.Fatalf("the runs are %v, and were told %v", runs, w.told(runs[len(runs)-1]))
	}
}

// An event on its way to another run is taken before a later one from the
// same sender to the same key, which it would otherwise be taken for a copy
// of, and lost.
func Test_ctl_event_on_its_way_is_not_overtaken(t *testing.T) {
	for range 8 {
		w := newWorld(t)
		sender := w.submit(router, routed, nil)
		w.settle()
		// The sender reads an entry its run has left behind, and its event goes
		// to a run that has ended.
		ended := w.submit(waits, nil, nil)
		w.settle()
		if _, err := w.client.Send(ctx, ended, "finish", nil, time.Hour); err != nil {
			t.Fatal(err)
		}
		w.settle()
		w.writeEntry("c1", "chat:1", ended, "ctl/inbox/ctl/gone")
		w.say(sender, "say", 1, "first")
		w.pass()
		// The entry goes before the sender's next event: that one starts a run.
		if _, err := w.owner.Delete(ctx, store.ClientKey("ctl", "c1", "chat:1"), store.Overwrite); err != nil {
			t.Fatal(err)
		}
		w.say(sender, "say", 1, "second")
		w.settle()
		runs := w.runsOf("talk")
		if len(runs) != 1 {
			t.Fatalf("the runs are %v", runs)
		}
		if got := w.told(runs[0]); !slices.Equal(got, []string{"first", "second"}) {
			t.Fatalf("the run was told %v", got)
		}
	}
}

// An entry a crash left, naming a run that has ended or that never began,
// blocks nothing.
func Test_ctl_stale_key_file_blocks_no_key(t *testing.T) {
	gone := "c1/20261010T120000.000Z-0000000000"
	for name, plant := range map[string]func(w *world, sender string){
		"a run that has ended": func(w *world, _ string) {
			w.say(w.submit(router, routed, nil), "stop", 1, "")
			run := w.submit(waits, nil, nil)
			w.settle()
			if _, err := w.client.Send(ctx, run, "finish", nil, time.Hour); err != nil {
				w.t.Fatal(err)
			}
			w.settle()
			w.writeEntry("c1", "chat:1", run, "ctl/inbox/ctl/gone")
		},
		"a run that never began": func(w *world, _ string) {
			w.writeEntry("c1", "chat:1", gone, "ctl/inbox/ctl/gone")
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			sender := w.submit(router, routed, nil)
			w.settle()
			plant(w, sender)
			w.say(sender, "tell", 1, "nobody")
			w.settle()
			if notes := w.keyNotes(sender); len(notes) != 1 {
				t.Fatalf("the record says %v", notes)
			}
			w.say(sender, "say", 1, "first")
			w.settle()
			runs := w.runsOf("talk")
			if len(runs) != 1 || w.head(runs[0]).Status != instance.Running || w.head(runs[0]).Key != "chat:1" {
				t.Fatalf("the send with a machine started %v", runs)
			}
			w.say(sender, "tell", 1, "now")
			w.settle()
			if got := w.told(runs[0]); !slices.Equal(got, []string{"first", "now"}) {
				t.Fatalf("the run was told %v", got)
			}
		})
	}
}

// A start that holds the key and has not taken its first step keeps it: its
// message is still waiting, and a crash left it so. An event sent to the key
// goes to that run, and starts no other.
func Test_ctl_key_file_of_a_start_still_waiting_is_kept(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	at, _ := w.place(w.client, waits, nil)
	first := w.startAt(w.client, at, client.Start{Life: time.Hour})
	_, id, _ := strings.Cut(first, "/")
	w.writeEntry("c1", "chat:1", first, store.Message("ctl", "c1", id))
	w.say(sender, "say", 1, "one")
	w.settle()
	if runs := w.runsOf("talk"); len(runs) != 0 || w.head(first) == nil || !slices.Equal(w.told(first), []string{"one"}) {
		t.Fatalf("the send started %v, and the first run has the head %+v and was told %v", runs, w.head(first), w.told(first))
	}
}

// A run started by a send outlives its sender with what it was given: its
// definition, the machines and scripts it names, the blobs in the event, and
// the ask handed down to it are all in its own record.
func Test_ctl_keyed_root_outlives_its_sender(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	inner := `
		statechart: inner
		initial: reading
		states:
		  reading: { job: { script: ./read.js, input: "${input}" }, on: { done: over } }
		  over: { end: done }
	`
	keeper := `
		statechart: keeper
		initial: open
		states:
		  open: { on: { go: reading } }
		  reading: { job: { machine: ./inner.yaml, input: "${input}" }, on: { done: asking } }
		  asking: { job: { script: ./read.js, approve: true }, on: { done: over } }
		  over: { end: done }
	`
	at, _ := w.place(w.client, `
		statechart: sender
		keep: 1h
		ask: ./ok.js
		input: { doc: blob }
		initial: ready
		states:
		  ready:
		    on:
		      go: { go: gone, send: { to: { machine: ./keeper.yaml, key: k }, event: hello, data: { doc: "${input.doc}" } } }
		  gone: { end: done }
	`, map[string]string{"keeper.yaml": keeper, "inner.yaml": inner, "read.js": "return input", "ok.js": "return { approve: input.jobs.map(j => j.name) }"})
	sender := w.startAt(w.client, at, client.Start{Input: map[string]any{"doc": client.InputFile{Name: "a.txt", Contents: strings.NewReader("what it was given")}}})
	w.settle()
	if _, err := w.client.Send(ctx, sender, "go", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	runs := w.runsOf("keeper")
	if len(runs) != 1 || w.head(sender).Status != instance.Done {
		t.Fatalf("the runs are %v and the sender is %s", runs, w.head(sender).Status)
	}
	run := runs[0]
	st := w.latest(run).State
	if st.Definition.Instance() != run {
		t.Fatalf("the definition is in the record of %s", st.Definition.Instance())
	}
	if st.Keep != machine.DefaultKeep {
		t.Fatalf("the run is kept for %s", st.Keep)
	}
	if st.Above.Ask == nil || st.Above.Ask.Definition.Instance() != run {
		t.Fatalf("the ask handed down is %+v", st.Above.Ask)
	}

	// The sender goes, record and all.
	w.tick(2 * time.Hour)
	w.settle()
	if left := w.list(store.Record("ctl", sender)); len(left) != 0 || w.head(sender) != nil {
		t.Fatalf("the sender's record holds %v", left)
	}

	if _, err := w.client.Send(ctx, run, "go", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	child := w.child(run)
	var job *instance.Job
	for _, msg := range w.inboxOf(r1) {
		if msg.Job != nil && msg.Job.Instance == child {
			job = msg.Job
		}
	}
	if job == nil {
		t.Fatalf("no job of the machine below it was sent: %v", w.list(store.Inbox("r1")))
	}
	if job.Script.Instance() != run {
		t.Fatalf("the script is in the record of %s", job.Script.Instance())
	}
	in, _ := job.Input.(map[string]any)
	ref, ok := blob.AsRef(in["doc"])
	if !ok || ref.Instance() != run {
		t.Fatalf("the job's input is %v", job.Input)
	}
	data, err := blob.ReadAll(ctx, w.owner, ref, 1<<20)
	if err != nil || string(data) != "what it was given" {
		t.Fatalf("the blob reads %q, %v", data, err)
	}

	// What it holds goes to the ask of a sender that is gone.
	w.result(r1, child, "reading", instance.Result{Status: instance.Done, Output: 1})
	w.settle()
	found := false
	for _, msg := range w.inboxOf(r1) {
		if msg.Job != nil && msg.Job.Instance == run && strings.HasPrefix(msg.Job.Name, instance.AskName) {
			found = msg.Job.Script.Instance() == run
		}
	}
	if !found {
		t.Fatalf("the ask was not sent: %+v", w.latest(run).Asks)
	}
}

// A chain of sends that each start a run ends at the depth limit of the root
// it began at: a run started by a send sits one level below its sender.
func Test_ctl_chain_of_sends_ends_at_the_depth_limit(t *testing.T) {
	w := newWorld(t)
	files := map[string]string{}
	hop := func(n int) string {
		return fmt.Sprintf(`
			statechart: hop%d
			initial: go
			states:
			  go: { entry: { send: { to: { machine: ./hop%d.yaml, key: "hop%d" }, event: hop } } }
		`, n, n+1, n+1)
	}
	for n := 1; n <= 6; n++ {
		files[fmt.Sprintf("hop%d.yaml", n)] = dedent(hop(n))
	}
	files["hop7.yaml"] = "statechart: hop7\ninitial: go\nstates:\n  go: { on: { x: go } }\n"
	root := strings.Replace(hop(0), "initial: go", "limits: { depth: 3 }\n\t\t\tinitial: go", 1)
	w.submit(root, files, nil)
	w.settle()
	started := 0
	for _, at := range w.list(store.Running("ctl")) {
		head := w.head(store.InstanceOf("ctl", at))
		if strings.HasPrefix(head.Machine, "hop") {
			started++
		}
		if head.Machine == "hop3" {
			if notes := w.keyNotes(head.Name); len(notes) != 1 || !strings.Contains(notes[0], "depth limit") {
				t.Fatalf("the last of the chain says %v", notes)
			}
		}
		if head.Machine == "hop4" {
			t.Fatal("the chain went past the limit")
		}
	}
	if started != 4 {
		t.Fatalf("%d runs started", started)
	}
}

// A send to a key costs one read of its entry, and never lists the store: a
// pass lists its inbox and its wakes and nothing else.
func Test_ctl_send_to_a_key_lists_nothing(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "tell", 1, "two")
	w.noted()
	w.settle()
	reads := 0
	for _, op := range w.noted() {
		if strings.Contains(op.Name, "/keys/") && op.Act == store.ActList {
			t.Fatalf("a pass listed %s", op.Name)
		}
		if op.Act == store.ActList && op.Name != store.Inbox("ctl") && op.Name != store.Wakes("ctl") {
			t.Fatalf("a pass listed %s", op.Name)
		}
		if strings.Contains(op.Name, "/keys/") && op.Act == store.ActRead {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("a send read the key's entry %d times", reads)
	}
}

// A machine takes all the events waiting in one step, so messages that arrive
// together are sent on together, however many there are. Their keys are read
// side by side, so a step that fits in a pass needs no more than a pass.
func Test_ctl_step_sends_to_any_number_of_keys(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	for chat := range 100 {
		w.say(sender, "say", chat, "one")
	}
	inner := w.ctrl.Store
	w.ctrl = controller.New(w.ctl, &delayedKeys{Store: inner})
	w.ctrl.Now = w.clock
	limited, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := w.ctrl.Pass(limited); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	w.ctrl = controller.New(w.ctl, inner)
	w.ctrl.Now = w.clock
	w.settle()
	if st := w.latest(sender).State; st.Status != instance.Running {
		t.Fatalf("the sender ended %s: %+v", st.Status, st.Error)
	}
	if got := len(w.runsOf("talk")); got != 100 {
		t.Fatalf("%d runs started", got)
	}
}

// delayedKeys takes 50 ms over each read of a key's file.
type delayedKeys struct{ store.Store }

func (s *delayedKeys) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if strings.Contains(name, "/keys/") {
		time.Sleep(50 * time.Millisecond)
	}
	return s.Store.Read(ctx, name)
}

// A machine made while running talks only to its parent and its children,
// whatever the job that starts it lists. That job may give it a key.
func Test_ctl_made_machine_uses_no_key(t *testing.T) {
	const job = `
		flow: maker
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}", allow: ["all"], KEY } }
	`
	for name, made := range map[string]string{
		"send": `
			statechart: made
			initial: a
			states:
			  a: { entry: { send: { to: { key: secret }, event: forged, data: 1 } } }
		`,
		"job": `
			flow: made
			stages:
			  a: { job: { machine: { flow: inner, stages: { b: { job: ./step.js } } }, key: secret } }
		`,
	} {
		w := newWorld(t)
		target := w.holding(w.client, "secret")
		r1 := w.newRunner()
		run := w.submit(strings.Replace(job, ", KEY", "", 1), map[string]string{"make.js": "return 1"}, nil)
		w.settle()
		w.made(r1, run, made)
		w.settle()
		st := w.latest(w.child(run)).State
		if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "sends to no key") {
			t.Fatalf("%s: the machine is %s: %+v", name, st.Status, st.Error)
		}
		if steps := w.steps(target); len(steps) != 1 {
			t.Fatalf("%s: the run with the key took %d steps", name, len(steps))
		}
	}

	w := newWorld(t)
	r1 := w.newRunner()
	run := w.submit(strings.Replace(job, "KEY", "key: mine", 1), map[string]string{"make.js": "return 1"}, nil)
	w.settle()
	w.made(r1, run, waits)
	w.settle()
	if entry := w.entry("c1", "mine"); entry == nil || entry.Run != w.child(run) {
		t.Fatalf("the key names %+v", entry)
	}
}

// Each run a send starts has a seed of its own, so no two issue one ticket,
// though one sender started them from one definition.
func Test_ctl_keyed_runs_of_one_sender_share_no_ticket(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, map[string]string{"talk.yaml": `
		statechart: talk
		initial: open
		states:
		  open: { ticket: verdict, on: { verdict: over } }
		  over: { end: done }
	`}, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.say(sender, "say", 2, "two")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 2 {
		t.Fatalf("the runs are %v", runs)
	}
	first, second := w.latest(runs[0]).State.Tickets["open"], w.latest(runs[1]).State.Tickets["open"]
	if first.Value == "" || first.Value == second.Value {
		t.Fatalf("the two runs issued %q and %q", first.Value, second.Value)
	}
}

// A run a send starts runs under the sender's layers, as a child does.
func Test_ctl_keyed_root_runs_as_a_child_of_its_sender(t *testing.T) {
	w := newWorld(t)
	allow := &permission.Layer{Allow: []string{"net:a.example"}}
	at, _ := w.place(w.client, router, routed)
	sender := w.startAt(w.client, at, client.Start{Allow: allow, Labels: map[string]string{"app": "x"}})
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	st := w.latest(w.runsOf("talk")[0]).State
	if len(st.Above.Layers) != 1 || st.Above.Layers[0].Allow[0] != "net:a.example" || st.Labels["app"] != "x" || st.Above.Made {
		t.Fatalf("it runs under %+v", st)
	}
}

// One sender's events reach a key in the order sent, and a carry-out done
// again delivers none twice.
func Test_ctl_keyed_events_arrive_once_in_order(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	want := []string{}
	for i := range 6 {
		text := fmt.Sprint("t", i)
		want = append(want, text)
		w.say(sender, "say", 1, text)
		if i%2 == 0 {
			w.pass()
		}
	}
	w.settle()
	run := w.runsOf("talk")[0]
	if got := w.told(run); !slices.Equal(got, want) {
		t.Fatalf("the run was told %v", got)
	}
	if heard := w.latest(run).State.Heard; len(heard) != 1 {
		t.Fatalf("the run remembers %d messages of one sender", len(heard))
	}
}

// sentAgain writes again the messages of events sent to keys that were
// written since the store was last asked, as a carry-out done again writes
// them, and gives how many.
func (w *world) sentAgain(ops []store.Op) int {
	w.t.Helper()
	again := 0
	for _, op := range ops {
		if op.Act == store.ActAdd && op.Done && strings.HasPrefix(op.Name, store.Inbox("ctl")+"/ctl/") && strings.HasSuffix(op.Name, ".key") {
			if _, err := w.owner.Write(ctx, op.Name, op.Value, store.OnlyIfNew); err != nil {
				w.t.Fatal(err)
			}
			again++
		}
	}
	return again
}

// A carry-out done again writes an event that has been taken. It goes to the
// run the step names, whether that run started for it, took it running, or
// has ended since, and no run is given it twice. A step written since is what
// makes it a copy: the run's latest step still names the event it took last.
func Test_ctl_carry_out_done_again_gives_a_key_no_event_twice(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.noted()
	w.say(sender, "say", 1, "one")
	w.settle()
	first := w.noted()
	w.say(sender, "say", 1, "two")
	w.settle()
	second := w.noted()
	if again := w.sentAgain(first); again != 1 {
		t.Fatalf("%d events were written again", again)
	}
	w.settle()
	w.say(sender, "say", 1, "bye")
	w.settle()
	if again := w.sentAgain(second); again != 1 {
		t.Fatalf("%d events were written again", again)
	}
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
	if got := w.told(runs[0]); !slices.Equal(got, []string{"one", "two", "bye"}) {
		t.Fatalf("the run was told %v", got)
	}
}

// An event of any other sender that ends a run in the same step as an event
// sent to a key would leave that event given and not acted on: it is taken
// first or left for the next step, and goes on to a new run.
func Test_ctl_keyed_event_is_never_given_after_the_run_ends(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	w.say(sender, "say", 1, "two")
	w.pass() // the sender writes the event
	if _, err := w.client.Send(ctx, run, "said", map[string]any{"text": "bye"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.say(sender, "say", 1, "three")
	w.settle()
	told := 0
	for _, r := range w.runsOf("talk") {
		told += len(w.told(r))
	}
	if told != 4 {
		t.Fatalf("the runs were told %d events", told)
	}
}

// The event names Corbie gives a machine cannot be sent to a key.
func Test_chart_send_to_a_key_cannot_take_an_events_own_name(t *testing.T) {
	for _, name := range []string{machine.Start, machine.Done, machine.Failed, machine.Timer} {
		_, err := machine.Parse([]byte(dedent(`
			statechart: s
			initial: a
			states:
			  a: { on: { x: { send: { to: { key: k }, event: `+name+` } } } }
		`)), nil)
		if err == nil || !strings.Contains(err.Error(), "no event can be sent") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// A send writes who it goes to as a word or a mapping, and a mapping names a
// key. Its machine is one the definition holds, written out.
func Test_chart_send_to_is_checked(t *testing.T) {
	for _, c := range []struct{ to, says string }{
		{"{ machine: ./talk.yaml }", "names a key"},
		{`{ machine: "${input.m}", key: k }`, "written out"},
		{"{ key: k, other: 1 }", "other"},
		{"[a]", "mapping"},
	} {
		_, err := machine.Parse([]byte(dedent(`
			statechart: s
			initial: a
			states:
			  a: { on: { x: { send: { to: `+c.to+`, event: e } } } }
		`)), machine.Given(routed))
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Fatalf("%s: %v", c.to, err)
		}
	}
	if _, err := machine.Parse([]byte(dedent(`
		flow: f
		stages:
		  a: { job: { script: ./step.js, key: k } }
	`)), machine.Given(map[string]string{"step.js": "return 1"})); err == nil || !strings.Contains(err.Error(), "only a machine") {
		t.Fatalf("a script with a key: %v", err)
	}
}

// The body of a key's entry is the controller's, signed and sealed to the
// controller and the client.
func Test_store_key_file_is_signed_and_sealed(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	data, _, err := w.owner.Read(ctx, store.ClientKey("ctl", "c1", "chat:1"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "chat:1") || strings.Contains(string(data), run) {
		t.Fatalf("the entry is written in the clear: %s", data)
	}
	var entry instance.KeyFile
	if _, err := identity.Read(data, store.ClientKey("ctl", "c1", "chat:1"), w.ctl.Card, w.c1, &entry); err != nil || entry.Run != run {
		t.Fatalf("the client reads %+v, %v", entry, err)
	}
	if _, err := identity.Read(data, store.ClientKey("ctl", "c1", "chat:2"), w.ctl.Card, w.c1, &entry); err == nil {
		t.Fatal("an entry checks under another key's name")
	}
	// A note from a client to the controller is no message it acts on.
	id := "20261010T120000.000000Z-ffffff"
	w.leave(w.c1, id, &instance.Message{Kind: instance.KindSend, Instance: run, From: run,
		Event: &machine.Event{Name: "said", Data: map[string]any{"text": "bye"}}})
	w.settle()
	if head := w.head(run); head.Status != instance.Running {
		t.Fatalf("a client's keyed message ended the run: %+v", head)
	}
}

// keyAndChild is a chart whose child has a key, and which sends that child one
// event by its key and one as its parent, in one step.
const keyAndChild = `
	statechart: both
	initial: ready
	states:
	  ready:
	    job: { machine: ./talk.yaml, key: "chat:1" }
	    on:
	      both:
	        send:
	          - { to: { key: "chat:1" }, event: said, data: { text: a } }
	          - { to: ready, event: said, data: { text: b } }
	      done: over
	  over: { end: done }
`

// twoToChild is the same with no key: two events as the child's parent.
const twoToChild = `
	statechart: twice
	initial: ready
	states:
	  ready:
	    job: { machine: ./talk.yaml }
	    on:
	      both:
	        send:
	          - { to: ready, event: said, data: { text: a } }
	          - { to: ready, event: said, data: { text: b } }
	      done: over
	  over: { end: done }
`

// A step's events are written in the order sent, so a controller stopped
// between two of them never has a run hear the later and take the earlier for
// a copy.
func Test_ctl_events_of_a_step_stopped_after_every_write(t *testing.T) {
	for _, chart := range []string{keyAndChild, twoToChild} {
		play := func(stopAfter int64) ([]string, bool) {
			w := newWorld(t)
			parent := w.submit(chart, routed, nil)
			w.settle()
			if _, err := w.client.Send(ctx, parent, "both", nil, 0); err != nil {
				t.Fatal(err)
			}
			inner := w.ctrl.Store
			s := &stopping{Store: inner}
			s.left.Store(stopAfter)
			w.ctrl = controller.New(w.ctl, s)
			w.ctrl.Now = w.clock
			for range 12 {
				if _, err := w.ctrl.Pass(ctx); err != nil {
					break
				}
			}
			w.afresh(inner)
			w.settle()
			runs := w.runsOf("talk")
			if len(runs) != 1 {
				t.Fatalf("the runs are %v", runs)
			}
			return w.told(runs[0]), s.stopped.Load()
		}
		for n := int64(0); ; n++ {
			got, stopped := play(n)
			if !stopped {
				break
			}
			if !slices.Equal(got, []string{"a", "b"}) {
				t.Fatalf("stopped after %d writes the child was told %v", n, got)
			}
		}
	}
}

// Two runs whose IDs begin alike are two senders.
func Test_ctl_senders_with_names_that_begin_alike_are_apart(t *testing.T) {
	w := newWorld(t)
	stamp := store.Stamp(w.now)
	at, _ := w.place(w.client, router, routed)
	start := func(run string) string {
		w.t.Helper()
		return w.startAt(w.client, at, client.Start{Run: run})
	}
	a := start("c1/" + stamp + "-aa")
	b := start("c1/" + stamp + "-aa.b")
	w.settle()
	w.say(a, "say", 1, "one")
	w.settle()
	w.say(b, "tell", 1, "two")
	w.settle()
	w.say(a, "tell", 1, "three")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 || !slices.Equal(w.told(runs[0]), []string{"one", "two", "three"}) {
		t.Fatalf("the runs are %v", runs)
	}
}

// No client can send an event under a name Corbie gives a machine.
func Test_mach_no_event_takes_a_name_of_corbies(t *testing.T) {
	const counting = `
		statechart: counting
		initial: a
		states:
		  a:
		    job: { machine: ./talk.yaml }
		    on: { done: over }
		  over: { end: done }
	`
	for _, name := range []string{machine.Start, machine.Done, machine.Failed, machine.Timer} {
		w := newWorld(t)
		run := w.submit(counting, routed, nil)
		w.settle()
		if _, err := w.client.Send(ctx, run, name, nil, 0); err != nil {
			t.Fatal(err)
		}
		w.settle()
		st := w.latest(run).State
		if st.Status != instance.Running || st.JobsStarted != 1 || len(w.runsOf("talk")) != 1 {
			t.Fatalf("%s: the chart is %s with %d jobs", name, st.Status, st.JobsStarted)
		}
	}
}

// A runner's word cannot end a job or start a chart: a value emitted under a
// name of Corbie's is dropped.
func Test_mach_emitted_value_takes_no_name_of_corbies(t *testing.T) {
	for _, name := range []string{machine.Start, machine.Done, machine.Failed, machine.Timer} {
		w := newWorld(t)
		r1 := w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{}, nil)
		run := w.submit(`
			statechart: listens
			initial: working
			states:
			  working:
			    job: { script: ./work.js }
			    on: { done: over }
			  over: { end: done }
		`, map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		before := len(w.steps(run))
		w.leave(r1, "20261010T120001.000000Z-a", &instance.Message{Kind: instance.KindEmit, Instance: run,
			Emit: &instance.EmitNote{Job: "working", Name: name, Index: 1, Data: "p1"}})
		w.settle()
		if st := w.latest(run).State; st.Status != instance.Running || len(w.steps(run)) != before {
			t.Fatalf("%s: the chart is %s after %d steps", name, st.Status, len(w.steps(run)))
		}
	}
}

// keyedMessage gives the name and the bytes of the one event waiting to go to
// a key.
func (w *world) keyedMessage() (string, []byte) {
	w.t.Helper()
	var found []string
	for _, name := range w.list(store.Inbox("ctl")) {
		data, _, err := w.owner.Read(ctx, name)
		if err != nil {
			w.t.Fatal(err)
		}
		var msg instance.Message
		if _, err := identity.Read(data, name, w.ctl.Card, w.ctl, &msg); err == nil && msg.Kind == instance.KindSend {
			found = append(found, name)
		}
	}
	if len(found) != 1 {
		w.t.Fatalf("the events waiting are %v", found)
	}
	data, _, err := w.owner.Read(ctx, found[0])
	if err != nil {
		w.t.Fatal(err)
	}
	return found[0], data
}

// A carry-out done again, long after the run it started has ended and has
// forgotten its sender, writes the message that started it. That is dropped:
// it does not start the run again, and it holds up nothing the sender sends.
func Test_ctl_start_written_again_after_its_run_ended_holds_up_nothing(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.pass()
	name, data := w.keyedMessage()
	w.settle()
	first := w.runsOf("talk")[0]
	at, _ := w.place(w.client, router, routed)
	var others []string
	for range 65 {
		others = append(others, w.startAt(w.client, at, client.Start{}))
	}
	w.settle()
	for i, other := range others[:64] {
		w.say(other, "tell", 1, fmt.Sprint(i))
	}
	w.settle()
	if _, err := w.owner.Write(ctx, name, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if got := w.told(first); len(got) != 65 {
		t.Fatalf("the run was told %d things", len(got))
	}
	w.say(others[64], "tell", 1, "bye")
	w.settle()
	if w.head(first).Status != instance.Done {
		t.Fatalf("the run is %s", w.head(first).Status)
	}
	if _, err := w.owner.Write(ctx, name, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if got := w.list(store.Inbox("ctl")); len(got) != 0 {
		t.Fatalf("the inbox holds %v", got)
	}
	w.say(sender, "say", 1, "two")
	w.settle()
	if runs := w.runsOf("talk"); len(runs) != 2 {
		t.Fatalf("the runs are %v", runs)
	}
}

// moving is a store that puts a message back under another's mark just before
// it is deleted, as another copy that moved it on would.
type moving struct {
	store.Store
	name string
	with []byte
}

func (s *moving) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	if name == s.name {
		s.name = ""
		if _, err := s.Store.Write(ctx, name, s.with, store.Overwrite); err != nil {
			return false, err
		}
	}
	return s.Store.Delete(ctx, name, how)
}

// An event another copy has moved on stays, and the sender's record does not
// say it was lost.
func Test_ctl_event_another_copy_moved_on_is_not_noted_as_lost(t *testing.T) {
	w := newWorld(t)
	sender := w.submit(router, routed, nil)
	w.settle()
	w.say(sender, "say", 1, "one")
	w.settle()
	gone := w.runsOf("talk")[0]
	w.say(sender, "tell", 1, "late")
	w.pass()
	name, data := w.keyedMessage()
	if _, err := w.owner.Delete(ctx, name, store.Overwrite); err != nil {
		t.Fatal(err)
	}
	w.settle()
	other := w.submit(router, routed, nil)
	w.settle()
	w.say(other, "tell", 1, "bye")
	w.say(other, "say", 2, "two")
	w.settle()
	var kept string
	for _, run := range w.runsOf("talk") {
		if run != gone {
			kept = run
		}
	}
	var msg instance.Message
	if _, err := identity.Read(data, name, w.ctl.Card, w.ctl, &msg); err != nil {
		t.Fatal(err)
	}
	msg.Instance = kept
	moved, err := w.ctl.Write(name, w.now, time.Time{}, &msg, w.ctl.Card)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, name, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	inner := w.ctrl.Store
	w.ctrl = controller.New(w.ctl, &moving{Store: inner, name: name, with: moved})
	w.ctrl.Now = w.clock
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 0 {
		t.Fatalf("the record says %v", notes)
	}
	if got := w.told(kept); !slices.Contains(got, "late") {
		t.Fatalf("the run was told %v", got)
	}
}
