package mcp

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

func start(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	name, err := plugin.Text(first, "the program")
	if err != nil {
		return nil, err
	}
	to := "exec:" + name
	var secrets []*plugin.Secret
	reveal := func(v any) (string, error) {
		if s, ok := v.(*plugin.Secret); ok {
			secrets = append(secrets, s)
		}
		return job.Reveal(v, to)
	}
	given, err := opt.List("args")
	if err != nil {
		return nil, err
	}
	args := make([]string, len(given))
	for i, arg := range given {
		if args[i], err = reveal(plugin.TextOf(arg)); err != nil {
			return nil, err
		}
	}
	vars, err := opt.Map("env")
	if err != nil {
		return nil, err
	}
	var env []string
	for name, value := range vars {
		text, err := reveal(value)
		if err != nil {
			return nil, err
		}
		env = append(env, name+"="+text)
	}
	dir, err := opt.Text("dir", "")
	if err != nil {
		return nil, err
	}
	if dir != "" {
		if dir, err = job.Path(dir, false); err != nil {
			return nil, err
		}
	}
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	cmd, err := job.Program(job.Context, name, plugin.ProgramOptions{Args: args, Env: env, Dir: dir, Secrets: secrets})
	if err != nil {
		return nil, err
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// What the program prints is read through a pipe of the plugin's own,
	// which is closed only once the program has ended and all it printed
	// has been read into the pipe.
	out, printed := io.Pipe()
	errors := &plugin.Whole{}
	cmd.Stdout, cmd.Stderr = printed, &plugin.Draining{To: errors}
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, plugin.Errorf(plugin.NotFound, "the program %q could not be started: %v", name, err)
	}
	p := &program{name: name, in: in, messages: make(chan *message, 64), ended: make(chan struct{}), errors: errors, kill: cmd.Process.Kill}
	go func() {
		cmd.Wait()
		printed.Close()
		close(p.ended)
	}()
	go p.read(out)
	s := &server{way: p, name: fmt.Sprintf("%q", name), limit: limit}
	job.OnEnd(func() { s.close() })
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	if err := s.agree(ctx); err != nil {
		s.close()
		return nil, s.failure(ctx, err)
	}
	return s.object(), nil
}

// program is the way to a server that is a program the plugin started:
// one message a line, written to what it reads and read from what it
// prints.
type program struct {
	name     string
	in       io.WriteCloser
	messages chan *message // each the program printed, until it ends
	ended    chan struct{} // closed when the program has ended
	errors   *plugin.Whole // what it printed as errors
	kill     func() error
}

// read takes each message the program prints. A line that is not one is
// passed over.
func (p *program) read(out io.Reader) {
	defer close(p.messages)
	lines := bufio.NewScanner(out)
	lines.Buffer(make([]byte, 64<<10), plugin.MaxWhole)
	for lines.Scan() {
		var m message
		if json.Unmarshal(lines.Bytes(), &m) == nil {
			p.messages <- &m
		}
	}
	// Whatever the program prints after a line too long to read is let
	// go, so that it is never held up.
	io.Copy(io.Discard, out)
}

func (p *program) write(m *message) error {
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := p.in.Write(append(line, '\n')); err != nil {
		return p.gone()
	}
	return nil
}

func (p *program) ask(ctx context.Context, m *message, _ map[string]string) (*message, error) {
	if err := p.write(m); err != nil {
		return nil, err
	}
	for {
		select {
		case got, open := <-p.messages:
			switch {
			case !open:
				return nil, p.gone()
			case got.Method != "" && got.ID != nil:
				// A server of an earlier version may ask the plugin
				// something: it is answered that it is there, and that the
				// plugin can do nothing else.
				answer := &message{JSONRPC: "2.0", ID: got.ID, Result: map[string]any{}}
				if got.Method != "ping" {
					answer = &message{JSONRPC: "2.0", ID: got.ID, Error: &rpcError{Code: methodNotFound, Message: "the client does not do " + got.Method}}
				}
				if err := p.write(answer); err != nil {
					return nil, err
				}
			case got.Method == "" && fmt.Sprint(got.ID) == fmt.Sprint(m.ID):
				return got, nil
			}
		case <-ctx.Done():
			p.write(&message{JSONRPC: "2.0", Method: "notifications/cancelled", Params: map[string]any{"requestId": m.ID}})
			return nil, ctx.Err()
		}
	}
}

func (p *program) tell(_ context.Context, m *message, _ map[string]string) error {
	return p.write(m)
}

// gone says the program has ended, with the end of what it printed as
// errors.
func (p *program) gone() error {
	select {
	case <-p.ended:
	case <-time.After(2 * time.Second):
		return plugin.Errorf(plugin.Failed, "the program %q stopped reading what it was sent", p.name)
	}
	said := strings.TrimSpace(p.errors.Text())
	if len(said) > 500 {
		said = "…" + said[len(said)-500:]
	}
	if said == "" {
		return plugin.Errorf(plugin.Failed, "the program %q ended", p.name)
	}
	return plugin.Errorf(plugin.Failed, "the program %q ended, and printed: %s", p.name, said)
}

// close stops the program: its input is closed, and if it has not ended
// two seconds later it is stopped.
func (p *program) close() {
	p.in.Close()
	select {
	case <-p.ended:
	case <-time.After(2 * time.Second):
		p.kill()
		<-p.ended
	}
	// What the program printed and no one asked for is let go, so that
	// what reads it ends.
	for range p.messages {
	}
}
