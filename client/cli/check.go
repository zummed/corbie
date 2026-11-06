package cli

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
)

// checkPrefix is the prefix the check's machine is submitted at, and the
// location its run starts at.
const checkPrefix = "/corbie-check"

// The check's run asks for one job and sets one timer, so that everything a
// setup has to do for a run happens once.
const checkChart = `statechart: check
initial: working
states:
  working:
    job: { script: ./check.js, timeout: 5s, tags: %s }
    on: { done: timing }
  timing:
    timers: { waited: 1s }
    on: { waited: checked }
  checked: { end: done }
`

const checkScript = `emit("taken", true)
return true
`

// The things that have to happen for the check's run to end, in the order
// they happen.
const (
	legStart  = "start"
	legJob    = "job"
	legResult = "result"
	legTimer  = "timer"
)

var legs = []string{legStart, legJob, legResult, legTimer}

var legSays = map[string]string{
	legStart:  "the controller took the start",
	legJob:    "a runner took the job",
	legResult: "the controller took the result",
	legTimer:  "the timer fired",
}

// legWaits is how the wait for each is said while it goes on.
var legWaits = map[string]string{
	legStart:  "for the controller to take the start",
	legJob:    "for a runner to take the job",
	legResult: "for the controller to take the result",
	legTimer:  "for the timer to fire",
}

const (
	legOK    = "ok"
	legLate  = "late"
	legNever = "never"
)

// late is how long a leg may take before it is no longer taken to have been
// called for. On a serverless host the once-a-minute call does whatever the
// bucket's own call should have, so a call that is missing shows only as a
// leg that took most of a minute.
const late = 20 * time.Second

// checkLooks is how often the check looks at its run: often, since how long
// each leg took is what it reports.
const checkLooks = 200 * time.Millisecond

// A Leg is one thing the check's run needed, and what became of it.
type Leg struct {
	Leg     string  `json:"leg"`
	Status  string  `json:"status"` // ok, late or never
	Seconds float64 `json:"seconds,omitzero"`
	Why     string  `json:"why,omitzero"`
}

// A Checked is what a check found.
type Checked struct {
	OK   bool  `json:"ok"`
	Legs []Leg `json:"legs"`
	// Run is the check's run, when it did not go as it should.
	Run *client.Run `json:"run,omitzero"`
}

// legOf says which leg an entry of the check's record shows to have
// happened, if any.
func legOf(entry client.Entry) string {
	if entry.Job != "" {
		return legJob // only a runner that has the job emits for it
	}
	for _, event := range entry.Events {
		switch event.Name {
		case "start":
			return legStart
		case "done":
			return legResult
		case "timer":
			return legTimer
		}
	}
	return ""
}

// judge makes a check's findings from when its start was sent, when each
// leg was first seen, when the looking stopped, and how the run then stood.
func judge(sent time.Time, seen map[string]time.Time, stopped time.Time, run *client.Run, controller, runner string) Checked {
	// A leg that was not seen itself happened if a later one did, and no
	// later than that one.
	for i := len(legs) - 2; i >= 0; i-- {
		if _, ok := seen[legs[i]]; !ok {
			if at, ok := seen[legs[i+1]]; ok {
				seen[legs[i]] = at
			}
		}
	}
	checked := Checked{OK: true}
	from := sent
	for _, name := range legs {
		at, ok := seen[name]
		if !ok {
			// A run that ended for a reason of its own says what it was. A
			// leg is missing only where the run is still waiting for it, or
			// gave up waiting.
			if !run.HasEnded() || run.Failure == machine.StatusTimeout {
				checked.Legs = append(checked.Legs, Leg{Leg: name, Status: legNever, Seconds: seconds(stopped.Sub(from)),
					Why: why(name, legNever, controller, runner)})
			}
			break
		}
		leg := Leg{Leg: name, Status: legOK, Seconds: seconds(max(at.Sub(from), 0))}
		// A timer on a serverless host waits for the once-a-minute call,
		// and that is no fault.
		if name != legTimer && at.Sub(from) > late {
			leg.Status, leg.Why = legLate, why(name, legLate, controller, runner)
		}
		checked.Legs = append(checked.Legs, leg)
		from = at
	}
	for _, leg := range checked.Legs {
		checked.OK = checked.OK && leg.Status == legOK
	}
	if checked.OK = checked.OK && len(checked.Legs) == len(legs) && run.Status == "done"; !checked.OK {
		checked.Run = run
	}
	return checked
}

func seconds(d time.Duration) float64 {
	return float64(d.Round(100*time.Millisecond)) / float64(time.Second)
}

