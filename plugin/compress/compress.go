// Package compress is the plugin for packing and unpacking: gzip, and
// archives of files.
package compress

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "compress", Page: page, Calls: map[string]*plugin.Call{
		"gzip":    {Options: []string{"as"}, Do: pack},
		"gunzip":  {Options: []string{"as"}, Do: unpack},
		"list":    {Do: list},
		"read":    {Options: []string{"entry", "as"}, Do: read},
		"archive": {Options: []string{"format", "as"}, Do: archive},
	}})
}

func pack(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	from, _, _, err := job.Reading(first)
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "bytes")
	if err != nil {
		return nil, err
	}
	return job.Give(form, "application/gzip", "the gzip", func(w io.Writer) error {
		z := gzip.NewWriter(w)
		if _, err := io.Copy(z, from); err != nil {
			return failed("it", "packed", err)
		}
		return failed("it", "packed", z.Close())
	})
}

func unpack(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	from, _, _, err := job.Reading(first)
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "bytes")
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(from)
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "it is not gzip: %v", err)
	}
	return job.Give(form, "", "what it unpacks to", func(w io.Writer) error {
		_, err := io.Copy(w, r)
		return failed("it", "unpacked", err)
	})
}

// failed gives the error of packing or unpacking something. Running out of
// room to hold it whole is not one: whoever holds it says so.
func failed(what, done string, err error) error {
	if err == nil || errors.Is(err, io.ErrShortWrite) {
		return nil
	}
	var known *plugin.Error
	if errors.As(err, &known) {
		return err
	}
	return plugin.Errorf(plugin.BadInput, "%s cannot be %s: %v", what, done, err)
}

// checkName refuses an entry's name that would land outside the directory an
// archive is unpacked into: one that starts at the root, or has .. in it.
func checkName(name string) error {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	bad := name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\")
	for _, part := range parts {
		bad = bad || part == ".."
	}
	if bad {
		return plugin.Errorf(plugin.BadInput, "an entry is named %q: a name may not start at the root or have .. in it", name)
	}
	return nil
}

