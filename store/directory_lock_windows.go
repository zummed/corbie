package store

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// A lockFile is an open lock file, locked with LockFileEx on its first byte.
type lockFile syscall.Handle

// openLockFile opens a lock file, for writing and made if missing, or only
// for reading. No program the process starts is given the handle.
func openLockFile(path string, write bool) (lockFile, error) {
	how := syscall.O_RDONLY
	if write {
		how = syscall.O_RDWR | syscall.O_CREAT
	}
	h, err := syscall.Open(path, how|syscall.O_CLOEXEC, 0o600)
	return lockFile(h), err
}

// tryLock takes the lock if no other process holds it in a way that keeps
// this one out, and says whether it did.
func tryLock(file lockFile, alone bool) (bool, error) {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if alone {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	err := windows.LockFileEx(windows.Handle(file), flags, 0, 1, 0, &windows.Overlapped{})
	if err == windows.ERROR_LOCK_VIOLATION {
		return false, nil
	}
	return err == nil, err
}

// closeLock lets go of the lock and closes the file. The lock is let go
// first: the system lets go of a closed file's lock only some time later.
func closeLock(file lockFile) {
	windows.UnlockFileEx(windows.Handle(file), 0, 1, 0, &windows.Overlapped{})
	windows.CloseHandle(windows.Handle(file))
}

// syncDir does nothing: Windows cannot sync a directory, and NTFS keeps
// its names in a journal of its own.
func syncDir(dir string) error { return nil }
