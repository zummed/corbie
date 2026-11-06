package controller_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
)

// bell is a singleton that takes rang and any event, and answers its sender.
const bell = `
	statechart: bell
	lifetime: singleton
	allow: ["send:/**"]
	initial: a
	states:
	  a:
	    on:
	      rang: { send: { to: "${event.from}", event: said, data: { text: "${event.from}" } } }
	      said: a
`

// talkFrom is the keyed room with a receive: list.
func talkFrom(receive string) string {
	return strings.Replace(talk, "lifetime: keyed", "lifetime: keyed\n\treceive: "+receive, 1)
}

// clientSend has a client send to an address written as /location,
// /location#key, or a run's ID, and gives the receipt.
func (w *world) clientSend(c *client.Client, to, event string, data any) *instance.Receipt {
	w.t.Helper()
	addr, err := c.Address(ctx, to)
	if err != nil {
		w.t.Fatal(err)
	}
	message, err := c.Send(ctx, addr, event, data, time.Hour)
	if err != nil {
		w.t.Fatal(err)
	}
	w.settle()
	r, err := c.Receipt(ctx, message)
	if err != nil || r == nil {
		w.t.Fatalf("no receipt for the send to %s: %v", to, err)
	}
	return r
}

// other enrols a second client with a ceiling.
func (w *world) other(name string, allow ...string) *client.Client {
	w.t.Helper()
	id := w.party(name, identity.Client)
	w.enrol(id, permission.Layer{Allow: allow}, nil)
	return w.clientOf(id)
}

// With no receive:, a run takes what is sent from every location of its own
// tree and from its tree's client, and nothing from another tree or client.
func Test_ctl_receive_default_is_own_tree_and_owner(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.say(sender, "say", 1, "from the tree")
	w.settle()
	away := strings.Replace(strings.Replace(router, "at: room", "at: /t/room", 1), `"send:**"`, `"send:/t/**"`, 1)
	at, _ := w.place(w.client, away, nil)
	far := w.startAt(w.client, at, client.Start{})
	w.settle()
	w.say(far, "say", 1, "from another tree")
	w.settle()
	if notes := w.keyNotes(far); len(notes) != 1 || !strings.Contains(notes[0], "/t/room does not receive said from the run "+far) {
		t.Fatalf("the other tree's record says %v", notes)
	}
	if r := w.clientSend(w.client, "/t/room#chat:1", "said", map[string]any{"text": "from the client"}); !r.Taken {
		t.Fatalf("the tree's client was refused: %s", r.Refused)
	}
	b := w.other("b", "send:/t/**")
	if r := w.clientSend(b, "/t/room#chat:1", "said", map[string]any{"text": "from b"}); r.Taken || !strings.Contains(r.Refused, "does not receive said from client:b") {
		t.Fatalf("another client got %+v", r)
	}
	runs := w.runsOf("talk")
	if len(runs) != 1 || !slices.Equal(w.told(runs[0]), []string{"from the tree", "from the client"}) {
		t.Fatalf("the runs are %v", runs)
	}
}

// A receive: list names exactly who may send: locations read from the tree's
// prefix, and clients. The tree's own client is always taken.
func Test_ctl_receive_list_names_who_may_send(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{"/t/room": talkFrom("[a/*, client:panel]"), "/t/a/one": router, "/t/b": router})
	one := w.startAt(w.client, "/t/a/one", client.Start{})
	b := w.startAt(w.client, "/t/b", client.Start{})
	w.settle()
	w.say(one, "say", 1, "from a/one")
	w.say(b, "say", 1, "from b")
	w.settle()
	if notes := w.keyNotes(b); len(notes) != 1 || !strings.Contains(notes[0], "does not receive") {
		t.Fatalf("b's record says %v", notes)
	}
	panel := w.other("panel", "send:/t/**")
	if r := w.clientSend(panel, "/t/room#chat:1", "said", map[string]any{"text": "from panel"}); !r.Taken {
		t.Fatalf("panel was refused: %s", r.Refused)
	}
	if r := w.clientSend(w.client, "/t/room#chat:1", "said", map[string]any{"text": "from the owner"}); !r.Taken {
		t.Fatalf("the owner was refused: %s", r.Refused)
	}
	x := w.other("x", "send:/t/**")
	if r := w.clientSend(x, "/t/room#chat:1", "said", map[string]any{"text": "from x"}); r.Taken {
		t.Fatalf("x was taken: %+v", r)
	}
	runs := w.runsOf("talk")
	if len(runs) != 1 || !slices.Equal(w.told(runs[0]), []string{"from a/one", "from panel", "from the owner"}) {
		t.Fatalf("the runs are %v, told %v", runs, w.told(runs[0]))
	}
}

