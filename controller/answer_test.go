package controller_test

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/agent"
)

// stepThatLies is the argument that makes the test binary a step's process
// that has been taken over: it answers with what the argument after it says.
const stepThatLies = "step-that-lies"

// heard is what a lie is told of the step.
type heard struct {
	Instance string         `json:"instance"`
	State    instance.State `json:"state"`
	Step     instance.Step  `json:"step"`
	Index    int            `json:"index,omitzero"`
	Number   int            `json:"number,omitzero"`
	Time     time.Time      `json:"time"`
}

// lie is an answer. The first group of fields says what a step could once
// say of itself; the second is what an answer holds now. A lie fills both.
type lie struct {
	State   instance.State `json:"state"`
	Step    instance.Step  `json:"step"`
	Index   int            `json:"index,omitzero"`
	Stepped bool           `json:"stepped,omitzero"`
	Asks    map[string]any `json:"asks,omitzero"`

	Input    any               `json:"input,omitzero"`
	Limit    string            `json:"limit,omitzero"`
	Inner    jsontext.Value    `json:"inner,omitzero"`
	Requests []machine.Request `json:"requests,omitzero"`
	Allow    []string          `json:"allow,omitzero"`
}

func lying(kind string, args []string) {
	wire, err := role.Open()
	if err != nil {
		os.Exit(2)
	}
	var in heard
	if err := wire.Receive(&in); err != nil {
		os.Exit(2)
	}
	out := lie{State: in.State, Step: in.Step, Index: in.Index, Stepped: true}
	out.State.Status, out.State.Machine, out.State.Type = instance.Running, "flow", "flow"
	asks := func(asks ...instance.Ask) { out.Step.Asks = append(out.Step.Asks, asks...) }
	ask := func(a instance.Ask) { asks(a) }
	soon := in.Time.Add(time.Minute)
	// A lie is told once: after it, a step asks for nothing. "after a list" is
	// another lie, told at the second step, the first a listed agent takes
	// here.
	if kind == "grown" {
		out.State, out.Step = instance.State{}, instance.Step{}
	}
	second := kind == "after a list" && max(in.Number, in.Step.Number) == 2
	if second {
		kind, args = args[0], args[1:]
	}
	if max(in.Number, in.Step.Number) > 1 && !second {
		if kind == "grown" && max(in.Number, in.Step.Number) == 2 {
			out.Requests = []machine.Request{{Ticket: &machine.TicketNamed{ID: "y", Name: "y", Value: strings.Repeat("a", 3<<20)}}}
		}
		kind = ""
	}
	switch kind {
	case "used":
		out.Step.Used = append(out.Step.Used, args...)

	case "above":
		job := &machine.Job{Name: "x", Script: "./step.js", Above: true, Allow: []string{"net:*"},
			Found: &machine.Found{In: "m", Blob: in.State.Definition}}
		ask(instance.Ask{Kind: instance.AskScript, Name: "x", Attempt: 1, Runner: "r1", Message: store.JobMessage(in.Instance, "x"), At: soon, What: "due.x~1", Job: job})
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "x", Script: "./step.js", Above: true, Allow: []string{"net:*"}}}}

	case "to":
		ref := in.State.Definition.Value()
		ask(instance.Ask{Kind: instance.AskEvent, To: args[0], Event: &machine.Event{Name: "e", Data: ref}, Message: "m.event"})
		ask(instance.Ask{Kind: instance.AskEnd, To: args[0], Name: "go", Output: ref, Message: "m.end"})
		out.Requests = []machine.Request{{Send: &machine.Send{To: args[0], Event: "e", Data: ref}}}

	case "receipt":
		ask(instance.Ask{Kind: instance.AskReceipt, Sender: "c1", Message: args[0], Receipt: &instance.Receipt{Taken: true}})

	case "cancel":
		ask(instance.Ask{Kind: instance.AskCancel, Name: "x", Runner: "r1", At: soon, Message: args[0] + ".cancel"})
		out.Requests = []machine.Request{{Cancel: args[0]}}

	case "taken":
		ask(instance.Ask{Kind: instance.AskScript, Name: "x", Attempt: 1, Runner: "r1", Message: args[0], At: soon, What: "due.x~1",
			Job: &machine.Job{Name: "x", Script: "./step.js", Found: &machine.Found{In: "m", Blob: in.State.Definition}}})

	case "state":
		out.State.Machine = "another"
		out.State.Keep = machine.Duration(1000 * time.Hour)
		out.State.TimeoutAt = time.Time{}
		out.State.Ask = nil
		out.State.Above.AskAbout = nil
		out.State.Above.Keep = machine.Duration(1000 * time.Hour)
		out.Limit = "1000h"

	case "unasked":
		// A job that an ask above must hold, with the list that holds it gone.
		out.State.Above.AskAbout = nil
		job := &machine.Job{Name: "x", Script: "./step.js", Allow: []string{"net:a.example"},
			Found: &machine.Found{In: "m", Blob: in.State.Definition}}
		out.State.Jobs = map[string]*instance.Going{"x": {Job: *job, Asked: 1, Attempt: 1, Runner: "r1", Due: soon}}
		ask(instance.Ask{Kind: instance.AskScript, Name: "x", Attempt: 1, Runner: "r1", Message: store.JobMessage(in.Instance, "x"), At: soon, What: "due.x~1", Job: job})
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "x", Script: "./step.js", Allow: []string{"net:a.example"}}}}

	case "fills":
		// A job that names an ask of the step's own choosing, and fills that
		// ask and the one a job above declares.
		at := "x"
		if len(args) > 0 {
			at = args[0]
		}
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "x", Script: "./step.js", Approve: true,
			Ask: &machine.JobSpec{Script: "./step.js"}, AskAt: at}}}
		filled := map[string]any{"job": &machine.Job{Name: "x", Script: "./step.js", Allow: []string{"all"}}}
		out.Asks = map[string]any{instance.AskName: filled, at: filled}

	case "two jobs":
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "x", Script: "./step.js"}}, {Job: &machine.Job{Name: "y", Script: "./step.js"}}}

	case "ended":
		out.State.Status = instance.Done
		out.State.Ended = in.Time
		out.State.Machine = "ended"

	case "many":
		// Bare asks of every kind, and a job that is nothing.
		asks(instance.Ask{Kind: instance.AskScript}, instance.Ask{Kind: instance.AskChild}, instance.Ask{Kind: instance.AskFail},
			instance.Ask{Kind: instance.AskEvent, To: "c1/nothing"}, instance.Ask{Kind: instance.AskCancel, Runner: "r1"},
			instance.Ask{Kind: instance.AskEnd, To: "c1/nothing"}, instance.Ask{Kind: instance.AskReceipt, Sender: "c1", Message: "m"})
		out.State.Jobs = map[string]*instance.Going{"x": nil, "y": {Job: machine.Job{Name: "y"}, Asked: 1, Attempt: -5, Runner: "r1"}}
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "y", Tags: []string{""}, Allow: []string{"", ":", "secret:", "secret:@"}, Restart: &machine.Restart{Limit: -3},
			Timeout: 1, Memory: -1, Input: map[string]any{"$blob": "../..", "type": "x", "size": 1.0, "key": "x"}, Wants: []string{""}}},
			{Cancel: "z"}, {Send: &machine.Send{To: "nobody"}}, {Clear: "z"}, {Drop: "z"}, {Note: "note"},
			{Timer: &machine.TimerAt{Name: "t"}}, {Ticket: &machine.TicketNamed{ID: "z", Name: "z"}}, {}}
		out.Inner = jsontext.Value(`{"stages":[null,{"":[]}]}`)

	case "wild":
		// A job that reaches a runner, and a machine that is started, with
		// everything a request can hold at its strangest.
		strange := []string{"", ":", "::", "secret", "secret:", "secret:@", "secret:*@*", "secret:a@", "all", "net:", "fs.read:", "exec:\x00", "secret:\u2028@*"}
		bad := func(at, typ string) any { return map[string]any{"$blob": at, "type": typ, "size": 1.0, "key": "x"} }
		long := machine.Duration(1<<63 - 1)
		out.Requests = []machine.Request{
			{Job: &machine.Job{Name: "x", Script: "./step.js", Allow: strange, Deny: strange, Timeout: long, Restart: &machine.Restart{Limit: -1}, Memory: -1,
				Input: bad("../..", "text/plain"), Wants: strange, Shown: bad("", ""), Files: strange}},
			{Job: &machine.Job{Name: "y", Stored: bad("../../secrets/x", blob.Definition), Allow: strange, Timeout: long, Input: bad("", blob.Definition)}},
			{Job: &machine.Job{Name: "z", Stored: bad("ctl/locations/+/secrets/x", blob.Definition)}},
			{Job: &machine.Job{Name: "w", Stored: bad("../x", "text/javascript")}},
			{Send: &machine.Send{To: machine.Parent, Event: "e", Data: bad("../..", blob.Definition)}},
			{Timer: &machine.TimerAt{Name: "t", At: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)}},
		}

	case "ended well":
		bad := map[string]any{"$blob": "../..", "type": blob.Definition, "size": 1.0, "key": "x"}
		out.Requests = []machine.Request{{End: &machine.Ending{Output: bad}}}

	case "named":
		out.Requests = []machine.Request{requested(args[0], args[1])}

	case "held":
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "b", Script: "./step.js", Allow: []string{"fs.read:/x/**"}}}}

	case "many asks":
		n, _ := strconv.Atoi(args[0])
		for i := range n {
			out.Requests = append(out.Requests, machine.Request{Job: &machine.Job{Name: fmt.Sprintf("j%d", i), Script: "./step.js", AskAt: args[1]}})
		}

	case "key send":
		// A send to a key: args are the key, and the machine if there is one.
		// What that starts is not the machine that lies.
		if in.State.Type != "statechart" && in.State.Type != "agent" {
			break
		}
		send := &machine.Send{Key: &args[0], Event: "e", Data: 1.0}
		if len(args) > 1 {
			send.Machine = args[1]
		}
		out.Requests = []machine.Request{{Send: send}}
		out.State.Above.Made = false
		out.State.Above.Layers = nil
		out.State.Above.Depth = 0

	case "key event":
		// A send of an event of Corbie's own to a key, which what the key names
		// would take for its start. args are the key and the event.
		if in.State.Type == "statechart" && in.State.Machine != "waits" {
			out.Requests = []machine.Request{{Send: &machine.Send{Key: &args[0], Event: args[1]}}}
		}

	case "key job":
		out.Requests = []machine.Request{{Job: &machine.Job{Name: "k", Stored: nil, Machine: args[1], RunKey: &args[0]}}}

	case "strange keys":
		// Keys of every odd form, and data that names nothing.
		bad := map[string]any{"$blob": "../..", "type": blob.Definition, "size": 1.0, "key": "x"}
		for _, key := range []string{"\x00", "../../secrets/x", "a/b", "\u2028", strings.Repeat("é", 256), "ctl/keys/c1/x", "${input}"} {
			out.Requests = append(out.Requests, machine.Request{Send: &machine.Send{Key: &key, Machine: args[0], Event: "\x00", Data: bad}})
		}

	case "many keys":
		n, _ := strconv.Atoi(args[0])
		for i := range n {
			key := fmt.Sprint("k", i)
			out.Requests = append(out.Requests, machine.Request{Send: &machine.Send{Key: &key, Event: "e"}})
		}

	case "many sends":
		n, _ := strconv.Atoi(args[0])
		for range n {
			out.Requests = append(out.Requests, machine.Request{Send: &machine.Send{To: machine.Parent, Event: "e"}})
		}

	case "strange job keys":
		// A job of the definition under keys of every odd form.
		for i, key := range []string{"\x00", "../../secrets/x", "a/b", "\u2028", strings.Repeat("é", 256), "ctl/keys/c1/x", "${input}"} {
			out.Requests = append(out.Requests, machine.Request{Job: &machine.Job{Name: fmt.Sprint("k", i), Machine: args[0], RunKey: &key}})
		}

	case "large":
		out.State.Inner = jsontext.Value(`"` + strings.Repeat("a", 5<<20) + `"`)
		out.Inner = out.State.Inner

	case "grown":
		// Within the limit by itself, on a state that is close to it already.
		out.Requests = []machine.Request{{Ticket: &machine.TicketNamed{ID: "x", Name: "x", Value: strings.Repeat("a", 3<<20)}}}

	case "deep":
		deep := strings.Repeat("[", 9985) + strings.Repeat("]", 9985)
		out.State.Inner = jsontext.Value(deep)
		out.Inner = out.State.Inner
		out.Requests = []machine.Request{{End: &machine.Ending{Output: nest(9985)}}}
	}
	wire.Send(out, 0, nil)
	os.Exit(0)
}

