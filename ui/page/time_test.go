package page

import (
	"testing"
	"time"
)

func Test_page_when_gives_the_date_on_any_other_day(t *testing.T) {
	now := time.Date(2026, 11, 4, 9, 30, 0, 0, time.Local)
	for at, want := range map[time.Time]string{
		time.Date(2026, 11, 4, 0, 5, 0, 0, time.Local):   "00:05",
		time.Date(2026, 11, 4, 23, 59, 0, 0, time.Local): "23:59",
		time.Date(2026, 11, 3, 23, 59, 0, 0, time.Local): "3 Nov 23:59",
		time.Date(2025, 11, 4, 9, 0, 0, 0, time.Local):   "4 Nov 09:00",
	} {
		if got := When(at, now); got != want {
			t.Errorf("%v is shown as %q, not %q", at, got, want)
		}
	}
}

func Test_page_clock_gives_the_second_and_the_date_on_another_day(t *testing.T) {
	day := time.Date(2026, 11, 3, 20, 29, 12, 0, time.Local)
	for at, want := range map[time.Time]string{
		time.Date(2026, 11, 3, 20, 29, 11, 0, time.Local): "20:29:11",
		time.Date(2026, 11, 3, 23, 59, 59, 0, time.Local): "23:59:59",
		time.Date(2026, 11, 4, 0, 0, 1, 0, time.Local):    "4 Nov 00:00:01",
	} {
		if got := Clock(at, day); got != want {
			t.Errorf("%v is shown as %q, not %q", at, got, want)
		}
	}
}
