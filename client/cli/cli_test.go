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

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
	_ "github.com/zummed/corbie/plugin/bundled"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	// No test reads or runs the config of the person running the tests.
	home, err := os.MkdirTemp("", "corbie-config-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", home)
	os.Unsetenv("CORBIE_CONFIG")
	os.Unsetenv("CORBIE_STORE")
	status := m.Run()
	os.RemoveAll(home)
	os.Exit(status)
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
	return m.runFor(30*time.Second, stdin, args...)
}

// runFor runs corbie as run does, stopping it after a time.
func (m machine) runFor(limit time.Duration, stdin string, args ...string) (int, string, string) {
	m.t.Helper()
	var out, said bytes.Buffer
	full := append(args, "--config", m.config)
	// Not a deadline: a runner takes a context's deadline for its host's
	// limit, and takes no job it could not finish before it.
	ctx, cancel := context.WithCancel(context.Background())
	defer time.AfterFunc(limit, cancel).Stop()
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

// recorded records a file as a tree of one file, as corbie run submit does,
// with the flags given, and gives its location: / and the file's name
// without its extension. A run of it is started by its location.
func (m machine) recorded(file string, flags ...string) string {
	m.t.Helper()
	m.ok("", append([]string{"run", "submit", file}, flags...)...)
	return "/" + strings.TrimSuffix(strings.TrimSuffix(filepath.Base(file), ".js"), ".yaml")
}

func files(t *testing.T, contents map[string]string) string {
	dir := t.TempDir()
	for name, text := range contents {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	return dir
}

const hello = `flow: hello
input: { name: any }
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

func TestInitRunSubmitStart(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	out := m.ok("", "init", "--store", storeDir, "--name", "box")
	for _, want := range []string{"box-controller", "box-runner", "box-client", "A directory keeps no party out", "The ceiling is empty"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init printed nothing of %q:\n%s", want, out)
		}
	}
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return { text: "hello " + input.name }`})
	roles := make(chan int, 1)
	var stop context.CancelFunc
	start := func() {
		var ctx context.Context
		ctx, stop = context.WithCancel(context.Background())
		go func() {
			var out, said bytes.Buffer
			roles <- cli.Run(ctx, []string{"start", "controller", "--interval", "200ms", "runner", "--interval", "200ms", "--config", m.config}, strings.NewReader(""), &out, &said)
		}()
	}
	start()
	defer func() { stop() }()
	if out := m.ok("", "run", "submit", filepath.Join(dir, "hello.yaml")); out != "/hello   transient   recorded\n" {
		t.Fatalf("the file was recorded as %q", out)
	}
	stop()
	<-roles

	// With the controller stopped, a run started is waiting to start.
	id := strings.TrimSpace(m.ok("", "run", "start", "/hello", "--input", `{"name": "directory"}`, "--label", "team=ops"))
	if strings.ContainsAny(id, " \n") || !strings.HasPrefix(id, "box-client/") {
		t.Fatalf("submit printed %q", id)
	}
	if out := m.ok("", "run", "show", id); !strings.Contains(out, "waiting") {
		t.Fatalf("before any controller ran, show printed %s", out)
	}
	start()
	if out := m.ok("", "run", "show", id, "--wait", "--json"); !strings.Contains(out, `"status":"done"`) || !strings.Contains(out, "hello directory") {
		t.Fatalf("the run ended as %s", out)
	}
	// A row of the listing shows the run's labels.
	if out := m.ok("", "run", "ls", "--all", "--label", "team=ops"); !strings.Contains(out, id) || !strings.Contains(out, "  labels: team=ops\n") {
		t.Fatalf("ls printed %s", out)
	}
	if out := m.ok("", "run", "show", id, "--definition"); !strings.Contains(out, `"name":"hello"`) {
		t.Fatalf("the definition printed %s", out)
	}
	// A run that does not exist is a failure that says so.
	if status, _, said := m.run("", "run", "show", "box-client/20200101T000000.000Z-0000000000"); status != 1 || !strings.Contains(said, "no such run") {
		t.Fatalf("no such run: %d %s", status, said)
	}
	// With no ceiling, a script that reads a file is denied, and the fix
	// names the command that changes it. The script's own list allowed it,
	// so nothing is said of adding to the command line.
	reads := files(t, map[string]string{"reads.js": `return fs.read("/etc/hostname")`})
	_, out, _ = m.run("", "run", "start", m.recorded(filepath.Join(reads, "reads.js"), "--allow", "disk"), "--wait")
	if !strings.Contains(out, "denied") || !strings.Contains(out, "corbie controller enrol box-client --allow fs.read:/etc/hostname") || strings.Contains(out, "add to the command") {
		t.Fatalf("a denied run printed %s", out)
	}
	m.ok("", "controller", "enrol", "box-client", "--allow", "fs.read:/etc/hostname")
	m.ok("", "runner", "trust", "box-controller", "--allow", "fs.read:/etc/hostname")
	if _, out, _ := m.run("", "run", "start", "/reads", "--wait"); !strings.Contains(out, "done") {
		t.Fatalf("after the ceilings were raised: %s", out)
	}
	stop()
	<-roles
}

