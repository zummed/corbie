package serve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/ui/page"

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
)

// corbie runs one of Corbie's commands, and fails the test if it fails.
func corbie(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	var out, said bytes.Buffer
	if status := cli.Run(context.Background(), args, strings.NewReader(stdin), &out, &said); status != 0 {
		t.Fatalf("corbie %s: %s%s", strings.Join(args, " "), said.String(), out.String())
	}
	return out.String()
}

// lab sets up one computer, as corbie init does, under a name.
func lab(t *testing.T, dir, name string) string {
	t.Helper()
	config := filepath.Join(dir, name)
	corbie(t, "", "init", "--store", filepath.Join(dir, name+"-store"), "--name", name, "--ceiling", "all", "--config", config)
	return config
}

// viewer sets up a viewer's config beside the lab in a directory: the
// lab's controller's card and its store, and no identity.
func viewer(t *testing.T, dir string) string {
	t.Helper()
	card := regexp.MustCompile(`corbie-card:\S+`).FindString(corbie(t, "", "controller", "card", "--config", filepath.Join(dir, "lab")))
	config := filepath.Join(dir, "viewer")
	corbie(t, "", "client", "trust", card, filepath.Join(dir, "lab-store"), "--config", config)
	return config
}

// started starts a UI on a config without listening, as corbie ui with
// the arguments would. A UI that is not started gives what corbie ui
// ended with and said.
func started(ctx context.Context, config, dir string, args ...string) (*ui, error) {
	var u *ui
	starting := []cli.Command{{Name: "", Usage: Commands[0].Usage, Run: func(e *cli.Env, args []string) error {
		o, err := parse(e, args)
		if err != nil {
			return err
		}
		o.dir = dir
		u, err = start(ctx, e, o)
		return err
	}}}
	status, _, said := ui0(starting, append(args, "--config", config)...)
	if status != 0 {
		return nil, fmt.Errorf("ended %d: %s", status, said)
	}
	return u, nil
}

// ui0 runs a table of one command as corbie ui, and gives its exit status,
// what it printed and what it said.
func ui0(commands []cli.Command, args ...string) (int, string, string) {
	var out, said bytes.Buffer
	only := commands[0]
	only.Name = ""
	status := cli.Program("ui", []cli.Command{only}, args, strings.NewReader(""), &out, &said)
	return status, out.String(), said.String()
}

// mistaken says whether a UI was refused as a mistake in how it was
// started.
func mistaken(err error) bool { return err != nil && strings.HasPrefix(err.Error(), "ended 2: ") }

const token = "0123456789abcdef0123456789abcdef"

// served starts a UI on a lab's config without listening, behind its guard.
func served(t *testing.T, parties ...string) http.Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	u, err := started(ctx, lab(t, t.TempDir(), "lab"), t.TempDir(), parties...)
	if err != nil {
		t.Fatal(err)
	}
	return guard(token, "127.0.0.1:8373", u.handler)
}

func ask(h http.Handler, method, path string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Host = "127.0.0.1:8373"
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func Test_serve_request_without_the_token_is_refused(t *testing.T) {
	h := served(t, "client", "controller")
	for _, r := range [][2]string{{"GET", "/runs"}, {"GET", "/runs/stream"}, {"GET", "/metrics"}, {"GET", "/assets/ui.css"},
		{"POST", "/run/lab-client/20261015T091204.118Z-3be1c07a2f/cancel"}, {"POST", "/runs/at/cancel"}} {
		if w := ask(h, r[0], r[1]); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without the token answered %d", r[0], r[1], w.Code)
		}
		if w := ask(h, r[0], r[1], "Cookie", "corbie-ui-8373=not-the-token"); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a wrong cookie answered %d", r[0], r[1], w.Code)
		}
	}
	if w := ask(h, "GET", "/metrics", "Authorization", "Bearer "+token); w.Code != http.StatusOK {
		t.Errorf("a scrape with the token answered %d", w.Code)
	}
}

