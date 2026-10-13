package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

func newJob(t *testing.T, allow, deny []string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow, Deny: deny}}}}
	job.Perms.Scratch = job.Scratch
	t.Cleanup(job.End)
	return job
}

func do(job *plugin.Job, name string, first any, opt plugin.Options) (any, error) {
	module, _ := plugin.Find("fs")
	return module.Calls[name].Invoke(job, "fs", name, first, opt)
}

// tree makes files, each holding its own name, and gives a func that says
// which of them are still there.
func tree(t *testing.T, root string, files ...string) func() string {
	t.Helper()
	for _, file := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(root, file)), 0o755)
		if err := os.WriteFile(filepath.Join(root, file), []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return func() string {
		var there []string
		for _, file := range files {
			if _, err := os.Lstat(filepath.Join(root, file)); err == nil {
				there = append(there, file)
			}
		}
		return strings.Join(there, " ")
	}
}

func Test_plug_fs_remove_link_not_target(t *testing.T) {
	// Everything on the disk is allowed, so nothing but the plugin stands
	// between a link and what it points to.
	job := newJob(t, []string{"disk"}, nil)
	elsewhere := t.TempDir()
	kept := tree(t, elsewhere, "file.txt", "directory/inside.txt", "directory/deeper/more.txt")
	os.Symlink(filepath.Join(elsewhere, "file.txt"), filepath.Join(job.Scratch, "to-file"))
	os.Symlink(filepath.Join(elsewhere, "directory"), filepath.Join(job.Scratch, "to-directory"))
	os.MkdirAll(filepath.Join(job.Scratch, "holds"), 0o755)
	os.Symlink(filepath.Join(elsewhere, "directory"), filepath.Join(job.Scratch, "holds", "a-link"))
	for _, remove := range []struct {
		path string
		opt  plugin.Options
	}{
		{"to-file", nil},
		{"to-directory", nil},
		{filepath.Join(job.Scratch, "holds"), plugin.Options{"deep": true}},
	} {
		if _, err := do(job, "remove", remove.path, remove.opt); err != nil {
			t.Fatalf("removing %s: %v", remove.path, err)
		}
	}
	os.Symlink(filepath.Join(elsewhere, "directory"), filepath.Join(job.Scratch, "deep-link"))
	if _, err := do(job, "remove", "deep-link", plugin.Options{"deep": true}); err != nil {
		t.Fatalf("removing a link to a directory, and all inside it: %v", err)
	}
	if left, _ := os.ReadDir(job.Scratch); len(left) != 0 {
		t.Errorf("%d of the links are still there", len(left))
	}
	if got := kept(); got != "file.txt directory/inside.txt directory/deeper/more.txt" {
		t.Fatalf("after the links were removed, what they pointed to holds: %s", got)
	}
	// A directory on the way to the name is followed, as for any path: what
	// is removed is the name where its directory really is, and that is where
	// it is checked.
	narrow := newJob(t, []string{"fs.write:**"}, nil)
	os.Symlink(filepath.Join(elsewhere, "directory"), filepath.Join(narrow.Scratch, "through"))
	_, err := do(narrow, "remove", "through/inside.txt", nil)
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || !strings.HasSuffix(refusal.What, "/directory/inside.txt") {
		t.Fatalf("removing a file through a link to a directory the job may not write gave %v", err)
	}
}

func Test_plug_fs_remove_checks_every_entry(t *testing.T) {
	files := []string{"tree/a.txt", "tree/sub/b.txt", "tree/sub/keep/c.txt", "tree/sub/keep/more/d.txt", "tree/z.txt"}
	// Writing is all the job is allowed: removing reads nothing.
	job := newJob(t, []string{"fs.write:**"}, []string{"fs.write:tree/sub/keep/**"})
	there := tree(t, job.Scratch, files...)
	_, err := do(job, "remove", "tree", plugin.Options{"deep": true})
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || refusal.What != "fs.write:tree/sub/keep" {
		t.Fatalf("removing a directory with a denied place inside it gave %v", err)
	}
	if got := there(); got != strings.Join(files, " ") {
		t.Fatalf("after a removal that was refused, the directory holds: %s", got)
	}
	// With nothing inside refused, all of it goes.
	free := newJob(t, []string{"fs.write:**"}, nil)
	there = tree(t, free.Scratch, files...)
	if _, err := do(free, "remove", "tree", plugin.Options{"deep": true}); err != nil || there() != "" {
		t.Fatalf("removing the directory gave %v, and left: %s", err, there())
	}
	if _, err := os.Lstat(filepath.Join(free.Scratch, "tree")); err == nil {
		t.Fatal("the directory itself is still there")
	}
	// And a directory that holds something is not removed by a call that does
	// not say all inside it.
	there = tree(t, free.Scratch, files...)
	if _, err := do(free, "remove", "tree", nil); err == nil || there() != strings.Join(files, " ") {
		t.Fatalf("removing a directory that is not empty, without saying deep, gave %v and left: %s", err, there())
	}
}

