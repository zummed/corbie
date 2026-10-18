package mcp

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

// client is a client's credentials, which a sign-in server takes for a
// token to the MCP server.
type client struct {
	id     string
	secret *plugin.Secret
	key    *plugin.Secret
	scope  string
}

func clientOf(opt plugin.Options) (*client, error) {
	given, err := opt.Map("client")
	if given == nil || err != nil {
		return nil, err
	}
	for name := range given {
		if !slices.Contains([]string{"id", "secret", "key", "scope"}, name) {
			return nil, plugin.Errorf(plugin.BadInput, "client takes id, secret or key, and scope, and not %s", name)
		}
	}
	c := &client{}
	c.id, _ = given["id"].(string)
	c.scope, _ = given["scope"].(string)
	c.secret, _ = given["secret"].(*plugin.Secret)
	c.key, _ = given["key"].(*plugin.Secret)
	switch {
	case c.id == "":
		return nil, plugin.Errorf(plugin.BadInput, "client takes the client's id, as text")
	case (given["secret"] != nil && c.secret == nil) || (given["key"] != nil && c.key == nil):
		return nil, plugin.Errorf(plugin.BadInput, "a client's secret or key is a handle to a secret, as secrets.get gives")
	case (c.secret == nil) == (c.key == nil):
		return nil, plugin.Errorf(plugin.BadInput, "client takes a secret or a key, and not both")
	}
	return c, nil
}

// token signs in for a server that refused a request, as its challenge
// says, and gives the token to send it.
//
//  1. The server's metadata names its sign-in server.
//  2. The sign-in server's metadata names where tokens are had.
//  3. The client's credentials are sent there, for a token to this server.
func (c *client) token(ctx context.Context, w *web, header string) (string, error) {
	said := challenge(header)
	resource, err := c.resource(ctx, w, said["resource_metadata"])
	if err != nil {
		return "", err
	}
	servers, _ := resource["authorization_servers"].([]any)
	issuer := ""
	if len(servers) > 0 {
		issuer, _ = servers[0].(string)
	}
	if issuer == "" {
		return "", plugin.Errorf(plugin.Failed, "the server %s names no sign-in server", w.to.Host)
	}
	meta, err := c.issuer(ctx, w, issuer)
	if err != nil {
		return "", err
	}
	endpoint, _ := meta["token_endpoint"].(string)
	to, err := address(endpoint)
	if err != nil {
		return "", plugin.Errorf(plugin.Failed, "the sign-in server %s gives no address for tokens: %v", issuer, err)
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	named, _ := resource["resource"].(string)
	form.Set("resource", named)
	scope := c.scope
	if scope == "" {
		scope = said["scope"]
	}
	if supported, _ := resource["scopes_supported"].([]any); scope == "" && len(supported) > 0 {
		var all []string
		for _, s := range supported {
			if text, ok := s.(string); ok {
				all = append(all, text)
			}
		}
		scope = strings.Join(all, " ")
	}
	if scope != "" {
		form.Set("scope", scope)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", to.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header = http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"application/json"}}
	if c.secret != nil {
		over, encrypted := connection(to)
		secret, err := w.job.RevealOver(c.secret, over, encrypted)
		if err != nil {
			return "", err
		}
		methods, _ := meta["token_endpoint_auth_methods_supported"].([]any)
		if slices.Contains(methods, any("client_secret_post")) && !slices.Contains(methods, any("client_secret_basic")) {
			form.Set("client_id", c.id)
			form.Set("client_secret", secret)
		} else {
			basic := url.QueryEscape(c.id) + ":" + url.QueryEscape(secret)
			req.Header["Authorization"] = []string{"Basic " + base64.StdEncoding.EncodeToString([]byte(basic))}
		}
	} else {
		key, err := w.job.Key(c.key)
		if err != nil {
			return "", err
		}
		assertion, err := signed(key, c.id, to.String())
		if err != nil {
			return "", err
		}
		form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		form.Set("client_assertion", assertion)
	}
	body := form.Encode()
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
	status, _, got, err := w.do(req)
	if err != nil {
		return "", err
	}
	defer got.Close()
	var answer map[string]any
	whole, _ := io.ReadAll(io.LimitReader(got, 1<<20))
	json.Unmarshal(whole, &answer)
	if status != 200 {
		description, _ := answer["error_description"].(string)
		if description == "" {
			description, _ = answer["error"].(string)
		}
		return "", plugin.Errorf(plugin.Refused, "the sign-in server %s refused the client %q (%d): %s", to.Host, c.id, status, description)
	}
	token, _ := answer["access_token"].(string)
	if token == "" {
		return "", plugin.Errorf(plugin.Failed, "the sign-in server %s gave no token", to.Host)
	}
	return token, nil
}

// resource reads the server's metadata: from where its challenge says, or
// else from where it is kept for the server's path and then for its host.
// The metadata must be of this server.
func (c *client) resource(ctx context.Context, w *web, at string) (map[string]any, error) {
	places := []string{at}
	if at == "" {
		base := w.to.Scheme + "://" + w.to.Host + "/.well-known/oauth-protected-resource"
		places = []string{base}
		if path := strings.TrimSuffix(w.to.EscapedPath(), "/"); path != "" {
			places = []string{base + path, base}
		}
	}
	for _, place := range places {
		meta, err := c.fetch(ctx, w, place)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			continue
		}
		resource, _ := meta["resource"].(string)
		if !covers(resource, w.to) {
			return nil, plugin.Errorf(plugin.Failed, "the metadata at %s is of %q, which is not the server %s", place, resource, w.to)
		}
		return meta, nil
	}
	return nil, plugin.Errorf(plugin.Failed, "the server %s wants a sign-in and says nowhere how", w.to.Host)
}

// covers says whether a resource's name is the server's address or what
// begins it: its host, or a path above it.
func covers(resource string, server *url.URL) bool {
	named, err := url.Parse(resource)
	if err != nil || named.Fragment != "" || !strings.EqualFold(named.Scheme, server.Scheme) || !strings.EqualFold(named.Host, server.Host) {
		return false
	}
	path := strings.TrimSuffix(named.EscapedPath(), "/")
	at := strings.TrimSuffix(server.EscapedPath(), "/")
	return path == "" || path == at || strings.HasPrefix(at, path+"/")
}

// issuer reads a sign-in server's metadata, from each place it may be
// kept, in turn. The metadata must name the issuer it was looked for by.
func (c *client) issuer(ctx context.Context, w *web, issuer string) (map[string]any, error) {
	named, err := address(issuer)
	if err != nil {
		return nil, plugin.Errorf(plugin.Failed, "the server %s names the sign-in server %q, which is not an address", w.to.Host, issuer)
	}
	base := named.Scheme + "://" + named.Host
	path := strings.TrimSuffix(named.EscapedPath(), "/")
	places := []string{base + "/.well-known/oauth-authorization-server", base + "/.well-known/openid-configuration"}
	if path != "" {
		places = []string{base + "/.well-known/oauth-authorization-server" + path, base + "/.well-known/openid-configuration" + path,
			base + path + "/.well-known/openid-configuration"}
	}
	for _, place := range places {
		meta, err := c.fetch(ctx, w, place)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			continue
		}
		if meta["issuer"] != issuer {
			return nil, plugin.Errorf(plugin.Failed, "the metadata at %s is of the issuer %v, and not %s", place, meta["issuer"], issuer)
		}
		return meta, nil
	}
	return nil, plugin.Errorf(plugin.Failed, "the sign-in server %s has no metadata", issuer)
}

