package runner_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/plugin/bundled"
)

// The test binary stands in for the process a runner starts for a job, and
// for the other processes the tests need.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	if len(os.Args) > 1 && standIn(os.Args[1], os.Args[2:]) {
		return
	}
	os.Exit(m.Run())
}

var ctx = context.Background()

const inst = "c1/20261010T120000.000Z-0123456789"

// world is a runner on a memory store, with a controller that is played
// by the test.
type world struct {
	t     *testing.T
	mem   *store.Memory
	owner store.Store
	ctl   *identity.Identity
	c1    *identity.Identity
	run   *identity.Identity
	mu    sync.Mutex
	ops   []store.Op
}

func newWorld(t *testing.T) *world {
	w := &world{t: t, mem: store.NewMemory()}
	w.mem.Note = func(op store.Op) {
		w.mu.Lock()
		w.ops = append(w.ops, op)
		w.mu.Unlock()
	}
	w.owner = w.mem.As(store.Party{Kind: store.Owner})
	w.ctl, _ = identity.New("ctl", identity.Controller)
	w.c1, _ = identity.New("c1", identity.Client)
	w.run, _ = identity.New("r1", identity.Runner)
	return w
}

// runner makes a copy of the runner, serving the controller under a
// ceiling.
func (w *world) runner(ceiling ...string) *runner.Runner {
	r := runner.New(w.run, runner.Served{Controller: w.ctl.Card, Ceiling: permission.Layer{Allow: ceiling},
		Store: w.mem.As(store.Party{Kind: store.Runner, Name: "r1", Controller: "ctl"})})
	r.Command = []string{os.Args[0], runner.JobArg}
	return r
}

// job leaves a job for the runner, as a controller would, and gives its
// name in the inbox.
func (w *world) job(as *identity.Identity, name, script string, change func(*instance.Job)) string {
	w.t.Helper()
	message, err := w.leave(as, name, script, change)
	if err != nil {
		w.t.Fatal(err)
	}
	return message
}

func (w *world) leave(as *identity.Identity, name, script string, change func(*instance.Job)) (string, error) {
	ref, err := blob.Write(ctx, w.owner, store.Blob(w.ctl.Name, inst, blob.NewName()), "text/javascript", blob.NewKey(), strings.NewReader(script))
	if err != nil {
		return "", err
	}
	job := &instance.Job{Instance: inst, Name: name, Attempt: 1, Request: inst + "/" + name, Script: ref,
		Layers: []permission.Layer{{Name: "the definition", Allow: []string{"all"}}}, Timeout: machine.Duration(10 * time.Second),
		Due: time.Now().Add(time.Minute), Client: w.c1.Card}
	if change != nil {
		change(job)
	}
	message := store.JobMessage(inst, name)
	at := store.Message("r1", as.Name, message)
	data, err := as.Write(at, time.Now(), job.Due, &instance.Message{Kind: instance.KindJob, Instance: inst, Job: job}, w.run.Card)
	if err != nil {
		return "", err
	}
	_, err = w.owner.Write(ctx, at, data, store.OnlyIfNew)
	return message, err
}

// results reads what the runner sent the controller: each job's result.
func (w *world) results() map[string]*instance.Result {
	w.t.Helper()
	out := map[string]*instance.Result{}
	names, _ := store.Names(w.owner.List(ctx, store.Inbox(w.ctl.Name)))
	for _, name := range names {
		data, _, _ := w.owner.Read(ctx, name)
		var msg instance.Message
		if _, err := identity.Read(data, name, w.run.Card, w.ctl, &msg); err != nil {
			w.t.Fatalf("%s: %v", name, err)
		}
		if msg.Kind == instance.KindResult {
			out[msg.Result.Job] = msg.Result
		}
	}
	return out
}

