package client_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/store"
)

const standing = `
	statechart: standing
	initial: ready
	states:
	  ready:
	    on: { go: working, stop: stopped }
	  working:
	    job: { script: ./work.js, input: "${event.data}" }
	    on:
	      done: ready
	      failed: ready
	  stopped: { end: done, output: "${states.working.output}" }
`

// A chart that is started once and used many times, found by its label.
func TestStandingChartIsFoundAndUsed(t *testing.T) {
	l := local(t)
	id, err := l.Start(ctx, draft(t, standing, map[string]string{"work.js": `return { did: input.n }`}),
		client.Start{Labels: map[string]string{"role": "nightly"}})
	if err != nil {
		t.Fatal(err)
	}
	found := waitFor(t, l, func([]*client.Run) string {
		runs, _ := l.List(ctx, client.Filter{Labels: map[string]string{"role": "nightly"}})
		if len(runs) == 1 && runs[0].Status == "running" {
			return runs[0].ID
		}
		return ""
	})
	if found != id {
		t.Fatalf("the label found %s, want %s", found, id)
	}
	if runs, _ := l.List(ctx, client.Filter{Labels: map[string]string{"role": "other"}}); len(runs) != 0 {
		t.Fatalf("another label found %d runs", len(runs))
	}
	for n := 1; n <= 2; n++ {
		message, _ := l.Send(ctx, id, "go", map[string]any{"n": n}, 0)
		waitFor(t, l, func([]*client.Run) string {
			if r, _ := l.Receipt(ctx, message); r != nil && r.Acted != nil && *r.Acted {
				return "acted"
			}
			return ""
		})
		waitFor(t, l, func(runs []*client.Run) string {
			for _, r := range runs {
				if r.ID == id && r.Jobs == 0 && r.Step >= 1+2*n {
					return "ready"
				}
			}
			return ""
		})
	}
	l.Send(ctx, id, "stop", nil, 0)
	r := wait(t, l, id)
	check(t, r.Status, r.Output, "done", `{"did": 2}`)
}

