package controller

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// A submit is taken in this order, and every write in it can be made again:
//
//  1. The tree's entry is written, naming the submit and what it does. A
//     submit taken again after a stop reads what it does from there, and
//     is carried out however late it is.
//  2. Each file that reads is copied to its location, then the entry of the
//     location is written, then the blobs it no longer names go.
//  3. The runs to be cancelled are sent a cancel, and each singleton to be
//     started is sent its start and then its key's file is written.
//  4. The locations whose files have gone are removed.
//  5. The receipt is written, then the uploads go, then the message.

// A treeEntry is what the controller keeps at a tree's prefix, sealed to
// itself.
type treeEntry struct {
	Client    string              `json:"client"`
	Directory string              `json:"directory"`
	Files     map[string]treeFile `json:"files,omitzero"`
	// Submit is the message of the submit that wrote the entry, and Does what
	// that submit does.
	Submit string `json:"submit"`
	Does   does   `json:"does"`
}

// A treeFile is what the tree's entry keeps of one of its files.
type treeFile struct {
	Hash     string `json:"hash"`
	Lifetime string `json:"lifetime"`
}

// does is what a submit does, worked out once.
type does struct {
	// Done is what the receipt says of each location.
	Done []instance.Recorded `json:"done,omitzero"`
	// Copy are the locations whose files are recorded, Cancel those whose
	// runs are cancelled, and Remove those whose files have gone.
	Copy   []string `json:"copy,omitzero"`
	Cancel []string `json:"cancel,omitzero"`
	Remove []string `json:"remove,omitzero"`
	// Before is the directory submitted before, when it was another.
	Before  string   `json:"before,omitzero"`
	Missing []string `json:"missing,omitzero"`
}

// A located is what the controller keeps at a location: the file recorded
// there, as a stored definition among the location's blobs, and its tree.
type located struct {
	Tree       string   `json:"tree"`
	Client     string   `json:"client"`
	Definition blob.Ref `json:"definition"`
	Hash       string   `json:"hash"`
	Lifetime   string   `json:"lifetime"`
}

// What a submit did at a location.
const (
	didStart   = "started"
	didRestart = "restarted"
	didRecord  = "recorded"
	didRemove  = "removed"
	didProblem = "problem"
)

// submit takes a client's submit of a tree.
func (p *pass) submit(a *arrival) error {
	msg, base := a.msg.Submit, a.name[strings.LastIndexByte(a.name, '/')+1:]
	answered, err := p.answered(a)
	if err != nil {
		return err
	}
	if answered {
		return p.submitted(a, base)
	}
	if err := store.CheckLocation(msg.Prefix); err != nil {
		return p.refuseSubmit(a, base, err.Error())
	}
	if msg.Prefix == "/" {
		return p.refuseSubmit(a, base, "/ is never a tree: it is where secrets and values every tree uses are set")
	}
	entry, mark, err := p.readTree(msg.Prefix)
	var other otherCase
	if errors.As(err, &other) {
		return p.refuseSubmit(a, base, other.Error())
	}
	if err != nil {
		return err
	}
	if entry != nil && entry.Client == a.sender && entry.Submit == base {
		// The entry names this submit: it was taken when the entry was
		// written, and is carried out however late it is now.
		return p.carryTree(a, base, entry)
	}
	if !a.good(p.now) {
		return p.refuseSubmit(a, base, fmt.Sprintf("the message came too late: it was good until %s", a.until.Format("2006-01-02T15:04:05Z07:00")))
	}
	refused, err := p.checkTree(a, msg)
	if err != nil {
		return err
	}
	if refused != "" {
		return p.refuseSubmit(a, base, refused)
	}
	switch {
	case entry != nil && entry.Client != a.sender:
		return p.refuseSubmit(a, base, fmt.Sprintf("the tree %s belongs to the client %s", msg.Prefix, entry.Client))
	case entry != nil && entry.Submit > base:
		return p.refuseSubmit(a, base, fmt.Sprintf("a later submit of the tree %s was taken before this one", msg.Prefix))
	}
	fresh, err := p.plan(a, base, entry)
	if err != nil {
		return err
	}
	how := store.OnlyIfNew
	if entry != nil {
		how = store.OnlyIfUnchanged(mark)
	}
	at := store.Tree(p.ctl, msg.Prefix)
	data, err := p.c.ID.Write(at, p.now, noEnd, fresh, p.c.ID.Card)
	if err != nil {
		return err
	}
	written, err := p.store.Write(p.ctx, at, data, how)
	if err != nil || !written {
		// Another copy wrote the entry first: the next pass reads it.
		p.later()
		return err
	}
	return p.carryTree(a, base, fresh)
}

