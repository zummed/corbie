//go:build unix || windows

package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for another process on a directory.
func TestMain(m *testing.M) {
	if dir := os.Getenv("CORBIE_STORE_HELPER_DIR"); dir != "" {
		helper(dir, os.Getenv("CORBIE_STORE_HELPER"))
		return
	}
	os.Exit(m.Run())
}

func helper(dir, what string) {
	s, _ := OpenDirectory(dir)
	switch what {
	case "count":
		// Add one to a number, only-if-unchanged, fifty times.
		for range 50 {
			for {
				value, mark, _ := s.Read(ctx, "n/count")
				var n int
				fmt.Sscan(string(value), &n)
				if done, _ := s.Write(ctx, "n/count", fmt.Append(nil, n+1), OnlyIfUnchanged(mark)); done {
					break
				}
			}
		}
	case "hold":
		// Take the directory's lock alone, say so, and wait to be killed.
		s.(*directory).lock(ctx, true)
		fmt.Println("held")
		time.Sleep(time.Hour)
	}
}

func helperCmd(dir, what string) *exec.Cmd {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "CORBIE_STORE_HELPER_DIR="+dir, "CORBIE_STORE_HELPER="+what)
	return cmd
}

func newDirectory(t *testing.T) (Store, string) {
	dir := t.TempDir()
	s, err := OpenDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func Test_store_directory_value_whole_or_absent(t *testing.T) {
	s, dir := newDirectory(t)
	a, b := bytes.Repeat([]byte("a"), 1<<20), bytes.Repeat([]byte("b"), 1<<20)
	// The value is there before the first read, and is written over until
	// the reading is done: so every read falls beside a write, however
	// the two are given their turns.
	if _, err := s.Write(ctx, "big/value", b, Overwrite); err != nil {
		t.Fatal(err)
	}
	const overwritten = 20
	var writes atomic.Int32
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			value := a
			if i%2 == 1 {
				value = b
			}
			if _, err := s.Write(ctx, "big/value", value, Overwrite); err != nil {
				t.Error(err)
				writes.Store(overwritten)
				return
			}
			writes.Add(1)
		}
	})
	for writes.Load() < overwritten {
		value, _, err := s.Read(ctx, "big/value")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(value, a) && !bytes.Equal(value, b) {
			t.Fatalf("read part of a value: %d bytes", len(value))
		}
	}
	close(stop)
	wg.Wait()

	// A writer killed partway leaves a temporary file and the name as it was.
	os.WriteFile(filepath.Join(dir, "big", tmpPrefix+"killed"), []byte("half"), 0o600)
	names, err := s.List(ctx, "big")
	if err != nil || !slices.Equal(names, []string{"big/value"}) {
		t.Fatalf("listing shows %v (%v)", names, err)
	}
}

func Test_store_directory_only_if_new_while_directory_comes_and_goes(t *testing.T) {
	s, _ := newDirectory(t)
	// One goroutine keeps emptying the directory, so it comes and goes
	// under the writers. No only-if-new write to a free name may be
	// answered "already there".
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			s.Write(ctx, "d/sub/churn", []byte("x"), Overwrite)
			s.Delete(ctx, "d/sub/churn", Overwrite)
		}
	})
	for i := range 500 {
		name := fmt.Sprintf("d/sub/n%d", i)
		done, err := s.Write(ctx, name, []byte("v"), OnlyIfNew)
		if err != nil || !done {
			t.Fatalf("write %s: done=%v err=%v", name, done, err)
		}
		if done, err := s.Write(ctx, name, []byte("w"), OnlyIfNew); err != nil || done {
			t.Fatalf("second write %s: done=%v err=%v", name, done, err)
		}
		s.Delete(ctx, name, Overwrite)
	}
	close(stop)
	wg.Wait()
}

func Test_store_directory_write_survives_vanishing_directory_and_temporary_file(t *testing.T) {
	s, _ := newDirectory(t)
	// Something clears the temporary file away, twice, before it is moved.
	for _, how := range []How{Overwrite, OnlyIfNew} {
		taken := 0
		tempWritten = func(tmp string) {
			if taken++; taken <= 2 {
				os.Remove(tmp)
				os.Remove(filepath.Dir(tmp))
			}
		}
		done, err := s.Write(ctx, "v/sub/"+how.String(), []byte("value"), how)
		tempWritten = func(string) {}
		if err != nil || !done {
			t.Fatalf("%s: done=%v err=%v", how, done, err)
		}
		if v, _ := read(t, s, "v/sub/"+how.String()); v != "value" {
			t.Fatalf("%s left %q", how, v)
		}
	}
}

