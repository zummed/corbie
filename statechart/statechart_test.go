package statechart

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
)

var entered = time.Date(2026, 10, 13, 17, 0, 0, 0, time.UTC) // a Tuesday

// chart is a chart under test, stepped by hand.
type chart struct {
	t     *testing.T
	def   *machine.Definition
	state jsontext.Value
	step  int
	at    time.Time
}

func parse(t *testing.T, src string) (*machine.Definition, error) {
	draft, err := machine.Parse([]byte(strings.ReplaceAll(src, "\t", "  ")), func(path string) ([]byte, error) {
		return []byte("return 1"), nil
	})
	if err != nil {
		return nil, err
	}
	return draft.Definition, nil
}

func newChart(t *testing.T, src string) *chart {
	t.Helper()
	def, err := parse(t, src)
	if err != nil {
		t.Fatalf("the chart does not pass:\n%v", err)
	}
	return &chart{t: t, def: def, at: entered}
}

// give takes one step with the events and tells what the chart asked for,
// one word for each request.
func (c *chart) give(events ...machine.Event) []string {
	c.t.Helper()
	c.step++
	out, err := chartType{}.Step(&machine.StepIn{Definition: c.def, State: c.state, Events: events,
		Input: map[string]any{"n": 1.0}, Time: c.at, Seed: []byte(fmt.Sprint("seed", c.step))})
	if err != nil {
		c.t.Fatal(err)
	}
	c.state, _ = json.Marshal(out.State)
	var told []string
	for _, r := range out.Requests {
		switch {
		case r.Job != nil:
			told = append(told, "job "+r.Job.Name)
		case r.Cancel != "":
			told = append(told, "cancel "+r.Cancel)
		case r.Send != nil:
			told = append(told, fmt.Sprintf("send %s to %s", r.Send.Event, r.Send.To))
		case r.Timer != nil:
			told = append(told, "timer "+r.Timer.Name)
		case r.Clear != "":
			told = append(told, "clear "+r.Clear)
		case r.Key != nil:
			told = append(told, "key "+r.Key.ID)
		case r.Drop != "":
			told = append(told, "drop "+r.Drop)
		case r.End != nil && r.End.Error != nil:
			told = append(told, "fail")
		case r.End != nil:
			told = append(told, fmt.Sprintf("end %v", r.End.Output))
		}
	}
	for _, i := range out.Ignored {
		told = append(told, "ignored "+events[i].Name)
	}
	return told
}

func (c *chart) active() []string {
	var st state
	json.Unmarshal(c.state, &st)
	return st.Active
}

func (c *chart) want(got []string, want ...string) {
	c.t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		c.t.Fatalf("the chart asked for\n  %q\nwant\n  %q", got, want)
	}
}

func start() machine.Event { return machine.Event{Name: machine.Start} }
func done(job string, v any) machine.Event {
	return machine.Event{Name: machine.Done, From: job, Data: v}
}
func failed(job string) machine.Event {
	return machine.Event{Name: machine.Failed, From: job, Data: map[string]any{"status": "error", "error": "it broke"}}
}

const nested = `
statechart: nested
initial: outer
states:
	outer:
		initial: inner
		failed: recovering
		after: { 10m: gave_up }
		states:
			inner:
				job: ./work.js
				done: inner_final
				on: { deeper: second }
			second:
				job: ./work.js
			inner_final: { final: true, output: "${states.inner.output}" }
		done: finished
	recovering:
		job: ./work.js
		done: finished
	finished: { final: true, output: { from: "${event.from}", was: "${event.data}" } }
	gave_up: { final: true, failed: true, output: late }
`

func Test_chart_finishes_innermost_first_and_once(t *testing.T) {
	c := newChart(t, nested)
	c.want(c.give(start()), "timer 0.outer", "job inner")
	// The job's finish enters the final state inside, which finishes the
	// holder, whose done: ends the chart: each once, innermost first.
	c.want(c.give(done("inner", 7.0)), "cancel inner", "clear 0.outer", "end map[from:outer was:7]")
}

