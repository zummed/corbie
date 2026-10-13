package http

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// The machine these tests run on trusts one certificate of the tests' own
// making, so that a test can tell the certificates a script gave from the
// machine's.
var machines tls.Certificate

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "corbie-http-test-")
	if err != nil {
		panic(err)
	}
	var text string
	machines, text = newCertificate("127.0.0.1")
	os.WriteFile(filepath.Join(dir, "roots.pem"), []byte(text), 0o600)
	os.Setenv("SSL_CERT_FILE", filepath.Join(dir, "roots.pem"))
	os.Setenv("SSL_CERT_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newCertificate makes a certificate for a name or an address, signed by
// itself, and gives it with its PEM text.
func newCertificate(name string) (tls.Certificate, string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name + " " + serial.String()},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	if ip := net.ParseIP(name); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{name}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// encrypted starts a server that shows a certificate.
func encrypted(t *testing.T, cert tls.Certificate, handler nethttp.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	// A handshake that is turned down is the point of some tests.
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func newJob(t *testing.T, secrets map[string]string, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), Secrets: secrets,
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}}}
	t.Cleanup(job.End)
	return job
}

func call(job *plugin.Job, name string, address any, opt plugin.Options) (map[string]any, error) {
	module, _ := plugin.Find("http")
	out, err := module.Calls[name].Invoke(job, "http", name, address, opt)
	answer, _ := out.(map[string]any)
	return answer, err
}

func codeOf(err error) string {
	var known *plugin.Error
	if errors.As(err, &known) {
		return known.Code
	}
	return ""
}

// wire is a server that keeps each request exactly as it arrived, line by
// line, and answers every one the same.
type wire struct {
	address string
	mu      sync.Mutex
	got     []string
}

func (w *wire) requests() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.got)
}

func onTheWire(t *testing.T) *wire {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	w := &wire{address: l.Addr().String()}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				head, length := "", 0
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					head += line
					if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(name, "Content-Length") {
						length, _ = strconv.Atoi(strings.TrimSpace(value))
					}
					if line == "\r\n" {
						break
					}
				}
				body := make([]byte, length)
				io.ReadFull(reader, body)
				w.mu.Lock()
				w.got = append(w.got, head+string(body))
				w.mu.Unlock()
				io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
			}()
		}
	}()
	return w
}

func Test_plug_connection_not_reused_across_destinations(t *testing.T) {
	var mu sync.Mutex
	var from []string
	// The server would gladly serve many requests on one connection.
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		mu.Lock()
		from = append(from, r.RemoteAddr)
		mu.Unlock()
		if r.URL.Path == "/first" {
			nethttp.Redirect(w, r, "/second", 302)
		}
	}))
	defer server.Close()
	job := newJob(t, nil, "network")
	if _, err := call(job, "get", server.URL+"/first", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := call(job, "get", server.URL+"/second", nil); err != nil {
		t.Fatal(err)
	}
	// Three requests, each the outcome of its own check: the one that was
	// redirected, the one it was redirected to, and the next call's.
	slices.Sort(from)
	if len(from) != 3 || len(slices.Compact(from)) != 3 {
		t.Fatalf("three requests arrived over these connections: %v", from)
	}
	if used := job.Perms.Used(); len(used) != 1 || used[0] != "net:"+strings.TrimPrefix(server.URL, "http://") {
		t.Fatalf("what was used is %v", used)
	}
}