func Test_store_directory_clearing_spares_temporary_file_in_use(t *testing.T) {
	s, dir := newDirectory(t)
	must[bool](t)(s.Write(ctx, "t/a", []byte("x"), Overwrite))
	fresh := filepath.Join(dir, "t", tmpPrefix+"fresh")
	old := filepath.Join(dir, "t", tmpPrefix+"old")
	os.WriteFile(fresh, []byte("in use"), 0o600)
	os.WriteFile(old, []byte("left over"), 0o600)
	os.Chtimes(old, time.Now().Add(-2*tmpMaxAge), time.Now().Add(-2*tmpMaxAge))
	if _, err := s.List(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a temporary file in use was cleared")
	}
	if _, err := os.Stat(old); err == nil {
		t.Fatal("an old temporary file was left")
	}
}

func Test_store_directory_list_while_names_replaced(t *testing.T) {
	s, _ := newDirectory(t)
	var want []string
	for i := range 200 {
		name := fmt.Sprintf("l/n%03d", i)
		want = append(want, name)
		must[bool](t)(s.Write(ctx, name, []byte("0"), Overwrite))
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				s.Write(ctx, want[(i*7+w*50)%len(want)], fmt.Append(nil, i), Overwrite)
			}
		})
	}
	for range 200 {
		names, err := s.List(ctx, "l")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(names, want) {
			t.Fatalf("a listing gave %d names, want %d", len(names), len(want))
		}
	}
	close(stop)
	wg.Wait()
}

func Test_store_directory_write_not_starved_by_listings(t *testing.T) {
	s, _ := newDirectory(t)
	for i := range 300 {
		must[bool](t)(s.Write(ctx, fmt.Sprintf("s/n%03d", i), []byte("0"), OnlyIfNew))
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				s.List(ctx, "s")
			}
		})
	}
	time.Sleep(10 * time.Millisecond)
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for i := range 50 {
		if _, err := s.Write(deadline, "s/n000", fmt.Append(nil, i), Overwrite); err != nil {
			t.Fatalf("write %d did not get its turn: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
}

func Test_store_directory_only_if_unchanged_across_processes(t *testing.T) {
	s, dir := newDirectory(t)
	must[bool](t)(s.Write(ctx, "n/count", []byte("0"), OnlyIfNew))
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if out, err := helperCmd(dir, "count").CombinedOutput(); err != nil {
				t.Errorf("helper: %v\n%s", err, out)
			}
		})
	}
	wg.Wait()
	if v, _ := read(t, s, "n/count"); v != "200" {
		t.Fatalf("the count is %s, want 200", v)
	}
}

// hold starts a process that holds the directory's lock alone.
func hold(t *testing.T, dir string) *exec.Cmd {
	cmd := helperCmd(dir, "hold")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	buf := make([]byte, 5)
	if _, err := out.Read(buf); err != nil || string(buf[:4]) != "held" {
		t.Fatalf("the helper did not take the lock: %q %v", buf, err)
	}
	return cmd
}

func Test_store_directory_wait_ends_with_deadline(t *testing.T) {
	s, dir := newDirectory(t)
	must[bool](t)(s.Write(ctx, "w/a", []byte("x"), Overwrite))
	hold(t, dir)

	// A call whose deadline has passed waits for nothing.
	past, cancel := context.WithCancel(ctx)
	cancel()
	start := time.Now()
	if _, err := s.Write(past, "w/a", []byte("y"), Overwrite); err == nil {
		t.Fatal("a write past its deadline went ahead")
	}
	if _, err := s.List(past, "w"); err == nil {
		t.Fatal("a listing past its deadline went ahead")
	}
	if time.Since(start) > time.Second {
		t.Fatal("a call past its deadline waited")
	}

	// A wait ends when the deadline does.
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	start = time.Now()
	if _, err := s.Write(short, "w/a", []byte("y"), Overwrite); err == nil {
		t.Fatal("a write got past a lock held elsewhere")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the wait lasted %v", took)
	}
	if v, _ := read(t, s, "w/a"); v != "x" {
		t.Fatalf("the value is %q", v)
	}
}

func Test_store_directory_many_waiting_writes_few_threads(t *testing.T) {
	s, dir := newDirectory(t)
	must[bool](t)(s.Write(ctx, "m/a", []byte("x"), Overwrite))
	holder := hold(t, dir)
	before := threads()
	var wg sync.WaitGroup
	var done atomic.Int32
	for i := range 2000 {
		wg.Go(func() {
			if _, err := s.Write(ctx, "m/a", fmt.Append(nil, i), Overwrite); err != nil {
				t.Error(err)
			}
			done.Add(1)
		})
	}
	time.Sleep(300 * time.Millisecond)
	if grew := threads() - before; grew > 100 {
		t.Fatalf("2000 waiting writes took %d more threads", grew)
	}
	if done.Load() != 0 {
		t.Fatal("a write got past a lock held elsewhere")
	}
	holder.Process.Kill()
	wg.Wait()
}

