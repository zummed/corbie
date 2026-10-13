package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

// makesFiles ends with blobs everywhere a result can hold one: under a
// name, in a list, in a map inside a map, and two written out inside a
// text.
const makesFiles = `
	const text = blobs.read(blobs.make([{ first: input.first }, { second: input.second }], { type: "text/plain" }))
	return {
		report: blobs.make("the report", { type: "text/markdown" }),
		pages: [blobs.make("page 0"), blobs.make("page 1")],
		inner: { notes: blobs.make("the notes") },
		text,
	}
`

// made runs a script that ends with files and gives the run.
func made(t *testing.T, l *client.Local) *client.Run {
	t.Helper()
	r := run(t, l, `
		flow: makes
		stages:
		  parts: { job: { script: ./parts.js } }
		  make: { job: { script: ./make.js, input: "${stages.parts.output}" } }
		output: ${stages.make.output}
	`, map[string]string{"make.js": makesFiles,
		"parts.js": `return { first: blobs.make("first inside"), second: blobs.make("second inside") }`}, nil)
	if r.Status != instance.Done {
		t.Fatalf("the run that makes the files ended %s: %s", r.Status, r.Error)
	}
	return r
}

// The listing of a result's files and the fetching of one agree: every
// blob a path of the result fetches is listed under that path, one written
// out inside a text among them, and every path listed fetches its blob.
func TestFilesListsEveryBlobAPathFetches(t *testing.T) {
	l := local(t)
	r := made(t, l)
	files, err := l.Files(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{ // path: the name it is listed under, and its contents
		"report":      {"report", "the report"},
		"pages[0]":    {"pages-0", "page 0"},
		"pages[1]":    {"pages-1", "page 1"},
		"inner.notes": {"notes", "the notes"},
		"text#1":      {"text-1", "first inside"},
		"text#2":      {"text-2", "second inside"},
	}
	if got := slices.Sorted(maps.Keys(files)); !slices.Equal(got, slices.Sorted(maps.Keys(want))) {
		t.Fatalf("the files listed are at %v", got)
	}
	for at, file := range files {
		contents, fetched, err := l.Fetch(ctx, r.ID, at)
		if err != nil {
			t.Errorf("the file listed at %q cannot be fetched: %v", at, err)
			continue
		}
		text, _ := io.ReadAll(contents)
		if string(text) != want[at][1] || file.Name != want[at][0] || file.Type != fetched.Type || file.Size != int64(len(text)) {
			t.Errorf("the file at %q is listed as %+v and fetched as %q, %+v", at, file, text, fetched)
		}
	}
	// What is shown of the result names the same paths.
	shown := jsonOf(r.Output)
	for at := range want {
		if !strings.Contains(shown, `$blob\":\"`+at+`\"`) && !strings.Contains(shown, `"$blob":"`+at+`"`) {
			t.Errorf("the result shown names no blob at %q: %s", at, shown)
		}
	}
}

// ordered is a client's store that notes each name written, in order.
type ordered struct {
	store.Store
	mu      sync.Mutex
	written []string
}

func (o *ordered) wrote(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.written = append(o.written, name)
}

func (o *ordered) names() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.written)
}

func (o *ordered) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	o.wrote(name)
	return o.Store.Write(ctx, name, value, how)
}

func (o *ordered) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	o.wrote(name)
	return o.Store.WriteFrom(ctx, name, r)
}

func (o *ordered) Copy(ctx context.Context, from, to string) (bool, error) {
	o.wrote(to)
	return o.Store.Copy(ctx, from, to)
}

// reads waits for an event and then reads the files it was started with.
const reads = `
	statechart: reads
	initial: held
	states:
	  held: { on: { go: reading } }
	  reading:
	    job: { script: ./read.js, input: "${input}" }
	    done: over
	  over: { final: true, output: "${states.reading.output}" }
`

// standingAt waits until a run has been taken up.
func standingAt(t *testing.T, l *client.Local, id string) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		r, err := l.Show(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != instance.Waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run %s was never taken up", id)
		}
	}
}

