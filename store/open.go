package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VersionName is the name of the value that holds the layout's version.
const VersionName = "version"

// Version is the layout this Corbie knows.
const Version = "1"

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
	if _, err := s.Write(ctx, VersionName, []byte(Version+"\n"), OnlyIfNew); err != nil {
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
		if f, ok := s.(*directory); ok {
			if _, err := os.Stat(f.root); gone(err) {
				return fmt.Errorf("store: the directory %s is not there (corbie init sets a store up)", f.root)
			}
		}
		return fmt.Errorf("store: this is not a Corbie store: it has no %s (corbie init sets one up)", VersionName)
	}
	if v := strings.TrimSpace(string(value)); v != Version {
		return fmt.Errorf("store: the store's layout is version %q, which this Corbie does not know (it knows %s)", v, Version)
	}
	return nil
}
