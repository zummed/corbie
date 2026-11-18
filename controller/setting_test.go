package controller_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// answer sends a set, remove or list, runs the controller and gives the
// receipt.
func (w *world) answer(c *client.Client, send func() (string, error)) *instance.Receipt {
	w.t.Helper()
	message, err := send()
	if err != nil {
		w.t.Fatal(err)
	}
	w.settle()
	r, err := c.Receipt(ctx, message)
	if err != nil || r == nil {
		w.t.Fatalf("no receipt: %v", err)
	}
	return r
}

// A client sets, removes and lists secrets and values where it could submit
// a tree, and the controller keeps them as it keeps the owner's.
func Test_sec_a_client_sets_at_its_own_prefix(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"network", "at:/work", "at:/spare/**", "at:/home", "at:/home/**"}}, nil)
	work := w.clientOf(c2)
	if r := w.submitTree(work, w.tree("/work", "", map[string]string{"/work/m": waiting}, nil)); !r.Taken {
		t.Fatal(r.Refused)
	}
	if r := w.submitTree(w.client, w.tree("/home", "", map[string]string{"/home/m": waiting}, nil)); !r.Taken {
		t.Fatal(r.Refused)
	}
	w.secret("K", "the owner's", "/")
	w.secret("K", "the owner's at /work/m", "/work/m")

	for _, at := range []string{"/work/m", "/work", "/spare/x"} {
		if r := w.answer(work, func() (string, error) { return work.SetSecret(ctx, at, "K", "work's at "+at, 0) }); !r.Taken {
			t.Fatalf("a set at %s was refused: %s", at, r.Refused)
		}
	}
	// The later set replaces the owner's at /work/m, and the nearest wins.
	for location, want := range map[string]string{"/work/m": "work's at /work/m", "/work/other": "work's at /work", "/home/m": "the owner's"} {
		if got, _, err := secret.Get(ctx, w.ctrl.Store, w.ctl, location, "K"); err != nil || got != want {
			t.Errorf("K for %s is %q (%v), want %q", location, got, err, want)
		}
	}
	if r := w.answer(work, func() (string, error) { return work.SetValue(ctx, "/work", "HOST", "work.lan", 0) }); !r.Taken {
		t.Fatal(r.Refused)
	}
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/home", "HOST", "home.lan"); err != nil {
		t.Fatal(err)
	}

	for refused, try := range map[string]func() (string, error){
		"/ is never a tree":                       func() (string, error) { return work.SetSecret(ctx, "/", "K", "v", 0) },
		"the tree /home belongs to the client c1": func() (string, error) { return work.SetSecret(ctx, "/home/m", "K", "v", 0) },
		"corbie controller enrol c2 --allow at:/elsewhere": func() (string, error) {
			return work.SetSecret(ctx, "/elsewhere", "K", "v", 0)
		},
		"the secret NONE is not set at /work": func() (string, error) { return work.RemoveSecret(ctx, "/work", "NONE", 0) },
		// Where it may not set, a remove does not say whether a name is set.
		"the tree /home belongs to the client c1 ": func() (string, error) { return work.RemoveValue(ctx, "/home", "HOST", 0) },
	} {
		r := w.answer(work, try)
		if r.Taken || !strings.Contains(r.Refused+" ", refused) || strings.Contains(r.Refused, "is set") {
			t.Errorf("want %q, got %+v", refused, r)
		}
	}

	r := w.answer(work, func() (string, error) { return work.ListSecrets(ctx, 0) })
	want := []instance.Listed{{Name: "K", At: "/spare/x"}, {Name: "K", At: "/work"}, {Name: "K", At: "/work/m"}}
	if !r.Taken || !slices.Equal(r.Listed, want) {
		t.Errorf("the secrets are listed as %+v, want %+v", r.Listed, want)
	}
	r = w.answer(work, func() (string, error) { return work.ListValues(ctx, 0) })
	if want := []instance.Listed{{Name: "HOST", At: "/work", Value: "work.lan"}}; !r.Taken || !slices.Equal(r.Listed, want) {
		t.Errorf("the values are listed as %+v, want %+v", r.Listed, want)
	}
	// The owner lists everything, the client's included.
	all, err := secret.List(ctx, w.ctrl.Store, "ctl")
	if err != nil || len(all) != 4 {
		t.Errorf("the owner lists %v (%v)", all, err)
	}

	if r := w.answer(work, func() (string, error) { return work.RemoveSecret(ctx, "/work/m", "K", 0) }); !r.Taken {
		t.Fatal(r.Refused)
	}
	if got, _, _ := secret.Get(ctx, w.ctrl.Store, w.ctl, "/work/m", "K"); got != "work's at /work" {
		t.Errorf("after the remove, K for /work/m is %q", got)
	}
}

