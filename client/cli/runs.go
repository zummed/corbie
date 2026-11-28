package cli

import (
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// list is a flag that may be given several times.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

// starting is everything a start may be given on the command line, and
// the secrets corbie test sets.
type starting struct {
	input, life, timeout, memory          string
	labels, allow, deny, tags, inputFiles list
	files, secrets, secretsFromEnv        list
	wait, watch                           bool
	// opened are the files given as input, open until the start is sent.
	opened []*os.File
}

// close closes the files given as input.
func (s *starting) close() {
	for _, file := range s.opened {
		file.Close()
	}
}

// flags sets every flag of a start and of a script file's job, which corbie
// test takes.
func (s *starting) flags(e *Env) {
	s.startFlags(e)
	s.jobFlags(e)
}

// startFlags sets the flags of a start.
func (s *starting) startFlags(e *Env) {
	e.Flags.StringVar(&s.input, "input", "", "")
	e.Flags.Var(&s.inputFiles, "input-file", "")
	e.Flags.Var(&s.labels, "label", "")
	e.Flags.Var(&s.allow, "allow", "")
	e.Flags.Var(&s.deny, "deny", "")
	e.Flags.StringVar(&s.life, "life", "", "")
	e.Flags.StringVar(&s.timeout, "timeout", "", "")
	e.Flags.BoolVar(&s.watch, "watch", false, "")
}

// jobFlags sets the flags of a script file's job that a start does not
// take. --allow, --deny and --timeout are a start's too.
func (s *starting) jobFlags(e *Env) {
	e.Flags.StringVar(&s.memory, "memory", "", "")
	e.Flags.Var(&s.tags, "tag", "")
	e.Flags.Var(&s.files, "files", "")
}

// startOnly lists the flags that are for starting a run, and scriptOnly
// those that set a script file's job.
var (
	startOnly  = []string{"input", "input-file", "label", "allow", "deny", "life", "timeout", "memory", "tag", "files", "watch"}
	scriptOnly = []string{"allow", "deny", "timeout", "memory", "tag", "files"}
)

// given lists the flags given of those named, as they are written.
func given(e *Env, names []string) []string {
	var out []string
	e.Flags.Visit(func(f *flag.Flag) {
		if slices.Contains(names, f.Name) {
			out = append(out, "--"+f.Name)
		}
	})
	return out
}

// isScript says whether a file is a script file, which is wrapped in a flow
// of one stage.
func isScript(file string) bool { return strings.HasSuffix(file, ".js") }

// job gives what a script file's job may do and its limits, which the
// command line gives. A flag that has no effect on what was given is a
// mistake.
func (s *starting) job(file string) (client.ScriptJob, error) {
	if !isScript(file) {
		for name, given := range map[string]bool{"--memory": s.memory != "", "--tag": len(s.tags) > 0, "--files": len(s.files) > 0} {
			if given {
				return client.ScriptJob{}, Mistake("%s applies only to a script file; a definition gives its jobs their own", name)
			}
		}
	}
	return client.ScriptJob{Allow: s.allow, Deny: s.deny, Timeout: s.timeout, Memory: s.memory, Tags: s.tags, Files: s.files}, nil
}

// record reads a file as a tree of one file at the prefix at: by default /
// and the file's name without its extension. takesAt says whether the
// command takes --at. A file with a problem is an error: nothing is
// uploaded and nothing is sent, so its location keeps what was recorded
// before.
func (s *starting) record(file, at string, takesAt bool) (*client.Tree, error) {
	if at == "" {
		at = "/" + strings.TrimSuffix(strings.TrimSuffix(filepath.Base(file), ".js"), ".yaml")
		if err := store.CheckLocation(at); err != nil || at == "/" {
			fix := "give the file another name"
			if takesAt {
				fix += ", or give --at"
			}
			return nil, Mistake("the file's name gives the location %s, which cannot be one: %s", at, fix)
		}
	}
	job, err := s.job(file)
	if err != nil {
		return nil, err
	}
	context, rel, err := contextOf(file)
	if err != nil {
		return nil, err
	}
	tree, err := client.ReadFile(context, rel, at, job)
	if err != nil {
		return nil, Mistake("%v", err)
	}
	if problem := tree.Machines[0].Problem; problem != nil {
		return nil, problem
	}
	return tree, nil
}

// start gathers what a start carries. For a script file --allow, --deny and
// --timeout are its job's, and not the start's.
func (s *starting) start(e *Env, script bool) (client.Start, error) {
	var start client.Start
	// The start's own list, which narrows the run and everything below.
	if !script && len(s.allow)+len(s.deny) > 0 {
		if err := permission.CheckList(s.allow); err != nil {
			return start, Mistake("--allow: %v", err)
		}
		if err := permission.CheckList(s.deny); err != nil {
			return start, Mistake("--deny: %v", err)
		}
		start.Allow = &permission.Layer{Allow: s.allow, Deny: s.deny}
	}
	if s.input != "" {
		text := []byte(s.input)
		if name, isFile := strings.CutPrefix(s.input, "@"); isFile {
			var err error
			if text, err = os.ReadFile(name); err != nil {
				return start, err
			}
		}
		if err := json.Unmarshal(text, &start.Input); err != nil {
			return start, Mistake("--input is JSON, or @file naming a file of JSON: %v", err)
		}
	}
	// A file is given as input at a place: --input-file files[0].file=a.pdf.
	files := map[string]client.InputFile{}
	for _, given := range s.inputFiles {
		place, at, ok := strings.Cut(given, "=")
		if !ok || place == "" || at == "" {
			return start, Mistake("--input-file takes place=path: the place the file has in the input, such as report or files[0].file, and where the file is")
		}
		if _, twice := files[place]; twice {
			return start, Mistake("--input-file gives two files at %q", place)
		}
		file, err := os.Open(at)
		if err != nil {
			return start, err
		}
		s.opened = append(s.opened, file)
		files[place] = client.InputFile{Name: filepath.Base(at), Contents: file}
	}
	var err error
	if start.Input, err = client.WithFiles(start.Input, files); err != nil {
		return start, err
	}
	labels, err := client.ParseLabels(s.labels)
	if err != nil {
		return start, Mistake("--label: %v", err)
	}
	for _, l := range labels {
		if start.Labels == nil {
			start.Labels = map[string]string{}
		}
		start.Labels[l.Name] = l.Value
	}
	for flagName, text := range map[string]string{"--timeout": s.timeout, "--life": s.life} {
		if text == "" {
			continue
		}
		d, err := machine.ParseDuration(text)
		if err != nil {
			return start, Mistake("%s: %v", flagName, err)
		}
		if flagName == "--timeout" {
			// A script's timeout is its job's, given above.
			if !script {
				start.Timeout = time.Duration(d)
			}
		} else {
			start.Life = time.Duration(d)
		}
	}
	return start, nil
}

// testSecrets reads the secrets corbie test is given: the variables before
// anything is typed. It comes after every other flag is read, so that every
// other mistake is found before a person types a value.
func (s *starting) testSecrets(e *Env) (map[string]string, error) {
	given := map[string]string{}
	for _, name := range slices.Concat(s.secretsFromEnv, s.secrets) {
		if err := checkSecretName(name); err != nil {
			return nil, err
		}
		if _, twice := given[name]; twice {
			return nil, Mistake("the secret %q is given twice: give it once", name)
		}
		given[name] = ""
	}
	for _, name := range s.secretsFromEnv {
		value, err := secretFromEnv(name)
		if err != nil {
			return nil, err
		}
		given[name] = value
	}
	for _, name := range s.secrets {
		value, err := secretTyped(e, name, "--secret-env reads it from the environment variable")
		if err != nil {
			return nil, err
		}
		given[name] = value
	}
	return given, nil
}

// interval is how often a command that waits looks again at a store that
// cannot tell it when something was written.
const interval = time.Second

// valuesGiven reads the values corbie test and corbie up are given, each
// NAME=VALUE, or a name whose value is the environment variable of that
// name, which are set with the secrets at / of the controller.
func valuesGiven(values, fromEnv []string) (map[string]string, error) {
	set := map[string]string{}
	for _, name := range fromEnv {
		if err := permission.CheckSecretName(name); err != nil {
			return nil, Mistake("--value-env: %v", err)
		}
		if _, twice := set[name]; twice {
			return nil, Mistake("the value %q is given twice: give it once", name)
		}
		value, err := secretFromEnv(name)
		if err != nil {
			return nil, err
		}
		set[name] = value
	}
	for _, given := range values {
		name, value, ok := strings.Cut(given, "=")
		if !ok {
			return nil, Mistake("--value takes NAME=VALUE")
		}
		if err := permission.CheckSecretName(name); err != nil {
			return nil, Mistake("--value: %v", err)
		}
		if _, twice := set[name]; twice {
			return nil, Mistake("the value %q is given twice: give it once", name)
		}
		set[name] = value
	}
	return set, nil
}

func cmdTest(e *Env, args []string) error {
	var s starting
	var values, valuesFromEnv list
	var definition bool
	var in string
	s.flags(e)
	e.Flags.Var(&values, "value", "")
	e.Flags.Var(&valuesFromEnv, "value-env", "")
	e.Flags.Var(&s.secrets, "secret", "")
	e.Flags.Var(&s.secretsFromEnv, "secret-env", "")
	e.Flags.BoolVar(&definition, "definition", false, "")
	e.Flags.StringVar(&in, "in", "", "")
	files, err := e.Parse(args)
	if err != nil {
		return err
	}
	dirs, err := directories(files)
	if err != nil {
		return err
	}
	ofAStart := append(slices.Clone(startOnly), "value", "value-env", "secret", "secret-env")
	if dirs {
		if given := given(e, append(ofAStart, "in", "definition")); len(given) > 0 {
			return Mistake("%s applies to one definition; a directory takes no flag of a start", strings.Join(given, ", "))
		}
		return e.checkMachines(files)
	}
	if len(files) != 1 {
		return Mistake("name directories, or one definition, or one script file")
	}
	if definition {
		return e.printDefinition(files[0], in, given(e, ofAStart))
	}
	set, err := valuesGiven(values, valuesFromEnv)
	if err != nil {
		return err
	}
	defer s.close()
	var tree *client.Tree
	var m *client.Machine
	if in == "" {
		if tree, err = s.record(files[0], "", false); err == nil {
			m = tree.Machines[0]
		}
	} else {
		tree, m, err = recordIn(e, files[0], in)
	}
	if err != nil {
		return err
	}
	// A singleton is started by the submit, and a start carries nothing to
	// it. A keyed file's runs start only at a send.
	switch m.Draft.Definition.Lifetime {
	case machine.LifetimeKeyed:
		return fmt.Errorf("%s is keyed: a run starts there when an event is sent to a key, so corbie test starts none; submit it in a tree with the machine that sends to it", files[0])
	case machine.LifetimeSingleton:
		if given := given(e, []string{"input", "input-file", "label", "allow", "deny", "timeout"}); len(given) > 0 {
			return Mistake("%s does not apply to a singleton: the submit starts its run", strings.Join(given, ", "))
		}
	}
	start, err := s.start(e, isScript(files[0]))
	if err != nil {
		return err
	}
	secrets, err := s.testSecrets(e)
	if err != nil {
		return err
	}
	// Everything in this process, the store in memory, nothing set up: the
	// ceilings allow everything, and the definition's own permissions
	// still apply.
	local, err := client.NewLocal(e.ctx, permission.Layer{Allow: []string{"all"}}, func(role string, err error) {
		e.Say("%s: %v", role, err)
	})
	if err != nil {
		return err
	}
	defer local.Close()
	local.Controller.AnyRunner = true
	for name, value := range set {
		if err := local.SetValue(e.ctx, "/", name, value); err != nil {
			return err
		}
	}
	for name, value := range secrets {
		if err := local.SetSecret(e.ctx, "/", name, value); err != nil {
			return err
		}
	}
	id, err := local.RunTree(e.ctx, tree, m.Location, start)
	if err != nil {
		return err
	}
	moving, stop := local.WhileMoving(e.ctx)
	defer stop()
	run, err := e.wait(moving, local.Client, id, s.watch)
	if err != nil {
		return err
	}
	// A run that has not ended, and was not cut short by the person, waits
	// on what nothing can move, or the look at the store failed.
	still := false
	if cause := context.Cause(moving); cause != nil && !run.HasEnded() && e.ctx.Err() == nil {
		if !errors.Is(cause, client.ErrStill) {
			return cause
		}
		still = true
	}
	if err := e.printRun(run, isScript(files[0])); err != nil {
		return err
	}
	// Beside the run, the runs a person would not otherwise see: each one
	// that failed, each one the submit or a send started at a singleton or
	// keyed location, and, when nothing can move them, each one running.
	lifetimes := map[string]string{}
	for _, m := range tree.Machines {
		lifetimes[m.Location] = m.Draft.Definition.Lifetime
	}
	ctx := context.WithoutCancel(e.ctx)
	runs, err := local.List(ctx, client.Filter{Finished: true})
	if err != nil {
		return err
	}
	for _, other := range runs {
		lifetime := lifetimes[other.Location]
		started := other.Parent == "" && (lifetime == machine.LifetimeSingleton || lifetime == machine.LifetimeKeyed)
		stuck := still && other.Status == instance.Running
		if other.ID == id || !started && !stuck && other.Status != instance.Failed {
			continue
		}
		shown, err := local.Show(ctx, other.ID)
		if err != nil {
			return err
		}
		if err := e.printRun(shown, false); err != nil {
			return err
		}
	}
	switch {
	case still:
		return client.ErrStill
	case run.HasEnded() && run.Status != instance.Done:
		return errRunFailed
	}
	return nil
}

// recordIn reads a directory as a tree, as a submit with no --at reads it,
// and gives the tree and the machine of a file in it. A file of the tree
// with a problem is printed, and fails the read.
func recordIn(e *Env, file, dir string) (*client.Tree, *client.Machine, error) {
	if isScript(file) {
		return nil, nil, Mistake("--in reads a definition with its tree; a script file runs alone")
	}
	tree, err := client.ReadTree(dir, "")
	if err != nil {
		return nil, nil, Mistake("%v", err)
	}
	rel, inside, err := within(tree.Directory, file)
	if err != nil {
		return nil, nil, err
	}
	i := slices.IndexFunc(tree.Machines, func(m *client.Machine) bool { return m.Path == filepath.ToSlash(rel) })
	switch {
	case !inside:
		return nil, nil, Mistake("%s is not in %s", file, dir)
	case i < 0:
		return nil, nil, Mistake("%s is no machine of the tree in %s: .corbieignore lists it", file, dir)
	}
	if err := reportProblems(e, tree); err != nil {
		return nil, nil, err
	}
	return tree, tree.Machines[i], nil
}

// printDefinition prints a definition file written out, each value with
// its file and line, and runs nothing. Given a directory in, it reads the
// file in that directory's tree.
func (e *Env) printDefinition(file, in string, starts []string) error {
	if len(starts) > 0 {
		return Mistake("%s applies to a run; --definition runs nothing", strings.Join(starts, ", "))
	}
	if isScript(file) {
		return Mistake("--definition prints a definition written out, and %s is a script file", file)
	}
	var draft *machine.Draft
	if in != "" {
		_, m, err := recordIn(e, file, in)
		if err != nil {
			return err
		}
		draft = m.Draft
	} else {
		context, rel, err := contextOf(file)
		if err != nil {
			return err
		}
		if draft, err = client.ReadMachine(context, rel); err != nil {
			return err
		}
	}
	return e.Print(draft.Definition, func() string { return string(draft.WrittenOut()) })
}

func cmdSubmit(e *Env, args []string) error {
	var s starting
	var at string
	e.Flags.Var(&s.allow, "allow", "")
	e.Flags.Var(&s.deny, "deny", "")
	e.Flags.StringVar(&s.timeout, "timeout", "", "")
	s.jobFlags(e)
	e.Flags.StringVar(&at, "at", "", "")
	names, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return Mistake("name one directory, one definition or one script file: each directory or file is a tree of its own")
	}
	name := names[0]
	if e.Given("at") && at == "" {
		return Mistake("--at is empty: a prefix is / and the names of one or more directories, as /home")
	}
	// The flags are a script file's job's. A definition gives its jobs
	// their own.
	if given := given(e, scriptOnly); len(given) > 0 && !isScript(name) {
		return Mistake("%s applies only to a script file; a definition gives its jobs their own", strings.Join(given, ", "))
	}
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	if info.IsDir() {
		tree, err := client.ReadTree(name, at)
		if err != nil {
			return Mistake("%v", err)
		}
		return e.submit(tree)
	}
	tree, err := s.record(name, at, true)
	if err != nil {
		return err
	}
	return e.submit(tree)
}

