package identity

import (
	"bytes"
	"errors"
	"fmt"
	"time"
)

// admitted is what a controller seals to a computer that is to become a
// member of its store: the controller's identity as it is kept, and the
// address the store's members are reached at.
type admitted struct {
	Identity []byte `json:"identity"`
	Store    string `json:"store"`
}

// admittedAt is the name what is sealed to a member belongs under.
const admittedAt = "member"

// Admit seals the controller's identity, and the address of its store, to
// the card of a computer that is to become a member. What it gives is the
// controller's card on a line and the sealed value after it: only the
// holder of the card's private key reads the identity.
func (id *Identity) Admit(to Card, store string, now time.Time) ([]byte, error) {
	if id.Kind != Controller || to.Kind != Member {
		return nil, errors.New("a controller's identity is sealed to a member's card")
	}
	kept, err := id.encode()
	if err != nil {
		return nil, err
	}
	sealed, err := id.Write(admittedAt, now, time.Time{}, admitted{kept, store}, to)
	if err != nil {
		return nil, err
	}
	return append([]byte(id.Card.String()+"\n"), sealed...), nil
}

// Admitted opens what Admit gave, as the member it was sealed to, and gives
// the controller's identity and the address of its store.
func (id *Identity) Admitted(text []byte) (*Identity, string, error) {
	bad := errors.New("this is not what corbie init store --admit printed: a controller's card, and its identity sealed to this computer's card")
	line, rest, ok := bytes.Cut(bytes.TrimSpace(bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))), []byte("\n"))
	if !ok {
		return nil, "", bad
	}
	card, err := ParseCard(string(line))
	if err != nil || card.Kind != Controller {
		return nil, "", bad
	}
	var got admitted
	if _, err := Read(rest, admittedAt, card, id, &got); errors.Is(err, ErrNotForUs) {
		return nil, "", fmt.Errorf("the controller's identity was sealed to another card than this computer's: corbie init store --card prints the card to admit")
	} else if err != nil {
		return nil, "", bad
	}
	ctl, err := decode(got.Identity, bad)
	if err != nil || !ctl.Card.Same(card) {
		return nil, "", bad
	}
	return ctl, got.Store, nil
}
