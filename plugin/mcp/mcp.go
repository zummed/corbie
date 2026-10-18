// Package mcp is the plugin for an MCP server: its tools, and calls to
// them, over the web or from a program it starts.
package mcp

import (
	"context"
	"encoding/base64"
	"errors"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

// latest is the version of MCP the plugin speaks first. It speaks the
// earlier ones, newest first, to a server that has not latest; each of
// them begins with a handshake.
const latest = "2026-07-28"

var earlier = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// wait is how long each call waits on the server unless it says.
var wait = time.Minute

func init() {
	plugin.Register(&plugin.Module{Name: "mcp", Page: page, Calls: map[string]*plugin.Call{
		"connect": {Options: []string{"headers", "client", "timeout"}, Do: connect},
		"start":   {Options: []string{"args", "env", "dir", "timeout"}, Do: start},
	}})
}

// A message is one JSON-RPC message: a request, a notification or an
// answer.
type message struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitzero"`
	Method  string         `json:"method,omitzero"`
	Params  map[string]any `json:"params,omitzero"`
	Result  map[string]any `json:"result,omitzero"`
	Error   *rpcError      `json:"error,omitzero"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitzero"`
}

// The codes of the errors a server answers with that the plugin acts on.
const (
	headerMismatch     = -32020
	missingCapability  = -32021
	unsupportedVersion = -32022
	invalidParams      = -32602
	methodNotFound     = -32601
)

// A way is how messages reach a server and come back.
type way interface {
	// ask sends a request and gives its answer. headers are the parts of
	// the request the web carries in headers as well; a program has none.
	ask(ctx context.Context, m *message, headers map[string]string) (*message, error)
	// tell sends a notification.
	tell(ctx context.Context, m *message, headers map[string]string) error
	close()
}

// unmodern is an answer that is not MCP of latest: a server of an earlier
// version answering something it does not know.
type unmodern struct{ said string }

func (u *unmodern) Error() string { return u.said }

// A server is one MCP server as a script holds it.
type server struct {
	way  way
	name string // the address or the program, as errors name it
	// web says the server is reached over the web, which carries parts of
	// a request in headers.
	web   bool
	limit time.Duration
	// legacy is the earlier version the server speaks, after the
	// handshake; empty for latest.
	legacy string
	next   int
	// schemas are the input schemas of the tools listed, by name: a call
	// carries the arguments they mark in headers.
	schemas map[string]map[string]any
	closed  bool
}

// self is who the plugin says it is to a server.
var self = func() map[string]any {
	version := "(devel)"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	return map[string]any{"name": "corbie", "version": version}
}()

// capabilities are what the plugin can do for a server: answer a request
// for input made with a form, through the script.
var capabilities = map[string]any{"elicitation": map[string]any{"form": map[string]any{}}}

// object is the server as the script holds it.
func (s *server) object() *plugin.Object {
	return &plugin.Object{Close: s.close, Calls: map[string]*plugin.Call{
		"tools": {Do: s.tools},
		"call":  {Options: []string{"args", "responses", "state"}, Do: s.call},
	}}
}

func (s *server) close() error {
	if !s.closed {
		s.closed = true
		s.way.close()
	}
	return nil
}

func (s *server) isOpen() error {
	if s.closed {
		return plugin.Errorf(plugin.BadInput, "the server %s is closed", s.name)
	}
	return nil
}

// agree finds which version the server speaks: latest, if it answers
// server/discover or says which versions it has; otherwise an earlier one,
// after the handshake.
func (s *server) agree(ctx context.Context) error {
	probe := ctx
	if !s.web {
		// A program of an earlier version may say nothing to a request it
		// does not know: half the time is left for the handshake.
		var cancel context.CancelFunc
		probe, cancel = context.WithTimeout(ctx, s.limit/2)
		defer cancel()
	}
	answer, err := s.send(probe, "server/discover", map[string]any{}, "", nil)
	var old *unmodern
	switch {
	case errors.As(err, &old):
		return s.handshake(ctx, earlier[0])
	case errors.Is(err, context.DeadlineExceeded) && !s.web && ctx.Err() == nil:
		return s.handshake(ctx, earlier[0])
	case err != nil:
		return err
	case answer.Error == nil:
		return s.choose(ctx, answer.Result["supportedVersions"])
	case answer.Error.Code == unsupportedVersion:
		data, _ := answer.Error.Data.(map[string]any)
		return s.choose(ctx, data["supported"])
	case answer.Error.Code == headerMismatch || answer.Error.Code == missingCapability:
		return s.failed(answer.Error)
	}
	return s.handshake(ctx, earlier[0])
}

