package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/cli"
	corbiectl "github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	_ "github.com/zummed/corbie/machine/statechart"
	"github.com/zummed/corbie/store"
)

func corbie(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	var out, said bytes.Buffer
	if status := cli.Run(context.Background(), args, strings.NewReader(stdin), &out, &said); status != 0 {
		t.Fatalf("corbie %s: %s%s", strings.Join(args, " "), said.String(), out.String())
	}
	return out.String()
}

// lab sets one computer up, enrols a runner with tags and removes it, keeps
// a secret at / and a value at /home, and records a tree at /home and a
// one-file tree at /hello; it gives the Controller page on it.
func lab(t *testing.T, command page.Command) http.Handler {
	t.Helper()
	dir := t.TempDir()
	config, at := filepath.Join(dir, "config"), filepath.Join(dir, "store")
	corbie(t, "", "init", "--store", at, "--name", "lab", "--ceiling", "network,disk", "--config", config)
	corbie(t, "", "controller", "enrol", "lab-client", "--ceiling", "network,disk,at:/home,at:/hello", "--config", config)
	card := regexp.MustCompile(`corbie-card:\S+`).FindString(corbie(t, "", "runner", "card", "--name", "gpu-box", "--config", filepath.Join(dir, "other")))
	corbie(t, "", "controller", "enrol", card, "--tags", "gpu,big", "--config", config)
	corbie(t, "", "runner", "card", "--name", "old-box", "--config", filepath.Join(dir, "old"))
	old := regexp.MustCompile(`corbie-card:\S+`).FindString(corbie(t, "", "runner", "card", "--config", filepath.Join(dir, "old")))
	corbie(t, "", "controller", "enrol", old, "--config", config)
	corbie(t, "", "controller", "remove", "old-box", "--config", config)
	corbie(t, "swordfish", "controller", "secret", "set", "ANTHROPIC_KEY", "-", "--config", config)
	corbie(t, "", "controller", "value", "set", "HUB", "192.0.2.8:8123", "--at", "/home", "--config", config)
	ctl, err := identity.Config{Dir: config}.Load(t.Context(), identity.Controller)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(at)
	if err != nil {
		t.Fatal(err)
	}
	house := write(t, map[string]string{
		"devices/boiler.yaml":   "statechart: boiler\nlifetime: singleton\ninitial: open\nstates:\n  open: { on: { stop: closed } }\n  closed: { end: done }\n",
		"rooms/thermostat.yaml": "statechart: thermostat\nlifetime: keyed\ninitial: open\nstates:\n  open: { on: { stop: closed } }\n  closed: { end: done }\n",
		"tools/lookup.yaml":     "statechart: lookup\ninitial: open\nstates:\n  open: { on: { stop: closed } }\n  closed: { end: done }\n",
	})
	tree, err := client.ReadTree(house, "/home")
	if err != nil {
		t.Fatal(err)
	}
	submit(t, config, s, ctl, tree)
	// One file is a tree of its own, as `corbie run submit hello.yaml` reads it.
	house = write(t, map[string]string{"hello.yaml": "statechart: hello\ninitial: open\nstates:\n  open: { on: { stop: closed } }\n  closed: { end: done }\n"})
	one, err := client.ReadFile(house, "hello.yaml", "/hello", client.ScriptJob{})
	if err != nil {
		t.Fatal(err)
	}
	submit(t, config, s, ctl, one)
	return New(Config{Controller: ctl, Store: s, Command: command, Interval: time.Second, Shell: page.Shell{}})
}

// write writes files into a new directory, and gives the directory.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "house")
	for name, text := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// submit records a tree, as `corbie run submit` and one pass of the
