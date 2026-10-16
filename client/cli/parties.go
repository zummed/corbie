package cli

import (
	"cmp"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
	"golang.org/x/term"
)

// ceiling reads a ceiling as it is given on a command line. --ceiling sets
// its allow list, entries with commas between them; --allow and --deny each
// add one entry, and with no --ceiling they are added to the ceiling the
// party has.
// none is what --ceiling and --tags are given to take away what was set.
const none = "none"

func ceiling(text string, allow, deny []string, has permission.Layer) (permission.Layer, error) {
	var set []string
	for _, entry := range strings.Split(text, ",") {
		if entry = strings.TrimSpace(entry); entry != "" && text != none {
			set = append(set, entry)
		}
	}
	for _, given := range []struct {
		flag    string
		entries []string
	}{{"--ceiling", set}, {"--allow", allow}, {"--deny", deny}} {
		if err := permission.CheckList(given.entries); err != nil {
			return permission.Layer{}, Mistake("%s: %v", given.flag, err)
		}
	}
	// With no --ceiling the ceiling on record stays, and --ceiling none
	// takes it away.
	layer := permission.Layer{Allow: set}
	if text == "" {
		layer = permission.Layer{Allow: has.Allow, Deny: has.Deny}
	}
	layer.Allow, layer.Deny = append(layer.Allow, allow...), append(layer.Deny, deny...)
	return layer, nil
}

// has gives the ceiling on record for a card: that of the same key under
// the same name, and nothing of what another key was given.
func has(old *identity.Trust, card identity.Card) permission.Layer {
	if old != nil && old.Card.Same(card) {
		return old.Ceiling
	}
	return permission.Layer{}
}

func hostName() string {
	name, err := os.Hostname()
	if err != nil || identity.CheckName(name) != nil {
		return "corbie"
	}
	return name
}

// ownStore refuses, as a controller's store, the address other computers
// reach a served directory by: a controller uses its directory as it is.
func ownStore(address string) error {
	if store.IsServed(address) {
		return Mistake("a controller's store is its own directory, or a bucket: %q is how other computers reach a directory that a controller's computer serves", address)
	}
	return nil
}

// storeListen is where the store's server listens when store is named with
// no --listen: every network interface, at this port.
const storeListen = ":8372"

// sharing reads the line of a command that sets the store's server up: the
// flags registered on e.Flags, wherever they are written, and store with
// its --listen after it. It says whether store is named, and where its server
// is to listen: by default storeListen, and nothing where --listen is
// written empty, which stops the directory being served.
func sharing(e *Env, args []string) (listen string, shared bool, err error) {
	mine, named, err := items(e, args)
	if err != nil {
		return "", false, err
	}
	if _, err := e.Parse(mine); err != nil {
		return "", false, err
	}
	for _, it := range named {
		if it.name != "store" {
			return "", false, Mistake("%q is not set up here: only the store's server is, as store [--listen host:port]", it.name)
		}
		set, err := settings(it)
		if err != nil {
			return "", false, err
		}
		shared, listen = true, set.listen
		if !set.listened {
			listen = storeListen
		}
	}
	return listen, shared, nil
}

// initStore is corbie init store: it sets up the store's server alone, on a
// config whose controller it proves the key of, or with no address stops
// the directory being served.
func initStore(e *Env, listen string) error {
	for _, flag := range []string{"ceiling", "deny", "name"} {
		if e.Given(flag) {
			return Mistake("--%s sets up the controller, the runner and the client, and store sets up only the store's server: run corbie init and corbie init store as two commands", flag)
		}
	}
	cfg, err := e.Config()
	if err != nil {
		return err
	}
	if !cfg.Has(identity.Controller) {
		return fmt.Errorf("this config has no controller, and the store's server proves the controller's key: corbie init sets the controller up first, and a store on a computer of its own is a bucket")
	}
	recorded, err := cfg.ControllerStore()
	if err != nil {
		return err
	}
	if err := servable(listen, cmp.Or(e.store, recorded)); err != nil {
		return err
	}
	if err := cfg.SetStoreListen(listen); err != nil {
		return err
	}
	done := struct {
		Served string `json:"served,omitzero"`
	}{}
	if listen != "" {
		if done.Served, err = servedAt(listen); err != nil {
			return err
		}
	}
	return e.Print(done, func() string {
		if done.Served == "" {
			return "This computer serves its store directory to no other computer.\n"
		}
		return "Other computers reach the directory at " + done.Served + " while corbie start runs here.\n"
	})
}

