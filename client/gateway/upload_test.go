package gateway_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// fetch asks the gateway for a file of a run and gives the answer's
// status, the type it says the file is, and the file.
func (c caller) fetch(id, path string) (int, string, string) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.url+"/runs/"+id+"/files/"+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	contents, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(contents)
}

// image is a file that is no text: every value a byte can have, and more
// of them than fit one piece of a blob.
func image(size int) string {
	bytes := make([]byte, size)
	for i := range bytes {
		bytes[i] = byte(i * 7)
	}
	return string(bytes)
}

const looks = `
flow: looks
stages:
  look: { job: { script: ./look.js, files: [./lib/names.js], input: "${input}" } }
output: ${stages.look.output}
`

var looksFiles = map[string]string{
	"./look.js": `
		const names = require("./lib/names.js")
		const bytes = blobs.read(input.image, { as: "bytes" })
		return { sum: crypto.hash(bytes), type: input.image.type, size: input.image.size, n: input.n,
			copy: blobs.make(bytes, { type: input.image.type }), says: names.says }`,
	"./lib/names.js": "exports.says = \"naïve \\r\\n\"",
}

// A start is an upload form: a definition, two scripts and an image as
// input go in with the one request, whatever their bytes, and the run's
// script reads the image's bytes unchanged. A file comes out by its path in
// the result, with its type.
func TestStartUploadsItsFilesWhateverTheirBytes(t *testing.T) {
	_, c := served(t)
	picture := image(200_000)
	parts := append(form(c, map[string]any{"definition": looks, "files": looksFiles, "input": map[string]any{"n": 3}, "labels": map[string]string{"team": "docs"}}),
		part{name: "input:image", filename: "photo.png", contents: picture})
	status, started, text := c.upload(parts...)
	id, _ := started["id"].(string)
	if status != 200 || id == "" {
		t.Fatalf("the start was answered %d %s", status, text)
	}
	_, run, text := c.do("GET", "/runs/"+id+"?wait", "")
	out, _ := run["output"].(map[string]any)
	sum := sha256.Sum256([]byte(picture))
	if run["status"] != "done" || out["sum"] != hex.EncodeToString(sum[:]) || out["type"] != "image/png" || out["size"] != float64(len(picture)) ||
		out["n"] != 3.0 || out["says"] != "naïve \r\n" || run["labels"].(map[string]any)["team"] != "docs" {
		t.Fatalf("the run ended %s", text)
	}
	// Out by its path in the result.
	copied, _ := out["copy"].(map[string]any)
	if copied["type"] != "image/png" || copied["size"] != float64(len(picture)) || copied["key"] != nil {
		t.Fatalf("the result shows the file as %v", copied)
	}
	if status, kind, contents := c.fetch(id, copied["$blob"].(string)); status != 200 || kind != "image/png" || contents != picture {
		t.Fatalf("the file of the result was answered %d as %s, %d bytes", status, kind, len(contents))
	}
	// And the input shown has the image as a blob, which is fetched too.
	_, entries := c.events("/runs/" + id + "/record?watch")
	given, _ := entries[0]["events"].([]any)[0].(map[string]any)["data"].(map[string]any)["image"].(map[string]any)
	if given["type"] != "image/png" || given["size"] != float64(len(picture)) || len(given) != 3 {
		t.Fatalf("the input shown has the image as %v", given)
	}
	if status, kind, contents := c.fetch(id, given["$blob"].(string)); status != 200 || kind != "image/png" || contents != picture {
		t.Fatalf("the image of the input was answered %d as %s, %d bytes", status, kind, len(contents))
	}
}

