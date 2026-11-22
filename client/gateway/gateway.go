// Package gateway is the client package over HTTP: a route for each thing
// the package sends and each thing it reads, and streams for what can be
// followed. A gateway acts as the client a caller's token names; it keeps
// nothing itself.
package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A Gateway serves its clients' runs to callers that show a token.
type Gateway struct {
	// Callers gives, by the token a caller shows, the client the request
	// acts as. The client's Person is the person the token was given to. A
	// request without one of the tokens is refused and does nothing.
	Callers map[string]*client.Client
	// Issuer is the OIDC issuer whose tokens the gateway takes too, if it
	// has one.
	Issuer *Issuer
	// Interval is how often a stream looks again at a store that cannot
	// tell it when something was written.
	Interval time.Duration
}

// start is what the part named start of a request to start a run holds,
// as JSON: the location and what the start carries. A field the gateway
// does not know is refused.
type start struct {
	At      string            `json:"at"`
	Input   any               `json:"input,omitzero"`
	Labels  map[string]string `json:"labels,omitzero"`
	Allow   []string          `json:"allow,omitzero"`
	Deny    []string          `json:"deny,omitzero"`
	Timeout machine.Duration  `json:"timeout,omitzero"`
	Life    machine.Duration  `json:"life,omitzero"`
}

// held is how much of an upload is held in memory. The rest waits in
// temporary files, which go when the request ends.
const held = 4 << 20

// A cancel, an event and an answer may each say how long the message is
// good for.
type cancel struct {
	Life machine.Duration `json:"life,omitzero"`
}

type event struct {
	Event string           `json:"event"`
	Data  any              `json:"data,omitzero"`
	Life  machine.Duration `json:"life,omitzero"`
}

type answer struct {
	Ticket string           `json:"ticket"`
	Value  any              `json:"value,omitzero"`
	Life   machine.Duration `json:"life,omitzero"`
}

func (g *Gateway) interval() time.Duration {
	if g.Interval > 0 {
		return g.Interval
	}
	return time.Second
}

// ServeHTTP answers one request.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := g.caller(r)
	if err != nil {
		fail(w, http.StatusUnauthorized, err)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	ctx := r.Context()
	query := r.URL.Query()
	if parts[0] != "runs" {
		// What a client sends that names no run of its own.
		switch route := strings.Join(parts, "/"); {
		case r.Method != http.MethodPost:
			fail(w, http.StatusNotFound, fmt.Errorf("there is no such route"))
		case !slices.Contains([]string{"trees", "secrets", "values", "events"}, route):
			fail(w, http.StatusNotFound, fmt.Errorf("there is no such route"))
		case !takes(w, query):
		case route == "trees":
			g.submit(w, r, c)
		case route == "events":
			g.sendTo(w, r, c)
		default:
			g.set(w, r, c, strings.TrimSuffix(route, "s"))
		}
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodPost:
		if takes(w, query) {
			g.start(w, r, c)
		}
	case len(parts) == 1 && r.Method == http.MethodGet:
		if !takes(w, query, "all", "status", "parent", "label", "at", "key", "watch") {
			return
		}
		filter := client.Filter{Status: query.Get("status"), Parent: query.Get("parent"), Location: query.Get("at"), Key: query.Get("key"), Finished: query.Has("all")}
		if filter.Status != "" && !slices.Contains(client.Statuses, filter.Status) {
			fail(w, http.StatusBadRequest, fmt.Errorf("status is %s", strings.Join(client.Statuses, ", ")))
			return
		}
		for _, label := range query["label"] {
			name, value, _ := strings.Cut(label, "=")
			if filter.Labels == nil {
				filter.Labels = map[string]string{}
			}
			filter.Labels[name] = value
		}
		if query.Has("watch") {
			s := newStream(w)
			err := c.Watch(ctx, filter, g.interval(), func(u client.Update) { s.send("run", u) })
			s.failed(ctx, err)
			return
		}
		runs, err := c.List(ctx, filter)
		if runs == nil {
			runs = []*client.Run{}
		}
		reply(w, runs, err)
	case len(parts) >= 3:
		id := parts[1] + "/" + parts[2]
		if store.CheckInstance(id) != nil {
			fail(w, http.StatusNotFound, client.ErrNoRun)
			return
		}
		g.run(w, r, c, id, parts[3:])
	default:
		fail(w, http.StatusNotFound, fmt.Errorf("there is no such route"))
	}
}

