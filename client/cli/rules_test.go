package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client/cli"
)

// said is what a command that stays has written so far, read while it
// goes on writing.
type said struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (s *said) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text.Write(p)
}

func (s *said) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text.String()
}

// until waits for something a command that stays has yet to do.
func until(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); !done(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s never came", what)
		}
	}
}

// staying runs a command that stays, until the test ends, and gives what
// it prints and what it says.
func (m machine) staying(args ...string) (out, errOut *said) {
	m.t.Helper()
	out, errOut = &said{}, &said{}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(ctx, append(args, "--config", m.config), strings.NewReader(""), out, errOut)
	}()
	m.t.Cleanup(func() {
		stop()
		<-done
	})
	return out, errOut
}

// working is a machine set up with corbie init, whose controller and
// runner run until the test ends. It gives the directory its store is in.
func working(t *testing.T, ceiling string) (machine, string) {
	t.Helper()
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box", "--ceiling", ceiling)
	m.staying("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms")
	return m, storeDir
}

// stored lists every value in a directory store.
func stored(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	return names
}

// lines reads what a command printed as JSON, one value to a line.
func lines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var values []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("a line printed is not JSON: %q", line)
		}
		values = append(values, v)
	}
	return values
}

// Preparing reads only the files beside the definition: a path that leaves
// their directory is refused, and nothing is uploaded or sent.
func Test_client_definition_path_outside_set_refused(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	os.MkdirAll(filepath.Join(inside, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "outside.js"), []byte(`return "from outside the set"`), 0o644)
	os.WriteFile(filepath.Join(root, "outside.yaml"), []byte("flow: outside\nstages: {}\n"), 0o644)
	os.WriteFile(filepath.Join(inside, "in.js"), []byte(`return 1`), 0o644)
	before := stored(t, storeDir)
	for _, job := range []string{
		`{ script: ../outside.js }`,
		`{ script: ./sub/../../outside.js }`,
		`{ script: ./in.js, files: [../outside.js] }`,
		`{ script: ` + filepath.Join(root, "outside.js") + ` }`,
		`{ machine: ../outside.yaml }`,
		`{ script: ./.. }`,
	} {
		file := filepath.Join(inside, "flow.yaml")
		os.WriteFile(file, []byte("flow: reads\nstages:\n  it: { job: "+job+" }\n"), 0o644)
		for _, command := range [][]string{{"run", "submit"}, {"test"}} {
			status, out, errOut := m.run("", append(command, file)...)
			if status != 1 || out != "" || !strings.Contains(errOut, "stages.it") {
				t.Errorf("corbie %s of a definition whose job is %s ended %d: %s%s", strings.Join(command, " "), job, status, out, errOut)
			}
		}
	}
	if after := stored(t, storeDir); !slices.Equal(before, after) {
		t.Fatalf("a refused definition left something in the store: %v", after)
	}
	// One inside the directory is read.
	os.WriteFile(filepath.Join(inside, "flow.yaml"), []byte("flow: reads\nstages:\n  it: { job: { script: ./sub/../in.js } }\n"), 0o644)
	if out := m.ok("", "test", filepath.Join(inside, "flow.yaml")); !strings.Contains(out, "done") {
		t.Fatalf("a definition naming a file beside it printed %s", out)
	}
}

