// Package store is the one place Corbie keeps anything: values with names,
// three ways of writing one, and the directory, bucket and memory stores.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"
)

// A Store holds named values. Every call concerns one name and nothing is
// held between calls.
//
// "Not there", "already there" and "changed" are answers, never errors: a
// read gives the mark None, and a write or delete gives false. An error
// means the store could not be reached or refused the call.
type Store interface {
	// Read gives the value under name and the mark to write it back
	// only-if-unchanged. A name that holds nothing, or that this party may
	// not read, gives the mark None.
	Read(ctx context.Context, name string) ([]byte, Mark, error)

	// ReadPart gives up to length bytes of the value under name starting at
	// from: fewer where the value ends first, none where it is not there.
	ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error)

	// Write writes the value in the way how says, and says whether it did.
	Write(ctx context.Context, name string, value []byte, how How) (bool, error)

	// WriteFrom writes a value of any size only-if-new, reading it from r.
	// The value is under its name only once all of it is in.
	WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error)

	// Copy puts the value under from under to as well, only-if-new, without
	// its bytes passing through the caller.
	Copy(ctx context.Context, from, to string) (bool, error)

	// Delete removes the value under name: whatever is there (Overwrite) or
	// only-if-unchanged. Deleting a name that holds nothing is done.
	Delete(ctx context.Context, name string, how How) (bool, error)

	// List gives every name under the path, in the order names sort.
	List(ctx context.Context, under string) ([]string, error)

	// First gives the first names under the path, in the order names sort,
	// and no more than n of them (at most a thousand). It costs one look,
	// however many names there are: what is owed to time is read this way,
	// soonest first.
	First(ctx context.Context, under string, n int) ([]string, error)

	// Notice gives a channel that is sent to when this process writes or
	// deletes a name under the path, and a func that ends it. A store that
	// cannot tell gives a channel that is never sent to.
	Notice(under string) (<-chan struct{}, func())
}

// firstMost is the most names First gives.
const firstMost = 1000

// A Mark is what a read saw of a value, for a later only-if-unchanged.
type Mark string

// None is the mark of a name that holds nothing.
const None Mark = ""

// How is one of the three ways of writing.
type How struct {
	kind int
	mark Mark
}

const (
	overwrite = iota
	onlyIfNew
	onlyIfUnchanged
)

var (
	// Overwrite writes, or deletes, whatever is there.
	Overwrite = How{kind: overwrite}
	// OnlyIfNew writes only if nothing has the name yet.
	OnlyIfNew = How{kind: onlyIfNew}
)

// OnlyIfUnchanged writes, or deletes, only if the value is still the one
// that was read with mark.
func OnlyIfUnchanged(mark Mark) How { return How{kind: onlyIfUnchanged, mark: mark} }

func (h How) String() string {
	return [...]string{"overwrite", "only-if-new", "only-if-unchanged"}[h.kind]
}

var (
	// ErrUnreachable is a call the store did not answer.
	ErrUnreachable = errors.New("could not reach the store")
	// ErrRefused is a write, delete or listing the store would not allow.
	ErrRefused = errors.New("the store refused")
	// ErrUnknown is a conditional write whose answer was lost: it may or
	// may not have been written.
	ErrUnknown = errors.New("the store's answer was lost")
	// ErrName is a name no store takes.
	ErrName = errors.New("not a name a store can hold")
	// ErrNothing is a copy of a name that holds nothing.
	ErrNothing = errors.New("nothing is there to copy")
)

// CheckPart says whether part can be one part of a name: not empty, not
// starting with a dot, and holding no slash.
func CheckPart(part string) error {
	if part == "" || part[0] == '.' || strings.ContainsAny(part, "/\\") || strings.ContainsFunc(part, unicode.IsControl) {
		return fmt.Errorf("%w: part %q", ErrName, part)
	}
	return nil
}

// checkName says whether name is made of good parts.
func checkName(name string) error {
	for part := range strings.SplitSeq(name, "/") {
		if err := CheckPart(part); err != nil {
			return fmt.Errorf("%w in %q", err, name)
		}
	}
	return nil
}

// checkUnder is checkName for a path to list, which may be the whole store.
func checkUnder(under string) error {
	if under == "" {
		return nil
	}
	return checkName(under)
}

// markOf is the mark of a value in the stores that have no marks of their
// own: a value read again unchanged has the same mark.
func markOf(value []byte) Mark {
	sum := sha256.Sum256(value)
	return Mark(hex.EncodeToString(sum[:12]))
}

// within says whether name is under the path.
func within(name, under string) bool {
	return under == "" || strings.HasPrefix(name, under+"/")
}

// notices tells the parts of one process that share a store what was
// written.
type notices struct {
	mu   sync.Mutex
	subs map[chan struct{}]func(name string) bool
}

func (n *notices) notice(under string) (<-chan struct{}, func()) {
	return n.noticeIf(func(name string) bool { return within(name, under) })
}

// noticeIf gives a channel that is sent to when a name the func picks out
// is written or deleted, and a func that ends it.
func (n *notices) noticeIf(picks func(name string) bool) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	if n.subs == nil {
		n.subs = map[chan struct{}]func(string) bool{}
	}
	n.subs[ch] = picks
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		delete(n.subs, ch)
		n.mu.Unlock()
	}
}

func (n *notices) wrote(name string) {
	n.mu.Lock()
	for ch, picks := range n.subs {
		if picks(name) {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
	n.mu.Unlock()
}
