package machines

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
)

const review = `statechart: review
input: { document: string }
initial: drafting
states:
  drafting:
    job: { machine: ./writer.yaml, input: { document: "${input.document}" } }
    on: { done: waiting, failed: gave_up }
  waiting:
    ticket: verdict
    on:
      verdict:
        - { go: approved, when: "${event.data.ok}" }
        - { go: drafting }
  approved: { end: done }
  gave_up: { end: failed }
`

const writer = `flow: writer
input: { document: string }
stages:
  draft:
    job: { script: ./write.js, input: { document: "${input.document}" }, files: [./lib.js] }
output: ${stages.draft.output}
`

// A lab is a store in memory with a controller, and the Machines and
// Toolkits pages on it, acting as the controller.
type lab struct {
	l     *client.Local
	pages *Pages
}

// newLab sets a lab up with nothing recorded.
func newLab(t *testing.T) *lab {
	t.Helper()
	l, err := client.NewLocal(context.Background(), permission.Layer{Allow: []string{"all"}}, func(string, error) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	l.Controller.AnyRunner = true
	return &lab{l: l, pages: New(Config{Controller: l.Controller.ID, Store: l.Controller.Store, Interval: time.Second, Shell: page.Shell{}})}
}

// submit records a directory as a tree at a prefix, by default / and the
// directory's name, as corbie run submit does.
func (b *lab) submit(t *testing.T, dir, at string) {
	t.Helper()
	tree, err := client.ReadTree(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	b.taken(t, tree)
}

// taken submits a tree and waits for the controller to record it.
func (b *lab) taken(t *testing.T, tree *client.Tree) {
	t.Helper()
	ctx := context.Background()
	message, err := b.l.Submit(ctx, tree, 0)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := b.l.WaitReceipt(ctx, message, 0, client.LocalInterval); err != nil || !receipt.Taken {
		t.Fatalf("the submit was not taken: %+v %v", receipt, err)
	}
}

// index gives what the pages read of the store now.
func (b *lab) index(t *testing.T) *index {
	t.Helper()
	ix, err := b.pages.h.index.current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

// recordedAt writes files into a new directory and records it at a prefix
// in a new lab.
func recordedAt(t *testing.T, at string, files map[string]string) *lab {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, files)
	b := newLab(t)
	b.submit(t, dir, at)
	return b
}

// reviewing is a directory with a statechart that runs a flow, and a file
// .corbieignore lists.
var reviewing = map[string]string{
	"review.yaml": review, "writer.yaml": writer, "write.js": "return 1", "lib.js": "module.exports = 1",
	"notes.yaml": "a: list of things\n", ".corbieignore": "notes.yaml\n",
}

func do(h http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	h.ServeHTTP(w, r)
	return w
}

// project writes a directory laid out as a project of ready-made machines
// is: a machine to keep running, machines in a directory whose name begins
// with a dot, and an agent with a toolkit from a file of its own, with the
// files that are no machine listed in .corbieignore.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{
		"router.yaml": "# the router of the bot\nflow: router\nstages:\n  listen: { job: { machine: ./.kit/listen.yaml } }\n",
		"later.yaml":  "flow: later\nstages:\n  post: { job: { machine: ./.kit/post.yaml } }\n",
		".kit/listen.yaml": `statechart: listen
initial: hearing
states:
  hearing:
    job: ./hear.js
    on:
      message: { send: { to: { at: .kit/conversation, key: "a" }, event: said } }
      done: talking
  talking:
    job: { machine: ./conversation.yaml }
    on: { done: hearing }
`,
		".kit/conversation.yaml": `# one conversation
statechart: conversation
initial: answering
states:
  answering:
    job:
      machine:
        flow: turn
        stages:
          answer: { job: { machine: support.yaml } }
          post: { job: { machine: ./post.yaml } }
    on: { done: over }
  over: { end: done }
`,
		".kit/unused.yaml":      "flow: unused\nstages:\n  a: { job: ./hear.js }\n",
		".kit/post.yaml":        "flow: post\nstages:\n  a: { job: ./hear.js }\n",
		".kit/hear.js":          "return 1",
		"support.yaml":          "agent: support\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\nask: { machine: .kit/post.yaml }\ntoolkits:\n  messages: { include: ./toolkits/toolkit.yaml }\n",
		"toolkits/toolkit.yaml": "description: messages\ntools:\n  tell: { description: tells, job: { machine: .kit/post.yaml, approve: true } }\n",
		"people/scopes.yaml":    "people: []\n",
		".corbieignore":         "toolkits/\npeople/scopes.yaml\nshared/\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	return dir
}

const shipper = `flow: shipper
allow: [net:x, secret:A@net:x]
stages:
  ship:
    job: { script: ./write.js, input: { hub: "${values.HUB}" }, allow: [net:y, secret:B@net:y, net:z, "secret:*@net:z"] }
`

// listOf gives the list as prefix, client and directory, then each
// machine's name, type and name, and lifetime.
func listOf(ix *index) []string {
	var out []string
	for _, t := range ix.Trees {
		out = append(out, t.Prefix+" "+t.Client+" "+t.Directory)
		for _, e := range t.Entries {
			out = append(out, e.Name+" "+e.First+" "+e.Lifetime)
		}
	}
	return out
}

func Test_machines_list_shows_what_each_item_is(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{
		"grouse/b.yaml":      "flow: b\ndescription: Counts grouse.\nlifetime: singleton\nstages:\n  a: { job: { machine: .kit/post.yaml } }\n",
		"bad.yaml":           "flow: bad\nstages: {}\ncolour: blue\n",
		"keyed.yaml":         "statechart: keyed\nlifetime: keyed\ninclude: ./shared/states.yaml\n",
		"shared/states.yaml": "initial: a\nstates:\n  a: { on: { go: b } }\n  b: { end: done }\n",
	})
	b := newLab(t)
	if p := pageOf(t, b.pages, "/machines"); len(p.List.Trees) != 0 || p.View.Open != "" {
		t.Errorf("a store with no tree lists %+v, with %q chosen", p.List.Trees, p.View.Open)
	}
	b.submit(t, dir, "/bot")
	// A file submitted alone is a tree of its own, whose prefix is its
	// location.
	alone := t.TempDir()
	write(t, alone, map[string]string{"flows/report.yaml": "flow: report\nstages:\n  a: { job: ./r.js }\n", "flows/r.js": "return 1"})
	tree, err := client.ReadFile(alone, "flows/report.yaml", "/report", client.ScriptJob{})
	if err != nil {
		t.Fatal(err)
	}
	b.taken(t, tree)
	real, _ := filepath.EvalSymlinks(dir)
	got := listOf(b.index(t))
	// A file with a problem is not recorded, and a listed file has no
	// location.
	want := []string{
		"/bot client " + real,
		".kit/conversation statechart: conversation transient", ".kit/listen statechart: listen transient", ".kit/post flow: post transient", ".kit/unused flow: unused transient",
		"grouse/b flow: b singleton", "keyed statechart: keyed keyed", "later flow: later transient", "router flow: router transient", "support agent: support transient",
		"/report client " + alone,
		"report flow: report transient",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the list is\n%s\nand not\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	p := pageOf(t, b.pages, "/machines?at=/bot/grouse/b")
	if e := p.entryOf("/bot/grouse/b"); e == nil || e.Type != "flow" || e.First != "flow: b" || e.Lifetime != "singleton" || e.Name != "grouse/b" || e.Description != "Counts grouse." {
		t.Errorf("the list gives /bot/grouse/b as %+v", e)
	}
	if e := p.entryOf("/report"); e == nil || e.Name != "report" {
		t.Errorf("a location its prefix names in full is listed as %+v", e)
	}
	if p.View.Open != "/bot/grouse/b" || p.View.First != "flow: b" || p.View.Kind != "flow" {
		t.Errorf("the page has %q chosen, as %q", p.View.Open, p.View.First)
	}
	if first := pageOf(t, b.pages, "/machines").View.Open; first != "/bot/.kit/conversation" {
		t.Errorf("with none chosen, the page shows %q", first)
	}
	if w := do(b.pages, "GET", "/machines?at=/bot/bad", nil); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "no machine is recorded at /bot/bad") {
		t.Errorf("a location with no machine recorded answered %d: %s", w.Code, w.Body.String())
	}
}

