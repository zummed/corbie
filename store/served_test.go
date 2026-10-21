package store

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A servedDirectory is a directory served in this process, with the parties it
// knows.
type servedDirectory struct {
	t         *testing.T
	directory Store
	server    *Server
	address   string
	keeper    ed25519.PrivateKey
	parties   map[string]servedParty
	// between, when set, stands between the wire and the server.
	between func(http.Handler) http.Handler
}

type servedParty struct {
	Party
	key ed25519.PrivateKey
}

func proofOf(t *testing.T, key ed25519.PrivateKey) tls.Certificate {
	t.Helper()
	proof, err := Proof(key)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func keyOf(key ed25519.PrivateKey) ed25519.PublicKey { return key.Public().(ed25519.PublicKey) }

func newServedDirectory(t *testing.T) *servedDirectory {
	t.Helper()
	directory, err := OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, keeper, _ := ed25519.GenerateKey(rand.Reader)
	f := &servedDirectory{t: t, directory: directory, keeper: keeper, parties: map[string]servedParty{}}
	f.server = &Server{Directory: directory,
		Who: func(_ context.Context, name string) (Party, ed25519.PublicKey, error) {
			p, ok := f.parties[name]
			if !ok {
				return Party{}, nil, nil
			}
			return p.Party, keyOf(p.key), nil
		}}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.address = ServedScheme + l.Addr().String()
	f.listen(l)
	return f
}

// listen serves the directory to the connections a listener takes, as the
// machine that holds the directory does.
func (f *servedDirectory) listen(l net.Listener) {
	listening := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var h http.Handler = f.server
		if f.between != nil {
			h = f.between(h)
		}
		h.ServeHTTP(w, r)
	})}
	f.t.Cleanup(func() { listening.Close() })
	go listening.Serve(ServedListener(l, proofOf(f.t, f.keeper)))
}

// as opens the served directory as a party, which the server then knows.
func (f *servedDirectory) as(p Party) Store {
	f.t.Helper()
	held, ok := f.parties[p.Name]
	if !ok {
		_, key, _ := ed25519.GenerateKey(rand.Reader)
		held = servedParty{p, key}
		f.parties[p.Name] = held
	}
	return f.opened(f.address, p.Name, held.key, keyOf(f.keeper))
}

