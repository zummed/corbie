//go:build unix || windows

package runner_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/runner"
)

// standIn plays one of the processes the tests need, and says whether it
// was asked for one. Each writes its number into the file it is given
// first, so a test can tell when it has gone.
func standIn(as string, args []string) bool {
	number := func() { os.WriteFile(args[0], []byte(strconv.Itoa(os.Getpid())), 0o600) }
	switch as {
	case "says-its-parent":
		// A program a job starts, which prints the job's process's number.
		fmt.Print(os.Getppid())
	case "starts-one":
		// A program a job starts, which starts one that goes on by
		// itself, prints the job's process's number and that program's,
		// and ends.
		sleeper := exec.Command(program(), "sleeps")
		if err := sleeper.Start(); err != nil {
			os.Exit(1)
		}
		fmt.Print(os.Getppid(), " ", sleeper.Process.Pid)
	case "says-where-it-is":
		// A program a job starts, which prints its directory and the
		// file it is given.
		where, _ := os.Getwd()
		text, _ := os.ReadFile(args[0])
		fmt.Print(where, "\n", string(text))
	case "sleeps":
		time.Sleep(10 * time.Minute)
	case "job-that-prints":
		// A job's process that prints far more as errors than anything
		// unread would hold, in one line and then a short one.
		os.Stderr.WriteString(strings.Repeat("x", 4<<20) + "\nthe last line\n")
		if args[0] == "and dies" {
			os.Exit(1)
		}
		runner.JobMain()
	case "job-that-dies-at-once":
		// A job's process that says why and is gone, without waiting to
		// be given its job.
		os.Stderr.WriteString("its last words\n")
		number()
		os.Exit(1)
	case "runner-with-a-job":
		// A runner, with a job that starts a program which goes on by
		// itself, says which processes are its own and the program, and
		// then never ends.
		w := newWorld(nil)
		_, err := w.leave(w.ctl, "work", `fs.write(input.marker, { text: exec.run(input.program, { args: ["starts-one"] }).out.trim() }); for (;;) {}`,
			func(j *instance.Job) {
				j.Input, j.Timeout = map[string]any{"marker": args[0], "program": program()}, machine.Duration(time.Minute)
			})
		if err != nil {
			os.Exit(1)
		}
		w.runner("all").Pass(ctx)
	default:
		return standInHere(as, args)
	}
	return true
}

// program is the test's own binary, which a job starts to play a program.
func program() string {
	p, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return p
}

// numbered waits for a process to write its number into a file, and gives
// it. The process is killed when the test ends, if it is still there.
func numbered(t *testing.T, file string) int {
	t.Helper()
	return several(t, file, 1)[0]
}

// several is numbered for a file that holds the numbers of several
// processes.
func several(t *testing.T, file string, count int) []int {
	t.Helper()
	var pids []int
	soon(t, "a process to say which it is", func() bool {
		text, _ := os.ReadFile(file)
		pids = nil
		for _, field := range strings.Fields(string(text)) {
			if pid, _ := strconv.Atoi(field); pid > 0 {
				pids = append(pids, pid)
			}
		}
		return len(pids) == count
	})
	t.Cleanup(func() {
		for _, pid := range pids {
			kill(pid)
		}
	})
	return pids
}

func Test_run_job_running_at_pass_end_is_stopped_and_answered(t *testing.T) {
	w := newWorld(t)
	marker := filepath.Join(t.TempDir(), "running")
	w.job(w.ctl, "long", `fs.write(input.marker, { text: exec.run(input.program, { args: ["says-its-parent"] }).out.trim() }); for (;;) {}`,
		func(j *instance.Job) {
			j.Input, j.Timeout = map[string]any{"marker": marker, "program": program()}, machine.Duration(8*time.Second)
		})
	end := passing(w.runner("all"))
	pid := numbered(t, marker)
	// The pass ends, as it does when its host calls time.
	if err := end(); err == nil {
		t.Fatal("a pass that was ended with a job running gave no error")
	}
	if !ended(pid) {
		t.Fatal("the job's process ran on past the end of the pass")
	}
	got := w.results()["long"]
	if got == nil || got.Status != machine.StatusError || !strings.Contains(got.Error.Error, "the runner stopped") {
		t.Fatalf("the job that was running: %+v", got)
	}
	if left := w.inbox(); len(left) != 0 {
		t.Fatalf("the answered job is still in the inbox: %v", left)
	}
}

func Test_run_job_end_stops_what_started_at_the_last_moment(t *testing.T) {
	w := newWorld(t)
	// The last thing the script does is start a program that goes on by
	// itself, and then it returns.
	w.job(w.ctl, "work", `return exec.run(input.program, { args: ["starts-one"] }).out.trim()`,
		func(j *instance.Job) { j.Input = map[string]any{"program": program()} })
	pass(t, w.runner("all"))
	got := w.results()["work"]
	if got == nil || got.Status != instance.Done {
		t.Fatalf("the result is %+v", got)
	}
	said, _ := got.Output.(string)
	pids := strings.Fields(said)
	pid := 0
	if len(pids) == 2 {
		pid, _ = strconv.Atoi(pids[1])
	}
	if pid <= 0 {
		t.Fatalf("the program's number came back as %v", got.Output)
	}
	t.Cleanup(func() { kill(pid) })
	soon(t, "the program the job started to be stopped", func() bool { return ended(pid) })
}

