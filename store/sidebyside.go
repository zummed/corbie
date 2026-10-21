package store

import (
	"context"
	"io"
	"sync"
)

// SideBySide is how many requests one Bounded store has open at once by
// default: the store of one controller pass, one start or one definition
// made while a machine runs; and how many of the values a listing names a
// watcher or a follower reads at once.
const SideBySide = 16

// Each does n things side by side, at most limit at a time, and gives the
// first error. Nothing more is begun once one has failed or the context has
// ended. One of them may itself do things side by side: what bounds the
// requests is a store from Bounded.
func Each(ctx context.Context, limit, n int, do func(i int) error) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	slots := make(chan struct{}, max(limit, 1))
	for i := range n {
		mu.Lock()
		stop := first != nil
		mu.Unlock()
		if stop || ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			if err := do(i); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if first == nil {
		first = ctx.Err()
	}
	return first
}

// Bounded gives a store that has at most n requests open at once, wherever
// they come from. A request waits for a place, and one whose context has
// ended is not made.
func Bounded(s Store, n int) Store {
	return bounded{s, make(chan struct{}, max(n, 1))}
}

type bounded struct {
	Store
	places chan struct{}
}

func (b bounded) take(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case b.places <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b bounded) free() { <-b.places }

func (b bounded) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := b.take(ctx); err != nil {
		return nil, None, err
	}
	defer b.free()
	return b.Store.Read(ctx, name)
}

func (b bounded) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := b.take(ctx); err != nil {
		return nil, err
	}
	defer b.free()
	return b.Store.ReadPart(ctx, name, from, length)
}

func (b bounded) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	if err := b.take(ctx); err != nil {
		return false, err
	}
	defer b.free()
	return b.Store.Write(ctx, name, value, how)
}

func (b bounded) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := b.take(ctx); err != nil {
		return false, err
	}
	defer b.free()
	return b.Store.WriteFrom(ctx, name, r)
}

func (b bounded) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := b.take(ctx); err != nil {
		return false, err
	}
	defer b.free()
	return b.Store.Copy(ctx, from, to)
}

func (b bounded) Delete(ctx context.Context, name string, how How) (bool, error) {
	if err := b.take(ctx); err != nil {
		return false, err
	}
	defer b.free()
	return b.Store.Delete(ctx, name, how)
}

func (b bounded) List(ctx context.Context, under string) ([]Listed, error) {
	if err := b.take(ctx); err != nil {
		return nil, err
	}
	defer b.free()
	return b.Store.List(ctx, under)
}

func (b bounded) First(ctx context.Context, under string, n int) ([]Listed, error) {
	if err := b.take(ctx); err != nil {
		return nil, err
	}
	defer b.free()
	return b.Store.First(ctx, under, n)
}
