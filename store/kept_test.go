package store

import (
	"bytes"
	"crypto/rand"
	"slices"
	"strings"
	"testing"
)

// A value can be kept at a path and another below it, in every kind of
// store, and each is listed under the path it was kept at.
func Test_store_kept_path_never_above_another(t *testing.T) {
	if strings.HasPrefix(Kept("ctl", "notes/tea"), Kept("ctl", "notes")+"/") {
		t.Fatal("a kept value's name is the path above another's")
	}
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) {
			for _, path := range []string{"notes", "notes/tea", "notes/tea/cup"} {
				if _, err := s.Write(ctx, Kept("ctl", path), []byte(path), Overwrite); err != nil {
					t.Fatalf("keeping %s: %v", path, err)
				}
			}
			for _, path := range []string{"notes", "notes/tea", "notes/tea/cup"} {
				if got, _ := read(t, s, Kept("ctl", path)); got != path {
					t.Errorf("the value kept at %s is %q", path, got)
				}
			}
			names := must[[]string](t)(Names(s.List(ctx, KeptList("ctl", "notes"))))
			var paths []string
			for _, name := range names {
				if path, ok := KeptPathOf("ctl", name); ok {
					paths = append(paths, path)
				}
			}
			if slices.Sort(paths); !slices.Equal(paths, []string{"notes/tea", "notes/tea/cup"}) {
				t.Errorf("below notes are %v", names)
			}
		})
	}
}

// Every value kept is encrypted with a nonce of its own: the same value
// kept twice at one path is two different values in the store, and
// neither shows what was kept.
func Test_store_kept_value_encrypted_afresh(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	value := []byte("Tea is up and cake is down.")
	first, err := SealKept(key, "notes/tea", value)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := SealKept(key, "notes/tea", value)
	if bytes.Equal(first, second) || bytes.Equal(first[:24], second[:24]) {
		t.Fatal("one value kept twice was encrypted the same way twice")
	}
	for _, sealed := range [][]byte{first, second} {
		if bytes.Contains(sealed, value) || bytes.Contains(sealed, []byte("cake")) {
			t.Fatal("a kept value shows in the store")
		}
		if got, err := OpenKept(key, "notes/tea", sealed); err != nil || !bytes.Equal(got, value) {
			t.Fatalf("a kept value read back is %q: %v", got, err)
		}
	}
}

// A kept value moved to another path, altered, or read with another key
// is refused as altered.
func Test_store_kept_value_moved_refused(t *testing.T) {
	top := make([]byte, 32)
	rand.Read(top)
	house, _ := KeptKey(top, "", "house/docs/boiler")
	garden, _ := KeptKey(top, "", "garden/docs/boiler")
	sealed, err := SealKept(house, "house/docs/boiler", []byte("Serviced in May."))
	if err != nil {
		t.Fatal(err)
	}
	altered := bytes.Clone(sealed)
	altered[len(altered)-1] ^= 1
	for what, open := range map[string]func() ([]byte, error){
		"moved to another path, with that path's key": func() ([]byte, error) { return OpenKept(garden, "garden/docs/boiler", sealed) },
		"moved to another path, with its own key":     func() ([]byte, error) { return OpenKept(house, "garden/docs/boiler", sealed) },
		"altered":           func() ([]byte, error) { return OpenKept(house, "house/docs/boiler", altered) },
		"cut short":         func() ([]byte, error) { return OpenKept(house, "house/docs/boiler", sealed[:10]) },
		"read with the top": func() ([]byte, error) { return OpenKept(top, "house/docs/boiler", sealed) },
	} {
		if _, err := open(); err == nil || !strings.Contains(err.Error(), "altered or moved") {
			t.Errorf("a kept value %s: %v", what, err)
		}
	}
}

// The key of a kept path is made from the key of any path above it, and
// from no other.
func TestKeptKeysAreMadeDownThePath(t *testing.T) {
	top := make([]byte, 32)
	rand.Read(top)
	house, _ := KeptKey(top, "", "house")
	boiler, _ := KeptKey(top, "", "house/docs/boiler")
	if got, ok := KeptKey(house, "house", "house/docs/boiler"); !ok || !bytes.Equal(got, boiler) {
		t.Fatal("the key of a path is not the one made from the path above it")
	}
	if got, ok := KeptKey(house, "house", "house"); !ok || !bytes.Equal(got, house) {
		t.Fatal("the key of a path, from itself, is not its own")
	}
	for _, path := range []string{"garden/a", "housekeeping", "top", ""} {
		if _, ok := KeptKey(house, "house", path); ok {
			t.Errorf("the key of house gave a key for %q", path)
		}
	}
	other, _ := KeptKey(top, "", "garden")
	if bytes.Equal(house, other) || bytes.Equal(house, top) {
		t.Fatal("two paths have one key")
	}
}

// A kept value of 64 MiB, encrypted, is one a served directory takes in
// place of another.
func TestServedDirectoryTakesTheLargestKeptValue(t *testing.T) {
	sealed, err := SealKept(make([]byte, 32), "notes/tea", []byte("tea"))
	if err != nil {
		t.Fatal(err)
	}
	if added := len(sealed) - len("tea"); 64<<20+added > servedValue {
		t.Fatalf("encrypting a kept value adds %d bytes, and a served directory takes %d", added, servedValue)
	}
}
