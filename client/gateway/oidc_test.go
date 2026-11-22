package gateway_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
)

// issuer stands in for an OIDC issuer: it publishes its keys and signs
// tokens.
type issuer struct {
	t      *testing.T
	server *httptest.Server
	rsa    *rsa.PrivateKey
	ec     *ecdsa.PrivateKey
	// keys counts how often the keys were read.
	keys atomic.Int64
	// later says the issuer has changed its RSA key for another.
	later atomic.Bool
	next  *rsa.PrivateKey
}

func b64(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	i := &issuer{t: t}
	var err error
	if i.rsa, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	if i.next, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	if i.ec, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	i.server = httptest.NewServer(mux)
	t.Cleanup(i.server.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": i.server.URL, "jwks_uri": i.server.URL + "/keys"})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		i.keys.Add(1)
		key, id := &i.rsa.PublicKey, "rsa-1"
		if i.later.Load() {
			key, id = &i.next.PublicKey, "rsa-2"
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": id, "use": "sig", "n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())},
			{"kty": "EC", "kid": "ec-1", "crv": "P-256", "x": b64(i.ec.X.Bytes()), "y": b64(i.ec.Y.Bytes())},
			{"kty": "oct", "kid": "shared", "k": b64([]byte("a secret both ends know"))},
		}})
	})
	return i
}

// token signs claims as the issuer does. The header names the algorithm and
// the key.
func (i *issuer) token(alg, kid string, claims map[string]any) string {
	i.t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	body, _ := json.Marshal(claims)
	signed := b64(header) + "." + b64(body)
	digest := sha256.Sum256([]byte(signed))
	var signature []byte
	var err error
	switch {
	case alg == "ES256":
		var r, s *big.Int
		if r, s, err = ecdsa.Sign(rand.Reader, i.ec, digest[:]); err == nil {
			signature = append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
		}
	case kid == "rsa-2":
		signature, err = rsa.SignPKCS1v15(rand.Reader, i.next, crypto.SHA256, digest[:])
	case alg == "RS256":
		signature, err = rsa.SignPKCS1v15(rand.Reader, i.rsa, crypto.SHA256, digest[:])
	}
	if err != nil {
		i.t.Fatal(err)
	}
	return signed + "." + b64(signature)
}

