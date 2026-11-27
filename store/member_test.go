package store

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// members is a served directory with members, each in this process with a
// copy of its own and a server on a port of this computer.
type members struct {
	t      *testing.T
	keeper ed25519.PrivateKey
	all    []*testMember

	mu      sync.Mutex
	parties map[string]servedParty
	cut     map[string]bool // the members cut off from the others
}

type testMember struct {
	address, dir string
	listener     net.Listener
	member       *Member
	listening    *http.Server
	failed       chan error
}

// quick has a party give up on a store out of reach sooner, for a test.
// The members' clocks run fast in every test of this package.
func quick(t *testing.T) {
	patience := servedPatience
	servedPatience = 4 * time.Second
	t.Cleanup(func() { servedPatience = patience })
}

func newMembers(t *testing.T, n int) *members {
	t.Helper()
	quick(t)
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	c := &members{t: t, keeper: keeper, parties: map[string]servedParty{}, cut: map[string]bool{}}
	cut := func(from, to string) bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.cut[from] || c.cut[to]
	}
	memberCut.Store(&cut)
	for range n {
		c.add()
	}
	return c
}

// add starts another member: the first alone, and each one after by asking
// the first to take it.
func (c *members) add() *testMember {
	c.t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		c.t.Fatal(err)
	}
	tm := &testMember{address: l.Addr().String(), dir: c.t.TempDir(), listener: l, failed: make(chan error, 1)}
	if len(c.all) > 0 {
		if err := SetMembers(tm.dir, []string{tm.address, c.all[0].address}); err != nil {
			c.t.Fatal(err)
		}
	}
	c.all = append(c.all, tm)
	c.start(tm)
	if len(c.all) > 1 {
		c.eventually("the member to be counted", func() bool {
			voters, _, _, _ := tm.member.Members()
			return slices.Contains(voters, tm.address)
		})
	}
	return tm
}

// start opens a member's copy and serves it where the member listens.
func (c *members) start(tm *testMember) {
	c.t.Helper()
	if tm.listener == nil {
		l, err := net.Listen("tcp", tm.address)
		if err != nil {
			c.t.Fatal(err)
		}
		tm.listener = l
	}
	m, err := OpenMember(tm.dir, tm.address, proofOf(c.t, c.keeper))
	if err != nil {
		c.t.Fatal(err)
	}
	m.Failed = func(err error) {
		select {
		case tm.failed <- err:
		default:
		}
	}
	tm.member = m
	server := &Server{Directory: m.Directory(), Member: m,
		Who: func(_ context.Context, name string) (Party, ed25519.PublicKey, error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			p, ok := c.parties[name]
			if !ok {
				return Party{}, nil, nil
			}
			return p.Party, keyOf(p.key), nil
		}}
	tm.listening = server.HTTP()
	go tm.listening.Serve(ServedListener(tm.listener, proofOf(c.t, c.keeper)))
	if err := m.Start(); err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { c.stop(tm) })
}

// stop ends a member's process, as far as a test can.
func (c *members) stop(tm *testMember) {
	if tm.listening == nil {
		return
	}
	tm.listening.Close()
	tm.member.Stop()
	tm.listening, tm.listener = nil, nil
}

func (c *members) address(of ...*testMember) string {
	if len(of) == 0 {
		of = c.all
	}
	var hosts []string
	for _, tm := range of {
		hosts = append(hosts, tm.address)
	}
	return ServedScheme + strings.Join(hosts, ",")
}

// as opens the store as a party, at the members named or at all of them.
func (c *members) as(p Party, of ...*testMember) Store {
	c.t.Helper()
	c.mu.Lock()
	held, ok := c.parties[p.Name]
	if !ok {
		_, key, _ := ed25519.GenerateKey(rand.Reader)
		held = servedParty{p, key}
		c.parties[p.Name] = held
	}
	c.mu.Unlock()
	s, err := OpenServed(c.address(of...), p.Name, proofOf(c.t, held.key), keyOf(c.keeper))
	if err != nil {
		c.t.Fatal(err)
	}
	return s
}

func (c *members) owner(of ...*testMember) Store {
	return c.as(Party{Kind: Owner, Name: "owner"}, of...)
}

