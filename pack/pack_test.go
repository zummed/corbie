package pack

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write writes files under a directory, by their paths.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

// packs is a directory of two packs: notes, whose files name shared, and
// shared.
func packs(t *testing.T) string {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"notes/chat/post.yaml":                    "flow: post\nstages: { say: { job: { script: .packs/shared/format.js } } }\n",
		"notes/AGENTS.md":                         "Place a call with include: .packs/notes/chat/post.yaml, or .packs/jira/chat/post.yaml\n",
		"notes/starter/TEAM/notes/TEAM.yaml":      "statechart: TEAM\n",
		"notes/starter/TEAM/notes/chat/post.yaml": "include: .packs/notes/chat/post.yaml\n",
		"shared/format.js":                        "return input\n",
		"shared/starter/never.yaml":               "flow: never\n",
	})
	return dir
}

func add(t *testing.T, project, source string, words map[string]string) Report {
	t.Helper()
	src, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Add(project, src, words)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func Test_pack_add_writes_the_pack_and_its_starting_files(t *testing.T) {
	from, project := packs(t), t.TempDir()
	rep := add(t, project, filepath.Join(from, "notes"), map[string]string{"TEAM": "ops"})
	if read(t, filepath.Join(project, ".packs/notes/chat/post.yaml")) == "" || read(t, filepath.Join(project, ".packs/shared/format.js")) != "return input\n" {
		t.Fatal("the pack, or the pack its files name, is not written")
	}
	if _, err := os.Stat(filepath.Join(project, ".packs/notes/starter")); err == nil {
		t.Fatal("the starting files are written under .packs")
	}
	if _, err := os.Stat(filepath.Join(project, "never.yaml")); err == nil {
		t.Fatal("a needed pack's starting file is written")
	}
	if got := read(t, filepath.Join(project, "ops/notes/ops.yaml")); got != "statechart: ops\n" {
		t.Fatalf("the router is %q", got)
	}
	if want := []string{"ops/notes/chat/post.yaml", "ops/notes/ops.yaml"}; !slices.Equal(rep.Started, want) || len(rep.Differ) != 0 {
		t.Fatalf("started %v, differ %v", rep.Started, rep.Differ)
	}
	if got := read(t, filepath.Join(project, ".corbieignore")); got != ".packs/\n" {
		t.Fatalf(".corbieignore is %q", got)
	}
	if len(rep.Packs) != 2 || rep.Packs[1].Needed != "notes" {
		t.Fatalf("the packs written are %+v", rep.Packs)
	}
}

func Test_pack_add_again_rewrites_the_pack_and_no_file_of_the_project(t *testing.T) {
	from, project := packs(t), t.TempDir()
	words := map[string]string{"TEAM": "ops"}
	add(t, project, filepath.Join(from, "notes"), words)
	write(t, project, map[string]string{
		"ops/notes/ops.yaml":          "statechart: mine\n",
		".packs/notes/chat/post.yaml": "changed\n",
		".packs/notes/stale.yaml":     "stale\n",
		".corbieignore":               "people.yaml\n.packs/\n",
	})
	rep := add(t, project, filepath.Join(from, "notes"), words)
	if read(t, filepath.Join(project, "ops/notes/ops.yaml")) != "statechart: mine\n" {
		t.Fatal("a file of the project was changed")
	}
	if !slices.Equal(rep.Differ, []string{"ops/notes/ops.yaml"}) || len(rep.Started) != 0 {
		t.Fatalf("started %v, differ %v", rep.Started, rep.Differ)
	}
	if strings.Contains(read(t, filepath.Join(project, ".packs/notes/chat/post.yaml")), "changed") {
		t.Fatal("the pack was not rewritten")
	}
	if _, err := os.Stat(filepath.Join(project, ".packs/notes/stale.yaml")); err == nil {
		t.Fatal("a file the pack no longer holds is left")
	}
	if got := read(t, filepath.Join(project, ".corbieignore")); got != "people.yaml\n.packs/\n" {
		t.Fatalf(".corbieignore is %q", got)
	}
	// A starting file a person removed is not written again, and one the
	// pack gained is.
	os.Remove(filepath.Join(project, "ops/notes/chat/post.yaml"))
	write(t, from, map[string]string{"notes/starter/TEAM/notes/files/get.yaml": "include: .packs/notes/chat/post.yaml\n"})
	rep = add(t, project, filepath.Join(from, "notes"), words)
	if _, err := os.Stat(filepath.Join(project, "ops/notes/chat/post.yaml")); err == nil || !slices.Equal(rep.Started, []string{"ops/notes/files/get.yaml"}) {
		t.Fatalf("after a starting file was removed and another added, it started %v", rep.Started)
	}
	sources, kept, err := Added(project)
	if err != nil || len(sources) != 1 || sources["notes"].Name != "notes" || kept["notes"]["TEAM"] != "ops" {
		t.Fatalf("the project's packs are %v with %v: %v", sources, kept, err)
	}
}

func Test_pack_add_refuses_a_word_not_set_or_not_held(t *testing.T) {
	from := packs(t)
	src, _ := Parse(filepath.Join(from, "notes"))
	if _, err := Add(t.TempDir(), src, nil); err == nil || !strings.Contains(err.Error(), "takes TEAM: add --set TEAM=<name>") {
		t.Fatalf("with no word set: %v", err)
	}
	if _, err := Add(t.TempDir(), src, map[string]string{"TEAM": "ops", "OTHER": "x"}); err == nil || !strings.Contains(err.Error(), "hold no OTHER") {
		t.Fatalf("with a word the pack does not hold: %v", err)
	}
	for _, name := range []string{"../up", "my bot", "a/b"} {
		project := t.TempDir()
		if _, err := Add(project, src, map[string]string{"TEAM": name}); err == nil || !strings.Contains(err.Error(), "is not a name") {
			t.Fatalf("with the word set to %q: %v", name, err)
		}
		if entries, _ := os.ReadDir(project); len(entries) != 0 {
			t.Fatalf("with the word set to %q, files were written", name)
		}
	}
	src, _ = Parse(filepath.Join(from, "absent"))
	if _, err := Add(t.TempDir(), src, nil); err == nil || !strings.Contains(err.Error(), "holds no pack absent") {
		t.Fatalf("a pack that is not there: %v", err)
	}
	write(t, from, map[string]string{"lone/a.yaml": "include: .packs/gone/a.yaml\n"})
	src, _ = Parse(filepath.Join(from, "lone"))
	if _, err := Add(t.TempDir(), src, nil); err == nil || !strings.Contains(err.Error(), "holds no pack gone beside it") {
		t.Fatalf("a pack that names one not there: %v", err)
	}
}

func Test_pack_parse_reads_a_repository_a_path_and_a_ref(t *testing.T) {
	for text, want := range map[string]Source{
		"github.com/owner/packs/notes":                  {Repo: "https://github.com/owner/packs", Path: "notes", Name: "notes"},
		"github.com/owner/packs/chat/notes@v0.4.0":      {Repo: "https://github.com/owner/packs", Path: "chat/notes", Ref: "v0.4.0", Name: "notes"},
		"github.com/owner/packs/notes@claude/try-packs": {Repo: "https://github.com/owner/packs", Path: "notes", Ref: "claude/try-packs", Name: "notes"},
		"gitlab.com/a/b/c.git/notes":                    {Repo: "https://gitlab.com/a/b/c.git", Path: "notes", Name: "notes"},
		"github.com/owner/notes":                        {Repo: "https://github.com/owner/notes", Path: "", Name: "notes"},
	} {
		got, err := Parse(text)
		want.Text = text
		if err != nil || got != want {
			t.Errorf("%s is read as %+v: %v", text, got, err)
		}
	}
	for _, text := range []string{"notes", "github.com/owner", "github.com/owner/packs/notes@", "github.com/owner/packs/no tes"} {
		if _, err := Parse(text); err == nil {
			t.Errorf("%q is read as a pack", text)
		}
	}
}

// A pack is read from a git repository at a branch, and the commit is
// recorded.
func Test_pack_add_reads_a_branch_of_a_repository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	from := packs(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", from, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	run("add", ".")
	run("commit", "-q", "-m", "packs")
	run("checkout", "-q", "-b", "try/branch")
	write(t, from, map[string]string{"shared/format.js": "return 'branch'\n"})
	run("commit", "-q", "-am", "branch")
	commit := run("rev-parse", "HEAD")
	run("checkout", "-q", "main")

	project := t.TempDir()
	rep := add(t, project, filepath.Join(from, "notes")+"@try/branch", map[string]string{"TEAM": "ops"})
	if got := read(t, filepath.Join(project, ".packs/shared/format.js")); got != "return 'branch'\n" {
		t.Fatalf("the needed pack is read as %q", got)
	}
	if rep.Packs[0].Commit != commit || !strings.Contains(read(t, filepath.Join(project, ".packs/notes/.pack")), "commit "+commit+"\n") {
		t.Fatalf("the commit recorded is %q, of %q", rep.Packs[0].Commit, commit)
	}
	src, _ := Parse(filepath.Join(from, "notes") + "@no-such-ref")
	if _, err := Add(t.TempDir(), src, map[string]string{"TEAM": "ops"}); err == nil || !strings.Contains(err.Error(), "git: ") {
		t.Fatalf("a ref that is not there: %v", err)
	}
}
