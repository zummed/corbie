package role_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
	_ "github.com/zummed/corbie/statechart"
)

var ctx = context.Background()

// world is a controller, a runner and a client that trust each other, on a
// stand-in host: the controller and the runner exist only as the copies the
// host starts.
type world struct {
	t      *testing.T
	h      *host
	owner  store.Store
	ctl    *identity.Identity
	r1     *identity.Identity
	c1     *identity.Identity
	client *client.Client

	mu      sync.Mutex
	changes int
	ops     []store.Op
	// answered are the jobs the test has answered in the runner's place.
	answered map[string]bool
}

func newWorld(t *testing.T) *world {
	w := &world{t: t, h: newHost(t), answered: map[string]bool{}}
	w.h.mem.Note = func(op store.Op) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if op.Act != store.ActRead && op.Act != store.ActList && op.Done {
			w.changes++
		}
		w.ops = append(w.ops, op)
	}
	w.owner = w.h.mem.As(store.Party{Kind: store.Owner})
	for name, kind := range map[string]identity.Kind{"ctl": identity.Controller, "r1": identity.Runner, "c1": identity.Client} {
		id, err := identity.New(name, kind)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case identity.Controller:
			w.ctl = id
		case identity.Runner:
			w.r1 = id
		default:
			w.c1 = id
		}
	}
	if err := store.SetUp(ctx, w.owner); err != nil {
		t.Fatal(err)
	}
	for _, id := range []*identity.Identity{w.r1, w.c1} {
		trust := identity.Trust{Card: id.Card, Ceiling: permission.Layer{Allow: []string{"all"}}}
		if err := identity.WriteTrust(ctx, w.owner, w.ctl, trust); err != nil {
			t.Fatal(err)
		}
	}
	w.client = client.New(w.c1, w.ctl.Card, w.h.mem.As(store.Party{Kind: store.Client, Name: "c1", Controller: "ctl"}))
	w.client.Now = w.h.clock
	return w
}

// controller makes a copy's controller: one that has never run before,
// with its key and its way to the store.
func (w *world) controller(c *copyOf) role.Pass {
	made := controller.New(w.ctl, c.store(store.Controller))
	made.Now = w.h.clock
	return made.Pass
}

// runner makes a copy's runner, whose jobs have the copy's disk for their
// scratch.
func (w *world) runner(c *copyOf) role.Pass {
	made := runner.New(w.r1, runner.Served{Controller: w.ctl.Card, Ceiling: permission.Layer{Allow: []string{"all"}}, Store: c.store(store.Runner)})
	made.Command = []string{os.Args[0], runner.JobArg}
	made.Scratch = c.disk()
	made.Now = w.h.clock
	return made.Pass
}

func (w *world) submit(src string, files map[string]string, input any) string {
	w.t.Helper()
	draft, err := machine.Parse([]byte(dedent(src)), func(path string) ([]byte, error) {
		if contents, ok := files[path]; ok {
			return []byte(dedent(contents)), nil
		}
		return nil, fmt.Errorf("no such file")
	})
	if err != nil {
		w.t.Fatalf("the definition does not pass:\n%v", err)
	}
	run, err := w.client.Start(ctx, draft, client.Start{Input: input})
	if err != nil {
		w.t.Fatal(err)
	}
	return run
}

// changed says how many changes have been made to the store.
func (w *world) changed() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.changes
}

// noted gives what has been done to the store since it was last asked.
func (w *world) noted() []store.Op {
	w.mu.Lock()
	defer w.mu.Unlock()
	ops := w.ops
	w.ops = nil
	return ops
}

func (w *world) list(under string) []string {
	w.t.Helper()
	names, err := w.owner.List(ctx, under)
	if err != nil {
		w.t.Fatal(err)
	}
	return names
}

func (w *world) head(run string) *instance.Head {
	w.t.Helper()
	head, _, err := instance.ReadHead(ctx, w.owner, w.ctl.Card, run)
	if err != nil {
		w.t.Fatal(err)
	}
	return head
}

func (w *world) steps(run string) []*instance.Step {
	w.t.Helper()
	var all []*instance.Step
	for n := 1; ; n++ {
		step, err := instance.ReadStep(ctx, w.owner, w.ctl.Card, w.ctl, run, n)
		if err != nil {
			w.t.Fatal(err)
		}
		if step == nil {
			return all
		}
		all = append(all, step)
	}
}

