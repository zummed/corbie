package controller

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/cli"
	corbiectl "github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	_ "github.com/zummed/corbie/machine/statechart"
	"github.com/zummed/corbie/store"
	"github.com/zummed/corbie/ui/page"
)

func corbie(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	var out, said bytes.Buffer
	if status := cli.Run(context.Background(), args, strings.NewReader(stdin), &out, &said); status != 0 {
		t.Fatalf("corbie %s: %s%s", strings.Join(args, " "), said.String(), out.String())
	}
	return out.String()
}

// lab sets one computer up as labbed does, and gives the Controller page
// on it, which runs its commands with command.
func lab(t *testing.T, command page.Command) http.Handler {
	t.Helper()
	_, ctl, s := labbed(t)
	return New(Config{Controller: ctl, Store: s, Command: command, Interval: time.Second, Changed: (&signal{}).next, Shell: page.Shell{}})
}

// A signal is a channel closed at each poke, and made again.
type signal struct {
	mu      sync.Mutex
	changed chan struct{}
}

func (g *signal) next() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.changed == nil {
		g.changed = make(chan struct{})
	}
	return g.changed
}

func (g *signal) poke() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.changed != nil {
		close(g.changed)
	}
	g.changed = make(chan struct{})
}

// labbed sets one computer up, enrols a runner with tags and removes it,
// keeps a secret at / and a value at /home, and records a tree at /home and
// a one-file tree at /hello; it gives its config directory, its controller
// and its store.
func labbed(t *testing.T) (string, *identity.Identity, store.Store) {
	t.Helper()
	dir := t.TempDir()
	config, at := filepath.Join(dir, "config"), filepath.Join(dir, "store")
	corbie(t, "", "init", "--store", at, "--name", "lab", "--ceiling", "network,disk", "--config", config)
	corbie(t, "", "controller", "enrol", "lab-client", "--ceiling", "network,disk,at:/home,at:/hello", "--config", config)
	card := regexp.MustCompile(`corbie-card:\S+`).FindString(corbie(t, "", "runner", "card", "--name", "gpu-box", "--config", filepath.Join(dir, "other")))
	corbie(t, "", "controller", "enrol", card, "--tags", "gpu,big", "--at", "/home", "--at", "/hello", "--config", config)
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
	return config, ctl, s
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

// shown is the page's parts, as the page is given them to draw.
type shown struct {
	Parties  []party   `json:"parties"`
	Trees    []Tree    `json:"trees"`
	Settings []setting `json:"settings"`
}

// partsOf reads the parts a page carries.
func partsOf(t *testing.T, body string) shown {
	t.Helper()
	m := regexp.MustCompile(`(?s)<script type="application/json" id="parts">(.*?)</script>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the page carries no parts:\n%s", body)
	}
	var s shown
	if err := json.Unmarshal([]byte(m[1]), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func partyOf(t *testing.T, s shown, name string) party {
	t.Helper()
	i := slices.IndexFunc(s.Parties, func(p party) bool { return p.Name == name })
	if i < 0 {
		t.Fatalf("the page lists no party %s: %+v", name, s.Parties)
	}
	return s.Parties[i]
}

func post(h http.Handler, act string, form url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/controller/"+act, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(w, r)
	return w
}

func Test_controller_lists_parties_trees_secrets_and_values(t *testing.T) {
	s := partsOf(t, get(t, lab(t, nil)))
	if p := partyOf(t, s, "lab-runner"); p.Kind != "runner" || fmt.Sprint(p.At) != "[/]" {
		t.Errorf("the page does not list at / a runner enrolled with no --at: %+v", p)
	}
	if p := partyOf(t, s, "gpu-box"); fmt.Sprint(p.At, p.Tags) != "[/home /hello] [gpu big]" {
		t.Errorf("the page lists the runner gpu-box as %+v", p)
	}
	if p := partyOf(t, s, "old-box"); !p.Removed {
		t.Errorf("the page does not list old-box as removed: %+v", p)
	}
	if p := partyOf(t, s, "lab-client"); p.Kind != "client" || fmt.Sprint(p.Ceiling) != "[network disk at:/home at:/hello]" {
		t.Errorf("the page lists the client lab-client as %+v", p)
	}
	if !regexp.MustCompile(`^[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}$`).MatchString(partyOf(t, s, "lab-client").Fingerprint) {
		t.Errorf("the page does not show a card's fingerprint")
	}
	trees := map[string]Tree{}
	for _, tr := range s.Trees {
		trees[tr.Prefix] = tr
	}
	home := trees["/home"]
	if home.Client != "lab-client" || home.Directory == "" {
		t.Errorf("the tree at /home is listed as %+v", home)
	}
	for _, want := range []Location{{"/home/devices/boiler", "devices/boiler", "singleton"}, {"/home/rooms/thermostat", "rooms/thermostat", "keyed"}, {"/home/tools/lookup", "tools/lookup", "transient"}} {
		if !slices.Contains(home.Locations, want) {
			t.Errorf("the tree at /home does not list %+v: %+v", want, home.Locations)
		}
	}
	if hello := trees["/hello"]; len(hello.Locations) != 1 || hello.Locations[0].At != "/hello" || hello.Locations[0].Name != "hello" {
		t.Errorf("the tree at /hello is listed as %+v", hello)
	}
	want := []setting{{At: "/", Name: "ANTHROPIC_KEY", Kind: "secret"}, {At: "/home", Name: "HUB", Kind: "value", Value: "192.0.2.8:8123"}}
	if !slices.Equal(s.Settings, want) {
		t.Errorf("the secrets and values are listed as %+v, not by location as %+v", s.Settings, want)
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
	w := post(h, "enrol", url.Values{"card": {" corbie-card:runner:new-box:abc "}, "tags": {"gpu, big"}, "prefixes": {"/home, /netops/tools"}, "replace": {"1"}})
	if !strings.Contains(w.Body.String(), "corbie-card:controller:lab-controller:x\n/srv/store") {
		t.Errorf("Enrol did not show the controller's card and the store's address: %q", w.Body.String())
	}
	post(h, "enrol", url.Values{"card": {"corbie-card:client:new-client:abc"}, "ceiling": {"network,disk"}})
	post(h, "tags", url.Values{"name": {"gpu-box"}, "tags": {"gpu"}})
	post(h, "prefixes", url.Values{"name": {"gpu-box"}, "prefixes": {"/home /hello"}})
	post(h, "ceiling", url.Values{"name": {"lab-client"}, "ceiling": {"all"}})
	post(h, "remove", url.Values{"name": {"gpu-box"}})
	post(h, "secret-remove", url.Values{"name": {"ANTHROPIC_KEY"}, "at": {"/"}})
	post(h, "value-set", url.Values{"name": {"HUB"}, "value": {"192.0.2.9:8123"}, "at": {"/home"}})
	post(h, "value-set", url.Values{"name": {"REGION"}, "value": {"eu-west-2"}})
	post(h, "value-remove", url.Values{"name": {"HUB"}, "at": {" /home "}})
	want := []string{
		"corbie controller enrol corbie-card:runner:new-box:abc --tags=gpu,big --at=/home --at=/netops/tools --replace",
		"corbie controller enrol corbie-card:client:new-client:abc --ceiling=network,disk",
		"corbie controller enrol gpu-box --tags=gpu",
		"corbie controller enrol gpu-box --at=/home --at=/hello",
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
	s := partsOf(t, get(t, h))
	if p := partyOf(t, s, "old-box"); fmt.Sprint(p.Acts) != "[enrol]" {
		t.Errorf("the removed party's line offers %v, not Enrol alone", p.Acts)
	}
	if p := partyOf(t, s, "gpu-box"); fmt.Sprint(p.Acts) != "[tags prefixes remove]" {
		t.Errorf("a runner's line offers %v", p.Acts)
	}
	if p := partyOf(t, s, "lab-client"); fmt.Sprint(p.Acts) != "[ceiling remove]" {
		t.Errorf("a client's line offers %v", p.Acts)
	}
	if w := post(h, "tags", url.Values{"name": {"old-box"}, "tags": {"gpu"}}); w.Code != http.StatusConflict || len(r.calls) != 0 {
		t.Errorf("setting a removed party's tags answered %d and ran %v", w.Code, r.calls)
	}
	if w := post(h, "prefixes", url.Values{"name": {"old-box"}, "prefixes": {"/home"}}); w.Code != http.StatusConflict || len(r.calls) != 0 {
		t.Errorf("setting a removed party's prefixes answered %d and ran %v", w.Code, r.calls)
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

func Test_controller_check_gives_each_part_of_the_setup(t *testing.T) {
	var ran []string
	answer := page.Answer{}
	h := lab(t, func(ctx context.Context, c page.Call) page.Answer { ran = append(ran, c.String()); return answer })
	printed, err := json.Marshal(cli.Checked{Legs: []cli.Leg{
		{Leg: "start", Status: "ok", Seconds: 0.42},
		{Leg: "job", Status: "never", Seconds: 90, Why: "no runner took the job: is a runner running?"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	answer = page.Answer{Status: 1, Out: string(printed)}
	w := post(h, "check", url.Values{})
	body := w.Body.String()
	if len(ran) != 1 || ran[0] != "corbie check" {
		t.Fatalf("Check ran %v", ran)
	}
	if w.Code != http.StatusConflict || body != "the controller took the start    ok     0.4s\na runner took the job            never  \n  no runner took the job: is a runner running?" {
		t.Errorf("a check that failed answered %d:\n%s", w.Code, body)
	}
	printed, _ = json.Marshal(cli.Checked{OK: true, Legs: []cli.Leg{{Leg: "start", Status: "ok", Seconds: 0.1}}})
	answer = page.Answer{Out: string(printed)}
	if w := post(h, "check", url.Values{}); w.Code != http.StatusOK || !strings.HasPrefix(w.Body.String(), "the controller took the start") {
		t.Errorf("a check that passed answered %d:\n%s", w.Code, w.Body.String())
	}
	// A check that could not start says why, as any action does.
	answer = page.Answer{Status: 1, Said: "this config knows no client"}
	if w := post(h, "check", url.Values{}); w.Code != http.StatusConflict || strings.TrimSpace(w.Body.String()) != "this config knows no client" {
		t.Errorf("a check that could not start answered %d:\n%s", w.Code, w.Body.String())
	}
}

// Test_controller_check_runs_corbie_check runs Check on a lab where no
// controller process runs, as the UI runs it: the start is never taken.
func Test_controller_check_runs_corbie_check(t *testing.T) {
	config, ctl, s := labbed(t)
	command := func(ctx context.Context, c page.Call) page.Answer {
		args := append(append(append(append([]string{}, c.Words...), c.Args...), c.Flags...), "--json", "--config", config, "--wait", "1s")
		var out, said bytes.Buffer
		status := cli.Run(ctx, args, strings.NewReader(c.Stdin), &out, &said)
		return page.Answer{Status: status, Out: out.String(), Said: said.String()}
	}
	h := New(Config{Controller: ctl, Store: s, Command: command, Interval: time.Second, Changed: (&signal{}).next})
	w := post(h, "check", url.Values{})
	if w.Code != http.StatusConflict || !regexp.MustCompile(`^the controller took the start\s+never`).MatchString(w.Body.String()) {
		t.Errorf("Check on a lab with no controller running answered %d:\n%s", w.Code, w.Body.String())
	}
}
