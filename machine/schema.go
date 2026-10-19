package machine

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// A definition's input, a tool's arguments and an agent's reply are each
// given by a JSON Schema of an object, called a shape where a definition
// writes one. One written with type: object is in full form; any other is
// the short form, each name with the type of its value.

// A ShortForm is how one place fills out the short form: what a blob is
// there, and whether a name the shape does not give is refused.
type ShortForm struct {
	Blob   map[string]any
	Closed bool
}

// InputForm is the short form of a definition's input. A blob there is
// the blob itself, and the input may give names its shape does not.
var InputForm = ShortForm{Blob: map[string]any{"type": "object", "required": []any{"$blob"}}}

// valueTypes are the types a name may have in the short form, and what each is
// in full form. A blob's is the short form's own.
var valueTypes = map[string]map[string]any{
	"string":  {"type": "string"},
	"number":  {"type": "number"},
	"boolean": {"type": "boolean"},
	"list":    {"type": "array"},
	"map":     {"type": "object"},
	"blob":    nil,
	"any":     {},
}

var typeNames = []string{"string", "number", "boolean", "list", "map", "blob", "any"}

// Full gives a shape in full form. A short form that names a type there is
// not is refused, saying so.
func (sf ShortForm) Full(shape map[string]any) (map[string]any, error) {
	if shape["type"] == "object" {
		return shape, nil
	}
	properties := map[string]any{}
	var required []any
	names := make([]string, 0, len(shape))
	for name := range shape {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		text, ok := shape[name].(string)
		if !ok {
			return nil, fmt.Errorf("%q: a type is written as a word (the types are %s; add ? for one that may be left out), or the shape is written in full as JSON Schema with type: object", name, strings.Join(typeNames, ", "))
		}
		kind, optional := strings.CutSuffix(text, "?")
		full, known := valueTypes[kind]
		if !known {
			return nil, fmt.Errorf("%q: %q is not a type (the types are %s; add ? for one that may be left out)", name, text, strings.Join(typeNames, ", "))
		}
		if kind == "blob" {
			full = sf.Blob
		}
		property := map[string]any{}
		for k, v := range full {
			property[k] = v
		}
		properties[name] = property
		if !optional {
			required = append(required, name)
		}
	}
	out := map[string]any{"type": "object", "properties": properties}
	if sf.Closed {
		out["additionalProperties"] = false
	}
	if required != nil {
		out["required"] = required
	}
	return out, nil
}

// at is where a shape is said to live, so that a $ref outside it has an
// address to be refused by.
const at = "https://shape.invalid/shape.json"

// CompileShape reads a shape in full form as JSON Schema. A $ref resolves
// only within the shape: the check runs on the client and again on the
// controller, and neither reads a file or connects anywhere for one.
func CompileShape(shape map[string]any) (*jsonschema.Schema, error) {
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

// Fits checks a value against a shape, and says each place in it that
// does not fit and what is wrong there.
func Fits(schema *jsonschema.Schema, v any) error {
	return wrong(schema.Validate(v))
}

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

// Input gives what an instance of a definition is started with: the input
// it was given, with each default its shape gives filled in, checked
// against the shape. A definition with no input: takes any input as it is.
func Input(def *Definition, given any) (any, error) {
	if def.Input == nil {
		return given, nil
	}
	schema, err := CompileShape(def.Input)
	if err != nil {
		return given, err
	}
	if given == nil {
		given = map[string]any{}
	}
	filled := withDefaults(schema, given)
	if err := Fits(schema, filled); err != nil {
		return given, fmt.Errorf("the input does not fit the definition's input: %v", err)
	}
	return filled, nil
}

// withDefaults gives a value with each name missing from an object filled
// with the default its shape gives, in the value and in everything reached
// through properties and items. A default filled in is filled the same way.
func withDefaults(s *jsonschema.Schema, v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for name, item := range v {
			out[name] = item
		}
		for name, sub := range s.Properties {
			if _, given := out[name]; !given && sub.Default != nil {
				out[name] = copied(*sub.Default)
			}
			if item, given := out[name]; given {
				out[name] = withDefaults(sub, item)
			}
		}
		return out
	case []any:
		items := itemsOf(s)
		out := slices.Clone(v)
		for i := len(s.PrefixItems); items != nil && i < len(out); i++ {
			out[i] = withDefaults(items, out[i])
		}
		return out
	}
	return v
}

// itemsOf gives the one schema that every item of a list has, if the
// shape gives one.
func itemsOf(s *jsonschema.Schema) *jsonschema.Schema {
	if s.Items2020 != nil {
		return s.Items2020
	}
	items, _ := s.Items.(*jsonschema.Schema)
	return items
}

// copied gives a value with its own mappings and lists, so that filling
// it in leaves the definition as it was.
func copied(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = copied(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = copied(item)
		}
		return out
	}
	return v
}

// misplacedDefaults says what is wrong with the defaults of a shape: a
// default where it is not filled in, and one that does not fit its own
// place. A default is filled in only in the shape itself and in what
// properties and items reach from it.
func misplacedDefaults(root *jsonschema.Schema) []string {
	filled := map[*jsonschema.Schema]bool{}
	var follow func(s *jsonschema.Schema)
	follow = func(s *jsonschema.Schema) {
		if s == nil || filled[s] {
			return
		}
		filled[s] = true
		for _, sub := range s.Properties {
			follow(sub)
		}
		follow(itemsOf(s))
	}
	follow(root)
	var problems []string
	seen := map[*jsonschema.Schema]bool{}
	var walk func(s *jsonschema.Schema)
	walk = func(s *jsonschema.Schema) {
		if s == nil || seen[s] {
			return
		}
		seen[s] = true
		place := strings.TrimPrefix(s.Location, at)
		switch {
		case s.Default == nil:
		case !filled[s]:
			problems = append(problems, fmt.Sprintf("at %s: a default is filled in only where properties and items lead from the top of the shape", place))
		case s.Validate(*s.Default) != nil:
			problems = append(problems, fmt.Sprintf("at %s: the default does not fit its own place: %v", place, wrong(s.Validate(*s.Default))))
		}
		for _, sub := range subschemas(s) {
			walk(sub)
		}
	}
	walk(root)
	sort.Strings(problems)
	return problems
}

// subschemas gives every schema a schema holds.
func subschemas(s *jsonschema.Schema) []*jsonschema.Schema {
	subs := []*jsonschema.Schema{s.Ref, s.RecursiveRef, s.Not, s.If, s.Then, s.Else, s.PropertyNames,
		s.UnevaluatedProperties, s.Contains, s.Items2020, s.UnevaluatedItems, s.ContentSchema}
	if s.DynamicRef != nil {
		subs = append(subs, s.DynamicRef.Ref)
	}
	subs = append(subs, s.AllOf...)
	subs = append(subs, s.AnyOf...)
	subs = append(subs, s.OneOf...)
	subs = append(subs, s.PrefixItems...)
	for _, sub := range s.Properties {
		subs = append(subs, sub)
	}
	for _, sub := range s.PatternProperties {
		subs = append(subs, sub)
	}
	for _, sub := range s.DependentSchemas {
		subs = append(subs, sub)
	}
	for _, sub := range s.Dependencies {
		if sub, ok := sub.(*jsonschema.Schema); ok {
			subs = append(subs, sub)
		}
	}
	for _, one := range []any{s.AdditionalProperties, s.AdditionalItems, s.Items} {
		switch one := one.(type) {
		case *jsonschema.Schema:
			subs = append(subs, one)
		case []*jsonschema.Schema:
			subs = append(subs, one...)
		}
	}
	return subs
}