// caller gives the client the token of a request names, or why the request
// is refused. Every token of the gateway's is compared, each in a way that
// takes the same time whether the one shown is nearly right or wholly wrong,
// and whatever its length: what is compared is a digest of each, and two
// digests are of one length. A token that is none of them is the issuer's
// to name a client by, where the gateway has an issuer.
func (g *Gateway) caller(r *http.Request) (*client.Client, error) {
	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	shown := sha256.Sum256([]byte(token))
	var named *client.Client
	for held, c := range g.Callers {
		digest := sha256.Sum256([]byte(held))
		if subtle.ConstantTimeCompare(shown[:], digest[:]) == 1 && held != "" && bearer {
			named = c
		}
	}
	switch {
	case named != nil:
		return named, nil
	case g.Issuer == nil || !bearer || token == "":
		return nil, errors.New("the request does not show a token of the gateway's (Authorization: Bearer …)")
	}
	c, err := g.Issuer.Caller(r.Context(), token)
	if err != nil {
		return nil, fmt.Errorf("the request's token is not taken: %w", err)
	}
	return c, nil
}

// takes says whether a request's query holds only parameters its route
// takes, and refuses the request if it holds another.
func takes(w http.ResponseWriter, query url.Values, names ...string) bool {
	for _, name := range slices.Sorted(maps.Keys(query)) {
		if !slices.Contains(names, name) {
			fail(w, http.StatusBadRequest, fmt.Errorf("the request has the parameter %q, which this route does not take", name))
			return false
		}
	}
	return true
}

// A part of an upload form is a field or a file: the gateway takes
// either, as its bytes.
type part struct {
	filename string // of a file
	open     func() (io.ReadCloser, error)
}

// text gives the whole of a part.
func (p part) text() (string, error) {
	contents, err := p.open()
	if err != nil {
		return "", err
	}
	defer contents.Close()
	data, err := io.ReadAll(contents)
	return string(data), err
}

// partsOf gives the parts of a form by their names. A name has one part.
func partsOf(form *multipart.Form) (map[string]part, error) {
	parts := map[string]part{}
	add := func(name string, p part) error {
		if _, twice := parts[name]; twice {
			return fmt.Errorf("the form has the part %q more than once", name)
		}
		parts[name] = p
		return nil
	}
	for name, fields := range form.Value {
		for _, field := range fields {
			if err := add(name, part{open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(field)), nil }}); err != nil {
				return nil, err
			}
		}
	}
	for name, files := range form.File {
		for _, file := range files {
			if err := add(name, part{filename: file.Filename, open: func() (io.ReadCloser, error) { return file.Open() }}); err != nil {
				return nil, err
			}
		}
	}
	return parts, nil
}