// cmdRunStart starts a run of a location.
func cmdRunStart(e *Env, args []string) error {
	var s starting
	s.startFlags(e)
	e.Flags.BoolVar(&s.wait, "wait", false, "")
	names, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return Mistake("name one location")
	}
	if !strings.HasPrefix(names[0], "/") {
		return Mistake("%s is not a location: a location starts with /, as /title; corbie run submit records a file or a directory", names[0])
	}
	if s.wait && s.watch {
		return Mistake("--wait and --watch are two ways of staying; give one")
	}
	defer s.close()
	c, err := e.Client()
	if err != nil {
		return err
	}
	start, err := s.start(e, false)
	if err != nil {
		return err
	}
	start.At = names[0]
	id, err := c.Start(e.ctx, start)
	if err != nil {
		return err
	}
	return e.started(c, id, &s, start.Life)
}

// started waits for a start's receipt, for no longer than a controller may
// take a start sent with the life given. It prints the refusal, or the run's
// ID, or with --wait or --watch stays until the run ends.
func (e *Env) started(c *client.Client, id string, s *starting, life time.Duration) error {
	receipt, err := e.receipt(c, client.StartMessage(id), life)
	if err != nil {
		return err
	}
	if !receipt.Taken {
		if err := e.Print(receipt, func() string { return refusal(receipt) }); err != nil {
			return err
		}
		return errRunFailed
	}
	if !s.wait && !s.watch {
		// The ID and nothing else: other commands take it.
		return e.Print(map[string]string{"id": id}, func() string { return id + "\n" })
	}
	e.Say("%s", id)
	return e.await(c, id, s.watch, false)
}