// A flag that has no effect on what was given is a mistake, and the
// command does nothing.
func Test_client_flag_without_effect_refused(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`})
	definition := filepath.Join(dir, "hello.yaml")
	id := strings.TrimSpace(m.ok("", "run", "submit", definition, "--input", `{"name": "x"}`))
	before := stored(t, storeDir)
	for flag, mistaken := range map[string][]string{
		// For a script file only: a definition gives its jobs their own.
		"--memory applies only to a script file": {"run", "submit", definition, "--memory", "64MB"},
		"--tag applies only to a script file":    {"run", "submit", definition, "--tag", "gpu"},
		// test always stays until the run ends.
		"-wait": {"test", definition, "--wait"},
		// A role's flag written after another role.
		"--jobs is for the runner, not the controller":             {"start", "controller", "--jobs", "8"},
		"--exec-wrapper is for the runner, not the controller":     {"start", "controller", "--exec-wrapper", "bwrap"},
		"--listen is for the gateway or the store, not the runner": {"start", "controller", "runner", "--listen", "127.0.0.1:0"},
		"--token-file is for the gateway, not the runner":          {"start", "runner", "--token-file", filepath.Join(dir, "token")},
		"--interval is for the controller or the runner":           {"start", "gateway", "--interval", "5s"},
		"--interval is written before any name":                    {"start", "--interval", "5s", "controller"},
		// show prints one thing.
		"--watch and --wait each ask":         {"run", "show", id, "--wait", "--watch"},
		"--definition and --tickets each ask": {"run", "show", id, "--definition", "--tickets"},
		"--step and --save each ask":          {"run", "show", id, "--step", "1", "--save", filepath.Join(dir, "saved")},
		"--watch and --definition each":       {"run", "show", id, "--watch", "--definition"},
		"--record and --watch each ask":       {"run", "show", id, "--record", "--watch"},
		// The two ways of staying.
		"--wait and --watch are two ways": {"run", "submit", definition, "--wait", "--watch"},
	} {
		status, out, errOut := m.run("", mistaken...)
		if status != 2 || out != "" || !strings.Contains(errOut, flag) || !strings.Contains(errOut, "usage: corbie "+mistaken[0]) {
			t.Errorf("corbie %s ended %d: %s%s", strings.Join(mistaken, " "), status, out, errOut)
		}
	}
	if after := stored(t, storeDir); !slices.Equal(before, after) {
		t.Fatalf("a command with a flag of no effect did something all the same: %v", after)
	}
	if _, err := os.Stat(filepath.Join(dir, "saved")); err == nil {
		t.Fatal("a command with a flag of no effect saved files all the same")
	}
}

// A command's result goes to standard output and everything said along the
// way to standard error, whether or not JSON was asked for; help that was
// asked for is a result.
func Test_client_results_on_stdout_progress_on_stderr(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `log("greeting"); return { text: "hello " + input.name }`})
	definition := filepath.Join(dir, "hello.yaml")
	input := []string{"--input", `{"name": "x"}`}
	ended := func(v map[string]any) bool { return v["status"] == "done" && v["output"] != nil }

	// The run is the result of a wait; its ID is said on the way.
	status, out, errOut := m.run("", append([]string{"run", "submit", definition, "--wait", "--json"}, input...)...)
	printed := lines(t, out)
	if status != 0 || len(printed) != 1 || !ended(printed[0]) || !strings.Contains(errOut, printed[0]["id"].(string)) {
		t.Fatalf("submit --wait --json printed %s and said %s", out, errOut)
	}
	id := printed[0]["id"].(string)
	// The entries of a record that is followed are what was asked for:
	// they are printed, each a JSON value on a line, and the run after them.
	for _, command := range [][]string{append([]string{"test", definition, "--watch"}, input...), {"run", "show", id, "--watch"},
		append([]string{"run", "submit", definition, "--watch"}, input...)} {
		status, out, errOut = m.run("", append(command, "--json")...)
		printed = lines(t, out)
		if status != 0 || len(printed) < 4 || printed[0]["step"] != 1.0 || !ended(printed[len(printed)-1]) || strings.Contains(errOut, "{") {
			t.Fatalf("corbie %s --json printed %s and said %s", strings.Join(command, " "), out, errOut)
		}
		// As text they go the same way.
		status, out, errOut = m.run("", command...)
		if status != 0 || !strings.Contains(out, "step 1  <- start") || !strings.Contains(out, "greet log: greeting") ||
			!strings.Contains(out, "output:") || strings.Contains(errOut, "step") {
			t.Fatalf("corbie %s printed %s and said %s", strings.Join(command, " "), out, errOut)
		}
	}
	// A listing that is followed prints the mark of the end of the first
	// listing among its rows.
	for _, asJSON := range []bool{true, false} {
		args := []string{"run", "ls", "--watch", "--all"}
		mark := "everything that already existed is above"
		if asJSON {
			args, mark = append(args, "--json"), `{"listed":true}`
		}
		out, errOut := m.staying(args...)
		until(t, "the mark of the end of the first listing", func() bool { return strings.Contains(out.String(), mark) })
		if !strings.Contains(out.String(), id) || strings.Contains(errOut.String(), "everything") || strings.Contains(errOut.String(), "listed") {
			t.Fatalf("corbie %s printed %s and said %s", strings.Join(args, " "), out, errOut)
		}
		if asJSON {
			lines(t, out.String())
		}
	}
	// Help that was asked for is printed; a mistake and a failure are said,
	// with nothing printed. corbie help lists no program from this
	// computer's PATH.
	t.Setenv("PATH", t.TempDir())
	for _, command := range [][]string{{"help"}, {"--help"}, {"run", "submit", "--help"}, {"controller", "secret", "set", "--help"}} {
		if status, out, errOut := m.run("", command...); status != 0 || !strings.Contains(out, "corbie") || errOut != "" {
			t.Errorf("corbie %s ended %d, printed %q and said %q", strings.Join(command, " "), status, out, errOut)
		}
	}
	for want, command := range map[int][]string{2: {"run", "show"}, 1: {"run", "show", "box-client/20200101T000000.000Z-0000000000"}} {
		if status, out, errOut := m.run("", command...); status != want || out != "" || !strings.HasPrefix(errOut, "corbie run show: ") {
			t.Errorf("corbie %s ended %d, printed %q and said %q", strings.Join(command, " "), status, out, errOut)
		}
	}
	if status, out, errOut := m.run("", "frobnicate"); status != 2 || out != "" || !strings.Contains(errOut, "is not a command") {
		t.Errorf("corbie frobnicate ended %d, printed %q and said %q", status, out, errOut)
	}
}

// corbie run submit with neither --wait nor --watch prints the run's ID and
// nothing else.
func Test_client_submit_prints_only_the_id(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`, "double.js": `return input.n * 2`})
	shape := regexp.MustCompile(`^box-client/\d{8}T\d{6}\.\d{3}Z-[0-9a-f]{10}\n$`)
	for _, command := range [][]string{
		{"run", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`},
		{"run", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`, "--label", "team=ops", "--timeout", "1h", "--life", "10m", "--allow", "network"},
		{"run", "submit", filepath.Join(dir, "double.js"), "--input", `{"n": 2}`, "--timeout", "5s", "--allow", "network"},
	} {
		status, out, errOut := m.run("", command...)
		if status != 0 || !shape.MatchString(out) || errOut != "" {
			t.Fatalf("corbie %s ended %d, printed %q and said %q", strings.Join(command, " "), status, out, errOut)
		}
		// The ID is taken by other commands as it was printed.
		if shown := m.ok("", "run", "show", strings.TrimSpace(out)); !strings.Contains(shown, strings.TrimSpace(out)+"  waiting") {
			t.Fatalf("show of the ID printed %s", shown)
		}
	}
}

