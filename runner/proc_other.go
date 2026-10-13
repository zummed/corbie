//go:build !unix

package runner

import "os/exec"

func ownGroup(cmd *exec.Cmd) {}

func endGroup() {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func (j *supervised) watchMemory(pid int, done <-chan struct{}) { <-done }

func closeOnExec(fds ...int) {}

// gone says whether no process has this number any more: here it cannot be
// told, so nothing is taken to be gone.
func gone(pid int) bool { return false }
