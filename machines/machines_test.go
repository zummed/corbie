package machines

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/cli"
	corbiectl "github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
)

const review = `statechart: review
input: { document: string }
initial: drafting
states:
  drafting:
    job: { machine: ./writer.yaml, input: { document: "${input.document}" } }
    on: { done: waiting, failed: gave_up }
  waiting:
    ticket: verdict
    on:
      verdict:
        - { go: approved, when: "${event.data.ok}" }
        - { go: drafting }
  approved: { end: done }
  gave_up: { end: failed }
`

const writer = `flow: writer
input: { document: string }
stages:
  draft:
    job: { script: ./write.js, input: { document: "${input.document}" }, files: [./lib.js] }
output: ${stages.draft.output}
`

// machines writes a directory of machines, and gives the page on it, as
// the config says: acting as its controller, or not.
func machines(t *testing.T, c Config) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{
		"review.yaml": review, "writer.yaml": writer, "write.js": "return 1", "lib.js": "module.exports = 1",
		"notes.yaml": "a: list of things\n", ".hidden/secret.yaml": "flow: hidden\n", ".corbieignore": "notes.yaml\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	c.Dir, c.Interval, c.Shell = dir, time.Second, page.Shell{}
	h, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	return h, dir
}

func corbie(t *testing.T, stdin string, args ...string) {
	t.Helper()
	var out, said bytes.Buffer
	if status := cli.Run(context.Background(), args, strings.NewReader(stdin), &out, &said); status != 0 {
		t.Fatalf("corbie %s: %s%s", strings.Join(args, " "), said.String(), out.String())
	}
}

// lab sets one computer up, as corbie init does, with the secret A set,
// and gives its controller and its store.
func lab(t *testing.T) (*identity.Identity, store.Store) {
	t.Helper()
	_, ctl, s := labIn(t)
	return ctl, s
}

// labIn sets one computer up as lab does, and gives its config directory as
// well.
func labIn(t *testing.T) (string, *identity.Identity, store.Store) {
	t.Helper()
	dir := t.TempDir()
	config, at := filepath.Join(dir, "config"), filepath.Join(dir, "store")
	corbie(t, "", "init", "--store", at, "--name", "lab", "--ceiling", "all", "--config", config)
	corbie(t, "swordfish", "controller", "secret", "set", "A", "-", "--config", config)
	ctl, err := identity.Config{Dir: config}.Load(t.Context(), identity.Controller)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(at)
	if err != nil {
		t.Fatal(err)
	}
	return config, ctl, s
}

func do(h http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	h.ServeHTTP(w, r)
	return w
}

// project writes a directory laid out as a project of ready-made machines
// is: a machine to keep running, machines in a directory whose name begins
// with a dot, and an agent with a toolkit from a file of its own, with the
// files that are no machine listed in .corbieignore.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{
		"router.yaml": "# the router of the bot\nflow: router\nstages:\n  listen: { job: { machine: ./.kit/listen.yaml } }\n",
		"later.yaml":  "flow: later\nstages:\n  post: { job: { machine: ./.kit/post.yaml } }\n",
		".kit/listen.yaml": `statechart: listen
initial: hearing
states:
  hearing:
    job: ./hear.js
    on:
      message: { send: { to: { at: .kit/conversation, key: "a" }, event: said } }
      done: talking
  talking:
    job: { machine: ./conversation.yaml }
    on: { done: hearing }
`,
		".kit/conversation.yaml": `# one conversation
statechart: conversation
initial: answering
states:
  answering:
    job:
      machine:
        flow: turn
        stages:
          answer: { job: { machine: support.yaml } }
          post: { job: { machine: ./post.yaml } }
    on: { done: over }
  over: { end: done }
`,
		".kit/unused.yaml":      "flow: unused\nstages:\n  a: { job: ./hear.js }\n",
		".kit/post.yaml":        "flow: post\nstages:\n  a: { job: ./hear.js }\n",
		".kit/hear.js":          "return 1",
		"support.yaml":          "agent: support\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\nask: { machine: .kit/post.yaml }\ntoolkits:\n  messages: { include: ./toolkits/toolkit.yaml }\n",
		"toolkits/toolkit.yaml": "description: messages\ntools:\n  tell: { description: tells, job: { machine: .kit/post.yaml, approve: true } }\n",
		"people/scopes.yaml":    "people: []\n",
		".corbieignore":         "toolkits/\npeople/scopes.yaml\nshared/\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	return dir
}

const shipper = `flow: shipper
allow: [net:x, secret:A@net:x]
stages:
  ship:
    job: { script: ./write.js, input: { hub: "${values.HUB}" }, allow: [net:y, secret:B@net:y, net:z, "secret:*@net:z"] }
`

// later writes its own allow: after its stages.
const later = `flow: later
stages:
  ship:
    job: { script: ./write.js, allow: [secret:B@net:y] }
allow: [secret:A@net:x]
`

// listed gives the list as directory, file, type and name, lifetime and
// problem count.
func listed(ix *index) []string {
	var out []string
	for _, g := range ix.Groups {
		for _, e := range g.Entries {
			out = append(out, strings.Join(slices.DeleteFunc([]string{g.Dir, e.Path, e.First, e.Lifetime, fmt.Sprint(e.Problems)}, func(f string) bool { return f == "" }), " "))
		}
	}
	return out
}

