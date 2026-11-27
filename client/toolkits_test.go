package client_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
)

// helper is an agent with a tool of its own and two toolkits, each a
// machine in a file of its own.
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
		  net: { machine: ./toolkits/net.yaml, input: { asker: "${input.question}" } }
		  payments: { machine: ./toolkits/payments.yaml }
	`, "ADDRESS", address), "EXTRA", extra)
}

var toolkits = map[string]string{
	"add.js": scripts["add.js"],
	"toolkits/net.yaml": `
		toolkit: net
		description: The firewall and the switch.
		input: { asker: any }
		instructions: Run show commands only. ${input.asker} is asking.
		tools:
		  show:
		    description: Run one show command
		    args: { command: string }
		    job: { script: ./show.js, input: { command: "${args.command}" } }
	`,
	"toolkits/show.js": `return "shown: " + input.command`,
	"toolkits/payments.yaml": `
		toolkit: payments
		description: Payments.
		tools:
		  pay:
		    description: Pay someone
		    args: { to: string }
		    job: { script: ./pay.js, input: { to: "${args.to}" } }
	`,
	"toolkits/pay.js": `return "paid " + input.to`,
}

const blobToolNames = "blob_code blob_read blob_search"

// opens gives the arguments of a call of open for one of the agent's own
// toolkits.
func opens(name string) string { return `{"list": "toolkits", "name": "` + name + `"}` }

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

// The model is shown open, with the list, the name and the description of
// each toolkit it has not opened. An open is a job of the entry, which runs
// the toolkit; its instructions are the result, and its tools are shown
// from the next turn, each under the entry's name and its own. A run that
// carries the conversation on has every toolkit closed.
func TestAgentToolkitsAreOpenedByTheModel(t *testing.T) {
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_show", `{"command": "show route"}`),
		calls("c3", "open", opens("payments")), says("done"))
	l := local(t, "all")
	id := startHelper(t, l, helper(m.URL, ""), map[string]any{"question": "q"})
	var jobs []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if e.ID == id && (ask.Kind == "script" || ask.Kind == "child") {
				jobs = append(jobs, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(jobs, " "); got != "turn open turn#2 net_show turn#3 open#2 turn#4" {
		t.Errorf("the jobs were: %s", got)
	}
	if got := offeredNames(m.Calls[0].Tools); got != "add "+blobToolNames+" open" {
		t.Errorf("the first turn was offered: %s", got)
	}
	// The model is told which toolkits it can open before its prompt, and
	// the description of open is the same at every turn.
	if shown := strings.ReplaceAll(jsonOf(m.Calls[0].Messages[0]), `\`, ""); !strings.Contains(shown, `"toolkits":[{"description":"The firewall and the switch.","name":"net"},{"description":"Payments.","name":"payments"}]`) ||
		!strings.Contains(shown, `call open with { list: "toolkits", name:`) {
		t.Errorf("the first turn was shown: %s", shown)
	}
	if first, later := openDescription(m.Calls[0].Tools), openDescription(m.Calls[1].Tools); first == "" || first != later || strings.Contains(first, "net") {
		t.Errorf("open says %q, then %q", first, later)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobToolNames+" net_show open" {
		t.Errorf("the turn after the open was offered: %s", got)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "Run show commands only. q is asking.") {
		t.Errorf("the open of net gave: %s", shown)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "shown: show route") {
		t.Errorf("the toolkit's tool gave: %s", shown)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "Opened payments.") {
		t.Errorf("the open of payments, which has no instructions, gave: %s", shown)
	}
	if got := offeredNames(m.Calls[3].Tools); got != "add "+blobToolNames+" net_show payments_pay" {
		t.Errorf("with both open, the turn was offered: %s", got)
	}
	conversation := r.Output.(map[string]any)["conversation"].(map[string]any)
	if got := strings.Join(slices.Sorted(maps.Keys(conversation)), " "); got != "blobs opened turns" {
		t.Errorf("the conversation holds: %s", got)
	}
	// A run that carries the conversation on has both open, and runs no open.
	m2 := newModel(t, calls("c4", "net_show", `{"command": "show arp"}`), says("again"))
	f := follow(t, l, startHelper(t, l, helper(m2.URL, "conversation: ${input.so}"), map[string]any{"question": "and?", "so": carried(conversation, r.ID)}))
	if got := strings.Join(f.jobs, " "); got != "turn show turn#2" {
		t.Errorf("the run that carried on had the script jobs: %s", got)
	}
	if got := offeredNames(m2.Calls[0].Tools); got != "add "+blobToolNames+" net_show payments_pay" {
		t.Errorf("the run that carried on was offered: %s", got)
	}
	if shown := jsonOf(m2.Calls[0]); strings.Count(shown, "Run show commands only.") != 1 {
		t.Errorf("the run that carried on was not shown the instructions once: %s", shown)
	}
	if shown := lastShown(m2.Calls[1]); !strings.Contains(shown, "shown: show arp") {
		t.Errorf("the run that carried on ran net_show as: %s", shown)
	}
}

// A conversation is input, written by whoever starts the run. It opens a
// toolkit only where a call of open with no hook would: the agent has the
// entry, as a toolkit, its when: holds, and it is not marked approve: true.
func Test_agent_conversation_opens_only_what_open_would(t *testing.T) {
	l := local(t, "all")
	m := newModel(t, says("first"))
	r := wait(t, l, startHelper(t, l, helper(m.URL, ""), map[string]any{"question": "q"}))
	conversation := maps.Clone(r.Output.(map[string]any)["conversation"].(map[string]any))
	list := func(entry, tool string) map[string]any {
		return map[string]any{"entry": entry, "entries": []any{map[string]any{"kind": "tool", "name": tool, "description": "Claimed", "args": map[string]any{"type": "object"}}}}
	}
	conversation["opened"] = map[string]any{"net": list("net", "show"), "payments": list("payments", "pay"), "add": list("add", "more"),
		"gone": list("gone", "x"), "other": list("net", "wipe"), "bad": "text"}
	for name, c := range map[string]struct{ old, new, want string }{
		"as it is":                {"", "", "add " + blobToolNames + " net_show other_wipe payments_pay"},
		"net marked for approval": {`input: { asker: "${input.question}" } }`, `input: { asker: "${input.question}" }, approve: true }`, "add " + blobToolNames + " open payments_pay"},
		"payments not offered":    {"payments: { machine: ./toolkits/payments.yaml }", `payments: { machine: ./toolkits/payments.yaml, when: "${input.question == 'pay'}" }`, "add " + blobToolNames + " net_show other_wipe"},
	} {
		m := newModel(t, calls("c1", "other_wipe", `{}`), says("done"))
		def := helper(m.URL, "conversation: ${input.so}\n\t\thooks: { check: { before: ./ask.js, when: \"${job.approve}\" } }")
		if c.old != "" {
			def = strings.Replace(def, c.old, c.new, 1)
		}
		files := maps.Clone(toolkits)
		files["ask.js"] = `return { ok: true }`
		wait(t, l, startWith(t, l, def, files, map[string]any{"question": "and?", "so": carried(conversation, r.ID)}))
		if got := offeredNames(m.Calls[0].Tools); got != c.want {
			t.Errorf("%s: the first turn was offered: %s", name, got)
		}
		// A kept item that misdescribes a tool misleads the model, and the
		// toolkit refuses the call.
		if name == "as it is" {
			if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `the toolkit net has no tool \"wipe\"`) {
				t.Errorf("a call of a tool the toolkit does not have was shown as: %s", shown)
			}
		}
	}
}

