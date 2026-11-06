package client_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
)

// hostile are expressions a step cannot stop from inside: two that ask for
// more memory than there is, and two that run on inside something built in.
var hostile = []struct{ name, expr string }{
	{"a text", `'a'.repeat(2**40)`},
	{"a number", `String(1n<<(1n<<40n))`},
	{"a pattern", `/(a+)+\1b/.test('a'.repeat(40))`},
	{"a list", `new Array(2**32-1).indexOf(1)`},
}

// maker is an owner's machine with a job that starts the machine its script
// made.
const maker = `
	flow: maker
	input: { text: any }
	stages:
	  make: { job: { script: ./make.js, input: { text: "${input.text}" } } }
	  go: { job: { machine: "${stages.make.output}" } }
	output: ${stages.go.output}
`

var makes = map[string]string{"make.js": `return machines.define(input.text, { files: { "./step.js": "return input" } })`}

// contained has a local whose steps of made machines stop soon.
func contained(t *testing.T, ceiling ...string) *client.Local {
	t.Helper()
	l := local(t, ceiling...)
	l.Controller.StepTime = 2 * time.Second
	return l
}

// runMade runs a machine made while running, from its text, and gives how
// its run ended.
func runMade(t *testing.T, l *client.Local, text string) *client.Run {
	t.Helper()
	return run(t, l, maker, makes, map[string]any{"text": text})
}