// servable says whether a directory can be served where its server is to
// listen: at a port, and only a directory.
func servable(listen, address string) error {
	if listen == "" {
		return nil
	}
	_, port, err := net.SplitHostPort(listen)
	if n, perr := strconv.Atoi(port); err != nil || perr != nil || n < 1 || n > 65535 {
		return Mistake("store --listen takes host:port, or :port for every network interface: :8372")
	}
	if !store.IsDirectory(address) {
		return Mistake("store is for a store that is a directory on this computer: a bucket is reached by every computer already")
	}
	return nil
}

// servedAt is the address other computers are given for a store's server that
// listens where listen says: the same, with this computer's host name where
// listen names every network interface.
func servedAt(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", err
	}
	if ip := net.ParseIP(host); host == "" || ip != nil && ip.IsUnspecified() {
		if host, err = os.Hostname(); err != nil {
			return "", err
		}
	}
	return store.ServedScheme + net.JoinHostPort(host, port), nil
}

func cmdInit(e *Env, args []string) error {
	var ceilingText, name string
	var deny list
	e.Flags.StringVar(&ceilingText, "ceiling", "", "")
	e.Flags.Var(&deny, "deny", "")
	e.Flags.StringVar(&name, "name", hostName(), "")
	listen, shared, err := sharing(e, args)
	if err != nil {
		return err
	}
	if shared {
		return initStore(e, listen)
	}
	cfg, err := e.Config()
	if err != nil {
		return err
	}
	// The store is the one named, or else the one the config records, so
	// that init run again keeps it, or else the one CORBIE_STORE names; a
	// config that records none has the directory store in it.
	if e.store == "" {
		if e.store, err = cfg.ControllerStore(); err != nil {
			return err
		}
	}
	if e.store == "" {
		e.store = os.Getenv("CORBIE_STORE")
	}
	if e.store == "" {
		e.store = filepath.Join(cfg.Dir, "store")
	}
	limit, err := ceiling(ceilingText, nil, deny, permission.Layer{})
	if err != nil {
		return err
	}
	if err := ownStore(e.store); err != nil {
		return err
	}
	address := e.store
	if store.IsDirectory(address) {
		if address, err = filepath.Abs(strings.TrimPrefix(address, "file://")); err != nil {
			return err
		}
	}
	s, err := store.Open(address)
	if err != nil {
		return err
	}
	if err := store.SetUp(e.ctx, s); err != nil {
		return err
	}
	// A controller this config has yet to make has signed nothing: trust
	// held in the store under its name is another controller's, and nothing
	// is made here under a name that is taken there.
	if !cfg.Has(identity.Controller) {
		held, err := s.First(e.ctx, store.TrustList(name+"-controller"), 1)
		if err != nil {
			return err
		}
		if len(held) > 0 {
			return fmt.Errorf("the name %q is taken on that store: another controller holds trust there under it. Give this computer's parties another name with --name", name+"-controller")
		}
	}
	ctl, err := cfg.Make(identity.Controller, name+"-controller")
	if err != nil {
		return err
	}
	run, err := cfg.Make(identity.Runner, name+"-runner")
	if err != nil {
		return err
	}
	cli, err := cfg.Make(identity.Client, name+"-client")
	if err != nil {
		return err
	}
	// The controller trusts the runner and the client; they trust it. A
	// name stays tied to the key it was first trusted with, here as when a
	// party is enrolled, and both are looked at before either is written.
	trusted := []identity.Trust{{Card: run.Card}, {Card: cli.Card, Ceiling: limit}}
	for _, t := range trusted {
		old, err := identity.ReadTrust(e.ctx, s, ctl.Card, t.Card.Name)
		if err != nil {
			return err
		}
		if _, err := identity.Take(old, t.Card, "", false); err != nil {
			return err
		}
	}
	if err := cfg.SetControllerStore(address); err != nil {
		return err
	}
	for _, t := range trusted {
		if err := identity.WriteTrust(e.ctx, s, ctl, t); err != nil {
			return err
		}
	}
	if err := cfg.SetTrust(identity.Runner, identity.Trust{Card: ctl.Card, Store: address, Ceiling: limit}); err != nil {
		return err
	}
	if err := cfg.SetTrust(identity.Client, identity.Trust{Card: ctl.Card, Store: address}); err != nil {
		return err
	}
	type party struct {
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		Fingerprint string `json:"fingerprint"`
	}
	done := struct {
		Config  string   `json:"config"`
		Store   string   `json:"store"`
		Served  string   `json:"served,omitzero"`
		Ceiling []string `json:"ceiling"`
		Deny    []string `json:"deny,omitzero"`
		Parties []party  `json:"parties"`
	}{Config: cfg.Dir, Store: address, Ceiling: append([]string{}, limit.Allow...), Deny: limit.Deny}
	if listen, err := cfg.StoreListen(); err != nil {
		return err
	} else if listen != "" {
		if done.Served, err = servedAt(listen); err != nil {
			return err
		}
	}
	for _, id := range []*identity.Identity{ctl, run, cli} {
		done.Parties = append(done.Parties, party{string(id.Kind), id.Name, id.Fingerprint()})
	}
	return e.Print(done, func() string {
		var b strings.Builder
		fmt.Fprintf(&b, "Set up in %s:\n", done.Config)
		for _, p := range done.Parties {
			fmt.Fprintf(&b, "  %-10s %s  (%s)\n", p.Kind, p.Name, p.Fingerprint)
		}
		fmt.Fprintf(&b, "The store is %s.\n", address)
		if store.IsDirectory(address) {
			b.WriteString("A directory keeps no party out: everyone on this computer who can read it can read and write all of it.\n")
		}
		if len(limit.Allow) == 0 {
			b.WriteString("The ceiling is empty: jobs on this computer may do nothing outside themselves.\nTo change that: corbie init --ceiling all   (or network, disk, exec, env, or single entries)\n")
		} else {
			fmt.Fprintf(&b, "The ceiling, which is the most jobs on this computer may do: %s\n", strings.Join(limit.Allow, ", "))
		}
		if len(limit.Deny) > 0 {
			fmt.Fprintf(&b, "Whatever the ceiling allows, it refuses: %s\n", strings.Join(limit.Deny, ", "))
		}
		if done.Served != "" {
			fmt.Fprintf(&b, "Other computers reach the directory at %s while corbie start runs here.\n", done.Served)
		}
		b.WriteString("Next: corbie start\n")
		return b.String()
	})
}

