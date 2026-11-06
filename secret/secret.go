// Package secret is how a secret is kept, bound to a destination, handed to
// a script and kept out of what comes back.
package secret

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A secret, and a value, is kept in the store at a location, sealed so that
// only the controller can read it. A run uses those set at its location or
// above, and the one set nearest its location wins.

// An Entry is a secret or a value and the location it is set at. A
// secret's Value is never given.
type Entry struct {
	Name, At, Value string
}

// Set keeps a secret at a location, in place of any kept there before
// under the name.
func Set(ctx context.Context, s store.Store, ctl *identity.Identity, at, name, value string) error {
	if err := permission.CheckSecretName(name); err != nil {
		return err
	}
	return keep(ctx, s, ctl, "secret", name, at, store.Secret(ctl.Name, at, name), value)
}

// SetValue keeps a value at a location, in place of any kept there before
// under the name.
func SetValue(ctx context.Context, s store.Store, ctl *identity.Identity, at, name, value string) error {
	if err := checkValueName(name); err != nil {
		return err
	}
	return keep(ctx, s, ctl, "value", name, at, store.Value(ctl.Name, at, name), value)
}

func keep(ctx context.Context, s store.Store, ctl *identity.Identity, kind, name, at, path, value string) error {
	if err := store.CheckLocation(at); err != nil {
		return err
	}
	if _, err := held(ctx, s, ctl, kind, name, at, path); err != nil {
		return err
	}
	data, err := ctl.Write(path, time.Now(), time.Time{}, value, ctl.Card)
	if err != nil {
		return err
	}
	_, err = s.Write(ctx, path, data, store.Overwrite)
	return err
}

// held says whether a secret or a value is kept under a path. On a file
// system that ignores case, /Work and /work are one directory: it refuses
// where what is kept there was set at a location of another case.
func held(ctx context.Context, s store.Store, ctl *identity.Identity, kind, name, at, path string) (bool, error) {
	data, mark, err := s.Read(ctx, path)
	if err != nil || mark == store.None {
		return false, err
	}
	if v, err := identity.Parse(data); err == nil && v.At != path && strings.EqualFold(v.At, path) {
		other, _, _ := store.ParseLocation(ctl.Name, v.At)
		return true, fmt.Errorf("the %s %s is set at %s, which this store keeps in the same file as %s because its file system ignores case", kind, name, other, at)
	}
	return true, nil
}

// Get reads the secret a run at a location uses: the one set nearest it,
// at it or above. It says whether one is set.
func Get(ctx context.Context, s store.Store, ctl *identity.Identity, location, name string) (value string, set bool, err error) {
	if permission.CheckSecretName(name) != nil || store.CheckLocation(location) != nil {
		return "", false, nil
	}
	for _, at := range store.Above(location) {
		value, set, err := open(ctx, s, ctl, store.Secret(ctl.Name, at, name))
		if err != nil {
			return "", false, fmt.Errorf("the secret %q set at %s: %w", name, at, err)
		}
		if set {
			return value, true, nil
		}
	}
	return "", false, nil
}

// Values reads the values a run at a location uses: for each name, the one
// set nearest it, at it or above.
func Values(ctx context.Context, s store.Store, ctl *identity.Identity, location string) (map[string]string, error) {
	if err := store.CheckLocation(location); err != nil {
		return nil, err
	}
	values := map[string]string{}
	above := store.Above(location)
	for i := len(above) - 1; i >= 0; i-- {
		listed, err := store.Names(s.List(ctx, store.Values(ctl.Name, above[i])))
		if err != nil {
			return nil, err
		}
		for _, name := range listed {
			value, set, err := open(ctx, s, ctl, name)
			if err != nil {
				return nil, fmt.Errorf("the value %q set at %s: %w", base(name), above[i], err)
			}
			if set {
				values[base(name)] = value
			}
		}
	}
	return values, nil
}