func Test_machines_list_shows_what_each_item_is(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{
		"grouse/b.yaml":      "flow: b\nlifetime: singleton\nstages:\n  a: { job: { machine: .kit/post.yaml } }\n",
		"bad.yaml":           "flow: bad\nstages: {}\ncolour: blue\n",
		"keyed.yaml":         "statechart: keyed\nlifetime: keyed\ninclude: ./shared/states.yaml\n",
		"shared/states.yaml": "initial: a\nstates:\n  a: { on: { go: b } }\n  b: { end: done }\n",
		"prompt.md":          "Be brief.\n",
		"talker.yaml":        "agent: talker\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\nsystem: { include: ./prompt.md }\n",
	})
	d, _ := newDir(dir)
	ix := d.index("")
	got := listed(ix)
	want := []string{
		". bad.yaml flow: bad 2", ". keyed.yaml statechart: keyed keyed 0", ". later.yaml flow: later transient 0", ". router.yaml flow: router transient 0",
		". support.yaml agent: support transient 0", ". talker.yaml agent: talker transient 0",
		".kit .kit/conversation.yaml statechart: conversation transient 0", ".kit .kit/listen.yaml statechart: listen transient 0", ".kit .kit/post.yaml flow: post transient 0", ".kit .kit/unused.yaml flow: unused transient 0",
		"grouse grouse/b.yaml flow: b singleton 0",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the list is\n%s\nand not\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var files []string
	for _, f := range ix.Files {
		files = append(files, fmt.Sprintf("%s %t %d", f.Path, f.Listed, f.Included))
	}
	if want := []string{"people/scopes.yaml true 0", "prompt.md false 1", "shared/ true 0", "shared/states.yaml true 1", "toolkits/ true 0", "toolkits/toolkit.yaml true 1"}; !slices.Equal(files, want) {
		t.Errorf("the files are %v, and not %v", files, want)
	}
	if ix.Prefix != "/"+filepath.Base(dir) || ix.Groups[len(ix.Groups)-1].Entries[0].Location != ix.Prefix+"/grouse/b" {
		t.Errorf("the prefix is %q and the locations %+v", ix.Prefix, ix.Groups)
	}
	h, _ := New(Config{Dir: dir, Interval: time.Second})
	body := do(h, "GET", "/machines?file=grouse/b.yaml", nil).Body.String()
	_, list, _ := strings.Cut(body, `id="files"`)
	list, _, _ = strings.Cut(list, "</nav>")
	list, filesList, _ := strings.Cut(list, `<h2 class="kind">Files<a href="/machines?file=.corbieignore">.corbieignore</a></h2>`)
	for _, want := range []string{`<h2 class="group" title="grouse">grouse/</h2>`, `<h2 class="group" title=".kit">.kit/</h2>`, `<use href="#g-agent"/>`, "<small>agent: support · transient</small>", "<small>flow: b · singleton</small>", `title="` + ix.Prefix + `/grouse/b">b.yaml<`, `<span class="bad" title="2 problems">2</span>`, `href="/machines?file=grouse%2fb.yaml" aria-current="page"`} {
		if !strings.Contains(list, want) {
			t.Errorf("the list does not show %s:\n%s", want, list)
		}
	}
	for _, gone := range []string{"people/scopes.yaml", "toolkit.yaml"} {
		if strings.Contains(list, gone) {
			t.Errorf("the machines hold %s:\n%s", gone, list)
		}
	}
	for _, want := range []string{">toolkits/toolkit.yaml</span><small>listed in .corbieignore · included by 1 machine</small>", ">people/scopes.yaml</span><small>listed in .corbieignore · included by none</small>", ">prompt.md</span><small>included by 1 machine</small>", `<div class="row"><span></span><span class="name" title="shared/">shared/</span><small>listed in .corbieignore</small></div>`} {
		if !strings.Contains(filesList, want) {
			t.Errorf("the files do not show %s:\n%s", want, filesList)
		}
	}
	if !strings.Contains(body, ">at "+ix.Prefix+"<") {
		t.Errorf("the page does not give the prefix:\n%s", body)
	}
	shared := do(h, "GET", "/machines?file=shared/states.yaml", nil).Body.String()
	if !strings.Contains(shared, `<section class="included"><h2>Included by</h2><a href="/machines?file=keyed.yaml">keyed.yaml</a></section>`) || !strings.Contains(shared, `<span class="facts">listed in .corbieignore</span>`) || strings.Contains(shared, ">Drawing<") {
		t.Errorf("a listed file does not show the machines that include it:\n%s", shared)
	}
	prompt := do(h, "GET", "/machines?file=prompt.md", nil).Body.String()
	if !strings.Contains(prompt, `<section class="included"><h2>Included by</h2><a href="/machines?file=talker.yaml">talker.yaml</a></section>`) || !strings.Contains(prompt, `<span class="facts">a file</span>`) {
		t.Errorf("a file a machine includes does not show the machines that include it:\n%s", prompt)
	}
	script := do(h, "GET", "/machines?file=.kit/hear.js", nil).Body.String()
	if !strings.Contains(script, `<span class="facts">a file</span>`) || strings.Contains(script, "Included by") {
		t.Errorf("a script no machine takes a value from is shown as one of the files:\n%s", script)
	}
	if body := do(h, "GET", "/machines?file=./grouse/b.yaml", nil).Body.String(); !strings.Contains(body, `aria-current="page"`) {
		t.Errorf("a path written with ./ is not the file chosen")
	}
}

