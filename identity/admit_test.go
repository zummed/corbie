package identity_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/identity"
)

// A controller's identity is sealed to the card of a computer that is to
// become a member: that computer reads it, with the address of the store,
// and no other does.
func Test_id_admitted_read_only_by_its_card(t *testing.T) {
	ctx := context.Background()
	made := func(kind identity.Kind, name string) *identity.Identity {
		id, err := identity.Config{Dir: t.TempDir()}.Make(ctx, kind, name)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ctl, member, other := made(identity.Controller, "fab-controller"), made(identity.Member, "sw2"), made(identity.Member, "sw2")

	text, err := ctl.Admit(member.Card, "corbie://sw1:8372", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, at, err := member.Admitted(text)
	if err != nil {
		t.Fatal(err)
	}
	// A pipe that ends each line as Windows does changes nothing.
	if _, _, err := member.Admitted(bytes.ReplaceAll(text, []byte("\n"), []byte("\r\n"))); err != nil {
		t.Fatalf("with lines ended as Windows ends them: %v", err)
	}
	if !got.Card.Same(ctl.Card) || at != "corbie://sw1:8372" {
		t.Fatalf("admitted gave the controller %s at %s", got.Card.Name, at)
	}
	// What it gives signs as the controller does.
	signed, err := got.Write("x", time.Now(), time.Time{}, "y")
	if err != nil {
		t.Fatal(err)
	}
	var y string
	if _, err := identity.Read(signed, "x", ctl.Card, nil, &y); err != nil || y != "y" {
		t.Fatalf("a value the admitted identity signed: %q %v", y, err)
	}

	// Another card of the same name does not read it.
	if _, _, err := other.Admitted(text); err == nil || !strings.Contains(err.Error(), "sealed to another card") {
		t.Fatalf("another card read what was admitted: %v", err)
	}
	// It is sealed to a member's card and no other, and a card alone is
	// not what was admitted.
	if _, err := ctl.Admit(made(identity.Runner, "r").Card, "corbie://sw1:8372", time.Now()); err == nil {
		t.Fatal("a controller's identity was sealed to a card that is no member's")
	}
	if _, _, err := member.Admitted([]byte(ctl.Card.String() + "\n")); err == nil {
		t.Fatal("a card alone was taken for what was admitted")
	}
}
