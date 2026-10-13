// Package fs is the plugin for files and directories.
package fs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

// maxWhole is the largest file handed to a script whole.
const maxWhole = plugin.MaxWhole

func init() {
	plugin.Register(&plugin.Module{Name: "fs", Page: page, Calls: map[string]*plugin.Call{
		"read":   {Options: []string{"as"}, Do: read},
		"write":  {Options: []string{"text", "bytes", "json", "blob", "secret", "append", "mode"}, Do: write},
		"list":   {Options: []string{"deep"}, Do: list},
		"stat":   {Do: stat},
		"exists": {Do: exists},
		"remove": {Options: []string{"deep"}, Do: remove},
		"mkdir":  {Do: mkdir},
		"move":   {Options: []string{"to"}, Do: move},
	}})
}

func path(job *plugin.Job, first any, write bool) (string, string, error) {
	given, err := plugin.Text(first, "the path")
	if err != nil {
		return "", "", err
	}
	real, err := job.Path(given, write)
	return given, real, err
}

func read(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, real, err := path(job, first, false)
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "text")
	if err != nil {
		return nil, err
	}
	if form == "blob" {
		// Read where it lies, not copied.
		return job.FileBlob(given, "")
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, plugin.FileError(err, given)
	}
	if info.Size() > maxWhole {
		return nil, plugin.Errorf(plugin.BadInput, "%s is %d bytes, too large to hold whole: read it as a blob, with { as: \"blob\" }", given, info.Size())
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return nil, plugin.FileError(err, given)
	}
	return job.As(data, form, plugin.TypeOf(given))
}

func write(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, real, err := path(job, first, true)
	if err != nil {
		return nil, err
	}
	var from io.Reader
	private := false
	given1 := 0
	for _, name := range []string{"text", "bytes", "json", "blob", "secret"} {
		if !opt.Has(name) {
			continue
		}
		given1++
		switch name {
		case "secret":
			// A secret goes into a file it is bound to.
			text, err := job.Reveal(opt[name], "fs.write:"+filepath.ToSlash(real))
			if err != nil {
				return nil, err
			}
			from, private = strings.NewReader(text), true
		default:
			// A blob is written as it is read, a part at a time.
			if from, _, _, err = job.Reading(opt[name]); err != nil {
				return nil, err
			}
		}
	}
	if given1 != 1 {
		return nil, plugin.Errorf(plugin.BadInput, "fs.write takes what to write as one of text, bytes, json, blob or secret")
	}
	appending, err := opt.Bool("append", false)
	if err != nil {
		return nil, err
	}
	// Writing over a file leaves the file's mode as it was.
	mode, err := opt.Int("mode", 0o644)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		return nil, plugin.FileError(err, given)
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if appending {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	if private {
		// A file a secret is written into is readable by the runner's user
		// alone.
		mode = 0o600
	}
	file, err := os.OpenFile(real, flags, fs.FileMode(mode)&0o777)
	if err != nil {
		return nil, plugin.FileError(err, given)
	}
	if private {
		if err := file.Chmod(0o600); err != nil {
			file.Close()
			return nil, plugin.FileError(err, given)
		}
	}
	wrote, err := io.Copy(file, from)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, plugin.FileError(err, given)
	}
	return float64(wrote), nil
}

func entry(name string, info fs.FileInfo) map[string]any {
	kind := "file"
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		kind = "link"
	case info.IsDir():
		kind = "directory"
	}
	return map[string]any{"name": name, "kind": kind, "size": float64(info.Size()),
		"modified": info.ModTime().UTC().Format(time.RFC3339), "mode": float64(info.Mode().Perm())}
}

func list(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, real, err := path(job, first, false)
	if err != nil {
		return nil, err
	}
	deep, err := opt.Bool("deep", false)
	if err != nil {
		return nil, err
	}
	var out []any
	var walk func(dir, prefix string) error
	walk = func(dir, prefix string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, k int) bool { return entries[i].Name() < entries[k].Name() })
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			full := filepath.Join(dir, e.Name())
			// What the job may not read is left out, and a link to a
			// directory is not gone through.
			if !job.MayRead(full) {
				continue
			}
			out = append(out, entry(prefix+e.Name(), info))
			if deep && e.IsDir() {
				if err := walk(full, prefix+e.Name()+"/"); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(real, ""); err != nil {
		return nil, plugin.FileError(err, given)
	}
	if out == nil {
		out = []any{}
	}
	return out, nil
}

func stat(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, real, err := path(job, first, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, plugin.FileError(err, given)
	}
	return entry(filepath.Base(given), info), nil
}

func exists(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	_, real, err := path(job, first, false)
	if err != nil {
		return nil, err
	}
	_, err = os.Lstat(real)
	return err == nil, nil
}

