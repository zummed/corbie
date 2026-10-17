//go:build !windows

package identity

import (
	"fmt"
	"io/fs"
	"os"
)

// makePrivate lets the file's owner alone read and write it.
func makePrivate(path string) error { return os.Chmod(path, 0o600) }

// checkPrivate refuses a file that anyone but its owner can read.
func checkPrivate(path string, info fs.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("the identity file %q can be read by others (mode %o); its key is no longer its owner's alone. If that is not so, run: chmod 600 %s",
			path, info.Mode().Perm(), path)
	}
	return nil
}
