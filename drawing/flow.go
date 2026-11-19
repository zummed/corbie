package drawing

import (
	"slices"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/flow"
)

// flowOf gives a flow's stages, by name, each with the stages it waits for.
func flowOf(f *flow.Flow, run *Run, w *walker) *Flow {
	out := &Flow{Stages: []Stage{}}
	stages := run.latest().Stages
	for _, n := range sortedKeys(f.Stages) {
		s := f.Stages[n]
		st := Stage{Card: Card{Name: n}, After: []string{}, When: s.When != "", Stack: s.Each != nil || s.Emits != "" || s.Blob != nil}
		for _, a := range waitsFor(s) {
			if _, ok := f.Stages[a]; ok && a != n {
				st.After = append(st.After, a)
			}
		}
		w.describe(&st.Card, n, s.Job)
		st.Card.Title = titleOf("", s.When)
		if run != nil {
			st.Mark = stageMark(n, stages[n], run)
		}
		out.Stages = append(out.Stages, st)
	}
	return out
}

// stageMark is how a stage's status is marked. A stage still running when
// its run ended is marked by how the run ended, and one whose job is held
// for the ask is marked held.
func stageMark(name, status string, run *Run) string {
	switch {
	case status == "running" && run.Failed:
		return "failed"
	case status == "running" && run.Ended:
		return "cancelled"
	case status == "running" && slices.Contains(run.Held, name):
		return "held"
	case status == "running":
		return "now"
	}
	switch status {
	case "done", "failed", "skipped":
		return status
	}
	return "never"
}

// waitsFor gives the stages a stage waits for: those its expressions read,
// those it follows, and the one whose emitted items are its own. It is the
// rule the flow package keeps to itself, applied to the same expressions.
func waitsFor(s *flow.Stage) []string {
	var reads []machine.Read
	if s.Job != nil {
		reads = append(reads, machine.ReadsIn(s.Job.Input, "stages")...)
		reads = append(reads, machine.Reads(s.Job.Machine, "stages")...)
		reads = append(reads, machine.Reads(s.Job.Script, "stages")...)
		reads = append(reads, machine.ReadsIn(s.Job.Addressed(), "stages")...)
	}
	reads = append(reads, machine.ReadsIn(s.Each, "stages")...)
	reads = append(reads, machine.ReadsIn(s.Blob, "stages")...)
	if s.When != "" {
		reads = append(reads, machine.ReadsOf(s.When, "stages")...)
	}
	var names []string
	add := func(n string) {
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	for _, r := range reads {
		add(r.Name)
	}
	for _, n := range s.Follows {
		add(n)
	}
	add(s.Emits)
	return names
}
