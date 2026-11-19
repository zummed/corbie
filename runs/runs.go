// Package runs is the Runs page, which lists the runs in progress by the
// root each is under, and a run's page, which shows one run and acts on it.
package runs

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/json/v2"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/store"
)

// A List is every run whose status the UI may read, as last read, and a
// signal of each change.
type List interface {
	Runs() ([]*client.Run, error)
	Changed() <-chan struct{}
}

// Config is what the Runs pages are given: the list, the parties that read
// runs, whether the UI acts as the controller, how a command runs, how
// often the store is read, and the shell every page shares.
type Config struct {
	List       List
	Lister     *client.Client
	Client     *client.Client
	Controller bool
	Command    page.Command
	// Machine gives the address on the Machines page of the machine at a
	// location of a tree submitted from a directory, with cards unfolded,
	// and its file; none where the UI offers no Machines page.
	Machine  func(ctx context.Context, location, dir string, cards []string) (href, file string, err error)
	Interval time.Duration
	Shell    page.Shell
}

type handler struct {
	Config
	steps  *steps
	kept   *kept
	titles *titles
}

//go:embed runs.js
var script []byte

// New gives the Runs pages.
func New(c Config) http.Handler {
	h := &handler{Config: c, steps: &steps{}, kept: &kept{}, titles: &titles{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /runs", h.listPage)
	mux.HandleFunc("GET /runs/stream", h.listStream)
	mux.HandleFunc("POST /runs/at/{act}", h.atAct)
	mux.HandleFunc("GET /runs/script.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(script)
	})
	mux.HandleFunc("GET /run/{who}/{stamp}", h.runPage)
	mux.HandleFunc("GET /run/{who}/{stamp}/stream", h.runStream)
	mux.HandleFunc("GET /run/{who}/{stamp}/at", h.at)
	mux.HandleFunc("GET /run/{who}/{stamp}/group", h.group)
	mux.HandleFunc("GET /run/{who}/{stamp}/file", h.file)
	mux.HandleFunc("POST /run/{who}/{stamp}/{act}", h.act)
	return mux
}

// listOf reads the Runs page: the runs, and the latest step of each run in
// a tree of the UI's client that it shows.
func (h *handler) listOf(ctx context.Context, n narrowing) (listView, error) {
	runs, facts, err := h.titled(ctx)
	if runs == nil && err != nil {
		return listView{}, err
	}
	mine := ""
	if h.Client != nil && h.Client.ID != nil {
		mine = h.Client.ID.Name
	}
	read := h.steps.reader(ctx, h.Client, runs)
	v := listOf(runs, read.of, facts, n, time.Now(), mine)
	return v, errors.Join(err, read.done())
}

func (h *handler) listParts(ctx context.Context, n narrowing) (any, error) {
	v, err := h.listOf(ctx, n)
	if err != nil {
		return nil, err
	}
	return listParts(v), nil
}

// ListParts are the parts of the Runs page. The ended runs are sent
// before the running ones, so that a run that ended is found running on
// the page as its new line arrives.
type ListParts struct {
	Narrowing  NarrowingPart `json:"narrowing"`
	Locations  LocationsPart `json:"locations"`
	Here       *placeHead    `json:"here"`
	Ended      section       `json:"ended"`
	Busy       section       `json:"busy"`
	Idle       section       `json:"idle"`
	Singletons section       `json:"singletons"`
}

// NarrowingPart is what the address of the Runs page narrows it to, as its
// head shows it: the location, the window and the labels as typed.
type NarrowingPart struct{ At, Window, Label string }

// LocationsPart is the list of locations: every location, then each
// client's trees.
type LocationsPart struct {
	Every   choice
	Clients []clientPlaces
}

func listParts(v listView) ListParts {
	return ListParts{
		Narrowing: NarrowingPart{At: v.Narrowing.At, Window: v.Narrowing.Window, Label: v.Narrowing.LabelText()},
		Locations: LocationsPart{Every: v.Every, Clients: v.Clients},
		Here:      v.Here, Ended: v.Ended, Busy: v.Busy, Idle: v.Idle, Singletons: v.Singletons,
	}
}

func (h *handler) listPage(w http.ResponseWriter, r *http.Request) {
	n := narrowingOf(r.URL.Query())
	v, _ := h.listOf(r.Context(), n)
	s := h.Shell
	s.Title, s.Current, s.Stream, s.Script = "Runs", "Runs", "/runs/stream"+n.query(), "/runs/script.js"
	page.Write(w, s, listParts(v))
}

func (h *handler) listStream(w http.ResponseWriter, r *http.Request) {
	n := narrowingOf(r.URL.Query())
	page.Stream(w, r, h.Interval, h.List.Changed, func(ctx context.Context) (any, error) { return h.listParts(ctx, n) })
}

func idOf(r *http.Request) (string, error) {
	id := r.PathValue("who") + "/" + r.PathValue("stamp")
	return id, store.CheckInstance(id)
}

func (h *handler) runPage(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		page.Error(w, http.StatusNotFound, err.Error())
		return
	}
	var entries []client.Entry
	if h.mine(id) {
		h.Client.Record(r.Context(), id, func(e client.Entry) { entries = append(entries, e) })
	}
	v, err := h.runOf(r.Context(), id, entries, &reading{})
	if err == nil && v.Mine {
		if last := newest(v.Rows); last != nil {
			v.Shown, err = h.shownAt(r.Context(), id, last, entries, h.byID(r.Context()), v.Run.Started)
		}
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, client.ErrNoRun) {
			status = http.StatusNotFound
		}
		page.Error(w, status, err.Error())
		return
	}
	s := h.Shell
	s.Title, s.Current, s.Stream, s.Script = v.Name, "Runs", "/run/"+id+"/stream", "/runs/script.js"
	parts := runParts(v)
	parts.Shown = &v.Shown
	page.Write(w, s, parts)
}