// choose speaks latest if the server has it, or the newest earlier
// version it has.
func (s *server) choose(ctx context.Context, supported any) error {
	versions, _ := supported.([]any)
	if slices.Contains(versions, any(latest)) {
		return nil
	}
	for _, version := range earlier {
		if slices.Contains(versions, any(version)) {
			return s.handshake(ctx, version)
		}
	}
	return plugin.Errorf(plugin.Failed, "the server %s speaks MCP %v, and the plugin speaks %s and %s", s.name, versions, latest, strings.Join(earlier, ", "))
}

// handshake begins speaking an earlier version.
func (s *server) handshake(ctx context.Context, version string) error {
	answer, err := s.way.ask(ctx, s.message("initialize", map[string]any{
		"protocolVersion": version, "capabilities": map[string]any{}, "clientInfo": self}), nil)
	var old *unmodern
	switch {
	case errors.As(err, &old):
		return plugin.Errorf(plugin.Failed, "the server %s took neither MCP %s nor the handshake of an earlier version: %s", s.name, latest, old.said)
	case err != nil:
		return s.failure(ctx, err)
	case answer.Error != nil:
		return plugin.Errorf(plugin.Failed, "the server %s took neither MCP %s nor the handshake of MCP %s: %s", s.name, latest, version, answer.Error.Message)
	}
	spoken, _ := answer.Result["protocolVersion"].(string)
	if !slices.Contains(earlier, spoken) {
		return plugin.Errorf(plugin.Failed, "the server %s answered the handshake with MCP %q, which the plugin does not speak", s.name, spoken)
	}
	s.legacy = spoken
	if err := s.way.tell(ctx, &message{JSONRPC: "2.0", Method: "notifications/initialized"}, s.headers("", "", nil)); err != nil {
		return s.failure(ctx, err)
	}
	return nil
}

func (s *server) message(method string, params map[string]any) *message {
	s.next++
	return &message{JSONRPC: "2.0", ID: s.next, Method: method, Params: params}
}

// send sends a request. In latest, every request says its version, who the
// plugin is and what it can do; name is the tool a call is to.
func (s *server) send(ctx context.Context, method string, params map[string]any, name string, args map[string]any) (*message, error) {
	if s.legacy == "" {
		params["_meta"] = map[string]any{
			"io.modelcontextprotocol/protocolVersion":    latest,
			"io.modelcontextprotocol/clientInfo":         self,
			"io.modelcontextprotocol/clientCapabilities": capabilities,
		}
	}
	return s.way.ask(ctx, s.message(method, params), s.headers(method, name, args))
}

// headers are the parts of a request the web carries in headers as well.
func (s *server) headers(method, name string, args map[string]any) map[string]string {
	if s.legacy != "" {
		return map[string]string{"MCP-Protocol-Version": s.legacy}
	}
	h := map[string]string{"MCP-Protocol-Version": latest, "Mcp-Method": method}
	if name != "" {
		h["Mcp-Name"] = headerValue(name)
		for k, v := range mirrored(s.schemas[name], args) {
			h[k] = v
		}
	}
	return h
}

// request sends a request and gives its result.
func (s *server) request(ctx context.Context, method string, params map[string]any, name string, args map[string]any) (map[string]any, error) {
	answer, err := s.send(ctx, method, params, name, args)
	if err != nil {
		return nil, s.failure(ctx, err)
	}
	if answer.Error != nil {
		return nil, s.failed(answer.Error)
	}
	return answer.Result, nil
}

// failed gives an error the server answered with the one shape of error.
func (s *server) failed(e *rpcError) error {
	if e.Code == invalidParams {
		return plugin.Errorf(plugin.BadInput, "the server %s refused the request: %s", s.name, e.Message)
	}
	return plugin.Errorf(plugin.Failed, "the server %s answered with an error: %s (%d)", s.name, e.Message, e.Code)
}

