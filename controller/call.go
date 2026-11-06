package controller

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A call starts a run of a transient location and waits for it. These are
// the rules every part of this file keeps:
//
//  1. The called run is a root of its location's tree client, named from
//     the message that starts it, so a carry-out done again names the same
//     run. It is no child: a step names it by Run, never by Child, and
//     nothing that removes what is below a run follows it.
//  2. A call job's run ends by sending the job its result, as a child does.
//     A run a client called writes its result beside the call's receipt.
//  3. A call that no run takes fails its job, or is refused in the client's
//     receipt; nothing of a run is written for it.

// addresses says a request sends to a run by its address or calls a
// location: what a machine made while running may not ask for.
func addresses(r machine.Request) bool {
	return sendsTo(r) || r.Job != nil && (r.Job.IsCall() || r.Job.IsSend())
}

// isCall says a start is of a run a call starts: a call job's, or a
// client's call into a tree it does not own.
func isCall(s *instance.Start) bool {
	return s != nil && s.Above != nil && (s.Above.CallJob != "" || s.Above.CallReceipt != nil)
}

// callJob says an arrival starts a run for a call job.
func (a *arrival) callJob() bool {
	return !a.wake && a.kind == identity.Controller && a.msg.Kind == instance.KindStart && isCall(a.msg.Start) && a.msg.Start.Above.CallJob != ""
}

// callTo works out the location a call job calls, and checks it as a send
// is checked: against the job's own lists, the lists the run was started
// under and its tree client's ceiling. It gives the failure of a call that
// cannot be made.
func (b *build) callTo(job *instance.Going) (string, *machine.Error, error) {
	at := job.Call.At
	if !strings.HasPrefix(at, "/") && b.st.Tree == "" {
		return "", b.p.failf("%s is read from the tree's prefix, and the run is in no tree", at), nil
	}
	at = machine.Resolve(b.st.Tree, at)
	if err := store.CheckLocation(at); err != nil {
		return "", b.p.failf("%v", err), nil
	}
	own := permission.Layer{Name: fmt.Sprintf("the job %q in the definition of %q", job.Name, b.def.Name), Allow: job.Allow, Deny: job.Deny}
	own.Fix = "add the line %s to the allow list of " + own.Name
	refused, err := b.refusedSend(at, own)
	return at, refused, err
}

// call starts the run a call job calls: a root of the location's tree
// client, named from the message that starts it. A location with nothing
// recorded, or one that is not transient, fails the job before anything is
// sent.
func (b *build) call(job *instance.Going) {
	at := job.Call.At
	file, err := b.p.readLocated(at)
	if err != nil && !isAltered(err) {
		b.err = err
		return
	}
	switch {
	case file == nil:
		b.fail(job, "nothing is recorded at %s: a call starts a run of a transient location", at)
		return
	case file.Lifetime != machine.LifetimeTransient:
		b.fail(job, "%s is %s: a call starts a run only of a transient location", at, file.Lifetime)
		return
	}
	message := b.message("call")
	job.Called = store.RootInstance(file.Client, b.time, message)
	b.ask(instance.Ask{Kind: instance.AskCall, Name: job.Full(), Job: &job.Job, Location: at, Run: job.Called, Message: message})
}

// called carries out a call job: the start of the run it calls, with the
// blobs its input names copied into that run's record first. The run counts
// one level below the caller, and takes nothing else from it.
func (p *pass) called(inst string, step *instance.Step, ask *instance.Ask) error {
	st := &step.State
	input, err := p.handUp(ask.Job.Input, ask.Run)
	if err != nil {
		return err
	}
	sender := &machine.Address{At: st.Location, Run: inst}
	if st.Lifetime == machine.LifetimeKeyed {
		key := st.Key
		sender.Key = &key
	}
	mac := hmac.New(sha256.New, st.Seed)
	mac.Write([]byte("call\x00" + ask.Message))
	msg := &instance.Message{Kind: instance.KindStart, Instance: ask.Run, From: inst, Sender: sender, Start: &instance.Start{
		Location: ask.Location, Input: input, Timeout: ask.Job.Timeout, Seed: mac.Sum(nil),
		Above: &instance.Above{Depth: st.Above.Depth + 1, StartedBy: inst, CallJob: ask.Name}}}
	failure, err := p.send(store.Message(p.ctl, p.ctl, ask.Message), noEnd, msg, p.c.ID.Card)
	if err != nil || failure == nil {
		return err
	}
	failure.Error = "the call could not be made: " + failure.Error
	return p.failJob(inst, ask.Name, ask.Message+".failed", failure)
}

