package controller_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/controller"
	_ "github.com/zummed/corbie/flow"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// stopping is a store that stops answering after a number of writes, as a
// controller's store does when the controller is stopped: that write and
// everything after it does not happen.
type stopping struct {
	store.Store
	left    atomic.Int64
	stopped atomic.Bool
}

func (s *stopping) spend() error {
	if s.stopped.Load() || s.left.Add(-1) < 0 {
		s.stopped.Store(true)
		return fmt.Errorf("%w: the controller was stopped", store.ErrUnreachable)
	}
	return nil
}

func (s *stopping) check() error {
	if s.stopped.Load() {
		return fmt.Errorf("%w: the controller was stopped", store.ErrUnreachable)
	}
	return nil
}

func (s *stopping) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if err := s.check(); err != nil {
		return nil, store.None, err
	}
	return s.Store.Read(ctx, name)
}

func (s *stopping) List(ctx context.Context, under string) ([]string, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	return s.Store.List(ctx, under)
}

func (s *stopping) First(ctx context.Context, under string, n int) ([]string, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	return s.Store.First(ctx, under, n)
}

func (s *stopping) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	if err := s.spend(); err != nil {
		return false, err
	}
	return s.Store.Write(ctx, name, value, how)
}

func (s *stopping) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := s.spend(); err != nil {
		return false, err
	}
	return s.Store.WriteFrom(ctx, name, r)
}

func (s *stopping) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := s.spend(); err != nil {
		return false, err
	}
	return s.Store.Copy(ctx, from, to)
}

func (s *stopping) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	if err := s.spend(); err != nil {
		return false, err
	}
	return s.Store.Delete(ctx, name, how)
}

const family = `
	statechart: parent
	initial: working
	states:
	  working:
	    job:
	      machine:
	        statechart: child
	        initial: first
	        states:
	          first:
	            entry: { send: { to: parent, event: hello, data: 1 } }
	            on: { ack: second }
	          second:
	            after: { 1s: last }
	            entry: { send: { to: parent, event: waiting } }
	          last: { final: true, output: { from: child } }
	    on:
	      hello: { send: { to: working, event: ack } }
	    done: finished
	  finished: { final: true, output: "${states.working.output}" }
`

// story is what happened in a store, told so that two runs of one
// workload can be compared: for each machine, each step's events and what
// it asked for.
func (w *world) story() string {
	var b strings.Builder
	heads := append(w.list(store.Running("ctl")), w.list(store.Finished("ctl"))...)
	byMachine := map[string]string{}
	for _, at := range heads {
		inst := store.InstanceOf("ctl", at)
		if head := w.head(inst); head != nil {
			byMachine[head.Machine] = inst
		}
	}
	for _, machine := range []string{"parent", "child"} {
		inst, ok := byMachine[machine]
		if !ok {
			fmt.Fprintf(&b, "%s: no instance\n", machine)
			continue
		}
		head := w.head(inst)
		fmt.Fprintf(&b, "%s: %s at step %d\n", machine, head.Status, head.Step)
		for _, step := range w.steps(inst) {
			fmt.Fprintf(&b, "  step %d:", step.Number)
			for _, event := range step.Events {
				fmt.Fprintf(&b, " <-%s", event.Name)
			}
			for _, ask := range step.Asks {
				if ask.Kind == instance.AskReceipt || (ask.Kind == instance.AskWake && (ask.What == "trim" || ask.What == "remove")) {
					continue
				}
				fmt.Fprintf(&b, " ->%s", ask.Kind)
				if ask.Event != nil {
					fmt.Fprintf(&b, "(%s)", ask.Event.Name)
				}
			}
			fmt.Fprintf(&b, " %s\n", step.State.Status)
		}
	}
	left := append(w.list(store.Inbox("ctl")), w.wakes()...)
	fmt.Fprintf(&b, "left waiting: %d %v\n", len(left), left)
	return b.String()
}

// play runs the family to its end, with the controller stopped after a
// number of writes and a fresh one carrying on. It says whether the stop
// ever happened.
func play(t *testing.T, stopAfter int64) (string, bool) {
	w := newWorld(t)
	w.submit(family, nil, nil)
	inner := w.ctrl.Store
	s := &stopping{Store: inner}
	s.left.Store(stopAfter)
	w.ctrl = controller.New(w.ctl, s)
	w.ctrl.Now = w.clock
	for range 12 {
		if _, err := w.ctrl.Pass(ctx); err != nil {
			break
		}
		w.tick(400 * time.Millisecond)
	}
	// A controller that has never run before, with only its key and the
	// store's address.
	w.ctrl = controller.New(w.ctl, inner)
	w.ctrl.Now = w.clock
	for range 8 {
		w.settle()
		w.tick(500 * time.Millisecond)
	}
	return w.story(), s.stopped.Load()
}

