package role_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/role"
)

// corbie runs one command of the command line and gives its exit status
// and everything it said.
func corbie(ctx context.Context, args ...string) (int, string) {
	var out, said bytes.Buffer
	status := cli.Run(ctx, args, strings.NewReader(""), &out, &said)
	return status, out.String() + said.String()
}

// A bucket stands in for the bucket a serverless host's copies share: it
// holds values and answers the requests a store makes of one.
type bucket struct {
	mu     sync.Mutex
	values map[string][]byte
}

func tag(value []byte) string {
	sum := sha256.Sum256(value)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

func (b *bucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/bkt"), "/")
	old, there := b.values[key]
	refuse := func(status int, code string) {
		w.WriteHeader(status)
		fmt.Fprintf(w, "<Error><Code>%s</Code></Error>", code)
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		var keys []string
		for key := range b.values {
			if strings.HasPrefix(key, r.URL.Query().Get("prefix")) {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		fmt.Fprint(w, "<ListBucketResult><IsTruncated>false</IsTruncated>")
		for _, key := range keys {
			fmt.Fprintf(w, "<Contents><Key>%s</Key></Contents>", url.QueryEscape(key))
		}
		fmt.Fprint(w, "</ListBucketResult>")
	case r.Method == http.MethodGet && !there:
		refuse(http.StatusNotFound, "NoSuchKey")
	case r.Method == http.MethodGet:
		w.Header().Set("ETag", tag(old))
		var from, to int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &from, &to); err == nil {
			if from >= len(old) {
				refuse(http.StatusRequestedRangeNotSatisfiable, "InvalidRange")
				return
			}
			w.WriteHeader(http.StatusPartialContent)
			old = old[from:min(to+1, len(old))]
		}
		w.Write(old)
	case r.Header.Get("If-None-Match") == "*" && there, r.Header.Get("If-Match") != "" && there && r.Header.Get("If-Match") != tag(old):
		refuse(http.StatusPreconditionFailed, "PreconditionFailed")
	case r.Header.Get("If-Match") != "" && !there:
		refuse(http.StatusNotFound, "NoSuchKey")
	case r.Method == http.MethodPut:
		value, _ := io.ReadAll(r.Body)
		if from := r.Header.Get("X-Amz-Copy-Source"); from != "" {
			source, _ := url.PathUnescape(from)
			if value, there = b.values[strings.TrimPrefix(source, "/bkt/")]; !there {
				refuse(http.StatusNotFound, "NoSuchKey")
				return
			}
		}
		b.values[key] = value
		w.Header().Set("ETag", tag(value))
	case r.Method == http.MethodDelete:
		delete(b.values, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		refuse(http.StatusNotImplemented, "NotImplemented")
	}
}

// has says whether the bucket holds a value whose name has a part.
func (b *bucket) has(part string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range b.values {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

// calls stands in for the part of a serverless host a process asks for its
// calls: it hands out a number of calls, each good for half a minute, notes
// the answers, and when the last is answered has done ended.
func calls(t *testing.T, n int, done func(), answered func(body string)) *httptest.Server {
	var mu sync.Mutex
	asked, answers := 0, 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/invocation/next"):
			if asked++; asked > n {
				mu.Unlock()
				<-r.Context().Done() // no more calls
				return
			}
			w.Header().Set("Lambda-Runtime-Aws-Request-Id", "call-"+strconv.Itoa(asked))
			w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.FormatInt(time.Now().Add(30*time.Second).UnixMilli(), 10))
		case strings.HasSuffix(r.URL.Path, "/response"):
			body, _ := io.ReadAll(r.Body)
			answered(string(body))
			if answers++; answers == n {
				done()
			}
		}
		mu.Unlock()
	}))
	t.Cleanup(host.Close)
	return host
}

// setUp sets up one machine's config and a store with corbie init, and
// leaves a start waiting in the store.
func setUp(t *testing.T, address string) (config string) {
	t.Helper()
	config = filepath.Join(t.TempDir(), "config")
	if status, said := corbie(ctx, "init", "--config", config, "--store", address, "--name", "host"); status != 0 {
		t.Fatalf("corbie init: %s", said)
	}
	definition := filepath.Join(t.TempDir(), "waits.yaml")
	os.WriteFile(definition, []byte("statechart: waits\ninitial: waiting\nstates:\n  waiting: { ticket: word }\n"), 0o600)
	if status, said := corbie(ctx, "run", "submit", definition, "--config", config); status != 0 {
		t.Fatalf("corbie run submit: %s", said)
	}
	return config
}