// callIn takes a client's start of a location in a tree another client
// owns: it is a call. The controller checks the client's ceiling, and
// writes to itself the start of a run named under the tree's client, with
// the client's input copied into that run's record. That run answers the
// client: its first step writes the receipt, and its end the result beside
// it. A call done again writes the same start.
func (b *build) callIn(start *arrival, file *located, s *instance.Start) error {
	p := b.p
	answered, err := p.answered(start)
	if err != nil {
		return err
	}
	base := start.name[strings.LastIndexByte(start.name, '/')+1:]
	name := clientPrefix + start.sender + "." + base
	if !answered {
		_, mark, err := p.store.Read(p.ctx, store.Message(p.ctl, p.ctl, name))
		if err != nil {
			return err
		}
		answered = mark != store.None
	}
	if answered {
		b.drop(start)
		return nil
	}
	switch {
	case file.Lifetime != machine.LifetimeTransient:
		b.refuse(start, "%s is in a tree of the client %s, and is %s: another client starts a run there only by calling a transient location", s.Location, file.Client, file.Lifetime)
		return nil
	case len(s.Labels) > 0 || s.Allow != nil:
		b.refuse(start, "%s is in a tree of the client %s: a call gives an input and a timeout, and labels and an allow list are for runs of the client's own trees", s.Location, file.Client)
		return nil
	}
	client, err := p.trusted(start.sender)
	if err != nil || client == nil {
		return err
	}
	if r := (&permission.Set{Layers: []permission.Layer{ceilingOf(client)}}).Check(permission.Send, s.Location); r != nil {
		b.refuse(start, "%s", r.Error())
		return nil
	}
	input, why, err := b.clientInput(s)
	if err != nil || why != "" {
		if why != "" {
			b.refuse(start, "%s", why)
		}
		return err
	}
	run := store.RootInstance(file.Client, start.sent, name)
	if input, err = p.handUp(input, run); err != nil {
		return err
	}
	seed := make([]byte, 32)
	rand.Read(seed)
	msg := &instance.Message{Kind: instance.KindStart, Instance: run, Sender: &machine.Address{Client: start.sender}, Start: &instance.Start{
		Location: s.Location, Input: input, Timeout: s.Timeout, Seed: seed,
		Above: &instance.Above{CallReceipt: &instance.CallReceipt{Client: start.sender, Message: base}}}}
	failure, err := p.send(store.Message(p.ctl, p.ctl, name), noEnd, msg, p.c.ID.Card)
	if err != nil {
		return err
	}
	if failure != nil {
		b.refuse(start, "the call could not be made: %s", failure.Error)
		return nil
	}
	p.later()
	b.drop(start)
	return nil
}

// noCall leaves out the start of a run a call started that does not start:
// a call job fails, and a client is told in its receipt.
func (b *build) noCall(start *arrival, why string) {
	if start.callJob() {
		b.out.dropped = append(b.out.dropped, dropped{a: start, failed: why})
		return
	}
	b.refuse(start, "%s", why)
}

// callFailed fails the job of a call whose run did not start: the caller
// is given the failure in the run's place. Written under a name that
// follows from the start's, before the start is deleted, so it is written
// once.
func (p *pass) callFailed(a *arrival, why string) error {
	s := a.msg.Start
	msg := &instance.Message{Kind: instance.KindResult, Instance: s.Above.StartedBy, From: a.msg.Instance,
		Result: &instance.Result{Job: s.Above.CallJob, Status: machine.StatusError, Final: true,
			Error: &machine.Error{Status: machine.StatusError, Error: why}}}
	if _, err := p.send(a.name+".failed", noEnd, msg, p.c.ID.Card); err != nil {
		return err
	}
	_, err := p.store.Delete(p.ctx, a.name, store.Overwrite)
	return err
}

// result writes what became of a run a client called beside the call's
// receipt: the blobs its output names first, and then the result, sealed
// to the client. A client the controller no longer has a card for is
// written nothing, and the receipt kept for the result goes.
func (p *pass) result(ask *instance.Ask) error {
	to, err := p.trusted(ask.Sender)
	if err != nil {
		return err
	}
	if to == nil {
		_, err := p.store.Delete(p.ctx, store.Receipt(p.ctl, ask.Sender, ask.Message), store.Overwrite)
		return err
	}
	beside := store.ReceiptBlobs(p.ctl, ask.Sender, ask.Message)
	result := *ask.Result
	result.Output, err = blob.Walk(result.Output, func(_ string, ref blob.Ref) (any, error) {
		made, err := blob.CopyWhole(p.ctx, p.store, ref, p.ctl, beside)
		if err != nil {
			return ref.Value(), p.blobError(err)
		}
		return made.Value(), nil
	})
	if err != nil {
		return err
	}
	_, err = p.write(store.Result(p.ctl, ask.Sender, ask.Message), noEnd, &result, to.Card)
	return err
}
