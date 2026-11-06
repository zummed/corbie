package controller

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// A step of a machine made while running reads its definition in a process
// of its own, with a limit on its memory and its time, and a step that
// passes either fails that machine and nothing else. So does a step of an
// agent after a list has given it shapes from outside. The process holds no
// key and reaches no store. It is given what it needs, and gives back what
// the machine asked for. That is data, and a machine's word, as a definition's
// would be: every name in the store, every runner and every flag follow from
// it here, as they do for any other machine.

// StepArg, after the argument that makes the binary run one job, makes it
// read one step.
const StepArg = "step"

// The limits of a step's process. A step of a small machine takes about 15
// ms and 25 MiB.
const (
	StepTime   = 10 * time.Second
	StepMemory = 1 << 30
)

const (
	// stepsAtOnce is how many of these steps one pass takes at a time. Past
	// it they wait for the next pass, so that many of them hold up no other
	// machine for longer than one of them.
	stepsAtOnce = 4
	// stepTries is how many times one step is taken before no answer is
	// taken to be its fault: a controller that is stopped takes the step's
	// process with it, and a step's process can take its controller with it.
	stepTries = 2
	// stepGrace is how long past its limit a step that was taken is left to
	// the copy that took it, and how much of a pass must be left after a
	// step's limit for what it asks for to be carried out.
	stepGrace = 5 * time.Second
	// answerDepth is how deep a step's answer nests: far past the 200 a
	// job's value may, and far from the 10000 where JSON ends.
	answerDepth = 1000
	// nameLimit is the longest name a step's answer may give: with what
	// surrounds it, a job's name in an inbox is one part of a name in the
	// store, and a directory store holds 255 bytes of that.
	nameLimit = 100
)

// errLeft is a step that is left for a later pass.
var errLeft = errors.New("the step is left for a later pass")

// stepIn is everything a step's process is given.
type stepIn struct {
	Instance   string              `json:"instance"`
	Definition *machine.Definition `json:"definition"`
	State      instance.State      `json:"state"`
	Number     int                 `json:"number"`
	Time       time.Time           `json:"time"`
	Events     []machine.Event     `json:"events,omitzero"`
	// First says this is an instance's first step, and Input is what the
	// start gives.
	First bool `json:"first,omitzero"`
	Input any  `json:"input,omitzero"`
}

// stepOut is what a step reads of a definition: for a step in a process of
// its own, what that process gives back. It is a machine's word on what it
// wants. It names nothing in the store and holds no flag of the controller's.
type stepOut struct {
	// Problems are what is wrong with the definition; a first step checks.
	Problems machine.Problems `json:"problems,omitzero"`
	// Input, Limit and Title are the start's input, with its defaults
	// filled in, and the time limit and the title the definition gives, for
	// a first step.
	Input any              `json:"input,omitzero"`
	Limit machine.Duration `json:"limit,omitzero"`
	Title string           `json:"title,omitzero"`
	// Inner is the machine's own state, which nothing here reads.
	Inner    jsontext.Value    `json:"inner,omitzero"`
	Requests []machine.Request `json:"requests,omitzero"`
	Ignored  []int             `json:"ignored,omitzero"`
	// Allow is the allow list written for the whole definition, filled in,
	// and Unfilled says why it could not be.
	Allow    []string `json:"allow,omitzero"`
	Unfilled string   `json:"unfilled,omitzero"`
	// Failure says the step failed, and why.
	Failure string `json:"failure,omitzero"`
}

// take is the step, in its process.
func (in *stepIn) take() *stepOut {
	b := &build{inst: in.Instance, def: in.Definition, st: &in.State, step: &instance.Step{Number: in.Number},
		time: in.Time, events: in.Events}
	if in.First {
		return b.readDefinition(&instance.Start{Input: in.Input})
	}
	return b.readDefinition(nil)
}

// StepMain is the whole of a step's process.
func StepMain() {
	wire, err := role.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "the step's process:", err)
		os.Exit(2)
	}
	role.LimitMemory(StepMemory)
	go role.WatchOwn(context.Background(), StepMemory, func() {
		fmt.Fprintf(os.Stderr, "the step went past its memory limit of %d bytes\n", int64(StepMemory))
		os.Exit(4)
	})
	var in stepIn
	if err := wire.Receive(&in); err != nil {
		fmt.Fprintln(os.Stderr, "what the step's process was given could not be read:", err)
		os.Exit(2)
	}
	// Nothing more comes on the channel. When it ends its controller has
	// gone, and a step that runs without end has nothing else to stop it.
	go func() {
		var nothing any
		wire.Receive(&nothing)
		role.Lost()
	}()
	if err := wire.Send(in.take(), 0, nil); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

// said keeps the first line of what a step's process prints, which is what
// went wrong when the process ended by itself, and reads the rest to its end.
type said struct {
	mu   sync.Mutex
	text []byte
}

