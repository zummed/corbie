package cli_test

import (
	"context"
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

// corbie up sets up a controller, a runner, a client and a gateway in the
// config .corbie of the current directory, with the store in it, and submits
// the directory. A command of another process finds the config from the
// directory it is run in. Run again, corbie up continues with what is there.
func TestUpRunsADirectoryFromItsOwnConfig(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet, "sub/note.txt": ""})
	at := "/" + filepath.Base(dir)
	out, said, stop := up(t, dir)
	until(t, "the submit", func() bool { return strings.Contains(out.String(), at+"/hello   transient   recorded") })
	for _, line := range []string{"controller ", "runner ", "gateway for the client "} {
		if !strings.Contains(said.String(), line) {
			t.Errorf("corbie up did not say %q:\n%s", line, said)
		}
	}
	config := filepath.Join(dir, ".corbie")
	if ignore, _ := os.ReadFile(filepath.Join(config, ".gitignore")); string(ignore) != "*\n" {
		t.Errorf("the config's .gitignore holds %q", ignore)
	}
	if _, err := os.Stat(filepath.Join(config, "store", "version")); err != nil {
		t.Errorf("the store is not in the config: %v", err)
	}
	// A command run below the directory, with no config named, uses the
	// config of this corbie up.
	t.Chdir(filepath.Join(dir, "sub"))
	status, ran, problem := corbie("", "run", "start", at+"/hello", "--input", `{"name":"up"}`, "--wait")
	if status != 0 || !strings.Contains(ran, `output: "hello, up"`) {
		t.Fatalf("a run started from below the directory ended %d and printed:\n%s%s", status, ran, problem)
	}
	if status, _, problem := corbie("the value\n", "controller", "secret", "set", "KEY", "-"); status != 0 {
		t.Fatalf("a secret set from below the directory ended %d and said: %s", status, problem)
	}
	// The directory is read again as the same tree: nothing in .corbie is a
	// machine.
	if status, again, problem := corbie("", "run", "submit", dir); status != 0 || strings.Count(again, "\n") != 1 {
		t.Fatalf("the directory submitted again ended %d and printed:\n%s%s", status, again, problem)
	}
	if status := stop(); status != 0 {
		t.Fatalf("corbie up, stopped, ended %d:\n%s", status, said)
	}
	// Run again, it continues with the runs and the secrets in the config.
	out, _, _ = up(t, dir)
	until(t, "the submit", func() bool { return strings.Contains(out.String(), "recorded") })
	if status, listed, problem := corbie("", "run", "ls", "--all"); status != 0 || !strings.Contains(listed, "done") {
		t.Fatalf("the runs listed after corbie up was run again ended %d:\n%s%s", status, listed, problem)
	}
	if status, listed, problem := corbie("", "controller", "secret", "list"); status != 0 || !strings.Contains(listed, "KEY") {
		t.Fatalf("the secrets listed after corbie up was run again ended %d:\n%s%s", status, listed, problem)
	}
	if status, _, problem := corbie("", "up", "--ceiling", "network"); status != 2 || !strings.Contains(problem, "corbie init --ceiling changes it") {
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
// token of the gateway. It is started once
// the directory is submitted, and when it stops everything stops.
func TestUpGivesAProgramTheConfigAndTheGateway(t *testing.T) {
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": greet, "values.yaml": "flow: values\nstages:\n  read: { job: { script: ./read.js } }\noutput: ${stages.read.output}\n", "read.js": "return values.HOST\n"})
	at := "/" + filepath.Base(dir)
	t.Setenv("PORT_OF_THIS_TEST", "5432")
	t.Chdir(dir)
	program := standIn(t, "frob", `corbie-run() { :; }; echo "frob ran"`)
	status, out, said := corbie("", "up", "--in", dir, "--value", "HOST=db.lan", "--value-env", "PORT_OF_THIS_TEST", "frob", "--flag")
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
	if status, listed, problem := corbie("", "controller", "value", "list"); status != 0 || !strings.Contains(listed, "db.lan") || !strings.Contains(listed, "5432") {
		t.Errorf("the values listed ended %d:\n%s%s", status, listed, problem)
	}
	if got := kept(t, program, "args"); got != "--flag\n" {
		t.Errorf("the program was started with %q", got)
	}
}
