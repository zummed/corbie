package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
)

// cmdMCP serves two tools to a model over standard input and output: the
// manual, and the command line. The model may run only the commands the
// server was started with permission for, with the config the server was
// started on. The server needs nothing running and keeps nothing.
func cmdMCP(e *Env, args []string) error {
	var allow list
	e.Flags.Var(&allow, "allow", "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return Mistake("mcp takes only flags: --allow <command>, as many times as there are commands to allow")
	}
	for _, name := range allow {
		if !slices.ContainsFunc(Commands, func(c Command) bool { return c.Name == name }) {
			return Mistake("--allow %q: that is not a command (corbie --help lists them)", name)
		}
	}
	type request struct {
		ID     any            `json:"id,omitzero"`
		Method string         `json:"method"`
		Params map[string]any `json:"params,omitzero"`
	}
	// Each call runs beside the reading, so a call that stays keeps no
	// other from being answered, and the host's notice that it has stopped
	// one is read while that one runs. When the host closes the connection,
	// the calls already running finish and are answered before the server
	// stops.
	var calls sync.WaitGroup
	var mu sync.Mutex // one answer is written at a time, and running is read and changed under it
	running := map[string]context.CancelFunc{}
	answer := func(id any, field string, v any) {
		mu.Lock()
		defer mu.Unlock()
		e.respond(map[string]any{"jsonrpc": "2.0", "id": id, field: v})
	}
	lines := bufio.NewScanner(e.In)
	lines.Buffer(make([]byte, 1<<20), 64<<20)
	for lines.Scan() {
		var req request
		if json.Unmarshal(lines.Bytes(), &req) != nil || req.Method == "" {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "corbie", "version": version()}, "instructions": briefing}
		case "tools/list":
			result = map[string]any{"tools": e.tools(allow)}
		case "tools/call":
			name, _ := req.Params["name"].(string)
			arguments, _ := req.Params["arguments"].(map[string]any)
			ctx, stop := context.WithCancel(e.ctx)
			call := fmt.Sprint(req.ID)
			mu.Lock()
			running[call] = stop
			mu.Unlock()
			calls.Go(func() {
				defer stop()
				text, failed := e.call(ctx, name, arguments, allow)
				mu.Lock()
				delete(running, call)
				mu.Unlock()
				// A call the host stopped is not answered: the host has
				// stopped listening for it.
				if req.ID != nil && ctx.Err() == nil {
					answer(req.ID, "result", map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": failed})
				}
			})
			continue
		case "notifications/cancelled":
			// The host has stopped waiting for a call: the call ends, as a
			// wait that is cut short does.
			mu.Lock()
			if stop := running[fmt.Sprint(req.Params["requestId"])]; stop != nil {
				stop()
			}
			mu.Unlock()
			continue
		case "ping":
			result = map[string]any{}
		default:
			if req.ID == nil {
				continue // a notice: nothing to answer
			}
			answer(req.ID, "error", map[string]any{"code": -32601, "message": "there is no such method"})
			continue
		}
		if req.ID != nil {
			answer(req.ID, "result", result)
		}
	}
	calls.Wait()
	return lines.Err()
}

// briefing is what a model is told when it connects: how to work for the
// person who asked it.
const briefing = `Corbie runs machines. A machine is a YAML definition of type flow, statechart or agent, and each of its jobs is a JavaScript script or another machine. A person has asked you to write, change, run or set up a machine.

1. Read the manual with the manual tool before you write or change a definition or a script. Read the pages "corbie" and "first-machine" first. Then read "definitions", "scripts", "permissions", the page of the machine type ("flow", "statechart" or "agent") and the page of each plugin the script calls. The manual gives every field and every call, so never guess one.
2. Give each job only the permissions it needs, in its allow list, and tell the person what you allowed.
3. Run what you wrote with the corbie tool and read the result. The command "test" runs a definition or a script with no setup. A refusal names the line that allows what was refused.
4. Run Corbie's commands with the corbie tool, not in a shell.
5. Tell the person what the run printed.`

func (e *Env) respond(v any) {
	data, _ := json.Marshal(v, json.Deterministic(true))
	fmt.Fprintln(e.Out, string(data))
}

