//go:build unix

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
	gate, err := f.openLock("gate", alone)
	if err == nil && gate >= 0 {
		err = flockCtx(ctx, gate, syscall.LOCK_EX)
	}
	if err != nil || gate < 0 {
		<-turn
		return func() {}, err
	}
	leave := func() {
		syscall.Close(gate)
		<-turn
	}
	how := syscall.LOCK_SH
	if alone {
		how = syscall.LOCK_EX
	}
	names, err := f.openLock("names", alone)
	if err == nil && names >= 0 {
		err = flockCtx(ctx, names, how)
	}
	if err != nil || names < 0 {
		leave()
		return func() {}, err
	}
	if !alone {
		leave()
		return func() { syscall.Close(names) }, nil
	}
	return func() {
		syscall.Close(names)
		leave()
	}, nil
}

// openLock opens one of the directory's two lock files, making it if it can.
// For a listing, a file that is not there and cannot be made gives -1 and no
// error.
func (f *directory) openLock(name string, must bool) (int, error) {
	path := filepath.Join(f.root, lockDir, name)
	fd, err := openFile(path, syscall.O_RDWR|syscall.O_CREAT)
	if gone(err) && must {
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			fd, err = openFile(path, syscall.O_RDWR|syscall.O_CREAT)
		}
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
		// A lock file that is there can be locked by one who can only read.
		fd, err = openFile(path, syscall.O_RDONLY)
	}
	if err != nil && !must {
		return -1, nil
	}
	if err != nil {
		return -1, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return fd, nil
}

func openFile(path string, how int) (int, error) {
	for {
		fd, err := syscall.Open(path, how|syscall.O_CLOEXEC, 0o600)
		if err != syscall.EINTR {
			return fd, err
		}
	}
}

// flockCtx takes a lock on fd, looking again every lockPause until ctx ends.
// On failure fd is closed.
func flockCtx(ctx context.Context, fd, how int) error {
	for {
		err := syscall.Flock(fd, how|syscall.LOCK_NB)
		switch err {
		case nil:
			return nil
		case syscall.EINTR:
			continue
		case syscall.EWOULDBLOCK:
		default:
			syscall.Close(fd)
			return err
		}
		t := time.NewTimer(lockPause)
		select {
		case <-ctx.Done():
			t.Stop()
			syscall.Close(fd)
			return ctx.Err()
		case <-t.C:
		}
	}
}

// syncDir has the system put a directory's names safely on disk.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
