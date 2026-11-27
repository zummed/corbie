package cli_test

import (
	"context"
	"os"
	"path/filepath"
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

// Three computers become the members of one store: the first admits the
// card of the second and of the third, and each joins with what that
// printed. A run goes on with the first stopped and with the second at
// another address, the first is removed by its name, and the last copy is
// made the whole store.
func Test_store_members_join_carry_on_and_leave(t *testing.T) {
	at1 := freePort(t)
	sw1 := machine{t, filepath.Join(t.TempDir(), "sw1")}
	first := filepath.Join(t.TempDir(), "store")
	sw1.ok("", "init", "--store", first, "--name", "fab", "--ceiling", "all")
	// Every computer of the test has one host name, so each card is named.
	sw1.ok("", "init", "store", "--card", "--name", "sw1")
	if out := sw1.ok("", "init", "store", "--listen", at1); !strings.Contains(out, "corbie://"+at1) {
		t.Fatalf("init store printed:\n%s", out)
	}
	_, stop1 := sw1.stays("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "store")

	// A computer that joins is given nothing by hand: its card goes to a
	// member, and what the member prints comes back.
	member := func(name string) (machine, string) {
		m := machine{t, filepath.Join(t.TempDir(), name)}
		card := m.ok("", "init", "store", "--card", "--name", name)
		admitted := sw1.ok(card, "init", "store", "--admit", "-")
		at := freePort(t)
		out := m.ok(admitted, "init", "store", "--listen", at, "--join", "-")
		if !strings.Contains(out, "joins the store's members as "+name+" when corbie start runs here") || !strings.Contains(out, "corbie://"+at) {
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
	said2, stop2 := sw2.stays("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "store")
	_, stop3 := sw3.stays("start", "controller", "--interval", "20ms", "store")
	// corbie init store with no --listen prints the members by name, and
	// keeps the address saved.
	until(t, "three members that count", func() bool {
		out := sw1.ok("", "init", "store")
		if !strings.Contains(out, "corbie://"+at1) {
			t.Fatalf("init store with no --listen changed the address saved:\n%s", out)
		}
		return strings.Contains(out, "sw2   "+at2+"   ") && strings.Contains(out, "sw3   "+at3+"   ") && !strings.Contains(out, "joining")
	})
	// It prints which member leads, as each member answers.
	until(t, "one member that leads and two that follow it", func() bool {
		out := sw3.ok("", "init", "store")
		return strings.Count(out, "   leads\n") == 1 && strings.Count(out, "   follows the leader\n") == 2
	})
	if !strings.Contains(said2.String(), "store: joining the members at "+at1) {
		t.Fatalf("a computer that joins said:\n%s", said2)
	}
	if again := sw2.ok("", "init", "store", "--card"); !strings.HasPrefix(again, "corbie-card:member:sw2:") {
		t.Fatalf("init store --card on a member printed %q", again)
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
	if lines := strings.Split(strings.TrimSpace(enrolled), "\n"); len(lines) != 2 || lines[1] != "corbie://"+strings.Join([]string{at1, at2, at3}, ",") {
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

	// A member is reached by the others, and does not stop being served.
	if status, _, said := sw2.run("", "init", "store", "--listen", ""); status != 1 || !strings.Contains(said, "this computer is the store's member sw2") {
		t.Fatalf(`init store --listen "" on a member: %d %s`, status, said)
	}
	// A member's address is a setting: the second member is started at
	// another, and the third finds it there by its name.
	stop2()
	moved := freePort(t)
	if out := sw2.ok("", "init", "store", "--listen", moved); !strings.Contains(out, "corbie://"+moved) || !strings.Contains(out, "The other members are told this address when corbie start next runs here.") {
		t.Fatalf("init store --listen with another address, on a member, printed:\n%s", out)
	}
	sw2.stays("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "store")
	until(t, "the second member at its new address", func() bool {
		out := sw3.ok("", "init", "store")
		return strings.Contains(out, "sw2   "+moved+"   ") && !strings.Contains(out, at2+" ") && strings.Count(out, "   leads\n") == 1
	})
	// A member's answer names the members' addresses as they now are. The
	// runner on the second member and the client on the laptop each save
	// them in their trust, and so carry on when the first member stops.
	greets("moved")
	now := "corbie://" + strings.Join([]string{at1, moved, at3}, ",")
	for _, trust := range []string{filepath.Join(sw2.config, "runner", "trust", "fab-controller"), filepath.Join(laptop.config, "client", "trust", "fab-controller")} {
		until(t, "the members' addresses saved in "+trust, func() bool {
			held, err := os.ReadFile(trust)
			if err != nil {
				t.Fatal(err)
			}
			return strings.Contains(string(held), now)
		})
	}
	if again2 := sw2.ok("", "controller", "enrol", "sw2"); !strings.HasSuffix(strings.TrimSpace(again2), now) {
		t.Fatalf("enrol after a member's address changed printed %q", again2)
	}

	// A name is one member's: a card of a member's name is not admitted,
	// and a member's card is no party's.
	other := machine{t, filepath.Join(t.TempDir(), "other")}
	again := other.ok("", "init", "store", "--card", "--name", "sw3")
	if status, _, said := sw2.run(again, "init", "store", "--admit", "-"); status != 1 || !strings.Contains(said, "sw3 is a member of the store already") {
		t.Fatalf("init store --admit of a member's name: %d %s", status, said)
	}
	if status, _, said := sw2.run(again, "controller", "enrol", "-"); status != 1 || !strings.Contains(said, "this is the card of a member") {
		t.Fatalf("controller enrol of a member's card: %d %s", status, said)
	}
	// What was admitted is sealed to one card, and another does not join
	// with it.
	sealed := sw2.ok(machine{t, filepath.Join(t.TempDir(), "sw4")}.ok("", "init", "store", "--card", "--name", "sw4"), "init", "store", "--admit", "-")
	if status, _, said := other.run(sealed, "init", "store", "--listen", freePort(t), "--join", "-"); status != 1 || !strings.Contains(said, "sealed to another card") {
		t.Fatalf("init store --join with what another card was admitted with: %d %s", status, said)
	}
	if _, err := os.Stat(filepath.Join(other.config, "controller")); !os.IsNotExist(err) {
		t.Fatalf("a computer that was not admitted holds something of the controller: %v", err)
	}

	// The first member stops, and the other two carry on.
	stop1()
	greets("two")
	until(t, "the members that are on to lead and follow", func() bool {
		out := sw2.ok("", "init", "store")
		return strings.Contains(out, "sw1   "+at1+"   does not answer\n") && strings.Count(out, "   leads\n") == 1 && strings.Count(out, "   follows the leader\n") == 1
	})

	// A name that is no member's is not removed.
	if status, _, said := sw2.run("", "init", "store", "--remove", "nobody"); status != 1 || !strings.Contains(said, "nobody is not a member of the store: its members are sw1, sw2, sw3") {
		t.Fatalf("init store --remove of no member: %d %s", status, said)
	}
	// The first member is removed by its name, from another member.
	if out := sw2.ok("", "init", "store", "--remove", "sw1"); !strings.Contains(out, "sw1 is no longer a member") {
		t.Fatalf("init store --remove printed:\n%s", out)
	}
	// Each member prints the members as its own copy has them, a moment
	// after the leader's.
	until(t, "two members", func() bool {
		out := sw2.ok("", "init", "store")
		return !strings.Contains(out, at1) && strings.Contains(out, "sw3   "+at3+"   ")
	})
	greets("after")
	// The removed member, started again, is turned away by the others:
	// corbie start says so and ends.
	if status, _, said := sw1.run("", "start", "store"); status != 1 || !strings.Contains(said, "sw1 was removed from the store's members") || !strings.Contains(said, "the store's server has stopped") {
		t.Fatalf("corbie start on a removed member: %d %s", status, said)
	}
	if status, _, said := sw2.run("", "start", "store", "--join", "-"); status != 2 || !strings.Contains(said, "corbie init store") {
		t.Fatalf("corbie start store --join: %d %s", status, said)
	}
	// It joins again under its name, with its card and an empty directory.
	back := sw2.ok(sw1.ok("", "init", "store", "--card"), "init", "store", "--admit", "-")
	sw1.ok(back, "init", "store", "--store", filepath.Join(t.TempDir(), "again"), "--join", "-")
	// A computer that is joining has not the whole store to be alone with.
	if status, _, said := sw1.run("", "init", "store", "--alone"); status != 1 || !strings.Contains(said, "is joining the store's members") {
		t.Fatalf("init store --alone on a computer that is joining: %d %s", status, said)
	}
	sw1.stays("start", "controller", "--interval", "20ms", "store")
	until(t, "the first member to count again", func() bool {
		out := sw3.ok("", "init", "store")
		return strings.Contains(out, "sw1   "+at1+"   ") && !strings.Contains(out, "joining") && !strings.Contains(out, "does not answer")
	})
	greets("again")

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
