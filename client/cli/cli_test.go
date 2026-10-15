package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/manual"
	"github.com/zummed/corbie/runner"

	_ "github.com/zummed/corbie/agent"
	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
	_ "github.com/zummed/corbie/statechart"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	os.Exit(m.Run())
}

// machine is one machine's config, and a way to run commands on it.
type machine struct {
	t      *testing.T
	config string
}

// run runs one command and gives its exit status, its result and what it
// said along the way.
func (m machine) run(stdin string, args ...string) (int, string, string) {
	m.t.Helper()
	var out, said bytes.Buffer
	full := append(args, "--config", m.config)
	// Not a deadline: a runner takes a context's deadline for its host's
	// limit, and takes no job it could not finish before it.
	ctx, cancel := context.WithCancel(context.Background())
	defer time.AfterFunc(30*time.Second, cancel).Stop()
	defer cancel()
	status := cli.Run(ctx, full, strings.NewReader(stdin), &out, &said)
	return status, out.String(), said.String()
}

func (m machine) ok(stdin string, args ...string) string {
	m.t.Helper()
	status, out, said := m.run(stdin, args...)
	if status != 0 {
		m.t.Fatalf("corbie %s ended %d:\n%s%s", strings.Join(args, " "), status, out, said)
	}
	return out
}

func files(t *testing.T, contents map[string]string) string {
	dir := t.TempDir()
	for name, text := range contents {
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	return dir
}

const hello = `flow: hello
input: [name]
stages:
  greet:
    job: { script: ./greet.js, input: { name: "${input.name}" } }
output: ${stages.greet.output}
`

func TestTestNeedsNothingSetUp(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return { text: "hello " + input.name }`})
	out := m.ok("", "test", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "world"}`, "--json")
	var run map[string]any
	if err := json.Unmarshal([]byte(out), &run); err != nil || run["status"] != "done" || run["output"].(map[string]any)["text"] != "hello world" {
		t.Fatalf("corbie test printed %s", out)
	}
	if entries, _ := os.ReadDir(m.config); len(entries) != 0 {
		t.Fatal("corbie test left a config behind")
	}
	// A run that does not end well fails the command.
	bad := files(t, map[string]string{"hello.yaml": hello, "greet.js": `throw new Error("no")`})
	if status, _, _ := m.run("", "test", filepath.Join(bad, "hello.yaml"), "--input", `{"name": "x"}`); status != 1 {
		t.Fatalf("a failed run ended the command with %d", status)
	}
	// A definition with mistakes is a failure that lists them, and a flag
	// written wrong is a mistake in how the command was written.
	wrong := files(t, map[string]string{"hello.yaml": strings.Replace(hello, "stages.greet", "stages.missing", 1), "greet.js": ""})
	if status, _, said := m.run("", "test", filepath.Join(wrong, "hello.yaml")); status != 1 || !strings.Contains(said, `no stage named "missing"`) {
		t.Fatalf("a wrong definition: %d %s", status, said)
	}
	if status, _, _ := m.run("", "test", filepath.Join(dir, "hello.yaml"), "--no-such-flag"); status != 2 {
		t.Fatalf("an unknown flag ended the command with %d", status)
	}
}

func TestAScriptFileIsSubmittedAsItIs(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{"double.js": `return input.n * 2`, "reads.js": `return fs.read("/etc/hostname")`})
	out := m.ok("", "test", filepath.Join(dir, "double.js"), "--input", `{"n": 21}`, "--json")
	if !strings.Contains(out, `"output":42`) {
		t.Fatalf("a script file printed %s", out)
	}
	// With nothing allowed on the command line, the script may do nothing
	// outside itself.
	if status, out, _ := m.run("", "test", filepath.Join(dir, "reads.js")); status != 1 || !strings.Contains(out, "denied") {
		t.Fatalf("a script allowed nothing printed (%d) %s", status, out)
	}
}

