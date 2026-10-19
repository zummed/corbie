package client_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

// A child started several steps back is followed, though the parent's
// latest step no longer mentions it.
func Test_inst_watch_finds_child_from_earlier_step(t *testing.T) {
	a := arrange(t)
	root, kid := a.named("root"), a.named("kid")
	a.step(root, 1, instance.Running, child("kid", kid))
	a.step(root, 2, instance.Running)
	a.step(root, 3, instance.Running)
	a.step(kid, 1, instance.Running)
	// The run ends only once the follow has listed the child's steps: by
	// then the child is being followed from the parent's first step.
	a.hooked.before = func(act, name string) error {
		if act == "list" && name == store.Steps("controller", kid) {
			a.step(kid, 2, instance.Done)
			a.step(root, 4, instance.Done)
		}
		return nil
	}
	run, told := a.follow(root)
	if run.Status != instance.Done || !slices.Contains(told, "kid step 1") || !slices.Contains(told, "kid step 2") {
		t.Fatalf("the follow ended %s and was told %v", run.Status, told)
	}
}

// A run that had ended before it was followed is followed all the way
// down: its child, that child's child, and the one below that.
func TestFollowOfAnEndedRunReachesEveryLevel(t *testing.T) {
	a := arrange(t)
	names := []string{a.named("one"), a.named("two"), a.named("three"), a.named("four")}
	for i, name := range names {
		if i+1 < len(names) {
			a.step(name, 1, instance.Running, child("below", names[i+1]))
			a.step(name, 2, instance.Done)
		} else {
			a.step(name, 1, instance.Done)
		}
	}
	run, told := a.follow(names[0])
	want := []string{"one step 1", "one step 2", "two step 1", "two step 2", "three step 1", "three step 2", "four step 1"}
	slices.Sort(told)
	slices.Sort(want)
	if run.Status != instance.Done || !slices.Equal(told, want) {
		t.Fatalf("the follow ended %s and was told %v, want %v", run.Status, told, want)
	}
}

// One look that finds several entries gives them in the order they were
// written, whichever instance and whichever job each came from: a job's
// emit before the step that took its result, and a child's last step before
// the step of its parent that followed it.
func TestFollowGivesTheEntriesOfOneLookInTheOrderTheyWereWritten(t *testing.T) {
	a := arrange(t)
	root, kid := a.named("root"), a.named("kid")
	a.step(root, 1, instance.Running, a.job("work"), child("kid", kid))
	a.emit(root, "work", 1, 1)
	a.step(kid, 1, instance.Running)
	a.emit(root, "work", 2, 2)
	a.step(kid, 2, instance.Done)
	a.step(root, 2, instance.Running)
	a.step(root, 3, instance.Done)
	_, told := a.follow(root)
	want := []string{"root step 1", "root emit 1", "kid step 1", "root emit 2", "kid step 2", "root step 2", "root step 3"}
	if !slices.Equal(told, want) {
		t.Fatalf("the follow gave %v, want %v", told, want)
	}
}

// A child with nothing in its record that can be read is not waited on once
// its parent has ended; a child that can be read is followed to its own
// end, also after its parent's.
func Test_inst_watch_ends_despite_unreadable_child(t *testing.T) {
	a := arrange(t)
	root, never, slow, sealed := a.named("root"), a.named("never"), a.named("slow"), a.named("sealed")
	a.step(root, 1, instance.Running, child("never", never), child("slow", slow), child("sealed", sealed))
	a.step(root, 2, instance.Cancelled)
	a.step(slow, 1, instance.Running)
	// One child's record is sealed to the controller alone.
	at := store.Step("controller", sealed, 1)
	value, err := a.ctl.Write(at, time.Now(), time.Time{}, &instance.Step{Number: 1}, a.ctl.Card)
	a.put(at, value, err)
	// The readable child ends only at the fourth look at it, well after the
	// parent has been seen to have ended.
	looks := 0
	a.hooked.before = func(act, name string) error {
		if act == "list" && name == store.Steps("controller", slow) {
			if looks++; looks == 4 {
				a.step(slow, 2, instance.Cancelled)
			}
		}
		return nil
	}
	run, told := a.follow(root)
	if run.Status != instance.Cancelled {
		t.Fatalf("the follow ended with the run %s", run.Status)
	}
	if !slices.Contains(told, "slow step 2") {
		t.Fatalf("a child that was still running was let go when its parent ended: told %v", told)
	}
	for _, entry := range told {
		if strings.HasPrefix(entry, "never ") || strings.HasPrefix(entry, "sealed ") {
			t.Fatalf("an entry was given of a record that cannot be read: %v", told)
		}
	}
}