// A location's definition is read again only when a submit changed it.
func Test_machines_definition_is_read_once_per_submit(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, reviewing)
	b := newLab(t)
	b.submit(t, dir, "/docs")
	first := b.index(t)
	if again := b.index(t); again != first {
		t.Errorf("the store is read again with nothing submitted")
	}
	if p, k := pageOf(t, b.pages, "/machines"), pageOf(t, b.pages, "/toolkits"); p.List.Stamp != first.Stamp || k.Toolkits.Stamp != first.Stamp {
		t.Errorf("the pages do not share what was read: %q %q %q", p.List.Stamp, k.Toolkits.Stamp, first.Stamp)
	}
	write(t, dir, map[string]string{"writer.yaml": strings.Replace(writer, "flow: writer", "flow: author", 1)})
	b.submit(t, dir, "/docs")
	next := b.index(t)
	if next == first || next.Stamp == first.Stamp {
		t.Fatalf("a submit that changed a file left the index as it was")
	}
	if next.machines["/docs/writer"] == first.machines["/docs/writer"] || next.machines["/docs/writer"].Draft.Definition.Name != "author" {
		t.Errorf("the changed file is not read again: %+v", next.machines["/docs/writer"].Draft.Definition)
	}
	// The statechart runs the flow as a child, so its own file changed
	// with it; the list is read from the definitions.
	if got := listOf(next); !slices.Contains(got, "writer flow: author transient") {
		t.Errorf("the list does not give the changed file: %v", got)
	}
	b.submit(t, dir, "/docs")
	if same := b.index(t); same != next {
		t.Errorf("a submit that changed nothing made the UI read the definitions again")
	}
}