func TestInitRunSubmit(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	out := m.ok("", "init", "--store", storeDir, "--name", "box")
	for _, want := range []string{"box-controller", "box-runner", "box-client", "A directory keeps no party out", "The ceiling is empty"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init printed nothing of %q:\n%s", want, out)
		}
	}
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return { text: "hello " + input.name }`})

	// With the controller stopped, a submitted run is waiting to start.
	id := strings.TrimSpace(m.ok("", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "directory"}`, "--label", "team=ops"))
	if strings.ContainsAny(id, " \n") || !strings.HasPrefix(id, "box-client/") {
		t.Fatalf("submit printed %q", id)
	}
	if out := m.ok("", "show", id); !strings.Contains(out, "waiting") {
		t.Fatalf("before any controller ran, show printed %s", out)
	}

	roles := make(chan int, 1)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		var out, said bytes.Buffer
		roles <- cli.Run(ctx, []string{"run", "controller", "runner", "--interval", "200ms", "--config", m.config}, strings.NewReader(""), &out, &said)
	}()
	if out := m.ok("", "show", id, "--wait", "--json"); !strings.Contains(out, `"status":"done"`) || !strings.Contains(out, "hello directory") {
		t.Fatalf("the run ended as %s", out)
	}
	// A row of the listing shows the run's labels.
	if out := m.ok("", "ls", "--all", "--label", "team=ops"); !strings.Contains(out, id) || !strings.Contains(out, "  labels: team=ops\n") {
		t.Fatalf("ls printed %s", out)
	}
	if out := m.ok("", "show", id, "--definition"); !strings.Contains(out, `"name":"hello"`) {
		t.Fatalf("the definition printed %s", out)
	}
	// A run that does not exist is a failure that says so.
	if status, _, said := m.run("", "show", "box-client/20200101T000000.000Z-0000000000"); status != 1 || !strings.Contains(said, "no such run") {
		t.Fatalf("no such run: %d %s", status, said)
	}
	// With no ceiling, a script that reads a file is denied, and the fix
	// names the command that changes it. The script's own list allowed it,
	// so nothing is said of adding to the command line.
	reads := files(t, map[string]string{"reads.js": `return fs.read("/etc/hostname")`})
	_, out, _ = m.run("", "submit", filepath.Join(reads, "reads.js"), "--allow", "disk", "--wait")
	if !strings.Contains(out, "denied") || !strings.Contains(out, "corbie controller enrol box-client --allow fs.read:/etc/hostname") || strings.Contains(out, "add to the command") {
		t.Fatalf("a denied run printed %s", out)
	}
	m.ok("", "controller", "enrol", "box-client", "--allow", "fs.read:/etc/hostname")
	m.ok("", "runner", "trust", "box-controller", "--allow", "fs.read:/etc/hostname")
	if _, out, _ := m.run("", "submit", filepath.Join(reads, "reads.js"), "--allow", "disk", "--wait"); !strings.Contains(out, "done") {
		t.Fatalf("after the ceilings were raised: %s", out)
	}
	stop()
	<-roles
}

