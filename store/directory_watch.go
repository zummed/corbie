package store

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// A watcher has the operating system report what any process on the
// computer writes into a directory store, for the processes here that wait
// on it. It watches the inboxes and the running heads alone: they hold only
// what is in flight, so watching them costs the same however many runs the
// store has kept. A directory the system will not watch is not reported,
// and whoever waits on it looks again at its own interval.
type watcher struct {
	root  string
	wrote func(name string)

	mu    sync.Mutex
	fs    *fsnotify.Watcher // nil while nobody waits
	users int
	paths []string        // the inboxes and running heads waited on, as names
	dirs  map[string]bool // the directories watched, as file paths
}

// inFlight gives the inboxes and the running heads under a path: those of a
// party for the party's own area, and the path itself where it is in one of
// them.
func inFlight(under string) []string {
	if under == "" {
		return nil
	}
	party, _, deeper := strings.Cut(under, "/")
	if !deeper {
		return []string{Inbox(party), Running(party)}
	}
	for _, path := range []string{Inbox(party), Running(party)} {
		if under == path || within(under, path) {
			return []string{under}
		}
	}
	return nil
}

// watch has the system report writes under a path, and gives a func that
// ends it.
func (w *watcher) watch(under string) func() {
	paths := inFlight(under)
	if len(paths) == 0 {
		return func() {}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fs == nil {
		fs, err := fsnotify.NewWatcher()
		if err != nil {
			return func() {}
		}
		w.fs, w.paths, w.dirs = fs, nil, map[string]bool{}
		go w.run(fs)
	}
	w.users++
	for _, path := range paths {
		if !slices.Contains(w.paths, path) {
			w.paths = append(w.paths, path)
			w.cover(path, false)
		}
	}
	return sync.OnceFunc(func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.users--; w.users == 0 {
			w.fs.Close()
			w.fs = nil
		}
	})
}

// run takes each event as the system gives it, and reports them from a
// second goroutine. On Windows a watch is added by the thread that gives the
// events, and that thread waits for each event to be taken: so an event is
// always taken at once, also while a watch is being added.
func (w *watcher) run(fs *fsnotify.Watcher) {
	var mu sync.Mutex
	var taken []fsnotify.Event
	more := make(chan struct{}, 1)
	defer close(more)
	go func() {
		for range more {
			mu.Lock()
			events := taken
			taken = nil
			mu.Unlock()
			w.mu.Lock()
			for _, event := range events {
				if w.fs == fs {
					w.event(event)
				}
			}
			w.mu.Unlock()
		}
	}()
	for {
		select {
		case event, open := <-fs.Events:
			if !open {
				return
			}
			mu.Lock()
			taken = append(taken, event)
			mu.Unlock()
			select {
			case more <- struct{}{}:
			default:
			}
		case _, open := <-fs.Errors:
			// Events the system dropped are writes not reported.
			if !open {
				return
			}
		}
	}
}

// event reports a name made, replaced or removed. A store deletes a
// directory it leaves empty and makes it again for the next write, so a
// directory made is watched from then on, and one removed no longer.
func (w *watcher) event(event fsnotify.Event) {
	at := event.Name
	switch {
	case strings.HasPrefix(filepath.Base(at), tmpPrefix):
	case event.Has(fsnotify.Create):
		if info, err := os.Lstat(at); err == nil && info.IsDir() {
			delete(w.dirs, at)
			w.made(at)
			return
		}
		w.report(at)
	case event.Has(fsnotify.Remove), event.Has(fsnotify.Rename):
		if w.dirs[at] {
			delete(w.dirs, at)
			w.fs.Remove(at)
			return
		}
		w.report(at)
	}
}

// made watches a directory just made that leads to a path waited on or is
// under one, and reports the names put there before it was watched.
func (w *watcher) made(dir string) {
	name := w.name(dir)
	for _, path := range w.paths {
		switch {
		case name == path || within(path, name):
			w.cover(path, true)
		case within(name, path):
			if w.add(dir) {
				w.below(dir, true)
			}
		}
	}
}

// cover watches each directory from the root down to a path, so that one
// made later is seen, and every directory under the path.
func (w *watcher) cover(path string, report bool) {
	dir := w.root
	if !w.add(dir) {
		return
	}
	for part := range strings.SplitSeq(path, "/") {
		dir = filepath.Join(dir, part)
		if !w.add(dir) {
			return
		}
	}
	w.below(dir, report)
}

// below watches every directory under a watched one.
func (w *watcher) below(dir string, report bool) {
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		at := filepath.Join(dir, entry.Name())
		switch {
		case entry.IsDir():
			if w.add(at) {
				w.below(at, report)
			}
		case report:
			w.report(at)
		}
	}
}

// add watches a directory, and says whether it is watched.
func (w *watcher) add(dir string) bool {
	if !w.dirs[dir] {
		if w.fs.Add(dir) != nil {
			return false
		}
		w.dirs[dir] = true
	}
	return true
}

// report tells those waiting of a name under a path waited on.
func (w *watcher) report(at string) {
	name := w.name(at)
	if strings.HasPrefix(filepath.Base(at), tmpPrefix) {
		return
	}
	for _, path := range w.paths {
		if within(name, path) {
			w.wrote(name)
			return
		}
	}
}

// name gives the name a file path holds.
func (w *watcher) name(at string) string {
	rel, err := filepath.Rel(w.root, at)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}