func Test_machines_drawing_unfolds_as_the_address_says(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{"here.yaml": "flow: here\nstages:\n  look:\n    job:\n      machine:\n        agent: in\n        model: { name: claude-sonnet-5-5, secret: KEY }\n        prompt: hello\n        toolkits:\n          messages: { include: ./toolkits/toolkit.yaml }\n"})
	b := newLab(t)
	b.submit(t, dir, "/bot")
	h := b.pages
	listen := pageOf(t, h, "/machines?at=/bot/.kit/listen").View
	if listen.View != "drawing" || card(listen.Drawing, "hearing") == nil || card(listen.Drawing, ".kit/conversation#a") == nil {
		t.Fatalf("the machine is not drawn: %+v", listen.Drawing)
	}
	if c := card(listen.Drawing, ".kit/conversation#a"); c.Href != "" {
		t.Errorf("a send, which starts no child, unfolds: %+v", c)
	}
	talking := card(listen.Drawing, "talking")
	if talking == nil || talking.Href != "/machines?at=%2Fbot%2F.kit%2Flisten&open=talking" {
		t.Fatalf("a job that names a machine has no link: %+v", talking)
	}
	unfolded := pageOf(t, h, talking.Href).View
	answering := card(unfolded.Drawing, "answering")
	if answering == nil || card(unfolded.Drawing, "talking").In == nil {
		t.Fatalf("%s does not unfold the machine: %+v", talking.Href, unfolded.Drawing)
	}
	if !strings.HasPrefix(answering.Href, "/machines?") {
		t.Fatalf("the unfolded machine has no links: %+v", answering)
	}
	deeper := pageOf(t, h, answering.Href).View
	if c := card(deeper.Drawing, "answer"); c == nil || !slices.Contains(card(deeper.Drawing, "answering").Lines, "flow: turn") {
		t.Errorf("a machine written in place does not unfold: %+v", deeper.Drawing)
	}
	folded := card(deeper.Drawing, "talking")
	if folded == nil || folded.In == nil || folded.Href == "" {
		t.Fatalf("the unfolded box has no link: %+v", folded)
	}
	if u, _ := url.Parse(folded.Href); len(u.Query()["open"]) != 0 {
		t.Errorf("folding a box leaves what is inside it unfolded: %s", u)
	}

	agent := pageOf(t, h, "/machines?at=/bot/support").View
	if card(agent.Drawing, "tell") == nil || card(agent.Drawing, "ask") == nil || !slices.Contains(card(agent.Drawing, "tell").Lines, "approve: true") {
		t.Errorf("the agent's drawing does not hold tell, its approval and the ask: %+v", agent.Drawing)
	}
	if agent.Drawing.Agent == nil || !slices.ContainsFunc(agent.Drawing.Agent.Toolkits, func(k drawing.Toolkit) bool { return k.File == "toolkits/toolkit.yaml" }) {
		t.Errorf("the agent's drawing does not give its toolkit's file: %+v", agent.Drawing.Agent)
	}
	// A toolkit of an agent written in place has its file too.
	v := pageOf(t, h, "/machines?at=/bot/here&open=look").View
	look := card(v.Drawing, "look")
	if look == nil || look.In == nil || look.In.Agent == nil {
		t.Fatalf("the page does not draw the agent unfolded: %+v", v.Drawing)
	}
	if !slices.ContainsFunc(look.In.Agent.Toolkits, func(k drawing.Toolkit) bool { return k.Name == "messages" && k.File == "toolkits/toolkit.yaml" }) {
		t.Errorf("the toolkit's file is not given with it: %+v", look.In.Agent.Toolkits)
	}
}

