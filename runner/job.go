package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/store"
)

// supervised is one job being run: a process of its own, watched from
// outside. The process holds no key and reaches no store; everything it
// needs of either it asks of its runner over its channel.
type supervised struct {
	r   *Runner
	s   Served
	w   *waiting
	job *instance.Job
	ctx context.Context

	mu   sync.Mutex
	why  *machine.Error // why it was stopped from outside
	wire *wire
	cmd  *exec.Cmd
	// masks knows every form the job's secrets leave in, for what the
	// job's process prints to be searched.
	masks *plugin.Job

	emits emits
}

// scriptLimit is the largest a script or one of its files may be.
const scriptLimit = 16 << 20

// stopGrace is how long a script that has been told to stop is given
// before its process is killed.
const stopGrace = 2 * time.Second

// stop stops the job from outside: the script is interrupted inside its
// process first, and the process and everything it started is killed if it
// has not ended shortly after.
func (j *supervised) stop(why *machine.Error) {
	j.mu.Lock()
	first := j.why == nil
	if first {
		j.why = why
	}
	w, cmd := j.wire, j.cmd
	j.mu.Unlock()
	if !first || cmd == nil {
		return
	}
	if w != nil {
		w.send(&piece{Op: opStop, Why: why}, nil)
	}
	time.AfterFunc(stopGrace, func() { killGroup(cmd) })
}

func (j *supervised) stopped() *machine.Error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.why
}

// execute runs the job and gives its result. However the job ends, there
// is a result.
func (j *supervised) execute() *instance.Result {
	result := j.run()
	if why := j.stopped(); why != nil && result.Status != instance.Done {
		result = failed(why)
	}
	result.Job, result.Attempt = j.job.Name, j.job.Attempt
	// A job's result is written only after every emit of that job.
	if err := j.emits.flush(j); err != nil {
		return &instance.Result{Job: j.job.Name, Attempt: j.job.Attempt, Status: machine.StatusError,
			Error: &machine.Error{Status: machine.StatusError, Error: "what the job emitted could not be written: " + err.Error()}}
	}
	return result
}

func jobError(format string, args ...any) *instance.Result {
	return failed(&machine.Error{Status: machine.StatusError, Error: fmt.Sprintf(format, args...)})
}

