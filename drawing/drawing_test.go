package drawing

import (
	"encoding/json/v2"
	"html/template"
	"math"
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

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
)

func definition(t *testing.T, src string, files map[string]string) *machine.Definition {
	t.Helper()
	draft, err := machine.Parse([]byte(src), machine.Given(files))
	if err != nil {
		t.Fatalf("the definition does not check: %v", err)
	}
	return draft.Definition
}

func draw(t *testing.T, def *machine.Definition, run *Run) string {
	t.Helper()
	svg, err := drawn(def, run)
	if err != nil {
		t.Fatal(err)
	}
	return string(svg)
}

// centre gives where a name is written on a drawing.
func centre[S ~string](t *testing.T, svg S, name string) (float64, float64) {
	t.Helper()
	m := regexp.MustCompile(`<text class="title" x="([0-9.]+)" y="([0-9.]+)" text-anchor="[a-z]+">` + regexp.QuoteMeta(name) + `</text>`).FindStringSubmatch(string(svg))
	if m == nil {
		t.Fatalf("%q is not drawn", name)
	}
	x, _ := strconv.ParseFloat(m[1], 64)
	y, _ := strconv.ParseFloat(m[2], 64)
	return x, y
}

// states gives a statechart's state at a step: the states it is in.
func states(names ...string) machine.Shown { return machine.Shown{States: names} }

// group gives the class of the shape that holds a name.
func group(t *testing.T, svg string, name string) string {
	t.Helper()
	m := regexp.MustCompile(`<g class="([^"]*)">(?:<rect[^>]*/>)+<text[^>]*>` + regexp.QuoteMeta(name) + `</text>`).FindStringSubmatch(svg)
	if m == nil {
		t.Fatalf("%q is not drawn as a shape", name)
	}
	return m[1]
}

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

func Test_drawing_flow_stage_follows_what_it_waits_for(t *testing.T) {
	def := definition(t, `flow: fan
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
`, map[string]string{"write.js": script})
	svg, _ := drawn(def, nil)
	_, fetch := centre(t, svg, "fetch")
	_, sum := centre(t, svg, "sum")
	_, translate := centre(t, svg, "translate")
	_, send := centre(t, svg, "send")
	if !(fetch < sum && fetch < translate) {
		t.Errorf("sum at %v and translate at %v are not drawn after fetch at %v", sum, translate, fetch)
	}
	if !(sum < send && translate < send) {
		t.Errorf("send at %v is not drawn after sum at %v and translate at %v, which it waits for", send, sum, translate)
	}
	if sum != translate {
		t.Errorf("sum at %v and translate at %v run at the same time and are not drawn side by side", sum, translate)
	}
}

func Test_drawing_statechart_arrows_name_event_condition_and_timer(t *testing.T) {
	svg := draw(t, definition(t, review, map[string]string{"write.js": script}), nil)
	for _, label := range []string{">done<", ">failed<", ">verdict, when eve…<", ">too_long, 48h<"} {
		if !regexp.MustCompile(`<text class="on"[^>]*` + label).MatchString(svg) {
			t.Errorf("no arrow is labelled %s", label)
		}
	}
	if !strings.Contains(svg, `<text class="note"`) || !strings.Contains(svg, ">verdict ↩ drafting<") {
		t.Errorf("the transition back to an earlier state is not written in its box:\n%s", svg)
	}
	if !strings.Contains(svg, "<title>verdict, when event.data.ok</title>") {
		t.Errorf("a chip's whole label is not its tooltip:\n%s", svg)
	}
	if strings.Count(svg, `class="edge`) != 5 {
		t.Errorf("%d arrows for four forward transitions and the start:\n%s", strings.Count(svg, `class="edge`), svg)
	}
	if strings.Count(svg, `class="edge timer"`) != 1 {
		t.Errorf("the timer's arrow is not the one dashed arrow:\n%s", svg)
	}
	if !strings.Contains(svg, ">ticket verdict<") || !strings.Contains(svg, ">timer too_long<") {
		t.Errorf("the ticket and timer of waiting are not written in it")
	}
	if c := group(t, svg, "gave_up"); c != "state final failed" {
		t.Errorf("gave_up ends failed and is drawn as %q", c)
	}
	if strings.Count(svg, `class="tile`) != 1 {
		t.Errorf("a state with no job has an icon: only drafting, whose job is a script, has one:\n%s", svg)
	}

	// States in different holders go by the rows of their holders: w3 is
	// drawn above report and first above deep, so those are arrows, and
	// deep's rule back to first is written in deep.
	levels, err := os.ReadFile("testdata/levels.yaml")
	if err != nil {
		t.Fatal(err)
	}
	svg = draw(t, definition(t, string(levels), nil), nil)
	if !strings.Contains(svg, ">again ↩ first<") || strings.Contains(svg, "↩ report<") || strings.Contains(svg, "↩ deep<") {
		t.Errorf("the rules back are not the rules to a row above:\n%s", svg)
	}
	for _, label := range []string{">ok<", ">done<", ">out<", ">quick<", ">bad<"} {
		if !strings.Contains(svg, label) {
			t.Errorf("no arrow is labelled %s", label)
		}
	}
}

