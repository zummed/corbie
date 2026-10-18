// Package standin is an MCP server for tests: a server of notes, which
// speaks MCP 2026-07-28 or only an earlier version, over a program's
// standard streams or over the web.
package standin

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
)

const latest = "2026-07-28"

// A Server is a server of notes, with these tools:
//
//	add_note        adds a note
//	find_notes      finds the notes with a text; shelf is carried in a header
//	delete_notes    deletes the notes with a text, once asked to confirm
//	picture         gives a picture and its size as data
//	locked          reports that it failed
type Server struct {
	// Legacy is the earlier version the server speaks, after a handshake;
	// empty, it speaks 2026-07-28 alone.
	Legacy string
	// Token, where set, is the bearer token a request over the web must
	// carry; Metadata is where a refusal says the server's metadata is.
	Token    string
	Metadata string
	// Only, where set, are the tools it has; it has them all otherwise.
	Only []string

	mu    sync.Mutex
	notes []string
	// initialized says a program of an earlier version has had its
	// handshake; over the web it is Sessions that do.
	initialized bool
	sessions    map[string]bool
	// Requests are the headers of each request over the web, in order.
	Requests []http.Header
	// Ended are the sessions a client ended.
	Ended []string
}

var tools = []any{
	map[string]any{"name": "add_note", "description": "Add a note", "inputSchema": map[string]any{
		"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []any{"text"}}},
	map[string]any{"name": "find_notes", "description": "Find the notes with a text", "inputSchema": map[string]any{
		"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"},
			"shelf": map[string]any{"type": "string", "x-mcp-header": "Shelf"}}, "required": []any{"text"}}},
	map[string]any{"name": "delete_notes", "description": "Delete the notes with a text", "inputSchema": map[string]any{
		"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}},
	map[string]any{"name": "picture", "description": "Draw a picture", "inputSchema": map[string]any{"type": "object"}},
	map[string]any{"name": "locked", "description": "Fail", "inputSchema": map[string]any{"type": "object"}},
	// A tool whose header is on a property of a list's items cannot be
	// carried, and a client over the web leaves it out.
	map[string]any{"name": "badly_marked", "description": "Marked where no header can be", "inputSchema": map[string]any{
		"type": "object", "properties": map[string]any{"tags": map[string]any{"type": "array",
			"items": map[string]any{"type": "string", "x-mcp-header": "Tag"}}}}},
}

func (s *Server) note() []string {
	if s.notes == nil {
		s.notes = []string{"tea at 4", "tea at 5", "cake at 6"}
	}
	return s.notes
}

func failure(id any, code int, message string, data any) map[string]any {
	e := map[string]any{"code": code, "message": message}
	if data != nil {
		e["data"] = data
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": e}
}

// Answer gives the answer to one message, or nothing for a notification.
// over says the message came over the web, in a session if one is named.
func (s *Server) Answer(m map[string]any, session string, web bool) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, method := m["id"], m["method"].(string)
	params, _ := m["params"].(map[string]any)
	if id == nil {
		return nil
	}
	result := func(r map[string]any) map[string]any {
		if _, said := r["resultType"]; !said && s.Legacy == "" {
			r["resultType"] = "complete"
		}
		return map[string]any{"jsonrpc": "2.0", "id": id, "result": r}
	}
	if s.Legacy != "" {
		started := s.initialized
		if web {
			started = s.sessions[session]
		}
		switch {
		case method == "initialize":
			return result(map[string]any{"protocolVersion": s.Legacy, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "notes", "version": "1"}})
		case !started:
			return failure(id, -32600, "the server is not initialized", nil)
		}
	} else {
		meta, _ := params["_meta"].(map[string]any)
		version, _ := meta["io.modelcontextprotocol/protocolVersion"].(string)
		switch {
		case method == "initialize":
			return failure(id, -32601, "initialize is not a method of MCP "+latest, nil)
		case version == "":
			return failure(id, -32602, "the request has no protocol version in _meta", nil)
		case version != latest:
			return failure(id, -32022, "Unsupported protocol version", map[string]any{"supported": []any{latest}, "requested": version})
		}
	}
	switch method {
	case "server/discover":
		if s.Legacy != "" {
			return failure(id, -32601, "Method not found", nil)
		}
		return result(map[string]any{"supportedVersions": []any{latest}, "capabilities": map[string]any{"tools": map[string]any{}}})
	case "tools/list":
		// Two tools a page.
		from := 0
		if cursor, ok := params["cursor"].(string); ok {
			fmt.Sscan(cursor, &from)
		}
		offered := tools
		if s.Only != nil {
			offered = nil
			for _, tool := range tools {
				if slices.Contains(s.Only, tool.(map[string]any)["name"].(string)) {
					offered = append(offered, tool)
				}
			}
		}
		r := map[string]any{"tools": offered[from:min(from+2, len(offered))]}
		if from+2 < len(offered) {
			r["nextCursor"] = fmt.Sprint(from + 2)
		}
		return result(r)
	case "tools/call":
		args, _ := params["arguments"].(map[string]any)
		text, _ := args["text"].(string)
		said := func(t string) map[string]any {
			return result(map[string]any{"content": []any{map[string]any{"type": "text", "text": t}}})
		}
		switch params["name"] {
		case "add_note":
			s.notes = append(s.note(), text)
			return said("added")
		case "find_notes":
			var found []string
			for _, n := range s.note() {
				if strings.Contains(n, text) {
					found = append(found, n)
				}
			}
			return said(fmt.Sprintf("%d notes: %s", len(found), strings.Join(found, ", ")))
		case "delete_notes":
			responses, _ := params["inputResponses"].(map[string]any)
			confirm, _ := responses["confirm"].(map[string]any)
			if params["requestState"] != "delete:"+text || confirm["action"] != "accept" {
				return result(map[string]any{"resultType": "input_required", "requestState": "delete:" + text, "inputRequests": map[string]any{
					"confirm": map[string]any{"method": "elicitation/create", "params": map[string]any{"mode": "form",
						"message": "Delete the notes with " + text + "?", "requestedSchema": map[string]any{"type": "object"}}}}})
			}
			var kept []string
			for _, n := range s.note() {
				if !strings.Contains(n, text) {
					kept = append(kept, n)
				}
			}
			gone := len(s.note()) - len(kept)
			s.notes = kept
			return said(fmt.Sprintf("deleted %d notes", gone))
		case "picture":
			return result(map[string]any{"content": []any{
				map[string]any{"type": "image", "mimeType": "image/png", "data": base64.StdEncoding.EncodeToString([]byte("\x89PNG a picture"))},
				map[string]any{"type": "resource_link", "uri": "notes://all", "name": "all notes"}},
				"structuredContent": map[string]any{"width": 1, "height": 1}})
		case "locked":
			r := said("the notes are locked")
			r["result"].(map[string]any)["isError"] = true
			return r
		}
		return failure(id, -32602, fmt.Sprintf("Unknown tool: %v", params["name"]), nil)
	}
	return failure(id, -32601, "Method not found", nil)
}

// Serve speaks over a program's standard streams: a message a line.
func (s *Server) Serve(in io.Reader, out io.Writer) {
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 64<<10), 16<<20)
	for lines.Scan() {
		var m map[string]any
		if json.Unmarshal(lines.Bytes(), &m) != nil {
			continue
		}
		if m["method"] == "notifications/initialized" {
			s.mu.Lock()
			s.initialized = true
			s.mu.Unlock()
		}
		if answer := s.Answer(m, "", false); answer != nil {
			line, _ := json.Marshal(answer)
			fmt.Fprintf(out, "%s\n", line)
		}
	}
}

