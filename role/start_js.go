//go:build js

package role

import (
	"io"
	"os/exec"
)

// In WebAssembly there is one process, and it starts no other. A piece of
// work that has a process of its own elsewhere is done here beside its
// starter, over a channel of the same kind.

// A Process is a piece of work started with a channel to it.
type Process struct {
	Cmd  *exec.Cmd // none here
	Wire *Wire
	end  func()
}

// Work is what a started piece of work does with its end of the channel,
// given the command that starts its process elsewhere.
var Work func(command []string, w *Wire)

// Start starts a piece of work beside its starter.
func Start(command []string, dir string, out io.Writer) (*Process, error) {
	toWork, fromStarter := io.Pipe()
	toStarter, fromWork := io.Pipe()
	go func() {
		Work(command, &Wire{R: toWork, W: fromWork})
		fromWork.Close()
	}()
	return &Process{Wire: &Wire{R: toStarter, W: fromStarter}, end: func() {
		fromStarter.Close()
		toStarter.Close()
	}}, nil
}

// Kill stops the work: its channel closes, and it ends as it would with its starter gone.
func (p *Process) Kill() { p.end() }

// Ended is nothing here: the work is no process to wait for.
func (p *Process) Ended() {}

// Close closes the starter's ends of the channel.
func (p *Process) Close() { p.end() }
