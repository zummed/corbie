package serve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
)

// newToken makes a token: 16 random bytes, written as hex.
func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// readToken reads the token a file holds.
func readToken(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 16 {
		return "", errShortToken
	}
	return token, nil
}

type tokenError string

func (e tokenError) Error() string { return string(e) }

const errShortToken = tokenError("the token file holds fewer than 16 characters: a token that short is guessed")

// guard lets through a request that shows the token, and refuses every
// other, having done nothing. A browser shows it once, in the address it
// was given, and is sent back to that address without it, holding a cookie
// in its place. A request that a page of another origin makes is refused:
// a page on another port of this computer is the same site to a browser.
func guard(token, listen string, next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(listen)
	name := "corbie-ui-" + port
	same := func(s string) bool { return subtle.ConstantTimeCompare([]byte(s), []byte(token)) == 1 }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "a request from a page of another origin is refused", http.StatusForbidden)
			return
		}
		if given := r.URL.Query().Get("token"); given != "" && r.Method == http.MethodGet {
			if !same(given) {
				http.Error(w, "the token is wrong: open the address corbie ui printed", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			q := r.URL.Query()
			q.Del("token")
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && same(bearer) {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(name); err == nil && same(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "the token is missing: open the address corbie ui printed", http.StatusUnauthorized)
	})
}