// opened opens a served directory at an address under a name, with a key of
// the party's and the key it takes to be the keeper's.
func (f *servedDirectory) opened(address, name string, key ed25519.PrivateKey, keeper ed25519.PublicKey) Store {
	f.t.Helper()
	s, err := OpenServed(address, name, proofOf(f.t, key), keeper)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func TestServedAddress(t *testing.T) {
	for _, bad := range []string{"corbie://desk", "corbie://desk:8372/main", "corbie://key@desk:8372", "corbie://desk:8372?region=x", "corbie://"} {
		if _, err := OpenServed(bad, "p", tls.Certificate{}, nil); err == nil {
			t.Errorf("%s was taken as a served directory's address", bad)
		}
	}
	if _, err := Open("corbie://desk:8372"); err == nil || IsDirectory("corbie://desk:8372") {
		t.Errorf("a served directory was opened with no party, or taken for a directory: %v", err)
	}
}

// A party on another computer is held to its row of the table by the server,
// as a bucket's policy holds it.
func Test_store_served_directory_holds_each_party_to_its_row(t *testing.T) {
	f := newServedDirectory(t)
	runner := f.as(Party{Kind: Runner, Name: "run", Controller: "ctl"})
	client := f.as(Party{Kind: Client, Name: "cli", Controller: "ctl"})
	other := f.as(Party{Kind: Client, Name: "other", Controller: "ctl"})
	put := func(name, value string) {
		t.Helper()
		if _, err := f.directory.Write(ctx, name, []byte(value), Overwrite); err != nil {
			t.Fatal(err)
		}
	}
	put("ctl/running/cli/i1", "head")
	put("ctl/records/cli/i1/steps/1", "step")
	put("run/inbox/ctl/job1", "job")
	put("ctl/secrets/TOKEN", "sealed")

	// A client reads its own heads and records, and nobody else's.
	if v, _ := read(t, client, "ctl/running/cli/i1"); v != "head" {
		t.Errorf("a client read its own head as %q", v)
	}
	if v, mark := read(t, other, "ctl/running/cli/i1"); v != "" || mark != None {
		t.Errorf("a client read another client's head: %q", v)
	}
	if _, err := Names(other.List(ctx, "ctl/records/cli")); !errors.Is(err, ErrRefused) {
		t.Errorf("a client listed another client's records: %v", err)
	}
	if v, _ := read(t, client, "ctl/secrets/TOKEN"); v != "" {
		t.Errorf("a client read a secret: %q", v)
	}
	// A client sends a message, and can neither change nor remove it.
	if done, err := client.Write(ctx, "ctl/inbox/cli/m1", []byte("start"), OnlyIfNew); !done || err != nil {
		t.Fatalf("a client's message: %v %v", done, err)
	}
	if _, err := client.Write(ctx, "ctl/inbox/cli/m1", []byte("other"), Overwrite); !errors.Is(err, ErrRefused) {
		t.Errorf("a client replaced a message: %v", err)
	}
	if _, err := client.Delete(ctx, "ctl/inbox/cli/m1", Overwrite); !errors.Is(err, ErrRefused) {
		t.Errorf("a client removed a message: %v", err)
	}
	if _, err := client.Write(ctx, "ctl/inbox/other/m1", []byte("start"), OnlyIfNew); !errors.Is(err, ErrRefused) {
		t.Errorf("a client wrote as another: %v", err)
	}
	// A runner takes its jobs, marks them and removes them, and adds to a
	// record; it reads no step and replaces nothing there.
	if v, _ := read(t, runner, "run/inbox/ctl/job1"); v != "job" {
		t.Errorf("a runner read its job as %q", v)
	}
	if done, err := runner.Write(ctx, "run/taken/t/job1", nil, OnlyIfNew); !done || err != nil {
		t.Errorf("a runner's mark: %v %v", done, err)
	}
	if done, err := runner.Delete(ctx, "run/inbox/ctl/job1", Overwrite); !done || err != nil {
		t.Errorf("a runner removing its job: %v %v", done, err)
	}
	if done, err := runner.Write(ctx, "ctl/records/cli/i1/emits/job1/1", []byte("emit"), OnlyIfNew); !done || err != nil {
		t.Errorf("a runner's emit: %v %v", done, err)
	}
	if v, _ := read(t, runner, "ctl/records/cli/i1/steps/1"); v != "" {
		t.Errorf("a runner read a step: %q", v)
	}
	if _, err := runner.Write(ctx, "ctl/records/cli/i1/steps/2", []byte("step"), OnlyIfNew); !errors.Is(err, ErrRefused) {
		t.Errorf("a runner wrote a step: %v", err)
	}
	// "Any one part of a name" is held exactly: nothing goes in deeper
	// than the layout draws.
	if _, err := client.Write(ctx, "ctl/inbox/cli/deeper/m2", []byte("x"), OnlyIfNew); !errors.Is(err, ErrRefused) {
		t.Errorf("a client wrote a name deeper than the layout draws: %v", err)
	}
	// A name that is no party is turned away whatever it asks.
	_, unknown, _ := ed25519.GenerateKey(rand.Reader)
	stranger := f.opened(f.address, "stranger", unknown, keyOf(f.keeper))
	if _, _, err := stranger.Read(ctx, VersionName); !errors.Is(err, ErrRefused) {
		t.Errorf("a name that is no party read the version: %v", err)
	}
	// A party's name with another's key is turned away too, a party's own
	// key among them.
	for who, key := range map[string]ed25519.PrivateKey{"a stranger": unknown, "another party": f.parties["other"].key} {
		if _, _, err := f.opened(f.address, "cli", key, keyOf(f.keeper)).Read(ctx, "ctl/running/cli/i1"); !errors.Is(err, ErrRefused) {
			t.Errorf("%s was answered as cli: %v", who, err)
		}
	}
}

// A served directory's wire is between a party and the computer that holds its
// controller's key, and nobody else is asked or answered anything.
func Test_store_served_directory_speaks_only_to_its_parties(t *testing.T) {
	f := newServedDirectory(t)
	client := f.as(Party{Kind: Client, Name: "cli", Controller: "ctl"})
	if _, err := f.directory.Write(ctx, "ctl/running/cli/i1", []byte("head"), Overwrite); err != nil {
		t.Fatal(err)
	}
	var asked atomic.Int32
	var from []string // the connection each request came by
	f.between = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked.Add(1)
			from = append(from, r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}
	defer func(patience time.Duration) { servedPatience = patience }(servedPatience)
	servedPatience = 300 * time.Millisecond

	// A machine that does not hold the controller's key is asked nothing: a
	// write is known not to have been sent.
	_, another, _ := ed25519.GenerateKey(rand.Reader)
	misled := f.opened(f.address, "cli", f.parties["cli"].key, keyOf(another))
	if done, err := misled.Write(ctx, "ctl/inbox/cli/m1", []byte("start"), OnlyIfNew); done || !errors.Is(err, ErrUnreachable) {
		t.Errorf("a write to a machine that does not hold the controller's key: %v %v", done, err)
	}
	// A request that comes by no such connection is not the store's to
	// answer: one in the plain, and one from an end that proves no key.
	host := strings.TrimPrefix(f.address, ServedScheme)
	for how, ask := range map[string]func() (*http.Response, error){
		"in the plain": func() (*http.Response, error) { return http.Get("http://" + host + "/read?name=" + VersionName) },
		"with no key": func() (*http.Response, error) {
			keyless := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			return keyless.Get("https://" + host + "/read?name=" + VersionName)
		},
	} {
		if resp, err := ask(); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Errorf("a request %s was answered", how)
			}
		}
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("%d requests reached the store from ends that proved nothing", n)
	}

	// Whose key a name's is, is asked at every request: a party that is
	// removed is turned away on the connection it already has.
	if v, _ := read(t, client, "ctl/running/cli/i1"); v != "head" {
		t.Fatalf("a client read its head as %q", v)
	}
	delete(f.parties, "cli")
	if _, _, err := client.Read(ctx, "ctl/running/cli/i1"); !errors.Is(err, ErrRefused) {
		t.Errorf("a party that was removed was answered: %v", err)
	}
	if len(from) != 2 || from[0] != from[1] {
		t.Errorf("the two requests did not come by one connection: %v", from)
	}
}

