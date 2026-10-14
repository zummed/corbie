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
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// A Command is one command of the binary. The commands, their help, the
// manual's pages about them and the names by which a model is allowed to
// run them all come from the one table of these.
type Command struct {
	Name    string // as it is typed: "submit", "controller enrol"
	Usage   string // what follows the name
	Summary string // one line
	Help    string // the rest of its page
	// Stays says the command stays until a run ends, or until stopped.
	Stays bool
	run   func(e *env, args []string) error
}

// Version is which Corbie this is.
var Version = "0.1.0"

// env is what a command is run with.
type env struct {
	ctx    context.Context
	in     io.Reader
	out    io.Writer // the command's result
	errOut io.Writer // everything said along the way
	flags  *flag.FlagSet
	cmd    *Command

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

func mistake(format string, args ...any) error { return usage{fmt.Sprintf(format, args...)} }

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

// Run is Main under a context.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	cmd, rest := find(args)
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
		if text := groupHelp(strings.Join(group, " ")); text != "" {
			fmt.Fprint(out, text)
			return 0
		}
		fmt.Fprintf(errOut, "corbie: %q is not a command.\n\n%s", strings.Join(args, " "), overview())
		return 2
	}
	return cmd.do(ctx, rest, in, out, errOut)
}

// do runs a command that has been found, with what followed its name, and
// gives its exit status. A command's result goes to out, and everything
// said along the way to errOut.
func (cmd *Command) do(ctx context.Context, rest []string, in io.Reader, out, errOut io.Writer) int {
	e := cmd.env(ctx, in, out, errOut)
	err := cmd.run(e, rest)
	var u usage
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(out, helpOf(cmd))
		return 0
	case errors.Is(err, errRunFailed):
		return 1
	case errors.As(err, &u):
		fmt.Fprintf(errOut, "corbie %s: %s\n\nusage: corbie %s %s\n", cmd.Name, u.text, cmd.Name, cmd.Usage)
		return 2
	}
	fmt.Fprintf(errOut, "corbie %s: %s\n", cmd.Name, e.clean(err.Error()))
	return 1
}

// env gives what a command is run with: where it reads and writes, and the
// flags every command takes. The command adds its own.
func (cmd *Command) env(ctx context.Context, in io.Reader, out, errOut io.Writer) *env {
	e := &env{ctx: ctx, in: in, out: out, errOut: errOut, cmd: cmd}
	e.flags = flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	e.flags.SetOutput(io.Discard)
	e.flags.StringVar(&e.config, "config", os.Getenv("CORBIE_CONFIG"), "")
	e.flags.StringVar(&e.store, "store", "", "")
	e.flags.BoolVar(&e.json, "json", false, "")
	return e
}

// clean takes a store's address out of an error, wherever the address came
// from: it may hold a password.
func (e *env) clean(text string) string {
	for _, address := range append([]string{e.store, os.Getenv("CORBIE_STORE")}, e.asked...) {
		if strings.Contains(address, "://") {
			text = strings.ReplaceAll(text, address, "the store")
		}
	}
	return text
}