func Test_drawing_flow_has_one_arrow_for_each_wait_that_says_something(t *testing.T) {
	def := definition(t, `flow: chain
stages:
  a: { job: ./w.js }
  b: { job: ./w.js, follows: [a] }
  c: { job: ./w.js, follows: [a, b] }
`, map[string]string{"w.js": script})
	svg := draw(t, def, nil)
	if n := strings.Count(svg, `class="edge"`); n != 2 {
		t.Errorf("%d arrows: c waits for a through b, and needs none from a:\n%s", n, svg)
	}
}

func Test_drawing_chips_of_arrows_into_one_state_are_apart(t *testing.T) {
	chart := definition(t, `statechart: two
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
`, nil)
	svg := draw(t, chart, nil)
	at := func(label string) string {
		m := regexp.MustCompile(`<text class="on" x="([0-9.]+)"[^>]*>` + label + `<`).FindStringSubmatch(svg)
		if m == nil {
			t.Fatalf("no chip says %s", label)
		}
		return m[1]
	}
	if at("ok") == at("fine") {
		t.Errorf("two chips are at x %s:\n%s", at("ok"), svg)
	}
}

// Every arrow of a statechart has a line of its own, from the card it
// leaves to the card it goes to, behind no card, and each chip is clear of
// other chips and cards, and 12 from other arrows. Arrows from one card
// share the line they leave it by.
func Test_drawing_arrows_and_chips_are_apart(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.yaml")
	sources := map[string]string{"review": review}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		sources[filepath.Base(f)] = string(b)
	}
	if len(sources) < 8 {
		t.Fatalf("the examples in testdata are not there")
	}
	for name, src := range sources {
		svg := draw(t, definition(t, src, map[string]string{"w.js": script, "write.js": script}), nil)
		lines, chips, cards, holders := geometry(t, svg)
		if len(chips) != len(lines) {
			t.Errorf("%s: %d chips for %d arrows", name, len(chips), len(lines))
		}
		for i, a := range lines {
			if !onBorder(a[0].p, append(cards, holders...)) || !onBorder(a[len(a)-1].q, append(cards, holders...)) {
				t.Errorf("%s: the arrow %v does not run from one card to another", name, a)
			}
			for _, s := range a {
				for _, c := range cards {
					if passes(c, s, -1) {
						t.Errorf("%s: the arrow %v runs behind the card at %v", name, a, c)
					}
				}
			}
			for _, b := range lines[i+1:] {
				if a[0].p == b[0].p {
					continue
				}
				for _, s := range a {
					for _, o := range b {
						if overlap(s, o) {
							t.Errorf("%s: the arrows %v and %v run on one line", name, a, b)
						}
					}
				}
			}
		}
		for i, c := range chips {
			for _, o := range chips[i+1:] {
				if c.x < o.x+o.w && o.x < c.x+c.w && c.y < o.y+o.h && o.y < c.y+c.h {
					t.Errorf("%s: the chips at %v and %v overlap", name, c, o)
				}
			}
			for _, b := range cards {
				if c.x < b.x+b.w && b.x < c.x+c.w && c.y < b.y+b.h && b.y < c.y+c.h {
					t.Errorf("%s: the chip at %v is on the card at %v", name, c, b)
				}
			}
			mid := point{c.x + c.w/2, c.y + c.h/2}
			for j, a := range lines {
				for _, s := range a {
					if j == i && passes(c, s, 0) && !on(mid, s) {
						t.Errorf("%s: the chip at %v is on its arrow %v twice", name, c, a)
					}
					if j != i && passes(c, s, 11.5) {
						t.Errorf("%s: the chip at %v is within 12 of the arrow %v", name, c, a)
					}
				}
			}
		}
	}
}

type point struct{ x, y float64 }

// A segment is a straight part of an arrow, across or down.
type segment struct{ p, q point }

type rect struct{ x, y, w, h float64 }

// geometry reads a drawing's arrows, each as its segments, and its
// chips, cards and holders.
func geometry(t *testing.T, svg string) ([][]segment, []rect, []rect, []rect) {
	t.Helper()
	f := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	var lines [][]segment
	for _, m := range regexp.MustCompile(`(<circle class="start"[^>]*/>)?<path class="edge[^"]*" d="([^"]*)"`).FindAllStringSubmatch(svg, -1) {
		if m[1] != "" {
			continue // the short arrow into the state a level starts in
		}
		var line []segment
		var at point
		for _, c := range regexp.MustCompile(`([MHVQ])([-0-9. ]+)`).FindAllStringSubmatch(m[2], -1) {
			v := strings.Fields(c[2])
			switch c[1] {
			case "M":
				at = point{f(v[0]), f(v[1])}
			case "H":
				line = append(line, segment{at, point{f(v[0]), at.y}})
				at = point{f(v[0]), at.y}
			case "V":
				line = append(line, segment{at, point{at.x, f(v[0])}})
				at = point{at.x, f(v[0])}
			case "Q":
				at = point{f(v[2]), f(v[3])}
			}
		}
		lines = append(lines, line)
	}
	rects := func(re string) []rect {
		var out []rect
		for _, m := range regexp.MustCompile(re+` x="([-0-9.]+)" y="([-0-9.]+)" width="([0-9.]+)" height="([0-9.]+)"`).FindAllStringSubmatch(svg, -1) {
			out = append(out, rect{f(m[1]), f(m[2]), f(m[3]), f(m[4])})
		}
		return out
	}
	var cards []rect
	for _, r := range rects(`<rect class="(?:box|outer)"`) {
		if !slices.ContainsFunc(cards, func(o rect) bool { return r.x >= o.x && r.y >= o.y && r.x+r.w <= o.x+o.w && r.y+r.h <= o.y+o.h }) {
			cards = append(cards, r)
		}
	}
	return lines, rects(`<rect class="chip[^"]*"`), cards, rects(`holder"><rect`)
}

