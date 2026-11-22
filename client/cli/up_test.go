package cli_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zummed/corbie/client/cli"
)

// greet is the script of the flow hello.
const greet = `return "hello, " + input.name`

// up runs corbie up on a directory until it is stopped, and gives what it
// prints, what it says, what stops it and waits for it, and its status.
func up(t *testing.T, dir string, args ...string) (out, errOut *said, stop func() int) {
	t.Helper()
	t.Chdir(dir)
	out, errOut = &said{}, &said{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(ctx, append([]string{"up", "--in", dir, "controller", "--interval", "20ms", "runner", "--interval", "20ms"}, args...), strings.NewReader(""), out, errOut)
	}()
	status := -1
	stop = func() int {
		if status < 0 {
			cancel()
			status = <-done
		}
		return status
	}
	t.Cleanup(func() { stop() })
	return out, errOut, stop
}

// corbie up sets up a controller, a runner, a client and a gateway on a
// store in its memory, submits the directory, and serves the store to a
// command of another process, which finds the config .corbie from the
// directory it is run in. When it stops nothing is left.
func TestUpRunsADirectoryAndKeepsNothing(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet, "sub/note.txt": ""})
	at := "/" + filepath.Base(dir)
	out, said, stop := up(t, dir)
	until(t, "the submit", func() bool { return strings.Contains(out.String(), at+"/hello   transient   recorded") })
	for _, line := range []string{"controller ", "runner ", "store of the controller ", "gateway for the client "} {
		if !strings.Contains(said.String(), line) {
			t.Errorf("corbie up did not say %q:\n%s", line, said)
		}
	}
	config := filepath.Join(dir, ".corbie")
	if _, err := os.Stat(filepath.Join(config, "controller", "identity")); !os.IsNotExist(err) {
		t.Errorf("the controller's key of a store in memory is in the config: %v", err)
	}
	if ignore, _ := os.ReadFile(filepath.Join(config, ".gitignore")); string(ignore) != "*\n" {
		t.Errorf("the config's .gitignore holds %q", ignore)
	}
	// A command run below the directory, with no config named, is this
	// corbie up's client.
	t.Chdir(filepath.Join(dir, "sub"))
	status, ran, problem := corbie("", "run", "start", at+"/hello", "--input", `{"name":"up"}`, "--wait")
	if status != 0 || !strings.Contains(ran, `output: "hello, up"`) {
		t.Fatalf("a run started from below the directory ended %d and printed:\n%s%s", status, ran, problem)
	}
	// The directory is read again as the same tree: nothing in .corbie is a
	// machine.
	if status, again, problem := corbie("", "run", "submit", dir); status != 0 || strings.Count(again, "\n") != 1 {
		t.Fatalf("the directory submitted again ended %d and printed:\n%s%s", status, again, problem)
	}
	// No other process is the controller of a store in memory.
	if status, _, problem := corbie("x\n", "controller", "secret", "set", "KEY", "-"); status != 1 || !strings.Contains(problem, "in the memory of the corbie up") {
		t.Fatalf("a secret set from another process ended %d and said: %s", status, problem)
	}
	// A second corbie up on the config is refused, and leaves it.
	t.Chdir(dir)
	if status, _, problem := corbie("", "up"); status != 1 || !strings.Contains(problem, "is running on the config") {
		t.Fatalf("a second corbie up ended %d and said: %s", status, problem)
	}
	if status := stop(); status != 0 {
		t.Fatalf("corbie up, stopped, ended %d:\n%s", status, said)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("corbie up left its config: %v", err)
	}
}

// A config that a killed corbie up left is replaced.
func TestUpReplacesAConfigLeftBehind(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet})
	// A port nothing listens at.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := l.Addr().String()
	l.Close()
	left := files(t, map[string]string{"controller/store": "memory:\n", "controller/listen": free + "\n", "client/identity": "left"})
	if err := os.Rename(left, filepath.Join(dir, ".corbie")); err != nil {
		t.Fatal(err)
	}
	out, said, stop := up(t, dir)
	until(t, "the submit", func() bool {
		return strings.Contains(out.String(), "recorded") || strings.Contains(said.String(), "corbie up:")
	})
	if status := stop(); status != 0 {
		t.Fatalf("corbie up on a config left behind ended %d:\n%s", status, said)
	}
}

