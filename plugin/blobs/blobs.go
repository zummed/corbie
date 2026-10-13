// Package blobs is the plugin that makes and reads blobs.
package blobs

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"strings"

	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "blobs", Page: page, Calls: map[string]*plugin.Call{
		"make": {Options: []string{"type"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, typ, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			if typ, err = opt.Text("type", typ); err != nil {
				return nil, err
			}
			return plugin.NewBlob(typ, data), nil
		}},
		"file": {Options: []string{"type"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			path, err := plugin.Text(first, "the file's path")
			if err != nil {
				return nil, err
			}
			typ, err := opt.Text("type", "")
			if err != nil {
				return nil, err
			}
			return job.FileBlob(path, typ)
		}},
		"read": {Options: []string{"as", "from", "length"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			b, err := one(first)
			if err != nil {
				return nil, err
			}
			form, err := opt.Text("as", "text")
			if err != nil {
				return nil, err
			}
			from, err := opt.Int("from", 0)
			if err != nil {
				return nil, err
			}
			length, err := opt.Int("length", int(b.Size))
			if err != nil {
				return nil, err
			}
			if int64(length) > maxWhole {
				return nil, plugin.Errorf(plugin.BadInput, "that is %d bytes, too much to hold at once: read it a part at a time with from and length", length)
			}
			data, err := b.Read(job, int64(from), int64(length))
			if err != nil {
				return nil, err
			}
			return job.As(data, form, b.Type)
		}},
		"count": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			next, err := items(job, first)
			if err != nil {
				return nil, err
			}
			n := 0
			for {
				if _, more, err := next(); err != nil || !more {
					return float64(n), err
				}
				n++
			}
		}},
		"items": {Options: []string{"from", "count"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			next, err := items(job, first)
			if err != nil {
				return nil, err
			}
			from, err := opt.Int("from", 0)
			if err != nil {
				return nil, err
			}
			count, err := opt.Int("count", -1)
			if err != nil {
				return nil, err
			}
			// What is given is walked, an item at a time, and holds no more
			// of the blob than the part it is reading.
			at, given := 0, 0
			over := map[string]any{"done": true}
			return &plugin.Object{Calls: map[string]*plugin.Call{"next": {Do: func(*plugin.Job, any, plugin.Options) (any, error) {
				for count < 0 || given < count {
					item, more, err := next()
					if err != nil {
						return nil, err
					}
					if !more {
						break
					}
					if at++; at > from {
						given++
						return map[string]any{"done": false, "value": item}, nil
					}
				}
				count = 0
				return over, nil
			}}}}, nil
		}},
	}})
}

// maxWhole is the most of a blob that is held whole.
const maxWhole = plugin.MaxWhole

func one(first any) (*plugin.Blob, error) {
	b, ok := first.(*plugin.Blob)
	if !ok {
		return nil, plugin.Errorf(plugin.BadInput, "the call takes a blob first")
	}
	return b, nil
}

// maxItem is the most one item may be. An item is handed to the script
// whole, so it is the most of anything that is.
const maxItem = plugin.MaxWhole

// metered reads a blob for its items, and fails when one item turns out
// larger than an item may be: it would otherwise be held whole, however
// large.
type metered struct {
	from  io.Reader
	since int // bytes read since the last item was given
}

func (m *metered) Read(p []byte) (int, error) {
	if m.since > maxItem {
		return 0, plugin.Errorf(plugin.BadInput, "an item of the blob is larger than %d MiB, too large to be an item", maxItem>>20)
	}
	// A little is read at a time, so that what has been read since the
	// last item is close to the size of the next.
	n, err := m.from.Read(p[:min(len(p), 64<<10)])
	m.since += n
	return n, err
}

