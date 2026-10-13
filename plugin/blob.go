package plugin

import (
	"bytes"
	"io"
	"os"

	"github.com/zummed/corbie/blob"
)

// A Blob is a blob as a plugin and a script hold it: a type, a size, and
// contents that can be read. One a job makes stays with the job until its
// reference leaves, in an emit or in the result; only then is it sent to
// the store.
type Blob struct {
	Type string
	Size int64
	// Ref is where the blob is in the store, once it is there.
	Ref *blob.Ref

	data []byte   // contents held by the job
	file *os.File // or a file read where it lies, held open until the job ends
}

// NewBlob makes a blob of contents the job holds.
func NewBlob(typ string, contents []byte) *Blob {
	if typ == "" {
		typ = blob.Bytes
	}
	return &Blob{Type: typ, Size: int64(len(contents)), data: contents}
}

// StoredBlob is a blob that is in the store.
func StoredBlob(ref blob.Ref) *Blob {
	return &Blob{Type: ref.Type, Size: ref.Size, Ref: &ref}
}

// TypeOf is the type contents have that come from a file of a name.
func TypeOf(name string) string { return blob.TypeOf(name) }

// FileBlob makes a blob of a file, read where it lies and not copied. The
// file is held open until the job ends, and is read to the size it had when
// it was opened.
func (j *Job) FileBlob(path, typ string) (*Blob, error) {
	real, err := j.Path(path, false)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(real)
	if err != nil {
		return nil, fileError(err, path)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, Errorf(BadInput, "%s is not a file", path)
	}
	j.OnEnd(func() { file.Close() })
	if typ == "" {
		typ = TypeOf(path)
	}
	return &Blob{Type: typ, Size: info.Size(), file: file}, nil
}

// Read reads part of a blob's contents.
func (b *Blob) Read(j *Job, from, length int64) ([]byte, error) {
	if from < 0 || length < 0 {
		return nil, Errorf(BadInput, "a part of a blob starts at nothing less than 0")
	}
	if from >= b.Size {
		return []byte{}, nil
	}
	length = min(length, b.Size-from)
	switch {
	case b.data != nil:
		return b.data[from : from+length], nil
	case b.file != nil:
		part := make([]byte, length)
		if n, err := b.file.ReadAt(part, from); int64(n) != length {
			return nil, Errorf(Failed, "the file changed after it was opened: %v", err)
		}
		return part, nil
	case b.Ref != nil:
		return j.Channel.ReadBlob(*b.Ref, from, length)
	}
	return []byte{}, nil
}

// MaxWhole is the most of anything that is handed to a script whole, and
// so held in the job's memory: a file's contents, a body, what a program
// printed, something unpacked, a blob read at once, one item of a blob. It
// is the one such size there is. Anything larger is had as a blob, which
// is read a part at a time and written as it arrives.
const MaxWhole = 64 << 20

// partSize is how much of a blob is fetched at a time when it is read
// through: large enough that reading a blob in a bucket from end to end is
// a few requests, small enough to hold.
const partSize = 4 << 20

// All reads the whole of a blob, which may be no larger than what is held
// whole.
func (b *Blob) All(j *Job) ([]byte, error) {
	if b.Size > MaxWhole {
		return nil, Errorf(BadInput, "the blob is %d bytes, too large to hold whole: read a part of it, walk its items, or write it to a file", b.Size)
	}
	return b.Read(j, 0, b.Size)
}

// parts reads a blob where it lies, a part at a time. One part is held,
// and a read that falls inside it fetches nothing.
type parts struct {
	b    *Blob
	j    *Job
	from int64 // where the held part starts
	held []byte
}

func (p *parts) ReadAt(buf []byte, at int64) (int, error) {
	n := 0
	for n < len(buf) {
		if at >= p.b.Size {
			return n, io.EOF
		}
		if at < p.from || at >= p.from+int64(len(p.held)) {
			from := at - at%partSize
			held, err := p.b.Read(p.j, from, partSize)
			if err != nil {
				return n, err
			}
			if len(held) == 0 {
				return n, io.ErrUnexpectedEOF
			}
			p.from, p.held = from, held
		}
		c := copy(buf[n:], p.held[at-p.from:])
		n, at = n+c, at+int64(c)
	}
	return n, nil
}

// Source gives the blob to be read at any place in it, a part at a time.
func (b *Blob) Source(j *Job) io.ReaderAt { return &parts{b: b, j: j} }

// Reader gives the blob to be read from start to end, a part at a time.
func (b *Blob) Reader(j *Job) io.Reader { return io.NewSectionReader(b.Source(j), 0, b.Size) }

// Store sends the blob to the store, if it is not there yet, and gives its
// reference.
func (b *Blob) Store(j *Job) (blob.Ref, error) {
	if b.Ref != nil {
		return *b.Ref, nil
	}
	var contents io.Reader = bytes.NewReader(b.data)
	if b.file != nil {
		contents = io.NewSectionReader(b.file, 0, b.Size)
	}
	ref, err := j.Channel.WriteBlob(b.Type, b.Size, contents)
	if err != nil {
		return blob.Ref{}, err
	}
	b.Ref = &ref
	return ref, nil
}

// fileError gives a file system's error the one shape.
func fileError(err error, path string) error {
	switch {
	case os.IsNotExist(err):
		return Errorf(NotFound, "%s is not there", path)
	case os.IsPermission(err):
		return Errorf(NotPermitted, "the system does not let the runner reach %s", path)
	}
	return Errorf(Failed, "%s: %v", path, err)
}

// FileError gives a file system's error the one shape, for a plugin.
func FileError(err error, path string) error { return fileError(err, path) }
