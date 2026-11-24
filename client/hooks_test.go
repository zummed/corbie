package client_test

import (
	"strings"
	"testing"
)

func TestHookSaysYesToSomeOfAStepsJobs(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: approvals
		hooks:
		  small:
		    before: { script: ./small.js, input: { limit: 2, n: "${job.input.n}", machine: "${job.run.machine}" } }
		    when: "${job.approve}"
		stages:
		  items:
		    each: [1, 2, 3, 4]
		    job: { script: ./add.js, input: { n: "${item}" }, approve: true }
		output: "${stages.items.output.map(r => r.output ? r.output.n : r.error)}"
	`, map[string]string{
		"add.js": scripts["add.js"],
		"small.js": `
			if (input.machine !== "approvals") throw new Error("not told which machine the job is of")
			return { ok: input.n <= input.limit, note: "only small numbers" }
		`,
	}, nil)
	check(t, r.Status, r.Output, "done", `[2, 3, "the hook small refused it: only small numbers", "the hook small refused it: only small numbers"]`)
}

func TestDenialNobodyHandlesFailsTheMachineAsDenied(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: refused
		hooks: { never: { before: ./no.js } }
		stages:
		  one:
		    job: { script: ./add.js, input: { n: 1 }, approve: true }
	`, map[string]string{"add.js": scripts["add.js"], "no.js": `return { ok: false }`}, nil)
	if r.Status != "failed" || r.Failure != "denied" || !strings.Contains(r.Error, "one: the hook never refused it") {
		t.Fatalf("the flow ended %s/%s: %q", r.Status, r.Failure, r.Error)
	}
}

func TestHookThatFailsOrGivesNoAnswerRefuses(t *testing.T) {
	l := local(t)
	for hook, want := range map[string]string{
		`throw new Error("no")`: "the hook check failed: ",
		`return "maybe"`:        "the hook check gave neither { ok: true } nor { ok: false, note }",
		`return { ok: "yes" }`:  "the hook check gave neither { ok: true } nor { ok: false, note }",
	} {
		r := run(t, l, `
			flow: undecided
			hooks: { check: { before: ./hook.js, when: "${job.approve}" } }
			stages:
			  one:
			    job: { script: ./add.js, input: { n: 1 }, approve: true }
			  note:
			    when: ${stages.one.failed}
			    job: { script: ./echo.js, input: { why: "${stages.one.error}" } }
			output: ${stages.note.output.why}
		`, map[string]string{"add.js": scripts["add.js"], "echo.js": scripts["echo.js"], "hook.js": hook}, nil)
		if r.Status != "done" || !strings.Contains(r.Output.(string), want) {
			t.Fatalf("with the hook %q the flow ended %s: %v %s", hook, r.Status, r.Output, r.Error)
		}
	}
}

func TestAPermissionThatNeedsAYesHoldsTheJob(t *testing.T) {
	l := local(t, "all")
	r := run(t, l, `
		flow: needs-a-yes
		hooks: { never: { before: ./no.js, when: "${job.approve}" } }
		approve: [exec]
		stages:
		  plain:
		    job: { script: ./add.js, input: { n: 1 } }
		  starts_a_program:
		    job: { script: ./add.js, input: { n: 10 }, allow: ["exec:git"] }
		output:
		  plain: ${stages.plain.output.n}
		  held: ${stages.starts_a_program.error}
	`, map[string]string{"add.js": scripts["add.js"], "no.js": `return { ok: false, note: input.job.name }`}, nil)
	check(t, r.Status, r.Output, "done", `{"plain": 2, "held": "the hook never refused it: starts_a_program"}`)
}

func TestHookThatIsAMachineWaitsForAPerson(t *testing.T) {
	l := local(t)
	id, err := l.Run(ctx, draft(t, `
		flow: careful
		hooks:
		  person:
		    when: "${job.approve}"
		    before:
		      machine:
		        statechart: person
		        initial: asking
		        states:
		          asking:
		            ticket: verdict
		            on: { verdict: answered }
		          answered: { end: done, output: { ok: "${event.data.ok}", note: "${event.data.why}" } }
		stages:
		  deploy:
		    job: { script: ./add.js, input: { n: 41 }, approve: true }
		output: ${stages.deploy.output.n}
	`, map[string]string{"add.js": scripts["add.js"]}), startInput(nil))
	if err != nil {
		t.Fatal(err)
	}
	// The hook's job is a child, and its ticket is answered by naming the child.
	child, ticket := waitForTicket(t, l, id)
	if _, err := l.Reply(ctx, child, ticket, map[string]any{"ok": true}, 0); err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	check(t, r.Status, r.Output, "done", `42`)
}