// checkTree says why a submit is refused, if it is: a prefix the client's
// ceiling does not allow, a tree inside or above another, a location not
// in the tree or below another's, a file or a blob it names that is not
// among the submit's uploads.
func (p *pass) checkTree(a *arrival, msg *instance.Submit) (string, error) {
	client, err := p.trusted(a.sender)
	if err != nil {
		return "", err
	}
	if !client.Trusted() {
		return fmt.Sprintf("the client %s is not trusted", a.sender), nil
	}
	ceiling := client.Ceiling
	ceiling.Name = "the ceiling of the client " + strconv.Quote(a.sender)
	ceiling.Fix = fmt.Sprintf("the controller's owner runs: corbie controller enrol %s --allow %%s", a.sender)
	if r := (&permission.Set{Layers: []permission.Layer{ceiling}}).Check(permission.At, msg.Prefix); r != nil {
		return r.Error(), nil
	}
	// No tree is above it, and none below.
	for _, above := range store.Above(msg.Prefix)[1:] {
		if above == "/" {
			break
		}
		_, mark, err := p.store.Read(p.ctx, store.Tree(p.ctl, above))
		if err != nil {
			return "", err
		}
		if mark != store.None {
			return fmt.Sprintf("%s is inside the tree %s, and trees do not nest", msg.Prefix, above), nil
		}
	}
	below, err := store.Names(p.store.List(p.ctx, strings.TrimSuffix(store.LocationDir(p.ctl, msg.Prefix), "/+")))
	if err != nil {
		return "", err
	}
	for _, name := range below {
		if location, entry, ok := store.ParseLocation(p.ctl, name); ok && entry == "tree" && location != msg.Prefix {
			return fmt.Sprintf("the tree %s is inside %s, and trees do not nest", location, msg.Prefix), nil
		}
	}
	locations := append(sortedKeys(msg.Files), msg.Problems...)
	slices.Sort(locations)
	for i, location := range locations {
		if err := store.CheckLocation(location); err != nil {
			return err.Error(), nil
		}
		if location != msg.Prefix && !strings.HasPrefix(location, msg.Prefix+"/") {
			return fmt.Sprintf("the location %s is not in the tree %s", location, msg.Prefix), nil
		}
		if i > 0 && strings.HasPrefix(location, locations[i-1]+"/") {
			return fmt.Sprintf("the location %s is below %s, and a location has nothing below it", location, locations[i-1]), nil
		}
	}
	uploads := store.Uploaded(p.ctl, a.sender, a.name[strings.LastIndexByte(a.name, '/')+1:])
	for _, location := range sortedKeys(msg.Files) {
		def := msg.Files[location].Definition
		if path.Dir(def.At) != uploads {
			return fmt.Sprintf("the file at %s is not among the submit's uploads", location), nil
		}
		named, err := blob.Named(p.ctx, p.store, def)
		if blob.Unusable(err) {
			continue // a file that does not read is a problem of its own
		}
		if err != nil {
			return "", err
		}
		for _, ref := range named {
			if path.Dir(ref.At) != uploads {
				return fmt.Sprintf("the file at %s names a blob that is not among the submit's uploads", location), nil
			}
		}
	}
	return "", nil
}