// leader gives the member that leads.
func (c *members) leader() *testMember {
	c.t.Helper()
	var lead *testMember
	c.eventually("a member to lead", func() bool {
		for _, tm := range c.all {
			if tm.listening == nil {
				continue
			}
			if s, err := tm.member.store(); err == nil {
				if _, leads := s.(leader); leads {
					lead = tm
					return true
				}
			}
		}
		return false
	})
	return lead
}

func (c *members) eventually(what string, ok func() bool) {
	c.t.Helper()
	for began := time.Now(); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Since(began) > 20*time.Second {
			c.t.Fatalf("waited 20s for %s", what)
		}
	}
}

// holds gives the values of a copy by name, as a person reads them.
func holds(t *testing.T, dir string) map[string]string {
	t.Helper()
	values := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil // gone while it was read
		case d.IsDir() && path != dir && strings.HasPrefix(d.Name(), "."):
			return fs.SkipDir
		case d.IsDir() || strings.HasPrefix(d.Name(), tmpPrefix):
			return nil
		}
		value, err := os.ReadFile(path)
		if err == nil {
			rel, _ := filepath.Rel(dir, path)
			values[filepath.ToSlash(rel)] = string(value)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return values
}

// agree waits until the copies of the members named, or of all, hold the
// same, and gives what they hold.
func (c *members) agree(of ...*testMember) map[string]string {
	c.t.Helper()
	if len(of) == 0 {
		of = c.all
	}
	var first map[string]string
	c.eventually("the copies to hold the same", func() bool {
		first = holds(c.t, of[0].dir)
		for _, tm := range of[1:] {
			if !maps.Equal(first, holds(c.t, tm.dir)) {
				return false
			}
		}
		return true
	})
	return first
}

// did fails the test over a change that was not made, with why each
// member that stopped did.
func (c *members) did(done bool, err error) {
	c.t.Helper()
	if err == nil && done {
		return
	}
	for _, tm := range c.all {
		select {
		case why := <-tm.failed:
			c.t.Errorf("%s stopped: %v", tm.address, why)
		default:
		}
	}
	c.t.Fatalf("not done: %v", err)
}

// did fails the test over a change that was not made.
func did(t *testing.T) func(bool, error) {
	return func(done bool, err error) {
		t.Helper()
		if err != nil || !done {
			t.Fatalf("not done: %v", err)
		}
	}
}

