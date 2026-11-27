package store

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/raft/v3"
	pb "go.etcd.io/raft/v3/raftpb"
	"google.golang.org/protobuf/proto"
)

// memberHard is what a member must not forget between two starts: the term
// it has seen and whom it voted for in it.
type memberHard struct {
	Term uint64 `json:"term"`
	Vote uint64 `json:"vote"`
}

// memberState is how far a member has applied the log, who the members
// were by then, and where the log it keeps begins.
type memberState struct {
	Applied  uint64              `json:"applied"`
	Voters   []uint64            `json:"voters"`
	Learners []uint64            `json:"learners"`
	Members  map[uint64]memberAt `json:"members"`
	// Compact is the last entry no longer kept, and CompactTerm its term.
	Compact     uint64 `json:"compact"`
	CompactTerm uint64 `json:"compactTerm"`
	// ConfIndex is the last entry that changed the members.
	ConfIndex uint64 `json:"confIndex"`
}

// node is what a member's loop alone reads and writes.
type node struct {
	rn     *raft.RawNode
	ms     *raft.MemoryStorage
	hard   memberHard
	commit uint64
	state  memberState // as last written down
	at     uint64      // how far the log is applied
	// since holds the values sent ahead whose entries are applied and not
	// yet written down as applied: each is kept until then, so that an
	// entry applied again after a stop finds its value here.
	since       []string
	recordedAt  time.Time
	first, last uint64 // the entries on disk
	peers       map[uint64]chan *pb.Message
	ticks       int
	promoting   bool
	away        map[uint64]bool // the members that turned this one away
	leaving     int             // the tick at which a member that was removed stops
}