func Test_chart_move_inward_keeps_holder(t *testing.T) {
	c := newChart(t, strings.Replace(nested, "on: { deeper: second }", "", 1)+"")
	def, err := parse(t, strings.Replace(nested, "failed: recovering", "failed: recovering\n\t\ton: { deeper: second }", 1))
	if err != nil {
		t.Fatal(err)
	}
	c.def = def
	c.give(start())
	// A move from the holder's rule to a state inside it leaves what was
	// active inside and does not leave the holder: its timer stands.
	c.want(c.give(machine.Event{Name: "deeper"}), "cancel inner", "job second")
	if got := c.active(); !reflect.DeepEqual(got, []string{"outer", "second"}) {
		t.Fatalf("the chart is in %v", got)
	}
}

func TestFailureOfferedOutward(t *testing.T) {
	c := newChart(t, nested)
	c.give(start())
	// inner has no failed: rule; the state that holds it does.
	c.want(c.give(failed("inner")), "cancel inner", "clear 0.outer", "job recovering")
	// A failure nobody takes fails the chart.
	c.want(c.give(failed("recovering")), "fail")
}

func Test_chart_failed_final_fails_chart_from_any_depth(t *testing.T) {
	c := newChart(t, `
statechart: deep
initial: a
states:
	a:
		initial: b
		done: fine
		states:
			b:
				initial: c
				states:
					c: { on: { go: bad } }
					bad: { final: true, failed: true, output: { why: deep } }
	fine: { final: true }
`)
	c.give(start())
	c.want(c.give(machine.Event{Name: "go"}), "fail")
}

func Test_chart_sends_in_one_move_go_in_fixed_order(t *testing.T) {
	c := newChart(t, `
statechart: order
initial: a
states:
	a:
		initial: a1
		exit: { send: { to: parent, event: leave_a } }
		states:
			a1:
				exit: { send: [{ to: parent, event: leave_a1 }, { to: parent, event: leave_a1_again }] }
				on: { go: { to: b1, send: { to: parent, event: moving } } }
	b:
		initial: b1
		entry: { send: { to: parent, event: enter_b } }
		states:
			b1:
				entry: { send: { to: parent, event: enter_b1 } }
`)
	c.give(start())
	c.want(c.give(machine.Event{Name: "go"}),
		"send leave_a1 to parent", "send leave_a1_again to parent", "send leave_a to parent",
		"send moving to parent", "send enter_b to parent", "send enter_b1 to parent")
}

