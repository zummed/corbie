package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zummed/corbie/client/cli"
)

// onlyJSON says whether what a command printed is JSON and nothing else:
// one value, or one value on each line.
func onlyJSON(out string) bool {
	if json.Valid([]byte(out)) {
		return true
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if !json.Valid([]byte(line)) {
			return false
		}
	}
	return true
}

// Every command in the table, asked for JSON, prints JSON and nothing else
// on standard output: its result where it has one, and where it has none,
// nothing.
func TestEveryCommandAskedForJSONPrintsOnlyJSON(t *testing.T) {
	tried := map[string]bool{}
	// asked runs one command with --json. A command with a result prints
	// it as JSON, whichever way the command ends.
	asked := func(m machine, stdin string, result bool, args ...string) string {
		t.Helper()
		cmd, _ := cli.Find(args)
		if cmd == nil {
			t.Fatalf("corbie %s is no command", strings.Join(args, " "))
		}
		tried[cmd.Name] = true
		status, out, said := m.run(stdin, append(args, "--json")...)
		switch {
		case status == 2:
			t.Errorf("corbie %s --json was written wrong: %s", strings.Join(args, " "), said)
		case result && (out == "" || !onlyJSON(out)):
			t.Errorf("corbie %s --json ended %d and printed, where its result as JSON was expected:\n%s%s", strings.Join(args, " "), status, out, said)
		case !result && out != "":
			t.Errorf("corbie %s --json has no result, and printed:\n%s", strings.Join(args, " "), out)
		}
		return out
	}
	object := func(out string) map[string]any {
		t.Helper()
		var v map[string]any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("what was printed is no JSON object: %s", out)
		}
		return v
	}

	// One machine with everything on it.
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	set := object(asked(m, "", true, "init", "--store", storeDir, "--name", "box", "--ceiling", "all"))
	if set["store"] != storeDir || set["config"] != m.config || len(set["parties"].([]any)) != 3 || len(set["ceiling"].([]any)) != 1 {
		t.Errorf("corbie init --json printed %v", set)
	}
	tried["start"] = true
	roles, _ := m.staying("start", "controller", "--interval", "20ms", "runner", "--interval", "20ms", "--json")
	dir := files(t, map[string]string{
		"make.js": `emit("note", 1); return { report: blobs.make("the report") }`,
		"door.yaml": `statechart: door
initial: closed
states:
  closed:
    ticket: knock
    on: { open: opened, knock: opened }
  opened: { end: done }
`})
	asked(m, "", true, "test", filepath.Join(dir, "make.js"))
	asked(m, "", true, "test", filepath.Join(dir, "make.js"), "--watch")
	id, _ := object(asked(m, "", true, "run", "submit", filepath.Join(dir, "make.js")))["id"].(string)
	asked(m, "", true, "run", "submit", filepath.Join(dir, "make.js"), "--wait")
	asked(m, "", true, "run", "submit", filepath.Join(dir, "make.js"), "--watch")
	asked(m, "", true, "run", "show", id, "--wait")
	asked(m, "", true, "run", "show", id)
	asked(m, "", true, "run", "show", id, "--record")
	asked(m, "", true, "run", "show", id, "--watch")
	asked(m, "", true, "run", "show", id, "--tickets")
	asked(m, "", true, "run", "show", id, "--step", "1")
	asked(m, "", true, "run", "show", id, "--definition")
	into := filepath.Join(t.TempDir(), "saved")
	var saved []string
	if err := json.Unmarshal([]byte(asked(m, "", true, "run", "show", id, "--save", into)), &saved); err != nil || len(saved) != 1 || saved[0] != filepath.Join(into, "report") {
		t.Errorf("corbie run show --save --json printed %v (%v)", saved, err)
	}
	asked(m, "", true, "run", "ls")
	asked(m, "", true, "run", "ls", "--all")
	if found := object(asked(m, "", true, "check")); found["ok"] != true || len(found["legs"].([]any)) != 4 {
		t.Errorf("corbie check --json printed %v", found)
	}
	door := func() string {
		id, _ := object(asked(m, "", true, "run", "submit", filepath.Join(dir, "door.yaml")))["id"].(string)
		return id
	}
	asked(m, "", true, "run", "send", door(), "open")
	asked(m, "", true, "run", "reply", door(), "not-the-key", "yes")
	asked(m, "", true, "run", "cancel", door())
	asked(m, "", false, "controller", "cancel", door())
	asked(m, "", true, "controller", "parties")
	asked(m, "the value\n", false, "controller", "secret", "set", "A_SECRET_OF_THIS_TEST", "-")
	asked(m, "", true, "controller", "secret", "list")
	asked(m, "", false, "controller", "secret", "remove", "A_SECRET_OF_THIS_TEST")
	asked(m, "", true, "controller", "policy", "--viewer", "--store", "s3://bucket/prefix?region=eu-west-2")
	asked(m, "", true, "manual")
	asked(m, "", true, "manual", "runs")
	asked(m, "", true, "version")
	asked(m, `{"jsonrpc": "2.0", "id": 1, "method": "ping"}`+"\n", true, "mcp")

	// A controller on one machine, and a runner and a client on another.
	ctl := machine{t, filepath.Join(t.TempDir(), "ctl")}
	laptop := machine{t, filepath.Join(t.TempDir(), "laptop")}
	if card := object(asked(ctl, "", true, "controller", "card", "--store", filepath.Join(t.TempDir(), "store"), "--name", "ctl")); !strings.HasPrefix(card["card"].(string), "corbie-card:controller:ctl:") {
		t.Errorf("corbie controller card --json printed %v", card)
	}
	for _, kind := range []string{"runner", "client"} {
		card := object(asked(laptop, "", true, kind, "card", "--name", "laptop-"+kind))
		if !strings.HasPrefix(card["card"].(string), "corbie-card:"+kind+":laptop-"+kind+":") || card["name"] != "laptop-"+kind || card["kind"] != kind || card["fingerprint"] == "" {
			t.Errorf("corbie %s card --json printed %v", kind, card)
		}
		enrolled := object(asked(ctl, "", true, "controller", "enrol", card["card"].(string)))
		if !strings.HasPrefix(enrolled["card"].(string), "corbie-card:controller:ctl:") || enrolled["store"] == "" || len(enrolled) != 2 {
			t.Errorf("corbie controller enrol --json printed %v", enrolled)
		}
		asked(laptop, "", false, kind, "trust", enrolled["card"].(string), enrolled["store"].(string))
		asked(laptop, "", false, kind, "remove", "ctl")
	}
	asked(ctl, "", false, "controller", "remove", "laptop-runner")

	tried["manual web"] = true
	web, saying := m.staying("manual", "web", "--listen", "127.0.0.1:0", "--json")
	until(t, "the manual served", func() bool { return strings.Contains(saying.String(), "the manual is at") })
	for name, out := range map[string]string{"start": roles.String(), "manual web": web.String()} {
		if out != "" {
			t.Errorf("corbie %s --json printed:\n%s", name, out)
		}
	}
	for _, c := range cli.Commands {
		if !tried[c.Name] {
			t.Errorf("corbie %s was not tried with --json", c.Name)
		}
	}
}
