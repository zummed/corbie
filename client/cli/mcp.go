package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/manual"
)

// cmdMCP serves two tools to a model over standard input and output: the
// manual, and corbie's command line. The model may run only the commands
// the server was started with permission for, with the config the server
// was started on. A command allowed may be a program's, as in --allow
// "rook init": the program's pages join the manual, and its commands are
// run as corbie <name> runs them. The server needs nothing running and
// keeps nothing.
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
	t := e.table
	if t.guests, err = e.guests(allow); err != nil {
		return err
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
				"serverInfo": map[string]any{"name": "corbie", "version": version()}, "instructions": t.briefing()}
		case "tools/list":
			result = map[string]any{"tools": t.tools(allow)}
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
				text, failed := e.call(ctx, t, name, arguments, allow)
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

// briefing is what a model is told when it connects to corbie mcp: how to
// work for the person who asked it.
const briefing = `Corbie runs machines. A machine is a YAML definition of type flow, statechart, agent or decision, and each of its jobs is a JavaScript script, a machine, or a send. A person has asked you to write, change, run or set up a machine.

1. Read the manual with the manual tool before you write or change a definition or a script. Read the pages "corbie" and "first-machine" first. Then read "definitions", "scripts", "permissions", the page of the machine type ("flow", "statechart" or "agent") and the page of each plugin the script calls. The manual gives every field and every call, so never guess one.
2. Give each job only the permissions it needs, in its allow list, and tell the person what you allowed.
3. Run what you wrote with the corbie tool and read the result. The command "test" runs a definition or a script with no setup. A refusal names the line that allows what was refused.
4. Run Corbie's commands with the corbie tool, not in a shell.
5. Tell the person what the run printed.`

// briefing is what a model is told when it connects: corbie's briefing,
// then the page named after each program it may run a command of, if the
// program has one.
func (t table) briefing() string {
	text := briefing
	for _, g := range t.guests {
		if at := slices.IndexFunc(g.pages, func(p manual.Page) bool { return p.Name == g.name }); at >= 0 {
			text += "\n\n" + strings.TrimRight(g.pages[at].Text, "\n")
		}
	}
	return text
}

func (e *Env) respond(v any) {
	data, _ := json.Marshal(v, json.Deterministic(true))
	fmt.Fprintln(e.Out, string(data))
}