// plan works out what a submit does, from the tree's entry before it.
func (p *pass) plan(a *arrival, base string, before *treeEntry) (*treeEntry, error) {
	msg := a.msg.Submit
	entry := &treeEntry{Client: a.sender, Directory: msg.Directory, Files: map[string]treeFile{}, Submit: base}
	old := map[string]treeFile{}
	if before != nil {
		old = before.Files
		if before.Directory != msg.Directory {
			entry.Does.Before = before.Directory
		}
	}
	d := &entry.Does
	for _, location := range sortedKeys(msg.Files) {
		file := msg.Files[location]
		def, err := machine.Load(p.ctx, p.store, file.Definition)
		if err != nil && !errorsIs(err, machine.ErrDefinition) {
			return nil, err
		}
		if err != nil {
			d.Done = append(d.Done, instance.Recorded{Location: location, What: didProblem})
			if kept, ok := old[location]; ok {
				entry.Files[location] = kept
			}
			continue
		}
		lifetime := def.Lifetime
		if lifetime == "" {
			lifetime = machine.LifetimeTransient
		}
		entry.Files[location] = treeFile{Hash: file.Hash, Lifetime: lifetime}
		// Every file is recorded, so its location has the lines it is
		// written at; one whose machine is the same is not changed.
		d.Copy = append(d.Copy, location)
		was, recorded := old[location]
		changed := !recorded || was.Hash != file.Hash || was.Lifetime != lifetime
		// A file whose lifetime changed is one removed and one new.
		restart := recorded && (was.Lifetime != lifetime || changed && (def.Update == machine.UpdateRestart || lifetime == machine.LifetimeSingleton))
		done := instance.Recorded{Location: location, Lifetime: lifetime, What: didRecord}
		if restart {
			d.Cancel = append(d.Cancel, location)
			done.What = didRestart
		}
		if lifetime == machine.LifetimeSingleton {
			running := false
			if !restart {
				if running, err = p.holds(location); err != nil {
					return nil, err
				}
			}
			if !running {
				if !restart {
					done.What = didStart
				}
				done.Run = store.RootInstance(a.sender, a.sent, base+"\x00"+location)
			}
		}
		d.Done = append(d.Done, done)
		missing, err := secret.Missing(p.ctx, p.store, p.c.ID, location, file.Secrets, file.Values)
		if err != nil {
			return nil, err
		}
		for _, name := range missing {
			d.Missing = append(d.Missing, name+" for "+location)
		}
	}
	for _, location := range msg.Problems {
		d.Done = append(d.Done, instance.Recorded{Location: location, What: didProblem})
		if kept, ok := old[location]; ok {
			entry.Files[location] = kept
		}
	}
	for _, location := range sortedKeys(old) {
		if _, kept := entry.Files[location]; kept {
			continue
		}
		if _, given := msg.Files[location]; given {
			continue
		}
		d.Done = append(d.Done, instance.Recorded{Location: location, Lifetime: old[location].Lifetime, What: didRemove})
		d.Cancel = append(d.Cancel, location)
		d.Remove = append(d.Remove, location)
	}
	slices.SortFunc(d.Done, func(x, y instance.Recorded) int { return strings.Compare(x.Location, y.Location) })
	return entry, nil
}

// holds says whether a location's singleton has a run: its key's file names
// one that is running, or whose start is waiting.
func (p *pass) holds(location string) (bool, error) {
	file, _, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, store.Key(p.ctl, location, ""), "")
	if err != nil && !isAltered(err) || file == nil {
		return false, err
	}
	return p.live(file)
}

// carryTree does what a submit's entry says it does.
func (p *pass) carryTree(a *arrival, base string, entry *treeEntry) error {
	msg, d := a.msg.Submit, &entry.Does
	err := p.each(len(d.Copy), func(i int) error {
		return p.record(msg.Prefix, entry.Client, d.Copy[i], msg.Files[d.Copy[i]], entry.Files[d.Copy[i]].Lifetime)
	})
	if err != nil {
		return err
	}
	starts := map[string]bool{}
	for _, done := range d.Done {
		if done.Run != "" {
			starts[done.Run] = true
		}
	}
	running, err := p.cancelAt(entry.Client, base, d.Cancel, starts)
	if err != nil {
		return err
	}
	var cancelled []string
	for _, runs := range running {
		cancelled = append(cancelled, runs...)
	}
	slices.Sort(cancelled)
	for _, done := range d.Done {
		if done.Run != "" {
			if err := p.startSingleton(base, done, running[done.Location]); err != nil {
				return err
			}
		}
	}
	for _, location := range d.Remove {
		if err := p.removeLocation(location); err != nil {
			return err
		}
	}
	missing := d.Missing
	receipt := &instance.Receipt{Taken: true, Tree: &instance.Submitted{Locations: d.Done, Cancelled: cancelled, Before: d.Before, Missing: missing}}
	if err := p.writeReceipt(a.sender, a.name, receipt); err != nil {
		return err
	}
	return p.submitted(a, base)
}

