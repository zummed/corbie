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
	events  []machine.Event   // those for the machine
	sources []*arrival        // where each came from
	index   int               // how many messages the step has named
	held    []*instance.Going // the jobs of this step that wait for the ask
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
			b.refuse(a, "the run has ended (it is %s)", st.Status)
		}
		return &b.out, nil
	}
	var err error
	if b.def, err = p.definition(b.st); err != nil {
		return nil, err
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
		b.out.rest = append(slices.Clone(waiting[:i]), waiting[i+1:]...)
		return b.done()
	}
	for _, a := range waiting {
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
		if d.receipt != nil {
			b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: d.a.sender, Message: d.a.name, Receipt: d.receipt})
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
// the client that started it, the controller's owner with the controller's
// own key, or the controller on behalf of the instance above.
func (b *build) mayCancel(a *arrival) bool {
	switch a.kind {
	case identity.Client:
		return a.sender == store.InstanceClient(b.inst)
	case identity.Controller:
		return a.msg.From == "" || a.msg.From == b.st.Above.Parent
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
	if !a.wake && a.kind == identity.Client {
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
	default:
		// A start for an instance that exists, or a kind that is not for a
		// controller.
		b.drop(a)
	}
	return nil
}

// eventFrom takes a named event sent to the instance: by the client that
// started it, which stands where a parent would, or by its parent.
func (b *build) eventFrom(a *arrival) {
	msg := a.msg
	if msg.Event == nil || msg.Event.Name == "" {
		b.refuse(a, "the event has no name")
		return
	}
	from := ""
	switch {
	case a.kind == identity.Client && a.sender == store.InstanceClient(b.inst):
		// A message a client sent that has its receipt has been taken
		// already: sent again, it gets the same answer and no second step.
		answered, err := b.p.answered(a)
		if err != nil {
			b.err = err
			return
		}
		if answered {
			b.drop(a)
			return
		}
		from = "client"
	case a.kind == identity.Controller && msg.From != "" && msg.From == b.st.Above.Parent:
		// From the parent: the controller may write one of these again
		// when it repeats a carry-out, and it arrives once.
		name := a.name[strings.LastIndexByte(a.name, '/')+1:]
		if name <= b.st.Taken {
			b.drop(a)
			return
		}
		b.st.Taken = name
		from = machine.Parent
	default:
		b.refuse(a, "only the client that started a run may send it events")
		return
	}
	b.give(machine.Event{Name: msg.Event.Name, From: from, Data: msg.Event.Data}, a)
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
// that is the job.
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
			if a.msg.From == b.inst || (job.Child != "" && job.Child == a.msg.From) {
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
		b.settle(job, machine.Event{Name: machine.Done, From: job.Name, Data: res.Output}, a)
		return
	}
	e := res.Error
	if e == nil {
		e = &machine.Error{Status: machine.StatusError, Error: "the job failed and gave no error"}
	}
	if e.Output == nil {
		e.Output = res.Output
	}
	retry := (e.Status == machine.StatusError || e.Status == machine.StatusTimeout) && job.Attempt < job.Attempts && !res.Final
	if !retry {
		b.settle(job, machine.Event{Name: machine.Failed, From: job.Name, Data: e.Value()}, a)
		return
	}
	// Only an error or a timeout is tried again, each attempt a little
	// later than the last.
	b.use(a)
	b.record(machine.Event{Name: machine.Failed, From: job.Full(), Data: e.Value()}, a, true)
	b.unwakeJob(job)
	job.Attempt++
	job.Runner, job.Child, job.Due = "", "", noEnd
	job.Again = b.time.Add(time.Second << min(job.Attempt-2, 8))
	job.Wake = b.wakeFor(wakeAgain, job.Key())
	b.ask(instance.Ask{Kind: instance.AskWake, At: job.Again, What: job.Wake})
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
			b.launch(job)
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

// machine gives the machine its events and applies what it asks for. If
// the step itself fails, the instance fails.
func (b *build) machine() error {
	t, ok := machine.TypeOf(b.def.Type)
	if !ok {
		b.end(instance.Failed, nil, b.p.failf("%q is not a type of machine this controller knows", b.def.Type))
		return nil
	}
	out, err := t.Step(&machine.StepIn{Definition: b.def, State: b.st.Inner, Events: b.events, Input: b.st.Input,
		Time: b.time, Seed: b.seed("step", fmt.Sprint(b.step.Number))})
	if err != nil {
		b.end(instance.Failed, nil, b.p.failf("%v", err))
		return nil
	}
	if b.st.Inner, err = json.Marshal(out.State, json.Deterministic(true)); err != nil {
		return err
	}
	// Each client's event is answered: the machine acted on it, or had no
	// rule for it.
	for i, a := range b.sources {
		if a.wake || a.kind != identity.Client {
			continue
		}
		receipt := &instance.Receipt{Taken: true}
		if a.msg.Kind == instance.KindEvent {
			acted := !slices.Contains(out.Ignored, i)
			receipt.Acted = &acted
		}
		b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: a.sender, Message: a.name, Receipt: receipt})
	}
	if err := b.apply(out.Requests); err != nil {
		return err
	}
	// The jobs of one step that need approval go to the ask together.
	if len(b.held) > 0 && b.st.Status == instance.Running {
		return b.startAsk()
	}
	return nil
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
func (b *build) start(req *machine.Job) error {
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
	job.Attempts = max(job.Attempts, 1)
	// The job's permissions are the lists written for the whole definition
	// and, with them, those written for the job.
	if b.st.Jobs == nil {
		b.st.Jobs = map[string]*instance.Going{}
	}
	b.st.Jobs[job.Name] = job
	b.st.JobsStarted++
	if !job.Alone && !job.IsMachine() {
		// The lists written for the whole definition are for the scripts
		// it runs. A machine it starts has its own.
		allow, err := b.definitionAllow()
		if err != nil {
			// A scope that cannot be filled fails the job before anything
			// is sent.
			b.fail(job, "%v", err)
			return nil
		}
		job.Allow = append(allow, job.Allow...)
		job.Deny = append(slices.Clone(b.def.Deny), job.Deny...)
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

// definitionAllow gives the allow list written for the whole definition,
// its scopes filled from the instance's input.
func (b *build) definitionAllow() ([]string, error) {
	var allow []string
	eval := machine.NewEvaluator(b.time, b.seed("allow"))
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
	job.Runner, job.Child = "", ""
	b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"), Error: b.p.failf(format, args...)})
}

// launch sends a job on its way: a script to a runner, a machine as a
// child of the controller's.
func (b *build) launch(job *instance.Going) {
	full := job.Full()
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
			Message: store.JobMessage(b.inst, job.Full()) + ".cancel"})
	case job.Child != "":
		b.ask(instance.Ask{Kind: instance.AskCancel, Name: job.Full(), Child: job.Child, Message: b.message("cancel")})
	}
}