func TestMoveOfADirectoryChecksEveryEntry(t *testing.T) {
	files := []string{"tree/a.txt", "tree/sub/b.txt", "tree/sub/keep/c.txt", "tree/z.txt"}
	refusedAt := func(err error) string {
		var refusal *permission.Refusal
		errors.As(err, &refusal)
		if refusal == nil {
			return ""
		}
		return refusal.What
	}
	// A denied place inside the directory that is moved: moving the directory
	// would take it away from where it is.
	job := newJob(t, []string{"fs.write:**"}, []string{"fs.write:tree/sub/keep/**"})
	there := tree(t, job.Scratch, files...)
	_, err := do(job, "move", "tree", plugin.Options{"to": "moved"})
	if at := refusedAt(err); at != "fs.write:tree/sub/keep" {
		t.Fatalf("moving a directory with a denied place inside it gave %v", err)
	}
	if _, err := os.Lstat(filepath.Join(job.Scratch, "moved")); err == nil || there() != strings.Join(files, " ") {
		t.Fatalf("after a move that was refused, the directory holds: %s", there())
	}
	// A denied place inside where the directory would go: moving the directory
	// would write it.
	job = newJob(t, []string{"fs.write:**"}, []string{"fs.write:moved/sub/keep/c.txt"})
	there = tree(t, job.Scratch, files...)
	_, err = do(job, "move", "tree", plugin.Options{"to": "moved"})
	if at := refusedAt(err); at != "fs.write:moved/sub/keep/c.txt" {
		t.Fatalf("moving a directory onto a denied place gave %v", err)
	}
	if _, err := os.Lstat(filepath.Join(job.Scratch, "moved")); err == nil || there() != strings.Join(files, " ") {
		t.Fatalf("after a move that was refused, the directory holds: %s", there())
	}
	// With nothing refused, all of it moves, and a link inside is moved
	// as a link.
	free := newJob(t, []string{"fs.write:**"}, nil)
	there = tree(t, free.Scratch, files...)
	os.Symlink(t.TempDir(), filepath.Join(free.Scratch, "tree", "a-link"))
	if _, err := do(free, "move", "tree", plugin.Options{"to": "moved"}); err != nil || there() != "" {
		t.Fatalf("moving the directory gave %v, and left: %s", err, there())
	}
	for _, file := range files {
		if _, err := os.Lstat(filepath.Join(free.Scratch, "moved", strings.TrimPrefix(file, "tree/"))); err != nil {
			t.Fatalf("after the move, %s is not where it went: %v", file, err)
		}
	}
	if info, err := os.Lstat(filepath.Join(free.Scratch, "moved", "a-link")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a link inside the directory did not move as a link: %v", err)
	}
}

// What a job used in its scratch directory is noted as a permission is
// written for a place there, so the entries of one job allow the same in
// the next, whose directory has another name.
func TestUsedEntryForAScratchPathWorksInTheNextJob(t *testing.T) {
	first := newJob(t, []string{"all"}, nil)
	if _, err := do(first, "write", "out/note.txt", plugin.Options{"text": "noted"}); err != nil {
		t.Fatal(err)
	}
	used := first.Perms.Used()
	if len(used) != 1 || used[0] != "fs.write:out/note.txt" {
		t.Fatalf("a write into the scratch directory is noted as %v", used)
	}
	next := newJob(t, used, nil)
	if _, err := do(next, "write", "out/note.txt", plugin.Options{"text": "noted"}); err != nil {
		t.Fatalf("with the entries the first job used, the next was refused: %v", err)
	}
	// And a refusal there names the line the same way.
	_, err := do(next, "write", "out/other.txt", plugin.Options{"text": "noted"})
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || refusal.What != "fs.write:out/other.txt" || !strings.Contains(refusal.Fix, "add the line fs.write:out/other.txt to") {
		t.Fatalf("a write the job is not allowed, into its scratch directory, gave %v", err)
	}
}