// runStream follows a run: its status from the list, and, for a run in a
// tree of the UI's client, its record as it is written.
func (h *handler) runStream(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		page.Error(w, http.StatusNotFound, err.Error())
		return
	}
	ctx := r.Context()
	rec := newRecord()
	if h.mine(id) {
		go h.Client.Follow(ctx, id, h.Interval, rec.add)
	}
	changed := func() <-chan struct{} { return either(ctx, h.List.Changed(), rec.Changed()) }
	read := &reading{}
	page.Stream(w, r, h.Interval, changed, func(ctx context.Context) (any, error) {
		v, err := h.runOf(ctx, id, rec.all(), read)
		if err != nil {
			return nil, err
		}
		return runParts(v), nil
	})
}

// at answers a run's page with its drawing and details as of one row of
// Steps, the row named by its key, from the record of the row's run, as
// JSON.
func (h *handler) at(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil || !h.mine(id) {
		page.Error(w, http.StatusNotFound, "only the client whose tree holds a run can read its record")
		return
	}
	ctx := r.Context()
	var entries []client.Entry
	if _, err := h.Client.Record(ctx, id, func(e client.Entry) { entries = append(entries, e) }); err != nil {
		page.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A row's time has its date where it is not on the date the page's run
	// started.
	at, byID, of := time.Now(), h.byID(r.Context()), r.URL.Query().Get("page")
	var day time.Time
	if p := cmp.Or(byID[of], byID[id]); p != nil {
		day = p.Started
	}
	row := rowAt(rowsOf(id, entries, byID, h.mine, at, day), r.URL.Query().Get("row"))
	if row == nil || row.Run.ID != id {
		page.Error(w, http.StatusNotFound, "the run has no such row")
		return
	}
	s, err := h.shownAt(ctx, of, row, entries, byID, day)
	if err != nil {
		page.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.MarshalWrite(w, s, json.Deterministic(true))
}

// group answers the rows of a send's or a call's group, as JSON: the row named by
// its key, of the run in the address, is the send or the call, and the rows
// come from the record of the run it reached. Both runs must be in a tree
// of the UI's client.
func (h *handler) group(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil || !h.mine(id) {
		page.Error(w, http.StatusNotFound, "only the client whose tree holds a run can read its record")
		return
	}
	ctx := r.Context()
	var entries []client.Entry
	if _, err := h.Client.Record(ctx, id, func(e client.Entry) { entries = append(entries, e) }); err != nil {
		page.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	at, byID, of := time.Now(), h.byID(ctx), r.URL.Query().Get("page")
	var day time.Time
	if p := cmp.Or(byID[of], byID[id]); p != nil {
		day = p.Started
	}
	row := rowAt(rowsOf(id, entries, byID, h.mine, at, day), r.URL.Query().Get("row"))
	if row == nil || row.Run.ID != id || row.Trace == nil || !h.mine(row.Trace.Run.ID) {
		page.Error(w, http.StatusNotFound, "the run has no such send or call")
		return
	}
	var reached []client.Entry
	if _, err := h.Client.Record(ctx, row.Trace.Run.ID, func(e client.Entry) { reached = append(reached, e) }); err != nil {
		page.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	depth := row.Depth
	if d, err := strconv.Atoi(r.URL.Query().Get("depth")); err == nil && d >= 0 {
		depth = d
	}
	w.Header().Set("Content-Type", "application/json")
	json.MarshalWrite(w, traced(row.Trace, reached, byID, h.mine, at, day, depth+1), json.Deterministic(true))
}

// titled gives the runs of the list, each run in a tree of the UI's client
// with its title, and what is fixed of each of those.
func (h *handler) titled(ctx context.Context) ([]*client.Run, map[string]fixed, error) {
	runs, err := h.List.Runs()
	titled, facts, read := h.titles.of(ctx, h.Client, runs)
	return titled, facts, errors.Join(err, read)
}

// defined gives where the Machines page shows a run's machine as its file
// now is: the file of the run's location, or of its root's for a child,
// unfolded down the cards of the jobs that started each child. A run in
// another client's tree, one whose machine was made while running, and one
// the Machines page does not show, have none.
func (h *handler) defined(ctx context.Context, id string) (*definedAt, error) {
	if h.Machine == nil || !h.mine(id) {
		return nil, nil
	}
	runs, facts, _ := h.titled(ctx)
	byID := map[string]*client.Run{}
	for _, r := range runs {
		byID[r.ID] = r
	}
	root, cards := cardsOf(id, byID, facts)
	if root == nil {
		return nil, nil
	}
	href, file, err := h.Machine(ctx, root.Location, facts[root.ID].Dir, cards)
	if href == "" || err != nil {
		return nil, err
	}
	return &definedAt{Href: href, File: file, Cards: strings.Join(cards, " › ")}, nil
}

// cardsOf gives the root of a run, and the cards of the jobs that started
// each child down to the run, from the root's machine: none where what is
// fixed of a run on the way is not read, or a machine was made while
// running.
func cardsOf(id string, byID map[string]*client.Run, facts map[string]fixed) (*client.Run, []string) {
	var cards []string
	for r := byID[id]; r != nil; r = byID[r.Parent] {
		f, ok := facts[r.ID]
		switch {
		case !ok || f.Made:
			return nil, nil
		case r.Parent == "":
			return r, cards
		}
		cards = append([]string{jobOf(f.Job)}, cards...)
	}
	return nil, nil
}

// definedAt is where the Machines page shows a run's machine: its address,
// its file, and the cards unfolded down to it.
type definedAt struct {
	Href, File, Cards string
}

// byID gives the runs of the list by their IDs.
func (h *handler) byID(ctx context.Context) map[string]*client.Run {
	runs, _, _ := h.titled(ctx)
	byID := map[string]*client.Run{}
	for _, r := range runs {
		byID[r.ID] = r
	}
	return byID
}

// either gives a channel closed when either of two is, or the context ends.
func either(ctx context.Context, a, b <-chan struct{}) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		select {
		case <-a:
		case <-b:
		case <-ctx.Done():
		}
		close(out)
	}()
	return out
}

// file sends one file of a run's result, as `corbie run show --save`
// fetches it.
func (h *handler) file(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil || !h.mine(id) {
		page.Error(w, http.StatusNotFound, "only the client whose tree holds a run can read its files")
		return
	}
	path := r.URL.Query().Get("path")
	reader, _, err := h.Client.Fetch(r.Context(), id, path)
	if err != nil {
		page.Error(w, http.StatusNotFound, err.Error())
		return
	}
	download(w, path, reader)
}

// download sends a file of a run's result to be saved and never shown: a
// result can hold HTML or SVG a script fetched from anywhere, which shown
// in a page would run with the UI's cookie.
func download(w http.ResponseWriter, path string, r io.Reader) {
	name := strings.Trim(strings.Map(func(r rune) rune {
		if r == '.' || r == '_' || r == '-' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			return r
		}
		return '-'
	}, path), "-.")
	if name == "" {
		name = "file"
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, r)
}

// act runs the command of an action on a run, and answers with what it
// printed: the receipt, or the error.
func (h *handler) act(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		page.Error(w, http.StatusNotFound, err.Error())
		return
	}
	mine := h.mine(id)
	act := r.PathValue("act")
	var call page.Call
	switch act {
	case "reply":
		value, err := jsonOf(r.FormValue("value"))
		if err != nil {
			page.Error(w, http.StatusBadRequest, "the value is not JSON: "+err.Error())
			return
		}
		call = page.Call{Words: []string{"run", "reply"}, Args: []string{id, r.FormValue("ticket")}}
		if value != "" {
			call.Args = append(call.Args, value)
		}
	case "send":
		data, err := jsonOf(r.FormValue("data"))
		if err != nil {
			page.Error(w, http.StatusBadRequest, "the data is not JSON: "+err.Error())
			return
		}
		call = page.Call{Words: []string{"run", "send"}, Args: []string{id, r.FormValue("event")}}
		if data != "" {
			call.Args = append(call.Args, data)
		}
	case "cancel":
		call = cancelOf(id)
		if !mine {
			call.Words = []string{"controller", "cancel"}
		}
	default:
		page.Error(w, http.StatusNotFound, "there is no such action")
		return
	}
	switch {
	case h.Client == nil && call.Words[0] != "controller":
		page.Error(w, http.StatusForbidden, "only a client acts on a run: start the UI as the client")
		return
	case call.Words[0] == "controller" && !h.Controller:
		page.Error(w, http.StatusForbidden, "only the client whose tree holds a run cancels it, or the controller")
		return
	}
	answer := h.Command(r.Context(), call)
	if answer.Status != 0 {
		page.Error(w, http.StatusConflict, refusal(answer))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, receipt(answer))
}

