package client_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
)

// helper is an agent with two parts, each in a file of its own beside a
// directory of parts.
func helper(address, extra string) string {
	return strings.ReplaceAll(strings.ReplaceAll(`
		agent: helper
		input: { question: any, so: "any?" }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		prompt: ${input.question}
		EXTRA
		tools:
		  add:
		    does: Add one to a number
		    args: { n: number }
		    job: { script: ./add.js, input: { n: "${args.n}" } }
		open:
		  net: { include: parts/net/part.yaml }
		  payments: { include: parts/payments/part.yaml }
	`, "ADDRESS", address), "EXTRA", extra)
}

var parts = map[string]string{
	"add.js": scripts["add.js"],
	"parts/net/part.yaml": `
		does: The firewall and the switch.
		meta: { people: [alice] }
		tools:
		  show:
		    does: Run one show command
		    args: { command: string }
		    job: { script: ./show.js, input: { command: "${args.command}" } }
		  files:
		    list: { script: ./list.js }
		    job: { script: ./list.js, input: { tool: "${tool}" } }
	`,
	"parts/net/show.js": `return "shown: " + input.command`,
	"parts/net/list.js": `return input?.tool ? "ran " + input.tool : [{ name: "read", does: "Read a file", args: { type: "object" } }]`,
	"parts/payments/part.yaml": `
		does: Payments.
		tools:
		  pay:
		    does: Pay someone
		    args: { to: string }
		    job: { script: ./pay.js, input: { to: "${args.to}" } }
	`,
	"parts/payments/pay.js": `return "paid " + input.to`,
}

// offeredNames gives the names of the tools one call to the model was
// offered.
func offeredNames(tools []any) string {
	var names []string
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	slices.Sort(names)
	return strings.Join(names, " ")
}

// openDoes gives what the tool that opens a part says, as the model is
// shown it.
func openDoes(tools []any) string {
	for _, tool := range tools {
		if t := tool.(map[string]any); t["name"] == "open" {
			return t["description"].(string)
		}
	}
	return ""
}

func startHelper(t *testing.T, l *client.Local, def string, input map[string]any) string {
	t.Helper()
	return startWith(t, l, def, parts, input)
}