// unreachable makes a store out of reach the first time each name is asked
// for, so every look of a reading fails once before it is answered.
func unreachable(a *arranged) (failed func() int) {
	asked := map[string]bool{}
	count := 0
	a.hooked.before = func(act, name string) error {
		if asked[act+" "+name] {
			return nil
		}
		asked[act+" "+name] = true
		count++
		return fmt.Errorf("%w: no answer", store.ErrUnreachable)
	}
	return func() int {
		a.hooked.mu.Lock()
		defer a.hooked.mu.Unlock()
		return count
	}
}

// A look that cannot reach the store is made again, and nothing is given
// as changed, gone or failed on its account.
func Test_inst_watch_store_unreachable_reports_nothing(t *testing.T) {
	a := arrange(t)
	root, kid := a.named("root"), a.named("kid")
	a.step(root, 1, instance.Running, child("kid", kid), a.job("work"))
	a.emit(root, "work", 1, "from the job")
	a.step(root, 2, instance.Done)
	a.step(kid, 1, instance.Done)
	a.head(root, instance.Done, 2)
	a.head(kid, instance.Done, 1)

	// Following a record, and waiting for a run.
	failed := unreachable(a)
	run, told := a.follow(root)
	want := []string{"root step 1", "root step 2", `root emit "from the job"`, "kid step 1"}
	slices.Sort(told)
	slices.Sort(want)
	if run.Status != instance.Done || !slices.Equal(told, want) || failed() < 5 {
		t.Fatalf("with the store out of reach %d times, the follow ended %s and was told %v, want %v", failed(), run.Status, told, want)
	}
	unreachable(a)
	within, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if run, err := a.Wait(within, root, time.Millisecond); err != nil || run.Status != instance.Done {
		t.Fatalf("with the store out of reach, a wait gave %+v, %v", run, err)
	}

	// Watching a listing: the runs are given once, and while the store is
	// out of reach nothing is given as changed or gone.
	var mu sync.Mutex
	var updates []client.Update
	out, failures := false, 0
	a.hooked.before = func(act, name string) error {
		if out {
			failures++
			return fmt.Errorf("%w: no answer", store.ErrUnreachable)
		}
		return nil
	}
	watching, stop := context.WithCancel(ctx)
	defer stop()
	go a.Watch(watching, client.Filter{Finished: true}, time.Millisecond, func(u client.Update) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
	})
	until := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s never came; the watcher was given %+v", what, updates)
			}
		}
	}
	given := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(updates)
	}
	until("the first listing", func() bool { return given() == 3 })
	a.hooked.mu.Lock()
	out = true
	a.hooked.mu.Unlock()
	until("looks at a store out of reach", func() bool {
		a.hooked.mu.Lock()
		defer a.hooked.mu.Unlock()
		return failures >= 5
	})
	a.hooked.mu.Lock()
	out = false
	a.hooked.mu.Unlock()
	// A run that changes afterwards is the next thing given.
	other := a.named("other")
	a.head(other, instance.Running, 1)
	until("the run that appeared afterwards", func() bool { return given() == 4 })
	mu.Lock()
	if !updates[2].Listed || updates[3].Run == nil || updates[3].Run.ID != other || !updates[3].New {
		t.Fatalf("with the store out of reach in between, the watcher was given %+v", updates)
	}
	mu.Unlock()

	// A look that is turned away after it has told one run's ending does not
	// tell it again, though the listing does not take in the finished.
	b := arrange(t)
	b.hooked.silent = true
	one, two := b.named("one"), b.named("two")
	b.head(one, instance.Running, 1)
	b.head(two, instance.Running, 1)
	var ends []client.Update
	turned := false
	go b.Watch(watching, client.Filter{}, time.Millisecond, func(u client.Update) {
		mu.Lock()
		defer mu.Unlock()
		if !u.Listed {
			ends = append(ends, u)
			return
		}
		// Both end, and the first read of the second's finished head is
		// turned away.
		ends = nil
		for _, id := range []string{one, two} {
			b.head(id, instance.Done, 2)
			b.owner.Delete(ctx, store.RunningHead("controller", id), store.Overwrite)
		}
		b.hooked.mu.Lock()
		defer b.hooked.mu.Unlock()
		b.hooked.before = func(act, name string) error {
			if act == "read" && name == store.FinishedHead("controller", two) && !turned {
				turned = true
				return fmt.Errorf("%w: no answer", store.ErrUnreachable)
			}
			return nil
		}
	})
	until("both runs' endings", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ends) >= 2
	})
	time.Sleep(20 * time.Millisecond) // long enough for a second telling, were there one
	mu.Lock()
	defer mu.Unlock()
	if len(ends) != 2 || ends[0].Run.ID != one || ends[1].Run.ID != two || ends[0].New || ends[1].New ||
		ends[0].Run.Status != instance.Done || ends[1].Run.Status != instance.Done {
		t.Fatalf("with the store out of reach partway through a look, the watcher was given %+v", ends)
	}
}

