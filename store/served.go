package store

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	party  string
	client *http.Client

	// origins are the members the address names, and origin the one last
	// found to answer: the leader, once a member has said who that is.
	mu      sync.Mutex
	origins []string
	origin  string
}

// errElsewhere is the answer of a member that does not lead the store and
// names the member that does, and errLeaderless that of one that knows of
// no leader. Neither did anything of what was asked.
var (
	errElsewhere  = errors.New("this member does not lead the store")
	errLeaderless = errors.New("this member knows of no leader")
)

// errLeft is the answer of a member that put a change in the log and then
// stopped leading: the leader after it has the change, or does not.
var errLeft = errors.New("this member stopped leading before the change was answered")

// errSilent is a member that was sent a request and then sent nothing for
// servedSilence.
var errSilent = errors.New("this member sent nothing after the request")

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

// servedSilence is how long a member may send nothing before a party takes
// it to be off and asks the next: while the connection is made, while each
// end proves its key, and after a request is sent. A member that is on sends
// a response at each servedWord until it answers. A computer that loses
// power sends nothing, and the operating system waits minutes for it.
var servedSilence = 3 * time.Second

// OpenServed opens the directory served at an address as a party: proof is the
// party's own, and keeper is the key of the controller whose machine serves
// the directory.
func OpenServed(address, party string, proof tls.Certificate, keeper ed25519.PublicKey) (Store, error) {
	hosts, err := ServedHosts(address)
	if err != nil {
		return nil, err
	}
	origins := make([]string, len(hosts))
	for i, host := range hosts {
		origins[i] = "https://" + host
	}
	return &served{origins: origins, origin: origins[0], party: party,
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
			DialContext:         (&net.Dialer{Timeout: servedSilence}).DialContext,
			TLSHandshakeTimeout: servedSilence,
			MaxIdleConnsPerHost: 64,
			// A connection that has sat unused is not used again: the
			// server may have been stopped and started since, and a write
			// that meets a closed connection cannot always be sent again.
			IdleConnTimeout: servedIdle,
			// How long a value that is read as it is sent waits for the
			// server to ask for it.
			ExpectContinueTimeout: servedPatience,
		}}}, nil
}

// Leader asks the member at an address, host:port, who leads the store, as
// a party. It says whether that member leads, or else the address of the
// member it names, which is nothing where it knows of none. An error is a
// member that did not answer.
func Leader(ctx context.Context, member, party string, proof tls.Certificate, keeper ed25519.PublicKey) (leads bool, leader string, err error) {
	opened, err := OpenServed(ServedScheme+member, party, proof, keeper)
	if err != nil {
		return false, "", err
	}
	s := opened.(*served)
	_, _, err = s.once(ctx, s.at(), call{method: http.MethodGet, route: "/read", query: named(VersionName)})
	switch {
	case err == nil:
		return true, "", nil
	case errors.Is(err, errElsewhere):
		return false, strings.TrimPrefix(s.at(), "https://"), nil
	case errors.Is(err, errLeaderless):
		return false, "", nil
	}
	return false, "", err
}

// ServedHosts gives the members a served directory's address names, each as
// host:port: corbie://host:port, or with several members
// corbie://host:port,host:port,host:port.
func ServedHosts(address string) ([]string, error) {
	bad := errors.New("store: a served directory's address reads corbie://host:port, with a comma between members")
	rest, ok := strings.CutPrefix(address, ServedScheme)
	if !ok || rest == "" || strings.ContainsAny(rest, "/?@#") {
		return nil, bad
	}
	hosts := strings.Split(rest, ",")
	for _, host := range hosts {
		if name, port, err := net.SplitHostPort(host); err != nil || name == "" || port == "" {
			return nil, bad
		}
	}
	return hosts, nil
}

// at gives the member to ask.
func (s *served) at() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.origin
}

// turn has the next call ask another member: the one named, or else the
// one after asked in the address.
func (s *served) turn(asked, leader string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.origin != asked {
		return // another call has turned already
	}
	if leader != "" {
		s.origin = "https://" + leader
		return
	}
	at := slices.Index(s.origins, asked)
	s.origin = s.origins[(at+1)%len(s.origins)]
}

// A call is one thing asked of the served directory.
type call struct {
	method, route string
	query         url.Values
	body          func() io.Reader // the value from its start; none for a call with no value
	twice         bool             // sending it twice is the same as sending it once
	// read is set for a value that is read as it is sent, and says whether
	// any of it has been: such a value is not sent from its start again.
	read func() bool
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
	said := map[string]error{} // what each member asked said last
	for began := time.Now(); ; wait = min(wait*2, servedPauseMax) {
		origin := s.at()
		a, mayHaveActed, err := s.once(ctx, origin, c)
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
		if errors.Is(err, errElsewhere) {
			// Told who leads: that member is asked at once.
			wait = servedPause / 10
		}
		if mayHaveActed && !c.twice {
			return servedAnswer{}, fmt.Errorf("%w: %w", ErrUnknown, err)
		}
		said[origin] = err
		if c.read != nil && c.read() {
			return servedAnswer{}, unreached(said)
		}
		if time.Since(began)+wait > servedPatience {
			return servedAnswer{}, unreached(said)
		}
		select {
		case <-ctx.Done():
			return servedAnswer{}, unreached(said)
		case <-time.After(wait):
		}
	}
}

