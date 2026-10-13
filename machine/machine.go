// Package machine is what every machine type shares: the definition, values
// filled in from input, the events a machine receives, the requests it
// makes, and the contract a type keeps with the controller.
package machine

import (
	"encoding/json/jsontext"
	"fmt"
	"sort"
	"strings"
	"time"
)

// An Event is anything that reaches a machine: a name, where it came from,
// and its data.
type Event struct {
	Name string `json:"name"`
	From string `json:"from,omitzero"`
	Data any    `json:"data,omitzero"`
}

// The names of the events Corbie itself sends a machine.
const (
	Start  = "start"
	Done   = "done"
	Failed = "failed"
	Timer  = "timer"
)

// The ways a job fails.
const (
	StatusError     = "error"
	StatusTimeout   = "timeout"
	StatusDenied    = "denied"
	StatusCancelled = "cancelled"
)

// An Error is how a job or a machine failed. It is the data of a failed
// event, with whatever output the job still gave.
type Error struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Output any    `json:"output,omitzero"`
	// A denial says what was refused, which layer refused it and what
	// would fix it, and passes up unchanged.
	Refused string `json:"refused,omitzero"`
	Layer   string `json:"layer,omitzero"`
	Fix     string `json:"fix,omitzero"`
}

// Unhandled gives the failure of a machine that had no rule for the
// failure of one of its jobs: its error names the job and then gives the
// job's own. A denial passes through unchanged, with what was refused,
// which layer refused it and what would fix it; anything else that failed
// a job fails its machine with an error.
func Unhandled(job string, data any) *Error {
	failed, _ := data.(map[string]any)
	e := &Error{Status: StatusError, Error: fmt.Sprintf("%s: %v", job, failed["error"])}
	if failed["status"] == StatusDenied {
		e.Status = StatusDenied
		e.Refused, _ = failed["refused"].(string)
		e.Layer, _ = failed["layer"].(string)
		e.Fix, _ = failed["fix"].(string)
	}
	return e
}

// Value is the error as the data of a failed event.
func (e *Error) Value() any {
	v := map[string]any{"status": e.Status, "error": e.Error}
	if e.Output != nil {
		v["output"] = e.Output
	}
	if e.Refused != "" {
		v["refused"], v["layer"], v["fix"] = e.Refused, e.Layer, e.Fix
	}
	return v
}

// A Job is a filled-in request for work: a script, which a runner
// executes, or a machine, which the controller starts as a child.
type Job struct {
	// Name says what asked for the job: a stage, a state, a tool, an
	// agent's turn, the ask; an item of a list adds its position.
	Name string `json:"name"`

	Script  string   `json:"script,omitzero"`  // a script of the definition, by its path
	Files   []string `json:"files,omitzero"`   // the files it loads, by their paths
	Machine string   `json:"machine,omitzero"` // a machine of the definition, by its name
	// Stored is a stored definition or script the job's own expression gave.
	Stored any `json:"stored,omitzero"`

	Input    any      `json:"input,omitzero"`
	Allow    []string `json:"allow,omitzero"`
	Deny     []string `json:"deny,omitzero"`
	Timeout  Duration `json:"timeout,omitzero"`
	Attempts int      `json:"attempts,omitzero"`
	Memory   int64    `json:"memory,omitzero"`
	Tags     []string `json:"tags,omitzero"`
	Approve  bool     `json:"approve,omitzero"`
	// Alone says the job has only its own permissions, and none of the
	// lists written for the whole definition.
	Alone bool `json:"alone,omitzero"`
	// Shown is what the ask is shown about the job, beside its name and
	// its input, where the job is held for approval.
	Shown any `json:"shown,omitzero"`
	// Wants names the emitted values the machine acts on.
	Wants []string `json:"wants,omitzero"`
}

// IsMachine says whether the job is a machine, started as a child.
func (r *Job) IsMachine() bool { return r.Machine != "" || isDefinition(r.Stored) }

// A Send is a request to send a named event to the parent or to a child.
type Send struct {
	To    string `json:"to"` // "parent", or the name of the job that started the child
	Event string `json:"event"`
	Data  any    `json:"data,omitzero"`
}