// A list too large to show is a blob: the open gives its handle, the model
// searches it with the blob tools, and opens a tool by its name. The tool
// is offered from the next turn.
func TestAgentListTooLargeToShowIsABlob(t *testing.T) {
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = "toolkit: net\ndescription: The firewall and the switch.\ninput: { asker: any }\njob: ./many.js\n"
	files["toolkits/many.js"] = `
		const r = input.request
		if (r.do !== "open") return { result: "ran " + r.tool }
		const entries = []
		for (let i = 0; i < 80; i++) entries.push({ kind: "tool", name: "t" + i, description: "Tool " + i + " " + "x".repeat(600), args: { type: "object" } })
		return { instructions: "Search first.", entries }`
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "blob_search", `{"blob": "blob1", "find": "Tool 42 "}`),
		calls("c3", "open", `{"list": "net", "name": "t42"}`), calls("c4", "net_t42", `{}`), calls("c5", "net_t7", `{}`), says("done"))
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[1]), `\`, ""); !strings.Contains(shown, `"instructions":"Search first."`) || !strings.Contains(shown, `"blob":"blob1"`) ||
		!strings.Contains(shown, `"items":80`) || !strings.Contains(shown, `"list":"net"`) {
		t.Errorf("the open of a long list was shown as: %.600s", shown)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobToolNames+" open" {
		t.Errorf("after the open of a long list, the turn was offered: %s", got)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[1]), `\`, ""); !strings.Contains(shown, `call open with { list: "net", name:`) {
		t.Errorf("the open of a long list did not say how to open from it: %.600s", shown)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[2]), `\`, ""); !strings.Contains(shown, `"name":"t42"`) {
		t.Errorf("the search gave: %.400s", shown)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "Opened net_t42") {
		t.Errorf("the open of a tool gave: %s", shown)
	}
	if got := offeredNames(m.Calls[3].Tools); got != "add "+blobToolNames+" net_t42 open" {
		t.Errorf("after the open of a tool, the turn was offered: %s", got)
	}
	if shown := lastShown(m.Calls[4]); !strings.Contains(shown, "ran t42") {
		t.Errorf("the call of the opened tool gave: %s", shown)
	}
	if shown := lastShown(m.Calls[5]); !strings.Contains(shown, `there is no tool named \"net_t7\"`) {
		t.Errorf("the call of a tool not opened was shown as: %s", shown)
	}
	// The list of the agent's own toolkits is a blob where it is as large.
	files = maps.Clone(toolkits)
	files["toolkits/net.yaml"] = strings.Replace(toolkits["toolkits/net.yaml"], "The firewall and the switch.", "The firewall. "+strings.Repeat("y", 20<<10), 1)
	files["toolkits/payments.yaml"] = strings.Replace(toolkits["toolkits/payments.yaml"], "Payments.", "Payments. "+strings.Repeat("z", 20<<10), 1)
	m = newModel(t, calls("c1", "open", opens("net")), says("done"))
	if r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if got := jsonOf(m.Calls[0].Tools); strings.Contains(got, "yyyy") {
		t.Errorf("with a long list of toolkits, the tools hold a description: %.300s", got)
	}
	if shown := strings.ReplaceAll(jsonOf(m.Calls[0]), `\`, ""); !strings.Contains(shown, `"list":"toolkits"`) || !strings.Contains(shown, `"items":2`) {
		t.Errorf("the first turn was shown: %.600s", shown)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobToolNames+" net_show open" {
		t.Errorf("after the open, the turn was offered: %s", got)
	}
}

// An entry that names a machine writes nothing else of a tool, and a
// toolkit's tool that would be shown under the name of another tool is an
// error from the open.
func TestAgentToolkitToolNamesAreTheirOwn(t *testing.T) {
	refused := func(old, new, want string) {
		t.Helper()
		_, err := machineParse(strings.Replace(helper("http://127.0.0.1:1", ""), old, new, 1), toolkits)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want a refusal holding %q, got: %v", want, err)
		}
	}
	const payments = "payments: { machine: ./toolkits/payments.yaml }"
	refused(payments, "payments: { machine: ./toolkits/payments.yaml, description: Pay }", "tools.payments.description: an entry with machine: or call: has no description")
	refused(payments, "payments: { machine: ./toolkits/payments.yaml, call: kits/pay }", "tools.payments: an entry names its machine by its file with machine:, or by its location with call:, and not both")
	refused(payments, "payments: { machine: ./toolkits/payments.yaml, input: { to: me } }", `tools.payments.input.to: the machine ./toolkits/payments.yaml takes no input named "to"`)
	refused(`input: { asker: "${input.question}" }`, `input: { request: { do: call } }`, `tools.net.input.request: the machine ./toolkits/net.yaml takes no input named "request"`)
	refused(`job: { script: ./add.js, input: { n: "${args.n}" } }`, `job: { script: ./add.js, input: { n: "${args.n}" } }
		    open: true`, "tools.add.open: open: belongs to an entry with machine: or call:")
	if _, err := machineParse(strings.Replace(helper("http://127.0.0.1:1", ""), payments, "sum: { machine: ./sum.yaml, open: true }", 1),
		map[string]string{"add.js": "", "toolkits/net.yaml": toolkits["toolkits/net.yaml"], "toolkits/show.js": "",
			"sum.yaml": "flow: sum\ndescription: Sums.\nstages: { one: { job: ./add.js } }\n"}); err == nil ||
		!strings.Contains(err.Error(), "tools.sum.open: the machine ./sum.yaml is no toolkit: open: is for an entry that names a toolkit") {
		t.Errorf("open: true on an entry that names a flow: %v", err)
	}
	// The stored definition holds the toolkit's description, and that it is one.
	d := draft(t, helper("http://127.0.0.1:1", ""), toolkits)
	for _, want := range []string{`"description":"The firewall and the switch."`, `"toolkit":true`, `"machine":"./toolkits/net.yaml"`} {
		if !strings.Contains(string(d.Definition.Body), want) {
			t.Errorf("the stored definition does not hold %s: %s", want, d.Definition.Body)
		}
	}
	m := newModel(t, calls("c1", "open", opens("net")), says("done"))
	def := strings.Replace(helper(m.URL, ""), "  add:\n", "  net_show:\n", 1)
	l := local(t, "all")
	if r := wait(t, l, startHelper(t, l, def, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `the tool \"show\" would be shown to the model as \"net_show\", which another tool has`) {
		t.Errorf("the open was shown as: %s", shown)
	}
	if got := offeredNames(m.Calls[1].Tools); !strings.Contains(got, "open") {
		t.Errorf("after the open that failed, the turn was offered: %s", got)
	}
}

