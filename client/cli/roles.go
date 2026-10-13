package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
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

func cmdRun(e *env, args []string) error {
	var every string
	var jobs int
	var wrapper string
	e.flags.StringVar(&every, "interval", "2s", "")
	e.flags.IntVar(&jobs, "jobs", 4, "")
	e.flags.StringVar(&wrapper, "exec-wrapper", "", "")
	e.flags.StringVar(&e.listen, "listen", "127.0.0.1:8371", "")
	e.flags.StringVar(&e.tokenFile, "token-file", "", "")
	names, err := e.parse(args)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return mistake("name what to run: controller, runner, gateway or store")
	}
	// A flag for a role that is not named would be taken and do nothing.
	for _, of := range [][2]string{{"jobs", "runner"}, {"exec-wrapper", "runner"}, {"listen", "gateway"}, {"token-file", "gateway"}} {
		if e.given(of[0]) && !slices.Contains(names, of[1]) {
			return mistake("--%s applies only to a %s, and none is named", of[0], of[1])
		}
	}
	if e.given("interval") && !slices.Contains(names, "controller") && !slices.Contains(names, "runner") {
		return mistake("--interval is how long a controller or a runner sleeps, and neither is named")
	}
	pause, err := time.ParseDuration(every)
	if err != nil || pause <= 0 {
		return mistake("--interval is a length of time, such as 2s")
	}
	cfg, err := e.cfg()
	if err != nil {
		return err
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
	var roles []role.Role
	for _, name := range names {
		if slices.ContainsFunc(roles, func(r role.Role) bool { return r.Name == name }) {
			return mistake("%s is named twice", name)
		}
		switch name {
		case "controller":
			id, s, err := e.controller()
			if err != nil {
				return err
			}
			notice, stop := s.Notice(store.Inbox(id.Name))
			defer stop()
			roles = append(roles, role.Role{Name: name, Pass: controller.New(id, s).Pass, Notice: notice})
			e.say("controller %s  (%s)", id.Name, id.Fingerprint())
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
						if s, err = e.open(t.Store, id, t.Card); err != nil {
							return nil, fmt.Errorf("the store of the controller %s: %w", t.Card.Name, err)
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
			r.Jobs = jobs
			r.Wrapper = strings.Fields(wrapper)
			roles = append(roles, role.Role{Name: name, Pass: r.Pass, Notice: woken})
			e.say("runner %s  (%s), serving %d controller(s), %d jobs at once", id.Name, id.Fingerprint(), len(serves), jobs)
		case "gateway":
			gateway, err := e.gateway()
			if err != nil {
				return err
			}
			defer gateway()
		case "store":
			stop, err := e.serve()
			if err != nil {
				return err
			}
			defer stop()
		default:
			return mistake("%q is not a role: the roles are controller, runner, gateway and store", name)
		}
	}
	report := func(name string, err error) { e.say("%s: %s", name, e.clean(err.Error())) }
	if role.Serverless() {
		// The host calls for each pass: there is no loop.
		return role.Serve(e.ctx, report, roles...)
	}
	role.Run(e.ctx, pause, time.Now, report, roles...)
	// A gateway or a served directory alone has no passes to make, and stays
	// up all the same.
	<-e.ctx.Done()
	return nil
}

func cmdManual(e *env, args []string) error {
	var web bool
	e.flags.BoolVar(&web, "web", false, "")
	e.flags.StringVar(&e.listen, "listen", "127.0.0.1:7403", "")
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	if web {
		if len(rest) > 0 {
			return mistake("--web serves every page, and takes no page's name")
		}
		listener, err := net.Listen("tcp", e.listen)
		if err != nil {
			return err
		}
		server := &http.Server{Handler: manual.Site(), ReadHeaderTimeout: 10 * time.Second}
		go server.Serve(listener)
		e.say("the manual is at http://%s until this is stopped", listener.Addr())
		<-e.ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		return nil
	}
	if e.given("listen") {
		return mistake("--listen says where --web serves the manual, and --web is not given")
	}
	if len(rest) == 0 {
		pages := manual.Pages()
		return e.print(pages, func() string {
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
		return mistake("%q is not a page of the manual; corbie manual lists them", name)
	}
	return e.print(page, func() string { return strings.TrimRight(page.Text, "\n") + "\n" })
}

func cmdVersion(e *env, args []string) error {
	if _, err := e.parse(args); err != nil {
		return err
	}
	holds := map[string]any{"version": Version, "machines": machine.Types(), "plugins": plugin.Names()}
	return e.print(holds, func() string {
		return fmt.Sprintf("corbie %s\nmachine types: %s\nplugins and libraries: %s\n", Version,
			strings.Join(machine.Types(), ", "), strings.Join(plugin.Names(), ", "))
	})
}

func init() {
	// The manual's pages for commands are made from the one table.
	manual.Add(func() []manual.Page {
		var pages []manual.Page
		for i := range Commands {
			c := &Commands[i]
			pages = append(pages, manual.Page{Name: "corbie " + c.Name, Group: "Commands", Summary: c.Summary, Text: helpOf(c)})
		}
		return pages
	})
}
