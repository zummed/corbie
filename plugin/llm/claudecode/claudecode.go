// Package claudecode is the model service that is a program: it starts
// Claude Code on the runner's machine and has it write one reply. It is an
// extension of its own; a Corbie built without it has only the services on
// the web.
package claudecode

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/plugin/llm"
)

// Name is the service's name, and Program the program it starts.
const (
	Name    = "claude-code"
	Program = "claude"
)

func init() {
	llm.Register(&llm.Service{Name: Name, Program: Program,
		Known:   []string{"fable", "opus", "sonnet", "haiku"},
		Tools:   []string{"WebSearch", "WebFetch", "Read", "Glob", "Grep", "Edit", "Write", "NotebookEdit", "Bash", "Skill"},
		Efforts: []string{"low", "medium", "high", "xhigh", "max"},
		Accepts: append(append([]llm.Accept{}, llm.Images...), llm.Documents...),
		Needs:   needs, Shapes: true, Chat: chat})
}

// needs says what a tool of the program's own could use. Corbie does not
// run such a tool and cannot hold it to a scope, so it may be switched on
// only where the whole of what it could use is allowed.
func needs(tool string) string {
	switch tool {
	case "WebSearch", "WebFetch":
		return "network"
	case "Read", "Glob", "Grep", "Edit", "Write", "NotebookEdit":
		return "disk"
	}
	// A shell, a skill, or anything this does not know could do anything.
	return "all"
}

// structured is the tool the program answers through when it is given a
// shape to answer in.
// signIn is the variable the program reads a key from: a subscription's
// token, as claude setup-token gives one, has a name of its own, and
// anything else is a key of the API.
func signIn(key string) string {
	if strings.HasPrefix(key, "sk-ant-oat") {
		return "CLAUDE_CODE_OAUTH_TOKEN"
	}
	return "ANTHROPIC_API_KEY"
}

const structured = "StructuredOutput"