// nest gives a value nested to a depth.
func nest(depth int) any {
	var v any = "x"
	for range depth {
		v = []any{v}
	}
	return v
}

// requested is a request that gives a name of a kind.
func requested(kind, name string) machine.Request {
	switch kind {
	case "job":
		return machine.Request{Job: &machine.Job{Name: name, Script: "./step.js"}}
	case "timer":
		return machine.Request{Timer: &machine.TimerAt{Name: name, At: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)}}
	case "ticket":
		return machine.Request{Ticket: &machine.TicketNamed{ID: "t", Name: name, Value: "v"}}
	}
	return machine.Request{Ticket: &machine.TicketNamed{ID: name, Name: "t", Value: "v"}}
}

// liesUnder is a machine that lists everything, so that what its made machine
// says of a key is held by nothing but the controller.
const liesUnder = `
	flow: maker
	stages:
	  make: { job: ./make.js }
	  go: { job: { machine: "${stages.make.output}", allow: ["all"] } }
`

// aChart is a made statechart that holds a machine, and waits.
const aChart = `
	statechart: liar
	initial: a
	states:
	  a:
	    on:
	      never: { send: { to: { machine: { flow: kid, keep: 1000h, stages: { a: { job: ./step.js } } }, key: k }, event: e } }
`

// A machine made while running fails when it asks to send to a key or to give
// a job one, whatever its answer says of itself and whatever the job above it
// lists. Nothing is read of the keys, and no other run hears of it.
func Test_ctl_answer_of_a_made_machine_uses_no_key(t *testing.T) {
	for _, lie := range [][]string{
		{"key send", "k"},
		{"key send", "k", "in-place-1"},
		{"key send", strings.Repeat("a", 300)},
		{"key event", "k", machine.Start},
		{"key job", "k", "in-place-1"},
		{"strange keys", "in-place-1"},
		{"many keys", "1000"},
	} {
		w := newWorld(t)
		target := w.holding(w.client, "k")
		files := map[string]string{"make.js": "return 1"}
		r1 := w.newRunner()
		run := w.submit(liesUnder, files, nil)
		w.settle()
		w.lieNext(lie[0], lie[1:]...)
		w.noted()
		w.made(r1, run, aChart)
		w.settle()
		st := w.latest(w.child(run)).State
		if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "sends to no key") {
			t.Fatalf("%s: the machine is %s: %+v", lie[0], st.Status, st.Error)
		}
		if steps := w.steps(target); len(steps) != 1 {
			t.Fatalf("%s: the run with the key took %d steps", lie[0], len(steps))
		}
		for _, op := range w.noted() {
			if strings.Contains(op.Name, "/keys/") {
				t.Fatalf("%s: the answer led to a look at %s", lie[0], op.Name)
			}
		}
	}
}

