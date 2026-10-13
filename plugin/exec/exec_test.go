package exec

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

func start(t *testing.T, job *plugin.Job, program string, opt plugin.Options) (map[string]any, error) {
	module, _ := plugin.Find("exec")
	out, err := module.Calls["run"].Invoke(job, "exec", "run", program, opt)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

// A blob given as a program's input is read by the program as it is read
// from where it lies, and a program that prints more than is handed back
// fails the call: nothing is cut off quietly.
func TestInputIsReadThroughAndOutputIsBounded(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell on this machine")
	}
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), SearchPath: "/usr/bin:/bin",
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"all"}}}}}
	job.Perms.Scratch = job.Scratch
	t.Cleanup(job.End)
	// Twenty megabytes in a file, as a blob, counted by a program.
	big := filepath.Join(job.Scratch, "big.bin")
	os.WriteFile(big, make([]byte, 20<<20), 0o600)
	input, err := job.FileBlob("big.bin", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := start(t, job, "wc", plugin.Options{"args": []any{"-c"}, "input": input})
	if err != nil || strings.TrimSpace(out["out"].(string)) != "20971520" {
		t.Fatalf("the program counted %v, %v", out, err)
	}
	// What a program prints may be as large as anything handed to a script
	// whole; more than that fails the call, saying the way out, and the
	// program is not held up.
	os.WriteFile(filepath.Join(job.Scratch, "huge.bin"), make([]byte, plugin.MaxWhole+1<<20), 0o600)
	if out, err = start(t, job, "cat", plugin.Options{"args": []any{"big.bin"}, "as": "bytes"}); err != nil || len(out["out"].(plugin.Bytes)) != 20<<20 {
		t.Fatalf("twenty megabytes printed back gave %v", err)
	}
	_, err = start(t, job, "cat", plugin.Options{"args": []any{"huge.bin"}})
	if err == nil || !strings.Contains(err.Error(), `ask for it as a blob, with { as: "blob" }`) {
		t.Fatalf("a program that printed more than is held whole gave %v", err)
	}
	// As a blob it goes into the scratch directory as it is printed.
	out, err = start(t, job, "cat", plugin.Options{"args": []any{"huge.bin"}, "as": "blob"})
	if err != nil || out["out"].(*plugin.Blob).Size != plugin.MaxWhole+1<<20 {
		t.Fatalf("the same as a blob gave %v, %v", out, err)
	}
}

func Test_plug_exec_relative_program_from_scratch(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell on this machine")
	}
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), SearchPath: "/usr/bin:/bin",
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"exec:bin/sh"}}}}}
	job.Perms.Scratch = job.Scratch
	t.Cleanup(job.End)
	os.MkdirAll(filepath.Join(job.Scratch, "bin"), 0o755)
	os.WriteFile(filepath.Join(job.Scratch, "bin", "sh"), []byte("#!/bin/sh\necho the one in the scratch directory\n"), 0o755)
	// The process stands at the root of the disk, where bin/sh is the
	// machine's own shell. The one that runs is the scratch directory's.
	t.Chdir("/")
	out, err := start(t, job, "bin/sh", plugin.Options{"args": []any{"-c", "echo the one at the root of the disk"}})
	if err != nil || out["out"] != "the one in the scratch directory\n" {
		t.Fatalf("bin/sh gave %v, %v", out, err)
	}
	if used := job.Perms.Used(); len(used) != 1 || used[0] != "exec:bin/sh" {
		t.Fatalf("what was used is %v", used)
	}
	// A job with no scratch directory has no place a path with no leading
	// slash could mean. It does not come to mean one from the root.
	nowhere := &plugin.Job{Context: context.Background(), SearchPath: "/usr/bin:/bin",
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"all"}}}}}
	t.Cleanup(nowhere.End)
	out, err = start(t, nowhere, "bin/sh", plugin.Options{"args": []any{"-c", "echo the one at the root of the disk"}})
	var bad *plugin.Error
	if !errors.As(err, &bad) || bad.Code != plugin.BadInput {
		t.Fatalf("with no scratch directory, bin/sh gave %v, %v", out, err)
	}
}

// A secret bound to a program named by a path goes to it, however the path
// is written: the program is named for the secret as it is for the start.
func TestSecretGoesToAProgramNamedByItsPath(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell on this machine")
	}
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), SearchPath: "/usr/bin:/bin",
		Secrets: map[string]string{"K": "value-of-k"},
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"exec:bin/tool", "secret:K@exec:bin/tool"}}}}}
	job.Perms.Scratch = job.Scratch
	t.Cleanup(job.End)
	os.MkdirAll(filepath.Join(job.Scratch, "bin"), 0o755)
	os.WriteFile(filepath.Join(job.Scratch, "bin", "tool"), []byte("#!/bin/sh\necho got $1\n"), 0o755)
	os.Symlink(filepath.Join(job.Scratch, "bin", "tool"), filepath.Join(job.Scratch, "other"))
	for _, name := range []string{"bin/tool", "./bin/tool", filepath.Join(job.Scratch, "bin", "tool"), "./other"} {
		out, err := start(t, job, name, plugin.Options{"args": []any{plugin.NewSecret("K")}})
		if err != nil || out["out"] != "got value-of-k\n" {
			t.Errorf("named %s, the program gave %v, %v", name, out, err)
		}
	}
	// And not to a program it is not bound to.
	os.WriteFile(filepath.Join(job.Scratch, "elsewhere"), []byte("#!/bin/sh\necho got $1\n"), 0o755)
	job.Perms.Layers[0].Allow = append(job.Perms.Layers[0].Allow, "exec:elsewhere")
	if _, err := start(t, job, "./elsewhere", plugin.Options{"args": []any{plugin.NewSecret("K")}}); err == nil {
		t.Error("the secret went to a program it is not bound to")
	}
}