// controller do.
func submit(t *testing.T, config string, s store.Store, ctl *identity.Identity, tree *client.Tree) {
	t.Helper()
	ctx := context.Background()
	me, err := identity.Config{Dir: config}.Load(t.Context(), identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(me, ctl.Card, s)
	message, err := c.Submit(ctx, tree, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := corbiectl.New(ctl, s).Pass(ctx); err != nil {
		t.Fatal(err)
	}
	if receipt, err := c.WaitReceipt(ctx, message, time.Minute, 10*time.Millisecond); err != nil || !receipt.Taken {
		t.Fatalf("the submit answered %+v, %v", receipt, err)
	}
}

func get(t *testing.T, h http.Handler) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/controller", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("the page answered %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func post(h http.Handler, act string, form url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/controller/"+act, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(w, r)
	return w
}

func Test_controller_lists_parties_trees_secrets_and_values(t *testing.T) {
	body := get(t, lab(t, nil))
	for _, want := range []string{"lab-runner", "lab-client", "network, disk, at:/home", "gpu-box", "gpu, big", `<tr data-key="old-box" class="removed">`,
		`<a href="/runs?at=%2Fhome">/home</a></td><td class="dim cut" title="the tree of lab-client, last submitted from `, ">tree of lab-client, from ",
		`<td class="in" title="/home/devices/boiler"><a href="/runs?at=%2Fhome%2Fdevices%2Fboiler">devices/boiler</a></td><td>singleton</td>`,
		`>rooms/thermostat</a></td><td>keyed</td>`, `>tools/lookup</a></td><td>transient</td>`,
		`<td class="in" title="/hello"><a href="/runs?at=%2Fhello">hello</a></td>`,
		`<td>/</td><td>ANTHROPIC_KEY</td><td class="dim">secret</td><td class="cut" title=""></td>`,
		`<td>/home</td><td>HUB</td><td class="dim">value</td><td class="cut" title="192.0.2.8:8123">192.0.2.8:8123</td>`,
		`<td class="cut" title="network, disk, at:/home, at:/hello">`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not show %s", want)
		}
	}
	if strings.Index(body, "ANTHROPIC_KEY") > strings.Index(body, ">HUB<") {
		t.Errorf("the secrets and values are not listed by location")
	}
	if !regexp.MustCompile(`<td class="dim">[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}</td>`).MatchString(body) {
		t.Errorf("the page does not show a card's fingerprint")
	}
}

// recorder runs no command, and keeps every call it is given.
type recorder struct{ calls []page.Call }

func (r *recorder) run(_ context.Context, c page.Call) page.Answer {
	r.calls = append(r.calls, c)
	return page.Answer{Out: `{"card":"corbie-card:controller:lab-controller:x","store":"/srv/store"}`, Said: "enrolled the runner gpu-box\n"}
}

func Test_controller_actions_run_their_commands(t *testing.T) {
	r := &recorder{}
	h := lab(t, r.run)
	w := post(h, "enrol", url.Values{"card": {" corbie-card:runner:new-box:abc "}, "tags": {"gpu, big"}, "replace": {"1"}})
	if !strings.Contains(w.Body.String(), "corbie-card:controller:lab-controller:x\n/srv/store") {
		t.Errorf("Enrol did not show the controller's card and the store's address: %q", w.Body.String())
	}
	post(h, "enrol", url.Values{"card": {"corbie-card:client:new-client:abc"}, "ceiling": {"network,disk"}})
	post(h, "tags", url.Values{"name": {"gpu-box"}, "tags": {"gpu"}})
	post(h, "ceiling", url.Values{"name": {"lab-client"}, "ceiling": {"all"}})
	post(h, "remove", url.Values{"name": {"gpu-box"}})
	post(h, "secret-remove", url.Values{"name": {"ANTHROPIC_KEY"}, "at": {"/"}})
	post(h, "value-set", url.Values{"name": {"HUB"}, "value": {"192.0.2.9:8123"}, "at": {"/home"}})
	post(h, "value-set", url.Values{"name": {"REGION"}, "value": {"eu-west-2"}})
	post(h, "value-remove", url.Values{"name": {"HUB"}, "at": {" /home "}})
	want := []string{
		"corbie controller enrol corbie-card:runner:new-box:abc --tags=gpu,big --replace",
		"corbie controller enrol corbie-card:client:new-client:abc --ceiling=network,disk",
		"corbie controller enrol gpu-box --tags=gpu",
		"corbie controller enrol lab-client --ceiling=all",
		"corbie controller remove gpu-box",
		"corbie controller secret remove ANTHROPIC_KEY --at=/",
		"corbie controller value set HUB 192.0.2.9:8123 --at=/home",
		"corbie controller value set REGION eu-west-2 --at=/",
		"corbie controller value remove HUB --at=/home",
	}
	if len(r.calls) != len(want) {
		t.Fatalf("the actions ran %v", r.calls)
	}
	for i, c := range r.calls {
		if c.String() != want[i] {
			t.Errorf("an action ran %q, not %q", c.String(), want[i])
		}
		if found, _ := cli.Find(c.Words); found == nil || found.Name != strings.Join(c.Words, " ") {
			t.Errorf("%q is not one of Corbie's commands", strings.Join(c.Words, " "))
		}
	}
}

func Test_controller_removed_party_offers_only_enrol(t *testing.T) {
	r := &recorder{}
	h := lab(t, r.run)
	body := get(t, h)
	row := regexp.MustCompile(`(?s)<tr data-key="old-box" class="removed">.*?</tr>`).FindString(body)
	if row == "" || strings.Contains(row, "/controller/tags") || strings.Contains(row, "/controller/remove") || !strings.Contains(row, `href="#enrol-runner"`) {
		t.Errorf("the removed party's line offers more than Enrol:\n%s", row)
	}
	if w := post(h, "tags", url.Values{"name": {"old-box"}, "tags": {"gpu"}}); w.Code != http.StatusConflict || len(r.calls) != 0 {
		t.Errorf("setting a removed party's tags answered %d and ran %v", w.Code, r.calls)
	}
}

func Test_controller_secret_value_goes_on_standard_input_only(t *testing.T) {
	r := &recorder{}
	h := lab(t, r.run)
	w := post(h, "secret-set", url.Values{"name": {"GITHUB_TOKEN"}, "value": {"hunter2"}, "at": {"/home"}})
	if len(r.calls) != 1 {
		t.Fatalf("Set ran %v", r.calls)
	}
	c := r.calls[0]
	if c.Stdin != "hunter2" || strings.Contains(c.String(), "hunter2") || c.String() != "corbie controller secret set GITHUB_TOKEN - --at=/home" {
		t.Errorf("Set ran %q with %q on standard input", c.String(), c.Stdin)
	}
	if strings.Contains(w.Body.String(), "hunter2") || strings.Contains(get(t, h), "swordfish") {
		t.Errorf("a secret's value was sent back to the page")
	}
}
