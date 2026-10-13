package client

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// A Run is a run as it is shown, wherever it is shown: the same shape and
// the same words at the command line, over HTTP and to an application.
type Run struct {
	ID      string `json:"id"`
	Machine string `json:"machine,omitzero"`
	Type    string `json:"type,omitzero"`
	// Status is waiting (to start), running, done, failed or cancelled.
	Status string `json:"status"`
	// Failure says why a failed run failed: error, timeout or denied.
	Failure string `json:"failure,omitzero"`
	Error   string `json:"error,omitzero"`
	// A denial says what was refused, which layer refused it and what
	// would fix it.
	Refused string `json:"refused,omitzero"`
	Layer   string `json:"layer,omitzero"`
	Fix     string `json:"fix,omitzero"`

	Output any `json:"output,omitzero"`
	// Used lists the permissions the run used: those of every job in it
	// and below it.
	Used []string `json:"used,omitzero"`

	Step    int               `json:"step,omitzero"`
	Started time.Time         `json:"started,omitzero"`
	Ended   time.Time         `json:"ended,omitzero"`
	Parent  string            `json:"parent,omitzero"`
	Jobs    int               `json:"jobs,omitzero"`
	Timers  int               `json:"timers,omitzero"`
	Keys    []string          `json:"keys,omitzero"`
	Labels  map[string]string `json:"labels,omitzero"`
}

// Ended says whether the run has ended.
func (r *Run) HasEnded() bool { return r.Status != instance.Waiting && r.Status != instance.Running }

// ErrNoRun is a run that does not exist: it was never started, or it is
// past keeping and gone.
var ErrNoRun = errors.New("there is no such run")

// ErrNeverTaken is a run whose start stopped being good before a controller
// took it up.
var ErrNeverTaken = fmt.Errorf("%w: its start was never taken up", ErrNoRun)

// ErrNoRecord is a run with nothing in its record to read yet: it is
// waiting to start, or its start was refused.
var ErrNoRecord = errors.New("the run has no record to read")

func runOf(head *instance.Head) *Run {
	return &Run{ID: head.Name, Machine: head.Machine, Type: head.Type, Status: head.Status, Failure: head.Failure,
		Step: head.Step, Started: head.Started, Ended: head.Ended, Parent: head.Parent, Jobs: head.Jobs,
		Timers: head.Timers, Keys: head.Keys, Labels: head.Labels}
}

// mine says whether the client may read what is inside a run.
func (c *Client) mine(run string) bool {
	return c.ID != nil && store.InstanceClient(run) == c.ID.Name
}

// Show gives one run: where it stands and, once it has ended, how. It is
// shown from its record: the head may be a moment behind, or not there yet.
func (c *Client) Show(ctx context.Context, id string) (*Run, error) {
	if err := store.CheckInstance(id); err != nil {
		return nil, err
	}
	head, _, err := instance.ReadHead(ctx, c.Store, c.Controller, id)
	if err != nil {
		return nil, err
	}
	if !c.mine(id) {
		if head == nil {
			return nil, ErrNoRun
		}
		return runOf(head), nil
	}
	run, from := &Run{ID: id}, 1
	if head != nil {
		run, from = runOf(head), head.Step
	}
	latest, err := instance.Latest(ctx, c.Store, c.Controller, c.ID, id, from)
	switch {
	case head != nil && errors.Is(err, identity.ErrNotForUs):
		return run, nil // sealed to a key this client no longer has
	case err != nil:
		return nil, err
	case latest == nil && head == nil:
		early, err := c.unstarted(ctx, id)
		if early != nil || err != nil {
			return early, err
		}
		// Its start was taken. The first step is written before the receipt
		// that says so: a start taken between the look at the record and
		// the look at the receipt has its step by now, and one that has
		// none is past keeping and gone.
		if latest, err = instance.Latest(ctx, c.Store, c.Controller, c.ID, id, from); err != nil {
			return nil, err
		}
		if latest == nil {
			return nil, ErrNoRun
		}
	case latest == nil:
		return run, nil
	}
	st := &latest.State
	run.Machine, run.Type, run.Started, run.Parent, run.Labels = st.Machine, st.Type, st.Started, st.Above.Parent, st.Labels
	// A blob in the output is shown as its type, its size and the path to
	// ask for it by: its key is left out here, where the one shape is made.
	run.Status, run.Step, run.Output, run.Used, run.Ended = st.Status, latest.Number, blob.Show(st.Output), st.Used, st.Ended
	run.Jobs, run.Timers, run.Keys = 0, len(st.Timers), nil
	for _, r := range st.Jobs {
		if r.Runner != "" {
			run.Jobs++
		}
	}
	for _, key := range st.Keys {
		if !slices.Contains(run.Keys, key.Name) {
			run.Keys = append(run.Keys, key.Name)
		}
	}
	slices.Sort(run.Keys)
	if st.Error != nil {
		run.Error, run.Refused, run.Layer, run.Fix = st.Error.Error, st.Error.Refused, st.Error.Layer, st.Error.Fix
		if st.Status == instance.Failed {
			run.Failure = st.Error.Status
		}
	}
	return run, nil
}