func startWith(t *testing.T, l *client.Local, def string, files map[string]string, input map[string]any) string {
	t.Helper()
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err := l.Run(ctx, draft(t, def, files), client.Start{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The model is shown open, with the name and what it is for of each part it
// has not opened. Once it opens one, the part's tools are shown from the
// next turn, its own and those its lists give. A run that carries the
// conversation on is shown the past calls of the part's tools and has the part
// closed.
func TestAgentPartsAreOpenedByTheModel(t *testing.T) {
	m := newModel(t, calls("c1", "open", `{"part": "net"}`), calls("c2", "show", `{"command": "show route"}`),
		calls("c3", "files_read", `{}`), says("done"))
	l := local(t, "all")
	id := startHelper(t, l, helper(m.URL, ""), map[string]any{"question": "q"})
	var jobs []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if ask.Kind == "script" {
				jobs = append(jobs, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(jobs, " "); got != "files turn open turn#2 show turn#3 files_read turn#4" {
		t.Errorf("the jobs were: %s", got)
	}
	blobTools := "blob_code blob_read blob_search"
	if got := offeredNames(m.Calls[0].Tools); got != "add "+blobTools+" open" {
		t.Errorf("the first turn was offered: %s", got)
	}
	if got := openDoes(m.Calls[0].Tools); !strings.Contains(got, "- net: The firewall and the switch.") || !strings.Contains(got, "- payments: Payments.") {
		t.Errorf("open says: %s", got)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobTools+" files_read open show" {
		t.Errorf("the turn after the open was offered: %s", got)
	}
	if got := openDoes(m.Calls[1].Tools); strings.Contains(got, "net") || !strings.Contains(got, "payments") {
		t.Errorf("after the open, open says: %s", got)
	}
	if shown := strings.ReplaceAll(jsonOf(m.Calls[3]), `\`, ""); !strings.Contains(shown, "shown: show route") || !strings.Contains(shown, "ran read") {
		t.Errorf("the part's tools did not run their own scripts: %s", shown)
	}
	conversation := r.Output.(map[string]any)["conversation"].(map[string]any)
	if _, has := conversation["open"]; has {
		t.Errorf("the conversation names parts: %s", jsonOf(conversation))
	}
	m2 := newModel(t, says("again"))
	next := wait(t, l, startHelper(t, l, helper(m2.URL, "conversation: ${input.so}"), map[string]any{"question": "and?", "so": carried(conversation, r.ID)}))
	if next.Status != "done" {
		t.Fatalf("the run that carried on ended %s: %s", next.Status, next.Error)
	}
	if shown := jsonOf(m2.Calls[0]); !strings.Contains(shown, `"show"`) {
		t.Errorf("the run that carried on was not shown the past call: %s", shown)
	}
	if got := offeredNames(m2.Calls[0].Tools); got != "add "+blobTools+" open" {
		t.Errorf("the run that carried on was offered: %s", got)
	}
	if got := openDoes(m2.Calls[0].Tools); !strings.Contains(got, "- net:") {
		t.Errorf("for the run that carried on, open says: %s", got)
	}
}

// A part's tool's name is checked against every other tool's, in the
// agent, its files of tools and every part, when the definition is sent.
func TestAgentPartToolNamesAreTheirOwn(t *testing.T) {
	refused := func(def string, files map[string]string, want string) {
		t.Helper()
		_, err := machineParse(def, files)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want a refusal holding %q, got: %v", want, err)
		}
	}
	twice := map[string]string{"add.js": "", "parts/net/part.yaml": parts["parts/net/part.yaml"], "parts/net/show.js": "", "parts/net/list.js": "",
		"parts/payments/part.yaml": strings.Replace(parts["parts/payments/part.yaml"], "pay:", "show:", 1), "parts/payments/pay.js": ""}
	refused(helper("http://127.0.0.1:1", ""), twice, `open.payments.tools.show: the tool "show" has the name of another tool of the agent`)
	own := map[string]string{"add.js": "", "parts/net/part.yaml": parts["parts/net/part.yaml"], "parts/net/show.js": "", "parts/net/list.js": "",
		"parts/payments/part.yaml": strings.Replace(parts["parts/payments/part.yaml"], "pay:", "add:", 1), "parts/payments/pay.js": ""}
	refused(helper("http://127.0.0.1:1", ""), own, `open.payments.tools.add: the tool "add" has the name of another tool of the agent`)
	nameless := map[string]string{"add.js": "", "parts/net/part.yaml": "tools: {}\n", "parts/payments/part.yaml": "does: Payments.\ncolour: blue\n"}
	refused(helper("http://127.0.0.1:1", ""), nameless, "open.net.does: a part says what it is for")
	refused(helper("http://127.0.0.1:1", ""), nameless, `open.payments.colour: "colour" is not a field that belongs here`)
	// The stored definition holds each part's tools written out, with their
	// paths from the agent's directory.
	d := draft(t, helper("http://127.0.0.1:1", ""), parts)
	for _, want := range []string{`"./parts/net/show.js"`, `"./parts/payments/pay.js"`, `"part":"net"`, `"meta":{"people":["alice"]}`} {
		if !strings.Contains(string(d.Definition.Body), want) {
			t.Errorf("the stored definition does not hold %s: %s", want, d.Definition.Body)
		}
	}
}

// A part marked approve: true holds its open for the ask, which is shown
// the part's name and what it is for, and the ask for a call of a part's
// tool is shown the part's name. An open not approved goes back to the
// model as denied, with the ask's note, and opens nothing.
func TestAgentPartOpenWaitsForTheAsk(t *testing.T) {
	def := strings.Replace(helper("ADDR", ""), "net: { include: parts/net/part.yaml }", "net: { include: parts/net/part.yaml, approve: true }", 1)
	def = strings.Replace(def, "\t\ttools:\n", "\t\task: ./ask.js\n\t\ttools:\n", 1)
	files := map[string]string{}
	for k, v := range parts {
		files[k] = v
	}
	files["parts/payments/part.yaml"] = strings.Replace(parts["parts/payments/part.yaml"], `input: { to: "${args.to}" } }`, `input: { to: "${args.to}" }, approve: true }`, 1)
	files["ask.js"] = `
		const fits = input.jobs.filter((j) => j.name === "open" ? j.asked.part === "net" && j.asked.does === "The firewall and the switch." : j.asked.part === "payments")
		return { approve: fits.map((j) => j.name), note: "the ask was shown " + JSON.stringify(input.jobs.map((j) => j.asked)) }`
	m := newModel(t, calls("c1", "open", `{"part": "net"}`), calls("c2", "open", `{"part": "payments"}`), calls("c3", "pay", `{"to": "me"}`), says("done"))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err := l.Run(ctx, draft(t, strings.Replace(def, "ADDR", m.URL, 1), files), client.Start{
		Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	for i, call := range m.Calls[1:] {
		if shown := lastShown(call); strings.Contains(shown, "denied") {
			t.Errorf("turn %d was shown: %s", i+2, shown)
		}
	}
	// An ask that approves nothing leaves the part closed.
	files["ask.js"] = `return { approve: [], note: "not today" }`
	m = newModel(t, calls("c1", "open", `{"part": "net"}`), says("done"))
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err = l.Run(ctx, draft(t, strings.Replace(def, "ADDR", m.URL, 1), files), client.Start{
		Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	if shown := lastShown(m.Calls[1]); r.Status != "done" || !strings.Contains(shown, "denied") || !strings.Contains(shown, "not today") {
		t.Errorf("the agent ended %s, and the open was shown as: %s", r.Status, shown)
	}
	if got := offeredNames(m.Calls[1].Tools); strings.Contains(got, "show") {
		t.Errorf("an open not approved opened the part: %s", got)
	}
	// A part marked for approval needs an ask.
	if _, err := machineParse(strings.Replace(strings.Replace(def, "\t\task: ./ask.js\n", "", 1), "ADDR", "http://127.0.0.1:1", 1), parts); err == nil ||
		!strings.Contains(err.Error(), "there is no ask for it to go to") {
		t.Errorf("a part marked for approval with no ask: %v", err)
	}
}

// A part's ask is the ask for opening the part and for the jobs of its
// tools, before the agent's own. It is read from the part's directory,
// filled from the agent's input, and its permissions are its own. Calls
// in one turn that go to different asks start each ask once.
func TestAgentPartHasItsOwnAsk(t *testing.T) {
	def := strings.Replace(helper("ADDR", ""), "net: { include: parts/net/part.yaml }", "net: { include: parts/net/part.yaml, approve: true }", 1)
	def = strings.Replace(def, "\t\ttools:\n", "\t\task: ./ask.js\n\t\ttools:\n", 1)
	files := map[string]string{}
	for k, v := range parts {
		files[k] = v
	}
	files["parts/net/part.yaml"] = strings.Replace(parts["parts/net/part.yaml"], "tools:\n", `ask: { script: ./ask.js, input: { who: "${input.question}" }, allow: [env:HOME] }
		tools:
		  sneak:
		    does: Read HOME
		    job: { script: ./sneak.js }
`, 1)
	files["parts/net/part.yaml"] = strings.Replace(files["parts/net/part.yaml"], `input: { command: "${args.command}" } }`, `input: { command: "${args.command}" }, approve: true }`, 1)
	files["parts/net/sneak.js"] = `return env.get("HOME")`
	files["parts/net/ask.js"] = `
		env.get("HOME")
		const ok = input.who === "q" && input.jobs.every((j) => j.asked.part === "net")
		return { approve: ok ? input.jobs.map((j) => j.name) : [], note: "the part's ask" }`
	def = strings.Replace(def, `input: { n: "${args.n}" } }`, `input: { n: "${args.n}" }, approve: true }`, 1)
	files["ask.js"] = `return { approve: input.jobs.filter((j) => j.name === "add").map((j) => j.name), note: "the agent's ask" }`
	openAndAdd := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c1", Name: "open", Args: `{"part": "net"}`}, {ID: "c2", Name: "add", Args: `{"n": 1}`}}}
	showAndSneak := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c3", Name: "show", Args: `{"command": "show route"}`}, {ID: "c4", Name: "sneak", Args: `{}`}}}
	m := newModel(t, openAndAdd, showAndSneak, says("done"))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err := l.Run(ctx, draft(t, strings.Replace(def, "ADDR", m.URL, 1), files), client.Start{
		Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	var asks []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if ask.Kind == "script" && strings.HasPrefix(ask.Name, "ask") {
				asks = append(asks, fmt.Sprintf("%d:%s", e.Step, ask.Name))
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	// The open goes to the part's ask and the call of add to the agent's,
	// each started once, in the same step.
	if len(asks) != 3 || !strings.HasSuffix(asks[0], ":ask") || asks[1] != strings.TrimSuffix(asks[0], ":ask")+":ask#2" || !strings.HasSuffix(asks[2], ":ask#3") {
		t.Errorf("the asks were: %v", asks)
	}
	if shown := lastShown(m.Calls[1]); strings.Contains(shown, "denied") {
		t.Errorf("the second turn was shown: %s", shown)
	}
	// What the part's ask is allowed goes into the ask's job and into no
	// tool's.
	if shown := lastShown(m.Calls[2]); strings.Count(shown, "denied") != 1 || !strings.Contains(shown, "shown: show route") || !strings.Contains(shown, "env:HOME") {
		t.Errorf("the last turn was shown: %s", shown)
	}
	// A part that has an ask needs none from the agent, and a machine run as
	// a part's ask takes none from above it.
	alone := strings.Replace(strings.Replace(strings.Replace(def, "\t\task: ./ask.js\n", "", 1), ", approve: true }", " }", 1), "ADDR", "http://127.0.0.1:1", 1)
	if _, err := machineParse(alone, files); err != nil {
		t.Errorf("an agent whose part has its own ask was refused: %v", err)
	}
	files["parts/net/part.yaml"] = strings.Replace(files["parts/net/part.yaml"], "ask: { script: ./ask.js,", "ask: { machine: ./approval.yaml,", 1)
	files["parts/net/approval.yaml"] = "flow: approval\nstages:\n  a: { job: { script: ./ask.js, approve: true } }\noutput: ${stages.a.output}\n"
	if _, err := machineParse(alone, files); err == nil || !strings.Contains(err.Error(), "open.net.ask: the machine run as the ask marks a job for approval") {
		t.Errorf("a part's ask that needs an ask: %v", err)
	}
}

// A conversation is input, and opens no part, whatever it names.
func TestAgentConversationOpensNoPart(t *testing.T) {
	m := newModel(t, says("done"))
	l := local(t, "all")
	so := map[string]any{"turns": []any{}, "blobs": map[string]any{}, "open": []any{"net", "payments"}}
	r := wait(t, l, startHelper(t, l, helper(m.URL, "conversation: ${input.so}"), map[string]any{"question": "q", "so": so}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if got := offeredNames(m.Calls[0].Tools); got != "add blob_code blob_read blob_search open" {
		t.Errorf("the first turn was offered: %s", got)
	}
	if got := openDoes(m.Calls[0].Tools); !strings.Contains(got, "- net:") || !strings.Contains(got, "- payments:") {
		t.Errorf("open says: %s", got)
	}
}

// netAsking gives the files of the helper with net's tool show marked for
// approval and net naming an ask whose input is the value given.
func netAsking(askInput string) map[string]string {
	files := map[string]string{}
	for k, v := range parts {
		files[k] = v
	}
	net := strings.Replace(parts["parts/net/part.yaml"], "tools:\n", "ask: { script: ./ask.js, input: "+askInput+" }\n\t\ttools:\n", 1)
	files["parts/net/part.yaml"] = strings.Replace(net, `input: { command: "${args.command}" } }`, `input: { command: "${args.command}" }, approve: true }`, 1)
	files["parts/net/ask.js"] = `return { approve: input.jobs.map((j) => j.name), note: "net's ask" }`
	return files
}

// The held jobs of one step that go to one ask start it once, though
// filling its values in twice would give two different values.
func Test_mach_one_run_of_each_ask_for_a_step(t *testing.T) {
	files := netAsking(`{ r: "${Math.random()}" }`)
	twice := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c1", Name: "show", Args: `{"command": "a"}`}, {ID: "c2", Name: "show", Args: `{"command": "b"}`}}}
	m := newModel(t, calls("c0", "open", `{"part": "net"}`), twice, says("done"))
	l := local(t, "all")
	id := startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"})
	var asks []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if ask.Kind == "script" && strings.HasPrefix(ask.Name, "ask") {
				asks = append(asks, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(asks, " "); got != "ask" {
		t.Errorf("the asks were: %s", got)
	}
	if shown := lastShown(m.Calls[2]); strings.Contains(shown, "denied") || !strings.Contains(shown, "shown: a") || !strings.Contains(shown, "shown: b") {
		t.Errorf("the last turn was shown: %s", shown)
	}
}

// A part's ask whose values cannot be filled in denies the jobs held for
// it, with a note that names the fault, as a definition's ask does, and
// the agent carries on.
func Test_agent_part_ask_that_cannot_be_filled_denies(t *testing.T) {
	files := netAsking(`{ who: "${input.question.nothing.here}" }`)
	m := newModel(t, calls("c0", "open", `{"part": "net"}`), calls("c1", "show", `{"command": "a"}`), says("done"))
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "denied: the ask could not be started: open.net.ask.input") || strings.Contains(shown, "shown: a") {
		t.Errorf("the call was shown as: %s", shown)
	}
}

// A part's ask is the nearest ask above every machine below its tools'
// jobs, before the agent's, with its own values and permissions, filled
// from the agent's input; a machine below that names an ask of its own
// uses its own. An agent with no ask of its own is accepted when the
// part's ask covers it.
func Test_agent_part_ask_is_nearest_above_its_tools_machines(t *testing.T) {
	def := strings.Replace(helper("ADDR", "ask: ./yes.js"), "payments: { include: parts/payments/part.yaml }", "", 1)
	files := map[string]string{"add.js": scripts["add.js"], "yes.js": `return { approve: input.jobs.map((j) => j.name) }`}
	files["parts/net/part.yaml"] = `
		does: The firewall and the switch.
		ask: { script: ./ask.js, input: { who: "${input.question}" }, allow: [env:HOME] }
		tools:
		  deep:
		    does: Do deep work
		    job: { machine: ./outer.yaml }
	`
	files["parts/net/ask.js"] = `
		env.get("HOME")
		return { approve: input.who === "q" ? input.jobs.map((j) => j.name) : [], note: "net's ask, for " + input.asking.machine }`
	files["parts/net/outer.yaml"] = "flow: outer\nstages:\n  in: { job: { machine: ./inner.yaml } }\noutput: ${stages.in.output}\n"
	files["parts/net/inner.yaml"] = "flow: inner\nstages:\n  work: { job: { script: ./work.js, approve: true } }\noutput: ${stages.work.output}\n"
	files["parts/net/work.js"] = `return "worked"`
	files["parts/net/own.js"] = `return { approve: [], note: "inner's own ask" }`
	l := local(t, "all")
	for _, run := range []struct {
		question, inner, want string
	}{
		{"q", "", "worked"},
		{"other", "", "net's ask, for inner"},
		{"q", "ask: ./own.js\n", "inner's own ask"},
	} {
		files["parts/net/inner.yaml"] = strings.Replace(files["parts/net/inner.yaml"], "flow: inner\n", "flow: inner\n"+run.inner, 1)
		m := newModel(t, calls("c0", "open", `{"part": "net"}`), calls("c1", "deep", `{}`), says("done"))
		r := wait(t, l, startWith(t, l, strings.Replace(def, "ADDR", m.URL, 1), files, map[string]any{"question": run.question}))
		if r.Status != "done" {
			t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
		}
		if shown := lastShown(m.Calls[2]); !strings.Contains(shown, run.want) {
			t.Errorf("asked %q with %q in inner, the call was shown as: %s", run.question, run.inner, shown)
		}
		files["parts/net/inner.yaml"] = strings.Replace(files["parts/net/inner.yaml"], run.inner, "", 1)
	}
	// The part's ask covers inner's job, and the agent needs no ask of its
	// own; without the part's ask, it does.
	def = strings.Replace(strings.Replace(def, "\t\task: ./yes.js\n", "", 1), "ADDR", "http://127.0.0.1:1", 1)
	if _, err := machineParse(def, files); err != nil {
		t.Errorf("an agent whose part's ask covers its tool's machine was refused: %v", err)
	}
	files["parts/net/part.yaml"] = strings.Replace(files["parts/net/part.yaml"], "ask:", "meta:", 1)
	if _, err := machineParse(def, files); err == nil || !strings.Contains(err.Error(), "there is no ask for it to go to") {
		t.Errorf("a part's tool whose machine holds a job, with no ask: %v", err)
	}
}

// A part's ask was not made while running, and it is the nearest above a
// machine made while running below one of the part's tools.
func Test_agent_part_ask_is_reached_from_a_made_machine_below_its_tool(t *testing.T) {
	m := newModel(t, calls("c0", "open", `{"part": "net"}`), calls("c1", "deep", `{}`), says("done"))
	def := `
		flow: top
		stages:
		  make: { job: ./make.js }
		  go:
		    job:
		      machine: ./helper.yaml
		      input: { question: q, made: "${stages.make.output}" }
	`
	files := map[string]string{"add.js": scripts["add.js"]}
	files["helper.yaml"] = strings.Replace(strings.Replace(helper(m.URL, ""), "payments: { include: parts/payments/part.yaml }", "", 1), "../parts", "./parts", 1)
	files["make.js"] = `return machines.define("flow: made\nstages:\n  work: { job: { script: ./step.js, approve: true } }\n",
		{ files: { "./step.js": "return 7" } })`
	files["parts/net/part.yaml"] = `
		does: The firewall and the switch.
		ask: ./ask.js
		tools:
		  deep:
		    does: Do deep work
		    job: { machine: "${input.made}" }
	`
	files["parts/net/ask.js"] = `return { approve: [], note: "net's ask, for " + input.asking.machine }`
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, def, files, nil))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "net's ask, for made") {
		t.Errorf("the call was shown as: %s", shown)
	}
}

// A tool's job in an agent made while running goes to the ask above the agent.
func Test_agent_made_while_running_has_no_ask_of_its_own(t *testing.T) {
	m := newModel(t, calls("c1", "act", `{}`), says("done"))
	made := strings.ReplaceAll(`agent: made
