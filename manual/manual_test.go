package manual_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/manual"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/runner"

	_ "github.com/zummed/corbie/agent"
	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
	_ "github.com/zummed/corbie/statechart"
)

// The test binary stands in for the process a runner starts for a job,
// in the examples' world, and for each program of that world.
func TestMain(m *testing.M) {
	if program, ok := programs[filepath.Base(os.Args[0])]; ok {
		os.Exit(program(os.Args[1:]))
	}
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		inWorld()
		notingCalls()
		runner.JobMain()
		return
	}
	os.Exit(m.Run())
}

// The test binary is built with an extension, as a binary of someone's own
// is: one more plugin and one more library, each bringing its page. Nothing
// in these tests but the one that looks for them knows they are there:
// their pages are listed, printed, run and checked because the binary
// holds them.
func init() {
	plugin.Register(&plugin.Module{Name: "greeting", Page: greetingPage, Calls: map[string]*plugin.Call{
		"make": {Options: []string{"loud"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			name, err := plugin.Text(first, "a name")
			if err != nil {
				return nil, err
			}
			loud, err := opt.Bool("loud", false)
			if err != nil {
				return nil, err
			}
			word, set, err := job.Variable("GREETING")
			if err != nil {
				return nil, err
			}
			if !set {
				word = "Hello"
			}
			if loud {
				return strings.ToUpper(word + ", " + name + "!"), nil
			}
			return word + ", " + name + ".", nil
		}},
	}})
	plugin.Register(&plugin.Module{Name: "echoes", Page: echoesPage, Library: `
exports.twice = function (text) { return text + " " + text }
`})
}

const greetingPage = `greeting: an extension's plugin, which greets.

  greeting.make(name, { loud })    a greeting for the name; with loud, a loud one

The word it greets with is the runner's variable GREETING, or Hello.

Needs env:GREETING.

    allow: [env:GREETING]

    greeting.make("world")                    // gives "Hello, world."
    greeting.make("world", { loud: true })    // gives "HELLO, WORLD!"
`

const echoesPage = `echoes: an extension's library, which repeats.

  require("echoes").twice(text)    the text, a blank, and the text again

    return require("echoes").twice("so")      // gives "so so"
`

// An extension's pages stand beside Corbie's own. They are in the list,
// the plugin with the kinds its call asks for, they print like any page,
// and their examples were run and checked with no other change.
func TestExtensionsPagesStandBesideCorbiesOwn(t *testing.T) {
	outcomes, err := ran()
	if err != nil {
		t.Fatal(err)
	}
	_, list, _ := corbie("", "manual")
	for _, extension := range []struct{ name, group, listed string }{
		{"greeting", "Plugins", "greeting: an extension's plugin, which greets.  (needs env)"},
		{"echoes", "Libraries", "echoes: an extension's library, which repeats."},
	} {
		// The part of the list under the group's name, to the next group.
		_, group, _ := strings.Cut(list, "\n"+extension.group+"\n")
		group, _, _ = strings.Cut(group, "\nLibraries\n")
		listed := false
		for _, entry := range strings.Split(group, "\n") {
			name, line, _ := strings.Cut(strings.TrimSpace(entry), " ")
			listed = listed || (name == extension.name && strings.TrimSpace(line) == extension.listed)
		}
		if !listed {
			t.Errorf("corbie manual does not list %s among the %s as %q:\n%s", extension.name, strings.ToLower(extension.group), extension.listed, list)
		}
		page, _ := manual.Find(extension.name)
		if status, printed, _ := corbie("", "manual", extension.name); status != 0 || printed != page.Text || !strings.Contains(printed, "an extension's") {
			t.Errorf("corbie manual %s ended %d and printed:\n%s", extension.name, status, printed)
		}
		compared, taken := 0, 0
		for _, o := range outcomes {
			if o.example.page == extension.name && o.failed == "" && o.differs == "" && len(o.without) == 0 {
				compared, taken = compared+o.checked, taken+o.taken
			}
		}
		if compared == 0 || (extension.name == "greeting" && taken != 1) {
			t.Errorf("the examples of %s: %d stated values were compared, and %d permissions were taken away in turn", extension.name, compared, taken)
		}
	}
}

