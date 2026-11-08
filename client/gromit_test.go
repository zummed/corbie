package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
)

// A gromit is the worked example in testdata/gromit, a project as Rook lays
// one out, submitted at /acme with stand-ins for Slack, GitLab, the
// hypervisor and the model. Slack is the test: it sends the listener what a
// person writes and presses.
type gromit struct {
	t          *testing.T
	l          *client.Local
	dir        string
	model      *llmstandin.Server
	hypervisor *hypervisor
	submitted  *instance.Submitted
	tree       *client.Tree
}

// A hypervisor keeps each restart it was asked for, with the key it was
// sent.
type hypervisor struct {
	mu       sync.Mutex
	restarts []string
}

func (h *hypervisor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	h.restarts = append(h.restarts, string(body)+" with "+r.Header.Get("Authorization"))
	h.mu.Unlock()
	io.WriteString(w, "restarted "+string(body))
}

func (h *hypervisor) seen() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.restarts)
}

// The model's replies, by the question that started the agent's run, one
// for each turn.
var gromitReplies = map[string][]llmstandin.Reply{
	"orders-2 is stuck, can you restart it?": {
		calls("c1", "open", `{"toolkit": "infra"}`),
		{Text: "restarting", Calls: []llmstandin.ToolCall{
			{ID: "c2", Name: "restart", Args: `{"name": "orders-2"}`},
			{ID: "c3", Name: "console", Args: `{"name": "orders-2"}`},
		}},
		says("Restarted orders-2."),
	},
	"restart orders-3": {
		calls("c1", "open", `{"toolkit": "infra"}`),
		calls("c2", "restart", `{"name": "orders-3"}`),
		says("I could not restart orders-3."),
	},
	"what is in config/app.yaml?": {
		calls("c1", "open", `{"toolkit": "orders"}`),
		{Text: "reading", Calls: []llmstandin.ToolCall{
			{ID: "c2", Name: "change_file", Args: `{"path": "config/app.yaml", "text": "x"}`},
			{ID: "c3", Name: "read_file", Args: `{"path": "config/app.yaml"}`},
		}},
		says("It holds the settings."),
	},
}

