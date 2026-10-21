package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

var owner = Party{Kind: Owner}

// sees fails the test unless the fake took exactly these requests since it
// was last asked.
func (f *fakeS3) sees(what string, want map[string]int) {
	f.t.Helper()
	if got := f.took(); !maps.Equal(got, want) {
		f.t.Errorf("%s: the bucket took %v, want %v", what, got, want)
	}
}

// The signatures S3's own manual works out for its example requests.
func TestBucketSignature(t *testing.T) {
	key := bucketKey{id: "AKIAIOSFODNN7EXAMPLE", secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	empty := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	for _, c := range []struct{ target, rng, want string }{
		{"https://examplebucket.s3.amazonaws.com/test.txt", "bytes=0-9",
			"f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{"https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J", "",
			"34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
		{"https://examplebucket.s3.amazonaws.com/?lifecycle=", "",
			"fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"},
	} {
		req := must[*http.Request](t)(http.NewRequest(http.MethodGet, c.target, nil))
		req.Header.Set("X-Amz-Content-Sha256", empty)
		if c.rng != "" {
			req.Header.Set("Range", c.rng)
		}
		key.sign(req, "us-east-1", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
		if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, "Signature="+c.want) {
			t.Errorf("%s is signed %s, want the signature %s", c.target, got, c.want)
		}
	}
}

// A name may hold any characters a part may, and comes back as it went.
func TestBucketNamesOfAnyCharacters(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	names := []string{"odd/a b+c&d=e%f", "odd/é~*'()!", "odd/q?x#y", "odd/sub dir/a:b@c$d,e;f"}
	for _, name := range names {
		if done, err := b.Write(ctx, name, []byte(name), OnlyIfNew); err != nil || !done {
			t.Fatalf("write %q: %v %v", name, done, err)
		}
		if v, _ := read(t, b, name); v != name {
			t.Errorf("%q reads %q", name, v)
		}
		if v, ok := f.value(name); !ok || v != name {
			t.Errorf("%q is not under its own name in the bucket", name)
		}
		if done, err := b.Copy(ctx, name, name+" too"); err != nil || !done {
			t.Errorf("copy %q: %v %v", name, done, err)
		}
		must[bool](t)(b.Delete(ctx, name+" too", Overwrite))
	}
	slices.Sort(names)
	f.set(func() { f.page = 1 })
	if got := must[[]string](t)(Names(b.List(ctx, "odd"))); !slices.Equal(got, names) {
		t.Errorf("listed %q, want %q", got, names)
	}
}

// S3 can say all is well and then, in the answer's body, that it is not:
// that is not taken for a value written.
func TestBucketErrorInsideAnAnswer(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	b.part = 4
	must[bool](t)(b.Write(ctx, "a/x", []byte("x"), OnlyIfNew))
	f.set(func() { f.sour = 2 })
	if done, err := b.Copy(ctx, "a/x", "a/copy"); done || err == nil {
		t.Errorf("a copy that failed inside its answer: %v %v", done, err)
	}
	if done, err := b.WriteFrom(ctx, "a/parts", strings.NewReader("in parts")); done || err == nil {
		t.Errorf("an upload that failed inside its answer: %v %v", done, err)
	}
	for _, name := range []string{"a/copy", "a/parts"} {
		if _, mark := read(t, b, name); mark != None {
			t.Errorf("%s is there", name)
		}
	}
	if got := f.took(); got[fCopy] != 1 || got[fEnd] != 1 || got[fGiveUp] != 1 {
		t.Errorf("the bucket took %v", got)
	}
}

func Test_store_list_follows_pages(t *testing.T) {
	for _, prefix := range []string{"pre/fix/", ""} {
		f := newFakeS3(t)
		f.prefix = prefix
		b := f.open(owner)
		var want []string
		for i := range 2500 {
			name := fmt.Sprintf("p/%04d", i)
			want = append(want, name)
			f.objects[prefix+name] = []byte("x")
		}
		f.objects[prefix+"q/other"] = []byte("x")
		if prefix != "" {
			f.objects["elsewhere/p/0000"] = []byte("x") // in the bucket, and not in the store
		}

		// A bucket gives a thousand names at a time.
		if got := must[[]string](t)(Names(b.List(ctx, "p"))); !slices.Equal(got, want) {
			t.Fatalf("listed %d names, want %d", len(got), len(want))
		}
		f.sees("a listing of 2500 names", map[string]int{fList: 3})

		f.set(func() { f.page = 300 })
		if got := must[[]string](t)(Names(b.List(ctx, "p"))); !slices.Equal(got, want) {
			t.Fatalf("listed %d names three hundred at a time, want %d", len(got), len(want))
		}
		if got := must[[]string](t)(Names(b.List(ctx, ""))); !slices.Equal(got, append(want, "q/other")) {
			t.Fatalf("the whole store lists %d names", len(got))
		}
	}
}

func Test_store_unreachable_is_never_an_answer(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	must[bool](t)(b.Write(ctx, "a/there", []byte("x"), Overwrite))
	_, mark := read(t, b, "a/there")

	// calls makes every kind of call; each must end in an error that says
	// the store was not reached, and never in an answer.
	calls := func(why string, reached ...error) {
		t.Helper()
		check := func(what string, answered bool, err error) {
			t.Helper()
			if answered || !slices.ContainsFunc(reached, func(want error) bool { return errors.Is(err, want) }) {
				t.Errorf("%s, %s: answered %v, error %v", why, what, answered, err)
			}
		}
		value, m, err := b.Read(ctx, "a/there")
		check("read", value != nil || m != None, err)
		_, m, err = b.Read(ctx, "a/nothing")
		check("read of nothing", m != None, err)
		part, err := b.ReadPart(ctx, "a/there", 0, 1)
		check("read part", part != nil, err)
		names, err := Names(b.List(ctx, "a"))
		check("list", names != nil, err)
		done, err := b.Write(ctx, "a/there", []byte("y"), Overwrite)
		check("overwrite", done, err)
		done, err = b.Write(ctx, "a/there", []byte("y"), OnlyIfNew)
		check("only-if-new over a value", done, err)
		done, err = b.Write(ctx, "a/free", []byte("y"), OnlyIfNew)
		check("only-if-new", done, err)
		done, err = b.Write(ctx, "a/there", []byte("y"), OnlyIfUnchanged(mark))
		check("only-if-unchanged", done, err)
		done, err = b.Write(ctx, "a/there", []byte("y"), OnlyIfUnchanged("stale"))
		check("only-if-unchanged of a changed value", done, err)
		done, err = b.Delete(ctx, "a/there", Overwrite)
		check("delete", done, err)
		done, err = b.Delete(ctx, "a/there", OnlyIfUnchanged("stale"))
		check("delete only-if-unchanged", done, err)
		done, err = b.Copy(ctx, "a/there", "a/copy")
		check("copy", done, err)
		done, err = b.WriteFrom(ctx, "a/there", strings.NewReader("y"))
		check("write from a reader", done, err)
	}

	// Turned away for load, for longer than the tries go on.
	f.set(func() { f.turnAway = 1 << 30 })
	calls("turned away", ErrUnreachable)

	// An error from the bucket, or from something on the way to it, that
	// leaves open whether a conditional write went in.
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout} {
		f.set(func() { f.status = status })
		calls(fmt.Sprint("answered ", status), ErrUnreachable, ErrUnknown)
		if _, _, err := b.Read(ctx, "a/nothing"); !errors.Is(err, ErrUnreachable) {
			t.Errorf("a read answered %d: %v", status, err)
		}
	}

	// The store is back, and holds what it held: nothing was acted on.
	f.set(func() { f.turnAway = 0 })
	if v, _ := read(t, b, "a/there"); v != "x" {
		t.Fatalf("the value is %q", v)
	}
	if _, m := read(t, b, "a/free"); m != None {
		t.Fatal("a write that was turned away went in")
	}

	// No reply at all: nothing is listening.
	f.srv.Close()
	b.client.CloseIdleConnections()
	calls("no reply", ErrUnreachable)
}

func Test_store_bucket_conditional_write_is_one_request(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	ok := must[bool](t)

	if !ok(b.Write(ctx, "a/x", []byte("one"), OnlyIfNew)) {
		t.Fatal("only-if-new on a free name did not write")
	}
	f.sees("only-if-new", map[string]int{fNew: 1})
	if ok(b.Write(ctx, "a/x", []byte("two"), OnlyIfNew)) {
		t.Fatal("only-if-new wrote over a value")
	}
	f.sees("only-if-new over a value", map[string]int{fNew: 1})
	if !ok(b.WriteFrom(ctx, "a/from", strings.NewReader("one"))) {
		t.Fatal("a value from a reader was not written")
	}
	f.sees("a value from a reader", map[string]int{fNew: 1})
	if !ok(b.Copy(ctx, "a/x", "a/copy")) || ok(b.Copy(ctx, "a/x", "a/copy")) {
		t.Fatal("a copy is made once")
	}
	f.sees("two copies", map[string]int{fCopy: 2})

	_, mark := read(t, b, "a/x")
	f.took()
	if !ok(b.Write(ctx, "a/x", []byte("two"), OnlyIfUnchanged(mark))) {
		t.Fatal("only-if-unchanged did not write an unchanged value")
	}
	f.sees("only-if-unchanged", map[string]int{fUnchanged: 1})
	if ok(b.Write(ctx, "a/x", []byte("three"), OnlyIfUnchanged(mark))) {
		t.Fatal("only-if-unchanged wrote over a changed value")
	}
	f.sees("only-if-unchanged of a changed value", map[string]int{fUnchanged: 1})
	if ok(b.Delete(ctx, "a/x", OnlyIfUnchanged(mark))) {
		t.Fatal("deleted a changed value only-if-unchanged")
	}
	f.sees("delete only-if-unchanged", map[string]int{fDeleteUnchanged: 1})

	// A mark that names no value is not sent as a condition: the bucket
	// would take it for none, and overwrite.
	if ok(b.Write(ctx, "a/x", []byte("four"), OnlyIfUnchanged(None))) || ok(b.Write(ctx, "a/none", []byte("four"), OnlyIfUnchanged(None))) {
		t.Fatal("only-if-unchanged wrote with the mark of nothing")
	}
	f.sees("only-if-unchanged with the mark of nothing", map[string]int{})

	ok(b.Write(ctx, "a/x", []byte("five"), Overwrite))
	ok(b.Delete(ctx, "a/x", Overwrite))
	f.sees("an overwrite and a delete", map[string]int{fOverwrite: 1, fDelete: 1})

	// A party that may add to a place it may not read is not refused a
	// look, since none is taken: it is told whether the name was free.
	r1 := f.open(Party{Kind: Runner, Name: "r1", Controller: "ctl"})
	if !ok(r1.Write(ctx, "ctl/inbox/r1/m1", []byte("result"), OnlyIfNew)) {
		t.Fatal("a runner's result was not written")
	}
	if ok(r1.Write(ctx, "ctl/inbox/r1/m1", []byte("again"), OnlyIfNew)) {
		t.Fatal("a runner's result was written twice")
	}
	if _, mark := read(t, r1, "ctl/inbox/r1/m1"); mark != None {
		t.Fatal("a runner read the controller's inbox")
	}
	f.sees("a runner's two writes and a read", map[string]int{fNew: 2, fRead: 1})
}

// conditional is every write whose condition makes a second sending
// different from the first, each with the name it is to change.
func conditional(t *testing.T, f *fakeS3, b *bucket) map[string]func() (bool, error) {
	for _, name := range []string{"c/unchanged", "c/delete", "c/source"} {
		must[bool](t)(b.Write(ctx, name, []byte("was"), Overwrite))
	}
	_, mark := read(t, b, "c/unchanged")
	b.part = 4
	f.took()
	return map[string]func() (bool, error){
		fNew:             func() (bool, error) { return b.Write(ctx, "c/new", []byte("new"), OnlyIfNew) },
		fUnchanged:       func() (bool, error) { return b.Write(ctx, "c/unchanged", []byte("new"), OnlyIfUnchanged(mark)) },
		fDeleteUnchanged: func() (bool, error) { return b.Delete(ctx, "c/delete", OnlyIfUnchanged(mark)) },
		fCopy:            func() (bool, error) { return b.Copy(ctx, "c/source", "c/copy") },
		fEnd:             func() (bool, error) { return b.WriteFrom(ctx, "c/parts", strings.NewReader("in parts")) },
	}
}

// wentIn fails the test unless every conditional write went in exactly once.
func wentIn(t *testing.T, f *fakeS3) {
	t.Helper()
	for name, want := range map[string]string{"c/new": "new", "c/unchanged": "new", "c/copy": "was", "c/parts": "in parts"} {
		if v, _ := f.value(name); v != want {
			t.Errorf("%s holds %q, want %q", name, v, want)
		}
	}
	if _, there := f.value("c/delete"); there {
		t.Error("c/delete is still there")
	}
}

func Test_store_bucket_conditional_write_not_resent_after_lost_answer(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	for kind, write := range conditional(t, f, b) {
		f.set(func() { f.lose[kind] = 1 })
		done, err := write()
		if done || !errors.Is(err, ErrUnknown) {
			t.Errorf("%s with its answer lost: %v %v, want not known", kind, done, err)
		}
		if n := f.took()[kind]; n != 1 {
			t.Errorf("%s with its answer lost was sent %d times", kind, n)
		}
	}
	// Each went in, so a second sending would have met its own value and
	// said "already there" or "changed".
	wentIn(t, f)
}

func Test_store_bucket_resends_only_what_is_safe_twice(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)

	// Turned away, the bucket did nothing: whatever it is, it is sent again.
	for kind, write := range conditional(t, f, b) {
		f.set(func() { f.turnAway, f.only = 2, kind })
		if done, err := write(); err != nil || !done {
			t.Errorf("%s turned away twice: %v %v", kind, done, err)
		}
		if n := f.took()[kind]; n != 3 {
			t.Errorf("%s turned away twice was sent %d times", kind, n)
		}
	}
	f.set(func() { f.only = "" })
	wentIn(t, f)

	// Its answer lost, a request is sent again only where twice is the same
	// as once.
	must[bool](t)(b.Write(ctx, "s/x", []byte("0123456789"), Overwrite))
	safe := map[string]func() error{
		fRead: func() error {
			v, _, err := b.Read(ctx, "s/x")
			return errors.Join(err, same(string(v), "0123456789"))
		},
		fReadPart: func() error {
			v, err := b.ReadPart(ctx, "s/x", 2, 3)
			return errors.Join(err, same(string(v), "234"))
		},
		fList: func() error {
			names, err := Names(b.List(ctx, "s"))
			return errors.Join(err, same(fmt.Sprint(names), "[s/x]"))
		},
		fOverwrite: func() error {
			done, err := b.Write(ctx, "s/x", []byte("0123456789"), Overwrite)
			return errors.Join(err, same(done, true))
		},
		fDelete: func() error {
			done, err := b.Delete(ctx, "s/y", Overwrite)
			return errors.Join(err, same(done, true))
		},
	}
	for kind, call := range safe {
		f.took()
		f.set(func() { f.lose[kind] = 1 })
		if err := call(); err != nil {
			t.Errorf("%s with its answer lost: %v", kind, err)
		}
		f.sees(kind+" with its answer lost", map[string]int{kind: 2})
	}
	f.set(func() { f.lose[fNew] = 1 })
	if done, err := b.Write(ctx, "s/new", []byte("new"), OnlyIfNew); done || !errors.Is(err, ErrUnknown) {
		t.Errorf("only-if-new with its answer lost: %v %v", done, err)
	}
	f.sees("only-if-new with its answer lost", map[string]int{fNew: 1})
}

