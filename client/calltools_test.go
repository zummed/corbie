package client_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
)

// orders is a tree whose agent has a toolkit of tools that call machines of
// the tree, each fixing the project, as a project of Rook's writes one.
func orders(address string) map[string]string {
	return map[string]string{
		".corbieignore": "toolkits/\n",
		"agents/support.yaml": strings.ReplaceAll(`
			agent: support
			input: { question: any, team: string }
			model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
			prompt: ${input.question}
			ask: toolkits/ask.js
			toolkits:
			  orders: { include: toolkits/orders.yaml }
			  infra: { include: toolkits/infra.yaml }
		`, "ADDRESS", address),
		"toolkits/infra.yaml": `
			description: The servers.
			tools:
			  vm_status: { call: infra/vms/status }
		`,
		"infra/vms/status.yaml": `
			flow: status
			description: The state of every virtual machine.
			stages: { s: { job: ./status.js } }
			output: ${stages.s.output}
		`,
		"infra/vms/status.js": `return "all running"`,
		"toolkits/orders.yaml": `
			description: The orders app.
			tools:
			  read_file: { call: gitlab/files/read, input: { project: group/orders } }
			  change_file: { call: gitlab/commits/create, input: { project: "${input.team}" }, approve: true }
		`,
		"toolkits/ask.js": `
			const fits = input.jobs.filter((j) => j.asked.toolkit === "orders" && j.asked.description === "Change one file." &&
				j.asked.args.path === "a.txt" && !("project" in j.asked.args))
			return { approved: fits.map((j) => j.name), note: "shown " + JSON.stringify(input.jobs.map((j) => j.asked)) }`,
		"gitlab/files/read.yaml": `
			flow: read
			description: Read one file of a project.
			input: { project: string, path: string, ref: "string?" }
			stages: { s: { job: { script: ./given.js, input: "${input}" } } }
			output: ${stages.s.output}
		`,
		"gitlab/files/given.js": `return { given: input }`,
		"gitlab/commits/create.yaml": `
			flow: create
			description: Change one file.
			input: { project: string, path: string, text: string }
			stages: { s: { job: { script: gitlab/files/given.js, input: "${input}" } } }
			output: ${stages.s.output}
		`,
	}
}

// offeredTool gives one tool as a call to the model offered it.
func offeredTool(tools []any, name string) map[string]any {
	for _, tool := range tools {
		if t := tool.(map[string]any); t["name"] == name {
			return t
		}
	}
	return nil
}

// A tool written call: <location> is offered with the description and the
// input of the machine at the location, without the names it fixes. The
// machine is given the fixed input with the model's arguments, and an
// optional argument left out is absent. A call marked approve: true goes to
// the agent's ask, which is shown the machine's description, the model's
// arguments and the toolkit.
func TestAgentToolCallsAMachineOfTheTree(t *testing.T) {
	m := newModel(t,
		llmstandin.Reply{Text: "opening", Calls: []llmstandin.ToolCall{
			{ID: "c1", Name: "open", Args: `{"toolkit": "orders"}`},
			{ID: "c4", Name: "open", Args: `{"toolkit": "infra"}`},
		}},
		llmstandin.Reply{Text: "reading", Calls: []llmstandin.ToolCall{
			{ID: "c2", Name: "read_file", Args: `{"path": "a.txt"}`},
			{ID: "c3", Name: "change_file", Args: `{"path": "a.txt", "text": "new"}`},
			{ID: "c5", Name: "vm_status", Args: `{}`},
		}},
		says("done"))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	dir := house(t, orders(m.URL))
	submitTree(t, l, dir, "/x")
	id, err := l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q", "team": "group/team"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	read := offeredTool(m.Calls[1].Tools, "read_file")
	want := map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "ref": map[string]any{"type": "string"}},
		"required": []any{"path"}, "additionalProperties": false}
	if read["description"] != "Read one file of a project." || !reflect.DeepEqual(read["input_schema"], want) {
		t.Fatalf("read_file was offered as %s", jsonOf(read))
	}
	status := offeredTool(m.Calls[1].Tools, "vm_status")
	if !reflect.DeepEqual(status["input_schema"], map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}) {
		t.Fatalf("vm_status was offered as %s", jsonOf(status))
	}
	shown := lastShown(m.Calls[2])
	if !strings.Contains(shown, `"content":"all running"`) {
		t.Errorf("vm_status gave: %s", shown)
	}
	if !strings.Contains(shown, `{\"given\":{\"path\":\"a.txt\",\"project\":\"group/orders\"}}`) {
		t.Errorf("read_file gave: %s", shown)
	}
	if !strings.Contains(shown, `{\"given\":{\"path\":\"a.txt\",\"project\":\"group/team\",\"text\":\"new\"}}`) || strings.Contains(shown, "denied") {
		t.Errorf("change_file gave: %s", shown)
	}
}