// Every .yaml file is a machine but those .corbieignore lists: one with no
// type is a machine with a problem, and its type may be written after other
// keys or come from a file it includes.
func Test_machines_every_yaml_is_a_machine_but_the_listed_files(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{
		"loose.yaml":        "people: []\n",
		"late.yaml":         "stages:\n  a: { job: .kit/hear.js }\nflow: late\n",
		"placed.yaml":       "include: shared/flow.yaml\n",
		"shared/flow.yaml":  "flow: placed\nstages:\n  a: { job: .kit/hear.js }\n",
		"broken-late.yaml":  "colour: blue\nstatechart: broken-late\ninitial: a\nstates:\n  a: { end: done }\n",
		"two.yaml":          "flow: a\nagent: b\n",
		"shared/notes.yaml": "flow: noted\n",
		"broken.yaml":       "flow: broken\nstages: [ { a:\n",
		"nul.yaml":          "flow:\nstages:\n  a: { job: .kit/hear.js }\n",
		".corbieignore":     "toolkits/\npeople/scopes.yaml\nshared/\n.rook/\n",
		".rook/AGENTS.md":   "Read this.\n",
		".rook/x.js":        "return 1",
		".rook/post.yaml":   "flow: post\nstages:\n  a: { job: ./x.js }\n",
		"post.yaml":         "include: .rook/post.yaml\n",
	})
	d, _ := newDir(dir)
	ix := d.index("")
	got := listed(ix)
	for _, want := range []string{". late.yaml flow: late transient 0", ". placed.yaml flow: placed transient 0", ". broken-late.yaml statechart: broken-late 1", ". loose.yaml 1", ". two.yaml 1", ". broken.yaml 1", ". post.yaml flow: post transient 0"} {
		if !slices.Contains(got, want) {
			t.Errorf("the list does not hold %s:\n%s", want, strings.Join(got, "\n"))
		}
	}
	if !slices.ContainsFunc(ix.Problems, func(p problem) bool { return p.File == "loose.yaml" }) {
		t.Errorf("a .yaml file with no type that .corbieignore does not list gives no problem: %+v", ix.Problems)
	}
	for _, f := range ix.Files {
		if f.Path == "loose.yaml" || f.Path == "two.yaml" {
			t.Errorf("%s is listed with the files that are no machine", f.Path)
		}
	}
	if f := ix.file("shared/notes.yaml"); f == nil || !f.Listed || f.Included != 0 || slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "noted") }) {
		t.Errorf("a listed file with a type is %+v, and the list %v", f, got)
	}
	if f := ix.file("shared/flow.yaml"); f == nil || !f.Listed || f.Included != 1 {
		t.Errorf("a listed file a machine includes is %+v", f)
	}
	// A machine whose one type key has no name is listed by its type alone.
	if !slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, ". nul.yaml flow ") }) {
		t.Errorf("a machine with a type and no name is not listed by its type alone:\n%s", strings.Join(got, "\n"))
	}
	// A listed directory is one row; a file below it is a row only where a
	// machine includes it.
	var files []string
	for _, f := range ix.Files {
		files = append(files, fmt.Sprintf("%s %t %d", f.Path, f.Listed, f.Included))
	}
	if want := []string{".rook/ true 0", ".rook/post.yaml true 1"}; !slices.Equal(slices.DeleteFunc(files, func(f string) bool { return !strings.HasPrefix(f, ".rook/") }), want) {
		t.Errorf("the files under a listed directory are %v, and not %v", files, want)
	}
	h, _ := New(Config{Dir: dir, Interval: time.Second})
	if body := do(h, "GET", "/machines?file=.rook/AGENTS.md", nil).Body.String(); !strings.Contains(body, `<span class="facts">listed in .corbieignore</span>`) || !strings.Contains(body, "no machine takes a value from this file") {
		t.Errorf("a file in a listed directory is not headed as listed:\n%s", body)
	}
	if body := do(h, "GET", "/machines?file=.corbieignore", nil).Body.String(); !strings.Contains(body, `<a href="/machines?file=.corbieignore" aria-current="page">.corbieignore</a></h2>`) || !strings.Contains(body, ".rook/") {
		t.Errorf("the Files heading does not open .corbieignore:\n%s", body)
	}
	for file, head := range map[string]string{"broken.yaml": "a machine", "nul.yaml": "flow"} {
		if body := do(h, "GET", "/machines?file="+file, nil).Body.String(); !strings.Contains(body, `<span class="facts">`+head+`</span>`) {
			t.Errorf("%s is not headed %q:\n%s", file, head, body)
		}
	}
	body := do(h, "GET", "/machines?file=loose.yaml", nil).Body.String()
	if !strings.Contains(body, `<span class="facts">a machine</span>`) || !strings.Contains(body, `aria-label="file"><title>file</title><use href="#g-file"/></svg><span class="name" title="/`+filepath.Base(dir)+`/loose">loose.yaml</span><span class="bad" title="1 problem">1</span><small></small>`) {
		t.Errorf("a machine with no type is not shown as a machine with a problem:\n%s", body)
	}
	if body := do(h, "GET", "/machines?file=shared/flow.yaml", nil).Body.String(); !strings.Contains(body, `<span class="facts">listed in .corbieignore</span>`) || !strings.Contains(body, `<a href="/machines?file=placed.yaml">placed.yaml</a>`) || strings.Contains(body, ">Drawing<") {
		t.Errorf("a listed file does not show the machines that include it:\n%s", body)
	}
}

func Test_machines_list_says_what_it_could_not_read(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every directory")
	}
	dir := project(t)
	locked := filepath.Join(dir, "a-locked")
	os.Mkdir(locked, 0o000)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	h, _ := New(Config{Dir: dir, Interval: time.Second})
	if body := do(h, "GET", "/machines", nil).Body.String(); !strings.Contains(body, `class="unread"`) || !strings.Contains(body, "a-locked") {
		t.Errorf("a directory that cannot be read is not reported:\n%s", body)
	}
}

func Test_machines_a_location_with_one_below_it_is_a_problem_of_its_files(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{"support/x.yaml": "flow: x\nstages:\n  a: { job: ./x.js }\n"})
	d, _ := newDir(dir)
	ix := d.index("")
	if ix.Unread != "" || len(ix.at("support.yaml")) == 0 || len(ix.at("support/x.yaml")) == 0 || len(ix.at("later.yaml")) != 0 {
		t.Errorf("a file beside a directory of the same name is not a problem of it and the files below it: %+v, says %q", ix.Groups, ix.Unread)
	}
	if !strings.Contains(fmt.Sprint(ix.Problems), "beside a directory of the same name") {
		t.Errorf("the problems do not say why: %+v", ix.Problems)
	}
}