// refusal is what a command that failed answered: the reason its receipt
// gives, in words, or its error as it printed it.
func refusal(a page.Answer) string {
	if said := receipt(a); strings.HasPrefix(said, "refused: ") {
		return said
	}
	return a.Error()
}

// receipt is an action's answer in words: taken, refused with the reason,
// for an event whether the machine acted on it and the run that took it,
// for a start the run it started, and for a cancel at a location each run
// it cancelled.
func receipt(a page.Answer) string {
	var got struct {
		ID      string `json:"id"`
		Taken   bool   `json:"taken"`
		Refused string `json:"refused"`
		Acted   *bool  `json:"acted"`
		Run     string `json:"run"`
	}
	var each []string
	if json.Unmarshal([]byte(strings.TrimSpace(a.Out)), &each) == nil {
		return strings.Join(each, "; ")
	}
	if json.Unmarshal([]byte(strings.TrimSpace(a.Out)), &got) != nil {
		return strings.TrimSpace(a.Said + "\n" + a.Out)
	}
	switch {
	case got.Refused != "":
		return "refused: " + got.Refused
	case got.ID != "":
		return "started " + got.ID
	case got.Acted != nil && !*got.Acted:
		return "taken; the machine has no rule for the event"
	case got.Taken && got.Run != "":
		return "taken by " + got.Run
	case got.Taken:
		return "taken"
	}
	return strings.TrimSpace(a.Said)
}

// jsonOf reads a value typed as JSON and gives it written out again, so a
// command reads it as the value it is. Nothing typed is no value.
func jsonOf(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return "", err
	}
	out, err := json.Marshal(v, json.Deterministic(true))
	return string(out), err
}