func Test_plug_http_address_escaped_as_written(t *testing.T) {
	w := onTheWire(t)
	job := newJob(t, map[string]string{"KEY": "k é"}, "network", "secret:*@*")
	line := func(address any, opt plugin.Options) string {
		t.Helper()
		before := len(w.requests())
		if _, err := call(job, "get", address, opt); err != nil {
			t.Fatal(err)
		}
		got := w.requests()
		if len(got) != before+1 {
			t.Fatalf("%d requests arrived", len(got)-before)
		}
		first, _, _ := strings.Cut(got[before], "\r\n")
		return first
	}
	base := "http://" + w.address
	// A blank and a letter beyond ASCII are escaped, in the path and in the
	// query. Everything else goes as it was written: what was escaped
	// already, the marks a path and a query may hold, the order of the
	// query.
	if got, want := line(base+"/a b/é%2Fc~'()!*;=/x%20y?q=é è&sig=a+b%3D%2F&z=1&a=2&list[]=1|2", nil),
		"GET /a%20b/%C3%A9%2Fc~'()!*;=/x%20y?q=%C3%A9%20%C3%A8&sig=a+b%3D%2F&z=1&a=2&list[]=1|2 HTTP/1.1"; got != want {
		t.Errorf("the request went as\n%s\nand not as\n%s", got, want)
	}
	// The marks no path may hold are escaped in the path, and what was
	// escaped beside them stays escaped. In the query they go as written.
	if got, want := line(base+"/a%2Fb/\"x\"<y>\\^`{|}%20z?q=\"<y>\\^`{|}%2F", nil),
		"GET /a%2Fb/%22x%22%3Cy%3E%5C%5E%60%7B%7C%7D%20z?q=\"<y>\\^`{|}%2F HTTP/1.1"; got != want {
		t.Errorf("the request went as\n%s\nand not as\n%s", got, want)
	}
	// What the query option names is added after, and does not rewrite
	// what the address asks already.
	if got, want := line(base+"/p?z=%7E&a=x+y", plugin.Options{"query": map[string]any{"n": 2.0, "b": "1 é"}}),
		"GET /p?z=%7E&a=x+y&b=1+%C3%A9&n=2 HTTP/1.1"; got != want {
		t.Errorf("with a query option the request went as\n%s\nand not as\n%s", got, want)
	}
	// The same holds for an address a secret is part of.
	address, _ := plugin.Join(base+"/v1/ü?key=", plugin.NewSecret("KEY"), "&p=%2B")
	if got, want := line(address, nil), "GET /v1/%C3%BC?key=k%20%C3%A9&p=%2B HTTP/1.1"; got != want {
		t.Errorf("with a secret the request went as\n%s\nand not as\n%s", got, want)
	}
	// And for an address a server redirects to.
	var asked []string
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		asked = append(asked, r.RequestURI)
		if r.URL.Path == "/first" {
			w.Header().Set("Location", "/moved%2Fhere/\"é x\"?q=%2F")
			w.WriteHeader(302)
		}
	}))
	defer server.Close()
	if _, err := call(job, "get", server.URL+"/first", nil); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/first", "/moved%2Fhere/%22%C3%A9%20x%22?q=%2F"}; !slices.Equal(asked, want) {
		t.Errorf("redirected, the requests went as %q and not as %q", asked, want)
	}
}

func TestRequestTakesItsMethodFromEachCall(t *testing.T) {
	w := onTheWire(t)
	job := newJob(t, nil, "network")
	for _, method := range []string{"post", "GET", "OPTIONS"} {
		if _, err := call(job, "request", "http://"+w.address+"/", plugin.Options{"method": method}); err != nil {
			t.Fatal(err)
		}
	}
	var sent []string
	for _, request := range w.requests() {
		method, _, _ := strings.Cut(request, " ")
		sent = append(sent, method)
	}
	if want := []string{"POST", "GET", "OPTIONS"}; !slices.Equal(sent, want) {
		t.Fatalf("three requests, each naming its method, went as %v and not as %v", sent, want)
	}
	// With none named, a request fetches.
	if _, err := call(job, "request", "http://"+w.address+"/", nil); err != nil {
		t.Fatal(err)
	}
	if last := w.requests()[3]; !strings.HasPrefix(last, "GET ") {
		t.Fatalf("a request that names no method went as %q", last)
	}
}

