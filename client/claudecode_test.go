package client_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
)

// standInProgram puts a program named claude first on the path, in a
// directory of its own. Each time it is started it keeps what it was started
// with (its arguments, its variables, the directory it runs in and what that
// holds, what it was sent, and the system text in the file it was named)
// and prints the reply it was given for that start. With a file named hang
// beside it, it reads nothing and does not end.
func standInProgram(t *testing.T, replies ...string) string {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "claude"), []byte(`#!/bin/sh
d=$(dirname "$0")
if [ -e "$d/hang" ]; then sleep 30; exit 0; fi
n=$(cat "$d/count" 2>/dev/null || echo 0); n=$((n+1)); echo $n > "$d/count"
printf '%s\n' "$@" > "$d/args.$n"
while [ $# -gt 0 ]; do
  if [ "$1" = --system-prompt-file ]; then cat "$2" > "$d/system.$n"; fi
  shift
done
pwd > "$d/pwd.$n"; ls -A > "$d/ls.$n"; env > "$d/env.$n"
cat > "$d/stdin.$n"
cat "$d/reply.$n"
`), 0o755)
	for i, reply := range replies {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("reply.%d", i+1)), []byte(reply), 0o600)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// printed is what the program prints for a reply: the answer a piece at a
// time, and then the result.
func printed(text string, calls string) string {
	answer := fmt.Sprintf(`{"text":%q,"tool_calls":%s}`, text, calls)
	half := len(answer) / 2
	piece := func(part string) string {
		data, _ := json.Marshal(part)
		return fmt.Sprintf(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%s}}}`, data)
	}
	return strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"StructuredOutput"}}}`,
		piece(answer[:half]), piece(answer[half:]),
		`{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":` + answer + `,"usage":{"input_tokens":10,"cache_read_input_tokens":90,"output_tokens":5}}`,
	}, "\n") + "\n"
}

const programAgent = `
	agent: sums
	input: { question: any }
	model: { name: sonnet, service: claude-code MORE }
	system: You add numbers.
	prompt: ${input.question}
	tools:
	  add:
	    does: Add one to a number
	    args: { n: number }
	    job: { script: ./add.js, input: { n: "${args.n}" } }
`