func Test_machines_problems_are_listed_once_at_their_file(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"a.yaml":                  "agent: a\n" + agentHead + "toolkits:\n  messages: { include: ./toolkits/m/toolkit.yaml }\n",
		"b.yaml":                  "agent: b\n" + agentHead + "toolkits:\n  msgs: { include: ./toolkits/m/toolkit.yaml }\n",
		".kit/c.yaml":             "agent: c\n" + agentHead + "toolkits:\n  messages: { include: toolkits/m/toolkit.yaml }\n",
		"toolkits/m/toolkit.yaml": "description: messages\ntools:\n  tell:\n    does: tells\n    job: { script: ./tell.js }\n",
		"toolkits/m/tell.js":      "return 1",
		"z.yaml":                  "flow: z\nstages:\n  a: { job: ./z.js }\ncolour: blue\n",
		"z.js":                    "return 1",
		".corbieignore":           "toolkits/\n",
	})
	d, _ := newDir(dir)
	ix := d.index("")
	var atToolkit int
	for _, p := range ix.Problems {
		switch p.File {
		case "toolkits/m/toolkit.yaml":
			atToolkit++
			if !slices.Equal(p.Breaks, []string{".kit/c.yaml", "a.yaml", "b.yaml"}) || !strings.HasPrefix(p.Place, "tools.tell") || p.Line == 0 {
				t.Errorf("a problem in the toolkit is %+v", p)
			}
		case "z.yaml":
			if !slices.Equal(p.Breaks, []string{"z.yaml"}) || p.Place != "colour" || p.Line != 4 {
				t.Errorf("a problem in a machine's own file is %+v", p)
			}
		default:
			t.Errorf("a problem is at %+v", p)
		}
	}
	if atToolkit == 0 || len(ix.Problems) != atToolkit+1 {
		t.Errorf("the problems are %+v", ix.Problems)
	}
	h, _ := New(Config{Dir: dir, Interval: time.Second})
	body := do(h, "GET", "/machines?file=z.yaml", nil).Body.String()
	if n := strings.Count(body, "breaks 3 machines"); n != atToolkit {
		t.Errorf("the page shows the toolkit's problems %d times, and not %d:\n%s", n, atToolkit, body)
	}
	// The Toolkits page lists the same problems, drawn the same way.
	toolkits := do(h, "GET", "/toolkits", nil).Body.String()
	box := func(page string) string {
		start := strings.Index(page, `<div class="problems">`)
		if start < 0 {
			return ""
		}
		return page[start : start+strings.Index(page[start:], "</div>")]
	}
	if box(toolkits) == "" || box(toolkits) != box(do(h, "GET", "/machines", nil).Body.String()) {
		t.Errorf("the Toolkits page lists the problems %q, and not as the Machines page does", box(toolkits))
	}

	// A file that three machines include, which cannot be read, is one
	// problem at that file, however each machine writes its path.
	for name, base := range map[string]string{"not YAML": "stages: [ { a: \n", "missing": ""} {
		dir := t.TempDir()
		files := map[string]string{
			"w.js":           "return 1",
			"one.yaml":       "flow: one\ninclude: ./base.yaml\n",
			"two.yaml":       "flow: two\ninclude: ./base.yaml\n",
			"sub/three.yaml": "flow: three\ninclude: base.yaml\n",
			".corbieignore":  "base.yaml\n",
		}
		if base != "" {
			files["base.yaml"] = base
		}
		write(t, dir, files)
		d, _ := newDir(dir)
		ix := d.index("")
		if len(ix.Problems) != 1 || ix.Problems[0].File != "base.yaml" || len(ix.Problems[0].Breaks) != 3 {
			t.Errorf("a %s file three machines include gives the problems %+v", name, ix.Problems)
		}
		if got := listed(ix); slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, " base.yaml ") }) {
			t.Errorf("with a %s file included, the list is %v", name, got)
		}
	}

	// A file the include: names that .corbieignore does not list is a
	// machine: where it cannot be read, its own problem is the one problem,
	// breaking it and each machine that includes it.
	dir = t.TempDir()
	write(t, dir, map[string]string{
		"one.yaml":  "flow: one\ninclude: ./base.yaml\n",
		"two.yaml":  "flow: two\ninclude: ./base.yaml\n",
		"base.yaml": "stages: [ { a: \n",
	})
	d, _ = newDir(dir)
	ix = d.index("")
	if len(ix.Problems) != 1 || ix.Problems[0].File != "base.yaml" || !slices.Equal(ix.Problems[0].Breaks, []string{"base.yaml", "one.yaml", "two.yaml"}) {
		t.Errorf("an unlisted file two machines include, which cannot be read, gives the problems %+v", ix.Problems)
	}
	if at := ix.at("one.yaml"); len(at) != 1 || at[0].File != "base.yaml" || len(at[0].Breaks) != 3 {
		t.Errorf("a machine that includes it shows the problems %+v", at)
	}
}

func Test_machines_directory_below_the_top_is_one_problem(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"proj/a.yaml":        "flow: a\nstages:\n  x: { job: { machine: b.yaml } }\n",
		"proj/b.yaml":        "flow: b\ndescription: b\nstages:\n  x: { job: ./r.js }\n",
		"proj/r.js":          "return 1",
		"proj/sub/c.yaml":    "flow: c\nstages:\n  x: { job: { machine: b.yaml } }\n",
		"proj/d.yaml":        "agent: d\n" + agentHead + "toolkits:\n  k: { description: k, tools: { b: { call: b } } }\n",
		"proj/e.yaml":        "agent: e\n" + agentHead + "toolkits:\n  k: { description: k, tools: { f: { call: lib/f } } }\n",
		"proj/lib/f.yaml":    "flow: f\ndescription: f\nstages:\n  x: { job: ./g.js }\n",
		"proj/lib/g.js":      "return 1",
		"proj/.corbieignore": "lib/\n",
		"z.yaml":             "flow: z\nstages: {}\ncolour: blue\n",
	})
	d, _ := newDir(dir)
	ix := d.index("")
	if len(ix.Problems) == 0 {
		t.Fatalf("no problems")
	}
	top := ix.Problems[0]
	// proj/d.yaml calls a location of the tree proj is the top of.
	// proj/e.yaml calls one proj/.corbieignore lists, so it reads with
	// problems from proj too.
	if !top.Top || top.File != "proj" || !slices.Equal(top.Breaks, []string{"proj/a.yaml", "proj/d.yaml", "proj/sub/c.yaml"}) {
		t.Errorf("the first problem is %+v", top)
	}
	for _, p := range ix.Problems[1:] {
		if p.File != "z.yaml" && p.File != "proj/e.yaml" {
			t.Errorf("a machine read from the wrong directory gave %+v", p)
		}
	}
	if !slices.ContainsFunc(ix.Problems, func(p problem) bool { return p.File == "proj/e.yaml" }) {
		t.Errorf("a machine that calls a location proj/.corbieignore lists is not listed with its own problems: %+v", ix.Problems)
	}
	if got := listed(ix); !slices.Contains(got, "proj proj/a.yaml flow: a 1") {
		t.Errorf("a machine read from the wrong directory counts other than the one problem shown at it: %v", got)
	}
	h, _ := New(Config{Dir: dir, Interval: time.Second})
	if body := do(h, "GET", "/machines", nil).Body.String(); !strings.Contains(body, "<code>proj/</code> is the top of a tree") || !strings.Contains(body, "breaks 3 machines") {
		t.Errorf("the page does not say which directory is the top:\n%s", body)
	}
	if body := do(h, "GET", "/machines?file=proj/a.yaml", nil).Body.String(); strings.Count(body, "is the top of a tree") != 2 || strings.Contains(body, "cannot be read") {
		t.Errorf("a machine read from the wrong directory does not say so:\n%s", body)
	}
}