// refusal is a refused start as text: the reason, and for a permission what
// was refused, the layer that refused it and the fix.
func refusal(receipt *instance.Receipt) string {
	if d := receipt.Denied; d != nil {
		return fmt.Sprintf("refused: %s\n  by: %s\n  fix: %s\n", d.Refused, d.Layer, d.Fix)
	}
	text := "refused: " + receipt.Refused + "\n"
	if len(receipt.Problems) > 0 {
		text += "  " + strings.ReplaceAll(receipt.Problems.Error(), "\n", "\n  ") + "\n"
	}
	return text
}

// directories tells whether the names are all directories, and refuses
// directories named beside a file.
func directories(names []string) (bool, error) {
	dirs := 0
	for _, name := range names {
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			dirs++
		}
	}
	if dirs > 0 && dirs < len(names) {
		return false, Mistake("name directories, or one definition, or one script file; not both")
	}
	return dirs > 0, nil
}

// quiet is how long a command waits for a controller before it says what it
// waits for: longer than a running controller takes.
const quiet = 2 * time.Second

// receipt waits for the receipt of a message sent with a life.
func (e *Env) receipt(c *client.Client, message string, life time.Duration) (*instance.Receipt, error) {
	var receipt *instance.Receipt
	err := e.waiting(life, func() (err error) {
		receipt, err = c.WaitReceipt(e.ctx, message, life, interval)
		return err
	})
	return receipt, err
}

