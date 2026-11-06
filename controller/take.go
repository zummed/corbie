package controller

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// noEnd is the time until which something is good that is good for ever.
var noEnd time.Time

// taken is what came of giving an instance's waiting messages and wakes to
// a step.
type taken struct {
	step    *instance.Step // the step to write; nil if nothing led to one
	used    []*arrival     // what the step used
	dropped []dropped      // what was refused or dropped without a step
	rest    []*arrival     // what waits for the step after
}

// dropped is a message or a wake that led to no step. A client's message
// that is refused gets a receipt saying why.
type dropped struct {
	a       *arrival
	receipt *instance.Receipt
	// move says it is an event sent to { at } or { at, key }, which goes on
	// to the run that has the key.
	move bool
	// note says why an event sent to an address was not delivered: its
	// sender is told.
	note string
	// failed says why the run a call job called did not start: the job
	// fails.
	failed string
}

// The names of what a wake is for.
const (
	wakeTimeout = "timeout"
	wakeTimer   = "timer."
	wakeDue     = "due."
	wakeAgain   = "again."
)

// build is one step being worked out from the state before and what
// arrived. It changes nothing in the store.
type build struct {
	p    *pass
	inst string
	def  *machine.Definition
	st   *instance.State
	step *instance.Step
	time time.Time

	out     taken
	events  []machine.Event             // those for the machine
	sources []*arrival                  // where each came from
	index   int                         // how many messages the step has named
	held    []*instance.Going           // the jobs of this step that wait for the ask
	read    *stepOut                    // what the step read of the definition
	asks    map[string]*machine.JobSpec // the asks the definition writes, read once a step
	targets map[addressed]*target       // what each key a step sends to has
	// fromLocation says the run starts from its location's file, which its
	// first step copies into its record.
	fromLocation bool
	// err is the store failing while the step was worked out: the step is
	// not one that failed, and nothing is written for it.
	err error
}

// take gives what is waiting for an instance to its next step. A start is
// taken by itself; a cancel or a passed timeout is acted on first and
// nothing after it is given to the machine.
func (p *pass) take(inst string, latest *instance.Step, waiting []*arrival) (*taken, error) {
	// Messages are taken before wakes, whatever order they were found in: a
	// result that has arrived wins over its own due time.
	slices.SortStableFunc(waiting, func(x, y *arrival) int {
		switch {
		case x.wake == y.wake:
			return 0
		case y.wake:
			return -1
		}
		return 1
	})
	if latest == nil {
		return p.begin(inst, waiting)
	}
	b := &build{p: p, inst: inst}
	st := latest.State
	if err := clone(&latest.State, &st); err != nil {
		return nil, err
	}
	b.st = &st
	b.step = &instance.Step{Number: latest.Number + 1}
	b.time = latest.Time

	if st.Status != instance.Running {
		for _, a := range waiting {
			switch {
			case a.isSend() && (heard(&st, a) || a.starts(inst)):
				b.drop(a)
			case a.movable():
				b.move(a)
			case a.isSend():
				b.undeliver(a, fmt.Sprintf("the run %s has ended", inst))
			default:
				b.refuse(a, "the run has ended (it is %s)", st.Status)
			}
		}
		return &b.out, nil
	}
	for _, a := range waiting {
		if at := a.time(); a.good(p.now) && at.After(b.time) {
			b.time = at
		}
	}
	b.step.Time = b.time

	// An instance that is to end must not first start work on the strength
	// of an event that happened to sort ahead.
	for i, a := range waiting {
		status, text := b.ending(a)
		if status == "" {
			continue
		}
		b.use(a)
		b.record(machine.Event{Name: a.label()}, a, true)
		if !a.wake && a.kind == identity.Client {
			b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: a.sender, Message: a.name, Receipt: &instance.Receipt{Taken: true}})
		}
		b.end(status, nil, &machine.Error{Status: text.status, Error: text.text})
		if status == instance.Cancelled {
			b.unkept()
		}
		b.out.rest = append(slices.Clone(waiting[:i]), waiting[i+1:]...)
		return b.done()
	}
	// An event sent to { at } or { at, key } is first in its step: nothing
	// before it can end the run with the event given and not acted on. One
	// sent to the run by its ID is the run's to take, acted on or not.
	for i, a := range waiting {
		if a.movable() && len(b.out.used) > 0 {
			b.out.rest = waiting[i:]
			break
		}
		if err := b.arrive(a); err != nil {
			return nil, err
		}
	}
	if len(b.events) > 0 {
		if err := b.machine(); err != nil {
			return nil, err
		}
	}
	return b.done()
}

// done closes the step: its receipts are asked for and the entry is whole.
func (b *build) done() (*taken, error) {
	if b.err != nil {
		return nil, b.err
	}
	if len(b.out.used) == 0 {
		return &b.out, nil
	}
	for _, a := range b.out.used {
		b.step.Used = append(b.step.Used, a.name)
	}
	// What this step refused or dropped is part of the entry: it is
	// answered and deleted only once the entry is the one that was written.
	for _, d := range b.out.dropped {
		b.step.Used = append(b.step.Used, d.a.name)
		switch client, message, _ := d.a.client(); {
		case d.note != "":
			b.told(d.a, d.note)
		case d.receipt != nil:
			b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: client, Message: message, Receipt: d.receipt})
		}
	}
	b.out.dropped = nil
	b.step.Used = slices.Compact(slices.Sorted(slices.Values(b.step.Used)))
	b.step.State = *b.st
	b.out.step = b.step
	return &b.out, nil
}

func (a *arrival) time() time.Time {
	if a.wake {
		return a.at
	}
	return a.sent
}

// label is what an arrival that is the controller's own to act on is
// called in the record.
func (a *arrival) label() string {
	if a.wake {
		return a.what
	}
	return a.msg.Kind
}

type endText struct{ status, text string }

// ending says whether an arrival ends the instance: a cancel from one who
// may cancel it, or its timeout having passed.
func (b *build) ending(a *arrival) (string, endText) {
	switch {
	case a.wake && a.what == wakeTimeout && sameTime(b.st.TimeoutAt, a.at):
		return instance.Failed, endText{machine.StatusTimeout, "the timeout passed"}
	case !a.wake && a.msg.Kind == instance.KindCancel && a.good(b.p.now) && b.mayCancel(a):
		return instance.Cancelled, endText{machine.StatusCancelled, "cancelled"}
	}
	return "", endText{}
}

// mayCancel says whether a cancel is from one who may cancel the instance:
// the client that owns its root's tree, the controller's owner with the
// controller's own key, the controller for a submit, or the controller on
// behalf of the instance above or of the run whose call started it.
func (b *build) mayCancel(a *arrival) bool {
	switch a.kind {
	case identity.Client:
		return a.sender == store.InstanceClient(b.inst)
	case identity.Controller:
		return a.msg.From == "" || a.msg.From == b.st.Above.Parent || b.st.Above.CallJob != "" && a.msg.From == b.st.Above.StartedBy
	}
	return false
}

func (b *build) use(a *arrival) { b.out.used = append(b.out.used, a) }

// drop leaves a message or a wake out of the step; it is deleted and
// changes nothing.
func (b *build) drop(a *arrival) { b.out.dropped = append(b.out.dropped, dropped{a: a}) }

// refuse drops a message and, for a client's, says why in its receipt.
func (b *build) refuse(a *arrival, format string, args ...any) {
	d := dropped{a: a}
	if _, _, ok := a.client(); ok {
		d.receipt = &instance.Receipt{Refused: fmt.Sprintf(format, args...)}
	}
	b.out.dropped = append(b.out.dropped, d)
}

// record notes an event in the entry. own says the controller acted on it
// itself and the machine was not consulted.
func (b *build) record(event machine.Event, a *arrival, own bool) {
	b.step.Events = append(b.step.Events, instance.Arrived{Event: event, Source: a.name, Own: own})
}