// Every example of every page is run: a paragraph a page sets in by four
// spaces is one, and there is nothing a page can write to have one left
// out. A page whose example no longer works fails here.
func Test_man_no_example_is_skipped(t *testing.T) {
	outcomes, err := ran()
	if err != nil {
		t.Fatal(err)
	}
	run := map[string]int{}
	for _, o := range outcomes {
		run[o.example.page]++
		if o.failed != "" {
			t.Errorf("%s does not run: %s", o.example.where(), o.failed)
		}
	}
	// Nothing written in an example or around it leaves it out: not a
	// remark, not a line of prose straight above it. Only a line that
	// carries on an entry of a list is not one.
	made := manual.Page{Name: "made up", Text: "Prose, and no line left empty:\n    return 1\n\nMore prose.\n\n" +
		"    // skip: not run\n    return 2\n\n  a list     of one entry\n             carried on\n\n    # skip.yaml\n    flow: skip\n"}
	if found := examplesOf(made); len(found) != 3 || found[0].line != 2 || found[1].line != 6 || found[2].line != 12 {
		t.Errorf("a page with three examples was read as having %d: %+v", len(found), found)
	}
	for _, kind := range machine.Types() {
		if run[kind] == 0 {
			t.Errorf("the page of the machine type %s has no example", kind)
		}
	}
	for _, name := range plugin.Names() {
		if run[name] == 0 {
			t.Errorf("the page of %s has no example", name)
		}
	}
}

// An example that reaches outside the machine reaches a stand-in: a job
// has the examples' world in place of the network, and nothing it asks
// for goes further.
func Test_man_examples_run_without_network(t *testing.T) {
	if _, err := ran(); err != nil {
		t.Fatal(err)
	}
	standIns := map[string]bool{}
	for _, line := range went.reached {
		what, where, _ := strings.Cut(line, " ")
		if what != "stand-in" {
			t.Errorf("an example reached for %s, and the examples' world has no stand-in for it", where)
		}
		standIns[where] = true
	}
	// The examples do reach outside: a web server, a model service, a
	// raw connection, another machine.
	for _, where := range []string{"api.github.com:443", "api.anthropic.com:443", "whois.iana.org:43", "build.example:22"} {
		if !standIns[where] {
			t.Errorf("no example reached the stand-in for %s", where)
		}
	}
}

// Where a page states what an example gives, in a script or under a
// command, that is what it gave.
func Test_man_example_output_matches_the_page(t *testing.T) {
	outcomes, err := ran()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, o := range outcomes {
		checked += o.checked
		if o.differs != "" {
			t.Errorf("%s does not give what the page states: %s", o.example.where(), o.differs)
		}
	}
	if checked < 20 {
		t.Errorf("only %d stated values were compared with what the examples gave", checked)
	}
}

// An example that states what it is allowed is allowed what it needs and no
// more: run again without any one entry of its list, it ends denied,
// refused something that entry allowed.
func TestExampleIsDeniedWithoutAnyOneOfItsPermissions(t *testing.T) {
	outcomes, err := ran()
	if err != nil {
		t.Fatal(err)
	}
	taken := 0
	for _, o := range outcomes {
		taken += o.taken
		for _, fault := range o.without {
			t.Errorf("%s: %s", o.example.where(), fault)
		}
	}
	if taken < 30 {
		t.Errorf("only %d entries were taken away from what the examples are allowed", taken)
	}
}

// The kinds of permission a plugin's page says it needs are the kinds its
// calls asked Corbie for when the examples ran, no more and no fewer.
func Test_man_plugin_kinds_match_what_calls_ask_for(t *testing.T) {
	if _, err := ran(); err != nil {
		t.Fatal(err)
	}
	// What the jobs noted: each call that was made, each call of
	// something a call left open, and each kind a call asked for.
	asked := map[string][]string{}
	has, made := map[string]bool{}, map[string]bool{}
	for _, name := range plugin.Names() {
		m, _ := plugin.Find(name)
		for call := range m.Calls {
			has[name+"."+call] = true
		}
	}
	for _, line := range went.asked {
		parts := strings.Fields(line)
		switch call := parts[0] + "." + parts[1]; parts[2] {
		case "has":
			has[call] = true
		case "made":
			made[call] = true
		default:
			if !slices.Contains(asked[parts[0]], parts[2]) {
				asked[parts[0]] = append(asked[parts[0]], parts[2])
			}
		}
	}
	// What a call asks for is known only of a call that was made, so
	// every call of every plugin is made by some example.
	var never []string
	for call := range has {
		if !made[call] {
			never = append(never, call)
		}
	}
	sort.Strings(never)
	if len(never) > 0 {
		t.Errorf("no example makes these calls, so what they ask for is not known: %s", strings.Join(never, ", "))
	}
	var list bytes.Buffer
	if status := cli.Run(context.Background(), []string{"manual"}, strings.NewReader(""), &list, &list); status != 0 {
		t.Fatalf("corbie manual ended %d: %s", status, list.String())
	}
	for _, page := range manual.Pages() {
		if page.Group != "Plugins" {
			continue
		}
		if !strings.Contains(page.Text, "\nNeeds ") {
			t.Errorf("the page %q has no paragraph that starts \"Needs\" and says what its calls need", page.Name)
		}
		got := asked[page.Name]
		sort.Strings(got)
		if !slices.Equal(got, page.Needs) {
			t.Errorf("the page %q says it needs %v, and in the examples its calls asked for %v", page.Name, page.Needs, got)
		}
		// The list of plugins gives each with those kinds.
		line := ""
		for _, listed := range strings.Split(list.String(), "\n") {
			if strings.HasPrefix(listed, "  "+page.Name+" ") {
				line = listed
			}
		}
		want := ""
		if len(got) > 0 {
			want = "(needs " + strings.Join(got, ", ") + ")"
		}
		if line == "" || !strings.HasSuffix(line, want) || (want == "" && strings.Contains(line, "(needs")) {
			t.Errorf("corbie manual lists %s as %q, and its calls asked for %v", page.Name, line, got)
		}
	}
}

