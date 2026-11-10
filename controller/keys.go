package controller

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// An event sent to a run by its address is a message the controller writes
// in its own inbox. These are the rules every part of this file keeps:
//
//  1. A run takes its key's file, only-if-new, before its first step is
//     written, so a run that loses has done nothing. A singleton's key is
//     the empty key.
//  2. The step that ends a run is written first and the file is deleted
//     after, so a run that has the key is running or has just ended.
//  3. An event sent to a singleton's { at } or to { at, key } leaves the
//     inbox only when a step of a running run used it. Until then it stays
//     under the name its sender's step gave it, and where its run has ended,
//     or lost its key, the message is rewritten in place to name the run
//     that has the key.
//  4. An event sent to { at, run } goes to that run alone, and is noted as
//     undelivered when that run cannot take it.
//  5. An event sent to a transient location's { at } starts a new run, as a
//     key no other send names would. The run takes a key file under its own
//     ID only where its file has limits: { runs }, so that it is counted.
//
// So a send reads the file once and fixes the run it goes to in its step. A
// carry-out done again goes to the same run, which has heard it.

// isSend says an arrival is an event the controller carried for a run or a
// client that sent it to an address. No other party writes one.
func (a *arrival) isSend() bool {
	return !a.wake && a.msg.Kind == instance.KindSend && a.kind == identity.Controller && a.msg.To != nil
}

// movable says an arrival is an event sent to a singleton's { at } or to
// { at, key }, which goes on to the run that has the key now.
func (a *arrival) movable() bool { return a.isSend() && a.msg.To.Run == "" && !a.fresh() }

// fresh says an arrival is an event sent to a transient location's { at },
// which starts a run of its own and goes nowhere else.
func (a *arrival) fresh() bool {
	return a.isSend() && a.msg.To.Run == "" && a.msg.To.Key == nil && a.msg.Start != nil
}

// starts says an arrival starts an instance: a start, or an event sent to a
// key that names the run it starts.
func (a *arrival) starts(inst string) bool {
	if a.wake || a.msg.Start == nil {
		return false
	}
	return a.msg.Kind == instance.KindStart || a.isSend() && a.msg.Start.Run == inst
}

// senderOf names the sender of a message the controller wrote for a run:
// the run, or client:<name> for a client.
func senderOf(msg *instance.Message) string {
	if msg.Sender != nil && msg.Sender.Client != "" {
		return "client:" + msg.Sender.Client
	}
	return msg.From
}

// clientPrefix begins the name of the message the controller writes for a
// client's send, followed by the client and the name of its own message.
const clientPrefix = "client."

// client gives the client a message answers to and the name of its message:
// a client's own, or the one the controller wrote for a client's send or
// call.
func (a *arrival) client() (sender, message string, ok bool) {
	switch {
	case a.wake:
		return "", "", false
	case a.kind == identity.Client:
		return a.sender, a.name, true
	case a.isSend() && a.msg.Sender != nil && a.msg.Sender.Client != "":
		base := a.name[strings.LastIndexByte(a.name, '/')+1:]
		return a.msg.Sender.Client, strings.TrimPrefix(base, clientPrefix+a.msg.Sender.Client+"."), true
	case a.kind == identity.Controller && a.msg.Kind == instance.KindStart && isCall(a.msg.Start) && a.msg.Start.Above.CallReceipt != nil:
		// The start of a run a client called answers the client's call.
		call := a.msg.Start.Above.CallReceipt
		return call.Client, call.Message, true
	}
	return "", "", false
}

// keyOf gives the key an address names: none for { at } and { at, run }.
func keyOf(a *machine.Address) string {
	if a.Key == nil {
		return ""
	}
	return *a.Key
}

// addressText writes an address as a note shows it: /loc, /loc#key, the
// run, or client:<name>.
func addressText(a *machine.Address) string {
	switch {
	case a == nil:
		return ""
	case a.Client != "":
		return "client:" + a.Client
	case a.Run != "":
		return fmt.Sprintf("the run %s at %s", a.Run, a.At)
	case a.Key != nil:
		return a.At + "#" + *a.Key
	}
	return a.At
}

