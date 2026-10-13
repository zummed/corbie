// Package parity_test runs the parity cases through every role in one
// process, and checks how each ends.
//
// A case is a directory of parity/machines. One that is written a second way
// has a directory for that way inside it, which holds the same files.
//
//	root.yaml     the definition that is started; its other files lie beside it
//	expect.json   how the run should end
//	drive.json    what a client sends while it runs, if anything
//	input.json    the run's input; a second way with none uses the case's own
//
// expect.json:
//
//	{ "status": "done|failed|cancelled", "failure": "error|timeout|denied",
//	  "output": …, "error_contains": "…" or ["…", …],
//	  "record": [ … ], "note": "…", "finding": "…", "seconds": 5 }
//
// or, for a definition that is refused before it runs, by the client's
// check or in the receipt for its start:
//
//	{ "refused": true, "problem_contains": "…" }
//
// Inside "output" three values stand for more than themselves: "<any>" is
// any value, "<blob>" is any blob, and { "<contains>": "text" } is any
// text that holds that text.
//
// "record" checks the record of the run and of everything below it:
//
//	{ "machine": "child", "status": "cancelled" }                  every instance of it ended so
//	{ "machine": "parent", "event": { "name": "up1" } }            it was given such an event
//	{ "machine": "parent", "ask": { "kind": "event", … } }         a step of it asked for this
//	{ "machine": "agent", "emit": { "job": "turn#2", "name": "log", "data": … } }   a job of it emitted this
//	{ "machine": "parent", "order": ["up1", "up2"] }               of the events with these names, it was given exactly these, in this order
//	{ …, "absent": true }                                          there was no such event or ask
//	{ "machine": "f", "ask": { … }, "not_before": { "name": "done", "from": "source" } }
//	                                                               no step asked for this before the step given such an event
//
// "finding" marks a case in which Corbie does not do what its
// specs say. Such a case is run and skipped, and fails if it starts to pass.
// One thing looked for in the record, or one step of the drive, may carry a
// "finding" of its own: then only that check is expected to fail, everything
// else about the case is still checked, and the case is skipped.
//
// drive.json is a list of steps, taken in order. Each waits and then acts:
//
//	{ "wait": { "machine": "child", "key": "answer", "input": { … }, "status": "done" },
//	  "do": "reply|send|cancel|nothing",
//	  "value": …,                      a reply's value
//	  "wrong_key": true,               reply with a key the run is not waiting on
//	  "remember": "k",                 keep the instance and key found, under a name
//	  "remembered": "k",               reply to the instance and key kept under a name
//	  "event": "go", "data": …,        an event to send, to the machine waited for
//	  "taken": false,                  what the receipt should say
//	  "acted": false,
//	  "refused_contains": "…" }
package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/runner"

	_ "github.com/zummed/corbie/agent"
	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
	_ "github.com/zummed/corbie/statechart"
)

// The test binary stands in for the process a runner starts for a job.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	os.Exit(m.Run())
}

var ctx = context.Background()

// How long a case may take, and how often a waiting test looks again.
const (
	limit        = 60 * time.Second
	receiptLimit = 3 * time.Second
	settleLimit  = 3 * time.Second
	poll         = 20 * time.Millisecond
)

type expect struct {
	Refused         bool   `json:"refused"`
	ProblemContains texts  `json:"problem_contains"`
	Status          string `json:"status"`
	Failure         string `json:"failure"`
	ErrorContains   texts  `json:"error_contains"`
	Record          []look `json:"record"`
	Note            string `json:"note"`
	Finding         string `json:"finding"`
	// Seconds is how long the case may take, where a minute is too long to
	// wait for one that is known not to end.
	Seconds float64 `json:"seconds"`

	output    any
	hasOutput bool
}

// texts is one text or a list of them.
type texts []string

func (t *texts) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*t = texts{one}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(t))
}