// give hands an event to the machine.
func (b *build) give(event machine.Event, a *arrival) {
	b.use(a)
	b.record(event, a, false)
	b.events = append(b.events, event)
	b.sources = append(b.sources, a)
}

// arrive works out what one message or wake means for the instance, by its
// kind and by who sent it, checked against the instance's state.
func (b *build) arrive(a *arrival) error {
	if a.wake {
		b.wake(a)
		return nil
	}
	if !a.good(b.p.now) {
		b.refuse(a, "the message came too late: it was good until %s", a.until.Format(time.RFC3339))
		return nil
	}
	msg := a.msg
	switch msg.Kind {
	case instance.KindCancel:
		b.refuse(a, "only the client that started a run may cancel it")
	case instance.KindEvent:
		b.eventFrom(a)
	case instance.KindAnswer:
		b.answer(a)
	case instance.KindResult:
		if job := b.jobOf(a, msg.Result.Job); job != nil {
			b.result(a, job, msg.Result)
		} else {
			b.refuse(a, "a result comes from the runner its job was sent to")
		}
	case instance.KindEmit:
		b.emit(a)
	case instance.KindSend:
		if a.isSend() {
			b.sent(a)
		} else {
			b.drop(a)
		}
	case instance.KindNote:
		if a.kind != identity.Controller {
			b.drop(a)
			return nil
		}
		b.use(a)
		b.record(machine.Event{Name: a.label(), Data: msg.Note}, a, true)
		b.ask(instance.Ask{Kind: instance.AskNote, Text: msg.Note})
	default:
		// A start for an instance that exists, or a kind that is not for a
		// controller.
		b.drop(a)
	}
	return nil
}

// eventFrom takes a named event a parent sent the instance. Any other
// sender sends to a run by its address.
func (b *build) eventFrom(a *arrival) {
	msg := a.msg
	if msg.Event == nil || msg.Event.Name == "" {
		b.refuse(a, "the event has no name")
		return
	}
	if machine.Reserved(msg.Event.Name) {
		b.refuse(a, "no event can be sent under the name %q, which Corbie gives a machine itself", msg.Event.Name)
		return
	}
	if a.kind != identity.Controller || msg.From == "" || msg.From != b.st.Above.Parent {
		b.refuse(a, "an event is sent to a run by its address: its location, with its key or its ID")
		return
	}
	// From the parent: the controller may write one of these again when it
	// repeats a carry-out, and it arrives once.
	if !b.hear(a) {
		b.drop(a)
		return
	}
	b.give(machine.Event{Name: msg.Event.Name, From: machine.Parent, Data: msg.Event.Data}, a)
}

// hear says whether a message the controller wrote on behalf of a run is one
// not taken yet, and notes it: the controller may write one again when it
// repeats a carry-out, and it arrives once.
func (b *build) hear(a *arrival) bool {
	if heard(b.st, a) {
		return false
	}
	from := senderOf(a.msg)
	b.st.Heard = slices.DeleteFunc(b.st.Heard, func(last instance.Heard) bool { return last.From == from })
	b.st.Heard = append(b.st.Heard, instance.Heard{From: from, Message: a.name[strings.LastIndexByte(a.name, '/')+1:]})
	if len(b.st.Heard) > instance.MaxHeard {
		b.st.Heard = b.st.Heard[1:]
	}
	return true
}

// heard says whether a message does not sort after the last one taken from
// its sender.
func heard(st *instance.State, a *arrival) bool {
	name, from := a.name[strings.LastIndexByte(a.name, '/')+1:], senderOf(a.msg)
	return slices.ContainsFunc(st.Heard, func(last instance.Heard) bool { return last.From == from && name <= last.Message })
}

// answer takes an answer to a ticket. The ticket is the authority: any
// client that holds it may answer, and a ticket is answered once.
func (b *build) answer(a *arrival) {
	if a.kind != identity.Client || a.msg.Answer == nil {
		b.drop(a)
		return
	}
	for _, id := range sortedKeys(b.st.Tickets) {
		ticket := b.st.Tickets[id]
		if !hmac.Equal([]byte(ticket.Value), []byte(a.msg.Answer.Ticket)) {
			continue
		}
		delete(b.st.Tickets, id)
		b.st.Answered = append(b.st.Answered, mark(ticket.Value))
		if len(b.st.Answered) > 16 {
			b.st.Answered = b.st.Answered[len(b.st.Answered)-16:]
		}
		b.give(machine.Event{Name: ticket.Name, From: id, Data: a.msg.Answer.Value, Answer: true}, a)
		return
	}
	if slices.Contains(b.st.Answered, mark(a.msg.Answer.Ticket)) {
		b.refuse(a, "the ticket was already answered")
		return
	}
	b.refuse(a, "the run is not waiting on that ticket")
}

// mark is a short mark of a ticket's value, by which an answered ticket is
// remembered without being kept.
func mark(ticket string) string {
	sum := sha1.Sum([]byte(ticket))
	return hex.EncodeToString(sum[:8])
}

// jobOf finds the job a result or an emit is for: one the instance is
// still waiting on, from the runner the job was sent to, or from the child
// or the called run that is the job.
func (b *build) jobOf(a *arrival, full string) *instance.Going {
	for _, job := range b.st.Jobs {
		if job.Full() != full || job.Held {
			continue
		}
		switch a.kind {
		case identity.Runner:
			if job.Runner == a.sender {
				return job
			}
		case identity.Controller:
			// From a child that is the job, or from the instance itself for
			// a job that failed before anything was sent.
			if a.msg.From == b.inst || (job.Child != "" && job.Child == a.msg.From) || (job.Called != "" && job.Called == a.msg.From) {
				return job
			}
		}
	}
	return nil
}

// result takes the outcome of a job. Where an attempt failed and another
// is due, the machine is not consulted: the step is the controller's own.
func (b *build) result(a *arrival, job *instance.Going, res *instance.Result) {
	for _, used := range res.Used {
		if !slices.Contains(b.st.Used, used) {
			b.st.Used = append(b.st.Used, used)
		}
	}
	slices.Sort(b.st.Used)
	if res.Status == instance.Done {
		event := machine.Event{Name: machine.Done, From: job.Name, Data: res.Output}
		if job.Restart == nil || !job.Restart.Always || job.Name == instance.AskName || !b.mayRestart(job) {
			b.settle(job, event, a)
			return
		}
		event.From = job.Full()
		job.Anew = true
		b.restart(job, event, a)
		return
	}
	e := res.Error
	if e == nil {
		e = &machine.Error{Status: machine.StatusError, Error: "the job failed and gave no error"}
	}
	if e.Output == nil {
		e.Output = res.Output
	}
	// Only an error or a timeout restarts a job.
	again := (e.Status == machine.StatusError || e.Status == machine.StatusTimeout) && job.Restart != nil && !res.Final && b.mayRestart(job)
	if !again {
		b.settle(job, machine.Event{Name: machine.Failed, From: job.Name, Data: e.Value()}, a)
		return
	}
	b.restart(job, machine.Event{Name: machine.Failed, From: job.Full(), Data: e.Value()}, a)
}

// mayRestart says whether a job's limit leaves it a restart.
func (b *build) mayRestart(job *instance.Going) bool {
	r := job.Restart
	if r.Limit == 0 {
		return true
	}
	if r.Within == 0 {
		return job.Restarts < r.Limit
	}
	job.Restarted = slices.DeleteFunc(job.Restarted, func(at time.Time) bool { return b.time.Sub(at) >= time.Duration(r.Within) })
	return len(job.Restarted) < r.Limit
}

// restart takes the end of a job that runs again. The machine is not
// consulted: the step is the controller's own, and the event is only recorded.
func (b *build) restart(job *instance.Going, event machine.Event, a *arrival) {
	b.use(a)
	b.record(event, a, true)
	b.unwakeJob(job)
	b.forgetEmitted(job)
	if !job.Anew {
		job.Attempt++
	}
	job.Restarts++
	if job.Restart.Within > 0 {
		job.Restarted = append(job.Restarted, b.time)
	}
	var wait time.Duration
	wait, job.Doubled = job.Restart.Wait(job.Doubled, b.time.Sub(job.Began))
	job.Runner, job.Child, job.Called, job.Due = "", "", "", noEnd
	job.Again = b.time.Add(wait)
	job.Wake = b.wakeFor(wakeAgain, job.Key())
	b.ask(instance.Ask{Kind: instance.AskWake, At: job.Again, What: job.Wake})
}