// A run's name is given under one word, its ID, by every command and over
// HTTP.
func Test_client_run_name_is_one_field_everywhere(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `emit("note", 1); return { text: "hello " + input.name }`})
	token := filepath.Join(dir, "token")
	os.WriteFile(token, []byte("the-token\n"), 0o600)
	_, saying := m.staying("start", "gateway", "--listen", "127.0.0.1:0", "--token-file", token)
	address := regexp.MustCompile(`listening on (http://\S+)`)
	until(t, "the gateway", func() bool { return address.MatchString(saying.String()) })
	url := address.FindStringSubmatch(saying.String())[1]
	kind := "" // of the body of the next request, where it has one
	get := func(method, path, body string) string {
		t.Helper()
		req, _ := http.NewRequest(method, url+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer the-token")
		if kind != "" {
			req.Header.Set("Content-Type", kind)
		}
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s %s was answered %d %s", method, path, resp.StatusCode, data)
		}
		return string(data)
	}
	definition := filepath.Join(dir, "hello.yaml")
	one := strings.TrimSpace(m.ok("", "run", "submit", definition, "--input", `{"name": "x"}`))
	m.ok("", "run", "show", one, "--wait")
	var request bytes.Buffer
	form := multipart.NewWriter(&request)
	form.WriteField("definition", hello)
	form.WriteField("file:./greet.js", "return 1")
	form.WriteField("start", `{"input": {"name": "y"}}`)
	form.Close()
	kind = form.FormDataContentType()
	started := get("POST", "/runs", request.String())
	kind = ""
	var answer map[string]string
	json.Unmarshal([]byte(started), &answer)
	two := answer["id"]
	if two == "" || len(answer) != 1 {
		t.Fatalf("a start over HTTP was answered %s", started)
	}
	m.ok("", "run", "show", two, "--wait")
	until(t, "both heads among the finished", func() bool { return strings.Count(m.ok("", "run", "ls", "--all", "--status", "done"), "\n") == 2 })

	// Everything that gives a run as JSON, by the command line and over
	// HTTP: wherever a run's name stands, the word beside it is id, and
	// everything that is a run or an entry of one has an id.
	var stream strings.Builder
	for _, line := range strings.Split(get("GET", "/runs/"+one+"/record", ""), "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			stream.WriteString(data + "\n")
		}
	}
	shown := map[string]string{
		"submit --json":       m.ok("", "run", "submit", definition, "--input", `{"name": "z"}`, "--json", "--life", "1s"),
		"show --json":         m.ok("", "run", "show", one, "--json"),
		"show --wait --json":  m.ok("", "run", "show", two, "--wait", "--json"),
		"show --watch --json": m.ok("", "run", "show", one, "--watch", "--json"),
		"show --step --json":  m.ok("", "run", "show", one, "--step", "1"),
		"ls --all --json":     m.ok("", "run", "ls", "--all", "--json"),
		"GET /runs/<id>":      get("GET", "/runs/"+one, ""),
		"GET /runs/<id>?wait": get("GET", "/runs/"+two+"?wait", ""),
		"GET /runs?all":       get("GET", "/runs?all", ""),
		"the record's stream": stream.String(),
		"POST /runs":          started,
	}
	names := []string{one, two}
	var walk func(where, key string, v any)
	walk = func(where, key string, v any) {
		switch v := v.(type) {
		case string:
			if slices.Contains(names, v) && key != "id" {
				t.Errorf("%s gives a run's name under the word %q", where, key)
			}
		case map[string]any:
			if _, isRun := v["status"]; isRun && v["id"] == nil {
				t.Errorf("%s gives a run with no id: %v", where, v)
			}
			for name, inner := range v {
				walk(where, name, inner)
			}
		case []any:
			for _, inner := range v {
				walk(where, key, inner)
			}
		}
	}
	for where, text := range shown {
		found := false
		for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
			var v any
			if line == "" {
				continue
			}
			if err := json.Unmarshal([]byte(line), &v); err != nil {
				t.Fatalf("%s printed %q, which is not JSON", where, line)
			}
			walk(where, "", v)
			found = found || strings.Contains(line, `"id":"box-client/`)
		}
		if !found && text != "" {
			t.Errorf("%s names no run under the word id: %s", where, text)
		}
	}
	// As text, a run is named by its ID at the start of its line.
	for _, command := range [][]string{{"run", "show", one}, {"run", "ls", "--all"}} {
		if out := m.ok("", command...); !strings.HasPrefix(out, one+"  ") {
			t.Errorf("corbie %s printed %s", strings.Join(command, " "), out)
		}
	}
}

