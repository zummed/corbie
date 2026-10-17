package identity

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

func party(t *testing.T, name string, kind Kind) *Identity {
	t.Helper()
	id, err := New(name, kind)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCard(t *testing.T) {
	r := party(t, "laptop", Runner)
	line := r.Card.String()
	if strings.ContainsAny(line, " \n") {
		t.Fatalf("a card is not one line: %q", line)
	}
	c, err := ParseCard(line + "\n")
	if err != nil || c.Name != "laptop" || c.Kind != Runner || !c.Same(r.Card) {
		t.Fatalf("a card read back: %+v %v", c, err)
	}
	if c.Fingerprint() != r.Fingerprint() {
		t.Fatal("a card's fingerprint differs between showings")
	}
	// One character of the name changed: not valid.
	if _, err := ParseCard(strings.Replace(line, "laptop", "laptoq", 1)); err == nil {
		t.Fatal("a card with a changed name passed")
	}
	if _, err := ParseCard(strings.Replace(line, ":runner:", ":client:", 1)); err == nil {
		t.Fatal("a card with a changed kind passed")
	}
	for _, bad := range []string{"", "corbie-card", line + ":x", "x" + line} {
		if _, err := ParseCard(bad); err == nil {
			t.Fatalf("%q passed as a card", bad)
		}
	}
	// Taken where another kind is expected, it is refused for what it is.
	if _, err := Take(nil, r.Card, Controller, false); err == nil || !strings.Contains(err.Error(), "runner") {
		t.Fatalf("a runner's card taken as a controller's: %v", err)
	}
}

func Test_id_fingerprint_changes_with_either_key(t *testing.T) {
	a := party(t, "p", Runner)
	_, sign, _ := ed25519.GenerateKey(rand.Reader)
	seal, _ := ecdh.X25519().GenerateKey(rand.Reader)
	otherSeal := newIdentity("p", Runner, a.signKey, seal)
	otherSign := newIdentity("p", Runner, sign, a.sealKey)
	if a.Fingerprint() == otherSeal.Fingerprint() {
		t.Fatal("swapping the decrypting key left the fingerprint")
	}
	if a.Fingerprint() == otherSign.Fingerprint() {
		t.Fatal("swapping the signing key left the fingerprint")
	}
}

func Test_id_identity_file_readable_by_others_refused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner", "identity")
	id := party(t, "r1", Runner)
	if err := id.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil || !back.Same(id.Card) || back.Name != "r1" {
		t.Fatalf("an identity read back: %v", err)
	}
	letOthersRead(t, path)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "read by others") {
		t.Fatalf("an identity file others can read: %v", err)
	}
}

func Test_id_identity_file_written_whole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	// A party stopped while its identity was being made leaves a temporary
	// file and nothing under the name.
	os.WriteFile(filepath.Join(dir, ".tmp-123"), []byte(`{"name":"r1","ki`), 0o600)
	if Exists(path) {
		t.Fatal("half an identity is under the name")
	}
	c := Config{Dir: dir}
	id, err := c.Make(Runner, "r1")
	if err != nil {
		t.Fatal(err)
	}
	// Made once, it is never made again, whatever name is asked for.
	again, err := c.Make(Runner, "other")
	if err != nil || again.Name != "r1" || !again.Same(id.Card) {
		t.Fatalf("a second making changed the identity: %v", err)
	}
}

func Test_id_private_key_never_printed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	id := party(t, "r1", Runner)
	id.Save(path)
	data, _ := os.ReadFile(path)
	secrets := []string{b64.EncodeToString(id.signKey.Seed()), b64.EncodeToString(id.sealKey.Bytes())}
	for _, damaged := range [][]byte{data[:len(data)/2], bytes.Replace(data, []byte(`"name"`), []byte(`"nome"`), 1), append(bytes.Clone(data[:len(data)-4]), "!!}"...)} {
		os.WriteFile(path, damaged, 0o600)
		_, err := Load(path)
		if err == nil {
			t.Fatal("a damaged identity file was read")
		}
		for _, s := range secrets {
			if strings.Contains(err.Error(), s) || strings.Contains(err.Error(), s[:12]) {
				t.Fatalf("an error shows a private key: %v", err)
			}
		}
	}
	if strings.Contains(id.Card.String(), secrets[0]) || strings.Contains(id.Card.String(), secrets[1]) {
		t.Fatal("a card shows a private key")
	}
}

type note struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

