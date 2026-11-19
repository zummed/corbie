package machines

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie-ui/page/pagetest"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
)

const room = `statechart: room
lifetime: keyed
initial: open
states:
  open:
    on: { break: broke, shut: closed }
  closed:
    on: { open: open }
  broke: { end: failed }
`

// runsOf is the runs a client may read, as the UI's list of runs gives
// them.
type runsOf struct{ from *client.Client }

func (l runsOf) Runs() ([]*client.Run, error) {
	return l.from.List(context.Background(), client.Filter{Finished: true})
}
func (l runsOf) Changed() <-chan struct{} { return make(chan struct{}) }

// house runs a store in memory with a controller and a runner, submits a
// directory holding the keyed room at /house, and sends the room three
// events: two runs are open, one is closed, and one ended failed. It gives
// the directory and the Machines page acting as the controller and the
// client.
func house(t *testing.T) (string, *Pages) {
	t.Helper()
	ctx := context.Background()
	l, err := client.NewLocal(ctx, permission.Layer{Allow: []string{"all"}}, func(string, error) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	l.Controller.AnyRunner = true
	dir := t.TempDir()
	write(t, dir, map[string]string{"room.yaml": room})
	tree, err := client.ReadTree(dir, "/house")
	if err != nil {
		t.Fatal(err)
	}
	message, err := l.Submit(ctx, tree, 0)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := l.WaitReceipt(ctx, message, 0, client.LocalInterval); err != nil || !receipt.Taken {
		t.Fatalf("the submit was not taken: %+v %v", receipt, err)
	}
	for key, event := range map[string]string{"1": "knock", "2": "knock", "3": "shut", "4": "break"} {
		k := key
		if _, err := l.Send(ctx, machine.Address{At: "/house/room", Key: &k}, event, nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(20 * time.Second); ; {
		runs, _ := runsOf{l.Client}.Runs()
		failed := slices.ContainsFunc(runs, func(r *client.Run) bool { return r.Status == instance.Failed })
		if len(runs) == 4 && failed && slices.IndexFunc(runs, func(r *client.Run) bool { return r.Step < 1 }) < 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the room's runs never came: %+v", runs)
		}
		time.Sleep(50 * time.Millisecond)
	}
	p, err := New(Config{Dir: dir, Controller: l.Controller.ID, Store: l.Controller.Store, Runs: true, List: runsOf{l.Client}, Client: l.Client,
		Interval: 100 * time.Millisecond, Shell: page.Shell{Offered: []string{"Runs", "Machines", "Toolkits"}}})
	if err != nil {
		t.Fatal(err)
	}
	return dir, p
}

func Test_machines_a_location_counts_its_runs(t *testing.T) {
	_, p := house(t)
	ix, err := p.h.tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	counted, err := p.h.counted(context.Background(), ix)
	if err != nil {
		t.Fatal(err)
	}
	at := counted["/house/room"]
	if at.Running != 3 || at.FailedToday != 1 {
		t.Errorf("the room counts %d running and %d failed today", at.Running, at.FailedToday)
	}
	if at.In["open"] != 2 || at.In["closed"] != 1 || len(at.In) != 2 {
		t.Errorf("the room's states hold %v of its runs", at.In)
	}
	if len(counted) != 1 {
		t.Errorf("runs are counted at locations the directory has no machine at: %v", counted)
	}

	// Another client's runs are counted at their location, and not in its
	// states, which the UI cannot read.
	p.h.Client = nil
	if got := must(p.h.counted(context.Background(), ix))["/house/room"]; got.Running != 3 || got.In != nil {
		t.Errorf("acting as no client, the room counts %+v", got)
	}

	// With no store, nothing is counted.
	p.h.List = nil
	if got := must(p.h.counted(context.Background(), ix)); len(got) != 0 {
		t.Errorf("with no store, the page counts %v", got)
	}
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func Test_machines_a_run_differs_from_its_file(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"room.yaml": room})
	p, err := New(Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	location := "/" + filepath.Base(dir) + "/room"
	same := must(machine.Parse([]byte(room), machine.Given(nil)))
	if got, err := p.Differs(ctx, location, real, same.Definition); err != nil || got != nil {
		t.Errorf("a run started from the file as it is differs by %q, %v", got, err)
	}
	older := strings.Replace(room, "    on: { break: broke, shut: closed }\n", "    on: { break: broke }\n", 1)
	was := must(machine.Parse([]byte("# an older room\n"+older), machine.Given(nil)))
	got, err := p.Differs(ctx, location, real, was.Definition)
	if err != nil || len(got) == 0 || !slices.ContainsFunc(got, func(l string) bool { return l == `+ states.open.on.shut.0.go: "closed"` }) {
		t.Errorf("a run started from an older file differs by %q, %v", got, err)
	}
	for _, l := range got {
		if !strings.HasPrefix(l, "+ ") && !strings.HasPrefix(l, "- ") {
			t.Errorf("a line of the difference is %q", l)
		}
	}
	if got, err := p.Differs(ctx, location, t.TempDir(), was.Definition); err != nil || got != nil {
		t.Errorf("a run of a tree submitted from another directory differs by %q, %v", got, err)
	}
	if got, err := p.Differs(ctx, location+"-gone", real, was.Definition); err != nil || got != nil {
		t.Errorf("a run at a location with no machine differs by %q, %v", got, err)
	}
}

// Test_machines_counts_in_the_browser runs counts_test.js on the room of a
// house with runs, as the UI acting as the controller and its client shows
// it.
func Test_machines_counts_in_the_browser(t *testing.T) {
	dir, p := house(t)
	pagetest.Run(t, pagetest.Config{Handler: servedBy(dir, p), Page: "/machines?file=room.yaml", Module: "machines/counts_test.js"})
}
