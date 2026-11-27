package store

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A directory can be served to other computers by the computer that holds it.
// The wire is the store's own calls, one request each:
//
//	GET  /read?name=                    the value, with its mark
//	GET  /part?name=&from=&length=      part of a value
//	POST /write?name=&how=&mark=        the body is the value
//	POST /copy?from=&to=
//	POST /delete?name=&how=&mark=
//	GET  /list?under=&first=            the names, one to a line, each with a tab and its mark
//	GET  /wait?under=                   answers when a name there is written
//
// A connection is made by a party with its own identity key, to the machine
// that holds the key of the store's keeper, the controller whose machine
// serves the directory. Each end proves its key as the connection is made, so
// what passes is private to the two and nobody between them can write,
// change what is asked, send it again, or answer.

const (
	headParty = "Corbie-Party"
	headMark  = "Corbie-Mark"
	headDone  = "Corbie-Done"
	// headMembers carries the members' addresses in every answer of a
	// member, so that a party has them as they are now.
	headMembers = "Corbie-Members"

	howNew       = "only-if-new"
	howOver      = "overwrite"
	howUnchanged = "only-if-unchanged"

	// servedWait is how long a wait is held before it answers that nothing
	// was written.
	servedWait = 25 * time.Second
	// servedValue is the most a value that replaces another may hold. A
	// value written only-if-new is of any size, and goes straight to disk.
	servedValue = 64 << 20
)

// servedWord is how often the server tells a party that its request has no
// answer yet. A party takes a server that sends nothing for servedSilence to
// be off.
var servedWord = time.Second

// Proof is what an end of a served directory's wire shows of itself as a
// connection is made: its identity key and nothing else. Whoever shows it is
// known to hold the key's other half.
func Proof(key ed25519.PrivateKey) (tls.Certificate, error) {
	bare := &x509.Certificate{SerialNumber: big.NewInt(1)}
	der, err := x509.CreateCertificate(rand.Reader, bare, bare, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// proven is the key the other end of a connection proved it holds.
func proven(c *tls.ConnectionState) crypto.PublicKey {
	if c == nil || len(c.PeerCertificates) == 0 {
		return nil
	}
	return c.PeerCertificates[0].PublicKey
}

// ServedListener takes the connections to a served directory: the keeper proves
// its key to each, and each has to prove a key of its own. Whose key that is
// the server asks at every request. No connection is taken up again from an
// earlier one, so nothing of a connection outlives it.
func ServedListener(l net.Listener, keeper tls.Certificate) net.Listener {
	return tls.NewListener(l, &tls.Config{
		Certificates:           []tls.Certificate{keeper},
		ClientAuth:             tls.RequireAnyClientCert,
		MinVersion:             tls.VersionTLS13,
		SessionTicketsDisabled: true,
	})
}

// A Server serves a directory to the parties of one controller.
type Server struct {
	// Directory is the directory served.
	Directory Store
	// Member is this computer's part in a directory with members. With it,
	// the server answers a party only while this member leads the others,
	// and otherwise says who does.
	Member *Member
	// Who says which party a name is and which key is its own. A name that
	// is no party of the controller's has no key.
	Who func(ctx context.Context, name string) (Party, ed25519.PublicKey, error)
	// Failed is told of what went wrong at this end. A party is told only
	// that something did: the error itself can name this computer's paths.
	Failed func(err error)
	// Timeout is how long a party may send nothing of a value it is sending.
	// None is no limit.
	Timeout time.Duration

	mu       sync.Mutex
	stop     sync.Once
	stopping chan struct{} // closed when the server is to stop
}

// Stop has every party that is waiting answered at once, so that a server
// being stopped is not kept up by those it has nothing to tell.
func (s *Server) Stop() { s.stop.Do(func() { close(s.stopped()) }) }

func (s *Server) stopped() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping == nil {
		s.stopping = make(chan struct{})
	}
	return s.stopping
}

// HTTP gives the server that takes the requests. A connection that sends
// nothing for the timeout, between two requests, is closed.
func (s *Server) HTTP() *http.Server {
	return &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: s.Timeout,
		ErrorLog: log.New(requestsOnly{log.Writer()}, "", log.LstdFlags)}
}

// requestsOnly drops what an HTTP server logs of a connection that ended
// before it proved a key: a computer that is no party, or one that only
// looked whether the server is there. It asked nothing of the store, and
// the line reads as a fault of the store.
type requestsOnly struct{ log io.Writer }

func (r requestsOnly) Write(line []byte) (int, error) {
	if bytes.Contains(line, []byte("TLS handshake error")) {
		return len(line), nil
	}
	return r.log.Write(line)
}

