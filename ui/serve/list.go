package serve

import (
	"context"
	"encoding/json/v2"
	"sync"
	"time"

	"github.com/zummed/corbie/client"
)

// A list is every run whose status the UI may read, as it was last read:
// once at every interval, for every page and scraper alike. A run that has
// left the store after its keep has left the list.
type list struct {
	mu      sync.Mutex
	runs    []*client.Run
	err     error
	seen    string
	changed chan struct{}
	poke    chan struct{}
}

// listed is the listing the UI follows: every run, ended or not.
var listed = client.Filter{Finished: true}

func newList() *list {
	return &list{changed: make(chan struct{}), poke: make(chan struct{}, 1)}
}

// A lister lists runs, as Corbie's client package does.
type lister interface {
	List(ctx context.Context, filter client.Filter) ([]*client.Run, error)
}

// follow reads the list at every interval, and at once after an action,
// until the context ends.
func (l *list) follow(ctx context.Context, c lister, interval time.Duration) {
	for {
		runs, err := c.List(ctx, listed)
		if ctx.Err() != nil {
			return
		}
		l.set(runs, err)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-l.poke:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// set keeps what a read found, and tells everyone waiting on a change when
// it differs from what was kept. A read that fails keeps the runs.
func (l *list) set(runs []*client.Run, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := l.seen
	if err == nil {
		data, _ := json.Marshal(runs)
		seen = string(data)
		l.runs = runs
	}
	same := seen == l.seen && (err == nil) == (l.err == nil)
	l.seen, l.err = seen, err
	if !same {
		close(l.changed)
		l.changed = make(chan struct{})
	}
}

// Runs gives the runs as last read, and the error of the last read.
func (l *list) Runs() ([]*client.Run, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.runs, l.err
}

// Changed gives a channel that is closed at the next change.
func (l *list) Changed() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.changed
}

// Poke has the list read again at once.
func (l *list) Poke() {
	select {
	case l.poke <- struct{}{}:
	default:
	}
}