// tools describes the two tools to the model.
func (e *Env) tools(allow []string) []any {
	var commands []string
	for _, c := range Commands {
		if !slices.Contains(allow, c.Name) {
			continue
		}
		line := fmt.Sprintf("corbie %s — %s", strings.TrimSpace(c.Name+" "+c.Usage), c.Summary)
		if c.Stays {
			// What a model is told about a command that stays says so.
			line += " (this call stays until the run ends, or until it is stopped: a run waiting on a ticket cannot be answered from inside the same call)"
		}
		commands = append(commands, line)
	}
	allowed := "No commands are allowed to you."
	if len(commands) > 0 {
		allowed = "The commands allowed to you:\n" + strings.Join(commands, "\n") +
			"\nrun show and run submit also stay until the run ends when given --wait or --watch, and run ls --watch stays until it is stopped."
	}
	return []any{
		map[string]any{"name": "manual",
			"description": "Corbie's manual. With no page it lists the pages; with a page it gives that page. Read a plugin's or a machine type's page before writing a script or a definition that uses it.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"page": map[string]any{"type": "string"}}}},
		map[string]any{"name": "corbie",
			"description": "Runs one command of Corbie's command line and gives back its result, what it said along the way, and which of three ways it ended: 0 success, 1 failure, 2 a mistake in how it was written. Give the arguments as a list, without the word corbie. Results are JSON.\n" + allowed,
			"inputSchema": map[string]any{"type": "object", "required": []any{"args"},
				"properties": map[string]any{"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}},
	}
}

// call runs one tool for the model.
func (e *Env) call(ctx context.Context, name string, arguments map[string]any, allow []string) (string, bool) {
	switch name {
	case "manual":
		page, _ := arguments["page"].(string)
		// The tool reads a page and starts nothing: it runs corbie manual
		// itself, never a command whose name begins with manual, and
		// takes no flag.
		if strings.HasPrefix(strings.TrimSpace(page), "-") || strings.Contains(page, " -") {
			return "the page is a page's name, as the list gives it", true
		}
		cmd, _ := own().find([]string{"manual"})
		var out, errOut bytes.Buffer
		status := own().do(ctx, cmd, strings.Fields(page), strings.NewReader(""), &out, &errOut)
		return out.String() + errOut.String(), status != 0
	case "corbie":
		given, _ := arguments["args"].([]any)
		var args []string
		for _, arg := range given {
			text, ok := arg.(string)
			if !ok {
				return "args is a list of texts", true
			}
			args = append(args, text)
		}
		// The name a command is allowed by and the command that then runs
		// are found by one reading of the command line, and a command that
		// is refused is refused before any part of it runs.
		cmd, rest := own().find(args)
		if cmd == nil {
			return fmt.Sprintf("%q is not a command", strings.Join(args, " ")), true
		}
		if !slices.Contains(allow, cmd.Name) {
			return fmt.Sprintf("the command %q is not one this server was started with permission for", cmd.Name), true
		}
		// A program on PATH is never run for a model, and corbie start's
		// line names programs beside roles: it is read as corbie start
		// reads it, and a program in it is refused.
		if cmd.Name == "start" {
			_, named, _ := items(cmd.env(ctx, nil, io.Discard, io.Discard), rest)
			for _, it := range named {
				if !slices.Contains(roles, it.name) {
					return fmt.Sprintf("%q is a program on PATH, and a program is never run for a model", it.name), true
				}
			}
		}
		for _, arg := range rest {
			flag := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
			if strings.HasPrefix(arg, "-") && (flag == "config" || flag == "store") {
				return "a call may not name a config or a store: the server's own are used", true
			}
		}
		full := []string{"--json"}
		if e.config != "" {
			full = append(full, "--config", e.config)
		}
		if e.store != "" {
			full = append(full, "--store", e.store)
		}
		var out, errOut bytes.Buffer
		status := own().do(ctx, cmd, append(full, rest...), strings.NewReader(""), &out, &errOut)
		answer, _ := json.Marshal(map[string]any{"ended": status, "result": out.String(), "said": errOut.String()}, json.Deterministic(true))
		return string(answer), status != 0
	}
	return fmt.Sprintf("there is no tool named %q", name), true
}
