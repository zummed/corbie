package llm

import (
	"strings"

	"github.com/zummed/corbie/plugin"
)

// The conversation a script hands over is a list of parts, each one of:
//
//	{ role: "user", content }                       what the model is told
//	{ role: "assistant", content, calls, kept }     what the model said
//	{ role: "tool", results: [{ id, name, result, error }] }
//
// Each service turns that into its own way of talking and back.

// kinds are how one service writes text, an image and a document as
// content.
type kinds struct {
	text     func(text string) any
	image    func(typ, data string) any
	document func(name, data string) any
}

// attached gives the files a part of the conversation carries as content
// in a service's way: each under the name the model knows it by.
func attached(job *plugin.Job, holder any, k kinds) ([]any, error) {
	var blocks []any
	for _, file := range Files(holder) {
		data, err := file.Data(job)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, k.text(file.Name+":"))
		if strings.HasPrefix(file.Blob.Type, "image/") {
			blocks = append(blocks, k.image(file.Blob.Type, data))
		} else {
			blocks = append(blocks, k.document(file.Name, data))
		}
	}
	return blocks, nil
}

// with gives a part's text, and what it carries beside text, as content.
func with(job *plugin.Job, holder any, said string, k kinds) (any, error) {
	files, err := attached(job, holder, k)
	if err != nil || files == nil {
		return said, err
	}
	return append([]any{k.text(said)}, files...), nil
}

// The types of content the services take beside text, and the largest of
// each, as the services say.
var (
	Images    = []Accept{{"image/png", 5 << 20}, {"image/jpeg", 5 << 20}, {"image/gif", 5 << 20}, {"image/webp", 5 << 20}}
	Documents = []Accept{{"application/pdf", 32 << 20}}
)

func init() {
	efforts := []string{"low", "medium", "high"}
	takes := append(append([]Accept{}, Images...), Documents...)
	Register(&Service{Name: "anthropic", Address: "https://api.anthropic.com", Models: []string{"claude"},
		Known:   []string{"claude-fable-5-1", "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-4-5-20251001"},
		Efforts: efforts, Accepts: takes, Chat: anthropic.chat})
	Register(&Service{Name: "openai", Address: "https://api.openai.com", Models: []string{"gpt", "o1", "o3", "o4"},
		Known:   []string{"gpt-5", "gpt-5-mini"},
		Efforts: efforts, Accepts: takes, Chat: openaiResponses.chat})
	Register(&Service{Name: "openai-chat", Address: "https://api.openai.com",
		Efforts: efforts, Accepts: takes, Chat: openaiChat.chat})
}

var anthropicKinds = kinds{
	text: func(text string) any { return map[string]any{"type": "text", "text": text} },
	image: func(typ, data string) any {
		return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": typ, "data": data}}
	},
	document: func(_, data string) any {
		return map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": data}}
	},
}