// A secret's value is never taken from the command line, and is read only
// from the source the command names. A value that is not found there is a
// mistake that says where it was looked for, and sends nothing.
func Test_client_secret_value_never_on_command_line(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`})
	definition := filepath.Join(dir, "hello.yaml")
	before := stored(t, storeDir)
	os.Unsetenv("CORBIE_TEST_TOKEN")
	refused := func(stdin, says string, args ...string) {
		t.Helper()
		status, out, errOut := m.run(stdin, args...)
		if status != 2 || out != "" || strings.Contains(errOut, "hunter2") || !strings.Contains(errOut, says) {
			t.Errorf("corbie %s ended %d: %s%s", strings.Join(args, " "), status, out, errOut)
		}
	}
	for _, command := range [][]string{{"run", "submit"}, {"test"}} {
		refused("", `the environment variable "CORBIE_TEST_TOKEN" is not set`, append(command, definition, "--secret-env", "CORBIE_TEST_TOKEN")...)
		refused("hunter2-on-standard-input\n", `the secret "CORBIE_TEST_TOKEN" is typed at a terminal, and there is none`, append(command, definition, "--secret", "CORBIE_TEST_TOKEN")...)
		for _, flag := range []string{"--secret", "--secret-env"} {
			refused("", "never given on the command line", append(command, definition, flag, "CORBIE_TEST_TOKEN=hunter2-on-the-line")...)
		}
	}
	set := []string{"controller", "secret", "set"}
	refused("", `the environment variable "CORBIE_TEST_TOKEN" is not set`, append(set, "CORBIE_TEST_TOKEN", "--env")...)
	refused("hunter2-on-standard-input\n", `the secret "CORBIE_TEST_TOKEN" is typed at a terminal, and there is none`, append(set, "CORBIE_TEST_TOKEN")...)
	refused("", `standard input gave no value for the secret "CORBIE_TEST_TOKEN"`, append(set, "CORBIE_TEST_TOKEN", "-")...)
	refused("hunter2-on-standard-input\n", "give one", append(set, "CORBIE_TEST_TOKEN", "-", "--env")...)
	refused("", "never given on the command line", append(set, "CORBIE_TEST_TOKEN=hunter2-on-the-line")...)
	refused("", "never given on the command line", append(set, "CORBIE_TEST_TOKEN", "hunter2-on-the-line")...)
	t.Setenv("CORBIE_TEST_TOKEN", "")
	refused("", `the environment variable "CORBIE_TEST_TOKEN" is empty`, "run", "submit", definition, "--secret-env", "CORBIE_TEST_TOKEN")
	refused("", `the environment variable "CORBIE_TEST_TOKEN" is empty`, append(set, "CORBIE_TEST_TOKEN", "--env")...)
	// A variable of the secret's name is read only when the command names
	// the environment, and one secret is read from one source.
	t.Setenv("CORBIE_TEST_TOKEN", "hunter2-in-the-environment")
	refused("", "is typed at a terminal, and there is none", "run", "submit", definition, "--secret", "CORBIE_TEST_TOKEN")
	refused("", "is typed at a terminal, and there is none", append(set, "CORBIE_TEST_TOKEN")...)
	refused("", `standard input gave no value for the secret "CORBIE_TEST_TOKEN"`, append(set, "CORBIE_TEST_TOKEN", "-")...)
	refused("", `the secret "CORBIE_TEST_TOKEN" is given twice: give it once`, "run", "submit", definition, "--secret-env", "CORBIE_TEST_TOKEN", "--secret", "CORBIE_TEST_TOKEN")
	refused("", `the secret "CORBIE_TEST_TOKEN" is given twice: give it once`, "run", "submit", definition, "--secret-env", "CORBIE_TEST_TOKEN", "--secret-env", "CORBIE_TEST_TOKEN")
	if after := stored(t, storeDir); !slices.Equal(before, after) {
		t.Fatalf("a secret that could not be read was sent all the same: %v", after)
	}
	// From the source named, each is sent, sealed.
	m.ok("", "run", "submit", definition, "--input", `{"name": "x"}`, "--secret-env", "CORBIE_TEST_TOKEN")
	m.ok("", append(set, "CORBIE_TEST_TOKEN", "--env")...)
	m.ok("hunter2-on-standard-input\n", append(set, "CORBIE_TEST_OTHER", "-")...)
	if listed := m.ok("", "controller", "secret", "list"); listed != "CORBIE_TEST_OTHER\nCORBIE_TEST_TOKEN\n" {
		t.Errorf("the controller's secrets are listed as %q", listed)
	}
	for _, name := range stored(t, storeDir) {
		if value, _ := os.ReadFile(name); strings.Contains(string(value), "hunter2") {
			t.Fatalf("%s holds the secret's value in the clear", name)
		}
	}
}

// A refusal is printed once: with the run it reached, and not again by its
// error's text or for each job it came from.
func Test_client_same_refusal_printed_once(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{
		"reads.js": `return fs.read("/etc/hostname")`,
		"fails.yaml": `flow: fails
stages:
  read:
    each: [1, 2, 3, 4, 5]
    job: { script: ./reads.js, input: "${item}" }
`,
		"carries-on.yaml": `flow: carries-on
stages:
  read:
    each: [1, 2, 3, 4, 5]
    job: { script: ./reads.js, input: "${item}" }
output: { failed: "${stages.read.failed}" }
`})
	const fix = "add the line fs.read:/etc/hostname"
	for _, command := range [][]string{
		{"test", filepath.Join(dir, "fails.yaml")},
		{"test", filepath.Join(dir, "fails.yaml"), "--watch"},
		{"test", filepath.Join(dir, "reads.js")},
		{"test", filepath.Join(dir, "reads.js"), "--watch"},
	} {
		status, out, errOut := m.run("", command...)
		if n := strings.Count(out+errOut, fix); status != 1 || n != 1 || strings.Count(out, "refused: fs.read:/etc/hostname") != 1 {
			t.Errorf("corbie %s ended %d and printed the fix %d times:\n%s%s", strings.Join(command, " "), status, n, out, errOut)
		}
	}
	// A flow that carries on past its items' refusals is told of each as it
	// is followed, and the fix is not printed for every one.
	status, out, errOut := m.run("", "test", filepath.Join(dir, "carries-on.yaml"), "--watch")
	if n := strings.Count(out, "denied: fs.read:/etc/hostname"); status != 0 || n != 5 || strings.Count(out+errOut, fix) > 1 {
		t.Errorf("a flow whose five items were each refused ended %d and printed:\n%s%s", status, out, errOut)
	}
}

// Saving a result's files checks every name before it writes any file.
func Test_client_saved_file_names_checked_first(t *testing.T) {
	m, _ := working(t, "all")
	made := func(returns string) string {
		t.Helper()
		dir := files(t, map[string]string{"make.js": "const b = (text) => blobs.make(text)\nreturn " + returns})
		status, out, errOut := m.run("", "run", "submit", filepath.Join(dir, "make.js"), "--wait", "--json")
		if status != 0 {
			t.Fatalf("the run that makes the files ended %d: %s%s", status, out, errOut)
		}
		return lines(t, out)[0]["id"].(string)
	}
	// Each file is saved under the name it has in the value that holds it.
	into := filepath.Join(t.TempDir(), "saved")
	good := made(`{ "report.pdf": b("the report"), chart: b("the chart"), pages: [b("page 0"), b("page 1")], inner: { "notes.txt": b("the notes") } }`)
	out := m.ok("", "run", "show", good, "--save", into)
	for name, want := range map[string]string{"report.pdf": "the report", "chart": "the chart", "pages-0": "page 0", "pages-1": "page 1", "notes.txt": "the notes"} {
		if got, err := os.ReadFile(filepath.Join(into, name)); err != nil || string(got) != want || !strings.Contains(out, filepath.Join(into, name)+"\n") {
			t.Errorf("the file %s was saved as %q, %v; the command printed %s", name, got, err, out)
		}
	}
	if entries, _ := os.ReadDir(into); len(entries) != 5 {
		t.Errorf("five files of a result were saved as %d", len(entries))
	}
	// A name that would land a file somewhere else, or that two files
	// share, is refused before anything is written: the files with good
	// names beside it included.
	for _, returns := range []string{
		`{ aaa: b("good"), "..": b("bad") }`,
		`{ aaa: b("good"), ".": b("bad") }`,
		`{ aaa: b("good"), "sub/escape": b("bad") }`,
		`{ aaa: b("good"), "..\\escape": b("bad") }`,
		`{ aaa: b("good"), "": b("bad") }`,
		`{ aaa: b("good"), one: { report: b("first") }, two: { report: b("second") } }`,
	} {
		into := filepath.Join(t.TempDir(), "saved")
		status, out, errOut := m.run("", "run", "show", made(returns), "--save", into)
		if status != 1 || out != "" || errOut == "" {
			t.Errorf("saving the files of %s ended %d: %s%s", returns, status, out, errOut)
		}
		if entries, err := os.ReadDir(into); err == nil {
			t.Errorf("saving the files of %s wrote %d files before it was refused", returns, len(entries))
		}
	}
}

// A time limit and a memory limit given with a script file are the limits
// of the stage it is wrapped in.
func Test_client_script_limits_reach_wrapped_stage(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{"quick.js": `return 1`, "spins.js": `for (;;) {}`})
	out := m.ok("", "test", filepath.Join(dir, "quick.js"), "--timeout", "7s", "--memory", "64MB", "--watch", "--json")
	var sent map[string]any
	for _, ask := range lines(t, out)[0]["asks"].([]any) {
		if ask := ask.(map[string]any); ask["kind"] == "script" {
			sent = ask["job"].(map[string]any)
		}
	}
	if sent["timeout"] != "7s" || sent["memory"] != float64(64<<20) {
		t.Fatalf("the job of a script given --timeout 7s --memory 64MB was sent as %v", sent)
	}
	// With none given, the stage has none of its own.
	out = m.ok("", "test", filepath.Join(dir, "quick.js"), "--watch", "--json")
	if strings.Contains(out, `"timeout"`) || strings.Contains(out, `"memory"`) {
		t.Fatalf("with no limits given, the job was sent as %s", out)
	}
	// The limit holds: a script that never returns is stopped at it.
	began := time.Now()
	status, out, _ := m.run("", "test", filepath.Join(dir, "spins.js"), "--timeout", "1s")
	if status != 1 || !strings.Contains(out, "time limit of 1s") || time.Since(began) > 20*time.Second {
		t.Fatalf("a script that never returns, given --timeout 1s, ended %d after %v: %s", status, time.Since(began), out)
	}
}

// A refusal met by a submitted script is followed by what to add to the
// command line; one met by a definition's job is not.
func Test_client_wrapped_script_refusal_names_the_flag(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := files(t, map[string]string{"reads.js": `return fs.read("/etc/hostname")`,
		"reads.yaml": "flow: reads\nstages:\n  it: { job: { script: ./reads.js } }\n"})
	status, out, _ := m.run("", "test", filepath.Join(dir, "reads.js"))
	if status != 1 || !strings.Contains(out, "denied") || strings.Count(out, "add to the command: --allow fs.read:/etc/hostname") != 1 {
		t.Fatalf("a refused script printed (%d) %s", status, out)
	}
	// What it says to add is what makes the script run.
	if out := m.ok("", "test", filepath.Join(dir, "reads.js"), "--allow", "fs.read:/etc/hostname", "--json"); !strings.Contains(out, `"status":"done"`) {
		t.Fatalf("with the flag added: %s", out)
	}
	status, out, _ = m.run("", "test", filepath.Join(dir, "reads.yaml"))
	if status != 1 || !strings.Contains(out, "fix: add the line") || strings.Contains(out, "add to the command") {
		t.Fatalf("a definition's refused run printed (%d) %s", status, out)
	}
}

// A store's address is not repeated in an error, wherever the address came
// from.
func Test_client_store_address_not_in_errors(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "not-a-real-key")
	for _, address := range []string{
		"s3://AKIDOTHER:hunter2secret@bucket/path?region=eu-west-1",
		"s3://bucket/path?region=eu-west-1&endpoint=https://user:hunter2secret@store.example:9000",
		"s3://bucket/path?region=eu-west-1&password=hunter2secret",
		"s3://bucket/hunter2secret?endpoint=https://store.example:9000",
		"s3://hunter2secret@/",
	} {
		m := machine{t, filepath.Join(t.TempDir(), "config")}
		m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box")
		card := strings.Fields(m.ok("", "controller", "card"))[0]
		// On the command line, in the environment, and recorded in a config.
		check := func(from string, status int, out, errOut string) {
			t.Helper()
			if status != 1 || strings.Contains(out+errOut, "hunter2secret") || !strings.HasPrefix(errOut, "corbie ") {
				t.Errorf("with the address %s given %s, the command ended %d: %s%s", address, from, status, out, errOut)
			}
		}
		for _, command := range [][]string{{"run", "ls"}, {"start", "controller"}, {"controller", "parties"}} {
			status, out, errOut := m.run("", append(command, "--store", address)...)
			check("on the command line", status, out, errOut)
		}
		t.Setenv("CORBIE_STORE", address)
		status, out, errOut := m.run("", "run", "ls")
		check("in the environment", status, out, errOut)
		os.Unsetenv("CORBIE_STORE")
		viewer := machine{t, filepath.Join(t.TempDir(), "viewer")}
		viewer.ok("", "client", "trust", card, address)
		status, out, errOut = viewer.run("", "run", "ls")
		check("in the config", status, out, errOut)
	}
}

// A store named on the command line is used in place of the one the
// environment names, and that in place of the one the config recorded.
func Test_client_store_flag_beats_environment_beats_config(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	recorded := filepath.Join(t.TempDir(), "recorded")
	m.ok("", "init", "--store", recorded, "--name", "box")
	// Two more stores, each a copy of the first as it was set up.
	fromEnv, fromFlag := filepath.Join(t.TempDir(), "from-env"), filepath.Join(t.TempDir(), "from-flag")
	for _, dir := range []string{fromEnv, fromFlag} {
		if err := os.CopyFS(dir, os.DirFS(recorded)); err != nil {
			t.Fatal(err)
		}
	}
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`})
	counts := func() [3]int {
		return [3]int{len(stored(t, recorded)), len(stored(t, fromEnv)), len(stored(t, fromFlag))}
	}
	submit := func(extra ...string) [3]int {
		t.Helper()
		before := counts()
		m.ok("", append([]string{"run", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`}, extra...)...)
		after := counts()
		return [3]int{after[0] - before[0], after[1] - before[1], after[2] - before[2]}
	}
	if wrote := submit(); wrote[0] == 0 || wrote[1] != 0 || wrote[2] != 0 {
		t.Fatalf("with no store named, the start went to (recorded, environment, flag) %v", wrote)
	}
	t.Setenv("CORBIE_STORE", fromEnv)
	if wrote := submit(); wrote[0] != 0 || wrote[1] == 0 || wrote[2] != 0 {
		t.Fatalf("with a store named in the environment, the start went to (recorded, environment, flag) %v", wrote)
	}
	if wrote := submit("--store", fromFlag); wrote[0] != 0 || wrote[1] != 0 || wrote[2] == 0 {
		t.Fatalf("with a store named on the command line, the start went to (recorded, environment, flag) %v", wrote)
	}
	// What a command reads follows the same order.
	if out := m.ok("", "run", "ls", "--store", recorded); out != "" {
		t.Fatalf("a store with no run in progress lists %s", out)
	}
}

// corbie test carries a run out with the roles corbie start runs: the same
// definition and input, run both ways, leave the same record.
func Test_client_test_and_start_share_roles(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{
		"outer.yaml": `flow: outer
input: [name]
stages:
  greet: { job: { script: ./greet.js, input: { name: "${input.name}" } } }
  kid: { job: { machine: ./kid.yaml, input: { said: "${stages.greet.output.text}" } } }
  reads: { job: { script: ./reads.js, input: "${stages.kid.output}" } }
output: { kid: "${stages.kid.output}", refused: "${stages.reads.failed}" }
`,
		"kid.yaml": `statechart: kid
initial: working
states:
  working:
    job: { script: ./greet.js, input: { name: "${input.said}" } }
    on: { done: over }
  over: { end: done, output: "${states.working.output}" }
`,
		"greet.js": `log("greeting", input.name); emit("made", { file: blobs.make("for " + input.name) }); return { text: "hello " + input.name }`,
		"reads.js": `return fs.read("/etc/hostname")`,
	})
	// The stages follow one another, so the steps fall the same way each
	// time. What a record's entries say, without what differs from run to run:
	// names of runs and of parties, times, and the seeds of names.
	varying := regexp.MustCompile(`"(id|time|at|child|to|message|source|sender|runner|started|ended)":"[^"]*",?|box-|client/[0-9TZ.]+-[0-9a-f]+|client\.[0-9TZ.]+-[0-9a-f]+`)
	record := func(command ...string) (entries []string, run string) {
		t.Helper()
		status, out, errOut := m.run("", append(command, filepath.Join(dir, "outer.yaml"), "--input", `{"name": "x"}`, "--watch", "--json")...)
		if status != 0 {
			t.Fatalf("corbie %s ended %d: %s%s", strings.Join(command, " "), status, out, errOut)
		}
		printed := strings.Split(strings.TrimSpace(out), "\n")
		for _, line := range printed {
			entries = append(entries, varying.ReplaceAllString(line, ""))
		}
		// The parent's entries and the child's are read side by side.
		slices.Sort(entries[:len(entries)-1])
		return entries[:len(entries)-1], entries[len(entries)-1]
	}
	tested, testedRun := record("test")
	submitted, submittedRun := record("run", "submit")
	if len(tested) < 8 || !slices.Equal(tested, submitted) {
		t.Errorf("corbie test left the record\n  %s\nand corbie start with corbie run submit the record\n  %s", strings.Join(tested, "\n  "), strings.Join(submitted, "\n  "))
	}
	if testedRun != submittedRun || !strings.Contains(testedRun, `"status":"done"`) {
		t.Errorf("corbie test ended with\n  %s\nand corbie start with corbie run submit with\n  %s", testedRun, submittedRun)
	}
}

// A waiting or following command given an ID that is no run ends, and at
// once, whatever the ID looks like.
func Test_client_wait_and_watch_end_for_unknown_run(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "box")
	stamp := func(at time.Time) string { return at.UTC().Format("20060102T150405.000Z") }
	began := time.Now()
	for _, id := range []string{
		"box-client/20200101T000000.000Z-0000000000",
		"box-client/" + stamp(time.Now()) + "-0000000000",
		"box-client/" + stamp(time.Now().Add(-time.Minute)) + "-0000000000",
		"box-client/" + stamp(time.Now().Add(time.Hour)) + "-0000000000",
		"box-client/" + stamp(time.Now().Add(1000*24*time.Hour)) + "-0000000000",
		"another-client/" + stamp(time.Now()) + "-0000000000",
	} {
		for _, how := range [][]string{{"--wait"}, {"--watch"}, {"--wait", "--json"}, {"--watch", "--json"}, {}} {
			status, out, errOut := m.run("", append([]string{"run", "show", id}, how...)...)
			if status != 1 || out != "" || !strings.Contains(errOut, "no such run") {
				t.Errorf("corbie run show %s %s ended %d: %s%s", id, strings.Join(how, " "), status, out, errOut)
			}
		}
	}
	// An ID of no run's shape is a failure too, and no wait.
	if status, _, errOut := m.run("", "run", "show", "not-a-run", "--wait"); status != 1 || !strings.Contains(errOut, "not the name of a run") {
		t.Errorf("corbie run show --wait of a name that is no ID ended %d: %s", status, errOut)
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("waiting for and following runs that do not exist took %v", took)
	}
}

// model is a model's host: it starts corbie mcp allowing some commands,
// makes its calls of the command tool, closes the connection, and gives
// each call's answer. The calls run side by side, and none may lean on
// another's having ended.
func (m machine) model(allow []string, calls ...[]string) []struct {
	Failed bool
	Text   string
	Ended  float64
	Result string
	Said   string
} {
	m.t.Helper()
	var input strings.Builder
	for i, args := range calls {
		call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call",
			"params": map[string]any{"name": "corbie", "arguments": map[string]any{"args": args}}})
		input.Write(append(call, '\n'))
	}
	args := []string{"mcp"}
	for _, name := range allow {
		args = append(args, "--allow", name)
	}
	out := m.ok(input.String(), args...)
	answers := make([]struct {
		Failed bool
		Text   string
		Ended  float64
		Result string
		Said   string
	}, len(calls))
	printed := lines(m.t, out)
	if len(printed) != len(calls) {
		m.t.Fatalf("%d calls were answered %d times:\n%s", len(calls), len(printed), out)
	}
	for _, answer := range printed {
		i := int(answer["id"].(float64)) - 1
		result := answer["result"].(map[string]any)
		answers[i].Failed = result["isError"].(bool)
		answers[i].Text = result["content"].([]any)[0].(map[string]any)["text"].(string)
		var ran map[string]any
		if json.Unmarshal([]byte(answers[i].Text), &ran) == nil {
			answers[i].Ended, _ = ran["ended"].(float64)
			answers[i].Result, _ = ran["result"].(string)
			answers[i].Said, _ = ran["said"].(string)
		}
	}
	return answers
}

