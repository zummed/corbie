package agent

import (
	"encoding/json/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
)

const urgent = `decision: urgent
input: { text: string }
allow: [disk]
model: { name: jev-latest, secret: TYPESAFE_KEY }
state: ${input.text}
questions:
  urgent: { type: noul, instructions: "The customer cannot use ${input.product}." }
`

func parseDecision(t *testing.T, src string) (*machine.Definition, error) {
	t.Helper()
	draft, err := machine.Parse([]byte(src), func(string) ([]byte, error) { return nil, nil })
	if err != nil {
		return nil, err
	}
	return draft.Definition, nil
}

// A decision asks for one job with its state and questions filled in, and
// ends with that job's output or its failure.
func TestDecisionAsksOneJobAndEndsWithIt(t *testing.T) {
	def, err := parseDecision(t, urgent)
	if err != nil {
		t.Fatal(err)
	}
	step := func(events ...machine.Event) *machine.StepOut {
		t.Helper()
		out, err := decisionType{}.Step(&machine.StepIn{Definition: def, Events: events,
			Input: map[string]any{"text": "checkout fails", "product": "the shop"}, Time: time.Unix(0, 0), Seed: []byte("s")})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := step(machine.Event{Name: machine.Start})
	if len(out.Requests) != 1 || out.Requests[0].Job == nil || out.Requests[0].Job.Name != "decide" {
		t.Fatalf("at its start a decision asked for %+v", out.Requests)
	}
	given, _ := json.Marshal(out.Requests[0].Job.Input, json.Deterministic(true))
	want := `{"model":{"address":"https://api.typesafe.ai","name":"jev-latest","secret":"TYPESAFE_KEY","service":"systemone"},` +
		`"questions":{"urgent":{"instructions":"The customer cannot use the shop.","type":"noul"}},"state":"checkout fails"}`
	if string(given) != want {
		t.Fatalf("the decide job was given\n%s\nwant\n%s", given, want)
	}

	answers := map[string]any{"answers": map[string]any{"urgent": map[string]any{"noul": 0.99}}}
	out = step(machine.Event{Name: machine.Done, From: "decide", Data: answers})
	if len(out.Requests) != 1 || out.Requests[0].End == nil || out.Requests[0].End.Error != nil {
		t.Fatalf("the job's end gave %+v", out.Requests)
	}
	if got, _ := json.Marshal(out.Requests[0].End.Output); string(got) != `{"answers":{"urgent":{"noul":0.99}}}` {
		t.Fatalf("the decision's output is %s", got)
	}
	out = step(machine.Event{Name: machine.Failed, From: "decide", Data: map[string]any{"status": "error", "error": "the service returned 500"}})
	if end := out.Requests[0].End; end == nil || end.Error == nil || end.Error.Error != "decide: the service returned 500" {
		t.Fatalf("the job's failure gave %+v", out.Requests)
	}
	// Any other event is not acted on.
	if out = step(machine.Event{Name: "poke"}); len(out.Requests) != 0 || !slices.Equal(out.Ignored, []int{0}) {
		t.Fatalf("an event sent to a decision gave %+v, ignored %v", out.Requests, out.Ignored)
	}

	for src, want := range map[string]string{
		strings.Replace(urgent, "state: ${input.text}\n", "", 1): "a decision gives state:",
		urgent[:strings.Index(urgent, "questions:")]:             "a decision gives questions:",
	} {
		if _, err := parseDecision(t, src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("a decision without it gave %v, want %q", err, want)
		}
	}
}

// A decision's model is of a service that has decide, and its job is
// written the connection to that service and the key bound to it.
func TestDecisionModelHasDecide(t *testing.T) {
	def, err := parseDecision(t, urgent)
	if err != nil {
		t.Fatal(err)
	}
	var d Decision
	if err := json.Unmarshal(def.Body, &d); err != nil {
		t.Fatal(err)
	}
	want := []string{"net:api.typesafe.ai:443", "secret:TYPESAFE_KEY@net:api.typesafe.ai:443"}
	if !slices.Equal(d.Decide.Allow, want) || !d.Decide.Alone {
		t.Fatalf("the decide job allows %v, alone %v; want %v and nothing from the definition", d.Decide.Allow, d.Decide.Alone, want)
	}
	for model, want := range map[string]string{
		"{ name: claude-sonnet-5-5 }":        "has no decide",
		"{ name: my-model }":                 "which service the model",
		"{ name: jev-latest, effort: high }": "effort",
		"{ secret: TYPESAFE_KEY }":           "a decision names its model",
	} {
		src := strings.Replace(urgent, "{ name: jev-latest, secret: TYPESAFE_KEY }", model, 1)
		if _, err := parseDecision(t, src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("model: %s gave %v, want %q", model, err, want)
		}
	}
}
