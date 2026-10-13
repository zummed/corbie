package client

import (
	"context"
	"time"

	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
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
	for _, t := range []identity.Trust{{Card: run.Card}, {Card: cli.Card, Ceiling: ceiling}} {
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
		role.Run(ctx, LocalInterval, time.Now, report,
			role.Role{Name: "controller", Pass: l.Controller.Pass, Notice: inbox},
			role.Role{Name: "runner", Pass: l.Runner.Pass, Notice: jobs})
	}()
	return l, nil
}

// LocalInterval is how long a role in one process sleeps when nothing is
// due and nothing wakes it.
var LocalInterval = time.Second

// Close stops the roles and waits for them.
func (l *Local) Close() {
	l.stop()
	<-l.done
}
