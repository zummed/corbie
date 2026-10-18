// Package runner does the work. It takes jobs from its inbox, runs each
// one's script in a process of its own with the permissions the job was
// given, and sends back the result.
package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A Runner serves the controllers it trusts, each in that controller's
// store and under the ceiling it gave that controller.
type Runner struct {
	ID     *identity.Identity
	Serves []Served
	// Trusted, if set, reads afresh at each pass which controllers the
	// runner serves and under what ceiling, from where its trust is kept:
	// a ceiling set again, or a controller removed, holds from the next
	// job.
	Trusted func() ([]Served, error)
	// Jobs is how many jobs are run at once.
	Jobs int
	// Command starts a job's process; by default, this binary.
	Command []string
	// Wrapper is the exec wrapper: a command put in front of every program
	// a job starts.
	Wrapper []string
	// Scratch is where jobs' scratch directories are made.
	Scratch string
	// Now is the runner's clock.
	Now func() time.Time

	sender  store.Sender
	cleared sync.Once // what a runner that is gone left behind is cleared once

	// What follows is one pass's own, and is made anew for each: nothing a
	// pass knows of the jobs it ran is carried to the next.
	mu       sync.Mutex
	copy     string              // which pass of which copy this is
	whole    time.Duration       // how long the host gave this pass as it began; none where the host sets no limit
	late     map[string]bool     // the jobs this pass found it had too little time left for
	running  map[string]*running // the jobs this pass is running, by name
	finished map[string]bool     // the jobs this pass ran, and whether each one's result was written
	failed   error               // what kept the results of jobs from being written
	freed    chan struct{}
}

// Served is one controller a runner serves.
type Served struct {
	Controller identity.Card
	// Ceiling is the most the runner lets that controller's jobs do.
	Ceiling permission.Layer
	Store   store.Store
}

// running is a job this copy is running.
type running struct {
	stop func(why *machine.Error)
}

// New makes a runner.
func New(id *identity.Identity, serves ...Served) *Runner {
	exe, _ := os.Executable()
	return &Runner{ID: id, Serves: serves, Jobs: 4, Command: []string{exe, JobArg}, Now: time.Now,
		running: map[string]*running{}, freed: make(chan struct{}, 1)}
}

// cancelPoll is how often the copy that is running a job looks for a
// cancel for it.
const cancelPoll = 2 * time.Second

// mark is what a runner writes to take a job: which copy took it, and when
// the job's result is due, after which the mark goes.
type mark struct {
	Copy string    `json:"copy"`
	Due  time.Time `json:"due"`
}

// scratchPrefix starts the name of every job's scratch directory, and the
// number of the process that made it follows.
const scratchPrefix = "corbie-job-"

// scratchBase is the directory jobs' scratch directories are made in.
func (r *Runner) scratchBase() (string, error) {
	base := r.Scratch
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Abs(base)
}

// clearScratch removes the scratch directories a runner that is gone left
// behind: one that was killed had no chance to remove its jobs' own. A
// directory is left alone while the process that made it is still there.
func (r *Runner) clearScratch() {
	base, err := r.scratchBase()
	if err != nil {
		return
	}
	left, _ := filepath.Glob(filepath.Join(base, scratchPrefix+"*"))
	for _, directory := range left {
		pid, _, _ := strings.Cut(strings.TrimPrefix(filepath.Base(directory), scratchPrefix), "-")
		if n, err := strconv.Atoi(pid); err == nil && n != os.Getpid() && gone(n) {
			os.RemoveAll(directory)
		}
	}
}

