package controller_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// start makes the start of a new run of a client's, with its definition in
// the run's record, and gives the run and the message.
func (w *world) start(client string) (string, *instance.Message) {
	w.t.Helper()
	run := store.NewInstance(client, w.clock())
	ref, err := w.draft(kept, nil).Store(ctx, w.owner, "ctl", run)
	if err != nil {
		w.t.Fatal(err)
	}
	return run, &instance.Message{Kind: instance.KindStart, Instance: run, Start: &instance.Start{Definition: ref, Seed: []byte("seed")}}
}

// put writes a message into the controller's inbox, signed by one party
// under a sender's name, sealed to a card, good until a time.
func (w *world) put(by *identity.Identity, message string, msg *instance.Message, to identity.Card, until time.Time) string {
	w.t.Helper()
	at := store.Message("ctl", by.Name, message)
	data, err := by.Write(at, w.clock(), until, msg, to)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, at, data, store.Overwrite); err != nil {
		w.t.Fatal(err)
	}
	return at
}

func Test_id_message_checked_before_contents_read(t *testing.T) {
	w := newWorld(t)
	good := w.clock().Add(time.Hour)
	// A start as c1 would send it is taken: what follows differs from it in
	// one thing each.
	run, msg := w.start("c1")
	w.put(w.c1, "a-start", msg, w.ctl.Card, good)
	w.settle()
	if w.head(run) == nil {
		t.Fatal("a start signed by a client the controller trusts, sealed to the controller and still good was not taken")
	}

	type refused struct {
		what    string
		run     string
		at      string
		receipt string // the name its receipt would have
	}
	var all []refused
	add := func(what, run, at string) {
		all = append(all, refused{what, run, at, store.Receipt("ctl", "c1", at[strings.LastIndexByte(at, '/')+1:])})
	}
	// Signed by a party that was never enrolled.
	stranger := w.party("stranger", identity.Client)
	run, msg = w.start("stranger")
	add("from a party never enrolled", run, w.put(stranger, "b-start", msg, w.ctl.Card, good))
	// Signed with another key under the name of a client that is trusted.
	forger := w.party("c1", identity.Client)
	run, msg = w.start("c1")
	add("signed with a key that is not c1's, under c1's name", run, w.put(forger, "c-start", msg, w.ctl.Card, good))
	// The same, and past its time as well: it is not c1's to be answered.
	run, msg = w.start("c1")
	add("not c1's, and past its time", run, w.put(forger, "d-start", msg, w.ctl.Card, w.clock().Add(-time.Hour)))
	// Signed by c1, and one byte of it changed afterwards.
	run, msg = w.start("c1")
	at := w.put(w.c1, "e-start", msg, w.ctl.Card, good)
	raw, _, _ := w.owner.Read(ctx, at)
	lines := bytes.Split(raw, []byte("\n"))
	lines[1][len(lines[1])/2] ^= 1
	w.owner.Write(ctx, at, bytes.Join(lines, []byte("\n")), store.Overwrite)
	add("altered after it was signed", run, at)
	// Signed by c1 for one name in the inbox, and found under another.
	run, msg = w.start("c1")
	at = w.put(w.c1, "f-start", msg, w.ctl.Card, good)
	raw, _, _ = w.owner.Read(ctx, at)
	w.owner.Delete(ctx, at, store.Overwrite)
	moved := store.Message("ctl", "c1", "g-start")
	w.owner.Write(ctx, moved, raw, store.Overwrite)
	add("moved to another name", run, moved)
	// Signed by c1, and sealed to another controller.
	elsewhere := w.party("ctl2", identity.Controller)
	run, msg = w.start("c1")
	add("sealed to another controller", run, w.put(w.c1, "h-start", msg, elsewhere.Card, good))

	w.settle()
	for _, r := range all {
		if w.head(r.run) != nil || len(w.steps(r.run)) != 0 {
			t.Errorf("a start %s was acted on", r.what)
		}
		if _, mark, _ := w.owner.Read(ctx, r.at); mark != store.None {
			t.Errorf("a start %s was left in the inbox", r.what)
		}
		if _, mark, _ := w.owner.Read(ctx, r.receipt); mark != store.None {
			t.Errorf("a start %s was given a receipt", r.what)
		}
	}
	// One that is c1's own, sealed to the controller and past its time, is
	// not acted on either; that it came too late is the last thing asked,
	// and c1 is told.
	run, msg = w.start("c1")
	late := w.put(w.c1, "i-start", msg, w.ctl.Card, w.clock().Add(-time.Hour))
	w.settle()
	if r := w.receipt("i-start"); w.head(run) != nil || r == nil || r.Taken || !strings.Contains(r.Refused, "too late") {
		t.Fatalf("c1's own start, past its time, got %+v", r)
	}
	if _, mark, _ := w.owner.Read(ctx, late); mark != store.None {
		t.Fatal("a start past its time was left in the inbox")
	}

	// A message c1 sent, written into the inbox a second time under another
	// name, is not a second message.
	run = w.submit(kept, nil, nil)
	w.settle()
	event, _ := w.client.Send(ctx, run, "tick", nil, 0)
	raw, _, _ = w.owner.Read(ctx, store.Message("ctl", "c1", event))
	w.owner.Write(ctx, store.Message("ctl", "c1", event+"-again"), raw, store.Overwrite)
	w.settle()
	if head := w.head(run); head.Step != 2 || len(w.list(store.Inbox("ctl"))) != 0 {
		t.Fatalf("an event written a second time under another name moved the run to %+v", head)
	}
}

