package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// found is what corbie check --json prints.
type found struct {
	OK   bool `json:"ok"`
	Legs []struct {
		Leg, Status, Why string
	} `json:"legs"`
	Run struct{ Error string } `json:"run"`
}

func (m machine) check(args ...string) (int, found) {
	m.t.Helper()
	status, out, said := m.run("", append([]string{"check", "--json"}, args...)...)
	var f found
	if err := json.Unmarshal([]byte(out), &f); err != nil {
		m.t.Fatalf("corbie check printed %q and said %q: %v", out, said, err)
	}
	return status, f
}

func TestCheckOfAWorkingMachine(t *testing.T) {
	m, _ := working(t, "")
	status, f := m.check()
	if status != 0 || !f.OK || len(f.Legs) != 4 {
		t.Fatalf("a working machine was checked as %d %+v", status, f)
	}
	for i, leg := range []string{"start", "job", "result", "timer"} {
		if f.Legs[i].Leg != leg || f.Legs[i].Status != "ok" {
			t.Fatalf("leg %d was %+v", i, f.Legs[i])
		}
	}
	text := m.ok("", "check")
	for _, line := range []string{"the controller took the start", "a runner took the job", "the controller took the result", "the timer fired"} {
		if !strings.Contains(text, line) {
			t.Fatalf("the check printed no line for %q:\n%s", line, text)
		}
	}
}

func TestCheckSaysWhichPartDidNotAnswer(t *testing.T) {
	// Nothing running: no controller takes the start.
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box")
	status, f := m.check("--wait", "300ms")
	if status != 1 || f.OK || len(f.Legs) != 1 || f.Legs[0].Leg != "start" || f.Legs[0].Status != "never" ||
		!strings.Contains(f.Legs[0].Why, "box-controller/inbox/") {
		t.Fatalf("with nothing running the check found %d %+v", status, f)
	}
	// While it waits it says what for.
	if _, _, said := m.run("", "check", "--wait", "100ms"); !strings.Contains(said, "waiting up to 100ms for the controller to take the start") {
		t.Fatalf("a check with nothing running said only:\n%s", said)
	}
	// A controller and no runner: the start is taken and the job is not,
	// and the check names the runner the job went to and its inbox.
	m.staying("run", "controller", "--interval", "20ms")
	status, f = m.check("--wait", "2s")
	if status != 1 || f.OK || len(f.Legs) != 2 || f.Legs[0].Status != "ok" || f.Legs[1].Leg != "job" || f.Legs[1].Status != "never" ||
		!strings.Contains(f.Legs[1].Why, "box-runner/inbox/") {
		t.Fatalf("with no runner the check found %d %+v", status, f)
	}
}

func TestCheckOfARunThatEndsForItsOwnReason(t *testing.T) {
	m, _ := working(t, "")
	status, f := m.check("--tag", "gpu")
	if status != 1 || f.OK || len(f.Legs) != 1 || f.Legs[0].Status != "ok" || !strings.Contains(f.Run.Error, "no runner has the tags") {
		t.Fatalf("a check sent to a tag no runner has found %d %+v", status, f)
	}
}

func Test_client_check_is_an_ordinary_run(t *testing.T) {
	m, _ := working(t, "")
	if status, f := m.check(); status != 0 {
		t.Fatalf("the check found %+v", f)
	}
	var runs []struct{ Machine, Type, Status string }
	if err := json.Unmarshal([]byte(m.ok("", "ls", "--all", "--json")), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Machine != "check" || runs[0].Type != "statechart" || runs[0].Status != "done" {
		t.Fatalf("after a check the runs listed were %+v", runs)
	}
}
