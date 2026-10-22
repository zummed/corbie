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
	input map[string]any
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
	input := c.input
	if input == nil {
		input = map[string]any{"n": 1.0}
	}
	out, err := chartType{}.Step(&machine.StepIn{Definition: c.def, State: c.state, Events: events,
		Input: input, Time: c.at, Seed: []byte(fmt.Sprint("seed", c.step))})
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
		case r.Ticket != nil:
			told = append(told, "ticket "+r.Ticket.ID)
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
		timers: { too_long: 10m }
		on:
		  done: finished
		  failed: recovering
		  too_long: gave_up
		states:
			inner:
				job: ./work.js
				on:
				  done: inner_final
				  deeper: second
			second:
				job: ./work.js
			inner_final: { end: done, output: "${states.inner.output}" }
	recovering:
		job: ./work.js
		on: { done: finished }
	finished: { end: done, output: { from: "${event.from}", was: "${event.data}" } }
	gave_up: { end: failed, output: late }
`

func Test_chart_finishes_innermost_first_and_once(t *testing.T) {
	c := newChart(t, nested)
	c.want(c.give(start()), "timer too_long.outer", "job inner")
	// The job's finish enters the final state inside, which finishes the
	// holder, whose done: ends the chart: each once, innermost first.
	c.want(c.give(done("inner", 7.0)), "cancel inner", "clear too_long.outer", "end map[from:outer was:7]")
}

func Test_chart_move_inward_keeps_holder(t *testing.T) {
	moved := strings.Replace(nested, "\n\t\t\t\t  deeper: second", "", 1)
	c := newChart(t, moved)
	def, err := parse(t, strings.Replace(moved, "failed: recovering", "failed: recovering\n\t\t  deeper: second", 1))
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
	c.want(c.give(failed("inner")), "cancel inner", "clear too_long.outer", "job recovering")
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
		on: { done: fine }
		states:
			b:
				initial: c
				states:
					c: { on: { next: bad } }
					bad: { end: failed, output: { why: deep } }
	fine: { end: done }
`)
	c.give(start())
	c.want(c.give(machine.Event{Name: "next"}), "fail")
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
				on: { next: { go: b1, send: { to: parent, event: moving } } }
	b:
		initial: b1
		entry: { send: { to: parent, event: enter_b } }
		states:
			b1:
				entry: { send: { to: parent, event: enter_b1 } }
`)
	c.give(start())
	c.want(c.give(machine.Event{Name: "next"}),
		"send leave_a1 to parent", "send leave_a1_again to parent", "send leave_a to parent",
		"send moving to parent", "send enter_b to parent", "send enter_b1 to parent")
}

func Test_chart_entry_send_reads_own_ticket(t *testing.T) {
	c := newChart(t, `
statechart: tickets
initial: outer
states:
	outer:
		ticket: withdraw
		initial: inner
		entry: { send: { to: parent, event: outer_ticket, data: "${tickets.withdraw}" } }
		on: { withdraw: outer_done }
		states:
			inner:
				ticket: answer
				entry: { send: { to: parent, event: inner_ticket, data: "${tickets.answer}" } }
				on: { answer: inner_done }
			inner_done: { end: done }
	outer_done: { end: done, output: outer }
`)
	c.step++
	out, err := chartType{}.Step(&machine.StepIn{Definition: c.def, Events: []machine.Event{start()}, Time: entered, Seed: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	c.state, _ = json.Marshal(out.State)
	tickets := map[string]string{}
	sent := map[string]string{}
	for _, r := range out.Requests {
		if r.Ticket != nil {
			tickets[r.Ticket.ID] = r.Ticket.Value
		}
		if r.Send != nil {
			sent[r.Send.Event], _ = r.Send.Data.(string)
		}
	}
	// Each state has its own ticket, which exists before its entry send
	// reads it.
	if tickets["outer"] == "" || tickets["inner"] == "" || tickets["outer"] == tickets["inner"] ||
		sent["outer_ticket"] != tickets["outer"] || sent["inner_ticket"] != tickets["inner"] {
		t.Fatalf("tickets %v, sent %v", tickets, sent)
	}
	// The outer state's answer is offered to the outer state.
	c.want(c.give(machine.Event{Name: "withdraw", From: "outer", Data: true, Answer: true}), "drop inner", "drop outer", "end outer")
}

// An event with a ticket's name is taken only as that ticket's answer, and
// one with a timer's name only as the timer being due: the same name from
// the client, the parent, or the state's own job, which has the state's
// name, finds no rule.
func TestEventWithATicketsOrATimersNameIsOnlyItsOwn(t *testing.T) {
	c := newChart(t, `
