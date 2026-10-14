package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A directory store keeps every value as one plain file at the path its name
// gives. It keeps no party out of anything.
//
// A value is written to a temporary file beside its name and put onto the
// name in one move: a link for only-if-new, a rename otherwise. A rename
// onto a name can hide that name from a listing going on at the same time,
// so a listing holds the directory's lock shared and a replace holds it alone.
// That lock is also what makes only-if-unchanged hold across processes.
type directory struct {
	root string
	*notices
}

const (
	tmpPrefix = ".tmp-"
	lockDir   = ".lock"

	// tmpMaxAge is how old a leftover temporary file must be, by this
	// machine's clock, before a listing removes it.
	tmpMaxAge = time.Hour

	// A write whose directory or temporary file is taken away under it is
	// tried again this many times, waiting a little longer each time.
	directoryTries = 200
	directoryPause = 50 * time.Microsecond
)

// tempWritten is told of each temporary file before it is moved, for a test
// to take it away.
var tempWritten = func(tmp string) {}

// directoryNotices holds, by directory, what the stores of this process opened on
// it share.
var directoryNotices sync.Map

// OpenDirectory opens the directory store at dir. Nothing is made until something
// is written.
func OpenDirectory(dir string) (Store, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	n, _ := directoryNotices.LoadOrStore(root, &notices{})
	return &directory{root: root, notices: n.(*notices)}, nil
}

func (f *directory) path(name string) string {
	return filepath.Join(f.root, filepath.FromSlash(name))
}

// gone says whether err is a file or a directory not being there.
func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func (f *directory) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := checkName(name); err != nil {
		return nil, None, err
	}
	if err := ctx.Err(); err != nil {
		return nil, None, err
	}
	value, err := os.ReadFile(f.path(name))
	switch {
	case err == nil:
		return value, markOf(value), nil
	case gone(err), errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EISDIR):
		return nil, None, nil
	}
	return nil, None, err
}

func (f *directory) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(f.path(name))
	if err != nil {
		if gone(err) || errors.Is(err, fs.ErrPermission) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	part := make([]byte, length)
	n, err := file.ReadAt(part, from)
	if errors.Is(err, syscall.EISDIR) {
		return nil, nil // a path with names under it holds no value
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	return part[:n], nil
}

// changesAtOnce is how many changes one process makes to directories at
// once. Each change waits on the disk, and a process waiting on the disk
// holds a thread of the operating system for each change, so any more wait
// for a turn, which holds none.
const changesAtOnce = 16

var changing = make(chan struct{}, changesAtOnce)

// startChange waits for a turn to change a directory, and gives the func
// that ends it.
func startChange(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case changing <- struct{}{}:
		return func() { <-changing }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *directory) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	end, err := startChange(ctx)
	if err != nil {
		return false, err
	}
	defer end()
	path := f.path(name)
	var done bool
	// The temporary file can go before it is moved: a listing clears old
	// ones, and a write can wait a long time for the lock.
	for range 3 {
		var tmp string
		if tmp, err = writeTemp(filepath.Dir(path), func(w io.Writer) error {
			_, err := w.Write(value)
			return err
		}); err != nil {
			return false, err
		}
		tempWritten(tmp)
		done, err = f.place(ctx, tmp, path, how)
		if !gone(err) {
			break
		}
	}
	if done {
		f.wrote(name)
	}
	return done, err
}

func (f *directory) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	end, err := startChange(ctx)
	if err != nil {
		return false, err
	}
	defer end()
	path := f.path(name)
	tmp, err := writeTemp(filepath.Dir(path), func(w io.Writer) error {
		_, err := io.Copy(w, r)
		return err
	})
	if err != nil {
		return false, err
	}
	done, err := f.place(ctx, tmp, path, OnlyIfNew)
	if done {
		f.wrote(name)
	}
	return done, err
}

// place puts the temporary file onto path in the way how says and leaves no
// temporary file behind. Only this final act answers "already there" or
// "changed".
func (f *directory) place(ctx context.Context, tmp, path string, how How) (bool, error) {
	defer os.Remove(tmp)
	if how.kind == onlyIfNew {
		err := os.Link(tmp, path)
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, f.onDisk(filepath.Dir(path))
	}
	if done, err := f.replace(ctx, tmp, path, how); !done {
		return false, err
	}
	return true, f.onDisk(filepath.Dir(path))
}

// replace renames the temporary file onto path, holding the directory's lock
// alone while it does and no longer.
func (f *directory) replace(ctx context.Context, tmp, path string, how How) (bool, error) {
	unlock, err := f.lock(ctx, true)
	if err != nil {
		return false, err
	}
	defer unlock()
	if how.kind == onlyIfUnchanged {
		if same, err := f.unchanged(path, how.mark); !same {
			return false, err
		}
	}
	err = os.Rename(tmp, path)
	return err == nil, err
}

// dirSynced is told of each directory made safe on disk, for a test to see
// that it was.
var dirSynced = func(dir string) {}

