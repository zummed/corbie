package agent

import (
	"strings"
	"testing"
	"time"
)

// A shape from outside that names itself twice at each hop is read up to a
// limit, and no further.
func TestShapeThatNamesItselfIsReadToALimit(t *testing.T) {
	again := map[string]any{"$ref": "#/$defs/a"}
	shape := map[string]any{"type": "object", "$ref": "#/$defs/a",
		"$defs": map[string]any{"a": map[string]any{"anyOf": []any{again, again}}}}
	began := time.Now()
	_, err := (&stepper{st: &state{}}).blobs(shape, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "too large to read") {
		t.Errorf("the shape gave %v", err)
	}
	if time.Since(began) > 10*time.Second {
		t.Errorf("the shape took %v to read", time.Since(began))
	}
}

// A toolkit and a tool of one list whose names are shown alike are both
// taken, and the model is offered the tool once.
func TestToolkitAndToolShownAlikeAreOfferedOnce(t *testing.T) {
	s := &stepper{a: &Agent{}, st: &state{}}
	entries := []any{
		map[string]any{"kind": "toolkit", "name": "a.b", "description": "A toolkit."},
		map[string]any{"kind": "tool", "name": "a_b", "description": "A tool."},
	}
	if _, err := s.take(&opening{List: "kit", Entry: "kit"}, map[string]any{"entries": entries}); err != nil {
		t.Fatal(err)
	}
	named := 0
	for _, tool := range s.offered() {
		if tool.(map[string]any)["name"] == "kit_a_b" {
			named++
		}
	}
	if named != 1 {
		t.Errorf("the tool kit_a_b was offered %d times", named)
	}
	// A second tool shown as the first is refused, with a toolkit between them.
	entries = append(entries, map[string]any{"kind": "tool", "name": "a b", "description": "Another."})
	if _, err := s.take(&opening{List: "kit2", Entry: "kit2"}, map[string]any{"entries": entries}); err == nil || !strings.Contains(err.Error(), "which another tool has") {
		t.Errorf("two tools shown alike gave %v", err)
	}
}
