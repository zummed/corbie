package cli_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// run start and run ls read a label as name or name=value, each name once:
// a name alone lists any value, and name= only the empty value.
func TestLabelsAreGivenEachNameOnce(t *testing.T) {
	m, _ := working(t, "all")
	at := m.recorded(filepath.Join(files(t, map[string]string{"one.js": `return 1`}), "one.js"))
	for _, flags := range [][]string{{"--label", "team=a", "--label", "team=b"}, {"--label", ""}, {"--label", "=x"}, {"--label", "bad label"}} {
		if got, out, said := m.run("", append([]string{"run", "start", at}, flags...)...); got != 2 || out != "" || !strings.Contains(said, "label") {
			t.Errorf("run start %s ended %d: %s%s", strings.Join(flags, " "), got, out, said)
		}
	}
	ids := map[string]string{}
	for _, label := range []string{"team=ops", "team="} {
		run := lines(t, m.ok("", "run", "start", at, "--label", label, "--wait", "--json"))[0]
		ids[run["id"].(string)] = label
	}
	for flags, want := range map[string]string{"team": "team= team=ops", "team=": "team=", "team=ops": "team=ops"} {
		var rows []struct{ ID string }
		if err := json.Unmarshal([]byte(m.ok("", "run", "ls", "--all", "--json", "--label", flags)), &rows); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, row := range rows {
			got = append(got, ids[row.ID])
		}
		if slices.Sort(got); strings.Join(got, " ") != want {
			t.Errorf("run ls --label %s listed %v; want %s", flags, got, want)
		}
	}
	if got, _, said := m.run("", "run", "ls", "--label", "team=a", "--label", "team=b"); got != 2 || !strings.Contains(said, `the label "team" is given twice`) {
		t.Errorf("run ls with team twice ended %d: %s", got, said)
	}
}