// record copies a file a submit uploaded to its location, writes the
// location's entry, and removes the blobs the location no longer names.
func (p *pass) record(tree, client, location string, file instance.SubmitFile, lifetime string) error {
	// A carry-out done again finds this upload recorded already.
	recorded, err := p.readLocated(location)
	if err != nil && !isAltered(err) {
		return err
	}
	def := blob.Ref{}
	if recorded != nil && path.Base(recorded.Definition.At) == path.Base(file.Definition.At) {
		def = recorded.Definition
	} else {
		if def, err = blob.CopyWhole(p.ctx, p.store, file.Definition, p.ctl, store.LocationBlobs(p.ctl, location)); err != nil {
			if blob.Unusable(err) {
				return nil // an upload that cannot be read leaves the location as it was
			}
			return err
		}
		at := store.LocationFile(p.ctl, location)
		data, err := p.c.ID.Write(at, p.now, noEnd, located{Tree: tree, Client: client, Definition: def, Hash: file.Hash, Lifetime: lifetime}, p.c.ID.Card)
		if err != nil {
			return err
		}
		if _, err := p.store.Write(p.ctx, at, data, store.Overwrite); err != nil {
			return err
		}
	}
	named, err := blob.Named(p.ctx, p.store, def)
	if err != nil {
		return err
	}
	keep := map[string]bool{def.At: true}
	for _, ref := range named {
		keep[ref.At] = true
	}
	return p.deleteAll(store.LocationBlobs(p.ctl, location), func(name string) bool { return !keep[name] })
}

// deleteAll deletes the names under a path that gone says are to go.
func (p *pass) deleteAll(under string, gone func(name string) bool) error {
	names, err := store.Names(p.store.List(p.ctx, under))
	if err != nil {
		return err
	}
	names = slices.DeleteFunc(names, func(name string) bool { return !gone(name) })
	return p.each(len(names), func(i int) error {
		_, err := p.store.Delete(p.ctx, names[i], store.Overwrite)
		return err
	})
}

// cancelAt cancels every root run at the locations given, of the runs of the
// tree's client, but for those the submit starts. It gives the runs by their
// locations.
func (p *pass) cancelAt(client, base string, locations []string, starts map[string]bool) (map[string][]string, error) {
	if len(locations) == 0 {
		return nil, nil
	}
	names, err := store.Names(p.store.List(p.ctx, store.Running(p.ctl)+"/"+client))
	if err != nil {
		return nil, err
	}
	heads := make([]*instance.Head, len(names))
	err = p.each(len(names), func(i int) error {
		head, _, err := instance.ReadHead(p.ctx, p.store, p.c.ID.Card, store.InstanceOf(p.ctl, names[i]))
		if isAltered(err) {
			return nil
		}
		heads[i] = head
		return err
	})
	if err != nil {
		return nil, err
	}
	cancelled := map[string][]string{}
	for _, head := range heads {
		if head == nil || head.Status != instance.Running || head.Parent != "" || starts[head.Name] || !slices.Contains(locations, head.Location) {
			continue
		}
		msg := &instance.Message{Kind: instance.KindCancel, Instance: head.Name}
		if _, err := p.send(store.Message(p.ctl, p.ctl, store.JobMessage(head.Name, base+".cancel")), noEnd, msg, p.c.ID.Card); err != nil {
			return nil, err
		}
		cancelled[head.Location] = append(cancelled[head.Location], head.Name)
	}
	for _, runs := range cancelled {
		slices.Sort(runs)
	}
	return cancelled, nil
}

