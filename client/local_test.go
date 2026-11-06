package client_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/agent"
	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
	_ "github.com/zummed/corbie/statechart"
)

// The test binary stands in for the process a runner starts for a job.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	os.Exit(m.Run())
}

var ctx = context.Background()

func local(t *testing.T, ceiling ...string) *client.Local {
	t.Helper()
	l, err := client.NewLocal(ctx, permission.Layer{Allow: ceiling}, func(role string, err error) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", role, err)
	})
	if err != nil {
		t.Fatal(err)
	}
	l.Runner.Command = []string{os.Args[0], runner.JobArg}
	t.Cleanup(l.Close)
	return l
}

// setSecret sets a secret at /, for every run of the controller.
func setSecret(t *testing.T, l *client.Local, name, value string) {
	t.Helper()
	if err := l.SetSecret(ctx, "/", name, value); err != nil {
		t.Fatal(err)
	}
}

// recorded submits a definition as a tree of one file at / and its name, and
// gives its location.
func recorded(t *testing.T, l *client.Local, d *machine.Draft) string {
	t.Helper()
	at := "/" + d.Definition.Name
	message, err := l.Submit(ctx, &client.Tree{Prefix: at, Machines: []*client.Machine{{Path: d.Definition.Name + ".yaml", Location: at, Draft: d}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := l.WaitReceipt(ctx, message, 0, 10*time.Millisecond); err != nil || !receipt.Taken {
		t.Fatalf("the submit of %s gave %+v, %v", at, receipt, err)
	}
	return at
}

func draft(t *testing.T, src string, files map[string]string) *machine.Draft {
	t.Helper()
	d, err := machine.Parse([]byte(dedent(src)), func(path string) ([]byte, error) {
		if contents, ok := files[path]; ok {
			return []byte(dedent(contents)), nil
		}
		return nil, fmt.Errorf("no such file")
	})
	if err != nil {
		t.Fatalf("the definition does not pass:\n%v", err)
	}
	return d
}

// run starts a definition and waits for it to end.
func run(t *testing.T, l *client.Local, src string, files map[string]string, input any) *client.Run {
	t.Helper()
	id, err := l.Run(ctx, draft(t, src, files), client.Start{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := l.Wait(waitCtx, id, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("waiting for %s: %v (it is %+v)", id, err, r)
	}
	return r
}

func dedent(text string) string {
	lines := strings.Split(strings.Trim(text, "\n"), "\n")
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, "\t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for i, line := range lines {
		if indent > 0 && len(line) >= indent {
			lines[i] = line[indent:]
		}
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\t", "  ") + "\n"
}

const sum = `
	statechart: sum
	input: { a: any }
	initial: adding
	states:
	  adding:
	    job: { script: ./add.js, files: [./lib.js], input: { a: "${input.a}", b: 2 } }
	    on: { done: finished }
	  finished: { end: done, output: "${states.adding.output}" }
`

func TestScriptRuns(t *testing.T) {
	l := local(t)
	r := run(t, l, sum, map[string]string{
		"add.js": `
			const lib = require("./lib.js")
			log("adding", input.a, "and", input.b)
			return { sum: lib.add(input.a, input.b), request: job.request.length > 0 }
		`,
		"lib.js": `exports.add = (a, b) => a + b`,
	}, map[string]any{"a": 40})
	if r.Status != "done" || !reflect.DeepEqual(r.Output, map[string]any{"sum": 42.0, "request": true}) {
		t.Fatalf("the run ended %s with %v (%s)", r.Status, r.Output, r.Error)
	}
}

func TestScriptThrows(t *testing.T) {
	l := local(t)
	r := run(t, l, sum, map[string]string{
		"add.js": "const x = 1\nthrow new Error(\"told to fail\")\n",
		"lib.js": ``,
	}, map[string]any{"a": 1})
	if r.Status != "failed" || r.Failure != "error" || !strings.Contains(r.Error, "adding: told to fail at line 2, column 7") {
		t.Fatalf("the run ended %s/%s with %q", r.Status, r.Failure, r.Error)
	}
}

func TestMissingInput(t *testing.T) {
	l := local(t)
	r := run(t, l, sum, map[string]string{"add.js": "return 1", "lib.js": ""}, map[string]any{})
	if r.Status != "failed" || !strings.Contains(r.Error, "missing property 'a'") {
		t.Fatalf("the run ended %s with %q", r.Status, r.Error)
	}
}

func startInput(input any) client.Start { return client.Start{Input: input} }

func wait(t *testing.T, l *client.Local, id string) *client.Run {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := l.Wait(waitCtx, id, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("waiting for %s: %v (it is %+v)", id, err, r)
	}
	return r
}

// waitForFinished waits until a run of the machine named has ended, and
// gives its id.
func waitForFinished(t *testing.T, l *client.Local, name string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := l.List(ctx, client.Filter{Finished: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			if r.Machine == name && r.Status != "running" {
				return r.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no run of %s ended", name)
	return ""
}

// waitForTicket waits until a run below the one named waits on a ticket, and
// gives that instance and the ticket's value, read from its record.
func waitForTicket(t *testing.T, l *client.Local, root string) (string, string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := l.List(ctx, client.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			if len(r.Tickets) == 0 {
				continue
			}
			tickets, err := l.Tickets(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range tickets {
				return r.ID, value
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no run below %s came to wait on a ticket", root)
	return "", ""
}

func ownerOf() store.Party { return store.Party{Kind: store.Owner} }

func pad8(n int) string { return fmt.Sprintf("%08d", n) }

func machineParse(src string, files map[string]string) (*machine.Draft, error) {
	return machine.Parse([]byte(dedent(src)), func(path string) ([]byte, error) {
		if contents, ok := files[path]; ok {
			return []byte(dedent(contents)), nil
		}
		return nil, fmt.Errorf("no such file")
	})
}