// Pass takes every job in the runner's inbox that it can finish and runs
// it. It does not end while a job it took is still running.
func (r *Runner) Pass(ctx context.Context) (time.Time, error) {
	r.cleared.Do(r.clearScratch)
	copyID := make([]byte, 6)
	rand.Read(copyID)
	r.mu.Lock()
	r.copy, r.finished, r.failed = hex.EncodeToString(copyID), map[string]bool{}, nil
	r.mu.Unlock()
	r.late = map[string]bool{}
	// What the host gives one call is what this pass has as it begins.
	r.whole = 0
	if deadline, ok := ctx.Deadline(); ok {
		r.whole = deadline.Sub(r.Now())
	}
	if r.Trusted != nil {
		serves, err := r.Trusted()
		if err != nil {
			return time.Time{}, err
		}
		r.Serves = serves
	}
	var notices []<-chan struct{}
	for _, s := range r.Serves {
		ch, stop := s.Store.Notice(store.Inbox(r.ID.Name))
		defer stop()
		notices = append(notices, ch)
	}
	// Nothing of the pass goes on once it has ended.
	watching, done := context.WithCancel(ctx)
	defer done()
	woken := merge(watching, notices)
	var failure error
	for {
		if err := r.look(ctx); err != nil {
			failure = err
		}
		r.mu.Lock()
		busy := len(r.running)
		r.mu.Unlock()
		if busy == 0 {
			break
		}
		select {
		case <-r.freed: // a place came free: look again at once
		case <-woken:
		case <-time.After(cancelPoll):
		case <-ctx.Done():
			// Nothing of a job runs past the end of its copy's pass.
			r.mu.Lock()
			for _, job := range r.running {
				job.stop(&machine.Error{Status: machine.StatusError, Error: "the runner stopped while the job was running"})
			}
			r.mu.Unlock()
			for {
				r.mu.Lock()
				busy := len(r.running)
				r.mu.Unlock()
				if busy == 0 {
					return time.Time{}, ctx.Err()
				}
				<-r.freed
			}
		}
	}
	// A result that could not be written is the pass's failure: its job is
	// still in the inbox.
	r.mu.Lock()
	failure = errors.Join(failure, r.failed)
	r.mu.Unlock()
	if failure == nil {
		failure = r.sweep(ctx)
	}
	return time.Time{}, failure
}

// merge gives one channel that is sent to when any of several is.
func merge(ctx context.Context, channels []<-chan struct{}) <-chan struct{} {
	out := make(chan struct{}, 1)
	for _, ch := range channels {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-ch:
					if !ok {
						return
					}
					select {
					case out <- struct{}{}:
					default:
					}
				}
			}
		}()
	}
	return out
}

// waiting is a message found in the runner's inbox.
type waiting struct {
	name  string
	base  string
	msg   *instance.Message
	value identity.Value
}

// look goes through the inbox in each controller's store and acts on what
// is there: jobs, and cancels of jobs, from a controller the runner trusts.
// It reads only what it may act on. A cancel is always read. A job is read
// when this copy has a place for it and no copy has marked it, which the
// names of the marks say: a copy among many, before a long inbox, reads a
// few jobs and not all of them.
func (r *Runner) look(ctx context.Context) error {
	for _, s := range r.Serves {
		names, err := s.Store.List(ctx, store.Inbox(r.ID.Name))
		if err != nil {
			return err
		}
		marks, err := s.Store.List(ctx, store.TakenList(r.ID.Name))
		if err != nil {
			return err
		}
		marked := map[string]bool{}
		for _, name := range marks {
			if job, _, ok := store.TakenOf(name); ok {
				marked[job] = true
			}
		}
		at := map[string]string{} // where each message is, by its own name
		for _, name := range names {
			at[path.Base(name)] = name
		}
		// A job whose cancel is already waiting is not started.
		for _, name := range names {
			if !strings.HasSuffix(name, cancelSuffix) {
				continue
			}
			w, err := r.open(ctx, s, name)
			if err != nil {
				return err
			}
			if w == nil || w.msg.Kind != instance.KindCancel {
				continue
			}
			target, waitingJobs := w.msg.Cancel, map[string]*waiting{}
			if job := at[target]; job != "" {
				r.mu.Lock()
				_, here := r.running[target]
				_, ran := r.finished[target]
				r.mu.Unlock()
				if marked[target] && !here && !ran {
					continue // another copy holds the job: the cancel is left for that copy
				}
				// A job nobody has taken yet is read, to be answered as cancelled.
				if !marked[target] {
					found, err := r.open(ctx, s, job)
					if err != nil {
						return err
					}
					if found != nil && found.msg.Kind == instance.KindJob {
						waitingJobs[target] = found
					}
					marked[target] = true
				}
			}
			if err := r.cancel(ctx, s, w, waitingJobs); err != nil {
				return err
			}
		}
		r.mu.Lock()
		finished := maps.Clone(r.finished)
		r.mu.Unlock()
		var free []string
		for _, name := range names {
			base := path.Base(name)
			written, ran := finished[base]
			switch {
			case written:
				// Its result is written; only the delete was missed.
				if _, err := s.Store.Delete(ctx, name, store.Overwrite); err != nil {
					return err
				}
			case ran || marked[base] || r.late[base] || strings.HasSuffix(name, cancelSuffix):
			default:
				free = append(free, name)
			}
		}
		// Copies that look at the same moment reach for different jobs.
		mrand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
		for _, name := range free {
			if r.full() {
				break
			}
			w, err := r.open(ctx, s, name)
			if err != nil {
				return err
			}
			if w == nil || w.msg.Kind != instance.KindJob {
				continue
			}
			later, err := r.take(ctx, s, w)
			if err != nil {
				return err
			}
			if later {
				// It will not fit later in this pass either: it is not read again.
				r.late[w.base] = true
			}
		}
	}
	return nil
}