statechart: one_way
initial: waiting
states:
	waiting:
		ticket: verdict
		timers: { too_long: 1h }
		job: ./work.js
		on: { verdict: decided, too_long: decided }
	decided: { end: done }
`)
	c.give(start())
	for _, from := range []string{"client", "parent", "waiting"} {
		c.want(c.give(machine.Event{Name: "verdict", From: from, Data: true}), "ignored verdict")
		c.want(c.give(machine.Event{Name: "too_long", From: from, Data: true}), "ignored too_long")
	}
	c.want(c.give(machine.Event{Name: "verdict", From: "waiting", Data: true, Answer: true}),
		"cancel waiting", "clear too_long.waiting", "drop waiting", "end <nil>")
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
		on:
		  done: finished
		  failed: broken
	finished: { end: done, output: "${states.working.output}" }
	broken: { end: done, output: "${event.from}" }
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
		on: { failed: broken }
	broken: { end: done, output: "${event.from}" }
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
		on:
		  done:
		    - { go: finished, when: "${event.data >= 3}" }
		    - working
	finished: { end: done, output: "${states.working.output}" }
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
		on: { done: a }
		states:
			only: { end: done }
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
	refused(t, head+"states:\n\ta: { end: done }\n\tb: { end: done }\n", "initial has to name")
	refused(t, head+"initial: a\nstates:\n\ta: { states: { x: { end: done } }, on: { done: b } }\n\tb: { states: { x: { end: done } } }\n", "two states named")
	refused(t, head+"initial: a\nstates:\n\ta: { job: ./w.js, states: { x: { end: done } } }\n", "has no job of its own")
	refused(t, head+"initial: a\nstates:\n\ta: { job: ./w.js, jobs: { p: ./w.js } }\n", "not both")
	refused(t, head+"initial: a\nstates:\n\ta: { on: { next: nowhere } }\n", "no state named")
	refused(t, head+"initial: a\nstates:\n\ta: { set: { x: 1 } }\n", "not a field that belongs here")
	refused(t, head+"initial: a\nstates:\n\ta: { entry: { send: { to: sibling, event: x } } }\n", "is not the parent or a child")
	refused(t, head+"initial: a\nstates:\n\ta: { job: ./w.js, entry: { send: { to: a, event: x } } }\n", "runs a script")
	refused(t, head+"initial: a\nstates:\n\ta: { job: { machine: { statechart: kid, states: { k: { end: done } } } }, exit: { send: { to: a, event: bye } } }\n", "on leaving a state cannot go to that state's own child")
	refused(t, head+"initial: ask\nstates:\n\task: { end: done }\n", "taken by the definition's ask")
	refused(t, head+"initial: a\nstates:\n\ta: { on: { next: { go: a, when: '${nothing.here}' } } }\n", `reads "nothing"`)
	refused(t, head+"initial: a\nstates:\n\ta: { end: finished }\n", "ends as done or as failed")
	refused(t, head+"initial: a\nstates:\n\ta: { end: done, on: { x: a } }\n", "a final state only ends")
	refused(t, head+"initial: a\nstates:\n\ta: { output: 1 }\n", "only a final state gives an output")
	refused(t, head+"initial: a\nstates:\n\ta: { on: { timer: a } }\n", "cannot be named under on")
	refused(t, head+"initial: a\nstates:\n\ta: { ticket: x, on: { x: b } }\n\tb: { ticket: x, on: { x: a } }\n", "a ticket's name is used once")
	refused(t, head+"initial: a\nstates:\n\ta: { on: { x: b } }\n\tb: { ticket: x, on: { x: a } }\n", `the state "a" can never receive "x". "x" is the ticket of the state "b"`)
	refused(t, head+"initial: a\nstates:\n\ta: { on: { t: b } }\n\tb: { timers: { t: 1h }, on: { t: a } }\n", `the state "a" can never receive "t". "t" is a timer of the state "b"`)
}

func Test_chart_timers_refused(t *testing.T) {
	head := "statechart: bad\ninitial: a\nstates:\n\tb: { end: done }\n\ta:\n"
	refused(t, head+"\t\ttimers: { late: \"25:00\" }\n\t\ton: { late: b }\n", "not a length of time or a time of day")
	refused(t, head+"\t\ttimers: { late: 1h }\n", "nothing goes anywhere when the timer is due")
	refused(t, head+"\t\ttimers: { done: 1h }\n\t\ton: { done: b }\n", `cannot be named "done"`)
	refused(t, head+"\t\tticket: late\n\t\ttimers: { late: 1h }\n\t\ton: { late: b }\n", `"late" is the name of the ticket of the state "a", and a timer cannot take a ticket's name`)
	refused(t, head+"\t\ttimers: { late: 1h }\n\t\ton: { late: b }\n\tc: { ticket: late, on: { late: a } }\n", `"late" is the name of the ticket of the state "c", and a timer cannot take a ticket's name`)
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

