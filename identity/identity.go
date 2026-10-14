// Package identity is who the parties are: their keys and cards, how two
// parties come to trust each other, and how they sign and seal what they
// write.
package identity

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zummed/corbie/store"
)

// Kind is what a party is.
type Kind string

const (
	Controller Kind = "controller"
	Runner     Kind = "runner"
	Client     Kind = "client"
)

func (k Kind) valid() bool { return k == Controller || k == Runner || k == Client }

// A Card is the public side of an identity: a name, a kind and two public
// keys, signed by itself. Its text form is one line.
type Card struct {
	Name string
	Kind Kind
	sign ed25519.PublicKey
	seal *ecdh.PublicKey
	sig  []byte
}

const cardPrefix = "corbie-card"

var b64 = base64.RawURLEncoding

// CheckName says whether a party may have the name: it is one part of a
// store's names, of letters, digits, dots, dashes and underscores.
func CheckName(name string) error {
	if err := store.CheckPart(name); err != nil || len(name) > 64 {
		return fmt.Errorf("%q is not a name a party can have", name)
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("%q is not a name a party can have: use letters, digits, '.', '-' and '_'", name)
		}
	}
	return nil
}

func (c Card) signed() []byte {
	return []byte(strings.Join([]string{cardPrefix, string(c.Kind), c.Name,
		b64.EncodeToString(c.sign), b64.EncodeToString(c.seal.Bytes())}, ":"))
}

// String is the card as one line of text.
func (c Card) String() string {
	if c.sign == nil {
		return ""
	}
	return string(c.signed()) + ":" + b64.EncodeToString(c.sig)
}

// IsZero says whether the card is no card at all.
func (c Card) IsZero() bool { return c.sign == nil }

// Same says whether two cards hold the same keys.
func (c Card) Same(o Card) bool {
	return !c.IsZero() && !o.IsZero() && c.sign.Equal(o.sign) && c.seal.Equal(o.seal)
}

// Fingerprint is a short mark of the card, made from its name, its kind and
// both of its keys, for two people to compare.
func (c Card) Fingerprint() string {
	sum := sha256.Sum256(c.signed())
	h := hex.EncodeToString(sum[:8])
	return h[0:4] + "-" + h[4:8] + "-" + h[8:12] + "-" + h[12:16]
}

// ParseCard reads a card from its line of text and checks its signature.
func ParseCard(line string) (Card, error) {
	bad := errors.New("this is not a valid card")
	parts := strings.Split(strings.TrimSpace(line), ":")
	if len(parts) != 6 || parts[0] != cardPrefix {
		return Card{}, bad
	}
	c := Card{Kind: Kind(parts[1]), Name: parts[2]}
	sign, err1 := b64.DecodeString(parts[3])
	seal, err2 := b64.DecodeString(parts[4])
	sig, err3 := b64.DecodeString(parts[5])
	if err1 != nil || err2 != nil || err3 != nil || len(sign) != ed25519.PublicKeySize ||
		!c.Kind.valid() || CheckName(c.Name) != nil {
		return Card{}, bad
	}
	key, err := ecdh.X25519().NewPublicKey(seal)
	if err != nil {
		return Card{}, bad
	}
	c.sign, c.seal, c.sig = sign, key, sig
	if !ed25519.Verify(c.sign, c.signed(), sig) {
		return Card{}, bad
	}
	return c, nil
}

func (c Card) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

func (c *Card) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*c = Card{}
		return nil
	}
	card, err := ParseCard(string(text))
	*c = card
	return err
}

// An Identity is a party's two private keys, with its card.
type Identity struct {
	Card
	signKey ed25519.PrivateKey
	sealKey *ecdh.PrivateKey
}

// New makes an identity with new keys.
func New(name string, kind Kind) (*Identity, error) {
	if err := CheckName(name); err != nil {
		return nil, err
	}
	if !kind.valid() {
		return nil, fmt.Errorf("%q is not a kind of party", kind)
	}
	_, signKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sealKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return newIdentity(name, kind, signKey, sealKey), nil
}

func newIdentity(name string, kind Kind, signKey ed25519.PrivateKey, sealKey *ecdh.PrivateKey) *Identity {
	id := &Identity{signKey: signKey, sealKey: sealKey}
	id.Card = Card{Name: name, Kind: kind, sign: signKey.Public().(ed25519.PublicKey), seal: sealKey.PublicKey()}
	id.Card.sig = ed25519.Sign(signKey, id.Card.signed())
	return id
}

// Proof is what the identity shows of itself to the other end of a served
// directory's wire: that it holds its signing key.
func (id *Identity) Proof() (tls.Certificate, error) { return store.Proof(id.signKey) }

// SignKey is the key a card's signatures are checked with.
func (c Card) SignKey() ed25519.PublicKey { return c.sign }

// identityFile is an identity as it is kept on disk.
type identityFile struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	Sign string `json:"sign"`
	Seal string `json:"seal"`
}

// Save writes the identity to a file only its owner can read, whole or not
// at all.
func (id *Identity) Save(path string) error {
	data, err := json.Marshal(identityFile{id.Name, id.Kind,
		b64.EncodeToString(id.signKey.Seed()), b64.EncodeToString(id.sealKey.Bytes())})
	if err != nil {
		return err
	}
	return writeWhole(path, append(data, '\n'))
}

// Load reads an identity from its file. A file that anyone but its owner
// can read is refused. No error it gives holds anything from the file.
func Load(path string) (*Identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("the identity file %q can be read by others (mode %o); its key is no longer its owner's alone. If that is not so, run: chmod 600 %s",
			path, info.Mode().Perm(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	damaged := fmt.Errorf("the identity file %q is damaged", path)
	var f identityFile
	if json.Unmarshal(data, &f, json.RejectUnknownMembers(true)) != nil || CheckName(f.Name) != nil || !f.Kind.valid() {
		return nil, damaged
	}
	sign, err1 := b64.DecodeString(f.Sign)
	seal, err2 := b64.DecodeString(f.Seal)
	if err1 != nil || err2 != nil || len(sign) != ed25519.SeedSize {
		return nil, damaged
	}
	sealKey, err := ecdh.X25519().NewPrivateKey(seal)
	if err != nil {
		return nil, damaged
	}
	return newIdentity(f.Name, f.Kind, ed25519.NewKeyFromSeed(sign), sealKey), nil
}

// Exists says whether there is an identity file at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// writeWhole writes a file only its owner can read, in one move.
func writeWhole(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
