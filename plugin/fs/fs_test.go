package fs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// kept is a job's channel that holds one blob as the store would, and
// notes the most that was fetched of it at once.
type kept struct {
	size    int64
	largest int64
	fetches int
}

func (k *kept) ReadBlob(ref blob.Ref, from, length int64) ([]byte, error) {
	length = min(length, k.size-from)
	k.largest = max(k.largest, length)
	k.fetches++
	return make([]byte, length), nil
}
func (*kept) Emit(string, any) error                               { return nil }
func (*kept) Variable(string) (string, bool, error)                { return "", false, nil }
func (*kept) WriteBlob(string, int64, io.Reader) (blob.Ref, error) { return blob.Ref{}, nil }
func (*kept) Define(string, map[string]string) (blob.Ref, error)   { return blob.Ref{}, nil }

func Test_blob_large_never_held_whole(t *testing.T) {
	const size = 90 << 20
	store := &kept{size: size}
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), Channel: store,
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"disk"}}}}}
	job.Perms.Scratch = job.Scratch
	t.Cleanup(job.End)
	module, _ := plugin.Find("fs")
	large := plugin.StoredBlob(blob.Ref{Type: "application/octet-stream", Size: size})

	// A blob larger than is ever held whole goes into a file a part at a
	// time: in a bucket, a few requests, and never one for all of it.
	if _, err := module.Calls["write"].Invoke(job, "fs", "write", "in.bin", plugin.Options{"blob": large}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(job.Scratch, "in.bin"))
	if err != nil || info.Size() != size {
		t.Fatalf("the file is %v, %v", info, err)
	}
	if store.largest > 8<<20 || store.fetches > 40 {
		t.Fatalf("ninety megabytes were fetched in %d parts, the largest of %d bytes", store.fetches, store.largest)
	}
	// Read whole, to be handed to the script, it is refused.
	if _, err := large.All(job); err == nil {
		t.Fatal("a blob too large to hold was read whole")
	}
	// And the file, as a blob, is read where it lies.
	made, err := module.Calls["read"].Invoke(job, "fs", "read", "in.bin", plugin.Options{"as": "blob"})
	if err != nil || made.(*plugin.Blob).Size != size {
		t.Fatalf("the file as a blob: %v, %v", made, err)
	}
}
