package manual

import "testing"

// A name that begins inside the name before it is part of that one, and
// the text is cut at no place twice.
func TestMentionsDoNotOverlap(t *testing.T) {
	known := func(name string) (Page, bool) {
		return Page{Name: name}, name == "corbie manual" || name == "run show"
	}
	text := "see corbie manual corbie manual run show"
	found := mentions(text, known)
	for i := 1; i < len(found); i++ {
		if found[i].at < found[i-1].end {
			t.Fatalf("%q is named at %d, inside %q, which ends at %d", found[i].name, found[i].at, found[i-1].name, found[i-1].end)
		}
	}
}
