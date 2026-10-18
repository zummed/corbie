package client_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

// arranged is a controller's store with no role running: a test writes
// what a controller and a runner would have written, in the order and at
// the moment it chooses, and a client reads it.
type arranged struct {
	*client.Client
	t        *testing.T
	owner    store.Store
	ctl, run *identity.Identity
	hooked   *hooked
}

// hooked is a client's store that tells a test of each look before it is
// made, so that a test can change the store, or fail the look, between two
// looks of one reading.
type hooked struct {
	store.Store
	mu sync.Mutex
	// before is told of each read and each listing: "read" or "list", and
	// the name. An error it gives is the look's answer.
	before func(act, name string) error
	// listed may change the names a listing gives.
	listed func(under string, names []string) []string
	// silent gives no notices, as a bucket does and as a store does for a
	// watcher in another process than the writer.
	silent bool
}

func (h *hooked) look(act, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.before == nil {
		return nil
	}
	return h.before(act, name)
}

func (h *hooked) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if err := h.look("read", name); err != nil {
		return nil, store.None, err
	}
	return h.Store.Read(ctx, name)
}

func (h *hooked) List(ctx context.Context, under string) ([]string, error) {
	if err := h.look("list", under); err != nil {
		return nil, err
	}
	names, err := h.Store.List(ctx, under)
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil && h.listed != nil {
		names = h.listed(under, names)
	}
	return names, err
}

func (h *hooked) First(ctx context.Context, under string, n int) ([]string, error) {
	if err := h.look("list", under); err != nil {
		return nil, err
	}
	return h.Store.First(ctx, under, n)
}

func (h *hooked) Notice(under string) (<-chan struct{}, func()) {
	if h.silent {
		return make(chan struct{}), func() {}
	}
	return h.Store.Notice(under)
}

func arrange(t *testing.T) *arranged {
	t.Helper()
	mem := store.NewMemory()
	a := &arranged{t: t, owner: mem.As(store.Party{Kind: store.Owner})}
	var cli *identity.Identity
	for _, made := range []struct {
		into **identity.Identity
		name string
		kind identity.Kind
	}{{&a.ctl, "controller", identity.Controller}, {&a.run, "runner", identity.Runner}, {&cli, "client", identity.Client}} {
		id, err := identity.New(made.name, made.kind)
		if err != nil {
			t.Fatal(err)
		}
		*made.into = id
	}
	if err := store.SetUp(ctx, a.owner); err != nil {
		t.Fatal(err)
	}
	a.hooked = &hooked{Store: mem.As(store.Party{Kind: store.Client, Name: "client", Controller: "controller"})}
	a.Client = client.New(cli, a.ctl.Card, a.hooked)
	return a
}

// named gives the name of a run of the client's, made a moment ago.
func (a *arranged) named(tail string) string {
	return "client/" + store.Stamp(time.Now().Add(-time.Second)) + "-" + tail
}

func (a *arranged) put(name string, value []byte, err error) {
	a.t.Helper()
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := a.owner.Write(ctx, name, value, store.Overwrite); err != nil {
		a.t.Fatal(err)
	}
}

// head writes a run's head as the controller would: among the running, or
// for a run that has ended among the finished. Each label is name=value.
func (a *arranged) head(id, status string, step int, labels ...string) {
	a.t.Helper()
	at := store.RunningHead("controller", id)
	if status != instance.Running {
		at = store.FinishedHead("controller", id)
	}
	head := &instance.Head{Name: id, Machine: "arranged", Type: "flow", Status: status, Step: step, Started: time.Now()}
	for _, label := range labels {
		name, value, _ := strings.Cut(label, "=")
		if head.Labels == nil {
			head.Labels = map[string]string{}
		}
		head.Labels[name] = value
	}
	value, err := a.ctl.Write(at, time.Now(), time.Time{}, head)
	a.put(at, value, err)
}

// step writes a step entry of a run as the controller would. A status that
// is not running ends the run there.
func (a *arranged) step(id string, number int, status string, asks ...instance.Ask) {
	a.t.Helper()
	at := store.Step("controller", id, number)
	step := &instance.Step{Number: number, Time: time.Now(), Asks: asks,
		State: instance.State{Machine: "arranged", Type: "flow", Status: status, Started: time.Now()}}
	if status == instance.Done {
		step.State.Output = map[string]any{"from": id}
	}
	value, err := a.ctl.Write(at, time.Now(), time.Time{}, step, a.ctl.Card, a.ID.Card)
	a.put(at, value, err)
}

// job is what a step asks for when it sends a job to the runner.
func (a *arranged) job(name string) instance.Ask {
	return instance.Ask{Kind: instance.AskScript, Name: name, Runner: "runner", Card: a.run.Card}
}

// child is what a step asks for when it starts a child.
func child(name, id string) instance.Ask {
	return instance.Ask{Kind: instance.AskChild, Name: name, Child: id}
}

// emit writes one emit entry of a job as its runner would.
func (a *arranged) emit(id, job string, number int, data any) {
	a.t.Helper()
	at := store.Emit("controller", id, job, number)
	entry := &instance.Emit{Job: job, Number: number, Values: []instance.Emitted{{Name: "note", Data: data, At: time.Now()}}}
	value, err := a.run.Write(at, time.Now(), time.Time{}, entry, a.ctl.Card, a.ID.Card)
	a.put(at, value, err)
}

// follow follows a run to its end and gives what it was told, each entry as
// "<run's tail> step <n>" or "<run's tail> emit <data>".
func (a *arranged) follow(id string) (*client.Run, []string) {
	a.t.Helper()
	within, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var told []string
	run, err := a.Follow(within, id, time.Millisecond, func(e client.Entry) {
		tail := e.ID[strings.LastIndexByte(e.ID, '-')+1:]
		if e.Step > 0 {
			told = append(told, tail+" step "+strings.TrimLeft(pad8(e.Step), "0"))
		} else {
			told = append(told, tail+" emit "+jsonOf(e.Data))
		}
	})
	if err != nil || within.Err() != nil {
		a.t.Fatalf("following %s did not end by itself: %v (told %v)", id, err, told)
	}
	return run, told
}

// plainOf gives a value as the data it is when printed as JSON.
func plainOf(t *testing.T, v any) (data any) {
	t.Helper()
	if err := json.Unmarshal([]byte(jsonOf(v)), &data); err != nil {
		t.Fatal(err)
	}
	return data
}
