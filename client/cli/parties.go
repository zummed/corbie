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
			return permission.Layer{}, mistake("%s: %v", given.flag, err)
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
		return mistake("a controller's store is its own directory, or a bucket: %q is how other computers reach a directory that a controller's computer serves", address)
	}
	return nil
}

// servable says whether a directory can be served where --serve says: at a
// host and a port other computers reach this one by, and only a directory.
func servable(serve, address string) error {
	if serve == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(serve)
	if n, perr := strconv.Atoi(port); err != nil || host == "" || perr != nil || n < 1 || n > 65535 {
		return mistake("--serve takes host:port, the name other computers reach this one by and a port: desk:8372")
	}
	if !store.IsDirectory(address) {
		return mistake("--serve is for a store that is a directory on this computer: a bucket is reached by every computer already")
	}
	return nil
}

func cmdInit(e *env, args []string) error {
	var ceilingText, name string
	var deny list
	e.flags.StringVar(&ceilingText, "ceiling", "", "")
	e.flags.Var(&deny, "deny", "")
	e.flags.StringVar(&name, "name", hostName(), "")
	var serve string
	e.flags.StringVar(&serve, "serve", "", "")
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return mistake("init takes no arguments, only flags")
	}
	if e.store == "" {
		e.store = os.Getenv("CORBIE_STORE")
	}
	if e.store == "" {
		return mistake("the store's address is needed: --store <directory or bucket>")
	}
	limit, err := ceiling(ceilingText, nil, deny, permission.Layer{})
	if err != nil {
		return err
	}
	if err := ownStore(e.store); err != nil {
		return err
	}
	cfg, err := e.cfg()
	if err != nil {
		return err
	}
	address := e.store
	if store.IsDirectory(address) {
		if address, err = filepath.Abs(strings.TrimPrefix(address, "file://")); err != nil {
			return err
		}
	}
	if err := servable(serve, address); err != nil {
		return err
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
	// Where the directory is served is set again each time, as the ceiling
	// is: with no --serve, this computer serves nothing.
	if err := cfg.SetServed(serve); err != nil {
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
	if at, err := cfg.Served(); err != nil {
		return err
	} else if at != "" {
		done.Served = store.ServedScheme + at
	}
	for _, id := range []*identity.Identity{ctl, run, cli} {
		done.Parties = append(done.Parties, party{string(id.Kind), id.Name, id.Fingerprint()})
	}
	return e.print(done, func() string {
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
			b.WriteString("The ceiling is empty: jobs on this computer may do nothing outside themselves.\nTo change that: corbie init --store <same> --ceiling all   (or network, disk, exec, env, or single entries)\n")
		} else {
			fmt.Fprintf(&b, "The ceiling, which is the most jobs on this computer may do: %s\n", strings.Join(limit.Allow, ", "))
		}
		if len(limit.Deny) > 0 {
			fmt.Fprintf(&b, "Whatever the ceiling allows, it refuses: %s\n", strings.Join(limit.Deny, ", "))
		}
		if done.Served != "" {
			fmt.Fprintf(&b, "Other computers reach the directory at %s while corbie run store is running here.\n", done.Served)
			b.WriteString("Next: corbie run controller runner store\n")
			return b.String()
		}
		b.WriteString("Next: corbie run controller runner\n")
		return b.String()
	})
}

// card is `corbie runner card`, `corbie client card` and `corbie
// controller card`: it makes the party's identity if it has none, and
// prints its card. The card alone goes to standard output, which the next
// step of a pipe reads.
func cmdCard(kind identity.Kind) func(e *env, args []string) error {
	return func(e *env, args []string) error {
		var name, serve string
		e.flags.StringVar(&name, "name", hostName()+"-"+string(kind), "")
		if kind == identity.Controller {
			e.flags.StringVar(&serve, "serve", "", "")
		}
		if _, err := e.parse(args); err != nil {
			return err
		}
		cfg, err := e.cfg()
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
				return mistake("the store's address is needed to make a controller: --store <address>")
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
			if serve != "" {
				if err := servable(serve, cmp.Or(e.store, recorded)); err != nil {
					return err
				}
				if err := cfg.SetServed(serve); err != nil {
					return err
				}
			}
		}
		id, err := cfg.Make(kind, name)
		if err != nil {
			return err
		}
		e.say("%s %s  (%s)", id.Kind, id.Name, id.Fingerprint())
		shown := map[string]string{"card": id.Card.String(), "kind": string(id.Kind), "name": id.Name, "fingerprint": id.Fingerprint()}
		return e.print(shown, func() string { return id.Card.String() + "\n" })
	}
}

// read gives the one thing a step of a pipe was handed: its argument, or
// with "-" everything on standard input, read to the end.
func (e *env) read(rest []string, what string) ([]string, error) {
	text := strings.Join(rest, " ")
	if len(rest) == 1 && rest[0] == "-" {
		data, err := io.ReadAll(e.in)
		if err != nil {
			return nil, err
		}
		text = string(data)
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, mistake("nothing was given: %s was expected (the step before in the pipe may have failed)", what)
	}
	return fields, nil
}

