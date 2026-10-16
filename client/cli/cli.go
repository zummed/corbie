// Package cli is Corbie's command line. Everything it does to a run it does
// through the client package, so one act gives one answer in one set of
// words however it is asked for.
package cli

import (
	"context"
	"crypto/tls"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// A Command is one command of the binary, or of a program that corbie
// <name> runs. The commands, their help, the manual's pages about them and
// the names by which a model is allowed to run them all come from the one
// table of these.
type Command struct {
	Name    string // as it is typed: "run submit", "controller enrol"
	Usage   string // what follows the name
	Summary string // one line
	Help    string // the rest of its page
	// Stays says the command stays until a run ends, or until stopped.
	Stays bool
	// Run carries the command out with what followed its name.
	Run func(e *Env, args []string) error
}

// Version is which Corbie this is. A release build is given it; any other
// build reads it from the module's build information.
var Version string

func version() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

// An Env is what a command is run with: where it reads and writes, and
// the flags every command takes. The command adds its own to Flags.
type Env struct {
	In    io.Reader
	Out   io.Writer // the command's result
	Err   io.Writer // everything said along the way
	Flags *flag.FlagSet
	ctx   context.Context
	cmd   *Command

	config string
	store  string
	json   bool
	// asked holds every store's address the command went to.
	asked []string

	// For the gateway, and for the manual in a browser.
	listen    string
	tokenFile string
}

// usage is a mistake in how a command was written.
type usage struct{ text string }

func (u usage) Error() string { return u.text }

// Mistake is a mistake in how a command was written. The command ends with
// status 2, and its usage line is printed after what is wrong.
func Mistake(format string, args ...any) error { return usage{fmt.Sprintf(format, args...)} }

// Context is ended when the command is stopped.
func (e *Env) Context() context.Context { return e.ctx }

// ended is a command that waited for a run that ended badly: the command
// fails, having printed the run.
var errRunFailed = errors.New("the run did not end well")

// Main runs the command line and gives its exit status: 0 for success, 2
// for a mistake in how the command was written, 1 for failure.
func Main(args []string, in io.Reader, out, errOut io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, args, in, out, errOut)
}

// Program runs the command line of a program that corbie <name> runs,
// from the program's own table of commands, and gives its exit status as
// Main does. A command named "" is the one run when no other is named.
func Program(name string, commands []Command, args []string, in io.Reader, out, errOut io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	t := table{"corbie " + name, commands, ""}
	if cmd, rest := t.find(args); cmd != nil {
		return t.do(ctx, cmd, rest, in, out, errOut)
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, t.groupHelp(""))
		return 0
	}
	fmt.Fprintf(errOut, "%s: %q is not a command.\n\n%s", t.name, strings.Join(args, " "), t.groupHelp(""))
	return 2
}

// Run is Main under a context.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	t := own()
	cmd, rest := t.find(args)
	if cmd == nil {
		if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
			fmt.Fprint(out, overview())
			return 0
		}
		// The first word of several commands, with nothing after it or with
		// a call for help, is answered with those commands.
		group := args
		if last := args[len(args)-1]; last == "--help" || last == "-h" {
			group = args[:len(args)-1]
		}
		if text := t.groupHelp(strings.Join(group, " ")); text != "" {
			fmt.Fprint(out, text)
			return 0
		}
		// A word that begins commands runs no program: what follows it is
		// a mistake, answered with those commands.
		if taken(args[0]) {
			fmt.Fprintf(errOut, "corbie: %q is not a command.\n\n%s", strings.Join(group, " "), t.groupHelp(args[0]))
			return 2
		}
		// A word that begins no command runs the program of that name on
		// PATH, with the rest of the line.
		if path, err := exec.LookPath("corbie-" + args[0]); err == nil {
			return runProgram(ctx, path, args[1:], in, out, errOut)
		}
		fmt.Fprintf(errOut, "corbie: %q is not a command, and no program corbie-%s is on PATH.\n\ncorbie --help lists the commands.\n", strings.Join(group, " "), args[0])
		return 2
	}
	return t.do(ctx, cmd, rest, in, out, errOut)
}

// runProgram runs a program on PATH with the arguments after its name,
// this process's standard input, output and error, and its environment,
// and gives the program's exit status, also when corbie was stopped and
// the program ended well. A program a signal ended gives 1.
func runProgram(ctx context.Context, path string, args []string, in io.Reader, out, errOut io.Writer) int {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errOut
	interruptible(cmd)
	err := cmd.Run()
	switch {
	case cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0:
		return cmd.ProcessState.ExitCode()
	case cmd.ProcessState != nil:
		return 1
	}
	fmt.Fprintf(errOut, "corbie: %v\n", err)
	return 1
}