var anthropic = &web{
	path: "/v1/messages",
	headers: func(key string) map[string]string {
		return map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}
	},
	request: func(job *plugin.Job, c *Call) (map[string]any, error) {
		var messages []any
		for _, part := range c.Said {
			switch field(part, "role") {
			case "assistant":
				// Parts the service wants back unchanged are sent as they came.
				content := field(part, "kept")
				if content == nil {
					var blocks []any
					if t := text(field(part, "content")); t != "" {
						blocks = append(blocks, map[string]any{"type": "text", "text": t})
					}
					for _, made := range list(field(part, "calls")) {
						blocks = append(blocks, map[string]any{"type": "tool_use", "id": field(made, "id"),
							"name": field(made, "name"), "input": argsOf(field(made, "args"))})
					}
					content = blocks
				}
				messages = append(messages, map[string]any{"role": "assistant", "content": content})
			case "tool":
				var blocks []any
				for _, result := range list(field(part, "results")) {
					shown, failed := resultText(result)
					// What a result carries beside text goes in the result.
					content, err := with(job, result, shown, anthropicKinds)
					if err != nil {
						return nil, err
					}
					blocks = append(blocks, map[string]any{"type": "tool_result", "tool_use_id": field(result, "id"),
						"content": content, "is_error": failed})
				}
				messages = append(messages, map[string]any{"role": "user", "content": blocks})
			default:
				content, err := with(job, part, text(field(part, "content")), anthropicKinds)
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]any{"role": "user", "content": content})
			}
		}
		room := c.Room
		if room == 0 {
			room = defaultRoom // this service refuses a call without it
		}
		body := map[string]any{"model": c.Model, "max_tokens": room, "messages": messages}
		if c.System != "" {
			body["system"] = c.System
		}
		if c.Effort != "" {
			body["output_config"] = map[string]any{"effort": c.Effort}
		}
		var tools []any
		for _, tool := range c.Tools {
			tools = append(tools, map[string]any{"name": field(tool, "name"), "description": field(tool, "does"),
				"input_schema": schema(field(tool, "args"))})
		}
		if tools != nil {
			body["tools"] = tools
		}
		return body, nil
	},
	reply: func(body map[string]any) (*Reply, error) {
		r := &Reply{Stop: StopEnd, Kept: body["content"]}
		for _, block := range list(body["content"]) {
			switch field(block, "type") {
			case "text":
				r.Text += text(field(block, "text"))
			case "tool_use":
				r.Calls = append(r.Calls, map[string]any{"id": field(block, "id"), "name": field(block, "name"), "args": argsOf(field(block, "input"))})
			}
		}
		switch body["stop_reason"] {
		case "tool_use":
			r.Stop = StopTools
		case "max_tokens", "model_context_window_exceeded":
			r.Stop = StopLength
		}
		// The input counted includes what the service read from and wrote
		// to its own cache.
		usage := body["usage"]
		r.Usage = map[string]any{
			"input":  number(field(usage, "input_tokens")) + number(field(usage, "cache_creation_input_tokens")) + number(field(usage, "cache_read_input_tokens")),
			"output": number(field(usage, "output_tokens")),
		}
		return r, nil
	},
	// The answer arrives as a message that is started, blocks of content
	// that are each started, added to and stopped, and an end that says why
	// it stopped and what was used.
	gather: func(whole, event map[string]any) (string, bool, error) {
		block := func() map[string]any {
			blocks := list(whole["content"])
			if i := int(number(event["index"])); i < len(blocks) {
				return fields(blocks[i])
			}
			return map[string]any{}
		}
		switch event["type"] {
		case "message_start":
			for name, value := range fields(event["message"]) {
				whole[name] = value
			}
		case "content_block_start":
			whole["content"] = append(list(whole["content"]), event["content_block"])
		case "content_block_delta":
			b, delta := block(), event["delta"]
			switch field(delta, "type") {
			case "text_delta":
				added := text(field(delta, "text"))
				b["text"] = text(b["text"]) + added
				return added, false, nil
			case "input_json_delta":
				b["arriving"] = text(b["arriving"]) + text(field(delta, "partial_json"))
			case "thinking_delta":
				b["thinking"] = text(b["thinking"]) + text(field(delta, "thinking"))
			case "signature_delta":
				b["signature"] = text(b["signature"]) + text(field(delta, "signature"))
			}
		case "content_block_stop":
			// A tool's arguments arrive as text, a piece at a time.
			if b := block(); b["arriving"] != nil {
				b["input"] = argsOf(b["arriving"])
				delete(b, "arriving")
			}
		case "message_delta":
			whole["stop_reason"] = field(event["delta"], "stop_reason")
			usage := fields(whole["usage"])
			if usage == nil {
				usage = map[string]any{}
				whole["usage"] = usage
			}
			for name, value := range fields(event["usage"]) {
				usage[name] = value
			}
		case "message_stop":
			return "", true, nil
		case "error":
			return "", false, failed(field(event["error"], "message"))
		}
		return "", false, nil
	},
}

var chatKinds = kinds{
	text: func(text string) any { return map[string]any{"type": "text", "text": text} },
	image: func(typ, data string) any {
		return map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + typ + ";base64," + data}}
	},
	document: func(name, data string) any {
		return map[string]any{"type": "file", "file": map[string]any{"filename": name + ".pdf", "file_data": "data:application/pdf;base64," + data}}
	},
}