// renew starts a job that ended well as a new job. The machine takes no step
// here, so what a step would have read is read for it. An expression of a
// machine made while running is never evaluated in the controller, and such a
// machine's jobs go only to an ask above it, which needs none of this.
func (b *build) renew(job *instance.Going) {
	var err error
	if b.def == nil {
		if b.def, err = b.p.definition(b.st); err != nil {
			if !errorsIs(err, machine.ErrDefinition) {
				b.err = err
				return
			}
			b.breach(nil, b.p.failf("%v", err))
			return
		}
	}
	if b.read == nil {
		b.read = &stepOut{}
		if !b.st.Above.Made {
			if b.read.Allow, err = b.definitionAllow(); err != nil {
				b.read.Unfilled = err.Error()
			}
		}
	}
	if err = b.begin(&job.Job, job); err != nil {
		b.err = err
		return
	}
	if len(b.held) > 0 && b.st.Status == instance.Running {
		b.startAsk()
	}
}

// settle gives the machine a job's final outcome. The outcome of the ask
// is the controller's own to act on.
func (b *build) settle(job *instance.Going, event machine.Event, a *arrival) {
	b.unwakeJob(job)
	b.forget(job)
	if job.Name == instance.AskName {
		b.askEnded(job, event, a)
		return
	}
	b.give(event, a)
}

// forget takes a job out of what the instance waits on.
func (b *build) forget(job *instance.Going) {
	delete(b.st.Jobs, job.Key())
	b.forgetEmitted(job)
}

// forgetEmitted lets the next attempt's values count from the start, so each
// is delivered.
func (b *build) forgetEmitted(job *instance.Going) {
	for key := range b.st.Emitted {
		if strings.HasPrefix(key, job.Name+"\x00") {
			delete(b.st.Emitted, key)
		}
	}
}

// wakeFor names a wake this step sets. The name carries the step's number,
// so no two steps ever name one wake.
func (b *build) wakeFor(kind, name string) string {
	return fmt.Sprintf("%s%s~%d", kind, name, b.step.Number)
}

// unwakeJob names a job's wake, for its due time or its next attempt, as
// no longer wanted.
func (b *build) unwakeJob(job *instance.Going) {
	at := job.Due
	if at.IsZero() {
		at = job.Again
	}
	if job.Wake != "" {
		b.unwake(at, job.Wake)
	}
	job.Wake = ""
}

// unwake names a wake that is no longer wanted, so it is deleted with what
// the step used.
func (b *build) unwake(at time.Time, what string) {
	if !at.IsZero() {
		b.step.Used = append(b.step.Used, store.Wake(b.p.ctl, at, b.inst, what))
	}
}

// emit takes a value a job sent up. The n-th value of a name from a job is
// given to the machine once, whichever attempt emits it and however many
// times the message arrives.
func (b *build) emit(a *arrival) {
	note := a.msg.Emit
	if note == nil {
		b.drop(a)
		return
	}
	job := b.jobOf(a, note.Job)
	if job == nil {
		b.refuse(a, "an emitted value comes from the runner its job was sent to")
		return
	}
	if machine.Reserved(note.Name) {
		b.drop(a)
		return
	}
	key := job.Name + "\x00" + note.Name
	if note.Index <= b.st.Emitted[key] {
		b.drop(a)
		return
	}
	if b.st.Emitted == nil {
		b.st.Emitted = map[string]int{}
	}
	b.st.Emitted[key] = note.Index
	b.give(machine.Event{Name: note.Name, From: job.Name, Data: note.Data}, a)
}

// wake acts on a wake that has come due. One for something the instance no
// longer waits on is deleted and changes nothing.
func (b *build) wake(a *arrival) {
	if name, ok := strings.CutPrefix(a.what, wakeTimer); ok {
		name = name[:max(strings.LastIndexByte(name, '~'), 0)]
		if t, set := b.st.Timers[name]; set && t.Wake == a.what && sameTime(t.At, a.at) {
			delete(b.st.Timers, name)
			b.give(machine.Event{Name: machine.Timer, From: name, Data: t.At.UTC().Format(time.RFC3339Nano)}, a)
			return
		}
	}
	for _, key := range sortedKeys(b.st.Jobs) {
		job := b.st.Jobs[key]
		switch {
		case job.Wake != a.what:
		case sameTime(job.Due, a.at):
			// No result came in time. One that has arrived was taken first,
			// and its job is no longer waited on.
			full := job.Full()
			b.cancel(job)
			job.Due, job.Wake = noEnd, ""
			b.result(a, job, &instance.Result{Job: full, Status: machine.StatusTimeout, Error: &machine.Error{
				Status: machine.StatusTimeout, Error: fmt.Sprintf("no result came within the job's time (%s)", timeout(job))}})
			return
		case sameTime(job.Again, a.at):
			b.use(a)
			b.record(machine.Event{Name: "again", From: job.Full()}, a, true)
			job.Again, job.Wake = noEnd, ""
			if job.Anew {
				b.renew(job)
			} else {
				b.launch(job)
			}
			return
		}
	}
	b.drop(a)
}

// sameTime says whether a time in the state is the time in a wake's name,
// which is kept to the millisecond.
func sameTime(at, wake time.Time) bool {
	return !at.IsZero() && store.Stamp(at) == store.Stamp(wake)
}

func timeout(job *instance.Going) machine.Duration {
	if job.Timeout > 0 {
		return job.Timeout
	}
	return machine.DefaultTimeout
}

// machine gives the machine its events. Each client's event is then
// answered: the machine acted on it, or had no rule for it. Its receipt
// stands before what the step asked for.
func (b *build) machine() error {
	at := len(b.step.Asks)
	ignored, stepped, err := b.consult()
	if err != nil {
		return err
	}
	var receipts []instance.Ask
	for i, a := range b.sources {
		client, message, ok := a.client()
		if !ok {
			continue
		}
		receipt := &instance.Receipt{Taken: true}
		if a.isSend() {
			receipt.Run = b.inst
			if stepped {
				acted := !slices.Contains(ignored, i)
				receipt.Acted = &acted
			}
		}
		receipts = append(receipts, instance.Ask{Kind: instance.AskReceipt, Sender: client, Message: message, Receipt: receipt})
	}
	b.step.Asks = slices.Insert(b.step.Asks, min(at, len(b.step.Asks)), receipts...)
	return nil
}

// consult gives the machine its events and applies what it asks for. If
// the step itself fails, the instance fails, and stepped is false. ignored
// lists the events the machine had no rule for.
func (b *build) consult() (ignored []int, stepped bool, err error) {
	if b.def == nil {
		if b.def, err = b.p.definition(b.st); err != nil {
			if !errorsIs(err, machine.ErrDefinition) {
				return nil, false, err
			}
			b.breach(nil, b.p.failf("%v", err))
			return nil, false, nil
		}
	}
	read, err := b.readStep(nil, nil)
	if err != nil || read == nil {
		return nil, false, err
	}
	return b.follow(read)
}

// readStep gives what a step reads of a definition: in a process of its own
// for a machine made while running and, after its first step, which holds
// nothing from outside, for one that takes shapes from outside; here for any
// other. s and start are the start of an instance's first step. It gives
// nothing where the machine ended instead.
func (b *build) readStep(s *instance.Start, start *arrival) (*stepOut, error) {
	if b.st.Above.Made || s == nil && machine.Lists(b.def) {
		return b.apart(s, start)
	}
	return b.readDefinition(s), nil
}