model: { name: test-model, service: anthropic, address: "ADDRESS", secret: TEST_KEY }
prompt: go
tools:
  act: { does: Act, job: { script: ./step.js, approve: true } }
`, "ADDRESS", m.URL)
	def := `
		flow: top
		ask: ./human.js
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}", allow: [ all ] } }
	`
	files := map[string]string{
		"human.js": `return { approve: [], note: "the owner's ask, for " + input.asking.machine }`,
		"make.js": "return machines.define(`" + made + "`, " +
			`{ files: { "./step.js": "return 7" } })`,
	}
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, def, files, nil))
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "the owner's ask, for made") {
		t.Errorf("the call was shown as: %s", shown)
	}
}

// The list of an entry in a part goes to the part's ask when it is held,
// and the ask is shown the part's name, as for a call of the part's tools:
// with no ask of the agent's, and with one, which is not asked.
func Test_agent_part_list_goes_to_the_parts_ask(t *testing.T) {
	files := map[string]string{}
	for k, v := range parts {
		files[k] = v
	}
	net := strings.Replace(parts["parts/net/part.yaml"], "tools:\n", "ask: ./ask.js\n\t\ttools:\n", 1)
	files["parts/net/part.yaml"] = strings.Replace(net, "list: { script: ./list.js }", "list: { script: ./list.js, approve: true }", 1)
	files["parts/net/ask.js"] = `return { approve: input.jobs.filter((j) => j.name === "files" && j.asked.part === "net").map((j) => j.name) }`
	files["no.js"] = `return { approve: [], note: "the agent's ask" }`
	for _, extra := range []string{"", "ask: ./no.js"} {
		m := newModel(t, calls("c1", "open", `{"part": "net"}`), calls("c2", "files_read", `{}`), says("done"))
		l := local(t, "all")
		id := startWith(t, l, helper(m.URL, extra), files, map[string]any{"question": "q"})
		var held, asks []string
		r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
			for _, ask := range e.Asks {
				switch {
				case ask.Kind == "hold":
					held = append(held, ask.Name)
				case ask.Kind == "script" && strings.HasPrefix(ask.Name, "ask"):
					asks = append(asks, ask.Name)
				}
			}
		})
		if err != nil || r.Status != "done" {
			t.Fatalf("with %q, the agent ended %+v, %v", extra, r, err)
		}
		if got := strings.Join(held, " ") + " / " + strings.Join(asks, " "); got != "files / ask" {
			t.Errorf("with %q, the jobs held, and the asks: %s", extra, got)
		}
		if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "ran read") {
			t.Errorf("with %q, the listed tool's call was shown as: %s", extra, shown)
		}
	}
}

// An error names the list of an entry in a part by where it is written,
// the place the check names it by.
func Test_agent_list_is_named_by_its_place(t *testing.T) {
	for list, want := range map[string]string{
		`throw new Error("no server")`: "open.net.tools.files.list: no server",
		`return "nothing"`:             "open.net.tools.files.list: the list gave no list of tools",
	} {
		files := map[string]string{}
		for k, v := range parts {
			files[k] = v
		}
		files["parts/net/list.js"] = list
		m := newModel(t)
		l := local(t, "all")
		r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"}))
		if r.Status != "failed" || !strings.Contains(r.Error, want) {
			t.Errorf("with a list that does %s, the agent ended %s: %s", list, r.Status, r.Error)
		}
	}
}

// following is what a run did: the script jobs it started, the jobs it
// held, and its asks, each ask with the step that started it.
type following struct {
	run         *client.Run
	jobs, held  []string
	asks, steps []string
}

func follow(t *testing.T, l *client.Local, id string) following {
	t.Helper()
	var f following
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			switch {
			case ask.Kind == "hold":
				f.held = append(f.held, ask.Name)
			case ask.Kind == "script" && strings.HasPrefix(ask.Name, "ask"):
				f.asks = append(f.asks, ask.Name)
				f.steps = append(f.steps, fmt.Sprint(e.Step))
			case ask.Kind == "script":
				f.jobs = append(f.jobs, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	f.run = r
	return f
}
