package controller

import (
	"fmt"
	"strings"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// A key's file holds the run the key names. These are the rules every part
// of this file keeps:
//
//  1. A run takes its key's file, only-if-new, before its first step is
//     written, so a run that loses has done nothing.
//  2. The step that ends a run is written first and the file is deleted
//     after, so a run that has the key is running or has just ended.
//  3. An event sent to a key leaves the inbox only when a step of a running
//     run used it. Until then it stays under the name its sender gave it, and
//     where its run has ended, or lost its key, the message is rewritten in
//     place to name the run that has the key.
//
// So a send reads the file once and fixes the run it goes to in its step. A
// carry-out done again goes to the same run, which has heard it.

// isKeyed says an arrival is an event the controller carried for a run that
// sent it to a key. No other party can send one.
func (a *arrival) isKeyed() bool {
	return !a.wake && a.msg.Kind == instance.KindKeyed && a.kind == identity.Controller
}

// starts says an arrival starts an instance: a start, or an event sent to a
// key that names the run it starts.
func (a *arrival) starts(inst string) bool {
	if a.wake || a.msg.Start == nil {
		return false
	}
	return a.msg.Kind == instance.KindStart || a.isKeyed() && a.msg.Start.Run == inst
}

// sendingTo is one run's events to one key.
type sendingTo struct{ from, key string }

// behind says whether an event sent to a key must wait for the next pass: an
// earlier one from the same run to the same key is on its way to another run,
// and is taken first. It is why an event is never heard out of order. sending
// holds the run the earliest of each goes to, and the inbox is read in order.
func (p *pass) behind(a *arrival, sending map[sendingTo]string) bool {
	if !a.isKeyed() {
		return false
	}
	to := sendingTo{a.msg.From, a.msg.Key}
	run, seen := sending[to]
	if !seen {
		sending[to] = a.msg.Instance
		return false
	}
	if run == a.msg.Instance {
		return false
	}
	p.later()
	return true
}

// readKeys reads the file of each key a step sends to, side by side. A step
// that does not fit in a pass is never written, and one pass can read many.
func (b *build) readKeys(requests []machine.Request) error {
	if b.keys == nil {
		b.keys = map[string]string{}
	}
	var keys []string
	for _, r := range requests {
		if r.Send == nil || r.Send.Key == nil {
			continue
		}
		key := *r.Send.Key
		if _, seen := b.keys[key]; !seen && machine.CheckKey(key) == nil {
			b.keys[key] = ""
			keys = append(keys, key)
		}
	}
	runs := make([]string, len(keys))
	err := b.p.each(len(keys), func(i int) error {
		file, _, err := instance.ReadKey(b.p.ctx, b.p.store, b.p.c.ID.Card, b.p.c.ID, store.InstanceClient(b.inst), keys[i])
		if err != nil && !isAltered(err) {
			return err
		}
		if file != nil {
			runs[i] = file.Run
		}
		return nil
	})
	for i, key := range keys {
		b.keys[key] = runs[i]
	}
	return err
}

// usesKey says whether a request sends to a key or gives a job one.
func usesKey(r machine.Request) bool {
	return r.Send != nil && r.Send.Key != nil || r.Job != nil && r.Job.RunKey != nil
}

// sendToKey takes a machine's request to send an event to a key. It works out
// the run the event goes to, once for each key in a step: the run with the
// key or, with none and a machine to start, the run this send starts. What
// cannot be sent is noted in the record and fails nothing.
func (b *build) sendToKey(s *machine.Send) {
	ask := instance.Ask{Kind: instance.AskEvent, Event: &machine.Event{Name: s.Event, Data: s.Data}}
	failed := func(why string) {
		ask.Undelivered, ask.Text = true, why
		b.ask(ask)
	}
	key := *s.Key
	if err := machine.CheckKey(key); err != nil {
		failed(err.Error())
		return
	}
	ask.Key = key
	var found *machine.Found
	if s.Machine != "" {
		if b.st.Above.Depth+1 > b.st.Above.MaxDepth {
			failed(fmt.Sprintf("depth limit reached: a root may have %d levels of machines below it", b.st.Above.MaxDepth))
			return
		}
		found = &machine.Found{In: b.def.Name, Blob: b.st.Definition.Beside(b.def.Machines[s.Machine])}
	}
	run := b.keys[key]
	if run == "" && found == nil {
		ask.Undelivered = true
		b.ask(ask)
		return
	}
	ask.Message = b.message("key")
	if found != nil {
		ask.Run = store.ChildInstance(b.inst, b.time, ask.Message)
		ask.Job = &machine.Job{Machine: s.Machine, Found: found, RunKey: &key}
	}
	if run == "" {
		run = ask.Run
	}
	ask.To = run
	b.keys[key] = run
	b.ask(ask)
}

// keyed takes an event sent to a key.
func (b *build) keyed(a *arrival) {
	// A message that starts this run has been taken: it is here again.
	if a.starts(b.inst) || !b.hear(a) {
		b.drop(a)
		return
	}
	b.give(machine.Event{Name: a.msg.Event.Name, From: a.msg.From, Data: a.msg.Event.Data}, a)
}

// move leaves an event sent to a key out of a run that cannot take it: it
// goes on to the run that has the key.
func (b *build) move(a *arrival) { b.out.dropped = append(b.out.dropped, dropped{a: a, move: true}) }

// claim takes a key's file for a run. It gives the run that has the key
// where that is another. A file whose run has ended, or never began, is
// replaced.
func (p *pass) claim(inst, key, start string) (string, error) {
	client := store.InstanceClient(inst)
	at := store.Key(p.ctl, client, key)
	for {
		file, mark, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, client, key)
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
		readers, err := p.readers(inst)
		if err != nil {
			return "", err
		}
		data, err := p.c.ID.Write(at, p.now, noEnd, instance.KeyFile{Key: key, Run: inst, Start: start}, readers...)
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

// free deletes a key's file, where it still names the run.
func (p *pass) free(inst, key string) error {
	client := store.InstanceClient(inst)
	file, mark, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, client, key)
	if err != nil && !isAltered(err) || mark == store.None || file != nil && file.Run != inst {
		return err
	}
	_, err = p.store.Delete(p.ctx, store.Key(p.ctl, client, key), store.OnlyIfUnchanged(mark))
	return err
}

// toKey carries out a send to a key. What the event names is in the record
// of the run it goes to before that run is told.
func (p *pass) toKey(inst string, step *instance.Step, ask *instance.Ask) error {
	if ask.To == "" || ask.Undelivered {
		return nil // written in the record, and nowhere else
	}
	data, err := p.handUp(ask.Event.Data, ask.To)
	if err != nil {
		return err
	}
	msg := &instance.Message{Kind: instance.KindKeyed, Instance: ask.To, From: inst, Key: ask.Key, Event: &machine.Event{Name: ask.Event.Name, Data: data}}
	if ask.Run != "" {
		// Named by the run it starts, so its seed is its own.
		named := *ask
		named.Name = ask.Run
		msg.Start = p.child(inst, step, &named)
		msg.Start.Run = ask.Run
		above := msg.Start.Above
		above.Parent, above.Job, above.StartedBy = "", "", inst
		// A root is kept as its definition says, whatever keeps its sender.
		above.Keep = 0
		if ask.To == ask.Run {
			if failure, err := p.own(msg.Start, ask.Run); err != nil || failure != nil {
				if err != nil {
					return err
				}
				return p.tell(inst, inst, ask.Message+".note", fmt.Sprintf("the event %s sent to the key %q was not delivered: %s", ask.Event.Name, ask.Key, failure.Error))
			}
		}
	}
	failure, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
	if err != nil || failure == nil {
		return err
	}
	return p.tell(inst, inst, ask.Message+".note", fmt.Sprintf("the event %s sent to the key %q was not delivered: %s", ask.Event.Name, ask.Key, failure.Error))
}

// own gives a run a send starts its own copy of what its start names. It is
// a root, and outlives the record of the run that sent: its definition, the
// machines and scripts that names, and the ask handed down to it.
func (p *pass) own(start *instance.Start, run string) (*machine.Error, error) {
	def, err := blob.CopyWhole(p.ctx, p.store, start.Definition, p.ctl, run)
	if err != nil {
		if err = p.blobError(err); err != nil {
			return nil, err
		}
		return p.failf("its machine is no longer in the store"), nil
	}
	start.Definition = def
	if start.Above.Ask == nil {
		return nil, nil
	}
	ask := *start.Above.Ask
	if ask.Definition, err = blob.CopyWhole(p.ctx, p.store, ask.Definition, p.ctl, run); err != nil {
		if err = p.blobError(err); err != nil {
			return nil, err
		}
		return p.failf("the ask above it is no longer in the store"), nil
	}
	if ask.Input, err = p.handUp(ask.Input, run); err != nil {
		return nil, err
	}
	start.Above.Ask = &ask
	return nil, nil
}

// tell writes a note into a run's record, as a step of the controller's own.
// A run that has ended takes nothing more.
func (p *pass) tell(to, from, message, text string) error {
	msg := &instance.Message{Kind: instance.KindNote, Instance: to, From: from, Note: text}
	_, err := p.send(store.Message(p.ctl, p.ctl, message), noEnd, msg, p.c.ID.Card)
	return err
}

// redirect moves on an event sent to a key whose run cannot take it: the run
// has ended, never began, or lost the key to another. It goes to the run that
// has the key now or, with none, to the run it starts. An event with neither
// has nowhere to go, and is noted in the sender's record. The message stays
// under its name, and sorts as it did, so it is rewritten in one write and
// nothing can come of a stop before or after it.
func (p *pass) redirect(a *arrival) error {
	msg := *a.msg
	drop := func(why string) error {
		// Another copy may have moved it on since: then it stays, and it has
		// not been lost.
		gone, err := p.store.Delete(p.ctx, a.name, store.OnlyIfUnchanged(a.mark))
		if err != nil || !gone {
			return err
		}
		text := fmt.Sprintf("the event %s sent to the key %q was not delivered: %s", msg.Event.Name, msg.Key, why)
		return p.tell(msg.From, msg.Instance, a.name[strings.LastIndexByte(a.name, '/')+1:]+".note", text)
	}
	run, err := p.holder(store.InstanceClient(msg.From), msg.Key)
	if err != nil {
		return err
	}
	switch {
	case run == msg.Instance && run != "":
		p.later() // it has the key after all: the next pass gives it the event
		return nil
	case run == "" && msg.Start == nil:
		return drop("no running run has it")
	case run == "" && msg.Start.Run == msg.Instance:
		p.later() // the run that won has ended: the next pass starts this one
		return nil
	case run == "":
		run = msg.Start.Run
		start, above := *msg.Start, *msg.Start.Above
		start.Above, msg.Start = &above, &start
		failure, err := p.own(&start, run)
		if err != nil {
			return err
		}
		if failure != nil {
			return drop(failure.Error)
		}
	}
	data, err := p.handUp(msg.Event.Data, run)
	if err != nil {
		return err
	}
	msg.Instance, msg.Event = run, &machine.Event{Name: msg.Event.Name, Data: data}
	signed, err := p.c.ID.Write(a.name, p.now, noEnd, &msg, p.c.ID.Card)
	if err != nil {
		return err
	}
	p.later()
	_, err = p.store.Write(p.ctx, a.name, signed, store.OnlyIfUnchanged(a.mark))
	return err
}

// holder gives the run that has a key: none where its file names a run that
// has ended or never began.
func (p *pass) holder(client, key string) (string, error) {
	file, _, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, client, key)
	if err != nil && !isAltered(err) || file == nil {
		return "", err
	}
	if live, err := p.live(file); err != nil || !live {
		return "", err
	}
	return file.Run, nil
}

// later says there is more for the next pass to do at once.
func (p *pass) later() {
	p.mu.Lock()
	p.more = true
	p.mu.Unlock()
}
