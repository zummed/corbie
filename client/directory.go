package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
)

const (
	// KeyFile begins the key of a run of a machine in a directory: the rest
	// is the machine's file, from the context.
	KeyFile = "file:"
	// LabelMachine and LabelHash are the labels of a run of a machine in a
	// directory: its file, and the hash of every file it was read from.
	LabelMachine = "machine"
	LabelHash    = "hash"
)

// A Machine is one machine file of a directory.
type Machine struct {
	Path    string // from the directory, such as bridges/listen.yaml
	Draft   *machine.Draft
	Hash    string // of every file the draft read
	Problem error  // what stopped the file being read; the rest is empty
}

// ReadMachines reads every machine file in dir, and those below it, or in the
// directories of dir it is given: each *.yaml whose first key gives a type of
// machine. Any other holds data, or fields to include. A file that cannot be
// read is given with its Problem.
func ReadMachines(dir string, under ...string) ([]*Machine, error) {
	if len(under) == 0 {
		under = []string{"."}
	}
	// A walk does not follow a link at its start, and the context may be one.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	var found []*Machine
	seen := map[string]bool{}
	for _, sub := range under {
		if err := readMachines(dir, sub, seen, &found); err != nil {
			return found, err
		}
	}
	return found, nil
}

func readMachines(dir, sub string, seen map[string]bool, found *[]*Machine) error {
	return filepath.WalkDir(filepath.Join(dir, filepath.FromSlash(sub)), func(full string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml"):
			return nil
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil {
			return err
		}
		m := &Machine{Path: filepath.ToSlash(rel)}
		if seen[m.Path] {
			return nil
		}
		seen[m.Path] = true
		key, err := firstKey(dir, full)
		if err != nil {
			m.Problem = err
			*found = append(*found, m)
			return nil
		}
		if _, ok := machine.TypeOf(key); !ok {
			return nil
		}
		m.Draft, m.Hash, m.Problem = ReadMachine(dir, m.Path)
		*found = append(*found, m)
		return nil
	})
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
// name any file inside dir and none outside. It gives the draft and a hash of
// every file read.
func ReadMachine(dir, rel string) (*machine.Draft, string, error) {
	read := map[string][]byte{}
	load := func(name string) ([]byte, error) {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := inside(dir, full); err != nil {
			return nil, err
		}
		src, err := os.ReadFile(full)
		read[name] = src
		return src, err
	}
	src, err := load(rel)
	if err != nil {
		return nil, "", err
	}
	draft, err := machine.ParseIn(path.Dir(rel), src, load)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.New()
	for _, name := range sortedNames(read) {
		fmt.Fprintf(sum, "%s\x00%d\x00%s", name, len(read[name]), read[name])
	}
	return draft, hex.EncodeToString(sum.Sum(nil)), nil
}

// A Submit brings the runs of directories of a context into line with their
// files. It keeps nothing of its own: a run is found by its key, so a Submit
// made again starts nothing new.
type Submit struct {
	Client   *Client
	Context  string   // absolute: a machine reads it as run.context
	Dirs     []string // from the context
	Again    []string // files, from the context, to start though their run has ended
	Interval time.Duration
	Say      func(format string, args ...any)

	cancelled map[string]time.Time // when each run was asked to end, by its ID
	started   map[string]bool      // the machines this Submit has started, by file
	failed    map[string]bool      // the machines it could not cancel or start, by file
	problems  []error              // what it has to report
}

// Run makes one pass after another until the runs are as the files say. A
// file with a problem is reported, and leaves its run as it is.
func (s *Submit) Run(ctx context.Context) error {
	s.cancelled, s.started, s.failed, s.problems = map[string]time.Time{}, map[string]bool{}, map[string]bool{}, nil
	machines, err := ReadMachines(s.Context, s.Dirs...)
	if err != nil {
		return err
	}
	for _, file := range s.Again {
		if !slices.ContainsFunc(machines, func(m *Machine) bool { return m.Path == file }) {
			return fmt.Errorf("%s is not a machine in the directories given", file)
		}
	}
	for _, m := range machines {
		if m.Problem != nil {
			s.note(fmt.Errorf("%s: %w", m.Path, m.Problem))
		}
	}
	for {
		runs, err := s.Client.List(ctx, Filter{Finished: true})
		if err != nil {
			return errors.Join(append(s.problems, err)...)
		}
		if s.pass(ctx, machines, runs) {
			return errors.Join(s.problems...)
		}
		select {
		case <-ctx.Done():
			return errors.Join(append(s.problems, ctx.Err())...)
		case <-time.After(s.Interval):
		}
	}
}

// note keeps a problem to report, once.
func (s *Submit) note(problem error) {
	if !slices.ContainsFunc(s.problems, func(e error) bool { return e.Error() == problem.Error() }) {
		s.problems = append(s.problems, problem)
	}
}

// within says whether a file is in one of the directories given.
func (s *Submit) within(file string) bool {
	return slices.ContainsFunc(s.Dirs, func(dir string) bool {
		return dir == "." || file == dir || strings.HasPrefix(file, dir+"/")
	})
}

// pass cancels what no longer fits and starts what has no run. It says whether
// all is as the files say, with nothing still to end or to begin. A machine it
// could not cancel or start is reported, and the rest are still done.
func (s *Submit) pass(ctx context.Context, machines []*Machine, runs []*Run) bool {
	wanted, read, unread := map[string]*Machine{}, map[string]bool{}, map[string]bool{}
	for _, m := range machines {
		read[m.Path] = true
		if m.Problem != nil {
			unread[m.Path] = true
		} else {
			wanted[m.Path] = m
		}
	}
	// The latest run of each file: the runs before it are history.
	latest := map[string]*Run{}
	for _, run := range runs {
		file, ours := strings.CutPrefix(run.Key, KeyFile)
		if !ours || !s.within(file) {
			continue
		}
		if at := latest[file]; at == nil || run.ID > at.ID {
			latest[file] = run
		}
	}
	settled := true
	for file, run := range latest {
		m := wanted[file]
		switch {
		case run.HasEnded(), unread[file], s.failed[file], m != nil && run.Labels[LabelHash] == m.Hash:
		case !read[file] && s.there(file):
			s.note(fmt.Errorf("%s: it no longer reads as a machine, so its run is left as it is", file))
		default:
			if err := s.cancel(ctx, run.ID, file); err != nil {
				s.note(fmt.Errorf("cancelling %s: %w", file, err))
				s.failed[file] = true
				continue
			}
			settled = false
		}
	}
	for _, file := range sortedNames(wanted) {
		run := latest[file]
		switch {
		case s.started[file], s.failed[file]:
			continue
		case run == nil:
		case !run.HasEnded():
			continue
		case run.Labels[LabelHash] == wanted[file].Hash && !slices.Contains(s.Again, file):
			continue
		}
		if _, err := s.start(ctx, wanted[file]); err != nil {
			s.note(fmt.Errorf("starting %s: %w", file, err))
			s.failed[file] = true
			continue
		}
		settled = false
		s.Say("started %s", file)
		s.started[file] = true
	}
	return settled
}

// there says whether a file of the context is still there.
func (s *Submit) there(file string) bool {
	_, err := os.Lstat(filepath.Join(s.Context, filepath.FromSlash(file)))
	return err == nil
}

func (s *Submit) start(ctx context.Context, m *Machine) (string, error) {
	start := Start{
		Key:     KeyFile + m.Path,
		Context: s.Context,
		Labels:  map[string]string{LabelMachine: m.Path, LabelHash: m.Hash},
	}
	id, err := s.Client.Start(ctx, m.Draft, start)
	if err != nil {
		return "", err
	}
	_, message, _ := strings.Cut(id, "/")
	receipt, err := s.Client.WaitReceipt(ctx, message, start.Life, s.Interval)
	switch {
	case err != nil:
		return "", err
	case !receipt.Taken && len(receipt.Problems) > 0:
		return "", fmt.Errorf("%s:\n%v", receipt.Refused, receipt.Problems)
	case !receipt.Taken:
		return "", errors.New(receipt.Refused)
	}
	return id, nil
}

// cancel asks a run to end, once, and fails when it has not ended by the time
// the message is no longer good.
func (s *Submit) cancel(ctx context.Context, id, file string) error {
	if at, asked := s.cancelled[id]; asked {
		if s.Client.Now().Sub(at) >= instance.MessageLife {
			return errors.New("the run has not ended")
		}
		return nil
	}
	if _, err := s.Client.Cancel(ctx, id, 0); err != nil {
		return err
	}
	s.cancelled[id] = s.Client.Now()
	s.Say("cancelled %s", file)
	return nil
}
