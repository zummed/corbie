package instance

import (
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
)

// The kinds of message.
const (
	KindStart  = "start"  // start a run
	KindCancel = "cancel" // cancel a run; to a runner, cancel a job
	KindEvent  = "event"  // send a run an event
	KindAnswer = "answer" // answer a ticket
	KindResult = "result" // a job's outcome: a script's from its runner, a child's from the controller
	KindEmit   = "emit"   // a value a job sent up that the machine wants
	KindJob    = "job"    // a job for a runner
)

// A Message is what one party leaves in another's inbox. Kind says which
// of the parts it carries.
type Message struct {
	Kind string `json:"kind"`
	// Instance is the instance the message is for.
	Instance string `json:"instance,omitzero"`
	// From is, in a message the controller sends itself, the instance it
	// is sent on behalf of.
	From string `json:"from,omitzero"`

	Start  *Start         `json:"start,omitzero"`
	Event  *machine.Event `json:"event,omitzero"`
	Answer *Answer        `json:"answer,omitzero"`
	Result *Result        `json:"result,omitzero"`
	Emit   *EmitNote      `json:"emit,omitzero"`
	Job    *Job           `json:"job,omitzero"`
	// Cancel names, for a runner, the job that is cancelled.
	Cancel string `json:"cancel,omitzero"`
}

// MessageLife is how long a message is good for when its sender says
// nothing.
const MessageLife = 5 * time.Minute

// A Start starts a run.
type Start struct {
	Definition blob.Ref          `json:"definition"`
	Input      any               `json:"input,omitzero"`
	Labels     map[string]string `json:"labels,omitzero"`
	// Allow is the start's own list, which narrows the root and everything
	// below it.
	Allow *permission.Layer `json:"allow,omitzero"`
	// Timeout is how long the run may take, if the start says.
	Timeout machine.Duration `json:"timeout,omitzero"`
	// Secrets are the run's own secrets, for it and everything below it.
	Secrets map[string]string `json:"secrets,omitzero"`
	// Seed is the secret the instance's tickets and random numbers follow
	// from.
	Seed []byte `json:"seed"`
	// Above is set by the controller when it starts a child.
	Above *Above `json:"above,omitzero"`
}

// An Answer answers a ticket: the ticket's value and the answer.
type Answer struct {
	Ticket string `json:"ticket"`
	Value  any    `json:"value,omitzero"`
}

// A Result is how a job ended.
type Result struct {
	Job     string `json:"job"` // the job, as its instance's step named it
	Attempt int    `json:"attempt,omitzero"`
	// Status is done, or one of the ways a job fails.
	Status string         `json:"status"`
	Output any            `json:"output,omitzero"`
	Error  *machine.Error `json:"error,omitzero"`
	// Used lists the permissions a job used.
	Used []string `json:"used,omitzero"`
	// Final says a failure is not to be tried again.
	Final bool `json:"final,omitzero"`
}

// An EmitNote tells the controller of a value a job sent up that the
// machine wants to act on. Index is which value of that name it is from
// that job, counting from one.
type EmitNote struct {
	Job   string `json:"job"`
	Name  string `json:"name"`
	Index int    `json:"index"`
	Data  any    `json:"data,omitzero"`
}

// Kept is the key of one kept path, which opens the value kept there and
// every value kept below it.
type Kept struct {
	Path string `json:"path,omitzero"` // the top is the empty path
	Key  []byte `json:"key"`
}

// A Job is everything a runner needs to run a script: with the job and the
// store, and nothing else from the controller, the runner can run it.
type Job struct {
	Instance string `json:"instance"`
	Name     string `json:"name"` // the job's full name, its attempt included
	Attempt  int    `json:"attempt"`
	// Request names what asked: every attempt at the same job has the same.
	Request string `json:"request"`

	Script blob.Ref `json:"script"`
	// Path is the script's path as the definition names it, and none for
	// a stored script. Files are keyed the same way.
	Path  string              `json:"path,omitzero"`
	Files map[string]blob.Ref `json:"files,omitzero"`
	Input any                 `json:"input,omitzero"`

	// Layers are every layer of permissions that applies, each by name.
	Layers []permission.Layer `json:"layers"`
	// Secrets are the values of the secrets the job names.
	Secrets map[string]string `json:"secrets,omitzero"`
	// Kept is the key to the kept values the job may use, where it may use
	// any. The runner holds it, and never gives it to the job's process.
	Kept *Kept `json:"kept,omitzero"`

	Timeout machine.Duration `json:"timeout"`
	Memory  int64            `json:"memory,omitzero"`
	// Due is when the job's result is due.
	Due time.Time `json:"due"`
	// Wants names the emitted values the machine wants to hear about.
	Wants []string `json:"wants,omitzero"`
	// Client is the card of the client that started the root: what the job
	// adds to the record is sealed to it and to the controller.
	Client identity.Card `json:"client"`
}
