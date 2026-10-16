package cli_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/store"
)

// With no controller running, cancel, send and reply given a life each
// come back once that life has passed, saying the message was not taken
// up; and what each sent is good for that life.
func TestCancelSendAndReplyTakeALife(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	s, err := store.Open(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inbox := store.Inbox("box-controller")
	id := "box-client/" + time.Now().UTC().Format("20060102T150405.000Z") + "-0000000000"
	for _, command := range [][]string{{"run", "cancel", id}, {"run", "send", id, "go"}, {"run", "reply", id, "the-key", "yes"}} {
		before, err := s.List(ctx, inbox)
		if err != nil {
			t.Fatal(err)
		}
		began := time.Now()
		status, out, said := m.run("", append(command, "--life", "200ms")...)
		if status != 1 || out != "" || !strings.Contains(said, "not taken up") {
			t.Errorf("corbie %s --life 200ms with no controller ended %d: %s%s", strings.Join(command[:2], " "), status, out, said)
		}
		if took := time.Since(began); took > 10*time.Second {
			t.Errorf("corbie %s --life 200ms waited %v for its receipt", strings.Join(command[:2], " "), took)
		}
		after, err := s.List(ctx, inbox)
		if err != nil || len(after) != len(before)+1 {
			t.Fatalf("corbie %s left %d messages in the inbox where there were %d (%v)", strings.Join(command[:2], " "), len(after), len(before), err)
		}
		// The one that is new: names sort by when they were made, and a
		// start's name is made before the start is sent.
		sent := slices.DeleteFunc(after, func(name string) bool { return slices.Contains(before, name) })
		data, _, err := s.Read(ctx, sent[0])
		if err != nil {
			t.Fatal(err)
		}
		value, err := identity.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if good := value.Until.Sub(value.Sent); good != 200*time.Millisecond {
			t.Errorf("what corbie %s --life 200ms sent is good for %v", strings.Join(command[:2], " "), good)
		}
		// A life that is no length of time is a mistake, and sends nothing.
		if status, _, said := m.run("", append(command, "--life", "soon")...); status != 2 || !strings.Contains(said, "--life") {
			t.Errorf("corbie %s --life soon ended %d: %s", strings.Join(command[:2], " "), status, said)
		}
		if again, _ := s.List(ctx, inbox); len(again) != len(before)+1 {
			t.Errorf("corbie %s --life soon sent a message", strings.Join(command[:2], " "))
		}
	}
}