// Parent is where a send to the machine's parent is addressed.
const Parent = "parent"

// A Request is one thing a step asks for. Exactly one field is set.
type Request struct {
	Job    *Job      `json:"job,omitzero"`
	Cancel string    `json:"cancel,omitzero"` // cancel the job of this name
	Send   *Send     `json:"send,omitzero"`
	Timer  *TimerAt  `json:"timer,omitzero"`
	Clear  string    `json:"clear,omitzero"` // cancel the timer of this name
	Key    *KeyNamed `json:"key,omitzero"`
	Drop   string    `json:"drop,omitzero"` // the key of this id is no longer waited on
	End    *Ending   `json:"end,omitzero"`
	Note   string    `json:"note,omitzero"`
}

// TimerAt asks for a timer event at a time.
type TimerAt struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

// KeyNamed asks for a key to be waited on. ID tells apart keys that wait
// under one name; the answer arrives as an event with the key's name, from
// the id.
type KeyNamed struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// An Ending is a machine reaching an end of its own.
type Ending struct {
	Output any    `json:"output,omitzero"`
	Error  *Error `json:"error,omitzero"`
}

// StepIn is what a step is given. A step's answer depends on nothing else.
type StepIn struct {
	Definition *Definition
	State      jsontext.Value // the state the machine was left in; nil at the start
	Events     []Event
	Input      any       // what the machine was started with
	Time       time.Time // the time of the step
	Seed       []byte    // the step's own secret, for keys and random numbers
	// HasAsk says an ask applies to the machine: its own, or one above it.
	HasAsk bool
}

// StepOut is what a step gives back: the new state and what it asks for.
type StepOut struct {
	State    any
	Requests []Request
	// Ignored lists the events, by their place in StepIn.Events, that the
	// machine had no rule for.
	Ignored []int
}

// A Type is a kind of machine. It supplies the filling-out of its short
// forms, the check of its definitions, its step, what a watcher is shown of
// its state, and its page of the manual.
type Type interface {
	Name() string
	// Shape is what a definition of the type may hold of the type's own:
	// its fields, and theirs. It is what the type's part is read by.
	Shape() *Shape
	// Fill reads the type's part of a written definition, by its shape,
	// and gives its full form, reporting what is wrong with it to the
	// parser.
	Fill(p *Parser, f *Fields) any
	// Check reports what is wrong with a definition in full form.
	Check(c *Checker, def *Definition)
	Step(in *StepIn) (*StepOut, error)
	// Show gives what a watcher is shown of a state.
	Show(def *Definition, state jsontext.Value) any
	Manual() string
}

var types = map[string]Type{}

// Register adds a machine type.
func Register(t Type) { types[t.Name()] = t }

// TypeOf gives the machine type of a name.
func TypeOf(name string) (Type, bool) {
	t, ok := types[name]
	return t, ok
}

// Types lists the names of the machine types.
func Types() []string {
	var names []string
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// A Duration is a length of time as definitions write it: 90s, 5m, 48h, 7d.
type Duration time.Duration

// ParseDuration reads a length of time.
func ParseDuration(text string) (Duration, error) {
	if days, ok := strings.CutSuffix(text, "d"); ok {
		var n float64
		if _, err := fmt.Sscanf(days, "%g", &n); err == nil && !strings.ContainsAny(days, "hmsu ") {
			return Duration(n * 24 * float64(time.Hour)), nil
		}
	}
	d, err := time.ParseDuration(text)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%q is not a length of time (write it like 30s, 5m, 48h or 7d)", text)
	}
	return Duration(d), nil
}

func (d Duration) String() string { return time.Duration(d).String() }

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := ParseDuration(string(text))
	*d = parsed
	return err
}

// JobName adds to a job's name what tells it apart from others asked for
// under the same name: the times that name has asked before, and the
// attempt.
func JobName(name string, asked, attempt int) string {
	if asked > 1 {
		name = fmt.Sprintf("%s#%d", name, asked)
	}
	if attempt > 1 {
		name = fmt.Sprintf("%s@%d", name, attempt)
	}
	return name
}
