package identity

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// Has says whether the config holds an identity of the kind.
func (c Config) Has(kind Kind) bool { return Exists(c.identityPath(kind)) }

// Load reads the identity of the kind.
func (c Config) Load(kind Kind) (*Identity, error) {
	id, err := Load(c.identityPath(kind))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("this computer has no %s yet (corbie init makes one)", kind)
	}
	return id, err
}

// Make gives the identity of the kind, making it with the name if there is
// none. A name once chosen is never changed: an identity that exists is
// given as it is.
func (c Config) Make(kind Kind, name string) (*Identity, error) {
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