// sendingTo is one sender's events to one key at one location.
type sendingTo struct{ from, at, key string }

// sendingAt is one sender's events to one location.
type sendingAt struct{ from, at string }

// ahead is what the inbox holds of each sender's events to keys, in the
// order it is read.
type ahead struct {
	to   map[sendingTo]string          // the run the earliest goes to
	held map[sendingTo]bool            // one waits, and so does every later one
	at   map[sendingAt]map[string]bool // every run they go to
}

// behind says whether an event sent to an address must wait for the next
// pass. One sent to { at } or { at, key } waits while an earlier one from
// the same sender to the same key is on its way to another run, and is taken
// first. One sent to { at, run } waits while an earlier one from the same
// sender to the same location goes to another run: it may be moving on to
// that run, and it is taken first. It is why an event is never heard out of
// order. The inbox is read in order.
func (p *pass) behind(a *arrival, seen *ahead) bool {
	if !a.isSend() || a.fresh() {
		return false
	}
	msg := a.msg
	at := sendingAt{senderOf(msg), msg.To.At}
	if msg.To.Run != "" {
		for run := range seen.at[at] {
			if run != msg.To.Run {
				p.later()
				return true
			}
		}
		return false
	}
	if seen.at[at] == nil {
		seen.at[at] = map[string]bool{}
	}
	seen.at[at][msg.Instance] = true
	to := sendingTo{at.from, at.at, keyOf(msg.To)}
	run, ok := seen.to[to]
	switch {
	case !ok:
		seen.to[to] = msg.Instance
		return false
	case run == msg.Instance && !seen.held[to]:
		return false
	}
	// Once one waits, every later one to the key waits behind it.
	seen.held[to] = true
	p.later()
	return true
}

// target is what the store says of a key at a location: the run that has
// it, or, with none, the location's file and how many runs it has.
type target struct {
	holder string
	file   *located
	runs   int
}

