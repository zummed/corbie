package plugin

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
)

// A Job is what a plugin is given of the job its script runs in: the few
// ways out, which Corbie keeps. A plugin never opens anything by itself. It
// asks here for a way to connect, a path, a program or a variable; every
// layer of the job's permissions is checked, what was used is noted, and
// the thing is handed over or refused.
type Job struct {
	Context context.Context
	// Perms is every layer that applies to the job.
	Perms *permission.Set
	// Scratch is the job's own directory: a path with no leading slash is in
	// it.
	Scratch string
	// Wrapper is the exec wrapper: the runner's own command, put in front
	// of every program started.
	Wrapper []string
	// Path is where programs are looked for.
	SearchPath string
	// Secrets are the values of the secrets the job was sent, by name.
	Secrets map[string]string
	// Channel is the job's own channel, which every job has.
	Channel Channel
	// Noted, where it is set, is told each further form a secret's value
	// is about to leave in, before the value is handed over. The job's
	// runner searches what the job's process prints, which the process
	// cannot do for itself, and learns the forms this way.
	Noted func(form, names string)

	mu      sync.Mutex
	closers []func()
	forms   []maskForm // the forms the secrets' values are searched for in
}

// The Channel is a job's own way to its runner: what it emits, its blobs,
// the machines it defines, and the runner's variables it is allowed.
type Channel interface {
	Emit(name string, data any) error
	Variable(name string) (value string, set bool, err error)
	ReadBlob(ref blob.Ref, from, length int64) ([]byte, error)
	WriteBlob(typ string, size int64, contents io.Reader) (blob.Ref, error)
	Define(source string, files map[string]string) (blob.Ref, error)
}

// OnEnd has something done when the job ends, however it ends.
func (j *Job) OnEnd(do func()) {
	j.mu.Lock()
	j.closers = append(j.closers, do)
	j.mu.Unlock()
}

// End does what was left for the job's end.
func (j *Job) End() {
	j.mu.Lock()
	closers := j.closers
	j.closers = nil
	j.mu.Unlock()
	for i := len(closers) - 1; i >= 0; i-- {
		closers[i]()
	}
}

// Dial opens a connection, plain, for a plugin to use or to encrypt. The
// destination is checked by the name asked for and again by the address
// the name resolves to, and only an address that was checked is connected
// to. A plugin that is handed further addresses dials each the same way.
//
// With secrets, the connection is one they may travel over: each must be
// bound to this destination, as it must be when its value is asked for.
func (j *Job) Dial(ctx context.Context, address string, with ...*Secret) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, Errorf(BadInput, "%q is not a host and a port", address)
	}
	if err := permission.CheckHost(host); err != nil {
		return nil, Errorf(BadInput, "%v", err)
	}
	host = permission.NormalHost(host)
	named := net.JoinHostPort(host, port)
	if ip := net.ParseIP(host); ip != nil {
		if r := j.Perms.Check(permission.Net, named); r != nil {
			return nil, r
		}
		if r := j.bound(with, permission.Net+":"+named); r != nil {
			return nil, r
		}
		return j.connect(ctx, named)
	}
	// A denied name is refused before it is looked up, and no name is
	// looked up for a connection no list could allow.
	if r := j.Perms.Denied(permission.Net, named); r != nil {
		return nil, r
	}
	if !j.Perms.Peek(permission.Net, named) && !j.Perms.AllowsAnAddress() {
		return nil, j.Perms.Check(permission.Net, named)
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, Errorf(NotFound, "the host name %q could not be looked up: %v", host, err)
	}
	var refusal *permission.Refusal
	var last error
	for _, addr := range addrs {
		at := net.JoinHostPort(addr, port)
		// An address a layer denies is never connected to, and a name that
		// passes only by one of its addresses is connected to at that one.
		if j.Perms.Denied(permission.Net, at) != nil || !j.Perms.Peek(permission.Net, named, at) {
			if refusal == nil {
				refusal = j.Perms.Check(permission.Net, named, at)
			}
			continue
		}
		j.Perms.Check(permission.Net, named, at)
		if r := j.bound(with, permission.Net+":"+named); r != nil {
			return nil, r
		}
		conn, err := j.connect(ctx, at)
		if err == nil {
			return conn, nil
		}
		last = err
	}
	if last != nil {
		return nil, last
	}
	if refusal == nil {
		refusal = j.Perms.Check(permission.Net, named)
	}
	return nil, refusal
}

