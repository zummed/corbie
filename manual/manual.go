// Package manual is the documentation the binary carries: pages of plain
// text with examples, for a person and for a model that has been asked to
// write definitions and scripts.
package manual

import (
	"embed"
	"slices"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// A Page is one page of the manual.
type Page struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Summary string `json:"summary"`
	Text    string `json:"text,omitzero"`
	// Needs are, for a plugin, the kinds of permission its calls ask for:
	// those its page names where it says what it needs.
	Needs []string `json:"needs,omitzero"`
}

//go:embed pages/*.txt
var ideas embed.FS

// The pages for the ideas a user needs, in the order to read them: each
// with the group the list shows it in.
var order = []struct{ name, group string }{
	{"corbie", "Start"},
	{"first-machine", "Start"},
	{"a-list", "Start"},
	{"machines-together", "Start"},
	{"a-real-run", "Start"},

	{"install", "Setting up"},
	{"one-computer", "Where it runs"},
	{"served-directory", "Where it runs"},
	{"bucket", "Where it runs"},
	{"functions", "Where it runs"},

	{"tightening", "Tasks"},
	{"using-secrets", "Tasks"},
	{"schedules", "Tasks"},
	{"approval", "Tasks"},
	{"gateway", "Tasks"},

	{"how-a-run-works", "How it works"},
	{"the-store", "How it works"},
	{"limits", "How it works"},

	{"definitions", "Reference"},
	{"scripts", "Reference"},
	{"permissions", "Reference"},
	{"runs", "Reference"},
	{"routes", "Reference"},
	{"parties", "Reference"},
}

var more []func() []Page

// Add has more pages made when the manual is read: the command line adds
// its commands' pages this way, from its one table.
func Add(pages func() []Page) { more = append(more, pages) }

// summary is a page's first line.
func summary(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(line)
}

// Pages lists every page: the ideas, the commands, each machine type and
// each plugin and library the binary holds.
func Pages() []Page {
	var pages []Page
	for _, idea := range order {
		text, err := ideas.ReadFile("pages/" + idea.name + ".txt")
		if err != nil {
			continue
		}
		pages = append(pages, Page{Name: idea.name, Group: idea.group, Summary: summary(string(text)), Text: string(text)})
	}
	for _, add := range more {
		pages = append(pages, add()...)
	}
	for _, name := range machine.Types() {
		t, _ := machine.TypeOf(name)
		pages = append(pages, Page{Name: name, Group: "Machine types", Summary: summary(t.Manual()), Text: t.Manual()})
	}
	for _, name := range plugin.Names() {
		m, _ := plugin.Find(name)
		group := "Plugins"
		if m.Library != "" {
			group = "Libraries"
		}
		pages = append(pages, Page{Name: name, Group: group, Summary: summary(m.Page), Text: m.Page, Needs: needs(m.Page)})
	}
	slices.SortStableFunc(pages, func(a, b Page) int { return groupOrder(a.Group) - groupOrder(b.Group) })
	return pages
}

// needs reads the kinds of permission a plugin's page says it needs. The
// page says so in a paragraph that starts "Needs", giving each kind as an
// entry is written, with its scope: net:host, fs.read:path.
func needs(page string) []string {
	var kinds []string
	for _, paragraph := range strings.Split(page, "\n\n") {
		if !strings.HasPrefix(paragraph, "Needs ") {
			continue
		}
		for _, kind := range []string{permission.Env, permission.Exec, permission.Read, permission.Write, permission.Net, permission.Secret, permission.KeptRead, permission.KeptWrite} {
			if strings.Contains(paragraph, kind+":") && !slices.Contains(kinds, kind) {
				kinds = append(kinds, kind)
			}
		}
	}
	slices.Sort(kinds)
	return kinds
}

// groupOrder places a group: a group not named here, a program's own,
// comes before Corbie's.
func groupOrder(group string) int {
	return slices.Index([]string{"Start", "Setting up", "Where it runs", "Tasks", "How it works", "Reference", "Machine types", "Plugins", "Libraries", "Commands"}, group)
}

// Find gives the page of a name.
func Find(name string) (Page, bool) {
	pages := Pages()
	at := Named(pages, name)
	if at < 0 {
		return Page{}, false
	}
	return pages[at], true
}

// Named gives where a page of a name is: the page of that name, or else
// the command's page that it names without corbie.
func Named(pages []Page, name string) int {
	at := -1
	for i, page := range pages {
		if page.Name == name {
			return i
		}
		if page.Name == "corbie "+name && at < 0 {
			at = i
		}
	}
	return at
}