// interruptible has a stopped corbie interrupt a program it started, and
// wait ten seconds for it to end before killing it.
func interruptible(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second
}

// A program is one found on PATH under the name corbie-<name>, and the
// arguments corbie start starts it with.
type program struct {
	name, path string
	args       []string
}

// programs lists the names of the programs on PATH that corbie <name>
// runs: the names found in each directory of PATH in order, each one that
// exec.LookPath finds, as corbie <name> does. A directory that cannot be
// read is skipped, and so is a name that is a command's or begins
// commands.
func programs() []string {
	var found []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name, ok := strings.CutPrefix(entry.Name(), "corbie-")
			if runtime.GOOS == "windows" {
				name = strings.TrimSuffix(name, ".exe")
			}
			if !ok || name == "" || slices.Contains(found, name) || taken(name) {
				continue
			}
			if _, err := exec.LookPath("corbie-" + name); err == nil {
				found = append(found, name)
			}
		}
	}
	return found
}

// taken says whether a word is help, a command's name or begins the names
// of commands: corbie <word> runs no program under it.
func taken(word string) bool {
	if word == "help" {
		return true
	}
	cmd, _ := own().find([]string{word})
	return cmd != nil || len(own().rows(word+" ")) > 0
}

// A table is the commands a command line is read against: corbie's own, or
// a program's, under the words they are typed after.
type table struct {
	name     string // "corbie", or "corbie <name>" for a program
	commands []Command
	ends     string // the line that ends every command's help
}

// own is corbie's own table.
func own() table { return table{"corbie", Commands, everywhere} }

// named is a command's whole name, as it is typed.
func (t table) named(c *Command) string { return strings.TrimSpace(t.name + " " + c.Name) }

// do runs a command that has been found, with what followed its name, and
// gives its exit status. A command's result goes to out, and everything
// said along the way to errOut.
func (t table) do(ctx context.Context, cmd *Command, rest []string, in io.Reader, out, errOut io.Writer) int {
	e := cmd.env(ctx, in, out, errOut)
	named := t.named(cmd)
	err := cmd.Run(e, rest)
	var u usage
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(out, t.helpOf(cmd))
		return 0
	case errors.Is(err, errRunFailed), errors.Is(err, context.Canceled):
		return 1
	case errors.As(err, &u):
		fmt.Fprintf(errOut, "%s: %s\n\nusage: %s\n", named, u.text, strings.TrimSpace(named+" "+cmd.Usage))
		return 2
	}
	fmt.Fprintf(errOut, "%s: %s\n", named, e.clean(err.Error()))
	return 1
}

// env gives what a command is run with: where it reads and writes, and the
// flags every command takes. The command adds its own.
func (cmd *Command) env(ctx context.Context, in io.Reader, out, errOut io.Writer) *Env {
	e := &Env{In: in, Out: out, Err: errOut, ctx: ctx, cmd: cmd}
	e.Flags = flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	e.Flags.SetOutput(io.Discard)
	e.Flags.StringVar(&e.config, "config", os.Getenv("CORBIE_CONFIG"), "")
	e.Flags.StringVar(&e.store, "store", "", "")
	e.Flags.BoolVar(&e.json, "json", false, "")
	return e
}

// clean takes a store's address out of an error, wherever the address came
// from: it may hold a password.
func (e *Env) clean(text string) string {
	for _, address := range append([]string{e.store, os.Getenv("CORBIE_STORE")}, e.asked...) {
		if strings.Contains(address, "://") {
			text = strings.ReplaceAll(text, address, "the store")
		}
	}
	return text
}

// find reads the command line once: the longest name in the table that the
// arguments begin with, and what follows it.
func (t table) find(args []string) (*Command, []string) {
	var found *Command
	var rest []string
	for i := range t.commands {
		c := &t.commands[i]
		words := strings.Fields(c.Name)
		if len(args) < len(words) {
			continue
		}
		if strings.Join(args[:len(words)], " ") == c.Name && (found == nil || len(words) > len(strings.Fields(found.Name))) {
			found, rest = c, args[len(words):]
		}
	}
	return found, rest
}

// Find gives the command a command line names, as the one reading of it.
func Find(args []string) (*Command, []string) { return own().find(args) }

// Parse reads a command's flags, which may be written anywhere among its
// arguments, and gives the arguments that are not flags.
func (e *Env) Parse(args []string) ([]string, error) {
	var plain []string
	for len(args) > 0 {
		if err := e.Flags.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, Mistake("%v", err)
		}
		args = e.Flags.Args()
		if len(args) > 0 {
			plain = append(plain, args[0])
			args = args[1:]
		}
	}
	return plain, nil
}