func Test_plug_http_address_with_login_refused(t *testing.T) {
	asked := 0
	var server *httptest.Server
	server = httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		asked++
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", strings.Replace(server.URL, "http://", "http://later:hunter2@", 1)+"/")
			w.WriteHeader(302)
		}
	}))
	defer server.Close()
	job := newJob(t, map[string]string{"PASS": "s3cr3t-pass"}, "network", "secret:*@*")
	host := strings.TrimPrefix(server.URL, "http://")
	withSecret, _ := plugin.Join("http://someone:", plugin.NewSecret("PASS"), "@"+host+"/")
	for what, address := range map[string]any{
		"a user name and a password":  "http://someone:hunter2@" + host + "/path",
		"a user name":                 "http://someone@" + host + "/path",
		"a password that is a secret": withSecret,
	} {
		_, err := call(job, "get", address, nil)
		if codeOf(err) != plugin.BadInput {
			t.Errorf("an address with %s gave %v", what, err)
			continue
		}
		for _, part := range []string{"someone", "hunter2", "s3cr3t-pass", host} {
			if strings.Contains(err.Error(), part) {
				t.Errorf("the refusal of an address with %s repeats %q: %v", what, part, err)
			}
		}
	}
	if asked != 0 {
		t.Fatalf("%d requests were sent", asked)
	}
	// A server cannot bring a login in by redirecting to one.
	_, err := call(job, "get", server.URL+"/redirect", nil)
	if codeOf(err) != plugin.BadInput || strings.Contains(err.Error(), "hunter2") || asked != 1 {
		t.Fatalf("a redirect to an address with a login gave %v after %d requests", err, asked)
	}
	// And an error that comes later says the host and no more of the
	// address.
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	gone := closed.Addr().String()
	closed.Close()
	_, err = call(job, "get", "http://"+gone+"/private/path?token=in-the-address", nil)
	if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "in-the-address") {
		t.Fatalf("a request that failed gave %v", err)
	}
}

func Test_plug_http_byte_order_mark_dropped(t *testing.T) {
	const mark = "\xEF\xBB\xBF"
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		io.WriteString(w, mark+`{"a":1}`)
	}))
	defer server.Close()
	job := newJob(t, nil, "network")
	text, err := call(job, "get", server.URL, nil)
	if err != nil || text["body"] != `{"a":1}` {
		t.Fatalf("read as text the body is %q (%v)", text["body"], err)
	}
	data, err := call(job, "get", server.URL, plugin.Options{"as": "json"})
	if err != nil || data["body"].(map[string]any)["a"] != 1.0 {
		t.Fatalf("read as data the body is %v (%v)", data["body"], err)
	}
	// Bytes are handed over as they are.
	raw, err := call(job, "get", server.URL, plugin.Options{"as": "bytes"})
	if err != nil || string(raw["body"].(plugin.Bytes)) != mark+`{"a":1}` {
		t.Fatalf("read as bytes the body is %q (%v)", raw["body"], err)
	}
}

