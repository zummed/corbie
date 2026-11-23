package machines

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// A tree is a tree the controller recorded: its prefix, the client that
// owns it, the directory last submitted there, and its locations.
type tree struct {
	Prefix, Client, Directory string
	Locations                 []location
}

// A location is a location of a tree: its name written from the tree's
// prefix, its lifetime, and the hash of the file recorded there.
type location struct {
	At, Name, Lifetime, Hash string
}

// Of what the controller keeps at a tree's prefix and at a location, the
// pages read only these fields.
type (
	treeEntry struct {
		Client    string `json:"client"`
		Directory string `json:"directory"`
		Files     map[string]struct {
			Hash     string `json:"hash"`
			Lifetime string `json:"lifetime"`
		} `json:"files"`
	}
	located struct {
		Definition blob.Ref `json:"definition"`
		Hash       string   `json:"hash"`
	}
)

// sealed reads a value the controller sealed to itself.
func sealed(ctx context.Context, s store.Store, ctl *identity.Identity, name string, into any) (bool, error) {
	data, mark, err := s.Read(ctx, name)
	if err != nil || mark == store.None {
		return false, err
	}
	var raw jsontext.Value
	if _, err := identity.Read(data, name, ctl.Card, ctl, &raw); err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, into)
}

// trees reads every tree the controller recorded, by prefix.
func trees(ctx context.Context, s store.Store, ctl *identity.Identity) ([]tree, error) {
	names, err := store.Names(s.List(ctx, store.Locations(ctl.Name)))
	if err != nil {
		return nil, err
	}
	var out []tree
	for _, name := range names {
		prefix, entry, ok := store.ParseLocation(ctl.Name, name)
		if !ok || entry != "tree" {
			continue
		}
		var r treeEntry
		found, err := sealed(ctx, s, ctl, name, &r)
		if err != nil {
			return nil, fmt.Errorf("the tree at %s: %w", prefix, err)
		}
		if !found {
			continue
		}
		t := tree{Prefix: prefix, Client: r.Client, Directory: r.Directory}
		for _, at := range sortedKeys(r.Files) {
			t.Locations = append(t.Locations, location{At: at, Name: nameIn(prefix, at), Lifetime: r.Files[at].Lifetime, Hash: r.Files[at].Hash})
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b tree) int { return strings.Compare(a.Prefix, b.Prefix) })
	return out, nil
}

// nameIn writes a location from its tree's prefix; a location the prefix
// names in full keeps its last name.
func nameIn(prefix, at string) string {
	if at == prefix {
		return path.Base(at)
	}
	return strings.TrimPrefix(at, strings.TrimSuffix(prefix, "/")+"/")
}

// A recorded is the machine recorded at a location: its tree's prefix, the
// hash of its file, the file it was submitted in, written from the
// directory submitted, and its definition with the child machines it names.
type recorded struct {
	Location, Tree, Hash, File string
	Draft                      *machine.Draft
}

// load reads the definition stored at a reference, and each child machine
// it names, as a draft with no files.
func load(ctx context.Context, s store.Store, ref blob.Ref) (*machine.Draft, error) {
	def, err := machine.Load(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	d := &machine.Draft{Definition: def, Children: map[string]*machine.Draft{}}
	for name, child := range def.Machines {
		if d.Children[name], err = load(ctx, s, ref.Beside(child)); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// An index is what the pages show of the store, read once for each change
// a submit makes: the list, the machine recorded at each location, and the
// toolkits.
type index struct {
	// Stamp is the mark of the trees as they were read: a page reads what
	// it shows again when it differs.
	Stamp    string
	Trees    []listed
	machines map[string]*recorded
	toolkits []toolkit
}

// A listed is a tree of the list, with the machine at each of its
// locations.
type listed struct {
	Prefix, Client, Directory string
	Entries                   []entry
}

// An entry is one machine of the list: its location, whole and written
// from its tree's prefix, its type, its type and name, its lifetime and its
// description.
type entry struct {
	Location, Name, Type, First, Lifetime, Description string
}

// indexed keeps the index of a store, and reads a location's definition
// again only when the hash its tree gives it differs.
type indexed struct {
	store store.Store
	ctl   *identity.Identity
	mu    sync.Mutex
	ix    *index
}

// current gives the index of the store as it is now.
func (c *indexed) current(ctx context.Context) (*index, error) {
	found, err := trees(ctx, c.store, c.ctl)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ix := &index{machines: map[string]*recorded{}}
	sum := sha256.New()
	for _, t := range found {
		l := listed{Prefix: t.Prefix, Client: t.Client, Directory: t.Directory}
		fmt.Fprintf(sum, "%q %q %q\n", t.Prefix, t.Client, t.Directory)
		for _, at := range t.Locations {
			m, err := c.recorded(ctx, t.Prefix, at)
			if err != nil {
				return nil, fmt.Errorf("the machine at %s: %w", at.At, err)
			}
			if m == nil {
				continue // its file is not recorded yet
			}
			ix.machines[at.At] = m
			def := m.Draft.Definition
			l.Entries = append(l.Entries, entry{Location: at.At, Name: at.Name, Type: def.Type, First: typeAndName(def.Type, def.Name),
				Lifetime: def.Lifetime, Description: def.Description})
			fmt.Fprintf(sum, "%q %q\n", at.At, m.Hash)
		}
		ix.Trees = append(ix.Trees, l)
	}
	ix.Stamp = hex.EncodeToString(sum.Sum(nil))
	if c.ix != nil && c.ix.Stamp == ix.Stamp {
		return c.ix, nil
	}
	ix.toolkits = toolkitsOf(ix)
	c.ix = ix
	return ix, nil
}

// recorded gives the machine recorded at a location: the one read before
// where its tree gives the same hash, or nothing where no file is recorded
// there.
func (c *indexed) recorded(ctx context.Context, prefix string, at location) (*recorded, error) {
	if c.ix != nil {
		if m := c.ix.machines[at.At]; m != nil && m.Hash == at.Hash && m.Tree == prefix {
			return m, nil
		}
	}
	var file located
	found, err := sealed(ctx, c.store, c.ctl, store.LocationFile(c.ctl.Name, at.At), &file)
	if err != nil || !found {
		return nil, err
	}
	draft, err := load(ctx, c.store, file.Definition)
	if err != nil {
		return nil, err
	}
	// A file submitted alone is the one that writes its type.
	from := at.Name + ".yaml"
	if def := draft.Definition; at.At == prefix {
		from = cmp.Or(fileOf(def.Lines[def.Type]), from)
	}
	return &recorded{Location: at.At, Tree: prefix, Hash: file.Hash, File: from, Draft: draft}, nil
}

// typeAndName gives a machine's type and name as the list and its page
// head them: the type alone where the name is empty.
func typeAndName(kind, name string) string {
	if kind == "" || name == "" {
		return kind
	}
	return kind + ": " + name
}

// sortedKeys gives a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
