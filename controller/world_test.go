package controller_test

import (
	"context"
	"fmt"
	"os"
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
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/secret"
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
	// placed counts the trees submitted, so that each has a prefix of its own.
	placed int
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

// submit submits a definition as a tree of one file, at a prefix of its
// own, and gives the run there: the one the submit starts for a singleton,
// and otherwise one started at the location with the input.
func (w *world) submit(src string, files map[string]string, input any) string {
	w.t.Helper()
	at, run := w.place(w.client, src, files)
	if run != "" {
		return run
	}
	return w.startAt(w.client, at, client.Start{Input: input})
}

// place has a client submit a definition as a tree of one file, at a prefix
// no other test of the world uses, and runs the controller until the submit
// is taken. It gives the file's location, which is the prefix, and the run
// the submit started there, if it started one.
func (w *world) place(c *client.Client, src string, files map[string]string) (at, run string) {
	w.t.Helper()
	draft := w.draft(src, files)
	w.mu.Lock()
	w.placed++
	at = fmt.Sprintf("/%s-%d", draft.Definition.Name, w.placed)
	w.mu.Unlock()
	message, err := c.Submit(ctx, &client.Tree{Prefix: at, Machines: []*client.Machine{{Path: draft.Definition.Name + ".yaml", Location: at, Draft: draft}}}, 0)
	if err != nil {
		w.t.Fatal(err)
	}
	for range 10 {
		w.pass()
		r, err := c.Receipt(ctx, message)
		switch {
		case err != nil:
			w.t.Fatal(err)
		case r == nil:
			continue
		case !r.Taken:
			w.t.Fatalf("the submit of %s was refused: %s", at, r.Refused)
		}
		for _, done := range r.Tree.Locations {
			run = done.Run
		}
		return at, run
	}
	w.t.Fatalf("the submit of %s has no receipt", at)
	return "", ""
}

// startAt has a client start a run at a location, and gives its name.
func (w *world) startAt(c *client.Client, at string, start client.Start) string {
	w.t.Helper()
	start.At = at
	run, err := c.Start(ctx, nil, start)
	if err != nil {
		w.t.Fatal(err)
	}
	return run
}

// secret sets a secret at a location, as the controller's owner does.
func (w *world) secret(name, value, at string) {
	w.t.Helper()
	if err := secret.Set(ctx, w.ctrl.Store, w.ctl, at, name, value); err != nil {
		w.t.Fatal(err)
	}
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
	names, err := store.Names(w.owner.List(ctx, under))
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

// The test binary stands in for the process of a step of a machine made
// while running.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	if len(os.Args) > 2 && os.Args[1] == stepThatHangs {
		hangs(os.Args[2])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == stepThatSaysTooMuch {
		saysTooMuch()
		return
	}
	if len(os.Args) > 2 && os.Args[1] == stepThatLies {
		lying(os.Args[2], os.Args[3:])
		return
	}
	os.Exit(m.Run())
}
