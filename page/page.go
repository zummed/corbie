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
// no store where it has none, the stream it follows, the script that draws
// it, and its parts as they are when it is loaded.
type Shell struct {
	Title   string
	Current string
	Offered []string
	Parties []string
	NoStore string
	Stream  string
	Script  string
	Parts   []Part
}

// Write writes a page, with its parts as JSON for its script to draw.
func Write(w http.ResponseWriter, s Shell) error {
	parts := map[string]any{}
	for _, p := range s.Parts {
		parts[p.ID] = p.Data
	}
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

// A Part is one part of a page that changes: its name, and what the page
// draws it from, which is sent as JSON.
type Part struct {
	ID   string
	Data any
}

// Stream sends a page its parts over a WebSocket: every part as it is
// now, then each part whose data changed, at each change and at every
// interval, until the page goes. When the parts cannot be read, the page
// is told the store is lost, and keeps what it shows. A page of another
// origin is refused the WebSocket.
func Stream(w http.ResponseWriter, r *http.Request, every time.Duration, changed func() <-chan struct{}, parts func(ctx context.Context) ([]Part, error)) {
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
		found, err := parts(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			if !lost && send(map[string]string{"store": "lost"}) != nil {
				return
			}
			lost = true
		case err == nil:
			if lost && send(map[string]string{"store": "live"}) != nil {
				return
			}
			lost = false
			for _, p := range found {
				data, err := json.Marshal(p.Data, json.Deterministic(true))
				if err != nil {
					return
				}
				if bytes.Equal(sent[p.ID], data) {
					continue
				}
				if send(map[string]any{"id": p.ID, "data": jsontext.Value(data)}) != nil {
					return
				}
				sent[p.ID] = data
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
