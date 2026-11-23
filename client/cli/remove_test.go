package cli_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// corbie run remove removes a tree: it prints each location, the run it
// cancelled and the secret and the value it removed, and the prefix then
// holds no tree. What is set above the prefix stays.
func TestRemoveRemovesATree(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{
		"home/devices/boiler.yaml": "statechart: boiler\nlifetime: singleton\ninitial: idle\nstates:\n  idle:\n    on: { stop: off }\n  off: { end: done }\n",
		"home/hello.yaml":          hello,
		"home/greet.js":            `return "hello, " + input.name`,
	})
	out := m.ok("", "run", "submit", filepath.Join(dir, "home"))
	started := regexp.MustCompile(`/home/devices/boiler   singleton   started (box-client/\S+)`).FindStringSubmatch(out)
	if started == nil {
		t.Fatalf("the submit printed %s", out)
	}
	m.ok("the password\n", "client", "secret", "set", "BOILER_PASSWORD", "-", "--at", "/home/devices/boiler")
	m.ok("", "client", "value", "set", "HOST", "boiler.lan", "--at", "/home")
	m.ok("", "controller", "value", "set", "HOST", "elsewhere", "--at", "/")
	want := "/home/devices/boiler   singleton   removed\n" +
		"/home/hello            transient   removed\n" +
		"cancelled: " + started[1] + "\n" +
		"removed: value HOST at /home\n" +
		"removed: secret BOILER_PASSWORD at /home/devices/boiler\n"
	if out := m.ok("", "run", "remove", "/home"); out != want {
		t.Fatalf("the removal printed:\n%s\nand not:\n%s", out, want)
	}
	if shown := m.ok("", "run", "show", started[1]); !strings.Contains(shown, "  cancelled  ") {
		t.Fatalf("after the removal the boiler is %s", shown)
	}
	if out := m.ok("", "controller", "value", "list"); strings.Contains(out, "boiler.lan") || !strings.Contains(out, "elsewhere") {
		t.Fatalf("the values are now:\n%s", out)
	}
	if out := m.ok("", "client", "secret", "list"); strings.Contains(out, "BOILER_PASSWORD") {
		t.Fatalf("the secrets are now:\n%s", out)
	}
	if status, out, said := m.run("", "run", "start", "/home/hello", "--input", `{"name": "x"}`, "--wait"); status == 0 || !strings.Contains(out+said, "nothing is recorded at /home/hello") {
		t.Fatalf("a start at a removed location ended %d, printed %s and said %s", status, out, said)
	}
	if out := m.ok("", "run", "remove", "/home"); out != "nothing was recorded or set at /home or under it\n" {
		t.Fatalf("a second removal printed %q", out)
	}
	// A tree is inside a prefix above it, and is submitted there afterwards.
	if out := m.ok("", "run", "submit", filepath.Join(dir, "home"), "--at", "/home/again"); !strings.Contains(out, "/home/again/devices/boiler   singleton   started") {
		t.Fatalf("a submit under the removed prefix printed %s", out)
	}
	for _, args := range [][]string{{"run", "remove"}, {"run", "remove", "/"}, {"run", "remove", "home"}, {"run", "remove", "/a", "/b"}} {
		if status, _, said := m.run("", args...); status != 2 || !strings.Contains(said, "name the prefix of one tree") {
			t.Errorf("corbie %s ended %d and said %s", strings.Join(args, " "), status, said)
		}
	}
}