// approved gives the helper with net marked approve: true and a hook before
// each marked job, and its files with the hook's script.
func approved(address, ask string) (string, map[string]string) {
	def := strings.Replace(helper(address, `hooks: { check: { before: ./ask.js, when: "${job.approve}" } }`),
		`input: { asker: "${input.question}" } }`, `input: { asker: "${input.question}" }, approve: true }`, 1)
	files := maps.Clone(toolkits)
	files["ask.js"] = ask
	return def, files
}

// An entry marked approve: true marks every job it starts for approval: the
// open of its toolkit and each call of a tool. A hook reads the entry's name
// as toolkit, and open or the tool's own name. An open a hook refuses goes
// back to the model as denied, with the hook's note, and opens nothing.
func TestAgentToolkitOpenWaitsForAHook(t *testing.T) {
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_show", `{"command": "a"}`), says("done"))
	def, files := approved(m.URL, `
		const tool = input.job.tool
		const open = input.job.name === "open" && tool.open === true && !("name" in tool)
		const call = input.job.name === "net_show" && tool.name === "show" && tool.args.command === "a" && !("open" in tool)
		return { ok: tool.toolkit === "net" && (open || call), note: "the hook read " + JSON.stringify(tool) }`)
	l := local(t, "all")
	if r := wait(t, l, startWith(t, l, def, files, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	for i, call := range m.Calls[1:] {
		if shown := lastShown(call); strings.Contains(shown, "denied") {
			t.Errorf("turn %d was shown: %s", i+2, shown)
		}
	}
	// A hook that says no leaves the toolkit closed.
	m = newModel(t, calls("c1", "open", opens("net")), says("done"))
	def, files = approved(m.URL, `return { ok: false, note: "not today" }`)
	r := wait(t, l, startWith(t, l, def, files, map[string]any{"question": "q"}))
	if shown := lastShown(m.Calls[1]); r.Status != "done" || !strings.Contains(shown, "denied") || !strings.Contains(shown, "not today") {
		t.Errorf("the agent ended %s, and the open was shown as: %s", r.Status, shown)
	}
	if got := offeredNames(m.Calls[1].Tools); strings.Contains(got, "net_show") {
		t.Errorf("an open a hook refused opened the toolkit: %s", got)
	}
	// An entry marked for approval needs a hook before its jobs.
	def, files = approved("http://127.0.0.1:1", "")
	if _, err := machineParse(strings.Replace(def, `hooks: { check: { before: ./ask.js, when: "${job.approve}" } }`, "", 1), files); err == nil ||
		!strings.Contains(err.Error(), "no hook runs before it") {
		t.Errorf("an entry marked for approval with no hook: %v", err)
	}
}

// A toolkit writes hooks of its own, which run for its tools' jobs whichever
// agent names it.
func TestAgentToolkitHooksAreItsOwn(t *testing.T) {
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = strings.Replace(strings.Replace(toolkits["toolkits/net.yaml"], "tools:\n", "hooks: { team: { before: ./team.js, when: \"${job.approve}\" } }\n\t\ttools:\n", 1),
		`input: { command: "${args.command}" } }`, `input: { command: "${args.command}" }, approve: true }`, 1)
	files["toolkits/team.js"] = `return { ok: false, note: "the team read " + input.job.tool.toolkit + "/" + input.job.tool.name }`
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_show", `{"command": "a"}`), says("done"))
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"}))
	if shown := lastShown(m.Calls[2]); r.Status != "done" || !strings.Contains(shown, "denied") || !strings.Contains(shown, "the team read net/show") {
		t.Errorf("the agent ended %s (%s), and the call was shown as: %s", r.Status, r.Error, shown)
	}
}

// A when: on an entry is a condition on the agent's input, checked once
// when the run starts. An entry whose when: does not hold is not offered:
// open does not name its toolkit, and a call of it runs nothing.
func TestAgentWhenSaysWhatIsOffered(t *testing.T) {
	const scoped = `payments: { machine: ./toolkits/payments.yaml, open: true, when: "${input.so == 'pay'}" }`
	def := func(address string) string {
		d := strings.Replace(helper(address, ""), "payments: { machine: ./toolkits/payments.yaml }", scoped, 1)
		return strings.Replace(d, `job: { script: ./add.js, input: { n: "${args.n}" } }`, `job: { script: ./add.js, input: { n: "${args.n}" } }
		    when: "${input.so == 'sum'}"`, 1)
	}
	m := newModel(t, calls("c1", "open", opens("payments")), calls("c2", "add", `{"n": 1}`), says("done"))
	l := local(t, "all")
	f := follow(t, l, startHelper(t, l, def(m.URL), map[string]any{"question": "q"}))
	if got := strings.Join(f.jobs, " "); got != "turn turn#2 turn#3" {
		t.Errorf("with neither when: holding, the jobs were: %s", got)
	}
	if got := offeredNames(m.Calls[0].Tools); got != blobToolNames+" open" {
		t.Errorf("the first turn was offered: %s", got)
	}
	if shown := jsonOf(m.Calls[0]); strings.Contains(shown, "Payments.") || !strings.Contains(shown, "The firewall and the switch.") {
		t.Errorf("the first turn was shown: %s", shown)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `there is no toolkit named \"payments\" to open in the list \"toolkits\"`) {
		t.Errorf("the open of a toolkit not offered was shown as: %s", shown)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, `there is no tool named \"add\"`) {
		t.Errorf("the call of a tool not offered was shown as: %s", shown)
	}
	// With the when: holding, the toolkit is opened before the first turn.
	m = newModel(t, says("done"))
	wait(t, l, startHelper(t, l, def(m.URL), map[string]any{"question": "q", "so": "pay"}))
	if got := offeredNames(m.Calls[0].Tools); got != blobToolNames+" open payments_pay" {
		t.Errorf("given the scope, the first turn was offered: %s", got)
	}
	// A when: that cannot be evaluated fails the agent at its start.
	m = newModel(t)
	bad := strings.Replace(def(m.URL), "${input.so == 'pay'}", "${input.so.scopes.includes('pay')}", 1)
	if r := wait(t, l, startHelper(t, l, bad, map[string]any{"question": "q"})); r.Status != "failed" || !strings.Contains(r.Error, "tools.payments.when") || len(m.Calls) != 0 {
		t.Errorf("a when: that cannot be evaluated ended %s after %d turns: %s", r.Status, len(m.Calls), r.Error)
	}
}

// A toolkit's instructions are filled from the input its entry gives, and
// are the result of open, shown to the model whole.
func TestAgentOpenGivesTheInstructions(t *testing.T) {
	long := strings.Repeat("x", 40<<10)
	m := newModel(t, calls("c1", "open", opens("net")), says("done"))
	l := local(t, "all")
	if r := wait(t, l, startHelper(t, l, helper(m.URL, ""), map[string]any{"question": long})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "Run show commands only. "+long+" is asking.") || strings.Contains(shown, `"blob"`) {
		t.Errorf("instructions of 40 KiB were shown as %d bytes", len(shown))
	}
}

// A toolkit with open: true is opened before the first turn: the first turn
// is offered its tools and holds its instructions. One that fails to open
// fails the agent.
func TestAgentToolkitOpenBeforeTheFirstTurn(t *testing.T) {
	const net = `input: { asker: "${input.question}" } }`
	m := newModel(t, calls("c1", "net_show", `{"command": "a"}`), says("done"))
	l := local(t, "all")
	id := startHelper(t, l, strings.Replace(helper(m.URL, ""), net, `input: { asker: "${input.question}" }, open: true }`, 1), map[string]any{"question": "q"})
	var jobs []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if e.ID == id && (ask.Kind == "script" || ask.Kind == "child") {
				jobs = append(jobs, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(jobs, " "); got != "open turn net_show turn#2" {
		t.Errorf("the jobs were: %s", got)
	}
	if got := offeredNames(m.Calls[0].Tools); got != "add "+blobToolNames+" net_show open" {
		t.Errorf("the first turn was offered: %s", got)
	}
	if shown := jsonOf(m.Calls[0]); !strings.Contains(shown, "(the toolkit net is open; its instructions follow)") || !strings.Contains(shown, "Run show commands only. q is asking.") {
		t.Errorf("the first turn was shown: %s", shown)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "shown: a") {
		t.Errorf("the call gave: %s", shown)
	}
	m = newModel(t)
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = "toolkit: net\ndescription: The firewall and the switch.\ninput: { asker: any }\njob: ./away.js\n"
	files["toolkits/away.js"] = `throw new Error("the server is away")`
	r = wait(t, l, startWith(t, l, strings.Replace(helper(m.URL, ""), net, `input: { asker: "${input.question}" }, open: true }`, 1), files, map[string]any{"question": "q"}))
	if r.Status != "failed" || !strings.Contains(r.Error, "tools.net") || !strings.Contains(r.Error, "the server is away") || len(m.Calls) != 0 {
		t.Errorf("with a toolkit that fails to open, the agent ended %s after %d turns: %s", r.Status, len(m.Calls), r.Error)
	}
}

// The toolkit checks a call's arguments: the agent gives them as the model
// did, and arguments that do not fit go back to the model as an error.
func TestAgentToolkitChecksTheArguments(t *testing.T) {
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_show", `{"command": 4}`), calls("c3", "net_show", `{"command": "a"}`), says("done"))
	l := local(t, "all")
	if r := wait(t, l, startHelper(t, l, helper(m.URL, ""), map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, `the arguments do not fit the tool \"show\"`) || !strings.Contains(shown, "command") {
		t.Errorf("arguments that do not fit were shown as: %s", shown)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "shown: a") {
		t.Errorf("the call that fits gave: %s", shown)
	}
}

// A toolkit's open may give a toolkit inside it. The model opens it from
// the toolkit's list, and the entry's job is asked with the names that lead
// to it.
func TestAgentOpensAToolkitInsideAToolkit(t *testing.T) {
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = "toolkit: net\ndescription: The firewall and the switch.\ninput: { asker: any }\njob: ./answer.js\n"
	files["toolkits/answer.js"] = `
		const r = input.request
		const inside = (r.toolkit || []).join("/")
		const tool = (name) => ({ kind: "tool", name, description: "Does " + name, args: { type: "object" }, call: "/elsewhere" })
		if (r.do !== "open") return { result: inside + ":" + r.tool }
		if (inside === "") return { instructions: "Top.", entries: [tool("find"), { kind: "toolkit", name: "review", description: "How to review." }] }
		return { instructions: "Review well.", entries: [tool("read")] }`
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "open", `{"list": "net", "name": "review"}`),
		calls("c3", "net_review_read", `{}`), calls("c4", "net_find", `{}`), says("done"))
	l := local(t, "all")
	if r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[1]), `\`, ""); !strings.Contains(shown, `"toolkits":[{"description":"How to review.","name":"review"}]`) || !strings.Contains(shown, `"instructions":"Top."`) {
		t.Errorf("the open of net gave: %s", shown)
	}
	if got := offeredNames(m.Calls[2].Tools); got != "add "+blobToolNames+" net_find net_review_read open" {
		t.Errorf("with both open, the turn was offered: %s", got)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "Review well.") {
		t.Errorf("the open of the toolkit inside gave: %s", shown)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "review:read") {
		t.Errorf("the call of the tool inside gave: %s", shown)
	}
	if shown := lastShown(m.Calls[4]); !strings.Contains(shown, ":find") || strings.Contains(shown, "review") {
		t.Errorf("the call of the toolkit's own tool gave: %s", shown)
	}
	if offered := jsonOf(m.Calls[2].Tools); strings.Contains(offered, "elsewhere") {
		t.Errorf("the model was offered a field an entry does not hold: %s", offered)
	}
}

// An entry that names a machine by its file is one tool: it is offered with
// the machine's description and its input without the names the entry
// fixes, and a call runs the machine as a child run.
func TestAgentEntryNamesAMachineByItsFile(t *testing.T) {
	m := newModel(t, calls("c1", "sum", `{"n": 5}`), says("done"))
	def := strings.Replace(helper(m.URL, ""), "payments: { machine: ./toolkits/payments.yaml }", "sum: { machine: ./sum.yaml, input: { by: 2 } }", 1)
	files := maps.Clone(toolkits)
	files["sum.yaml"] = "flow: sum\ndescription: Add to a number.\ninput: { n: number, by: number }\nstages: { s: { job: { script: ./given.js, input: \"${input}\" } } }\noutput: ${stages.s.output}\n"
	files["given.js"] = `return { given: input }`
	l := local(t, "all")
	id := startWith(t, l, def, files, map[string]any{"question": "q"})
	var children []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if e.ID == id && ask.Kind == "child" {
				children = append(children, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(children, " "); got != "sum" {
		t.Errorf("the child runs were: %s", got)
	}
	sum := offeredTool(m.Calls[0].Tools, "sum")
	if sum["description"] != "Add to a number." || !strings.Contains(jsonOf(sum["input_schema"]), `"n"`) || strings.Contains(jsonOf(sum["input_schema"]), `"by"`) {
		t.Errorf("sum was offered as %s", jsonOf(sum))
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `{\"given\":{\"by\":2,\"n\":5}}`) {
		t.Errorf("the call gave: %s", shown)
	}
}

// slow gives the helper's files with net's tools replaced by two whose
// calls do not end: export, whose work ends at the third ask, and choose,
// which asks for input.
func slow() map[string]string {
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = `
		toolkit: net
		description: The firewall and the switch.
		input: { asker: any }
		tools:
		  export:
		    description: Export something
		    args: { what: string }
		    job: { script: ./export.js, input: { what: "${args.what}", request: "${input.request}" } }
		  choose:
		    description: Choose a colour
		    job: { script: ./choose.js, input: { request: "${input.request}" } }
	`
	files["toolkits/export.js"] = `
		const r = input.request
		if (r.do === "cancel") return "cancelled at ask " + r.state.asked
		const asked = r.do === "call" ? 0 : r.state.asked
		if (asked < 2) return { working: { said: "asked " + (asked + 1), wait: 0.05 }, state: { asked: asked + 1 } }
		return "exported " + input.what + " at ask " + (asked + 1)`
	files["toolkits/choose.js"] = `
		const r = input.request
		if (r.do === "call") return { requests: { colour: { message: "Which colour?" } }, state: { step: 1 } }
		return "chose " + r.responses.colour + " at step " + r.state.step`
	return files
}

// A call of a toolkit's tool that ends working is a task: the model is
// shown its name, and carries it on with resume, which waits for the work
// and gives its result. resume with cancel: true cancels it. An agent that
// ends with a task working names it in its conversation, and a run that
// carries the conversation on resumes it.
func TestAgentResumesACallThatHasNotEnded(t *testing.T) {
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_export", `{"what": "notes"}`),
		calls("c3", "resume", `{"task": "task1"}`), calls("c4", "resume", `{"task": "task1"}`), says("done"))
	l := local(t, "all")
	id := startWith(t, l, helper(m.URL, ""), slow(), map[string]any{"question": "q"})
	var jobs []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if e.ID == id && (ask.Kind == "script" || ask.Kind == "child") {
				jobs = append(jobs, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(jobs, " "); got != "turn open turn#2 net_export turn#3 resume turn#4 turn#5" {
		t.Errorf("the jobs were: %s", got)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[2]), `\`, ""); !strings.Contains(shown, `"task":"task1"`) || !strings.Contains(shown, `"said":"asked 1"`) {
		t.Errorf("the call that did not end was shown as: %s", shown)
	}
	if got := offeredNames(m.Calls[1].Tools); strings.Contains(got, "resume") {
		t.Errorf("with no task, the turn was offered: %s", got)
	}
	resume := offeredTool(m.Calls[2].Tools, "resume")
	if resume == nil || strings.Contains(resume["description"].(string), "task1") {
		t.Errorf("resume was offered as %s", jsonOf(resume))
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "exported notes at ask 3") {
		t.Errorf("the resume gave: %s", shown)
	}
	if got := offeredNames(m.Calls[3].Tools); strings.Contains(got, "resume") {
		t.Errorf("with the task ended, the turn was offered: %s", got)
	}
	if shown := lastShown(m.Calls[4]); !strings.Contains(shown, `there is no task named \"task1\"`) {
		t.Errorf("the resume of a task that ended was shown as: %s", shown)
	}
	// A cancel is the tool's job, asked to cancel.
	m = newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_export", `{"what": "notes"}`),
		calls("c3", "resume", `{"task": "task1", "cancel": true}`), says("done"))
	if r := wait(t, l, startWith(t, l, helper(m.URL, ""), slow(), map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "cancelled at ask 1") {
		t.Errorf("the cancel gave: %s", shown)
	}
	// An agent that ends with a task working names it, and another carries it on.
	m = newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_export", `{"what": "notes"}`), says("later"))
	r = wait(t, l, startWith(t, l, helper(m.URL, ""), slow(), map[string]any{"question": "q"}))
	conversation := r.Output.(map[string]any)["conversation"].(map[string]any)
	if got := jsonOf(conversation["tasks"]); got != `{"task1":{"args":{"what":"notes"},"entry":"net","state":{"asked":1},"tool":"export"}}` {
		t.Fatalf("the conversation names the tasks as: %s", got)
	}
	m2 := newModel(t, calls("c3", "resume", `{"task": "task1"}`), says("done"))
	wait(t, l, startWith(t, l, helper(m2.URL, "conversation: ${input.so}"), slow(), map[string]any{"question": "and?", "so": carried(conversation, r.ID)}))
	if shown := lastShown(m2.Calls[1]); !strings.Contains(shown, "exported notes at ask 3") {
		t.Errorf("the run that carried on resumed the task as: %s", shown)
	}
}

// A call that asks for input is a task too: the model is shown the requests
// and gives its responses with resume. A resume is the call carried on, so
// the hook that holds the entry's jobs holds it, reads the requests and the
// model's responses, and gives a person's answer in place of the model's.
func TestAgentHookHoldsAResume(t *testing.T) {
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_choose", `{}`),
		calls("c3", "resume", `{"task": "task1", "responses": {"colour": "red"}}`), says("done"))
	def, files := approved(m.URL, `
		const held = input.job, tool = held.tool
		if (!tool.resume) return { ok: true }
		const fits = tool.toolkit === "net" && tool.name === "choose" && tool.resume === "task1" &&
			tool.requests.colour.message === "Which colour?" && tool.responses.colour === "red"
		if (!fits) return { ok: false, note: "the hook read " + JSON.stringify(tool) }
		const request = { ...held.input.request, responses: { colour: "blue" } }
		return { ok: true, input: { ...held.input, request } }`)
	for name, text := range slow() {
		files[name] = text
	}
	l := local(t, "all")
	if r := wait(t, l, startWith(t, l, def, files, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[2]), `\`, ""); !strings.Contains(shown, `"task":"task1"`) || !strings.Contains(shown, `"message":"Which colour?"`) {
		t.Errorf("the call that asks for input was shown as: %s", shown)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "chose blue at step 1") {
		t.Errorf("the resume gave: %s", shown)
	}
}

// asking gives the helper with its tool add marked for approval and a hook
// whose input is the value given, and its files.
func asking(address, askInput string) (def string, files map[string]string) {
	files = maps.Clone(toolkits)
	files["ask.js"] = `return { ok: true, note: "the agent's hook" }`
	def = helper(address, "hooks: { check: { before: { script: ./ask.js, input: "+askInput+" }, when: \"${job.approve}\" } }")
	return strings.Replace(def, `job: { script: ./add.js, input: { n: "${args.n}" } }`, `job: { script: ./add.js, input: { n: "${args.n}" }, approve: true }`, 1), files
}

// A hook runs once for each job: two held calls of one turn start its job
// twice, each named for the call and the hook.
func Test_mach_a_hook_runs_once_for_each_job(t *testing.T) {
	twice := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c1", Name: "add", Args: `{"n": 1}`}, {ID: "c2", Name: "add", Args: `{"n": 5}`}}}
	m := newModel(t, twice, says("done"))
	def, files := asking(m.URL, `{ r: "${Math.random()}" }`)
	l := local(t, "all")
	id := startWith(t, l, def, files, map[string]any{"question": "q"})
	var asks []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		for _, ask := range e.Asks {
			if ask.Kind == "script" && strings.HasSuffix(ask.Name, ".check") {
				asks = append(asks, ask.Name)
			}
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	if got := strings.Join(asks, " "); got != "add.check add.1.check" {
		t.Errorf("the hook's jobs were: %s", got)
	}
	if shown := lastShown(m.Calls[1]); strings.Contains(shown, "denied") || !strings.Contains(shown, `"n":2`) && !strings.Contains(shown, `\"n\":2`) {
		t.Errorf("the last turn was shown: %s", shown)
	}
}

// A hook whose values cannot be filled in denies the job held for it, with a
// note that names the fault, and the agent carries on.
func TestAgentHookThatCannotBeFilledDenies(t *testing.T) {
	m := newModel(t, calls("c1", "add", `{"n": 1}`), says("done"))
	def, files := asking(m.URL, `{ who: "${input.question.nothing.here}" }`)
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, def, files, map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "denied: the hook check could not be started: hooks.check.before.input") {
		t.Errorf("the call was shown as: %s", shown)
	}
}

