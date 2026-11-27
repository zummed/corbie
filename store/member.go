package store

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	pb "go.etcd.io/raft/v3/raftpb"
)

// A served directory has members: the computers that each hold a copy of it
// and serve it. With one member the copy is the store and nothing here runs.
// With more, the members keep their copies in step with Raft:
//
//	a change is asked of the leader, which checks its condition against its
//	own copy, sends the value to the other members, and puts in the log what
//	the change does: put this value under this name, delete this name;
//	a member applies the log to its copy in order.
//
// What is in the log is what a change does, never its condition, so a member
// that applies an entry twice, or applies entries to a copy made a little
// after them, ends with the same copy as the others.
//
// A member keeps, in .log at the top of its copy:
//
//	members          each member's name and address, one member to a line
//	hard             the term and the vote
//	applied          how far the log is applied, who the members are, and
//	                 where the kept log begins
//	entries/<index>  the log
//	staged/<id>      values sent ahead of their entries
const (
	membersDir  = ".log"
	membersFile = "members"

	headFrom   = "Corbie-From"
	headAt     = "Corbie-At"
	headLeader = "Corbie-Leader"

	// memberElection and memberHeartbeat are in ticks.
	memberElection  = 10
	memberHeartbeat = 1

	// memberRecordEvery is how many entries are applied before how far is
	// written down, where memberRecord has not passed first.
	memberRecordEvery = 256

	// stagedMaxAge is how old a value sent ahead must be, with no entry
	// that names it, before it is removed.
	stagedMaxAge = time.Hour
)

var (
	// memberTick is how often a member's clock moves.
	memberTick = 100 * time.Millisecond
	// memberRecord is how long applied entries go before how far is
	// written down.
	memberRecord = time.Second
	// memberTrail is how many entries are kept behind what is applied, for
	// a member that was off for a moment to follow without copying the
	// directory again.
	memberTrail uint64 = 1000

	// memberStopRecords is whether a member that stops writes down how far
	// it has applied the log, for a test to stop one as losing power does.
	memberStopRecords = true

	// memberCut, when set, says whether the wire from one member to
	// another is cut, for a test to cut it.
	memberCut atomic.Pointer[func(from, to string) bool]
)

// errMembers is a change asked of a directory that has members by anything
// but its member's server.
var errMembers = errors.New("store: this directory has members, and only a member's server changes it")

// errStopped is what a stopped member answers.
var errStopped = fmt.Errorf("%w: this member has stopped", ErrUnreachable)

// notLeader is the answer of a member that does not lead: the leader's
// address, where it knows one.
type notLeader struct{ leader string }

func (e *notLeader) Error() string { return "store: this member does not lead the store" }

// errLost is a change the leader put in the log and then stopped leading
// before it could answer.
var errLost = fmt.Errorf("%w: the member stopped leading before the change was answered", ErrUnknown)

// HasMembers says whether the directory at root has members.
func HasMembers(root string) bool {
	_, err := os.Stat(filepath.Join(root, membersDir, membersFile))
	return err == nil
}

// A MemberAt is a member as the members file lists it: its name, the
// address the others reach it at, as host:port, and whether it is joining:
// it copies the directory, and does not yet count.
type MemberAt struct {
	Name    string
	Address string
	Joining bool
}

// unnamed stands in the members file for the name of a member a computer
// is to ask and has not yet heard from.
const unnamed = "-"

