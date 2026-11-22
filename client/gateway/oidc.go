package gateway

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/client"
)

// An Issuer is an OIDC issuer whose tokens the gateway takes. A token it
// signed for the audience names a client of the gateway's by a claim, and a
// principal by another.
type Issuer struct {
	// Address is the issuer as its tokens name it, and where it publishes
	// its keys: https, or http on this computer.
	Address string
	// Audience is who a token must be for.
	Audience string
	// ClientClaim is the claim that names the client: text, or a list of
	// texts. PrincipalClaim is the claim that is the principal.
	ClientClaim, PrincipalClaim string
	// Clients are the gateway's clients. A token names the first whose name
	// is among its claim's values.
	Clients []*client.Client
	// Now is the gateway's clock.
	Now func() time.Time

	mu      sync.Mutex
	keysAt  string // where the issuer publishes its keys
	keys    map[string]crypto.PublicKey
	fetched time.Time
}

// leeway is how far an issuer's clock and the gateway's may differ.
const leeway = time.Minute

// refetch is how often, at most, the issuer's keys are read again for a
// token signed with a key the gateway does not have.
const refetch = time.Minute

// fetch limits what the gateway reads of the issuer: an answer in ten
// seconds, of a megabyte at most, and no redirect.
var fetch = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// Open reads where the issuer publishes its keys, and the keys. An issuer
// that cannot be read, or that names itself otherwise, is an error.
func (i *Issuer) Open(ctx context.Context) error {
	at, err := url.Parse(i.Address)
	if err != nil || !private(at) {
		return fmt.Errorf("an OIDC issuer's address is https, or http on this computer: %q is neither", i.Address)
	}
	if i.Audience == "" {
		return errors.New("an OIDC issuer is given with the audience its tokens must be for")
	}
	var found struct {
		Issuer string `json:"issuer"`
		Keys   string `json:"jwks_uri"`
	}
	if err := published(ctx, strings.TrimSuffix(i.Address, "/")+"/.well-known/openid-configuration", &found); err != nil {
		return fmt.Errorf("the OIDC issuer %s: %w", i.Address, err)
	}
	if found.Issuer != i.Address {
		return fmt.Errorf("the OIDC issuer at %s names itself %q: give that as the issuer, as its tokens do", i.Address, found.Issuer)
	}
	if keys, err := url.Parse(found.Keys); err != nil || !private(keys) {
		return fmt.Errorf("the OIDC issuer %s publishes its keys at %q, which is neither https nor http on this computer", i.Address, found.Keys)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.keysAt = found.Keys
	return i.readKeys(ctx)
}

// private says whether what is read from an address cannot be read or
// changed on its way: https, or http on this computer.
func private(at *url.URL) bool {
	host := at.Hostname()
	ip := net.ParseIP(host)
	return at.Host != "" && (at.Scheme == "https" || at.Scheme == "http" && (host == "localhost" || ip != nil && ip.IsLoopback()))
}

// published reads JSON from an address of the issuer's.
func published(ctx context.Context, address string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := fetch.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", address, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return stdjson.Unmarshal(data, into)
}

// readKeys reads the issuer's keys: its RSA keys of 2048 bits or more, and
// its keys on the curve P-256.
func (i *Issuer) readKeys(ctx context.Context) error {
	var set struct {
		Keys []struct {
			Kind  string `json:"kty"`
			ID    string `json:"kid"`
			Use   string `json:"use"`
			N, E  string
			Curve string `json:"crv"`
			X, Y  string
		} `json:"keys"`
	}
	i.fetched = i.now()
	if err := published(ctx, i.keysAt, &set); err != nil {
		return fmt.Errorf("the keys of the OIDC issuer %s: %w", i.Address, err)
	}
	number := func(text string) *big.Int {
		data, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil || len(data) == 0 {
			return nil
		}
		return new(big.Int).SetBytes(data)
	}
	keys := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		switch {
		case k.Kind == "RSA":
			if n, e := number(k.N), number(k.E); n != nil && e != nil && e.IsInt64() && n.BitLen() >= 2048 {
				keys[k.ID] = &rsa.PublicKey{N: n, E: int(e.Int64())}
			}
		case k.Kind == "EC" && k.Curve == "P-256":
			if x, y := number(k.X), number(k.Y); x != nil && y != nil && elliptic.P256().IsOnCurve(x, y) {
				keys[k.ID] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
			}
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("the OIDC issuer %s publishes no key the gateway takes: an RSA key of 2048 bits or more, or one on the curve P-256", i.Address)
	}
	i.keys = keys
	return nil
}

func (i *Issuer) now() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

// key gives the issuer's key of an ID. For one the gateway does not have,
// the keys are read again, once a minute at most: an issuer changes its
// keys from time to time.
func (i *Issuer) key(ctx context.Context, id string) crypto.PublicKey {
	i.mu.Lock()
	defer i.mu.Unlock()
	if key, has := i.keys[id]; has {
		return key
	}
	if i.now().Sub(i.fetched) < refetch {
		return nil
	}
	if err := i.readKeys(ctx); err != nil {
		return nil
	}
	return i.keys[id]
}

// Caller gives the client a token names, sending for the token's principal,
// or why the token is not taken. A token is taken when the issuer signed
// it, it names the issuer and the audience, its time has come and has not
// passed, one of its client claim's values is a client of the gateway's,
// and it has the principal's claim.
func (i *Issuer) Caller(ctx context.Context, token string) (*client.Client, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("it is no token of the issuer's")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Key       string `json:"kid"`
	}
	var claims map[string]any
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || segment(parts[0], &header) != nil || segment(parts[1], &claims) != nil {
		return nil, errors.New("it is no token of the issuer's")
	}
	key := i.key(ctx, header.Key)
	if key == nil {
		return nil, fmt.Errorf("it is signed with the key %q, which the issuer does not publish", header.Key)
	}
	// The key says which algorithm it is for, and the token must name that
	// one: a token never chooses how it is checked.
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	switch key := key.(type) {
	case *rsa.PublicKey:
		if header.Algorithm != "RS256" || rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
			return nil, errors.New("its signature is not the issuer's")
		}
	case *ecdsa.PublicKey:
		if header.Algorithm != "ES256" || len(signature) != 64 ||
			!ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
			return nil, errors.New("its signature is not the issuer's")
		}
	default:
		return nil, errors.New("its signature is not the issuer's")
	}
	now := i.now()
	expires, timed := claims["exp"].(float64)
	begins, _ := claims["nbf"].(float64)
	switch {
	case claims["iss"] != i.Address:
		return nil, fmt.Errorf("it is from the issuer %v, and the gateway's is %s", claims["iss"], i.Address)
	case !slices.Contains(texts(claims["aud"]), i.Audience):
		return nil, fmt.Errorf("it is for the audience %v, and the gateway's is %s", claims["aud"], i.Audience)
	case !timed || now.After(time.Unix(int64(expires), 0).Add(leeway)):
		return nil, errors.New("its time has passed")
	case now.Add(leeway).Before(time.Unix(int64(begins), 0)):
		return nil, errors.New("its time has not come")
	}
	principal, _ := claims[i.PrincipalClaim].(string)
	if principal == "" {
		return nil, fmt.Errorf("it has no claim %s, which names the principal", i.PrincipalClaim)
	}
	named := texts(claims[i.ClientClaim])
	var clients []string
	for _, c := range i.Clients {
		if slices.Contains(named, c.ID.Name) {
			return c.As(principal), nil
		}
		clients = append(clients, c.ID.Name)
	}
	return nil, fmt.Errorf("its claim %s is %s, which names none of the gateway's clients: %s", i.ClientClaim, listed(named), strings.Join(clients, ", "))
}

// segment reads one part of a token: JSON, in base64.
func segment(text string, into any) error {
	data, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return err
	}
	return stdjson.Unmarshal(data, into)
}

// listed writes a claim's values for a refusal, or that it has none.
func listed(values []string) string {
	if len(values) == 0 {
		return "not there"
	}
	return strings.Join(values, ", ")
}

// texts gives a claim's values: a text, or each text of a list.
func texts(claim any) []string {
	switch claim := claim.(type) {
	case string:
		return []string{claim}
	case []any:
		var out []string
		for _, value := range claim {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}
