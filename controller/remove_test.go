package controller_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// removeTree sends a removal of a tree from a client and runs the
// controller until it comes to rest. It gives the receipt.
func (w *world) removeTree(c *client.Client, prefix string) *instance.Receipt {
	w.t.Helper()
	message, err := c.RemoveTree(ctx, prefix, 0)
	if err != nil {
		w.t.Fatal(err)
	}
	w.settle()
	r, err := c.Receipt(ctx, message)
	if err != nil || r == nil {
		w.t.Fatalf("the removal has no receipt: %v", err)
	}
	return r
}

// has says whether a value is in the store under a name.
func (w *world) has(name string) bool {
	w.t.Helper()
	_, mark, err := w.owner.Read(ctx, name)
	if err != nil {
		w.t.Fatal(err)
	}
	return mark != store.None
}

// A removal of a tree cancels its runs, and removes its locations, the
// secrets and values set at its prefix or under it, and the tree. What is
// above the prefix and another tree stay, and another client then submits
// at the prefix.
func Test_ctl_remove_removes_the_tree_and_what_is_set_under_it(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"all"}}, nil)
	other := w.clientOf(c2)
	run := w.submitTree(w.client, w.tree("/x", "/srv/x", map[string]string{"/x/kept": waiting, "/x/deep/one": waiting}, nil)).Tree.Locations[1].Run
	w.submitTree(w.client, w.tree("/y", "", map[string]string{"/y": waiting}, nil))
	w.secret("K", "everywhere", "/")
	w.secret("K", "in x", "/x")
	w.secret("K", "in y", "/y")
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/x/deep/one", "HOST", "h"); err != nil {
		t.Fatal(err)
	}
	if r := w.removeTree(other, "/x"); r.Taken || !strings.Contains(r.Refused, "the tree /x belongs to the client c1") {
		t.Fatalf("another client's removal gave %+v", r)
	}
	if r := w.removeTree(w.client, "/x/deep"); r.Taken || !strings.Contains(r.Refused, "/x/deep is inside the tree /x") {
		t.Fatalf("a removal inside a tree gave %+v", r)
	}
	r := w.removeTree(w.client, "/x")
	if !r.Taken {
		t.Fatalf("the removal was refused: %s", r.Refused)
	}
	done := r.Tree
	if len(done.Locations) != 2 || done.Locations[0].What != "removed" || done.Locations[1].What != "removed" || len(done.Cancelled) != 2 ||
		!slices.Equal(done.Removed, []string{"secret K at /x", "value HOST at /x/deep/one"}) {
		t.Fatalf("the removal did %+v", done)
	}
	if status := w.head(run).Status; status != "cancelled" {
		t.Fatalf("the run is %s", status)
	}
	for _, name := range w.list(store.Locations("ctl")) {
		if at, entry, _ := store.ParseLocation("ctl", name); (at == "/x" || strings.HasPrefix(at, "/x/")) && !strings.HasPrefix(entry, "keys/") {
			t.Fatalf("%s is still in the store", name)
		}
	}
	for at, want := range map[string]string{"/x/kept": "everywhere", "/y": "in y"} {
		if value, _, err := secret.Get(ctx, w.ctrl.Store, w.ctl, at, "K"); err != nil || value != want {
			t.Fatalf("K at %s is %q: %v", at, value, err)
		}
	}
	if len(w.at("/y")) != 1 || !w.has(store.Tree("ctl", "/y")) {
		t.Fatalf("the tree /y is %v", w.at("/y"))
	}
	// The prefix holds no tree: a removal finds nothing, and another client
	// submits there.
	if r := w.removeTree(w.client, "/x"); !r.Taken || len(r.Tree.Locations)+len(r.Tree.Removed) != 0 {
		t.Fatalf("a second removal gave %+v", r)
	}
	if w.has(store.Tree("ctl", "/x")) {
		t.Fatalf("a removal of no tree left its entry")
	}
	if r := w.submitTree(other, w.tree("/x", "", map[string]string{"/x/kept": waiting}, nil)); !r.Taken || r.Tree.Locations[0].What != "started" {
		t.Fatalf("another client's submit at the prefix gave %+v", r)
	}
}