// A controller stopped after any single write leaves the same messages and
// wakes waiting, and the next pass repeats only what is harmless: every
// such run ends with the same steps as a run never stopped.
func TestStoppedAfterEveryWrite(t *testing.T) {
	want, _ := play(t, 1<<40)
	if !strings.Contains(want, "parent: done") || !strings.Contains(want, "child: done") || !strings.Contains(want, "left waiting: 0") {
		t.Fatalf("the run never stopped did not end well:\n%s", want)
	}
	for n := int64(0); ; n++ {
		got, stopped := play(t, n)
		if !stopped {
			if n < 10 {
				t.Fatalf("the workload made only %d writes", n)
			}
			return
		}
		if got != want {
			t.Fatalf("stopped after %d writes, the run went otherwise.\nstopped:\n%s\nnever stopped:\n%s", n, got, want)
		}
	}
}

// Two copies of the controller take the same steps at the same moment: one
// entry has each number, both finish without error, and the run ends as it
// would have with one.
func TestTwoCopiesAtOnce(t *testing.T) {
	want, _ := play(t, 1<<40)
	for round := range 20 {
		w := newWorld(t)
		w.submit(family, nil, nil)
		other := controller.New(w.ctl, w.ctrl.Store)
		other.Now = w.clock
		for range 10 {
			done := make(chan error, 2)
			for _, c := range []*controller.Controller{w.ctrl, other} {
				go func() {
					_, err := c.Pass(ctx)
					done <- err
				}()
			}
			for range 2 {
				if err := <-done; err != nil {
					t.Fatalf("round %d: a copy's pass failed: %v", round, err)
				}
			}
			w.tick(300 * time.Millisecond)
		}
		if got := w.story(); got != want {
			t.Fatalf("round %d: with two copies the run went otherwise.\ntwo copies:\n%s\none:\n%s", round, got, want)
		}
	}
}

// cutShort is a store that ends a pass after a number of writes, as a
// host's time limit does: the pass's context is over, and what it had not
// done is left.
type cutShort struct {
	store.Store
	left atomic.Int64
	end  context.CancelFunc
}

func (s *cutShort) spend() {
	if s.left.Add(-1) < 0 {
		s.end()
	}
}

func (s *cutShort) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	s.spend()
	return s.Store.Write(ctx, name, value, how)
}

func (s *cutShort) Copy(ctx context.Context, from, to string) (bool, error) {
	s.spend()
	return s.Store.Copy(ctx, from, to)
}

// passes runs passes that each end after a number of writes, until the run
// has ended, and gives how many it took. The run asks for many things in
// one step: more than one such pass can carry out.
func passes(t *testing.T, w *world, run string, writes int64) int {
	t.Helper()
	inner := w.ctrl.Store
	for n := 1; n <= 400; n++ {
		limited, end := context.WithCancel(ctx)
		s := &cutShort{Store: inner, end: end}
		s.left.Store(writes)
		w.ctrl = controller.New(w.ctl, s)
		w.ctrl.Now = w.clock
		w.ctrl.Pass(limited)
		end()
		if head := w.head(run); head != nil && head.Status != instance.Running {
			if head.Status != instance.Done {
				t.Fatalf("the run ended %s: %+v", head.Status, w.latest(run).State.Error)
			}
			return n
		}
		w.tick(100 * time.Millisecond)
	}
	t.Fatalf("after 400 passes of %d writes each the run has not ended", writes)
	return 0
}

const many = 300

func manyItems() map[string]any {
	items := make([]any, many)
	for i := range items {
		items[i] = float64(i)
	}
	return map[string]any{"items": items}
}

