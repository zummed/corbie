//go:build !js

package role

import (
	"io"
	"os"
	"os/exec"
)

// A Process is a process started with a channel to it. Nothing but the
// starter holds its ends of the channel.
type Process struct {
	Cmd  *exec.Cmd
	Wire *Wire
	to   *os.File
	from *os.File
}

// Start starts a command in a group of its own, in a directory, with what it
// prints going to out.
func Start(command []string, dir string, out io.Writer) (*Process, error) {
	toChild, toChildW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer toChild.Close()
	fromChildR, fromChild, err := os.Pipe()
	if err != nil {
		toChildW.Close()
		return nil, err
	}
	defer fromChild.Close()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = []string{} // nothing of the starter's
	cmd.Dir = dir
	giveChannel(cmd, toChild, fromChild)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = Grace
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		toChildW.Close()
		fromChildR.Close()
		return nil, err
	}
	return &Process{Cmd: cmd, Wire: &Wire{R: fromChildR, W: toChildW}, to: toChildW, from: fromChildR}, nil
}

// Watch calls over once, when what the process and everything it started
// hold passes a limit, and returns when done closes.
func (p *Process) Watch(limit int64, done <-chan struct{}, over func()) {
	Watch(p.Cmd.Process.Pid, limit, done, over)
}

// Kill stops the process and everything it started.
func (p *Process) Kill() { killGroup(p.Cmd) }

// Ended sees the process and everything it started gone, and what it
// printed read to its end.
func (p *Process) Ended() {
	killGroup(p.Cmd)
	p.Cmd.Wait()
	killGroup(p.Cmd) // once more, for what started at the last moment
}

// Close ends the process and closes the starter's ends of the channel.
func (p *Process) Close() {
	p.Ended()
	p.to.Close()
	p.from.Close()
}