// A written call is a call as its page gives it, on a line of a list, with
// the options it takes and what it gives:
//
//	fs.list(path, { deep })       the options in braces, after the first thing
//	c.read({ max, timeout })      or in its place, for a call that takes
//	                              only options
//	http.get(url, options)        or as those of the page's paragraph
//	                              that starts "Options:"
//	      and head, post          after the call, the calls written the same
//	      with { method }         after the call, more options of its own
//	      a file, as text         after the call, what it gives; or on the
//	                              line set in under it
//	      gives { status, out }   what it gives, as the names a map has
//	      [{ name, size }]        or as the names each map of a list has
//
// A call written after the plugin's name is the plugin's, and so is a
// library's, written after require("name"). One written after anything
// else is a call on what a call of the plugin's left open.
type written struct {
	options []string
	first   bool     // the options stand where another call's first thing goes
	says    bool     // the page says what it gives
	gives   []string // the names of what it gives, where the page gives them
	list    bool     // they are the names of each thing in a list
}

var (
	ofLibrary = regexp.MustCompile(`^require\("(\w+)"\)\.`)
	callLine  = regexp.MustCompile(`^(\w+)\.(\w+)(?:\(([^()]*)\)|\x60[^\x60]*\x60)\s*(.*)$`)
	inBraces  = regexp.MustCompile(`\{([^{}]*)\}`)
	theSame   = regexp.MustCompile(`^and (\w+(?:, \w+)*)$`)
	itsOwn    = regexp.MustCompile(`^with \{([^{}]*)\}$`)
	anOption  = regexp.MustCompile(`^  (\w+)(?:\s|$)`)
	aShape    = regexp.MustCompile(`(gives |\[)\{ ?(\w+(?:, \w+)*) ?\}`)
)

// say notes what a page says a call gives.
func (w *written) say(text string) {
	if text == "" {
		return
	}
	w.says = true
	if shape := aShape.FindStringSubmatch(text); shape != nil && w.gives == nil {
		w.gives, w.list = namesIn(shape[2]), shape[1] == "["
	}
}