// look is one thing to look for in the record.
type look struct {
	Machine string         `json:"machine,omitempty"`
	Status  string         `json:"status,omitempty"`
	Event   map[string]any `json:"event,omitempty"`
	Ask     map[string]any `json:"ask,omitempty"`
	Emit    map[string]any `json:"emit,omitempty"`
	Order   []string       `json:"order,omitempty"`
	// NotBefore is an event that the ask looked for must not come before.
	NotBefore map[string]any `json:"not_before,omitempty"`
	Absent    bool           `json:"absent,omitempty"`
	Finding   string         `json:"finding,omitempty"`
}

type drive struct {
	Wait struct {
		Machine string `json:"machine"`
		Key     string `json:"key"`
		Input   any    `json:"input"`
		Status  string `json:"status"`
	} `json:"wait"`
	Do              string `json:"do"`
	Value           any    `json:"value"`
	WrongKey        bool   `json:"wrong_key"`
	Remember        string `json:"remember"`
	Remembered      string `json:"remembered"`
	Event           string `json:"event"`
	Data            any    `json:"data"`
	Taken           *bool  `json:"taken"`
	Acted           *bool  `json:"acted"`
	RefusedContains string `json:"refused_contains"`
	Finding         string `json:"finding"`
}

func readJSON(t *testing.T, path string, into any) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return true
}

func TestParity(t *testing.T) {
	cases, err := filepath.Glob("machines/*/expect.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("there are no cases")
	}
	// A case written a second way has a directory for it inside the case.
	ways, err := filepath.Glob("machines/*/*/expect.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range append(cases, ways...) {
		dir := filepath.Dir(file)
		name := strings.ReplaceAll(strings.TrimPrefix(filepath.ToSlash(dir), "machines/"), "/", "-")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var want expect
			var raw map[string]json.RawMessage
			if !readJSON(t, filepath.Join(dir, "expect.json"), &want) {
				t.Fatal("the case has no expect.json")
			}
			readJSON(t, filepath.Join(dir, "expect.json"), &raw)
			if out, ok := raw["output"]; ok {
				want.hasOutput = true
				if err := json.Unmarshal(out, &want.output); err != nil {
					t.Fatal(err)
				}
			}
			wrong, findings := runCase(t, dir, &want)
			switch {
			case want.Finding != "" && len(wrong) == 0:
				t.Fatalf("the case passes, and is marked as a finding: %s", want.Finding)
			case want.Finding != "":
				t.Skipf("finding: %s\n%s", want.Finding, strings.Join(append(findings, wrong...), "\n"))
			case len(wrong) > 0:
				t.Fatal(strings.Join(wrong, "\n"))
			case len(findings) > 0:
				t.Skip(strings.Join(findings, "\n"))
			}
		})
	}
}

