package role_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// The test binary stands in for the process a runner starts for a job, and
// for a process that makes one pass of a role and is then killed.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	if name := os.Getenv(passOf); name != "" {
		onePass(name)
		return
	}
	os.Exit(m.Run())
}

// A host stands in for a serverless host. It starts a copy for every call,
// gives each a disk of its own, freezes it once it has answered, kills it
// at its limit, and counts any copy that works on after answering.
//
// A copy shares nothing with another but the store, and reaches the store
// only through the host. That is how the host does to a copy what a real
// one does: a request from a copy that is frozen is held until the copy is
// thawed, and a request from a copy that has been killed is never made.
// The host's limit is counted in requests, so that where it falls is the
// test's to choose: when a copy's time is up its pass is told so, and a
// copy that begins more than a few requests after that is killed.
type host struct {
	t    *testing.T
	mem  *store.Memory
	base time.Time

	mu     sync.Mutex
	change *sync.Cond
	ahead  time.Duration // how far the clock has been moved on
	copies []*copyOf
	// each is told of every request a copy begins while it is in a call:
	// a test freezes or kills the copy there, or has its time be up.
	each func(c *copyOf, name string, write bool)
	// limit is how long the host says a call may run: fifteen minutes, as
	// a real host gives at the most, unless a test says otherwise.
	limit time.Duration
}

// grace is how many requests a copy may still begin once its time is up:
// what it has in hand. At one more it is killed.
const grace = 16

// A copyOf is one copy of a role on the host.
type copyOf struct {
	h     *host
	party string
	// disk is the copy's own: no other copy sees it.
	disk func() string

	stop func() // tells the pass its time is up

	// All of these are the host's, under its lock.
	answered bool // its pass has returned
	frozen   bool
	killed   bool
	timeUp   bool
	held     int // requests held while it is frozen
	late     int // requests begun after its time was up
	after    int // requests begun after it had answered
	writes   int // writes begun in its call

	next time.Time
	err  error
	done chan struct{} // closed when its pass has returned
}

var errKilled = fmt.Errorf("%w: the host has killed this copy", store.ErrUnreachable)

func newHost(t *testing.T) *host {
	h := &host{t: t, mem: store.NewMemory(), base: time.Now(), limit: 15 * time.Minute}
	h.change = sync.NewCond(&h.mu)
	t.Cleanup(func() {
		// When the test is over every copy is killed, and none is left
		// behind in the test's process.
		h.mu.Lock()
		for _, c := range h.copies {
			c.kill()
		}
		copies := h.copies
		h.mu.Unlock()
		for _, c := range copies {
			<-c.done
		}
	})
	return h
}

// clock is the host's clock, which a test moves by hand.
func (h *host) clock() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.base.Add(h.ahead)
}

func (h *host) tick(d time.Duration) {
	h.mu.Lock()
	h.ahead += d
	h.mu.Unlock()
}

// call starts a copy and has it make one pass: the role is made anew for
// the copy, from nothing but what the copy has. It returns when the copy
// has answered, has been killed, or has been frozen in the middle of its
// pass.
func (h *host) call(party string, made func(c *copyOf) role.Pass) *copyOf {
	h.t.Helper()
	c := &copyOf{h: h, party: party, done: make(chan struct{})}
	c.disk = sync.OnceValue(h.t.TempDir) // made when the copy first uses it
	ctx, stop := context.WithCancel(context.Background())
	c.stop = stop
	h.mu.Lock()
	h.copies = append(h.copies, c)
	h.mu.Unlock()
	pass := made(c)
	go func() {
		next, err := role.Once(ctx, time.Now().Add(h.limit), role.HostMargin, pass)
		stop()
		h.mu.Lock()
		c.next, c.err = next, err
		// A copy that has answered is frozen until it is called again, and
		// this host calls no copy twice.
		c.answered, c.frozen = true, true
		h.change.Broadcast()
		h.mu.Unlock()
		close(c.done)
	}()
	c.wait()
	return c
}

// wait waits until the copy has answered or been killed, or is frozen with
// a request held.
func (c *copyOf) wait() {
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	for !c.answered && !(c.frozen && c.held > 0) {
		c.h.change.Wait()
	}
}

// ended says whether the copy made its pass to the end: it answered, and
// its pass did not end early.
func (c *copyOf) ended() bool {
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	return c.answered && !c.killed && c.err == nil
}