// play answers the jobs in the runner's inbox as a runner would, without
// running anything: the first attempt at a job named "flaky" fails, and
// every other job is done. A job it has answered before it leaves alone,
// as a runner leaves one whose mark is there, and deletes once the job is
// no longer good.
func (w *world) play() {
	w.t.Helper()
	sender := store.Sender{}
	for _, name := range w.list(store.Inbox("r1")) {
		data, _, _ := w.owner.Read(ctx, name)
		var msg instance.Message
		value, err := identity.Read(data, name, w.ctl.Card, w.r1, &msg)
		if err != nil {
			w.t.Fatalf("%s: %v", name, err)
		}
		switch {
		case !value.Good(w.h.clock()) || msg.Kind != instance.KindJob:
			w.owner.Delete(ctx, name, store.Overwrite)
		case w.answered[name]:
		default:
			job := msg.Job
			result := &instance.Result{Job: job.Name, Attempt: job.Attempt, Status: instance.Done, Output: map[string]any{"attempt": float64(job.Attempt)}}
			if strings.HasPrefix(job.Name, "flaky") && job.Attempt == 1 {
				result.Status, result.Output = machine.StatusError, nil
				result.Error = &machine.Error{Status: machine.StatusError, Error: "not the first time"}
			}
			at := store.Message("ctl", "r1", sender.Next(w.h.clock()))
			signed, err := w.r1.Write(at, w.h.clock(), time.Time{}, &instance.Message{Kind: instance.KindResult, Instance: job.Instance, Result: result}, w.ctl.Card)
			if err != nil {
				w.t.Fatal(err)
			}
			w.owner.Write(ctx, at, signed, store.OnlyIfNew)
			w.owner.Delete(ctx, name, store.Overwrite)
			w.answered[name] = true
		}
	}
}

// work has the host call for passes until nothing more comes of them, and
// then again a second later, for some seconds. The controller is called
// until it has nothing more to do before the runner is; runs is what stands
// for the runner: the test, or a copy of the real one.
func (w *world) work(seconds int, runs func()) {
	w.t.Helper()
	for range seconds {
		for range 30 {
			for range 30 {
				before := w.changed()
				c := w.h.call("ctl", w.controller)
				if c.ended() && w.changed() == before {
					break
				}
			}
			before := w.changed()
			runs()
			if w.changed() == before {
				break
			}
		}
		w.h.tick(time.Second)
	}
}

// outcome is what came of the work in the store, told without regard to
// which step took what: for each machine how it ended, every event it was
// given and everything it asked for, and what is left waiting. A pass that
// stops part way leaves messages to be taken together that an unbroken one
// takes one by one, so the steps may differ where nothing is lost and
// nothing is done twice.
func (w *world) outcome() string {
	var b strings.Builder
	byMachine := map[string]string{}
	for _, at := range append(w.list(store.Running("ctl")), w.list(store.Finished("ctl"))...) {
		inst := store.InstanceOf("ctl", at)
		if head := w.head(inst); head != nil {
			byMachine[head.Machine] = inst
		}
	}
	machines := make([]string, 0, len(byMachine))
	for name := range byMachine {
		machines = append(machines, name)
	}
	slices.Sort(machines)
	for _, name := range machines {
		inst := byMachine[name]
		var given, asked []string
		var last *instance.Step
		for _, step := range w.steps(inst) {
			last = step
			for _, event := range step.Events {
				given = append(given, event.Name)
			}
			for _, ask := range step.Asks {
				if ask.Kind == instance.AskReceipt || (ask.Kind == instance.AskWake && (ask.What == "trim" || ask.What == "remove")) {
					continue
				}
				text := ask.Kind + " " + ask.Name
				if ask.Event != nil {
					text += ask.Event.Name
				}
				asked = append(asked, strings.TrimSpace(text))
			}
		}
		slices.Sort(given)
		slices.Sort(asked)
		fmt.Fprintf(&b, "%s: %s\n  given: %s\n  asked for: %s\n", name, w.head(inst).Status, strings.Join(given, ", "), strings.Join(asked, ", "))
		if last != nil {
			fmt.Fprintf(&b, "  output: %v\n", last.State.Output)
		}
	}
	left := w.waiting()
	fmt.Fprintf(&b, "left waiting: %d %v\n", len(left), left)
	return b.String()
}

// waiting gives what is left in the inboxes and among the wakes, leaving
// out the controller's own wakes for housekeeping.
func (w *world) waiting() []string {
	left := append(w.list(store.Inbox("ctl")), w.list(store.Inbox("r1"))...)
	for _, name := range w.list(store.Wakes("ctl")) {
		if !strings.HasSuffix(name, "/trim") && !strings.HasSuffix(name, "/remove") && !strings.HasSuffix(name, "/house/keep") {
			left = append(left, name)
		}
	}
	return left
}

// dedent takes the common leading tabs off a text written inside a test.
func dedent(text string) string {
	lines := strings.Split(strings.Trim(text, "\n"), "\n")
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, "\t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for i, line := range lines {
		if indent > 0 && len(line) >= indent {
			lines[i] = line[indent:]
		}
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\t", "  ") + "\n"
}

// everything is a run with a job that takes two attempts, a child, events
// both ways and a timer.
const everything = `
	statechart: parent
	initial: working
	states:
	  working:
	    jobs:
	      flaky: { script: ./work.js, attempts: 2 }
	      kid:
	        machine:
	          statechart: child
	          initial: first
	          states:
	            first:
	              entry: { send: { to: parent, event: hello, data: 1 } }
	              on: { ack: second }
	            second:
	              after: { 1s: last }
	            last: { final: true, output: { from: child } }
	    on:
	      hello: { send: { to: kid, event: ack } }
	    done: finished
	  finished: { final: true, output: "${states.working.output}" }
`