// No receipt holds a secret's value.
func Test_sec_a_clients_secret_in_no_receipt(t *testing.T) {
	w := newWorld(t)
	message, err := w.client.SetSecret(ctx, "/x", "K", "the very value", 0)
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(message); r == nil || !r.Taken {
		t.Fatalf("the set gave %+v", r)
	}
	for _, name := range w.list(store.Receipts("ctl", "c1")) {
		data, _, _ := w.owner.Read(ctx, name)
		var receipt instance.Receipt
		if _, err := identity.Read(data, name, w.ctl.Card, w.ctl, &receipt); err != nil || strings.Contains(string(data), "the very value") || receipt.Listed != nil {
			t.Errorf("the receipt %s: %+v, %v", name, receipt, err)
		}
	}
}

// A set, remove or list taken again, after a stop at any write, gives the
// answer it gave first and leaves what it left.
func Test_ctl_setting_taken_again_gives_the_same_answer(t *testing.T) {
	w := newWorld(t)
	resend := func(send func() (string, error)) (first, again *instance.Receipt) {
		t.Helper()
		message, err := send()
		if err != nil {
			t.Fatal(err)
		}
		name := store.Message("ctl", "c1", message)
		data, _, err := w.owner.Read(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		first = w.receipt(message)
		// The message is in the inbox again, as after a stop before it was
		// deleted.
		if _, err := w.owner.Write(ctx, name, data, store.Overwrite); err != nil {
			t.Fatal(err)
		}
		w.settle()
		if left := w.list(store.Inbox("ctl")); len(left) > 0 {
			t.Fatalf("the inbox holds %v", left)
		}
		return first, w.receipt(message)
	}
	first, again := resend(func() (string, error) { return w.client.SetSecret(ctx, "/x", "K", "v", 0) })
	if !first.Taken || !again.Taken {
		t.Errorf("the set gave %+v, then %+v", first, again)
	}
	first, again = resend(func() (string, error) { return w.client.RemoveSecret(ctx, "/x", "K", 0) })
	if !first.Taken || !again.Taken {
		t.Errorf("the remove gave %+v, then %+v", first, again)
	}
	if _, set, _ := secret.Get(ctx, w.ctrl.Store, w.ctl, "/x", "K"); set {
		t.Error("K is set after the remove was taken again")
	}
	first, again = resend(func() (string, error) { return w.client.RemoveSecret(ctx, "/x", "K", 0) })
	if first.Taken || again.Refused != first.Refused {
		t.Errorf("the second remove gave %+v, then %+v", first, again)
	}
	// Stopped after the receipt and before the value was kept, the set is
	// carried out when it is taken again.
	message, err := w.client.SetValue(ctx, "/x", "V", "v", 0)
	if err != nil {
		t.Fatal(err)
	}
	name := store.Message("ctl", "c1", message)
	data, _, err := w.owner.Read(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	if err := secret.RemoveValue(ctx, w.ctrl.Store, w.ctl, "/x", "V"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, name, data, store.Overwrite); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if values, err := secret.Values(ctx, w.ctrl.Store, w.ctl, "/x"); err != nil || values["V"] != "v" {
		t.Errorf("the set taken again left %v (%v)", values, err)
	}
}

// Where a client may not set, its set, remove or list says nothing of what
// is set there.
func Test_ctl_setting_refused_learns_nothing(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"at:/shop"}}, nil)
	shop := w.clientOf(c2)
	if r := w.submitTree(w.client, w.tree("/work", "", map[string]string{"/work/m": waiting}, nil)); !r.Taken {
		t.Fatal(r.Refused)
	}
	w.secret("K", "work's", "/work/m")
	var answers []string
	for _, name := range []string{"K", "NONE"} {
		r := w.answer(shop, func() (string, error) { return shop.RemoveSecret(ctx, "/work/m", name, 0) })
		answers = append(answers, strings.ReplaceAll(r.Refused, name, "?"))
	}
	if answers[0] != answers[1] || strings.Contains(answers[0], "not set") {
		t.Errorf("the removes of a name set and of one not set gave %q", answers)
	}
	if r := w.answer(shop, func() (string, error) { return shop.ListSecrets(ctx, 0) }); !r.Taken || r.Listed != nil {
		t.Errorf("the list gave %+v", r)
	}
	if got, _, _ := secret.Get(ctx, w.ctrl.Store, w.ctl, "/work/m", "K"); got != "work's" {
		t.Errorf("K is %q", got)
	}
}

