package plugin

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
)

// allowed is a job whose one layer has these lists.
func allowed(t *testing.T, allow, deny []string) *Job {
	j := &Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow, Deny: deny}}}}
	j.Perms.Scratch = j.Scratch
	t.Cleanup(j.End)
	return j
}

// names has the name svc.test looked up as the addresses given, each time
// it is asked the next list, and counts the asking.
func names(t *testing.T, answers ...[]string) *int {
	asked := 0
	was := lookup
	lookup = func(ctx context.Context, host string) ([]string, error) {
		answer := answers[min(asked, len(answers)-1)]
		asked++
		return answer, nil
	}
	t.Cleanup(func() { lookup = was })
	return &asked
}

// twoPlaces listens on one port at two addresses of this machine, and
// gives the port and a func that says which address was connected to.
func twoPlaces(t *testing.T) (port string, reached func() string) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ = net.SplitHostPort(first.Addr().String())
	second, err := net.Listen("tcp", "127.0.0.2:"+port)
	if err != nil {
		t.Skip("this machine has only one address to itself:", err)
	}
	got := make(chan string, 4)
	for _, l := range []net.Listener{first, second} {
		t.Cleanup(func() { l.Close() })
		go func() {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				host, _, _ := net.SplitHostPort(conn.LocalAddr().String())
				got <- host
				conn.Close()
			}
		}()
	}
	return port, func() string { return <-got }
}

func refused(err error) bool {
	var r *permission.Refusal
	return errors.As(err, &r)
}

func Test_perm_no_lookup_before_check(t *testing.T) {
	asked := names(t, []string{"127.0.0.1"})
	// No list could allow it: refused, and the name was not looked up.
	if _, err := allowed(t, []string{"net:other.test"}, nil).Dial(context.Background(), "svc.test:443"); !refused(err) || *asked != 0 {
		t.Fatalf("with no entry for it: %v, looked up %d times", err, *asked)
	}
	// Denied by name: refused before it is looked up.
	if _, err := allowed(t, []string{"network"}, []string{"net:svc.test"}).Dial(context.Background(), "svc.test:443"); !refused(err) || *asked != 0 {
		t.Fatalf("with the name denied: %v, looked up %d times", err, *asked)
	}
	// An entry that is an address could allow it, so it is looked up.
	if _, err := allowed(t, []string{"net:10.9.9.9"}, nil).Dial(context.Background(), "svc.test:443"); !refused(err) || *asked != 1 {
		t.Fatalf("with an address allowed: %v, looked up %d times", err, *asked)
	}
}

func Test_perm_address_allow_reaches_only_that_address(t *testing.T) {
	port, reached := twoPlaces(t)
	names(t, []string{"127.0.0.2", "127.0.0.1"})
	j := allowed(t, []string{"net:127.0.0.1:" + port}, nil)
	if _, err := j.Dial(context.Background(), "svc.test:"+port); err != nil {
		t.Fatal(err)
	}
	if at := reached(); at != "127.0.0.1" {
		t.Fatalf("the name was connected to at %s, which no entry allows", at)
	}
}

func Test_perm_connects_only_to_what_was_checked(t *testing.T) {
	port, reached := twoPlaces(t)
	// Every address the name has is checked: the denied one is passed over.
	asked := names(t, []string{"127.0.0.2", "127.0.0.1"}, []string{"127.0.0.2"})
	j := allowed(t, []string{"net:svc.test"}, []string{"net:127.0.0.2"})
	if _, err := j.Dial(context.Background(), "svc.test:"+port); err != nil {
		t.Fatal(err)
	}
	// And the name is looked up once: a second answer is never used.
	if at := reached(); at != "127.0.0.1" || *asked != 1 {
		t.Fatalf("connected to %s after looking the name up %d times", at, *asked)
	}
	// A name whose every address is denied is refused.
	names(t, []string{"127.0.0.2"})
	if _, err := j.Dial(context.Background(), "svc.test:"+port); !refused(err) {
		t.Fatalf("a name with only a denied address gave %v", err)
	}
}

