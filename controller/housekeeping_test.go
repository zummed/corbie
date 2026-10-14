package controller_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

const kept = `
	statechart: kept
	keep: 1d
	initial: waiting
	states:
	  waiting:
	    on: { stop: stopped, tick: waiting }
	  stopped: { end: done }
`

// An instance is kept for as long as its definition says after it ends,
// and then its record goes, and then its head.
func TestFinishedInstanceGoesWhenItsKeepHasPassed(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	w.client.Send(ctx, run, "stop", nil, 0)
	w.settle()
	if head := w.head(run); head == nil || head.Status != instance.Done || head.Keep.String() != "24h0m0s" {
		t.Fatalf("the run ended with the head %+v", head)
	}
	w.tick(23 * time.Hour)
	w.settle()
	if w.head(run) == nil || len(w.list(store.Record("ctl", run))) == 0 {
		t.Fatal("the run was removed before its keep had passed")
	}
	w.tick(2 * time.Hour)
	w.settle()
	if w.head(run) != nil {
		t.Fatal("the head is still there after the keep")
	}
	for _, name := range w.list("ctl") {
		if strings.Contains(name, run) && !strings.HasSuffix(name, "/trim") {
			t.Fatalf("something of the run is still in the store: %s", name)
		}
	}
}

// While an instance runs, its step entries older than its keep are
// removed, its latest step excepted.
func TestOlderHistoryOfARunningInstanceIsTrimmed(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	for range 30 {
		w.tick(2 * time.Hour)
		w.client.Send(ctx, run, "tick", nil, 0)
		w.settle()
	}
	steps := w.list(store.Steps("ctl", run))
	if len(steps) >= 31 || len(steps) < 2 {
		t.Fatalf("after sixty hours with a keep of a day, %d steps remain", len(steps))
	}
	// It carries on correctly from what is left.
	w.client.Send(ctx, run, "stop", nil, 0)
	w.settle()
	if head := w.head(run); head.Status != instance.Done || head.Step != 32 {
		t.Fatalf("after trimming the run ended as %+v", head)
	}
	if def, err := w.client.Definition(ctx, run); err != nil || def.Name != "kept" {
		t.Fatalf("the definition after trimming: %v", err)
	}
}

func TestReceiptsAndUploadsNeverStartedGo(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	w.settle()
	message, _ := w.client.Send(ctx, run, "tick", nil, 0)
	// A message under a name that carries no time gets a receipt too.
	w.leave(w.c1, "a-name-of-the-clients-own", &instance.Message{Kind: instance.KindEvent, Instance: run, Event: &machine.Event{Name: "tick"}})
	w.settle()
	if w.receipt("a-name-of-the-clients-own") == nil {
		t.Fatal("a message under a name with no time got no receipt")
	}
	// An upload for a run that is never started.
	orphan, _ := w.client.NewRun()
	w.blob(orphan, w.clock(), "left behind")
	w.tick(23 * time.Hour)
	w.settle()
	if w.receipt(message) == nil || w.receipt("a-name-of-the-clients-own") == nil || len(w.list(store.Record("ctl", orphan))) != 1 {
		t.Fatal("a receipt or an upload went within the day")
	}
	w.tick(2 * time.Hour)
	w.settle()
	if w.receipt(message) != nil || w.receipt("a-name-of-the-clients-own") != nil {
		t.Fatal("a receipt is still there after a day")
	}
	if got := w.list(store.Record("ctl", orphan)); len(got) != 0 {
		t.Fatalf("an upload that never started is still there: %v", got)
	}
	// The uploads of the run that did start are untouched.
	if def, err := w.client.Definition(ctx, run); err != nil || def == nil {
		t.Fatalf("the started run's definition: %v", err)
	}
}

