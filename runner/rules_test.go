package runner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// between is a runner's store with the test standing in it.
type between struct {
	store.Store
	// reading is told before part of a value is read.
	reading func(name string)
	// written is told after a write, and gives the answer the runner gets.
	written func(name string, written bool, err error) (bool, error)
	// read is told before a whole value is read.
	read func(name string)
	// listing is told before a listing.
	listing func(under string)
	// quiet is a store that tells nobody what was written, as a bucket
	// does not.
	quiet bool
}

func (b *between) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if b.read != nil {
		b.read(name)
	}
	return b.Store.Read(ctx, name)
}

func (b *between) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if b.reading != nil {
		b.reading(name)
	}
	return b.Store.ReadPart(ctx, name, from, length)
}

func (b *between) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	written, err := b.Store.Write(ctx, name, value, how)
	if b.written != nil {
		return b.written(name, written, err)
	}
	return written, err
}

func (b *between) List(ctx context.Context, under string) ([]store.Listed, error) {
	if b.listing != nil {
		b.listing(under)
	}
	return b.Store.List(ctx, under)
}

func (b *between) Notice(under string) (<-chan struct{}, func()) {
	if b.quiet {
		return nil, func() {}
	}
	return b.Store.Notice(under)
}

// stand puts the test in a runner's store.
func stand(r *runner.Runner, b *between) {
	b.Store = r.Serves[0].Store
	r.Serves[0].Store = b
}

// cancel leaves a cancel for a job, as a controller would, and gives its
// name in the inbox.
func (w *world) cancel(message string) string {
	w.t.Helper()
	at := store.Message("r1", "ctl", message+".cancel")
	data, err := w.ctl.Write(at, time.Now(), time.Now().Add(time.Minute), &instance.Message{Kind: instance.KindCancel, Instance: inst, Cancel: message}, w.run.Card)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, at, data, store.OnlyIfNew); err != nil {
		w.t.Fatal(err)
	}
	return at
}

// emitted reads the entries a job's emits were written as, in order.
func (w *world) emitted(job string) []*instance.Emit {
	w.t.Helper()
	var entries []*instance.Emit
	names, _ := store.Names(w.owner.List(ctx, store.Emits("ctl", inst)+"/"+job))
	for _, name := range names {
		data, _, _ := w.owner.Read(ctx, name)
		var entry instance.Emit
		if _, err := identity.Read(data, name, w.run.Card, w.ctl, &entry); err != nil {
			w.t.Fatalf("%s: %v", name, err)
		}
		entries = append(entries, &entry)
	}
	return entries
}

// inbox lists what is waiting for the runner.
func (w *world) inbox() []string {
	names, _ := store.Names(w.owner.List(ctx, store.Inbox("r1")))
	return names
}

// passing runs a pass beside the test, and gives a way to end it and wait
// for it.
func passing(r *runner.Runner) (end func() error) {
	passCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := r.Pass(passCtx)
		done <- err
	}()
	return sync.OnceValue(func() error {
		cancel()
		return <-done
	})
}

// soon waits for something the runner or a job is about to do.
func soon(t *testing.T, what string, happened func() bool) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); !happened(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("waited in vain for %s", what)
		}
	}
}

// running is a script that says it has started, by writing the file its
// input names, and then never ends.
const running = `fs.write(input.marker, { text: "running" }); for (;;) {}`

func marked(marker string) func() bool {
	return func() bool {
		_, err := os.Stat(marker)
		return err == nil
	}
}

func Test_run_thawed_copy_does_not_start_overdue_job(t *testing.T) {
	w := newWorld(t)
	var script string
	w.job(w.ctl, "late", `emit("ran", 1); return "ran"`, func(j *instance.Job) { script = j.Script.At })
	r := w.runner("all")
	// The copy is frozen while it fetches the script, after it has taken
	// the job, and is thawed an hour on.
	var frozen atomic.Int64
	r.Now = func() time.Time { return time.Now().Add(time.Duration(frozen.Load())) }
	stand(r, &between{reading: func(name string) {
		if name == script {
			frozen.Store(int64(time.Hour))
		}
	}})
	pass(t, r)
	got := w.results()["late"]
	if got == nil || got.Status != machine.StatusTimeout || !strings.Contains(got.Error.Error, "overdue") {
		t.Fatalf("a job overdue when its script would start: %+v", got)
	}
	if len(w.emitted("late")) != 0 {
		t.Fatal("the script of an overdue job was started")
	}
}

