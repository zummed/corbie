package store

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// On Windows a file that is open can be neither replaced nor removed, and a
// file being replaced cannot be opened. Every reader holds a file open for
// a moment, and so do the programs that watch the disk, as a virus scanner
// and the search indexer do. A directory being removed takes no new file
// until it is gone. A call that meets a file held open, or a directory being
// removed, is tried again, for up to heldFor, as Go's own tools do.
// This is how these calls behave on Windows, not a layer that retries.
const heldFor = 2 * time.Second

// inUse says whether err is a file being held open by someone else.
func inUse(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == windows.ERROR_ACCESS_DENIED || errno == windows.ERROR_SHARING_VIOLATION)
}

// whileHeld calls do until it ends without meeting a held file, or heldFor
// has passed, waiting a little longer each time.
func whileHeld(do func() error) error {
	start, pause := time.Now(), time.Millisecond
	for {
		err := do()
		if !inUse(err) || time.Since(start)+pause >= heldFor {
			return err
		}
		time.Sleep(pause)
		pause += rand.N(pause)
	}
}

// readFile reads a file. A directory gives EISDIR, as on unix.
func readFile(path string) ([]byte, error) {
	var value []byte
	err := whileHeld(func() (err error) {
		value, err = os.ReadFile(path)
		return err
	})
	if err != nil && isDir(path) {
		return nil, &fs.PathError{Op: "read", Path: path, Err: syscall.EISDIR}
	}
	return value, err
}

// openFile opens a file to read. A directory gives EISDIR, as reading one
// does on unix.
func openFile(path string) (*os.File, error) {
	var file *os.File
	err := whileHeld(func() (err error) {
		file, err = os.Open(path)
		return err
	})
	if err == nil && isDir(path) {
		file.Close()
		return nil, &fs.PathError{Op: "read", Path: path, Err: syscall.EISDIR}
	}
	return file, err
}

// createTemp makes a new temporary file in dir.
func createTemp(dir string) (*os.File, error) {
	var file *os.File
	err := whileHeld(func() (err error) {
		file, err = os.CreateTemp(dir, tmpPrefix+"*")
		return err
	})
	return file, err
}

func link(from, to string) error {
	return whileHeld(func() error { return os.Link(from, to) })
}

func rename(from, to string) error {
	return whileHeld(func() error { return os.Rename(from, to) })
}

func remove(path string) error {
	return whileHeld(func() error { return os.Remove(path) })
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
