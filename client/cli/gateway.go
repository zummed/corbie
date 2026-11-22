package cli

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// gateway starts the gateway beside whatever other roles the process has,
// and gives what stops it. A gateway acts as the clients of the configs
// --client names, or with none named as the client of this config, and each
// of its tokens names one of them.
func (e *Env) gateway() (func(), error) {
	if role.Serverless() {
		return nil, fmt.Errorf("a gateway needs a process that stays up, and this is a serverless host")
	}
	var clients []*client.Client
	if len(e.clients) == 0 {
		c, err := e.Client()
		if err != nil {
			return nil, err
		}
		if c.ID == nil {
			return nil, fmt.Errorf("a gateway acts as this computer's client, and there is none (corbie client card makes one)")
		}
		clients = append(clients, c)
	}
	for _, dir := range e.clients {
		c, err := e.clientOf(identity.Config{Dir: dir})
		switch {
		case err != nil:
			return nil, fmt.Errorf("--client %s: %w", dir, err)
		case c.ID == nil:
			return nil, fmt.Errorf("--client %s: the config holds no client (corbie client card makes one)", dir)
		case slices.ContainsFunc(clients, func(o *client.Client) bool { return o.ID.Name == c.ID.Name }):
			return nil, fmt.Errorf("--client %s: the gateway has a client named %s already", dir, c.ID.Name)
		}
		clients = append(clients, c)
	}
	var names []string
	for _, c := range clients {
		names = append(names, c.ID.Name)
	}
	var callers map[string]*client.Client
	first := ""
	if e.tokenFile != "" {
		data, err := os.ReadFile(e.tokenFile)
		if err != nil {
			return nil, err
		}
		if callers, first, err = callersOf(string(data), clients); err != nil {
			return nil, fmt.Errorf("%s: %w", e.tokenFile, err)
		}
		// What the file gave, by client, so that a line read otherwise than
		// it was meant shows when the gateway starts.
		for _, c := range clients {
			var persons []string
			tokens := 0
			for _, named := range callers {
				if named.ID.Name != c.ID.Name {
					continue
				}
				tokens++
				if named.Person != "" {
					persons = append(persons, named.Person)
				}
			}
			slices.Sort(persons)
			line := fmt.Sprintf("gateway tokens for the client %s: %d", c.ID.Name, tokens)
			if len(persons) > 0 {
				line += ", for " + strings.Join(persons, ", ")
			}
			if len(callers) > 0 {
				e.Say("%s", line)
			}
		}
	}
	// The tokens of an OIDC issuer are taken where one is given.
	var issuer *gateway.Issuer
	if o := e.oidc; o.issuer != "" || o.audience != "" {
		if o.issuer == "" || o.audience == "" {
			return nil, fmt.Errorf("--oidc-issuer and --oidc-audience are given together: the issuer, and who its tokens must be for")
		}
		issuer = &gateway.Issuer{Address: o.issuer, Audience: o.audience, ClientClaim: o.client, PersonClaim: o.person, Clients: clients}
		if err := issuer.Open(e.ctx); err != nil {
			return nil, err
		}
	}
	if len(callers) == 0 && issuer == nil {
		// With no token given, a new one for each client.
		callers = map[string]*client.Client{}
		for _, c := range clients {
			random := make([]byte, 24)
			rand.Read(random)
			token := hex.EncodeToString(random)
			callers[token] = c
			if first == "" {
				first = token
			}
			if len(clients) == 1 {
				e.Say("gateway token (callers send it as Authorization: Bearer …): %s", token)
			} else {
				e.Say("gateway token for the client %s (callers send it as Authorization: Bearer …): %s", c.ID.Name, token)
			}
		}
	}
	listener, err := net.Listen("tcp", e.listen)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: &gateway.Gateway{Callers: callers, Issuer: issuer}, ReadHeaderTimeout: 10 * time.Second}
	go server.Serve(listener)
	e.gatewayAt, e.gatewayToken = "http://"+listener.Addr().String(), first
	if issuer != nil {
		e.Say("gateway takes the tokens of the OIDC issuer %s for the audience %s", issuer.Address, issuer.Audience)
		e.Say("gateway takes a token as the first of the clients %s that its claim %s names, for the person in its claim %s", strings.Join(names, ", "), issuer.ClientClaim, issuer.PersonClaim)
	}
	if len(names) == 1 {
		e.Say("gateway for the client %s, listening on http://%s", names[0], listener.Addr())
	} else {
		e.Say("gateway for the clients %s, listening on http://%s", strings.Join(names, ", "), listener.Addr())
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}, nil
}

