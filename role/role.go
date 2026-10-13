// Package role is how a controller or a runner goes about its work,
// wherever it is hosted: in passes.
package role

import (
	"context"
	"sync"
	"time"
)

// A Pass looks at the store, does everything that is owed, and stops. It
// says when the next thing is due, if it knows of one. Nothing is carried
// in memory from one pass to the next.
type Pass func(ctx context.Context) (next time.Time, err error)

// A Role is one role of a process: its pass, and the notice its store
// gives when something it reads is written by this same process.
type Role struct {
	Name   string
	Pass   Pass
	Notice <-chan struct{}
}

// Run runs each role's passes in a loop until the context ends: a pass,
// then a sleep until the next thing is due or for the interval, whichever
// is sooner. A role is woken at once when another role in the process
// writes something it reads; that changes when a pass happens and never
// what a pass does.
//
// report is told of each pass that failed. Such a pass has failed nothing:
// what was waiting is still waiting for the next one.
func Run(ctx context.Context, interval time.Duration, now func() time.Time, report func(role string, err error), roles ...Role) {
	var wg sync.WaitGroup
	for _, r := range roles {
		wg.Go(func() {
			for ctx.Err() == nil {
				next, err := r.Pass(ctx)
				if err != nil && ctx.Err() == nil && report != nil {
					report(r.Name, err)
				}
				wait := interval
				if !next.IsZero() {
					if until := next.Sub(now()); until < wait {
						wait = max(until, 0)
					}
				}
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
				case <-timer.C:
				case <-r.Notice:
				}
				timer.Stop()
			}
		})
	}
	wg.Wait()
}

// Once runs one pass for a host that calls for passes itself and kills a
// copy at a time it gives. The pass is told to stop taking up new work a
// margin before that time, or halfway to it where less than two margins
// are left, and nothing of it goes on after it has answered.
func Once(ctx context.Context, until time.Time, margin time.Duration, pass Pass) (time.Time, error) {
	left := max(time.Until(until), 0)
	ctx, cancel := context.WithDeadline(ctx, until.Add(-min(margin, left/2)))
	defer cancel()
	return pass(ctx)
}
