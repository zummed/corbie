package role

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// The binary runs one piece of work in a process of its own: a job for a
// runner, a step for a controller. The process is one its starter can stop at
// any moment, and it cannot bring its starter down. It starts with nothing of
// the starter's environment, and the two talk over a channel of their own,
// which nothing it prints can reach.

// Arg is the argument that makes the binary run one piece of work, as the
// process that is started for it.
const Arg = "run-one-job"

// Grace is how long a process that is told to stop is given before it is
// killed, and how long its output is read after it has ended.
const Grace = 2 * time.Second

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

// Open is how a process started by Start begins: it takes its ends of the
// channel, and makes itself a group where the system needs that. Everything
// it starts is in that group, and goes when the group is ended.
func Open() (*Wire, error) {
	from, to := takeChannel()
	if err := startGroup(); err != nil {
		return nil, fmt.Errorf("the process could not make a group of its own: %w", err)
	}
	return &Wire{R: from, W: to}, nil
}

// Lost ends the process, and everything it started with it: its starter has
// gone away, and nothing else would stop them.
func Lost() {
	endGroup()
	os.Exit(3)
}

// WatchOwn calls over once, when what this process holds passes a limit.
func WatchOwn(ctx context.Context, limit int64, over func()) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if int64(m.HeapAlloc) > limit {
				// The count includes what is no longer held and has not
				// been collected yet, so that is collected before the
				// count is judged.
				runtime.GC()
				runtime.ReadMemStats(&m)
			}
			if int64(m.HeapAlloc) > limit {
				over()
				return
			}
		}
	}
}
