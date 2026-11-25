package store

import (
	"errors"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// otherProcess has another process write or delete one name of a directory.
func otherProcess(t *testing.T, dir, what, name string) {
	t.Helper()
	cmd := helperCmd(dir, what)
	cmd.Env = append(cmd.Env, "CORBIE_STORE_HELPER_NAME="+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s in another process: %v\n%s", what, name, err, out)
	}
}

// told says whether a notice comes within a few seconds.
func told(notice <-chan struct{}) bool {
	select {
	case <-notice:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

// settled waits until no notice has come for a moment, and takes those
// that did: one write can be reported more than once.
func settled(notices ...<-chan struct{}) {
	for quiet := false; !quiet; {
		quiet = true
		for _, notice := range notices {
			select {
			case <-notice:
				quiet = false
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func Test_store_directory_tells_of_another_processs_write(t *testing.T) {
	s, dir := newDirectory(t)
	// Nothing is in the directory yet: the inbox and the running heads are
	// made by the writes that follow.
	inbox, stop := s.Notice(Inbox("ctl"))
	defer stop()
	area, stop := s.Notice("ctl")
	defer stop()
	records, stop := s.Notice(Records("ctl"))
	defer stop()
	runner, stop := s.Notice(Inbox("runner"))
	defer stop()

	message := Message("ctl", "client", "m1")
	otherProcess(t, dir, "write", message)
	if !told(inbox) || !told(area) {
		t.Fatal("a message another process wrote into an inbox was not told to those waiting on the inbox and on the party's area")
	}
	settled(inbox, area)

	// A store removes the directory a delete leaves empty, and makes it
	// again for the next message.
	otherProcess(t, dir, "delete", message)
	if !told(inbox) || !told(area) {
		t.Fatal("a message another process deleted was not told")
	}
	settled(inbox, area)
	otherProcess(t, dir, "write", Message("ctl", "client", "m2"))
	if !told(inbox) {
		t.Fatal("a message written into an inbox that was removed and made again was not told")
	}
	settled(inbox, area)

	head := RunningHead("ctl", "client/run")
	otherProcess(t, dir, "write", head)
	if !told(area) {
		t.Fatal("a running head another process wrote was not told")
	}
	settled(area)
	otherProcess(t, dir, "write", head)
	if !told(area) {
		t.Fatal("a running head another process replaced was not told")
	}
	settled(area)

	// A record and a finished head grow with every run, and are not watched.
	otherProcess(t, dir, "write", Step("ctl", "client/run", 1))
	otherProcess(t, dir, "write", FinishedHead("ctl", "client/run"))
	select {
	case <-records:
		t.Error("a step another process wrote was told")
	case <-area:
		t.Error("a step or a finished head another process wrote was told")
	case <-inbox:
		t.Error("a write outside the inbox was told to one waiting on the inbox")
	case <-runner:
		t.Error("a write in one party's area was told to one waiting on another's inbox")
	case <-time.After(300 * time.Millisecond):
	}

	// A write of this process is told wherever it is.
	if _, err := s.Write(ctx, Step("ctl", "client/run", 2), []byte("step"), OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	if !told(records) || !told(area) {
		t.Fatal("a step this process wrote was not told")
	}
}

func Test_store_bucket_tells_the_process_that_wrote(t *testing.T) {
	f := newFakeS3(t)
	writer, waiter := f.open(owner), f.open(owner)
	inbox, stop := waiter.Notice(Inbox("ctl"))
	defer stop()
	other, stop := waiter.Notice(Inbox("runner"))
	defer stop()
	message := Message("ctl", "client", "m1")
	if done, err := writer.Write(ctx, message, []byte("message"), OnlyIfNew); err != nil || !done {
		t.Fatal(done, err)
	}
	select {
	case <-inbox:
	default:
		t.Fatal("a write to a bucket was not told to the same process, waiting on the path through another store opened on the bucket")
	}
	if done, err := writer.Write(ctx, message, []byte("again"), OnlyIfNew); err != nil || done {
		t.Fatal(done, err)
	}
	select {
	case <-inbox:
		t.Fatal("a write the bucket refused as already there was told")
	default:
	}
	if done, err := writer.Delete(ctx, message, Overwrite); err != nil || !done {
		t.Fatal(done, err)
	}
	select {
	case <-inbox:
	default:
		t.Fatal("a delete was not told")
	}
	select {
	case <-other:
		t.Fatal("a write under one path was told to one waiting on another")
	default:
	}
}

func Test_store_directory_says_why_it_is_not_watched(t *testing.T) {
	refusal := errors.New("the system has no watcher to give")
	given := newWatcher
	newWatcher = func() (*fsnotify.Watcher, error) { return nil, refusal }
	defer func() { newWatcher = given }()

	s, _ := newDirectory(t)
	if err := Unreported(s); err != nil {
		t.Fatalf("a directory nobody waits on says %v", err)
	}
	inbox, stop := s.Notice(Inbox("ctl"))
	defer stop()
	if err := Unreported(s); !errors.Is(err, refusal) {
		t.Fatalf("a directory the system would not watch says %v", err)
	}
	// A write of this process is told all the same.
	if _, err := s.Write(ctx, Message("ctl", "client", "m1"), []byte("message"), OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	if !told(inbox) {
		t.Fatal("a write of this process was not told in a directory the system would not watch")
	}
	if err := Unreported(NewMemory().As(Party{Kind: Owner})); err != nil {
		t.Fatalf("a store in memory says %v", err)
	}

	// The system gives a watcher again: the next wait is watched.
	newWatcher = given
	again, stop := s.Notice(Inbox("ctl"))
	defer stop()
	if err := Unreported(s); err != nil {
		t.Fatalf("a directory the system watches says %v", err)
	}
	select {
	case <-again:
		t.Fatal("a wait was told of a write made before it began")
	default:
	}
}