// A step's events are written one after another, so an answer sends a bounded
// number of them.
func Test_ctl_answer_with_many_sends_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	run := w.submit(liesUnder, map[string]string{"make.js": "return 1"}, nil)
	w.settle()
	w.lieNext("many sends", "4000")
	w.made(r1, run, aChart)
	w.settle()
	st := w.latest(w.child(run)).State
	if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "at most") {
		t.Fatalf("the machine is %s: %+v", st.Status, st.Error)
	}
}

// A listed agent's answer can give a job of its definition a key, and the
// oddest are text in a step like any other.
func Test_ctl_answer_with_strange_keys_ends_no_controller(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	files := map[string]string{"kid.yaml": waits}
	for name, text := range listerFiles {
		files[name] = text
	}
	w.secret("KEY", "k", "/")
	run := w.submit(strings.Replace(lister, "  t:\n", "  m: { does: M, job: { machine: ./kid.yaml } }\n\t  t:\n", 1), files, nil)
	w.settle()
	w.lieNext("after a list", "strange job keys", "./kid.yaml")
	w.result(r1, run, "t", oneTool)
	w.settle()
	if head := w.head(run); head == nil {
		t.Fatal("the agent has no head")
	}
	other := w.submit(family, nil, nil)
	w.settle()
	w.done(other)
}

// A step of a type no definition of which can send to a key cannot say so.
func Test_ctl_answer_of_a_listed_agent_cannot_send_to_a_key(t *testing.T) {
	w := newWorld(t)
	run, r1 := w.listed()
	target := w.holding(w.client, "k")
	w.lieNext("after a list", "key send", "k")
	w.result(r1, run, "t", oneTool)
	w.settle()
	if st := w.latest(run).State; st.Status != instance.Failed || !strings.Contains(st.Error.Error, "sent to a key") {
		t.Fatalf("the agent is %s: %+v", st.Status, st.Error)
	}
	if steps := w.steps(target); len(steps) != 1 {
		t.Fatalf("the run took %d steps", len(steps))
	}
}

const plain = "flow: plain\nstages:\n  a: { job: ./step.js }\n"

// lied has the next step of a made machine answered with a lie. It gives
// the machine above.
func (w *world) lied(text, kind string, args ...string) string {
	w.t.Helper()
	w.lieNext(kind, args...)
	return w.startMade(text)
}

// madeBy is startMade for a world with a runner of its own.
func (w *world) madeBy(r1 *identity.Identity, text string) string {
	w.t.Helper()
	run := w.holder(r1)
	w.made(r1, run, text)
	return run
}

func (w *world) lieNext(kind string, args ...string) {
	w.ctrl.Command = append([]string{os.Args[0], stepThatLies, kind}, args...)
}

// child gives the machine a run started, once it has.
func (w *world) child(run string) string {
	w.t.Helper()
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			if ask.Child != "" {
				return ask.Child
			}
		}
	}
	w.t.Fatalf("%s started no machine", run)
	return ""
}

// done takes an owner's machine that has nothing to do with the lie to its
// end, and says so if it does not get there.
func (w *world) done(run string) {
	w.t.Helper()
	for range 8 {
		w.settle()
		w.tick(time.Second)
	}
	if head := w.head(run); head == nil || head.Status != instance.Done {
		w.t.Fatalf("an owner's machine ended as %+v", head)
	}
}

// inboxOf reads the messages in a runner's inbox.
func (w *world) inboxOf(r *identity.Identity) map[string]*instance.Message {
	w.t.Helper()
	jobs := map[string]*instance.Message{}
	for _, name := range w.list(store.Inbox(r.Name)) {
		data, _, err := w.owner.Read(ctx, name)
		if err != nil {
			w.t.Fatal(err)
		}
		var msg instance.Message
		if _, err := identity.Read(data, name, w.ctl.Card, r, &msg); err != nil {
			w.t.Fatal(err)
		}
		jobs[name[strings.LastIndexByte(name, '/')+1:]] = &msg
	}
	return jobs
}

// madeUnder runs an owner's machine whose job go starts a made machine, as
// far as the first step of that machine, which is answered with a lie.
func (w *world) madeUnder(owner, text string, files map[string]string, kind string, args ...string) (run string, r1 *identity.Identity) {
	w.t.Helper()
	r1 = w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run = w.submit(owner, files, nil)
	w.settle()
	w.lieNext(kind, args...)
	w.made(r1, run, text)
	w.pass() // the machine above asks for the machine below
	w.settle()
	return run, r1
}