// A directory with one member is given two more: each copies what it held,
// and from then on every copy holds what a party wrote, by whichever member
// it asked.
func Test_store_members_joiner_copies_the_directory(t *testing.T) {
	c := newMembers(t, 1)
	alone := c.owner()
	large := bytes.Repeat([]byte("corbie "), 300_000)
	did(t)(alone.Write(ctx, "ctl/running/c1/i1", []byte("head"), Overwrite))
	did(t)(alone.Write(ctx, "ctl/records/c1/i1/blobs/b", large, OnlyIfNew))
	if HasMembers(c.all[0].dir) {
		t.Fatal("a directory with one member has a log")
	}

	c.add()
	c.add()
	for _, tm := range c.all {
		c.eventually(tm.address+" to count three members", func() bool {
			voters, joining, _, many := tm.member.Members()
			return many && len(voters) == 3 && len(joining) == 0
		})
		if listed, _ := MembersOf(tm.dir); len(listed) != 3 {
			t.Fatalf("%s lists the members %v for its own computer's processes", tm.address, listed)
		}
	}
	s := c.owner()
	did(t)(s.Write(ctx, "ctl/running/c1/i1", []byte("head 2"), Overwrite))
	did(t)(s.Copy(ctx, "ctl/records/c1/i1/blobs/b", "ctl/receipts/c1/m/blobs/b"))
	did(t)(s.Delete(ctx, "ctl/records/c1/i1/blobs/b", Overwrite))
	got := c.agree()
	if got["ctl/running/c1/i1"] != "head 2" || got["ctl/receipts/c1/m/blobs/b"] != string(large) || len(got) != 2 {
		t.Fatalf("the copies hold %d values, the head %q", len(got), got["ctl/running/c1/i1"])
	}

	// Only a member's server changes a copy.
	plain, err := OpenDirectory(c.all[0].dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Write(ctx, "ctl/running/c1/i2", []byte("x"), Overwrite); !errors.Is(err, errMembers) {
		t.Fatalf("a process wrote a member's copy past its server: %v", err)
	}
}

// With three members the store loses one and carries on, the leader
// included, and the one that comes back holds what the others do.
func Test_store_members_carry_on_without_one(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/one", []byte("1"), OnlyIfNew))

	lead := c.leader()
	c.stop(lead)
	// A member that has not yet started answers no party from its copy.
	notYet, err := OpenMember(lead.dir, lead.address, proofOf(t, c.keeper))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notYet.store(); !errors.As(err, new(*notLeader)) {
		t.Fatalf("a member that has not started answers a party: %v", err)
	}
	did(t)(s.Write(ctx, "a/two", []byte("2"), OnlyIfNew))
	if v, _ := read(t, s, "a/one"); v != "1" {
		t.Fatalf("a value answered before the leader stopped reads %q", v)
	}
	if done, err := s.Write(ctx, "a/one", []byte("again"), OnlyIfNew); done || err != nil {
		t.Fatalf("a name written before the leader stopped was written only-if-new again: %v %v", done, err)
	}

	c.start(lead)
	did(t)(s.Write(ctx, "a/three", []byte("3"), OnlyIfNew))
	if got := c.agree(); len(got) != 3 || got["a/two"] != "2" {
		t.Fatalf("the copies hold %v", got)
	}
}

// A member that cannot reach a majority answers nothing: a party that asks
// it is told the store cannot be reached, and never that a name is free.
// The others carry on, and of two writes only-if-new to one name, one on
// each side, one is done.
func Test_store_members_minority_answers_nothing(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/one", []byte("1"), OnlyIfNew))

	old := c.leader()
	c.mu.Lock()
	c.cut[old.address] = true
	c.mu.Unlock()

	cutOff := c.owner(old)
	// It does not yet know that it is cut off, and a name its copy holds
	// may be free by now.
	if done, err := cutOff.Write(ctx, "a/one", []byte("again"), OnlyIfNew); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a member cut off from the others answered that a name is taken: %v %v", done, err)
	}
	_, err := cutOff.Write(ctx, "a/free", []byte("minority"), OnlyIfNew)
	if !errors.Is(err, ErrUnreachable) && !errors.Is(err, ErrUnknown) {
		t.Fatalf("a member cut off from the others answered a write: %v", err)
	}
	if _, _, err := cutOff.Read(ctx, "a/one"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a member cut off from the others answered a read: %v", err)
	}

	var rest []*testMember
	for _, tm := range c.all {
		if tm != old {
			rest = append(rest, tm)
		}
	}
	majority := c.owner(rest...)
	did(t)(majority.Write(ctx, "a/free", []byte("majority"), OnlyIfNew))

	c.mu.Lock()
	c.cut[old.address] = false
	c.mu.Unlock()
	if got := c.agree(); got["a/free"] != "majority" || len(got) != 2 {
		t.Fatalf("the copies hold %v", got)
	}
}

// A member that was off while the others cut their log copies the directory
// again, and then holds what they hold.
func Test_store_members_follow_by_copying_when_the_log_is_cut(t *testing.T) {
	trail, every := memberTrail, memberRecord
	memberTrail, memberRecord = 2, 10*time.Millisecond
	t.Cleanup(func() { memberTrail, memberRecord = trail, every })
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/kept", []byte("kept"), OnlyIfNew))
	did(t)(s.Write(ctx, "a/goes", []byte("goes"), OnlyIfNew))
	c.agree()

	var off *testMember
	lead := c.leader()
	for _, tm := range c.all {
		if tm != lead {
			off = tm
		}
	}
	c.stop(off)
	for i := range 40 {
		did(t)(s.Write(ctx, fmt.Sprintf("b/%02d", i), []byte("v"), OnlyIfNew))
		did(t)(s.Write(ctx, "a/head", fmt.Append(nil, i), Overwrite))
	}
	did(t)(s.Delete(ctx, "a/goes", Overwrite))
	time.Sleep(50 * time.Millisecond)

	c.start(off)
	got := c.agree()
	if _, there := got["a/goes"]; there || got["a/head"] != "39" || got["a/kept"] != "kept" || len(got) != 42 {
		t.Fatalf("the copies hold %d values, the head %q", len(got), got["a/head"])
	}
}

