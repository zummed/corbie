package controller

import (
	"encoding/json/v2"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// What is past keeping is removed as part of the controller's passes, after
// the machines have been advanced, and how far it has got is never kept in
// memory. Each instance carries two wakes of the controller's own: one for
// when its older history is due to be trimmed, and one, set as it ends, for
// when it is due to be removed. So the cost of housekeeping follows what is
// due, not how much is kept; and a wake is deleted only when what it is for
// is done, so a pass that is stopped leaves the rest to the next.

const (
	wakeTrim   = "trim"
	wakeRemove = "remove"
)

// keepOf is how long an instance is kept: what its definition says, or a
// week. An instance whose definition cannot be used is kept a week.
func (p *pass) keepOf(st *instance.State) (time.Duration, error) {
	def, err := p.definition(st)
	if err != nil && !errorsIs(err, machine.ErrDefinition) {
		return 0, err
	}
	return p.keepFor(def, &st.Above), nil
}

// keepBatch is how many of the instances' wakes for housekeeping one pass
// acts on.
const keepBatch = 64

// housekeep removes what is past keeping: first what the instances' own
// wakes have brought up, one instance after another and soonest first, and
// then the controller's own hourly round. It is done a little at a time:
// with more wakes due than one pass acts on, the pass says there is more
// to do at once, and the machines come first again in the next. busy says
// the pass found something owed.
func (p *pass) housekeep(kept []*arrival, house []string, busy bool) error {
	for i, a := range kept {
		if i == keepBatch {
			p.mu.Lock()
			p.more = true
			p.mu.Unlock()
			break
		}
		if err := p.keep(a); err != nil {
			return err
		}
	}
	return p.hourly(house, busy || len(kept) > 0)
}

// keep acts on one of an instance's own wakes for housekeeping. It goes by
// the head, which says whether the instance has ended and how long it is
// kept, and which is the last thing of an instance to go.
func (p *pass) keep(a *arrival) error {
	head, _, err := instance.ReadHead(p.ctx, p.store, p.c.ID.Card, a.inst)
	if err != nil && !isAltered(err) {
		return err
	}
	switch {
	case head == nil:
		// The instance has been removed: nothing else will take its wake.
	case a.what == wakeRemove && !head.HasEnded():
		// The step that ended it has not been finished: the wake waits.
		return nil
	case a.what == wakeRemove:
		if err := p.remove(a.inst); err != nil {
			return err
		}
	case !head.HasEnded():
		latest, err := instance.Latest(p.ctx, p.store, p.c.ID.Card, p.c.ID, a.inst, head.Step)
		if err != nil {
			return err
		}
		if latest == nil {
			return nil // the head has moved on since it was read: the wake waits
		}
		keep := time.Duration(head.Keep)
		if err := p.trim(a.inst, latest, keep); err != nil {
			return err
		}
		// The next is a keep after this one, whenever this one is acted on:
		// acted on twice, it sets one wake.
		next := a.at.Add(keep)
		if _, err := p.store.Write(p.ctx, store.Wake(p.ctl, next, a.inst, wakeTrim), []byte("\n"), store.OnlyIfNew); err != nil {
			return err
		}
		p.due(next)
	}
	_, err = p.store.Delete(p.ctx, a.name, store.Overwrite)
	return err
}

// remove removes an instance that is past keeping: everything below it
// first, then its record, then its head, and the run is gone. What is below
// is read from the record, where each step that started a machine or
// cancelled one names it. A machine whose step has been trimmed away ended
// before this one did, and its own wake came due before this one's.
func (p *pass) remove(inst string) error {
	names, err := p.store.List(p.ctx, store.Record(p.ctl, inst))
	if err != nil {
		return err
	}
	var steps []string
	for _, name := range names {
		if strings.HasPrefix(name, store.Steps(p.ctl, inst)+"/") {
			steps = append(steps, name)
		}
	}
	started := make([][]string, len(steps))
	err = p.each(len(steps), func(i int) error {
		step, err := instance.ReadStep(p.ctx, p.store, p.c.ID.Card, p.c.ID, inst, store.Number(steps[i]))
		if err != nil && !isAltered(err) {
			return err
		}
		if step != nil {
			for _, ask := range step.Asks {
				if ask.Child != "" {
					started[i] = append(started[i], ask.Child)
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	below := slices.Compact(slices.Sorted(slices.Values(slices.Concat(started...))))
	err = p.each(len(below), func(i int) error { return p.remove(below[i]) })
	if err != nil {
		return err
	}
	err = p.each(len(names), func(i int) error {
		_, err := p.store.Delete(p.ctx, names[i], store.Overwrite)
		return err
	})
	if err != nil {
		return err
	}
	for _, head := range []string{store.RunningHead(p.ctl, inst), store.FinishedHead(p.ctl, inst)} {
		if _, err := p.store.Delete(p.ctx, head, store.Overwrite); err != nil {
			return err
		}
	}
	return nil
}

// written reads when a signed value was written from its frame, which is
// its first line, without opening it.
func (p *pass) written(name string) (time.Time, error) {
	part, err := p.store.ReadPart(p.ctx, name, 0, 512)
	if err != nil || part == nil {
		return time.Time{}, err
	}
	line, _, _ := strings.Cut(string(part), "\n")
	var frame struct {
		Sent time.Time `json:"sent"`
	}
	json.Unmarshal([]byte(line), &frame)
	return frame.Sent, nil
}

// trim removes the older history of an instance that is still running: its
// step and emit entries older than its keep, its latest step excepted, and
// the blobs that are older than the keep and that the current state of the
// instance, and of every instance running below it, no longer refers to.
// The definition, its scripts and the run's secrets always stay.
func (p *pass) trim(inst string, latest *instance.Step, keep time.Duration) error {
	before := p.now.Add(-keep)
	names, err := p.store.List(p.ctx, store.Record(p.ctl, inst))
	if err != nil {
		return err
	}
	refs, err := p.referred(inst, latest)
	if err != nil {
		return err
	}
	var gone []string
	steps, emits := store.Steps(p.ctl, inst)+"/", store.Emits(p.ctl, inst)+"/"
	// The steps, and the emit entries of each job, are each written in
	// order under a directory of their own.
	entries := map[string][]string{}
	for _, name := range names {
		switch {
		case strings.HasPrefix(name, steps) && store.Number(name) >= latest.Number:
			// The latest step stays, whatever its age.
		case strings.HasPrefix(name, steps), strings.HasPrefix(name, emits):
			directory := name[:strings.LastIndexByte(name, '/')]
			entries[directory] = append(entries[directory], name)
		case strings.HasPrefix(name, store.Blobs(p.ctl, inst)+"/"):
			made := blob.Made(name)
			if !refs[name] && !made.IsZero() && made.Before(before) {
				gone = append(gone, name)
			}
		}
	}
	for _, written := range entries {
		old, err := p.older(written, before)
		if err != nil {
			return err
		}
		gone = append(gone, written[:old]...)
	}
	return p.each(len(gone), func(i int) error {
		_, err := p.store.Delete(p.ctx, gone[i], store.Overwrite)
		return err
	})
}

// older gives how many of some entries were written before a time. They
// are written in order, so the first one young enough to keep is found by
// halving, and each of them goes or stays by its own age.
func (p *pass) older(names []string, before time.Time) (int, error) {
	lo, hi := 0, len(names) // the answer is in [lo, hi]
	for lo < hi {
		mid := (lo + hi) / 2
		at, err := p.written(names[mid])
		if err != nil {
			return 0, err
		}
		if at.IsZero() || at.Before(before) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

// referred gives the names of the blobs that the current state of an
// instance, and of every instance running below it to any depth, refers
// to, with what the stored definitions among them name.
func (p *pass) referred(inst string, latest *instance.Step) (map[string]bool, error) {
	refs := map[string]bool{}
	data, err := identity.Encode(latest.State)
	if err != nil {
		return nil, err
	}
	var state any
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	for _, ref := range blob.Refs(state) {
		refs[ref.At] = true
		named, err := blob.Named(p.ctx, p.store, ref)
		if err != nil {
			if p.blobError(err) != nil {
				return nil, err
			}
			continue
		}
		for _, inner := range named {
			refs[inner.At] = true
		}
	}
	for _, job := range latest.State.Jobs {
		if job.Child == "" {
			continue
		}
		below, err := instance.Current(p.ctx, p.store, p.c.ID.Card, p.c.ID, job.Child)
		if err != nil || below == nil {
			if err != nil && !isAltered(err) {
				return nil, err
			}
			continue
		}
		more, err := p.referred(job.Child, below)
		if err != nil {
			return nil, err
		}
		for name := range more {
			refs[name] = true
		}
	}
	return refs, nil
}

// hourly removes what no instance's wake brings to the controller's
// attention: receipts kept longer than a day, and uploads for runs that
// never started within a day. It has a wake of its own, once an hour. It is
// given the housekeeping wakes the pass read.
func (p *pass) hourly(wakes []string, busy bool) error {
	next := store.Wake(p.ctl, p.now.Truncate(time.Hour).Add(time.Hour), store.House(p.ctl), "keep")
	var due []string
	for _, name := range wakes {
		if name < next {
			due = append(due, name)
		}
	}
	if len(due) == 0 {
		// None is due. Where none was read at all, a pass that found
		// something owed sets the next: it is what starts housekeeping on a
		// new store. A pass that found nothing owed has only listed.
		if len(wakes) > 0 || !busy {
			return nil
		}
		_, err := p.store.Write(p.ctx, next, []byte("\n"), store.OnlyIfNew)
		p.due(p.now.Truncate(time.Hour).Add(time.Hour))
		return err
	}
	gone, more, err := p.pastReceipts()
	if err != nil {
		return err
	}
	// An upload for a run that has not started within a day: a record that
	// holds blobs and no step, under a name older than a day.
	records, err := p.store.List(p.ctx, store.Records(p.ctl))
	if err != nil {
		return err
	}
	started := map[string]bool{}
	uploads := map[string][]string{}
	for _, name := range records {
		inst := store.InstanceOf(p.ctl, name)
		if strings.Contains(name, "/steps/") {
			started[inst] = true
		} else {
			uploads[inst] = append(uploads[inst], name)
		}
	}
	for inst, names := range uploads {
		named, err := store.InstanceTime(inst)
		if started[inst] || err != nil || p.now.Sub(named) < 24*time.Hour+identity.ClockAllowance {
			continue
		}
		gone = append(gone, names...)
	}
	err = p.each(len(gone), func(i int) error {
		_, err := p.store.Delete(p.ctx, gone[i], store.Overwrite)
		return err
	})
	if err != nil || more {
		// With more that is due, the wake stays and the next pass goes on.
		return err
	}
	// The next wake is set before the one that was due is removed, so a
	// controller stopped between the two still has one.
	if _, err := p.store.Write(p.ctx, next, []byte("\n"), store.OnlyIfNew); err != nil {
		return err
	}
	p.due(p.now.Truncate(time.Hour).Add(time.Hour))
	for _, name := range due {
		if _, err := p.store.Delete(p.ctx, name, store.Overwrite); err != nil {
			return err
		}
	}
	return nil
}

// pastReceipts gives the receipts kept longer than a day. A receipt's name
// is its message's, which carries the time it was sent, so each client's
// list oldest first and a batch of them is read: the cost follows what is
// due, not how many are kept. more says a whole batch was due.
func (p *pass) pastReceipts() (gone []string, more bool, err error) {
	parties, err := p.parties()
	if err != nil {
		return nil, false, err
	}
	for _, party := range parties {
		if party.Card.Kind != identity.Client {
			continue
		}
		names, err := p.store.First(p.ctx, store.Receipts(p.ctl, party.Card.Name), wakeBatch)
		if err != nil {
			return nil, false, err
		}
		past := 0
		for _, name := range names {
			message := name[strings.LastIndexByte(name, '/')+1:]
			at, err := time.Parse("20060102T150405", message[:min(15, len(message))])
			if err != nil {
				// A client may name a message as it likes. Where the name
				// carries no time, the receipt says when it was written.
				if at, err = p.written(name); err != nil {
					return nil, false, err
				}
			}
			if !at.IsZero() && p.now.Sub(at) > instance.ReceiptLife+identity.ClockAllowance {
				gone = append(gone, name)
				past++
			}
		}
		more = more || past == wakeBatch
	}
	return gone, more, nil
}
