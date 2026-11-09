// Package time is the plugin that turns an instant into the date and time
// of day a time zone's clock shows, and back, with the zones and the rule
// timers use.
package time

import (
	"math"
	"time"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "time", Page: page, Calls: map[string]*plugin.Call{
		"clock":   {Options: []string{"zone"}, Do: clock},
		"instant": {Options: []string{"zone"}, Do: instant},
	}})
}

// The forms of a date and of a time of day, with seconds and without.
const (
	dateForm    = "2006-01-02"
	secondsForm = "15:04:05"
	minutesForm = "15:04"
)

// maxMS is the furthest instant from 1970 a script's Date holds.
const maxMS = 8.64e15

func clock(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	ms, ok := first.(float64)
	if !ok || math.IsNaN(ms) || math.Abs(ms) > maxMS {
		return nil, plugin.Errorf(plugin.BadInput, "time.clock needs an instant first: milliseconds since 1970 UTC, as Date.now() gives")
	}
	zone, err := zoneOf(opt)
	if err != nil {
		return nil, err
	}
	at := time.UnixMilli(int64(math.Floor(ms))).In(zone)
	return map[string]any{"date": at.Format(dateForm), "time": at.Format(secondsForm)}, nil
}

func instant(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	bad := plugin.Errorf(plugin.BadInput, "time.instant needs a clock first: { date, time }, such as { date: \"2026-03-30\", time: \"07:30\" }")
	given, ok := first.(map[string]any)
	if !ok || len(given) != 2 {
		return nil, bad
	}
	date, okDate := given["date"].(string)
	hour, okTime := given["time"].(string)
	if !okDate || !okTime {
		return nil, bad
	}
	day, err := time.Parse(dateForm, date)
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "%q is not a date of the calendar, written YYYY-MM-DD", date)
	}
	form := secondsForm
	if len(hour) == len(minutesForm) {
		form = minutesForm
	}
	of, err := time.Parse(form, hour)
	if err != nil || len(hour) != len(form) {
		return nil, plugin.Errorf(plugin.BadInput, "%q is not a time of day, written HH:MM or HH:MM:SS", hour)
	}
	zone, err := zoneOf(opt)
	if err != nil {
		return nil, err
	}
	at := machine.At(day.Year(), day.Month(), day.Day(), of.Hour(), of.Minute(), of.Second(), zone)
	return float64(at.UnixMilli()), nil
}

// zoneOf reads the zone option, UTC by default.
func zoneOf(opt plugin.Options) (*time.Location, error) {
	name, err := opt.Text("zone", "UTC")
	if err != nil {
		return nil, err
	}
	zone, err := machine.Zone(name)
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "%v", err)
	}
	return zone, nil
}

const page = `time: this plugin turns an instant into a date and time of day in a time zone, and back.

  time.clock(ms, { zone })        gives { date, time }: what the zone's clock shows at the instant
  time.instant(clock, { zone })   the instant, in ms, at which the zone's clock shows clock

ms is an instant as Date.now() gives it: milliseconds since 1970 UTC. clock
is { date, time }, as time.clock gives it.

  date   YYYY-MM-DD
  time   HH:MM:SS from time.clock; HH:MM or HH:MM:SS to time.instant

Options:
  zone   a time zone, such as Europe/London; default UTC

- A time the zone's clock skips on a day is the moment the clock jumps past
  it, and a time the clock shows twice is the first, as timers have it.
- A day in a zone is not always 24 hours long, so a script steps a day on
  date, as the second example does, and never adds 86400000 to ms.
- A time a definition writes is a timer's. A time worked out from data is
  time.instant's, and a statechart waits for it with a timer of
  "${Math.max(0, at - Date.now())}ms".

Errors:
  bad_input   a date or a time of day the calendar does not have, a clock with
              another field, an unknown zone, or Local, the zone of whichever
              computer reads it

Needs nothing.

07:30 in London is 07:30 UTC before the clocks change in March 2026, and 06:30
UTC after. The clock skips 01:30 on 29 March and shows it twice on 25 October.

    const zone = "Europe/London"
    time.instant({ date: "2026-03-27", time: "07:30" }, { zone })   // gives Date.UTC(2026, 2, 27, 7, 30)
    time.instant({ date: "2026-03-30", time: "07:30" }, { zone })   // gives Date.UTC(2026, 2, 30, 6, 30)
    time.instant({ date: "2026-03-29", time: "01:30" }, { zone })   // gives Date.UTC(2026, 2, 29, 1, 0)
    time.instant({ date: "2026-10-25", time: "01:30" }, { zone })   // gives Date.UTC(2026, 9, 25, 0, 30)

This script works out the next 07:30 in London after 09:00 on 28 March 2026.

    const zone = "Europe/London"
    const now = time.clock(Date.UTC(2026, 2, 28, 9, 0), { zone })   // gives { date: "2026-03-28", time: "09:00:00" }
    const [y, m, d] = now.date.split("-").map(Number)
    const tomorrow = new Date(Date.UTC(y, m - 1, d + 1)).toISOString().slice(0, 10)
    time.instant({ date: tomorrow, time: "07:30" }, { zone })     // gives Date.UTC(2026, 2, 29, 6, 30)
`
