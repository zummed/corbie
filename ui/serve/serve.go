// Package serve is the program corbie-ui, which corbie ui runs: the parties
// it acts as, where it listens, the token every request shows, and the
// pages each party opens.
package serve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/zummed/corbie-ui/controller"
	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/machines"
	"github.com/zummed/corbie-ui/metrics"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie-ui/runs"
	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/manual"
	"github.com/zummed/corbie/role"
)

// Commands is corbie ui, in a table of one command, which corbie-ui reads
// as corbie reads its own.
var Commands = []cli.Command{{
	Name:    "",
	Usage:   "[client] [controller] [none] [--listen host:port] [--token-file <file>] [--dir <directory>] [--interval 1s]",
	Summary: "serve the UI to a browser until stopped",
	Run:     cmdUI,
	Help: `
It shows the machines in a directory, and needs no store for them. It acts
as the parties named, from the config: the config's client, its controller,
or both. By default it acts as every party the config holds, and a store it
cannot open leaves the directory alone. As none it is a viewer, whatever the
config holds: it shows only the status of every run the store lets it read.

  --listen host:port    where it listens; default 127.0.0.1:8373
  --token-file <file>   a file holding the token every request must show;
                        by default it makes one and prints it
  --dir <directory>     the directory of machines, read as a tree; default
                        the directory it was started in
  --interval <time>     how often it reads the store again; default 1s, or
                        10s for a bucket

A UI listens on this computer only, unless none is named.

It takes --config and --store, as every corbie command does: see corbie --help.`,
}}

// options are what the command line gives.
type options struct {
	listen, tokenFile, dir, config, store string
	interval                              time.Duration
	asClient, asController, asNone        bool
}

// parse reads the command line, with the flags every command takes and
// the UI's own.
func parse(e *cli.Env, args []string) (options, error) {
	o := options{}
	e.Flags.StringVar(&o.listen, "listen", "127.0.0.1:8373", "")
	e.Flags.StringVar(&o.tokenFile, "token-file", "", "")
	e.Flags.StringVar(&o.dir, "dir", ".", "")
	e.Flags.DurationVar(&o.interval, "interval", 0, "")
	named, err := e.Parse(args)
	if err != nil {
		return o, err
	}
	// The commands the UI runs are given the config and the store it was.
	o.config, o.store = e.Flags.Lookup("config").Value.String(), e.Flags.Lookup("store").Value.String()
	if e.Given("json") {
		return o, cli.Mistake("it prints no JSON")
	}
	for _, n := range named {
		switch {
		case n == "client" && !o.asClient:
			o.asClient = true
		case n == "controller" && !o.asController:
			o.asController = true
		case n == "none" && !o.asNone:
			o.asNone = true
		default:
			return o, cli.Mistake("%q is not a party the UI acts as: the parties are client and controller, or none, each named once", n)
		}
	}
	if o.asNone && (o.asClient || o.asController) {
		return o, cli.Mistake("none names no party, and is not named beside client or controller")
	}
	if o.interval != 0 && o.interval < 100*time.Millisecond {
		return o, cli.Mistake("--interval is shorter than 100ms")
	}
	return o, nil
}

// loopback says whether an address to listen on is on this computer only.
func loopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// cmdUI serves the UI until it is stopped.
func cmdUI(e *cli.Env, args []string) error {
	ctx, out := e.Context(), e.Out
	o, err := parse(e, args)
	if err != nil {
		return err
	}
	if role.Serverless() {
		return fmt.Errorf("the UI needs a process that stays up, and this is a serverless host")
	}
	token := newToken()
	if o.tokenFile != "" {
		if token, err = readToken(o.tokenFile); err != nil {
			return err
		}
	}
	u, err := start(ctx, e, o)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", o.listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: guard(token, listener.Addr().String(), u.handler), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shut)
	}()
	who := "a viewer"
	switch {
	case u.noStore != nil:
		who = "the directory " + u.dir
		fmt.Fprintf(out, "no store: %v\n", u.noStore)
	case len(u.parties.names) > 0:
		who = strings.Join(u.parties.names, " and ")
	}
	address := "http://" + listener.Addr().String() + "/"
	if o.tokenFile == "" {
		address += "?token=" + token
	}
	fmt.Fprintf(out, "ui for %s, at %s\n", who, address)
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// A ui is a UI as it was started: its parties, the list it reads, its pages
// before any guard, its directory, and why it has no store where it has none.
type ui struct {
	parties *parties
	list    *list
	handler http.Handler
	dir     string
	noStore error
}