// startSingleton starts a location's singleton: its start, which waits for
// the run it replaces to end, and then its key's file, which names it. The
// run it replaces is one running at the location, which the submit has
// cancelled; with none, the run its key's file names, if that run has the
// key. A start still waiting is never the one replaced: it waits itself, and
// is left once the key's file names this one.
func (p *pass) startSingleton(base string, done instance.Recorded, running []string) error {
	at := store.Key(p.ctl, done.Location, "")
	name := store.Message(p.ctl, p.ctl, store.JobMessage(done.Run, base+".start"))
	for {
		file, mark, err := instance.ReadKey(p.ctx, p.store, p.c.ID.Card, p.c.ID, at, "")
		if err != nil && !isAltered(err) {
			return err
		}
		if file != nil && file.Run == done.Run {
			return nil
		}
		replaces := ""
		if len(running) > 0 {
			replaces = running[0]
		} else if file != nil {
			live, err := p.live(file)
			if err != nil {
				return err
			}
			if live {
				replaces = file.Run
			}
		}
		// The start is written only-if-new: one written before stands, with
		// its seed.
		seed := make([]byte, 32)
		rand.Read(seed)
		msg := &instance.Message{Kind: instance.KindStart, Instance: done.Run,
			Start: &instance.Start{Location: done.Location, Replaces: replaces, Seed: seed}}
		if _, err := p.send(name, noEnd, msg, p.c.ID.Card); err != nil {
			return err
		}
		how := store.OnlyIfNew
		if mark != store.None {
			how = store.OnlyIfUnchanged(mark)
		}
		data, err := p.c.ID.Write(at, p.now, noEnd, instance.KeyFile{Run: done.Run, Start: name}, p.c.ID.Card)
		if err != nil {
			return err
		}
		if written, err := p.store.Write(p.ctx, at, data, how); err != nil || written {
			return err
		}
		// The file changed under the write: read it again.
	}
}

// removeLocation removes what is recorded at a location whose file has
// gone: its entry, and then its blobs.
func (p *pass) removeLocation(location string) error {
	if _, err := p.store.Delete(p.ctx, store.LocationFile(p.ctl, location), store.Overwrite); err != nil {
		return err
	}
	return p.deleteAll(store.LocationBlobs(p.ctl, location), func(string) bool { return true })
}

// refuseSubmit answers a submit that is refused.
func (p *pass) refuseSubmit(a *arrival, base, why string) error {
	if err := p.writeReceipt(a.sender, a.name, &instance.Receipt{Refused: why}); err != nil {
		return err
	}
	return p.submitted(a, base)
}

// submitted deletes what a submit leaves: its uploads, and then the message.
func (p *pass) submitted(a *arrival, base string) error {
	if err := p.deleteAll(store.Uploaded(p.ctl, a.sender, base), func(string) bool { return true }); err != nil {
		return err
	}
	_, err := p.store.Delete(p.ctx, a.name, store.Overwrite)
	p.later()
	return err
}

// readTree reads the entry of a tree at a prefix: nil where there is none.
func (p *pass) readTree(prefix string) (*treeEntry, store.Mark, error) {
	at := store.Tree(p.ctl, prefix)
	data, mark, err := p.store.Read(p.ctx, at)
	if err != nil || mark == store.None {
		return nil, mark, err
	}
	var entry treeEntry
	v, err := identity.Read(data, at, p.c.ID.Card, p.c.ID, &entry)
	if err != nil && v.At != at && strings.EqualFold(v.At, at) {
		other, _, _ := store.ParseLocation(p.ctl, v.At)
		return nil, mark, otherCase{tree: other, prefix: prefix}
	}
	if err != nil {
		return nil, mark, fmt.Errorf("the tree at %s: %w", prefix, err)
	}
	return &entry, mark, nil
}

// otherCase is the entry of a tree at a prefix of another case, read where
// a file system that ignores case keeps both in one file.
type otherCase struct{ tree, prefix string }

func (o otherCase) Error() string {
	return fmt.Sprintf("the tree %s is kept in the same file as %s, because this store's file system ignores case", o.tree, o.prefix)
}