// items gives a way to walk the items of a blob, which depend on its type:
// the lines of a text, the rows of a table, the entries of a list. Each
// call gives the next item, or says there are no more. The blob is read
// forward a part at a time and never held whole.
func items(job *plugin.Job, first any) (next func() (any, bool, error), err error) {
	b, err := one(first)
	if err != nil {
		return nil, err
	}
	meter := &metered{from: b.Reader(job)}
	in := bufio.NewReaderSize(meter, 64<<10)
	if start, _ := in.Peek(3); bytes.Equal(start, []byte{0xEF, 0xBB, 0xBF}) {
		in.Discard(3)
	}
	// line gives the next line, without its end.
	line := func() (string, bool, error) {
		text, err := in.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", false, err
		}
		if err == io.EOF && text == "" {
			return "", false, nil
		}
		return strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r"), true, nil
	}
	switch {
	case b.Type == "application/json":
		dec := jsontext.NewDecoder(in)
		if dec.PeekKind() != '[' {
			// Data that is not a list is one item.
			done := false
			next = func() (any, bool, error) {
				if done {
					return nil, false, nil
				}
				done = true
				var v any
				if err := json.UnmarshalDecode(dec, &v); err != nil {
					return nil, false, notJSON(err)
				}
				return v, true, nil
			}
			break
		}
		dec.ReadToken()
		next = func() (any, bool, error) {
			if kind := dec.PeekKind(); kind == ']' || kind == 0 {
				if _, err := dec.ReadToken(); err != nil && err != io.EOF {
					return nil, false, notJSON(err)
				}
				return nil, false, nil
			}
			var v any
			if err := json.UnmarshalDecode(dec, &v); err != nil {
				return nil, false, notJSON(err)
			}
			return v, true, nil
		}
	case b.Type == "application/jsonl":
		next = func() (any, bool, error) {
			for {
				text, more, err := line()
				if err != nil || !more {
					return nil, false, err
				}
				if strings.TrimSpace(text) == "" {
					continue
				}
				var v any
				if err := json.Unmarshal([]byte(text), &v); err != nil {
					return nil, false, plugin.Errorf(plugin.BadInput, "a line of the blob is not JSON: %v", err)
				}
				return v, true, nil
			}
		}
	case b.Type == "text/csv" || b.Type == "text/tab-separated-values":
		r := csv.NewReader(in)
		if b.Type != "text/csv" {
			r.Comma = '\t'
		}
		var header []string
		next = func() (any, bool, error) {
			if header == nil {
				if header, err = r.Read(); err == io.EOF {
					return nil, false, nil
				}
			}
			// A row with another number of fields than the header fails
			// the read: quietly padded or cut, it puts values under the
			// wrong names.
			var record []string
			if err == nil {
				record, err = r.Read()
			}
			if err == io.EOF {
				return nil, false, nil
			}
			if err != nil {
				var known *plugin.Error
				if errors.As(err, &known) {
					return nil, false, err
				}
				return nil, false, plugin.Errorf(plugin.BadInput, "the table cannot be read: %v", err)
			}
			row := map[string]any{}
			for i, name := range header {
				row[name] = record[i]
			}
			return row, true, nil
		}
	case strings.HasPrefix(b.Type, "text/"):
		next = func() (any, bool, error) {
			text, more, err := line()
			return text, more, err
		}
	default:
		return nil, plugin.Errorf(plugin.BadInput, "a blob of type %s has no items: text has lines, a table rows, a list entries", b.Type)
	}
	walk := next
	return func() (any, bool, error) {
		item, more, err := walk()
		meter.since = 0
		return item, more, err
	}, nil
}

func notJSON(err error) error {
	var known *plugin.Error
	if errors.As(err, &known) {
		return err
	}
	return plugin.Errorf(plugin.BadInput, "the blob is not JSON: %v", err)
}

const page = `blobs: this plugin makes and reads blobs.

A blob is a large value: a file, a long text, an image. In a script it is an
object with type and size. A blob that a job makes is stored only when the
script emits or returns it.

  blobs.make(contents, { type })           a blob of text, bytes or data
  blobs.file(path, { type })               a blob of a file, read only when the blob is read
  blobs.read(blob, { as, from, length })   the contents, or a part of them
  blobs.count(blob)                        the number of items
  blobs.items(blob, { from, count })       an iterator over the items, reading one at a time

Options:
  type     the media type; default text/plain for text, application/octet-stream for bytes,
           application/json for data, and by its extension for a file
  as       "text", "bytes" or "json"; default "text"
  from     the first byte to read, or the number of items to skip; default 0
  length   the number of bytes to read; default to the end
  count    the number of items; default all

A blob's items depend on its type.

  text/csv, text/tab-separated-values   the rows after the header, each a map from the header's names
  application/json                      the entries of a list; any other value is one item
  application/jsonl                     the values, one on each line
  any other text type                   the lines
  any other type                        none

Sizes:
  a blob                         any size
  a result that is not a blob    at most 64 MiB, held in memory: one read, or one item
  a result with { as: "blob" }   any size, written into the job's scratch directory as it arrives

fs.write, exec.run, compress and http read a blob a part at a time.
Array.from(blobs.items(blob)) keeps every item in memory, so give it from and
count. Skipping with from reads the skipped items, and blobs.count reads the
whole blob.

Errors:
  bad_input       an argument that is not a blob, a type with no items, contents that do not
                  parse as the type, or a read or an item over 64 MiB
  not_found       blobs.file: the file does not exist
  not_permitted   blobs.file: the operating system refuses the runner

Needs fs.read:path for the file that blobs.file reads, and nothing otherwise.

This script totals a column of a CSV file, one row at a time.

    allow: [fs.read:/srv/reports/**]

    const table = blobs.file("/srv/reports/2026/q3.csv", { type: "text/csv" })
    let total = 0
    for (const row of blobs.items(table)) total += Number(row.amount)
    return total                                         // gives 42

This script makes a blob, counts its lines and returns it.

    const report = blobs.make("line one\nline two\n", { type: "text/plain" })
    blobs.count(report)                                  // gives 2
    Array.from(blobs.items(report, { from: 1 }))         // gives ["line two"]
    return { report }
`
