package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noted is a script that reads the file its input names, and two files
// for it to read. inputOf gives the input that names one.
func noted(t *testing.T) (script, note, other string) {
	t.Helper()
	dir := files(t, map[string]string{"reads.js": `return fs.read(input.path)`, "note.txt": "the note", "other.txt": "another"})
	// A file is named where it really is, as a permission is checked.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "reads.js"), filepath.Join(real, "note.txt"), filepath.Join(real, "other.txt")
}

func inputOf(file string) string {
	data, _ := json.Marshal(map[string]string{"path": file})
	return string(data)
}

// A ceiling is given a deny list beside its allow list: when a machine is
// set up, when a controller enrols a client and when a runner trusts a
// controller. What it denies is refused whatever the ceiling allows, the
// refusal names the ceiling, and the rest of what it allows still passes.
func TestCeilingIsGivenADenyList(t *testing.T) {
	script, note, other := noted(t)
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	if out := m.ok("", "init", "--store", storeDir, "--name", "box", "--ceiling", "all", "--deny", "fs.read:"+note); !strings.Contains(out, "it refuses: fs.read:"+note) {
		t.Fatalf("init with a deny entry printed:\n%s", out)
	}
	m.staying("run", "controller", "runner", "--interval", "20ms")
	const client, runner = "the ceiling of the client \"box-client\"", "the ceiling of the runner \"box-runner\""
	// reads runs the script on a file with everything allowed on the
	// command line, and gives which layers refused the file, if any did.
	reads := func(file string) string {
		t.Helper()
		_, out, said := m.run("", "submit", script, "--allow", "all", "--input", inputOf(file), "--wait", "--json")
		var run struct{ Status, Refused, Layer string }
		if err := json.Unmarshal([]byte(out), &run); err != nil {
			t.Fatalf("submit printed %q and said %q", out, said)
		}
		if (run.Status != "done" || run.Layer != "") && run.Refused != "fs.read:"+file {
			t.Fatalf("the run ended %s", out)
		}
		return run.Layer
	}
	// denied says that the note is refused by these layers and no others,
	// and that the other file is read.
	denied := func(when, by string) {
		t.Helper()
		if layer := reads(note); layer != by {
			t.Fatalf("%s, the file denied was refused by %q, want %q", when, layer, by)
		}
		if layer := reads(other); layer != "" {
			t.Fatalf("%s, a file not denied was refused by %q", when, layer)
		}
	}
	denied("set up with a ceiling that denies the file", client+" and by "+runner)
	// A ceiling set again holds only what it is then given.
	m.ok("", "controller", "enrol", "box-client", "--ceiling", "all")
	m.ok("", "runner", "trust", "box-controller", "--ceiling", "all")
	denied("with both ceilings set again with no deny list", "")
	// One entry is added to the deny list of a ceiling that is there.
	m.ok("", "controller", "enrol", "box-client", "--deny", "fs.read:"+note)
	if out := m.ok("", "controller", "parties"); !strings.Contains(out, "ceiling: all  deny: fs.read:"+note) {
		t.Fatalf("the controller's parties: %s", out)
	}
	denied("with the client's ceiling denying the file", client)
	// An entry added to either list leaves the other as it was.
	m.ok("", "controller", "enrol", "box-client", "--allow", "net:example.com")
	if out := m.ok("", "controller", "parties"); !strings.Contains(out, "ceiling: all,net:example.com  deny: fs.read:"+note) {
		t.Fatalf("the controller's parties: %s", out)
	}
	// Enrolled again with no ceiling named, the ceiling stays as it was, and
	// only --ceiling none takes it away.
	m.ok("", "controller", "enrol", "box-client")
	if out := m.ok("", "controller", "parties"); !strings.Contains(out, "ceiling: all,net:example.com  deny: fs.read:"+note) {
		t.Fatalf("enrolled again with nothing named, the controller's parties: %s", out)
	}
	m.ok("", "controller", "enrol", "box-client", "--ceiling", "none")
	if out := m.ok("", "controller", "parties"); strings.Contains(out, "ceiling: all") || strings.Contains(out, "deny: fs.read") {
		t.Fatalf("with --ceiling none, the controller's parties: %s", out)
	}
	m.ok("", "controller", "enrol", "box-client", "--ceiling", "all")
	m.ok("", "runner", "trust", "box-controller", "--deny", "fs.read:"+note)
	denied("with the runner's ceiling denying the file", runner)
	// The same holds for the ceiling a runner gives.
	m.ok("", "runner", "trust", "box-controller")
	denied("with the runner trusting again and nothing named", runner)
	// An entry that is no permission is a mistake, and names its flag.
	for _, mistaken := range [][]string{
		{"init", "--store", storeDir, "--deny", "nonsense"},
		{"controller", "enrol", "box-client", "--deny", "nonsense"},
		{"runner", "trust", "box-controller", "--deny", "nonsense"},
	} {
		if status, _, said := m.run("", mistaken...); status != 2 || !strings.Contains(said, "--deny: ") {
			t.Errorf("corbie %s ended %d: %s", strings.Join(mistaken, " "), status, said)
		}
	}
}

// A start is given a deny list beside its allow list, for a definition as
// for a script file: what it denies is refused whatever the start allows,
// and the refusal names the start's list.
func TestStartIsGivenADenyList(t *testing.T) {
	script, note, _ := noted(t)
	input := inputOf(note)
	definition := filepath.Join(filepath.Dir(script), "reads.yaml")
	os.WriteFile(definition, []byte("flow: reads\nallow: [disk]\ninput: [path]\nstages:\n  it: { job: { script: ./reads.js, input: { path: \"${input.path}\" } } }\noutput: ${stages.it.output}\n"), 0o644)
	m := machine{t, t.TempDir()}
	for file, layer := range map[string]string{definition: "the list the run was started with", script: "the job \"script\" in the definition of \"reads\""} {
		if out := m.ok("", "test", file, "--input", input, "--allow", "disk", "--json"); !strings.Contains(out, `"output":"the note"`) {
			t.Fatalf("corbie test %s --allow disk printed %s", file, out)
		}
		status, out, _ := m.run("", "test", file, "--input", input, "--allow", "disk", "--deny", "fs.read:"+note, "--json")
		var run struct{ Refused, Layer string }
		json.Unmarshal([]byte(out), &run)
		if status != 1 || run.Refused != "fs.read:"+note || run.Layer != layer {
			t.Fatalf("corbie test %s --allow disk --deny the file ended %d: %s", file, status, out)
		}
	}
	if status, _, said := m.run("", "test", definition, "--input", input, "--deny", "nonsense"); status != 2 || !strings.Contains(said, "--deny: ") {
		t.Fatalf("a deny entry that is no permission ended %d: %s", status, said)
	}
}
