package client_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

// A listing reads the running heads before the finished ones, so a run
// that ends between the two listings is still in one of them.
func Test_client_listing_never_misses_a_run_that_ends(t *testing.T) {
	a := arrange(t)
	ends, other := a.named("ends"), a.named("other")
	a.head(ends, instance.Running, 1)
	a.head(other, instance.Running, 1)
	// The run ends after the first of the two listings, as the controller
	// ends one: its head is written among the finished, and then removed
	// from the running.
	listings := 0
	a.hooked.listed = func(under string, names []string) []string {
		if listings++; listings == 1 {
			a.head(ends, instance.Done, 2)
			if _, err := a.owner.Delete(ctx, store.RunningHead("controller", ends), store.Overwrite); err != nil {
				t.Error(err)
			}
		}
		return names
	}
	runs, err := a.List(ctx, client.Filter{Finished: true})
	if err != nil || len(runs) != 2 {
		t.Fatalf("a run that ended during the listing is missing from it: %+v, %v", runs, err)
	}
	for _, run := range runs {
		if want := map[string]string{ends: instance.Done, other: instance.Running}[run.ID]; run.Status != want {
			t.Fatalf("%s is listed as %s, want %s", run.ID, run.Status, want)
		}
	}
}

// A follow reads the record once more after it sees the run has ended: an
// emit written just before the last step can be listed just after it.
func Test_client_follow_reads_once_more_after_end(t *testing.T) {
	a := arrange(t)
	root := a.named("root")
	a.step(root, 1, instance.Running, a.job("work"))
	a.emit(root, "work", 1, "the last thing the job said")
	a.step(root, 2, instance.Done)
	// The store's first listing of the emits does not show the entry yet.
	listings := 0
	a.hooked.listed = func(under string, names []string) []string {
		if under == store.Emits("controller", root) {
			if listings++; listings == 1 {
				return nil
			}
		}
		return names
	}
	run, told := a.follow(root)
	if run.Status != instance.Done || !slices.Contains(told, `root emit "the last thing the job said"`) {
		t.Fatalf("the follow ended %s having been told %v", run.Status, told)
	}
}

// A receipt is believed only if the controller signed it for the message
// it is filed under.
func Test_client_receipt_for_another_message_not_believed(t *testing.T) {
	a := arrange(t)
	// The controller refused one start. Whoever keeps the store files that
	// receipt under another run's start, and under a cancel.
	refused, good := a.named("0000000001"), a.named("0000000002")
	message := func(run string) string { return run[strings.IndexByte(run, '/')+1:] }
	at := store.Receipt("controller", "client", message(refused))
	value, err := a.ctl.Write(at, time.Now(), time.Time{}, &instance.Receipt{Refused: "the definition does not pass its check"}, a.ID.Card)
	a.put(at, value, err)
	if run, err := a.Show(ctx, refused); err != nil || run.Status != instance.Failed {
		t.Fatalf("the refused start is shown as %+v, %v", run, err)
	}
	a.put(store.Receipt("controller", "client", message(good)), value, nil)
	a.put(store.Receipt("controller", "client", "a-cancel"), value, nil)
	if run, err := a.Show(ctx, good); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a receipt moved from another message was believed: the run is shown as %+v, %v", run, err)
	}
	if receipt, err := a.Receipt(ctx, "a-cancel"); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a receipt moved from another message was believed: %+v, %v", receipt, err)
	}
	// One signed by somebody else is no better.
	at = store.Receipt("controller", "client", "another")
	value, err = a.run.Write(at, time.Now(), time.Time{}, &instance.Receipt{Taken: true}, a.ID.Card)
	a.put(at, value, err)
	if receipt, err := a.Receipt(ctx, "another"); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a receipt the controller did not sign was believed: %+v, %v", receipt, err)
	}
}

// A step is believed only for the instance and the number the controller
// signed it for.
func Test_client_step_moved_between_records_not_believed(t *testing.T) {
	a := arrange(t)
	ended, going := a.named("ended"), a.named("going")
	a.step(ended, 1, instance.Running)
	a.step(ended, 2, instance.Done)
	a.step(going, 1, instance.Running)
	a.head(going, instance.Running, 1)
	if run, err := a.Show(ctx, going); err != nil || run.Status != instance.Running {
		t.Fatalf("the run is shown as %+v, %v", run, err)
	}
	// Whoever keeps the store moves the other run's last step into this
	// run's record, where its next step would be.
	moved, _, err := a.owner.Read(ctx, store.Step("controller", ended, 2))
	a.put(store.Step("controller", going, 2), moved, err)
	if run, err := a.Show(ctx, going); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a step moved in from another record was believed: the run is shown as %+v, %v", run, err)
	}
	within, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if run, err := a.Follow(within, going, time.Millisecond, func(client.Entry) {}); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a follow believed a step moved in from another record: %+v, %v", run, err)
	}
	// A step of its own under another number is no better.
	a.owner.Delete(ctx, store.Step("controller", going, 2), store.Overwrite)
	own, _, err := a.owner.Read(ctx, store.Step("controller", going, 1))
	a.put(store.Step("controller", going, 2), own, err)
	if run, err := a.Show(ctx, going); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a step moved to another number was believed: the run is shown as %+v, %v", run, err)
	}
}