// The three steps of enrolling across machines join into one line.
func TestEnrolPipe(t *testing.T) {
	ctl := machine{t, filepath.Join(t.TempDir(), "ctl")}
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	storeDir := filepath.Join(t.TempDir(), "store")
	ctl.ok("", "controller", "card", "--store", storeDir, "--name", "ctl")

	_, card, said := laptop.run("", "runner", "card", "--name", "laptop")
	if strings.Count(card, "\n") != 1 || !strings.HasPrefix(card, "corbie-card:runner:laptop:") || !strings.Contains(said, "laptop") {
		t.Fatalf("runner card printed %q and said %q", card, said)
	}
	_, enrolled, said := ctl.run(card, "controller", "enrol", "-", "--tags", "laptop")
	lines := strings.Split(strings.TrimSpace(enrolled), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "corbie-card:controller:ctl:") || lines[1] != storeDir {
		t.Fatalf("enrol printed %q (said %q)", enrolled, said)
	}
	laptop.ok(enrolled, "runner", "trust", "-", "--ceiling", "disk,exec")
	if out := ctl.ok("", "controller", "parties"); !strings.Contains(out, "laptop") || !strings.Contains(out, "tags: laptop") {
		t.Fatalf("the controller's parties: %s", out)
	}
	// Enrolled again by name with no tags named, the tags stay, and only
	// --tags none takes them away.
	ctl.ok("", "controller", "enrol", "laptop")
	if out := ctl.ok("", "controller", "parties"); !strings.Contains(out, "tags: laptop") {
		t.Fatalf("enrolled again with nothing named, the runner's tags are gone: %s", out)
	}
	ctl.ok("", "controller", "enrol", "laptop", "--tags", "none")
	if out := ctl.ok("", "controller", "parties"); strings.Contains(out, "tags: laptop") {
		t.Fatalf("with --tags none, the runner still has its tags: %s", out)
	}
	ctl.ok("", "controller", "enrol", "laptop", "--tags", "laptop")
	// A step given empty input trusts nothing and says what it expected.
	if status, _, said := laptop.run("", "runner", "trust", "-"); status != 2 || !strings.Contains(said, "was expected") {
		t.Fatalf("trust on empty input: %d %s", status, said)
	}
	// Another key under a used name is refused unless it replaces.
	other := machine{t, filepath.Join(t.TempDir(), "other")}
	_, otherCard, _ := other.run("", "runner", "card", "--name", "laptop")
	if status, out, said := ctl.run(otherCard, "controller", "enrol", "-"); status != 1 || out != "" || !strings.Contains(said, "already used with another key") {
		t.Fatalf("a different key under a used name: %d %q %s", status, out, said)
	}
	ctl.ok(otherCard, "controller", "enrol", "-", "--replace")
	// A client's card where a controller's is needed is refused.
	_, clientCard, _ := laptop.run("", "client", "card", "--name", "someone")
	if status, _, said := laptop.run(clientCard+" "+storeDir, "runner", "trust", "-"); status != 1 || !strings.Contains(said, "controller's is needed") {
		t.Fatalf("a client's card given to runner trust: %d %s", status, said)
	}
}

func TestManualAndHelp(t *testing.T) {
	m := machine{t, t.TempDir()}
	listing := m.ok("", "manual")
	// Every page the list names can be printed, and everything the binary
	// holds has a page.
	for _, page := range manual.Pages() {
		if !strings.Contains(listing, page.Name) {
			t.Errorf("the page %s is not in the list", page.Name)
		}
		if out := m.ok("", "manual", page.Name); len(out) < 40 {
			t.Errorf("the page %s prints %q", page.Name, out)
		}
	}
	for _, want := range []string{"flow", "statechart", "agent", "http", "blobs", "corbie submit", "permissions"} {
		if _, ok := manual.Find(want); !ok {
			t.Errorf("there is no page for %s", want)
		}
	}
	if status, _, _ := m.run("", "manual", "no-such-page"); status != 2 {
		t.Fatalf("a page that is not one ended %d", status)
	}
	for i := range cli.Commands {
		c := &cli.Commands[i]
		status, out, _ := m.run("", append(strings.Fields(c.Name), "--help")...)
		if status != 0 || !strings.Contains(out, c.Summary) {
			t.Errorf("corbie %s --help: %d %q", c.Name, status, out)
		}
	}
	if status, out, _ := m.run("", "help"); status != 0 || !strings.Contains(out, "corbie submit") {
		t.Fatalf("corbie help: %d", status)
	}
	if status, _, _ := m.run("", "frobnicate"); status != 2 {
		t.Fatalf("an unknown command ended %d", status)
	}
}

