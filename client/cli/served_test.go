package cli_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// freePort gives host:port on this computer that nothing listens on.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// desk is a machine that holds a directory and its controller and serves the
// directory, with the address others reach it at.
func desk(t *testing.T, roles ...string) (machine, string) {
	t.Helper()
	at := freePort(t)
	m := machine{t, filepath.Join(t.TempDir(), "desk")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "desk", "--ceiling", "all")
	out := m.ok("", "init", "store", "--listen", at)
	if out != "Other computers reach the directory at corbie://"+at+" while corbie start runs here.\n" {
		t.Fatalf("init store --listen printed:\n%s", out)
	}
	if len(roles) == 0 {
		return m, "corbie://" + at
	}
	args := []string{"start"}
	for _, r := range roles {
		args = append(args, r)
		if r == "controller" || r == "runner" {
			args = append(args, "--interval", "20ms")
		}
	}
	_, said := m.staying(args...)
	until(t, "the store being served", func() bool { return strings.Contains(said.String(), "served at corbie://"+at) })
	return m, "corbie://" + at
}

// joins enrols a party of another machine with the desk, by the one line
// of the parties page, and gives that machine.
func joins(t *testing.T, d machine, address, kind, name string, enrol ...string) machine {
	t.Helper()
	m := machine{t, filepath.Join(t.TempDir(), name)}
	card := m.ok("", kind, "card", "--name", name)
	enrolled := d.ok(card, append([]string{"controller", "enrol", "-"}, enrol...)...)
	if lines := strings.Split(strings.TrimSpace(enrolled), "\n"); len(lines) != 2 || lines[1] != address {
		t.Fatalf("enrol on a machine that serves its directory printed %q, and not the address others reach it at", enrolled)
	}
	trust := []string{kind, "trust", "-"}
	if kind == "runner" {
		trust = append(trust, "--ceiling", "all")
	}
	m.ok(enrolled, trust...)
	return m
}

func TestADirectoryServedToOtherMachines(t *testing.T) {
	d, address := desk(t, "controller", "runner", "store")
	gpu := joins(t, d, address, "runner", "gpu", "--tags", "gpu")
	gpu.staying("start", "runner", "--interval", "20ms")
	laptop := joins(t, d, address, "client", "laptop", "--ceiling", "all")

	// A client on one machine starts a run whose job is for the runner on
	// another: both reach the store through the desk.
	dir := files(t, map[string]string{"greet.js": `return { text: "hello " + input.name }`})
	out := laptop.ok("", "run", "submit", filepath.Join(dir, "greet.js"), "--tag", "gpu", "--input", `{"name": "world"}`, "--wait")
	if !strings.Contains(out, `"text":"hello world"`) {
		t.Fatalf("a run started and done on other computers printed:\n%s", out)
	}
	if status, f := d.check("--tag", "gpu"); status != 0 {
		t.Fatalf("the check of the runner on another computer found %+v", f)
	}
	var runs []struct{ Machine string }
	if err := json.Unmarshal([]byte(laptop.ok("", "run", "ls", "--all", "--json")), &runs); err != nil || len(runs) != 1 || runs[0].Machine != "greet" {
		t.Fatalf("the client on another computer lists %v (%v): its own run and not the desk's", runs, err)
	}

	// A party the controller removes is turned away from then on.
	d.ok("", "controller", "remove", "laptop")
	if status, _, said := laptop.run("", "run", "ls"); status != 1 || !strings.Contains(said, "no party of this store's controller") {
		t.Fatalf("a removed client was answered: %d %s", status, said)
	}
}