// Housekeeping has a wake of its own, once an hour, as everything owed to
// time has: the first pass that finds something owed sets one on a new
// store, a pass is told when it is due, and the next is set before the one
// that was due is removed.
func TestHousekeepingIsAWake(t *testing.T) {
	w := newWorld(t)
	// The world starts at noon: the pass that takes the first start sets the
	// wake for one.
	w.submit(kept, nil, nil)
	if next := w.pass(); len(w.house()) != 1 || !strings.Contains(w.house()[0], "T130000.000Z") || !next.Equal(w.clock().Truncate(time.Hour).Add(time.Hour)) {
		t.Fatalf("a new store has the housekeeping wakes %v, and the pass says it is next due at %s", w.house(), next)
	}
	// A controller that was stopped for days does one round, and has one
	// wake after it.
	w.tick(50 * time.Hour)
	w.settle()
	if got := w.house(); len(got) != 1 || !strings.Contains(got[0], "T150000.000Z") {
		t.Fatalf("after two days stopped the housekeeping wakes are %v", got)
	}
}

// house lists the controller's own wakes for its hourly housekeeping.
func (w *world) house() []string {
	var names []string
	for _, name := range w.list(store.Wakes("ctl")) {
		if strings.HasSuffix(name, "/house/keep") {
			names = append(names, name)
		}
	}
	return names
}

// cutAfter is a store that ends a pass after a number of changes, deletes
// among them, as a host's time limit does.
type cutAfter struct {
	store.Store
	left atomic.Int64
	end  context.CancelFunc
}

func (s *cutAfter) spend() {
	if s.left.Add(-1) < 0 {
		s.end()
	}
}

func (s *cutAfter) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	s.spend()
	return s.Store.Write(ctx, name, value, how)
}

func (s *cutAfter) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	s.spend()
	return s.Store.Delete(ctx, name, how)
}

func Test_ctl_housekeeping_gets_through_all_across_fresh_starts(t *testing.T) {
	w := newWorld(t)
	// Fifty runs that have ended, with their receipts, and an upload for a
	// run that never started.
	for range 50 {
		run := w.submit(kept, nil, nil)
		w.settle()
		w.client.Send(ctx, run, "stop", nil, 0)
		w.settle()
		w.tick(time.Second)
	}
	orphan, _ := w.client.NewRun()
	w.blob(orphan, w.clock(), "left behind")
	w.tick(26 * time.Hour) // every one of them is past keeping
	late := w.submit(kept, nil, nil)
	past := func() (names []string) {
		for _, under := range []string{store.Finished("ctl"), store.Records("ctl"), "ctl/receipts", store.Wakes("ctl")} {
			for _, name := range w.list(under) {
				if !strings.Contains(name, late[strings.IndexByte(late, '/')+1:]) && !strings.HasSuffix(name, "/house/keep") {
					names = append(names, name)
				}
			}
		}
		return names
	}
	before := len(past())
	if before < 400 {
		t.Fatalf("only %d names are past keeping", before)
	}
	inner := w.ctrl.Store
	w.noted()
	for n := 1; len(past()) > 0; n++ {
		if n > 100 {
			t.Fatalf("after %d passes, each started afresh, %d of %d names past keeping are still there: %v", n, len(past()), before, past()[:min(len(past()), 5)])
		}
		// A controller started afresh, whose pass is over after forty
		// changes to the store.
		limited, end := context.WithCancel(ctx)
		s := &cutAfter{Store: inner, end: end}
		s.left.Store(40)
		w.afresh(s)
		w.ctrl.Pass(limited)
		end()
		if n == 1 && len(past()) == 0 {
			t.Fatal("one pass of forty changes removed everything: nothing was tried")
		}
	}
	// The message that was waiting was handled before anything was removed.
	stepAt, removedAt := -1, -1
	for i, op := range w.noted() {
		switch {
		case !op.Done:
		case op.Act == store.ActAdd && op.Name == store.Step("ctl", late, 1):
			stepAt = i
		case op.Act == store.ActDelete && removedAt < 0 && (strings.HasPrefix(op.Name, store.Records("ctl")+"/") || strings.HasPrefix(op.Name, store.Finished("ctl")+"/")):
			removedAt = i
		}
	}
	if stepAt < 0 || removedAt < stepAt {
		t.Fatalf("the waiting start's step was written at %d and the first removal made at %d", stepAt, removedAt)
	}
	if head := w.head(late); head == nil || head.Status != instance.Running {
		t.Fatalf("the run that is not past keeping is %+v", head)
	}

	// Stopped at each point of removing a run and what is below it, the
	// removal is finished by a controller started afresh; and at the stop
	// the finished head is there for as long as any entry of the record is.
	for k := int64(0); ; k++ {
		w := newWorld(t)
		run := w.submit(family, nil, nil)
		for range 8 {
			w.settle()
			w.tick(500 * time.Millisecond)
		}
		if head := w.head(run); head == nil || head.Status != instance.Done {
			t.Fatalf("the family ended as %+v", head)
		}
		all := append(w.list(store.Finished("ctl")), w.list(store.Records("ctl"))...)
		w.tick(8 * 24 * time.Hour)
		inner := w.ctrl.Store
		s := &stopping{Store: inner}
		s.left.Store(k)
		w.afresh(s)
		w.ctrl.Pass(ctx)
		if !s.stopped.Load() {
			if k < 10 {
				t.Fatalf("removing the family took only %d writes", k)
			}
			break
		}
		for _, at := range w.list(store.Records("ctl")) {
			inst := store.InstanceOf("ctl", at)
			if data, _, _ := w.owner.Read(ctx, store.FinishedHead("ctl", inst)); data == nil {
				t.Fatalf("stopped after %d writes, %s is in the store and its instance has no head", k, at)
			}
		}
		w.afresh(inner)
		w.settle()
		for _, name := range all {
			if data, _, _ := w.owner.Read(ctx, name); data != nil {
				t.Fatalf("stopped after %d writes of a removal, it was never finished: %s is still there", k, name)
			}
		}
		if left := w.list(store.Wakes("ctl")); len(left) != 1 {
			t.Fatalf("stopped after %d writes of a removal, these wakes are left: %v", k, left)
		}
	}
}

