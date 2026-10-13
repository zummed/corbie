package identity

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// Trust is what one party holds about another it has taken the card of:
// the card, and the limit trust was given with. A controller gives a client
// a ceiling and a runner its tags; a runner gives a controller a ceiling,
// and notes the address of that controller's store, as a client does.
//
// A removed party stays on record with its key, so its name stays tied to
// that key.
type Trust struct {
	Card    Card             `json:"card"`
	Ceiling permission.Layer `json:"ceiling,omitzero"`
	Tags    []string         `json:"tags,omitzero"`
	Store   string           `json:"store,omitzero"`
	Removed bool             `json:"removed,omitzero"`
}

// Trusted says whether anything from the party is accepted.
func (t *Trust) Trusted() bool { return t != nil && !t.Removed }

// Take is what trusting a card gives, given what is on record under its
// name. A name stays tied to the key it was first taken with, also after
// the party was removed: a different key is refused unless replace says it
// takes the old one's place. What the old key was given is not carried over.
func Take(old *Trust, card Card, want Kind, replace bool) (Trust, error) {
	if card.Kind != want && want != "" {
		return Trust{}, fmt.Errorf("this is the card of a %s, and a %s's is needed", card.Kind, want)
	}
	if old != nil && !old.Card.Same(card) && !replace {
		return Trust{}, fmt.Errorf("the name %q is already used with another key (%s); to let this card (%s) take its place, say --replace",
			card.Name, old.Card.Fingerprint(), card.Fingerprint())
	}
	return Trust{Card: card}, nil
}

// ReadTrust reads what a controller holds about a party from its store. It
// is read afresh each time it is asked for.
func ReadTrust(ctx context.Context, s store.Store, ctl Card, party string) (*Trust, error) {
	if err := CheckName(party); err != nil {
		return nil, nil
	}
	name := store.Trust(ctl.Name, party)
	data, mark, err := s.Read(ctx, name)
	if err != nil || mark == store.None {
		return nil, err
	}
	var t Trust
	if _, err := Read(data, name, ctl, nil, &t); err != nil {
		return nil, fmt.Errorf("the trust held for %s: %w", party, err)
	}
	if t.Card.Name != party {
		return nil, fmt.Errorf("the trust held for %s: %w", party, ErrAltered)
	}
	return &t, nil
}

// WriteTrust writes what a controller holds about a party into its store.
func WriteTrust(ctx context.Context, s store.Store, ctl *Identity, t Trust) error {
	name := store.Trust(ctl.Name, t.Card.Name)
	data, err := ctl.Write(name, time.Now(), time.Time{}, t)
	if err != nil {
		return err
	}
	_, err = s.Write(ctx, name, data, store.Overwrite)
	return err
}

// ListTrust reads everything a controller holds about other parties.
func ListTrust(ctx context.Context, s store.Store, ctl Card) ([]Trust, error) {
	names, err := s.List(ctx, store.TrustList(ctl.Name))
	if err != nil {
		return nil, err
	}
	var all []Trust
	for _, name := range names {
		t, err := ReadTrust(ctx, s, ctl, name[strings.LastIndexByte(name, '/')+1:])
		if err != nil {
			return nil, err
		}
		if t != nil {
			all = append(all, *t)
		}
	}
	return all, nil
}

// HasTags says whether the party was given every one of the tags.
func (t Trust) HasTags(tags []string) bool {
	for _, tag := range tags {
		if !slices.Contains(t.Tags, tag) {
			return false
		}
	}
	return true
}