// an answer is what the server says to one request.
type servedAnswer struct {
	status int
	mark   Mark
	done   string
	leader string
	body   []byte
}

func said(status int, format string, args ...any) servedAnswer {
	return servedAnswer{status: status, body: fmt.Appendf(nil, format, args...)}
}

// notParty turns away a name that is no party and a party's name given
// with another key in the same words, so that neither says which names are
// parties.
func notParty(name string) servedAnswer {
	return said(http.StatusUnauthorized, "%s is no party of this store's controller, or the connection was not made with its key", name)
}

// unhurried is the value of a request, read with the server's timeout begun
// again at each read: a party that sends nothing for that long is cut off,
// and one that goes on sending has as long as its value needs.
type unhurried struct {
	value   io.ReadCloser
	conn    *http.ResponseController
	timeout time.Duration
}

func (u unhurried) Read(p []byte) (int, error) {
	u.conn.SetReadDeadline(time.Now().Add(u.timeout))
	n, err := u.value.Read(p)
	if err != nil {
		// Whatever the request then takes on this computer is not timed.
		u.conn.SetReadDeadline(time.Time{})
	}
	return n, err
}

func (u unhurried) Close() error { return u.value.Close() }

// interim sends a party the responses that come before a request's answer:
// 102 at each servedWord, so that the party knows the server is on, and 100
// when the server first reads a value the party waits to send. One response
// is written at a time, and none once the answer is.
type interim struct {
	mu   sync.Mutex
	w    http.ResponseWriter
	done bool
}

func (i *interim) send(status int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.done {
		i.w.WriteHeader(status)
	}
}

// begin sends 102 at each servedWord until the function it gives is called.
func (i *interim) begin() (end func()) {
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(servedWord)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				i.send(http.StatusProcessing)
			}
		}
	}()
	return func() {
		close(stop)
		i.mu.Lock()
		i.done = true
		i.mu.Unlock()
	}
}

// awaited is a value a party sends only once the server asks for it. The
// server asks at the first read.
type awaited struct {
	io.ReadCloser
	interim *interim
	asked   sync.Once
}

func (a *awaited) Read(p []byte) (int, error) {
	a.asked.Do(func() { a.interim.send(http.StatusContinue) })
	return a.ReadCloser.Read(p)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Timeout > 0 {
		r.Body = unhurried{r.Body, http.NewResponseController(w), s.Timeout}
	}
	if strings.HasPrefix(r.URL.Path, "/member/") && s.Member != nil {
		s.Member.serve(w, r)
		return
	}
	early := &interim{w: w}
	if r.Header.Get("Expect") != "" {
		r.Body = &awaited{ReadCloser: r.Body, interim: early}
	}
	a := func() servedAnswer {
		defer early.begin()()
		return s.answer(r)
	}()
	select {
	case <-s.stopped():
		// A server that is stopping answers what it has in hand and keeps
		// no connection open for more.
		w.Header().Set("Connection", "close")
	default:
	}
	w.Header().Set(headMark, string(a.mark))
	w.Header().Set(headDone, a.done)
	if a.status == http.StatusMisdirectedRequest {
		w.Header().Set(headLeader, a.leader)
	}
	if s.Member != nil {
		if at := s.Member.counted(); at != "" {
			w.Header().Set(headMembers, at)
		}
	}
	w.WriteHeader(a.status)
	w.Write(a.body)
}