// start opens the parties, reads the list once, and gives the pages.
func start(ctx context.Context, e *cli.Env, o options) (*ui, error) {
	cfg, err := e.Config()
	if err != nil {
		return nil, err
	}
	// With no party named, the UI acts as every party the config holds.
	nothingNamed := !o.asClient && !o.asController && !o.asNone
	if nothingNamed {
		o.asClient, o.asController = cfg.Has(identity.Client), cfg.Has(identity.Controller)
	}
	if !o.asNone && !loopback(o.listen) {
		return nil, cli.Mistake("a UI listens on this computer only, and %s is not on it: its token shows the directory and acts as its parties, and it has no TLS; none makes it a viewer of runs alone", o.listen)
	}
	dir, err := filepath.Abs(o.dir)
	if err != nil {
		return nil, err
	}
	p, err := open(ctx, e, cfg, o.asClient, o.asController)
	u := &ui{parties: p, dir: dir}
	if err != nil {
		if !nothingNamed || !errors.As(err, new(storeError)) {
			return nil, err
		}
		u.parties, u.noStore = &parties{}, err
		p = u.parties
	}
	interval := o.interval
	if interval == 0 {
		interval = time.Second
		if strings.HasPrefix(p.address, "s3://") {
			interval = 10 * time.Second
		}
	}
	command := func(ctx context.Context, c page.Call) page.Answer {
		answer := commandOf(ctx, o, c)
		if u.list != nil {
			u.list.Poke()
		}
		return answer
	}
	shell := page.Shell{Parties: p.names}
	if u.noStore != nil {
		shell.NoStore = u.noStore.Error()
	} else {
		shell.Offered = append(shell.Offered, "Runs")
	}
	if !o.asNone {
		shell.Offered = append(shell.Offered, "Machines", "Toolkits")
	}
	if p.controller != nil {
		shell.Offered = append(shell.Offered, "Controller")
	}

	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.Handle("GET /manual/", http.StripPrefix("/manual", manual.Site()))
	home := "/machines"
	// The list of runs is read first: the Machines page counts its runs.
	if u.noStore == nil {
		u.list = newList()
		found, err := p.lister.List(ctx, listed)
		if err != nil {
			return nil, err
		}
		u.list.set(found, nil)
		go u.list.follow(ctx, p.lister, interval)
	}
	// One reader of the runs' latest steps serves both pages that read them.
	steps := &runs.Steps{}
	var areaMachines *machines.Pages
	if !o.asNone {
		var err error
		mc := machines.Config{Dir: dir, Controller: p.controller, Store: p.store, Client: p.client, Runs: u.noStore == nil, Interval: interval, Shell: shell, Latest: steps.Latest}
		if u.list != nil {
			mc.List = u.list
		}
		areaMachines, err = machines.New(mc)
		if err != nil {
			return nil, err
		}
		mux.Handle("/machines", areaMachines)
		mux.Handle("/machines/", areaMachines)
		mux.Handle("/toolkits", areaMachines)
		mux.Handle("/toolkits/", areaMachines)
	}
	if u.noStore == nil {
		home = "/runs"
		mux.Handle("GET /metrics", metrics.Handler(u.list))
		c := runs.Config{List: u.list, Lister: p.lister, Client: p.client, Controller: p.controller != nil, Steps: steps,
			Command: command, Interval: interval, Shell: shell}
		if areaMachines != nil {
			c.Machine, c.Differs = areaMachines.Href, areaMachines.Differs
		}
		areaRuns := runs.New(c)
		mux.Handle("/runs", areaRuns)
		mux.Handle("/runs/", areaRuns)
		mux.Handle("/run/", areaRuns)
	}
	if p.controller != nil {
		areaController := controller.New(controller.Config{Controller: p.controller, Store: p.store, Command: command, Interval: interval, Changed: u.list.Changed, Shell: shell})
		mux.Handle("/controller", areaController)
		mux.Handle("/controller/", areaController)
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, home, http.StatusSeeOther) })
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/icon.svg", http.StatusMovedPermanently)
	})
	u.handler = mux
	return u, nil
}

// commandOf runs a call in this process, as corbie mcp runs one:
// with --json, and with the config and the store the UI was given. A value
// from a page that begins with "-" is refused, since the command line would
// read it as a flag.
func commandOf(ctx context.Context, o options, c page.Call) page.Answer {
	for _, a := range c.Args {
		if strings.HasPrefix(a, "-") {
			return page.Answer{Status: 2, Said: fmt.Sprintf("%q begins with \"-\", which the command line reads as a flag", a)}
		}
	}
	args := append(append(append(append([]string{}, c.Words...), c.Args...), c.Flags...), "--json")
	if o.config != "" {
		args = append(args, "--config", o.config)
	}
	if o.store != "" {
		args = append(args, "--store", o.store)
	}
	var out, said bytes.Buffer
	status := cli.Run(ctx, args, strings.NewReader(c.Stdin), &out, &said)
	return page.Answer{Status: status, Out: out.String(), Said: said.String()}
}
