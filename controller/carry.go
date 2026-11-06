package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

// finish finishes a step that has been written: it carries out what the
// entry asks for, brings the head up to date, and last deletes the
// messages and wakes the entry names. Every write is only-if-new under a
// name the entry gives, so doing any of it again is harmless.
func (p *pass) finish(inst string, step *instance.Step) error {
	// The step that ends a run frees its key before anything else: a
	// singleton's is the empty key.
	if st := &step.State; st.Status != instance.Running && (st.Lifetime == machine.LifetimeKeyed || st.Lifetime == machine.LifetimeSingleton) {
		if err := p.free(inst, store.Key(p.ctl, st.Location, st.Key), st.Key); err != nil {
			return err
		}
	}
	// What does not depend on anything else is written side by side. A step's
	// events go after it, one after another in the order sent, since a run
	// takes what sorts before an event it has heard as a copy. The ending is
	// written last, after the step's events.
	var last, rest, events []instance.Ask
	for _, ask := range step.Asks {
		switch ask.Kind {
		case instance.AskEnd:
			last = append(last, ask)
		case instance.AskEvent:
			events = append(events, ask)
		default:
			rest = append(rest, ask)
		}
	}
	apart := len(rest)
	rest = append(rest, events...)
	// A step that asks for more than a pass can carry out is taken further
	// by each pass: how far it has got is noted in the record a batch at a
	// time, and a later pass begins there.
	done := 0
	if len(rest) > carryBatch {
		var err error
		if done, err = p.carried(inst, step.Number); err != nil {
			return err
		}
	}
	for done < len(rest) {
		from, to := done, min(done+carryBatch, len(rest))
		if from < apart {
			to = min(to, apart)
			if err := p.each(to-from, func(i int) error { return p.carry(inst, step, &rest[from+i]) }); err != nil {
				return err
			}
		}
		for i := max(from, apart); i < to; i++ {
			if err := p.carry(inst, step, &rest[i]); err != nil {
				return err
			}
		}
		if done = to; done < len(rest) && len(rest) > carryBatch {
			if err := p.noteCarried(inst, step.Number, done); err != nil {
				return err
			}
		}
	}
	for i := range last {
		if err := p.carry(inst, step, &last[i]); err != nil {
			return err
		}
	}
	if err := p.head(inst, step); err != nil {
		return err
	}
	// Of what the entry used, the wakes that have not come due go first:
	// they are wakes it no longer wants, and would lead no pass back here.
	// What did lead here goes only when they are gone, so a controller
	// stopped between the two is led here again.
	var unwanted, led []string
	for _, name := range step.Used {
		if at, _, _, ok := store.ParseWake(p.ctl, name); ok && at.After(p.now) {
			unwanted = append(unwanted, name)
		} else {
			led = append(led, name)
		}
	}
	for _, names := range [][]string{unwanted, led} {
		err := p.each(len(names), func(i int) error {
			_, err := p.store.Delete(p.ctx, names[i], store.Overwrite)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// carryBatch is how many of a step's asks are carried out between two
// notes of how far carry-out has got.
const carryBatch = 64

// A carriedNote says how many of a step's asks have been carried out.
type carriedNote struct {
	Step int `json:"step"`
	Asks int `json:"asks"`
}

// carried gives how many of a step's asks an earlier pass carried out.
func (p *pass) carried(inst string, step int) (int, error) {
	at := store.Carried(p.ctl, inst)
	data, mark, err := p.store.Read(p.ctx, at)
	if err != nil || mark == store.None {
		return 0, err
	}
	var note carriedNote
	if _, err := identity.Read(data, at, p.c.ID.Card, nil, &note); err != nil || note.Step != step {
		return 0, nil // not this step's, or not the controller's: begin at the start
	}
	return max(note.Asks, 0), nil
}

// noteCarried notes how far a step's carry-out has got: only-if-unchanged
// and only forward, so a copy that is behind never puts the note back.
func (p *pass) noteCarried(inst string, step, asks int) error {
	at := store.Carried(p.ctl, inst)
	data, err := p.c.ID.Write(at, p.now, time.Time{}, carriedNote{Step: step, Asks: asks})
	if err != nil {
		return err
	}
	for {
		old, mark, err := p.store.Read(p.ctx, at)
		if err != nil {
			return err
		}
		how := store.OnlyIfNew
		if mark != store.None {
			var note carriedNote
			_, err := identity.Read(old, at, p.c.ID.Card, nil, &note)
			if err == nil && (note.Step > step || note.Step == step && note.Asks >= asks) {
				return nil
			}
			how = store.OnlyIfUnchanged(mark)
		}
		if done, err := p.store.Write(p.ctx, at, data, how); err != nil || done {
			return err
		}
		// The note changed under the write: read it again.
	}
}

// carry carries out one thing a step asked for.
func (p *pass) carry(inst string, step *instance.Step, ask *instance.Ask) error {
	st := &step.State
	switch ask.Kind {
	case instance.AskWake:
		_, err := p.store.Write(p.ctx, store.Wake(p.ctl, ask.At, inst, ask.What), []byte("\n"), store.OnlyIfNew)
		p.due(ask.At)
		return err

	case instance.AskScript:
		msg, to, failure, err := p.job(inst, step, ask)
		if err != nil {
			return err
		}
		if failure == nil {
			failure, err = p.send(store.Message(ask.Runner, p.ctl, ask.Message), ask.At, msg, to)
			if err != nil {
				return err
			}
			if failure != nil {
				failure.Error = "the job could not be sent: " + failure.Error
			}
		}
		if failure != nil {
			return p.failJob(inst, ask.Name, ask.Message+".failed", failure)
		}
		// The wake for the job's due time is written after the job.
		_, err = p.store.Write(p.ctx, store.Wake(p.ctl, ask.At, inst, ask.What), []byte("\n"), store.OnlyIfNew)
		p.due(ask.At)
		return err

	case instance.AskFail:
		return p.failJob(inst, ask.Name, ask.Message, ask.Error)

	case instance.AskChild:
		msg := &instance.Message{Kind: instance.KindStart, Instance: ask.Child, From: inst, Start: p.child(inst, step, ask)}
		failure, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
		if err != nil {
			return err
		}
		if failure != nil {
			failure.Error = "the machine could not be started: " + failure.Error
			return p.failJob(inst, ask.Name, ask.Message+".failed", failure)
		}
		return nil

	case instance.AskCancel:
		if ask.Runner != "" {
			to, err := p.trusted(ask.Runner)
			if err != nil || !to.Trusted() {
				return err
			}
			msg := &instance.Message{Kind: instance.KindCancel, Instance: inst, Cancel: strings.TrimSuffix(ask.Message, cancelSuffix)}
			_, err = p.send(store.Message(ask.Runner, p.ctl, ask.Message), ask.At, msg, to.Card)
			return err
		}
		to := ask.Child
		if ask.Run != "" {
			to = ask.Run // a run a call started
		}
		msg := &instance.Message{Kind: instance.KindCancel, Instance: to, From: inst}
		_, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
		return err

	case instance.AskCall:
		return p.called(inst, step, ask)

	case instance.AskResult:
		return p.result(ask)

	case instance.AskTell:
		return p.tell(ask.To, inst, ask.Message, ask.Text)

	case instance.AskEvent:
		if ask.Job != nil {
			return p.sendJob(inst, step, ask)
		}
		if ask.Location != "" {
			return p.toAddress(inst, step, ask)
		}
		if ask.To == "" || ask.Undelivered {
			return nil // written in the record, and nowhere else
		}
		// An event goes down to a child, or up from one: either way it
		// belongs to the job the child is.
		parent, child, job := inst, ask.To, ""
		if going := st.Jobs[ask.Name]; going != nil {
			job = going.Full()
		}
		msg := &instance.Message{Kind: instance.KindEvent, Instance: ask.To, From: inst, Event: ask.Event}
		if ask.Name == "" {
			// Up to the parent: what it carries is in the parent's record
			// before the parent is told.
			data, err := p.handUp(ask.Event.Data, ask.To)
			if err != nil {
				return err
			}
			parent, child, job = ask.To, inst, st.Above.Job
			msg = &instance.Message{Kind: instance.KindEmit, Instance: ask.To, From: inst,
				Emit: &instance.EmitNote{Job: st.Above.Job, Name: ask.Event.Name, Index: ask.Attempt, Data: data}}
		}
		failure, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
		if err != nil || failure == nil {
			return err
		}
		// An event that cannot be delivered fails the job it belongs to: the
		// parent is given the failure in the event's place, and the child is
		// cancelled as its parent would cancel it.
		failure.Error = fmt.Sprintf("the event %s could not be sent: %s", ask.Event.Name, failure.Error)
		msg = &instance.Message{Kind: instance.KindResult, Instance: parent, From: child,
			Result: &instance.Result{Job: job, Status: failure.Status, Error: failure, Final: true}}
		if _, err := p.send(store.Message(p.ctl, p.ctl, ask.Message+".failed"), noEnd, msg, p.c.ID.Card); err != nil {
			return err
		}
		msg = &instance.Message{Kind: instance.KindCancel, Instance: child, From: parent}
		_, err = p.send(store.Message(p.ctl, p.ctl, ask.Message+cancelSuffix), noEnd, msg, p.c.ID.Card)
		return err

	case instance.AskEnd:
		result := &instance.Result{Job: ask.Name, Status: instance.Done}
		if st.Above.Parent != "" {
			// What a child used is its parent's; a called run's is its own
			// tree's.
			result.Used = st.Used
		}
		output, err := p.handUp(ask.Output, ask.To)
		if err != nil {
			return err
		}
		result.Output = output
		if ask.Error != nil {
			e := *ask.Error
			e.Output = nil
			result.Status, result.Error = e.Status, &e
		}
		msg := &instance.Message{Kind: instance.KindResult, Instance: ask.To, From: inst, Result: result}
		failure, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
		if err != nil || failure == nil {
			return err
		}
		// A result that cannot be delivered fails the job it belongs to.
		failure.Error = "the result could not be passed to its parent: " + failure.Error
		msg.Result = &instance.Result{Job: ask.Name, Status: machine.StatusError, Error: failure, Final: true}
		_, err = p.send(store.Message(p.ctl, p.ctl, ask.Message+".failed"), noEnd, msg, p.c.ID.Card)
		return err

	case instance.AskReceipt:
		if ask.Run != "" {
			// A taken call's receipt is kept until its result has been
			// written a day, and so has no end of its own.
			return p.answer(ask.Sender, ask.Message, noEnd, ask.Receipt)
		}
		return p.writeReceipt(ask.Sender, ask.Message, ask.Receipt)
	}
	return nil
}

// handUp gives a value as its parent is to have it: every blob it refers
// to copied into the parent's record. A reference that names no blob is
// handed up as it is, and opens nothing there as it opened nothing here.
func (p *pass) handUp(v any, to string) (any, error) {
	return blob.Walk(v, func(_ string, ref blob.Ref) (any, error) {
		copied, err := blob.CopyWhole(p.ctx, p.store, ref, p.ctl, store.Blobs(p.ctl, to))
		if err != nil {
			return ref.Value(), p.blobError(err)
		}
		return copied.Value(), nil
	})
}

// blobError tells a blob that cannot be used from a store that cannot be
// reached. Only the second ends the pass.
func (p *pass) blobError(err error) error {
	if blob.Unusable(err) {
		return nil
	}
	return err
}

// due notes a wake's time, for the pass to say when the next thing is due.
func (p *pass) due(at time.Time) {
	p.mu.Lock()
	if at.After(p.now) && (p.next.IsZero() || at.Before(p.next)) {
		p.next = at
	}
	p.mu.Unlock()
}

// send writes a message into an inbox only-if-new, sealed to its receiver.
// One too large to be a message is not written, and the reason is given. So
// is a name in the controller's own inbox that holds some other message: a
// carry-out done again finds this same message there, or finds it gone. A
// runner's inbox cannot be read, and only one job has a job's name.
func (p *pass) send(name string, until time.Time, msg *instance.Message, to identity.Card) (*machine.Error, error) {
	data, err := p.c.ID.Write(name, p.now, until, msg, to)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxMessage {
		return &machine.Error{Status: machine.StatusError,
			Error: fmt.Sprintf("it is too large: it is %d bytes and a message may be %d; pass large values as blobs", len(data), MaxMessage)}, nil
	}
	written, err := p.store.Write(p.ctx, name, data, store.OnlyIfNew)
	if err != nil || written || to.Name != p.ctl {
		return nil, err
	}
	same, err := p.same(name, msg)
	if err != nil || same {
		return nil, err
	}
	return &machine.Error{Status: machine.StatusError, Error: "its name is taken by another message: " + name}, nil
}

// same says whether what is under a name in the controller's own inbox is a
// message as given.
func (p *pass) same(name string, msg *instance.Message) (bool, error) {
	data, mark, err := p.store.Read(p.ctx, name)
	if err != nil || mark == store.None {
		return err == nil, err
	}
	var there instance.Message
	if _, err := identity.Read(data, name, p.c.ID.Card, p.c.ID, &there); err != nil {
		return false, nil
	}
	// An event sent to a key may have been moved on to another run.
	return there.Kind == msg.Kind && there.From == msg.From && (there.Instance == msg.Instance || msg.Kind == instance.KindSend), nil
}

// failJob fails a job before anything was sent for it. The failure reaches
// the machine as a result does: as a message in the controller's own
// inbox, under a name the entry gives.
func (p *pass) failJob(inst, job, message string, failure *machine.Error) error {
	msg := &instance.Message{Kind: instance.KindResult, Instance: inst, From: inst,
		Result: &instance.Result{Job: job, Status: failure.Status, Error: failure, Final: true}}
	_, err := p.send(store.Message(p.ctl, p.ctl, message), noEnd, msg, p.c.ID.Card)
	return err
}

// ownLayer is the layer a job's own permissions make, by name.
func ownLayer(job *machine.Job) permission.Layer {
	name := fmt.Sprintf("the job %q in the definition of %q", job.Name, job.Found.In)
	return permission.Layer{Name: name, Allow: job.Allow, Deny: job.Deny,
		Fix: "add the line %s to the allow list of " + name}
}

// job builds the job a runner is sent: everything it needs to run the
// script. A job that cannot be built fails, with a reason.
func (p *pass) job(inst string, step *instance.Step, ask *instance.Ask) (*instance.Message, identity.Card, *machine.Error, error) {
	st, asked := &step.State, ask.Job
	none := identity.Card{}
	runner, err := p.trusted(ask.Runner)
	if err != nil {
		return nil, none, nil, err
	}
	if !runner.Trusted() {
		return nil, none, p.failf("the runner %q is no longer trusted", ask.Runner), nil
	}
	client, err := p.trusted(store.InstanceClient(inst))
	if err != nil {
		return nil, none, nil, err
	}
	if client == nil {
		return nil, none, p.failf("the client %q is not known to the controller", store.InstanceClient(inst)), nil
	}
	request, _, _ := strings.Cut(ask.Name, "@")
	job := &instance.Job{Instance: inst, Name: ask.Name, Attempt: ask.Attempt, Request: inst + "/" + request,
		Script: asked.Found.Blob, Path: asked.Script, Files: asked.Found.Files,
		Input: asked.Input, Timeout: timeoutOf(asked), Memory: asked.Memory, Due: ask.At, Wants: asked.Wants, Client: client.Card}

	// Every layer the controller knows of, each by name: the job in its
	// definition, the start, each parent's job, and the client's ceiling.
	job.Layers = append([]permission.Layer{ownLayer(asked)}, layersAbove(st, asked)...)
	job.Layers = append(job.Layers, ceilingOf(client))
	location := st.Location

	// The values of the secrets the job names, and no others: those set
	// nearest the run's location. One that a layer does not let travel at
	// all is not sent, and the job is refused as its script would be on
	// using it.
	for _, name := range secret.Named(asked.Allow) {
		if r := refusedSecret(job.Layers, asked.Allow, name); r != nil {
			return nil, none, &machine.Error{Status: machine.StatusDenied, Error: r.Error(), Refused: r.What, Layer: r.Layer, Fix: r.Fix}, nil
		}
		value, found, err := secret.Get(p.ctx, p.store, p.c.ID, location, name)
		if err != nil && !isAltered(err) {
			return nil, none, nil, err
		}
		if !found {
			return nil, none, p.failf("%s is not set at %s or above (the controller's owner sets it with: corbie controller secret set %s%s)", name, location, name, atFlag(location)), nil
		}
		if job.Secrets == nil {
			job.Secrets = map[string]string{}
		}
		job.Secrets[name] = value
	}
	return &instance.Message{Kind: instance.KindJob, Instance: inst, Job: job}, runner.Card, nil, nil
}

// atFlag is the flag that names a location on the command line: none for
// "/", which is the default.
func atFlag(location string) string {
	if location == "/" {
		return ""
	}
	return " --at " + location
}

// refusedSecret says which layers do not let a secret the job names travel
// at all: those with no allow entry for it, and those with a deny entry
// that stops it everywhere. They are asked about the job's own entry for
// it, which they refuse, so the refusal reads as any other does.
func refusedSecret(layers []permission.Layer, allow []string, name string) *permission.Refusal {
	var against []permission.Layer
	for _, layer := range layers {
		one := &permission.Set{Layers: []permission.Layer{layer}}
		if !permission.AllowsSecret(one.Layers, name) || one.Denied(permission.Secret, name+"@*") != nil {
			against = append(against, layer)
		}
	}
	if len(against) == 0 {
		return nil
	}
	for _, entry := range allow {
		if tried, ok := strings.CutPrefix(entry, permission.Secret+":"); ok && strings.HasPrefix(tried, name+"@") {
			return (&permission.Set{Layers: against}).Check(permission.Secret, tried)
		}
	}
	return nil
}

// layersAbove gives the layers above a job: the instance's own or, for an ask
// taken from above, those of the machine that declares it, so that a machine
// which inherits an ask cannot widen what the ask may reach.
func layersAbove(st *instance.State, job *machine.Job) []permission.Layer {
	if job.Above {
		return st.Above.Ask.Layers
	}
	return st.Above.Layers
}

func timeoutOf(job *machine.Job) machine.Duration {
	if job.Timeout > 0 {
		return job.Timeout
	}
	return machine.DefaultTimeout
}

// child builds the start of a child machine. What is handed down is read
// where it is. A job that carries a list narrows the child with it, and one
// that carries none adds no layer: a child the definition names is the
// definition's own words. A machine made while running is not, so it is
// always narrowed, and with no list it is allowed nothing; and everything
// below it was made while running too.
func (p *pass) child(inst string, step *instance.Step, ask *instance.Ask) *instance.Start {
	st, job := &step.State, ask.Job
	layers := slices.Clone(layersAbove(st, job))
	_, made := blob.AsRef(job.Stored)
	if made || len(job.Allow)+len(job.Deny) > 0 {
		layers = append(layers, ownLayer(job))
	}
	above := &instance.Above{Parent: inst, Job: ask.Name, Depth: st.Above.Depth + 1, MaxDepth: st.Above.MaxDepth,
		Layers:   layers,
		AskAbout: st.Above.AskAbout, Ask: st.Above.Ask, Context: st.Above.Context, Values: st.Above.Values,
		IsAsk: job.Name == instance.AskName,
		Made:  made || st.Above.Made && !job.Above, Keep: st.Keep}
	// The nearest ask above the child is the one its job goes to: an ask
	// nearer than its machine's, or the machine's own. A machine made while
	// running keeps none, so it hands down the one above it.
	switch {
	case job.Ask != nil:
		above.Ask = &instance.AskAbove{Spec: job.Ask, At: job.AskAt, Definition: st.Definition, Input: st.Input, Layers: st.Above.Layers}
	case st.Ask != nil:
		above.Ask = &instance.AskAbove{Spec: st.Ask, At: instance.AskName, Definition: st.Definition, Input: st.Input, Layers: st.Above.Layers}
	}
	if above.IsAsk {
		// A machine run as an ask takes no ask from above it, and is not
		// held to the lists of what must be asked about above it.
		above.Ask, above.AskAbout = nil, nil
	}
	mac := hmac.New(sha256.New, st.Seed)
	mac.Write([]byte("child\x00" + ask.Name))
	// A child runs at its parent's location.
	start := &instance.Start{Definition: job.Found.Blob, Input: job.Input, Labels: st.Labels, Timeout: job.Timeout,
		Seed: mac.Sum(nil), Above: above, Location: st.Location, Tree: st.Tree}
	return start
}

// receipt answers a client's message that led to no step.
func (p *pass) receipt(a *arrival, r *instance.Receipt) error {
	client, message, _ := a.client()
	return p.writeReceipt(client, message, r)
}

// writeReceipt writes the answer to a client's message under the message's
// own name, sealed to the client that sent it. A message sent twice gets
// the same answer twice: the first receipt stands.
func (p *pass) writeReceipt(sender, message string, r *instance.Receipt) error {
	return p.answer(sender, message, p.now.Add(instance.ReceiptLife), r)
}

// answer writes a receipt good until a time: none for a taken call's.
func (p *pass) answer(sender, message string, until time.Time, r *instance.Receipt) error {
	to, err := p.trusted(sender)
	if err != nil || to == nil {
		return err
	}
	name := store.Receipt(p.ctl, sender, message[strings.LastIndexByte(message, '/')+1:])
	_, err = p.write(name, until, r, to.Card)
	return err
}

// head brings an instance's head up to date with a step: only-if-unchanged
// and only forward. When the instance has ended, its head is written among
// the finished before it is removed from the running.
func (p *pass) head(inst string, step *instance.Step) error {
	st := &step.State
	head := &instance.Head{Name: inst, Machine: st.Machine, Type: st.Type, Status: st.Status, Step: step.Number,
		Started: st.Started, Ended: st.Ended, Parent: st.Above.Parent, Location: st.Location, Key: st.Key, StartedBy: st.Above.StartedBy,
		Timers: len(st.Timers), Labels: st.Labels, Keep: st.Keep}
	if st.Error != nil && st.Status == instance.Failed {
		head.Failure = st.Error.Status
	}
	for _, job := range st.Jobs {
		if job.Runner != "" {
			head.Jobs++
		}
	}
	for _, ticket := range st.Tickets {
		if !slices.Contains(head.Tickets, ticket.Name) {
			head.Tickets = append(head.Tickets, ticket.Name)
		}
	}
	slices.Sort(head.Tickets)

	running, finished := store.RunningHead(p.ctl, inst), store.FinishedHead(p.ctl, inst)
	if st.Status != instance.Running {
		// A note written in the record after the end moves the finished
		// head on with it.
		if _, err := p.forward(finished, head); err != nil {
			return err
		}
		_, err := p.store.Delete(p.ctx, running, store.Overwrite)
		return err
	}
	made, err := p.forward(running, head)
	if err != nil || !made {
		return err
	}
	// There was no running head: the instance is new, or it has ended and a
	// later step moved its head. Which of the two is asked only once the head
	// is written: an instance that has ended, however lately, is then sure to
	// be found among the finished, and the head just written is taken away
	// again.
	if _, ended, err := p.store.Read(p.ctx, finished); err != nil || ended == store.None {
		return err
	}
	_, err = p.store.Delete(p.ctx, running, store.Overwrite)
	return err
}

// forward writes a head only-if-unchanged and only forward: a head already
// at the step or past it stays. It says whether there was no head before.
func (p *pass) forward(at string, head *instance.Head) (made bool, err error) {
	signed, err := p.c.ID.Write(at, p.now, noEnd, head)
	if err != nil {
		return false, err
	}
	for {
		data, mark, err := p.store.Read(p.ctx, at)
		if err != nil {
			return false, err
		}
		how := store.OnlyIfNew
		if mark != store.None {
			var current instance.Head
			if _, err := identity.Read(data, at, p.c.ID.Card, nil, &current); err == nil && current.Step >= head.Step {
				return false, nil
			}
			how = store.OnlyIfUnchanged(mark)
		}
		if done, err := p.store.Write(p.ctx, at, signed, how); err != nil || done {
			return done && mark == store.None, err
		}
		// The head changed under the write: read it again.
	}
}
