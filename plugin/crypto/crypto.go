// Package crypto is the plugin for hashes, keyed hashes, random bytes,
// signing and checking, encryption, and writing bytes as text.
package crypto

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash"

	"github.com/zummed/corbie/plugin"
)

// A hashing is one of the algorithms a hash is made with: how a hash of it
// is begun, and the name a signature knows it by.
type hashing struct {
	begin func() hash.Hash
	known crypto.Hash
}

var algorithms = map[string]hashing{
	"sha256": {sha256.New, crypto.SHA256}, "sha512": {sha512.New, crypto.SHA512},
	"sha1": {sha1.New, crypto.SHA1}, "md5": {md5.New, crypto.MD5},
}

func algorithm(opt plugin.Options) (hashing, error) {
	name, err := opt.Text("algorithm", "sha256")
	if err != nil {
		return hashing{}, err
	}
	h, ok := algorithms[name]
	if !ok {
		return hashing{}, plugin.Errorf(plugin.BadInput, "algorithm: is sha256, sha512, sha1 or md5, not %q", name)
	}
	return h, nil
}

// of gives the hash of data.
func (h hashing) of(data []byte) []byte {
	sum := h.begin()
	sum.Write(data)
	return sum.Sum(nil)
}

// keyOf reads the key of a call, written as PEM, as a key file holds it:
// a private key to sign with, or a public key to check with. It may be a
// handle to a secret bound to anywhere. What is said of a key that cannot
// be read holds nothing of the key.
func keyOf(job *plugin.Job, opt plugin.Options) (any, error) {
	text, err := job.Key(opt["key"])
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(text)
	if block == nil {
		return nil, plugin.Errorf(plugin.BadInput, "key: is a key as PEM text, as a key file holds it, from its -----BEGIN line to its -----END line")
	}
	var key any
	switch block.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	case "PUBLIC KEY":
		key, err = x509.ParsePKIXPublicKey(block.Bytes)
	default:
		return nil, plugin.Errorf(plugin.BadInput, "key: holds a %s, and a key is a PRIVATE KEY, an RSA PRIVATE KEY, an EC PRIVATE KEY or a PUBLIC KEY; one with a passphrase is not read", block.Type)
	}
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "key: the %s cannot be read", block.Type)
	}
	return key, nil
}

// signed is what a call signs or checks a signature of: for an Ed25519
// key the data itself, which that key signs with no hash to choose, and
// for any other key the hash of the data.
func signed(key any, data []byte, opt plugin.Options) ([]byte, crypto.Hash, error) {
	switch key.(type) {
	case ed25519.PrivateKey, ed25519.PublicKey:
		if opt.Has("algorithm") {
			return nil, 0, plugin.Errorf(plugin.BadInput, "algorithm: an Ed25519 key signs the data itself, and takes none")
		}
		return data, 0, nil
	}
	h, err := algorithm(opt)
	if err != nil {
		return nil, 0, err
	}
	return h.of(data), h.known, nil
}