// A gateway with an OIDC issuer takes a token the issuer signed for the
// gateway's audience: one of the claim's values names the client, and
// another claim is the person. A token that is forged, from another issuer,
// for another audience, past its time, signed in a way the key is not for,
// or that names no client or no person, is refused, and the refusal says
// why.
func Test_client_gateway_takes_an_issuers_tokens(t *testing.T) {
	s := newSeveral(t)
	viewer, editor := s.enrol("viewer"), s.enrol("editor", "at:/**")
	idp := newIssuer(t)
	var ahead atomic.Int64
	held := &gateway.Issuer{Address: idp.server.URL, Audience: "api://corbie", ClientClaim: "roles", PersonClaim: "preferred_username",
		Clients: []*client.Client{viewer, editor}, Now: func() time.Time { return time.Now().Add(time.Duration(ahead.Load())) }}
	if err := held.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"static": editor}, Issuer: held, Interval: 10 * time.Millisecond})
	t.Cleanup(server.Close)
	as := func(token string) caller { return caller{t: t, url: server.URL, token: token, client: editor} }
	claims := func(change map[string]any) map[string]any {
		all := map[string]any{"iss": idp.server.URL, "aud": "api://corbie", "exp": time.Now().Add(time.Hour).Unix(), "nbf": time.Now().Add(-time.Minute).Unix(),
			"sub": "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", "preferred_username": "alice@example.com", "roles": []string{"editor"}}
		for name, value := range change {
			if value == nil {
				delete(all, name)
			} else {
				all[name] = value
			}
		}
		return all
	}

	// The issuer's token names the client and the person.
	alice := as(idp.token("RS256", "rsa-1", claims(nil)))
	at := alice.place(door, nil)
	id := alice.start(map[string]any{"at": at})
	if !strings.HasPrefix(id, "editor/") {
		t.Fatalf("the run started with the issuer's token is %s", id)
	}
	var run map[string]any
	for range 500 {
		if _, run, _ = alice.do("GET", "/runs/"+id, ""); run["status"] == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cause, _ := run["cause"].(map[string]any); cause["client"] != "editor" || cause["person"] != "alice@example.com" {
		t.Fatalf("the run's cause is %v", run["cause"])
	}
	// A key on the curve P-256, an audience among several, a role among
	// several, and a claim that is one text.
	for name, token := range map[string]string{
		"ES256":             idp.token("ES256", "ec-1", claims(nil)),
		"several audiences": idp.token("RS256", "rsa-1", claims(map[string]any{"aud": []string{"other", "api://corbie"}})),
		"a role as text":    idp.token("RS256", "rsa-1", claims(map[string]any{"roles": "editor"})),
	} {
		if status, _, text := as(token).do("GET", "/runs", ""); status != 200 || !strings.Contains(text, id) {
			t.Errorf("a token with %s was answered %d %s", name, status, text)
		}
	}
	// The first of the gateway's clients the claim names.
	both := as(idp.token("RS256", "rsa-1", claims(map[string]any{"roles": []string{"editor", "viewer", "admin"}, "preferred_username": "bob@example.com"})))
	if called := both.start(map[string]any{"at": at}); !strings.HasPrefix(called, "viewer/") {
		t.Errorf("a token that names both clients started %s", called)
	}
	// A token of the gateway's own is still taken.
	if status, _, text := as("static").do("GET", "/runs", ""); status != 200 {
		t.Errorf("the gateway's own token was answered %d %s", status, text)
	}

	// What is refused, and why.
	forged := idp.token("RS256", "rsa-1", claims(nil))
	header, _ := json.Marshal(map[string]string{"alg": "none", "kid": "rsa-1"})
	body, _ := json.Marshal(claims(nil))
	hmac, _ := json.Marshal(map[string]string{"alg": "HS256", "kid": "shared"})
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	stranger := &issuer{t: t, rsa: other}
	for want, token := range map[string]string{
		"its signature is not the issuer's#changed":                                           forged[:len(forged)-4] + "AAAA",
		"its signature is not the issuer's#another's key":                                     stranger.token("RS256", "rsa-1", claims(nil)),
		"its signature is not the issuer's#none":                                              b64(header) + "." + b64(body) + ".",
		"its signature is not the issuer's#other alg":                                         idp.token("RS256", "ec-1", claims(nil)),
		`the key \"shared\", which the issuer does not`:                                       b64(hmac) + "." + b64(body) + "." + b64([]byte("x")),
		`the key \"unknown\", which the issuer does not`:                                      idp.token("RS256", "unknown", claims(nil)),
		"it is from the issuer https://elsewhere, and the gateway's is http://127.0.0.1":      idp.token("RS256", "rsa-1", claims(map[string]any{"iss": "https://elsewhere"})),
		"it is for the audience api://other":                                                  idp.token("RS256", "rsa-1", claims(map[string]any{"aud": "api://other"})),
		"its time has passed#exp":                                                             idp.token("RS256", "rsa-1", claims(map[string]any{"exp": time.Now().Add(-2 * time.Minute).Unix()})),
		"its time has passed#none":                                                            idp.token("RS256", "rsa-1", claims(map[string]any{"exp": nil})),
		"its time has not come":                                                               idp.token("RS256", "rsa-1", claims(map[string]any{"nbf": time.Now().Add(10 * time.Minute).Unix()})),
		"its claim roles is admin, which names none of the gateway's clients: viewer, editor": idp.token("RS256", "rsa-1", claims(map[string]any{"roles": []string{"admin"}})),
		"its claim roles is not there, which names none of the gateway's clients":             idp.token("RS256", "rsa-1", claims(map[string]any{"roles": nil})),
		"it has no claim preferred_username":                                                  idp.token("RS256", "rsa-1", claims(map[string]any{"preferred_username": nil})),
		"it is no token of the issuer's":                                                      "a.b.c",
		"it is no token of the issuer's#two parts":                                            "static.x",
	} {
		want, _, _ = strings.Cut(want, "#")
		status, _, text := as(token).do("POST", "/runs/"+id+"/cancel", "{}")
		if status != 401 || !strings.Contains(text, "the request's token is not taken") || !strings.Contains(text, want) {
			t.Errorf("a token that should be refused because %s was answered %d %s", want, status, text)
		}
	}
	if _, run, text := alice.do("GET", "/runs/"+id, ""); run["status"] != "running" {
		t.Errorf("after the refused cancels the run is %s", text)
	}
	// A token is taken until its time has passed, by the gateway's clock.
	short := as(idp.token("RS256", "rsa-1", claims(map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})))
	if status, _, text := short.do("GET", "/runs", ""); status != 200 {
		t.Fatalf("a token good for ten minutes was answered %d %s", status, text)
	}
	ahead.Store(int64(12 * time.Minute))
	if status, _, text := short.do("GET", "/runs", ""); status != 401 || !strings.Contains(text, "its time has passed") {
		t.Errorf("twelve minutes on, the token was answered %d %s", status, text)
	}

	// The issuer changes its key: the keys are read again for a key the
	// gateway does not have, and once a minute at most.
	read := idp.keys.Load()
	idp.later.Store(true)
	changed := as(idp.token("RS256", "rsa-2", claims(nil)))
	if status, _, text := changed.do("GET", "/runs", ""); status != 200 || idp.keys.Load() != read+1 {
		t.Fatalf("a token signed with the issuer's new key was answered %d %s, and the keys were read %d times more", status, text, idp.keys.Load()-read)
	}
	for range 5 {
		as(idp.token("RS256", "unknown", claims(nil))).do("GET", "/runs", "")
	}
	if more := idp.keys.Load() - read; more != 1 {
		t.Errorf("five tokens with a key the issuer does not publish had the keys read %d times more", more-1)
	}
	if status, _, text := alice.do("GET", "/runs", ""); status != 401 || !strings.Contains(text, `the key \"rsa-1\"`) {
		t.Errorf("a token signed with the key the issuer dropped was answered %d %s", status, text)
	}
}

