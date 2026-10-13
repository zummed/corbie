package controller_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/statechart"
)

var ctx = context.Background()

// world is a controller and a client on a memory store, with a clock the
// test moves by hand.
type world struct {
	t      *testing.T
	mem    *store.Memory
	owner  store.Store
	ctl    *identity.Identity
	c1     *identity.Identity
	ctrl   *controller.Controller
	client *client.Client

	mu     sync.Mutex
	now    time.Time
	writes int
	ops    []store.Op
}

func newWorld(t *testing.T) *world {
	w := &world{t: t, mem: store.NewMemory(), now: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	w.mem.Note = func(op store.Op) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if op.Act != store.ActRead && op.Act != store.ActList && op.Done {
			w.writes++
		}
		w.ops = append(w.ops, op)
	}
	w.owner = w.mem.As(store.Party{Kind: store.Owner})
	w.ctl = w.party("ctl", identity.Controller)
	w.c1 = w.party("c1", identity.Client)
	ctlStore := w.mem.As(store.Party{Kind: store.Controller, Name: "ctl"})
	if err := store.SetUp(ctx, ctlStore); err != nil {
		t.Fatal(err)
	}
	w.ctrl = controller.New(w.ctl, ctlStore)
	w.ctrl.Now = w.clock
	w.enrol(w.c1, permission.Layer{Allow: []string{"all"}}, nil)
	w.client = w.clientOf(w.c1)
	return w
}

func (w *world) clock() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

// tick moves the clock on.
func (w *world) tick(d time.Duration) {
	w.mu.Lock()
	w.now = w.now.Add(d)
	w.mu.Unlock()
}

func (w *world) party(name string, kind identity.Kind) *identity.Identity {
	id, err := identity.New(name, kind)
	if err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *world) enrol(id *identity.Identity, ceiling permission.Layer, tags []string) {
	if err := identity.WriteTrust(ctx, w.ctrl.Store, w.ctl, identity.Trust{Card: id.Card, Ceiling: ceiling, Tags: tags}); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) clientOf(id *identity.Identity) *client.Client {
	c := client.New(id, w.ctl.Card, w.mem.As(store.Party{Kind: store.Client, Name: id.Name, Controller: "ctl"}))
	c.Now = w.clock
	return c
}

// draft fills out a definition written in the test, with its files.
func (w *world) draft(src string, files map[string]string) *machine.Draft {
	w.t.Helper()
	draft, err := machine.Parse([]byte(dedent(src)), func(path string) ([]byte, error) {
		if contents, ok := files[path]; ok {
			return []byte(dedent(contents)), nil
		}
		return nil, fmt.Errorf("no such file")
	})
	if err != nil {
		w.t.Fatalf("the definition does not pass:\n%v", err)
	}
	return draft
}

// submit starts a run and gives its name.
func (w *world) submit(src string, files map[string]string, input any) string {
	w.t.Helper()
	run, err := w.client.Start(ctx, w.draft(src, files), client.Start{Input: input})
	if err != nil {
		w.t.Fatal(err)
	}
	return run
}

// pass runs one pass of the controller.
func (w *world) pass() time.Time {
	w.t.Helper()
	next, err := w.ctrl.Pass(ctx)
	if err != nil {
		w.t.Fatal("pass:", err)
	}
	return next
}

// settle runs passes until one changes nothing.
func (w *world) settle() {
	w.t.Helper()
	for range 200 {
		w.mu.Lock()
		before := w.writes
		w.mu.Unlock()
		w.pass()
		w.mu.Lock()
		same := w.writes == before
		w.mu.Unlock()
		if same {
			return
		}
	}
	w.t.Fatal("the controller does not come to rest")
}

func (w *world) head(run string) *instance.Head {
	w.t.Helper()
	head, _, err := instance.ReadHead(ctx, w.owner, w.ctl.Card, run)
	if err != nil {
		w.t.Fatal(err)
	}
	return head
}

// latest reads a run's latest step, as the controller.
func (w *world) latest(run string) *instance.Step {
	w.t.Helper()
	step, err := instance.Latest(ctx, w.owner, w.ctl.Card, w.ctl, run, 1)
	if err != nil {
		w.t.Fatal(err)
	}
	return step
}

// steps reads every step of a run.
func (w *world) steps(run string) []*instance.Step {
	w.t.Helper()
	var all []*instance.Step
	for n := 1; ; n++ {
		step, err := instance.ReadStep(ctx, w.owner, w.ctl.Card, w.ctl, run, n)
		if err != nil {
			w.t.Fatal(err)
		}
		if step == nil {
			return all
		}
		all = append(all, step)
	}
}

func (w *world) list(under string) []string {
	names, err := w.owner.List(ctx, under)
	if err != nil {
		w.t.Fatal(err)
	}
	return names
}

func (w *world) receipt(message string) *instance.Receipt {
	w.t.Helper()
	r, err := w.client.Receipt(ctx, message)
	if err != nil {
		w.t.Fatal(err)
	}
	return r
}

// dedent takes the common leading tabs off a text written inside a test.
func dedent(text string) string {
	lines := strings.Split(strings.Trim(text, "\n"), "\n")
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, "\t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for i, line := range lines {
		if indent > 0 && len(line) >= indent {
			lines[i] = line[indent:]
		}
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\t", "  ") + "\n"
}

// wakes lists the wakes that are owed to machines, leaving out the
// controller's own for housekeeping.
func (w *world) wakes() []string {
	var wakes []string
	for _, name := range w.list(store.Wakes("ctl")) {
		if !strings.HasSuffix(name, "/trim") && !strings.HasSuffix(name, "/remove") && !strings.HasSuffix(name, "/house/keep") {
			wakes = append(wakes, name)
		}
	}
	return wakes
}

// afresh puts a controller that has never run before, with only its key
// and a store, in the place of the one there is.
func (w *world) afresh(s store.Store) {
	w.ctrl = controller.New(w.ctl, s)
	w.ctrl.Now = w.clock
}

// noted gives what has been done to the store since it was last asked, or
// since the world began.
func (w *world) noted() []store.Op {
	w.mu.Lock()
	defer w.mu.Unlock()
	ops := w.ops
	w.ops = nil
	return ops
}

// of gives the names in the store that belong to an instance: its heads,
// its record, and its wakes.
func (w *world) of(inst string) []string {
	var names []string
	for _, name := range w.list("ctl") {
		if strings.Contains(name, "/"+inst+"/") || strings.HasSuffix(name, "/"+inst) {
			names = append(names, name)
		}
	}
	return names
}
