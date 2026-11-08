package client_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
)

// followAll runs a definition and gives every entry of its record.
func followAll(t *testing.T, l *client.Local, def string, files map[string]string, input any) (*client.Run, []client.Entry) {
	t.Helper()
	id, err := l.Run(ctx, draft(t, def, files), client.Start{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	var entries []client.Entry
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) { entries = append(entries, e) })
	if err != nil || r.Status != instance.Done {
		t.Fatalf("the run ended %+v, %v", r, err)
	}
	return r, entries
}

// shownAfter gives where a run stood after each of its steps.
func shownAfter(entries []client.Entry) []machine.Shown {
	var shown []machine.Shown
	for _, e := range entries {
		if e.Step > 0 {
			shown = append(shown, e.Shown)
		}
	}
	return shown
}

// Each step is shown with where the run stands, in one shape for every type:
// a flow's stages with their statuses, a statechart's active states, an
// agent's turns. Nothing the head says is in it.
func Test_inst_step_shows_where_the_run_stands(t *testing.T) {
	l := local(t, "all")
	_, entries := followAll(t, l, `
		flow: two
		stages:
		  first: { job: { script: ./one.js } }
		  second: { follows: [first], job: { script: ./one.js } }
	`, map[string]string{"one.js": "return 1"}, nil)
	flow := shownAfter(entries)
	if got := flow[0]; got.Stages["first"] != "running" || got.Stages["second"] != instance.Waiting || got.States != nil || got.Turn != 0 {
		t.Fatalf("the flow's first step is shown as %+v", got)
	}
	if got := flow[len(flow)-1]; got.Stages["first"] != instance.Done || got.Stages["second"] != instance.Done {
		t.Fatalf("the flow's last step is shown as %+v", got)
	}

	_, entries = followAll(t, l, `
		statechart: door
		initial: shut
		states:
		  shut:
		    job: { script: ./one.js }
		    on: { done: open }
		  open: { end: done }
	`, map[string]string{"one.js": "return 1"}, nil)
	chart := shownAfter(entries)
	if !slices.Equal(chart[0].States, []string{"shut"}) || chart[0].Stages != nil {
		t.Fatalf("the chart's first step is shown as %+v", chart[0])
	}

	m := newModel(t, calls("call_1", "add", `{"n": 41}`), says("It is 42."))
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	_, entries = followAll(t, l, agentDef(m.URL, ""), scripts, map[string]any{"question": "what is 41 plus one?"})
	agent := shownAfter(entries)
	if agent[0].Turn != 1 || agent[len(agent)-1].Turn != 2 || agent[0].States != nil {
		t.Fatalf("the agent's steps are shown as %+v", agent)
	}
	for _, e := range entries {
		if e.Step == 0 {
			continue
		}
		data, _ := json.Marshal(e)
		var fields struct {
			State map[string]any `json:"state"`
		}
		json.Unmarshal(data, &fields)
		for name := range fields.State {
			if name != "turn" {
				t.Fatalf("an agent's step shows %s: %s", name, data)
			}
		}
	}
}

// An agent's record holds its turns as jobs named turn, the reply's text
// emitted under text, each turn's result with its text, calls, stop and
// usage, and each tool call as a job named after its tool.
func Test_agent_record_names_its_entries(t *testing.T) {
	m := newModel(t, calls("call_1", "add", `{"n": 41}`), says("It is 42."))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	_, entries := followAll(t, l, agentDef(m.URL, ""), scripts, map[string]any{"question": "what is 41 plus one?"})
	var jobs, texts []string
	var turns []map[string]any
	for _, e := range entries {
		if e.Step == 0 && e.Name == "text" {
			texts = append(texts, e.Job)
		}
		for _, ask := range e.Asks {
			if ask.Kind != instance.AskScript {
				continue
			}
			jobs = append(jobs, ask.Name)
			if model, _ := ask.Job.Input.(map[string]any)["model"].(map[string]any); ask.Name == "turn" && model["name"] != "test-model" {
				t.Fatalf("the turn was given the model %v", model)
			}
		}
		for _, ev := range e.Events {
			if ev.Name == machine.Done && (ev.From == "turn" || ev.From == "turn#2") {
				data, _ := ev.Data.(map[string]any)
				turns = append(turns, data)
			}
		}
	}
	if !slices.Equal(jobs, []string{"turn", "add", "turn#2"}) {
		t.Fatalf("the jobs were %v", jobs)
	}
	if len(texts) == 0 || texts[0] != "turn" {
		t.Fatalf("text was emitted by %v", texts)
	}
	if len(turns) != 2 {
		t.Fatalf("the turns gave %v", turns)
	}
	first, last := turns[0], turns[1]
	if calls, _ := first["calls"].([]any); len(calls) != 1 || first["stop"] == nil || first["turn"] == nil || first["usage"] == nil {
		t.Fatalf("the first turn gave %v", first)
	}
	if last["text"] != "It is 42." || last["stop"] != "end" {
		t.Fatalf("the last turn gave %v", last)
	}
}