// entries goes through the files of an archive, in its order, until visit
// says to stop. Only files are entries: directories, links and the rest are
// passed over. A zip, a tar and a gzipped tar are told apart by how they
// start. The archive is read where it lies: a zip by its list of contents
// and then the one file asked for, a tar from its start.
func entries(src io.ReaderAt, size int64, visit func(name string, size int64, open func() (io.Reader, error)) (stop bool, err error)) error {
	start := make([]byte, 2)
	if n, _ := src.ReadAt(start, 0); n < 2 {
		return plugin.Errorf(plugin.BadInput, "the archive cannot be read: it is not a zip, a tar or a gzipped tar")
	}
	if string(start) == "PK" {
		z, err := zip.NewReader(src, size)
		if err != nil {
			return plugin.Errorf(plugin.BadInput, "the archive cannot be read: %v", err)
		}
		for _, f := range z.File {
			if !f.Mode().IsRegular() {
				continue
			}
			if err := checkName(f.Name); err != nil {
				return err
			}
			stop, err := visit(f.Name, int64(f.UncompressedSize64), func() (io.Reader, error) { return f.Open() })
			if stop || err != nil {
				return err
			}
		}
		return nil
	}
	var from io.Reader = io.NewSectionReader(src, 0, size)
	if start[0] == 0x1f && start[1] == 0x8b {
		unzipped, err := gzip.NewReader(from)
		if err != nil {
			return plugin.Errorf(plugin.BadInput, "the archive cannot be read: %v", err)
		}
		from = unzipped
	}
	t := tar.NewReader(from)
	for {
		header, err := t.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return plugin.Errorf(plugin.BadInput, "the archive cannot be read: it is not a zip, a tar or a gzipped tar that can be followed (%v)", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if err := checkName(header.Name); err != nil {
			return err
		}
		stop, err := visit(header.Name, header.Size, func() (io.Reader, error) { return t, nil })
		if stop || err != nil {
			return err
		}
	}
}

func list(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	src, size, err := job.Source(first)
	if err != nil {
		return nil, err
	}
	found := []any{}
	err = entries(src, size, func(name string, size int64, _ func() (io.Reader, error)) (bool, error) {
		found = append(found, map[string]any{"name": name, "size": float64(size)})
		return false, nil
	})
	return found, err
}

func read(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	src, size, err := job.Source(first)
	if err != nil {
		return nil, err
	}
	entry, err := opt.Text("entry", "")
	if err != nil {
		return nil, err
	}
	if entry == "" {
		return nil, plugin.Errorf(plugin.BadInput, "compress.read needs entry: the name of the file to read")
	}
	form, err := opt.Text("as", "bytes")
	if err != nil {
		return nil, err
	}
	var contents any
	found := false
	err = entries(src, size, func(name string, _ int64, open func() (io.Reader, error)) (bool, error) {
		if name != entry {
			return false, nil
		}
		found = true
		r, err := open()
		if err != nil {
			return true, plugin.Errorf(plugin.BadInput, "%s cannot be unpacked: %v", entry, err)
		}
		contents, err = job.Give(form, plugin.TypeOf(entry), entry, func(w io.Writer) error {
			_, err := io.Copy(w, r)
			return failed(entry, "unpacked", err)
		})
		return true, err
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, plugin.Errorf(plugin.NotFound, "the archive has no file named %s", entry)
	}
	return contents, nil
}

func archive(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	files, ok := first.(map[string]any)
	if !ok {
		return nil, plugin.Errorf(plugin.BadInput, "compress.archive takes a map first: each file's name and its contents")
	}
	format, err := opt.Text("format", "zip")
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "bytes")
	if err != nil {
		return nil, err
	}
	typ, ok := map[string]string{"zip": "application/zip", "tar": "application/x-tar", "tar.gz": "application/gzip"}[format]
	if !ok {
		return nil, plugin.Errorf(plugin.BadInput, "format: is \"zip\", \"tar\" or \"tar.gz\", not %q", format)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if err := checkName(name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	sort.Strings(names)
	// Each file is packed as it is read, and the archive written as it is
	// packed: nothing of it is held.
	return job.Give(form, typ, "the archive", func(w io.Writer) error {
		var z *zip.Writer
		var t *tar.Writer
		var gz *gzip.Writer
		switch format {
		case "zip":
			z = zip.NewWriter(w)
		case "tar.gz":
			gz = gzip.NewWriter(w)
			t = tar.NewWriter(gz)
		default:
			t = tar.NewWriter(w)
		}
		for _, name := range names {
			from, size, _, err := job.Reading(files[name])
			if err != nil {
				return err
			}
			var into io.Writer = t
			if z != nil {
				into, err = z.Create(name)
			} else {
				err = t.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: size})
			}
			if err != nil {
				return plugin.Errorf(plugin.BadInput, "%s cannot be an entry: %v", name, err)
			}
			if _, err := io.Copy(into, from); err != nil {
				return failed(name, "packed", err)
			}
		}
		var err error
		if z != nil {
			err = z.Close()
		} else if err = t.Close(); err == nil && gz != nil {
			err = gz.Close()
		}
		return failed("the archive", "packed", err)
	})
}

const page = `compress: this plugin compresses data with gzip, and makes and reads archives.

  compress.gzip(data, { as })               the data, compressed
  compress.gunzip(data, { as })             the data, decompressed
  compress.list(archive)                    the files: [{ name, size }], size in bytes uncompressed
  compress.read(archive, { entry, as })     the contents of one file
  compress.archive(files, { format, as })   an archive of the files

data, an archive and a file's contents are text, bytes or a blob. files maps
each file's name to its contents. An archive is a zip, a tar or a gzipped tar,
detected from its first bytes.

Options:
  as       "bytes", "text", "json" or "blob"; default "bytes"
  entry    the name of the file to read; required
  format   "zip", "tar" or "tar.gz"; default "zip"

Only files are entries: directories and links are skipped. One file of a zip
is read without reading the rest. A result that is not a blob is at most
64 MiB (` + "`corbie manual blobs`" + `).

Errors:
  bad_input   data that is not gzip, an archive that cannot be read, an entry's name that
              starts with / or contains .., or a result over 64 MiB that is not a blob
  not_found   compress.read: the archive has no file of that name

Needs nothing.

This script compresses text and decompresses it.

    const packed = compress.gzip("hello")
    compress.gunzip(packed, { as: "text" })                       // gives "hello"

This script makes a zip, lists it and reads one file.

    const zip = compress.archive({ "a.txt": "one", "notes/b.txt": "two" })
    compress.list(zip)                                            // gives [{ name: "a.txt", size: 3 }, { name: "notes/b.txt", size: 3 }]
    compress.read(zip, { entry: "notes/b.txt", as: "text" })      // gives "two"
`