func (s *said) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.text) < 256 {
		s.text = append(s.text, p[:min(len(p), 256)]...)
	}
	return len(p), nil
}

func (s *said) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	line, _, _ := bytes.Cut(s.text, []byte("\n"))
	return strings.TrimSpace(string(line))
}

// breach ends the machine in a step of the controller's own: the machine
// was not consulted, and what arrived for it goes no further. start is the
// start of an instance's first step.
func (b *build) breach(start *arrival, why *machine.Error) {
	if start != nil {
		b.use(start)
		b.record(machine.Event{Name: machine.Start, Data: b.st.Input}, start, true)
	}
	for i := range b.step.Events {
		b.step.Events[i].Own = true
	}
	b.end(instance.Failed, nil, why)
}

// A triedNote says how many times a step has been taken and when last.
type triedNote struct {
	Step  int       `json:"step"`
	Tries int       `json:"tries"`
	At    time.Time `json:"at"`
}

// try notes in the record that a step is being taken, before its process
// starts. It gives false when no process is to start: another copy is taking
// the step, or the step has been taken as many times as it may be and no
// answer came, in which case lost says so. A controller killed along with a
// step's process finds the note when it comes back.
func (p *pass) try(inst string, step int) (take, lost bool, err error) {
	at := store.Tried(p.ctl, inst)
	data, mark, err := p.store.Read(p.ctx, at)
	if err != nil {
		return false, false, err
	}
	how, note, now := store.OnlyIfNew, triedNote{}, p.c.Now().UTC()
	if mark != store.None {
		how = store.OnlyIfUnchanged(mark)
		if _, err := identity.Read(data, at, p.c.ID.Card, nil, &note); err != nil {
			note = triedNote{}
		}
	}
	tries := 1
	if note.Step > step {
		return false, false, nil // the instance has moved on since it was read
	}
	if note.Step == step {
		if free := note.At.Add(p.c.StepTime + stepGrace); now.Before(free) {
			p.due(free)
			return false, false, nil
		}
		tries = note.Tries + 1
	}
	if tries > stepTries {
		return false, true, nil
	}
	signed, err := p.c.ID.Write(at, p.now, noEnd, triedNote{Step: step, Tries: tries, At: now})
	if err != nil {
		return false, false, err
	}
	took, err := p.store.Write(p.ctx, at, signed, how)
	return took, false, err
}

// apart takes the part of a step that reads the definition in a process of
// its own. s and start are the start of an instance's first step. A step that
// passes a limit, or whose process ends without an answer, ends the machine,
// and it gives nothing. A pass that has no place for it, or too little time,
// or that finds another copy taking it, gives errLeft.
func (b *build) apart(s *instance.Start, start *arrival) (*stepOut, error) {
	p := b.p
	// It is taken only if its whole time limit fits in the time the host
	// has left.
	if deadline, ok := p.ctx.Deadline(); ok && time.Until(deadline) < p.c.StepTime+stepGrace {
		if p.c.StepTime+stepGrace > p.whole {
			// No call on this host is long enough for it. It fails now and
			// says so: left alone, it would wait and say nothing.
			b.breach(start, p.failf("the step may take %s, and this controller's host gives a call %s less the %s kept for carrying it out: give the host's calls more time",
				p.c.StepTime, p.whole.Round(time.Second), stepGrace))
			return nil, nil
		}
		return nil, p.leave()
	}
	select {
	case p.steps <- struct{}{}:
		defer func() { <-p.steps }()
	default:
		return nil, p.leave()
	}
	take, lost, err := p.try(b.inst, b.step.Number)
	switch {
	case err != nil:
		return nil, err
	case lost:
		b.breach(start, p.failf("the step was taken %d times and no answer came: the controller, or the host's call, ended while it ran", stepTries))
		return nil, nil
	case !take:
		return nil, errLeft
	}
	in := &stepIn{Instance: b.inst, Definition: b.def, State: *b.st, Number: b.step.Number, Time: b.time, Events: b.events}
	if s != nil {
		in.First, in.Input = true, s.Input
	}
	out, size, why, err := p.inProcess(in)
	if err != nil {
		return nil, err
	}
	if why == nil {
		why = b.wrong(out, size)
	}
	if why != nil {
		b.breach(start, why)
		return nil, nil
	}
	return out, nil
}

// leave leaves a step for the next pass.
func (p *pass) leave() error {
	p.mu.Lock()
	p.more = true
	p.mu.Unlock()
	return errLeft
}