// goesFirst says whether everything of one instance was deleted before
// anything of another's record was.
func goesFirst(ops []store.Op, first, then string) error {
	lastOfFirst, firstOfThen := -1, -1
	for i, op := range ops {
		if !op.Done || op.Act != store.ActDelete {
			continue
		}
		switch {
		case op.Name == store.FinishedHead("ctl", first) || op.Name == store.RunningHead("ctl", first) || strings.HasPrefix(op.Name, store.Record("ctl", first)+"/"):
			lastOfFirst = i
		case strings.HasPrefix(op.Name, store.Record("ctl", then)+"/") && firstOfThen < 0:
			firstOfThen = i
		}
	}
	if lastOfFirst < 0 || firstOfThen < 0 {
		return fmt.Errorf("one of the two was not removed (the last delete of the first is at %d, the first delete of the second at %d)", lastOfFirst, firstOfThen)
	}
	if lastOfFirst > firstOfThen {
		return fmt.Errorf("something of the one below was still there (deleted at %d) when the record above began to go (at %d)", lastOfFirst, firstOfThen)
	}
	return nil
}

// below gives the instance a parent's job was started as.
func (w *world) below(parent string) string {
	w.t.Helper()
	for _, under := range []string{store.Running("ctl"), store.Finished("ctl")} {
		for _, at := range w.list(under) {
			inst := store.InstanceOf("ctl", at)
			if head := w.head(inst); head != nil && head.Parent == parent {
				return inst
			}
		}
	}
	w.t.Fatalf("nothing is below %s", parent)
	return ""
}

const threeDeep = `
	statechart: top
	keep: 1d
	initial: working
	states:
	  working:
	    job:
	      machine:
	        statechart: middle
	        keep: 30d
	        initial: working
	        states:
	          working:
	            job:
	              machine:
	                statechart: bottom
	                initial: waiting
	                states:
	                  waiting: { on: { end: over } }
	                  over: { end: done }
	            on:
	              done: over
	              end: { send: { to: working, event: end } }
	          over: { end: done }
	    on:
	      done: over
	      end: { send: { to: working, event: end } }
	  over: { end: done }
`

