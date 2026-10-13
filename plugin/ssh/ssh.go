// Package ssh is the plugin for running a command on another machine.
package ssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zummed/corbie/plugin"
)

// wait is how long a call may take unless it says.
var wait = time.Minute

func init() {
	plugin.Register(&plugin.Module{Name: "ssh", Page: page, Calls: map[string]*plugin.Call{
		"run": {Options: []string{"user", "key", "password", "command", "input", "hostKey", "timeout", "as"}, Do: run},
	}})
}

func run(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	address, err := plugin.Text(first, "the machine's address")
	if err != nil {
		return nil, err
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "22")
	}
	user, err := opt.Text("user", "")
	if err != nil {
		return nil, err
	}
	command, err := opt.Text("command", "")
	if err != nil {
		return nil, err
	}
	if user == "" || command == "" {
		return nil, plugin.Errorf(plugin.BadInput, "ssh.run needs user: and command:")
	}
	input, _, err := job.Contents(opt["input"])
	if err != nil {
		return nil, err
	}
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	check, algorithms, err := hostKey(opt)
	if err != nil {
		return nil, err
	}
	if opt.Has("key") == opt.Has("password") {
		return nil, plugin.Errorf(plugin.BadInput, "ssh.run needs key: or password:, one of them")
	}
	signIn := opt["password"]
	if opt.Has("key") {
		signIn = opt["key"]
	}
	handle, isHandle := signIn.(*plugin.Secret)
	if !isHandle {
		return nil, plugin.Errorf(plugin.BadInput, "a key or a password is taken only as a handle to a secret, as secrets.get(\"NAME\") gives: one written into a script is in the record for every reader of it")
	}

	// The limit covers the whole of the call. When it runs out, the
	// connection is closed under whatever is waiting on it.
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	conn, err := job.Dial(ctx, address, handle)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	failure := func(err error) error {
		var known *plugin.Error
		switch {
		case errors.As(err, &known):
			return known
		case ctx.Err() == context.DeadlineExceeded:
			return plugin.Errorf(plugin.TimedOut, "%s did not finish within %s", address, limit)
		}
		return plugin.Errorf(plugin.Refused, "%s: %v", address, err)
	}

	// The connection is encrypted by ssh itself, which has no plain way to
	// fall back to: every sign-in goes inside it.
	secret, err := job.RevealOver(handle, address, true)
	if err != nil {
		return nil, err
	}
	// Many servers take a password only as the answer to a question they
	// ask, so it is offered both ways.
	auth := []ssh.AuthMethod{ssh.Password(secret), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = secret
		}
		return answers, nil
	})}
	if opt.Has("key") {
		signer, err := ssh.ParsePrivateKey([]byte(secret))
		if err != nil {
			return nil, plugin.Errorf(plugin.BadInput, "key: the secret is not a private key that can be read: it is one in PEM text, with no passphrase")
		}
		auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	}
	// Nothing is taken from the machine: no keys, no agent, no list of
	// known hosts.
	link, channels, requests, err := ssh.NewClientConn(conn, address, &ssh.ClientConfig{
		User: user, Auth: auth, HostKeyCallback: check, HostKeyAlgorithms: algorithms,
	})
	if err != nil {
		return nil, failure(err)
	}
	client := ssh.NewClient(link, channels, requests)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return nil, failure(err)
	}
	defer session.Close()
	form, err := opt.Text("as", "text")
	if err != nil {
		return nil, err
	}
	// What the command prints is handed back whole, or goes into a blob as
	// it is printed. Either way the command is never held up.
	errOut := &plugin.Whole{}
	out, printed := job.Give(form, "text/plain", "what the command printed", func(w io.Writer) error {
		stdout := &plugin.Draining{To: w}
		session.Stdin, session.Stdout, session.Stderr = bytes.NewReader(input), stdout, &plugin.Draining{To: errOut}
		err = session.Run(command)
		return stdout.Err
	})
	status := 0
	var exit *ssh.ExitError
	switch {
	case errors.As(err, &exit):
		// A command that ran and ended badly is an answer, with its status.
		status = exit.ExitStatus()
	case err != nil:
		return nil, failure(err)
	}
	if printed != nil {
		return nil, printed
	}
	if errOut.Over {
		return nil, plugin.TooLarge("what the command printed as errors")
	}
	return map[string]any{"status": float64(status), "out": out, "err": errOut.Text()}, nil
}