// A removal that names a file is refused, and removes nothing.
func Test_ctl_remove_names_no_file(t *testing.T) {
	w := newWorld(t)
	w.submitTree(w.client, w.tree("/x", "", map[string]string{"/x": waiting}, nil))
	name := "20261010T120000.000000Z-000009"
	msg := &instance.Message{Kind: instance.KindSubmit, Submit: &instance.Submit{Prefix: "/x", Remove: true, Problems: []string{"/x"}}}
	at := store.Message("ctl", "c1", name)
	data, err := w.c1.Write(at, w.clock(), w.clock().Add(time.Minute), msg, w.ctl.Card)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.client.Store.Write(ctx, at, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(name); r == nil || r.Taken || !strings.Contains(r.Refused, "names no file") {
		t.Fatalf("the removal gave %+v", r)
	}
	if !w.has(store.Tree("ctl", "/x")) || len(w.at("/x")) != 1 {
		t.Fatalf("the tree is %v", w.list(store.Locations("ctl")))
	}
}

// removeStory submits a tree, sets a secret and a value in it, then removes
// it, and stops the controller after a number of writes of the removal; a
// controller that has never run before carries on, late by as much as is
// given. It gives what is left, the receipt, and whether the stop happened.
func removeStory(t *testing.T, stopAfter int64, late time.Duration) (string, *instance.Receipt, bool) {
	w := newWorld(t)
	gone := strings.Replace(waiting, "statechart: waiting", "statechart: gone", 1)
	w.submitTree(w.client, w.tree("/x", "/srv/a", map[string]string{"/x/kept": waiting, "/x/gone": gone}, nil))
	w.secret("K", "in x", "/x")
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/x/kept", "HOST", "h"); err != nil {
		t.Fatal(err)
	}
	message, err := w.client.RemoveTree(ctx, "/x", 0)
	if err != nil {
		t.Fatal(err)
	}
	inner := w.ctrl.Store
	s := &stopping{Store: inner}
	s.left.Store(stopAfter)
	w.ctrl = controller.New(w.ctl, s)
	w.ctrl.Now = w.clock
	for range 6 {
		if _, err := w.ctrl.Pass(ctx); err != nil {
			break
		}
		w.tick(400 * time.Millisecond)
	}
	w.tick(late)
	w.afresh(inner)
	for range 6 {
		w.settle()
		w.tick(time.Second)
	}
	return w.trees(), w.receipt(message), s.stopped.Load()
}

// A controller stopped after any single write of a removal leaves what a
// controller never stopped leaves: no entry for the tree, no location, no
// secret and no value under the prefix, each run cancelled, and the same
// receipt. So does one that comes back after the message would have been
// too late.
func Test_ctl_removal_stopped_after_every_write(t *testing.T) {
	want, wantReceipt, _ := removeStory(t, 1<<40, 0)
	if !strings.HasPrefix(want, "/x/kept runs: [cancelled]\n/x/gone runs: [cancelled]\n") || !strings.Contains(want, "waiting: [] uploads: []") {
		t.Fatalf("the removal never stopped left:\n%s", want)
	}
	tooLate := instance.MessageLife + identity.ClockAllowance + time.Minute
	for _, late := range []time.Duration{0, tooLate} {
		for n := int64(0); ; n++ {
			got, receipt, stopped := removeStory(t, n, late)
			if !stopped {
				if n < 8 {
					t.Fatalf("the removal made only %d writes", n)
				}
				break
			}
			if late > 0 && n == 0 {
				if receipt == nil || receipt.Taken || !strings.Contains(receipt.Refused, "the message came too late") {
					t.Fatalf("taken late before any write, the receipt is %+v", receipt)
				}
				continue
			}
			if got != want {
				t.Fatalf("stopped after %d writes and late by %s, the removal left:\n%s\nnever stopped:\n%s", n, late, got, want)
			}
			if receipt == nil || !receipt.Taken || len(receipt.Tree.Locations) != 2 || len(receipt.Tree.Cancelled) != 2 ||
				!slices.Equal(receipt.Tree.Removed, wantReceipt.Tree.Removed) || len(receipt.Tree.Removed) != 2 {
				t.Fatalf("stopped after %d writes and late by %s, the receipt is %+v", n, late, receipt.Tree)
			}
		}
	}
}

// ownerRemoves writes the owner's removal of a tree into the controller's
// inbox, as corbie controller tree remove does. It gives the message's name.
func (w *world) ownerRemoves(prefix string) string {
	w.t.Helper()
	w.placed++
	name := store.Message("ctl", "ctl", fmt.Sprintf("owner-20261010T120000.000000Z-%06d", w.placed))
	msg := &instance.Message{Kind: instance.KindSubmit, Submit: &instance.Submit{Prefix: prefix, Remove: true}}
	data, err := w.ctl.Write(name, w.clock(), w.clock().Add(24*time.Hour), msg, w.ctl.Card)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.ctrl.Store.Write(ctx, name, data, store.OnlyIfNew); err != nil {
		w.t.Fatal(err)
	}
	return name
}