// With no --store, the store is the directory store in the config, and the
// roles run on it as on any other.
func TestInitWithNoStoreSetsItUpInTheConfig(t *testing.T) {
	t.Setenv("CORBIE_STORE", "")
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	out := m.ok("", "init", "--name", "box")
	storeDir := filepath.Join(m.config, "store")
	if !strings.Contains(out, "The store is "+storeDir+".") || !strings.Contains(out, "To change that: corbie init --ceiling all") {
		t.Fatalf("init with no --store printed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(storeDir, "version")); err != nil {
		t.Fatalf("the store was not set up in the config: %v", err)
	}
	m.staying("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms")
	dir := files(t, map[string]string{"one.js": `return 1`})
	if out := m.ok("", "run", "start", m.recorded(filepath.Join(dir, "one.js")), "--wait", "--json"); !strings.Contains(out, `"status":"done"`) || !strings.Contains(out, `"output":1`) {
		t.Fatalf("the run ended as %s", out)
	}
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
	t.Setenv("PATH", t.TempDir())
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
	for _, want := range []string{"flow", "statechart", "agent", "http", "blobs", "corbie run submit", "permissions"} {
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
	if status, out, _ := m.run("", "help"); status != 0 || !strings.Contains(out, "\n  run ") {
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
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		var out, said bytes.Buffer
		done <- cli.Run(ctx, []string{"start", "controller", "--interval", "100ms", "runner", "--interval", "100ms", "--config", m.config}, strings.NewReader(""), &out, &said)
	}()
	id := strings.TrimSpace(m.ok("", "run", "start", m.recorded(filepath.Join(dir, "hello.yaml")), "--input", `{"name": "x"}`))
	m.ok("", "run", "show", id, "--wait")
	// The record says the run has ended a moment before its head does:
	// let the controller finish the step before it is stopped.
	for range 200 {
		if strings.Contains(m.ok("", "run", "ls", "--all"), "done") {
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
	if out := viewer.ok("", "run", "ls", "--all"); !strings.Contains(out, id) || !strings.Contains(out, "done") {
		t.Fatalf("a viewer's listing: %s", out)
	}
	// What is inside a run cannot be read.
	if out := viewer.ok("", "run", "show", id, "--json"); strings.Contains(out, "output") || strings.Contains(out, "input") {
		t.Fatalf("a viewer was shown what is inside a run: %s", out)
	}
	if status, _, _ := viewer.run("", "run", "show", id, "--definition"); status == 0 {
		t.Fatal("a viewer was given a run's definition")
	}
}

func TestTestReadsADirectory(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "on"), 0o755)
	os.WriteFile(filepath.Join(dir, "on", "hello.yaml"), []byte(hello), 0o644)
	os.WriteFile(filepath.Join(dir, "on", "greet.js"), []byte(`return input`), 0o644)
	from, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(from)
	if out := m.ok("", "test", "on"); out != "on/hello.yaml  ok\n" {
		t.Fatalf("corbie test on printed %q", out)
	}
	os.MkdirAll(filepath.Join(dir, "on", "shared"), 0o755)
	os.WriteFile(filepath.Join(dir, "on", "shared", "base.yaml"), []byte("allow: [env:HOME]\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "on", "notes.yaml"), []byte("- later\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "on", ".corbieignore"), []byte("shared/\nnotes.yaml\n"), 0o644)
	if out := m.ok("", "test", "on"); out != "on/hello.yaml  ok\non/notes.yaml  listed\non/shared/     listed\n" {
		t.Fatalf("corbie test on, with paths listed, printed %q", out)
	}
	os.Remove(filepath.Join(dir, "on", ".corbieignore"))
	if status, out, _ := m.run("", "test", "on"); status != 1 || !strings.Contains(out, "on/shared/base.yaml\n    it gives no type") {
		t.Fatalf("with nothing listed, corbie test on ended %d and printed %q", status, out)
	}
	os.RemoveAll(filepath.Join(dir, "on", "shared"))
	os.Remove(filepath.Join(dir, "on", "notes.yaml"))
	if entries, _ := os.ReadDir(m.config); len(entries) != 0 {
		t.Fatal("corbie test on a directory left a config behind")
	}
	if status, _, said := m.run("", "test", "on", "--input", "{}"); status != 2 || !strings.Contains(said, "--input applies to one definition") {
		t.Fatalf("corbie test on --input ended %d and said %q", status, said)
	}
	if status, _, _ := m.run("", "run", "submit", "on", "--check"); status != 2 {
		t.Fatalf("corbie run submit --check ended %d", status)
	}
	os.Remove(filepath.Join(dir, "on", "greet.js"))
	status, out, _ := m.run("", "test", "on")
	if status != 1 || !strings.Contains(out, "on/hello.yaml\n    ") {
		t.Fatalf("with its script gone, corbie test on ended %d and printed %q", status, out)
	}
}

// corbie test given a directory checks each send and call to a location of
// the tree against the file there and the sender's own lists, and names a
// location with no machine without failing.
func TestTestChecksTheSendsOfATree(t *testing.T) {
	m := machine{t, t.TempDir()}
	chart := func(head, entry string) string {
		return "statechart: x\n" + head + "initial: a\nstates:\n  a:\n" + entry + "    on: { set: a }\n"
	}
	dir := files(t, map[string]string{
		"rooms/room.yaml": chart("lifetime: keyed\n", ""),
		"boiler.yaml":     chart("lifetime: singleton\nreceive: [panel]\n", ""),
		"weather.yaml":    "flow: weather\nstages:\n  a: { job: ./a.js }\n",
		"steady.yaml":     "flow: steady\nlifetime: singleton\nstages:\n  a: { job: ./a.js }\n",
		"a.js":            "return 1",
		"panel.yaml": chart(`allow: ["send:**"]`+"\n", "    entry:\n      send:\n"+
			"        - { to: { at: rooms/room, key: k }, event: set }\n"+
			"        - { to: { at: boiler }, event: set }\n"+
			"        - { to: { at: \"rooms/${input.r}\" }, event: set }\n"+
			"        - { to: { at: later }, event: set }\n"+
			"        - { to: { at: weather }, event: go }\n"+
			"    job: { call: { at: weather } }\n"),
		"sensor.yaml": chart(`allow: ["send:rooms/**", "send:steady"]`+"\n", "    entry:\n      send:\n"+
			"        - { to: { at: rooms/room }, event: set }\n"+
			"        - { to: { at: boiler }, event: set }\n"+
			"        - { to: { at: steady }, event: set }\n"+
			"    job: { call: { at: rooms/room } }\n"),
		"door.yaml": chart(`allow: ["send:**"]`+"\n", "    entry:\n      send:\n"+
			"        - { to: { at: boiler }, event: set }\n"+
			"        - { to: { at: bell }, event: ring }\n"+
			"    job: { call: { at: weather }, deny: [\"send:weather\"] }\n"),
		"bell.yaml": "statechart: bell\nlifetime: singleton\ninitial: a\nstates:\n  a: { timers: { ring: 1h }, on: { ring: a } }\n",
	})
	status, out, _ := m.run("", "test", dir)
	out = strings.ReplaceAll(out, dir+"/", "")
	name := filepath.Base(dir)
	for _, want := range []string{
		"panel.yaml       ok\n",
		"bell.yaml        ok\n",
		"sensor.yaml\n",
		"    states.a.job.call: /" + name + "/rooms/room is keyed: a call starts a run only of a transient location\n",
		"    states.a.entry.send[0].to: /" + name + "/rooms/room is keyed: a send to it names a key, as /" + name + "/rooms/room#<key>\n",
		"    states.a.entry.send[1].to: send:/" + name + "/boiler was refused by the definition of \"x\"",
		"    states.a.entry.send[2].to: /" + name + "/steady is a flow, which takes no sent event\n",
		"door.yaml\n    states.a.job.call: send:/" + name + "/weather was refused by the job states.a.job in the definition of \"x\"",
		"    states.a.entry.send[0].to: /" + name + "/boiler does not receive set from /" + name + "/door\n",
		"    states.a.entry.send[1].to: \"ring\" is the name of an event /" + name + "/bell gives itself\n",
		"nothing in the tree is at /" + name + "/later, named by panel.yaml states.a.entry.send[3].to\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("corbie test printed no %q:\n%s", want, out)
		}
	}
	if status != 1 {
		t.Errorf("corbie test ended %d", status)
	}
	os.Remove(filepath.Join(dir, "sensor.yaml"))
	os.Remove(filepath.Join(dir, "door.yaml"))
	os.Remove(filepath.Join(dir, "bell.yaml"))
	if out := m.ok("", "test", dir); !strings.Contains(out, "nothing in the tree is at /"+name+"/later") {
		t.Errorf("a location with no machine failed the test:\n%s", out)
	}
}

// corbie test runs a file by its lifetime: a transient file with its input,
// a singleton started by its submit, and a keyed file not at all.
func TestTestRunsEachLifetime(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return { text: "hello " + input.name }`,
		"once.yaml": "statechart: once\nlifetime: singleton\ninitial: over\nstates:\n  over: { end: done, output: once }\n",
		"room.yaml": "statechart: room\nlifetime: keyed\ninitial: open\nstates:\n  open: { on: { said: open } }\n"})
	if out := m.ok("", "test", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`, "--json"); !strings.Contains(out, `"text":"hello x"`) {
		t.Fatalf("a transient file printed %s", out)
	}
	if out := m.ok("", "test", filepath.Join(dir, "once.yaml"), "--json"); !strings.Contains(out, `"output":"once"`) {
		t.Fatalf("a singleton printed %s", out)
	}
	status, out, said := m.run("", "test", filepath.Join(dir, "room.yaml"))
	if status != 1 || out != "" || !strings.Contains(said, "room.yaml is keyed: a run starts there when an event is sent to a key") {
		t.Fatalf("a keyed file ended %d: %s%s", status, out, said)
	}
}

