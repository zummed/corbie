package agent

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// A shape, of a tool's arguments or of an agent's reply, is a JSON Schema of
// an object. One written with type: object is in full form; any other is
// the short form, each name with the type of its value.

// types are the types a name may have in the short form, and what each is
// in full form.
var types = map[string]map[string]any{
	"string":  {"type": "string"},
	"number":  {"type": "number"},
	"boolean": {"type": "boolean"},
	"list":    {"type": "array"},
	"map":     {"type": "object"},
	"blob":    {"type": "string", "format": "blob"},
	"any":     {},
}

var typeNames = []string{"string", "number", "boolean", "list", "map", "blob", "any"}

// full gives a shape in full form. A short form that names a type there is
// not is refused, saying so.
func full(shape map[string]any) (map[string]any, error) {
	if shape["type"] == "object" {
		return shape, nil
	}
	properties := map[string]any{}
	var required []any
	for _, name := range sortedNames(shape) {
		text, ok := shape[name].(string)
		if !ok {
			return nil, fmt.Errorf("%q: a type is written as a word (the types are %s; add ? for one that may be left out), or the shape is written in full as JSON Schema with type: object", name, strings.Join(typeNames, ", "))
		}
		kind, optional := strings.CutSuffix(text, "?")
		if _, known := types[kind]; !known {
			return nil, fmt.Errorf("%q: %q is not a type (the types are %s; add ? for one that may be left out)", name, text, strings.Join(typeNames, ", "))
		}
		property := map[string]any{}
		for k, v := range types[kind] {
			property[k] = v
		}
		properties[name] = property
		if !optional {
			required = append(required, name)
		}
	}
	out := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if required != nil {
		out["required"] = required
	}
	return out, nil
}

// at is where a shape is said to live, so that a $ref outside it has an
// address to be refused by.
const at = "https://shape.invalid/shape.json"

// compiled reads a shape in full form as JSON Schema. A $ref resolves only
// within the shape: the check runs on the client and again on the
// controller, and neither reads a file or connects anywhere for one.
func compiled(shape map[string]any) (*jsonschema.Schema, error) {
	if shape["type"] != "object" {
		return nil, fmt.Errorf("a shape is a JSON Schema of an object, with type: object")
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(within{})
	if err := c.AddResource(at, shape); err != nil {
		return nil, err
	}
	schema, err := c.Compile(at)
	var outside *jsonschema.LoadURLError
	var invalid *jsonschema.SchemaValidationError
	switch {
	case errors.As(err, &outside):
		return nil, fmt.Errorf("a $ref resolves only within its own shape, and %q is outside it", strings.TrimPrefix(outside.URL, "https://shape.invalid/"))
	case errors.As(err, &invalid):
		return nil, fmt.Errorf("it is not JSON Schema: %v", wrong(invalid.Err))
	case err != nil:
		return nil, fmt.Errorf("%s", strings.ReplaceAll(err.Error(), at, "the shape"))
	}
	return schema, nil
}

type within struct{}

func (within) Load(url string) (any, error) { return nil, fmt.Errorf("outside the shape") }

// wrong says what is wrong with a value that does not fit a schema: each
// place in it and what is wrong there.
func wrong(err error) error {
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return err
	}
	var said []string
	var leaves func(u jsonschema.OutputUnit)
	leaves = func(u jsonschema.OutputUnit) {
		for _, e := range u.Errors {
			leaves(e)
		}
		if len(u.Errors) > 0 || u.Error == nil {
			return
		}
		line := u.Error.String()
		if u.InstanceLocation != "" {
			line = "at " + u.InstanceLocation + ": " + line
		}
		if !slices.Contains(said, line) {
			said = append(said, line)
		}
	}
	leaves(*invalid.DetailedOutput())
	return errors.New(strings.Join(said, "; "))
}

// blobs gives a value with each handle's name, where the shape marks a
// blob with format: blob, turned into the blob's reference. The value has
// been checked against the shape already.
func (s *stepper) blobs(root, shape map[string]any, v any, hops int) (any, error) {
	// A $ref and the keywords that apply more schemas to the same value
	// consume nothing of it: a shape that only goes round them is not
	// followed for ever.
	if hops > 64 {
		return v, nil
	}
	var err error
	if ref, ok := shape["$ref"].(string); ok {
		if to, found := pointer(root, ref); found {
			if v, err = s.blobs(root, to, v, hops+1); err != nil {
				return nil, err
			}
		}
	}
	for _, many := range []string{"allOf", "anyOf", "oneOf"} {
		subs, _ := shape[many].([]any)
		for _, sub := range subs {
			if m, ok := sub.(map[string]any); ok {
				if v, err = s.blobs(root, m, v, hops+1); err != nil {
					return nil, err
				}
			}
		}
	}
	switch v := v.(type) {
	case string:
		if shape["format"] != "blob" {
			return v, nil
		}
		ref, known := s.st.Handles[v]
		if !known {
			return nil, fmt.Errorf("%q should name a blob the conversation has shown, and is not one", v)
		}
		return ref, nil
	case map[string]any:
		properties, _ := shape["properties"].(map[string]any)
		more, _ := shape["additionalProperties"].(map[string]any)
		out := make(map[string]any, len(v))
		for name, item := range v {
			sub, _ := properties[name].(map[string]any)
			if _, named := properties[name]; !named {
				sub = more
			}
			if out[name], err = s.blobs(root, sub, item, 0); err != nil {
				return nil, err
			}
		}
		return out, nil
	case []any:
		first, _ := shape["prefixItems"].([]any)
		items := shape["items"]
		if old, isList := items.([]any); isList {
			first, items = old, shape["additionalItems"]
		}
		out := make([]any, len(v))
		for i, item := range v {
			sub, _ := items.(map[string]any)
			if i < len(first) {
				sub, _ = first[i].(map[string]any)
			}
			if out[i], err = s.blobs(root, sub, item, 0); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return v, nil
}

// pointer finds the part of a shape a $ref names by a JSON pointer within
// it, as #/$defs/user.
func pointer(root map[string]any, ref string) (map[string]any, bool) {
	path, ok := strings.CutPrefix(ref, "#")
	if !ok {
		return nil, false
	}
	var at any = root
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if path == "" {
			break
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := at.(type) {
		case map[string]any:
			at = v[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(v) {
				return nil, false
			}
			at = v[i]
		default:
			return nil, false
		}
	}
	m, ok := at.(map[string]any)
	return m, ok
}
