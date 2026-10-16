package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/manual"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// held names the roles a config holds, which corbie start runs when it is
// named none: its controller, its runner, and the store's server when the
// config records where it listens.
func held(cfg identity.Config) ([]string, error) {
	var names []string
	for _, kind := range []identity.Kind{identity.Controller, identity.Runner} {
		if cfg.Has(kind) {
			names = append(names, string(kind))
		}
	}
	listen, err := cfg.StoreListen()
	if err != nil {
		return nil, err
	}
	if listen != "" && cfg.Has(identity.Controller) {
		names = append(names, "store")
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("the config %s holds no controller and no runner: corbie init sets this computer up", cfg.Dir)
	}
	return names, nil
}

// An item is one name on corbie start's line, with the words written after
// it up to the next name.
type item struct {
	name  string
	words []string
}

// roles are the names of the roles corbie start runs.
var roles = []string{"controller", "runner", "gateway", "store"}

// items reads a line of names once: the command's own flags, wherever they
// are written, and each name with the words after it, up to the next name.
// A name is a role, or a program corbie-<name> on PATH. The command's own
// flags are those registered on e.Flags, and the call for help.
func items(e *Env, args []string) (own []string, named []item, err error) {
	for i := 0; i < len(args); i++ {
		word := args[i]
		if strings.HasPrefix(word, "-") {
			written, _, valued := strings.Cut(strings.TrimLeft(word, "-"), "=")
			if f := e.Flags.Lookup(written); f != nil || written == "help" || written == "h" {
				own = append(own, word)
				if f != nil && !boolean(f) && !valued && i+1 < len(args) {
					i++
					own = append(own, args[i])
				}
				continue
			}
		} else if name(word) {
			named = append(named, item{name: word})
			continue
		}
		if len(named) == 0 {
			return nil, nil, Mistake("%s is written before any name: write it after the role or program it is for", word)
		}
		named[len(named)-1].words = append(named[len(named)-1].words, word)
	}
	return own, named, nil
}