func Test_run_mark_whose_answer_was_lost_is_known_as_own(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "work", `return "ran"`, nil)
	r := w.runner("all")
	// The mark is written, and the store's answer never comes back.
	lost := 0
	stand(r, &between{written: func(name string, written bool, err error) (bool, error) {
		if strings.HasPrefix(name, store.TakenList("r1")+"/") && written {
			lost++
			return false, store.ErrUnknown
		}
		return written, err
	}})
	pass(t, r)
	if got := w.results()["work"]; lost != 1 || got == nil || got.Output != "ran" {
		t.Fatalf("the answer was lost %d times, and the job's result is %+v", lost, got)
	}
	// A mark that another copy wrote is not taken for its own: the job is
	// left alone.
	taken := w.job(w.ctl, "taken", `return "ran"`, nil)
	at := store.Taken("r1", time.Now().Add(time.Minute), taken)
	theirs, _ := w.run.Write(at, time.Now(), time.Time{}, map[string]any{"copy": "another", "due": time.Now().Add(time.Minute)})
	w.owner.Write(ctx, at, theirs, store.OnlyIfNew)
	pass(t, r)
	if left := w.inbox(); len(left) != 1 || w.results()["taken"] != nil {
		t.Fatalf("a job another copy had marked: the inbox holds %v and its result is %+v", left, w.results()["taken"])
	}
}

func Test_run_write_whose_answer_was_lost_is_sent_again(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "work", `emit("note", 1); return "ran"`, nil)
	r := w.runner("all")
	// Everything the job writes for the controller arrives, and the
	// store's answer to it never comes back, the first time.
	lost := map[string]int{}
	stand(r, &between{written: func(name string, written bool, err error) (bool, error) {
		if !strings.HasPrefix(name, "r1/") && lost[name] == 0 {
			lost[name]++
			return false, store.ErrUnknown
		}
		return written, err
	}})
	pass(t, r)
	got := w.results()["work"]
	if len(lost) < 2 || got == nil || got.Output != "ran" {
		t.Fatalf("answers were lost for %v, and the job's result is %+v", lost, got)
	}
	if len(w.emitted("work")) != 1 || len(w.inbox()) != 0 {
		t.Fatalf("the job emitted %v and the inbox holds %v", w.emitted("work"), w.inbox())
	}
}

func Test_run_freed_place_takes_waiting_job_at_once(t *testing.T) {
	w := newWorld(t)
	for _, name := range []string{"a", "b", "c", "d"} {
		w.job(w.ctl, name, `return job.name`, nil)
	}
	r := w.runner("all")
	r.Jobs = 1
	// Nothing tells this copy of a write: it looks again when a place
	// comes free, or at its next interval, which is seconds away.
	stand(r, &between{quiet: true})
	started := time.Now()
	pass(t, r)
	if got := w.results(); len(got) != 4 {
		t.Fatalf("%d results came for four jobs", len(got))
	}
	// Three jobs each waited for a place. Left to the interval, that is
	// three intervals of two seconds.
	if took := time.Since(started); took > 4*time.Second {
		t.Fatalf("four short jobs, one at a time, took %v: a waiting job sat out an interval", took)
	}
}

func Test_run_job_taken_only_if_limit_fits_time_left(t *testing.T) {
	w := newWorld(t)
	// The host gave this copy a hundred seconds, and thirty have gone by
	// when it looks at its jobs. A job that may run for 62 seconds leaves
	// no time to write its result.
	long := w.job(w.ctl, "long", `return "ran"`, func(j *instance.Job) { j.Timeout = machine.Duration(62 * time.Second) })
	w.job(w.ctl, "short", `return "ran"`, func(j *instance.Job) { j.Timeout = machine.Duration(5 * time.Second) })
	r := w.runner("all")
	passCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	begun := false
	r.Now = func() time.Time {
		if !begun {
			begun = true
			return time.Now()
		}
		return time.Now().Add(30 * time.Second)
	}
	if _, err := r.Pass(passCtx); err != nil {
		t.Fatal(err)
	}
	got := w.results()
	if len(got) != 1 || got["short"] == nil || got["short"].Output != "ran" {
		t.Fatalf("the results are %v", got)
	}
	// The other is left unmarked, for a pass with time enough.
	if left := w.inbox(); len(left) != 1 || !strings.HasSuffix(left[0], "/"+long) {
		t.Fatalf("the inbox holds %v", left)
	}
	if marks, _ := store.Names(w.owner.List(ctx, store.TakenList("r1"))); len(marks) != 1 || strings.HasSuffix(marks[0], "/"+long) {
		t.Fatalf("the marks are %v", marks)
	}
	// With no limit from its host, the copy takes it.
	r.Now = time.Now
	pass(t, r)
	if got := w.results()["long"]; got == nil || got.Output != "ran" {
		t.Fatalf("a pass with time enough: %+v", got)
	}
}

