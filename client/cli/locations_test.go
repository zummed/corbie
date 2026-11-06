package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// A file given alone is a tree of one file, whose location is its prefix,
// and a location given is started there.
func TestSubmitRecordsAFileAndStartsALocation(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{
		"hello.yaml": hello,
		"greet.js":   `return "hello, " + input.name`,
		"steady.yaml": `statechart: steady
lifetime: singleton
initial: waiting
states:
  waiting: {}
`,
		"broken.yaml": "flow: broken\nstages: 3\n",
	})
	if out := m.ok("", "run", "submit", filepath.Join(dir, "hello.yaml")); out != "/hello   transient   recorded\n" {
		t.Fatalf("a transient file submitted alone printed %q", out)
	}
	id := strings.TrimSpace(m.ok("", "run", "submit", "/hello", "--input", `{"name": "world"}`))
	if out := m.ok("", "run", "show", id, "--wait"); !strings.Contains(out, `output: "hello, world"`) {
		t.Fatalf("the run started at /hello printed:\n%s", out)
	}
	if out := m.ok("", "run", "submit", filepath.Join(dir, "steady.yaml"), "--at", "/standing/steady"); !strings.HasPrefix(out, "/standing/steady   singleton   started box-client/") {
		t.Fatalf("a singleton submitted alone at --at printed %q", out)
	}
	for _, mistake := range []struct {
		says string
		args []string
	}{
		{"--key does not apply to a start of a location", []string{"run", "submit", "/hello", "--key", "k"}},
		{"--secret-env does not apply to a start of a location", []string{"run", "submit", "/hello", "--secret-env", "HOME"}},
		{"--at applies to a file or a directory, and /hello is a location", []string{"run", "submit", "/hello", "--at", "/x"}},
		{"--input applies to a start; a file given with --at is recorded as a tree", []string{"run", "submit", filepath.Join(dir, "hello.yaml"), "--at", "/x", "--input", "{}"}},
		{"--at /: a prefix is / and the names", []string{"run", "submit", filepath.Join(dir, "hello.yaml"), "--at", "/"}},
	} {
		if status, _, said := m.run("", mistake.args...); status != 2 || !strings.Contains(said, mistake.says) {
			t.Errorf("corbie %s ended %d, and said:\n%s", strings.Join(mistake.args, " "), status, said)
		}
	}
	// A file with a problem is a tree with nothing to record: nothing is sent.
	if status, out, said := m.run("", "run", "submit", filepath.Join(dir, "broken.yaml")); status != 1 || out != "" || !strings.Contains(said, "stages") {
		t.Errorf("a file with a problem submitted alone ended %d, printed %q and said:\n%s", status, out, said)
	}
	if status, out, said := m.run("", "run", "submit", "/broken", "--wait"); status != 1 || !strings.Contains(out+said, "/broken") {
		t.Errorf("a start of a location where nothing was recorded ended %d, printed:\n%s\nand said:\n%s", status, out, said)
	}
}