func TestModelServer(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{"double.js": `return input.n * 2`})
	calls := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"manual","arguments":{"page":"flow"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"corbie","arguments":{"args":["test","` + filepath.Join(dir, "double.js") + `","--input","{\"n\":4}"]}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"corbie","arguments":{"args":["init","--store","/tmp/x"]}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"corbie","arguments":{"args":["test","x.js","--store","/elsewhere"]}}}`,
	}, "\n") + "\n"
	out := m.ok(calls, "mcp", "--allow", "test")
	// Calls run side by side and are answered as each ends: an answer
	// begins with the ID of its call, and they are read in that order.
	answers := strings.Split(strings.TrimSpace(out), "\n")
	slices.Sort(answers)
	if len(answers) != 6 {
		t.Fatalf("six calls were answered %d times:\n%s", len(answers), out)
	}
	if !strings.Contains(answers[1], "corbie test") || strings.Contains(answers[1], "corbie init ") || !strings.Contains(answers[1], "stays until the run ends") {
		t.Fatalf("the tools were described as %s", answers[1])
	}
	if !strings.Contains(answers[2], `"text":"Flow: a flow `) {
		t.Fatalf("the manual tool gave %s", answers[2])
	}
	if !strings.Contains(answers[3], `\"output\\\":8`) && !strings.Contains(answers[3], `output\\\":8`) {
		t.Fatalf("the command tool gave %s", answers[3])
	}
	if !strings.Contains(answers[4], "not one this server was started with permission for") || !strings.Contains(answers[4], `"isError":true`) {
		t.Fatalf("a command not allowed was answered %s", answers[4])
	}
	if !strings.Contains(answers[5], "may not name a config or a store") {
		t.Fatalf("a call naming a store was answered %s", answers[5])
	}
}

// enrolled sets up a controller on one config and gives it and the
// address of its store.
func controllerOn(t *testing.T) (machine, string) {
	ctl := machine{t, filepath.Join(t.TempDir(), "ctl")}
	storeDir := filepath.Join(t.TempDir(), "store")
	ctl.ok("", "controller", "card", "--store", storeDir, "--name", "ctl")
	return ctl, storeDir
}

func Test_id_enrol_pipe_output_is_only_what_next_step_reads(t *testing.T) {
	ctl, storeDir := controllerOn(t)
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	_, card, said := laptop.run("", "runner", "card", "--name", "laptop")
	// The card alone on standard output; the fingerprint beside it.
	if strings.TrimSpace(card) != strings.Fields(card)[0] || !strings.Contains(said, "-") || strings.Contains(card, "(") {
		t.Fatalf("runner card printed %q and said %q", card, said)
	}
	_, enrolled, said := ctl.run(card, "controller", "enrol", "-")
	if fields := strings.Fields(enrolled); len(fields) != 2 || fields[1] != storeDir || !strings.Contains(said, "enrolled the runner laptop") {
		t.Fatalf("enrol printed %q and said %q", enrolled, said)
	}
	// Both showings of each card's fingerprint agree.
	_, _, again := laptop.run("", "runner", "card")
	fingerprint := said[strings.Index(said, "(")+1 : strings.Index(said, ")")]
	if !strings.Contains(again, fingerprint) {
		t.Fatalf("the runner's fingerprint was shown as %q by the controller and as %q by the runner", fingerprint, again)
	}
}

func Test_id_enrol_prints_nothing_when_trust_not_written(t *testing.T) {
	ctl, storeDir := controllerOn(t)
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	_, card, _ := laptop.run("", "runner", "card", "--name", "laptop")
	// The store goes out of reach: a file stands where its directory was.
	os.RemoveAll(storeDir)
	os.WriteFile(storeDir, []byte("not a directory"), 0o600)
	status, out, _ := ctl.run(card, "controller", "enrol", "-")
	if status == 0 || out != "" {
		t.Fatalf("with the store out of reach enrol ended %d and printed %q", status, out)
	}
}