// The hook above an agent made while running runs for the agent's marked tool.
func Test_agent_made_while_running_takes_the_hooks_above_it(t *testing.T) {
	m := newModel(t, calls("c1", "act", `{}`), says("done"))
	made := strings.ReplaceAll(`agent: made
model: { name: test-model, service: anthropic, address: "ADDRESS", secret: TEST_KEY }
prompt: go
tools:
  act: { description: Act, job: { script: ./step.js, approve: true } }
`, "ADDRESS", m.URL)
	def := `
		flow: top
		hooks: { human: { before: ./human.js, when: "${job.approve}" } }
		stages:
		  make: { job: ./make.js }
		  go: { job: { machine: "${stages.make.output}", allow: [ all ] } }
	`
	files := map[string]string{
		"human.js": `return { ok: false, note: "the owner's hook, for " + input.job.run.machine }`,
		"make.js": "return machines.define(`" + made + "`, " +
			`{ files: { "./step.js": "return 7" } })`,
	}
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, def, files, nil))
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "the owner's hook, for made") {
		t.Errorf("the call was shown as: %s", shown)
	}
}

// following is what a run did: the script jobs it started, the jobs it
// held, and its hooks' jobs, each with the step that started it.
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
			case ask.Kind == "script" && strings.HasSuffix(ask.Name, ".check"):
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

