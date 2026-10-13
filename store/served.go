package store

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ServedScheme begins the address of a directory that another computer serves:
// corbie://host:port.
const ServedScheme = "corbie://"

// IsServed says whether an address names a directory another computer serves.
func IsServed(address string) bool { return strings.HasPrefix(address, ServedScheme) }

// served is a directory another computer serves, as one party sees it. Every
// connection to it is made with the party's own key, and only to the machine
// that proves it holds the keeper's.
type served struct {
	origin string
	party  string
	client *http.Client
}

// errKeeper is a machine that answers at a served directory's address and does
// not hold the key of the store's keeper.
var errKeeper = errors.New("store: the computer at this address does not hold the key of this store's controller")

// A request that got no answer is sent again, a little later each time,
// where sending it twice is the same as sending it once, and whatever it is
// where it was never sent at all. The tries go on for about as long as the
// serving machine's process takes to be stopped and started again, so that
// a restart there loses nothing that was in flight elsewhere.
var (
	servedPatience = 10 * time.Second
	servedPause    = 100 * time.Millisecond
	servedPauseMax = 2 * time.Second
	servedIdle     = 3 * time.Second
)

// OpenServed opens the directory served at an address as a party: proof is the
// party's own, and keeper is the key of the controller whose machine serves
// the directory.
func OpenServed(address, party string, proof tls.Certificate, keeper ed25519.PublicKey) (Store, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "corbie" || u.Host == "" || u.Port() == "" || u.User != nil || strings.Trim(u.Path, "/") != "" || u.RawQuery != "" {
		return nil, errors.New("store: a served directory's address reads corbie://host:port")
	}
	return &served{origin: "https://" + u.Host, party: party,
		client: &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{proof},
				MinVersion:   tls.VersionTLS13,
				// The other end is known by its key and by nothing else: no
				// name, no date and no one's word for it.
				InsecureSkipVerify: true,
				VerifyConnection: func(c tls.ConnectionState) error {
					if !keeper.Equal(proven(&c)) {
						return errKeeper
					}
					return nil
				},
			},
			MaxIdleConnsPerHost: 64,
			// A connection that has sat unused is not used again: the
			// server may have been stopped and started since, and a write
			// that meets a closed connection cannot always be sent again.
			IdleConnTimeout:       servedIdle,
			ResponseHeaderTimeout: servedWait + 30*time.Second,
		}}}, nil
}

// A call is one thing asked of the served directory.
type call struct {
	method, route string
	query         url.Values
	body          func() io.Reader // the value from its start; none for a call with no value
	twice         bool             // sending it twice is the same as sending it once
}

// send sends a call until the server answers it, and then says what the
// answer was. A call the server may have acted on is sent again only where
// twice is the same as once; otherwise the caller is told the outcome is
// not known.
func (s *served) send(ctx context.Context, c call) (servedAnswer, error) {
	if err := ctx.Err(); err != nil {
		return servedAnswer{}, err
	}
	wait := servedPause
	for began := time.Now(); ; wait = min(wait*2, servedPauseMax) {
		a, mayHaveActed, err := s.once(ctx, c)
		if err == nil && a.status >= http.StatusInternalServerError {
			// The server's own word that it failed: asking again gets the
			// same, and it may have got part of the way.
			err, mayHaveActed = a.problem(), true
			if c.twice {
				return servedAnswer{}, err
			}
		}
		if err == nil {
			return a, a.problem()
		}
		if mayHaveActed && !c.twice {
			return servedAnswer{}, fmt.Errorf("%w: %w", ErrUnknown, err)
		}
		if time.Since(began)+wait > servedPatience {
			return servedAnswer{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
		}
		select {
		case <-ctx.Done():
			return servedAnswer{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
		case <-time.After(wait):
		}
	}
}

// once sends a call one time, and gives the server's answer. Of a try that
// got none it says whether the server may have acted on the call all the
// same.
func (s *served) once(ctx context.Context, c call) (a servedAnswer, mayHaveActed bool, err error) {
	target := s.origin + c.route
	if len(c.query) > 0 {
		target += "?" + c.query.Encode()
	}
	var body io.Reader
	if c.body != nil {
		// Not the value's own reader, which the client would close.
		body = io.NopCloser(c.body())
	}
	req, err := http.NewRequestWithContext(ctx, c.method, target, body)
	if err != nil {
		return a, false, withoutAddress(err)
	}
	req.Header.Set(headParty, s.party)

	resp, err := s.client.Do(req)
	if err != nil {
		// The other connections kept to the server are as likely gone as
		// this one: the next try starts on a new one.
		s.client.CloseIdleConnections()
		// A request that never found the server, or found another computer
		// in its place, was never sent.
		var op *net.OpError
		neverSent := errors.As(err, &op) && op.Op == "dial" || errors.Is(err, errKeeper)
		return a, !neverSent, withoutAddress(err)
	}
	defer resp.Body.Close()
	said, err := io.ReadAll(resp.Body)
	if err != nil {
		return a, true, err
	}
	return servedAnswer{status: resp.StatusCode, mark: Mark(resp.Header.Get(headMark)), done: resp.Header.Get(headDone), body: said}, false, nil
}

// problem is the error an answer stands for, if it stands for one.
func (a servedAnswer) problem() error {
	said := string(a.body)
	switch a.status {
	case http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("%w%s", ErrRefused, said)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrRefused, said)
	case http.StatusGone:
		return fmt.Errorf("%w%s", ErrNothing, said)
	case http.StatusBadRequest:
		return fmt.Errorf("%w%s", ErrName, said)
	}
	return fmt.Errorf("store: %s", said)
}

func named(name string) url.Values { return url.Values{"name": {name}} }

func (s *served) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := checkName(name); err != nil {
		return nil, None, err
	}
	a, err := s.send(ctx, call{method: http.MethodGet, route: "/read", query: named(name), twice: true})
	if err != nil || a.status == http.StatusNotFound {
		return nil, None, err
	}
	return a.body, a.mark, nil
}