// lookup gives the addresses a name has. A name is looked up once for a
// connection, and what is connected to is what that gave.
var lookup = func(ctx context.Context, host string) ([]string, error) {
	return (&net.Resolver{PreferGo: true}).LookupHost(ctx, host)
}

// dial opens a connection to an address that was checked.
var dial = func(ctx context.Context, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, "tcp", address)
}

func (j *Job) connect(ctx context.Context, address string) (net.Conn, error) {
	conn, err := dial(ctx, address)
	if err != nil {
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, Errorf(TimedOut, "connecting to %s timed out", address)
		}
		return nil, Errorf(Refused, "could not connect to %s: %v", address, err)
	}
	j.OnEnd(func() { conn.Close() })
	return conn, nil
}

// bound checks that every secret in the handles may go to a destination,
// written as a permission is: "net:host:port", "exec:program" or
// "fs.write:/path". It is the one check of where a secret may go: a way
// out asked for with secrets makes it, and so does the asking for a value,
// so the two cannot disagree. A connection is a secret's destination by
// the host as it was asked for, and not by an address the name resolves
// to.
func (j *Job) bound(with []*Secret, destination string) *permission.Refusal {
	for _, s := range with {
		for _, name := range s.Names() {
			if r := j.Perms.Check(permission.Secret, name+"@"+destination); r != nil {
				return r
			}
		}
	}
	return nil
}

// inScratch gives a path with no leading slash as a place in the job's
// scratch directory. It is never taken from anywhere else: not from the root
// of the disk, and not from wherever the process happens to stand.
func (j *Job) inScratch(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if j.Scratch == "" {
		return "", Errorf(BadInput, "the job has no scratch directory, so %s, with no leading /, names no place", path)
	}
	return filepath.Join(j.Scratch, path), nil
}

// Path gives a file's real location, to read or to write. The path is
// checked where it really points, after links are followed.
func (j *Job) Path(path string, write bool) (string, error) {
	if path == "" {
		return "", Errorf(BadInput, "the path is empty")
	}
	path, err := j.inScratch(path)
	if err != nil {
		return "", err
	}
	real, err := permission.RealPath(path)
	if err != nil {
		return "", Errorf(BadInput, "%v", err)
	}
	kind := permission.Read
	if write {
		kind = permission.Write
	}
	if r := j.Perms.Check(kind, filepath.ToSlash(real)); r != nil {
		return "", r
	}
	return real, nil
}

// Entry gives the real location of a name in its directory, for the name
// itself to be removed. The directory is taken where it really is, and the
// name is not followed if it is a link: removing a link removes the link
// and not what it points to. It is checked against fs.write.
func (j *Job) Entry(path string) (string, error) {
	if path == "" {
		return "", Errorf(BadInput, "the path is empty")
	}
	path, err := j.inScratch(path)
	if err != nil {
		return "", err
	}
	directory, err := permission.RealPath(filepath.Dir(path))
	if err != nil {
		return "", Errorf(BadInput, "%v", err)
	}
	real := filepath.Join(directory, filepath.Base(path))
	if r := j.Perms.Check(permission.Write, filepath.ToSlash(real)); r != nil {
		return "", r
	}
	return real, nil
}

// MayRead says whether the job may read what is at a path, given as it
// really is, without noting it as used: a listing leaves out what the job
// may not read.
func (j *Job) MayRead(real string) bool {
	return j.Perms.Peek(permission.Read, filepath.ToSlash(real))
}

// Whole checks that the job is allowed the whole of a group: "network",
// "disk" or "all". It is for something a plugin starts that does its own
// reaching, which Corbie cannot hold to a scope: it may be had only where
// everything it could use is allowed, and its use is noted.
func (j *Job) Whole(group string) error {
	if r := j.Perms.CheckGroup(group); r != nil {
		return r
	}
	return nil
}