// The status waiting means only that a run has been sent and not yet taken
// up. A run that has started and waits on a job, a timer or a key is
// running, with what it waits for beside its status.
func Test_client_waiting_status_only_before_taken_up(t *testing.T) {
	// With no controller, a run that has been sent is waiting to start.
	a := arrange(t)
	sent, err := a.Start(ctx, draft(t, standing, map[string]string{"work.js": "return 1"}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	if run, err := a.Show(ctx, sent); err != nil || run.Status != instance.Waiting || run.Step != 0 || run.Jobs+run.Timers+len(run.Tickets) != 0 {
		t.Fatalf("a run sent and not taken up is shown as %+v, %v", run, err)
	}

	l := local(t)
	shown := func(id string, waits func(*client.Run) bool) *client.Run {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			run, err := l.Show(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != instance.Waiting && (run.HasEnded() || waits(run)) {
				return run
			}
			if time.Now().After(deadline) {
				t.Fatalf("the run never came to wait: %+v", run)
			}
		}
	}
	// Waiting on a key and a timer.
	held, err := l.Start(ctx, draft(t, `
		statechart: held
		initial: asking
		states:
		  asking:
		    ticket: verdict
		    timers: { an_hour: 1h }
		    on:
		      an_hour: over
		      verdict: over
		  over: { end: done }
	`, nil), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	run := shown(held, func(r *client.Run) bool { return len(r.Tickets) > 0 })
	if run.Status != instance.Running || !slices.Equal(run.Tickets, []string{"verdict"}) || run.Timers != 1 {
		t.Fatalf("a run waiting on a ticket and a timer is shown as %+v", run)
	}
	// Waiting on a job.
	busy, err := l.Start(ctx, draft(t, `
		flow: busy
		stages:
		  spin: { job: { script: ./spin.js, timeout: 30s } }
	`, map[string]string{"spin.js": "for (;;) {}"}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	run = shown(busy, func(r *client.Run) bool { return r.Jobs > 0 })
	if run.Status != instance.Running || run.Jobs != 1 {
		t.Fatalf("a run waiting on a job is shown as %+v", run)
	}
	// A listing gives the same word, and none is listed as waiting. A run
	// is shown from its record, and its head is written a moment after the
	// step it tells of: the listing is looked at until both heads are in it.
	var rows []*client.Run
	for deadline := time.Now().Add(20 * time.Second); len(rows) != 2; time.Sleep(5 * time.Millisecond) {
		var err error
		if rows, err = l.List(ctx, client.Filter{}); err != nil || time.Now().After(deadline) {
			t.Fatalf("the listing gave %+v, %v", rows, err)
		}
	}
	for _, row := range rows {
		if row.Status != instance.Running {
			t.Fatalf("a run that has started is listed as %s", row.Status)
		}
	}
	if rows, _ := l.List(ctx, client.Filter{Status: instance.Waiting}); len(rows) != 0 {
		t.Fatalf("a run that has started is found by the status waiting: %+v", rows[0])
	}
	for _, id := range []string{held, busy} {
		if _, err := l.Cancel(ctx, id, 0); err != nil {
			t.Fatal(err)
		}
		if run := wait(t, l, id); run.Status != instance.Cancelled {
			t.Fatalf("the cancelled run ended %s", run.Status)
		}
	}
}

// A name for which nothing was ever sent is no run at once, whatever time
// it carries; a start that was sent is waiting while it is good, and
// after that a wait on it ends, saying it was never taken up.
func TestUnstartedRunIsWaitingOnlyWhileItsStartIsGood(t *testing.T) {
	a := arrange(t)
	for _, never := range []string{
		"client/" + store.Stamp(time.Now()) + "-0000000000",
		"client/" + store.Stamp(time.Now().Add(-24*time.Hour)) + "-0000000000",
		"client/" + store.Stamp(time.Now().Add(24*time.Hour)) + "-0000000000",
	} {
		within, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := a.Wait(within, never, time.Millisecond)
		if !errors.Is(err, client.ErrNoRun) || errors.Is(err, client.ErrNeverTaken) || within.Err() != nil {
			t.Fatalf("a wait on %s, which was never sent, gave %v", never, err)
		}
		if _, err := a.Follow(within, never, time.Millisecond, func(client.Entry) {}); !errors.Is(err, client.ErrNoRun) || within.Err() != nil {
			t.Fatalf("a follow of %s, which was never sent, gave %v", never, err)
		}
		cancel()
	}
	d := draft(t, standing, map[string]string{"work.js": "return 1"})
	sent, err := a.Start(ctx, d, client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	if run, err := a.Show(ctx, sent); err != nil || run.Status != instance.Waiting {
		t.Fatalf("a start just sent is shown as %+v, %v", run, err)
	}
	// One named a day ahead of the clock was not named by this client.
	ahead, err := a.Start(ctx, d, client.Start{Run: "client/" + store.Stamp(time.Now().Add(24*time.Hour)) + "-0000000001"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Show(ctx, ahead); !errors.Is(err, client.ErrNeverTaken) {
		t.Fatalf("a start named a day ahead is shown as %v", err)
	}
	// The start's time passes with no controller.
	a.Now = func() time.Time {
		return time.Now().Add(instance.MessageLife + 2*identity.ClockAllowance + time.Second)
	}
	within, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for what, wait := range map[string]func() (*client.Run, error){
		"a wait":   func() (*client.Run, error) { return a.Wait(within, sent, time.Millisecond) },
		"a follow": func() (*client.Run, error) { return a.Follow(within, sent, time.Millisecond, func(client.Entry) {}) },
	} {
		if run, err := wait(); !errors.Is(err, client.ErrNeverTaken) || !strings.Contains(err.Error(), "never taken up") || within.Err() != nil {
			t.Fatalf("%s on a start past its time gave %+v, %v", what, run, err)
		}
	}
}

// uploads counts the blob writes a store has open at once, and notes, when
// a message is written, how many were open and how many had been made.
type uploads struct {
	store.Store
	open, most, made  atomic.Int64
	openAt, madeAt    atomic.Int64
	fail              error // what each blob write after the fifth gives
	messages, refused atomic.Int64
}

func (u *uploads) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	n := u.open.Add(1)
	defer u.open.Add(-1)
	for {
		most := u.most.Load()
		if n <= most || u.most.CompareAndSwap(most, n) {
			break
		}
	}
	time.Sleep(50 * time.Microsecond) // long enough for writes made side by side to meet
	if u.fail != nil && u.made.Load() >= 5 {
		u.refused.Add(1)
		return false, u.fail
	}
	done, err := u.Store.WriteFrom(ctx, name, r)
	if err == nil {
		u.made.Add(1)
	}
	return done, err
}

func (u *uploads) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	if strings.HasPrefix(name, store.Inbox("controller")+"/") {
		u.messages.Add(1)
		u.openAt.Store(u.open.Load())
		u.madeAt.Store(u.made.Load())
	}
	return u.Store.Write(ctx, name, value, how)
}

// A start's uploads, the definition, its files and the input's files, are
// made side by side, a bounded number at a time, and the start is sent once
// every one is in the store.
func Test_client_start_uploads_side_by_side(t *testing.T) {
	const scripts, inner = 40, 3
	var src strings.Builder
	files := map[string]string{}
	src.WriteString("flow: wide\nstages:\n")
	for i := range scripts {
		fmt.Fprintf(&src, "  s%d: { job: { script: ./s%d.js } }\n", i, i)
		files[fmt.Sprintf("s%d.js", i)] = "return 1"
	}
	src.WriteString("  kid:\n    job:\n      machine:\n        flow: kid\n        stages:\n")
	for i := range inner {
		fmt.Fprintf(&src, "          k%d: { job: { script: ./k%d.js } }\n", i, i)
		files[fmt.Sprintf("k%d.js", i)] = "return 2"
	}
	d := draft(t, src.String(), files)
	input := func() map[string]any {
		file := func(name string) client.InputFile {
			return client.InputFile{Name: name, Contents: strings.NewReader(name)}
		}
		return map[string]any{"one": file("one.txt"), "more": []any{file("two.txt"), map[string]any{"three": file("three.txt")}}}
	}
	const blobs = scripts + inner + 2 + 3 // the two definitions and the input's files

	a := arrange(t)
	u := &uploads{Store: a.hooked}
	c := client.New(a.ID, a.ctl.Card, u)
	run, err := c.Start(ctx, d, client.Start{Input: input()})
	if err != nil {
		t.Fatal(err)
	}
	if most := u.most.Load(); most < 2 || most > store.SideBySide {
		t.Fatalf("a start wrote %d blobs at once; the bound is %d", most, store.SideBySide)
	}
	if u.messages.Load() != 1 || u.openAt.Load() != 0 || u.madeAt.Load() != blobs {
		t.Fatalf("the start was sent with %d blob writes open and %d of %d made", u.openAt.Load(), u.madeAt.Load(), blobs)
	}
	if names, err := a.owner.List(ctx, store.Blobs("controller", run)); err != nil || len(names) != blobs {
		t.Fatalf("the run's record holds %d blobs, %v", len(names), err)
	}

	// A write that fails is the start's error, and nothing is sent.
	refused := errors.New("refused")
	u = &uploads{Store: a.hooked, fail: refused}
	c = client.New(a.ID, a.ctl.Card, u)
	if _, err := c.Start(ctx, d, client.Start{Input: input()}); !errors.Is(err, refused) || u.messages.Load() != 0 {
		t.Fatalf("a start whose upload failed gave %v and sent %d messages", err, u.messages.Load())
	}
}