func Test_run_relative_scratch_setting_resolved_once(t *testing.T) {
	w := newWorld(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	os.Mkdir("scratch-here", 0o700)
	w.job(w.ctl, "work", `
		fs.write("a.txt", { text: "written" })
		const [where, read] = exec.run(input.program, { args: ["says-where-it-is", "a.txt"] }).out.split("\n")
		return { read, where, files: fs.list(".", { deep: true }).length }`,
		func(j *instance.Job) { j.Input = map[string]any{"program": program()} })
	r := w.runner("all")
	r.Scratch = "scratch-here"
	pass(t, r)
	got := w.results()["work"]
	if got == nil || got.Status != instance.Done {
		t.Fatalf("the result is %+v", got)
	}
	out := got.Output.(map[string]any)
	where, _ := out["where"].(string)
	// The job's directory is in the directory the setting names, and the file is
	// in the job's directory, not in a second scratch-here inside it.
	if filepath.Dir(where) != filepath.Join(dir, "scratch-here") || out["read"] != "written" || out["files"] != 1.0 {
		t.Fatalf("the scratch directory is %s, a program started in it read %q, and it holds %v files", where, out["read"], out["files"])
	}
	if left, _ := os.ReadDir("scratch-here"); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func Test_run_long_error_output_does_not_stall_job(t *testing.T) {
	w := newWorld(t)
	// A job whose process prints megabytes as errors before the script
	// starts still runs.
	w.job(w.ctl, "prints", `return "ran"`, nil)
	r := w.runner("all")
	r.Command = []string{os.Args[0], "job-that-prints", "and runs", runner.JobArg}
	pass(t, r)
	if got := w.results()["prints"]; got == nil || got.Output != "ran" {
		t.Fatalf("the job whose process printed a great deal: %+v", got)
	}
	// What a process printed before it died is in the job's error: its
	// end, with the over-long line cut.
	w.job(w.ctl, "dies", `return "ran"`, nil)
	r.Command = []string{os.Args[0], "job-that-prints", "and dies"}
	pass(t, r)
	got := w.results()["dies"]
	if got == nil || got.Status != machine.StatusError || !strings.HasSuffix(got.Error.Error, "the last line") || len(got.Error.Error) > 16<<10 {
		size, text := 0, ""
		if got != nil && got.Error != nil {
			size, text = len(got.Error.Error), got.Error.Error
		}
		t.Fatalf("the job whose process printed and died has an error of %d bytes: %.200s … %s", size, text, text[max(len(text)-100, 0):])
	}
}

// A process can be gone before its runner has given it the job. What it
// printed is in the job's error then too, whichever of the two came first.
func TestProcessGoneBeforeItsJobIsGivenSaysWhatItPrinted(t *testing.T) {
	w := newWorld(t)
	w.job(w.ctl, "dies", `return "ran"`, nil)
	file := filepath.Join(t.TempDir(), "job")
	r := w.runner("all")
	r.Scratch = t.TempDir()
	r.Command = []string{os.Args[0], "job-that-dies-at-once", file}
	// The runner reads its clock once more after it has started the
	// process, and here that takes until the process is gone.
	r.Now = func() time.Time {
		if started, _ := filepath.Glob(filepath.Join(r.Scratch, "*")); len(started) > 0 {
			soon(t, "the job's process to be gone", func() bool {
				data, err := os.ReadFile(file)
				pid, _ := strconv.Atoi(string(data))
				return err == nil && pid > 0 && ended(pid)
			})
		}
		return time.Now()
	}
	pass(t, r)
	got := w.results()["dies"]
	if got == nil || got.Error == nil {
		t.Fatalf("the job whose process was gone before it was given the job has the result %+v", got)
	}
	if got.Status != machine.StatusError || !strings.HasSuffix(got.Error.Error, "its last words") {
		t.Fatalf("the job whose process was gone before it was given the job: %s", got.Error.Error)
	}
}

func Test_run_job_process_does_not_outlive_killed_runner(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "job")
	killed := exec.Command(os.Args[0], "runner-with-a-job", marker)
	killed.Stderr = os.Stderr
	if err := killed.Start(); err != nil {
		t.Fatal(err)
	}
	defer killed.Process.Kill()
	pids := several(t, marker, 2)
	job, program := pids[0], pids[1]
	if ended(job) || ended(program) {
		t.Fatal("the job's process, or the program it started, had ended before the runner was killed")
	}
	// Killed outright: the runner does nothing more, for the job or anything.
	killed.Process.Kill()
	killed.Wait()
	soon(t, "the job's process to end", func() bool { return ended(job) })
	// Nothing is left to stop what the job started, so it goes with it.
	soon(t, "the program the job started to end", func() bool { return ended(program) })
}