// namesIn gives the names a list on a page holds, parted by commas or,
// where one of several is given, by bars.
func namesIn(list string) []string {
	var names []string
	for _, name := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == '|' }) {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// writtenCalls reads the calls a page gives: the plugin's or the library's
// own, those on what its calls leave open, and the options the page's
// "Options:" paragraphs list.
func writtenCalls(page manual.Page) (own, open map[string]*written, listed []string) {
	_, lines := split(page)
	inOptions := false
	for _, line := range strings.Split(page.Text, "\n") {
		if m := anOption.FindStringSubmatch(line); inOptions && m != nil {
			listed = append(listed, m[1])
		}
		inOptions = line == "Options:" || (inOptions && strings.TrimSpace(line) != "")
	}
	own, open = map[string]*written{}, map[string]*written{}
	var above []*written // the calls written on the lines just above
	for _, line := range lines {
		m := callLine.FindStringSubmatch(ofLibrary.ReplaceAllString(strings.TrimSpace(line), "$1."))
		if m == nil || !strings.HasPrefix(line, " ") {
			// A line set in under a call carries on what the page says
			// of it. Where nothing was said of it yet, the line says it
			// also of the calls above it of which nothing was said.
			if last := len(above) - 1; last >= 0 && strings.HasPrefix(line, " ") {
				silent := !above[last].says
				above[last].say(strings.TrimSpace(line))
				for i := last - 1; silent && i >= 0 && !above[i].says; i-- {
					above[i].say(strings.TrimSpace(line))
				}
				continue
			}
			above = nil
			continue
		}
		on, name, args, after := m[1], m[2], strings.TrimSpace(m[3]), m[4]
		w := &written{}
		if in := inBraces.FindStringSubmatch(args); in != nil {
			w.options, w.first = namesIn(in[1]), strings.HasPrefix(args, "{")
		} else if args == "options" || strings.HasSuffix(args, ", options") {
			w.options = slices.Clone(listed)
		}
		calls := []string{name}
		if more := itsOwn.FindStringSubmatch(after); more != nil {
			w.options = append(w.options, namesIn(more[1])...)
		} else if same := theSame.FindStringSubmatch(after); same != nil {
			calls = append(calls, namesIn(same[1])...)
		} else {
			w.say(after)
		}
		for _, call := range calls {
			if on == page.Name {
				own[call] = w
			} else {
				open[call] = w
			}
		}
		above = append(above, w)
	}
	return own, open, listed
}

// notingCalls is run by a job's process before the job. It has the calls
// of every plugin note what the manual's tests cannot see from outside:
// the names of what a call gave back, where its page gives names to hold
// them to; and, of each call on something a call left open, a connection
// or a page that was read, what it takes and whether it refuses what it
// does not. Those calls are there only once the thing is.
func notingCalls() {
	scratch, _ := os.Getwd()
	note := func(file, line string) {
		f, err := os.OpenFile(filepath.Join(filepath.Dir(scratch), file), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		fmt.Fprintln(f, line)
		f.Close()
	}
	for _, page := range manual.Pages() {
		m, isPlugin := plugin.Find(page.Name)
		if page.Group != "Plugins" || !isPlugin {
			continue
		}
		own, open, _ := writtenCalls(page)
		for name, call := range m.Calls {
			watched(page.Name+"\t"+name, call, own[name], open, note)
		}
	}
}

func watched(name string, call *plugin.Call, w *written, open map[string]*written, note func(file, line string)) {
	do := call.Do
	call.Do = func(job *plugin.Job, first any, options plugin.Options) (any, error) {
		result, err := do(job, first, options)
		names := func(of map[string]any) string { return strings.Join(slices.Sorted(maps.Keys(of)), ",") }
		switch got := result.(type) {
		case map[string]any:
			if w != nil && w.gives != nil {
				note("gave", name+"\tmap\t"+names(got))
			}
		case []any:
			if each, ok := append(got, nil)[0].(map[string]any); ok && w != nil && w.gives != nil {
				note("gave", name+"\tlist\t"+names(each))
			}
		case *plugin.Object:
			for inner, c := range got.Calls {
				takes, refuses := taken(job, c, open[inner] != nil && open[inner].first)
				note("takes", fmt.Sprintf("%s.%s\t%s\t%t", name, inner, strings.Join(takes, ","), refuses))
				watched(name+"."+inner, c, open[inner], open, note)
			}
		}
		return result, err
	}
}

// stray is an option no call takes.
const stray = "an option no call takes"

var knows = regexp.MustCompile(`\((?:its options are: ([\w, ]+)|it takes no options)\)$`)

// taken gives the options a call takes, and whether it refuses one it
// does not take. They are the options the call lists. A call that takes
// only options, written where another call's first thing goes, lists none
// of its own: it says which it knows when it is given one it does not.
func taken(job *plugin.Job, call *plugin.Call, inPlace bool) (takes []string, refuses bool) {
	refusal := func(first any, options plugin.Options) string {
		_, err := call.Invoke(job, "", "", first, options)
		var turned *plugin.Error
		if errors.As(err, &turned) && turned.Code == plugin.BadInput && strings.Contains(turned.Message, stray) {
			return turned.Message
		}
		return ""
	}
	refuses = refusal(nil, plugin.Options{stray: true}) != ""
	if len(call.Options) > 0 || !inPlace {
		return call.Options, refuses
	}
	said := knows.FindStringSubmatch(refusal(map[string]any{stray: true}, nil))
	if said == nil {
		return []string{stray}, false
	}
	return namesIn(said[1]), refuses
}

// A plugin's or a library's page is the full account of its calls. The
// calls a script can make and the calls the page gives are the same; each
// call takes the options its page gives and refuses any other; and the
// page says what each gives, which, where it names what a map holds, is
// what the examples got back. What a call takes is read from the call.
func TestPageGivesEveryCallItsOptionsAndWhatItGives(t *testing.T) {
	if _, err := ran(); err != nil {
		t.Fatal(err)
	}
	if went.unasked != nil {
		t.Fatal(went.unasked)
	}
	// A job that is allowed nothing, for the calls that are tried here.
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "the test"}}}}
	t.Cleanup(job.End)
	// What the examples' jobs noted, by plugin and call.
	noted := func(lines []string, of string) map[string][][]string {
		by := map[string][][]string{}
		for _, line := range lines {
			if parts := strings.Split(line, "\t"); parts[0] == of {
				by[parts[1]] = append(by[parts[1]], parts[2:])
			}
		}
		return by
	}
	compared := 0
	for _, page := range manual.Pages() {
		m, held := plugin.Find(page.Name)
		if !held {
			continue
		}
		own, open, listed := writtenCalls(page)
		// The calls a script can make are those it finds on what require
		// gives it.
		if gives, can := slices.Sorted(maps.Keys(own)), went.calls[page.Name]; !slices.Equal(gives, can) {
			t.Errorf("the page %q gives the calls %v, and a script can make %v", page.Name, gives, can)
		}
		known := map[string]bool{}
		gave := noted(went.gave, page.Name)
		compare := func(call string, takes []string, refuses bool, w *written) {
			if w == nil {
				w = &written{}
			}
			takes, gives := slices.Sorted(slices.Values(takes)), slices.Sorted(slices.Values(w.options))
			for _, option := range takes {
				known[option] = true
			}
			compared += len(takes)
			if !slices.Equal(takes, gives) {
				t.Errorf("the page %q gives %s.%s the options %v, and the call takes %v", page.Name, page.Name, call, gives, takes)
			}
			if !refuses {
				t.Errorf("%s.%s does not refuse an option its page does not give", page.Name, call)
			}
			if !w.says {
				t.Errorf("the page %q does not say what %s.%s gives", page.Name, page.Name, call)
			}
			if w.gives == nil {
				return
			}
			kind, names := "map", strings.Join(slices.Sorted(slices.Values(w.gives)), ",")
			if w.list {
				kind = "list"
			}
			if len(gave[call]) == 0 {
				t.Errorf("the page %q says %s.%s gives a %s of %s, and no example got one back", page.Name, page.Name, call, kind, names)
			}
			for _, got := range gave[call] {
				if got[0] != kind || got[1] != names {
					t.Errorf("the page %q says %s.%s gives a %s of %s, and an example got back a %s of %s", page.Name, page.Name, call, kind, names, got[0], got[1])
					break
				}
			}
		}
		for name, call := range m.Calls {
			if call.Variadic {
				// Called as the language calls a tag, with no options.
				compare(name, nil, true, own[name])
				continue
			}
			takes, refuses := taken(job, call, own[name] != nil && own[name].first)
			compare(name, takes, refuses, own[name])
		}
		// The calls on something left open, as the jobs that opened one
		// noted them. Every such thing has close, and one that is walked
		// has next, which the language calls.
		opened := map[string]bool{"close": true}
		for call, notes := range noted(went.takes, page.Name) {
			name := call[strings.LastIndex(call, ".")+1:]
			opened[name] = true
			if name == "next" && open[name] == nil {
				continue
			}
			compare(call, namesIn(notes[0][0]), notes[0][1] == "true", open[name])
		}
		for name, w := range own {
			if m.Calls[name] == nil && !w.says {
				t.Errorf("the page %q does not say what %s.%s gives", page.Name, page.Name, name)
			}
		}
		for name := range open {
			if !opened[name] {
				t.Errorf("the page %q gives the call %s on something a call leaves open, and nothing an example opened has it", page.Name, name)
			}
		}
		for _, option := range listed {
			if !known[option] {
				t.Errorf("the page %q lists the option %s, and no call of the plugin takes it", page.Name, option)
			}
		}
	}
	if compared < 50 {
		t.Errorf("only %d options of calls were compared with their pages", compared)
	}
}

