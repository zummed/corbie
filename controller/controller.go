// Package controller advances machines. It reads its inbox and its wakes in
// the store, takes one step at a time for each instance with something
// waiting, and writes down the result. Everything it does is a write to the
// store.
package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// A Controller brings nothing with it but its key. Who it trusts, its
// secrets and every instance are in the store.
type Controller struct {
	ID    *identity.Identity
	Store store.Store
	// Now is the controller's clock.
	Now func() time.Time
	// SideBySide bounds how many store requests a pass has open at once,
	// however much is owed.
	SideBySide int
	// AnyRunner has a job carried out whatever tags it names, as under
	// corbie test.
	AnyRunner bool
}

// New makes a controller on a store.
func New(id *identity.Identity, s store.Store) *Controller {
	return &Controller{ID: id, Store: s, Now: time.Now, SideBySide: store.SideBySide}
}

// wakeBatch is how many wakes a pass reads at once.
const wakeBatch = 1000

// MaxMessage is the largest a message may be.
const MaxMessage = 256 << 10

// pass is one pass over the store. Nothing outlives it.
type pass struct {
	c   *Controller
	ctx context.Context
	now time.Time
	ctl string
	// store is the controller's store, with the pass's bound on how many
	// requests are open at once.
	store store.Store

	mu     sync.Mutex
	trust  map[string]*identity.Trust // read once in the pass that asks
	all    []identity.Trust           // every party, read when a runner is to be picked
	allSet bool
	defs   map[string]*machine.Definition
	next   time.Time // when the next thing is due, if anything is
	more   bool      // more is due already than this pass has acted on
}

// An arrival is a message in the inbox or a wake that has come due.
type arrival struct {
	name string // its name in the store

	// For a message.
	msg    *instance.Message
	sender string
	kind   identity.Kind // what the sender is
	sent   time.Time
	until  time.Time
	value  identity.Value

	// For a wake.
	wake bool
	inst string
	at   time.Time
	what string
}

// good says whether a message is still good, asked only when it is about
// to be given to a step.
func (a *arrival) good(now time.Time) bool { return a.wake || a.value.Good(now) }