// waiting runs a wait for a controller to take a message sent with a life.
// A wait that goes on says once what it waits for and for how long, so that
// a setup with no controller running does not look like a command that has
// hung.
func (e *Env) waiting(life time.Duration, wait func() error) error {
	got, said := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(said)
		timer := time.NewTimer(quiet)
		defer timer.Stop()
		select {
		case <-got:
		case <-timer.C:
			e.Say("waiting up to %s for a controller to take it", short(client.Taking(life)))
		}
	}()
	err := wait()
	close(got)
	<-said
	return err
}

// short writes a length of time without the units that are zero at its
// end: 15m, and not 15m0s.
func short(d time.Duration) string {
	text := d.String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}

// submit records a tree, and stays until each run it cancelled has ended and
// each run it started has been taken up.
func (e *Env) submit(tree *client.Tree) error {
	c, err := e.Client()
	if err != nil {
		return err
	}
	message, err := c.Submit(e.ctx, tree, 0)
	if err != nil {
		return err
	}
	receipt, err := e.receipt(c, message, 0)
	if err != nil {
		return err
	}
	if !receipt.Taken {
		e.Print(receipt, func() string { return "refused: " + receipt.Refused + "\n" })
		return errRunFailed
	}
	done := receipt.Tree
	for _, id := range done.Cancelled {
		if _, err := c.Wait(e.ctx, id, interval); err != nil {
			return err
		}
	}
	for _, row := range done.Locations {
		if row.Run != "" {
			if err := c.Started(e.ctx, row.Run, interval); err != nil {
				return err
			}
		}
	}
	// The problems go with the JSON, keyed by location, as the text gives
	// them under each file.
	printed := struct {
		*instance.Submitted
		Problems map[string]string `json:"problems,omitzero"`
	}{Submitted: done}
	for _, m := range tree.Machines {
		if m.Problem != nil {
			if printed.Problems == nil {
				printed.Problems = map[string]string{}
			}
			printed.Problems[m.Location] = m.Problem.Error()
		}
	}
	err = e.Print(printed, func() string {
		var b strings.Builder
		if done.Before != "" {
			fmt.Fprintf(&b, "%s: last submitted from %s\n", tree.Prefix, done.Before)
		}
		width := 0
		for _, row := range done.Locations {
			width = max(width, len(row.Location))
		}
		for _, row := range done.Locations {
			what := row.What
			if row.Run != "" {
				what += " " + row.Run
			}
			fmt.Fprintf(&b, "%-*s   %-9s   %s\n", width, row.Location, row.Lifetime, what)
		}
		for _, id := range done.Cancelled {
			fmt.Fprintf(&b, "cancelled: %s\n", id)
		}
		for _, m := range tree.Machines {
			if m.Problem != nil {
				fmt.Fprintf(&b, "%s\n", m.Path)
				for _, line := range strings.Split(m.Problem.Error(), "\n") {
					fmt.Fprintf(&b, "    %s\n", line)
				}
			}
		}
		for _, name := range done.Missing {
			fmt.Fprintf(&b, "not set at its location or above: %s\n", name)
		}
		for _, name := range done.Untrusted {
			fmt.Fprintf(&b, "not a client the controller trusts: %s\n", name)
		}
		return b.String()
	})
	if err == nil && len(printed.Problems) > 0 {
		return fmt.Errorf("%d of %d files have problems, and their locations keep what was recorded before", len(printed.Problems), len(tree.Machines))
	}
	return err
}

// cmdRemoveTree removes the tree at a prefix, and stays until each run the
// removal cancelled has ended.
func cmdRemoveTree(e *Env, args []string) error {
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 || store.CheckLocation(rest[0]) != nil || rest[0] == "/" {
		return Mistake("name the prefix of one tree: / and the names of one or more directories, as /home")
	}
	c, err := e.Client()
	if err != nil {
		return err
	}
	message, err := c.RemoveTree(e.ctx, rest[0], 0)
	if err != nil {
		return err
	}
	receipt, err := e.receipt(c, message, 0)
	if err != nil {
		return err
	}
	if !receipt.Taken {
		e.Print(receipt, func() string { return "refused: " + receipt.Refused + "\n" })
		return errRunFailed
	}
	done := receipt.Tree
	for _, id := range done.Cancelled {
		if _, err := c.Wait(e.ctx, id, interval); err != nil {
			return err
		}
	}
	return e.Print(done, func() string {
		var b strings.Builder
		width := 0
		for _, row := range done.Locations {
			width = max(width, len(row.Location))
		}
		for _, row := range done.Locations {
			fmt.Fprintf(&b, "%-*s   %-9s   %s\n", width, row.Location, row.Lifetime, row.What)
		}
		for _, id := range done.Cancelled {
			fmt.Fprintf(&b, "cancelled: %s\n", id)
		}
		for _, name := range done.Removed {
			fmt.Fprintf(&b, "removed: %s\n", name)
		}
		if len(done.Locations)+len(done.Removed) == 0 {
			fmt.Fprintf(&b, "nothing was recorded or set at %s or under it\n", rest[0])
		}
		return b.String()
	})
}

