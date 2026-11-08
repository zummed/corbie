// Package machine is what every machine type shares: the definition, values
// filled in from input, the events a machine receives, the requests it
// makes, and the contract a type keeps with the controller.
package machine

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/store"
)

// An Event is anything that reaches a machine: a name, where it came from,
// and its data.
type Event struct {
	Name string `json:"name"`
	From string `json:"from,omitzero"`
	Data any    `json:"data,omitzero"`
	// Sender is the run or the client that sent the event to the run by
	// its address. Only the controller sets it.
	Sender *Address `json:"sender,omitzero"`
	// Answer says the event is the answer to a ticket. Only the controller
	// sets it, when it takes an answer.
	Answer bool `json:"answer,omitzero"`
}

// The names of the events Corbie itself sends a machine.
const (
	Start  = "start"
	Done   = "done"
	Failed = "failed"
	Timer  = "timer"
)

// Reserved says whether a name is one Corbie gives a machine itself, which no
// client, machine or script can send.
func Reserved(name string) bool {
	return name == Start || name == Done || name == Failed || name == Timer
}

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
	// Found is where the script or the machine is, worked out when the job
	// is asked for: what carries it out reads no definition.
	Found *Found `json:"found,omitzero"`

	// Send is an event the job sends to a run by its address, and Call a
	// location whose run the job starts and waits for.
	Send *Send    `json:"send,omitzero"`
	Call *Address `json:"call,omitzero"`

	Input   any      `json:"input,omitzero"`
	Allow   []string `json:"allow,omitzero"`
	Deny    []string `json:"deny,omitzero"`
	Timeout Duration `json:"timeout,omitzero"`
	Restart *Restart `json:"restart,omitzero"`
	Memory  int64    `json:"memory,omitzero"`
	Tags    []string `json:"tags,omitzero"`
	Approve bool     `json:"approve,omitzero"`
	// Alone says the job has only its own permissions, and none of the
	// lists written for the whole definition.
	Alone bool `json:"alone,omitzero"`
	// Shown is what the ask is shown about the job, beside its name and
	// its input, where the job is held for approval.
	Shown any `json:"shown,omitzero"`
	// Above says the job is an ask taken from a machine above, whose
	// script or machine is in that machine's definition. Only the controller
	// sets it.
	Above bool `json:"above,omitzero"`
	// Wants names the emitted values the machine acts on.
	Wants []string `json:"wants,omitzero"`
}

// Found is where a job's script or machine is: the blob, the files the
// script loads, and the name of the definition that holds the job.
type Found struct {
	Blob  blob.Ref            `json:"blob"`
	Files map[string]blob.Ref `json:"files,omitzero"`
	In    string              `json:"in"`
}

// IsMachine says whether the job is a machine, started as a child.
func (r *Job) IsMachine() bool { return r.Machine != "" || isDefinition(r.Stored) }

// IsSend says whether the job sends an event to a run, and IsCall whether it
// calls a location.
func (r *Job) IsSend() bool { return r.Send != nil }
func (r *Job) IsCall() bool { return r.Call != nil }

// An Address names a run: the run of a singleton location, the run for a key
// at a keyed location, or one run by its ID. As the sender of an event it may
// name a client instead. A key is kept as it was filled in, empty or past
// KeyLimit included: such a send is undelivered, and fails nothing.
type Address struct {
	At     string  `json:"at,omitzero"`
	Key    *string `json:"key,omitzero"`
	Run    string  `json:"run,omitzero"`
	Client string  `json:"client,omitzero"`
}

// Check says what is wrong with an address once it is filled in. One that
// names a run and a key, as the sender of an event from a keyed run does,
// goes to the run. Its key is checked where it is delivered.
func (a *Address) Check() error {
	switch {
	case a.Client != "" && (a.At != "" || a.Key != nil || a.Run != ""):
		return errors.New("an address names a client or a location, not both")
	case a.Client != "":
		return nil
	case a.Run != "":
		if err := store.CheckInstance(a.Run); err != nil {
			return err
		}
	}
	return CheckLocation(a.At)
}

