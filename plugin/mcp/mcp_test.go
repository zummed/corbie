package mcp_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	_ "github.com/zummed/corbie/plugin/bundled"
	"github.com/zummed/corbie/plugin/mcp/standin"
)

// A test's own binary is the server a program is started as: told so, it
// speaks MCP on its standard streams and does nothing else.
func TestMain(m *testing.M) {
	switch os.Getenv("MCP_STANDIN") {
	case "":
		os.Exit(m.Run())
	case "notes":
		(&standin.Server{Legacy: os.Getenv("MCP_LEGACY")}).Serve(os.Stdin, os.Stdout)
	case "broken":
		fmt.Fprintln(os.Stderr, "the notes file is missing")
		os.Exit(3)
	case "silent":
		io.Copy(io.Discard, os.Stdin)
	}
	os.Exit(0)
}

func newJob(t *testing.T, secrets map[string]string, allow ...string) *plugin.Job {
	t.Helper()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), Secrets: secrets, SearchPath: os.Getenv("PATH"),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: allow}}}}
	job.Perms.Scratch = job.Scratch
	t.Cleanup(job.End)
	return job
}

func mcp(job *plugin.Job, name string, first any, opt plugin.Options) (*plugin.Object, error) {
	m, _ := plugin.Find("mcp")
	out, err := m.Calls[name].Invoke(job, "mcp", name, first, opt)
	if err != nil {
		return nil, err
	}
	return out.(*plugin.Object), nil
}

