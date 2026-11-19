// Package page is what every page of the UI shares: its bar, its look, the
// commands its buttons run, the script that draws it in the browser, and the
// stream by which it follows the store.
package page

import (
	"bytes"
	"context"
	"embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

//go:embed assets layout.html
var files embed.FS

var layout = template.Must(template.New("layout.html").Funcs(template.FuncMap{"lower": strings.ToLower}).ParseFS(files, "layout.html"))

// Assets serves what a browser is given beside a page: its style, its
// script, Solid, which the script draws pages with, and its fonts.
func Assets() http.Handler {
	sub, _ := fs.Sub(files, "assets")
	return http.FileServerFS(sub)
}

// A Shell is a page around what is its own: its title, the pages the UI
// offers and the one it belongs to, the parties the UI acts as, why it has
// no store where it has none, the stream it follows, and the script that
// draws it.
type Shell struct {
	Title   string
	Current string
	Offered []string
	Parties []string
	NoStore string
	Stream  string
	Script  string
}

// Write writes a page, with its parts as JSON for its script to draw.
// parts is a struct whose fields are the page's parts, each named by its
// field's JSON name; a field left out of the JSON is no part.
func Write(w http.ResponseWriter, s Shell, parts any) error {
	// Escaped for HTML, so no value ends the script element it is in.
	data, err := json.Marshal(parts, json.Deterministic(true), jsontext.EscapeForHTML(true))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return layout.Execute(w, struct {
		Shell
		Data template.JS
	}{s, template.JS(data)})
}

// A Call is one of Corbie's commands, as an action on a page runs it: its
// name, its arguments, its flags, and what it reads on its standard input.
// An argument is a value from the page, and none begins with "-". A flag
// is written whole, as --name=value, or alone where it takes no value.
type Call struct {
	Words []string
	Args  []string
	Flags []string
	Stdin string
}

// String is the command as a person would type it.
func (c Call) String() string {
	return strings.Join(append(append(append([]string{"corbie"}, c.Words...), c.Args...), c.Flags...), " ")
}

// MarshalText gives a call to a page as the command a person would type,
// which its button shows.
func (c Call) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// An Answer is what a command printed: its result as JSON, what it said
// along the way, and how it ended, 0 being success.
type Answer struct {
	Status int
	Out    string
	Said   string
}

// Error is the command's error, as it printed it, or nothing when it
// succeeded.
func (a Answer) Error() string {
	if a.Status == 0 {
		return ""
	}
	if said := strings.TrimSpace(a.Said); said != "" {
		return said
	}
	return strings.TrimSpace(a.Out)
}

// A Command runs a call in the UI's process.
type Command func(ctx context.Context, call Call) Answer

// A PartMessage gives a page one of its parts: its name, and the data the
// page draws it from.
type PartMessage struct {
	Type string         `json:"type" ts:"\"part\""`
	ID   string         `json:"id"`
	Data jsontext.Value `json:"data"`
}

// A StoreMessage tells a page the store cannot be read ("lost"), or can be
// again ("live").
type StoreMessage struct {
	Type  string `json:"type" ts:"\"store\""`
	Store string `json:"store" ts:"\"lost\" | \"live\""`
}

// partsOf gives the parts of a page's parts, in the order of their fields.
func partsOf(parts any) ([]PartMessage, error) {
	data, err := json.Marshal(parts, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	d := jsontext.NewDecoder(bytes.NewReader(data))
	if _, err := d.ReadToken(); err != nil {
		return nil, err
	}
	var out []PartMessage
	for d.PeekKind() != '}' {
		name, err := d.ReadToken()
		if err != nil {
			return nil, err
		}
		// A token is read before the next read voids it.
		id := name.String()
		value, err := d.ReadValue()
		if err != nil {
			return nil, err
		}
		out = append(out, PartMessage{Type: "part", ID: id, Data: value.Clone()})
	}
	return out, nil
}

// Stream sends a page its parts over a WebSocket: every part as it is
// now, then each part whose data changed, at each change and at every
// interval, until the page goes. When the parts cannot be read, the page
// is told the store is lost, and keeps what it shows. A page of another
// origin is refused the WebSocket.
// parts gives a struct whose fields are the parts, as Write takes it.
func Stream(w http.ResponseWriter, r *http.Request, every time.Duration, changed func() <-chan struct{}, parts func(ctx context.Context) (any, error)) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())
	send := func(v any) error {
		data, err := json.Marshal(v, json.Deterministic(true))
		if err != nil {
			return err
		}
		return conn.Write(ctx, websocket.MessageText, data)
	}
	sent := map[string][]byte{}
	lost := false
	for {
		// Taken before the read, so a change made after the read is not missed.
		changes := changed()
		read, err := parts(ctx)
		var found []PartMessage
		if err == nil {
			found, err = partsOf(read)
		}
		switch {
		case err != nil && ctx.Err() == nil:
			if !lost && send(StoreMessage{Type: "store", Store: "lost"}) != nil {
				return
			}
			lost = true
		case err == nil:
			if lost && send(StoreMessage{Type: "store", Store: "live"}) != nil {
				return
			}
			lost = false
			for _, p := range found {
				if bytes.Equal(sent[p.ID], p.Data) {
					continue
				}
				if send(p) != nil {
					return
				}
				sent[p.ID] = p.Data
			}
		}
		timer := time.NewTimer(every)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-changes:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// Error writes an error as the page shows it beside what failed, untouched.
func Error(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, text)
}
