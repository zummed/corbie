package runs

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"
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

// The definition recorded at the location of fails differs from the one its
// run started with, as the Machines page gives it: a value by its path, and
// a script.
func differs(ctx context.Context, location string, def blob.Ref) ([]string, error) {
	if location != "/trouble/fails" || def == (blob.Ref{}) {
		return nil, nil
	}
	return []string{"stages.find.job: ./find.js", "a script changed: ./check.js"}, nil
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

// viewOf gives a trouble run's page as it is drawn.
func (b *troubled) viewOf(t *testing.T, name string) runView {
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
	return v
}

func Test_runs_trouble_in_the_browser(t *testing.T) {
	b := newTrouble(t)
	t.Run("runs_why_follows_the_record", func(t *testing.T) {
		names := func(facts []fact) string {
			var out []string
			for _, f := range facts {
				out = append(out, f.Name)
			}
			return strings.Join(out, " ")
		}
		for _, c := range []struct {
			run, lead, rest, facts, row string
		}{
			{"fails", "failed (error)", "at stage check", "error", "stage check failed"},
			{"denied", "failed (denied)", "at stage read", "refused by fix", "stage read failed"},
			{"slow-child", "failed (error)", "at stage wait", "error timeout child", "stage wait failed"},
			{"stuck", "waiting", "on ticket verdict", "", ""},
			{"unruled", "waiting", "on an event in resting", "", ""},
			{"slow", "waiting", "on job work on runner", "", ""},
		} {
			v := b.viewOf(t, c.run)
			w := v.Why
			if w == nil || w.Lead != c.lead || w.Rest != c.rest || names(w.Facts) != c.facts {
				t.Errorf("%s: why is %+v", c.run, w)
				continue
			}
			if c.row != "" {
				if r := rowAt(v.Rows, w.Row); r == nil || r.Said() != c.row || v.Follows != w.Row {
					t.Errorf("%s: why is at %+v, and the page follows %s", c.run, r, v.Follows)
				}
			}
		}
		if w := b.viewOf(t, "slow").Why; w.Since.IsZero() {
			t.Errorf("a job's wait does not say since when: %+v", w)
		}
		if w := b.viewOf(t, "slow-child").Why; w.Facts[1].Text != "1s" || w.Facts[2].Text != "statechart stuck failed (timeout)" {
			t.Errorf("a stage whose child passed its timeout gives %+v", w.Facts)
		}
	})
	for _, name := range []string{"fails", "denied", "stuck", "slow-child", "unruled", "logs", "slow"} {
		t.Run(name, func(t *testing.T) {
			pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/run/" + b.ids[name], Module: "runs/trouble_test.js", Fixtures: map[string]any{"which": name}})
		})
	}
}
