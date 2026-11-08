package controller_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// talk is the keyed file at /t/room: a run for each key. It ends when it is
// told bye.
const talk = `
	statechart: talk
	lifetime: keyed
	initial: open
	states:
	  open:
	    on:
	      said:
	        - { go: over, when: "${event.data.text == 'bye'}" }
	        - { go: open }
	  over: { end: done }
`

// router is the transient file at /t/router. It sends what it is told to the
// run of a chat at /t/room, and rings the run of the location it is given.
const router = `
	statechart: router
	allow: ["send:**"]
	initial: ready
	states:
	  ready:
	    on:
	      say: { send: { to: { at: room, key: "chat:${event.data.chat}" }, event: said, data: "${event.data}" } }
	      ring: { send: { to: { at: "${event.data.at}" }, event: rang } }
	      poke: { send: { to: { at: "${event.data.at}", run: "${event.data.run}" }, event: said, data: "${event.data}" } }
	      stop: gone
	  gone: { end: done }
`

// rooms submits the tree /t: the router and the room, and any other files
// by their location. It gives a run of the router.
func (w *world) rooms(files map[string]string, scripts ...string) string {
	w.t.Helper()
	given := map[string]string{}
	for _, name := range scripts {
		given[name] = "return input"
	}
	return w.roomsWith(files, given)
}

// roomsWith is rooms, with the files the machines name.
func (w *world) roomsWith(files, named map[string]string) string {
	w.t.Helper()
	all := map[string]string{"/t/router": router, "/t/room": talk}
	for at, src := range files {
		all[at] = src
	}
	if r := w.submitTree(w.client, w.tree("/t", "/srv/t", all, named)); !r.Taken {
		w.t.Fatalf("the tree was refused: %s", r.Refused)
	}
	return w.router()
}

// router starts another run of the router.
func (w *world) router() string {
	w.t.Helper()
	run := w.startAt(w.client, "/t/router", client.Start{})
	w.settle()
	return run
}

// say has a client send a router an event.
func (w *world) say(run, event string, chat int, text string) {
	w.t.Helper()
	if _, err := w.client.Send(ctx, w.to(run), event, map[string]any{"chat": float64(chat), "text": text}, time.Hour); err != nil {
		w.t.Fatal(err)
	}
}

// tell has a client send a router an event with data.
func (w *world) tell(run, event string, data map[string]any) {
	w.t.Helper()
	if _, err := w.client.Send(ctx, w.to(run), event, data, time.Hour); err != nil {
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

// entry reads the file of a key at a location.
func (w *world) entry(location, key string) *instance.KeyFile {
	w.t.Helper()
	entry, _, err := instance.ReadKey(ctx, w.owner, w.ctl.Card, w.ctl, store.Key("ctl", location, key), key)
	if err != nil {
		w.t.Fatal(err)
	}
	return entry
}

// writeEntry signs a key's file, as a controller stopped at the right moment
// leaves it.
func (w *world) writeEntry(location, key, run, start string) {
	w.t.Helper()
	at := store.Key("ctl", location, key)
	data, err := w.ctl.Write(at, w.now, time.Time{}, instance.KeyFile{Key: key, Run: run, Start: start}, w.ctl.Card)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, at, data, store.Overwrite); err != nil {
		w.t.Fatal(err)
	}
}

// keyNotes gives what a run's record says of its sends that were not
// delivered.
func (w *world) keyNotes(run string) []string {
	w.t.Helper()
	var notes []string
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			switch {
			case ask.Kind == instance.AskNote:
				notes = append(notes, ask.Text)
			case ask.Kind == instance.AskEvent && ask.Undelivered && ask.Name == "":
				notes = append(notes, ask.Text)
			}
		}
	}
	return notes
}

// sentBy gives the asks of a run's record that send to an address.
func (w *world) sentBy(run string) []instance.Ask {
	w.t.Helper()
	var asks []instance.Ask
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskEvent && ask.Location != "" {
				asks = append(asks, ask)
			}
		}
	}
	return asks
}

// A send to a key with no running run starts one, a root of the client of
// the receiving tree, gives it the event's data as its input and then the
// event, and goes to that run after. The run's head names its location, its
// key and its sender, and its record names the sender's address.
func Test_chart_send_to_a_key_starts_a_run_then_delivers(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
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
	if head.Location != "/t/room" || head.Key != "chat:1" || head.Parent != "" || head.StartedBy != sender || store.InstanceClient(run) != "c1" {
		t.Fatalf("the head is %+v", head)
	}
	if st.Above.Parent != "" || st.Above.StartedBy != sender || st.Above.Depth != 1 || st.Lifetime != machine.LifetimeKeyed {
		t.Fatalf("it runs under %+v", st.Above)
	}
	if in, _ := st.Input.(map[string]any); in["text"] != "one" {
		t.Fatalf("its input is %v", st.Input)
	}
	if entry := w.entry("/t/room", "chat:1"); entry == nil || entry.Run != run {
		t.Fatalf("the entry is %+v", entry)
	}
	first := w.steps(run)[0]
	if len(first.Events) != 2 || first.Events[0].Name != machine.Start || first.Events[1].Name != "said" {
		t.Fatalf("the first step took %+v", first.Events)
	}
	if from := first.Events[1].Event.Sender; from == nil || from.At != "/t/router" || from.Run != sender || from.Key != nil {
		t.Fatalf("the event came from %+v", from)
	}
	asks := w.sentBy(sender)
	if len(asks) != 2 || asks[0].Run != run || asks[0].To != run || asks[1].Run != "" || asks[1].To != run || asks[0].Location != "/t/room" || asks[0].Key != "chat:1" {
		t.Fatalf("the sender's record says %+v", asks)
	}
}