func (s *Server) answer(r *http.Request) servedAnswer {
	ctx := r.Context()
	name := r.Header.Get(headParty)
	if CheckPart(name) != nil {
		return said(http.StatusUnauthorized, "the request names no party")
	}
	// A member that does not lead says who does before anything else: its
	// own copy may not yet hold a party the leader's does.
	directory := s.Directory
	if s.Member != nil {
		var err error
		if directory, err = s.Member.store(); err != nil {
			return s.failed(err)
		}
	}
	party, key, err := s.Who(ctx, name)
	if err != nil {
		return s.failed(err)
	}
	if key == nil {
		return notParty(name)
	}
	// The party is whoever holds the key of that name, and the connection
	// was made by whoever holds the key it proved.
	if !key.Equal(proven(r.TLS)) {
		return notParty(name)
	}

	held := held{directory, party}
	q := r.URL.Query()
	how := Overwrite
	switch q.Get("how") {
	case howNew:
		how = OnlyIfNew
	case howUnchanged:
		how = OnlyIfUnchanged(Mark(q.Get("mark")))
	}
	var a servedAnswer
	var done bool
	switch r.Method + " " + r.URL.Path {
	case "GET /read":
		a.body, a.mark, err = held.Read(ctx, q.Get("name"))
		if err == nil && a.mark == None {
			return said(http.StatusNotFound, "nothing is there")
		}
	case "GET /part":
		from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
		length, _ := strconv.ParseInt(q.Get("length"), 10, 64)
		if from < 0 || length < 0 || length > servedValue {
			return said(http.StatusUnprocessableEntity, "no such part")
		}
		a.body, err = held.ReadPart(ctx, q.Get("name"), from, length)
	case "POST /write":
		if how == OnlyIfNew {
			done, err = held.WriteFrom(ctx, q.Get("name"), r.Body)
			break
		}
		var value []byte
		if value, err = io.ReadAll(io.LimitReader(r.Body, servedValue+1)); err == nil {
			if len(value) > servedValue {
				return said(http.StatusRequestEntityTooLarge, "a value that replaces another holds at most %d bytes", servedValue)
			}
			done, err = held.Write(ctx, q.Get("name"), value, how)
		}
	case "POST /copy":
		done, err = held.Copy(ctx, q.Get("from"), q.Get("to"))
	case "POST /delete":
		done, err = held.Delete(ctx, q.Get("name"), how)
	case "GET /list":
		var listed []Listed
		if first := q.Get("first"); first != "" {
			n, nerr := strconv.Atoi(first)
			if nerr != nil || n < 0 {
				return said(http.StatusUnprocessableEntity, "no such number of names")
			}
			listed, err = held.First(ctx, q.Get("under"), n)
		} else {
			listed, err = held.List(ctx, q.Get("under"))
		}
		lines := make([]string, len(listed))
		for i, l := range listed {
			lines[i] = l.Name + "\t" + string(l.Mark)
		}
		a.body = []byte(strings.Join(lines, "\n"))
	case "GET /wait":
		done, err = s.wait(ctx, party, q.Get("under"), directory)
	default:
		return said(http.StatusNotFound, "a served directory has no such call")
	}
	switch {
	case errors.Is(err, ErrRefused):
		return saidAs(http.StatusForbidden, ErrRefused, err)
	case errors.Is(err, ErrName):
		return saidAs(http.StatusBadRequest, ErrName, err)
	case errors.Is(err, ErrNothing):
		return saidAs(http.StatusGone, ErrNothing, err)
	case errors.Is(err, os.ErrDeadlineExceeded):
		return said(http.StatusRequestTimeout, "the request sent nothing for %s", s.Timeout)
	case err != nil:
		return s.failed(err)
	}
	a.status = http.StatusOK
	if r.Method == http.MethodPost || r.URL.Path == "/wait" {
		a.done = strconv.FormatBool(done)
	}
	return a
}

// saidAs answers with an error of a kind both ends know: what is sent is
// what the error says beyond its kind, and the party puts the kind back.
func saidAs(status int, kind, err error) servedAnswer {
	_, after, _ := strings.Cut(err.Error(), kind.Error())
	return servedAnswer{status: status, body: []byte(after)}
}

// failed answers that something went wrong at this end, and tells this end
// what. A member that does not lead the others has done nothing, and says
// who leads; one that stopped leading with a change in the log does not
// know what became of it.
func (s *Server) failed(err error) servedAnswer {
	var not *notLeader
	switch {
	case errors.As(err, &not):
		a := said(http.StatusMisdirectedRequest, "this member does not lead the store")
		a.leader = not.leader
		return a
	case errors.Is(err, ErrUnknown):
		return said(http.StatusServiceUnavailable, "this member stopped leading before the change was answered")
	case errors.Is(err, ErrUnreachable):
		return said(http.StatusMisdirectedRequest, "this member has stopped")
	}
	if s.Failed != nil {
		s.Failed(err)
	}
	return said(http.StatusInternalServerError, "the served directory could not do what was asked; the computer that serves it says why")
}

// wait answers once a name under the path that the party may read is
// written, or after a while that nothing was. A party learns nothing of
// names it may not read, not even that they changed.
func (s *Server) wait(ctx context.Context, p Party, under string, served Store) (bool, error) {
	if err := checkUnder(under); err != nil {
		return false, err
	}
	var f *directory
	switch served := served.(type) {
	case *directory:
		f = served
	case leader:
		f = served.m.dir
	default:
		return false, fmt.Errorf("store: only a directory is served")
	}
	written, stop := f.noticeIf(under, func(name string) bool { return p.Allows(ActRead, name) })
	defer stop()
	held := time.After(servedWait)
	for {
		select {
		case <-written:
			return true, nil
		case <-held:
			return false, nil
		case <-s.stopped():
			return false, nil
		case <-ctx.Done():
			return false, nil
		case <-time.After(time.Second):
			// A member that has stopped leading has nothing more to tell.
			if s.Member != nil {
				if _, err := s.Member.store(); err != nil {
					return false, nil
				}
			}
		}
	}
}
