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
	t.Setenv("PATH", t.TempDir())
	overview := overview()
	for i := range Commands {
		cmd := &Commands[i]
		e := cmd.env(context.Background(), strings.NewReader(""), io.Discard, io.Discard)
		everywhere := map[string]bool{}
		e.Flags.VisitAll(func(f *flag.Flag) { everywhere[f.Name] = true })
		if err := cmd.Run(e, []string{"--help"}); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("corbie %s --help did not ask for help: %v", cmd.Name, err)
			continue
		}
		help := own().helpOf(cmd)
		e.Flags.VisitAll(func(f *flag.Flag) {
			// The name as a whole flag: --input is not named by --input-file.
			named := regexp.MustCompile(`--` + regexp.QuoteMeta(f.Name) + `([^a-z-]|$)`)
			if !everywhere[f.Name] && !named.MatchString(help) {
				t.Errorf("corbie %s takes --%s, and its help does not name it", cmd.Name, f.Name)
			}
		})
		for name := range everywhere {
			if !strings.Contains(overview, "--"+name) {
				t.Errorf("every command takes --%s, and the overview does not name it", name)
			}
		}
	}
}

// corbie help and the list of each group of commands fit a terminal of
// the usual width, with a command's usage left to its own help.
func TestHelpFitsATerminal(t *testing.T) {
	t.Setenv("PATH", "")
	texts := map[string]string{"corbie help": overview()}
	for _, c := range Commands {
		words := strings.Fields(c.Name)
		for i := 1; i < len(words); i++ {
			group := strings.Join(words[:i], " ")
			texts["corbie "+group] = own().groupHelp(group)
		}
	}
	for name, text := range texts {
		for _, line := range strings.Split(text, "\n") {
			if len(line) > columns {
				t.Errorf("%s has a line %d wide: %q", name, len(line), line)
			}
		}
		if strings.Contains(text, "[--") {
			t.Errorf("%s gives a command's usage", name)
		}
	}
}
