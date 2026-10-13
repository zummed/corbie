//go:build unix

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zummed/corbie/machine"
)

// ownGroup puts a job's process, and so everything it starts, in one group
// of its own.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// endGroup stops everything in the group of the process that calls it, that
// process included: a job's process, and everything it started.
func endGroup() { syscall.Kill(0, syscall.SIGKILL) }

// gone says whether no process has this number any more.
func gone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }

// killGroup stops a job's process and everything it started.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// watchMemory kills the job's process when the memory it really holds goes
// well past its limit: the script's engine counts only its own values.
func (j *supervised) watchMemory(pid int, done <-chan struct{}) {
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
			if held := pages * int64(os.Getpagesize()); held > 2*j.job.Memory+(64<<20) {
				j.stop(&machine.Error{Status: machine.StatusError,
					Error: fmt.Sprintf("the job went past its memory limit of %d bytes", j.job.Memory)})
				killGroup(j.cmd)
				return
			}
		}
	}
}

// closeOnExec keeps files of the job's process from every program it
// starts.
func closeOnExec(fds ...int) {
	for _, fd := range fds {
		syscall.CloseOnExec(fd)
	}
}