// A step's answer names nothing to delete: what the step used is what the
// controller gave it. The store keeps everything else.
func Test_ctl_answer_says_nothing_of_what_a_step_used(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	w.settle()
	secret := store.Secret("ctl", "/", "token")
	if _, err := w.owner.Write(ctx, secret, []byte("sealed"), store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	wakes := w.wakes()
	targets := append([]string{store.Trust("ctl", "c1"), secret, store.Trust("ctl", "r1"), store.Step("ctl", owner, 1)}, wakes...)
	run := w.lied(plain, "used", targets...)
	w.settle()
	for _, name := range targets {
		if _, mark, err := w.owner.Read(ctx, name); err != nil || mark == store.None {
			t.Fatalf("the answer of %s deleted %s", run, name)
		}
	}
	w.done(owner)
}

// A job in an answer has no say in whether it is an ask from above. With no
// ask above, that used to end the controller.
func Test_ctl_answer_cannot_mark_a_job_as_from_above(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run := w.lied(plain, "above")
	w.settle()
	if job := w.latest(w.child(run)).State.Jobs["x"]; job == nil || job.Above {
		t.Fatalf("the job is %+v", job)
	}
	w.done(owner)
}

const asking = `
	flow: maker
	ask: ./ok.js
	stages:
	  make: { job: ./make.js }
	  go: { job: { machine: "${stages.make.output}" } }
`

var asked = map[string]string{"make.js": "return 1", "ok.js": "return { approve: input.jobs.map(j => j.name) }"}

// With an ask above, a job marked as from above would run under that ask's
// layers, and the narrowing of a made machine would be gone.
func Test_ctl_answer_cannot_drop_a_made_machines_narrowing(t *testing.T) {
	w := newWorld(t)
	_, r1 := w.madeUnder(asking, plain, asked, "above")
	found := false
	for name, msg := range w.inboxOf(r1) {
		if msg.Job == nil || !strings.HasSuffix(name, ".x") {
			continue
		}
		found = true
		if !slices.ContainsFunc(msg.Job.Layers, func(l permission.Layer) bool { return strings.Contains(l.Name, `the job "go"`) }) {
			t.Fatalf("the job went with the layers %+v", msg.Job.Layers)
		}
	}
	if !found {
		t.Fatal("the job was not sent")
	}
}

// An answer sends nothing to any instance but its own parent and children.
func Test_ctl_answer_cannot_send_to_another_instance(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	w.settle()
	blobs := w.list(store.Blobs("ctl", owner))
	run := w.lied(plain, "to", owner)
	w.settle()
	if got, kept := w.list(store.Blobs("ctl", owner)), blobs; !slices.Equal(got, kept) {
		t.Fatalf("the answer of %s copied a blob into another record: %v and not %v", run, got, kept)
	}
	w.done(owner)
}

// An answer writes no receipt.
func Test_ctl_answer_cannot_forge_a_receipt(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	w.lied(plain, "receipt", "a-message-to-come")
	w.settle()
	if got := w.list(store.Receipts("ctl", "c1")); slices.Contains(got, store.Receipt("ctl", "c1", "a-message-to-come")) {
		t.Fatalf("the receipts are %v", got)
	}
	w.done(owner)
}

// An answer stops no job of another instance.
func Test_ctl_answer_cannot_cancel_another_instances_job(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	owner := w.holder(r1)
	job := store.JobMessage(owner, "make")
	before := w.list(store.Inbox("r1"))
	w.lieNext("cancel", job)
	w.madeBy(r1, plain)
	w.settle()
	for _, name := range w.list(store.Inbox("r1")) {
		if strings.HasSuffix(name, job+".cancel") {
			t.Fatalf("%s is in the runner's inbox beside %v", name, before)
		}
	}
}

// An answer takes no name that another machine's message will need.
func Test_ctl_answer_cannot_take_a_message_name(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	owner := w.submit(`
		flow: two
		stages:
		  a: { job: ./a.js }
		  b: { follows: [a], job: ./a.js }
	`, map[string]string{"a.js": "return 1"}, nil)
	w.settle()
	// The next thing the owner's machine sends: its second job.
	job := store.JobMessage(owner, "b")
	w.lieNext("taken", job)
	w.madeBy(r1, plain)
	w.settle()
	w.result(r1, owner, "a", instance.Result{Status: instance.Done, Output: 1})
	w.settle()
	if msg := w.inboxOf(r1)[job]; msg == nil || msg.Job == nil || msg.Job.Instance != owner {
		t.Fatalf("the name of the job is held by %+v", msg)
	}
}

const keptTwoHours = `
	flow: plain
	keep: 2h
	limits: { timeout: 3h }
	stages:
	  a: { job: ./step.js }
`

// What a definition says is read from the definition, and an answer that
// says otherwise changes nothing.
func Test_ctl_answer_cannot_change_what_the_controller_holds(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run := w.lied(keptTwoHours, "state")
	w.settle()
	st := w.latest(w.child(run)).State
	if st.Machine != "plain" || st.Type != "flow" {
		t.Fatalf("the state is %+v", st)
	}
	if time.Duration(st.Keep) != 2*time.Hour {
		t.Fatalf("the instance is kept for %s", st.Keep)
	}
	if st.Above.Keep != w.latest(run).State.Keep {
		t.Fatalf("what the instance was started under became %+v", st.Above)
	}
	w.done(owner)
}

// An answer's word on the time limit is taken as a definition's would be,
// and is no longer than its parent's job allows.
func Test_ctl_answer_cannot_outlast_its_job(t *testing.T) {
	w := newWorld(t)
	const timed = `
		flow: maker
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}", timeout: 1h } }
	`
	run, _ := w.madeUnder(timed, plain, map[string]string{"make.js": "return 1"}, "state")
	st := w.latest(w.child(run)).State
	if st.TimeoutAt.IsZero() || st.TimeoutAt.Sub(st.Started) != time.Hour {
		t.Fatalf("the machine times out at %s, started at %s", st.TimeoutAt, st.Started)
	}
}

// A keep past what the parent's own allows is cut to it, as a definition's
// is.
func Test_ctl_keep_of_a_made_machine_is_no_longer_than_its_parents(t *testing.T) {
	w := newWorld(t)
	run := w.lied("flow: plain\nkeep: 1000h\nstages:\n  a: { job: ./step.js }\n", "state")
	w.settle()
	st := w.latest(w.child(run)).State
	if parent := w.latest(run).State; st.Keep != parent.Keep {
		t.Fatalf("the machine is kept for %s and its parent for %s", st.Keep, parent.Keep)
	}
}

const asksFirst = `
	flow: maker
	ask: ./ok.js
	ask_about: [ network ]
	stages:
	  make: { job: ./make.js }
	  go: { job: { machine: "${stages.make.output}" } }
`

// What an ask above must be asked about stays held, whatever the answer
// says of the list.
func Test_ctl_answer_cannot_clear_what_approval_holds(t *testing.T) {
	w := newWorld(t)
	run, r1 := w.madeUnder(asksFirst, plain, asked, "unasked")
	if st := w.latest(w.child(run)).State; !slices.Contains(st.Above.AskAbout, "network") {
		t.Fatalf("the list above the machine is %v", st.Above.AskAbout)
	}
	if job := w.latest(w.child(run)).State.Jobs["x"]; job == nil || !job.Held || job.Runner != "" {
		t.Fatalf("the job is %+v", job)
	}
	for name := range w.inboxOf(r1) {
		if strings.HasSuffix(name, ".x") {
			t.Fatalf("%s went to the runner with no ask", name)
		}
	}
}

// An answer has nowhere to name or fill an ask. A step taken over cannot put
// its own script there and have it decide what the machine holds, or run it
// with the layers of the machine that declares the ask.
func Test_ctl_answer_cannot_fill_an_owners_ask(t *testing.T) {
	w := newWorld(t)
	_, r1 := w.madeUnder(asking, plain, asked, "fills")
	for _, msg := range w.inboxOf(r1) {
		if msg.Job == nil || msg.Job.Name != instance.AskName {
			continue
		}
		if msg.Job.Path != "./ok.js" {
			t.Fatalf("the ask runs %s", msg.Job.Path)
		}
		return
	}
	t.Fatal("no ask was sent")
}

// An owner's machine that names who decides, and starts a machine made while
// running that is allowed what must be asked about.
const humanAsks = `
	flow: maker
	ask: ./human.js
	ask_about: [ "fs.read:/x/**" ]
	stages:
	  make: { job: ./make.js }
	  go: { job: { machine: "${stages.make.output}", allow: [ "fs.read:/x/**" ] } }
`

// madeUnderHuman runs humanAsks, has its ask approve the job that starts the
// made machine, and takes that machine as far as its jobs go. It gives the
// run and the machine made.
func (w *world) madeUnderHuman(r1 *identity.Identity, made *machine.Draft) (run, child string) {
	w.t.Helper()
	run = w.submit(humanAsks, map[string]string{"make.js": "return 1", "human.js": "return {}"}, nil)
	w.settle()
	ref, err := made.Store(ctx, w.ctrl.Store, "ctl", run)
	if err != nil {
		w.t.Fatal(err)
	}
	w.result(r1, run, "make", instance.Result{Status: instance.Done, Output: ref.Value()})
	w.settle()
	w.result(r1, run, instance.AskName, instance.Result{Status: instance.Done, Output: map[string]any{"approve": []any{"go"}}})
	w.settle()
	w.pass()
	w.settle()
	return run, w.child(run)
}

// wantHuman says the ask an instance started is the owner's: its path, the
// blob of the owner's own script, and no narrowing of the machine that asks.
func (w *world) wantHuman(r1 *identity.Identity, run, asker string) {
	w.t.Helper()
	var theirs, owners *instance.Job
	for _, msg := range w.inboxOf(r1) {
		if msg.Job == nil || !strings.HasPrefix(msg.Job.Name, instance.AskName) {
			continue
		}
		switch msg.Instance {
		case run:
			owners = msg.Job
		case asker:
			theirs = msg.Job
		}
		if msg.Job.Path != "./human.js" {
			w.t.Fatalf("%s asks %s", msg.Instance, msg.Job.Path)
		}
	}
	switch {
	case owners == nil || theirs == nil:
		w.t.Fatalf("the owner's ask is %+v, and that of the machine below is %+v", owners, theirs)
	case theirs.Script.At != owners.Script.At:
		w.t.Fatalf("the machine below runs %+v, and the owner's ask is %+v", theirs.Script, owners.Script)
	case slices.ContainsFunc(theirs.Layers, func(l permission.Layer) bool { return strings.Contains(l.Name, `the job "go"`) }):
		w.t.Fatalf("the ask has the layers of the machine that asks: %+v", theirs.Layers)
	}
}

// madeHolding answers a machine's job with a made machine that holds a job. A
// client's check would refuse it for having no ask.
func (w *world) madeHolding(r1 *identity.Identity, run string) {
	w.t.Helper()
	made, err := machine.ParseMade([]byte("flow: below\nstages:\n  b: { job: { script: ./step.js, approve: true } }\n"),
		machine.Given(map[string]string{"step.js": "return 1"}))
	if err != nil {
		w.t.Fatal(err)
	}
	ref, err := made.Store(ctx, w.ctrl.Store, "ctl", run)
	if err != nil {
		w.t.Fatal(err)
	}
	w.result(r1, run, "make", instance.Result{Status: instance.Done, Output: ref.Value()})
}

func (w *world) newRunner() *identity.Identity {
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	return r1
}

// A definition made while running does not replace the decider the owner
// named. Its check refuses an ask, and a process that skips the check gains
// nothing: the ask is not kept, and what it holds goes to the owner's ask,
// under the machine that holds it.
func Test_ctl_made_machine_has_no_ask_of_its_own(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	w.lieNext("held")
	run, child := w.madeUnderHuman(r1, w.draft(`
		flow: below
		ask: ./step.js
		stages:
		  b: { job: { script: ./step.js, allow: [ "fs.read:/x/**" ] } }
	`, map[string]string{"step.js": "return 1"}))
	if st := w.latest(child).State; st.Ask != nil {
		t.Fatalf("the machine keeps the ask %+v", st.Ask)
	}
	w.wantHuman(r1, run, child)
}

// A machine that writes an ask is told it has none, at the top of its
// definition or in a set, and the owner's flow carries on.
func Test_ctl_made_machine_that_writes_an_ask_fails(t *testing.T) {
	for what, draft := range map[string]func(w *world) *machine.Draft{
		"at its top": func(w *world) *machine.Draft {
			return w.draft("flow: below\nask: ./step.js\nstages:\n  b: { job: ./step.js }\n", map[string]string{"step.js": "return 1"})
		},
		"in a part": func(w *world) *machine.Draft {
			return w.draft(`
				agent: below
				model: { name: m, service: anthropic, address: "http://127.0.0.1:1", secret: KEY }
				prompt: hello
				open:
				  s: { include: ./part.yaml }
			`, map[string]string{"step.js": "return 1", "part.yaml": "does: Things.\nask: ./step.js\ntools:\n  t: { does: T, job: ./step.js }\n"})
		},
	} {
		w := newWorld(t)
		r1 := w.newRunner()
		run := w.holder(r1)
		ref, err := draft(w).Store(ctx, w.ctrl.Store, "ctl", run)
		if err != nil {
			t.Fatal(err)
		}
		w.result(r1, run, "make", instance.Result{Status: instance.Done, Output: ref.Value()})
		w.settle()
		w.pass()
		w.settle()
		st := w.latest(w.child(run)).State
		if st.Status != instance.Failed || st.Error == nil || !strings.Contains(st.Error.Error, "no ask of its own") {
			t.Errorf("with an ask %s, the machine ended %s: %+v", what, st.Status, st.Error)
		}
	}
}

// A made machine hands down the ask above it: one written in place in it goes
// to the owner's ask too.
func Test_ctl_made_machine_hands_down_the_ask_above_it(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	run, child := w.madeUnderHuman(r1, w.draft(`
		flow: above
		stages:
		  a:
		    job:
		      machine:
		        flow: below
		        stages:
		          b: { job: { script: ./step.js, allow: [ "fs.read:/x/**" ] } }
	`, map[string]string{"step.js": "return 1"}))
	w.wantHuman(r1, run, w.child(child))
}

// With no ask above it a made machine has none to go to, and its machine is
// told why.
func Test_ctl_made_machine_with_no_ask_above_is_denied(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	run := w.holder(r1)
	w.madeHolding(r1, run)
	w.settle()
	w.pass()
	w.settle()
	st := w.latest(w.child(run)).State
	if st.Status != instance.Failed || st.Error == nil || st.Error.Status != machine.StatusDenied ||
		!strings.Contains(st.Error.Error, "a machine made while running uses only an ask above it") {
		t.Errorf("the machine ended %s: %+v", st.Status, st.Error)
	}
	for _, msg := range w.inboxOf(r1) {
		if msg.Job != nil && msg.Job.Path == "./step.js" {
			t.Errorf("%s went to the runner", msg.Job.Name)
		}
	}
}

// An owner's ask that is a machine runs for a made machine, and takes no ask
// from above it.
func Test_ctl_ask_that_is_a_machine_runs_for_a_made_machine(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	run := w.submit(`
		flow: maker
		ask: { machine: ./asks.yaml }
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}" } }
	`, map[string]string{"make.js": "return 1", "human.js": "return {}",
		"asks.yaml": "flow: asks\nstages:\n  say: { job: ./human.js }\n"}, nil)
	w.settle()
	w.madeHolding(r1, run)
	w.settle()
	w.pass()
	w.settle()
	child := w.child(run)
	st := w.latest(w.child(child)).State
	if !st.Above.IsAsk || st.Above.Ask != nil || st.Above.AskAbout != nil || st.Machine != "asks" {
		t.Fatalf("the machine run as the ask has %+v", st)
	}
	for _, msg := range w.inboxOf(r1) {
		if msg.Job != nil && msg.Job.Path == "./step.js" {
			t.Fatalf("%s went to the runner", msg.Job.Name)
		}
	}
}

// An ask that is a machine is the words of the machine that declares it, for
// whoever it runs. Its own ask stands, and what it holds is held, not denied.
func Test_ctl_owners_ask_machine_for_a_made_machine_keeps_its_own_ask(t *testing.T) {
	w := newWorld(t)
	r1 := w.newRunner()
	run := w.submit(`
		flow: maker
		ask: { machine: ./asks.yaml }
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}" } }
	`, map[string]string{"make.js": "return 1", "human.js": "return {}",
		"asks.yaml": "flow: asks\nask: ./human.js\nstages:\n  say: { job: { script: ./human.js, approve: true } }\n"}, nil)
	w.settle()
	w.madeHolding(r1, run)
	w.settle()
	w.pass()
	w.settle()
	asks := w.child(w.child(run))
	st := w.latest(asks).State
	if st.Above.Made || !st.Above.IsAsk || st.Status != instance.Running {
		t.Fatalf("the machine run as the ask is %s: %+v, %+v", st.Status, st.Above, st.Error)
	}
	if msg := w.inboxOf(r1)[w.jobName(r1, instance.AskName)]; msg == nil || msg.Job.Path != "./human.js" || msg.Instance != asks {
		t.Fatalf("the ask of the machine run as the ask is %+v", msg)
	}
}

// A job's name in an answer costs what its bytes cost: however many jobs name
// a place, the definition is read for it once, and not at all for a machine
// that has no ask of its own.
func Test_ctl_asks_of_a_definition_are_read_once_a_step(t *testing.T) {
	agent, _ := machine.TypeOf("agent")
	counted := &askCounter{Type: agent}
	machine.Register(counted)
	t.Cleanup(func() { machine.Register(agent) })

	w := newWorld(t)
	run, r1 := w.listed()
	w.lieNext("after a list", "many asks", "100", "open.s.ask")
	w.result(r1, run, "t", oneTool)
	w.settle()
	if st := w.latest(run).State; len(st.Jobs) < 100 || counted.asks != 1 {
		t.Fatalf("for %d jobs, the asks were read %d times", len(st.Jobs), counted.asks)
	}

	counted.asks = 0
	w = newWorld(t)
	r1 = w.newRunner()
	holder := w.holder(r1)
	w.lieNext("many asks", "100", "open.s.ask")
	ref, err := w.draft(lister, listerFiles).Store(ctx, w.ctrl.Store, "ctl", holder)
	if err != nil {
		t.Fatal(err)
	}
	w.result(r1, holder, "make", instance.Result{Status: instance.Done, Output: ref.Value()})
	w.pass()
	w.pass()
	w.settle()
	if st := w.latest(w.child(holder)).State; len(st.Jobs) < 100 || counted.asks != 0 {
		t.Fatalf("for %d jobs of a made machine, the asks were read %d times", len(st.Jobs), counted.asks)
	}
}

// askCounter is a machine type that counts how often its asks are read.
type askCounter struct {
	machine.Type
	asks int
}

func (c *askCounter) Lists(def *machine.Definition) bool {
	return c.Type.(machine.Reader).Lists(def)
}

func (c *askCounter) Asks(def *machine.Definition) map[string]*machine.JobSpec {
	c.asks++
	return c.Type.(machine.Reader).Asks(def)
}

// A client starts a run at a location: the run's definition is the file
// recorded there, copied into the run's own record, whatever else the start
// names. A start that names a definition in another run's record, and no
// location, starts nothing: the definition is not read, and the controller
// is not given it to read as an owner's.
func Test_ctl_client_start_names_a_location(t *testing.T) {
	w := newWorld(t)
	w.ctrl.Command = []string{"/nonexistent/no-step-process"}
	w.newRunner()
	at, _ := w.place(w.client, kept, nil)
	holder := w.startAt(w.client, at, client.Start{})
	w.settle()
	ref, err := w.draft(plain, map[string]string{"step.js": "return 1"}).Store(ctx, w.ctrl.Store, "ctl", holder)
	if err != nil {
		t.Fatal(err)
	}
	start := func(s *instance.Start) (string, *instance.Receipt) {
		t.Helper()
		run, err := w.client.NewRun()
		if err != nil {
			t.Fatal(err)
		}
		_, id, _ := strings.Cut(run, "/")
		s.Seed = make([]byte, 32)
		w.leave(w.c1, id, &instance.Message{Kind: instance.KindStart, Instance: run, Start: s})
		w.settle()
		return run, w.receipt(id)
	}
	run, r := start(&instance.Start{Location: at, Definition: ref})
	if r == nil || !r.Taken || w.head(run) == nil {
		t.Fatalf("the start of %s has the head %+v and the receipt %+v", at, w.head(run), r)
	}
	if st := w.latest(run).State; st.Machine != "kept" || st.Location != at || !strings.HasPrefix(st.Definition.At, store.Blobs("ctl", run)+"/") {
		t.Fatalf("the run at %s is of %q at %s, from the definition %s", at, st.Machine, st.Location, st.Definition.At)
	}
	run, r = start(&instance.Start{Definition: ref})
	if w.head(run) != nil || r == nil || r.Taken || r.Refused == "" {
		t.Fatalf("the start of a definition in another record has the head %+v and the receipt %+v", w.head(run), r)
	}
}

// lister is an owner's agent with an entry that lists tools from elsewhere,
// and a set with an ask.
const lister = `
	agent: lister
	model: { name: m, service: anthropic, address: "http://127.0.0.1:1", secret: KEY }
	ask: ./ok.js
	prompt: hello
	tools:
	  t:
	    list: { script: ./step.js }
	    job: ./step.js
	open:
	  s: { include: ./part.yaml }
`

var listerFiles = map[string]string{"step.js": "return 1", "ok.js": "return {}", "own.js": "return {}",
	"part.yaml": "does: Things.\nask: ./own.js\ntools:\n  u: { does: U, job: ./step.js }\n"}

// listed runs lister as far as the runner that has its list, and gives a tool.
func (w *world) listed() (run string, r1 *identity.Identity) {
	w.t.Helper()
	r1 = w.newRunner()
	w.secret("KEY", "k", "/")
	run = w.submit(lister, listerFiles, nil)
	w.settle()
	return run, r1
}

var oneTool = instance.Result{Status: instance.Done, Output: []any{
	map[string]any{"name": "read", "does": "Read", "args": map[string]any{"type": "object"}}}}

// A list brings shapes from outside the definition, so what follows it is
// taken in a process of its own. The first step holds nothing from outside.
func Test_ctl_listed_agent_steps_after_its_first_are_taken_in_a_process(t *testing.T) {
	w := newWorld(t)
	w.ctrl.Command = []string{"/nonexistent/no-step-process"}
	run, r1 := w.listed()
	if st := w.latest(run).State; st.Status != instance.Running {
		t.Fatalf("after its first step, the agent is %+v", st)
	}
	w.result(r1, run, "t", oneTool)
	w.settle()
	if st := w.latest(run).State; st.Status != instance.Failed || !strings.Contains(st.Error.Error, "the step's process could not be started") {
		t.Fatalf("with no process, the agent is %s: %+v", st.Status, st.Error)
	}
	w = newWorld(t)
	run, r1 = w.listed()
	w.result(r1, run, "t", oneTool)
	w.settle()
	turn := w.inboxOf(r1)[w.jobName(r1, "turn")]
	if turn == nil || !strings.Contains(fmt.Sprint(turn.Job.Input), "t_read") {
		t.Fatalf("the turn is %+v", turn)
	}
}

// jobName gives the name in a runner's inbox of a job.
func (w *world) jobName(r1 *identity.Identity, job string) string {
	w.t.Helper()
	for name, msg := range w.inboxOf(r1) {
		if msg.Job != nil && msg.Job.Name == job {
			return name
		}
	}
	return ""
}

// A step that passes a limit fails that agent, with the reason, and nothing
// else.
func Test_ctl_listed_step_past_a_limit_fails_that_agent(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run, r1 := w.listed()
	w.ctrl.Command, w.ctrl.StepTime = []string{os.Args[0], stepThatHangs, t.TempDir() + "/started"}, 100*time.Millisecond
	w.result(r1, run, "t", oneTool)
	w.settle()
	if st := w.latest(run).State; st.Status != instance.Failed || !strings.Contains(st.Error.Error, "may take") {
		t.Fatalf("the agent is %s: %+v", st.Status, st.Error)
	}
	w.done(owner)
}

// A shape from outside that costs too much to read costs that agent, and the
// controller goes on with the others.
func Test_ctl_listed_shape_past_a_limit_fails_that_agent(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run, r1 := w.listed()
	w.ctrl.StepTime = 500 * time.Millisecond
	huge := map[string]any{"pattern": strings.Repeat("(a{1000})", 3000)}
	w.result(r1, run, "t", instance.Result{Status: instance.Done, Output: []any{map[string]any{"name": "read",
		"args": map[string]any{"type": "object", "allOf": []any{huge, huge, huge, huge, huge, huge, huge, huge}}}}})
	w.settle()
	st := w.latest(run).State
	if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "may take") && !strings.Contains(st.Error.Error, "without an answer") {
		t.Fatalf("the agent is %s: %+v", st.Status, st.Error)
	}
	w.done(owner)
}