func Test_run_job_no_call_could_finish_fails_at_once(t *testing.T) {
	w := newWorld(t)
	// The host gives every call a minute. A job that may run for five can
	// never be taken, however fresh the copy: it fails now and says why,
	// and does not wait out its time in the inbox.
	w.job(w.ctl, "long", `return "ran"`, func(j *instance.Job) { j.Timeout = machine.Duration(5 * time.Minute) })
	w.job(w.ctl, "short", `return "ran"`, func(j *instance.Job) { j.Timeout = machine.Duration(5 * time.Second) })
	r := w.runner("all")
	passCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if _, err := r.Pass(passCtx); err != nil {
		t.Fatal(err)
	}
	got := w.results()
	if got["short"] == nil || got["short"].Output != "ran" {
		t.Fatalf("the job that fits: %+v", got["short"])
	}
	long := got["long"]
	if long == nil || long.Status != machine.StatusError || long.Error == nil ||
		!strings.Contains(long.Error.Error, "may take 5m0s") || !strings.Contains(long.Error.Error, "shorter timeout") {
		t.Fatalf("the job no call could finish: %+v", long)
	}
	if left := w.inbox(); len(left) != 0 {
		t.Fatalf("the inbox still holds %v", left)
	}
}

func Test_run_result_written_after_all_emits(t *testing.T) {
	w := newWorld(t)
	// More than one batch, the last of them not full when the script
	// returns, and every value one the machine wants to be told of.
	w.job(w.ctl, "work", `for (let i = 0; i < 40; i++) emit("n", i); return "done"`, func(j *instance.Job) { j.Wants = []string{"n"} })
	pass(t, w.runner("all"))
	w.mu.Lock()
	defer w.mu.Unlock()
	values, told, results := 0, 0, 0
	for _, op := range w.ops {
		if op.Act != store.ActAdd || !op.Done {
			continue
		}
		switch {
		case strings.Contains(op.Name, "/emits/work/"):
			var entry instance.Emit
			if _, err := identity.Read(op.Value, op.Name, w.run.Card, w.ctl, &entry); err != nil {
				t.Fatal(err)
			}
			if results > 0 {
				t.Fatalf("the entry %s was written after the result", op.Name)
			}
			values += len(entry.Values)
		case strings.HasPrefix(op.Name, "ctl/inbox/r1/"):
			var msg instance.Message
			if _, err := identity.Read(op.Value, op.Name, w.run.Card, w.ctl, &msg); err != nil {
				t.Fatal(err)
			}
			switch {
			case msg.Kind == instance.KindResult:
				results++
			case results > 0:
				t.Fatalf("the controller was told of the emit %d after the result", msg.Emit.Index)
			default:
				told++
			}
		}
	}
	if values != 40 || told != 40 || results != 1 {
		t.Fatalf("%d values were written, the controller was told of %d, and %d results were sent", values, told, results)
	}
}

func Test_run_result_stays_good_while_controller_is_away(t *testing.T) {
	w := newWorld(t)
	// The job is due in a minute. Its controller comes back in a year.
	w.job(w.ctl, "work", `emit("n", 1); return "done"`, func(j *instance.Job) { j.Wants = []string{"n"} })
	pass(t, w.runner("all"))
	later := time.Now().AddDate(1, 0, 0)
	names, _ := store.Names(w.owner.List(ctx, store.Inbox("ctl")))
	kinds := map[string]int{}
	for _, name := range names {
		data, _, _ := w.owner.Read(ctx, name)
		var msg instance.Message
		v, err := identity.Read(data, name, w.run.Card, w.ctl, &msg)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Good(later) {
			t.Errorf("the %s the runner sent is no longer good a year on", msg.Kind)
		}
		kinds[msg.Kind]++
	}
	if kinds[instance.KindResult] != 1 || kinds[instance.KindEmit] != 1 {
		t.Fatalf("the controller was sent %v", kinds)
	}
}

