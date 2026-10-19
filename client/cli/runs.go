package cli

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
)

// list is a flag that may be given several times.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

// starting is everything a start may be given on the command line.
type starting struct {
	input, life, timeout, memory     string
	labels, allow, deny, tags, files list
	secrets, secretsFromEnv          list
	wait, watch                      bool
	// opened are the files given as input, open until the start is sent.
	opened []*os.File
}

// close closes the files given as input.
func (s *starting) close() {
	for _, file := range s.opened {
		file.Close()
	}
}

func (s *starting) flags(e *Env) {
	e.Flags.StringVar(&s.input, "input", "", "")
	e.Flags.Var(&s.files, "input-file", "")
	e.Flags.Var(&s.labels, "label", "")
	e.Flags.Var(&s.allow, "allow", "")
	e.Flags.Var(&s.deny, "deny", "")
	e.Flags.Var(&s.secrets, "secret", "")
	e.Flags.Var(&s.secretsFromEnv, "secret-env", "")
	e.Flags.StringVar(&s.life, "life", "", "")
	e.Flags.StringVar(&s.timeout, "timeout", "", "")
	e.Flags.StringVar(&s.memory, "memory", "", "")
	e.Flags.Var(&s.tags, "tag", "")
	e.Flags.BoolVar(&s.watch, "watch", false, "")
}

// addSecret puts a run secret into a start, its value read by read.
func addSecret(start *client.Start, name string, read func(string) (string, error)) error {
	value, err := read(name)
	if err != nil {
		return err
	}
	if start.Secrets == nil {
		start.Secrets = map[string]string{}
	}
	start.Secrets[name] = value
	return nil
}

