// Package secret is how a secret is kept, bound to a destination, handed to
// a script and kept out of what comes back.
package secret

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A controller secret is kept in the store as its value, sealed so that
// only the controller can read it.

// Set keeps a controller secret, in place of any kept before under the
// name.
func Set(ctx context.Context, s store.Store, ctl *identity.Identity, name, value string) error {
	if err := permission.CheckSecretName(name); err != nil {
		return err
	}
	at := store.Secret(ctl.Name, name)
	data, err := ctl.Write(at, time.Now(), time.Time{}, value, ctl.Card)
	if err != nil {
		return err
	}
	_, err = s.Write(ctx, at, data, store.Overwrite)
	return err
}

// Get reads a controller secret, and says whether one is set.
func Get(ctx context.Context, s store.Store, ctl *identity.Identity, name string) (value string, set bool, err error) {
	if permission.CheckSecretName(name) != nil {
		return "", false, nil
	}
	at := store.Secret(ctl.Name, name)
	data, mark, err := s.Read(ctx, at)
	if err != nil || mark == store.None {
		return "", false, err
	}
	if _, err := identity.Read(data, at, ctl.Card, ctl, &value); err != nil {
		return "", false, fmt.Errorf("the secret %q: %w", name, err)
	}
	return value, true, nil
}

// Remove removes a controller secret.
func Remove(ctx context.Context, s store.Store, ctl *identity.Identity, name string) error {
	if err := permission.CheckSecretName(name); err != nil {
		return err
	}
	_, err := s.Delete(ctx, store.Secret(ctl.Name, name), store.Overwrite)
	return err
}

// List gives the names of the controller's secrets.
func List(ctx context.Context, s store.Store, ctl string) ([]string, error) {
	names, err := s.List(ctx, store.Secrets(ctl))
	if err != nil {
		return nil, err
	}
	for i, name := range names {
		names[i] = name[strings.LastIndexByte(name, '/')+1:]
	}
	return names, nil
}

// Named gives the secrets a job names: those its allow entries write out
// in full. An entry that covers secrets by a pattern permits them and names
// none.
func Named(allow []string) []string {
	var names []string
	for _, entry := range allow {
		rest, ok := strings.CutPrefix(entry, "secret:")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, "@")
		if name != "" && !strings.Contains(name, "*") && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}