func pass(t *testing.T, r *runner.Runner) {
	t.Helper()
	if _, err := r.Pass(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestJobIsRunAndAnswered(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "work", `log("working"); emit("n", 1); return { sum: input.a + 2 }`, func(j *instance.Job) { j.Input = map[string]any{"a": 40.0} })
	pass(t, w.runner("all"))
	got := w.results()["work"]
	if got == nil || got.Status != instance.Done || got.Output.(map[string]any)["sum"] != 42.0 {
		t.Fatalf("the result is %+v", got)
	}
	// The job is gone from the inbox only once its result is written.
	if left, _ := store.Names(w.owner.List(ctx, store.Inbox("r1"))); len(left) != 0 {
		t.Fatalf("the job is still in the inbox: %v", left)
	}
	resultAt, deleteAt := -1, -1
	for i, op := range w.ops {
		if !op.Done {
			continue
		}
		switch {
		case op.Act == store.ActAdd && strings.HasPrefix(op.Name, "ctl/inbox/r1/"):
			resultAt = i
		case op.Act == store.ActDelete && strings.HasPrefix(op.Name, "r1/inbox/"):
			deleteAt = i
		}
	}
	if resultAt < 0 || deleteAt < resultAt {
		t.Fatalf("result at %d, delete at %d", resultAt, deleteAt)
	}
	// The mark stays until the job's due time has passed.
	if marks, _ := store.Names(w.owner.List(ctx, store.TakenList("r1"))); len(marks) != 1 {
		t.Fatalf("the marks are %v", marks)
	}
}

// Two copies of one runner: a job is run once.
// A job whose result could not be written stays in the inbox, and the
// pass says what went wrong.
func TestJobStaysWhenItsResultIsNotWritten(t *testing.T) {
	w := newWorld(t)
	gate := filepath.Join(t.TempDir(), "gate")
	quick := w.job(w.ctl, "quick", `return "ran"`, nil)
	w.job(w.ctl, "slow", `while (!fs.exists(input.gate)) {} return "ran"`, func(j *instance.Job) { j.Input = map[string]any{"gate": gate} })
	r := w.runner("all")
	// The first result to be written is the quick job's, and that write
	// fails. The other job is let go on only then, so the pass looks at
	// its inbox again with the quick job finished and unanswered.
	lost := errors.New("the store is away")
	var once sync.Once
	stand(r, &between{written: func(name string, written bool, err error) (bool, error) {
		if strings.HasPrefix(name, store.Inbox("ctl")+"/") {
			once.Do(func() {
				w.owner.Delete(ctx, name, store.Overwrite)
				written, err = false, lost
				os.WriteFile(gate, nil, 0o600)
			})
		}
		return written, err
	}})
	if _, err := r.Pass(ctx); !errors.Is(err, lost) {
		t.Fatalf("the pass whose job's result could not be written gave the error %v", err)
	}
	if got := w.results(); len(got) != 1 || got["slow"] == nil || got["slow"].Output != "ran" {
		t.Fatalf("the results are %v", got)
	}
	if left := w.inbox(); len(left) != 1 || !strings.HasSuffix(left[0], "/"+quick) {
		t.Fatalf("the inbox holds %v, and should hold the job that has no result", left)
	}
}

// A script well under its limit that keeps making values it does not hold
// is not stopped: what is no longer held does not count.
func TestMemoryLimitCountsOnlyWhatIsHeld(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "churn", `const held = []; while (held.length < 36) held.push("x".repeat(1 << 20) + held.length)
		let made = 0; for (const until = Date.now() + 1000; Date.now() < until; made++) ("y".repeat(1 << 20) + made).length
		return held.length`, func(j *instance.Job) { j.Memory = 64 << 20 })
	pass(t, w.runner("all"))
	if got := w.results()["churn"]; got == nil || got.Status != instance.Done || got.Output != 36.0 {
		t.Fatalf("a script holding 36 MiB under a limit of 64 MiB: %+v", got)
	}
}

