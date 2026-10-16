//go:build unix || windows

package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// lockPause is how long a wait for a lock another process holds sleeps
// between looks.
const lockPause = 100 * time.Microsecond

// turns holds, by directory, this process's turn at that directory's lock: a
// channel of one, so that any number of goroutines wait for the lock without
// each holding a thread, and every wait ends with its context.
var turns sync.Map

func turnAt(root string) chan struct{} {
	turn, _ := turns.LoadOrStore(root, make(chan struct{}, 1))
	return turn.(chan struct{})
}

// lock takes the directory's lock: shared for a listing, alone for a replace.
// The returned func lets it go. A lock held by a process that dies is let go
// by the system.
//
// There are two files. The system gives a shared lock to a newcomer while
// someone waits to hold the lock alone, so listings that overlap would keep
// every replace out. So everyone first takes the gate alone. A listing then
// takes the names lock shared and lets the gate go at once; a replace keeps
// the gate until it is done, which stops new listings starting while it
// waits for those in progress.
//
// A directory that can only be read, with no lock files, is listed without.
func (f *directory) lock(ctx context.Context, alone bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	turn := turnAt(f.root)
	select {
	case turn <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	gate, ok, err := f.openLock("gate", alone)
	if err == nil && ok {
		err = waitLock(ctx, gate, true)
	}
	if err != nil || !ok {
		<-turn
		return func() {}, err
	}
	leave := func() {
		closeLock(gate)
		<-turn
	}
	names, ok, err := f.openLock("names", alone)
	if err == nil && ok {
		err = waitLock(ctx, names, alone)
	}
	if err != nil || !ok {
		leave()
		return func() {}, err
	}
	if !alone {
		leave()
		return func() { closeLock(names) }, nil
	}
	return func() {
		closeLock(names)
		leave()
	}, nil
}

// openLock opens one of the directory's two lock files, making it if it can.
// For a listing, a file that is not there and cannot be made is not opened,
// and is no error.
func (f *directory) openLock(name string, must bool) (lockFile, bool, error) {
	path := filepath.Join(f.root, lockDir, name)
	file, err := openLockFile(path, true)
	if gone(err) && must {
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			file, err = openLockFile(path, true)
		}
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
		// A lock file that is there can be locked by one who can only read.
		file, err = openLockFile(path, false)
	}
	if err != nil && !must {
		return file, false, nil
	}
	if err != nil {
		return file, false, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return file, true, nil
}

// waitLock takes a lock on the file, alone or shared, looking again every
// lockPause until ctx ends. On failure the file is closed.
func waitLock(ctx context.Context, file lockFile, alone bool) error {
	for {
		taken, err := tryLock(file, alone)
		if taken {
			return nil
		}
		if err != nil {
			closeLock(file)
			return err
		}
		t := time.NewTimer(lockPause)
		select {
		case <-ctx.Done():
			t.Stop()
			closeLock(file)
			return ctx.Err()
		case <-t.C:
		}
	}
}
