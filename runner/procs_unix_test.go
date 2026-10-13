//go:build unix

package runner_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	// The two ends of a job's channel, which only a job's process has.
	fromRunner := func() *os.File { return os.NewFile(3, "from the runner") }
	toRunner := func() *os.File { return os.NewFile(4, "to the runner") }
	switch as {
	case "job-with-output-held":
		// A job's process that has started a program which holds the
		// process's own output open, and is in another group.
		holder := exec.Command(args[1], "600")
		holder.Stdout, holder.Stderr = os.Stdout, os.Stderr
		holder.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := holder.Start(); err != nil {
			os.Exit(1)
		}
		os.WriteFile(args[0], []byte(strconv.Itoa(holder.Process.Pid)), 0o600)
		runner.JobMain()
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
	case "job-that-says":
		// A job's process that says something of its own on the channel,
		// and then stays.
		number()
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(args[1])))
		toRunner().Write(append(size[:], args[1]...))
		io.Copy(io.Discard, fromRunner())
	case "job-that-holds-memory":
		// A job's process that holds memory no script made, and stays.
		number()
		held := make([]byte, 192<<20)
		for i := range held {
			held[i] = 1
		}
		io.Copy(io.Discard, fromRunner())
		os.Stdout.Write(held[:1])
	case "runner-with-a-job":
		// A runner, with a job that starts a program which goes on by
		// itself, says which processes are its own and the program, and
		// then never ends.
		w := newWorld(nil)
		_, err := w.leave(w.ctl, "work", `fs.write(input.marker, { text: exec.run("sh", { args: ["-c", "sleep 600 >/dev/null 2>&1 & echo $PPID $!"] }).out.trim() }); for (;;) {}`,
			func(j *instance.Job) {
				j.Input, j.Timeout = map[string]any{"marker": args[0]}, machine.Duration(time.Minute)
			})
		if err != nil {
			os.Exit(1)
		}
		w.runner("all").Pass(ctx)
	default:
		return false
	}
	return true
}

// ended says whether a process has ended: nothing has its number, or what
// has it is only waiting to be collected.
func ended(pid int) bool {
	if syscall.Kill(pid, 0) == syscall.ESRCH {
		return true
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if i := bytes.LastIndexByte(stat, ')'); err == nil && i >= 0 && i+2 < len(stat) {
		return stat[i+2] == 'Z'
	}
	return false
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
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return pids
}

func Test_run_job_running_at_pass_end_is_stopped_and_answered(t *testing.T) {
	w := newWorld(t)
	marker := filepath.Join(t.TempDir(), "running")
	w.job(w.ctl, "long", `fs.write(input.marker, { text: exec.run("sh", { args: ["-c", "echo $PPID"] }).out.trim() }); for (;;) {}`,
		func(j *instance.Job) {
			j.Input, j.Timeout = map[string]any{"marker": marker}, machine.Duration(8*time.Second)
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
	w.job(w.ctl, "work", `return exec.run("sh", { args: ["-c", "sleep 600 >/dev/null 2>&1 & echo $!"] }).out.trim()`, nil)
	pass(t, w.runner("all"))
	got := w.results()["work"]
	if got == nil || got.Status != instance.Done {
		t.Fatalf("the result is %+v", got)
	}
	pid, _ := strconv.Atoi(got.Output.(string))
	if pid <= 0 {
		t.Fatalf("the program's number came back as %v", got.Output)
	}
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	soon(t, "the program the job started to be stopped", func() bool { return ended(pid) })
}

func Test_run_job_ends_though_started_program_holds_its_output(t *testing.T) {
	w := newWorld(t)
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "holder")
	w.job(w.ctl, "work", `return "ran"`, nil)
	r := w.runner("all")
	r.Command = []string{os.Args[0], "job-with-output-held", file, sleep}
	// A pass that waits on that output for ever cannot be ended, so the
	// test only waits for it as long as it waits for anything.
	done := make(chan error, 1)
	go func() {
		_, err := r.Pass(ctx)
		done <- err
	}()
	holder := numbered(t, file)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the runner is still waiting for an output that a started program holds open")
	}
	if got := w.results()["work"]; got == nil || got.Output != "ran" {
		t.Fatalf("the result is %+v", got)
	}
	if ended(holder) {
		t.Fatal("the program that held the output had ended: the test showed nothing")
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

func Test_run_unreadable_channel_stops_job_and_fails_it(t *testing.T) {
	// Something that is not a piece at all, and a piece cut short.
	for name, said := range map[string]string{"junk": "this is nothing a job says", "cut-short": `{"op":"result","result":{"status":"do`} {
		w := newWorld(t)
		file := filepath.Join(t.TempDir(), "job")
		w.job(w.ctl, name, `return "ran"`, nil)
		r := w.runner("all")
		r.Command = []string{os.Args[0], "job-that-says", file, said}
		end := passing(r)
		pid := numbered(t, file)
		soon(t, "the job to be answered", func() bool { return w.results()[name] != nil })
		got := w.results()[name]
		if got.Status != machine.StatusError || !strings.Contains(got.Error.Error, "could not be read") {
			t.Errorf("%s on the channel: the job's result is %+v", name, got)
		}
		// The process had not ended by itself: the runner stopped it.
		soon(t, "the job's process to be stopped", func() bool { return ended(pid) })
		end()
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