func Test_inst_removal_takes_children_first(t *testing.T) {
	// Three machines, one below another, end from the bottom up. The one in
	// the middle says it is kept a month, under a top kept a day.
	w := newWorld(t)
	top := w.submit(threeDeep, nil, nil)
	w.settle()
	middle := w.below(top)
	bottom := w.below(middle)
	w.client.Send(ctx, top, "end", nil, 0)
	w.settle()
	for _, inst := range []string{top, middle, bottom} {
		if head := w.head(inst); head.Status != instance.Done {
			t.Fatalf("%s ended as %+v", inst, head)
		}
	}
	w.tick(23 * time.Hour)
	w.settle()
	for _, inst := range []string{top, middle, bottom} {
		if w.head(inst) == nil {
			t.Fatalf("%s went before a day had passed", inst)
		}
	}
	w.tick(2 * time.Hour)
	w.noted()
	w.settle()
	ops := w.noted()
	if err := goesFirst(ops, bottom, middle); err != nil {
		t.Fatalf("the bottom and the middle: %v", err)
	}
	if err := goesFirst(ops, middle, top); err != nil {
		t.Fatalf("the middle and the top: %v", err)
	}
	if left := append(append(w.of(top), w.of(middle)...), w.of(bottom)...); len(left) != 0 {
		t.Fatalf("left in the store: %v", left)
	}

	// The top is cancelled, and what is below it ends after it: minutes
	// later, so that by its own wake it would outlive the top.
	w = newWorld(t)
	top = w.submit(threeDeep, nil, nil)
	w.settle()
	middle = w.below(top)
	bottom = w.below(middle)
	w.client.Cancel(ctx, top, 0)
	w.tick(4 * time.Minute)
	w.pass() // the top ends as of when its cancel was sent
	w.tick(4 * time.Minute)
	w.settle() // and now the two below
	for _, inst := range []string{top, middle, bottom} {
		if head := w.head(inst); head.Status != instance.Cancelled {
			t.Fatalf("%s ended as %+v", inst, head)
		}
	}
	if !w.head(middle).Ended.After(w.head(top).Ended) {
		t.Fatalf("the middle ended at %s and the top at %s", w.head(middle).Ended, w.head(top).Ended)
	}
	w.tick(24*time.Hour - 7*time.Minute) // a day after the top ended, and not yet after the others did
	w.noted()
	w.settle()
	ops = w.noted()
	if err := goesFirst(ops, bottom, middle); err != nil {
		t.Fatalf("ended after the top, the bottom and the middle: %v", err)
	}
	if err := goesFirst(ops, middle, top); err != nil {
		t.Fatalf("ended after the top, the middle and the top: %v", err)
	}
	// The wakes of those that went early find nothing, and go.
	w.tick(time.Hour)
	w.settle()
	if left := append(append(w.of(top), w.of(middle)...), w.of(bottom)...); len(left) != 0 {
		t.Fatalf("left in the store: %v", left)
	}

	// A machine that started another long ago has trimmed the step that
	// did so. The two end in the same instant, and still the one below
	// goes first, in every run.
	for round := range 8 {
		w := newWorld(t)
		top := w.submit(`
			statechart: long
			keep: 1d
			initial: working
			states:
			  working:
			    job:
			      machine:
			        statechart: kid
			        initial: waiting
			        states:
			          waiting: { on: { end: over } }
			          over: { end: done }
			    on:
			      done: over
			      tick: { send: { to: parent, event: ticked } }
			      end: { send: { to: working, event: end } }
			  over: { end: done }
		`, nil, nil)
		w.settle()
		kid := w.below(top)
		for range 30 {
			w.tick(2 * time.Hour)
			w.client.Send(ctx, top, "tick", nil, 0)
			w.settle()
		}
		if steps := w.list(store.Steps("ctl", top)); len(steps) == 0 || store.Number(steps[0]) == 1 {
			t.Fatalf("the step that started the machine below was not trimmed: %v", steps)
		}
		w.client.Send(ctx, top, "end", nil, 0)
		w.settle()
		if a, b := w.head(top), w.head(kid); a.Status != instance.Done || !a.Ended.Equal(b.Ended) {
			t.Fatalf("the two ended as %+v and %+v", a, b)
		}
		w.tick(25 * time.Hour)
		w.noted()
		w.settle()
		if err := goesFirst(w.noted(), kid, top); err != nil {
			t.Fatalf("round %d, with the step that started it trimmed: %v", round, err)
		}
	}
}

