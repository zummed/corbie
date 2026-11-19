package runs

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie-ui/page/pagetest"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
	// The denied machine's job calls the fs plugin, which the corbie
	// program carries among its bundled plugins.
	_ "github.com/zummed/corbie/plugin/fs"
)

// troubled is a store in memory holding a run of each machine in
// testdata/trouble, each where it goes wrong: the Runs pages over it, their
// handler, the client, and each run's ID by its machine's name.
type troubled struct {
	l       *client.Local
	h       *handler
	handler http.Handler
	ids     map[string]string
}

// The file of fails differs from the definition its run started with.
func differs(ctx context.Context, location, dir string, def *machine.Definition) ([]string, error) {
	if location != "/trouble/fails" || def.Name != "fails" {
		return nil, nil
	}
	return []string{"stages.check.job.script: ./check.js"}, nil
}

func newTrouble(t *testing.T) *troubled {
	t.Helper()
	l, viewer, _ := local(t)
	ctx := context.Background()
	files := map[string]string{}
	paths, _ := filepath.Glob("testdata/trouble/*")
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(p)] = string(data)
	}
	submit(t, l, "/trouble", files)
	ids := map[string]string{}
	for _, name := range []string{"fails", "denied", "stuck", "slow-child", "unruled", "logs", "slow"} {
		id, err := l.Start(ctx, client.Start{At: "/trouble/" + name})
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = id
	}
	for _, name := range []string{"fails", "denied", "slow-child", "logs"} {
		finished(t, l.Client, ids[name])
	}
	waitFor(t, l.Client, func(r *client.Run) bool { return r.ID == ids["stuck"] && len(r.Tickets) == 1 })
	waitFor(t, l.Client, func(r *client.Run) bool { return r.ID == ids["unruled"] && r.Step >= 1 })
	if _, err := l.Client.Send(ctx, machine.Address{Run: ids["unruled"]}, "poke", nil, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, l.Client, func(r *client.Run) bool { return r.ID == ids["unruled"] && r.Step >= 2 })
	// The slow stage's second attempt is going, and has logged.
	for deadline := time.Now().Add(20 * time.Second); ; {
		var logged bool
		l.Client.Record(ctx, ids["slow"], func(e client.Entry) { logged = logged || e.Job == "work@2" })
		if logged {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the slow stage's second attempt never logged")
		}
		time.Sleep(50 * time.Millisecond)
	}

	h := newHandler(Config{List: &listed{from: viewer}, Lister: viewer, Client: l.Client, Interval: time.Second, Shell: page.Shell{Offered: []string{"Runs"}, Parties: []string{l.Client.ID.Name}},
		Command: func(ctx context.Context, c page.Call) page.Answer { return page.Answer{Out: `{"taken": true}`} },
		Differs: differs})
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.Handle("/", h.mux())
	return &troubled{l: l, h: h, handler: mux, ids: ids}
}

// whyOf gives what a trouble run's page says of why it is where it is.
func (b *troubled) whyOf(t *testing.T, name string) *why {
	t.Helper()
	ctx := context.Background()
	var entries []client.Entry
	if _, err := b.l.Client.Record(ctx, b.ids[name], func(e client.Entry) { entries = append(entries, e) }); err != nil {
		t.Fatal(err)
	}
	v, err := b.h.runOf(ctx, b.ids[name], entries, &reading{})
	if err != nil {
		t.Fatal(err)
	}
	return v.Why
}

func Test_runs_trouble_in_the_browser(t *testing.T) {
	b := newTrouble(t)
	t.Run("why", func(t *testing.T) {
		for _, name := range []string{"fails", "denied", "stuck", "slow-child", "unruled", "slow"} {
			w := b.whyOf(t, name)
			t.Logf("%s: %+v", name, *w)
		}
	})
	for _, name := range []string{"fails", "denied", "stuck", "slow-child", "unruled", "logs", "slow"} {
		t.Run(name, func(t *testing.T) {
			pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/run/" + b.ids[name], Module: "runs/trouble_test.js", Fixtures: map[string]any{"which": name}})
		})
	}
}
