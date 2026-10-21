package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

// kinds gives a fresh store of each kind that needs nothing outside the
// test. The bucket is a stand-in in this process, and its key is in the
// environment as a party's would be. The served directory is served in this
// process and opened as its owner.
func kinds(t *testing.T) map[string]Store {
	directory, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	memory, err := Open("memory:")
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeS3(t)
	key := fake.newKey(Party{Kind: Owner})
	t.Setenv("AWS_ACCESS_KEY_ID", key.id)
	t.Setenv("AWS_SECRET_ACCESS_KEY", key.secret)
	t.Setenv("AWS_SESSION_TOKEN", "")
	bucket, err := Open(fake.address())
	if err != nil {
		t.Fatal(err)
	}
	served := newServedDirectory(t).as(Party{Kind: Owner, Name: "owner"})
	return map[string]Store{"directory": directory, "memory": memory, "bucket": bucket, "served": served}
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func read(t *testing.T, s Store, name string) (string, Mark) {
	t.Helper()
	value, mark, err := s.Read(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return string(value), mark
}

// TestNamedValues is the one set of checks every kind of store passes.
func TestNamedValues(t *testing.T) {
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) { checkNamedValues(t, s) })
	}
}

func checkNamedValues(t *testing.T, s Store) {
	ok := must[bool](t)

	// A name never written is not there.
	if _, mark := read(t, s, "a/never"); mark != None {
		t.Fatal("a name never written is there")
	}

	// Overwrite writes whatever is there.
	ok(s.Write(ctx, "a/x", []byte("one"), Overwrite))
	ok(s.Write(ctx, "a/x", []byte("two"), Overwrite))
	if v, _ := read(t, s, "a/x"); v != "two" {
		t.Fatalf("overwrite left %q", v)
	}

	// Only-if-new leaves what is there and says so.
	if !ok(s.Write(ctx, "a/new", []byte("first"), OnlyIfNew)) {
		t.Fatal("only-if-new on a free name did not write")
	}
	if ok(s.Write(ctx, "a/new", []byte("second"), OnlyIfNew)) {
		t.Fatal("only-if-new wrote over a value")
	}
	if v, _ := read(t, s, "a/new"); v != "first" {
		t.Fatalf("only-if-new left %q", v)
	}

	// Only-if-unchanged: of two writers that read one value, one writes.
	_, mark := read(t, s, "a/x")
	if !ok(s.Write(ctx, "a/x", []byte("three"), OnlyIfUnchanged(mark))) {
		t.Fatal("only-if-unchanged did not write an unchanged value")
	}
	if ok(s.Write(ctx, "a/x", []byte("four"), OnlyIfUnchanged(mark))) {
		t.Fatal("only-if-unchanged wrote over a changed value")
	}
	if v, _ := read(t, s, "a/x"); v != "three" {
		t.Fatalf("only-if-unchanged left %q", v)
	}
	// A writer whose value was deleted after it read is told "changed".
	_, mark = read(t, s, "a/x")
	ok(s.Delete(ctx, "a/x", Overwrite))
	if ok(s.Write(ctx, "a/x", []byte("five"), OnlyIfUnchanged(mark))) {
		t.Fatal("only-if-unchanged wrote a deleted value back")
	}

	// Deleting: outright, twice, and only-if-unchanged.
	if _, mark := read(t, s, "a/x"); mark != None {
		t.Fatal("a deleted name is there")
	}
	if !ok(s.Delete(ctx, "a/x", Overwrite)) {
		t.Fatal("deleting nothing is not done")
	}
	ok(s.Write(ctx, "a/y", []byte("old"), Overwrite))
	_, mark = read(t, s, "a/y")
	ok(s.Write(ctx, "a/y", []byte("newer"), Overwrite))
	if ok(s.Delete(ctx, "a/y", OnlyIfUnchanged(mark))) {
		t.Fatal("deleted a changed value only-if-unchanged")
	}
	if v, mark := read(t, s, "a/y"); v != "newer" {
		t.Fatalf("the newer value is %q", v)
	} else if !ok(s.Delete(ctx, "a/y", OnlyIfUnchanged(mark))) {
		t.Fatal("did not delete an unchanged value")
	}

	// Listing: every name under a path, in order, and nothing else.
	for _, name := range []string{"p/b", "q/a", "p/c/d", "p/a", "pp/a"} {
		ok(s.Write(ctx, name, []byte(name), OnlyIfNew))
	}
	names := must[[]string](t)(Names(s.List(ctx, "p")))
	if want := []string{"p/a", "p/b", "p/c/d"}; !slices.Equal(names, want) {
		t.Fatalf("listing gave %v, want %v", names, want)
	}
	// Each name is listed with the mark a read of it gives, which an
	// only-if-unchanged write takes, and which is another once the value is
	// rewritten.
	listed := must[[]Listed](t)(s.List(ctx, "p"))
	for _, l := range listed {
		if _, mark := read(t, s, l.Name); l.Mark != mark {
			t.Fatalf("%s is listed with the mark %q and read with %q", l.Name, l.Mark, mark)
		}
	}
	if first := must[[]Listed](t)(s.First(ctx, "p", 1)); len(first) != 1 || first[0] != listed[0] {
		t.Fatalf("the first under p is %v, and the listing gave %v", first, listed[0])
	}
	if !ok(s.Write(ctx, "p/a", []byte("p/a again"), OnlyIfUnchanged(listed[0].Mark))) {
		t.Fatal("only-if-unchanged with the mark a listing gave did not write")
	}
	again := must[[]Listed](t)(s.List(ctx, "p"))
	if _, mark := read(t, s, "p/a"); again[0].Mark != mark || mark == listed[0].Mark || !slices.Equal(again[1:], listed[1:]) {
		t.Fatalf("with p/a rewritten the listing gave %v, and before %v", again, listed)
	}
	// The first names under a path, in order, and no more than asked for.
	for _, name := range []string{"w/20261010T120000.000Z/c/i/a", "w/20261010T110000.000Z/c/i/b", "w/20261010T110000.000Z/c/i/a", "w/20261011T090000.000Z/c/i/a"} {
		ok(s.Write(ctx, name, []byte("\n"), OnlyIfNew))
	}
	if got := must[[]string](t)(Names(s.First(ctx, "w", 2))); !slices.Equal(got, []string{"w/20261010T110000.000Z/c/i/a", "w/20261010T110000.000Z/c/i/b"}) {
		t.Fatalf("the first two names are %v", got)
	}
	if got := must[[]string](t)(Names(s.First(ctx, "w", 100))); len(got) != 4 || !slices.IsSorted(got) {
		t.Fatalf("the first hundred names are %v", got)
	}
	if got := must[[]string](t)(Names(s.First(ctx, "nothing/here", 5))); len(got) != 0 {
		t.Fatalf("the first names of an empty path are %v", got)
	}
	if names := must[[]string](t)(Names(s.List(ctx, "nothing/here"))); len(names) != 0 {
		t.Fatalf("listing an empty path gave %v", names)
	}

	// A part of a value, a value from a reader, and a copy.
	big := bytes.Repeat([]byte("0123456789"), 1000)
	if !ok(s.WriteFrom(ctx, "b/big", bytes.NewReader(big))) {
		t.Fatal("a value from a reader was not written")
	}
	if ok(s.WriteFrom(ctx, "b/big", strings.NewReader("other"))) {
		t.Fatal("a value from a reader replaced one")
	}
	part := must[[]byte](t)(s.ReadPart(ctx, "b/big", 15, 10))
	if string(part) != "5678901234" {
		t.Fatalf("a part read %q", part)
	}
	if part := must[[]byte](t)(s.ReadPart(ctx, "b/big", 9995, 10)); string(part) != "56789" {
		t.Fatalf("a part at the end read %q", part)
	}
	for _, from := range []int64{10000, 20000} {
		if part := must[[]byte](t)(s.ReadPart(ctx, "b/big", from, 10)); len(part) != 0 {
			t.Fatalf("a part past the end read %q", part)
		}
	}
	if part := must[[]byte](t)(s.ReadPart(ctx, "b/none", 0, 10)); part != nil {
		t.Fatalf("a part of nothing read %q", part)
	}
	if !ok(s.Copy(ctx, "b/big", "c/copy")) || ok(s.Copy(ctx, "b/big", "c/copy")) {
		t.Fatal("a copy is made once, only-if-new")
	}
	ok(s.Delete(ctx, "b/big", Overwrite))
	if v, _ := read(t, s, "c/copy"); v != string(big) {
		t.Fatal("a copy did not outlive its original")
	}
	if _, err := s.Copy(ctx, "b/none", "c/other"); err == nil {
		t.Fatal("a copy of nothing was made")
	}
}