// unstarted says where a run with no head and no step stands: refused
// before it started, waiting to start, never taken up, or no run at all.
// For one whose start was taken it gives nothing: that run has a record.
func (c *Client) unstarted(ctx context.Context, id string) (*Run, error) {
	receipt, err := c.Receipt(ctx, startMessage(id))
	if err != nil {
		return nil, err
	}
	if receipt != nil && !receipt.Taken {
		run := &Run{ID: id, Status: instance.Failed, Failure: machine.StatusError, Error: receipt.Refused}
		if len(receipt.Problems) > 0 {
			run.Error += ":\n" + receipt.Problems.Error()
		}
		return run, nil
	}
	if receipt != nil {
		return nil, nil
	}
	// A start's definition is written into the run's record before the
	// start is sent, so a name with nothing in its record was never sent.
	uploaded, err := c.Store.First(ctx, store.Blobs(c.ctl(), id), 1)
	if err != nil {
		return nil, err
	}
	if len(uploaded) == 0 {
		return nil, ErrNoRun
	}
	// The store does not let a client see its own start, so how long the
	// start is good for is reckoned from the time in the run's name. A name
	// dated further ahead than clocks may differ is no start of this
	// client's.
	named, _ := store.InstanceTime(id)
	if age := c.Now().Sub(named); age > instance.MessageLife+2*identity.ClockAllowance || age < -identity.ClockAllowance {
		return nil, ErrNeverTaken
	}
	return &Run{ID: id, Status: instance.Waiting}, nil
}

// Wait stays until a run has ended and gives it. It looks again when this
// process writes to the store, and otherwise at the interval; a look that
// cannot reach the store is made again. A wait that is cut short, by the
// context's limit or by its caller going away, gives the run as it stands.
func (c *Client) Wait(ctx context.Context, id string, interval time.Duration) (*Run, error) {
	notice, stop := c.Store.Notice(c.ctl())
	defer stop()
	for {
		run, err := c.Show(ctx, id)
		switch {
		case ctx.Err() != nil:
			return c.Show(context.WithoutCancel(ctx), id)
		case err == nil && run.HasEnded():
			return run, nil
		case err != nil && !errors.Is(err, store.ErrUnreachable):
			return nil, err
		}
		if !pause(ctx, interval, notice) {
			return c.Show(context.WithoutCancel(ctx), id)
		}
	}
}

// pause stays until something was written, the interval has passed, or the
// context has ended, which it reports as false.
func pause(ctx context.Context, interval time.Duration, notice <-chan struct{}) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-notice:
	case <-timer.C:
	}
	return true
}

// current gives the latest step of a run this client started. A run with
// no step is no run, or has no record to read yet.
func (c *Client) current(ctx context.Context, id string) (*instance.Step, error) {
	if err := c.identity(); err != nil {
		return nil, err
	}
	if err := store.CheckInstance(id); err != nil {
		return nil, err
	}
	if !c.mine(id) {
		return nil, fmt.Errorf("the run %s was started by another client, and only the client that started a run can read its record", id)
	}
	latest, err := instance.Current(ctx, c.Store, c.Controller, c.ID, id)
	if err != nil || latest != nil {
		return latest, err
	}
	run, err := c.Show(ctx, id)
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: it is %s", ErrNoRecord, run.Status)
}

// Keys gives the keys a run is waiting on, each name with its value. The
// value is the whole right to answer; it is in the record, which only the
// client that started the run can read.
func (c *Client) Keys(ctx context.Context, id string) (map[string]string, error) {
	latest, err := c.current(ctx, id)
	if err != nil {
		return nil, err
	}
	keys := map[string]string{}
	for _, key := range latest.State.Keys {
		keys[key.Name] = key.Value
	}
	return keys, nil
}
