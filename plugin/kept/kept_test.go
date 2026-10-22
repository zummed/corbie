package kept_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	_ "github.com/zummed/corbie/plugin/kept"
)

// values is a job's channel that keeps values as a runner would, in
// memory, and lists every one below a path.
type values struct {
	plugin.Channel
	kept map[string][]byte
}

func (v *values) ReadKept(path string) ([]byte, bool, error) {
	value, ok := v.kept[path]
	return value, ok, nil
}

func (v *values) WriteKept(path string, value []byte) error {
	v.kept[path] = value
	return nil
}

func (v *values) ListKept(path string) ([]string, error) {
	var paths []string
	for p := range v.kept {
		if strings.HasPrefix(p, path+"/") {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

func (v *values) RemoveKept(path string) error {
	delete(v.kept, path)
	return nil
}

func job(v *values, layer permission.Layer) *plugin.Job {
	layer.Name = "the layer"
	return &plugin.Job{Context: context.Background(), Channel: v, Perms: &permission.Set{Layers: []permission.Layer{layer}}}
}

func call(j *plugin.Job, name string, first any, opt plugin.Options) (any, error) {
	m, _ := plugin.Find("kept")
	return m.Calls[name].Invoke(j, "kept", name, first, opt)
}

func codeOf(err error) string {
	var known *plugin.Error
	if errors.As(err, &known) {
		return known.Code
	}
	return ""
}

// A listing is checked as reading everything below the path, and leaves
// out what a deny entry refuses.
func Test_plug_kept_list_checked_as_reading_below(t *testing.T) {
	v := &values{kept: map[string][]byte{"notes": nil, "notes/tea": nil, "notes/secret/plan": nil, "notesy": nil}}
	listed, err := call(job(v, permission.Layer{Allow: []string{"kept.read:notes/**"}, Deny: []string{"kept.read:notes/secret/**"}}), "list", "notes", nil)
	if err != nil || !slices.Equal(listed.([]any), []any{"notes/tea"}) {
		t.Fatalf("listed past a deny: %v %v", listed, err)
	}
	// Allowed the path alone, a listing is refused, and the refusal gives
	// the line that lets it list.
	_, err = call(job(v, permission.Layer{Allow: []string{"kept.read:notes"}}), "list", "notes", nil)
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || refusal.What != "kept.read:notes/**" || !strings.Contains(refusal.Fix, "add the line kept.read:notes/** to the layer") {
		t.Fatalf("a listing allowed only the path: %v", err)
	}
	if listed, err := call(job(v, permission.Layer{Allow: []string{"kept.read:notes/**", "kept.read:notes"}}), "list", "notes", nil); err != nil || len(listed.([]any)) != 2 {
		t.Fatalf("a listing allowed below: %v %v", listed, err)
	}
}

// A value is kept whole in place of what was there and read back in the
// form asked for; what cannot be kept is refused as bad input, before
// anything is asked of the runner.
func TestKeptValuesAreKeptWhole(t *testing.T) {
	v := &values{kept: map[string][]byte{}}
	j := job(v, permission.Layer{Allow: []string{"kept"}})
	t.Cleanup(j.End)
	for _, value := range []plugin.Options{{"text": "first"}, {"bytes": plugin.Bytes("second")}} {
		if _, err := call(j, "write", "notes/tea", value); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := call(j, "read", "notes/tea", nil); err != nil || got != "second" {
		t.Fatalf("after two writes, the value is %v: %v", got, err)
	}
	if got, err := call(j, "read", "notes/tea", plugin.Options{"as": "bytes"}); err != nil || string(got.(plugin.Bytes)) != "second" {
		t.Fatalf("read as bytes: %v %v", got, err)
	}
	if got, err := call(j, "remove", "notes/none", nil); err != nil || got != nil {
		t.Fatalf("removing nothing: %v %v", got, err)
	}
	handle := plugin.NewSecret("TOKEN")
	for what, try := range map[string]func() (any, error){
		"a path in capitals": func() (any, error) { return call(j, "read", "Notes/tea", nil) },
		"a path ending in /": func() (any, error) { return call(j, "list", "notes/", nil) },
		"two values": func() (any, error) {
			return call(j, "write", "notes/tea", plugin.Options{"text": "a", "bytes": plugin.Bytes("b")})
		},
		"a handle": func() (any, error) { return call(j, "write", "notes/tea", plugin.Options{"text": handle}) },
		"a handle inside json": func() (any, error) {
			return call(j, "write", "notes/tea", plugin.Options{"json": map[string]any{"k": handle}})
		},
		"a value over 64 MiB": func() (any, error) {
			return call(j, "write", "notes/big", plugin.Options{"bytes": make(plugin.Bytes, plugin.MaxWhole+1)})
		},
		"an option it does not take": func() (any, error) { return call(j, "read", "notes/tea", plugin.Options{"deep": true}) },
	} {
		if _, err := try(); codeOf(err) != plugin.BadInput {
			t.Errorf("%s: %v", what, err)
		}
	}
	if _, ok := v.kept["notes/big"]; ok {
		t.Fatal("a value too large was kept")
	}
}
