//go:build !unix && !windows

package runner

import (
	"os"
	"os/exec"
)

func ownGroup(cmd *exec.Cmd) {}

func endGroup() {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func (j *supervised) watchMemory(pid int, done <-chan struct{}) { <-done }

func giveChannel(cmd *exec.Cmd, toJob, fromJob *os.File) {
	cmd.ExtraFiles = []*os.File{toJob, fromJob}
}

func takeChannel() (fromRunner, toRunner *os.File) {
	return os.NewFile(3, "from the runner"), os.NewFile(4, "to the runner")
}

func startGroup() error { return nil }

// gone says whether no process has this number any more: here it cannot be
// told, so nothing is taken to be gone.
func gone(pid int) bool { return false }