// Everything the binary holds has a page, in the part of the list where
// such things are: each machine type, each plugin, each library and each
// command. And a page that is not of an idea is of something it holds.
func Test_man_everything_held_has_a_page(t *testing.T) {
	held := map[string]string{}
	for _, kind := range machine.Types() {
		held[kind] = "Machine types"
	}
	for _, name := range plugin.Names() {
		held[name] = "Plugins"
		if m, _ := plugin.Find(name); m.Library != "" {
			held[name] = "Libraries"
		}
	}
	for i := range cli.Commands {
		held["corbie "+cli.Commands[i].Name] = "Commands"
	}
	for name, group := range held {
		if page, ok := manual.Find(name); !ok || page.Group != group {
			t.Errorf("the binary holds %s, and there is no page of that name among the %s", name, strings.ToLower(group))
		}
	}
	for _, page := range manual.Pages() {
		if strings.TrimSpace(page.Text) == "" || page.Summary == "" {
			t.Errorf("the page %q is empty", page.Name)
		}
		if of := held[page.Name]; (of != "" || slices.Contains([]string{"Machine types", "Plugins", "Libraries", "Commands"}, page.Group)) && of != page.Group {
			t.Errorf("the page %q is among the %s, and the binary holds nothing of that name there", page.Name, strings.ToLower(page.Group))
		}
	}
}

