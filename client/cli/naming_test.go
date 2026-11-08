package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const room = `statechart: room
lifetime: keyed
initial: open
states:
  open: { on: { said: open } }
`

// run cancel /location#key cancels the running run of that key, and no
// other; with no run of that key it fails.
func Test_client_cancel_by_key(t *testing.T) {
	m, _ := working(t, "all")
	dir := filepath.Join(t.TempDir(), "house")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "room.yaml"), []byte(room), 0o644)
	m.ok("", "run", "submit", dir)
	m.ok("", "run", "send", "/house/room#a", "said")
	m.ok("", "run", "send", "/house/room#b", "said")
	a, b := shownAt(t, m, "/house/room#a"), shownAt(t, m, "/house/room#b")
	out := m.ok("", "run", "cancel", "/house/room#a")
	if !strings.Contains(out, a+"  taken") || strings.Contains(out, b) {
		t.Fatalf("cancel of /house/room#a printed %s", out)
	}
	until(t, "the cancel", func() bool { return strings.Contains(m.ok("", "run", "show", a), "cancelled") })
	if shown := m.ok("", "run", "show", b); !strings.Contains(shown, "running") {
		t.Fatalf("the run of key b was touched: %s", shown)
	}
	// The list of running runs drops a cancelled run a moment after its end.
	until(t, "the key's run to leave the running list", func() bool {
		status, _, _ := m.run("", "run", "show", "/house/room#a")
		return status != 0
	})
	status, out, said := m.run("", "run", "cancel", "/house/room#a")
	if status != 1 || out != "" || !strings.Contains(said, "no run of this client's is running at /house/room#a") || strings.Contains(said, "usage") {
		t.Fatalf("a cancel of a key with no run ended %d: %s%s", status, out, said)
	}
}

// shownAt waits for a running run at a location, which a send or a submit
// has started but the controller may not have taken yet, and gives its ID.
func shownAt(t *testing.T, m machine, at string) string {
	t.Helper()
	var shown string
	until(t, "a run at "+at, func() bool {
		status, out, _ := m.run("", "run", "show", at)
		shown = out
		return status == 0 && strings.Contains(out, "running")
	})
	return strings.Fields(shown)[0]
}

// run show and run cancel read an argument that starts with / as a
// location, even when a directory of that name is on this computer.
func Test_client_location_that_is_a_path_on_disk(t *testing.T) {
	m, _ := working(t, "all")
	dir := filepath.Join(t.TempDir(), "tmp")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "room.yaml"), []byte(strings.Replace(room, "keyed", "singleton", 1)), 0o644)
	// /tmp is a directory on every computer the tests run on.
	m.ok("", "run", "submit", dir, "--at", "/tmp")
	// The singleton the submit started may not have been taken yet.
	shownAt(t, m, "/tmp/room")
	if out := m.ok("", "run", "cancel", "/tmp/room"); !strings.Contains(out, "taken") {
		t.Fatalf("run cancel /tmp/room printed %s", out)
	}
}