// runCase runs one case and says what about it is not as expected, and
// which of the checks marked as findings failed as their findings say.
func runCase(t *testing.T, dir string, want *expect) (wrong, findings []string) {
	t.Helper()
	say := func(format string, args ...any) { wrong = append(wrong, fmt.Sprintf(format, args...)) }
	// mark sorts the outcome of one check that may be marked as a finding.
	mark := func(finding, problem string) {
		switch {
		case finding == "" && problem != "":
			wrong = append(wrong, problem)
		case finding != "" && problem == "":
			wrong = append(wrong, "a check passes, and is marked as a finding: "+finding)
		case finding != "":
			findings = append(findings, "finding: "+finding+"\n  "+problem)
		}
	}
	src, err := os.ReadFile(filepath.Join(dir, "root.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	draft, err := machine.Parse(src, func(path string) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	})
	// refused checks what a refusal says against what the case expects.
	refused := func(problems string) ([]string, []string) {
		for _, text := range want.ProblemContains {
			if !strings.Contains(problems, text) {
				say("the problems do not mention %q:\n%s", text, problems)
			}
		}
		return wrong, nil
	}
	if want.Refused && err != nil {
		return refused(err.Error())
	}
	if err != nil {
		return []string{fmt.Sprintf("the definition does not pass:\n%v", err)}, nil
	}

	var input any
	if !readJSON(t, filepath.Join(dir, "input.json"), &input) {
		readJSON(t, filepath.Join(filepath.Dir(dir), "input.json"), &input)
	}
	l, err := client.NewLocal(ctx, permission.Layer{Allow: []string{"all"}}, func(role string, err error) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", role, err)
	})
	if err != nil {
		t.Fatal(err)
	}
	l.Runner.Command = []string{os.Args[0], runner.JobArg}
	l.Controller.AnyRunner = true
	t.Cleanup(l.Close)

	long := limit
	if want.Seconds > 0 {
		long = time.Duration(want.Seconds * float64(time.Second))
	}
	within, cancel := context.WithTimeout(ctx, long)
	defer cancel()
	id, err := l.Start(within, draft, client.Start{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	if want.Refused {
		// The client's check let it through: the controller checks it
		// again, and says in the start's receipt whether it took it.
		_, message, _ := strings.Cut(id, "/")
		for until := time.Now().Add(receiptLimit); time.Now().Before(until); time.Sleep(poll) {
			receipt, err := l.Receipt(within, message)
			if err != nil {
				t.Fatal(err)
			}
			if receipt != nil && !receipt.Taken {
				return refused(receipt.Refused + "\n" + receipt.Problems.Error())
			}
			if receipt != nil {
				break
			}
		}
		return []string{"the definition passed its check and its start was taken: it should have been refused"}, nil
	}
	var steps []drive
	readJSON(t, filepath.Join(dir, "drive.json"), &steps)
	d := &driver{t: t, l: l, root: id, ctx: within, kept: map[string][2]string{}}
	for i, step := range steps {
		problem := d.take(step)
		if problem != "" {
			problem = fmt.Sprintf("drive step %d: %s", i+1, problem)
		}
		mark(step.Finding, problem)
		if strings.Contains(problem, ": waited") {
			break
		}
	}
	r, err := l.Wait(within, id, poll)
	if err != nil {
		say("waiting for the run: %v (it is %s)", err, jsonOf(r))
		return wrong, findings
	}
	if r.Status != want.Status || r.Failure != want.Failure {
		say("the run ended %s %s, want %s %s (error %q, output %s)", r.Status, r.Failure, want.Status, want.Failure, r.Error, jsonOf(r.Output))
	}
	for _, text := range want.ErrorContains {
		if !strings.Contains(r.Error, text) {
			say("the error is %q, and does not hold %q", r.Error, text)
		}
	}
	if want.hasOutput {
		// The output as a client is shown it, read back as plain data.
		var got any
		json.Unmarshal([]byte(jsonOf(r.Output)), &got)
		if at := differ(want.output, got, "output"); at != "" {
			say("%s\n  the output is %s\n  want          %s", at, jsonOf(got), jsonOf(want.output))
		}
	}
	if len(want.Record) > 0 {
		for i, problem := range d.record(want.Record) {
			mark(want.Record[i].Finding, problem)
		}
	}
	return wrong, findings
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// differ says where a value is not what is wanted, or nothing.
func differ(want, got any, at string) string {
	switch w := want.(type) {
	case string:
		switch w {
		case "<any>":
			return ""
		case "<blob>":
			if m, ok := got.(map[string]any); ok && m["$blob"] != nil {
				return ""
			}
			return at + " is not a blob"
		}
	case map[string]any:
		if text, ok := w["<contains>"].(string); ok && len(w) == 1 {
			if s, isText := got.(string); isText && strings.Contains(s, text) {
				return ""
			}
			return fmt.Sprintf("%s does not hold %q", at, text)
		}
		g, ok := got.(map[string]any)
		if !ok {
			return at + " is not a map"
		}
		for name, value := range w {
			if _, has := g[name]; !has && value != nil {
				return fmt.Sprintf("%s.%s is missing", at, name)
			}
			if d := differ(value, g[name], at+"."+name); d != "" {
				return d
			}
		}
		for name, value := range g {
			if _, has := w[name]; !has && value != nil {
				return fmt.Sprintf("%s.%s is there and was not wanted", at, name)
			}
		}
		return ""
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return fmt.Sprintf("%s is not a list of %d", at, len(w))
		}
		for i := range w {
			if d := differ(w[i], g[i], fmt.Sprintf("%s[%d]", at, i)); d != "" {
				return d
			}
		}
		return ""
	}
	if !reflect.DeepEqual(want, got) {
		return fmt.Sprintf("%s is %s, want %s", at, jsonOf(got), jsonOf(want))
	}
	return ""
}

// within says whether everything in a pattern is in a value: a map may
// hold more than the pattern names, and everything else must be equal.
func within(pattern, value any) bool {
	switch p := pattern.(type) {
	case map[string]any:
		if text, ok := p["<contains>"].(string); ok && len(p) == 1 {
			return strings.Contains(jsonOf(value), text)
		}
		v, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for name, want := range p {
			if !within(want, v[name]) {
				return false
			}
		}
		return true
	case []any:
		v, ok := value.([]any)
		if !ok || len(v) != len(p) {
			return false
		}
		for i := range p {
			if !within(p[i], v[i]) {
				return false
			}
		}
		return true
	}
	return differ(pattern, value, "") == ""
}

// driver is the client of a case: it waits for what a step of the drive
// names and sends what it says.
type driver struct {
	t    *testing.T
	l    *client.Local
	root string
	ctx  context.Context
	kept map[string][2]string // an instance and a key, by the name they were kept under
}

// find waits for an instance that is as a step says, and gives it with the
// value of the key waited for, if one was.
func (d *driver) find(step drive) (id, key string, problem string) {
	w := step.Wait
	if w.Machine == "" && w.Key == "" && w.Status == "" {
		return d.root, "", ""
	}
	for {
		runs, err := d.l.List(d.ctx, client.Filter{Finished: true})
		if err != nil {
			return "", "", "waited: " + err.Error()
		}
		for _, r := range runs {
			if w.Machine != "" && r.Machine != w.Machine {
				continue
			}
			if w.Input != nil {
				first, err := d.l.Step(d.ctx, r.ID, 1)
				if err != nil || len(first.Events) == 0 || differ(w.Input, plain(first.Events[0].Data), "") != "" {
					continue
				}
			}
			if w.Status != "" {
				// The record is ahead of the head, and is what is right.
				shown, err := d.l.Show(d.ctx, r.ID)
				if err != nil || shown.Status != w.Status {
					continue
				}
			}
			if w.Key == "" {
				return r.ID, "", ""
			}
			keys, err := d.l.Keys(d.ctx, r.ID)
			if err != nil {
				continue
			}
			if value, ok := keys[w.Key]; ok {
				return r.ID, value, ""
			}
		}
		select {
		case <-d.ctx.Done():
			return "", "", fmt.Sprintf("waited for %s and it never came", jsonOf(w))
		case <-time.After(poll):
		}
	}
}

func plain(v any) any {
	var out any
	json.Unmarshal([]byte(jsonOf(v)), &out)
	return out
}

func (d *driver) take(step drive) string {
	id, key, problem := d.find(step)
	if problem != "" {
		return problem
	}
	if step.Remember != "" {
		d.kept[step.Remember] = [2]string{id, key}
	}
	if step.Remembered != "" {
		kept, ok := d.kept[step.Remembered]
		if !ok {
			return "nothing was kept as " + step.Remembered
		}
		id, key = kept[0], kept[1]
	}
	var message string
	var err error
	switch step.Do {
	case "reply":
		if step.WrongKey {
			key = "not-a-key-the-run-waits-on"
		}
		message, err = d.l.Reply(d.ctx, id, key, step.Value, 0)
	case "send":
		message, err = d.l.Send(d.ctx, id, step.Event, step.Data, 0)
	case "cancel":
		message, err = d.l.Cancel(d.ctx, d.root, 0)
	case "nothing", "":
		return ""
	default:
		return "the drive does not know how to " + step.Do
	}
	if err != nil {
		return err.Error()
	}
	if step.Taken == nil && step.Acted == nil && step.RefusedContains == "" {
		return ""
	}
	var receipt *instance.Receipt
	for until := time.Now().Add(receiptLimit); receipt == nil; {
		if receipt, err = d.l.Receipt(d.ctx, message); err != nil {
			return err.Error()
		}
		if receipt != nil {
			break
		}
		if time.Now().After(until) {
			return "no receipt came for the " + step.Do
		}
		time.Sleep(poll)
	}
	switch {
	case step.Taken != nil && receipt.Taken != *step.Taken:
		return fmt.Sprintf("the receipt for the %s says taken: %v (%s)", step.Do, receipt.Taken, receipt.Refused)
	case step.Acted != nil && (receipt.Acted == nil || *receipt.Acted != *step.Acted):
		return fmt.Sprintf("the receipt for the %s does not say acted: %v", step.Do, *step.Acted)
	case !strings.Contains(receipt.Refused, step.RefusedContains):
		return fmt.Sprintf("the receipt for the %s says %q, which does not hold %q", step.Do, receipt.Refused, step.RefusedContains)
	}
	return ""
}

// record reads the record of the run and of everything below it, and says
// for each of the things looked for what is not as wanted, or nothing.
func (d *driver) record(looks []look) []string {
	wrong := make([]string, len(looks))
	all := func(text string) []string {
		for i := range wrong {
			wrong[i] = text
		}
		return wrong
	}
	// A machine that ends cancels what is below it, and that takes the
	// controller a pass or two more: the record is read once nothing is
	// running, or after a while if something never stops.
	for until := time.Now().Add(settleLimit); time.Now().Before(until); time.Sleep(poll) {
		if running, err := d.l.List(d.ctx, client.Filter{}); err == nil && len(running) == 0 {
			break
		}
	}
	var entries []client.Entry
	if _, err := d.l.Follow(d.ctx, d.root, poll, func(e client.Entry) { entries = append(entries, e) }); err != nil {
		return all("reading the record: " + err.Error())
	}
	runs, err := d.l.List(d.ctx, client.Filter{Finished: true})
	if err != nil {
		return all("listing the runs: " + err.Error())
	}
	machines := map[string]string{}
	for _, r := range runs {
		machines[r.ID] = r.Machine
	}
	for i, look := range looks {
		found, order, statuses := false, []string{}, []string{}
		given, early := false, false // the event of not_before was given; the ask came before it
		for _, r := range runs {
			if look.Machine == "" || r.Machine == look.Machine {
				shown, err := d.l.Show(d.ctx, r.ID)
				if err == nil {
					statuses = append(statuses, shown.Status)
				}
			}
		}
		for _, e := range entries {
			if look.Machine != "" && machines[e.ID] != look.Machine {
				continue
			}
			if look.Emit != nil && e.Job != "" && within(map[string]any(look.Emit), plain(e)) {
				found = true
			}
			for _, event := range e.Events {
				if look.Event != nil && within(map[string]any(look.Event), plain(event)) {
					found = true
				}
				if slices.Contains(look.Order, event.Name) && !event.Own {
					order = append(order, event.Name)
				}
				if look.NotBefore != nil && within(map[string]any(look.NotBefore), plain(event)) {
					given = true
				}
			}
			for _, ask := range e.Asks {
				if look.Ask != nil && within(map[string]any(look.Ask), plain(ask)) {
					found = true
					early = early || !given
				}
			}
		}
		what := jsonOf(look)
		switch {
		case look.Status != "":
			if len(statuses) == 0 {
				wrong[i] = fmt.Sprintf("record: there is no instance of %s", look.Machine)
			}
			for _, status := range statuses {
				if status != look.Status {
					wrong[i] = fmt.Sprintf("record: %s ended %v, want %s", look.Machine, statuses, look.Status)
					break
				}
			}
		case look.Order != nil:
			if !slices.Equal(order, look.Order) {
				wrong[i] = fmt.Sprintf("record: %s was given %v, want %v", look.Machine, order, look.Order)
			}
		case look.NotBefore != nil && early:
			wrong[i] = "record: it came before the event: " + what
		case found && look.Absent:
			wrong[i] = "record: there should be no " + what
		case !found && !look.Absent:
			wrong[i] = "record: there is no " + what
		}
	}
	return wrong
}
