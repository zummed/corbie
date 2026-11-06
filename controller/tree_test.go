package controller_test

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// waiting is a singleton that runs until it is sent end, and then gives the
// value HOST as its output.
const waiting = `
	statechart: waiting
	lifetime: singleton
	initial: w
	states:
	  w: { on: { end: over } }
	  over: { end: done, output: "${values.HOST}" }
`

// tree makes a tree of definitions written in the test, each by its
// location, from a directory.
func (w *world) tree(prefix, dir string, files map[string]string, scripts map[string]string) *client.Tree {
	w.t.Helper()
	t := &client.Tree{Prefix: prefix, Directory: dir}
	for _, location := range sortedNames(files) {
		draft, err := machine.ParseAt(path.Base(location)+".yaml", []byte(dedent(files[location])), func(path string) ([]byte, error) {
			if contents, ok := scripts[path]; ok {
				return []byte(dedent(contents)), nil
			}
			return nil, fmt.Errorf("no such file")
		})
		if err != nil {
			w.t.Fatalf("%s does not pass:\n%v", location, err)
		}
		t.Machines = append(t.Machines, &client.Machine{Path: location + ".yaml", Location: location, Draft: draft})
	}
	return t
}

func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// submitTree sends a tree from a client and runs the controller until it
// comes to rest. It gives the receipt.
func (w *world) submitTree(c *client.Client, tree *client.Tree) *instance.Receipt {
	w.t.Helper()
	message, err := c.Submit(ctx, tree, 0)
	if err != nil {
		w.t.Fatal(err)
	}
	w.settle()
	r, err := c.Receipt(ctx, message)
	if err != nil || r == nil {
		w.t.Fatalf("the submit has no receipt: %v", err)
	}
	return r
}

// at gives the root runs at a location, each with its status.
func (w *world) at(location string) map[string]string {
	runs := map[string]string{}
	for _, name := range append(w.list(store.Running("ctl")), w.list(store.Finished("ctl"))...) {
		head := w.head(store.InstanceOf("ctl", name))
		if head != nil && head.Location == location && head.Parent == "" {
			runs[head.Name] = head.Status
		}
	}
	return runs
}

// A client submits a tree only at a prefix an at: entry of its ceiling
// matches, and the refusal gives the line that allows it.
func Test_ctl_submit_needs_an_at_entry(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"network"}}, nil)
	tree := w.tree("/home", "/srv/house", map[string]string{"/home/m": waiting}, nil)
	r := w.submitTree(w.clientOf(c2), tree)
	if r.Taken || !strings.Contains(r.Refused, `at:/home was refused by the ceiling of the client "c2"`) ||
		!strings.Contains(r.Refused, "corbie controller enrol c2 --allow at:/home") {
		t.Fatalf("the submit gave %+v", r)
	}
	if runs := w.at("/home/m"); len(runs) > 0 {
		t.Fatalf("a refused submit started %v", runs)
	}
	w.enrol(c2, permission.Layer{Allow: []string{"network", "at:/home"}}, nil)
	if r := w.submitTree(w.clientOf(c2), tree); !r.Taken {
		t.Fatalf("with at:/home the submit was refused: %s", r.Refused)
	}
}