var openaiChat = &web{
	path:    "/v1/chat/completions",
	headers: func(key string) map[string]string { return map[string]string{"Authorization": "Bearer " + key} },
	request: func(job *plugin.Job, c *Call) (map[string]any, error) {
		var messages []any
		if c.System != "" {
			messages = append(messages, map[string]any{"role": "system", "content": c.System})
		}
		for _, part := range c.Said {
			switch field(part, "role") {
			case "assistant":
				message := map[string]any{"role": "assistant", "content": text(field(part, "content"))}
				var calls []any
				for _, made := range list(field(part, "calls")) {
					calls = append(calls, map[string]any{"id": field(made, "id"), "type": "function",
						"function": map[string]any{"name": field(made, "name"), "arguments": argsText(field(made, "args"))}})
				}
				if calls != nil {
					message["tool_calls"] = calls
				}
				messages = append(messages, message)
			case "tool":
				// This service takes only text in a tool's result: what the
				// results carry beside it goes in a message of the user's
				// that follows them.
				var beside []any
				for _, result := range list(field(part, "results")) {
					shown, _ := resultText(result)
					messages = append(messages, map[string]any{"role": "tool", "tool_call_id": field(result, "id"), "content": shown})
					files, err := attached(job, result, chatKinds)
					if err != nil {
						return nil, err
					}
					beside = append(beside, files...)
				}
				if beside != nil {
					messages = append(messages, map[string]any{"role": "user", "content": beside})
				}
			default:
				content, err := with(job, part, text(field(part, "content")), chatKinds)
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]any{"role": "user", "content": content})
			}
		}
		// The count of tokens is asked for, since an answer read as it
		// arrives otherwise comes without one.
		body := map[string]any{"model": c.Model, "messages": messages, "stream_options": map[string]any{"include_usage": true}}
		if c.Room > 0 {
			body["max_completion_tokens"] = c.Room
		}
		if c.Effort != "" {
			body["reasoning_effort"] = c.Effort
		}
		var tools []any
		for _, tool := range c.Tools {
			// Offered without the service's strict reading of the shapes of
			// their arguments.
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": field(tool, "name"), "description": field(tool, "does"), "parameters": schema(field(tool, "args"))}})
		}
		if tools != nil {
			body["tools"] = tools
		}
		return body, nil
	},
	reply: func(body map[string]any) (*Reply, error) {
		choices := list(body["choices"])
		if len(choices) == 0 {
			return nil, plugin.Errorf(plugin.Refused, "the model service's answer holds no reply")
		}
		message := field(choices[0], "message")
		r := &Reply{Stop: StopEnd, Text: text(field(message, "content"))}
		for _, made := range list(field(message, "tool_calls")) {
			fn := field(made, "function")
			r.Calls = append(r.Calls, map[string]any{"id": field(made, "id"), "name": field(fn, "name"), "args": argsOf(field(fn, "arguments"))})
		}
		switch field(choices[0], "finish_reason") {
		case "tool_calls":
			r.Stop = StopTools
		case "length":
			r.Stop = StopLength
		}
		usage := body["usage"]
		r.Usage = map[string]any{"input": number(field(usage, "prompt_tokens")), "output": number(field(usage, "completion_tokens"))}
		return r, nil
	},
	// The answer arrives as pieces of one message: its text, and the tools
	// it asks for, each with its arguments as text a piece at a time. The
	// piece that says why it stopped is the last of the message.
	gather: func(whole, event map[string]any) (string, bool, error) {
		if said := event["error"]; said != nil {
			return "", false, failed(field(said, "message"))
		}
		if usage := event["usage"]; usage != nil {
			whole["usage"] = usage
		}
		pieces := list(event["choices"])
		if len(pieces) == 0 {
			return "", false, nil
		}
		if whole["choices"] == nil {
			whole["choices"] = []any{map[string]any{"message": map[string]any{"content": ""}}}
		}
		choice := fields(list(whole["choices"])[0])
		message, delta := fields(choice["message"]), field(pieces[0], "delta")
		added, _ := field(delta, "content").(string)
		message["content"] = text(message["content"]) + added
		calls := list(message["tool_calls"])
		for _, piece := range list(field(delta, "tool_calls")) {
			at := int(number(field(piece, "index")))
			for len(calls) <= at {
				calls = append(calls, map[string]any{"function": map[string]any{"name": "", "arguments": ""}})
			}
			made, fn := fields(calls[at]), fields(field(calls[at], "function"))
			if id := field(piece, "id"); id != nil {
				made["id"] = id
			}
			fn["name"] = text(fn["name"]) + text(field(field(piece, "function"), "name"))
			fn["arguments"] = text(fn["arguments"]) + text(field(field(piece, "function"), "arguments"))
		}
		if calls != nil {
			message["tool_calls"] = calls
		}
		if why := field(pieces[0], "finish_reason"); why != nil {
			choice["finish_reason"] = why
			return added, true, nil
		}
		return added, false, nil
	},
}

