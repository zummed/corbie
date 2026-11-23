package machines

import (
	"context"
	"slices"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// atLocation is what the Machines page shows of the runs at one location:
// how many root runs are running there, how many failed today, and how many
// of the UI's client's running runs are in each state or stage, by its
// name.
type atLocation struct {
	Running, FailedToday int
	In                   map[string]int
}

// counted gives the runs at each location a machine is recorded at, from
// the list of runs the UI reads and the latest step of each of its client's
// running runs there. With no list, nothing is counted.
func (h *handler) counted(ctx context.Context, ix *index) (map[string]atLocation, error) {
	out := map[string]atLocation{}
	if h.List == nil {
		return out, nil
	}
	runs, err := h.List.Runs()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, r := range runs {
		if r.Parent != "" || ix.machines[r.Location] == nil {
			continue
		}
		at := out[r.Location]
		switch {
		case !r.HasEnded():
			at.Running++
			in, err := h.in(ctx, r)
			if err != nil {
				return nil, err
			}
			for _, name := range in {
				if at.In == nil {
					at.In = map[string]int{}
				}
				at.In[name]++
			}
		case r.Status == instance.Failed && sameDay(r.Ended, now):
			at.FailedToday++
		}
		out[r.Location] = at
	}
	return out, nil
}

// sameDay says two times fall on one date, on this computer's clock.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Local().Date()
	by, bm, bd := b.Local().Date()
	return ay == by && am == bm && ad == bd
}

// in gives the states a running run of the UI's client is in and the
// stages it is running, from its latest step; nothing for another client's
// run, or where the UI acts as no client.
func (h *handler) in(ctx context.Context, run *client.Run) ([]string, error) {
	c := h.Client
	if c == nil || c.ID == nil || h.Latest == nil || store.InstanceClient(run.ID) != c.ID.Name {
		return nil, nil
	}
	step, err := h.Latest(ctx, c, run)
	if err != nil || step == nil {
		return nil, err
	}
	t, ok := machine.TypeOf(step.State.Type)
	if !ok {
		return nil, nil
	}
	shown := t.Show(step.State.Inner)
	names := slices.Clone(shown.States)
	for _, name := range sortedKeys(shown.Stages) {
		if shown.Stages[name] == instance.Running {
			names = append(names, name)
		}
	}
	return names, nil
}