// contextOf gives the context of a file and its path from there: the current
// directory, or the file's own directory for a file outside it.
func contextOf(file string) (context, rel string, err error) {
	if context, err = os.Getwd(); err != nil {
		return "", "", err
	}
	rel, inside, err := within(context, file)
	if err != nil {
		return "", "", err
	}
	if inside {
		return context, rel, nil
	}
	full, err := filepath.Abs(file)
	if err != nil {
		return "", "", err
	}
	return filepath.Dir(full), filepath.Base(full), nil
}

// within gives a file's path from a directory, and says whether it is inside
// it. Their real paths are compared, since a directory can be reached by a link.
func within(dir, name string) (string, bool, error) {
	full, err := filepath.Abs(name)
	if err != nil {
		return "", false, err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false, err
	}
	// A directory is followed. A file's own name stays as it is: a link is the
	// file, which its reader refuses or takes.
	real, base := full, ""
	if info, err := os.Stat(full); err != nil || !info.IsDir() {
		real, base = filepath.Dir(full), filepath.Base(full)
	}
	if real, err = filepath.EvalSymlinks(real); err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(root, filepath.Join(real, base))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, nil
	}
	return filepath.ToSlash(rel), true, nil
}

// checkMachines prints each machine of each directory ok or its problems,
// among them the sends and calls the controller would refuse, and fails if
// any has one.
func (e *Env) checkMachines(dirs []string) error {
	var paths, problems, listed []string
	nothing := map[string][]string{}
	for _, dir := range dirs {
		tree, err := client.ReadTree(dir, "")
		if err != nil {
			return err
		}
		unsent, empty := tree.Unsent()
		for at, from := range empty {
			for _, f := range from {
				nothing[at] = append(nothing[at], filepath.ToSlash(filepath.Join(dir, f)))
			}
		}
		for _, m := range tree.Machines {
			var lines []string
			if m.Problem != nil {
				lines = append(lines, m.Problem.Error())
			}
			lines = append(lines, unsent[m]...)
			paths = append(paths, filepath.ToSlash(filepath.Join(dir, m.Path)))
			problems = append(problems, strings.Join(lines, "\n"))
		}
		for _, name := range tree.Listed {
			full := filepath.ToSlash(filepath.Join(dir, name))
			if strings.HasSuffix(name, "/") {
				full += "/"
			}
			listed = append(listed, full)
		}
	}
	bad := 0
	report := struct {
		Files   map[string]string   `json:"files"`
		Listed  []string            `json:"listed,omitzero"`
		Nothing map[string][]string `json:"nothing_at,omitzero"`
	}{map[string]string{}, listed, nothing}
	width := 0
	for _, path := range listed {
		width = max(width, len(path))
	}
	for i, path := range paths {
		width = max(width, len(path))
		report.Files[path] = "ok"
		if problems[i] != "" {
			bad++
			report.Files[path] = problems[i]
		}
	}
	err := e.Print(report, func() string {
		var b strings.Builder
		for i, path := range paths {
			if problems[i] == "" {
				fmt.Fprintf(&b, "%-*s  ok\n", width, path)
				continue
			}
			fmt.Fprintf(&b, "%s\n", path)
			for _, line := range strings.Split(problems[i], "\n") {
				fmt.Fprintf(&b, "    %s\n", line)
			}
		}
		for _, path := range listed {
			fmt.Fprintf(&b, "%-*s  listed\n", width, path)
		}
		for _, at := range slices.Sorted(maps.Keys(nothing)) {
			fmt.Fprintf(&b, "nothing in the tree is at %s, named by %s\n", at, strings.Join(nothing[at], ", "))
		}
		return b.String()
	})
	if err == nil && bad > 0 {
		return fmt.Errorf("%d of %d files have problems", bad, len(paths))
	}
	return err
}

// await stays until a run ends and prints it, following its record on the
// way if asked to. A command that waits for a run fails on a run that
// ended badly.
func (e *Env) await(c *client.Client, id string, watch, script bool) error {
	run, err := e.wait(e.ctx, c, id, watch)
	if err != nil {
		return err
	}
	if err := e.printRun(run, script); err != nil {
		return err
	}
	// A wait that was cut short has printed the run as it stands, and that
	// is no failure of the run's.
	if run.HasEnded() && run.Status != instance.Done {
		return errRunFailed
	}
	return nil
}

// wait stays until a run ends, or the context does, and gives the run as it
// stands, following its record on the way if asked to.
func (e *Env) wait(ctx context.Context, c *client.Client, id string, watch bool) (*client.Run, error) {
	if watch {
		return c.Follow(ctx, id, interval, func(entry client.Entry) { e.entry(entry) })
	}
	return c.Wait(ctx, id, interval)
}

// printRun prints a run in the one shape.
func (e *Env) printRun(run *client.Run, script bool) error {
	return e.Print(run, func() string { return runText(run, script) })
}