// The name a command is allowed by and the command that then runs are one
// reading of the command line.
func Test_client_allowed_name_and_command_from_one_reading(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`})
	id := strings.TrimSpace(m.ok("", "run", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`))
	m.ok("", "run", "show", id, "--wait")
	refused := "not one this server was started with permission for"
	// Allowed run cancel, a model cannot run controller cancel, which ends
	// with the same word; allowed controller cancel, it cannot run corbie
	// run cancel.
	answers := m.model([]string{"run cancel"}, []string{"controller", "cancel", id}, []string{"run", "cancel", id})
	if !answers[0].Failed || !strings.Contains(answers[0].Text, `"controller cancel"`) || !strings.Contains(answers[0].Text, refused) {
		t.Errorf("allowed run cancel, a model's controller cancel was answered %+v", answers[0])
	}
	if strings.Contains(answers[1].Text, refused) || !strings.Contains(answers[1].Result, `"taken"`) {
		t.Errorf("allowed run cancel, a model's run cancel was answered %+v", answers[1])
	}
	answers = m.model([]string{"controller cancel"}, []string{"run", "cancel", id}, []string{"controller", "cancel", id})
	if !answers[0].Failed || !strings.Contains(answers[0].Text, `"run cancel"`) || !strings.Contains(answers[0].Text, refused) {
		t.Errorf("allowed controller cancel, a model's run cancel was answered %+v", answers[0])
	}
	if answers[1].Failed || !strings.Contains(answers[1].Said, "the cancel is in the controller's inbox") {
		t.Errorf("allowed controller cancel, a model's controller cancel was answered %+v", answers[1])
	}
	// A command's name among another's arguments allows nothing, and
	// changes nothing of which command runs.
	answers = m.model([]string{"run ls", "run show"}, []string{"run", "ls", "controller", "parties"}, []string{"run", "show", "--json", "controller", "parties"},
		[]string{"controller", "parties", "run", "ls"}, []string{"run", "show", id, "run", "ls"}, []string{"run", "ls", "--status", "done", "--all"})
	for i, want := range []string{"run ls takes no run", "name one run by its ID", refused, "name one run by its ID", ""} {
		if !strings.Contains(answers[i].Text, want) || strings.Contains(answers[i].Text, "box-runner") {
			t.Errorf("call %d was answered %+v", i+1, answers[i])
		}
	}
	if !strings.Contains(answers[4].Result, id) {
		t.Errorf("an allowed command with its own arguments was answered %+v", answers[4])
	}
}

