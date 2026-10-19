package client_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
	"github.com/zummed/corbie/plugin/mcp/standin"
)

// model is a stand-in for a model service: it gives the replies it was
// scripted with, in order.
type model struct {
	*llmstandin.Server
	URL string
}

func newModel(t *testing.T, replies ...llmstandin.Reply) *model {
	m := &model{Server: &llmstandin.Server{Answer: func(c llmstandin.Call) llmstandin.Reply {
		if c.Turn > len(replies) {
			return llmstandin.Reply{Error: "the script has no more replies"}
		}
		return replies[c.Turn-1]
	}}}
	web := httptest.NewServer(m.Server)
	t.Cleanup(web.Close)
	m.URL = web.URL
	return m
}

func says(text string) llmstandin.Reply {
	return llmstandin.Reply{Text: text}
}

func calls(id, tool, args string) llmstandin.Reply {
	return llmstandin.Reply{Text: "let me see", Calls: []llmstandin.ToolCall{{ID: id, Name: tool, Args: args}}}
}

func agentDef(address, extra string) string {
	return strings.ReplaceAll(strings.ReplaceAll(`
		agent: sums
		input: { question: any }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		system: You add numbers.
		prompt: ${input.question}
		EXTRA
		tools:
		  add:
		    does: Add one to a number
		    args: { n: number }
		    job: { script: ./add.js, input: { n: "${args.n}" } }
	`, "ADDRESS", address), "EXTRA", extra)
}

// turnsOf gives the turns of the conversation an agent's result carries.
func turnsOf(out map[string]any) []any {
	conversation, _ := out["conversation"].(map[string]any)
	turns, _ := conversation["turns"].([]any)
	return turns
}

// carried gives a value of one run's result as a start's input carries it
// to another run: each blob in it says the run it was shown for.
func carried(v any, run string) any {
	switch v := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, item := range v {
			out[k] = carried(item, run)
		}
		if _, isBlob := v["$blob"]; isBlob {
			out["run"] = run
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = carried(item, run)
		}
		return out
	}
	return v
}

func runAgent(t *testing.T, l *client.Local, def string, files map[string]string) *client.Run {
	t.Helper()
	id, err := l.Start(ctx, draft(t, def, files), client.Start{
		Input: map[string]any{"question": "what is 41 plus one?"}, Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	return wait(t, l, id)
}

func TestAgentLoop(t *testing.T) {
	m := newModel(t, calls("call_1", "add", `{"n": 41}`), says("It is 42."))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, ""), scripts)
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	if out["reply"] != "It is 42." || len(turnsOf(out)) != 2 {
		t.Fatalf("the result is %v", jsonOf(out))
	}
	if tokens := out["tokens"].(map[string]any); tokens["input"] != 20.0 || tokens["output"] != 10.0 {
		t.Fatalf("the tokens are %v", tokens)
	}
	if len(m.Calls) != 2 || m.Calls[0].Key != "sk-test-key" {
		t.Fatalf("the model was called %d times: %v", len(m.Calls), m.Calls)
	}
	// The model was shown the tool by its name, what it does and its
	// arguments, and nothing of how it is run.
	first := jsonOf(m.Calls[0])
	if !strings.Contains(first, `"name":"add"`) || !strings.Contains(first, "Add one to a number") || strings.Contains(first, "add.js") ||
		!strings.Contains(first, "what is 41 plus one?") || !strings.Contains(first, "You add numbers.") {
		t.Fatalf("the first call sent %s", first)
	}
	// The second turn showed it the tool's result.
	second := jsonOf(m.Calls[1])
	if !strings.Contains(second, `"tool_use_id":"call_1"`) || !strings.Contains(second, `{\"n\":42}`) {
		t.Fatalf("the second call sent %s", second)
	}
	// The key is nowhere in the result or in what was shown.
	if strings.Contains(jsonOf(r), "sk-test-key") {
		t.Fatal("the result shows the key")
	}
}

