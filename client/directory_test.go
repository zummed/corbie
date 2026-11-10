package client_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
)

// keptMachine is a singleton that runs until it is told to end.
const keptMachine = `
	statechart: kept
	lifetime: singleton
	initial: running
	states:
	  running:
	    on: { end: over }
	  over: { end: done }
`

// A tree's singleton is started by the submit, a transient file is recorded,
// and a file .corbieignore lists has no location.
func TestTreeStartsItsSingletons(t *testing.T) {
	l := local(t, "all")
	dir := house(t, map[string]string{
		"on/kept.yaml":  keptMachine,
		"plain.yaml":    "flow: plain\nstages:\n  s: { job: { script: ./one.js } }\noutput: ${stages.s.output}\n",
		"one.js":        "return 1",
		"listed.yaml":   "allow: [env:HOME]\n",
		".corbieignore": "listed.yaml\n",
	})
	done := submitTree(t, l, dir, "/house")
	want := []instance.Recorded{
		{Location: "/house/on/kept", Lifetime: "singleton", What: "started", Run: done.Locations[0].Run},
		{Location: "/house/plain", Lifetime: "transient", What: "recorded"},
	}
	if !reflect.DeepEqual(done.Locations, want) || done.Locations[0].Run == "" {
		t.Fatalf("the submit did %+v", done.Locations)
	}
	run := show(t, l, done.Locations[0].Run)
	if run.Status != "running" || run.Input != nil {
		t.Fatalf("the singleton is %s, with input %v", run.Status, run.Input)
	}
	if runs := roots(t, l, done.Locations[0].Run); len(runs) != 1 {
		t.Fatalf("there are %d runs", len(runs))
	}
	// The transient file runs when its tree's client starts it.
	id, err := l.Start(ctx, client.Start{At: "/house/plain"})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" || r.Output != float64(1) {
		t.Fatalf("the run of /house/plain ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

func TestSecondSubmitStartsNothing(t *testing.T) {
	l := local(t, "all")
	dir := house(t, map[string]string{"kept.yaml": keptMachine})
	first := submitTree(t, l, dir, "").Locations[0].Run
	again := submitTree(t, l, dir, "")
	if again.Locations[0].What != "recorded" || len(again.Cancelled) > 0 || again.Before != "" {
		t.Fatalf("the second submit did %+v", again)
	}
	if runs := roots(t, l, first); len(runs) != 1 || runs[0].ID != first {
		t.Fatalf("the runs are %v", runs)
	}
}

// A changed singleton is cancelled and started again on the new file.
func TestChangedSingletonStartedAgain(t *testing.T) {
	l := local(t, "all")
	dir := house(t, map[string]string{"kept.yaml": keptMachine})
	first := submitTree(t, l, dir, "").Locations[0].Run
	write(t, dir, "kept.yaml", dedent(keptMachine)+"title: changed\n")
	done := submitTree(t, l, dir, "")
	second := done.Locations[0].Run
	if done.Locations[0].What != "restarted" || !slices.Equal(done.Cancelled, []string{first}) || second == first {
		t.Fatalf("the submit did %+v", done)
	}
	old, now := show(t, l, first), show(t, l, second)
	if old.Status != "cancelled" || now.Status != "running" || now.Title != "changed" {
		t.Fatalf("the old run is %s, and the new one %s, titled %q", old.Status, now.Status, now.Title)
	}
}

// A file gone from the tree cancels every run at its location.
func TestRemovedFileCancelsItsRuns(t *testing.T) {
	l := local(t, "all")
	dir := house(t, map[string]string{"kept.yaml": keptMachine, "other.yaml": "flow: other\nstages:\n  s: { job: { script: ./one.js } }\n", "one.js": "return 1"})
	first := submitTree(t, l, dir, "/a").Locations[0].Run
	if err := os.Remove(filepath.Join(dir, "kept.yaml")); err != nil {
		t.Fatal(err)
	}
	done := submitTree(t, l, dir, "/a")
	if got := show(t, l, first).Status; got != "cancelled" || done.Locations[0] != (instance.Recorded{Location: "/a/kept", Lifetime: "singleton", What: "removed"}) {
		t.Fatalf("the run is %s, and the submit did %+v", got, done.Locations)
	}
	if _, err := l.Start(ctx, client.Start{At: "/a/kept"}); err != nil {
		t.Fatal(err)
	}
	// A file listed in .corbieignore after a submit is gone from the tree,
	// as a removed file is.
	listed := house(t, map[string]string{"kept.yaml": keptMachine})
	second := submitTree(t, l, listed, "/b").Locations[0].Run
	write(t, listed, ".corbieignore", "kept.yaml\n")
	done = submitTree(t, l, listed, "/b")
	if got := show(t, l, second).Status; got != "cancelled" || len(done.Locations) != 1 || done.Locations[0] != (instance.Recorded{Location: "/b/kept", Lifetime: "singleton", What: "removed"}) {
		t.Fatalf("the listed file's run is %s, and the submit did %+v", got, done.Locations)
	}
}

// A transient file changed with update: drain leaves its running runs on the
// definition they started with, and the next run gets the new one.
func TestDrainLeavesRunsOnTheirDefinition(t *testing.T) {
	l := local(t, "all")
	waits := "statechart: waits\ninitial: w\nstates:\n  w: { on: { go: over } }\n  over: { end: done, output: %s }\n"
	dir := house(t, map[string]string{"waits.yaml": strings.Replace(waits, "%s", "old", 1)})
	submitTree(t, l, dir, "/d")
	old, err := l.Start(ctx, client.Start{At: "/d/waits"})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Taken(ctx, old, 0, interval); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "waits.yaml", strings.Replace(waits, "%s", "new", 1))
	if done := submitTree(t, l, dir, "/d"); len(done.Cancelled) > 0 {
		t.Fatalf("a drain cancelled %v", done.Cancelled)
	}
	fresh, err := l.Start(ctx, client.Start{At: "/d/waits"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{old, fresh} {
		if err := l.Taken(ctx, id, 0, interval); err != nil {
			t.Fatal(err)
		}
		if _, err := l.Send(ctx, addressOf(t, l.Client, id), "go", nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	if a, b := wait(t, l, old), wait(t, l, fresh); a.Output != "old" || b.Output != "new" {
		t.Fatalf("the runs gave %v and %v", a.Output, b.Output)
	}
}

// A file with a problem is reported, and its location keeps what was
// recorded before and its run; the rest of the tree is done.
func TestFileWithAProblemKeepsItsLocation(t *testing.T) {
	l := local(t, "all")
	dir := house(t, map[string]string{"kept.yaml": keptMachine})
	first := submitTree(t, l, dir, "/p").Locations[0].Run
	write(t, dir, "kept.yaml", "statechart: kept\nlifetime: singleton\nnot a field: 1\n")
	write(t, dir, "new.yaml", dedent(keptMachine))
	done := submitTree(t, l, dir, "/p")
	if done.Locations[0].What != "problem" || done.Locations[1].What != "started" || len(done.Cancelled) > 0 {
		t.Fatalf("the submit did %+v", done)
	}
	if got := show(t, l, first).Status; got != "running" {
		t.Fatalf("the run of the file with a problem is %s", got)
	}
}

// A run reads the tree's directory as run.context and its location as
// run.location, and so does a machine it runs as a child.
func TestContextIsReadByARunAndItsChildren(t *testing.T) {
	l := local(t, "all")
	dir := house(t, map[string]string{
		"on/top.yaml": `
			flow: top
			stages:
			  first: { job: { script: ./one.js, input: { at: "${run.context}" } } }
			  kid: { job: { machine: kid.yaml } }
			output: { first: "${stages.first.output}", mine: "${run.context}", at: "${run.location}", tree: "${run.tree}", kid: "${stages.kid.output}" }
		`,
		"kid.yaml":  "flow: kid\nstages:\n  a: { job: { script: ./one.js } }\noutput: [\"${run.context}\", \"${run.location}\", \"${run.tree}\"]\n",
		"on/one.js": "return input && input.at",
		"one.js":    "return 1",
	})
	submitTree(t, l, dir, "/c")
	id, err := l.Start(ctx, client.Start{At: "/c/on/top"})
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	run := wait(t, l, id)
	if want := map[string]any{"first": real, "mine": real, "at": "/c/on/top", "tree": "/c", "kid": []any{real, "/c/on/top", "/c"}}; !reflect.DeepEqual(run.Output, want) {
		t.Fatalf("the output is %v: %s", run.Output, run.Error)
	}
}

// --at is / and the directory's own name by default, and . is read as the
// directory's real name.
func TestTreeIsAtItsNameByDefault(t *testing.T) {
	dir := house(t, map[string]string{"house/kept.yaml": keptMachine})
	tree, err := client.ReadTree(filepath.Join(dir, "house"), "")
	if err != nil || tree.Prefix != "/house" || tree.Machines[0].Location != "/house/kept" {
		t.Fatalf("read %+v, %v", tree, err)
	}
	t.Chdir(filepath.Join(dir, "house"))
	if tree, err = client.ReadTree(".", ""); err != nil || tree.Prefix != "/house" {
		t.Fatalf("read . as %+v, %v", tree, err)
	}
	if _, err := client.ReadTree(".", "/"); err == nil {
		t.Fatal("a tree at / was read")
	}
}

// A file beside a directory of the same name, or with a name no location can
// have, is reported among the files with problems, and the rest of the tree
// is done.
func TestTreeLocationsAreLeaves(t *testing.T) {
	for name, c := range map[string]struct {
		files map[string]string
		bad   []string
	}{
		"beside": {map[string]string{"boiler.yaml": keptMachine, "boiler/x.yaml": keptMachine}, []string{"boiler.yaml", "boiler/x.yaml"}},
		"plus":   {map[string]string{"+.yaml": keptMachine}, []string{"+.yaml"}},
		"hash":   {map[string]string{"a#b.yaml": keptMachine}, []string{"a#b.yaml"}},
		"star":   {map[string]string{"a*.yaml": keptMachine}, []string{"a*.yaml"}},
	} {
		c.files["good.yaml"] = keptMachine
		tree, err := client.ReadTree(house(t, c.files), "/x")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var bad []string
		for _, m := range tree.Machines {
			if m.Problem != nil {
				bad = append(bad, m.Path)
			} else if m.Path != "good.yaml" {
				t.Errorf("%s: %s has no problem", name, m.Path)
			}
		}
		slices.Sort(bad)
		if !slices.Equal(bad, c.bad) {
			t.Errorf("%s: the files with problems are %v", name, bad)
		}
	}
	l := local(t, "all")
	done := submitTree(t, l, house(t, map[string]string{"boiler.yaml": keptMachine, "boiler/x.yaml": keptMachine, "good.yaml": keptMachine}), "/leaf")
	var what []string
	for _, row := range done.Locations {
		what = append(what, row.Location+" "+row.What)
	}
	if want := []string{"/leaf/boiler problem", "/leaf/boiler/x problem", "/leaf/good started"}; !slices.Equal(what, want) {
		t.Fatalf("the submit did %v", what)
	}
	tree, err := client.ReadTree(house(t, map[string]string{".rook/review.yaml": keptMachine}), "/project")
	if err != nil || tree.Machines[0].Location != "/project/.rook/review" {
		t.Fatalf("a file under a dot directory: %+v, %v", tree, err)
	}
	tree, err = client.ReadTree(house(t, map[string]string{".rook/review.yaml": keptMachine, ".corbieignore": ".rook/\n", "good.yaml": keptMachine}), "/project")
	if err != nil || len(tree.Machines) != 1 || tree.Machines[0].Location != "/project/good" {
		t.Fatalf("a listed dot directory: %+v, %v", tree, err)
	}
}

// A file that is not YAML is reported with the same words whether it is read
// alone or in a directory.
func TestNotYAMLSaidOnce(t *testing.T) {
	dir := house(t, map[string]string{"bad.yaml": "flow: w\n# c\n# c\ninput: { days: number? }\n"})
	tree, err := client.ReadTree(dir, "/x")
	if err != nil || len(tree.Machines) != 1 || tree.Machines[0].Problem == nil {
		t.Fatalf("read %+v, %v", tree, err)
	}
	_, alone := client.ReadMachine(dir, "bad.yaml")
	want := "this is not YAML that can be read: line 4: "
	if got := tree.Machines[0].Problem.Error(); !strings.HasPrefix(got, want) || alone == nil || !strings.HasPrefix(alone.Error(), want) {
		t.Fatalf("in a directory %q, alone %v", got, alone)
	}
}

func TestLinksStayInTheContext(t *testing.T) {
	dir := house(t, map[string]string{
		"outside/m.yaml":  "statechart: m\ninitial: e\nstates:\n  e: { end: done }\n",
		"outside/s.js":    "return 1",
		"house/a.yaml":    "flow: a\nstages:\n  s: { job: { script: ./s.js } }\n",
		"house/real.js":   "return 1",
		"house/near.yaml": "flow: near\nstages:\n  s: { job: { script: ./near.js } }\n",
	})
	for link, target := range map[string]string{"house/x.yaml": "../outside/m.yaml", "house/s.js": "../outside/s.js", "house/near.js": "real.js"} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := client.ReadTree(filepath.Join(dir, "house"), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range tree.Machines {
		got[m.Path] = m.Problem == nil
	}
	if want := map[string]bool{"a.yaml": false, "x.yaml": false, "near.yaml": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read %v", got)
	}
}

// A tree reached by a link reads as the directory it names.
func TestLinkedContextReads(t *testing.T) {
	dir := house(t, map[string]string{"real/m.yaml": "statechart: m\ninitial: e\nstates:\n  e: { end: done }\n"})
	link := filepath.Join(dir, "link")
	if err := os.Symlink("real", link); err != nil {
		t.Fatal(err)
	}
	tree, err := client.ReadTree(link, "")
	if err != nil || len(tree.Machines) != 1 || tree.Machines[0].Path != "m.yaml" || tree.Machines[0].Problem != nil || tree.Prefix != "/real" {
		t.Fatalf("read %+v, %v", tree, err)
	}
}

// Every *.yaml is a machine, and one that is not is a problem, however little
// it holds, until .corbieignore lists it.
func TestEveryYAMLIsAMachine(t *testing.T) {
	data := map[string]string{"notes.yaml": "# for later\n", "empty.yaml": "", "list.yaml": "- a\n", "people.yaml": "people: {}\n"}
	tree, err := client.ReadTree(house(t, data), "/x")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range tree.Machines {
		got[m.Path] = m.Problem.Error()
	}
	listIt := "a file in a tree that is no machine is listed in .corbieignore"
	want := map[string]string{
		"notes.yaml":  "the definition is empty; " + listIt,
		"empty.yaml":  "the definition is empty; " + listIt,
		"list.yaml":   "a definition is a mapping; " + listIt,
		"people.yaml": "it gives no type: a machine writes one of agent:, decision:, flow:, statechart: with its name, or takes it from a file it includes; " + listIt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read %q", got)
	}
	data[".corbieignore"] = "# not machines\n\nnotes.yaml\n./empty.yaml\nlist.yaml\npeople.yaml\n"
	if tree, err = client.ReadTree(house(t, data), "/x"); err != nil || len(tree.Machines) != 0 {
		t.Fatalf("read %+v, %v", tree, err)
	}
	if want := []string{"empty.yaml", "list.yaml", "notes.yaml", "people.yaml"}; !slices.Equal(tree.Listed, want) {
		t.Fatalf("listed %v", tree.Listed)
	}
}

// A listed directory is not read, whatever it holds, and a listed file has
// no location. A machine still reads them: by include:, as a child, and as a
// job's files:. A send to a listed path is to no location.
func TestListedFileHasNoLocation(t *testing.T) {
	dir := house(t, map[string]string{
		".corbieignore":      "shared/\nchild.yaml\nrules.yaml\nnothing/here\n",
		"shared/base.yaml":   "stages:\n  s: { job: { script: ./s.js, files: [rules.yaml] } }\n",
		"shared/s.js":        `return require("../rules.yaml").who`,
		"shared/broken.yaml": "flow: [\n",
		"child.yaml":         "flow: child\nstages:\n  a: { job: ./a.js }\noutput: ${stages.a.output}\n",
		"a.js":               "return 'child'",
		"rules.yaml":         "who: alice\n",
		"top.yaml": `
			flow: top
			include: shared/base.yaml
			stages:
			  kid: { job: { machine: child.yaml } }
			output: ["${stages.s.output}", "${stages.kid.output}"]
		`,
		"teller.yaml": "flow: teller\nallow: [send:child]\nstages:\n  tell: { job: { send: { to: { at: child }, event: go } } }\n",
	})
	tree, err := client.ReadTree(dir, "/listed")
	if err != nil {
		t.Fatal(err)
	}
	var at []string
	for _, m := range tree.Machines {
		if m.Problem != nil {
			t.Fatalf("%s: %v", m.Path, m.Problem)
		}
		at = append(at, m.Location)
	}
	if !slices.Equal(at, []string{"/listed/teller", "/listed/top"}) {
		t.Fatalf("the locations are %v", at)
	}
	if want := []string{"child.yaml", "rules.yaml", "shared/"}; !slices.Equal(tree.Listed, want) {
		t.Fatalf("listed %v", tree.Listed)
	}
	if _, empty := tree.Unsent(); !reflect.DeepEqual(empty, map[string][]string{"/listed/child": {"teller.yaml stages.tell.job.send.to"}}) {
		t.Fatalf("a send to a listed file: %v", empty)
	}
	l := local(t, "all")
	submitTree(t, l, dir, "/listed")
	child, err := l.Start(ctx, client.Start{At: "/listed/child"})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, child); r.Status != "failed" || !strings.Contains(r.Error, "nothing is recorded at /listed/child") {
		t.Fatalf("a start of a listed file ended %s: %s", r.Status, r.Error)
	}
	id, err := l.Start(ctx, client.Start{At: "/listed/top"})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" || !reflect.DeepEqual(r.Output, []any{"alice", "child"}) {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// A line of .corbieignore that is no path in the directory fails the read,
// naming the line.
func TestBadIgnoreLineFailsTheRead(t *testing.T) {
	for line, want := range map[string]string{
		"../up":     `.corbieignore line 2: "../up": a line is a path inside the directory`,
		"/top":      `.corbieignore line 2: "/top"`,
		`a\\b`:      `.corbieignore line 2: "a\\\\b"`,
		"a/../../b": `.corbieignore line 2: "a/../../b"`,
		"a\x01b":    `.corbieignore line 2`,
		".":         `.corbieignore line 2: ".": a line is a path inside the directory, not the directory itself`,
		"./":        `.corbieignore line 2: "./"`,
		"*.yaml":    `.corbieignore line 2: "*.yaml": a line is a path inside the directory, not the directory itself, with no .., no / or ! at its start, no \, *, ? or [`,
		"a?.yaml":   `.corbieignore line 2: "a?.yaml"`,
		"[ab].yaml": `.corbieignore line 2: "[ab].yaml"`,
		"!keep":     `.corbieignore line 2: "!keep"`,
	} {
		_, err := client.ReadTree(house(t, map[string]string{".corbieignore": "ok/\n" + line + "\n"}), "/x")
		if err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%q: %v", line, err)
		}
	}
}

// A file of one line places the machine a listed file holds: the location is
// the placed file's, and the type, the name and the rest are the listed
// file's. Two placements are two locations. Read alone from the top of the
// tree, a placed file runs.
func TestOneLinePlacement(t *testing.T) {
	dir := house(t, map[string]string{
		".corbieignore":              ".rook/\n",
		".rook/slack/chat/post.yaml": "flow: post\nreceive: [bot]\nstages:\n  s: { job: { script: ./post.js, input: \"${input}\" } }\noutput: ${stages.s.output}\n",
		".rook/slack/chat/post.js":   "return 'posted ' + input.text",
		"bot/chat/post.yaml":         "include: .rook/slack/chat/post.yaml\n",
		"ops/chat/post.yaml":         "include: .rook/slack/chat/post.yaml\nreceive: [ops]\n",
	})
	tree, err := client.ReadTree(dir, "/x")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tree.Listed, []string{".rook/"}) || len(tree.Machines) != 2 {
		t.Fatalf("read %+v", tree)
	}
	for i, want := range []struct{ at, receive string }{{"/x/bot/chat/post", "bot"}, {"/x/ops/chat/post", "ops"}} {
		m := tree.Machines[i]
		if m.Problem != nil {
			t.Fatalf("%s: %v", m.Path, m.Problem)
		}
		def := m.Draft.Definition
		if m.Location != want.at || def.Type != "flow" || def.Name != "post" || !reflect.DeepEqual(def.Receive, map[string][]string{"": {want.receive}}) {
			t.Fatalf("%s is a %s named %q at %s, receiving %v", m.Path, def.Type, def.Name, m.Location, def.Receive)
		}
		if _, ok := def.Scripts["./.rook/slack/chat/post.js"]; !ok {
			t.Fatalf("%s names %v", m.Path, def.Scripts)
		}
		if got := string(m.Draft.WrittenOut()); !strings.HasPrefix(got, "flow: post") {
			t.Fatalf("%s is written out as\n%s", m.Path, got)
		}
	}
	l := local(t, "all")
	alone, err := client.ReadFile(dir, "bot/chat/post.yaml", "/alone", client.ScriptJob{})
	if err != nil || alone.Machines[0].Problem != nil {
		t.Fatalf("read alone: %v, %v", err, alone.Machines[0].Problem)
	}
	submit, err := l.Submit(ctx, alone, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.WaitReceipt(ctx, submit, 0, interval); err != nil {
		t.Fatal(err)
	}
	id, err := l.Start(ctx, client.Start{At: "/alone", Input: map[string]any{"text": "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" || r.Output != "posted hi" {
		t.Fatalf("the placed file ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

// A bare path reads from the tree's directory and ./ from the file's
// directory, in any file at any depth. ../ is refused however it is written,
// and so is a path from the top of the disk.
func TestPathsReadFromTheContext(t *testing.T) {
	dir := house(t, map[string]string{
		"outside.js":                 "return 1",
		"house/lib/s.js":             "return 1",
		"house/lib/base.yaml":        "stages:\n  s: { job: { script: ./s.js } }\n",
		"house/deep/down/a.yaml":     "flow: a\nstages:\n  s: { job: { script: lib/s.js } }\n  t: { job: { script: ./near.js } }\n",
		"house/deep/down/near.js":    "return 1",
		"house/deep/included.yaml":   "flow: included\ninclude: lib/base.yaml\n",
		"house/up.yaml":              "flow: up\nstages:\n  s: { job: { script: ../outside.js } }\n",
		"house/hidden.yaml":          "flow: hidden\nstages:\n  s: { job: { script: ./lib/../../outside.js } }\n",
		"house/deep/stays.yaml":      "flow: stays\nstages:\n  s: { job: { script: ./x/../lib/s.js } }\n",
		"house/deep/climb.yaml":      "flow: climb\nstages:\n  s: { job: { script: ../lib/s.js } }\n",
		"house/deep/machine.yaml":    "flow: machine\nstages:\n  s: { job: { machine: ../up.yaml } }\n",
		"house/deep/include-up.yaml": "flow: includeup\ninclude: ../lib/base.yaml\n",
		"house/.git/old.yaml":        "flow: old\nnot a field: 1\n",
		"house/data.yaml":            "people: {}\n",
		"house/.corbieignore":        "lib/\ndata.yaml\n",
	})
	if err := os.WriteFile(filepath.Join(dir, "house/top.yaml"), []byte("flow: top\nstages:\n  s: { job: { script: "+filepath.Join(dir, "outside.js")+" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := client.ReadTree(filepath.Join(dir, "house"), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range tree.Machines {
		got[m.Path] = m.Problem == nil
	}
	want := map[string]bool{"deep/down/a.yaml": true, "deep/included.yaml": true, "up.yaml": false, "hidden.yaml": false,
		"deep/stays.yaml": false, "deep/climb.yaml": false, "deep/machine.yaml": false, "deep/include-up.yaml": false, "top.yaml": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read %v", got)
	}
}

// A file loaded with require is found from the file that loads it, whichever way
// the job names it, and a file .corbieignore lists is read so.
func TestRequireFindsWhatAJobNames(t *testing.T) {
	l := local(t)
	dir := house(t, map[string]string{
		".corbieignore":   "rules.yaml\n",
		"rules.yaml":      "who: alice\n",
		"blocks/who.yaml": "flow: who\nstages:\n  s: { job: { script: ./who.js, files: [rules.yaml, ./near.js] } }\noutput: ${stages.s.output}\n",
		"blocks/who.js":   `return [require("../rules.yaml").who, require("./near.js").value]`,
		"blocks/near.js":  "exports.value = 'near'",
		"on/start.yaml":   "flow: start\nstages:\n  s: { job: { machine: blocks/who.yaml } }\noutput: ${stages.s.output}\n",
	})
	submitTree(t, l, dir, "/require")
	id, err := l.Start(context.Background(), client.Start{At: "/require/on/start"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Wait(context.Background(), id, interval)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "done" || !reflect.DeepEqual(r.Output, []any{"alice", "near"}) {
		t.Fatalf("the run ended %s with %v: %s", r.Status, r.Output, r.Error)
	}
}

const interval = 20 * time.Millisecond

// submitTree submits a directory as a tree, and stays until each run it
// cancelled has ended and each it started has been taken up, as corbie run
// submit does.
func submitTree(t *testing.T, l *client.Local, dir, at string) *instance.Submitted {
	t.Helper()
	tree, err := client.ReadTree(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	message, err := l.Submit(ctx, tree, 0)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := l.WaitReceipt(ctx, message, 0, interval)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Taken {
		t.Fatalf("the submit was refused: %s", receipt.Refused)
	}
	for _, id := range receipt.Tree.Cancelled {
		wait(t, l, id)
	}
	for _, done := range receipt.Tree.Locations {
		if done.Run != "" {
			if err := l.Taken(ctx, done.Run, 0, interval); err != nil {
				t.Fatal(err)
			}
		}
	}
	return receipt.Tree
}

// write writes a file of a directory.
func write(t *testing.T, dir, name, text string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// roots gives the root runs, finished ones too.
func roots(t *testing.T, l *client.Local, listed ...string) []*client.Run {
	t.Helper()
	// A run's head is written a moment after its record, so the runs
	// started are waited for before the roots are counted.
	deadline := time.Now().Add(20 * time.Second)
	for {
		runs, err := l.List(ctx, client.Filter{Finished: true})
		if err != nil {
			t.Fatal(err)
		}
		var out []*client.Run
		var ids []string
		for _, run := range runs {
			if run.Parent == "" {
				out = append(out, run)
				ids = append(ids, run.ID)
			}
		}
		all := true
		for _, id := range listed {
			all = all && slices.Contains(ids, id)
		}
		if all || time.Now().After(deadline) {
			return out
		}
		time.Sleep(interval)
	}
}

// show gives a run as it stands.
func show(t *testing.T, l *client.Local, id string) *client.Run {
	t.Helper()
	run, err := l.Show(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

// house writes files into a new directory.
func house(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		write(t, dir, name, dedent(text))
	}
	return dir
}
