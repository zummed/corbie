// Package pagetest runs the tests of what a page draws in Chrome, from a
// Go test: the test modules beside each area's scripts, with Solid's
// development builds, so a fault Solid finds fails the test.
package pagetest

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// A Config is a test: the UI's pages, the page to open, the test module to
// run on it, and the fixtures the Go test made, by name.
type Config struct {
	// Handler serves the UI, as serve mounts it: its pages, /assets/ and
	// the areas' scripts. Solid's files under /assets/solid/ are given in
	// their development builds in place of the ones Handler serves.
	Handler http.Handler
	// Page is the address of the page to open, such as "/runs".
	Page string
	// Module is the path of the test module, under the repository, such
	// as "runs/runs_test.js".
	Module string
	// Fixtures are given to the tests as JSON at /fixture/<name>.
	Fixtures map[string]any
}

// Run opens a page of the UI in Chrome, runs a test module on it, and
// reports each of its tests as a subtest. A Solid diagnostic or an error on
// the page fails the test as a whole. With no Chrome, the test fails.
func Run(t *testing.T, c Config) {
	t.Helper()
	root := repoRoot(t)
	mux := http.NewServeMux()
	mux.Handle("GET /assets/solid/", http.StripPrefix("/assets/solid", http.FileServerFS(os.DirFS(filepath.Join(root, "page", "testdata", "solid-dev")))))
	mux.Handle("GET /test/", http.StripPrefix("/test", http.FileServerFS(os.DirFS(root))))
	mux.HandleFunc("GET /fixture/{name}", func(w http.ResponseWriter, r *http.Request) {
		v, ok := c.Fixtures[r.PathValue("name")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.MarshalWrite(w, v, json.Deterministic(true))
	})
	mux.Handle("/", c.Handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.NoSandbox, chromedp.WindowSize(1440, 900),
		chromedp.Flag("force-device-scale-factor", "1"), chromedp.Flag("font-render-hinting", "none"))
	if path := os.Getenv("CHROME"); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}
	actx, stop := chromedp.NewExecAllocator(ctx, opts...)
	defer stop()
	bctx, closeTab := chromedp.NewContext(actx)
	defer closeTab()

	var mu sync.Mutex
	var faults []string
	if err := chromedp.Do(bctx); err != nil {
		t.Fatalf("Chrome did not start: the browser tests need Chrome or Chromium, by its usual name or at $CHROME: %v", err)
	}
	go func() {
		for m, err := range chromedp.Console(bctx) {
			if err != nil {
				return
			}
			if m.Type == chromedp.ConsoleException || m.Type == chromedp.ConsoleError || isDiagnostic(m.Text) {
				mu.Lock()
				faults = append(faults, string(m.Type)+": "+m.Text)
				mu.Unlock()
			}
		}
	}()

	type result struct {
		Name  string  `json:"name"`
		OK    bool    `json:"ok"`
		MS    float64 `json:"ms"`
		Error string  `json:"error"`
	}
	run := fmt.Sprintf(`document.fonts.ready.then(async () => { const t = await import("/test/page/testdata/test.js"); await import(%q); return t.run(); })`, "/test/"+c.Module)
	if err := chromedp.Do(bctx, chromedp.Navigate(server.URL+c.Page)); err != nil {
		t.Fatalf("%s did not open: %v", c.Page, err)
	}
	results, err := chromedp.Run(bctx, chromedp.Evaluate[[]result](run, chromedp.EvalAwaitPromise))
	if err != nil {
		mu.Lock()
		t.Fatalf("the tests of %s did not run: %v\n%s", c.Module, err, strings.Join(faults, "\n"))
	}
	if len(results) == 0 {
		t.Fatalf("%s has no tests", c.Module)
	}
	for _, r := range results {
		t.Run(r.Name, func(t *testing.T) {
			if !r.OK {
				t.Error(r.Error)
			}
		})
	}
	time.Sleep(50 * time.Millisecond) // the console's last messages
	mu.Lock()
	defer mu.Unlock()
	for _, f := range faults {
		t.Errorf("%s on %s: %s", c.Module, c.Page, f)
	}
}

// isDiagnostic says whether a console line is one of Solid's diagnostics,
// which begin with their code in capitals: [STRICT_READ_UNTRACKED].
func isDiagnostic(line string) bool {
	code, _, ok := strings.Cut(strings.TrimPrefix(line, "["), "]")
	return strings.HasPrefix(line, "[") && ok && code != "" && strings.ToUpper(code) == code && !strings.ContainsAny(code, " ")
}

// repoRoot gives the top of the repository, which holds page/.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := goruntime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