// why says what leaves a leg late or missing, on each kind of host.
func why(leg, status, controller, runner string) string {
	ctlInbox, runInbox := controller+"/inbox/", runner+"/inbox/"
	switch {
	case leg == legStart && status == legNever:
		return "On a computer, corbie start controller is not running on this store. On a serverless host, nothing calls the controller's function: neither a write under " + ctlInbox + " nor the once-a-minute call."
	case leg == legJob && status == legNever:
		return "The job went to " + runner + ". On a computer, corbie start runner is not running there, or that runner does not trust this controller. On a serverless host, the bucket does not call the runner's function when something is written under " + runInbox + ", or the function's key may not read it (corbie controller policy " + runner + " prints what it needs)."
	case leg == legJob:
		return "The job went to " + runner + ". On a computer, that runner is busy or its --interval is long. On a serverless host, the bucket does not call the runner's function when something is written under " + runInbox + ", and another call found the job."
	case leg == legResult && status == legNever:
		return "The runner had the job and no result reached the controller. The runner stopped, or its key may not write under " + ctlInbox + " (corbie controller policy " + runner + " prints what it needs)."
	case leg == legTimer:
		return "On a serverless host, nothing calls the controller's function once a minute, and a timer is noticed only by that call."
	}
	return "On a computer, the controller is busy or its --interval is long. On a serverless host, the bucket does not call the controller's function when something is written under " + ctlInbox + ", and the once-a-minute call did the work."
}

func cmdCheck(e *Env, args []string) error {
	var tags list
	wait := "90s"
	e.Flags.Var(&tags, "tag", "")
	e.Flags.StringVar(&wait, "wait", wait, "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return Mistake("check takes only flags: --tag and --wait")
	}
	patience, err := machine.ParseDuration(wait)
	if err != nil {
		return Mistake("--wait: %v", err)
	}
	c, err := e.Client()
	if err != nil {
		return err
	}
	// A list written as JSON is YAML too.
	chosen, err := json.Marshal(append([]string{}, tags...))
	if err != nil {
		return err
	}
	draft, err := machine.Parse(fmt.Appendf(nil, checkChart, chosen), func(string) ([]byte, error) { return []byte(checkScript), nil })
	if err != nil {
		return err
	}
	// What is being waited for is said as the wait begins, so a setup with
	// nothing running does not look like a command that has hung.
	waiting := func(leg string) { e.Say("waiting up to %s %s", wait, legWaits[leg]) }
	waiting(legs[0])
	// The check's machine is submitted as a tree of one file, and its run
	// started there once the submit is taken. The submit, and then each leg,
	// is waited for this long from the one before it. The run is followed as any run is, and
	// what the check reports is what turned up in the store: it asks the
	// host nothing.
	ctx, stop := context.WithCancel(e.ctx)
	defer stop()
	sent := time.Now()
	giveUp := time.AfterFunc(time.Duration(patience), stop)
	message, err := c.Submit(e.ctx, &client.Tree{Prefix: checkPrefix, Machines: []*client.Machine{{Path: "check.yaml", Location: checkPrefix, Draft: draft}}}, 0)
	if err != nil {
		return err
	}
	receipt, err := c.WaitReceipt(ctx, message, 0, checkLooks)
	switch {
	case err != nil && ctx.Err() == nil && !errors.Is(err, client.ErrNotTaken):
		return err
	case err != nil:
		checked := Checked{Legs: []Leg{{Leg: legStart, Status: legNever, Seconds: seconds(time.Since(sent)), Why: why(legStart, legNever, c.Controller.Name, "")}}}
		if err := e.Print(checked, func() string { return checkedText(checked) }); err != nil {
			return err
		}
		return errRunFailed
	case !receipt.Taken:
		if err := e.Print(receipt, func() string { return "refused: " + receipt.Refused + "\n" }); err != nil {
			return err
		}
		return errRunFailed
	}
	// The first leg is the start alone: it is timed from here.
	sent = time.Now()
	giveUp.Reset(time.Duration(patience))
	id, err := c.Start(e.ctx, client.Start{At: checkPrefix})
	if err != nil {
		return err
	}
	e.Say("%s", id)
	seen := map[string]time.Time{}
	runner := "the runner"
	run, err := c.Follow(ctx, id, checkLooks, func(entry client.Entry) {
		for _, ask := range entry.Asks {
			if ask.Runner != "" {
				runner = ask.Runner
			}
		}
		leg := legOf(entry)
		if _, already := seen[leg]; leg == "" || already {
			return
		}
		seen[leg] = time.Now()
		giveUp.Reset(time.Duration(patience))
		if next := slices.Index(legs, leg) + 1; next < len(legs) {
			if _, already := seen[legs[next]]; !already {
				waiting(legs[next])
			}
		}
	})
	if err != nil {
		return err
	}
	checked := judge(sent, seen, time.Now(), run, c.Controller.Name, runner)
	if err := e.Print(checked, func() string { return checkedText(checked) }); err != nil {
		return err
	}
	if !checked.OK {
		return errRunFailed
	}
	return nil
}

// checkedText is what a check found, as it is printed.
func checkedText(checked Checked) string {
	var b strings.Builder
	for _, leg := range checked.Legs {
		took := fmt.Sprintf("%.1fs", leg.Seconds)
		if leg.Status == legNever {
			took = ""
		}
		fmt.Fprintf(&b, "  %-32s %-6s %s\n", legSays[leg.Leg], leg.Status, took)
		if leg.Why != "" {
			fmt.Fprintf(&b, "    %s\n", leg.Why)
		}
	}
	if checked.Run != nil && checked.Run.HasEnded() {
		b.WriteString(runText(checked.Run, false))
	}
	return b.String()
}
