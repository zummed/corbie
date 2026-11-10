package client

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// A Filter narrows a listing of runs, or widens it to take in those that
// have ended.
type Filter struct {
	// Labels keeps runs that carry every one of these: a label with a
	// value must match it, and one with none need only be there.
	Labels map[string]string
	// Status keeps runs with this status.
	Status string
	// Parent keeps the runs this instance started.
	Parent string
	// Location keeps the runs at a location.
	Location string
	// Key keeps the runs of a keyed location with this key.
	Key string
	// Finished takes in the runs that have ended and are still kept. A
	// Status of a run that has ended takes them in as well.
	Finished bool
}

func (f Filter) finished() bool {
	return f.Finished || (f.Status != "" && f.Status != instance.Running)
}

// Statuses are the words a listing can be narrowed to by status: what a
// run that is listed can be.
var Statuses = []string{instance.Running, instance.Done, instance.Failed, instance.Cancelled}

func (f Filter) keeps(r *Run) bool {
	if f.Status != "" && r.Status != f.Status {
		return false
	}
	if f.Parent != "" && r.Parent != f.Parent {
		return false
	}
	if f.Location != "" && r.Location != f.Location {
		return false
	}
	if f.Key != "" && r.Key != f.Key {
		return false
	}
	for name, want := range f.Labels {
		if got, ok := r.Labels[name]; !ok || (want != "" && got != want) {
			return false
		}
	}
	return true
}

// List gives the runs this party may read, each as its head gives it:
// every instance, not only roots. A client sees its own and those whose
// file's read: lists it; a viewer with access to heads sees everyone's.
func (c *Client) List(ctx context.Context, filter Filter) ([]*Run, error) {
	heads, _, _, err := c.heads(ctx, filter.finished(), nil)
	var runs []*Run
	for _, run := range heads {
		if filter.keeps(run) {
			runs = append(runs, run)
		}
	}
	return runs, err
}

// A headRead is a head as it was last read: the mark it was read at, and the
// run it gives, which is nil for a head that does not check.
type headRead struct {
	mark store.Mark
	run  *Run
}

// heads gives every head this party may read, in the order of their names,
// and each one's mark, by which a change is noticed. A head listed with the
// mark it was read at in last is not read again. What this look read, and
// what it took from last, is given by name for the next.
func (c *Client) heads(ctx context.Context, finished bool, last map[string]headRead) ([]*Run, map[string]store.Mark, map[string]headRead, error) {
	// The running heads are read before the finished ones: a head is
	// written among the finished before it leaves the running, so a run
	// that ends in between is in one or the other.
	places := []string{store.Running(c.ctl())}
	if finished {
		places = append(places, store.Finished(c.ctl()))
	}
	found := map[string]*Run{}
	marks := map[string]store.Mark{}
	read := map[string]headRead{}
	for _, place := range places {
		listed, err := c.listHeads(ctx, place)
		if err != nil {
			return nil, nil, nil, err
		}
		var unread []string
		for _, l := range listed {
			if was, known := last[l.Name]; known && was.mark == l.Mark {
				read[l.Name] = was
			} else {
				unread = append(unread, l.Name)
			}
		}
		now := make([]headRead, len(unread))
		err = store.Each(ctx, store.SideBySide, len(unread), func(i int) error {
			head, mark, err := c.readHead(ctx, unread[i])
			now[i].mark = mark
			if head != nil && c.lists(head) {
				now[i].run = runOf(head)
			}
			return err
		})
		if err != nil {
			return nil, nil, nil, err
		}
		for i, name := range unread {
			if now[i].mark != store.None {
				read[name] = now[i]
			}
		}
		for _, l := range listed {
			if was := read[l.Name]; was.run != nil {
				// One found in both places goes by the finished one.
				found[was.run.ID] = was.run
				marks[was.run.ID] = was.mark
			}
		}
	}
	runs := make([]*Run, 0, len(found))
	for _, name := range sortedNames(found) {
		runs = append(runs, found[name])
	}
	return runs, marks, read, nil
}

// lists says whether a listing gives a run: a viewer is given every run,
// and a client those it may read.
func (c *Client) lists(head *instance.Head) bool { return c.ID == nil || c.reads(head.Name, head) }

