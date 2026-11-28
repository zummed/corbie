package client_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
)

// A label is name or name=value, each name once. A listing narrowed to a
// name keeps the runs with that label of any value, and one narrowed to
// name=value those with that value, the empty value too. A start whose
// label has a name that cannot be written is refused.
func TestLabelsAreReadOneWay(t *testing.T) {
	labels, err := client.ParseLabels([]string{"team=ops", "nightly", "empty=", "k=v=w"})
	want := []client.Label{{Name: "team", Value: "ops", Valued: true}, {Name: "nightly"}, {Name: "empty", Valued: true}, {Name: "k", Value: "v=w", Valued: true}}
	if err != nil || len(labels) != len(want) {
		t.Fatalf("the labels read as %+v: %v", labels, err)
	}
	for i, l := range labels {
		if l != want[i] || l.String() != []string{"team=ops", "nightly", "empty=", "k=v=w"}[i] {
			t.Errorf("label %d read as %+v, written %q", i, l, l.String())
		}
	}
	for _, texts := range [][]string{{""}, {"=x"}, {"bad label"}, {"tab\tbed"}, {strings.Repeat("n", 257)}} {
		if _, err := client.ParseLabels(texts); err == nil || !strings.Contains(err.Error(), "1 to 256 characters") {
			t.Errorf("the label %q read with %v", texts[0], err)
		}
	}
	if _, err := client.ParseLabels([]string{strings.Repeat("ü", 256)}); err != nil {
		t.Errorf("a name of 256 characters: %v", err)
	}
	if _, err := client.ParseLabels([]string{"team=a", "team=b"}); err == nil || !strings.Contains(err.Error(), `"team" is given twice`) {
		t.Errorf("a name given twice read with %v", err)
	}

	l := local(t)
	d := draft(t, `
		flow: one
		stages:
		  one: { job: { script: ./one.js } }
	`, map[string]string{"one.js": `return 1`})
	ids := map[string]string{}
	for name, labels := range map[string]map[string]string{"ops": {"team": "ops"}, "empty": {"team": ""}, "other": {"other": "x"}} {
		id, err := l.Run(ctx, d, client.Start{Labels: labels})
		if err != nil {
			t.Fatal(err)
		}
		wait(t, l, id)
		ids[id] = name
	}
	for text, want := range map[string]string{"team": "empty ops", "team=": "empty", "team=ops": "ops", "team=web": "", "other team": ""} {
		labels, err := client.ParseLabels(strings.Fields(text))
		if err != nil {
			t.Fatal(err)
		}
		runs, err := l.List(ctx, client.Filter{Labels: labels, Finished: true})
		var got []string
		for _, r := range runs {
			got = append(got, ids[r.ID])
		}
		if slices.Sort(got); err != nil || strings.Join(got, " ") != want {
			t.Errorf("a listing narrowed to %s gave %v (%v); want %s", text, got, err, want)
		}
	}
	if _, err := l.Run(ctx, d, client.Start{Labels: map[string]string{"bad label": "x"}}); !errors.Is(err, client.ErrStart) || !strings.Contains(err.Error(), `"bad label"`) {
		t.Errorf("a start with the label name \"bad label\" gave %v", err)
	}
}