// newGromit copies the example, points the agent at a stand-in model and
// makes its quiet timer two seconds, sets the secrets and values at their
// directories, and submits it.
func newGromit(t *testing.T) *gromit {
	t.Helper()
	g := &gromit{t: t, l: local(t, "all"), dir: filepath.Join(t.TempDir(), "gromit"), hypervisor: &hypervisor{}}
	if err := os.CopyFS(g.dir, os.DirFS("testdata/gromit")); err != nil {
		t.Fatal(err)
	}
	g.model = &llmstandin.Server{Key: "sk-anthropic", Answer: func(c llmstandin.Call) llmstandin.Reply {
		replies := gromitReplies[promptOf(c)]
		if turn := (len(c.Messages) + 1) / 2; turn <= len(replies) {
			return replies[turn-1]
		}
		return llmstandin.Reply{Error: "the script has no reply for this turn"}
	}}
	model := httptest.NewServer(g.model)
	t.Cleanup(model.Close)
	hv := httptest.NewServer(g.hypervisor)
	t.Cleanup(hv.Close)
	g.edit("agents/support.yaml", "secret: ANTHROPIC_KEY }", "secret: ANTHROPIC_KEY, address: "+model.URL+" }")
	g.edit("conversation.yaml", "quiet: 1d", "quiet: 2s")
	for _, set := range []struct{ at, name, value string }{
		{"/acme/agents", "ANTHROPIC_KEY", "sk-anthropic"},
		{"/acme/infra", "HYPERVISOR_KEY", "hv-key-7"},
	} {
		if err := g.l.SetSecret(ctx, set.at, set.name, set.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.l.SetValue(ctx, "/acme/infra", "HYPERVISOR", strings.TrimPrefix(hv.URL, "http://")); err != nil {
		t.Fatal(err)
	}
	var err error
	if g.tree, err = client.ReadTree(g.dir, "/acme"); err != nil {
		t.Fatal(err)
	}
	g.submitted = submitTree(t, g.l, g.dir, "/acme")
	return g
}

// promptOf gives the question a call to the model was asked first.
func promptOf(c llmstandin.Call) string {
	first := jsonOf(c.Messages[0])
	for prompt := range gromitReplies {
		if strings.Contains(first, prompt) {
			return prompt
		}
	}
	return ""
}

// callsFor gives the calls to the model of the run that a question started.
func (g *gromit) callsFor(prompt string) []llmstandin.Call {
	var out []llmstandin.Call
	for _, c := range g.model.Calls {
		if promptOf(c) == prompt {
			out = append(out, c)
		}
	}
	return out
}

func (g *gromit) edit(name, old, new string) {
	g.t.Helper()
	full := filepath.Join(g.dir, name)
	text, err := os.ReadFile(full)
	if err != nil || !strings.Contains(string(text), old) {
		g.t.Fatalf("%s does not hold %q (%v)", name, old, err)
	}
	if err := os.WriteFile(full, []byte(strings.Replace(string(text), old, new, 1)), 0o644); err != nil {
		g.t.Fatal(err)
	}
}

// running gives the run at a location that is running, once there is one
// that is not among those given.
func (g *gromit) running(filter client.Filter, not ...string) *client.Run {
	g.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := g.l.List(ctx, filter)
		if err != nil {
			g.t.Fatal(err)
		}
		for _, r := range runs {
			// A child runs at its parent's location, so a filter by
			// location alone keeps roots.
			root := filter.Parent != "" || r.Parent == ""
			if r.Status == "running" && root && !slices.Contains(not, r.ID) {
				return r
			}
		}
		time.Sleep(interval)
	}
	g.t.Fatalf("no run of %+v came to run; the runs are:\n%s", filter, g.runs())
	return nil
}

// runs gives every run, one to a line, with a failed run's error, for a
// test that fails.
func (g *gromit) runs() string {
	all, _ := g.l.List(ctx, client.Filter{Finished: true})
	var lines []string
	for _, r := range all {
		if shown, err := g.l.Show(ctx, r.ID); err == nil && r.Status == "failed" {
			r = shown
		}
		lines = append(lines, strings.Join([]string{r.ID, r.Parent, r.Location, r.Machine, r.Status, r.Error, r.Refused}, " "))
	}
	return strings.Join(lines, "\n")
}

// router gives the run of a bot's router.
func (g *gromit) router(at string) *client.Run {
	g.t.Helper()
	return g.running(client.Filter{Location: at})
}

// listener gives the router's child that hears Slack.
func (g *gromit) listener(at string) *client.Run {
	g.t.Helper()
	return g.running(client.Filter{Parent: g.router(at).ID})
}

// slack sends the listener of Gromit what Slack would: heard for a
// message, pressed for a press of a button.
func (g *gromit) slack(event string, data map[string]any) {
	g.t.Helper()
	to := machine.Address{At: "/acme/gromit/slack/gromit", Run: g.listener("/acme/gromit/slack/gromit").ID}
	if _, err := g.l.Send(ctx, to, event, data, 0); err != nil {
		g.t.Fatal(err)
	}
}

// say is a person writing in a place of Slack.
func (g *gromit) say(email, place, chat, text string) {
	g.t.Helper()
	g.slack("heard", map[string]any{"place": place, "chat": chat, "email": email, "text": text})
}

// question waits for a question of Gromit's to be asked, and gives it.
func (g *gromit) question(not ...string) *client.Run {
	g.t.Helper()
	return g.running(client.Filter{Location: "/acme/gromit/slack/chat/ask"}, not...)
}

// press is a person pressing a button of a question.
func (g *gromit) press(email, choice string, at, run string) {
	g.t.Helper()
	g.slack("pressed", map[string]any{"to": map[string]any{"at": at, "run": run}, "email": email, "choice": choice})
}

// posted waits for Gromit to post in a chat, and gives what it posted.
func (g *gromit) posted(chat string) map[string]any {
	g.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := g.l.List(ctx, client.Filter{Location: "/acme/gromit/slack/chat/post", Status: "done", Finished: true})
		if err != nil {
			g.t.Fatal(err)
		}
		for _, r := range runs {
			out, _ := show(g.t, g.l, r.ID).Output.(map[string]any)
			if out["in"] == chat {
				return out
			}
		}
		time.Sleep(interval)
	}
	g.t.Fatalf("nothing was posted in %s; the runs are:\n%s", chat, g.runs())
	return nil
}