func Test_plug_http_default_time_limit(t *testing.T) {
	if wait != 10*time.Second {
		t.Fatalf("a request with no time of its own is given %s", wait)
	}
	release := make(chan struct{})
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.URL.Path {
		case "/at-once":
			io.WriteString(w, "there")
			return
		case "/headers-only":
			// The headers at once, and then a body that never ends.
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(200)
			io.WriteString(w, "a start")
			w.(nethttp.Flusher).Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	job := newJob(t, nil, "network")
	shortened := func(d time.Duration) {
		was := wait
		wait = d
		t.Cleanup(func() { wait = was })
	}
	// The tests do not wait ten seconds: the limit a call gets when it
	// gives none is made short, and it is that limit that ends the call.
	shortened(150 * time.Millisecond)
	for _, path := range []string{"/never", "/headers-only"} {
		_, err := call(job, "get", server.URL+path, nil)
		if codeOf(err) != plugin.TimedOut || !strings.Contains(err.Error(), "within 150ms") {
			t.Errorf("%s, with no time given, gave %v", path, err)
		}
	}
	// A blob takes a body as it arrives, and the limit covers that too.
	if _, err := call(job, "get", server.URL+"/headers-only", plugin.Options{"as": "blob"}); codeOf(err) != plugin.TimedOut {
		t.Errorf("a body that never ends, into a blob, gave %v", err)
	}
	// The script's own time takes the default's place, shorter or longer.
	_, err := call(job, "get", server.URL+"/never", plugin.Options{"timeout": 0.05})
	if codeOf(err) != plugin.TimedOut || !strings.Contains(err.Error(), "within 50ms") {
		t.Errorf("with 50ms given the call gave %v", err)
	}
	shortened(time.Nanosecond)
	if _, err := call(job, "get", server.URL+"/at-once", nil); codeOf(err) != plugin.TimedOut {
		t.Errorf("with no time left the call gave %v", err)
	}
	if out, err := call(job, "get", server.URL+"/at-once", plugin.Options{"timeout": "30s"}); err != nil || out["body"] != "there" {
		t.Errorf("with 30s given the call gave %v, %v", out, err)
	}
}

func Test_plug_http_given_certificates_every_hop(t *testing.T) {
	asked := map[string]int{}
	var mu sync.Mutex
	count := func(name string) {
		mu.Lock()
		asked[name]++
		mu.Unlock()
	}
	answers := func(name string) nethttp.HandlerFunc {
		return func(w nethttp.ResponseWriter, r *nethttp.Request) {
			count(name)
			io.WriteString(w, "this is "+name)
		}
	}
	certA, pemA := newCertificate("127.0.0.1")
	certB, pemB := newCertificate("127.0.0.1")
	certNamed, pemNamed := newCertificate("other.example")
	b := encrypted(t, certB, answers("b"))
	machine := encrypted(t, machines, answers("the machine's"))
	named := encrypted(t, certNamed, answers("named"))
	a := encrypted(t, certA, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		count("a")
		switch r.URL.Path {
		case "/to-b":
			nethttp.Redirect(w, r, b.URL, 302)
		case "/to-the-machines":
			nethttp.Redirect(w, r, machine.URL, 302)
		}
	})
	job := newJob(t, nil, "network")
	get := func(address string, trust any) (any, error) {
		opt := plugin.Options{}
		if trust != nil {
			opt["trust"] = trust
		}
		out, err := call(job, "get", address, opt)
		return out["body"], err
	}
	// The certificates given are trusted at every place the call goes.
	if body, err := get(a.URL+"/to-b", pemA+pemB); err != nil || body != "this is b" {
		t.Fatalf("with both certificates given, a call redirected from one to the other gave %v, %v", body, err)
	}
	// Only those are trusted. One the machine trusts is not, at the first
	// place nor at one redirected to.
	if runtime.GOOS == "linux" {
		if body, err := get(machine.URL, nil); err != nil || body != "this is the machine's" {
			t.Fatalf("the machine does not trust its own certificate in this test: %v, %v", body, err)
		}
		before := asked["the machine's"]
		if _, err := get(machine.URL, pemA); codeOf(err) != plugin.Refused {
			t.Errorf("a certificate the machine trusts and the script did not give was taken: %v", err)
		}
		if _, err := get(a.URL+"/to-the-machines", pemA); codeOf(err) != plugin.Refused {
			t.Errorf("a redirect to a certificate the machine trusts and the script did not give was followed: %v", err)
		}
		if asked["the machine's"] != before {
			t.Errorf("a request reached a server whose certificate the script did not give")
		}
	}
	before := asked["b"]
	if _, err := get(a.URL+"/to-b", pemA); codeOf(err) != plugin.Refused || asked["b"] != before {
		t.Errorf("a redirect to a certificate that was not given gave %v", err)
	}
	// The name on a certificate is still checked.
	if _, err := get(named.URL, pemNamed); codeOf(err) != plugin.Refused || asked["named"] != 0 {
		t.Errorf("a certificate for another name gave %v", err)
	}
	// A certificate that cannot be read is refused, beside a good one too,
	// and nothing is sent.
	before = asked["a"]
	half := pemB[:len(pemB)/2]
	notOne := "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"
	key := "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"
	for what, trust := range map[string]string{
		"a certificate cut short":     pemA + half,
		"one that is not well formed": pemA + notOne,
		"a key":                       pemA + key,
		"no certificate at all":       "trust me",
	} {
		if _, err := get(a.URL, trust); codeOf(err) != plugin.BadInput {
			t.Errorf("trust holding %s gave %v", what, err)
		}
	}
	if asked["a"] != before {
		t.Errorf("a request was sent with certificates that could not all be read")
	}
	// Text around the certificates, as a file of them often has, is no
	// fault.
	if _, err := get(a.URL, "# ours\n"+pemA+"\n# and theirs\n"+pemB+"\nend\n"); err != nil {
		t.Errorf("certificates with notes between them gave %v", err)
	}
}

