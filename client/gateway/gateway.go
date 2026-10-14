// Package gateway is the client package over HTTP: a route for each thing
// the package sends and each thing it reads, and streams for what can be
// followed. A gateway is one client with one ceiling; it keeps nothing
// itself.
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
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A Gateway serves one client's runs to callers that show its token.
type Gateway struct {
	Client *client.Client
	// Token is what callers show; a request without the right one is
	// refused and does nothing.
	Token string
	// Interval is how often a stream looks again at a store that cannot
	// tell it when something was written.
	Interval time.Duration
}

// start is what the part named start of a request to start a run holds,
// as JSON. A field the gateway does not know is refused.
type start struct {
	Input   any               `json:"input,omitzero"`
	Labels  map[string]string `json:"labels,omitzero"`
	Allow   []string          `json:"allow,omitzero"`
	Deny    []string          `json:"deny,omitzero"`
	Timeout machine.Duration  `json:"timeout,omitzero"`
	Secrets map[string]string `json:"secrets,omitzero"`
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
	// The token is compared in a way that takes the same time whether it
	// is nearly right or wholly wrong, and whatever its length: what is
	// compared is a digest of each, and two digests are of one length.
	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	shown := sha256.Sum256([]byte(token))
	held := sha256.Sum256([]byte(g.Token))
	if g.Token == "" || !bearer || subtle.ConstantTimeCompare(shown[:], held[:]) != 1 {
		fail(w, http.StatusUnauthorized, errors.New("the request does not show the gateway's token (Authorization: Bearer …)"))
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if parts[0] != "runs" {
		fail(w, http.StatusNotFound, fmt.Errorf("there is no such route; the routes begin /runs"))
		return
	}
	ctx := r.Context()
	query := r.URL.Query()
	switch {
	case len(parts) == 1 && r.Method == http.MethodPost:
		if takes(w, query) {
			g.start(w, r)
		}
	case len(parts) == 1 && r.Method == http.MethodGet:
		if !takes(w, query, "all", "status", "parent", "label", "watch") {
			return
		}
		filter := client.Filter{Status: query.Get("status"), Parent: query.Get("parent"), Finished: query.Has("all")}
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
			g.Client.Watch(ctx, filter, g.interval(), func(u client.Update) { s.send("run", u) })
			return
		}
		runs, err := g.Client.List(ctx, filter)
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
		g.run(w, r, id, parts[3:])
	default:
		fail(w, http.StatusNotFound, fmt.Errorf("there is no such route"))
	}
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

// start starts a run from an upload form. Its parts are:
//
//	definition      the definition
//	file:<path>     a file beside the definition, at the path it names it by
//	input:<name>    a file given as input under that name; the part's file
//	                name says what type of file it is
//	start           everything else a start may carry, as JSON
//
// A part the gateway does not know is refused, and so is one given twice.
func (g *Gateway) start(w http.ResponseWriter, r *http.Request) {
	const shape = "a start is an upload form (multipart/form-data) whose parts are definition, file:<path>, input:<name> and start"
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
	if _, given := parts["definition"]; !given {
		fail(w, http.StatusBadRequest, fmt.Errorf("the form has no part named definition; %s", shape))
		return
	}
	var s start
	var st client.Start
	var definition string
	beside := map[string]string{}
	for name, p := range parts {
		kind, rest, _ := strings.Cut(name, ":")
		var err error
		switch {
		case name == "definition":
			definition, err = p.text()
		case name == "start":
			var text string
			if text, err = p.text(); err == nil {
				if err := json.Unmarshal([]byte(text), &s, json.RejectUnknownMembers(true)); err != nil {
					fail(w, http.StatusBadRequest, fmt.Errorf("the part start cannot be read: %v", err))
					return
				}
			}
		case kind == "file" && rest != "":
			beside[rest], err = p.text()
		case kind == "input" && rest != "":
			var contents io.ReadCloser
			if contents, err = p.open(); err == nil {
				defer contents.Close()
				if st.Files == nil {
					st.Files = map[string]client.InputFile{}
				}
				st.Files[rest] = client.InputFile{Name: p.filename, Contents: contents}
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
	// Only the files given with the definition are read, and each path
	// names one thing: not a file and a directory both, nor one file twice.
	given := map[string]string{}
	for name := range beside {
		clean := path.Clean(name)
		if other, twice := given[clean]; twice {
			fail(w, http.StatusBadRequest, fmt.Errorf("the files %q and %q are one path, given twice", other, name))
			return
		}
		given[clean] = name
	}
	for clean, name := range given {
		for dir := path.Dir(clean); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if file, both := given[dir]; both {
				fail(w, http.StatusBadRequest, fmt.Errorf("%q is given as a file, and as a directory by %q: a path names one or the other", file, name))
				return
			}
		}
	}
	draft, err := machine.Parse([]byte(definition), machine.Given(beside))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	// An input of null is no input.
	st.Input, st.Labels, st.Secrets = s.Input, s.Labels, s.Secrets
	st.Timeout, st.Life = time.Duration(s.Timeout), time.Duration(s.Life)
	// The start's own list, which narrows the run and everything below.
	if len(s.Allow)+len(s.Deny) > 0 {
		if err := permission.CheckList(slices.Concat(s.Allow, s.Deny)); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		st.Allow = &permission.Layer{Allow: s.Allow, Deny: s.Deny}
	}
	id, err := g.Client.Start(r.Context(), draft, st)
	reply(w, map[string]any{"id": id}, err)
}

// run answers a request about one run.
func (g *Gateway) run(w http.ResponseWriter, r *http.Request, id string, rest []string) {
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
	shown, err := g.Client.Show(ctx, id)
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
			shown, err = g.Client.Wait(ctx, id, g.interval())
		}
		reply(w, shown, err)
	case what == "record" && r.Method == http.MethodGet && !r.URL.Query().Has("watch"):
		// The record so far, and the run as it stands: what the stream
		// gives, without staying.
		entries := []client.Entry{}
		run, err := g.Client.Record(ctx, id, func(entry client.Entry) { entries = append(entries, entry) })
		reply(w, map[string]any{"entries": entries, "run": run}, err)
	case what == "record" && r.Method == http.MethodGet:
		s := newStream(w)
		run, err := g.Client.Follow(ctx, id, g.interval(), func(entry client.Entry) { s.send("entry", entry) })
		// A stream of a record says when the run has ended and how, so
		// its end is told apart from a connection that was lost.
		if err == nil && run.HasEnded() {
			s.send("end", run)
		}
	case what == "definition" && r.Method == http.MethodGet:
		def, err := g.Client.Definition(ctx, id)
		reply(w, def, err)
	case what == "files" && r.Method == http.MethodGet:
		// The path is the one a blob was shown with, as it is.
		contents, file, err := g.Client.Fetch(ctx, id, strings.Join(rest[1:], "/"))
		if err != nil {
			reply(w, nil, err)
			return
		}
		w.Header().Set("Content-Type", file.Type)
		w.Header().Set("Content-Length", fmt.Sprint(file.Size))
		io.Copy(w, contents)
	case what == "cancel" && r.Method == http.MethodPost:
		var c cancel
		if read(w, r, &c) {
			g.receipt(w, r, c.Life, func(life time.Duration) (string, error) { return g.Client.Cancel(ctx, id, life) })
		}
	case what == "events" && r.Method == http.MethodPost:
		var e event
		if read(w, r, &e) {
			g.receipt(w, r, e.Life, func(life time.Duration) (string, error) { return g.Client.Send(ctx, id, e.Event, e.Data, life) })
		}
	case what == "tickets" && r.Method == http.MethodPost:
		var a answer
		if read(w, r, &a) {
			g.receipt(w, r, a.Life, func(life time.Duration) (string, error) { return g.Client.Reply(ctx, id, a.Ticket, a.Value, life) })
		}
	default:
		fail(w, http.StatusNotFound, fmt.Errorf("there is no such route for a run"))
	}
}

// receipt sends a message, good for the life the request gives, and
// answers with what the package reports for it: its receipt, or that it
// was not taken up.
func (g *Gateway) receipt(w http.ResponseWriter, r *http.Request, life machine.Duration, send func(life time.Duration) (string, error)) {
	message, err := send(time.Duration(life))
	if err != nil {
		reply(w, nil, err)
		return
	}
	receipt, err := g.Client.WaitReceipt(r.Context(), message, time.Duration(life), g.interval())
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
		case errors.Is(err, client.ErrNoRun), errors.Is(err, client.ErrNoFile):
			status = http.StatusNotFound
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