// Every member stops at once, with entries applied and not yet written down
// as applied, as when the power goes. Started again, each applies those
// entries a second time and holds what it held.
func Test_store_members_apply_twice_and_hold_the_same(t *testing.T) {
	every := memberRecord
	memberRecord = time.Hour
	t.Cleanup(func() { memberRecord = every })
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/x", []byte("one"), Overwrite))
	did(t)(s.Write(ctx, "a/x", []byte("two"), Overwrite))
	did(t)(s.Write(ctx, "a/blob", []byte("blob"), OnlyIfNew))
	did(t)(s.Copy(ctx, "a/blob", "a/copy"))
	did(t)(s.Delete(ctx, "a/blob", Overwrite))
	did(t)(s.Write(ctx, "a/gone", []byte("gone"), OnlyIfNew))
	did(t)(s.Delete(ctx, "a/gone", Overwrite))
	want := c.agree()

	memberStopRecords = false
	for _, tm := range c.all {
		c.stop(tm)
	}
	memberStopRecords = true
	for _, tm := range c.all {
		c.start(tm)
	}
	did(t)(s.Write(ctx, "a/after", []byte("after"), OnlyIfNew))
	want["a/after"] = "after"
	if got := c.agree(); !maps.Equal(got, want) || got["a/x"] != "two" || got["a/copy"] != "blob" {
		t.Fatalf("the copies hold %v, and held %v", got, want)
	}
}

// A member that is removed is turned away by the others, which carry on,
// and it stops, saying that it was removed.
func Test_store_members_removed_member_is_turned_away(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/one", []byte("1"), OnlyIfNew))
	lead := c.leader()
	var gone, stays *testMember
	for _, tm := range c.all {
		if tm != lead {
			gone, stays = stays, tm
		}
	}
	if err := stays.member.Remove(ctx, gone.address); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-gone.failed:
		if !strings.Contains(err.Error(), "removed") {
			t.Fatalf("the member removed stopped over %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the member removed went on")
	}
	did(t)(s.Write(ctx, "a/two", []byte("2"), OnlyIfNew))
	if voters, _, _, _ := lead.member.Members(); len(voters) != 2 {
		t.Fatalf("the members are %v", voters)
	}
}

// A copy made the whole store has no log, and is a directory with one
// member again.
func Test_store_members_alone_is_a_directory_again(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/one", []byte("1"), OnlyIfNew))
	c.agree()
	for _, tm := range c.all {
		c.stop(tm)
	}
	last := c.all[2]
	if err := LeaveMembers(last.dir); err != nil {
		t.Fatal(err)
	}
	if HasMembers(last.dir) {
		t.Fatal("the copy has members still")
	}
	plain, err := OpenDirectory(last.dir)
	if err != nil {
		t.Fatal(err)
	}
	did(t)(plain.Write(ctx, "a/two", []byte("2"), OnlyIfNew))
	if v, _ := read(t, plain, "a/one"); v != "1" {
		t.Fatalf("the copy made the whole store reads %q", v)
	}
}

// A party asks any member: one that does not lead says who does, and the
// party asks that one.
func Test_store_members_any_member_is_asked(t *testing.T) {
	c := newMembers(t, 3)
	lead := c.leader()
	for i, tm := range c.all {
		if tm == lead {
			continue
		}
		s := c.owner(tm)
		name := fmt.Sprint(i)
		c.did(s.Write(ctx, "a/"+name, []byte("asked of one that does not lead"), OnlyIfNew))
		if v, _ := read(t, s, "a/"+name); v != "asked of one that does not lead" {
			t.Fatalf("a value written by way of %s reads %q", tm.address, v)
		}
		// A value that is read as it is sent reaches the leader whole.
		large := strings.Repeat("read once ", 100_000)
		c.did(c.owner(tm).WriteFrom(ctx, "b/"+name, strings.NewReader(large)))
		if v, _ := read(t, s, "b/"+name); v != large {
			t.Fatalf("a value sent as it was read by way of %s holds %d bytes of %d", tm.address, len(v), len(large))
		}
	}
	if got := c.agree(); len(got) != 4 {
		t.Fatalf("the copies hold %d values", len(got))
	}
}

// answering is a computer that proves the key of a store's controller and
// answers a party as a test has it answer.
func answering(t *testing.T, answer http.HandlerFunc) Store {
	t.Helper()
	quick(t)
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: answer}
	go server.Serve(ServedListener(l, proofOf(t, keeper)))
	t.Cleanup(func() { server.Close() })
	s, err := OpenServed(ServedScheme+l.Addr().String(), "party", proofOf(t, key), keyOf(keeper))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A member that has read a value and then finds that it does not lead has
