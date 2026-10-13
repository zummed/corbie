package net

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	gonet "net"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

func newJob(t *testing.T, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}},
		Scratch: t.TempDir(),
		Secrets: map[string]string{"PASS": "hunter22"},
	}
	t.Cleanup(job.End)
	return job
}

// serve starts a listener on this machine and hands each connection to
// serve, giving the listener's address.
func serve(t *testing.T, handle func(gonet.Conn)) string {
	t.Helper()
	l, err := gonet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				handle(conn)
			}()
		}
	}()
	return l.Addr().String()
}

func echo(conn gonet.Conn) { io.Copy(conn, conn) }

// certificate makes a certificate for a name or an address, signed by
// itself, and gives it with its PEM text.
func certificate(t *testing.T, name string) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	if ip := gonet.ParseIP(name); ip != nil {
		template.IPAddresses = []gonet.IP{ip}
	} else {
		template.DNSNames = []string{name}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func open(t *testing.T, job *plugin.Job, address string, opt plugin.Options) *plugin.Object {
	t.Helper()
	module, _ := plugin.Find("net")
	c, err := module.Calls["connect"].Invoke(job, "net", "connect", address, opt)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return c.(*plugin.Object)
}

func call(job *plugin.Job, c *plugin.Object, name string, first any, opt plugin.Options) (any, error) {
	return c.Calls[name].Invoke(job, "object", name, first, opt)
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func Test_net_line_and_bytes_out_and_back(t *testing.T) {
	job := newJob(t, "all")
	c := open(t, job, serve(t, echo), nil)
	if _, err := call(job, c, "write", "hello\r\n", nil); err != nil {
		t.Fatal(err)
	}
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "hello" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
	if _, err := call(job, c, "write", plugin.Bytes("0123456789"), nil); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for len(got) < 10 {
		data, err := call(job, c, "read", map[string]any{"max": 4.0}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(data.(plugin.Bytes)) > 4 {
			t.Fatalf("read gave %d bytes, past its max", len(data.(plugin.Bytes)))
		}
		got = append(got, data.(plugin.Bytes)...)
	}
	if string(got) != "0123456789" {
		t.Fatalf("read gave %q", got)
	}
}

func Test_net_close(t *testing.T) {
	job := newJob(t, "all")
	closed := make(chan error, 1)
	c := open(t, job, serve(t, func(conn gonet.Conn) {
		_, err := conn.Read(make([]byte, 1))
		closed <- err
	}), nil)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != io.EOF {
		t.Fatalf("the other end saw %v, not the connection closed", err)
	}
	if _, err := call(job, c, "write", "more", nil); code(err) != plugin.BadInput {
		t.Fatalf("a call after close gave %v", err)
	}
}

func Test_net_end_of_connection(t *testing.T) {
	job := newJob(t, "all")
	c := open(t, job, serve(t, func(conn gonet.Conn) { conn.Write([]byte("one\nlast")) }), nil)
	for _, want := range []any{"one", "last", nil} {
		if line, err := call(job, c, "readLine", nil, nil); err != nil || line != want {
			t.Fatalf("readLine gave %v, %v, not %v", line, err, want)
		}
	}
	if data, err := call(job, c, "read", nil, nil); err != nil || data != nil {
		t.Fatalf("read at the end gave %v, %v", data, err)
	}
}

func Test_net_unknown_option(t *testing.T) {
	job := newJob(t, "all")
	c := open(t, job, serve(t, echo), nil)
	_, err := call(job, c, "read", map[string]any{"mx": 4.0}, nil)
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"mx"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
	module, _ := plugin.Find("net")
	_, err = module.Calls["connect"].Invoke(job, "net", "connect", "127.0.0.1:1", plugin.Options{"tsl": true})
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"tsl"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
}

func Test_net_refused_without_permission(t *testing.T) {
	seen := make(chan bool, 1)
	address := serve(t, func(gonet.Conn) { seen <- true })
	job := newJob(t)
	module, _ := plugin.Find("net")
	_, err := module.Calls["connect"].Invoke(job, "net", "connect", address, nil)
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || refusal.What != "net:"+address {
		t.Fatalf("connect gave %v, not a refusal naming net:%s", err, address)
	}
	select {
	case <-seen:
		t.Fatal("the server saw a connection")
	case <-time.After(50 * time.Millisecond):
	}
}

func Test_net_encrypted_checks_the_name(t *testing.T) {
	right, rightPEM := certificate(t, "127.0.0.1")
	wrong, wrongPEM := certificate(t, "other.example")
	secure := func(cert tls.Certificate) string {
		return serve(t, func(conn gonet.Conn) {
			echo(tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}}))
		})
	}
	job := newJob(t, "all")
	c := open(t, job, secure(right), plugin.Options{"tls": true, "trust": rightPEM})
	// A secret bound to the host may go over the encrypted connection.
	if _, err := call(job, c, "write", plugin.NewSecret("PASS"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := call(job, c, "write", "\n", nil); err != nil {
		t.Fatal(err)
	}
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "hunter22" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}

	module, _ := plugin.Find("net")
	_, err := module.Calls["connect"].Invoke(job, "net", "connect", secure(wrong), plugin.Options{"tls": true, "trust": wrongPEM})
	if code(err) != plugin.Refused {
		t.Fatalf("a certificate for another name gave %v", err)
	}
	// A certificate nobody the script trusts signed is turned down too.
	_, err = module.Calls["connect"].Invoke(job, "net", "connect", secure(right), plugin.Options{"tls": true, "trust": wrongPEM})
	if code(err) != plugin.Refused {
		t.Fatalf("a certificate signed by no one trusted gave %v", err)
	}
	_, err = module.Calls["connect"].Invoke(job, "net", "connect", secure(right), plugin.Options{"tls": true, "trust": "not a certificate"})
	if code(err) != plugin.BadInput {
		t.Fatalf("trust that cannot be read gave %v", err)
	}
}

func Test_sec_encrypted_judged_when_the_value_goes(t *testing.T) {
	cert, certPEM := certificate(t, "127.0.0.1")
	plain := make(chan string, 1)
	address := serve(t, func(conn gonet.Conn) {
		// A line in the clear, and then the rest protected.
		line := make([]byte, 64)
		n, _ := conn.Read(line)
		plain <- string(line[:n])
		echo(tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}}))
	})
	host, _, _ := gonet.SplitHostPort(address)
	job := newJob(t, "net:"+host, "secret:PASS@net:"+host)
	c := open(t, job, address, nil)
	// The connection was opened plain, and while it is plain a secret
	// bound to the host does not go into it.
	_, err := call(job, c, "write", plugin.NewSecret("PASS"), nil)
	var refusal *permission.Refusal
	if !errors.As(err, &refusal) || refusal.What != "secret:PASS@net.unencrypted:"+address || !strings.Contains(refusal.Fix, "not encrypted") {
		t.Fatalf("a secret bound to a host, over a plain connection, gave %v", err)
	}
	if _, err := call(job, c, "write", "STARTTLS\n", nil); err != nil {
		t.Fatal(err)
	}
	if got := <-plain; got != "STARTTLS\n" {
		t.Fatalf("in the clear the server was sent %q", got)
	}
	// Turned encrypted partway, the same connection carries it.
	if _, err := call(job, c, "startTLS", map[string]any{"trust": certPEM}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := call(job, c, "write", plugin.NewSecret("PASS"), nil); err != nil {
		t.Fatalf("after the connection was encrypted the secret gave %v", err)
	}
	call(job, c, "write", "\n", nil)
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "hunter22" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
	// A secret bound to anywhere may go over a plain one.
	anywhere := newJob(t, "net:"+host, "secret:PASS@*")
	c = open(t, anywhere, serve(t, echo), nil)
	if _, err := call(anywhere, c, "write", plugin.NewSecret("PASS"), nil); err != nil {
		t.Fatalf("a secret bound to anywhere, over a plain connection, gave %v", err)
	}
}