// Every field a definition may hold is on a page: a type's own on the
// type's page, and what every type takes, its limits, and a job, on the
// page of definitions. What a definition is read by is the one account of its
// fields, so a field added there and written on no page is found here.
func Test_man_every_field_is_on_its_page(t *testing.T) {
	shared, _ := manual.Find("definitions")
	seen := 0
	for _, kind := range machine.Types() {
		page, _ := manual.Find(kind)
		d, _ := machine.Describe(kind)
		for name, shape := range d.Shapes {
			on := page
			if name == "definition" || name == "limits" || name == "job" {
				on = shared
			}
			for _, f := range shape.Fields {
				if seen++; f.Does == "" {
					t.Errorf("the field %s of %s does not say what it is for", f.Name, shape.What)
				}
				// A field is on a page as it is written, with its colon; or in
				// a list of fields: at the start of its own line, or among the
				// names between braces.
				name := regexp.QuoteMeta(f.Name)
				written := regexp.MustCompile(`(^|[^a-z_])` + name + `:|(?m)^ +` + name + `  |[{,] ?` + name + ` ?[,}]`)
				if !written.MatchString(on.Text) {
					t.Errorf("the field %s of %s is not on the page %q", f.Name, shape.What, on.Name)
				}
			}
		}
	}
	if seen < 60 {
		t.Errorf("only %d fields were looked for on their pages", seen)
	}
}

// A command's page is what the command's help prints: both are made from
// the one table of commands, so an argument or a flag written there is on
// the page.
func TestCommandsPageIsItsHelp(t *testing.T) {
	for i := range cli.Commands {
		name := cli.Commands[i].Name
		status, help, said := corbie("", append(strings.Fields(name), "--help")...)
		if status != 0 || said != "" {
			t.Fatalf("corbie %s --help ended %d and said %q", name, status, said)
		}
		if page, _ := manual.Find("corbie " + name); page.Text != help || !strings.Contains(help, cli.Commands[i].Usage) || !strings.Contains(help, cli.Commands[i].Summary) {
			t.Errorf("the page of corbie %s is\n%s\nand its help is\n%s", name, page.Text, help)
		}
	}
}

// A page is plain text: printed to a terminal it reads as it stands. It
// holds what can be printed and the ends of its lines, and nothing else.
func TestPagesArePlainText(t *testing.T) {
	for _, page := range manual.Pages() {
		if !utf8.ValidString(page.Text) {
			t.Errorf("the page %q is not text", page.Name)
		}
		for n, line := range strings.Split(page.Text, "\n") {
			if at := strings.IndexFunc(line, func(r rune) bool { return !unicode.IsPrint(r) }); at >= 0 {
				t.Errorf("the page %q, line %d, holds %q, which does not print as it stands", page.Name, n+1, line[at:at+1])
			}
		}
	}
}

// corbie runs one command and gives how it ended, its result, and what it
// said along the way.
func corbie(in string, args ...string) (status int, out, said string) {
	var result, along bytes.Buffer
	status = cli.Run(context.Background(), args, strings.NewReader(in), &result, &along)
	return status, result.String(), along.String()
}

// nothingSetUp puts a test on a machine with nothing of Corbie's: it
// stands in an empty home directory, and no config and no store is named.
// Nothing is there afterwards either.
func nothingSetUp(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CORBIE_CONFIG", "")
	t.Setenv("CORBIE_STORE", "")
	t.Chdir(home)
	t.Cleanup(func() {
		if left, _ := os.ReadDir(home); len(left) > 0 {
			t.Errorf("reading the manual made %s in a home directory that held nothing", left[0].Name())
		}
	})
}

