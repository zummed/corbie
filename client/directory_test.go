package client_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// keptMachine is a machine that runs until it is told to end.
const keptMachine = `
	statechart: kept
	initial: running
	states:
	  running:
	    on: { end: over }
	  over: { end: done }
`

func TestDirectoryStartsItsMachines(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine, "plain.yaml": "statechart: plain\ninitial: e\nstates:\n  e: { end: done }\n"})
	w.submit(dir, "on")
	run := w.show(w.running("on/kept.yaml"))
	if run.Labels[client.LabelMachine] != "on/kept.yaml" || run.Input != nil {
		t.Fatalf("the run is labelled %v, with input %v", run.Labels, run.Input)
	}
	if runs := w.roots(nil); len(runs) != 1 {
		t.Fatalf("there are %d runs", len(runs))
	}
}

func TestSecondSubmitStartsNothing(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	w.submit(dir, "on")
	if runs := w.roots(nil); len(runs) != 1 || runs[0].ID != first {
		t.Fatalf("the runs are %v", runs)
	}
}

func TestChangedMachineStartedAgain(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	w.write(dir, "on/kept.yaml", dedent(keptMachine)+"# changed\n")
	w.submit(dir, "on")
	if w.show(first).Status != "cancelled" || w.running("on/kept.yaml") == first {
		t.Fatalf("the runs are %v", w.roots(nil))
	}
}

// A file put back as it was is a change: what counts is the latest run, and not
// that some run once had the hash.
func TestFilePutBackStartsAgain(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	w.write(dir, "on/kept.yaml", dedent(keptMachine)+"# changed\n")
	w.submit(dir, "on")
	second := w.running("on/kept.yaml")
	w.write(dir, "on/kept.yaml", dedent(keptMachine))
	w.submit(dir, "on")
	if w.show(second).Status != "cancelled" || w.running("on/kept.yaml") == second {
		t.Fatalf("the runs are %v", w.roots(nil))
	}
}

func TestRemovedMachineIsCancelled(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	if err := os.Remove(filepath.Join(dir, "on/kept.yaml")); err != nil {
		t.Fatal(err)
	}
	w.submit(dir, "on")
	if got := w.show(first).Status; got != "cancelled" {
		t.Fatalf("the run is %s", got)
	}
}

// A submit acts only on the directories it is given, though a name begins as
// one of them does.
func TestOtherDirectoriesAreLeftAlone(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine, "on-call/other.yaml": keptMachine})
	w.submit(dir, "on", "on-call")
	other := w.running("on-call/other.yaml")
	if err := os.Remove(filepath.Join(dir, "on-call/other.yaml")); err != nil {
		t.Fatal(err)
	}
	w.submit(dir, "on")
	if got := w.show(other).Status; got != "running" {
		t.Fatalf("the run is %s", got)
	}
}

// A run a send starts carries its sender's labels and is no run of a directory.
func TestRunStartedBySendIsLeftAlone(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{
		"on/listen.yaml": `
			statechart: listen
			initial: hearing
			states:
			  hearing:
			    on:
			      say: { send: { to: { machine: chat.yaml, key: "chat:${event.data}" }, event: said, data: "${event.data}" } }
		`,
		"chat.yaml": `
			statechart: chat
			initial: open
			states:
			  open:
			    on: { said: open, end: over }
			  over: { end: done }
		`,
	})
	w.submit(dir, "on")
	listen := w.running("on/listen.yaml")
	w.send(listen, "say", "one")
	w.until("the chat to start", func() bool { return len(w.roots(nil)) == 2 })
	w.submit(dir, "on")
	if got := w.running("on/listen.yaml"); got != listen || len(w.roots(nil)) != 2 {
		t.Fatalf("the runs are %v", w.roots(nil))
	}
}