// A machine stored by a job goes past what a define checks. What it costs to
// read falls on its step, which fails that machine, with the reason.
func Test_ctl_made_definition_past_a_limit_fails_that_machine(t *testing.T) {
	var refs []string
	for i := range 40 {
		refs = append(refs, fmt.Sprintf(`"d%d":{"allOf":[{"$ref":"#/$defs/d%d"},{"$ref":"#/$defs/d%d"}]}`, i, i+1, i+1))
	}
	shape := func(text string) func(map[string]any) {
		return func(def map[string]any) {
			var input map[string]any
			if err := json.Unmarshal([]byte(text), &input); err != nil {
				t.Fatal(err)
			}
			def["input"] = input
		}
	}
	for name, edit := range map[string]func(map[string]any){
		"$refs that each name the next twice": shape(`{"type":"object","properties":{"a":{"default":1,"$ref":"#/$defs/d0"}},"$defs":{` + strings.Join(refs, ",") + `,"d40":{}}}`),
		"3 MB of brackets": func(def map[string]any) {
			def["body"].(map[string]any)["output"] = "${" + strings.Repeat("(", 1500000) + "1" + strings.Repeat(")", 1500000) + "}"
		},
	} {
		w := newWorld(t)
		r1 := w.newRunner()
		run := w.holder(r1)
		w.ctrl.StepTime = 3 * time.Second
		draft := w.draft(plain, map[string]string{"step.js": "return 1"})
		ref, err := draft.Store(ctx, w.ctrl.Store, "ctl", run)
		if err != nil {
			t.Fatal(err)
		}
		def, err := machine.Load(ctx, w.ctrl.Store, ref)
		if err != nil {
			t.Fatal(err)
		}
		written, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		var open map[string]any
		if err := json.Unmarshal(written, &open); err != nil {
			t.Fatal(err)
		}
		edit(open)
		if written, err = json.Marshal(open); err != nil {
			t.Fatal(err)
		}
		ref, err = blob.Write(ctx, w.ctrl.Store, store.Blob("ctl", run, blob.NewName()), blob.Definition, blob.NewKey(), bytes.NewReader(written))
		if err != nil {
			t.Fatal(err)
		}
		w.result(r1, run, "make", instance.Result{Status: instance.Done, Output: ref.Value()})
		w.settle()
		st := w.latest(w.child(run)).State
		if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "may take") && !strings.Contains(st.Error.Error, "without an answer") {
			t.Errorf("%s: the machine is %s: %+v", name, st.Status, st.Error)
		}
	}
}

