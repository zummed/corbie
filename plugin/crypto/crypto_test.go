package crypto

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// newJob is a job that was sent the secrets given and is allowed what is
// listed: with nothing listed, no layer allows it anything.
func newJob(t *testing.T, secrets map[string]string, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), Secrets: secrets,
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}}}
	t.Cleanup(job.End)
	return job
}

func do(job *plugin.Job, name string, first any, opt plugin.Options) (any, error) {
	module, _ := plugin.Find("crypto")
	return module.Calls[name].Invoke(job, "crypto", name, first, opt)
}

func codeOf(err error) string {
	var known *plugin.Error
	if errors.As(err, &known) {
		return known.Code
	}
	return ""
}

// inPEM gives a key as a key file holds it.
func inPEM(kind string, der []byte, err error) string {
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
}

// A pair is a private key and the public key that goes with it, each as a
// key file holds it.
type pair struct {
	what            string
	private, public string
	key             gocrypto.Signer
}

// pairs makes a key of each kind, in each way a key file writes one.
func pairs(t *testing.T) []pair {
	t.Helper()
	ofRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ofEC, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, ofEd, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public := func(key gocrypto.Signer) string {
		der, err := x509.MarshalPKIXPublicKey(key.Public())
		return inPEM("PUBLIC KEY", der, err)
	}
	inPKCS8 := func(key gocrypto.Signer) string {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		return inPEM("PRIVATE KEY", der, err)
	}
	sec1, err := x509.MarshalECPrivateKey(ofEC)
	return []pair{
		{"an RSA key", inPKCS8(ofRSA), public(ofRSA), ofRSA},
		{"an RSA key written the older way", inPEM("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(ofRSA), nil), public(ofRSA), ofRSA},
		{"an EC key", inPKCS8(ofEC), public(ofEC), ofEC},
		{"an EC key written the older way", inPEM("EC PRIVATE KEY", sec1, err), public(ofEC), ofEC},
		{"an Ed25519 key", inPKCS8(ofEd), public(ofEd), ofEd},
	}
}

// checks says whether a signature is that data's, by the standard library
// alone: what any other holder of the public key would find.
func checks(key gocrypto.Signer, data, signature []byte) bool {
	hashed := sha256.Sum256(data)
	switch public := key.Public().(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(public, gocrypto.SHA256, hashed[:], signature) == nil
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(public, hashed[:], signature)
	case ed25519.PublicKey:
		return ed25519.Verify(public, data, signature)
	}
	return false
}

func TestHashesAndRandomBytesInAJobAllowedNothing(t *testing.T) {
	job := newJob(t, nil)
	for algorithm, want := range map[string]string{
		"sha256": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"sha1":   "a9993e364706816aba3e25717850c26c9cd0d89d",
		"md5":    "900150983cd24fb0d6963f7d28e17f72",
		"sha512": "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
	} {
		if got, err := do(job, "hash", "abc", plugin.Options{"algorithm": algorithm}); err != nil || got != want {
			t.Errorf("the %s hash of abc is %v, %v", algorithm, got, err)
		}
	}
	if got, err := do(job, "hash", "abc", nil); err != nil || got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("the hash of abc, with no algorithm named, is %v, %v", got, err)
	}
	// The keyed hash of abc under the key "key".
	if got, err := do(job, "hmac", "abc", plugin.Options{"key": "key"}); err != nil || got != "9c196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab" {
		t.Errorf("the keyed hash of abc is %v, %v", got, err)
	}
	first, err := do(job, "random", 16.0, plugin.Options{"as": "bytes"})
	second, _ := do(job, "random", 16.0, plugin.Options{"as": "bytes"})
	if err != nil || len(first.(plugin.Bytes)) != 16 || len(second.(plugin.Bytes)) != 16 || bytes.Equal(first.(plugin.Bytes), second.(plugin.Bytes)) {
		t.Errorf("sixteen random bytes, twice: %x and %x, %v", first, second, err)
	}
}

func TestSignatureChecksAsGoodAndAsBadOnceAByteChanges(t *testing.T) {
	job := newJob(t, nil)
	data := plugin.Bytes("the release, as it was built")
	for _, p := range pairs(t) {
		made, err := do(job, "sign", data, plugin.Options{"key": p.private})
		if err != nil {
			t.Errorf("%s: signing gave %v", p.what, err)
			continue
		}
		signature := made.(plugin.Bytes)
		if good, err := do(job, "verify", data, plugin.Options{"key": p.public, "signature": signature}); err != nil || good != true {
			t.Errorf("%s: its own signature checks as %v, %v", p.what, good, err)
		}
		// One byte of the data changed, and one byte of the signature.
		changed := bytes.Clone(data)
		changed[3] ^= 1
		spoilt := bytes.Clone(signature)
		spoilt[len(spoilt)/2] ^= 1
		for what, opt := range map[string][2]any{"the data": {changed, signature}, "the signature": {data, plugin.Bytes(spoilt)}} {
			if good, err := do(job, "verify", opt[0], plugin.Options{"key": p.public, "signature": opt[1]}); err != nil || good != false {
				t.Errorf("%s: with a byte of %s changed the signature checks as %v, %v", p.what, what, good, err)
			}
		}
		// The signature is one anybody with the public key can check, and
		// one made anywhere is checked here.
		if !checks(p.key, data, signature) {
			t.Errorf("%s: its signature is not one the standard library takes", p.what)
		}
		hashed, how := sha256.Sum256(data), gocrypto.SHA256
		what := hashed[:]
		if _, ed := p.key.(ed25519.PrivateKey); ed {
			what, how = data, 0
		}
		elsewhere, err := p.key.Sign(rand.Reader, what, how)
		if err != nil {
			t.Fatal(err)
		}
		if good, err := do(job, "verify", data, plugin.Options{"key": p.public, "signature": plugin.Bytes(elsewhere)}); err != nil || good != true {
			t.Errorf("%s: a signature made elsewhere checks as %v, %v", p.what, good, err)
		}
		// Another key's signature is no good.
		for _, other := range pairs(t) {
			if good, err := do(job, "verify", data, plugin.Options{"key": other.public, "signature": signature}); good == true {
				t.Errorf("%s: its signature checks as good against another key, %s (%v)", p.what, other.what, err)
			}
		}
	}
	// The hash a signature is made over is the one named, on both sides.
	rsaKey := pairs(t)[0]
	made, err := do(job, "sign", data, plugin.Options{"key": rsaKey.private, "algorithm": "sha512", "as": "base64"})
	if err != nil {
		t.Fatal(err)
	}
	signature, err := do(job, "decode", made, nil)
	if err != nil {
		t.Fatalf("a signature written as base64 is %v, which does not read back: %v", made, err)
	}
	for algorithm, want := range map[string]bool{"sha512": true, "sha256": false} {
		if good, err := do(job, "verify", data, plugin.Options{"key": rsaKey.public, "signature": signature, "algorithm": algorithm}); err != nil || good != want {
			t.Errorf("a signature over sha512, checked over %s, checks as %v, %v", algorithm, good, err)
		}
	}
}

func TestWhatWasEncryptedDecryptsToTheSameBytes(t *testing.T) {
	job := newJob(t, nil)
	data := plugin.Bytes("a note, with a zero byte \x00 and a letter beyond ASCII: é")
	for _, size := range []int{16, 24, 32} {
		key := make(plugin.Bytes, size)
		rand.Read(key)
		made, err := do(job, "encrypt", data, plugin.Options{"key": key})
		if err != nil {
			t.Fatalf("with a key of %d bytes, encrypting gave %v", size, err)
		}
		sealed := made.(plugin.Bytes)
		if bytes.Contains(sealed, data[:10]) {
			t.Fatalf("what was encrypted with a key of %d bytes holds the data as it was", size)
		}
		if got, err := do(job, "decrypt", sealed, plugin.Options{"key": key}); err != nil || !bytes.Equal(got.(plugin.Bytes), data) {
			t.Errorf("with a key of %d bytes, what was encrypted decrypts to %q, %v", size, got, err)
		}
		if got, err := do(job, "decrypt", sealed, plugin.Options{"key": key, "as": "text"}); err != nil || got != string(data) {
			t.Errorf("as text, what was encrypted decrypts to %q, %v", got, err)
		}
		// Encrypted again, the same data comes out different.
		again, _ := do(job, "encrypt", data, plugin.Options{"key": key})
		if bytes.Equal(again.(plugin.Bytes), sealed) {
			t.Errorf("the same data encrypted twice with one key came out the same")
		}
		// One byte changed, or another key, and nothing is given.
		spoilt := bytes.Clone(sealed)
		spoilt[len(spoilt)-20] ^= 1
		other := bytes.Clone(key)
		other[0] ^= 1
		for what, opt := range map[string][2]plugin.Bytes{"a byte changed": {spoilt, key}, "another key": {sealed, other}, "its end cut off": {sealed[:len(sealed)-1], key}, "nothing": {{}, key}} {
			if got, err := do(job, "decrypt", opt[0], plugin.Options{"key": opt[1]}); codeOf(err) != plugin.BadInput || got != nil {
				t.Errorf("with %s, decrypting gave %v, %v", what, got, err)
			}
		}
		// It is the usual cipher, written the usual way: twelve bytes of
		// its own first, then the data and its tag.
		block, _ := aes.NewCipher(key)
		usual, _ := cipher.NewGCM(block)
		if opened, err := usual.Open(nil, sealed[:12], sealed[12:], nil); err != nil || !bytes.Equal(opened, data) {
			t.Errorf("what was encrypted is not AES-GCM as the standard library opens it: %v", err)
		}
		start := make([]byte, 12)
		rand.Read(start)
		if got, err := do(job, "decrypt", plugin.Bytes(usual.Seal(start, start, data, nil)), plugin.Options{"key": key}); err != nil || !bytes.Equal(got.(plugin.Bytes), data) {
			t.Errorf("what was encrypted elsewhere decrypts to %q, %v", got, err)
		}
	}
	// A key of any other size is refused, and what is encrypted can be had
	// as text.
	for _, size := range []int{0, 15, 31, 33} {
		if _, err := do(job, "encrypt", data, plugin.Options{"key": make(plugin.Bytes, size)}); codeOf(err) != plugin.BadInput {
			t.Errorf("a key of %d bytes gave %v", size, err)
		}
	}
	text, err := do(job, "encrypt", "a note", plugin.Options{"key": "0123456789abcdef", "as": "base64"})
	if err != nil {
		t.Fatal(err)
	}
	back, _ := do(job, "decode", text, nil)
	if got, err := do(job, "decrypt", back, plugin.Options{"key": "0123456789abcdef", "as": "text"}); err != nil || got != "a note" {
		t.Errorf("encrypted as base64 and read back, the note is %v, %v", got, err)
	}
	// Text is not taken for what was encrypted: it would be read as bytes
	// it is not.
	if _, err := do(job, "decrypt", text, plugin.Options{"key": "0123456789abcdef"}); codeOf(err) != plugin.BadInput || !strings.Contains(err.Error(), "crypto.decode") {
		t.Errorf("decrypting text gave %v", err)
	}
}

// A secret used as a key gives ordinary data, so only one bound to
// anywhere may be: to sign, to encrypt, and to decrypt.
func TestSecretAsAKeyIsOneBoundToAnywhere(t *testing.T) {
	signing := pairs(t)[4]
	const cipherKey = "0123456789abcdef0123456789abcdef"
	secrets := map[string]string{"SIGNING": signing.private, "VAULT": cipherKey}
	anywhere := newJob(t, secrets, "secret:SIGNING@*", "secret:VAULT@*")
	bound := newJob(t, secrets, "secret:SIGNING@net:api.example", "secret:VAULT@net:api.example")
	data := plugin.Bytes("the body")
	sealed, _ := do(anywhere, "encrypt", data, plugin.Options{"key": cipherKey})
	for call, c := range map[string]struct {
		first  any
		secret string
	}{"sign": {data, "SIGNING"}, "encrypt": {data, "VAULT"}, "decrypt": {sealed, "VAULT"}} {
		opt := plugin.Options{"key": plugin.NewSecret(c.secret)}
		got, err := do(anywhere, call, c.first, opt)
		if _, isBytes := got.(plugin.Bytes); err != nil || !isBytes {
			t.Errorf("crypto.%s with a secret bound to anywhere gave %v, %v", call, got, err)
		}
		got, err = do(bound, call, c.first, opt)
		var refusal *permission.Refusal
		if !errors.As(err, &refusal) || refusal.What != "secret:"+c.secret+"@*" || !strings.Contains(refusal.Fix, "secret:"+c.secret+"@*") || got != nil {
			t.Errorf("crypto.%s with a secret bound to a host gave %v, %v", call, got, err)
		}
	}
	// What the key made is the key's work: the signature is that key's,
	// and what was encrypted opens with the value.
	signature, _ := do(anywhere, "sign", data, plugin.Options{"key": plugin.NewSecret("SIGNING")})
	if !checks(signing.key, data, signature.(plugin.Bytes)) {
		t.Error("the signature made with a secret is not that key's")
	}
	made, _ := do(anywhere, "encrypt", data, plugin.Options{"key": plugin.NewSecret("VAULT")})
	if got, err := do(anywhere, "decrypt", made, plugin.Options{"key": cipherKey}); err != nil || !bytes.Equal(got.(plugin.Bytes), data) {
		t.Errorf("what a secret encrypted decrypts with its value to %v, %v", got, err)
	}
	// A handle is a key and never the data: what comes back to the script
	// would hold the value in a form nothing searches for.
	handle := plugin.NewSecret("VAULT")
	for call, opt := range map[string]plugin.Options{
		"sign":    {"key": signing.private},
		"verify":  {"key": signing.public, "signature": signature},
		"encrypt": {"key": cipherKey},
		"decrypt": {"key": cipherKey},
	} {
		if got, err := do(anywhere, call, handle, opt); codeOf(err) != plugin.BadInput || got != nil {
			t.Errorf("crypto.%s of a handle gave %v, %v", call, got, err)
		}
	}
	if got, err := do(anywhere, "verify", data, plugin.Options{"key": signing.public, "signature": handle}); codeOf(err) != plugin.BadInput || got != nil {
		t.Errorf("a handle as the signature gave %v, %v", got, err)
	}
	// What is said of a key that is no good says nothing of the key: not
	// its text, and not how long it is.
	secrets = map[string]string{"SHORT": "a-key-of-23-bytes-long!", "NOKEY": "-----BEGIN PRIVATE KEY-----\nbm90IGEga2V5IGF0IGFsbCwgYnV0IGl0IGlzIGJhc2U2NA==\n-----END PRIVATE KEY-----\n"}
	odd := newJob(t, secrets, "secret:*@*")
	for call, name := range map[string]string{"encrypt": "SHORT", "sign": "NOKEY"} {
		_, err := do(odd, call, data, plugin.Options{"key": plugin.NewSecret(name)})
		if codeOf(err) != plugin.BadInput {
			t.Errorf("crypto.%s with a secret that is no key gave %v", call, err)
			continue
		}
		for _, part := range []string{secrets[name], "bm90IGEga2V5", "23"} {
			if strings.Contains(err.Error(), part) {
				t.Errorf("crypto.%s said of a secret that is no key: %v", call, err)
			}
		}
	}
}

func TestKeysAndSignaturesThatAreNotOnesAreRefused(t *testing.T) {
	job := newJob(t, nil)
	keys := pairs(t)
	rsaKey, ed := keys[0], keys[4]
	data := plugin.Bytes("the body")
	signature, _ := do(job, "sign", data, plugin.Options{"key": ed.private})
	for what, try := range map[string]func() (any, error){
		"signing with no key":              func() (any, error) { return do(job, "sign", data, nil) },
		"signing with text that is no key": func() (any, error) { return do(job, "sign", data, plugin.Options{"key": "hunter2"}) },
		"signing with a public key":        func() (any, error) { return do(job, "sign", data, plugin.Options{"key": ed.public}) },
		"checking with a private key": func() (any, error) {
			return do(job, "verify", data, plugin.Options{"key": ed.private, "signature": signature})
		},
		"checking with no signature": func() (any, error) { return do(job, "verify", data, plugin.Options{"key": ed.public}) },
		"checking a signature given as hex": func() (any, error) {
			return do(job, "verify", data, plugin.Options{"key": ed.public, "signature": "00ff"})
		},
		"a hash named for an Ed25519 key": func() (any, error) {
			return do(job, "sign", data, plugin.Options{"key": ed.private, "algorithm": "sha256"})
		},
		"a hash that is not one": func() (any, error) {
			return do(job, "sign", data, plugin.Options{"key": rsaKey.private, "algorithm": "sha3"})
		},
		"a certificate for a key": func() (any, error) {
			return do(job, "verify", data, plugin.Options{"key": inPEM("CERTIFICATE", []byte("not one"), nil), "signature": signature})
		},
		"an option that is not one": func() (any, error) {
			return do(job, "encrypt", data, plugin.Options{"key": "0123456789abcdef", "iv": "x"})
		},
	} {
		if got, err := try(); codeOf(err) != plugin.BadInput || got != nil {
			t.Errorf("%s gave %v, %v", what, got, err)
		}
	}
}
