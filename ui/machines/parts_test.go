package machines

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/ui/drawing"
)

// shown is a Machines or Toolkits page's parts, as the page is given them
// to draw, and the JSON they are given as. List is the list of either
// page: Trees holds the machines of each tree on the Machines page, and
// Toolkits the list of the Toolkits page.
type shown struct {
	List struct {
		Stamp string
		Trees []listed
	} `json:"list"`
	View     view          `json:"view"`
	Toolkits toolkitList   `json:"-"`
	Toolkit  toolkitChosen `json:"toolkit"`
	JSON     string        `json:"-"`
}

var partsAt = regexp.MustCompile(`(?s)<script type="application/json" id="parts">(.*?)</script>`)

// pageOf reads a page of the handler at an address, and the parts it
// carries.
func pageOf(t *testing.T, h http.Handler, target string) shown {
	t.Helper()
	w := do(h, "GET", target, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("%s answered %d: %s", target, w.Code, w.Body.String())
	}
	m := partsAt.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("%s carries no parts:\n%s", target, w.Body.String())
	}
	var s shown
	if err := json.Unmarshal([]byte(m[1]), &s); err != nil {
		t.Fatalf("%s carries parts that do not read: %v", target, err)
	}
	if err := json.Unmarshal([]byte(m[1]), &struct {
		List *toolkitList `json:"list"`
	}{&s.Toolkits}); err != nil {
		t.Fatalf("%s carries a list that does not read: %v", target, err)
	}
	var v jsontext.Value = []byte(m[1])
	v.Indent()
	s.JSON = string(v)
	return s
}

// entryOf gives the machine of the list at a location.
func (s shown) entryOf(location string) *entry {
	for _, t := range s.List.Trees {
		for i, e := range t.Entries {
			if e.Location == location {
				return &t.Entries[i]
			}
		}
	}
	return nil
}

// text gives a definition as the page shows it, a line at a
// time, each line's spans written as class:text.
func text(lines []codeLine) []string {
	var out []string
	for _, l := range lines {
		var b strings.Builder
		for _, s := range l.Spans {
			if s.Class != "" {
				b.WriteString("[" + s.Class + ":" + s.Text + "]")
			} else {
				b.WriteString(s.Text)
			}
		}
		out = append(out, b.String())
	}
	return out
}

// cards gives every card of a drawing, the cards unfolded inside cards
// among them.
func cards(s *drawing.Scene) []drawing.Card {
	if s == nil {
		return nil
	}
	var out []drawing.Card
	add := func(c drawing.Card) {
		out = append(out, c)
		out = append(out, cards(c.In)...)
	}
	if s.Flow != nil {
		for _, st := range s.Flow.Stages {
			add(st.Card)
		}
	}
	if s.Chart != nil {
		for _, st := range s.Chart.States {
			add(st.Card)
		}
	}
	if s.Agent != nil {
		for _, k := range s.Agent.Toolkits {
			for _, tool := range k.Tools {
				add(tool.Card)
			}
		}
		for _, hook := range s.Agent.Hooks {
			add(hook.Card)
		}
	}
	if s.Decision != nil {
		add(s.Decision.Answers)
	}
	if s.Kit != nil {
		for _, tool := range s.Kit.Tools {
			add(tool.Card)
		}
		for _, hook := range s.Kit.Hooks {
			add(hook.Card)
		}
	}
	return out
}

// card gives the card of a drawing with a name, at any depth.
func card(s *drawing.Scene, name string) *drawing.Card {
	all := cards(s)
	if i := slices.IndexFunc(all, func(c drawing.Card) bool { return c.Name == name }); i >= 0 {
		return &all[i]
	}
	return nil
}