var responsesKinds = kinds{
	text: func(text string) any { return map[string]any{"type": "input_text", "text": text} },
	image: func(typ, data string) any {
		return map[string]any{"type": "input_image", "image_url": "data:" + typ + ";base64," + data}
	},
	document: func(name, data string) any {
		return map[string]any{"type": "input_file", "filename": name + ".pdf", "file_data": "data:application/pdf;base64," + data}
	},
}

var openaiResponses = &web{
	path:    "/v1/responses",
	headers: func(key string) map[string]string { return map[string]string{"Authorization": "Bearer " + key} },
	request: func(job *plugin.Job, c *Call) (map[string]any, error) {
		var input []any
		for _, part := range c.Said {
			switch field(part, "role") {
			case "assistant":
				// Nothing is kept at the service: its reasoning comes
				// sealed, is kept with the conversation and sent back.
				if kept := list(field(part, "kept")); kept != nil {
					input = append(input, kept...)
					continue
				}
				if t := text(field(part, "content")); t != "" {
					input = append(input, map[string]any{"role": "assistant", "content": t})
				}
				for _, made := range list(field(part, "calls")) {
					input = append(input, map[string]any{"type": "function_call", "call_id": field(made, "id"),
						"name": field(made, "name"), "arguments": argsText(field(made, "args"))})
				}
			case "tool":
				// As for the other form: what the results carry beside text
				// follows them in a message of the user's.
				var beside []any
				for _, result := range list(field(part, "results")) {
					shown, _ := resultText(result)
					input = append(input, map[string]any{"type": "function_call_output", "call_id": field(result, "id"), "output": shown})
					files, err := attached(job, result, responsesKinds)
					if err != nil {
						return nil, err
					}
					beside = append(beside, files...)
				}
				if beside != nil {
					input = append(input, map[string]any{"role": "user", "content": beside})
				}
			default:
				content, err := with(job, part, text(field(part, "content")), responsesKinds)
				if err != nil {
					return nil, err
				}
				input = append(input, map[string]any{"role": "user", "content": content})
			}
		}
		body := map[string]any{"model": c.Model, "input": input, "store": false, "include": []any{"reasoning.encrypted_content"}}
		if c.System != "" {
			body["instructions"] = c.System
		}
		if c.Room > 0 {
			body["max_output_tokens"] = c.Room
		}
		if c.Effort != "" {
			body["reasoning"] = map[string]any{"effort": c.Effort}
		}
		var tools []any
		for _, tool := range c.Tools {
			tools = append(tools, map[string]any{"type": "function", "name": field(tool, "name"), "description": field(tool, "does"),
				"parameters": schema(field(tool, "args")), "strict": false})
		}
		if tools != nil {
			body["tools"] = tools
		}
		return body, nil
	},
	reply: func(body map[string]any) (*Reply, error) {
		r := &Reply{Stop: StopEnd, Kept: body["output"]}
		for _, item := range list(body["output"]) {
			switch field(item, "type") {
			case "message":
				for _, content := range list(field(item, "content")) {
					if field(content, "type") == "output_text" {
						r.Text += text(field(content, "text"))
					}
				}
			case "function_call":
				r.Calls = append(r.Calls, map[string]any{"id": field(item, "call_id"), "name": field(item, "name"), "args": argsOf(field(item, "arguments"))})
			}
		}
		if body["status"] == "incomplete" {
			r.Stop = StopLength
		}
		usage := body["usage"]
		r.Usage = map[string]any{"input": number(field(usage, "input_tokens")), "output": number(field(usage, "output_tokens"))}
		return r, nil
	},
	// The answer arrives as its text a piece at a time, and then the whole
	// of it in the event that says it is complete.
	gather: func(whole, event map[string]any) (string, bool, error) {
		switch event["type"] {
		case "response.output_text.delta":
			return text(event["delta"]), false, nil
		case "response.completed", "response.incomplete":
			for name, value := range fields(event["response"]) {
				whole[name] = value
			}
			return "", true, nil
		case "response.failed":
			return "", false, failed(field(field(event["response"], "error"), "message"))
		case "error":
			return "", false, failed(event["message"])
		}
		return "", false, nil
	},
}