// A send to { at } goes to the singleton's run. With none running, or where
// nothing is recorded, it is written in the sender's record as undelivered,
// saying which, and fails nothing.
func Test_chart_send_to_a_singleton_with_no_run_is_undelivered(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/bell": `
		statechart: bell
		lifetime: singleton
		initial: a
		states:
		  a: { on: { rang: b } }
		  b: { end: done }
	`})
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.settle()
	bell := w.at("/t/bell")
	if len(bell) != 1 {
		t.Fatalf("the bell's runs are %v", bell)
	}
	for _, at := range []string{"bell", "nowhere"} {
		w.tell(sender, "ring", map[string]any{"at": at})
		w.settle()
	}
	notes := w.keyNotes(sender)
	if len(notes) != 2 || !strings.Contains(notes[0], "no run of the singleton at /t/bell") ||
		!strings.Contains(notes[1], "nothing is recorded at /t/nowhere") {
		t.Fatalf("the record says %v", notes)
	}
	if head := w.head(sender); head.Status != instance.Running {
		t.Fatalf("the sender is %s", head.Status)
	}
}

// A send to a transient location's { at } starts a new run of it each time,
// as a key no other send names would: two sends in one step start two runs,
// and a client's send starts one too. A statechart is given the event after
// its start, and a flow, which takes no sent event, only its start, with the
// event's data as its input. No run takes a key's file, and the sender's
// record notes nothing.
func Test_ctl_send_to_a_transient_location_starts_a_run(t *testing.T) {
	w := newWorld(t)
	w.newRunner()
	sender := w.rooms(map[string]string{
		"/t/bell": `
			statechart: bell
			initial: a
			states:
			  a: { on: { rang: b } }
			  b: { end: done, output: "${input}" }
		`,
		"/t/twice": `
			statechart: twice
			allow: ["send:**"]
			initial: a
			states:
			  a: { entry: { send: [{ to: { at: bell }, event: rang, data: 1 }, { to: { at: bell }, event: rang, data: 2 }] } }
		`,
		"/t/work": `
			flow: work
			stages:
			  a: { job: { script: ./step.js, input: "${input}" } }
		`,
	}, "step.js")
	w.startAt(w.client, "/t/twice", client.Start{})
	w.settle()
	w.tell(sender, "ring", map[string]any{"at": "work"})
	w.settle()
	if _, err := w.client.Send(ctx, machine.Address{At: "/t/bell"}, "rang", 3, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	bells := w.runsOf("bell")
	var outputs []string
	for _, run := range bells {
		st := w.latest(run).State
		outputs = append(outputs, fmt.Sprint(st.Status, " ", st.Output))
		if head := w.head(run); head.Key != "" || head.Parent != "" {
			t.Fatalf("the run %s has the key %q and the parent %q", run, head.Key, head.Parent)
		}
	}
	slices.Sort(outputs)
	if fmt.Sprint(outputs) != "[done 1 done 2 done 3]" {
		t.Fatalf("the bells ended %v", outputs)
	}
	works := w.runsOf("work")
	if len(works) != 1 {
		t.Fatalf("the flows are %v", works)
	}
	if steps := w.steps(works[0]); len(steps) != 1 || len(steps[0].Events) != 1 || steps[0].Events[0].Name != machine.Start ||
		steps[0].State.Status != instance.Running || steps[0].State.Above.StartedBy != sender {
		t.Fatalf("the flow's record is %+v", steps)
	}
	if notes := w.keyNotes(sender); len(notes) != 0 {
		t.Fatalf("the router's record says %v", notes)
	}
	if left := w.list(store.Keys("ctl", "/t/bell")); len(left) != 0 {
		t.Fatalf("key files: %v", left)
	}
}

// A send that would start a run of a transient location is refused by its
// receive: as a keyed start is: the sender's record notes it undelivered, a
// client's receipt says refused, and no run starts.
func Test_ctl_send_to_a_transient_location_is_refused_by_receive(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/picky": `
		statechart: picky
		receive: [client:b]
		initial: a
		states:
		  a: { on: { rang: b } }
		  b: { end: done }
	`})
	w.tell(sender, "ring", map[string]any{"at": "picky"})
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "/t/picky does not receive rang from the run "+sender) {
		t.Fatalf("the router's record says %v", notes)
	}
	c := w.other("c", "send:/t/**")
	if r := w.clientSend(c, "/t/picky", "rang", nil); r.Taken || !strings.Contains(r.Refused, "does not receive rang from client:c") {
		t.Fatalf("c got %+v", r)
	}
	if runs := w.runsOf("picky"); len(runs) != 0 {
		t.Fatalf("the runs of picky are %v", runs)
	}
}

// A key is free again once its run ends, and so is a singleton's.
func Test_ctl_key_is_free_once_its_run_ends(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/bell": `
		statechart: bell
		lifetime: singleton
		initial: a
		states:
		  a: { on: { rang: b } }
		  b: { end: done }
	`})
	w.say(sender, "say", 1, "bye")
	w.settle()
	if entry := w.entry("/t/room", "chat:1"); entry != nil {
		t.Fatalf("the entry of a run that ended is %+v", entry)
	}
	w.say(sender, "say", 1, "again")
	w.settle()
	if runs := w.runsOf("talk"); len(runs) != 2 {
		t.Fatalf("the runs are %v", runs)
	}
	if entry := w.entry("/t/bell", ""); entry == nil {
		t.Fatal("the singleton's key has no file")
	}
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.settle()
	if entry := w.entry("/t/bell", ""); entry != nil {
		t.Fatalf("the singleton ended and its key names %+v", entry)
	}
}

// A key is any text: it is in no name in the store, however it is written.
func Test_store_a_key_is_in_no_name(t *testing.T) {
	for _, key := range []string{"a/b", "../../x", "with space", strings.Repeat("é", 256), " "} {
		sum := sha256.Sum256([]byte(key))
		if name := store.Key("ctl", "/t/room", key); name != "ctl/locations/t/room/+/keys/"+hex.EncodeToString(sum[:]) {
			t.Fatalf("the key %q is named %s", key, name)
		}
	}
}

// One key at two locations is two runs, and a send names the run of the
// location it is sent to.
func Test_ctl_a_key_names_runs_of_one_location(t *testing.T) {
	w := newWorld(t)
	other := strings.Replace(router, "at: room", "at: hall", 1)
	sender := w.rooms(map[string]string{"/t/hall": talk, "/t/other": other})
	second := w.startAt(w.client, "/t/other", client.Start{})
	w.settle()
	w.say(sender, "say", 1, "one")
	w.say(second, "say", 1, "two")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 2 || w.head(runs[0]).Location == w.head(runs[1]).Location {
		t.Fatalf("the runs are %v", runs)
	}
	for _, run := range runs {
		if w.head(run).Key != "chat:1" {
			t.Fatalf("the run %s has the key %q", run, w.head(run).Key)
		}
	}
}

// A key past its bound, or empty, is undelivered before anything is read.
func Test_mach_key_past_its_bound_writes_nothing(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/keys": `
		statechart: keys
		allow: ["send:room"]
		initial: ready
		states:
		  ready:
		    on:
		      go: { send: { to: { at: room, key: "${event.data}" }, event: said, data: { text: x } } }
	`})
	sender = w.startAt(w.client, "/t/keys", client.Start{})
	w.settle()
	w.noted()
	for _, key := range []any{strings.Repeat("a", 257), "", nil} {
		if _, err := w.client.Send(ctx, w.to(sender), "go", key, time.Hour); err != nil {
			t.Fatal(err)
		}
		w.settle()
	}
	for _, op := range w.noted() {
		if strings.Contains(op.Name, "/+/keys/") {
			t.Fatalf("an undelivered key led to %s %s", op.Act, op.Name)
		}
	}
	notes := w.keyNotes(sender)
	if len(notes) != 3 || !strings.Contains(notes[0], "at most 256") || !strings.Contains(notes[1], "not empty") || !strings.Contains(notes[2], "not empty") {
		t.Fatalf("the record says %v", notes)
	}
	if len(w.runsOf("talk")) != 0 || w.head(sender).Status != instance.Running {
		t.Fatalf("the runs are %v", w.runsOf("talk"))
	}
	if _, err := w.client.Send(ctx, w.to(sender), "go", strings.Repeat("é", 256), time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if got := w.runsOf("talk"); len(got) != 1 {
		t.Fatalf("256 characters started %v", got)
	}
}

// A run's data that does not fit its machine's input fails the run as it
// starts, and the sender's record says so. The key is free again.
func Test_ctl_send_whose_data_does_not_fit_fails_in_the_senders_record(t *testing.T) {
	w := newWorld(t)
	strict := strings.Replace(talk, "initial: open", "input: { count: number }\n\tinitial: open", 1)
	sender := w.rooms(map[string]string{"/t/room": strict})
	w.say(sender, "say", 1, "no count in this")
	w.settle()
	notes := w.keyNotes(sender)
	if len(notes) != 1 || !strings.Contains(notes[0], "/t/room#chat:1") || !strings.Contains(notes[0], "count") {
		t.Fatalf("the record says %v", notes)
	}
	if entry := w.entry("/t/room", "chat:1"); entry != nil {
		t.Fatalf("the key is held by %+v", entry)
	}
	if left := w.list(store.Inbox("ctl")); len(left) != 0 {
		t.Fatalf("left waiting: %v", left)
	}
}

// A flow takes no sent event: a send to a flow's run by its ID is noted as
// undelivered.
func Test_ctl_send_to_a_flow_or_agent_run_is_undelivered(t *testing.T) {
	w := newWorld(t)
	w.newRunner()
	sender := w.rooms(map[string]string{"/t/work": `
		flow: work
		stages:
		  a: { job: { script: ./step.js } }
	`}, "step.js")
	work := w.startAt(w.client, "/t/work", client.Start{})
	w.settle()
	w.tell(sender, "poke", map[string]any{"at": "work", "run": work, "text": "x"})
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "takes no sent event") {
		t.Fatalf("the record says %v", notes)
	}
	if st := w.latest(work).State; st.Status != instance.Running || len(w.steps(work)) != 1 {
		t.Fatalf("the flow is %s after %d steps", st.Status, len(w.steps(work)))
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
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	// Three for the sender: the first ends the run the other two are sent
	// to.
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
	if entry := w.entry("/t/room", "chat:1"); entry == nil || w.head(entry.Run).Status != instance.Running {
		t.Fatalf("the key's entry is %+v", entry)
	}
	return strings.Join(told, " "), s.stopped.Load()
}

// Two senders race to start one key: one run starts, and each event reaches
// it in each sender's order.
func Test_ctl_two_senders_race_for_one_key(t *testing.T) {
	w := newWorld(t)
	first := w.rooms(nil)
	second := w.router()
	w.say(first, "say", 1, "a1")
	w.say(second, "say", 1, "b1")
	w.say(first, "say", 1, "a2")
	w.say(second, "say", 1, "b2")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
	got := w.told(runs[0])
	if len(got) != 4 || slices.Index(got, "a1") > slices.Index(got, "a2") || slices.Index(got, "b1") > slices.Index(got, "b2") {
		t.Fatalf("the run was told %v", got)
	}
}

// Two senders race, stopped after any single write: one run starts, and each
// event is given to it once.
func Test_ctl_two_senders_stopped_after_every_write(t *testing.T) {
	for n := int64(0); ; n++ {
		w := newWorld(t)
		first := w.rooms(nil)
		second := w.router()
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
		first := w.rooms(nil)
		second := w.router()
		w.say(first, "say", 1, "a")
		w.say(second, "say", 1, "b")
		w.pass() // the sends are routed to the routers
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

// An event sent to a key whose run ended before it took the event starts a
// fresh run with it. One sent to a singleton whose run has ended, or to a run
// by its ID that has ended, is noted in the sender's record.
func Test_ctl_event_for_a_run_that_ended_starts_anew_or_is_noted(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/bell": `
		statechart: bell
		lifetime: singleton
		initial: a
		states:
		  a: { on: { rang: b } }
		  b: { end: done }
	`})
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	w.say(sender, "say", 1, "bye")
	w.say(sender, "say", 1, "late")
	w.tell(sender, "poke", map[string]any{"at": "room", "run": run, "text": "later"})
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 2 || !slices.Equal(w.told(runs[0]), []string{"late"}) {
		t.Fatalf("the runs are %v", runs)
	}
	// The event for the key went to a fresh run, which the sender is told.
	notes := w.keyNotes(sender)
	if len(notes) != 3 || !strings.Contains(notes[0], "went to the run "+runs[0]+", not "+run) || !strings.Contains(notes[1], "has ended") || !strings.Contains(notes[2], "no run of the singleton") {
		t.Fatalf("the record says %v", notes)
	}
}

// An event in flight to a run that has ended goes to the run that has the
// key by then, under the name it was sent with.
func Test_ctl_event_in_flight_goes_to_the_run_that_has_the_key_now(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "say", 1, "bye")
	w.say(sender, "say", 1, "two")
	w.say(sender, "say", 1, "three")
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
		sender := w.rooms(nil)
		// The sender reads a file its run has left behind, and its event goes
		// to a run that has ended.
		ended := w.router()
		w.tell(ended, "stop", nil)
		w.settle()
		w.writeEntry("/t/room", "chat:1", ended, "ctl/inbox/ctl/gone")
		w.say(sender, "say", 1, "first")
		w.pass() // routed
		w.pass() // the sender's step
		// The file goes before the sender's next event: that one starts a run.
		if _, err := w.owner.Delete(ctx, store.Key("ctl", "/t/room", "chat:1"), store.Overwrite); err != nil {
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

// A send to a run by its ID from a sender whose earlier send to a key at that
// location is on its way to another run waits for it: the run hears both, in
// the order sent, and drops neither.
func Test_ctl_direct_send_waits_behind_a_moved_send(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	gone := w.runsOf("talk")[0]
	// The run that has the key ends, and a file left behind still names it
	// when the sender sends its next event to the key.
	w.tell(gone, "said", map[string]any{"text": "bye"})
	w.settle()
	w.writeEntry("/t/room", "chat:1", gone, "ctl/inbox/ctl/gone")
	w.say(sender, "say", 1, "two")
	w.pass() // routed
	w.pass() // the sender names the run that has ended
	moving, _ := w.keyedMessage()
	if _, err := w.owner.Delete(ctx, store.Key("ctl", "/t/room", "chat:1"), store.Overwrite); err != nil {
		t.Fatal(err)
	}
	// While the event is on its way, another run takes the key, and the
	// sender sends to that run by its ID.
	inner := w.ctrl.Store
	w.afresh(&hiding{Store: inner, hide: func(n string) bool { return n == moving }})
	other := w.router()
	w.say(other, "say", 1, "fresh")
	w.settle()
	var next string
	for _, run := range w.runsOf("talk") {
		if run != gone && w.head(run).Status == instance.Running {
			next = run
		}
	}
	if next == "" {
		t.Fatalf("no run has the key: %v", w.runsOf("talk"))
	}
	w.tell(sender, "poke", map[string]any{"at": "room", "run": next, "text": "three"})
	w.pass() // routed
	w.pass() // the sender sends to the run by its ID
	w.afresh(inner)
	w.settle()
	got := w.told(next)
	if !slices.Equal(got, []string{"fresh", "two", "three"}) {
		t.Fatalf("the run was told %v", got)
	}
	// The sender is told the run its moved event went to.
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "went to the run "+next+", not "+gone) {
		t.Fatalf("the record says %v", notes)
	}
}

// An event moved twice is noted twice in its sender's record, and the last
// note names the run that took it.
func Test_ctl_event_moved_twice_is_noted_twice(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.say(sender, "say", 2, "two")
	w.say(sender, "say", 3, "three")
	w.settle()
	w.say(sender, "say", 1, "bye")
	w.settle()
	runs := map[string]string{}
	for _, run := range w.runsOf("talk") {
		runs[w.told(run)[0]] = run
	}
	gone, first, last := runs["one"], runs["two"], runs["three"]
	// The file of chat:1 names the run that has ended when the sender sends
	// to the key, and then a run that is running.
	w.writeEntry("/t/room", "chat:1", gone, "ctl/inbox/ctl/gone")
	w.say(sender, "say", 1, "moved")
	w.pass() // routed
	w.pass() // the sender names the run that has ended
	w.writeEntry("/t/room", "chat:1", first, "ctl/inbox/ctl/first")
	w.pass() // moved to the first run
	// The first run is cancelled before it takes the event, and the file
	// names another.
	if _, err := w.client.Cancel(ctx, first, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.writeEntry("/t/room", "chat:1", last, "ctl/inbox/ctl/last")
	w.settle()
	if got := w.told(last); !slices.Equal(got, []string{"three", "moved"}) {
		t.Fatalf("the last run was told %v", got)
	}
	notes := w.keyNotes(sender)
	if len(notes) != 2 || !strings.Contains(notes[0], "went to the run "+first+", not "+gone) || !strings.Contains(notes[1], "went to the run "+last+", not "+first) {
		t.Fatalf("the record says %v", notes)
	}
}

// A file a crash left, naming a run that has ended or that never began,
// blocks nothing.
func Test_ctl_stale_key_file_blocks_no_key(t *testing.T) {
	gone := "c1/20261010T120000.000Z-0000000000"
	for name, plant := range map[string]func(w *world){
		"a run that has ended": func(w *world) {
			run := w.router()
			w.tell(run, "stop", nil)
			w.settle()
			w.writeEntry("/t/room", "chat:1", run, "ctl/inbox/ctl/gone")
		},
		"a run that never began": func(w *world) {
			w.writeEntry("/t/room", "chat:1", gone, "ctl/inbox/ctl/gone")
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			sender := w.rooms(nil)
			plant(w)
			w.say(sender, "say", 1, "first")
			w.settle()
			runs := w.runsOf("talk")
			if len(runs) != 1 || w.head(runs[0]).Status != instance.Running || w.head(runs[0]).Key != "chat:1" {
				t.Fatalf("the send started %v", runs)
			}
			w.say(sender, "say", 1, "now")
			w.settle()
			if got := w.told(runs[0]); !slices.Equal(got, []string{"first", "now"}) {
				t.Fatalf("the run was told %v", got)
			}
		})
	}
}

// hiding is a store that leaves some names out of a listing of the inbox.
type hiding struct {
	store.Store
	hide func(name string) bool
}

func (h *hiding) List(ctx context.Context, under string) ([]store.Listed, error) {
	names, err := h.Store.List(ctx, under)
	if under != store.Inbox("ctl") {
		return names, err
	}
	return slices.DeleteFunc(names, func(l store.Listed) bool { return h.hide(l.Name) }), err
}

// A key's file whose run has not taken its first step keeps the key while
// its start is waiting: a controller stopped between the two leaves it so.
// A send to the key goes to that run once it starts, and starts no other.
func Test_ctl_key_file_of_a_start_still_waiting_is_kept(t *testing.T) {
	w := newWorld(t)
	first := w.rooms(nil)
	second := w.router()
	w.say(first, "say", 1, "one")
	w.pass() // routed
	w.pass() // the first sender's step writes the start of a run
	name, data := w.keyedMessage()
	var msg instance.Message
	if _, err := identity.Read(data, name, w.ctl.Card, w.ctl, &msg); err != nil || msg.Start == nil || msg.Start.Run == "" {
		t.Fatalf("the message is %+v, %v", msg, err)
	}
	started := msg.Start.Run
	// The run took its key and stopped before its first step.
	w.writeEntry("/t/room", "chat:1", started, name)
	inner := w.ctrl.Store
	w.afresh(&hiding{Store: inner, hide: func(n string) bool { return n == name }})
	w.say(second, "say", 1, "two")
	w.pass() // routed
	w.pass() // the second sender's step finds the start waiting
	if w.head(started) != nil {
		t.Fatalf("the run %s began while its start was hidden", started)
	}
	if asks := w.sentBy(second); len(asks) != 1 || asks[0].To != started || asks[0].Run != "" {
		t.Fatalf("the second sender's record says %+v", asks)
	}
	w.afresh(inner)
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 || runs[0] != started || !slices.Equal(w.told(started), []string{"one", "two"}) {
		t.Fatalf("the runs are %v, and %s was told %v", runs, started, w.told(started))
	}
}

// A run started by a send outlives its sender with what it was given: its
// definition, the machines and scripts it names, and the blobs in the event
// are in its own record, and it outlives a submit that drains its file.
func Test_ctl_keyed_root_outlives_its_sender(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	keeper := `
		statechart: keeper
		lifetime: keyed
		update: drain
		ask: ./ok.js
		initial: open
		states:
		  open: { on: { go: reading } }
		  reading: { job: { script: ./read.js, input: "${input}", approve: true }, on: { done: over } }
		  over: { end: done }
	`
	sender := `
		statechart: sender
		keep: 1h
		allow: ["send:keeper"]
		input: { doc: blob }
		initial: ready
		states:
		  ready:
		    on:
		      go: { go: gone, send: { to: { at: keeper, key: k }, event: hello, data: { doc: "${input.doc}" } } }
		  gone: { end: done }
	`
	scripts := map[string]string{"read.js": "return input", "ok.js": "return { approved: input.jobs.map(j => j.name) }"}
	tree := func() *client.Tree {
		return w.tree("/k", "/srv/k", map[string]string{"/k/keeper": keeper, "/k/sender": sender}, scripts)
	}
	if r := w.submitTree(w.client, tree()); !r.Taken {
		t.Fatal(r.Refused)
	}
	from := w.startAt(w.client, "/k/sender", client.Start{Input: map[string]any{"doc": client.InputFile{Name: "a.txt", Contents: strings.NewReader("what it was given")}}})
	w.settle()
	w.tell(from, "go", nil)
	w.settle()
	runs := w.runsOf("keeper")
	if len(runs) != 1 || w.head(from).Status != instance.Done {
		t.Fatalf("the runs are %v and the sender is %s", runs, w.head(from).Status)
	}
	run := runs[0]
	st := w.latest(run).State
	if st.Definition.Instance() != run || st.Keep != machine.DefaultKeep || st.Above.Ask != nil {
		t.Fatalf("the run's definition is in the record of %s, it is kept %s, and the ask above is %+v", st.Definition.Instance(), st.Keep, st.Above.Ask)
	}

	// The sender goes, record and all, and its file is submitted again.
	w.tick(2 * time.Hour)
	w.settle()
	if left := w.list(store.Record("ctl", from)); len(left) != 0 || w.head(from) != nil {
		t.Fatalf("the sender's record holds %v", left)
	}
	again := tree()
	again.Machines[0].Draft = w.draft(strings.Replace(keeper, "initial: open", "title: changed\n\t\tinitial: open", 1), scripts)
	w.submitTree(w.client, again)

	w.tell(run, "go", nil)
	w.settle()
	var job *instance.Job
	for _, msg := range w.inboxOf(r1) {
		if msg.Job != nil && msg.Job.Instance == run && strings.HasPrefix(msg.Job.Name, instance.AskName) {
			job = msg.Job
		}
	}
	if job == nil || job.Script.Instance() != run {
		t.Fatalf("its own ask was not sent: %+v", w.latest(run).Asks)
	}
	in, _ := w.latest(run).State.Input.(map[string]any)
	ref, ok := blob.AsRef(in["doc"])
	if !ok || ref.Instance() != run {
		t.Fatalf("the input is %v", in)
	}
	data, err := blob.ReadAll(ctx, w.owner, ref, 1<<20)
	if err != nil || string(data) != "what it was given" {
		t.Fatalf("the blob reads %q, %v", data, err)
	}
}

// A chain of sends that each start a run ends at the depth limit of the file
// of the run that would pass it: a run started by a send sits one level below
// its sender, and fails as it starts, telling its sender.
func Test_ctl_chain_of_sends_ends_at_the_depth_limit(t *testing.T) {
	w := newWorld(t)
	files := map[string]string{}
	for n := 1; n <= 6; n++ {
		files[fmt.Sprintf("/c/hop%d", n)] = fmt.Sprintf(`
			statechart: hop%d
			lifetime: keyed
			limits: { depth: 3 }
			allow: ["send:hop%d"]
			initial: go
			states:
			  go: { entry: { send: { to: { at: hop%d, key: "h" }, event: hop } }, on: { x: go } }
		`, n, n+1, n+1)
	}
	files["/c/hop0"] = `
		statechart: hop0
		allow: ["send:hop1"]
		initial: go
		states:
		  go: { entry: { send: { to: { at: hop1, key: "h" }, event: hop } }, on: { x: go } }
	`
	if r := w.submitTree(w.client, w.tree("/c", "/srv/c", files, nil)); !r.Taken {
		t.Fatal(r.Refused)
	}
	w.startAt(w.client, "/c/hop0", client.Start{})
	w.settle()
	running := 0
	for _, at := range append(w.list(store.Running("ctl")), w.list(store.Finished("ctl"))...) {
		head := w.head(store.InstanceOf("ctl", at))
		if head.Machine == "hop3" {
			if notes := w.keyNotes(head.Name); len(notes) != 1 || !strings.Contains(notes[0], "depth limit") {
				t.Fatalf("the last of the chain says %v", notes)
			}
		}
		if head.Machine == "hop4" && head.Status != instance.Failed {
			t.Fatalf("the chain went past the limit: %+v", head)
		}
		if head.Status == instance.Running {
			running++
		}
	}
	if running != 4 {
		t.Fatalf("%d runs are running", running)
	}
}

// A send to a key reads its file, and the location's file when it starts a
// run. It lists nothing at a location with no limit on its runs, and lists
// the keys once a step at one with a limit.
func Test_ctl_send_to_a_key_lists_nothing(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "say", 1, "two")
	w.noted()
	w.settle()
	reads := 0
	for _, op := range w.noted() {
		if op.Act == store.ActList && op.Name != store.Inbox("ctl") && op.Name != store.Wakes("ctl") {
			t.Fatalf("a pass listed %s", op.Name)
		}
		if strings.Contains(op.Name, "/keys/") && op.Act == store.ActRead {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("a send read the key's file %d times", reads)
	}

	w = newWorld(t)
	sender = w.rooms(map[string]string{"/t/room": strings.Replace(talk, "lifetime: keyed", "lifetime: keyed\n\tlimits: { runs: 9 }", 1)})
	w.noted()
	w.say(sender, "say", 1, "one")
	w.say(sender, "say", 2, "two")
	w.pass() // routed
	w.pass() // one step of the sender takes both
	lists := 0
	for _, op := range w.noted() {
		if op.Act == store.ActList && strings.HasSuffix(op.Name, "/keys") {
			lists++
		}
	}
	if lists != 2 || len(w.sentBy(sender)) != 2 {
		t.Fatalf("two sends listed the keys %d times", lists)
	}
}

// A machine takes all the events waiting in one step, so messages that arrive
// together are sent on together, however many there are. Their keys are read
// side by side, so a step that fits in a pass needs no more than a pass.
func Test_ctl_step_sends_to_any_number_of_keys(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	for chat := range 300 {
		w.say(sender, "say", chat, "one")
	}
	w.pass() // routed
	inner := w.ctrl.Store
	w.ctrl = controller.New(w.ctl, &delayedKeys{Store: inner})
	w.ctrl.Now = w.clock
	limited, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := w.ctrl.Pass(limited); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if steps := w.steps(sender); len(steps) != 2 {
		t.Fatalf("the sender took %d steps in the pass", len(steps))
	}
	w.ctrl = controller.New(w.ctl, inner)
	w.ctrl.Now = w.clock
	w.settle()
	if st := w.latest(sender).State; st.Status != instance.Running {
		t.Fatalf("the sender ended %s: %+v", st.Status, st.Error)
	}
	if got := len(w.runsOf("talk")); got != 300 {
		t.Fatalf("%d runs started", got)
	}
}

// delayedKeys takes 20 ms over each read of a key's file.
type delayedKeys struct{ store.Store }

func (s *delayedKeys) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if strings.Contains(name, "/keys/") {
		time.Sleep(20 * time.Millisecond)
	}
	return s.Store.Read(ctx, name)
}

// A machine made while running talks only to its parent and its children,
// whatever the job that starts it lists: a send to an address fails it.
func Test_ctl_made_machine_sends_to_no_address(t *testing.T) {
	const job = `
		flow: maker
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}", allow: ["all"] } }
	`
	w := newWorld(t)
	target := w.rooms(nil)
	w.say(target, "say", 1, "one")
	w.settle()
	room := w.runsOf("talk")[0]
	r1 := w.newRunner()
	at, _ := w.place(w.client, job, map[string]string{"make.js": "return 1"})
	run := w.startAt(w.client, at, client.Start{})
	w.settle()
	w.made(r1, run, `
		statechart: made
		initial: a
		states:
		  a: { entry: { send: { to: { at: /t/room, key: "chat:1" }, event: said, data: { text: forged } } } }
	`)
	w.settle()
	st := w.latest(w.child(run)).State
	if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "sends only to its parent and its children") {
		t.Fatalf("the machine is %s: %+v", st.Status, st.Error)
	}
	if got := w.told(room); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("the run with the key was told %v", got)
	}

	run = w.startAt(w.client, at, client.Start{})
	w.settle()
	w.made(r1, run, `
		statechart: made
		initial: a
		states:
		  a: { entry: { send: { to: parent, event: hello } }, on: { x: a } }
	`)
	w.settle()
	if st := w.latest(w.child(run)).State; st.Status != instance.Running {
		t.Fatalf("a made machine that sends to its parent is %s: %+v", st.Status, st.Error)
	}
}

// Each run a send starts has a seed of its own, so no two issue one ticket,
// though one sender started them from one file.
func Test_ctl_keyed_runs_of_one_sender_share_no_ticket(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/room": `
		statechart: talk
		lifetime: keyed
		initial: open
		states:
		  open: { ticket: verdict, on: { verdict: over } }
		  over: { end: done }
	`})
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

// A run a send starts takes nothing from its sender: no layer, no label, no
// ask, and the values of its own location. Its held job goes to its own ask,
// and with none it is denied.
func Test_ctl_sent_root_takes_nothing_from_its_sender(t *testing.T) {
	w := newWorld(t)
	w.newRunner()
	held := `
		statechart: talk
		lifetime: keyed
		initial: open
		states:
		  open: { job: { script: ./step.js, approve: true }, on: { done: open, failed: open, said: open } }
	`
	w.rooms(map[string]string{"/t/room": held}, "step.js")
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/t/router", "WHERE", "router"); err != nil {
		t.Fatal(err)
	}
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/t/room", "WHERE", "room"); err != nil {
		t.Fatal(err)
	}
	from := w.startAt(w.client, "/t/router", client.Start{Allow: &permission.Layer{Allow: []string{"send:**"}}, Labels: map[string]string{"app": "x"}})
	w.settle()
	w.say(from, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	st := w.latest(run).State
	if len(st.Above.Layers) != 0 || len(st.Labels) != 0 || st.Above.Made || st.Above.Parent != "" || st.Above.StartedBy != from ||
		st.Above.Ask != nil || st.Above.Values["WHERE"] != "room" {
		t.Fatalf("it runs under %+v with labels %v", st.Above, st.Labels)
	}
	failed := st.Error != nil && strings.Contains(st.Error.Error, "no ask applies")
	for _, step := range w.steps(run) {
		for _, e := range step.Events {
			if e.Name == machine.Failed && strings.Contains(fmt.Sprint(e.Data), "no ask applies") {
				failed = true
			}
		}
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskFail && strings.Contains(ask.Error.Error, "no ask applies") {
				failed = true
			}
		}
	}
	if !failed {
		t.Fatalf("its held job was not denied: %+v", w.latest(run))
	}
}

// One sender's events reach a key in the order sent, once each.
func Test_ctl_keyed_events_arrive_once_in_order(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	want := []string{}
	for i := range 50 {
		text := fmt.Sprint("t", i)
		want = append(want, text)
		w.say(sender, "say", 1, text)
		if i%7 == 0 {
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

// sentAgain writes again the messages of events sent to addresses that were
// written since the store was last asked, as a carry-out done again writes
// them, and gives how many.
func (w *world) sentAgain(ops []store.Op) int {
	w.t.Helper()
	again := 0
	for _, op := range ops {
		if op.Act == store.ActAdd && op.Done && strings.HasPrefix(op.Name, store.Inbox("ctl")+"/ctl/") && strings.HasSuffix(op.Name, ".send") {
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
// has ended since, and no run is given it twice.
func Test_ctl_carry_out_done_again_gives_a_key_no_event_twice(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
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
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	w.say(sender, "say", 1, "two")
	w.pass() // routed
	w.pass() // the sender writes the event
	w.tell(run, "said", map[string]any{"text": "bye"})
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

// An event sent to a run by its ID that arrives in the step that ends the
// run is taken there and not acted on, as an event with no rule is: it is
// in the run's record, and its sender is not told it was undelivered.
func Test_ctl_direct_event_with_the_runs_end_is_taken_and_not_acted_on(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	w.tell(sender, "poke", map[string]any{"at": "room", "run": run, "text": "bye"})
	w.tell(sender, "poke", map[string]any{"at": "room", "run": run, "text": "late"})
	w.settle()
	steps := w.steps(run)
	last := steps[len(steps)-1]
	if got := w.told(run); !slices.Equal(got, []string{"one", "bye", "late"}) || events(last) != "said said" || last.State.Status != instance.Done {
		t.Fatalf("the run was told %v, its last step took %q and is %s", got, events(last), last.State.Status)
	}
	if notes := w.keyNotes(sender); len(notes) != 0 {
		t.Fatalf("the sender's notes are %q", notes)
	}
	if left := w.list(store.Inbox("ctl")); len(left) != 0 {
		t.Fatalf("left waiting: %v", left)
	}
}

// The event names Corbie gives a machine cannot be sent, and a run's own
// timer and ticket names are undelivered at the run.
func Test_chart_send_cannot_take_an_events_own_name(t *testing.T) {
	for _, name := range []string{machine.Start, machine.Done, machine.Failed, machine.Timer} {
		_, err := machine.Parse([]byte(dedent(`
			statechart: s
			initial: a
			states:
			  a: { on: { x: { send: { to: { at: room, key: k }, event: `+name+` } } } }
		`)), nil)
		if err == nil || !strings.Contains(err.Error(), "no event can be sent") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	w := newWorld(t)
	owner := `
		statechart: owner
		allow: ["send:room"]
		initial: ready
		states:
		  ready:
		    on:
		      tick: { send: { to: { at: room, key: "chat:1" }, event: tick } }
		      ok: { send: { to: { at: room, key: "chat:1" }, event: ok } }
	`
	sender := w.rooms(map[string]string{"/t/room": `
		statechart: talk
		lifetime: keyed
		initial: open
		states:
		  open: { timers: { tick: 1h }, ticket: ok, on: { tick: open, ok: open, said: open } }
	`, "/t/owner": owner})
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	ticker := w.startAt(w.client, "/t/owner", client.Start{})
	w.settle()
	w.tell(ticker, "tick", nil)
	w.tell(ticker, "ok", nil)
	w.settle()
	if notes := w.keyNotes(ticker); len(notes) != 2 || !strings.Contains(notes[0], `"tick" is the name of an event the run gives itself`) || !strings.Contains(notes[1], `"ok"`) {
		t.Fatalf("the record says %v", notes)
	}
	if steps := w.steps(run); len(steps) != 1 {
		t.Fatalf("the run took %d steps", len(steps))
	}
}

// The body of a key's file is the controller's, signed and sealed to the
// controller alone.
func Test_store_key_file_is_signed_and_sealed(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	at := store.Key("ctl", "/t/room", "chat:1")
	data, _, err := w.owner.Read(ctx, at)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "chat:1") || strings.Contains(string(data), run) {
		t.Fatalf("the file is written in the clear: %s", data)
	}
	var entry instance.KeyFile
	if _, err := identity.Read(data, at, w.ctl.Card, w.c1, &entry); err == nil {
		t.Fatal("the client reads the file")
	}
	if _, err := identity.Read(data, at, w.ctl.Card, w.ctl, &entry); err != nil || entry.Run != run {
		t.Fatalf("the controller reads %+v, %v", entry, err)
	}
	if _, err := identity.Read(data, store.Key("ctl", "/t/room", "chat:2"), w.ctl.Card, w.ctl, &entry); err == nil {
		t.Fatal("a file checks under another key's name")
	}
	if got, _, _ := w.clientOf(w.c1).Store.Read(ctx, at); got != nil {
		t.Fatal("the client's store gives the file")
	}
	// A client's message that looks like the controller's is no message it
	// acts on.
	key := "chat:1"
	w.leave(w.c1, "20261010T120000.000000Z-ffffff", &instance.Message{Kind: instance.KindSend, Instance: run, From: run,
		To: &machine.Address{At: "/t/room", Key: &key}, Event: &machine.Event{Name: "said", Data: map[string]any{"text": "bye"}}})
	w.settle()
	if head := w.head(run); head.Status != instance.Running {
		t.Fatalf("a client's message ended the run: %+v", head)
	}
}

// twoToChild is a chart that sends its child two events as its parent, in one
// step.
const twoToChild = `
	statechart: twice
	initial: ready
	states:
	  ready:
	    job: { machine: ./child.yaml }
	    on:
	      both:
	        send:
	          - { to: ready, event: said, data: { text: a } }
	          - { to: ready, event: said, data: { text: b } }
	      done: over
	  over: { end: done }