// failure gives a failed request the one shape of error. A refusal passes
// through as it is.
func (s *server) failure(ctx context.Context, err error) error {
	if refusal := plugin.Refusal(err); refusal != nil {
		return refusal
	}
	var known *plugin.Error
	var old *unmodern
	switch {
	case errors.As(err, &known):
		return known
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		return plugin.Errorf(plugin.TimedOut, "the server %s did not answer within %s", s.name, s.limit)
	case errors.As(err, &old):
		return plugin.Errorf(plugin.Failed, "the server %s answered: %s", s.name, old.said)
	}
	return plugin.Errorf(plugin.Refused, "the connection to the server %s failed: %v", s.name, err)
}

// tools lists every tool the server has, reading its pages to the end.
func (s *server) tools(job *plugin.Job, first any, _ plugin.Options) (any, error) {
	if first != nil {
		return nil, plugin.Errorf(plugin.BadInput, "tools takes nothing")
	}
	if err := s.isOpen(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(job.Context, s.limit)
	defer cancel()
	return s.list(ctx)
}

func (s *server) list(ctx context.Context) ([]any, error) {
	tools := []any{}
	s.schemas = map[string]map[string]any{}
	cursor := ""
	for page := 0; ; page++ {
		if page == 1000 {
			return nil, plugin.Errorf(plugin.Failed, "the server %s's list of tools did not end after a thousand pages", s.name)
		}
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := s.request(ctx, "tools/list", params, "", nil)
		if err != nil {
			return nil, err
		}
		listed, _ := result["tools"].([]any)
		for _, item := range listed {
			tool, _ := item.(map[string]any)
			name, _ := tool["name"].(string)
			does, _ := tool["description"].(string)
			schema, _ := tool["inputSchema"].(map[string]any)
			// A tool whose arguments the web cannot carry in headers as it
			// says is left out.
			if s.web && !mirrorable(schema) {
				continue
			}
			s.schemas[name] = schema
			tools = append(tools, map[string]any{"name": name, "does": does, "args": schema})
		}
		if cursor, _ = result["nextCursor"].(string); cursor == "" {
			return tools, nil
		}
	}
}

// call calls a tool. A server that asks for input first gives back its
// requests and its state, and the same call made with the responses and
// the state carries on.
func (s *server) call(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	name, err := plugin.Text(first, "the tool's name")
	if err != nil {
		return nil, err
	}
	if err := s.isOpen(); err != nil {
		return nil, err
	}
	args, err := opt.Map("args")
	if err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	responses, err := opt.Map("responses")
	if err != nil {
		return nil, err
	}
	state, err := opt.Text("state", "")
	if err != nil {
		return nil, err
	}
	for what, v := range map[string]any{"arguments": args, "responses": responses} {
		if err := data(what, v); err != nil {
			return nil, err
		}
	}
	if s.legacy != "" && (responses != nil || state != "") {
		return nil, plugin.Errorf(plugin.BadInput, "the server %s speaks MCP %s, which has no requests for input to respond to", s.name, s.legacy)
	}
	params := func() map[string]any {
		p := map[string]any{"name": name, "arguments": args}
		if responses != nil {
			p["inputResponses"] = responses
		}
		if state != "" {
			p["requestState"] = state
		}
		return p
	}
	ctx, cancel := context.WithTimeout(job.Context, s.limit)
	defer cancel()
	answer, err := s.send(ctx, "tools/call", params(), name, args)
	if err == nil && answer.Error != nil && answer.Error.Code == headerMismatch && s.web {
		// The server wants arguments carried in headers that the plugin
		// learns of from the list of tools.
		if _, err := s.list(ctx); err != nil {
			return nil, err
		}
		answer, err = s.send(ctx, "tools/call", params(), name, args)
	}
	if err != nil {
		return nil, s.failure(ctx, err)
	}
	if answer.Error != nil {
		return nil, s.failed(answer.Error)
	}
	result := answer.Result
	switch result["resultType"] {
	case "input_required":
		out := map[string]any{}
		if requests, ok := result["inputRequests"]; ok {
			out["requests"] = requests
		}
		if state, ok := result["requestState"]; ok {
			out["state"] = state
		}
		return out, nil
	case nil, "complete":
	default:
		return nil, plugin.Errorf(plugin.Failed, "the server %s answered with a result of type %v, which MCP does not have", s.name, result["resultType"])
	}
	content, err := s.content(result["content"])
	if err != nil {
		return nil, err
	}
	if result["isError"] == true {
		var said []string
		for _, item := range content {
			if text, ok := item.(string); ok {
				said = append(said, text)
			}
		}
		if len(said) == 0 {
			said = []string{"the tool failed, and gave no text"}
		}
		return nil, plugin.Errorf(plugin.Failed, "%s", strings.Join(said, "\n"))
	}
	out := map[string]any{"content": content}
	if structured, ok := result["structuredContent"]; ok {
		out["data"] = structured
	}
	return out, nil
}

// content gives what a tool gave as a script holds it: text as text, an
// image, audio or a file as a blob, and a link as { link, name }.
func (s *server) content(v any) ([]any, error) {
	items, _ := v.([]any)
	out := make([]any, 0, len(items))
	for _, item := range items {
		part, _ := item.(map[string]any)
		switch part["type"] {
		case "text":
			out = append(out, part["text"])
		case "image", "audio":
			blob, err := s.blob(part["data"], part["mimeType"])
			if err != nil {
				return nil, err
			}
			out = append(out, blob)
		case "resource_link":
			out = append(out, map[string]any{"link": part["uri"], "name": part["name"]})
		case "resource":
			resource, _ := part["resource"].(map[string]any)
			if text, ok := resource["text"]; ok {
				out = append(out, text)
				continue
			}
			blob, err := s.blob(resource["blob"], resource["mimeType"])
			if err != nil {
				return nil, err
			}
			out = append(out, blob)
		default:
			out = append(out, part)
		}
	}
	return out, nil
}

func (s *server) blob(encoded, typ any) (*plugin.Blob, error) {
	text, _ := encoded.(string)
	contents, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, plugin.Errorf(plugin.Failed, "the server %s gave contents that are not base64: %v", s.name, err)
	}
	kind, _ := typ.(string)
	return plugin.NewBlob(kind, contents), nil
}