// readLocated reads what is recorded at a location: nil where nothing is.
func (p *pass) readLocated(location string) (*located, error) {
	at := store.LocationFile(p.ctl, location)
	data, mark, err := p.store.Read(p.ctx, at)
	if err != nil || mark == store.None {
		return nil, err
	}
	var file located
	if _, err := identity.Read(data, at, p.c.ID.Card, p.c.ID, &file); err != nil {
		return nil, fmt.Errorf("the file at %s: %w", location, err)
	}
	return &file, nil
}

// located gives the start of a run at a location, from the file recorded
// there: copied into the run's record, with the directory last submitted at
// its tree and the values set at the location or above, read once here. It
// gives nil where the start is refused, or waits for a later pass.
func (b *build) located(start *arrival) (*instance.Start, error) {
	p, s := b.p, *start.msg.Start
	if err := store.CheckLocation(s.Location); err != nil {
		b.refuse(start, "%v", err)
		return nil, nil
	}
	if s.Key != "" || s.Run != "" {
		b.refuse(start, "a start of a location names no key")
		return nil, nil
	}
	file, err := p.readLocated(s.Location)
	if err != nil && !isAltered(err) {
		return nil, err
	}
	switch {
	case file == nil:
		b.refuse(start, "nothing is recorded at %s: a run starts at the location of a file in a tree", s.Location)
		return nil, nil
	case store.InstanceClient(b.inst) != file.Client:
		b.refuse(start, "%s is in a tree of the client %s, and only that client starts a run there", s.Location, file.Client)
		return nil, nil
	case start.kind == identity.Client && file.Lifetime == machine.LifetimeSingleton:
		b.refuse(start, "the file at %s is a singleton: its run starts when its tree is submitted", s.Location)
		return nil, nil
	case start.kind == identity.Client && file.Lifetime == machine.LifetimeKeyed:
		b.refuse(start, "the file at %s is keyed: a run starts there when an event is sent to a key", s.Location)
		return nil, nil
	case start.kind == identity.Controller && file.Lifetime != machine.LifetimeSingleton:
		b.drop(start)
		return nil, nil
	}
	if s.Replaces != "" {
		head, _, err := instance.ReadHead(p.ctx, p.store, p.c.ID.Card, s.Replaces)
		if err != nil && !isAltered(err) {
			return nil, err
		}
		if head != nil && head.Status == instance.Running {
			// A singleton started again waits for the run it replaces to
			// end, and so does everything waiting for it.
			b.out.rest = nil
			p.due(p.now.Add(time.Second))
			return nil, nil
		}
	}
	if start.kind == identity.Client {
		// A client's input is the blob its start names in the run's record.
		in := s.InputBlob
		if in.At != store.Blob(p.ctl, b.inst, path.Base(in.At)) {
			b.refuse(start, "a client's start names its input, written in the run's record")
			return nil, nil
		}
		data, err := blob.ReadAll(p.ctx, p.store, in, blob.StepLimit)
		if err != nil {
			if err = p.blobError(err); err != nil {
				return nil, err
			}
			b.refuse(start, "the start's input cannot be read")
			return nil, nil
		}
		s.Input = nil
		if err := identity.Decode(data, &s.Input); err != nil {
			b.refuse(start, "the start's input is not JSON")
			return nil, nil
		}
	}
	def, err := blob.CopyWhole(p.ctx, p.store, file.Definition, p.ctl, store.Blobs(p.ctl, b.inst))
	if err != nil {
		if err = p.blobError(err); err != nil {
			return nil, err
		}
		b.refuse(start, "the file at %s is no longer in the store", s.Location)
		return nil, nil
	}
	tree, _, err := p.readTree(file.Tree)
	if err != nil && !isAltered(err) {
		return nil, err
	}
	values, err := secret.Values(p.ctx, p.store, p.c.ID, s.Location)
	if err != nil {
		if !isAltered(err) {
			return nil, err
		}
		b.refuse(start, "the values set at %s or above cannot be read: %v", s.Location, err)
		return nil, nil
	}
	above := &instance.Above{Values: values}
	if tree != nil {
		above.Context = tree.Directory
	}
	s.Definition, s.Above, s.Tree = def, above, file.Tree
	b.st.Lifetime = file.Lifetime
	return &s, nil
}
