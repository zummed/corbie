package store

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
)

// local is a directory as a party on its own computer uses it: as it is
// while the directory has one member, and through the members' servers from
// the moment it has more. A change that is not in the members' log would be
// in one copy and not in the others.
type local struct {
	directory *directory
	self      string
	open      func(address string) (Store, error)

	mu     sync.Mutex
	served Store
}

// OpenLocal opens the directory at dir as a party on the computer that
// holds it. self is the address of this computer's member, as host:port,
// which the party asks first: it is the nearest, and names the leader.
// proof is the party's own, and keeper the key of the store's controller,
// which every member proves.
func OpenLocal(dir, self, party string, proof tls.Certificate, keeper ed25519.PublicKey) (Store, error) {
	opened, err := OpenDirectory(dir)
	if err != nil {
		return nil, err
	}
	return &local{directory: opened.(*directory), self: self, open: func(address string) (Store, error) {
		return OpenServed(address, party, proof, keeper)
	}}, nil
}

// now gives the store to use for one call.
func (l *local) now() (Store, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.served != nil {
		return l.served, nil
	}
	members, err := MembersOf(l.directory.root)
	if err != nil || len(members) == 0 {
		return l.directory, err
	}
	if at := slices.Index(members, l.self); at > 0 {
		members[0], members[at] = members[at], members[0]
	}
	l.served, err = l.open(ServedScheme + strings.Join(members, ","))
	return l.served, err
}

// changed makes a change, and makes it again through the members' servers
// where the directory was given members as it was made.
func (l *local) changed(change func(Store) (bool, error)) (bool, error) {
	s, err := l.now()
	if err != nil {
		return false, err
	}
	done, err := change(s)
	if errors.Is(err, errMembers) {
		if s, err = l.now(); err == nil {
			done, err = change(s)
		}
	}
	return done, err
}

func (l *local) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	s, err := l.now()
	if err != nil {
		return nil, None, err
	}
	return s.Read(ctx, name)
}

func (l *local) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	s, err := l.now()
	if err != nil {
		return nil, err
	}
	return s.ReadPart(ctx, name, from, length)
}

func (l *local) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	return l.changed(func(s Store) (bool, error) { return s.Write(ctx, name, value, how) })
}

// WriteFrom is not made again: the value has been read.
func (l *local) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	s, err := l.now()
	if err != nil {
		return false, err
	}
	return s.WriteFrom(ctx, name, r)
}

func (l *local) Copy(ctx context.Context, from, to string) (bool, error) {
	return l.changed(func(s Store) (bool, error) { return s.Copy(ctx, from, to) })
}

func (l *local) Delete(ctx context.Context, name string, how How) (bool, error) {
	return l.changed(func(s Store) (bool, error) { return s.Delete(ctx, name, how) })
}

func (l *local) List(ctx context.Context, under string) ([]Listed, error) {
	s, err := l.now()
	if err != nil {
		return nil, err
	}
	return s.List(ctx, under)
}

func (l *local) First(ctx context.Context, under string, n int) ([]Listed, error) {
	s, err := l.now()
	if err != nil {
		return nil, err
	}
	return s.First(ctx, under, n)
}

// Notice tells of what is written to this computer's copy, which with
// members is a moment after the leader answered it.
func (l *local) Notice(under string) (<-chan struct{}, func()) { return l.directory.Notice(under) }