`

// keyAndChild is a chart that sends one event to a key and one to its child,
// in one step.
const keyAndChild = `
	statechart: both
	allow: ["send:room"]
	initial: ready
	states:
	  ready:
	    job: { machine: ./child.yaml }
	    on:
	      both:
	        send:
	          - { to: { at: room, key: "chat:1" }, event: said, data: { text: a } }
	          - { to: ready, event: said, data: { text: b } }
	      done: over
	  over: { end: done }
`

// A step's events are written in the order sent, so a controller stopped
// between two of them never has a run hear the later and take the earlier for
// a copy.
func Test_ctl_events_of_a_step_stopped_after_every_write(t *testing.T) {
	child := strings.Replace(talk, "\tlifetime: keyed\n", "", 1)
	for _, chart := range []string{keyAndChild, twoToChild} {
		play := func(stopAfter int64) ([]string, bool) {
			w := newWorld(t)
			w.roomsWith(map[string]string{"/t/both": chart}, map[string]string{"child.yaml": child})
			parent := w.startAt(w.client, "/t/both", client.Start{})
			w.settle()
			w.tell(parent, "both", nil)
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
				told = append(told, w.told(run)...)
			}
			slices.Sort(told)
			return told, s.stopped.Load()
		}
		for n := int64(0); ; n++ {
			got, stopped := play(n)
			if !stopped {
				break
			}
			if !slices.Equal(got, []string{"a", "b"}) {
				t.Fatalf("stopped after %d writes the runs were told %v", n, got)
			}
		}
	}
}

// Two runs whose IDs begin alike are two senders, and a client is a sender
// apart from every run.
func Test_ctl_senders_with_names_that_begin_alike_are_apart(t *testing.T) {
	w := newWorld(t)
	w.rooms(nil)
	stamp := store.Stamp(w.now)
	start := func(run string) string {
		w.t.Helper()
		return w.startAt(w.client, "/t/router", client.Start{Run: run})
	}
	a := start("c1/" + stamp + "-aa")
	b := start("c1/" + stamp + "-aa.b")
	w.settle()
	w.say(a, "say", 1, "one")
	w.settle()
	w.say(b, "say", 1, "two")
	w.settle()
	key := "chat:1"
	if _, err := w.client.Send(ctx, machine.Address{At: "/t/room", Key: &key}, "said", map[string]any{"text": "three"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	w.say(a, "say", 1, "four")
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 || !slices.Equal(w.told(runs[0]), []string{"one", "two", "three", "four"}) {
		t.Fatalf("the runs are %v, told %v", runs, w.told(runs[0]))
	}
	if heard := w.latest(runs[0]).State.Heard; len(heard) != 3 {
		t.Fatalf("the run remembers %+v", heard)
	}
}

// No client can send an event under a name Corbie gives a machine.
func Test_mach_no_event_takes_a_name_of_corbies(t *testing.T) {
	for _, name := range []string{machine.Start, machine.Done, machine.Failed, machine.Timer} {
		w := newWorld(t)
		sender := w.rooms(nil)
		message, err := w.client.Send(ctx, w.to(sender), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		if r := w.receipt(message); r == nil || r.Taken || !strings.Contains(r.Refused, "no event can be sent") {
			t.Fatalf("%s: the receipt is %+v", name, r)
		}
		if st := w.latest(sender).State; st.Status != instance.Running || len(w.steps(sender)) != 1 {
			t.Fatalf("%s: the chart is %s", name, st.Status)
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
// an address.
func (w *world) keyedMessage() (string, []byte) {
	w.t.Helper()
	var found []string
	for _, name := range w.list(store.Inbox("ctl")) {
		data, _, err := w.owner.Read(ctx, name)
		if err != nil {
			w.t.Fatal(err)
		}
		var msg instance.Message
		if _, err := identity.Read(data, name, w.ctl.Card, w.ctl, &msg); err == nil && msg.Kind == instance.KindSend && msg.From != "" {
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
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "one")
	w.pass() // routed
	w.pass() // the sender's step
	name, data := w.keyedMessage()
	w.settle()
	first := w.runsOf("talk")[0]
	var others []string
	for range 65 {
		others = append(others, w.startAt(w.client, "/t/router", client.Start{}))
	}
	w.settle()
	for i, other := range others[:64] {
		w.say(other, "say", 1, fmt.Sprint(i))
	}
	w.settle()
	if _, err := w.owner.Write(ctx, name, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if got := w.told(first); len(got) != 65 {
		t.Fatalf("the run was told %d things", len(got))
	}
	w.say(others[64], "say", 1, "bye")
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
	sender := w.rooms(map[string]string{"/t/bell": `
		statechart: bell
		lifetime: singleton
		initial: a
		states:
		  a: { on: { rang: a } }
	`})
	bell := ""
	for run := range w.at("/t/bell") {
		bell = run
	}
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.pass() // routed
	w.pass() // the sender's step
	name, data := w.keyedMessage()
	if _, err := w.owner.Delete(ctx, name, store.Overwrite); err != nil {
		t.Fatal(err)
	}
	// The bell's run ends, and the message finds it ended.
	if _, err := w.client.Cancel(ctx, bell, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	var msg instance.Message
	if _, err := identity.Read(data, name, w.ctl.Card, w.ctl, &msg); err != nil {
		t.Fatal(err)
	}
	msg.Instance = sender
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
	w.pass()
	if notes := w.keyNotes(sender); len(notes) != 0 {
		t.Fatalf("the record says %v", notes)
	}
}

// emptying deletes what is under a name once a write is made under another.
type emptying struct {
	store.Store
	when, under string
	done        atomic.Bool
}

func (e *emptying) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	written, err := e.Store.Write(ctx, name, value, how)
	if err == nil && strings.HasPrefix(name, e.when) && !e.done.Swap(true) {
		names, err := store.Names(e.Store.List(ctx, e.under))
		if err != nil {
			return false, err
		}
		for _, n := range names {
			if _, err := e.Store.Delete(ctx, n, store.Overwrite); err != nil {
				return false, err
			}
		}
	}
	return written, err
}

// A run a send starts that fails as it starts, for a definition that is no
// longer in the store, for its depth or for its input, adds the reason to
// its sender's record. Its first step is failed, and its key is free.
func Test_ctl_sent_root_fails_at_start_and_tells_its_sender(t *testing.T) {
	for _, c := range []struct {
		name string
		when string // the definition's blobs go once a write is made under it
		want string
	}{
		{"gone before it starts", store.Inbox("ctl") + "/ctl/", "its machine is no longer in the store"},
		{"gone after its key is taken", store.Keys("ctl", "/t/room"), "its machine is no longer in the store"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			sender := w.rooms(nil)
			w.ctrl.Store = &emptying{Store: w.ctrl.Store, when: c.when, under: store.LocationBlobs("ctl", "/t/room")}
			w.say(sender, "say", 1, "one")
			w.settle()
			notes := w.keyNotes(sender)
			if len(notes) != 1 || !strings.Contains(notes[0], c.want) {
				t.Fatalf("the sender's record says %q", notes)
			}
			var runs []string
			for _, at := range w.list(store.Finished("ctl")) {
				if run := store.InstanceOf("ctl", at); w.head(run).Location == "/t/room" && !slices.Contains(runs, run) {
					runs = append(runs, run)
				}
			}
			if len(runs) != 1 {
				t.Fatalf("the runs at /t/room are %v", runs)
			}
			if st := w.latest(runs[0]).State; st.Status != instance.Failed || !strings.Contains(st.Error.Error, c.want) {
				t.Fatalf("the run is %s: %+v", st.Status, st.Error)
			}
			if entry := w.entry("/t/room", "chat:1"); entry != nil {
				t.Fatalf("the key is held by %+v", entry)
			}
			if left := w.list(store.Inbox("ctl")); len(left) != 0 {
				t.Fatalf("left waiting: %v", left)
			}
		})
	}
}