func on(job *plugin.Job, server *plugin.Object, name string, first any, opt plugin.Options) (any, error) {
	return server.Calls[name].Invoke(job, "server", name, first, opt)
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

// self is this test's binary, which is started as a server.
func self(t *testing.T) string {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// way is one way to a server of notes: over the web, or as a program.
type way struct {
	name    string
	legacy  string
	server  *standin.Server // the one over the web
	connect func(job *plugin.Job, opt plugin.Options) (*plugin.Object, error)
}

func ways(t *testing.T, versions ...string) []way {
	var out []way
	for _, legacy := range versions {
		server := &standin.Server{Legacy: legacy}
		web := httptest.NewServer(server)
		t.Cleanup(web.Close)
		out = append(out, way{"the web", legacy, server, func(job *plugin.Job, opt plugin.Options) (*plugin.Object, error) {
			return mcp(job, "connect", web.URL+"/mcp", opt)
		}}, way{"a program", legacy, nil, func(job *plugin.Job, opt plugin.Options) (*plugin.Object, error) {
			if opt == nil {
				opt = plugin.Options{}
			}
			opt["env"] = map[string]any{"MCP_STANDIN": "notes", "MCP_LEGACY": legacy}
			return mcp(job, "start", self(t), opt)
		}})
	}
	return out
}

func names(tools any) []string {
	var out []string
	for _, tool := range tools.([]any) {
		out = append(out, tool.(map[string]any)["name"].(string))
	}
	return out
}

// The same script lists the same tools and gets the same result, from a
// server of latest and from one of an earlier version, over the web and
// as a program.
func TestEveryVersionOverEveryWay(t *testing.T) {
	for _, w := range ways(t, "", "2025-11-25", "2025-06-18") {
		job := newJob(t, nil, "all")
		server, err := w.connect(job, nil)
		if err != nil {
			t.Fatalf("%s, %q: %v", w.name, w.legacy, err)
		}
		tools, err := on(job, server, "tools", nil, nil)
		want := []string{"add_note", "find_notes", "delete_notes", "picture", "locked", "badly_marked"}
		if w.server != nil {
			// Over the web a tool whose header cannot be carried is left out.
			want = want[:5]
		}
		if err != nil || !reflect.DeepEqual(names(tools), want) {
			t.Fatalf("%s, %q: the tools were %v, %v", w.name, w.legacy, tools, err)
		}
		args := tools.([]any)[1].(map[string]any)["args"].(map[string]any)
		if args["required"].([]any)[0] != "text" || args["type"] != "object" {
			t.Errorf("%s, %q: the args of find_notes were %v", w.name, w.legacy, args)
		}
		found, err := on(job, server, "call", "find_notes", plugin.Options{"args": map[string]any{"text": "tea"}})
		if err != nil || !reflect.DeepEqual(found, map[string]any{"content": []any{"2 notes: tea at 4, tea at 5"}}) {
			t.Fatalf("%s, %q: the call gave %v, %v", w.name, w.legacy, found, err)
		}
		server.Close()
		if _, err := on(job, server, "tools", nil, nil); codeOf(err) != plugin.BadInput {
			t.Errorf("%s, %q: a closed server gave %v", w.name, w.legacy, err)
		}
		// A server of an earlier version over the web is told the
		// conversation has ended.
		if w.server != nil && w.legacy != "" && !reflect.DeepEqual(w.server.Ended, []string{"session-1"}) {
			t.Errorf("%s, %q: the sessions ended were %v", w.name, w.legacy, w.server.Ended)
		}
	}
}

// A call gives text as text, a picture as a blob, a link as a link, and
// structured content as data. A tool that reports an error fails the call
// with its text.
func TestCallGivesWhatTheToolGave(t *testing.T) {
	for _, w := range ways(t, "") {
		job := newJob(t, nil, "all")
		server, err := w.connect(job, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := on(job, server, "call", "picture", nil)
		if err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}
		out := got.(map[string]any)
		picture, _ := out["content"].([]any)[0].(*plugin.Blob)
		if picture == nil || picture.Type != "image/png" || picture.Size != int64(len("\x89PNG a picture")) {
			t.Errorf("%s: the picture was %#v", w.name, out["content"])
		}
		if link := out["content"].([]any)[1]; !reflect.DeepEqual(link, map[string]any{"link": "notes://all", "name": "all notes"}) {
			t.Errorf("%s: the link was %v", w.name, link)
		}
		if !reflect.DeepEqual(out["data"], map[string]any{"width": 1.0, "height": 1.0}) {
			t.Errorf("%s: the data was %v", w.name, out["data"])
		}
		if _, err := on(job, server, "call", "locked", nil); codeOf(err) != plugin.Failed || err.Error() != "the notes are locked" {
			t.Errorf("%s: a tool that failed gave %v", w.name, err)
		}
		if _, err := on(job, server, "call", "burn", nil); codeOf(err) != plugin.BadInput || !strings.Contains(err.Error(), "Unknown tool: burn") {
			t.Errorf("%s: a tool the server does not have gave %v", w.name, err)
		}
		// A tool's arguments are data, and never a handle or a blob.
		for _, args := range []map[string]any{{"text": plugin.NewSecret("K")}, {"text": plugin.NewBlob("", []byte("b"))}} {
			if _, err := on(job, server, "call", "add_note", plugin.Options{"args": args}); codeOf(err) != plugin.BadInput {
				t.Errorf("%s: arguments holding %T gave %v", w.name, args["text"], err)
			}
		}
	}
}

// A server that asks for input gives its requests and its state, and a
// later job carries the call on with the responses.
func TestInputIsAskedForAndGiven(t *testing.T) {
	for _, w := range ways(t, "") {
		first := newJob(t, nil, "all")
		server, err := w.connect(first, nil)
		if err != nil {
			t.Fatal(err)
		}
		asked, err := on(first, server, "call", "delete_notes", plugin.Options{"args": map[string]any{"text": "tea"}})
		requests, _ := asked.(map[string]any)["requests"].(map[string]any)
		confirm, _ := requests["confirm"].(map[string]any)
		if err != nil || confirm["method"] != "elicitation/create" || asked.(map[string]any)["state"] != "delete:tea" {
			t.Fatalf("%s: the first call gave %v, %v", w.name, asked, err)
		}
		first.End()
		// A program started again keeps no notes: each starts with the
		// same three, and it is the state that carries the call on.
		second := newJob(t, nil, "all")
		server, err = w.connect(second, nil)
		if err != nil {
			t.Fatal(err)
		}
		done, err := on(second, server, "call", "delete_notes", plugin.Options{"args": map[string]any{"text": "tea"},
			"responses": map[string]any{"confirm": map[string]any{"action": "accept"}}, "state": "delete:tea"})
		if err != nil || !reflect.DeepEqual(done, map[string]any{"content": []any{"deleted 2 notes"}}) {
			t.Fatalf("%s: the second call gave %v, %v", w.name, done, err)
		}
	}
	// A server of an earlier version has no requests for input.
	for _, w := range ways(t, "2025-11-25") {
		job := newJob(t, nil, "all")
		server, err := w.connect(job, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := on(job, server, "call", "delete_notes", plugin.Options{"state": "delete:tea"}); codeOf(err) != plugin.BadInput {
			t.Errorf("%s: responses to a server of 2025-11-25 gave %v", w.name, err)
		}
	}
}

// Over the web a request carries its method, its tool and the arguments
// the tool's schema marks in headers, in base64 where a header cannot
// hold them as they are. A server that wants a header the plugin did not
// know of has its tools listed, and the call made again.
func TestHeadersSayWhatTheBodyDoes(t *testing.T) {
	w := ways(t, "")[0]
	job := newJob(t, nil, "all")
	server, err := w.connect(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	found, err := on(job, server, "call", "find_notes", plugin.Options{"args": map[string]any{"text": "cake", "shelf": "Küche"}})
	if err != nil || !reflect.DeepEqual(found, map[string]any{"content": []any{"1 notes: cake at 6"}}) {
		t.Fatalf("the call gave %v, %v", found, err)
	}
	last := w.server.Requests[len(w.server.Requests)-1]
	for name, want := range map[string]string{"Mcp-Method": "tools/call", "Mcp-Name": "find_notes", "MCP-Protocol-Version": "2026-07-28",
		"Mcp-Param-Shelf": "=?base64?" + base64.StdEncoding.EncodeToString([]byte("Küche")) + "?=", "Accept": "application/json, text/event-stream"} {
		if got := last.Get(name); got != want {
			t.Errorf("the call's %s was %q", name, got)
		}
	}
	// The call was refused once, the tools listed, and the call made again.
	var methods []string
	for _, h := range w.server.Requests {
		methods = append(methods, h.Get("Mcp-Method"))
	}
	if got := strings.Join(methods, " "); got != "server/discover tools/call tools/list tools/list tools/list tools/call" {
		t.Errorf("the requests were %s", got)
	}
}

// A server is reached only with the permission its page names, and a
// call without it reaches nothing.
func TestServerNeedsItsPermission(t *testing.T) {
	ws := ways(t, "")
	web, program := ws[0], ws[1]
	if _, err := web.connect(newJob(t, nil, "exec"), nil); !strings.HasPrefix(refusedAs(err), "net:127.0.0.1:") {
		t.Errorf("a server over the web was reached without net: %v", err)
	}
	if len(web.server.Requests) != 0 {
		t.Errorf("the server was sent %d requests", len(web.server.Requests))
	}
	if _, err := program.connect(newJob(t, nil, "network"), nil); refusedAs(err) != "exec:"+self(t) {
		t.Errorf("a program was started without exec: %v", err)
	}
	if _, err := program.connect(newJob(t, nil, "exec:"+self(t)), nil); err != nil {
		t.Errorf("a program was not started with exec for it: %v", err)
	}
}

// A program that ends says what it printed as errors; one that says
// nothing is waited for to the timeout and no further.
func TestProgramThatEndsOrSaysNothing(t *testing.T) {
	job := newJob(t, nil, "all")
	_, err := mcp(job, "start", self(t), plugin.Options{"env": map[string]any{"MCP_STANDIN": "broken"}})
	if codeOf(err) != plugin.Failed || !strings.Contains(err.Error(), "ended, and printed: the notes file is missing") {
		t.Errorf("a program that ended gave %v", err)
	}
	began := time.Now()
	_, err = mcp(job, "start", self(t), plugin.Options{"env": map[string]any{"MCP_STANDIN": "silent"}, "timeout": 1.0})
	if codeOf(err) != plugin.TimedOut || time.Since(began) > 5*time.Second {
		t.Errorf("a program that said nothing gave %v after %s", err, time.Since(began))
	}
}

// signIn is a sign-in server that gives a token for a client's secret or
// for an assertion signed with its key.
type signIn struct {
	*httptest.Server
	secret string
	key    ed25519.PublicKey
	token  string
	// asked are the forms it was sent.
	asked []map[string]string
}

func newSignIn(t *testing.T, token string) *signIn {
	s := &signIn{token: token, secret: "s3cret-of-the-client"}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			json.NewEncoder(w).Encode(map[string]any{"issuer": s.URL, "token_endpoint": s.URL + "/token",
				"token_endpoint_auth_methods_supported": []any{"client_secret_basic", "private_key_jwt"}})
		case "/token":
			r.ParseForm()
			form := map[string]string{}
			for k := range r.PostForm {
				form[k] = r.PostForm.Get(k)
			}
			s.asked = append(s.asked, form)
			id, secret, basic := r.BasicAuth()
			good := basic && id == "corbie-client" && secret == s.secret
			if assertion := form["client_assertion"]; assertion != "" {
				good = s.checks(assertion, "corbie-client", s.URL+"/token")
			}
			if !good || form["grant_type"] != "client_credentials" {
				w.WriteHeader(401)
				json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client", "error_description": "the client is not known"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": s.token, "token_type": "Bearer"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *signIn) checks(assertion, id, audience string) bool {
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return false
	}
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if !ed25519.Verify(s.key, []byte(parts[0]+"."+parts[1]), signature) {
		return false
	}
	var claims map[string]any
	text, _ := base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(text, &claims)
	return claims["iss"] == id && claims["sub"] == id && claims["aud"] == audience && claims["exp"].(float64) > float64(time.Now().Unix())
}

// A server takes a bearer token in a header, or one the plugin gets from
// its sign-in server with a client's secret or key. Every host the sign-in
// reaches is checked as the server is.
func TestSignIn(t *testing.T) {
	const token = "the-t0ken-for-notes"
	signIn := newSignIn(t, token)
	notes := &standin.Server{Token: token}
	mux := http.NewServeMux()
	mux.Handle("/mcp", notes)
	web := httptest.NewServer(mux)
	t.Cleanup(web.Close)
	notes.Metadata = web.URL + "/.well-known/oauth-protected-resource/mcp"
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"resource": web.URL + "/mcp", "authorization_servers": []any{signIn.URL}})
	})
	address := web.URL + "/mcp"
	server, signer := strings.TrimPrefix(web.URL, "http://"), strings.TrimPrefix(signIn.URL, "http://")
	works := func(job *plugin.Job, opt plugin.Options) error {
		s, err := mcp(job, "connect", address, opt)
		if err != nil {
			return err
		}
		_, err = on(job, s, "call", "find_notes", plugin.Options{"args": map[string]any{"text": "tea"}})
		return err
	}

	// A bearer token in a header, as a handle.
	bearer, _ := plugin.Join("Bearer ", plugin.NewSecret("NOTES_TOKEN"))
	job := newJob(t, map[string]string{"NOTES_TOKEN": token}, "net:"+server, "secret:NOTES_TOKEN@net.unencrypted:"+server)
	if err := works(job, plugin.Options{"headers": map[string]any{"Authorization": bearer}}); err != nil {
		t.Errorf("a token in a header gave %v", err)
	}
	if err := works(newJob(t, nil, "net:"+server), nil); codeOf(err) != plugin.Refused || !strings.Contains(err.Error(), "wants a sign-in") {
		t.Errorf("no sign-in gave %v", err)
	}

	// A client's secret, which goes to the sign-in server and nowhere else.
	client := map[string]any{"id": "corbie-client", "secret": plugin.NewSecret("CLIENT_SECRET")}
	secrets := map[string]string{"CLIENT_SECRET": signIn.secret}
	job = newJob(t, secrets, "net:"+server, "net:"+signer, "secret:CLIENT_SECRET@net.unencrypted:"+signer)
	if err := works(job, plugin.Options{"client": client}); err != nil {
		t.Fatalf("a client's secret gave %v", err)
	}
	if asked := signIn.asked[len(signIn.asked)-1]; asked["resource"] != address || asked["scope"] != "notes:read" || asked["client_secret"] != "" {
		t.Errorf("the sign-in server was sent %v", asked)
	}
	// Without the sign-in server's host the sign-in is refused, naming it,
	// and so is a secret bound to the server and not the sign-in server.
	if err := works(newJob(t, secrets, "net:"+server, "secret:CLIENT_SECRET@net.unencrypted:"+signer), plugin.Options{"client": client}); refusedAs(err) != "net:"+signer {
		t.Errorf("a sign-in without its host gave %v", err)
	}
	if err := works(newJob(t, secrets, "net:"+server, "net:"+signer, "secret:CLIENT_SECRET@net.unencrypted:"+server), plugin.Options{"client": client}); refusedAs(err) != "secret:CLIENT_SECRET@net.unencrypted:"+signer {
		t.Errorf("a secret bound to the server gave %v", err)
	}
	signIn.secret = "another"
	if err := works(job, plugin.Options{"client": client}); codeOf(err) != plugin.Refused || !strings.Contains(err.Error(), "the client is not known") {
		t.Errorf("a wrong secret gave %v", err)
	}

	// A client's key, which the plugin signs with: a key is used only when
	// it may go anywhere.
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	signIn.key = public
	der, _ := x509.MarshalPKCS8PrivateKey(private)
	keys := map[string]string{"CLIENT_KEY": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))}
	keyed := map[string]any{"id": "corbie-client", "key": plugin.NewSecret("CLIENT_KEY")}
	if err := works(newJob(t, keys, "net:"+server, "net:"+signer, "secret:CLIENT_KEY@*"), plugin.Options{"client": keyed}); err != nil {
		t.Errorf("a client's key gave %v", err)
	}
	if err := works(newJob(t, keys, "net:"+server, "net:"+signer, "secret:CLIENT_KEY@net.unencrypted:"+signer), plugin.Options{"client": keyed}); refusedAs(err) != "secret:CLIENT_KEY@*" {
		t.Errorf("a key bound to the sign-in server gave %v", err)
	}

	// A client is written whole, and as handles.
	for _, wrong := range []map[string]any{{"id": "corbie-client", "secret": "s3cret"}, {"secret": plugin.NewSecret("CLIENT_SECRET")},
		{"id": "corbie-client"}, {"id": "corbie-client", "secret": plugin.NewSecret("A"), "key": plugin.NewSecret("B")}} {
		if _, err := mcp(job, "connect", address, plugin.Options{"client": wrong}); codeOf(err) != plugin.BadInput {
			t.Errorf("the client %v gave %v", wrong, err)
		}
	}
}
