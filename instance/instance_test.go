package instance

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/store"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
)

type world struct {
	t   *testing.T
	s   store.Store
	ctl *identity.Identity
	c1  *identity.Identity
}

func newWorld(t *testing.T) *world {
	s, _ := store.Open("memory:")
	ctl, _ := identity.New("ctl", identity.Controller)
	c1, _ := identity.New("c1", identity.Client)
	return &world{t, s, ctl, c1}
}

func (w *world) put(at string, body any, to ...identity.Card) {
	w.t.Helper()
	data, err := w.ctl.Write(at, now, time.Time{}, body, to...)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.s.Write(ctx, at, data, store.Overwrite); err != nil {
		w.t.Fatal(err)
	}
}

const run = "c1/20261010T120000.000Z-0123456789"

func Test_inst_head_in_both_places_reads_as_finished(t *testing.T) {
	w := newWorld(t)
	// A controller stopped between the two writes of the move leaves both.
	w.put(store.RunningHead("ctl", run), &Head{Name: run, Status: Running, Step: 3})
	w.put(store.FinishedHead("ctl", run), &Head{Name: run, Status: Done, Step: 4})
	head, _, err := ReadHead(ctx, w.s, w.ctl.Card, run)
	if err != nil || head.Status != Done || head.Step != 4 {
		t.Fatalf("a head in both places reads as %+v (%v)", head, err)
	}
}

// moving is a store in which a head moves from the running to the
// finished between two looks.
type moving struct {
	store.Store
	finished []byte
	looks    int
}

func (m *moving) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	m.looks++
	if name == store.FinishedHead("ctl", run) && m.looks >= 3 {
		return m.finished, "moved", nil
	}
	return nil, store.None, nil // not among the finished yet, and gone from the running
}

func Test_inst_head_moves_between_list_and_read(t *testing.T) {
	w := newWorld(t)
	finished, _ := w.ctl.Write(store.FinishedHead("ctl", run), now, time.Time{}, &Head{Name: run, Status: Done, Step: 4})
	head, _, err := ReadHead(ctx, &moving{Store: w.s, finished: finished}, w.ctl.Card, run)
	if err != nil || head == nil || head.Status != Done {
		t.Fatalf("a head that moved between the looks reads as %+v (%v)", head, err)
	}
}

func TestHeadIsCheckedAgainstTheControllersCard(t *testing.T) {
	w := newWorld(t)
	at := store.RunningHead("ctl", run)
	w.put(at, &Head{Name: run, Status: Running, Step: 2, Machine: "review", Labels: map[string]string{"team": "ops"}})
	// Anyone who can read it reads all of it, with no identity of their own.
	head, _, err := ReadHead(ctx, w.s, w.ctl.Card, run)
	if err != nil || head.Machine != "review" || head.Labels["team"] != "ops" {
		t.Fatalf("the head reads as %+v (%v)", head, err)
	}
	// One byte changed, and it does not check.
	raw, _, _ := w.s.Read(ctx, at)
	w.s.Write(ctx, at, bytes.Replace(raw, []byte("review"), []byte("revieW"), 1), store.Overwrite)
	if _, _, err := ReadHead(ctx, w.s, w.ctl.Card, run); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("an altered head: %v", err)
	}
	// One instance's head under another's name is refused.
	other := "c1/20261010T120000.000Z-9999999999"
	moved, _ := w.ctl.Write(at, now, time.Time{}, &Head{Name: run, Status: Done})
	w.s.Write(ctx, store.RunningHead("ctl", other), moved, store.Overwrite)
	if _, _, err := ReadHead(ctx, w.s, w.ctl.Card, other); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a head moved to another name: %v", err)
	}
	// Signed by someone who is not the controller, it does not check.
	forged, _ := w.c1.Write(at, now, time.Time{}, &Head{Name: run, Status: Done})
	w.s.Write(ctx, at, forged, store.Overwrite)
	if _, _, err := ReadHead(ctx, w.s, w.ctl.Card, run); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a head signed by another party: %v", err)
	}
}

func Test_store_value_moved_to_another_name_refused(t *testing.T) {
	w := newWorld(t)
	w.put(store.Step("ctl", run, 1), &Step{Number: 1}, w.ctl.Card, w.c1.Card)
	w.put(store.Step("ctl", run, 2), &Step{Number: 2}, w.ctl.Card, w.c1.Card)
	// An earlier step put in place of a later one.
	early, _, _ := w.s.Read(ctx, store.Step("ctl", run, 1))
	w.s.Write(ctx, store.Step("ctl", run, 2), early, store.Overwrite)
	if _, err := ReadStep(ctx, w.s, w.ctl.Card, w.c1, run, 2); !errors.Is(err, identity.ErrAltered) {
		t.Fatalf("a step moved to another number: %v", err)
	}
	if step, err := ReadStep(ctx, w.s, w.ctl.Card, w.c1, run, 1); err != nil || step.Number != 1 {
		t.Fatalf("the step in its own place: %v", err)
	}
}

func Test_ctl_latest_step_found_past_stale_head(t *testing.T) {
	w := newWorld(t)
	for n := 1; n <= 5; n++ {
		w.put(store.Step("ctl", run, n), &Step{Number: n}, w.ctl.Card)
	}
	// The head says 3 while entries 4 and 5 exist.
	latest, err := Latest(ctx, w.s, w.ctl.Card, w.ctl, run, 3)
	if err != nil || latest.Number != 5 {
		t.Fatalf("from a head at step 3 the latest step is %+v (%v)", latest, err)
	}
	// With the older steps trimmed away, the head still leads to the latest.
	for n := 1; n <= 4; n++ {
		w.s.Delete(ctx, store.Step("ctl", run, n), store.Overwrite)
	}
	w.put(store.RunningHead("ctl", run), &Head{Name: run, Status: Running, Step: 5})
	if current, err := Current(ctx, w.s, w.ctl.Card, w.ctl, run); err != nil || current == nil || current.Number != 5 {
		t.Fatalf("after trimming the current step is %+v (%v)", current, err)
	}
}

// Everything in a record is sealed so that the controller and the client
// that started the root can read it, and nobody else.
func TestRecordIsSealedToControllerAndClient(t *testing.T) {
	w := newWorld(t)
	c2, _ := identity.New("c2", identity.Client)
	w.put(store.Step("ctl", run, 1), &Step{Number: 1, State: State{Output: "MARKER"}}, w.ctl.Card, w.c1.Card)
	raw, _, _ := w.s.Read(ctx, store.Step("ctl", run, 1))
	if bytes.Contains(raw, []byte("MARKER")) {
		t.Fatal("a step entry shows what it holds")
	}
	for _, reader := range []*identity.Identity{w.ctl, w.c1} {
		if step, err := ReadStep(ctx, w.s, w.ctl.Card, reader, run, 1); err != nil || step.State.Output != "MARKER" {
			t.Fatalf("%s cannot read the record: %v", reader.Name, err)
		}
	}
	if _, err := ReadStep(ctx, w.s, w.ctl.Card, c2, run, 1); !errors.Is(err, identity.ErrNotForUs) {
		t.Fatalf("another client reading the record: %v", err)
	}
}
