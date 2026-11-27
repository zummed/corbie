package machines

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"
)

// room is a keyed room whose runs the tests send events to. Its job and
// its child machine are in states no test enters, so no job runs.
const room = `statechart: room
lifetime: keyed
initial: open
states:
  open:
    on: { break: broke, shut: closed, clean: cleaning, call: calling }
  closed:
    on: { open: open }
  cleaning:
    job: ./clean.js
  calling:
    job: { machine: ./inner.yaml }
  broke: { end: failed }
`

const inner = "statechart: inner\ninitial: a\nstates:\n  a: { end: done }\n"

// poked is the list of runs a client may read, as the UI's list gives
// them, and a signal closed when a test pokes it.
type poked struct {
	from    *client.Client
	mu      sync.Mutex
	changed chan struct{}
}

func (l *poked) Runs() ([]*client.Run, error) {
	return l.from.List(context.Background(), client.Filter{Finished: true})
}

func (l *poked) Changed() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.changed == nil {
		l.changed = make(chan struct{})
	}
	return l.changed
}

func (l *poked) poke() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.changed != nil {
		close(l.changed)
	}
	l.changed = make(chan struct{})
}

// latest reads a run's latest step, as the Runs page's steps do.
func latest(ctx context.Context, c *client.Client, run *client.Run) (*instance.Step, error) {
	step, err := instance.Latest(ctx, c.Store, c.Controller, c.ID, run.ID, run.Step)
	if errors.Is(err, identity.ErrNotForUs) {
		return nil, nil
	}
	return step, err
}

// A house is a store in memory with a controller and a runner, to which a
// directory holding the keyed room is submitted at /house, with the
// Machines page on it acting as the controller and the client.
type house struct {
	dir   string
	l     *client.Local
	list  *poked
	pages *Pages
}

