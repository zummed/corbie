package client_test

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
	"github.com/zummed/corbie/store"
)

// enrolled trusts a client with a ceiling, as the controller's owner does.
func enrolled(t *testing.T, l *client.Local, name string, ceiling ...string) *client.Client {
	t.Helper()
	id, err := identity.New(name, identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.WriteTrust(ctx, l.Controller.Store, l.Controller.ID, identity.Trust{Card: id.Card, Ceiling: permission.Layer{Allow: ceiling}}); err != nil {
		t.Fatal(err)
	}
	return client.New(id, l.Controller.ID.Card, l.Memory.As(store.Party{Kind: store.Client, Name: name, Controller: l.Controller.ID.Name}))
}

// submitted submits a directory as a tree of a client, and waits for its
// receipt.
func submitted(t *testing.T, c *client.Client, dir, at string) {
	t.Helper()
	tree, err := client.ReadTree(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	message, err := c.Submit(ctx, tree, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := c.WaitReceipt(ctx, message, 0, interval); err != nil || !r.Taken {
		t.Fatalf("the submit gave %+v (%v)", r, err)
	}
}

// netops is a tree of another client's, whose machine show takes calls from
// the agents of /x.
func netops(receive string) map[string]string {
	files := map[string]string{
		"tools/show.yaml": `
			flow: show
			description: Run one show command on a device.
			input: { device: string, command: string }
			receive: [RECEIVE]
			stages: { s: { job: { script: ./given.js, input: "${input}" } } }
		`,
		"tools/given.js": `return { given: input }`,
	}
	files["tools/show.yaml"] = strings.Replace(files["tools/show.yaml"], "RECEIVE", receive, 1)
	return files
}

// support is a tree with one agent, whose tools are written in.
func support(address, tools string) map[string]string {
	return map[string]string{"agents/support.yaml": strings.ReplaceAll(strings.ReplaceAll(`
		agent: support
		input: { question: any }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		prompt: ${input.question}
		tools:
		  TOOLS
	`, "ADDRESS", address), "TOOLS", tools)}
}

// A tool written call: with a location in full is offered the description
// and the input of the machine recorded there when the run starts, without
// the names the tool fixes, and its call runs there. A run keeps what it
// read: the machine changed and submitted again is offered changed to the
// next run, and unchanged to a run already started.
func TestAgentToolCallsAMachineOfAnotherTree(t *testing.T) {
	gate := make(chan struct{})
	replies := []llmstandin.Reply{calls("c1", "show", `{"command": "uptime"}`), says("done"), says("done")}
	m := &model{Server: &llmstandin.Server{Answer: func(c llmstandin.Call) llmstandin.Reply {
		if c.Turn == 1 {
			<-gate
		}
		return replies[c.Turn-1]
	}}}
	web := httptest.NewServer(m.Server)
	t.Cleanup(web.Close)
	m.URL = web.URL
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	ops := enrolled(t, l, "netops", "at:/netops")
	submitted(t, ops, house(t, netops("/x/agents/support")), "/netops")
	dir := house(t, support(m.URL, "show: { call: /netops/tools/show, input: { device: fw1 } }"))
	if tree, err := client.ReadTree(dir, "/x"); err != nil || tree.Machines[0].Problem != nil {
		t.Fatalf("the tree reads with %v (%v)", tree.Machines[0].Problem, err)
	}
	submitTree(t, l, dir, "/x")
	first, err := l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Taken(ctx, first, 0, interval); err != nil {
		t.Fatal(err)
	}
	changed := netops("/x/agents/support")
	changed["tools/show.yaml"] = strings.NewReplacer("Run one show", "Run any show", "command: string }", `command: string, vrf: "string?" }`).Replace(changed["tools/show.yaml"])
	submitted(t, ops, house(t, changed), "/netops")
	close(gate)
	if r := wait(t, l, first); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	second, err := l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, second); r.Status != "done" {
		t.Fatalf("the second agent ended %s: %s", r.Status, r.Error)
	}
	was := map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}},
		"required": []any{"command"}, "additionalProperties": false}
	for _, i := range []int{0, 1} {
		show := offeredTool(m.Calls[i].Tools, "show")
		if show["description"] != "Run one show command on a device." || !reflect.DeepEqual(show["input_schema"], was) {
			t.Errorf("turn %d of the first run was offered show as %s", i+1, jsonOf(show))
		}
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `{\"given\":{\"command\":\"uptime\",\"device\":\"fw1\"}}`) {
		t.Errorf("show gave: %s", shown)
	}
	show := offeredTool(m.Calls[2].Tools, "show")
	properties, _ := show["input_schema"].(map[string]any)["properties"].(map[string]any)
	if show["description"] != "Run any show command on a device." || properties["vrf"] == nil || properties["device"] != nil {
		t.Errorf("the second run was offered show as %s", jsonOf(show))
	}
}

// A tool written call: with a location in full that is offered and cannot
// be read fails the agent at its start, naming the tool's place and the
// reason; one that is not offered is not read against.
func TestAgentToolOfAnotherTreeThatCannotBeRead(t *testing.T) {
	m := newModel(t, says("done"), says("done"))
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	ops := enrolled(t, l, "netops", "at:/netops")
	files := netops("/x/agents/other")
	files["tools/ping.yaml"] = "flow: ping\ndescription: Ping a device.\ninput: { device: string }\nreceive: [/x/agents/support]\nstages: { s: { job: ./given.js } }\n"
	submitted(t, ops, house(t, files), "/netops")
	team := enrolled(t, l, "team", "at:/team", "network", "secret:*@*")
	submitted(t, team, house(t, support(m.URL, "show: { call: /netops/tools/show }")), "/team")
	cases := []struct{ name, tools, want string }{
		{"receive", "show: { call: /netops/tools/show }",
			"tools.show.call: /netops/tools/show does not receive a call from the run "},
		{"nothing recorded", "none: { call: /netops/tools/none }",
			"tools.none.call: nothing is recorded at /netops/tools/none: a call starts a run of a transient location"},
		{"a fixed name", "ping: { call: /netops/tools/ping, input: { vrf: red } }",
			`tools.ping.input.vrf: the machine at /netops/tools/ping takes no input named "vrf"`},
	}
	for _, c := range cases {
		submitTree(t, l, house(t, support(m.URL, c.tools)), "/x")
		id, err := l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q"}})
		if err != nil {
			t.Fatal(err)
		}
		if r := wait(t, l, id); r.Status != "failed" || !strings.Contains(r.Error, c.want) {
			t.Errorf("%s: the agent ended %s: %s", c.name, r.Status, r.Error)
		}
	}
	id, err := team.Start(ctx, client.Start{At: "/team/agents/support", Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := team.Wait(ctx, id, interval); err != nil || r.Status != "failed" ||
		!strings.Contains(r.Error, "tools.show.call: ") || !strings.Contains(r.Error, `the ceiling of the client "team"`) {
		t.Errorf("an agent whose ceiling lacks send: ended %+v (%v)", r, err)
	}
	submitTree(t, l, house(t, support(m.URL, `none: { call: /netops/tools/none, when: "${input.question == 'never'}" }`)), "/x")
	id, err = l.Start(ctx, client.Start{At: "/x/agents/support", Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Errorf("an agent whose tool is not offered ended %s: %s", r.Status, r.Error)
	}
}