// CheckLocation says what is wrong with a location as an address or a
// receive: entry writes it: a path under the tree's prefix, or a full path
// from /.
func CheckLocation(loc string) error { return checkLocation(loc, false) }

// CheckPattern says what is wrong with a pattern of locations, where * is
// any part of one name and ** any number of names.
func CheckPattern(pattern string) error { return checkLocation(pattern, true) }

func checkLocation(loc string, pattern bool) error {
	if loc == "" {
		return errors.New("a location is not empty")
	}
	for _, part := range strings.Split(strings.TrimPrefix(loc, "/"), "/") {
		switch {
		case part == "":
			return fmt.Errorf("%q: a location has no empty name and does not end with /", loc)
		case part == "." || part == "..":
			return fmt.Errorf("%q: a location is read from the tree's prefix, or from / in full, and has no . or .. in it", loc)
		case part == "+" || strings.ContainsAny(part, "#\\") || strings.ContainsFunc(part, unicode.IsControl):
			return fmt.Errorf("%q: a name in a location is not + and holds no #, backslash or control character", loc)
		case !pattern && strings.Contains(part, "*"):
			return fmt.Errorf("%q: a location holds no *", loc)
		}
	}
	return nil
}

// Resolve gives a location from a tree's prefix: one written from / is a
// full path, and any other is read from the prefix.
func Resolve(prefix, loc string) string {
	if strings.HasPrefix(loc, "/") {
		return loc
	}
	return strings.TrimSuffix(prefix, "/") + "/" + loc
}

// A Send is a request to send a named event to the parent, to a child, or
// to a run by its address.
type Send struct {
	To    string   `json:"to,omitzero"` // "parent", or the name of the job that started the child
	At    *Address `json:"at,omitzero"` // the run it is sent to, in place of To
	Event string   `json:"event"`
	Data  any      `json:"data,omitzero"`
}

// Parent is where a send to the machine's parent is addressed.
const Parent = "parent"

// KeyLimit is how long a key may be, in characters.
const KeyLimit = 256

// TitleLimit is how long a title may be, in characters.
const TitleLimit = 200

// CheckKey says what is wrong with a key. A key is any text of at most
// KeyLimit characters that is not empty, and no name in the store holds it.
func CheckKey(key string) error {
	switch {
	case key == "":
		return errors.New("a key is not empty")
	case !utf8.ValidString(key):
		return errors.New("a key is text, and this is not")
	case utf8.RuneCountInString(key) > KeyLimit:
		return fmt.Errorf("a key is at most %d characters, and this one starts %.40q", KeyLimit, key)
	}
	return nil
}

// A Request is one thing a step asks for. Exactly one field is set.
type Request struct {
	Job    *Job         `json:"job,omitzero"`
	Cancel string       `json:"cancel,omitzero"` // cancel the job of this name
	Send   *Send        `json:"send,omitzero"`
	Timer  *TimerAt     `json:"timer,omitzero"`
	Clear  string       `json:"clear,omitzero"` // cancel the timer of this name
	Ticket *TicketNamed `json:"ticket,omitzero"`
	Drop   string       `json:"drop,omitzero"` // the ticket of this id is no longer waited on
	End    *Ending      `json:"end,omitzero"`
	Note   string       `json:"note,omitzero"`
}

// TimerAt asks for a timer event at a time.
type TimerAt struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

// TicketNamed asks for a ticket to be waited on. ID is the state that
// issued it; the answer arrives as an event with the ticket's name, from the
// id.
type TicketNamed struct {
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
	Run        Run       // the run it is
	Time       time.Time // the time of the step
	Seed       []byte    // the step's own secret, for tickets and random numbers
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
	// Show gives the state a watcher is shown of a run of the type.
	Show(state jsontext.Value) Shown
	Manual() string
}