// hostKey reads which key the other machine must show: a key, a key's
// fingerprint, or the word that says outright that it is not checked.
func hostKey(opt plugin.Options) (ssh.HostKeyCallback, []string, error) {
	given, err := opt.Text("hostKey", "")
	if err != nil {
		return nil, nil, err
	}
	differs := func(shown ssh.PublicKey) error {
		return plugin.Errorf(plugin.Refused, "the machine showed a key that is not the one given: its key has changed, or it is another machine. The key it showed is %s %s", shown.Type(), ssh.FingerprintSHA256(shown))
	}
	switch {
	case given == "":
		return nil, nil, plugin.Errorf(plugin.BadInput, "ssh.run needs hostKey: the other machine's key, as \"ssh-ed25519 AAAA...\", or its fingerprint, as \"SHA256:...\"; or \"unchecked\", to take whatever key it shows")
	case given == "unchecked":
		return ssh.InsecureIgnoreHostKey(), nil, nil
	case strings.HasPrefix(given, "SHA256:"):
		return func(_ string, _ net.Addr, shown ssh.PublicKey) error {
			if ssh.FingerprintSHA256(shown) != given {
				return differs(shown)
			}
			return nil
		}, nil, nil
	}
	want, _, _, _, err := ssh.ParseAuthorizedKey([]byte(given))
	if err != nil {
		return nil, nil, plugin.Errorf(plugin.BadInput, "hostKey: is not a key that can be read: it is a line such as \"ssh-ed25519 AAAA...\", a fingerprint such as \"SHA256:...\", or \"unchecked\"")
	}
	// The machine is asked for the kind of key that was given, of the
	// several it may have.
	algorithms := []string{want.Type()}
	if want.Type() == ssh.KeyAlgoRSA {
		algorithms = []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return func(_ string, _ net.Addr, shown ssh.PublicKey) error {
		if !bytes.Equal(shown.Marshal(), want.Marshal()) {
			return differs(shown)
		}
		return nil
	}, algorithms, nil
}

const page = `ssh: this plugin runs a command on another computer over SSH.

  ssh.run(address, { user, key | password, command, input, hostKey, timeout, as })
      gives { status, out, err }

address is the host, with :port when the port is not 22. Each call signs in
and runs one command.

Options:
  user       the user to sign in as; required
  key        a handle to a secret that holds a private key: PEM text, no passphrase
  password   a handle to a secret that holds a password; give key or password, not both
  command    the command, which the other computer's shell reads; required
  input      the command's stdin: text, bytes or a blob
  hostKey    the other computer's public key ("ssh-ed25519 AAAA..."), its
             fingerprint ("SHA256:...") or "unchecked"; required
  timeout    for the whole call: seconds, or text such as "5m"; default 1m
  as         the form of out: "text", "bytes", "json" or "blob"; default "text"

Returns:
  status   the exit code; a code other than 0 is not an error
  out      stdout: at most 64 MiB, or a blob of any size, written to the job's
           scratch directory as the command prints it
  err      stderr, as text: at most 64 MiB

- The call refuses a key or a password that is not a handle.
- The plugin sends a password as a password and as the answer to a question
  the server asks.
- The call checks the other computer's key against hostKey before it signs
  in. When they differ, the error gives the fingerprint of the key shown.
- With hostKey: "unchecked" the call accepts any key.
- The plugin uses no SSH key, agent or known_hosts file of the runner's.

Errors:
  refused     the sign-in was rejected, the key shown is not hostKey, or the connection failed
  timeout     the call ran past its timeout
  not_found   the host's name could not be resolved
  bad_input   out or err is over 64 MiB

Needs net:host for the other computer, and secret:NAME@net:host for the key
or password.

This script runs uptime on a build server.

    allow: [net:build.example, "secret:BUILD_KEY@net:build.example"]

    const r = ssh.run("build.example", {
      user: "deploy",
      key: secrets.get("BUILD_KEY"),
      hostKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGL60EhV1iO6PGQF8k0yngzkvuC3Kim6+iSe9a5KF0aa",
      command: "uptime",
    })
    if (r.status !== 0) throw new Error(r.err)
    return r.out.trim()
    // gives "12:00:00 up 3 days,  2 users,  load average: 0.10, 0.12, 0.09"
`
