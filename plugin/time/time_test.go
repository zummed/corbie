package time_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zummed/corbie/machine"
	_ "github.com/zummed/corbie/machine/statechart"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	_ "github.com/zummed/corbie/plugin/time"
)

// A job no layer allows anything: the plugin needs no permission.
func call(t *testing.T, name string, first any, opt plugin.Options) (any, error) {
	t.Helper()
	job := &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test"}}},
		Scratch: t.TempDir(),
	}
	module, _ := plugin.Find("time")
	return module.Calls[name].Invoke(job, "time", name, first, opt)
}

func ms(at time.Time) float64 { return float64(at.UnixMilli()) }

func TestClockAndInstant(t *testing.T) {
	at := time.Date(2026, 7, 1, 11, 2, 3, 400_000_000, time.UTC)
	got, err := call(t, "clock", ms(at), plugin.Options{"zone": "America/New_York"})
	if err != nil {
		t.Fatal(err)
	}
	clock := got.(map[string]any)
	if clock["date"] != "2026-07-01" || clock["time"] != "07:02:03" {
		t.Fatalf("11:02:03 UTC in New York in July is %v", clock)
	}
	back, err := call(t, "instant", clock, plugin.Options{"zone": "America/New_York"})
	if err != nil || back != ms(at.Truncate(time.Second)) {
		t.Fatalf("the clock given back is the instant %v, %v; want %v", back, err, ms(at.Truncate(time.Second)))
	}
	// Without a zone both are UTC.
	if got, err := call(t, "clock", ms(at), nil); err != nil || got.(map[string]any)["time"] != "11:02:03" {
		t.Fatalf("without a zone the clock is %v, %v", got, err)
	}
	if got, err := call(t, "instant", map[string]any{"date": "2026-07-01", "time": "11:02"}, nil); err != nil || got != ms(time.Date(2026, 7, 1, 11, 2, 0, 0, time.UTC)) {
		t.Fatalf("without a zone 11:02 is %v, %v", got, err)
	}
}

func TestBadInput(t *testing.T) {
	clock := func(date, hour string) map[string]any { return map[string]any{"date": date, "time": hour} }
	for what, do := range map[string]func() (any, error){
		"a date the calendar does not have": func() (any, error) { return call(t, "instant", clock("2026-02-30", "07:30"), nil) },
		"24:00":                             func() (any, error) { return call(t, "instant", clock("2026-02-27", "24:00"), nil) },
		"an hour of one digit":              func() (any, error) { return call(t, "instant", clock("2026-02-27", "7:30"), nil) },
		"a clock with another field": func() (any, error) {
			return call(t, "instant", map[string]any{"date": "2026-02-27", "time": "07:30", "day": "fri"}, nil)
		},
		"a clock with no time": func() (any, error) { return call(t, "instant", map[string]any{"date": "2026-02-27"}, nil) },
		"an unknown zone":      func() (any, error) { return call(t, "clock", 0.0, plugin.Options{"zone": "Nowhere/Atall"}) },
		"Local": func() (any, error) {
			return call(t, "instant", clock("2026-02-27", "07:30"), plugin.Options{"zone": "Local"})
		},
		"text where an instant goes":  func() (any, error) { return call(t, "clock", "2026-02-27", nil) },
		"an instant no Date can hold": func() (any, error) { return call(t, "clock", 1e16, nil) },
		"an instant in the year 10000": func() (any, error) {
			return call(t, "clock", ms(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)), nil)
		},
		"an instant in the year 10000 in the zone": func() (any, error) {
			return call(t, "clock", ms(time.Date(9999, 12, 31, 23, 0, 0, 0, time.UTC)), plugin.Options{"zone": "Pacific/Kiritimati"})
		},
	} {
		var e *plugin.Error
		if _, err := do(); !errors.As(err, &e) || e.Code != plugin.BadInput {
			t.Errorf("%s gave %v, want bad_input", what, err)
		}
	}
}

// A timer and time.instant give the same moment for the same time of day,
// on the days the clock skips it and shows it twice.
func TestTimersAndTimeAgree(t *testing.T) {
	chart, _ := machine.TypeOf("statechart")
	draft, err := machine.Parse([]byte(`statechart: waiting
initial: waiting
states:
  waiting:
    timers: { due: "${input}" }
    on: { due: done }
  done: { end: done }
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ zone, date, time string }{
		{"Europe/London", "2026-03-29", "01:30"},
		{"Europe/London", "2026-10-25", "01:30"},
		{"Europe/London", "2026-03-30", "07:30"},
		{"America/New_York", "2026-03-08", "02:30"},
		{"America/New_York", "2026-11-01", "01:30"},
		{"Australia/Sydney", "2026-10-04", "02:30"},
		{"Australia/Sydney", "2026-04-05", "02:30"},
	} {
		zone, _ := time.LoadLocation(c.zone)
		day, _ := time.Parse("2006-01-02", c.date)
		midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
		out, err := chart.Step(&machine.StepIn{Definition: draft.Definition, Events: []machine.Event{{Name: machine.Start}},
			Input: c.time + " " + c.zone, Time: midnight, Seed: []byte("seed")})
		if err != nil {
			t.Fatal(err)
		}
		var timer time.Time
		for _, r := range out.Requests {
			if r.Timer != nil {
				timer = r.Timer.At
			}
		}
		got, err := call(t, "instant", map[string]any{"date": c.date, "time": c.time}, plugin.Options{"zone": c.zone})
		if err != nil || got != ms(timer) {
			t.Errorf("%s %s in %s: a timer is due %v, and time.instant gives %v, %v", c.date, c.time, c.zone, timer, got, err)
		}
	}
}
