package runs

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"
)

// The tests of what the Runs pages draw run in Chrome, on the pages the
// handlers serve, over a store in memory holding a review waiting on its
// ticket, a lookup whose stage started a review that waits on its own, a
// flow that failed, and a desk that sent a knock to a room.

const lookup = `flow: lookup
stages:
  find:
    job: ./find.js
  check:
    job: { machine: ./review.yaml }
    follows: [find]
  tell:
    job: { script: ./tell.js, input: "${stages.check.output}" }
`

const broken = `flow: broken
stages:
  find:
    job: ./find.js
  fail:
    job: ./fail.js
    follows: [find]
`

// lab is the store the browser tests read: the UI's pages, the commands
// they ran, and the IDs of the lookup, the review it started and the desk.
type lab struct {
	handler              http.Handler
	mu                   sync.Mutex
	commands             []string
	lookup, review, desk string
}

func newLab(t *testing.T) *lab {
	t.Helper()
	l, viewer, _ := local(t)
	ctx := context.Background()
	files := machine.Given(map[string]string{"find.js": "return 'found'", "tell.js": "return 'told'", "fail.js": "throw new Error('no such person')", "review.yaml": review, "write.js": "return 'a draft'"})
	var ids []string
	for _, text := range []string{lookup, broken} {
		draft, err := machine.Parse([]byte(text), files)
		if err != nil {
			t.Fatal(err)
		}
		id, err := l.Run(ctx, draft, client.Start{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	submit(t, l, "/house", map[string]string{
		"room.yaml": "statechart: room\nlifetime: keyed\ninitial: open\nstates:\n  open:\n    on:\n      knock: open\n",
		"desk.yaml": "statechart: desk\nallow: [send:room]\ninitial: first\nstates:\n  first:\n    entry:\n      send:\n        - { to: { at: room, key: \"1\" }, event: knock }\n    timers: { pause: 1s }\n    on: { pause: last }\n  last: { end: done }\n",
	})
	desk, err := l.Start(ctx, client.Start{At: "/house/desk"})
	if err != nil {
		t.Fatal(err)
	}
	b := &lab{lookup: ids[0], desk: desk}
	b.review = waitFor(t, l.Client, func(r *client.Run) bool { return r.Parent == b.lookup && len(r.Tickets) == 1 })
	finished(t, l.Client, ids[1])
	finished(t, l.Client, desk)
	h := New(Config{List: &listed{from: viewer}, Lister: viewer, Client: l.Client, Interval: time.Second, Shell: page.Shell{Offered: []string{"Runs"}, Parties: []string{l.Client.ID.Name}},
		Command: func(ctx context.Context, c page.Call) page.Answer {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.commands = append(b.commands, c.String())
			return page.Answer{Out: `{"taken": true}`}
		}})
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.Handle("/", h)
	b.handler = mux
	return b
}

func Test_runs_in_the_browser(t *testing.T) {
	b := newLab(t)
	t.Run("list", func(t *testing.T) {
		pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/runs", Module: "runs/list_test.js"})
	})
	t.Run("run", func(t *testing.T) {
		pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/run/" + b.lookup, Module: "runs/run_test.js",
			Fixtures: map[string]any{"ids": map[string]string{"lookup": b.lookup, "review": b.review}}})
	})
	t.Run("child", func(t *testing.T) {
		pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/run/" + b.review, Module: "runs/child_test.js"})
	})
	t.Run("send", func(t *testing.T) {
		pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/run/" + b.desk, Module: "runs/send_test.js"})
	})
	b.mu.Lock()
	defer b.mu.Unlock()
	if got := strings.Join(b.commands, "\n"); got != "corbie run cancel "+b.lookup+"\ncorbie run send "+b.review+" verdict {\"ok\":true}" {
		t.Errorf("the pages ran:\n%s", got)
	}
}