func TestServingIsTheControllersComputersAndADirectorysOnly(t *testing.T) {
	// A machine that was never told where it serves does not serve.
	plain := machine{t, filepath.Join(t.TempDir(), "plain")}
	plain.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "plain")
	if status, _, said := plain.run("", "start", "store"); status != 1 || !strings.Contains(said, "corbie init store sets it up") {
		t.Fatalf("corbie start store with nowhere to serve: %d %s", status, said)
	}
	// A bucket is not served, and store --listen says what it takes.
	if status, _, said := plain.run("", "init", "--store", "s3://bucket/main?region=eu-west-2", "store"); status != 2 || !strings.Contains(said, "a directory on this computer") {
		t.Fatalf("store with a bucket: %d %s", status, said)
	}
	if status, _, said := plain.run("", "init", "--store", filepath.Join(t.TempDir(), "s"), "store", "--listen", "8372"); status != 2 || !strings.Contains(said, "host:port") {
		t.Fatalf("store --listen with no port: %d %s", status, said)
	}
	// A machine with no controller serves nothing, and is told that.
	only := machine{t, filepath.Join(t.TempDir(), "only")}
	only.ok("", "runner", "card", "--name", "only")
	if status, _, said := only.run("", "start", "store"); status != 1 || !strings.Contains(said, "holds its controller") {
		t.Fatalf("corbie start store with no controller: %d %s", status, said)
	}
	// The address others reach a served directory by is no store for a
	// controller.
	if status, _, said := plain.run("", "init", "--store", "corbie://desk:8372"); status != 2 || !strings.Contains(said, "its own directory") {
		t.Fatalf("init with a served directory's address: %d %s", status, said)
	}
	// init store sets up the store's server alone: run again, init leaves
	// it as it is, and --listen written empty serves nothing.
	was := machine{t, filepath.Join(t.TempDir(), "was")}
	wasStore := filepath.Join(t.TempDir(), "s")
	was.ok("", "init", "--store", wasStore, "--name", "was")
	was.ok("", "init", "store", "--listen", "127.0.0.1:8372")
	if out := was.ok("", "init", "--store", wasStore, "--name", "was"); !strings.Contains(out, "corbie://127.0.0.1:8372") {
		t.Fatalf("init again forgot the store's server:\n%s", out)
	}
	if out := was.ok("", "init", "store", "--listen", ""); out != "This computer serves its store directory to no other computer.\n" {
		t.Fatalf("init store --listen \"\" printed %q", out)
	}
	if status, _, said := was.run("", "start", "store"); status != 1 || !strings.Contains(said, "corbie init store sets it up") {
		t.Fatalf("corbie start store after init store --listen \"\": %d %s", status, said)
	}
	// init store makes no controller: on a config with none it is refused
	// and makes nothing, and it sets up nothing but the store's server.
	fresh := machine{t, filepath.Join(t.TempDir(), "fresh")}
	if status, _, said := fresh.run("", "init", "store"); status != 1 || !strings.Contains(said, "corbie init sets the controller up first") {
		t.Fatalf("init store on a fresh config: %d %s", status, said)
	}
	if _, err := os.Stat(fresh.config); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init store on a fresh config made %s", fresh.config)
	}
	if status, _, said := was.run("", "init", "--ceiling", "all", "store"); status != 2 || !strings.Contains(said, "as two commands") {
		t.Fatalf("init --ceiling all store: %d %s", status, said)
	}
	// Someone with no identity is not shown a served directory: a watcher
	// there is a client.
	d, address := desk(t, "store")
	card := d.ok("", "controller", "card")
	viewer := machine{t, filepath.Join(t.TempDir(), "viewer")}
	viewer.ok(strings.TrimSpace(card)+" "+address, "client", "trust", "-")
	if status, _, said := viewer.run("", "run", "ls"); status != 1 || !strings.Contains(said, "corbie client card makes one") {
		t.Fatalf("a viewer with no identity on a served directory: %d %s", status, said)
	}
}