// Pass looks at the store, does everything that is owed, and stops. It
// says when the next thing is due, if it knows of one.
func (c *Controller) Pass(ctx context.Context) (next time.Time, err error) {
	p := &pass{c: c, ctx: ctx, now: c.Now().UTC(), ctl: c.ID.Name,
		store: store.Bounded(c.Store, c.SideBySide),
		trust: map[string]*identity.Trust{}, defs: map[string]*machine.Definition{}}

	// The inbox is read before any wake is acted on: a result that has
	// arrived wins over its own due time.
	names, err := p.store.List(ctx, store.Inbox(p.ctl))
	if err != nil {
		return next, err
	}
	// Wakes are named by their times and read soonest first, a batch at a
	// time: a pass costs what is due, however many wakes there are.
	wakes, err := p.store.First(ctx, store.Wakes(p.ctl), wakeBatch)
	if err != nil {
		return next, err
	}
	// A whole batch that is all due says nothing of what is behind it: a
	// machine's wake may be due there, behind housekeeping that waits its
	// turn. Then every wake is read.
	if len(wakes) == wakeBatch {
		if at, _, _, ok := store.ParseWake(p.ctl, wakes[wakeBatch-1]); !ok || !at.After(p.now) {
			if wakes, err = p.store.List(ctx, store.Wakes(p.ctl)); err != nil {
				return next, err
			}
		}
	}
	messages := make([]*arrival, len(names))
	err = p.each(len(names), func(i int) error {
		a, err := p.open(names[i])
		messages[i] = a
		return err
	})
	if err != nil {
		return next, err
	}
	waiting := map[string][]*arrival{}
	var order, house []string
	var kept []*arrival // the instances' own wakes for housekeeping, soonest first
	add := func(inst string, a *arrival) {
		if _, seen := waiting[inst]; !seen {
			order = append(order, inst)
		}
		waiting[inst] = append(waiting[inst], a)
	}
	for _, a := range messages {
		if a != nil {
			add(a.msg.Instance, a)
		}
	}
	for _, name := range wakes {
		at, inst, what, ok := store.ParseWake(p.ctl, name)
		if !ok {
			if _, err := p.store.Delete(ctx, name, store.Overwrite); err != nil {
				return next, err
			}
			continue
		}
		a := &arrival{name: name, inst: inst, wake: true, at: at, what: what}
		switch {
		case inst == store.House(p.ctl):
			house = append(house, name)
		case at.After(p.now):
			p.due(at)
		case what == wakeTrim || what == wakeRemove:
			kept = append(kept, a)
		default:
			add(inst, a)
		}
	}
	// Copies that begin at different moments begin at different places in
	// the list, so they seldom work on the same instance at once.
	if n := len(order); n > 1 {
		from := int(p.now.UnixNano() % int64(n))
		order = append(order[from:len(order):len(order)], order[:from]...)
	}
	err = p.each(len(order), func(i int) error {
		return p.advance(order[i], waiting[order[i]])
	})
	// What is past keeping goes after the machines have been advanced.
	if err == nil {
		err = p.housekeep(kept, house, len(order) > 0)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.more && err == nil {
		return p.now, nil
	}
	return p.next, err
}

// each does n things side by side, a bounded number at a time, and gives
// the first error. One of them may itself do things side by side: what
// bounds the pass is how many requests its store has open.
func (p *pass) each(n int, do func(i int) error) error {
	return store.Each(p.ctx, p.c.SideBySide, n, do)
}

// open reads one message from the inbox. It is checked in order: signed by
// a party the controller trusts, sealed to the controller; nothing it
// carries is read before both hold. One that is not accepted is deleted and
// gets no receipt. It gives nil for a message that is gone or was deleted.
func (p *pass) open(name string) (*arrival, error) {
	data, mark, err := p.store.Read(p.ctx, name)
	if err != nil || mark == store.None {
		return nil, err // listed and gone: another copy took it
	}
	a, err := p.accept(name, data)
	if err != nil {
		return nil, err
	}
	if a == nil {
		_, err := p.store.Delete(p.ctx, name, store.Overwrite)
		return nil, err
	}
	return a, nil
}

// accept says whether a message is one the controller accepts; nil if not.
// An error is the store's.
func (p *pass) accept(name string, data []byte) (*arrival, error) {
	parts := strings.Split(strings.TrimPrefix(name, store.Inbox(p.ctl)+"/"), "/")
	if len(parts) != 2 {
		return nil, nil
	}
	sender := parts[0]
	v, err := identity.Parse(data)
	if err != nil || v.By != sender {
		return nil, nil
	}
	card, kind := p.c.ID.Card, identity.Controller
	if sender != p.ctl {
		trust, err := p.trusted(sender)
		if err != nil {
			return nil, err
		}
		if !trust.Trusted() {
			return nil, nil
		}
		card, kind = trust.Card, trust.Card.Kind
	}
	if v.Check(card, name) != nil {
		return nil, nil
	}
	var msg instance.Message
	if v.Open(p.c.ID, &msg) != nil || store.CheckInstance(msg.Instance) != nil || !whole(&msg) {
		return nil, nil
	}
	sent := v.Sent
	if sent.After(p.now) {
		sent = p.now
	}
	return &arrival{name: name, msg: &msg, sender: sender, kind: kind, sent: sent, until: v.Until, value: v}, nil
}

// whole says whether a message carries the part its kind says it does.
// One that does not is not a message at all.
func whole(msg *instance.Message) bool {
	switch msg.Kind {
	case instance.KindStart:
		return msg.Start != nil
	case instance.KindEvent:
		return msg.Event != nil && msg.Event.Name != ""
	case instance.KindAnswer:
		return msg.Answer != nil
	case instance.KindResult:
		return msg.Result != nil
	case instance.KindEmit:
		return msg.Emit != nil
	case instance.KindCancel:
		return true
	}
	return false
}

// trusted reads what the controller holds about a party, from where trust
// is kept, once in this pass.
func (p *pass) trusted(party string) (*identity.Trust, error) {
	p.mu.Lock()
	t, ok := p.trust[party]
	p.mu.Unlock()
	if ok {
		return t, nil
	}
	t, err := identity.ReadTrust(p.ctx, p.store, p.c.ID.Card, party)
	if err != nil {
		if errors.Is(err, identity.ErrAltered) {
			return nil, nil // a trust value that does not check is no trust
		}
		return nil, err
	}
	p.mu.Lock()
	p.trust[party] = t
	p.mu.Unlock()
	return t, nil
}

// parties reads everything the controller holds about other parties, once
// in this pass.
func (p *pass) parties() ([]identity.Trust, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.allSet {
		all, err := identity.ListTrust(p.ctx, p.store, p.c.ID.Card)
		if err != nil {
			return nil, err
		}
		p.all, p.allSet = all, true
	}
	return p.all, nil
}

// runners reads every runner the controller trusts. If one cannot be read
// the pass ends: a job must not fail for want of a runner because of a
// moment's trouble.
func (p *pass) runners() ([]identity.Trust, error) {
	all, err := p.parties()
	if err != nil {
		return nil, err
	}
	var runners []identity.Trust
	for _, t := range all {
		if t.Trusted() && t.Card.Kind == identity.Runner {
			runners = append(runners, t)
		}
	}
	slices.SortFunc(runners, func(a, b identity.Trust) int { return strings.Compare(a.Card.Name, b.Card.Name) })
	return runners, nil
}

// definition reads an instance's stored definition, once in this pass.
func (p *pass) definition(st *instance.State) (*machine.Definition, error) {
	return p.definitionAt(st.Definition)
}

func (p *pass) definitionAt(ref blob.Ref) (*machine.Definition, error) {
	p.mu.Lock()
	def, ok := p.defs[ref.At]
	p.mu.Unlock()
	if ok {
		return def, nil
	}
	def, err := machine.Load(p.ctx, p.store, ref)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.defs[ref.At] = def
	p.mu.Unlock()
	return def, nil
}

// answered says whether a client's message has its receipt already.
func (p *pass) answered(a *arrival) (bool, error) {
	name := store.Receipt(p.ctl, a.sender, a.name[strings.LastIndexByte(a.name, '/')+1:])
	_, mark, err := p.store.Read(p.ctx, name)
	return mark != store.None, err
}

// write signs a value, seals it to the cards given, and writes it
// only-if-new under its name.
func (p *pass) write(name string, until time.Time, body any, to ...identity.Card) (bool, error) {
	data, err := p.c.ID.Write(name, p.now, until, body, to...)
	if err != nil {
		return false, err
	}
	return p.store.Write(p.ctx, name, data, store.OnlyIfNew)
}

func (p *pass) failf(format string, args ...any) *machine.Error {
	return &machine.Error{Status: machine.StatusError, Error: fmt.Sprintf(format, args...)}
}