// Say writes a line of what the command says along the way.
func (e *Env) Say(format string, args ...any) { fmt.Fprintf(e.Err, format+"\n", args...) }

// Given says whether a flag was written on the command line. A flag that
// would have no effect on what was asked for is refused as a mistake, so
// that nobody believes a limit is in force that is not.
func (e *Env) Given(name string) bool {
	found := false
	e.Flags.Visit(func(f *flag.Flag) { found = found || f.Name == name })
	return found
}

// Print writes a command's result: as JSON if that was asked for, or as
// the text given.
func (e *Env) Print(v any, text func() string) error {
	if e.json {
		data, err := json.Marshal(v, json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(e.Out, string(data))
		return err
	}
	_, err := fmt.Fprint(e.Out, text())
	return err
}

// Config is the config the command uses: the one --config or
// CORBIE_CONFIG names, or by default the one in the user's config directory.
func (e *Env) Config() (identity.Config, error) {
	if e.config != "" {
		return identity.Config{Dir: e.config}, nil
	}
	return identity.DefaultConfig()
}

// address is the store to use: the one named on the command line, in place
// of the one the environment names, and that in place of the one the
// config recorded.
func (e *Env) address(recorded string) (string, error) {
	for _, address := range []string{e.store, os.Getenv("CORBIE_STORE"), recorded} {
		if address != "" {
			e.asked = append(e.asked, address)
			return address, nil
		}
	}
	return "", fmt.Errorf("no store is known: name one with --store, or set this computer up with corbie init")
}

// Address is the address of the store the command opened last, or "" when
// it has opened none.
func (e *Env) Address() string {
	if len(e.asked) == 0 {
		return ""
	}
	return e.asked[len(e.asked)-1]
}

// Open opens the store the command uses: the one --store names, or else
// the one CORBIE_STORE names, or else recorded. It refuses a store whose
// layout this Corbie does not know. A directory another computer serves is
// opened as a party: as is the party's identity, and ctl the card of the
// controller whose computer serves it.
func (e *Env) Open(recorded string, as *identity.Identity, ctl identity.Card) (store.Store, error) {
	address, err := e.address(recorded)
	if err != nil {
		return nil, err
	}
	if err := role.StoreFits(address); err != nil {
		return nil, err
	}
	var s store.Store
	switch {
	case !store.IsServed(address):
		s, err = store.Open(address)
	case as == nil:
		return nil, fmt.Errorf("a directory another computer serves is read only by a party with an identity: corbie client card makes one, to be enrolled")
	case as.Kind == identity.Controller:
		return nil, fmt.Errorf("a controller uses its directory directly, on the computer that serves it: its store is the directory, not %q", address)
	default:
		var proof tls.Certificate
		if proof, err = as.Proof(); err == nil {
			s, err = store.OpenServed(address, as.Name, proof, ctl.SignKey())
		}
	}
	if err != nil {
		return nil, err
	}
	return s, store.Check(e.ctx, s)
}

// Client gives the client of this config: its identity, the controller it
// trusts and that controller's store. With no identity it is a viewer.
func (e *Env) Client() (*client.Client, error) {
	cfg, err := e.Config()
	if err != nil {
		return nil, err
	}
	trusted, err := cfg.TrustList(identity.Client)
	if err != nil {
		return nil, err
	}
	var held *identity.Trust
	for i := range trusted {
		if trusted[i].Trusted() {
			held = &trusted[i]
		}
	}
	if held == nil {
		return nil, fmt.Errorf("this computer's client trusts no controller yet: corbie init sets one computer up, and corbie client trust takes a controller's card")
	}
	var id *identity.Identity
	if cfg.Has(identity.Client) {
		if id, err = cfg.Load(identity.Client); err != nil {
			return nil, err
		}
	}
	s, err := e.Open(held.Store, id, held.Card)
	if err != nil {
		return nil, err
	}
	return client.New(id, held.Card, s), nil
}

// Controller gives the controller of this config and its store.
func (e *Env) Controller() (*identity.Identity, store.Store, error) {
	cfg, err := e.Config()
	if err != nil {
		return nil, nil, err
	}
	id, err := cfg.Load(identity.Controller)
	if err != nil {
		return nil, nil, err
	}
	recorded, err := cfg.ControllerStore()
	if err != nil {
		return nil, nil, err
	}
	s, err := e.Open(recorded, id, id.Card)
	return id, s, err
}

// columns is how wide the help of corbie and of a group of commands is.
const columns = 80

// A row is one line of the help of corbie or of a group of commands: a word
// and its text.
type row struct{ word, text string }

// rows gives a row for each word that follows the given words in the names
// of the commands, in the table's order. A command's last word has its
// summary, a word that begins several commands has the words that follow
// it, and a word that is a command and begins others has its summary, a
// semicolon and those words.
func (t table) rows(begins string) []row {
	var words []string
	summary := map[string]string{}
	next := map[string][]string{}
	for _, c := range t.commands {
		rest, ok := strings.CutPrefix(c.Name, begins)
		if !ok || rest == "" {
			continue
		}
		word, after, several := strings.Cut(rest, " ")
		if !slices.Contains(words, word) {
			words = append(words, word)
		}
		if !several {
			summary[word] = c.Summary
			continue
		}
		if following, _, _ := strings.Cut(after, " "); !slices.Contains(next[word], following) {
			next[word] = append(next[word], following)
		}
	}
	var listed []row
	for _, word := range words {
		text := summary[word]
		switch {
		case text == "":
			text = strings.Join(next[word], ", ")
		case len(next[word]) > 0:
			text += "; " + strings.Join(next[word], ", ")
		}
		listed = append(listed, row{word, text})
	}
	return listed
}

// width is how wide the first column of rows is: as wide as its widest
// word.
func width(listed []row) int {
	w := 0
	for _, r := range listed {
		w = max(w, len(r.word))
	}
	return w
}

// lines sets rows out in two columns, the first as wide as given, with a
// text too long for the columns going on below, under the text.
func lines(listed []row, w int) string {
	var b strings.Builder
	for _, r := range listed {
		fmt.Fprintf(&b, "  %-*s   %s\n", w, r.word, wrap(r.text, 2+w+3))
	}
	return b.String()
}

// wrap breaks text into lines that fit the columns when it begins at the
// column given, each line after the first indented to that column.
func wrap(text string, at int) string {
	var b strings.Builder
	line := 0
	for _, word := range strings.Fields(text) {
		switch {
		case line == 0:
		case at+line+1+len(word) > columns:
			b.WriteString("\n" + strings.Repeat(" ", at))
			line = 0
		default:
			b.WriteString(" ")
			line++
		}
		b.WriteString(word)
		line += len(word)
	}
	return b.String()
}

// groupHelp is what is printed for the first word, or words, of several
// commands: the words that follow, each with its text. "" is every word of
// the table. For words that begin no command it gives nothing.
func (t table) groupHelp(group string) string {
	begins, of := group+" ", strings.TrimSpace(t.name+" "+group)
	if group == "" {
		begins = ""
	}
	listed := t.rows(begins)
	if len(listed) == 0 {
		return ""
	}
	return fmt.Sprintf("The commands of %s:\n\n%s\n%s <command> --help prints a command's help.\n", of, lines(listed, width(listed)), of)
}

// overview is what corbie prints when asked for help with no command: the
// commands, then the names of the programs on PATH.
func overview() string {
	commands := own().rows("")
	var b strings.Builder
	b.WriteString("Corbie runs machines. A machine is a YAML definition of type flow, statechart\nor agent, and each of its jobs is a script or another machine.\n\n")
	b.WriteString(lines(commands, width(commands)))
	if found := programs(); len(found) > 0 {
		b.WriteString("\n" + wrap("Programs on PATH, which corbie <name> runs: "+strings.Join(found, ", ")+".", 0) + "\n")
	}
	b.WriteString(`
Every command takes these flags.

  --json                 print the result as JSON
  --config <directory>   default $CORBIE_CONFIG, or corbie in the user's config
                         directory: ~/.config/corbie on Linux
  --store <address>      default $CORBIE_STORE, or the store the config records

corbie <name> --help prints the help of a command or a program, and
corbie run --help lists the commands of corbie run. corbie manual lists the
manual's pages.
`)
	return b.String()
}

// helpOf is a command's help, and its page in the manual: its usage line,
// its summary, its help, the line the table ends every help with, and the
// commands its name begins.
func (t table) helpOf(c *Command) string {
	text := fmt.Sprintf("%s\n\n%s\n", strings.TrimSpace(t.named(c)+" "+c.Usage), c.Summary)
	if c.Help != "" {
		text += "\n" + strings.Trim(c.Help, "\n") + "\n"
	}
	if t.ends != "" {
		text += "\n" + t.ends + "\n"
	}
	if group := t.groupHelp(c.Name); group != "" {
		text += "\n" + group
	}
	return text
}

// Pages gives a page of the manual for each command, made from the table.
func Pages() map[string]string {
	pages := map[string]string{}
	for i := range Commands {
		c := &Commands[i]
		pages["corbie "+c.Name] = own().helpOf(c)
	}
	return pages
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
