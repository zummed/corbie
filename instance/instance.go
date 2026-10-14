// Package instance is the two things an instance is in the store: its head,
// which says where it stands, and its record, which is everything that
// happened to it; and the messages and receipts that pass between a client
// and the controller about it.
package instance

import (
	"context"
	"encoding/json/jsontext"
	"fmt"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// The words for where an instance stands. A run that has been sent and not
// yet taken up is waiting; it has no head.
const (
	Waiting   = "waiting"
	Running   = "running"
	Done      = "done"
	Failed    = "failed"
	Cancelled = "cancelled"
)

// A Head is the small public summary of an instance, signed by the
// controller and sealed to nobody. Apart from its labels it holds nothing
// the instance was given or produced.
type Head struct {
	Name    string `json:"name"`    // the instance
	Machine string `json:"machine"` // what the definition is called
	Type    string `json:"type"`
	Status  string `json:"status"`
	// Failure says how a failed instance failed: error, timeout or denied.
	Failure string            `json:"failure,omitzero"`
	Step    int               `json:"step"`
	Started time.Time         `json:"started"`
	Ended   time.Time         `json:"ended,omitzero"`
	Parent  string            `json:"parent,omitzero"`
	Jobs    int               `json:"jobs,omitzero"`
	Timers  int               `json:"timers,omitzero"`
	Tickets []string          `json:"tickets,omitzero"`
	Labels  map[string]string `json:"labels,omitzero"`
	// Keep is how long the instance is kept after it ends.
	Keep machine.Duration `json:"keep,omitzero"`
}

// Ended says whether the instance has ended.
func (h *Head) HasEnded() bool { return h.Status != Running }

// A Step is one step entry of a record: the events that arrived and what
// they came from, the state the instance was left in, and what the step
// asked for. The latest step is the instance's whole state.
type Step struct {
	Number int       `json:"number"`
	Time   time.Time `json:"time"`
	// Events are what arrived, in the order they were taken.
	Events []Arrived `json:"events,omitzero"`
	// Used names every message and wake the step used. They are deleted
	// only when the step has been carried out.
	Used  []string `json:"used,omitzero"`
	State State    `json:"state"`
	// Asks are what the step asked for, each under a name the entry fixes.
	Asks []Ask `json:"asks,omitzero"`
}

// Arrived is an event with the message or wake it came from. Machine says
// whether it was given to the machine or was the controller's own to act
// on.
type Arrived struct {
	machine.Event
	Source string `json:"source,omitzero"`
	Own    bool   `json:"own,omitzero"`
}

// State is everything about an instance that is not its history.
type State struct {
	Machine string         `json:"machine"`
	Type    string         `json:"type"`
	Status  string         `json:"status"`
	Output  any            `json:"output,omitzero"`
	Error   *machine.Error `json:"error,omitzero"`
	Started time.Time      `json:"started"`
	Ended   time.Time      `json:"ended,omitzero"`

	Definition blob.Ref          `json:"definition"`
	Input      any               `json:"input,omitzero"`
	Labels     map[string]string `json:"labels,omitzero"`
	// Seed is the instance's own secret: its keys and random numbers follow
	// from it.
	Seed []byte `json:"seed"`

	// Above is what the instance was started under.
	Above Above `json:"above"`

	// TimeoutAt is when the instance passes its timeout, if it has one.
	TimeoutAt time.Time `json:"timeout_at,omitzero"`
	// Inner is the machine type's own state.
	Inner jsontext.Value `json:"inner,omitzero"`

	// Jobs are the jobs still going, by the name that asked for them.
	Jobs map[string]*Going `json:"jobs,omitzero"`
	// Timers are the timers set, by name.
	Timers map[string]Timer `json:"timers,omitzero"`
	// Tickets are the tickets waited on, by the id the machine gave each.
	Tickets map[string]Ticket `json:"tickets,omitzero"`
	// Asked counts how many times each name has asked for a job.
	Asked map[string]int `json:"asked,omitzero"`
	// JobsStarted counts the jobs the instance has started.
	JobsStarted int `json:"jobs_started,omitzero"`
	// Taken is the name of the last message taken from the instance's
	// parent. The controller writes those under names that sort in the
	// order sent, and may write one again when it repeats a carry-out: one
	// that does not sort after the last taken has been taken already.
	Taken string `json:"taken,omitzero"`
	// Emitted counts, for each job and name, the values given to the
	// machine, so a later attempt's are not given twice.
	Emitted map[string]int `json:"emitted,omitzero"`
	// Sent counts the events the instance has sent up to its parent.
	Sent int `json:"sent,omitzero"`
	// Used lists the permissions used by every job in the instance and
	// below it.
	Used []string `json:"used,omitzero"`
	// Answered remembers, by a short mark, the last few tickets answered, so a
	// second answer can be told it came second.
	Answered []string `json:"answered,omitzero"`
}

// Above is what an instance was started under: who started it and the
// limits that came with the start.
type Above struct {
	Parent string `json:"parent,omitzero"` // the instance that started it
	Job    string `json:"job,omitzero"`    // the job of the parent's that it is
	Depth  int    `json:"depth,omitzero"`  // how many machines are above it
	// MaxDepth is how many levels the root allows below itself.
	MaxDepth int `json:"max_depth,omitzero"`
	// Layers are the permissions that narrow it: the start's own list, and
	// the job of each parent that led to it.
	Layers []permission.Layer `json:"layers,omitzero"`
	// AskAbout are the lists, from the machines above, of permissions that
	// must be asked about.
	AskAbout []string `json:"ask_about,omitzero"`
	// Ask is the nearest ask above, for a machine that has none of its own.
	Ask *AskAbove `json:"ask,omitzero"`
	// IsAsk says the instance is itself run as an ask.
	IsAsk bool `json:"is_ask,omitzero"`
	// Secrets are the run's own secrets, sent with the root's start.
	Secrets map[string]string `json:"secrets,omitzero"`
	// Keep is the longest the instance may be kept after it ends: no
	// longer than what started it, so a child's record goes no later than
	// its parent's.
	Keep machine.Duration `json:"keep,omitzero"`
}

// AskAbove is an ask a machine takes from above: the definition that
// declares it and the input its values are filled from.
type AskAbove struct {
	Definition blob.Ref           `json:"definition"`
	Input      any                `json:"input,omitzero"`
	Layers     []permission.Layer `json:"layers,omitzero"`
}

// A Going is a job still going.
type Going struct {
	machine.Job
	Asked   int `json:"asked"`   // which time its name asked for a job
	Attempt int `json:"attempt"` // which attempt this is
	// Runner is the runner a job was sent to.
	Runner string `json:"runner,omitzero"`
	// Child is the instance a machine was started as.
	Child string `json:"child,omitzero"`
	// Due is when a job's result is due.
	Due time.Time `json:"due,omitzero"`
	// Again is when the next attempt is due, after one has failed.
	Again time.Time `json:"again,omitzero"`
	// Wake is what the job's wake, for its due time or its next attempt,
	// is called.
	Wake string `json:"wake,omitzero"`
	// Held says the job waits for the ask before it starts.
	Held bool `json:"held,omitzero"`
	// ForAsk lists the held jobs this job of the ask is deciding.
	ForAsk []string `json:"for_ask,omitzero"`
}

// AskName is the name of the job that gives the yes when a job needs
// approval. Nothing else in a definition may take it.
const AskName = "ask"

// Key is the name a job is kept under among those still going: its own
// name, of which one is going at a time; or for a job of the ask, of which
// several may be, its name with which time it was asked for.
func (r *Going) Key() string {
	if r.Name == AskName {
		return machine.JobName(r.Name, r.Asked, 1)
	}
	return r.Name
}

// Full is the job's name with what tells it apart from the others its name
// asked for.
func (r *Going) Full() string { return machine.JobName(r.Name, r.Asked, r.Attempt) }

// A Timer is a timer set: when it is due, and what its wake is called. A
// wake's name carries the step that set it, so no two steps ever name one
// wake.
type Timer struct {
	At   time.Time `json:"at"`
	Wake string    `json:"wake"`
}

// A Ticket is a ticket waited on: its name and its value. The value is the
// whole right to answer.
type Ticket struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// The kinds of thing a step asks for.
const (
	AskScript  = "script"  // send a script to a runner
	AskChild   = "child"   // start a child machine
	AskCancel  = "cancel"  // cancel a job
	AskEvent   = "event"   // send an event to the parent or a child
	AskWake    = "wake"    // be woken at a time
	AskReceipt = "receipt" // answer a client's message
	AskEnd     = "end"     // tell the parent the instance has ended
	AskFail    = "fail"    // a job failed before anything was sent
	AskHold    = "hold"    // a job is held for the ask
	AskNote    = "note"    // something to be seen in the record
)

// An Ask is one thing a step asked for.
type Ask struct {
	Kind string `json:"kind"`
	// Name is the job a script, a child, a cancel or a failure is about.
	Name string       `json:"name,omitzero"`
	Job  *machine.Job `json:"job,omitzero"`
	// Attempt is which attempt at the job this is.
	Attempt int    `json:"attempt,omitzero"`
	Runner  string `json:"runner,omitzero"`
	// Card is the card of the runner a job goes to: what that job adds to
	// the record is checked against it.
	Card  identity.Card `json:"card,omitzero"`
	Child string        `json:"child,omitzero"`
	// To is the instance an event is sent to; none for an event a root
	// sends up, which is only written here.
	To    string         `json:"to,omitzero"`
	Event *machine.Event `json:"event,omitzero"`
	// Undelivered says the event was for a child that is not running.
	Undelivered bool `json:"undelivered,omitzero"`
	// At is a wake's time, or a job's due time.
	At   time.Time `json:"at,omitzero"`
	What string    `json:"what,omitzero"` // what a wake is for
	// Message is the name of the message, in an inbox, that carrying this
	// out writes.
	Message string         `json:"message,omitzero"`
	Sender  string         `json:"sender,omitzero"` // whose message a receipt answers
	Receipt *Receipt       `json:"receipt,omitzero"`
	Error   *machine.Error `json:"error,omitzero"`
	Output  any            `json:"output,omitzero"`
	Text    string         `json:"text,omitzero"`
}

// A Receipt is the controller's answer to one message of a client's: that
// it was taken, or why it was refused.
type Receipt struct {
	Taken   bool   `json:"taken"`
	Refused string `json:"refused,omitzero"`
	// Problems are what is wrong with a definition that was refused.
	Problems machine.Problems `json:"problems,omitzero"`
	// Acted says, for an event, whether the machine acted on it or had no
	// rule for it.
	Acted *bool `json:"acted,omitzero"`
}

// ReceiptLife is how long a receipt is kept.
const ReceiptLife = 24 * time.Hour

// An Emit is one emit entry of a record: values a job emitted, in the
// order it made them, written by the job's runner.
type Emit struct {
	Job    string    `json:"job"`    // the job that emitted
	Number int       `json:"number"` // which of the job's emit entries it is, counted from one
	Values []Emitted `json:"values"`
}

// Emitted is one value a job emitted while it ran: a log line, a note of
// progress, a named value.
type Emitted struct {
	Name string    `json:"name"`
	Data any       `json:"data,omitzero"`
	At   time.Time `json:"at"`
}

// The names of the emits Corbie itself gives a script.
const (
	EmitLog      = "log"
	EmitProgress = "progress"
)

// ReadHead reads an instance's head, wherever it is. An instance whose head
// is among the finished has ended, also when a head is still among the
// running. A head that moves between the two looks is found.
func ReadHead(ctx context.Context, s store.Store, ctl identity.Card, name string) (*Head, store.Mark, error) {
	finished := store.FinishedHead(ctl.Name, name)
	for _, at := range []string{finished, store.RunningHead(ctl.Name, name), finished} {
		data, mark, err := s.Read(ctx, at)
		if err != nil {
			return nil, store.None, err
		}
		if mark == store.None {
			continue
		}
		var head Head
		if _, err := identity.Read(data, at, ctl, nil, &head); err != nil {
			return nil, store.None, fmt.Errorf("the head of %s: %w", name, err)
		}
		if head.Name != name {
			return nil, store.None, fmt.Errorf("the head of %s: %w", name, identity.ErrAltered)
		}
		return &head, mark, nil
	}
	return nil, store.None, nil
}

// ReadStep reads one step entry of a record. It gives nil where there is
// none.
func ReadStep(ctx context.Context, s store.Store, ctl identity.Card, id *identity.Identity, name string, number int) (*Step, error) {
	at := store.Step(ctl.Name, name, number)
	data, mark, err := s.Read(ctx, at)
	if err != nil || mark == store.None {
		return nil, err
	}
	var step Step
	if _, err := identity.Read(data, at, ctl, id, &step); err != nil {
		return nil, fmt.Errorf("step %d of %s: %w", number, name, err)
	}
	if step.Number != number {
		return nil, fmt.Errorf("step %d of %s: %w", number, name, identity.ErrAltered)
	}
	return &step, nil
}

// Latest finds an instance's latest step by reading on from a number to the
// last one there. It gives nil where the instance has no step.
func Latest(ctx context.Context, s store.Store, ctl identity.Card, id *identity.Identity, name string, from int) (*Step, error) {
	var latest *Step
	for number := max(from, 1); ; number++ {
		step, err := ReadStep(ctx, s, ctl, id, name, number)
		if err != nil {
			return nil, err
		}
		if step == nil {
			return latest, nil
		}
		latest = step
	}
}

// Current gives an instance's latest step, found from its head. Older steps
// may have been trimmed, so reading always starts from the number the head
// gives. It gives nil where the instance has no head.
func Current(ctx context.Context, s store.Store, ctl identity.Card, id *identity.Identity, name string) (*Step, error) {
	head, _, err := ReadHead(ctx, s, ctl, name)
	if err != nil || head == nil {
		// A first step may be there with no head yet.
		if err == nil {
			return Latest(ctx, s, ctl, id, name, 1)
		}
		return nil, err
	}
	return Latest(ctx, s, ctl, id, name, head.Step)
}
