package client_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
)

func TestAskApprovesSomeOfAStepsJobs(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: approvals
		ask: { script: ./ask.js, input: { limit: 2 } }
		stages:
		  items:
		    each: [1, 2, 3, 4]
		    job: { script: ./add.js, input: { n: "${item}" }, approve: true }
		output: "${stages.items.output.map(r => r.output ? r.output.n : r.error)}"
	`, map[string]string{
		"add.js": scripts["add.js"],
		"ask.js": `
			if (input.asking.machine !== "approvals") throw new Error("not told who is asking")
			return {
				approve: input.jobs.filter(r => r.input.n <= input.limit).map(r => r.name),
				note: "only small numbers",
			}
		`,
	}, nil)
	check(t, r.Status, r.Output, "done", `[2, 3, "the ask did not approve the job: only small numbers", "the ask did not approve the job: only small numbers"]`)
}

func TestDenialNobodyHandlesFailsTheMachineAsDenied(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: refused
		ask: { script: ./ask.js }
		stages:
		  one:
		    job: { script: ./add.js, input: { n: 1 }, approve: true }
	`, map[string]string{"add.js": scripts["add.js"], "ask.js": `return { approve: [] }`}, nil)
	if r.Status != "failed" || r.Failure != "denied" || !strings.Contains(r.Error, "one: the ask did not approve the job") {
		t.Fatalf("the flow ended %s/%s: %q", r.Status, r.Failure, r.Error)
	}
}

func TestAskThatFailsOrDoesNotDecideApprovesNothing(t *testing.T) {
	l := local(t)
	for ask, want := range map[string]string{
		`throw new Error("no")`: "the ask failed, which approves nothing",
		`return "maybe"`:        "the ask's result has no approve list",
	} {
		r := run(t, l, `
			flow: undecided
			ask: { script: ./ask.js }
			stages:
			  one:
			    job: { script: ./add.js, input: { n: 1 }, approve: true }
			  note:
			    when: ${stages.one.failed}
			    job: { script: ./echo.js, input: { why: "${stages.one.error}" } }
			output: ${stages.note.output.why}
		`, map[string]string{"add.js": scripts["add.js"], "echo.js": scripts["echo.js"], "ask.js": ask}, nil)
		if r.Status != "done" || !strings.Contains(r.Output.(string), want) {
			t.Fatalf("with the ask %q the flow ended %s: %v %s", ask, r.Status, r.Output, r.Error)
		}
	}
}

func TestAPermissionListedToBeAskedAboutHoldsTheJob(t *testing.T) {
	l := local(t, "all")
	r := run(t, l, `
		flow: asked-about
		ask: { script: ./ask.js }
		ask_about: [exec]
		stages:
		  plain:
		    job: { script: ./add.js, input: { n: 1 } }
		  starts_a_program:
		    job: { script: ./add.js, input: { n: 10 }, allow: ["exec:git"] }
		output:
		  plain: ${stages.plain.output.n}
		  held: ${stages.starts_a_program.error}
	`, map[string]string{"add.js": scripts["add.js"], "ask.js": `return { approve: [], note: input.jobs.map(r => r.name).join() }`}, nil)
	check(t, r.Status, r.Output, "done", `{"plain": 2, "held": "the ask did not approve the job: starts_a_program"}`)
}

func TestAskThatIsAMachineWaitsForAPerson(t *testing.T) {
	l := local(t)
	id, err := l.Start(ctx, draft(t, `
		flow: careful
		ask:
		  machine:
		    statechart: person
		    initial: asking
		    states:
		      asking:
		        ticket: verdict
		        on:
		          verdict:
		            - { go: yes, when: "${event.data.ok}" }
		            - { go: no }
		      yes: { end: done, output: { approve: "${input.jobs.map(r => r.name)}" } }
		      no: { end: done, output: { approve: [], note: "${event.data.why}" } }
		stages:
		  deploy:
		    job: { script: ./add.js, input: { n: 41 }, approve: true }
		output: ${stages.deploy.output.n}
	`, map[string]string{"add.js": scripts["add.js"]}), startInput(nil))
	if err != nil {
		t.Fatal(err)
	}
	// The ask is a child, and its ticket is answered by naming the child.
	child, ticket := waitForTicket(t, l, id)
	if _, err := l.Reply(ctx, child, ticket, map[string]any{"ok": true}, 0); err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	check(t, r.Status, r.Output, "done", `42`)
}