// aimAt reads what a send to { at } or { at, key } needs: the key's file
// and, where it names no run, the location's file and, for a location with
// a limit on its runs, how many key files it has. live says the run the
// file names must still have the key: a send takes the file's word, and the
// event moves on from a run that has ended.
func (p *pass) aimAt(at, key string, live bool) (*target, error) {
	t := &target{}
	file, _, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, store.Key(p.ctl, at, key), key)
	if err != nil && !isAltered(err) {
		return nil, err
	}
	if file != nil {
		t.holder = file.Run
		if live {
			if has, err := p.live(file); err != nil || !has {
				t.holder = ""
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if t.holder != "" {
		return t, nil
	}
	if t.file, err = p.readLocated(at); err != nil && !isAltered(err) {
		return nil, err
	}
	if t.file == nil || t.file.Runs == 0 {
		return t, nil
	}
	files, err := store.Names(p.store.List(p.ctx, store.Keys(p.ctl, at)))
	t.runs = len(files)
	return t, err
}

// aim says where a send to { at } or { at, key } goes: to the run that has
// the key, or to a run it starts, which the caller names; or why it is not
// delivered. A send to a transient location's { at } always starts one.
func (t *target) aim(at string, keyed bool) (to string, starts bool, why string) {
	switch {
	case t.holder != "":
		return t.holder, false, ""
	case t.file == nil:
		return "", false, fmt.Sprintf("nothing is recorded at %s", at)
	}
	if why := machine.Addressable(at, t.file.Lifetime, keyed); why != "" {
		return "", false, why
	}
	switch {
	case !keyed && t.file.Lifetime != machine.LifetimeTransient:
		return "", false, fmt.Sprintf("no run of the singleton at %s is running", at)
	case t.file.Runs > 0 && t.runs >= t.file.Runs:
		return "", false, fmt.Sprintf("%s is at its limit of %d runs", at, t.file.Runs)
	}
	return "", true, ""
}

// addressed is one key at one location a step sends to.
type addressed struct{ at, key string }

// readAddresses reads what each send of a step to { at } or { at, key }
// needs, side by side and once for each. A step that does not fit in a
// pass is never written, and one pass can read many.
func (b *build) readAddresses(requests []machine.Request) error {
	if b.targets == nil {
		b.targets = map[addressed]*target{}
	}
	var places []addressed
	for _, r := range requests {
		if r.Send == nil || r.Send.At == nil || r.Send.At.Client != "" || r.Send.At.Run != "" {
			continue
		}
		at := machine.Resolve(b.st.Tree, r.Send.At.At)
		if store.CheckLocation(at) != nil || r.Send.At.Key != nil && machine.CheckKey(*r.Send.At.Key) != nil {
			continue
		}
		place := addressed{at, keyOf(r.Send.At)}
		if _, seen := b.targets[place]; !seen {
			b.targets[place] = nil
			places = append(places, place)
		}
	}
	found := make([]*target, len(places))
	err := b.p.each(len(places), func(i int) error {
		var err error
		found[i], err = b.p.aimAt(places[i].at, places[i].key, false)
		return err
	})
	for i, place := range places {
		b.targets[place] = found[i]
	}
	return err
}

// sendsTo says a request sends to a run by its address.
func sendsTo(r machine.Request) bool { return r.Send != nil && r.Send.At != nil }

// refusedSend says why a send to a location is refused, if it is: a layer
// of the run refuses it, or its tree client is not known to the controller.
// The layers are the sender's own lists, those of the definition for a send
// action and those of the job for a call, and the lists the run was started
// under, read from the tree's prefix, and then its tree client's ceiling,
// which is written in full. The runner's ceiling takes no part: no runner
// carries a send.
func (b *build) refusedSend(at string, own permission.Layer) (*machine.Error, error) {
	denied := func(r *permission.Refusal) *machine.Error {
		return &machine.Error{Status: machine.StatusDenied, Error: r.Error(), Refused: r.What, Layer: r.Layer, Fix: r.Fix}
	}
	layers := append([]permission.Layer{own}, b.st.Above.Layers...)
	if r := (&permission.Set{Layers: layers, Prefix: b.st.Tree}).Check(permission.Send, at); r != nil {
		return denied(r), nil
	}
	name := store.InstanceClient(b.inst)
	client, err := b.p.trusted(name)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return b.p.failf("the client %q is not known to the controller", name), nil
	}
	if r := (&permission.Set{Layers: []permission.Layer{ceilingOf(client)}}).Check(permission.Send, at); r != nil {
		return denied(r), nil
	}
	return nil, nil
}

// ceilingOf is a client's ceiling as a layer, by name.
func ceilingOf(client *identity.Trust) permission.Layer {
	ceiling := client.Ceiling
	ceiling.Name = fmt.Sprintf("the ceiling of the client %q", client.Card.Name)
	ceiling.Fix = fmt.Sprintf("the controller's owner runs: corbie controller enrol %s --allow %%s", client.Card.Name)
	return ceiling
}

// sendTo takes a machine's request to send an event to a run by its
// address. A send the run's own layers refuse ends the run as denied, and it
// says so; what cannot be delivered is noted in the record and fails
// nothing.
func (b *build) sendTo(s *machine.Send) (denied bool) {
	ask := instance.Ask{Kind: instance.AskEvent, Event: &machine.Event{Name: s.Event, Data: s.Data}}
	at, why := b.addressAt(s.At)
	if why == "" {
		if b.read.Unfilled != "" {
			b.end(instance.Failed, nil, b.p.failf("%s", b.read.Unfilled))
			return true
		}
		own := fmt.Sprintf("the definition of %q", b.def.Name)
		refused, err := b.refusedSend(at, permission.Layer{Name: own, Allow: b.read.Allow, Deny: b.def.Deny, Fix: "add the line %s to the allow list of " + own})
		if err != nil {
			b.err = err
			return true
		}
		if refused != nil {
			b.end(instance.Failed, nil, refused)
			return true
		}
	}
	b.aim(&ask, s, at, why)
	return false
}

// addressAt gives the location an address names, read from the run's tree,
// or why a send to it is not delivered.
func (b *build) addressAt(addr *machine.Address) (at, why string) {
	switch {
	case addr.Client != "":
		return "", "a client has no inbox"
	case !strings.HasPrefix(addr.At, "/") && b.st.Tree == "":
		return "", fmt.Sprintf("%s is read from the tree's prefix, and the run is in no tree", addr.At)
	}
	at = machine.Resolve(b.st.Tree, addr.At)
	if err := store.CheckLocation(at); err != nil {
		return at, err.Error()
	}
	return at, ""
}

// aim asks for a send, its layers passed, to the run it goes to: the run
// with the key or, with none at a keyed location, the run this send starts,
// worked out once for each key in a step. why says it is not delivered,
// which is noted in the record. A send job's ask names its message before,
// since its result follows from it, delivered or not.
func (b *build) aim(ask *instance.Ask, s *machine.Send, at, why string) {
	addr := s.At
	ask.Location, ask.Client = at, addr.Client
	if at != "" {
		ask.Key = keyOf(addr)
	}
	undelivered := func(format string, args ...any) {
		ask.Undelivered, ask.Text = true, fmt.Sprintf(format, args...)
		b.ask(*ask)
	}
	switch {
	case why != "":
		undelivered("%s", why)
		return
	case machine.Reserved(s.Event):
		undelivered("no event can be sent under the name %q, which Corbie gives a machine itself", s.Event)
		return
	}
	name := func() {
		if ask.Message == "" {
			ask.Message = b.message("send")
		}
	}
	if addr.Run != "" {
		name()
		ask.Direct, ask.To = true, addr.Run
		b.ask(*ask)
		return
	}
	keyed := addr.Key != nil
	if keyed {
		if err := machine.CheckKey(*addr.Key); err != nil {
			undelivered("%v", err)
			return
		}
	}
	if b.targets == nil {
		b.targets = map[addressed]*target{}
	}
	t := b.targets[addressed{at, ask.Key}]
	if t == nil {
		var err error
		if t, err = b.p.aimAt(at, ask.Key, false); err != nil {
			b.err = err
			return
		}
		b.targets[addressed{at, ask.Key}] = t
	}
	to, starts, why := t.aim(at, keyed)
	if why != "" {
		undelivered("%s", why)
		return
	}
	name()
	if starts {
		// Named under the tree's client, from the sender's message, so a
		// carry-out done again names the same run. A second send in the
		// step to the key goes to it; one to a transient location starts
		// another.
		to = store.RootInstance(t.file.Client, b.time, ask.Message)
		ask.Run = to
		if keyed {
			t.holder = to
		}
		for place, other := range b.targets {
			if place.at == at && other != nil {
				other.runs++
			}
		}
	}
	ask.To = to
	b.ask(*ask)
}

// sent takes an event sent to the run by its address.
func (b *build) sent(a *arrival) {
	// A message that starts this run has been taken: it is here again.
	if a.starts(b.inst) || !b.hear(a) {
		b.drop(a)
		return
	}
	if b.def == nil {
		def, err := b.p.definition(b.st)
		if err != nil && !errors.Is(err, machine.ErrDefinition) {
			b.err = err
			return
		}
		if err != nil {
			// The step fails the run for its definition, as any step does.
			b.give(machine.Event{Name: a.msg.Event.Name, Data: a.msg.Event.Data, Sender: a.msg.Sender}, a)
			return
		}
		b.def = def
	}
	if why := b.refuses(a.msg, b.def); why != "" {
		b.undeliver(a, why)
		return
	}
	b.give(machine.Event{Name: a.msg.Event.Name, Data: a.msg.Event.Data, Sender: a.msg.Sender}, a)
}

// refuses says why the run, of the definition given, does not take an event
// sent to it by its address: it is at another location, its type takes no
// sent event, the event has a name of Corbie's or of its own, or its
// receive: does not list the sender. It gives "" where it takes the event.
func (b *build) refuses(msg *instance.Message, def *machine.Definition) string {
	name := msg.Event.Name
	switch {
	case msg.To.At != b.st.Location && !(msg.To.At == "" && msg.Sender != nil && msg.Sender.Client == store.InstanceClient(b.inst)):
		// The tree's client names a run of its own by its ID alone.
		return fmt.Sprintf("the run %s is not at %s", b.inst, msg.To.At)
	case !machine.Receives(def) && !(msg.Start != nil && msg.Start.Run == b.inst):
		// A send that starts a run of a flow or an agent is its start.
		return fmt.Sprintf("the run %s is a %s, which takes no sent event", b.inst, def.Type)
	case machine.Reserved(name) || machine.Owns(def, name):
		return fmt.Sprintf("%q is the name of an event the run gives itself", name)
	case !b.receives(def, msg.Sender, name):
		return fmt.Sprintf("%s does not receive %s from %s", b.st.Location, name, addressText(msg.Sender))
	}
	return ""
}

// receives says whether a definition's receive: lists a sender for an
// event. The tree's own client is always listed.
func (b *build) receives(def *machine.Definition, sender *machine.Address, event string) bool {
	if sender != nil && sender.Client != "" && sender.Client == store.InstanceClient(b.inst) {
		return true
	}
	return machine.ReceivesFrom(def, b.st.Tree, sender, event)
}

// undeliver leaves out an event sent to the run by its address, which it
// does not take: the sender is told why.
func (b *build) undeliver(a *arrival, why string) {
	b.out.dropped = append(b.out.dropped, dropped{a: a, note: why})
}

// move leaves an event sent to { at } or { at, key } out of a run that
// cannot take it: it goes on to the run that has the key.
func (b *build) move(a *arrival) { b.out.dropped = append(b.out.dropped, dropped{a: a, move: true}) }

// notDelivered is the note that tells the sender of an event why it was not
// delivered.
func notDelivered(msg *instance.Message, why string) string {
	return fmt.Sprintf("the event %s sent to %s was not delivered: %s", msg.Event.Name, addressText(msg.To), why)
}

// told asks for what tells the sender of an event, which a step left out,
// why it was not delivered.
func (b *build) told(a *arrival, why string) {
	if client, message, ok := a.client(); ok {
		b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: client, Message: message, Receipt: &instance.Receipt{Refused: why}})
		return
	}
	b.ask(instance.Ask{Kind: instance.AskTell, To: a.msg.From, Message: noteName(a.name), Text: notDelivered(a.msg, why)})
}

