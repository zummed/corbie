// Package controller is the Controller page: the runners and clients the
// controller trusts, its trees, and the secrets and values set at each
// location, read through Corbie's packages and changed with its corbie
// controller commands.
package controller

import (
	"cmp"
	"context"
	"embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
	"github.com/zummed/corbie/ui/page"
)

// Config is what the Controller page is given: the controller, its store,
// how a command runs, how often the store is read, and the shell every
// page shares.
type Config struct {
	Controller *identity.Identity
	Store      store.Store
	Command    page.Command
	Interval   time.Duration
	// Changed gives a channel closed at the next change the UI makes or
	// reads in the store, so the page is sent it at once.
	Changed func() <-chan struct{}
	Shell   page.Shell
}

type handler struct{ Config }

//go:embed controller.js
var files embed.FS

// A party is one runner or client the controller trusts, as `corbie
// controller parties` lists it.
type party struct {
	Name, Kind, Fingerprint string
	Tags, Ceiling, Deny     []string
	// At holds a runner's prefixes, "/" where it is sent every tree's jobs.
	At      []string
	Removed bool
	// Acts are the actions its line offers: a runner's tags, prefixes and
	// remove, a client's ceiling and remove, and for a removed party enrol
	// alone.
	Acts []string
}

// A Tree is one the controller recorded at its prefix: its client, the
// directory last submitted there, and its locations.
type Tree struct {
	Prefix, Client, Directory string
	Locations                 []Location
}

// A Location is one of a tree's, named from the tree's prefix.
type Location struct {
	At, Name, Lifetime string
}

// A setting is a secret or a value set at a location. A secret's Value is
// never given.
type setting struct {
	At, Name, Kind, Value string
}

type view struct {
	Parties []party
	Trees   []Tree
	Set     []setting
}

// recorded is what the controller keeps of a tree at its prefix, sealed to
// itself.
type recorded struct {
	Client    string `json:"client"`
	Directory string `json:"directory"`
	Files     map[string]struct {
		Lifetime string `json:"lifetime"`
	} `json:"files"`
}

// New gives the Controller page.
func New(c Config) http.Handler {
	h := &handler{c}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /controller", h.show)
	mux.HandleFunc("GET /controller/stream", h.stream)
	mux.HandleFunc("GET /controller/script.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		data, _ := files.ReadFile("controller.js")
		w.Write(data)
	})
	mux.HandleFunc("POST /controller/{act}", h.act)
	return mux
}

func (h *handler) read(ctx context.Context) (view, error) {
	var v view
	trusted, err := identity.ListTrust(ctx, h.Store, h.Controller.Card)
	if err != nil {
		return v, err
	}
	for _, t := range trusted {
		p := party{Name: t.Card.Name, Kind: string(t.Card.Kind), Fingerprint: t.Card.Fingerprint(),
			Tags: t.Tags, Ceiling: t.Ceiling.Allow, Deny: t.Ceiling.Deny, Removed: t.Removed}
		p.Acts = []string{"ceiling", "remove"}
		if t.Card.Kind == identity.Runner {
			if p.At = t.Prefixes; len(p.At) == 0 {
				p.At = []string{"/"}
			}
			p.Acts = []string{"tags", "prefixes", "remove"}
		}
		if t.Removed {
			p.Acts = []string{"enrol"}
		}
		v.Parties = append(v.Parties, p)
	}
	slices.SortFunc(v.Parties, func(a, b party) int { return strings.Compare(a.Kind+a.Name, b.Kind+b.Name) })
	if v.Trees, err = h.trees(ctx); err != nil {
		return v, err
	}
	secrets, err := secret.List(ctx, h.Store, h.Controller.Name)
	if err != nil {
		return v, err
	}
	for _, e := range secrets {
		v.Set = append(v.Set, setting{At: e.At, Name: e.Name, Kind: "secret"})
	}
	values, err := secret.ListValues(ctx, h.Store, h.Controller)
	if err != nil {
		return v, err
	}
	for _, e := range values {
		v.Set = append(v.Set, setting{At: e.At, Name: e.Name, Kind: "value", Value: e.Value})
	}
	slices.SortFunc(v.Set, func(a, b setting) int {
		return cmp.Or(strings.Compare(a.At, b.At), strings.Compare(a.Name, b.Name), strings.Compare(a.Kind, b.Kind))
	})
	return v, nil
}

