package controller_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// open counts how many requests of any kind a store has open at once.
type open struct {
	store.Store
	now, most atomic.Int64
}

func (o *open) begin() func() {
	n := o.now.Add(1)
	for {
		most := o.most.Load()
		if n <= most || o.most.CompareAndSwap(most, n) {
			break
		}
	}
	time.Sleep(50 * time.Microsecond) // long enough for requests made side by side to meet
	return func() { o.now.Add(-1) }
}

func (o *open) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	defer o.begin()()
	return o.Store.Read(ctx, name)
}

func (o *open) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	defer o.begin()()
	return o.Store.ReadPart(ctx, name, from, length)
}

func (o *open) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	defer o.begin()()
	return o.Store.Write(ctx, name, value, how)
}

func (o *open) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	defer o.begin()()
	return o.Store.WriteFrom(ctx, name, r)
}

func (o *open) Copy(ctx context.Context, from, to string) (bool, error) {
	defer o.begin()()
	return o.Store.Copy(ctx, from, to)
}

func (o *open) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	defer o.begin()()
	return o.Store.Delete(ctx, name, how)
}

func (o *open) List(ctx context.Context, under string) ([]string, error) {
	defer o.begin()()
	return o.Store.List(ctx, under)
}

func (o *open) First(ctx context.Context, under string, n int) ([]string, error) {
	defer o.begin()()
	return o.Store.First(ctx, under, n)
}

// wide is a chart whose first step asks for a number of jobs at once.
func wide(jobs int) string {
	var runs strings.Builder
	for i := range jobs {
		fmt.Fprintf(&runs, "\t      r%d: ./work.js\n", i)
	}
	return "\tstatechart: wide\n\tinitial: s\n\tstates:\n\t  s:\n\t    jobs:\n" + runs.String()
}

func Test_role_pass_bounded_side_by_side(t *testing.T) {
	w := newWorld(t)
	w.enrol(w.party("r1", identity.Runner), permission.Layer{}, nil)
	o := &open{Store: w.ctrl.Store}
	w.ctrl = controller.New(w.ctl, o)
	w.ctrl.Now = w.clock
	// Forty instances are owed a step in one pass, and each step asks for
	// forty jobs: the pass works on the instances side by side, and on each
	// one's jobs side by side.
	const instances, jobs = 40, 40
	for range instances {
		w.submit(wide(jobs), map[string]string{"work.js": "return 1"}, nil)
	}
	w.pass()
	if sent := len(w.list(store.Inbox("r1"))); sent != instances*jobs {
		t.Fatalf("%d jobs were sent", sent)
	}
	if most := o.most.Load(); most > int64(w.ctrl.SideBySide) || most < 2 {
		t.Fatalf("the pass had %d store requests open at once; the bound is %d", most, w.ctrl.SideBySide)
	}
}

func Test_role_empty_pass_only_lists(t *testing.T) {
	only := func(w *world, when string) {
		t.Helper()
		w.noted()
		w.pass()
		ops := w.noted()
		if len(ops) == 0 {
			t.Fatalf("%s: the pass did not look at the store", when)
		}
		for _, op := range ops {
			if op.Act != store.ActList {
				t.Fatalf("%s: a pass that found nothing owed made a %s of %s", when, op.Act, op.Name)
			}
		}
	}
	w := newWorld(t)
	only(w, "on a new store")
	// A run that waits on a key, with a timer that is due sooner than the
	// controller's own wake and one of its own for its history that is due
	// later.
	run := w.submit(door, nil, map[string]any{"greeting": "x"})
	w.settle()
	w.client.Send(ctx, run, "open", nil, 0)
	w.settle()
	if head := w.head(run); head.Timers != 1 || len(head.Keys) != 1 {
		t.Fatalf("the run waits as %+v", head)
	}
	only(w, "with a run waiting")
	w.tick(5 * time.Second)
	only(w, "five seconds later, with nothing yet due")
}