func Test_ctl_carry_out_larger_than_a_pass_finishes(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		flow: wide
		input: [items]
		stages:
		  kids:
		    each: "${input.items}"
		    job:
		      machine:
		        statechart: kid
		        initial: over
		        states:
		          over: { final: true, output: "${input}" }
		      input: "${item}"
		output: "${stages.kids.output}"
	`, nil, manyItems())
	// One step asks for three hundred children, and a pass ends after a
	// hundred and fifty writes.
	w.noted()
	n := passes(t, w, run, 150)
	if got, _ := w.latest(run).State.Output.([]any); len(got) != many {
		t.Fatalf("after %d passes the run gave %d results", n, len(got))
	}
	// The note of how far a carry-out has got is written as everything a
	// role writes is: never whatever is there.
	notes := 0
	for _, op := range w.noted() {
		if op.Name == store.Carried("ctl", run) && op.Act != store.ActRead && op.Act != store.ActDelete {
			notes++
			if op.How == store.Overwrite {
				t.Fatalf("the note of how far the carry-out had got was overwritten")
			}
		}
	}
	if notes < 2 {
		t.Fatalf("how far the carry-out had got was noted %d times", notes)
	}
}

func Test_role_long_carry_out_finishes_over_several_passes(t *testing.T) {
	w := newWorld(t)
	w.enrol(w.party("r1", identity.Runner), permission.Layer{Allow: []string{"all"}}, nil)
	run := w.submit(`
		flow: wide
		input: [items]
		stages:
		  jobs: { each: "${input.items}", job: { script: ./work.js, input: "${item}" } }
	`, map[string]string{"work.js": "return input"}, manyItems())
	// One step asks for three hundred jobs. Each pass takes the sending
	// further, and none begins it again: every job is in the runner's
	// inbox after a few passes, though no pass could send half of them.
	inner := w.ctrl.Store
	for n := 1; ; n++ {
		limited, end := context.WithCancel(ctx)
		s := &cutShort{Store: inner, end: end}
		s.left.Store(200)
		w.ctrl = controller.New(w.ctl, s)
		w.ctrl.Now = w.clock
		w.ctrl.Pass(limited)
		end()
		if sent := len(w.list(store.Inbox("r1"))); sent == many {
			if head := w.head(run); head == nil || head.Step != 1 {
				t.Fatalf("every job is sent and the head is %+v", head)
			}
			break
		} else if n == 20 {
			t.Fatalf("after %d passes %d of %d jobs are sent", n, sent, many)
		}
	}
}

// keeps is a store that cannot delete one name: the controller is stopped
// there, with every other delete it makes side by side done.
type keeps struct {
	store.Store
	name func(string) bool
}

func (k keeps) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	if k.name(name) {
		return false, fmt.Errorf("%w: the controller was stopped", store.ErrUnreachable)
	}
	return k.Store.Delete(ctx, name, how)
}

// A step that takes a job's result no longer wants the wake for the job's
// due time, and deletes both. Stopped with the result deleted and the wake
// not, nothing would lead a pass back to the step: so the wake goes first.
func TestStoppedBetweenDeletesLeavesNoWake(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	if due := w.wakes(); len(due) != 1 {
		t.Fatalf("the wakes are %v", due)
	}
	w.result(r1, run, "working", instance.Result{Attempt: 1, Status: instance.Done, Output: "in time"})
	inner := w.ctrl.Store
	w.afresh(keeps{inner, func(name string) bool { return strings.Contains(name, "/due.working") }})
	if _, err := w.ctrl.Pass(ctx); err == nil {
		t.Fatal("a pass that was stopped ended as if it had done everything")
	}
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("the run is %+v", head)
	}
	w.afresh(inner)
	w.settle()
	if left := append(w.list(store.Inbox("ctl")), w.wakes()...); len(left) != 0 {
		t.Fatalf("stopped between the deletes of what a step used, this is left: %v", left)
	}
}

// early is a store through which a copy reads its inbox a moment too
// early: once it has read a message, something else happens before it
// goes on. It counts what the copy then writes and deletes.
type early struct {
	store.Store
	once   sync.Once
	then   func()
	writes atomic.Int64
	names  sync.Map
}

func (e *early) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	data, mk, err := e.Store.Read(ctx, name)
	if strings.Contains(name, "/inbox/") {
		e.once.Do(e.then)
	}
	return data, mk, err
}

func (e *early) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	// The controller's own hourly wake is not part of any step.
	if strings.Contains(name, "/house/") {
		return e.Store.Write(ctx, name, value, how)
	}
	e.names.Store("write "+name, true)
	e.writes.Add(1)
	return e.Store.Write(ctx, name, value, how)
}

func (e *early) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	e.writes.Add(1)
	return e.Store.WriteFrom(ctx, name, r)
}

func (e *early) Copy(ctx context.Context, from, to string) (bool, error) {
	e.writes.Add(1)
	return e.Store.Copy(ctx, from, to)
}

func (e *early) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	e.names.Store("delete "+name, true)
	e.writes.Add(1)
	return e.Store.Delete(ctx, name, how)
}

func Test_ctl_copy_that_read_too_early_finishes_nothing(t *testing.T) {
	w := newWorld(t)
	run := w.submit(family, nil, nil)
	// A second copy reads the start from the inbox, and before it does
	// anything more the first copy takes the step, carries it all out and
	// deletes the start.
	e := &early{Store: w.ctrl.Store}
	e.then = func() { w.pass() }
	other := controller.New(w.ctl, e)
	other.Now = w.clock
	if _, err := other.Pass(ctx); err != nil {
		t.Fatal(err)
	}
	if head := w.head(run); head == nil || head.Step < 1 {
		t.Fatalf("the first copy did not take the step: %+v", head)
	}
	// The second finds the step written, the head up to date and the
	// start gone: it carries nothing out again.
	if n := e.writes.Load(); n != 0 {
		e.names.Range(func(k, _ any) bool { t.Log(k); return true })
		t.Fatalf("the copy that read too early made %d writes and deletes", n)
	}
}
