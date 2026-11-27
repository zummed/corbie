package machines

import (
	"net/http"
	"testing"

	"github.com/zummed/corbie/ui/drawing"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"
)

// served gives the UI as serve mounts it for the Machines page: the page,
// the assets and the drawing's files.
func served(h *Pages) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("GET /drawing/", http.StripPrefix("/drawing", drawing.Files()))
	mux.Handle("/", h)
	return mux
}

// Test_machines_in_the_browser runs machines_test.js on the Machines page
// of a project recorded at /bot, open at its agent. POST /submitted
// records the project again with its agent's prompt changed, as corbie run
// submit does.
func Test_machines_in_the_browser(t *testing.T) {
	dir := project(t)
	b := newLab(t)
	b.submit(t, dir, "/bot")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /submitted", func(w http.ResponseWriter, r *http.Request) {
		write(t, dir, map[string]string{"support.yaml": "agent: support\ndescription: Answers people.\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello again\n"})
		b.submit(t, dir, "/bot")
	})
	mux.Handle("/", served(b.pages))
	pagetest.Run(t, pagetest.Config{Handler: mux, Page: "/machines?at=/bot/support", Module: "machines/machines_test.js"})
}
