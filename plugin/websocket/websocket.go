// Package websocket is the plugin for a connection that carries messages
// both ways, kept alive while the script works.
package websocket

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"

	"github.com/zummed/corbie/plugin"
)

// wait is how long a call waits on the other side unless it says.
var wait = 30 * time.Second

// pingEvery is how often the other side is asked whether it is still
// there, which keeps what lies between from dropping a quiet connection.
var pingEvery = 30 * time.Second

const (
	// maxMessage is the largest message that is taken.
	maxMessage = plugin.MaxWhole
	// waiting is how many messages are held for the script to receive.
	waiting = 256
)

func init() {
	plugin.Register(&plugin.Module{Name: "websocket", Page: page, Calls: map[string]*plugin.Call{
		"connect": {Options: []string{"headers", "timeout"}, Do: connect},
	}})
}

// A socket is one open connection and the messages that have arrived on it
// that the script has not received yet.
type socket struct {
	conn     *websocket.Conn
	to       *url.URL
	messages chan message
	// ended is why messages stopped arriving. It is read only once
	// messages is closed.
	ended  error
	stop   context.CancelFunc
	closed bool
}

type message struct {
	kind websocket.MessageType
	data []byte
}

// own are the headers that open a connection, which are written here and
// are not a script's to set.
var own = []string{"Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Version"}

// connection is the connection an address is reached over: its host and
// port, and whether it is encrypted. Over wss it always is: nothing falls
// back to a plain connection.
func connection(u *url.URL) (address string, encrypted bool) {
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "ws" {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port), u.Scheme == "wss"
}

func connect(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	address, err := plugin.Text(first, "the address, starting ws:// or wss://")
	if err != nil {
		return nil, err
	}
	to, err := url.Parse(plugin.EscapedAddress(address))
	if err != nil || to.Host == "" || (to.Scheme != "ws" && to.Scheme != "wss") {
		return nil, plugin.Errorf(plugin.BadInput, "the call takes an address, starting ws:// or wss://, first")
	}
	if to.User != nil {
		return nil, plugin.Errorf(plugin.BadInput, "an address may not hold a user name or a password: send a sign-in as a header")
	}
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	headers, err := opt.Map("headers")
	if err != nil {
		return nil, err
	}
	over, encrypted := connection(to)
	header, err := job.Headers(headers, over, encrypted, own...)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             nil, // nothing of the machine's own settings is used
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return job.Dial(ctx, address)
			},
		},
		// A redirect would carry the headers to a place they were not
		// revealed for.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, to.String(), &websocket.DialOptions{HTTPClient: client, HTTPHeader: header})
	if err != nil {
		return nil, failure(err, to, limit)
	}
	conn.SetReadLimit(maxMessage)
	open, stop := context.WithCancel(job.Context)
	s := &socket{conn: conn, to: to, messages: make(chan message, waiting), stop: stop}
	go s.listen(open)
	go s.keepAlive(open, pingEvery)
	job.OnEnd(func() {
		stop()
		conn.CloseNow()
	})
	return &plugin.Object{Close: s.close, Calls: map[string]*plugin.Call{
		"send":    {Options: []string{"timeout"}, Do: s.send},
		"receive": named("receive", []string{"timeout"}, s.receive),
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

// failure gives a failed call the one shape of error. A refusal passes
// through as it is.
func failure(err error, to *url.URL, limit time.Duration) error {
	if refusal := plugin.Refusal(err); refusal != nil {
		return refusal
	}
	var known *plugin.Error
	if errors.As(err, &known) {
		return known
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return plugin.Errorf(plugin.TimedOut, "%s did not answer within %s", to.Host, limit)
	}
	// What is said of a failure names the host and no more of the address.
	var with *url.Error
	if errors.As(err, &with) {
		err = with.Err
	}
	return plugin.Errorf(plugin.Refused, "the connection to %s failed: %v", to.Host, err)
}

// listen takes messages as they arrive, so that the other side's pings are
// answered while the script does other things.
func (s *socket) listen(open context.Context) {
	defer close(s.messages)
	for {
		kind, data, err := s.conn.Read(open)
		if err != nil {
			s.ended = err
			return
		}
		select {
		case s.messages <- message{kind, data}:
		case <-open.Done():
			s.ended = open.Err()
			return
		}
	}
}

func (s *socket) keepAlive(open context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-open.Done():
			return
		case <-tick.C:
			// An answer that does not come is found by the script's next
			// call, not here.
			ctx, cancel := context.WithTimeout(open, every)
			s.conn.Ping(ctx)
			cancel()
		}
	}
}

