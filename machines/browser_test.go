package machines

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zummed/corbie-ui/drawing"
	"github.com/zummed/corbie-ui/page"
	"github.com/zummed/corbie-ui/page/pagetest"
)

// served gives the UI as serve mounts it for the pages of a directory: the
// pages, the assets and the drawing's files, and an address by which a
// test writes a file into the directory, as a person's editor would.
func served(t *testing.T, dir string) http.Handler {
	t.Helper()
	h, err := New(Config{Dir: dir, Interval: 100 * time.Millisecond, Shell: page.Shell{Offered: []string{"Machines", "Toolkits"}}})
	if err != nil {
		t.Fatal(err)
	}
	return servedBy(dir, h)
}

// servedBy gives the UI as serve mounts it for pages already made.
func servedBy(dir string, h *Pages) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.HandleFunc("POST /written", func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(r.FormValue("file"))), []byte(r.FormValue("text")), 0o644); err != nil {
			page.Error(w, http.StatusInternalServerError, err.Error())
		}
	})
	mux.Handle("/", h)
	return mux
}

// Test_machines_in_the_browser runs machines_test.js on the Machines page
// of a project, with a machine that has a problem, open at its agent.
func Test_machines_in_the_browser(t *testing.T) {
	dir := project(t)
	write(t, dir, map[string]string{"broken.yaml": "flow: broken\nstages: nope\n"})
	pagetest.Run(t, pagetest.Config{Handler: served(t, dir), Page: "/machines?file=support.yaml", Module: "machines/machines_test.js"})
}

// Test_machines_toolkits_in_the_browser runs toolkits_test.js on the
// Toolkits page, open at the toolkit the tests' project offers.
func Test_machines_toolkits_in_the_browser(t *testing.T) {
	dir := toolkitsProject(t)
	v := toolkitsView_(t, dir, "")
	open := byDescription(t, v, "messages")
	pagetest.Run(t, pagetest.Config{Handler: served(t, dir), Page: "/toolkits?toolkit=" + open.ID, Module: "machines/toolkits_test.js",
		Fixtures: map[string]any{"toolkits": v}})
}
