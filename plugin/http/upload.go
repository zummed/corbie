package http

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"slices"
	"strings"

	"github.com/zummed/corbie/plugin"
)

// upload builds an upload form (multipart/form-data) from a map of fields,
// in the order of their names. A field is text, or a file: bytes, a blob,
// or { data, name, type }. A blob in it is sent as it is read, so a form
// has no largest size, and its length is known before it is sent.
func upload(job *plugin.Job, fields map[string]any, reveal func(any) (string, error)) (io.Reader, int64, string, error) {
	var raw [15]byte
	rand.Read(raw[:])
	boundary := "corbie-" + hex.EncodeToString(raw[:])
	var parts []io.Reader
	size := int64(0)
	add := func(head string, data io.Reader, length int64) {
		parts = append(parts, strings.NewReader(head), data, strings.NewReader("\r\n"))
		size += int64(len(head)) + length + 2
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		head := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"" + quoted(name) + "\""
		value, file, typ := fields[name], "", ""
		if m, ok := value.(map[string]any); ok {
			for key := range m {
				if key != "data" && key != "name" && key != "type" {
					return nil, 0, "", plugin.Errorf(plugin.BadInput, "upload: the file %q has the field %q, and a file is { data, name, type }", name, key)
				}
			}
			value = m["data"]
			file, _ = m["name"].(string)
			typ, _ = m["type"].(string)
			if file == "" {
				return nil, 0, "", plugin.Errorf(plugin.BadInput, "upload: the file %q has no name", name)
			}
		}
		var data io.Reader
		var length int64
		switch v := value.(type) {
		case plugin.Bytes:
			data, length = bytes.NewReader(v), int64(len(v))
		case *plugin.Blob:
			data, length = v.Reader(job), v.Size
			if typ == "" {
				typ = v.Type
			}
		default:
			if file != "" {
				text, _, err := job.Contents(value)
				if err != nil {
					return nil, 0, "", err
				}
				data, length = bytes.NewReader(text), int64(len(text))
				break
			}
			// A text field, which may hold a secret.
			text, err := reveal(plugin.TextOf(value))
			if err != nil {
				return nil, 0, "", err
			}
			add(head+"\r\n\r\n", strings.NewReader(text), int64(len(text)))
			continue
		}
		// Bytes and a blob are a file whether or not they were given a
		// name; one with none is named as its field is.
		if file == "" {
			file = name
		}
		if typ == "" {
			typ = "application/octet-stream"
		}
		// A line break in a part's type would start a header of its own.
		if strings.ContainsAny(typ, "\r\n\x00") {
			return nil, 0, "", plugin.Errorf(plugin.BadInput, "upload: the type of the file %q holds a line break or a zero byte", name)
		}
		add(head+"; filename=\""+quoted(file)+"\"\r\nContent-Type: "+typ+"\r\n\r\n", data, length)
	}
	end := "--" + boundary + "--\r\n"
	parts = append(parts, strings.NewReader(end))
	return io.MultiReader(parts...), size + int64(len(end)), "multipart/form-data; boundary=" + boundary, nil
}

// quoted writes a field's or a file's name as a browser does inside the
// quotes of a part's header: a quote and a line break are escaped, so the
// name cannot end the quotes or the line.
func quoted(name string) string {
	return strings.NewReplacer("\"", "%22", "\r", "%0D", "\n", "%0A", "\x00", "%00").Replace(name)
}