// tools describes the two tools to the model: the manual, and the command
// line.
func (t table) tools(allow []string) []any {
	var commands []string
	for i := range t.commands {
		c := &t.commands[i]
		if !slices.Contains(allow, c.Name) {
			continue
		}
		line := fmt.Sprintf("%s — %s", strings.TrimSpace(t.named(c)+" "+c.Usage), c.Summary)
		if c.Stays {
			// What a model is told about a command that stays says so.
			line += " (this call stays until the run ends, or until it is stopped: a run waiting on a ticket cannot be answered from inside the same call)"
		}
		commands = append(commands, line)
	}
	// A program's command is described by its page, whose first line is
	// the command as it is typed.
	for _, g := range t.guests {
		for _, page := range g.commands() {
			if slices.Contains(allow, strings.TrimPrefix(page.Name, "corbie ")) {
				usage, _, _ := strings.Cut(page.Text, "\n")
				commands = append(commands, fmt.Sprintf("%s — %s", usage, page.Summary))
			}
		}
	}
	results := "Results are JSON."
	if len(t.guests) > 0 {
		results = "Corbie's results are JSON, and a program's are as the program prints them."
	}
	allowed := "No commands are allowed to you."
	if len(commands) > 0 {
		allowed = "The commands allowed to you:\n" + strings.Join(commands, "\n") +
			"\nrun show and run start also stay until the run ends when given --wait or --watch, and run ls --watch stays until it is stopped."
	}
	return []any{
		map[string]any{"name": "manual",
			"description": "The manual of corbie, with the pages of each program a command is allowed of. With no page it lists the pages; with a page it gives that page. Read a plugin's or a machine type's page before writing a script or a definition that uses it.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"page": map[string]any{"type": "string"}}}},
		map[string]any{"name": "corbie",
			"description": "Runs one command of corbie and gives back its result, what it said along the way, and which of three ways it ended: 0 success, 1 failure, 2 a mistake in how it was written. Give the arguments as a list, without the word corbie. " + results + "\n" + allowed,
			"inputSchema": map[string]any{"type": "object", "required": []any{"args"},
				"properties": map[string]any{"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}},
	}
}

// call runs one tool for the model, from the table.
func (e *Env) call(ctx context.Context, t table, name string, arguments map[string]any, allow []string) (string, bool) {
	switch name {
	case "manual":
		page, _ := arguments["page"].(string)
		// The tool reads a page and starts nothing: it runs corbie manual
		// itself, never a command whose name begins with manual, and
		// takes no flag.
		if strings.HasPrefix(strings.TrimSpace(page), "-") || strings.Contains(page, " -") {
			return "the page is a page's name, as the list gives it", true
		}
		cmd, _ := t.find([]string{"manual"})
		var out, errOut bytes.Buffer
		status := t.do(ctx, cmd, strings.Fields(page), strings.NewReader(""), &out, &errOut)
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
		// A program's command is found by its whole name among the
		// program's pages, and runs as corbie <name> runs it, with the
		// server's config and store.
		if at := slices.IndexFunc(t.guests, func(g guest) bool { return len(args) > 0 && g.name == args[0] }); at >= 0 {
			g := t.guests[at]
			whole := g.find(args)
			if whole == "" {
				return fmt.Sprintf("%q is not a command", strings.Join(args, " ")), true
			}
			if !slices.Contains(allow, whole) {
				return fmt.Sprintf("the command %q is not one this server was started with permission for", whole), true
			}
			if namesConfigOrStore(args) {
				return "a call may not name a config or a store: the server's own are used", true
			}
			var out, errOut bytes.Buffer
			status := runProgram(ctx, g.path, args[1:], e.programEnv(), strings.NewReader(""), &out, &errOut)
			return ran(status, out, errOut)
		}
		// The name a command is allowed by and the command that then runs
		// are found by one reading of the command line, and a command that
		// is refused is refused before any part of it runs.
		cmd, rest := t.find(args)
		if cmd == nil {
			return fmt.Sprintf("%q is not a command", strings.Join(args, " ")), true
		}
		if !slices.Contains(allow, cmd.Name) {
			return fmt.Sprintf("the command %q is not one this server was started with permission for", cmd.Name), true
		}
		// corbie start's line names programs beside roles: it is read as
		// corbie start reads it, and a program in it is refused.
		if cmd.Name == "start" {
			_, named, _ := items(cmd.env(ctx, nil, io.Discard, io.Discard), rest)
			for _, it := range named {
				if !slices.Contains(roles, it.name) {
					return fmt.Sprintf("%q is a program on PATH, and corbie start never starts a program for a model", it.name), true
				}
			}
		}
		if namesConfigOrStore(rest) {
			return "a call may not name a config or a store: the server's own are used", true
		}
		// A config or a store the server's line named is given to the
		// command; one the environment names reaches it as it is.
		full := []string{"--json"}
		if e.Given("config") {
			full = append(full, "--config", e.config)
		}
		if e.Given("store") {
			full = append(full, "--store", e.store)
		}
		var out, errOut bytes.Buffer
		status := t.do(ctx, cmd, append(full, rest...), strings.NewReader(""), &out, &errOut)
		return ran(status, out, errOut)
	}
	return fmt.Sprintf("there is no tool named %q", name), true
}

// namesConfigOrStore says whether a command line names a config or a
// store.
func namesConfigOrStore(args []string) bool {
	return slices.ContainsFunc(args, func(arg string) bool {
		flag := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		return strings.HasPrefix(arg, "-") && (flag == "config" || flag == "store")
	})
}

// ran is the answer to a call of a command: its result, what it said along
// the way, and how it ended.
func ran(status int, out, errOut bytes.Buffer) (string, bool) {
	answer, _ := json.Marshal(map[string]any{"ended": status, "result": out.String(), "said": errOut.String()}, json.Deterministic(true))
	return string(answer), status != 0
}

// A guest is a program on PATH, corbie-<name>, that the server's line
// allows a command of. The server reads its manual once, when it starts:
// the owner's line runs it, never a model.
type guest struct {
	name, path string
	pages      []manual.Page // the program's pages that Corbie's manual lacks
}

// guests are the programs the allowed commands name, each with its pages.
// A command allowed is Corbie's or a program's, as the program's manual
// names it.
func (e *Env) guests(allow []string) ([]guest, error) {
	var out []guest
	for _, name := range allow {
		if slices.ContainsFunc(e.table.commands, func(c Command) bool { return c.Name == name }) {
			continue
		}
		word, _, _ := strings.Cut(name, " ")
		path, err := exec.LookPath("corbie-" + word)
		if word == "" || taken(word) || err != nil {
			return nil, Mistake("--allow %q: that is neither a command nor a program corbie-%s on PATH (corbie --help lists them)", name, word)
		}
		at := slices.IndexFunc(out, func(g guest) bool { return g.name == word })
		if at < 0 {
			g, err := e.readGuest(word, path)
			if err != nil {
				return nil, err
			}
			out, at = append(out, g), len(out)
		}
		if out[at].find(strings.Fields(name)) != name {
			return nil, Mistake("--allow %q: that is not a command of corbie %s (corbie %s --help lists them)", name, word, word)
		}
	}
	return out, nil
}

// readGuest reads a program's manual, as corbie <name> manual --json
// prints it, and keeps the pages Corbie's manual lacks, leaving out the
// commands every program is given.
func (e *Env) readGuest(name, path string) (guest, error) {
	ctx, stop := context.WithTimeout(e.ctx, 30*time.Second)
	defer stop()
	var out, errOut bytes.Buffer
	if status := runProgram(ctx, path, []string{"manual", "--json"}, e.programEnv(), strings.NewReader(""), &out, &errOut); status != 0 {
		return guest{}, fmt.Errorf("corbie %s manual --json ended %d: %s", name, status, strings.TrimSpace(errOut.String()))
	}
	var pages []manual.Page
	if err := json.Unmarshal(out.Bytes(), &pages); err != nil {
		return guest{}, fmt.Errorf("corbie %s manual --json printed no manual: %v", name, err)
	}
	own := manual.Pages()
	g := guest{name: name, path: path}
	for _, page := range pages {
		given := slices.ContainsFunc(forPrograms(name), func(c Command) bool { return page.Name == "corbie "+name+" "+c.Name })
		if !given && !slices.ContainsFunc(own, func(p manual.Page) bool { return p.Name == page.Name }) {
			g.pages = append(g.pages, page)
		}
	}
	return g, nil
}

// commands are the pages of the program's commands, each named as it is
// typed: corbie rook init.
func (g guest) commands() []manual.Page {
	var out []manual.Page
	for _, page := range g.pages {
		if page.Group == "Commands" && (page.Name == "corbie "+g.name || strings.HasPrefix(page.Name, "corbie "+g.name+" ")) {
			out = append(out, page)
		}
	}
	return out
}

// find gives the whole name of the program's command a command line
// begins with, as rook init, or "" if it begins none: the longest name
// that matches.
func (g guest) find(args []string) string {
	found := ""
	for _, page := range g.commands() {
		words := strings.Fields(strings.TrimPrefix(page.Name, "corbie "))
		if len(words) <= len(args) && slices.Equal(words, args[:len(words)]) && len(words) > len(strings.Fields(found)) {
			found = strings.Join(words, " ")
		}
	}
	return found
}

// forPrograms are the commands every program cli.Program reads is given
// beside its own: the manual, with the program's commands among Corbie's
// pages, and the version.
func forPrograms(name string) []Command {
	return []Command{
		{Name: "manual", Usage: "[page]", Summary: "print a page of the manual, or list the pages", Run: cmdManual, Help: `
With no page it lists the pages: Corbie's, and this program's. It reads no
config and no store.`},
		{Name: "version", Usage: "", Summary: "print the version of the program and of the Corbie it is built with", Run: cmdProgramVersion},
	}
}

// cmdProgramVersion prints a program's version and its Corbie's.
func cmdProgramVersion(e *Env, args []string) error {
	if _, err := e.Parse(args); err != nil {
		return err
	}
	program := strings.ReplaceAll(e.table.name, " ", "-")
	holds := map[string]any{"version": version(), "corbie": corbieVersion()}
	return e.Print(holds, func() string {
		return fmt.Sprintf("%s %s\ncorbie %s\n", program, version(), corbieVersion())
	})
}