// Refusal gives the refusal inside an error, or nil. What a way out
// refused can come back to a plugin inside the error of a library it
// wraps, and the plugin passes the refusal on as it is.
func Refusal(err error) error {
	var refusal *permission.Refusal
	if errors.As(err, &refusal) {
		return refusal
	}
	return nil
}

// Directory makes a fresh directory inside the job's scratch directory, for a
// plugin's own working files. It goes with the scratch directory when the job
// ends.
func (j *Job) Directory(pattern string) (string, error) {
	dir, err := os.MkdirTemp(j.Scratch, pattern)
	if err != nil {
		return "", fileError(err, "the job's scratch directory")
	}
	return dir, nil
}

// File makes a fresh file inside the job's scratch directory, holding the
// contents, for a plugin to hand a program. Only the runner's user can read
// it, and it goes with the scratch directory when the job ends.
func (j *Job) File(pattern string, contents []byte) (string, error) {
	file, err := os.CreateTemp(j.Scratch, pattern)
	if err != nil {
		return "", fileError(err, "the job's scratch directory")
	}
	_, err = file.Write(contents)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fileError(err, "the job's scratch directory")
	}
	return file.Name(), nil
}

// ProgramOptions are what a started program is given.
type ProgramOptions struct {
	Args  []string
	Env   []string // name=value
	Dir   string
	Stdin io.Reader
	// Secrets are the secrets that go to the program, in its arguments, its
	// variables or its input.
	Secrets []*Secret
}

// Program starts a program. The program the script named is checked before
// it is looked for, and it is what is noted as used; the exec wrapper, if
// there is one, goes in front.
func (j *Job) Program(ctx context.Context, name string, opt ProgramOptions) (*exec.Cmd, error) {
	if name == "" {
		return nil, Errorf(BadInput, "the program has no name")
	}
	name, err := j.named(name)
	if err != nil {
		return nil, err
	}
	path := filepath.FromSlash(name)
	// The program is checked before it is looked for.
	if r := j.Perms.Check(permission.Exec, name); r != nil {
		return nil, r
	}
	if r := j.bound(opt.Secrets, permission.Exec+":"+name); r != nil {
		return nil, r
	}
	if !strings.ContainsRune(name, filepath.Separator) {
		if path = j.find(name); path == "" {
			return nil, Errorf(NotFound, "the program %q was not found", name)
		}
	}
	for _, variable := range opt.Env {
		if name, _, _ := strings.Cut(variable, "="); !variableName(name) {
			return nil, Errorf(BadInput, "%q is not a variable's name", name)
		}
	}
	argv := []string{path}
	env := append([]string{"PATH=" + j.SearchPath}, opt.Env...)
	if len(j.Wrapper) > 0 {
		// The wrapper is the runner's own, and is started with nothing a
		// script chose: a variable can change what any program loads. The
		// program's variables are handed to it inside, by env.
		wrapper := j.Wrapper[0]
		if !strings.ContainsRune(wrapper, filepath.Separator) {
			if wrapper = j.find(wrapper); wrapper == "" {
				return nil, Errorf(NotFound, "the runner's exec wrapper %q was not found", j.Wrapper[0])
			}
		}
		if len(opt.Env) > 0 {
			// env takes anything holding = for a variable, and what
			// follows for the program.
			if strings.Contains(path, "=") {
				return nil, Errorf(BadInput, "this runner wraps programs, and behind its wrapper a program whose path holds = cannot be given variables")
			}
			argv = append(append([]string{"env"}, opt.Env...), path)
		}
		argv = append(append([]string{wrapper}, j.Wrapper[1:]...), argv...)
		env = []string{"PATH=" + j.SearchPath, "CORBIE_SCRATCH=" + j.Scratch, "CORBIE_ALLOW=" + strings.Join(j.allowed(), "\n")}
	}
	argv = append(argv, opt.Args...)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Dir = opt.Dir
	if cmd.Dir == "" {
		cmd.Dir = j.Scratch
	}
	cmd.Stdin = opt.Stdin
	return cmd, nil
}