// A skill a person writes is a toolkit whose instructions are its SKILL.md
// and whose files: are its other files. The toolkit then has the tool read,
// which gives one of them, and takes no other path.
func TestAgentOpensASkillWrittenAsAToolkit(t *testing.T) {
	files := maps.Clone(toolkits)
	files["toolkits/review/toolkit.yaml"] = `
		toolkit: review
		description: Review code using the team's checklist.
		instructions: { include: ./SKILL.md }
		files: [./references/checklist.md]
	`
	files["toolkits/review/SKILL.md"] = "# Code review\n\nRead `references/checklist.md`, then review the diff.\n"
	files["toolkits/review/references/checklist.md"] = "# Review checklist\n\nCheck correctness, tests, and compatibility.\n"
	m := newModel(t, calls("c1", "open", opens("payments")), calls("c2", "payments_read", `{"path": "references/checklist.md"}`),
		calls("c3", "payments_read", `{"path": "SKILL.md"}`), says("done"))
	def := strings.Replace(helper(m.URL, ""), "./toolkits/payments.yaml", "./toolkits/review/toolkit.yaml", 1)
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, def, files, map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "then review the diff") {
		t.Errorf("the open of the skill gave: %s", shown)
	}
	read := offeredTool(m.Calls[1].Tools, "payments_read")
	if read == nil || !strings.Contains(jsonOf(read["input_schema"]), `"enum":["references/checklist.md"]`) {
		t.Errorf("read was offered as %s", jsonOf(read))
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, "Check correctness, tests, and compatibility.") {
		t.Errorf("read gave: %s", shown)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "the arguments do not fit the tool") {
		t.Errorf("read of a file the toolkit does not list was shown as: %s", shown)
	}
	if _, err := machineParse("toolkit: k\ndescription: K\nfiles: [./a.md]\ntools:\n  read: { description: R, job: ./r.js }\n", map[string]string{"a.md": "a", "r.js": ""}); err == nil ||
		!strings.Contains(err.Error(), `tools.read: the tool "read" has the name of the tool a toolkit with files: has`) {
		t.Errorf("a toolkit with files: and a tool read: %v", err)
	}
}