// card is `corbie runner card`, `corbie client card` and `corbie
// controller card`: it makes the party's identity if it has none, and
// prints its card. The card alone goes to standard output, which the next
// step of a pipe reads.
func cmdCard(kind identity.Kind) func(e *Env, args []string) error {
	return func(e *Env, args []string) error {
		var name string
		e.Flags.StringVar(&name, "name", hostName()+"-"+string(kind), "")
		if _, err := e.Parse(args); err != nil {
			return err
		}
		cfg, err := e.Config()
		if err != nil {
			return err
		}
		if kind == identity.Controller {
			// A controller's identity is made together with the address
			// of its store.
			recorded, err := cfg.ControllerStore()
			if err != nil {
				return err
			}
			if e.store == "" && recorded == "" {
				return Mistake("the store's address is needed to make a controller: --store <address>")
			}
			if e.store != "" {
				if err := ownStore(e.store); err != nil {
					return err
				}
				s, err := store.Open(e.store)
				if err != nil {
					return err
				}
				if err := store.SetUp(e.ctx, s); err != nil {
					return err
				}
				if err := cfg.SetControllerStore(e.store); err != nil {
					return err
				}
			}

		}
		id, err := cfg.Make(kind, name)
		if err != nil {
			return err
		}
		e.Say("%s %s  (%s)", id.Kind, id.Name, id.Fingerprint())
		shown := map[string]string{"card": id.Card.String(), "kind": string(id.Kind), "name": id.Name, "fingerprint": id.Fingerprint()}
		return e.Print(shown, func() string { return id.Card.String() + "\n" })
	}
}

