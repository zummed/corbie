package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/manual"
)

// standIn puts a program named corbie-<name> first on PATH, in a directory
// of its own: a shell script that keeps its arguments, its environment and
// what it was sent in files beside it, then runs the lines given. It gives
// the directory.
func standIn(t *testing.T, name, lines string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nd=$(dirname \"$0\")\nprintf '%s\\n' \"$@\" > \"$d/args\"\nenv > \"$d/env\"\ncat > \"$d/stdin\"\n" + lines + "\n"
	if err := os.WriteFile(filepath.Join(dir, "corbie-"+name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// kept reads a file the stand-in program wrote beside itself.
func kept(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("the program kept no %s: %v", name, err)
	}
	return string(data)
}

func corbie(stdin string, args ...string) (int, string, string) {
	var out, said bytes.Buffer
	status := cli.Run(context.Background(), args, strings.NewReader(stdin), &out, &said)
	return status, out.String(), said.String()
}

// A word that begins no command runs the program corbie-<word> on PATH
// with the rest of the line, its input and its output corbie's own, and
// corbie ends with the program's status.
func TestAWordRunsTheProgramOnPath(t *testing.T) {
	dir := standIn(t, "frob", `echo "frob printed $1"; echo "frob said" >&2; exit 3`)
	status, out, said := corbie("sent\n", "frob", "one", "--two")
	if status != 3 || out != "frob printed one\n" || said != "frob said\n" {
		t.Fatalf("corbie frob ended %d, printed %q and said %q", status, out, said)
	}
	if args, sent := kept(t, dir, "args"), kept(t, dir, "stdin"); args != "one\n--two\n" || sent != "sent\n" {
		t.Fatalf("the program was started with %q and sent %q", args, sent)
	}
	// Help after the word is the program's.
	if status, _, _ := corbie("", "frob", "--help"); status != 3 || kept(t, dir, "args") != "--help\n" {
		t.Fatalf("corbie frob --help ended %d with the program started with %q", status, kept(t, dir, "args"))
	}
	// A command in the table wins over a program of its name.
	serve := standIn(t, "start", `echo "the program ran"`)
	if status, out, said := corbie("", "start", "--config", t.TempDir()); status != 1 || strings.Contains(out+said, "the program ran") {
		t.Fatalf("corbie start ended %d, printed %q and said %q", status, out, said)
	}
	if _, err := os.Stat(filepath.Join(serve, "args")); err == nil {
		t.Fatal("corbie start ran the program corbie-start")
	}
	// A word that begins commands, followed by a word that is none, is a
	// mistake answered with those commands, and runs no program of its
	// name.
	controller := standIn(t, "controller", `echo "the program ran"`)
	status, out, said = corbie("", "controller", "frob")
	if status != 2 || out != "" || !strings.Contains(said, `corbie: "controller frob" is not a command.`) || !strings.Contains(said, "  enrol ") || strings.Contains(said, "corbie-controller") {
		t.Fatalf("corbie controller frob ended %d, printed %q and said %q", status, out, said)
	}
	if _, err := os.Stat(filepath.Join(controller, "args")); err == nil {
		t.Fatal("corbie controller frob ran the program corbie-controller")
	}
}

// A word that begins no command and names no program on PATH is a mistake
// that names the program and points to corbie --help, without the commands.
func TestAWordThatIsNeitherIsAMistake(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	status, out, said := corbie("", "frob", "x")
	if status != 2 || out != "" || said != "corbie: \"frob x\" is not a command, and no program corbie-frob is on PATH.\n\ncorbie --help lists the commands.\n" {
		t.Fatalf("corbie frob x ended %d, printed %q and said %q", status, out, said)
	}
	// submit alone begins no command.
	status, out, said = corbie("", "submit", "flow.yaml")
	if status != 2 || out != "" || said != "corbie: \"submit flow.yaml\" is not a command, and no program corbie-submit is on PATH.\n\ncorbie --help lists the commands.\n" {
		t.Fatalf("corbie submit flow.yaml ended %d, printed %q and said %q", status, out, said)
	}
}

// corbie start starts a program named beside the roles once they are set up,
// with the config and the store as it was given them in the program's
// environment and what the program prints as its own output, and fails
// naming the program when it stops.
func TestStartStartsAProgramBesideTheRoles(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	dir := standIn(t, "frob", `echo "frob: up"; echo "frob: grumbles" >&2; exit 1`)
	status, out, said := m.run("", "start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "frob")
	if status != 1 || out != "frob: up\n" || !strings.Contains(said, "frob: grumbles\n") || !strings.HasSuffix(said, "corbie start: the program frob stopped (exit status 1)\n") {
		t.Fatalf("corbie start ended %d, printed %q and said:\n%s", status, out, said)
	}
	if !strings.Contains(said, "controller box-controller") || !strings.Contains(said, "runner box-runner") {
		t.Fatalf("the roles were not set up:\n%s", said)
	}
	if env, args := kept(t, dir, "env"), kept(t, dir, "args"); !strings.Contains(env, "CORBIE_CONFIG="+m.config+"\n") || strings.Contains(env, "CORBIE_STORE="+storeDir) || args != "\n" {
		t.Fatalf("the program was started with %q and the environment:\n%s", args, env)
	}
	if _, _, said := m.run("", "start", "controller", "runner", "frob", "--store", storeDir); !strings.Contains(kept(t, dir, "env"), "CORBIE_STORE="+storeDir+"\n") {
		t.Fatalf("given --store, the program's environment was:\n%s\nand corbie start said:\n%s", kept(t, dir, "env"), said)
	}
	if status, _, said := m.run("", "start", "controller", "frob", "frob"); status != 2 || !strings.Contains(said, `"frob" is named twice`) {
		t.Fatalf("a program named twice: %d %s", status, said)
	}
	// The words after a program's name are its own, up to the next name;
	// --config is corbie start's wherever it is written.
	if _, _, said := m.run("", "start", "controller", "frob", "--flag", "x"); kept(t, dir, "args") != "--flag\nx\n" {
		t.Fatalf("named with arguments, the program was started with %q and corbie start said:\n%s", kept(t, dir, "args"), said)
	}
	// A name ends the words of the program before it, and a role's flags
	// are written after the role.
	if status, _, said := m.run("", "start", "frob", "--flag", "runner", "--jobs", "2"); status != 1 || kept(t, dir, "args") != "--flag\n" || !strings.Contains(said, "2 jobs at once") {
		t.Fatalf("corbie start frob --flag runner --jobs 2 ended %d, started the program with %q and said:\n%s", status, kept(t, dir, "args"), said)
	}
	t.Setenv("PATH", t.TempDir())
	if status, _, said := m.run("", "start", "controller", "frob"); status != 2 || !strings.Contains(said, `"frob" is neither a role nor a program corbie-frob on PATH, and controller takes only flags`) {
		t.Fatalf("a name that is neither: %d %s", status, said)
	}
}

// A stopped corbie start interrupts each program and waits for it.
func TestStoppingStartEndsTheProgram(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box")
	dir := standIn(t, "frob", `trap 'kill $!; echo interrupted > "$d/ended"; exit 0' INT TERM
echo "frob: up"
sleep 30 >/dev/null 2>&1 & wait`)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	out, errOut := &said{}, &said{}
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(ctx, []string{"start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "frob", "--config", m.config}, strings.NewReader(""), out, errOut)
	}()
	until(t, "the program's line", func() bool { return strings.Contains(out.String(), "frob: up\n") })
	stop()
	if status := <-done; status != 0 || strings.Contains(errOut.String(), "stopped") {
		t.Fatalf("stopped, corbie start ended %d and said:\n%s", status, errOut.String())
	}
	if kept(t, dir, "ended") != "interrupted\n" {
		t.Fatal("the program was not interrupted")
	}
}

// A stopped corbie <name> interrupts the program and ends with the status
// the program ended with.
func TestStoppingAProgramGivesItsStatus(t *testing.T) {
	standIn(t, "frob", `trap 'kill $!; exit 0' INT TERM
echo "frob: up"
sleep 30 >/dev/null 2>&1 & wait`)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	out, errOut := &said{}, &said{}
	done := make(chan int, 1)
	go func() { done <- cli.Run(ctx, []string{"frob"}, strings.NewReader(""), out, errOut) }()
	until(t, "the program's line", func() bool { return strings.Contains(out.String(), "frob: up\n") })
	stop()
	if status := <-done; status != 0 {
		t.Fatalf("stopped, the program ended well and corbie frob ended %d, saying %q", status, errOut.String())
	}
}

// corbie help lists the commands as a tree: a word and its summary, or the
// words that follow it.
const commands = `Corbie runs machines. A machine is a YAML definition of type flow, statechart
or agent, and each of its jobs is a script or another machine.

  init         create a controller, a runner and a client on this computer
  start        run the controller, a runner, the gateway, the store's server or
               a program, until stopped
  check        start one small run, and report which part of a setup did not
               respond
  test         run a definition or a script in this process, with no setup
  run          submit, ls, show, cancel, send, reply
  controller   card, enrol, remove, parties, secret, policy, cancel
  runner       card, trust, remove
  client       card, trust, remove
  manual       print a page of the manual, or list the pages; web
  mcp          serve the manual and the commands to a model, as an MCP server
  version      print the version, the machine types and the plugins
`

// The flags every command takes, and where to read more.
const flags = `
Every command takes these flags.

  --json                 print the result as JSON
  --config <directory>   default $CORBIE_CONFIG, or corbie in the user's config
                         directory: ~/.config/corbie on Linux
  --store <address>      default $CORBIE_STORE, or the store the config records

corbie <name> --help prints the help of a command or a program, and
corbie run --help lists the commands of corbie run. corbie manual lists the
manual's pages.
`

// corbie help lists, after the commands, the names of the programs on
// PATH that corbie <name> runs, and starts none of them: under a name the
// first found, only a file that can be run, and none under a command's
// name or a word that begins commands.
func TestHelpListsTheProgramsOnPath(t *testing.T) {
	// The stand-ins find the programs they use, and no other program
	// corbie-<name>.
	tools := t.TempDir()
	for _, name := range []string{"cat", "dirname", "env"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools)
	if status, out, _ := corbie("", "help"); status != 0 || out != commands+flags {
		t.Fatalf("with no program on PATH, corbie help ended %d and printed:\n%s", status, out)
	}
	later := standIn(t, "frob", "exit 0")
	for _, name := range []string{"start", "run", "controller", "help", "quiet"} {
		standIn(t, name, "exit 0")
	}
	first := standIn(t, "frob", "exit 0")
	if err := os.WriteFile(filepath.Join(first, "corbie-notes"), []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := commands + "\nPrograms on PATH, which corbie <name> runs: frob, quiet.\n" + flags
	for _, args := range [][]string{{"help"}, {}} {
		if status, out, _ := corbie("", args...); status != 0 || out != want {
			t.Fatalf("corbie %s ended %d and printed:\n%s", strings.Join(args, " "), status, out)
		}
	}
	for _, dir := range []string{first, later} {
		if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
			t.Fatalf("corbie help started the program in %s", dir)
		}
	}
	// A relative directory of PATH is one corbie <name> runs nothing from,
	// and nothing in it is listed.
	t.Chdir(first)
	t.Setenv("PATH", ".")
	if _, out, _ := corbie("", "help"); out != commands+flags {
		t.Fatalf("with PATH a relative directory, corbie help printed:\n%s", out)
	}
	if status, _, said := corbie("", "frob"); status != 2 || !strings.Contains(said, "no program corbie-frob is on PATH") {
		t.Fatalf("with PATH a relative directory, corbie frob ended %d and said %q", status, said)
	}
}

// corbie init run again with no --store keeps the store the config
// records, as every command uses the recorded store when none is named.
func TestInitAgainKeepsTheStore(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	// CORBIE_STORE names a store for a new config, and moves no config's.
	t.Setenv("CORBIE_STORE", filepath.Join(t.TempDir(), "elsewhere"))
	out := m.ok("", "init", "--ceiling", "all", "--name", "box")
	if !strings.Contains(out, "The store is "+storeDir+".\n") || strings.Contains(out, filepath.Join(m.config, "store")) {
		t.Fatalf("init again printed:\n%s", out)
	}
}

// --help is the one way to ask for help: of a command, of the words that
// begin commands, and of a program, which the program gives. help after
// words is no call for help.
func TestHelpIsAskedForWithTheFlag(t *testing.T) {
	if status, out, said := corbie("", "start", "--help"); status != 0 || !strings.HasPrefix(out, "corbie start [") || said != "" {
		t.Fatalf("corbie start --help ended %d, printed %q and said %q", status, out, said)
	}
	run := `The commands of corbie run:

  submit   start a run and print its ID
  ls       list runs
  show     show one run: its status and its result
  cancel   cancel a run that this client started
  send     send an event to a run that this client started
  reply    answer a ticket that a run waits on

corbie run <command> --help prints a command's help.
`
	for _, args := range [][]string{{"run"}, {"run", "--help"}, {"run", "-h"}} {
		if status, out, said := corbie("", args...); status != 0 || out != run || said != "" {
			t.Fatalf("corbie %s ended %d, printed %q and said %q", strings.Join(args, " "), status, out, said)
		}
	}
	if status, out, said := corbie("", "run", "help"); status != 2 || out != "" || !strings.HasPrefix(said, "corbie: \"run help\" is not a command.\n\nThe commands of corbie run:") {
		t.Fatalf("corbie run help ended %d, printed %q and said %q", status, out, said)
	}
	// A command whose name begins others ends its help with them.
	manual := `
The commands of corbie manual:

  web   serve the pages to a browser on this computer, until stopped

corbie manual <command> --help prints a command's help.
`
	if status, out, _ := corbie("", "manual", "--help"); status != 0 || !strings.HasPrefix(out, "corbie manual [page]\n") || !strings.HasSuffix(out, manual) {
		t.Fatalf("corbie manual --help ended %d and printed %q", status, out)
	}
	dir := standIn(t, "frob", `echo "the help of frob"`)
	if status, out, _ := corbie("", "frob", "--help"); status != 0 || out != "the help of frob\n" || kept(t, dir, "args") != "--help\n" {
		t.Fatalf("corbie frob --help ended %d, printed %q, with the program started with %q", status, out, kept(t, dir, "args"))
	}
}

// frob is a program's table of commands, read with cli.Program.
var frob = []cli.Command{
	{Name: "", Usage: "[<word>] [--loud]", Summary: "say a word", Run: func(e *cli.Env, args []string) error {
		loud := e.Flags.Bool("loud", false, "")
		words, err := e.Parse(args)
		if err != nil {
			return err
		}
		said := strings.Join(words, " ")
		if *loud {
			said = strings.ToUpper(said)
		}
		return e.Print(map[string]string{"said": said}, func() string { return said + "\n" })
	}},
	{Name: "check", Usage: "<folder>", Summary: "check a folder", Run: func(e *cli.Env, args []string) error {
		folders, err := e.Parse(args)
		if err != nil {
			return err
		}
		if len(folders) != 1 {
			return cli.Mistake("it takes one folder, and %d are named", len(folders))
		}
		if e.Given("config") {
			return cli.Mistake("it takes no --config")
		}
		return errors.New("nothing is in " + folders[0])
	}},
	{Name: "ls", Usage: "", Summary: "list runs", Run: func(e *cli.Env, args []string) error {
		if _, err := e.Parse(args); err != nil {
			return err
		}
		c, err := e.Client()
		if err != nil {
			return err
		}
		runs, err := c.List(e.Context(), client.Filter{})
		if err != nil {
			return err
		}
		e.Say("on %s", e.Address())
		return e.Print(runs, func() string { return fmt.Sprintln(len(runs)) })
	}},
}

func program(args ...string) (int, string, string) {
	return programReading("", args...)
}

func programReading(stdin string, args ...string) (int, string, string) {
	var out, said bytes.Buffer
	status := cli.Program("frob", frob, args, strings.NewReader(stdin), &out, &said)
	return status, out.String(), said.String()
}

// frob's page in the manual, which a model is told when it connects.
const frobPage = "Frob says words.\n\nRead the page flow before frob check.\n"

func init() {
	manual.Add(func() []manual.Page {
		return []manual.Page{{Name: "frob", Group: "Frob", Summary: "Frob says words.", Text: frobPage}}
	})
}

// A program read with cli.Program has corbie's manual, with its commands'
// pages among Corbie's, and a model server that runs its commands and
// tells a model the page named after it.
func TestAProgramHasTheManualAndTheModelServer(t *testing.T) {
	status, out, _ := program("manual")
	for _, page := range []string{"frob", "flow", "corbie run submit", "corbie frob check", "corbie frob mcp"} {
		if status != 0 || !strings.Contains(out, "  "+page+" ") {
			t.Fatalf("corbie frob manual ended %d and lists no page %s:\n%s", status, page, out)
		}
	}
	if !strings.HasPrefix(out, "The pages of the manual. Print one with: corbie frob manual <page>\n\nFrob\n  frob ") {
		t.Fatalf("corbie frob manual does not begin with the program's own page:\n%s", out)
	}
	if status, out, _ := program("manual", "frob", "check"); status != 0 || !strings.HasPrefix(out, "corbie frob check <folder>\n\ncheck a folder\n") {
		t.Fatalf("corbie frob manual frob check ended %d and printed %q", status, out)
	}
	if status, _, said := program("manual", "nosuch"); status != 2 || !strings.Contains(said, "corbie frob manual lists them") {
		t.Fatalf("corbie frob manual nosuch ended %d and said %q", status, said)
	}
	if status, _, said := program("mcp", "--allow", "run submit"); status != 2 || !strings.Contains(said, "corbie frob --help lists them") {
		t.Fatalf("corbie frob mcp allowed a command of corbie's: ended %d and said %q", status, said)
	}

	calls := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"manual","arguments":{"page":"frob check"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"frob","arguments":{"args":["check","here"]}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"frob","arguments":{"args":["ls"]}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"corbie","arguments":{"args":["check","here"]}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"frob","arguments":{"args":["hello"]}}}`,
	}, "\n") + "\n"
	// A config the environment names reaches the command as it is, and is
	// not added to its line.
	t.Setenv("CORBIE_CONFIG", t.TempDir())
	status, out, said := programReading(calls, "mcp", "--allow", "check", "--allow", "")
	answers := strings.Split(strings.TrimSpace(out), "\n")
	slices.Sort(answers)
	if status != 0 || len(answers) != 7 {
		t.Fatalf("corbie frob mcp ended %d, said %q and answered seven calls with:\n%s", status, said, out)
	}
	var started struct {
		Result struct {
			Instructions string
			ServerInfo   struct{ Name string }
		}
	}
	if err := json.Unmarshal([]byte(answers[0]), &started); err != nil || started.Result.Instructions != frobPage || started.Result.ServerInfo.Name != "corbie-frob" {
		t.Fatalf("a model connecting was told %s", answers[0])
	}
	var listed struct {
		Result struct {
			Tools []struct{ Name, Description string }
		}
	}
	if err := json.Unmarshal([]byte(answers[1]), &listed); err != nil || len(listed.Result.Tools) != 2 || listed.Result.Tools[0].Name != "manual" || listed.Result.Tools[1].Name != "frob" ||
		!strings.Contains(listed.Result.Tools[1].Description, "corbie frob check <folder> — check a folder") || strings.Contains(listed.Result.Tools[1].Description, "corbie frob ls") {
		t.Fatalf("the tools were listed as %s", answers[1])
	}
	if !strings.Contains(answers[2], `"text":"corbie frob check \u003cfolder\u003e`) && !strings.Contains(answers[2], `"text":"corbie frob check <folder>`) {
		t.Fatalf("the manual tool gave %s", answers[2])
	}
	if !strings.Contains(answers[3], `\"ended\":1`) || !strings.Contains(answers[3], `\"result\":\"\"`) || !strings.Contains(answers[3], "nothing is in here") {
		t.Fatalf("frob check was answered %s", answers[3])
	}
	if !strings.Contains(answers[4], "not one this server was started with permission for") {
		t.Fatalf("frob ls, not allowed, was answered %s", answers[4])
	}
	if !strings.Contains(answers[5], `there is no tool named \"corbie\"`) {
		t.Fatalf("the tool corbie was answered %s", answers[5])
	}
	// A program's command prints as it does on the command line: no
	// --json is added to it.
	if !strings.Contains(answers[6], `\"result\":\"hello\\n\"`) {
		t.Fatalf("frob hello was answered %s", answers[6])
	}
}

// A program read with cli.Program has Corbie's help, flags, mistakes,
// failures and exit statuses, and finds its client and store as corbie run
// ls does.
func TestAProgramKeepsCorbiesCommandLine(t *testing.T) {
	help := `corbie frob [<word>] [--loud]

say a word

The commands of corbie frob:

  check    check a folder
  ls       list runs
  manual   print a page of the manual, or list the pages
  mcp      serve the manual and the commands to a model, as an MCP server

corbie frob <command> --help prints a command's help.
`
	if status, out, said := program("--help"); status != 0 || out != help || said != "" {
		t.Fatalf("corbie frob --help ended %d, printed %q and said %q", status, out, said)
	}
	if status, out, _ := program("check", "--help"); status != 0 || !strings.HasPrefix(out, "corbie frob check <folder>\n\ncheck a folder\n") {
		t.Fatalf("corbie frob check --help ended %d and printed %q", status, out)
	}
	if status, out, _ := program("hello", "--loud", "there"); status != 0 || out != "HELLO THERE\n" {
		t.Fatalf("corbie frob with a flag among its words ended %d and printed %q", status, out)
	}
	if status, out, _ := program("hello", "--json"); status != 0 || out != `{"said":"hello"}`+"\n" {
		t.Fatalf("corbie frob --json ended %d and printed %q", status, out)
	}
	if status, out, said := program("check"); status != 2 || out != "" || said != "corbie frob check: it takes one folder, and 0 are named\n\nusage: corbie frob check <folder>\n" {
		t.Fatalf("corbie frob check with no folder ended %d, printed %q and said %q", status, out, said)
	}
	if status, _, said := program("check", "--nosuch", "x"); status != 2 || !strings.HasPrefix(said, "corbie frob check: flag provided but not defined: -nosuch\n\nusage: corbie frob check <folder>\n") {
		t.Fatalf("corbie frob check with a flag it does not have ended %d and said %q", status, said)
	}
	if status, out, said := program("check", "here"); status != 1 || out != "" || said != "corbie frob check: nothing is in here\n" {
		t.Fatalf("corbie frob check that fails ended %d, printed %q and said %q", status, out, said)
	}
	// The store: the one recorded, then the one CORBIE_STORE names, then
	// the one --store names, each as corbie run ls finds it.
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	store := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", store, "--name", "box")
	t.Setenv("CORBIE_STORE", "")
	same := func(extra ...string) (int, string) {
		t.Helper()
		wantStatus, wantOut, wantSaid := m.run("", append([]string{"run", "ls", "--json"}, extra...)...)
		status, out, said := program(append([]string{"ls", "--json", "--config", m.config}, extra...)...)
		said = strings.TrimPrefix(said, "on "+store+"\n")
		if status != wantStatus || out != wantOut || strings.TrimPrefix(said, "corbie frob ls: ") != strings.TrimPrefix(wantSaid, "corbie run ls: ") {
			t.Fatalf("corbie frob ls %v ended %d, printed %q and said %q; corbie run ls ended %d, printed %q and said %q", extra, status, out, said, wantStatus, wantOut, wantSaid)
		}
		return status, said
	}
	if status, said := same(); status != 0 {
		t.Fatalf("corbie frob ls on the recorded store ended %d and said %q", status, said)
	}
	// An empty directory, then a directory that is not there.
	t.Setenv("CORBIE_STORE", t.TempDir())
	if status, said := same(); status != 1 || !strings.Contains(said, "it has no version") {
		t.Fatalf("corbie frob ls with CORBIE_STORE ended %d and said %q", status, said)
	}
	if status, said := same("--store", filepath.Join(t.TempDir(), "from-flag")); status != 1 || !strings.Contains(said, "from-flag") {
		t.Fatalf("corbie frob ls with --store ended %d and said %q", status, said)
	}
}

// corbie start with nothing named runs the roles the config holds, and a
// name means exactly what is named.
func TestStartRunsWhatTheConfigHolds(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box", "--ceiling", "all")
	_, said := m.staying("start")
	until(t, "the controller and the runner", func() bool {
		return strings.Contains(said.String(), "controller box-controller") && strings.Contains(said.String(), "runner box-runner")
	})

	// A name means exactly what is named.
	alone := machine{t, filepath.Join(t.TempDir(), "config")}
	alone.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box")
	_, said = alone.staying("start", "runner", "--interval", "20ms")
	until(t, "the runner", func() bool { return strings.Contains(said.String(), "runner box-runner") })
	if strings.Contains(said.String(), "controller box-controller") {
		t.Fatalf("corbie start runner ran a controller: %s", said.String())
	}

	// A config that serves its directory runs the store's server as well.
	d, address := desk(t)
	_, said = d.staying("start")
	until(t, "the store's server", func() bool { return strings.Contains(said.String(), "served at "+address) })

	// A config that holds only a runner runs the runner alone.
	gpu := joins(t, d, address, "runner", "gpu")
	_, said = gpu.staying("start")
	until(t, "the runner", func() bool { return strings.Contains(said.String(), "runner gpu") })
	if strings.Contains(said.String(), "controller ") {
		t.Fatalf("corbie start on a runner's config ran a controller: %s", said.String())
	}

	none := machine{t, filepath.Join(t.TempDir(), "none")}
	if status, _, said := none.run("", "start"); status != 1 || !strings.Contains(said, "corbie init") {
		t.Fatalf("corbie start on an empty config ended %d and said %q", status, said)
	}
}