// send takes a machine's request to send an event: up to its parent, or
// down to a child. A root's parent is its client, and what a root sends up
// is only written in its record. An event for a child that is not running
// is noted as undelivered and fails nothing.
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
	gone := b.time.Add(b.p.keepFor(b.def, &b.st.Above) - time.Duration(st.Above.Depth)*time.Millisecond)
	b.ask(instance.Ask{Kind: instance.AskWake, At: gone, What: wakeRemove})
	if st.Above.Parent != "" {
		b.ask(instance.Ask{Kind: instance.AskEnd, To: st.Above.Parent, Name: st.Above.Job, Output: output, Error: e,
			Message: b.message("result")})
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
		if !a.wake && a.msg.Kind == instance.KindStart && a.msg.Start != nil && start == nil {
			start = a
			continue
		}
		b.out.rest = append(b.out.rest, a)
	}
	if start == nil {
		// Nothing will ever take a message or a wake for an instance that
		// does not exist.
		b.out.rest = nil
		for _, a := range waiting {
			b.refuse(a, "there is no such run")
		}
		return &b.out, nil
	}
	s, above := start.msg.Start, instance.Above{}
	switch {
	case !start.good(p.now):
		b.refuse(start, "the start came too late: it was good until %s, and the run was never taken up", start.until.Format(time.RFC3339))
		return &b.out, nil
	case start.kind == identity.Client && (start.sender != store.InstanceClient(inst) || s.Above != nil):
		b.refuse(start, "a run's name begins with the name of the client that starts it")
		return &b.out, nil
	case start.kind == identity.Controller && s.Above != nil:
		above = *s.Above
	case start.kind != identity.Client:
		b.drop(start)
		return &b.out, nil
	}
	b.time = start.sent
	b.step.Time = b.time
	st := b.st
	st.Status, st.Started, st.Definition, st.Input, st.Labels, st.Seed, st.Above = instance.Running, b.time, s.Definition, s.Input, s.Labels, s.Seed, above

	// The controller checks the definition again with the same code as the
	// client: it does not take a client's word.
	def, err := machine.Load(p.ctx, p.store, s.Definition)
	var problems machine.Problems
	needsAsk := false
	switch {
	case err == nil:
		problems, needsAsk = machine.Check(def)
	case errorsIs(err, machine.ErrDefinition):
		problems = machine.Problems{{Text: err.Error()}}
		def = &machine.Definition{}
	default:
		return nil, err
	}
	b.def = def
	st.Machine, st.Type = def.Name, def.Type
	if len(problems) == 0 && needsAsk && def.Ask == nil && above.Ask == nil {
		problems = machine.Problems{{Text: "a job is marked for approval, or a permission is listed to be asked about, and no ask applies: neither this definition nor any machine above it has one"}}
	}
	if len(problems) > 0 && start.kind == identity.Client {
		// Refused before any instance exists.
		b.out.dropped = append(b.out.dropped, dropped{a: start, receipt: &instance.Receipt{
			Refused: "the definition did not pass its check", Problems: problems}})
		return &b.out, nil
	}
	// The input the machine is started with has its defaults filled in.
	var inputErr error
	if len(problems) == 0 {
		st.Input, inputErr = machine.Input(def, s.Input)
	}
	b.use(start)
	b.record(machine.Event{Name: machine.Start, Data: st.Input}, start, false)
	if start.kind == identity.Client {
		b.ask(instance.Ask{Kind: instance.AskReceipt, Sender: start.sender, Message: start.name, Receipt: &instance.Receipt{Taken: true}})
		st.Above.MaxDepth = machine.DefaultDepth
		if def.Depth != nil {
			st.Above.MaxDepth = *def.Depth
		}
		if s.Allow != nil {
			layer := *s.Allow
			layer.Name = "the list the run was started with"
			layer.Fix = "start the run with %s in its list as well (corbie run submit --allow)"
			st.Above.Layers = []permission.Layer{layer}
		}
		st.Above.Secrets = s.Secrets
	}
	st.Above.AskAbout = append(slices.Clone(st.Above.AskAbout), def.AskAbout...)
	if len(problems) > 0 {
		b.end(instance.Failed, nil, b.p.failf("the definition did not pass its check: %s", strings.ReplaceAll(problems.Error(), "\n", "; ")))
		return b.done()
	}
	if inputErr != nil {
		b.end(instance.Failed, nil, b.p.failf("%v", inputErr))
		return b.done()
	}
	// Its older history is trimmed as it runs, a keep at a time.
	b.ask(instance.Ask{Kind: instance.AskWake, At: b.time.Add(p.keepFor(def, &st.Above)), What: wakeTrim})
	limit, err := machine.NewEvaluator(b.time, b.seed("timeout")).Duration(def.Timeout, machine.Scope{"input": st.Input}, "limits.timeout")
	if err != nil {
		b.end(instance.Failed, nil, b.p.failf("%v", err))
		return b.done()
	}
	if s.Timeout > 0 && (limit == 0 || s.Timeout < limit) {
		limit = s.Timeout
	}
	if limit > 0 {
		st.TimeoutAt = b.time.Add(time.Duration(limit))
		b.ask(instance.Ask{Kind: instance.AskWake, At: st.TimeoutAt, What: wakeTimeout})
	}
	b.events = []machine.Event{{Name: machine.Start, Data: st.Input}}
	b.sources = []*arrival{{wake: true}}
	if err := b.machine(); err != nil {
		return nil, err
	}
	return b.done()
}