// threads is how many threads of the operating system this process has.
func threads() int {
	entries, err := os.ReadDir("/proc/self/task")
	if err != nil {
		n, _ := runtime.ThreadCreateProfile(nil)
		return n
	}
	return len(entries)
}

func Test_store_directory_killed_party_blocks_nobody(t *testing.T) {
	s, dir := newDirectory(t)
	must[bool](t)(s.Write(ctx, "k/a", []byte("x"), Overwrite))
	holder := hold(t, dir)
	holder.Process.Kill()
	holder.Wait()
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.Write(deadline, "k/a", []byte("y"), Overwrite); err != nil {
		t.Fatal("a killed party still holds the lock:", err)
	}
	if _, err := s.List(deadline, "k"); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryIsPlainFiles(t *testing.T) {
	s, dir := newDirectory(t)
	must[bool](t)(s.Write(ctx, "ctl/running/c1/i1", []byte("head"), Overwrite))
	must[bool](t)(s.Write(ctx, "ctl/records/c1/i1/steps/00000001", []byte("step"), OnlyIfNew))
	if b, err := os.ReadFile(filepath.Join(dir, "ctl", "running", "c1", "i1")); err != nil || string(b) != "head" {
		t.Fatalf("the head is not a plain file: %q %v", b, err)
	}
	// A copy is a link: one file under two names.
	must[bool](t)(s.Copy(ctx, "ctl/records/c1/i1/steps/00000001", "ctl/records/c1/i2/blobs/b"))
	a, _ := os.Stat(filepath.Join(dir, "ctl/records/c1/i1/steps/00000001"))
	b, _ := os.Stat(filepath.Join(dir, "ctl/records/c1/i2/blobs/b"))
	if !os.SameFile(a, b) {
		t.Fatal("a copy is not a link")
	}
	// Deleting leaves no empty directories.
	must[bool](t)(s.Delete(ctx, "ctl/records/c1/i2/blobs/b", Overwrite))
	if _, err := os.Stat(filepath.Join(dir, "ctl/records/c1/i2")); err == nil {
		t.Fatal("an empty directory was left")
	}
}

func Test_store_directory_answered_change_is_on_disk(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	var synced []string
	dirSynced = func(d string) { synced = append(synced, d) }
	defer func() { dirSynced = func(string) {} }()
	ctx := context.Background()
	// Each way of changing a name has its directory made safe on disk
	// before it answers, and every directory above it in the directory, which
	// may have been made for the name.
	changes := []struct {
		what string
		do   func() (bool, error)
		dir  string
	}{
		{"only-if-new", func() (bool, error) { return s.Write(ctx, "a/b/one", []byte("1"), OnlyIfNew) }, "a/b"},
		{"overwrite", func() (bool, error) { return s.Write(ctx, "a/b/one", []byte("2"), Overwrite) }, "a/b"},
		{"only-if-unchanged", func() (bool, error) {
			return s.Write(ctx, "a/b/one", []byte("3"), OnlyIfUnchanged(markOf([]byte("2"))))
		}, "a/b"},
		{"a value read from elsewhere", func() (bool, error) { return s.WriteFrom(ctx, "a/b/two", bytes.NewReader([]byte("4"))) }, "a/b"},
		{"copy", func() (bool, error) { return s.Copy(ctx, "a/b/one", "a/c/one") }, "a/c"},
		// A delete that empties its directory takes the directory away:
		// what is made safe is the one left holding the change.
		{"delete", func() (bool, error) { return s.Delete(ctx, "a/c/one", Overwrite) }, "a"},
	}
	for _, c := range changes {
		synced = nil
		done, err := c.do()
		if err != nil || !done {
			t.Fatalf("%s: %v %v", c.what, done, err)
		}
		var want []string
		for d := filepath.Join(dir, filepath.FromSlash(c.dir)); len(d) >= len(dir); d = filepath.Dir(d) {
			want = append(want, d)
		}
		if !slices.Equal(synced, want) {
			t.Errorf("%s answered with %v made safe on disk, and not %v", c.what, synced, want)
		}
	}
	// A write that changes nothing has nothing to make safe.
	synced = nil
	if done, err := s.Write(ctx, "a/b/one", []byte("5"), OnlyIfNew); done || err != nil || len(synced) != 0 {
		t.Errorf("an only-if-new write that found a value: %v %v, synced %v", done, err, synced)
	}
}