// passes says a segment goes through the inside of a box, grown by grow.
func passes(b rect, s segment, grow float64) bool {
	x0, y0, x1, y1 := b.x-grow, b.y-grow, b.x+b.w+grow, b.y+b.h+grow
	if s.p.y == s.q.y {
		return s.p.y > y0 && s.p.y < y1 && max(s.p.x, s.q.x) > x0 && min(s.p.x, s.q.x) < x1
	}
	return s.p.x > x0 && s.p.x < x1 && max(s.p.y, s.q.y) > y0 && min(s.p.y, s.q.y) < y1
}

// overlap says two segments run along one line for more than a
// point.
func overlap(a, b segment) bool {
	if a.p.y == a.q.y && b.p.y == b.q.y && math.Abs(a.p.y-b.p.y) < 1 {
		return min(max(a.p.x, a.q.x), max(b.p.x, b.q.x))-max(min(a.p.x, a.q.x), min(b.p.x, b.q.x)) > 1
	}
	if a.p.x == a.q.x && b.p.x == b.q.x && math.Abs(a.p.x-b.p.x) < 1 {
		return min(max(a.p.y, a.q.y), max(b.p.y, b.q.y))-max(min(a.p.y, a.q.y), min(b.p.y, b.q.y)) > 1
	}
	return false
}

// on says a point is on a segment.
func on(p point, s segment) bool {
	return p.x >= min(s.p.x, s.q.x)-0.5 && p.x <= max(s.p.x, s.q.x)+0.5 && p.y >= min(s.p.y, s.q.y)-0.5 && p.y <= max(s.p.y, s.q.y)+0.5
}

// onBorder says a point is on the edge of one of the boxes.
func onBorder(p point, boxes []rect) bool {
	return slices.ContainsFunc(boxes, func(b rect) bool {
		in := p.x >= b.x-0.5 && p.x <= b.x+b.w+0.5 && p.y >= b.y-0.5 && p.y <= b.y+b.h+0.5
		edge := math.Abs(p.x-b.x) < 0.5 || math.Abs(p.x-b.x-b.w) < 0.5 || math.Abs(p.y-b.y) < 0.5 || math.Abs(p.y-b.y-b.h) < 0.5
		return in && edge
	})
}

func Test_drawing_holder_makes_room_for_its_notes(t *testing.T) {
	chart := definition(t, `statechart: held
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
`, nil)
	svg := draw(t, chart, nil)
	last := 0.0
	for _, m := range regexp.MustCompile(`<text class="note" x="[0-9.]+" y="([0-9.]+)"`).FindAllStringSubmatch(svg, -1) {
		y, _ := strconv.ParseFloat(m[1], 64)
		last = max(last, y)
	}
	_, one := centre(t, template.HTML(svg), "one")
	if last == 0 || last+20 >= one {
		t.Errorf("the last note at %v overlaps the state inside at %v:\n%s", last, one, svg)
	}
	holder := regexp.MustCompile(`<g class="state holder"><rect x="[0-9.]+" y="([0-9.]+)" width="[0-9.]+" height="([0-9.]+)"`).FindStringSubmatch(svg)
	if holder == nil {
		t.Fatalf("busy is not drawn around its states:\n%s", svg)
	}
	top, _ := strconv.ParseFloat(holder[1], 64)
	high, _ := strconv.ParseFloat(holder[2], 64)
	_, two := centre(t, template.HTML(svg), "two")
	if top+high < two+16 {
		t.Errorf("the state inside at %v is below the holder's bottom at %v:\n%s", two, top+high, svg)
	}
}

func Test_drawing_of_a_big_machine_is_quick(t *testing.T) {
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
	// The graph is read in Go, and laid out here by goja, which runs the
	// layout about 70 times slower than a browser does: the chart takes
	// about 150ms in Chromium.
	for _, src := range []string{flow.String(), chart.String()} {
		def := definition(t, src, map[string]string{"w.js": script})
		began := time.Now()
		s, err := Draw(def, nil)
		if err != nil {
			t.Fatal(err)
		}
		if took := time.Since(began); took > time.Second {
			t.Errorf("%s took %v to read", def.Name, took)
		}
		began = time.Now()
		if _, err := laid(s); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(began); took > 30*time.Second {
			t.Errorf("%s took %v to lay out", def.Name, took)
		}
	}
}

