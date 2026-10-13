// Package net is the plugin for a raw connection, plain or encrypted.
package net

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	gonet "net"
	"os"
	"time"

	"github.com/zummed/corbie/plugin"
)

// wait is how long a call waits on the other side unless it says.
var wait = 30 * time.Second

const (
	// chunk is the most one read hands over unless it says.
	chunk = 64 << 10
	// maxLine is the longest line that is read.
	maxLine = 1 << 20
)

func init() {
	plugin.Register(&plugin.Module{Name: "net", Page: page, Calls: map[string]*plugin.Call{
		"connect": {Options: []string{"tls", "trust", "timeout"}, Do: connect},
	}})
}

// A connection is one open connection and what has arrived on it that the
// script has not read yet.
type connection struct {
	address, host string
	// conn is what is read and written: the plain connection, or the
	// encrypted one over it.
	conn      gonet.Conn
	encrypted bool
	unread    []byte
	closed    bool
}

func connect(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	address, err := plugin.Text(first, "the address, as host:port")
	if err != nil {
		return nil, err
	}
	host, _, err := gonet.SplitHostPort(address)
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "%q is not a host and a port", address)
	}
	encrypt, err := opt.Bool("tls", false)
	if err != nil {
		return nil, err
	}
	if opt.Has("trust") && !encrypt {
		return nil, plugin.Errorf(plugin.BadInput, "trust: is for an encrypted connection, which tls: true asks for")
	}
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	conn, err := job.Dial(ctx, address)
	if err != nil {
		return nil, err
	}
	c := &connection{address: address, host: host, conn: conn}
	if encrypt {
		if err := c.encrypt(job, opt, limit); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return &plugin.Object{Close: c.close, Calls: map[string]*plugin.Call{
		"read":     named("read", []string{"max", "timeout"}, c.read),
		"readLine": named("readLine", []string{"timeout"}, c.readLine),
		"write":    {Options: []string{"timeout"}, Do: c.write},
		"startTLS": named("startTLS", []string{"trust", "timeout"}, c.startTLS),
	}}, nil
}

// named makes a call that takes only named options. A script writes them
// where another call's first thing goes, and they are checked as any
// call's options are.
func named(name string, options []string, do func(*plugin.Job, plugin.Options) (any, error)) *plugin.Call {
	inner := &plugin.Call{Options: options, Do: func(job *plugin.Job, _ any, opt plugin.Options) (any, error) {
		return do(job, opt)
	}}
	return &plugin.Call{Do: func(job *plugin.Job, first any, _ plugin.Options) (any, error) {
		opt, ok := first.(map[string]any)
		if first != nil && !ok {
			return nil, plugin.Errorf(plugin.BadInput, "%s takes only named options, as %s({ timeout: 5 })", name, name)
		}
		return inner.Invoke(job, "connection", name, nil, opt)
	}}
}

// encrypt puts encryption over the connection, checking the other side's
// certificate against the name that was asked for.
func (c *connection) encrypt(job *plugin.Job, opt plugin.Options, limit time.Duration) error {
	config := &tls.Config{ServerName: c.host}
	if opt.Has("trust") {
		// Only the certificates the script gives are trusted.
		pem, _, err := job.Contents(opt["trust"])
		if err != nil {
			return err
		}
		if config.RootCAs, err = plugin.Certificates(pem); err != nil {
			return err
		}
	}
	secure := tls.Client(c.conn, config)
	c.conn.SetDeadline(time.Now().Add(limit))
	err := secure.Handshake()
	c.conn.SetDeadline(time.Time{})
	if err != nil {
		return c.failure(err, limit)
	}
	c.conn, c.encrypted = secure, true
	return nil
}

func (c *connection) failure(err error, limit time.Duration) error {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return plugin.Errorf(plugin.TimedOut, "%s did not answer within %s", c.address, limit)
	}
	return plugin.Errorf(plugin.Refused, "the connection to %s failed: %v", c.address, err)
}

func (c *connection) open() error {
	if c.closed {
		return plugin.Errorf(plugin.BadInput, "the connection to %s is closed", c.address)
	}
	return nil
}

// more waits for more to arrive. A wait that runs out of time ends the
// wait and not the connection, and what had arrived is kept. It gives
// io.EOF once the other side has closed.
func (c *connection) more(until time.Time, limit time.Duration) error {
	buf := make([]byte, chunk)
	c.conn.SetReadDeadline(until)
	n, err := c.conn.Read(buf)
	c.unread = append(c.unread, buf[:n]...)
	switch {
	case n > 0:
		return nil
	case errors.Is(err, io.EOF):
		return io.EOF
	}
	return c.failure(err, limit)
}

func (c *connection) read(job *plugin.Job, opt plugin.Options) (any, error) {
	most, err := opt.Int("max", chunk)
	if err != nil {
		return nil, err
	}
	if most < 1 {
		return nil, plugin.Errorf(plugin.BadInput, "max: is the most bytes to read, 1 or more")
	}
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	if err := c.open(); err != nil {
		return nil, err
	}
	if len(c.unread) == 0 {
		switch err := c.more(time.Now().Add(limit), limit); {
		case err == io.EOF:
			return nil, nil
		case err != nil:
			return nil, err
		}
	}
	n := min(most, len(c.unread))
	data := plugin.Bytes(bytes.Clone(c.unread[:n]))
	c.unread = c.unread[n:]
	return data, nil
}

