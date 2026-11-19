package machines

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
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
	read := h.steps.reader(ctx, h.Client)
	now := time.Now()
	for _, r := range runs {
		if r.Parent != "" || !ours[r.Location] {
			continue
		}
		at := out[r.Location]
		switch {
		case !r.HasEnded():
			at.Running++
			for _, name := range read.in(r) {
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
	return out, read.done()
}

// sameDay says two times fall on one date, on this computer's clock.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Local().Date()
	by, bm, bd := b.Local().Date()
	return ay == by && am == bm && ad == bd
}

// steps keeps the states and stages each of the client's running runs was
// in at its latest step, by the step of its head it was read at: a step
// never changes once it is written.
type steps struct {
	mu   sync.Mutex
	read map[string]stepIn
}

type stepIn struct {
	at int
	in []string
}

// A stepReader reads the steps one count of the page needs. The page is
// counted by one reader at a time.
type stepReader struct {
	ctx  context.Context
	c    *client.Client
	s    *steps
	kept map[string]stepIn
	errs []error
}

func (s *steps) reader(ctx context.Context, c *client.Client) *stepReader {
	s.mu.Lock()
	return &stepReader{ctx: ctx, c: c, s: s, kept: map[string]stepIn{}}
}

// in gives the states a run is in and the stages it is running, from its
// latest step, read again only where its head has moved on; nothing for
// another client's run.
func (r *stepReader) in(run *client.Run) []string {
	if r.c == nil || r.c.ID == nil || store.InstanceClient(run.ID) != r.c.ID.Name {
		return nil
	}
	got, ok := r.s.read[run.ID]
	if !ok || got.at != run.Step {
		step, err := instance.Latest(r.ctx, r.c.Store, r.c.Controller, r.c.ID, run.ID, run.Step)
		if err != nil && !errors.Is(err, identity.ErrNotForUs) {
			r.errs = append(r.errs, err)
			return nil
		}
		got = stepIn{at: run.Step, in: inOf(step)}
	}
	r.kept[run.ID] = got
	return got.in
}

// done keeps the steps of the runs counted, forgets the rest, and gives
// what could not be read.
func (r *stepReader) done() error {
	r.s.read = r.kept
	r.s.mu.Unlock()
	return errors.Join(r.errs...)
}

// inOf gives the states a step left its run in and the stages it left
// running.
func inOf(step *instance.Step) []string {
	if step == nil {
		return nil
	}
	t, ok := machine.TypeOf(step.State.Type)
	if !ok {
		return nil
	}
	shown := t.Show(step.State.Inner)
	names := append([]string{}, shown.States...)
	for _, name := range sortedKeys(shown.Stages) {
		if shown.Stages[name] == instance.Running {
			names = append(names, name)
		}
	}
	return names
}

// Differs gives the lines of the machine now in the directory at a location
// that differ from def, the definition a run there started with: each
// value removed with "-" before it and each value added with "+", as a
// plain line diff of the two definitions written one value to a line. It gives none where they
// are the same, and where the page does not show the location's machine:
// a tree submitted from another directory, or no machine there. The files
// a definition's jobs run are named, not compared.
func (p *Pages) Differs(ctx context.Context, location, dir string, def *machine.Definition) ([]string, error) {
	if dir != p.h.dir.real || def == nil {
		return nil, nil
	}
	ix, err := p.h.tree(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range ix.machines {
		if m.Location != location || m.Draft == nil {
			continue
		}
		was, err := compared(def)
		if err != nil {
			return nil, err
		}
		now, err := compared(m.Draft.Definition)
		if err != nil {
			return nil, err
		}
		return lineDiff(was, now), nil
	}
	return nil, nil
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

// mostLines is how long two definitions may be for Differs to give their
// lines: past it, it gives that they differ.
const mostLines = 2000

// lineDiff gives the lines of b that are not in a, with "+", and of a that
// are not in b, with "-", in order, by the longest run of lines they share.
func lineDiff(a, b []string) []string {
	if len(a) > mostLines || len(b) > mostLines {
		if strings.Join(a, "\n") == strings.Join(b, "\n") {
			return nil
		}
		return []string{"the definition differs, and is too long to show line by line"}
	}
	// longest[i][j] is the length of the longest run a[i:] and b[j:] share.
	longest := make([][]int, len(a)+1)
	for i := range longest {
		longest[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				longest[i][j] = longest[i+1][j+1] + 1
			} else {
				longest[i][j] = max(longest[i+1][j], longest[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i, j = i+1, j+1
		case j < len(b) && (i == len(a) || longest[i][j+1] >= longest[i+1][j]):
			out = append(out, "+ "+b[j])
			j++
		default:
			out = append(out, "- "+a[i])
			i++
		}
	}
	return out
}