// readDefinition is the part of a step that reads the definition: everything
// that compiles or evaluates. What it gives is a function of the definition,
// the state and the events. s is the start of an instance's first step.
func (b *build) readDefinition(s *instance.Start) *stepOut {
	st, read := b.st, &stepOut{}
	if s != nil {
		problems, needsAsk := machine.Check(b.def)
		// A machine made while running has no ask of its own, and its author
		// is told so. Nothing it wrote of one is kept either way.
		if st.Above.Made && (b.def.Ask != nil || len(machine.Asks(b.def)) > 0) {
			problems = append(problems, machine.Problem{Text: "a machine made while running has no ask of its own: take out its `ask:`, and what it holds goes to the ask above it"})
		}
		// What a machine made while running holds is denied when it is held:
		// its definition's ask is not one that applies.
		if len(problems) == 0 && needsAsk && !st.Above.Made && st.Ask == nil && st.Above.Ask == nil {
			problems = machine.Problems{{Text: "a job is marked for approval, or a permission is listed to be asked about, and no ask applies: neither this definition nor any machine above it has one"}}
		}
		if read.Problems = problems; len(problems) > 0 {
			return read
		}
		// The input the machine is started with has its defaults filled in.
		var err error
		st.Input, err = machine.Input(b.def, s.Input)
		read.Input = st.Input
		if err != nil {
			read.Failure = err.Error()
			return read
		}
		read.Limit, err = machine.NewEvaluator(b.time, b.seed("timeout"), b.run()).Duration(b.def.Timeout, machine.Scope{"input": st.Input}, "limits.timeout")
		if err != nil {
			read.Failure = err.Error()
			return read
		}
		read.Title, err = machine.NewEvaluator(b.time, b.seed("title"), b.run()).Title(b.def.Title, machine.Scope{"input": st.Input}, "title")
		if err != nil {
			read.Failure = err.Error()
			return read
		}
		b.events = append([]machine.Event{{Name: machine.Start, Data: st.Input}}, b.events...)
	}
	t, ok := machine.TypeOf(b.def.Type)
	if !ok {
		read.Failure = fmt.Sprintf("%q is not a type of machine this controller knows", b.def.Type)
		return read
	}
	out, err := t.Step(&machine.StepIn{Definition: b.def, State: st.Inner, Events: b.events, Input: st.Input,
		Run: b.run(), Time: b.time, Seed: b.seed("step", fmt.Sprint(b.step.Number))})
	if err != nil {
		read.Failure = err.Error()
		return read
	}
	if read.Inner, err = json.Marshal(out.State, json.Deterministic(true)); err != nil {
		read.Failure = err.Error()
		return read
	}
	read.Requests, read.Ignored = out.Requests, out.Ignored
	b.readAllow(read)
	return read
}

// run is what an expression reads of the instance.
func (b *build) run() machine.Run {
	return machine.Run{ID: b.inst, Location: b.st.Location, Context: b.st.Above.Context, Values: b.st.Above.Values}
}

// readAllow fills in the allow list written for the whole definition, for
// the scripts of a step and for an ask of the machine's own.
func (b *build) readAllow(read *stepOut) {
	if !slices.ContainsFunc(read.Requests, func(r machine.Request) bool { return r.Job != nil || sendsTo(r) }) {
		return
	}
	var err error
	if read.Allow, err = b.definitionAllow(); err != nil {
		read.Unfilled = err.Error()
	}
}

// fill fills in an ask, here, from the input of the machine that declares it.
func (b *build) fill(spec *machine.JobSpec, input any, at string) (*machine.Job, error) {
	eval := machine.NewEvaluator(b.time, b.seed("ask", fmt.Sprint(b.step.Number), at), b.run())
	return spec.Fill(eval, machine.Scope{"input": input}, at, instance.AskName)
}

// askOf says which ask a held job goes to: the ask it carries or, with none,
// the machine's own or the nearest one above it. above says it is one above.
func (b *build) askOf(job *machine.Job) (spec *machine.JobSpec, input any, at string, above bool) {
	spec, input, at = job.Ask, b.st.Input, job.AskAt
	if spec == nil {
		spec, at = b.st.Ask, instance.AskName
	}
	if spec == nil && b.st.Above.Ask != nil {
		spec, input, at, above = b.st.Above.Ask.Spec, b.st.Above.Ask.Input, b.st.Above.Ask.At, true
	}
	return spec, input, at, above
}

// askAt gives the ask a definition writes at a place. A machine made while
// running has none of its own, and nothing of its definition is read for one.
func (b *build) askAt(at string) *machine.JobSpec {
	if at == "" || b.st.Above.Made {
		return nil
	}
	if b.asks == nil {
		b.asks = machine.Asks(b.def)
	}
	return b.asks[at]
}

// follow does what a step read: the machine's state is the one it read, and
// what the machine asked for is taken as any machine's is.
func (b *build) follow(read *stepOut) (ignored []int, stepped bool, err error) {
	if read.Failure != "" {
		b.end(instance.Failed, nil, b.p.failf("%s", read.Failure))
		return nil, false, nil
	}
	b.read, b.st.Inner = read, read.Inner
	if err := b.apply(read.Requests); err != nil {
		return nil, false, err
	}
	// The jobs of one step that need approval go to the ask together.
	if len(b.held) > 0 && b.st.Status == instance.Running {
		b.startAsk()
	}
	return read.Ignored, true, nil
}

// seed gives a secret that follows from the instance's own and from what
// it is for.
func (b *build) seed(what ...string) []byte {
	mac := hmac.New(sha256.New, b.st.Seed)
	for _, w := range what {
		mac.Write([]byte(w))
		mac.Write([]byte{0})
	}
	return mac.Sum(nil)
}

// ask adds to what the step asks for.
func (b *build) ask(a instance.Ask) { b.step.Asks = append(b.step.Asks, a) }

// message names the next message the step asks to be written to the
// controller's own inbox. The names of one step sort in the order asked,
// after those of every earlier step of the instance.
func (b *build) message(what string) string {
	b.index++
	return store.LinkMessage(b.inst, b.step.Number, b.index, what)
}

// apply turns what a machine asked for into what the controller will do.
func (b *build) apply(requests []machine.Request) error {
	if b.st.Above.Made && slices.ContainsFunc(requests, addresses) {
		b.end(instance.Failed, nil, b.p.failf("a machine made while running sends only to its parent and its children, and calls nothing"))
		return nil
	}
	if err := b.readAddresses(requests); err != nil {
		return err
	}
	// A send to a child waits until the step's jobs have been taken: one
	// made on entering a state, to that state's own child, is for the child
	// the same step starts.
	var down []*machine.Send
	flush := func() {
		for _, s := range down {
			b.send(s)
		}
		down = nil
	}
	defer flush()
	for _, r := range requests {
		switch {
		case r.Job != nil:
			if err := b.start(r.Job); err != nil {
				return err
			}
		case r.Cancel != "":
			if job, ok := b.st.Jobs[r.Cancel]; ok {
				b.cancel(job)
				b.unwakeJob(job)
				b.forget(job)
				if job.Held {
					b.unhold(job)
				}
			}
		case sendsTo(r):
			if b.sendTo(r.Send) {
				// The run was denied the send: nothing after it is done.
				down = nil
				return nil
			}
		case r.Send != nil && r.Send.To != machine.Parent:
			down = append(down, r.Send)
		case r.Send != nil:
			b.send(r.Send)
		case r.Timer != nil:
			if b.st.Timers == nil {
				b.st.Timers = map[string]instance.Timer{}
			}
			if old, set := b.st.Timers[r.Timer.Name]; set {
				b.unwake(old.At, old.Wake)
			}
			timer := instance.Timer{At: r.Timer.At, Wake: b.wakeFor(wakeTimer, r.Timer.Name)}
			b.st.Timers[r.Timer.Name] = timer
			b.ask(instance.Ask{Kind: instance.AskWake, At: timer.At, What: timer.Wake})
		case r.Clear != "":
			if old, set := b.st.Timers[r.Clear]; set {
				b.unwake(old.At, old.Wake)
			}
			delete(b.st.Timers, r.Clear)
		case r.Ticket != nil:
			if b.st.Tickets == nil {
				b.st.Tickets = map[string]instance.Ticket{}
			}
			b.st.Tickets[r.Ticket.ID] = instance.Ticket{Name: r.Ticket.Name, Value: r.Ticket.Value}
		case r.Drop != "":
			delete(b.st.Tickets, r.Drop)
		case r.Note != "":
			b.ask(instance.Ask{Kind: instance.AskNote, Text: r.Note})
		case r.End != nil:
			flush()
			if r.End.Error != nil {
				b.end(instance.Failed, r.End.Error.Output, r.End.Error)
			} else {
				b.end(instance.Done, r.End.Output, nil)
			}
			return nil
		}
	}
	return nil
}

