package drawing

import (
	"encoding/json/v2"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
)

// The tests of how a graph is laid out and drawn are in layout_test.js and
// drawing_test.js, which run in Chrome on the graphs these tests read in Go,
// given to them as fixtures by name. The tests here check the graphs.

func definition(t *testing.T, src string, files map[string]string) *machine.Definition {
	t.Helper()
	draft, err := machine.Parse([]byte(src), machine.Given(files))
	if err != nil {
		t.Fatalf("the definition does not check: %v", err)
	}
	return draft.Definition
}

func scene(t *testing.T, def *machine.Definition, run *Run) Scene {
	t.Helper()
	s, err := Draw(def, run)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// states gives a statechart's state at a step: the states it is in.
func states(names ...string) machine.Shown { return machine.Shown{States: names} }

const script = "return 1"

const review = `statechart: review
input: { document: string }
initial: drafting
states:
  drafting:
    job: { script: ./write.js, input: { document: "${input.document}" } }
    on:
      done: waiting
      failed: gave_up
  waiting:
    ticket: verdict
    timers: { too_long: 48h }
    on:
      verdict:
        - { go: approved, when: "${event.data.ok}" }
        - { go: drafting }
      too_long: gave_up
  approved: { end: done }
  gave_up: { end: failed }
`

const held = `statechart: held
initial: idle
states:
  idle:
    ticket: go
    on: { go: busy }
  busy:
    initial: one
    on: { failed: idle, cancel: idle, retry: busy }
    states:
      one:
        ticket: more
        on: { more: two }
      two: { end: done }
`

const two = "flow: two\nstages:\n  a: { job: ./w.js }\n  b: { job: ./w.js, follows: [a] }\n  c: { job: ./w.js, follows: [b] }\n"

const librarian = `agent: librarian
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
tools:
  shelf: { description: a shelf, args: { shelf: string }, job: ./w.js }
  helper: { description: a helper, args: { topic: string }, job: { machine: ./two.yaml } }
`

const helper = `agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
limits: { turns: 7 }
ask: { machine: ./approval.yaml }
tools:
  look: { description: looks at a thing, job: ./w.js }
  idle: { description: idles, job: ./w.js }
toolkits:
  messages:
    description: messages
    tools:
      tell: { description: tells someone, job: { script: ./w.js, approve: true } }
`

const toolkits = `agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
tools:
  look: { description: looks, job: ./w.js }
toolkits:
  bins:
    description: bins
    tools:
      days: { description: bin days, job: ./w.js }
      put: { description: puts out, job: ./w.js }
  home:
    description: home
    tools:
      lights: { description: lights, job: ./w.js }
  net:
    description: net
    tools:
      ping: { description: pings, job: ./w.js }
  servers:
    description: servers
    tools:
      disk: { description: disks, job: ./w.js }
`

const conditions = `agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
toolkits:
  orders:
    include: ./orders.yaml
    when: "${input.asker.scopes.includes('orders')}"
  notes:
    description: notes
    tools:
      jot: { description: jots, job: ./w.js }
tools:
  look: { description: looks, job: ./w.js, when: "${input.look}" }
`

const deploy = `flow: deploy
ask: { machine: ./approval.yaml }
stages:
  plan: { job: ./w.js }
  ship: { job: { script: ./w.js, approve: true }, follows: [plan] }
  pipeline: { job: { call: { at: ci/pipeline }, approve: true }, follows: [plan] }
  later: { job: { call: { at: "${input.at}" } }, follows: [plan] }
  far: { job: { call: { at: /work/ci/pipeline } }, follows: [plan] }
`

const trip = `flow: trip
stages:
  weather: { job: { call: { at: services/weather }, input: { days: 2 } } }
  heat:
    job:
      send: { to: { at: devices/boiler }, event: set, data: { target: "${stages.weather.output.celsius}" } }
  room:
    job:
      send: { to: { at: rooms/hall, key: "${input.k}" }, event: set }
`

const nested = `flow: a
stages:
  outer: { job: { machine: ./b.yaml } }
  plain: { job: ./w.js }
  named: { job: { machine: "${input.m}" } }
  here:
    job:
      machine:
        flow: written
        stages:
          there: { job: ./w.js }
`

// given are the files the definitions read.
var given = map[string]string{
	"w.js": script, "write.js": script,
	"two.yaml":      "flow: two\nstages:\n  a: { job: ./w.js }\n",
	"approval.yaml": "flow: approval\nstages:\n  a: { job: ./w.js }\n",
	"orders.yaml":   "description: orders\ntools:\n  read: { description: reads, job: ./w.js }\n",
	"messages.yaml": "description: messages\ntools:\n  tell: { description: tells, job: ./w.js }\n",
	"notes.yaml":    "description: notes\ntools: {}\n",
	"sub/c.yaml":    "flow: c\nstages:\n  deep: { job: w.js }\n",
	"b.yaml":        "flow: b\nstages:\n  inner: { job: { machine: ./sub/c.yaml } }\n",
}

// big gives a flow of sixty stages, each waiting for every one before it,
// and a statechart of two hundred states with five rules each.
func big() (string, string) {
	var flow, chart strings.Builder
	flow.WriteString("flow: wide\nstages:\n")
	chart.WriteString("statechart: big\ninitial: s0\nstates:\n")
	for i := range 60 {
		var follows []string
		for j := range i {
			follows = append(follows, "f"+strconv.Itoa(j))
		}
		flow.WriteString("  f" + strconv.Itoa(i) + ": { job: ./w.js, follows: [" + strings.Join(follows, ", ") + "] }\n")
	}
	for i := range 200 {
		chart.WriteString("  s" + strconv.Itoa(i) + ":\n    ticket: t" + strconv.Itoa(i) + "\n    on:\n")
		for j := range 5 {
			chart.WriteString("      e" + strconv.Itoa(j) + ": s" + strconv.Itoa((i+j*j+1)%200) + "\n")
		}
	}
	return flow.String(), chart.String()
}

// fixtures gives the graphs the browser tests draw, by name.
func fixtures(t *testing.T) map[string]any {
	t.Helper()
	def := func(src string) *machine.Definition { return definition(t, src, given) }
	f := map[string]any{}
	add := func(name string, d *machine.Definition, run *Run) { f[name] = scene(t, d, run) }

	add("fan", def(`flow: fan
stages:
  fetch:
    job: ./write.js
  sum:
    each: ${stages.fetch.output}
    job: ./write.js
  translate:
    job: { script: ./write.js, input: "${stages.fetch.output}" }
  send:
    follows: [sum]
    when: ${!stages.translate.failed}
    job: ./write.js
`), nil)
	add("chain", def("flow: chain\nstages:\n  a: { job: ./w.js }\n  b: { job: ./w.js, follows: [a] }\n  c: { job: ./w.js, follows: [a, b] }\n"), nil)
	add("review", def(review), nil)
	add("review-running", def(review), &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting"), Events: []string{"done"}}}})
	add("review-ended", def(review), &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting")}, {Shown: states("approved")}}, Ended: true})
	add("review-passed", def(review), &Run{Steps: []Step{{Shown: states("waiting"), Events: []string{"start", "done"}}}})
	add("review-back", def(review), &Run{Steps: []Step{{Shown: states("waiting")}, {Shown: states("drafting"), Events: []string{"verdict"}}}})
	add("two", def(`statechart: two
initial: s
states:
  s:
    ticket: go
    on: { left: a, right: b }
  a:
    ticket: more
    on: { ok: c }
  b:
    ticket: again
    on: { fine: c }
  c: { end: done }
`), nil)
	add("held", def(held), nil)
	add("listen", def(`statechart: listen
initial: hearing
states:
  hearing:
    job: ./w.js
    on:
      message: { send: { to: { at: chat, key: "a" }, event: said } }
      again: hearing
      done: over
  over: { end: done }
`), nil)
	examples, _ := filepath.Glob("testdata/*.yaml")
	var names []string
	for _, e := range examples {
		src, err := os.ReadFile(e)
		if err != nil {
			t.Fatal(err)
		}
		name := "testdata/" + filepath.Base(e)
		add(name, def(string(src)), nil)
		names = append(names, name)
	}
	f["examples"] = append(names, "review", "two", "held", "listen")

	add("flow-running", def(two), &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"a": "done", "b": "running", "c": "waiting"}}}}})
	add("flow-cancelled", def(two), &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"a": "done", "b": "running", "c": "waiting"}}}}, Ended: true})
	add("librarian-running", def(librarian), &Run{Steps: []Step{{Shown: machine.Shown{Turn: 2}, Asked: []string{"shelf"}}}, Going: []string{"shelf"}})
	add("librarian-failed", def(librarian), &Run{Steps: []Step{{Shown: machine.Shown{Turn: 1}, Asked: []string{"turn"}, Failed: []string{"turn"}}}, Ended: true, Failed: true})
	add("helper", def(helper), nil)
	add("helper-asking", def(helper), &Run{
		Steps: []Step{{Asked: []string{"look"}}, {Shown: machine.Shown{Turn: 2}, Asked: []string{"ask"}}},
		Going: []string{"tell", "ask"}, Held: []string{"tell"},
		Children: map[string]Child{"ask": {ID: "a-client/2", Machine: "approval", Type: "flow"}},
	})
	add("helper-corbie-tool", def(helper), &Run{Steps: []Step{{Shown: machine.Shown{Turn: 1}, Asked: []string{"blob_read"}}}, Going: []string{"blob_read"}})
	add("helper-included", def(`agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
toolkits:
  messages: { include: ./messages.yaml }
  notes: { include: ./notes.yaml }
`), nil)
	add("toolkits", def(toolkits), nil)
	add("toolkits-running", def(toolkits), &Run{Steps: []Step{{Asked: []string{"lights"}}, {Shown: machine.Shown{Turn: 1}, Asked: []string{"ping"}}}, Going: []string{"ping"}})
	add("conditions", def(conditions), nil)
	add("conditions-folded", def(conditions), &Run{Steps: []Step{{Asked: []string{"jot"}}}})
	add("deploy-running", def(deploy), &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"plan": "done", "ship": "running", "pipeline": "running"}}}}, Going: []string{"ship", "pipeline"}, Held: []string{"ship"}})
	add("deploys-held", def(`statechart: deploys
ask: { machine: ./approval.yaml }
initial: shipping
states:
  shipping:
    job: { script: ./w.js, approve: true }
    on: { done: over }
  over: { end: done }
`), &Run{Steps: []Step{{Shown: states("shipping")}}, Going: []string{"shipping"}, Held: []string{"shipping"}})
	add("trip", def(trip), nil)
	add("limited", def(limited), nil)
	add("trip-called", def(trip), &Run{Children: map[string]Child{"weather": {ID: "lab/7", Machine: "weather", Type: "flow"}}})
	add("triage", def(`decision: triage
input: { text: string }
model: { name: jev-latest, secret: TYPESAFE_KEY }
state: ${input.text}
questions:
  urgent: { type: noul, instructions: The customer cannot use the product. }
  team: { type: choice, instructions: Who handles it, criteria: { billing: money, support: help } }
`), nil)

	draft, err := machine.Parse([]byte(nested), machine.Given(given))
	if err != nil {
		t.Fatal(err)
	}
	unfold := func(open ...string) Scene {
		u := Unfold{Open: map[string]bool{}, Href: func(path string) string { return "/go?to=" + path }}
		for _, p := range open {
			u.Open[p] = true
		}
		s, err := Unfolded(draft, u)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	f["nested"] = unfold()
	f["nested-outer"] = unfold("outer")
	f["nested-deep"] = unfold("outer", "outer/inner", "here")
	f["nested-inner-alone"] = unfold("outer/inner")
	add("nested-run", draft.Definition, &Run{Children: map[string]Child{"outer": {ID: "a-client/1", Machine: "b", Type: "flow"}}})
	add("nested-machines", draft.Definition, &Run{Machines: map[string]Machine{"./b.yaml": {Type: "flow", Name: "b"}}})

	// A run of a flow whose stage started a child, drawn inside its card,
	// itself running a stage.
	child := scene(t, def("flow: pair\nstages:\n  a: { job: ./w.js }\n  b: { job: ./w.js, follows: [a] }\n"), &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"a": "done", "b": "running"}}}}})
	parent := def("flow: lookup\nstages:\n  find: { job: ./w.js }\n  check: { job: { machine: ./two.yaml }, follows: [find] }\n  tell: { job: ./w.js, follows: [check] }\n")
	children := map[string]Child{"check": {ID: "a-client/9", Machine: "two", Type: "flow"}}
	add("lookup-running", parent, &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"find": "running"}}}}, Going: []string{"find"}})
	add("lookup-in-child", parent, &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"find": "done", "check": "running"}}}}, Going: []string{"check"}, Children: children, In: map[string]Scene{"check": child}})

	flow, chart := big()
	add("big-flow", def(flow), nil)
	add("big-chart", def(chart), nil)
	return f
}

