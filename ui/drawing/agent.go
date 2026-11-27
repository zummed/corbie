package drawing

import (
	"slices"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
	"github.com/zummed/corbie/machine/toolkit"
)

// agentOf gives an agent's model, its entries, each a card, and its hooks.
// An entry that names a toolkit is marked by the jobs the run asked for
// through it.
func agentOf(def *machine.Definition, a *agent.Agent, run *Run, w *walker) *Agent {
	out := &Agent{Model: a.Model.Name, Turns: a.Turns, Tools: []Tool{}, Turn: run.latest().Turn}
	if out.Model == "" {
		out.Model = "model"
	}
	if out.Turns == 0 {
		out.Turns = 20
	}
	out.Mark = jobMark(run, "turn")
	out.ModelLine = lineOf(w.lines, "model")
	if out.Mark != "" && out.Mark != "failed" && out.Mark != "now" {
		out.Mark = "done"
	}
	for _, t := range sortedKeys(a.Tools) {
		tool := a.Tools[t]
		if tool.Job != nil && strings.HasPrefix(tool.Job.Script, "corbie:") {
			continue // Corbie's own, which no definition writes
		}
		c := Card{Name: t, Title: titleOf(tool.Description, tool.When), Line: lineOf(w.lines, "tools."+t)}
		w.describe(&c, t, tool.Job)
		if tool.Toolkit {
			c.Icon = "toolkit"
		}
		out.Tools = append(out.Tools, Tool{Card: c, When: tool.When != "", Mark: jobMark(run, t)})
	}
	out.Hooks = hooksOf(def, run, w)
	return out
}

// hooksOf gives a definition's hooks, each a card with its mark.
func hooksOf(def *machine.Definition, run *Run, w *walker) []Hook {
	var out []Hook
	for _, name := range sortedKeys(def.Hooks) {
		h, when, job := def.Hooks[name], "before", def.Hooks[name].Before
		if job == nil {
			when, job = "after", h.After
		}
		c := Card{Name: name, Title: titleOf(when+" a job", h.When), Line: lineOf(w.lines, "hooks."+name)}
		w.describe(&c, HookJob(name), job)
		out = append(out, Hook{Card: c, When: h.When != "", Mark: jobMark(run, HookJob(name))})
	}
	return out
}

// kitOf gives a toolkit's tools, each a card with its mark, or the one job
// that answers every request, and its hooks.
func kitOf(def *machine.Definition, k *toolkit.Toolkit, run *Run, w *walker) *Kit {
	out := &Kit{Tools: []Tool{}, Hooks: hooksOf(def, run, w)}
	if k.Job != nil {
		c := Card{Name: "job", Title: "answers every request", Line: lineOf(w.lines, "job")}
		w.describe(&c, "job", k.Job)
		out.Tools = append(out.Tools, Tool{Card: c, Mark: jobMark(run, "job")})
	}
	for _, name := range sortedKeys(k.Tools) {
		c := Card{Name: name, Title: k.Tools[name].Description, Line: lineOf(w.lines, "tools."+name)}
		w.describe(&c, name, k.Tools[name].Job)
		out.Tools = append(out.Tools, Tool{Card: c, Mark: jobMark(run, name)})
	}
	return out
}

// decisionOf gives a decision's model, and its answers, with each
// question's name.
func decisionOf(dec *agent.Decision, run *Run) *Decision {
	out := &Decision{Model: dec.Model.Name, Answers: Card{Name: "answers"}}
	if out.Model == "" {
		out.Model = "model"
	}
	if questions, ok := dec.Questions.(map[string]any); ok {
		for _, question := range sortedKeys(questions) {
			out.Answers.Lines = append(out.Answers.Lines, fit(question))
		}
	}
	out.Mark = jobMark(run, "decide")
	if out.Mark != "" && out.Mark != "failed" && out.Mark != "now" {
		out.Mark = "done"
	}
	return out
}

// jobMark is how a tool or a hook of a run is marked: where the run failed,
// held for a hook, going now, asked for at an earlier step, or never asked
// for.
func jobMark(run *Run, name string) string {
	switch {
	case run == nil:
		return ""
	case run.failedAt(name):
		return "failed"
	case slices.Contains(run.Held, name) && !run.Ended:
		return "held"
	case slices.Contains(run.Going, name) && !run.Ended:
		return "now"
	case run.asked(name):
		return "done"
	}
	return "never"
}
