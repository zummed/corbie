package page_test

import (
	"net/http"
	"testing"

	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"
)

func Test_page_in_the_browser(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page.Write(w, page.Shell{Title: "Test", Stream: "/none"}, struct{}{})
	})
	pagetest.Run(t, pagetest.Config{Handler: mux, Page: "/", Module: "page/ui_test.js"})
}

// The builds of Solid the binary serves draw a page as the development
// builds do.
func Test_page_in_the_browser_with_the_shipped_solid(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page.Write(w, page.Shell{Title: "Test", Stream: "/none"}, struct{}{})
	})
	pagetest.Run(t, pagetest.Config{Handler: mux, Page: "/", Module: "page/ui_test.js", Shipped: true})
}