// done nothing, and the party cannot send the value again: it reports that
// the store was not reached, and never sends an empty value in its place.
func Test_store_members_a_value_read_once_is_not_sent_again(t *testing.T) {
	var asked atomic.Int32
	s := answering(t, func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusMisdirectedRequest)
	})
	done, err := s.WriteFrom(ctx, "a/blob", strings.NewReader("a value read as it is sent"))
	if done || !errors.Is(err, ErrUnreachable) || asked.Load() != 1 {
		t.Fatalf("a value a member read and did not write: done %v, %v, asked %d times", done, err, asked.Load())
	}
}

// A change in the log of a leader that stops leading is the next leader's
// to apply or drop. A party sends it to the next leader where twice is the
// same as once, and otherwise reports that the outcome is not known.
func Test_store_members_a_change_in_flight_goes_to_the_next_leader(t *testing.T) {
	var asked atomic.Int32
	s := answering(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if asked.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set(headDone, "true")
	})
	did(t)(s.Write(ctx, "a/head", []byte("head"), Overwrite))
	if asked.Load() != 2 {
		t.Fatalf("an overwrite was sent %d times", asked.Load())
	}
	asked.Store(0)
	if done, err := s.Write(ctx, "a/new", []byte("new"), OnlyIfNew); done || !errors.Is(err, ErrUnknown) || asked.Load() != 1 {
		t.Fatalf("a write only-if-new: done %v, %v, sent %d times", done, err, asked.Load())
	}
}

// A change to a name that can hold no value, because it is under a value
// or names are under it, does nothing on any member and stops none.
func Test_store_members_a_name_with_no_room_stops_no_member(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/x", []byte("value"), OnlyIfNew))
	if done, err := s.Write(ctx, "a/x/y", []byte("under a value"), OnlyIfNew); done || err == nil {
		t.Fatalf("a write under a value: %v %v", done, err)
	}
	if done, err := s.Copy(ctx, "a/x", "a/x/z"); done || err == nil {
		t.Fatalf("a copy to a name under a value: %v %v", done, err)
	}
	if done, err := s.Delete(ctx, "a", Overwrite); done || err == nil {
		t.Fatalf("a delete of a name with names under it: %v %v", done, err)
	}
	did(t)(s.Write(ctx, "a/after", []byte("after"), OnlyIfNew))
	if got := c.agree(); len(got) != 2 || got["a/x"] != "value" {
		t.Fatalf("the copies hold %v", got)
	}
	for _, tm := range c.all {
		select {
		case err := <-tm.failed:
			t.Fatalf("%s stopped: %v", tm.address, err)
		default:
		}
	}
}

// A name is held from the moment the leader puts an entry for it in the
// log until the entry is applied, also where whoever asked has gone: of two
// copies to one name, the second finds the name taken.
func Test_store_members_a_name_is_held_until_its_entry_is_applied(t *testing.T) {
	c := newMembers(t, 3)
	lead := c.leader()
	served, err := lead.member.store()
	if err != nil {
		t.Fatal(err)
	}
	did(t)(served.Write(ctx, "a/x", []byte("x"), OnlyIfNew))
	did(t)(served.Write(ctx, "a/y", []byte("y"), OnlyIfNew))
	term := func() uint64 {
		lead.member.mu.Lock()
		defer lead.member.mu.Unlock()
		return lead.member.term
	}
	led := term()

	// For a moment nothing the leader sends arrives, so its entries wait.
	var cut atomic.Bool
	cut.Store(true)
	silent := func(from, to string) bool { return cut.Load() && from == lead.address }
	memberCut.Store(&silent)

	type answer struct {
		done bool
		err  error
	}
	first, second := make(chan answer, 1), make(chan answer, 1)
	asking, gone := context.WithCancel(ctx)
	go func() {
		done, err := served.Copy(asking, "a/x", "a/n")
		first <- answer{done, err}
	}()
	c.eventually("the first copy to be in the log", func() bool {
		lead.member.mu.Lock()
		defer lead.member.mu.Unlock()
		return len(lead.member.waiting) == 1
	})
	gone()
	go func() {
		done, err := served.Copy(ctx, "a/y", "a/n")
		second <- answer{done, err}
	}()
	time.Sleep(3 * memberTick)
	cut.Store(false)
	one, two := <-first, <-second
	if term() != led {
		t.Skip("the leader changed while it was cut off")
	}
	if !one.done || two.done || two.err != nil {
		t.Fatalf("the first copy: %v %v; the second: %v %v", one.done, one.err, two.done, two.err)
	}
	if got := c.agree(); got["a/n"] != "x" {
		t.Fatalf("the name holds %q", got["a/n"])
	}
}

