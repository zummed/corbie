// Package pack writes packs into a project. A pack is a directory of
// ready-made machines, in a git repository or on this computer.
package pack

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Dir is the directory in a project that holds its packs. Corbie rewrites
// it, and a person changes nothing in it.
const Dir = ".packs"

// starter is the directory in a pack that holds its starting files.
const starter = "starter"

// record is the file in a pack's directory that says where it came from.
const record = ".pack"

// A Source is where a pack comes from, as corbie add was given it.
type Source struct {
	Text string // as it was written, with its ref
	Repo string // what git fetches from; empty for a directory that is copied as it is
	Path string // the pack's directory: under the repository, or on this computer
	Ref  string // a tag, a branch or a commit; empty for the default branch
	Name string // the pack's name: the last part of its path
}

var packName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Parse reads a pack as it is written: host/owner/repository/path@ref, or
// a directory on this computer. A repository whose address has more than
// three parts is written with .git after it, as host/a/b/c.git/path. A
// directory with a ref is read from the git repository it is in.
func Parse(text string) (Source, error) {
	src := Source{Text: text}
	at, ref, has := strings.Cut(text, "@")
	if has && ref == "" {
		return src, fmt.Errorf("%q names no ref after @", text)
	}
	src.Ref = ref
	local := strings.HasPrefix(at, ".") || strings.HasPrefix(at, "/") || filepath.IsAbs(at)
	switch {
	case local && !has:
		src.Path = filepath.Clean(at)
		abs, err := filepath.Abs(src.Path)
		if err != nil {
			return src, err
		}
		src.Name = filepath.Base(abs)
	case local:
		abs, err := filepath.Abs(at)
		if err != nil {
			return src, err
		}
		root := abs
		for {
			if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
				break
			}
			if filepath.Dir(root) == root {
				return src, fmt.Errorf("%s is in no git repository, so it has no %s", at, ref)
			}
			root = filepath.Dir(root)
		}
		src.Repo = root
		src.Path, _ = filepath.Rel(root, abs)
		src.Name = filepath.Base(abs)
	default:
		parts := strings.Split(strings.Trim(at, "/"), "/")
		n := 3
		for i, part := range parts {
			if strings.HasSuffix(part, ".git") {
				n = i + 1
			}
		}
		if len(parts) < n || len(parts) < 3 {
			return src, fmt.Errorf("%q is not a pack: write host/owner/repository/path, or a directory starting with ./ or /", text)
		}
		src.Repo = "https://" + strings.Join(parts[:n], "/")
		src.Path = filepath.Join(parts[n:]...)
		src.Name = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}
	if !packName.MatchString(src.Name) {
		return src, fmt.Errorf("%q is not a pack's name: use letters, digits, '_' and '-'", src.Name)
	}
	return src, nil
}

// A Written is one pack written into a project.
type Written struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"` // as corbie add was given it
	Commit string `json:"commit,omitempty"` // the commit it was read at
	Needed string `json:"needed_by,omitempty"`
}

// A Report is what corbie add did.
type Report struct {
	Packs   []Written `json:"packs"`
	Started []string  `json:"started"` // the starting files it wrote
	Differ  []string  `json:"differ"`  // the starting files that were there, and are not as the pack starts them
}