// freeze freezes the copy where it is. Call it from the host's each.
func (c *copyOf) freeze() { c.frozen = true }

// kill kills the copy: nothing it has not yet done is done. Call it from
// the host's each, or with the host's lock held.
func (c *copyOf) kill() {
	c.killed = true
	c.stop()
	c.h.change.Broadcast()
}

// up has the copy's time be up: its pass is told, and the copy is killed
// if it goes on to begin more than what it had in hand. Call it from the
// host's each.
func (c *copyOf) up() {
	c.timeUp = true
	c.stop()
}

// thaw lets a frozen copy go on from where it was.
func (c *copyOf) thaw() {
	c.h.mu.Lock()
	c.frozen = false
	c.h.change.Broadcast()
	c.h.mu.Unlock()
}

// begin is the copy beginning a request of the store. It says whether the
// request is to be made.
func (c *copyOf) begin(name string, write bool) error {
	h := c.h
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case c.killed:
		return errKilled
	case c.answered:
		c.after++ // it works on after answering
	default:
		if c.timeUp {
			if c.late++; c.late > grace {
				c.kill()
				return errKilled
			}
		}
		if write {
			c.writes++
		}
		if h.each != nil {
			h.each(c, name, write)
		}
	}
	if c.frozen && !c.killed {
		c.held++
		h.change.Broadcast()
		for c.frozen && !c.killed {
			h.change.Wait()
		}
		c.held--
	}
	if c.killed {
		return errKilled
	}
	return nil
}

// worked is how many copies went on working after they had answered.
func (h *host) worked() (n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.copies {
		if c.after > 0 {
			n++
		}
	}
	return n
}

// killed is how many copies the host has killed.
func (h *host) killed() (n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.copies {
		if c.killed {
			n++
		}
	}
	return n
}

// store is the copy's way to the store, as a party.
func (c *copyOf) store(kind store.Kind) store.Store {
	return link{c, c.h.mem.As(store.Party{Kind: kind, Name: c.party, Controller: "ctl"})}
}

// A link is a copy's way to the store, through its host. The store itself
// answers for as long as the copy is alive: that its time is up is the
// pass's to notice.
type link struct {
	c     *copyOf
	inner store.Store
}

func (l link) Read(ctx context.Context, name string) ([]byte, store.Mark, error) {
	if err := l.c.begin(name, false); err != nil {
		return nil, store.None, err
	}
	return l.inner.Read(context.WithoutCancel(ctx), name)
}

func (l link) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := l.c.begin(name, false); err != nil {
		return nil, err
	}
	return l.inner.ReadPart(context.WithoutCancel(ctx), name, from, length)
}

func (l link) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	if err := l.c.begin(name, true); err != nil {
		return false, err
	}
	return l.inner.Write(context.WithoutCancel(ctx), name, value, how)
}

func (l link) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := l.c.begin(name, true); err != nil {
		return false, err
	}
	return l.inner.WriteFrom(context.WithoutCancel(ctx), name, r)
}

func (l link) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := l.c.begin(to, true); err != nil {
		return false, err
	}
	return l.inner.Copy(context.WithoutCancel(ctx), from, to)
}

func (l link) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	if err := l.c.begin(name, true); err != nil {
		return false, err
	}
	return l.inner.Delete(context.WithoutCancel(ctx), name, how)
}

func (l link) List(ctx context.Context, under string) ([]string, error) {
	if err := l.c.begin(under, false); err != nil {
		return nil, err
	}
	return l.inner.List(context.WithoutCancel(ctx), under)
}

func (l link) First(ctx context.Context, under string, n int) ([]string, error) {
	if err := l.c.begin(under, false); err != nil {
		return nil, err
	}
	return l.inner.First(context.WithoutCancel(ctx), under, n)
}

// A bucket does not tell a copy what another has written.
func (l link) Notice(string) (<-chan struct{}, func()) { return make(chan struct{}), func() {} }

// nth gives an each that acts on the copy of one party that begins that
// party's n-th write, counted over every copy since it was set.
func nth(party string, n int, act func(c *copyOf)) func(c *copyOf, name string, write bool) {
	seen := 0
	return func(c *copyOf, _ string, write bool) {
		if c.party != party || !write {
			return
		}
		if seen++; seen == n {
			act(c)
		}
	}
}