// boolean says whether a flag takes no value.
func boolean(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// name says whether a word on corbie start's line is a name: a role's, or a
// program's on PATH.
func name(word string) bool {
	if slices.Contains(roles, word) {
		return true
	}
	_, err := exec.LookPath("corbie-" + word)
	return err == nil
}

// A setting is what a role is given by the flags written after it.
type setting struct {
	interval          time.Duration
	jobs              int
	wrapper           string
	listen, tokenFile string
	listened          bool // --listen was written, if empty
}

// roleFlags gives the flags written after a role, which set what the role
// is given.
func roleFlags(role string, set *setting, every *string) *flag.FlagSet {
	flags := flag.NewFlagSet(role, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	switch role {
	case "controller":
		flags.StringVar(every, "interval", "2s", "")
	case "runner":
		flags.StringVar(every, "interval", "2s", "")
		flags.IntVar(&set.jobs, "jobs", 4, "")
		flags.StringVar(&set.wrapper, "exec-wrapper", "", "")
	case "gateway":
		flags.StringVar(&set.listen, "listen", "127.0.0.1:8371", "")
		flags.StringVar(&set.tokenFile, "token-file", "", "")
	case "store":
		flags.StringVar(&set.listen, "listen", "", "")
	}
	return flags
}

// owners names the roles a flag is written after, as a mistake says them:
// "the runner", "the controller or the runner", or nothing for none.
func owners(name string) string {
	var of []string
	for _, r := range roles {
		if roleFlags(r, &setting{}, new(string)).Lookup(name) != nil {
			of = append(of, "the "+r)
		}
	}
	return strings.Join(of, " or ")
}

// settings reads the flags written after a role, which are its own.
func settings(it item) (setting, error) {
	var set setting
	var every string
	flags := roleFlags(it.name, &set, &every)
	for _, word := range it.words {
		written, _, _ := strings.Cut(strings.TrimLeft(word, "-"), "=")
		if of := owners(written); strings.HasPrefix(word, "-") && flags.Lookup(written) == nil && of != "" {
			return set, Mistake("--%s is for %s, not the %s", written, of, it.name)
		}
	}
	if err := flags.Parse(it.words); errors.Is(err, flag.ErrHelp) {
		return set, err
	} else if err != nil {
		return set, Mistake("%s: %v", it.name, err)
	}
	flags.Visit(func(f *flag.Flag) { set.listened = set.listened || f.Name == "listen" })
	if flags.NArg() > 0 {
		return set, Mistake("%q is neither a role nor a program corbie-%s on PATH, and %s takes only flags", flags.Arg(0), flags.Arg(0), it.name)
	}
	if every != "" {
		var err error
		if set.interval, err = time.ParseDuration(every); err != nil || set.interval <= 0 {
			return set, Mistake("%s: --interval is a length of time, such as 2s", it.name)
		}
	}
	return set, nil
}

func cmdStart(e *Env, args []string) error {
	mine, named, err := items(e, args)
	if err != nil {
		return err
	}
	if _, err := e.Parse(mine); err != nil {
		return err
	}
	cfg, err := e.Config()
	if err != nil {
		return err
	}
	if len(named) == 0 {
		held, err := held(cfg)
		if err != nil {
			return err
		}
		for _, name := range held {
			named = append(named, item{name: name})
		}
	}
	// Every mistake on the line, and a program on a serverless host, is
	// found before anything starts.
	sets, paths := make([]setting, len(named)), make([]string, len(named))
	for i, it := range named {
		if slices.ContainsFunc(named[:i], func(o item) bool { return o.name == it.name }) {
			return Mistake("%q is named twice", it.name)
		}
		if slices.Contains(roles, it.name) {
			if sets[i], err = settings(it); err != nil {
				return err
			}
			continue
		}
		if paths[i], err = exec.LookPath("corbie-" + it.name); err != nil {
			return Mistake("%q is not a role, and no program corbie-%s is on PATH: the roles are controller, runner, gateway and store", it.name, it.name)
		}
		if role.Serverless() {
			return fmt.Errorf("a program needs a process that stays up, and this is a serverless host")
		}
	}
	// On a serverless host the roles use a copy of the config that is
	// theirs alone.
	own, err := role.OwnConfig(cfg.Dir)
	if err != nil {
		return err
	}
	if own != cfg.Dir {
		defer os.RemoveAll(own)
	}
	cfg.Dir, e.config = own, own
	var running []role.Role
	var programs []program
	for i, it := range named {
		name, set := it.name, sets[i]
		switch name {
		case "controller":
			id, s, err := e.Controller()
			if err != nil {
				return err
			}
			notice, stop := s.Notice(store.Inbox(id.Name))
			defer stop()
			running = append(running, role.Role{Name: name, Pass: controller.New(id, s).Pass, Interval: set.interval, Notice: notice})
			e.Say("controller %s  (%s)", id.Name, id.Fingerprint())
		case "runner":
			id, err := cfg.Load(identity.Runner)
			if err != nil {
				return err
			}
			// Each controller has its own store and its own ceiling, read
			// afresh from the config at every pass.
			woken := make(chan struct{}, 1)
			stores := map[string]store.Store{}
			trusted := func() ([]runner.Served, error) {
				held, err := cfg.TrustList(identity.Runner)
				if err != nil {
					return nil, err
				}
				var serves []runner.Served
				for _, t := range held {
					if !t.Trusted() {
						continue
					}
					s, opened := stores[t.Store]
					if !opened {
						if s, err = e.Open(t.Store, id, t.Card); err != nil {
							return nil, fmt.Errorf("the store of the controller %q: %w", t.Card.Name, err)
						}
						stores[t.Store] = s
						notice, _ := s.Notice(store.Inbox(id.Name))
						go func() {
							for range notice {
								select {
								case woken <- struct{}{}:
								default:
								}
							}
						}()
					}
					serves = append(serves, runner.Served{Controller: t.Card, Ceiling: t.Ceiling, Store: s})
				}
				return serves, nil
			}
			serves, err := trusted()
			if err != nil {
				return err
			}
			if len(serves) == 0 {
				return fmt.Errorf("this runner trusts no controller yet: corbie runner trust takes a controller's card")
			}
			r := runner.New(id, serves...)
			r.Trusted = trusted
			r.Jobs = set.jobs
			r.Wrapper = strings.Fields(set.wrapper)
			running = append(running, role.Role{Name: name, Pass: r.Pass, Interval: set.interval, Notice: woken})
			e.Say("runner %s  (%s), serving %d controller(s), %d jobs at once", id.Name, id.Fingerprint(), len(serves), set.jobs)
		case "gateway":
			e.listen, e.tokenFile = set.listen, set.tokenFile
			gateway, err := e.gateway()
			if err != nil {
				return err
			}
			defer gateway()
		case "store":
			stop, err := e.serve(set.listen)
			if err != nil {
				return err
			}
			defer stop()
		default:
			programs = append(programs, program{name: name, path: paths[i], args: it.words})
		}
	}
	report := func(name string, err error) { e.Say("%s: %s", name, e.clean(err.Error())) }
	if role.Serverless() {
		// The host calls for each pass: there is no loop.
		return role.Serve(e.ctx, report, running...)
	}
	// A program is started once every role named is set up, and is waited
	// for before this returns. When one stops, everything stops; when this
	// is stopped, each program is interrupted.
	var started sync.WaitGroup
	defer started.Wait()
	ctx, stop := context.WithCancel(e.ctx)
	defer stop()
	stopped := make(chan error, len(programs))
	for _, p := range programs {
		cmd := exec.CommandContext(ctx, p.path, p.args...)
		cmd.Stdout, cmd.Stderr = e.Out, e.Err
		cmd.Env = os.Environ()
		if e.Given("config") {
			cmd.Env = append(cmd.Env, "CORBIE_CONFIG="+e.config)
		}
		if e.Given("store") {
			cmd.Env = append(cmd.Env, "CORBIE_STORE="+e.store)
		}
		interruptible(cmd)
		if err := cmd.Start(); err != nil {
			return err
		}
		started.Go(func() {
			err := cmd.Wait()
			if ctx.Err() != nil {
				return
			}
			why := ""
			if err != nil {
				why = fmt.Sprintf(" (%v)", err)
			}
			stopped <- fmt.Errorf("the program %s stopped%s", p.name, why)
			stop()
		})
	}
	role.Run(ctx, time.Now, report, running...)
	// A gateway or a served directory alone has no passes to make, and stays
	// up all the same.
	<-ctx.Done()
	select {
	case err := <-stopped:
		return err
	default:
		return nil
	}
}

func cmdManual(e *Env, args []string) error {
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		pages := manual.Pages()
		return e.Print(pages, func() string {
			var b strings.Builder
			b.WriteString("The pages of the manual. Print one with: corbie manual <page>\n\n")
			group := ""
			for _, page := range pages {
				if page.Group != group {
					group = page.Group
					fmt.Fprintf(&b, "%s\n", group)
				}
				line := page.Summary
				if len(page.Needs) > 0 {
					// The kinds of permission a plugin's calls ask for.
					line += "  (needs " + strings.Join(page.Needs, ", ") + ")"
				}
				fmt.Fprintf(&b, "  %-22s %s\n", page.Name, line)
			}
			return b.String()
		})
	}
	name := strings.Join(rest, " ")
	page, ok := manual.Find(name)
	if !ok {
		return Mistake("%q is not a page of the manual; corbie manual lists them", name)
	}
	return e.Print(page, func() string { return strings.TrimRight(page.Text, "\n") + "\n" })
}

func cmdManualWeb(e *Env, args []string) error {
	e.Flags.StringVar(&e.listen, "listen", "127.0.0.1:7403", "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return Mistake("manual web serves every page, and takes no page's name")
	}
	listener, err := net.Listen("tcp", e.listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: manual.Site(), ReadHeaderTimeout: 10 * time.Second}
	go server.Serve(listener)
	e.Say("the manual is at http://%s until this is stopped", listener.Addr())
	<-e.ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	return nil
}

func cmdVersion(e *Env, args []string) error {
	if _, err := e.Parse(args); err != nil {
		return err
	}
	holds := map[string]any{"version": version(), "machines": machine.Types(), "plugins": plugin.Names()}
	return e.Print(holds, func() string {
		return fmt.Sprintf("corbie %s\nmachine types: %s\nplugins and libraries: %s\n", version(),
			strings.Join(machine.Types(), ", "), strings.Join(plugin.Names(), ", "))
	})
}

func init() {
	// The manual's pages for commands are made from the one table.
	manual.Add(func() []manual.Page {
		var pages []manual.Page
		for i := range Commands {
			c := &Commands[i]
			pages = append(pages, manual.Page{Name: "corbie " + c.Name, Group: "Commands", Summary: c.Summary, Text: own().helpOf(c)})
		}
		return pages
	})
}