// The worked example of a project under Rook, run without services: a
// person asks in Slack, the router sends it to the conversation, the agent
// is offered the toolkits the person's scopes allow, a call that needs a
// yes goes to the agent's ask, which asks the approvers of the call's
// toolkit in that toolkit's channel and is answered with a press, and the
// reply is posted in the thread.
func TestWorkedExample(t *testing.T) {
	g := newGromit(t)

	t.Run("listed paths have no location and one file placed twice is two locations", func(t *testing.T) {
		g.t = t
		if !slices.Equal(g.tree.Listed, []string{".rook/", "people/scopes.yaml", "toolkits/"}) {
			t.Errorf("listed: %v", g.tree.Listed)
		}
		var at []string
		lifetimes := map[string]string{}
		for _, done := range g.submitted.Locations {
			at = append(at, done.Location)
			lifetimes[done.Location] = done.Lifetime + " " + done.What
		}
		want := []string{"/acme/agents/support", "/acme/conversation", "/acme/deploys", "/acme/gitlab/pipeline/create",
			"/acme/gitlab/repository/commits/create", "/acme/gitlab/repository/files/read", "/acme/gromit/slack/chat/ask",
			"/acme/gromit/slack/chat/post", "/acme/gromit/slack/gromit", "/acme/infra/vms/restart", "/acme/infra/vms/status",
			"/acme/ops/slack/chat/post", "/acme/ops/slack/ops", "/acme/people/find"}
		slices.Sort(at)
		if !slices.Equal(at, want) {
			t.Errorf("the locations are %v", at)
		}
		if lifetimes["/acme/gromit/slack/gromit"] != "singleton started" || lifetimes["/acme/ops/slack/ops"] != "singleton started" ||
			lifetimes["/acme/conversation"] != "keyed recorded" || lifetimes["/acme/gromit/slack/chat/post"] != "transient recorded" {
			t.Errorf("the submit did %v", lifetimes)
		}
		for _, m := range g.tree.Machines {
			if strings.HasSuffix(m.Location, "/slack/chat/post") && (m.Draft.Definition.Name != "slack-chat-post" || m.Draft.Definition.Type != "flow") {
				t.Errorf("%s is a %s named %s", m.Location, m.Draft.Definition.Type, m.Draft.Definition.Name)
			}
		}
		for _, at := range []string{"/acme/.rook/slack/chat/post", "/acme/people/scopes", "/acme/toolkits/infra"} {
			id, err := g.l.Start(ctx, client.Start{At: at, Input: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			if r := wait(t, g.l, id); r.Status != "failed" || !strings.Contains(r.Error, "nothing is recorded at "+at) {
				t.Errorf("a start at %s ended %s: %s", at, r.Status, r.Error)
			}
		}
	})

	t.Run("a held call goes to its toolkit's approvers and an approved one runs where its secret is set", func(t *testing.T) {
		g.t = t
		const asked = "orders-2 is stuck, can you restart it?"
		g.say("cy@acme.example", "#support", "C1/1", asked)
		q := g.question()
		if q.Cause.Run == "" {
			t.Fatalf("the question was started by nothing: %+v", q)
		}
		approval := show(t, g.l, q.Cause.Run)
		jobs, _ := approval.Input.(map[string]any)["jobs"].([]any)
		if approval.Machine != "approval" || approval.Location != "/acme/agents/support" || len(jobs) != 1 {
			t.Fatalf("the question was asked by %s at %s for %s", approval.Machine, approval.Location, jsonOf(approval.Input))
		}
		shown := jobs[0].(map[string]any)["asked"].(map[string]any)
		if shown["description"] != "Restart one virtual machine by its name." || shown["toolkit"] != "infra" ||
			!reflect.DeepEqual(shown["args"], map[string]any{"name": "orders-2"}) {
			t.Errorf("the ask was shown %s", jsonOf(shown))
		}
		g.press("bo@acme.example", "yes", "/acme/gromit/slack/chat/ask", q.ID)
		if got := g.posted("C1/1"); got["text"] != "Restarted orders-2." {
			t.Errorf("posted %v", got)
		}
		if r := wait(t, g.l, approval.ID); r.Status != "done" || !reflect.DeepEqual(r.Output, map[string]any{"approved": []any{"restart"}, "note": "Bo said yes."}) {
			t.Errorf("the ask ended %s with %s", r.Status, jsonOf(r.Output))
		}
		if got := g.hypervisor.seen(); !slices.Equal(got, []string{"orders-2 with hv-key-7"}) {
			t.Errorf("the hypervisor was asked %v", got)
		}
		runs, err := g.l.List(ctx, client.Filter{Location: "/acme/infra/vms/restart", Finished: true})
		if err != nil || len(runs) != 1 || runs[0].Status != "done" || !strings.Contains(strings.Join(show(t, g.l, runs[0].ID).Used, " "), "secret:HYPERVISOR_KEY@net.unencrypted:") {
			t.Errorf("the restart ran as %s (%v)", jsonOf(runs), err)
		}
		model := g.callsFor(asked)
		if len(model) != 3 || model[0].Key != "sk-anthropic" {
			t.Fatalf("the model was called %d times", len(model))
		}
		last := lastShown(model[2])
		if !strings.Contains(last, "restarted orders-2") {
			t.Errorf("the restart was shown as %s", last)
		}
		if !strings.Contains(last, "HYPERVISOR_KEY is not set at /acme/agents/support or above") {
			t.Errorf("console, at the agent's location, was shown as %s", last)
		}
	})

	t.Run("the model is offered the toolkits the asker's scopes allow, with arguments from the machines called", func(t *testing.T) {
		g.t = t
		model := g.callsFor("orders-2 is stuck, can you restart it?")
		if got := openDescription(model[0].Tools); !strings.Contains(got, "- infra: The servers.") || strings.Contains(got, "orders") {
			t.Errorf("Cy, with infra:read, was offered: %s", got)
		}
		if got := offeredNames(model[1].Tools); got != "blob_code blob_read blob_search console restart vm_status" {
			t.Errorf("with infra open, Cy was offered: %s", got)
		}
		restart := offeredTool(model[1].Tools, "restart")
		want := map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}},
			"required": []any{"name"}, "additionalProperties": false}
		if restart["description"] != "Restart one virtual machine by its name." || !reflect.DeepEqual(restart["input_schema"], want) {
			t.Errorf("restart was offered as %s", jsonOf(restart))
		}

		const asked = "what is in config/app.yaml?"
		g.say("ana@acme.example", "#support", "C3/3", asked)
		if got := g.posted("C3/3"); got["text"] != "It holds the settings." {
			t.Errorf("posted %v", got)
		}
		model = g.callsFor(asked)
		if got := openDescription(model[0].Tools); !strings.Contains(got, "- orders: The orders app.") || strings.Contains(got, "infra") {
			t.Errorf("Ana, with orders:read, was offered: %s", got)
		}
		if got := offeredNames(model[1].Tools); got != "blob_code blob_read blob_search read_file" {
			t.Errorf("with orders open and no orders:write, Ana was offered: %s", got)
		}
		read := offeredTool(model[1].Tools, "read_file")
		want = map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "ref": map[string]any{"type": "string"}},
			"required": []any{"path"}, "additionalProperties": false}
		if read["description"] != "Read one file of a project." || !reflect.DeepEqual(read["input_schema"], want) {
			t.Errorf("read_file was offered as %s", jsonOf(read))
		}
		last := lastShown(model[2])
		if !strings.Contains(last, "there is no tool named") || !strings.Contains(last, `{\"given\":{\"path\":\"config/app.yaml\",\"project\":\"group/orders\"}}`) {
			t.Errorf("change_file and read_file were shown as %s", last)
		}
	})

	t.Run("opening a toolkit gives its instructions", func(t *testing.T) {
		g.t = t
		for prompt, want := range map[string]string{
			"orders-2 is stuck, can you restart it?": "The virtual machines run on one hypervisor.",
			"what is in config/app.yaml?":            "The orders app lives in group/orders.",
		} {
			if shown := lastShown(g.callsFor(prompt)[1]); !strings.Contains(shown, want) {
				t.Errorf("opening gave %s", shown)
			}
		}
	})

	t.Run("a press the router's when: does not hold for is dropped and the router goes on", func(t *testing.T) {
		g.t = t
		const asked = "restart orders-3"
		router := g.router("/acme/gromit/slack/gromit").ID
		g.say("bo@acme.example", "dm", "D2", asked)
		q := g.question()
		g.press("bo@acme.example", "yes", "/elsewhere/gromit/slack/chat/ask", q.ID)
		g.press("bo@acme.example", "yes", "/acme/gromit/slack/chat/ask", q.ID)
		g.posted("D2")
		if r := show(t, g.l, router); r.Status != "running" {
			t.Errorf("the router is %s: %s", r.Status, r.Error)
		}
		runs, err := g.l.List(ctx, client.Filter{Location: "/acme/gromit/slack/chat/ask", Finished: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			if r.ID == q.ID && r.Status != "done" {
				t.Errorf("the question is %s", r.Status)
			}
		}
	})

	t.Run("a denied call goes back to the model", func(t *testing.T) {
		g.t = t
		const asked = "restart orders-3"
		if got := g.posted("D2"); got["text"] != "I could not restart orders-3." {
			t.Errorf("posted %v", got)
		}
		model := g.callsFor(asked)
		if shown := lastShown(model[2]); !strings.Contains(shown, "denied") || !strings.Contains(shown, "the yes did not count") {
			t.Errorf("Bo's own yes was shown as %s", shown)
		}
		for _, seen := range g.hypervisor.seen() {
			if strings.HasPrefix(seen, "orders-3") {
				t.Errorf("the hypervisor was asked to restart orders-3")
			}
		}
	})

	t.Run("a definition's approve: list holds a call in a keyed statechart until its ask approves", func(t *testing.T) {
		g.t = t
		g.say("di@acme.example", "#deploys", "C5/5", "deploy orders")
		q := g.question()
		approval := show(t, g.l, q.Cause.Run)
		jobs, _ := approval.Input.(map[string]any)["jobs"].([]any)
		if approval.Location != "/acme/deploys" || len(jobs) != 1 || jobs[0].(map[string]any)["name"] != "deploying" {
			t.Fatalf("the ask at %s was given %s", approval.Location, jsonOf(approval.Input))
		}
		g.press("di@acme.example", "yes", "/acme/gromit/slack/chat/ask", q.ID)
		if got := g.posted("C5/5"); got["text"] != "Deployed." {
			t.Errorf("posted %v", got)
		}
		runs, err := g.l.List(ctx, client.Filter{Location: "/acme/gitlab/pipeline/create", Finished: true})
		if err != nil || len(runs) != 1 {
			t.Fatalf("the pipeline ran %d times (%v)", len(runs), err)
		}
		if out := show(t, g.l, runs[0].ID).Output; !reflect.DeepEqual(out, map[string]any{"given": map[string]any{"project": "group/orders", "ref": "main"}}) {
			t.Errorf("the pipeline was given %s", jsonOf(out))
		}
	})

	t.Run("a listener that ends is started again under the same router", func(t *testing.T) {
		g.t = t
		router := g.router("/acme/gromit/slack/gromit").ID
		first := g.listener("/acme/gromit/slack/gromit").ID
		g.slack("lost", map[string]any{})
		second := g.running(client.Filter{Parent: router}, first).ID
		if r := show(t, g.l, first); r.Status != "done" {
			t.Errorf("the first listener is %s", r.Status)
		}
		if r := g.router("/acme/gromit/slack/gromit"); r.ID != router {
			t.Errorf("the router was started again: %s", r.ID)
		}
		if got := g.listener("/acme/gromit/slack/gromit").ID; got != second {
			t.Errorf("the listener is %s", got)
		}
	})

	t.Run("a state entered again in the step it was entered starts its timer again", func(t *testing.T) {
		g.t = t
		// A reaction in a thread with no conversation starts one, whose
		// first state takes it by entering itself again.
		g.slack("heard", map[string]any{"place": "#support", "chat": "C9/9", "email": "cy@acme.example", "change": "reaction"})
		run := g.running(client.Filter{Location: "/acme/conversation", Key: "C9/9"})
		waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if r, err := g.l.Wait(waitCtx, run.ID, interval); err != nil || r.Status != "done" {
			t.Errorf("the conversation with no message did not end on its quiet timer: %+v, %v", r, err)
		}
	})

	t.Run("a change to an included file changes every placement of it", func(t *testing.T) {
		g.t = t
		gromitRouter, opsRouter := g.router("/acme/gromit/slack/gromit").ID, g.router("/acme/ops/slack/ops").ID
		g.edit(".rook/slack/listen.yaml", "description: Hears one Slack workspace", "description: Hears a Slack workspace")
		g.edit(".rook/slack/chat/post.js", "return { in: input.in, text: input.text }", "return { in: input.in, text: input.text, version: 2 }")
		g.edit(".rook/slack/chat/post.yaml", "description: Posts text in a channel or a thread.", "description: Posts text in a Slack channel or thread.")
		done := submitTree(t, g.l, g.dir, "/acme")
		what := map[string]string{}
		for _, r := range done.Locations {
			what[r.Location] = r.What
		}
		if what["/acme/gromit/slack/gromit"] != "restarted" || what["/acme/ops/slack/ops"] != "restarted" ||
			what["/acme/gromit/slack/chat/post"] != "recorded" || what["/acme/ops/slack/chat/post"] != "recorded" ||
			!slices.Contains(done.Cancelled, gromitRouter) || !slices.Contains(done.Cancelled, opsRouter) || len(done.Cancelled) != 2 {
			t.Errorf("the submit did %v and cancelled %v", what, done.Cancelled)
		}
		for _, at := range []string{"/acme/gromit/slack/chat/post", "/acme/ops/slack/chat/post"} {
			id, err := g.l.Start(ctx, client.Start{At: at, Input: map[string]any{"in": "#x", "text": "hi"}})
			if err != nil {
				t.Fatal(err)
			}
			if r := wait(t, g.l, id); r.Status != "done" || r.Output.(map[string]any)["version"] != 2.0 {
				t.Errorf("%s ended %s with %v", at, r.Status, r.Output)
			}
			def, err := g.l.Definition(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if def.Description != "Posts text in a Slack channel or thread." {
				t.Errorf("%s was started from the description %q", at, def.Description)
			}
		}
	})
}