// A command refused to a model is refused before any part of it runs.
func Test_client_model_refused_command_does_nothing(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	marker := filepath.Join(t.TempDir(), "marker")
	saved := filepath.Join(t.TempDir(), "saved")
	fresh := filepath.Join(t.TempDir(), "fresh-store")
	dir := files(t, map[string]string{"hello.yaml": hello, "greet.js": `return 1`,
		"writes.js": `fs.write(` + quote(marker) + `, { text: "ran" }); return 1`, "broken.yaml": "flow: broken\nstages: { a: { job: { script: ./missing.js } } }\n"})
	before := stored(t, storeDir)
	configBefore := stored(t, m.config)
	answers := m.model([]string{"run ls"},
		[]string{"run", "submit", filepath.Join(dir, "hello.yaml"), "--input", `{"name": "x"}`},
		[]string{"test", filepath.Join(dir, "writes.js"), "--allow", "fs.write:" + marker},
		[]string{"run", "submit", filepath.Join(dir, "broken.yaml")},
		[]string{"run", "submit", filepath.Join(dir, "no-such-file.yaml")},
		[]string{"init", "--store", fresh},
		[]string{"run", "show", "box-client/20200101T000000.000Z-0000000000", "--save", saved},
		[]string{"controller", "secret", "set", "HOME"},
		[]string{"client", "card", "--name", "another"},
		[]string{"run", "submit", "--no-such-flag"},
		[]string{"run", "ls"})
	for i, answer := range answers[:9] {
		// Refused as not allowed, and for nothing the command itself would
		// have found: not its files, its flags or its store.
		if !answer.Failed || !strings.Contains(answer.Text, "not one this server was started with permission for") ||
			strings.Contains(answer.Text, "missing.js") || strings.Contains(answer.Text, "no-such") || strings.Contains(answer.Text, "ended") {
			t.Errorf("call %d, of a command that is not allowed, was answered %+v", i+1, answer)
		}
	}
	if answers[9].Failed || answers[9].Ended != 0 {
		t.Errorf("the allowed command was answered %+v", answers[9])
	}
	if after := stored(t, storeDir); !slices.Equal(before, after) {
		t.Errorf("a refused command wrote to the store: %v", after)
	}
	if after := stored(t, m.config); !slices.Equal(configBefore, after) {
		t.Errorf("a refused command wrote to the config: %v", after)
	}
	for _, path := range []string{marker, saved, fresh} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("a refused command made %s", path)
		}
	}
}

