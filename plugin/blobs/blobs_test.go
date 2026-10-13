package blobs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// kept is a job's channel that holds one blob as the store would, and
// counts what is fetched of it.
type kept struct {
	contents string
	fetched  int
}

func (k *kept) ReadBlob(ref blob.Ref, from, length int64) ([]byte, error) {
	part := k.contents[from:min(from+length, int64(len(k.contents)))]
	k.fetched += len(part)
	return []byte(part), nil
}
func (*kept) Emit(string, any) error                               { return nil }
func (*kept) Variable(string) (string, bool, error)                { return "", false, nil }
func (*kept) WriteBlob(string, int64, io.Reader) (blob.Ref, error) { return blob.Ref{}, nil }
func (*kept) Define(string, map[string]string) (blob.Ref, error)   { return blob.Ref{}, nil }

func newJob(channel plugin.Channel) *plugin.Job {
	return &plugin.Job{Context: context.Background(), Channel: channel,
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test"}}}}
}

// walk gives the items of contents of a type, from an item and no more
// than a count of them, each as text.
func walk(t *testing.T, typ, contents string, opt plugin.Options) ([]string, error) {
	t.Helper()
	module, _ := plugin.Find("blobs")
	job := newJob(nil)
	walked, err := module.Calls["items"].Invoke(job, "blobs", "items", plugin.NewBlob(typ, []byte(contents)), opt)
	if err != nil {
		return nil, err
	}
	var items []string
	for {
		step, err := walked.(*plugin.Object).Calls["next"].Do(job, nil, nil)
		if err != nil {
			return items, err
		}
		if step.(map[string]any)["done"] == true {
			return items, nil
		}
		items = append(items, fmt.Sprint(step.(map[string]any)["value"]))
	}
}

// What a blob's items are depends on its type: the lines of a text, the
// rows of a table, the entries of a list.
func TestItemsOfEachType(t *testing.T) {
	for _, c := range []struct{ typ, contents, want string }{
		{"text/plain", "one\r\ntwo\n\nfour", "one|two||four"},
		{"text/plain", "\xEF\xBB\xBFone\ntwo\n", "one|two"},
		{"text/plain", "", ""},
		{"text/csv", "name,n\na,1\n\"b, c\",2\n", "map[n:1 name:a]|map[n:2 name:b, c]"},
		{"text/csv", "name,n\n", ""},
		{"text/tab-separated-values", "name\tn\na\t1\n", "map[n:1 name:a]"},
		{"application/json", `[1, "two", {"three": 3}, [4]]`, "1|two|map[three:3]|[4]"},
		{"application/json", `[]`, ""},
		{"application/json", `{"only": "one"}`, "map[only:one]"},
		{"application/jsonl", "{\"a\":1}\n\n{\"a\":2}\n", "map[a:1]|map[a:2]"},
	} {
		got, err := walk(t, c.typ, c.contents, nil)
		if err != nil || strings.Join(got, "|") != c.want {
			t.Fatalf("%s %q gave %q, %v; want %q", c.typ, c.contents, strings.Join(got, "|"), err, c.want)
		}
	}
	// A part of them, by where it starts and how many.
	if got, _ := walk(t, "text/plain", "a\nb\nc\nd\ne\n", plugin.Options{"from": 1.0, "count": 2.0}); strings.Join(got, "|") != "b|c" {
		t.Fatalf("two items from the second gave %q", got)
	}
	if got, _ := walk(t, "text/plain", "a\nb\n", plugin.Options{"from": 5.0}); len(got) != 0 {
		t.Fatalf("items from past the end gave %q", got)
	}
	// A type that is not text, a table or a list has none, and says so.
	if _, err := walk(t, "image/png", "\x89PNG", nil); err == nil || !strings.Contains(err.Error(), "has no items") {
		t.Fatalf("an image's items gave %v", err)
	}
	if _, err := walk(t, "application/json", `[1, 2`, nil); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("a list cut short gave %v", err)
	}
}

func Test_blob_table_row_of_wrong_length_fails(t *testing.T) {
	for _, table := range []string{"name,n\na,1\nb\n", "name,n\na,1\nb,2,3\n"} {
		got, err := walk(t, "text/csv", table, nil)
		if err == nil || !strings.Contains(err.Error(), "the table cannot be read") || len(got) != 1 {
			t.Fatalf("%q gave %q, %v", table, got, err)
		}
	}
}

// A large blob's items are walked while only a part of it is held: what
// was fetched follows how far the walk has got, and counting is one pass.
func TestItemsAreWalkedAPartAtATime(t *testing.T) {
	var table strings.Builder
	table.WriteString("n,word\n")
	for i := range 1_500_000 {
		fmt.Fprintf(&table, "%d,word-%d\n", i, i)
	}
	store := &kept{contents: table.String()}
	size := len(store.contents)
	job := newJob(store)
	module, _ := plugin.Find("blobs")
	large := plugin.StoredBlob(blob.Ref{Type: "text/csv", Size: int64(size)})

	walked, err := module.Calls["items"].Invoke(job, "blobs", "items", large, plugin.Options{"count": 3.0})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := walked.(*plugin.Object).Calls["next"].Do(job, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if store.fetched >= size/2 {
		t.Fatalf("three items of a blob of %d bytes were read by fetching %d", size, store.fetched)
	}
	store.fetched = 0
	n, err := module.Calls["count"].Invoke(job, "blobs", "count", large, nil)
	if err != nil || n != 1_500_000.0 || store.fetched != size {
		t.Fatalf("counting gave %v, %v, having fetched %d of %d bytes", n, err, store.fetched, size)
	}
}

// One item may be no larger than the part that is held while a blob is
// walked: a larger one fails the walk, saying so, and is not held.
func Test_blob_item_too_large_fails_the_walk(t *testing.T) {
	long := "first\n" + strings.Repeat("x", maxItem+(1<<20)) + "\nlast\n"
	got, err := walk(t, "text/plain", long, nil)
	if err == nil || !strings.Contains(err.Error(), "too large to be an item") || len(got) != 1 {
		t.Fatalf("a line of five megabytes gave %d items, %v", len(got), err)
	}
	got, err = walk(t, "application/json", `["first", "`+strings.Repeat("x", maxItem+(1<<20))+`"]`, nil)
	if err == nil || !strings.Contains(err.Error(), "too large to be an item") || len(got) != 1 {
		t.Fatalf("an entry of five megabytes gave %d items, %v", len(got), err)
	}
}