func chat(job *plugin.Job, c *llm.Call) (*llm.Reply, error) {
	// A tool of the program's own is refused before the program starts.
	for _, tool := range c.Own {
		if err := job.Whole(needs(tool)); err != nil {
			return nil, err
		}
	}
	// The system text, which grows with every tool, is given in a file:
	// a command line has a limit, and anyone on the machine can read it.
	prompt, err := job.File("claude-system-*.md", []byte(system(c)))
	if err != nil {
		return nil, err
	}
	schema, _ := json.Marshal(shape(c), json.Deterministic(true))
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--include-partial-messages", "--system-prompt-file", prompt, "--json-schema", string(schema),
		"--strict-mcp-config", "--no-session-persistence", "--model", c.Model,
		"--tools", strings.Join(c.Own, ",")}
	if len(c.Own) > 0 {
		args = append(args, "--allowed-tools", strings.Join(c.Own, ","))
	}
	if !slices.Contains(c.Own, "Skill") {
		// With no skills asked for, nothing the machine's user has set up
		// for the program is loaded: no skills, no hooks, no instructions.
		args = append(args, "--safe-mode")
	}
	if c.Effort != "" {
		args = append(args, "--effort", c.Effort)
	}
	// The program signs in with the key, if the call has one, and
	// otherwise as the runner's user is signed in.
	opt := plugin.ProgramOptions{Args: args}
	if handle, ok := c.Key.(*plugin.Secret); ok {
		key, err := job.Reveal(handle, "exec:"+Program)
		if err != nil {
			return nil, err
		}
		opt.Env, opt.Secrets = []string{signIn(key) + "=" + key}, []*plugin.Secret{handle}
	} else {
		home, _, err := job.Variable(plugin.HomeVariable)
		if err != nil {
			return nil, err
		}
		opt.Env = []string{plugin.HomeVariable + "=" + home}
	}
	// It runs in a fresh directory of its own.
	dir, err := job.Directory("claude-*")
	if err != nil {
		return nil, err
	}
	opt.Dir = dir
	said, err := conversation(job, c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(job.Context, c.Limit)
	defer cancel()
	cmd, err := job.Program(ctx, Program, opt)
	if err != nil {
		return nil, err
	}
	// A program that does not read what it is sent, or does not end when
	// it is stopped, does not hold the call.
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	errs := &plugin.Whole{}
	cmd.Stderr = &plugin.Draining{To: errs}
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, plugin.Errorf(plugin.NotFound, "the program %q could not be started: %v", Program, err)
	}
	go func() {
		stdin.Write(said)
		stdin.Close()
	}()
	result, arrived, err := read(job, stdout)
	waited := cmd.Wait()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return nil, plugin.Errorf(plugin.TimedOut, "the model did not answer within %s", c.Limit)
	case err != nil:
		return nil, err
	case result == nil:
		return nil, plugin.Errorf(plugin.Failed, "%s ended without an answer (%v): %s", Program, waited, tail(job.Mask(errs.Text())))
	}
	// A reply that ran out of room is passed on as that, with what had
	// arrived of it: the program has no whole answer to give then.
	stop := llm.StopEnd
	if result.StopReason == "max_tokens" || result.StopReason == "model_context_window_exceeded" {
		stop = llm.StopLength
	}
	if result.IsError && stop != llm.StopLength {
		why := strings.Join(result.Errors, "; ")
		if why == "" {
			why = result.Result
		}
		return nil, plugin.Errorf(plugin.Refused, "%s gave an error (%s): %s", Program, result.Subtype, tail(why))
	}
	var out struct {
		Text  string `json:"text"`
		Calls []struct {
			Name  string `json:"name"`
			Input any    `json:"input"`
		} `json:"tool_calls"`
		Answer jsontext.Value `json:"answer"`
	}
	if json.Unmarshal(result.Structured, &out) != nil || len(result.Structured) == 0 {
		if stop != llm.StopLength {
			return nil, plugin.Errorf(plugin.Refused, "%s gave no answer in the shape it was asked for: %s", Program, tail(result.Result))
		}
		out.Text = arrived
	}
	// A final reply in the shape that was wanted is the reply: the words
	// beside it were for a watcher.
	if len(out.Calls) == 0 && len(out.Answer) > 0 && string(out.Answer) != "null" {
		out.Text = string(out.Answer)
	}
	reply := &llm.Reply{Text: out.Text, Stop: stop, Usage: map[string]any{
		"input":  result.Usage.Input + result.Usage.CacheMade + result.Usage.CacheRead,
		"output": result.Usage.Output,
	}}
	for _, made := range out.Calls {
		// The program gives its requests no names. Each is given one that
		// no other request in any conversation has.
		id := make([]byte, 8)
		rand.Read(id)
		if made.Input == nil {
			made.Input = map[string]any{}
		}
		reply.Calls = append(reply.Calls, map[string]any{"id": "call_" + hex.EncodeToString(id), "name": made.Name, "args": made.Input})
	}
	return reply, nil
}