func quote(text string) string {
	data, _ := json.Marshal(text)
	return string(data)
}

// A model's --allow takes only commands of the table, and a program on
// PATH is never run for a model.
func Test_client_model_runs_no_program(t *testing.T) {
	m := machine{t, t.TempDir()}
	dir := standIn(t, "frob", `echo "the program ran"`)
	if status, out, said := m.run("", "mcp", "--allow", "frob"); status != 2 || out != "" || !strings.Contains(said, `--allow "frob": that is not a command`) {
		t.Fatalf("corbie mcp --allow frob ended %d, printed %q and said %q", status, out, said)
	}
	answers := m.model([]string{"run ls"}, []string{"frob"}, []string{"frob", "--help"})
	for i, answer := range answers {
		if !answer.Failed || !strings.Contains(answer.Text, "is not a command") || strings.Contains(answer.Text, "the program ran") {
			t.Errorf("call %d, of a program, was answered %+v", i+1, answer)
		}
	}
	// corbie start names programs beside roles, and a model allowed it is
	// still refused a program.
	for i, answer := range m.model([]string{"start"}, []string{"start", "frob"}, []string{"start", "controller", "frob", "--loud"}) {
		if !answer.Failed || !strings.Contains(answer.Text, "never run for a model") || strings.Contains(answer.Text, "the program ran") {
			t.Errorf("call %d, of corbie start with a program, was answered %+v", i+1, answer)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("the program was run for the model")
	}
}

// A pipe's writing end is the model's host; closing it is the host closing
// the connection.
func Test_client_model_server_finishes_calls_when_input_ends(t *testing.T) {
	m := machine{t, t.TempDir()}
	marker := filepath.Join(t.TempDir(), "started")
	dir := files(t, map[string]string{"slow.js": `
		fs.write(` + quote(marker) + `, { text: "started" })
		const until = Date.now() + 300
		while (Date.now() < until) {}
		return "finished"`})
	in, toServer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var out, errOut said
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(context.Background(), []string{"mcp", "--allow", "test", "--config", m.config}, in, &out, &errOut)
	}()
	call := func(id int, args ...string) {
		line, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{"name": "corbie", "arguments": map[string]any{"args": args}}})
		if _, err := toServer.Write(append(line, '\n')); err != nil {
			t.Error(err)
		}
	}
	// Two calls are sent, and the connection is closed while the first is
	// still running its script.
	call(1, "test", filepath.Join(dir, "slow.js"), "--allow", "fs.write:"+marker)
	call(2, "test", filepath.Join(dir, "slow.js"), "--allow", "fs.write:"+marker)
	until(t, "the first call's script", func() bool { _, err := os.Stat(marker); return err == nil })
	if strings.Contains(out.String(), "finished") {
		t.Fatal("the first call was answered before the connection was closed: the test closed it too late")
	}
	toServer.Close()
	select {
	case status := <-done:
		if status != 0 {
			t.Fatalf("the server ended %d: %s", status, errOut.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the server did not stop after its input ended")
	}
	// Every call that had been sent was finished and answered.
	answers := lines(t, out.String())
	if len(answers) != 2 || answers[0]["id"].(float64)+answers[1]["id"].(float64) != 3 {
		t.Fatalf("the call that was running when the connection closed was answered:\n%s", out.String())
	}
	for _, answer := range answers {
		if text := answer["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, `finished`) {
			t.Errorf("a call was answered half done: %s", text)
		}
	}
}

const door = `statechart: door
initial: closed
states:
  closed:
    ticket: knock
    on: { knock: opened }
  opened: { end: done }
`

// A call that stays keeps no other call from being answered, and ends when
// the model's host stops it.
func Test_client_model_server_answers_beside_a_call_that_stays(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{"door.yaml": door})
	id := strings.TrimSpace(m.ok("", "run", "submit", filepath.Join(dir, "door.yaml")))
	in, toServer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var out, errOut said
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(context.Background(), []string{"mcp", "--allow", "run show", "--config", m.config}, in, &out, &errOut)
	}()
	send := func(v map[string]any) {
		line, _ := json.Marshal(v)
		if _, err := toServer.Write(append(line, '\n')); err != nil {
			t.Error(err)
		}
	}
	call := func(n int, args ...string) {
		send(map[string]any{"jsonrpc": "2.0", "id": n, "method": "tools/call",
			"params": map[string]any{"name": "corbie", "arguments": map[string]any{"args": args}}})
	}
	// The first call waits for a run that waits on a key, which nothing
	// answers. The second is answered while the first stays.
	call(1, "run", "show", id, "--wait")
	call(2, "run", "show", id)
	until(t, "the answer to the call made beside the one that stays", func() bool { return strings.Contains(out.String(), `"id":2`) })
	if strings.Contains(out.String(), `"id":1`) {
		t.Fatalf("a wait for a run that has not ended was answered: %s", out.String())
	}
	// The host stops the first call and closes the connection: the server
	// stops, having answered only the second.
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": 1}})
	toServer.Close()
	select {
	case status := <-done:
		if status != 0 {
			t.Fatalf("the server ended %d: %s", status, errOut.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the call the host stopped went on, and the server did not stop")
	}
	if answers := lines(t, out.String()); len(answers) != 1 || answers[0]["id"] != 2.0 {
		t.Fatalf("with one call stopped and one answered, the server printed:\n%s", out.String())
	}
}

// show --record prints what show --watch prints, without staying: the
// entries of the record so far, then the run as it stands.
func TestShowRecordIsWatchWithoutStaying(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{"door.yaml": door})
	id := strings.TrimSpace(m.ok("", "run", "submit", filepath.Join(dir, "door.yaml")))
	until(t, "the run's first step", func() bool { return strings.Contains(m.ok("", "run", "show", id), "waiting on ticket knock") })
	// For a run in progress it returns, where --watch would stay.
	out := m.ok("", "run", "show", id, "--record")
	if !strings.Contains(out, "step 1  <- start") || !strings.HasSuffix(out, m.ok("", "run", "show", id)) {
		t.Fatalf("show --record of a run in progress printed:\n%s", out)
	}
	var tickets map[string]string
	if err := json.Unmarshal([]byte(m.ok("", "run", "show", id, "--tickets", "--json")), &tickets); err != nil || tickets["knock"] == "" {
		t.Fatalf("the run's tickets were %v (%v)", tickets, err)
	}
	m.ok("", "run", "reply", id, tickets["knock"], "yes")
	m.ok("", "run", "show", id, "--wait")
	// For one that has ended the two print the same, as text and as JSON.
	for _, as := range [][]string{nil, {"--json"}} {
		record := m.ok("", append([]string{"run", "show", id, "--record"}, as...)...)
		watch := m.ok("", append([]string{"run", "show", id, "--watch"}, as...)...)
		if record != watch || !strings.Contains(record, "done") {
			t.Errorf("show --record %v printed:\n%s\nand show --watch:\n%s", as, record, watch)
		}
	}
}