func Test_plug_http_headers_sent_as_written(t *testing.T) {
	w := onTheWire(t)
	job := newJob(t, map[string]string{"TOKEN": "tok-en/123456789"}, "network", "secret:*@*")
	bearer, _ := plugin.Join("Bearer ", plugin.NewSecret("TOKEN"))
	_, err := call(job, "post", "http://"+w.address+"/", plugin.Options{
		"json": map[string]any{"a": 1.0},
		"headers": map[string]any{
			"x-all-lower":     "as written",
			"X-MiXeD-CaSe":    "Kept, As Is",
			"X-Number":        7.0,
			"authorization":   bearer,
			"user-agent":      "mine/1.0",
			"content-type":    "application/vnd.mine+json",
			"accept-encoding": "br",
			"X-Twice":         "one",
			"x-twice":         "two",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := w.requests()
	if len(got) != 1 {
		t.Fatalf("%d requests arrived", len(got))
	}
	head, _, _ := strings.Cut(got[0], "\r\n\r\n")
	lines := strings.Split(head, "\r\n")[1:]
	for _, want := range []string{
		"x-all-lower: as written", "X-MiXeD-CaSe: Kept, As Is", "X-Number: 7", "authorization: Bearer tok-en/123456789",
		"user-agent: mine/1.0", "content-type: application/vnd.mine+json", "accept-encoding: br", "X-Twice: one", "x-twice: two",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("the header %q did not arrive as written, among:\n%s", want, head)
		}
	}
	// Nothing is put beside a header the script wrote, and the only others
	// are the ones that say where the request goes and how it is framed.
	added := []string{}
	for _, line := range lines {
		name, _, _ := strings.Cut(line, ":")
		if !slices.Contains([]string{"x-all-lower", "X-MiXeD-CaSe", "X-Number", "authorization", "user-agent", "content-type", "accept-encoding", "X-Twice", "x-twice"}, name) {
			added = append(added, name)
		}
	}
	slices.Sort(added)
	if strings.Join(added, ",") != "Connection,Content-Length,Host" {
		t.Errorf("beside the script's own, these headers were sent: %v", added)
	}
	// With none of the script's, the request says whose it is and what its
	// body is.
	if _, err := call(job, "post", "http://"+w.address+"/", plugin.Options{"json": map[string]any{"a": 1.0}}); err != nil {
		t.Fatal(err)
	}
	plain := w.requests()[1]
	if !strings.Contains(plain, "\r\nUser-Agent: corbie\r\n") || !strings.Contains(plain, "\r\nContent-Type: application/json\r\n") {
		t.Errorf("with no headers given the request was:\n%s", plain)
	}
	// A header that cannot be sent as written is refused aloud, not
	// dropped or rewritten out of sight, and nothing is sent.
	for _, name := range []string{"Host", "host", "Content-Length", "content-length", "Transfer-Encoding", "Trailer", "Connection", "bad name", "bad:name", ""} {
		_, err := call(job, "get", "http://"+w.address+"/", plugin.Options{"headers": map[string]any{name: "x"}})
		if codeOf(err) != plugin.BadInput {
			t.Errorf("the header %q gave %v", name, err)
		}
	}
	if got := w.requests(); len(got) != 2 {
		t.Fatalf("%d requests arrived", len(got))
	}
}

func Test_plug_http_no_body_on_fetch(t *testing.T) {
	asked := 0
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// The body is taken before the answer is given: a server that
		// answers and closes while a body is still being written leaves
		// the sender with a failed write, sometimes, in place of an answer.
		io.Copy(io.Discard, r.Body)
		asked++
	}))
	defer server.Close()
	job := newJob(t, map[string]string{"TOKEN": "tok-en/123456789"}, "network", "secret:*@*")
	bodies := map[string]any{
		"body":   "text",
		"json":   map[string]any{"a": 1.0},
		"form":   map[string]any{"a": "b"},
		"upload": map[string]any{"a": "b"},
	}
	for option, body := range bodies {
		for _, fetch := range []string{"get", "head"} {
			if _, err := call(job, fetch, server.URL, plugin.Options{option: body}); codeOf(err) != plugin.BadInput {
				t.Errorf("%s with %s gave %v", fetch, option, err)
			}
			// However the method is written.
			if _, err := call(job, "request", server.URL, plugin.Options{"method": fetch, option: body}); codeOf(err) != plugin.BadInput {
				t.Errorf("a request with method %s and %s gave %v", fetch, option, err)
			}
		}
	}
	for what, body := range map[string]any{"bytes": plugin.Bytes("x"), "a blob": plugin.NewBlob("text/plain", []byte("x")), "a handle": plugin.NewSecret("TOKEN")} {
		if _, err := call(job, "get", server.URL, plugin.Options{"body": body}); codeOf(err) != plugin.BadInput {
			t.Errorf("get with a body of %s gave %v", what, err)
		}
	}
	if asked != 0 {
		t.Fatalf("%d requests were sent", asked)
	}
	// A request that sends carries one.
	for option, body := range bodies {
		if _, err := call(job, "post", server.URL, plugin.Options{option: body}); err != nil {
			t.Errorf("post with %s gave %v", option, err)
		}
	}
	if _, err := call(job, "get", server.URL, nil); err != nil || asked != len(bodies)+1 {
		t.Fatalf("a get with no body gave %v, after %d requests", err, asked)
	}
}

