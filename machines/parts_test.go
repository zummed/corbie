package machines

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie-ui/drawing"
)

// listedPart is the part "files" as the page is given it.
type listedPart struct {
	Dir, DirFull, Open, Prefix, Unread string
	Groups                             []group
	Files                              []entry
	Ignore                             bool
	Problems                           []problem
}

// shown is a Machines or Toolkits page's parts, as the page is given them
// to draw, and the JSON they are given as.
type shown struct {
	Files    listedPart   `json:"files"`
	View     view         `json:"view"`
	Disk     disk         `json:"disk"`
	Toolkits toolkitsView `json:"toolkits"`
	JSON     string       `json:"-"`
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
	var v jsontext.Value = []byte(m[1])
	v.Indent()
	s.JSON = string(v)
	return s
}

// entryOf gives the machine of the list in a file.
func (s shown) entryOf(file string) *entry {
	for _, g := range s.Files.Groups {
		for i, e := range g.Entries {
			if e.Path == file {
				return &g.Entries[i]
			}
		}
	}
	return nil
}

// text gives a file's text as the page shows it, a line at a time, each
// line's spans written as class:text, and the lines marked with a !.
func text(lines []codeLine) []string {
	var out []string
	for _, l := range lines {
		var b strings.Builder
		if l.Bad {
			b.WriteString("!")
		}
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

// problemsSaid gives the problems as the page writes them, with the
// machines each breaks.
func problemsSaid(found []problem) string {
	var out []string
	for _, p := range found {
		line := p.File + " " + p.Place + " " + p.Text
		if p.Top {
			line = p.File + "/ is the top of a tree"
		}
		if len(p.Breaks) > 0 {
			line += " breaks " + strings.Join(p.Breaks, ",")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
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
		if s.Agent.Ask != nil {
			add(*s.Agent.Ask)
		}
	}
	if s.Decision != nil {
		add(s.Decision.Answers)
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
