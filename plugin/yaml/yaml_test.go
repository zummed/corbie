package yaml

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// A job no layer allows anything: the plugin needs no permission.
func call(t *testing.T, name string, first any) (any, error) {
	t.Helper()
	job := &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test"}}},
		Scratch: t.TempDir(),
	}
	module, _ := plugin.Find("yaml")
	return module.Calls[name].Invoke(job, "yaml", name, first, nil)
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

const hub = `# a hub
does: The firewall and the switch.
base: &base { timeout: 20s }
tools:
  junos_show:
    does: >-
      Run one show
      command.
    args: { device: string, n: 3, ok: true, none: ~ }
    job:
      <<: *base
      script: ./tools/junos-show.js
      files: [./tools/junos.js]
when: 2026-10-29
`

func Test_yaml_document_as_data(t *testing.T) {
	doc, err := call(t, "parse", hub)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"does": "The firewall and the switch.",
		"base": map[string]any{"timeout": "20s"},
		"tools": map[string]any{"junos_show": map[string]any{
			"does": "Run one show command.",
			"args": map[string]any{"device": "string", "n": 3.0, "ok": true, "none": nil},
			"job":  map[string]any{"<<": map[string]any{"timeout": "20s"}, "script": "./tools/junos-show.js", "files": []any{"./tools/junos.js"}},
		}},
		"when": "2026-10-29",
	}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("parse gave %v", doc)
	}
	// Bytes and a blob are read the same.
	if doc, err := call(t, "parse", plugin.Bytes("a: 1\n")); err != nil || !reflect.DeepEqual(doc, map[string]any{"a": 1.0}) {
		t.Fatalf("parse of bytes gave %v, %v", doc, err)
	}
	if doc, err := call(t, "parse", ""); err != nil || doc != nil {
		t.Fatalf("parse of nothing gave %v, %v", doc, err)
	}
}

func Test_yaml_not_a_document(t *testing.T) {
	for document, why := range map[string]string{
		"a: [1, 2":          "did not find expected",
		"a: 1\n---\nb: 2\n": "more than one document",
		"? [a]\n: 1\n":      "line 1: a key is plain text",
		"a: 1\na: 2\n":      "", // the library refuses it, with its own words
		"a: &x [*x]\n":      "",
		"a: &a [1]\nb: &b [*a, *a, *a, *a, *a, *a, *a, *a, *a, *a]\nc: &c [*b, *b, *b, *b, *b, *b, *b, *b, *b, *b]\nd: &d [*c, *c, *c, *c, *c, *c, *c, *c, *c, *c]\ne: &e [*d, *d, *d, *d, *d, *d, *d, *d, *d, *d]\nf: &f [*e, *e, *e, *e, *e, *e, *e, *e, *e, *e]\ng: [*f, *f, *f, *f, *f, *f, *f, *f, *f, *f]\n": "far larger",
	} {
		_, err := call(t, "parse", document)
		if code(err) != plugin.BadInput || !strings.Contains(err.Error(), why) {
			t.Errorf("parse of %q gave %v", document, err)
		}
	}
}

func Test_yaml_write(t *testing.T) {
	out, err := call(t, "write", map[string]any{"name": "tea", "sizes": []any{1.0, 2.5}, "hot": "true", "none": nil, "deep": map[string]any{"b": "x", "a": "w"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "deep:\n  a: w\n  b: x\nhot: \"true\"\nname: tea\nnone: null\nsizes:\n  - 1\n  - 2.5\n"
	if out != want {
		t.Fatalf("write gave %q", out)
	}
	// What is written reads back as the same data.
	back, err := call(t, "parse", out)
	if err != nil || back.(map[string]any)["hot"] != "true" || back.(map[string]any)["sizes"].([]any)[1] != 2.5 {
		t.Fatalf("it read back as %v, %v", back, err)
	}
	for what, data := range map[string]any{
		"a handle": map[string]any{"k": plugin.NewSecret("K")},
		"bytes":    []any{plugin.Bytes("x")},
		"a blob":   plugin.NewBlob("text/plain", []byte("x")),
	} {
		if out, err := call(t, "write", data); code(err) != plugin.BadInput {
			t.Errorf("%s was written: %v, %v", what, out, err)
		}
	}
}
