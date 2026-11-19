package machines

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"slices"
	"strconv"
	"time"

	"github.com/zummed/corbie/blob"
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

// counted gives the runs at each location of the directory, from the list of
// runs the UI reads and the latest step of each of its client's running
// runs there. With no list, nothing is counted.
func (h *handler) counted(ctx context.Context, ix *index) (map[string]atLocation, error) {
	out := map[string]atLocation{}
	if h.List == nil {
		return out, nil
	}
	runs, err := h.List.Runs()
	if err != nil {
		return nil, err
	}
	ours := map[string]bool{}
	for _, m := range ix.machines {
		if m.Location != "" {
			ours[m.Location] = true
		}
	}
	now := time.Now()
	for _, r := range runs {
		if r.Parent != "" || !ours[r.Location] {
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

// Differs gives how the machine now in the directory at a location differs
// from the definition a run there started with, stored at def in the run's
// record, which the UI's client reads:
//
//   - each value removed, with "-" before it and its path, and each value
//     added, with "+";
//   - each file a job runs whose contents differ: `a script changed: ./find.js`;
//   - each child machine that differs in any of these: `a child changed: ./stuck.yaml`.
//
// It gives none where they are the same, and where the page does not show
// the location's machine: a tree submitted from another directory, or no
// machine there.
func (p *Pages) Differs(ctx context.Context, location, dir string, def blob.Ref) ([]string, error) {
	h := p.h
	if dir != h.dir.real || h.Client == nil {
		return nil, nil
	}
	ix, err := h.tree(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range ix.machines {
		if m.Location == location && m.Draft != nil {
			return differs(ctx, h.Client.Store, def, m.Draft)
		}
	}
	return nil, nil
}

// differs gives how a draft differs from the definition stored at ref, as
// Differs gives it.
func differs(ctx context.Context, s store.Store, ref blob.Ref, d *machine.Draft) ([]string, error) {
	was, err := machine.Load(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	a, err := compared(was)
	if err != nil {
		return nil, err
	}
	b, err := compared(d.Definition)
	if err != nil {
		return nil, err
	}
	out := lineDiff(a, b)
	for _, name := range sortedKeys(was.Scripts) {
		now, ok := d.Files[name]
		if !ok {
			continue // its job is gone, which a value's line says
		}
		contents, err := blob.ReadAll(ctx, s, ref.Beside(was.Scripts[name]), blob.StepLimit)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(contents, now) {
			out = append(out, "a script changed: "+name)
		}
	}
	for _, name := range sortedKeys(was.Machines) {
		child := d.Children[name]
		if child == nil {
			continue
		}
		changed, err := differs(ctx, s, ref.Beside(was.Machines[name]), child)
		if err != nil {
			return nil, err
		}
		if len(changed) > 0 {
			out = append(out, "a child changed: "+name)
		}
	}
	return out, nil
}

// compared gives a definition as the lines Differs compares: one line for
// each value, with its path, such as `states.open.on.shut.0.go: "closed"`,
// without what a store or a file's layout gives it: the references to its
// stored files and machines, and each value's line. The paths of the
// type's own fields are written from its top, as the file writes them.
func compared(def *machine.Definition) ([]string, error) {
	d := *def
	d.Scripts, d.Machines, d.Lines = nil, nil, nil
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	body := v["body"]
	delete(v, "body")
	var out []string
	flatten("", v, &out)
	flatten("", body, &out)
	return out, nil
}

// flatten writes a value as one line for each value in it, with its path.
func flatten(at string, v any, out *[]string) {
	join := func(name string) string {
		if at == "" {
			return name
		}
		return at + "." + name
	}
	switch v := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(v) {
			flatten(join(k), v[k], out)
		}
	case []any:
		for i, item := range v {
			flatten(join(strconv.Itoa(i)), item, out)
		}
	default:
		data, _ := json.Marshal(v)
		*out = append(*out, at+": "+string(data))
	}
}

// lineDiff gives the lines of a that b has not, with "-", then the lines of
// b that a has not, with "+". Each line holds its value's path, so a line
// is in both or it differs.
func lineDiff(a, b []string) []string {
	in := func(lines []string) map[string]bool {
		set := map[string]bool{}
		for _, l := range lines {
			set[l] = true
		}
		return set
	}
	inA, inB := in(a), in(b)
	var out []string
	for _, l := range a {
		if !inB[l] {
			out = append(out, "- "+l)
		}
	}
	for _, l := range b {
		if !inA[l] {
			out = append(out, "+ "+l)
		}
	}
	return out
}