// listHeads lists the heads in a place.
func (c *Client) listHeads(ctx context.Context, place string) ([]store.Listed, error) {
	listed, err := c.Store.List(ctx, place)
	if errors.Is(err, store.ErrRefused) {
		return nil, fmt.Errorf("the store gave this party nothing to read")
	}
	return listed, err
}

// readHead reads the head at a name, and gives the mark it was read at. It
// gives no head for one that was listed and is gone, which has moved and
// has the mark None, and for one that does not check, which is not shown.
func (c *Client) readHead(ctx context.Context, name string) (*instance.Head, store.Mark, error) {
	data, mark, err := c.Store.Read(ctx, name)
	if err != nil || mark == store.None {
		return nil, store.None, err
	}
	var head instance.Head
	if _, err := identity.Read(data, name, c.Controller, nil, &head); err != nil {
		return nil, mark, nil
	}
	return &head, mark, nil
}

// An Update is one thing a watcher of a listing is told: a run that is new
// or has changed, or the mark of the end of the first listing.
type Update struct {
	Run *Run `json:"run,omitzero"`
	New bool `json:"new,omitzero"`
	// Listed marks the end of the first listing: everything that already
	// existed has been given.
	Listed bool `json:"listed,omitzero"`
}

// Watch follows a listing as it changes: the runs that already exist, then
// the mark, then each run that appears or changes, until the context ends.
// A run is given once more when it ends, saying how, also when the listing
// does not take in the finished. A run that the listing keeps, and that
// started and ended between two looks, is given once, as new and ended. It
// writes nothing and asks nothing of the controller. A look that cannot
// reach the store is made again at the next interval, and nothing is given
// on its account.
func (c *Client) Watch(ctx context.Context, filter Filter, interval time.Duration, tell func(Update)) error {
	notice, stop := c.Store.Notice(c.ctl())
	defer stop()
	w := &watched{seen: map[string]store.Mark{}}
	first := true
	for {
		err := c.changes(ctx, filter, w, tell)
		switch {
		case err == nil && first:
			tell(Update{Listed: true})
			first = false
		case err != nil && ctx.Err() == nil && !errors.Is(err, store.ErrUnreachable):
			return err
		}
		if !pause(ctx, interval, notice) {
			return nil
		}
	}
}

// watched is what a watcher of a listing has seen.
type watched struct {
	seen map[string]store.Mark // the runs given, by ID, and the mark each was given at
	read map[string]headRead   // the heads listed at the last look, by name, as each was read
	// ended holds the names of the finished heads at the last look, for a
	// listing that does not take in the finished. It is nil until a look
	// has been made.
	ended map[string]bool
}

// changes looks at the listing once and gives what is new or has changed
// since the marks last seen. For a listing that does not take in the
// finished it lists the finished heads by name and reads only those it has
// not listed before: a run that started and ended between two looks is
// among them.
func (c *Client) changes(ctx context.Context, filter Filter, w *watched, tell func(Update)) error {
	runs, marks, read, err := c.heads(ctx, filter.finished(), w.read)
	if err != nil {
		return err
	}
	var ended []string
	if !filter.finished() {
		if ended, err = store.Names(c.listHeads(ctx, store.Finished(c.ctl()))); err != nil {
			return err
		}
	}
	w.read = read
	for _, run := range runs {
		old, known := w.seen[run.ID]
		if !known && !filter.keeps(run) {
			continue
		}
		if !known || old != marks[run.ID] {
			tell(Update{Run: run, New: !known})
		}
		w.seen[run.ID] = marks[run.ID]
	}
	if w.ended != nil {
		var unread []string
		for _, name := range ended {
			// One given before is given as it leaves the listing, below.
			_, given := w.seen[strings.TrimPrefix(name, store.Finished(c.ctl())+"/")]
			if !given && !w.ended[name] {
				unread = append(unread, name)
			}
		}
		read := make([]*instance.Head, len(unread))
		err := store.Each(ctx, store.SideBySide, len(unread), func(i int) (err error) {
			read[i], _, err = c.readHead(ctx, unread[i])
			return err
		})
		if err != nil {
			return err
		}
		for i, head := range read {
			if head != nil && c.lists(head) && filter.keeps(runOf(head)) {
				tell(Update{Run: runOf(head), New: true})
			}
			w.ended[unread[i]] = true
		}
	}
	for _, id := range sortedNames(w.seen) {
		if _, listed := marks[id]; listed {
			continue
		}
		// It has left the listing: it has ended, or is past keeping and
		// gone.
		head, _, err := instance.ReadHead(ctx, c.Store, c.Controller, id)
		if err != nil {
			return err
		}
		if head != nil {
			tell(Update{Run: runOf(head)})
		}
		delete(w.seen, id)
		if w.ended != nil {
			w.ended[store.FinishedHead(c.ctl(), id)] = true
		}
	}
	if !filter.finished() {
		w.ended = map[string]bool{}
		for _, name := range ended {
			w.ended[name] = true
		}
	}
	return nil
}

