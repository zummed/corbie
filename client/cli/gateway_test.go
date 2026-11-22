package cli_test

import (
	"io"
	"net/http"
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
