//go:build unix

package role

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ownGroup puts a process, and so everything it starts, in one group of its
// own.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// endGroup stops everything in the group of the process that calls it, that
// process included.
func endGroup() { syscall.Kill(0, syscall.SIGKILL) }

// Gone says whether no process has this number any more.
func Gone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }

// killGroup stops a process and everything it started.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// Watch calls over when the memory a process really holds goes past a limit:
// what its engine counts is only its own values.
func Watch(pid int, limit int64, done <-chan struct{}, over func()) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
			if err != nil {
				continue
			}
			fields := strings.Fields(string(data))
			if len(fields) < 2 {
				continue
			}
			pages, _ := strconv.ParseInt(fields[1], 10, 64)
			if pages*int64(os.Getpagesize()) > limit {
				over()
				return
			}
		}
	}
}

// giveChannel hands a process its two ends of the channel, as its files 3
// and 4.
func giveChannel(cmd *exec.Cmd, toJob, fromJob *os.File) {
	cmd.ExtraFiles = []*os.File{toJob, fromJob}
}

// takeChannel gives a process its ends of the channel, kept from every
// program it starts.
func takeChannel() (from, to *os.File) {
	syscall.CloseOnExec(3)
	syscall.CloseOnExec(4)
	return os.NewFile(3, "from its starter"), os.NewFile(4, "to its starter")
}

// startGroup does nothing: the starter started the process in a group of its
// own.
func startGroup() error { return nil }
