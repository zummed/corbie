package runner_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// The runner keeps, reads, lists and removes a job's kept values with the
// key the job was sent, and refuses one that was altered or that the key
// does not open.
func TestRunnerKeepsValuesWithTheKeyTheJobHolds(t *testing.T) {
	w := newWorld(t)
	key := bytes.Repeat([]byte{7}, 32)
	sent := func(j *instance.Job) { j.Kept = &instance.Kept{Path: "notes", Key: key} }
	w.job(w.ctl, "keep", `
		kept.write("notes/tea", { text: "Tea is up." })
		kept.write("notes/cake", { text: "Cake is down." })
		kept.write("notes/cake/crumb", { text: "" })
		kept.remove("notes/cake")
		return { tea: kept.read("notes/tea"), crumb: kept.read("notes/cake/crumb"), cake: kept.read("notes/cake"), listed: kept.list("notes") }
	`, sent)
	pass(t, w.runner("all"))
	got := w.results()["keep"]
	if got == nil || got.Status != instance.Done {
		t.Fatalf("the result is %+v", got)
	}
	out := got.Output.(map[string]any)
	if out["tea"] != "Tea is up." || out["crumb"] != "" || out["cake"] != nil || len(out["listed"].([]any)) != 2 {
		t.Fatalf("the job read back %v", out)
	}
	// In the store, the value is encrypted under its own path's key.
	sealed, _, _ := w.owner.Read(ctx, store.Kept("ctl", "notes/tea"))
	own, _ := store.KeptKey(key, "notes", "notes/tea")
	if value, err := store.OpenKept(own, "notes/tea", sealed); err != nil || string(value) != "Tea is up." {
		t.Fatalf("the kept value in the store opens as %q: %v", value, err)
	}

	// A value the key does not open, and one that was altered, fail the
	// job; the script can tell neither from a value that was never kept.
	w.owner.Write(ctx, store.Kept("ctl", "other/tea"), sealed, store.Overwrite)
	w.owner.Write(ctx, store.Kept("ctl", "notes/soup"), sealed, store.Overwrite)
	for job, path := range map[string]string{"elsewhere": "other/tea", "moved": "notes/soup"} {
		w.job(w.ctl, job, `return kept.read(input.path)`, func(j *instance.Job) { sent(j); j.Input = map[string]any{"path": path} })
	}
	pass(t, w.runner("all"))
	for job, says := range map[string]string{"elsewhere": "no key to the kept value at other/tea", "moved": "the kept value at notes/soup has been altered or moved"} {
		got := w.results()[job]
		if got == nil || got.Status != machine.StatusError || !strings.Contains(got.Error.Error, says) {
			t.Errorf("the job %s ended %+v", job, got)
		}
	}
}