func Test_net_start_tls(t *testing.T) {
	cert, certPEM := certificate(t, "127.0.0.1")
	address := serve(t, func(conn gonet.Conn) {
		conn.Write([]byte("220 ready\r\n"))
		echo(tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}}))
	})
	job := newJob(t, "all")
	c := open(t, job, address, nil)
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "220 ready" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
	if _, err := call(job, c, "startTLS", map[string]any{"trust": certPEM}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := call(job, c, "write", "protected\n", nil); err != nil {
		t.Fatal(err)
	}
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "protected" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
	if _, err := call(job, c, "startTLS", nil, nil); code(err) != plugin.BadInput {
		t.Fatalf("encrypting twice gave %v", err)
	}
}

func Test_plug_net_no_encrypting_over_unread_bytes(t *testing.T) {
	cert, certPEM := certificate(t, "127.0.0.1")
	handshakes := make(chan error, 1)
	address := serve(t, func(conn gonet.Conn) {
		// What follows the answer was put in by someone in the middle.
		conn.Write([]byte("220 ready\r\nMAIL FROM:<someone@else.example>\r\n"))
		secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
		conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
		handshakes <- secure.Handshake()
	})
	job := newJob(t, "all")
	c := open(t, job, address, nil)
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "220 ready" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
	_, err := call(job, c, "startTLS", map[string]any{"trust": certPEM}, nil)
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), "unread") {
		t.Fatalf("encrypting over unread bytes gave %v", err)
	}
	if err := <-handshakes; err == nil {
		t.Fatal("the connection was encrypted")
	}
	// Once they are read, as what they are, the connection is still plain.
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "MAIL FROM:<someone@else.example>" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
}