// named gives a program as it is checked and noted. One named by its path
// is taken where the path really points, as every path is, and one with no
// leading slash is in the scratch directory, never from the root of the disk.
func (j *Job) named(program string) (string, error) {
	if !strings.ContainsRune(program, filepath.Separator) {
		return program, nil
	}
	within, err := j.inScratch(program)
	if err != nil {
		return "", err
	}
	real, err := permission.RealPath(within)
	if err != nil {
		return "", Errorf(BadInput, "%v", err)
	}
	return filepath.ToSlash(real), nil
}

// find gives the path of a program named with no path, looked for where
// the runner looks for programs, or nothing.
func (j *Job) find(name string) string {
	for _, dir := range filepath.SplitList(j.SearchPath) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

// variableName says whether name can be the name of a variable: letters,
// digits and _, not starting with a digit.
func variableName(name string) bool {
	for i, c := range name {
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return name != ""
}

// allowed lists what the job's own definition allows, for the exec wrapper
// to use if it wants.
func (j *Job) allowed() []string {
	if len(j.Perms.Layers) == 0 {
		return nil
	}
	return j.Perms.Layers[0].Allow
}

// Variable gives the value of one of the runner's variables.
func (j *Job) Variable(name string) (string, bool, error) {
	if name == "" {
		return "", false, Errorf(BadInput, "the variable has no name")
	}
	if r := j.Perms.Check(permission.Env, name); r != nil {
		return "", false, r
	}
	return j.Channel.Variable(name)
}

// Reveal gives the real text of a value that may hold secrets, for the
// program or the file it is going into: "exec:program" or
// "fs.write:/path". A plugin calls it only together with the way out the
// value is going into. Every secret in the value must be bound to that
// destination, and if one is not, nothing is given.
func (j *Job) Reveal(value any, destination string) (string, error) {
	return j.reveal(value, destination, false)
}

// RevealOver gives the real text of a value that may hold secrets, for the
// connection it is going into, to an address written host:port. encrypted
// is the plugin's word that the connection is encrypted at this moment,
// and that nothing it wraps can fall back to a plain one. Without the
// word, a secret goes only when its entry lets it go to the host over a
// connection that is not encrypted, or anywhere.
func (j *Job) RevealOver(value any, address string, encrypted bool) (string, error) {
	return j.reveal(value, permission.Net+":"+address, encrypted)
}

func (j *Job) reveal(value any, destination string, encrypted bool) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case *Secret:
		kind, address, _ := strings.Cut(destination, ":")
		if kind == permission.Exec {
			// A program is named here as it is when it is started.
			program, err := j.named(address)
			if err != nil {
				return "", err
			}
			destination = kind + ":" + program
		}
		if kind == permission.Net && !encrypted {
			// A layer that denies the secret to this host denies it over
			// any connection.
			for _, name := range v.Names() {
				if r := j.Perms.Denied(permission.Secret, name+"@"+destination); r != nil {
					return "", r
				}
			}
			if r := j.bound([]*Secret{v}, permission.Unencrypted+":"+address); r != nil {
				r.Fix += "; the connection to " + address + " is not encrypted, and a secret bound with net: goes only over an encrypted one"
				return "", r
			}
			return v.reveal(j.Secrets, j.Note)
		}
		if r := j.bound([]*Secret{v}, destination); r != nil {
			return "", r
		}
		return v.reveal(j.Secrets, j.Note)
	}
	return "", Errorf(BadInput, "this should be text or a secret, and is %T", value)
}

// Sent says whether the job was sent a secret of a name.
func (j *Job) Sent(name string) bool {
	_, sent := j.Secrets[name]
	return sent
}

// Note has a text searched for like the other forms a secret's value
// leaves in: a text an encoding wrote with the secrets named in it. A form
// that was not known is passed on to Noted.
func (j *Job) Note(form, names string) {
	if len(form) < shortSecret {
		return
	}
	known := j.maskForms()
	j.mu.Lock()
	fresh := !slices.ContainsFunc(known, func(f maskForm) bool { return f.text == form })
	if fresh {
		j.forms = append(slices.Clone(known), maskForm{form, names})
		sort.SliceStable(j.forms, func(a, b int) bool { return len(j.forms[a].text) > len(j.forms[b].text) })
	}
	j.mu.Unlock()
	if fresh && j.Noted != nil {
		j.Noted(form, names)
	}
}