// A name that was listed and is gone when it is read is passed over: a
// head that moved, a step that was trimmed, an emit that was removed.
func Test_store_listed_name_gone_on_read_is_skipped(t *testing.T) {
	a := arrange(t)
	gone, stays := a.named("gone"), a.named("stays")
	a.head(gone, instance.Running, 1)
	a.head(stays, instance.Running, 1)
	// A long-running run whose older steps are trimmed: the first step there
	// is the fifth.
	long := a.named("long")
	for number := 5; number <= 7; number++ {
		a.step(long, number, instance.Running, a.job(fmt.Sprintf("work.%d", number)))
		a.emit(long, fmt.Sprintf("work.%d", number), 1, number)
	}
	a.step(long, 8, instance.Done)
	a.head(long, instance.Done, 8)
	// Each is removed by another copy between the listing and the read.
	remove := map[string]bool{
		store.RunningHead("controller", gone):       true,
		store.Step("controller", long, 5):           true,
		store.Emit("controller", long, "work.6", 1): true,
	}
	a.hooked.before = func(act, name string) error {
		if act == "read" && remove[name] {
			if _, err := a.owner.Delete(ctx, name, store.Overwrite); err != nil {
				t.Error(err)
			}
		}
		return nil
	}
	runs, err := a.List(ctx, client.Filter{})
	if err != nil || len(runs) != 1 || runs[0].ID != stays {
		t.Fatalf("with a head gone between the listing and the read, the listing gave %+v, %v", runs, err)
	}
	run, told := a.follow(long)
	want := []string{"long step 6", "long step 7", "long step 8", "long emit 7"}
	slices.Sort(told)
	slices.Sort(want)
	if run.Status != instance.Done || !slices.Equal(told, want) {
		t.Fatalf("with a step and an emit gone between the listing and the read, the follow ended %s and was told %v, want %v", run.Status, told, want)
	}
}

// A watcher of a listing that does not take in the finished is still given
// a run once more when it ends, saying how.
func TestWatchGivesARunOnceMoreWhenItEnds(t *testing.T) {
	a := arrange(t)
	id := a.named("ends")
	a.head(id, instance.Running, 1)
	var mu sync.Mutex
	var updates []client.Update
	watching, stop := context.WithCancel(ctx)
	defer stop()
	go a.Watch(watching, client.Filter{Status: instance.Running}, time.Millisecond, func(u client.Update) {
		mu.Lock()
		defer mu.Unlock()
		if updates = append(updates, u); len(updates) == 2 {
			// It has been listed: now it ends.
			a.head(id, instance.Done, 2)
			a.owner.Delete(ctx, store.RunningHead("controller", id), store.Overwrite)
		}
	})
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		mu.Lock()
		n := len(updates)
		mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the watcher was given %+v", updates)
		}
	}
	time.Sleep(20 * time.Millisecond) // long enough for a second telling, were there one
	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 3 || updates[2].Run == nil || updates[2].Run.ID != id || updates[2].Run.Status != instance.Done || updates[2].New {
		t.Fatalf("the watcher was given %+v", updates)
	}
}

