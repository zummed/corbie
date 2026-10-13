package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"regexp"
	"strings"
	"testing"
)

// Every flag a command takes is named in its help. A command makes its
// flags before it reads its arguments, so asking it for help makes them all
// and does nothing else. The flags every command takes are named once, in
// the overview.
func TestHelpNamesEveryFlagACommandTakes(t *testing.T) {
	for i := range Commands {
		cmd := &Commands[i]
		e := cmd.env(context.Background(), strings.NewReader(""), io.Discard, io.Discard)
		everywhere := map[string]bool{}
		e.flags.VisitAll(func(f *flag.Flag) { everywhere[f.Name] = true })
		if err := cmd.run(e, []string{"--help"}); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("corbie %s --help did not ask for help: %v", cmd.Name, err)
			continue
		}
		help := helpOf(cmd)
		e.flags.VisitAll(func(f *flag.Flag) {
			// The name as a whole flag: --input is not named by --input-file.
			named := regexp.MustCompile(`--` + regexp.QuoteMeta(f.Name) + `([^a-z-]|$)`)
			if !everywhere[f.Name] && !named.MatchString(help) {
				t.Errorf("corbie %s takes --%s, and its help does not name it", cmd.Name, f.Name)
			}
		})
		for name := range everywhere {
			if !strings.Contains(overview(), "--"+name) {
				t.Errorf("every command takes --%s, and the overview does not name it", name)
			}
		}
	}
}