func (j *supervised) run() *instance.Result {
	job, st := j.job, j.s.Store
	script, err := blob.ReadAll(j.ctx, st, job.Script, scriptLimit)
	if err != nil {
		return jobError("the script could not be read whole: %v", err)
	}
	files := map[string]string{}
	for path, ref := range job.Files {
		contents, err := blob.ReadAll(j.ctx, st, ref, scriptLimit)
		if err != nil {
			return jobError("the file %q could not be read whole: %v", path, err)
		}
		files[path] = string(contents)
	}
	base, err := j.r.scratchBase()
	if err != nil {
		return jobError("%v", err)
	}
	scratch, err := os.MkdirTemp(base, fmt.Sprintf("%s%d-*", scratchPrefix, os.Getpid()))
	if err != nil {
		return jobError("no scratch directory could be made: %v", err)
	}
	defer os.RemoveAll(scratch)

	toJob, toJobW, err := os.Pipe()
	if err != nil {
		return jobError("%v", err)
	}
	fromJobR, fromJob, err := os.Pipe()
	if err != nil {
		return jobError("%v", err)
	}
	cmd := exec.Command(j.r.Command[0], j.r.Command[1:]...)
	cmd.Env = []string{} // nothing of the runner's
	cmd.Dir = scratch
	giveChannel(cmd, toJob, fromJob)
	j.masks = &plugin.Job{Secrets: job.Secrets}
	said := &tail{limit: 8 << 10, mask: j.masks.Mask}
	cmd.Stdout, cmd.Stderr = said, said
	cmd.WaitDelay = stopGrace
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return jobError("the job's process could not be started: %v", err)
	}
	toJob.Close()
	fromJob.Close()
	w := &wire{r: fromJobR, w: toJobW}
	j.mu.Lock()
	j.wire, j.cmd = w, cmd
	already := j.why
	j.mu.Unlock()
	// ended sees the process and everything it started gone, and what it
	// printed read to its end.
	ended := func() {
		killGroup(cmd)
		cmd.Wait()
		killGroup(cmd) // once more, for what started at the last moment
	}
	defer func() {
		ended()
		toJobW.Close()
		fromJobR.Close()
	}()
	if already != nil {
		return failed(already)
	}
	// From outside, the time limit is a little past the one the script is
	// interrupted at from inside.
	limit := time.AfterFunc(time.Duration(job.Timeout)+time.Second, func() {
		j.stop(&machine.Error{Status: machine.StatusTimeout, Error: fmt.Sprintf("the script ran past its time limit of %s", job.Timeout)})
	})
	defer limit.Stop()
	if job.Memory > 0 {
		done := make(chan struct{})
		defer close(done)
		go j.watchMemory(cmd.Process.Pid, done)
	}
	// Whether a job is overdue is checked last, at the moment its script
	// would start: a copy can be frozen for any length of time on the way
	// here.
	if !j.r.Now().Before(job.Due) {
		return failed(&machine.Error{Status: machine.StatusTimeout, Error: "the job's result was already overdue when it came to be started"})
	}
	err = w.send(&piece{Op: opBegin, Begin: &begin{Job: job, Script: string(script), Files: files,
		Layers: append(slices.Clone(job.Layers), j.r.ceiling(j.s)), Scratch: scratch, Wrapper: j.r.Wrapper,
		SearchPath: os.Getenv("PATH")}}, nil)
	if err != nil {
		// The process may be gone already. All it printed is quoted, as it
		// is where its channel ends.
		ended()
		return jobError("the job could not be given to its process: %v: %s", err, said.String())
	}
	for {
		p, err := w.receive()
		if err != nil {
			if why := j.stopped(); why != nil {
				return failed(why)
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				// All it printed is quoted, not what had been read of it
				// when its channel closed.
				ended()
				return jobError("the job's process ended without a result: %s", said.String())
			}
			// Nothing that follows an unreadable piece can be trusted.
			return jobError("what the job's process said could not be read: %v", err)
		}
		if p.Op == opResult && p.Result != nil {
			return p.Result
		}
		if err := j.serve(w, p); err != nil {
			return jobError("%v", err)
		}
	}
}

// serve does one thing the job's process asks of its runner.
func (j *supervised) serve(w *wire, p *piece) error {
	st, ctl := j.s.Store, j.s.Controller.Name
	fail := func(err error) error {
		return w.send(&piece{Op: opAnswer, Error: err.Error()}, nil)
	}
	switch p.Op {
	case opEmit:
		return j.emits.add(j, instance.Emitted{Name: p.Name, Data: p.Data, At: j.r.Now().UTC()})
	case opEnv:
		value, set := os.LookupEnv(p.Name)
		return w.send(&piece{Op: opAnswer, Value: value, Set: set}, nil)
	case opRead:
		if p.Ref == nil || p.Length < 0 || p.Length > maxPiece {
			return fail(fmt.Errorf("that is not a part of a blob that can be read"))
		}
		reader, err := blob.Open(j.ctx, st, *p.Ref)
		if err != nil {
			return fail(err)
		}
		part := make([]byte, min(p.Length, max(p.Ref.Size-p.From, 0)))
		if len(part) > 0 {
			if _, err := reader.ReadAt(part, p.From); err != nil && err != io.EOF {
				return fail(err)
			}
		}
		return w.send(&piece{Op: opAnswer, Size: int64(len(part))}, bytes.NewReader(part))
	case opBlob:
		// A blob the job made: its bytes follow, and go into the record of
		// the instance the job is for.
		contents := io.LimitReader(w.r, p.Size)
		ref, err := blob.Write(j.ctx, st, store.Blob(ctl, j.job.Instance, blob.NewName()), p.Type, blob.NewKey(), contents)
		io.Copy(io.Discard, contents)
		if err != nil {
			return fail(err)
		}
		return w.send(&piece{Op: opAnswer, Ref: &ref}, nil)
	case opForm:
		j.masks.Note(p.Value, p.Name)
		return w.send(&piece{Op: opAnswer}, nil)
	case opDefine:
		// A definition made while a machine runs is filled out, checked and
		// stored exactly as one sent by a client is.
		draft, err := machine.ParseMade([]byte(p.Source), machine.Given(p.Files))
		if err != nil {
			var problems machine.Problems
			if errors.As(err, &problems) {
				return w.send(&piece{Op: opAnswer, Problems: problems}, nil)
			}
			return fail(err)
		}
		ref, err := draft.Store(j.ctx, store.Bounded(st, store.SideBySide), ctl, j.job.Instance)
		if err != nil {
			return fail(err)
		}
		return w.send(&piece{Op: opAnswer, Ref: &ref}, nil)
	}
	return fmt.Errorf("the job's process said %q, which is not something a job says", p.Op)
}