// An Entry is one entry of a run's record as a follower is given it: a
// step, or a value a job emitted. Values are as they are shown: a blob is
// its type, its size and the path to ask for it by.
type Entry struct {
	ID   string    `json:"id"` // the instance whose record it is in
	Time time.Time `json:"time"`
	// For a step.
	Step   int                `json:"step,omitzero"`
	Events []instance.Arrived `json:"events,omitzero"`
	Asks   []instance.Ask     `json:"asks,omitzero"`
	Status string             `json:"status,omitzero"`
	Shown  machine.Shown      `json:"state,omitzero"` // the run's state after the step
	// For an emit.
	Job  string `json:"job,omitzero"` // the job that emitted
	Name string `json:"name,omitzero"`
	Data any    `json:"data,omitzero"`
}

// followed is one instance a follower is reading.
type followed struct {
	parent string
	next   int  // the first step not yet read
	listed bool // whether its steps have been listed: after that they are read by number
	read   bool // whether any step of it could be read
	ended  bool
	jobs   map[int]sent    // the scripts its steps sent, by their numbers
	emits  map[string]bool // the emit entries already given
}

// sent is a script a step sent: its full name and the runner it went to.
type sent struct {
	name   string
	runner identity.Card
}

// Follow gives each entry of a run's record as it is written, with those of
// every instance below it, and ends when the run and everything below it
// have ended, saying how the run ended. Each entry is read once. A look
// that cannot reach the store is made again at the next interval. A follow
// that is cut short gives the run as it stands.
func (c *Client) Follow(ctx context.Context, id string, interval time.Duration, tell func(Entry)) (*Run, error) {
	if err := c.identity(); err != nil {
		return nil, err
	}
	if err := store.CheckInstance(id); err != nil {
		return nil, err
	}
	if err := c.readable(ctx, id); err != nil {
		return nil, err
	}
	notice, stop := c.Store.Notice(store.Records(c.ctl()))
	defer stop()
	all := map[string]*followed{id: {jobs: map[int]sent{}, emits: map[string]bool{}}}
	ended := false
	for {
		run, err := c.look(ctx, id, all, tell)
		switch {
		case ctx.Err() != nil:
			return c.Show(context.WithoutCancel(ctx), id)
		case errors.Is(err, store.ErrUnreachable):
			run = nil
		case err != nil:
			return nil, err
		case run != nil && ended:
			return run, nil
		}
		// The record is read once more after the run is seen to have ended,
		// before the follow closes: an emit written just before the last
		// step can be listed just after it.
		if ended = run != nil; ended {
			continue
		}
		if !pause(ctx, interval, notice) {
			return c.Show(context.WithoutCancel(ctx), id)
		}
	}
}

// Record gives each entry of a run's record so far, with those of every
// instance below it, and then the run as it stands: what a follow gives,
// without staying.
func (c *Client) Record(ctx context.Context, id string, tell func(Entry)) (*Run, error) {
	if err := c.identity(); err != nil {
		return nil, err
	}
	if err := store.CheckInstance(id); err != nil {
		return nil, err
	}
	if err := c.readable(ctx, id); err != nil {
		return nil, err
	}
	all := map[string]*followed{id: {jobs: map[int]sent{}, emits: map[string]bool{}}}
	if run, err := c.look(ctx, id, all, tell); err != nil || run != nil {
		return run, err
	}
	return c.Show(ctx, id)
}