func (s *served) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	q := named(name)
	q.Set("from", strconv.FormatInt(from, 10))
	q.Set("length", strconv.FormatInt(length, 10))
	a, err := s.send(ctx, call{method: http.MethodGet, route: "/part", query: q, twice: true})
	if err != nil || len(a.body) == 0 {
		return nil, err
	}
	return a.body, nil
}

// in adds the way of writing to a call's query.
func in(q url.Values, how How) url.Values {
	q.Set("how", how.String())
	if how.kind == onlyIfUnchanged {
		q.Set("mark", string(how.mark))
	}
	return q
}

func (s *served) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	return s.change(ctx, call{method: http.MethodPost, route: "/write", query: in(named(name), how),
		body: func() io.Reader { return bytes.NewReader(value) }, twice: how.kind == overwrite})
}

// WriteFrom sends the value as it is read. It is written only-if-new, so it
// is sent again only where it was never sent, and so never read.
func (s *served) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	return s.change(ctx, call{method: http.MethodPost, route: "/write", query: in(named(name), OnlyIfNew),
		body: func() io.Reader { return r }})
}

func (s *served) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := errors.Join(checkName(from), checkName(to)); err != nil {
		return false, err
	}
	return s.change(ctx, call{method: http.MethodPost, route: "/copy", query: url.Values{"from": {from}, "to": {to}}})
}

func (s *served) Delete(ctx context.Context, name string, how How) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	return s.change(ctx, call{method: http.MethodPost, route: "/delete", query: in(named(name), how), twice: how.kind == overwrite})
}

// change sends a call that writes, copies or deletes, and says whether it
// did.
func (s *served) change(ctx context.Context, c call) (bool, error) {
	a, err := s.send(ctx, c)
	return err == nil && a.done == "true", err
}

func (s *served) List(ctx context.Context, under string) ([]string, error) {
	return s.names(ctx, under, url.Values{"under": {under}})
}

func (s *served) First(ctx context.Context, under string, n int) ([]string, error) {
	return s.names(ctx, under, url.Values{"under": {under}, "first": {strconv.Itoa(max(min(n, firstMost), 0))}})
}

func (s *served) names(ctx context.Context, under string, q url.Values) ([]string, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	a, err := s.send(ctx, call{method: http.MethodGet, route: "/list", query: q, twice: true})
	if err != nil || len(a.body) == 0 {
		return nil, err
	}
	return strings.Split(string(a.body), "\n"), nil
}

// Notice asks the server, again and again, to say when a name under the
// path is written: a party on another computer is told as one on the
// serving machine is, without looking at intervals.
func (s *served) Notice(under string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	ctx, stop := context.WithCancel(context.Background())
	go func() {
		for ctx.Err() == nil {
			a, _, err := s.once(ctx, call{method: http.MethodGet, route: "/wait", query: url.Values{"under": {under}}})
			switch {
			case err != nil || a.status != http.StatusOK:
				// Out of reach or turned away: whoever waits still looks
				// at its own interval, and the asking goes on more slowly.
				select {
				case <-ctx.Done():
				case <-time.After(servedPauseMax):
				}
			case a.done == "true":
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
	return ch, stop
}