func Test_chart_entry_send_reads_own_key(t *testing.T) {
	c := newChart(t, `
statechart: keys
initial: outer
states:
	outer:
		wait: answer
		initial: inner
		entry: { send: { to: parent, event: outer_key, data: "${keys.answer}" } }
		on: { answer: outer_done }
		states:
			inner:
				wait: answer
				entry: { send: { to: parent, event: inner_key, data: "${keys.answer}" } }
				on: { answer: inner_done }
			inner_done: { final: true }
	outer_done: { final: true, output: outer }
`)
	c.step++
	out, err := chartType{}.Step(&machine.StepIn{Definition: c.def, Events: []machine.Event{start()}, Time: entered, Seed: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	c.state, _ = json.Marshal(out.State)
	keys := map[string]string{}
	sent := map[string]string{}
	for _, r := range out.Requests {
		if r.Key != nil {
			keys[r.Key.ID] = r.Key.Value
		}
		if r.Send != nil {
			sent[r.Send.Event], _ = r.Send.Data.(string)
		}
	}
	// Two states wait under one name; each has its own key, and an
	// expression that reads the key gets its own state's.
	if keys["outer"] == "" || keys["inner"] == "" || keys["outer"] == keys["inner"] ||
		sent["outer_key"] != keys["outer"] || sent["inner_key"] != keys["inner"] {
		t.Fatalf("keys %v, sent %v", keys, sent)
	}
	// An answer carrying the outer state's key is offered to the outer
	// state, and the inner state's rule is not taken.
	c.want(c.give(machine.Event{Name: "answer", From: "outer", Data: true}), "drop inner", "drop outer", "end outer")
}

func TestSeveralNamedJobs(t *testing.T) {
	c := newChart(t, `
statechart: both
initial: working
states:
	working:
		jobs:
			left: ./work.js
			right: ./work.js
		done: finished
		failed: broken
	finished: { final: true, output: "${states.working.output}" }
	broken: { final: true, output: "${event.from}" }
`)
	c.want(c.give(start()), "job left", "job right")
	// Done when all of them are done; the output is each one's by name.
	c.want(c.give(done("right", "r")))
	c.want(c.give(done("left", "l")), "cancel left", "cancel right", "end map[left:l right:r]")

	// Failed as soon as one fails; the others are cancelled as the state
	// is left.
	c = newChart(t, strings.Replace(`
statechart: both
initial: working
states:
	working:
		jobs:
			left: ./work.js
			right: ./work.js
		failed: broken
	broken: { final: true, output: "${event.from}" }
`, "", "", 0))
	c.give(start())
	c.want(c.give(failed("left")), "cancel left", "cancel right", "end left")
}

func Test_chart_event_not_offered_to_reentered_state(t *testing.T) {
	c := newChart(t, `
statechart: loop
initial: working
states:
	working:
		job: ./work.js
		done:
			- { to: finished, when: event.data >= 3 }
			- working
	finished: { final: true, output: "${states.working.output}" }
`)
	c.want(c.give(start()), "job working")
	// Two results arrive in one step: the first leaves the state and
	// enters it again, and the second, which was for the job that was
	// cancelled, is not given to the new one.
	c.want(c.give(done("working", 1.0), done("working", 5.0)), "cancel working", "job working")
	c.want(c.give(done("working", 3.0)), "cancel working", "end 3")
}

func TestChainThatDoesNotComeToRestFailsTheChart(t *testing.T) {
	c := newChart(t, `
statechart: spinning
initial: a
states:
	a:
		done: a
		states:
			only: { final: true }
`)
	c.want(c.give(start()), "fail")
}

func TestEventsNobodyTakes(t *testing.T) {
	c := newChart(t, nested)
	c.give(start())
	c.want(c.give(machine.Event{Name: "unknown", From: "client"}), "ignored unknown")
}

func refused(t *testing.T, src, want string) {
	t.Helper()
	_, err := parse(t, src)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want a refusal holding %q, got: %v", want, err)
	}
}

func TestChartsRefused(t *testing.T) {
	head := "statechart: bad\n"
	refused(t, head+"states:\n\ta: { final: true }\n\tb: { final: true }\n", "initial has to name")
	refused(t, head+"initial: a\nstates:\n\ta: { states: { x: { final: true } }, done: b }\n\tb: { states: { x: { final: true } } }\n", "two states named")
	refused(t, head+"initial: a\nstates:\n\ta: { job: ./w.js, states: { x: { final: true } } }\n", "has no job of its own")
	refused(t, head+"initial: a\nstates:\n\ta: { job: ./w.js, jobs: { p: ./w.js } }\n", "not both")
	refused(t, head+"initial: a\nstates:\n\ta: { on: { go: nowhere } }\n", "no state named")
	refused(t, head+"initial: a\nstates:\n\ta: { set: { x: 1 } }\n", "not a field that belongs here")
	refused(t, head+"initial: a\nstates:\n\ta: { entry: { send: { to: sibling, event: x } } }\n", "is not the parent or a child")
	refused(t, head+"initial: a\nstates:\n\ta: { job: ./w.js, entry: { send: { to: a, event: x } } }\n", "runs a script")
	refused(t, head+"initial: a\nstates:\n\ta: { job: { machine: { statechart: kid, states: { k: { final: true } } } }, exit: { send: { to: a, event: bye } } }\n", "on leaving a state cannot go to that state's own child")
	refused(t, head+"initial: ask\nstates:\n\task: { final: true }\n", "taken by the definition's ask")
	refused(t, head+"initial: a\nstates:\n\ta: { on: { go: { to: a, when: nothing.here } } }\n", `reads "nothing"`)
}

func Test_chart_same_time_written_twice_is_refused(t *testing.T) {
	head := "statechart: bad\ninitial: a\nstates:\n\tb: { final: true }\n\ta:\n\t\tafter:\n"
	refused(t, head+"\t\t\t1h: b\n\t\t\t60m: b\n", "are the same time")
	refused(t, head+"\t\t\t\"18:00\": b\n\t\t\t\"18:00 UTC\": b\n", "are the same time")
	refused(t, head+"\t\t\t\"25:00\": b\n", "not a length of time or a time of day")
}

func Test_chart_zone_known_without_host_zone_files(t *testing.T) {
	if _, err := parseAt("09:00 Europe/London"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseAt("09:00 Local"); err == nil {
		t.Fatal("a zone that means wherever the controller runs was taken")
	}
	if _, err := parseAt("09:00 Nowhere/Atall"); err == nil {
		t.Fatal("a zone that does not exist was taken")
	}
}

func due(t *testing.T, at string, from time.Time) time.Time {
	t.Helper()
	w, err := parseAt(at)
	if err != nil {
		t.Fatal(err)
	}
	return w.next(from)
}

func TestTimeOfDay(t *testing.T) {
	day := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, time.UTC) }
	// A length of time counts from entry.
	if got := due(t, "48h", entered); !got.Equal(entered.Add(48 * time.Hour)) {
		t.Fatalf("48h is due %v", got)
	}
	// A time of day is due at its next occurrence after entry.
	if got := due(t, "18:00", day(13, 17, 0)); !got.Equal(day(13, 18, 0)) {
		t.Fatalf("18:00 entered at 17:00 is due %v", got)
	}
	if got := due(t, "18:00", day(13, 19, 0)); !got.Equal(day(14, 18, 0)) {
		t.Fatalf("18:00 entered at 19:00 is due %v", got)
	}
	// A weekday: entered on a Tuesday, Monday at 09:00 is the following Monday.
	if got := due(t, "mon 09:00", day(13, 17, 0)); !got.Equal(day(19, 9, 0)) {
		t.Fatalf("Monday 09:00 entered on a Tuesday is due %v", got)
	}
	// A zone: 09:00 in London in July is 08:00 UTC.
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if got := due(t, "09:00 Europe/London", july); !got.Equal(july.Add(8 * time.Hour)) {
		t.Fatalf("09:00 London in July is due %v", got)
	}
}

