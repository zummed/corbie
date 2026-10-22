package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

// KeptKey gives the key of a kept path from the key of a path at or above
// it, the top being the empty path: the key of each path below is a keyed
// hash of the key of the path above it and its own last part. A path that
// is not at or below the one whose key is given has no key here.
func KeptKey(key []byte, of, path string) ([]byte, bool) {
	rest := path
	if of != "" {
		var ok bool
		if rest, ok = strings.CutPrefix(path, of); !ok || rest != "" && rest[0] != '/' {
			return nil, false
		}
		rest = strings.TrimPrefix(rest, "/")
	}
	if rest == "" {
		return key, true
	}
	for part := range strings.SplitSeq(rest, "/") {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(part))
		key = h.Sum(nil)
	}
	return key, true
}

// SealKept encrypts a value to be kept at a path, under the path's key,
// with a nonce of its own and the path bound in. The key is the same for
// every value kept at the path for ever, and a nonce used twice under it
// would give both values away.
func SealKept(key []byte, path string, value []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(value)+aead.Overhead())
	rand.Read(nonce)
	return aead.Seal(nonce, nonce, value, []byte(path)), nil
}

// OpenKept decrypts a value kept at a path, and refuses one that was
// altered, or sealed for another path and moved here.
func OpenKept(key []byte, path string, sealed []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	altered := fmt.Errorf("the kept value at %s has been altered or moved", path)
	if len(sealed) < aead.NonceSize() {
		return nil, altered
	}
	value, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(path))
	if err != nil {
		return nil, altered
	}
	return value, nil
}
