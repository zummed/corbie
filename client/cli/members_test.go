package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/client/cli"
)

// stays runs a command that stays, and gives what it says and what stops it.
func (m machine) stays(args ...string) (*said, func()) {
	m.t.Helper()
	out, errOut := &said{}, &said{}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(ctx, append(args, "--config", m.config), strings.NewReader(""), out, errOut)
	}()
	stopped := false
	end := func() {
		if !stopped {
			stopped = true
			stop()
			<-done
		}
	}
	m.t.Cleanup(end)
	return errOut, end
}

// Three computers become the members of one store: the second and third are
// given the controller's identity and join the first. A run goes on with the
// first stopped, the first is removed, and the last copy is made the whole
// store.
func Test_store_members_join_carry_on_and_leave(t *testing.T) {
	at1 := freePort(t)
	sw1 := machine{t, filepath.Join(t.TempDir(), "sw1")}
	first := filepath.Join(t.TempDir(), "store")
	sw1.ok("", "init", "--store", first, "--name", "fab", "--ceiling", "all")
	sw1.ok("", "init", "store", "--listen", at1)
	_, stop1 := sw1.stays("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "store")

	member := func(name string) (machine, string) {
		m := machine{t, filepath.Join(t.TempDir(), name)}
		identity, err := os.ReadFile(filepath.Join(sw1.config, "controller", "identity"))
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Join(m.config, "controller"), 0o700)
		if err := os.WriteFile(filepath.Join(m.config, "controller", "identity"), identity, 0o600); err != nil {
			t.Fatal(err)
		}
		at := freePort(t)
		if out := m.ok("", "init", "store", "--listen", at, "--join", "corbie://"+at1); !strings.Contains(out, "joins the store's members when corbie start runs here") || !strings.Contains(out, "corbie://"+at) {
			t.Fatalf("init store --join printed:\n%s", out)
		}
		return m, at
	}
	sw2, at2 := member("sw2")
	sw3, at3 := member("sw3")

	// The second member has a runner of its own, enrolled on it: it holds
	// the controller's identity, and reaches the store through the first.
	card := sw2.ok("", "runner", "card", "--name", "sw2")
	sw2.ok(sw2.ok(card, "controller", "enrol", "-", "--tags", "sw2"), "runner", "trust", "-", "--ceiling", "all")
	said2, _ := sw2.stays("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "store")
	_, stop3 := sw3.stays("start", "controller", "--interval", "20ms", "store")
	// corbie init store with no --listen prints the members, and keeps the
	// address saved: a member is known to the others by it.
	until(t, "three members that count", func() bool {
		out := sw1.ok("", "init", "store")
		if !strings.Contains(out, "corbie://"+at1) {
			t.Fatalf("init store with no --listen changed the address saved:\n%s", out)
		}
		return strings.Contains(out, at2) && strings.Contains(out, at3) && !strings.Contains(out, "joining")
	})
	// It prints which member leads, as each member answers.
	until(t, "one member that leads and two that follow it", func() bool {
		out := sw3.ok("", "init", "store")
		return strings.Count(out, "   leads\n") == 1 && strings.Count(out, "   follows the leader\n") == 2
	})
	if !strings.Contains(said2.String(), "store: joining the members at "+at1) {
		t.Fatalf("a computer that joins said:\n%s", said2)
	}
	// Only a member's server changes a member's copy, and a person reads
	// it as any directory.
	if _, err := os.Stat(filepath.Join(sw2.config, "store", "version")); err != nil {
		t.Fatalf("the copy of a computer that joined has no version: %v", err)
	}

	// corbie init run again on a member writes through the members' servers.
	if out := sw1.ok("", "init", "--store", first, "--name", "fab", "--ceiling", "all"); !strings.Contains(out, "corbie://"+at1) {
		t.Fatalf("init again on a member printed:\n%s", out)
	}

	// A party is given every member's address.
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	enrolled := sw1.ok(laptop.ok("", "client", "card", "--name", "laptop"), "controller", "enrol", "-", "--ceiling", "all")
	all := []string{at1, at2, at3}
	slices.Sort(all)
	if lines := strings.Split(strings.TrimSpace(enrolled), "\n"); len(lines) != 2 || lines[1] != "corbie://"+strings.Join(all, ",") {
		t.Fatalf("enrol on a member printed %q, and not every member's address", enrolled)
	}
	laptop.ok(enrolled, "client", "trust", "-")

	dir := files(t, map[string]string{"greet.js": `return { text: "hello " + input.name }`})
	greet := laptop.recorded(filepath.Join(dir, "greet.js"), "--tag", "sw2")
	greets := func(name string) {
		t.Helper()
		if out := laptop.ok("", "run", "start", greet, "--input", `{"name": "`+name+`"}`, "--wait"); !strings.Contains(out, `"text":"hello `+name+`"`) {
			t.Fatalf("a run on the members printed:\n%s", out)
		}
	}
	greets("three")

	// The first member stops, and the other two carry on.
	stop1()
	greets("two")
	until(t, "the members that are on to lead and follow", func() bool {
		out := sw2.ok("", "init", "store")
		return strings.Contains(out, at1+"   does not answer\n") && strings.Count(out, "   leads\n") == 1 && strings.Count(out, "   follows the leader\n") == 1
	})

	// A member's address is not changed: the others know it by it.
	for _, listen := range []string{"127.0.0.1:1", ""} {
		if status, _, said := sw2.run("", "init", "store", "--listen", listen); status != 1 || !strings.Contains(said, "this computer is the store's member "+at2) {
			t.Fatalf("init store --listen %q on a member: %d %s", listen, status, said)
		}
	}
	if out := sw2.ok("", "init", "store", "--listen", at2); !strings.Contains(out, "corbie://"+at2) {
		t.Fatalf("init store with the address saved, on a member, printed:\n%s", out)
	}
	// An address that is no member is not removed.
	if status, _, said := sw2.run("", "init", "store", "--remove", "127.0.0.1:1"); status != 1 || !strings.Contains(said, "127.0.0.1:1 is not a member of the store") {
		t.Fatalf("init store --remove of no member: %d %s", status, said)
	}
	// The first member is removed, from another member.
	if out := sw2.ok("", "init", "store", "--remove", at1); !strings.Contains(out, at1+" is no longer a member") {
		t.Fatalf("init store --remove printed:\n%s", out)
	}
	// Each member prints the members as its own copy has them, a moment
	// after the leader's.
	until(t, "two members", func() bool {
		out := sw2.ok("", "init", "store")
		return !strings.Contains(out, at1) && strings.Contains(out, at3)
	})
	greets("after")
	// The removed member, started again, is turned away by the others:
	// corbie start says so and ends.
	if status, _, said := sw1.run("", "start", "store"); status != 1 || !strings.Contains(said, at1+" was removed from the store's members") || !strings.Contains(said, "the store's server has stopped") {
		t.Fatalf("corbie start on a removed member: %d %s", status, said)
	}
	if status, _, said := sw2.run("", "start", "store", "--join", "corbie://"+at3); status != 2 || !strings.Contains(said, "corbie init store") {
		t.Fatalf("corbie start store --join: %d %s", status, said)
	}

	// The third member's copy is made the whole store, with its server
	// stopped.
	if status, _, said := sw3.run("", "init", "store", "--alone"); status != 1 || !strings.Contains(said, "the store's server is running on this computer") {
		t.Fatalf("init store --alone beside a running server: %d %s", status, said)
	}
	stop3()
	if out := sw3.ok("", "init", "store", "--alone"); !strings.Contains(out, "is now the whole store, with one member") {
		t.Fatalf("init store --alone printed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(sw3.config, "store", ".log")); !os.IsNotExist(err) {
		t.Fatalf("a copy made the whole store has a log still: %v", err)
	}
	if status, _, said := sw3.run("", "init", "store", "--alone"); status != 1 || !strings.Contains(said, "has one member") {
		t.Fatalf("init store --alone on a store with one member: %d %s", status, said)
	}
}