// An owner's agent takes its steps in a process, and its asks are still its
// definition's words: an answer names only a place, and what is written there
// is read from the definition and filled here. A place the definition does
// not write gives the agent's own ask. The process can pick another place of
// the definition, as "open.s.ask" here, for a tool outside that part: that is
// within what the specs say such a process can do (Permissions), and is no
// more than it could do by dropping the approval.
func Test_ctl_answer_cannot_write_a_listed_agents_ask(t *testing.T) {
	for at, want := range map[string]string{"x": "./ok.js", "open.s.ask": "./own.js"} {
		w := newWorld(t)
		run, r1 := w.listed()
		w.lieNext("after a list", "fills", at)
		w.result(r1, run, "t", oneTool)
		w.settle()
		ask := w.inboxOf(r1)[w.jobName(r1, instance.AskName)]
		if ask == nil || ask.Job.Path != want {
			t.Errorf("with the place %s, the ask is %+v", at, ask)
		}
	}
}

// A definition's limit on jobs and its deny list are read from the
// definition, and the answer has no say in either.
func Test_ctl_answer_cannot_pass_what_its_definition_limits(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.holder(r1)
	w.lieNext("two jobs")
	w.made(r1, run, `
		flow: plain
		limits: { jobs: 1 }
		deny: [ "net:a.example" ]
		stages:
		  a: { job: ./step.js }
	`)
	w.pass()
	w.pass()
	var sent []string
	for name, msg := range w.inboxOf(r1) {
		if msg.Job == nil || msg.Job.Instance != w.child(run) {
			continue
		}
		sent = append(sent, name)
		if !slices.Contains(msg.Job.Layers[0].Deny, "net:a.example") {
			t.Fatalf("the job went with %+v", msg.Job.Layers[0])
		}
	}
	if len(sent) != 1 {
		t.Fatalf("the machine sent %v", sent)
	}
}