func (m *Member) loadJSON(name string, v any) error {
	data, err := os.ReadFile(filepath.Join(m.log, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (m *Member) entryPath(index uint64) string {
	return filepath.Join(m.log, "entries", fmt.Sprintf("%020d", index))
}

// begin starts the member's loop from what its copy records.
func (m *Member) begin() error {
	if err := m.takesNames(); err != nil {
		return err
	}
	for _, dir := range []string{"entries", "staged"} {
		if err := mkdirAll(filepath.Join(m.log, dir)); err != nil {
			return err
		}
	}
	var state memberState
	var hard memberHard
	if err := errors.Join(m.loadJSON("applied", &state), m.loadJSON("hard", &hard)); err != nil {
		return fmt.Errorf("store: the member's log in %s: %w", m.log, err)
	}
	if n := len(state.Voters) + len(state.Learners); n > 0 && !slices.Contains(state.Voters, m.id) && !slices.Contains(state.Learners, m.id) {
		return fmt.Errorf("store: %s is not a member of this store: it was removed, or the copy in %s is another member's", m.name, m.dir.root)
	}
	entries, err := m.loadEntries(state.Compact, state.CompactTerm)
	if err != nil {
		return err
	}
	ms := raft.NewMemoryStorage()
	if state.Compact > 0 {
		addresses, _ := json.Marshal(state.Members)
		if err := ms.ApplySnapshot(&pb.Snapshot{Data: addresses, Metadata: &pb.SnapshotMetadata{
			Index: &state.Compact, Term: &state.CompactTerm,
			ConfState: &pb.ConfState{Voters: state.Voters, Learners: state.Learners}}}); err != nil {
			return err
		}
	}
	commit := max(state.Applied, state.Compact)
	if err := errors.Join(
		ms.SetHardState(&pb.HardState{Term: &hard.Term, Vote: &hard.Vote, Commit: &commit}),
		ms.Append(entries)); err != nil {
		return err
	}
	if state.ConfIndex > state.Compact {
		addresses, _ := json.Marshal(state.Members)
		if _, err := ms.CreateSnapshot(state.ConfIndex, &pb.ConfState{Voters: state.Voters, Learners: state.Learners}, addresses); err != nil {
			return err
		}
	}
	rn, err := raft.NewRawNode(&raft.Config{
		ID: m.id, ElectionTick: memberElection, HeartbeatTick: memberHeartbeat,
		Storage: ms, Applied: state.Applied,
		MaxSizePerMsg: 1 << 20, MaxInflightMsgs: 256,
		// A leader that hears from no majority stops leading, and a member
		// that was cut off does not unseat the leader when it comes back.
		CheckQuorum: true, PreVote: true,
		// What is asked of a member that does not lead is not passed on:
		// the party is told who leads.
		DisableProposalForwarding: true,
		StepDownOnRemoval:         true,
		Logger:                    &raft.DefaultLogger{Logger: log.New(io.Discard, "", 0)},
	})
	if err != nil {
		return err
	}
	m.node = node{rn: rn, ms: ms, hard: hard, commit: commit, state: state, at: state.Applied,
		recordedAt: time.Now(), peers: map[uint64]chan *pb.Message{}}
	if len(entries) > 0 {
		m.first, m.last = entries[0].GetIndex(), entries[len(entries)-1].GetIndex()
	}
	m.sweep()
	m.mu.Lock()
	m.running, m.applied, m.term = true, state.Applied, hard.Term
	m.voters, m.learners = state.Voters, state.Learners
	for id, member := range state.Members {
		m.addrs[id], m.called[id] = member.Address, member.Name
	}
	// A member is reached where it now listens, whatever it recorded.
	moved := len(state.Members) > 0 && state.Members[m.id].Address != m.self
	m.addrs[m.id], m.called[m.id] = m.self, m.name
	m.mu.Unlock()
	if moved {
		if err := m.noteAddresses(); err != nil {
			return err
		}
	}
	go m.run()
	return nil
}

// noteAddresses writes down the members' addresses as this member now has
// them, with what it last recorded of the log.
func (m *Member) noteAddresses() error {
	m.mu.Lock()
	for id, member := range m.state.Members {
		if address := m.addrs[id]; address != "" {
			member.Address = address
			m.state.Members[id] = member
		}
	}
	m.mu.Unlock()
	if err := m.saveJSON("applied", m.state); err != nil {
		return err
	}
	return m.writeMembers()
}

// loadEntries reads the log kept after compact, whose term is term: the
// entries that follow one another from there, each whole. What else is in
// the directory is removed: an entry that was being written when the
// computer stopped, entries that others have since taken the place of, and
// the log a copy of the directory made needless.
func (m *Member) loadEntries(compact, term uint64) ([]*pb.Entry, error) {
	files, err := os.ReadDir(filepath.Join(m.log, "entries"))
	if err != nil {
		return nil, err
	}
	var entries []*pb.Entry
	for index := compact + 1; ; index++ {
		data, err := os.ReadFile(m.entryPath(index))
		if err != nil || len(data) < 8 {
			break
		}
		sum := sha256.Sum256(data[8:])
		e := &pb.Entry{}
		if !bytes.Equal(sum[:8], data[:8]) || proto.Unmarshal(data[8:], e) != nil || e.GetIndex() != index || e.GetTerm() < term {
			break
		}
		term = e.GetTerm()
		entries = append(entries, e)
	}
	for _, file := range files {
		index, err := strconv.ParseUint(file.Name(), 10, 64)
		if err != nil || index <= compact || index > compact+uint64(len(entries)) {
			os.Remove(filepath.Join(m.log, "entries", file.Name()))
		}
	}
	return entries, nil
}

// saveEntries puts entries on disk, in place of any it has from there on.
func (m *Member) saveEntries(entries []*pb.Entry) error {
	from := entries[0].GetIndex()
	for index := m.last; index >= from && index > 0; index-- {
		os.Remove(m.entryPath(index))
	}
	for _, e := range entries {
		data, err := proto.Marshal(e)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		file, err := os.OpenFile(m.entryPath(e.GetIndex()), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		_, err = file.Write(append(sum[:8:8], data...))
		if err == nil {
			err = file.Sync()
		}
		if cerr := file.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	if m.first == 0 || from < m.first {
		m.first = from
	}
	m.last = entries[len(entries)-1].GetIndex()
	return syncDir(filepath.Join(m.log, "entries"))
}

// run is the member's loop: it alone moves the member's clock, takes what
// the other members send, and does what Raft then says is ready.
func (m *Member) run() {
	defer close(m.stopped)
	defer m.drop()
	tick := time.NewTicker(memberTick)
	defer tick.Stop()
	for {
		select {
		case <-m.stop:
			if memberStopRecords {
				m.record(true)
			}
			return
		case <-tick.C:
			m.rn.Tick()
			if err := m.ticked(); err != nil {
				m.fail(err)
				return
			}
		case f := <-m.do:
			f()
		}
		for m.rn.HasReady() {
			if err := m.ready(m.rn.Ready()); err != nil {
				if !errors.Is(err, errStopped) {
					m.fail(err)
				}
				return
			}
		}
	}
}

// drop tells everyone who waits on this member that it will not answer.
func (m *Member) drop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for req, done := range m.waiting {
		done <- errLost
		delete(m.waiting, req)
	}
	for id, at := range m.reads {
		close(at)
		delete(m.reads, id)
	}
}

// ready does what Raft says is ready, in the order that keeps its word:
// what must not be forgotten is on disk before anything is sent or applied.
func (m *Member) ready(rd raft.Ready) error {
	if rd.SoftState != nil {
		m.mu.Lock()
		led := m.leading
		m.lead, m.leading = rd.SoftState.Lead, rd.SoftState.RaftState == raft.StateLeader
		m.mu.Unlock()
		if led && rd.SoftState.RaftState != raft.StateLeader {
			m.drop()
		}
	}
	if rd.HardState != nil && !raft.IsEmptyHardState(rd.HardState) {
		m.commit = rd.HardState.GetCommit()
		if hard := (memberHard{rd.HardState.GetTerm(), rd.HardState.GetVote()}); hard != m.hard {
			if err := m.saveJSON("hard", hard); err != nil {
				return err
			}
			m.hard = hard
			m.mu.Lock()
			m.term = hard.Term
			m.mu.Unlock()
		}
		if err := m.ms.SetHardState(rd.HardState); err != nil {
			return err
		}
	}
	if rd.Snapshot != nil && !raft.IsEmptySnap(rd.Snapshot) {
		if err := m.restore(rd.Snapshot); err != nil {
			return err
		}
	}
	if len(rd.Entries) > 0 {
		if err := m.saveEntries(rd.Entries); err != nil {
			return err
		}
		if err := m.ms.Append(rd.Entries); err != nil {
			return err
		}
	}
	m.send(rd.Messages)
	for _, e := range rd.CommittedEntries {
		if err := m.apply(e); err != nil {
			return err
		}
	}
	m.mu.Lock()
	for _, rs := range rd.ReadStates {
		if at := m.reads[string(rs.RequestCtx)]; at != nil {
			at <- rs.Index
		}
	}
	m.mu.Unlock()
	m.rn.Advance(rd)
	return nil
}

// apply does what one entry of the log says to this member's copy.
func (m *Member) apply(e *pb.Entry) error {
	var req string
	var refused error
	removed, changed := false, false
	switch {
	case e.GetType() == pb.EntryConfChange:
		cc, about := &pb.ConfChange{}, memberChange{}
		if err := proto.Unmarshal(e.GetData(), cc); err != nil {
			return err
		}
		json.Unmarshal(cc.GetContext(), &about)
		cs := m.rn.ApplyConfChange(cc)
		m.mu.Lock()
		m.voters, m.learners = cs.GetVoters(), cs.GetLearners()
		if cc.GetType() == pb.ConfChangeRemoveNode {
			if removed = cc.GetNodeId() == m.id; !removed {
				delete(m.addrs, cc.GetNodeId())
				delete(m.called, cc.GetNodeId())
			}
		} else if about.Address != "" {
			m.called[cc.GetNodeId()] = about.Name
			// A member heard from since it joined has said where it is.
			if m.addrs[cc.GetNodeId()] == "" {
				m.addrs[cc.GetNodeId()] = about.Address
			}
		}
		m.mu.Unlock()
		// What a member that must copy the directory is sent names the
		// members as they are from here on: Raft has a member take it
		// only if it names that member.
		m.state.ConfIndex = e.GetIndex()
		if _, err := m.ms.CreateSnapshot(e.GetIndex(), cs, m.addresses()); err != nil {
			return err
		}
		if err := m.writeMembers(); err != nil {
			return err
		}
		req, changed = about.Req, true
	case len(e.GetData()) > 0:
		var c change
		if err := json.Unmarshal(e.GetData(), &c); err != nil {
			return err
		}
		// A change to a name that can hold no value does nothing on any
		// member, and stops none: whoever asked is told.
		if err := m.carry(c, e.GetIndex()); errors.Is(err, errNoRoom) {
			refused = err
		} else if err != nil {
			return err
		}
		if c.ID != "" {
			m.since = append(m.since, c.ID)
		}
		req = c.Req
	}
	m.at = e.GetIndex()
	m.mu.Lock()
	m.applied, m.served = e.GetIndex(), e.GetTerm()
	if done := m.waiting[req]; done != nil {
		done <- refused
		delete(m.waiting, req)
	}
	m.mu.Unlock()
	if changed {
		if err := m.record(true); err != nil {
			return err
		}
	}
	if removed {
		// A leader that is removed has just sent the others word that the
		// entry is in the log for good. It stays until that has arrived,
		// and gives its vote meanwhile: with it stopped at once, the other
		// of two members is left with an entry it can never count as done.
		m.leaving = m.ticks + 3*memberElection
	}
	return nil
}

func (m *Member) errRemoved() error {
	return fmt.Errorf("store: %s was removed from the store's members", m.name)
}

// table gives the members' names and addresses.
func (m *Member) table() map[uint64]memberAt {
	m.mu.Lock()
	defer m.mu.Unlock()
	table := map[uint64]memberAt{}
	for id, name := range m.called {
		table[id] = memberAt{name, m.addrs[id]}
	}
	return table
}

// addresses gives the members' names and addresses as a snapshot carries
// them.
func (m *Member) addresses() []byte {
	addresses, _ := json.Marshal(m.table())
	return addresses
}

// carry does what a change says. A value comes from where it was sent
// ahead; a member that was not sent it, or that has applied the entry
// before, finds it under the name, and otherwise asks another member.
func (m *Member) carry(c change, index uint64) error {
	ctx := context.Background()
	staged := m.staged(c.ID)
	if m.dir.above(c.Name) {
		return fmt.Errorf("%w: names are under %s", errNoRoom, c.Name)
	}
	if at := m.dir.below(c.Name); at != "" && c.Op != "delete" {
		return fmt.Errorf("%w: %s is under %s, which holds a value", errNoRoom, c.Name, at)
	}
	switch c.Op {
	case "delete":
		_, err := m.dir.Delete(ctx, c.Name, Overwrite)
		return err
	case "copy":
		if !exists(staged) {
			// The value copied is sent nowhere: each member has it under
			// the name it is copied from.
			if mark, err := m.dir.markAt(c.From); err != nil {
				return err
			} else if mark == c.Mark {
				if err := link(m.dir.path(c.From), staged); err != nil && !exists(staged) {
					return err
				}
			}
		}
		fallthrough
	case "put":
		if exists(staged) {
			return m.dir.adopt(ctx, staged, c.Name)
		}
		if mark, err := m.dir.markAt(c.Name); err != nil || mark == c.Mark {
			return err
		}
		return m.pull(c, index)
	}
	return fmt.Errorf("store: the log holds a change this Corbie does not know: %q", c.Op)
}

// pull has another member give the value an entry names, where this member
// was not sent it: the value as it was sent ahead, or else what the name
// holds on a member that has applied the entry. That can be a later value;
// the entries that follow bring this copy to the same.
func (m *Member) pull(c change, index uint64) error {
	for !m.isStopped() {
		var errs []error
		for _, address := range m.others() {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
			done, err := m.fetch(ctx, address, "/member/staged?id="+c.ID, m.staged(c.ID), c.Name)
			if !done && err == nil {
				id := newID()
				done, err = m.fetch(ctx, address, "/member/value?name="+url.QueryEscape(c.Name)+"&at="+strconv.FormatUint(index, 10), m.staged(id), c.Name)
				os.Remove(m.staged(id))
				if !done && err == nil {
					// The name holds nothing there: it was deleted since.
					_, err = m.dir.Delete(ctx, c.Name, Overwrite)
					done = true
				}
			}
			cancel()
			switch {
			case done && err != nil && m.isStopped():
				return errStopped
			case done:
				// What this copy would not take stops the member, and the
				// entry is applied again when it next starts.
				return err
			}
			errs = append(errs, err)
		}
		if err := errors.Join(errs...); err != nil {
			m.slow(err)
		}
		select {
		case <-m.stop:
		case <-time.After(2 * memberTick):
		}
	}
	return errStopped
}

// fetch asks a member for a value, keeps it at path and puts it under name.
// It says whether the member gave one, and a member that has no such value
// is no error. An error beside a value given is this copy's own.
func (m *Member) fetch(ctx context.Context, address, route, path, name string) (bool, error) {
	resp, err := m.call(ctx, address, http.MethodGet, route, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("store: the member at %s: %s", address, resp.Status)
	}
	tmp, err := writeTemp(filepath.Dir(path), func(w io.Writer) error {
		_, err := io.Copy(w, resp.Body)
		return err
	})
	if err != nil {
		return false, err
	}
	if err := rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	return true, m.dir.adopt(ctx, path, name)
}

// restore makes this member's copy the same as the others' where the log
// they keep no longer goes back far enough for it to follow: it copies the
// directory from a member that has applied the log as far as the snapshot
// says. The copy is not of one moment, and need not be: it is no older
// than the snapshot, and the entries after the snapshot are applied to it.
func (m *Member) restore(snap *pb.Snapshot) error {
	index, term := snap.GetMetadata().GetIndex(), snap.GetMetadata().GetTerm()
	addresses := map[uint64]memberAt{}
	json.Unmarshal(snap.GetData(), &addresses)
	m.mu.Lock()
	for id, member := range addresses {
		m.called[id] = member.Name
		if id != m.id {
			m.addrs[id] = member.Address
		}
	}
	m.mu.Unlock()
	for copied := false; !copied; {
		if m.isStopped() {
			return errStopped
		}
		var errs []error
		for _, address := range m.others() {
			err := m.copyFrom(address, index)
			if copied = err == nil; copied {
				break
			}
			errs = append(errs, err)
		}
		if !copied {
			if err := errors.Join(errs...); err != nil {
				m.slow(err)
			}
			select {
			case <-m.stop:
			case <-time.After(2 * memberTick):
			}
		}
	}
	cs := snap.GetMetadata().GetConfState()
	m.mu.Lock()
	m.voters, m.learners, m.applied = cs.GetVoters(), cs.GetLearners(), index
	m.mu.Unlock()
	m.at, m.since = index, nil
	m.state = memberState{Applied: index, Compact: index, CompactTerm: term, ConfIndex: index,
		Voters: cs.GetVoters(), Learners: cs.GetLearners(), Members: m.table()}
	// How far the copy is applied is written down before the log it makes
	// needless goes: a computer that stops between the two starts with a
	// log it does not need, and never with one that ends before what it
	// has written down as applied.
	if err := m.saveJSON("applied", m.state); err != nil {
		return err
	}
	for ; m.first != 0 && m.first <= m.last; m.first++ {
		os.Remove(m.entryPath(m.first))
	}
	m.first, m.last = 0, 0
	if err := m.ms.ApplySnapshot(snap); err != nil {
		return err
	}
	return m.writeMembers()
}

// copyFrom makes this copy hold what a member's copy holds, once that
// member has applied the log as far as index.
func (m *Member) copyFrom(address string, index uint64) error {
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	at := "&at=" + strconv.FormatUint(index, 10)
	resp, err := m.call(ctx, address, http.MethodGet, "/member/names?"+at[1:], nil)
	if err != nil {
		return err
	}
	said, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("store: the member at %s did not list the whole directory: %w", address, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("store: the member at %s did not list the directory: %s", address, resp.Status)
	}
	theirs := map[string]Mark{}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(said), "\n"), "\n") {
		if name, mark, ok := strings.Cut(line, "\t"); ok {
			theirs[name] = Mark(mark)
		}
	}
	mine, err := m.dir.List(ctx, "")
	if err != nil {
		return err
	}
	for _, l := range mine {
		if mark, ok := theirs[l.Name]; ok && mark == l.Mark {
			delete(theirs, l.Name)
		} else if !ok {
			if _, err := m.dir.Delete(ctx, l.Name, Overwrite); err != nil {
				return err
			}
		}
	}
	for name := range theirs {
		id := newID()
		_, err := m.fetch(ctx, address, "/member/value?name="+url.QueryEscape(name)+at, m.staged(id), name)
		os.Remove(m.staged(id))
		if err != nil {
			return err
		}
	}
	return nil
}

// writeMembers writes the members' addresses where a process on this
// computer that opens the directory finds them.
func (m *Member) writeMembers() error {
	m.mu.Lock()
	var members []MemberAt
	for _, id := range m.voters {
		if m.addrs[id] != "" {
			members = append(members, MemberAt{Name: cmp.Or(m.called[id], unnamed), Address: m.addrs[id]})
		}
	}
	for _, id := range m.learners {
		if m.addrs[id] != "" {
			members = append(members, MemberAt{Name: cmp.Or(m.called[id], unnamed), Address: m.addrs[id], Joining: true})
		}
	}
	m.mu.Unlock()
	return SetMembers(m.dir.root, members)
}

// record writes down how far the log is applied, once enough is applied or
// enough time has passed, and then lets go of what that made needless: the
// values sent ahead for those entries, and the log further back than a
// member that was off for a moment needs.
func (m *Member) record(force bool) error {
	if m.at == m.state.Applied {
		return nil
	}
	if !force && len(m.since) < memberRecordEvery && time.Since(m.recordedAt) < memberRecord {
		return nil
	}
	state := m.state
	state.Applied = m.at
	m.mu.Lock()
	state.Voters, state.Learners = slices.Clone(m.voters), slices.Clone(m.learners)
	m.mu.Unlock()
	state.Members = m.table()
	if m.at > state.Compact+2*memberTrail {
		cut := m.at - memberTrail
		term, err := m.ms.Term(cut)
		if err != nil {
			return err
		}
		// Since the members last changed they have been as they are now.
		if cut > state.ConfIndex {
			addresses, _ := json.Marshal(state.Members)
			if _, err := m.ms.CreateSnapshot(cut, &pb.ConfState{Voters: state.Voters, Learners: state.Learners}, addresses); err != nil {
				return err
			}
		}
		if err := m.ms.Compact(cut); err != nil {
			return err
		}
		state.Compact, state.CompactTerm = cut, term
	}
	if err := m.saveJSON("applied", state); err != nil {
		return err
	}
	m.state, m.recordedAt = state, time.Now()
	for _, id := range m.since {
		os.Remove(m.staged(id))
	}
	m.since = nil
	for ; m.first != 0 && m.first <= state.Compact; m.first++ {
		os.Remove(m.entryPath(m.first))
	}
	return nil
}

// ticked does what a member does as time passes.
func (m *Member) ticked() error {
	m.ticks++
	if m.leaving != 0 && m.ticks >= m.leaving {
		return m.errRemoved()
	}
	if err := m.record(false); err != nil {
		return err
	}
	if m.ticks%memberElection != 0 {
		return nil
	}
	if m.ticks%(int(stagedMaxAge/time.Second)*memberElection) == 0 {
		m.sweep()
	}
	// A computer that has joined counts as a member once its copy is
	// whole: it asks the leader to count it.
	m.mu.Lock()
	joining, led := slices.Contains(m.learners, m.id), m.lead != 0
	m.mu.Unlock()
	if joining && led && m.at > 0 && m.at >= m.commit && !m.promoting {
		m.promoting = true
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			m.unanswered(m.askLeader(ctx, "/member/promote?id="+strconv.FormatUint(m.id, 10)))
			cancel()
			m.later(func() { m.promoting = false })
		}()
	}
	return nil
}

// sweep removes the values sent ahead that no entry will name: those old
// enough, and named by no entry still to be applied or written down.
func (m *Member) sweep() {
	keep := map[string]bool{}
	for _, id := range m.since {
		keep[id] = true
	}
	if entries, err := m.ms.Entries(m.state.Applied+1, m.last+1, math.MaxUint64); err == nil {
		for _, e := range entries {
			var c change
			if e.GetType() == pb.EntryNormal && json.Unmarshal(e.GetData(), &c) == nil {
				keep[c.ID] = true
			}
		}
	}
	files, _ := os.ReadDir(filepath.Join(m.log, "staged"))
	for _, file := range files {
		info, err := file.Info()
		if err == nil && !keep[file.Name()] && time.Since(info.ModTime()) > stagedMaxAge {
			os.Remove(filepath.Join(m.log, "staged", file.Name()))
		}
	}
}

// send gives the other members what Raft has for them. Each has a queue of
// its own, so that one that does not answer holds up no other; what does
// not fit is dropped, and Raft sends it again.
func (m *Member) send(messages []*pb.Message) {
	for _, msg := range messages {
		to := msg.GetTo()
		queue := m.peers[to]
		if queue == nil {
			queue = make(chan *pb.Message, 256)
			m.peers[to] = queue
			go m.sendTo(to, queue)
		}
		select {
		case queue <- msg:
		default:
		}
	}
}

func (m *Member) sendTo(to uint64, queue chan *pb.Message) {
	for {
		var batch []*pb.Message
		select {
		case <-m.stop:
			return
		case msg := <-queue:
			batch = append(batch, msg)
		}
		for more := true; more && len(batch) < 64; {
			select {
			case msg := <-queue:
				batch = append(batch, msg)
			default:
				more = false
			}
		}
		var body []byte
		snapshot, refused := false, false
		for _, msg := range batch {
			data, err := proto.Marshal(msg)
			if err != nil {
				continue
			}
			body = binary.BigEndian.AppendUint32(body, uint32(len(data)))
			body = append(body, data...)
			snapshot = snapshot || msg.GetType() == pb.MsgSnap
		}
		m.mu.Lock()
		address := m.addrs[to]
		m.mu.Unlock()
		err := fmt.Errorf("store: member %d has no address", to)
		if address != "" {
			ctx, cancel := context.WithTimeout(m.ctx, memberElection*memberTick)
			var resp *http.Response
			if resp, err = m.call(ctx, address, http.MethodPost, "/member/raft", bytes.NewReader(body)); err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					err = fmt.Errorf("store: the member at %s: %s", address, resp.Status)
				}
				refused = resp.StatusCode == http.StatusForbidden
			}
			cancel()
		}
		m.later(func() {
			m.turned(to, refused)
			if err != nil {
				m.rn.ReportUnreachable(to)
			}
			if snapshot {
				status := raft.SnapshotFinish
				if err != nil {
					status = raft.SnapshotFailure
				}
				m.rn.ReportSnapshot(to, status)
			}
		})
		if err != nil {
			select {
			case <-m.stop:
				return
			case <-time.After(memberTick):
			}
		}
	}
}