func kept(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func Test_model_local_program_starts_bare(t *testing.T) {
	dir := standInProgram(t,
		printed("let me see", `[{"name":"add","input":{"n":41}},{"name":"add","input":{"n":1}}]`),
		printed("It is 42.", `[]`))
	l := local(t, "all")
	id, err := l.Start(ctx, draft(t, strings.Replace(programAgent, "MORE", "", 1), scripts), client.Start{Input: map[string]any{"question": "what is 41 plus one?"}})
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	r, err := l.Follow(ctx, id, 20e6, func(e client.Entry) {
		if e.Name == "text" {
			seen = append(seen, e.Data.(string))
		}
	})
	if err != nil || r.Status != "done" {
		t.Fatalf("the agent ended %+v, %v", r, err)
	}
	out := r.Output.(map[string]any)
	if out["reply"] != "It is 42." || jsonOf(out["tokens"]) != `{"input":200,"output":10}` {
		t.Fatalf("the result is %v", jsonOf(out))
	}
	// Its text was seen as it was written.
	if got := strings.Join(seen, ""); got != "let me seeIt is 42." {
		t.Fatalf("the follower saw %q", seen)
	}
	// What the turn needs is the start of the program, and where the
	// runner's user is signed in; nothing else.
	def, err := l.Definition(ctx, id)
	if err != nil || !strings.Contains(jsonOf(def), `"allow":["exec:claude","env:HOME","env:USERPROFILE"]`) {
		t.Fatalf("the stored definition gives its turn: %s, %v", jsonOf(def), err)
	}
	// The program was started with its own tools, saved sessions and
	// settings turned off, in a fresh directory, with nothing of the runner's
	// but its path and where its user is signed in.
	args := kept(t, dir, "args.1")
	for _, want := range []string{"--tools\n\n", "--safe-mode\n", "--strict-mcp-config\n", "--no-session-persistence\n", "--model\nsonnet\n", "-p\n"} {
		if !strings.Contains(args, want) {
			t.Fatalf("the program was started without %q: %s", want, args)
		}
	}
	if strings.Contains(args, "--allowed-tools") {
		t.Fatalf("the program was allowed tools of its own: %s", args)
	}
	if where, held := kept(t, dir, "pwd.1"), kept(t, dir, "ls.1"); !strings.Contains(where, "corbie-job-") || held != "" || where == kept(t, dir, "pwd.2") {
		t.Fatalf("it ran in %q, which held %q", where, held)
	}
	for _, line := range strings.Split(strings.TrimSpace(kept(t, dir, "env.1")), "\n") {
		name, _, _ := strings.Cut(line, "=")
		if name != "PATH" && name != "HOME" && name != "PWD" && name != "SHLVL" && name != "_" && name != "OLDPWD" {
			t.Fatalf("the program was given the variable %s", line)
		}
	}
	// It was told the tools in its system text and not given them, and the
	// second time it was shown the whole conversation with the results.
	if system := kept(t, dir, "system.1"); !strings.Contains(system, "## add") || !strings.Contains(system, "tool_calls") {
		t.Fatalf("the system text does not tell of the tools: %s", system)
	}
	second := kept(t, dir, "stdin.2")
	if !strings.Contains(second, "what is 41 plus one?") || !strings.Contains(second, "Assistant called add") ||
		!strings.Contains(second, `{\"n\":42}`) || !strings.Contains(second, `{\"n\":2}`) {
		t.Fatalf("the second call was sent: %s", second)
	}
}

func Test_model_made_up_request_names_never_repeat(t *testing.T) {
	// Three turns, each asking for the same tool twice, the second and the
	// third as if the earlier parts of the conversation were gone.
	calls := `[{"name":"add","input":{"n":1}},{"name":"add","input":{"n":2}}]`
	dir := standInProgram(t, printed("", calls), printed("", calls), printed("", calls), printed("done", `[]`))
	l := local(t, "all")
	r := run(t, l, strings.Replace(programAgent, "MORE", "", 1), scripts, map[string]any{"question": "q"})
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	names := map[string]bool{}
	for _, name := range regexp.MustCompile(`call_[0-9a-f]{16}`).FindAllString(kept(t, dir, "stdin.4"), -1) {
		names[name] = true
	}
	if len(names) != 6 {
		t.Fatalf("six requests were given %d names: %v", len(names), names)
	}
}

func Test_model_local_program_cannot_hold_the_call(t *testing.T) {
	dir := standInProgram(t)
	os.WriteFile(filepath.Join(dir, "hang"), nil, 0o600)
	l := local(t, "all")
	r := run(t, l, `
		flow: held
		stages:
		  call:
		    job: { script: ./ask.js, allow: ["exec:claude", "env:HOME"], timeout: 30s }
	`, map[string]string{"ask.js": `
		const big = "x".repeat(3000000)
		return llm.chat([{ role: "user", content: big }], { service: "claude-code", model: "sonnet", timeout: 1 })
	`}, nil)
	if r.Status != "failed" || !strings.Contains(r.Error, "the model did not answer within 1s") {
		t.Fatalf("a program that reads nothing and does not end gave %s: %s", r.Status, r.Error)
	}
}

// A tool of the program's own is one Corbie does not run and cannot hold
// to a scope. A definition switches one on by name, and it is allowed
// only where the whole of what it could use is.
func Test_model_programs_own_tool_needs_its_whole_group(t *testing.T) {
	dir := standInProgram(t, printed("It is 42.", `[]`), printed("It is 42.", `[]`))
	def := strings.Replace(programAgent, "MORE", ", own: [WebSearch, WebFetch]", 1)
	// Under a ceiling that allows one host, browsing is refused, and the
	// program is not started.
	l := local(t, "exec", "env", "net:api.example")
	r := run(t, l, def, scripts, map[string]any{"question": "q"})
	if r.Failure != "denied" || r.Refused != "network" || !strings.Contains(r.Layer, "ceiling") {
		t.Fatalf("under a narrow ceiling: %s/%s refused %q by %q: %s", r.Status, r.Failure, r.Refused, r.Layer, r.Error)
	}
	if _, err := os.Stat(filepath.Join(dir, "args.1")); err == nil {
		t.Fatal("the program was started all the same")
	}
	// Where the whole network is allowed, the program has those tools and
	// no others, and still none of the user's settings.
	l = local(t, "exec", "env", "network")
	id, err := l.Start(ctx, draft(t, def, scripts), client.Start{Input: map[string]any{"question": "q"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := wait(t, l, id); r.Status != "done" {
		t.Fatalf("with the network allowed the agent ended %s: %s", r.Status, r.Error)
	}
	args := kept(t, dir, "args.1")
	if !strings.Contains(args, "--tools\nWebSearch,WebFetch\n") || !strings.Contains(args, "--allowed-tools\nWebSearch,WebFetch\n") || !strings.Contains(args, "--safe-mode") {
		t.Fatalf("the program was started with: %s", args)
	}
	stored, _ := l.Definition(ctx, id)
	if !strings.Contains(jsonOf(stored), `"allow":["exec:claude","env:HOME","env:USERPROFILE","network"]`) {
		t.Fatalf("the stored definition gives its turn: %s", jsonOf(stored))
	}
	// Skills are the user's own set-up of the program, and could do
	// anything: they need everything allowed, and switch its settings on.
	skilled := strings.Replace(programAgent, "MORE", ", own: [Skill]", 1)
	if r := run(t, l, skilled, scripts, map[string]any{"question": "q"}); r.Failure != "denied" || r.Refused != "all" {
		t.Fatalf("skills under a ceiling that is not all: %s/%s refused %q", r.Status, r.Failure, r.Refused)
	}
	// Tools of its own are for a program: a service on the web has none.
	if _, err := machineParse(strings.Replace(programAgent, "claude-code MORE", "anthropic, key: K, own: [WebSearch]", 1), scripts); err == nil || !strings.Contains(err.Error(), "own: is for a service that is a program") {
		t.Fatalf("own tools for a service on the web gave %v", err)
	}
}

func TestProgramAnswersInTheShapeWanted(t *testing.T) {
	// Where a definition wants its result as data, the program is given the
	// shape as part of what it must answer in, and what it puts there is
	// the reply: a model need not write JSON inside its words.
	answer := `{"text":"I have it.","tool_calls":[],"answer":{"sum":42,"note":"forty-one and one"}}`
	dir := standInProgram(t, strings.Join([]string{`{"type":"system","subtype":"init"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":` + answer + `,"usage":{"input_tokens":10,"output_tokens":5}}`}, "\n")+"\n")
	l := local(t, "all")
	def := strings.Replace(strings.Replace(programAgent, "MORE", "", 1), "prompt:", "reply: { sum: number, note: string }\n\tprompt:", 1)
	r := runAgent(t, l, def, scripts)
	if r.Status != "done" {
		t.Fatalf("the agent ended %s: %s", r.Status, r.Error)
	}
	reply, _ := r.Output.(map[string]any)["reply"].(map[string]any)
	if reply["sum"] != float64(42) || reply["note"] != "forty-one and one" {
		t.Errorf("the reply is %v", r.Output.(map[string]any)["reply"])
	}
	args := kept(t, dir, "args.1")
	if !strings.Contains(args, `"answer":{`) || !strings.Contains(args, `"sum":{"type":"number"}`) {
		t.Errorf("the program was not given the shape to answer in: %s", args)
	}
	if system := kept(t, dir, "system.1"); !strings.Contains(system, "Put it in the `answer` field") || strings.Contains(system, "as JSON only") {
		t.Errorf("the program was not told where its final reply goes: %s", system)
	}
}