// start takes a machine's request for a job: it is counted against the
// instance's limits and then launched.
func (b *build) start(req *machine.Job) error { return b.begin(req, nil) }

// begin starts a job. earlier is the job it follows, for one restarted after it
// ended well: the job is new, with what its limit and its delay have counted so
// far, and its lists were joined to the definition's when it first started.
func (b *build) begin(req *machine.Job, earlier *instance.Going) error {
	if old, ok := b.st.Jobs[req.Name]; ok {
		b.cancel(old)
		b.unwakeJob(old)
		b.forget(old)
	}
	if b.st.Asked == nil {
		b.st.Asked = map[string]int{}
	}
	b.st.Asked[req.Name]++
	job := &instance.Going{Job: *req, Asked: b.st.Asked[req.Name], Attempt: 1}
	if earlier != nil {
		job.Restarts, job.Restarted, job.Doubled = earlier.Restarts, earlier.Restarted, earlier.Doubled
	}
	// Where a job is, whether it is an ask from above, and what is written
	// at the place its ask is, the controller says.
	job.Found, job.Above, job.Ask = nil, false, b.askAt(job.AskAt)
	// The job's permissions are the lists written for the whole definition
	// and, with them, those written for the job.
	if b.st.Jobs == nil {
		b.st.Jobs = map[string]*instance.Going{}
	}
	b.st.Jobs[job.Name] = job
	b.st.JobsStarted++
	if !job.Alone && !job.IsMachine() && earlier == nil {
		// The lists written for the whole definition are for the scripts
		// it runs. A machine it starts has its own.
		if b.read.Unfilled != "" {
			// A scope that cannot be filled fails the job before anything
			// is sent.
			b.fail(job, "%s", b.read.Unfilled)
			return nil
		}
		job.Allow = append(slices.Clone(b.read.Allow), job.Allow...)
		job.Deny = append(slices.Clone(b.def.Deny), job.Deny...)
	}
	if job.IsSend() {
		// A send job is checked as a send action is, against every layer of
		// the run and the job's own lists, while the step is worked out.
		failure, err := b.sendJobTo(job)
		if err != nil {
			b.err = err
			return nil
		}
		if failure != nil {
			b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"), Error: failure})
			return nil
		}
	} else if job.IsCall() {
		// A call is checked as a send is, against every layer of the run
		// and the job's own lists, while the step is worked out.
		at, failure, err := b.callTo(job)
		if err != nil {
			b.err = err
			return nil
		}
		if failure != nil {
			b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"), Error: failure})
			return nil
		}
		job.Call = &machine.Address{At: at}
	} else {
		var err error
		if job.Found, err = found(&job.Job, b.def, b.st.Definition); err != nil {
			b.fail(job, "%v", err)
			return nil
		}
	}
	switch {
	case b.def.Jobs > 0 && b.st.JobsStarted > b.def.Jobs:
		b.fail(job, "job limit reached: this instance may start %d jobs", b.def.Jobs)
	case job.IsMachine() && b.st.Above.Depth+1 > b.st.Above.MaxDepth:
		b.fail(job, "depth limit reached: a root may have %d levels of machines below it", b.st.Above.MaxDepth)
	case b.holds(job):
		// Held before anything of it exists: no message to a runner, and no
		// child.
		job.Held = true
		b.held = append(b.held, job)
		b.ask(instance.Ask{Kind: instance.AskHold, Name: job.Full(), Job: &job.Job})
	default:
		b.launch(job)
	}
	return nil
}

// found says where a job's script or machine is, in the definition that
// holds the job or in the store where the job's expression gave it.
func found(job *machine.Job, def *machine.Definition, beside blob.Ref) (*machine.Found, error) {
	at := &machine.Found{In: def.Name}
	if stored, ok := blob.AsRef(job.Stored); ok {
		at.Blob = stored
		return at, nil
	}
	if job.Machine != "" {
		ref, ok := def.Machines[job.Machine]
		if !ok {
			return nil, fmt.Errorf("the machine %q is not among those the definition holds", job.Machine)
		}
		at.Blob = beside.Beside(ref)
		return at, nil
	}
	ref, ok := def.Scripts[job.Script]
	if !ok {
		return nil, fmt.Errorf("the script %s is not among those the definition holds", job.Script)
	}
	at.Blob = beside.Beside(ref)
	for _, file := range job.Files {
		ref, ok := def.Scripts[file]
		if !ok {
			return nil, fmt.Errorf("the file %q is not among those the definition holds", file)
		}
		if at.Files == nil {
			at.Files = map[string]blob.Ref{}
		}
		at.Files[file] = beside.Beside(ref)
	}
	return at, nil
}

// definitionAllow gives the allow list written for the whole definition,
// its scopes filled from the instance's input.
func (b *build) definitionAllow() ([]string, error) {
	var allow []string
	eval := machine.NewEvaluator(b.time, b.seed("allow"), b.run())
	for i, entry := range b.def.Allow {
		filled, err := eval.Entry(entry, machine.Scope{"input": b.st.Input}, fmt.Sprintf("allow[%d]", i))
		if err != nil {
			return nil, err
		}
		allow = append(allow, filled)
	}
	return allow, nil
}

// fail fails a job before anything is sent. The failure reaches the
// machine as a result does, as a message the carry-out writes, and the job
// is not tried again.
func (b *build) fail(job *instance.Going, format string, args ...any) {
	job.Runner, job.Child, job.Called = "", "", ""
	b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"), Error: b.p.failf(format, args...)})
}

// launch sends a job on its way: a script to a runner, a machine as a
// child of the controller's.
func (b *build) launch(job *instance.Going) {
	full := job.Full()
	job.Began = b.time
	if job.IsCall() {
		b.call(job)
		return
	}
	if job.IsSend() {
		// Its result, done with no output, follows the event, delivered or
		// not.
		at, why := b.addressAt(job.Send.At)
		ask := instance.Ask{Kind: instance.AskEvent, Name: full, Job: &job.Job, Message: b.message("send"),
			Event: &machine.Event{Name: job.Send.Event, Data: job.Send.Data}}
		b.aim(&ask, job.Send, at, why)
		return
	}
	if job.IsMachine() {
		job.Child = store.ChildInstance(b.inst, b.time, full)
		b.ask(instance.Ask{Kind: instance.AskChild, Name: full, Job: &job.Job, Attempt: job.Attempt, Child: job.Child,
			Message: b.message("start")})
		return
	}
	runners, err := b.p.runners()
	if err != nil {
		// The trusted runners could not be read: the pass ends, and the job
		// does not fail for a moment's trouble.
		b.err = err
		return
	}
	runner, card, err := b.pick(job, runners)
	if err != nil {
		// Which runner a job goes to is settled here, once; with no runner
		// it fails before anything is sent.
		b.fail(job, "%v", err)
		return
	}
	job.Runner = runner
	job.Due = b.time.Add(time.Duration(timeout(job)) + dueGrace)
	job.Wake = b.wakeFor(wakeDue, full)
	// The job, and then the wake for when its result is due.
	b.ask(instance.Ask{Kind: instance.AskScript, Name: full, Job: &job.Job, Attempt: job.Attempt, Runner: runner, Card: card,
		At: job.Due, What: job.Wake, Message: store.JobMessage(b.inst, full)})
}