func Test_mach_denial_says_why_the_ask_gave_no_yes(t *testing.T) {
	l := local(t)
	for ask, want := range map[string]string{
		`return { approve: [], note: "not today" }`: "the ask did not approve the job: not today",
		`throw new Error("no")`:                     "the ask failed, which approves nothing",
		`return "maybe"`:                            "the ask's result has no approve list",
	} {
		r := run(t, l, `
			flow: why
			ask: { script: ./ask.js }
			stages:
			  one:
			    job: { script: ./add.js, input: { n: 1 }, approve: true }
		`, map[string]string{"add.js": scripts["add.js"], "ask.js": ask}, nil)
		if r.Status != "failed" || r.Failure != "denied" || !strings.Contains(r.Error, want) {
			t.Fatalf("with the ask %q the flow ended %s/%s: %q, want %q", ask, r.Status, r.Failure, r.Error, want)
		}
	}
}

func Test_mach_every_refusal_is_recorded(t *testing.T) {
	l := local(t)
	id, err := l.Start(ctx, draft(t, `
		flow: several
		ask: { script: ./ask.js }
		stages:
		  one:   { job: { script: ./add.js, input: { n: 1 }, approve: true } }
		  two:   { job: { script: ./add.js, input: { n: 2 }, approve: true } }
		  three: { job: { script: ./add.js, input: { n: 3 }, approve: true } }
	`, map[string]string{"add.js": scripts["add.js"], "ask.js": `return { approve: [] }`}), startInput(nil))
	if err != nil {
		t.Fatal(err)
	}
	// Nothing reads a stage's failure, so the first denial fails the flow;
	// the other two are in the record all the same.
	denied := map[string]bool{}
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, arrived := range e.Events {
			if arrived.Name == machine.Failed {
				denied[arrived.From] = true
			}
		}
	})
	if err != nil || r.Status != "failed" || r.Failure != "denied" {
		t.Fatalf("the flow ended %+v, %v", r, err)
	}
	if !denied["one"] || !denied["two"] || !denied["three"] {
		t.Fatalf("the record holds the refusals of %v", denied)
	}
}

func Test_mach_inherited_ask_filled_from_declaring_machine(t *testing.T) {
	const def = `
		flow: declares
		input: { who: any, variable: any }
		ask: { script: ./ask.js, input: { who: "${input.who}" }, allow: ["env:${input.variable}"] }
		stages:
		  below:
		    job:
		      input: { who: child, variable: PATH }
		      machine:
		        flow: inherits
		        input: { who: any, variable: any }
		        stages:
		          work: { job: { script: ./WORK, input: { n: 1 }, approve: true } }
		        output: ${stages.work.output}
		output: ${stages.below.output}
	`
	files := map[string]string{
		"add.js":   scripts["add.js"],
		"sneak.js": `return env.get("HOME")`,
		// The ask reads the variable its own machine's input names, and
		// approves only if it was told who that machine was started for.
		"ask.js": `
			env.get("HOME")
			return { approve: input.who === "parent" ? input.jobs.map(r => r.name) : [], note: "asked for " + input.who }
		`,
	}
	input := map[string]any{"who": "parent", "variable": "HOME"}
	l := local(t, "all")
	// The ask, run for the child, has the input and the permissions the
	// parent's input gave it, and nothing from the child's.
	r := run(t, l, strings.Replace(def, "WORK", "add.js", 1), files, input)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
	// What the ask is allowed goes into the ask's job and into no other job
	// of the machine that inherits it.
	r = run(t, l, strings.Replace(def, "WORK", "sneak.js", 1), files, input)
	if r.Failure != "denied" || r.Refused != "env:HOME" {
		t.Fatalf("a job of the child that reads what only the ask may: %s/%s refused %q: %s", r.Status, r.Failure, r.Refused, r.Error)
	}
}