// A party on another computer is told when a name it may read is written,
// and is told nothing of names it may not.
func Test_store_served_directory_tells_a_waiting_party(t *testing.T) {
	f := newServedDirectory(t)
	runner := f.as(Party{Kind: Runner, Name: "run", Controller: "ctl"})
	told, stop := runner.Notice("run/inbox")
	defer stop()
	other, stopOther := f.as(Party{Kind: Runner, Name: "run2", Controller: "ctl"}).Notice("run/inbox")
	defer stopOther()
	// The asking has to have reached the server before the write.
	time.Sleep(200 * time.Millisecond)
	if _, err := f.directory.Write(ctx, "run/inbox/ctl/job1", []byte("job"), OnlyIfNew); err != nil {
		t.Fatal(err)
	}
	select {
	case <-told:
	case <-time.After(5 * time.Second):
		t.Fatal("a runner waiting on its inbox was not told of a job written there")
	}
	select {
	case <-other:
		t.Error("a runner was told of a write to another runner's inbox")
	case <-time.After(300 * time.Millisecond):
	}
}

// A request made while the serving machine's process is being stopped and
// started again waits for it, whatever kind of request it is: nothing was
// sent, so nothing can happen twice.
func Test_store_served_directory_tries_outlast_a_restart(t *testing.T) {
	f := newServedDirectory(t)
	client := f.as(Party{Kind: Client, Name: "cli", Controller: "ctl"})
	// The machine is away: nothing listens where it was.
	away, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	away.Close()
	held := f.parties["cli"]
	s := f.opened(ServedScheme+away.Addr().String(), "cli", held.key, keyOf(f.keeper))
	// It comes back, at the same address, a moment later.
	go func() {
		time.Sleep(700 * time.Millisecond)
		l, err := net.Listen("tcp", away.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		f.listen(l)
	}()
	if done, err := s.Write(ctx, "ctl/inbox/cli/m1", []byte("start"), OnlyIfNew); !done || err != nil {
		t.Fatalf("an only-if-new write made while the server was away: %v %v", done, err)
	}
	if v, _ := read(t, client, "ctl/receipts/cli/none"); v != "" {
		t.Fatal(v)
	}
	// A machine that stays away is reported as out of reach, in the end.
	defer func(patience time.Duration) { servedPatience = patience }(servedPatience)
	servedPatience = 300 * time.Millisecond
	never := f.opened(ServedScheme+"127.0.0.1:1", "cli", held.key, keyOf(f.keeper))
	if _, _, err := never.Read(ctx, VersionName); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a served directory that never answers: %v", err)
	}
}

