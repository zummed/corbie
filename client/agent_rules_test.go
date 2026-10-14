package client_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
)

func Test_agent_next_turn_waits_for_open_approval(t *testing.T) {
	// One turn asks for a tool that is held for a yes, and for one that is
	// not. The ask is a person, who takes their time.
	both := `{"content":[{"type":"tool_use","id":"held","name":"deploy","input":{"n":10}},{"type":"tool_use","id":"free","name":"add","input":{"n":1}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	m := newModel(t, both, says("done"))
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, strings.ReplaceAll(`
		agent: careful
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		prompt: Add, and put it out.
		ask:
		  machine:
		    statechart: person
		    initial: asking
		    states:
		      asking:
		        ticket: verdict
		        on: { verdict: decided }
		      decided: { end: done, output: { approve: "${input.jobs.map(r => r.name)}" } }
		tools:
		  add:
		    does: Add one to a number
		    args: { n: number }
		    job: { script: ./add.js, input: { n: "${args.n}" } }
		  deploy:
		    does: Put a number out
		    args: { n: number }
		    job: { script: ./add.js, input: { n: "${args.n}" }, approve: true }
	`, "ADDRESS", m.URL), scripts), client.Start{Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var steps []client.Entry
	ended := make(chan *client.Run, 1)
	go func() {
		r, _ := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
			if e.ID == id && e.Step > 0 {
				mu.Lock()
				steps = append(steps, e)
				mu.Unlock()
			}
		})
		ended <- r
	}()
	// The ask is open, and the tool that needed no yes has settled: the
	// step that took its result asks for no turn.
	child, ticket := waitForTicket(t, l, id)
	var settled *client.Entry
	for deadline := time.Now().Add(20 * time.Second); settled == nil; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the tool that was not held never settled")
		}
		mu.Lock()
		for _, step := range steps {
			for _, event := range step.Events {
				if event.From == "add" && event.Name == machine.Done {
					settled = &step
				}
			}
		}
		mu.Unlock()
	}
	for _, ask := range settled.Asks {
		if ask.Kind == "script" {
			t.Fatalf("with the approval still open, the agent asked for %s", ask.Name)
		}
	}
	// Once the ask has said yes and the held tool has run, the next turn
	// shows the model both results.
	if _, err := l.Reply(ctx, child, ticket, nil, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-ended:
		if r == nil || r.Status != "done" {
			t.Fatalf("the agent ended %+v", r)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the agent did not end")
	}
	if len(m.sent) != 2 {
		t.Fatalf("the model was called %d times", len(m.sent))
	}
	if second := jsonOf(m.sent[1]); !strings.Contains(second, `"content":"{\"n\":11}","is_error":false,"tool_use_id":"held"`) ||
		!strings.Contains(second, `"content":"{\"n\":2}","is_error":false,"tool_use_id":"free"`) {
		t.Fatalf("the second turn was shown: %s", second)
	}
}

func Test_agent_handle_name_is_stable_and_never_reused(t *testing.T) {
	// The model is shown one blob, then another; the first again; a third
	// in the same turn as the second again; and at the end reads the first
	// and the third by the names it was given.
	two := `{"content":[{"type":"tool_use","id":"c4","name":"make","input":{"text":"the third"}},{"type":"tool_use","id":"c5","name":"again","input":{"it":"blob2"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	reads := `{"content":[{"type":"tool_use","id":"c6","name":"blob_read","input":{"blob":"blob1"}},{"type":"tool_use","id":"c7","name":"blob_read","input":{"blob":"blob3"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	m := newModel(t, calls("c1", "make", `{"text": "the first"}`), calls("c2", "make", `{"text": "the second"}`),
		calls("c3", "again", `{"it": "blob1"}`), two, reads, says("done"))
	l := local(t, "all")
	def := strings.Replace(agentDef(m.URL, ""), "tools:\n", `tools:
		  make:
		    does: Make a file
		    args: { text: string }
		    job: { script: ./make.js, input: { text: "${args.text}" } }
		  again:
		    does: Give a blob back, and beside it one never seen
		    args: { it: blob }
		    job: { script: ./again.js, input: { it: "${args.it}" } }
`, 1)
	r := runAgent(t, l, def, map[string]string{"add.js": scripts["add.js"],
		"make.js":  `return { file: blobs.make(input.text) }`,
		"again.js": `return { same: input.it }`})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	shown := func(turn int) string { return strings.ReplaceAll(lastShown(m.sent[turn]), `\`, "") }
	for turn, want := range map[int][]string{
		1: {`"file":{"blob":"blob1"`},
		2: {`"file":{"blob":"blob2"`},
		// Shown again, a blob has the name it had.
		3: {`"same":{"blob":"blob1"`},
		// A blob not seen before never takes a name that was given.
		4: {`"file":{"blob":"blob3"`, `"same":{"blob":"blob2"`},
		// Turns later, each name still means the blob it was given to.
		5: {`"items":["the first"]`, `"items":["the third"]`},
	} {
		for _, part := range want {
			if !strings.Contains(shown(turn), part) {
				t.Errorf("turn %d was not shown %s: %s", turn+1, part, shown(turn))
			}
		}
	}
	if all := strings.ReplaceAll(jsonOf(m.sent), `\`, ""); strings.Contains(all, `"blob":"blob4"`) {
		t.Errorf("three blobs were given more than three names: %s", all)
	}

	// An agent that carries the conversation on keeps the names: a blob it
	// has not seen takes one the conversation never used, and a name from
	// the earlier turns still means the blob it was given to.
	both := `{"content":[{"type":"tool_use","id":"d1","name":"make","input":{"text":"the fourth"}},{"type":"tool_use","id":"d2","name":"blob_read","input":{"blob":"blob1"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	m2 := newModel(t, both, says("done"))
	id, err := l.Start(ctx, draft(t, strings.Replace(def, m.URL, m2.URL, 1)+"\n\t\tconversation: ${input.so}\n", map[string]string{"add.js": scripts["add.js"],
		"make.js":  `return { file: blobs.make(input.text) }`,
		"again.js": `return { same: input.it }`}), client.Start{
		Input:   map[string]any{"question": "and again?", "so": carried(r.Output.(map[string]any)["conversation"], r.ID)},
		Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	if second := wait(t, l, id); second.Status != "done" {
		t.Fatalf("the agent that carried on ended %s: %s", second.Status, second.Error)
	}
	last := strings.ReplaceAll(lastShown(m2.sent[1]), `\`, "")
	for _, part := range []string{`"file":{"blob":"blob4"`, `"items":["the first"]`} {
		if !strings.Contains(last, part) {
			t.Errorf("the agent that carried on was not shown %s: %s", part, last)
		}
	}
}
