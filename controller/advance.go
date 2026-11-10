package controller

import (
	"errors"
	"slices"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

// advance does everything owed to one instance. For as long as something
// is waiting for it:
//
//  1. find the latest step, reading on from the number the head gives;
//  2. finish that step first, if what it used is still waiting;
//  3. give what is left to the next step and write it only-if-new;
//  4. carry out what the entry asks for, bring the head up to date, and
//     delete what the entry used.
//
// A copy that finds the step's number taken throws its own result away and
// starts again from 1, so it acts only on the entry that was written.
func (p *pass) advance(inst string, waiting []*arrival) error {
	for len(waiting) > 0 {
		head, _, err := instance.ReadHead(p.ctx, p.store, p.c.ID.Card, inst)
		if err != nil && !isAltered(err) {
			return err
		}
		from := 1
		if head != nil {
			from = head.Step
		}
		latest, err := instance.Latest(p.ctx, p.store, p.c.ID.Card, p.c.ID, inst, from)
		if err != nil {
			return err
		}
		if head != nil && latest == nil {
			// A head whose step is not there: another copy moved the head
			// on and trimmed, or is removing the instance. It is not a run
			// that does not exist: what is waiting waits for the next pass.
			return nil
		}
		if latest != nil && head != nil && head.Step >= latest.Number && uses(latest, waiting) {
			// The step is written and the head has it. If what it used is
			// gone as well, the copy that wrote it has carried it all out,
			// and this one, which read the inbox a moment too early, has
			// nothing to finish.
			gone, err := p.gone(latest, waiting)
			if err != nil {
				return err
			}
			if gone {
				waiting = slices.DeleteFunc(waiting, func(a *arrival) bool { return slices.Contains(latest.Used, a.name) })
				continue
			}
		}
		if latest != nil && (head == nil || head.Step < latest.Number || uses(latest, waiting)) {
			if err := p.finish(inst, latest); err != nil {
				return err
			}
			waiting = slices.DeleteFunc(waiting, func(a *arrival) bool { return slices.Contains(latest.Used, a.name) })
			continue
		}
		taken, err := p.take(inst, latest, waiting)
		if errors.Is(err, errLeft) {
			return nil // what is waiting waits for a later pass
		}
		if err != nil {
			return err
		}
		// What was refused or dropped is answered and deleted, the receipt
		// before the message.
		moved := map[sendingTo]bool{}
		for _, d := range taken.dropped {
			if d.move {
				// One at a time: a later event goes where the first has gone.
				to := sendingTo{senderOf(d.a.msg), d.a.msg.To.At, keyOf(d.a.msg.To)}
				if moved[to] {
					p.later()
				} else if err := p.redirect(d.a); err != nil {
					return err
				}
				moved[to] = true
				continue
			}
			if d.note != "" {
				if err := p.undeliver(d.a, d.note); err != nil {
					return err
				}
				continue
			}
			if d.failed != "" {
				if err := p.callFailed(d.a, d.failed); err != nil {
					return err
				}
				continue
			}
			if d.receipt != nil {
				if err := p.receipt(d.a, d.receipt); err != nil {
					return err
				}
			}
			if _, err := p.store.Delete(p.ctx, d.a.name, store.Overwrite); err != nil {
				return err
			}
		}
		waiting = taken.rest
		if taken.step == nil {
			continue
		}
		readers, err := p.readers(inst, &taken.step.State)
		if err != nil {
			return err
		}
		written, err := p.write(store.Step(p.ctl, inst, taken.step.Number), noEnd, taken.step, append(readers, p.c.ID.Card)...)
		if err != nil {
			return err
		}
		if !written {
			// Another copy got there first. What this copy worked out is
			// thrown away; everything it was given is still waiting.
			waiting = append(taken.used, waiting...)
			continue
		}
		if err := p.finish(inst, taken.step); err != nil {
			return err
		}
	}
	return nil
}

// gone says whether everything a step used, of what this pass found
// waiting, has been deleted since. What a step used is deleted last, after
// the step has been carried out.
func (p *pass) gone(step *instance.Step, waiting []*arrival) (bool, error) {
	for _, a := range waiting {
		if !slices.Contains(step.Used, a.name) {
			continue
		}
		if _, mk, err := p.store.Read(p.ctx, a.name); err != nil || mk != store.None {
			return false, err
		}
	}
	return true, nil
}

// uses says whether any of what a step used is still waiting.
func uses(step *instance.Step, waiting []*arrival) bool {
	for _, a := range waiting {
		if slices.Contains(step.Used, a.name) {
			return true
		}
	}
	return false
}

func isAltered(err error) bool {
	return errors.Is(err, identity.ErrAltered)
}

// readers are the clients everything in an instance's record is sealed to,
// beside the controller: the client of the root's tree, and each client its
// root's file lists in read: that the controller trusts.
func (p *pass) readers(inst string, st *instance.State) ([]identity.Card, error) {
	var cards []identity.Card
	owner := store.InstanceClient(inst)
	t, err := p.trusted(owner)
	if err != nil {
		return nil, err
	}
	if t != nil {
		cards = append(cards, t.Card)
	}
	for _, name := range st.Above.Readers {
		reader, err := p.trusted(name)
		if err != nil {
			return nil, err
		}
		if name != owner && reader.Trusted() && reader.Card.Kind == identity.Client {
			cards = append(cards, reader.Card)
		}
	}
	return cards, nil
}