func Test_serve_token_in_the_address_becomes_a_cookie(t *testing.T) {
	h := served(t, "none")
	w := ask(h, "GET", "/runs?token="+token+"&window=1h")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/runs?window=1h" {
		t.Fatalf("an address with the token answered %d to %q", w.Code, w.Header().Get("Location"))
	}
	cookie := w.Header().Get("Set-Cookie")
	for _, want := range []string{"corbie-ui-8373=" + token, "Path=/", "HttpOnly", "SameSite=Strict"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("the cookie %q lacks %s", cookie, want)
		}
	}
	if w := ask(h, "GET", "/runs", "Cookie", "corbie-ui-8373="+token); w.Code != http.StatusOK {
		t.Errorf("a page with the cookie answered %d", w.Code)
	}
	if w := ask(h, "GET", "/runs?token=wrong"); w.Code != http.StatusUnauthorized || w.Header().Get("Set-Cookie") != "" {
		t.Errorf("a wrong token in the address answered %d and set %q", w.Code, w.Header().Get("Set-Cookie"))
	}
}

func Test_serve_request_from_another_origin_is_refused(t *testing.T) {
	h := served(t, "client")
	cookie := "corbie-ui-8373=" + token
	for _, origin := range []string{"http://127.0.0.1:9000", "http://localhost:8373", "https://example.com"} {
		if w := ask(h, "GET", "/runs", "Cookie", cookie, "Origin", origin); w.Code != http.StatusForbidden {
			t.Errorf("a page from %s answered %d", origin, w.Code)
		}
		for _, action := range []string{"/run/lab-client/20261015T091204.118Z-3be1c07a2f/cancel", "/runs/at/cancel"} {
			if w := ask(h, "POST", action, "Cookie", cookie, "Origin", origin); w.Code != http.StatusForbidden {
				t.Errorf("the action %s from %s answered %d", action, origin, w.Code)
			}
		}
	}
	if w := ask(h, "GET", "/runs", "Cookie", cookie, "Origin", "http://127.0.0.1:8373"); w.Code != http.StatusOK {
		t.Errorf("a page of the UI's own origin answered %d", w.Code)
	}
}

func Test_serve_party_listens_on_this_computer_only(t *testing.T) {
	dir := t.TempDir()
	config := lab(t, dir, "lab")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, args := range [][]string{{"client", "--listen", ":8373"}, {"controller", "--listen", "0.0.0.0:8373"}, {"client", "controller", "--listen", "192.168.1.5:8373"}, {"--listen", ":8373"}} {
		if _, err := started(ctx, config, dir, args...); !mistaken(err) || !strings.Contains(err.Error(), "this computer") || !strings.Contains(err.Error(), "none") {
			t.Errorf("corbie ui %s gave %v", strings.Join(args, " "), err)
		}
	}
	for _, args := range [][]string{{"client", "--listen", "127.0.0.1:8373"}, {"client", "--listen", "localhost:8373"}, {"controller", "--listen", "[::1]:8373"}, {"none", "--listen", ":8373"}} {
		if _, err := started(ctx, config, dir, args...); err != nil {
			t.Errorf("corbie ui %s was refused: %v", strings.Join(args, " "), err)
		}
	}
	if _, err := started(ctx, viewer(t, dir), dir, "none", "--listen", ":8373"); err != nil {
		t.Errorf("corbie ui none --listen :8373 on a viewer's config was refused: %v", err)
	}
	if _, err := started(ctx, viewer(t, dir), dir, "--listen", ":8373"); !mistaken(err) {
		t.Errorf("a UI that shows the directory listened on every address: %v", err)
	}
	if _, err := started(ctx, filepath.Join(dir, "no-such-config"), dir, "--listen", ":8373"); !mistaken(err) {
		t.Errorf("a UI of the directory alone listened on every address: %v", err)
	}
}

