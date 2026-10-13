package controller

import (
	"context"
	"io"

	"github.com/zummed/corbie/store"
)

// bounded is a store that has only so many requests open at once. A pass
// works on many instances side by side, and for each on the many things a
// step asks for: the bound is on the requests, wherever they come from. A
// request waits for a place, and one whose pass is over is not made.
type bounded struct {
	store.Store
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

func (b bounded) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if err := b.take(ctx); err != nil {
		return nil, store.None, err
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

func (b bounded) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
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

func (b bounded) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	if err := b.take(ctx); err != nil {
		return false, err
	}
	defer b.free()
	return b.Store.Delete(ctx, name, how)
}

func (b bounded) List(ctx context.Context, under string) ([]string, error) {
	if err := b.take(ctx); err != nil {
		return nil, err
	}
	defer b.free()
	return b.Store.List(ctx, under)
}

func (b bounded) First(ctx context.Context, under string, n int) ([]string, error) {
	if err := b.take(ctx); err != nil {
		return nil, err
	}
	defer b.free()
	return b.Store.First(ctx, under, n)
}
