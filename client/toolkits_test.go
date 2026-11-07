package client_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
)

// helper is an agent with two toolkits, each in a file of its own beside a
// directory of toolkits.
func helper(address, extra string) string {
	return strings.ReplaceAll(strings.ReplaceAll(`
		agent: helper
		input: { question: any, so: "any?" }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		prompt: ${input.question}
		EXTRA
		tools:
		  add:
		    description: Add one to a number
		    args: { n: number }
		    job: { script: ./add.js, input: { n: "${args.n}" } }
		toolkits:
		  net: { include: toolkits/net/net.yaml }
		  payments: { include: toolkits/payments/payments.yaml }
	`, "ADDRESS", address), "EXTRA", extra)
}

var toolkits = map[string]string{
	"add.js": scripts["add.js"],
	"toolkits/net/net.yaml": `
		description: The firewall and the switch.
		meta: { people: [alice] }
		tools:
		  show:
		    description: Run one show command
		    args: { command: string }
		    job: { script: ./show.js, input: { command: "${args.command}" } }
		  files:
		    list: { script: ./list.js }
		    job: { script: ./list.js, input: { tool: "${tool}" } }
	`,
	"toolkits/net/show.js": `return "shown: " + input.command`,
	"toolkits/net/list.js": `return input?.tool ? "ran " + input.tool : [{ name: "read", description: "Read a file", args: { type: "object" } }]`,
	"toolkits/payments/payments.yaml": `
		description: Payments.
		tools:
		  pay:
		    description: Pay someone
		    args: { to: string }
		    job: { script: ./pay.js, input: { to: "${args.to}" } }
	`,
	"toolkits/payments/pay.js": `return "paid " + input.to`,
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

// openDescription gives what the tool that opens a toolkit says, as the model is
// shown it.
func openDescription(tools []any) string {
	for _, tool := range tools {
		if t := tool.(map[string]any); t["name"] == "open" {
			return t["description"].(string)
		}
	}
	return ""
}

func startHelper(t *testing.T, l *client.Local, def string, input map[string]any) string {
	t.Helper()
	return startWith(t, l, def, toolkits, input)
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

// The model is shown open, with the name and what it is for of each toolkit it
// has not opened. Once it opens one, the toolkit's tools are shown from the
// next turn, its own and those its lists give. A run that carries the
// conversation on is shown the past calls of the toolkit's tools and has the toolkit
// closed.
func TestAgentToolkitsAreOpenedByTheModel(t *testing.T) {
	m := newModel(t, calls("c1", "open", `{"toolkit": "net"}`), calls("c2", "show", `{"command": "show route"}`),
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
	if got := openDescription(m.Calls[0].Tools); !strings.Contains(got, "- net: The firewall and the switch.") || !strings.Contains(got, "- payments: Payments.") {
		t.Errorf("open says: %s", got)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobTools+" files_read open show" {
		t.Errorf("the turn after the open was offered: %s", got)
	}
	if got := openDescription(m.Calls[1].Tools); strings.Contains(got, "net") || !strings.Contains(got, "payments") {
		t.Errorf("after the open, open says: %s", got)
	}
	if shown := strings.ReplaceAll(jsonOf(m.Calls[3]), `\`, ""); !strings.Contains(shown, "shown: show route") || !strings.Contains(shown, "ran read") {
		t.Errorf("the toolkit's tools did not run their own scripts: %s", shown)
	}
	conversation := r.Output.(map[string]any)["conversation"].(map[string]any)
	if _, has := conversation["opened"]; has {
		t.Errorf("the conversation names toolkits: %s", jsonOf(conversation))
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
	if got := openDescription(m2.Calls[0].Tools); !strings.Contains(got, "- net:") {
		t.Errorf("for the run that carried on, open says: %s", got)
	}
}

// A toolkit's tool's name is checked against every other tool's, in the
// agent, its files of tools and every toolkit, when the definition is sent.
func TestAgentToolkitToolNamesAreTheirOwn(t *testing.T) {
	refused := func(def string, files map[string]string, want string) {
		t.Helper()
		_, err := machineParse(def, files)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want a refusal holding %q, got: %v", want, err)
		}
	}
	twice := map[string]string{"add.js": "", "toolkits/net/net.yaml": toolkits["toolkits/net/net.yaml"], "toolkits/net/show.js": "", "toolkits/net/list.js": "",
		"toolkits/payments/payments.yaml": strings.Replace(toolkits["toolkits/payments/payments.yaml"], "pay:", "show:", 1), "toolkits/payments/pay.js": ""}
	refused(helper("http://127.0.0.1:1", ""), twice, `toolkits.payments.tools.show: the tool "show" has the name of another tool of the agent`)
	own := map[string]string{"add.js": "", "toolkits/net/net.yaml": toolkits["toolkits/net/net.yaml"], "toolkits/net/show.js": "", "toolkits/net/list.js": "",
		"toolkits/payments/payments.yaml": strings.Replace(toolkits["toolkits/payments/payments.yaml"], "pay:", "add:", 1), "toolkits/payments/pay.js": ""}
	refused(helper("http://127.0.0.1:1", ""), own, `toolkits.payments.tools.add: the tool "add" has the name of another tool of the agent`)
	nameless := map[string]string{"add.js": "", "toolkits/net/net.yaml": "tools: {}\n", "toolkits/payments/payments.yaml": "description: Payments.\ncolour: blue\n"}
	refused(helper("http://127.0.0.1:1", ""), nameless, "toolkits.net.description: a toolkit says what it is for")
	refused(helper("http://127.0.0.1:1", ""), nameless, `toolkits.payments.colour: "colour" is not a field that belongs here`)
	// A description is shown to the model as written, so it holds no
	// expression.
	filled := map[string]string{"add.js": "", "toolkits/net/net.yaml": "description: ${input.question}\ntools: {}\n",
		"toolkits/payments/payments.yaml": strings.Replace(toolkits["toolkits/payments/payments.yaml"], "Pay someone", "Pay ${input.so}", 1), "toolkits/payments/pay.js": ""}
	refused(helper("http://127.0.0.1:1", ""), filled, "toolkits.net.description: a description is plain text, with no expression")
	refused(helper("http://127.0.0.1:1", ""), filled, "toolkits.payments.tools.pay.description: a description is plain text, with no expression")
	// The stored definition holds each toolkit's tools written out, with their
	// paths from the agent's directory.
	d := draft(t, helper("http://127.0.0.1:1", ""), toolkits)
	for _, want := range []string{`"./toolkits/net/show.js"`, `"./toolkits/payments/pay.js"`, `"toolkit":"net"`, `"meta":{"people":["alice"]}`} {
		if !strings.Contains(string(d.Definition.Body), want) {
			t.Errorf("the stored definition does not hold %s: %s", want, d.Definition.Body)
		}
	}
}

// A toolkit marked approve: true holds its open for the ask, which is shown
// the toolkit's name and what it is for, and the ask for a call of a toolkit's
// tool is shown the toolkit's name. An open not approved goes back to the
// model as denied, with the ask's note, and opens nothing.
func TestAgentToolkitOpenWaitsForTheAsk(t *testing.T) {
	def := strings.Replace(helper("ADDR", ""), "net: { include: toolkits/net/net.yaml }", "net: { include: toolkits/net/net.yaml, approve: true }", 1)
	def = strings.Replace(def, "\t\ttools:\n", "\t\task: ./ask.js\n\t\ttools:\n", 1)
	files := map[string]string{}
	for k, v := range toolkits {
		files[k] = v
	}
	files["toolkits/payments/payments.yaml"] = strings.Replace(toolkits["toolkits/payments/payments.yaml"], `input: { to: "${args.to}" } }`, `input: { to: "${args.to}" }, approve: true }`, 1)
	files["ask.js"] = `
		const fits = input.jobs.filter((j) => j.name === "open" ? j.asked.toolkit === "net" && j.asked.description === "The firewall and the switch." : j.asked.toolkit === "payments")
		return { approved: fits.map((j) => j.name), note: "the ask was shown " + JSON.stringify(input.jobs.map((j) => j.asked)) }`
	m := newModel(t, calls("c1", "open", `{"toolkit": "net"}`), calls("c2", "open", `{"toolkit": "payments"}`), calls("c3", "pay", `{"to": "me"}`), says("done"))
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
	// An ask that approves nothing leaves the toolkit closed.
	files["ask.js"] = `return { approved: [], note: "not today" }`
	m = newModel(t, calls("c1", "open", `{"toolkit": "net"}`), says("done"))
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
		t.Errorf("an open not approved opened the toolkit: %s", got)
	}
	// A toolkit marked for approval needs an ask.
	if _, err := machineParse(strings.Replace(strings.Replace(def, "\t\task: ./ask.js\n", "", 1), "ADDR", "http://127.0.0.1:1", 1), toolkits); err == nil ||
		!strings.Contains(err.Error(), "there is no ask for it to go to") {
		t.Errorf("a toolkit marked for approval with no ask: %v", err)
	}
}

// A toolkit's ask is the ask for opening the toolkit and for the jobs of its
// tools, before the agent's own. It is read from the toolkit's directory,
// filled from the agent's input, and its permissions are its own. Calls
// in one turn that go to different asks start each ask once.
func TestAgentToolkitHasItsOwnAsk(t *testing.T) {
	def := strings.Replace(helper("ADDR", ""), "net: { include: toolkits/net/net.yaml }", "net: { include: toolkits/net/net.yaml, approve: true }", 1)
	def = strings.Replace(def, "\t\ttools:\n", "\t\task: ./ask.js\n\t\ttools:\n", 1)
	files := map[string]string{}
	for k, v := range toolkits {
		files[k] = v
	}
	files["toolkits/net/net.yaml"] = strings.Replace(toolkits["toolkits/net/net.yaml"], "tools:\n", `ask: { script: ./ask.js, input: { who: "${input.question}" }, allow: [env:HOME] }
		tools:
		  sneak:
		    description: Read HOME
		    job: { script: ./sneak.js }
`, 1)
	files["toolkits/net/net.yaml"] = strings.Replace(files["toolkits/net/net.yaml"], `input: { command: "${args.command}" } }`, `input: { command: "${args.command}" }, approve: true }`, 1)
	files["toolkits/net/sneak.js"] = `return env.get("HOME")`
	files["toolkits/net/ask.js"] = `
		env.get("HOME")
		const ok = input.who === "q" && input.jobs.every((j) => j.asked.toolkit === "net")
		return { approved: ok ? input.jobs.map((j) => j.name) : [], note: "the toolkit's ask" }`
	def = strings.Replace(def, `input: { n: "${args.n}" } }`, `input: { n: "${args.n}" }, approve: true }`, 1)
	files["ask.js"] = `return { approved: input.jobs.filter((j) => j.name === "add").map((j) => j.name), note: "the agent's ask" }`
	openAndAdd := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c1", Name: "open", Args: `{"toolkit": "net"}`}, {ID: "c2", Name: "add", Args: `{"n": 1}`}}}
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
	// The open goes to the toolkit's ask and the call of add to the agent's,
	// each started once, in the same step.
	if len(asks) != 3 || !strings.HasSuffix(asks[0], ":ask") || asks[1] != strings.TrimSuffix(asks[0], ":ask")+":ask#2" || !strings.HasSuffix(asks[2], ":ask#3") {
		t.Errorf("the asks were: %v", asks)
	}
	if shown := lastShown(m.Calls[1]); strings.Contains(shown, "denied") {
		t.Errorf("the second turn was shown: %s", shown)
	}
	// What the toolkit's ask is allowed goes into the ask's job and into no
	// tool's.
	if shown := lastShown(m.Calls[2]); strings.Count(shown, "denied") != 1 || !strings.Contains(shown, "shown: show route") || !strings.Contains(shown, "env:HOME") {
		t.Errorf("the last turn was shown: %s", shown)
	}
	// A toolkit that has an ask needs none from the agent, and a machine run as
	// a toolkit's ask takes none from above it.
	alone := strings.Replace(strings.Replace(strings.Replace(def, "\t\task: ./ask.js\n", "", 1), ", approve: true }", " }", 1), "ADDR", "http://127.0.0.1:1", 1)
	if _, err := machineParse(alone, files); err != nil {
		t.Errorf("an agent whose toolkit has its own ask was refused: %v", err)
	}
	files["toolkits/net/net.yaml"] = strings.Replace(files["toolkits/net/net.yaml"], "ask: { script: ./ask.js,", "ask: { machine: ./approval.yaml,", 1)
	files["toolkits/net/approval.yaml"] = "flow: approval\nstages:\n  a: { job: { script: ./ask.js, approve: true } }\noutput: ${stages.a.output}\n"
	if _, err := machineParse(alone, files); err == nil || !strings.Contains(err.Error(), "toolkits.net.ask: the machine run as the ask marks a job for approval") {
		t.Errorf("a toolkit's ask that needs an ask: %v", err)
	}
}

// A toolkit or a tool whose when: does not hold when the run starts is not
// offered: open does not name the toolkit, its lists do not run, and a call
// of either goes back to the model as an error. Each run checks its own
// input, a run that carries a conversation on as well.
func TestAgentWhenSaysWhatIsOffered(t *testing.T) {
	def := func(address, extra string) string {
		d := strings.Replace(helper(address, extra), `so: "any?" }`, `so: "any?", scopes: "any?" }`, 1)
		d = strings.Replace(d, "payments: { include: toolkits/payments/payments.yaml }",
			`payments: { include: toolkits/payments/payments.yaml, when: "${(input.scopes || []).includes('payments')}" }`, 1)
		return strings.Replace(d, "job: { script: ./add.js,", "when: \"${input.question !== 'no add'}\"\n\t\t    job: { script: ./add.js,", 1)
	}
	blobTools := "blob_code blob_read blob_search"
	l := local(t, "all")
	m := newModel(t, calls("c1", "open", `{"toolkit": "payments"}`), says("done"))
	r := wait(t, l, startHelper(t, l, def(m.URL, ""), map[string]any{"question": "q", "scopes": []any{}}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if got := openDescription(m.Calls[0].Tools); !strings.Contains(got, "- net:") || strings.Contains(got, "payments") {
		t.Errorf("without the scope, open says: %s", got)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "the arguments do not fit") {
		t.Errorf("an open of a toolkit not offered was shown as: %s", shown)
	}
	m = newModel(t, calls("c1", "open", `{"toolkit": "payments"}`), says("done"))
	r = wait(t, l, startHelper(t, l, def(m.URL, ""), map[string]any{"question": "q", "scopes": []any{"payments"}}))
	if r.Status != "done" {
		t.Fatalf("with the scope, the agent ended %s: %s", r.Status, r.Error)
	}
	if got := openDescription(m.Calls[0].Tools); !strings.Contains(got, "- net:") || !strings.Contains(got, "- payments:") {
		t.Errorf("with the scope, open says: %s", got)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobTools+" open pay" {
		t.Errorf("with the scope, after the open, the turn was offered: %s", got)
	}
	// A run that carries that conversation on checks its own input.
	conversation := r.Output.(map[string]any)["conversation"]
	m2 := newModel(t, says("again"))
	next := wait(t, l, startHelper(t, l, def(m2.URL, "conversation: ${input.so}"), map[string]any{"question": "and?", "scopes": []any{}, "so": carried(conversation, r.ID)}))
	if next.Status != "done" {
		t.Fatalf("the run that carried on ended %s: %s", next.Status, next.Error)
	}
	if got := openDescription(m2.Calls[0].Tools); strings.Contains(got, "payments") {
		t.Errorf("the run that carried on, without the scope, was offered: %s", got)
	}
	// A tool not offered is not shown, and a call of it runs nothing.
	m = newModel(t, calls("c1", "add", `{"n": 1}`), says("done"))
	f := follow(t, l, startHelper(t, l, def(m.URL, ""), map[string]any{"question": "no add"}))
	if got := offeredNames(m.Calls[0].Tools); got != blobTools+" open" {
		t.Errorf("the turn was offered: %s", got)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `there is no tool named \"add\"`) {
		t.Errorf("a call of a tool not offered was shown as: %s", shown)
	}
	if got := strings.Join(f.jobs, " "); got != "files turn turn#2" {
		t.Errorf("the jobs were: %s", got)
	}
	// A toolkit not offered has its lists not run.
	quiet := strings.Replace(def("ADDR", ""), "net: { include: toolkits/net/net.yaml }", `net: { include: toolkits/net/net.yaml, when: "${input.question === 'net'}" }`, 1)
	m = newModel(t, says("done"))
	f = follow(t, l, startHelper(t, l, strings.Replace(quiet, "ADDR", m.URL, 1), map[string]any{"question": "q"}))
	if got := strings.Join(f.jobs, " "); got != "turn" {
		t.Errorf("with net not offered, the jobs were: %s", got)
	}
	// A when: that cannot be evaluated fails the agent at its start.
	broken := strings.Replace(def("ADDR", ""), "net: { include: toolkits/net/net.yaml }", `net: { include: toolkits/net/net.yaml, when: "${input.nothing.here}" }`, 1)
	m = newModel(t)
	r = wait(t, l, startHelper(t, l, strings.Replace(broken, "ADDR", m.URL, 1), map[string]any{"question": "q"}))
	if r.Status != "failed" || !strings.Contains(r.Error, "toolkits.net.when") || len(m.Calls) != 0 {
		t.Errorf("with a when: that cannot be evaluated, the agent ended %s: %s", r.Status, r.Error)
	}
	// A when: is one expression, and reads the agent's input.
	for when, want := range map[string]string{
		"maybe":               "toolkits.net.when: \"maybe\": a condition is one expression",
		"${args.n > 1}":       "toolkits.net.when",
		"${input.n} and more": "a condition is one expression",
	} {
		d := strings.Replace(def("http://127.0.0.1:1", ""), "net: { include: toolkits/net/net.yaml }", "net: { include: toolkits/net/net.yaml, when: \""+when+"\" }", 1)
		if _, err := machineParse(d, toolkits); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("with when: %s, want a refusal holding %q, got: %v", when, want, err)
		}
	}
	// A tool of a toolkit not offered is not shown once the toolkit is open,
	// and a call of it runs nothing.
	hidden := maps.Clone(toolkits)
	hidden["toolkits/net/net.yaml"] = strings.Replace(toolkits["toolkits/net/net.yaml"], "description: Run one show command", "description: Run one show command\n\t\t    when: \"${input.question !== 'no show'}\"", 1)
	m = newModel(t, calls("c1", "open", `{"toolkit": "net"}`), calls("c2", "show", `{"command": "x"}`), says("done"))
	r = wait(t, l, startWith(t, l, def(m.URL, ""), hidden, map[string]any{"question": "no show"}))
	if r.Status != "done" {
		t.Fatalf("with show not offered, the agent ended %s: %s", r.Status, r.Error)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobTools+" files_read" {
		t.Errorf("with show not offered, after the open, the turn was offered: %s", got)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, `there is no tool named \"show\"`) {
		t.Errorf("a call of a toolkit's tool not offered was shown as: %s", shown)
	}
}