// sealing gives the cipher of a call that encrypts or decrypts: AES in
// Galois counter mode, which shows whether what it is given back is what
// it made. Each thing it encrypts starts with random bytes of its own, so
// one key encrypts many things.
func sealing(job *plugin.Job, opt plugin.Options) (cipher.AEAD, error) {
	key, err := job.Key(opt["key"])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		// The size of a key that is a secret is not said.
		return nil, plugin.Errorf(plugin.BadInput, "key: is 16, 24 or 32 bytes: text, bytes, or a handle to a secret bound to anywhere")
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// written gives bytes in the form a script asked for.
func written(data []byte, opt plugin.Options, otherwise string) (any, error) {
	form, err := opt.Text("as", otherwise)
	if err != nil {
		return nil, err
	}
	switch form {
	case "hex":
		return hex.EncodeToString(data), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(data), nil
	case "base64url":
		return base64.RawURLEncoding.EncodeToString(data), nil
	case "bytes":
		return plugin.Bytes(data), nil
	}
	return nil, plugin.Errorf(plugin.BadInput, "as: is \"hex\", \"base64\", \"base64url\" or \"bytes\", not %q", form)
}

func init() {
	plugin.Register(&plugin.Module{Name: "crypto", Page: page, Calls: map[string]*plugin.Call{
		"hash": {Options: []string{"algorithm", "as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			h, err := algorithm(opt)
			if err != nil {
				return nil, err
			}
			return written(h.of(data), opt, "hex")
		}},
		"hmac": {Options: []string{"key", "algorithm", "as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			h, err := algorithm(opt)
			if err != nil {
				return nil, err
			}
			// A secret used as a key gives ordinary data, so only one bound
			// to anywhere may be used this way.
			key, err := job.Key(opt["key"])
			if err != nil {
				return nil, err
			}
			if len(key) == 0 {
				return nil, plugin.Errorf(plugin.BadInput, "crypto.hmac needs a key: text, bytes, or a handle to a secret bound to anywhere")
			}
			mac := hmac.New(h.begin, key)
			mac.Write(data)
			return written(mac.Sum(nil), opt, "hex")
		}},
		"sign": {Options: []string{"key", "algorithm", "as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			key, err := keyOf(job, opt)
			if err != nil {
				return nil, err
			}
			signer, ok := key.(crypto.Signer)
			if !ok {
				return nil, plugin.Errorf(plugin.BadInput, "key: crypto.sign takes a private key: RSA, EC or Ed25519")
			}
			what, hashed, err := signed(key, data, opt)
			if err != nil {
				return nil, err
			}
			signature, err := signer.Sign(rand.Reader, what, hashed)
			if err != nil {
				return nil, plugin.Errorf(plugin.BadInput, "key: this key cannot sign with this algorithm")
			}
			return written(signature, opt, "bytes")
		}},
		"verify": {Options: []string{"key", "signature", "algorithm"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			signature, ok := opt["signature"].(plugin.Bytes)
			if !ok {
				return nil, plugin.Errorf(plugin.BadInput, "signature: is the signature as bytes; one written as text is turned into bytes with crypto.decode")
			}
			key, err := keyOf(job, opt)
			if err != nil {
				return nil, err
			}
			what, hashed, err := signed(key, data, opt)
			if err != nil {
				return nil, err
			}
			switch public := key.(type) {
			case *rsa.PublicKey:
				return rsa.VerifyPKCS1v15(public, hashed, what, signature) == nil, nil
			case *ecdsa.PublicKey:
				return ecdsa.VerifyASN1(public, what, signature), nil
			case ed25519.PublicKey:
				return ed25519.Verify(public, what, signature), nil
			}
			return nil, plugin.Errorf(plugin.BadInput, "key: crypto.verify takes a public key: RSA, EC or Ed25519")
		}},
		"encrypt": {Options: []string{"key", "as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			sealer, err := sealing(job, opt)
			if err != nil {
				return nil, err
			}
			return written(sealer.Seal(nil, nil, data, nil), opt, "bytes")
		}},
		"decrypt": {Options: []string{"key", "as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			if _, text := first.(string); text {
				return nil, plugin.Errorf(plugin.BadInput, "crypto.decrypt takes what crypto.encrypt gave, as bytes; written as text it is turned into bytes with crypto.decode")
			}
			sealed, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			sealer, err := sealing(job, opt)
			if err != nil {
				return nil, err
			}
			data, err := sealer.Open(nil, nil, sealed, nil)
			if err != nil {
				return nil, plugin.Errorf(plugin.BadInput, "this was not encrypted with this key, or it has been changed since")
			}
			form, err := opt.Text("as", "bytes")
			if err != nil {
				return nil, err
			}
			return job.As(data, form, "")
		}},
		"random": {Options: []string{"as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			n, ok := first.(float64)
			if !ok || n < 1 || n > 1<<20 || n != float64(int(n)) {
				return nil, plugin.Errorf(plugin.BadInput, "crypto.random takes how many bytes first")
			}
			data := make([]byte, int(n))
			rand.Read(data)
			return written(data, opt, "hex")
		}},
		"uuid": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			var b [16]byte
			rand.Read(b[:])
			b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
			return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
		}},
		"encode": {Options: []string{"as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			data, _, err := job.Contents(first)
			if err != nil {
				return nil, err
			}
			return written(data, opt, "base64")
		}},
		"decode": {Options: []string{"from", "as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			text, ok := first.(string)
			if !ok {
				return nil, plugin.Errorf(plugin.BadInput, "crypto.decode takes text first")
			}
			from, err := opt.Text("from", "base64")
			if err != nil {
				return nil, err
			}
			var data []byte
			switch from {
			case "hex":
				data, err = hex.DecodeString(text)
			case "base64":
				data, err = base64.StdEncoding.DecodeString(text)
			case "base64url":
				data, err = base64.RawURLEncoding.DecodeString(text)
			default:
				return nil, plugin.Errorf(plugin.BadInput, "from: is \"hex\", \"base64\" or \"base64url\", not %q", from)
			}
			if err != nil {
				return nil, plugin.Errorf(plugin.BadInput, "the text is not %s: %v", from, err)
			}
			form, err := opt.Text("as", "bytes")
			if err != nil {
				return nil, err
			}
			return job.As(data, form, "")
		}},
	}})
}

