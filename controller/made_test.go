package controller_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	_ "github.com/zummed/corbie/flow"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// stepThatHangs is the argument that makes the test binary a step's process
// that notes its start in a file and never answers.
const stepThatHangs = "step-that-hangs"

func hangs(file string) {
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	fmt.Fprintln(f, os.Getpid())
	f.Close()
	time.Sleep(time.Hour)
}

// stepThatSaysTooMuch is the argument that makes the test binary a step's
// process that announces more than the channel carries.
const stepThatSaysTooMuch = "step-that-says-too-much"

func saysTooMuch() {
	wire, err := role.Open()
	if err != nil {
		os.Exit(2)
	}
	wire.W.Write([]byte{0xff, 0xff, 0xff, 0xff})
	time.Sleep(time.Hour)
}

// soon waits for something to happen.
func soon(t *testing.T, what string, happened func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if happened() {
			return
		}
	}
	t.Fatalf("waiting for %s", what)
}

// started gives how many steps' processes have begun.
func started(file string) int {
	data, _ := os.ReadFile(file)
	return strings.Count(string(data), "\n")
}

const held = `
	flow: maker
	stages:
	  make: { job: ./make.js }
	  go: { job: { machine: "${stages.make.output}" } }
`

// holder runs a machine as far as the job that makes the machine it starts.
func (w *world) holder(r1 *identity.Identity) string {
	w.t.Helper()
	run := w.submit(held, map[string]string{"make.js": "return 1"}, nil)
	w.settle()
	return run
}

// made answers that job, as a runner, with a stored definition.
func (w *world) made(r1 *identity.Identity, run, text string) {
	w.t.Helper()
	ref, err := w.draft(text, map[string]string{"step.js": "return 1"}).Store(ctx, w.ctrl.Store, "ctl", run)
	if err != nil {
		w.t.Fatal(err)
	}
	w.result(r1, run, "make", instance.Result{Status: instance.Done, Output: ref.Value()})
}

// startMade runs a machine as far as the start of the machine its job made
// from the text, and gives the machine above.
func (w *world) startMade(text string) string {
	w.t.Helper()
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	run := w.holder(r1)
	w.made(r1, run, text)
	return run
}

// outcome says how the machine above and the machine below it ended, in how
// many steps below, and what is left waiting.
func (w *world) outcome(run string) string {
	w.t.Helper()
	child := ""
	if latest := w.latest(run); latest != nil {
		for _, step := range w.steps(run) {
			for _, ask := range step.Asks {
				if ask.Child != "" {
					child = ask.Child
				}
			}
		}
	}
	if child == "" {
		return "no machine below"
	}
	last := w.latest(child)
	if last == nil {
		return "no step below"
	}
	left := append(w.list(store.Inbox("ctl")), w.wakes()...)
	why, _, _ := strings.Cut(w.latest(run).State.Error.Error, "block")
	return fmt.Sprintf("%s after %s; below, %d steps, %s: %v; waiting: %d", w.head(run).Status, why,
		len(w.steps(child)), last.State.Status, last.Events[0].Own, len(left))
}

const past = "flow: bad\nstages:\n  a: { job: { script: ./step.js, input: { v: \"${'a'.repeat(2**40)}\" } } }\n"

// A controller stopped after any write of a step past a limit leaves the
// step to be taken again, and one that starts afresh ends the machine once,
// and goes on.
func Test_ctl_made_step_past_a_limit_stopped_after_any_write(t *testing.T) {
	want := ""
	for n := int64(0); ; n++ {
		w := newWorld(t)
		run := w.startMade(past)
		s := &stopping{Store: w.ctrl.Store}
		s.left.Store(n)
		inner := w.ctrl.Store
		w.afresh(s)
		for range 8 {
			if _, err := w.ctrl.Pass(ctx); err != nil {
				break
			}
		}
		w.afresh(inner)
		// Whatever was taken before the stop may be taken once again.
		for range 4 {
			w.settle()
			w.tick(controller.StepTime + time.Minute)
		}
		got := w.outcome(run)
		if want == "" {
			want = got
			if !strings.Contains(want, "failed after go: ") || !strings.Contains(want, "without an answer") || !strings.Contains(want, "1 steps, failed: true; waiting: 0") {
				t.Fatalf("the machine past its limit: %s", want)
			}
		}
		if got != want {
			t.Fatalf("stopped after %d writes: %s, and not stopped: %s", n, got, want)
		}
		if !s.stopped.Load() {
			if n < 8 {
				t.Fatalf("the workload made only %d writes", n)
			}
			return
		}
	}
}

