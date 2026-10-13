package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reading is a script that says what the file it was given is.
const reading = `
const bytes = blobs.read(input.report, { as: "bytes" })
return { type: input.report.type, size: input.report.size, sum: crypto.hash(bytes), n: input.n }
`

// corbie submit and corbie test give a file as input under a name: the
// run's script reads the file's bytes there, whatever they are, and the
// blob is in the new run's record.
func TestSubmitGivesAFileAsInput(t *testing.T) {
	m, storeDir := working(t, "all")
	pdf := make([]byte, 3000)
	for i := range pdf {
		pdf[i] = byte(i * 11)
	}
	dir := files(t, map[string]string{"read.js": reading, "report.pdf": string(pdf)})
	sum := sha256.Sum256(pdf)
	for _, command := range [][]string{{"submit", "--wait"}, {"test"}} {
		out := m.ok("", append(command, filepath.Join(dir, "read.js"), "--input-file", "report="+filepath.Join(dir, "report.pdf"), "--input", `{"n": 3}`, "--json")...)
		run := lines(t, out)[0]
		got, _ := run["output"].(map[string]any)
		if run["status"] != "done" || got["type"] != "application/pdf" || got["size"] != 3000.0 || got["sum"] != hex.EncodeToString(sum[:]) || got["n"] != 3.0 {
			t.Fatalf("corbie %s with a file as input printed %s", command[0], out)
		}
		if command[0] != "submit" {
			continue
		}
		// The definition, the script and the file: three blobs, in the
		// record of the run that was started.
		blobs, err := os.ReadDir(filepath.Join(storeDir, "box-controller", "records", filepath.FromSlash(run["id"].(string)), "blobs"))
		if err != nil || len(blobs) != 3 {
			t.Fatalf("the new run's record holds %d blobs (%v)", len(blobs), err)
		}
	}
	// A file given wrongly is a mistake, one that is not there a failure,
	// and neither uploads or sends anything.
	before := len(stored(t, storeDir))
	for status, flags := range map[int][]string{
		2: {"--input-file", "report"},
		1: {"--input-file", "report=" + filepath.Join(dir, "missing.pdf")},
	} {
		if got, out, said := m.run("", append([]string{"submit", filepath.Join(dir, "read.js")}, flags...)...); got != status || out != "" {
			t.Errorf("corbie submit %s ended %d: %s%s", strings.Join(flags, " "), got, out, said)
		}
	}
	twice := []string{"submit", filepath.Join(dir, "read.js"), "--input-file", "report=" + filepath.Join(dir, "report.pdf"), "--input-file", "report=" + filepath.Join(dir, "read.js")}
	if got, _, said := m.run("", twice...); got != 2 || !strings.Contains(said, "report") {
		t.Errorf("two files under one name ended %d: %s", got, said)
	}
	if got, _, said := m.run("", "submit", filepath.Join(dir, "read.js"), "--input-file", "report="+filepath.Join(dir, "report.pdf"), "--input", `[1]`); got != 1 || !strings.Contains(said, "not a map") {
		t.Errorf("a file given with an input that is a list ended %d: %s", got, said)
	}
	if after := len(stored(t, storeDir)); after != before {
		t.Fatalf("the refused commands left %d new values in the store", after-before)
	}
}

// A file of one run's result is given as input to another from the command
// line: the blob as corbie show prints it, with the run added.
func TestSubmitCarriesAFileFromAnotherRun(t *testing.T) {
	m, _ := working(t, "all")
	dir := files(t, map[string]string{
		"make.js": `return { report: blobs.make("the report", { type: "text/markdown" }) }`,
		"read.js": `return { text: blobs.read(input.doc), type: input.doc.type }`,
	})
	first := lines(t, m.ok("", "submit", filepath.Join(dir, "make.js"), "--wait", "--json"))[0]
	id, _ := first["id"].(string)
	shown, _ := first["output"].(map[string]any)["report"].(map[string]any)
	if first["status"] != "done" || shown["$blob"] != "report" {
		t.Fatalf("the first run was printed as %v", first)
	}
	input := `{"doc": {"$blob": "report", "type": "text/markdown", "size": 10, "run": "` + id + `"}}`
	out := m.ok("", "submit", filepath.Join(dir, "read.js"), "--input", input, "--wait", "--json")
	if got, _ := lines(t, out)[0]["output"].(map[string]any); got["text"] != "the report" || got["type"] != "text/markdown" {
		t.Fatalf("the second run printed %s", out)
	}
	// One that is not there fails the command, which says which.
	missing := strings.Replace(input, `"report"`, `"nothing"`, 1)
	if status, out, said := m.run("", "submit", filepath.Join(dir, "read.js"), "--input", missing); status != 1 || out != "" || !strings.Contains(said, `"nothing"`) || !strings.Contains(said, id) {
		t.Fatalf("a start carrying a file that is not there ended %d: %s%s", status, out, said)
	}
}