// What open gives back is the toolkit's instructions, filled from the
// agent's input when it is opened and shown whole, however long; with none,
// it says the toolkit was opened. A run that carries the conversation on
// and opens the toolkit again is given them again.
func TestAgentOpenGivesTheInstructions(t *testing.T) {
	long := strings.Repeat("Check twice. ", 40<<10/13+1)
	files := map[string]string{}
	for k, v := range toolkits {
		files[k] = v
	}
	files["toolkits/net/net.yaml"] = strings.Replace(toolkits["toolkits/net/net.yaml"], "meta:", "instructions: Show only, for ${input.question}. "+long+"\n\t\tmeta:", 1)
	m := newModel(t, calls("c1", "open", `{"toolkit": "net"}`), calls("c2", "open", `{"toolkit": "payments"}`), says("done"))
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "Ann"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "Show only, for Ann. Check twice.") || !strings.Contains(shown, strings.TrimSpace(long)) || strings.Contains(shown, `"blob"`) {
		t.Errorf("the open of net was shown as: %.300s", shown)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, `"content":"Opened payments."`) {
		t.Errorf("the open of payments was shown as: %s", shown)
	}
	conversation := r.Output.(map[string]any)["conversation"]
	m2 := newModel(t, calls("c3", "open", `{"toolkit": "net"}`), says("again"))
	next := wait(t, l, startWith(t, l, helper(m2.URL, "conversation: ${input.so}"), files, map[string]any{"question": "Bo", "so": carried(conversation, r.ID)}))
	if next.Status != "done" {
		t.Fatalf("the run that carried on ended %s: %s", next.Status, next.Error)
	}
	if shown := lastShown(m2.Calls[1]); !strings.Contains(shown, "Show only, for Bo.") {
		t.Errorf("the open of net, again, was shown as: %.300s", shown)
	}
}

