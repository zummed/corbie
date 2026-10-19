package client_test

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
)

// A blob whose contents hold another blob's reference, and are typed as
// text, so that what is read from it is text and not data.
const wrapsAsText = `blobs.make([{ inner: input.it, note: "first" }, { note: "second" }], { type: "text/plain" })`

func Test_blob_reference_inside_contents_hides_key(t *testing.T) {
	// Handed to a model: as the preview of the blob, as its items, and as
	// what the model's own code gives back of them.
	code, _ := json.Marshal(map[string]any{"blob": "blob2", "code": `let all = ""; for (const line of items) all += line; return all`})
	m := newModel(t, calls("c1", "make", `{"text": "inner text"}`), calls("c2", "wrap", `{"it": "blob1"}`),
		calls("c3", "blob_read", `{"blob": "blob2"}`), calls("c4", "blob_code", string(code)), says("done"))
	l := local(t, "all")
	files := map[string]string{"add.js": scripts["add.js"], "make.js": insideFiles["make.js"], "wrap.js": `return { list: ` + wrapsAsText + ` }`}
	if r := runAgent(t, l, inside(m), files); r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	// What the model is sent is text within text, so the marks that
	// quote one inside the other are taken off before it is read.
	sent := func(turn int) string { return strings.ReplaceAll(lastShown(m.Calls[turn]), `\`, "") }
	for turn, what := range map[int]string{2: "the preview", 3: "the items", 4: "what the code gave back"} {
		// The blob inside is the handle the model already knows.
		if shown := sent(turn); !strings.Contains(shown, `{"inner":{"blob":"blob1","size":10,"type":"text/plain"}`) || !strings.Contains(shown, "second") {
			t.Errorf("%s was shown as: %s", what, shown)
		}
	}
	if all := strings.ReplaceAll(jsonOf(m.Calls), `\`, ""); strings.Contains(all, "$blob") || strings.Contains(all, `"key"`) {
		t.Fatalf("a blob's reference reached the model: %s", all)
	}

	// Displayed: in a result, and in a line a job logged.
	id, err := l.Start(ctx, draft(t, `
		flow: within
		stages:
		  make: { job: { script: ./make.js, input: { text: inner text } } }
		  wrap: { job: { script: ./wrap.js, input: { it: "${stages.make.output.file}" } } }
		output: ${stages.wrap.output}
	`, map[string]string{"make.js": insideFiles["make.js"], "wrap.js": `const text = blobs.read(` + wrapsAsText + `); log(text); return { text }`}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	var logged string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		if e.Name == "log" {
			logged = jsonOf(e.Data)
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the run ended %+v, %v", r, err)
	}
	for what, shown := range map[string]string{"the result": jsonOf(wait(t, l, id).Output), "the log line": logged} {
		// As any blob is: its type, its size and a name to ask for it by.
		if !strings.Contains(shown, `{\"inner\":{\"$blob\":\"`) || !strings.Contains(shown, `\"size\":10,\"type\":\"text/plain\"}`) || strings.Contains(shown, "key") {
			t.Errorf("%s was shown as: %s", what, shown)
		}
	}
	contents, _, err := l.Fetch(ctx, id, "text#1")
	if err != nil {
		t.Fatal(err)
	}
	if inner, _ := io.ReadAll(contents); string(inner) != "inner text" {
		t.Fatalf("asked for by its name, the blob inside gave %q", inner)
	}
}
