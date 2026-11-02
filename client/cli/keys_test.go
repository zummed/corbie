package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// A run takes a key on the command line, shows it, and is sent to by it.
func Test_client_a_key_on_the_command_line(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box", "--ceiling", "all")
	m.staying("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms")
	dir := files(t, map[string]string{"door.yaml": `statechart: door
initial: closed
states:
  closed: { on: { open: opened } }
  opened: { end: done, output: "${run.key}" }
`})
	door := filepath.Join(dir, "door.yaml")
	id := strings.TrimSpace(m.ok("", "run", "submit", door, "--key", "front door"))
	until(t, "the run", func() bool { return strings.Contains(m.ok("", "run", "ls"), `key "front door"`) })
	if out := m.ok("", "run", "show", id); !strings.Contains(out, `key: "front door"`) {
		t.Fatalf("corbie run show printed %s", out)
	}
	if status, _, said := m.run("", "run", "submit", door, "--key", ""); status != 2 || !strings.Contains(said, "not empty") {
		t.Fatalf("an empty key ended %d: %s", status, said)
	}
	if status, _, said := m.run("", "run", "send", "--key", "back door", "open"); status == 0 || !strings.Contains(said, "no run has the key") {
		t.Fatalf("a send to a key nobody has ended %d: %s", status, said)
	}
	if out := m.ok("", "run", "send", "--key", "front door", "open"); !strings.Contains(out, "acted on it") {
		t.Fatalf("corbie run send --key printed %s", out)
	}
	if out := m.ok("", "run", "show", id, "--wait"); !strings.Contains(out, `output: "front door"`) {
		t.Fatalf("the run ended as %s", out)
	}
	if status, _, said := m.run("", "run", "send", "--key", "front door", "open"); status == 0 || !strings.Contains(said, "no run has the key") {
		t.Fatalf("a send to a key whose run ended ended %d: %s", status, said)
	}
}