// A timer's time may come from input. It is filled in when the state is
// entered, and a filled time that is not a length of time or a time of day
// fails the chart.
func TestTimerTimeComesFromInput(t *testing.T) {
	const src = `
statechart: reminder
initial: waiting
states:
	waiting:
		timers: { t: "${input.due}", z: "${input.due} Europe/London" }
		on: { t: done, z: done }
	done: { end: done }
`
	c := newChart(t, src)
	c.input = map[string]any{"due": "18:00"}
	c.give(start())
	if at, _ := c.timer("t.waiting"); !at.Equal(time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("18:00 from input is due %v", at)
	}
	if at, _ := c.timer("z.waiting"); !at.Equal(time.Date(2026, 10, 14, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("18:00 London from input, entered at 18:00 London, is due %v", at)
	}
	_, err := chartType{}.Step(&machine.StepIn{Definition: c.def, Events: []machine.Event{start()},
		Input: map[string]any{"due": "soon"}, Time: entered, Seed: []byte("seed")})
	if err == nil || !strings.Contains(err.Error(), "states.waiting.timers.t: ") || !strings.Contains(err.Error(), "not a length of time") {
		t.Fatalf("want the chart to fail naming the timer, got: %v", err)
	}
	refused(t, strings.Replace(src, "${input.due}", "${nothing.here}", 1), `reads "nothing"`)
}

func TestTimeOfDay(t *testing.T) {
	day := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, time.UTC) }
	// A length of time counts from entry.
	if got, _ := Next("48h", entered); !got.Equal(entered.Add(48 * time.Hour)) {
		t.Fatalf("48h is due %v", got)
	}
	// A time of day is due at its next occurrence after entry.
	if got, _ := Next("18:00", day(13, 17, 0)); !got.Equal(day(13, 18, 0)) {
		t.Fatalf("18:00 entered at 17:00 is due %v", got)
	}
	if got, _ := Next("18:00", day(13, 19, 0)); !got.Equal(day(14, 18, 0)) {
		t.Fatalf("18:00 entered at 19:00 is due %v", got)
	}
	// A weekday: entered on a Tuesday, Monday at 09:00 is the following Monday.
	if got, _ := Next("mon 09:00", day(13, 17, 0)); !got.Equal(day(19, 9, 0)) {
		t.Fatalf("Monday 09:00 entered on a Tuesday is due %v", got)
	}
	// A zone: 09:00 in London in July is 08:00 UTC.
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if got, _ := Next("09:00 Europe/London", july); !got.Equal(july.Add(8 * time.Hour)) {
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
		timers: { too_long: 48h }
		on:
		  too_long: gave_up
		  out: away
	away:
		on: { back: waiting }
	gave_up: { end: done, output: late }
`)
	c.want(c.give(start()), "timer too_long.waiting")
	first, old := c.timer("too_long.waiting")
	if !first.Equal(entered.Add(48 * time.Hour)) {
		t.Fatalf("entered at %v, the timer is due %v", entered, first)
	}
	c.at = entered.Add(time.Hour)
	c.want(c.give(machine.Event{Name: "out", From: "client"}), "clear too_long.waiting")
	c.want(c.give(machine.Event{Name: "back", From: "client"}), "timer too_long.waiting")
	second, again := c.timer("too_long.waiting")
	if !second.Equal(entered.Add(49 * time.Hour)) {
		t.Fatalf("entered again an hour later, the timer is due %v, want 48 hours after the second entry", second)
	}
	// The first entry's timer, come due, is not this entry's.
	c.at = first
	c.want(c.give(old))
	c.at = second
	c.want(c.give(again), "end late")
}

// A recurring job is a chart that waits on a timer and starts its work as
// a child. Over three days it starts three, each at nine.
func TestRecurringJobStartsItsWorkAtItsTimeEachDay(t *testing.T) {
	c := newChart(t, `
statechart: nightly
initial: waiting
states:
	waiting:
		timers: { morning: "09:00" }
		on: { morning: working }
	working:
		job: { machine: { statechart: report, states: { written: { end: done } } } }
		on: { done: waiting }
`)
	c.want(c.give(start()), "timer morning.waiting")
	for day := 14; day <= 16; day++ {
		at, fires := c.timer("morning.waiting")
		if want := time.Date(2026, 10, day, 9, 0, 0, 0, time.UTC); !at.Equal(want) {
			t.Fatalf("the job is next due %v, want %v", at, want)
		}
		c.at = at
		c.want(c.give(fires), "job working")
		c.at = at.Add(20 * time.Minute)
		c.want(c.give(done("working", nil)), "cancel working", "timer morning.waiting")
	}
}

func Test_chart_time_of_day_is_strictly_after_entry(t *testing.T) {
	at := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	if got, _ := Next("18:00", at); !got.Equal(at.Add(24 * time.Hour)) {
		t.Fatalf("entered at exactly its time, it is due %v", got)
	}
}

func Test_chart_time_of_day_in_skipped_hour_fires_at_the_jump(t *testing.T) {
	// In London the clock goes from 01:00 to 02:00 on 29 March 2026.
	before := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	want := time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)
	if got, _ := Next("01:30 Europe/London", before); !got.Equal(want) {
		t.Fatalf("a time the clock skips is due %v, want the jump at %v", got, want)
	}
}

func Test_chart_next_is_when_the_timer_is_due(t *testing.T) {
	c := newChart(t, `
statechart: waits
initial: waiting
states:
	waiting:
		timers: { a: 48h, b: "18:00", c: "mon 09:00", d: "09:00 Europe/London", e: "01:30 Europe/London" }
		on: { a: done, b: done, c: done, d: done, e: done }
	done: { end: done }
`)
	// The day before London's clock goes from 01:00 to 02:00.
	c.at = time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	c.give(start())
	for timer, at := range map[string]string{"a": "48h", "b": "18:00", "c": "mon 09:00", "d": "09:00 Europe/London", "e": "01:30 Europe/London"} {
		want, err := Next(at, c.at)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := c.timer(timer + ".waiting"); !got.Equal(want) {
			t.Errorf("%s: the chart's timer is due %v, and Next gives %v", at, got, want)
		}
	}
	if _, err := Next("soon", c.at); err == nil || !strings.Contains(err.Error(), "not a length of time or a time of day") {
		t.Fatalf("want a time that is not one refused as the chart refuses it, got: %v", err)
	}
}

func Test_chart_state_does_not_grow_with_use(t *testing.T) {
	c := newChart(t, `
statechart: standing
initial: ready
states:
	ready:
		on: { work: working }
	working:
		job: ./work.js
		on:
		  done: ready
		  failed: ready
`)
	c.give(start())
	size := 0
	for i := range 1000 {
		c.give(machine.Event{Name: "work", From: "client", Data: map[string]any{"n": float64(i)}})
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
	forms := []string{"on: { done: waiting }", "on: { done: { go: waiting } }", "on: { done: [{ go: waiting }] }"}
	var stored []string
	for _, form := range forms {
		def, err := parse(t, "statechart: forms\ninitial: a\nstates:\n\ta:\n\t\tjob: ./w.js\n\t\t"+form+"\n\twaiting: { end: done }\n")
		if err != nil {
			t.Fatal(err)
		}
		stored = append(stored, string(def.Body))
	}
	if stored[0] != stored[1] || stored[1] != stored[2] {
		t.Fatalf("three ways of writing one rule give different stored forms:\n%s", strings.Join(stored, "\n"))
	}
}

// A ticket's value is handed to a person, who writes it as it is after a
// command or into an address: it holds letters and digits and nothing a
// command line or an address reads as something else, whatever the seed.
func TestTicketValueIsLettersAndDigits(t *testing.T) {
	def, err := parse(t, "statechart: asks\ninitial: waiting\nstates:\n\twaiting:\n\t\tticket: answer\n\t\ton: { answer: answered }\n\tanswered: { end: done }\n")
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
			if r.Ticket == nil {
				continue
			}
			value := r.Ticket.Value
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
