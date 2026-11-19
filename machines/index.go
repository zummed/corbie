package machines

import (
	"cmp"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
)

// An index is what the pages show of the directory, read once each time a
// file in it changes or its prefix does: each machine as Corbie reads it, at
// its location under the prefix, the list, the files and directories that
// are no machine, whether the directory has a .corbieignore, the files it
// lists, each problem once, and the toolkits.
type index struct {
	Prefix   string
	machines map[string]*client.Machine
	located  map[string]string // the file of the machine at each location
	own      map[string][]problem
	Groups   []group
	Files    []entry
	Ignore   bool
	listed   map[string]bool
	included map[string][]string
	Problems []problem
	Unread   string
	toolkits []toolkit
}

// A group is the machines of the list in one directory.
type group struct {
	Dir     string
	Entries []entry
}

// An entry is one machine or one other file in the list: its file; a
// machine's type and name, location, lifetime, description and how
// many problems it has; and whether .corbieignore lists another file or
// directory, a directory's path ending in /, and how many machines include
// it.
type entry struct {
	Path, Type, First, Location, Lifetime, Description string
	Problems, Included                                 int
	Listed                                             bool
}

// indexed keeps the index of a directory, and reads it again only when the
// directory changed, or the prefix it is read at.
type indexed struct {
	dir       *dir
	mu        sync.Mutex
	stamp, at string
	ix        *index
}

// current gives the index of the directory as it is now, read at the
// prefix at, or at the prefix a submit gives it by default where at is "".
func (c *indexed) current(at string) *index {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stamp := c.dir.stamp(); c.ix == nil || stamp != c.stamp || at != c.at {
		c.ix, c.stamp, c.at = c.dir.index(at), stamp, at
	}
	return c.ix
}

// index reads every machine in the directory with Corbie's own reader, as
// a tree at the prefix at, or at the prefix a submit gives it by default
// where at is "".
func (d *dir) index(at string) *index {
	ix := &index{machines: map[string]*client.Machine{}, located: map[string]string{}, own: map[string][]problem{}, listed: map[string]bool{}, included: map[string][]string{}}
	tree, err := client.ReadTree(d.root, at)
	if err != nil {
		ix.Unread = err.Error()
		return ix
	}
	ix.Prefix = tree.Prefix
	found := tree.Machines
	for _, m := range found {
		ix.machines[m.Path] = m
		if m.Location != "" {
			ix.located[m.Location] = m.Path
		}
		if m.Problem != nil {
			ix.own[m.Path] = slices.SortedStableFunc(slices.Values(d.problemsOf(m.Path, m.Problem)), byPlace)
		}
	}
	// A file an include: names that cannot be read and is itself a machine
	// gives the problem it has as a machine, so the machines that include
	// it are broken by that one problem.
	for _, problems := range ix.own {
		for i, p := range problems {
			if mine := ix.own[p.File]; p.included && len(mine) > 0 && mine[0].File == p.File {
				problems[i] = mine[0]
			}
		}
	}
	ix.Problems = d.problemsOnce(ix.own)
	groups := map[string][]entry{}
	for _, m := range found {
		en := entry{Path: m.Path, Location: m.Location, Problems: len(ix.at(m.Path))}
		if m.Draft != nil {
			def := m.Draft.Definition
			en.Type, en.First, en.Lifetime, en.Description = def.Type, def.Name, def.Lifetime, def.Description
			for _, file := range filesOf(m.Draft.Definition) {
				if file != m.Path && ix.machines[file] == nil {
					ix.included[file] = append(ix.included[file], m.Path)
				}
			}
		} else if src, _, err := d.read(m.Path); err == nil {
			en.Type, en.First = typeIn(src)
		}
		en.First = typeAndName(en.Type, en.First)
		groups[path.Dir(m.Path)] = append(groups[path.Dir(m.Path)], en)
	}
	for _, at := range sortedKeys(groups) {
		ix.Groups = append(ix.Groups, group{Dir: at, Entries: groups[at]})
	}
	files, err := d.files()
	if err != nil {
		ix.Unread = err.Error()
	}
	for _, file := range files {
		ix.Ignore = ix.Ignore || file == ".corbieignore"
		if slices.ContainsFunc(tree.Listed, func(l string) bool { return l == file || strings.HasSuffix(l, "/") && strings.HasPrefix(file, l) }) {
			ix.listed[file] = true
		}
	}
	shown := map[string]bool{}
	for _, l := range tree.Listed {
		shown[l] = true
	}
	for file := range ix.included {
		shown[file] = true
	}
	for _, file := range sortedKeys(shown) {
		ix.Files = append(ix.Files, entry{Path: file, Listed: ix.listed[file] || slices.Contains(tree.Listed, file), Included: len(ix.included[file])})
	}
	ix.toolkits = toolkitsOf(ix)
	return ix
}