func Test_machines_secrets_and_values_used_are_listed(t *testing.T) {
	files := map[string]string{
		"shipper.yaml": shipper,
		"shared.yaml":  "allow: [net:z, secret:C@net:z]\n",
		"inc.yaml":     "flow: inc\nallow: [{ include: ./shared.yaml#allow }]\nstages:\n  a: { job: { script: ./write.js, allow: [secret:C@net:z] } }\n",
		// A child machine's secrets are its parent's too.
		"outer.yaml":    "flow: outer\nstages:\n  a: { job: { machine: ./shipper.yaml } }\n",
		".corbieignore": "shared.yaml\n",
	}
	for name, text := range reviewing {
		files[name] = text
	}
	files[".corbieignore"] = "shared.yaml\nnotes.yaml\n"
	b := recordedAt(t, "/dock", files)
	ctx, s, ctl := context.Background(), b.l.Controller.Store, b.l.Controller.ID
	for _, set := range []error{
		secret.Set(ctx, s, ctl, "/", "A", "swordfish"),
		secret.SetValue(ctx, s, ctl, "/dock", "HUB", "hub.lan"),
		secret.Set(ctx, s, ctl, "/elsewhere", "B", "kept"),
	} {
		if set != nil {
			t.Fatal(set)
		}
	}
	shipping := pageOf(t, b.pages, "/machines?view=definition&at=/dock/shipper")
	want := []use{{"A", "secret", "set at /"}, {"B", "secret", "not set at /dock/shipper or above"}, {"HUB", "value", "set at /dock"}}
	if !slices.Equal(shipping.View.Uses, want) {
		t.Errorf("the secrets and values are shown as %+v, not %+v", shipping.View.Uses, want)
	}
	for _, kept := range []string{"hub.lan", "swordfish"} {
		if strings.Contains(shipping.JSON, kept) {
			t.Errorf("the Machines page reads %q:\n%s", kept, shipping.JSON)
		}
	}
	if got := pageOf(t, b.pages, "/machines?at=/dock/shipper").View.Uses; !slices.Equal(got, want) {
		t.Errorf("the drawing does not list the secrets: %+v", got)
	}
	if got := pageOf(t, b.pages, "/machines?at=/dock/inc").View.Uses; !slices.ContainsFunc(got, func(u use) bool { return u.Name == "C" }) {
		t.Errorf("a secret an included file names is not listed: %+v", got)
	}
	if got := pageOf(t, b.pages, "/machines?at=/dock/outer").View.Uses; len(got) != 3 || got[2] != (use{"HUB", "value", "set at /dock"}) {
		t.Errorf("a child machine's secrets and values are not listed: %+v", got)
	}
	if got := pageOf(t, b.pages, "/machines?at=/dock/review").View.Uses; len(got) != 0 {
		t.Errorf("a machine that uses no secret or value shows %+v", got)
	}
}

