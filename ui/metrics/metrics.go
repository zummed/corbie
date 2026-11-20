// Package metrics gives the status of runs in the text format Prometheus
// reads, from the runs whose status the UI reads.
package metrics

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
)

// A List is every run whose status the UI may read, as last read.
type List interface {
	Runs() ([]*client.Run, error)
}

// A series is one line of the answer: a metric's name, its labels in
// order, and its value.
type series struct {
	name   string
	labels [][2]string
	value  float64
}

var help = []struct{ name, help string }{
	{"corbie_runs", "Runs, running or ended and still kept, by status, failure, type, machine and location."},
	{"corbie_waiting", "What running runs wait on: jobs on runners, timers and tickets, by type, machine and location."},
	{"corbie_oldest_running_seconds", "How long the oldest running run of each machine at each location has run."},
	{"corbie_last_ended_timestamp_seconds", "When a run of each machine at each location last ended, by status."},
}

// Handler answers a scrape from the list. When the store could not be
// read at the last look, the scrape fails, so that Prometheus records it.
func Handler(list List) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runs, err := list.Runs()
		if err != nil {
			http.Error(w, "the store cannot be read: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, Text(runs, time.Now()))
	})
}

// Text is the answer to a scrape at a time.
func Text(runs []*client.Run, at time.Time) string {
	counts := map[string]*series{}
	add := func(name string, labels [][2]string, v float64, keep func(old, v float64) float64) {
		key := name
		for _, l := range labels {
			key += "\x00" + l[0] + "=" + l[1]
		}
		if s, ok := counts[key]; ok {
			s.value = keep(s.value, v)
			return
		}
		counts[key] = &series{name: name, labels: labels, value: v}
	}
	sum := func(old, v float64) float64 { return old + v }
	most := func(old, v float64) float64 { return max(old, v) }
	for _, r := range runs {
		kind := [][2]string{{"type", r.Type}, {"machine", r.Machine}, {"location", r.Location}}
		status := [][2]string{{"status", r.Status}}
		if r.Failure != "" {
			status = append(status, [2]string{"failure", r.Failure})
		}
		add("corbie_runs", append(status, kind...), 1, sum)
		switch r.Status {
		case instance.Running:
			add("corbie_waiting", append([][2]string{{"on", "jobs"}}, kind...), float64(r.Jobs), sum)
			add("corbie_waiting", append([][2]string{{"on", "timers"}}, kind...), float64(r.Timers), sum)
			add("corbie_waiting", append([][2]string{{"on", "tickets"}}, kind...), float64(len(r.Tickets)), sum)
			add("corbie_oldest_running_seconds", kind, at.Sub(r.Started).Seconds(), most)
		default:
			add("corbie_last_ended_timestamp_seconds", append([][2]string{{"status", r.Status}}, kind...), float64(r.Ended.Unix()), most)
		}
	}
	all := make([]*series, 0, len(counts))
	for _, s := range counts {
		all = append(all, s)
	}
	slices.SortFunc(all, func(a, b *series) int { return cmp.Compare(a.line(), b.line()) })
	var b strings.Builder
	for _, h := range help {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", h.name, h.help, h.name)
		for _, s := range all {
			if s.name == h.name {
				fmt.Fprintf(&b, "%s %s\n", s.line(), number(s.value))
			}
		}
	}
	return b.String()
}

func (s *series) line() string {
	var parts []string
	for _, l := range s.labels {
		parts = append(parts, l[0]+"="+quote(l[1]))
	}
	return s.name + "{" + strings.Join(parts, ",") + "}"
}

// quote writes a label's value as the text format takes it: in double
// quotes, with a backslash, a double quote and a new line escaped.
func quote(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v) + `"`
}

func number(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.3f", v)
}