// unreached is the error of a call that no member answered: what the member
// asked said, or what each of several said.
func unreached(said map[string]error) error {
	format, args := "%w: %w", []any{ErrUnreachable}
	for i, origin := range slices.Sorted(maps.Keys(said)) {
		err := said[origin]
		// A connection that was not made names the member already.
		var op *net.OpError
		if len(said) > 1 && !(errors.As(err, &op) && op.Op == "dial") {
			err = fmt.Errorf("%s: %w", strings.TrimPrefix(origin, "https://"), err)
		}
		if i > 0 {
			format += "; %w"
		}
		args = append(args, err)
	}
	return fmt.Errorf(format, args...)
}

// once sends a call one time, and gives the server's answer. Of a try that
// got none it says whether the server may have acted on the call all the
// same.
func (s *served) once(ctx context.Context, origin string, c call) (a servedAnswer, mayHaveActed bool, err error) {
	target := origin + c.route
	if len(c.query) > 0 {
		target += "?" + c.query.Encode()
	}
	var body io.Reader
	if c.body != nil {
		// Not the value's own reader, which the client would close.
		body = io.NopCloser(c.body())
	}
	// The try ends when the member sends nothing for servedSilence after
	// the request: no interim response, and none of the answer.
	ctx, end := context.WithCancelCause(ctx)
	defer end(nil)
	silence := time.AfterFunc(servedSilence, func() { end(errSilent) })
	silence.Stop() // it begins when the request is sent
	defer silence.Stop()
	heard := func() { silence.Reset(servedSilence) }
	var sent atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteHeaders:   func() { sent.Store(true); heard() },
		Got1xxResponse: func(int, textproto.MIMEHeader) error { heard(); return nil },
	})
	req, err := http.NewRequestWithContext(ctx, c.method, target, body)
	if err != nil {
		return a, false, withoutAddress(err)
	}
	req.Header.Set(headParty, s.party)
	if c.read != nil {
		// The server asks for the value once it has found that it leads
		// the store and who the party is: a member that does not lead is
		// sent none of it, and the value is whole for the one that does.
		req.Header.Set("Expect", "100-continue")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if context.Cause(ctx) == errSilent {
			err = errSilent
		}
		// The other connections kept to the server are as likely gone as
		// this one: the next try starts on a new one.
		s.client.CloseIdleConnections()
		// A request that never found the server, or found another computer
		// in its place, was never sent.
		s.turn(origin, "")
		return a, sent.Load(), withoutAddress(err)
	}
	defer resp.Body.Close()
	heard()
	if resp.StatusCode == http.StatusMisdirectedRequest {
		io.Copy(io.Discard, resp.Body)
		leader := resp.Header.Get(headLeader)
		s.turn(origin, leader)
		if leader == "" {
			return a, false, errLeaderless
		}
		return a, false, errElsewhere
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		io.Copy(io.Discard, resp.Body)
		s.turn(origin, "")
		return a, true, errLeft
	}
	said, err := io.ReadAll(hearing{resp.Body, heard})
	if err != nil {
		if context.Cause(ctx) == errSilent {
			err = errSilent
		}
		return a, true, err
	}
	return servedAnswer{status: resp.StatusCode, mark: Mark(resp.Header.Get(headMark)), done: resp.Header.Get(headDone), body: said}, false, nil
}

// hearing reads an answer, and notes each part of it that arrives.
type hearing struct {
	from  io.Reader
	heard func()
}

func (h hearing) Read(p []byte) (int, error) {
	n, err := h.from.Read(p)
	h.heard()
	return n, err
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

// WriteFrom sends the value as it is read. It is written only-if-new, and
// is sent again only where none of it was read.
func (s *served) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	value := &readOnce{from: r}
	return s.change(ctx, call{method: http.MethodPost, route: "/write", query: in(named(name), OnlyIfNew),
		body: func() io.Reader { return value }, read: value.begun.Load})
}

// readOnce is a value that is read as it is sent, and notes that it was.
type readOnce struct {
	from  io.Reader
	begun atomic.Bool
}

func (r *readOnce) Read(p []byte) (int, error) {
	r.begun.Store(true)
	return r.from.Read(p)
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

func (s *served) List(ctx context.Context, under string) ([]Listed, error) {
	return s.list(ctx, under, url.Values{"under": {under}})
}

func (s *served) First(ctx context.Context, under string, n int) ([]Listed, error) {
	return s.list(ctx, under, url.Values{"under": {under}, "first": {strconv.Itoa(max(min(n, firstMost), 0))}})
}

func (s *served) list(ctx context.Context, under string, q url.Values) ([]Listed, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	a, err := s.send(ctx, call{method: http.MethodGet, route: "/list", query: q, twice: true})
	if err != nil || len(a.body) == 0 {
		return nil, err
	}
	var listed []Listed
	for line := range strings.SplitSeq(string(a.body), "\n") {
		at := strings.LastIndex(line, "\t")
		if at < 0 {
			return nil, fmt.Errorf("store: the served directory listed %q with no mark", line)
		}
		listed = append(listed, Listed{line[:at], Mark(line[at+1:])})
	}
	return listed, nil
}

// Notice asks the server, again and again, to say when a name under the
// path is written: a party on another computer is told as one on the
// serving machine is, without looking at intervals.
func (s *served) Notice(under string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	ctx, stop := context.WithCancel(context.Background())
	go func() {
		for ctx.Err() == nil {
			a, _, err := s.once(ctx, s.at(), call{method: http.MethodGet, route: "/wait", query: url.Values{"under": {under}}})
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