// timer gives when a timer the chart holds is due, and the event that
// arrives then.
func (c *chart) timer(name string) (time.Time, machine.Event) {
	c.t.Helper()
	var st state
	json.Unmarshal(c.state, &st)
	at, ok := st.Timers[name]
	if !ok {
		c.t.Fatalf("the chart holds no timer %s", name)
	}
	return at, machine.Event{Name: machine.Timer, From: name, Data: at.UTC().Format(time.RFC3339Nano)}
}

// A length of time counts from the entry the timer was started by: a state
// left and entered again waits its whole time again.
func TestTimerOfAStateEnteredAgainCountsFromTheSecondEntry(t *testing.T) {
	c := newChart(t, `
statechart: patient
initial: waiting
states:
	waiting:
		after: { 48h: gave_up }
		on: { out: away }
	away:
		on: { back: waiting }
	gave_up: { final: true, output: late }
`)
	c.want(c.give(start()), "timer 0.waiting")
	first, old := c.timer("0.waiting")
	if !first.Equal(entered.Add(48 * time.Hour)) {
		t.Fatalf("entered at %v, the timer is due %v", entered, first)
	}
	c.at = entered.Add(time.Hour)
	c.want(c.give(machine.Event{Name: "out", From: "client"}), "clear 0.waiting")
	c.want(c.give(machine.Event{Name: "back", From: "client"}), "timer 0.waiting")
	second, again := c.timer("0.waiting")
	if !second.Equal(entered.Add(49 * time.Hour)) {
		t.Fatalf("entered again an hour later, the timer is due %v, want 48 hours after the second entry", second)
	}
	// The first entry's timer, come due, is not this entry's.
	c.at = first
	c.want(c.give(old))
	c.at = second
	c.want(c.give(again), "end late")
}