func same[T comparable](got, want T) error {
	if got != want {
		return fmt.Errorf("got %v, want %v", got, want)
	}
	return nil
}

func Test_store_bucket_retries_outlast_a_burst(t *testing.T) {
	// As built, tries go on for a few seconds.
	if all := bucketWait * (1<<(bucketTries-1) - 1); all < 3*time.Second || all > 10*time.Second {
		t.Fatalf("tries go on for %v", all)
	}
	f := newFakeS3(t)
	if b := must[*bucket](t)(newBucket(f.address(), f.newKey(owner))); b.wait != bucketWait {
		t.Fatalf("a bucket is opened waiting %v before its second try", b.wait)
	}

	// A burst that turns away every try but the last is outlasted, and the
	// waits between tries grow.
	b := f.open(owner)
	b.wait = 2 * time.Millisecond
	f.set(func() { f.turnAway = bucketTries - 1 })
	if done, err := b.Write(ctx, "a/x", []byte("x"), OnlyIfNew); err != nil || !done {
		t.Fatalf("a write through a burst: %v %v", done, err)
	}
	f.set(func() {
		if len(f.at) != bucketTries {
			t.Fatalf("the write was sent %d times", len(f.at))
		}
		for i := 1; i < len(f.at); i++ {
			if gap, want := f.at[i].Sub(f.at[i-1]), b.wait<<(i-1); gap < want {
				t.Errorf("try %d came %v after the one before, want %v or more", i+1, gap, want)
			}
		}
	})

	// One that lasts longer is reported as the store out of reach, and the
	// tries stop.
	f.took()
	f.set(func() { f.turnAway = bucketTries })
	if done, err := b.Write(ctx, "a/y", []byte("x"), OnlyIfNew); done || !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a write through a longer burst: %v %v", done, err)
	}
	f.sees("a write through a longer burst", map[string]int{fNew: bucketTries})

	// A caller that gives up ends the tries.
	f.set(func() { f.turnAway = bucketTries })
	b.wait = time.Minute
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, _, err := b.Read(short, "a/x"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a read given up: %v", err)
	}
}

