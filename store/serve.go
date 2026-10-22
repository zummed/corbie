package store

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
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

	howNew       = "only-if-new"
	howOver      = "overwrite"
	howUnchanged = "only-if-unchanged"

	// servedWait is how long a wait is held before it answers that nothing
	// was written.
	servedWait = 25 * time.Second
	// servedValue is the most a value that replaces another may hold: a
	// kept value of 64 MiB with its nonce and tag. A value written
	// only-if-new is of any size, and goes straight to disk.
	servedValue = 64<<20 + chacha20poly1305.NonceSizeX + chacha20poly1305.Overhead
)

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
	// Who says which party a name is and which key is its own. A name that
	// is no party of the controller's has no key.
	Who func(ctx context.Context, name string) (Party, ed25519.PublicKey, error)
	// Failed is told of what went wrong at this end. A party is told only
	// that something did: the error itself can name this computer's paths.
	Failed func(err error)

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

// an answer is what the server says to one request.
type servedAnswer struct {
	status int
	mark   Mark
	done   string
	body   []byte
}

func said(status int, format string, args ...any) servedAnswer {
	return servedAnswer{status: status, body: fmt.Appendf(nil, format, args...)}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := s.answer(r)
	select {
	case <-s.stopped():
		// A server that is stopping answers what it has in hand and keeps
		// no connection open for more.
		w.Header().Set("Connection", "close")
	default:
	}
	w.Header().Set(headMark, string(a.mark))
	w.Header().Set(headDone, a.done)
	w.WriteHeader(a.status)
	w.Write(a.body)
}

func (s *Server) answer(r *http.Request) servedAnswer {
	ctx := r.Context()
	name := r.Header.Get(headParty)
	if CheckPart(name) != nil {
		return said(http.StatusUnauthorized, "the request names no party")
	}
	party, key, err := s.Who(ctx, name)
	if err != nil {
		return s.failed(err)
	}
	if key == nil {
		return said(http.StatusUnauthorized, "%s is no party of this store's controller", name)
	}
	// The party is whoever holds the key of that name, and the connection
	// was made by whoever holds the key it proved.
	if !key.Equal(proven(r.TLS)) {
		return said(http.StatusUnauthorized, "the connection was not made with the key of %s", name)
	}

	held := held{s.Directory, party}
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
		done, err = s.wait(ctx, party, q.Get("under"))
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
// what.
func (s *Server) failed(err error) servedAnswer {
	if s.Failed != nil {
		s.Failed(err)
	}
	return said(http.StatusInternalServerError, "the served directory could not do what was asked; the computer that serves it says why")
}

// wait answers once a name under the path that the party may read is
// written, or after a while that nothing was. A party learns nothing of
// names it may not read, not even that they changed.
func (s *Server) wait(ctx context.Context, p Party, under string) (bool, error) {
	if err := checkUnder(under); err != nil {
		return false, err
	}
	f, ok := s.Directory.(*directory)
	if !ok {
		return false, fmt.Errorf("store: only a directory is served")
	}
	written, stop := f.noticeIf(func(name string) bool { return within(name, under) && p.Allows(ActRead, name) })
	defer stop()
	select {
	case <-written:
		return true, nil
	case <-time.After(servedWait):
		return false, nil
	case <-s.stopped():
		return false, nil
	case <-ctx.Done():
		return false, nil
	}
}
