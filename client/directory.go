package client

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// A Tree is a directory read to be submitted at a prefix: each of its *.yaml
// files that .corbieignore does not list is a machine at a location.
type Tree struct {
	Prefix string
	// Directory is the directory's absolute path, every link followed. A
	// machine reads it as run.context.
	Directory string
	Machines  []*Machine
	// Listed is each file and directory .corbieignore lists that the
	// directory holds, from the directory, a directory ending in /.
	Listed []string
}

// A Machine is one *.yaml file of a tree.
type Machine struct {
	Path     string // from the directory, such as devices/boiler.yaml
	Location string // such as /home/devices/boiler
	Draft    *machine.Draft
	Problem  error // what stopped the file being read; Draft is nil
}

// ReadTree reads every *.yaml in dir and below it as a machine, but for .git
// and what the .corbieignore at the top of dir lists. A listed file has no
// location, and a machine may still read it. at is the prefix, by default /
// and the directory's own name, and one that cannot be a prefix is an error,
// as is a line of .corbieignore that is no path in dir. A file that cannot be
// read as a machine, or whose path gives no location, is given with its
// Problem.
func ReadTree(dir, at string) (*Tree, error) {
	// A walk does not follow a link at its start, and the directory may be
	// one.
	full, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if full, err = filepath.EvalSymlinks(full); err != nil {
		return nil, err
	}
	if at == "" {
		at = "/" + filepath.Base(full)
		if store.CheckLocation(at) != nil {
			return nil, fmt.Errorf("the directory's name gives the prefix %s, which cannot be one: a part holds no #, * or +; give --at", at)
		}
	}
	if err := store.CheckLocation(at); err != nil || at == "/" {
		return nil, fmt.Errorf("--at %s: a prefix is / and the names of one or more directories, as /home, and is not / alone", at)
	}
	listed, err := readIgnore(full)
	if err != nil {
		return nil, err
	}
	t := &Tree{Prefix: at, Directory: full}
	err = filepath.WalkDir(full, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(full, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case listed[rel] && d.IsDir():
			t.Listed = append(t.Listed, rel+"/")
			return filepath.SkipDir
		case listed[rel]:
			t.Listed = append(t.Listed, rel)
			return nil
		case d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml"):
			return nil
		}
		m := &Machine{Path: rel}
		m.Location = at + "/" + strings.TrimSuffix(m.Path, ".yaml")
		m.Draft, m.Problem = readMachine(full, m.Path, machine.ParseAt)
		t.Machines = append(t.Machines, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// A location's name is one the store holds, and a location has nothing
	// below it: a file that breaks either is a problem, reported with the
	// rest, and both files of a pair are.
	placed := map[string]*Machine{}
	for _, m := range t.Machines {
		if err := store.CheckLocation(m.Location); err != nil {
			m.Draft, m.Problem = nil, err
			continue
		}
		placed[m.Location] = m
	}
	for _, m := range t.Machines {
		if placed[m.Location] != m {
			continue
		}
		for above := path.Dir(m.Location); above != at && above != "/"; above = path.Dir(above) {
			if a := placed[above]; a != nil {
				a.Draft, a.Problem = nil, fmt.Errorf("it is beside a directory of the same name, which holds the machine at %s: a location has nothing below it", m.Location)
				m.Draft, m.Problem = nil, fmt.Errorf("it is below the location of %s: a location has nothing below it", a.Path)
			}
		}
	}
	return t, nil
}

// ReadFile reads the file at rel in the context dir as a tree of its own,
// whose prefix at is the file's location. A script file is wrapped as Script
// wraps it, with job. A file that cannot be read as a machine is given with
// its Problem.
func ReadFile(dir, rel, at string, job ScriptJob) (*Tree, error) {
	if err := store.CheckLocation(at); err != nil || at == "/" {
		return nil, fmt.Errorf("--at %s: a prefix is / and the names of one or more directories, as /home, and is not / alone", at)
	}
	m := &Machine{Path: rel, Location: at}
	if strings.HasSuffix(rel, ".js") {
		src, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		m.Draft, m.Problem = Script(path.Base(rel), src, job)
	} else {
		m.Draft, m.Problem = readMachine(dir, rel, machine.ParseAt)
	}
	return &Tree{Prefix: at, Directory: dir, Machines: []*Machine{m}}, nil
}

// ignoreFile is the file at the top of a tree's directory that lists what
// in it is no machine.
const ignoreFile = ".corbieignore"

// readIgnore reads the paths the .corbieignore at the top of a directory
// lists, each from the directory: one a line, and a directory for everything
// below it. A line starting with # and an empty line are passed over. A
// directory with none lists nothing.
func readIgnore(dir string) (map[string]bool, error) {
	src, err := os.ReadFile(filepath.Join(dir, ignoreFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	listed := map[string]bool{}
	for i, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if path.IsAbs(line) || slices.Contains(strings.Split(line, "/"), "..") || strings.ContainsFunc(line, func(r rune) bool { return r == '\\' || unicode.IsControl(r) }) {
			return nil, fmt.Errorf("%s line %d: %q: a line is a path from the directory, with no .., no / at its start, no \\ and no control character", ignoreFile, i+1, line)
		}
		listed[path.Clean(line)] = true
	}
	return listed, nil
}

// inside says whether a file is in dir once every link to it is followed.
func inside(dir, full string) error {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("it is outside the directory %s", dir)
	}
	return nil
}

// ReadMachine reads the machine at rel in the context dir. A path in it may
// name any file inside dir and none outside.
func ReadMachine(dir, rel string) (*machine.Draft, error) {
	return readMachine(dir, rel, machine.ParseIn)
}

func readMachine(dir, rel string, parse func(string, []byte, func(string) ([]byte, error)) (*machine.Draft, error)) (*machine.Draft, error) {
	load := func(name string) ([]byte, error) {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := inside(dir, full); err != nil {
			return nil, err
		}
		return os.ReadFile(full)
	}
	src, err := load(rel)
	if err != nil {
		return nil, err
	}
	return parse(rel, src, load)
}

// secretsOf gives the names of the secrets a draft's jobs and its child
// definitions name, in order.
func secretsOf(d *machine.Draft) []string {
	all := map[string]bool{}
	var add func(d *machine.Draft)
	add = func(d *machine.Draft) {
		allow := slices.Clone(d.Definition.Allow)
		for _, job := range machine.Jobs(d.Definition) {
			allow = append(allow, job.Job.Allow...)
		}
		if d.Definition.Ask != nil {
			allow = append(allow, d.Definition.Ask.Allow...)
		}
		for _, name := range secret.Named(allow) {
			all[name] = true
		}
		for _, child := range d.Children {
			add(child)
		}
	}
	add(d)
	return slices.Sorted(maps.Keys(all))
}