// noteName is the name of the note that a message was not delivered.
func noteName(message string) string {
	return message[strings.LastIndexByte(message, '/')+1:] + ".note"
}

// claim takes a key's file, named at, for a run. It gives the run that has
// the key where that is another. A file whose run has ended, or never
// began, is replaced. The file is sealed to the controller alone.
func (p *pass) claim(inst, at, key, start string) (string, error) {
	for {
		file, mark, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, at, key)
		if err != nil && !isAltered(err) {
			return "", err
		}
		how := store.OnlyIfNew
		if mark != store.None {
			how = store.OnlyIfUnchanged(mark)
		}
		if file != nil {
			if file.Run == inst {
				return "", nil
			}
			live, err := p.live(file)
			if err != nil {
				return "", err
			}
			if live {
				return file.Run, nil
			}
		}
		data, err := p.c.ID.Write(at, p.now, noEnd, instance.KeyFile{Key: key, Run: inst, Start: start}, p.c.ID.Card)
		if err != nil {
			return "", err
		}
		if written, err := p.store.Write(p.ctx, at, data, how); err != nil || written {
			return "", err
		}
		// The file changed under the write: read it again.
	}
}

// live says whether the run a file names has the key: it is running, or
// the message that starts it is still waiting.
func (p *pass) live(file *instance.KeyFile) (bool, error) {
	step, err := instance.Current(p.ctx, p.store, p.c.ID.Card, p.c.ID, file.Run)
	if err != nil && !isAltered(err) {
		return false, err
	}
	if step != nil {
		return step.State.Status == instance.Running, nil
	}
	waiting, err := p.store.ReadPart(p.ctx, file.Start, 0, 1)
	return waiting != nil, err
}