// A party on the computer that holds the directory uses it as it is while
// it has one member, and through the members' servers from the moment a
// second joins, with nothing set up again.
func Test_store_members_a_party_on_a_member_uses_its_server(t *testing.T) {
	c := newMembers(t, 1)
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	c.mu.Lock()
	c.parties["here"] = servedParty{Party{Kind: Owner, Name: "here"}, key}
	c.mu.Unlock()
	here, err := OpenLocal(c.all[0].dir, c.all[0].address, "here", proofOf(t, key), keyOf(c.keeper))
	if err != nil {
		t.Fatal(err)
	}
	did(t)(here.Write(ctx, "a/alone", []byte("as it is"), OnlyIfNew))
	if HasMembers(c.all[0].dir) {
		t.Fatal("a directory with one member has a log")
	}

	c.add()
	c.add()
	did(t)(here.Write(ctx, "a/members", []byte("through a server"), OnlyIfNew))
	if done, err := here.Write(ctx, "a/alone", []byte("again"), OnlyIfNew); done || err != nil {
		t.Fatalf("a name written before the directory had members was written only-if-new again: %v %v", done, err)
	}
	if v, _ := read(t, here, "a/members"); v != "through a server" {
		t.Fatalf("the party reads %q", v)
	}
	if got := c.agree(); got["a/members"] != "through a server" || got["a/alone"] != "as it is" {
		t.Fatalf("the copies hold %v", got)
	}

	// A party on a member asks that member first.
	for _, tm := range c.all {
		there, err := OpenLocal(tm.dir, tm.address, "here", proofOf(t, key), keyOf(c.keeper))
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := read(t, there, "a/members"); v != "through a server" {
			t.Fatalf("the party on %s reads %q", tm.address, v)
		}
		if first := there.(*local).served.(*served).origins[0]; first != "https://"+tm.address {
			t.Fatalf("the party on %s asks %s first", tm.address, first)
		}
	}
}

// leads says whether a member answers parties as the leader.
func (tm *testMember) leads() bool {
	s, err := tm.member.store()
	_, leads := s.(leader)
	return err == nil && leads
}

// A member that is given a value and cannot put it under the name has not
// applied the entry: it stops, and applies the entry again at its next
// start.
func Test_store_members_a_value_not_put_under_its_name_is_not_applied(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/x", []byte("value"), OnlyIfNew))
	c.agree()
	_, mark, err := s.Read(ctx, "a/x")
	if err != nil {
		t.Fatal(err)
	}
	lead := c.leader()
	for _, tm := range c.all {
		if tm == lead {
			continue
		}
		// This copy has names where the value goes, and cannot take it.
		at := filepath.Join(tm.dir, "a", "x")
		if err := errors.Join(os.Remove(at), os.MkdirAll(at, 0o700), os.WriteFile(filepath.Join(at, "y"), []byte("in the way"), 0o600)); err != nil {
			t.Fatal(err)
		}
		if err := tm.member.pull(change{Op: "put", Name: "a/x", ID: newID(), Mark: mark}, 0); err == nil {
			t.Fatal("a value that was not put under its name counted as applied")
		}
		return
	}
}