func Test_machines_text_shows_the_problems_found_in_it(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{
		"broken.yaml":     "# one stage\nflow: broken\nstages:\n  a: { job: ./missing.js }\n",
		"a.yaml":          "agent: a\n" + agentHead + "toolkits:\n  m: { include: ./toolkits/m.yaml }\n",
		"toolkits/m.yaml": "description: m\ntools:\n  r:\n    description: r\n    job: { script: ./r.js }\n    colour: blue\n",
		"toolkits/r.js":   "return 1",
	})
	h, _ := New(Config{Dir: dir, Interval: time.Second})
	body := do(h, "GET", "/machines?file=broken.yaml", nil).Body.String()
	if strings.Contains(body, `id="drawing"`) || !strings.Contains(body, "missing.js") || !strings.Contains(body, `<span class="l err" id="L4"><span class="k">  a</span>`) && !strings.Contains(body, `<span class="l err" id="L4">  <span class="k">a</span>`) {
		t.Errorf("a machine with problems is not shown as text with its line marked:\n%s", body)
	}
	if !strings.Contains(body, `<span class="l" id="L1"><span class="c"># one stage</span></span>`) {
		t.Errorf("a comment is not set apart:\n%s", body)
	}
	toolkit := do(h, "GET", "/machines?file=toolkits/m.yaml&view=text", nil).Body.String()
	if !strings.Contains(toolkit, "tools.r.colour") || !strings.Contains(toolkit, "breaks 1 machine") || strings.Count(toolkit, `class="l err"`) != 1 {
		t.Errorf("a toolkit's text does not show the problem found in it:\n%s", toolkit)
	}
	if a := do(h, "GET", "/machines?file=a.yaml", nil).Body.String(); !strings.Contains(a, "tools.r.colour") || !strings.Contains(a, "breaks 1 machine") || strings.Contains(a, `class="l err"`) {
		t.Errorf("a machine marks its lines for a problem in a file it includes:\n%s", a)
	}
	for _, file := range []string{"people/scopes.yaml", "support.yaml"} {
		text := do(h, "GET", "/machines?view=text&file="+file, nil).Body.String()
		if strings.Contains(text, `class="found"`) || strings.Contains(text, "No problems") || strings.Contains(text, "<textarea") || !strings.Contains(text, `<pre class="code"`) {
			t.Errorf("%s is not shown as read-only text with no problems:\n%s", file, text)
		}
	}
}