// A submit whose files have problems fails whatever the output's shape, and
// its JSON gives each problem's text.
func Test_client_submit_problems_in_json(t *testing.T) {
	m, _ := working(t, "all")
	dir := filepath.Join(t.TempDir(), "bad")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("flow: a\nlimits: { runs: 2 }\nstages:\n  s: { job: { script: ./x.js } }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "x.js"), []byte("return 1"), 0o644)
	status, out, said := m.run("", "run", "submit", dir, "--json")
	if status != 1 || !strings.Contains(said, "1 of 1 files have problems") {
		t.Fatalf("a submit with --json of a bad file ended %d: %s%s", status, out, said)
	}
	var printed struct {
		Locations []map[string]any
		Problems  map[string]string
	}
	if err := json.Unmarshal([]byte(out), &printed); err != nil {
		t.Fatalf("the submit printed %s", out)
	}
	if !strings.Contains(printed.Problems["/bad/a"], "limits.runs") {
		t.Fatalf("the JSON gives the problems %v", printed.Problems)
	}
}

// A run's record prints a line for each call job, naming the run it
// started, and that run prints the caller's step as its cause; a reply to
// a client names the client; and a note, written after the run ended or
// not, is printed once, with the run's status once.
func Test_client_record_names_calls_clients_and_notes(t *testing.T) {
	m, _ := working(t, "all")
	dir := filepath.Join(t.TempDir(), "t")
	os.Mkdir(dir, 0o755)
	for name, text := range map[string]string{
		"w.yaml":     "flow: w\nstages:\n  a: { job: { script: ./five.js } }\noutput: ${stages.a.output}\n",
		"five.js":    "return 5",
		"c.yaml":     "flow: c\nallow: [send:w]\nstages:\n  c: { job: { call: { at: w } } }\noutput: ${stages.c.output}\n",
		"picky.yaml": strings.Replace(room, "lifetime: keyed", "lifetime: keyed\nreceive: [client:nobody]", 1),
		"poke.yaml":  "flow: poke\nallow: [send:picky]\nstages:\n  p: { job: { send: { to: { at: picky, key: k1 }, event: said } } }\n",
		"echo.yaml":  "statechart: echo\nlifetime: singleton\ninitial: open\nstates:\n  open: { on: { ping: { go: open, send: { to: \"${event.from}\", event: pong } } } }\n",
	} {
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	m.ok("", "run", "submit", dir)

	id := strings.TrimSpace(m.ok("", "run", "submit", "/t/c"))
	m.ok("", "run", "show", id, "--wait")
	out := m.ok("", "run", "show", id, "--record")
	called := regexp.MustCompile(`-> job c: a call to /t/w, (box-client/\S+)`).FindStringSubmatch(out)
	if called == nil {
		t.Fatalf("the caller's record printed:\n%s", out)
	}
	if shown := m.ok("", "run", "show", called[1]); !strings.Contains(shown, "  cause: "+id+" step 1\n") {
		t.Errorf("the called run printed:\n%s", shown)
	}

	id = strings.TrimSpace(m.ok("", "run", "submit", "/t/poke"))
	m.ok("", "run", "show", id, "--wait")
	until(t, "the note", func() bool {
		out = m.ok("", "run", "show", id, "--record")
		return strings.Contains(out, "was not delivered")
	})
	if strings.Count(out, "was not delivered") != 1 || strings.Count(out, "  done\n") != 1 {
		t.Errorf("the ended sender's record printed:\n%s", out)
	}

	m.ok("", "run", "send", "/t/echo", "ping")
	until(t, "the reply", func() bool {
		_, out, _ = m.run("", "run", "show", "/t/echo", "--record")
		return strings.Contains(out, "pong")
	})
	if !strings.Contains(out, "-> send pong to client:box-client · undelivered: a client has no inbox") {
		t.Errorf("the reply to a client printed:\n%s", out)
	}
}

// A value or secret is set at a location, which holds no *.
func Test_client_value_at_a_pattern_refused(t *testing.T) {
	m, _ := working(t, "all")
	for _, at := range []string{"/home/*", "/home/**"} {
		if status, _, said := m.run("", "controller", "value", "set", "HUB", "x", "--at", at); status != 2 || !strings.Contains(said, "holds # or *") {
			t.Errorf("value set --at %s ended %d: %s", at, status, said)
		}
	}
	if status, _, said := m.run("", "controller", "value", "set", "bad name", "x"); status != 2 || !strings.Contains(said, "a name holds no @") {
		t.Errorf("value set of a bad name ended %d: %s", status, said)
	}
}

// A run in a tree of another client is no run of this client's: show and
// send say whose tree it is in, and fail with no usage line.
func Test_client_run_of_another_tree_named(t *testing.T) {
	m, _ := working(t, "all")
	id := "another-client/" + time.Now().UTC().Format("20060102T150405.000Z") + "-0000000000"
	for _, line := range [][]string{{"run", "show", id}, {"run", "show", id, "--record"}, {"run", "send", id, "ping"}} {
		status, out, said := m.run("", line...)
		if status != 1 || out != "" || !strings.Contains(said, "no such run of this client's: the run is in a tree of the client another-client") || strings.Contains(said, "usage") {
			t.Errorf("corbie %v ended %d: %s%s", line, status, out, said)
		}
	}
}