// Two writers race for one name only-if-new: exactly one writes.
func TestOnlyIfNewRace(t *testing.T) {
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) {
			var wg sync.WaitGroup
			wins := make(chan int, 20)
			for i := range 20 {
				wg.Go(func() {
					done, err := s.Write(ctx, "race/name", fmt.Append(nil, i), OnlyIfNew)
					if err != nil {
						t.Error(err)
					}
					if done {
						wins <- i
					}
				})
			}
			wg.Wait()
			close(wins)
			var won []int
			for i := range wins {
				won = append(won, i)
			}
			if len(won) != 1 {
				t.Fatalf("%d writers wrote", len(won))
			}
			if v, _ := read(t, s, "race/name"); v != fmt.Sprint(won[0]) {
				t.Fatalf("the name holds %q, the winner wrote %d", v, won[0])
			}
		})
	}
}

// Many writers add one to a number only-if-unchanged: none is lost.
func TestOnlyIfUnchangedRace(t *testing.T) {
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) {
			must[bool](t)(s.Write(ctx, "n/count", []byte("0"), OnlyIfNew))
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 25 {
						for {
							value, mark, err := s.Read(ctx, "n/count")
							if err != nil {
								t.Error(err)
								return
							}
							var n int
							fmt.Sscan(string(value), &n)
							done, err := s.Write(ctx, "n/count", fmt.Append(nil, n+1), OnlyIfUnchanged(mark))
							if err != nil {
								t.Error(err)
								return
							}
							if done {
								break
							}
						}
					}
				})
			}
			wg.Wait()
			if v, _ := read(t, s, "n/count"); v != "200" {
				t.Fatalf("the count is %s, want 200", v)
			}
		})
	}
}

