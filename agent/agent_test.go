package agent

import (
	"encoding/json/v2"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/plugin/llm"
)

// A service that is a program, as the one for Claude Code is: it is on the
// runner's machine, and has tools of its own.
func init() {
	llm.Register(&llm.Service{Name: "a-program", Program: "a-program", Efforts: []string{"low", "most"},
		Known: []string{"its-model"}, Tools: []string{"Search", "Shell"}, Needs: func(string) string { return "all" }})
}

// What a person is offered for a model is what the model is checked
// against: every service, and for each the efforts it takes. A level a
// service is not said to take is refused.
func TestWhatAModelIsOfferedIsWhatItIsCheckedAgainst(t *testing.T) {
	d, _ := machine.Describe("agent")
	model := d.Shapes["model"]
	field := func(name string) machine.Field {
		i := slices.IndexFunc(model.Fields, func(f machine.Field) bool { return f.Name == name })
		if i < 0 {
			t.Fatalf("a model has no field %s", name)
		}
		return model.Fields[i]
	}
	services := field("service").Values
	if !slices.Equal(services, llm.Names()) || len(services) == 0 {
		t.Fatalf("the services offered are %v, and Corbie holds %v", services, llm.Names())
	}
	names, efforts := field("name"), field("effort")
	if !names.Open || names.By != "service" || efforts.Open || efforts.By != "service" {
		t.Fatalf("a model's name is any name, and its effort one of its service's: %+v %+v", names, efforts)
	}
	for _, service := range services {
		s, _ := llm.Find(service)
		m := Model{Name: "some-model", Service: service}
		if s.Program == "" {
			m.Address = "https://models.example"
		}
		for _, level := range efforts.ValuesBy[service] {
			if m.Effort = level; m.check() != nil {
				t.Errorf("%s is offered the effort %s, and the check refuses it: %v", service, level, m.check())
			}
		}
		if m.Effort = "more-than-any"; m.check() == nil {
			t.Errorf("%s took an effort it is not offered", service)
		}
		// A model it is known to offer is taken, as any name is.
		for _, name := range names.ValuesBy[service] {
			if m.Name, m.Effort = name, ""; m.check() != nil {
				t.Errorf("%s is offered the model %s, and the check refuses it: %v", service, name, m.check())
			}
		}
	}
	// A model is of one service or another, and holds what that service's
	// models hold: a field it is not said to hold is refused, and the
	// service of a model that names none follows from its name.
	sorts := model.Sorts
	if sorts == nil || sorts.By != "service" {
		t.Fatalf("a model is not said to be of one service or another: %+v", sorts)
	}
	for _, service := range services {
		s, _ := llm.Find(service)
		good := Model{Name: "some-model", Service: service}
		if s.Program == "" {
			good.Address = "https://models.example"
		}
		for field, with := range map[string]func(*Model){
			"address": func(m *Model) { m.Address = "https://models.example" },
			"reply":   func(m *Model) { m.Reply = 4000 },
			"own":     func(m *Model) { m.Own = []string{"WebSearch"} },
			"effort":  func(m *Model) { m.Effort = s.Efforts[0] },
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
	// The tools of its own a program is known to have are offered for it, and any may be named.
	if own := field("own"); !own.Open || own.By != "service" || !slices.Equal(own.ValuesBy["a-program"], []string{"Search", "Shell"}) || own.ValuesBy["anthropic"] != nil {
		t.Errorf("a program's own tools are not offered by its service: %+v", own)
	}
	// A client is given the sorts with the rest.
	sent, err := json.Marshal(d, json.Deterministic(true))
	if err != nil || !strings.Contains(string(sent), `"sorts":{"by":"service","fields":{"a-program":["name","key","service","effort","own"],"anthropic":["name","key","service","effort","address","reply"]`) {
		t.Errorf("a client is not given what a model of each service holds: %v\n%s", err, sent)
	}

	// With no service written it follows from the name. The names offered
	// there are each one a service claims; and the efforts are every level
	// such a service takes, so that none its own service would take is
	// kept from a person, and each is taken for some model offered.
	if len(names.ValuesBy[""]) == 0 || len(efforts.ValuesBy[""]) == 0 {
		t.Fatalf("nothing is offered where no service is written: %v %v", names.ValuesBy[""], efforts.ValuesBy[""])
	}
	for _, name := range names.ValuesBy[""] {
		if m := (Model{Name: name}); m.check() != nil {
			t.Errorf("%s is offered with no service written, and the check refuses it: %v", name, m.check())
		}
		service, _ := llm.Find(llm.Of(name))
		for _, level := range service.Efforts {
			if !slices.Contains(efforts.ValuesBy[""], level) {
				t.Errorf("%s takes the effort %s, and it is not offered where no service is written", name, level)
			}
		}
	}
	for _, level := range efforts.ValuesBy[""] {
		if !slices.ContainsFunc(names.ValuesBy[""], func(name string) bool { return (Model{Name: name, Effort: level}).check() == nil }) {
			t.Errorf("the effort %s is offered with no service written, and no model offered there takes it", level)
		}
	}
}
