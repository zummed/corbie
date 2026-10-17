//go:build !windows

package identity

import (
	"os"
	"testing"
)

// letOthersRead checks that only the file's owner can read and write it,
// then lets everyone read it.
func letOthersRead(t *testing.T, path string) {
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the identity file has mode %o", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
}