// With nothing set up and nothing running, corbie manual lists every page
// once, and prints each whole under the name it is listed by. A name that
// is no page is a mistake, and no page is printed for it.
func Test_man_every_listed_page_prints(t *testing.T) {
	nothingSetUp(t)
	status, list, said := corbie("", "manual")
	if status != 0 || said != "" {
		t.Fatalf("corbie manual ended %d and said %q", status, said)
	}
	// The entries of the list are the lines set in under each group.
	var entries []string
	for _, line := range strings.Split(list, "\n") {
		if strings.HasPrefix(line, "  ") {
			entries = append(entries, line)
		}
	}
	pages := manual.Pages()
	seen := map[string]bool{}
	for _, page := range pages {
		if seen[page.Name] {
			t.Errorf("two pages are named %q", page.Name)
		}
		seen[page.Name] = true
		listed := 0
		for _, entry := range entries {
			// The name, a blank, and the page's first line.
			if rest, ok := strings.CutPrefix(entry, "  "+page.Name+" "); ok && strings.HasPrefix(strings.TrimLeft(rest, " "), page.Summary) {
				listed++
			}
		}
		if listed != 1 {
			t.Errorf("corbie manual lists the page %q %d times", page.Name, listed)
		}
		status, printed, said := corbie("", append([]string{"manual"}, strings.Fields(page.Name)...)...)
		if status != 0 || said != "" || printed != strings.TrimRight(page.Text, "\n")+"\n" {
			t.Errorf("corbie manual %s ended %d, said %q, and printed %d bytes of a page of %d", page.Name, status, said, len(printed), len(page.Text))
		}
	}
	if len(entries) != len(pages) {
		t.Errorf("corbie manual lists %d entries, and there are %d pages", len(entries), len(pages))
	}
	status, printed, said := corbie("", "manual", "nosuchpage")
	if status != 2 || printed != "" || !strings.Contains(said, "nosuchpage") || !strings.Contains(said, "corbie manual lists") {
		t.Errorf("corbie manual nosuchpage ended %d, printed %q and said %q", status, printed, said)
	}
}

// The model server's manual tool gives a model what corbie manual prints:
// the same list, and each page as the same text. It too needs nothing set
// up and nothing running.
func TestModelServerGivesThePagesAsPrinted(t *testing.T) {
	nothingSetUp(t)
	asked := []string{""}
	for _, page := range manual.Pages() {
		asked = append(asked, page.Name)
	}
	var calls strings.Builder
	for i, page := range asked {
		call, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call",
			"params": map[string]any{"name": "manual", "arguments": map[string]any{"page": page}}})
		if err != nil {
			t.Fatal(err)
		}
		calls.Write(append(call, '\n'))
	}
	status, out, said := corbie(calls.String(), "mcp")
	answers := strings.Split(strings.TrimSpace(out), "\n")
	if status != 0 || len(answers) != len(asked) {
		t.Fatalf("corbie mcp ended %d, said %q, and answered %d calls with %d answers", status, said, len(asked), len(answers))
	}
	// The calls run side by side, and each answer carries the ID of its
	// call.
	for _, line := range answers {
		var answer struct {
			ID     int `json:"id"`
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &answer); err != nil || answer.ID < 1 || answer.ID > len(asked) || len(answer.Result.Content) != 1 || answer.Result.IsError {
			t.Fatalf("the tool answered %s", line)
		}
		page := asked[answer.ID-1]
		if _, printed, _ := corbie("", append([]string{"manual"}, strings.Fields(page)...)...); answer.Result.Content[0].Text != printed {
			t.Errorf("asked for the page %q, the tool gave a text that is not what corbie manual prints:\n%s", page, answer.Result.Content[0].Text)
		}
	}
}