func Test_serve_client_must_trust_the_configs_controller(t *testing.T) {
	dir := t.TempDir()
	a, b := lab(t, dir, "a"), lab(t, dir, "b")
	card := regexp.MustCompile(`corbie-card:\S+`).FindString(corbie(t, "", "client", "card", "--config", a))
	enrolled := corbie(t, "", "controller", "enrol", card, "--ceiling", "all", "--config", b)
	corbie(t, "", "client", "remove", "a-controller", "--config", a)
	corbie(t, enrolled, "client", "trust", "-", "--config", a)
	for _, args := range [][]string{{"client", "controller"}, nil} {
		_, err := started(context.Background(), a, dir, args...)
		if err == nil || !strings.Contains(err.Error(), "b-controller") || !strings.Contains(err.Error(), "a-controller") || !strings.Contains(err.Error(), "name one party") {
			t.Errorf("a UI whose client trusts another controller, started with corbie ui %s, gave %v", strings.Join(args, " "), err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := started(ctx, a, dir, "client"); err != nil {
		t.Errorf("the client alone was refused: %v", err)
	}
}

func Test_serve_page_value_cannot_become_a_flag(t *testing.T) {
	config := lab(t, t.TempDir(), "lab")
	elsewhere := filepath.Join(t.TempDir(), "other")
	for _, value := range []string{"--config=" + elsewhere, "-5", "--store=/tmp/nowhere"} {
		answer := commandOf(context.Background(), options{config: config}, page.Call{Words: []string{"run", "send"}, Args: []string{"lab-client/20261015T091204.118Z-3be1c07a2f", value}})
		if answer.Status == 0 || !strings.Contains(answer.Said, "flag") {
			t.Errorf("the value %q gave %+v", value, answer)
		}
	}
	if _, err := os.Stat(elsewhere); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a command ran with the config a page named")
	}
}

// listing gives runs from a list a test sets.
type listing struct{ runs chan []*client.Run }

func (l listing) List(context.Context, client.Filter) ([]*client.Run, error) { return <-l.runs, nil }

func Test_serve_run_that_leaves_the_store_leaves_the_list(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kept := &client.Run{ID: "lab-client/20261015T091204.118Z-3be1c07a2f", Status: "done"}
	other := &client.Run{ID: "lab-client/20261015T091300.000Z-00000000aa", Status: "running"}
	l, from := newList(), listing{runs: make(chan []*client.Run)}
	go l.follow(ctx, from, 10*time.Millisecond)
	from.runs <- []*client.Run{kept, other}
	from.runs <- []*client.Run{other}
	from.runs <- []*client.Run{other}
	runs, err := l.Runs()
	if err != nil || len(runs) != 1 || runs[0].ID != other.ID {
		t.Errorf("after the run left the store, the list holds %v, %v", runs, err)
	}
}

// corbie ui answers help and mistakes as Corbie's commands do.
func Test_serve_keeps_corbies_command_line(t *testing.T) {
	line := "corbie ui " + Commands[0].Usage
	for _, args := range [][]string{{"--help"}, {"-h"}, {"client", "-h"}} {
		if status, out, said := ui0(Commands, args...); status != 0 || !strings.HasPrefix(out, line+"\n\nserve the UI") || said != "" {
			t.Errorf("corbie ui %s ended %d, printed %q and said %q", strings.Join(args, " "), status, out, said)
		}
	}
	for args, want := range map[string]string{
		"--json":         "corbie ui: it prints no JSON",
		"help":           `corbie ui: "help" is not a party the UI acts as: the parties are client and controller, or none, each named once`,
		"frob":           `corbie ui: "frob" is not a party the UI acts as: the parties are client and controller, or none, each named once`,
		"client --bogus": "corbie ui: flag provided but not defined: -bogus",
	} {
		if status, out, said := ui0(Commands, strings.Fields(args)...); status != 2 || out != "" || said != want+"\n\nusage: "+line+"\n" {
			t.Errorf("corbie ui %s ended %d, printed %q and said %q", args, status, out, said)
		}
	}
}

func Test_serve_acts_as_the_parties_named(t *testing.T) {
	dir := t.TempDir()
	config := lab(t, dir, "lab")
	for _, c := range []struct {
		config string
		args   []string
		names  string
	}{
		{config, nil, "lab-client lab-controller"},
		{config, []string{"client"}, "lab-client"},
		{config, []string{"controller"}, "lab-controller"},
		{config, []string{"client", "controller"}, "lab-client lab-controller"},
		{config, []string{"none"}, ""},
		{viewer(t, dir), nil, ""},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		u, err := started(ctx, c.config, t.TempDir(), c.args...)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(u.parties.names, " "); got != c.names {
			t.Errorf("corbie ui %s on the config %s acts as %q", strings.Join(c.args, " "), filepath.Base(c.config), got)
		}
		if (u.parties.client != nil) != strings.Contains(c.names, "lab-client") || (u.parties.controller != nil) != strings.Contains(c.names, "lab-controller") {
			t.Errorf("corbie ui %s on the config %s holds %+v", strings.Join(c.args, " "), filepath.Base(c.config), u.parties)
		}
		cancel()
	}
	for _, args := range [][]string{{"none", "client"}, {"controller", "none"}} {
		if status, _, said := ui0(Commands, args...); status != 2 {
			t.Errorf("corbie ui %s ended %d and said %q", strings.Join(args, " "), status, said)
		}
	}
}

func Test_serve_each_party_opens_its_pages(t *testing.T) {
	config := lab(t, t.TempDir(), "lab")
	for _, c := range []struct {
		args               []string
		open, shut, header []string
	}{
		{[]string{"none"}, []string{"/runs", "/metrics"}, []string{"/machines", "/toolkits", "/controller"}, nil},
		{[]string{"client"}, []string{"/runs", "/metrics", "/machines", "/toolkits"}, []string{"/controller"}, []string{"/machines Machines", "/toolkits Toolkits"}},
		{[]string{"controller"}, []string{"/runs", "/metrics", "/machines", "/toolkits", "/controller"}, nil, []string{"/machines Machines", "/toolkits Toolkits", "/controller Controller"}},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		u, err := started(ctx, config, t.TempDir(), c.args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range c.open {
			if w := ask(u.handler, "GET", path); w.Code != http.StatusOK {
				t.Errorf("as %s, %s answered %d", strings.Join(c.args, " "), path, w.Code)
			}
		}
		page := ask(u.handler, "GET", "/runs").Body.String()
		// The header offers each page after Runs by its word.
		var offered []string
		if nav := regexp.MustCompile(`<nav><a href="/runs"[^>]*>Runs</a>(.*?)<a href="/manual/"`).FindStringSubmatch(page); nav != nil {
			for _, m := range regexp.MustCompile(`<a href="(/[a-z]+)">([A-Za-z]+)</a>`).FindAllStringSubmatch(nav[1], -1) {
				offered = append(offered, m[1]+" "+m[2])
			}
		}
		if want := c.header; !slices.Equal(offered, want) {
			t.Errorf("as %s, the header offers %q", strings.Join(c.args, " "), offered)
		}
		for _, path := range c.shut {
			if w := ask(u.handler, "GET", path); w.Code != http.StatusNotFound {
				t.Errorf("as %s, %s answered %d", strings.Join(c.args, " "), path, w.Code)
			}
			if strings.Contains(page, `href="`+path+`"`) {
				t.Errorf("as %s, the header offers %s", strings.Join(c.args, " "), path)
			}
		}
		cancel()
	}
}

func Test_serve_directory_alone_needs_no_store(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("# a comment\nflow: a\nstages:\n  one: { job: ./a.js }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.js"), []byte("return 1"), 0o644)
	config := filepath.Join(t.TempDir(), "no-such-config")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u, err := started(ctx, config, dir)
	if err != nil {
		t.Fatalf("corbie ui with no store was refused: %v", err)
	}
	if u.noStore == nil || !strings.Contains(u.noStore.Error(), "knows no controller") {
		t.Errorf("the UI does not say why it has no store: %v", u.noStore)
	}
	h := guard(token, "127.0.0.1:8373", u.handler)
	cookie := []string{"Cookie", "corbie-ui-8373=" + token}
	if w := ask(h, "GET", "/machines"); w.Code != http.StatusUnauthorized {
		t.Errorf("the directory was shown without the token: %d", w.Code)
	}
	w := ask(h, "GET", "/machines", cookie...)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `"Name":"one"`) {
		t.Errorf("Machines answered %d:\n%s", w.Code, body)
	}
	if !strings.Contains(body, "no store") || strings.Contains(body, `href="/runs"`) || strings.Contains(body, `href="/controller"`) {
		t.Errorf("the header offers what is not there:\n%s", body)
	}
	if w := ask(h, "GET", "/", cookie...); w.Header().Get("Location") != "/machines" {
		t.Errorf("/ goes to %q", w.Header().Get("Location"))
	}
	for _, path := range []string{"/runs", "/metrics", "/controller"} {
		if w := ask(h, "GET", path, cookie...); w.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, w.Code)
		}
	}
	for _, named := range []string{"none", "client", "controller"} {
		if _, err := started(ctx, config, dir, named); err == nil {
			t.Errorf("corbie ui %s started with no store", named)
		}
	}
	u, err = started(ctx, viewer(t, labbed(t)), dir)
	if err != nil {
		t.Fatal(err)
	}
	broken := labbed(t)
	entries, _ := os.ReadDir(filepath.Join(broken, "lab", "client"))
	for _, entry := range entries {
		os.WriteFile(filepath.Join(broken, "lab", "client", entry.Name()), []byte("not a key"), 0o600)
	}
	if len(entries) == 0 {
		t.Fatalf("the config holds no client's files to break")
	}
	if _, err := started(ctx, filepath.Join(broken, "lab"), dir); err == nil {
		t.Errorf("a client's key that cannot be read left the directory alone")
	}
	for _, path := range []string{"/runs", "/machines"} {
		if w := ask(u.handler, "GET", path); w.Code != http.StatusOK {
			t.Errorf("a viewer's config with a store: %s answered %d", path, w.Code)
		}
	}
}

