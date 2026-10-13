package plugin

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"strconv"
)

// Contents gives the bytes of something a script hands a plugin as
// contents: text, bytes, a blob, or any other data, which is written as
// JSON. The second result is the type such contents have.
func (j *Job) Contents(v any) ([]byte, string, error) {
	switch v := v.(type) {
	case nil:
		return nil, "", nil
	case string:
		return []byte(v), "text/plain", nil
	case Bytes:
		return v, "application/octet-stream", nil
	case *Blob:
		data, err := v.All(j)
		return data, v.Type, err
	case *Secret:
		return nil, "", Errorf(BadInput, "a secret is not contents: it can go only where a plugin's page says a secret may")
	}
	data, err := JSON(v)
	return data, "application/json", err
}

// JSON writes data as JSON. A handle anywhere in it is refused, as it is
// wherever a handle would be written out as data.
func JSON(v any) ([]byte, error) {
	data, err := json.Marshal(Plain(v), json.Deterministic(true))
	if err != nil {
		return nil, Errorf(BadInput, "this cannot be written as data: %v", err)
	}
	return data, nil
}

// TextOf gives a number or a true or false as the text it is written as,
// and anything else as it is. It is for a place that takes text or a
// handle, where a script may as well write a number.
func TextOf(v any) any {
	switch v := v.(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return v
}

// Reading gives something a script hands a plugin as contents to be read
// from start to end, with its size and its type. A blob is read a part at
// a time, however large it is, and never held whole.
func (j *Job) Reading(v any) (io.Reader, int64, string, error) {
	if b, ok := v.(*Blob); ok {
		return b.Reader(j), b.Size, b.Type, nil
	}
	data, typ, err := j.Contents(v)
	return bytes.NewReader(data), int64(len(data)), typ, err
}

// Source gives contents to be read at any place in them, with their size:
// a blob where it lies, anything else from memory.
func (j *Job) Source(v any) (io.ReaderAt, int64, error) {
	if b, ok := v.(*Blob); ok {
		return b.Source(j), b.Size, nil
	}
	data, _, err := j.Contents(v)
	return bytes.NewReader(data), int64(len(data)), err
}

// Give hands a script what a plugin produces, in the form the script asked
// for. fill writes it. As a blob it goes into a file in the job's scratch
// directory as it is written, whatever its size, and is sent to the store only
// if it leaves the job. In any other form it is held whole, and one too
// large to hold fails with the advice to ask for a blob.
func (j *Job) Give(form, typ, what string, fill func(w io.Writer) error) (any, error) {
	if form == "blob" {
		file, err := os.CreateTemp(j.Scratch, "blob-*")
		if err != nil {
			return nil, fileError(err, "the job's scratch directory")
		}
		j.OnEnd(func() { file.Close() })
		if err := fill(file); err != nil {
			var known *Error
			if errors.As(err, &known) {
				return nil, err
			}
			return nil, fileError(err, "the job's scratch directory")
		}
		info, err := file.Stat()
		if err != nil {
			return nil, fileError(err, "the job's scratch directory")
		}
		if typ == "" {
			typ = "application/octet-stream"
		}
		return &Blob{Type: typ, Size: info.Size(), file: file}, nil
	}
	held := &Whole{}
	err := fill(held)
	if held.Over {
		return nil, TooLarge(what)
	}
	if err != nil {
		return nil, err
	}
	return j.As(held.data, form, typ)
}

// TooLarge is the one failure of something too large to hand a script
// whole. It says the way out, which is the same everywhere.
func TooLarge(what string) error {
	return Errorf(BadInput, "%s is larger than %d MiB, too large to hold whole: ask for it as a blob, with { as: \"blob\" }", what, MaxWhole>>20)
}

// Whole keeps what is written to it, up to the most that is held whole.
// Past that it takes no more, and says so.
type Whole struct {
	data []byte
	Over bool
}

func (h *Whole) Write(p []byte) (int, error) {
	if len(h.data)+len(p) > MaxWhole {
		h.Over = true
		return 0, io.ErrShortWrite
	}
	h.data = append(h.data, p...)
	return len(p), nil
}

// Text is what was kept.
func (h *Whole) Text() string { return string(h.data) }

// Draining passes on what a program prints, and never holds the program
// up: once what it writes to takes no more, the rest is let go, and the
// failure kept.
type Draining struct {
	To  io.Writer
	Err error
}

func (d *Draining) Write(p []byte) (int, error) {
	if d.Err == nil {
		_, d.Err = d.To.Write(p)
	}
	return len(p), nil
}

// Plain gives data with every blob in it as its reference, which a blob
// has only once it is in the store, so that it can be written as JSON.
func Plain(v any) any {
	switch v := v.(type) {
	case *Blob:
		if v.Ref != nil {
			return v.Ref.Value()
		}
		return map[string]any{"type": v.Type, "size": v.Size}
	case Bytes:
		return string(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = Plain(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = Plain(item)
		}
		return out
	}
	return v
}

// As gives bytes to a script in the form it asked for: "text", "bytes",
// "json" or "blob". A byte order mark at the start of text or data is
// dropped.
func (j *Job) As(data []byte, form, typ string) (any, error) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	switch form {
	case "text":
		return string(bytes.TrimPrefix(data, bom)), nil
	case "bytes":
		return Bytes(data), nil
	case "blob":
		return NewBlob(typ, data), nil
	case "json":
		var v any
		if err := json.Unmarshal(bytes.TrimPrefix(data, bom), &v); err != nil {
			return nil, Errorf(BadInput, "it is not JSON: %v", err)
		}
		return v, nil
	}
	return nil, Errorf(BadInput, "as: is \"text\", \"bytes\", \"json\" or \"blob\", not %q", form)
}
