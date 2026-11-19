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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/drawing"
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
	p := pageOf(t, h, "/machines?file=grouse/b.yaml")
	var dirs []string
	for _, g := range p.Files.Groups {
		dirs = append(dirs, g.Dir)
	}
	if !slices.Equal(dirs, []string{".", ".kit", "grouse"}) {
		t.Errorf("the list is by the directories %v", dirs)
	}
	if e := p.entryOf("support.yaml"); e == nil || e.Type != "agent" || e.First != "agent: support" || e.Lifetime != "transient" {
		t.Errorf("the list gives support.yaml as %+v", e)
	}
	if e := p.entryOf("grouse/b.yaml"); e == nil || e.First != "flow: b" || e.Lifetime != "singleton" || e.Location != ix.Prefix+"/grouse/b" {
		t.Errorf("the list gives grouse/b.yaml as %+v", e)
	}
	if e := p.entryOf("bad.yaml"); e == nil || e.Problems != 2 {
		t.Errorf("the list gives bad.yaml as %+v", e)
	}
	if p.View.Open != "grouse/b.yaml" || !p.Files.Ignore {
		t.Errorf("the page has %q chosen, and .corbieignore %t", p.View.Open, p.Files.Ignore)
	}
	for _, gone := range []string{"people/scopes.yaml", "toolkits/toolkit.yaml"} {
		if p.entryOf(gone) != nil {
			t.Errorf("the machines hold %s", gone)
		}
	}
	if !slices.Equal(p.Files.Files, ix.Files) {
		t.Errorf("the page lists the files %+v, not %+v", p.Files.Files, ix.Files)
	}
	if p.Files.Prefix != ix.Prefix {
		t.Errorf("the page gives the prefix %q", p.Files.Prefix)
	}
	if shared := pageOf(t, h, "/machines?file=shared/states.yaml").View; shared.File == nil || !slices.Equal(shared.Included, []string{"keyed.yaml"}) || shared.Facts != "listed in .corbieignore" || shared.Drawable {
		t.Errorf("a listed file does not show the machines that include it: %+v", shared)
	}
	if prompt := pageOf(t, h, "/machines?file=prompt.md").View; prompt.File == nil || !slices.Equal(prompt.Included, []string{"talker.yaml"}) || prompt.Facts != "a file" {
		t.Errorf("a file a machine includes does not show the machines that include it: %+v", prompt)
	}
	if script := pageOf(t, h, "/machines?file=.kit/hear.js").View; script.Facts != "a file" || script.File != nil {
		t.Errorf("a script no machine takes a value from is shown as one of the files: %+v", script)
	}
	if p := pageOf(t, h, "/machines?file=./grouse/b.yaml"); p.View.Open != "grouse/b.yaml" {
		t.Errorf("a path written with ./ is not the file chosen: %q", p.View.Open)
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
	if v := pageOf(t, h, "/machines?file=.rook/AGENTS.md").View; v.Facts != "listed in .corbieignore" || v.File == nil || len(v.Included) != 0 {
		t.Errorf("a file in a listed directory is not headed as listed: %+v", v)
	}
	if p := pageOf(t, h, "/machines?file=.corbieignore"); !p.Files.Ignore || p.View.Open != ".corbieignore" || !slices.ContainsFunc(text(p.View.Text), func(l string) bool { return strings.Contains(l, ".rook/") }) {
		t.Errorf("the Files heading does not open .corbieignore: %+v", p.View)
	}
	for file, head := range map[string]string{"broken.yaml": "a machine", "nul.yaml": "flow"} {
		if v := pageOf(t, h, "/machines?file="+file).View; v.Facts != head {
			t.Errorf("%s is headed %q, not %q", file, v.Facts, head)
		}
	}
	p := pageOf(t, h, "/machines?file=loose.yaml")
	if e := p.entryOf("loose.yaml"); p.View.Facts != "a machine" || e == nil || e.Type != "" || e.Problems != 1 || e.First != "" || e.Location != "/"+filepath.Base(dir)+"/loose" {
		t.Errorf("a machine with no type is not shown as a machine with a problem: %+v, %+v", p.View, e)
	}
	if v := pageOf(t, h, "/machines?file=shared/flow.yaml").View; v.Facts != "listed in .corbieignore" || !slices.Equal(v.Included, []string{"placed.yaml"}) || v.Drawable {
		t.Errorf("a listed file does not show the machines that include it: %+v", v)
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
	if p := pageOf(t, h, "/machines"); !strings.Contains(p.Files.Unread, "a-locked") {
		t.Errorf("a directory that cannot be read is not reported: %q", p.Files.Unread)
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
			if !slices.Equal(p.Breaks, []string{".kit/c.yaml", "a.yaml", "b.yaml"}) || !strings.HasPrefix(p.Place, "toolkits.messages.tools.tell") || p.Line == 0 {
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
	z := pageOf(t, h, "/machines?file=z.yaml")
	if n := strings.Count(problemsSaid(z.Files.Problems), "breaks .kit/c.yaml,a.yaml,b.yaml"); n != atToolkit {
		t.Errorf("the page shows the toolkit's problems %d times, and not %d:\n%s", n, atToolkit, problemsSaid(z.Files.Problems))
	}
	// The Toolkits page lists the same problems, drawn the same way.
	toolkits := pageOf(t, h, "/toolkits").Toolkits.Problems
	if len(toolkits) == 0 || problemsSaid(toolkits) != problemsSaid(z.Files.Problems) {
		t.Errorf("the Toolkits page lists the problems\n%s\nand not as the Machines page does", problemsSaid(toolkits))
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
	if p := pageOf(t, h, "/machines"); len(p.Files.Problems) == 0 || !p.Files.Problems[0].Top || p.Files.Problems[0].File != "proj" || len(p.Files.Problems[0].Breaks) != 3 {
		t.Errorf("the page does not say which directory is the top:\n%s", problemsSaid(p.Files.Problems))
	}
	if p := pageOf(t, h, "/machines?file=proj/a.yaml"); !slices.ContainsFunc(p.View.Found, func(f problem) bool { return f.Top }) || strings.Contains(problemsSaid(p.View.Found), "cannot be read") {
		t.Errorf("a machine read from the wrong directory does not say so:\n%s", problemsSaid(p.View.Found))
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
	broken := pageOf(t, h, "/machines?file=broken.yaml").View
	lines := text(broken.Text)
	if broken.View != "text" || broken.Drawing != nil || !strings.Contains(problemsSaid(broken.Found), "missing.js") || len(lines) < 4 || lines[3] != "![k:  a]: { job: ./missing.js }" && lines[3] != "!  [k:a]: { job: ./missing.js }" {
		t.Errorf("a machine with problems is not shown as text with its line marked: %q\n%s", lines, problemsSaid(broken.Found))
	}
	if broken.Text[3].ID != "L4" || len(lines) == 0 || lines[0] != "[c:# one stage]" {
		t.Errorf("a comment is not set apart, or a line has no anchor: %q", lines)
	}
	toolkit := pageOf(t, h, "/machines?file=toolkits/m.yaml&view=text").View
	marked := slices.DeleteFunc(text(toolkit.Text), func(l string) bool { return !strings.HasPrefix(l, "!") })
	if !strings.Contains(problemsSaid(toolkit.Found), "tools.r.colour") || !strings.Contains(problemsSaid(toolkit.Found), "breaks a.yaml") || len(marked) != 1 {
		t.Errorf("a toolkit's text does not show the problem found in it: %q\n%s", marked, problemsSaid(toolkit.Found))
	}
	a := pageOf(t, h, "/machines?file=a.yaml").View
	if !strings.Contains(problemsSaid(a.Found), "tools.r.colour") || !strings.Contains(problemsSaid(a.Found), "breaks a.yaml") || slices.ContainsFunc(a.Text, func(l codeLine) bool { return l.Bad }) {
		t.Errorf("a machine marks its lines for a problem in a file it includes:\n%s", problemsSaid(a.Found))
	}
	for _, file := range []string{"people/scopes.yaml", "support.yaml"} {
		if v := pageOf(t, h, "/machines?view=text&file="+file).View; v.View != "text" || len(v.Found) != 0 || len(v.Text) == 0 || slices.ContainsFunc(v.Text, func(l codeLine) bool { return l.Bad }) {
			t.Errorf("%s is not shown as read-only text with no problems: %+v", file, v)
		}
	}
}

func Test_machines_drawing_unfolds_as_the_address_says(t *testing.T) {
	h, err := New(Config{Dir: project(t), Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	listen := pageOf(t, h, "/machines?file=.kit/listen.yaml").View
	if listen.View != "drawing" || card(listen.Drawing, "hearing") == nil || card(listen.Drawing, ".kit/conversation#a") == nil {
		t.Fatalf("the machine is not drawn: %+v", listen.Drawing)
	}
	if c := card(listen.Drawing, ".kit/conversation#a"); c.Href != "" {
		t.Errorf("a send, which starts no child, unfolds: %+v", c)
	}
	talking := card(listen.Drawing, "talking")
	if talking == nil || !strings.HasPrefix(talking.Href, "/machines?") {
		t.Fatalf("a job that names a machine has no link: %+v", talking)
	}
	unfolded := pageOf(t, h, talking.Href).View
	answering := card(unfolded.Drawing, "answering")
	if answering == nil || card(unfolded.Drawing, "talking").In == nil {
		t.Fatalf("%s does not unfold the machine: %+v", talking.Href, unfolded.Drawing)
	}
	if !strings.HasPrefix(answering.Href, "/machines?") {
		t.Fatalf("the unfolded machine has no links: %+v", answering)
	}
	deeper := pageOf(t, h, answering.Href).View
	if c := card(deeper.Drawing, "answer"); c == nil || !slices.Contains(card(deeper.Drawing, "answering").Lines, "flow: turn") {
		t.Errorf("a machine written in place does not unfold: %+v", deeper.Drawing)
	}
	folded := card(deeper.Drawing, "talking")
	if folded == nil || folded.In == nil || folded.Href == "" {
		t.Fatalf("the unfolded box has no link: %+v", folded)
	}
	if u, _ := url.Parse(folded.Href); len(u.Query()["open"]) != 0 {
		t.Errorf("folding a box leaves what is inside it unfolded: %s", u)
	}

	agent := pageOf(t, h, "/machines?file=support.yaml").View
	if card(agent.Drawing, "tell") == nil || card(agent.Drawing, "ask") == nil || !slices.Contains(card(agent.Drawing, "tell").Lines, "approve: true") {
		t.Errorf("the agent's drawing does not hold tell, its approval and the ask: %+v", agent.Drawing)
	}
	if agent.Drawing.Agent == nil || !slices.ContainsFunc(agent.Drawing.Agent.Toolkits, func(k drawing.Toolkit) bool { return k.File == "toolkits/toolkit.yaml" }) {
		t.Errorf("the agent's drawing does not give its toolkit's file: %+v", agent.Drawing.Agent)
	}
	if !agent.Drawable {
		t.Errorf("the drawing does not offer the text")
	}
	shown := pageOf(t, h, "/machines?file=support.yaml&view=text").View
	if shown.View != "text" || !shown.Drawable || !slices.Contains(text(shown.Text), "[k:agent]: support") {
		t.Errorf("the text is not shown: %q", text(shown.Text))
	}
	if scopes := pageOf(t, h, "/machines?file=people/scopes.yaml").View; !slices.Contains(text(scopes.Text), "[k:people]: []") || scopes.Drawable {
		t.Errorf("a file that is not a machine does not show its text alone: %+v", scopes)
	}
}

func Test_machines_drawing_labels_a_toolkit_of_an_agent_written_in_place(t *testing.T) {
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "here.yaml"), []byte("flow: here\nstages:\n  look:\n    job:\n      machine:\n        agent: in\n        model: { name: claude-sonnet-5-5, secret: KEY }\n        prompt: hello\n        toolkits:\n          messages: { include: ./toolkits/toolkit.yaml }\n"), 0o644)
	h, err := New(Config{Dir: dir, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	v := pageOf(t, h, "/machines?file=here.yaml&open=look").View
	look := card(v.Drawing, "look")
	if look == nil || look.In == nil || look.In.Agent == nil {
		t.Fatalf("the page does not draw the agent unfolded: %+v", v.Drawing)
	}
	if !slices.ContainsFunc(look.In.Agent.Toolkits, func(k drawing.Toolkit) bool { return k.Name == "messages" && k.File == "toolkits/toolkit.yaml" }) {
		t.Errorf("the toolkit's file is not given with it: %+v", look.In.Agent.Toolkits)
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
	p := pageOf(t, h, "/machines?file=out.yaml")
	if strings.Contains(p.JSON, "kept-elsewhere") {
		t.Errorf("a file a link outside the directory names is shown:\n%s", p.JSON)
	}
	if !strings.Contains(problemsSaid(p.View.Found), "has no ../") {
		t.Errorf("a script outside the directory of machines was read:\n%s", problemsSaid(p.View.Found))
	}
}

func Test_machines_secrets_and_values_used_are_listed(t *testing.T) {
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
	shipping := pageOf(t, h, "/machines?view=text&file=shipper.yaml")
	want := []use{{"A", "secret", "set at /"}, {"B", "secret", "not set at " + prefix + "/shipper or above"}, {"HUB", "value", "set at " + prefix}}
	if !slices.Equal(shipping.View.Uses, want) || !shipping.View.Controller {
		t.Errorf("as the controller, the secrets and values are shown as %+v, not %+v", shipping.View.Uses, want)
	}
	if strings.Contains(shipping.JSON, "hub.lan") {
		t.Errorf("a value's text is read on the Machines page:\n%s", shipping.JSON)
	}
	if got := pageOf(t, h, "/machines?file=shipper.yaml").View.Uses; len(got) == 0 || got[0].Name != "A" {
		t.Errorf("the drawing does not list the secrets: %+v", got)
	}
	if got := pageOf(t, h, "/machines?file=inc.yaml").View.Uses; !slices.ContainsFunc(got, func(u use) bool { return u.Name == "C" }) {
		t.Errorf("a secret an included file names is not listed: %+v", got)
	}

	h, _ = machines(t, Config{})
	if got := pageOf(t, h, "/machines?view=text&file=review.yaml").View.Uses; len(got) != 0 {
		t.Errorf("a machine that uses no secret or value shows %+v", got)
	}
	h, dir = machines(t, Config{})
	os.WriteFile(filepath.Join(dir, "shipper.yaml"), []byte(shipper), 0o644)
	alone := pageOf(t, h, "/machines?file=shipper.yaml").View
	if want := []use{{"A", "secret", ""}, {"B", "secret", ""}, {"HUB", "value", ""}}; !slices.Equal(alone.Uses, want) || alone.Controller || alone.Place.Location != "/"+filepath.Base(dir)+"/shipper" {
		t.Errorf("as a client alone, the secrets and values are shown as %+v, at %+v", alone.Uses, alone.Place)
	}
}

func Test_machines_directory_is_read_at_its_recorded_prefix(t *testing.T) {
	config, ctl, s := labIn(t)
	h, dir := machines(t, Config{Controller: ctl, Store: s})
	write(t, dir, map[string]string{"shipper.yaml": shipper})
	ctx := context.Background()
	if err := secret.SetValue(ctx, s, ctl, "/home", "HUB", "hub.lan"); err != nil {
		t.Fatal(err)
	}
	before := pageOf(t, h, "/machines?file=shipper.yaml")
	if before.Files.Prefix != "/"+filepath.Base(dir) || !slices.Contains(before.View.Uses, use{"HUB", "value", "not set at /" + filepath.Base(dir) + "/shipper or above"}) {
		t.Errorf("a directory with no tree is not read at the prefix a submit gives it by default: %q, %+v", before.Files.Prefix, before.View.Uses)
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
	after := pageOf(t, h, "/machines?file=shipper.yaml")
	if after.Files.Prefix != "/home" || after.View.Place.Location != "/home/shipper" || !slices.Contains(after.View.Uses, use{"HUB", "value", "set at /home"}) {
		t.Errorf("the directory submitted at /home is shown at %q, %+v, %+v", after.Files.Prefix, after.View.Place, after.View.Uses)
	}
	h, _ = New(Config{Dir: dir, Interval: time.Second, Shell: page.Shell{}})
	if alone := pageOf(t, h, "/machines?file=shipper.yaml"); alone.Files.Prefix != "/"+filepath.Base(dir) {
		t.Errorf("a client alone does not read the directory at the prefix a submit gives it by default: %q", alone.Files.Prefix)
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
	placeOf := func(file string) place {
		t.Helper()
		v := pageOf(t, h, "/machines?file="+file).View
		if v.Place == nil {
			t.Fatalf("%s has no location", file)
		}
		return *v.Place
	}
	transient := "a call, a send, or corbie run start " + prefix + "/services/weather, starts each run"
	drain := "a changed file leaves its runs to finish on the definition they started with"
	for file, want := range map[string]place{
		"devices/boiler.yaml": {Description: "Turns the boiler on and off.", Location: prefix + "/devices/boiler", Lifetime: "singleton", Started: "the submit starts its one run",
			Update: "restart", Updated: "a changed file cancels its runs", Receives: []string{"set from schedules/*, client:panel", "stop from client:panel"}},
		"rooms/thermostat.yaml": {Location: prefix + "/rooms/thermostat", Lifetime: "keyed", Started: "a send to a key starts the run for that key", Runs: 3, Update: "restart", Updated: "a changed file cancels its runs",
			Sends: []string{"devices/boiler", "/work/log"}, Approve: []string{"send:devices/boiler", "net:hub.lan"}},
		"services/weather.yaml": {Location: prefix + "/services/weather", Lifetime: "transient", Started: transient, Update: "drain", Updated: drain, Receives: []string{"every event from rooms/*"}},
		"services/hub.yaml":     {Sends: []string{"** (every location in its tree)", "/** (every location)"}},
		"services/root.yaml":    {Sends: []string{"all (every location)"}},
		"services/closed.yaml":  {Receives: []string{"every event from none"}},
		// A tool written call: may send to the location it calls.
		"services/asker.yaml": {Sends: []string{"devices/boiler", "services/find (by the tools again, who)", "services/look (by the tool room)"}},
		// A location allow: sends to that a tool also calls is listed once.
		"services/both.yaml": {Sends: []string{"services/find (by the tools again, who)", "devices/boiler"}},
	} {
		got := placeOf(file)
		if want.Location == "" {
			// Only what the case is about is compared.
			want.Description, want.Location, want.Lifetime, want.Started, want.Update, want.Updated, want.Runs = got.Description, got.Location, got.Lifetime, got.Started, got.Update, got.Updated, got.Runs
			if want.Receives == nil {
				want.Receives = got.Receives
			}
			if want.Sends == nil {
				want.Sends = got.Sends
			}
			want.Approve = got.Approve
		}
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Errorf("%s shows\n%+v\nand not\n%+v", file, got, want)
		}
	}
	if e := pageOf(t, h, "/machines?file=services/weather.yaml").entryOf("devices/boiler.yaml"); e == nil || e.Description != "Turns the boiler on and off." || e.First != "statechart: boiler" || e.Lifetime != "singleton" {
		t.Errorf("the list does not give a machine's description: %+v", e)
	}
	if got := pageOf(t, h, "/machines?file=notes.yaml").View.Place; got != nil {
		t.Errorf("a listed file has a location: %+v", got)
	}
	// Where the UI offers Runs, a location links to the runs there.
	h, err := New(Config{Dir: dir, Runs: true, Interval: time.Second, Shell: page.Shell{}})
	if err != nil {
		t.Fatal(err)
	}
	if v := pageOf(t, h, "/machines?file=devices/boiler.yaml").View; !v.Runs || v.Place.Location != prefix+"/devices/boiler" {
		t.Errorf("a location does not link to its runs: %+v", v)
	}
}

func Test_machines_written_out_shows_where_each_value_is_from(t *testing.T) {
	h, dir := machines(t, Config{})
	write(t, dir, map[string]string{
		"family.yaml": "agent: family\ninclude: ./base.yaml\nmodel: { effort: high }\nallow:\n  - { include: ./base.yaml#allow }\n  - send:devices/*\n",
		"base.yaml":   "model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }\nprompt: ${input.said}\nallow: [\"fs.read:${input.dir}/**\"]\n",
	})
	v := pageOf(t, h, "/machines?file=family.yaml&view=written").View
	if v.View != "written" {
		t.Errorf("the Written out tab is not chosen: %q", v.View)
	}
	code := strings.Join(text(v.Text), "\n")
	links := map[string]bool{}
	for _, l := range v.Text {
		for _, sp := range l.Spans {
			if sp.Href != "" {
				links[sp.Text+" "+sp.Href] = true
			}
		}
	}
	for _, want := range []string{"[k:name]: claude-sonnet-5-5", "[k:effort]: high"} {
		if !strings.Contains(code, want) {
			t.Errorf("the definition written out does not hold %s:\n%s", want, code)
		}
	}
	for _, want := range []string{"# base.yaml:1 /machines?file=base.yaml&view=text#L1", "# family.yaml:3 /machines?file=family.yaml&view=text#L3", "# base.yaml:3 /machines?file=base.yaml&view=text#L3", "# family.yaml:6 /machines?file=family.yaml&view=text#L6"} {
		if !links[want] {
			t.Errorf("the definition written out does not link %s: %v", want, links)
		}
	}
	if strings.Contains(code, "include") {
		t.Errorf("the definition written out holds an include:\n%s", code)
	}
	if base := pageOf(t, h, "/machines?file=base.yaml&view=text").View; len(base.Text) < 3 || base.Text[2].ID != "L3" {
		t.Errorf("a line of a file has no anchor to link to: %+v", base.Text)
	}
	if notes := pageOf(t, h, "/machines?file=notes.yaml&view=written").View; notes.Drawable || notes.View != "text" || !strings.Contains(strings.Join(text(notes.Text), "\n"), "[k:a]") {
		t.Errorf("a listed file is written out: %+v", notes)
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
	// That the pages show no form is checked in the browser, by
	// machines_test.js and toolkits_test.js.
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
