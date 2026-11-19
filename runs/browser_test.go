package runs

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie-ui/page/pagetest"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
)

// The tests of what the Runs pages draw run in Chrome, on the pages the
// handlers serve, over a store in memory holding a review waiting on its
// ticket, a lookup whose stage started a review that waits on its own, and
// a flow that failed.

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
// they ran, and the IDs of the lookup and the review it started.
type lab struct {
	handler        http.Handler
	mu             sync.Mutex
	commands       []string
	lookup, review string
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
	b := &lab{lookup: ids[0]}
	b.review = waitFor(t, l.Client, func(r *client.Run) bool { return r.Parent == b.lookup && len(r.Tickets) == 1 })
	finished(t, l.Client, ids[1])
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
	b.mu.Lock()
	defer b.mu.Unlock()
	if got := strings.Join(b.commands, "\n"); got != "corbie run cancel "+b.lookup+"\ncorbie run send "+b.review+" verdict {\"ok\":true}" {
		t.Errorf("the pages ran:\n%s", got)
	}
}