// A receive: mapping lists who may send each event; an event it does not
// name is taken from no one but the tree's client.
func Test_ctl_receive_mapping_is_by_event(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/room": talkFrom("{ said: [router], stop: [client:panel] }")})
	w.say(sender, "say", 1, "one")
	w.settle()
	run := w.runsOf("talk")[0]
	panel := w.other("panel", "send:/t/**")
	if r := w.clientSend(panel, "/t/room#chat:1", "said", map[string]any{"text": "two"}); r.Taken || !strings.Contains(r.Refused, "does not receive said") {
		t.Fatalf("panel's said got %+v", r)
	}
	if r := w.clientSend(panel, "/t/room#chat:1", "stop", nil); !r.Taken || r.Run != run {
		t.Fatalf("panel's stop got %+v", r)
	}
	if r := w.clientSend(panel, "/t/room#chat:1", "other", nil); r.Taken {
		t.Fatalf("panel's other was taken: %+v", r)
	}
	if r := w.clientSend(w.client, run, "other", nil); !r.Taken {
		t.Fatalf("the owner's other was refused: %s", r.Refused)
	}
	if got := w.told(run); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("the run was told %v", got)
	}
}

// A send to a run by its ID is taken only by a run at the location it names:
// a child is at its parent's location.
func Test_ctl_send_to_a_run_not_at_its_address_is_undelivered(t *testing.T) {
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
	kid := w.child(parent)
	w.say(sender, "say", 1, "one")
	w.settle()
	room := w.runsOf("talk")
	w.tell(sender, "poke", map[string]any{"at": "router", "run": kid, "text": "wrong"})
	w.tell(sender, "poke", map[string]any{"at": "parent", "run": kid, "text": "right"})
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "is not at /t/router") {
		t.Fatalf("the record says %v", notes)
	}
	if got := w.told(kid); !slices.Equal(got, []string{"right"}) || len(room) != 2 {
		t.Fatalf("the child was told %v; the runs are %v", got, room)
	}
}

// A send that a layer of the sender does not allow fails the chart as
// denied, naming the layer and the line that allows it. The runner's ceiling
// takes no part.
func Test_ctl_send_denied_by_a_layer_fails_the_chart(t *testing.T) {
	quiet := strings.Replace(router, `allow: ["send:**"]`, `allow: ["net:example.com"]`, 1)
	w := newWorld(t)
	w.rooms(map[string]string{"/t/quiet": quiet})
	run := w.startAt(w.client, "/t/quiet", client.Start{})
	w.settle()
	w.say(run, "say", 1, "one")
	w.settle()
	st := w.latest(run).State
	if st.Status != instance.Failed || st.Error.Status != machine.StatusDenied || st.Error.Refused != "send:/t/room" ||
		!strings.Contains(st.Error.Layer, `the definition of "router"`) || !strings.Contains(st.Error.Fix, "send:/t/room") {
		t.Fatalf("the chart is %s: %+v", st.Status, st.Error)
	}

	w = newWorld(t)
	w.rooms(nil)
	run = w.startAt(w.client, "/t/router", client.Start{Allow: &permission.Layer{Allow: []string{"net:example.com"}}})
	w.settle()
	w.say(run, "say", 1, "one")
	w.settle()
	if st := w.latest(run).State; st.Status != instance.Failed || !strings.Contains(st.Error.Layer, "the list the run was started with") {
		t.Fatalf("the chart is %s: %+v", st.Status, st.Error)
	}

	w = newWorld(t)
	w.enrol(w.c1, permission.Layer{Allow: []string{"at:/**"}}, nil)
	w.rooms(nil)
	run = w.startAt(w.client, "/t/router", client.Start{})
	w.settle()
	w.say(run, "say", 1, "one")
	w.settle()
	if st := w.latest(run).State; st.Status != instance.Failed || !strings.Contains(st.Error.Layer, `the ceiling of the client "c1"`) ||
		!strings.Contains(st.Error.Fix, "corbie controller enrol c1 --allow send:/t/room") {
		t.Fatalf("the chart is %s: %+v", st.Status, st.Error)
	}
	if len(w.runsOf("talk")) != 0 {
		t.Fatalf("a denied send started %v", w.runsOf("talk"))
	}
}