// start starts a run of a location from an upload form. Its parts are:
//
//	start           the location and everything else a start carries, as JSON
//	input:<name>    a file given as input under that name; the part's file
//	                name says what type of file it is
//
// A part the gateway does not know is refused, and so is one given twice.
func (g *Gateway) start(w http.ResponseWriter, r *http.Request, c *client.Client) {
	const shape = "a start is JSON, or an upload form (multipart/form-data) whose parts are start and input:<name>"
	var s start
	var st client.Start
	files := map[string]client.InputFile{}
	if kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); kind == "application/json" {
		// A start with no file is the part start alone, as the request's body.
		if !read(w, r, &s) {
			return
		}
	} else {
		if err := r.ParseMultipartForm(held); err != nil {
			fail(w, http.StatusBadRequest, fmt.Errorf("%s: %v", shape, err))
			return
		}
		// What was not held in memory is in temporary files, and they go
		// before the gateway answers: it keeps nothing.
		defer r.MultipartForm.RemoveAll()
		parts, err := partsOf(r.MultipartForm)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		if _, given := parts["start"]; !given {
			fail(w, http.StatusBadRequest, fmt.Errorf("the form has no part named start; %s", shape))
			return
		}
		for name, p := range parts {
			kind, rest, _ := strings.Cut(name, ":")
			var err error
			switch {
			case name == "start":
				var text string
				if text, err = p.text(); err == nil {
					if err := json.Unmarshal([]byte(text), &s, json.RejectUnknownMembers(true)); err != nil {
						fail(w, http.StatusBadRequest, fmt.Errorf("the part start cannot be read: %v", err))
						return
					}
				}
			case kind == "input" && rest != "":
				var contents io.ReadCloser
				if contents, err = p.open(); err == nil {
					defer contents.Close()
					files[rest] = client.InputFile{Name: p.filename, Contents: contents}
				}
			default:
				fail(w, http.StatusBadRequest, fmt.Errorf("the form has a part named %q; %s", name, shape))
				return
			}
			if err != nil {
				fail(w, http.StatusInternalServerError, err)
				return
			}
		}
	}
	var err error
	if err := store.CheckLocation(s.At); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("a start names the location the run starts at, as at: %v", err))
		return
	}
	// An input of null is no input.
	if st.Input, err = client.WithFiles(s.Input, files); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	st.At, st.Labels = s.At, s.Labels
	st.Timeout, st.Life = time.Duration(s.Timeout), time.Duration(s.Life)
	// The start's own list, which narrows the run and everything below.
	if len(s.Allow)+len(s.Deny) > 0 {
		if err := permission.CheckList(slices.Concat(s.Allow, s.Deny)); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		st.Allow = &permission.Layer{Allow: s.Allow, Deny: s.Deny}
	}
	id, err := c.Start(r.Context(), st)
	reply(w, map[string]any{"id": id}, err)
}

// submit is what the part named submit of a request to submit a tree holds,
// as JSON: the prefix the tree is recorded at, the directory it came from,
// which its machines read as run.context, and, for a tree of one file, the
// file.
type submit struct {
	At        string           `json:"at"`
	Directory string           `json:"directory,omitzero"`
	File      string           `json:"file,omitzero"`
	Life      machine.Duration `json:"life,omitzero"`
}

// submitted is the answer to a submit: its receipt, and what is wrong with
// each file that could not be read as a machine, by its location.
type submitted struct {
	Taken    bool                `json:"taken"`
	Refused  string              `json:"refused,omitzero"`
	Tree     *instance.Submitted `json:"tree,omitzero"`
	Problems map[string]string   `json:"problems,omitzero"`
}