// A part of the form is taken as a field or as a file alike.
func TestStartTakesAPartAsAFieldOrAFile(t *testing.T) {
	_, c := served(t)
	var outputs []string
	for _, parts := range [][]part{
		{{name: "definition", contents: looks}, {name: "file:./look.js", contents: looksFiles["./look.js"]}, {name: "file:lib/names.js", contents: looksFiles["./lib/names.js"]},
			{name: "start", contents: `{"input": {"n": 3}}`}, {name: "input:image", filename: "a.gif", contents: "GIF89a"}},
		{{name: "definition", filename: "looks.yaml", contents: looks}, {name: "file:look.js", filename: "look.js", contents: looksFiles["./look.js"]},
			{name: "file:./lib/names.js", filename: "names.js", contents: looksFiles["./lib/names.js"]},
			{name: "start", filename: "start.json", contents: `{"input": {"n": 3}}`}, {name: "input:image", filename: "a.gif", contents: "GIF89a"}},
	} {
		status, started, text := c.upload(parts...)
		id, _ := started["id"].(string)
		if status != 200 || id == "" {
			t.Fatalf("the start was answered %d %s", status, text)
		}
		_, run, text := c.do("GET", "/runs/"+id+"?wait", "")
		out, _ := run["output"].(map[string]any)
		if run["status"] != "done" || out["type"] != "image/gif" {
			t.Fatalf("the run ended %s", text)
		}
		delete(out, "copy")
		outputs = append(outputs, quotedAny(out))
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("given as fields the run ended with %s, and given as files with %s", outputs[0], outputs[1])
	}
	// An input file given as a field has no name to take a type from.
	status, started, text := c.upload(part{name: "definition", contents: looks}, part{name: "file:./look.js", contents: looksFiles["./look.js"]},
		part{name: "file:./lib/names.js", contents: looksFiles["./lib/names.js"]}, part{name: "input:image", contents: "bytes"})
	if status != 200 {
		t.Fatalf("the start was answered %d %s", status, text)
	}
	if _, run, text := c.do("GET", "/runs/"+started["id"].(string)+"?wait", ""); run["status"] != "done" || run["output"].(map[string]any)["type"] != "application/octet-stream" {
		t.Fatalf("the run ended %s", text)
	}
}

// A start that is not the form a start is, is refused, the answer says
// what is wrong, and nothing is uploaded and nothing sent.
func TestStartThatIsNoUploadFormIsRefused(t *testing.T) {
	c0, c := alone(t)
	definition := part{name: "definition", contents: door}
	for what, sent := range map[string]struct {
		parts []part
		names string
	}{
		"no definition":                  {[]part{{name: "start", contents: `{}`}}, "definition"},
		"two definitions":                {[]part{definition, definition}, "more than once"},
		"a definition as field and file": {[]part{definition, {name: "definition", filename: "door.yaml", contents: door}}, "more than once"},
		"a file with no path":            {[]part{definition, {name: "file:", contents: "return 1"}}, `file:`},
		"an input file with no name":     {[]part{definition, {name: "input:", filename: "a.png", contents: "x"}}, `input:`},
		"a part it does not know":        {[]part{definition, {name: "inputs:image", filename: "a.png", contents: "x"}}, `inputs:image`},
		"a start that is not JSON":       {[]part{definition, {name: "start", contents: `labels: {a: b}`}}, "start"},
		"a start with nothing in it":     {[]part{definition, {name: "start"}}, "start"},
		"a timeout that is no time":      {[]part{definition, {name: "start", contents: `{"timeout": "soon"}`}}, "soon"},
		"a file and an input no map":     {[]part{definition, {name: "start", contents: `{"input": [1]}`}, {name: "input:image", filename: "a.png", contents: "x"}}, "not a map"},
		"a file and a value, one name":   {[]part{definition, {name: "start", contents: `{"input": {"image": 1}}`}, {name: "input:image", filename: "a.png", contents: "x"}}, "image"},
	} {
		if status, _, text := c.upload(sent.parts...); status != 400 || !strings.Contains(text, sent.names) {
			t.Errorf("a start with %s was answered %d %s", what, status, text)
		}
	}
	// A body that is no form at all.
	if status, _, text := c.do("POST", "/runs", `{"definition": "statechart: door"}`); status != 400 || !strings.Contains(text, "upload form") {
		t.Errorf("a start that is JSON was answered %d %s", status, text)
	}
	if names, _ := c0.Store.List(context.Background(), store.Records("controller")+"/client"); len(names) != 0 {
		t.Fatalf("the refused starts left %d values in the store: %v", len(names), names)
	}
}

// looking is a client's store that is told when a value is written from
// a reader, which is how a blob is.
type looking struct {
	store.Store
	writing func()
}

func (l looking) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	l.writing()
	return l.Store.WriteFrom(ctx, name, r)
}

