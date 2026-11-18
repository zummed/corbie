package controller

import (
	"fmt"
	"strings"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// A client's set, remove or list of its secrets or values is taken in this
// order, and every write in it can be made again:
//
//  1. The receipt is written, sealed to the client and to the controller,
//     naming by its hash the setting it answers.
//  2. What a taken receipt says is carried out, where the client may set
//     at that moment: the secret or the value is set, or removed.
//  3. The message is deleted.
//
// A message found under a name whose receipt answers another setting was
// answered already, and is deleted with nothing carried out: a client
// chooses its messages' names, and can send one again under a name whose
// first message is gone.

// setting takes a client's set, remove or list.
func (p *pass) setting(a *arrival) error {
	set := a.msg.Set
	at := store.Receipt(p.ctl, a.sender, a.name[strings.LastIndexByte(a.name, '/')+1:])
	receipt, err := p.ownReceipt(at)
	if err != nil {
		return err
	}
	if receipt == nil {
		decided, err := p.decide(a)
		if err != nil {
			return err
		}
		decided.Answers = set.Hash()
		to, err := p.trusted(a.sender)
		if err != nil || to == nil {
			return err
		}
		written, err := p.write(at, p.now.Add(instance.ReceiptLife), decided, to.Card, p.c.ID.Card)
		if err != nil {
			return err
		}
		receipt = decided
		if !written {
			// Another copy answered it first: its answer is carried out.
			if receipt, err = p.ownReceipt(at); err != nil || receipt == nil {
				return err
			}
		}
	}
	if receipt.Taken && receipt.Answers == set.Hash() && (set.Act == instance.ActSet || set.Act == instance.ActRemove) {
		// Nothing is set or removed without the check made now.
		refused, err := p.maySet(a.sender, set.At)
		if err != nil {
			return err
		}
		if refused == "" && set.Act == instance.ActSet {
			err = keep(p, set.Of, set.At, set.Name, set.Value)
		}
		if refused == "" && set.Act == instance.ActRemove {
			err = drop(p, set.Of, set.At, set.Name)
		}
		if err != nil {
			return err
		}
	}
	_, err = p.store.Delete(p.ctx, a.name, store.Overwrite)
	return err
}

// keep sets a secret or a value as the controller's owner does.
func keep(p *pass, of, at, name, value string) error {
	if of == instance.OfValue {
		return secret.SetValue(p.ctx, p.store, p.c.ID, at, name, value)
	}
	return secret.Set(p.ctx, p.store, p.c.ID, at, name, value)
}

// drop removes a secret or a value, and finds nothing to remove where it is
// gone already.
func drop(p *pass, of, at, name string) error {
	refused, err := secret.Refusal(p.ctx, p.store, p.c.ID, of, at, name, true)
	if err != nil || refused != "" {
		return err
	}
	if of == instance.OfValue {
		return secret.RemoveValue(p.ctx, p.store, p.c.ID, at, name)
	}
	return secret.Remove(p.ctx, p.store, p.c.ID, at, name)
}

// ownReceipt reads the receipt of a set, remove or list: nil where there is
// none.
func (p *pass) ownReceipt(at string) (*instance.Receipt, error) {
	data, mark, err := p.store.Read(p.ctx, at)
	if err != nil || mark == store.None {
		return nil, err
	}
	var receipt instance.Receipt
	if _, err := identity.Read(data, at, p.c.ID.Card, p.c.ID, &receipt); err != nil {
		if isAltered(err) {
			return &instance.Receipt{}, nil // one that does not open is no answer to carry out
		}
		return nil, err
	}
	return &receipt, nil
}

// decide works out the answer to a set, remove or list.
func (p *pass) decide(a *arrival) (*instance.Receipt, error) {
	set := a.msg.Set
	refuse := func(format string, args ...any) (*instance.Receipt, error) {
		return &instance.Receipt{Refused: fmt.Sprintf(format, args...)}, nil
	}
	if !a.good(p.now) {
		return refuse("the message came too late: it was good until %s", a.until.Format("2006-01-02T15:04:05Z07:00"))
	}
	switch {
	case set.Of != instance.OfSecret && set.Of != instance.OfValue:
		return refuse("%q is neither %s nor %s", set.Of, instance.OfSecret, instance.OfValue)
	case set.Act == instance.ActList:
		return p.listed(a.sender, set.Of)
	case set.Act != instance.ActSet && set.Act != instance.ActRemove:
		return refuse("%q is not %s, %s or %s", set.Act, instance.ActSet, instance.ActRemove, instance.ActList)
	}
	// Where the client may not set, it learns nothing of what is set there.
	if err := store.CheckLocation(set.At); err != nil {
		return refuse("%v", err)
	}
	refused, err := p.maySet(a.sender, set.At)
	if err != nil || refused != "" {
		return &instance.Receipt{Refused: refused}, err
	}
	refused, err = secret.Refusal(p.ctx, p.store, p.c.ID, set.Of, set.At, set.Name, set.Act == instance.ActRemove)
	if err != nil || refused != "" {
		return &instance.Receipt{Refused: refused}, err
	}
	return &instance.Receipt{Taken: true}, nil
}

// maySet says why a client may not set a secret or a value at a location,
// if it may not: a client sets them only at or under a prefix it could
// submit a tree at, by the check a submit is given.
func (p *pass) maySet(sender, location string) (string, error) {
	first := ""
	for _, at := range store.Above(location) {
		refused, err := p.refusePrefix(sender, at)
		if err != nil || refused == "" {
			return "", err
		}
		if first == "" {
			first = refused
		}
		// Where a tree is, its prefix is the one place at or around it a
		// client could submit at.
		_, mark, err := p.store.Read(p.ctx, store.Tree(p.ctl, at))
		if err != nil {
			return "", err
		}
		if mark != store.None {
			first = refused
			break
		}
	}
	return fmt.Sprintf("a client sets secrets and values only where it could submit a tree, and at %s: %s", location, first), nil
}

// listed gives the secrets or the values set at the locations a client may
// set them at; a secret's with no value.
func (p *pass) listed(sender, of string) (*instance.Receipt, error) {
	var entries []secret.Entry
	var err error
	if of == instance.OfValue {
		entries, err = secret.ListValues(p.ctx, p.store, p.c.ID)
	} else {
		entries, err = secret.List(p.ctx, p.store, p.ctl)
	}
	if err != nil {
		return nil, err
	}
	receipt := &instance.Receipt{Taken: true}
	for _, e := range entries {
		refused, err := p.maySet(sender, e.At)
		if err != nil {
			return nil, err
		}
		if refused == "" {
			receipt.Listed = append(receipt.Listed, instance.Listed{Name: e.Name, At: e.At, Value: e.Value})
		}
	}
	return receipt, nil
}