// A recurring job is a chart that waits with after: and starts its work as
// a child. Over three days it starts three, each at nine.
func TestRecurringJobStartsItsWorkAtItsTimeEachDay(t *testing.T) {
	c := newChart(t, `
statechart: nightly
initial: waiting
states:
	waiting:
		after: { "09:00": working }
	working:
		job: { machine: { statechart: report, states: { written: { final: true } } } }
		done: waiting
`)
	c.want(c.give(start()), "timer 0.waiting")
	for day := 14; day <= 16; day++ {
		at, fires := c.timer("0.waiting")
		if want := time.Date(2026, 10, day, 9, 0, 0, 0, time.UTC); !at.Equal(want) {
			t.Fatalf("the job is next due %v, want %v", at, want)
		}
		c.at = at
		c.want(c.give(fires), "job working")
		c.at = at.Add(20 * time.Minute)
		c.want(c.give(done("working", nil)), "cancel working", "timer 0.waiting")
	}
}

func Test_chart_time_of_day_is_strictly_after_entry(t *testing.T) {
	at := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	if got := due(t, "18:00", at); !got.Equal(at.Add(24 * time.Hour)) {
		t.Fatalf("entered at exactly its time, it is due %v", got)
	}
}

func Test_chart_time_of_day_in_skipped_hour_fires_at_the_jump(t *testing.T) {
	// In London the clock goes from 01:00 to 02:00 on 29 March 2026.
	before := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	want := time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)
	if got := due(t, "01:30 Europe/London", before); !got.Equal(want) {
		t.Fatalf("a time the clock skips is due %v, want the jump at %v", got, want)
	}
}

func Test_chart_state_does_not_grow_with_use(t *testing.T) {
	c := newChart(t, `
statechart: standing
initial: ready
states:
	ready:
		on: { go: working }
	working:
		job: ./work.js
		done: ready
		failed: ready
`)
	c.give(start())
	size := 0
	for i := range 1000 {
		c.give(machine.Event{Name: "go", From: "client", Data: map[string]any{"n": float64(i)}})
		c.give(done("working", "the same size of output"))
		if i == 0 {
			size = len(c.state)
		}
	}
	if len(c.state) > size {
		t.Fatalf("after a thousand pieces of work the state is %d bytes; after the first it was %d", len(c.state), size)
	}
}

func TestShortFormsGiveOneFullForm(t *testing.T) {
	forms := []string{"done: waiting", "done: { to: waiting }", "done: [{ to: waiting }]"}
	var stored []string
	for _, form := range forms {
		def, err := parse(t, "statechart: forms\ninitial: a\nstates:\n\ta:\n\t\tjob: ./w.js\n\t\t"+form+"\n\twaiting: { final: true }\n")
		if err != nil {
			t.Fatal(err)
		}
		stored = append(stored, string(def.Body))
	}
	if stored[0] != stored[1] || stored[1] != stored[2] {
		t.Fatalf("three ways of writing one rule give different stored forms:\n%s", strings.Join(stored, "\n"))
	}
}

// A key's value is handed to a person, who writes it as it is after a
// command or into an address: it holds letters and digits and nothing a
// command line or an address reads as something else, whatever the seed.
func TestKeyValueIsLettersAndDigits(t *testing.T) {
	def, err := parse(t, "statechart: asks\ninitial: waiting\nstates:\n\twaiting:\n\t\twait: answer\n\t\ton: { answer: answered }\n\tanswered: { final: true }\n")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := range 2000 {
		out, err := chartType{}.Step(&machine.StepIn{Definition: def, Events: []machine.Event{start()}, Time: entered, Seed: []byte(fmt.Sprint("seed", i))})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range out.Requests {
			if r.Key == nil {
				continue
			}
			value := r.Key.Value
			plain := strings.IndexFunc(value, func(r rune) bool {
				return (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
			}) < 0
			if !plain || len(value) < 32 {
				t.Fatalf("with the seed %d the key's value is %q", i, value)
			}
			seen[value] = true
		}
	}
	if len(seen) != 2000 {
		t.Fatalf("2000 seeds gave %d different keys", len(seen))
	}
}