// Nothing of an upload remains on the gateway's machine once the gateway
// has answered the start, also of a file too large to be held in memory.
func TestStartLeavesNothingOfItsUploadsBehind(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	local, _ := served(t)
	kept := func() int64 {
		entries, err := os.ReadDir(temp)
		if err != nil {
			t.Error(err)
		}
		return int64(len(entries))
	}
	// What the gateway keeps on its disk while the start's files go to
	// the store, and at the moment it has answered.
	var during, after atomic.Int64
	local.Client.Store = looking{local.Client.Store, func() { during.Store(max(during.Load(), kept())) }}
	g := &gateway.Gateway{Client: local.Client, Token: "the-token", Interval: 10 * time.Millisecond}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.ServeHTTP(w, r)
		after.Store(kept())
	}))
	t.Cleanup(server.Close)
	c := caller{t: t, url: server.URL, token: "the-token"}
	large := image(5 << 20)
	parts := append(form(c, map[string]any{"definition": looks, "files": looksFiles}), part{name: "input:image", filename: "scan.png", contents: large})
	status, started, text := c.upload(parts...)
	if status != 200 {
		t.Fatalf("the start was answered %d %s", status, text)
	}
	if during.Load() == 0 {
		t.Fatal("the upload was held in memory whole: this shows nothing of what the gateway leaves on its disk")
	}
	if after.Load() != 0 {
		t.Fatalf("once the gateway had answered, %d files of the upload were still on its disk", after.Load())
	}
	_, run, text := c.do("GET", "/runs/"+started["id"].(string)+"?wait", "")
	sum := sha256.Sum256([]byte(large))
	if out, _ := run["output"].(map[string]any); run["status"] != "done" || out["sum"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("the run ended %s", text)
	}
}

// A cancel, an event and an answer each take a life: with no controller
// running, each is answered once that life has passed, saying the message
// was not taken up, and what was sent is good for that life.
func TestCancelEventAndAnswerTakeALife(t *testing.T) {
	mem, _, c := aloneOn(t)
	owner := mem.As(store.Party{Kind: store.Owner})
	ctx := context.Background()
	goodFor := func(message string) time.Duration {
		t.Helper()
		data, _, err := owner.Read(ctx, message)
		if err != nil {
			t.Fatal(err)
		}
		value, err := identity.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		return value.Until.Sub(value.Sent)
	}
	// A start takes one too.
	id := c.start(map[string]any{"definition": door, "life": "90s"})
	if sent, err := owner.List(ctx, store.Inbox("controller")); err != nil || len(sent) != 1 || goodFor(sent[0]) != 90*time.Second {
		t.Fatalf("a start with a life of 90s left %v in the inbox (%v)", sent, err)
	}
	for path, body := range map[string]string{
		"/cancel":  `{"life": "200ms"}`,
		"/events":  `{"event": "open", "life": "200ms"}`,
		"/tickets": `{"ticket": "the-ticket", "value": true, "life": "200ms"}`,
	} {
		before, err := owner.List(ctx, store.Inbox("controller"))
		if err != nil {
			t.Fatal(err)
		}
		began := time.Now()
		status, _, text := c.do("POST", "/runs/"+id+path, body)
		if status != http.StatusGatewayTimeout || !strings.Contains(text, "not taken up") {
			t.Errorf("%s with a life of 200ms and no controller was answered %d %s", path, status, text)
		}
		if took := time.Since(began); took > 10*time.Second {
			t.Errorf("%s with a life of 200ms was answered after %v", path, took)
		}
		after, err := owner.List(ctx, store.Inbox("controller"))
		if err != nil || len(after) != len(before)+1 {
			t.Fatalf("%s left %d messages in the inbox where there were %d (%v)", path, len(after), len(before), err)
		}
		// The one that is new: names sort by when they were made, and a
		// start's name is made before the start is sent.
		sent := slices.DeleteFunc(after, func(name string) bool { return slices.Contains(before, name) })
		if good := goodFor(sent[0]); good != 200*time.Millisecond {
			t.Errorf("what %s sent with a life of 200ms is good for %v", path, good)
		}
		// A life that is no length of time, and a field that is not one a
		// message has, are refused and send nothing.
		for _, wrong := range []string{`{"life": "soon"}`, `{"lief": "1s"}`} {
			if status, _, text := c.do("POST", "/runs/"+id+path, wrong); status != 400 || !strings.Contains(text, wrong[2:6]) {
				t.Errorf("%s given %s was answered %d %s", path, wrong, status, text)
			}
		}
		if again, _ := owner.List(ctx, store.Inbox("controller")); len(again) != len(before)+1 {
			t.Errorf("%s given a life that is none sent a message", path)
		}
	}
}