// A watcher of a listing that does not take in the finished, on a store
// that gives no notices, is told of a run the listing keeps that started
// and ended between two looks: once, as new and ended. A run the listing
// does not keep is not told, and a finished head that was there before the
// watch began is never read.
func Test_inst_watch_reports_run_ended_between_looks(t *testing.T) {
	a := arrange(t)
	a.hooked.silent = true
	before, flash, other := a.named("before"), a.named("flash"), a.named("other")
	a.head(before, instance.Done, 2, "role=out")
	reads := map[string]int{}
	a.hooked.before = func(act, name string) error {
		if act == "read" {
			reads[name]++
		}
		return nil
	}
	var mu sync.Mutex
	var updates []client.Update
	watching, stop := context.WithCancel(ctx)
	defer stop()
	go a.Watch(watching, client.Filter{Labels: map[string]string{"role": "out"}}, time.Millisecond, func(u client.Update) {
		mu.Lock()
		defer mu.Unlock()
		if updates = append(updates, u); u.Listed {
			// Between this look and the next, two runs start and end.
			for _, id := range []string{flash, other} {
				role := map[string]string{flash: "role=out", other: "role=in"}[id]
				a.head(id, instance.Running, 1, role)
				a.head(id, instance.Done, 2, role)
				a.owner.Delete(ctx, store.RunningHead("controller", id), store.Overwrite)
			}
		}
	})
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		mu.Lock()
		n := len(updates)
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a run that started and ended between two looks was never told: the watcher was given %+v", updates)
		}
	}
	time.Sleep(50 * time.Millisecond) // many looks, long enough for a second telling, were there one
	stop()
	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 || !updates[0].Listed || updates[1].Run == nil || updates[1].Run.ID != flash ||
		updates[1].Run.Status != instance.Done || !updates[1].New {
		t.Fatalf("the watcher was given %+v", updates)
	}
	a.hooked.mu.Lock()
	defer a.hooked.mu.Unlock()
	if n := reads[store.FinishedHead("controller", before)]; n != 0 {
		t.Fatalf("a finished head there before the watch began was read %d times", n)
	}
	if n := reads[store.FinishedHead("controller", flash)]; n != 1 {
		t.Fatalf("the head of the run that ended between two looks was read %d times", n)
	}
}

// A start that has been taken, whose head is not there yet, is shown from
// its record.
func TestRunWithAReceiptAndNoHeadIsShownFromItsRecord(t *testing.T) {
	a := arrange(t)
	id := a.named("0000000001")
	a.step(id, 1, instance.Running, a.job("work"))
	if run, err := a.Show(ctx, id); err != nil || run.Status != instance.Running || run.Step != 1 || run.Machine != "arranged" {
		t.Fatalf("with a first step and no head, the run is shown as %+v, %v", run, err)
	}
}

// A start that is taken between a look at the run's record and the look at
// its receipt is shown from its record: the first step is written before
// the receipt that says the start was taken. A taken start with no step is
// a run past keeping.
func TestRunTakenBetweenTwoLooksIsShownFromItsRecord(t *testing.T) {
	a := arrange(t)
	id := a.named("0000000001")
	at := store.Receipt("controller", "client", strings.TrimPrefix(id, "client/"))
	taken := func() {
		value, err := a.ctl.Write(at, time.Now(), time.Time{}, &instance.Receipt{Taken: true}, a.ID.Card)
		a.put(at, value, err)
	}
	a.hooked.before = func(act, name string) error {
		if act == "read" && name == at {
			a.step(id, 1, instance.Running, a.job("work"))
			taken()
		}
		return nil
	}
	if run, err := a.Show(ctx, id); err != nil || run.Status != instance.Running || run.Step != 1 {
		t.Fatalf("a start taken while it was being shown is shown as %+v, %v", run, err)
	}
	gone := a.named("0000000002")
	at = store.Receipt("controller", "client", strings.TrimPrefix(gone, "client/"))
	a.hooked.before = nil
	taken()
	if run, err := a.Show(ctx, gone); !errors.Is(err, client.ErrNoRun) {
		t.Fatalf("a start that was taken and has no record is shown as %+v, %v", run, err)
	}
}