// prepare reads the definition or the script a command names, with the
// files beside it, fills it out and checks it, and gathers what the start
// carries. Nothing is uploaded and nothing is sent if anything is wrong.
func (s *starting) prepare(e *Env, file string) (*machine.Draft, client.Start, error) {
	var start client.Start
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, start, err
	}
	var draft *machine.Draft
	script := strings.HasSuffix(file, ".js")
	if script {
		// What a script file may do and its limits are given on the
		// command line.
		draft, err = client.Script(filepath.Base(file), src, client.ScriptJob{Allow: s.allow, Deny: s.deny,
			Timeout: s.timeout, Memory: s.memory, Tags: s.tags})
	} else {
		// A flag that has no effect on what was given is a mistake.
		for name, given := range map[string]bool{"--memory": s.memory != "", "--tag": len(s.tags) > 0} {
			if given {
				return nil, start, Mistake("%s applies only to a script file; a definition gives its jobs their own", name)
			}
		}
		// The start's own list, which narrows the run and everything below.
		if len(s.allow)+len(s.deny) > 0 {
			if err := permission.CheckList(s.allow); err != nil {
				return nil, start, Mistake("--allow: %v", err)
			}
			if err := permission.CheckList(s.deny); err != nil {
				return nil, start, Mistake("--deny: %v", err)
			}
			start.Allow = &permission.Layer{Allow: s.allow, Deny: s.deny}
		}
		dir := filepath.Dir(file)
		// Only the files beside the definition are read: a path that leaves
		// their directory is refused.
		load := func(name string) ([]byte, error) {
			if clean := path.Clean(name); clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
				return nil, fmt.Errorf("it is outside the directory the definition is in")
			}
			return os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		}
		draft, err = machine.Parse(src, load)
	}
	if err != nil {
		return nil, start, err
	}
	if s.input != "" {
		text := []byte(s.input)
		if name, isFile := strings.CutPrefix(s.input, "@"); isFile {
			if text, err = os.ReadFile(name); err != nil {
				return nil, start, err
			}
		}
		if err := json.Unmarshal(text, &start.Input); err != nil {
			return nil, start, Mistake("--input is JSON, or @file naming a file of JSON: %v", err)
		}
	}
	// A file is given as input under a name: --input-file report=report.pdf.
	files := map[string]client.InputFile{}
	for _, given := range s.files {
		name, at, ok := strings.Cut(given, "=")
		if !ok || name == "" || at == "" {
			return nil, start, Mistake("--input-file takes name=path: the name the file has in the input, and where the file is")
		}
		if _, twice := files[name]; twice {
			return nil, start, Mistake("--input-file gives two files under the name %q", name)
		}
		file, err := os.Open(at)
		if err != nil {
			return nil, start, err
		}
		s.opened = append(s.opened, file)
		files[name] = client.InputFile{Name: filepath.Base(at), Contents: file}
	}
	if start.Input, err = client.WithFiles(start.Input, files); err != nil {
		return nil, start, err
	}
	for _, label := range s.labels {
		name, value, _ := strings.Cut(label, "=")
		if start.Labels == nil {
			start.Labels = map[string]string{}
		}
		start.Labels[name] = value
	}
	for flagName, text := range map[string]string{"--timeout": s.timeout, "--life": s.life} {
		if text == "" {
			continue
		}
		d, err := machine.ParseDuration(text)
		if err != nil {
			return nil, start, Mistake("%s: %v", flagName, err)
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
	// The secrets come last, and the variables before anything is typed, so
	// that every other mistake is found before a person types a value.
	given := map[string]bool{}
	for _, name := range slices.Concat(s.secretsFromEnv, s.secrets) {
		if err := checkSecretName(name); err != nil {
			return nil, start, err
		}
		if given[name] {
			return nil, start, Mistake("the secret %q is given twice: give it once", name)
		}
		given[name] = true
	}
	for _, name := range s.secretsFromEnv {
		if err := addSecret(&start, name, secretFromEnv); err != nil {
			return nil, start, err
		}
	}
	typed := func(name string) (string, error) {
		return secretTyped(e, name, "--secret-env reads it from the environment variable")
	}
	for _, name := range s.secrets {
		if err := addSecret(&start, name, typed); err != nil {
			return nil, start, err
		}
	}
	return draft, start, nil
}

// interval is how often a command that waits looks again at a store that
// cannot tell it when something was written.
const interval = time.Second

func cmdTest(e *Env, args []string) error {
	var s starting
	s.flags(e)
	files, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(files) != 1 {
		return Mistake("name one definition, or one script file")
	}
	defer s.close()
	draft, start, err := s.prepare(e, files[0])
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
	id, err := local.Start(e.ctx, draft, start)
	if err != nil {
		return err
	}
	return e.await(local.Client, id, s.watch, strings.HasSuffix(files[0], ".js"))
}

func cmdSubmit(e *Env, args []string) error {
	var s starting
	s.flags(e)
	e.Flags.BoolVar(&s.wait, "wait", false, "")
	files, err := e.Parse(args)
	if err != nil {
		return err
	}
	if len(files) != 1 {
		return Mistake("name one definition, or one script file")
	}
	if s.wait && s.watch {
		return Mistake("--wait and --watch are two ways of staying; give one")
	}
	defer s.close()
	// The client comes first, so that nobody types a secret's value for a
	// start that cannot be sent.
	c, err := e.Client()
	if err != nil {
		return err
	}
	draft, start, err := s.prepare(e, files[0])
	if err != nil {
		return err
	}
	id, err := c.Start(e.ctx, draft, start)
	if err != nil {
		return err
	}
	if !s.wait && !s.watch {
		// The ID and nothing else: other commands take it.
		return e.Print(map[string]string{"id": id}, func() string { return id + "\n" })
	}
	e.Say("%s", id)
	return e.await(c, id, s.watch, strings.HasSuffix(files[0], ".js"))
}

// await stays until a run ends and prints it, following its record on the
// way if asked to. A command that waits for a run fails on a run that
// ended badly.
func (e *Env) await(c *client.Client, id string, watch, script bool) error {
	var run *client.Run
	var err error
	if watch {
		run, err = c.Follow(e.ctx, id, interval, func(entry client.Entry) { e.entry(entry) })
	} else {
		run, err = c.Wait(e.ctx, id, interval)
	}
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
	if waits := waitingOn(run); waits != "" && !run.HasEnded() {
		fmt.Fprintf(&b, "  waiting on %s\n", waits)
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
	}
	if run.Input != nil {
		in, _ := json.Marshal(run.Input, json.Deterministic(true))
		fmt.Fprintf(&b, "  input: %s\n", in)
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
	at := entry.Time.Local().Format("15:04:05")
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
	for _, event := range entry.Events {
		line := event.Name
		if event.From != "" {
			line += " from " + event.From
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
		switch ask.Kind {
		case instance.AskScript:
			line = fmt.Sprintf("job %s: a script, on %s", ask.Name, ask.Runner)
		case instance.AskChild:
			line = fmt.Sprintf("job %s: a machine, %s", ask.Name, ask.Child)
		case instance.AskCancel:
			line = "cancel " + ask.Name
		case instance.AskEvent:
			to := "its parent"
			if ask.Name != "" {
				to = ask.Name
			}
			line = fmt.Sprintf("send %s to %s", ask.Event.Name, to)
			if ask.Undelivered {
				line += " (undelivered: it is not running)"
			}
		case instance.AskFail:
			line = fmt.Sprintf("%s fails: %s", ask.Name, ask.Error.Error)
			if ask.Error.Refused != "" {
				line = fmt.Sprintf("%s fails, denied: %s", ask.Name, ask.Error.Refused)
			}
		case instance.AskHold:
			line = fmt.Sprintf("hold %s for the ask", ask.Name)
		case instance.AskNote:
			line = ask.Text
		default:
			continue
		}
		fmt.Fprintf(e.Out, "%s %s  step %d  -> %s\n", at, short, entry.Step, line)
	}
	if entry.Status != instance.Running {
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
	for _, label := range labels {
		name, value, _ := strings.Cut(label, "=")
		if filter.Labels == nil {
			filter.Labels = map[string]string{}
		}
		filter.Labels[name] = value
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
		if run.Parent != "" {
			line += "  under " + run.Parent
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
		return Mistake("name one run by its ID")
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
		// record, which only the client that started the run can read.
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
// message is good for the life given with --life, and the command waits no
// longer than that for its receipt.
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
	receipt, err := c.WaitReceipt(e.ctx, message, life, interval)
	if err != nil {
		return err
	}
	if err := e.Print(receipt, func() string {
		switch {
		case !receipt.Taken:
			return "refused: " + receipt.Refused + "\n"
		case receipt.Acted != nil && !*receipt.Acted:
			return "taken; the machine had no rule for it\n"
		case receipt.Acted != nil:
			return "taken; the machine acted on it\n"
		}
		return "taken\n"
	}); err != nil {
		return err
	}
	if !receipt.Taken {
		return errRunFailed
	}
	return nil
}

func cmdCancel(e *Env, args []string) error {
	return e.answered(args, "name one run by its ID", func(n int) bool { return n == 1 },
		func(c *client.Client, rest []string, life time.Duration) (string, error) {
			return c.Cancel(e.ctx, rest[0], life)
		})
}

func cmdSend(e *Env, args []string) error {
	return e.answered(args, "name a run, an event, and optionally its data", func(n int) bool { return n == 2 || n == 3 },
		func(c *client.Client, rest []string, life time.Duration) (string, error) {
			return c.Send(e.ctx, rest[0], rest[1], value(rest, 2), life)
		})
}

func cmdReply(e *Env, args []string) error {
	return e.answered(args, "name a run, the ticket, and optionally the answer", func(n int) bool { return n == 2 || n == 3 },
		func(c *client.Client, rest []string, life time.Duration) (string, error) {
			return c.Reply(e.ctx, rest[0], rest[1], value(rest, 2), life)
		})
}