// What the machine gave the job reaches the script as it is: only what a
// plugin hands back is searched for secrets.
func TestInputIsNotSearchedForSecrets(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "reads", `fs.write("a.txt", { text: input.path }); return { given: input.path === "reports/a.txt", back: fs.read("a.txt") }`,
		func(j *instance.Job) {
			j.Input, j.Secrets = map[string]any{"path": "reports/a.txt"}, map[string]string{"TOKEN": "reports"}
		})
	pass(t, w.runner("all"))
	got := w.results()["reads"]
	if got == nil || got.Status != instance.Done {
		t.Fatalf("the result is %+v", got)
	}
	if out := got.Output.(map[string]any); out["given"] != true || out["back"] != "[secret TOKEN]/a.txt" {
		t.Fatalf("the script was given its input unchanged: %v; a file's text came back as %q", out["given"], out["back"])
	}
}

func TestTwoCopiesRunAJobOnce(t *testing.T) {
	w := newWorld(t)
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		w.job(w.ctl, name, `emit("ran", job.name); return job.name`, nil)
	}
	var wg sync.WaitGroup
	for range 3 {
		r := w.runner("all")
		wg.Go(func() { pass(t, r) })
	}
	wg.Wait()
	if got := w.results(); len(got) != 6 {
		t.Fatalf("%d results came for six jobs", len(got))
	}
	// Each job emitted once: one entry under its name.
	emits, _ := store.Names(w.owner.List(ctx, store.Emits("ctl", inst)))
	if len(emits) != 6 {
		t.Fatalf("six jobs left %d emit entries: a job ran more than once", len(emits))
	}
}