// look reads what is new in the record of a run and of every instance below
// it, and gives the run once it and everything below it have ended. What
// one look finds is given in the order it was written: by the time each
// entry carries, an instance's own entries staying in their own order where
// the times are the same.
func (c *Client) look(ctx context.Context, id string, all map[string]*followed, tell func(Entry)) (*Run, error) {
	var found []Entry
	err := c.readAll(ctx, all, func(entry Entry) { found = append(found, entry) })
	slices.SortStableFunc(found, func(a, b Entry) int { return a.Time.Compare(b.Time) })
	for _, entry := range found {
		tell(entry)
	}
	if err != nil {
		return nil, err
	}
	for name, f := range all {
		switch {
		case f.ended:
		case name == id && !f.read:
			// Not taken up yet, refused, or no run at all: showing it says.
		case name != id && !f.read && all[f.parent].ended:
			// A child with nothing that can be read, whose parent has
			// ended, is not waited on.
		default:
			return nil, nil
		}
	}
	run, err := c.Show(ctx, id)
	if err != nil || !run.HasEnded() {
		return nil, err
	}
	return run, nil
}

// readAll reads what is new in the record of every instance followed. A
// child is read in the look that finds it, and so is what it started.
func (c *Client) readAll(ctx context.Context, all map[string]*followed, tell func(Entry)) error {
	looked := map[string]bool{}
	for more := true; more; {
		more = false
		for _, name := range sortedNames(all) {
			if looked[name] {
				continue
			}
			looked[name], more = true, true
			if err := c.read(ctx, name, all, tell); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedNames[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// read gives the entries of one instance's record that have not been
// given: its new steps, then the new emits of the jobs its steps sent. A
// child one of its steps started is followed from then on. A look that
// fails lists the steps again at the next.
func (c *Client) read(ctx context.Context, name string, all map[string]*followed, tell func(Entry)) error {
	f := all[name]
	readable, err := c.steps(ctx, name, all, tell)
	if err != nil {
		f.listed = false
		return err
	}
	if !readable {
		return nil // a record this client cannot read
	}
	if len(f.jobs) == 0 {
		return nil
	}
	names, err := store.Names(c.Store.List(ctx, store.Emits(c.ctl(), name)))
	if err != nil {
		f.listed = false
		return err
	}
	var unread []string
	var jobs []int
	for _, at := range names {
		if f.emits[at] {
			continue
		}
		job, _, ok := store.EmitOf(c.ctl(), name, at)
		if _, known := f.jobs[job]; !ok || !known {
			continue // from a runner that had no such job, or its step is not read yet
		}
		unread, jobs = append(unread, at), append(jobs, job)
	}
	emits := make([]*instance.Emit, len(unread))
	err = store.Each(ctx, store.SideBySide, len(unread), func(i int) (err error) {
		emits[i], err = c.emit(ctx, name, f.jobs[jobs[i]], jobs[i], store.Number(unread[i]))
		return err
	})
	if err != nil {
		f.listed = false
		return err
	}
	for i, emit := range emits {
		f.emits[unread[i]] = true
		if emit == nil {
			continue
		}
		if emit, err = shown(emitPlace(emit.Job, emit.Number), emit); err != nil {
			return err
		}
		for _, value := range emit.Values {
			tell(Entry{ID: name, Time: value.At, Job: emit.Job, Name: value.Name, Data: value.Data})
		}
	}
	return nil
}

// steps gives the step entries of one instance's record that have not been
// given. The first look lists them: the older steps of a run that has gone
// on a long time are trimmed, so reading starts from the first that is
// there, and one that is listed and gone when it is read is passed over.
// A later look reads on by number from the first not given, until one is
// not there, and lists again where the head is past that one. It says
// false for a record this client cannot read.
func (c *Client) steps(ctx context.Context, name string, all map[string]*followed, tell func(Entry)) (bool, error) {
	f := all[name]
	if f.ended {
		return true, nil
	}
	for f.listed && !f.ended {
		step, err := instance.ReadStep(ctx, c.Store, c.Controller, c.ID, name, f.next)
		if err != nil {
			return !errors.Is(err, identity.ErrNotForUs), ignore(err, identity.ErrNotForUs)
		}
		if step == nil {
			// Not written yet, or trimmed before it was read by a follower
			// held up longer than the keep: the head says which.
			head, _, err := instance.ReadHead(ctx, c.Store, c.Controller, name)
			if err != nil || head == nil || head.Step < f.next {
				return true, err
			}
			f.listed = false
			break
		}
		if err := c.give(name, step, all, tell); err != nil {
			return true, err
		}
		f.next++
	}
	if f.listed || f.ended {
		return true, nil
	}
	listed, err := store.Names(c.Store.List(ctx, store.Steps(c.ctl(), name)))
	if err != nil {
		return true, err
	}
	f.listed = true
	var numbers []int
	for _, at := range listed {
		if number := store.Number(at); number >= f.next {
			numbers = append(numbers, number)
		}
	}
	read := make([]*instance.Step, len(numbers))
	failed := make([]error, len(numbers))
	// A step this client cannot read ends what is given, so each step's
	// error is kept for its turn.
	err = store.Each(ctx, store.SideBySide, len(numbers), func(i int) error {
		read[i], failed[i] = instance.ReadStep(ctx, c.Store, c.Controller, c.ID, name, numbers[i])
		return nil
	})
	if err != nil {
		return true, err
	}
	for i, step := range read {
		if failed[i] != nil {
			// The next look reads it again by number, and is given the
			// same answer.
			f.next = numbers[i]
			return !errors.Is(failed[i], identity.ErrNotForUs), ignore(failed[i], identity.ErrNotForUs)
		}
		if step != nil {
			if err := c.give(name, step, all, tell); err != nil {
				return true, err
			}
		}
		f.next = numbers[i] + 1
	}
	f.next = max(f.next, 1)
	return true, nil
}

// ignore gives err, or nil where it is the error named.
func ignore(err, named error) error {
	if errors.Is(err, named) {
		return nil
	}
	return err
}

// give gives one step entry of an instance's record, and follows from then
// on the jobs it sent and the children it started.
func (c *Client) give(name string, step *instance.Step, all map[string]*followed, tell func(Entry)) error {
	f := all[name]
	for _, ask := range step.Asks {
		switch ask.Kind {
		case instance.AskScript:
			f.jobs[ask.Number] = sent{name: ask.Name, runner: ask.Card}
		case instance.AskChild:
			// A child is found from any step that started one.
			if _, known := all[ask.Child]; !known {
				all[ask.Child] = &followed{parent: name, jobs: map[int]sent{}, emits: map[string]bool{}}
			}
		}
	}
	entry, err := stepEntry(name, step)
	if err != nil {
		return err
	}
	f.read, f.ended = true, step.State.Status != instance.Running
	tell(entry)
	return nil
}

// stepPlace and emitPlace are where an entry is in a run's record, as the
// path of a blob shown in it begins.
func stepPlace(number int) string { return fmt.Sprintf("steps/%d", number) }

func emitPlace(job string, number int) string { return fmt.Sprintf("emits/%s/%d", job, number) }

// plain gives a value as the data it is when written as JSON: maps, lists,
// text and numbers.
func plain(v any) (data any, err error) {
	text, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, json.Unmarshal(text, &data)
}

// shown gives a copy of an entry of a record in which every blob is as it
// is shown: its type, its size and the path to ask for it by, which is the
// entry's place in the record and then the blob's place in the entry. No
// key is left anywhere in it, so nothing made from it can hold one.
func shown[T any](place string, entry *T) (*T, error) {
	data, err := plain(entry)
	if err != nil {
		return nil, err
	}
	data, _ = blob.Shown(data, func(at string, ref blob.Ref) (any, error) {
		return map[string]any{"$blob": place + "/" + at, "type": ref.Type, "size": float64(ref.Size)}, nil
	})
	text, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	out := new(T)
	return out, json.Unmarshal(text, out)
}

// stepEntry gives a step entry of a record as it is shown, without the
// cards of the runners its jobs went to.
func stepEntry(name string, step *instance.Step) (Entry, error) {
	step, err := shown(stepPlace(step.Number), step)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{ID: name, Time: step.Time, Step: step.Number, Events: step.Events, Status: step.State.Status}
	for _, ask := range step.Asks {
		ask.Card = identity.Card{}
		if ask.Job != nil {
			job := *ask.Job
			job.Found = nil
			ask.Job = &job
		}
		entry.Asks = append(entry.Asks, ask)
	}
	if t, ok := machine.TypeOf(step.State.Type); ok {
		entry.Shown = t.Show(step.State.Inner)
	}
	return entry, nil
}

// emit reads one emit entry of a script a step sent, filed under the
// script's number. It gives nil for one that is not there, that was not
// signed by the runner the script went to, or that names another job: a
// runner can add to any record, and what it adds for a job it never had is
// ignored.
func (c *Client) emit(ctx context.Context, name string, job sent, jobNumber, number int) (*instance.Emit, error) {
	at := store.Emit(c.ctl(), name, jobNumber, number)
	data, mark, err := c.Store.Read(ctx, at)
	if err != nil || mark == store.None {
		return nil, err
	}
	var emit instance.Emit
	if _, err := identity.Read(data, at, job.runner, c.ID, &emit); err != nil || emit.Job != job.name || emit.Number != number {
		return nil, nil
	}
	return &emit, nil
}

// sentAs gives the script a step sent under a full name, and its number,
// from the step that asked for it, looking from the latest step back.
func (c *Client) sentAs(ctx context.Context, name, job string, latest int) (sent, int, error) {
	for number := latest; number > 0; number-- {
		step, err := instance.ReadStep(ctx, c.Store, c.Controller, c.ID, name, number)
		if err != nil || step == nil {
			return sent{}, 0, err // below here the steps are trimmed
		}
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskScript && ask.Name == job {
				return sent{name: ask.Name, runner: ask.Card}, ask.Number, nil
			}
		}
	}
	return sent{}, 0, nil
}

// A File is a blob of a run as it is shown: what it is and how large, and
// no key. Name is what the value that holds it calls it.
type File struct {
	Name string `json:"name,omitzero"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// ErrNoFile is a path that names no blob of the run.
var ErrNoFile = errors.New("the run shows no file at the path")

// Fetch gives the contents of a blob of a run, by the path it was shown
// with.
//
// A path shown in a run's result says where the blob is in the output:
// report, pages[2]. A path shown anywhere else begins with the place of
// what holds the blob (steps/<number> or emits/<job>/<number>, which is an
// entry of the run's record, input or definition) and after the next slash
// says where the blob is in that.
func (c *Client) Fetch(ctx context.Context, id, path string) (io.Reader, File, error) {
	ref, err := c.find(ctx, id, path)
	if err != nil {
		return nil, File{}, err
	}
	reader, err := blob.Open(ctx, c.Store, ref)
	return reader, File{Type: ref.Type, Size: ref.Size}, err
}

// find gives the reference of the blob a path names.
func (c *Client) find(ctx context.Context, id, path string) (blob.Ref, error) {
	latest, err := c.current(ctx, id)
	if errors.Is(err, ErrCalled) {
		// A call's result is beside its receipt, and only its output can be
		// read.
		output, called, callErr := c.calledOutput(ctx, id)
		switch {
		case callErr != nil:
			return blob.Ref{}, callErr
		case !called:
			return blob.Ref{}, fmt.Errorf("%w: it is running", ErrNoRecord)
		}
		data, err := plain(output)
		if err != nil {
			return blob.Ref{}, err
		}
		if ref, ok := blob.Find(data, path); ok {
			return ref, nil
		}
		return blob.Ref{}, fmt.Errorf("%w %q", ErrNoFile, path)
	}
	if err != nil {
		return blob.Ref{}, err
	}
	holder, in, beside := any(latest.State.Output), path, false
	parts := strings.SplitN(path, "/", 4)
	switch {
	case parts[0] == "steps" && len(parts) >= 3:
		number, _ := strconv.Atoi(parts[1])
		holder, err = instance.ReadStep(ctx, c.Store, c.Controller, c.ID, id, number)
		in = strings.Join(parts[2:], "/")
	case parts[0] == "emits" && len(parts) == 4 && store.CheckPart(parts[1]) == nil:
		number, _ := strconv.Atoi(parts[2])
		var job sent
		var jobNumber int
		holder = nil
		if job, jobNumber, err = c.sentAs(ctx, id, parts[1], latest.Number); err == nil && jobNumber > 0 {
			holder, err = c.emit(ctx, id, job, jobNumber, number)
		}
		in = parts[3]
	case parts[0] == "input" && len(parts) >= 2:
		holder, in = latest.State.Input, strings.Join(parts[1:], "/")
	case parts[0] == "definition" && len(parts) >= 2:
		// A definition names its scripts and machines by their own names
		// in the record it is in.
		holder, err = machine.Load(ctx, c.Store, latest.State.Definition)
		in, beside = strings.Join(parts[1:], "/"), true
	}
	if err != nil {
		return blob.Ref{}, err
	}
	data, err := plain(holder)
	if err != nil {
		return blob.Ref{}, err
	}
	ref, ok := blob.Find(data, in)
	if !ok {
		return blob.Ref{}, fmt.Errorf("%w %q", ErrNoFile, path)
	}
	if beside {
		ref = latest.State.Definition.Beside(ref)
	}
	return ref, nil
}

// Files lists the blobs in a run's result by their paths, each with the
// name it has in the value that holds it. It lists every blob a path of the
// result fetches: one written out inside a text is among them.
func (c *Client) Files(ctx context.Context, id string) (map[string]File, error) {
	var given any
	latest, err := c.current(ctx, id)
	switch {
	case errors.Is(err, ErrCalled):
		output, called, callErr := c.calledOutput(ctx, id)
		if callErr != nil {
			return nil, callErr
		}
		if !called {
			return nil, fmt.Errorf("%w: it is running", ErrNoRecord)
		}
		given = output
	case err != nil:
		return nil, err
	default:
		given = latest.State.Output
	}
	output, err := plain(given)
	if err != nil {
		return nil, err
	}
	files := map[string]File{}
	named(output, "", "", files)
	return files, nil
}

// calledOutput gives the output of a call into another client's tree, from
// its result. called says the run is such a call and its result is there.
func (c *Client) calledOutput(ctx context.Context, id string) (output any, called bool, err error) {
	run, err := c.Show(ctx, id)
	if err != nil || run.Called == "" || !run.HasEnded() {
		return nil, false, err
	}
	result, err := c.result(ctx, id)
	if err != nil || result == nil {
		return nil, false, err
	}
	return result.Output, true, nil
}

// named lists the blobs of a value, each under the path it is shown with.
// A blob's name is its key in the map that holds it; for an item of a list
// it is the list's name and the item's place in it, and for a blob written
// out inside a text the text's name and which blob of the text it is.
func named(v any, at, name string, files map[string]File) {
	// place is the name of one of several blobs that share a name.
	place := func(which string) string {
		if name == "" {
			return which
		}
		return name + "-" + which
	}
	if ref, ok := blob.AsRef(v); ok {
		files[at] = File{Name: name, Type: ref.Type, Size: ref.Size}
		return
	}
	switch v := v.(type) {
	case map[string]any:
		for key, inner := range v {
			sub := key
			if at != "" {
				sub = at + "." + key
			}
			named(inner, sub, key, files)
		}
	case []any:
		for i, inner := range v {
			named(inner, at+"["+strconv.Itoa(i)+"]", place(strconv.Itoa(i)), files)
		}
	case string:
		// The blobs of a text are counted where a text is shown.
		blob.Shown(v, func(which string, ref blob.Ref) (any, error) {
			files[at+which] = File{Name: place(strings.TrimPrefix(which, "#")), Type: ref.Type, Size: ref.Size}
			return nil, nil
		})
	}
}

// Definition gives the definition a run was started from, in its full
// form, as it is shown: each script and each machine it names as a blob's
// type, its size and the path to ask for it by.
func (c *Client) Definition(ctx context.Context, id string) (*machine.Definition, error) {
	latest, err := c.current(ctx, id)
	if err != nil {
		return nil, err
	}
	def, err := machine.Load(ctx, c.Store, latest.State.Definition)
	if err != nil {
		return nil, err
	}
	for table, refs := range map[string]map[string]blob.Ref{"scripts": def.Scripts, "machines": def.Machines} {
		for name, ref := range refs {
			refs[name] = blob.Ref{At: "definition/" + table + "." + name, Type: ref.Type, Size: ref.Size}
		}
	}
	return def, nil
}

// Step gives one step entry of a run's record, as it is shown.
func (c *Client) Step(ctx context.Context, id string, number int) (*Entry, error) {
	if _, err := c.current(ctx, id); err != nil {
		return nil, err
	}
	step, err := instance.ReadStep(ctx, c.Store, c.Controller, c.ID, id, number)
	if err != nil {
		return nil, err
	}
	if step == nil {
		return nil, fmt.Errorf("the run has no step %d", number)
	}
	entry, err := stepEntry(id, step)
	return &entry, err
}