// A step is taken twice when no answer comes because the controller stopped.
// The third time, the machine fails: a step's process can take its controller
// with it, and a machine that does must not do so for ever.
func Test_ctl_made_step_lost_twice_fails_its_machine(t *testing.T) {
	w := newWorld(t)
	run := w.startMade(past)
	file := filepath.Join(t.TempDir(), "started")
	w.ctrl.Command = []string{os.Args[0], stepThatHangs, file}
	w.pass() // the machine above asks for the machine below
	// A pass that ends with its step's process running is a controller
	// stopped: nothing comes of the step but the note that it was taken.
	lose := func() {
		short, cancel := context.WithCancel(ctx)
		defer cancel()
		time.AfterFunc(time.Second, cancel)
		w.ctrl.Pass(short)
	}
	for tries := 1; tries <= 2; tries++ {
		lose()
		if got := started(file); got != tries {
			t.Fatalf("after %d passes that were stopped, %d processes were started", tries, got)
		}
		// Until the time a step may take has passed, another copy may be
		// taking it, and it is left alone.
		lose()
		if got := started(file); got != tries {
			t.Fatalf("the step was taken again while it might be running: %d", got)
		}
		w.tick(w.ctrl.StepTime + time.Minute)
	}
	for range 3 {
		w.settle()
		w.tick(w.ctrl.StepTime + time.Minute)
	}
	if got := started(file); got != 2 {
		t.Fatalf("the step was taken %d times", got)
	}
	if got := w.outcome(run); !strings.Contains(got, "failed after go: the step was taken 2 times") || !strings.Contains(got, "waiting: 0") {
		t.Fatal(got)
	}
}

// One copy takes a step. Another that finds it being taken leaves it, and
// does not count it against the machine.
func Test_ctl_made_step_is_taken_by_one_copy(t *testing.T) {
	w := newWorld(t)
	w.startMade(past)
	file := filepath.Join(t.TempDir(), "started")
	w.ctrl.Command = []string{os.Args[0], stepThatHangs, file}
	w.pass()
	first := controller.New(w.ctl, w.ctrl.Store)
	first.Now = w.clock
	first.Command = []string{os.Args[0], stepThatHangs, file}
	done := make(chan struct{})
	short, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer close(done)
		first.Pass(short)
	}()
	soon(t, "the step's process to start", func() bool { return started(file) == 1 })
	second := controller.New(w.ctl, w.ctrl.Store)
	second.Now = w.clock
	second.Command = first.Command
	if _, err := second.Pass(ctx); err != nil {
		t.Fatal(err)
	}
	if got := started(file); got != 1 {
		t.Fatalf("%d processes were started for one step", got)
	}
	cancel()
	<-done
}

// A pass takes a few of these steps at a time, however many are waiting,
// and the rest wait for the next pass. So one pass waits no longer than one
// step may take, and an owner's machine with a message in that pass goes on.
func Test_ctl_made_steps_at_once_are_bounded(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	familyAt, _ := w.place(w.client, family, nil)
	var runs []string
	for range 7 {
		runs = append(runs, w.holder(r1))
	}
	for _, run := range runs {
		w.made(r1, run, past)
		w.tick(time.Millisecond)
	}
	w.pass() // each machine above asks for the machine below
	owner := w.startAt(w.client, familyAt, client.Start{})
	file := filepath.Join(t.TempDir(), "started")
	w.ctrl.Command = []string{os.Args[0], stepThatHangs, file}
	w.ctrl.StepTime = time.Second
	before := time.Now()
	w.pass()
	if got, took := started(file), time.Since(before); got != 4 || took > 5*time.Second {
		t.Fatalf("one pass started %d processes and took %v", got, took)
	}
	if w.head(owner) == nil {
		t.Fatal("an owner's machine was not taken up in that pass")
	}
	for range 3 {
		w.pass()
	}
	if got := started(file); got != 7 {
		t.Fatalf("the passes started %d processes", got)
	}
	w.settle()
	for _, run := range runs {
		if got := w.outcome(run); !strings.Contains(got, "may take") {
			t.Fatal(got)
		}
	}
}

// What a step's process says is read only up to what the channel carries.
func Test_ctl_made_step_answer_is_bounded(t *testing.T) {
	w := newWorld(t)
	run := w.startMade(past)
	w.ctrl.Command = []string{os.Args[0], stepThatSaysTooMuch}
	w.settle()
	if got := w.outcome(run); !strings.Contains(got, "could not be read: a piece of 4294967295 bytes") {
		t.Fatal(got)
	}
}

// An owner's steps start no process: this controller has none to start.
func Test_ctl_owner_step_spawns_no_process(t *testing.T) {
	w := newWorld(t)
	w.ctrl.Command = []string{"/nonexistent/no-step-process"}
	run := w.submit(family, nil, nil)
	for range 8 {
		w.settle()
		w.tick(500 * time.Millisecond)
	}
	if head := w.head(run); head == nil || head.Status != instance.Done {
		t.Fatalf("the run ended %+v", head)
	}
}