func Test_drawing_run_is_marked_on_the_drawing(t *testing.T) {
	chart := definition(t, review, map[string]string{"write.js": script})
	svg := draw(t, chart, &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting"), Events: []string{"done"}}}})
	for name, want := range map[string]string{"waiting": "state now", "drafting": "state done", "approved": "state never final"} {
		if got := group(t, svg, name); got != want {
			t.Errorf("%s is drawn as %q, not %q", name, got, want)
		}
	}
	ended := draw(t, chart, &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting")}, {Shown: states("approved")}}, Ended: true})
	if got := group(t, ended, "approved"); got != "state done final" {
		t.Errorf("the state a run ended in is drawn as %q", got)
	}

	flow := definition(t, "flow: two\nstages:\n  a: { job: ./w.js }\n  b: { job: ./w.js, follows: [a] }\n  c: { job: ./w.js, follows: [b] }\n", map[string]string{"w.js": script})
	svg = draw(t, flow, &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"a": "done", "b": "running", "c": "waiting"}}}}})
	for name, want := range map[string]string{"a": "state stage done", "b": "state stage now", "c": "state stage never"} {
		if got := group(t, svg, name); got != want {
			t.Errorf("stage %s is drawn as %q, not %q", name, got, want)
		}
	}

	librarian := definition(t, `agent: librarian
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
tools:
  shelf: { description: a shelf, args: { shelf: string }, job: ./w.js }
  helper: { description: a helper, args: { topic: string }, job: { machine: ./two.yaml } }
`, map[string]string{"w.js": script, "two.yaml": "flow: two\nstages:\n  a: { job: ./w.js }\n"})
	svg = draw(t, librarian, &Run{Steps: []Step{{Shown: machine.Shown{Turn: 2}, Asked: []string{"shelf"}}}, Going: []string{"shelf"}})
	if got := group(t, svg, "shelf"); got != "state tool now" {
		t.Errorf("the tool the agent waits on is drawn as %q", got)
	}
	if got := group(t, svg, "helper"); got != "state tool never" {
		t.Errorf("a tool not in use is drawn as %q", got)
	}
	if !strings.Contains(svg, ">turn 2<") || !strings.Contains(svg, `class="tag" x=`) {
		t.Errorf("the agent's turn is not written beside its model")
	}
	if !strings.Contains(svg, ">machine: ./two.yaml<") {
		t.Errorf("the tool whose job is a machine does not say which")
	}

	// A state passed through within one step was entered: the run started
	// in drafting and left it in its first step.
	svg = draw(t, chart, &Run{Steps: []Step{{Shown: states("waiting"), Events: []string{"start", "done"}}}})
	if got := group(t, svg, "drafting"); got != "state done" {
		t.Errorf("the state the run started in and left in its first step is drawn as %q", got)
	}

	// An agent waiting on the ask: the ask is going, the tool of a toolkit
	// it holds is held, and a tool used at an earlier step is done.
	helper := definition(t, `agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
ask: { machine: ./approval.yaml }
tools:
  look: { description: looks, job: ./w.js }
  idle: { description: idles, job: ./w.js }
toolkits:
  messages:
    description: messages
    tools:
      tell: { description: tells someone, job: { script: ./w.js, approve: true } }
`, map[string]string{"w.js": script, "approval.yaml": "flow: approval\nstages:\n  a: { job: ./w.js }\n"})
	svg = draw(t, helper, &Run{
		Steps: []Step{{Asked: []string{"look"}}, {Shown: machine.Shown{Turn: 2}, Asked: []string{"ask"}}},
		Going: []string{"tell", "ask"}, Held: []string{"tell"},
		Children: map[string]Child{"ask": {ID: "a-client/2", Machine: "approval", Type: "flow"}},
	})
	for name, want := range map[string]string{"tell": "state tool held", "look": "state tool done", "idle": "state tool never"} {
		if got := group(t, svg, name); got != want {
			t.Errorf("%s is drawn as %q, not %q", name, got, want)
		}
	}
	asks := regexp.MustCompile(`<g class="([^"]*)">(?:<rect[^>]*/>)+<text[^>]*>ask</text><text class="sub"[^>]*>([^<]*)</text>`).FindAllStringSubmatch(svg, -1)
	var got []string
	for _, a := range asks {
		got = append(got, a[1]+": "+a[2])
	}
	if strings.Join(got, "; ") != "state ask now: flow: approval" {
		t.Errorf("the agent's ask is drawn as %q", got)
	}
	if strings.Count(svg, `href="/run/a-client/2"`) != 1 {
		t.Errorf("the run the ask started is linked from another card:\n%s", svg)
	}

	// A run that ended: a flow's stage still running is marked by how the run
	// ended, and an agent whose turn failed has its model marked failed.
	svg = draw(t, flow, &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"a": "done", "b": "running", "c": "waiting"}}}}, Ended: true})
	if got := group(t, svg, "b"); got != "state stage cancelled" {
		t.Errorf("the stage running when its run was cancelled is drawn as %q", got)
	}
	svg = draw(t, librarian, &Run{Steps: []Step{{Shown: machine.Shown{Turn: 1}, Asked: []string{"turn"}, Failed: []string{"turn"}}}, Ended: true, Failed: true})
	if got := group(t, svg, "claude-sonnet-5-5"); got != "state model failed" {
		t.Errorf("the model of an agent whose turn failed is drawn as %q", got)
	}
}