// blob puts a blob into a run's record, as a party that may, under a name
// that says it was made at a time of the test's choosing.
func (w *world) blob(run string, made time.Time, contents string) blob.Ref {
	w.t.Helper()
	random := make([]byte, 10)
	rand.Read(random)
	name := store.Stamp(made) + "-" + hex.EncodeToString(random)
	ref, err := blob.Write(ctx, w.owner, store.Blob("ctl", run, name), "text/plain", blob.NewKey(), strings.NewReader(contents))
	if err != nil {
		w.t.Fatal(err)
	}
	return ref
}

func (w *world) there(ref blob.Ref) bool {
	_, mark, err := w.owner.Read(ctx, ref.At)
	if err != nil {
		w.t.Fatal(err)
	}
	return mark != store.None
}

func Test_inst_trim_spares_blob_of_a_recent_entry(t *testing.T) {
	w := newWorld(t)
	started := w.clock()
	// The run's input holds a file for as long as it runs.
	run, _ := w.client.NewRun()
	held := w.blob(run, started, "held by the current state")
	if _, err := w.client.Start(ctx, w.draft(kept, nil), client.Start{Run: run, Input: map[string]any{"file": held.Value()}}); err != nil {
		t.Fatal(err)
	}
	w.settle()
	// Two files no state refers to: one made as the run began, and one made
	// twenty hours before the trim, which a step or an emit of those twenty
	// hours may refer to.
	old := w.blob(run, started, "made long ago")
	recent := w.blob(run, started.Add(52*time.Hour), "made lately")
	for range 36 {
		w.tick(2 * time.Hour)
		w.client.Send(ctx, run, "tick", nil, 0)
		w.settle()
	}
	// Three days on, with a keep of a day, the history was last trimmed at
	// the start of the third day.
	if w.there(old) {
		t.Fatal("a file older than the keep that no state refers to was not trimmed")
	}
	if !w.there(recent) {
		t.Fatal("a file younger than the keep was trimmed, though an entry as young may refer to it")
	}
	if !w.there(held) {
		t.Fatal("a file the current state refers to was trimmed")
	}
	// Once it is older than the keep as well, it goes.
	w.tick(24 * time.Hour)
	w.settle()
	if w.there(recent) || !w.there(held) {
		t.Fatalf("a day later the file no state refers to is there: %v; the one the state holds: %v", w.there(recent), w.there(held))
	}
}

func Test_inst_trim_keeps_blob_a_grandchild_uses(t *testing.T) {
	w := newWorld(t)
	w.enrol(w.party("r1", identity.Runner), permission.Layer{Allow: []string{"all"}}, nil)
	// A file is passed down as an event's data, from the top to the middle
	// and on to the bottom, where a job is given it. Neither the top's
	// state nor the middle's mentions it afterwards.
	top := w.submit(`
		statechart: top
		keep: 1d
		initial: working
		states:
		  working:
		    job:
		      machine:
		        statechart: middle
		        initial: working
		        states:
		          working:
		            job:
		              machine:
		                statechart: bottom
		                initial: idle
		                states:
		                  idle: { on: { file: busy } }
		                  busy:
		                    job: { script: ./work.js, input: "${event.data}", timeout: 30d }
		                    on: { end: over }
		                  over: { end: done }
		            on:
		              done: over
		              file: { send: { to: working, event: file, data: "${event.data}" } }
		              end: { send: { to: working, event: end } }
		          over: { end: done }
		    on:
		      file: { send: { to: working, event: file, data: "${event.data}" } }
		      end: { send: { to: working, event: end } }
		      tick: { send: { to: parent, event: ticked } }
	`, map[string]string{"work.js": "return 1"}, nil)
	w.settle()
	middle := w.below(top)
	bottom := w.below(middle)
	file := w.blob(top, w.clock(), "handed down")
	w.client.Send(ctx, top, "file", file.Value(), 0)
	w.settle()
	holds := func(inst string) bool {
		data, err := identity.Encode(w.latest(inst).State)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(string(data), file.At)
	}
	if holds(top) || holds(middle) || !holds(bottom) || w.head(bottom).Jobs != 1 {
		t.Fatalf("the file is held by the top: %v, the middle: %v, the bottom: %v", holds(top), holds(middle), holds(bottom))
	}
	// Three keeps later the file is still in the top's record, where the
	// bottom's job reads it.
	for range 36 {
		w.tick(2 * time.Hour)
		w.client.Send(ctx, top, "tick", nil, 0)
		w.settle()
	}
	if steps := w.list(store.Steps("ctl", top)); store.Number(steps[0]) == 1 {
		t.Fatal("the top's history was not trimmed")
	}
	if !w.there(file) {
		t.Fatal("a file that a machine two levels down still uses was trimmed from the record it is read in")
	}
	// The two below end; a keep later nothing refers to the file, and it
	// goes.
	w.client.Send(ctx, top, "end", nil, 0)
	w.settle()
	if head := w.head(bottom); head.Status != instance.Done || w.head(top).Status != instance.Running {
		t.Fatalf("the bottom ended as %+v", head)
	}
	w.tick(49 * time.Hour)
	w.settle()
	if w.there(file) {
		t.Fatal("the file is still there a keep after the last machine that used it ended")
	}
}