// cancelSuffix ends the name of a job's cancel, which is the job's own
// name before it.
const cancelSuffix = ".cancel"

// full says whether the copy is running as many jobs as it may.
func (r *Runner) full() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running) >= max(r.Jobs, 1)
}

// open reads one message. It is acted on only if it is from a controller
// the runner trusts, signed by that controller, sealed to this runner and
// still good; anything else is deleted.
func (r *Runner) open(ctx context.Context, s Served, name string) (*waiting, error) {
	data, mk, err := s.Store.Read(ctx, name)
	if err != nil || mk == store.None {
		return nil, err
	}
	refuse := func() (*waiting, error) {
		_, err := s.Store.Delete(ctx, name, store.Overwrite)
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(name, store.Inbox(r.ID.Name)+"/"), "/")
	v, err := identity.Parse(data)
	if err != nil || len(parts) != 2 || parts[0] != s.Controller.Name || v.Check(s.Controller, name) != nil {
		return refuse()
	}
	var msg instance.Message
	if v.Open(r.ID, &msg) != nil || !v.Good(r.Now()) {
		return refuse()
	}
	switch {
	case msg.Kind == instance.KindJob && msg.Job != nil:
	case msg.Kind == instance.KindCancel && msg.Cancel != "":
	default:
		return refuse() // a runner acts only on jobs, and on cancels of jobs
	}
	return &waiting{name: name, base: parts[1], msg: &msg, value: v}, nil
}

// cancel acts on a cancel, and deletes it when it has: the pass that is
// running the job stops it, and one that finds the job waiting answers it
// as cancelled. A cancel for a job that another copy holds is left for
// that copy. A job stays in the inbox until it is answered, so a cancel
// whose job is not there has nothing left to stop, and goes: what a pass
// does with a cancel is read from the store, whoever finished the job.
func (r *Runner) cancel(ctx context.Context, s Served, w *waiting, jobs map[string]*waiting) error {
	target := w.msg.Cancel
	r.mu.Lock()
	job, isRunning := r.running[target]
	_, ran := r.finished[target]
	r.mu.Unlock()
	switch {
	case isRunning:
		job.stop(&machine.Error{Status: machine.StatusCancelled, Error: "the job was cancelled"})
	case ran:
	case jobs[target] != nil:
		// The job has not been started: mark it and answer it as cancelled.
		waitingJob := jobs[target]
		delete(jobs, target)
		mine, err := r.mark(ctx, s, target, waitingJob.msg.Job.Due)
		if err != nil || !mine {
			return err
		}
		job := waitingJob.msg.Job
		result := &instance.Result{Job: job.Name, Attempt: job.Attempt, Status: machine.StatusCancelled,
			Error: &machine.Error{Status: machine.StatusCancelled, Error: "the job was cancelled before it started"}}
		if _, err := r.answer(ctx, s, waitingJob, result); err != nil {
			return err
		}
	}
	_, err := s.Store.Delete(ctx, w.name, store.Overwrite)
	return err
}