// submit records a tree from an upload form. Its parts are:
//
//	submit          the prefix and what else a submit carries, as JSON
//	file:<path>     a file of the directory, by its path from the directory
//
// The files are read as corbie run submit reads a directory: from a
// directory of the gateway's computer that is gone before the gateway
// answers.
func (g *Gateway) submit(w http.ResponseWriter, r *http.Request, c *client.Client) {
	const shape = "a submit is an upload form (multipart/form-data) whose parts are submit and file:<path>"
	if err := r.ParseMultipartForm(held); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("%s: %v", shape, err))
		return
	}
	defer r.MultipartForm.RemoveAll()
	parts, err := partsOf(r.MultipartForm)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	given, has := parts["submit"]
	if !has {
		fail(w, http.StatusBadRequest, fmt.Errorf("the form has no part named submit; %s", shape))
		return
	}
	var s submit
	if text, err := given.text(); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	} else if err := json.Unmarshal([]byte(text), &s, json.RejectUnknownMembers(true)); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("the part submit cannot be read: %v", err))
		return
	}
	if err := store.CheckLocation(s.At); err != nil || s.At == "/" {
		fail(w, http.StatusBadRequest, fmt.Errorf("a submit names the prefix the tree is recorded at, as at: / and the names of one or more directories, as /home"))
		return
	}
	dir, err := os.MkdirTemp("", "corbie-tree-")
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(dir)
	// A file is written inside the directory and nowhere else.
	root, err := os.OpenRoot(dir)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	defer root.Close()
	for name, p := range parts {
		kind, file, _ := strings.Cut(name, ":")
		switch {
		case name == "submit":
			continue
		case kind != "file" || file == "":
			fail(w, http.StatusBadRequest, fmt.Errorf("the form has a part named %q; %s", name, shape))
			return
		case !filepath.IsLocal(filepath.FromSlash(file)) || strings.Contains(file, `\`):
			fail(w, http.StatusBadRequest, fmt.Errorf("the file %q is not a path inside the directory", file))
			return
		}
		if err := place(root, file, p); err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
	}
	var tree *client.Tree
	if s.File != "" {
		tree, err = client.ReadFile(dir, s.File, s.At, client.ScriptJob{})
	} else {
		tree, err = client.ReadTree(dir, s.At)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, errors.New(strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), "")))
		return
	}
	tree.Directory = s.Directory
	message, err := c.Submit(r.Context(), tree, time.Duration(s.Life))
	if err != nil {
		reply(w, nil, err)
		return
	}
	receipt, err := c.WaitReceipt(r.Context(), message, time.Duration(s.Life), g.interval())
	if err != nil {
		reply(w, nil, err)
		return
	}
	answer := submitted{Taken: receipt.Taken, Refused: receipt.Refused, Tree: receipt.Tree}
	for _, m := range tree.Machines {
		if m.Problem != nil {
			if answer.Problems == nil {
				answer.Problems = map[string]string{}
			}
			answer.Problems[m.Location] = strings.ReplaceAll(m.Problem.Error(), dir+string(filepath.Separator), "")
		}
	}
	reply(w, answer, nil)
}

// place writes a part of an upload form as a file of a directory.
func place(root *os.Root, file string, p part) error {
	if err := root.MkdirAll(filepath.Dir(filepath.FromSlash(file)), 0o700); err != nil {
		return err
	}
	contents, err := p.open()
	if err != nil {
		return err
	}
	defer contents.Close()
	out, err := root.OpenFile(filepath.FromSlash(file), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, contents); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// setting is a set, a remove or a list of the secrets or the values a
// client may set.
type setting struct {
	Act   string           `json:"act"`
	At    string           `json:"at,omitzero"`
	Name  string           `json:"name,omitzero"`
	Value string           `json:"value,omitzero"`
	Life  machine.Duration `json:"life,omitzero"`
}

// set sets, removes or lists the client's secrets or values, and answers
// with the receipt.
func (g *Gateway) set(w http.ResponseWriter, r *http.Request, c *client.Client, of string) {
	var s setting
	if !read(w, r, &s) {
		return
	}
	ctx := r.Context()
	send := map[string]func(life time.Duration) (string, error){
		"secret set":    func(life time.Duration) (string, error) { return c.SetSecret(ctx, s.At, s.Name, s.Value, life) },
		"secret remove": func(life time.Duration) (string, error) { return c.RemoveSecret(ctx, s.At, s.Name, life) },
		"secret list":   func(life time.Duration) (string, error) { return c.ListSecrets(ctx, life) },
		"value set":     func(life time.Duration) (string, error) { return c.SetValue(ctx, s.At, s.Name, s.Value, life) },
		"value remove":  func(life time.Duration) (string, error) { return c.RemoveValue(ctx, s.At, s.Name, life) },
		"value list":    func(life time.Duration) (string, error) { return c.ListValues(ctx, life) },
	}[of+" "+s.Act]
	if send == nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("act is set, remove or list"))
		return
	}
	g.receipt(w, r, c, s.Life, send)
}

// sent is an event sent to an address: a location, or a location and a
// key.
type sent struct {
	To    string           `json:"to"`
	Event string           `json:"event"`
	Data  any              `json:"data,omitzero"`
	Life  machine.Duration `json:"life,omitzero"`
}

// sendTo sends an event to a location, or to a key at one, and answers
// with the receipt.
func (g *Gateway) sendTo(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var s sent
	if !read(w, r, &s) {
		return
	}
	if !strings.HasPrefix(s.To, "/") {
		fail(w, http.StatusBadRequest, fmt.Errorf("to is a location from /, or a location and a key as /location#key; a run is sent an event at /runs/<id>/events"))
		return
	}
	ctx := r.Context()
	g.receipt(w, r, c, s.Life, func(life time.Duration) (string, error) {
		to, err := c.Address(ctx, s.To)
		if err != nil {
			return "", fmt.Errorf("%w: %w", client.ErrStart, err)
		}
		return c.Send(ctx, to, s.Event, s.Data, life)
	})
}

// run answers a request about one run.
func (g *Gateway) run(w http.ResponseWriter, r *http.Request, c *client.Client, id string, rest []string) {
	ctx := r.Context()
	what, asks := "", []string{}
	switch {
	case len(rest) > 0 && rest[0] == "record":
		what, asks = rest[0], []string{"watch"}
	case len(rest) > 0:
		what = rest[0]
	case r.Method == http.MethodGet:
		asks = []string{"wait"}
	}
	if !takes(w, r.URL.Query(), asks...) {
		return
	}
	// An ID that is no run is answered as not found at once, on every
	// route that names a run; one started a moment ago is waiting.
	shown, err := c.Show(ctx, id)
	if err != nil {
		reply(w, nil, err)
		return
	}
	switch {
	case what == "" && r.Method == http.MethodGet:
		if query := r.URL.Query(); query.Has("wait") {
			// A wait may give its own limit. Cut short, by its limit or by
			// the caller going away, it answers with the run as it stands.
			if limit := query.Get("wait"); limit != "" {
				d, err := machine.ParseDuration(limit)
				if err != nil {
					fail(w, http.StatusBadRequest, fmt.Errorf("wait: %v", err))
					return
				}
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(d))
				defer cancel()
			}
			shown, err = c.Wait(ctx, id, g.interval())
		}
		reply(w, shown, err)
	case what == "record" && r.Method == http.MethodGet && !r.URL.Query().Has("watch"):
		// The record so far, and the run as it stands: what the stream
		// gives, without staying.
		entries := []client.Entry{}
		run, err := c.Record(ctx, id, func(entry client.Entry) { entries = append(entries, entry) })
		reply(w, map[string]any{"entries": entries, "run": run}, err)
	case what == "record" && r.Method == http.MethodGet:
		s := newStream(w)
		run, err := c.Follow(ctx, id, g.interval(), func(entry client.Entry) { s.send("entry", entry) })
		// A stream of a record says when the run has ended and how, so
		// its end is told apart from a connection that was lost.
		if err == nil && run.HasEnded() {
			s.send("end", run)
		}
		s.failed(ctx, err)
	case what == "definition" && r.Method == http.MethodGet:
		def, err := c.Definition(ctx, id)
		reply(w, def, err)
	case what == "files" && r.Method == http.MethodGet && len(rest) == 1:
		files, err := c.Files(ctx, id)
		reply(w, files, err)
	case what == "tickets" && r.Method == http.MethodGet:
		tickets, err := c.Tickets(ctx, id)
		reply(w, tickets, err)
	case what == "steps" && r.Method == http.MethodGet && len(rest) == 2:
		number, err := strconv.Atoi(rest[1])
		if err != nil || number < 1 {
			fail(w, http.StatusNotFound, fmt.Errorf("a step is named by its number, from 1"))
			return
		}
		step, err := c.Step(ctx, id, number)
		reply(w, step, err)
	case what == "files" && r.Method == http.MethodGet:
		// The path is the one a blob was shown with, as it is.
		contents, file, err := c.Fetch(ctx, id, strings.Join(rest[1:], "/"))
		if err != nil {
			reply(w, nil, err)
			return
		}
		w.Header().Set("Content-Type", file.Type)
		w.Header().Set("Content-Length", fmt.Sprint(file.Size))
		io.Copy(w, contents)
	case what == "cancel" && r.Method == http.MethodPost:
		var stop cancel
		if read(w, r, &stop) {
			g.receipt(w, r, c, stop.Life, func(life time.Duration) (string, error) { return c.Cancel(ctx, id, life) })
		}
	case what == "events" && r.Method == http.MethodPost:
		var e event
		if read(w, r, &e) {
			g.receipt(w, r, c, e.Life, func(life time.Duration) (string, error) {
				to, err := c.Address(ctx, id)
				if err != nil {
					return "", err
				}
				return c.Send(ctx, to, e.Event, e.Data, life)
			})
		}
	case what == "tickets" && r.Method == http.MethodPost:
		var a answer
		if read(w, r, &a) {
			g.receipt(w, r, c, a.Life, func(life time.Duration) (string, error) { return c.Reply(ctx, id, a.Ticket, a.Value, life) })
		}
	default:
		fail(w, http.StatusNotFound, fmt.Errorf("there is no such route for a run"))
	}
}

// receipt sends a message, good for the life the request gives, and
// answers with what the package reports for it: its receipt, or that it
// was not taken up.
func (g *Gateway) receipt(w http.ResponseWriter, r *http.Request, c *client.Client, life machine.Duration, send func(life time.Duration) (string, error)) {
	message, err := send(time.Duration(life))
	if err != nil {
		reply(w, nil, err)
		return
	}
	receipt, err := c.WaitReceipt(r.Context(), message, time.Duration(life), g.interval())
	reply(w, receipt, err)
}

// read reads a request's body, refusing a field the gateway does not know.
func read(w http.ResponseWriter, r *http.Request, into any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err == nil && len(body) > 0 {
		err = json.Unmarshal(body, into, json.RejectUnknownMembers(true))
	}
	if err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("the request cannot be read: %v", err))
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		var problems machine.Problems
		switch {
		case errors.Is(err, client.ErrNoRun), errors.Is(err, client.ErrNoFile), errors.Is(err, client.ErrNoStep):
			status = http.StatusNotFound
		case errors.Is(err, client.ErrCalled):
			status = http.StatusForbidden
		case errors.Is(err, client.ErrNoRecord):
			// The run is there, and what was asked for is not yet.
			status = http.StatusConflict
		case errors.As(err, &problems), errors.Is(err, client.ErrStart):
			status = http.StatusBadRequest
		case errors.Is(err, client.ErrNotTaken):
			status = http.StatusGatewayTimeout
		case errors.Is(err, context.Canceled):
			return
		}
		fail(w, status, err)
		return
	}
	data, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(append(data, '\n'))
}

func fail(w http.ResponseWriter, status int, err error) {
	data, _ := json.Marshal(map[string]any{"error": err.Error()})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(append(data, '\n'))
}

// failed tells the caller of a stream why it stopped, when the caller did
// not stop it.
func (s *stream) failed(ctx context.Context, err error) {
	if err != nil && ctx.Err() == nil {
		s.send("error", map[string]any{"error": err.Error()})
	}
}

// stream is a stream of events to a caller.
type stream struct {
	w http.ResponseWriter
}

func newStream(w http.ResponseWriter) *stream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return &stream{w: w}
}

func (s *stream) send(name string, v any) {
	data, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}