// dueGrace is how long after a script's own time limit its result is
// allowed to take to arrive.
const dueGrace = 30 * time.Second

// pick chooses the runner for an attempt at a job from those that have
// every tag the job names. It takes no account of load: the same attempt
// always goes to the same runner, and a later attempt to a different one
// where more than one qualifies.
func (b *build) pick(job *instance.Going, runners []identity.Trust) (string, identity.Card, error) {
	var fit []identity.Card
	for _, r := range runners {
		if r.HasTags(job.Tags) || b.p.c.AnyRunner {
			fit = append(fit, r.Card)
		}
	}
	if len(fit) == 0 {
		if len(job.Tags) == 0 {
			return "", identity.Card{}, fmt.Errorf("the controller has no runner to send the job to")
		}
		return "", identity.Card{}, fmt.Errorf("no runner has the tags the job names: %s", strings.Join(job.Tags, ", "))
	}
	sum := sha256.Sum256([]byte(b.inst + "\x00" + machine.JobName(job.Name, job.Asked, 1)))
	card := fit[(binary.BigEndian.Uint64(sum[:8])+uint64(job.Attempt-1))%uint64(len(fit))]
	return card.Name, card, nil
}

// cancel asks for a job that is going to be stopped: a script by a message
// to its runner, a child by a message to itself.
func (b *build) cancel(job *instance.Going) {
	switch {
	case job.Runner != "":
		b.ask(instance.Ask{Kind: instance.AskCancel, Name: job.Full(), Runner: job.Runner, At: job.Due,
			Message: store.JobMessage(b.inst, job.Full()) + cancelSuffix})
	case job.Child != "":
		b.ask(instance.Ask{Kind: instance.AskCancel, Name: job.Full(), Child: job.Child, Message: b.message("cancel")})
	case job.Called != "":
		// The run a call started is no child: it is named by Run, which
		// nothing that removes what is below a run follows.
		b.ask(instance.Ask{Kind: instance.AskCancel, Name: job.Full(), Run: job.Called, Message: b.message("cancel")})
	}
}

// send takes a machine's request to send an event up to its parent or down
// to a child. A root's parent is its client, and what a root sends up is
// only written in its record. An event for a child that is not running is
// noted as undelivered and fails nothing.
func (b *build) send(s *machine.Send) {
	event := &machine.Event{Name: s.Event, Data: s.Data}
	if s.To == machine.Parent {
		ask := instance.Ask{Kind: instance.AskEvent, Event: event}
		if b.st.Above.Parent != "" {
			b.st.Sent++
			ask.To, ask.Attempt, ask.Message = b.st.Above.Parent, b.st.Sent, b.message("emit")
		}
		b.ask(ask)
		return
	}
	event.From = machine.Parent
	job, ok := b.st.Jobs[s.To]
	if !ok || job.Child == "" {
		b.ask(instance.Ask{Kind: instance.AskEvent, Name: s.To, Event: event, Undelivered: true})
		return
	}
	b.ask(instance.Ask{Kind: instance.AskEvent, Name: s.To, To: job.Child, Event: event, Message: b.message("event")})
}

// end ends the instance. Everything still going below it is cancelled, and
// its parent is told last, after any events the step sent.
func (b *build) end(status string, output any, e *machine.Error) {
	st := b.st
	st.Status, st.Output, st.Error, st.Ended = status, output, e, b.time
	for _, name := range sortedKeys(st.Jobs) {
		job := st.Jobs[name]
		b.cancel(job)
		b.unwakeJob(job)
	}
	for _, name := range sortedKeys(st.Timers) {
		b.unwake(st.Timers[name].At, st.Timers[name].Wake)
	}
	b.unwake(st.TimeoutAt, wakeTimeout)
	st.Jobs, st.Timers, st.Tickets, st.Emitted = nil, nil, nil, nil
	// When its keep has passed, the instance is due to be removed: a moment
	// sooner for each machine above it, so that what is below is due first.
	gone := b.time.Add(time.Duration(st.Keep) - time.Duration(st.Above.Depth)*time.Millisecond)
	b.ask(instance.Ask{Kind: instance.AskWake, At: gone, What: wakeRemove})
	switch {
	case st.Above.Parent != "":
		b.ask(instance.Ask{Kind: instance.AskEnd, To: st.Above.Parent, Name: st.Above.Job, Output: output, Error: e,
			Message: b.message("result")})
	case st.Above.CallJob != "":
		// A called run's end is its call job's result.
		b.ask(instance.Ask{Kind: instance.AskEnd, To: st.Above.StartedBy, Name: st.Above.CallJob, Output: output, Error: e,
			Message: b.message("result")})
	case st.Above.CallReceipt != nil:
		// Written beside the receipt of the client that called.
		result := &instance.CallResult{Status: status, Output: output}
		if e != nil {
			failure := *e
			failure.Output = nil
			result.Error = &failure
		}
		b.ask(instance.Ask{Kind: instance.AskResult, Sender: st.Above.CallReceipt.Client, Message: st.Above.CallReceipt.Message, Result: result})
	}
	// A run that a send started and that fails as it starts is that send's
	// failure.
	if st.Above.StartedBy != "" && st.Above.CallJob == "" && status == instance.Failed && b.step.Number == 1 {
		b.ask(instance.Ask{Kind: instance.AskTell, To: st.Above.StartedBy, Message: b.message("tell"),
			Text: fmt.Sprintf("the run %s, started by a send to %s#%s, failed as it started: %s", b.inst, st.Location, st.Key, e.Error)})
	}
}

// unkept notes, in the record of each run that sent one, the events a
// cancelled run kept to take later: they are not delivered.
func (b *build) unkept() {
	at := addressText(&machine.Address{At: b.st.Location, Run: b.inst})
	for _, event := range machine.Kept(b.st.Type, b.st.Inner) {
		if event.Sender == nil || event.Sender.Run == "" {
			continue
		}
		b.ask(instance.Ask{Kind: instance.AskTell, To: event.Sender.Run, Message: b.message("tell"),
			Text: fmt.Sprintf("the event %s sent to %s was not delivered: the run was cancelled while it kept the event under defer:", event.Name, at)})
	}
}