func Test_run_copy_at_its_limit_leaves_jobs_unmarked(t *testing.T) {
	w := newWorld(t)
	for _, name := range []string{"a", "b", "c", "d"} {
		w.job(w.ctl, name, `const until = Date.now() + 150; while (Date.now() < until) {}; return job.name`, nil)
	}
	r := w.runner("all")
	r.Jobs = 2
	most := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		pass(t, r)
	}()
	for {
		select {
		case <-done:
			// One pass ran all four, never more than two at a time, and a
			// place that came free was taken at once.
			if got := w.results(); len(got) != 4 {
				t.Fatalf("%d results came for four jobs", len(got))
			}
			if most > 2 {
				t.Fatalf("%d marks were held at once by a copy that runs two", most)
			}
			return
		default:
			marks, _ := store.Names(w.owner.List(ctx, store.TakenList("r1")))
			inbox, _ := store.Names(w.owner.List(ctx, store.Inbox("r1")))
			// A mark is written only for a job this copy has room for.
			if held := len(marks) - (4 - len(inbox)); held > most {
				most = held
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestOverdueJobIsNotStarted(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "late", `emit("ran", 1); return 1`, func(j *instance.Job) { j.Due = time.Now().Add(-time.Second) })
	pass(t, w.runner("all"))
	if got := w.results(); len(got) != 0 {
		t.Fatalf("an overdue job was answered: %+v", got["late"])
	}
	if emits, _ := store.Names(w.owner.List(ctx, store.Emits("ctl", inst))); len(emits) != 0 {
		t.Fatal("an overdue job was run")
	}
}

func Test_id_refused_message_removed_from_inbox(t *testing.T) {
	w := newWorld(t)
	stranger, _ := identity.New("ctl", identity.Controller) // another key under the controller's name
	other, _ := identity.New("elsewhere", identity.Controller)
	w.job(stranger, "forged", `emit("ran", 1); return 1`, nil)
	// Junk, a message under another sender's name, and a kind a runner
	// does not act on.
	w.owner.Write(ctx, store.Message("r1", "ctl", "junk"), []byte("not a message at all"), store.Overwrite)
	w.job(other, "foreign", `return 1`, nil)
	start := store.Message("r1", "ctl", "a-start")
	data, _ := w.ctl.Write(start, time.Now(), time.Now().Add(time.Minute), &instance.Message{Kind: instance.KindStart, Instance: inst}, w.run.Card)
	w.owner.Write(ctx, start, data, store.Overwrite)
	w.job(w.ctl, "good", `return "ran"`, nil) // what comes after is not held up
	pass(t, w.runner("all"))
	got := w.results()
	if len(got) != 1 || got["good"] == nil || got["good"].Output != "ran" {
		t.Fatalf("the results are %v", got)
	}
	if left, _ := store.Names(w.owner.List(ctx, store.Inbox("r1"))); len(left) != 0 {
		t.Fatalf("what the runner does not accept was left in its inbox: %v", left)
	}
}

func Test_id_runner_ceiling_chosen_by_jobs_signer(t *testing.T) {
	w := newWorld(t)
	other, _ := identity.New("ctl2", identity.Controller)
	script := `return env.get("HOME", { otherwise: "unset" }) !== null`
	w.job(w.ctl, "from-one", script, nil)
	// The second controller's job waits in the second controller's store.
	mem2 := store.NewMemory()
	owner2 := mem2.As(store.Party{Kind: store.Owner})
	w2 := &world{t: t, mem: mem2, owner: owner2, ctl: other, c1: w.c1, run: w.run}
	w2.job(other, "from-two", script, nil)
	r := runner.New(w.run,
		runner.Served{Controller: w.ctl.Card, Ceiling: permission.Layer{Allow: []string{"disk"}}, Store: w.mem.As(store.Party{Kind: store.Runner, Name: "r1", Controller: "ctl"})},
		runner.Served{Controller: other.Card, Ceiling: permission.Layer{Allow: []string{"env"}}, Store: mem2.As(store.Party{Kind: store.Runner, Name: "r1", Controller: "ctl2"})})
	r.Command = []string{os.Args[0], runner.JobArg}
	pass(t, r)
	// Refused under the ceiling given to the first controller; run under
	// the one given to the second; each result in its own store.
	if got := w.results()["from-one"]; got == nil || got.Status != machine.StatusDenied || !strings.Contains(got.Error.Layer, "ceiling of the runner") {
		t.Fatalf("the first controller's job: %+v", got)
	}
	if got := w2.results()["from-two"]; got == nil || got.Status != instance.Done {
		t.Fatalf("the second controller's job: %+v", got)
	}
}

func Test_run_job_with_cancel_waiting_is_not_started(t *testing.T) {
	w := newWorld(t)
	message := w.job(w.ctl, "unwanted", `emit("ran", 1); return 1`, nil)
	at := store.Message("r1", "ctl", message+".cancel")
	data, _ := w.ctl.Write(at, time.Now(), time.Now().Add(time.Minute), &instance.Message{Kind: instance.KindCancel, Instance: inst, Cancel: message}, w.run.Card)
	w.owner.Write(ctx, at, data, store.OnlyIfNew)
	pass(t, w.runner("all"))
	got := w.results()["unwanted"]
	if got == nil || got.Status != machine.StatusCancelled {
		t.Fatalf("a job whose cancel was waiting: %+v", got)
	}
	if emits, _ := store.Names(w.owner.List(ctx, store.Emits("ctl", inst))); len(emits) != 0 {
		t.Fatal("a job whose cancel was waiting was started")
	}
	if left, _ := store.Names(w.owner.List(ctx, store.Inbox("r1"))); len(left) != 0 {
		t.Fatalf("left in the inbox: %v", left)
	}
}

// A stand-in plugin, built on the public package as an extension is. Its
// one call waits on the outside as any call does, for as long as the job's
// context lasts. It makes the file it is given once it is waiting, and a
// second beside it when it has seen the wait ended.
func init() {
	plugin.Register(&plugin.Module{Name: "waiting", Calls: map[string]*plugin.Call{
		"until_stopped": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			began, _ := first.(string)
			os.WriteFile(began, nil, 0o600)
			<-job.Context.Done()
			os.WriteFile(began+".ended", nil, 0o600)
			return nil, plugin.Errorf(plugin.TimedOut, "the wait was ended")
		}},
	}})
}