// wrong says what is wrong with an answer that no definition could give, and
// that would not fit the step it is for. The names it gives go into names in
// the store, and the definition's own goes into a head, which is small: only
// the step's process has checked either. size is how large the answer is.
func (b *build) wrong(out *stepOut, size int) *machine.Error {
	if !stepName(b.st.Machine) {
		b.st.Machine = ""
		if len(out.Problems) == 0 {
			return b.p.failf("the definition gives a name no definition holds")
		}
	}
	if _, known := machine.TypeOf(b.st.Type); !known {
		b.st.Type = ""
		if len(out.Problems) == 0 {
			return b.p.failf("the definition gives a type no definition holds")
		}
	}
	state := *b.st
	state.Inner = nil
	held, err := json.Marshal(&state, json.Deterministic(true))
	if err != nil {
		return b.p.failf("%v", err)
	}
	if len(held)+size > blob.StepLimit {
		return b.p.failf("the step would take the state past the %d MiB a step in a process of its own may hold", blob.StepLimit>>20)
	}
	sends := 0
	for _, r := range out.Requests {
		if r.Send != nil {
			if sends++; sends > maxSends {
				return b.p.failf("a step taken in a process of its own sends at most %d events", maxSends)
			}
		}
		for _, name := range named(r) {
			if !stepName(name) {
				return b.p.failf("the step gave the name %.40q, which no definition holds", name)
			}
		}
		if r.Job != nil && (r.Job.Name == instance.AskName || strings.HasSuffix(r.Job.Name, cancelSuffix)) {
			return b.p.failf("the step gave the name %.40q, which no definition holds", r.Job.Name)
		}
		if r.Send != nil && r.Send.At != nil && !machine.SendsToAddresses(b.def) {
			return b.p.failf("the step sent to an address, which no definition of this type can ask for")
		}
		if r.End != nil && r.End.Error != nil && !slices.Contains(endings, r.End.Error.Status) {
			return b.p.failf("the step gave the status %.40q, which is no way to fail", r.End.Error.Status)
		}
	}
	return nil
}

// maxSends is how many events a step taken in a process of its own may send.
// A step's events are written one after another.
const maxSends = 1024

// cancelSuffix ends the name a job's cancel has in a runner's inbox.
const cancelSuffix = ".cancel"

// endings are the ways a machine can fail.
var endings = []string{machine.StatusError, machine.StatusTimeout, machine.StatusDenied, machine.StatusCancelled}

// named gives the names a request gives that go into names in the store or
// into a head.
func named(r machine.Request) []string {
	switch {
	case r.Job != nil:
		return []string{r.Job.Name}
	case r.Timer != nil:
		return []string{r.Timer.Name}
	case r.Ticket != nil:
		return []string{r.Ticket.ID, r.Ticket.Name}
	}
	return nil
}

// stepName says whether a name is one a definition's names and positions
// could make: words of letters, digits, '_' and '-', with '.' between them.
func stepName(name string) bool { return len(name) <= nameLimit && stepNameForm.MatchString(name) }

var stepNameForm = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

// inProcess runs a step in a process of its own. why says the step failed;
// an error says the pass ended, and the step waits for a later one.
func (p *pass) inProcess(in *stepIn) (out *stepOut, size int, why *machine.Error, err error) {
	printed := &said{}
	proc, err := role.Start(p.c.Command, "", printed)
	if err != nil {
		return nil, 0, p.failf("the step's process could not be started: %v", err), nil
	}
	defer proc.Close()
	passed := make(chan struct{})
	limit := time.AfterFunc(p.c.StepTime, func() {
		close(passed)
		proc.Kill()
	})
	defer limit.Stop()
	// A pass that ends takes the process with it.
	stop := context.AfterFunc(p.ctx, proc.Kill)
	defer stop()

	var unread error
	if proc.Wire.Send(in, 0, nil) == nil {
		var data []byte
		if data, unread = proc.Wire.ReceiveUpTo(blob.StepLimit); unread == nil {
			if nested(data) > answerDepth {
				return nil, 0, p.failf("what the step's process said is nested too deep: more than %d", answerDepth), nil
			}
			var answer stepOut
			if err := json.Unmarshal(data, &answer); err != nil {
				return nil, 0, p.failf("what the step's process said could not be read: %v", err), nil
			}
			return &answer, len(data), nil, nil
		}
	}
	proc.Ended()
	select {
	case <-p.ctx.Done():
		return nil, 0, nil, p.ctx.Err()
	case <-passed:
		return nil, 0, p.failf("the step took longer than the %s a step in a process of its own may take", p.c.StepTime), nil
	default:
	}
	if unread != nil && !errors.Is(unread, io.EOF) && !errors.Is(unread, io.ErrUnexpectedEOF) {
		return nil, 0, p.failf("what the step's process said could not be read: %v", unread), nil
	}
	return nil, 0, p.failf("the step's process ended without an answer: %s", printed), nil
}

// nested says how deep a JSON text nests.
func nested(data []byte) int {
	most, depth, text := 0, 0, false
	for i := 0; i < len(data); i++ {
		switch c := data[i]; {
		case text && c == '\\':
			i++
		case c == '"':
			text = !text
		case text:
		case c == '[' || c == '{':
			depth++
			most = max(most, depth)
		case c == ']' || c == '}':
			depth--
		}
	}
	return most
}