func Test_store_bucket_idle_connection_not_reused(t *testing.T) {
	if bucketIdle < time.Second || bucketIdle > 5*time.Second {
		t.Fatalf("a connection is kept %v unused", bucketIdle)
	}
	f := newFakeS3(t)
	b := f.open(owner)
	transport := b.client.Transport.(*http.Transport)
	if transport.IdleConnTimeout != bucketIdle {
		t.Fatalf("a bucket is opened keeping a connection %v unused", transport.IdleConnTimeout)
	}
	transport.IdleConnTimeout = 300 * time.Millisecond
	conns := func() (n int) {
		f.set(func() { n = f.conns })
		return n
	}

	// One request after another goes down the one connection.
	for range 3 {
		read(t, b, "a/x")
	}
	if n := conns(); n != 1 {
		t.Fatalf("three requests in a row used %d connections", n)
	}
	// After a pause it is not used again, for a conditional write either.
	time.Sleep(3 * transport.IdleConnTimeout)
	if done, err := b.Write(ctx, "a/x", []byte("x"), OnlyIfNew); err != nil || !done {
		t.Fatalf("a write after a pause: %v %v", done, err)
	}
	if n := conns(); n != 2 {
		t.Fatalf("a request after a pause: %d connections in all, want a second", n)
	}
}