// in serves a page for the drawing's tests to draw on, with what every page
// is given and the drawing's files.
func in() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", Files()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page.Write(w, page.Shell{Title: "Drawing"}, struct{}{})
	})
	return mux
}

func Test_drawing_layout_in_the_browser(t *testing.T) {
	pagetest.Run(t, pagetest.Config{Handler: in(), Page: "/", Module: "drawing/layout_test.js", Fixtures: fixtures(t)})
}

func Test_drawing_drawn_in_the_browser(t *testing.T) {
	pagetest.Run(t, pagetest.Config{Handler: in(), Page: "/", Module: "drawing/drawing_test.js", Fixtures: fixtures(t)})
}

// A stage waits for each stage its expressions read, those it follows and
// the one whose items it takes, by the flow package's own rule.
func Test_drawing_flow_waits_by_the_flow_rule(t *testing.T) {
	s := scene(t, definition(t, `flow: fan
stages:
  fetch: { job: ./w.js }
  sum: { each: "${stages.fetch.output}", job: ./w.js }
  translate: { job: { script: ./w.js, input: "${stages.fetch.output}" } }
  send: { follows: [sum], when: "${!stages.translate.failed}", job: ./w.js }
  heat:
    job:
      send: { to: { at: devices/boiler }, event: set, data: { target: "${stages.fetch.output.celsius}" } }
`, given), nil)
	after := map[string][]string{}
	for _, st := range s.Flow.Stages {
		after[st.Card.Name] = st.After
	}
	want := map[string][]string{"fetch": {}, "sum": {"fetch"}, "translate": {"fetch"}, "send": {"translate", "sum"}, "heat": {"fetch"}}
	for name, w := range want {
		got := slices.Clone(after[name])
		slices.Sort(got)
		slices.Sort(w)
		if !slices.Equal(got, w) {
			t.Errorf("%s waits for %v, not %v", name, got, w)
		}
	}
}