// MembersOf gives the members of the directory at root, by name, or none
// where the directory has one member.
func MembersOf(root string) ([]MemberAt, error) {
	data, err := os.ReadFile(filepath.Join(root, membersDir, membersFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var members []MemberAt
	for line := range strings.Lines(string(data)) {
		if fields := strings.Fields(line); len(fields) >= 2 {
			members = append(members, MemberAt{Name: fields[0], Address: fields[1], Joining: len(fields) > 2})
		}
	}
	return members, err
}

// AddressesOf gives the addresses of the members of the directory at root.
func AddressesOf(root string) ([]string, error) {
	members, err := MembersOf(root)
	addresses := make([]string, len(members))
	for i, member := range members {
		addresses[i] = member.Address
	}
	return addresses, err
}

// SetMembers writes the members file: a line for each member, with its
// name, its address and, where it is joining, the word joining.
func SetMembers(root string, members []MemberAt) error {
	log := filepath.Join(root, membersDir)
	if err := mkdirAll(log); err != nil {
		return err
	}
	var lines []string
	for _, member := range members {
		line := member.Name + " " + member.Address
		if member.Joining {
			line += " joining"
		}
		lines = append(lines, line)
	}
	slices.Sort(lines)
	return writeWhole(filepath.Join(log, membersFile), []byte(strings.Join(lines, "\n")+"\n"))
}

// SetJoining sets the directory at root up to join a store's members: the
// computer itself, by the name and address it is to have, and the addresses
// of the members it is to ask.
func SetJoining(root, name, address string, ask []string) error {
	members := []MemberAt{{Name: name, Address: address, Joining: true}}
	for _, address := range ask {
		members = append(members, MemberAt{Name: unnamed, Address: address})
	}
	return SetMembers(root, members)
}

// LeaveMembers makes the copy at root the whole store, with one member: the
// log goes, and what of it was not applied goes with it.
func LeaveMembers(root string) error {
	return os.RemoveAll(filepath.Join(root, membersDir))
}

// MemberID is the number a member is known to the others by, made of the
// key of the member's own card. A computer that takes a member's place has
// another card, and so another number under the same name.
func MemberID(key ed25519.PublicKey) uint64 {
	sum := sha256.Sum256(key)
	return max(binary.BigEndian.Uint64(sum[:8]), 1)
}

func newID() string {
	var b [12]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// writeWhole puts data at path, whole and safely on disk.
func writeWhole(path string, data []byte) error {
	tmp, err := writeTemp(filepath.Dir(path), func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	if err != nil {
		return err
	}
	if err := rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return syncDir(filepath.Dir(path))
}

// A change is one entry of the log: what a write, a copy or a delete does.
type change struct {
	Req  string `json:"req"`
	Op   string `json:"op"` // put, copy or delete
	Name string `json:"name"`
	From string `json:"from,omitzero"` // of a copy
	ID   string `json:"id,omitzero"`   // the value sent ahead
	Mark Mark   `json:"mark,omitzero"` // of the value
}

// memberChange rides with a change of members: who asked, and the member's
// name and address.
type memberChange struct {
	Req     string `json:"req"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// memberAt is a member's name and address, as the members keep them.
type memberAt struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// A Member is one computer's part in a served directory: its copy, and with
// more than one member its place among them.
type Member struct {
	// Failed is told of what stops the member.
	Failed func(error)
	// Held is told of what goes wrong between this member and another and
	// does not stop it: a member that does not give it the directory.
	Held func(error)

	dir    *directory
	log    string
	name   string
	self   string // the address the others reach this member at
	id     uint64
	keeper ed25519.PublicKey
	client *http.Client

	names   keyed
	changes sync.Mutex // one change of members at a time
	confirm confirmer

	mu       sync.Mutex
	many     bool // the directory has more than one member
	running  bool
	lead     uint64
	leading  bool
	term     uint64
	served   uint64 // the term of the last entry applied
	applied  uint64
	voters   []uint64
	learners []uint64
	addrs    map[uint64]string
	called   map[uint64]string // the members' names
	waiting  map[string]chan error
	reads    map[string]chan uint64
	err      error
	held     string // what Held was last told

	node     // the loop's own
	do       chan func()
	stop     chan struct{}
	stopOnce sync.Once
	stopped  chan struct{}
	// ctx ends when the member is to stop, and with it what the member
	// asks of the others.
	ctx  context.Context
	halt context.CancelFunc
}

// OpenMember opens this computer's copy of a served directory, as the member
// of a name and a number, which other computers reach at self, host:port.
// proof is the key of the store's controller, which every member holds.
func OpenMember(dir, name string, id uint64, self string, proof tls.Certificate) (*Member, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	key, ok := proof.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("store: a member proves the key of the store's controller")
	}
	if _, _, err := net.SplitHostPort(self); err != nil {
		return nil, fmt.Errorf("store: a member's address reads host:port: %w", err)
	}
	opened, err := OpenDirectory(root)
	if err != nil {
		return nil, err
	}
	keeper := key.Public().(ed25519.PublicKey)
	ctx, halt := context.WithCancel(context.Background())
	return &Member{
		ctx: ctx, halt: halt, many: HasMembers(root),
		dir:  &directory{root: root, notices: opened.(*directory).notices, member: true},
		log:  filepath.Join(root, membersDir),
		name: name, self: self, id: id, keeper: keeper,
		client: &http.Client{Transport: &http.Transport{
			TLSClientConfig: memberTLS(proof, keeper),
			// A member that is off holds up no connection for the minutes
			// the operating system waits.
			DialContext:           (&net.Dialer{Timeout: servedSilence}).DialContext,
			TLSHandshakeTimeout:   servedSilence,
			MaxIdleConnsPerHost:   16,
			IdleConnTimeout:       time.Minute,
			ResponseHeaderTimeout: time.Minute,
		}},
		addrs:   map[uint64]string{},
		called:  map[uint64]string{},
		waiting: map[string]chan error{},
		reads:   map[string]chan uint64{},
		do:      make(chan func()),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}, nil
}

// memberTLS is how one end of the wire knows the other: by the key it
// proves, and by nothing else.
func memberTLS(proof tls.Certificate, keeper ed25519.PublicKey) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{proof},
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
		VerifyConnection: func(c tls.ConnectionState) error {
			if !keeper.Equal(proven(&c)) {
				return errKeeper
			}
			return nil
		},
	}
}

// Address is where other computers reach the member, as host:port.
func (m *Member) Address() string { return m.self }

// Directory is the member's copy, as its server reads and writes it.
func (m *Member) Directory() Store { return m.dir }

// Start has the member take its place: among the members its copy records,
// or, on a computer set up to join, by asking them to take it. With one
// member it starts nothing.
func (m *Member) Start() error {
	members, _ := MembersOf(m.dir.root)
	switch {
	case exists(filepath.Join(m.log, "applied")):
		return m.begin()
	case len(members) == 1 && !members[0].Joining:
		// The computer stopped as the first other computer joined, before
		// anything was in the log: the directory has one member still.
		m.mu.Lock()
		m.many = false
		m.mu.Unlock()
		return LeaveMembers(m.dir.root)
	case HasMembers(m.dir.root):
		if err := m.begin(); err != nil {
			return err
		}
		go m.join()
	}
	return nil
}

// Stop ends the member's part. Its copy stays as it is.
func (m *Member) Stop() {
	m.end()
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()
	if running {
		<-m.stopped
	}
}

// fail stops the member over something it cannot go on after, and says what.
func (m *Member) fail(err error) {
	m.mu.Lock()
	first := m.err == nil
	if first {
		m.err = err
	}
	m.mu.Unlock()
	if first && m.Failed != nil {
		m.Failed(err)
	}
	m.end()
}

// end has the member's loop stop, and ends what the member is asking of
// the others.
func (m *Member) end() {
	m.stopOnce.Do(func() {
		close(m.stop)
		m.halt()
	})
}

// Err gives what stopped the member, where something did.
func (m *Member) Err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

// slow tells Held of something, once until it is told of something else.
func (m *Member) slow(err error) {
	m.mu.Lock()
	again := m.held == err.Error()
	m.held = err.Error()
	m.mu.Unlock()
	if !again && m.Held != nil && !m.isStopped() {
		m.Held(err)
	}
}

func (m *Member) isStopped() bool {
	select {
	case <-m.stop:
		return true
	default:
		return false
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Members gives the members' names as this member has them, voters first,
// the leader's address where it knows one, and whether the directory has
// more than one member at all.
func (m *Member) Members() (voters, joining []string, leader string, many bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.voters {
		voters = append(voters, m.called[id])
	}
	for _, id := range m.learners {
		joining = append(joining, m.called[id])
	}
	slices.Sort(voters)
	slices.Sort(joining)
	return voters, joining, m.addrs[m.lead], m.running
}

// Led says whether the member knows of a leader, itself or another: a
// majority of the members can then reach each other.
func (m *Member) Led() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lead != 0
}

// store gives what answers a party's request here: the copy itself while
// the directory has one member, and this member while it leads the others.
// A member that has not yet started answers as one that does not lead.
func (m *Member) store() (Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.err != nil:
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, m.err)
	case !m.running && m.many:
		return nil, &notLeader{}
	case !m.running:
		return m.dir, nil
	case m.leading && m.served == m.term:
		return leader{m, m.term}, nil
	}
	lead := m.addrs[m.lead]
	if m.lead == m.id {
		lead = "" // leading, and not yet caught up with its own log
	}
	return nil, &notLeader{lead}
}

// leader is the store a leading member is to the parties, in the term it
// was found to lead in.
type leader struct {
	m    *Member
	term uint64
}

func (l leader) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := l.m.confirm.do(ctx, l.m.confirmed); err != nil {
		return nil, None, err
	}
	return l.m.dir.Read(ctx, name)
}

func (l leader) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := l.m.confirm.do(ctx, l.m.confirmed); err != nil {
		return nil, err
	}
	return l.m.dir.ReadPart(ctx, name, from, length)
}

func (l leader) List(ctx context.Context, under string) ([]Listed, error) {
	if err := l.m.confirm.do(ctx, l.m.confirmed); err != nil {
		return nil, err
	}
	return l.m.dir.List(ctx, under)
}

func (l leader) First(ctx context.Context, under string, n int) ([]Listed, error) {
	if err := l.m.confirm.do(ctx, l.m.confirmed); err != nil {
		return nil, err
	}
	return l.m.dir.First(ctx, under, n)
}

func (l leader) Notice(under string) (<-chan struct{}, func()) { return l.m.dir.Notice(under) }

func (l leader) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	return l.m.put(ctx, name, bytes.NewReader(value), how, l.term)
}

func (l leader) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	return l.m.put(ctx, name, r, OnlyIfNew, l.term)
}

func (l leader) Copy(ctx context.Context, from, to string) (bool, error) {
	m := l.m
	if err := errors.Join(checkMemberName(from), checkMemberName(to)); err != nil {
		return false, err
	}
	defer m.names.lock(from, to)()
	var mark Mark
	fits, err := m.settled(ctx, func() (bool, error) {
		var err error
		mark, err = m.dir.markAt(from)
		return mark != None && !exists(m.dir.path(to)), err
	})
	switch {
	case err != nil:
		return false, err
	case mark == None:
		return false, notThere(from)
	case !fits:
		return false, nil
	}
	return m.propose(ctx, change{Op: "copy", Name: to, From: from, ID: newID(), Mark: mark}, l.term)
}

func (l leader) Delete(ctx context.Context, name string, how How) (bool, error) {
	m := l.m
	if err := checkName(name); err != nil {
		return false, err
	}
	defer m.names.lock(name)()
	// A name the directory took when it had one member is deleted, though
	// a store with members takes no such name: every member holds it.
	// Windows holds no such name, and opens another file by it.
	if err := checkMemberName(name); err != nil && (runtime.GOOS == "windows" || !exists(m.dir.path(name))) {
		return false, err
	}
	if fits, err := m.settled(ctx, func() (bool, error) { return m.fits(name, how) }); err != nil || !fits {
		return false, err
	}
	return m.propose(ctx, change{Op: "delete", Name: name}, l.term)
}

// put writes a value in the way how says: the value is sent ahead to the
// members, and the log then says which name holds it.
func (m *Member) put(ctx context.Context, name string, r io.Reader, how How, term uint64) (bool, error) {
	if err := checkMemberName(name); err != nil {
		return false, err
	}
	// The value is taken in before the name is held: a party sends it as
	// slowly as it likes, and holds up no other write of the name.
	id, mark, err := m.stage(ctx, r)
	if err != nil {
		return false, err
	}
	defer m.names.lock(name)()
	fits, err := m.settled(ctx, func() (bool, error) { return m.fits(name, how) })
	if err == nil && fits {
		err = m.spread(id)
	}
	if err != nil || !fits {
		os.Remove(m.staged(id))
		return false, err
	}
	return m.propose(ctx, change{Op: "put", Name: name, ID: id, Mark: mark}, term)
}

// settled says whether a change's condition holds. That it does not is this
// copy's word alone, and a leader that was cut off does not know what the
// others have done since: so a majority first confirm that this member
// still leads, as for a read, and the condition is then looked at again.
func (m *Member) settled(ctx context.Context, holds func() (bool, error)) (bool, error) {
	if fits, err := holds(); err != nil || fits {
		return fits, err
	}
	if err := m.confirm.do(ctx, m.confirmed); err != nil {
		return false, err
	}
	return holds()
}

// A member's names have the same limits on every computer, so that a name
// the leader puts in the log is one each member's file system takes.
const (
	memberPartMax = 255
	memberNameMax = 512
)

// checkMemberName says whether name is one every member holds a value under. Every
// member applies an entry of the log, and one that cannot stops, at each
// start again: so a name is counted before a change is put in the log, and
// a computer whose file system does not take the longest is no member.
func checkMemberName(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	switch {
	case !utf8.ValidString(name):
		return fmt.Errorf("%w: %q is not UTF-8", ErrName, name)
	case len(name) > memberNameMax:
		return fmt.Errorf("%w: %q is %d bytes, and a store with members takes a name of %d", ErrName, name, len(name), memberNameMax)
	}
	for part := range strings.SplitSeq(name, "/") {
		if len(part) > memberPartMax {
			return fmt.Errorf("%w: part %q is %d bytes, and a store with members takes a part of %d", ErrName, part, len(part), memberPartMax)
		}
		if windowsRefuses(part) {
			return fmt.Errorf("%w: Windows takes no file named %q, and a store with members takes a name every member's computer does", ErrName, part)
		}
	}
	return nil
}

// windowsRefuses says whether part is no file's name on Windows, or on a
// file system made for it: one of its characters is kept for something
// else, it ends in a dot or a space, or it is the name of a device, alone
// or before a dot.
func windowsRefuses(part string) bool {
	if strings.ContainsAny(part, `<>:"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
		return true
	}
	device, _, _ := strings.Cut(part, ".")
	device = strings.ToUpper(strings.TrimRight(device, " "))
	switch device {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(device) < 4 || device[:3] != "COM" && device[:3] != "LPT" {
		return false
	}
	port := device[3:]
	return utf8.RuneCountInString(port) == 1 && strings.ContainsAny(port, "0123456789¹²³")
}

// takesNames says whether this computer's file system takes the longest
// name a member is given: a part of the most bytes as the name of a file,
// and the path of the temporary file a member writes beside the longest
// name.
func (m *Member) takesNames() error {
	part := strings.Repeat("n", memberPartMax)
	if _, err := os.Lstat(m.dir.path(part)); errors.Is(err, syscall.ENAMETOOLONG) {
		return fmt.Errorf("store: this computer cannot be a member: the file system of %s takes no file name of %d bytes, and a member's names have parts that long", m.dir.root, memberPartMax)
	}
	longest := part + "/" + strings.Repeat("n", memberNameMax-memberPartMax-3) + "/n"
	if _, err := os.Lstat(adopting(m.dir.path(longest))); errors.Is(err, syscall.ENAMETOOLONG) {
		return fmt.Errorf("store: this computer cannot be a member: the path %s is too long for a name of %d bytes under it", m.dir.root, memberNameMax)
	}
	return nil
}

// fits says whether a change's condition holds on the leader's copy. The
// name is held, so nothing the leader has put in the log for it is still to
// be applied.
func (m *Member) fits(name string, how How) (bool, error) {
	switch how.kind {
	case onlyIfNew:
		return !exists(m.dir.path(name)), nil
	case onlyIfUnchanged:
		// A name that holds nothing has changed since it was read.
		if how.mark == None {
			return false, nil
		}
		mark, err := m.dir.markAt(name)
		return mark == how.mark, err
	}
	return true, nil
}

func (m *Member) staged(id string) string { return filepath.Join(m.log, "staged", id) }

// stage takes a value in as a file of its own, safely on disk, and gives
// the name it is kept under and its mark.
func (m *Member) stage(ctx context.Context, r io.Reader) (string, Mark, error) {
	id := newID()
	h := sha256.New()
	tmp, err := writeTemp(filepath.Join(m.log, "staged"), func(w io.Writer) error {
		piece := make([]byte, 256<<10)
		for {
			n, rerr := r.Read(piece)
			if rerr != nil && rerr != io.EOF {
				return rerr
			}
			turn, err := startChange(ctx)
			if err != nil {
				return err
			}
			_, err = w.Write(piece[:n])
			turn()
			h.Write(piece[:n])
			if err != nil || rerr == io.EOF {
				return err
			}
		}
	})
	if err != nil {
		return "", None, err
	}
	if err := rename(tmp, m.staged(id)); err != nil {
		os.Remove(tmp)
		return "", None, err
	}
	return id, Mark(hex.EncodeToString(h.Sum(nil)[:12])), syncDir(filepath.Join(m.log, "staged"))
}

// spread sends a value ahead to the other members, and comes back once a
// majority of the members have it on disk. The rest go on being sent it.
func (m *Member) spread(id string) error {
	m.mu.Lock()
	need := len(m.voters)/2 + 1
	if slices.Contains(m.voters, m.id) {
		need--
	}
	type other struct {
		address string
		voter   bool
	}
	var others []other
	for _, ids := range [][]uint64{m.voters, m.learners} {
		for _, id := range ids {
			if id != m.id && m.addrs[id] != "" {
				others = append(others, other{m.addrs[id], slices.Contains(m.voters, id)})
			}
		}
	}
	m.mu.Unlock()

	has := make(chan bool, len(others))
	for _, o := range others {
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
			defer cancel()
			err := m.sendStaged(ctx, o.address, id)
			has <- err == nil && o.voter
		}()
	}
	for range others {
		if need <= 0 {
			break
		}
		if <-has {
			need--
		}
	}
	if need > 0 {
		return &notLeader{}
	}
	return nil
}

func (m *Member) sendStaged(ctx context.Context, address, id string) error {
	file, err := os.Open(m.staged(id))
	if err != nil {
		return err
	}
	defer file.Close()
	resp, err := m.call(ctx, address, http.MethodPost, "/member/stage?id="+id, file)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("store: the member at %s did not take the value: %s", address, resp.Status)
	}
	return nil
}

// propose puts a change in the log and waits for it to be applied here.
// The change's condition was looked at in the term the party was let in. A
// member that leads in a later term may not have led in between, and the
// leader of that time may have changed the name: the change is then not
// made, and the party asks again.
func (m *Member) propose(ctx context.Context, c change, term uint64) (bool, error) {
	c.Req = newID()
	data, err := json.Marshal(c)
	if err != nil {
		return false, err
	}
	return m.wait(ctx, c.Req, func() error {
		if m.hard.Term != term {
			return &notLeader{}
		}
		return m.rn.Propose(data)
	})
}

// wait has the loop put something in the log, and waits for the entry to
// be applied here. What the loop would not take was never in the log. What
// it took is waited for however long the caller would: whoever holds the
// entry's name holds it until the entry is applied, or until this member
// stops leading and the entry is another leader's to apply or drop.
func (m *Member) wait(ctx context.Context, req string, put func() error) (bool, error) {
	done := make(chan error, 1)
	m.mu.Lock()
	m.waiting[req] = done
	m.mu.Unlock()
	if err := m.onLoop(ctx, put); err != nil {
		m.mu.Lock()
		delete(m.waiting, req)
		m.mu.Unlock()
		if errors.Is(err, errStopped) || ctx.Err() != nil {
			return false, err
		}
		return false, &notLeader{}
	}
	select {
	case err := <-done:
		return err == nil, err
	case <-m.stopped:
		return false, errLost
	}
}

// onLoop has the loop do something, and gives what it said.
func (m *Member) onLoop(ctx context.Context, f func() error) error {
	done := make(chan error, 1)
	select {
	case m.do <- func() { done <- f() }:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stop:
		return errStopped
	}
	select {
	case err := <-done:
		return err
	case <-m.stopped:
		return errStopped
	}
}

// later has the loop do something when it next can.
func (m *Member) later(f func()) {
	select {
	case m.do <- f:
	case <-m.stop:
	}
}

// confirmed has a majority of the members confirm that this member still
// leads, and waits until it has applied all that was in the log by then.
func (m *Member) confirmed() error {
	id := newID()
	at := make(chan uint64, 1)
	m.mu.Lock()
	m.reads[id] = at
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.reads, id)
		m.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*memberElection*memberTick)
	defer cancel()
	if err := m.onLoop(ctx, func() error { m.rn.ReadIndex([]byte(id)); return nil }); err != nil {
		return &notLeader{}
	}
	select {
	case index, ok := <-at:
		if !ok {
			return &notLeader{}
		}
		return m.appliedTo(ctx, index)
	case <-ctx.Done():
		return &notLeader{}
	case <-m.stop:
		return errStopped
	}
}

