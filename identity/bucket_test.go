package identity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zummed/corbie/store"
)

// kept stands in for the bucket that keeps identities: it holds objects,
// answers a read and a write, and notes every write that was not
// only-if-new.
type kept struct {
	mu      sync.Mutex
	objects map[string][]byte
	plain   int
}

func (k *kept) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	k.mu.Lock()
	defer k.mu.Unlock()
	old, there := k.objects[r.URL.Path]
	switch {
	case r.Method == http.MethodGet && there:
		w.Header().Set("ETag", `"1"`)
		w.Write(old)
	case r.Method == http.MethodGet:
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodPut && r.Header.Get("If-None-Match") != "*":
		k.plain++
		w.WriteHeader(http.StatusForbidden)
	case r.Method == http.MethodPut && there:
		w.WriteHeader(http.StatusPreconditionFailed)
	case r.Method == http.MethodPut:
		k.objects[r.URL.Path], _ = io.ReadAll(r.Body)
		w.Header().Set("ETag", `"1"`)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// keeping starts a bucket that keeps identities and gives its address.
func keeping(t *testing.T) (*kept, string) {
	t.Helper()
	k := &kept{objects: map[string][]byte{}}
	served := httptest.NewServer(k)
	t.Cleanup(served.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "the-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "the-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	return k, "s3://keys/billing?region=test-1&endpoint=" + served.URL
}

func TestIdentityInBucket(t *testing.T) {
	ctx := context.Background()
	k, address := keeping(t)
	c := Config{Dir: t.TempDir()}
	if err := c.SetIdentityAddress(Controller, address); err != nil {
		t.Fatal(err)
	}
	id, err := c.Make(ctx, Controller, "ctl")
	if err != nil {
		t.Fatal(err)
	}
	// The object is under the path, named for the kind, and holds what the
	// file would.
	object := k.objects["/keys/billing/controller/identity"]
	if back, err := decode(object, os.ErrInvalid); err != nil || !back.Same(id.Card) {
		t.Fatalf("the bucket holds %d objects, and the identity read from it: %v", len(k.objects), err)
	}
	// The config holds the address and no private key.
	files, _ := filepath.Glob(filepath.Join(c.Dir, "controller", "*"))
	if len(files) != 1 || filepath.Base(files[0]) != "identity-address" || !c.Has(Controller) {
		t.Fatalf("the config holds %v", files)
	}
	if recorded, _ := os.ReadFile(files[0]); strings.TrimSpace(string(recorded)) != address {
		t.Fatalf("the config records %q", recorded)
	}
	// A process reads it from there each time it starts.
	loaded, err := c.Load(ctx, Controller)
	if err != nil || !loaded.Same(id.Card) || loaded.Name != "ctl" {
		t.Fatalf("the identity loaded from the bucket: %v", err)
	}
	// A bucket that holds none is said, and nothing is made by a load.
	empty := Config{Dir: t.TempDir()}
	empty.SetIdentityAddress(Runner, address)
	if _, err := empty.Load(ctx, Runner); err == nil || !strings.Contains(err.Error(), "holds none") || len(k.objects) != 1 {
		t.Fatalf("loading an identity the bucket does not hold: %v", err)
	}
	// Only a bucket keeps one.
	if err := empty.SetIdentityAddress(Client, t.TempDir()); err == nil {
		t.Fatal("a directory was taken as an identity's address")
	}
}

func Test_id_identity_in_bucket_never_replaced(t *testing.T) {
	ctx := context.Background()
	k, address := keeping(t)
	first := Config{Dir: t.TempDir()}
	first.SetIdentityAddress(Controller, address)
	id, err := first.Make(ctx, Controller, "ctl")
	if err != nil {
		t.Fatal(err)
	}
	// Made again, from this config or from another that names the address,
	// it is the identity that is there, whatever name is asked for.
	second := Config{Dir: t.TempDir()}
	second.SetIdentityAddress(Controller, address)
	for _, c := range []Config{first, second} {
		again, err := c.Make(ctx, Controller, "other")
		if err != nil || again.Name != "ctl" || !again.Same(id.Card) {
			t.Fatalf("a second making changed the identity: %v", err)
		}
	}
	// One that appears between the look and the write is kept too: the
	// write is only-if-new, and the bucket refuses any other.
	k.mu.Lock()
	raced := k.objects["/keys/billing/controller/identity"]
	delete(k.objects, "/keys/billing/controller/identity")
	k.mu.Unlock()
	late, err := New("late", Controller)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := late.encode()
	s, _ := second.bucket(Controller)
	if written, err := s.Write(ctx, Kept(Controller), data, store.OnlyIfNew); err != nil || !written {
		t.Fatalf("writing where nothing is: %v", err)
	}
	k.mu.Lock()
	k.objects["/keys/billing/controller/identity"] = raced
	k.mu.Unlock()
	if written, _ := s.Write(ctx, Kept(Controller), data, store.OnlyIfNew); written {
		t.Fatal("a write replaced the identity the bucket held")
	}
	if k.plain != 0 {
		t.Fatalf("%d writes were not only-if-new", k.plain)
	}
	// The address a config records is not changed either.
	if err := first.SetIdentityAddress(Controller, strings.Replace(address, "billing", "other", 1)); err == nil {
		t.Fatal("a config took another address for an identity it has")
	}
}

func Test_id_config_with_file_and_address_refused(t *testing.T) {
	ctx := context.Background()
	_, address := keeping(t)
	c := Config{Dir: t.TempDir()}
	if _, err := c.Make(ctx, Runner, "r1"); err != nil {
		t.Fatal(err)
	}
	// A config that holds the file takes no address.
	if err := c.SetIdentityAddress(Runner, address); err == nil {
		t.Fatal("a config that holds the identity file took an address for it")
	}
	// One that holds both, however it came to, is refused, and the refusal
	// says which two.
	os.WriteFile(filepath.Join(c.Dir, "runner", "identity-address"), []byte(address+"\n"), 0o600)
	for what, err := range map[string]error{
		"load": func() error { _, err := c.Load(ctx, Runner); return err }(),
		"make": func() error { _, err := c.Make(ctx, Runner, "r1"); return err }(),
	} {
		if err == nil || !strings.Contains(err.Error(), "as a file and the address") {
			t.Fatalf("%s with both the file and the address: %v", what, err)
		}
	}
}

func TestDamagedIdentityInBucketShowsNoKey(t *testing.T) {
	ctx := context.Background()
	k, address := keeping(t)
	c := Config{Dir: t.TempDir()}
	c.SetIdentityAddress(Client, address)
	k.objects["/keys/billing/client/identity"] = []byte(`{"name":"c1","kind":"client","sign":"c2VjcmV0LXNlZWQ","seal":"!"}`)
	if _, err := c.Load(ctx, Client); err == nil || strings.Contains(err.Error(), "c2VjcmV0") {
		t.Fatalf("a damaged identity in the bucket: %v", err)
	}
}