// file gives the entry of a file that is no machine: one .corbieignore
// lists, itself or in a directory it lists, or one a machine takes a value
// from.
func (ix *index) file(name string) *entry {
	for i := range ix.Files {
		if ix.Files[i].Path == name {
			return &ix.Files[i]
		}
	}
	if ix.listed[name] {
		return &entry{Path: name, Listed: true}
	}
	return nil
}

// typeAndName gives a machine's type and name as the list and its page
// head them: the type alone where the name is empty.
func typeAndName(kind, name string) string {
	if kind == "" || name == "" {
		return kind
	}
	return kind + ": " + name
}

// filesOf gives the files a definition written out takes its values from,
// in order, its own among them.
func filesOf(def *machine.Definition) []string {
	files := map[string]bool{}
	for _, at := range def.Lines {
		if i := strings.LastIndexByte(at, ':'); i > 0 {
			files[at[:i]] = true
		}
	}
	return sortedKeys(files)
}

// problemsOnce gives each problem of the directory's machines once, at the
// file and line Corbie names, with the machines it breaks. Machines that
// include a file under different names give a problem in it a different
// place each, and it is shown with the first. A machine that reads
// without problems from a directory below this one gives one problem: that
// directory is the top of its tree.
func (d *dir) problemsOnce(own map[string][]problem) []problem {
	var tops, rest []*problem
	found := map[string]*problem{}
	read := map[string]map[string]bool{}
	for _, file := range sortedKeys(own) {
		if top := d.topOf(file, read); top != "" {
			if found[top] == nil {
				found[top] = &problem{File: top, Top: true}
				tops = append(tops, found[top])
			}
			found[top].Breaks = append(found[top].Breaks, file)
			continue
		}
		for _, p := range own[file] {
			key := p.File + "\x00" + strconv.Itoa(p.Line) + "\x00" + p.Text
			if found[key] == nil {
				p.Breaks = nil
				found[key] = &p
				rest = append(rest, found[key])
			}
			if !slices.Contains(found[key].Breaks, file) {
				found[key].Breaks = append(found[key].Breaks, file)
			}
		}
	}
	slices.SortStableFunc(rest, func(a, b *problem) int { return byPlace(*a, *b) })
	var out []problem
	for _, p := range append(tops, rest...) {
		out = append(out, *p)
	}
	return out
}

// byPlace orders problems by their file, then their line.
func byPlace(a, b problem) int {
	return cmp.Or(strings.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}

// topOf gives the directory below this one from which a machine reads without
// problems, nearest first, or nothing. Each directory is read as a tree of
// its own, with its own .corbieignore, once: read keeps the machines that read
// without problems from each.
func (d *dir) topOf(file string, read map[string]map[string]bool) string {
	for at := path.Dir(file); at != "."; at = path.Dir(at) {
		if _, ok := read[at]; !ok {
			read[at] = d.readable(at)
		}
		if read[at][strings.TrimPrefix(file, at+"/")] {
			return at
		}
	}
	return ""
}

// readable gives the machines that read without problems from a directory
// below this one, read as a tree of its own, by their paths from it.
func (d *dir) readable(at string) map[string]bool {
	good := map[string]bool{}
	tree, err := client.ReadTree(filepath.Join(d.root, filepath.FromSlash(at)), "/top")
	if err != nil {
		return good
	}
	for _, m := range tree.Machines {
		if m.Problem == nil {
			good[m.Path] = true
		}
	}
	return good
}

// at gives the problems found in a file: a machine's own, each with the
// machines it breaks as the list gives them, or the one problem of the
// directory that is the top of its tree, or for any other file those
// found in it by the machines that read it.
func (ix *index) at(file string) []problem {
	for _, p := range ix.Problems {
		if p.Top && slices.Contains(p.Breaks, file) {
			return []problem{p}
		}
	}
	if _, ok := ix.machines[file]; ok {
		var out []problem
		for _, p := range ix.own[file] {
			for _, listed := range ix.Problems {
				if listed.File == p.File && listed.Place == p.Place && listed.Text == p.Text {
					p.Breaks = listed.Breaks
				}
			}
			out = append(out, p)
		}
		return out
	}
	var out []problem
	for _, p := range ix.Problems {
		if p.File == file && !p.Top {
			out = append(out, p)
		}
	}
	return out
}
