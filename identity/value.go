package identity

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hpke"
	"crypto/rand"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
)

// ClockAllowance is how far two parties' clocks may differ. A receiver
// allows this much before it treats another party's time as passed.
const ClockAllowance = 5 * time.Minute

// A Value is something a party signed: three lines of text. The first is
// the frame, which says who signed it, which name in the store it belongs
// under, when it was written and when it stops being good. The second is
// the body, as JSON, or sealed so that only the parties it was sealed to can
// read it. The third is the signature over the first two.
//
// The frame is bound into the sealing, so nobody can wrap a sealed body in a
// frame of their own.
type Value struct {
	By     string    `json:"by"`
	At     string    `json:"at"`
	Sent   time.Time `json:"sent"`
	Until  time.Time `json:"until,omitzero"`
	Sealed bool      `json:"sealed,omitzero"`

	frame, body, sig []byte
}

var (
	// ErrAltered is a value whose signature does not hold, or that was
	// found under a name it was not signed for.
	ErrAltered = errors.New("the value has been altered or moved")
	// ErrNotForUs is a sealed value this identity cannot open.
	ErrNotForUs = errors.New("the value is not sealed to this party")
)

const sealInfo = "corbie seal 1"

var strict = json.RejectUnknownMembers(true)

// Encode writes v as JSON in the one form signed and stored values have:
// the same value always gives the same bytes.
func Encode(v any) ([]byte, error) { return json.Marshal(v, json.Deterministic(true)) }

// Decode reads JSON strictly: an unknown name, a repeated name or a name in
// the wrong case is an error.
func Decode(data []byte, into any) error { return json.Unmarshal(data, into, strict) }

// Write signs body as a value that belongs under the name at, sent now and
// good until the time given (for ever, if zero). With cards, the body is
// sealed to them and nobody else can read it.
func (id *Identity) Write(at string, now, until time.Time, body any, to ...Card) ([]byte, error) {
	frame, err := Encode(Value{By: id.Name, At: at, Sent: now.UTC(), Until: until.UTC(), Sealed: len(to) > 0})
	if err != nil {
		return nil, err
	}
	data, err := Encode(body)
	if err != nil {
		return nil, err
	}
	if len(to) > 0 {
		sealed, err := seal(data, frame, to)
		if err != nil {
			return nil, err
		}
		data = []byte(b64.EncodeToString(sealed))
	}
	signed := append(append(frame, '\n'), data...)
	sig := b64.EncodeToString(ed25519.Sign(id.signKey, signed))
	return append(append(append(signed, '\n'), sig...), '\n'), nil
}

// Parse reads a value's frame. Nothing is checked: until Check has passed,
// the frame is whatever its writer chose.
func Parse(data []byte) (Value, error) {
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) != 3 {
		return Value{}, ErrAltered
	}
	var v Value
	if err := Decode(lines[0], &v); err != nil {
		return Value{}, ErrAltered
	}
	sig, err := b64.DecodeString(string(lines[2]))
	if err != nil {
		return Value{}, ErrAltered
	}
	v.frame, v.body, v.sig = lines[0], lines[1], sig
	return v, nil
}

// Check says whether the value was signed by the card and belongs under the
// name it was found at.
func (v Value) Check(signer Card, at string) error {
	signed := append(append(bytes.Clone(v.frame), '\n'), v.body...)
	if signer.IsZero() || v.By != signer.Name || v.At != at || !ed25519.Verify(signer.sign, signed, v.sig) {
		return ErrAltered
	}
	return nil
}

// Good says whether the value is still good at now, by the reader's own
// clock, allowing for clocks that differ.
func (v Value) Good(now time.Time) bool {
	return v.Until.IsZero() || !now.After(v.Until.Add(ClockAllowance))
}

// Open reads the body into a Go value, opening it first if it is sealed.
// Call it only after Check.
func (v Value) Open(id *Identity, into any) error {
	body := v.body
	if v.Sealed {
		sealed, err := b64.DecodeString(string(body))
		if err != nil {
			return ErrAltered
		}
		if id == nil {
			return ErrNotForUs
		}
		if body, err = id.open(sealed, v.frame); err != nil {
			return err
		}
	}
	if err := Decode(body, into); err != nil {
		return fmt.Errorf("%w: %v", ErrAltered, err)
	}
	return nil
}

// Read is Parse, Check and Open together, for a value whose signer is known
// before it is read.
func Read(data []byte, at string, signer Card, id *Identity, into any) (Value, error) {
	v, err := Parse(data)
	if err == nil {
		err = v.Check(signer, at)
	}
	if err == nil {
		err = v.Open(id, into)
	}
	return v, err
}

// A sealed body is: how many parties it is sealed to; for each, the body's
// key sealed to that party; a nonce; and the body encrypted under the key
// with the frame bound in.
const wrappedLen = 32 + 32 + 16

func seal(plain, frame []byte, to []Card) ([]byte, error) {
	if len(to) > 255 {
		return nil, errors.New("sealed to too many parties")
	}
	key := make([]byte, 32)
	rand.Read(key)
	out := []byte{byte(len(to))}
	for _, card := range to {
		if card.IsZero() {
			return nil, errors.New("sealing to a party with no card")
		}
		pub, err := hpke.NewDHKEMPublicKey(card.seal)
		if err != nil {
			return nil, err
		}
		wrapped, err := hpke.Seal(pub, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), []byte(sealInfo), key)
		if err != nil {
			return nil, err
		}
		out = append(out, wrapped...)
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plain, frame), nil
}

func (id *Identity) open(sealed, frame []byte) ([]byte, error) {
	if len(sealed) < 1 {
		return nil, ErrAltered
	}
	n := int(sealed[0])
	rest := sealed[1:]
	if len(rest) < n*wrappedLen+12 {
		return nil, ErrAltered
	}
	priv, err := hpke.NewDHKEMPrivateKey(id.sealKey)
	if err != nil {
		return nil, err
	}
	body := rest[n*wrappedLen:]
	for i := range n {
		key, err := hpke.Open(priv, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), []byte(sealInfo), rest[i*wrappedLen:(i+1)*wrappedLen])
		if err != nil {
			continue
		}
		aead, err := newAEAD(key)
		if err != nil {
			return nil, err
		}
		plain, err := aead.Open(nil, body[:12], body[12:], frame)
		if err != nil {
			return nil, ErrAltered
		}
		return plain, nil
	}
	return nil, ErrNotForUs
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