func Test_drawing_transition_just_taken_is_marked(t *testing.T) {
	chart := definition(t, review, map[string]string{"write.js": script})
	svg := draw(t, chart, &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting"), Events: []string{"done"}}}})
	taken := regexp.MustCompile(`class="edge taken"`).FindAllString(svg, -1)
	if len(taken) != 1 {
		t.Fatalf("%d arrows are marked as taken, not one", len(taken))
	}
	if !regexp.MustCompile(`<rect class="chip taken"[^>]*/><text class="on"[^>]*>done</text>`).MatchString(svg) || strings.Count(svg, `class="chip taken"`) != 1 {
		t.Errorf("the arrow marked as taken is not the one done goes by")
	}
}

func Test_drawing_transition_back_is_marked_in_its_card(t *testing.T) {
	chart := definition(t, review, map[string]string{"write.js": script})
	svg := draw(t, chart, &Run{Steps: []Step{{Shown: states("waiting")}, {Shown: states("drafting"), Events: []string{"verdict"}}}})
	if !strings.Contains(svg, `class="note taken"`) || strings.Contains(svg, `class="edge taken"`) {
		t.Errorf("the transition back is not marked in its card alone:\n%s", svg)
	}
}

func Test_drawing_statechart_send_is_a_card(t *testing.T) {
	chart := definition(t, `statechart: listen
initial: hearing
states:
  hearing:
    job: ./w.js
    on:
      message: { send: { to: { at: chat, key: "a" }, event: said } }
      again: hearing
      done: over
  over: { end: done }
`, map[string]string{"w.js": script})
	svg, _ := drawn(chart, nil)
	_, hearing := centre(t, svg, "hearing")
	_, box := centre(t, svg, "chat#a")
	if box <= hearing {
		t.Errorf("the send's box at %v is not below the state at %v", box, hearing)
	}
	if !strings.Contains(string(svg), `<path class="edge send"`) || !regexp.MustCompile(`<rect class="chip send"[^>]*/><text class="on"[^>]*>message</text>`).MatchString(string(svg)) {
		t.Errorf("the send's arrow is not dashed and labelled with the event:\n%s", svg)
	}
	if !strings.Contains(group(t, string(svg), "chat#a"), "sends") {
		t.Errorf("the send's box is not marked as one")
	}
	if !strings.Contains(string(svg), ">again ↻<") || strings.Contains(string(svg), `>again</text>`) {
		t.Errorf("a rule back to the same state is not written in its card alone:\n%s", svg)
	}
}

func Test_drawing_agent_is_a_loop_of_model_and_tools(t *testing.T) {
	def := definition(t, `agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
limits: { turns: 7 }
ask: { machine: ./approval.yaml }
tools:
  look: { description: looks at a thing, job: ./w.js }
toolkits:
  messages:
    description: messages
    tools:
      tell: { description: tells someone, job: { script: ./w.js, approve: true } }
`, map[string]string{"w.js": script, "approval.yaml": "flow: approval\nstages:\n  a: { job: ./w.js }\n"})
	svg, _ := drawn(def, nil)
	_, model := centre(t, svg, "claude-sonnet-5-5")
	_, tell := centre(t, svg, "tell")
	_, look := centre(t, svg, "look")
	_, ask := centre(t, svg, "ask")
	_, reply := centre(t, svg, "reply")
	if !(model < tell && model < look && tell < ask && ask < reply && look < reply) {
		t.Errorf("model %v, tools %v and %v, ask %v and reply %v are not drawn top to bottom", model, tell, look, ask, reply)
	}
	text := string(svg)
	for _, want := range []string{">messages<", ">tools<", ">7 turns<", ">approve: true<", ">machine: ./approval.yaml<", "<title>tells someone</title>"} {
		if !strings.Contains(text, want) {
			t.Errorf("the drawing does not hold %s", want)
		}
	}
	x := func(name string) float64 { v, _ := centre(t, svg, name); return v }
	if tools, toolkits := regexp.MustCompile(`class="name"[^>]*>tools<`).FindStringIndex(text), regexp.MustCompile(`class="name"[^>]*>messages<`).FindStringIndex(text); toolkits == nil || tools == nil {
		t.Errorf("the toolkit and the tools are not held in boxes of their own")
	}
	if x("tell") == x("look") {
		t.Errorf("the toolkit and the tools are not side by side")
	}
	if n := strings.Count(text, `class="edge`); n != 2 {
		t.Errorf("%d arrows, and an agent has one from its model to its tools and one to its reply:\n%s", n, text)
	}
	running, _ := drawn(def, &Run{Steps: []Step{{Shown: machine.Shown{Turn: 1}, Asked: []string{"blob_read"}}}, Going: []string{"blob_read"}})
	for _, svg := range []string{text, string(running)} {
		if strings.Contains(svg, "blob_read") || strings.Contains(svg, ">open<") {
			t.Errorf("a tool of Corbie's own is drawn, and no definition wrote it")
		}
	}

	// A toolkit included from a file gives that file in its tooltip: the
	// file its tools are written in, or its description: where it has none.
	def = definition(t, `agent: helper
model: { name: claude-sonnet-5-5, secret: KEY }
prompt: hello
toolkits:
  messages: { include: ./messages.yaml }
  notes: { include: ./notes.yaml }
`, map[string]string{"w.js": script,
		"messages.yaml": "description: messages\ntools:\n  tell: { description: tells, job: ./w.js }\n",
		"notes.yaml":    "description: notes\ntools: {}\n"})
	text = draw(t, def, nil)
	for _, want := range []string{">messages</text><title>messages.yaml</title>", ">notes</text><title>notes.yaml</title>"} {
		if !strings.Contains(text, want) {
			t.Errorf("a toolkit's tooltip does not give its file, %s:\n%s", want, text)
		}
	}
}

func Test_drawing_machine_unfolds_in_its_card(t *testing.T) {
	files := map[string]string{
		"w.js":       script,
		"sub/c.yaml": "flow: c\nstages:\n  deep: { job: w.js }\n",
		"b.yaml":     "flow: b\nstages:\n  inner: { job: { machine: ./sub/c.yaml } }\n",
	}
	src := `flow: a
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
	draft, err := machine.Parse([]byte(src), machine.Given(files))
	if err != nil {
		t.Fatal(err)
	}
	unfold := func(open ...string) string {
		u := Unfold{Open: map[string]bool{}, Href: func(path string) string { return "/go?to=" + path }}
		for _, p := range open {
			u.Open[p] = true
		}
		svg, err := unfolded(draft, u)
		if err != nil {
			t.Fatal(err)
		}
		return string(svg)
	}
	folded := unfold()
	if !strings.Contains(folded, `<a href="/go?to=outer">`) || !strings.Contains(folded, `<a href="/go?to=here">`) {
		t.Errorf("a box whose job names a machine is not a link:\n%s", folded)
	}
	if strings.Contains(folded, "/go?to=plain") || strings.Contains(folded, "/go?to=named") {
		t.Errorf("a script or a machine that is an expression has a link:\n%s", folded)
	}
	if strings.Contains(folded, ">inner<") {
		t.Errorf("a box that is not unfolded shows what is inside it")
	}
	if !strings.Contains(folded, ">flow: written<") {
		t.Errorf("a machine written in place does not say what it is:\n%s", folded)
	}
	one := unfold("outer")
	if !strings.Contains(one, ">inner<") || strings.Contains(one, ">deep<") {
		t.Errorf("one box unfolded does not show one level:\n%s", one)
	}
	if !strings.Contains(one, `<a href="/go?to=outer/inner">`) {
		t.Errorf("a box inside an unfolded box has no link by its path:\n%s", one)
	}
	two := unfold("outer", "outer/inner", "here")
	for _, want := range []string{">deep<", ">there<"} {
		if !strings.Contains(two, want) {
			t.Errorf("%s is not drawn when its boxes are unfolded", want)
		}
	}
	if strings.Contains(unfold("outer/inner"), ">deep<") {
		t.Errorf("a box inside a folded box is drawn")
	}
	if want := `height="`; !strings.Contains(two, want) {
		t.Fatal(two)
	}
	size := func(svg string) float64 {
		m := regexp.MustCompile(`height="([0-9.]+)"`).FindStringSubmatch(svg)
		v, _ := strconv.ParseFloat(m[1], 64)
		return v
	}
	if size(one) <= size(folded) {
		t.Errorf("an unfolded box does not make room for what it holds: %v and %v", size(one), size(folded))
	}
	if !strings.Contains(folded, ">flow: b<") {
		t.Errorf("a box whose job names a machine does not say its type and name:\n%s", folded)
	}

	// On a run's page, a card links to the run its job started, and says
	// what that run is, as on the Machines page.
	run := string(must(drawn(draft.Definition, &Run{Children: map[string]Child{"outer": {ID: "a-client/1", Machine: "b", Type: "flow"}}})))
	if !strings.Contains(run, `<a href="/run/a-client/1">`) || !strings.Contains(run, ">flow: b<") {
		t.Errorf("a card whose job started a run does not link to it:\n%s", run)
	}
	if strings.Contains(run, "/go?to=") || strings.Count(run, "<a href") != 1 {
		t.Errorf("a card whose job started no run has a link on a run's page:\n%s", run)
	}
	// A job that started no run yet says what its machine is, from the
	// definition the run was started from.
	run = string(must(drawn(draft.Definition, &Run{Machines: map[string]Machine{"./b.yaml": {Type: "flow", Name: "b"}}})))
	if !strings.Contains(run, ">flow: b<") || strings.Contains(run, "<a href") {
		t.Errorf("a card whose job started no run does not say its machine's type and name:\n%s", run)
	}
}

