package identity

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zummed/corbie/store"
)

// A Config is the directory that holds a machine's parties: a place each for a
// controller, a runner and a client.
//
//	<config>/controller/identity      the controller's keys
//	<config>/controller/store         the address of its store
//	<config>/runner/identity          the runner's keys
//	<config>/runner/trust/<name>      a controller the runner serves
//	<config>/client/identity          the client's keys
//	<config>/client/trust/<name>      the client's controller
//
// In place of identity, a party's directory may hold identity-address: the
// address of the bucket that keeps its keys.
type Config struct {
	Dir string
}

// DefaultConfig is where a machine's parties live unless it is told
// otherwise.
func DefaultConfig() (Config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	return Config{Dir: filepath.Join(dir, "corbie")}, nil
}

func (c Config) identityPath(kind Kind) string {
	return filepath.Join(c.Dir, string(kind), "identity")
}

func (c Config) identityAddressPath(kind Kind) string {
	return filepath.Join(c.Dir, string(kind), "identity-address")
}

// Kept is the name an identity of the kind has in the bucket that
// keeps it.
func Kept(kind Kind) string { return string(kind) + "/identity" }

// IdentityAddress is the address of the bucket that keeps the identity of
// the kind, or nothing where the identity is a file in the config.
func (c Config) IdentityAddress(kind Kind) (string, error) {
	data, err := os.ReadFile(c.identityAddressPath(kind))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(data)), err
}

// SetIdentityAddress notes the bucket that keeps the identity of the kind.
// An identity stays where it was made: a config that holds the file, or
// records another address, is refused.
func (c Config) SetIdentityAddress(kind Kind, address string) error {
	if !strings.HasPrefix(address, "s3://") {
		return errors.New("an identity's address is a bucket's: s3://bucket/path?region=…")
	}
	recorded, err := c.IdentityAddress(kind)
	switch {
	case err != nil:
		return err
	case Exists(c.identityPath(kind)):
		return fmt.Errorf("this config holds the %s's identity as a file, and an identity stays where it was made", kind)
	case recorded == address:
		return nil
	case recorded != "":
		return fmt.Errorf("this config records another address for the %s's identity, and an identity stays where it was made", kind)
	}
	return writeWhole(c.identityAddressPath(kind), []byte(address+"\n"))
}

// Has says whether the config holds an identity of the kind, or the address
// of one.
func (c Config) Has(kind Kind) bool {
	return Exists(c.identityPath(kind)) || Exists(c.identityAddressPath(kind))
}

// bucket gives the bucket that keeps the identity of the kind, or nothing
// where the identity is a file in the config.
func (c Config) bucket(kind Kind) (store.Store, error) {
	address, err := c.IdentityAddress(kind)
	if err != nil || address == "" {
		return nil, err
	}
	if Exists(c.identityPath(kind)) {
		return nil, fmt.Errorf("this config holds the %s's identity as a file and the address of one in a bucket: remove one of the two", kind)
	}
	return store.Open(address)
}

// read reads the identity of the kind from the bucket that keeps it, and
// says whether one is there.
func read(ctx context.Context, s store.Store, kind Kind) (*Identity, bool, error) {
	data, mark, err := s.Read(ctx, Kept(kind))
	if err != nil || mark == store.None {
		return nil, false, err
	}
	id, err := decode(data, fmt.Errorf("the %s's identity in the bucket is damaged", kind))
	return id, true, err
}

// Load reads the identity of the kind.
func (c Config) Load(ctx context.Context, kind Kind) (*Identity, error) {
	s, err := c.bucket(kind)
	if err != nil {
		return nil, err
	}
	if s == nil {
		id, err := Load(c.identityPath(kind))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("this computer has no %s yet (corbie init makes one)", kind)
		}
		return id, err
	}
	id, there, err := read(ctx, s, kind)
	if err == nil && !there {
		return nil, fmt.Errorf("the bucket the config records for the %s's identity holds none, or this access key may not read it", kind)
	}
	return id, err
}

// Make gives the identity of the kind, making it with the name if there is
// none. A name once chosen is never changed: an identity that exists is
// given as it is.
func (c Config) Make(ctx context.Context, kind Kind, name string) (*Identity, error) {
	s, err := c.bucket(kind)
	if err != nil {
		return nil, err
	}
	if s == nil {
		path := c.identityPath(kind)
		if Exists(path) {
			return Load(path)
		}
		id, err := New(name, kind)
		if err != nil {
			return nil, err
		}
		return id, id.Save(path)
	}
	id, there, err := read(ctx, s, kind)
	if err != nil || there {
		return id, err
	}
	if id, err = New(name, kind); err != nil {
		return nil, err
	}
	data, err := id.encode()
	if err != nil {
		return nil, err
	}
	// Only-if-new: of two that make it at once, one writes and the other
	// reads what the first wrote.
	written, err := s.Write(ctx, Kept(kind), data, store.OnlyIfNew)
	if err != nil || written {
		return id, err
	}
	return c.Load(ctx, kind)
}

// ControllerStore is the address of the controller's store.
func (c Config) ControllerStore() (string, error) {
	data, err := os.ReadFile(filepath.Join(c.Dir, string(Controller), "store"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(data)), err
}

// SetControllerStore notes the address of the controller's store.
func (c Config) SetControllerStore(address string) error {
	return writeWhole(filepath.Join(c.Dir, string(Controller), "store"), []byte(address+"\n"))
}

// StoreListen is where the store's server on the controller's computer
// listens, as host:port, or nothing where the directory is not served.
func (c Config) StoreListen() (string, error) {
	data, err := os.ReadFile(filepath.Join(c.Dir, string(Controller), "listen"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(data)), err
}

// SetStoreListen notes where the store's server listens, or with nothing
// given that the directory is not served.
func (c Config) SetStoreListen(at string) error {
	if at == "" {
		err := os.Remove(filepath.Join(c.Dir, string(Controller), "listen"))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return writeWhole(filepath.Join(c.Dir, string(Controller), "listen"), []byte(at+"\n"))
}

func (c Config) trustPath(kind Kind, name string) string {
	return filepath.Join(c.Dir, string(kind), "trust", name)
}

// Trust reads what the runner or the client of this config holds about a
// controller.
func (c Config) Trust(kind Kind, controller string) (*Trust, error) {
	if err := CheckName(controller); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(c.trustPath(kind, controller))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t Trust
	if err := Decode(data, &t); err != nil || t.Card.Name != controller {
		return nil, fmt.Errorf("the trust this %s holds for %s is damaged", kind, controller)
	}
	return &t, nil
}

// SetTrust writes what the runner or the client holds about a controller.
func (c Config) SetTrust(kind Kind, t Trust) error {
	data, err := Encode(t)
	if err != nil {
		return err
	}
	return writeWhole(c.trustPath(kind, t.Card.Name), append(data, '\n'))
}

// TrustList reads every controller the runner or the client holds, removed
// ones included.
func (c Config) TrustList(kind Kind) ([]Trust, error) {
	entries, err := os.ReadDir(filepath.Join(c.Dir, string(kind), "trust"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var all []Trust
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		t, err := c.Trust(kind, e.Name())
		if err != nil {
			return nil, err
		}
		if t != nil {
			all = append(all, *t)
		}
	}
	slices.SortFunc(all, func(a, b Trust) int { return strings.Compare(a.Card.Name, b.Card.Name) })
	return all, nil
}
