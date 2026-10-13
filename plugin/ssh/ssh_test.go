package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// A machine is a stand-in for the other machine: what key it shows, what
// sign-in it takes, and how many connections and sign-ins it has seen.
type machine struct {
	hostKey ssh.Signer
	// What it takes from the user "deploy": a key, a password, or a
	// password as the answer to a question it asks.
	key      ssh.PublicKey
	password string
	answer   string

	connections, signIns atomic.Int32
}

func newKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	return signer, string(pem.EncodeToMemory(block))
}

func line(key ssh.PublicKey) string { return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) }

// start has the machine listen, and gives its address.
func (m *machine) start(t *testing.T) string {
	t.Helper()
	if m.hostKey == nil {
		m.hostKey, _ = newKey(t)
	}
	config := &ssh.ServerConfig{}
	config.AddHostKey(m.hostKey)
	taken := func(c ssh.ConnMetadata, ok bool) (*ssh.Permissions, error) {
		m.signIns.Add(1)
		if c.User() != "deploy" || !ok {
			return nil, errors.New("not taken")
		}
		return nil, nil
	}
	if m.key != nil {
		config.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return taken(c, bytes.Equal(key.Marshal(), m.key.Marshal()))
		}
	}
	if m.password != "" {
		config.PasswordCallback = func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			return taken(c, string(password) == m.password)
		}
	}
	if m.answer != "" {
		config.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			answers, err := ask("", "", []string{"Password: "}, []bool{false})
			return taken(c, err == nil && len(answers) == 1 && answers[0] == m.answer)
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			m.connections.Add(1)
			go serve(conn, config)
		}
	}()
	return l.Addr().String()
}

func serve(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()
	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	for asked := range channels {
		if asked.ChannelType() != "session" {
			asked.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		channel, requests, err := asked.Accept()
		if err != nil {
			return
		}
		go func() {
			defer channel.Close()
			for request := range requests {
				if request.Type != "exec" {
					request.Reply(false, nil)
					continue
				}
				var exec struct{ Command string }
				ssh.Unmarshal(request.Payload, &exec)
				request.Reply(true, nil)
				status := command(exec.Command, channel)
				channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
				return
			}
		}()
	}
}

// command is the few commands the stand-in knows.
func command(written string, channel ssh.Channel) uint32 {
	switch written {
	case "echo hello":
		io.WriteString(channel, "hello\n")
	case "cat":
		io.Copy(channel, channel)
	case "sleep":
		time.Sleep(3 * time.Second)
	default:
		io.WriteString(channel.Stderr(), written+": not found\n")
		return 127
	}
	return 0
}

func newJob(t *testing.T, secrets map[string]string, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}},
		Scratch: t.TempDir(),
		Secrets: secrets,
	}
	t.Cleanup(job.End)
	return job
}