func Test_store_bucket_never_copies_a_value_onto_itself(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	must[bool](t)(b.Write(ctx, "ctl/records/c1/i1/blobs/b1", []byte("blob"), OnlyIfNew))
	_, mark := read(t, b, "ctl/records/c1/i1/blobs/b1")
	f.took()

	// As the owner, and as a party that may only add.
	for _, s := range []Store{b, f.as(Party{Kind: Client, Name: "c1", Controller: "ctl"})} {
		if done, err := s.Copy(ctx, "ctl/records/c1/i1/blobs/b1", "ctl/records/c1/i1/blobs/b1"); done || err != nil {
			t.Fatalf("a copy onto itself: %v %v", done, err)
		}
	}
	f.sees("two copies of a value onto itself", map[string]int{})
	if v, now := read(t, b, "ctl/records/c1/i1/blobs/b1"); v != "blob" || now != mark {
		t.Fatalf("the value is %q, marked %s where it was %s", v, now, mark)
	}
}

// watched calls each before every read of r.
type watched struct {
	r    io.Reader
	each func()
}

func (w watched) Read(p []byte) (int, error) {
	w.each()
	return w.r.Read(p)
}

func Test_store_bucket_value_in_parts_appears_whole(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	b.part = 1000
	big := bytes.Repeat([]byte("0123456789"), 1050) // ten full parts and a half
	uploads := func() (n int) {
		f.set(func() { n = len(f.uploads) })
		return n
	}

	// While its parts go in, the name holds nothing; then it holds it all.
	seen := 0
	absent := watched{bytes.NewReader(big), func() {
		if _, there := f.value("b/big"); there {
			t.Error("the value is under its name before its last part is in")
		}
		seen = max(seen, uploads())
	}}
	if done, err := b.WriteFrom(ctx, "b/big", absent); err != nil || !done {
		t.Fatalf("a value in parts: %v %v", done, err)
	}
	if seen != 1 {
		t.Fatalf("%d uploads were under way", seen)
	}
	f.sees("a value in parts", map[string]int{fStart: 1, fPart: 11, fEnd: 1})
	if v, _ := read(t, b, "b/big"); v != string(big) {
		t.Fatalf("the value reads %d bytes, want %d", len(v), len(big))
	}
	// One of exactly a part's length, too.
	if done, err := b.WriteFrom(ctx, "b/one", bytes.NewReader(big[:1000])); err != nil || !done {
		t.Fatalf("a value of one part: %v %v", done, err)
	}
	if v, _ := read(t, b, "b/one"); v != string(big[:1000]) {
		t.Fatalf("a value of one part reads %d bytes", len(v))
	}
	f.took()

	// Only-if-new still: a second is told "already there", and its parts go.
	if done, err := b.WriteFrom(ctx, "b/big", bytes.NewReader(bytes.ToUpper(big))); err != nil || done {
		t.Fatalf("a second value in parts: %v %v", done, err)
	}
	if got := f.took(); got[fEnd] != 1 || got[fGiveUp] != 1 || uploads() != 0 {
		t.Fatalf("a second value in parts: the bucket took %v and holds %d uploads", got, uploads())
	}
	if v, _ := read(t, b, "b/big"); v != string(big) {
		t.Fatal("a second value in parts replaced the first")
	}

	// An upload given up partway leaves no value and no parts: where the
	// reader fails, where the caller goes, and where the bucket stops
	// answering.
	broken := errors.New("the reader broke")
	gone, cancel := context.WithCancel(ctx)
	for why, write := range map[string]func() (bool, error){
		"the reader failing": func() (bool, error) {
			return b.WriteFrom(ctx, "b/part", io.MultiReader(bytes.NewReader(big[:3500]), iotest.ErrReader(broken)))
		},
		"the caller going": func() (bool, error) {
			return b.WriteFrom(gone, "b/part", watched{bytes.NewReader(big), func() {
				if uploads() == 1 {
					cancel()
				}
			}})
		},
		"a part turned away": func() (bool, error) {
			return b.WriteFrom(ctx, "b/part", watched{bytes.NewReader(big), func() {
				if uploads() == 1 {
					f.set(func() { f.turnAway = bucketTries })
				}
			}})
		},
	} {
		f.took()
		done, err := write()
		if done || err == nil {
			t.Fatalf("%s: %v %v", why, done, err)
		}
		if why == "the reader failing" && !errors.Is(err, broken) {
			t.Fatalf("%s: the error is %v", why, err)
		}
		if got := f.took(); got[fStart] != 1 || got[fGiveUp] != 1 || got[fEnd] != 0 || uploads() != 0 {
			t.Fatalf("%s: the bucket took %v and holds %d uploads", why, got, uploads())
		}
		if _, there := f.value("b/part"); there {
			t.Fatalf("%s left a value", why)
		}
	}

	// A party that may only add sends a large blob the same way.
	c1 := f.open(Party{Kind: Client, Name: "c1", Controller: "ctl"})
	c1.part = 1000
	if done, err := c1.WriteFrom(ctx, "ctl/records/c1/i1/blobs/up", bytes.NewReader(big)); err != nil || !done {
		t.Fatalf("a client's large blob: %v %v", done, err)
	}
	if done, err := c1.WriteFrom(ctx, "ctl/records/c2/i2/blobs/up", bytes.NewReader(big)); done || !errors.Is(err, ErrRefused) {
		t.Fatalf("a client's large blob in another's record: %v %v", done, err)
	}
	if _, there := f.value("ctl/records/c2/i2/blobs/up"); there || uploads() != 0 {
		t.Fatal("a refused upload left something")
	}
}