// mark takes a job by writing its mark only-if-new, and says whether this
// copy is the one that took it.
func (r *Runner) mark(ctx context.Context, s Served, job string, due time.Time) (bool, error) {
	at := store.Taken(r.ID.Name, due, job)
	data, err := r.ID.Write(at, r.Now(), noEnd, mark{Copy: r.copy, Due: due})
	if err != nil {
		return false, err
	}
	written, err := s.Store.Write(ctx, at, data, store.OnlyIfNew)
	if written {
		return true, nil
	}
	// The mark is there, or the write's answer was lost: a pass finds out
	// whether the mark is its own before it leaves the job.
	existing, mk, rerr := s.Store.Read(ctx, at)
	if rerr != nil {
		return false, rerr
	}
	var m mark
	if mk != store.None {
		if _, perr := identity.Read(existing, at, r.ID.Card, nil, &m); perr == nil && m.Copy == r.copy {
			return true, nil
		}
	}
	if mk == store.None {
		return false, err
	}
	return false, nil
}

var noEnd time.Time

// take takes a job and starts it, if this copy has room and the job can
// still be finished.
func (r *Runner) take(ctx context.Context, s Served, w *waiting) (later bool, err error) {
	job := w.msg.Job
	r.mu.Lock()
	_, isRunning := r.running[w.base]
	_, ran := r.finished[w.base]
	r.mu.Unlock()
	if isRunning || ran || r.full() {
		return false, nil // a copy at its number of jobs writes no mark for the next
	}
	// It is taken only if its whole time limit fits in the time the host
	// has left.
	if deadline, ok := ctx.Deadline(); ok && r.Now().Add(time.Duration(job.Timeout)+hostMargin).After(deadline) {
		if time.Duration(job.Timeout)+hostMargin <= r.whole {
			return true, nil // a copy with more of its time left takes it
		}
		// No call on this host is long enough for it. It fails now and
		// says so: left alone, it would wait out its time and say nothing.
		mine, err := r.mark(ctx, s, w.base, job.Due)
		if err != nil || !mine {
			return false, err
		}
		_, err = r.answer(ctx, s, w, &instance.Result{Job: job.Name, Attempt: job.Attempt, Status: machine.StatusError,
			Error: &machine.Error{Status: machine.StatusError, Error: fmt.Sprintf(
				"the job may take %s, and this runner's host gives a call %s less the %s kept for writing the result: give the job a shorter timeout, or the host's calls more time",
				time.Duration(job.Timeout), r.whole.Round(time.Second), hostMargin)}})
		return false, err
	}
	if !r.Now().Before(job.Due) {
		return false, nil // overdue: it goes when it is no longer good
	}
	mine, err := r.mark(ctx, s, w.base, job.Due)
	if err != nil || !mine {
		return false, err
	}
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	run := &supervised{r: r, s: s, w: w, job: job, ctx: jobCtx}
	r.mu.Lock()
	r.running[w.base] = &running{stop: run.stop}
	r.mu.Unlock()
	go func() {
		defer cancel()
		written, err := r.answer(jobCtx, s, w, run.execute())
		r.mu.Lock()
		delete(r.running, w.base)
		r.finished[w.base] = written
		r.failed = errors.Join(r.failed, err)
		r.mu.Unlock()
		select {
		case r.freed <- struct{}{}:
		default:
		}
	}()
	return false, nil
}

// hostMargin is how long before its host's limit a pass must have what it
// has in hand finished.
const hostMargin = 10 * time.Second

// answer sends a job's result to the controller and, only when it is
// written, deletes the job from the inbox; it says whether the result was
// written. A result stays good while the instance it is for exists.
func (r *Runner) answer(ctx context.Context, s Served, w *waiting, result *instance.Result) (bool, error) {
	job := w.msg.Job
	send := func(result *instance.Result) (int, error) {
		name := store.Message(s.Controller.Name, r.ID.Name, r.sender.Next(r.Now()))
		msg := &instance.Message{Kind: instance.KindResult, Instance: job.Instance, Result: result}
		data, err := r.ID.Write(name, r.Now(), noEnd, msg, s.Controller)
		if err != nil || len(data) > maxResult {
			return len(data), err
		}
		return 0, add(ctx, s.Store, name, data)
	}
	size, err := send(result)
	if err == nil && size > 0 {
		// An output too large to send is sent with its long texts and
		// lists in blobs.
		sent := *result
		if sent.Output, err = r.blobbed(ctx, s, job.Instance, result.Output); err == nil {
			result = &sent
			size, err = send(result)
		}
	}
	if err == nil && size > 0 {
		// What is left too large to send is sent as an error that says so.
		_, err = send(&instance.Result{Job: job.Name, Attempt: job.Attempt, Status: machine.StatusError, Final: true, Used: result.Used,
			Error: &machine.Error{Status: machine.StatusError, Error: fmt.Sprintf(
				"the job's result is %d bytes, too large to send even with its long texts and lists in blobs; a list that holds blobs stays a list", size)}})
	}
	if err != nil {
		return false, err
	}
	_, err = s.Store.Delete(ctx, w.name, store.Overwrite)
	return true, err
}