// What a model is told of a command that stays says that it stays.
func Test_client_model_told_which_commands_stay(t *testing.T) {
	m := machine{t, t.TempDir()}
	var allow []string
	for _, c := range cli.Commands {
		allow = append(allow, "--allow", c.Name)
	}
	out := m.ok(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n", append([]string{"mcp"}, allow...)...)
	var listed struct {
		Result struct {
			Tools []struct{ Name, Description string }
		}
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil || len(listed.Result.Tools) != 2 {
		t.Fatalf("the tools were listed as %s", out)
	}
	told := listed.Result.Tools[1].Description
	described := map[string]string{}
	for _, line := range strings.Split(told, "\n") {
		for _, c := range cli.Commands {
			if strings.HasPrefix(line, "corbie "+c.Name+" ") && len(c.Name) > len(described[line]) {
				described[line] = c.Name
			}
		}
	}
	lineOf := map[string]string{}
	for line, name := range described {
		lineOf[name] = line
	}
	for _, c := range cli.Commands {
		line := lineOf[c.Name]
		if line == "" {
			t.Errorf("a model allowed corbie %s is told nothing of it", c.Name)
		}
		if says := strings.Contains(line, "this call stays until"); says != c.Stays {
			t.Errorf("corbie %s stays: %v; a model is told: %s", c.Name, c.Stays, line)
		}
	}
	// The commands that always stay, and those that stay when asked to.
	for _, name := range []string{"test", "start", "manual web", "mcp"} {
		if !strings.Contains(lineOf[name], "this call stays until") {
			t.Errorf("a model is not told that corbie %s stays: %s", name, lineOf[name])
		}
	}
	for _, want := range []string{"run show and run submit also stay until the run ends when given --wait or --watch", "run ls --watch stays until it is stopped",
		"a run waiting on a ticket cannot be answered from inside the same call"} {
		if !strings.Contains(told, want) {
			t.Errorf("a model is not told: %s\nIt is told:\n%s", want, told)
		}
	}
	// A model allowed nothing that stays is told of none.
	out = m.ok(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n", "mcp", "--allow", "run cancel")
	if strings.Contains(out, "this call stays until") {
		t.Errorf("a model allowed only run cancel is told %s", out)
	}
}
