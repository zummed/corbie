package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// VersionName is the name of the value that holds the layout's version.
const VersionName = "version"

// Version is the layout this Corbie knows.
const Version = 2

// Open opens the store an address names. The address alone says which kind
// it is: "memory:" is a new memory store, "s3://…" a bucket, "corbie://…" a
// directory another computer serves, which OpenServed opens, and anything else a
// directory.
func Open(address string) (Store, error) {
	switch {
	case address == "":
		return nil, fmt.Errorf("store: no address given")
	case address == "memory:":
		return NewMemory().As(Party{Kind: Owner}), nil
	case strings.HasPrefix(address, "s3://"):
		return openBucket(address)
	case IsServed(address):
		return nil, fmt.Errorf("store: a served directory is opened by a party, with its identity, and there is none here")
	}
	dir := strings.TrimPrefix(address, "file://")
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, dir[1:])
	}
	return OpenDirectory(dir)
}

// IsDirectory says whether an address names a directory, which keeps no party out
// of anything.
func IsDirectory(address string) bool {
	return address != "memory:" && !strings.HasPrefix(address, "s3://") && !IsServed(address)
}

// SetUp writes the layout's version into a store that has none.
func SetUp(ctx context.Context, s Store) error {
	if _, err := s.Write(ctx, VersionName, []byte(strconv.Itoa(Version)+"\n"), OnlyIfNew); err != nil {
		return err
	}
	return Check(ctx, s)
}

// Check refuses a store whose layout this Corbie does not know.
func Check(ctx context.Context, s Store) error {
	value, mark, err := s.Read(ctx, VersionName)
	if err != nil {
		return err
	}
	if mark == None {
		// A directory that is not there is said to be that, as a bucket that
		// cannot be reached is.
		// A read the store refuses counts as "not there", so a directory
		// this user may not read is said to be that, and a bucket, which
		// answers a refused read as it answers a name that holds nothing,
		// is said to be one or the other.
		switch f := s.(type) {
		case *directory:
			if _, err := os.Stat(f.root); gone(err) {
				return fmt.Errorf("store: the directory %q is not there (corbie init sets a store up)", f.root)
			}
			if file, err := openFile(f.path(VersionName)); errors.Is(err, fs.ErrPermission) {
				return fmt.Errorf("store: %w", err)
			} else if err == nil {
				file.Close()
			}
		case *bucket:
			return fmt.Errorf("store: this is not a Corbie store, or this key may not read it: the bucket gave no %s (corbie init sets a store up)", VersionName)
		}
		return fmt.Errorf("store: this is not a Corbie store: it has no %s (corbie init sets one up)", VersionName)
	}
	v := strings.TrimSpace(string(value))
	if n, err := strconv.Atoi(v); err == nil && n > 0 && strconv.Itoa(n) == v && n < Version {
		return fmt.Errorf("store: the store's layout is version %s, set up by an older Corbie, and this Corbie does not know it (it knows %d and reads nothing older; corbie init sets up a new store in an empty directory or bucket)", v, Version)
	}
	if v != strconv.Itoa(Version) {
		return fmt.Errorf("store: the store's layout is version %q, which this Corbie does not know (it knows %d)", v, Version)
	}
	return nil
}
