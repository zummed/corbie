package serve

import (
	"context"
	"fmt"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/store"
)

// A storeError is a store that cannot be opened, or a config that knows of
// none. With nothing named, it leaves the directory alone.
type storeError struct{ error }

func (e storeError) Unwrap() error { return e.error }

// parties is what the UI acts as: a lister of runs, which reads every
// run's status, and the client and the controller
// where it was started as them.
type parties struct {
	lister     *client.Client
	client     *client.Client
	controller *identity.Identity
	names      []string
	address    string
	store      store.Store
}

// open opens the parties named after the command from a config, as
// Corbie's command line opens the same parties for its own commands.
func open(ctx context.Context, e *cli.Env, cfg identity.Config, asClient, asController bool) (*parties, error) {
	p := &parties{}
	var card identity.Card
	var recorded string
	var me *identity.Identity

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
	if asController || (held == nil && cfg.Has(identity.Controller)) {
		ctl, err := cfg.Load(ctx, identity.Controller)
		if err != nil {
			return nil, err
		}
		if recorded, err = cfg.ControllerStore(); err != nil {
			return nil, err
		}
		card = ctl.Card
		if asController {
			p.controller = ctl
		}
	}
	if asClient || (held != nil && !asController) {
		if held == nil {
			return nil, storeError{fmt.Errorf("this computer's client trusts no controller yet: corbie init sets one computer up, and corbie client trust takes a controller's card")}
		}
		if asController && (held.Card.Name != card.Name || held.Card.Fingerprint() != card.Fingerprint()) {
			return nil, fmt.Errorf("the config's client trusts the controller %s, not the config's own %s: name one party, client or controller", held.Card.Name, card.Name)
		}
		card, recorded = held.Card, held.Store
	}
	if card.Name == "" {
		return nil, storeError{fmt.Errorf("this config knows no controller: corbie client trust takes a controller's card, or corbie init sets one computer up")}
	}
	if asClient {
		if me, err = cfg.Load(ctx, identity.Client); err != nil {
			return nil, err
		}
	}

	s, err := e.Open(recorded, me, card)
	if err != nil {
		return nil, storeError{err}
	}

	if me != nil {
		p.client = client.New(me, card, s)
		p.names = append(p.names, me.Name)
	}
	if p.controller != nil {
		p.names = append(p.names, p.controller.Name)
	}
	p.address, p.store = e.Address(), s
	// Every run's status is listed as a viewer lists it: the store lets
	// every party that reads heads read them all.
	p.lister = client.New(nil, card, s)
	if _, err := p.lister.List(ctx, client.Filter{}); err != nil {
		return nil, storeError{err}
	}
	return p, nil
}