// The state an answer gives is not the state: a machine that ended by
// saying so has not ended.
func Test_ctl_answer_cannot_end_its_machine_by_saying_so(t *testing.T) {
	w := newWorld(t)
	run := w.lied(plain, "ended")
	w.settle()
	if st := w.latest(w.child(run)).State; st.Status != instance.Running || st.Machine != "plain" {
		t.Fatalf("the state is %s %s", st.Status, st.Machine)
	}
}

// Whatever comes back, the controller stays up, and takes it no further
// than any definition could.
func Test_ctl_answer_holding_nothing_ends_no_controller(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run := w.lied(plain, "many")
	w.settle()
	if head := w.head(run); head == nil {
		t.Fatal("no head")
	}
	w.done(owner)
}

// The strangest values a request can hold are a definition's to give, and
// a job or a machine that cannot use them fails by itself.
func Test_ctl_answer_with_strange_values_ends_no_controller(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run := w.lied(plain, "wild")
	w.settle()
	if head := w.head(w.child(run)); head == nil {
		t.Fatal("no head")
	}
	w.done(owner)
}

// A reference that names nothing goes up as any other that opens nothing
// does.
func Test_ctl_answer_ending_with_a_reference_to_nothing_ends_as_any_other(t *testing.T) {
	w := newWorld(t)
	run := w.lied(plain, "ended well")
	w.settle()
	if head := w.head(w.child(run)); head == nil || head.Status != instance.Done {
		t.Fatalf("the machine is %+v", head)
	}
	if head := w.head(run); head == nil || head.Status == instance.Running && len(w.list(store.Inbox("ctl"))) != 0 {
		t.Fatalf("the machine above is %+v", head)
	}
}

// A stored step with hostile content is carried out by a controller started
// afresh, stopped after any write, and it goes on.
func Test_ctl_stored_step_with_hostile_content_ends_no_controller(t *testing.T) {
	for n := int64(0); ; n++ {
		w := newWorld(t)
		owner := w.submit(family, nil, nil)
		w.settle()
		run := w.lied(plain, "many")
		s := &stopping{Store: w.ctrl.Store}
		s.left.Store(n)
		inner := w.ctrl.Store
		command := w.ctrl.Command
		w.afresh(s)
		w.ctrl.Command = command
		for range 8 {
			if _, err := w.ctrl.Pass(ctx); err != nil {
				break
			}
		}
		w.afresh(inner)
		w.ctrl.Command = command
		for range 3 {
			w.settle()
			w.tick(controller.StepTime + time.Minute)
		}
		if head := w.head(run); head == nil {
			t.Fatalf("stopped after %d writes the run has no head", n)
		}
		w.done(owner)
		if !s.stopped.Load() {
			if n < 8 {
				t.Fatalf("the workload made only %d writes", n)
			}
			return
		}
	}
}

// A name a step gives goes into names in the store, so it is one a
// definition could hold, and any other fails that machine.
func Test_ctl_answer_with_a_name_no_definition_holds_fails_its_machine(t *testing.T) {
	long := strings.Repeat("a", 101)
	for _, c := range []struct{ kind, name string }{
		{"job", ""}, {"job", "ask"}, {"job", "ask#2"}, {"job", "../../secrets/x"}, {"job", "a/b"}, {"job", "x@2"}, {"job", "x#2"}, {"job", "x.cancel"}, {"job", long},
		{"timer", "a/b"}, {"timer", ".."}, {"timer", long}, {"ticket", long}, {"id", long},
	} {
		w := newWorld(t)
		owner := w.submit(family, nil, nil)
		run := w.lied(plain, "named", c.kind, c.name)
		w.settle()
		st := w.latest(w.child(run)).State
		if st.Status != instance.Failed || st.Error == nil || !strings.Contains(st.Error.Error, "no definition holds") {
			t.Fatalf("a %s named %.20q: the machine is %s, %+v", c.kind, c.name, st.Status, st.Error)
		}
		w.done(owner)
	}
}