var now = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func TestSignedAndSealed(t *testing.T) {
	ctl, r1, r2 := party(t, "ctl", Controller), party(t, "r1", Runner), party(t, "r2", Runner)
	at := "r1/inbox/ctl/job1"
	data, err := ctl.Write(at, now, now.Add(time.Minute), note{"job", "marker-text"}, r1.Card)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("marker-text")) {
		t.Fatal("a sealed value shows its body")
	}
	var got note
	v, err := Read(data, at, ctl.Card, r1, &got)
	if err != nil || got.Text != "marker-text" || v.By != "ctl" || !v.Sent.Equal(now) {
		t.Fatalf("the receiver read %+v: %v", got, err)
	}
	// Another party cannot open it.
	if _, err := Read(data, at, ctl.Card, r2, &got); !errors.Is(err, ErrNotForUs) {
		t.Fatalf("another party opening: %v", err)
	}
	// Found under another name, it is refused.
	if _, err := Read(data, "r2/inbox/ctl/job1", ctl.Card, r1, &got); !errors.Is(err, ErrAltered) {
		t.Fatalf("a value moved to another name: %v", err)
	}
	// Any byte changed, it is refused.
	for i := 0; i < len(data)-1; i += 7 {
		changed := bytes.Clone(data)
		changed[i] ^= 1
		if _, err := Read(changed, at, ctl.Card, r1, &got); err == nil {
			t.Fatalf("a value with byte %d changed was read", i)
		}
	}
	// Checked against another party's card, it is not that party's.
	if _, err := Read(data, at, r2.Card, r1, &got); !errors.Is(err, ErrAltered) {
		t.Fatalf("a value checked against another card: %v", err)
	}

	// An open value is readable by anyone and still signed.
	head, _ := ctl.Write("ctl/running/c1/i1", now, time.Time{}, note{"head", "public"})
	if !bytes.Contains(head, []byte(`"public"`)) {
		t.Fatal("an open value does not show its body")
	}
	if _, err := Read(head, "ctl/running/c1/i1", ctl.Card, nil, &got); err != nil || got.Text != "public" {
		t.Fatalf("an open value read with no identity: %v", err)
	}
}

// Runner B copies a result runner A wrote and files it as its own.
func Test_id_unsigned_parts_of_message_not_believed(t *testing.T) {
	ctl, a, b := party(t, "ctl", Controller), party(t, "a", Runner), party(t, "b", Runner)
	at := "ctl/inbox/a/m1"
	data, _ := a.Write(at, now, now.Add(time.Minute), note{"result", "x"}, ctl.Card)
	v, _ := Parse(data)

	// B wraps A's sealed body in a frame of its own, signed by B.
	theirs := "ctl/inbox/b/m1"
	frame, _ := Encode(Value{By: "b", At: theirs, Sent: now, Until: now.Add(time.Minute), Sealed: true})
	signed := append(append(frame, '\n'), v.body...)
	forged := append(append(append(signed, '\n'), b64.EncodeToString(ed25519.Sign(b.signKey, signed))...), '\n')
	var got note
	fv, err := Parse(forged)
	if err != nil || fv.Check(b.Card, theirs) != nil {
		t.Fatal("the wrapper should carry a true signature of its own")
	}
	if err := fv.Open(ctl, &got); err == nil {
		t.Fatal("a body wrapped in another party's frame was opened")
	}

	// A's value left under B's name is not B's.
	if _, err := Read(data, theirs, b.Card, ctl, &got); !errors.Is(err, ErrAltered) {
		t.Fatalf("a's value under b's name: %v", err)
	}
}

func Test_id_signed_value_names_matched_exactly(t *testing.T) {
	ctl := party(t, "ctl", Controller)
	sign := func(frame, body string) []byte {
		signed := []byte(frame + "\n" + body)
		return []byte(string(signed) + "\n" + b64.EncodeToString(ed25519.Sign(ctl.signKey, signed)) + "\n")
	}
	var got note
	good := sign(`{"at":"x/y","by":"ctl","sent":"2026-10-10T12:00:00Z"}`, `{"kind":"k","text":"t"}`)
	if _, err := Read(good, "x/y", ctl.Card, nil, &got); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{
		sign(`{"at":"x/y","By":"ctl","sent":"2026-10-10T12:00:00Z"}`, `{"kind":"k","text":"t"}`),
		sign(`{"at":"x/y","at":"x/z","by":"ctl","sent":"2026-10-10T12:00:00Z"}`, `{"kind":"k","text":"t"}`),
		sign(`{"at":"x/y","by":"ctl","sent":"2026-10-10T12:00:00Z"}`, `{"kind":"k","Kind":"other","text":"t"}`),
		sign(`{"at":"x/y","by":"ctl","sent":"2026-10-10T12:00:00Z"}`, `{"kind":"k","kind":"other","text":"t"}`),
		sign(`{"at":"x/y","by":"ctl","sent":"2026-10-10T12:00:00Z"}`, `{"kind":"k","text":"t","more":1}`),
	} {
		if _, err := Read(bad, "x/y", ctl.Card, nil, &got); err == nil {
			t.Fatalf("read a value with a name twice or in the wrong case: %s", bad)
		}
	}
}