// A blob shown in an entry of a record, in a step or in what a job
// emitted, is fetched by the path it was shown with, as one in the result
// is; and so is a script of the definition.
func TestBlobShownInAnEntryIsFetchedByItsPath(t *testing.T) {
	l := local(t)
	id, err := l.Start(ctx, draft(t, `
		flow: files
		stages:
		  make: { job: { script: ./make.js, input: { given: "${input.given}" } } }
		  kid: { job: { machine: ./kid.yaml, input: { report: "${stages.make.output.report}" } } }
		output: { report: "${stages.make.output.report}", kid: "${stages.kid.output}" }
	`, map[string]string{
		"make.js": `
			emit("draft", { early: blobs.make("an early draft") })
			emit("draft", { early: blobs.make("a later draft") })
			return { report: blobs.make("the report", { type: "text/markdown" }), given: input.given }`,
		"kid.yaml": `
			flow: kid
			stages:
			  again: { job: { script: ./again.js, input: "${input}" } }
			output: "${stages.again.output}"`,
		"again.js": `
			emit("seen", { mine: blobs.make("made by the child") })
			return { copy: blobs.make(blobs.read(input.report, { as: "text" }) + ", copied") }`,
	}), client.Start{Input: map[string]any{"given": client.InputFile{Name: "notes.txt", Contents: strings.NewReader("given with the start")}}})
	if err != nil {
		t.Fatal(err)
	}
	// Every blob shown: the instance its entry came from, and its path.
	shown := map[[2]string]bool{}
	var gather func(id string, v any)
	gather = func(id string, v any) {
		switch v := v.(type) {
		case map[string]any:
			if path, ok := v["$blob"].(string); ok {
				shown[[2]string{id, path}] = true
				return
			}
			for _, inner := range v {
				gather(id, inner)
			}
		case []any:
			for _, inner := range v {
				gather(id, inner)
			}
		}
	}
	run, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) { gather(e.ID, plainOf(t, e)) })
	if err != nil || run.Status != instance.Done {
		t.Fatalf("the run ended %+v, %v", run, err)
	}
	gather(id, plainOf(t, run.Output))
	def, err := l.Definition(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	gather(id, plainOf(t, def))
	contents := map[string]int{}
	for at := range shown {
		from, path := at[0], at[1]
		reader, file, err := l.Fetch(ctx, from, path)
		if err != nil {
			t.Errorf("the path %q, shown for a blob of %s, cannot be fetched: %v", path, from, err)
			continue
		}
		text, err := io.ReadAll(reader)
		if err != nil || int64(len(text)) != file.Size {
			t.Errorf("the blob at %q reads %q, %v; its size is given as %d", path, text, err, file.Size)
		}
		if strings.HasPrefix(path, "definition/scripts.") {
			text = []byte("script " + strings.TrimSpace(strings.SplitN(string(text), "\n", 2)[0]))
		}
		contents[string(text)]++
	}
	// Each was shown somewhere, and each path gave the blob it was shown
	// for: the two drafts stand at one place in two values of one job.
	for _, want := range []string{"given with the start", "an early draft", "a later draft", "the report", "made by the child", "the report, copied",
		`script emit("draft", { early: blobs.make("an early draft") })`} {
		if contents[want] == 0 {
			t.Errorf("no path shown fetched %q; the paths were %v", want, shown)
		}
	}
	if contents["an early draft"] != 1 || contents["a later draft"] != 1 {
		t.Errorf("the two drafts, emitted one after the other, were fetched %d and %d times", contents["an early draft"], contents["a later draft"])
	}
	// A path that names nothing, in the result or in an entry, gives no
	// file, and the keys of a run are not files.
	for _, path := range []string{"nothing", "steps/1/events[0].data.nothing", "steps/99/events[0].data", "emits/make/9/values[0].data.early",
		"emits/nobody/1/values[0].data.early", "definition/scripts.nothing", "steps/1/state.seed"} {
		if _, _, err := l.Fetch(ctx, id, path); !errors.Is(err, client.ErrNoFile) {
			t.Errorf("the path %q gave %v", path, err)
		}
	}
}