func Test_machines_drawing_unfolds_as_the_address_says(t *testing.T) {
	h, err := New(Config{Dir: project(t), Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	body := do(h, "GET", "/machines?file=.kit/listen.yaml", nil).Body.String()
	if !strings.Contains(body, ">hearing<") || !strings.Contains(body, ">.kit/conversation#a<") {
		t.Fatalf("the machine is not drawn:\n%s", body)
	}
	if regexp.MustCompile(`<a href="(/machines\?[^"]*)"><g class="[^"]*sends`).MatchString(body) {
		t.Errorf("a send, which starts no child, unfolds:\n%s", body)
	}
	link := regexp.MustCompile(`<a href="(/machines\?[^"]*)"><g class="[^"]*"><rect[^>]*/><text[^>]*>talking<`).FindStringSubmatch(body)
	if link == nil {
		t.Fatalf("a job that names a machine has no link:\n%s", body)
	}
	address := strings.ReplaceAll(strings.ReplaceAll(link[1], "&amp;", "&"), "&#43;", "+")
	unfolded := do(h, "GET", address, nil).Body.String()
	if !strings.Contains(unfolded, ">answering<") {
		t.Fatalf("%s does not unfold the machine:\n%s", address, unfolded)
	}
	second := regexp.MustCompile(`<a href="(/machines\?[^"]*)"><g class="[^"]*"><rect[^>]*/><text[^>]*>answering<`).FindStringSubmatch(unfolded)
	if second == nil {
		t.Fatalf("the unfolded machine has no links:\n%s", unfolded)
	}
	deeper := do(h, "GET", strings.ReplaceAll(second[1], "&amp;", "&"), nil).Body.String()
	if !strings.Contains(deeper, ">flow: turn<") || !strings.Contains(deeper, ">answer<") {
		t.Errorf("a machine written in place does not unfold:\n%s", deeper)
	}
	folded := regexp.MustCompile(`<a href="(/machines\?[^"]*)"><g class="[^"]*unfolded[^"]*"><rect[^>]*/><text[^>]*>talking<`).FindStringSubmatch(deeper)
	if folded == nil {
		t.Fatalf("the unfolded box has no link:\n%s", deeper)
	}
	if u, _ := url.Parse(strings.ReplaceAll(folded[1], "&amp;", "&")); len(u.Query()["open"]) != 0 {
		t.Errorf("folding a box leaves what is inside it unfolded: %s", u)
	}

	agent := do(h, "GET", "/machines?file=support.yaml", nil).Body.String()
	for _, want := range []string{">tell<", "toolkits/toolkit.yaml", ">approve: true<", ">ask<"} {
		if !strings.Contains(agent, want) {
			t.Errorf("the agent's drawing does not hold %s", want)
		}
	}
	if !strings.Contains(agent, `href="/machines?file=support.yaml&amp;view=text"`) {
		t.Errorf("the drawing does not offer the text")
	}
	text := do(h, "GET", "/machines?file=support.yaml&view=text", nil).Body.String()
	if !strings.Contains(text, `<span class="k">agent</span>: support`) || !strings.Contains(text, `href="/machines?file=support.yaml">Drawing</a>`) {
		t.Errorf("the text is not shown:\n%s", text)
	}
	if text := do(h, "GET", "/machines?file=people/scopes.yaml", nil).Body.String(); !strings.Contains(text, `<span class="k">people</span>: []`) || strings.Contains(text, ">Drawing<") {
		t.Errorf("a file that is not a machine does not show its text alone:\n%s", text)
	}
}

func Test_machines_drawing_labels_a_toolkit_of_an_agent_written_in_place(t *testing.T) {
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "here.yaml"), []byte("flow: here\nstages:\n  look:\n    job:\n      machine:\n        agent: in\n        model: { name: claude-sonnet-5-5, secret: KEY }\n        prompt: hello\n        toolkits:\n          messages: { include: ./toolkits/toolkit.yaml }\n"), 0o644)
	h, err := New(Config{Dir: dir, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	body := do(h, "GET", "/machines?file=here.yaml&open=look", nil).Body.String()
	_, drawn, ok := strings.Cut(body, `id="drawing"`)
	if !ok {
		t.Fatalf("the page has no drawing:\n%s", body)
	}
	if !strings.Contains(drawn, ">messages</text><title>toolkits/toolkit.yaml</title>") {
		t.Errorf("the toolkit's file is not in its tooltip:\n%s", drawn)
	}
}

func Test_machines_path_outside_the_directory_is_refused(t *testing.T) {
	h, dir := machines(t, Config{})
	outside := filepath.Join(t.TempDir(), "elsewhere.yaml")
	os.WriteFile(outside, []byte("flow: kept-elsewhere\nstages: {}\n"), 0o644)
	os.Symlink(outside, filepath.Join(dir, "link.yaml"))
	os.WriteFile(filepath.Join(dir, "out.yaml"), []byte("flow: out\nstages:\n  a: { job: ../../../outside.js }\n"), 0o644)
	for _, path := range []string{"../elsewhere.yaml", outside, "/etc/passwd", "link.yaml", "sub/../../x.yaml"} {
		if w := do(h, "GET", "/machines?view=text&file="+url.QueryEscape(path), nil); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "leaves the directory") {
			t.Errorf("reading %s answered %d %s", path, w.Code, w.Body.String())
		}
	}
	body := do(h, "GET", "/machines?file=out.yaml", nil).Body.String()
	if strings.Contains(body, "kept-elsewhere") {
		t.Errorf("a file a link outside the directory names is shown:\n%s", body)
	}
	if !strings.Contains(body, "has no ../") {
		t.Errorf("a script outside the directory of machines was read:\n%s", body)
	}
}

func Test_machines_secrets_and_values_used_are_listed(t *testing.T) {
	section := regexp.MustCompile(`(?s)<section class="uses">.*?</section>`)
	ctl, s := lab(t)
	h, dir := machines(t, Config{Controller: ctl, Store: s})
	prefix := "/" + filepath.Base(dir)
	write(t, dir, map[string]string{
		"shipper.yaml": shipper,
		"later.yaml":   later,
		"shared.yaml":  "allow: [net:z, secret:C@net:z]\n",
		"inc.yaml":     "flow: inc\nallow: [{ include: ./shared.yaml#allow }]\nstages:\n  a: { job: { script: ./write.js, allow: [secret:C@net:z] } }\n",
	})
	ctx := context.Background()
	if err := secret.SetValue(ctx, s, ctl, prefix, "HUB", "hub.lan"); err != nil {
		t.Fatal(err)
	}
	if err := secret.Set(ctx, s, ctl, "/elsewhere", "B", "kept"); err != nil {
		t.Fatal(err)
	}
	got := section.FindString(do(h, "GET", "/machines?view=text&file=shipper.yaml", nil).Body.String())
	for _, want := range []string{
		`<span>A</span><span class="dim">secret · set at /</span>`,
		`<span>B</span><span class="dim">secret · not set at ` + prefix + `/shipper or above</span>`,
		`<span>HUB</span><span class="dim">value · set at ` + prefix + `</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("as the controller, the secrets and values do not show %s:\n%s", want, got)
		}
	}
	if strings.Count(got, "<span>") != 3 || strings.Contains(got, "corbie controller") {
		t.Errorf("as the controller, the secrets and values are shown as\n%s", got)
	}
	if strings.Contains(got, "<form") || strings.Contains(got, "<button") || strings.Contains(got, "<input") || strings.Contains(got, "hub.lan") {
		t.Errorf("the secrets can be set, or a value read, from the Machines page:\n%s", got)
	}
	if got := section.FindString(do(h, "GET", "/machines?file=shipper.yaml", nil).Body.String()); !strings.Contains(got, ">A<") {
		t.Errorf("the drawing does not list the secrets:\n%s", got)
	}
	if got := section.FindString(do(h, "GET", "/machines?file=inc.yaml", nil).Body.String()); !strings.Contains(got, `<span>C</span>`) {
		t.Errorf("a secret an included file names is not listed:\n%s", got)
	}

	h, _ = machines(t, Config{})
	got = section.FindString(do(h, "GET", "/machines?view=text&file=review.yaml", nil).Body.String())
	if got != "" {
		t.Errorf("a machine that uses no secret or value shows %s", got)
	}
	h, dir = machines(t, Config{})
	os.WriteFile(filepath.Join(dir, "shipper.yaml"), []byte(shipper), 0o644)
	got = section.FindString(do(h, "GET", "/machines?file=shipper.yaml", nil).Body.String())
	if !strings.Contains(got, `<span>A</span><span class="dim">secret</span>`) || !strings.Contains(got, `<span>HUB</span><span class="dim">value</span>`) || !strings.Contains(got, "corbie controller secret set NAME --at /"+filepath.Base(dir)+"/shipper") {
		t.Errorf("as a client alone, the secrets and values are shown as\n%s", got)
	}
}

// The directory a client submitted at a prefix of its own is read at that
// prefix, where the UI acts as the controller, so its locations and where
// its secrets and values are set are those of its runs.
func Test_machines_directory_is_read_at_its_recorded_prefix(t *testing.T) {
	config, ctl, s := labIn(t)
	h, dir := machines(t, Config{Controller: ctl, Store: s})
	write(t, dir, map[string]string{"shipper.yaml": shipper})
	ctx := context.Background()
	if err := secret.SetValue(ctx, s, ctl, "/home", "HUB", "hub.lan"); err != nil {
		t.Fatal(err)
	}
	before := do(h, "GET", "/machines?file=shipper.yaml", nil).Body.String()
	if !strings.Contains(before, "at /"+filepath.Base(dir)) || !strings.Contains(before, "value · not set at /"+filepath.Base(dir)+"/shipper or above") {
		t.Errorf("a directory with no tree is not read at the prefix a submit gives it by default:\n%s", before)
	}
	tree, err := client.ReadTree(dir, "/home")
	if err != nil {
		t.Fatal(err)
	}
	me, err := identity.Config{Dir: config}.Load(t.Context(), identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(me, ctl.Card, s)
	message, err := c.Submit(ctx, tree, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := corbiectl.New(ctl, s).Pass(ctx); err != nil {
		t.Fatal(err)
	}
	if receipt, err := c.WaitReceipt(ctx, message, time.Minute, 10*time.Millisecond); err != nil || !receipt.Taken {
		t.Fatalf("the submit answered %+v, %v", receipt, err)
	}
	after := do(h, "GET", "/machines?file=shipper.yaml", nil).Body.String()
	for _, want := range []string{"at /home", "/home/shipper", "value · set at /home"} {
		if !strings.Contains(after, want) {
			t.Errorf("the directory submitted at /home does not show %s:\n%s", want, after)
		}
	}
	h, _ = New(Config{Dir: dir, Interval: time.Second, Shell: page.Shell{}})
	if alone := do(h, "GET", "/machines?file=shipper.yaml", nil).Body.String(); !strings.Contains(alone, "at /"+filepath.Base(dir)) {
		t.Errorf("a client alone does not read the directory at the prefix a submit gives it by default:\n%s", alone)
	}
}

func Test_machines_a_machine_shows_its_location(t *testing.T) {
	h, dir := machines(t, Config{})
	prefix := "/" + filepath.Base(dir)
	write(t, dir, map[string]string{
		"devices/boiler.yaml":   "statechart: boiler\ndescription: Turns the boiler on and off.\nlifetime: singleton\nreceive: { set: [schedules/*, client:panel], stop: [client:panel] }\ninitial: idle\nstates:\n  idle: { on: { set: idle } }\n",
		"rooms/thermostat.yaml": "statechart: thermostat\nlifetime: keyed\nlimits: { runs: 3 }\nupdate: restart\nallow: [send:devices/boiler, send:/work/log]\napprove: [send:devices/boiler, net:hub.lan]\ninitial: on\nstates:\n  on: { on: { set: on } }\n",
		"services/weather.yaml": "flow: weather\nreceive: [rooms/*]\nstages:\n  a: { job: write.js }\n",
		"services/hub.yaml":     "flow: hub\nallow: [send:**, send:/**]\nstages:\n  a: { job: write.js }\n",
		"services/root.yaml":    "flow: root\nallow: [all]\nstages:\n  a: { job: write.js }\n",
		"services/closed.yaml":  "flow: closed\nreceive: []\nstages:\n  a: { job: write.js }\n",
		"services/find.yaml":    "flow: find\ndescription: Find a person.\nstages:\n  a: { job: write.js }\n",
		"services/look.yaml":    "flow: look\ndescription: Look a room up.\nstages:\n  a: { job: write.js }\n",
		"services/asker.yaml": "agent: asker\n" + agentHead + "allow: [send:devices/boiler]\ntools:\n  who: { call: services/find }\n  again: { call: services/find }\n" +
			"toolkits:\n  rooms: { description: the rooms, tools: { room: { call: services/look } } }\n",
		"services/both.yaml": "agent: both\n" + agentHead + "allow: [send:services/find, send:devices/boiler]\ntools:\n  who: { call: services/find }\n  again: { call: services/find }\n",
	})
	place := regexp.MustCompile(`(?s)<section class="place">.*?</section>`)
	rows := func(file string) string {
		got := place.FindString(do(h, "GET", "/machines?file="+file, nil).Body.String())
		return regexp.MustCompile(`\s*\n\s*`).ReplaceAllString(got, "")
	}
	for file, want := range map[string][]string{
		"devices/boiler.yaml": {
			`<section class="place"><p class="description">Turns the boiler on and off.</p><div class="named">`,
			"<span>location</span><span>" + prefix + "/devices/boiler</span>",
			"<span>lifetime</span><span>singleton: the submit starts its one run</span>",
			"<span>update</span><span>restart: a changed file cancels its runs</span>",
			"<span>receives</span><span>set from schedules/*, client:panel<br>stop from client:panel<br>and anything from its tree's client</span>",
			"<span>sends to</span><span>none</span>",
		},
		"rooms/thermostat.yaml": {
			"<span>lifetime</span><span>keyed: a send to a key starts the run for that key, at most 3 running at once</span>",
			"<span>receives</span><span>from its own tree and its tree's client</span>",
			"<span>sends to</span><span>devices/boiler, /work/log</span>",
			"<span>approve</span><span>send:devices/boiler, net:hub.lan</span>",
		},
		"services/weather.yaml": {
			"<span>lifetime</span><span>transient: a call, a send, or corbie run start " + prefix + "/services/weather, starts each run</span>",
			"<span>update</span><span>drain: a changed file leaves its runs to finish on the definition they started with</span>",
			"<span>receives</span><span>every event from rooms/*<br>and anything from its tree's client</span>",
			"<span>sends to</span><span>none</span>",
		},
		"services/hub.yaml":    {"<span>sends to</span><span>** (every location in its tree), /** (every location)</span>"},
		"services/root.yaml":   {"<span>sends to</span><span>all (every location)</span>"},
		"services/closed.yaml": {"<span>receives</span><span>every event from none<br>and anything from its tree's client</span>"},
		// A tool written call: may send to the location it calls.
		"services/asker.yaml": {"<span>sends to</span><span>devices/boiler, services/find (by the tools again, who), services/look (by the tool room)</span>"},
		// A location allow: sends to that a tool also calls is listed once.
		"services/both.yaml": {"<span>sends to</span><span>services/find (by the tools again, who), devices/boiler</span>"},
	} {
		got := rows(file)
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s does not show %s:\n%s", file, w, got)
			}
		}
	}
	if got := rows("services/weather.yaml"); strings.Contains(got, "description") || strings.Contains(got, "approve") {
		t.Errorf("a machine with no description: and no approve: shows them:\n%s", got)
	}
	if body := do(h, "GET", "/machines?file=services/weather.yaml", nil).Body.String(); !strings.Contains(body, `<small title="Turns the boiler on and off.">statechart: boiler · singleton</small>`) {
		t.Errorf("the list does not give a machine's description:\n%s", body)
	}
	if got := place.FindString(do(h, "GET", "/machines?file=notes.yaml", nil).Body.String()); got != "" {
		t.Errorf("a listed file has a location:\n%s", got)
	}
	// Where the UI offers Runs, a location links to the runs there.
	h, err := New(Config{Dir: dir, Runs: true, Interval: time.Second, Shell: page.Shell{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rows("devices/boiler.yaml"); !strings.Contains(got, `<span>location</span><span><a href="/runs?at=%2F`+strings.TrimPrefix(prefix, "/")+`%2Fdevices%2Fboiler" title="the runs at `+prefix+`/devices/boiler">`+prefix+`/devices/boiler</a></span>`) {
		t.Errorf("a location does not link to its runs:\n%s", got)
	}
}

func Test_machines_written_out_shows_where_each_value_is_from(t *testing.T) {
	h, dir := machines(t, Config{})
	write(t, dir, map[string]string{
		"family.yaml": "agent: family\ninclude: ./base.yaml\nmodel: { effort: high }\nallow:\n  - { include: ./base.yaml#allow }\n  - send:devices/*\n",
		"base.yaml":   "model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }\nprompt: ${input.said}\nallow: [\"fs.read:${input.dir}/**\"]\n",
	})
	body := do(h, "GET", "/machines?file=family.yaml&view=written", nil).Body.String()
	if !strings.Contains(body, `href="/machines?file=family.yaml&amp;view=written" aria-current="page">Written out<`) {
		t.Errorf("the Written out tab is not chosen:\n%s", body)
	}
	_, code, _ := strings.Cut(body, `<pre class="code written"`)
	for _, want := range []string{
		`<span class="k">name</span>: claude-sonnet-5-5`,
		`<a class="c" href="/machines?file=base.yaml&amp;view=text#L1"># base.yaml:1</a>`,
		`<span class="k">effort</span>: high`,
		`<a class="c" href="/machines?file=family.yaml&amp;view=text#L3"># family.yaml:3</a>`,
		`<a class="c" href="/machines?file=base.yaml&amp;view=text#L3"># base.yaml:3</a>`,
		`<a class="c" href="/machines?file=family.yaml&amp;view=text#L6"># family.yaml:6</a>`,
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the definition written out does not hold %s:\n%s", want, code)
		}
	}
	if strings.Contains(code, "include") {
		t.Errorf("the definition written out holds an include:\n%s", code)
	}
	if text := do(h, "GET", "/machines?file=base.yaml&view=text", nil).Body.String(); !strings.Contains(text, `<span class="l" id="L3">`) {
		t.Errorf("a line of a file has no anchor to link to:\n%s", text)
	}
	if body := do(h, "GET", "/machines?file=notes.yaml&view=written", nil).Body.String(); strings.Contains(body, "Written out") || !strings.Contains(body, `<span class="k">a</span>`) {
		t.Errorf("a listed file is written out:\n%s", body)
	}
}