// Key gives a secret's value to be used as a key, to sign or to make a
// keyed hash. What comes out is ordinary data, so only a secret bound to
// anywhere may be used this way.
func (j *Job) Key(value any) ([]byte, error) {
	if b, ok := value.(Bytes); ok {
		return b, nil
	}
	text, err := j.reveal(value, "*", false)
	return []byte(text), err
}

// Mask takes every secret's value out of a text that is going back to the
// script, or out of the job: the value as it is and in the usual
// encodings, each replaced with the secret's name. It catches accidents,
// such as a server that echoes a header.
func (j *Job) Mask(text string) string {
	// The longest form is replaced first: a shorter one found inside a
	// longer one would leave the rest of the value showing.
	for _, f := range j.maskForms() {
		if strings.Contains(text, f.text) {
			text = strings.ReplaceAll(text, f.text, "[secret "+f.name+"]")
		}
	}
	return text
}

// maskForms gives the forms a secret of the job can leave in, longest
// first.
func (j *Job) maskForms() []maskForm {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.forms == nil {
		j.forms = maskForms(j.Secrets)
	}
	return j.forms
}

// Telling gives a way to emit a text under a name as it arrives, a piece
// at a time. The pieces are searched for secrets across their joins: the
// end of what has arrived is held back until enough has followed to know
// it is not the start of a value.
func (j *Job) Telling(name string) *Told {
	return &Told{job: j, name: name}
}

// Told is a text being emitted as it arrives.
type Told struct {
	job  *Job
	name string
	held string // what has arrived and not gone out
}

// Add takes the next piece, and emits what can no longer be part of a
// value: all but the end, as long as the longest form less one.
func (t *Told) Add(piece string) {
	t.held = t.job.Mask(t.held + piece)
	cut := len(t.held)
	if forms := t.job.maskForms(); len(forms) > 0 {
		cut -= len(forms[0].text) - 1
	}
	for cut > 0 && cut < len(t.held) && !utf8.RuneStart(t.held[cut]) {
		cut--
	}
	if cut > 0 {
		t.emit(t.held[:cut])
		t.held = t.held[cut:]
	}
}

// End emits what was held back: nothing more is coming.
func (t *Told) End() {
	t.emit(t.held)
	t.held = ""
}

func (t *Told) emit(text string) {
	if text != "" && t.job.Channel != nil {
		t.job.Channel.Emit(t.name, text)
	}
}

// maskForm is one form a secret's value can leave in.
type maskForm struct{ text, name string }

// shortSecret is the length below which a value is searched for only as
// it is: its encoded forms are a few letters that turn up in ordinary text.
const shortSecret = 8

// leastSecret is the length below which a value is not searched for at
// all: one or two characters turn up in every text, and replacing them
// spoils what the script reads.
const leastSecret = 3

func maskForms(secrets map[string]string) []maskForm {
	var forms []maskForm
	add := func(name, text string) {
		if text != "" {
			forms = append(forms, maskForm{text, name})
		}
	}
	for name, value := range secrets {
		if len(value) < leastSecret {
			continue
		}
		add(name, value)
		if len(value) < shortSecret {
			continue
		}
		add(name, url.QueryEscape(value))
		add(name, url.PathEscape(value))
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			add(name, enc.EncodeToString([]byte(value)))
		}
		if escaped, err := inJSON(value); err == nil {
			add(name, escaped)
		}
		// Each line of a value of several lines.
		if lines := strings.Split(value, "\n"); len(lines) > 1 {
			for _, line := range lines {
				if len(strings.TrimSpace(line)) >= shortSecret {
					add(name, strings.TrimSpace(line))
				}
			}
		}
	}
	sort.Slice(forms, func(a, b int) bool { return len(forms[a].text) > len(forms[b].text) })
	return forms
}