func Test_role_serverless_host_refuses_directory_and_memory(t *testing.T) {
	directory := t.TempDir()
	config := setUp(t, directory)
	// The same command starts on a computer: it is stopped once it
	// has taken the start.
	loop, stop := context.WithCancel(ctx)
	go func() {
		for began := time.Now(); time.Since(began) < 10*time.Second; time.Sleep(time.Millisecond) {
			if heads, _ := filepath.Glob(filepath.Join(directory, "host-controller", "running", "*", "*")); len(heads) > 0 {
				break
			}
		}
		stop()
	}()
	if status, said := corbie(loop, "start", "controller", "--config", config, "--interval", "5ms"); status != 0 || !strings.Contains(said, "controller host-controller") {
		t.Fatalf("on a computer, corbie start with a directory ended %d: %s", status, said)
	}

	// On a serverless host it refuses, for a directory and for memory, and
	// asks the host for no call.
	asked := 0
	host := calls(t, 1, func() {}, func(string) { asked++ })
	t.Setenv("AWS_LAMBDA_RUNTIME_API", strings.TrimPrefix(host.URL, "http://"))
	for address, want := range map[string]string{directory: "a store that is a directory", "file://" + directory: "a store that is a directory", "memory:": "a store in memory"} {
		limited, cancel := context.WithTimeout(ctx, 10*time.Second)
		status, said := corbie(limited, "start", "controller", "--config", config, "--store", address)
		cancel()
		if status != 1 || !strings.Contains(said, want) || !strings.Contains(said, "serverless host") || !strings.Contains(said, "bucket") {
			t.Fatalf("on a serverless host, corbie start with the store %s ended %d: %s", address, status, said)
		}
	}
	// The runner's store is the one its controller's card came with.
	limited, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if status, said := corbie(limited, "start", "runner", "--config", config); status != 1 || !strings.Contains(said, "a store that is a directory") {
		t.Fatalf("on a serverless host, a runner whose controller's store is a directory ended %d: %s", status, said)
	}
	if asked != 0 {
		t.Fatalf("a role that refused its store took %d calls", asked)
	}
	if err := role.StoreFits("s3://bucket/path?region=eu-west-1"); err != nil {
		t.Fatalf("a bucket is refused: %v", err)
	}
}

func Test_role_host_package_readable_by_all_still_starts(t *testing.T) {
	held := &bucket{values: map[string][]byte{}}
	served := httptest.NewServer(held)
	defer served.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "the-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "the-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	// The config is made on its owner's machine, and a start is waiting.
	config := setUp(t, "s3://bkt?region=test-1&endpoint="+served.URL)
	// The host unpacks its package read-only and readable by everyone.
	modes := func(dir string) (all []string) {
		filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			info, _ := entry.Info()
			all = append(all, fmt.Sprintf("%s %o", strings.TrimPrefix(path, dir), info.Mode().Perm()))
			return err
		})
		return all
	}
	filepath.WalkDir(config, func(path string, entry fs.DirEntry, err error) error {
		if entry.IsDir() {
			return os.Chmod(path, 0o555)
		}
		return os.Chmod(path, 0o444)
	})
	t.Cleanup(func() {
		filepath.WalkDir(config, func(path string, _ fs.DirEntry, _ error) error { return os.Chmod(path, 0o700) })
	})
	unpacked := modes(config)
	// Used where it is, as on a computer, it is not accepted.
	limited, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if status, said := corbie(limited, "start", "controller", "--config", config); status != 1 || !strings.Contains(said, "can be read by others") {
		t.Fatalf("with an identity that others can read, corbie start ended %d: %s", status, said)
	}
	if held.has("/running/") {
		t.Fatal("a controller whose identity others can read took a start")
	}

	// On the host the role starts all the same: it takes the host's call
	// and the start that was waiting, with a copy of its config that is on
	// the copy's own disk and its owner's alone.
	disk := t.TempDir()
	t.Setenv("TMPDIR", disk)
	var answers, own []string
	serving, stop := context.WithCancel(ctx)
	host := calls(t, 1, stop, func(body string) {
		answers = append(answers, body)
		own = modes(disk)
	})
	t.Setenv("AWS_LAMBDA_RUNTIME_API", strings.TrimPrefix(host.URL, "http://"))
	defer time.AfterFunc(20*time.Second, stop).Stop()
	if status, said := corbie(serving, "start", "controller", "--config", config); status != 0 {
		t.Fatalf("on a serverless host, with its config readable by everyone, corbie start ended %d: %s", status, said)
	}
	if len(answers) != 1 || !strings.Contains(answers[0], `"controller":"done"`) || !held.has("/running/") {
		t.Fatalf("the controller answered the host's call with %v, and took the start: %v", answers, held.has("/running/"))
	}
	identities := 0
	for _, line := range own {
		name, mode, _ := strings.Cut(line, " ")
		if name != "" && mode != "700" && mode != "600" {
			t.Fatalf("on the copy's own disk, %s has the mode %s", name, mode)
		}
		if strings.HasSuffix(name, "/controller/identity") {
			identities++
		}
	}
	if identities != 1 {
		t.Fatalf("the copy's own disk held %v while it worked", own)
	}
	// The package is as the host unpacked it, and the copy is gone with
	// the role.
	if after := modes(config); !slices.Equal(after, unpacked) {
		t.Fatalf("the package was changed:\n%v\nwas\n%v", after, unpacked)
	}
	if left := modes(disk); len(left) != 1 {
		t.Fatalf("the role left on its disk: %v", left)
	}
}
