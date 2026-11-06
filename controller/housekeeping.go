package controller

import (
	"encoding/json/v2"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
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
	names, err := store.Names(p.store.List(p.ctx, store.Record(p.ctl, inst)))
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
	sent, _, err := p.frame(name)
	return sent, err
}

// frame reads when a signed value was written, and until when it is good,
// from its first line, without opening it.
func (p *pass) frame(name string) (sent, until time.Time, err error) {
	part, err := p.store.ReadPart(p.ctx, name, 0, 512)
	if err != nil || part == nil {
		return time.Time{}, time.Time{}, err
	}
	line, _, _ := strings.Cut(string(part), "\n")
	var frame struct {
		Sent  time.Time `json:"sent"`
		Until time.Time `json:"until"`
	}
	json.Unmarshal([]byte(line), &frame)
	return frame.Sent, frame.Until, nil
}

// trim removes the older history of an instance that is still running: its
// step and emit entries older than its keep, its latest step excepted, and
// the blobs that are older than the keep and that the current state of the
// instance, and of every instance running below it, no longer refers to.
// The definition, its scripts and the run's secrets always stay.
func (p *pass) trim(inst string, latest *instance.Step, keep time.Duration) error {
	before := p.now.Add(-keep)
	names, err := store.Names(p.store.List(p.ctx, store.Record(p.ctl, inst)))
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
// attention: receipts kept longer than a day, with the results of calls
// beside them, and uploads for runs that never started and messages never
// taken within a day. It has a wake of its own, once an hour. It is
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
	beside, receipts, more, err := p.pastReceipts()
	if err != nil {
		return err
	}
	gone, err := p.pastUploads()
	if err != nil {
		return err
	}
	gone = append(gone, beside...)
	// An upload for a run that has not started within a day: a record that
	// holds blobs and no step, under a name older than a day.
	records, err := store.Names(p.store.List(p.ctx, store.Records(p.ctl)))
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
	for _, names := range [][]string{gone, receipts} {
		err = p.each(len(names), func(i int) error {
			_, err := p.store.Delete(p.ctx, names[i], store.Overwrite)
			return err
		})
		if err != nil {
			return err
		}
	}
	if more {
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

// pastReceipts gives the receipts kept longer than a day, each with
// everything in its directory: those last, the rest first, so a pass
// stopped between the two leaves the receipt for the next. A taken call's
// receipt has no end of its own: it stays while there is no result beside
// it, and the two go a day after the result was written. One round removes
// at most a batch of directories, and more says there were more past
// keeping.
func (p *pass) pastReceipts() (beside, receipts []string, more bool, err error) {
	parties, err := p.parties()
	if err != nil {
		return nil, nil, false, err
	}
	past := 0
	for _, party := range parties {
		if party.Card.Kind != identity.Client {
			continue
		}
		client := party.Card.Name
		names, err := store.Names(p.store.List(p.ctx, store.Receipts(p.ctl, client)))
		if err != nil {
			return nil, nil, false, err
		}
		var order []string
		held := map[string][]string{}
		for _, name := range names {
			message, ok := store.ReceiptMessage(p.ctl, name)
			if !ok {
				continue
			}
			if _, seen := held[message]; !seen {
				order = append(order, message)
			}
			held[message] = append(held[message], name)
		}
		for _, message := range order {
			gone, err := p.pastReceipt(client, message, held[message])
			if err != nil {
				return nil, nil, false, err
			}
			if !gone {
				continue
			}
			if past == wakeBatch {
				return beside, receipts, true, nil
			}
			past++
			for _, name := range held[message] {
				if name == store.Receipt(p.ctl, client, message) {
					receipts = append(receipts, name)
				} else {
					beside = append(beside, name)
				}
			}
		}
	}
	return beside, receipts, false, nil
}

// pastReceipt says whether the directory of one message's receipt is past
// keeping. The receipt and the result are each aged from when they were
// written, as their frames say. A message's name may carry the time it was
// sent, and the receipt of one sent within the day is not read: it is
// removed at most a day late.
func (p *pass) pastReceipt(client, message string, names []string) (bool, error) {
	if sent, err := time.Parse("20060102T150405", message[:min(15, len(message))]); err == nil && !p.past(sent) && !sent.After(p.now) {
		return false, nil
	}
	receipt, result := store.Receipt(p.ctl, client, message), store.Result(p.ctl, client, message)
	switch {
	case slices.Contains(names, result):
		written, _, err := p.frame(result)
		return !written.IsZero() && p.past(written), err
	case slices.Contains(names, receipt):
		written, until, err := p.frame(receipt)
		// A taken call's receipt waits for its result.
		return !written.IsZero() && !until.IsZero() && p.past(written), err
	}
	return true, nil // what is left of a directory whose receipt has gone
}

// past says whether something written at a time is past a day's keeping.
func (p *pass) past(at time.Time) bool {
	return p.now.Sub(at) > instance.ReceiptLife+identity.ClockAllowance
}

// pastUploads gives the uploads of messages the controller never took that
// are a day old: a message's uploads go as it is carried out. A message's
// name carries the time it was sent; where it does not, an upload's name
// says when it was made.
func (p *pass) pastUploads() ([]string, error) {
	names, err := store.Names(p.store.List(p.ctx, p.ctl+"/uploads"))
	if err != nil {
		return nil, err
	}
	var gone []string
	for _, name := range names {
		parts := strings.Split(strings.TrimPrefix(name, p.ctl+"/uploads/"), "/")
		if len(parts) < 3 {
			continue
		}
		message := parts[1]
		at, err := time.Parse("20060102T150405", message[:min(15, len(message))])
		if err != nil {
			at = blob.Made(name)
		}
		if !at.IsZero() && p.past(at) {
			gone = append(gone, name)
		}
	}
	return gone, nil
}
