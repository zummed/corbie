package controller_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A submit is carried on only where it is the one the tree's entry was
// written for, and only where the checks a submit is given pass at that
// moment. A message under the name of an earlier submit, with another
// content, records nothing.
func Test_ctl_submit_carried_on_only_as_entered_and_allowed(t *testing.T) {
	plain := "flow: plain\nstages:\n  s: { job: { script: ./one.js } }\n"
	hashAt := func(w *world, location string) string {
		t.Helper()
		file := store.LocationFile("ctl", location)
		raw, mark, err := w.owner.Read(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		if mark == store.None {
			return ""
		}
		var got map[string]any
		if _, err := identity.Read(raw, file, w.ctl.Card, w.ctl, &got); err != nil {
			t.Fatal(err)
		}
		h, _ := got["hash"].(string)
		return h
	}
	// reuse writes a new message under a name, with a changed definition
	// uploaded under another message's uploads, and runs the controller
	// after the message's life is over.
	reuse := func(w *world, c *identity.Identity, name string) {
		t.Helper()
		cl := w.clientOf(c)
		changed := w.tree("/t", "", map[string]string{"/t/m": plain + "title: planted\n"}, map[string]string{"one.js": "return 2"}).Machines[0].Draft
		def, err := changed.Write(ctx, func(typ string, contents []byte) (blob.Ref, error) {
			return blob.Write(ctx, cl.Store, store.Upload("ctl", c.Name, "elsewhere", blob.NewName()), typ, blob.NewKey(), bytes.NewReader(contents))
		})
		if err != nil {
			t.Fatal(err)
		}
		msg := &instance.Message{Kind: instance.KindSubmit, Submit: &instance.Submit{Prefix: "/t",
			Files: map[string]instance.SubmitFile{"/t/m": {Definition: def, Hash: "planted"}}}}
		at := store.Message("ctl", c.Name, name)
		data, err := c.Write(at, w.clock(), w.clock().Add(time.Minute), msg, w.ctl.Card)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cl.Store.Write(ctx, at, data, store.OnlyIfNew); err != nil {
			t.Fatal(err)
		}
		w.tick(10 * time.Minute)
		w.settle()
	}

	for _, ceilingKept := range []bool{false, true} {
		w := newWorld(t)
		w.submit(waiting, nil, nil) // a run of c1's, so housekeeping has its wake
		c2 := w.party("c2", identity.Client)
		w.enrol(c2, permission.Layer{Allow: []string{"network", "at:/t"}}, nil)
		cl := w.clientOf(c2)
		tree := w.tree("/t", "", map[string]string{"/t/m": plain}, map[string]string{"one.js": "return 1"})
		name, err := cl.Submit(ctx, tree, 0)
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		first := hashAt(w, "/t/m")
		// A day later the receipt is gone, and the tree's entry still names
		// the submit.
		for range 30 {
			w.tick(2 * time.Hour)
			w.settle()
		}
		if _, mark, _ := w.owner.Read(ctx, store.Receipt("ctl", "c2", name)); mark != store.None {
			t.Fatal("the receipt is still there")
		}
		if !ceilingKept {
			w.enrol(c2, permission.Layer{Allow: []string{"network"}}, nil)
			if r := w.submitTree(cl, tree); r.Taken {
				t.Fatal("an ordinary submit was taken")
			}
		}
		reuse(w, c2, name)
		if got := hashAt(w, "/t/m"); got != first {
			t.Errorf("with the ceiling kept %v, a message under a used name changed the file at /t/m to %q", ceilingKept, got)
		}
		if r, _ := cl.Receipt(ctx, name); r == nil || r.Taken {
			t.Errorf("with the ceiling kept %v, the reused name's receipt is %+v", ceilingKept, r)
		}
		if left := w.list(store.Inbox("ctl")); len(left) > 0 {
			t.Errorf("the inbox holds %v", left)
		}
	}

	// A controller stopped after the tree's entry was written takes the
	// same message again after its life is over and finishes it, while the
	// client may submit there; with the at: entry taken away meanwhile, it
	// records nothing more.
	for _, ceilingKept := range []bool{true, false} {
		w := newWorld(t)
		c2 := w.party("c2", identity.Client)
		w.enrol(c2, permission.Layer{Allow: []string{"network", "at:/t"}}, nil)
		cl := w.clientOf(c2)
		message, err := cl.Submit(ctx, w.tree("/t", "", map[string]string{"/t/m": plain}, map[string]string{"one.js": "return 1"}), 0)
		if err != nil {
			t.Fatal(err)
		}
		inner := w.ctrl.Store
		s := &stopping{Store: inner}
		s.left.Store(1)
		w.ctrl = controller.New(w.ctl, s)
		w.ctrl.Now = w.clock
		w.ctrl.Pass(ctx)
		if !s.stopped.Load() || hashAt(w, "/t/m") != "" {
			t.Fatal("the controller did not stop after the tree's entry")
		}
		w.afresh(inner)
		if !ceilingKept {
			w.enrol(c2, permission.Layer{Allow: []string{"network"}}, nil)
		}
		w.tick(instance.MessageLife + identity.ClockAllowance + time.Minute)
		w.settle()
		r, _ := cl.Receipt(ctx, message)
		recorded := hashAt(w, "/t/m") != ""
		switch {
		case ceilingKept && (r == nil || !r.Taken || !recorded):
			t.Errorf("taken again late, the submit gave %+v and recorded %v", r, recorded)
		case !ceilingKept && (r == nil || r.Taken || !strings.Contains(r.Refused, "at:/t") || recorded):
			t.Errorf("taken again with at:/t taken away, the submit gave %+v and recorded %v", r, recorded)
		}
	}
}