// A way to connect asked for with a secret, and the asking for the
// secret's value over that connection, are one check: whatever the secret
// is bound to and however the connection is asked for, both pass or both
// refuse, in the same words.
func TestDialAndRevealCheckASecretsDestinationAlike(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	opened := make(chan struct{}, 8)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			opened <- struct{}{}
			conn.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	names(t, []string{"127.0.0.1"})
	handle := NewSecret("K")
	for _, c := range []struct {
		bound, asked string
		passes       bool
	}{
		{"svc.test", "svc.test", true},
		{"127.0.0.1", "127.0.0.1", true},
		// Bound to the address the name resolves to, and asked for by name.
		{"127.0.0.1", "svc.test", false},
		{"svc.test", "127.0.0.1", false},
	} {
		j := allowed(t, []string{"net:svc.test", "net:127.0.0.1", "secret:K@net:" + c.bound}, nil)
		j.Secrets = map[string]string{"K": "the value"}
		address := net.JoinHostPort(c.asked, port)
		conn, dialled := j.Dial(context.Background(), address, handle)
		value, revealed := j.RevealOver(handle, address, true)
		var atDial, atReveal *permission.Refusal
		errors.As(dialled, &atDial)
		errors.As(revealed, &atReveal)
		switch {
		case c.passes:
			if dialled != nil || revealed != nil || value != "the value" {
				t.Errorf("bound to %s and asked for by %s: the opening gave %v and the value %q, %v", c.bound, c.asked, dialled, value, revealed)
				continue
			}
			<-opened
			conn.Close()
		case atDial == nil || atReveal == nil || atDial.What != atReveal.What || atDial.What != "secret:K@net:"+address || value != "":
			t.Errorf("bound to %s and asked for by %s: the opening gave %v and the value %q, %v", c.bound, c.asked, dialled, value, revealed)
		}
	}
	// A refused opening opened nothing.
	select {
	case <-opened:
		t.Error("a connection was opened for a secret that may not go over it")
	default:
	}
}

func Test_perm_program_refused_before_it_is_looked_for(t *testing.T) {
	const name = "no-such-program-anywhere"
	j := allowed(t, []string{"exec:git"}, nil)
	j.SearchPath = "/usr/bin:/bin"
	_, there := j.Program(context.Background(), "sh", ProgramOptions{})
	_, missing := j.Program(context.Background(), name, ProgramOptions{})
	if !refused(there) || !refused(missing) || there.Error() == "" || strings.Contains(missing.Error(), "not found") {
		t.Fatalf("a refused program that is there gave %v, and one that is not gave %v", there, missing)
	}
	// Allowed, it is looked for, and only then is it found to be missing.
	j = allowed(t, []string{"exec:" + name}, nil)
	j.SearchPath = "/usr/bin:/bin"
	if _, err := j.Program(context.Background(), name, ProgramOptions{}); err == nil || refused(err) || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("an allowed program that is not there gave %v", err)
	}
}

func Test_perm_one_path_rule_for_every_kind(t *testing.T) {
	real := t.TempDir()
	os.WriteFile(filepath.Join(real, "tool"), []byte("#!/bin/sh\n"), 0o700)
	// Reading, writing and starting a program by its path each go through a
	// link to the same real place, and each is checked there.
	for _, kind := range []string{permission.Read, permission.Write, permission.Exec} {
		j := allowed(t, []string{kind + ":" + filepath.ToSlash(real) + "/**"}, nil)
		os.Symlink(real, filepath.Join(j.Scratch, "link"))
		try := func(j *Job) error {
			switch kind {
			case permission.Exec:
				_, err := j.Program(context.Background(), "link/tool", ProgramOptions{})
				return err
			default:
				_, err := j.Path("link/tool", kind == permission.Write)
				return err
			}
		}
		if err := try(j); err != nil {
			t.Fatalf("%s through a link to an allowed place: %v", kind, err)
		}
		// Allowed only where the link is, and not where it points: refused,
		// and the refusal names the real place.
		other := allowed(t, nil, nil)
		other.Perms.Layers[0].Allow = []string{kind + ":" + filepath.ToSlash(other.Scratch) + "/**"}
		os.Symlink(real, filepath.Join(other.Scratch, "link"))
		var r *permission.Refusal
		if err := try(other); !errors.As(err, &r) || !strings.Contains(r.Error(), filepath.ToSlash(real)+"/tool") {
			t.Fatalf("%s through a link out of the allowed place gave %v", kind, err)
		}
	}
}