// A tool that calls a machine takes only the names the machine's input
// gives and the tool does not fix: the model giving a fixed name, or any
// other, does not fit, and runs nothing.
func Test_agent_call_tool_arguments_are_closed(t *testing.T) {
	m := newModel(t,
		calls("c1", "open", `{"toolkit": "orders"}`),
		llmstandin.Reply{Text: "reading", Calls: []llmstandin.ToolCall{
			{ID: "c2", Name: "read_file", Args: `{"path": "a.txt", "project": "group/secret"}`},
			{ID: "c3", Name: "read_file", Args: `{"path": "a.txt", "branch": "main"}`},
		}},
		says("done"))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	submitTree(t, l, house(t, orders(m.URL)), "/x")
	id, err := l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q", "team": "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := lastShown(m.Calls[2])
	if strings.Count(shown, "the arguments do not fit the tool") != 2 || strings.Contains(shown, "given") {
		t.Fatalf("the calls were answered: %s", shown)
	}
}

// A tool that calls an agent is shown what that agent replied.
func TestAgentToolCallsAnAgent(t *testing.T) {
	m := newModel(t, calls("c1", "helper", `{"question": "how?"}`), says("like this"), says("done"))
	files := orders(m.URL)
	files["agents/support.yaml"] = strings.Replace(files["agents/support.yaml"], "toolkits:\n", "tools:\n\t\t\t  helper: { call: agents/helper }\n\t\t\ttoolkits:\n", 1)
	files["agents/helper.yaml"] = strings.ReplaceAll(`
		agent: helper
		description: Answer a question.
		input: { question: string }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		prompt: ${input.question}
	`, "ADDRESS", m.URL)
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	dir := house(t, files)
	tree, err := client.ReadTree(dir, "/x")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range tree.Machines {
		if m.Problem != nil {
			t.Fatalf("%s: %v", m.Path, m.Problem)
		}
	}
	submitTree(t, l, dir, "/x")
	id, err := l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q", "team": "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[2]); !strings.Contains(shown, `"content":"like this"`) || strings.Contains(shown, "conversation") {
		t.Fatalf("the helper was shown as: %s", shown)
	}
}

