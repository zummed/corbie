package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zummed/corbie/machine"
)

// shortForm is the short form of a tool's arguments and an agent's reply.
// A blob there is the name the model was shown, and a name the shape does
// not give is refused.
var shortForm = machine.ShortForm{Blob: map[string]any{"type": "string", "format": "blob"}, Closed: true}

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
