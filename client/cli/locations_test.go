package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// A submit prints changed for a changed keyed or transient file, restarted
// and the new run for a changed singleton, recorded for an unchanged file,
// and each run it cancelled.
func TestSubmitPrintsWhatChanged(t *testing.T) {
	m, _ := working(t, "all")
	steady := "statechart: steady\nlifetime: singleton\ninitial: waiting\nstates:\n  waiting: {}\n"
	waits := "statechart: waits\ninitial: w\nstates:\n  w: {}\n"
	dir := files(t, map[string]string{"steady.yaml": steady, "waits.yaml": waits, "same.yaml": waits})
	first := regexp.MustCompile(`started (\S+)`).FindStringSubmatch(m.ok("", "run", "submit", dir, "--at", "/d"))
	if first == nil {
		t.Fatal("the first submit started no singleton")
	}
	for name, text := range map[string]string{"steady.yaml": steady + "title: changed\n", "waits.yaml": waits + "title: changed\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := m.ok("", "run", "submit", dir, "--at", "/d")
	want := regexp.MustCompile(`^/d/same     transient   recorded\n/d/steady   singleton   restarted box-client/\S+\n/d/waits    transient   changed\ncancelled: ` + regexp.QuoteMeta(first[1]) + "\n$")
	if !want.MatchString(out) || strings.Contains(out, "restarted "+first[1]) {
		t.Fatalf("the second submit printed:\n%s", out)
	}
}

// A file given alone is a tree of one file, whose location is its prefix,
// and a location given is started there, with the run's ID printed and
// nothing else.
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
	// A start prints the run's ID and nothing else.
	status, out, said := m.run("", "run", "start", "/hello", "--input", `{"name": "world"}`)
	if status != 0 || !regexp.MustCompile(`^box-client/\d{8}T\d{6}\.\d{3}Z-[0-9a-f]{10}\n$`).MatchString(out) || said != "" {
		t.Fatalf("a start of /hello ended %d, printed %q and said %q", status, out, said)
	}
	// A location with the name of a directory on this computer is started
	// as any other, and the directory is not read.
	if runtime.GOOS != "windows" {
		m.ok("", "run", "submit", filepath.Join(dir, "hello.yaml"), "--at", dir)
		if out := m.ok("", "run", "start", dir, "--input", `{"name": "world"}`, "--wait"); !strings.Contains(out, `output: "hello, world"`) {
			t.Fatalf("a start of the location %s, which is a directory here, printed:\n%s", dir, out)
		}
	}
	id := strings.TrimSpace(out)
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
		{"flag provided but not defined: -at", []string{"run", "start", "/hello", "--at", "/x"}},
		{"--at /: a prefix is / and the names", []string{"run", "submit", filepath.Join(dir, "hello.yaml"), "--at", "/"}},
	} {
		if status, _, said := m.run("", mistake.args...); status != 2 || !strings.Contains(said, mistake.says) {
			t.Errorf("corbie %s ended %d, and said:\n%s", strings.Join(mistake.args, " "), status, said)
		}
	}
	// A file's name that is not there is no location: nothing is sent.
	if status, out, said := m.run("", "run", "submit", filepath.Join(dir, "helo.yaml")); status == 0 || out != "" || !strings.Contains(said, "no such file") {
		t.Errorf("a file that is not there ended %d, printed %q and said:\n%s", status, out, said)
	}
	// A file with a problem is a tree with nothing to record: nothing is sent.
	if status, out, said := m.run("", "run", "submit", filepath.Join(dir, "broken.yaml")); status != 1 || out != "" || !strings.Contains(said, "stages") {
		t.Errorf("a file with a problem submitted alone ended %d, printed %q and said:\n%s", status, out, said)
	}
	// A start where nothing is recorded is refused, with --wait and without.
	for _, wait := range [][]string{nil, {"--wait"}} {
		if status, out, said := m.run("", append([]string{"run", "start", "/broken"}, wait...)...); status != 1 || !strings.HasPrefix(out, "refused: nothing is recorded at /broken") || said != "" {
			t.Errorf("a start of a location where nothing was recorded, with %v, ended %d, printed:\n%s\nand said:\n%s", wait, status, out, said)
		}
	}
	// A start a permission refuses prints what was refused, the layer and
	// the fix.
	other := m.second("other", "network")
	if status, out, said := other.run("", "run", "start", "/hello"); status != 1 || !regexp.MustCompile(`^refused: send:/hello\n  by: .+\n  fix: .+\n$`).MatchString(out) || said != "" {
		t.Errorf("a start the ceiling refuses ended %d, printed:\n%s\nand said:\n%s", status, out, said)
	}
}