// What goes wrong on the serving machine is told to that machine. The party
// is told only that something did, at once, and not as a store it cannot
// reach.
func Test_store_served_directory_failure_names_nothing_of_the_machine(t *testing.T) {
	f := newServedDirectory(t)
	client := f.as(Party{Kind: Client, Name: "cli", Controller: "ctl"})
	who := f.server.Who
	var told []error
	f.server.Failed = func(err error) { told = append(told, err) }
	f.server.Who = func(ctx context.Context, name string) (Party, ed25519.PublicKey, error) {
		return Party{}, nil, errors.New("open /srv/corbie/ctl/trust/cli: input/output error")
	}
	began := time.Now()
	_, _, err := client.Read(ctx, VersionName)
	if err == nil || errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), "/srv/corbie") {
		t.Errorf("a failure on the serving machine reached the party as: %v", err)
	}
	if time.Since(began) > 2*time.Second {
		t.Errorf("a read the server said it could not do was asked for again for %v", time.Since(began))
	}
	if len(told) != 1 || !strings.Contains(told[0].Error(), "/srv/corbie") {
		t.Errorf("the serving machine was told %v", told)
	}
	// A write that may have been part done is not known to have been, or
	// not.
	if _, err := client.Write(ctx, "ctl/inbox/cli/m1", []byte("x"), OnlyIfNew); !errors.Is(err, ErrUnknown) {
		t.Errorf("an only-if-new write the server failed at: %v", err)
	}
	f.server.Who = who

	// A number of names that is no number drops no connection.
	for kind, s := range map[string]Store{"served": f.as(Party{Kind: Owner, Name: "owner"}), "directory": f.directory} {
		if names, err := Names(s.First(ctx, "ctl", -1)); err != nil || len(names) != 0 {
			t.Errorf("%s: the first -1 names: %v %v", kind, names, err)
		}
	}
	if part, err := f.as(Party{Kind: Owner, Name: "owner"}).ReadPart(ctx, "ctl", 0, 10); err != nil || part != nil {
		t.Errorf("a part of a path that holds names: %q %v", part, err)
	}
}

// A server that is stopped answers those who wait at once, and keeps no
// connection open for more.
func Test_store_served_directory_stopping_answers_what_it_has_in_hand(t *testing.T) {
	f := newServedDirectory(t)
	var closing []string
	answered := make(chan struct{}, 4)
	f.between = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			if r.URL.Path == "/wait" {
				closing = append(closing, w.Header().Get("Connection"))
				answered <- struct{}{}
			}
		})
	}
	_, stop := f.as(Party{Kind: Runner, Name: "run", Controller: "ctl"}).Notice("run/inbox")
	defer stop()
	time.Sleep(200 * time.Millisecond) // the asking has reached the server
	f.server.Stop()
	select {
	case <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("a party waiting on a server that was stopped was left waiting")
	}
	if closing[0] != "close" {
		t.Errorf("a stopping server answered and kept the connection: %q", closing[0])
	}
}
