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
	"strings"
	"time"

	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/store"
)

// gateway starts the gateway beside whatever other roles the process has,
// and gives what stops it. A gateway is one client: every caller's run is
// started by the client of this config.
func (e *Env) gateway() (func(), error) {
	if role.Serverless() {
		return nil, fmt.Errorf("a gateway needs a process that stays up, and this is a serverless host")
	}
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	if c.ID == nil {
		return nil, fmt.Errorf("a gateway acts as this computer's client, and there is none (corbie client card makes one)")
	}
	token := ""
	if e.tokenFile != "" {
		data, err := os.ReadFile(e.tokenFile)
		if err != nil {
			return nil, err
		}
		token = strings.TrimSpace(string(data))
	}
	if token == "" {
		random := make([]byte, 24)
		rand.Read(random)
		token = hex.EncodeToString(random)
		e.Say("gateway token (callers send it as Authorization: Bearer …): %s", token)
	}
	listener, err := net.Listen("tcp", e.listen)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: &gateway.Gateway{Client: c, Token: token}, ReadHeaderTimeout: 10 * time.Second}
	go server.Serve(listener)
	e.Say("gateway for the client %s, listening on http://%s", c.ID.Name, listener.Addr())
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}, nil
}

// serve starts serving the controller's directory to other computers, beside
// whatever other roles the process has, and gives what stops it. Each
// request comes by a connection its party made with its own key, and the
// party is found in the controller's own list of who it trusts, read afresh
// each time: a party that is removed is turned away from then on.
func (e *Env) serve(listen string) (func(), error) {
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
	served := &store.Server{Directory: directory, Who: who,
		Failed: func(err error) { e.Say("store: %s", e.clean(err.Error())) }}
	server := &http.Server{Handler: served, ReadHeaderTimeout: 10 * time.Second}
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