// The leader of two members is removed. Its word that the entry is in the
// log for good takes a moment to arrive, and it stays that long: the other
// member leads alone, and answers a party.
func Test_store_members_the_leader_is_removed(t *testing.T) {
	c := newMembers(t, 2)
	s := c.owner()
	did(t)(s.Write(ctx, "a/one", []byte("1"), OnlyIfNew))
	lead := c.leader()
	stays := c.all[0]
	if stays == lead {
		stays = c.all[1]
	}
	far := func(from, to string) bool {
		if from == lead.address {
			time.Sleep(30 * time.Millisecond)
		}
		return false
	}
	memberCut.Store(&far)
	if err := stays.member.Remove(ctx, lead.address); err != nil {
		t.Fatal(err)
	}
	c.eventually("the member that stays to lead", stays.leads)
	did(t)(c.owner(stays).Write(ctx, "a/two", []byte("2"), OnlyIfNew))
	select {
	case err := <-lead.failed:
		if !strings.Contains(err.Error(), "removed") {
			t.Fatalf("the member removed stopped over %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the member removed went on")
	}
}

// A change whose condition was looked at in one term is not made in
// another: a member that led, did not, and leads again does not know what
// the leader between did to the name.
func Test_store_members_a_change_let_in_in_one_term_is_not_made_in_another(t *testing.T) {
	c := newMembers(t, 3)
	first := c.leader()
	before, err := first.member.store()
	if err != nil {
		t.Fatal(err)
	}
	other := c.all[0]
	if other == first {
		other = c.all[1]
	}
	hand := func(from, to *testMember) {
		t.Helper()
		if err := from.member.onLoop(ctx, func() error { from.member.rn.TransferLeader(to.member.id); return nil }); err != nil {
			t.Fatal(err)
		}
		c.eventually("the member handed the lead to lead", to.leads)
	}
	hand(first, other)
	hand(other, first)
	var not *notLeader
	if done, err := before.Write(ctx, "a/late", []byte("of a term past"), OnlyIfNew); done || !errors.As(err, &not) {
		t.Fatalf("a write let in two terms ago: done %v, %v", done, err)
	}
	now, err := first.member.store()
	if err != nil {
		t.Fatal(err)
	}
	did(t)(now.Write(ctx, "a/late", []byte("of this term"), OnlyIfNew))
	if got := c.agree(); got["a/late"] != "of this term" {
		t.Fatalf("the copies hold %v", got)
	}
}

// A member answers that a name holds nothing only where nothing is there:
// the member that asks deletes the name from its own copy. A file that is
// there and cannot be opened is another answer.
func Test_store_members_a_file_not_opened_is_not_answered_as_none(t *testing.T) {
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	m, err := OpenMember(dir, "127.0.0.1:1", proofOf(t, keeper))
	if err != nil {
		t.Fatal(err)
	}
	loop := filepath.Join(dir, "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Skipf("no symbolic link to make a file that cannot be opened: %v", err)
	}
	for path, want := range map[string]int{
		loop:                         http.StatusInternalServerError,
		filepath.Join(dir, "none"):   http.StatusNotFound,
		filepath.Join(dir, ".."):     http.StatusNotFound,
		filepath.Join(loop, "under"): http.StatusInternalServerError,
	} {
		w := httptest.NewRecorder()
		m.sendFile(w, path)
		if w.Code != want {
			t.Errorf("%s was answered %d, and not %d", path, w.Code, want)
		}
	}
}

// A computer that stopped as the first other computer joined, before
// anything was in the log, starts as the store's one member.
func Test_store_members_a_first_join_cut_short_leaves_one_member(t *testing.T) {
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	plain, err := OpenDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	did(t)(plain.Write(ctx, "a/one", []byte("1"), OnlyIfNew))
	const self = "127.0.0.1:1"
	if err := SetMembers(dir, []string{self}); err != nil {
		t.Fatal(err)
	}
	m, err := OpenMember(dir, self, proofOf(t, keeper))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	s, err := m.store()
	if err != nil || HasMembers(dir) || exists(filepath.Join(dir, membersDir)) {
		t.Fatalf("a first join cut short left %v, and a log: %v", err, exists(filepath.Join(dir, membersDir)))
	}
	if v, _ := read(t, s, "a/one"); v != "1" {
		t.Fatalf("the store's one member reads %q", v)
	}
	did(t)(plain.Write(ctx, "a/two", []byte("2"), OnlyIfNew))
}

// A computer that cannot join says why: the members it asks are not there,
// or are another store's.
func Test_store_members_a_computer_that_cannot_join_says_why(t *testing.T) {
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	nobody := l.Addr().String()
	l.Close()
	dir := t.TempDir()
	const self = "127.0.0.1:1"
	if err := SetMembers(dir, []string{self + " joining", nobody}); err != nil {
		t.Fatal(err)
	}
	m, err := OpenMember(dir, self, proofOf(t, keeper))
	if err != nil {
		t.Fatal(err)
	}
	why := make(chan error, 1)
	m.Held = func(err error) {
		select {
		case why <- err:
		default:
		}
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	select {
	case err := <-why:
		if !strings.Contains(err.Error(), "the member at "+nobody) {
			t.Fatalf("a computer that cannot join said %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a computer that cannot join said nothing")
	}
}

// While no member leads, a party asks again a little later each time, and
// not at once as it does when a member names the leader.
func Test_store_members_a_party_waits_longer_each_time_while_none_leads(t *testing.T) {
	var asked atomic.Int32
	s := answering(t, func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.WriteHeader(http.StatusMisdirectedRequest)
	})
	servedPatience = time.Second
	if _, _, err := s.Read(ctx, "a/x"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a read with no leader: %v", err)
	}
	if n := asked.Load(); n > 6 {
		t.Fatalf("a party asked %d times in a second", n)
	}
}

// A name longer than a member takes, not UTF-8, or with a part that is no
// file's name on Windows is refused before it is in the log and stops no
// member, by a write, a copy and a delete alike. The longest name a member
// takes is written to every copy.
func Test_store_members_a_name_no_member_takes_is_refused(t *testing.T) {
	c := newMembers(t, 3)
	s := c.owner()
	did(t)(s.Write(ctx, "a/x", []byte("value"), OnlyIfNew))
	long := strings.Repeat("n", memberPartMax+1)
	deep := strings.Repeat("d/", memberNameMax/2) + "x"
	windows := []string{"a/x:y", "q?/x", "a/x.", "a/x ", "a/NUL", "aux/x", "a/com1.txt", "a/Lpt² .x"}
	for _, name := range append([]string{"a/" + long, "new/" + long, long + "/x", deep, "a/\xff"}, windows...) {
		for what, change := range map[string]func() (bool, error){
			"a write":  func() (bool, error) { return s.Write(ctx, name, []byte("long"), Overwrite) },
			"a copy":   func() (bool, error) { return s.Copy(ctx, "a/x", name) },
			"a delete": func() (bool, error) { return s.Delete(ctx, name, Overwrite) },
		} {
			if done, err := change(); done || !errors.Is(err, ErrName) {
				t.Fatalf("%s of %q, of %d bytes: %v %v", what, name, len(name), done, err)
			}
		}
	}
	longest := long[1:] + "/" + strings.Repeat("n", memberNameMax-memberPartMax-3) + "/n"
	if len(longest) != memberNameMax {
		t.Fatalf("the longest name is %d bytes in this test", len(longest))
	}
	did(t)(s.Write(ctx, longest, []byte("longest"), OnlyIfNew))
	// A name that only looks like one Windows refuses is taken.
	for _, name := range []string{"a/console", "a/com10", "a/x.y z", "a/nul0.txt"} {
		did(t)(s.Write(ctx, name, []byte("taken"), OnlyIfNew))
	}
	if got := c.agree(); len(got) != 6 || got[longest] != "longest" {
		t.Fatalf("the copies hold %v", got)
	}
	for _, tm := range c.all {
		select {
		case err := <-tm.failed:
			t.Fatalf("%s stopped: %v", tm.address, err)
		default:
		}
	}
}

// A computer whose directory has a path too long for a member's longest
// name is no member: its start says why, before it asks to join.
func Test_store_members_a_path_too_long_for_a_members_names_is_no_member(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the test knows the longest path of Linux alone")
	}
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	for len(dir) < 3700 {
		dir = filepath.Join(dir, strings.Repeat("d", 99))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	const self = "127.0.0.1:1"
	if err := SetMembers(dir, []string{self + " joining", "127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	m, err := OpenMember(dir, self, proofOf(t, keeper))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if err := m.Start(); err == nil || !strings.Contains(err.Error(), "cannot be a member") || !strings.Contains(err.Error(), "is too long for a name of 512 bytes") {
		t.Fatalf("a member in a directory with a path of %d bytes started: %v", len(dir), err)
	}
}

// A call that no member answers reports what each member asked said: the
// members that are off, and the one that is on and knows of no leader.
func TestServedUnreachableSaysWhatEachMemberSaid(t *testing.T) {
	c := newMembers(t, 3)
	owner := c.owner()
	c.did(owner.Write(ctx, "a/x", []byte("x"), OnlyIfNew))
	on := c.all[0]
	c.stop(c.all[1])
	c.stop(c.all[2])
	servedPatience = 2 * time.Second
	_, _, err := owner.Read(ctx, "a/x")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a read with two of three members off: %v", err)
	}
	for _, want := range []string{
		on.address + ": this member knows of no leader",
		"dial tcp " + c.all[1].address,
		"dial tcp " + c.all[2].address,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("a read with two of three members off says %q, and not %q", err, want)
		}
	}
}