func Test_id_enrol_refuses_the_controllers_own_name(t *testing.T) {
	ctl, storeDir := controllerOn(t)
	other := machine{t, filepath.Join(t.TempDir(), "other")}
	_, card, _ := other.run("", "runner", "card", "--name", "ctl")
	status, out, said := ctl.run(card, "controller", "enrol", "-")
	if status == 0 || out != "" || !strings.Contains(said, "controller's own name") {
		t.Fatalf("enrol of the controller's own name ended %d, printed %q and said %q", status, out, said)
	}
	filepath.WalkDir(storeDir, func(path string, _ os.DirEntry, _ error) error {
		if strings.Contains(path, "/trust/") {
			t.Fatalf("trust was written: %s", path)
		}
		return nil
	})
}

func Test_id_enrol_prints_address_without_credentials(t *testing.T) {
	ctl, storeDir := controllerOn(t)
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	_, card, _ := laptop.run("", "client", "card", "--name", "laptop")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "the-controllers-own-key")
	_, enrolled, said := ctl.run(card, "controller", "enrol", "-")
	if strings.Contains(enrolled+said, "the-controllers-own-key") || !strings.Contains(enrolled, storeDir) {
		t.Fatalf("enrol printed %q", enrolled)
	}
}

func Test_id_trust_on_empty_input_trusts_nothing(t *testing.T) {
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	laptop.ok("", "runner", "card", "--name", "laptop")
	status, _, said := laptop.run("", "runner", "trust", "-")
	if status != 2 || !strings.Contains(said, "was expected") {
		t.Fatalf("trust on empty input ended %d: %s", status, said)
	}
	if entries, _ := os.ReadDir(filepath.Join(laptop.config, "runner", "trust")); len(entries) != 0 {
		t.Fatalf("empty input left trust behind: %v", entries)
	}
}

// The commands of a pipe start together: the last step reads its input to
// the end before it looks at its own config.
func Test_id_trust_reads_input_before_config(t *testing.T) {
	ctl, _ := controllerOn(t)
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	_, card, _ := laptop.run("", "client", "card", "--name", "laptop")
	_, enrolled, _ := ctl.run(card, "controller", "enrol", "-")
	// A config that does not exist yet when trust starts.
	fresh := machine{t, filepath.Join(t.TempDir(), "not", "there", "yet")}
	fresh.ok(enrolled, "client", "trust", "-")
	if _, err := os.Stat(filepath.Join(fresh.config, "client", "trust", "ctl")); err != nil {
		t.Fatalf("trust was not written: %v", err)
	}
}

// Seeing where runs stand needs no identity: the controller's card and
// the store's address are enough.
func TestViewerNeedsNoIdentity(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`})
	id := strings.TrimSpace(m.ok("", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`))
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		var out, said bytes.Buffer
		done <- cli.Run(ctx, []string{"run", "controller", "runner", "--interval", "100ms", "--config", m.config}, strings.NewReader(""), &out, &said)
	}()
	m.ok("", "show", id, "--wait")
	// The record says the run has ended a moment before its head does:
	// let the controller finish the step before it is stopped.
	for range 200 {
		if strings.Contains(m.ok("", "ls", "--all"), "done") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done

	card := strings.Fields(m.ok("", "controller", "card"))[0]
	viewer := machine{t, filepath.Join(t.TempDir(), "viewer")}
	viewer.ok("", "client", "trust", card, storeDir)
	if _, err := os.Stat(filepath.Join(viewer.config, "client", "identity")); err == nil {
		t.Fatal("a viewer was given an identity")
	}
	if out := viewer.ok("", "ls", "--all"); !strings.Contains(out, id) || !strings.Contains(out, "done") {
		t.Fatalf("a viewer's listing: %s", out)
	}
	// What is inside a run cannot be read.
	if out := viewer.ok("", "show", id, "--json"); strings.Contains(out, "output") {
		t.Fatalf("a viewer was shown what is inside a run: %s", out)
	}
	if status, _, _ := viewer.run("", "show", id, "--definition"); status == 0 {
		t.Fatal("a viewer was given a run's definition")
	}
}
