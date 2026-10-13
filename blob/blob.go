// Package blob is the values too large to sit inside the entry that
// mentions them: where they live, how they are protected, and how a
// reference stands in for one.
package blob

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/zummed/corbie/store"
)

// A Ref stands in for a blob wherever the blob is used. It says where the
// blob is, what type and size it is, and holds the key that opens it:
// holding the reference is the whole right to read the blob.
type Ref struct {
	At   string `json:"$blob"` // the blob's name in the store
	Type string `json:"type"`
	Size int64  `json:"size"`
	Key  string `json:"key,omitzero"`
}

// Definition is the type of a stored definition, the one kind of blob whose
// contents Corbie reads: it names its scripts and child definitions.
const Definition = "application/x-corbie-definition"

// StepLimit is the largest blob a step may read or write. Work on larger
// ones is done by jobs.
const StepLimit = 4 << 20

// A fault is something wrong with a blob or with the reference to it, as
// distinct from a store that cannot be reached.
type fault string

func (f fault) Error() string { return string(f) }

// Unusable says whether an error is a blob's own: the blob cannot be used,
// and asking again will not change that.
func Unusable(err error) bool {
	var f fault
	return errors.As(err, &f)
}

var (
	// ErrAltered is a blob whose bytes are not the ones that were written.
	ErrAltered error = fault("the blob has been altered")
	// ErrCutShort is a blob that ends before its reference says it does.
	ErrCutShort error = fault("the blob has been cut short")
	// ErrGone is a blob that is not in the store.
	ErrGone error = fault("the blob is not in the store")
	// ErrTooLarge is a blob over the limit of whoever is reading it.
	ErrTooLarge error = fault("the blob is too large for a step")
	// ErrKey is a reference whose key is not one that opens a blob.
	ErrKey error = fault("the reference holds no key that opens a blob")
	// ErrDefinition is a blob typed as a stored definition that is not one.
	ErrDefinition error = fault("the stored definition is not readable")
)

// String shows a reference without its key.
func (r Ref) String() string { return fmt.Sprintf("blob(%s, %d bytes)", r.Type, r.Size) }

// Format keeps the key out of every way of printing a reference.
func (r Ref) Format(f fmt.State, verb rune) { io.WriteString(f, r.String()) }

// Instance is the instance whose record the blob lives in.
func (r Ref) Instance() string {
	ctl, _, _ := strings.Cut(r.At, "/")
	return store.InstanceOf(ctl, r.At)
}

// A blob is stored as pieces, each encrypted and checked by itself, so a
// reader can fetch part of a large blob. A piece's place and whether it is
// the last are bound into it, so pieces cannot be swapped, taken from
// another blob, or dropped from the end.
const (
	pieceSize = 64 << 10
	tagSize   = 16
)

func pieces(size int64) int64 {
	if size == 0 {
		return 1
	}
	return (size + pieceSize - 1) / pieceSize
}

func newAEAD(key string) (cipher.AEAD, error) {
	raw, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func nonce(piece int64, last bool) []byte {
	n := make([]byte, 12)
	binary.BigEndian.PutUint64(n, uint64(piece))
	if last {
		n[11] = 1
	}
	return n
}

// NewKey makes a key for a new blob.
func NewKey() string {
	key := make([]byte, 32)
	rand.Read(key)
	return base64.RawURLEncoding.EncodeToString(key)
}

// NewName makes a name for a new blob. Like an instance's, it carries the
// time it was made: how old a blob is is read from its name, never from a
// file's times.
func NewName() string {
	name := make([]byte, 10)
	rand.Read(name)
	return store.Stamp(time.Now()) + "-" + hex.EncodeToString(name)
}

// Made is when a blob was named. A blob whose name carries no time, as one
// a step wrote has none, gives the zero time.
func Made(at string) time.Time {
	name := at[strings.LastIndexByte(at, '/')+1:]
	if len(name) < len(store.Stamp(time.Time{})) {
		return time.Time{}
	}
	made, err := time.Parse("20060102T150405.000Z", name[:len(store.Stamp(time.Time{}))])
	if err != nil {
		return time.Time{}
	}
	return made
}

// encrypter turns a reader of a blob's contents into a reader of what is
// stored, counting the contents as it goes.
type encrypter struct {
	src   io.Reader
	aead  cipher.AEAD
	piece int64
	size  int64
	out   bytes.Buffer
	next  []byte // one piece read ahead, to know which is the last
	done  bool
}

func (e *encrypter) Read(p []byte) (int, error) {
	for e.out.Len() == 0 && !e.done {
		if e.next == nil {
			e.next = make([]byte, 0, pieceSize)
			if err := e.fill(&e.next); err != nil {
				return 0, err
			}
		}
		cur := e.next
		e.next = make([]byte, 0, pieceSize)
		if len(cur) == pieceSize {
			if err := e.fill(&e.next); err != nil {
				return 0, err
			}
		}
		last := len(e.next) == 0
		e.size += int64(len(cur))
		e.out.Write(e.aead.Seal(nil, nonce(e.piece, last), cur, nil))
		e.piece++
		e.done = last
	}
	if e.out.Len() == 0 {
		return 0, io.EOF
	}
	return e.out.Read(p)
}

func (e *encrypter) fill(buf *[]byte) error {
	n, err := io.ReadFull(e.src, (*buf)[:pieceSize])
	*buf = (*buf)[:n]
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return nil
	}
	return err
}

