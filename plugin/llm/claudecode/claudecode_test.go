package claudecode_test

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"

	_ "github.com/zummed/corbie/plugin/llm/claudecode"
)

// channel is a job's channel that gives the runner's variables and keeps
// nothing.
type channel struct{}

func (channel) Emit(string, any) error                               { return nil }
func (channel) Variable(name string) (string, bool, error)           { return os.Getenv(name), true, nil }
func (channel) ReadBlob(blob.Ref, int64, int64) ([]byte, error)      { return nil, nil }
func (channel) WriteBlob(string, int64, io.Reader) (blob.Ref, error) { return blob.Ref{}, nil }
func (channel) Define(string, map[string]string) (blob.Ref, error)   { return blob.Ref{}, nil }

// ask calls the model through a program named claude that reads what it
// is sent and prints the lines it is given.
func ask(t *testing.T, lines ...string) map[string]any {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell on this machine")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\ncat > /dev/null\ncat \"$(dirname \"$0\")/reply\"\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "reply"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), SearchPath: dir + ":/usr/bin:/bin", Channel: channel{},
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"exec:claude", "env:HOME"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("llm")
	out, err := module.Calls["chat"].Invoke(job, "llm", "chat", []any{map[string]any{"role": "user", "content": "Say hello."}},
		plugin.Options{"service": "claude-code", "model": "sonnet",
			"tools": []any{map[string]any{"name": "look", "does": "Look at a thing", "args": map[string]any{"at": "string"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return out.(map[string]any)
}

// arriving is a piece of the program's answer as it is written.
func arriving(part string) string {
	data, _ := json.Marshal(part)
	return fmt.Sprintf(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%s}}}`, data)
}

const starts = `{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"StructuredOutput"}}}`

// Why the model stopped is passed on from the program as it is from a
// service on the web: a reply that finished, one that asked for tools, and
// one that ran out of room each say which they were.
func TestProgramSaysWhyTheModelStopped(t *testing.T) {
	finished := ask(t, starts, arriving(`{"text":"Hello.","tool_calls":[]}`),
		`{"type":"assistant","message":{"role":"assistant","content":[],"stop_reason":null}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"","stop_reason":"end_turn","structured_output":{"text":"Hello.","tool_calls":[]},"usage":{"input_tokens":10,"output_tokens":5}}`)
	if finished["stop"] != "end" || finished["text"] != "Hello." {
		t.Errorf("a reply that finished gave %v", finished)
	}
	tools := ask(t, starts,
		`{"type":"result","subtype":"success","is_error":false,"result":"","stop_reason":"end_turn","structured_output":{"text":"","tool_calls":[{"name":"look","input":{"at":"door"}}]},"usage":{"input_tokens":10,"output_tokens":5}}`)
	if calls, _ := tools["calls"].([]any); tools["stop"] != "tools" || len(calls) != 1 {
		t.Errorf("a reply that asked for a tool gave %v", tools)
	}
	// Out of room, as the result says it: the same text as the finished
	// reply, and told apart from it.
	cut := ask(t, starts, arriving(`{"text":"Hello.","tool_calls":[]}`),
		`{"type":"result","subtype":"success","is_error":false,"result":"","stop_reason":"max_tokens","structured_output":{"text":"Hello.","tool_calls":[{"name":"look","input":{}}]},"usage":{"input_tokens":10,"output_tokens":5}}`)
	if calls, _ := cut["calls"].([]any); cut["stop"] != "length" || cut["text"] != "Hello." || len(calls) != 0 {
		t.Errorf("a reply that ran out of room gave %v", cut)
	}
	// Out of room in the middle of the answer, as the model's last message
	// says it: the program has no whole answer, and what arrived of it is
	// given.
	half := ask(t, starts, arriving(`{"text":"Hello th`),
		`{"type":"assistant","message":{"role":"assistant","content":[],"stop_reason":"max_tokens"}}`,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"","usage":{"input_tokens":10,"output_tokens":5}}`)
	if half["stop"] != "length" || half["text"] != "Hello th" {
		t.Errorf("a reply cut off in the middle of its answer gave %v", half)
	}
}

// A key signs the program in under the name the program reads that kind of
// key from: a subscription's token under one, a key of the API under the
// other. The runner's own sign-in is not given beside it.
func TestProgramIsGivenItsKeyAsTheKindItIs(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell on this machine")
	}
	for value, name := range map[string]string{
		"sk-ant-oat01-stand1n-token": "CLAUDE_CODE_OAUTH_TOKEN",
		"sk-ant-api03-stand1n-key":   "ANTHROPIC_API_KEY",
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\ncat > /dev/null\nhere=$(dirname \"$0\")\nenv | cut -d= -f1 > \"$here/given\"\ncat \"$here/reply\"\n"), 0o755)
		os.WriteFile(filepath.Join(dir, "reply"), []byte(`{"type":"result","subtype":"success","is_error":false,"result":"","stop_reason":"end_turn","structured_output":{"text":"Hello.","tool_calls":[]},"usage":{"input_tokens":10,"output_tokens":5}}`+"\n"), 0o600)
		job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), SearchPath: dir + ":/usr/bin:/bin", Channel: channel{},
			Secrets: map[string]string{"K": value},
			Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"exec:claude", "secret:K@exec:claude"}}}}}
		t.Cleanup(job.End)
		module, _ := plugin.Find("llm")
		if _, err := module.Calls["chat"].Invoke(job, "llm", "chat", []any{map[string]any{"role": "user", "content": "Say hello."}},
			plugin.Options{"service": "claude-code", "model": "sonnet", "key": plugin.NewSecret("K")}); err != nil {
			t.Fatal(err)
		}
		given, _ := os.ReadFile(filepath.Join(dir, "given"))
		names := strings.Fields(string(given))
		for _, other := range []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "HOME"} {
			if has := slices.Contains(names, other); has != (other == name) {
				t.Errorf("with a key that goes under %s, the program was given %s: %v", name, other, has)
			}
		}
	}
}