// fetch reads a document of metadata, or nothing where there is none.
func (c *client) fetch(ctx context.Context, w *web, place string) (map[string]any, error) {
	to, err := address(place)
	if err != nil {
		return nil, plugin.Errorf(plugin.Failed, "the server %s gives %q for its metadata, which is not an address", w.to.Host, place)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", to.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{"Accept": {"application/json"}}
	status, _, body, err := w.do(req)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	if status != 200 {
		return nil, nil
	}
	whole, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		return nil, err
	}
	var meta map[string]any
	if err := json.Unmarshal(whole, &meta); err != nil {
		return nil, plugin.Errorf(plugin.Failed, "the metadata at %s is not JSON: %v", place, err)
	}
	return meta, nil
}

// challenge reads the parameters of a WWW-Authenticate header.
func challenge(header string) map[string]string {
	out := map[string]string{}
	rest := header
	if scheme, after, found := strings.Cut(strings.TrimSpace(rest), " "); found && !strings.Contains(scheme, "=") {
		rest = after
	}
	for rest != "" {
		rest = strings.TrimLeft(rest, " ,\t")
		name, after, found := strings.Cut(rest, "=")
		if !found {
			break
		}
		name = strings.ToLower(strings.TrimSpace(name))
		value := ""
		if strings.HasPrefix(after, `"`) {
			end := strings.Index(after[1:], `"`)
			if end < 0 {
				break
			}
			value, rest = after[1:end+1], after[end+2:]
		} else {
			value, rest, _ = strings.Cut(after, ",")
			value = strings.TrimSpace(value)
		}
		out[name] = value
	}
	return out
}

// signed is the assertion a client signs with its key: a JWT that names the
// client and the address it is sent to, good for five minutes.
func signed(pemText []byte, id, audience string) (string, error) {
	block, _ := pem.Decode(pemText)
	if block == nil {
		return "", plugin.Errorf(plugin.BadInput, "the client's key is not a key written as PEM")
	}
	var key any
	var err error
	if key, err = x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		if key, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
			if key, err = x509.ParseECPrivateKey(block.Bytes); err != nil {
				return "", plugin.Errorf(plugin.BadInput, "the client's key cannot be read: %v", err)
			}
		}
	}
	alg := ""
	switch k := key.(type) {
	case *rsa.PrivateKey:
		alg = "RS256"
	case *ecdsa.PrivateKey:
		if k.Curve.Params().Name != "P-256" {
			return "", plugin.Errorf(plugin.BadInput, "the client's key is on the curve %s, and a key for ES256 is on P-256", k.Curve.Params().Name)
		}
		alg = "ES256"
	case ed25519.PrivateKey:
		alg = "EdDSA"
	default:
		return "", plugin.Errorf(plugin.BadInput, "the client's key is a %T, and one signs with RSA, P-256 or Ed25519", key)
	}
	jti := make([]byte, 16)
	rand.Read(jti)
	now := time.Now()
	header, _ := json.Marshal(map[string]any{"alg": alg, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": id, "sub": id, "aud": audience, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		"jti": hex.EncodeToString(jti)})
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(input))
	var signature []byte
	switch k := key.(type) {
	case *rsa.PrivateKey:
		signature, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, sum[:])
	case *ecdsa.PrivateKey:
		r, s, failed := ecdsa.Sign(rand.Reader, k, sum[:])
		signature, err = append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...), failed
	case ed25519.PrivateKey:
		signature = ed25519.Sign(k, []byte(input))
	}
	if err != nil {
		return "", err
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