func cmdEnrol(e *env, args []string) error {
	var ceilingText, tags string
	var allow, deny list
	var replace bool
	e.flags.StringVar(&ceilingText, "ceiling", "", "")
	e.flags.Var(&allow, "allow", "")
	e.flags.Var(&deny, "deny", "")
	e.flags.StringVar(&tags, "tags", "", "")
	e.flags.BoolVar(&replace, "replace", false, "")
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	given, err := e.read(rest, "a runner's or a client's card, or the name of a party already enrolled")
	if err != nil {
		return err
	}
	ctl, s, err := e.controller()
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
		return mistake("a runner is given tags; its ceiling is the runner's own to set, with corbie runner trust")
	}
	if card.Kind == identity.Client && tags != "" {
		return mistake("a client is given a ceiling; tags are for runners")
	}
	// The card and the address are printed only after the trust is
	// written.
	if err := identity.WriteTrust(e.ctx, s, ctl, trust); err != nil {
		return err
	}
	cfg, _ := e.cfg()
	recorded, _ := cfg.ControllerStore()
	address, err := e.address(recorded)
	if err != nil {
		return err
	}
	// A directory that is served is given by the address other computers reach
	// it at: the path means nothing anywhere else.
	if at, _ := cfg.Served(); at != "" && address == recorded {
		address = store.ServedScheme + at
	}
	e.say("enrolled the %s %s  (%s)", card.Kind, card.Name, card.Fingerprint())
	e.say("this controller is %s  (%s)", ctl.Name, ctl.Fingerprint())
	return e.print(map[string]string{"card": ctl.Card.String(), "store": address}, func() string {
		return ctl.Card.String() + "\n" + address + "\n"
	})
}

// trust is `corbie runner trust` and `corbie client trust`: it takes what a
// controller's enrol printed, its card and the address of its store.
func cmdTrust(kind identity.Kind) func(e *env, args []string) error {
	return func(e *env, args []string) error {
		var ceilingText string
		var allow, deny list
		var replace bool
		if kind == identity.Runner {
			// A runner gives a ceiling; a client gives none.
			e.flags.StringVar(&ceilingText, "ceiling", "", "")
			e.flags.Var(&allow, "allow", "")
			e.flags.Var(&deny, "deny", "")
		}
		e.flags.BoolVar(&replace, "replace", false, "")
		rest, err := e.parse(args)
		if err != nil {
			return err
		}
		// The input is read to its end before the config is looked at: the
		// commands of a pipe start together.
		given, err := e.read(rest, "what corbie controller enrol printed: the controller's card and the address of its store")
		if err != nil {
			return err
		}
		cfg, err := e.cfg()
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
			return mistake("the address of the controller's store is needed after its card")
		}
		if err := cfg.SetTrust(kind, trust); err != nil {
			return err
		}
		e.say("this %s now trusts the controller %s  (%s)", kind, card.Name, card.Fingerprint())
		return nil
	}
}

// remove is `corbie runner remove` and `corbie client remove`. The
// controller stays on record with its key, marked as removed.
func cmdUntrust(kind identity.Kind) func(e *env, args []string) error {
	return func(e *env, args []string) error {
		rest, err := e.parse(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return mistake("name the controller to remove")
		}
		cfg, err := e.cfg()
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

func cmdRemove(e *env, args []string) error {
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return mistake("name the party to remove")
	}
	ctl, s, err := e.controller()
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

func cmdParties(e *env, args []string) error {
	if _, err := e.parse(args); err != nil {
		return err
	}
	ctl, s, err := e.controller()
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
	return e.print(rows, func() string {
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

func cmdSecretSet(e *env, args []string) error {
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return mistake("name the secret; its value is read from the environment variable of that name, or from standard input")
	}
	// The value is never given on the command line.
	value, ok := os.LookupEnv(rest[0])
	if !ok {
		data, err := io.ReadAll(e.in)
		if err != nil {
			return err
		}
		value = strings.TrimRight(string(data), "\r\n")
	}
	if value == "" {
		return mistake("the secret has no value: set the environment variable %q, or give the value on standard input", rest[0])
	}
	ctl, s, err := e.controller()
	if err != nil {
		return err
	}
	return secret.Set(e.ctx, s, ctl, rest[0], value)
}

func cmdSecretRemove(e *env, args []string) error {
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return mistake("name the secret to remove")
	}
	ctl, s, err := e.controller()
	if err != nil {
		return err
	}
	return secret.Remove(e.ctx, s, ctl, rest[0])
}

func cmdSecretList(e *env, args []string) error {
	if _, err := e.parse(args); err != nil {
		return err
	}
	ctl, s, err := e.controller()
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
	return e.print(names, func() string {
		if len(names) == 0 {
			return ""
		}
		return strings.Join(names, "\n") + "\n"
	})
}

// cmdControllerCancel cancels any run, whichever client started it, with
// the controller's own key.
func cmdControllerCancel(e *env, args []string) error {
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return mistake("name one run by its ID")
	}
	if err := store.CheckInstance(rest[0]); err != nil {
		return mistake("%v", err)
	}
	ctl, s, err := e.controller()
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
	e.say("the cancel is in the controller's inbox; the controller acts on it at its next pass")
	return nil
}

// cmdPolicy prints the policy that limits one party's key to a bucket to
// what that party may touch.
func cmdPolicy(e *env, args []string) error {
	var viewer bool
	e.flags.BoolVar(&viewer, "viewer", false, "")
	rest, err := e.parse(args)
	if err != nil {
		return err
	}
	cfg, err := e.cfg()
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
		s, err := e.open(recorded, ctl, ctl.Card)
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
		return mistake("name one party, or give --viewer for one who may only see where runs stand")
	}
	policy, err := store.PolicyFor(address, party)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(e.out, policy)
	return err
}