// A file with a problem is reported and leaves its run as it is, and the rest
// are still done.
func TestFileWithAProblemKeepsItsRun(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	w.write(dir, "on/kept.yaml", "statechart: kept\nnot a field: 1\n")
	w.write(dir, "on/next.yaml", dedent(keptMachine))
	err := w.trySubmit(dir, nil, "on")
	if err == nil || !strings.Contains(err.Error(), "on/kept.yaml") {
		t.Fatalf("the submit gave %v", err)
	}
	if run := w.show(first); run.Status != "running" {
		t.Fatalf("a typo ended the run: %+v", run)
	}
	w.running("on/next.yaml")
}

// A problem with one file stops nothing else: a machine changed beside it is
// still started again.
func TestAProblemLeavesTheRestDone(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/a.yaml": keptMachine, "on/b.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/a.yaml")
	w.write(dir, "on/a.yaml", dedent(keptMachine)+"# changed\n")
	w.write(dir, "on/b.yaml", "data: 1\n")
	err := w.trySubmit(dir, nil, "on")
	if err == nil || !strings.Contains(err.Error(), "no longer reads as a machine") {
		t.Fatalf("the submit gave %v", err)
	}
	if w.show(first).Status != "cancelled" || w.running("on/a.yaml") == first {
		t.Fatalf("the runs are %v", w.roots(nil))
	}
}

// A file that no longer reads as a machine is reported, and leaves its run.
func TestFileThatIsNoLongerAMachineKeepsItsRun(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	w.write(dir, "on/kept.yaml", "# being written\n")
	err := w.trySubmit(dir, nil, "on")
	if err == nil || !strings.Contains(err.Error(), "no longer reads as a machine") {
		t.Fatalf("the submit gave %v", err)
	}
	if run := w.show(first); run.Status != "running" {
		t.Fatalf("the run is %+v", run)
	}
}

// A run that ended stays ended until its files change.
func TestRunThatEndedStaysEnded(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	w.send(first, "end", nil)
	w.until("the run to end", func() bool { return w.show(first).Status == "done" })
	w.submit(dir, "on")
	if runs := w.roots(nil); len(runs) != 1 {
		t.Fatalf("it was started again: %d runs", len(runs))
	}
	w.write(dir, "on/kept.yaml", dedent(keptMachine)+"# changed\n")
	w.submit(dir, "on")
	w.running("on/kept.yaml")
}

// A run cancelled by hand stays stopped, so a stop holds across submits.
func TestRunCancelledByHandStaysStopped(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	if _, err := w.Cancel(context.Background(), first, 0); err != nil {
		t.Fatal(err)
	}
	w.until("the run to end", func() bool { return w.show(first).Status == "cancelled" })
	w.submit(dir, "on")
	if runs := w.roots(nil); len(runs) != 1 {
		t.Fatalf("it was started again: %d runs", len(runs))
	}
}

// --again starts a machine whose run has ended, and no other.
func TestAgainStartsARunThatEnded(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine, "on/other.yaml": keptMachine})
	w.submit(dir, "on")
	first, other := w.running("on/kept.yaml"), w.running("on/other.yaml")
	for _, id := range []string{first, other} {
		w.send(id, "end", nil)
	}
	w.until("the runs to end", func() bool { return w.show(first).Status == "done" && w.show(other).Status == "done" })
	if err := w.trySubmit(dir, []string{"on/kept.yaml"}, "on"); err != nil {
		t.Fatal(err)
	}
	if w.running("on/kept.yaml") == first {
		t.Fatal("it was not started again")
	}
	if runs := w.roots(nil); len(runs) != 3 {
		t.Fatalf("there are %d runs", len(runs))
	}
	if err := w.trySubmit(dir, []string{"on/nothing.yaml"}, "on"); err == nil {
		t.Fatal("a file that is no machine was taken")
	}
}

// --again on a machine that is running starts nothing.
func TestAgainLeavesARunningRun(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{"on/kept.yaml": keptMachine})
	w.submit(dir, "on")
	first := w.running("on/kept.yaml")
	if err := w.trySubmit(dir, []string{"on/kept.yaml"}, "on"); err != nil {
		t.Fatal(err)
	}
	if runs := w.roots(nil); len(runs) != 1 || runs[0].ID != first {
		t.Fatalf("the runs are %v", runs)
	}
}

