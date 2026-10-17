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

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/runner"
)

// standInHere plays the processes only tests on unix need.
func standInHere(as string, args []string) bool {
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
	default:
		return false
	}
	return true
}

// kill stops a process, if it is still there.
func kill(pid int) { syscall.Kill(pid, syscall.SIGKILL) }

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