// ServeHTTP speaks over the web: each message is a POST of its own, and a
// call is answered as server-sent events.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.Requests = append(s.Requests, r.Header.Clone())
	session := r.Header.Get("Mcp-Session-Id")
	if r.Method == "DELETE" {
		s.Ended = append(s.Ended, session)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if s.Token != "" && r.Header.Get("Authorization") != "Bearer "+s.Token {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s", scope="notes:read"`, s.Metadata))
		w.WriteHeader(401)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		http.Error(w, "not JSON", 400)
		return
	}
	method, _ := m["method"].(string)
	params, _ := m["params"].(map[string]any)
	reply := func(status int, answer map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(answer)
	}
	if s.Legacy != "" {
		s.mu.Lock()
		known := s.sessions[session]
		switch {
		case method == "initialize":
			if s.sessions == nil {
				s.sessions = map[string]bool{}
			}
			session = fmt.Sprintf("session-%d", len(s.sessions)+1)
			w.Header().Set("Mcp-Session-Id", session)
		case !known:
			s.mu.Unlock()
			// As servers of the time do, before a handshake.
			reply(400, failure(nil, -32000, "Bad Request: No valid session ID provided", nil))
			return
		case method == "notifications/initialized":
			s.mu.Unlock()
			w.WriteHeader(202)
			return
		}
		s.mu.Unlock()
		answer := s.Answer(m, session, true)
		if method == "initialize" {
			s.mu.Lock()
			s.sessions[session] = true
			s.mu.Unlock()
		}
		reply(200, answer)
		return
	}
	// A server of latest checks that the headers say what the body does.
	meta, _ := params["_meta"].(map[string]any)
	args, _ := params["arguments"].(map[string]any)
	name, _ := params["name"].(string)
	shelf, _ := args["shelf"].(string)
	switch {
	case r.Header.Get("MCP-Protocol-Version") == "" || r.Header.Get("Mcp-Method") == "":
		reply(400, failure(m["id"], -32020, "Header mismatch: a required header is missing", nil))
		return
	case r.Header.Get("MCP-Protocol-Version") != meta["io.modelcontextprotocol/protocolVersion"]:
		reply(400, failure(m["id"], -32020, "Header mismatch: MCP-Protocol-Version", nil))
		return
	case r.Header.Get("Mcp-Method") != method:
		reply(400, failure(m["id"], -32020, "Header mismatch: Mcp-Method", nil))
		return
	case method == "tools/call" && decoded(r.Header.Get("Mcp-Name")) != name:
		reply(400, failure(m["id"], -32020, "Header mismatch: Mcp-Name", nil))
		return
	case method == "tools/call" && name == "find_notes" && decoded(r.Header.Get("Mcp-Param-Shelf")) != shelf:
		reply(400, failure(m["id"], -32020, "Header mismatch: Mcp-Param-Shelf", nil))
		return
	}
	answer := s.Answer(m, "", true)
	if answer == nil {
		w.WriteHeader(202)
		return
	}
	if _, failed := answer["error"]; failed {
		reply(400, answer)
		return
	}
	if method != "tools/call" {
		reply(200, answer)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	progress, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress", "params": map[string]any{"progress": 1}})
	whole, _ := json.Marshal(answer)
	fmt.Fprintf(w, ": the call has begun\n\nevent: message\ndata: %s\n\ndata: %s\n\n", progress, whole)
}

// decoded reads a header's value written in base64.
func decoded(v string) string {
	if inner, ok := strings.CutPrefix(v, "=?base64?"); ok && strings.HasSuffix(inner, "?=") {
		text, _ := base64.StdEncoding.DecodeString(strings.TrimSuffix(inner, "?="))
		return string(text)
	}
	return v
}
