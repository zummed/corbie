package client_test

import (
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
	id, err := l.Start(ctx, draft(t, agentDef(m.URL, ""), scripts), client.Start{
		Input: map[string]any{"question": "what is 41 plus one?"}, Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
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
