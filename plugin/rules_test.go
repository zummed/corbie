package plugin_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	_ "github.com/zummed/corbie/plugin/bundled"
)

func newJob(t *testing.T, secrets map[string]string, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), Secrets: secrets, SearchPath: "/usr/bin:/bin",
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}}}
	t.Cleanup(job.End)
	return job
}

func call(job *plugin.Job, module, name string, first any, opt plugin.Options) (any, error) {
	m, _ := plugin.Find(module)
	return m.Calls[name].Invoke(job, module, name, first, opt)
}

func codeOf(err error) string {
	var known *plugin.Error
	if errors.As(err, &known) {
		return known.Code
	}
	return ""
}

func refusedAs(err error) string {
	var refusal *permission.Refusal
	if errors.As(err, &refusal) {
		return refusal.What
	}
	return ""
}

// listening is a place on this machine that takes connections and keeps
// everything it is sent. It says one line in the clear to whoever
// connects, and no more.
type listening struct {
	address string
	mu      sync.Mutex
	opened  int
	got     []byte
}

func (l *listening) seen() (opened int, got string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.opened, string(l.got)
}

func listen(t *testing.T) *listening {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	here := &listening{address: l.Addr().String()}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			here.mu.Lock()
			here.opened++
			here.mu.Unlock()
			go func() {
				defer conn.Close()
				conn.Write([]byte("this place does not encrypt\r\n"))
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					here.mu.Lock()
					here.got = append(here.got, buf[:n]...)
					here.mu.Unlock()
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	return here
}

func Test_sec_plugins_word_on_encryption(t *testing.T) {
	const value = "tok-en/123456789"
	bound := newJob(t, map[string]string{"K": value}, "network", "secret:K@net:api.example")
	anywhere := newJob(t, map[string]string{"K": value}, "network", "secret:K@*")
	unencrypted := newJob(t, map[string]string{"K": value}, "network", "secret:K@net.unencrypted:api.example")
	handle := plugin.NewSecret("K")
	// With the plugin's word that the connection is encrypted, a secret
	// bound to the host goes into it.
	if got, err := bound.RevealOver(handle, "api.example:443", true); err != nil || got != value {
		t.Fatalf("with the plugin's word the secret gave %q, %v", got, err)
	}
	// Without the word it does not: not when the plugin says the connection
	// is plain, and not when the plugin says nothing about it at all.
	said, err := bound.RevealOver(handle, "api.example:443", false)
	nothing, silent := bound.Reveal(handle, "net:api.example:443")
	for what, err := range map[string]error{"said to be plain": err, "with nothing said": silent} {
		var refusal *permission.Refusal
		if !errors.As(err, &refusal) || refusal.What != "secret:K@net.unencrypted:api.example:443" || !strings.Contains(refusal.Fix, "add the line secret:K@net.unencrypted:api.example:443 to test") ||
			!strings.Contains(refusal.Fix, "the connection to api.example:443 is not encrypted") {
			t.Errorf("over a connection %s, a secret bound to its host gave %v", what, err)
		}
	}
	if said != "" || nothing != "" {
		t.Fatalf("a refused secret was given: %q, %q", said, nothing)
	}
	// A secret whose entry says the connection need not be encrypted goes
	// to that host without the word, and with it, and to no other host.
	for _, encrypted := range []bool{false, true} {
		if got, err := unencrypted.RevealOver(handle, "api.example:80", encrypted); err != nil || got != value {
			t.Fatalf("a secret bound to its host over any connection gave %q, %v (encrypted: %v)", got, err, encrypted)
		}
	}
	if got, err := unencrypted.RevealOver(handle, "other.example:80", false); refusedAs(err) != "secret:K@net.unencrypted:other.example:80" || got != "" {
		t.Fatalf("a secret bound to one host over any connection went to another: %q, %v", got, err)
	}
	// A layer that denies the secret to the host denies it there over a
	// plain connection too.
	denied := newJob(t, map[string]string{"K": value}, "network", "secret:K@*")
	denied.Perms.Layers[0].Deny = []string{"secret:K@net:api.example"}
	if got, err := denied.RevealOver(handle, "api.example:80", false); refusedAs(err) == "" || got != "" {
		t.Fatalf("a secret denied to a host went to it over a plain connection: %q, %v", got, err)
	}
	// So does a secret bound to anywhere.
	if got, err := anywhere.RevealOver(handle, "api.example:80", false); err != nil || got != value {
		t.Fatalf("a secret bound to anywhere, over a plain connection, gave %q, %v", got, err)
	}
	if got, err := anywhere.Reveal(handle, "net:api.example:80"); err != nil || got != value {
		t.Fatalf("a secret bound to anywhere, with nothing said, gave %q, %v", got, err)
	}

	// A plugin that gives its word leaves no way to fall back to a plain
	// connection: asked for an encrypted one to a server that speaks only
	// in the clear, each fails, and the secret is not sent.
	plain := listen(t)
	job := newJob(t, map[string]string{"K": value}, "network", "secret:K@net:127.0.0.1")
	headers := map[string]any{"Authorization": handle}
	short := plugin.Options{"headers": headers, "timeout": 0.3}
	if _, err := call(job, "http", "get", "https://"+plain.address+"/", short); err == nil {
		t.Error("http reached a server that does not encrypt, at an https address")
	}
	if _, err := call(job, "websocket", "connect", "wss://"+plain.address+"/", short); err == nil {
		t.Error("websocket reached a server that does not encrypt, at a wss address")
	}
	if _, err := call(job, "mcp", "connect", "https://"+plain.address+"/", short); err == nil {
		t.Error("mcp reached a server that does not encrypt, at an https address")
	}
	if _, err := call(job, "ssh", "run", plain.address, plugin.Options{"user": "u", "command": "true", "password": handle, "hostKey": "unchecked", "timeout": 0.3}); err == nil {
		t.Error("ssh signed in to a server that does not encrypt")
	}
	// And where the address asks for a plain connection, they give no word.
	for module, address := range map[string]string{"http": "http://", "websocket": "ws://", "mcp": "http://"} {
		name := map[string]string{"http": "get", "websocket": "connect", "mcp": "connect"}[module]
		if _, err := call(job, module, name, address+plain.address+"/", short); refusedAs(err) != "secret:K@net.unencrypted:"+plain.address {
			t.Errorf("%s sent a secret bound to a host to a plain address: %v", module, err)
		}
	}
	if _, got := plain.seen(); strings.Contains(got, value) || strings.Contains(got, "Authorization") {
		t.Fatal("the server that does not encrypt was sent the secret")
	}
}

func Test_sec_all_checked_before_any_taken(t *testing.T) {
	const a, b = "aaaa-value-of-a", "bbbb-value-of-b"
	server := listen(t)
	host, _, _ := net.SplitHostPort(server.address)
	dir := t.TempDir()
	file, marker := filepath.Join(dir, "key"), filepath.Join(dir, "started")
	// A may go to each place and B to none of them.
	job := newJob(t, map[string]string{"A": a, "B": b}, "network", "disk", "exec",
		"secret:A@net:"+host, "secret:A@fs.write:"+filepath.ToSlash(dir)+"/**", "secret:A@exec:sh", "secret:B@net:other.example")
	handleA, handleB := plugin.NewSecret("A"), plugin.NewSecret("B")
	both, _ := plugin.Join(handleA, ":", handleB)
	touch := []any{"-c", "touch " + marker}
	calls := map[string]func() (any, error){
		"a web request with one in each of two headers": func() (any, error) {
			return call(job, "http", "get", "https://"+server.address+"/", plugin.Options{"headers": map[string]any{"X-A": handleA, "X-B": handleB}})
		},
		"a web request with one in a header and one in the body": func() (any, error) {
			return call(job, "http", "post", "https://"+server.address+"/", plugin.Options{"headers": map[string]any{"X-A": handleA}, "body": handleB})
		},
		"a web request with both joined in the address": func() (any, error) {
			address, _ := plugin.Join("https://"+server.address+"/?k=", both)
			return call(job, "http", "get", address, nil)
		},
		"a connection opened with both in a header": func() (any, error) {
			return call(job, "websocket", "connect", "wss://"+server.address+"/", plugin.Options{"headers": map[string]any{"Authorization": both}})
		},
		"an MCP server connected to with one in each of two headers": func() (any, error) {
			return call(job, "mcp", "connect", "https://"+server.address+"/", plugin.Options{"headers": map[string]any{"X-A": handleA, "X-B": handleB}})
		},
		"an MCP server started with one as an argument and one as a variable": func() (any, error) {
			return call(job, "mcp", "start", "sh", plugin.Options{"args": append(touch, handleA), "env": map[string]any{"B": handleB}})
		},
		"a sign-in on another machine with both joined": func() (any, error) {
			return call(job, "ssh", "run", server.address, plugin.Options{"user": "u", "command": "true", "password": both, "hostKey": "unchecked"})
		},
		"a file written from both joined": func() (any, error) {
			return call(job, "fs", "write", file, plugin.Options{"secret": both})
		},
		"a program given one as an argument and one as a variable": func() (any, error) {
			return call(job, "exec", "run", "sh", plugin.Options{"args": append(touch, handleA), "env": map[string]any{"B": handleB}})
		},
		"a program given both joined as its input": func() (any, error) {
			return call(job, "exec", "run", "sh", plugin.Options{"args": touch, "input": both})
		},
	}
	for what, do := range calls {
		if _, err := do(); !strings.HasPrefix(refusedAs(err), "secret:B@") {
			t.Errorf("%s gave %v", what, err)
		}
	}
	// Nothing was made for any of the calls: no connection, no file, no
	// started program.
	if opened, _ := server.seen(); opened != 0 {
		t.Errorf("%d connections were opened", opened)
	}
	for what, path := range map[string]string{"the file": file, "what the program would have made": marker} {
		if _, err := os.Lstat(path); err == nil {
			t.Errorf("%s is there", what)
		}
	}
}

func Test_sec_handle_never_the_data(t *testing.T) {
	const value = "tok-en/123456789"
	// Bound to anywhere, the secret could go to any place, and still it is
	// not something to be worked on and handed back.
	job := newJob(t, map[string]string{"K": value}, "all")
	handle := plugin.NewSecret("K")
	joined, _ := plugin.Join("text and ", handle)
	for _, data := range []any{handle, joined, map[string]any{"inside": handle}, []any{"a list", handle}} {
		for what, do := range map[string]func() (any, error){
			"hashed":             func() (any, error) { return call(job, "crypto", "hash", data, nil) },
			"given a keyed hash": func() (any, error) { return call(job, "crypto", "hmac", data, plugin.Options{"key": "k"}) },
			"written as base64":  func() (any, error) { return call(job, "crypto", "encode", data, nil) },
			"packed":             func() (any, error) { return call(job, "compress", "gzip", data, nil) },
			"unpacked":           func() (any, error) { return call(job, "compress", "gunzip", data, nil) },
			"packed as a file":   func() (any, error) { return call(job, "compress", "archive", map[string]any{"a.txt": data}, nil) },
			"made a blob":        func() (any, error) { return call(job, "blobs", "make", data, nil) },
			"read as a page":     func() (any, error) { return call(job, "html", "parse", data, nil) },
			"read as XML":        func() (any, error) { return call(job, "xml", "parse", data, nil) },
			"written as XML": func() (any, error) {
				return call(job, "xml", "write", map[string]any{"tag": "a", "text": data}, nil)
			},
			"read as YAML":    func() (any, error) { return call(job, "yaml", "parse", data, nil) },
			"written as YAML": func() (any, error) { return call(job, "yaml", "write", map[string]any{"a": data}, nil) },
			"written to a file as text": func() (any, error) {
				return call(job, "fs", "write", "out.txt", plugin.Options{"text": data})
			},
			"written to a file as data": func() (any, error) {
				return call(job, "fs", "write", "out.json", plugin.Options{"json": data})
			},
		} {
			if out, err := do(); codeOf(err) != plugin.BadInput {
				t.Errorf("a handle (%T) was %s: %v, %v", data, what, out, err)
			}
		}
	}
	if files, _ := os.ReadDir(job.Scratch); len(files) != 0 {
		t.Errorf("%d files were made", len(files))
	}
	// As a key it gives ordinary data, which is another thing: that is
	// allowed to a secret bound to anywhere.
	if out, err := call(job, "crypto", "hmac", "abc", plugin.Options{"key": handle}); err != nil || strings.Contains(out.(string), value) {
		t.Errorf("a keyed hash under the secret gave %v, %v", out, err)
	}
}

func Test_sec_handle_only_where_sent_as_written(t *testing.T) {
	const value = "tok-en/123456789"
	server := listen(t)
	job := newJob(t, map[string]string{"K": value}, "network", "secret:K@*")
	handle := plugin.NewSecret("K")
	joined, _ := plugin.Join("x-", handle)
	// The headers that say where a request goes and how it is framed, and
	// the ones that open a websocket connection, are written by the plugin
	// from other things. A handle there is refused, however the name is
	// cased, and its value is not taken.
	framing := []string{"Host", "host", "Content-Length", "CONTENT-LENGTH", "Transfer-Encoding", "Trailer", "Connection"}
	opening := []string{"Upgrade", "Sec-WebSocket-Key", "sec-websocket-key", "Sec-WebSocket-Version"}
	for _, name := range framing {
		for _, secret := range []*plugin.Secret{handle, joined} {
			if _, err := call(job, "http", "post", "http://"+server.address+"/", plugin.Options{"body": "b", "headers": map[string]any{name: secret}}); codeOf(err) != plugin.BadInput {
				t.Errorf("http took a handle as the header %s: %v", name, err)
			}
		}
	}
	for _, name := range append(framing, opening...) {
		if _, err := call(job, "websocket", "connect", "ws://"+server.address+"/", plugin.Options{"headers": map[string]any{name: handle}}); codeOf(err) != plugin.BadInput {
			t.Errorf("websocket took a handle as the header %s: %v", name, err)
		}
	}
	// An MCP server's own headers are the plugin's to write.
	for _, name := range append(framing, "Accept", "Content-Type", "MCP-Protocol-Version", "Mcp-Method", "Mcp-Name", "Mcp-Session-Id") {
		if _, err := call(job, "mcp", "connect", "http://"+server.address+"/", plugin.Options{"headers": map[string]any{name: handle}}); codeOf(err) != plugin.BadInput {
			t.Errorf("mcp took a handle as the header %s: %v", name, err)
		}
	}
	// The method is text and never a handle.
	if _, err := call(job, "http", "request", "http://"+server.address+"/", plugin.Options{"method": handle}); codeOf(err) != plugin.BadInput {
		t.Errorf("http took a handle as the method: %v", err)
	}
	if opened, _ := server.seen(); opened != 0 {
		t.Fatalf("%d connections were opened", opened)
	}
	if used := job.Perms.Used(); len(used) != 0 {
		t.Fatalf("a value was taken for a call that was refused: %v was used", used)
	}
	// In a header of the script's own a handle is in its place.
	headers, err := job.Headers(map[string]any{"X-Api-Key": handle, "authorization": joined}, server.address, false, opening...)
	if err != nil || headers["X-Api-Key"][0] != value || headers["authorization"][0] != "x-"+value {
		t.Fatalf("a script's own headers gave %v, %v", headers, err)
	}
}

func Test_sec_no_line_break_into_a_header(t *testing.T) {
	server := listen(t)
	values := map[string]string{
		"BREAK":  "tok-en\r\nX-Made-Up: 1",
		"LINE":   "tok-en\nX-Made-Up: 1",
		"RETURN": "tok-en\rX-Made-Up: 1",
		"ZERO":   "tok-en\x00more",
		"GOOD":   "tok-en/123456789",
	}
	job := newJob(t, values, "network", "secret:*@*")
	for name, value := range values {
		if name == "GOOD" {
			continue
		}
		bearer, _ := plugin.Join("Bearer ", plugin.NewSecret(name))
		// As a secret's value, as a secret joined into text, and as plain
		// text, beside a header that is as it should be.
		for what, header := range map[string]any{"a secret": plugin.NewSecret(name), "a secret in text": bearer, "text": value} {
			headers := map[string]any{"X-Good": plugin.NewSecret("GOOD"), "X-Bad": header}
			if _, err := call(job, "http", "get", "http://"+server.address+"/", plugin.Options{"headers": headers}); codeOf(err) != plugin.BadInput {
				t.Errorf("http, with %s holding %q: %v", what, value, err)
			}
			if _, err := call(job, "websocket", "connect", "ws://"+server.address+"/", plugin.Options{"headers": headers}); codeOf(err) != plugin.BadInput {
				t.Errorf("websocket, with %s holding %q: %v", what, value, err)
			}
			if _, err := call(job, "mcp", "connect", "http://"+server.address+"/", plugin.Options{"headers": headers}); codeOf(err) != plugin.BadInput {
				t.Errorf("mcp, with %s holding %q: %v", what, value, err)
			}
		}
	}
	if opened, got := server.seen(); opened != 0 {
		t.Fatalf("%d connections were opened, and sent %q", opened, got)
	}
	// And no error said the value.
	_, err := call(job, "http", "get", "http://"+server.address+"/", plugin.Options{"headers": map[string]any{"X-Bad": plugin.NewSecret("BREAK")}})
	if err == nil || strings.Contains(err.Error(), "tok-en") {
		t.Fatalf("the refusal was %v", err)
	}
}

// A model service's error is cut to length, and is searched for the key
// before it is cut: here the cut falls inside the key the service quotes.
func TestModelServiceErrorIsSearchedBeforeItIsCut(t *testing.T) {
	const key = "sk-key-1234567890abcdefghij"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, strings.Repeat("x", 390)+r.Header.Get("X-Api-Key")+" is not a key we know")
	}))
	defer server.Close()
	job := newJob(t, map[string]string{"K": key}, "network", "secret:K@*")
	_, err := call(job, "llm", "chat", []any{map[string]any{"role": "user", "text": "hello"}},
		plugin.Options{"model": "any", "address": server.URL, "key": plugin.NewSecret("K")})
	if codeOf(err) != plugin.Refused || !strings.Contains(err.Error(), "answered 401") || strings.Contains(err.Error(), "sk-key") {
		t.Fatalf("the service's error was handed on as: %v", err)
	}
}