// find reads the command line once: the longest name in the table that the
// arguments begin with, and what follows it.
func find(args []string) (*Command, []string) {
	var found *Command
	var rest []string
	for i := range Commands {
		c := &Commands[i]
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
func Find(args []string) (*Command, []string) { return find(args) }

// parse reads a command's flags, which may be written anywhere among its
// arguments, and gives the arguments that are not flags.
func (e *env) parse(args []string) ([]string, error) {
	var plain []string
	for len(args) > 0 {
		if err := e.flags.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, mistake("%v", err)
		}
		args = e.flags.Args()
		if len(args) > 0 {
			plain = append(plain, args[0])
			args = args[1:]
		}
	}
	return plain, nil
}

func (e *env) say(format string, args ...any) { fmt.Fprintf(e.errOut, format+"\n", args...) }

// given says whether a flag was written on the command line. A flag that
// would have no effect on what was asked for is refused as a mistake, so
// that nobody believes a limit is in force that is not.
func (e *env) given(name string) bool {
	found := false
	e.flags.Visit(func(f *flag.Flag) { found = found || f.Name == name })
	return found
}

// print writes a command's result: as JSON if that was asked for, or as
// the text given.
func (e *env) print(v any, text func() string) error {
	if e.json {
		data, err := json.Marshal(v, json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(e.out, string(data))
		return err
	}
	_, err := fmt.Fprint(e.out, text())
	return err
}

func (e *env) cfg() (identity.Config, error) {
	if e.config != "" {
		return identity.Config{Dir: e.config}, nil
	}
	return identity.DefaultConfig()
}

// address is the store to use: the one named on the command line, in place
// of the one the environment names, and that in place of the one the
// config recorded.
func (e *env) address(recorded string) (string, error) {
	for _, address := range []string{e.store, os.Getenv("CORBIE_STORE"), recorded} {
		if address != "" {
			e.asked = append(e.asked, address)
			return address, nil
		}
	}
	return "", fmt.Errorf("no store is known: name one with --store, or set this computer up with corbie init")
}

// open opens a store and refuses one whose layout this Corbie does not
// know. A directory another computer serves is opened as a party: as is the
// party's identity, and ctl the card of the controller whose machine serves
// it.
func (e *env) open(recorded string, as *identity.Identity, ctl identity.Card) (store.Store, error) {
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

// client gives the client of this config: its identity, the controller it
// trusts and that controller's store. With no identity it is a viewer.
func (e *env) client() (*client.Client, error) {
	cfg, err := e.cfg()
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
	s, err := e.open(held.Store, id, held.Card)
	if err != nil {
		return nil, err
	}
	return client.New(id, held.Card, s), nil
}

// controller gives the controller of this config and its store.
func (e *env) controller() (*identity.Identity, store.Store, error) {
	cfg, err := e.cfg()
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
	s, err := e.open(recorded, id, id.Card)
	return id, s, err
}

// rows gives a line for each command whose name begins as given: what is
// typed, and the command's one line.
func rows(begins string) string {
	var b strings.Builder
	width := 0
	for _, c := range Commands {
		if strings.HasPrefix(c.Name, begins) {
			width = max(width, len(c.Name)+1+len(c.Usage))
		}
	}
	for _, c := range Commands {
		if strings.HasPrefix(c.Name, begins) {
			fmt.Fprintf(&b, "  corbie %-*s  %s\n", width, strings.TrimSpace(c.Name+" "+c.Usage), c.Summary)
		}
	}
	return b.String()
}

// groupHelp is what corbie prints for the first word, or words, of several
// commands: those commands, each with its one line. For words that begin
// no command it gives nothing.
func groupHelp(group string) string {
	listed := rows(group + " ")
	if listed == "" {
		return ""
	}
	return fmt.Sprintf("The commands of corbie %s:\n\n%s\nEach prints its help with --help.\n", group, listed)
}

// overview is what corbie prints when asked for help with no command.
func overview() string {
	var b strings.Builder
	b.WriteString("Corbie runs machines. A machine is a YAML definition of type flow, statechart\nor agent, and each of its jobs is a script or another machine.\n\n")
	b.WriteString(rows(""))
	b.WriteString(`
Every command takes these flags.

  --help                 print the command's help, which is its page in the
                         manual
  --json                 print the result as JSON
  --config <directory>   the config directory; default $CORBIE_CONFIG, or
                         corbie in the user's config directory
                         (~/.config/corbie on Linux)
  --store <address>      the store's address, in place of $CORBIE_STORE and of
                         the address in the config

corbie manual lists the manual's pages, and corbie manual --web serves them to
a browser.
`)
	return b.String()
}

// helpOf is a command's help, and its page in the manual: its usage line,
// its summary, its help, and a line naming the flags every command takes.
func helpOf(c *Command) string {
	text := fmt.Sprintf("corbie %s\n\n%s\n", strings.TrimSpace(c.Name+" "+c.Usage), c.Summary)
	if c.Help != "" {
		text += "\n" + strings.Trim(c.Help, "\n") + "\n"
	}
	return text + "\n" + everywhere + "\n"
}

// Pages gives a page of the manual for each command, made from the table.
func Pages() map[string]string {
	pages := map[string]string{}
	for i := range Commands {
		c := &Commands[i]
		pages["corbie "+c.Name] = helpOf(c)
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