func (c *connection) readLine(job *plugin.Job, opt plugin.Options) (any, error) {
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	if err := c.open(); err != nil {
		return nil, err
	}
	until := time.Now().Add(limit)
	for {
		if end := bytes.IndexByte(c.unread, '\n'); end >= 0 {
			line := string(bytes.TrimSuffix(c.unread[:end], []byte("\r")))
			c.unread = c.unread[end+1:]
			return line, nil
		}
		if len(c.unread) > maxLine {
			return nil, plugin.Errorf(plugin.Failed, "%s sent more than %d bytes with no end of line: read them as bytes", c.address, maxLine)
		}
		switch err := c.more(until, limit); {
		case err == io.EOF && len(c.unread) == 0:
			return nil, nil
		case err == io.EOF:
			// A last line with no line end is a line too.
			line := string(c.unread)
			c.unread = nil
			return line, nil
		case err != nil:
			return nil, err
		}
	}
}

func (c *connection) write(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	if err := c.open(); err != nil {
		return nil, err
	}
	var data []byte
	switch v := first.(type) {
	case nil:
		return nil, plugin.Errorf(plugin.BadInput, "write takes what to send first: text, bytes, a blob or a handle to a secret")
	case *plugin.Secret:
		// Whether the connection is encrypted is judged now, as the value
		// goes into it, not when it was opened.
		text, err := job.RevealOver(v, c.address, c.encrypted)
		if err != nil {
			return nil, err
		}
		data = []byte(text)
	default:
		if data, _, err = job.Contents(v); err != nil {
			return nil, err
		}
	}
	c.conn.SetWriteDeadline(time.Now().Add(limit))
	if _, err := c.conn.Write(data); err != nil {
		return nil, c.failure(err, limit)
	}
	return nil, nil
}

func (c *connection) startTLS(job *plugin.Job, opt plugin.Options) (any, error) {
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	if err := c.open(); err != nil {
		return nil, err
	}
	if c.encrypted {
		return nil, plugin.Errorf(plugin.BadInput, "the connection to %s is already encrypted", c.address)
	}
	if len(c.unread) > 0 {
		// They would be taken for part of the protected conversation.
		return nil, plugin.Errorf(plugin.BadInput, "%d bytes that arrived from %s in the clear are still unread: read them before the connection is encrypted", len(c.unread), c.address)
	}
	return nil, c.encrypt(job, opt, limit)
}

func (c *connection) close() error {
	if !c.closed {
		c.closed = true
		c.conn.Close()
	}
	return nil
}

const page = `net: this plugin opens a TCP connection, plain or encrypted with TLS, and reads and writes bytes and lines on it.

  net.connect(address, { tls, trust, timeout })    returns a connection, c, to host:port
  c.write(data, { timeout })                       sends text, bytes, a blob or a handle to a secret
  c.read({ max, timeout })                         returns the next bytes, or null
  c.readLine({ timeout })                          returns the next line as text, without its line end, or null
  c.startTLS({ trust, timeout })                   encrypts a plain connection
  c.close()                                        closes the connection

Options:
  tls       true encrypts from the first byte; default false
  trust     certificates, as PEM text, to trust in place of the computer's
  max       the most bytes a read returns; default 65536
  timeout   seconds, or text such as "5m"; default 30s

- read and readLine return null once the other side has closed and nothing is
  left.
- readLine returns a last line that has no line end.
- TLS always checks the other side's certificate against the host in address.
- For a protocol that starts plain and then upgrades, the script sends the
  protocol's request, reads the whole answer and calls startTLS.

Errors:
  timeout     the call waited past its timeout; after a read the connection
              stays open and no bytes are lost
  refused     the connection could not be opened or failed, or the certificate was rejected
  not_found   the host's name could not be resolved
  bad_input   startTLS was called while bytes that arrived unencrypted are unread
  failed      readLine read more than 1 MiB with no line end

Needs net:host for the host connected to. To write a handle, the job needs one
of these.
  secret:NAME@net:host               on an encrypted connection
  secret:NAME@net.unencrypted:host   on a plain connection too

This script reads a WHOIS answer line by line until the server closes.

    allow: [net:whois.iana.org]

    const c = net.connect("whois.iana.org:43")
    c.write("example.com\r\n")
    const lines = []
    for (let line = c.readLine(); line !== null; line = c.readLine()) lines.push(line)
    c.close()
    return lines
    // gives ["% IANA WHOIS server", "domain:       EXAMPLE.COM", "status:       ACTIVE"]

This script writes a secret to a connection encrypted from the first byte.

    allow: [net:queue.example.com, "secret:QUEUE_TOKEN@net:queue.example.com"]

    const c = net.connect("queue.example.com:6380", { tls: true })
    c.write(secrets.text` + "`AUTH ${secrets.get(\"QUEUE_TOKEN\")}\\r\\n`" + `)
    const answer = c.readLine()         // gives "+OK"
    c.close()
    return answer

This script upgrades a plain connection with STARTTLS.

    allow: [net:mail.example.com]

    const c = net.connect("mail.example.com:587")
    c.readLine()                        // gives "220 mail.example.com ready"
    c.write("STARTTLS\r\n")
    c.readLine()                        // gives "220 go ahead"
    c.startTLS()
    c.write("NOOP\r\n")
    c.read({ max: 64 }).length          // gives 8
    c.close()
`