// free deletes a key's file, named at, where it still names the run.
func (p *pass) free(inst, at, key string) error {
	file, mark, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, at, key)
	if err != nil && !isAltered(err) || mark == store.None || file != nil && file.Run != inst {
		return err
	}
	_, err = p.store.Delete(p.ctx, at, store.OnlyIfUnchanged(mark))
	return err
}

// toAddress carries out a send action to a run by its address. An event
// that cannot be written is noted in the sender's record.
func (p *pass) toAddress(inst string, step *instance.Step, ask *instance.Ask) error {
	msg, failure, err := p.addressed(inst, step, ask)
	if err != nil || failure == nil {
		return err
	}
	return p.tell(inst, inst, ask.Message+".note", notDelivered(msg, failure.Error), asked(inst, step))
}

// sendJob carries out a send job: its event, and then its result, done with
// no output. An event that cannot be written fails the job.
func (p *pass) sendJob(inst string, step *instance.Step, ask *instance.Ask) error {
	_, failure, err := p.addressed(inst, step, ask)
	if err != nil {
		return err
	}
	if failure != nil {
		failure.Error = fmt.Sprintf("the event %s could not be sent: %s", ask.Event.Name, failure.Error)
		return p.failJob(inst, ask.Name, ask.Message+".failed", failure)
	}
	msg := &instance.Message{Kind: instance.KindResult, Instance: inst, From: inst,
		Result: &instance.Result{Job: ask.Name, Status: instance.Done, Final: true}}
	_, err = p.send(store.Message(p.ctl, p.ctl, ask.Message+".done"), noEnd, msg, p.c.ID.Card)
	return err
}