// holds says whether a job waits for a yes before it starts: it is marked
// for approval, or it declares a permission that its machine, or a machine
// above it, lists as one to be asked about. Whether it is held depends on
// what it declares alone, before anything is sent.
func (b *build) holds(job *instance.Going) bool {
	return job.Approve || permission.Declares(job.Allow, b.st.Above.AskAbout)
}

// startAsk starts the ask once for the jobs this step held. The ask is
// the machine's own, or the nearest one above it; its values are filled
// from the input of the machine that declares it.
func (b *build) startAsk() error {
	held := b.held
	b.held = nil
	deny := func(format string, args ...any) {
		for _, job := range held {
			job.Held = false
			b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"),
				Error: &machine.Error{Status: machine.StatusDenied, Error: fmt.Sprintf(format, args...)}})
		}
	}
	spec, input := b.def.Ask, b.st.Input
	if spec == nil && b.st.Above.Ask != nil {
		above, err := b.p.definitionAt(b.st.Above.Ask.Definition)
		if err != nil {
			return err
		}
		spec, input = above.Ask, b.st.Above.Ask.Input
	}
	if spec == nil {
		deny("the job needs approval and no ask applies: neither this machine nor any above it has one")
		return nil
	}
	eval := machine.NewEvaluator(b.time, b.seed("ask", fmt.Sprint(b.step.Number)))
	filled, err := spec.Fill(eval, machine.Scope{"input": input}, instance.AskName, instance.AskName)
	if err != nil {
		deny("the ask could not be started: %v", err)
		return nil
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
	if b.def.Ask != nil && !filled.IsMachine() {
		// A script of the definition has the lists written for the whole of it.
		allow, err := b.definitionAllow()
		if err != nil {
			deny("the ask could not be started: %v", err)
			return nil
		}
		filled.Allow = append(allow, filled.Allow...)
		filled.Deny = append(slices.Clone(b.def.Deny), filled.Deny...)
	}
	told["asking"] = map[string]any{"machine": b.st.Machine, "instance": b.inst}
	told["jobs"] = jobs
	filled.Input = told
	b.st.Asked[instance.AskName]++
	ask := &instance.Going{Job: *filled, Asked: b.st.Asked[instance.AskName], Attempt: 1, ForAsk: names}
	ask.Attempts = max(ask.Attempts, 1)
	b.st.Jobs[ask.Key()] = ask
	if ask.IsMachine() && b.st.Above.Depth+1 > b.st.Above.MaxDepth {
		b.fail(ask, "depth limit reached: a root may have %d levels of machines below it", b.st.Above.MaxDepth)
		return nil
	}
	b.launch(ask)
	return nil
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