func TestListLeavesOutWhatTheJobMayNotRead(t *testing.T) {
	job := newJob(t, []string{"fs.read:**"}, []string{"fs.read:tree/sub/keep/**", "fs.read:tree/z.txt"})
	tree(t, job.Scratch, "tree/a.txt", "tree/sub/b.txt", "tree/sub/keep/c.txt", "tree/z.txt")
	listed, err := do(job, "list", "tree", plugin.Options{"deep": true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range listed.([]any) {
		names = append(names, entry.(map[string]any)["name"].(string))
	}
	if got := strings.Join(names, " "); got != "a.txt sub sub/b.txt" {
		t.Fatalf("the listing shows: %s", got)
	}
	// Only the directory asked for is noted as used: looking at what is in it
	// used nothing.
	if used := job.Perms.Used(); len(used) != 1 || used[0] != "fs.read:tree" {
		t.Fatalf("the listing is noted as having used %v", used)
	}
}

func Test_plug_fs_write_keeps_mode(t *testing.T) {
	job := newJob(t, []string{"disk"}, nil)
	mode := func(name string) os.FileMode {
		t.Helper()
		info, err := os.Stat(filepath.Join(job.Scratch, name))
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	for name, was := range map[string]os.FileMode{"private": 0o600, "program": 0o750, "read-only-to-others": 0o640} {
		if err := os.WriteFile(filepath.Join(job.Scratch, name), []byte("as it was"), was); err != nil {
			t.Fatal(err)
		}
		os.Chmod(filepath.Join(job.Scratch, name), was)
		for _, opt := range []plugin.Options{{"text": "written over"}, {"text": " and more", "append": true}, {"bytes": plugin.Bytes("bytes")}} {
			if _, err := do(job, "write", name, opt); err != nil {
				t.Fatal(err)
			}
			if got := mode(name); got != was {
				t.Fatalf("a file of mode %v, written over with %v, has mode %v", was, opt, got)
			}
		}
		if got, _ := os.ReadFile(filepath.Join(job.Scratch, name)); string(got) != "bytes" {
			t.Fatalf("the file holds %q", got)
		}
	}
	// A file that is new is given the mode the call says, or the usual one.
	if _, err := do(job, "write", "new", plugin.Options{"text": "x"}); err != nil || mode("new")&^0o644 != 0 || mode("new")&0o600 != 0o600 {
		t.Fatalf("a new file: %v, mode %v", err, mode("new"))
	}
	if _, err := do(job, "write", "new-private", plugin.Options{"text": "x", "mode": float64(0o600)}); err != nil || mode("new-private") != 0o600 {
		t.Fatalf("a new file made private: %v, mode %v", err, mode("new-private"))
	}
}

// Moving a link moves the link, as removing one removes it: what it points
// to stays where it is, and a link in the way is replaced, not followed.
func TestMoveMovesTheLinkAndNotItsTarget(t *testing.T) {
	job := newJob(t, []string{"disk"}, nil)
	elsewhere := t.TempDir()
	kept := tree(t, elsewhere, "file.txt", "other.txt")
	os.Symlink(filepath.Join(elsewhere, "file.txt"), filepath.Join(job.Scratch, "link"))
	os.Symlink(filepath.Join(elsewhere, "other.txt"), filepath.Join(job.Scratch, "in-the-way"))
	if _, err := do(job, "move", "link", plugin.Options{"to": "in-the-way"}); err != nil {
		t.Fatal(err)
	}
	if to, err := os.Readlink(filepath.Join(job.Scratch, "in-the-way")); err != nil || to != filepath.Join(elsewhere, "file.txt") {
		t.Fatalf("the moved link points to %q (%v)", to, err)
	}
	if _, err := os.Lstat(filepath.Join(job.Scratch, "link")); err == nil || kept() != "file.txt other.txt" {
		t.Fatalf("after the move the link is still there, or what it pointed to is not: %s", kept())
	}
	// A file is moved as before, into a directory that is made for it.
	tree(t, job.Scratch, "a.txt")
	if _, err := do(job, "move", "a.txt", plugin.Options{"to": "made/b.txt"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(job.Scratch, "made", "b.txt")); string(got) != "a.txt" {
		t.Fatalf("the moved file holds %q", got)
	}
}
