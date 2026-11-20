package page

import "time"

// When is a time as a page shows it: the time of day when it is today, and
// the date before it on any other day.
func When(t, now time.Time) string { return stamp(t, now, "15:04") }

// Clock is a time to the second, as a run's steps show it: the time of day
// when it is on day's date, and the date before it on any other.
func Clock(t, day time.Time) string { return stamp(t, day, "15:04:05") }

// Dated says whether a time is on another date than day's, and so is shown
// with its date.
func Dated(t, day time.Time) bool {
	t, day = t.Local(), day.Local()
	y, m, d := day.Date()
	return t.Year() != y || t.Month() != m || t.Day() != d
}

func stamp(t, day time.Time, clock string) string {
	if Dated(t, day) {
		clock = "2 Jan " + clock
	}
	return t.Local().Format(clock)
}