// addressed writes an event sent to a run by its address. What the event
// names is in the record of the run it goes to before that run is told. An
// event sent to a key carries the start of a run, so one that arrives after
// the run with the key has ended starts a fresh one, and so does one that
// starts a run of a transient location. It gives the message, and why it
// could not be written, if it could not.
func (p *pass) addressed(inst string, step *instance.Step, ask *instance.Ask) (*instance.Message, *machine.Error, error) {
	if ask.To == "" || ask.Undelivered {
		return nil, nil, nil // written in the record, and nowhere else
	}
	st := &step.State
	data, err := p.handUp(ask.Event.Data, ask.To)
	if err != nil {
		return nil, nil, err
	}
	sender := runOf(inst, st).Address()
	to := &machine.Address{At: ask.Location}
	msg := &instance.Message{Kind: instance.KindSend, Instance: ask.To, From: inst, To: to, Sender: sender, Cause: asked(inst, step),
		Event: &machine.Event{Name: ask.Event.Name, Data: data}}
	switch {
	case ask.Direct:
		to.Run = ask.To
	case ask.Key != "" || ask.Run != "":
		key := ask.Key
		if key != "" {
			to.Key = &key
		}
		mac := hmac.New(sha256.New, st.Seed)
		mac.Write([]byte("send\x00" + ask.Message))
		msg.Start = &instance.Start{Location: ask.Location, Key: key, Run: ask.Run, Seed: mac.Sum(nil),
			Above: &instance.Above{Depth: st.Above.Depth + 1}}
	}
	failure, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
	return msg, failure, err
}