// line is one line of what the program prints: a piece of its reply as it
// is written, a message of the model's, or the result at the end.
type line struct {
	Type string `json:"type"`
	// StopReason is why the model stopped, on the result; Message carries
	// it on a message of the model's.
	StopReason string `json:"stop_reason"`
	Message    struct {
		StopReason string `json:"stop_reason"`
	} `json:"message"`
	Event struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
			JSON string `json:"partial_json"`
		} `json:"delta"`
		Block struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content_block"`
	} `json:"event"`
	Subtype    string         `json:"subtype"`
	IsError    bool           `json:"is_error"`
	Result     string         `json:"result"`
	Errors     []string       `json:"errors"`
	Structured jsontext.Value `json:"structured_output"`
	Usage      struct {
		Input     float64 `json:"input_tokens"`
		Output    float64 `json:"output_tokens"`
		CacheMade float64 `json:"cache_creation_input_tokens"`
		CacheRead float64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// read follows what the program prints to its end and gives its result,
// and the answer's text as far as it arrived. The reply's text is emitted
// as it is written, and so is each use of a tool of the program's own, so
// that the record shows what Corbie did not run. Why the model stopped is
// read from the result, or from the model's last message where the result
// does not say.
func read(job *plugin.Job, from io.Reader) (*line, string, error) {
	var result *line
	arrived, stopped := "", ""
	answering := map[int]*strings.Builder{} // the blocks that are the answer, as they arrive
	told := 0                               // how much of the answer's text has been emitted
	said := job.Telling("text")
	defer said.End()
	in := bufio.NewReaderSize(from, 64<<10)
	for {
		text, err := in.ReadString('\n')
		var l line
		if json.Unmarshal([]byte(strings.TrimSpace(text)), &l) == nil {
			switch ev := l.Event; {
			case l.Type == "result" && result == nil:
				result = &l
			case l.Type == "assistant" && l.Message.StopReason != "":
				stopped = l.Message.StopReason
			case l.Type != "stream_event":
			case ev.Type == "content_block_start" && ev.Block.Type == "tool_use" && ev.Block.Name == structured:
				answering[ev.Index] = &strings.Builder{}
			case ev.Type == "content_block_start" && ev.Block.Type == "tool_use":
				if job.Channel != nil {
					job.Channel.Emit("own", map[string]any{"tool": ev.Block.Name})
				}
			case ev.Type == "content_block_delta" && ev.Delta.Type == "input_json_delta" && answering[ev.Index] != nil:
				answering[ev.Index].WriteString(ev.Delta.JSON)
				if so := textSoFar(answering[ev.Index].String()); len(so) > told {
					said.Add(so[told:])
					told, arrived = len(so), so
				}
			}
		}
		if err == io.EOF {
			if result != nil && result.StopReason == "" {
				result.StopReason = stopped
			}
			return result, arrived, nil
		}
		if err != nil {
			return nil, "", plugin.Errorf(plugin.Failed, "what %s printed could not be read: %v", Program, err)
		}
	}
}

var textStarts = regexp.MustCompile(`"text"\s*:\s*"`)

// textSoFar gives as much of the answer's text as has arrived, from the
// JSON of the answer as far as it has been written.
func textSoFar(arrived string) string {
	at := textStarts.FindStringIndex(arrived)
	if at == nil {
		return ""
	}
	raw := arrived[at[1]:]
	end := len(raw)
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' {
			i++
		} else if raw[i] == '"' {
			end = i
			break
		}
	}
	raw = raw[:end]
	// What is cut in the middle of an escape is left for the next piece.
	for cut := 0; cut < 7 && cut <= len(raw); cut++ {
		var text string
		if json.Unmarshal([]byte(`"`+raw[:len(raw)-cut]+`"`), &text) == nil {
			return text
		}
	}
	return ""
}

// tail is the end of what a program said went wrong.
func tail(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 400 {
		return "…" + text[len(text)-400:]
	}
	return text
}

// shape is the shape the program is asked to answer in: its text, and the
// tools it asks for where it is offered any.
func shape(c *llm.Call) map[string]any {
	object := func(required []any, properties map[string]any) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
	}
	text := map[string]any{"type": "string"}
	// Where a shape of answer is wanted, the final reply has a place of its
	// own, which the program holds to that shape.
	with := func(properties map[string]any) map[string]any {
		if len(c.Shape) > 0 {
			properties["answer"] = llm.Schema(c.Shape)
		}
		return properties
	}
	if len(c.Tools) == 0 {
		return object([]any{"text"}, with(map[string]any{"text": text}))
	}
	var names []any
	for _, tool := range c.Tools {
		names = append(names, field(tool, "name"))
	}
	call := object([]any{"name", "input"}, map[string]any{
		"name": map[string]any{"type": "string", "enum": names}, "input": map[string]any{"type": "object"}})
	return object([]any{"text", "tool_calls"}, with(map[string]any{"text": text, "tool_calls": map[string]any{"type": "array", "items": call}}))
}