// appliedTo waits until the member has applied the log as far as index.
func (m *Member) appliedTo(ctx context.Context, index uint64) error {
	for {
		m.mu.Lock()
		applied := m.applied
		m.mu.Unlock()
		if applied >= index {
			return nil
		}
		select {
		case <-ctx.Done():
			return &notLeader{}
		case <-m.stop:
			return errStopped
		case <-time.After(time.Millisecond):
		}
	}
}

// A confirmer has several reads share one confirmation: a read that comes
// while one is being made waits for the next, which begins after it came.
type confirmer struct {
	mu        sync.Mutex
	now, next *confirmation
}

type confirmation struct {
	done chan struct{}
	err  error
}

func (c *confirmer) do(ctx context.Context, confirm func() error) error {
	c.mu.Lock()
	if c.next == nil {
		c.next = &confirmation{done: make(chan struct{})}
	}
	mine := c.next
	if c.now == nil {
		c.begin(confirm)
	}
	c.mu.Unlock()
	select {
	case <-mine.done:
		return mine.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// begin starts the next confirmation. The confirmer is held.
func (c *confirmer) begin(confirm func() error) {
	c.now, c.next = c.next, nil
	now := c.now
	go func() {
		now.err = confirm()
		close(now.done)
		c.mu.Lock()
		c.now = nil
		if c.next != nil {
			c.begin(confirm)
		}
		c.mu.Unlock()
	}()
}

// keyed holds names, one holder at a time each.
type keyed struct {
	mu   sync.Mutex
	held map[string]*keyHold
}

type keyHold struct {
	sync.Mutex
	waiting int
}

// lock holds the names, and gives what lets them go.
func (k *keyed) lock(names ...string) func() {
	slices.Sort(names)
	names = slices.Compact(names)
	for _, name := range names {
		k.mu.Lock()
		if k.held == nil {
			k.held = map[string]*keyHold{}
		}
		h := k.held[name]
		if h == nil {
			h = &keyHold{}
			k.held[name] = h
		}
		h.waiting++
		k.mu.Unlock()
		h.Lock()
	}
	return func() {
		for _, name := range names {
			k.mu.Lock()
			h := k.held[name]
			if h.waiting--; h.waiting == 0 {
				delete(k.held, name)
			}
			k.mu.Unlock()
			h.Unlock()
		}
	}
}

// call asks something of another member.
func (m *Member) call(ctx context.Context, address, method, route string, body io.Reader) (*http.Response, error) {
	if cut := memberCut.Load(); cut != nil && (*cut)(m.self, address) {
		return nil, fmt.Errorf("%w: the member at %s", ErrUnreachable, address)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+address+route, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headFrom, strconv.FormatUint(m.id, 10))
	req.Header.Set(headAt, m.self)
	return m.client.Do(req)
}

// others gives the other members' addresses, the leader's first.
func (m *Member) others() []string {
	m.mu.Lock()
	var addresses []string
	if lead := m.addrs[m.lead]; lead != "" && m.lead != m.id {
		addresses = append(addresses, lead)
	}
	for id, address := range m.addrs {
		if id != m.id && id != m.lead {
			addresses = append(addresses, address)
		}
	}
	m.mu.Unlock()
	recorded, _ := AddressesOf(m.dir.root)
	for _, address := range recorded {
		if address != m.self && !slices.Contains(addresses, address) {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// askLeader asks something of the member that leads, going by what each
// member it asks says of who that is.
func (m *Member) askLeader(ctx context.Context, route string) error {
	var last error = errors.New("store: no other member is known")
	tried := map[string]bool{}
	addresses := m.others()
	for len(addresses) > 0 {
		address := addresses[0]
		addresses = addresses[1:]
		if tried[address] {
			continue
		}
		tried[address] = true
		resp, err := m.call(ctx, address, http.MethodPost, route, nil)
		if err != nil {
			last = fmt.Errorf("store: the member at %s: %w", address, withoutAddress(err))
			continue
		}
		said, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			return nil
		case http.StatusMisdirectedRequest:
			last = fmt.Errorf("%w: the member at %s does not lead the store", ErrUnreachable, address)
			if lead := resp.Header.Get(headLeader); lead != "" {
				addresses = append([]string{lead}, addresses...)
			}
		default:
			last = fmt.Errorf("store: the member at %s: %s", address, strings.TrimSpace(string(said)))
		}
	}
	return last
}

// join asks the members to take this computer, until one does.
func (m *Member) join() {
	for !m.isStopped() {
		m.mu.Lock()
		taken := slices.Contains(m.voters, m.id) || slices.Contains(m.learners, m.id)
		m.mu.Unlock()
		if taken {
			return
		}
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		m.unanswered(m.askLeader(ctx, "/member/join?"+url.Values{
			"id": {strconv.FormatUint(m.id, 10)}, "name": {m.name}, "address": {m.self}}.Encode()))
		cancel()
		select {
		case <-m.stop:
		case <-time.After(5 * memberTick):
		}
	}
}

// unanswered tells Held why the members did not do what this one asked.
// That no member leads yet is no fault: it is so for a moment at every
// start, and whoever waits for a majority is told of that.
func (m *Member) unanswered(err error) {
	if err != nil && !errors.Is(err, ErrUnreachable) {
		m.slow(err)
	}
}

// Change adds a member that is joining, counts one that has joined, or
// removes one. It is asked of the leader.
func (m *Member) change(ctx context.Context, kind pb.ConfChangeType, id uint64, name, address string) error {
	m.changes.Lock()
	defer m.changes.Unlock()
	m.mu.Lock()
	voter, learner := slices.Contains(m.voters, id), slices.Contains(m.learners, id)
	named := false // another member has the name
	for other, has := range m.called {
		named = named || has == name && other != id
	}
	m.mu.Unlock()
	switch {
	case kind == pb.ConfChangeAddLearnerNode && (voter || named):
		// A member that counts and asks to join has lost its copy, and
		// with it what it had promised the others. A name is one member's.
		return errMember(name)
	case kind == pb.ConfChangeAddLearnerNode && learner,
		kind == pb.ConfChangeAddNode && voter,
		kind == pb.ConfChangeRemoveNode && !voter && !learner:
		return nil
	case kind == pb.ConfChangeAddNode && !learner:
		return fmt.Errorf("store: %s has not joined the store's members", name)
	}
	req := newID()
	about, err := json.Marshal(memberChange{req, name, address})
	if err != nil {
		return err
	}
	cc := &pb.ConfChange{Type: kind.Enum(), NodeId: &id, Context: about}
	_, err = m.wait(ctx, req, func() error { return m.rn.ProposeConfChange(cc) })
	return err
}

// errMember refuses a join under a name that is a member's.
func errMember(name string) error {
	return fmt.Errorf("store: %s is a member of the store already: corbie init store --remove %s on another member removes it, and this computer then joins", name, name)
}

// Remove takes the member of a name out of the store's members. It is
// asked of whichever member leads.
func (m *Member) Remove(ctx context.Context, name string) error {
	if name == m.name {
		return errors.New("store: a member is removed from another member")
	}
	return m.askLeader(ctx, "/member/remove?name="+url.QueryEscape(name))
}

// serve answers another member. Whoever asks has proved the key of the
// store's controller, which only a member holds.
func (m *Member) serve(w http.ResponseWriter, r *http.Request) {
	if !m.keeper.Equal(proven(r.TLS)) {
		http.Error(w, "only a member of the store asks this", http.StatusUnauthorized)
		return
	}
	m.heard(r)
	ctx, q := r.Context(), r.URL.Query()
	switch r.Method + " " + r.URL.Path {
	case "POST /member/raft":
		m.serveRaft(w, r)
	case "POST /member/stage":
		id := q.Get("id")
		if CheckPart(id) != nil {
			http.Error(w, "no such value", http.StatusBadRequest)
			return
		}
		tmp, err := writeTemp(filepath.Join(m.log, "staged"), func(w io.Writer) error {
			_, err := io.Copy(w, r.Body)
			return err
		})
		if err == nil {
			if err = rename(tmp, m.staged(id)); err == nil {
				err = syncDir(filepath.Join(m.log, "staged"))
			}
		}
		m.said(w, err)
	case "GET /member/staged":
		id := q.Get("id")
		if CheckPart(id) != nil {
			http.Error(w, "no such value", http.StatusBadRequest)
			return
		}
		m.sendFile(w, m.staged(id))
	case "GET /member/value":
		if !m.servedTo(ctx, w, q.Get("at")) {
			return
		}
		if err := checkName(q.Get("name")); err != nil {
			http.Error(w, "no such name", http.StatusBadRequest)
			return
		}
		m.sendFile(w, m.dir.path(q.Get("name")))
	case "GET /member/names":
		if !m.servedTo(ctx, w, q.Get("at")) {
			return
		}
		// The names are sent as each is read: a large directory takes
		// long to read, and whoever asks does not wait that long for the
		// first of it. A listing that fails is cut off, and so is not
		// taken for a whole one.
		names, err := m.dir.names(ctx, "")
		if err != nil {
			m.said(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		http.NewResponseController(w).Flush()
		for _, name := range names {
			listed, err := m.dir.marked(ctx, []string{name})
			if err != nil {
				m.slow(err)
				panic(http.ErrAbortHandler)
			}
			for _, l := range listed {
				fmt.Fprintf(w, "%s\t%s\n", l.Name, l.Mark)
			}
		}
	case "POST /member/join", "POST /member/promote", "POST /member/remove":
		m.serveChange(w, r)
	default:
		http.Error(w, "a member has no such call", http.StatusNotFound)
	}
}

// heard notes the address a member that asks something says it is reached
// at: a member tells the others its address with everything it asks, so
// one that starts at another address is found there.
func (m *Member) heard(r *http.Request) {
	id, _ := strconv.ParseUint(r.Header.Get(headFrom), 10, 64)
	at := r.Header.Get(headAt)
	if _, _, err := net.SplitHostPort(at); err != nil {
		return
	}
	m.mu.Lock()
	known := m.running && (slices.Contains(m.voters, id) || slices.Contains(m.learners, id))
	moved := known && m.addrs[id] != at
	if moved || m.running && len(m.voters)+len(m.learners) == 0 {
		m.addrs[id] = at
	}
	m.mu.Unlock()
	if moved {
		go m.later(func() {
			if err := m.noteAddresses(); err != nil {
				m.slow(err)
			}
		})
	}
}

func (m *Member) said(w http.ResponseWriter, err error) {
	var not *notLeader
	switch {
	case errors.As(err, &not):
		w.Header().Set(headLeader, not.leader)
		http.Error(w, not.Error(), http.StatusMisdirectedRequest)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// sendFile answers with a file, or that there is none. A file that is there
// and cannot be read is not answered as none, and one that cannot be read
// to its end is cut off: the member that asks deletes a name it is told
// holds nothing, and keeps what it is sent as the whole value.
func (m *Member) sendFile(w http.ResponseWriter, path string) {
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		var info os.FileInfo
		if info, err = file.Stat(); err == nil && info.IsDir() {
			err = os.ErrNotExist
		}
	}
	switch {
	case gone(err):
		http.Error(w, "nothing is there", http.StatusNotFound)
	case err != nil:
		m.slow(err)
		m.said(w, err)
	default:
		if _, err := io.Copy(w, file); err != nil {
			m.slow(err)
			panic(http.ErrAbortHandler)
		}
	}
}

// servedTo waits a moment for the member to have applied the log as far as
// the request says, and says whether it has.
func (m *Member) servedTo(ctx context.Context, w http.ResponseWriter, at string) bool {
	index, err := strconv.ParseUint(at, 10, 64)
	if err != nil {
		http.Error(w, "no such place in the log", http.StatusBadRequest)
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if m.appliedTo(ctx, index) != nil {
		http.Error(w, "this member has not applied that much of the log", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// serveChange answers a change of members, which only the leader makes. The
// first to join a directory with one member finds no members yet, and makes
// this computer the first.
func (m *Member) serveChange(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, _ := strconv.ParseUint(q.Get("id"), 10, 64)
	name, address := q.Get("name"), q.Get("address")
	kind := map[string]pb.ConfChangeType{
		"/member/join":    pb.ConfChangeAddLearnerNode,
		"/member/promote": pb.ConfChangeAddNode,
		"/member/remove":  pb.ConfChangeRemoveNode,
	}[r.URL.Path]
	switch kind {
	case pb.ConfChangeAddLearnerNode:
		if _, _, err := net.SplitHostPort(address); err != nil || id == 0 || CheckPart(name) != nil || name == unnamed {
			http.Error(w, "a member joins with its number, its name and its address, host:port", http.StatusBadRequest)
			return
		}
	case pb.ConfChangeRemoveNode:
		// A member is removed by its name, which the leader knows the
		// number of.
		m.mu.Lock()
		for other, has := range m.called {
			if has == name {
				id = other
			}
		}
		m.mu.Unlock()
	}
	if kind == pb.ConfChangeAddLearnerNode {
		// A join that is refused leaves a directory with one member as it
		// was.
		if id == m.id || name == m.name {
			m.said(w, errMember(name))
			return
		}
		if err := m.becomeFirst(); err != nil {
			m.said(w, err)
			return
		}
	}
	if _, err := m.store(); err != nil {
		m.said(w, err)
		return
	}
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()
	if !running {
		http.Error(w, "this store has one member", http.StatusConflict)
		return
	}
	m.said(w, m.change(r.Context(), kind, id, name, address))
}

// becomeFirst makes this computer the first member of a directory that had one:
// from here on the directory has a log, and only its members' servers
// change it.
func (m *Member) becomeFirst() error {
	m.changes.Lock()
	defer m.changes.Unlock()
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()
	if running || HasMembers(m.dir.root) {
		return nil
	}
	// From here on a party is answered through the log, and by the copy
	// as it is no more.
	m.mu.Lock()
	m.many = true
	m.mu.Unlock()
	state := memberState{Applied: 1, Compact: 1, CompactTerm: 1, ConfIndex: 1,
		Voters: []uint64{m.id}, Members: map[uint64]memberAt{m.id: {m.name, m.self}}}
	err := SetMembers(m.dir.root, []MemberAt{{Name: m.name, Address: m.self}})
	if err == nil {
		err = m.saveJSON("hard", memberHard{Term: 1})
	}
	if err == nil {
		err = m.saveJSON("applied", state)
	}
	if err == nil {
		err = m.begin()
	}
	if err != nil {
		// Nothing is in the log: the directory has one member still.
		LeaveMembers(m.dir.root)
		m.mu.Lock()
		m.many = false
		m.mu.Unlock()
	}
	return err
}

func (m *Member) saveJSON(name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeWhole(filepath.Join(m.log, name), data)
}