func Test_id_removal_holds_in_every_copy_at_next_pass(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{}, nil)
	// Two copies of the controller, each of which has accepted messages
	// from the client and from the runner.
	copies := []*controller.Controller{w.ctrl, controller.New(w.ctl, w.ctrl.Store)}
	copies[1].Now = w.clock
	var waiting []string
	for _, c := range copies {
		run := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
		if _, err := c.Pass(ctx); err != nil {
			t.Fatal(err)
		}
		done := w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
		if _, err := c.Pass(ctx); err != nil {
			t.Fatal(err)
		}
		w.result(r1, done, "working", instance.Result{Attempt: 1, Status: instance.Done, Output: "taken"})
		if _, err := c.Pass(ctx); err != nil {
			t.Fatal(err)
		}
		if head := w.head(done); head == nil || head.Status != instance.Done {
			t.Fatalf("before any removal, a copy left the run as %+v", head)
		}
		waiting = append(waiting, run)
	}
	// The client and the runner are removed. They stay on record, marked.
	for _, id := range []*identity.Identity{w.c1, r1} {
		if err := identity.WriteTrust(ctx, w.ctrl.Store, w.ctl, identity.Trust{Card: id.Card, Removed: true}); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range copies {
		before := len(w.list(store.Running("ctl"))) + len(w.list(store.Finished("ctl")))
		started := w.submit(kept, nil, nil)
		w.client.Send(ctx, waiting[i], "tick", nil, 0)
		w.result(r1, waiting[i], "working", instance.Result{Attempt: 1, Status: instance.Done, Output: "from a runner that was removed"})
		if _, err := c.Pass(ctx); err != nil {
			t.Fatal(err)
		}
		if w.head(started) != nil || len(w.list(store.Running("ctl")))+len(w.list(store.Finished("ctl"))) != before {
			t.Fatalf("copy %d took a start from a client removed before its pass", i)
		}
		if head := w.head(waiting[i]); head.Status != instance.Running || head.Step != 1 || head.Jobs != 1 {
			t.Fatalf("copy %d accepted a result from a runner removed before its pass: %+v", i, head)
		}
		if left := w.list(store.Inbox("ctl")); len(left) != 0 {
			t.Fatalf("copy %d left what it did not accept in the inbox: %v", i, left)
		}
	}
	// Nothing is sent to a runner that has been removed: a run started by
	// a client that is still trusted finds no runner, with either copy.
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{}, nil)
	for i, c := range copies {
		run, err := w.clientOf(c2).Start(ctx, w.draft(oneJob, map[string]string{"work.js": "return 1"}), client.Start{})
		if err != nil {
			t.Fatal(err)
		}
		for range 3 {
			if _, err := c.Pass(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if head := w.head(run); head == nil || head.Status != instance.Done || w.latest(run).State.Output != "error" {
			t.Fatalf("copy %d, with its one runner removed, left the run as %+v", i, head)
		}
	}
	if at := w.jobsAt(); len(at) != 1 || at["r1"] != 4 {
		t.Fatalf("after the runner was removed the jobs sent are %v", at)
	}
}

// unread is a store whose inbox can be listed and not read.
type unread struct {
	store.Store
	fail atomic.Bool
	// after is called when the inbox has been listed.
	after func()
}

func (u *unread) List(ctx context.Context, under string) ([]string, error) {
	names, err := u.Store.List(ctx, under)
	if under == store.Inbox("ctl") && u.after != nil {
		u.after()
	}
	return names, err
}

func (u *unread) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if u.fail.Load() && strings.HasPrefix(name, store.Inbox("ctl")+"/") {
		return nil, store.None, fmt.Errorf("%w: turned away", store.ErrUnreachable)
	}
	return u.Store.Read(ctx, name)
}