// Write stores a new blob under the name at, reading its contents from r,
// and gives its reference. The blob is complete in the store before the
// reference exists.
func Write(ctx context.Context, s store.Store, at, typ, key string, r io.Reader) (Ref, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return Ref{}, err
	}
	enc := &encrypter{src: r, aead: aead}
	if _, err := s.WriteFrom(ctx, at, enc); err != nil {
		return Ref{}, err
	}
	if !enc.done {
		// The name was taken and the store read nothing. A blob's name is
		// random or follows from its contents: what is there is this blob.
		if _, err := io.Copy(io.Discard, enc); err != nil {
			return Ref{}, err
		}
	}
	return Ref{At: at, Type: typ, Size: enc.size, Key: key}, nil
}

// gone gives the store's answer for a name that holds nothing, or that no
// store can hold, as the blob's: a reference with such a name names no blob.
func gone(err error) error {
	if errors.Is(err, store.ErrNothing) || errors.Is(err, store.ErrName) {
		return ErrGone
	}
	return err
}

// A Reader reads a blob, any part of it, checking each piece before giving
// any byte of it.
type Reader struct {
	ctx  context.Context
	s    store.Store
	ref  Ref
	aead cipher.AEAD
	off  int64
}

// Open gives a reader of the blob a reference stands for.
func Open(ctx context.Context, s store.Store, ref Ref) (*Reader, error) {
	aead, err := newAEAD(ref.Key)
	if err != nil {
		return nil, err
	}
	if ref.Size < 0 {
		return nil, ErrAltered
	}
	return &Reader{ctx: ctx, s: s, ref: ref, aead: aead}, nil
}

// ReadAt reads from a place in the blob.
func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.ref.Size {
		return 0, io.EOF
	}
	want := min(int64(len(p)), r.ref.Size-off)
	first, last := off/pieceSize, (off+want-1)/pieceSize
	total := pieces(r.ref.Size)
	stored, err := r.s.ReadPart(r.ctx, r.ref.At, first*(pieceSize+tagSize), (last-first+1)*(pieceSize+tagSize))
	if err != nil {
		return 0, gone(err)
	}
	if stored == nil {
		return 0, ErrGone
	}
	n := 0
	for piece := first; piece <= last; piece++ {
		plainLen := int64(pieceSize)
		if piece == total-1 {
			plainLen = r.ref.Size - piece*pieceSize
		}
		start := (piece - first) * (pieceSize + tagSize)
		end := start + plainLen + tagSize
		if end > int64(len(stored)) {
			return 0, ErrCutShort
		}
		plain, err := r.aead.Open(nil, nonce(piece, piece == total-1), stored[start:end], nil)
		if err != nil {
			return 0, ErrAltered
		}
		from := max(off-piece*pieceSize, 0)
		n += copy(p[n:want], plain[from:])
	}
	if int64(n) < int64(len(p)) {
		return n, io.EOF
	}
	return n, nil
}

func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.ReadAt(p, r.off)
	r.off += int64(n)
	if n > 0 && err == io.EOF {
		err = nil
	}
	return n, err
}

// Size is the size of the blob's contents.
func (r *Reader) Size() int64 { return r.ref.Size }

// ReadAll reads the whole of a blob no larger than limit.
func ReadAll(ctx context.Context, s store.Store, ref Ref, limit int64) ([]byte, error) {
	if ref.Size > limit {
		return nil, fmt.Errorf("%w (%d bytes)", ErrTooLarge, ref.Size)
	}
	r, err := Open(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	if ref.Size == 0 {
		// Still check the one empty piece.
		stored, err := s.ReadPart(ctx, ref.At, 0, tagSize)
		if err != nil {
			return nil, gone(err)
		}
		if stored == nil {
			return nil, ErrGone
		}
		if _, err := r.aead.Open(nil, nonce(0, true), stored, nil); err != nil {
			return nil, ErrAltered
		}
		return []byte{}, nil
	}
	contents := make([]byte, ref.Size)
	if _, err := io.ReadFull(r, contents); err != nil {
		return nil, err
	}
	return contents, nil
}

// Copy puts a blob into another instance's record under the same name,
// without its bytes passing through, and gives the copy's reference. Done
// twice, it is done once.
func Copy(ctx context.Context, s store.Store, ref Ref, ctl, instance string) (Ref, error) {
	if ref.Instance() == instance {
		return ref, nil
	}
	name := path.Base(ref.At)
	if ref.At != store.Blob(ctl, ref.Instance(), name) {
		return Ref{}, ErrGone
	}
	to := store.Blob(ctl, instance, name)
	if _, err := s.Copy(ctx, ref.At, to); err != nil {
		return Ref{}, gone(err)
	}
	ref.At = to
	return ref, nil
}