// A made definition's own name goes into a head, which is small, and only
// the step's process has checked it.
func Test_ctl_made_definition_with_a_name_past_the_limit_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	run := w.lied("flow: "+strings.Repeat("a", 101)+"\nstages:\n  a: { job: ./step.js }\n", "state")
	w.settle()
	if head := w.head(w.child(run)); head == nil || head.Status != instance.Failed || head.Machine != "" {
		t.Fatalf("the machine is %+v", head)
	}
}

// The names the three types give are all names a definition holds.
func Test_ctl_names_of_the_types_pass_the_check_of_an_answer(t *testing.T) {
	for _, c := range []struct{ kind, name string }{
		{"job", "get_title"}, {"job", "get_title.3"}, {"job", "shelf.1"}, {"job", "a-b_c"}, {"job", strings.Repeat("a", 100)},
		{"timer", "too_long.waiting"}, {"ticket", "verdict"}, {"id", "waiting"},
	} {
		w := newWorld(t)
		run := w.lied(plain, "named", c.kind, c.name)
		w.settle()
		if st := w.latest(w.child(run)).State; st.Status != instance.Running {
			t.Fatalf("a %s named %q: the machine is %s, %+v", c.kind, c.name, st.Status, st.Error)
		}
	}
}

// An answer past 4 MiB is not taken.
func Test_ctl_made_step_answer_past_a_steps_limit_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run := w.lied(plain, "large")
	w.settle()
	if got := w.outcome(run); !strings.Contains(got, "could not be read") {
		t.Fatal(got)
	}
	w.done(owner)
}

// An answer within the limit that would take the state past it fails its
// machine too: a state that grew a step at a time is read on every pass.
func Test_ctl_made_step_state_past_a_steps_limit_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	run := w.lied(plain, "grown")
	w.settle()
	st := w.latest(w.child(run)).State
	if st.Status != instance.Running {
		t.Fatalf("the first step failed its machine: %+v", st.Error)
	}
	w.tick(time.Second)
	w.put(w.c1, store.Stamp(w.clock())+"-event", &instance.Message{Kind: instance.KindEvent, Instance: w.child(run), Event: &machine.Event{Name: "go"}},
		w.ctl.Card, w.clock().Add(time.Hour))
	w.settle()
	st = w.latest(w.child(run)).State
	if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "MiB") {
		t.Fatalf("the machine is %s: %+v", st.Status, st.Error)
	}
}

// A value nested as deep as JSON goes could not be written in a step, and
// the step would be taken for ever.
func Test_ctl_answer_nested_too_deep_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	owner := w.submit(family, nil, nil)
	run := w.lied(plain, "deep")
	w.settle()
	st := w.latest(w.child(run)).State
	if st.Status != instance.Failed || !strings.Contains(st.Error.Error, "deep") {
		t.Fatalf("the machine is %s: %+v", st.Status, st.Error)
	}
	w.done(owner)
}

// lagging is a store whose first listing takes a while.
type lagging struct {
	store.Store
	once sync.Once
	lag  time.Duration
}

func (s *lagging) List(ctx context.Context, under string) ([]store.Listed, error) {
	s.once.Do(func() { time.Sleep(s.lag) })
	return s.Store.List(ctx, under)
}

// A step that would not finish in the time the host has left waits for the
// next call, and that costs it none of its tries.
func Test_ctl_made_step_waits_for_a_call_with_time_for_it(t *testing.T) {
	w := newWorld(t)
	run := w.startMade(plain)
	w.pass() // the machine above asks for the machine below
	file := t.TempDir() + "/started"
	inner := w.ctrl.Store
	w.afresh(&lagging{Store: inner, lag: 300 * time.Millisecond})
	w.ctrl.Command = []string{os.Args[0], stepThatHangs, file}
	w.ctrl.StepTime = time.Second
	// The pass begins with time for a step, and the store takes it from it.
	c, cancel := context.WithTimeout(ctx, w.ctrl.StepTime+5*time.Second+200*time.Millisecond)
	defer cancel()
	next, err := w.ctrl.Pass(c)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Equal(w.clock()) {
		t.Fatalf("the pass says the next thing is due at %s", next)
	}
	if got := started(file); got != 0 {
		t.Fatalf("%d processes were started", got)
	}
	for _, name := range w.list("ctl/records") {
		if strings.HasSuffix(name, "/tried") {
			t.Fatalf("the step spent a try: %s", name)
		}
	}
	if head := w.head(w.child(run)); head != nil {
		t.Fatalf("the machine has a head: %+v", head)
	}
	// The next call has the time, and takes the step.
	w.afresh(inner)
	w.settle()
	if head := w.head(w.child(run)); head == nil {
		t.Fatal("the step was not taken")
	}
}

// A host whose every call is too short for a step says so: the machine
// fails with the reason, and waits for nothing.
func Test_ctl_made_step_too_long_for_any_call_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	run := w.startMade(past)
	w.pass()
	w.ctrl.StepTime = time.Hour
	c, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if _, err := w.ctrl.Pass(c); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if got := w.outcome(run); !strings.Contains(got, "host gives a call") {
		t.Fatal(got)
	}
}

// taking is a store in which a name is taken a moment before it is written.
type taking struct {
	store.Store
	at    string
	value []byte
}

func (s *taking) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	if name == s.at {
		if _, err := s.Store.Write(ctx, name, s.value, store.OnlyIfNew); err != nil {
			return false, err
		}
	}
	return s.Store.Write(ctx, name, value, how)
}

// A message that finds its name taken by another is noticed: what it was
// for fails with the reason, and nothing waits for what will never come.
func Test_ctl_message_finds_its_name_taken(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.holder(r1)
	// The start of the machine below comes next. Its name is taken by
	// another of the controller's own messages.
	at := store.Message("ctl", "ctl", store.LinkMessage(run, 2, 1, "start"))
	other := &instance.Message{Kind: instance.KindEvent, Instance: run, From: "nobody", Event: &machine.Event{Name: "x"}}
	data, err := w.ctl.Write(at, w.clock(), time.Time{}, other, w.ctl.Card)
	if err != nil {
		t.Fatal(err)
	}
	inner := w.ctrl.Store
	w.afresh(&taking{Store: inner, at: at, value: data})
	w.made(r1, run, plain)
	w.settle()
	if st := w.latest(run).State; st.Status == instance.Running || !strings.Contains(st.Error.Error, "taken") {
		t.Fatalf("the machine is %s: %+v", st.Status, st.Error)
	}
}

// A carry-out done again finds its own messages under their names, and that
// is no fault.
func Test_ctl_carry_out_done_again_finds_only_its_own_messages(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.holder(r1)
	w.made(r1, run, plain)
	s := &stopping{Store: w.ctrl.Store}
	for n := int64(1); n < 12; n++ {
		s.left.Store(n)
		s.stopped.Store(false)
		inner := w.ctrl.Store
		w.afresh(s)
		w.ctrl.Pass(ctx)
		w.afresh(inner)
	}
	w.ctrl.Command = []string{os.Args[0], stepThatHangs, t.TempDir() + "/started"}
	w.ctrl.StepTime = time.Second
	w.settle()
	if st := w.latest(run).State; st.Status == instance.Failed && strings.Contains(st.Error.Error, "taken") {
		t.Fatalf("the machine failed: %+v", st.Error)
	}
}