func TestWatchingAListing(t *testing.T) {
	l := local(t)
	first, _ := l.Start(ctx, draft(t, standing, map[string]string{"work.js": "return 1"}), client.Start{})
	waitFor(t, l, func(runs []*client.Run) string {
		if len(runs) == 1 {
			return "there"
		}
		return ""
	})
	watchCtx, stop := context.WithCancel(ctx)
	defer stop()
	var mu sync.Mutex
	var told []client.Update
	go l.Watch(watchCtx, client.Filter{Finished: true}, 20*time.Millisecond, func(u client.Update) {
		mu.Lock()
		told = append(told, u)
		mu.Unlock()
	})
	seen := func(want func([]client.Update) bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			ok := want(told)
			mu.Unlock()
			if ok {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("the watcher was told %+v", told)
	}
	// What already existed, then the mark of the end of the first listing.
	seen(func(u []client.Update) bool { return len(u) >= 2 && u[0].New && u[0].Run.ID == first && u[1].Listed })
	second, _ := l.Start(ctx, draft(t, standing, map[string]string{"work.js": "return 1"}), client.Start{})
	seen(func(u []client.Update) bool {
		for _, one := range u[2:] {
			if one.Run != nil && one.Run.ID == second && one.New {
				return true
			}
		}
		return false
	})
	l.Send(ctx, first, "stop", nil, 0)
	seen(func(u []client.Update) bool {
		for _, one := range u[2:] {
			if one.Run != nil && one.Run.ID == first && !one.New && one.Run.Status == "done" {
				return true
			}
		}
		return false
	})
	// A head that has not changed is not reported again.
	mu.Lock()
	count := map[string]int{}
	for _, u := range told {
		if u.Run != nil {
			count[u.Run.ID+" step "+string(rune('0'+u.Run.Step))+u.Run.Status]++
		}
	}
	mu.Unlock()
	for what, n := range count {
		if n > 1 {
			t.Errorf("%s was reported %d times", what, n)
		}
	}
}

func Test_client_follow_of_unknown_run_ends(t *testing.T) {
	l := local(t)
	started := time.Now()
	_, err := l.Follow(ctx, "client/20200101T000000.000Z-0000000000", time.Second, func(client.Entry) {})
	if !errors.Is(err, client.ErrNoRun) || time.Since(started) > 5*time.Second {
		t.Fatalf("following a name that is no run: %v after %v", err, time.Since(started))
	}
	if _, err := l.Show(ctx, "not a run's name"); err == nil {
		t.Fatal("a name of the wrong shape was shown")
	}
}

// A start the controller refused is shown as failed, the reason taken from
// its receipt, found from the run's name alone.
func Test_client_start_receipt_found_from_run_name(t *testing.T) {
	l := local(t)
	good := draft(t, standing, map[string]string{"work.js": "return 1"})
	// The definition is spoilt after the client's own check, as a client
	// that does not check would send it.
	good.Definition.Body = []byte(strings.Replace(string(good.Definition.Body), `"go":"stopped"`, `"go":"nowhere"`, 1))
	id, err := l.Start(ctx, good, client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	fresh := client.New(l.ID, l.Controller.ID.Card, l.Store) // knows nothing but the name
	var shown *client.Run
	waitFor(t, l, func([]*client.Run) string {
		shown, err = fresh.Show(ctx, id)
		if err == nil && shown.Status != "waiting" {
			return "answered"
		}
		return ""
	})
	if shown.Status != "failed" || shown.Failure != "error" || !strings.Contains(shown.Error, `no state named "nowhere"`) {
		t.Fatalf("a refused start is shown as %+v", shown)
	}
	if runs, _ := l.List(ctx, client.Filter{Finished: true}); len(runs) != 0 {
		t.Fatalf("a refused start left an instance: %+v", runs[0])
	}
}

// A blob in a result is shown as its type, its size and the path to ask
// for it by; its key is never shown; and the path gives the blob.
func TestBlobInAResult(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: files
		stages:
		  make: { job: { script: ./make.js } }
		output: { report: "${stages.make.output.report}", list: ["${stages.make.output.report}"] }
	`, map[string]string{"make.js": `return { report: blobs.make("the report's text", { type: "text/markdown" }) }`}, nil)
	shown := jsonOf(r.Output)
	if r.Status != "done" || !strings.Contains(shown, `"$blob":"report"`) || !strings.Contains(shown, `"type":"text/markdown"`) || strings.Contains(shown, `"key"`) {
		t.Fatalf("the result is shown as %s", shown)
	}
	for _, path := range []string{"report", "list[0]"} {
		contents, ref, err := l.Fetch(ctx, r.ID, path)
		if err != nil {
			t.Fatal(err)
		}
		if text, _ := io.ReadAll(contents); string(text) != "the report's text" || ref.Type != "text/markdown" {
			t.Fatalf("the file at %s holds %q", path, text)
		}
	}
	if _, _, err := l.Fetch(ctx, r.ID, "nothing.here"); err == nil {
		t.Fatal("a path that names no file gave one")
	}
	// The raw store shows no key and none of the text.
	owner := l.Memory.As(store.Party{Kind: store.Owner})
	names, _ := store.Names(owner.List(ctx, "controller"))
	for _, name := range names {
		raw, _, _ := owner.Read(ctx, name)
		if strings.Contains(string(raw), "the report's text") {
			t.Fatalf("%s shows the blob's contents", name)
		}
	}
}

// A file named in several places is stored once in the run's record.
func Test_client_file_named_twice_uploaded_once(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: shared
		stages:
		  a: { job: { script: ./add.js, input: { n: 1 } } }
		  b: { job: { script: ./add.js, input: { n: 2 } } }
		  c: { job: { script: ./add.js, input: { n: 3 } } }
	`, scripts, nil)
	if r.Status != "done" {
		t.Fatal(r.Error)
	}
	blobs, _ := store.Names(l.Memory.As(store.Party{Kind: store.Owner}).List(ctx, "controller/records/"+r.ID+"/blobs"))
	if len(blobs) != 2 { // the script, and the definition
		t.Fatalf("a script used by three stages left %d blobs in the record", len(blobs))
	}
}

// Whatever the package hands back about a run, turned into JSON, has no
// key of a blob in it: the result, every entry of its record, a child's
// failure, the list of its files, and its definition.
func Test_client_blob_in_package_value_has_no_key(t *testing.T) {
	l := local(t)
	id, err := l.Start(ctx, draft(t, `
		flow: files
		stages:
		  make: { job: { script: ./make.js } }
		  kid:
		    each: ["${stages.make.output.report}"]
		    job: { machine: ./kid.yaml, input: { report: "${item}" } }
		output: { report: "${stages.make.output.report}", kid: "${stages.kid.output}" }
	`, map[string]string{
		"make.js": `
			const report = blobs.make("the report's text", { type: "text/markdown" })
			emit("made", { report })
			return { report }`,
		"kid.yaml": `
			statechart: kid
			initial: over
			states:
			  over: { end: failed, output: { report: "${input.report}" } }`,
	}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	keep := func(what string, v any) {
		t.Helper()
		text := jsonOf(v)
		if strings.Contains(text, `"key"`) {
			t.Errorf("%s holds a key: %s", what, text)
		}
		all = append(all, text)
	}
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) { keep("an entry of the record", e) })
	if err != nil || r.Status != "done" {
		t.Fatalf("the run ended %+v, %v", r, err)
	}
	keep("the result", r)
	for n := 1; ; n++ {
		step, err := l.Step(ctx, id, n)
		if err != nil || step == nil {
			break
		}
		keep("a step", step)
	}
	files, err := l.Files(ctx, id)
	if err != nil || files["report"].Type != "text/markdown" || files["report"].Size != 17 {
		t.Fatalf("the files are %v, %v", files, err)
	}
	keep("the list of files", files)
	_, file, err := l.Fetch(ctx, id, "report")
	if err != nil {
		t.Fatal(err)
	}
	keep("a fetched file", file)
	def, err := l.Definition(ctx, id)
	if err != nil || len(def.Scripts) != 1 || len(def.Machines) != 1 {
		t.Fatalf("the definition is %s, %v", jsonOf(def), err)
	}
	keep("the definition", def)
	// The blob was in each of them: in an emit, in a child's failure, in
	// the result.
	together := strings.Join(all, "\n")
	for _, want := range []string{`"name":"made"`, `a final state with end: failed`, `"type":"text/markdown"`, `"scripts":{`} {
		if !strings.Contains(together, want) {
			t.Errorf("nothing shown holds %s", want)
		}
	}
}

// A child's result that names blobs that are not there does not stop the
// controller or leave the parent waiting: what can be copied is, and the
// rest is handed up as it is and opens nothing.
func Test_blob_reference_to_nothing_handed_up_opens_nothing(t *testing.T) {
	l := local(t)
	key := blob.NewKey()
	ref := func(at, typ, key string) string {
		return fmt.Sprintf(`{ "$blob": %q, type: %q, size: 4, key: %q }`, at, typ, key)
	}
	trust := store.Trust("controller", "client")
	id, err := l.Start(ctx, draft(t, `
		flow: outer
		stages:
		  kid: { job: { machine: ./kid.yaml } }
		  read: { job: { script: ./read.js, input: "${stages.kid.output}" } }
		output: { kid: "${stages.kid.output}", read: "${stages.read.output}" }
	`, map[string]string{
		"kid.yaml": `
			flow: kid
			stages:
			  make: { job: { script: ./make.js } }
			output: "${stages.make.output}"`,
		"make.js": `
			const all = {
			  good: blobs.make("a real file"),
			  none: ` + ref("controller/records/client/x/blobs/none", "text/plain", key) + `,
			  odd: ` + ref("../../etc/passwd", "text/plain", key) + `,
			  other: ` + ref(trust, "text/plain", key) + `,
			  nokey: ` + ref("controller/records/client/x/blobs/none", "text/plain", "x") + `,
			  nodef: ` + ref("controller/records/client/x/blobs/none", blob.Definition, key) + `,
			  notdef: blobs.make("not a definition", { type: "` + blob.Definition + `" }),
			}
			emit("made", all)
			return all`,
		"read.js": `
			let none
			try { blobs.read(input.none, { as: "text" }) } catch (e) { none = e.message }
			return { good: blobs.read(input.good, { as: "text" }), none }`,
	}), client.Start{})
	if err != nil {
		t.Fatal(err)
	}
	waiting, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	r, err := l.Wait(waiting, id, 20*time.Millisecond)
	if err != nil || r.Status != "done" {
		t.Fatalf("the run ended %+v, %v", r, err)
	}
	read := r.Output.(map[string]any)["read"].(map[string]any)
	if read["good"] != "a real file" || !strings.Contains(fmt.Sprint(read["none"]), "not in the store") {
		t.Fatalf("a later job read %v", read)
	}
	if contents, _, err := l.Fetch(ctx, id, "kid.good"); err != nil {
		t.Fatal(err)
	} else if text, _ := io.ReadAll(contents); string(text) != "a real file" {
		t.Fatalf("the file beside them reads %q", text)
	}
	// Nothing but blobs was copied anywhere.
	names, _ := store.Names(l.Memory.As(store.Party{Kind: store.Owner}).List(ctx, store.Records("controller")))
	for _, name := range names {
		if strings.HasSuffix(name, "/"+path.Base(trust)) || strings.HasSuffix(name, "/passwd") {
			t.Errorf("%s was copied as if it were a blob", name)
		}
	}
}