func (h *handler) trees(ctx context.Context) ([]Tree, error) {
	return Trees(ctx, h.Store, h.Controller)
}

// Trees reads every tree the controller recorded, by prefix.
func Trees(ctx context.Context, s store.Store, ctl *identity.Identity) ([]Tree, error) {
	names, err := store.Names(s.List(ctx, store.Locations(ctl.Name)))
	if err != nil {
		return nil, err
	}
	var trees []Tree
	for _, name := range names {
		prefix, entry, ok := store.ParseLocation(ctl.Name, name)
		if !ok || entry != "tree" {
			continue
		}
		data, mark, err := s.Read(ctx, name)
		if err != nil {
			return nil, err
		}
		if mark == store.None {
			continue
		}
		// Of what the controller keeps, the page reads only some fields.
		var raw jsontext.Value
		var r recorded
		_, err = identity.Read(data, name, ctl.Card, ctl, &raw)
		if err == nil {
			err = json.Unmarshal(raw, &r)
		}
		if err != nil {
			return nil, fmt.Errorf("the tree at %s: %w", prefix, err)
		}
		t := Tree{Prefix: prefix, Client: r.Client, Directory: r.Directory}
		for _, at := range slices.Sorted(maps.Keys(r.Files)) {
			t.Locations = append(t.Locations, Location{At: at, Name: nameIn(prefix, at), Lifetime: r.Files[at].Lifetime})
		}
		trees = append(trees, t)
	}
	slices.SortFunc(trees, func(a, b Tree) int { return strings.Compare(a.Prefix, b.Prefix) })
	return trees, nil
}

// nameIn writes a location from its tree's prefix; a location the prefix
// names in full keeps its last name.
func nameIn(prefix, at string) string {
	if at == prefix {
		return path.Base(at)
	}
	return strings.TrimPrefix(at, strings.TrimSuffix(prefix, "/")+"/")
}

// parts gives the page's parts: the runners and clients, the trees, and
// the secrets and values set.
func (h *handler) parts(ctx context.Context) (Parts, error) {
	v, err := h.read(ctx)
	if err != nil {
		return Parts{}, err
	}
	return Parts{Parties: v.Parties, Trees: v.Trees, Settings: v.Set}, nil
}

// Parts are the parts of the Controller page.
type Parts struct {
	Parties  []party   `json:"parties"`
	Trees    []Tree    `json:"trees"`
	Settings []setting `json:"settings"`
}

