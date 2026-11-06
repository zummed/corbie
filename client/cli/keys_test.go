package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// A send to a key no run has fails, saying so.
func Test_client_a_key_on_the_command_line(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box", "--ceiling", "all")
	if status, _, said := m.run("", "run", "send", "--key", "back door", "open"); status == 0 || !strings.Contains(said, "no run has the key") {
		t.Fatalf("a send to a key nobody has ended %d: %s", status, said)
	}
	if status, _, said := m.run("", "run", "send", "--key", "", "open"); status == 0 || !strings.Contains(said, "not empty") {
		t.Fatalf("a send to an empty key ended %d: %s", status, said)
	}
}

// run show prints a run's title, and as JSON gives it as title. A listing,
// which reads heads, has none.
func Test_client_run_show_prints_a_title(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box", "--ceiling", "all")
	m.staying("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms")
	dir := files(t, map[string]string{"door.yaml": `statechart: door
title: "${input.channel}: ${input.said}"
initial: closed
states:
  closed: { on: { open: opened } }
  opened: { end: done }
`})
	id := strings.TrimSpace(m.ok("", "run", "submit", m.recorded(filepath.Join(dir, "door.yaml")), "--input", `{"channel": "#testing", "said": "give me a release card"}`))
	until(t, "the run", func() bool { return strings.Contains(m.ok("", "run", "ls"), id) })
	if out := m.ok("", "run", "show", id); !strings.Contains(out, "\n  title: #testing: give me a release card\n") {
		t.Fatalf("corbie run show printed %s", out)
	}
	if out := m.ok("", "run", "show", id, "--json"); !strings.Contains(out, `"title":"#testing: give me a release card"`) {
		t.Fatalf("corbie run show --json printed %s", out)
	}
	if out := m.ok("", "run", "ls", "--json"); strings.Contains(out, "title") {
		t.Fatalf("corbie run ls --json printed a title: %s", out)
	}
}