func Test_mach_denial_says_why_a_hook_refused(t *testing.T) {
	l := local(t)
	for hook, want := range map[string]string{
		`return { ok: false, note: "not today" }`: "the hook check refused it: not today",
		`return { ok: false }`:                    "the hook check refused it",
		`throw new Error("no")`:                   "the hook check failed: ",
		`return "maybe"`:                          "the hook check gave neither { ok: true } nor { ok: false, note }",
	} {
		r := run(t, l, `
			flow: why
			hooks: { check: { before: ./hook.js } }
			stages:
			  one:
			    job: { script: ./add.js, input: { n: 1 }, approve: true }
		`, map[string]string{"add.js": scripts["add.js"], "hook.js": hook}, nil)
		if r.Status != "failed" || r.Failure != "denied" || !strings.Contains(r.Error, want) {
			t.Fatalf("with the hook %q the flow ended %s/%s: %q, want %q", hook, r.Status, r.Failure, r.Error, want)
		}
	}
}

func Test_mach_hook_from_above_filled_from_declaring_machine(t *testing.T) {
	const def = `
		flow: declares
		input: { who: any, variable: any }
		hooks:
		  check:
		    before: { script: ./check.js, input: { who: "${input.who}" }, allow: ["env:${input.variable}"] }
		    when: "${job.approve}"
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
		// The hook reads the variable its own machine's input names, and says
		// yes only if it was told who that machine was started for.
		"check.js": `
			env.get("HOME")
			return { ok: input.who === "parent", note: "asked for " + input.who }
		`,
	}
	input := map[string]any{"who": "parent", "variable": "HOME"}
	l := local(t, "all")
	// The hook, run for the child's job, has the input and the permissions
	// the parent's input gave it, and nothing from the child's.
	r := run(t, l, strings.Replace(def, "WORK", "add.js", 1), files, input)
	check(t, r.Status, r.Output, "done", `{"n": 2}`)
	// What the hook is allowed goes into the hook's job and into no other job
	// of the machine below.
	r = run(t, l, strings.Replace(def, "WORK", "sneak.js", 1), files, input)
	if r.Failure != "denied" || r.Refused != "env:HOME" {
		t.Fatalf("a job of the child that reads what only the hook may: %s/%s refused %q: %s", r.Status, r.Failure, r.Refused, r.Error)
	}
}

// A hook writes one of before: and after:, and its name is a name.
func Test_mach_a_hook_writes_before_or_after(t *testing.T) {
	for hooks, want := range map[string]string{
		`{ both: { before: ./add.js, after: ./add.js } }`:                                                    "hooks.both: a hook writes one of before: and after:",
		`{ neither: { when: "${job.approve}" } }`:                                                            "hooks.neither: a hook writes one of before: and after:",
		`{ "two words": { before: ./add.js } }`:                                                              `"two words" is not a name`,
		`{ reads: { before: ./add.js, when: "${output}" } }`:                                                 `reads "output"`,
		`{ fine: { after: { script: ./add.js, input: { n: "${output.n}" } }, when: "${job.name == 'a'}" } }`: "",
	} {
		_, err := machineParse("flow: f\nhooks: "+hooks+"\nstages:\n  a: { job: ./add.js }\n", map[string]string{"add.js": scripts["add.js"]})
		switch {
		case want == "" && err != nil:
			t.Errorf("%s was refused: %v", hooks, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s was not refused with %q: %v", hooks, want, err)
		}
	}
}

// A hook reads the meta: of a job its owner wrote, and none of a job of a
// machine made while running, which a script wrote.
func Test_mach_made_machine_gives_hooks_no_meta(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: top
		hooks:
		  see:
		    before: ./see.js
		    when: "${job.name == 'own' || job.name == 'theirs'}"
		stages:
		  own: { job: { script: ./one.js, meta: { team: mine } } }
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}" } }
		output: ${stages.go.output}
	`, map[string]string{
		"one.js": "return 1",
		"see.js": `
			const held = input.job
			const fits = held.name === "own" ? held.meta.team === "mine" : !("meta" in held)
			return { ok: fits, note: "the hook read the meta " + JSON.stringify(held.meta) }`,
		"make.js": `
			return machines.define(
				"flow: made\nstages:\n  theirs: { job: { script: ./step.js, meta: { team: mine } } }\noutput: ${stages.theirs.output}\n",
				{ files: { "./step.js": "return 7" } })`,
	}, nil)
	check(t, r.Status, r.Output, "done", `7`)
}
