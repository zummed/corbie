package machine

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
)

// A restart's limit and delay read as they are written.
func Test_mach_a_restart_reads_as_written(t *testing.T) {
	read := func(written string) (*JobSpec, error) {
		draft, err := Parse([]byte("probe: p\nwork:\n  a: { script: ./a.js, restart: "+written+" }\n"), Given(map[string]string{"./a.js": "return 1"}))
		if err != nil {
			return nil, err
		}
		var body probeBody
		if err := json.Unmarshal(draft.Definition.Body, &body); err != nil {
			t.Fatal(err)
		}
		return body.Jobs["a"], nil
	}
	for written, want := range map[string]Restart{
		"{}":                  {Delay: DefaultDelay, Longest: DefaultLongest},
		"{ limit: 2 }":        {Limit: 2, Delay: DefaultDelay, Longest: DefaultLongest},
		"{ limit: 5 in 1h }":  {Limit: 5, Within: Duration(time.Hour), Delay: DefaultDelay, Longest: DefaultLongest},
		"{ after: always }":   {Always: true, Delay: DefaultDelay, Longest: DefaultLongest},
		"{ after: failure }":  {Delay: DefaultDelay, Longest: DefaultLongest},
		"{ delay: 30s }":      {Delay: Duration(30 * time.Second), Longest: Duration(30 * time.Second)},
		"{ delay: 2s to 1m }": {Delay: Duration(2 * time.Second), Longest: Duration(time.Minute)},
	} {
		job, err := read(written)
		if err != nil {
			t.Fatalf("%s: %v", written, err)
		}
		if *job.Restart != want {
			t.Errorf("%s reads as %+v", written, *job.Restart)
		}
	}
	for written, want := range map[string]string{
		"{ after: never }":        "after is failure or always",
		"{ limit: 0 }":            "at least 1",
		"{ limit: -1 }":           "at least 1",
		"{ limit: 1.5 }":          "at least 1",
		"{ limit: soon }":         "at least 1",
		"{ limit: 3 in 0s }":      "more than nothing",
		"{ limit: 1e30 }":         "at least 1",
		"{ limit: 5 in soon }":    "soon",
		"{ delay: soon }":         "soon",
		"{ delay: 1m to 1s }":     "doubles to a longer one",
		"{ delay: 0s }":           "more than nothing",
		"{ delay: 1s to soon }":   "soon",
		"{ later: 1 }":            "later",
		`{ limit: "${input.n}" }`: "at least 1",
	} {
		if _, err := read(written); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s gave %v", written, err)
		}
	}
}

// A definition sent by a client that wrote it by hand is checked as a written
// one is.
func Test_mach_a_restart_is_checked(t *testing.T) {
	for _, bad := range []Restart{{Limit: -1, Delay: 1, Longest: 1}, {Within: -1, Limit: 1, Delay: 1, Longest: 1}, {Delay: 0, Longest: 1}, {Delay: 2, Longest: 1}, {Within: 1, Delay: 1, Longest: 1}} {
		c := &Checker{def: &Definition{Scripts: map[string]blob.Ref{"./a.js": {}}}}
		c.Job("work.a", &JobSpec{Script: "./a.js", Restart: &bad})
		if len(c.problems) == 0 {
			t.Errorf("%+v passed", bad)
		}
	}
}

// A delay doubles to its longest wait, and starts again once an attempt has stayed
// up for twice that.
func Test_mach_a_delay_doubles_and_starts_again(t *testing.T) {
	r := &Restart{Delay: Duration(time.Second), Longest: Duration(5 * time.Second)}
	doubled := 0
	var waits []time.Duration
	for range 5 {
		var wait time.Duration
		wait, doubled = r.Wait(doubled, 0)
		waits = append(waits, wait)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("the waits are %v", waits)
		}
	}
	if wait, _ := r.Wait(doubled, 10*time.Second-time.Millisecond); wait != 5*time.Second {
		t.Fatalf("a wait started again early: %v", wait)
	}
	wait, next := r.Wait(doubled, 10*time.Second)
	if wait != time.Second || next != 1 {
		t.Fatalf("after a long run the wait is %v, and the next count %d", wait, next)
	}
	fixed := &Restart{Delay: Duration(30 * time.Second), Longest: Duration(30 * time.Second)}
	for doubled := range 70 {
		if wait, _ := fixed.Wait(doubled, 0); wait != 30*time.Second {
			t.Fatalf("a fixed delay waited %v", wait)
		}
	}
}

// What a step's process wrote cannot give a wait of nothing or overflow it.
func Test_mach_a_wait_takes_nothing_on_trust(t *testing.T) {
	for _, r := range []Restart{{}, {Delay: -5, Longest: -5}, {Delay: 1, Longest: 1<<63 - 1}, {Delay: 1<<63 - 1, Longest: 1}} {
		doubled := 0
		for range 200 {
			var wait time.Duration
			wait, doubled = r.Wait(doubled, 0)
			if wait <= 0 {
				t.Fatalf("%+v waited %v", r, wait)
			}
		}
	}
}