// Add writes a pack into a project. It rewrites the pack's directory under
// .packs, and that of each pack of the same source that the pack's files
// name. It writes each starting file that is not there and that it has not
// written before, with each word set written in place of itself in the
// file's path and text. It adds .packs/
// to the project's .corbieignore when it lacks it. It changes no other
// file.
func Add(project string, src Source, words map[string]string) (Report, error) {
	var rep Report
	dir, commit, done, err := fetch(src)
	if err != nil {
		return rep, err
	}
	defer done()
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return rep, fmt.Errorf("%s holds no pack %s", src.Text, src.Name)
	}
	if err := check(dir, src, words); err != nil {
		return rep, err
	}
	// The packs the pack's files name are its siblings in the source.
	type found struct{ name, dir, needed string }
	packs := []found{{src.Name, dir, ""}}
	for i := 0; i < len(packs); i++ {
		names, err := named(packs[i].dir)
		if err != nil {
			return rep, err
		}
		for _, name := range names {
			if slices.ContainsFunc(packs, func(p found) bool { return p.name == name }) {
				continue
			}
			sibling := filepath.Join(filepath.Dir(dir), name)
			if info, err := os.Stat(sibling); err != nil || !info.IsDir() {
				return rep, fmt.Errorf("the pack %s names %s/%s/, and %s holds no pack %s beside it", packs[i].name, Dir, name, src.Text, name)
			}
			packs = append(packs, found{name, sibling, packs[i].name})
		}
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		return rep, err
	}
	// A starting file written before is not written again: a person who
	// removed it meant to.
	before := started(filepath.Join(project, Dir, src.Name))
	starting, err := startingFiles(dir, words)
	if err != nil {
		return rep, err
	}
	for _, rel := range sorted(starting) {
		to := filepath.Join(project, rel)
		have, err := os.ReadFile(to)
		switch {
		case os.IsNotExist(err) && slices.Contains(before, rel):
		case os.IsNotExist(err):
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return rep, err
			}
			if err := os.WriteFile(to, []byte(starting[rel]), 0o644); err != nil {
				return rep, err
			}
			rep.Started = append(rep.Started, rel)
		case err != nil:
			return rep, err
		case string(have) != starting[rel]:
			rep.Differ = append(rep.Differ, rel)
		}
	}
	for _, rel := range rep.Started {
		before = append(before, rel)
	}
	slices.Sort(before)
	for _, p := range packs {
		to := filepath.Join(project, Dir, p.name)
		// A pack a person added is not replaced by one another pack needs.
		if p.needed != "" && added(to) {
			continue
		}
		if err := os.RemoveAll(to); err != nil {
			return rep, err
		}
		if err := copyPack(p.dir, to); err != nil {
			return rep, err
		}
		w := Written{Name: p.name, Commit: commit, Needed: p.needed}
		if p.needed == "" {
			w.Source = src.Text
		}
		text := w.record(words)
		if p.needed == "" {
			for _, rel := range before {
				text += "started " + filepath.ToSlash(rel) + "\n"
			}
		}
		if err := os.WriteFile(filepath.Join(to, record), []byte(text), 0o644); err != nil {
			return rep, err
		}
		rep.Packs = append(rep.Packs, w)
	}
	return rep, list(filepath.Join(project, ".corbieignore"))
}

// Added gives the source and the words of each pack in a project that a
// person added, by the pack's name, as its record holds them.
func Added(project string) (map[string]Source, map[string]map[string]string, error) {
	sources, words := map[string]Source{}, map[string]map[string]string{}
	entries, err := os.ReadDir(filepath.Join(project, Dir))
	if os.IsNotExist(err) {
		return sources, words, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		text, err := os.ReadFile(filepath.Join(project, Dir, e.Name(), record))
		if err != nil {
			continue
		}
		set := map[string]string{}
		for _, line := range strings.Split(string(text), "\n") {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "source":
				src, err := Parse(value)
				if err != nil {
					return nil, nil, err
				}
				sources[e.Name()] = src
			case "set":
				word, to, _ := strings.Cut(value, "=")
				set[word] = to
			}
		}
		if _, ok := sources[e.Name()]; ok {
			words[e.Name()] = set
		}
	}
	return sources, words, nil
}

// started gives the starting files a pack's record says were written.
func started(dir string) []string {
	text, _ := os.ReadFile(filepath.Join(dir, record))
	var out []string
	for _, line := range strings.Split(string(text), "\n") {
		if rel, ok := strings.CutPrefix(line, "started "); ok {
			out = append(out, filepath.FromSlash(rel))
		}
	}
	return out
}

// added says whether a pack's directory holds the record of a pack a person
// added.
func added(dir string) bool {
	text, _ := os.ReadFile(filepath.Join(dir, record))
	return strings.HasPrefix(string(text), "source ")
}

// record gives a pack's record: its source or the pack that needs it, its
// commit, and the words it was added with.
func (w Written) record(words map[string]string) string {
	var b strings.Builder
	if w.Needed != "" {
		fmt.Fprintf(&b, "needed-by %s\n", w.Needed)
	} else {
		fmt.Fprintf(&b, "source %s\n", w.Source)
	}
	if w.Commit != "" {
		fmt.Fprintf(&b, "commit %s\n", w.Commit)
	}
	if w.Needed == "" {
		for _, word := range sorted(words) {
			fmt.Fprintf(&b, "set %s=%s\n", word, words[word])
		}
	}
	return b.String()
}