func clone[T any](from, to *T) error {
	data, err := json.Marshal(from, json.Deterministic(true))
	if err != nil {
		return err
	}
	var fresh T
	if err := json.Unmarshal(data, &fresh); err != nil {
		return err
	}
	*to = fresh
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// begin takes the first step of an instance that has none: a start, by
// itself. Any other message for the instance waits for the step after.
func (p *pass) begin(inst string, waiting []*arrival) (*taken, error) {
	b := &build{p: p, inst: inst, st: &instance.State{}, step: &instance.Step{Number: 1}}
	var start *arrival
	for _, a := range waiting {
		if a.starts(inst) && start == nil {
			start = a
			continue
		}
		b.out.rest = append(b.out.rest, a)
	}
	if start == nil {
		// Nothing will ever take a message or a wake for an instance that
		// does not exist, but an event sent to { at } or { at, key } goes
		// on.
		b.out.rest = nil
		for _, a := range waiting {
			switch {
			case a.movable():
				b.move(a)
			case a.isSend():
				b.undeliver(a, fmt.Sprintf("there is no such run as %s", inst))
			default:
				b.refuse(a, "there is no such run")
			}
		}
		return &b.out, nil
	}
	s, above := start.msg.Start, &instance.Above{}
	switch {
	case !start.good(p.now):
		b.refuse(start, "the start came too late: it was good until %s, and the run was never taken up", start.until.Format(time.RFC3339))
		return &b.out, nil
	case start.kind == identity.Client && (start.sender != store.InstanceClient(inst) || s.Above != nil):
		b.refuse(start, "a run's name begins with the name of the client that starts it")
		return &b.out, nil
	case start.kind == identity.Client && s.Location == "":
		b.refuse(start, "a client's start names a location: a run starts at the location of a file in a tree")
		return &b.out, nil
	case start.isSend() || s.Location != "" && (s.Above == nil || isCall(s)) && (start.kind == identity.Client || start.kind == identity.Controller):
		// A start of a location: the run begins from the file recorded
		// there. A client's start of a location in another client's tree
		// is a call, which starts a run under that client.
		var err error
		if s, err = b.located(start); err != nil || s == nil {
			return &b.out, err
		}
		above = s.Above
	case start.kind == identity.Controller && s.Above != nil:
		above = s.Above
	default:
		b.drop(start)
		return &b.out, nil
	}
	b.time = start.sent
	b.step.Time = b.time
	st := b.st
	st.Status, st.Started, st.Definition, st.Input, st.Labels, st.Seed, st.Above, st.Key = instance.Running, b.time, s.Definition, s.Input, s.Labels, s.Seed, *above, s.Key
	if st.Location == "" {
		// A child runs at its parent's location.
		st.Location, st.Tree = s.Location, s.Tree
	}
	if start.isSend() {
		// The data of the event is the run's input, and the event its first
		// after its start.
		copied := *s
		s, copied.Input = &copied, start.msg.Event.Data
		st.Input = s.Input
		b.events = []machine.Event{sentEvent(start.msg)}
		b.hear(start)
	}
	refused, err := b.first(s, start)
	if err != nil {
		return nil, err
	}
	if refused {
		return &b.out, nil
	}
	return b.done()
}

// sentEvent is the event a message sent to an address gives the run.
func sentEvent(msg *instance.Message) machine.Event {
	return machine.Event{Name: msg.Event.Name, Data: msg.Event.Data, Sender: msg.Sender}
}

// first takes the first step of an instance from the point where its
// definition is read, and says whether its start was refused, or left for
// another run, before any instance exists.
func (b *build) first(s *instance.Start, start *arrival) (refused bool, err error) {
	p, st := b.p, b.st

	// The controller checks the definition again with the same code as the
	// client: it does not take a client's word.
	def, err := p.definitionAt(s.Definition)
	read := &stepOut{}
	switch {
	case err == nil:
	case errorsIs(err, machine.ErrDefinition):
		read.Problems = machine.Problems{{Text: err.Error()}}
		def = &machine.Definition{}
	default:
		return false, err
	}
	b.def = def
	// A run a send starts takes the event only where its location's file
	// takes it. One whose file cannot be read fails as it starts.
	if start.isSend() && len(read.Problems) == 0 {
		if why := b.refuses(start.msg, def); why != "" {
			b.undeliver(start, why)
			return true, nil
		}
	}
	// A call is taken only where its location's file takes the caller. Its
	// receive: is read as for every event: a mapping, which names events,
	// takes a call from no one but the tree's client.
	call := isCall(s)
	if call && len(read.Problems) == 0 && !b.receives(def, start.msg.Sender, "") {
		b.noCall(start, fmt.Sprintf("%s does not receive a call from %s", st.Location, addressText(start.msg.Sender)))
		return true, nil
	}
	// The key is taken before anything of the run is written, so a run that
	// loses it has done nothing.
	if st.Lifetime == machine.LifetimeKeyed || st.Lifetime == machine.LifetimeSingleton {
		inUse, err := p.claim(b.inst, store.Key(p.ctl, st.Location, s.Key), s.Key, start.name)
		if err != nil {
			return false, err
		}
		switch {
		case inUse != "" && start.isSend():
			b.move(start)
			return true, nil
		case inUse != "":
			// A later submit started the singleton again: this start is left.
			b.drop(start)
			return true, nil
		}
	}
	// A run started at a location keeps its own copy of the file recorded
	// there, and of what that names.
	gone := false
	if b.fromLocation {
		copied, err := blob.CopyWhole(p.ctx, p.store, s.Definition, p.ctl, store.Blobs(p.ctl, b.inst))
		switch {
		case err == nil:
			st.Definition = copied
		case p.blobError(err) != nil:
			return false, p.blobError(err)
		case start.kind == identity.Client || call:
			b.noCall(start, fmt.Sprintf("the file at %s is no longer in the store", st.Location))
			return true, nil
		default:
			gone = true
		}
	}
	st.Machine, st.Type = def.Name, def.Type
	// A machine made while running has no ask of its own, and what it wrote
	// of one is not kept.
	if !st.Above.Made {
		st.Ask = def.Ask
	}
	st.Keep = machine.Duration(p.keepFor(def, &st.Above))
	st.Above.AskAbout = append(slices.Clone(st.Above.AskAbout), def.AskAbout...)
	if len(read.Problems) == 0 && !gone {
		if read, err = b.readStep(s, start); err != nil || read == nil {
			return false, err
		}
	}
	problems := read.Problems
	switch {
	case len(problems) > 0 && start.callJob():
		// Refused before any instance exists.
		b.noCall(start, fmt.Sprintf("the definition did not pass its check: %s", strings.ReplaceAll(problems.Error(), "\n", "; ")))
		return true, nil
	case len(problems) > 0 && (start.kind == identity.Client || call):
		// Refused before any instance exists, in the receipt.
		b.out.dropped = append(b.out.dropped, dropped{a: start, receipt: &instance.Receipt{
			Refused: "the definition did not pass its check", Problems: problems}})
		return true, nil
	}
	if len(problems) == 0 {
		st.Input, st.Title = read.Input, read.Title
	}
	b.use(start)
	b.record(machine.Event{Name: machine.Start, Data: st.Input}, start, false)
	if start.isSend() {
		b.record(sentEvent(start.msg), start, false)
	}
	if start.kind == identity.Client || st.Lifetime != "" {
		// A root started at its location counts its depth from its own
		// file.
		st.Above.MaxDepth = machine.DefaultDepth
		if def.Depth != nil {
			st.Above.MaxDepth = *def.Depth
		}
	}
	if call && st.Above.CallReceipt != nil {
		// The client that called is told the call was taken, and which run
		// it started.
		b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: st.Above.CallReceipt.Client, Message: st.Above.CallReceipt.Message,
			Run: b.inst, Receipt: &instance.Receipt{Taken: true, Run: b.inst}})
	}
	if start.kind == identity.Client {
		b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: start.sender, Message: start.name, Receipt: &instance.Receipt{Taken: true}})
		if s.Allow != nil {
			layer := *s.Allow
			layer.Name = "the list the run was started with"
			layer.Fix = "start the run with %s in its list as well (corbie run submit --allow)"
			st.Above.Layers = []permission.Layer{layer}
		}
	}
	answer := func(ignored []int, stepped bool) {
		// A client whose send started the run is told the run took it.
		client, message, ok := start.client()
		if !ok || !start.isSend() {
			return
		}
		receipt := &instance.Receipt{Taken: true, Run: b.inst}
		if stepped {
			acted := !slices.Contains(ignored, 1)
			receipt.Acted = &acted
		}
		b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: client, Message: message, Receipt: receipt})
	}
	switch {
	case gone:
		answer(nil, false)
		b.end(instance.Failed, nil, b.p.failf("its machine is no longer in the store"))
		return false, nil
	case len(problems) > 0:
		answer(nil, false)
		b.end(instance.Failed, nil, b.p.failf("the definition did not pass its check: %s", strings.ReplaceAll(problems.Error(), "\n", "; ")))
		return false, nil
	case st.Above.Depth > st.Above.MaxDepth:
		answer(nil, false)
		b.end(instance.Failed, nil, b.p.failf("depth limit reached: the run counts %d levels below the run that began its chain of sends, and its file allows %d", st.Above.Depth, st.Above.MaxDepth))
		return false, nil
	case read.Failure != "":
		answer(nil, false)
		_, _, err = b.follow(read)
		return false, err
	}
	// Its older history is trimmed as it runs, a keep at a time.
	b.ask(instance.Ask{Kind: instance.AskWake, At: b.time.Add(time.Duration(st.Keep)), What: wakeTrim})
	limit := read.Limit
	if s.Timeout > 0 && (limit == 0 || s.Timeout < limit) {
		limit = s.Timeout
	}
	if limit > 0 {
		st.TimeoutAt = b.time.Add(time.Duration(limit))
		b.ask(instance.Ask{Kind: instance.AskWake, At: st.TimeoutAt, What: wakeTimeout})
	}
	ignored, stepped, err := b.follow(read)
	answer(ignored, stepped)
	return false, err
}