// A bucket's listing gives each name the ETag the bucket lists it with,
// which is the mark a read gives and the one If-Match takes. No value is
// read to give it.
func Test_store_bucket_listing_reads_no_value(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	f.set(func() { f.page = 2 })
	for _, name := range []string{"l/a", "l/b", "l/c"} {
		must[bool](t)(b.Write(ctx, name, []byte("the value of "+name), Overwrite))
	}
	f.took()
	listed := must[[]Listed](t)(b.List(ctx, "l"))
	first := must[[]Listed](t)(b.First(ctx, "l", 1))
	f.sees("a listing of two pages and the first name", map[string]int{fList: 3})
	if len(listed) != 3 || len(first) != 1 || first[0] != listed[0] {
		t.Fatalf("the listing gave %v and the first %v", listed, first)
	}
	for _, l := range listed {
		if value, _ := f.value(l.Name); l.Mark != Mark(etag([]byte(value))) {
			t.Fatalf("%s is listed with %q, not its ETag", l.Name, l.Mark)
		}
		if _, mark := read(t, b, l.Name); l.Mark != mark {
			t.Fatalf("%s is listed with %q and read with %q", l.Name, l.Mark, mark)
		}
	}
	f.took()
	if !must[bool](t)(b.Write(ctx, "l/b", []byte("another"), OnlyIfUnchanged(listed[1].Mark))) {
		t.Fatal("only-if-unchanged with the ETag a listing gave did not write")
	}
	f.sees("only-if-unchanged with a listed ETag", map[string]int{fUnchanged: 1})
}

