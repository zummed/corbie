package client

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// A Tree is a directory read to be submitted at a prefix: each of its files
// with a type line is a machine at a location.
type Tree struct {
	Prefix string
	// Directory is the directory's absolute path, every link followed. A
	// machine reads it as run.context.
	Directory string
	Machines  []*Machine
}

// A Machine is one machine file of a tree.
type Machine struct {
	Path     string // from the directory, such as devices/boiler.yaml
	Location string // such as /home/devices/boiler
	Draft    *machine.Draft
	Problem  error // what stopped the file being read; Draft is nil
}

// ReadTree reads every machine file in dir and below it, but for .git: each
// *.yaml whose first key gives a type of machine. Any other is a piece, which
// a machine reads by including it. at is the prefix, by default / and the
// directory's own name. A file that cannot be read is given with its
// Problem; a tree whose files cannot all have locations is an error.
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
	}
	if err := store.CheckLocation(at); err != nil || at == "/" {
		return nil, fmt.Errorf("--at %s: a prefix is / and the names of one or more directories, as /home, and is not / alone", at)
	}
	t := &Tree{Prefix: at, Directory: full}
	err = filepath.WalkDir(full, func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml"):
			return nil
		}
		rel, err := filepath.Rel(full, name)
		if err != nil {
			return err
		}
		m := &Machine{Path: filepath.ToSlash(rel)}
		m.Location = at + "/" + strings.TrimSuffix(m.Path, ".yaml")
		key, err := firstKey(full, name)
		if err != nil {
			m.Problem = err
			t.Machines = append(t.Machines, m)
			return nil
		}
		if _, ok := machine.TypeOf(key); !ok {
			return nil
		}
		m.Draft, m.Problem = readMachine(full, m.Path, machine.ParseAt)
		t.Machines = append(t.Machines, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// A location has nothing below it, and its name is one the store holds.
	locations := make([]string, len(t.Machines))
	for i, m := range t.Machines {
		if err := store.CheckLocation(m.Location); err != nil {
			return nil, fmt.Errorf("%s: %w", m.Path, err)
		}
		locations[i] = m.Location
	}
	slices.Sort(locations)
	for i := 1; i < len(locations); i++ {
		if strings.HasPrefix(locations[i], locations[i-1]+"/") {
			return nil, fmt.Errorf("%s.yaml is beside a directory of the same name, which holds the machine at %s: a location has nothing below it", strings.TrimPrefix(locations[i-1], at+"/"), locations[i])
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

// firstKey gives the first key of a YAML file, or "" when it holds no mapping.
// A file that is not YAML, as one half written is, is a problem: it may be a
// machine.
func firstKey(dir, full string) (string, error) {
	if err := inside(dir, full); err != nil {
		return "", err
	}
	src, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 {
		return "", nil
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode || len(top.Content) == 0 {
		return "", nil
	}
	return top.Content[0].Value, nil
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