// The graph a drawing is laid out from holds what each card says and how
// the run marks it, and no place on the drawing: the browser lays it out.
func Test_drawing_scene_is_the_graph_and_its_marks(t *testing.T) {
	s := scene(t, definition(t, review, given), &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting"), Events: []string{"done"}}}})
	marks := map[string]string{}
	for _, st := range s.Chart.States {
		marks[st.Name] = st.Mark
	}
	if s.Type != "statechart" || s.Chart.Start != "drafting" || marks["waiting"] != "now" || marks["drafting"] != "done" || marks["approved"] != "never" {
		t.Errorf("the states are marked %v, starting at %q", marks, s.Chart.Start)
	}
	var taken []string
	for _, r := range s.Chart.Rules {
		if r.Taken {
			taken = append(taken, r.From+" "+r.Event+" "+r.To)
		}
	}
	if !slices.Equal(taken, []string{"drafting done waiting"}) {
		t.Errorf("the rules taken are %v", taken)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`"(x|y|w|h|X|Y|W|H)":`).Match(data) {
		t.Errorf("the graph holds a place on the drawing:\n%s", data)
	}
}

// On a run's page, the graph of a run a job started is drawn inside the
// job's card, which links to that run.
func Test_drawing_child_run_is_inside_its_card(t *testing.T) {
	child := scene(t, definition(t, two, given), nil)
	s := scene(t, definition(t, "flow: lookup\nstages:\n  check: { job: { machine: ./two.yaml } }\n  tell: { job: ./w.js }\n", given),
		&Run{Children: map[string]Child{"check": {ID: "a-client/9", Machine: "two", Type: "flow"}}, In: map[string]Scene{"check": child}})
	for _, st := range s.Flow.Stages {
		switch st.Card.Name {
		case "check":
			if st.Card.In == nil || st.Card.In.Name != "two" || st.Card.Href != "/run/a-client/9" {
				t.Errorf("the child's card is %+v", st.Card)
			}
		case "tell":
			if st.Card.In != nil {
				t.Errorf("a card whose job started no run holds a drawing")
			}
		}
	}
}

// On the Machines page, a call's card links to the machine at the location
// it writes out; a location an expression gives is not looked up.
func Test_drawing_call_links_to_the_machine_it_calls(t *testing.T) {
	draft, err := machine.Parse([]byte("flow: deploy\nstages:\n  pipeline: { job: { call: { at: ci/pipeline } } }\n  later: { job: { call: { at: \"${input.at}\" } } }\n  gone: { job: { call: { at: ci/gone } } }\n"), machine.Given(nil))
	if err != nil {
		t.Fatal(err)
	}
	var asked []string
	s, err := Unfolded(draft, Unfold{Href: func(string) string { return "" }, Called: func(at string) string {
		asked = append(asked, at)
		if at == "ci/pipeline" {
			return "/machines?file=ci/pipeline.yaml"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	hrefs := map[string]string{}
	for _, st := range s.Flow.Stages {
		hrefs[st.Card.Name] = st.Card.Href
	}
	if hrefs["pipeline"] != "/machines?file=ci/pipeline.yaml" || hrefs["gone"] != "" || hrefs["later"] != "" {
		t.Errorf("the calls link to %v", hrefs)
	}
	if slices.Contains(asked, "${input.at}") {
		t.Errorf("a location an expression gives is looked up: %v", asked)
	}
}

// Reading the graph of a big machine is quick; laying it out is tested in
// the browser.
func Test_drawing_graph_of_a_big_machine_is_quick(t *testing.T) {
	flow, chart := big()
	for _, src := range []string{flow, chart} {
		def := definition(t, src, given)
		began := time.Now()
		scene(t, def, nil)
		if took := time.Since(began); took > time.Second {
			t.Errorf("%s took %v to read", def.Name, took)
		}
	}
}

func Test_drawing_files_are_served(t *testing.T) {
	for _, name := range []string{"/layout.js", "/drawing.js"} {
		w := httptest.NewRecorder()
		Files().ServeHTTP(w, httptest.NewRequest("GET", name, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "export function") || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
			t.Errorf("%s is served as %d %q", name, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

const limited = `flow: limited
stages:
  find: { job: ./w.js }
  tell:
    job: { script: ./w.js, input: "${stages.find.output}", timeout: 5s, tags: [gpu], allow: [network], restart: { limit: 2 in 1h, delay: 2s } }
  after: { job: ./w.js, follows: [tell] }
  sum: { each: { emitted: find }, job: ./w.js }
`

// A card gives where its file writes it, as file:line: a stage, a state,
// a state inside another, a tool, a tool of a toolkit, the ask and the
// model.
func Test_drawing_card_gives_its_line(t *testing.T) {
	flow := scene(t, definition(t, limited, given), nil)
	lines := map[string]string{}
	for _, st := range flow.Flow.Stages {
		lines[st.Card.Name] = st.Card.Line
	}
	if want := map[string]string{"find": "3", "tell": "5", "after": "6", "sum": "7"}; !maps.Equal(lines, want) {
		t.Errorf("the stages are written at %v, not %v", lines, want)
	}
	// A card whose values are in two files, as where a value is included,
	// gives the first line in the file most of them are in.
	split := map[string]string{"stages.a.when": "base.yaml:1", "stages.a.job.script": "inc.yaml:5", "stages.a.job.timeout": "inc.yaml:6", "stages.b.job": "base.yaml:9"}
	if got := lineOf(split, "stages.a"); got != "inc.yaml:5" {
		t.Errorf("a card written across two files gives the line %q, not inc.yaml:5", got)
	}
	if got := lineOf(map[string]string{"stages.a.when": "b.yaml:7", "stages.a.job": "a.yaml:9"}, "stages.a"); got != "a.yaml:9" {
		t.Errorf("a card with as many values in two files gives the line %q, not that of the file named first, a.yaml:9", got)
	}
	chart := scene(t, definition(t, held, given), nil)
	lines = map[string]string{}
	for _, st := range chart.Chart.States {
		lines[st.Name] = st.Card.Line
	}
	if want := map[string]string{"idle": "5", "busy": "8", "one": "12", "two": "14"}; !maps.Equal(lines, want) {
		t.Errorf("the states are written at %v, not %v", lines, want)
	}
	agent := scene(t, definition(t, helper, given), nil)
	lines = map[string]string{"model": agent.Agent.ModelLine, "ask": agent.Agent.Ask.Line}
	for _, g := range agent.Agent.Toolkits {
		for _, tool := range g.Tools {
			lines[tool.Card.Name] = tool.Card.Line
		}
	}
	if want := map[string]string{"model": "2", "ask": "5", "look": "7", "idle": "8", "tell": "13"}; !maps.Equal(lines, want) {
		t.Errorf("the agent's cards are written at %v, not %v", lines, want)
	}
}

// A card's tooltip gives its job's limits, with lengths of time as a
// definition writes them, and a flow gives why each stage waits for
// another: what it reads, follows: or each: { emitted: … }.
func Test_drawing_tooltips_give_limits_and_why_a_stage_waits(t *testing.T) {
	s := scene(t, definition(t, limited, given), nil)
	stages := map[string]Stage{}
	for _, st := range s.Flow.Stages {
		stages[st.Card.Name] = st
	}
	if got, want := stages["tell"].Card.Title, "timeout: 5s\nrestart: { after: failure, limit: 2 in 1h, delay: 2s }\ntags: gpu\nallow: network"; got != want {
		t.Errorf("the tooltip of a job with limits is %q, not %q", got, want)
	}
	if got := stages["find"].Card.Title; got != "" {
		t.Errorf("the tooltip of a job with no limits is %q", got)
	}
	why := map[string]string{"tell": stages["tell"].Why["find"], "after": stages["after"].Why["tell"], "sum": stages["sum"].Why["find"]}
	if want := map[string]string{"tell": "reads find.output", "after": "follows: tell", "sum": "each: { emitted: find }"}; !maps.Equal(why, want) {
		t.Errorf("the stages wait %v, not %v", why, want)
	}
}