func TestAgentToolErrorsGoBackToTheModel(t *testing.T) {
	m := newModel(t,
		calls("c1", "nothing_like_it", `{}`),
		calls("c2", "add", `{"n": "not a number"}`),
		calls("c3", "add", `{"n": 1}`),
		says("done"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, ""), map[string]string{"add.js": `throw new Error("the tool broke")`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	for i, want := range []string{"there is no tool named", "the arguments do not fit the tool", "the tool broke"} {
		if sent := jsonOf(m.Calls[i+1]); !strings.Contains(sent, want) || !strings.Contains(sent, `"is_error":true`) {
			t.Fatalf("turn %d was not told %q: %s", i+2, want, sent)
		}
	}
}

func TestAgentOutOfTurnsFails(t *testing.T) {
	m := newModel(t, calls("c1", "add", `{"n": 1}`), calls("c2", "add", `{"n": 2}`), says("too late"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, "limits: { turns: 2 }"), scripts)
	if r.Status != "failed" || !strings.Contains(r.Error, "used its 2 turns") {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	// A failed agent still has its conversation.
	if out, _ := r.Output.(map[string]any); out == nil || len(turnsOf(out)) != 2 {
		t.Fatalf("a failed agent's output is %v", jsonOf(r.Output))
	}
}

func TestAgentShapedReply(t *testing.T) {
	m := newModel(t, says("The answer is 42."), says("```json\n{\"answer\": 42}\n```"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, "reply: { answer: number }"), scripts)
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	check(t, r.Status, r.Output.(map[string]any)["reply"], "done", `{"answer": 42}`)
	if sent := jsonOf(m.Calls[1]); !strings.Contains(sent, "does not fit what is wanted") {
		t.Fatalf("the model was not told its reply did not fit: %s", sent)
	}
}

func TestAgentWithoutItsKeyIsDenied(t *testing.T) {
	m := newModel(t, says("never reached"))
	// The ceiling allows the connection and not the secret.
	l := local(t, "network")
	r := runAgent(t, l, agentDef(m.URL, ""), scripts)
	if r.Status != "failed" || len(m.Calls) != 0 {
		t.Fatalf("the agent ended %s (%s) having called the model %d times", r.Status, r.Error, len(m.Calls))
	}
}

func agentRefused(t *testing.T, def, want string) string {
	t.Helper()
	_, err := machineParse(def, scripts)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want a refusal holding %q, got: %v", want, err)
	}
	return err.Error()
}

func Test_agent_bad_model_is_reported_once(t *testing.T) {
	got := agentRefused(t, `
		agent: nomodel
		model: { name: mystery-model, secret: KEY }
		prompt: hi
	`, "which service")
	if strings.Count(got, "\n") != 0 {
		t.Fatalf("a model named wrongly was reported more than once:\n%s", got)
	}
	agentRefused(t, strings.Replace(agentDef("https://user:pass@models.example", ""), "ADDRESS", "", 1), "may not hold a user name or a password")
}

// A type in a shape is a word; anything else is refused, saying so.
func TestShapeTypeIsAWord(t *testing.T) {
	agentRefused(t, agentDef("http://127.0.0.1:1", "reply: { answer: 1 }"), `"answer": a type is written as a word`)
	agentRefused(t, agentDef("http://127.0.0.1:1", "reply: { answer: text }"), `"answer": "text" is not a type`)
}

// A shape is JSON Schema: the short form is filled out into the full one,
// the model is shown the full form as written, and what it sends is
// checked against it.
func TestShapeIsJSONSchema(t *testing.T) {
	short, err := machineParse(agentDef("http://127.0.0.1:1", ""), scripts)
	if err != nil {
		t.Fatal(err)
	}
	written, err := machineParse(strings.Replace(agentDef("http://127.0.0.1:1", ""), "args: { n: number }",
		"args: { type: object, properties: { n: { type: number } }, required: [n], additionalProperties: false }", 1), scripts)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := jsonOf(short.Definition), jsonOf(written.Definition); a != b {
		t.Fatalf("the short form and the full form were stored differently:\n%s\n%s", a, b)
	}

	m := newModel(t, calls("c1", "file", `{"labels": ["urgent"], "assign": {"user": "sam"}}`), says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  file:
		    does: File an issue
		    args:
		      type: object
		      properties:
		        labels: { type: array, items: { type: string, enum: [bug, docs] } }
		        assign: { type: object, properties: { user: { type: string } } }
		    job: { script: ./add.js, input: { n: 1 } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": `throw new Error("the tool must not run")`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if offered := jsonOf(m.Calls[0].Tools); !strings.Contains(offered, `"enum":["bug","docs"]`) || !strings.Contains(offered, `"assign":{"properties":{"user":{"type":"string"}},"type":"object"}`) {
		t.Errorf("the tool was offered as %s", offered)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "the arguments do not fit the tool: at /labels/0") || strings.Contains(shown, "must not run") {
		t.Errorf("a label outside the enum was shown as %s", shown)
	}

	// A shape is 2020-12 unless its $schema names another dialect: a list
	// of items is draft-07's way of writing them, and not 2020-12's.
	listed := `args: { type: object, properties: { n: { type: array, items: [ { type: number } ] } } }`
	agentRefused(t, strings.Replace(agentDef("http://127.0.0.1:1", ""), "args: { n: number }", listed, 1), "it is not JSON Schema")
	if _, err := machineParse(strings.Replace(agentDef("http://127.0.0.1:1", ""), "args: { n: number }",
		strings.Replace(listed, "{ type: object", `{ $schema: "http://json-schema.org/draft-07/schema#", type: object`, 1), 1), scripts); err != nil {
		t.Fatalf("a shape that names draft-07 was not read by it: %v", err)
	}
}

// format: blob marks a blob anywhere in a shape: the tool's job is given
// the blob's reference wherever the model gave its name.
func TestShapeMarksABlobAtAnyDepth(t *testing.T) {
	m := newModel(t, calls("c1", "make", `{"text": "page one"}`), calls("c2", "make", `{"text": "page two"}`),
		calls("c3", "bind", `{"pages": [{"scan": "blob1"}, {"scan": "blob2"}]}`), says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  make:
		    does: Make a file
		    args: { text: string }
		    job: { script: ./make.js, input: { text: "${args.text}" } }
		  bind:
		    does: Bind pages into a book
		    args:
		      type: object
		      properties:
		        pages: { type: array, items: { type: object, properties: { scan: { type: string, format: blob } } } }
		    job: { script: ./bind.js, input: { pages: "${args.pages}" } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"],
		"make.js": `return { file: blobs.make(input.text) }`,
		"bind.js": `return { text: input.pages.map(p => blobs.read(p.scan)).join(", ") }`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := lastShown(m.Calls[3]); !strings.Contains(shown, "page one, page two") {
		t.Errorf("the tool was not given the blobs: %s", shown)
	}
}

func Test_agent_shape_ref_outside_the_shape_is_refused(t *testing.T) {
	for _, ref := range []string{"other.json", "file:///etc/hostname", "https://schemas.example/user.json"} {
		agentRefused(t, strings.Replace(agentDef("http://127.0.0.1:1", ""), "args: { n: number }",
			`args: { type: object, properties: { n: { $ref: "`+ref+`" } } }`, 1),
			"a $ref resolves only within its own shape")
	}
	if _, err := machineParse(strings.Replace(agentDef("http://127.0.0.1:1", ""), "args: { n: number }",
		`args: { type: object, $defs: { count: { type: number } }, properties: { n: { $ref: "#/$defs/count" } } }`, 1), scripts); err != nil {
		t.Fatalf("a $ref within the shape was refused: %v", err)
	}
}

func Test_agent_tool_reading_unnamed_argument_is_refused(t *testing.T) {
	agentRefused(t, strings.Replace(agentDef("http://127.0.0.1:1", ""), `input: { n: "${args.n}" }`, `input: { n: "${args.other}" }`, 1),
		`reads "args.other", which the tool's args do not name`)
}

func Test_agent_tool_cannot_take_an_agents_own_job_name(t *testing.T) {
	for name, want := range map[string]string{
		"turn":      `the tool "turn" has the name of a job the agent itself makes`,
		"blob_read": `the tool "blob_read" has the name of one of the tools every agent has for blobs`,
		"ask":       `the name "ask" is taken by the definition's ask`,
	} {
		agentRefused(t, strings.Replace(agentDef("http://127.0.0.1:1", ""), "  add:\n", "  "+name+":\n", 1), want)
	}
}

func Test_agent_cut_short_call_runs_nothing(t *testing.T) {
	cut := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "c1", Name: "add", Args: `{"n":1}`}}, Cut: true}
	m := newModel(t, cut, says("done after all"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, ""), map[string]string{"add.js": `throw new Error("the tool must not run")`})
	if r.Status != "done" || !strings.Contains(jsonOf(m.Calls[1]), "cut short") || strings.Contains(jsonOf(m.Calls[1]), "must not run") {
		t.Fatalf("after a reply cut short in a call: %s %s\n%s", r.Status, r.Error, jsonOf(m.Calls[1]))
	}
}

func Test_agent_failure_always_has_a_conversation(t *testing.T) {
	m := newModel(t) // no replies: the first turn fails
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, ""), scripts)
	if r.Status != "failed" || !strings.Contains(r.Error, "turn:") {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	out, _ := r.Output.(map[string]any)
	if conversation, ok := out["conversation"].(map[string]any); !ok || len(turnsOf(out)) != 0 || conversation["blobs"] == nil || out["tokens"] == nil {
		t.Fatalf("a failed agent's output is %s", jsonOf(r.Output))
	}
}

func Test_agent_limit_reached_says_what_was_wrong_with_last_reply(t *testing.T) {
	m := newModel(t, says("not json"), says("still not json"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, "limits: { turns: 2 }\n\t\treply: { answer: number }"), scripts)
	if r.Status != "failed" || !strings.Contains(r.Error, "used its 2 turns") || !strings.Contains(r.Error, "its last reply did not fit") {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
}

func Test_agent_shaped_reply_accepts_one_fence_and_refuses_trailing_text(t *testing.T) {
	m := newModel(t, says("```json\n{\"answer\": 1}\n```\nI hope that helps!"), says("```\n{\"answer\": 2}\n```"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, "reply: { answer: number }"), scripts)
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	check(t, r.Status, r.Output.(map[string]any)["reply"], "done", `{"answer": 2}`)
}

func Test_agent_results_shown_in_call_order(t *testing.T) {
	two := llmstandin.Reply{Calls: []llmstandin.ToolCall{{ID: "slow", Name: "add", Args: `{"n":100}`}, {ID: "fast", Name: "add", Args: `{"n":1}`}}}
	m := newModel(t, two, says("done"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, ""), map[string]string{"add.js": `
		const until = Date.now() + (input.n > 50 ? 400 : 0); while (Date.now() < until) {}
		return { n: input.n + 1 }
	`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	sent := jsonOf(m.Calls[1])
	if slow, fast := strings.Index(sent, `"tool_use_id":"slow"`), strings.Index(sent, `"tool_use_id":"fast"`); slow < 0 || fast < slow {
		t.Fatalf("the results were not shown in the order the calls were made: %s", sent)
	}
}

// A blob is shown to the model as a handle; the model names it where a
// tool's shape asks for a blob; and no key reaches the model.
func Test_agent_name_becomes_reference_only_where_shape_asks(t *testing.T) {
	m := newModel(t,
		calls("c1", "make", `{"text": "line one\nline two\nblob1 is only a word here"}`),
		calls("c2", "blob_read", `{"blob": "blob1"}`),
		calls("c3", "echo", `{"text": "blob1"}`),
		says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  make:
		    does: Make a file
		    args: { text: string }
		    job: { script: ./make.js, input: { text: "${args.text}" } }
		  echo:
		    does: Say it back
		    args: { text: string }
		    job: { script: ./echo.js, input: { text: "${args.text}" } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"], "echo.js": scripts["echo.js"],
		"make.js": `return { file: blobs.make(input.text) }`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	all := jsonOf(m.Calls)
	if strings.Contains(all, `\"key\"`) || strings.Contains(all, `$blob`) {
		t.Fatalf("a blob's reference reached the model: %s", all)
	}
	if shown := jsonOf(m.Calls[1]); !strings.Contains(shown, `\"blob\":\"blob1\"`) {
		t.Fatalf("the model was not shown a handle: %s", shown)
	}
	// Where the shape asks for a blob, the name became the blob.
	if read := jsonOf(m.Calls[2]); !strings.Contains(read, `{\"from\":0,\"items\":[\"line one\",\"line two\",\"blob1 is only a word here\"]}`) {
		t.Fatalf("blob_read by a handle's name gave: %s", read)
	}
	// Where it asks for text, the name stayed text.
	if echoed := jsonOf(m.Calls[3]); !strings.Contains(echoed, `{\"text\":\"blob1\"}`) {
		t.Fatalf("a handle's name in ordinary text was turned into something else: %s", echoed)
	}
}

// Whoever follows an agent's run sees the model's reply while it is being
// written: the first of its text is in the record before the model has
// finished, and the pieces together are the reply.
func TestModelTextIsSeenWhileItIsWritten(t *testing.T) {
	const reply = "It is forty-two, as it happens, which is one more than forty-one."
	// The model holds the second half of its reply until the follower,
	// having seen the first, sends to it.
	hold := make(chan struct{})
	m := newModel(t, llmstandin.Reply{Text: reply, Hold: hold})
	waited := false
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, agentDef(m.URL, ""), scripts), client.Start{
		Input: map[string]any{"question": "what is 41 plus one?"}, Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	following, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	r, err := l.Follow(following, id, 20*time.Millisecond, func(e client.Entry) {
		if e.Name == "text" {
			if len(seen) == 0 {
				select {
				case hold <- struct{}{}:
					waited = true
				case <-time.After(10 * time.Second):
				}
			}
			seen = append(seen, e.Data.(string))
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %v, %v", r, err)
	}
	if !waited || strings.Join(seen, "") != reply || len(seen) < 2 {
		t.Fatalf("the model was still writing when the first text was seen: %v; the follower saw %q", waited, seen)
	}
}

// lastShown is what the model was told last in a call: the results of its
// tools, as the text it was sent.
func lastShown(c llmstandin.Call) string {
	return jsonOf(c.Messages[len(c.Messages)-1])
}

// A blob is shown to the model as a handle that says how many items it
// has and roughly what reading it would cost, and, the first time it is
// shown, the first few of its items. A job makes the preview, between the
// tool's result and the next turn.
func TestHandleSaysItemsCostAndThePreviewOnce(t *testing.T) {
	m := newModel(t,
		calls("c1", "table", `{"rows": 1000}`),
		calls("c2", "again", `{"it": "blob1"}`),
		calls("c3", "blob_search", `{"blob": "blob1", "find": "city-7"}`),
		calls("c4", "blob_search", `{"blob": "blob1", "find": "city-7", "from": 500}`),
		calls("c5", "blob_code", `{"blob": "blob1", "code": "let n = 0; for (const r of items) n += Number(r.amount); return n"}`),
		says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  table:
		    does: Make a table
		    args: { rows: number }
		    job: { script: ./table.js, input: { rows: "${args.rows}" } }
		  again:
		    does: Give a blob back
		    args: { it: blob }
		    job: { script: ./again.js, input: { it: "${args.it}" } }
`, 1)
	id, err := l.Start(ctx, draft(t, def, map[string]string{"add.js": scripts["add.js"],
		"again.js": `return { same: input.it }`,
		"table.js": `
			let text = "city,amount\n"
			for (let i = 0; i < input.rows; i++) text += "city-" + (i % 10) + "," + i + "\n"
			return { table: blobs.make(text, { type: "text/csv" }) }
		`}), client.Start{Input: map[string]any{"question": "q"}, Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
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
	// The record shows the job that made the preview, between the tool and
	// the next turn, and no second one when the blob is shown again.
	if got := strings.Join(jobs, " "); got != "turn table preview turn#2 again turn#3 blob_search turn#4 blob_search#2 turn#5 blob_code turn#6" {
		t.Fatalf("the jobs were: %s", got)
	}
	first := lastShown(m.Calls[1])
	for _, want := range []string{`\"blob\":\"blob1\"`, `\"type\":\"text/csv\"`, `\"items\":1000`, `tokens to read whole`,
		`\"preview\":[{\"amount\":\"0\",\"city\":\"city-0\"}`, `{\"amount\":\"4\",\"city\":\"city-4\"}]`} {
		if !strings.Contains(first, want) {
			t.Fatalf("the handle first shown lacks %s: %s", want, first)
		}
	}
	if again := lastShown(m.Calls[2]); !strings.Contains(again, `\"items\":1000`) || strings.Contains(again, "preview") {
		t.Fatalf("the handle shown again: %s", again)
	}
	// A search gives fifty matches and where to go on from, and goes on
	// from there.
	found := lastShown(m.Calls[3])
	if !strings.Contains(found, `{\"at\":7,\"item\":{\"amount\":\"7\",\"city\":\"city-7\"}}`) || !strings.Contains(found, `\"at\":497,`) ||
		strings.Contains(found, `\"at\":507,`) || !strings.Contains(found, `\"next\":507`) {
		t.Fatalf("the first search gave: %s", found)
	}
	if more := lastShown(m.Calls[4]); !strings.Contains(more, `\"at\":507,`) || !strings.Contains(more, `\"at\":997,`) || strings.Contains(more, `\"at\":497,`) || strings.Contains(more, "next") {
		t.Fatalf("the search from 500 gave: %s", more)
	}
	// The model's code walks the items and gives back only its answer.
	if answer := lastShown(m.Calls[5]); !strings.Contains(answer, `"content":"499500"`) {
		t.Fatalf("the code's answer: %s", answer)
	}
}

// A tool result too large to show is stored as a blob, and the model is
// given its handle, with its preview.
func TestLargeResultIsStoredAsABlob(t *testing.T) {
	m := newModel(t, calls("c1", "lots", `{"n": 3000}`), calls("c2", "blob_read", `{"blob": "blob1", "from": 2998}`), says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  lots:
		    does: Give many things
		    args: { n: number }
		    job: { script: ./lots.js, input: { n: "${args.n}" } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"],
		"lots.js": `return Array.from({ length: input.n }, (_, i) => ({ at: i, word: "something-" + i }))`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := lastShown(m.Calls[1])
	if len(shown) > 4000 || !strings.Contains(shown, `\"blob\":\"blob1\"`) || !strings.Contains(shown, `\"items\":3000`) ||
		!strings.Contains(shown, `\"preview\":[{\"at\":0,\"word\":\"something-0\"}`) {
		t.Fatalf("a result of three thousand things was shown as: %s", shown)
	}
	// What was stored is the whole result, which the blob tools read.
	if read := lastShown(m.Calls[2]); !strings.Contains(read, `{\"at\":2999,\"word\":\"something-2999\"}`) {
		t.Fatalf("reading the end of the stored result gave: %s", read)
	}
}

// inside is an agent with a tool that gives a blob and one that gives a
// list, as a blob, whose first entry holds the first blob's reference.
func inside(m *model) string {
	return strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  make:
		    does: Make a file
		    args: { text: string }
		    job: { script: ./make.js, input: { text: "${args.text}" } }
		  wrap:
		    does: Put a blob inside a list
		    args: { it: blob }
		    job: { script: ./wrap.js, input: { it: "${args.it}" } }
`, 1)
}

var insideFiles = map[string]string{"add.js": scripts["add.js"],
	"make.js": `return { file: blobs.make(input.text) }`,
	"wrap.js": `return { list: blobs.make([{ inner: input.it, note: "first" }, { note: "second" }], { type: "application/json" }) }`}

func Test_agent_preview_shows_no_key(t *testing.T) {
	m := newModel(t, calls("c1", "make", `{"text": "inner text"}`), calls("c2", "wrap", `{"it": "blob1"}`), says("done"))
	l := local(t, "all")
	if r := runAgent(t, l, inside(m), insideFiles); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	// The list's preview holds its first entry, in which the blob inside
	// is the handle the model already knows, and no key.
	shown := lastShown(m.Calls[2])
	if !strings.Contains(shown, `\"preview\":[{\"inner\":{\"blob\":\"blob1\"`) || !strings.Contains(shown, `\"note\":\"second\"`) {
		t.Fatalf("the preview was shown as: %s", shown)
	}
	if all := jsonOf(m.Calls); strings.Contains(all, `key\"`) || strings.Contains(all, `$blob`) {
		t.Fatalf("a blob's reference reached the model: %s", all)
	}
}

func Test_agent_code_tool_hides_reference_from_model_code(t *testing.T) {
	// The model's code looks for anything but the items: the tool's input,
	// what the thing it was handed is made of, and what it is as text.
	const code = `const seen = []; for (const item of items) seen.push(item); ` +
		`return { input: typeof input, own: Object.getOwnPropertyNames(items).join(), text: String(JSON.stringify(items)), seen }`
	args, _ := json.Marshal(map[string]any{"blob": "blob2", "code": code})
	m := newModel(t, calls("c1", "make", `{"text": "inner text"}`), calls("c2", "wrap", `{"it": "blob1"}`),
		calls("c3", "blob_code", string(args)), says("done"))
	l := local(t, "all")
	if r := runAgent(t, l, inside(m), insideFiles); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	answer := lastShown(m.Calls[3])
	if !strings.Contains(answer, `\"input\":\"undefined\"`) || !strings.Contains(answer, `\"note\":\"second\"`) {
		t.Fatalf("the code's answer was: %s", answer)
	}
	// Not the reference of the blob it walks, and not that of the blob
	// inside an item, which comes back to the model as a handle.
	if strings.Contains(answer, `key\"`) || strings.Contains(answer, `$blob`) || !strings.Contains(answer, `\"inner\":{\"blob\":\"blob1\"`) {
		t.Fatalf("the code reached a reference: %s", answer)
	}
}

func Test_agent_blob_tools_get_none_of_the_definitions_permissions(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	// The definition allows every job it runs the variable, and the model
	// writes code for the code tool that reads it.
	args, _ := json.Marshal(map[string]any{"blob": "blob1", "code": `return env.get("HOME")`})
	m := newModel(t, calls("c1", "make", `{"text": "inner text"}`), calls("c2", "blob_code", string(args)), says("done"))
	l := local(t, "all")
	def := strings.Replace(inside(m), "tools:\n", "allow: [env:HOME]\n\t\ttools:\n", 1)
	if r := runAgent(t, l, def, insideFiles); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := jsonOf(m.Calls[2])
	if !strings.Contains(shown, "denied") || !strings.Contains(shown, "env:HOME") || strings.Contains(shown, "/home/someone") {
		t.Fatalf("the model's code ran with the definition's permissions: %s", shown)
	}
}

// A blob of a type the model's service takes as content, an image or a
// document, is sent to the model as it is, beside its handle and under
// its name, in every call from the turn it is first shown. A blob of any
// other type stays a handle, which the model works on with its tools.
func TestModelIsSentTheFilesItsServiceTakes(t *testing.T) {
	m := newModel(t, calls("c1", "fetch", `{}`), calls("c2", "add", `{"n": 1}`), says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  fetch:
		    does: Fetch a picture and a table
		    args: {}
		    job: { script: ./fetch.js }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"],
		"fetch.js": `return { picture: blobs.make("not really a picture", { type: "image/png" }), table: blobs.make("a,b\n1,2\n", { type: "text/csv" }) }`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	const picture = `{"text":"blob1:","type":"text"},{"source":{"data":"bm90IHJlYWxseSBhIHBpY3R1cmU=","media_type":"image/png","type":"base64"},"type":"image"}`
	for i, sent := range m.Calls[1:] {
		all := jsonOf(sent)
		if !strings.Contains(all, picture) || strings.Contains(all, `blob2:`) || !strings.Contains(all, `\"blob\":\"blob2\"`) {
			t.Fatalf("call %d sent: %s", i+2, all)
		}
	}
	// The picture is a file beside its handle, and the handle still says
	// what it is; nothing of it is in the conversation that is kept.
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, `\"blob\":\"blob1\"`) || !strings.Contains(shown, `\"type\":\"image/png\"`) {
		t.Fatalf("the handle was shown as: %s", shown)
	}
	if kept := jsonOf(r.Output); strings.Contains(kept, "bm90IHJlYWxseSBhIHBpY3R1cmU") {
		t.Fatalf("the picture's bytes are in the agent's result: %s", kept)
	}
}

func TestAgentSharedToolFiles(t *testing.T) {
	// An agent names two files of tools; the model is offered the tools of
	// both beside the agent's own, and one of the agent's own stands before
	// a file's of the same name.
	m := newModel(t, calls("c1", "twice", `{"n": 4}`), calls("c2", "add", `{"n": 8}`), says("done"))
	l := local(t, "all")
	r := runAgent(t, l, agentDef(m.URL, "tools_file: [./first.yaml, ./second.yaml]"), map[string]string{
		"add.js":      scripts["add.js"],
		"twice.js":    `return { n: input.n * 2 }`,
		"first.yaml":  "half:\n  does: Halve a number\n  args: { n: number }\n  job: { script: ./twice.js, input: { n: \"${args.n}\" } }\n",
		"second.yaml": "twice:\n  does: Double a number\n  args: { n: number }\n  job: { script: ./twice.js, input: { n: \"${args.n}\" } }\nadd:\n  does: Not the agent's own\n  args: { n: number }\n  job: { script: ./twice.js, input: { n: \"${args.n}\" } }\n",
	})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	offered := jsonOf(m.Calls[0].Tools)
	for _, name := range []string{`"name":"half"`, `"name":"twice"`, `"name":"add"`} {
		if !strings.Contains(offered, name) {
			t.Errorf("the model was not offered %s: %s", name, offered)
		}
	}
	if strings.Contains(offered, "Not the agent's own") {
		t.Errorf("a file's tool took the place of the agent's own: %s", offered)
	}
	if shown := strings.ReplaceAll(jsonOf(m.Calls[2]), `\`, ""); !strings.Contains(shown, `{"n":8}`) || !strings.Contains(shown, `{"n":9}`) {
		t.Errorf("the tools did not run as written: %s", shown)
	}
}

func TestAgentToolsFileReadFromItsDirectory(t *testing.T) {
	// An agent names ../shared/tools.yaml and ./own/tools.yaml. A path in
	// each is read from that file's directory, so both can name ./run.js.
	m := newModel(t, calls("c1", "run", `{}`), calls("c2", "wait", `{}`), calls("c3", "mine", `{}`), says("done"))
	l := local(t, "all")
	def := agentDef(m.URL, "tools_file: [../shared/tools.yaml, ./own/tools.yaml]")
	files := map[string]string{
		"add.js": scripts["add.js"],
		"../shared/tools.yaml": "run:\n  does: Run\n  job: ./run.js\n" +
			"wait:\n  does: Wait\n  job: { machine: ./wait.yaml }\n",
		"../shared/run.js":    `return "shared run"`,
		"../shared/wait.yaml": "flow: wait\nstages:\n  w: { job: ./w.js }\noutput: ${stages.w.output}\n",
		"../shared/w.js":      `return "shared wait"`,
		"own/tools.yaml":      "mine:\n  does: Mine\n  job: ./run.js\n",
		"own/run.js":          `return "own run"`,
	}
	r := runAgent(t, l, def, files)
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if shown := jsonOf(m.Calls[3]); !strings.Contains(shown, "shared run") || !strings.Contains(shown, "shared wait") || !strings.Contains(shown, "own run") {
		t.Errorf("the tools did not run their own scripts: %s", shown)
	}
	// The stored definition names each path from the agent's directory.
	d := draft(t, def, files)
	for _, name := range []string{"../shared/run.js", "./own/run.js"} {
		if _, ok := d.Definition.Scripts[name]; !ok || !strings.Contains(string(d.Definition.Body), strconv.Quote(name)) {
			t.Errorf("the definition does not name %s: %s", name, d.Definition.Body)
		}
	}
	if _, ok := d.Definition.Machines["../shared/wait.yaml"]; !ok {
		t.Errorf("the definition's machines are %v", d.Definition.Machines)
	}
}

func TestAgentAsATool(t *testing.T) {
	// An agent whose tool is another agent is shown what that agent
	// replied, and none of the conversation it had.
	helper := newModel(t, says("Breaker four."))
	m := newModel(t, calls("c1", "sparks", `{"question": "Which breaker?"}`), says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  sparks:
		    does: Ask the electrician
		    args: { question: string }
		    job: { machine: ./sparks.yaml, input: { question: "${args.question}" } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"], "sparks.yaml": strings.ReplaceAll(`
		agent: sparks
		input: { question: any }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		prompt: ${input.question}
	`, "ADDRESS", helper.URL)})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := strings.ReplaceAll(lastShown(m.Calls[1]), `\`, "")
	if !strings.Contains(shown, "Breaker four.") {
		t.Errorf("the model was not shown what the other agent replied: %s", shown)
	}
	if strings.Contains(shown, "conversation") || strings.Contains(shown, "tokens") {
		t.Errorf("the model was shown more of the other agent than its reply: %s", shown)
	}
}

// listed is an agent with an entry whose tools come from a list.
func listed(address, list string) string {
	return strings.Replace(agentDef(address, ""), "tools:\n", `tools:
		  files:
		    list: { script: ./list.js, input: { tools: `+list+` } }
		    job: { script: ./files.js, input: { tool: "${tool}", args: "${args}" } }
`, 1)
}

var listedScripts = map[string]string{
	"add.js":   scripts["add.js"],
	"list.js":  `if (input.tools === "none") throw new Error("the server is away"); return input.tools`,
	"files.js": `return { tool: input.tool, args: input.args }`,
}

const fileTools = `[
		      { name: read, does: Read a file, args: { type: object, properties: { path: { type: string } }, required: [path] } },
		      { name: open.recent, does: Open the last file, args: { type: object } } ]`

func TestAgentListedTools(t *testing.T) {
	m := newModel(t, calls("c1", "files_read", `{"path": "a.txt"}`), calls("c2", "files_open_recent", `{}`), says("done"))
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, listed(m.URL, fileTools), listedScripts),
		client.Start{Input: map[string]any{"question": "q"}, Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
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
	// The list runs once, before the first turn.
	if got := strings.Join(jobs, " "); got != "files turn files_read turn#2 files_open_recent turn#3" {
		t.Fatalf("the jobs were: %s", got)
	}
	// The model is offered each listed tool under the entry's name and its
	// own, as the list gave it, and not the entry itself.
	offered := jsonOf(m.Calls[0].Tools)
	for _, want := range []string{`"name":"files_read"`, `"description":"Read a file"`, `"required":["path"]`, `"name":"files_open_recent"`} {
		if !strings.Contains(offered, want) {
			t.Errorf("the model was not offered %s: %s", want, offered)
		}
	}
	if strings.Contains(offered, `"name":"files"`) {
		t.Errorf("the model was offered the entry: %s", offered)
	}
	if offered != jsonOf(m.Calls[2].Tools) {
		t.Errorf("the third turn was offered other tools: %s", jsonOf(m.Calls[2].Tools))
	}
	// The entry's job runs each, given the name the list gave.
	if shown := strings.ReplaceAll(lastShown(m.Calls[1]), `\`, ""); !strings.Contains(shown, `"args":{"path":"a.txt"}`) || !strings.Contains(shown, `"tool":"read"`) {
		t.Errorf("the first call gave: %s", shown)
	}
	if shown := strings.ReplaceAll(lastShown(m.Calls[2]), `\`, ""); !strings.Contains(shown, `"tool":"open.recent"`) {
		t.Errorf("the second call gave: %s", shown)
	}
	// A run that carries the conversation on lists the tools again.
	m2 := newModel(t, says("done again"))
	id, err = l.Start(ctx, draft(t, listed(m2.URL, fileTools)+"\n\t\tconversation: ${input.so}\n", listedScripts), client.Start{
		Input:   map[string]any{"question": "and again?", "so": carried(r.Output.(map[string]any)["conversation"], r.ID)},
		Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	if second := wait(t, l, id); second.Status != "done" || jsonOf(m2.Calls[0].Tools) != offered {
		t.Fatalf("the run that carried on ended %s (%s), offered %s", second.Status, second.Error, jsonOf(m2.Calls[0].Tools))
	}
}

// The ask is shown a listed call's entry and the tool's own name, and
// nothing more for any other tool.
func TestAgentAskIsShownAListedCallsEntryAndTool(t *testing.T) {
	both := llmstandin.Reply{Text: "let me see", Calls: []llmstandin.ToolCall{{ID: "issue", Name: "github_create_issue", Args: `{"title":"Broken"}`}, {ID: "sum", Name: "add", Args: `{"n":1}`}}}
	m := newModel(t, both, says("done"))
	def := listed(m.URL, `[ { name: create_issue, does: Open an issue, args: { type: object } } ]`)
	def = strings.Replace(def, "  files:\n", "  github:\n", 1)
	def = strings.Replace(def, "\t\ttools:\n", "\t\task: ./ask.js\n\t\ttools:\n", 1)
	def = strings.Replace(def, `args: "${args}" } }`, `args: "${args}" }, approve: true }`, 1)
	def = strings.Replace(def, `input: { n: "${args.n}" } }`, `input: { n: "${args.n}" }, approve: true }`, 1)
	files := maps.Clone(listedScripts)
	files["ask.js"] = `
		const listed = (a) => a.entry === "github" && a.tool === "create_issue" && a.args.title === "Broken" && a.does === "Open an issue" && a.said === "let me see"
		const own = (a) => !("entry" in a) && !("tool" in a) && a.args.n === 1 && a.does === "Add one to a number"
		const fits = input.jobs.filter((j) => j.name.startsWith("github_") ? listed(j.asked) : own(j.asked))
		return { approve: fits.map((j) => j.name), note: "the ask was shown " + JSON.stringify(input.jobs.map((j) => j.asked)) }`
	if r := runAgent(t, local(t, "all"), def, files); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := jsonOf(m.Calls[1])
	if !strings.Contains(shown, `"is_error":false,"tool_use_id":"issue"`) || !strings.Contains(shown, `"is_error":false,"tool_use_id":"sum"`) {
		t.Fatalf("the second turn was shown: %s", shown)
	}
}

// A list that fails, or that gives a tool the model cannot be offered,
// fails the agent before its first turn.
func TestAgentListThatCannotBeUsedFails(t *testing.T) {
	long := strings.Repeat("x", 60)
	for list, want := range map[string]string{
		`none`: "tools.files.list: Error: the server is away",
		`[ { does: Nameless, args: { type: object } } ]`:                                     "a tool in the list has no name",
		`[ { name: count, args: { type: string } } ]`:                                        `the tool "count"'s args`,
		`[ { name: count } ]`:                                                                `the tool "count"'s args`,
		`[ { name: ` + long + `, args: { type: object } } ]`:                                 "longer than 64 characters",
		`[ { name: read, args: { type: object } }, { name: read, args: { type: object } } ]`: "which another tool has",
	} {
		m := newModel(t)
		r := runAgent(t, local(t, "all"), listed(m.URL, list), listedScripts)
		if r.Status != "failed" || !strings.Contains(r.Error, want) || len(m.Calls) != 0 {
			t.Errorf("with the list %s the agent ended %s after %d turns: %s", list, r.Status, len(m.Calls), r.Error)
		}
	}
	// An entry named blob that lists read would be shown as blob_read, the
	// name of one of the blob tools.
	m := newModel(t)
	r := runAgent(t, local(t, "all"), strings.Replace(listed(m.URL, `[ { name: read, args: { type: object } } ]`), "  files:\n", "  blob:\n", 1), listedScripts)
	if r.Status != "failed" || !strings.Contains(r.Error, `"blob_read", which another tool has`) {
		t.Errorf("the agent ended %s: %s", r.Status, r.Error)
	}
}

func Test_agent_listed_shape_marking_a_blob_fails(t *testing.T) {
	m := newModel(t)
	r := runAgent(t, local(t, "all"), listed(m.URL,
		`[ { name: read, args: { type: object, properties: { pages: { type: array, items: { type: string, format: blob } } } } } ]`), listedScripts)
	if r.Status != "failed" || !strings.Contains(r.Error, `the tool "read"'s args mark a blob`) || len(m.Calls) != 0 {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
}

func Test_agent_listed_job_reads_args_whole(t *testing.T) {
	agentRefused(t, strings.Replace(listed("http://127.0.0.1:1", fileTools), `args: "${args}"`, `args: "${args.path}"`, 1),
		`it reads "args.path"; with list:, the job reads ${args} only whole`)
	agentRefused(t, strings.Replace(listed("http://127.0.0.1:1", fileTools), "    job: { script: ./files.js", "    does: Files\n		    job: { script: ./files.js", 1),
		"a tool with list: is given what each tool does and its args by the list")
	// Only a job of an entry with a list reads tool.
	agentRefused(t, strings.Replace(agentDef("http://127.0.0.1:1", ""), `input: { n: "${args.n}" }`, `input: { n: "${tool}" }`, 1), "tool")
	if _, err := machineParse(listed("http://127.0.0.1:1", fileTools), listedScripts); err != nil {
		t.Fatalf("an entry whose job reads ${args} whole was refused: %v", err)
	}
}

// An agent's entry lists the tools of an MCP server, and one script both
// lists them and calls the one the model picks.
func TestAgentWithTheToolsOfAnMCPServer(t *testing.T) {
	notes := httptest.NewServer(&standin.Server{Only: []string{"add_note", "find_notes"}})
	t.Cleanup(notes.Close)
	allow := `allow: ["net:` + strings.TrimPrefix(notes.URL, "http://") + `"]`
	m := newModel(t, calls("c1", "notes_find_notes", `{"text": "tea"}`), says("Two notes."))
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  notes:
		    list: { script: ./notes.js, input: { address: "`+notes.URL+`/mcp" }, `+allow+` }
		    job: { script: ./notes.js, input: { address: "`+notes.URL+`/mcp", tool: "${tool}", args: "${args}" }, `+allow+` }
`, 1)
	r := runAgent(t, local(t, "all"), def, map[string]string{"add.js": scripts["add.js"], "notes.js": `
		const server = mcp.connect(input.address)
		return input.tool ? server.call(input.tool, { args: input.args }) : server.tools()
	`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if offered := jsonOf(m.Calls[0].Tools); !strings.Contains(offered, `"name":"notes_add_note"`) || !strings.Contains(offered, `"description":"Find the notes with a text"`) {
		t.Errorf("the model was offered %s", offered)
	}
	if shown := lastShown(m.Calls[1]); !strings.Contains(shown, "2 notes: tea at 4, tea at 5") {
		t.Errorf("the call gave %s", shown)
	}
}
