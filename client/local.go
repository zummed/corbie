package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// Local is every role in one process with the store in memory: a
// controller, a runner and a client that trust each other. It is the same
// loop as on a computer, and each role works under its own row of
// the table of who may touch what. Nothing is set up beforehand and
// nothing is left behind.
type Local struct {
	*Client
	Memory     *store.Memory
	Controller *controller.Controller
	Runner     *runner.Runner
	stop       context.CancelFunc
	done       chan struct{}
}

// NewLocal starts a controller and a runner in this process. The ceiling
// is what jobs may do on this computer. report is told of each pass of a
// role that failed.
func NewLocal(ctx context.Context, ceiling permission.Layer, report func(role string, err error)) (*Local, error) {
	mem := store.NewMemory()
	ctl, err1 := identity.New("controller", identity.Controller)
	run, err2 := identity.New("runner", identity.Runner)
	cli, err3 := identity.New("client", identity.Client)
	for _, err := range []error{err1, err2, err3} {
		if err != nil {
			return nil, err
		}
	}
	as := func(kind store.Kind, name string) store.Store {
		return mem.As(store.Party{Kind: kind, Name: name, Controller: ctl.Name})
	}
	ctlStore := as(store.Controller, ctl.Name)
	if err := store.SetUp(ctx, ctlStore); err != nil {
		return nil, err
	}
	// The client may submit a tree at any prefix.
	clientCeiling := ceiling
	clientCeiling.Allow = append(slices.Clone(ceiling.Allow), permission.At+":/**")
	for _, t := range []identity.Trust{{Card: run.Card}, {Card: cli.Card, Ceiling: clientCeiling}} {
		if err := identity.WriteTrust(ctx, ctlStore, ctl, t); err != nil {
			return nil, err
		}
	}
	l := &Local{Memory: mem, done: make(chan struct{})}
	l.Controller = controller.New(ctl, ctlStore)
	l.Runner = runner.New(run, runner.Served{Controller: ctl.Card, Ceiling: ceiling, Store: as(store.Runner, run.Name)})
	l.Client = New(cli, ctl.Card, as(store.Client, cli.Name))

	ctx, l.stop = context.WithCancel(ctx)
	inbox, stopInbox := ctlStore.Notice(store.Inbox(ctl.Name))
	jobs, stopJobs := ctlStore.Notice(store.Inbox(run.Name))
	go func() {
		defer close(l.done)
		defer stopInbox()
		defer stopJobs()
		role.Run(ctx, time.Now, report,
			role.Role{Name: "controller", Pass: l.Controller.Pass, Interval: LocalInterval, Notice: inbox},
			role.Role{Name: "runner", Pass: l.Runner.Pass, Interval: LocalInterval, Notice: jobs})
	}()
	return l, nil
}

// LocalInterval is how long a role in one process sleeps when nothing is
// due and nothing wakes it.
var LocalInterval = time.Second

// SetSecret sets a secret at a location, as the controller's owner does.
func (l *Local) SetSecret(ctx context.Context, at, name, value string) error {
	return secret.Set(ctx, l.Controller.Store, l.Controller.ID, at, name, value)
}

// SetValue sets a value at a location, as the controller's owner does.
func (l *Local) SetValue(ctx context.Context, at, name, value string) error {
	return secret.SetValue(ctx, l.Controller.Store, l.Controller.ID, at, name, value)
}

// Run submits a definition as a tree of one file, at a prefix of its own
// that begins with the machine's name, and gives the run RunTree gives.
func (l *Local) Run(ctx context.Context, draft *machine.Draft, start Start) (string, error) {
	random := make([]byte, 3)
	rand.Read(random)
	at := "/" + draft.Definition.Name + "-" + hex.EncodeToString(random)
	return l.RunTree(ctx, &Tree{Prefix: at, Machines: []*Machine{{Path: draft.Definition.Name + ".yaml", Location: at, Draft: draft}}}, start)
}

// RunTree submits a tree of one file and gives the run at its location: the
// one the submit starts for a singleton, and otherwise one started there
// with what start carries. A start that carries anything but its life to a
// singleton is an error.
func (l *Local) RunTree(ctx context.Context, tree *Tree, start Start) (string, error) {
	message, err := l.Submit(ctx, tree, start.Life)
	if err != nil {
		return "", err
	}
	receipt, err := l.WaitReceipt(ctx, message, start.Life, LocalInterval)
	switch {
	case err != nil:
		return "", err
	case !receipt.Taken:
		return "", fmt.Errorf("%w: the submit was refused: %s", ErrStart, receipt.Refused)
	}
	for _, done := range receipt.Tree.Locations {
		if done.Run == "" {
			continue
		}
		carried := start
		carried.Life = 0
		if !reflect.DeepEqual(carried, Start{}) {
			return "", fmt.Errorf("%w: the submit started the singleton at %s, and a start carries nothing to it", ErrStart, done.Location)
		}
		// The singleton has nothing to read until its start is taken.
		return done.Run, l.Started(ctx, done.Run, LocalInterval)
	}
	start.At = tree.Machines[0].Location
	return l.Start(ctx, start)
}

// Close stops the roles and waits for them.
func (l *Local) Close() {
	l.stop()
	<-l.done
}
