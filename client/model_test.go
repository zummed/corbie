package client_test

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
)

// A self-hosted model at a plain address: the turn is written the
// connection and, beside it, the key bound to that host over a connection
// that is not encrypted, so the key goes with every turn and nowhere else.
// At an encrypted address the key is bound to the host.
func Test_model_plain_address_key_bound_to_it(t *testing.T) {
	m := newModel(t, calls("call_1", "add", `{"n": 41}`), says("It is 42."))
	if !strings.HasPrefix(m.URL, "http://") {
		t.Fatalf("the stand-in is at %s, which is not a plain address", m.URL)
	}
	at, _ := url.Parse(m.URL)
	l := local(t, "all")
	setSecret(t, l, "TEST_KEY", "sk-test-key")
	id, err := l.Run(ctx, draft(t, agentDef(m.URL, ""), scripts), client.Start{
		Input: map[string]any{"question": "what is 41 plus one?"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("the agent ended %s/%s: %s", r.Status, r.Failure, r.Error)
	}
	if len(m.Calls) != 2 || m.Calls[0].Key != "sk-test-key" || m.Calls[1].Key != "sk-test-key" {
		t.Fatalf("the model was sent the calls %v", m.Calls)
	}
	stored, err := l.Definition(ctx, id)
	if want := `"allow":["net:` + at.Host + `","secret:TEST_KEY@net.unencrypted:` + at.Host + `"]`; err != nil || !strings.Contains(jsonOf(stored), want) {
		t.Fatalf("the stored definition does not give its turn %s: %s, %v", want, jsonOf(stored), err)
	}
	// Where the connection is encrypted the key is bound to it, and to
	// nowhere else.
	encrypted, err := machineParse(agentDef("https://models.example", ""), scripts)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"allow":["net:models.example:443","secret:TEST_KEY@net:models.example:443"]`; !strings.Contains(jsonOf(encrypted.Definition), want) {
		t.Fatalf("at an encrypted address the turn is not given %s: %s", want, jsonOf(encrypted.Definition))
	}
}

// Each turn of an agent asks Anthropic's service to keep the request's
// start, with a mark on the request's last block, and starts with the
// tools, the system text and the messages of the turn before, the same
// but for the mark.
func Test_model_turn_starts_with_the_turn_before(t *testing.T) {
	m := newModel(t, calls("call_1", "add", `{"n": 41}`), calls("call_2", "double", `{"n": 42}`), says("It is 84."))
	def := strings.ReplaceAll(`
		agent: sums
		input: { question: any }
		model: { name: test-model, service: anthropic, address: ADDRESS, secret: TEST_KEY }
		system: You add numbers.
		prompt: ${input.question}
		tools:
		  add:
		    description: Add one to a number
		    args: { n: number }
		    job: { script: ./add.js, input: { n: "${args.n}" } }
		  double:
		    description: Double a number
		    args: { n: number }
		    job: { script: ./echo.js, input: { n: "${args.n}" } }
	`, "ADDRESS", m.URL)
	if r := runAgent(t, local(t, "all"), def, scripts); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	if len(m.Calls) != 3 {
		t.Fatalf("the model was called %d times", len(m.Calls))
	}
	type sent struct {
		Cache    json.RawMessage   `json:"cache_control"`
		Tools    json.RawMessage   `json:"tools"`
		System   json.RawMessage   `json:"system"`
		Messages []json.RawMessage `json:"messages"`
	}
	var turns []sent
	for _, c := range m.Calls {
		var s sent
		if err := json.Unmarshal(c.Body, &s); err != nil {
			t.Fatal(err)
		}
		if s.Cache != nil {
			t.Errorf("turn %d has the field cache_control beside its messages: %s", c.Turn, s.Cache)
		}
		// The mark is on the last block of the last message, and nowhere else.
		marks := 0
		for i, message := range s.Messages {
			var m map[string]any
			if err := json.Unmarshal(message, &m); err != nil {
				t.Fatal(err)
			}
			blocks, _ := m["content"].([]any)
			for j, block := range blocks {
				block := block.(map[string]any)
				if block["cache_control"] == nil {
					continue
				}
				marks++
				if i != len(s.Messages)-1 || j != len(blocks)-1 {
					t.Errorf("turn %d marks block %d of message %d", c.Turn, j+1, i+1)
				}
				delete(block, "cache_control")
			}
			unmarked, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			s.Messages[i] = unmarked
		}
		if marks != 1 {
			t.Errorf("turn %d asked the service to keep its start %d times", c.Turn, marks)
		}
		turns = append(turns, s)
	}
	for i := 1; i < len(turns); i++ {
		before, now := turns[i-1], turns[i]
		if !bytes.Equal(before.Tools, now.Tools) || !bytes.Equal(before.System, now.System) {
			t.Errorf("turn %d was sent the tools %s and the system text %s, after %s and %s", i+1, now.Tools, now.System, before.Tools, before.System)
		}
		if len(now.Messages) <= len(before.Messages) {
			t.Fatalf("turn %d was sent %d messages, after %d", i+1, len(now.Messages), len(before.Messages))
		}
		for j, message := range before.Messages {
			if !bytes.Equal(message, now.Messages[j]) {
				t.Errorf("turn %d sent message %d as %s, after %s", i+1, j+1, now.Messages[j], message)
			}
		}
	}
}
