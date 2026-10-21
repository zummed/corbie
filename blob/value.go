package blob

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/zummed/corbie/store"
)

// Values here are what JSON decodes to: maps, lists, text, numbers.

// AsRef says whether a value is a reference, and gives it.
func AsRef(v any) (Ref, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 4 {
		return Ref{}, false
	}
	at, ok1 := m["$blob"].(string)
	typ, ok2 := m["type"].(string)
	size, ok3 := m["size"].(float64)
	key, ok4 := m["key"].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 || size < 0 || size != float64(int64(size)) {
		return Ref{}, false
	}
	return Ref{At: at, Type: typ, Size: int64(size), Key: key}, true
}

// Value is the reference as it sits inside a value.
func (r Ref) Value() any {
	return map[string]any{"$blob": r.At, "type": r.Type, "size": float64(r.Size), "key": r.Key}
}

// Walk gives a copy of a value in which every reference has been replaced
// by what fn gives for it. The path says where in the value it was found. A
// reference written out inside a text is text, and is passed over.
func Walk(v any, fn func(path string, ref Ref) (any, error)) (any, error) {
	return walk(v, "", fn, false)
}

// Shown is Walk for a value that is to be displayed or handed to a model:
// a reference written out as JSON inside a text is found too, and what fn
// gives for it is written in its place. A text is where the contents of a
// blob end up, and they may hold another blob's reference.
func Shown(v any, fn func(path string, ref Ref) (any, error)) (any, error) {
	return walk(v, "", fn, true)
}

func walk(v any, at string, fn func(string, Ref) (any, error), texts bool) (any, error) {
	if ref, ok := AsRef(v); ok {
		return fn(at, ref)
	}
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		// In the order of the names, so the same value is always walked
		// the same way.
		for _, k := range slices.Sorted(maps.Keys(v)) {
			sub := k
			if at != "" {
				sub = at + "." + k
			}
			w, err := walk(v[k], sub, fn, texts)
			if err != nil {
				return nil, err
			}
			out[k] = w
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			w, err := walk(item, at+"["+strconv.Itoa(i)+"]", fn, texts)
			if err != nil {
				return nil, err
			}
			out[i] = w
		}
		return out, nil
	case string:
		if texts {
			return written(v, at, fn)
		}
	}
	return v, nil
}

// written gives a text in which every reference written out as JSON has
// been replaced by what fn gives for it, written the same way. The path of
// one is the text's own, and which reference in the text it is, counted
// from one. Anything that only looks like a reference is passed over.
func written(text, at string, fn func(string, Ref) (any, error)) (string, error) {
	const name = `"$blob"`
	var out strings.Builder
	found := 0
	for {
		i := strings.Index(text, name)
		if i < 0 {
			break
		}
		open, end, ref := enclosing(text, i)
		if open < 0 {
			out.WriteString(text[:i+len(name)])
			text = text[i+len(name):]
			continue
		}
		found++
		shown, err := fn(at+"#"+strconv.Itoa(found), ref)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(shown, json.Deterministic(true))
		if err != nil {
			return "", err
		}
		out.WriteString(text[:open])
		out.Write(data)
		text = text[end:]
	}
	out.WriteString(text)
	return out.String(), nil
}

// enclosing finds the reference that the name at i is part of: where its
// JSON starts, at the brace before the name, and where it ends.
func enclosing(text string, i int) (open, end int, ref Ref) {
	if open = strings.LastIndexByte(text[:i], '{'); open < 0 {
		return -1, -1, Ref{}
	}
	raw, err := jsontext.NewDecoder(strings.NewReader(text[open:])).ReadValue()
	if err != nil {
		return -1, -1, Ref{}
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return -1, -1, Ref{}
	}
	ref, ok := AsRef(v)
	if !ok {
		return -1, -1, Ref{}
	}
	return open, open + len(raw), ref
}

// Refs gives every reference in a value.
func Refs(v any) []Ref {
	var refs []Ref
	Walk(v, func(_ string, ref Ref) (any, error) {
		refs = append(refs, ref)
		return nil, nil
	})
	return refs
}

// Show gives a value as it is displayed to a person or handed to a model:
// each blob appears as its type, its size and the path to ask for it by,
// and no key is shown.
func Show(v any) any {
	shown, _ := Shown(v, func(path string, ref Ref) (any, error) {
		return map[string]any{"$blob": path, "type": ref.Type, "size": float64(ref.Size)}, nil
	})
	return shown
}

// Find gives the reference at a path in a value, as Show named it.
func Find(v any, path string) (Ref, bool) {
	var found Ref
	var ok bool
	Shown(v, func(at string, ref Ref) (any, error) {
		if at == path {
			found, ok = ref, true
		}
		return nil, nil
	})
	return found, ok
}

// A stored definition names its scripts and child definitions by
// references that give only a blob's own name. They are found in the record
// the definition is in, so a definition can be copied from one record to
// another with what it names.

// Beside resolves a reference found inside a stored definition.
func (r Ref) Beside(inner Ref) Ref {
	if strings.Contains(inner.At, "/") {
		return inner
	}
	inner.At = path.Dir(r.At) + "/" + inner.At
	return inner
}

// Local is the reference as a stored definition in the same record names
// it.
func (r Ref) Local() Ref {
	r.At = path.Base(r.At)
	return r
}

// Named gives the blobs a stored definition names, and those the
// definitions among them name, each resolved to where it is.
func Named(ctx context.Context, s store.Store, def Ref) ([]Ref, error) {
	if def.Type != Definition {
		return nil, nil
	}
	contents, err := ReadAll(ctx, s, def, StepLimit)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(contents, &v); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDefinition, err)
	}
	var all []Ref
	for _, inner := range Refs(v) {
		inner = def.Beside(inner)
		all = append(all, inner)
		more, err := Named(ctx, s, inner)
		if err != nil {
			return nil, err
		}
		all = append(all, more...)
	}
	return all, nil
}

// CopyWhole copies a blob into an instance's record, with what it names if
// it is a stored definition. It gives the copy's reference.
func CopyWhole(ctx context.Context, s store.Store, ref Ref, ctl, instance string) (Ref, error) {
	named, err := Named(ctx, s, ref)
	if err != nil {
		return Ref{}, err
	}
	for _, inner := range named {
		if _, err := Copy(ctx, s, inner, ctl, instance); err != nil {
			return Ref{}, err
		}
	}
	return Copy(ctx, s, ref, ctl, instance)
}