// sendAs writes a set, remove or list into the controller's inbox under a
// name the client chose, and runs the controller.
func (w *world) sendAs(id *identity.Identity, name string, set instance.Setting) {
	w.t.Helper()
	at := store.Message("ctl", id.Name, name)
	data, err := id.Write(at, w.clock(), w.clock().Add(time.Hour), &instance.Message{Kind: instance.KindSet, Set: &set}, w.ctl.Card)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.clientOf(id).Store.Write(ctx, at, data, store.OnlyIfNew); err != nil {
		w.t.Fatal(err)
	}
	w.settle()
}

// A set or a remove is carried out only where the client may set at that
// moment, and only the one its receipt answered: a message under a name
// that was answered already is deleted, whatever it says.
func Test_ctl_setting_carried_out_only_as_answered_and_allowed(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"network", "at:/work"}}, nil)
	c3 := w.party("c3", identity.Client)
	w.enrol(c3, permission.Layer{Allow: []string{"network"}}, nil)
	if r := w.submitTree(w.client, w.tree("/home", "", map[string]string{"/home/m": waiting}, nil)); !r.Taken {
		t.Fatal(r.Refused)
	}
	w.secret("K", "the owner's", "/home")
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/", "HUB", "hub.lan"); err != nil {
		t.Fatal(err)
	}
	secretAt := func(at string) string {
		got, _, err := secret.Get(ctx, w.ctrl.Store, w.ctl, at, "K")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	empty := func() {
		t.Helper()
		if left := w.list(store.Inbox("ctl")); len(left) > 0 {
			t.Fatalf("the inbox holds %v", left)
		}
	}

	// The name of a taken set, sent again with a set and a remove in
	// another client's tree.
	w.sendAs(c2, "0001", instance.Setting{Of: "secret", Act: "set", At: "/work", Name: "K", Value: "mine"})
	w.sendAs(c2, "0001", instance.Setting{Of: "secret", Act: "set", At: "/home/m", Name: "K", Value: "planted by c2"})
	w.sendAs(c2, "0001", instance.Setting{Of: "secret", Act: "remove", At: "/home", Name: "K"})
	empty()
	if got := secretAt("/home/m"); got != "the owner's" {
		t.Errorf("K for /home/m is %q", got)
	}
	// The same location and name with another value.
	w.sendAs(c2, "0001", instance.Setting{Of: "secret", Act: "set", At: "/work", Name: "K", Value: "another"})
	if got := secretAt("/work"); got != "mine" {
		t.Errorf("K for /work is %q", got)
	}

	// The name of a list, from a client with no at: entry, sent again with
	// sets at /.
	w.sendAs(c3, "0001", instance.Setting{Of: "value", Act: "list"})
	w.sendAs(c3, "0001", instance.Setting{Of: "value", Act: "set", At: "/", Name: "HUB", Value: "evil.example"})
	w.sendAs(c3, "0001", instance.Setting{Of: "secret", Act: "set", At: "/", Name: "K", Value: "c3's"})
	empty()
	if values, err := secret.Values(ctx, w.ctrl.Store, w.ctl, "/home/m"); err != nil || values["HUB"] != "hub.lan" {
		t.Errorf("HUB for /home/m is %v (%v)", values, err)
	}
	if _, set, _ := secret.Get(ctx, w.ctrl.Store, w.ctl, "/x", "K"); set {
		t.Error("K is set at /")
	}

	// A set whose receipt was written, taken again after the client's
	// ceiling lost its at: entry, is not carried out.
	set := instance.Setting{Of: "secret", Act: "set", At: "/work", Name: "K", Value: "after"}
	w.sendAs(c2, "0002", set)
	if got := secretAt("/work"); got != "after" {
		t.Fatalf("K for /work is %q", got)
	}
	w.secret("K", "the owner's at /work", "/work")
	w.enrol(c2, permission.Layer{Allow: []string{"network"}}, nil)
	w.sendAs(c2, "0002", set)
	empty()
	if got := secretAt("/work"); got != "the owner's at /work" {
		t.Errorf("a set taken again after the ceiling changed left K for /work %q", got)
	}
	// With the entry back, the same message taken again after a stop
	// between the receipt and the set is carried out.
	w.enrol(c2, permission.Layer{Allow: []string{"network", "at:/work"}}, nil)
	w.sendAs(c2, "0002", set)
	if got := secretAt("/work"); got != "after" {
		t.Errorf("the set taken again left K for /work %q", got)
	}
}