func remove(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, err := plugin.Text(first, "the path")
	if err != nil {
		return nil, err
	}
	deep, err := opt.Bool("deep", false)
	if err != nil {
		return nil, err
	}
	// The name itself is removed, not what it points to if it is a link.
	target, err := job.Entry(given)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, plugin.FileError(err, given)
	}
	if info.IsDir() && deep {
		// Every entry is checked, and nothing is removed if one is refused.
		for _, inside := range within(target) {
			if _, err := job.Entry(inside); err != nil {
				return nil, err
			}
		}
		if err := os.RemoveAll(target); err != nil {
			return nil, plugin.FileError(err, given)
		}
		return nil, nil
	}
	if err := os.Remove(target); err != nil {
		return nil, plugin.FileError(err, given)
	}
	return nil, nil
}

// within lists a name and, where it is a directory, everything inside it,
// each by its own name: no link is gone through.
func within(target string) []string {
	var all []string
	filepath.WalkDir(target, func(inside string, _ fs.DirEntry, err error) error {
		if err == nil {
			all = append(all, inside)
		}
		return nil
	})
	return all
}

func mkdir(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, real, err := path(job, first, true)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(real, 0o755); err != nil {
		return nil, plugin.FileError(err, given)
	}
	return nil, nil
}

func move(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	given, err := plugin.Text(first, "the path")
	if err != nil {
		return nil, err
	}
	wanted, err := plugin.Text(opt["to"], "to:, the path to move it to,")
	if err != nil {
		return nil, err
	}
	// It is the name that moves, and the name that is taken: a link is
	// moved and not what it points to.
	from, err := job.Entry(given)
	if err != nil {
		return nil, err
	}
	to, err := job.Entry(wanted)
	if err != nil {
		return nil, err
	}
	// A directory takes everything inside it along. Every entry is checked,
	// where it is and where it goes, and nothing moves if one is refused.
	for _, inside := range within(from) {
		if _, err := job.Entry(inside); err != nil {
			return nil, err
		}
		if _, err := job.Entry(to + strings.TrimPrefix(inside, from)); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, plugin.FileError(err, given)
	}
	if err := os.Rename(from, to); err != nil {
		return nil, plugin.FileError(err, given)
	}
	return nil, nil
}

const page = `fs: this plugin reads and writes files and directories.

  fs.read(path, { as })              the file's contents
  fs.write(path, { text | bytes | json | blob | secret, append, mode })
                                     writes the file; returns the number of bytes written
  fs.list(path, { deep })            the entries, by name: [{ name, kind, size, modified, mode }]
  fs.stat(path)                      one entry
  fs.exists(path)                    true if the path exists
  fs.remove(path, { deep })          removes a file, a link or an empty directory; returns nothing
  fs.mkdir(path)                     makes the directory and those above it; returns nothing
  fs.move(path, { to })              moves or renames a file, a link or a directory; returns nothing

A path with no leading / is in the job's scratch directory (` + "`corbie manual scripts`" + `).

Options:
  as       "text", "bytes", "json" or "blob"; default "text"
  secret   a handle to a secret (` + "`corbie manual secrets`" + `); the file's mode is then 0600
  append   adds to the end of the file; default false
  mode     the mode of a new file; default 0o644
  deep     lists the directories below too, or removes a directory with its contents; default false

An entry's fields:
  kind       "file", "directory" or "link"
  modified   a time in UTC, as "2026-10-13T20:02:07Z"
  mode       the permission bits, as a number

A file read or written as a blob may be any size (` + "`corbie manual blobs`" + `).

Errors:
  not_found       the path does not exist
  not_permitted   the operating system refuses the runner
  bad_input       a file over 64 MiB read in a form other than "blob"; a file read as "json" that is not JSON
  failed          anything else, such as removing a directory that is not empty

Needs these permissions, at the path's real location after links are followed:
  fs.read:path                read, list, stat, exists
  fs.write:path               write, mkdir, remove, and move for both paths
  secret:NAME@fs.write:path   a secret written to the file

- fs.remove and fs.move act on the name: a link is removed or moved, not what
  it points to.
- A directory removed or moved needs fs.write:path for every entry inside, at
  the old path and the new. If one is refused, nothing is removed or moved.
- fs.list omits the entries that the job may not read.

This script writes a blob to a file and reads the file as a blob.

    allow: [fs.write:in.csv, fs.read:in.csv]

    const table = blobs.make("item,amount\ntea,12\n", { type: "text/csv" })
    fs.write("in.csv", { blob: table })                     // gives 19
    return fs.read("in.csv", { as: "blob" }).size           // gives 19

This script makes the other calls.

    allow: [fs.read:/srv/reports/**, fs.read:out/**, fs.write:out/**]

    const text = fs.read("/srv/reports/latest.md")
    fs.write("out/summary.txt", { text: text.slice(0, 200) })    // gives 52
    fs.exists("out/summary.txt")                                 // gives true
    fs.stat("out/summary.txt").size                              // gives 52
    fs.mkdir("out/2026")
    fs.move("out/summary.txt", { to: "out/2026/q3.txt" })
    fs.list("out", { deep: true }).map((entry) => entry.name)    // gives ["2026", "2026/q3.txt"]
    fs.remove("out/2026", { deep: true })

This script writes a secret to a file.

    allow: [fs.write:/srv/app/key, "secret:APP_KEY@fs.write:/srv/app/key"]

    fs.write("/srv/app/key", { secret: secrets.get("APP_KEY") })
`
