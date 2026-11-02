//go:build !unix && !windows

package role

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

// Watch does nothing here: what a process holds is watched from inside.
func Watch(pid int, limit int64, done <-chan struct{}, over func()) { <-done }

func giveChannel(cmd *exec.Cmd, toJob, fromJob *os.File) {
	cmd.ExtraFiles = []*os.File{toJob, fromJob}
}

func takeChannel() (from, to *os.File) {
	return os.NewFile(3, "from its starter"), os.NewFile(4, "to its starter")
}

func startGroup() error { return nil }

// Gone says whether no process has this number any more: here it cannot be
// told, so nothing is taken to be gone.
func Gone(pid int) bool { return false }
