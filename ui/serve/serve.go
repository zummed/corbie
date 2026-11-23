// Package serve is the command corbie ui: the parties
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
	"strings"
	"time"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/manual"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/ui/controller"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/machines"
	"github.com/zummed/corbie/ui/metrics"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/runs"
)

// Commands is corbie ui, in a table of one command, which the binary adds
// to its own. corbie start runs it beside the roles.
var Commands = []cli.Command{{
	Name:    "ui",
	Stays:   true,
	Beside:  true,
	Usage:   "[client] [controller] [none] [--listen host:port] [--token-file <file>] [--interval 1s]",
	Summary: "serve the UI to a browser until stopped",
	Run:     cmdUI,
	Help: `
It acts as the parties named, from the config: the config's client, its
controller, or both. By default it acts as every party the config holds. As
the client it shows that client's runs in full, and as the controller the
machines the controller recorded. As none it is a viewer, whatever the
config holds: it shows only the status of every run the store lets it read.

  --listen host:port    where it listens; default 127.0.0.1:8373, or a free
                        port when that one is in use
  --token-file <file>   a file holding the token every request must show;
                        by default it makes one and prints it
  --interval <time>     how often it reads the store again; default 1s, or
                        10s for a bucket

A UI listens on this computer only, unless none is named. It prints no JSON.`,
}}

// options are what the command line gives.
type options struct {
	listen, tokenFile, config, store string
	interval                         time.Duration
	asClient, asController, asNone   bool
}

// parse reads the command line, with the flags every command takes and
// the UI's own.
func parse(e *cli.Env, args []string) (options, error) {
	o := options{}
	e.Flags.StringVar(&o.listen, "listen", "127.0.0.1:8373", "")
	e.Flags.StringVar(&o.tokenFile, "token-file", "", "")
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
	if err != nil && !e.Given("listen") {
		// The default port is in use, by another UI: a free one.
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
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
	if len(u.parties.names) > 0 {
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

// A ui is a UI as it was started: its parties, the list it reads, and its
// pages before any guard.
type ui struct {
	parties *parties
	list    *list
	handler http.Handler
}

// start opens the parties, reads the list once, and gives the pages.
func start(ctx context.Context, e *cli.Env, o options) (*ui, error) {
	cfg, err := e.Config()
	if err != nil {
		return nil, err
	}
	// With no party named, the UI acts as every party the config holds.
	if !o.asClient && !o.asController && !o.asNone {
		o.asClient, o.asController = cfg.Has(identity.Client), cfg.Has(identity.Controller)
	}
	if !o.asNone && !loopback(o.listen) {
		return nil, cli.Mistake("a UI listens on this computer only, and %s is not on it: its token acts as its parties, and it has no TLS; none makes it a viewer of runs alone", o.listen)
	}
	p, err := open(ctx, e, cfg, o.asClient, o.asController)
	if err != nil {
		return nil, err
	}
	u := &ui{parties: p, list: newList()}
	interval := o.interval
	if interval == 0 {
		interval = time.Second
		if strings.HasPrefix(p.address, "s3://") {
			interval = 10 * time.Second
		}
	}
	command := func(ctx context.Context, c page.Call) page.Answer {
		answer := commandOf(ctx, o, c)
		u.list.Poke()
		return answer
	}
	shell := page.Shell{Parties: p.names, Offered: []string{"Runs"}}
	if p.controller != nil {
		shell.Offered = append(shell.Offered, "Machines", "Toolkits", "Controller")
	}

	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.Handle("GET /manual/", http.StripPrefix("/manual", manual.Site()))
	// The list of runs is read first: the Machines page counts its runs.
	found, err := p.lister.List(ctx, listed)
	if err != nil {
		return nil, err
	}
	u.list.set(found, nil)
	go u.list.follow(ctx, p.lister, interval)
	mux.Handle("GET /metrics", metrics.Handler(u.list))
	// One reader of the runs' latest steps serves both pages that read them.
	steps := &runs.Steps{}
	c := runs.Config{List: u.list, Lister: p.lister, Client: p.client, Controller: p.controller != nil, Steps: steps,
		Command: command, Interval: interval, Shell: shell}
	if p.controller != nil {
		areaMachines := machines.New(machines.Config{Controller: p.controller, Store: p.store, Client: p.client, List: u.list,
			Interval: interval, Shell: shell, Latest: steps.Latest})
		mux.Handle("/machines", areaMachines)
		mux.Handle("/machines/", areaMachines)
		mux.Handle("/toolkits", areaMachines)
		mux.Handle("/toolkits/", areaMachines)
		c.Machine, c.Differs = areaMachines.Href, areaMachines.Differs
		areaController := controller.New(controller.Config{Controller: p.controller, Store: p.store, Command: command, Interval: interval, Changed: u.list.Changed, Shell: shell})
		mux.Handle("/controller", areaController)
		mux.Handle("/controller/", areaController)
	}
	areaRuns := runs.New(c)
	mux.Handle("/runs", areaRuns)
	mux.Handle("/runs/", areaRuns)
	mux.Handle("/run/", areaRuns)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/runs", http.StatusSeeOther) })
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