// runText is a run in the one shape, as text.
func runText(run *client.Run, script bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s", run.ID, run.Status)
	if run.Failure != "" {
		fmt.Fprintf(&b, " (%s)", run.Failure)
	}
	if run.Machine != "" {
		fmt.Fprintf(&b, "  %s %s", run.Type, run.Machine)
	}
	if run.Step > 0 {
		fmt.Fprintf(&b, "  step %d", run.Step)
	}
	b.WriteString("\n")
	if run.Title != "" {
		fmt.Fprintf(&b, "  title: %s\n", run.Title)
	}
	if len(run.State.States) > 0 {
		fmt.Fprintf(&b, "  states: %s\n", strings.Join(run.State.States, " "))
	}
	if len(run.State.Stages) > 0 {
		var stages []string
		for _, name := range slices.Sorted(maps.Keys(run.State.Stages)) {
			stages = append(stages, name+" "+run.State.Stages[name])
		}
		fmt.Fprintf(&b, "  stages: %s\n", strings.Join(stages, ", "))
	}
	if run.State.Turn > 0 {
		fmt.Fprintf(&b, "  turn: %d\n", run.State.Turn)
	}
	if waits := waitingOn(run); waits != "" && !run.HasEnded() {
		fmt.Fprintf(&b, "  waiting on %s\n", waits)
	}
	if run.Location != "" {
		fmt.Fprintf(&b, "  at: %s\n", run.Location)
	}
	if run.Key != "" {
		fmt.Fprintf(&b, "  key: %q\n", run.Key)
	}
	if run.Cause.Run != "" {
		fmt.Fprintf(&b, "  cause: %s step %d\n", run.Cause.Run, run.Cause.Step)
	}
	if run.Cause.Principal != "" {
		fmt.Fprintf(&b, "  principal: %q\n", run.Cause.Principal)
	}
	if len(run.Labels) > 0 {
		fmt.Fprintf(&b, "  labels: %s\n", labelsOf(run))
	}
	// A refusal is printed once, as its three parts: the error's text
	// says the same again.
	if run.Error != "" && run.Refused == "" {
		fmt.Fprintf(&b, "  error: %s\n", run.Error)
	}
	if run.Refused != "" {
		fmt.Fprintf(&b, "  refused: %s\n  by: %s\n  fix: %s\n", run.Refused, run.Layer, run.Fix)
		if script && strings.HasPrefix(run.Layer, wrapped) {
			// A submitted script has no definition to change: what
			// allows it, or what denies it, is on the command line. A
			// layer further out is not changed there, and its own fix
			// says how it is.
			if entry, denied := strings.CutPrefix(run.Fix, "take the deny entry "); denied {
				entry, _, _ = strings.Cut(entry, " out of ")
				fmt.Fprintf(&b, "  for a script file, take off the command: --deny %s\n", entry)
			} else {
				fmt.Fprintf(&b, "  for a script file, add to the command: --allow %s\n", run.Refused)
			}
		}
		// What the error says past the refusal, such as the stages that
		// read the failure and were skipped, is printed on its own line.
		if _, more, _ := strings.Cut(run.Error, "To allow it: "+run.Fix+"; "); more != "" {
			fmt.Fprintf(&b, "  %s\n", more)
		}
	}
	if run.Input != nil {
		in, _ := json.Marshal(run.Input, json.Deterministic(true))
		fmt.Fprintf(&b, "  input: %s\n", in)
	}
	if run.State.Outputs != nil {
		outputs, _ := json.Marshal(run.State.Outputs, json.Deterministic(true))
		fmt.Fprintf(&b, "  outputs: %s\n", outputs)
	}
	if run.Output != nil {
		out, _ := json.Marshal(run.Output, json.Deterministic(true))
		fmt.Fprintf(&b, "  output: %s\n", out)
	}
	if len(run.Used) > 0 {
		fmt.Fprintf(&b, "  used: %s\n", strings.Join(run.Used, " "))
	}
	return b.String()
}

// wrapped is how a refusal names the layer of a script file's own list,
// which is that of the one stage it is wrapped in.
const wrapped = `the job "` + client.ScriptStage + `" in the definition of `

// labelsOf prints a run's labels: each name with its value, or alone if it
// has none.
func labelsOf(run *client.Run) string {
	var labels []string
	for _, name := range sortedKeys(run.Labels) {
		labels = append(labels, strings.TrimSuffix(name+"="+run.Labels[name], "="))
	}
	return strings.Join(labels, " ")
}