// failedAtLimit says a run failed for a step past a limit, in the job go.
func failedAtLimit(t *testing.T, r *client.Run) {
	t.Helper()
	if r.Status != "failed" || !strings.Contains(r.Error, "go: ") ||
		!strings.Contains(r.Error, "may take") && !strings.Contains(r.Error, "without an answer") {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
}

// Each of them fails the machine that holds it. The run goes on and ends
// failed, and another run ends done.
func Test_ctl_made_step_past_a_limit_fails_that_machine(t *testing.T) {
	l := contained(t)
	for _, h := range hostile {
		t.Run(h.name, func(t *testing.T) {
			failedAtLimit(t, runMade(t, l, fmt.Sprintf("flow: bad\nstages:\n  a: { job: ./step.js }\noutput: ${%s}\n", h.expr)))
			other := run(t, l, "flow: other\nstages:\n  add: { job: { script: ./add.js, input: { n: 1 } } }\noutput: ${stages.add.output.n}\n",
				map[string]string{"add.js": scripts["add.js"]}, nil)
			check(t, other.Status, other.Output, "done", `2`)
		})
	}
}

// Wherever a made definition holds an expression, it is read in the step's
// process.
func Test_ctl_made_step_limits_hold_at_every_place(t *testing.T) {
	places := []struct{ name, text string }{
		{"output", "flow: m\nstages:\n  a: { job: ./step.js }\noutput: ${X}\n"},
		{"when", "flow: m\nstages:\n  a: { job: ./step.js, when: \"${X}\" }\n"},
		{"a job's input", "flow: m\nstages:\n  a: { job: { script: ./step.js, input: { v: \"${X}\" } } }\n"},
		{"a timer's time", "statechart: m\ninitial: a\nstates:\n  a:\n    timers: { t: \"${X}\" }\n    on: { t: b }\n  b: { end: done }\n"},
		{"a machine's timeout", "flow: m\nlimits: { timeout: \"${X}\" }\nstages:\n  a: { job: ./step.js }\n"},
		{"a permission", "flow: m\nallow: [\"env:${X}\"]\nstages:\n  a: { job: ./step.js }\n"},
		{"a child's child", "flow: m\nstages:\n  a:\n    job:\n      machine:\n        flow: n\n        stages:\n          b:\n            job:\n              machine:\n                flow: o\n                stages:\n                  c: { job: ./step.js }\n                output: ${X}\n"},
	}
	l := contained(t)
	for _, place := range places {
		for _, h := range hostile {
			t.Run(place.name+" "+h.name, func(t *testing.T) {
				quoted := strings.ReplaceAll(h.expr, `\`, `\\`)
				text := strings.ReplaceAll(place.text, `${X}"`, `${`+quoted+`}"`)
				failedAtLimit(t, runMade(t, l, strings.ReplaceAll(text, "X", h.expr)))
			})
		}
	}
}

// A definition a script stored as a blob of the type is made as any other,
// and what takes twice as long with each part stops at the step's time.
func Test_ctl_made_step_of_a_definition_stored_as_a_blob(t *testing.T) {
	l := contained(t)
	files := map[string]string{"make.js": `
		const stored = machines.define("flow: m\nstages:\n  a: { job: ./step.js }\n", { files: { "./step.js": "return 1" } })
		const def = JSON.parse(blobs.read(stored))
		def.timeout = "${" + input.expr + "}"
		return blobs.make(JSON.stringify(def), { type: "application/x-corbie-definition" })
	`}
	const stored = `
		flow: maker
		input: { expr: any }
		stages:
		  make: { job: { script: ./make.js, input: { expr: "${input.expr}" } } }
		  go: { job: { machine: "${stages.make.output}" } }
	`
	for _, expr := range []string{hostile[0].expr, strings.Repeat("1&&", 40) + "input", strings.Repeat("(a=", 40) + "1" + strings.Repeat(")=>1", 40)} {
		failedAtLimit(t, run(t, l, stored, files, map[string]any{"expr": expr}))
	}
}

// A statechart made while running waits as any other does, across steps of
// their own: on a ticket, on an event from its parent's client, and on a
// timer.
func Test_ctl_made_statechart_waits_across_steps(t *testing.T) {
	l := contained(t)
	id, err := l.Run(ctx, draft(t, `
		statechart: holder
		input: { text: any }
		initial: making
		states:
		  making:
		    job: { script: ./make.js, input: { text: "${input.text}" } }
		    on: { done: running }
		  running:
		    job: { machine: "${states.making.output}" }
		    on:
		      nudge: { send: { to: running, event: nudge } }
		      done: ended
		  ended: { end: done, output: "${states.running.output}" }
	`, makes), client.Start{Input: map[string]any{"text": `statechart: waits
initial: asking
states:
  asking:
    ticket: verdict
    on: { verdict: nudged }
  nudged:
    on: { nudge: pausing }
  pausing:
    timers: { pause: 100ms }
    on: { pause: ended }
  ended: { end: done, output: "${event.name}" }
`}})
	if err != nil {
		t.Fatal(err)
	}
	child, ticket := waitForTicket(t, l, id)
	if _, err := l.Reply(ctx, child, ticket, "yes", 0); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := l.Send(ctx, addressOf(t, l.Client, id), "nudge", nil, 0); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		r, err := l.Show(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != "running" {
			check(t, r.Status, r.Output, "done", `"pause"`)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the made statechart did not end: %+v", r)
		}
	}
}

// An agent made while running goes turn by turn, each in a step of its own.
func Test_ctl_made_agent_works_as_any_other(t *testing.T) {
	m := newModel(t, calls("call_1", "add", `{"n": 41}`), says("It is 42."))
	l := contained(t, "all")
	text := "agent: sums\nmodel: { name: test-model, service: anthropic, address: " + m.URL + ", secret: TEST_KEY }\nprompt: what is 41 plus one?\n" +
		"tools:\n  add:\n    does: Add one to a number\n    args: { n: number }\n    job: { script: ./step.js, input: { n: \"${args.n}\" } }\n"
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err := l.Run(ctx, draft(t, strings.Replace(maker, `machine: "${stages.make.output}"`, `machine: "${stages.make.output}", allow: [all]`, 1), makes),
		client.Start{Input: map[string]any{"text": text}})
	if err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	out, _ := json.Marshal(r.Output)
	if !strings.Contains(string(out), "It is 42.") || len(m.Calls) != 2 {
		t.Fatalf("the agent gave %s after %d calls", out, len(m.Calls))
	}
}

// The machine above a machine that passed a limit is told of it as of any
// failed job, and acts on it.
func Test_ctl_made_step_past_a_limit_is_an_ordinary_failure(t *testing.T) {
	l := contained(t)
	r := run(t, l, `
		statechart: holder
		input: { text: any }
		initial: making
		states:
		  making:
		    job: { script: ./make.js, input: { text: "${input.text}" } }
		    on: { done: running }
		  running:
		    job: { machine: "${states.making.output}" }
		    on:
		      failed: { go: mended }
		  mended: { end: done, output: "${event.data.status}" }
	`, makes, map[string]any{"text": "flow: bad\nstages:\n  a: { job: ./step.js }\noutput: ${'a'.repeat(2**40)}\n"})
	check(t, r.Status, r.Output, "done", `"error"`)
}