// Shown is the state of a run a watcher is shown for each step, beside
// what its head says. Each type fills one field.
type Shown struct {
	// Stages are a flow's stages, each with its status: waiting, running,
	// done, failed or skipped.
	Stages map[string]string `json:"stages,omitzero"`
	// States are a statechart's active states, outermost first.
	States []string `json:"states,omitzero"`
	// Turn is how many turns an agent has taken.
	Turn int `json:"turn,omitzero"`
}

// A Limiter is a type that adds limits of its own to those every type
// takes. It reads them with Parser.Limits.
type Limiter interface {
	Limits() []Field
}

// A Reader is a type with a part of its definitions that the controller
// needs. It reads them as plain JSON, and compiles and evaluates nothing.
type Reader interface {
	// Lists says the definition takes shapes from outside as it runs.
	Lists(def *Definition) bool
}

// Lists says a definition takes shapes from outside as it runs.
func Lists(def *Definition) bool {
	t, _ := TypeOf(def.Type)
	r, ok := t.(Reader)
	return ok && r.Lists(def)
}

// A Receiver is a type whose runs take events sent to them by their address,
// and so may be keyed.
type Receiver interface {
	Receives()
}

// Receives says a definition's type takes events sent to its runs.
func Receives(def *Definition) bool {
	t, _ := TypeOf(def.Type)
	_, ok := t.(Receiver)
	return ok
}

// An AddressSender is a type whose definitions can send an event to a run
// by its address.
type AddressSender interface {
	SendsToAddresses()
}

// SendsToAddresses says a definition's type can send an event to a run by
// its address.
func SendsToAddresses(def *Definition) bool {
	t, _ := TypeOf(def.Type)
	_, ok := t.(AddressSender)
	return ok
}

// A Keeper is a type whose runs keep events sent to them to take later.
type Keeper interface {
	Kept(state jsontext.Value) []Event
}

// Kept gives the events a run of a type keeps, by its state.
func Kept(typ string, state jsontext.Value) []Event {
	t, _ := TypeOf(typ)
	if k, ok := t.(Keeper); ok {
		return k.Kept(state)
	}
	return nil
}

// An Owner is a type that gives events names of its own, such as a timer's
// or a ticket's, which no sender may take.
type Owner interface {
	Owns(def *Definition, name string) bool
}

// Owns says whether a name is one a definition gives its own events.
func Owns(def *Definition, name string) bool {
	t, _ := TypeOf(def.Type)
	o, ok := t.(Owner)
	return ok && o.Owns(def, name)
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

// A Restart says what happens when a job ends. With no policy it runs once.
type Restart struct {
	// Always restarts the job after it ends well too, and not only after an
	// error or a timeout.
	Always bool `json:"always,omitzero"`
	// Limit is how many restarts are allowed, in the last Within if it is
	// given, and with no Limit there is no end to them.
	Limit  int      `json:"limit,omitzero"`
	Within Duration `json:"within,omitzero"`
	// Delay is the first wait before a restart, which doubles up to Longest.
	Delay   Duration `json:"delay"`
	Longest Duration `json:"longest"`
}

// Wait gives how long a restart waits, and how many times the delay has doubled
// after it, from how many times it had before and how long the attempt that
// ended stayed up. A delay starts again once an attempt has stayed up for twice
// its longest wait. A step's process can write any values here, so nothing in
// them is taken on trust: a wait is always more than nothing, and cannot overflow.
func (r *Restart) Wait(doubled int, up time.Duration) (time.Duration, int) {
	first, longest := time.Duration(r.Delay), time.Duration(r.Longest)
	if first <= 0 {
		first = time.Duration(DefaultDelay)
	}
	longest = max(longest, first)
	if up/2 >= longest {
		doubled = 0
	}
	wait := first
	for range doubled {
		if wait > longest/2 {
			return longest, doubled
		}
		wait *= 2
	}
	return wait, doubled + 1
}

// What a restart's after: says, and the delay of a policy that gives none.
const (
	RestartFailure = "failure"
	RestartAlways  = "always"
	DefaultDelay   = Duration(time.Second)
	DefaultLongest = Duration(5 * time.Minute)
)

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
