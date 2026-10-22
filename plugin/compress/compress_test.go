package compress

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// A job no layer allows anything: the plugin needs no permission.
func newJob(t *testing.T) *plugin.Job {
	return &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test"}}},
		Scratch: t.TempDir(),
	}
}

func call(t *testing.T, name string, first any, opt plugin.Options) (any, error) {
	t.Helper()
	module, _ := plugin.Find("compress")
	return module.Calls[name].Invoke(newJob(t), "compress", name, first, opt)
}

func must(t *testing.T, name string, first any, opt plugin.Options) any {
	t.Helper()
	v, err := call(t, name, first, opt)
	if err != nil {
		t.Fatalf("compress.%s: %v", name, err)
	}
	return v
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func Test_compress_gzip_there_and_back(t *testing.T) {
	data := plugin.Bytes(strings.Repeat("bytes \x00\xff ", 1000))
	packed := must(t, "gzip", data, nil).(plugin.Bytes)
	if len(packed) >= len(data) {
		t.Fatalf("%d bytes packed to %d", len(data), len(packed))
	}
	if back := must(t, "gunzip", packed, nil).(plugin.Bytes); !bytes.Equal(back, data) {
		t.Fatal("what was unpacked is not what was packed")
	}
	// A blob goes in and comes out as any bytes do.
	blob := must(t, "gzip", plugin.NewBlob("text/plain", []byte("in a blob")), plugin.Options{"as": "blob"}).(*plugin.Blob)
	if blob.Type != "application/gzip" {
		t.Fatalf("the blob's type is %s", blob.Type)
	}
	if text := must(t, "gunzip", blob, plugin.Options{"as": "text"}); text != "in a blob" {
		t.Fatalf("gunzip gave %v", text)
	}
}

func Test_compress_unpacks_what_the_usual_tool_packed(t *testing.T) {
	// printf 'packed by the usual tool\n' | gzip -n -9
	packed, _ := hex.DecodeString("1f8b08000000000002032b484cce4e4d5148aa5428c94855282d2e4dcc5128c9cfcfe1020004221d4b19000000")
	if text := must(t, "gunzip", plugin.Bytes(packed), plugin.Options{"as": "text"}); text != "packed by the usual tool\n" {
		t.Fatalf("gunzip gave %q", text)
	}
	if _, err := call(t, "gunzip", "not gzip", nil); code(err) != plugin.BadInput {
		t.Fatalf("text that is not gzip gave %v", err)
	}
}

func Test_compress_archive_list_and_read(t *testing.T) {
	files := map[string]any{"a.txt": "one", "notes/b.bin": plugin.Bytes{0, 1, 2, 3}}
	want := []any{
		map[string]any{"name": "a.txt", "size": 3.0},
		map[string]any{"name": "notes/b.bin", "size": 4.0},
	}
	for _, format := range []string{"zip", "tar", "tar.gz"} {
		packed := must(t, "archive", files, plugin.Options{"format": format})
		if got := must(t, "list", packed, nil); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: list gave %v", format, got)
		}
		if got := must(t, "read", packed, plugin.Options{"entry": "a.txt", "as": "text"}); got != "one" {
			t.Fatalf("%s: read gave %v", format, got)
		}
		if got := must(t, "read", packed, plugin.Options{"entry": "notes/b.bin"}); !bytes.Equal(got.(plugin.Bytes), []byte{0, 1, 2, 3}) {
			t.Fatalf("%s: read gave %v", format, got)
		}
		if _, err := call(t, "read", packed, plugin.Options{"entry": "c.txt"}); code(err) != plugin.NotFound {
			t.Fatalf("%s: a file that is not there gave %v", format, err)
		}
	}
	if _, err := call(t, "archive", files, plugin.Options{"format": "rar"}); code(err) != plugin.BadInput {
		t.Fatalf("a format that is not known gave %v", err)
	}
	if _, err := call(t, "list", "neither a zip nor a tar, whatever it is", nil); code(err) != plugin.BadInput {
		t.Fatalf("something that is no archive gave %v", err)
	}
}

func Test_compress_one_entry_read_without_the_rest(t *testing.T) {
	// The second file's contents are broken; the first is still read.
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	w, _ := z.Create("good.txt")
	w.Write([]byte("good"))
	w, _ = z.Create("bad.txt")
	w.Write([]byte(strings.Repeat("something that packs ", 100)))
	z.Close()
	data := out.Bytes()
	at := bytes.Index(data, []byte("bad.txt")) + len("bad.txt")
	data[at+3] ^= 0xff
	if got := must(t, "read", plugin.Bytes(data), plugin.Options{"entry": "good.txt", "as": "text"}); got != "good" {
		t.Fatalf("read gave %v", got)
	}
	if _, err := call(t, "read", plugin.Bytes(data), plugin.Options{"entry": "bad.txt"}); code(err) != plugin.BadInput {
		t.Fatalf("a broken file gave %v", err)
	}
}

