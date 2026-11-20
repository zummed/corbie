package drawing

import (
	"slices"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/machine/agent"
)

// agentOf gives an agent's model, what it can call, and its ask. On a run's
// page a toolkit whose tools the run has not asked for is folded.
func agentOf(def *machine.Definition, a *agent.Agent, run *Run, w *walker) *Agent {
	out := &Agent{Model: a.Model.Name, Turns: a.Turns, Toolkits: []Toolkit{}, Turn: run.latest().Turn}
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
	own := w.file
	if own == "" {
		own = fileOf(def.Lines[def.Type])
	}
	at := map[string]int{}
	for _, name := range sortedKeys(a.Toolkits) {
		at[name] = len(out.Toolkits)
		out.Toolkits = append(out.Toolkits, Toolkit{Name: name, File: ToolkitFile(def, name, own), When: a.Toolkits[name].When, Toolkit: true, Tools: []Tool{}})
	}
	var free *Toolkit
	for _, t := range sortedKeys(a.Tools) {
		tool := a.Tools[t]
		if tool.Job != nil && strings.HasPrefix(tool.Job.Script, "corbie:") {
			continue // Corbie's own, which no definition writes
		}
		place := "tools." + t
		if tool.Toolkit != "" {
			place = "toolkits." + tool.Toolkit + ".tools." + t
		}
		c := Card{Name: t, Title: titleOf(tool.Description, tool.When), Line: lineOf(w.lines, place)}
		w.describe(&c, t, tool.Job)
		it := Tool{Card: c, When: tool.When != "", Mark: jobMark(run, t)}
		if i, ok := at[tool.Toolkit]; ok {
			out.Toolkits[i].Tools = append(out.Toolkits[i].Tools, it)
			continue
		}
		if free == nil {
			free = &Toolkit{Tools: []Tool{}}
		}
		free.Tools = append(free.Tools, it)
	}
	if free != nil {
		out.Toolkits = append(out.Toolkits, *free)
	}
	for i, g := range out.Toolkits {
		used := slices.ContainsFunc(g.Tools, func(t Tool) bool { return t.Mark != "never" })
		out.Toolkits[i].Folded = run != nil && g.Toolkit && !used
	}
	if ask := def.Ask; ask != nil {
		c := Card{Name: "ask", Line: lineOf(w.lines, "ask")}
		w.describe(&c, "ask", ask)
		out.Ask, out.AskMark = &c, jobMark(run, "ask")
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

// jobMark is how a tool or an ask of a run is marked: where the run failed,
// held for the ask, going now, asked for at an earlier step, or never asked
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