// stuck leaves a job whose script waits in a call, runs a pass of the
// runner, and gives the file the call makes once it is waiting and a
// channel that closes when the pass is over.
func (w *world) stuck(limit time.Duration) (began string, over chan struct{}, message string) {
	began = filepath.Join(w.t.TempDir(), "began")
	message = w.job(w.ctl, "stuck", `try { waiting.until_stopped(input.began) } catch (e) {}; return "finished"`, func(j *instance.Job) {
		j.Timeout, j.Input = machine.Duration(limit), map[string]any{"began": began}
	})
	r := w.runner("all")
	over = make(chan struct{})
	go func() {
		defer close(over)
		if _, err := r.Pass(ctx); err != nil {
			w.t.Error(err)
		}
	}()
	return began, over, message
}

func there(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func Test_run_cancel_stops_script_waiting_in_a_call(t *testing.T) {
	w := newWorld(t)
	began, over, message := w.stuck(time.Minute)
	// The cancel is left once the script is inside the call.
	for !there(began) {
		select {
		case <-over:
			t.Fatalf("the job ended before its script was waiting: %+v", w.results()["stuck"])
		case <-time.After(5 * time.Millisecond):
		}
	}
	at := store.Message("r1", "ctl", message+".cancel")
	data, _ := w.ctl.Write(at, time.Now(), time.Now().Add(time.Minute), &instance.Message{Kind: instance.KindCancel, Instance: inst, Cancel: message}, w.run.Card)
	w.owner.Write(ctx, at, data, store.OnlyIfNew)
	<-over
	if got := w.results()["stuck"]; got == nil || got.Status != machine.StatusCancelled {
		t.Fatalf("a cancelled job: %+v", got)
	}
	// The call itself was ended. A stop that reached only the script would
	// have waited with the call, until the job's process was killed.
	if !there(began + ".ended") {
		t.Fatal("the call the script was waiting in was not ended: the job stopped only when its process was killed")
	}
}

// A script's time limit ends the call it is waiting in the same way.
func TestTimeLimitEndsTheCallAScriptIsWaitingIn(t *testing.T) {
	w := newWorld(t)
	began, over, _ := w.stuck(300 * time.Millisecond)
	<-over
	if got := w.results()["stuck"]; got == nil || got.Status != machine.StatusTimeout {
		t.Fatalf("a job past its time limit: %+v", got)
	}
	if !there(began) || !there(began+".ended") {
		t.Fatal("the call the script was waiting in was not ended: the job stopped only when its process was killed")
	}
}

// A second stand-in plugin. Its one call prints what it is given and ends
// the job's process there and then, as a plugin that went badly wrong
// would.
func init() {
	plugin.Register(&plugin.Module{Name: "dying", Calls: map[string]*plugin.Call{
		"holding": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			text, err := job.Reveal(first, "exec:dying")
			if err != nil {
				return nil, err
			}
			os.Stderr.WriteString(text)
			os.Exit(7)
			return nil, nil
		}},
	}})
}

// What a job's process printed before it died is searched by its runner,
// which knows the usual forms of a secret's value beforehand. A form a
// script made, by joining the secret with text and encoding the whole, is
// known only once it is made, inside the process: the runner is told of it
// before the value is handed over.
func TestDeadProcessOutputIsSearchedForFormsMadeInTheJob(t *testing.T) {
	const value = "tok-en-1234567890abcdef"
	w := newWorld(t)
	w.job(w.ctl, "dies", "const t = secrets.get(\"TOKEN\")\n"+
		"const signIn = secrets.encoded(secrets.text`someone:${t}`)\n"+
		"dying.holding(secrets.text`as it is: ${t}; as a sign-in: ${signIn}; and that once more, for an address: ${secrets.encoded(signIn, { as: \"url\" })}.`)",
		func(j *instance.Job) { j.Secrets = map[string]string{"TOKEN": value} })
	pass(t, w.runner("all"))
	got := w.results()["dies"]
	if got == nil || got.Status != machine.StatusError || !strings.Contains(got.Error.Error, "ended without a result") {
		t.Fatalf("a job whose process died: %+v", got)
	}
	if want := "as it is: [secret TOKEN]; as a sign-in: [secret TOKEN]; and that once more, for an address: [secret TOKEN]."; !strings.Contains(got.Error.Error, want) {
		t.Errorf("what the process printed was kept as: %s", got.Error.Error)
	}
	signIn := base64.StdEncoding.EncodeToString([]byte("someone:" + value))
	for what, form := range map[string]string{"the value": value, "the sign-in": signIn, "the sign-in without its end": signIn[:len(signIn)-4]} {
		if strings.Contains(got.Error.Error, form) {
			t.Errorf("what the process printed was kept with %s in it: %s", what, got.Error.Error)
		}
	}
}