func Test_run_cancel_reaches_the_copy_that_runs_the_job(t *testing.T) {
	w := newWorld(t)
	marker := filepath.Join(t.TempDir(), "running")
	message := w.job(w.ctl, "long", running, func(j *instance.Job) {
		j.Input, j.Timeout = map[string]any{"marker": marker}, machine.Duration(5*time.Second)
	})
	a, b := w.runner("all"), w.runner("all")
	// Copy A, which runs the job, is kept from its next look at the inbox
	// until copy B has had its own.
	var hold sync.RWMutex
	stand(a, &between{listing: func(under string) {
		if under == store.Inbox("r1") {
			hold.RLock()
			hold.RUnlock()
		}
	}})
	end := passing(a)
	defer end()
	soon(t, "the job to start", marked(marker))
	hold.Lock()
	cancel := w.cancel(message)
	pass(t, b)
	// B is not running the job: it leaves the cancel where it is.
	if left := w.inbox(); len(left) != 2 || len(w.results()) != 0 {
		hold.Unlock()
		t.Fatalf("after the other copy's pass the inbox holds %v and the results are %v", left, w.results())
	}
	hold.Unlock()
	// A finds it, stops the job, and is the one that deletes it.
	soon(t, "the job to be answered", func() bool { return w.results()["long"] != nil })
	if got := w.results()["long"]; got.Status != machine.StatusCancelled {
		t.Fatalf("the cancelled job: %+v", got)
	}
	soon(t, "the cancel to be deleted", func() bool {
		_, mk, _ := w.owner.Read(ctx, cancel)
		return mk == store.None
	})
}

func Test_run_cancel_for_job_this_copy_finished_is_deleted(t *testing.T) {
	w := newWorld(t)
	message := w.job(w.ctl, "quick", `return "ran"`, nil)
	a, b := w.runner("all"), w.runner("all")
	pass(t, a)
	w.cancel(message)
	// The job is answered and gone from the inbox, so no copy is running
	// it: whichever copy looks next deletes the cancel, also one that
	// remembers nothing of the job.
	pass(t, b)
	if left := w.inbox(); len(left) != 0 {
		t.Fatalf("the cancel of a finished job is still there: %v", left)
	}
	if got := w.results(); len(got) != 1 || got["quick"].Output != "ran" {
		t.Fatalf("the results are %v", got)
	}
}

func Test_run_emits_batched_by_count_or_short_time(t *testing.T) {
	w := newWorld(t)
	// Seventy values in one go, and then the script does not end.
	w.job(w.ctl, "work", `for (let i = 0; i < 70; i++) emit("n", i); for (;;) {}`, func(j *instance.Job) { j.Timeout = machine.Duration(20 * time.Second) })
	end := passing(w.runner("all"))
	defer end()
	// All seventy are in the record while the job is still running: the
	// last few do not wait for a batch to fill.
	var entries []*instance.Emit
	soon(t, "the last values to be written", func() bool {
		entries = w.emitted("work")
		count := 0
		for _, entry := range entries {
			count += len(entry.Values)
		}
		return count == 70
	})
	if len(w.results()) != 0 {
		t.Fatal("the job had ended")
	}
	// A full batch is written at once, and no entry holds more.
	next, full := 0, 0
	for i, entry := range entries {
		if entry.Number != i+1 || len(entry.Values) > 32 {
			t.Fatalf("entry %d is numbered %d and holds %d values", i+1, entry.Number, len(entry.Values))
		}
		if len(entry.Values) == 32 {
			full++
		}
		for _, value := range entry.Values {
			if value.Data != float64(next) {
				t.Fatalf("value %d is %v", next, value.Data)
			}
			next++
		}
	}
	if full == 0 || len(entries) > 10 {
		t.Fatalf("seventy values were written as %d entries, %d of them full", len(entries), full)
	}
}

func Test_run_long_inbox_read_only_where_a_job_can_be_taken(t *testing.T) {
	w := newWorld(t)
	// Forty jobs wait, and other copies have marked all but the first.
	var jobs []string
	for i := range 40 {
		jobs = append(jobs, w.job(w.ctl, fmt.Sprintf("job%02d", i), `return "ran"`, nil))
	}
	for _, job := range jobs[1:] {
		at := store.Taken("r1", time.Now().Add(time.Minute), job)
		theirs, _ := w.run.Write(at, time.Now(), time.Time{}, map[string]any{"copy": "another"})
		w.owner.Write(ctx, at, theirs, store.OnlyIfNew)
	}
	r := w.runner("all")
	read := map[string]int{}
	stand(r, &between{read: func(name string) { read[strings.Split(name, "/")[1]]++ }})
	pass(t, r)
	if got := w.results()["job00"]; got == nil || got.Output != "ran" {
		t.Fatalf("the one job nobody had taken: %+v", got)
	}
	// It read the one job it could take, and none of the thirty-nine that
	// were another copy's; and it cleared no mark by reading it.
	if read["inbox"] != 1 || read["taken"] != 0 {
		t.Fatalf("before forty jobs, one of them free, the pass read %d messages and %d marks", read["inbox"], read["taken"])
	}
	if left := w.inbox(); len(left) != 39 {
		t.Fatalf("the inbox holds %d jobs", len(left))
	}
}
