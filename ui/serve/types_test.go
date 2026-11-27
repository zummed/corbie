package serve

import (
	"encoding"
	"encoding/json/jsontext"
	"fmt"
	"html/template"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/ui/controller"
	"github.com/zummed/corbie/ui/machines"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/runs"
)

// The types of what a page is sent are written for editors as TypeScript
// declarations, from the Go types the UI sends, so a page's script is
// checked against them. When the file differs, the test writes it and
// fails, so the change is seen and committed.
func Test_serve_types_are_what_pages_are_sent(t *testing.T) {
	roots := []struct {
		name string
		v    any
	}{
		{"PartMessage", page.PartMessage{}},
		{"StoreMessage", page.StoreMessage{}},
		{"RunsListParts", runs.ListParts{}},
		{"RunParts", runs.RunParts{}},
		{"MachinesParts", machines.MachinesParts{}},
		{"ControllerParts", controller.Parts{}},
	}
	w := &typeWriter{names: map[reflect.Type]string{}}
	for _, r := range roots {
		w.names[reflect.TypeOf(r.v)] = r.name
	}
	for _, r := range roots {
		w.declare(reflect.TypeOf(r.v))
	}
	got := "// The types of what the UI sends a page, written by `go test ./serve`\n// from the Go types; do not edit.\n\n" +
		"export type Message = PartMessage | StoreMessage;\n\n" + strings.Join(w.out, "\n")
	const file = "../types.d.ts"
	if was, _ := os.ReadFile(file); string(was) != got {
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Errorf("%s was not the types of what pages are sent; it is written again", file)
	}
}

// A typeWriter writes a declaration for each struct type it meets, by the
// Go type's name, made unique by its package where two share one.
type typeWriter struct {
	names map[reflect.Type]string
	done  []reflect.Type
	out   []string
}

var (
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	timeType      = reflect.TypeFor[time.Time]()
	rawType       = reflect.TypeFor[jsontext.Value]()
	htmlType      = reflect.TypeFor[template.HTML]()
)

// ts gives the TypeScript type of a Go type as encoding/json/v2 writes it.
func (w *typeWriter) ts(t reflect.Type) string {
	switch {
	case t == timeType, t == htmlType, t.Implements(textMarshaler), reflect.PointerTo(t).Implements(textMarshaler) && t.Kind() == reflect.Struct:
		return "string"
	case t == rawType:
		return "unknown"
	}
	switch t.Kind() {
	case reflect.Pointer:
		return w.ts(t.Elem()) + " | null"
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return "string"
		}
		elem := w.ts(t.Elem())
		if strings.Contains(elem, " ") {
			elem = "(" + elem + ")"
		}
		return elem + "[]"
	case reflect.Map:
		return "Record<string, " + w.ts(t.Elem()) + ">"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Interface:
		return "unknown"
	case reflect.Struct:
		w.declare(t)
		return w.name(t)
	}
	panic(fmt.Sprintf("no TypeScript type for %v", t))
}

func (w *typeWriter) name(t reflect.Type) string {
	if n, ok := w.names[t]; ok {
		return n
	}
	n := strings.ToUpper(t.Name()[:1]) + t.Name()[1:]
	for _, taken := range w.names {
		if taken == n {
			n = strings.ToUpper(pkgName(t)[:1]) + pkgName(t)[1:] + n
			break
		}
	}
	w.names[t] = n
	return n
}

func pkgName(t reflect.Type) string {
	p := t.PkgPath()
	return p[strings.LastIndexByte(p, '/')+1:]
}

// declare writes an interface for a struct, with each field as JSON names
// it: an embedded struct's fields as its own, a field left out when zero
// as optional, a field marked "-" not at all, and a field with a ts tag as
// the type the tag gives.
func (w *typeWriter) declare(t reflect.Type) {
	if slices.Contains(w.done, t) {
		return
	}
	w.done = append(w.done, t)
	name := w.name(t)
	var fields []string
	var add func(t reflect.Type)
	add = func(t reflect.Type) {
		for f := range t.Fields() {
			tag := f.Tag.Get("json")
			jsonName, opts, _ := strings.Cut(tag, ",")
			switch {
			case jsonName == "-":
				continue
			case f.Anonymous && jsonName == "" && f.Type.Kind() == reflect.Struct:
				add(f.Type)
				continue
			case !f.IsExported():
				continue
			}
			if jsonName == "" {
				jsonName = f.Name
			}
			optional := ""
			if strings.Contains(opts, "omitzero") || strings.Contains(opts, "omitempty") {
				optional = "?"
			}
			typ := f.Tag.Get("ts") // a field whose values are few names them
			if typ == "" {
				typ = w.ts(f.Type)
			}
			fields = append(fields, fmt.Sprintf("  %s%s: %s;", jsonName, optional, typ))
		}
	}
	add(t)
	w.out = append(w.out, fmt.Sprintf("export interface %s {\n%s\n}\n", name, strings.Join(fields, "\n")))
}
