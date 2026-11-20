package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
)

var at = time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

var runs = []*client.Run{
	{ID: "a/1", Machine: "review", Type: "statechart", Location: "/docs/review", Key: "spec", Status: "running", Started: at.Add(-time.Hour), Timers: 1, Tickets: []string{"verdict"}},
	{ID: "a/2", Machine: "review", Type: "statechart", Location: "/docs/review", Key: "plan", Status: "running", Started: at.Add(-10 * time.Minute), Jobs: 2},
	{ID: "a/3", Machine: "fetch-title", Type: "flow", Location: "/web/fetch-title", Status: "failed", Failure: "denied", Started: at.Add(-3 * time.Minute), Ended: at.Add(-2 * time.Minute)},
	{ID: "a/4", Machine: "fetch-title", Type: "flow", Location: "/web/fetch-title", Status: "done", Started: at.Add(-5 * time.Minute), Ended: at.Add(-4 * time.Minute)},
	{ID: "a/5", Machine: "fetch-title", Type: "flow", Location: "/web/fetch-title", Status: "done", Started: at.Add(-50 * time.Minute), Ended: at.Add(-49 * time.Minute)},
	{ID: "a/6", Machine: "fetch-title", Type: "flow", Location: "/work/fetch-title", Status: "done", Started: at.Add(-50 * time.Minute), Ended: at.Add(-30 * time.Minute)},
}

func Test_metrics_series_and_their_labels(t *testing.T) {
	text := Text(runs, at)
	for _, want := range []string{
		`corbie_runs{status="running",type="statechart",machine="review",location="/docs/review"} 2`,
		`corbie_runs{status="failed",failure="denied",type="flow",machine="fetch-title",location="/web/fetch-title"} 1`,
		`corbie_runs{status="done",type="flow",machine="fetch-title",location="/web/fetch-title"} 2`,
		`corbie_runs{status="done",type="flow",machine="fetch-title",location="/work/fetch-title"} 1`,
		`corbie_waiting{on="jobs",type="statechart",machine="review",location="/docs/review"} 2`,
		`corbie_waiting{on="timers",type="statechart",machine="review",location="/docs/review"} 1`,
		`corbie_waiting{on="tickets",type="statechart",machine="review",location="/docs/review"} 1`,
		`corbie_oldest_running_seconds{type="statechart",machine="review",location="/docs/review"} 3600`,
		`corbie_last_ended_timestamp_seconds{status="done",type="flow",machine="fetch-title",location="/web/fetch-title"} ` + strconv.FormatInt(at.Add(-4*time.Minute).Unix(), 10),
		`corbie_last_ended_timestamp_seconds{status="done",type="flow",machine="fetch-title",location="/work/fetch-title"} ` + strconv.FormatInt(at.Add(-30*time.Minute).Unix(), 10),
	} {
		if !strings.Contains(text, want+"\n") {
			t.Errorf("the answer lacks %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, "spec") || strings.Contains(text, "plan") {
		t.Errorf("a run's key is a label:\n%s", text)
	}
}

func Test_metrics_every_series_is_a_gauge(t *testing.T) {
	text := Text(runs, at)
	types := strings.Count(text, "# TYPE ")
	if types != 4 || strings.Count(text, " gauge\n") != types {
		t.Errorf("not every series is a gauge:\n%s", text)
	}
}

type failing struct{}

func (failing) Runs() ([]*client.Run, error) { return runs, errors.New("the bucket refused the key") }

func Test_metrics_scrape_fails_while_the_store_cannot_be_read(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(failing{}).ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "corbie_runs") {
		t.Errorf("a scrape while the store cannot be read answered %d:\n%s", w.Code, w.Body.String())
	}
}