// The tree's own client sends to its tree with no send: entry.
func Test_ctl_owner_client_needs_no_send_entry(t *testing.T) {
	w := newWorld(t)
	w.enrol(w.c1, permission.Layer{Allow: []string{"at:/t"}}, nil)
	w.rooms(nil)
	if r := w.clientSend(w.client, "/t/room#k", "said", map[string]any{"text": "one"}); !r.Taken || r.Run == "" {
		t.Fatalf("the owner's send got %+v", r)
	}
}

// Another client needs send: in its ceiling, and its name in receive:.
func Test_ctl_other_client_needs_send_and_receive(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{"/t/room": talkFrom("[client:b]")})
	without := w.other("b", "net:example.com")
	if r := w.clientSend(without, "/t/room#k", "said", map[string]any{"text": "x"}); r.Taken || !strings.Contains(r.Refused, "send:/t/room") ||
		!strings.Contains(r.Refused, "corbie controller enrol b --allow send:/t/room") {
		t.Fatalf("b with no send: got %+v", r)
	}
	b := w.other("b", "send:/t/**")
	if r := w.clientSend(b, "/t/room#k", "said", map[string]any{"text": "one"}); !r.Taken {
		t.Fatalf("b was refused: %s", r.Refused)
	}
	c := w.other("c", "send:/t/**")
	if r := w.clientSend(c, "/t/room#k", "said", map[string]any{"text": "two"}); r.Taken || !strings.Contains(r.Refused, "does not receive said from client:c") {
		t.Fatalf("c got %+v", r)
	}
}

