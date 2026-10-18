package llm

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// watcher is a job's channel that keeps what was emitted.
type watcher struct {
	mu     sync.Mutex
	pieces []string
	seen   chan struct{} // closed at the first emit
}

func (w *watcher) Emit(name string, data any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pieces) == 0 {
		close(w.seen)
	}
	w.pieces = append(w.pieces, data.(string))
	return nil
}
func (*watcher) Variable(string) (string, bool, error)                { return "", false, nil }
func (*watcher) ReadBlob(blob.Ref, int64, int64) ([]byte, error)      { return nil, nil }
func (*watcher) WriteBlob(string, int64, io.Reader) (blob.Ref, error) { return blob.Ref{}, nil }
func (*watcher) Define(string, map[string]string) (blob.Ref, error)   { return blob.Ref{}, nil }
func (w *watcher) text() string                                       { return strings.Join(w.pieces, "") }

// standIn is a model service built for the tests. It answers each call
// with the events it is given, one after another, and keeps what it was
// sent. Where an event is the word "wait", it sends nothing more until the
// watcher has been emitted to.
type standIn struct {
	*httptest.Server
	sent  []map[string]any
	paths []string // where each call was sent
	watch *watcher
	job   *plugin.Job
}

func newStandIn(t *testing.T, events ...string) *standIn {
	s := &standIn{watch: &watcher{seen: make(chan struct{})}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &sent)
		s.sent = append(s.sent, sent)
		s.paths = append(s.paths, r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			if event == "wait" {
				select {
				case <-s.watch.seen:
				case <-time.After(5 * time.Second):
				}
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", event)
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(s.Close)
	s.job = &plugin.Job{Context: context.Background(), Channel: s.watch,
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	t.Cleanup(s.job.End)
	return s
}

// ask calls the stand-in as the named service.
func (s *standIn) ask(t *testing.T, service string, more plugin.Options) (map[string]any, error) {
	t.Helper()
	opt := plugin.Options{"service": service, "model": "test-model", "address": s.URL, "key": "sk-test"}
	for name, value := range more {
		opt[name] = value
	}
	out, err := chat(s.job, []any{map[string]any{"role": "user", "content": "Say hello."}}, opt)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func (s *standIn) must(t *testing.T, service string, more plugin.Options) map[string]any {
	t.Helper()
	out, err := s.ask(t, service, more)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The three services' ways of sending "Hello there." and asking for the
// tool look, a piece at a time.
var arriving3 = map[string][]string{
	"anthropic": {
		`{"type":"message_start","message":{"role":"assistant","content":[],"usage":{"input_tokens":10,"cache_read_input_tokens":300,"cache_creation_input_tokens":20,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"ping"}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`wait`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"look","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"at\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" \"door\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	},
	"openai-chat": {
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
		`wait`,
		`{"choices":[{"index":0,"delta":{"content":" there."},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t1","type":"function","function":{"name":"look","arguments":"{\"at\":"}}]},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" \"door\"}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":330,"completion_tokens":7}}`,
		`[DONE]`,
	},
	"openai": {
		`{"type":"response.created","response":{"status":"in_progress"}}`,
		`{"type":"response.output_text.delta","delta":"Hello"}`,
		`wait`,
		`{"type":"response.output_text.delta","delta":" there."}`,
		`{"type":"response.completed","response":{"status":"completed","output":[` +
			`{"type":"reasoning","encrypted_content":"sealed"},` +
			`{"type":"message","content":[{"type":"output_text","text":"Hello there."}]},` +
			`{"type":"function_call","call_id":"t1","name":"look","arguments":"{\"at\": \"door\"}"}],` +
			`"usage":{"input_tokens":330,"output_tokens":7}}}`,
	},
}

// The model's text is emitted as it arrives: a watcher has the first piece
// while the service is still answering, and the pieces together are the
// whole of what the call gives back.
func TestReplyIsEmittedAsItArrives(t *testing.T) {
	for service, events := range arriving3 {
		s := newStandIn(t, events...)
		out := s.must(t, service, nil)
		if out["text"] != "Hello there." || s.watch.text() != "Hello there." || len(s.watch.pieces) != 2 {
			t.Fatalf("%s: the call gave %q and the watcher saw %q", service, out["text"], s.watch.pieces)
		}
		if got := text(out["calls"]); got != `[{"args":{"at":"door"},"id":"t1","name":"look"}]` || out["stop"] != StopTools {
			t.Fatalf("%s: the tools asked for are %s, and it stopped for %v", service, got, out["stop"])
		}
		if got := text(out["usage"]); got != `{"input":330,"output":7}` {
			t.Fatalf("%s: the tokens used are %s", service, got)
		}
		if s.sent[0]["stream"] != true {
			t.Fatalf("%s: the answer was not asked for as it is written", service)
		}
	}
}

// An answer that stops before the service has said it is complete is not
// an answer, and a failure the service reports part-way is the call's.
func TestAnswerCutShortIsNoAnswer(t *testing.T) {
	for service, events := range arriving3 {
		half := []string{}
		for _, event := range events[:3] {
			if event != "wait" {
				half = append(half, event)
			}
		}
		if _, err := newStandIn(t, half...).ask(t, service, nil); err == nil || !strings.Contains(err.Error(), "cut short") {
			t.Fatalf("%s: half an answer gave %v", service, err)
		}
	}
	for service, failure := range map[string]string{
		"anthropic":   `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
		"openai-chat": `{"error":{"message":"Overloaded"}}`,
		"openai":      `{"type":"response.failed","response":{"error":{"message":"Overloaded"}}}`,
	} {
		_, err := newStandIn(t, arriving3[service][0], failure).ask(t, service, nil)
		if err == nil || !strings.Contains(err.Error(), "failed while answering: Overloaded") {
			t.Fatalf("%s: a failure part-way gave %v", service, err)
		}
	}
}

func Test_model_cut_off_tool_request_not_passed_on(t *testing.T) {
	out := newStandIn(t,
		`{"type":"message_start","message":{"role":"assistant","content":[],"usage":{"input_tokens":10}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"remove","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"/srv/rep"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	).must(t, "anthropic", nil)
	if out["stop"] != StopLength || len(out["calls"].([]any)) != 0 {
		t.Fatalf("a reply that ran out of room gave back %v, stopped for %v", out["calls"], out["stop"])
	}
}

func Test_model_tool_requests_mean_stopped_for_tools(t *testing.T) {
	out := newStandIn(t,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"look","arguments":"{}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	).must(t, "openai-chat", nil)
	if out["stop"] != StopTools || len(out["calls"].([]any)) != 1 {
		t.Fatalf("the service said it finished, with a tool asked for: %v, stopped for %v", out["calls"], out["stop"])
	}
}

func Test_model_window_full_is_out_of_room(t *testing.T) {
	out := newStandIn(t,
		`{"type":"message_start","message":{"role":"assistant","content":[],"usage":{"input_tokens":10}}}`,
		`{"type":"message_delta","delta":{"stop_reason":"model_context_window_exceeded"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	).must(t, "anthropic", nil)
	if out["stop"] != StopLength {
		t.Fatalf("a full window was passed on as %v", out["stop"])
	}
}

func Test_model_usage_counts_cached_input(t *testing.T) {
	out := newStandIn(t, arriving3["anthropic"]...).must(t, "anthropic", nil)
	if got := field(out["usage"], "input"); got != 330.0 {
		t.Fatalf("10 tokens read, 300 from the cache and 20 written to it count as %v", got)
	}
}

func Test_model_reply_size_always_sent_where_needed(t *testing.T) {
	s := newStandIn(t, arriving3["anthropic"]...)
	s.must(t, "anthropic", nil)
	s.must(t, "anthropic", plugin.Options{"reply_tokens": 500.0})
	if s.sent[0]["max_tokens"] != float64(defaultRoom) || s.sent[1]["max_tokens"] != 500.0 {
		t.Fatalf("the most a reply may hold was sent as %v, then %v", s.sent[0]["max_tokens"], s.sent[1]["max_tokens"])
	}
}

// One call gives the conversation so far, the tools on offer and the shape
// of answer wanted, and each service is sent all three. The shape is told
// to the model at the end of the system text.
func TestConversationToolsAndShapeReachEachService(t *testing.T) {
	said := []any{
		map[string]any{"role": "user", "content": "Which book is on the top shelf?"},
		map[string]any{"role": "assistant", "content": "I will look."},
	}
	shape := map[string]any{"type": "object", "required": []any{"title", "pages"}, "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "pages": map[string]any{"type": "number"}, "cover": map[string]any{"type": "string", "format": "blob"}}}
	const wanted = `Give your final reply as JSON only, fitting this JSON Schema: {"properties":{"cover":{"description":"the name of a blob, as it was shown: blob1, blob2, …","format":"blob","type":"string"},"pages":{"type":"number"},"title":{"type":"string"}},"required":["title","pages"],"type":"object"}`
	for service, where := range map[string]string{"anthropic": "system", "openai": "instructions", "openai-chat": "messages"} {
		s := newStandIn(t, arriving3[service]...)
		ask := func(more plugin.Options) map[string]any {
			t.Helper()
			opt := plugin.Options{"service": service, "model": "test-model", "address": s.URL, "key": "sk-test",
				"tools": []any{map[string]any{"name": "look", "does": "Look at a shelf", "args": map[string]any{"type": "object", "properties": map[string]any{"at": map[string]any{"type": "string"}}}}}}
			for name, value := range more {
				opt[name] = value
			}
			if _, err := chat(s.job, said, opt); err != nil {
				t.Fatal(err)
			}
			return s.sent[len(s.sent)-1]
		}
		sent := ask(plugin.Options{"system": "You keep a library.",
			"shape": shape})
		all := text(sent)
		if !strings.Contains(all, "Which book is on the top shelf?") || !strings.Contains(all, "I will look.") || !strings.Contains(all, `"name":"look"`) {
			t.Errorf("%s was not sent the conversation and the tool: %s", service, all)
		}
		systemOf := func(sent map[string]any) string {
			if where == "messages" {
				return text(field(list(sent[where])[0], "content"))
			}
			return text(sent[where])
		}
		if system := systemOf(sent); system != "You keep a library.\n\n"+wanted {
			t.Errorf("%s was told, as its system text: %q", service, system)
		}
		// With no system text, the shape is all of it; with no shape,
		// nothing is added.
		if alone := systemOf(ask(plugin.Options{"shape": shape})); alone != wanted {
			t.Errorf("%s, given a shape and no system text, was told %s", service, alone)
		}
		if none := text(ask(plugin.Options{"system": "You keep a library."})); strings.Contains(none, "JSON only") {
			t.Errorf("%s, given no shape, was sent %s", service, none)
		}
	}
}

// A tool's arguments and the shape of answer are taken only as JSON
// Schema. A blob that says nothing of itself is described as the name the
// model was shown, and one that says something keeps what it says.
func TestChatTakesShapesAsJSONSchemaOnly(t *testing.T) {
	s := newStandIn(t, arriving3["anthropic"]...)
	short := []any{map[string]any{"name": "look", "does": "Look at a shelf", "args": map[string]any{"shelf": "string"}}}
	var refused *plugin.Error
	if _, err := s.ask(t, "anthropic", plugin.Options{"tools": short}); !errors.As(err, &refused) || refused.Code != plugin.BadInput {
		t.Fatalf("a tool's arguments in the short form gave: %v", err)
	}
	if _, err := s.ask(t, "anthropic", plugin.Options{"shape": map[string]any{"title": "string"}}); !errors.As(err, &refused) || refused.Code != plugin.BadInput {
		t.Fatalf("a shape of answer in the short form gave: %v", err)
	}
	args := func(cover map[string]any) plugin.Options {
		return plugin.Options{"tools": []any{map[string]any{"name": "look", "does": "Look at a shelf",
			"args": map[string]any{"type": "object", "properties": map[string]any{"cover": cover}}}}}
	}
	s.must(t, "anthropic", args(map[string]any{"type": "string", "format": "blob"}))
	sent := text(s.sent[len(s.sent)-1])
	if !strings.Contains(sent, `"cover":{"description":"the name of a blob, as it was shown: blob1, blob2, …","format":"blob","type":"string"}`) {
		t.Errorf("a blob that says nothing of itself was sent as %s", sent)
	}
	s.must(t, "anthropic", args(map[string]any{"type": "string", "format": "blob", "description": "the scan of the cover"}))
	sent = text(s.sent[len(s.sent)-1])
	if !strings.Contains(sent, `"cover":{"description":"the scan of the cover","format":"blob","type":"string"}`) {
		t.Errorf("a blob with a description of its own was sent as %s", sent)
	}
}

// The level of effort a call asks for reaches Anthropic's service, and a
// call that asks for none says nothing of it.
func TestEffortReachesAnthropic(t *testing.T) {
	s := newStandIn(t, arriving3["anthropic"]...)
	s.must(t, "anthropic", plugin.Options{"effort": "high"})
	s.must(t, "anthropic", nil)
	if got := text(s.sent[0]["output_config"]); got != `{"effort":"high"}` {
		t.Fatalf("a call that asked for high effort was sent output_config: %s", got)
	}
	if s.sent[1]["output_config"] != nil {
		t.Fatalf("a call that asked for no level was sent output_config: %s", text(s.sent[1]["output_config"]))
	}
}

func Test_model_openai_tools_not_strict(t *testing.T) {
	tools := plugin.Options{"tools": []any{map[string]any{"name": "look", "does": "Look at a thing", "args": map[string]any{"type": "object", "properties": map[string]any{"at": map[string]any{"type": "string"}}}}}}
	s := newStandIn(t, arriving3["openai"]...)
	s.must(t, "openai", tools)
	if tool := list(s.sent[0]["tools"])[0]; field(tool, "strict") != false {
		t.Fatalf("the tool was offered as %s", text(tool))
	}
	s = newStandIn(t, arriving3["openai-chat"]...)
	s.must(t, "openai-chat", tools)
	if fn := field(list(s.sent[0]["tools"])[0], "function"); field(fn, "strict") == true || field(fn, "name") != "look" {
		t.Fatalf("the tool was offered as %s", text(fn))
	}
}

func Test_model_address_with_login_refused(t *testing.T) {
	job := &plugin.Job{Context: context.Background(), Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	for call, err := range map[string]error{
		"chat":   second(chat(job, []any{}, plugin.Options{"model": "m", "address": "https://user:hunter2@models.example"})),
		"decide": second(decide(job, "a state", plugin.Options{"model": "m", "address": "https://user:hunter2@models.example", "questions": urgent})),
	} {
		if err == nil || !strings.Contains(err.Error(), "may not hold a user name or a password") || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "models.example") {
			t.Fatalf("%s: an address with a login gave %v", call, err)
		}
	}
}

func second(_ any, err error) error { return err }

func Test_model_no_redirect_followed(t *testing.T) {
	reached := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer other.Close()
	s := &standIn{watch: &watcher{seen: make(chan struct{})}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer s.Close()
	s.job = &plugin.Job{Context: context.Background(), Channel: s.watch,
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	if _, err := s.ask(t, "anthropic", nil); err == nil || !strings.Contains(err.Error(), "answered 307") || reached {
		t.Fatalf("a redirect gave %v, and the other place was reached: %v", err, reached)
	}
	_, err := decide(s.job, "a state", plugin.Options{"model": "jev-latest", "address": s.URL, "questions": urgent})
	if err == nil || !strings.Contains(err.Error(), "answered 307") || reached {
		t.Fatalf("decide: a redirect gave %v, and the other place was reached: %v", err, reached)
	}
}

// decisions is a System One model built for the tests. It answers every
// call with the one answer it is given, after any delay it is given, and
// keeps what it was sent.
type decisions struct {
	*httptest.Server
	delay   time.Duration
	sent    []map[string]any
	paths   []string
	headers []http.Header
	job     *plugin.Job
}

func newDecisions(t *testing.T, answer string) *decisions {
	d := &decisions{}
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &sent)
		d.sent = append(d.sent, sent)
		d.paths = append(d.paths, r.URL.Path)
		d.headers = append(d.headers, r.Header)
		time.Sleep(d.delay)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, answer)
	}))
	t.Cleanup(d.Close)
	d.job = &plugin.Job{Context: context.Background(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	t.Cleanup(d.job.End)
	return d
}

// ask puts the question urgent, and any more, to the stand-in.
func (d *decisions) ask(more plugin.Options) (map[string]any, error) {
	opt := plugin.Options{"model": "jev-latest", "address": d.URL, "key": "ts-test", "questions": urgent}
	for name, value := range more {
		opt[name] = value
	}
	out, err := decide(d.job, "Checkout has been failing for every customer for the last hour.", opt)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

// Three questions, one of each type, as the System One format has them.
var (
	urgent = map[string]any{"urgent": map[string]any{"type": "noul", "instructions": "The customer cannot use the product."}}
	three  = map[string]any{
		"urgent": urgent["urgent"],
		"team": map[string]any{"type": "choice", "instructions": "Which team should handle this?",
			"criteria": map[string]any{"billing": "Payments and refunds", "technical": "Outages and errors"}},
		"severity": map[string]any{"type": "score", "instructions": "How severe is the impact?",
			"criteria": []any{"No impact", "Minor", "Major", "Critical"}},
	}
	// The answers as TypeSafe's service writes them, with a field the plugin
	// does not know.
	answered = `{"model":"jev-1.13.0","answers":{` +
		`"urgent":{"type":"noul","noul":0.99},` +
		`"team":{"type":"choice","choice":"technical","confidence":1.0,"probabilities":{"technical":1.0,"billing":0.0}},` +
		`"severity":{"type":"score","score":2.67,"confidence":0.5,"legend":{"0":"No impact","1":"Minor","2":"Major","3":"Critical"},"probabilities":{"0":0.0,"1":0.0,"2":0.33,"3":0.67},"latency_ms":41}},` +
		`"usage":{"input_tokens":428,"output_tokens":73}}`
)

// The state and the questions reach the service as they were written, at
// its path and with the key as a bearer token, and the answers come back as
// the service wrote them, with the model that answered and the tokens used.
func TestQuestionsReachTheModelAndAnswersComeBackAsWritten(t *testing.T) {
	d := newDecisions(t, answered)
	out, err := d.ask(plugin.Options{"questions": three})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"jev-latest","questions":` + text(three) + `,"state":"Checkout has been failing for every customer for the last hour."}`
	if got := text(d.sent[0]); got != want || d.paths[0] != "/v1/systemone" || d.headers[0].Get("Authorization") != "Bearer ts-test" {
		t.Fatalf("the service was sent %s\n at %s with %q", got, d.paths[0], d.headers[0].Get("Authorization"))
	}
	var sent map[string]any
	json.Unmarshal([]byte(answered), &sent)
	if text(out["answers"]) != text(sent["answers"]) || out["model"] != "jev-1.13.0" || text(out["usage"]) != `{"input":428,"output":73}` {
		t.Fatalf("the call gave %s", text(out))
	}
}

// A jev model is of the systemone service, at TypeSafe's address; address
// sends the call to another host, and a call with no key sends none.
func TestAJevModelIsOfSystemOne(t *testing.T) {
	if of := Of("jev-latest"); of != "systemone" || services[of].Address != "https://api.typesafe.ai" {
		t.Fatalf("jev-latest is of %q, at %q", of, services[of].Address)
	}
	d := newDecisions(t, answered)
	if _, err := d.ask(plugin.Options{"key": nil, "model": "nimble"}); err != nil {
		t.Fatal(err)
	}
	if _, sent := d.headers[0]["Authorization"]; sent || !strings.HasPrefix(d.URL, "http://") {
		t.Fatalf("a call with no key, to %s, was sent the header Authorization: %q", d.URL, d.headers[0].Get("Authorization"))
	}
}

// Each call needs a service that makes it, and decide needs its questions.
func TestEachCallNeedsAServiceThatMakesIt(t *testing.T) {
	d := newDecisions(t, answered)
	for what, err := range map[string]error{
		"chat with systemone":   second(chat(d.job, []any{}, plugin.Options{"service": "systemone", "model": "jev-latest", "address": d.URL})),
		"decide with anthropic": second(d.ask(plugin.Options{"service": "anthropic"})),
	} {
		if err == nil || !strings.Contains(err.Error(), "service: is one of") || len(d.sent) != 0 {
			t.Errorf("%s gave %v, and %d calls were sent", what, err, len(d.sent))
		}
	}
	if _, err := d.ask(plugin.Options{"questions": nil}); err == nil || !strings.Contains(err.Error(), "needs questions:") {
		t.Errorf("decide with no questions gave %v", err)
	}
	if _, err := decide(d.job, nil, plugin.Options{"model": "jev-latest", "address": d.URL, "questions": urgent}); err == nil || !strings.Contains(err.Error(), "takes the state first") {
		t.Errorf("decide with no state gave %v", err)
	}
}

// An answer that is not JSON, or holds no answers, is no answer; and the
// call has a time limit like any other wait.
func TestASystemOneAnswerIsReadWhole(t *testing.T) {
	for answer, want := range map[string]string{"<html>": "is not JSON", `{"model":"jev-1.13.0"}`: "holds no answers"} {
		if _, err := newDecisions(t, answer).ask(nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("the answer %s gave %v", answer, err)
		}
	}
	d := newDecisions(t, answered)
	d.delay = 300 * time.Millisecond
	if _, err := d.ask(plugin.Options{"timeout": 0.05}); err == nil || !strings.Contains(err.Error(), "within 50ms") {
		t.Errorf("a slow answer gave %v", err)
	}
}

// shows is a conversation that holds a picture the user gives, and a tool's
// result that carries a document.
func shows(picture, document *plugin.Blob) []any {
	return []any{
		map[string]any{"role": "user", "content": "What is in blob1?", "files": map[string]any{"blob1": picture}},
		map[string]any{"role": "assistant", "content": "", "calls": []any{map[string]any{"id": "t1", "name": "fetch", "args": map[string]any{}}}},
		map[string]any{"role": "tool", "results": []any{map[string]any{"id": "t1", "name": "fetch",
			"result": map[string]any{"blob": "blob2"}, "files": map[string]any{"blob2": document}}}},
	}
}

func (s *standIn) send(service string, said []any) (string, error) {
	_, err := chat(s.job, said, plugin.Options{"service": service, "model": "test-model", "address": s.URL, "key": "sk-test"})
	if len(s.sent) == 0 {
		return "", err
	}
	return text(s.sent[len(s.sent)-1]), err
}

// A conversation may hold content beside text, an image or a document,
// for a service that takes it: it reaches the service as it is, under the
// name the model knows it by. A kind the service does not take fails the
// call as bad input, naming the kind, and nothing is sent.
func TestContentBesideText(t *testing.T) {
	picture, document := plugin.NewBlob("image/png", []byte("\x89PNG picture")), plugin.NewBlob("application/pdf", []byte("%PDF document"))
	sent, err := newStandIn(t, arriving3["anthropic"]...).send("anthropic", shows(picture, document))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`[{"text":"What is in blob1?","type":"text"},{"text":"blob1:","type":"text"},{"source":{"data":"iVBORyBwaWN0dXJl","media_type":"image/png","type":"base64"},"type":"image"}]`,
		`{"text":"blob2:","type":"text"},{"source":{"data":"JVBERiBkb2N1bWVudA==","media_type":"application/pdf","type":"base64"},"type":"document"}],"is_error":false,"tool_use_id":"t1","type":"tool_result"`,
	} {
		if !strings.Contains(sent, want) {
			t.Fatalf("the service was not sent %s\n in %s", want, sent)
		}
	}
	s := newStandIn(t, arriving3["anthropic"]...)
	_, err = s.send("anthropic", shows(plugin.NewBlob("video/mp4", []byte("film")), document))
	if err == nil || !strings.Contains(err.Error(), `does not take content of type "video/mp4"`) || len(s.sent) != 0 {
		t.Fatalf("a film gave %v, and %d calls were sent", err, len(s.sent))
	}
}

func Test_model_openai_tool_result_content_follows(t *testing.T) {
	picture, document := plugin.NewBlob("image/png", []byte("\x89PNG picture")), plugin.NewBlob("application/pdf", []byte("%PDF document"))
	for service, want := range map[string]string{
		// The result is text alone, and what it carried is in a message of
		// the user's that follows it.
		"openai":      `{"call_id":"t1","output":"{\"blob\":\"blob2\"}","type":"function_call_output"},{"content":[{"text":"blob2:","type":"input_text"},{"file_data":"data:application/pdf;base64,JVBERiBkb2N1bWVudA==","filename":"blob2.pdf","type":"input_file"}],"role":"user"}`,
		"openai-chat": `{"content":"{\"blob\":\"blob2\"}","role":"tool","tool_call_id":"t1"},{"content":[{"text":"blob2:","type":"text"},{"file":{"file_data":"data:application/pdf;base64,JVBERiBkb2N1bWVudA==","filename":"blob2.pdf"},"type":"file"}],"role":"user"}`,
	} {
		sent, err := newStandIn(t, arriving3[service]...).send(service, shows(picture, document))
		if err != nil || !strings.Contains(sent, want) || !strings.Contains(sent, "data:image/png;base64,iVBORyBwaWN0dXJl") {
			t.Fatalf("%s: %v; the service was sent %s", service, err, sent)
		}
	}
}

func Test_model_too_large_content_refused_before_sending(t *testing.T) {
	s := newStandIn(t, arriving3["anthropic"]...)
	large := plugin.StoredBlob(blob.Ref{Type: "image/png", Size: 6 << 20})
	_, err := s.send("anthropic", shows(large, plugin.NewBlob("application/pdf", []byte("%PDF"))))
	if err == nil || !strings.Contains(err.Error(), `content of type "image/png" may be 5242880 bytes`) || len(s.sent) != 0 {
		t.Fatalf("a picture of six megabytes gave %v, and %d calls were sent", err, len(s.sent))
	}
}

// A reply's parts that the service wants back unchanged, here its
// reasoning, go back to it exactly as they came, and are left out of what
// another service is sent: that one gets what the reply said and asked for.
func Test_model_kept_parts_sent_back_unchanged(t *testing.T) {
	thinking := []string{
		`{"type":"message_start","message":{"role":"assistant","content":[],"usage":{"input_tokens":10}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"The door, "}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"then."}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"c2lnbmVk"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello there."}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"t1","name":"look","input":{}}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"at\": \"door\"}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	}
	// The conversation carried on from a reply: the reply as the standard
	// turn keeps it, and the result of the tool it asked for.
	after := func(reply map[string]any) []any {
		return []any{
			map[string]any{"role": "user", "content": "Say hello."},
			map[string]any{"role": "assistant", "content": reply["text"], "calls": reply["calls"], "kept": reply["kept"]},
			map[string]any{"role": "tool", "results": []any{map[string]any{"id": "t1", "name": "look", "result": "shut"}}},
		}
	}
	const reasoned = `{"signature":"c2lnbmVk","thinking":"The door, then.","type":"thinking"}`
	const sealed = `{"encrypted_content":"sealed","type":"reasoning"}`
	for _, from := range []struct {
		service string
		events  []string
		kept    string // its reasoning, as it came
		said    map[string]string
	}{
		{"anthropic", thinking, reasoned, map[string]string{
			"openai":      `{"content":"Hello there.","role":"assistant"},{"arguments":"{\"at\":\"door\"}","call_id":"t1","name":"look","type":"function_call"}`,
			"openai-chat": `{"content":"Hello there.","role":"assistant","tool_calls":[{"function":{"arguments":"{\"at\":\"door\"}","name":"look"},"id":"t1","type":"function"}]}`,
		}},
		{"openai", arriving3["openai"], sealed, map[string]string{
			"anthropic":   `{"content":[{"text":"Hello there.","type":"text"},{"id":"t1","input":{"at":"door"},"name":"look","type":"tool_use"}],"role":"assistant"}`,
			"openai-chat": `{"content":"Hello there.","role":"assistant","tool_calls":[{"function":{"arguments":"{\"at\":\"door\"}","name":"look"},"id":"t1","type":"function"}]}`,
		}},
	} {
		s := newStandIn(t, from.events...)
		said := after(s.must(t, from.service, nil))
		// Back to the service it came from, the reasoning is as it came.
		back, err := s.send(from.service, said)
		if err != nil || !strings.Contains(back, from.kept) {
			t.Fatalf("%s: %v; its reasoning did not go back as it came, in %s", from.service, err, back)
		}
		// To another service it is left out, and the reply is still there.
		for other, reply := range from.said {
			sent, err := newStandIn(t, arriving3[other]...).send(other, said)
			if err != nil {
				t.Fatalf("%s to %s: %v", from.service, other, err)
			}
			for _, part := range []string{"thinking", "c2lnbmVk", `"type":"reasoning"`, "sealed"} {
				if strings.Contains(sent, part) {
					t.Errorf("%s was sent what %s wanted back (%s): %s", other, from.service, part, sent)
				}
			}
			if !strings.Contains(sent, reply) {
				t.Errorf("%s was not sent the reply %s\n in %s", other, reply, sent)
			}
		}
	}
}

// OpenAI is spoken to in two ways. In its Responses way nothing is kept at
// the service: every call says so, asks for the reasoning sealed, and
// carries the whole conversation, the sealed reasoning of earlier replies
// in it. Its Chat Completions way is the other, and a gpt model is taken
// to speak the first, where tools and reasoning go together.
func Test_model_openai_two_forms(t *testing.T) {
	more := plugin.Options{"effort": "high",
		"tools": []any{map[string]any{"name": "look", "does": "Look at a thing", "args": map[string]any{"type": "object", "properties": map[string]any{"at": map[string]any{"type": "string"}}}}}}
	s := newStandIn(t, arriving3["openai"]...)
	reply := s.must(t, "openai", more)
	if reply["text"] != "Hello there." || reply["stop"] != StopTools || text(reply["calls"]) != `[{"args":{"at":"door"},"id":"t1","name":"look"}]` {
		t.Fatalf("the Responses way gave %s", text(reply))
	}
	said := []any{
		map[string]any{"role": "user", "content": "Say hello."},
		map[string]any{"role": "assistant", "content": reply["text"], "calls": reply["calls"], "kept": reply["kept"]},
		map[string]any{"role": "tool", "results": []any{map[string]any{"id": "t1", "name": "look", "result": "shut"}}},
	}
	if _, err := s.send("openai", said); err != nil {
		t.Fatal(err)
	}
	for i, sent := range s.sent {
		if s.paths[i] != "/v1/responses" || sent["store"] != false || text(sent["include"]) != `["reasoning.encrypted_content"]` || sent["previous_response_id"] != nil {
			t.Fatalf("call %d went to %s and does not say that nothing is to be kept, and the reasoning sealed: %s", i+1, s.paths[i], text(sent))
		}
	}
	if first := text(s.sent[0]); !strings.Contains(first, `"reasoning":{"effort":"high"}`) || !strings.Contains(first, `"name":"look"`) {
		t.Fatalf("the first call does not ask for reasoning and offer the tool together: %s", first)
	}
	// The second call is the whole conversation: what the user said, the
	// reply with its sealed reasoning as it came, and the tool's result.
	want := `"input":[{"content":"Say hello.","role":"user"},{"encrypted_content":"sealed","type":"reasoning"},` +
		`{"content":[{"text":"Hello there.","type":"output_text"}],"type":"message"},` +
		`{"arguments":"{\"at\": \"door\"}","call_id":"t1","name":"look","type":"function_call"},` +
		`{"call_id":"t1","output":"shut","type":"function_call_output"}]`
	if second := text(s.sent[1]); !strings.Contains(second, want) {
		t.Fatalf("the second call was not sent %s\n in %s", want, second)
	}

	c := newStandIn(t, arriving3["openai-chat"]...)
	reply = c.must(t, "openai-chat", more)
	if reply["text"] != "Hello there." || reply["stop"] != StopTools || text(reply["calls"]) != `[{"args":{"at":"door"},"id":"t1","name":"look"}]` || reply["kept"] != nil {
		t.Fatalf("the Chat Completions way gave %s", text(reply))
	}
	sent := text(c.sent[0])
	if c.paths[0] != "/v1/chat/completions" || !strings.Contains(sent, `"messages":[{"content":"Say hello.","role":"user"}]`) ||
		!strings.Contains(sent, `"reasoning_effort":"high"`) || c.sent[0]["input"] != nil {
		t.Fatalf("the Chat Completions way went to %s with %s", c.paths[0], sent)
	}
	if of := Of("gpt-5"); of != "openai" {
		t.Fatalf("a gpt model is taken to be of the service %q", of)
	}
}