func Test_plug_http_secret_not_carried_to_another_host(t *testing.T) {
	const value = "tok-en/123456789"
	cert, trust := newCertificate("127.0.0.1")
	var mu sync.Mutex
	var reached []string
	second := encrypted(t, cert, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		reached = append(reached, r.Header.Get("Authorization")+" "+string(body))
		mu.Unlock()
	})
	first := encrypted(t, cert, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Header.Get("Authorization") != "Bearer "+value {
			t.Errorf("the first host was sent %q", r.Header.Get("Authorization"))
		}
		// The method and the body are kept by this kind of redirect.
		nethttp.Redirect(w, r, second.URL+"/next", 307)
	})
	here, there := strings.TrimPrefix(first.URL, "https://"), strings.TrimPrefix(second.URL, "https://")
	bearer, _ := plugin.Join("Bearer ", plugin.NewSecret("TOKEN"))
	post := func(job *plugin.Job, opt plugin.Options) error {
		opt["trust"] = trust
		_, err := call(job, "post", first.URL, opt)
		return err
	}
	// Bound to the first host alone, the secret is not taken for the place
	// the server redirects to, and nothing goes there: not in a header, and
	// not in a body.
	for what, opt := range map[string]plugin.Options{
		"a header": {"headers": map[string]any{"Authorization": bearer}},
		"a body":   {"headers": map[string]any{"Authorization": "Bearer " + value}, "body": plugin.NewSecret("TOKEN")},
	} {
		job := newJob(t, map[string]string{"TOKEN": value}, "network", "secret:TOKEN@net:"+here)
		err := post(job, opt)
		var refusal *permission.Refusal
		if !errors.As(err, &refusal) || refusal.What != "secret:TOKEN@net:"+there {
			t.Fatalf("a secret in %s, redirected to a host it is not bound to, gave %v", what, err)
		}
	}
	if len(reached) != 0 {
		t.Fatalf("the second host was sent %q", reached)
	}
	// Bound to both, it is taken afresh for the second and arrives there.
	job := newJob(t, map[string]string{"TOKEN": value}, "network", "secret:TOKEN@net:"+here, "secret:TOKEN@net:"+there)
	if err := post(job, plugin.Options{"headers": map[string]any{"Authorization": bearer}, "body": plugin.NewSecret("TOKEN")}); err != nil {
		t.Fatal(err)
	}
	if len(reached) != 1 || reached[0] != "Bearer "+value+" "+value {
		t.Fatalf("the second host was sent %q", reached)
	}
}