func Test_machines_a_machine_shows_its_location(t *testing.T) {
	b := recordedAt(t, "/home", map[string]string{
		"write.js":              "return 1",
		"devices/boiler.yaml":   "statechart: boiler\ndescription: Turns the boiler on and off.\nlifetime: singleton\nreceive: { set: [schedules/*, client:panel], stop: [client:panel] }\ninitial: idle\nstates:\n  idle: { on: { set: idle } }\n",
		"rooms/thermostat.yaml": "statechart: thermostat\nlifetime: keyed\nlimits: { runs: 3 }\nupdate: restart\nallow: [send:devices/boiler, send:/work/log]\napprove: [send:devices/boiler, net:hub.lan]\ninitial: on\nstates:\n  on: { on: { set: on } }\n",
		"services/weather.yaml": "flow: weather\nreceive: [rooms/*]\nstages:\n  a: { job: write.js }\n",
		"services/hub.yaml":     "flow: hub\nallow: [send:**, send:/**]\nstages:\n  a: { job: write.js }\n",
		"services/root.yaml":    "flow: root\nallow: [all]\nstages:\n  a: { job: write.js }\n",
		"services/closed.yaml":  "flow: closed\nreceive: []\nstages:\n  a: { job: write.js }\n",
		"services/find.yaml":    "flow: find\ndescription: Find a person.\nstages:\n  a: { job: write.js }\n",
		"services/look.yaml":    "flow: look\ndescription: Look a room up.\nstages:\n  a: { job: write.js }\n",
		"services/asker.yaml": "agent: asker\n" + agentHead + "allow: [send:devices/boiler]\ntools:\n  who: { call: services/find }\n  again: { call: services/find }\n" +
			"toolkits:\n  rooms: { description: the rooms, tools: { room: { call: services/look } } }\n",
		"services/both.yaml": "agent: both\n" + agentHead + "allow: [send:services/find, send:devices/boiler]\ntools:\n  who: { call: services/find }\n  again: { call: services/find }\n",
	})
	h := b.pages
	placeOf := func(at string) place {
		t.Helper()
		v := pageOf(t, h, "/machines?at="+at).View
		if v.Place == nil {
			t.Fatalf("%s has no location", at)
		}
		return *v.Place
	}
	transient := "a call, a send, or corbie run start /home/services/weather, starts each run"
	drain := "a changed file leaves its runs to finish on the definition they started with"
	for at, want := range map[string]place{
		"/home/devices/boiler": {Description: "Turns the boiler on and off.", Location: "/home/devices/boiler", Lifetime: "singleton", Started: "the submit starts its one run",
			Update: "restart", Updated: "a changed file cancels its runs", Receives: []string{"set from schedules/*, client:panel", "stop from client:panel"}},
		"/home/rooms/thermostat": {Location: "/home/rooms/thermostat", Lifetime: "keyed", Started: "a send to a key starts the run for that key", Runs: 3, Update: "restart", Updated: "a changed file cancels its runs",
			Sends: []string{"devices/boiler", "/work/log"}, Approve: []string{"send:devices/boiler", "net:hub.lan"}},
		"/home/services/weather": {Location: "/home/services/weather", Lifetime: "transient", Started: transient, Update: "drain", Updated: drain, Receives: []string{"every event from rooms/*"}},
		"/home/services/hub":     {Sends: []string{"** (every location in its tree)", "/** (every location)"}},
		"/home/services/root":    {Sends: []string{"all (every location)"}},
		"/home/services/closed":  {Receives: []string{"every event from none"}},
		// A tool written call: may send to the location it calls.
		"/home/services/asker": {Sends: []string{"devices/boiler", "services/find (by the tools again, who)", "services/look (by the tool room)"}},
		// A location allow: sends to that a tool also calls is listed once.
		"/home/services/both": {Sends: []string{"services/find (by the tools again, who)", "devices/boiler"}},
	} {
		got := placeOf(at)
		if want.Location == "" {
			// Only what the case is about is compared.
			want.Description, want.Location, want.Lifetime, want.Started, want.Update, want.Updated, want.Runs = got.Description, got.Location, got.Lifetime, got.Started, got.Update, got.Updated, got.Runs
			if want.Receives == nil {
				want.Receives = got.Receives
			}
			if want.Sends == nil {
				want.Sends = got.Sends
			}
			want.Approve = got.Approve
		}
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Errorf("%s shows\n%+v\nand not\n%+v", at, got, want)
		}
	}
}