// turned notes that another member turned this one away, or did not. The
// leader stops sending to a member once it is removed, so a removed member
// is not always sent the entry that removes it: it learns that it is no
// member when every other member it knows says so.
func (m *Member) turned(by uint64, away bool) {
	if !away {
		delete(m.away, by)
		return
	}
	if m.away == nil {
		m.away = map[uint64]bool{}
	}
	m.away[by] = true
	m.mu.Lock()
	others := 0
	for _, id := range m.voters {
		if id != m.id && !m.away[id] {
			others++
		}
	}
	known := len(m.voters) > 1
	m.mu.Unlock()
	if known && others == 0 {
		m.fail(m.errRemoved())
	}
}

// serveRaft takes what another member's Raft sends. A computer that is no
// member is turned away.
func (m *Member) serveRaft(w http.ResponseWriter, r *http.Request) {
	from := r.Header.Get(headAt)
	id, _ := strconv.ParseUint(r.Header.Get(headFrom), 10, 64)
	m.mu.Lock()
	running := m.running
	member := len(m.voters)+len(m.learners) == 0 || slices.Contains(m.voters, id) || slices.Contains(m.learners, id)
	m.mu.Unlock()
	switch {
	case !running:
		http.Error(w, "this store has one member", http.StatusConflict)
		return
	case !member:
		http.Error(w, from+" is not a member of this store", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		http.Error(w, "the message did not arrive whole", http.StatusBadRequest)
		return
	}
	for len(body) >= 4 {
		n := int(binary.BigEndian.Uint32(body))
		if n > len(body)-4 {
			break
		}
		msg := &pb.Message{}
		if proto.Unmarshal(body[4:4+n], msg) == nil && msg.GetFrom() == id {
			select {
			case m.do <- func() { m.rn.Step(msg) }:
			case <-r.Context().Done():
			case <-m.stop:
			}
		}
		body = body[4+n:]
	}
}