// corbie up with a store named keeps the store and the keys, and run again
// continues with both.
func TestUpWithAStoreKeepsIt(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet})
	at := "/" + filepath.Base(dir)
	config := filepath.Join(dir, ".corbie")
	out, said, stop := up(t, dir, "--store", filepath.Join(config, "store"))
	until(t, "the submit", func() bool { return strings.Contains(out.String(), "recorded") })
	if strings.Contains(said.String(), "store of the controller") {
		t.Errorf("a directory store was served:\n%s", said)
	}
	status, ran, problem := corbie("", "run", "start", at+"/hello", "--input", `{"name":"kept"}`, "--wait")
	if status != 0 {
		t.Fatalf("the run ended %d:\n%s%s", status, ran, problem)
	}
	if status := stop(); status != 0 {
		t.Fatalf("corbie up, stopped, ended %d:\n%s", status, said)
	}
	if _, err := os.Stat(filepath.Join(config, "controller", "identity")); err != nil {
		t.Fatalf("corbie up with a store did not keep its keys: %v", err)
	}
	// Run again with no store named, it uses the one the config records.
	out, _, _ = up(t, dir)
	until(t, "the submit", func() bool { return strings.Contains(out.String(), "recorded") })
	if status, listed, problem := corbie("", "run", "ls", "--all"); status != 0 || !strings.Contains(listed, "done") {
		t.Fatalf("the runs listed after corbie up was run again ended %d:\n%s%s", status, listed, problem)
	}
	if status, _, problem := corbie("", "up", "--in", dir, "--ceiling", "network"); status != 2 || !strings.Contains(problem, "corbie init --ceiling changes it") {
		t.Fatalf("--ceiling on a config that is set up ended %d and said: %s", status, problem)
	}
}

// A file with a problem is reported before anything starts, and nothing is
// left.
func TestUpStartsNothingWithAProblem(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet, "bad.yaml": "flow: bad\nstages: 3\n"})
	t.Chdir(dir)
	status, out, problem := corbie("", "up")
	if status != 1 || out != "" || !strings.Contains(problem, "bad.yaml\n") || !strings.Contains(problem, "1 of 2 files have problems, and nothing is started") || strings.Contains(problem, "controller ") {
		t.Fatalf("corbie up with a file that has a problem ended %d and said:\n%s%s", status, out, problem)
	}
	if _, err := os.Stat(filepath.Join(dir, ".corbie")); !os.IsNotExist(err) {
		t.Fatalf("it left a config: %v", err)
	}
}

// A program corbie up starts is given the config, and the address and the
// token of the gateway, and no store in memory by name. It is started once
// the directory is submitted, and when it stops everything stops.
func TestUpGivesAProgramTheConfigAndTheGateway(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet, "values.yaml": "flow: values\nstages:\n  read: { job: { script: ./read.js } }\noutput: ${stages.read.output}\n", "read.js": "return values.HOST\n"})
	at := "/" + filepath.Base(dir)
	t.Setenv("CORBIE_STORE", "")
	t.Chdir(dir)
	program := standIn(t, "frob", `corbie-run() { :; }; echo "frob ran"`)
	status, out, said := corbie("", "up", "--in", dir, "--value", "HOST=db.lan", "frob", "--flag")
	if status != 1 || !strings.Contains(said, "the program frob stopped") {
		t.Fatalf("corbie up frob ended %d and said:\n%s", status, said)
	}
	if !strings.Contains(out, at+"/hello ") || strings.Index(out, "recorded") > strings.Index(out, "frob ran") {
		t.Fatalf("the program was not started after the submit:\n%s", out)
	}
	env := kept(t, program, "env")
	token := ""
	for _, line := range strings.Split(said, "\n") {
		if _, after, ok := strings.Cut(line, "Bearer …): "); ok {
			token = after
		}
	}
	for _, want := range []string{"CORBIE_CONFIG=" + filepath.Join(dir, ".corbie") + "\n", "CORBIE_GATEWAY=http://127.0.0.1:", "CORBIE_GATEWAY_TOKEN=" + token + "\n"} {
		if token == "" || !strings.Contains(env, want) {
			t.Errorf("the program's environment has no %q:\n%s", want, env)
		}
	}
	if strings.Contains(env, "CORBIE_STORE=memory:") {
		t.Errorf("the program was given the store in memory by name")
	}
	if got := kept(t, program, "args"); got != "--flag\n" {
		t.Errorf("the program was started with %q", got)
	}
}
