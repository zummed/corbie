package controller_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/store"
)

// layout is every line of the store's drawing, as a pattern.
var layout = []*regexp.Regexp{
	regexp.MustCompile(`^version$`),
	regexp.MustCompile(`^ctl/inbox/[^/]+/[^/]+$`),
	regexp.MustCompile(`^ctl/receipts/[^/]+/[^/]+$`),
	regexp.MustCompile(`^ctl/(running|finished)/[^/]+/[^/]+$`),
	regexp.MustCompile(`^ctl/records/[^/]+/[^/]+/steps/\d{8}$`),
	regexp.MustCompile(`^ctl/records/[^/]+/[^/]+/emits/[^/]+/\d{8}$`),
	regexp.MustCompile(`^ctl/records/[^/]+/[^/]+/blobs/[^/]+$`),
	regexp.MustCompile(`^ctl/wakes/\d{8}T\d{6}\.\d{3}Z/[^/]+/[^/]+/[^/]+$`),
	regexp.MustCompile(`^ctl/trust/[^/]+$`),
	regexp.MustCompile(`^ctl/secrets/[^/]+$`),
}

// A whole run, with children, events, a ticket, a timer and receipts, keeps
// to what the store's overview promises of every run.
func TestWhatEveryRunKeepsTo(t *testing.T) {
	w := newWorld(t)
	const marker = "MARKER-IN-A-VALUE"
	door := w.submit(door, nil, map[string]any{"greeting": marker})
	fam := w.submit(family, nil, map[string]any{"note": marker})
	w.settle()
	w.client.Send(ctx, door, "open", map[string]any{"said": marker}, 0)
	w.settle()
	var ticket string
	for _, k := range w.latest(door).State.Tickets {
		ticket = k.Value
	}
	w.client.Reply(ctx, door, ticket, 42, 0)
	for range 6 {
		w.settle()
		w.tick(500 * time.Millisecond)
	}
	for _, run := range []string{door, fam} {
		if head := w.head(run); head == nil || head.Status != "done" {
			t.Fatalf("the run %s ended as %+v", run, head)
		}
	}
	w.mu.Lock()
	ops := w.ops
	w.mu.Unlock()
	heads := map[string]bool{}
	for _, op := range ops {
		if !op.Done || op.Act == store.ActRead || op.Act == store.ActList {
			continue
		}
		// The drawing is the whole layout: no name is written that it does
		// not draw.
		drawn := false
		for _, line := range layout {
			drawn = drawn || line.MatchString(op.Name)
		}
		if !drawn {
			t.Errorf("a name the layout does not draw was written: %s", op.Name)
		}
		// Only the head of a running instance, a trust entry and a secret
		// are ever rewritten.
		rewritable := strings.HasPrefix(op.Name, "ctl/running/") || strings.HasPrefix(op.Name, "ctl/trust/") || strings.HasPrefix(op.Name, "ctl/secrets/")
		if op.Act == store.ActRewrite && !rewritable {
			t.Errorf("%s was rewritten (%s)", op.Name, op.How)
		}
		// Everything in a record is written once.
		if strings.HasPrefix(op.Name, "ctl/records/") && op.Act == store.ActRewrite {
			t.Errorf("a record's entry was rewritten: %s", op.Name)
		}
		isHead := strings.HasPrefix(op.Name, "ctl/running/") || strings.HasPrefix(op.Name, "ctl/finished/")
		if isHead && op.Act != store.ActDelete {
			heads[op.Name] = true
			// The head holds nothing the instance was given or produced, and
			// no ticket's value.
			if strings.Contains(string(op.Value), marker) || strings.Contains(string(op.Value), ticket) {
				t.Errorf("a version of the head %s holds a value of the run's", op.Name)
			}
		}
		// What is sealed cannot be read by whoever runs the store.
		sealed := strings.HasPrefix(op.Name, "ctl/inbox/") || strings.HasPrefix(op.Name, "ctl/receipts/") || strings.HasPrefix(op.Name, "ctl/records/")
		if sealed && op.Value != nil && (strings.Contains(string(op.Value), marker) || strings.Contains(string(op.Value), ticket)) {
			t.Errorf("%s shows a value of the run's to whoever reads the store", op.Name)
		}
	}
	if len(heads) < 4 {
		t.Fatalf("only %d heads were written", len(heads))
	}
	// An instance is never without a head: its finished head was written
	// before its running one was removed.
	finished := map[string]bool{}
	for _, op := range ops {
		if !op.Done {
			continue
		}
		if rest, ok := strings.CutPrefix(op.Name, "ctl/finished/"); ok && op.Act != store.ActDelete && op.Act != store.ActRead {
			finished[rest] = true
		}
		if rest, ok := strings.CutPrefix(op.Name, "ctl/running/"); ok && op.Act == store.ActDelete && !finished[rest] {
			t.Errorf("the running head of %s was removed before its finished head was written", rest)
		}
	}
}