func must[S ~string](svg S, err error) S {
	if err != nil {
		panic(err)
	}
	return svg
}

func Test_drawing_unused_toolkit_is_folded(t *testing.T) {
	def := definition(t, `agent: helper
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
`, map[string]string{"w.js": script})
	svg := draw(t, def, &Run{Steps: []Step{{Asked: []string{"lights"}}, {Shown: machine.Shown{Turn: 1}, Asked: []string{"ping"}}}, Going: []string{"ping"}})
	for name, want := range map[string]string{"bins": "state holder toolkit folded", "servers": "state holder toolkit folded", "home": "state holder toolkit", "net": "state holder toolkit"} {
		if got := group(t, svg, name); got != want {
			t.Errorf("toolkit %s is drawn as %q, not %q", name, got, want)
		}
	}
	for _, hidden := range []string{">days</text>", ">put</text>", ">disk</text>"} {
		if strings.Contains(svg, hidden) {
			t.Errorf("a tool of a folded toolkit is drawn as a card: %s", hidden)
		}
	}
	if !strings.Contains(svg, ">2 tools<") || !strings.Contains(svg, "<title>days, put</title>") {
		t.Errorf("a folded toolkit does not give how many tools it has and name them")
	}
	at := func(toolkit string) (float64, float64) {
		m := regexp.MustCompile(`<rect x="([0-9.]+)" y="([0-9.]+)"[^>]*/><text class="name"[^>]*>` + toolkit + `<`).FindStringSubmatch(svg)
		if m == nil {
			t.Fatalf("toolkit %s has no holder", toolkit)
		}
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		return x, y
	}
	bx, by := at("bins")
	sx, sy := at("servers")
	hx, _ := at("home")
	nx, _ := at("net")
	lx, _ := centre(t, template.HTML(svg), "look")
	if bx != sx || sy <= by {
		t.Errorf("the folded toolkits are not one below another: bins at %v,%v and servers at %v,%v", bx, by, sx, sy)
	}
	if !(hx < nx && nx < lx && lx < bx) {
		t.Errorf("the folded toolkits are not after the others: home %v, net %v, look %v, folded %v", hx, nx, lx, bx)
	}

	page, _ := drawn(def, nil)
	if strings.Contains(string(page), "folded") || !strings.Contains(string(page), ">disk</text>") {
		t.Errorf("a drawing of no run folds a toolkit")
	}
}

