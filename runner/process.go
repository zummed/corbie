package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// JobArg is the argument that makes the binary run one job, as the process
// a runner starts for it.
const JobArg = "run-one-job"

// JobMain is the whole of a job's process. It is given the job on the
// channel from its runner, runs the script, and says how it ended.
func JobMain() {
	// The channel is the job's own: no program the job starts is handed it.
	from, to := takeChannel()
	w := &wire{r: from, w: to}
	// Everything the job starts is in its group, and goes when the group
	// is ended.
	if err := startGroup(); err != nil {
		fmt.Fprintln(os.Stderr, "the job's process could not make a group of its own:", err)
		os.Exit(2)
	}
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
		go watchMemory(ctx, b.Job.Memory, stop)
	}
	result := e.run()
	limit.Stop()
	job.End()
	result.Job, result.Attempt, result.Used = b.Job.Name, b.Job.Attempt, job.Perms.Used()
	w.send(&piece{Op: opResult, Result: result}, nil)
	os.Exit(0)
}

// watchMemory stops the script when what the process holds passes the
// job's limit.
func watchMemory(ctx context.Context, limit int64, stop func(why *machine.Error)) {
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
				stop(&machine.Error{Status: machine.StatusError,
					Error: fmt.Sprintf("the script went past its memory limit of %d bytes", limit)})
				return
			}
		}
	}
}

// channel is the job's end of its own channel to the runner. A script
// waits for each answer, so one thing is asked at a time.
type channel struct {
	*wire
	answers chan *piece
	stop    func(why *machine.Error)
	ask     sync.Mutex
}

// lost ends the job's process, and everything it started with it: its
// runner has gone away, and nothing else would stop them.
func lost() {
	endGroup()
	os.Exit(3)
}

// listen reads what the runner says: answers, and the order to stop. When
// the runner goes away, for any reason, also when it is killed outright,
// its end of the channel closes and the job's process ends.
func (c *channel) listen() {
	for {
		p, err := c.receive()
		if err != nil {
			lost()
		}
		switch p.Op {
		case opStop:
			c.stop(p.Why)
		case opAnswer:
			if p.Size > 0 {
				data := make([]byte, p.Size)
				if _, err := io.ReadFull(c.r, data); err != nil {
					lost()
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

func (c *channel) Define(source string, files map[string]string) (blob.Ref, error) {
	answer, err := c.asks(&piece{Op: opDefine, Source: source, Files: files}, nil)
	if err != nil {
		return blob.Ref{}, err
	}
	if len(answer.Problems) > 0 {
		return blob.Ref{}, plugin.Errorf(plugin.BadInput, "the definition does not pass its check: %s", strings.ReplaceAll(answer.Problems.Error(), "\n", "; "))
	}
	return *answer.Ref, nil
}

// failed makes the result of a job that failed.
func failed(e *machine.Error) *instance.Result {
	return &instance.Result{Status: e.Status, Error: e}
}
