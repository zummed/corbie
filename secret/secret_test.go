package secret_test

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

var ctx = context.Background()

func newController(t *testing.T) (*identity.Identity, store.Store) {
	t.Helper()
	ctl, err := identity.New("ctl", identity.Controller)
	if err != nil {
		t.Fatal(err)
	}
	return ctl, store.NewMemory().As(store.Party{Kind: store.Owner})
}

// A run uses the secret set nearest its location, and one can be set
// before any tree is submitted there. Removing the nearest leaves the one
// above it.
func TestSecretSetNearestALocationWins(t *testing.T) {
	ctl, s := newController(t)
	set := func(at, value string) {
		t.Helper()
		if err := secret.Set(ctx, s, ctl, at, "ANTHROPIC_KEY", value); err != nil {
			t.Fatal(err)
		}
	}
	uses := func(location, want string) {
		t.Helper()
		value, found, err := secret.Get(ctx, s, ctl, location, "ANTHROPIC_KEY")
		if err != nil || found != (want != "") || value != want {
			t.Errorf("a run at %s uses %q (%v, %v), want %q", location, value, found, err, want)
		}
	}
	uses("/work/agents/support", "")
	set("/", "every tree")
	set("/work", "work")
	uses("/work/agents/support", "work")
	uses("/work", "work")
	uses("/home/devices/boiler", "every tree")
	uses("/workshop", "every tree")
	if err := secret.Set(ctx, s, ctl, "/", "BOILER_PASSWORD", "b"); err != nil {
		t.Fatal(err)
	}
	listed, err := secret.List(ctx, s, ctl.Name)
	if want := []secret.Entry{{Name: "ANTHROPIC_KEY", At: "/"}, {Name: "BOILER_PASSWORD", At: "/"}, {Name: "ANTHROPIC_KEY", At: "/work"}}; err != nil || !slices.Equal(listed, want) {
		t.Errorf("the secrets are listed as %v (%v), want %v", listed, err, want)
	}
	if err := secret.Remove(ctx, s, ctl, "/work", "ANTHROPIC_KEY"); err != nil {
		t.Fatal(err)
	}
	uses("/work/agents/support", "every tree")
	for _, at := range []string{"work", "/work/", "/+", ""} {
		if err := secret.Set(ctx, s, ctl, at, "ANTHROPIC_KEY", "x"); !errors.Is(err, store.ErrName) {
			t.Errorf("a secret was set at %q: %v", at, err)
		}
	}
}

// A value is set, used, listed and removed as a secret is, and the nearest
// wins for each name.
func TestValueSetNearestALocationWins(t *testing.T) {
	ctl, s := newController(t)
	for _, v := range [][3]string{{"/", "HUB", "hub.lan"}, {"/home", "HUB", "192.0.2.8:8123"}, {"/", "ZONE", "eu"}} {
		if err := secret.SetValue(ctx, s, ctl, v[0], v[1], v[2]); err != nil {
			t.Fatal(err)
		}
	}
	values, err := secret.Values(ctx, s, ctl, "/home/devices/boiler")
	if want := map[string]string{"HUB": "192.0.2.8:8123", "ZONE": "eu"}; err != nil || !maps.Equal(values, want) {
		t.Errorf("a run at /home/devices/boiler reads %v (%v), want %v", values, err, want)
	}
	listed, err := secret.ListValues(ctx, s, ctl)
	want := []secret.Entry{{"HUB", "/", "hub.lan"}, {"ZONE", "/", "eu"}, {"HUB", "/home", "192.0.2.8:8123"}}
	if err != nil || !slices.Equal(listed, want) {
		t.Errorf("the values are listed as %v (%v), want %v", listed, err, want)
	}
	missing, err := secret.Missing(ctx, s, ctl, "/home/devices/boiler", []string{"BOILER_PASSWORD"}, []string{"HUB", "PORT"})
	if err != nil || !slices.Equal(missing, []string{"secret BOILER_PASSWORD", "value PORT"}) {
		t.Errorf("missing at /home/devices/boiler: %v (%v)", missing, err)
	}
	if err := secret.RemoveValue(ctx, s, ctl, "/home", "HUB"); err != nil {
		t.Fatal(err)
	}
	if values, _ := secret.Values(ctx, s, ctl, "/home/devices/boiler"); values["HUB"] != "hub.lan" {
		t.Errorf("after the value at /home is removed, a run reads %v", values)
	}
	for _, name := range []string{"", "A=B", "A B", ".A", "-A", "A/B"} {
		if err := secret.SetValue(ctx, s, ctl, "/", name, "x"); err == nil {
			t.Errorf("a value was set under the name %q", name)
		}
	}
}

// A secret and a value are sealed to the controller: whoever runs the store
// cannot read them, and a runner's or a client's key does not open them.
// One copied to another location's path is refused as altered.
func TestSecretAndValueSealedToTheController(t *testing.T) {
	ctl, s := newController(t)
	if err := secret.Set(ctx, s, ctl, "/home", "K", "the secret value"); err != nil {
		t.Fatal(err)
	}
	if err := secret.SetValue(ctx, s, ctl, "/home", "V", "the plain value"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []identity.Kind{identity.Runner, identity.Client} {
		other, err := identity.New("other", kind)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{store.Secret("ctl", "/home", "K"): "the secret value", store.Value("ctl", "/home", "V"): "the plain value"} {
			data, _, _ := s.Read(ctx, name)
			var read string
			if _, err := identity.Read(data, name, ctl.Card, other, &read); err == nil || bytes.Contains(data, []byte(value)) {
				t.Errorf("%s is in the clear, or a %s's key opens it (%v)", name, kind, err)
			}
		}
	}
	for from, to := range map[string]string{
		store.Secret("ctl", "/home", "K"): store.Secret("ctl", "/home/devices", "K"),
		store.Value("ctl", "/home", "V"):  store.Value("ctl", "/home/devices", "V"),
	} {
		if _, err := s.Copy(ctx, from, to); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := secret.Get(ctx, s, ctl, "/home/devices/boiler", "K"); !errors.Is(err, identity.ErrAltered) {
		t.Errorf("a secret moved to another location was read: %v", err)
	}
	if _, err := secret.Values(ctx, s, ctl, "/home/devices/boiler"); !errors.Is(err, identity.ErrAltered) {
		t.Errorf("a value moved to another location was read: %v", err)
	}
}