// A blob shown for one run is given to the start of another by adding the
// run it was shown for. The client copies it into the new run's record
// before the start is sent, and the new run reads it after the first run
// is gone. No key passes through the caller's hands.
func TestStartCarriesABlobFromAnotherRun(t *testing.T) {
	l := local(t)
	writes := &ordered{Store: l.Client.Store}
	l.Client.Store = writes
	owner := l.Memory.As(ownerOf())
	a := made(t, l)
	out := a.Output.(map[string]any)
	if strings.Contains(jsonOf(out), "key") {
		t.Fatalf("the result handed to the caller holds a key: %s", jsonOf(out))
	}
	// As it was shown, with the run added; and by its path and run alone.
	report := maps.Clone(out["report"].(map[string]any))
	report["run"] = a.ID
	input := map[string]any{
		"doc":    report,
		"more":   []any{map[string]any{"$blob": "pages[1]", "run": a.ID}},
		"inside": map[string]any{"$blob": "text#2", "run": a.ID},
		"plain":  map[string]any{"run": a.ID, "note": "a value that only names a run"},
	}
	d := draft(t, reads, map[string]string{"read.js": `
		return { doc: blobs.read(input.doc), type: input.doc.type, more: blobs.read(input.more[0]), inside: blobs.read(input.inside), plain: input.plain }
	`})
	before := len(writes.names())
	b, err := l.Start(ctx, d, client.Start{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	// The three blobs are under the new run's record, under the names they
	// have in the first run's, and were written before the start was.
	var copied []int
	sent := -1
	theirs, err := owner.List(ctx, store.Blobs("controller", a.ID))
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range writes.names()[before:] {
		switch {
		case name == store.Message("controller", "client", strings.TrimPrefix(b, "client/")):
			sent = i
		case strings.HasPrefix(name, store.Blobs("controller", b)+"/") && slices.Contains(theirs, store.Blob("controller", a.ID, path.Base(name))):
			copied = append(copied, i)
		}
	}
	if len(copied) != 3 || sent < 0 || slices.Max(copied) > sent {
		t.Fatalf("the start was the write numbered %d and the copies into the new run's record those numbered %v, of %v", sent, copied, writes.names()[before:])
	}
	// The first run goes, record and heads, before the second reads.
	standingAt(t, l, b)
	gone, err := owner.List(ctx, store.Record("controller", a.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append(gone, store.FinishedHead("controller", a.ID)) {
		if _, err := owner.Delete(ctx, name, store.Overwrite); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.Show(ctx, a.ID); !errors.Is(err, client.ErrNoRun) {
		t.Fatalf("the first run, removed, is shown as %v", err)
	}
	if _, err := l.Send(ctx, b, "go", nil, 0); err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, b)
	want := map[string]any{"doc": "the report", "type": "text/markdown", "more": "page 1", "inside": "second inside",
		"plain": map[string]any{"run": a.ID, "note": "a value that only names a run"}}
	if r.Status != instance.Done || jsonOf(r.Output) != jsonOf(want) {
		t.Fatalf("the second run ended %s with %s (%s)", r.Status, jsonOf(r.Output), r.Error)
	}
	// What is shown of the second run's input is a blob of its own.
	step, err := l.Step(ctx, b, 1)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := plainOf(t, step.Events[0].Data).(map[string]any)["doc"].(map[string]any)
	at, _ := doc["$blob"].(string)
	contents, _, err := l.Fetch(ctx, b, at)
	if err != nil || doc["type"] != "text/markdown" || doc["key"] != nil {
		t.Fatalf("the second run's input shows the file as %v: %v", doc, err)
	}
	if text, _ := io.ReadAll(contents); string(text) != "the report" {
		t.Fatalf("the file in the second run's input reads %q", text)
	}
}

// A start that names a blob which is not there, or a run that is not the
// client's, is refused, and nothing is written and nothing sent.
func TestStartCarryingABlobThatIsNotThereSendsNothing(t *testing.T) {
	l := local(t)
	writes := &ordered{Store: l.Client.Store}
	l.Client.Store = writes
	a := made(t, l)
	d := draft(t, reads, map[string]string{"read.js": `return 1`})
	before := len(writes.names())
	for what, shown := range map[string]map[string]any{
		"a path that names nothing":  {"$blob": "nothing", "run": a.ID},
		"a run of another client's":  {"$blob": "report", "run": "another/" + strings.TrimPrefix(a.ID, "client/")},
		"a run that was never there": {"$blob": "report", "run": "client/" + store.Stamp(time.Now()) + "-0000000000"},
		"a name that is no run's":    {"$blob": "report", "run": "nowhere"},
	} {
		// A good one beside it is not copied either.
		input := map[string]any{"good": map[string]any{"$blob": "report", "run": a.ID}, "bad": shown}
		if id, err := l.Start(ctx, d, client.Start{Input: input}); err == nil {
			t.Errorf("a start carrying %s was sent as %s", what, id)
		} else if what == "a path that names nothing" && !errors.Is(err, client.ErrNoFile) {
			t.Errorf("a start carrying %s was refused with %v", what, err)
		}
	}
	if after := writes.names()[before:]; len(after) != 0 {
		t.Fatalf("the refused starts wrote %v", after)
	}
}

// A start gives a file as input under a name: the run's script reads the
// file's bytes there, whatever they are, the input shown has a blob of the
// file's type and size, and the blob is in the new run's record.
func TestStartGivesAFileAsInput(t *testing.T) {
	l := local(t)
	// Bytes that are no text: every value a byte can have, twice over.
	pdf := make([]byte, 512)
	for i := range pdf {
		pdf[i] = byte(i)
	}
	d := draft(t, `
		flow: takes
		stages:
		  read: { job: { script: ./read.js, input: "${input}" } }
		output: ${stages.read.output}
	`, map[string]string{"read.js": `
		const bytes = blobs.read(input.report, { as: "bytes" })
		return { type: input.report.type, size: input.report.size, sum: crypto.hash(bytes), n: input.n }
	`})
	id, err := l.Start(ctx, d, client.Start{Input: map[string]any{"n": 3},
		Files: map[string]client.InputFile{"report": {Name: "report.pdf", Contents: bytes.NewReader(pdf)}}})
	if err != nil {
		t.Fatal(err)
	}
	r := wait(t, l, id)
	sum := run(t, l, `
		flow: sums
		stages:
		  sum: { job: { script: ./sum.js } }
		output: ${stages.sum.output}
	`, map[string]string{"sum.js": `
		const all = new Uint8Array(512)
		for (let i = 0; i < 512; i++) all[i] = i % 256
		return crypto.hash(all)
	`}, nil).Output
	want := map[string]any{"type": "application/pdf", "size": 512.0, "sum": sum, "n": 3.0}
	if r.Status != instance.Done || jsonOf(r.Output) != jsonOf(want) || sum == nil {
		t.Fatalf("the run ended %s with %s (%s); want %s", r.Status, jsonOf(r.Output), r.Error, jsonOf(want))
	}
	step, err := l.Step(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	shown, _ := plainOf(t, step.Events[0].Data).(map[string]any)["report"].(map[string]any)
	at, _ := shown["$blob"].(string)
	if shown["type"] != "application/pdf" || shown["size"] != 512.0 || len(shown) != 3 {
		t.Fatalf("the input shown has the file as %v", shown)
	}
	contents, _, err := l.Fetch(ctx, id, at)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(contents); !bytes.Equal(got, pdf) {
		t.Fatalf("the file fetched from the input is %d bytes that are not the file's", len(got))
	}

	// A file is given under a name of a map: an input that is something
	// else, or that has a value under the name, is refused, and nothing is
	// written.
	writes := &ordered{Store: l.Client.Store}
	l.Client.Store = writes
	for what, input := range map[string]any{"a list": []any{1}, "a text": "text", "a value under the name": map[string]any{"report": 1}} {
		_, err := l.Start(ctx, d, client.Start{Input: input,
			Files: map[string]client.InputFile{"report": {Name: "report.pdf", Contents: bytes.NewReader(pdf)}}})
		if err == nil || len(writes.names()) != 0 {
			t.Errorf("a file given with an input that is %s: %v, and %d values written", what, err, len(writes.names()))
		}
	}
	// With no input, the file is the whole of it.
	id, err = l.Start(ctx, d, client.Start{Files: map[string]client.InputFile{"report": {Name: "notes.txt", Contents: strings.NewReader("notes")}}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != instance.Done || r.Output.(map[string]any)["type"] != "text/plain" || r.Output.(map[string]any)["size"] != 5.0 {
		t.Fatalf("a run given only a file ended %s with %s (%s)", r.Status, jsonOf(r.Output), r.Error)
	}
	if blobs, _ := l.Memory.As(ownerOf()).List(ctx, store.Blobs("controller", id)); len(blobs) != 3 {
		t.Fatalf("the run's record holds %d blobs; its definition, its script and the file are three", len(blobs))
	}
}

// An agent is started with the conversation of another agent's result,
// carried from that run to this one, and carries on from it: its first
// turn is shown the first one's turns followed by the new prompt, and its
// own conversation begins with them.
func TestAgentCarriesOnFromAnotherRunsConversation(t *testing.T) {
	m := newModel(t, calls("call_1", "add", `{"n": 41}`), says("It is 42."), says("And one more is 43."))
	l := local(t, "all")
	first := runAgent(t, l, agentDef(m.URL, ""), scripts)
	if first.Status != instance.Done {
		t.Fatalf("the first agent ended %s: %s", first.Status, first.Error)
	}
	so := turnsOf(first.Output.(map[string]any))
	if len(so) != 2 {
		t.Fatalf("the first agent's conversation is %s", jsonOf(so))
	}
	id, err := l.Start(ctx, draft(t, agentDef(m.URL, "conversation: ${input.so}"), scripts), client.Start{
		Input:   map[string]any{"question": "and one more?", "so": carried(first.Output.(map[string]any)["conversation"], first.ID)},
		Secrets: map[string]string{"TEST_KEY": "sk-test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	second := wait(t, l, id)
	if second.Status != instance.Done || second.Output.(map[string]any)["reply"] != "And one more is 43." {
		t.Fatalf("the second agent ended %s with %s (%s)", second.Status, jsonOf(second.Output), second.Error)
	}
	if len(m.sent) != 3 {
		t.Fatalf("the model was called %d times", len(m.sent))
	}
	// The second agent's first turn: everything the first one's last turn
	// was shown, what it replied, and the new prompt.
	was, now := m.sent[1]["messages"].([]any), m.sent[2]["messages"].([]any)
	if len(now) != len(was)+2 || jsonOf(now[:len(was)]) != jsonOf(was) ||
		!strings.Contains(jsonOf(now[len(was)]), "It is 42.") || !strings.Contains(jsonOf(now[len(was)+1]), "and one more?") {
		t.Fatalf("the second agent's first turn was shown\n  %s\nand the first agent's last turn\n  %s", jsonOf(now), jsonOf(was))
	}
	// Its conversation begins with the first one's turns, as blobs of its
	// own record.
	ours := turnsOf(second.Output.(map[string]any))
	if len(ours) != 3 {
		t.Fatalf("the second agent's conversation is %s", jsonOf(ours))
	}
	for i := range so {
		theirs, _, err1 := l.Fetch(ctx, first.ID, so[i].(map[string]any)["$blob"].(string))
		mine, _, err2 := l.Fetch(ctx, id, ours[i].(map[string]any)["$blob"].(string))
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		a, _ := io.ReadAll(theirs)
		b, _ := io.ReadAll(mine)
		if len(a) == 0 || !bytes.Equal(a, b) {
			t.Errorf("turn %d of the second agent's conversation is not the first one's", i)
		}
	}
	names, err := l.Memory.As(ownerOf()).List(ctx, store.Blobs("controller", id))
	theirs, _ := l.Memory.As(ownerOf()).List(ctx, store.Blobs("controller", first.ID))
	held := 0
	for _, name := range names {
		if slices.Contains(theirs, store.Blob("controller", first.ID, path.Base(name))) {
			held++
		}
	}
	if err != nil || held != 2 {
		t.Fatalf("the second agent's record holds %d of the first one's two turns (%v)", held, err)
	}
}