func Test_sec_escaped_by_what_wrote_the_text(t *testing.T) {
	const value = `quo"te and back\slash & more`
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
	}))
	defer server.Close()
	job := newJob(t, map[string]string{"KEY": value}, "network", "secret:*@*")
	sent := func(opt plugin.Options) string {
		t.Helper()
		if _, err := call(job, "post", server.URL, opt); err != nil {
			t.Fatal(err)
		}
		return bodies[len(bodies)-1]
	}
	asJSON := map[string]any{"Content-Type": "application/json"}
	// The script wrote the text around the secret, so the script says how
	// the secret is escaped into it. Only the secret is: the text after it
	// arrives as written.
	inJSON, _ := plugin.NewSecret("KEY").Encoded("json")
	body, _ := plugin.Join(`{"key":"`, inJSON, `","note":"a \"quoted\" word"}`)
	var got map[string]string
	if err := json.Unmarshal([]byte(sent(plugin.Options{"body": body, "headers": asJSON})), &got); err != nil ||
		got["key"] != value || got["note"] != `a "quoted" word` {
		t.Errorf("a secret told to be escaped for JSON arrived as %q (%v)", bodies[len(bodies)-1], err)
	}
	inForm, _ := plugin.NewSecret("KEY").Encoded("url")
	body, _ = plugin.Join("key=", inForm, "&note=a+b%26c")
	if got := sent(plugin.Options{"body": body}); got != "key="+url.QueryEscape(value)+"&note=a+b%26c" {
		t.Errorf("a secret told to be escaped for a form arrived as %q", got)
	}
	// Told nothing, the secret goes as it is, whatever type the request
	// states: nothing reads the type to guess.
	body, _ = plugin.Join(`{"key":"`, plugin.NewSecret("KEY"), `"}`)
	for _, typ := range []string{"application/json", "application/x-www-form-urlencoded", "text/xml"} {
		if got := sent(plugin.Options{"body": body, "headers": map[string]any{"Content-Type": typ}}); got != `{"key":"`+value+`"}` {
			t.Errorf("stated to be %s, a body with a secret arrived as %q", typ, got)
		}
	}
	// Where the plugin writes the text, the plugin escapes what goes into
	// it: a form's fields, and text sent as JSON.
	form, _ := url.ParseQuery(sent(plugin.Options{"form": map[string]any{"key": plugin.NewSecret("KEY"), "note": "a&b=c"}}))
	if form.Get("key") != value || form.Get("note") != "a&b=c" {
		t.Errorf("a form arrived as %q", bodies[len(bodies)-1])
	}
	var text string
	if err := json.Unmarshal([]byte(sent(plugin.Options{"json": `say "hi" \ bye`})), &text); err != nil || text != `say "hi" \ bye` {
		t.Errorf("text sent as JSON arrived as %q (%v)", bodies[len(bodies)-1], err)
	}
	// It does not write a secret into JSON, and sends nothing in its place.
	before := len(bodies)
	for what, data := range map[string]any{"a handle": plugin.NewSecret("KEY"), "data holding a handle": map[string]any{"deep": []any{plugin.NewSecret("KEY")}}} {
		if _, err := call(job, "post", server.URL, plugin.Options{"json": data}); codeOf(err) != plugin.BadInput {
			t.Errorf("%s sent as JSON gave %v", what, err)
		}
	}
	if len(bodies) != before {
		t.Errorf("a request was sent with something in a secret's place: %q", bodies[before:])
	}
}