// Missing gives the secrets and the values, of those named, that are not
// set at a location or above, each as "secret NAME" or "value NAME".
func Missing(ctx context.Context, s store.Store, ctl *identity.Identity, location string, secrets, values []string) ([]string, error) {
	var missing []string
	for _, name := range secrets {
		if _, set, err := Get(ctx, s, ctl, location, name); err != nil {
			return nil, err
		} else if !set {
			missing = append(missing, "secret "+name)
		}
	}
	set, err := Values(ctx, s, ctl, location)
	if err != nil {
		return nil, err
	}
	for _, name := range values {
		if _, ok := set[name]; !ok {
			missing = append(missing, "value "+name)
		}
	}
	return missing, nil
}

// open reads a secret or a value kept under a name, and says whether one
// is there.
func open(ctx context.Context, s store.Store, ctl *identity.Identity, name string) (value string, set bool, err error) {
	data, mark, err := s.Read(ctx, name)
	if err != nil || mark == store.None {
		return "", false, err
	}
	if _, err := identity.Read(data, name, ctl.Card, ctl, &value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

// Remove removes a secret set at a location. One set above it is then the
// one used.
func Remove(ctx context.Context, s store.Store, ctl *identity.Identity, at, name string) error {
	if err := permission.CheckSecretName(name); err != nil {
		return err
	}
	return remove(ctx, s, ctl, "secret", name, at, store.Secret(ctl.Name, at, name))
}

// RemoveValue removes a value set at a location. One set above it is then
// the one used.
func RemoveValue(ctx context.Context, s store.Store, ctl *identity.Identity, at, name string) error {
	if err := checkValueName(name); err != nil {
		return err
	}
	return remove(ctx, s, ctl, "value", name, at, store.Value(ctl.Name, at, name))
}

func remove(ctx context.Context, s store.Store, ctl *identity.Identity, kind, name, at, path string) error {
	if err := store.CheckLocation(at); err != nil {
		return err
	}
	set, err := held(ctx, s, ctl, kind, name, at, path)
	if err != nil {
		return err
	}
	if !set {
		return fmt.Errorf("the %s %s is not set at %s", kind, name, at)
	}
	_, err = s.Delete(ctx, path, store.Overwrite)
	return err
}

// List gives the names of the secrets set at every location, and where
// each is set. There is no index of them: it lists every location's
// entries, files, blobs and keys among them, and a directory store reads
// every file it lists.
func List(ctx context.Context, s store.Store, ctl string) ([]Entry, error) {
	return list(ctx, s, ctl, "secrets")
}

// ListValues gives the values set at every location, and where each is
// set.
func ListValues(ctx context.Context, s store.Store, ctl *identity.Identity) ([]Entry, error) {
	entries, err := list(ctx, s, ctl.Name, "values")
	if err != nil {
		return nil, err
	}
	for i, e := range entries {
		name := store.Value(ctl.Name, e.At, e.Name)
		if entries[i].Value, _, err = open(ctx, s, ctl, name); err != nil {
			return nil, fmt.Errorf("the value %q set at %s: %w", e.Name, e.At, err)
		}
	}
	return entries, nil
}

func list(ctx context.Context, s store.Store, ctl, kind string) ([]Entry, error) {
	names, err := store.Names(s.List(ctx, store.Locations(ctl)))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, name := range names {
		at, entry, ok := store.ParseLocation(ctl, name)
		if found, ok2 := strings.CutPrefix(entry, kind+"/"); ok && ok2 && !strings.Contains(found, "/") {
			entries = append(entries, Entry{Name: found, At: at})
		}
	}
	return entries, nil
}

// checkValueName refuses a name a value cannot have: a value's name
// follows the rule for a secret's.
func checkValueName(name string) error {
	if permission.CheckSecretName(name) != nil {
		return fmt.Errorf("%q is not a name a value can have: a name holds no @, :, =, *, $, {, }, slash, backslash or blank, and does not start with - or .", name)
	}
	return nil
}

func base(name string) string { return name[strings.LastIndexByte(name, '/')+1:] }

// Named gives the secrets a job names: those its allow entries write out
// in full. An entry that covers secrets by a pattern permits them and names
// none.
func Named(allow []string) []string {
	var names []string
	seen := map[string]bool{}
	for _, entry := range allow {
		rest, ok := strings.CutPrefix(entry, "secret:")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, "@")
		if name != "" && !strings.Contains(name, "*") && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