func Test_machines_directory_is_read_once_per_change(t *testing.T) {
	dir := project(t)
	d, _ := newDir(dir)
	c := &indexed{dir: d}
	first := c.current("")
	if c.current("") != first {
		t.Errorf("the directory was read again with nothing changed")
	}
	write(t, dir, map[string]string{"new.yaml": "flow: new\nstages:\n  a: { job: .kit/hear.js }\n"})
	next := c.current("")
	if next == first || !slices.Contains(listed(next), ". new.yaml flow: new transient 0") {
		t.Errorf("a file written is not read: %v", listed(next))
	}
	write(t, dir, map[string]string{"toolkits/toolkit.yaml": "description: changed\ntools: {}\n"})
	if c.current("") == next {
		t.Errorf("a change to a file that is not a machine is not read")
	}
}

func Test_machines_dotted_files_are_read_as_any_other(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{
		".kit/alone.yaml":   "agent: alone\n" + agentHead + "toolkits:\n  hidden: { description: hidden, tools: { r: { description: r, job: { script: ./hear.js } } } }\n",
		".kit/broken.yaml":  "flow: broken\nstages: {}\ncolour: blue\n",
		".kit/reached.yaml": "agent: reached\n" + agentHead + "toolkits:\n  seen: { description: seen, tools: { r: { description: r, job: { script: ./hear.js } } } }\n",
		"uses.yaml":         "flow: uses\nstages:\n  a: { job: { machine: .kit/reached.yaml } }\n",
	})
	d, _ := newDir(dir)
	ix := d.index("")
	prefix := "/" + filepath.Base(dir)
	for _, want := range []string{".kit .kit/alone.yaml agent: alone transient 0", ".kit .kit/broken.yaml flow: broken 2"} {
		if !slices.Contains(listed(ix), want) {
			t.Errorf("the list does not hold %s: %v", want, listed(ix))
		}
	}
	if m := ix.machines[".kit/alone.yaml"]; m == nil || m.Location != prefix+"/.kit/alone" {
		t.Errorf("a machine in a dotted directory is %+v", m)
	}
	if !slices.ContainsFunc(ix.Problems, func(p problem) bool { return p.File == ".kit/broken.yaml" }) {
		t.Errorf("a machine in a dotted directory gives no problems: %+v", ix.Problems)
	}
	v := ix.toolkitsView("")
	hidden := byDescription(t, v, "hidden")
	if len(hidden.Offered) != 1 || hidden.Offered[0].File != ".kit/alone.yaml" || len(hidden.Offered[0].Starts) != 0 || hidden.Offered[0].Lifetime != "transient" {
		t.Errorf("an agent in a dotted directory that nothing names is %+v", hidden.Offered)
	}
	seen := byDescription(t, v, "seen")
	if len(seen.Offered) != 1 || seen.Offered[0].File != ".kit/reached.yaml" || len(seen.Offered[0].Starts) != 1 || seen.Offered[0].Starts[0].File != "uses.yaml" {
		t.Errorf("an agent in a dotted directory that a machine names is %+v", seen.Offered)
	}
}

