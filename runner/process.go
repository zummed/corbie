package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/role"
)

// JobArg is the argument that makes the binary run one job, as the process
// a runner starts for it.
const JobArg = role.Arg

// JobMain is the whole of a job's process. It is given the job on the
// channel from its runner, runs the script, and says how it ended. The same
// argument starts the process of a controller's step, so that a program
// built on Corbie needs no more than this.
func JobMain() {
	if len(os.Args) > 2 && os.Args[2] == controller.StepArg {
		controller.StepMain()
		return
	}
	// The channel is the job's own: no program the job starts is handed it.
	// Everything the job starts is in its group, and goes when the group is
	// ended.
	opened, err := role.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "the job's process:", err)
		os.Exit(2)
	}
	w := wire{opened}
	first, err := w.receive()
	if err != nil || first.Op != opBegin || first.Begin == nil {
		os.Exit(2)
	}
	b := first.Begin
	ctx, cancel := context.WithCancel(context.Background())
	ch := &channel{wire: w, answers: make(chan *piece, 1)}
	job := &plugin.Job{Context: ctx, Perms: &permission.Set{Layers: b.Layers, Scratch: b.Scratch}, Scratch: b.Scratch,
		Wrapper: b.Wrapper, SearchPath: b.SearchPath, Secrets: b.Job.Secrets, Channel: ch, Noted: ch.noted}
	e := newEngine(b, job, ch)
	// A stop interrupts the script, and ends the call the script is
	// waiting in, if it is in one: an interrupt reaches only script code.
	stop := func(why *machine.Error) {
		e.stop(why)
		cancel()
	}
	ch.stop = stop
	go ch.listen()

	// A script past its time limit is stopped from inside first.
	limit := time.AfterFunc(time.Duration(b.Job.Timeout), func() {
		stop(&machine.Error{Status: machine.StatusTimeout,
			Error: fmt.Sprintf("the script ran past its time limit of %s", b.Job.Timeout)})
	})
	if b.Job.Memory > 0 {
		go role.WatchOwn(ctx, b.Job.Memory, func() {
			stop(&machine.Error{Status: machine.StatusError,
				Error: fmt.Sprintf("the script went past its memory limit of %d bytes", b.Job.Memory)})
		})
	}
	result := e.run()
	limit.Stop()
	job.End()
	result.Job, result.Attempt, result.Used = b.Job.Name, b.Job.Attempt, job.Perms.Used()
	w.send(&piece{Op: opResult, Result: result}, nil)
	os.Exit(0)
}

// channel is the job's end of its own channel to the runner. A script
// waits for each answer, so one thing is asked at a time.
type channel struct {
	wire
	answers chan *piece
	stop    func(why *machine.Error)
	ask     sync.Mutex
}

// listen reads what the runner says: answers, and the order to stop. When
// the runner goes away, for any reason, also when it is killed outright,
// its end of the channel closes and the job's process ends.
func (c *channel) listen() {
	for {
		p, err := c.receive()
		if err != nil {
			role.Lost()
		}
		switch p.Op {
		case opStop:
			c.stop(p.Why)
		case opAnswer:
			if p.Size > 0 {
				data := make([]byte, p.Size)
				if _, err := io.ReadFull(c.R, data); err != nil {
					role.Lost()
				}
				p.Data = data
			}
			c.answers <- p
		}
	}
}

func (c *channel) asks(p *piece, contents io.Reader) (*piece, error) {
	c.ask.Lock()
	defer c.ask.Unlock()
	if err := c.send(p, contents); err != nil {
		return nil, err
	}
	answer := <-c.answers
	if answer.Error != "" {
		code := answer.Code
		if code == "" {
			code = plugin.Failed
		}
		return nil, plugin.Errorf(code, "%s", answer.Error)
	}
	return answer, nil
}

// noted tells the runner one more form a secret's value is about to leave
// in, and waits until the runner has it. The runner searches what this
// process prints, so it must know the form before the value can be
// printed.
func (c *channel) noted(form, names string) {
	c.asks(&piece{Op: opForm, Value: form, Name: names}, nil)
}

func (c *channel) Emit(name string, data any) error {
	return c.send(&piece{Op: opEmit, Name: name, Data: data}, nil)
}

func (c *channel) Variable(name string) (string, bool, error) {
	answer, err := c.asks(&piece{Op: opEnv, Name: name}, nil)
	if err != nil {
		return "", false, err
	}
	return answer.Value, answer.Set, nil
}

func (c *channel) ReadBlob(ref blob.Ref, from, length int64) ([]byte, error) {
	answer, err := c.asks(&piece{Op: opRead, Ref: &ref, From: from, Length: length}, nil)
	if err != nil {
		return nil, err
	}
	data, _ := answer.Data.([]byte)
	if data == nil {
		data = []byte{}
	}
	return data, nil
}

func (c *channel) WriteBlob(typ string, size int64, contents io.Reader) (blob.Ref, error) {
	if size == 0 {
		contents = bytes.NewReader(nil)
	}
	answer, err := c.asks(&piece{Op: opBlob, Type: typ, Size: size}, contents)
	if err != nil || answer.Ref == nil {
		return blob.Ref{}, err
	}
	return *answer.Ref, nil
}

// Define fills out, checks and stores a definition the script made. It does
// this in the job's process, under the job's limits, so that what reading a
// definition costs falls on this job alone. What it stores goes to the
// runner as blobs the job makes, and the controller checks it again when a
// job starts it.
func (c *channel) Define(source string, files map[string]string) (blob.Ref, error) {
	draft, err := machine.ParseMade([]byte(source), machine.Given(files))
	if err != nil {
		var problems machine.Problems
		if errors.As(err, &problems) {
			return blob.Ref{}, plugin.Errorf(plugin.BadInput, "the definition does not pass its check: %s", strings.ReplaceAll(problems.Error(), "\n", "; "))
		}
		return blob.Ref{}, err
	}
	return draft.Write(context.Background(), func(typ string, contents []byte) (blob.Ref, error) {
		return c.WriteBlob(typ, int64(len(contents)), bytes.NewReader(contents))
	})
}

// failed makes the result of a job that failed.
func failed(e *machine.Error) *instance.Result {
	return &instance.Result{Status: e.Status, Error: e}
}