// tell writes a note into a run's record, as a step of the controller's own,
// whether or not the run has ended. Its cause is the step the note is about.
func (p *pass) tell(to, from, message, text string, cause *instance.Cause) error {
	msg := &instance.Message{Kind: instance.KindNote, Instance: to, From: from, Note: text, Cause: cause}
	_, err := p.send(store.Message(p.ctl, p.ctl, message), noEnd, msg, p.c.ID.Card)
	return err
}

// undeliver deletes an event sent to an address that no step took, and then
// tells its sender why. Another copy may have moved it on since: then it
// stays, and it has not been lost.
//
// A client is answered first, and then the message is deleted: stopped
// between the two the other way round, the controller would leave it with no
// answer and nothing waiting that would give one.
func (p *pass) undeliver(a *arrival, why string) error {
	if client, message, ok := a.client(); ok {
		if err := p.writeReceipt(client, message, &instance.Receipt{Refused: why}); err != nil {
			return err
		}
		_, err := p.store.Delete(p.ctx, a.name, store.OnlyIfUnchanged(a.mark))
		return err
	}
	gone, err := p.store.Delete(p.ctx, a.name, store.OnlyIfUnchanged(a.mark))
	if err != nil || !gone {
		return err
	}
	return p.tell(a.msg.From, a.msg.Instance, noteName(a.name), notDelivered(a.msg, why), a.msg.Cause)
}

// redirect moves on an event sent to { at } or { at, key } whose run cannot
// take it: the run has ended, never began, or lost the key to another. It
// goes to the run that has the key now or, with none at a keyed location,
// to the run it starts. An event with neither has nowhere to go, and its
// sender is told. The message stays under its name, and sorts as it did,
// so it is rewritten in one write and nothing can come of a stop before or
// after it.
func (p *pass) redirect(a *arrival) error {
	msg := *a.msg
	keyed := msg.To.Key != nil
	t, err := p.aimAt(msg.To.At, keyOf(msg.To), true)
	if err != nil {
		return err
	}
	run := t.holder
	switch {
	case run != "" && run == msg.Instance:
		p.later() // it has the key after all: the next pass gives it the event
		return nil
	case run == "" && keyed && msg.Start != nil && msg.Start.Run == msg.Instance:
		p.later() // the run that won has ended: the next pass starts this one
		return nil
	case run == "":
		_, _, why := t.aim(msg.To.At, keyed)
		if why != "" || msg.Start == nil {
			if why == "" {
				why = "no running run has the key"
			}
			return p.undeliver(a, why)
		}
		start := *msg.Start
		if start.Run == "" {
			start.Run = store.RootInstance(t.file.Client, p.now, a.name[strings.LastIndexByte(a.name, '/')+1:])
		}
		msg.Start, run = &start, start.Run
	}
	data, err := p.handUp(msg.Event.Data, run)
	if err != nil {
		return err
	}
	before := msg.Instance
	msg.Instance, msg.Event = run, &machine.Event{Name: msg.Event.Name, Data: data}
	signed, err := p.c.ID.Write(a.name, p.now, noEnd, &msg, p.c.ID.Card)
	if err != nil {
		return err
	}
	p.later()
	written, err := p.store.Write(p.ctx, a.name, signed, store.OnlyIfUnchanged(a.mark))
	if err != nil || !written {
		return err
	}
	// The sender's record names the run the event was aimed at: it is told
	// the run the event went to, in a note named for that run, so an event
	// moved twice is noted twice. A client's receipt names that run itself.
	if _, _, ok := a.client(); ok || run == before {
		return nil
	}
	base, tail := a.name[strings.LastIndexByte(a.name, '/')+1:], run[strings.LastIndexByte(run, '/')+1:]
	return p.tell(msg.From, run, base+".moved."+tail, fmt.Sprintf("the event %s sent to %s went to the run %s, not %s", msg.Event.Name, addressText(msg.To), run, before), msg.Cause)
}

