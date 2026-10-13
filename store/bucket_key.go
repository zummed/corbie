package store

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// A bucketKey is one party's key for a bucket. It is never printed, and is
// never part of an address or an error.
type bucketKey struct {
	id, secret string
	token      string // set where the key is one lent for a while
}

// bucketKeyHere gives this party's key: from the environment, and failing
// that from a profile in ~/.aws/credentials.
func bucketKeyHere() (bucketKey, error) {
	key := bucketKey{os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_SESSION_TOKEN")}
	if key.id != "" && key.secret != "" {
		return key, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return bucketKey{}, err
	}
	file, err := os.ReadFile(filepath.Join(home, ".aws", "credentials"))
	if err != nil {
		return bucketKey{}, fmt.Errorf("store: no key for the bucket: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are not set, and %w", err)
	}
	profile := cmp.Or(os.Getenv("AWS_PROFILE"), "default")
	key, in := bucketKey{}, false
	for line := range strings.Lines(string(file)) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = line == "["+profile+"]"
			continue
		}
		name, value, _ := strings.Cut(line, "=")
		if !in {
			continue
		}
		switch strings.TrimSpace(name) {
		case "aws_access_key_id":
			key.id = strings.TrimSpace(value)
		case "aws_secret_access_key":
			key.secret = strings.TrimSpace(value)
		case "aws_session_token":
			key.token = strings.TrimSpace(value)
		}
	}
	if key.id == "" || key.secret == "" {
		return bucketKey{}, errors.New("store: no key for the bucket: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are not set, and ~/.aws/credentials has none under the profile in use")
	}
	return key, nil
}

// sign adds to a request the proof that it comes from the holder of the
// key: Signature Version 4 over the method, the path, the query, every
// header set so far and the hash of the body.
func (k bucketKey) sign(req *http.Request, region string, now time.Time) {
	date := now.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", date)
	if k.token != "" {
		req.Header.Set("X-Amz-Security-Token", k.token)
	}
	names := []string{"host"}
	for name := range req.Header {
		names = append(names, strings.ToLower(name))
	}
	slices.Sort(names)
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s/%s/s3/aws4_request, SignedHeaders=%s, Signature=%s",
		k.id, date[:8], region, strings.Join(names, ";"), signature(k.secret, region, req, names)))
}

// signature is the signature of a request over the headers named, as the
// one who sends it and the one who receives it both work it out. The path
// and the query are taken as they go over the wire.
func signature(secret, region string, req *http.Request, names []string) string {
	date := req.Header.Get("X-Amz-Date")
	day := date[:min(8, len(date))]
	host := cmp.Or(req.Host, req.URL.Host)
	var headers strings.Builder
	for _, name := range names {
		value := host
		if name != "host" {
			value = strings.Join(req.Header.Values(name), ",")
		}
		headers.WriteString(name + ":" + strings.TrimSpace(value) + "\n")
	}
	asked := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		headers.String(),
		strings.Join(names, ";"),
		req.Header.Get("X-Amz-Content-Sha256"),
	}, "\n")
	sum := sha256.Sum256([]byte(asked))
	scope := day + "/" + region + "/s3/aws4_request"
	key := []byte("AWS4" + secret)
	for _, part := range []string{day, region, "s3", "aws4_request",
		"AWS4-HMAC-SHA256\n" + date + "\n" + scope + "\n" + hex.EncodeToString(sum[:])} {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(part))
		key = mac.Sum(nil)
	}
	return hex.EncodeToString(key)
}
