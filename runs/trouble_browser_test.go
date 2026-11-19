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

// The machines in testdata/trouble go wrong on purpose. Each run's page is
// tested in Chrome for what it says of what went wrong.
func Test_runs_trouble_in_the_browser(t *testing.T) {
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
	start := func(at string) string {
		t.Helper()
		id, err := l.Start(ctx, client.Start{At: at})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ids := map[string]string{}
	for _, name := range []string{"fails", "denied", "stuck", "slow-child", "unruled", "logs"} {
		ids[name] = start("/trouble/" + name)
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

	h := New(Config{List: &listed{from: viewer}, Lister: viewer, Client: l.Client, Interval: time.Second, Shell: page.Shell{Offered: []string{"Runs"}, Parties: []string{l.Client.ID.Name}},
		Command: func(ctx context.Context, c page.Call) page.Answer { return page.Answer{Out: `{"taken": true}`} },
		// The file of fails differs from the definition its run started with.
		Differs: func(ctx context.Context, location, dir string, def *machine.Definition) ([]string, error) {
			if location != "/trouble/fails" || def.Name != "fails" {
				return nil, nil
			}
			return []string{"- throw new Error(\"no such person: ada\")", "+ return { found: true }"}, nil
		}})
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.Handle("/", h)
	for _, name := range []string{"fails", "denied", "stuck", "slow-child", "unruled", "logs"} {
		t.Run(name, func(t *testing.T) {
			pagetest.Run(t, pagetest.Config{Handler: mux, Page: "/run/" + ids[name], Module: "runs/trouble_test.js", Fixtures: map[string]any{"which": name}})
		})
	}
}