// fetch gives the directory of a source's pack on this computer, and the
// commit it was read at: the directory itself, or a copy git fetched,
// which done removes.
func fetch(src Source) (dir, commit string, done func(), err error) {
	if src.Repo == "" {
		return src.Path, "", func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "corbie-pack-")
	if err != nil {
		return "", "", nil, err
	}
	done = func() { os.RemoveAll(tmp) }
	ref := src.Ref
	if ref == "" {
		ref = "HEAD"
	}
	for _, args := range [][]string{
		{"init", "-q", tmp},
		{"-C", tmp, "fetch", "-q", "--depth", "1", src.Repo, ref},
		{"-C", tmp, "checkout", "-q", "FETCH_HEAD"},
	} {
		if _, err := git(args...); err != nil {
			done()
			return "", "", nil, fmt.Errorf("%s: %w", src.Text, err)
		}
	}
	commit, err = git("-C", tmp, "rev-parse", "HEAD")
	if err != nil {
		done()
		return "", "", nil, err
	}
	return filepath.Join(tmp, src.Path), commit, done, nil
}

// git runs git, and gives what it printed. A failure carries what git
// said.
func git(args ...string) (string, error) {
	var out, said bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &out, &said
	if err := cmd.Run(); err != nil {
		if text := strings.TrimSpace(said.String()); text != "" {
			return "", fmt.Errorf("git: %s", text)
		}
		return "", fmt.Errorf("git: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

var names = regexp.MustCompile(`\.packs/([A-Za-z0-9][A-Za-z0-9_-]*)/`)

// named gives the packs a pack's definitions and scripts name, as
// .packs/<name>/, other than itself, sorted. A page that names a pack
// needs none.
func named(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !slices.Contains([]string{".yaml", ".js"}, filepath.Ext(path)) {
			return err
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range names.FindAllSubmatch(text, -1) {
			if name := string(m[1]); name != filepath.Base(dir) && !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

var wordName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

var capitals = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// check refuses a word the pack's starting files do not hold, and a
// starting directory named in capitals whose word is not set.
func check(dir string, src Source, words map[string]string) error {
	starting, err := startingFiles(dir, nil)
	if err != nil {
		return err
	}
	for _, word := range sorted(words) {
		if !capitals.MatchString(word) {
			return fmt.Errorf("--set %s: a word is written in capitals, as TEAM", word)
		}
		if !wordName.MatchString(words[word]) {
			return fmt.Errorf("--set %s=%s: %q is not a name: use letters, digits, '_' and '-', starting with a letter", word, words[word], words[word])
		}
		held := false
		for rel, text := range starting {
			held = held || strings.Contains(rel, word) || strings.Contains(text, word)
		}
		if !held {
			return fmt.Errorf("--set %s: the starting files of the pack %s hold no %s", word, src.Name, word)
		}
	}
	for _, rel := range sorted(starting) {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for _, part := range parts[:len(parts)-1] {
			if _, set := words[part]; capitals.MatchString(part) && !set {
				return fmt.Errorf("the pack %s takes %s: add --set %s=<name>", src.Name, part, part)
			}
		}
	}
	return nil
}

// startingFiles gives the text of each of a pack's starting files, by its
// path in a project, with each word written as it is set.
func startingFiles(dir string, words map[string]string) (map[string]string, error) {
	out := map[string]string{}
	from := filepath.Join(dir, starter)
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == from {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		body := string(text)
		for _, word := range sorted(words) {
			rel, body = strings.ReplaceAll(rel, word, words[word]), strings.ReplaceAll(body, word, words[word])
		}
		out[rel] = body
		return nil
	})
	return out, err
}

// copyPack copies a pack's files to its directory in a project, without
// its starting files.
func copyPack(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if d.IsDir() {
			if rel == starter || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(to, rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), text, 0o644)
	})
}

// list adds .packs/ to a .corbieignore that does not hold it.
func list(name string) error {
	text, err := os.ReadFile(name)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := Dir + "/"
	if slices.Contains(strings.Split(string(text), "\n"), line) {
		return nil
	}
	if len(text) > 0 && !bytes.HasSuffix(text, []byte("\n")) {
		line = "\n" + line
	}
	return os.WriteFile(name, append(text, line+"\n"...), 0o644)
}

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