// A client's send is answered by the run that takes it: the receipt names
// the run and whether the machine acted on the event. Sent twice, the answer
// is the same.
func Test_ctl_client_send_receipt_names_the_run_and_whether_it_acted(t *testing.T) {
	w := newWorld(t)
	w.rooms(nil)
	r := w.clientSend(w.client, "/t/room#k", "said", map[string]any{"text": "one"})
	runs := w.runsOf("talk")
	if !r.Taken || len(runs) != 1 || r.Run != runs[0] || r.Acted == nil || !*r.Acted {
		t.Fatalf("the start's receipt is %+v, the runs %v", r, runs)
	}
	to := machine.Address{At: "/t/room", Key: new(string)}
	*to.Key = "k"
	message, err := w.client.Send(ctx, to, "nothing", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(message); !r.Taken || r.Run != runs[0] || r.Acted == nil || *r.Acted {
		t.Fatalf("the receipt of an event with no rule is %+v", r)
	}
	again := *w.receipt(message)
	w.leave(w.c1, message, &instance.Message{Kind: instance.KindSend, To: &to, Event: &machine.Event{Name: "nothing"}})
	w.settle()
	if r := w.receipt(message); r.Taken != again.Taken || r.Run != again.Run || len(w.steps(runs[0])) != 2 {
		t.Fatalf("sent again, the receipt is %+v and the run took %d steps", r, len(w.steps(runs[0])))
	}
}

// A client's sends to one new key start one run, and arrive in the order
// sent.
func Test_ctl_client_sends_arrive_in_order(t *testing.T) {
	w := newWorld(t)
	w.rooms(nil)
	key := "k"
	var want []string
	for i := range 20 {
		text := string(rune('a' + i))
		want = append(want, text)
		if _, err := w.client.Send(ctx, machine.Address{At: "/t/room", Key: &key}, "said", map[string]any{"text": text}, time.Hour); err != nil {
			t.Fatal(err)
		}
		if i%6 == 0 {
			w.pass()
		}
	}
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 || !slices.Equal(w.told(runs[0]), want) {
		t.Fatalf("the runs are %v, told %v", runs, w.told(runs[0]))
	}
}

// The controller writes the sender into event.from: { at, run } for a run,
// with key for a keyed run, and { client } for a client. A reply to it
// arrives at the sender, and one to a client is noted as undelivered.
func Test_ctl_event_from_is_the_senders_address(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/bell": bell, "/t/asker": `
		statechart: asker
		lifetime: keyed
		allow: ["send:bell"]
		initial: a
		states:
		  a: { entry: { send: { to: { at: bell }, event: rang } }, on: { said: b } }
		  b: { on: { x: b } }
	`})
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.settle()
	if got := w.told(sender); len(got) != 1 || !strings.Contains(got[0], "/t/router") || !strings.Contains(got[0], sender) {
		t.Fatalf("the reply to the router was %v", got)
	}
	w.clientSend(w.client, "/t/asker#k1", "go", nil)
	asker := ""
	for _, run := range w.runsOf("asker") {
		asker = run
	}
	if got := w.told(asker); len(got) != 1 || !strings.Contains(got[0], "key:k1") {
		t.Fatalf("the reply to the keyed run was %v", got)
	}
	r := w.clientSend(w.client, "/t/bell", "rang", nil)
	if !r.Taken {
		t.Fatalf("the client's send was refused: %s", r.Refused)
	}
	var b string
	for run := range w.at("/t/bell") {
		b = run
	}
	notes := w.keyNotes(b)
	if len(notes) != 1 || !strings.Contains(notes[0], "a client has no inbox") {
		t.Fatalf("the bell's record says %v", notes)
	}
	for _, step := range w.steps(b) {
		for _, e := range step.Events {
			if e.Name == "rang" && e.Sender != nil && e.Sender.Client != "" && e.Sender.Client != "c1" {
				t.Fatalf("the event came from %+v", e.Sender)
			}
		}
	}
}

// limits: { runs } caps the runs of a keyed location: a send to a new key
// past it is noted, sends to running keys go through, and once a run ends a
// new key starts.
func Test_ctl_limit_runs_caps_new_keys(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/room": strings.Replace(talk, "lifetime: keyed", "lifetime: keyed\n\tlimits: { runs: 2 }", 1)})
	w.say(sender, "say", 1, "one")
	w.say(sender, "say", 2, "two")
	w.settle()
	w.say(sender, "say", 3, "three")
	w.say(sender, "say", 1, "again")
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "/t/room is at its limit of 2 runs") {
		t.Fatalf("the record says %v", notes)
	}
	if runs := w.runsOf("talk"); len(runs) != 2 {
		t.Fatalf("the runs are %v", runs)
	}
	w.say(sender, "say", 2, "bye")
	w.settle()
	w.say(sender, "say", 3, "three")
	w.settle()
	if runs := w.runsOf("talk"); len(runs) != 3 {
		t.Fatalf("after one ended the runs are %v", runs)
	}
}

// A message in flight to a run that ended moves on to start a fresh run only
// where the location's limit leaves room.
func Test_ctl_limit_runs_checked_when_a_message_moves_on(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/room": strings.Replace(talk, "lifetime: keyed", "lifetime: keyed\n\tlimits: { runs: 1 }", 1)})
	w.say(sender, "say", 1, "one")
	w.settle()
	w.say(sender, "say", 1, "bye")
	w.say(sender, "say", 1, "late")
	w.pass() // routed
	w.pass() // both go to the run with the key
	// Another key takes the only place before the late event moves on.
	w.writeEntry("/t/room", "other", "c1/20261010T120000.000Z-0000000001", w.list("ctl/locations/t/room/+")[0])
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "at its limit of 1") {
		t.Fatalf("the record says %v", notes)
	}
	if runs := w.runsOf("talk"); len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
}

// A send to a key at a location whose file has gone is noted.
func Test_ctl_send_to_a_location_that_was_removed_is_undelivered(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	w.submitTree(w.client, w.tree("/t", "/srv/t", map[string]string{"/t/router": router}, nil))
	w.say(sender, "say", 1, "one")
	w.settle()
	if notes := w.keyNotes(sender); len(notes) != 1 || !strings.Contains(notes[0], "nothing is recorded at /t/room") {
		t.Fatalf("the record says %v", notes)
	}
}

// A send to a client's address is noted as undelivered: a client has no
// inbox.
func Test_ctl_send_to_client_is_undelivered(t *testing.T) {
	w := newWorld(t)
	w.rooms(map[string]string{"/t/caller": `
		statechart: caller
		allow: ["send:/**"]
		initial: a
		states:
		  a: { entry: { send: { to: "${input.to}", event: hello } }, on: { x: a } }
	`})
	run := w.startAt(w.client, "/t/caller", client.Start{Input: map[string]any{"to": map[string]any{"client": "c1"}}})
	w.settle()
	if notes := w.keyNotes(run); len(notes) != 1 || !strings.Contains(notes[0], "a client has no inbox") {
		t.Fatalf("the record says %v", notes)
	}
}

// A send to a singleton in flight when a submit restarts it goes to the new
// run, through the singleton's key file.
func Test_ctl_singleton_restart_passes_a_send_in_flight_to_the_new_run(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(map[string]string{"/t/bell": bell})
	var old string
	for run := range w.at("/t/bell") {
		old = run
	}
	w.tell(sender, "ring", map[string]any{"at": "bell"})
	w.pass() // routed
	w.pass() // the send names the run there now
	name, _ := w.keyedMessage()
	inner := w.ctrl.Store
	w.afresh(&hiding{Store: inner, hide: func(n string) bool { return n == name }})
	changed := strings.Replace(bell, "initial: a", "title: changed\n\tinitial: a", 1)
	w.submitTree(w.client, w.tree("/t", "/srv/t", map[string]string{"/t/router": router, "/t/room": talk, "/t/bell": changed}, nil))
	w.afresh(inner)
	w.settle()
	if w.head(old).Status != instance.Cancelled {
		t.Fatalf("the old run is %s", w.head(old).Status)
	}
	var fresh string
	for run, status := range w.at("/t/bell") {
		if run != old && status == instance.Running {
			fresh = run
		}
	}
	rang := false
	for _, step := range w.steps(fresh) {
		for _, e := range step.Events {
			rang = rang || e.Name == "rang"
		}
	}
	if fresh == "" || !rang {
		t.Fatalf("the new run %q was not given the event", fresh)
	}
}

// Only the controller says who sent an event: a sender a client writes in its
// message is replaced with the client's name.
func Test_ctl_client_cannot_write_its_sender(t *testing.T) {
	w := newWorld(t)
	sender := w.rooms(nil)
	key := "k"
	w.leave(w.c1, "20261010T120000.000000Z-forged", &instance.Message{Kind: instance.KindSend,
		To: &machine.Address{At: "/t/room", Key: &key}, Sender: &machine.Address{At: "/t/router", Run: sender},
		Event: &machine.Event{Name: "said", Data: map[string]any{"text": "forged"}}})
	w.settle()
	runs := w.runsOf("talk")
	if len(runs) != 1 {
		t.Fatalf("the runs are %v", runs)
	}
	for _, step := range w.steps(runs[0]) {
		for _, e := range step.Events {
			if e.Name == "said" && (e.Sender == nil || e.Sender.Client != "c1" || e.Sender.Run != "") {
				t.Fatalf("the event came from %+v", e.Sender)
			}
		}
	}
	if w.head(runs[0]).StartedBy != "" {
		t.Fatalf("the run names %s as its sender", w.head(runs[0]).StartedBy)
	}
}