func Test_machines_definition_shows_where_each_value_is_from(t *testing.T) {
	b := recordedAt(t, "/kin", map[string]string{
		"family.yaml":   "agent: family\ninclude: ./base.yaml\nmodel: { effort: high }\nallow:\n  - { include: ./base.yaml#allow }\n  - send:devices/*\ntools:\n  look: { description: looks, job: ./look.js }\n",
		"base.yaml":     "model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }\nprompt: |\n  ${input.said}\n  Be brief.\nallow: [\"fs.read:${input.dir}/**\"]\n",
		"look.js":       "return 1",
		".corbieignore": "base.yaml\n",
	})
	v := pageOf(t, b.pages, "/machines?at=/kin/family&view=definition").View
	if v.View != "definition" || v.Drawing != nil {
		t.Errorf("the Definition tab is not chosen: %q", v.View)
	}
	lines := text(v.Text)
	code := strings.Join(lines, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "[k:agent]: family ") || !strings.HasSuffix(lines[0], "[c:# family.yaml:1]") {
		t.Fatalf("the definition does not begin with its type and name:\n%s", code)
	}
	at := func(want string) string {
		t.Helper()
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), want) {
				_, from, _ := strings.Cut(l, "[c:# ")
				return strings.TrimSuffix(from, "]")
			}
		}
		t.Errorf("the definition does not hold %s:\n%s", want, code)
		return ""
	}
	for want, from := range map[string]string{
		"[k:name]: claude-sonnet-5-5":   "base.yaml:1",
		"[k:effort]: high":              "family.yaml:3",
		"- send:devices/*":              "family.yaml:6",
		"- fs.read:[x:${input.dir}]/**": "base.yaml:5",
		// A short form filled out has the line of the short form.
		"[k:script]: ./look.js": "family.yaml:8",
		"[k:prompt]: |":         "base.yaml:2",
		// A value no file writes has no line.
		"[k:lifetime]: transient": "",
	} {
		if got := at(want); got != from {
			t.Errorf("%s is from %q, not %q:\n%s", want, got, from, code)
		}
	}
	if !slices.Contains(lines, "  Be brief.") {
		t.Errorf("a text of several lines is not written as a block:\n%s", code)
	}
	if strings.Contains(code, "include") || strings.Contains(code, "[k:lines]") || strings.Contains(code, "[k:body]") {
		t.Errorf("the definition holds an include or its lines:\n%s", code)
	}
	// The fields follow the type and name by name.
	var keys []string
	for _, l := range lines[1:] {
		if k, _, ok := strings.Cut(l, "]"); ok && strings.HasPrefix(l, "[k:") {
			keys = append(keys, strings.TrimPrefix(k, "[k:"))
		}
	}
	if !slices.IsSorted(keys) || !slices.Contains(keys, "model") || !slices.Contains(keys, "allow") {
		t.Errorf("the fields are not written by name: %v", keys)
	}
}

func Test_machines_pages_only_read(t *testing.T) {
	b := recordedAt(t, "/docs", reviewing)
	ctx, s, ctl := context.Background(), b.l.Controller.Store, b.l.Controller.ID
	held := func() []string {
		names, err := store.Names(s.List(ctx, ctl.Name))
		if err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(names, func(n string) bool { return !strings.HasPrefix(n, store.Locations(ctl.Name)+"/") })
	}
	before := held()
	for _, path := range []string{"/machines", "/machines/save", "/machines/new", "/machines/start", "/machines/check", "/machines/secret-set", "/machines/raw", "/toolkits"} {
		form := url.Values{"at": {"/docs/review"}, "text": {"flow: x\n"}, "type": {"flow"}, "name": {"x"}, "value": {"v"}}
		if w := do(b.pages, "POST", path, form); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s answered %d", path, w.Code)
		}
	}
	for _, path := range []string{"/machines/test?at=/docs/review", "/machines/raw?at=/docs/review"} {
		if w := do(b.pages, "GET", path, nil); w.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d", path, w.Code)
		}
	}
	pageOf(t, b.pages, "/machines?at=/docs/review&view=definition")
	pageOf(t, b.pages, "/toolkits")
	// That the pages show no form is checked in the browser, by
	// machines_test.js and toolkits_test.js.
	if after := held(); len(before) == 0 || !slices.Equal(after, before) {
		t.Errorf("the store holds %v under locations/, and held %v", after, before)
	}
}

func Test_machines_a_runs_machine_is_found_by_its_location(t *testing.T) {
	// A run's machine is the one recorded at its location; the cards down
	// to a child are unfolded one inside another.
	b := recordedAt(t, "/docs", reviewing)
	ctx := context.Background()
	for _, c := range []struct {
		location string
		cards    []string
		href     string
	}{
		{"/docs/review", nil, "/machines?at=%2Fdocs%2Freview"},
		{"/docs/review", []string{"drafting", "draft"}, "/machines?at=%2Fdocs%2Freview&open=drafting&open=drafting%2Fdraft"},
		{"/work/review", nil, ""},
		{"/docs/notes", nil, ""},
	} {
		href, err := b.pages.Href(ctx, c.location, c.cards)
		if err != nil || href != c.href {
			t.Errorf("%s, %v, is at %q, %v", c.location, c.cards, href, err)
		}
	}
	v := pageOf(t, b.pages, "/machines?at=%2Fdocs%2Freview&open=drafting").View
	if c := card(v.Drawing, "drafting"); c == nil || c.In == nil || card(v.Drawing, "draft") == nil {
		t.Errorf("the address does not unfold the card: %+v", v.Drawing)
	}
}