func runOn(job *plugin.Job, address string, opt plugin.Options) (any, error) {
	module, _ := plugin.Find("ssh")
	return module.Calls["run"].Invoke(job, "ssh", "run", address, opt)
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// with gives a call's options, with some of them changed.
func with(opt plugin.Options, changes plugin.Options) plugin.Options {
	out := plugin.Options{}
	for name, value := range opt {
		out[name] = value
	}
	for name, value := range changes {
		if out[name] = value; value == nil {
			delete(out, name)
		}
	}
	return out
}

func Test_ssh_runs_a_command(t *testing.T) {
	signer, private := newKey(t)
	m := &machine{key: signer.PublicKey()}
	address := m.start(t)
	host, _, _ := net.SplitHostPort(address)
	// What the page says is needed, and nothing more.
	job := newJob(t, map[string]string{"BUILD_KEY": private}, "net:"+host, "secret:BUILD_KEY@net:"+host)
	opt := plugin.Options{"user": "deploy", "key": plugin.NewSecret("BUILD_KEY"), "hostKey": line(m.hostKey.PublicKey()), "command": "echo hello"}

	got, err := runOn(job, address, opt)
	want := map[string]any{"status": 0.0, "out": "hello\n", "err": ""}
	if err != nil || got.(map[string]any)["out"] != want["out"] || got.(map[string]any)["status"] != want["status"] || got.(map[string]any)["err"] != want["err"] {
		t.Fatalf("run gave %v, %v", got, err)
	}
	got, err = runOn(job, address, with(opt, plugin.Options{"command": "cat", "input": "fed in"}))
	if err != nil || got.(map[string]any)["out"] != "fed in" {
		t.Fatalf("run gave %v, %v", got, err)
	}
	// A command that ended badly is an answer, with its status.
	got, err = runOn(job, address, with(opt, plugin.Options{"command": "nothing"}))
	if err != nil || got.(map[string]any)["status"] != 127.0 || got.(map[string]any)["err"] != "nothing: not found\n" {
		t.Fatalf("run gave %v, %v", got, err)
	}
	if _, err := runOn(job, address, with(opt, plugin.Options{"user": "someone"})); code(err) != plugin.Refused {
		t.Fatalf("a user the machine does not take gave %v", err)
	}
	_, err = runOn(job, address, with(opt, plugin.Options{"port": 22.0}))
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"port"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
}

func Test_ssh_refused_without_permission(t *testing.T) {
	signer, private := newKey(t)
	m := &machine{key: signer.PublicKey()}
	address := m.start(t)
	host, _, _ := net.SplitHostPort(address)
	secrets := map[string]string{"BUILD_KEY": private}
	opt := plugin.Options{"user": "deploy", "key": plugin.NewSecret("BUILD_KEY"), "hostKey": line(m.hostKey.PublicKey()), "command": "echo hello"}
	for missing, job := range map[string]*plugin.Job{
		"net:" + address:                  newJob(t, secrets),
		"secret:BUILD_KEY@net:" + address: newJob(t, secrets, "net:"+host),
		"net:" + address + " ":            newJob(t, secrets, "secret:BUILD_KEY@net:"+host),
	} {
		_, err := runOn(job, address, opt)
		var refusal *permission.Refusal
		if !errors.As(err, &refusal) || refusal.What != strings.TrimSpace(missing) {
			t.Fatalf("run gave %v, not a refusal naming %s", err, missing)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if n := m.connections.Load(); n != 0 {
		t.Fatalf("the machine saw %d connections", n)
	}
}

func Test_plug_ssh_host_key_given_or_waived_outright(t *testing.T) {
	signer, private := newKey(t)
	other, _ := newKey(t)
	m := &machine{key: signer.PublicKey()}
	address := m.start(t)
	job := newJob(t, map[string]string{"BUILD_KEY": private}, "all")
	opt := plugin.Options{"user": "deploy", "key": plugin.NewSecret("BUILD_KEY"), "command": "echo hello"}
	shown := ssh.FingerprintSHA256(m.hostKey.PublicKey())

	// A call that gives no key fails, and nothing is connected to.
	if _, err := runOn(job, address, opt); code(err) != plugin.BadInput || !strings.Contains(err.Error(), "hostKey") {
		t.Fatalf("a call that gives no key gave %v", err)
	}
	if n := m.connections.Load(); n != 0 {
		t.Fatalf("the machine saw %d connections", n)
	}
	// The key that is given, as a key or as its fingerprint, is taken.
	for _, given := range []string{line(m.hostKey.PublicKey()), shown} {
		if got, err := runOn(job, address, with(opt, plugin.Options{"hostKey": given})); err != nil || got.(map[string]any)["out"] != "hello\n" {
			t.Fatalf("with the machine's key given as %s, run gave %v, %v", given, got, err)
		}
	}
	// Another key is not: the failure shows the fingerprint of the key
	// that was shown, and no sign-in was sent.
	before := m.signIns.Load()
	for _, given := range []string{line(other.PublicKey()), ssh.FingerprintSHA256(other.PublicKey())} {
		_, err := runOn(job, address, with(opt, plugin.Options{"hostKey": given}))
		if code(err) != plugin.Refused || !strings.Contains(err.Error(), shown) {
			t.Fatalf("with another key given, run gave %v, which does not show %s", err, shown)
		}
	}
	if n := m.signIns.Load(); n != before {
		t.Fatalf("a sign-in was sent to a machine that showed another key")
	}
	if _, err := runOn(job, address, with(opt, plugin.Options{"hostKey": "the usual one"})); code(err) != plugin.BadInput {
		t.Fatalf("a key that cannot be read gave %v", err)
	}
	if _, err := runOn(job, address, with(opt, plugin.Options{"hostKey": false})); code(err) != plugin.BadInput {
		t.Fatalf("hostKey: false gave %v", err)
	}
	// Only a call that says so outright goes ahead unchecked.
	if got, err := runOn(job, address, with(opt, plugin.Options{"hostKey": "unchecked"})); err != nil || got.(map[string]any)["out"] != "hello\n" {
		t.Fatalf("unchecked, run gave %v, %v", got, err)
	}
}

func Test_plug_ssh_sign_in_as_handle_both_ways(t *testing.T) {
	signer, private := newKey(t)
	secrets := map[string]string{"BUILD_KEY": private, "BUILD_PASSWORD": "correct horse", "WRONG": "battery staple"}
	opt := plugin.Options{"user": "deploy", "hostKey": "unchecked", "command": "echo hello"}

	// Plain text is refused, and nothing is connected to.
	m := &machine{key: signer.PublicKey(), password: "correct horse"}
	address := m.start(t)
	job := newJob(t, secrets, "all")
	for name, written := range map[string]any{"key": private, "password": "correct horse"} {
		_, err := runOn(job, address, with(opt, plugin.Options{name: written}))
		if code(err) != plugin.BadInput || !strings.Contains(err.Error(), "handle") || strings.Contains(err.Error(), "correct horse") {
			t.Fatalf("%s written into the script gave %v", name, err)
		}
	}
	if _, err := runOn(job, address, opt); code(err) != plugin.BadInput {
		t.Fatalf("no sign-in gave %v", err)
	}
	both := plugin.Options{"key": plugin.NewSecret("BUILD_KEY"), "password": plugin.NewSecret("BUILD_PASSWORD")}
	if _, err := runOn(job, address, with(opt, both)); code(err) != plugin.BadInput {
		t.Fatalf("two sign-ins gave %v", err)
	}
	if n := m.connections.Load(); n != 0 {
		t.Fatalf("the machine saw %d connections", n)
	}

	// A password is taken as a password, and as the answer to a question.
	password := with(opt, plugin.Options{"password": plugin.NewSecret("BUILD_PASSWORD")})
	for way, m := range map[string]*machine{
		"as a password":               {password: "correct horse"},
		"as the answer to a question": {answer: "correct horse"},
	} {
		address := m.start(t)
		if got, err := runOn(job, address, password); err != nil || got.(map[string]any)["out"] != "hello\n" {
			t.Fatalf("a password taken %s: run gave %v, %v", way, got, err)
		}
		_, err := runOn(job, address, with(opt, plugin.Options{"password": plugin.NewSecret("WRONG")}))
		if code(err) != plugin.Refused || strings.Contains(err.Error(), "battery staple") {
			t.Fatalf("a wrong password taken %s gave %v", way, err)
		}
	}
	// A secret that holds no key is said to be that, and is not shown.
	_, err := runOn(job, address, with(opt, plugin.Options{"key": plugin.NewSecret("BUILD_PASSWORD")}))
	if code(err) != plugin.BadInput || strings.Contains(err.Error(), "correct horse") {
		t.Fatalf("a secret that is no key gave %v", err)
	}
}

func Test_plug_every_wait_has_a_deadline(t *testing.T) {
	wait = 200 * time.Millisecond
	t.Cleanup(func() { wait = time.Minute })
	signer, private := newKey(t)
	m := &machine{key: signer.PublicKey()}
	job := newJob(t, map[string]string{"BUILD_KEY": private}, "all")
	opt := plugin.Options{"user": "deploy", "key": plugin.NewSecret("BUILD_KEY"), "hostKey": "unchecked", "command": "sleep"}
	if _, err := runOn(job, m.start(t), opt); code(err) != plugin.TimedOut {
		t.Fatalf("a command that does not end, with no time given, gave %v", err)
	}

	// A machine that takes the connection and never says anything.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go io.Copy(io.Discard, conn)
		}
	}()
	if _, err := runOn(job, l.Addr().String(), opt); code(err) != plugin.TimedOut {
		t.Fatalf("a machine that says nothing, with no time given, gave %v", err)
	}
}

func Test_plug_wrapped_library_makes_no_lookup(t *testing.T) {
	// A name reaches the check as it was written, not looked up, with the
	// port a machine is reached on unless the address says another.
	opt := plugin.Options{"user": "deploy", "password": plugin.NewSecret("P"), "hostKey": "unchecked", "command": "uptime"}
	for address, want := range map[string]string{
		"build.corbie.invalid":      "net:build.corbie.invalid:22",
		"Build.Corbie.invalid:2222": "net:build.corbie.invalid:2222",
	} {
		_, err := runOn(newJob(t, map[string]string{"P": "p"}), address, opt)
		var refusal *permission.Refusal
		if !errors.As(err, &refusal) || refusal.What != want {
			t.Fatalf("running on %s gave %v, not a refusal naming %s", address, err, want)
		}
	}
}