// longText is the longest a text in an output too large to send stays
// where it is; a longer one is stored as a blob.
const longText = 64 << 10

// blobbed gives an output with its long values stored in blobs in the
// instance's record, from the inside out: each text longer than longText
// as a blob of text, and each list longer than that as JSON as a blob of
// JSON. A list that holds blobs, those just made among them, stays a list:
// a reference inside a blob is not copied with the blob, and the blob
// could outlive what it names.
func (r *Runner) blobbed(ctx context.Context, s Served, instanceID string, output any) (any, error) {
	write := func(typ string, contents []byte) (any, error) {
		ref, err := blob.Write(ctx, s.Store, store.Blob(s.Controller.Name, instanceID, blob.NewName()), typ, blob.NewKey(), bytes.NewReader(contents))
		return ref.Value(), err
	}
	var put func(v any) (any, error)
	put = func(v any) (any, error) {
		switch v := v.(type) {
		case string:
			if len(v) > longText {
				return write("text/plain", []byte(v))
			}
		case map[string]any:
			if _, isRef := blob.AsRef(v); isRef {
				return v, nil
			}
			out := make(map[string]any, len(v))
			for k, item := range v {
				var err error
				if out[k], err = put(item); err != nil {
					return nil, err
				}
			}
			return out, nil
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				var err error
				if out[i], err = put(item); err != nil {
					return nil, err
				}
			}
			if len(blob.Refs(out)) > 0 {
				return out, nil
			}
			contents, err := json.Marshal(out)
			if err != nil || len(contents) <= longText {
				return out, err
			}
			return write("application/json", contents)
		}
		return v, nil
	}
	return put(output)
}

// add writes a value under a name that is this job's alone, only-if-new. A
// write whose answer was lost is sent again, a few times: the name can hold
// nothing but this value, so finding it there means an earlier one arrived.
func add(ctx context.Context, s store.Store, name string, value []byte) error {
	var err error
	for range lostTries {
		if _, err = s.Write(ctx, name, value, store.OnlyIfNew); !errors.Is(err, store.ErrUnknown) {
			break
		}
	}
	return err
}

// lostTries is how many times a write whose answer keeps being lost is
// sent. A store that is being stopped and started can lose more than one
// answer in a row.
const lostTries = 5

// maxResult is the largest a result's message may be.
const maxResult = 256 << 10

// sweep removes each mark whose job's due time has passed, allowing for
// clocks that differ. A mark's name says when that is, so none is read.
func (r *Runner) sweep(ctx context.Context) error {
	now := r.Now()
	for _, s := range r.Serves {
		names, err := s.Store.List(ctx, store.TakenList(r.ID.Name))
		if err != nil {
			return err
		}
		for _, name := range names {
			if _, due, ok := store.TakenOf(name); ok && !now.After(due.Add(identity.ClockAllowance)) {
				continue
			}
			if _, err := s.Store.Delete(ctx, name, store.Overwrite); err != nil {
				return err
			}
		}
	}
	return nil
}

// ceiling is the runner's own layer for a controller's jobs.
func (r *Runner) ceiling(s Served) permission.Layer {
	layer := s.Ceiling
	layer.Name = "the ceiling of the runner " + strconv.Quote(r.ID.Name)
	layer.Fix = fmt.Sprintf("the runner's owner runs, on the runner's computer: corbie runner trust %s --allow %%s", s.Controller.Name)
	return layer
}

func sortedNames[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
