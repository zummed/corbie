//go:build !unix

package store

import (
	"context"
	"sync"
)

var turns sync.Map

// lock keeps the writers and listings of this process apart. Where the
// system has no lock between processes, a directory is for one process.
func (f *directory) lock(ctx context.Context, alone bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, _ := turns.LoadOrStore(f.root, make(chan struct{}, 1))
	turn := t.(chan struct{})
	select {
	case turn <- struct{}{}:
		return func() { <-turn }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// syncDir does nothing where the system cannot sync a directory.
func syncDir(dir string) error { return nil }