func (h *handler) show(w http.ResponseWriter, r *http.Request) {
	parts, err := h.parts(r.Context())
	if err != nil {
		page.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s := h.Shell
	s.Title, s.Current, s.Stream, s.Script = "Controller", "Controller", "/controller/stream", "/controller/script.js"
	page.Write(w, s, parts)
}

func (h *handler) stream(w http.ResponseWriter, r *http.Request) {
	page.Stream(w, r, h.Interval, h.Changed, func(ctx context.Context) (any, error) { return h.parts(ctx) })
}

// act runs the corbie controller command of an action, and answers with
// what it printed back, or its error.
func (h *handler) act(w http.ResponseWriter, r *http.Request) {
	list := func(name string) string {
		return strings.Join(strings.Fields(strings.ReplaceAll(r.FormValue(name), ",", " ")), ",")
	}
	// A runner's prefixes go to the command as --at, once for each.
	prefixes := func() []string {
		var flags []string
		for _, p := range strings.Fields(strings.ReplaceAll(r.FormValue("prefixes"), ",", " ")) {
			flags = append(flags, "--at="+p)
		}
		return flags
	}
	at := "--at=" + cmp.Or(strings.TrimSpace(r.FormValue("at")), "/")
	var call page.Call
	switch r.PathValue("act") {
	case "enrol":
		call = page.Call{Words: []string{"controller", "enrol"}, Args: []string{strings.TrimSpace(r.FormValue("card"))}}
		if tags := list("tags"); tags != "" {
			call.Flags = append(call.Flags, "--tags="+tags)
		}
		call.Flags = append(call.Flags, prefixes()...)
		if ceiling := list("ceiling"); ceiling != "" {
			call.Flags = append(call.Flags, "--ceiling="+ceiling)
		}
		if r.FormValue("replace") != "" {
			call.Flags = append(call.Flags, "--replace")
		}
	case "tags", "prefixes", "ceiling":
		name := r.FormValue("name")
		v, err := h.read(r.Context())
		if err != nil {
			page.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		i := slices.IndexFunc(v.Parties, func(p party) bool { return p.Name == name })
		if i < 0 || v.Parties[i].Removed {
			page.Error(w, http.StatusConflict, "the party is not trusted: enrol it again with its card")
			return
		}
		call = page.Call{Words: []string{"controller", "enrol"}, Args: []string{name}, Flags: []string{"--" + r.PathValue("act") + "=" + list(r.PathValue("act"))}}
		if r.PathValue("act") == "prefixes" {
			call.Flags = prefixes()
		}
	case "remove":
		call = page.Call{Words: []string{"controller", "remove"}, Args: []string{r.FormValue("name")}}
	case "secret-set":
		// The value goes on standard input, which "-" names to the command.
		call = page.Call{Words: []string{"controller", "secret", "set"}, Args: []string{r.FormValue("name")}, Flags: []string{"-", at}, Stdin: r.FormValue("value")}
	case "value-set":
		call = page.Call{Words: []string{"controller", "value", "set"}, Args: []string{r.FormValue("name"), r.FormValue("value")}, Flags: []string{at}}
	case "check":
		call = page.Call{Words: []string{"check"}}
	case "secret-remove", "value-remove":
		kind, _, _ := strings.Cut(r.PathValue("act"), "-")
		call = page.Call{Words: []string{"controller", kind, "remove"}, Args: []string{r.FormValue("name")}, Flags: []string{at}}
	default:
		page.Error(w, http.StatusNotFound, "there is no such action")
		return
	}
	answer := h.Command(r.Context(), call)
	if r.PathValue("act") == "check" {
		if text, ok := checked(answer); ok {
			status := http.StatusOK
			if answer.Status != 0 {
				status = http.StatusConflict
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(status)
			io.WriteString(w, text)
			return
		}
	}
	if answer.Status != 0 {
		page.Error(w, http.StatusConflict, answer.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, said(answer))
}

// legSays is what each leg of `corbie check` is, in Corbie's words for it:
// a copy of legSays in Corbie's client/cli/check.go, which Corbie keeps to
// itself.
var legSays = map[string]string{
	"start":  "the controller took the start",
	"job":    "a runner took the job",
	"result": "the controller took the result",
	"timer":  "the timer fired",
}

// checked gives what `corbie check` printed, one line for each part of the
// setup it checked: what it is, whether it responded (ok, late or never),
// how long it took, and why where it did not. It says whether the check
// printed what it checked; one that could not start printed none.
func checked(a page.Answer) (string, bool) {
	var c cli.Checked
	if json.Unmarshal([]byte(strings.TrimSpace(a.Out)), &c) != nil || len(c.Legs) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, l := range c.Legs {
		took := fmt.Sprintf("%.1fs", l.Seconds)
		if l.Status == "never" {
			took = ""
		}
		fmt.Fprintf(&b, "%-32s %-6s %s\n", cmp.Or(legSays[l.Leg], l.Leg), l.Status, took)
		if l.Why != "" {
			fmt.Fprintf(&b, "  %s\n", l.Why)
		}
	}
	return strings.TrimRight(b.String(), "\n"), true
}

// said is what an action's command printed back: for an enrol, the
// controller's card and the store's address, which the other side trusts.
func said(a page.Answer) string {
	var printed struct {
		Card  string `json:"card"`
		Store string `json:"store"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(a.Out)), &printed) == nil && printed.Card != "" {
		return strings.TrimSpace(a.Said) + "\n" + printed.Card + "\n" + printed.Store
	}
	if text := strings.TrimSpace(a.Said); text != "" {
		return text
	}
	return "done"
}
