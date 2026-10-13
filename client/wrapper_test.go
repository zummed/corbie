package client_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
)

// wrapped gives a runner an exec wrapper that writes down what it was
// started with, its arguments and then its variables, and runs what it was
// given. read gives what it wrote, one start after another.
func wrapped(t *testing.T, l *client.Local, name string) (read func() string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	body := "#!/bin/sh\n{ echo \"args: $*\"; env; echo end; } >> " + log + "\nshift\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "box"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if name == "" {
		name = filepath.Join(dir, "box")
	}
	t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+dir)
	l.Runner.Wrapper = []string{name, "--mark"}
	return func() string {
		written, _ := os.ReadFile(log)
		return string(written)
	}
}

func Test_run_exec_wrapper_fronts_programs_a_plugin_starts(t *testing.T) {
	l := local(t, "all")
	// Named with no path, the wrapper is found where the runner looks for
	// programs.
	read := wrapped(t, l, "box")
	r := script(t, l, `
		fs.write("here.txt", { text: "in the scratch directory" })
		const named = exec.run("echo", { args: ["by", "name"] }).out.trim()
		const where = exec.run("sh", { args: ["-c", "command -v cat"] }).out.trim()
		return { named, pathed: exec.run(where, { args: ["here.txt"] }).out, scratch: exec.run("pwd").out.trim() }
	`, `timeout: 10s, allow: ["all"]`)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	log := read()
	if out["named"] != "by name" || out["pathed"] != "in the scratch directory" || strings.Count(log, "args: --mark ") != 4 {
		t.Fatalf("the programs gave %v, and the wrapper was started:\n%s", out, log)
	}
	// It is given the program as the runner found it, the scratch directory
	// and what the job is allowed.
	for _, want := range []string{"/echo by name\n", "CORBIE_SCRATCH=" + out["scratch"].(string) + "\n", "CORBIE_ALLOW=all\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("the wrapper was not started with %q:\n%s", want, log)
		}
	}
	// Programs are started in one place, so there is no way round it.
	filepath.WalkDir("../plugin", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == "../plugin/job.go" {
			return err
		}
		source, _ := os.ReadFile(path)
		for _, start := range []string{"exec.Command", "os.StartProcess", "syscall.Exec", "syscall.ForkExec"} {
			if strings.Contains(string(source), start) {
				t.Errorf("%s starts a program itself (%s)", path, start)
			}
		}
		return nil
	})
}

func Test_run_job_cannot_change_or_skip_exec_wrapper(t *testing.T) {
	l := local(t, "all")
	read := wrapped(t, l, "")
	r := script(t, l, `
		const env = { LD_PRELOAD: "/tmp/of-the-scripts.so", PATH: "/of/the/script:/usr/bin:/bin", CORBIE_SCRATCH: "/elsewhere",
		              CORBIE_ALLOW: "all", BASH_ENV: "/tmp/run-me", MARK: "for the program\nonly" }
		const out = { seen: exec.run("sh", { args: ["-c", "printf '%s|%s|%s' \"$MARK\" \"$PATH\" \"$BASH_ENV\""], env }).out }
		for (const name of ["-i", "A B", "", "1ST"]) {
		  try { exec.run("true", { env: { [name]: "x" } }); out["took " + name] = true } catch (e) { out[name] = e.code }
		}
		fs.write("a=b", { text: "#!/bin/sh\necho ran", mode: 0o755 })
		out.plain = exec.run("./a=b").out.trim()
		try { exec.run("./a=b", { args: ["sh", "-c", "echo round"], env: { A: "1" } }); out.round = true } catch (e) { out.equals = e.code }
		return out
	`, `timeout: 10s, allow: ["all"]`)
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	check(t, r.Status, r.Output, "done", `{"seen": "for the program\nonly|/of/the/script:/usr/bin:/bin|/tmp/run-me",
		"-i": "bad_input", "A B": "bad_input", "": "bad_input", "1ST": "bad_input", "plain": "ran", "equals": "bad_input"}`)
	// The wrapper itself was started each time, with nothing the script
	// chose among its variables.
	starts := strings.Split(strings.TrimSuffix(read(), "end\n"), "end\n")
	if len(starts) != 2 {
		t.Fatalf("the wrapper was started %d times:\n%s", len(starts), read())
	}
	lines := strings.Split(strings.TrimSpace(starts[0]), "\n")
	if !strings.HasPrefix(lines[0], "args: --mark env ") || !strings.Contains(lines[0], " MARK=for the program") {
		t.Errorf("the wrapper was started with %q", lines[0])
	}
	scratch := ""
	for _, line := range lines {
		name, value, _ := strings.Cut(line, "=")
		switch {
		case name == "CORBIE_SCRATCH":
			scratch = value
		case name == "PATH" && value != os.Getenv("PATH"), name == "CORBIE_ALLOW" && value != "all",
			slices.Contains([]string{"LD_PRELOAD", "BASH_ENV", "MARK"}, name):
			t.Errorf("the wrapper's own variables hold %s", line)
		}
	}
	if scratch == "" || scratch == "/elsewhere" {
		t.Errorf("the wrapper was told the scratch directory is %q", scratch)
	}
}

func Test_run_exec_checked_against_program_the_script_named(t *testing.T) {
	l := local(t, "all")
	read := wrapped(t, l, "")
	r := script(t, l, `return exec.run("echo", { args: ["allowed"] }).out.trim()`, `timeout: 10s, allow: ["exec:echo"]`)
	check(t, r.Status, r.Output, "done", `"allowed"`)
	if !slices.Equal(r.Used, []string{"exec:echo"}) {
		t.Errorf("the run used %v", r.Used)
	}
	r = script(t, l, `return exec.run("cat", { args: ["/etc/hostname"] }).out`, `timeout: 10s, allow: ["exec:echo"]`)
	if r.Status != "failed" || r.Refused != "exec:cat" {
		t.Fatalf("a program the job may not start ended %s, refused %q", r.Status, r.Refused)
	}
	// What was refused never reached the wrapper.
	if log := read(); strings.Count(log, "args: ") != 1 || strings.Contains(log, "cat") {
		t.Errorf("the wrapper was started:\n%s", log)
	}
}