// holds says whether a job waits for a yes before it starts: it is marked
// for approval, or it declares a permission that its machine, or a machine
// above it, lists as one to be asked about. Whether it is held depends on
// what it declares alone, before anything is sent.
func (b *build) holds(job *instance.Going) bool {
	declared := job.Allow
	switch {
	case job.IsCall():
		// A call is a send to its location.
		declared = append(slices.Clone(declared), permission.Send+":"+job.Call.At)
	case job.IsSend() && strings.HasPrefix(job.Send.At.At, "/"):
		declared = append(slices.Clone(declared), permission.Send+":"+job.Send.At.At)
	}
	return job.Approve || permission.Declares(declared, b.st.Above.AskAbout, b.st.Tree)
}

// startAsk starts each ask once for the jobs this step held that go to
// it. The asks start in the order their first job was held.
func (b *build) startAsk() {
	var places []string
	held := map[string][]*instance.Going{}
	for _, job := range b.held {
		_, _, at, _ := b.askOf(&job.Job)
		if _, seen := held[at]; !seen {
			places = append(places, at)
		}
		held[at] = append(held[at], job)
	}
	b.held = nil
	for _, at := range places {
		b.startOneAsk(held[at])
	}
}

// startOneAsk starts an ask for the jobs that go to it: the ask they carry
// or, with none, the machine's own or the nearest one above it. It is
// filled once, from the input of the machine that declares it. An ask that
// cannot be filled denies the jobs, with a note that names the fault.
func (b *build) startOneAsk(held []*instance.Going) {
	deny := func(format string, args ...any) {
		for _, job := range held {
			job.Held = false
			b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"),
				Error: &machine.Error{Status: machine.StatusDenied, Error: fmt.Sprintf(format, args...)}})
		}
	}
	spec, input, at, above := b.askOf(&held[0].Job)
	switch {
	case spec == nil && b.st.Above.Made:
		deny("the job needs approval and no ask applies: a machine made while running uses only an ask above it, and none is above it")
		return
	case spec == nil:
		deny("the job needs approval and no ask applies: neither this machine nor any above it has one")
		return
	}
	filled, err := b.fill(spec, input, at)
	if err != nil {
		deny("the ask could not be started: %v", err)
		return
	}
	filled.Name, filled.Above = instance.AskName, above
	// An ask taken from above is held in the definition that declares it.
	def, beside := b.def, b.st.Definition
	if above {
		beside = b.st.Above.Ask.Definition
		if def, err = b.p.definitionAt(beside); err != nil {
			if !errorsIs(err, machine.ErrDefinition) {
				b.err = err
				return
			}
			deny("the ask could not be started: %v", err)
			return
		}
	}
	if filled.Found, err = found(filled, def, beside); err != nil {
		deny("the ask could not be started: %v", err)
		return
	}
	// The ask is told which machine is asking and, for each job, what it
	// is and what input it would get.
	told := map[string]any{}
	if given, ok := filled.Input.(map[string]any); ok {
		for k, v := range given {
			told[k] = v
		}
	}
	var jobs []any
	var names []string
	for _, job := range held {
		what := map[string]any{"name": job.Full(), "input": job.Input, "allow": toList(job.Allow)}
		if job.Shown != nil {
			what["asked"] = job.Shown
		}
		if job.IsMachine() {
			what["machine"] = job.Machine
		} else {
			what["script"] = job.Script
		}
		jobs = append(jobs, what)
		names = append(names, job.Key())
	}
	if !above && !filled.IsMachine() {
		// A script of the definition has the lists written for the whole of it.
		if b.read.Unfilled != "" {
			deny("the ask could not be started: %s", b.read.Unfilled)
			return
		}
		filled.Allow = append(slices.Clone(b.read.Allow), filled.Allow...)
		filled.Deny = append(slices.Clone(b.def.Deny), filled.Deny...)
	}
	told["asking"] = map[string]any{"machine": b.st.Machine, "instance": b.inst}
	told["jobs"] = jobs
	filled.Input = told
	b.st.Asked[instance.AskName]++
	ask := &instance.Going{Job: *filled, Asked: b.st.Asked[instance.AskName], Attempt: 1, ForAsk: names}
	b.st.Jobs[ask.Key()] = ask
	if ask.IsMachine() && b.st.Above.Depth+1 > b.st.Above.MaxDepth {
		b.fail(ask, "depth limit reached: a root may have %d levels of machines below it", b.st.Above.MaxDepth)
		return
	}
	b.launch(ask)
}

func toList(texts []string) []any {
	list := make([]any, len(texts))
	for i, text := range texts {
		list[i] = text
	}
	return list
}

// askEnded acts on how the ask ended. Approved jobs start. A job not
// approved never starts: its machine sees it as a failed job with the
// status denied, and every one of them is given and written in the record.
// An ask that fails, or whose result does not list the jobs it approves,
// approves nothing.
func (b *build) askEnded(ask *instance.Going, outcome machine.Event, a *arrival) {
	b.use(a)
	b.record(machine.Event{Name: outcome.Name, From: ask.Full(), Data: outcome.Data}, a, true)
	approved := map[string]bool{}
	reason := "the ask did not approve the job"
	result, _ := outcome.Data.(map[string]any)
	switch list, isList := result["approve"].([]any); {
	case outcome.Name == machine.Failed:
		reason = fmt.Sprintf("the ask failed, which approves nothing: %v", result["error"])
	case !isList:
		reason = "the ask's result has no approve list, so nothing is approved: its result should be { approve: [names of jobs], note }"
	default:
		for _, name := range list {
			if text, ok := name.(string); ok {
				approved[text] = true
			}
		}
		if note, ok := result["note"].(string); ok && note != "" {
			reason += ": " + note
		}
	}
	for _, key := range ask.ForAsk {
		job, ok := b.st.Jobs[key]
		if !ok || !job.Held {
			continue
		}
		job.Held = false
		if approved[job.Full()] {
			b.launch(job)
			continue
		}
		b.forget(job)
		failure := &machine.Error{Status: machine.StatusDenied, Error: reason}
		b.record(machine.Event{Name: machine.Failed, From: job.Name, Data: failure.Value()}, a, false)
		b.events = append(b.events, machine.Event{Name: machine.Failed, From: job.Name, Data: failure.Value()})
		b.sources = append(b.sources, a)
	}
}

// unhold drops a held job that is no longer wanted. An ask left with no
// held job is cancelled.
func (b *build) unhold(job *instance.Going) {
	for _, key := range sortedKeys(b.st.Jobs) {
		ask := b.st.Jobs[key]
		at := slices.Index(ask.ForAsk, job.Key())
		if ask.Name != instance.AskName || at < 0 {
			continue
		}
		ask.ForAsk = slices.Delete(ask.ForAsk, at, at+1)
		if len(ask.ForAsk) == 0 {
			b.cancel(ask)
			b.unwakeJob(ask)
			b.forget(ask)
		}
	}
}

// keepFor is how long an instance is kept after it ends: what its
// definition says, or a week, and no longer than what started it is kept.
func (p *pass) keepFor(def *machine.Definition, above *instance.Above) time.Duration {
	keep := time.Duration(machine.DefaultKeep)
	if def != nil && def.Keep > 0 {
		keep = time.Duration(def.Keep)
	}
	if above.Keep > 0 && time.Duration(above.Keep) < keep {
		keep = time.Duration(above.Keep)
	}
	return keep
}