// tail keeps the end of what a job's process prints, read to its end so
// that the process is never held up. The values of the job's secrets are
// taken out of what is kept before any of it is cut or squeezed: a value
// cut first can be split so that it is no longer found.
type tail struct {
	mu    sync.Mutex
	buf   []byte
	limit int
	mask  func(string) string
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = []byte(t.mask(string(append(t.buf, p...))))
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(strings.Fields(string(t.buf)), " ")
}

// emits gathers what a job emits and writes it to the instance's record a
// few values at a time: when a batch is full, or a short time after its
// first value, whichever comes first.
type emits struct {
	mu      sync.Mutex
	batch   []instance.Emitted
	entries int            // how many entries have been written
	counts  map[string]int // how many values of each name the job has emitted
	timer   *time.Timer
	err     error
}

const (
	emitBatch = 32
	emitWait  = 200 * time.Millisecond
)

func (e *emits) add(j *supervised, value instance.Emitted) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	e.batch = append(e.batch, value)
	if len(e.batch) >= emitBatch {
		e.err = e.write(j)
		return e.err
	}
	if e.timer == nil {
		e.timer = time.AfterFunc(emitWait, func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			if e.err == nil {
				e.err = e.write(j)
			}
		})
	}
	return nil
}

func (e *emits) flush(j *supervised) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err == nil {
		e.err = e.write(j)
	}
	return e.err
}

// write writes the batch as the next entry of the job's emits, numbered
// from one with none left out, and then tells the controller of each value
// the machine wants to hear about.
func (e *emits) write(j *supervised) error {
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
	if len(e.batch) == 0 {
		return nil
	}
	batch := e.batch
	e.batch = nil
	e.entries++
	r, job, ctl := j.r, j.job, j.s.Controller
	at := store.Emit(ctl.Name, job.Instance, job.Name, e.entries)
	entry := &instance.Emit{Job: job.Name, Number: e.entries, Values: batch}
	data, err := r.ID.Write(at, r.Now(), noEnd, entry, ctl, job.Client)
	if err != nil {
		return err
	}
	if err := add(j.ctx, j.s.Store, at, data); err != nil {
		return err
	}
	if e.counts == nil {
		e.counts = map[string]int{}
	}
	for _, value := range batch {
		e.counts[value.Name]++
		if !wanted(job.Wants, value.Name) {
			continue
		}
		name := store.Message(ctl.Name, r.ID.Name, r.sender.Next(r.Now()))
		msg := &instance.Message{Kind: instance.KindEmit, Instance: job.Instance,
			Emit: &instance.EmitNote{Job: job.Name, Name: value.Name, Index: e.counts[value.Name], Data: value.Data}}
		note, err := r.ID.Write(name, r.Now(), noEnd, msg, ctl)
		if err != nil {
			return err
		}
		if err := add(j.ctx, j.s.Store, name, note); err != nil {
			return err
		}
	}
	return nil
}

// wanted says whether the machine wants to hear about emitted values of a
// name. "*" stands for every name but the two that are for a watcher's
// benefit.
func wanted(wants []string, name string) bool {
	if slices.Contains(wants, name) {
		return true
	}
	return slices.Contains(wants, "*") && name != instance.EmitLog && name != instance.EmitProgress
}
