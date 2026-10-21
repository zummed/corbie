package blob

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zummed/corbie/store"
)

var ctx = context.Background()

// counting counts the bytes of blobs a store hands over.
type counting struct {
	store.Store
	read int64
}

func (c *counting) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	part, err := c.Store.ReadPart(ctx, name, from, length)
	c.read += int64(len(part))
	return part, err
}

func memory(t *testing.T) *counting {
	s, _ := store.Open("memory:")
	return &counting{Store: s}
}

func write(t *testing.T, s store.Store, at string, contents []byte) Ref {
	t.Helper()
	ref, err := Write(ctx, s, at, "text/plain", NewKey(), bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestWriteAndRead(t *testing.T) {
	s := memory(t)
	for _, size := range []int{0, 1, 100, pieceSize - 1, pieceSize, pieceSize + 1, 3*pieceSize + 77, 4 * pieceSize} {
		contents := make([]byte, size)
		rand.Read(contents)
		copy(contents, "marker-string")
		at := fmt.Sprintf("ctl/records/c1/i1/blobs/b%d", size)
		ref := write(t, s, at, contents)
		if ref.Size != int64(size) {
			t.Fatalf("size %d: the reference says %d", size, ref.Size)
		}
		raw, _, _ := s.Read(ctx, at)
		if bytes.Contains(raw, []byte("marker-string")) {
			t.Fatal("the stored bytes show the contents")
		}
		got, err := ReadAll(ctx, s, ref, 1<<30)
		if err != nil || !bytes.Equal(got, contents) {
			t.Fatalf("size %d: read back wrong: %v", size, err)
		}
		// With another key nothing is read.
		wrong := ref
		wrong.Key = NewKey()
		if _, err := ReadAll(ctx, s, wrong, 1<<30); !errors.Is(err, ErrAltered) {
			t.Fatalf("size %d: read with another key: %v", size, err)
		}
	}
	if _, err := ReadAll(ctx, s, Ref{At: "ctl/records/c1/i1/blobs/none", Size: 5, Key: NewKey()}, 100); !errors.Is(err, ErrGone) {
		t.Fatalf("a blob that is not there: %v", err)
	}
	big := write(t, s, "ctl/records/c1/i1/blobs/big", make([]byte, 1000))
	if _, err := ReadAll(ctx, s, big, 999); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a blob over the limit: %v", err)
	}
}

// A hundred bytes from the middle of a large blob cost a small part of it.
func TestReadPart(t *testing.T) {
	s := memory(t)
	contents := make([]byte, 20*pieceSize+123)
	rand.Read(contents)
	ref := write(t, s, "ctl/records/c1/i1/blobs/big", contents)
	r, err := Open(ctx, s, ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, off := range []int64{0, 10*pieceSize - 50, 7*pieceSize + 5, int64(len(contents)) - 100} {
		s.read = 0
		part := make([]byte, 100)
		if _, err := r.ReadAt(part, off); err != nil || !bytes.Equal(part, contents[off:off+100]) {
			t.Fatalf("a part at %d: %v", off, err)
		}
		if s.read > 2*(pieceSize+tagSize) {
			t.Fatalf("a hundred bytes cost %d bytes of the store", s.read)
		}
	}
	part := make([]byte, 100)
	if n, err := r.ReadAt(part, int64(len(contents))-10); n != 10 || err != io.EOF {
		t.Fatalf("a part over the end: %d %v", n, err)
	}
}

func Test_blob_no_byte_before_its_piece_is_checked(t *testing.T) {
	s := memory(t)
	contents := bytes.Repeat([]byte("abcdefgh"), 3*pieceSize/8)
	at := "ctl/records/c1/i1/blobs/b"
	ref := write(t, s, at, contents)
	raw, _, _ := s.Read(ctx, at)
	stored := pieceSize + tagSize

	check := func(what string, changed []byte, off int64, want error) {
		t.Helper()
		s.Write(ctx, at, changed, store.Overwrite)
		r, _ := Open(ctx, s, ref)
		part := bytes.Repeat([]byte{'#'}, 64)
		if n, err := r.ReadAt(part, off); !errors.Is(err, want) || n != 0 {
			t.Fatalf("%s: %d bytes, %v", what, n, err)
		}
		if !bytes.Equal(part, bytes.Repeat([]byte{'#'}, 64)) {
			t.Fatalf("%s: bytes of the piece reached the reader", what)
		}
	}
	// One byte changed in the second piece.
	one := bytes.Clone(raw)
	one[stored+100] ^= 1
	check("one byte changed", one, pieceSize+10, ErrAltered)
	// Two pieces swapped.
	swapped := bytes.Clone(raw)
	copy(swapped[:stored], raw[stored:2*stored])
	copy(swapped[stored:], raw[:stored])
	check("pieces swapped", swapped, 10, ErrAltered)
	// A piece from another blob.
	other := write(t, s, "ctl/records/c1/i1/blobs/other", contents)
	otherRaw, _, _ := s.Read(ctx, other.At)
	foreign := bytes.Clone(raw)
	copy(foreign[:stored], otherRaw[:stored])
	check("a piece of another blob", foreign, 10, ErrAltered)
	// The untouched piece still reads.
	s.Write(ctx, at, one, store.Overwrite)
	r, _ := Open(ctx, s, ref)
	part := make([]byte, 8)
	if _, err := r.ReadAt(part, 0); err != nil || string(part) != "abcdefgh" {
		t.Fatalf("an untouched piece: %v", err)
	}
}

func Test_blob_whole_pieces_removed_from_end_refused(t *testing.T) {
	s := memory(t)
	contents := bytes.Repeat([]byte("x"), 3*pieceSize)
	at := "ctl/records/c1/i1/blobs/b"
	ref := write(t, s, at, contents)
	raw, _, _ := s.Read(ctx, at)
	for _, keep := range []int{2 * (pieceSize + tagSize), 2*(pieceSize+tagSize) + 500, pieceSize + tagSize, 0} {
		s.Write(ctx, at, raw[:keep], store.Overwrite)
		if got, err := ReadAll(ctx, s, ref, 1<<30); err == nil || got != nil {
			t.Fatalf("a blob cut to %d bytes was read as whole", keep)
		} else if !errors.Is(err, ErrCutShort) && !errors.Is(err, ErrGone) {
			t.Fatalf("a blob cut to %d bytes: %v", keep, err)
		}
	}
	// And a reference that claims fewer pieces than were written is refused
	// too: the piece it takes for the last is not marked as the last.
	s.Write(ctx, at, raw, store.Overwrite)
	short := ref
	short.Size = 2 * pieceSize
	if _, err := ReadAll(ctx, s, short, 1<<30); !errors.Is(err, ErrAltered) {
		t.Fatalf("a reference with a smaller size: %v", err)
	}
}

func Test_blob_reference_in_log_or_error_hides_key(t *testing.T) {
	ref := Ref{At: "ctl/records/c1/i1/blobs/b", Type: "text/plain", Size: 12, Key: NewKey()}
	err := fmt.Errorf("could not use %v (%s) %+v %#v %q", ref, ref, ref, ref, ref)
	for _, text := range []string{err.Error(), fmt.Sprint(ref), fmt.Sprint(&ref), fmt.Sprint([]Ref{ref}), fmt.Sprintf("%v", struct{ R Ref }{ref})} {
		if strings.Contains(text, ref.Key) {
			t.Fatalf("a printed reference shows its key: %s", text)
		}
	}
	shown, _ := json.Marshal(Show(map[string]any{"files": []any{ref.Value()}, "inner": map[string]any{"f": ref.Value()}}))
	if strings.Contains(string(shown), ref.Key) || !strings.Contains(string(shown), `"files[0]"`) || !strings.Contains(string(shown), `"inner.f"`) {
		t.Fatalf("a shown value: %s", shown)
	}
}

func Test_blob_reference_search_passes_over_lookalikes(t *testing.T) {
	ref := Ref{At: "a/b", Type: "t", Size: 1, Key: "k"}
	value := map[string]any{
		"real":     ref.Value(),
		"no key":   map[string]any{"$blob": "x", "type": "t", "size": 1.0},
		"extra":    map[string]any{"$blob": "x", "type": "t", "size": 1.0, "key": "k", "more": true},
		"wrong":    map[string]any{"$blob": 5.0, "type": "t", "size": 1.0, "key": "k"},
		"negative": map[string]any{"$blob": "x", "type": "t", "size": -1.0, "key": "k"},
		"text":     `{"$blob":"x","type":"t","size":1,"key":"k"}`,
		"list":     []any{"$blob", nil, 3.0},
	}
	refs := Refs(value)
	if len(refs) != 1 || refs[0] != ref {
		t.Fatalf("found %v", refs)
	}
	if got, ok := Find(value, "real"); !ok || got != ref {
		t.Fatal("the real reference is not found by its path")
	}
}

func definition(t *testing.T, s store.Store, instance string, inner ...Ref) Ref {
	t.Helper()
	var named []any
	for _, r := range inner {
		named = append(named, r.Local().Value())
	}
	contents, _ := json.Marshal(map[string]any{"flow": "x", "scripts": named})
	ref, err := Write(ctx, s, store.Blob("ctl", instance, NewName()), Definition, NewKey(), bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// copyAll gives a value with every blob in it copied whole into an
// instance's record, as a value is handed up.
func copyAll(s store.Store, v any, instance string) (any, error) {
	return Walk(v, func(_ string, ref Ref) (any, error) {
		copied, err := CopyWhole(ctx, s, ref, "ctl", instance)
		return copied.Value(), err
	})
}

func Test_blob_copy_up_takes_blobs_referred_to_inside_blobs(t *testing.T) {
	s := memory(t)
	child, parent := "c1/20261010T120000.000Z-child", "c1/20261010T120000.000Z-parent"
	script := write(t, s, store.Blob("ctl", child, "script"), []byte("return 1"))
	innerDef := definition(t, s, child, script)
	def := definition(t, s, child, innerDef)
	file := write(t, s, store.Blob("ctl", child, "file"), []byte("contents"))
	// A plain blob that only looks as if it named another.
	lookalike, _ := json.Marshal(map[string]any{"scripts": []any{Ref{At: "missing", Type: "t", Size: 1, Key: NewKey()}.Value()}})
	plain := write(t, s, store.Blob("ctl", child, "plain"), lookalike)

	value := map[string]any{"def": def.Value(), "files": []any{file.Value(), plain.Value()}, "n": 3.0}
	up, err := copyAll(s, value, parent)
	if err != nil {
		t.Fatal(err)
	}
	// The child's record goes.
	names, _ := store.Names(s.List(ctx, store.Record("ctl", child)))
	for _, name := range names {
		s.Delete(ctx, name, store.Overwrite)
	}
	for _, ref := range Refs(up) {
		if ref.Instance() != parent {
			t.Fatalf("a reference still points into the child: %s", ref.At)
		}
		if _, err := ReadAll(ctx, s, ref, StepLimit); err != nil {
			t.Fatal(err)
		}
	}
	upDef, _ := Find(up, "def")
	named, err := Named(ctx, s, upDef)
	if err != nil || len(named) != 2 {
		t.Fatalf("the copied definition names %d blobs: %v", len(named), err)
	}
	for _, ref := range named {
		if got, err := ReadAll(ctx, s, ref, StepLimit); err != nil || ref.Instance() != parent {
			t.Fatalf("what the definition names did not come with it: %v %s", err, got)
		}
	}
	// Done twice, it is done once.
	before, _ := store.Names(s.List(ctx, "ctl"))
	if _, err := copyAll(s, up, parent); err != nil {
		t.Fatal(err)
	}
	if after, _ := store.Names(s.List(ctx, "ctl")); len(after) != len(before) {
		t.Fatal("a second copy wrote more blobs")
	}
}

func Test_blob_directory_linked_copy_survives_original_removed(t *testing.T) {
	dir := t.TempDir()
	s, _ := store.Open(dir)
	contents := bytes.Repeat([]byte("large"), 100000)
	ref := write(t, s, store.Blob("ctl", "c1/i1", "b"), contents)
	copied, err := Copy(ctx, s, ref, "ctl", "c1/i2")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat(filepath.Join(dir, filepath.FromSlash(ref.At)))
	b, _ := os.Stat(filepath.Join(dir, filepath.FromSlash(copied.At)))
	if !os.SameFile(a, b) {
		t.Fatal("a copy in a directory is not a link")
	}
	s.Delete(ctx, ref.At, store.Overwrite)
	if got, err := ReadAll(ctx, s, copied, 1<<30); err != nil || !bytes.Equal(got, contents) {
		t.Fatalf("the copy after its original was removed: %v", err)
	}
}

func Test_blob_type_from_name_same_everywhere(t *testing.T) {
	for name, want := range map[string]string{"a.txt": "text/plain", "REPORT.PDF": "application/pdf", "x/y/data.csv": "text/csv",
		"photo.JPG": "image/jpeg", "noext": Bytes, "odd.zzz": Bytes, "archive.tar.gz": "application/gzip"} {
		if got := TypeOf(name); got != want {
			t.Errorf("%s is %s, want %s", name, got, want)
		}
	}
}

// What is wrong with a blob or its reference is told from a store that
// cannot be reached, whichever way it is wrong.
func TestFaultsOfABlobAreItsOwn(t *testing.T) {
	s := memory(t)
	real, err := Write(ctx, s, "ctl/records/c1/i1/blobs/real", Definition, NewKey(), strings.NewReader("not a definition"))
	if err != nil {
		t.Fatal(err)
	}
	s.Write(ctx, "ctl/secrets/KEY", []byte("sealed"), store.OnlyIfNew)
	for what, ref := range map[string]Ref{
		"a name that holds nothing":      {At: "ctl/records/c1/i1/blobs/none", Size: 4, Key: NewKey()},
		"a name no store can hold":       {At: "../../etc/passwd", Size: 4, Key: NewKey()},
		"a value that is not a blob":     {At: "ctl/secrets/KEY", Size: 4, Key: NewKey()},
		"a key that is not a key":        {At: "ctl/records/c1/i1/blobs/none", Type: Definition, Size: 4, Key: "x"},
		"a definition that is not one":   real,
		"a definition that is not there": {At: "ctl/records/c1/i1/blobs/none", Type: Definition, Size: 4, Key: NewKey()},
	} {
		_, err := CopyWhole(ctx, s, ref, "ctl", "c1/i2")
		if !Unusable(err) {
			t.Errorf("%s: copying gives %v", what, err)
		}
	}
	if names, _ := store.Names(s.List(ctx, "ctl/records/c1/i2")); len(names) != 0 {
		t.Errorf("copied all the same: %v", names)
	}
	if Unusable(store.ErrUnreachable) || Unusable(nil) {
		t.Error("a store that cannot be reached is taken for a blob's fault")
	}
}

// A reference written out inside a text is found where a value is shown,
// and only there; what only looks like one is left as it is.
func TestReferencesWrittenInAText(t *testing.T) {
	one := Ref{At: "ctl/records/c1/i1/blobs/one", Type: "text/plain", Size: 12, Key: NewKey()}
	odd := Ref{At: "ctl/records/c1/i1/blobs/odd", Type: `a {type} with "braces"`, Size: 3, Key: NewKey()}
	write := func(v any) string {
		text, _ := json.Marshal(v, json.Deterministic(true))
		return string(text)
	}
	lookalike := `{"$blob":"x","type":"t","size":1} and "$blob" and {"$blob":5,"type":"t","size":1,"key":"k"}`
	value := map[string]any{
		"log":  "made " + write(one.Value()) + " and { then " + write(odd.Value()) + "; " + lookalike,
		"rows": []any{"[" + write(map[string]any{"file": one.Value(), "n": 1.0}) + "]"},
	}
	shown, _ := json.Marshal(Show(value))
	if strings.Contains(string(shown), one.Key) || strings.Contains(string(shown), odd.Key) {
		t.Fatalf("a key is shown: %s", shown)
	}
	back := Show(value).(map[string]any)
	want := `made {"$blob":"log#1","size":12,"type":"text/plain"} and { then {"$blob":"log#2","size":3,"type":"a {type} with \"braces\""}; ` + lookalike
	if back["log"] != want {
		t.Fatalf("the text is shown as\n  %s\nwant\n  %s", back["log"], want)
	}
	for path, ref := range map[string]Ref{"log#1": one, "log#2": odd, "rows[0]#1": one} {
		if got, ok := Find(value, path); !ok || got != ref {
			t.Errorf("the reference shown at %s is not found by that path", path)
		}
	}
	// Where blobs are followed, kept or copied, a text is only a text.
	if refs := Refs(value); len(refs) != 0 {
		t.Fatalf("references were taken from inside texts: %v", refs)
	}
}