// read gives the one thing a step of a pipe was handed: its argument, or
// with "-" everything on standard input, read to the end.
func (e *Env) read(rest []string, what string) ([]string, error) {
	text := strings.Join(rest, " ")
	if len(rest) == 1 && rest[0] == "-" {
		data, err := io.ReadAll(e.In)
		if err != nil {
			return nil, err
		}
		text = string(data)
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, Mistake("nothing was given: %s was expected (the step before in the pipe may have failed)", what)
	}
	return fields, nil
}

func cmdEnrol(e *Env, args []string) error {
	var ceilingText, tags string
	var allow, deny list
	var replace bool
	e.Flags.StringVar(&ceilingText, "ceiling", "", "")
	e.Flags.Var(&allow, "allow", "")
	e.Flags.Var(&deny, "deny", "")
	e.Flags.StringVar(&tags, "tags", "", "")
	e.Flags.BoolVar(&replace, "replace", false, "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	given, err := e.read(rest, "a runner's or a client's card, or the name of a party already enrolled")
	if err != nil {
		return err
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	var card identity.Card
	var old *identity.Trust
	if strings.HasPrefix(given[0], "corbie-card:") {
		if card, err = identity.ParseCard(given[0]); err != nil {
			return err
		}
		if card.Kind == identity.Controller {
			return fmt.Errorf("this is the card of a controller, and a runner's or a client's is needed")
		}
		if card.Name == ctl.Card.Name {
			return fmt.Errorf("%q is the controller's own name", card.Name)
		}
		if old, err = identity.ReadTrust(e.ctx, s, ctl.Card, card.Name); err != nil {
			return err
		}
	} else {
		// A party already enrolled is named, to set again what it was given.
		if old, err = identity.ReadTrust(e.ctx, s, ctl.Card, given[0]); err != nil {
			return err
		}
		if old == nil {
			return fmt.Errorf("no runner or client named %q is enrolled, and that is not a card", given[0])
		}
		card = old.Card
	}
	trust, err := identity.Take(old, card, "", replace)
	if err != nil {
		return err
	}
	if trust.Ceiling, err = ceiling(ceilingText, allow, deny, has(old, card)); err != nil {
		return err
	}
	for _, tag := range strings.Split(tags, ",") {
		if tag = strings.TrimSpace(tag); tag != "" && tags != none {
			trust.Tags = append(trust.Tags, tag)
		}
	}
	// With no --tags the tags on record stay, and --tags none takes them
	// away.
	limited := ceilingText != "" || len(allow)+len(deny) > 0
	if tags == "" && old != nil && old.Card.Same(card) {
		trust.Tags = old.Tags
	}
	if card.Kind == identity.Runner && limited {
		return Mistake("a runner is given tags; its ceiling is the runner's own to set, with corbie runner trust")
	}
	if card.Kind == identity.Client && tags != "" {
		return Mistake("a client is given a ceiling; tags are for runners")
	}
	// The card and the address are printed only after the trust is
	// written.
	if err := identity.WriteTrust(e.ctx, s, ctl, trust); err != nil {
		return err
	}
	cfg, _ := e.Config()
	recorded, _ := cfg.ControllerStore()
	address, err := e.address(recorded)
	if err != nil {
		return err
	}
	// A directory that is served is given by the address other computers reach
	// it at: the path means nothing anywhere else.
	if listen, _ := cfg.StoreListen(); listen != "" && address == recorded {
		if address, err = servedAt(listen); err != nil {
			return err
		}
	}
	e.Say("enrolled the %s %s  (%s)", card.Kind, card.Name, card.Fingerprint())
	e.Say("this controller is %s  (%s)", ctl.Name, ctl.Fingerprint())
	return e.Print(map[string]string{"card": ctl.Card.String(), "store": address}, func() string {
		return ctl.Card.String() + "\n" + address + "\n"
	})
}

// trust is `corbie runner trust` and `corbie client trust`: it takes what a
// controller's enrol printed, its card and the address of its store.
func cmdTrust(kind identity.Kind) func(e *Env, args []string) error {
	return func(e *Env, args []string) error {
		var ceilingText string
		var allow, deny list
		var replace bool
		if kind == identity.Runner {
			// A runner gives a ceiling; a client gives none.
			e.Flags.StringVar(&ceilingText, "ceiling", "", "")
			e.Flags.Var(&allow, "allow", "")
			e.Flags.Var(&deny, "deny", "")
		}
		e.Flags.BoolVar(&replace, "replace", false, "")
		rest, err := e.Parse(args)
		if err != nil {
			return err
		}
		// The input is read to its end before the config is looked at: the
		// commands of a pipe start together.
		given, err := e.read(rest, "what corbie controller enrol printed: the controller's card and the address of its store")
		if err != nil {
			return err
		}
		cfg, err := e.Config()
		if err != nil {
			return err
		}
		var card identity.Card
		var old *identity.Trust
		address := ""
		if strings.HasPrefix(given[0], "corbie-card:") {
			if card, err = identity.ParseCard(given[0]); err != nil {
				return err
			}
			if len(given) > 1 {
				address = given[1]
			}
			if old, err = cfg.Trust(kind, card.Name); err != nil {
				return err
			}
		} else {
			if old, err = cfg.Trust(kind, given[0]); err != nil || old == nil {
				return fmt.Errorf("no controller named %q is trusted here, and that is not a card", given[0])
			}
			card = old.Card
		}
		trust, err := identity.Take(old, card, identity.Controller, replace)
		if err != nil {
			return err
		}
		if kind == identity.Client {
			// A client has one controller.
			all, err := cfg.TrustList(kind)
			if err != nil {
				return err
			}
			for _, other := range all {
				if other.Trusted() && other.Card.Name != card.Name {
					return fmt.Errorf("this client already has the controller %q; remove it first (corbie client remove %s)", other.Card.Name, other.Card.Name)
				}
			}
		}
		if trust.Ceiling, err = ceiling(ceilingText, allow, deny, has(old, card)); err != nil {
			return err
		}
		trust.Store = address
		if address == "" && old != nil {
			trust.Store = old.Store
		}
		if trust.Store == "" {
			return Mistake("the address of the controller's store is needed after its card")
		}
		if err := cfg.SetTrust(kind, trust); err != nil {
			return err
		}
		e.Say("this %s now trusts the controller %s  (%s)", kind, card.Name, card.Fingerprint())
		return nil
	}
}

// remove is `corbie runner remove` and `corbie client remove`. The
// controller stays on record with its key, marked as removed.
func cmdUntrust(kind identity.Kind) func(e *Env, args []string) error {
	return func(e *Env, args []string) error {
		rest, err := e.Parse(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return Mistake("name the controller to remove")
		}
		cfg, err := e.Config()
		if err != nil {
			return err
		}
		trust, err := cfg.Trust(kind, rest[0])
		if err != nil {
			return err
		}
		if trust == nil {
			return fmt.Errorf("no controller named %q is trusted here", rest[0])
		}
		trust.Removed = true
		return cfg.SetTrust(kind, *trust)
	}
}

func cmdRemove(e *Env, args []string) error {
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return Mistake("name the party to remove")
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	trust, err := identity.ReadTrust(e.ctx, s, ctl.Card, rest[0])
	if err != nil {
		return err
	}
	if trust == nil {
		return fmt.Errorf("no runner or client named %q is enrolled", rest[0])
	}
	// Its name stays on record with its key, marked as removed.
	trust.Removed = true
	return identity.WriteTrust(e.ctx, s, ctl, *trust)
}

func cmdParties(e *Env, args []string) error {
	if _, err := e.Parse(args); err != nil {
		return err
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	all, err := identity.ListTrust(e.ctx, s, ctl.Card)
	if err != nil {
		return err
	}
	type row struct {
		Name        string   `json:"name"`
		Kind        string   `json:"kind"`
		Fingerprint string   `json:"fingerprint"`
		Ceiling     []string `json:"ceiling,omitzero"`
		Deny        []string `json:"deny,omitzero"`
		Tags        []string `json:"tags,omitzero"`
		Removed     bool     `json:"removed,omitzero"`
	}
	rows := []row{}
	for _, t := range all {
		rows = append(rows, row{t.Card.Name, string(t.Card.Kind), t.Card.Fingerprint(), t.Ceiling.Allow, t.Ceiling.Deny, t.Tags, t.Removed})
	}
	return e.Print(rows, func() string {
		var b strings.Builder
		for _, r := range rows {
			fmt.Fprintf(&b, "%-8s %s  (%s)", r.Kind, r.Name, r.Fingerprint)
			if len(r.Ceiling) > 0 {
				fmt.Fprintf(&b, "  ceiling: %s", strings.Join(r.Ceiling, ","))
			}
			if len(r.Deny) > 0 {
				fmt.Fprintf(&b, "  deny: %s", strings.Join(r.Deny, ","))
			}
			if len(r.Tags) > 0 {
				fmt.Fprintf(&b, "  tags: %s", strings.Join(r.Tags, ","))
			}
			if r.Removed {
				b.WriteString("  removed")
			}
			b.WriteString("\n")
		}
		return b.String()
	})
}

func cmdSecretSet(e *Env, args []string) error {
	var fromEnv bool
	e.Flags.BoolVar(&fromEnv, "env", false, "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return Mistake("name the secret")
	}
	name := rest[0]
	if err := checkSecretName(name); err != nil {
		return err
	}
	fromStdin := len(rest) == 2 && rest[1] == "-"
	if len(rest) != 1 && !fromStdin {
		return Mistake("name the secret, and nothing else: a secret's value is never given on the command line")
	}
	if fromEnv && fromStdin {
		return Mistake("--env and - each give the value: give one")
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	var value string
	switch {
	case fromEnv:
		value, err = secretFromEnv(name)
	case fromStdin:
		var data []byte
		data, err = io.ReadAll(e.In)
		value = strings.TrimRight(string(data), "\r\n")
		if err == nil && value == "" {
			err = Mistake("standard input gave no value for the secret %q", name)
		}
	default:
		value, err = secretTyped(e, name, "--env reads it from the environment variable, and - from standard input")
	}
	if err != nil {
		return err
	}
	return secret.Set(e.ctx, s, ctl, name, value)
}

// checkSecretName refuses a name a secret cannot have, and a value written
// in place of a name. A value is never given on the command line, where
// every user of the computer can read it, and is never repeated.
func checkSecretName(name string) error {
	if strings.Contains(name, "=") {
		return Mistake("a secret's value is never given on the command line: give the secret's name alone")
	}
	if err := permission.CheckSecretName(name); err != nil {
		return Mistake("%v", err)
	}
	return nil
}

// secretFromEnv reads a secret's value from the environment variable of its
// name.
func secretFromEnv(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	switch {
	case !ok:
		return "", Mistake("the environment variable %q is not set", name)
	case value == "":
		return "", Mistake("the environment variable %q is empty", name)
	}
	return value, nil
}

// secretTyped asks for a secret's value at the terminal, and does not show
// what is typed. Without a terminal it is a mistake, which says what reads
// the value otherwise.
func secretTyped(e *Env, name, otherwise string) (string, error) {
	in, ok := e.In.(*os.File)
	if !ok || !term.IsTerminal(int(in.Fd())) {
		return "", Mistake("the secret %q is typed at a terminal, and there is none: %s", name, otherwise)
	}
	fd := int(in.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return "", err
	}
	type line struct {
		typed []byte
		err   error
	}
	read := make(chan line, 1)
	fmt.Fprintf(e.Err, "%s: ", name)
	go func() {
		typed, err := term.ReadPassword(fd)
		read <- line{typed, err}
	}()
	// An interrupt ends the command at once, with the terminal as it was.
	var got line
	select {
	case got = <-read:
	case <-e.ctx.Done():
		got.err = e.ctx.Err()
		term.Restore(fd, state)
	}
	fmt.Fprintln(e.Err)
	switch {
	case got.err != nil:
		return "", got.err
	case len(got.typed) == 0:
		return "", Mistake("nothing was typed for the secret %q", name)
	}
	return string(got.typed), nil
}

func cmdSecretRemove(e *Env, args []string) error {
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return Mistake("name the secret to remove")
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	return secret.Remove(e.ctx, s, ctl, rest[0])
}

func cmdSecretList(e *Env, args []string) error {
	if _, err := e.Parse(args); err != nil {
		return err
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	names, err := secret.List(e.ctx, s, ctl.Name)
	if err != nil {
		return err
	}
	if names == nil {
		names = []string{}
	}
	return e.Print(names, func() string {
		if len(names) == 0 {
			return ""
		}
		return strings.Join(names, "\n") + "\n"
	})
}

// cmdControllerCancel cancels any run, whichever client started it, with
// the controller's own key.
func cmdControllerCancel(e *Env, args []string) error {
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return Mistake("name one run by its ID")
	}
	if err := store.CheckInstance(rest[0]); err != nil {
		return Mistake("%v", err)
	}
	ctl, s, err := e.Controller()
	if err != nil {
		return err
	}
	// A run has a head from its first step until it is past keeping.
	head, _, err := instance.ReadHead(e.ctx, s, ctl.Card, rest[0])
	if err != nil {
		return err
	}
	if head == nil {
		return client.ErrNoRun
	}
	var sender store.Sender
	now := time.Now()
	name := store.Message(ctl.Name, ctl.Name, "owner-"+sender.Next(now))
	data, err := ctl.Write(name, now, now.Add(24*time.Hour), &instance.Message{Kind: instance.KindCancel, Instance: rest[0]}, ctl.Card)
	if err != nil {
		return err
	}
	if _, err := s.Write(e.ctx, name, data, store.OnlyIfNew); err != nil {
		return err
	}
	e.Say("the cancel is in the controller's inbox; the controller acts on it at its next pass")
	return nil
}

// cmdPolicy prints the policy that limits one party's key to a bucket to
// what that party may touch.
func cmdPolicy(e *Env, args []string) error {
	var viewer bool
	e.Flags.BoolVar(&viewer, "viewer", false, "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	cfg, err := e.Config()
	if err != nil {
		return err
	}
	ctl, err := cfg.Load(identity.Controller)
	if err != nil {
		return err
	}
	recorded, _ := cfg.ControllerStore()
	address, err := e.address(recorded)
	if err != nil {
		return err
	}
	if store.IsDirectory(address) {
		return fmt.Errorf("the store is a directory, which keeps no party out of anything: a policy is for a bucket")
	}
	party := store.Party{Kind: store.Viewer, Controller: ctl.Name}
	switch {
	case viewer && len(rest) == 0:
	case !viewer && len(rest) == 1 && rest[0] == ctl.Name:
		party = store.Party{Kind: store.Controller, Name: ctl.Name}
	case !viewer && len(rest) == 1:
		s, err := e.Open(recorded, ctl, ctl.Card)
		if err != nil {
			return err
		}
		trust, err := identity.ReadTrust(e.ctx, s, ctl.Card, rest[0])
		if err != nil {
			return err
		}
		if trust == nil {
			return fmt.Errorf("no runner or client named %q is enrolled", rest[0])
		}
		party.Name, party.Kind = trust.Card.Name, store.Client
		if trust.Card.Kind == identity.Runner {
			party.Kind = store.Runner
		}
	default:
		return Mistake("name one party, or give --viewer for one who may only see where runs stand")
	}
	policy, err := store.PolicyFor(address, party)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(e.Out, policy)
	return err
}