func Test_store_bucket_reads_bounded_at_once(t *testing.T) {
	f := newFakeS3(t)
	b := f.open(owner)
	if got := b.client.Transport.(*http.Transport).MaxConnsPerHost; got != bucketConns {
		t.Fatalf("a bucket is opened with %d connections at most", got)
	}
	var names []string
	for i := range 10 * bucketConns {
		names = append(names, fmt.Sprintf("many/%03d", i))
		f.objects["pre/fix/"+names[i]] = []byte(names[i])
	}
	f.set(func() { f.hold = 10 * time.Millisecond })

	// A caller reads every name a listing gave, all at once.
	var wg sync.WaitGroup
	for _, name := range must[[]string](t)(Names(b.List(ctx, "many"))) {
		wg.Go(func() {
			if value, _, err := b.Read(ctx, name); err != nil || string(value) != name {
				t.Errorf("read %s: %q %v", name, value, err)
			}
		})
	}
	wg.Wait()
	f.set(func() {
		if f.mostBusy > bucketConns || f.mostBusy < 2 || f.conns > bucketConns {
			t.Fatalf("%d requests were with the bucket at once, over %d connections; the bound is %d", f.mostBusy, f.conns, bucketConns)
		}
		if f.seen[fRead] != len(names) {
			t.Fatalf("%d of %d names were read", f.seen[fRead], len(names))
		}
	})
}