// With tool_changes: true, the tools sent to the service are those of the
// first turn at every turn, and a tool added or removed later is a message
// of the conversation, at the place the change was made. A run that carries
// the conversation on sends the same tools and the same messages.
func TestAgentToolChangesAreSentAsMessages(t *testing.T) {
	changesIn := func(c llmstandin.Call) []string {
		var out []string
		for i, m := range c.Messages {
			message, _ := m.(map[string]any)
			if message["role"] != "system" {
				continue
			}
			for _, b := range message["content"].([]any) {
				block := b.(map[string]any)
				tool := block["tool"].(map[string]any)
				name, _ := tool["name"].(string)
				if definition, ok := tool["definition"].(map[string]any); ok {
					name = definition["name"].(string)
					if definition["input_schema"] == nil || definition["description"] == nil {
						t.Errorf("a tool was added without its definition: %v", definition)
					}
				}
				out = append(out, fmt.Sprintf("%d:%s:%s", i, block["type"], name))
			}
		}
		return out
	}
	with := func(address, extra string) string {
		return strings.Replace(helper(address, extra), "service: anthropic,", "service: anthropic, tool_changes: true,", 1)
	}
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_show", `{"command": "a"}`),
		calls("c3", "open", opens("payments")), says("done"))
	l := local(t, "all")
	r := wait(t, l, startHelper(t, l, with(m.URL, ""), map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	first := offeredNames(m.Calls[0].Tools)
	for i, c := range m.Calls {
		if got := offeredNames(c.Tools); got != first || c.Beta != "inline-tools-2026-09-15" {
			t.Errorf("turn %d was sent the tools %s with the beta %q; the first was sent %s", i+1, got, c.Beta, first)
		}
	}
	if got := changesIn(m.Calls[0]); len(got) != 0 {
		t.Errorf("the first turn was sent the changes %v", got)
	}
	if got, want := strings.Join(changesIn(m.Calls[1]), " "), "4:tool_addition:net_show"; got != want {
		t.Errorf("the turn after the open was sent the changes %s, want %s", got, want)
	}
	// With both open, open is removed, and the earlier change is where it was.
	if got, want := strings.Join(changesIn(m.Calls[3]), " "), "4:tool_addition:net_show 9:tool_removal:open 9:tool_addition:payments_pay"; got != want {
		t.Errorf("the last turn was sent the changes %s, want %s", got, want)
	}
	// The messages before a change are sent again as they were.
	before, _ := json.Marshal(m.Calls[1].Messages[:4])
	again, _ := json.Marshal(m.Calls[3].Messages[:4])
	if string(before) != string(again) {
		t.Errorf("the start of the conversation was sent as\n%s\nand then as\n%s", before, again)
	}
	m2 := newModel(t, says("again"))
	conversation := r.Output.(map[string]any)["conversation"]
	wait(t, l, startHelper(t, l, with(m2.URL, "conversation: ${input.so}"), map[string]any{"question": "and?", "so": carried(conversation, r.ID)}))
	if got := offeredNames(m2.Calls[0].Tools); got != first {
		t.Errorf("the run that carried on was sent the tools %s", got)
	}
	if got := changesIn(m2.Calls[0]); len(got) != 3 || got[0] != "4:tool_addition:net_show" {
		t.Errorf("the run that carried on was sent the changes %v", got)
	}
	// Without tool_changes, the tools sent are those of each turn, with no beta.
	m3 := newModel(t, calls("c1", "open", opens("net")), says("done"))
	wait(t, l, startHelper(t, l, helper(m3.URL, ""), map[string]any{"question": "q"}))
	if got := offeredNames(m3.Calls[1].Tools); !strings.Contains(got, "net_show") || m3.Calls[1].Beta != "" || len(changesIn(m3.Calls[1])) != 0 {
		t.Errorf("without tool_changes the second turn was sent %s, the beta %q and the changes %v", got, m3.Calls[1].Beta, changesIn(m3.Calls[1]))
	}
	// A service with no such message refuses the field.
	if _, err := machineParse(strings.Replace(with("http://127.0.0.1:1", ""), "service: anthropic, tool_changes: true,", "service: openai, tool_changes: true,", 1), toolkits); err == nil ||
		!strings.Contains(err.Error(), "tool_changes") {
		t.Errorf("tool_changes: on a model of the service openai: %v", err)
	}
}

// A list too large to offer, whose names fit in what the model is shown, is
// shown as its names. The agent keeps the entries as a blob, and of the
// list only the tools the model opened. An entry is opened by its name: a
// tool is offered from the next turn, and a toolkit is opened as any is.
func TestAgentLongListIsShownAsItsNames(t *testing.T) {
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = "toolkit: net\ndescription: The firewall and the switch.\ninput: { asker: any }\njob: ./many.js\n"
	files["toolkits/many.js"] = `
		const r = input.request
		const inside = (r.toolkit || []).join("/")
		if (r.do !== "open") return { result: inside + ":" + r.tool }
		if (inside !== "") return { instructions: "Review well.", entries: [{ kind: "tool", name: "read", description: "Reads", args: { type: "object" } }] }
		const entries = [{ kind: "toolkit", name: "review", description: "How to review." }]
		for (let i = 0; i < 80; i++) entries.push({ kind: "tool", name: "t" + i, description: "Tool " + i,
			args: { type: "object", properties: { note: { type: "string", description: "x".repeat(600) } } } })
		return { instructions: "Choose one.", entries }`
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "open", `{"list": "net", "name": "t42"}`),
		calls("c3", "open", `{"list": "net", "name": "review"}`), calls("c4", "open", `{"list": "net", "name": "none"}`),
		calls("c5", "net_t42", `{}`), calls("c6", "net_review_read", `{}`), says("done"))
	l := local(t, "all")
	r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"}))
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := strings.ReplaceAll(lastShown(m.Calls[1]), `\`, "")
	if !strings.Contains(shown, `"instructions":"Choose one."`) || !strings.Contains(shown, `{"description":"Tool 42","kind":"tool","name":"t42"}`) ||
		!strings.Contains(shown, `{"description":"How to review.","kind":"toolkit","name":"review"}`) || !strings.Contains(shown, `call open with { list: "net", name:`) {
		t.Errorf("the open of a long list was shown as: %.900s", shown)
	}
	if strings.Contains(shown, "xxxx") || strings.Contains(shown, `"blob"`) {
		t.Errorf("the names of a long list hold arguments, or are a blob: %.900s", shown)
	}
	if got := offeredNames(m.Calls[1].Tools); got != "add "+blobToolNames+" open" {
		t.Errorf("after the open of a long list, the turn was offered: %s", got)
	}
	if got := offeredNames(m.Calls[2].Tools); got != "add "+blobToolNames+" net_t42 open" {
		t.Errorf("after the open of a tool, the turn was offered: %s", got)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "Review well.") {
		t.Errorf("the open of a toolkit in a long list gave: %s", shown)
	}
	if shown := lastShown(m.Calls[4]); !strings.Contains(shown, `there is no entry named \"none\" in the list \"net\"`) {
		t.Errorf("the open of a name the list does not hold gave: %s", shown)
	}
	if shown := lastShown(m.Calls[5]); !strings.Contains(shown, ":t42") {
		t.Errorf("the call of the opened tool gave: %s", shown)
	}
	if shown := lastShown(m.Calls[6]); !strings.Contains(shown, "review:read") {
		t.Errorf("the call of the tool inside gave: %s", shown)
	}
	// Of the list, the conversation holds the blob of its entries and the
	// tool that was opened.
	net := r.Output.(map[string]any)["conversation"].(map[string]any)["opened"].(map[string]any)["net"].(map[string]any)
	if entries, _ := net["entries"].([]any); len(entries) != 1 || net["stored"] == nil || net["blob"] != nil {
		t.Errorf("the conversation holds the long list as: %.600s", jsonOf(net))
	}
}