func Test_id_fast_receiver_clock_does_not_drop_good_messages(t *testing.T) {
	c := party(t, "c1", Client)
	data, _ := c.Write("ctl/inbox/c1/m", now, now.Add(time.Minute), note{})
	v, _ := Parse(data)
	if !v.Good(now.Add(4 * time.Minute)) {
		t.Fatal("a receiver four minutes ahead dropped a message good for one")
	}
	if !v.Good(now.Add(time.Minute + ClockAllowance)) {
		t.Fatal("the allowance is not whole")
	}
	if v.Good(now.Add(time.Minute + ClockAllowance + time.Second)) {
		t.Fatal("a message six minutes past its time is still good")
	}
	forever, _ := c.Write("ctl/inbox/c1/m", now, time.Time{}, note{})
	if v, _ := Parse(forever); !v.Good(now.Add(1000 * time.Hour)) {
		t.Fatal("a message with no end stopped being good")
	}
}

func Test_id_removed_name_stays_reserved(t *testing.T) {
	ctx := context.Background()
	s, _ := store.Open("memory:")
	ctl := party(t, "ctl", Controller)
	first, second := party(t, "laptop", Runner), party(t, "laptop", Runner)

	tr, err := Take(nil, first.Card, "", false)
	if err != nil {
		t.Fatal(err)
	}
	tr.Tags = []string{"gpu"}
	if err := WriteTrust(ctx, s, ctl, tr); err != nil {
		t.Fatal(err)
	}
	held, err := ReadTrust(ctx, s, ctl.Card, "laptop")
	if err != nil || !held.Trusted() || !held.HasTags([]string{"gpu"}) {
		t.Fatalf("trust read back: %+v %v", held, err)
	}
	// Another key under the name is refused; the first is unchanged.
	if _, err := Take(held, second.Card, "", false); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("a different key under a used name: %v", err)
	}
	// Removed, the name stays on record with its key.
	held.Removed = true
	WriteTrust(ctx, s, ctl, *held)
	held, _ = ReadTrust(ctx, s, ctl.Card, "laptop")
	if held == nil || held.Trusted() {
		t.Fatal("a removed party is erased or still trusted")
	}
	if _, err := Take(held, second.Card, "", false); err == nil {
		t.Fatal("a removed name was free to another key")
	}
	// The same card is taken again without being called a replacement.
	if again, err := Take(held, first.Card, "", false); err != nil || again.Removed {
		t.Fatalf("the same card again: %v", err)
	}
	// A trust value someone altered is refused.
	raw, _, _ := s.Read(ctx, store.Trust("ctl", "laptop"))
	s.Write(ctx, store.Trust("ctl", "laptop"), bytes.Replace(raw, []byte("gpu"), []byte("cpu"), 1), store.Overwrite)
	if _, err := ReadTrust(ctx, s, ctl.Card, "laptop"); !errors.Is(err, ErrAltered) {
		t.Fatalf("altered trust: %v", err)
	}
}

func Test_id_replacement_does_not_inherit_ceiling(t *testing.T) {
	first, second := party(t, "c1", Client), party(t, "c1", Client)
	old := Trust{Card: first.Card, Ceiling: permission.Layer{Allow: []string{"all"}}, Tags: []string{"x"}}
	got, err := Take(&old, second.Card, Client, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ceiling.Allow) != 0 || len(got.Tags) != 0 || !got.Card.Same(second.Card) {
		t.Fatalf("a replacement was given %+v", got)
	}
}

func TestConfigTrust(t *testing.T) {
	c := Config{Dir: t.TempDir()}
	ctl := party(t, "ctl", Controller)
	if got, err := c.Trust(Runner, "ctl"); got != nil || err != nil {
		t.Fatalf("trust never given: %v %v", got, err)
	}
	want := Trust{Card: ctl.Card, Store: "/srv/corbie", Ceiling: permission.Layer{Allow: []string{"disk", "exec"}}}
	if err := c.SetTrust(Runner, want); err != nil {
		t.Fatal(err)
	}
	got, err := c.Trust(Runner, "ctl")
	if err != nil || !got.Card.Same(ctl.Card) || got.Store != "/srv/corbie" || len(got.Ceiling.Allow) != 2 {
		t.Fatalf("trust read back: %+v %v", got, err)
	}
	all, err := c.TrustList(Runner)
	if err != nil || len(all) != 1 {
		t.Fatalf("trust listed: %v %v", all, err)
	}
	if err := c.SetControllerStore("/srv/corbie"); err != nil {
		t.Fatal(err)
	}
	if addr, _ := c.ControllerStore(); addr != "/srv/corbie" {
		t.Fatalf("the controller's store is %q", addr)
	}
}