// A conversation is input, and opens no toolkit, whatever it names.
func TestAgentConversationOpensNoToolkit(t *testing.T) {
	m := newModel(t, says("done"))
	l := local(t, "all")
	so := map[string]any{"turns": []any{}, "blobs": map[string]any{}, "opened": []any{"net", "payments"}}
	r := wait(t, l, startHelper(t, l, helper(m.URL, "conversation: ${input.so}"), map[string]any{"question": "q", "so": so}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if got := offeredNames(m.Calls[0].Tools); got != "add blob_code blob_read blob_search open" {
		t.Errorf("the first turn was offered: %s", got)
	}
	if got := openDescription(m.Calls[0].Tools); !strings.Contains(got, "- net:") || !strings.Contains(got, "- payments:") {
		t.Errorf("open says: %s", got)
	}
}

// netAsking gives the files of the helper with net's tool show marked for
// approval and net naming an ask whose input is the value given.
func netAsking(askInput string) map[string]string {
	files := map[string]string{}
	for k, v := range toolkits {
		files[k] = v
	}
	net := strings.Replace(toolkits["toolkits/net/net.yaml"], "tools:\n", "ask: { script: ./ask.js, input: "+askInput+" }\n\t\ttools:\n", 1)
	files["toolkits/net/net.yaml"] = strings.Replace(net, `input: { command: "${args.command}" } }`, `input: { command: "${args.command}" }, approve: true }`, 1)
	files["toolkits/net/ask.js"] = `return { approved: input.jobs.map((j) => j.name), note: "net's ask" }`
	return files
}

// The held jobs of one step that go to one ask start it once, though
// filling its values in twice would give two different values.
func Test_mach_one_run_of_each_ask_for_a_step(t *testing.T) {
	files := netAsking(`{ r: "${Math.random()}" }`)
	twice := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c1", Name: "show", Args: `{"command": "a"}`}, {ID: "c2", Name: "show", Args: `{"command": "b"}`}}}
	m := newModel(t, calls("c0", "open", `{"toolkit": "net"}`), twice, says("done"))
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

// A toolkit's ask whose values cannot be filled in denies the jobs held for
// it, with a note that names the fault, as a definition's ask does, and
// the agent carries on.
func Test_agent_toolkit_ask_that_cannot_be_filled_denies(t *testing.T) {
	files := netAsking(`{ who: "${input.question.nothing.here}" }`)
	m := newModel(t, calls("c0", "open", `{"toolkit": "net"}`), calls("c1", "show", `{"command": "a"}`), says("done"))
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "denied: the ask could not be started: toolkits.net.ask.input") || strings.Contains(shown, "shown: a") {
		t.Errorf("the call was shown as: %s", shown)
	}
}

// A toolkit's ask is the nearest ask above every machine below its tools'
// jobs, before the agent's, with its own values and permissions, filled
// from the agent's input; a machine below that names an ask of its own
// uses its own. An agent with no ask of its own is accepted when the
// toolkit's ask covers it.
func Test_agent_toolkit_ask_is_nearest_above_its_tools_machines(t *testing.T) {
	def := strings.Replace(helper("ADDR", "ask: ./yes.js"), "payments: { include: toolkits/payments/payments.yaml }", "", 1)
	files := map[string]string{"add.js": scripts["add.js"], "yes.js": `return { approved: input.jobs.map((j) => j.name) }`}
	files["toolkits/net/net.yaml"] = `
		description: The firewall and the switch.
		ask: { script: ./ask.js, input: { who: "${input.question}" }, allow: [env:HOME] }
		tools:
		  deep:
		    description: Do deep work
		    job: { machine: ./outer.yaml }
	`
	files["toolkits/net/ask.js"] = `
		env.get("HOME")
		return { approved: input.who === "q" ? input.jobs.map((j) => j.name) : [], note: "net's ask, for " + input.asking.machine }`
	files["toolkits/net/outer.yaml"] = "flow: outer\nstages:\n  in: { job: { machine: ./inner.yaml } }\noutput: ${stages.in.output}\n"
	files["toolkits/net/inner.yaml"] = "flow: inner\nstages:\n  work: { job: { script: ./work.js, approve: true } }\noutput: ${stages.work.output}\n"
	files["toolkits/net/work.js"] = `return "worked"`
	files["toolkits/net/own.js"] = `return { approved: [], note: "inner's own ask" }`
	l := local(t, "all")
	for _, run := range []struct {
		question, inner, want string
	}{
		{"q", "", "worked"},
		{"other", "", "net's ask, for inner"},
		{"q", "ask: ./own.js\n", "inner's own ask"},
	} {
		files["toolkits/net/inner.yaml"] = strings.Replace(files["toolkits/net/inner.yaml"], "flow: inner\n", "flow: inner\n"+run.inner, 1)
		m := newModel(t, calls("c0", "open", `{"toolkit": "net"}`), calls("c1", "deep", `{}`), says("done"))
		r := wait(t, l, startWith(t, l, strings.Replace(def, "ADDR", m.URL, 1), files, map[string]any{"question": run.question}))
		if r.Status != "done" {
			t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
		}
		if shown := lastShown(m.Calls[2]); !strings.Contains(shown, run.want) {
			t.Errorf("asked %q with %q in inner, the call was shown as: %s", run.question, run.inner, shown)
		}
		files["toolkits/net/inner.yaml"] = strings.Replace(files["toolkits/net/inner.yaml"], run.inner, "", 1)
	}
	// The toolkit's ask covers inner's job, and the agent needs no ask of its
	// own; without the toolkit's ask, it does.
	def = strings.Replace(strings.Replace(def, "\t\task: ./yes.js\n", "", 1), "ADDR", "http://127.0.0.1:1", 1)
	if _, err := machineParse(def, files); err != nil {
		t.Errorf("an agent whose toolkit's ask covers its tool's machine was refused: %v", err)
	}
	files["toolkits/net/net.yaml"] = strings.Replace(files["toolkits/net/net.yaml"], "ask:", "meta:", 1)
	if _, err := machineParse(def, files); err == nil || !strings.Contains(err.Error(), "there is no ask for it to go to") {
		t.Errorf("a toolkit's tool whose machine holds a job, with no ask: %v", err)
	}
}

// A toolkit's ask was not made while running, and it is the nearest above a
// machine made while running below one of the toolkit's tools.
func Test_agent_toolkit_ask_is_reached_from_a_made_machine_below_its_tool(t *testing.T) {
	m := newModel(t, calls("c0", "open", `{"toolkit": "net"}`), calls("c1", "deep", `{}`), says("done"))
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
	files["helper.yaml"] = strings.Replace(strings.Replace(helper(m.URL, ""), "payments: { include: toolkits/payments/payments.yaml }", "", 1), "../toolkits", "./toolkits", 1)
	files["make.js"] = `return machines.define("flow: made\nstages:\n  work: { job: { script: ./step.js, approve: true } }\n",
		{ files: { "./step.js": "return 7" } })`
	files["toolkits/net/net.yaml"] = `
		description: The firewall and the switch.
		ask: ./ask.js
		tools:
		  deep:
		    description: Do deep work
		    job: { machine: "${input.made}" }
	`
	files["toolkits/net/ask.js"] = `return { approved: [], note: "net's ask, for " + input.asking.machine }`
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
  act: { description: Act, job: { script: ./step.js, approve: true } }
`, "ADDRESS", m.URL)
	def := `
		flow: top
		ask: ./human.js
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}", allow: [ all ] } }
	`
	files := map[string]string{
		"human.js": `return { approved: [], note: "the owner's ask, for " + input.asking.machine }`,
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

// The list of an entry in a toolkit goes to the toolkit's ask when it is held,
// and the ask is shown the toolkit's name, as for a call of the toolkit's tools:
// with no ask of the agent's, and with one, which is not asked.
func Test_agent_toolkit_list_goes_to_the_toolkits_ask(t *testing.T) {
	files := map[string]string{}
	for k, v := range toolkits {
		files[k] = v
	}
	net := strings.Replace(toolkits["toolkits/net/net.yaml"], "tools:\n", "ask: ./ask.js\n\t\ttools:\n", 1)
	files["toolkits/net/net.yaml"] = strings.Replace(net, "list: { script: ./list.js }", "list: { script: ./list.js, approve: true }", 1)
	files["toolkits/net/ask.js"] = `return { approved: input.jobs.filter((j) => j.name === "files" && j.asked.toolkit === "net").map((j) => j.name) }`
	files["no.js"] = `return { approved: [], note: "the agent's ask" }`
	for _, extra := range []string{"", "ask: ./no.js"} {
		m := newModel(t, calls("c1", "open", `{"toolkit": "net"}`), calls("c2", "files_read", `{}`), says("done"))
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

// An error names the list of an entry in a toolkit by where it is written,
// the place the check names it by.
func Test_agent_list_is_named_by_its_place(t *testing.T) {
	for list, want := range map[string]string{
		`throw new Error("no server")`: "toolkits.net.tools.files.list: no server",
		`return "nothing"`:             "toolkits.net.tools.files.list: the list gave no list of tools",
	} {
		files := map[string]string{}
		for k, v := range toolkits {
			files[k] = v
		}
		files["toolkits/net/list.js"] = list
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