func Test_store_address_and_errors_carry_no_key(t *testing.T) {
	f := newFakeS3(t)
	held := f.newKey(Party{Kind: Viewer, Controller: "ctl"})
	const token = "tOkEn/lent+for=a%while"
	f.keys[held.id] = fakeKey{secret: held.secret, token: token, party: Party{Kind: Viewer, Controller: "ctl"}}
	secrets := []string{held.id, held.secret, token}
	var said []string
	say := func(err error) {
		t.Helper()
		if err == nil {
			t.Error("no error where one was looked for")
			return
		}
		said = append(said, err.Error())
	}

	// The key comes from the environment, and the address has none.
	t.Setenv("AWS_ACCESS_KEY_ID", held.id)
	t.Setenv("AWS_SECRET_ACCESS_KEY", held.secret)
	t.Setenv("AWS_SESSION_TOKEN", token)
	address := f.address()
	s := must[Store](t)(Open(address))
	b := s.(*bucket)
	b.wait = time.Millisecond / 4
	f.objects["pre/fix/ctl/running/c1/i1"] = []byte("head")
	if v, _ := read(t, s, "ctl/running/c1/i1"); v != "head" {
		t.Fatal("the key in the environment did not open the store")
	}

	// Refused, out of reach, not known, and an answer that makes no sense.
	_, err := s.Write(ctx, "ctl/running/c1/i1", []byte("x"), Overwrite)
	say(err)
	_, err = s.Delete(ctx, "ctl/running/c1/i1", Overwrite)
	say(err)
	_, err = Names(s.List(ctx, "ctl/trust"))
	say(err)
	f.set(func() { f.turnAway = bucketTries })
	_, _, err = s.Read(ctx, "ctl/running/c1/i1")
	say(err)
	f.set(func() { f.turnAway, f.status = 1, http.StatusBadGateway })
	_, err = s.Write(ctx, "ctl/running/c1/x", []byte("x"), OnlyIfNew)
	say(err)
	f.set(func() { f.turnAway, f.status = 1, http.StatusTeapot })
	_, err = Names(s.List(ctx, "ctl/running"))
	say(err)
	f.set(func() { f.lose[fRead] = 1 << 30 })
	_, _, err = s.Read(ctx, "ctl/running/c1/i1")
	say(err)
	f.set(func() { f.lose[fRead] = 0 })

	// A key the bucket does not know: it quotes the key in its answer.
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDUNKNOWN")
	unknown := must[Store](t)(Open(address))
	_, err = unknown.Write(ctx, "a/x", []byte("x"), OnlyIfNew)
	say(err)
	_, err = Names(unknown.List(ctx, "a"))
	say(err)
	secrets = append(secrets, "AKIDUNKNOWN")

	// Nothing listening.
	const nowhereAt = "http://127.0.0.1:1"
	nowhere := must[Store](t)(Open("s3://" + fakeBucket + "?region=" + fakeRegion + "&endpoint=" + nowhereAt))
	nowhere.(*bucket).wait = time.Millisecond / 4
	_, err = Names(nowhere.List(ctx, "a"))
	say(err)

	// An address that wrongly has a key in it is not opened, nor repeated.
	for _, bad := range []string{
		"s3://" + held.id + ":" + held.secret + "@bkt?region=r",
		"s3://" + held.id + ":sEcReTwithNoSlash@bkt?region=r",
		"s3://bkt?region=r&" + held.id + "=" + held.secret,
		"s3://bkt?region=r&endpoint=https://" + held.id + ":" + token + "@host",
		"s3://bkt?region=r&endpoint=" + held.secret,
	} {
		_, err := Open(bad)
		say(err)
		secrets = append(secrets, bad, "sEcReTwithNoSlash")
	}

	// Failing the environment, the key is a profile's; and where there is
	// none, the error names no key.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	_, err = Open(address)
	say(err)
	other := f.newKey(owner)
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o700); err != nil {
		t.Fatal(err)
	}
	profiles := "# keys\n[default]\naws_access_key_id = " + held.id + "\naws_secret_access_key=" + held.secret + "\n" +
		"aws_session_token = " + token + "\n\n" +
		"[other]\naws_access_key_id=" + other.id + "\n aws_secret_access_key = " + other.secret + " \n[empty]\naws_access_key_id = " + held.id + "\n"
	if err := os.WriteFile(filepath.Join(home, ".aws", "credentials"), []byte(profiles), 0o600); err != nil {
		t.Fatal(err)
	}
	byDefault := must[Store](t)(Open(address))
	if v, _ := read(t, byDefault, "ctl/running/c1/i1"); v != "head" {
		t.Fatal("the default profile's key did not open the store")
	}
	if _, err := byDefault.Write(ctx, "ctl/running/c1/x", []byte("x"), OnlyIfNew); !errors.Is(err, ErrRefused) {
		t.Fatalf("the default profile's key is the viewer's: %v", err)
	}
	t.Setenv("AWS_PROFILE", "other")
	byName := must[Store](t)(Open(address))
	if done, err := byName.Write(ctx, "ctl/running/c1/x", []byte("x"), OnlyIfNew); err != nil || !done {
		t.Fatalf("the named profile's key is the owner's: %v %v", done, err)
	}
	for _, profile := range []string{"empty", "missing"} {
		t.Setenv("AWS_PROFILE", profile)
		_, err = Open(address)
		say(err)
	}
	secrets = append(secrets, other.id, other.secret)

	for _, text := range append(said, address) {
		for _, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Errorf("%q shows %q", text, secret)
			}
		}
	}
	for _, text := range said {
		if strings.Contains(text, address) || strings.Contains(text, f.srv.URL) || strings.Contains(text, nowhereAt) {
			t.Errorf("%q repeats the address", text)
		}
	}
}
