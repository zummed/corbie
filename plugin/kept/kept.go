// Package kept is the plugin that keeps values in the store, where they
// stay after every run has ended.
package kept

import "github.com/zummed/corbie/plugin"

func init() {
	plugin.Register(&plugin.Module{Name: "kept", Page: page, Calls: map[string]*plugin.Call{
		"write":  {Options: []string{"text", "bytes", "json", "blob"}, Do: write},
		"read":   {Options: []string{"as", "otherwise"}, Do: read},
		"list":   {Do: list},
		"remove": {Do: remove},
	}})
}

func write(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	path, err := plugin.Text(first, "the kept path")
	if err != nil {
		return nil, err
	}
	var given []string
	for _, name := range []string{"text", "bytes", "json", "blob"} {
		if opt.Has(name) {
			given = append(given, name)
		}
	}
	if len(given) != 1 {
		return nil, plugin.Errorf(plugin.BadInput, "kept.write takes what to keep as one of text, bytes, json or blob")
	}
	value, _, err := job.Contents(opt[given[0]])
	if err != nil {
		return nil, err
	}
	return nil, job.WriteKept(path, value)
}

func read(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	path, err := plugin.Text(first, "the kept path")
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "text")
	if err != nil {
		return nil, err
	}
	value, kept, err := job.ReadKept(path)
	if err != nil {
		return nil, err
	}
	if !kept {
		return opt["otherwise"], nil
	}
	return job.As(value, form, plugin.TypeOf(path))
}

func list(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	path, err := plugin.Text(first, "the kept path")
	if err != nil {
		return nil, err
	}
	paths, err := job.ListKept(path)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, p := range paths {
		out = append(out, p)
	}
	return out, nil
}

func remove(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	path, err := plugin.Text(first, "the kept path")
	if err != nil {
		return nil, err
	}
	return nil, job.RemoveKept(path)
}

const page = `kept: this plugin keeps values in the store, where they stay after every run has ended.

A job keeps a value at a kept path, and a later job allowed that path reads
it. A value stays until a job removes it.

  kept.write(path, { text | bytes | json | blob })   keeps the value at the path in place of any there; returns nothing
  kept.read(path, { as, otherwise })                 the value kept at the path
  kept.list(path)                                    every kept path below the path that the job may read, in order
  kept.remove(path)                                  removes the value at the path; returns nothing

Options:
  as          "text", "bytes", "json" or "blob"; default "text"
  otherwise   what kept.read returns when nothing is kept at the path; default null

A kept path is parts joined by /. A part holds the lower-case letters a to z,
digits, ., - and _, and does not start with a dot. A path is at most 256
characters.

- A value is bytes, at most 64 MiB, and is read back in any form.
- The last write wins.
- A value can be kept at notes and another at notes/tea.
- A handle to a secret cannot be kept.
- The runner encrypts each value, and the job's process never holds the key.

Errors:
  bad_input   a path that is not a kept path; a value over 64 MiB; a value read as "json" that is not JSON
  failed      anything else, such as a value that was altered in the store

Needs these permissions, where * stands for any characters in one part and ** for any number of parts:
  kept.read:path       read
  kept.read:path/**    list below the path
  kept.write:path      write, remove

This script keeps two notes, reads them back, removes one and lists the rest.

    allow: [kept.write:notes/**, kept.read:notes/**]

    kept.write("notes/tea", { text: "Tea is up." })
    kept.write("notes/cake", { json: { down: true } })
    kept.read("notes/tea")                            // gives "Tea is up."
    kept.read("notes/cake", { as: "json" })           // gives { down: true }
    kept.read("notes/soup", { otherwise: "none" })    // gives "none"
    kept.remove("notes/cake")
    kept.list("notes")                                // gives ["notes/tea"]
`
