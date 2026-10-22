package permission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeptKinds(t *testing.T) {
	for _, c := range []struct {
		entry, kind, value string
		want               bool
	}{
		{"kept.read:notes/**", KeptRead, "notes/tea", true},
		{"kept.read:notes/**", KeptRead, "notes/a/b", true},
		{"kept.read:notes/**", KeptRead, "notes", true},
		{"kept.read:notes/**", KeptRead, "other/tea", false},
		{"kept.read:notes/**", KeptWrite, "notes/tea", false},
		{"kept.write:notes/tea", KeptWrite, "notes/tea", true},
		{"kept.write:notes/tea", KeptRead, "notes/tea", false},
		{"kept.read:notes/*", KeptRead, "notes/a/b", false},
		{"kept.read:*/docs/*.md", KeptRead, "house/docs/boiler.md", true},
		{"kept.read:**/tea", KeptRead, "a/b/tea", true},
		{"kept", KeptRead, "a/b", true},
		{"kept", KeptWrite, "a/b", true},
		{"all", KeptWrite, "a/b", true},
		{"disk", KeptRead, "a/b", false},
		{"fs.read:/**", KeptRead, "a/b", false},
	} {
		if got := (&Set{}).matches(c.entry, c.kind, c.value); got != c.want {
			t.Errorf("%s against %s:%s is %v, want %v", c.entry, c.kind, c.value, got, c.want)
		}
	}
	for _, entry := range []string{"kept.read:notes/**", "kept.write:notes/tea", "kept"} {
		if err := CheckWritten(entry); err != nil {
			t.Errorf("%s: %v", entry, err)
		}
	}
	if err := CheckWritten("kept.delete:notes"); err == nil || !strings.Contains(err.Error(), "kept.read and kept.write") {
		t.Errorf("a kind that is not one of the eight: %v", err)
	}
	// A listed kept.write holds a job that allows any kept.write scope
	// inside it, or the group.
	for entry, want := range map[string]bool{"kept.write:notes/**": true, "kept": true, "all": true, "kept.read:notes/**": false, "disk": false} {
		if got := Declares([]string{entry}, []string{"kept.write:**"}); got != want {
			t.Errorf("%s declares kept.write:**: %v, want %v", entry, got, want)
		}
	}
}

// A kept scope is matched as it is written: no link is followed, it is not
// in the scratch directory, and its case counts on every system.
func Test_perm_kept_scope_is_not_a_file(t *testing.T) {
	if (&Set{}).matches("kept.read:notes/**", KeptRead, "NOTES/tea") || (&Set{}).matches("kept.read:notes/tea", KeptRead, "notes/TEA") {
		t.Fatal("a kept scope matched a path in another case")
	}
	scratch := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(scratch, "notes")); err != nil {
		t.Skip("no link can be made here:", err)
	}
	s := &Set{Layers: []Layer{allow("kept.read:notes/**", "fs.read:notes/**")}, Scratch: scratch}
	if r := s.Check(KeptRead, "notes/tea"); r != nil {
		t.Fatalf("a kept path was read as a file: %v", r)
	}
	if got := s.Used(); len(got) != 1 || got[0] != "kept.read:notes/tea" {
		t.Fatalf("what was used is noted as %v", got)
	}
	// The file the link leads to is allowed by the fs.read entry, and its
	// path allows nothing kept.
	if s.Check(Read, filepath.ToSlash(filepath.Join(elsewhere, "tea"))) != nil {
		t.Fatal("the fs.read entry was not resolved through the link")
	}
	if s.Check(KeptRead, strings.TrimPrefix(filepath.ToSlash(filepath.Join(elsewhere, "tea")), "/")) == nil {
		t.Fatal("a kept scope was resolved through a link on the disk")
	}
}

// One rule says what a kept path is, in a call, in a scope and in the
// store.
func Test_perm_kept_path_one_rule(t *testing.T) {
	long := strings.Repeat("a", 256)
	for _, path := range []string{"notes", "notes/tea", "house/docs/boiler.md", "a-b_c/1.2", long} {
		if err := CheckKeptPath(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if err := CheckWritten("kept.read:" + path); err != nil {
			t.Errorf("kept.read:%s: %v", path, err)
		}
	}
	for _, path := range []string{"", "/notes", "notes/", "notes//tea", ".notes", "notes/.tea", "Notes", "notes/Tea",
		"no tes", "notes\\tea", "notes=", "notes:tea", "notes/*", "**", "é", long + "a"} {
		if CheckKeptPath(path) == nil {
			t.Errorf("%q passed as a kept path", path)
		}
	}
	// In a scope, * stands inside a part and ** for whole parts.
	for _, scope := range []string{"notes/*", "notes/**", "**", "*/docs/*.md", "a/**/b", "${input.dir}/**"} {
		if err := CheckWritten("kept.write:" + scope); err != nil {
			t.Errorf("kept.write:%s: %v", scope, err)
		}
	}
	for _, scope := range []string{"Notes/**", "notes/a**", "/notes/**", "notes/../x", "notes/", long + "/**"} {
		if CheckWritten("kept.write:"+scope) == nil {
			t.Errorf("kept.write:%s passed as a kept scope", scope)
		}
	}
}

// A job is sent the key of the deepest kept path below which lies
// everything every layer allows of kept values.
func TestKeptBelow(t *testing.T) {
	for _, c := range []struct {
		layers [][]string
		path   string
		ok     bool
	}{
		{[][]string{{"kept.read:notes/**"}, {"all"}}, "notes", true},
		{[][]string{{"kept.read:notes/tea"}, {"kept"}}, "notes/tea", true},
		{[][]string{{"kept.read:house/docs/**", "kept.write:house/notes/*"}, {"all"}}, "house", true},
		{[][]string{{"kept.read:house/**"}, {"kept.read:house/docs/**"}, {"all"}}, "house/docs", true},
		{[][]string{{"kept.read:*/docs/**"}, {"kept.read:house/**"}}, "house", true},
		{[][]string{{"kept.read:**"}, {"all"}}, "", true},
		{[][]string{{"all"}, {"all"}}, "", true},
		{[][]string{{"kept.read:house/**"}, {"kept.read:garden/**"}}, "", false},
		{[][]string{{"kept.read:house/**"}, {"disk"}}, "", false},
		{[][]string{{"network", "fs.read:/srv/**"}, {"all"}}, "", false},
		{[][]string{{"kept.read:${input.dir}/**"}, {"all"}}, "", false},
		{nil, "", false},
	} {
		var layers []Layer
		for _, entries := range c.layers {
			layers = append(layers, allow(entries...))
		}
		if path, ok := KeptBelow(layers); path != c.path || ok != c.ok {
			t.Errorf("%v: %q %v, want %q %v", c.layers, path, ok, c.path, c.ok)
		}
	}
	// A ceiling of all that denies a kind at every kept path allows nothing
	// of that kind.
	for _, c := range []struct {
		deny []string
		path string
		ok   bool
	}{
		{[]string{"kept"}, "", false},
		{[]string{"all"}, "", false},
		{[]string{"kept.read:**", "kept.write:**"}, "", false},
		{[]string{"kept.read:**"}, "notes", true},
		{[]string{"kept.read:notes/secret/**"}, "notes", true},
	} {
		layers := []Layer{allow("kept.read:notes/**", "kept.write:notes/**"), {Allow: []string{"all"}, Deny: c.deny}}
		if path, ok := KeptBelow(layers); path != c.path || ok != c.ok {
			t.Errorf("under all denying %v: %q %v, want %q %v", c.deny, path, ok, c.path, c.ok)
		}
	}
}