// With more past keeping than one pass removes, a pass removes some, says
// there is more to do at once, and the passes that follow remove the rest.
func TestHousekeepingIsDoneALittleAtATime(t *testing.T) {
	w := newWorld(t)
	const runs = 70
	for range runs {
		run := w.submit(kept, nil, nil)
		w.settle()
		w.client.Send(ctx, run, "stop", nil, 0)
		w.settle()
	}
	w.tick(26 * time.Hour)
	if next := w.pass(); !next.Equal(w.clock()) {
		t.Fatalf("a pass that left housekeeping undone said the next thing is due at %s", next)
	}
	if left := len(w.list(store.Finished("ctl"))); left == 0 || left == runs {
		t.Fatalf("one pass left %d of %d runs that are past keeping", left, runs)
	}
	w.settle()
	if left := append(w.list(store.Finished("ctl")), w.list(store.Records("ctl"))...); len(left) != 0 {
		t.Fatalf("after the passes that followed, this is left: %v", left[:min(len(left), 5)])
	}
}

// While an instance runs, each of a job's emit entries goes when it is
// older than the keep and stays until then: whether the job is still going
// or has ended, and whatever the age of the entries beside it.
func TestEmitEntriesAreTrimmedEachByItsOwnAge(t *testing.T) {
	for _, ended := range []bool{false, true} {
		w := newWorld(t)
		r1 := w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{Allow: []string{"all"}}, nil)
		run := w.submit(`
			statechart: emits
			keep: 1d
			initial: working
			states:
			  working:
			    job: { script: ./work.js, timeout: 10d }
			    on: { done: waiting }
			  waiting:
			    on: { stop: stopped }
			  stopped: { end: done }
		`, map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		jobs := w.list(store.Inbox("r1"))
		if len(jobs) != 1 {
			t.Fatalf("%d jobs were sent", len(jobs))
		}
		msg, _ := w.sent(r1, jobs[0])
		job, started := msg.Job.Name, w.clock()
		// The job writes an entry as it starts, one thirty hours on and one
		// eighty-two hours on.
		for number, hours := range map[int]int{1: 0, 2: 30, 3: 82} {
			at := store.Emit("ctl", run, job, number)
			entry := &instance.Emit{Job: job, Number: number, Values: []instance.Emitted{{Name: "log", Data: number}}}
			data, err := r1.Write(at, started.Add(time.Duration(hours)*time.Hour), time.Time{}, entry, w.ctl.Card, w.c1.Card)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.owner.Write(ctx, at, data, store.OnlyIfNew); err != nil {
				t.Fatal(err)
			}
		}
		w.tick(100 * time.Hour)
		if ended {
			w.result(r1, run, job, instance.Result{Status: instance.Done, Output: 1})
		}
		w.settle()
		if head := w.head(run); head.Status != instance.Running || (head.Jobs == 0) != ended {
			t.Fatalf("job ended %v: the run is %s with %d jobs going", ended, head.Status, head.Jobs)
		}
		// At a hundred hours the entries are a hundred, seventy and eighteen
		// hours old, and the keep is a day.
		got := w.list(store.Emits("ctl", run))
		if len(got) != 1 || got[0] != store.Emit("ctl", run, job, 3) {
			t.Fatalf("job ended %v: with a keep of a day, at a hundred hours these emit entries are left: %v", ended, got)
		}
	}
}

