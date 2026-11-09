package machine

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // a host with no zone files still knows every zone
)

// Zone gives the time zone a name names, such as Europe/London. Local, the
// zone of whichever computer reads it, is not one.
func Zone(name string) (*time.Location, error) {
	if strings.EqualFold(name, "local") {
		return nil, fmt.Errorf("a zone has a name, such as Europe/London, and %q, the zone of whichever computer reads it, is not one", name)
	}
	zone, err := time.LoadLocation(name)
	if err != nil || name == "" {
		return nil, fmt.Errorf("%q is not a time zone", name)
	}
	return zone, nil
}

// At gives the moment a zone's clock shows a date and a time of day. A
// time the clock skips that day is the moment the clock jumps past it, and
// a time it shows twice is the first. The date may run over, as
// time.Date's does.
func At(year int, month time.Month, day, hour, minute, second int, zone *time.Location) time.Time {
	wall := time.Date(year, month, day, hour, minute, second, 0, time.UTC)
	// The clock shows the time at wall less the offset the zone has then.
	// An offset today is at most 14 hours either way, so each offset the zone
	// has from 14 hours before wall to 14 hours after gives a moment, and
	// those moments at which the zone has that offset show the time.
	var first, lo, hi time.Time
	for i, probe := range []time.Time{wall.Add(-14 * time.Hour), wall, wall.Add(14 * time.Hour)} {
		_, offset := probe.In(zone).Zone()
		at := wall.Add(-time.Duration(offset) * time.Second)
		if i == 0 || at.Before(lo) {
			lo = at
		}
		if i == 0 || at.After(hi) {
			hi = at
		}
		if _, shown := at.In(zone).Zone(); shown == offset && (first.IsZero() || at.Before(first)) {
			first = at
		}
	}
	if first.IsZero() {
		// No moment shows the time, so the clock jumps past it between them.
		return jump(lo, hi, zone)
	}
	return first
}

// jump finds the moment between lo and hi at which the zone's clock
// changes.
func jump(lo, hi time.Time, zone *time.Location) time.Time {
	_, start := lo.In(zone).Zone()
	for hi.Sub(lo) > time.Second {
		mid := lo.Add(hi.Sub(lo) / 2)
		if _, offset := mid.In(zone).Zone(); offset == start {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi.Truncate(time.Second).UTC()
}