// onDisk makes what was just done to the names in a directory safe on disk:
// a change that was answered is still there after the machine loses power.
// Every directory above it in the directory is made safe as well: one made for
// the name a moment before, by this party or another, is itself a name in
// the one above, and is not on disk until that one is. A directory that has
// gone since was emptied by someone else, who answers for it.
func (f *directory) onDisk(dir string) error {
	for ; ; dir = filepath.Dir(dir) {
		err := syncDir(dir)
		if gone(err) {
			return nil
		}
		if err != nil {
			return err
		}
		dirSynced(dir)
		if dir == f.root {
			return nil
		}
	}
}

// unchanged says whether the file at path still has the mark. The directory's
// lock is held.
func (f *directory) unchanged(path string, mark Mark) (bool, error) {
	value, err := os.ReadFile(path)
	if gone(err) {
		return false, nil
	}
	return err == nil && markOf(value) == mark, err
}

func (f *directory) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := errors.Join(checkName(from), checkName(to)); err != nil {
		return false, err
	}
	end, err := startChange(ctx)
	if err != nil {
		return false, err
	}
	defer end()
	src, dst := f.path(from), f.path(to)
	for i := range directoryTries {
		if err = os.MkdirAll(filepath.Dir(dst), 0o700); err == nil {
			err = os.Link(src, dst)
		}
		switch {
		case err == nil:
			f.wrote(to)
			return true, f.onDisk(filepath.Dir(dst))
		case errors.Is(err, fs.ErrExist):
			if _, serr := os.Lstat(dst); serr == nil {
				return false, nil
			}
		case gone(err):
			if _, serr := os.Lstat(src); gone(serr) {
				return false, notThere(from)
			}
		default:
			return false, err
		}
		time.Sleep(time.Duration(i) * directoryPause)
	}
	return false, fmt.Errorf("store: no link made at %s: %v", to, err)
}

func (f *directory) Delete(ctx context.Context, name string, how How) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	end, err := startChange(ctx)
	if err != nil {
		return false, err
	}
	defer end()
	path := f.path(name)
	if how.kind == onlyIfUnchanged {
		unlock, err := f.lock(ctx, true)
		if err != nil {
			return false, err
		}
		defer unlock()
		if same, err := f.unchanged(path, how.mark); !same {
			return false, err
		}
	}
	err = os.Remove(path)
	if err != nil && !gone(err) {
		return false, err
	}
	if err == nil {
		// Leave no empty directories behind, as far up as they go.
		dir := filepath.Dir(path)
		for ; dir != f.root && os.Remove(dir) == nil; dir = filepath.Dir(dir) {
		}
		f.wrote(name)
		return true, f.onDisk(dir)
	}
	return true, nil
}

func (f *directory) List(ctx context.Context, under string) ([]string, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	unlock, err := f.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	var names []string
	top := f.path(under)
	err = filepath.WalkDir(top, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if gone(err) {
				return nil // deleted while the listing was going on
			}
			return err
		}
		base := d.Name()
		if path == top && !d.IsDir() {
			return nil // a name, not a path with names under it
		}
		if d.IsDir() {
			if path != top && strings.HasPrefix(base, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(base, tmpPrefix) {
			if info, err := d.Info(); err == nil && time.Since(info.ModTime()) > tmpMaxAge {
				os.Remove(path)
			}
			return nil
		}
		rel, err := filepath.Rel(f.root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// First reads the directories under the path one at a time, in order, and
// stops once it has enough names: so a path whose first parts are times,
// as the wakes are, is read only as far as is needed.
func (f *directory) First(ctx context.Context, under string, n int) ([]string, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	n = max(min(n, firstMost), 0)
	entries, err := os.ReadDir(f.path(under))
	if err != nil && !gone(err) {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if len(names) >= n {
			break
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		at := e.Name()
		if under != "" {
			at = under + "/" + at
		}
		if !e.IsDir() {
			names = append(names, at)
			continue
		}
		below, err := f.List(ctx, at)
		if err != nil {
			return nil, err
		}
		names = append(names, below...)
	}
	slices.Sort(names)
	return names[:min(len(names), n)], nil
}

func (f *directory) Notice(under string) (<-chan struct{}, func()) {
	return f.notice(under)
}

// writeTemp fills a new temporary file in dir, safely on disk, and gives its
// path. The directory is made if missing. A delete removes each directory it
// leaves empty, so dir can go between being made and being used: that says
// nothing about the name being written, and is tried again.
func writeTemp(dir string, fill func(io.Writer) error) (string, error) {
	var file *os.File
	var err error
	for i := range directoryTries {
		if err = os.MkdirAll(dir, 0o700); err == nil {
			if file, err = os.CreateTemp(dir, tmpPrefix+"*"); err == nil {
				break
			}
		}
		if !gone(err) && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		time.Sleep(time.Duration(i) * directoryPause)
	}
	if err != nil {
		// Not the error itself, which can read as the name being taken.
		return "", fmt.Errorf("store: no temporary file made in %s: %v", dir, err)
	}
	err = fill(file)
	if err == nil {
		err = file.Sync()
	}
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func notThere(name string) error {
	return fmt.Errorf("store: %w at %s", ErrNothing, name)
}