func Test_run_altered_script_is_not_run(t *testing.T) {
	w := newWorld(t)
	var at string
	w.job(w.ctl, "spoilt", `return "ran"`, func(j *instance.Job) { at = j.Script.At })
	raw, _, _ := w.owner.Read(ctx, at)
	raw[len(raw)/2] ^= 1
	w.owner.Write(ctx, at, raw, store.Overwrite)
	pass(t, w.runner("all"))
	if got := w.results()["spoilt"]; got == nil || got.Status != machine.StatusError || !strings.Contains(got.Error.Error, "could not be read whole") {
		t.Fatalf("a job whose script was altered: %+v", got)
	}
}

func TestMarkGoesOnceTheDueTimeHasPassed(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "quick", `return 1`, func(j *instance.Job) { j.Due = time.Now().Add(2 * time.Second) })
	r := w.runner("all")
	pass(t, r)
	if marks, _ := store.Names(w.owner.List(ctx, store.TakenList("r1"))); len(marks) != 1 {
		t.Fatalf("after the job the marks are %v", marks)
	}
	// Five minutes past the due time, by the runner's own clock.
	r.Now = func() time.Time { return time.Now().Add(4 * time.Minute) }
	pass(t, r)
	if marks, _ := store.Names(w.owner.List(ctx, store.TakenList("r1"))); len(marks) != 1 {
		t.Fatal("the mark went before the allowance for clocks had passed")
	}
	r.Now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	pass(t, r)
	if marks, _ := store.Names(w.owner.List(ctx, store.TakenList("r1"))); len(marks) != 0 {
		t.Fatalf("the mark is still there: %v", marks)
	}
}

// A runner that was killed had no chance to remove its jobs' scratch
// directories. The next to start removes them, and leaves alone one whose
// runner is still there.
func Test_run_scratch_left_by_runner_that_is_gone_removed(t *testing.T) {
	w := newWorld(t)
	base := t.TempDir()
	// A process that has ended, whose number nothing has now.
	ended := exec.Command(os.Args[0], "-test.run", "^$")
	if err := ended.Run(); err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(base, fmt.Sprintf("corbie-job-%d-123", ended.Process.Pid))
	live := filepath.Join(base, fmt.Sprintf("corbie-job-%d-456", os.Getppid()))
	other := filepath.Join(base, "someone-elses")
	for _, directory := range []string{left, live, other} {
		os.MkdirAll(directory, 0o700)
		os.WriteFile(filepath.Join(directory, "blob-1"), []byte("left behind"), 0o600)
	}
	r := w.runner("all")
	r.Scratch = base
	passCtx, cancel := context.WithCancel(ctx)
	cancel()
	r.Pass(passCtx)
	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Fatalf("the directory of a runner that is gone is still there: %v", err)
	}
	for _, directory := range []string{live, other} {
		if _, err := os.Stat(directory); err != nil {
			t.Fatalf("%s was removed: %v", directory, err)
		}
	}
}

// An empty view at the very end of its buffer is told from text without
// being converted out of the script engine.
func Test_run_empty_bytes_view_at_end_of_buffer_is_safe(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "work", `const buf = new ArrayBuffer(64)
		const view = new Float64Array(buf, 64, 0)
		log(view, new Uint8Array(buf, 64, 0), "text")
		return { length: view.length }`, nil)
	pass(t, w.runner("all"))
	got := w.results()["work"]
	if got == nil || got.Status != instance.Done || got.Output.(map[string]any)["length"] != 0.0 {
		t.Fatalf("the result is %+v", got)
	}
}
