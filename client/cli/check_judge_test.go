package cli

import (
	"testing"
	"time"

	"github.com/zummed/corbie/client"
)

func Test_client_check_slow_leg_is_late(t *testing.T) {
	sent := time.Now()
	after := func(s int) time.Time { return sent.Add(time.Duration(s) * time.Second) }
	done := &client.Run{Status: "done"}
	// The controller took the result only at the next once-a-minute call,
	// and the timer waited for the one after: the first is late, the
	// second is not.
	seen := map[string]time.Time{legStart: after(1), legJob: after(2), legResult: after(58), legTimer: after(118)}
	got := judge(sent, seen, after(118), done, "ctl", "run")
	if got.OK || len(got.Legs) != 4 || got.Legs[2].Status != legLate || got.Legs[2].Why == "" || got.Legs[3].Status != legOK {
		t.Fatalf("a result taken after 56s and a timer after 60s were judged %+v", got)
	}
	if got.Legs[0].Status != legOK || got.Legs[1].Status != legOK {
		t.Fatalf("the quick legs were judged %+v", got)
	}
	// Every leg prompt: nothing is late.
	seen = map[string]time.Time{legStart: after(1), legJob: after(2), legResult: after(3), legTimer: after(50)}
	if got := judge(sent, seen, after(50), done, "ctl", "run"); !got.OK || got.Run != nil {
		t.Fatalf("a prompt run was judged %+v", got)
	}
	// A job whose result never came, and whose run gave up waiting: the
	// leg is missing, and the run says how it ended.
	seen = map[string]time.Time{legStart: after(1)}
	timedOut := &client.Run{Status: "failed", Failure: "timeout"}
	got = judge(sent, seen, after(40), timedOut, "ctl", "run")
	if got.OK || len(got.Legs) != 2 || got.Legs[1].Leg != legJob || got.Legs[1].Status != legNever || got.Run != timedOut {
		t.Fatalf("a job no runner took was judged %+v", got)
	}
}
