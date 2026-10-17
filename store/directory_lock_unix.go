//go:build unix

package store

import (
	"os"
	"syscall"
)

// A lockFile is an open lock file, locked with flock.
type lockFile int

// openLockFile opens a lock file, for writing and made if missing, or only
// for reading.
func openLockFile(path string, write bool) (lockFile, error) {
	how := syscall.O_RDONLY
	if write {
		how = syscall.O_RDWR | syscall.O_CREAT
	}
	for {
		fd, err := syscall.Open(path, how|syscall.O_CLOEXEC, 0o600)
		if err != syscall.EINTR {
			return lockFile(fd), err
		}
	}
}

// tryLock takes the lock if no other process holds it in a way that keeps
// this one out, and says whether it did.
func tryLock(file lockFile, alone bool) (bool, error) {
	how := syscall.LOCK_SH
	if alone {
		how = syscall.LOCK_EX
	}
	for {
		switch err := syscall.Flock(int(file), how|syscall.LOCK_NB); err {
		case nil:
			return true, nil
		case syscall.EINTR:
		case syscall.EWOULDBLOCK:
			return false, nil
		default:
			return false, err
		}
	}
}

// closeLock closes the file, which lets go of its lock.
func closeLock(file lockFile) { syscall.Close(int(file)) }

// syncDir has the system put a directory's names safely on disk.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
