//go:build unix

package store

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func Test_store_directory_list_when_read_only(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	for _, withLocks := range []bool{false, true} {
		s, dir := newDirectory(t)
		must[bool](t)(s.Write(ctx, "r/a", []byte("x"), OnlyIfNew))
		if withLocks {
			must[bool](t)(s.Write(ctx, "r/b", []byte("x"), Overwrite))
			s.Delete(ctx, "r/b", Overwrite)
		}
		var dirs []string
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if d.IsDir() {
				dirs = append(dirs, path)
			} else {
				os.Chmod(path, 0o400)
			}
			return nil
		})
		for _, d := range dirs {
			os.Chmod(d, 0o500)
		}
		t.Cleanup(func() {
			for _, d := range dirs {
				os.Chmod(d, 0o700)
			}
		})
		names, err := Names(s.List(ctx, "r"))
		if err != nil || !slices.Equal(names, []string{"r/a"}) {
			t.Fatalf("listing a directory that can only be read: %v %v", names, err)
		}
		if v, _ := read(t, s, "r/a"); v != "x" {
			t.Fatal("reading a directory that can only be read")
		}
	}
}

// A directory store this user may not read is reported as that, and not as
// a directory that holds no store.
func TestCheckSaysADirectoryItMayNotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetUp(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, VersionName), 0); err != nil {
		t.Fatal(err)
	}
	err = Check(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "not a Corbie store") {
		t.Fatalf("a version this user may not read gave: %v", err)
	}
}