func Test_drawing_call_and_send_jobs_are_cards(t *testing.T) {
	def := definition(t, `flow: trip
stages:
  weather: { job: { call: { at: services/weather }, input: { days: 2 } } }
  heat:
    job:
      send: { to: { at: devices/boiler }, event: set, data: { target: "${stages.weather.output.celsius}" } }
  room:
    job:
      send: { to: { at: rooms/hall, key: "${input.k}" }, event: set }
`, nil)
	svg := draw(t, def, nil)
	for _, want := range []string{">call: services/weather<", ">send: set<", ">to: devices/boiler<", ">to: rooms/hall#${input.k}<"} {
		if !strings.Contains(svg, want) {
			t.Errorf("the drawing does not hold %s:\n%s", want, svg)
		}
	}
	if _, weather := centre(t, template.HTML(svg), "weather"); func() bool { _, heat := centre(t, template.HTML(svg), "heat"); return heat <= weather }() {
		t.Errorf("a send whose data reads a stage is not drawn below it:\n%s", svg)
	}
	if strings.Contains(svg, "<a href") {
		t.Errorf("a call that started no run, or a send, has a link:\n%s", svg)
	}
	// On a run's page, a call's card links to the run it called.
	run := draw(t, def, &Run{Children: map[string]Child{"weather": {ID: "lab/7", Machine: "weather", Type: "flow"}}})
	if !strings.Contains(run, `<a href="/run/lab/7">`) || !strings.Contains(run, ">flow: weather<") || !strings.Contains(run, ">call: services/weather<") {
		t.Errorf("a call's card does not link to the run it called:\n%s", run)
	}
}