func Test_blob_archive_item_name_cannot_leave_its_directory(t *testing.T) {
	for _, name := range []string{"/etc/passwd", "../up.txt", "a/../../up.txt", `..\up.txt`, `\\server\share`} {
		for _, format := range []string{"zip", "tar"} {
			if _, err := call(t, "archive", map[string]any{name: "x"}, plugin.Options{"format": format}); code(err) != plugin.BadInput {
				t.Fatalf("writing %q into a %s gave %v", name, format, err)
			}
		}
		var zipped, tarred bytes.Buffer
		z := zip.NewWriter(&zipped)
		w, _ := z.Create(name)
		w.Write([]byte("x"))
		z.Close()
		tw := tar.NewWriter(&tarred)
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1})
		tw.Write([]byte("x"))
		tw.Close()
		for format, data := range map[string][]byte{"zip": zipped.Bytes(), "tar": tarred.Bytes()} {
			if _, err := call(t, "list", plugin.Bytes(data), nil); code(err) != plugin.BadInput {
				t.Fatalf("listing a %s with %q gave %v", format, name, err)
			}
			if _, err := call(t, "read", plugin.Bytes(data), plugin.Options{"entry": name}); code(err) != plugin.BadInput {
				t.Fatalf("reading %q from a %s gave %v", name, format, err)
			}
		}
	}
	// A name with two dots that is not a step up is a name like any other.
	must(t, "archive", map[string]any{"notes..txt": "x", "a/..b/c": "y"}, nil)
}

func Test_compress_only_files_are_entries(t *testing.T) {
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	tw.WriteHeader(&tar.Header{Name: "directory/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.WriteHeader(&tar.Header{Name: "directory/file.txt", Mode: 0o644, Size: 2})
	tw.Write([]byte("hi"))
	tw.Close()
	want := []any{map[string]any{"name": "directory/file.txt", "size": 2.0}}
	if got := must(t, "list", plugin.Bytes(out.Bytes()), nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("list gave %v", got)
	}
	if _, err := call(t, "read", plugin.Bytes(out.Bytes()), plugin.Options{"entry": "link"}); code(err) != plugin.NotFound {
		t.Fatalf("reading a link gave %v", err)
	}
}

func Test_compress_unknown_option(t *testing.T) {
	_, err := call(t, "gzip", "x", plugin.Options{"levle": 9.0})
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"levle"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
}

// kept is a job's channel that holds one blob as the store would, and
// counts what is fetched of it.
type kept struct {
	plugin.Channel
	contents []byte
	fetched  int64
}

func (k *kept) ReadBlob(ref blob.Ref, from, length int64) ([]byte, error) {
	part := k.contents[from:min(from+length, int64(len(k.contents)))]
	k.fetched += int64(len(part))
	return part, nil
}
func (*kept) Emit(string, any) error                               { return nil }
func (*kept) Variable(string) (string, bool, error)                { return "", false, nil }
func (*kept) WriteBlob(string, int64, io.Reader) (blob.Ref, error) { return blob.Ref{}, nil }
func (*kept) Define(string, map[string]string) (blob.Ref, error)   { return blob.Ref{}, nil }

func Test_blob_item_read_on_demand(t *testing.T) {
	// An archive of ten files of four megabytes each, none of which packs
	// smaller, lies in the store.
	var packed bytes.Buffer
	z := zip.NewWriter(&packed)
	want := make([]byte, 4<<20)
	for i := range 10 {
		w, _ := z.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("part%d.bin", i), Method: zip.Store})
		file := make([]byte, 4<<20)
		rand.Read(file)
		if i == 3 {
			want = file
		}
		w.Write(file)
	}
	z.Close()
	store := &kept{contents: packed.Bytes()}
	job := newJob(t)
	job.Channel = store
	module, _ := plugin.Find("compress")
	archive := plugin.StoredBlob(blob.Ref{Type: "application/zip", Size: int64(packed.Len())})

	// Listing it fetches its list of contents, and one file its own bytes:
	// neither fetches the archive.
	listed, err := module.Calls["list"].Invoke(job, "compress", "list", archive, nil)
	if err != nil || len(listed.([]any)) != 10 || store.fetched > 5<<20 {
		t.Fatalf("listing gave %v, %v, and fetched %d bytes of %d", listed, err, store.fetched, packed.Len())
	}
	store.fetched = 0
	got, err := module.Calls["read"].Invoke(job, "compress", "read", archive, plugin.Options{"entry": "part3.bin"})
	if err != nil || !bytes.Equal(got.(plugin.Bytes), want) {
		t.Fatalf("reading one file: %v", err)
	}
	if store.fetched > int64(packed.Len())/2 {
		t.Fatalf("one file of ten was read by fetching %d bytes of %d", store.fetched, packed.Len())
	}
}

// What is unpacked is held whole only up to a size; as a blob it is written
// into the scratch directory as it is unpacked, whatever its size.
func TestLargeAnswerGoesIntoABlobAsItIsMade(t *testing.T) {
	var packed bytes.Buffer
	z := gzip.NewWriter(&packed)
	zeros := make([]byte, 1<<20)
	for range 70 {
		z.Write(zeros)
	}
	z.Close()
	job := newJob(t)
	t.Cleanup(job.End)
	module, _ := plugin.Find("compress")
	_, err := module.Calls["gunzip"].Invoke(job, "compress", "gunzip", plugin.Bytes(packed.Bytes()), nil)
	if err == nil || !strings.Contains(err.Error(), `ask for it as a blob, with { as: "blob" }`) {
		t.Fatalf("seventy megabytes held whole gave %v", err)
	}
	got, err := module.Calls["gunzip"].Invoke(job, "compress", "gunzip", plugin.Bytes(packed.Bytes()), plugin.Options{"as": "blob"})
	if err != nil {
		t.Fatal(err)
	}
	if b := got.(*plugin.Blob); b.Size != 70<<20 {
		t.Fatalf("the blob is %d bytes", b.Size)
	}
	files, _ := os.ReadDir(job.Scratch)
	if len(files) != 1 {
		t.Fatalf("the scratch directory holds %d files", len(files))
	}
}