// Several runners on other computers share the work of one flow through a
// served directory, and every job is run once.
func TestManyRunnersOnAServedDirectory(t *testing.T) {
	d, address := desk(t, "controller", "store")
	for n := range 5 {
		name := fmt.Sprintf("r%d", n)
		joins(t, d, address, "runner", name, "--tags", "farm").staying("start", "runner", "--jobs", "4", "--interval", "2s")
	}
	laptop := joins(t, d, address, "client", "laptop", "--ceiling", "all")
	const items = 150
	ran := t.TempDir()
	dir := files(t, map[string]string{
		"farm.yaml": `flow: farm
input: { items: any }
stages:
  work:
    each: ${input.items}
    job: { script: ./work.js, input: { i: "${item}" }, tags: [farm], allow: ["fs.write:` + ran + `/**"] }
output: ${stages.work.output}
`,
		// One file for each time an item is run.
		"work.js": `fs.write("` + ran + `/" + input.i + "." + Math.floor(Math.random() * 1e9), { text: "" })
return input.i
`})
	list := make([]int, items)
	for i := range list {
		list[i] = i
	}
	input, _ := json.Marshal(map[string]any{"items": list})
	var run struct {
		Status string
		Output []struct{ Output int }
	}
	out := laptop.ok("", "run", "submit", filepath.Join(dir, "farm.yaml"), "--input", string(input), "--wait", "--json")
	if err := json.Unmarshal([]byte(out), &run); err != nil || run.Status != "done" || len(run.Output) != items {
		t.Fatalf("a flow of %d jobs over five runners ended %s with %d outputs (%v)", items, run.Status, len(run.Output), err)
	}
	for i, o := range run.Output {
		if o.Output != i {
			t.Fatalf("output %d is %d", i, o.Output)
		}
	}
	entries, err := os.ReadDir(ran)
	if err != nil || len(entries) != items {
		t.Fatalf("%d jobs were run for %d items (%v): one was run twice, or not at all", len(entries), items, err)
	}
}

// The store's server listens at the port other computers are given, on every
// network interface, and --listen changes where it listens and not the
// address they are given.
func TestTheStoreListensWhereItIsTold(t *testing.T) {
	d, address := desk(t)
	at := strings.TrimPrefix(address, "corbie://")
	elsewhere := freePort(t)
	_, said := d.staying("start", "store", "--listen", elsewhere)
	until(t, "the store's server", func() bool { return strings.Contains(said.String(), "listening on "+elsewhere) })
	if !strings.Contains(said.String(), "served at "+address) {
		t.Fatalf("with --listen, the store's server said:\n%s", said.String())
	}
	if c, err := net.Dial("tcp", elsewhere); err != nil {
		t.Fatalf("nothing listens where --listen said: %v", err)
	} else {
		c.Close()
	}
	if c, err := net.Dial("tcp", at); err == nil {
		c.Close()
		t.Fatalf("with --listen, something listens at %s as well", at)
	}
	if status, _, mistake := d.run("", "start", "controller", "--listen", elsewhere); status != 2 || !strings.Contains(mistake, "--listen is for the gateway or the store, not the controller") {
		t.Fatalf("--listen after the controller ended %d and said %q", status, mistake)
	}
}

// store with no --listen listens on every network interface at port 8372,
// and other computers are given this computer's host name.
func TestAServedDirectoryByDefault(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "desk")}
	m.ok("", "init", "--store", filepath.Join(t.TempDir(), "store"), "--name", "desk")
	out := m.ok("", "init", "store")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	want := "corbie://" + net.JoinHostPort(host, "8372")
	if !strings.Contains(out, "Other computers reach the directory at "+want+" ") {
		t.Fatalf("init store printed:\n%s", out)
	}
	gpu := machine{t, filepath.Join(t.TempDir(), "gpu")}
	enrolled := m.ok(gpu.ok("", "runner", "card", "--name", "gpu"), "controller", "enrol", "-")
	if lines := strings.Split(strings.TrimSpace(enrolled), "\n"); len(lines) != 2 || lines[1] != want {
		t.Fatalf("enrol printed %q, and not %s", enrolled, want)
	}
}