// newHouse sets a house up and sends the room four events: two runs are
// open, one is closed, and one ended failed. The page reads again at the
// interval, or when the list is poked.
func newHouse(t *testing.T, interval time.Duration) *house {
	t.Helper()
	ctx := context.Background()
	l, err := client.NewLocal(ctx, permission.Layer{Allow: []string{"all"}}, func(string, error) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	l.Controller.AnyRunner = true
	dir := t.TempDir()
	write(t, dir, map[string]string{"room.yaml": room, "clean.js": "return 'clean'", "inner.yaml": inner})
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
	h := &house{dir: dir, l: l, list: &poked{from: l.Client}}
	for key, event := range map[string]string{"1": "knock", "2": "knock", "3": "shut", "4": "break"} {
		h.send(t, key, event)
	}
	h.waitFor(t, func(runs []*client.Run) bool {
		return len(runs) == 4 && slices.ContainsFunc(runs, func(r *client.Run) bool { return r.Status == instance.Failed })
	})
	h.pages = New(Config{Controller: l.Controller.ID, Store: l.Controller.Store, List: h.list, Client: l.Client, Latest: latest,
		Interval: interval, Shell: page.Shell{Offered: []string{"Runs", "Machines"}}})
	return h
}

// send sends the room's run at a key an event, which starts it where none
// runs.
func (h *house) send(t *testing.T, key, event string) {
	t.Helper()
	if _, err := h.l.Send(context.Background(), machine.Address{At: "/house/room", Key: &key}, event, nil, 0); err != nil {
		t.Fatal(err)
	}
}

// waitFor waits until the runs the client reads match, each past its
// first step.
func (h *house) waitFor(t *testing.T, match func([]*client.Run) bool) []*client.Run {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); ; {
		runs, _ := h.list.Runs()
		if match(runs) && !slices.ContainsFunc(runs, func(r *client.Run) bool { return r.Step < 1 }) {
			return runs
		}
		if time.Now().After(deadline) {
			t.Fatalf("the room's runs never came to match: %+v", runs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func Test_machines_a_location_counts_its_runs(t *testing.T) {
	h := newHouse(t, time.Second)
	p := h.pages
	ix, err := p.h.index.current(context.Background())
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
		t.Errorf("runs are counted at locations with none: %v", counted)
	}

	// Another client's runs are counted at their location, and not in its
	// states, which the UI cannot read.
	p.h.Client = nil
	if got := must(p.h.counted(context.Background(), ix))["/house/room"]; got.Running != 3 || got.In != nil {
		t.Errorf("acting as no client, the room counts %+v", got)
	}

	// With no list, nothing is counted.
	p.h.List = nil
	if got := must(p.h.counted(context.Background(), ix)); len(got) != 0 {
		t.Errorf("with no list, the page counts %v", got)
	}
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

// submit writes files into the house's directory and records it again at
// /house.
func (h *house) submit(t *testing.T, files map[string]string) {
	t.Helper()
	write(t, h.dir, files)
	(&lab{l: h.l}).submit(t, h.dir, "/house")
}

func Test_machines_a_run_differs_from_the_definition_recorded(t *testing.T) {
	h := newHouse(t, time.Second)
	ctx := context.Background()
	runs, err := h.list.Runs()
	if err != nil {
		t.Fatal(err)
	}
	run := runs[slices.IndexFunc(runs, func(r *client.Run) bool { return r.Key == "1" })]
	def := must(latest(ctx, h.l.Client, run)).State.Definition
	differs := func() []string {
		t.Helper()
		got, err := h.pages.Differs(ctx, "/house/room", def)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := differs(); got != nil {
		t.Errorf("a run started from the definition recorded differs by %q", got)
	}
	// The room drains: a submit leaves its runs on the definition they
	// started with.
	h.submit(t, map[string]string{"clean.js": "return 'cleaner'"})
	if got := differs(); !slices.Equal(got, []string{"a script changed: ./clean.js"}) {
		t.Errorf("with its script changed, a run differs by %q", got)
	}
	h.submit(t, map[string]string{"clean.js": "return 'clean'", "inner.yaml": strings.Replace(inner, "end: done", "end: failed", 1)})
	if got := differs(); !slices.Equal(got, []string{"a child changed: ./inner.yaml"}) {
		t.Errorf("with its child changed, a run differs by %q", got)
	}
	h.submit(t, map[string]string{"inner.yaml": inner, "room.yaml": "# a room\n" + strings.Replace(room, "  broke: { end: failed }\n", "  broke: { end: done }\n", 1)})
	if got := differs(); !slices.Equal(got, []string{`- states.broke.end: "failed"`, `+ states.broke.end: "done"`}) {
		t.Errorf("with a value changed, a run differs by %q", got)
	}
	if got, err := h.pages.Differs(ctx, "/house/gone", def); err != nil || got != nil {
		t.Errorf("a run at a location with no machine differs by %q, %v", got, err)
	}
}

// Test_machines_counts_in_the_browser runs counts_test.js on the room of a
// house with runs, as the UI acting as the controller and its client shows
// it. The page reads again only when the list of runs changes: its
// interval is an hour. POST /started starts the room's run at a key and
// pokes the list, as the UI's list does when it reads a change.
func Test_machines_counts_in_the_browser(t *testing.T) {
	h := newHouse(t, time.Hour)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /started", func(w http.ResponseWriter, r *http.Request) {
		key := r.FormValue("key")
		if _, err := h.l.Send(r.Context(), machine.Address{At: "/house/room", Key: &key}, "knock", nil, 0); err != nil {
			page.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
			runs, err := h.list.Runs()
			if err == nil && slices.ContainsFunc(runs, func(r *client.Run) bool { return r.Key == key && r.Step > 0 }) {
				break
			}
			if time.Now().After(deadline) {
				page.Error(w, http.StatusInternalServerError, "the run never started")
				return
			}
		}
		h.list.poke()
	})
	mux.Handle("/", served(h.pages))
	pagetest.Run(t, pagetest.Config{Handler: mux, Page: "/machines?at=/house/room", Module: "machines/counts_test.js"})
}