func Test_drawing_conditions_approvals_and_calls_are_marked(t *testing.T) {
	// A toolkit or a tool with when: has a dashed border and its condition
	// in its tooltip.
	def := definition(t, `agent: helper
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
`, map[string]string{"w.js": script, "orders.yaml": "description: orders\ntools:\n  read: { description: reads, job: ./w.js }\n"})
	svg := draw(t, def, nil)
	for _, want := range []string{
		`<g class="state holder toolkit when"><rect`,
		`>orders</text><title>orders.yaml` + "\n" + `when: input.asker.scopes.includes(&#39;orders&#39;)</title>`,
		`<g class="state holder toolkit"><rect`,
		"<title>looks\nwhen: input.look</title>",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("the drawing does not hold %q:\n%s", want, svg)
		}
	}
	if got := group(t, svg, "look"); got != "state tool when" {
		t.Errorf("a tool with when: is drawn as %q", got)
	}
	if got := group(t, svg, "jot"); got != "state tool" {
		t.Errorf("a tool with no when: is drawn as %q", got)
	}
	folded := draw(t, def, &Run{Steps: []Step{{Asked: []string{"jot"}}}})
	if !strings.Contains(folded, `<g class="state holder toolkit when folded">`) || !strings.Contains(folded, "<title>read\nwhen: input.asker") {
		t.Errorf("a folded toolkit with when: is not dashed with its condition:\n%s", folded)
	}

	// A job of any machine with approve: true says so, and a stage or a
	// state whose job is held for the ask is marked held.
	flow := definition(t, `flow: deploy
ask: { machine: ./approval.yaml }
stages:
  plan: { job: ./w.js }
  ship: { job: { script: ./w.js, approve: true }, follows: [plan] }
  pipeline: { job: { call: { at: ci/pipeline }, approve: true }, follows: [plan] }
  later: { job: { call: { at: "${input.at}" } }, follows: [plan] }
  far: { job: { call: { at: /work/ci/pipeline } }, follows: [plan] }
`, map[string]string{"w.js": script, "approval.yaml": "flow: approval\nstages:\n  a: { job: ./w.js }\n"})
	svg = draw(t, flow, &Run{Steps: []Step{{Shown: machine.Shown{Stages: map[string]string{"plan": "done", "ship": "running", "pipeline": "running"}}}}, Going: []string{"ship", "pipeline"}, Held: []string{"ship"}})
	if strings.Count(svg, ">approve: true<") != 2 {
		t.Errorf("the jobs with approve: true do not say so:\n%s", svg)
	}
	for name, want := range map[string]string{"ship": "state stage held", "pipeline": "state stage now"} {
		if got := group(t, svg, name); got != want {
			t.Errorf("stage %s is drawn as %q, not %q", name, got, want)
		}
	}
	chart := definition(t, `statechart: deploys
ask: { machine: ./approval.yaml }
initial: shipping
states:
  shipping:
    job: { script: ./w.js, approve: true }
    on: { done: over }
  over: { end: done }
`, map[string]string{"w.js": script, "approval.yaml": "flow: approval\nstages:\n  a: { job: ./w.js }\n"})
	svg = draw(t, chart, &Run{Steps: []Step{{Shown: states("shipping")}}, Going: []string{"shipping"}, Held: []string{"shipping"}})
	if got := group(t, svg, "shipping"); got != "state held" || !strings.Contains(svg, ">approve: true<") {
		t.Errorf("a state whose job is held is drawn as %q:\n%s", got, svg)
	}

	// On the Machines page a call's card links to the machine at the
	// location it writes out; one an expression gives has no link.
	draft, err := machine.Parse([]byte(`flow: deploy
stages:
  pipeline: { job: { call: { at: ci/pipeline } } }
  later: { job: { call: { at: "${input.at}" } } }
  gone: { job: { call: { at: ci/gone } } }
`), machine.Given(nil))
	if err != nil {
		t.Fatal(err)
	}
	var asked []string
	u := Unfold{Href: func(string) string { return "" }, Called: func(at string) string {
		asked = append(asked, at)
		if at == "ci/pipeline" {
			return "/machines?file=ci/pipeline.yaml"
		}
		return ""
	}}
	page := string(must(unfolded(draft, u)))
	if !strings.Contains(page, `<a href="/machines?file=ci/pipeline.yaml"><g class="state stage">`) || strings.Count(page, "<a href") != 1 {
		t.Errorf("a call's card does not link to the machine it calls:\n%s", page)
	}
	if slices.Contains(asked, "${input.at}") {
		t.Errorf("a location an expression gives is looked up: %v", asked)
	}
}

// A decision is its model above its answers, with each question's name.
func Test_drawing_decision_is_its_model_and_its_questions(t *testing.T) {
	svg := draw(t, definition(t, `decision: triage
input: { text: string }
model: { name: jev-latest, secret: TYPESAFE_KEY }
state: ${input.text}
questions:
  urgent: { type: noul, instructions: The customer cannot use the product. }
  team: { type: choice, instructions: Who handles it, criteria: { billing: money, support: help } }
`, nil), nil)
	for _, want := range []string{"jev-latest", "answers", "urgent", "team"} {
		if !strings.Contains(svg, ">"+want+"<") {
			t.Errorf("the drawing of a decision does not say %q:\n%s", want, svg)
		}
	}
}

// The graph a drawing is laid out from holds what each card says and how
// the run marks it, and no place on the drawing: the browser lays it out.
func Test_drawing_scene_is_the_graph_and_its_marks(t *testing.T) {
	s, err := Draw(definition(t, review, map[string]string{"write.js": script}), &Run{Steps: []Step{{Shown: states("drafting")}, {Shown: states("waiting"), Events: []string{"done"}}}})
	if err != nil {
		t.Fatal(err)
	}
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

func Test_drawing_files_are_served(t *testing.T) {
	for _, name := range []string{"/layout.js", "/drawing.js"} {
		w := httptest.NewRecorder()
		Files().ServeHTTP(w, httptest.NewRequest("GET", name, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "export function") || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
			t.Errorf("%s is served as %d %q", name, w.Code, w.Header().Get("Content-Type"))
		}
	}
}
