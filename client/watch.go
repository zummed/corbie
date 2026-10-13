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
	// Finished takes in the runs that have ended and are still kept.
	Finished bool
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
	for name, want := range f.Labels {
		if got, ok := r.Labels[name]; !ok || (want != "" && got != want) {
			return false
		}
	}
	return true
}

// List gives the runs this party may read, each as its head gives it:
// every instance, not only roots. A client sees its own; a viewer with
// access to heads sees everyone's.
func (c *Client) List(ctx context.Context, filter Filter) ([]*Run, error) {
	heads, _, err := c.heads(ctx, filter.Finished)
	var runs []*Run
	for _, run := range heads {
		if filter.keeps(run) {
			runs = append(runs, run)
		}
	}
	return runs, err
}

// heads gives every head this party may read, in the order of their names,
// and each one's mark, by which a change is noticed.
func (c *Client) heads(ctx context.Context, finished bool) ([]*Run, map[string]store.Mark, error) {
	under := func(base string) string {
		if c.ID != nil {
			return base + "/" + c.ID.Name
		}
		return base
	}
	// The running heads are read before the finished ones: a head is
	// written among the finished before it leaves the running, so a run
	// that ends in between is in one or the other.
	places := []string{under(store.Running(c.ctl()))}
	if finished {
		places = append(places, under(store.Finished(c.ctl())))
	}
	found := map[string]*Run{}
	marks := map[string]store.Mark{}
	for _, place := range places {
		names, err := c.Store.List(ctx, place)
		if err != nil {
			if errors.Is(err, store.ErrRefused) {
				return nil, nil, fmt.Errorf("the store gave this party nothing to read")
			}
			return nil, nil, err
		}
		for _, name := range names {
			data, mark, err := c.Store.Read(ctx, name)
			if err != nil {
				return nil, nil, err
			}
			if mark == store.None {
				continue // listed and gone: it has moved
			}
			var head instance.Head
			if _, err := identity.Read(data, name, c.Controller, nil, &head); err != nil {
				continue // a head that does not check is not shown
			}
			// One found in both places goes by the finished one.
			found[head.Name] = runOf(&head)
			marks[head.Name] = mark
		}
	}
	runs := make([]*Run, 0, len(found))
	for _, name := range sortedNames(found) {
		runs = append(runs, found[name])
	}
	return runs, marks, nil
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
// does not take in the finished. It writes nothing and asks nothing of the
// controller. A look that cannot reach the store is made again at the next
// interval, and nothing is given on its account.
func (c *Client) Watch(ctx context.Context, filter Filter, interval time.Duration, tell func(Update)) error {
	notice, stop := c.Store.Notice(c.ctl())
	defer stop()
	seen := map[string]store.Mark{}
	first := true
	for {
		err := c.changes(ctx, filter, seen, tell)
		switch {
		case err == nil && first:
			tell(Update{Listed: true})
			first = false
		case err != nil && first && ctx.Err() == nil && !errors.Is(err, store.ErrUnreachable):
			return err
		}
		if !pause(ctx, interval, notice) {
			return nil
		}
	}
}

// changes looks at the listing once and gives what is new or has changed
// since the marks last seen.
func (c *Client) changes(ctx context.Context, filter Filter, seen map[string]store.Mark, tell func(Update)) error {
	runs, marks, err := c.heads(ctx, filter.Finished)
	if err != nil {
		return err
	}
	for _, run := range runs {
		old, known := seen[run.ID]
		if !known && !filter.keeps(run) {
			continue
		}
		if !known || old != marks[run.ID] {
			tell(Update{Run: run, New: !known})
		}
		seen[run.ID] = marks[run.ID]
	}
	for _, id := range sortedNames(seen) {
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
		delete(seen, id)
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
	Shown  any                `json:"state,omitzero"` // what the machine's type shows of its state
	// For an emit.
	Job  string `json:"job,omitzero"` // the job that emitted
	Name string `json:"name,omitzero"`
	Data any    `json:"data,omitzero"`
}

// followed is one instance a follower is reading.
type followed struct {
	parent string
	next   int  // the first step not yet read
	read   bool // whether any step of it could be read
	ended  bool
	jobs   map[string]identity.Card // the runner each job went to, by the job's full name
	emits  map[string]bool          // the emit entries already given
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
	notice, stop := c.Store.Notice(store.Records(c.ctl()))
	defer stop()
	all := map[string]*followed{id: {jobs: map[string]identity.Card{}, emits: map[string]bool{}}}
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
	all := map[string]*followed{id: {jobs: map[string]identity.Card{}, emits: map[string]bool{}}}
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
// child one of its steps started is followed from then on.
func (c *Client) read(ctx context.Context, name string, all map[string]*followed, tell func(Entry)) error {
	f := all[name]
	var steps []string
	if !f.ended {
		// The steps are read from their listing. The older steps of a run
		// that has gone on a long time are trimmed, so reading starts from
		// the first that is there, and one that is listed and gone when it
		// is read is passed over.
		var err error
		if steps, err = c.Store.List(ctx, store.Steps(c.ctl(), name)); err != nil {
			return err
		}
	}
	for _, at := range steps {
		number := store.Number(at)
		if number < f.next {
			continue
		}
		step, err := instance.ReadStep(ctx, c.Store, c.Controller, c.ID, name, number)
		if errors.Is(err, identity.ErrNotForUs) {
			return nil // a record this client cannot read
		}
		if err != nil {
			return err
		}
		if step == nil {
			continue
		}
		for _, ask := range step.Asks {
			switch ask.Kind {
			case instance.AskScript:
				f.jobs[ask.Name] = ask.Card
			case instance.AskChild:
				// A child is found from any step that started one.
				if _, known := all[ask.Child]; !known {
					all[ask.Child] = &followed{parent: name, jobs: map[string]identity.Card{}, emits: map[string]bool{}}
				}
			}
		}
		entry, err := stepEntry(name, step)
		if err != nil {
			return err
		}
		f.next, f.read, f.ended = number+1, true, step.State.Status != instance.Running
		tell(entry)
	}
	if len(f.jobs) == 0 {
		return nil
	}
	names, err := c.Store.List(ctx, store.Emits(c.ctl(), name))
	if err != nil {
		return err
	}
	for _, at := range names {
		if f.emits[at] {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(at, store.Emits(c.ctl(), name)+"/"), "/")
		runner, known := f.jobs[parts[0]]
		if len(parts) != 2 || !known {
			continue // from a runner that had no such job, or its step is not read yet
		}
		emit, err := c.emit(ctx, name, parts[0], store.Number(at), runner)
		if err != nil {
			return err
		}
		f.emits[at] = true
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
		entry.Asks = append(entry.Asks, ask)
	}
	if t, ok := machine.TypeOf(step.State.Type); ok {
		entry.Shown = t.Show(nil, step.State.Inner)
	}
	return entry, nil
}

// emit reads one emit entry of a job. It gives nil for one that is not
// there, or that was not signed by the runner the job went to: a runner can
// add to any record, and what it adds for a job it never had is ignored.
func (c *Client) emit(ctx context.Context, name, job string, number int, runner identity.Card) (*instance.Emit, error) {
	at := store.Emit(c.ctl(), name, job, number)
	data, mark, err := c.Store.Read(ctx, at)
	if err != nil || mark == store.None {
		return nil, err
	}
	var emit instance.Emit
	if _, err := identity.Read(data, at, runner, c.ID, &emit); err != nil || emit.Job != job || emit.Number != number {
		return nil, nil
	}
	return &emit, nil
}

// runnerOf gives the card of the runner a job was sent to, from the step
// that asked for it, looking from the latest step back.
func (c *Client) runnerOf(ctx context.Context, name, job string, latest int) (identity.Card, error) {
	for number := latest; number > 0; number-- {
		step, err := instance.ReadStep(ctx, c.Store, c.Controller, c.ID, name, number)
		if err != nil || step == nil {
			return identity.Card{}, err // below here the steps are trimmed
		}
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskScript && ask.Name == job {
				return ask.Card, nil
			}
		}
	}
	return identity.Card{}, nil
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
// entry of the run's record, or definition) and after the next slash says
// where the blob is in that.
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
		var runner identity.Card
		if runner, err = c.runnerOf(ctx, id, parts[1], latest.Number); err == nil {
			holder, err = c.emit(ctx, id, parts[1], number, runner)
		}
		in = parts[3]
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
	latest, err := c.current(ctx, id)
	if err != nil {
		return nil, err
	}
	output, err := plain(latest.State.Output)
	if err != nil {
		return nil, err
	}
	files := map[string]File{}
	named(output, "", "", files)
	return files, nil
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
