// Package yaml is the plugin for reading a YAML document as data, and
// writing data as YAML.
package yaml

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "yaml", Page: page, Calls: map[string]*plugin.Call{
		"parse": {Do: parse},
		"write": {Do: write},
	}})
}

func parse(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	data, _, err := job.Contents(first)
	if err != nil {
		return nil, err
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := d.Decode(&doc); err != nil && err != io.EOF {
		return nil, plugin.Errorf(plugin.BadInput, "the document cannot be read: %s", strings.TrimPrefix(err.Error(), "yaml: "))
	}
	var more yaml.Node
	if err := d.Decode(&more); err != io.EOF {
		return nil, plugin.Errorf(plugin.BadInput, "the document cannot be read: it holds more than one document")
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}
	budget := 200000
	return plain(doc.Content[0], &budget, map[*yaml.Node]bool{})
}

// plain turns a node into plain data. Aliases may not grow a document far
// beyond what was written, nor contain themselves.
func plain(n *yaml.Node, budget *int, inside map[*yaml.Node]bool) (any, error) {
	bad := func(why string) (any, error) {
		return nil, plugin.Errorf(plugin.BadInput, "the document cannot be read: line %d: %s", n.Line, why)
	}
	if *budget--; *budget < 0 {
		return bad("its aliases make it far larger than what is written")
	}
	switch n.Kind {
	case yaml.AliasNode:
		if inside[n.Alias] {
			return bad("an alias contains itself")
		}
		inside[n.Alias] = true
		defer delete(inside, n.Alias)
		return plain(n.Alias, budget, inside)
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return bad("a key is plain text, a number or true or false")
			}
			if _, dup := m[k.Value]; dup {
				return bad(fmt.Sprintf("the key %q is written twice", k.Value))
			}
			v, err := plain(n.Content[i+1], budget, inside)
			if err != nil {
				return nil, err
			}
			m[k.Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			v, err := plain(item, budget, inside)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.ScalarNode:
		// A plain value YAML would read as a date stays the text written.
		switch n.ShortTag() {
		case "!!null":
			return nil, nil
		case "!!bool":
			var b bool
			if err := n.Decode(&b); err == nil {
				return b, nil
			}
		case "!!int", "!!float":
			var f float64
			if err := n.Decode(&f); err == nil {
				return f, nil
			}
		}
		return n.Value, nil
	}
	return bad("this part cannot be read")
}

func write(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	if err := check(first); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(first); err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "this cannot be written as YAML: %v", err)
	}
	if err := enc.Close(); err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "this cannot be written as YAML: %v", err)
	}
	return out.String(), nil
}

// check refuses anything that is not plain data: a handle to a secret, a
// blob or bytes anywhere in it.
func check(v any) error {
	switch v := v.(type) {
	case nil, bool, float64, string:
		return nil
	case map[string]any:
		for _, item := range v {
			if err := check(item); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, item := range v {
			if err := check(item); err != nil {
				return err
			}
		}
		return nil
	case *plugin.Secret:
		return plugin.Errorf(plugin.BadInput, "a secret cannot be written as YAML: it can go only where a plugin's page says a secret may")
	}
	return plugin.Errorf(plugin.BadInput, "only plain data can be written as YAML: text, numbers, true or false, null, lists and maps, and not %s", kind(v))
}

func kind(v any) string {
	switch v.(type) {
	case *plugin.Blob:
		return "a blob"
	case plugin.Bytes:
		return "bytes"
	}
	return fmt.Sprintf("%T", v)
}

const page = `yaml: this plugin parses a YAML document into data and writes data as YAML.

  yaml.parse(document)    returns the document's data
  yaml.write(data)        returns the data as a YAML document

document is text, bytes or a blob, and holds one YAML document.

- parse gives maps, lists, text, numbers, true or false, and null.
- A value YAML would read as a date or a time stays the text written.
- An alias gives a copy of what its anchor marks. A merge key, <<, is an
  ordinary key.
- write writes a map's keys in order, with an indent of two spaces, and puts
  text in quotes where it would otherwise read as another value.

Errors:
  bad_input   the document is not YAML (the message gives the line), it holds
              more than one document, or a key is a map, a list or written
              twice; or write was given something that is not plain data

Needs nothing.

This script reads a document and writes another.

    const hub = yaml.parse("does: The bins.\ntools:\n  bin_days: { args: {} }\n")
    hub.does                    // gives "The bins."
    Object.keys(hub.tools)      // gives ["bin_days"]

    yaml.write({ name: "tea", sizes: [1, 2], hot: "true" })
    // gives "hot: \"true\"\nname: tea\nsizes:\n  - 1\n  - 2\n"
`