// Trees do not nest, a tree belongs to the client that first submitted it,
// / is never a tree, and a location has nothing below it.
func Test_ctl_submit_is_refused_naming_the_tree(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"all"}}, nil)
	// A tree of the locations given, or of one file at its prefix.
	at := func(prefix string, locations ...string) *client.Tree {
		if len(locations) == 0 {
			locations = []string{prefix}
		}
		files := map[string]string{}
		for _, location := range locations {
			files[location] = waiting
		}
		return w.tree(prefix, "", files, nil)
	}
	for _, prefix := range []string{"/home", "/a/b"} {
		if r := w.submitTree(w.client, at(prefix)); !r.Taken {
			t.Fatalf("%s: %s", prefix, r.Refused)
		}
	}
	for refused, try := range map[string]func() *instance.Receipt{
		"/home/x is inside the tree /home":                 func() *instance.Receipt { return w.submitTree(w.client, at("/home/x")) },
		"the tree /a/b is inside /a":                       func() *instance.Receipt { return w.submitTree(w.client, at("/a", "/a/c")) },
		"the tree /home belongs to the client c1":          func() *instance.Receipt { return w.submitTree(w.clientOf(c2), at("/home")) },
		"/ is never a tree":                                func() *instance.Receipt { return w.submitTree(w.client, at("/", "/m")) },
		"the location /h/a/b is below /h/a":                func() *instance.Receipt { return w.submitTree(w.client, at("/h", "/h/a", "/h/a/b")) },
		"the location /elsewhere/m is not in the tree /in": func() *instance.Receipt { return w.submitTree(w.client, at("/in", "/elsewhere/m")) },
		"the location /r/m is below /r":                    func() *instance.Receipt { return w.submitTree(w.client, at("/r", "/r", "/r/m")) },
	} {
		if r := try(); r.Taken || !strings.Contains(r.Refused, refused) {
			t.Errorf("want %q, the submit gave %+v", refused, r)
		}
	}
	// On a file system that ignores case, /Home's entry is /home's.
	data, _, err := w.owner.Read(ctx, store.Tree("ctl", "/home"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Write(ctx, store.Tree("ctl", "/Home"), data, store.Overwrite); err != nil {
		t.Fatal(err)
	}
	if r := w.submitTree(w.client, at("/Home")); r.Taken || !strings.Contains(r.Refused, "the tree /home is kept in the same file as /Home") {
		t.Errorf("a tree of another case gave %+v", r)
	}
	// A submit taken after a later one of the same tree is refused.
	var messages []string
	for range 2 {
		message, err := w.client.Submit(ctx, at("/home"), 0)
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, store.Message("ctl", "c1", message))
	}
	earlier, _, err := w.owner.Read(ctx, messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Delete(ctx, messages[0], store.Overwrite); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if _, err := w.owner.Write(ctx, messages[0], earlier, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(path.Base(messages[0])); r == nil || r.Taken || !strings.Contains(r.Refused, "a later submit of the tree /home was taken before this one") {
		t.Errorf("the earlier submit gave %+v", r)
	}
}

// The receipt says what the submit did at each location, the directory
// submitted there before when it was another, and the secrets and values
// the files use that are not set at their locations or above.
func Test_ctl_submit_receipt_names_what_it_did(t *testing.T) {
	w := newWorld(t)
	uses := `
		flow: uses
		stages:
		  s: { job: { script: ./work.js, allow: ["secret:TOKEN@net:${values.HOST}:443"] } }
	`
	files := map[string]string{"/h/uses": uses, "/h/waiting": waiting}
	r := w.submitTree(w.client, w.tree("/h", "/srv/a", files, map[string]string{"work.js": "return 1"}))
	done := r.Tree
	if !r.Taken || done.Before != "" || len(done.Locations) != 2 || done.Locations[1].What != "started" ||
		done.Locations[0] != (instance.Recorded{Location: "/h/uses", Lifetime: "transient", What: "recorded"}) {
		t.Fatalf("the first submit gave %+v %+v", r, done)
	}
	if want := []string{"secret TOKEN for /h/uses", "value HOST for /h/uses", "value HOST for /h/waiting"}; !slices.Equal(done.Missing, want) {
		t.Fatalf("the reminder is %q", done.Missing)
	}
	if err := secret.Set(ctx, w.ctrl.Store, w.ctl, "/h", "TOKEN", "t"); err != nil {
		t.Fatal(err)
	}
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/", "HOST", "hub.lan"); err != nil {
		t.Fatal(err)
	}
	done = w.submitTree(w.client, w.tree("/h", "/srv/b", files, map[string]string{"work.js": "return 1"})).Tree
	if done.Before != "/srv/a" || len(done.Missing) > 0 || done.Locations[1].What != "recorded" {
		t.Fatalf("the second submit gave %+v", done)
	}
}

// A singleton is started by the submit once, and not again while it runs.
// Once it has ended, the next submit starts it again.
func Test_ctl_singleton_is_started_once(t *testing.T) {
	w := newWorld(t)
	tree := w.tree("/s", "", map[string]string{"/s": waiting}, nil)
	first := w.submitTree(w.client, tree).Tree.Locations[0].Run
	w.submitTree(w.client, tree)
	if runs := w.at("/s"); len(runs) != 1 || runs[first] != "running" {
		t.Fatalf("the runs at /s are %v", runs)
	}
	if _, err := w.client.Cancel(ctx, first, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	again := w.submitTree(w.client, tree).Tree.Locations[0]
	if again.What != "started" || again.Run == first || w.at("/s")[again.Run] != "running" {
		t.Fatalf("after the run ended the submit did %+v: %v", again, w.at("/s"))
	}
}

// A singleton started again on a changed file starts only once the run it
// replaces has ended: the old run's finished head is written before the
// new run's first step.
func Test_ctl_singleton_restart_has_no_overlap(t *testing.T) {
	w := newWorld(t)
	first := w.submitTree(w.client, w.tree("/s", "", map[string]string{"/s": waiting}, nil)).Tree.Locations[0].Run
	w.noted()
	changed := strings.Replace(waiting, "initial: w", "initial: w\n\ttitle: changed", 1)
	message, err := w.client.Submit(ctx, w.tree("/s", "", map[string]string{"/s": changed}, nil), 0)
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		w.settle()
		w.tick(time.Second)
	}
	done := w.receipt(message).Tree
	second := done.Locations[0].Run
	if done.Locations[0].What != "restarted" || !slices.Equal(done.Cancelled, []string{first}) {
		t.Fatalf("the submit did %+v", done)
	}
	ended, began := -1, -1
	for i, op := range w.noted() {
		switch {
		case op.Act == store.ActAdd && op.Done && op.Name == store.FinishedHead("ctl", first):
			ended = i
		case op.Act == store.ActAdd && op.Done && op.Name == store.Step("ctl", second, 1):
			began = i
		}
	}
	if ended < 0 || began < ended {
		t.Fatalf("the old run ended at write %d and the new one began at %d", ended, began)
	}
	if runs := w.at("/s"); runs[first] != "cancelled" || runs[second] != "running" || w.latest(second).State.Title != "changed" {
		t.Fatalf("the runs at /s are %v", runs)
	}
	// Started again twice before the running run has ended, the second
	// start waits for that run too, and the first is left.
	var messages []string
	for _, title := range []string{"twice", "thrice"} {
		message, err := w.client.Submit(ctx, w.tree("/s", "", map[string]string{"/s": strings.Replace(changed, "changed", title, 1)}, nil), 0)
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	for range 6 {
		w.settle()
		w.tick(time.Second)
	}
	left, third := w.receipt(messages[0]).Tree.Locations[0].Run, w.receipt(messages[1]).Tree.Locations[0].Run
	ended, began = -1, -1
	for i, op := range w.noted() {
		switch {
		case op.Act == store.ActAdd && op.Done && op.Name == store.FinishedHead("ctl", second):
			ended = i
		case op.Act == store.ActAdd && op.Done && op.Name == store.Step("ctl", third, 1):
			began = i
		}
	}
	if ended < 0 || began < ended {
		t.Fatalf("the running run ended at write %d and the third began at %d", ended, began)
	}
	if runs := w.at("/s"); len(runs) != 3 || runs[second] != "cancelled" || runs[third] != "running" || w.head(left) != nil {
		t.Fatalf("the runs at /s are %v", runs)
	}
}

// A file whose machine is the same, written on other lines, is recorded
// with its new lines and leaves its runs as they were.
func Test_ctl_a_moved_line_is_recorded(t *testing.T) {
	w := newWorld(t)
	transient := strings.Replace(waiting, "lifetime: singleton", "update: restart", 1)
	w.submitTree(w.client, w.tree("/m", "", map[string]string{"/m": transient}, nil))
	start := func() string {
		run, err := w.client.Start(ctx, client.Start{At: "/m"})
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		return run
	}
	line := func(run string) string {
		def, err := machine.Load(ctx, w.owner, w.latest(run).State.Definition)
		if err != nil {
			t.Fatal(err)
		}
		return def.Lines["initial"]
	}
	before := start()
	done := w.submitTree(w.client, w.tree("/m", "", map[string]string{"/m": "# moved\n" + dedent(transient)}, nil)).Tree
	if done.Locations[0].What != "recorded" || len(done.Cancelled) > 0 || w.head(before).Status != "running" {
		t.Fatalf("the submit did %+v", done)
	}
	if after := start(); line(before) == line(after) {
		t.Fatalf("both runs have initial: at %s", line(after))
	}
}

// A file gone from the tree cancels the root runs at its location, and a
// child running at it ends with its parent.
func Test_ctl_removal_cancels_only_root_runs(t *testing.T) {
	w := newWorld(t)
	parent := `
		statechart: parent
		lifetime: singleton
		initial: w
		states:
		  w:
		    job: { machine: { statechart: child, initial: c, states: { c: { on: { never: d } }, d: { end: done } } } }
	`
	root := w.submitTree(w.client, w.tree("/r", "", map[string]string{"/r/p": parent, "/r/q": waiting}, nil)).Tree.Locations[0].Run
	child := ""
	for _, name := range w.list(store.Running("ctl")) {
		if head := w.head(store.InstanceOf("ctl", name)); head.Parent == root {
			child = head.Name
			if head.Location != "/r/p" {
				t.Fatalf("the child runs at %q", head.Location)
			}
		}
	}
	done := w.submitTree(w.client, w.tree("/r", "", map[string]string{"/r/q": waiting}, nil)).Tree
	if !slices.Equal(done.Cancelled, []string{root}) || done.Locations[0].What != "removed" {
		t.Fatalf("the submit did %+v", done)
	}
	if w.head(root).Status != "cancelled" || w.head(child).Status != "cancelled" {
		t.Fatalf("the root is %s and its child %s", w.head(root).Status, w.head(child).Status)
	}
	if names := w.list(store.LocationDir("ctl", "/r/p")); len(names) > 0 {
		t.Fatalf("the location is still there: %v", names)
	}
}

// A client starts a transient location in a tree it owns, and no other
// location: a singleton starts at a submit and a keyed run at a send.
func Test_ctl_client_starts_its_own_transient_locations(t *testing.T) {
	w := newWorld(t)
	c2 := w.party("c2", identity.Client)
	w.enrol(c2, permission.Layer{Allow: []string{"all"}}, nil)
	transient := strings.Replace(waiting, "lifetime: singleton", "", 1)
	keyed := strings.Replace(waiting, "lifetime: singleton", "lifetime: keyed", 1)
	held := "flow: held\nstages:\n  s: { job: { script: ./work.js, approve: true } }\n"
	done := w.submitTree(w.client, w.tree("/t", "", map[string]string{"/t/a": transient, "/t/s": waiting, "/t/k": keyed, "/t/held": held},
		map[string]string{"work.js": "return 1"})).Tree
	if done.Locations[1] != (instance.Recorded{Location: "/t/held", Lifetime: "transient", What: "recorded"}) {
		t.Fatalf("a file whose approvals have no ask: %+v", done.Locations)
	}
	start := func(c *client.Client, at string) string {
		run, err := c.Start(ctx, client.Start{At: at})
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		r, err := c.Receipt(ctx, strings.SplitN(run, "/", 2)[1])
		if err != nil || r == nil {
			t.Fatalf("no receipt for %s: %v", run, err)
		}
		return r.Refused
	}
	if refused := start(w.client, "/t/a"); refused != "" {
		t.Fatalf("the start of /t/a was refused: %s", refused)
	}
	for at, want := range map[string]string{
		"/t/s":    "is a singleton: its run starts when its tree is submitted",
		"/t/k":    "is keyed: a run starts there when an event is sent to a key",
		"/t/none": "nothing is recorded at /t/none",
		"/t/held": "the definition did not pass its check",
	} {
		if refused := start(w.client, at); !strings.Contains(refused, want) {
			t.Errorf("the start of %s gave %q", at, refused)
		}
	}
	// Another client's start is a call, which /t/a's receive: refuses.
	if refused := start(w.clientOf(c2), "/t/a"); !strings.Contains(refused, "/t/a does not receive a call from client:c2") {
		t.Fatalf("another client's start gave %q", refused)
	}
}

// A run reads the values set at its location or above once, at its first
// step, and keeps them; a value set nearer wins. A value not set fails what
// reads it, naming the value and the location.
func Test_ctl_values_are_read_at_the_first_step(t *testing.T) {
	w := newWorld(t)
	set := func(at, value string) {
		if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, at, "HOST", value); err != nil {
			t.Fatal(err)
		}
	}
	transient := strings.Replace(waiting, "lifetime: singleton", "", 1)
	w.submitTree(w.client, w.tree("/v", "", map[string]string{"/v/m": transient}, nil))
	start := func() string {
		run, err := w.client.Start(ctx, client.Start{At: "/v/m"})
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		return run
	}
	end := func(run string) *instance.State {
		if _, err := w.client.Send(ctx, w.to(run), "end", nil, 0); err != nil {
			t.Fatal(err)
		}
		w.settle()
		return &w.latest(run).State
	}
	unset := start()
	if st := end(unset); st.Status != "failed" || !strings.Contains(st.Error.Error, "the value HOST is not set at /v/m or above") {
		t.Fatalf("with no value the run ended %s: %v", st.Status, st.Error)
	}
	set("/", "far")
	set("/v", "near")
	run := start()
	set("/v", "later")
	if st := end(run); st.Status != "done" || st.Output != "near" {
		t.Fatalf("the run ended %s with %v: %v", st.Status, st.Output, st.Error)
	}
	// A child takes its parent's values.
	parent := `
		flow: parent
		stages:
		  kid: { job: { machine: { statechart: kid, initial: e, states: { e: { end: done, output: "${values.HOST}" } } } } }
		output: "${stages.kid.output}"
	`
	w.submitTree(w.client, w.tree("/p", "", map[string]string{"/p/m": parent}, nil))
	set("/p", "parent's")
	id, err := w.client.Start(ctx, client.Start{At: "/p/m"})
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	if st := w.latest(id).State; st.Status != "done" || st.Output != "parent's" {
		t.Fatalf("the parent ended %s with %v: %v", st.Status, st.Output, st.Error)
	}
}

// A value fills an allow scope, and a secret set nearest the run's location
// is the one its job is sent.
func Test_ctl_values_and_secrets_by_location(t *testing.T) {
	w := newWorld(t)
	r1 := w.party("r1", identity.Runner)
	w.enrol(r1, permission.Layer{Allow: []string{"all"}}, nil)
	for at, value := range map[string]string{"/": "far", "/w": "near"} {
		if err := secret.Set(ctx, w.ctrl.Store, w.ctl, at, "K", value); err != nil {
			t.Fatal(err)
		}
	}
	if err := secret.SetValue(ctx, w.ctrl.Store, w.ctl, "/", "HOST", "hub.lan"); err != nil {
		t.Fatal(err)
	}
	uses := `
		flow: uses
		allow: ["net:${values.HOST}:443"]
		stages:
		  s: { job: { script: ./work.js, allow: ["secret:K@net:${values.HOST}:443"] } }
	`
	w.submitTree(w.client, w.tree("/w", "", map[string]string{"/w/m": uses}, map[string]string{"work.js": "return 1"}))
	if _, err := w.client.Start(ctx, client.Start{At: "/w/m"}); err != nil {
		t.Fatal(err)
	}
	w.settle()
	jobs := w.list(store.Inbox("r1"))
	if len(jobs) != 1 {
		t.Fatalf("the runner was sent %v", jobs)
	}
	data, _, _ := w.owner.Read(ctx, jobs[0])
	var msg instance.Message
	if _, err := identity.Read(data, jobs[0], w.ctl.Card, r1, &msg); err != nil {
		t.Fatal(err)
	}
	own := msg.Job.Layers[0].Allow
	if msg.Job.Secrets["K"] != "near" || !slices.Equal(own, []string{"net:hub.lan:443", "secret:K@net:hub.lan:443"}) {
		t.Fatalf("the job was sent the secret %q and the list %v", msg.Job.Secrets["K"], own)
	}
}

// What a submit leaves in the store: its tree's entry, its locations and
// their runs, and nothing waiting.
func (w *world) trees() string {
	var b strings.Builder
	for _, name := range w.list(store.Locations("ctl")) {
		if location, entry, ok := store.ParseLocation("ctl", name); ok && !strings.HasPrefix(entry, "blobs/") {
			fmt.Fprintf(&b, "%s %s\n", location, entry)
		}
	}
	for _, location := range []string{"/x/kept", "/x/gone", "/x/plain"} {
		var statuses []string
		for _, status := range w.at(location) {
			statuses = append(statuses, status)
		}
		slices.Sort(statuses)
		fmt.Fprintf(&b, "%s runs: %v\n", location, statuses)
	}
	fmt.Fprintf(&b, "waiting: %v uploads: %v\n", w.list(store.Inbox("ctl")), w.list("ctl/uploads"))
	return b.String()
}

// story submits a tree, then the same tree changed with a file gone, and
// stops the controller after a number of writes in the second; a controller
// that has never run before carries on, late by as much as is given. It
// gives what is left, the second receipt, and whether the stop happened.
func submitStory(t *testing.T, stopAfter int64, late time.Duration) (string, *instance.Receipt, bool) {
	w := newWorld(t)
	plain := "flow: plain\nstages:\n  s: { job: { script: ./one.js } }\n"
	gone := strings.Replace(waiting, "statechart: waiting", "statechart: gone", 1)
	w.submitTree(w.client, w.tree("/x", "/srv/a", map[string]string{"/x/kept": waiting, "/x/gone": gone, "/x/plain": plain}, map[string]string{"one.js": "return 1"}))
	changed := strings.Replace(waiting, "initial: w", "initial: w\n\ttitle: changed", 1)
	message, err := w.client.Submit(ctx, w.tree("/x", "/srv/b", map[string]string{"/x/kept": changed, "/x/plain": plain + "title: p\n"}, map[string]string{"one.js": "return 1"}), 0)
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

// A controller stopped after any single write of a submit leaves what a
// controller never stopped leaves: one entry for the tree, each file at its
// location, one singleton running on the changed file, the removed file's
// run cancelled, and the same receipt. So does one that comes back after
// the message would have been too late: the tree's entry, written first,
// says the submit was taken.
func Test_ctl_submit_stopped_after_every_write(t *testing.T) {
	want, wantReceipt, _ := submitStory(t, 1<<40, 0)
	if !strings.Contains(want, "/x/kept runs: [cancelled running]") || !strings.Contains(want, "/x/gone runs: [cancelled]") ||
		!strings.Contains(want, "waiting: [] uploads: []") || strings.Contains(want, "/x/gone file") {
		t.Fatalf("the submit never stopped left:\n%s", want)
	}
	tooLate := instance.MessageLife + identity.ClockAllowance + time.Minute
	for _, late := range []time.Duration{0, tooLate} {
		for n := int64(0); ; n++ {
			got, receipt, stopped := submitStory(t, n, late)
			if !stopped {
				if n < 10 {
					t.Fatalf("the submit made only %d writes", n)
				}
				break
			}
			if late > 0 && n == 0 {
				// Stopped before its first write, the submit was never taken.
				if receipt == nil || receipt.Taken || !strings.Contains(receipt.Refused, "the message came too late") {
					t.Fatalf("taken late before any write, the receipt is %+v", receipt)
				}
				continue
			}
			if got != want {
				t.Fatalf("stopped after %d writes and late by %s, the submit left:\n%s\nnever stopped:\n%s", n, late, got, want)
			}
			if receipt == nil || !receipt.Taken || len(receipt.Tree.Locations) != len(wantReceipt.Tree.Locations) ||
				len(receipt.Tree.Cancelled) != len(wantReceipt.Tree.Cancelled) || len(receipt.Tree.Cancelled) != 2 {
				t.Fatalf("stopped after %d writes and late by %s, the receipt is %+v", n, late, receipt)
			}
			for i, done := range receipt.Tree.Locations {
				if done.What != wantReceipt.Tree.Locations[i].What {
					t.Fatalf("stopped after %d writes and late by %s, the receipt says %+v", n, late, receipt.Tree.Locations)
				}
			}
		}
	}
}

// A submit's files are read only from its own uploads, and what the
// controller keeps of a tree and its locations is sealed to it.
func Test_ctl_submit_reads_only_its_own_uploads(t *testing.T) {
	w := newWorld(t)
	w.submitTree(w.client, w.tree("/k", "", map[string]string{"/k": waiting}, nil))
	for _, name := range []string{store.Tree("ctl", "/k"), store.LocationFile("ctl", "/k")} {
		data, _, err := w.owner.Read(ctx, name)
		if err != nil || data == nil {
			t.Fatalf("%s: %v", name, err)
		}
		var v any
		if _, err := identity.Read(data, name, w.ctl.Card, w.c1, &v); err == nil {
			t.Fatalf("the client opens %s", name)
		}
	}
	// A definition in a record of the client's own, named in a submit.
	run := w.submit(waiting, nil, nil)
	w.settle()
	ref := w.latest(run).State.Definition
	name := "20261010T120000.000000Z-000001"
	msg := &instance.Message{Kind: instance.KindSubmit, Submit: &instance.Submit{Prefix: "/o",
		Files: map[string]instance.SubmitFile{"/o": {Definition: ref, Hash: "h"}}}}
	at := store.Message("ctl", "c1", name)
	data, err := w.c1.Write(at, w.clock(), w.clock().Add(time.Minute), msg, w.ctl.Card)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.client.Store.Write(ctx, at, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(name); r == nil || r.Taken || !strings.Contains(r.Refused, "the file at /o is not among the submit's uploads") {
		t.Fatalf("the submit gave %+v", r)
	}
	// A definition among the uploads that names a script in that record.
	name = "20261010T120000.000000Z-000002"
	elsewhere := store.Blob("ctl", run, blob.NewName())
	draft := w.tree("/o", "", map[string]string{"/o": "flow: plain\nstages:\n  s: { job: { script: ./one.js } }\n"},
		map[string]string{"one.js": "return 1"}).Machines[0].Draft
	def, err := draft.Write(ctx, func(typ string, contents []byte) (blob.Ref, error) {
		if typ != blob.Definition {
			return blob.Write(ctx, w.client.Store, elsewhere, typ, blob.NewKey(), bytes.NewReader(contents))
		}
		contents = bytes.Replace(contents, []byte(`"`+path.Base(elsewhere)+`"`), []byte(`"`+elsewhere+`"`), 1)
		return blob.Write(ctx, w.client.Store, store.Upload("ctl", "c1", name, blob.NewName()), typ, blob.NewKey(), bytes.NewReader(contents))
	})
	if err != nil {
		t.Fatal(err)
	}
	msg = &instance.Message{Kind: instance.KindSubmit, Submit: &instance.Submit{Prefix: "/o",
		Files: map[string]instance.SubmitFile{"/o": {Definition: def, Hash: "h"}}}}
	at = store.Message("ctl", "c1", name)
	if data, err = w.c1.Write(at, w.clock(), w.clock().Add(time.Minute), msg, w.ctl.Card); err != nil {
		t.Fatal(err)
	}
	if _, err := w.client.Store.Write(ctx, at, data, store.OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if r := w.receipt(name); r == nil || r.Taken || !strings.Contains(r.Refused, "the file at /o names a blob that is not among the submit's uploads") {
		t.Fatalf("the submit gave %+v", r)
	}
}