// callersOf reads a gateway's token file: a line for each token, which
// holds the token, then the name of the client it names, then the person it
// was given to. A line with a token alone names the gateway's one client. It
// gives each token's client, sending for the line's person, and the first
// token.
func callersOf(text string, clients []*client.Client) (callers map[string]*client.Client, first string, err error) {
	var names []string
	for _, c := range clients {
		names = append(names, c.ID.Name)
	}
	callers, lines := map[string]*client.Client{}, map[string]int{}
	for i, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		token, name, person := fields[0], "", ""
		if len(fields) > 1 {
			name, person = fields[1], strings.Join(fields[2:], " ")
		}
		var named *client.Client
		for _, c := range clients {
			if c.ID.Name == name || name == "" && len(clients) == 1 {
				named = c
			}
		}
		switch {
		case lines[token] > 0:
			return nil, "", fmt.Errorf("line %d has the token of line %d: a token names one client", i+1, lines[token])
		case named == nil && name == "":
			return nil, "", fmt.Errorf("line %d names no client, and the gateway has several: %s", i+1, strings.Join(names, ", "))
		case named == nil:
			return nil, "", fmt.Errorf("line %d names the client %s, and the gateway's clients are: %s", i+1, name, strings.Join(names, ", "))
		}
		callers[token], lines[token] = named.As(person), i+1
		if first == "" {
			first = token
		}
	}
	return callers, first, nil
}

// serve starts serving the controller's directory to other computers, beside
// whatever other roles the process has, and gives what stops it. Each
// request comes by a connection its party made with its own key, and the
// party is found in the controller's own list of who it trusts, read afresh
// each time: a party that is removed is turned away from then on.
func (e *Env) serve(listen string, timeout time.Duration) (func(), error) {
	if role.Serverless() {
		return nil, fmt.Errorf("serving a directory needs a process that stays up, and this is a serverless host")
	}
	cfg, err := e.Config()
	if err != nil {
		return nil, err
	}
	if !cfg.Has(identity.Controller) {
		return nil, fmt.Errorf("a directory is served by the computer that holds its controller, and this config has none")
	}
	recorded, err := cfg.ControllerStore()
	if err != nil {
		return nil, err
	}
	address, err := e.address(recorded)
	if err != nil {
		return nil, err
	}
	if !store.IsDirectory(address) {
		return nil, fmt.Errorf("only a directory on this computer is served, and this controller's store is not one")
	}
	saved, err := cfg.StoreListen()
	if err != nil {
		return nil, err
	}
	if saved == "" {
		return nil, fmt.Errorf("this computer's store's server is not set up: corbie init store sets it up")
	}
	at, err := servedAt(saved)
	if err != nil {
		return nil, err
	}
	ctl, directory, err := e.Controller()
	if err != nil {
		return nil, err
	}
	// By default it listens where the config says.
	listen = cmp.Or(listen, saved)
	proof, err := ctl.Proof()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	who := func(ctx context.Context, name string) (store.Party, ed25519.PublicKey, error) {
		trust, err := identity.ReadTrust(ctx, directory, ctl.Card, name)
		if err != nil || !trust.Trusted() {
			return store.Party{}, nil, err
		}
		kind := store.Client
		if trust.Card.Kind == identity.Runner {
			kind = store.Runner
		}
		return store.Party{Kind: kind, Name: name, Controller: ctl.Name}, trust.Card.SignKey(), nil
	}
	served := &store.Server{Directory: directory, Who: who, Timeout: timeout,
		Failed: func(err error) { e.Say("store: %s", e.clean(err.Error())) }}
	server := served.HTTP()
	go server.Serve(store.ServedListener(listener, proof))
	e.Say("store of the controller %s, served at %s, listening on %s", ctl.Name, at, listener.Addr())
	return func() {
		// Every request in hand is answered before the server goes: one
		// cut off would leave its party not knowing whether it was done.
		served.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		server.Close()
	}, nil
}