func (s *socket) isOpen() error {
	if s.closed {
		return plugin.Errorf(plugin.BadInput, "the connection to %s is closed", s.to.Host)
	}
	return nil
}

func (s *socket) send(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	if err := s.isOpen(); err != nil {
		return nil, err
	}
	kind := websocket.MessageBinary
	var data []byte
	switch v := first.(type) {
	case string:
		kind, data = websocket.MessageText, []byte(v)
	case *plugin.Secret:
		over, encrypted := connection(s.to)
		text, err := job.RevealOver(v, over, encrypted)
		if err != nil {
			return nil, err
		}
		kind, data = websocket.MessageText, []byte(text)
	case plugin.Bytes, *plugin.Blob:
		if data, _, err = job.Contents(v); err != nil {
			return nil, err
		}
	default:
		return nil, plugin.Errorf(plugin.BadInput, "send takes a message first: text, bytes, a blob or a handle to a secret")
	}
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	if err := s.conn.Write(ctx, kind, data); err != nil {
		return nil, failure(err, s.to, limit)
	}
	return nil, nil
}

func (s *socket) receive(job *plugin.Job, opt plugin.Options) (any, error) {
	limit, err := opt.Duration("timeout", wait)
	if err != nil {
		return nil, err
	}
	if err := s.isOpen(); err != nil {
		return nil, err
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case m, more := <-s.messages:
		switch {
		case more && m.kind == websocket.MessageText:
			return string(m.data), nil
		case more:
			return plugin.Bytes(m.data), nil
		}
		if status := websocket.CloseStatus(s.ended); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
			return nil, nil
		}
		return nil, plugin.Errorf(plugin.Refused, "the connection to %s ended: %v", s.to.Host, s.ended)
	case <-timer.C:
		// The wait ends and not the connection: nothing is lost.
		return nil, plugin.Errorf(plugin.TimedOut, "nothing arrived from %s within %s", s.to.Host, limit)
	}
}

func (s *socket) close() error {
	if !s.closed {
		s.closed = true
		s.conn.Close(websocket.StatusNormalClosure, "")
		s.stop()
	}
	return nil
}

const page = `websocket: this plugin opens a WebSocket connection, which carries messages in both directions.

  websocket.connect(address, { headers, timeout })    returns a connection, ws
  ws.send(message, { timeout })                       sends text or a handle as text, bytes or a blob as binary
  ws.receive({ timeout })                             returns the next message as text or bytes, or null
  ws.close()                                          closes the connection

address is a URL that starts ws:// or wss://. The plugin percent-encodes its
spaces, its non-ASCII characters and the characters a path cannot contain, and
keeps an escape already in it, such as %2F.

Options:
  headers   headers for the request that opens the connection, sent as written;
            a value may be a handle to a secret
  timeout   seconds, or text such as "5m"; default 30s

- A script cannot set Host, Connection, Content-Length, Transfer-Encoding,
  Trailer, Upgrade, Sec-WebSocket-Key or Sec-WebSocket-Version.
- The plugin reads messages and answers pings while the script does other
  work, and pings the other side every 30 seconds.
- receive returns messages in the order they arrived, and null once the other
  side has closed the connection.
- The plugin keeps up to 256 unread messages, and reads no more until the
  script receives one.
- The plugin follows no redirect.

Errors:
  timeout     the call waited past its timeout; after a receive the connection
              stays open and no message is lost
  refused     the connection could not be opened, or it broke
  not_found   the host's name could not be resolved

Needs net:host for the host connected to. To send a handle in a header or a
message, the job needs one of these.
  secret:NAME@net:host               over wss://
  secret:NAME@net.unencrypted:host   over ws:// too

This script sends a token in a header, subscribes and returns the first message.

    allow: [net:feed.example.com, "secret:FEED_TOKEN@net:feed.example.com"]

    const ws = websocket.connect("wss://feed.example.com/v1", {
      headers: { Authorization: secrets.text` + "`Bearer ${secrets.get(\"FEED_TOKEN\")}`" + ` },
    })
    ws.send(JSON.stringify({ subscribe: "prices" }))
    const first = JSON.parse(ws.receive())
    ws.close()
    return first                        // gives { prices: { tea: 4.2, cake: 3 } }
`