// system is the system text and, where tools are offered, what they are
// and the one way to ask for them. The program is not given them as tools
// of its own: Corbie runs them.
func system(c *llm.Call) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(c.System, "\n"))
	if c.System == "" {
		b.WriteString("You are a helpful assistant.")
	}
	if len(c.Shape) > 0 {
		b.WriteString("\n\n# Your final reply\n\n")
		b.WriteString("Your final reply is data, not prose. Put it in the `answer` field of your structured output, in the shape that field's schema gives. ")
		b.WriteString("A reply without `answer` is not final. Leave `answer` out for as long as you are still asking for tools.\n")
	}
	if len(c.Tools) == 0 {
		return b.String()
	}
	b.WriteString("\n\n# Tools\n\n")
	b.WriteString("You can use the tools listed below, but you cannot call them directly: they are not tools of yours, and calling one directly fails. ")
	b.WriteString("The ONLY way to call them is to put the calls in the `tool_calls` field of your structured output, each with the tool's `name` and its `input`. ")
	b.WriteString("Put anything you want to say in `text`. Each call's result comes back to you in the next message. ")
	b.WriteString("When your reply is final, leave `tool_calls` empty.\n")
	for _, tool := range c.Tools {
		fmt.Fprintf(&b, "\n## %v\n\n%v\n\nInput schema: %s\n", field(tool, "name"), field(tool, "does"), must(llm.Schema(field(tool, "args"))))
	}
	return b.String()
}

// conversation writes the whole conversation as the one message the
// program is sent: who said what, in order, with each file where it was
// said, and then the request for the next reply.
func conversation(job *plugin.Job, c *llm.Call) ([]byte, error) {
	var parts []any
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			parts = append(parts, map[string]any{"type": "text", "text": text.String()})
			text.Reset()
		}
	}
	files := func(holder any) error {
		for _, file := range llm.Files(holder) {
			data, err := file.Data(job)
			if err != nil {
				return err
			}
			fmt.Fprintf(&text, "%s:\n", file.Name)
			flush()
			kind := "document"
			if strings.HasPrefix(file.Blob.Type, "image/") {
				kind = "image"
			}
			parts = append(parts, map[string]any{"type": kind, "source": map[string]any{"type": "base64", "media_type": file.Blob.Type, "data": data}})
		}
		return nil
	}
	for _, part := range c.Said {
		switch field(part, "role") {
		case "assistant":
			fmt.Fprintf(&text, "Assistant:\n%s\n", llm.Text(field(part, "content")))
			for _, made := range list(field(part, "calls")) {
				fmt.Fprintf(&text, "Assistant called %v (id %v) with %s\n", field(made, "name"), field(made, "id"), must(field(made, "args")))
			}
		case "tool":
			fmt.Fprintf(&text, "User:\n")
			for _, result := range list(field(part, "results")) {
				shown, failed := llm.ResultText(result)
				what := "Result of"
				if failed {
					what = "Error from"
				}
				fmt.Fprintf(&text, "%s %v:\n%s\n", what, field(result, "id"), shown)
				if err := files(result); err != nil {
					return nil, err
				}
			}
		default:
			fmt.Fprintf(&text, "User:\n%s\n", llm.Text(field(part, "content")))
			if err := files(part); err != nil {
				return nil, err
			}
		}
		text.WriteString("\n")
	}
	text.WriteString("Write the assistant's next reply to the conversation above.\n")
	flush()
	message, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": parts}}, json.Deterministic(true))
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "the conversation cannot be sent: %v", err)
	}
	return append(message, '\n'), nil
}

func must(v any) string {
	data, _ := json.Marshal(v, json.Deterministic(true))
	return string(data)
}

func field(m any, name string) any {
	if mm, ok := m.(map[string]any); ok {
		return mm[name]
	}
	return nil
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}