func Test_plug_net_read_time_limit_keeps_connection(t *testing.T) {
	rest := make(chan bool)
	address := serve(t, func(conn gonet.Conn) {
		conn.Write([]byte("par"))
		<-rest
		conn.Write([]byte("tial\nmore"))
		echo(conn)
	})
	job := newJob(t, "all")
	c := open(t, job, address, nil)
	_, err := call(job, c, "readLine", map[string]any{"timeout": 0.1}, nil)
	if code(err) != plugin.TimedOut {
		t.Fatalf("a line that did not come gave %v", err)
	}
	close(rest)
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "partial" {
		t.Fatalf("after the wait ran out, readLine gave %v, %v", line, err)
	}
	if data, err := call(job, c, "read", nil, nil); err != nil || string(data.(plugin.Bytes)) != "more" {
		t.Fatalf("read gave %v, %v", data, err)
	}
	// A quiet connection that is looked in on stays open.
	if _, err := call(job, c, "read", map[string]any{"timeout": "50ms"}, nil); code(err) != plugin.TimedOut {
		t.Fatalf("a read with nothing to read gave %v", err)
	}
	if _, err := call(job, c, "write", "still here\n", nil); err != nil {
		t.Fatal(err)
	}
	if line, err := call(job, c, "readLine", nil, nil); err != nil || line != "still here" {
		t.Fatalf("readLine gave %v, %v", line, err)
	}
}

func Test_plug_every_wait_has_a_deadline(t *testing.T) {
	wait = 100 * time.Millisecond
	t.Cleanup(func() { wait = 30 * time.Second })
	silent := serve(t, func(conn gonet.Conn) { io.Copy(io.Discard, conn) })
	job := newJob(t, "all")
	c := open(t, job, silent, nil)
	for _, name := range []string{"read", "readLine", "startTLS"} {
		if _, err := call(job, c, name, nil, nil); code(err) != plugin.TimedOut {
			t.Fatalf("%s, with no time given, on a connection that says nothing gave %v", name, err)
		}
	}
	module, _ := plugin.Find("net")
	_, err := module.Calls["connect"].Invoke(job, "net", "connect", silent, plugin.Options{"tls": true})
	if code(err) != plugin.TimedOut {
		t.Fatalf("connecting, with no time given, to a server that says nothing gave %v", err)
	}
}

func Test_plug_left_open_closed_at_job_end(t *testing.T) {
	closed := make(chan error, 1)
	address := serve(t, func(conn gonet.Conn) {
		_, err := conn.Read(make([]byte, 1))
		closed <- err
	})
	job := newJob(t, "all")
	open(t, job, address, nil)
	job.End()
	select {
	case err := <-closed:
		if err != io.EOF {
			t.Fatalf("the other end saw %v, not the connection closed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the connection outlived the job")
	}
}