// labbed sets a lab up, and gives the directory it is in.
func labbed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	lab(t, dir, "lab")
	return dir
}

func Test_page_loads_nothing_from_another_site(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "two.yaml"), []byte("flow: two\nstages:\n  a: { job: ./a.js }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.js"), []byte("return 1"), 0o644)
	u, err := started(ctx, lab(t, t.TempDir(), "lab"), dir, "client", "controller")
	if err != nil {
		t.Fatal(err)
	}
	far := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)
	for _, path := range []string{"/runs", "/machines", "/toolkits", "/controller", "/assets/ui.css", "/assets/ui.js", "/runs/script.js", "/machines/script.js", "/toolkits/script.js", "/controller/script.js", "/drawing/drawing.js", "/drawing/layout.js"} {
		w := ask(u.handler, "GET", path)
		if w.Code != http.StatusOK {
			t.Errorf("%s answered %d", path, w.Code)
		}
		if found := far.FindString(w.Body.String()); found != "" {
			t.Errorf("%s loads %s from another site", path, found)
		}
	}
	// Solid's files name sites in their messages, and load only one another.
	imports := regexp.MustCompile(`(?:from|import)\s*\(?\s*["']([^"']+)["']`)
	for _, path := range solidFiles(t) {
		w := ask(u.handler, "GET", path)
		if w.Code != http.StatusOK {
			t.Errorf("%s answered %d", path, w.Code)
		}
		for _, m := range imports.FindAllStringSubmatch(w.Body.String(), -1) {
			if !strings.HasPrefix(m[1], "./") && !strings.HasPrefix(m[1], "../") && m[1] != "solid-js" && m[1] != "solid-js/internal" && !strings.HasPrefix(m[1], "@solidjs/") {
				t.Errorf("%s loads %s", path, m[1])
			}
		}
	}
}

// solidFiles gives the address of every file of Solid the UI serves.
func solidFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir("../page/assets/solid", func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".js") {
			paths = append(paths, "/assets/solid/"+filepath.ToSlash(strings.TrimPrefix(p, "../page/assets/solid/")))
		}
		return err
	})
	if err != nil || len(paths) == 0 {
		t.Fatalf("Solid's files are not found: %v", err)
	}
	return paths
}
