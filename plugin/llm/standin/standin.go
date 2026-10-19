// Package standin is a model service for tests: it speaks Anthropic's
// Messages API, streams each reply as server-sent events, and keeps every
// call it is sent.
package standin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// A Server answers each call to /v1/messages with the reply Answer gives.
type Server struct {
	Answer func(Call) Reply
	// Key, where set, is the x-api-key a call must carry; a call without
	// it is refused with 401.
	Key string

	mu sync.Mutex
	// Calls are the calls the server was sent, in order. A test reads
	// them once the server is no longer being called.
	Calls []Call
}

// A Call is what one call sent: the system text, and the messages and
// tools as they were given.
type Call struct {
	Turn     int    // 1 for the first call the server is sent
	Key      string // the x-api-key header
	System   string
	Messages []any
	Tools    []any
}

// A Reply is the model's answer: its text, then the tools it calls. Each
// reply uses 10 tokens in and 5 out.
type Reply struct {
	Text  string
	Calls []ToolCall
	// Cut says the reply was cut short at its length limit.
	Cut bool
	// Error, where set, is sent with status 500 in place of a reply.
	Error string
	// Hold, where set, holds the reply after the first half of its text
	// has been sent, until something is sent on it, it is closed or the
	// caller gives up. A held reply needs a text of two characters or
	// more, and one with less is answered with status 500.
	Hold <-chan struct{}
}

// A ToolCall is one tool the model calls, with its arguments as JSON.
type ToolCall struct {
	ID, Name, Args string
}

// ServeHTTP answers one call.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	failure := func(status int, kind, message string) {
		failed, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": kind, "message": message}})
		http.Error(w, string(failed), status)
	}
	if r.Method != "POST" || r.URL.Path != "/v1/messages" {
		failure(404, "not_found_error", "not found")
		return
	}
	if s.Key != "" && r.Header.Get("x-api-key") != s.Key {
		failure(401, "authentication_error", "invalid x-api-key")
		return
	}
	var sent struct {
		System   string `json:"system"`
		Messages []any  `json:"messages"`
		Tools    []any  `json:"tools"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &sent); err != nil {
		failure(400, "invalid_request_error", err.Error())
		return
	}
	s.mu.Lock()
	call := Call{Turn: len(s.Calls) + 1, Key: r.Header.Get("x-api-key"), System: sent.System, Messages: sent.Messages, Tools: sent.Tools}
	s.Calls = append(s.Calls, call)
	s.mu.Unlock()
	reply := s.Answer(call)
	text := pieces(reply.Text)
	if reply.Hold != nil && len(text) < 2 {
		reply.Error = "the stand-in cannot hold a reply whose text is less than two characters"
	}
	if reply.Error != "" {
		failure(500, "api_error", reply.Error)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	event := func(v map[string]any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", v["type"], data)
		w.(http.Flusher).Flush()
	}
	delta := func(index int, kind, field, piece string) {
		event(map[string]any{"type": "content_block_delta", "index": index, "delta": map[string]any{"type": kind, field: piece}})
	}
	event(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 1}}})
	index := 0
	if reply.Text != "" {
		event(map[string]any{"type": "content_block_start", "index": index, "content_block": map[string]any{"type": "text", "text": ""}})
		for i, piece := range text {
			if i == len(text)/2 && reply.Hold != nil {
				select {
				case <-reply.Hold:
				case <-r.Context().Done():
					return
				}
			}
			delta(index, "text_delta", "text", piece)
		}
		event(map[string]any{"type": "content_block_stop", "index": index})
		index++
	}
	for _, c := range reply.Calls {
		event(map[string]any{"type": "content_block_start", "index": index,
			"content_block": map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": map[string]any{}}})
		for _, piece := range pieces(c.Args) {
			delta(index, "input_json_delta", "partial_json", piece)
		}
		event(map[string]any{"type": "content_block_stop", "index": index})
		index++
	}
	stop := "end_turn"
	switch {
	case reply.Cut:
		stop = "max_tokens"
	case len(reply.Calls) > 0:
		stop = "tool_use"
	}
	event(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop}, "usage": map[string]any{"output_tokens": 5}})
	event(map[string]any{"type": "message_stop"})
}

// pieces cuts a text into four pieces of about the same length, between
// characters.
func pieces(text string) []string {
	runes := []rune(text)
	var out []string
	for i := range 4 {
		if piece := runes[len(runes)*i/4 : len(runes)*(i+1)/4]; len(piece) > 0 {
			out = append(out, string(piece))
		}
	}
	return out
}