// A file of one run's result is given as input to another through the
// gateway as it is through the package: the blob as it was shown, with the
// run it was shown for.
func TestStartCarriesAFileFromAnotherRun(t *testing.T) {
	_, c := served(t)
	first := c.start(map[string]any{"definition": makes, "files": map[string]string{"./make.js": `return { report: blobs.make("the report", { type: "text/markdown" }) }`}})
	_, run, text := c.do("GET", "/runs/"+first+"?wait", "")
	shown, _ := run["output"].(map[string]any)["report"].(map[string]any)
	if run["status"] != "done" || shown == nil {
		t.Fatalf("the first run ended %s", text)
	}
	shown["run"] = first
	const reads = `
flow: reads
stages:
  read: { job: { script: ./read.js, input: "${input}" } }
output: ${stages.read.output}
`
	files := map[string]string{"./read.js": `return { text: blobs.read(input.doc), type: input.doc.type }`}
	second := c.start(map[string]any{"definition": reads, "files": files, "input": map[string]any{"doc": shown}})
	if _, run, text := c.do("GET", "/runs/"+second+"?wait", ""); run["status"] != "done" || quotedAny(run["output"]) != `{"text":"the report","type":"text/markdown"}` {
		t.Fatalf("the second run ended %s", text)
	}
	// One that is not there is answered as not found, and starts nothing.
	status, _, text := c.upload(form(c, map[string]any{"definition": reads, "files": files,
		"input": map[string]any{"doc": map[string]any{"$blob": "nothing", "run": first}}})...)
	if status != 404 || !strings.Contains(text, "nothing") {
		t.Fatalf("a start carrying a file that is not there was answered %d %s", status, text)
	}
	if _, _, listed := c.do("GET", "/runs?all", ""); strings.Count(listed, `"id"`) != 2 {
		t.Fatalf("after two starts and one refused, the listing is %s", listed)
	}
}

// One start carries a definition, its script, an input, a label, a run
// secret, a narrowing list and a timeout, and each is in effect for the
// run it starts.
func TestStartCarriesEverythingAStartMay(t *testing.T) {
	local, err := client.NewLocal(context.Background(), permission.Layer{Allow: []string{"all"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	local.Runner.Command = []string{os.Args[0], runner.JobArg}
	t.Cleanup(local.Close)
	c := serve(t, local.Client)
	var shown atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shown.Store(r.Header.Get("Authorization"))
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	const carries = `
flow: carries
input: [url, host, n]
stages:
  call:
    job:
      script: ./call.js
      input: { url: "${input.url}", n: "${input.n}" }
      allow: ["net:${input.host}", "secret:TOKEN@*", "fs.read:/srv/**"]
      timeout: 30s
output: ${stages.call.output}
`
	started := func(n int, timeout string) (map[string]any, string) {
		t.Helper()
		id := c.start(map[string]any{"definition": carries, "files": map[string]string{"./call.js": `
			const status = http.get(input.url, { headers: { Authorization: secrets.get("TOKEN") } }).status
			if (input.n === 2) fs.read("/srv/other/notes.txt")
			if (input.n === 4) fs.read("/srv/reports/kept/notes.txt")
			if (input.n === 3) for (;;) {}
			return { status, n: input.n }`},
			"input":  map[string]any{"url": server.URL, "host": host, "n": n},
			"labels": map[string]string{"team": "docs"}, "secrets": map[string]string{"TOKEN": "the-start's-own-token"},
			"allow": []string{"net:" + host, "secret:TOKEN@*", "fs.read:/srv/reports/**"},
			"deny":  []string{"fs.read:/srv/reports/kept/**"}, "timeout": timeout})
		_, run, text := c.do("GET", "/runs/"+id+"?wait", "")
		return run, text
	}
	run, text := started(1, "30s")
	if run["status"] != "done" || quotedAny(run["output"]) != `{"n":1,"status":200}` || run["labels"].(map[string]any)["team"] != "docs" || shown.Load() != "the-start's-own-token" {
		t.Fatalf("the run ended %s, and its job showed the token %q", text, shown.Load())
	}
	if _, _, listed := c.do("GET", "/runs?all&label=team=docs", ""); !strings.Contains(listed, run["id"].(string)) {
		t.Fatalf("the listing narrowed to the label is %s", listed)
	}
	// What the list narrowed away is refused, and the refusal names the
	// start's list.
	if run, text := started(2, "30s"); run["status"] != "failed" || run["failure"] != "denied" || !strings.Contains(text, "/srv/other/notes.txt") || run["layer"] != "the list the run was started with" {
		t.Fatalf("the run that read what its start narrowed away ended %s", text)
	}
	// So is what the start's list denies, though it allows it too.
	if run, text := started(4, "30s"); run["status"] != "failed" || run["failure"] != "denied" || run["refused"] != "fs.read:/srv/reports/kept/notes.txt" || run["layer"] != "the list the run was started with" {
		t.Fatalf("the run that read what its start denied ended %s", text)
	}
	if run, text := started(3, "300ms"); run["status"] != "failed" || run["failure"] != "timeout" {
		t.Fatalf("the run that passed its timeout ended %s", text)
	}
}