const page = `crypto: this plugin hashes, signs, encrypts and encodes data, and makes random bytes.

  crypto.hash(data, { algorithm, as })                 the hash
  crypto.hmac(data, { key, algorithm, as })            the HMAC
  crypto.random(n, { as })                             n random bytes, 1048576 at most
  crypto.uuid()                                        a random UUID, as text
  crypto.sign(data, { key, algorithm, as })            the signature
  crypto.verify(data, { key, signature, algorithm })   true if the signature is of the data, false if not
  crypto.encrypt(data, { key, as })                    the data, encrypted
  crypto.decrypt(data, { key, as })                    the data that was encrypted
  crypto.encode(data, { as })                          the data as text
  crypto.decode(text, { from, as })                    the bytes that the text encodes

data is text, bytes or a blob of at most 64 MiB. For crypto.decrypt it is bytes
or a blob.

Options:
  algorithm   "sha256", "sha512", "sha1" or "md5"; default "sha256"
  from        "base64", "base64url" or "hex"; default "base64"
  signature   bytes; crypto.decode turns a signature in text into bytes

as is the form of the result.

  hash, hmac, random   "hex", "base64", "base64url" or "bytes"; default "hex"
  sign, encrypt        the same four; default "bytes"
  encode               "base64", "base64url" or "hex"; default "base64"
  decrypt, decode      "bytes", "text" or "json"; default "bytes"

key is text, bytes or a handle to a secret (` + "`corbie manual secrets`" + `).

  hmac               any text or bytes
  sign               a private key in PEM: PRIVATE KEY, RSA PRIVATE KEY or EC PRIVATE KEY
  verify             a public key in PEM: PUBLIC KEY
  encrypt, decrypt   16, 24 or 32 bytes, such as the 32 characters that openssl rand -hex 16 prints

- A key that signs or verifies is RSA, EC or Ed25519, with no passphrase.
- algorithm is the hash that is signed, the same to sign and to verify. An
  Ed25519 key signs the data itself and takes none.
- An RSA signature is PKCS #1 v1.5, and an EC signature is DER.
- crypto.verify returns false for a signature of other data, with no error.
- crypto.encrypt uses AES in Galois counter mode.
- Its result is 12 random bytes, the encrypted data and a tag of 16 bytes, so
  the same data never encrypts to the same bytes twice.
- crypto.decrypt with another key, or after a byte has changed, is an error.
- A call whose key is a handle returns ordinary data, which the script can
  send anywhere, so the secret's entry must be secret:NAME@*.
- data is never a handle.

Errors:
  bad_input   an unknown algorithm or form, a key of the wrong kind or size, a signature that is
              not bytes, text that is not in the encoding, or data that does not decrypt

Needs secret:NAME@* for a secret used as a key, and nothing otherwise.

These calls hash, encode, decode and make random values.

    crypto.hash("hello", { algorithm: "md5" })         // gives "5d41402abc4b2a76b9719d911017c592"
    crypto.encode("hi")                                // gives "aGk="
    crypto.decode("aGk=", { as: "text" })              // gives "hi"
    crypto.random(16).length                           // gives 32
    crypto.uuid().length                               // gives 36

This script signs with a secret private key and verifies with the public key.

    allow: ["secret:RELEASE_KEY@*"]

    const release = "corbie 1.4, as built"
    const signature = crypto.sign(release, { key: secrets.get("RELEASE_KEY") })
    crypto.encode(signature, { as: "hex" }).slice(0, 16)       // gives "e9bea10d9a6f5ab3"

    const theirs = ` + "`" + `-----BEGIN PUBLIC KEY-----
    MCowBQYDK2VwAyEAiRjIS/cn74dGizRuH5M9NgFrAsVHGRKz+313ziVPp6A=
    -----END PUBLIC KEY-----` + "`" + `
    crypto.verify(release, { key: theirs, signature })         // gives true
    crypto.verify("corbie 1.5", { key: theirs, signature })    // gives false

This script encrypts and decrypts with a secret key.

    allow: ["secret:VAULT_KEY@*"]

    const key = secrets.get("VAULT_KEY")
    const sealed = crypto.encrypt("the note", { key })
    sealed.length                                      // gives 36
    crypto.decrypt(sealed, { key, as: "text" })        // gives "the note"

This script computes an HMAC with a secret key.

    allow: ["secret:SIGNING@*"]

    const signed = crypto.hmac("the body", { key: secrets.get("SIGNING") })
    signed.length                                      // gives 64
`
