package cli_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// second sets up a config with a client of its own, which the controller of
// this computer enrols under a ceiling.
func (m machine) second(name, ceiling string) machine {
	m.t.Helper()
	c := machine{m.t, filepath.Join(m.t.TempDir(), name)}
	card := c.ok("", "client", "card", "--name", name)
	c.ok(m.ok(card, "controller", "enrol", "-", "--ceiling", ceiling), "client", "trust", "-")
	return c
}

// A gateway acts as the clients of the configs --client names. Each line of
// its token file is a token, the client it names and the person it was given
// to, and with no file it makes a token for each client and prints it. A
// file with a token twice, or a client the gateway does not have, stops the
// gateway from starting, naming the line.
func Test_client_gateway_token_file(t *testing.T) {
	m, _ := working(t, "all")
	viewer, editor := m.second("viewer", "network"), m.second("editor", "at:/**")
	dir := files(t, map[string]string{"door.yaml": `statechart: door
initial: closed
states:
  closed:
    on: { knock: closed, open: opened }
  opened: { end: done }
`, "tokens": "views viewer\nalices editor alice@example.com\n\nbobs\teditor   Bob  Smith\n"})
	clients := []string{"--client", viewer.config, "--client", editor.config}
	listening := regexp.MustCompile(`listening on (http://\S+)`)
	gateway := func(flags ...string) (url, said string) {
		t.Helper()
		_, saying := m.staying(append(append([]string{"start", "gateway", "--listen", "127.0.0.1:0"}, clients...), flags...)...)
		until(t, "the gateway", func() bool { return listening.MatchString(saying.String()) })
		return listening.FindStringSubmatch(saying.String())[1], saying.String()
	}
	ask := func(url, token, method, path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(method, url+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}
	id := regexp.MustCompile(`"id":"([^"]+)"`)

	url, said := gateway("--token-file", filepath.Join(dir, "tokens"))
	if !strings.Contains(said, "gateway for the clients viewer, editor, listening on") || strings.Contains(said, "gateway token") {
		t.Fatalf("the gateway said %s", said)
	}
	at := editor.recorded(filepath.Join(dir, "door.yaml"))
	// Each token is answered as its line's client.
	status, started := ask(url, "alices", "POST", "/runs", `{"at": "`+at+`"}`)
	if status != 200 || !strings.Contains(started, `"id":"editor/`) {
		t.Fatalf("alice's start was answered %d %s", status, started)
	}
	run := id.FindStringSubmatch(started)[1]
	if status, called := ask(url, "views", "POST", "/runs", `{"at": "`+at+`"}`); status != 200 || !strings.Contains(called, `"id":"viewer/`) {
		t.Fatalf("the viewer's start was answered %d %s", status, called)
	}
	for _, wrong := range []string{"", "alice", "viewer", "editor"} {
		if status, text := ask(url, wrong, "GET", "/runs", ""); status != 401 {
			t.Errorf("the token %q was answered %d %s", wrong, status, text)
		}
	}
	// The person of a line is the rest of the line, and the run shows the
	// person who started it.
	if status, text := ask(url, "bobs", "POST", "/runs/"+run+"/events", `{"event": "knock"}`); status != 200 || !strings.Contains(text, `"taken":true`) {
		t.Fatalf("bob's event was answered %d %s", status, text)
	}
	if shown := editor.ok("", "run", "show", run); !strings.Contains(shown, `  person: "alice@example.com"`+"\n") {
		t.Errorf("run show printed %s", shown)
	}
	if shown := editor.ok("", "run", "show", run, "--record", "--json"); !strings.Contains(shown, `"person":"Bob Smith"`) || !strings.Contains(shown, `"person":"alice@example.com"`) {
		t.Errorf("run show --record --json printed %s", shown)
	}
	if listed := editor.ok("", "run", "ls", "--json"); !strings.Contains(listed, run) || strings.Contains(listed, "person") {
		t.Errorf("run ls --json printed %s", listed)
	}

	// With no file, a token for each client.
	url, said = gateway()
	for _, name := range []string{"viewer", "editor"} {
		token := regexp.MustCompile(`gateway token for the client ` + name + ` \(callers send it as Authorization: Bearer …\): (\S+)`).FindStringSubmatch(said)
		if token == nil {
			t.Fatalf("the gateway printed no token for %s: %s", name, said)
		}
		status, started := ask(url, token[1], "POST", "/runs", `{"at": "`+at+`"}`)
		if status != 200 || !strings.Contains(started, `"id":"`+name+`/`) {
			t.Errorf("the token printed for %s was answered %d %s", name, status, started)
		}
	}

	// A file the gateway cannot act on stops it from starting.
	for text, want := range map[string]string{
		"one viewer\ntwo editor\none editor\n": "line 3 has the token of line 1",
		"one viewer\ntwo nobody\n":             "line 2 names the client nobody, and the gateway's clients are: viewer, editor",
		"one viewer\ntwo\n":                    "line 2 names no client, and the gateway has several: viewer, editor",
	} {
		bad := filepath.Join(t.TempDir(), "tokens")
		os.WriteFile(bad, []byte(text), 0o600)
		args := append(append([]string{"start", "gateway", "--listen", "127.0.0.1:0"}, clients...), "--token-file", bad)
		if status, _, said := m.runFor(10*time.Second, "", args...); status != 1 || !strings.Contains(said, want) || strings.Contains(said, "listening") {
			t.Errorf("the token file %q ended %d and said %s", text, status, said)
		}
	}
	// A line with a token alone names the client of a gateway that has one.
	alone := filepath.Join(t.TempDir(), "token")
	os.WriteFile(alone, []byte("the-token\n"), 0o600)
	_, saying := m.staying("start", "gateway", "--listen", "127.0.0.1:0", "--token-file", alone)
	until(t, "the gateway", func() bool { return listening.MatchString(saying.String()) })
	if status, text := ask(listening.FindStringSubmatch(saying.String())[1], "the-token", "GET", "/runs", ""); status != 200 || !strings.Contains(saying.String(), "gateway for the client box-client, listening on") {
		t.Errorf("a token alone was answered %d %s by %s", status, text, saying.String())
	}
}

// A command a client sends gives one answer, in one set of words, with the
// client's own config and through a gateway with a token of that client's:
// what it prints is the same. Through the gateway it uses no config. A
// command that is not a client's refuses --gateway.
func Test_client_commands_through_a_gateway(t *testing.T) {
	m, _ := working(t, "all")
	editor := m.second("editor", "all,at:/**")
	dir := files(t, map[string]string{
		"tokens":                    "the-token editor alice@example.com\n",
		"house/devices/boiler.yaml": "statechart: boiler\nlifetime: singleton\ninitial: idle\nstates:\n  idle:\n    ticket: knock\n    on: { poke: idle, knock: idle, stop: off }\n  off: { end: done }\n",
		"house/greet.yaml":          "include: shared/base.yaml\nflow: greet\nstages:\n  say: { job: { script: ./greet.js, input: { name: \"${input.name}\" } } }\noutput: { text: \"${stages.say.output.text}\", report: \"${stages.say.output.report}\", colour: \"${values.COLOUR}\" }\n",
		"house/greet.js":            `log("greeting"); return { text: "hello " + input.name, report: blobs.make("the report") }`,
		"house/.corbieignore":       "shared\n",
		"house/shared/base.yaml":    "input: { name: any }\n",
		"house/bad.yaml":            "flow: bad\nstages: 3\n",
		"double.js":                 `return input.n * 2`,
	})
	_, saying := m.staying("start", "gateway", "--listen", "127.0.0.1:0", "--client", editor.config, "--token-file", filepath.Join(dir, "tokens"))
	listening := regexp.MustCompile(`listening on (http://\S+)`)
	until(t, "the gateway", func() bool { return listening.MatchString(saying.String()) })
	url := listening.FindStringSubmatch(saying.String())[1]
	t.Setenv("CORBIE_GATEWAY_TOKEN", "the-token")
	// A computer with nothing set up.
	bare := machine{t, filepath.Join(t.TempDir(), "none")}
	via := func(args ...string) (int, string, string) {
		t.Helper()
		return bare.run("", append(args, "--gateway", url)...)
	}
	sent := func(args ...string) string {
		t.Helper()
		status, out, said := via(args...)
		if status != 0 {
			t.Fatalf("corbie %s through the gateway ended %d:\n%s%s", strings.Join(args, " "), status, out, said)
		}
		return out
	}

	// A submit: the directory's files are sent, and the answer is the one a
	// submit prints.
	status, out, said := via("run", "submit", filepath.Join(dir, "house"))
	started := regexp.MustCompile(`/house/devices/boiler   singleton   started (editor/\S+)`).FindStringSubmatch(out)
	if status != 1 || started == nil || !strings.Contains(out, "/house/greet            transient   recorded") ||
		!strings.Contains(out, "bad.yaml\n    line 2: stages:") || !strings.Contains(out, "not set at its location or above: value COLOUR") || !strings.Contains(said, "1 of 3 files have problems") {
		t.Fatalf("the submit ended %d, printed:\n%sand said:\n%s", status, out, said)
	}
	boiler := started[1]
	// A script file, with what its job may do.
	if out := sent("run", "submit", filepath.Join(dir, "double.js"), "--timeout", "30s"); !strings.Contains(out, "/double   transient   recorded") {
		t.Fatalf("the submit of a script printed %s", out)
	}
	if out := sent("run", "start", "/double", "--input", `{"n": 21}`, "--wait"); !strings.Contains(out, "output: 42") {
		t.Fatalf("the script's run printed %s", out)
	}
	// Values and secrets.
	if out := sent("client", "value", "set", "COLOUR", "blue", "--at", "/house"); out != "taken\n" {
		t.Fatalf("the value's set printed %q", out)
	}
	if status, out, said := bare.run("the secret\n", "client", "secret", "set", "KEY", "-", "--at", "/house", "--gateway", url); status != 0 || out != "taken\n" {
		t.Fatalf("the secret's set ended %d: %s%s", status, out, said)
	}
	// A run, waited for and watched.
	out = sent("run", "start", "/house/greet", "--input", `{"name": "x"}`, "--wait")
	if !strings.Contains(out, "  done  flow greet  step 2") || !strings.Contains(out, `person: "alice@example.com"`) || !strings.Contains(out, `"colour":"blue"`) {
		t.Fatalf("the run waited for printed %s", out)
	}
	if out := sent("run", "start", "/house/greet", "--input", `{"name": "w"}`, "--watch"); !strings.Contains(out, "say log: greeting") || !strings.Contains(out, "  done  flow greet  step 2") {
		t.Fatalf("the run watched printed %s", out)
	}
	id := strings.TrimSpace(sent("run", "start", "/house/greet", "--input", `{"name": "y"}`))
	editor.ok("", "run", "show", id, "--wait")

	// What a command prints is the same both ways.
	for _, command := range [][]string{
		{"run", "show", id}, {"run", "show", id, "--json"}, {"run", "show", id, "--record"}, {"run", "show", id, "--record", "--json"},
		{"run", "show", id, "--step", "1"}, {"run", "show", id, "--definition"}, {"run", "show", id, "--wait"},
		{"run", "show", boiler, "--tickets"}, {"run", "show", boiler, "--tickets", "--json"}, {"run", "show", "/house/devices/boiler"},
		{"run", "ls", "--all"}, {"run", "ls", "--all", "--json"}, {"run", "ls", "--at", "/house/greet", "--status", "done"},
		{"client", "value", "list"}, {"client", "value", "list", "--json"}, {"client", "secret", "list"},
		{"run", "show", "editor/20261122T000000.000Z-0000000000"}, {"run", "show", id, "--step", "9"},
	} {
		wantStatus, want, wantSaid := editor.run("", command...)
		status, got, said := via(command...)
		if status != wantStatus || got != want || said != wantSaid {
			t.Errorf("corbie %s\nthrough the gateway ended %d, printed:\n%sand said:\n%s\nwith the client's config it ended %d, printed:\n%sand said:\n%s",
				strings.Join(command, " "), status, got, said, wantStatus, want, wantSaid)
		}
	}
	// The files of a result.
	saved := filepath.Join(t.TempDir(), "saved")
	sent("run", "show", id, "--save", saved)
	if data, err := os.ReadFile(filepath.Join(saved, "report")); err != nil || string(data) != "the report" {
		t.Errorf("the saved file is %q (%v)", data, err)
	}
	// An event to a location and to a run, an answer to a ticket, and a
	// cancel.
	for _, command := range [][]string{{"run", "send", "/house/devices/boiler", "poke"}, {"run", "send", boiler, "poke", `{"n": 1}`}} {
		if out := sent(command...); !strings.Contains(out, "taken") {
			t.Errorf("corbie %s printed %s", strings.Join(command, " "), out)
		}
	}
	ticket := strings.Fields(sent("run", "show", boiler, "--tickets"))[1]
	if out := sent("run", "reply", boiler, ticket, `"who is there"`); !strings.Contains(out, "taken") {
		t.Errorf("the reply printed %s", out)
	}
	if record := editor.ok("", "run", "show", boiler, "--record", "--json"); strings.Count(record, `"person":"alice@example.com"`) < 4 || !strings.Contains(record, "who is there") {
		t.Errorf("the boiler's record is %s", record)
	}
	sent("run", "cancel", boiler)
	if shown := editor.ok("", "run", "show", boiler); !strings.Contains(shown, "  cancelled  ") {
		t.Errorf("after the cancel the boiler is %s", shown)
	}

	// What is refused.
	if status, _, said := via("controller", "parties"); status != 2 || !strings.Contains(said, "--gateway is for the commands a client sends") {
		t.Errorf("corbie controller parties --gateway ended %d and said %s", status, said)
	}
	if status, _, said := via("check"); status != 2 || !strings.Contains(said, "--gateway is for the commands a client sends") {
		t.Errorf("corbie check --gateway ended %d and said %s", status, said)
	}
	t.Setenv("CORBIE_GATEWAY_TOKEN", "another")
	if status, _, said := via("run", "ls"); status != 1 || !strings.Contains(said, "does not show a token of the gateway's") {
		t.Errorf("a wrong token ended %d and said %s", status, said)
	}
	t.Setenv("CORBIE_GATEWAY_TOKEN", "")
	if status, _, said := via("run", "ls"); status != 1 || !strings.Contains(said, "CORBIE_GATEWAY_TOKEN") {
		t.Errorf("no token ended %d and said %s", status, said)
	}
	if _, err := os.Stat(bare.config); err == nil {
		t.Errorf("a command sent through a gateway made the config %s", bare.config)
	}
}

// A gateway given an OIDC issuer and an audience takes the issuer's tokens:
// a claim names the client and another the person. It makes no token of its
// own, and an issuer it cannot read stops it from starting.
func Test_client_gateway_oidc_flags(t *testing.T) {
	m, _ := working(t, "all")
	editor := m.second("editor", "all,at:/**")
	dir := files(t, map[string]string{"door.yaml": "statechart: door\ninitial: closed\nstates:\n  closed:\n    on: { open: opened }\n  opened: { end: done }\n"})

	// A stand-in for the issuer: it publishes one key, and signs with it.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	mux := http.NewServeMux()
	idp := httptest.NewServer(mux)
	defer idp.Close()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer": %q, "jwks_uri": %q}`, idp.URL, idp.URL+"/keys")
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"keys": [{"kty": "RSA", "kid": "k1", "use": "sig", "n": %q, "e": %q}]}`, b64(key.N.Bytes()), b64(big.NewInt(int64(key.E)).Bytes()))
	})
	token := func(claims string) string {
		signed := b64([]byte(`{"alg":"RS256","kid":"k1"}`)) + "." + b64([]byte(claims))
		digest := sha256.Sum256([]byte(signed))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return signed + "." + b64(signature)
	}
	alice := token(fmt.Sprintf(`{"iss": %q, "aud": "api://corbie", "exp": %d, "roles": ["editor"], "sub": "s-1", "preferred_username": "alice@example.com"}`, idp.URL, time.Now().Add(time.Hour).Unix()))

	gateway := []string{"start", "gateway", "--listen", "127.0.0.1:0", "--client", editor.config}
	_, saying := m.staying(append(gateway, "--oidc-issuer", idp.URL, "--oidc-audience", "api://corbie", "--oidc-person", "preferred_username")...)
	listening := regexp.MustCompile(`listening on (http://\S+)`)
	until(t, "the gateway", func() bool { return listening.MatchString(saying.String()) })
	url := listening.FindStringSubmatch(saying.String())[1]
	if said := saying.String(); strings.Contains(said, "gateway token") ||
		!strings.Contains(said, "gateway takes the tokens of the OIDC issuer "+idp.URL+" for the audience api://corbie: the client is named by roles, and the person by preferred_username") {
		t.Fatalf("the gateway said %s", said)
	}
	// The issuer's token, sent by a command.
	bare := machine{t, filepath.Join(t.TempDir(), "none")}
	t.Setenv("CORBIE_GATEWAY_TOKEN", alice)
	if status, out, said := bare.run("", "run", "submit", filepath.Join(dir, "door.yaml"), "--gateway", url); status != 0 || !strings.Contains(out, "/door   transient   recorded") {
		t.Fatalf("the submit ended %d: %s%s", status, out, said)
	}
	_, id, _ := bare.run("", "run", "start", "/door", "--gateway", url)
	id = strings.TrimSpace(id)
	until(t, "the run", func() bool { return strings.Contains(editor.ok("", "run", "show", id), "running") })
	if shown := editor.ok("", "run", "show", id); !strings.HasPrefix(id, "editor/") || !strings.Contains(shown, `person: "alice@example.com"`) {
		t.Errorf("the run started with the issuer's token is %s", shown)
	}
	t.Setenv("CORBIE_GATEWAY_TOKEN", alice[:len(alice)-3]+"AAA")
	if status, _, said := bare.run("", "run", "ls", "--gateway", url); status != 1 || !strings.Contains(said, "the request's token is not taken: its signature is not the issuer's") {
		t.Errorf("a changed token ended %d and said %s", status, said)
	}
	// By default the person is the token's sub.
	_, saying = m.staying(append(gateway, "--oidc-issuer", idp.URL, "--oidc-audience", "api://corbie")...)
	until(t, "the gateway", func() bool { return listening.MatchString(saying.String()) })
	if !strings.Contains(saying.String(), "the client is named by roles, and the person by sub") {
		t.Errorf("the gateway said %s", saying.String())
	}

	// What stops the gateway from starting.
	for want, flags := range map[string][]string{
		"--oidc-issuer and --oidc-audience are given together":        {"--oidc-issuer", idp.URL},
		"--oidc-issuer and --oidc-audience are given together#":       {"--oidc-audience", "api://corbie"},
		"an OIDC issuer's address is https, or http on this computer": {"--oidc-issuer", "http://issuer.example.com", "--oidc-audience", "a"},
		"names itself": {"--oidc-issuer", idp.URL + "/", "--oidc-audience", "a"},
		"the OIDC issuer " + idp.URL + "/other: " + idp.URL + "/other/.well": {"--oidc-issuer", idp.URL + "/other", "--oidc-audience", "a"},
	} {
		want, _, _ = strings.Cut(want, "#")
		if status, _, said := m.runFor(20*time.Second, "", append(gateway, flags...)...); status != 1 || !strings.Contains(said, want) || strings.Contains(said, "listening") {
			t.Errorf("corbie start gateway %s ended %d and said %s", strings.Join(flags, " "), status, said)
		}
	}
	if status, _, said := m.run("", "start", "runner", "--oidc-issuer", idp.URL); status != 2 || !strings.Contains(said, "--oidc-issuer is for the gateway, not the runner") {
		t.Errorf("--oidc-issuer after the runner ended %d and said %s", status, said)
	}
}