// limits: { show: } is the most the model is shown in place: a larger
// result is a blob, and a blob tool gives the items that fit, with the place
// to go on from.
func TestAgentShowLimitsWhatTheModelIsShown(t *testing.T) {
	def := strings.Replace(agentDef("ADDRESS", "limits: { show: 2KB }"), "tools:\n", `tools:
		  lines:
		    description: Make some lines
		    args: { n: number }
		    job: { script: ./lines.js, input: { n: "${args.n}" } }
`, 1)
	files := map[string]string{"add.js": scripts["add.js"],
		"lines.js": `const out = []; for (let i = 0; i < input.n; i++) out.push("line " + i + " " + "x".repeat(92)); return out`}
	m := newModel(t, calls("c1", "lines", `{"n": 10}`), calls("c2", "lines", `{"n": 40}`),
		calls("c3", "blob_read", `{"blob": "blob1"}`), calls("c4", "blob_search", `{"blob": "blob1", "find": "line"}`), says("done"))
	l := local(t, "all")
	if r := runAgent(t, l, strings.Replace(def, "ADDRESS", m.URL, 1), files); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "line 9 ") || strings.Contains(shown, "blob1") {
		t.Errorf("a result under the limit was shown as: %.300s", shown)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[2]), `\`, ""); !strings.Contains(shown, `"blob":"blob1"`) || !strings.Contains(shown, `"items":40`) {
		t.Errorf("a result over the limit was shown as: %.300s", shown)
	}
	for i, tool := range []string{"blob_read", "blob_search"} {
		shown := strings.ReplaceAll(lastShown(m.Calls[3+i]), `\`, "")
		if len(shown) > 2048+200 || !strings.Contains(shown, "line 0 ") || strings.Contains(shown, "line 30 ") || !strings.Contains(shown, `"next":`) || strings.Contains(shown, "blob2") {
			t.Errorf("%s gave %d bytes: %.300s", tool, len(shown), shown)
		}
	}
}

// A blob among the arguments of a toolkit's tool is the name the model was
// shown: the agent turns it into the blob, and the tool's job reads it.
func TestAgentGivesAToolkitsToolABlob(t *testing.T) {
	files := maps.Clone(toolkits)
	files["toolkits/net.yaml"] = `
		toolkit: net
		description: The firewall and the switch.
		input: { asker: any }
		tools:
		  make:
		    description: Make a file
		    args: { text: string }
		    job: { script: ./make.js, input: { text: "${args.text}" } }
		  size:
		    description: Count a file's bytes
		    args: { file: blob }
		    job: { script: ./size.js, input: { file: "${args.file}" } }
	`
	files["toolkits/make.js"] = `return { file: blobs.make(input.text) }`
	files["toolkits/size.js"] = `return "read: " + blobs.read(input.file, { as: "text" })`
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_make", `{"text": "some text"}`),
		calls("c3", "net_size", `{"file": "blob1"}`), calls("c4", "net_size", `{"file": "blob9"}`), says("done"))
	l := local(t, "all")
	if r := wait(t, l, startWith(t, l, helper(m.URL, ""), files, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if offered := strings.ReplaceAll(jsonOf(m.Calls[1].Tools), `\`, ""); !strings.Contains(offered, `"format":"blob","type":"string"}},"required":["file"]`) {
		t.Errorf("the tool's blob was offered as: %s", offered)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "read: some text") {
		t.Errorf("the tool given a blob gave: %s", shown)
	}
	if shown := lastShown(m.Calls[4]); !strings.Contains(shown, `should name a blob the conversation has shown`) {
		t.Errorf("a name that is no blob's gave: %s", shown)
	}
	if all := jsonOf(m.Calls); strings.Contains(all, `$blob`) {
		t.Fatalf("a blob's reference reached the model: %.600s", all)
	}
}

// A resume sends the blob the call was sent, and a hook reads the
// arguments of the call and of its resume as the model gave them.
func TestAgentResumeSendsTheBlobAgain(t *testing.T) {
	m := newModel(t, calls("c1", "open", opens("net")), calls("c2", "net_make", `{"text": "some text"}`),
		calls("c3", "net_later", `{"file": "blob1"}`), calls("c4", "resume", `{"task": "task1"}`), says("done"))
	def, files := approved(m.URL, `
		const tool = input.job.tool
		if (tool.name === "later" && tool.args.file !== "blob1") return { ok: false, note: "the hook read " + JSON.stringify(tool.args) }
		return { ok: true }`)
	files["toolkits/net.yaml"] = `
		toolkit: net
		description: The firewall and the switch.
		input: { asker: any }
		tools:
		  make:
		    description: Make a file
		    args: { text: string }
		    job: { script: ./make.js, input: { text: "${args.text}" } }
		  later:
		    description: Read a file, later
		    args: { file: blob }
		    job: { script: ./later.js, input: { file: "${args.file}", request: "${input.request}" } }
	`
	files["toolkits/make.js"] = `return { file: blobs.make(input.text) }`
	files["toolkits/later.js"] = `
		if (input.request.do === "call") return { requests: { go: { message: "Go on?" } }, state: { step: 1 } }
		return "read later: " + blobs.read(input.file, { as: "text" })`
	l := local(t, "all")
	if r := wait(t, l, startWith(t, l, def, files, map[string]any{"question": "q"})); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "task1") {
		t.Errorf("the call was shown as: %s", shown)
	}
	if shown := lastShown(m.Calls[4]); !strings.Contains(shown, "read later: some text") {
		t.Errorf("the resume gave: %s", shown)
	}
}