func Test_machines_pages_only_read(t *testing.T) {
	ctl, s := lab(t)
	h, dir := machines(t, Config{Controller: ctl, Store: s})
	d, _ := newDir(dir)
	before := d.stamp()
	for _, path := range []string{"/machines", "/machines/save", "/machines/new", "/machines/start", "/machines/check", "/machines/secret-set", "/machines/raw", "/toolkits"} {
		form := url.Values{"file": {"review.yaml"}, "text": {"flow: x\n"}, "type": {"flow"}, "name": {"x"}, "value": {"v"}}
		if w := do(h, "POST", path, form); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s answered %d", path, w.Code)
		}
	}
	for _, path := range []string{"/machines/test?file=review.yaml", "/machines/raw?file=review.yaml"} {
		if w := do(h, "GET", path, nil); w.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d", path, w.Code)
		}
	}
	for _, path := range []string{"/machines?file=review.yaml", "/machines?file=review.yaml&view=text", "/toolkits"} {
		body := do(h, "GET", path, nil).Body.String()
		for _, control := range []string{"<form", "<textarea", "<input", "<button"} {
			if strings.Contains(body, control) {
				t.Errorf("%s shows %s:\n%s", path, control, body)
			}
		}
	}
	if d.stamp() != before {
		t.Errorf("the directory changed")
	}
}

func Test_machines_a_runs_machine_is_found_by_its_location(t *testing.T) {
	// A run's machine is the file at its location, where its tree was
	// submitted from the directory the page reads and at the prefix it
	// reads it at; the cards down to a child are unfolded one inside
	// another.
	_, dir := machines(t, Config{})
	p, err := New(Config{Dir: dir, Interval: time.Second, Shell: page.Shell{}})
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	prefix, ctx := "/"+filepath.Base(dir), context.Background()
	for _, c := range []struct {
		location, dir string
		cards         []string
		href, file    string
	}{
		{prefix + "/review", real, nil, "/machines?file=review.yaml", "review.yaml"},
		{prefix + "/review", real, []string{"draft", "check"}, "/machines?file=review.yaml&open=draft&open=draft%2Fcheck", "review.yaml"},
		{prefix + "/review", "/elsewhere", nil, "", ""},
		{"/work/review", real, nil, "", ""},
		{prefix + "/notes", real, nil, "", ""},
	} {
		href, file, err := p.Href(ctx, c.location, c.dir, c.cards)
		if err != nil || href != c.href || file != c.file {
			t.Errorf("%s from %s, %v, is at %q %q, %v", c.location, c.dir, c.cards, href, file, err)
		}
	}
	if got := do(p, "GET", "/machines?file=review.yaml&open=draft", nil).Code; got != 200 {
		t.Errorf("the address answered %d", got)
	}
}