// route takes a client's send to an address. The controller checks it as
// it checks a run's, names the run it goes to, and writes it to itself as a
// message named from the client's, which the receiving run answers.
// Refused, it is answered here. A route done again writes the same message.
func (p *pass) route(a *arrival) error {
	base := a.name[strings.LastIndexByte(a.name, '/')+1:]
	answered, err := p.answered(a)
	if err != nil {
		return err
	}
	name := clientPrefix + a.sender + "." + base
	if !answered {
		// Routed before the controller stopped: the run takes what was
		// written, whatever has changed since.
		_, mark, err := p.store.Read(p.ctx, store.Message(p.ctl, p.ctl, name))
		if err != nil {
			return err
		}
		answered = mark != store.None
	}
	if answered {
		_, err := p.store.Delete(p.ctx, a.name, store.Overwrite)
		return err
	}
	refuse := func(format string, args ...any) error {
		if err := p.writeReceipt(a.sender, a.name, &instance.Receipt{Refused: fmt.Sprintf(format, args...)}); err != nil {
			return err
		}
		_, err := p.store.Delete(p.ctx, a.name, store.Overwrite)
		return err
	}
	msg, to := a.msg, a.msg.To
	// A client names a run of its own by its ID alone.
	own := to.At == "" && to.Run != "" && store.InstanceClient(to.Run) == a.sender && store.CheckInstance(to.Run) == nil
	switch err := to.Check(); {
	case !a.good(p.now):
		return refuse("the message came too late: it was good until %s", a.until.Format(time.RFC3339))
	case own:
	case err != nil:
		return refuse("%v", err)
	case to.Client != "":
		return refuse("a client has no inbox")
	case !strings.HasPrefix(to.At, "/"):
		return refuse("%s: a client names a location in full, from /", to.At)
	case store.CheckLocation(to.At) != nil:
		return refuse("%v", store.CheckLocation(to.At))
	}
	if machine.Reserved(msg.Event.Name) {
		return refuse("no event can be sent under the name %q, which Corbie gives a machine itself", msg.Event.Name)
	}
	owner := to.Run != "" && store.InstanceClient(to.Run) == a.sender
	if !owner && to.Run == "" {
		file, err := p.readLocated(to.At)
		if err != nil && !isAltered(err) {
			return err
		}
		owner = file != nil && file.Client == a.sender
	}
	if !owner {
		client, err := p.trusted(a.sender)
		if err != nil || client == nil {
			return err
		}
		if r := (&permission.Set{Layers: []permission.Layer{ceilingOf(client)}}).Check(permission.Send, to.At); r != nil {
			return refuse("%s", r.Error())
		}
	}
	out := &instance.Message{Kind: instance.KindSend, To: to, Sender: &machine.Address{Client: a.sender},
		Cause: &instance.Cause{Client: a.sender, Message: base}, Event: msg.Event}
	if to.Run != "" {
		out.Instance = to.Run
	} else {
		if to.Key != nil {
			if err := machine.CheckKey(*to.Key); err != nil {
				return refuse("%v", err)
			}
		}
		t, err := p.aimAt(to.At, keyOf(to), false)
		if err != nil {
			return err
		}
		run, starts, why := t.aim(to.At, to.Key != nil)
		if why != "" {
			return refuse("%s", why)
		}
		if starts {
			run = store.RootInstance(t.file.Client, a.sent, name)
		}
		out.Instance = run
		if to.Key != nil || starts {
			seed := make([]byte, 32)
			rand.Read(seed)
			out.Start = &instance.Start{Location: to.At, Key: keyOf(to), Seed: seed, Above: &instance.Above{}}
			if starts {
				out.Start.Run = run
			}
		}
	}
	failure, err := p.send(store.Message(p.ctl, p.ctl, name), noEnd, out, p.c.ID.Card)
	if err != nil {
		return err
	}
	if failure != nil {
		return refuse("%s", failure.Error)
	}
	p.later()
	_, err = p.store.Delete(p.ctx, a.name, store.Overwrite)
	return err
}

// later says there is more for the next pass to do at once.
func (p *pass) later() {
	p.mu.Lock()
	p.more = true
	p.mu.Unlock()
}