func Test_store_name_part_cannot_leave_its_place(t *testing.T) {
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) {
			for _, name := range []string{"", "a//b", "a/../b", "../x", "a/.", "/a", "a/", "a/.hidden", "a/b\nc"} {
				if _, err := s.Write(ctx, name, []byte("x"), Overwrite); !errors.Is(err, ErrName) {
					t.Errorf("write %q: %v", name, err)
				}
				if _, _, err := s.Read(ctx, name); !errors.Is(err, ErrName) {
					t.Errorf("read %q: %v", name, err)
				}
				if _, err := s.Delete(ctx, name, Overwrite); !errors.Is(err, ErrName) {
					t.Errorf("delete %q: %v", name, err)
				}
			}
			if _, err := Names(s.List(ctx, "a/../..")); !errors.Is(err, ErrName) {
				t.Errorf("list: %v", err)
			}
		})
	}
	for _, part := range []string{"", ".", "..", "a/b", ".x", "a\nb", "a\tb", "a\x00b"} {
		if CheckPart(part) == nil {
			t.Errorf("part %q passed", part)
		}
	}
}

func TestNotice(t *testing.T) {
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) {
			in, stop := s.Notice("ctl/inbox")
			defer stop()
			other, stopOther := s.Notice("ctl/running")
			defer stopOther()
			// A served directory tells over the wire: the asking has to reach
			// the server before the write, and the telling takes a moment.
			var wait time.Duration
			if kind == "served" {
				wait = 300 * time.Millisecond
				time.Sleep(wait)
			}
			must[bool](t)(s.Write(ctx, "ctl/inbox/c1/m1", []byte("x"), OnlyIfNew))
			got := func(ch <-chan struct{}) bool {
				select {
				case <-ch:
					return true
				case <-time.After(wait):
				}
				select {
				case <-ch:
					return true
				default:
					return false
				}
			}
			// A bucket does not tell what was written to it.
			if told := got(in); told != (kind != "bucket") {
				t.Fatalf("a write under the path: notice given is %v", told)
			}
			if got(other) {
				t.Fatal("a write elsewhere gave notice")
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for kind, s := range kinds(t) {
		t.Run(kind, func(t *testing.T) {
			if err := Check(ctx, s); err == nil {
				t.Fatal("a store with no version passed")
			}
			if err := SetUp(ctx, s); err != nil {
				t.Fatal(err)
			}
			if err := SetUp(ctx, s); err != nil {
				t.Fatal("setting up twice:", err)
			}
			must[bool](t)(s.Write(ctx, VersionName, []byte("999\n"), Overwrite))
			if err := Check(ctx, s); err == nil || !strings.Contains(err.Error(), "does not know") {
				t.Fatalf("an unknown version: %v", err)
			}
		})
	}
}