// The owner removes any client's tree with the controller's own key, also
// one whose client is trusted no longer: its runs are cancelled, and its
// locations, what is set under its prefix and the tree are removed. A
// prefix inside a tree is refused, and a submit the controller's key sends
// that is no removal is no message.
func Test_ctl_owner_removes_any_tree(t *testing.T) {
	w := newWorld(t)
	run := w.submitTree(w.client, w.tree("/x", "", map[string]string{"/x/kept": waiting}, nil)).Tree.Locations[0].Run
	w.submitTree(w.client, w.tree("/y", "", map[string]string{"/y": waiting}, nil))
	w.secret("K", "in x", "/x/kept")
	if err := identity.WriteTrust(ctx, w.ctrl.Store, w.ctl, identity.Trust{Card: w.c1.Card, Removed: true}); err != nil {
		t.Fatal(err)
	}
	w.ownerRemoves("/x/kept")
	w.settle()
	if !w.has(store.Tree("ctl", "/x")) || !w.has(store.LocationFile("ctl", "/x/kept")) || w.head(run).Status != "running" {
		t.Fatalf("a removal inside the tree left %v", w.list(store.Locations("ctl")))
	}
	w.ownerRemoves("/x")
	w.settle()
	if status := w.head(run).Status; status != "cancelled" {
		t.Fatalf("the run is %s", status)
	}
	for _, name := range w.list(store.Locations("ctl")) {
		if at, entry, _ := store.ParseLocation("ctl", name); (at == "/x" || strings.HasPrefix(at, "/x/")) && !strings.HasPrefix(entry, "keys/") {
			t.Fatalf("%s is still in the store", name)
		}
	}
	if !w.has(store.Tree("ctl", "/y")) || len(w.at("/y")) != 1 {
		t.Fatalf("the tree /y is %v", w.at("/y"))
	}
	if left := append(w.list(store.Inbox("ctl")), w.list(store.Receipts("ctl", "ctl"))...); len(left) > 0 {
		t.Fatalf("the removals left %v", left)
	}
	// A submit under the controller's key that removes nothing.
	name := store.Message("ctl", "ctl", "owner-20261010T120000.000000Z-000099")
	data, err := w.ctl.Write(name, w.clock(), w.clock().Add(time.Hour), &instance.Message{Kind: instance.KindSubmit, Submit: &instance.Submit{Prefix: "/y"}}, w.ctl.Card)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.ctrl.Store.Write(ctx, name, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if !w.has(store.Tree("ctl", "/y")) || len(w.at("/y")) != 1 || w.head(w.onlyAt("/y")).Status != "running" {
		t.Fatalf("a submit of the controller's own changed the tree /y: %v", w.at("/y"))
	}
}

// onlyAt gives the one root run at a location.
func (w *world) onlyAt(location string) string {
	w.t.Helper()
	for run := range w.at(location) {
		return run
	}
	w.t.Fatalf("no run is at %s", location)
	return ""
}

// The owner's removal, stopped after any single write, leaves what one never
// stopped leaves.
func Test_ctl_owner_removal_stopped_after_every_write(t *testing.T) {
	story := func(stopAfter int64) (string, bool) {
		w := newWorld(t)
		gone := strings.Replace(waiting, "statechart: waiting", "statechart: gone", 1)
		w.submitTree(w.client, w.tree("/x", "/srv/a", map[string]string{"/x/kept": waiting, "/x/gone": gone}, nil))
		w.secret("K", "in x", "/x")
		w.ownerRemoves("/x")
		inner := w.ctrl.Store
		s := &stopping{Store: inner}
		s.left.Store(stopAfter)
		w.ctrl = controller.New(w.ctl, s)
		w.ctrl.Now = w.clock
		for range 6 {
			if _, err := w.ctrl.Pass(ctx); err != nil {
				break
			}
			w.tick(400 * time.Millisecond)
		}
		w.afresh(inner)
		for range 6 {
			w.settle()
			w.tick(time.Second)
		}
		return w.trees(), s.stopped.Load()
	}
	want, _ := story(1 << 40)
	if !strings.HasPrefix(want, "/x/kept runs: [cancelled]\n/x/gone runs: [cancelled]\n") || !strings.Contains(want, "waiting: [] uploads: []") {
		t.Fatalf("the removal never stopped left:\n%s", want)
	}
	for n := int64(0); ; n++ {
		got, stopped := story(n)
		if !stopped {
			if n < 6 {
				t.Fatalf("the removal made only %d writes", n)
			}
			break
		}
		if got != want {
			t.Fatalf("stopped after %d writes, the removal left:\n%s\nnever stopped:\n%s", n, got, want)
		}
	}
}