// noBlobs is a store that cannot be reached for any blob.
type noBlobs struct{ store.Store }

func (s noBlobs) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if strings.Contains(name, "/blobs/") {
		return nil, store.None, fmt.Errorf("%w: no blob can be read", store.ErrUnreachable)
	}
	return s.Store.Read(ctx, name)
}

func (s noBlobs) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if strings.Contains(name, "/blobs/") {
		return nil, fmt.Errorf("%w: no blob can be read", store.ErrUnreachable)
	}
	return s.Store.ReadPart(ctx, name, from, length)
}

// A pass that cannot read an instance's definition for how long it is kept
// ends there: it does not write a head that says a week for an instance
// whose definition says a day.
func TestHeadIsNotWrittenWithAKeepThatCouldNotBeRead(t *testing.T) {
	w := newWorld(t)
	run := w.submit(kept, nil, nil)
	// The first pass is stopped once the step is written: its head is not.
	good := w.ctrl.Store
	stop := &stopping{Store: good}
	stop.left.Store(1)
	w.afresh(stop)
	if _, err := w.ctrl.Pass(ctx); err == nil || len(w.steps(run)) != 1 || w.head(run) != nil {
		t.Fatalf("the pass that was stopped: %v, with %d steps and the head %+v", err, len(w.steps(run)), w.head(run))
	}
	w.afresh(noBlobs{good})
	if _, err := w.ctrl.Pass(ctx); err == nil || w.head(run) != nil {
		t.Fatalf("with the definition out of reach the pass ended with %v and wrote the head %+v", err, w.head(run))
	}
	w.afresh(good)
	w.settle()
	if head := w.head(run); head == nil || head.Keep.String() != "24h0m0s" {
		t.Fatalf("the head is %+v", head)
	}
}

// A machine's wake that is due is acted on in the first pass at or after
// its time, however much housekeeping is due before it; and a pass with
// many wakes still to come, and none due, says when the soonest is.
func TestDueTimerDoesNotWaitBehindHousekeeping(t *testing.T) {
	w := newWorld(t)
	run := w.submit(`
		statechart: timed
		initial: waiting
		states:
		  waiting:
		    timers: { an_hour: 1h }
		    on: { an_hour: over }
		  over: { end: done }
	`, nil, nil)
	w.settle()
	// More wakes for housekeeping than a pass reads at once, all due half an
	// hour before the timer: for instances that have gone already, so each
	// is quick to act on.
	soonest := w.clock().Add(30 * time.Minute)
	const owed = 1100
	for range owed {
		name := store.Wake("ctl", soonest, store.NewInstance("c1", w.clock()), "remove")
		if _, err := w.owner.Write(ctx, name, []byte("\n"), store.OnlyIfNew); err != nil {
			t.Fatal(err)
		}
	}
	if next := w.pass(); !next.Equal(soonest) {
		t.Fatalf("with nothing due, the pass said the next thing is due at %s; the soonest wake is at %s", next, soonest)
	}
	w.tick(2 * time.Hour)
	next := w.pass()
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("one pass after its timer came due, behind %d wakes for housekeeping, the run is %s", owed, head.Status)
	}
	// The housekeeping is still done a little at a time.
	left := func() (n int) {
		for _, name := range w.list(store.Wakes("ctl")) {
			if strings.HasSuffix(name, "/remove") && !strings.Contains(name, run) {
				n++
			}
		}
		return n
	}
	if n := left(); n == 0 || n == owed || !next.Equal(w.clock()) {
		t.Fatalf("the pass left %d of %d wakes for housekeeping, and said the next thing is due at %s", n, owed, next)
	}
	w.settle()
	if n := left(); n != 0 {
		t.Fatalf("after the passes that followed, %d wakes for housekeeping are left", n)
	}
}