// An issuer is https, or http on this computer, names itself as it was
// given, and is given with an audience.
func Test_client_gateway_issuer_is_checked_when_it_opens(t *testing.T) {
	idp := newIssuer(t)
	for want, i := range map[string]*gateway.Issuer{
		"is https, or http on this computer":         {Address: "http://issuer.example.com", Audience: "a"},
		"is https, or http on this computer#no host": {Address: "issuer", Audience: "a"},
		"with the audience its tokens must be for":   {Address: idp.server.URL},
		"names itself": {Address: idp.server.URL + "/", Audience: "a"},
		"/.well-known/openid-configuration answered": {Address: idp.server.URL + "/tenant", Audience: "a"},
	} {
		want, _, _ = strings.Cut(want, "#")
		if err := i.Open(context.Background()); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("an issuer that should be refused because it %s gave %v", want, err)
		}
	}
	// Keys the gateway does not take: a short RSA key alone.
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer": %q, "jwks_uri": %q}`, server.URL, server.URL+"/keys")
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": "weak", "n": b64(weak.N.Bytes()), "e": b64(big.NewInt(int64(weak.E)).Bytes())}}})
	})
	if err := (&gateway.Issuer{Address: server.URL, Audience: "a"}).Open(context.Background()); err == nil || !strings.Contains(err.Error(), "publishes no key the gateway takes") {
		t.Errorf("an issuer with a 1024-bit key alone gave %v", err)
	}
	// Keys published where another computer could change them on their way.
	var elsewhere *httptest.Server
	elsewhere = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer": %q, "jwks_uri": "http://keys.example.com/keys"}`, elsewhere.URL)
	}))
	defer elsewhere.Close()
	if err := (&gateway.Issuer{Address: elsewhere.URL, Audience: "a"}).Open(context.Background()); err == nil || !strings.Contains(err.Error(), "neither https nor http on this computer") {
		t.Errorf("an issuer whose keys are at http on another computer gave %v", err)
	}
}