// data says what is wrong with a value that is sent to a server, which
// takes data and nothing else.
func data(what string, v any) error {
	switch v := v.(type) {
	case nil, string, float64, bool:
		return nil
	case map[string]any:
		for _, item := range v {
			if err := data(what, item); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, item := range v {
			if err := data(what, item); err != nil {
				return err
			}
		}
		return nil
	case *plugin.Secret:
		return plugin.Errorf(plugin.BadInput, "a handle to a secret cannot be in a tool's %s", what)
	case *plugin.Blob:
		return plugin.Errorf(plugin.BadInput, "a blob cannot be in a tool's %s: they are text, numbers, true or false, lists and maps", what)
	}
	return plugin.Errorf(plugin.BadInput, "a tool's %s are text, numbers, true or false, lists and maps, and not bytes", what)
}

// headerValue writes a value as a header carries it: as it is, or in
// base64 where it is not plain text that a header can hold.
func headerValue(v string) string {
	plain := v == strings.TrimSpace(v) && !(strings.HasPrefix(v, "=?base64?") && strings.HasSuffix(v, "?="))
	for _, c := range []byte(v) {
		if (c < 0x20 || c > 0x7e) && c != '\t' {
			plain = false
		}
	}
	if plain {
		return v
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(v)) + "?="
}

// mirrored gives the headers that carry the arguments a tool's schema
// marks with x-mcp-header. An argument that is not there has none.
func mirrored(schema map[string]any, args map[string]any) map[string]string {
	out := map[string]string{}
	var walk func(schema, value map[string]any)
	walk = func(schema, value map[string]any) {
		properties, _ := schema["properties"].(map[string]any)
		for key, sub := range properties {
			inner, _ := sub.(map[string]any)
			v, given := value[key]
			if inner == nil || !given || v == nil {
				continue
			}
			if name, ok := inner["x-mcp-header"].(string); ok {
				text := ""
				switch v := v.(type) {
				case string:
					text = v
				case float64:
					text = strconv.FormatFloat(v, 'f', -1, 64)
				case bool:
					text = strconv.FormatBool(v)
				}
				out["Mcp-Param-"+name] = headerValue(text)
			}
			if object, ok := v.(map[string]any); ok {
				walk(inner, object)
			}
		}
	}
	walk(schema, args)
	return out
}

// mirrorable says whether every x-mcp-header in a schema can be carried:
// each on a property reached only through properties, of a type a header
// can carry, named as a header can be, and no two named alike.
func mirrorable(schema map[string]any) bool {
	var all func(v any) int
	all = func(v any) int {
		n := 0
		switch v := v.(type) {
		case map[string]any:
			if _, marked := v["x-mcp-header"]; marked {
				n++
			}
			for _, item := range v {
				n += all(item)
			}
		case []any:
			for _, item := range v {
				n += all(item)
			}
		}
		return n
	}
	seen := map[string]bool{}
	reached, fine := 0, true
	var walk func(map[string]any)
	walk = func(s map[string]any) {
		properties, _ := s["properties"].(map[string]any)
		for _, sub := range properties {
			inner, _ := sub.(map[string]any)
			if inner == nil {
				continue
			}
			if marked, ok := inner["x-mcp-header"]; ok {
				reached++
				name, _ := marked.(string)
				kind, _ := inner["type"].(string)
				if !token(name) || seen[strings.ToLower(name)] || (kind != "string" && kind != "integer" && kind != "boolean") {
					fine = false
				}
				seen[strings.ToLower(name)] = true
			}
			walk(inner)
		}
	}
	walk(schema)
	return fine && reached == all(schema)
}

// token says whether a text is a header's name.
func token(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}

const page = `mcp: this plugin is a client of an MCP server: it lists the server's tools and calls them.

  mcp.connect(address, { headers, client, timeout })   returns a server, over the web
  mcp.start(program, { args, env, dir, timeout })       returns a server, a program it starts
  server.tools()                                        returns [{ name, does, args }]
  server.call(name, { args, responses, state })         returns { content, data }, or { requests, state }
  server.close()                                        closes the connection, or stops the program

address is a URL that starts https:// or http://. program is as for exec.run
(` + "`corbie manual exec`" + `).

Options:
  headers     headers sent with every request, as written; a value may be a handle
  client      { id, secret } or { id, key }: signs in with these OAuth client
              credentials, each a handle; and scope, if the server wants one
  args, env, dir   as for exec.run
  timeout     seconds, or text such as "5m", for connecting and for each call;
              default 1m
  args        the tool's arguments, a map
  responses   the responses to a server's requests for input, by their names
  state       the state the server gave with its requests for input

- The plugin speaks MCP 2026-07-28, and the earlier version a server has if
  it does not speak that one.
- tools gives the server's tools in the form an agent's list: job returns
  (` + "`corbie manual agent`" + `). args is the tool's input schema as the server
  wrote it. Over the web, a tool that marks an argument for a header where
  no header can carry it is left out.
- call gives content as a list: text as text, an image, audio or a file as
  a blob, and a link as { link, name }. data is the tool's structured
  content, if it gave one.
- A server that needs input to finish a call gives { requests, state }. The
  same call made with { responses, state }, in this job or a later one,
  carries on. The plugin tells a server it takes requests made with a form,
  and the script gives the responses.
- With client, the plugin finds the server's sign-in server from the server,
  gets a token, and sends it to that server and no other.
- A program the plugin started is stopped when the job ends.
- The plugin follows no redirect, and does not speak the HTTP+SSE transport
  of MCP 2024-11-05.

Errors:
  failed      the tool reported an error, and the message is its text; or
              the server answered with an error
  bad_input   the server refused the request: a tool it does not have, or
              arguments it does not take
  not_found   the program was not found
  refused     the server turned the connection or the sign-in away
  timeout     the server did not answer within the timeout

Needs net:host for the server, and with client for its sign-in server; or
exec:program for a program. A handle needs one of these.
  secret:NAME@net:host        in a header, or a client's secret to the sign-in server
  secret:NAME@exec:program    in a program's args or env
  secret:NAME@*               a client's key, which the plugin signs with

This script lists the tools of a server on the web and calls one, with a token
in a header.

    allow: [net:notes.example.com, "secret:NOTES_TOKEN@net:notes.example.com"]

    const server = mcp.connect("https://notes.example.com/mcp", {
      headers: { Authorization: secrets.text` + "`Bearer ${secrets.get(\"NOTES_TOKEN\")}`" + ` },
    })
    const names = server.tools().map((tool) => tool.name)   // ["add_note", "find_notes"]
    return server.call("add_note", { args: { text: "buy tea" } })   // gives { content: ["added"] }

This script lists the tools of a server it starts, and calls one.

    allow: [exec:notes-server, "secret:NOTES_KEY@exec:notes-server"]

    const server = mcp.start("notes-server", { env: { NOTES_KEY: secrets.get("NOTES_KEY") } })
    const names = server.tools().map((tool) => tool.name)   // ["add_note", "find_notes"]
    const found = server.call("find_notes", { args: { text: "tea" } })
    server.close()
    return found                        // gives { content: ["2 notes: tea at 4, tea at 5"] }
`