func Test_ctl_due_wake_not_acted_on_before_inbox_is_read(t *testing.T) {
	// begin leaves a job's result waiting, a second before the job's due
	// time, for a controller that next looks an hour later.
	var run string
	begin := func() *world {
		w := newWorld(t)
		r1 := w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{}, nil)
		run = w.submit(oneJob, map[string]string{"work.js": "return 1"}, nil)
		w.settle()
		w.tick(89 * time.Second)
		w.result(r1, run, "working", instance.Result{Attempt: 1, Status: instance.Done, Output: "in time"})
		w.tick(time.Hour)
		if due := w.wakes(); len(due) != 1 || !strings.Contains(due[0], "/due.working") {
			t.Fatalf("the wakes are %v", due)
		}
		return w
	}
	ended := func(w *world, when string) {
		t.Helper()
		w.afresh(w.ctrl.Store.(*unread).Store)
		w.settle()
		last := w.latest(run)
		if last.State.Status != instance.Done || last.State.Output != "in time" || len(w.steps(run)) != 2 {
			t.Fatalf("%s: the run ended %s with %v in %d steps", when, last.State.Status, last.State.Output, len(w.steps(run)))
		}
	}
	// The pass lists the inbox and its wakes, and then cannot read the
	// result: the job's due time, which has passed, is not acted on.
	w := begin()
	u := &unread{Store: w.ctrl.Store}
	u.fail.Store(true)
	w.afresh(u)
	if _, err := w.ctrl.Pass(ctx); err == nil {
		t.Fatal("a pass that could not read its inbox ended as if nothing was wrong")
	}
	if n := len(w.steps(run)); n != 1 || len(w.wakes()) != 1 {
		t.Fatalf("a pass that had not read the result acted on the job's due time: %d steps, wakes %v", n, w.wakes())
	}
	ended(w, "after a pass that could not read its inbox")

	// The pass runs out of time once the inbox has been listed.
	w = begin()
	limited, end := context.WithCancel(ctx)
	u = &unread{Store: w.ctrl.Store, after: end}
	w.afresh(u)
	if _, err := w.ctrl.Pass(limited); err == nil {
		t.Fatal("a pass whose time ran out ended as if it had done everything")
	}
	if n := len(w.steps(run)); n != 1 || len(w.wakes()) != 1 {
		t.Fatalf("a pass that ran out of time before reading the result acted on the job's due time: %d steps, wakes %v", n, w.wakes())
	}
	ended(w, "after a pass that ran out of time")
}