// A blob in the called machine's input is an argument the model gives as
// the name it was shown.
func TestAgentCallToolTakesABlobByItsName(t *testing.T) {
	files := orders("http://127.0.0.1:1")
	files["gitlab/files/read.yaml"] = strings.Replace(files["gitlab/files/read.yaml"], `ref: "string?"`, "attached: blob", 1)
	tree, err := client.ReadTree(house(t, files), "/x")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range tree.Machines {
		if m.Problem != nil {
			t.Fatalf("%s: %v", m.Path, m.Problem)
		}
		if m.Path != "agents/support.yaml" {
			continue
		}
		var body struct {
			Tools map[string]struct {
				Args map[string]any `json:"args"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(m.Draft.Definition.Body, &body); err != nil {
			t.Fatal(err)
		}
		args := body.Tools["read_file"].Args["properties"].(map[string]any)
		if !reflect.DeepEqual(args["attached"], map[string]any{"type": "string", "format": "blob"}) {
			t.Fatalf("the blob is an argument %v", args["attached"])
		}
	}
}

// What a tool that calls a machine names is checked as the tree is read,
// and a file with any of these problems is not submitted.
func TestAgentCallToolIsRefused(t *testing.T) {
	cases := []struct {
		name, tool, want string
		files            map[string]string
	}{
		{"no machine", "{ call: nowhere }", "tools.t.call: there is no machine at nowhere: the tree has no file nowhere.yaml", nil},
		{"a listed file", "{ call: toolkits/orders }", "tools.t.call: there is no machine at toolkits/orders: .corbieignore lists toolkits/orders.yaml", nil},
		{"no description", "{ call: plain }", "tools.t.call: the machine at plain has no description:",
			map[string]string{"plain.yaml": "flow: plain\nstages: { s: { job: gitlab/files/given.js } }\n"}},
		{"a fixed name not in the input", "{ call: gitlab/files/read, input: { branch: main } }", `tools.t.input.branch: the machine at gitlab/files/read takes no input named "branch"`, nil},
		{"job beside call", "{ call: gitlab/files/read, job: ./x.js }", "tools.t.job: a tool with call: has no job: the description and arguments come from the machine called", nil},
		{"args beside call", "{ call: gitlab/files/read, args: { a: string } }", "tools.t.args: a tool with call: has no args:", nil},
		{"description beside call", "{ call: gitlab/files/read, description: Read }", "tools.t.description: a tool with call: has no description:", nil},
		{"a singleton", "{ call: always }", "tools.t.call: the machine at always is singleton: a call starts a run only of a transient location",
			map[string]string{"always.yaml": "statechart: always\ndescription: Always.\nlifetime: singleton\ninitial: a\nstates: { a: {} }\n"}},
		{"an absolute location", "{ call: /x/gitlab/files/read }", `tools.t.call: "/x/gitlab/files/read": the location is one in the same tree`, nil},
		{"an expression", `{ call: "${input.where}" }`, "tools.t.call: \"${input.where}\": the location is written out", nil},
		{"input without call", `{ description: D, args: {}, job: gitlab/files/given.js, input: { a: 1 } }`, "tools.t.input: input: belongs to a tool with call:", nil},
		{"approve without call", `{ description: D, args: {}, job: gitlab/files/given.js, approve: true }`, "tools.t.approve: approve: belongs to a tool with call:", nil},
		{"an input written with allOf", "{ call: anyof }", "tools.t.call: the input of the machine at anyof is written with allOf",
			map[string]string{"anyof.yaml": "flow: anyof\ndescription: A.\ninput: { type: object, allOf: [{ required: [a] }] }\nstages: { s: { job: gitlab/files/given.js } }\n"}},
	}
	for _, c := range cases {
		files := orders("http://127.0.0.1:1")
		for name, text := range c.files {
			files[name] = text
		}
		files["agents/support.yaml"] = strings.Replace(files["agents/support.yaml"], "toolkits:\n", "tools:\n\t\t\t  t: "+c.tool+"\n\t\t\ttoolkits:\n", 1)
		tree, err := client.ReadTree(house(t, files), "/x")
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range tree.Machines {
			if m.Path == "agents/support.yaml" && (m.Problem == nil || !strings.Contains(m.Problem.Error(), c.want)) {
				t.Errorf("%s: %v", c.name, m.Problem)
			}
		}
	}
}

// A tool that calls a location is read with its tree: it is refused in a
// machine made while running, and in a file read alone.
func Test_agent_call_tool_needs_its_tree(t *testing.T) {
	support := dedent(orders("http://127.0.0.1:1")["agents/support.yaml"])
	support = strings.Replace(support, "toolkits:\n  orders: { include: toolkits/orders.yaml }\n", "tools:\n  t: { call: gitlab/files/read }\n", 1)
	if _, err := machine.ParseMade([]byte(support), machine.Given(nil)); err == nil || !strings.Contains(err.Error(), "tools.t.call: a machine made while running has no tree, and calls no location") {
		t.Errorf("made while running: %v", err)
	}
	dir := house(t, orders("http://127.0.0.1:1"))
	alone, err := client.ReadFile(dir, "agents/support.yaml", "/alone", client.ScriptJob{})
	if err != nil {
		t.Fatal(err)
	}
	if err := alone.Machines[0].Problem; err == nil || !strings.Contains(err.Error(), "toolkits/orders.yaml: toolkits.orders.tools.read_file.call: the machine at gitlab/files/read is read from the tree this file is in: give the directory, as in corbie test <dir>") {
		t.Errorf("read alone: %v", err)
	}
}
