package agent

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/plugin/llm"
)

// A service that is a program, as the one for Claude Code is: it is on the
// runner's machine, and has tools of its own.
func init() {
	llm.Register(&llm.Service{Name: "a-program", Program: "a-program", Efforts: []string{"low", "most"},
		Needs: func(string) string { return "all" },
		Chat:  func(*plugin.Job, *llm.Call) (*llm.Reply, error) { return nil, nil }})
}

// An agent's model is of a service that has chat. The System One service is
// no sort of an agent's model, and is not taken.
func Test_model_agent_model_has_chat(t *testing.T) {
	for _, m := range []Model{{Name: "jev-latest"}, {Name: "some-model", Service: "systemone", Address: "https://models.example"}} {
		if err := m.check(); err == nil || !strings.Contains(err.Error(), "has no chat") {
			t.Errorf("an agent whose model is %+v passed its check: %v", m, err)
		}
	}
	d, _ := machine.Describe("agent")
	if _, ok := d.Shapes["model"].Sorts.Fields["systemone"]; ok {
		t.Error("the System One service is a sort of an agent's model")
	}
}

// An unknown field of a model names the fields its service's models hold,
// and of a tool the fields a tool takes.
func TestUnknownFieldNamesTheFieldsTaken(t *testing.T) {
	src := "agent: a\nmodel: { name: its-model, service: a-program, colour: blue }\nprompt: hi\ntools:\n  tell: { does: tells, job: ./t.js }\n"
	_, err := machine.Parse([]byte(src), func(string) ([]byte, error) { return []byte("return 1"), nil })
	for _, want := range []string{
		`model.colour: "colour" is not a field that belongs here: a model with service: a-program takes name, secret, service, effort, own`,
		`tools.tell.does: "does" is not a field that belongs here: a tool takes description, args, job, machine, call, input, approve, meta, open, when`,
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("read: %v\nwant: %s", err, want)
		}
	}
}

// A model is of one service or another, and holds what that service's
// models hold: a field it is not said to hold is refused, each effort its
// service takes is taken, and the service of a model that names none
// follows from its name.
func TestAModelHoldsWhatItsServiceSays(t *testing.T) {
	d, _ := machine.Describe("agent")
	model := d.Shapes["model"]
	sorts := model.Sorts
	if sorts == nil || sorts.By != "service" {
		t.Fatalf("a model is not said to be of one service or another: %+v", sorts)
	}
	services := slices.Sorted(maps.Keys(sorts.Fields))
	if !slices.Equal(services, slices.Sorted(slices.Values(chats()))) || len(services) == 0 {
		t.Fatalf("a model is of %v, and Corbie holds %v that have chat", services, chats())
	}
	for _, service := range services {
		s, _ := llm.Find(service)
		good := Model{Name: "some-model", Service: service}
		if s.Program == "" {
			good.Address = "https://models.example"
		}
		for _, level := range s.Efforts {
			m := good
			m.Effort = level
			if m.check() != nil {
				t.Errorf("%s takes the effort %s, and the check refuses it: %v", service, level, m.check())
			}
		}
		m := good
		m.Effort = "more-than-any"
		if m.check() == nil {
			t.Errorf("%s took an effort it does not take", service)
		}
		for field, with := range map[string]func(*Model){
			"address":      func(m *Model) { m.Address = "https://models.example" },
			"reply_tokens": func(m *Model) { m.ReplyTokens = 4000 },
			"own":          func(m *Model) { m.Own = []string{"WebSearch"} },
			"effort":       func(m *Model) { m.Effort = s.Efforts[0] },
		} {
			m := good
			if field != "address" {
				with(&m)
			} else if s.Program != "" {
				with(&m)
			}
			if held, taken := slices.Contains(sorts.Fields[service], field), m.check() == nil; held != taken {
				t.Errorf("a model of %s is said to hold %s: %v, and the check takes it: %v (%v)", service, field, held, taken, m.check())
			}
		}
	}
	if got := model.Sort(map[string]any{"name": "claude-sonnet-5-5"}); got != "anthropic" {
		t.Errorf("a model named claude-sonnet-5-5 with no service written is of %q", got)
	}
	if got := model.Sort(map[string]any{"name": "sonnet", "service": "openai-chat"}); got != "openai-chat" {
		t.Errorf("a model whose service is written is of %q", got)
	}
}
