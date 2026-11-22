package gateway_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/machine"
)

// The client package sends to a gateway what it otherwise does on a store:
// a start with a file, a listing followed as it changes, a record followed
// to the run's end, a file fetched, an event and a cancel with their
// receipts. A refusal is the error the package gives for it.
func Test_client_package_sends_to_a_gateway(t *testing.T) {
	s := newSeveral(t)
	editor, viewer := s.enrol("editor", "all", "at:/**"), s.enrol("viewer")
	as := s.serve(editor, map[string]*client.Client{"edits": editor.As("alice@example.com"), "views": viewer})
	c := as("edits")
	remote, views := client.Remote(c.url, "edits"), client.Remote(c.url, "views")
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()

	at := c.place(`
flow: copies
input: { note: any, name: any }
stages:
  copy:
    job: { script: ./copy.js, input: { note: "${input.note}", name: "${input.name}" } }
output: ${stages.copy.output}
`, map[string]string{"./copy.js": `log("copying"); return { name: input.name, copy: blobs.make(blobs.read(input.note).toUpperCase()) }`})

	// A listing followed: the mark, then the run that starts.
	watching, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	updates := make(chan client.Update, 100)
	go remote.Watch(watching, client.Filter{Location: at}, time.Second, func(u client.Update) { updates <- u })
	if u := <-updates; !u.Listed {
		t.Fatalf("the first thing the listing gave is %+v", u)
	}
	id, err := remote.Start(ctx, client.Start{At: at, Labels: map[string]string{"team": "docs"}, Timeout: time.Minute,
		Input: map[string]any{"name": "x", "note": client.InputFile{Name: "note.txt", Contents: strings.NewReader("a note")}}})
	if err != nil || !strings.HasPrefix(id, "editor/") {
		t.Fatalf("the start gave %q, %v", id, err)
	}
	if u := <-updates; u.Run == nil || u.Run.ID != id || !u.New {
		t.Fatalf("the listing gave %+v for the run that started", u)
	}
	stopWatching()
	// Its record followed to its end.
	var entries []client.Entry
	run, err := remote.Follow(ctx, id, time.Second, func(e client.Entry) { entries = append(entries, e) })
	if err != nil || run.Status != "done" || run.Labels["team"] != "docs" || run.Cause.Principal != "alice@example.com" {
		t.Fatalf("the follow gave %+v, %v", run, err)
	}
	logged := false
	for _, e := range entries {
		logged = logged || e.Name == "log" && e.Data == "copying"
	}
	if len(entries) < 3 || !logged || entries[0].Step != 1 || entries[0].Events[0].Cause.Principal != "alice@example.com" {
		t.Fatalf("the follow gave the entries %+v", entries)
	}
	var again []client.Entry
	if _, err := remote.Record(ctx, id, func(e client.Entry) { again = append(again, e) }); err != nil || len(again) != len(entries) {
		t.Fatalf("the record gave %d entries where the follow gave %d (%v)", len(again), len(entries), err)
	}
	// The file of its result.
	files, err := remote.Files(ctx, id)
	if err != nil || len(files) != 1 || files["copy"].Size != 6 {
		t.Fatalf("the files are %+v, %v", files, err)
	}
	contents, file, err := remote.Fetch(ctx, id, "copy")
	if err != nil || file.Size != 6 {
		t.Fatalf("the fetch gave %+v, %v", file, err)
	}
	if data, err := io.ReadAll(contents); err != nil || string(data) != "A NOTE" {
		t.Fatalf("the file holds %q (%v)", data, err)
	}

	// A message and its receipt.
	door := c.place(door, nil)
	waiting, err := remote.Start(ctx, client.Start{At: door})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Taken(ctx, waiting, 0, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	to, err := remote.Address(ctx, waiting)
	if err != nil {
		t.Fatal(err)
	}
	message, err := remote.Send(ctx, to, "knock", nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := remote.WaitReceipt(ctx, message, time.Minute, time.Second); err != nil || !receipt.Taken || receipt.Acted == nil || *receipt.Acted {
		t.Fatalf("the receipt of an event the machine has no rule for is %+v, %v", receipt, err)
	}
	if _, err := remote.WaitReceipt(ctx, message, time.Minute, time.Second); err == nil {
		t.Error("a receipt was given twice")
	}
	if message, err = remote.Cancel(ctx, waiting, 0); err != nil {
		t.Fatal(err)
	}
	if receipt, err := remote.WaitReceipt(ctx, message, 0, time.Second); err != nil || !receipt.Taken {
		t.Fatalf("the receipt of the cancel is %+v, %v", receipt, err)
	}
	if run, err := remote.Wait(ctx, waiting, 10*time.Millisecond); err != nil || run.Status != "cancelled" {
		t.Fatalf("the wait gave %+v, %v", run, err)
	}

	// A refusal is the package's error.
	if _, err := remote.Show(ctx, "editor/20261122T000000.000Z-0000000000"); !errors.Is(err, client.ErrNoRun) {
		t.Errorf("a run that never was gave %v", err)
	}
	if _, err := views.Record(ctx, id, func(client.Entry) {}); !errors.Is(err, client.ErrOthers) || !errors.Is(err, client.ErrNoRun) {
		t.Errorf("another client's record gave %v", err)
	}
	if _, _, err := remote.Fetch(ctx, id, "nothing"); !errors.Is(err, client.ErrNoFile) {
		t.Errorf("a file the run does not show gave %v", err)
	}
	if _, err := remote.Step(ctx, id, 99); !errors.Is(err, client.ErrNoStep) {
		t.Errorf("a step the record does not hold gave %v", err)
	}
	if _, err := remote.Start(ctx, client.Start{At: "nowhere"}); err == nil || !strings.Contains(err.Error(), "a start names the location") {
		t.Errorf("a start at no location gave %v", err)
	}
	if _, err := client.Remote(c.url, "wrong").List(ctx, client.Filter{}); err == nil || !strings.Contains(err.Error(), "does not show a token") {
		t.Errorf("a wrong token gave %v", err)
	}
	// A file is a value of the input's top level, and a tree is one that was
	// read from files.
	if _, err := remote.Start(ctx, client.Start{At: at, Input: map[string]any{"deep": map[string]any{"note": client.InputFile{Name: "n.txt", Contents: strings.NewReader("x")}}}}); !errors.Is(err, client.ErrStart) {
		t.Errorf("a file below the input's top level gave %v", err)
	}
	draft, err := machine.Parse([]byte("flow: made\nstages:\n  a: { job: { script: ./a.js } }\n"), machine.Given(map[string]string{"./a.js": "return 1"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Submit(ctx, &client.Tree{Prefix: "/made", Machines: []*client.Machine{{Path: "made.yaml", Location: "/made", Draft: draft}}}, 0); err == nil || !strings.Contains(err.Error(), "ReadTree") {
		t.Errorf("a tree that was not read from files gave %v", err)
	}
}

// A message no controller takes is, through a gateway, the error the
// package gives for it.
func Test_client_package_gateway_message_not_taken(t *testing.T) {
	_, local, c := aloneOn(t)
	id := c.start(map[string]any{"at": "/door"})
	// The first look for a receipt moves the clock past the time a
	// controller may take the message.
	var ahead atomic.Int64
	local.Now = func() time.Time { return time.Now().Add(time.Duration(ahead.Load())) }
	local.Store = sentThenLooking{local.Store, &atomic.Bool{}, func() {
		ahead.Add(int64(200*time.Millisecond + 2*identity.ClockAllowance + time.Second))
	}}
	if _, err := client.Remote(c.url, c.token).Cancel(context.Background(), id, 200*time.Millisecond); !errors.Is(err, client.ErrNotTaken) {
		t.Errorf("a cancel no controller takes gave %v", err)
	}
}