// corbie test --definition prints the definition written out, each value
// with its file and line, and runs nothing; a flag of a start beside it is
// a mistake.
func Test_client_test_definition_prints_file_and_line(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{
		"base.yaml":   "allow: [net:example.com]\ntitle: base\n",
		"family.yaml": "flow: family\ninclude: base.yaml\nstages:\n  greet: { job: ./greet.js }\n",
		"greet.js":    "this script would fail: (",
	})
	out := m.ok("", "test", filepath.Join(dir, "family.yaml"), "--definition")
	for _, line := range []string{"flow: family", "# family.yaml:1", "net:example.com", "# base.yaml:1", "title: base", "# base.yaml:2", "job: ./greet.js", "# family.yaml:4"} {
		if !strings.Contains(out, line) {
			t.Fatalf("--definition printed no %q:\n%s", line, out)
		}
	}
	var def map[string]any
	if err := json.Unmarshal([]byte(m.ok("", "test", filepath.Join(dir, "family.yaml"), "--definition", "--json")), &def); err != nil || def["name"] != "family" || def["lines"] == nil {
		t.Fatalf("--definition --json printed %v (%v)", def, err)
	}
	for _, flags := range [][]string{{"--input", "{}"}, {"--watch"}, {"--value", "A=b"}} {
		args := append([]string{"test", filepath.Join(dir, "family.yaml"), "--definition"}, flags...)
		if status, out, said := m.run("", args...); status != 2 || out != "" || !strings.Contains(said, "--definition runs nothing") {
			t.Errorf("corbie %s ended %d: %s%s", strings.Join(args, " "), status, out, said)
		}
	}
	if status, _, said := m.run("", "test", dir, "--definition"); status != 2 || !strings.Contains(said, "--definition applies to one definition") {
		t.Fatalf("a directory with --definition ended %d: %s", status, said)
	}
}