// A link does not take a machine out of the context.
func TestLinksStayInTheContext(t *testing.T) {
	dir := house(t, map[string]string{
		"outside/m.yaml":  "statechart: m\ninitial: e\nstates:\n  e: { end: done }\n",
		"outside/s.js":    "return 1",
		"house/a.yaml":    "flow: a\nstages:\n  s: { job: { script: ./s.js } }\n",
		"house/real.js":   "return 1",
		"house/near.yaml": "flow: near\nstages:\n  s: { job: { script: ./near.js } }\n",
	})
	for link, target := range map[string]string{"house/x.yaml": "../outside/m.yaml", "house/s.js": "../outside/s.js", "house/near.js": "real.js"} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	found, err := client.ReadMachines(filepath.Join(dir, "house"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range found {
		got[m.Path] = m.Problem == nil
	}
	if want := map[string]bool{"a.yaml": false, "x.yaml": false, "near.yaml": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read %v", got)
	}
}

// A context reached by a link reads as the directory it names.
func TestLinkedContextReads(t *testing.T) {
	dir := house(t, map[string]string{"real/m.yaml": "statechart: m\ninitial: e\nstates:\n  e: { end: done }\n"})
	link := filepath.Join(dir, "link")
	if err := os.Symlink("real", link); err != nil {
		t.Fatal(err)
	}
	found, err := client.ReadMachines(link, ".")
	if err != nil || len(found) != 1 || found[0].Path != "m.yaml" || found[0].Problem != nil {
		t.Fatalf("read %v, %v", found, err)
	}
}

// A file of data, however little it holds, is no problem.
func TestDataIsNoProblem(t *testing.T) {
	dir := house(t, map[string]string{"notes.yaml": "# for later\n", "empty.yaml": "", "list.yaml": "- a\n", "people.yaml": "people: {}\n"})
	found, err := client.ReadMachines(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("read %v", found[0].Path)
	}
}

// A bare path reads from the context and ./ from the file's directory, in any
// file at any depth. ../ is refused however it is written, and so is a path from
// the top of the disk.
func TestPathsReadFromTheContext(t *testing.T) {
	dir := house(t, map[string]string{
		"outside.js":                 "return 1",
		"house/lib/s.js":             "return 1",
		"house/lib/base.yaml":        "stages:\n  s: { job: { script: ./s.js } }\n",
		"house/deep/down/a.yaml":     "flow: a\nstages:\n  s: { job: { script: lib/s.js } }\n  t: { job: { script: ./near.js } }\n",
		"house/deep/down/near.js":    "return 1",
		"house/deep/included.yaml":   "flow: included\ninclude: lib/base.yaml\n",
		"house/up.yaml":              "flow: up\nstages:\n  s: { job: { script: ../outside.js } }\n",
		"house/hidden.yaml":          "flow: hidden\nstages:\n  s: { job: { script: ./lib/../../outside.js } }\n",
		"house/deep/stays.yaml":      "flow: stays\nstages:\n  s: { job: { script: ./x/../lib/s.js } }\n",
		"house/deep/climb.yaml":      "flow: climb\nstages:\n  s: { job: { script: ../lib/s.js } }\n",
		"house/deep/machine.yaml":    "flow: machine\nstages:\n  s: { job: { machine: ../up.yaml } }\n",
		"house/deep/include-up.yaml": "flow: includeup\ninclude: ../lib/base.yaml\n",
		"house/.git/old.yaml":        "flow: old\nnot a field: 1\n",
		"house/data.yaml":            "people: {}\n",
	})
	if err := os.WriteFile(filepath.Join(dir, "house/top.yaml"), []byte("flow: top\nstages:\n  s: { job: { script: "+filepath.Join(dir, "outside.js")+" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := client.ReadMachines(filepath.Join(dir, "house"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range found {
		got[m.Path] = m.Problem == nil
	}
	want := map[string]bool{"deep/down/a.yaml": true, "deep/included.yaml": true, "up.yaml": false, "hidden.yaml": false,
		"deep/stays.yaml": false, "deep/climb.yaml": false, "deep/machine.yaml": false, "deep/include-up.yaml": false, "top.yaml": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read %v", got)
	}
}

// A file loaded with require is found from the file that loads it, whichever way
// the job names it.
func TestRequireFindsWhatAJobNames(t *testing.T) {
	l := local(t)
	dir := house(t, map[string]string{
		"rules.yaml":      "who: alice\n",
		"blocks/who.yaml": "flow: who\nstages:\n  s: { job: { script: ./who.js, files: [rules.yaml, ./near.js] } }\noutput: ${stages.s.output}\n",
		"blocks/who.js":   `return [require("../rules.yaml").who, require("./near.js").value]`,
		"blocks/near.js":  "exports.value = 'near'",
		"on/start.yaml":   "flow: start\nstages:\n  s: { job: { machine: blocks/who.yaml } }\noutput: ${stages.s.output}\n",
	})
	draft, _, err := client.ReadMachine(dir, "on/start.yaml")
	if err != nil {
		t.Fatal(err)
	}
	id, err := l.Start(context.Background(), draft, client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Wait(context.Background(), id, interval)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "done" || !reflect.DeepEqual(r.Output, []any{"alice", "near"}) {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// A run reads the context as run.context, from its first step, and so does what
// it starts.
func TestContextIsReadByARunAndItsChildren(t *testing.T) {
	w := newWorld(t)
	dir := house(t, map[string]string{
		"on/top.yaml": `
			flow: top
			stages:
			  first: { job: { script: ./one.js, input: { at: "${run.context}" } } }
			  kid: { job: { machine: kid.yaml } }
			output: { first: "${stages.first.output}", mine: "${run.context}", kid: "${stages.kid.output}" }
		`,
		"kid.yaml":  "flow: kid\nstages:\n  a: { job: { script: ./one.js } }\noutput: ${run.context}\n",
		"on/one.js": "return input && input.at",
		"one.js":    "return 1",
	})
	w.submit(dir, "on")
	var run *client.Run
	w.until("the run to end", func() bool {
		for _, r := range w.roots(nil) {
			if r.Key == client.KeyFile+"on/top.yaml" {
				run = w.show(r.ID)
			}
		}
		return run != nil && run.Status == "done"
	})
	if want := map[string]any{"first": dir, "mine": dir, "kid": dir}; !reflect.DeepEqual(run.Output, want) {
		t.Fatalf("the output is %v, and the input %v", run.Output, run.Input)
	}
}

const interval = 20 * time.Millisecond

// A world is a controller, a runner and a client in the test's process,
// with the store in memory and one clock the test moves. client.NewLocal
// takes no clock, so the world is put together here as it puts it
// together.
type world struct {
	*client.Client
	t     *testing.T
	ahead atomic.Int64 // how far the clock is ahead of the computer's
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{t: t}
	mem := store.NewMemory()
	ctl, err1 := identity.New("controller", identity.Controller)
	run, err2 := identity.New("runner", identity.Runner)
	cli, err3 := identity.New("client", identity.Client)
	for _, err := range []error{err1, err2, err3} {
		if err != nil {
			t.Fatal(err)
		}
	}
	as := func(kind store.Kind, name string) store.Store {
		return mem.As(store.Party{Kind: kind, Name: name, Controller: ctl.Name})
	}
	ctlStore := as(store.Controller, ctl.Name)
	ctx, stop := context.WithCancel(context.Background())
	if err := store.SetUp(ctx, ctlStore); err != nil {
		t.Fatal(err)
	}
	ceiling := permission.Layer{Allow: []string{"all"}}
	for _, trust := range []identity.Trust{{Card: run.Card}, {Card: cli.Card, Ceiling: ceiling}} {
		if err := identity.WriteTrust(ctx, ctlStore, ctl, trust); err != nil {
			t.Fatal(err)
		}
	}
	c := controller.New(ctl, ctlStore)
	c.Now, c.AnyRunner = w.now, true
	r := runner.New(run, runner.Served{Controller: ctl.Card, Ceiling: ceiling, Store: as(store.Runner, run.Name)})
	r.Now, r.Command = w.now, []string{os.Args[0], runner.JobArg}
	w.Client = client.New(cli, ctl.Card, as(store.Client, cli.Name))
	w.Client.Now = w.now
	inbox, stopInbox := ctlStore.Notice(store.Inbox(ctl.Name))
	jobs, stopJobs := ctlStore.Notice(store.Inbox(run.Name))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer stopInbox()
		defer stopJobs()
		role.Run(ctx, w.now, func(role string, err error) { t.Logf("%s: %v", role, err) },
			role.Role{Name: "controller", Pass: c.Pass, Interval: interval, Notice: inbox},
			role.Role{Name: "runner", Pass: r.Pass, Interval: interval, Notice: jobs})
	}()
	t.Cleanup(func() { stop(); <-done })
	return w
}

func (w *world) now() time.Time { return time.Now().Add(time.Duration(w.ahead.Load())) }

// set moves the clock to a time.
func (w *world) set(at time.Time) { w.ahead.Store(int64(time.Until(at))) }

// submit brings the runs of directories of a context into line with their files.
func (w *world) submit(dir string, dirs ...string) {
	w.t.Helper()
	if err := w.trySubmit(dir, nil, dirs...); err != nil {
		w.t.Fatal(err)
	}
}

// trySubmit is submit, with the files to start again and what it gave.
func (w *world) trySubmit(dir string, again []string, dirs ...string) error {
	s := &client.Submit{Client: w.Client, Context: dir, Dirs: dirs, Again: again, Interval: interval, Say: func(format string, args ...any) {
		w.t.Logf("submit: "+format, args...)
	}}
	return s.Run(context.Background())
}

// write writes a file of a context.
func (w *world) write(dir, name, text string) {
	w.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// roots gives the root runs that carry every one of the labels.
func (w *world) roots(labels map[string]string) []*client.Run {
	w.t.Helper()
	runs, err := w.List(context.Background(), client.Filter{Labels: labels, Finished: true})
	if err != nil {
		w.t.Fatal(err)
	}
	var out []*client.Run
	for _, run := range runs {
		if run.Parent == "" {
			out = append(out, run)
		}
	}
	return out
}

// until waits for a condition, and fails the test after a few seconds.
func (w *world) until(what string, cond func() bool) {
	w.t.Helper()
	for end := time.Now().Add(10 * time.Second); !cond(); {
		if time.Now().After(end) {
			w.t.Fatalf("waited for %s", what)
		}
		time.Sleep(interval)
	}
}

// running waits for the running run of a machine in a directory, and gives its ID.
func (w *world) running(path string) string {
	w.t.Helper()
	var id string
	w.until("the run of "+path, func() bool {
		id = ""
		for _, run := range w.roots(nil) {
			if run.Key == client.KeyFile+path && run.Status == "running" {
				id = run.ID
			}
		}
		return id != ""
	})
	return id
}

// send sends a run an event.
func (w *world) send(run, event string, data any) {
	w.t.Helper()
	if _, err := w.Send(context.Background(), run, event, data, 0); err != nil {
		w.t.Fatal(err)
	}
}

// show gives a run as it stands.
func (w *world) show(id string) *client.Run {
	w.t.Helper()
	run, err := w.Show(context.Background(), id)
	if err != nil {
		w.t.Fatal(err)
	}
	return run
}

// house writes files into a new directory.
func house(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(dedent(text)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
