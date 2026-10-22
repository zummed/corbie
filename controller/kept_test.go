package controller_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A job is sent the key of the deepest kept path at or below which lies
// everything every layer the controller knows of allows of kept values,
// and no key where some layer allows none.
func TestJobIsSentTheKeyOfTheKeptPathItsLayersAllow(t *testing.T) {
	for _, c := range []struct {
		allow, start, ceiling, deny []string
		path                        string
		sent                        bool
	}{
		{allow: []string{"kept.read:notes/**"}, ceiling: []string{"all"}, path: "notes", sent: true},
		{allow: []string{"kept.read:notes/**", "kept.write:notes/tea"}, start: []string{"kept.read:notes/tea"}, ceiling: []string{"all"}, path: "notes/tea", sent: true},
		{allow: []string{"kept"}, ceiling: []string{"kept.read:house/**", "disk"}, path: "house", sent: true},
		{allow: []string{"all"}, ceiling: []string{"all"}, path: "", sent: true},
		{allow: []string{"kept.read:notes/**"}, ceiling: []string{"disk"}},
		{allow: []string{"kept.read:notes/**"}, ceiling: []string{"all"}, deny: []string{"kept"}},
		{allow: []string{"kept.read:notes/**"}, start: []string{"kept.read:other/**"}, ceiling: []string{"all"}},
		{allow: []string{"network"}, ceiling: []string{"all"}},
	} {
		w := newWorld(t)
		r1 := w.party("r1", identity.Runner)
		w.enrol(r1, permission.Layer{}, nil)
		w.enrol(w.c1, permission.Layer{Allow: c.ceiling, Deny: c.deny}, nil)
		start := client.Start{}
		if c.start != nil {
			start.Allow = &permission.Layer{Allow: c.start}
		}
		if _, err := w.client.Start(ctx, w.draft(fmt.Sprintf(`
			statechart: keeps
			initial: working
			states:
			  working:
			    job: { script: ./work.js, allow: [%s] }
			    on: { done: finished }
			  finished: { end: done }
		`, strings.Join(c.allow, ", ")), map[string]string{"work.js": "return 1"}), start); err != nil {
			t.Fatal(err)
		}
		w.settle()
		jobs := w.list(store.Inbox("r1"))
		if len(jobs) != 1 {
			t.Fatalf("the jobs sent are %v", jobs)
		}
		msg, _ := w.sent(r1, jobs[0])
		kept := msg.Job.Kept
		if !c.sent {
			if kept != nil {
				t.Errorf("allowed %v, started with %v, under %v: the job was sent the key of %q", c.allow, c.start, c.ceiling, kept.Path)
			}
			continue
		}
		top, err := w.ctl.KeptKey()
		if err != nil {
			t.Fatal(err)
		}
		want, _ := store.KeptKey(top, "", c.path)
		if kept == nil || kept.Path != c.path || !bytes.Equal(kept.Key, want) {
			t.Errorf("allowed %v, started with %v, under %v: the job was sent %+v, want the key of %q", c.allow, c.start, c.ceiling, kept, c.path)
		}
	}
}
