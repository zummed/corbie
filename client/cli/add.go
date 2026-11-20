package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zummed/corbie/pack"
)

// words are the --set flags of corbie add, each written WORD=name.
type words map[string]string

func (w words) String() string { return "" }

func (w words) Set(text string) error {
	word, name, ok := strings.Cut(text, "=")
	if !ok || word == "" || name == "" {
		return fmt.Errorf("write WORD=<name>")
	}
	w[word] = name
	return nil
}

func cmdAdd(e *Env, args []string) error {
	set := words{}
	var project string
	e.Flags.Var(set, "set", "")
	e.Flags.StringVar(&project, "in", ".", "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	type adding struct {
		src   pack.Source
		words map[string]string
	}
	var all []adding
	for _, text := range rest {
		src, err := pack.Parse(text)
		if err != nil {
			return Mistake("%v", err)
		}
		all = append(all, adding{src, set})
	}
	if len(rest) == 0 {
		if len(set) > 0 {
			return Mistake("--set is written with a pack")
		}
		sources, kept, err := pack.Added(project)
		if err != nil {
			return err
		}
		if len(sources) == 0 {
			return Mistake("name a pack: %s holds none", project)
		}
		names := make([]string, 0, len(sources))
		for name := range sources {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			all = append(all, adding{sources[name], kept[name]})
		}
	}
	var rep pack.Report
	for _, a := range all {
		one, err := pack.Add(project, a.src, a.words)
		if err != nil {
			return err
		}
		rep.Packs = append(rep.Packs, one.Packs...)
		rep.Started = append(rep.Started, one.Started...)
		rep.Differ = append(rep.Differ, one.Differ...)
	}
	return e.Print(rep, func() string {
		var b strings.Builder
		wide := 0
		for _, p := range rep.Packs {
			wide = max(wide, len(p.Name))
		}
		for _, p := range rep.Packs {
			from := p.Source
			if p.Needed != "" {
				from = "needed by " + p.Needed
			}
			if p.Commit != "" {
				from += "  " + p.Commit[:min(len(p.Commit), 10)]
			}
			fmt.Fprintf(&b, "%s/%-*s  %s\n", pack.Dir, wide, p.Name, from)
		}
		if len(rep.Started) > 0 {
			fmt.Fprintf(&b, "\nWrote these starting files:\n\n  %s\n", strings.Join(rep.Started, "\n  "))
		}
		if len(rep.Differ) > 0 {
			fmt.Fprintf(&b, "\nThese of your files are not as the pack starts them, and are left as they are:\n\n  %s\n", strings.Join(rep.Differ, "\n  "))
		}
		b.WriteString("\nThen: corbie test .\n")
		return b.String()
	})
}