// With nothing set up, corbie manual --web serves a browser what corbie
// manual prints: a first page that leads to every page, and each page as
// the same words. It stays until it is stopped, and ends well.
func TestBrowserIsGivenThePagesAsPrinted(t *testing.T) {
	nothingSetUp(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	said, saying := io.Pipe()
	ended := make(chan int, 1)
	go func() {
		ended <- cli.Run(ctx, []string{"manual", "--web", "--listen", "127.0.0.1:0"}, strings.NewReader(""), io.Discard, saying)
		saying.Close()
	}()
	line, _ := bufio.NewReader(said).ReadString('\n')
	at := regexp.MustCompile(`http://\S+`).FindString(line)
	if at == "" {
		t.Fatalf("corbie manual --web did not say where the manual is: %q", line)
	}
	get := func(path string) (int, string) {
		t.Helper()
		answer, err := http.Get(at + path)
		if err != nil {
			t.Fatal(err)
		}
		defer answer.Body.Close()
		body, err := io.ReadAll(answer.Body)
		if err != nil {
			t.Fatal(err)
		}
		return answer.StatusCode, string(body)
	}
	// The first page leads to each part of the manual, and the page a part
	// opens at leads to every page of the part.
	_, first := get("/")
	leads := regexp.MustCompile(`href="([^"/.:][^"/:]*)"`)
	reached := map[string]bool{}
	for _, to := range leads.FindAllStringSubmatch(first, -1) {
		reached["/"+to[1]] = true
		_, opened := get("/" + to[1])
		for _, on := range leads.FindAllStringSubmatch(opened, -1) {
			reached["/"+on[1]] = true
		}
	}
	tags := regexp.MustCompile(`<[^>]+>`)
	for _, page := range manual.Pages() {
		path := "/" + url.PathEscape(page.Name)
		if !reached[path] {
			t.Errorf("the first page a browser is given does not lead, by way of a part, to the page %q", page.Name)
		}
		// The page's text, read back out of what a browser is shown: its
		// words in their order, however the lines fall.
		status, shown := get(path)
		_, text, _ := strings.Cut(shown, `<main class="page">`)
		text, _, _ = strings.Cut(text, `<nav class="turn"`)
		words := func(s string) string {
			return strings.Join(strings.Fields(regexp.MustCompile(`(?m)^[0-9]+\. |^- |^-{3,}$`).ReplaceAllString(s, " ")), " ")
		}
		_, text, _ = strings.Cut(html.UnescapeString(tags.ReplaceAllString(text, " ")), page.Group)
		// The title and what the page covers stand apart at its head; a
		// page whose first line gives it no title is headed by its name.
		// A page another names is shown by its title.
		titled := regexp.MustCompile(`corbie\s+manual\s+([a-z][a-z0-9-]*)`).ReplaceAllStringFunc(page.Text, func(found string) string {
			if to, ok := manual.Find(strings.Fields(found)[2]); ok {
				return to.Title()
			}
			return found
		})
		got, want := squeezed(words(text)), words(titled)
		if status != 200 || got != squeezed(strings.Replace(want, ": ", " ", 1)) && got != squeezed(page.Name+" "+want) {
			headed := squeezed(strings.Replace(want, ": ", " ", 1))
			at := 0
			for at < len(got) && at < len(headed) && got[at] == headed[at] {
				at++
			}
			t.Errorf("a browser asking for the page %q is answered %d, and not with the page's words: they part after %q", page.Name, status, got[max(0, at-60):at])
		}
	}
	// A page another names is a way to it, shown by its title.
	if _, first := get("/first-machine"); !strings.Contains(first, `<a class="to" href="a-list">A list</a>`) {
		t.Errorf("the page first-machine does not lead a browser to the page it names")
	}
	if status, _ := get("/nosuchpage"); status != 404 {
		t.Errorf("a browser asking for a page that is none is answered %d", status)
	}
	// Everything a page asks for beside itself is served with it.
	_, start := get("/first-machine")
	for _, asked := range regexp.MustCompile(`(?:src|href)="(web/[^"]+)"`).FindAllStringSubmatch(start, -1) {
		if status, _ := get("/" + asked[1]); status != 200 {
			t.Errorf("a page asks for %s, and is answered %d", asked[1], status)
		}
	}
	stop()
	if status := <-ended; status != 0 {
		t.Errorf("corbie manual --web, stopped, ended %d", status)
	}
	// A page is printed or every page is served, and where they are served
	// is said only when they are.
	for _, args := range [][]string{{"manual", "--web", "start"}, {"manual", "--listen", "127.0.0.1:0"}} {
		if status, printed, _ := corbie("", args...); status != 2 || printed != "" {
			t.Errorf("corbie %s ended %d and printed %q", strings.Join(args, " "), status, printed)
		}
	}
}

// squeezed is a text with its blanks and the marks around its commands
// taken out and its capitals put down, so that two settings of the same
// words are the same.
func squeezed(text string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "`", "").Replace(text))
}

// The model server's manual tool reads and starts nothing: a flag where a
// page's name goes is refused, and the call comes back.
func TestModelServersManualToolTakesNoFlag(t *testing.T) {
	nothingSetUp(t)
	for _, page := range []string{"--web", "start --web", "-web"} {
		call, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": "manual", "arguments": map[string]any{"page": page}}})
		if err != nil {
			t.Fatal(err)
		}
		status, out, _ := corbie(string(call)+"\n", "mcp")
		if status != 0 || !strings.Contains(out, `"isError":true`) {
			t.Errorf("asked for the page %q, the model server ended %d and answered %s", page, status, out)
		}
	}
}