const page = `llm: this plugin calls a model and returns its reply.

  llm.chat(conversation, { service, model, address, key, system, tools, shape, effort, reply_tokens, timeout, own })
      gives { text, calls, stop, usage, kept }
  llm.takes(blob, { service })    returns true when the service accepts the blob in files

conversation is a list of parts, each one of these.

  { role: "user", content, files }                                   what the model is told
  { role: "assistant", content, calls, kept }                        a reply of the model's
  { role: "tool", results: [{ id, name, result, error, files }] }    the results of its tool calls

files maps a name to a blob: an image (png, jpeg, gif or webp, up to 5 MiB) or
a PDF (up to 32 MiB). The plugin sends each to the model after a line that
gives its name. Any other blob fails the call before anything is sent.

Options:
  service         anthropic, openai (the Responses API), openai-chat (the
                  Chat Completions API) or claude-code (the claude program
                  on the runner's computer); default anthropic
  model           the model's name; required
  address         the service's URL, when it is not https://api.anthropic.com
                  or https://api.openai.com
  key             a handle to the secret that holds the service's key
  system          the system prompt
  tools           the tools the model may call: [{ name, does, args }], with
                  args as { name: type }
  shape           the fields of a reply that is data, as { name: type }; text
                  is then JSON
  effort          low, medium or high; claude-code also takes xhigh and max
  reply_tokens    the most tokens a reply may hold
  timeout         seconds, or text such as "5m"; default 3m
  own             for claude-code: the program's own tools that it may use,
                  by name

Returns:
  text    the model's reply; also emitted as "text" while it arrives, for a
          client that watches the run
  calls   the tool calls the model made: [{ id, name, args }]
  stop    "end", "tools" when it made tool calls, or "length" when the reply
          was cut at its limit
  usage   { input, output }, in tokens
  kept    what the service needs back unchanged, or null; put it in the
          assistant part

Errors:
  bad_input   the service does not accept a blob in files
  refused     the service could not be reached, returned an error, or stopped
              before the reply was complete
  timeout     the reply was not complete within the timeout
  not_found   the claude program could not be started
  failed      the claude program ended without a reply

Needs net:host for the service and secret:NAME@net:host for its key.
claude-code needs these in their place.
  exec:claude                to start the program
  env:HOME                   with no key: the program is signed in as the runner's user
  secret:NAME@exec:claude    with a key: an API key, or the token that claude setup-token prints
  network                    for WebSearch or WebFetch in own:
  disk                       for Read, Glob, Grep, Edit, Write or NotebookEdit in own:
  all                        for any other tool in own:, such as Bash or Skill

claude-code ignores address and reply. ` + "`corbie manual agent`" + ` lists the types,
and says how Corbie starts the claude program.

This script asks for a reply as data.

    allow: [net:api.anthropic.com, "secret:ANTHROPIC_KEY@net:api.anthropic.com"]

    const reply = llm.chat([{ role: "user", content: "Say hello." }], {
      model: "claude-sonnet-5-5", key: secrets.get("ANTHROPIC_KEY"),
      shape: { greeting: "string" },
    })
    return JSON.parse(reply.text)       // gives { greeting: "Hello." }

This script sends an image with its question.

    allow: [fs.read:/srv/reports/**, net:api.anthropic.com, "secret:ANTHROPIC_KEY@net:api.anthropic.com"]

    const chart = blobs.file("/srv/reports/chart.png")
    llm.takes(chart, { service: "anthropic" })       // gives true
    const reply = llm.chat([{ role: "user", content: "What is this?", files: { chart } }], {
      model: "claude-sonnet-5-5", key: secrets.get("ANTHROPIC_KEY"),
    })
    return reply.text                                // gives "A picture, named chart."

This script calls the claude program, signed in as the runner's user.

    allow: [exec:claude, env:HOME]

    const reply = llm.chat([{ role: "user", content: "Say hello." }], { service: "claude-code", model: "sonnet" })
    return reply.text                   // gives "Hello."
`