func waitingOn(run *client.Run) string {
	var parts []string
	if run.Jobs > 0 {
		parts = append(parts, plural(run.Jobs, "job"))
	}
	if run.Timers > 0 {
		parts = append(parts, plural(run.Timers, "timer"))
	}
	if len(run.Tickets) > 0 {
		parts = append(parts, "ticket "+strings.Join(run.Tickets, ", "))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// entry prints one entry of a record as it is followed: the entries are
// what was asked for, and go where a result goes.
func (e *Env) entry(entry client.Entry) {
	if e.json {
		data, _ := json.Marshal(entry, json.Deterministic(true))
		fmt.Fprintln(e.Out, string(data))
		return
	}
	at := entry.Time.Format("15:04:05")
	short := entry.ID[strings.LastIndexByte(entry.ID, '-')+1:]
	if entry.Step == 0 {
		data, _ := json.Marshal(entry.Data, json.Deterministic(true))
		text := string(data)
		if s, ok := entry.Data.(string); ok {
			text = s
		}
		fmt.Fprintf(e.Out, "%s %s  %s %s: %s\n", at, short, entry.Job, entry.Name, text)
		return
	}
	notes := true // whether the entry holds nothing but notes
	for _, event := range entry.Events {
		if event.Own && event.Name == instance.KindNote {
			continue // printed once, as the note below
		}
		notes = false
		line := event.Name
		switch from := event.Sender; {
		case from != nil && from.Client != "":
			line += " from client:" + from.Client
		case from != nil && from.Key != nil:
			line += fmt.Sprintf(" from %s#%s %s", from.At, *from.Key, from.Run)
		case from != nil:
			line += fmt.Sprintf(" from %s %s", from.At, from.Run)
		case event.From != "":
			line += " from " + event.From
		}
		if principal := event.Cause.Principal; principal != "" {
			line += fmt.Sprintf(" for %q", principal)
		}
		if failed, _ := event.Data.(map[string]any); failed["refused"] != nil {
			// A refusal that reaches the run is printed in full with the
			// run, once: each job it came from says only what was refused.
			line += fmt.Sprintf(" denied: %v", failed["refused"])
		} else if event.Data != nil {
			data, _ := json.Marshal(event.Data, json.Deterministic(true))
			line += " " + clip(string(data))
		}
		fmt.Fprintf(e.Out, "%s %s  step %d  <- %s\n", at, short, entry.Step, line)
	}
	for _, ask := range entry.Asks {
		line := ""
		notes = notes && ask.Kind == instance.AskNote
		switch ask.Kind {
		case instance.AskScript:
			line = fmt.Sprintf("job %s: a script, on %s", ask.Name, ask.Runner)
		case instance.AskChild:
			line = fmt.Sprintf("job %s: a machine, %s", ask.Name, ask.Child)
		case instance.AskCall:
			line = fmt.Sprintf("job %s: a call to %s, %s", ask.Name, ask.Location, ask.Run)
		case instance.AskCancel:
			line = "cancel " + ask.Name
		case instance.AskEvent:
			to := "its parent"
			switch {
			case ask.Location != "" && ask.Key != "":
				to = ask.Location + "#" + ask.Key
			case ask.Location != "":
				to = ask.Location
			case ask.Client != "":
				to = "client:" + ask.Client
			case ask.Direct:
				to = ask.To
			case ask.Name != "":
				to = ask.Name
			}
			line = fmt.Sprintf("send %s to %s", ask.Event.Name, to)
			switch {
			case ask.Undelivered && ask.Text != "":
				line += " · undelivered: " + ask.Text
			case ask.Undelivered:
				line += " · undelivered: it is not running"
			case ask.Run != "":
				line += " · started " + ask.Run
			case ask.Location != "":
				line += " · " + ask.To
			}
		case instance.AskFail:
			line = fmt.Sprintf("%s fails: %s", ask.Name, ask.Error.Error)
			if ask.Error.Refused != "" {
				line = fmt.Sprintf("%s fails, denied: %s", ask.Name, ask.Error.Refused)
			}
		case instance.AskHold:
			line = fmt.Sprintf("hold %s for its hooks", ask.Name)
		case instance.AskNote:
			line = ask.Text
		default:
			continue
		}
		fmt.Fprintf(e.Out, "%s %s  step %d  -> %s\n", at, short, entry.Step, line)
	}
	// A note that comes after the run has ended does not end it again.
	if entry.Status != instance.Running && !notes {
		fmt.Fprintf(e.Out, "%s %s  step %d  %s\n", at, short, entry.Step, entry.Status)
	}
}

func clip(text string) string {
	if len(text) > 200 {
		return text[:200] + "…"
	}
	return text
}

func cmdLs(e *Env, args []string) error {
	var labels list
	var filter client.Filter
	var watch bool
	e.Flags.Var(&labels, "label", "")
	e.Flags.StringVar(&filter.Status, "status", "", "")
	e.Flags.StringVar(&filter.Parent, "parent", "", "")
	e.Flags.BoolVar(&filter.Finished, "all", false, "")
	e.Flags.BoolVar(&watch, "watch", false, "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return Mistake("run ls takes no run; corbie run show <id> shows one")
	}
	if filter.Labels, err = client.ParseLabels(labels); err != nil {
		return Mistake("--label: %v", err)
	}
	if filter.Status != "" && !slices.Contains(client.Statuses, filter.Status) {
		return Mistake("--status is %s", strings.Join(client.Statuses, ", "))
	}
	c, err := e.Client()
	if err != nil {
		return err
	}
	row := func(run *client.Run) string {
		line := fmt.Sprintf("%s  %-9s  %s %s  step %d", run.ID, run.Status, run.Type, run.Machine, run.Step)
		if waits := waitingOn(run); waits != "" {
			line += "  waiting on " + waits
		}
		if run.Location != "" {
			line += "  at " + run.Location
		}
		if run.Parent != "" {
			line += "  under " + run.Parent
		}
		if run.Key != "" {
			line += fmt.Sprintf("  key %q", run.Key)
		}
		if len(run.Labels) > 0 {
			line += "  labels: " + labelsOf(run)
		}
		return line + "\n"
	}
	if watch {
		return c.Watch(e.ctx, filter, interval, func(u client.Update) {
			e.Print(u, func() string {
				if u.Listed {
					return "-- everything that already existed is above --\n"
				}
				return row(u.Run)
			})
		})
	}
	runs, err := c.List(e.ctx, filter)
	if err != nil {
		return err
	}
	if runs == nil {
		runs = []*client.Run{}
	}
	return e.Print(runs, func() string {
		var b strings.Builder
		for _, run := range runs {
			b.WriteString(row(run))
		}
		return b.String()
	})
}

func cmdShow(e *Env, args []string) error {
	var record, watch, wait, definition, tickets bool
	var step int
	var save string
	e.Flags.BoolVar(&record, "record", false, "")
	e.Flags.BoolVar(&watch, "watch", false, "")
	e.Flags.BoolVar(&wait, "wait", false, "")
	e.Flags.BoolVar(&definition, "definition", false, "")
	e.Flags.BoolVar(&tickets, "tickets", false, "")
	e.Flags.IntVar(&step, "step", 0, "")
	e.Flags.StringVar(&save, "save", "", "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return Mistake("name one run by its ID, or a location")
	}
	// Each of these asks for something else to be printed: only one of
	// them would have its way.
	var asked []string
	for _, name := range []string{"record", "watch", "wait", "definition", "tickets", "step", "save"} {
		if e.Given(name) {
			asked = append(asked, "--"+name)
		}
	}
	if len(asked) > 1 {
		return Mistake("%s each ask for something else: give one", strings.Join(asked, " and "))
	}
	id := rest[0]
	c, err := e.Client()
	if err != nil {
		return err
	}
	if strings.HasPrefix(id, "/") {
		if id, err = e.runAt(c, id); err != nil {
			return err
		}
	}
	switch {
	case definition:
		def, err := c.Definition(e.ctx, id)
		if err != nil {
			return err
		}
		data, err := json.Marshal(def, json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(e.Out, string(data))
		return err
	case tickets:
		// A ticket's value is the whole right to answer. It is in the
		// record, which only the client of the run's tree can read.
		held, err := c.Tickets(e.ctx, id)
		if err != nil {
			return err
		}
		if held == nil {
			held = map[string]string{}
		}
		return e.Print(held, func() string {
			var b strings.Builder
			for _, name := range sortedKeys(held) {
				fmt.Fprintf(&b, "%s  %s\n", name, held[name])
			}
			return b.String()
		})
	case step > 0:
		entry, err := c.Step(e.ctx, id, step)
		if err != nil {
			return err
		}
		data, err := json.Marshal(entry, json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(e.Out, string(data))
		return err
	case save != "":
		return e.save(c, id, save)
	case record:
		// What --watch prints, without staying: the entries so far, then
		// the run as it stands.
		run, err := c.Record(e.ctx, id, func(entry client.Entry) { e.entry(entry) })
		if err != nil {
			return err
		}
		return e.printRun(run, false)
	case watch || wait:
		return e.await(c, id, watch, false)
	}
	run, err := c.Show(e.ctx, id)
	if err != nil {
		return err
	}
	return e.printRun(run, false)
}

// save writes the files of a run's result into a directory. Every name is
// checked before any file is written: names come from a run's output, and a
// run must not choose where on the disk a file lands.
func (e *Env) save(c *client.Client, id, dir string) error {
	files, err := c.Files(e.ctx, id)
	if err != nil {
		return err
	}
	names := map[string]string{}
	for _, p := range sortedKeys(files) {
		// A file is saved under the name it has in the value that holds it.
		name := files[p].Name
		if name == "." || name == ".." || name == "" || strings.ContainsAny(name, `/\`) {
			return fmt.Errorf("the result names a file %q, which cannot be saved under that name", name)
		}
		if other, taken := names[name]; taken {
			return fmt.Errorf("two files of the result, at %q and %q, would be saved under the one name %q", other, p, name)
		}
		names[name] = p
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	saved := []string{}
	for _, name := range sortedKeys(names) {
		contents, _, err := c.Fetch(e.ctx, id, names[name])
		if err != nil {
			return err
		}
		file, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		_, err = io.Copy(file, contents)
		if cerr := file.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		saved = append(saved, filepath.Join(dir, name))
	}
	// Where each file was saved.
	return e.Print(saved, func() string {
		if len(saved) == 0 {
			return ""
		}
		return strings.Join(saved, "\n") + "\n"
	})
}

// value reads an optional argument as JSON, or as plain text if it is not.
func value(args []string, at int) any {
	if len(args) <= at {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(args[at]), &v); err != nil {
		return args[at]
	}
	return v
}

// answered sends a message and prints what became of it: the receipt. The
// message is good for the life given with --life, and the command waits for
// its receipt no longer than a controller may take it.
func (e *Env) answered(args []string, usage string, takes func(n int) bool, send func(c *client.Client, rest []string, life time.Duration) (string, error)) error {
	var lifeText string
	e.Flags.StringVar(&lifeText, "life", "", "")
	rest, err := e.Parse(args)
	if err != nil {
		return err
	}
	if !takes(len(rest)) {
		return Mistake("%s", usage)
	}
	var life time.Duration
	if lifeText != "" {
		d, err := machine.ParseDuration(lifeText)
		if err != nil {
			return Mistake("--life: %v", err)
		}
		life = time.Duration(d)
	}
	c, err := e.Client()
	if err != nil {
		return err
	}
	message, err := send(c, rest, life)
	if err != nil {
		return err
	}
	receipt, err := e.receipt(c, message, life)
	if err != nil {
		return err
	}
	if err := e.Print(receipt, func() string {
		by := ""
		if receipt.Run != "" {
			by = " by " + receipt.Run
		}
		switch {
		case !receipt.Taken:
			return "refused: " + receipt.Refused + "\n"
		case receipt.Acted != nil && !*receipt.Acted:
			return "taken" + by + "; the machine had no rule for it\n"
		case receipt.Acted != nil:
			return "taken" + by + "; the machine acted on it\n"
		}
		return "taken" + by + "\n"
	}); err != nil {
		return err
	}
	if !receipt.Taken {
		return errRunFailed
	}
	return nil
}

func cmdCancel(e *Env, args []string) error {
	var lifeText string
	if len(args) > 0 && strings.HasPrefix(args[0], "/") {
		e.Flags.StringVar(&lifeText, "life", "", "")
		rest, err := e.Parse(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return Mistake("name one run by its ID, one location, or a location and a key as /location#key")
		}
		return e.cancelAt(rest[0], lifeText)
	}
	return e.answered(args, "name one run by its ID, one location, or a location and a key as /location#key", func(n int) bool { return n == 1 },
		func(c *client.Client, rest []string, life time.Duration) (string, error) {
			return c.Cancel(e.ctx, rest[0], life)
		})
}

// cancelAt cancels every run of this client's running at a location, or
// the one of a key given as /location#key: each root, whose cancel ends
// everything below it.
func (e *Env) cancelAt(location, lifeText string) error {
	var life time.Duration
	if lifeText != "" {
		d, err := machine.ParseDuration(lifeText)
		if err != nil {
			return Mistake("--life: %v", err)
		}
		life = time.Duration(d)
	}
	c, err := e.Client()
	if err != nil {
		return err
	}
	at, key, keyed := strings.Cut(location, "#")
	runs, err := c.List(e.ctx, client.Filter{Location: at, Key: key, Status: instance.Running})
	if err != nil {
		return err
	}
	var lines []string
	refused := false
	for _, run := range runs {
		if run.Parent != "" {
			continue
		}
		message, err := c.Cancel(e.ctx, run.ID, life)
		if err != nil {
			return err
		}
		receipt, err := e.receipt(c, message, life)
		if err != nil {
			return err
		}
		line := run.ID + "  taken"
		if !receipt.Taken {
			line, refused = run.ID+"  refused: "+receipt.Refused, true
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 && keyed {
		return fmt.Errorf("no run of this client's is running at %s", location)
	}
	if len(lines) == 0 {
		lines = []string{"no run of this client's is running at " + location}
	}
	if err := e.Print(lines, func() string { return strings.Join(lines, "\n") + "\n" }); err != nil {
		return err
	}
	if refused {
		return errRunFailed
	}
	return nil
}

// runAt gives the one run of this client's running at a location, or with
// the key given as /location#key.
func (e *Env) runAt(c *client.Client, text string) (string, error) {
	at, key, _ := strings.Cut(text, "#")
	runs, err := c.List(e.ctx, client.Filter{Location: at, Key: key, Status: instance.Running})
	if err != nil {
		return "", err
	}
	var ids []string
	for _, run := range runs {
		if run.Parent == "" {
			ids = append(ids, run.ID)
		}
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no run of this client's is running at %s", text)
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("%d runs are running at %s: name one by its ID (%s)", len(ids), text, strings.Join(ids, ", "))
}

func cmdSend(e *Env, args []string) error {
	return e.answered(args, "name a run or a location, then an event, and optionally its data", func(n int) bool { return n == 2 || n == 3 },
		func(c *client.Client, rest []string, life time.Duration) (string, error) {
			to, err := c.Address(e.ctx, rest[0])
			if errors.Is(err, client.ErrOthers) {
				return "", err
			}
			if err != nil {
				return "", Mistake("%v", err)
			}
			return c.Send(e.ctx, to, rest[1], value(rest, 2), life)
		})
}

func cmdReply(e *Env, args []string) error {
	return e.answered(args, "name a run, the ticket, and optionally the answer", func(n int) bool { return n == 2 || n == 3 },
		func(c *client.Client, rest []string, life time.Duration) (string, error) {
			return c.Reply(e.ctx, rest[0], rest[1], value(rest, 2), life)
		})
}
