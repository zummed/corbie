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
	KindSubmit = "submit" // record a tree at a prefix
	KindStart  = "start"  // start a run
	KindCancel = "cancel" // cancel a run; to a runner, cancel a job
	KindEvent  = "event"  // send a run an event
	KindAnswer = "answer" // answer a ticket
	KindResult = "result" // a job's outcome: a script's from its runner, a child's from the controller
	KindEmit   = "emit"   // a value a job sent up that the machine wants
	KindJob    = "job"    // a job for a runner
	KindSend   = "send"   // an event sent to an address, with what starts a run for it if there is one
	KindNote   = "note"   // something to be written in a run's record
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
	// To is the address an event is sent to or a call starts a run at, and
	// Sender the run or client it is from. Only the controller writes
	// Sender.
	To     *machine.Address `json:"to,omitzero"`
	Sender *machine.Address `json:"sender,omitzero"`
	// Cause is, in a message the controller sends itself, what caused it:
	// the step that asked for it, or the client's message it carries on.
	Cause *Cause `json:"cause,omitzero"`

	Start  *Start         `json:"start,omitzero"`
	Submit *Submit        `json:"submit,omitzero"`
	Event  *machine.Event `json:"event,omitzero"`
	Answer *Answer        `json:"answer,omitzero"`
	Result *Result        `json:"result,omitzero"`
	Emit   *EmitNote      `json:"emit,omitzero"`
	Job    *Job           `json:"job,omitzero"`
	Note   string         `json:"note,omitzero"`
	// Cancel names, for a runner, the job that is cancelled.
	Cancel string `json:"cancel,omitzero"`
}

// MessageLife is how long a message is good for when its sender says
// nothing.
const MessageLife = 5 * time.Minute

// A Start starts a run. A client's start names a location, and its input
// is the blob InputBlob in the run's record; Definition and Input are
// written by the controller, from the file recorded there and that blob, or
// from the job that starts a child.
type Start struct {
	Definition blob.Ref          `json:"definition,omitzero"`
	Input      any               `json:"input,omitzero"`
	InputBlob  blob.Ref          `json:"input_blob,omitzero"`
	Labels     map[string]string `json:"labels,omitzero"`
	// Allow is the start's own list, which narrows the root and everything
	// below it.
	Allow *permission.Layer `json:"allow,omitzero"`
	// Timeout is how long the run may take, if the start says.
	Timeout machine.Duration `json:"timeout,omitzero"`
	// Seed is the secret the instance's tickets and random numbers follow
	// from.
	Seed []byte `json:"seed"`
	// Above is set by the controller when it starts a child.
	Above *Above `json:"above,omitzero"`
	// Location is the location the run is started at: its machine is the
	// file recorded there.
	Location string `json:"location,omitzero"`
	// Key is, for an event sent to a key, the key of the run it starts.
	Key string `json:"key,omitzero"`
	// Tree is the prefix of the tree the location is in, for a child, which
	// runs at its parent's location.
	Tree string `json:"tree,omitzero"`
	// Replaces is, for a singleton a submit starts again, the runs it
	// replaces: the new run starts once every one has ended.
	Replaces []string `json:"replaces,omitzero"`
	// Run is the run an event sent to a key starts, where it names one: the
	// event's data is its input. What a client or a job starts is the
	// message's own instance.
	Run string `json:"run,omitzero"`
}

// A Submit records a directory as a tree at a prefix. Files are its
// machines, by location, each uploaded with the message; Problems are the
// locations of files that could not be read, which keep what was recorded
// before.
type Submit struct {
	Prefix    string                `json:"prefix"`
	Directory string                `json:"directory"`
	Files     map[string]SubmitFile `json:"files,omitzero"`
	Problems  []string              `json:"problems,omitzero"`
}

// A SubmitFile is one machine of a submit: its stored definition among the
// message's uploads, the hash of what it runs, and the names of the secrets
// and the values it uses.
type SubmitFile struct {
	Definition blob.Ref `json:"definition"`
	Hash       string   `json:"hash"`
	Secrets    []string `json:"secrets,omitzero"`
	Values     []string `json:"values,omitzero"`
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

	Timeout machine.Duration `json:"timeout"`
	Memory  int64            `json:"memory,omitzero"`
	// Due is when the job's result is due.
	Due time.Time `json:"due"`
	// Wants names the emitted values the machine wants to hear about.
	Wants []string `json:"wants,omitzero"`
	// Client is the card of the client that owns the root's tree: what the
	// job adds to the record is sealed to it and to the controller.
	Client identity.Card `json:"client"`
}
