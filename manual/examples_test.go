package manual_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/manual"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// How a page writes an example, and what running it means.
//
// A paragraph a page sets in by four spaces is an example, and is run;
// lists of calls, of fields and of commands, which are not, are set in by
// two. An example is files and commands:
//
//	# name.yaml, // name.js    a line that names a file starts that file
//	flow: name                 a definition with no such line is name.yaml
//	allow: [net:host]          a first part that is an allow list is what
//	                           the script after it may do
//	anything else              a script
//	corbie …                   a command, run where the files are; the
//	                           lines set in under it are lines it prints
//
// An example with no command is run as corbie test runs its first file,
// with the allow list and each secret the list names. The files and the
// config of a page's earlier examples are there for its later ones. Each
// file a page of the group Start gives is there for every page after it,
// as it was first given, so a later page can run the first pages' machine
// without giving it again.
//
// An example with an allow list is then run again for each entry of the
// list, with that entry taken away, and each time must end denied.
//
// In a script, a line that ends "// gives <value>" states what the
// statement on it gives; on a line of its own it states it of the
// statement above. The statement is one line: an expression, a
// declaration, or a return.
//
// A line a command prints is shown as it is printed. A line of a record may
// be shown without the time and the run number it begins with, and <id> in
// a shown line stands for the ID of a run.
//
// A command must end well, unless the page shows it printing a line that
// starts "error:" or "refused:", and then it must not. In a command, <id>
// is the ID of the run the page's last corbie run submit started, and <ticket>
// the ticket that run waits on, once it does.

// An example is one block of a page, read.
type example struct {
	page     string
	line     int // the line of the page it starts on
	files    []*file
	allow    []string
	commands []*command
	fault    string // why it cannot be read as an example
}

type file struct {
	name string
	text string
}

type command struct {
	text  string
	shown []string // lines the page shows it printing
}

// where says which example this is, for a report.
func (ex example) where() string {
	return fmt.Sprintf("the page %q, the example at line %d", ex.page, ex.line)
}

var names = regexp.MustCompile(`^(?:#|//) ([\w./-]+\.(?:yaml|js))(?::.*)?$`)

// examplesOf reads the examples of a page.
func examplesOf(page manual.Page) []example {
	found, _ := split(page)
	return found
}

// split reads a page as its examples and the lines that are of none.
func split(page manual.Page) (found []example, rest []string) {
	var block []string
	start := 0
	flush := func() {
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}
		if len(block) == 0 {
			return
		}
		ex, err := read(block)
		ex.page, ex.line = page.Name, start
		if err != nil {
			ex.fault = err.Error()
		}
		block = nil
		found = append(found, ex)
	}
	// A line set in by four spaces is of an example, unless it only
	// carries on the line above it in a list: the second line of an entry
	// that is set in by less.
	list := false // the last line was of a list
	for i, line := range strings.Split(page.Text, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			list = false
			if len(block) > 0 {
				block = append(block, "")
			}
		case strings.HasPrefix(line, "    ") && !list:
			if len(block) == 0 {
				start = i + 1
			}
			block = append(block, line[4:])
		default:
			list = strings.HasPrefix(line, " ")
			rest = append(rest, line)
			flush()
		}
	}
	flush()
	return found, rest
}

// read splits an example's lines into its files and its commands.
func read(lines []string) (example, error) {
	var ex example
	var into *file        // the file being read
	var allow []string    // the lines of an allow list being read
	var printing *command // the command whose printed lines are being read
	endAllow := func() error {
		if allow == nil {
			return nil
		}
		var list struct{ Allow []string }
		if err := yaml.Unmarshal([]byte(strings.Join(allow, "\n")), &list); err != nil || len(list.Allow) == 0 {
			return fmt.Errorf("its allow list cannot be read: %v", err)
		}
		ex.allow, allow = list.Allow, nil
		return nil
	}
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		switch {
		case allow != nil && !blank:
			allow = append(allow, line)
		case allow != nil:
			if err := endAllow(); err != nil {
				return ex, err
			}
		case names.MatchString(line):
			into, printing = &file{name: names.FindStringSubmatch(line)[1], text: line + "\n"}, nil
			ex.files = append(ex.files, into)
		case strings.HasPrefix(line, "corbie "):
			into, printing = nil, &command{text: line}
			ex.commands = append(ex.commands, printing)
		case printing != nil && strings.HasPrefix(line, " ") && !blank:
			printing.shown = append(printing.shown, strings.TrimSpace(line))
		case blank:
			printing = nil
			if into != nil {
				into.text += "\n"
			}
		case into == nil && strings.HasPrefix(line, "allow:") && len(ex.files) == 0 && len(ex.commands) == 0:
			allow = []string{line}
		default:
			if into == nil {
				// A file with no line that names it: a definition is named
				// by its own first line, and anything else is a script.
				into = &file{name: "example.js"}
				for _, kind := range machine.Types() {
					if name, ok := strings.CutPrefix(line, kind+": "); ok {
						into.name = strings.TrimSpace(name) + ".yaml"
					}
				}
				ex.files = append(ex.files, into)
			}
			printing = nil
			into.text += line + "\n"
		}
	}
	if err := endAllow(); err != nil {
		return ex, err
	}
	for _, f := range ex.files {
		f.text = strings.TrimRight(f.text, "\n") + "\n"
	}
	if len(ex.files) == 0 && len(ex.commands) == 0 {
		return ex, fmt.Errorf("it holds nothing to run")
	}
	return ex, nil
}

const gives = "// gives "

var statement = regexp.MustCompile(`^(return\s+|(?:const|let|var)\s+[A-Za-z_$][\w$]*\s*=\s*)?(.+?);?$`)

// stated rewrites a script so that each value its page states is checked
// as the script runs: the statement's value is compared with the stated
// one, and a difference stops the script with an error that gives both.
// It says how many values the script states.
func stated(script string) (string, int, error) {
	lines := strings.Split(script, "\n")
	count := 0
	for i, line := range lines {
		at := strings.LastIndex(line, gives)
		if at < 0 {
			continue
		}
		want, of := strings.TrimSpace(line[at+len(gives):]), i
		if strings.TrimSpace(line[:at]) == "" {
			// On a line of its own: of the statement above.
			lines[i] = ""
			for of--; of >= 0 && strings.TrimSpace(lines[of]) == ""; of-- {
			}
			if of < 0 {
				return "", 0, fmt.Errorf("line %d states a value of no statement", i+1)
			}
		} else {
			lines[of] = line[:at]
		}
		code := strings.TrimRight(lines[of], " \t")
		inset := code[:len(code)-len(strings.TrimLeft(code, " \t"))]
		m := statement.FindStringSubmatch(strings.TrimSpace(code))
		if m == nil || want == "" {
			return "", 0, fmt.Errorf("line %d states a value of something that is not one statement on one line", i+1)
		}
		lines[of] = fmt.Sprintf("%s%s__gives(%d, (%s), (%s))", inset, m[1], of+1, m[2], want)
		count++
	}
	if count == 0 {
		return script, 0, nil
	}
	return strings.Join(lines, "\n") + `
function __gives(line, got, want) {
  const plain = (v) => JSON.stringify(v, (name, x) => x && typeof x === "object" && !Array.isArray(x) && !(x instanceof Uint8Array)
    ? Object.keys(x).sort().reduce((sorted, n) => { sorted[n] = x[n]; return sorted }, {}) : x)
  if (plain(got) !== plain(want)) throw new Error("` + mismatch + ` line " + line + " gives " + plain(want) + ", and it gives " + plain(got))
  return got
}
`, count, nil
}

// mismatch is how an error starts when a value is not the one stated.
const mismatch = "the page says"

// words reads a command as a shell would the little of a shell's writing
// the pages use: words, quotes, ~ for the home directory, a remark after #,
// and & at the end for a command that is left running.
func words(line, home string) (args []string, left bool, err error) {
	var word strings.Builder
	in, quote := false, rune(0)
	end := func() {
		if in {
			args = append(args, word.String())
			word.Reset()
			in = false
		}
	}
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			in, quote = true, r
		case r == ' ' || r == '\t':
			end()
		case r == '#' && !in:
			end()
			return finish(args, home)
		case strings.ContainsRune("|<>$;`\\", r):
			return nil, false, fmt.Errorf("the command %q needs a shell: an example's command is one command of corbie's", line)
		default:
			in = true
			word.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, false, fmt.Errorf("the command %q has a quote that is not closed", line)
	}
	end()
	return finish(args, home)
}

func finish(args []string, home string) ([]string, bool, error) {
	left := len(args) > 0 && args[len(args)-1] == "&"
	if left {
		args = args[:len(args)-1]
	}
	for i, arg := range args {
		if arg == "~" || strings.HasPrefix(arg, "~/") {
			args[i] = home + arg[1:]
		}
	}
	if len(args) < 2 || args[0] != "corbie" {
		return nil, false, fmt.Errorf("an example's command is one command of corbie's")
	}
	return args, left, nil
}

// An outcome is what became of one example.
type outcome struct {
	example example
	// failed says why it did not run as the page has it; differs says
	// where what it gave is not what the page states.
	failed  string
	differs string
	checked int // how many stated values and printed lines were compared
	// without says, for an example that states what it is allowed, what
	// was wrong with how it ended when an entry of that list was taken
	// away; taken is how many entries were.
	without []string
	taken   int
}

// A session is one page being gone through: the directory its files are in,
// its home directory, where its config is, and the commands it left running.
type session struct {
	w       *world
	dir     string
	home    string
	running []*left
	id      string // the run the last corbie run submit started
}

// A left command is one a page left running: what it has printed so far,
// and how it is stopped.
type left struct {
	command string
	printed *output
	stop    func()
}

// An output is what a command prints, which can be read while the command
// is still printing.
type output struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

func (w *world) session(n int) (*session, error) {
	s := &session{w: w, dir: filepath.Join(w.dir, "work", strconv.Itoa(n)), home: filepath.Join(w.dir, "home", strconv.Itoa(n))}
	for _, dir := range []string{s.dir, s.home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	os.Setenv("HOME", s.home)
	return s, os.Chdir(s.dir)
}

func (s *session) end() {
	for _, l := range s.running {
		l.stop()
	}
}

// limit is how long one command of an example may take.
const limit = 30 * time.Second

// do runs one command of corbie's to its end and gives the status it ended
// with and what it printed.
func do(args []string) (status int, printed string, err error) {
	var said output
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Not a deadline: a runner takes a context's deadline for its host's
	// limit.
	stopped := time.AfterFunc(limit, cancel)
	status = cli.Run(ctx, args, strings.NewReader(""), &said, &said)
	if !stopped.Stop() {
		return 0, "", fmt.Errorf("corbie %s was stopped after %s, and had printed:\n%s", strings.Join(args, " "), limit, inset(said.String()))
	}
	return status, said.String(), nil
}

// inset sets a text in under the line that speaks of it, and says so when
// there is none.
func inset(text string) string {
	if strings.TrimSpace(text) == "" {
		return "  (nothing)"
	}
	return "  " + strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", "\n  ")
}

// placed gives a text with the world's directory in place of /srv, and seen
// gives what was printed with /srv back in its place.
func (w *world) placed(text string) string { return strings.ReplaceAll(text, "/srv/", w.srv+"/") }
func (w *world) seen(text string) string   { return strings.ReplaceAll(text, w.srv+"/", "/srv/") }

// run runs one example and says what became of it.
func (s *session) run(ex example) outcome {
	out := outcome{example: ex, failed: ex.fault}
	if ex.fault != "" {
		return out
	}
	for _, f := range ex.files {
		text := s.w.placed(f.text)
		if strings.HasSuffix(f.name, ".js") {
			checked, count, err := stated(text)
			if err != nil {
				out.failed = fmt.Sprintf("%s: %v", f.name, err)
				return out
			}
			text, out.checked = checked, out.checked+count
		}
		path := filepath.Join(s.dir, filepath.FromSlash(f.name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			out.failed = err.Error()
			return out
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			out.failed = err.Error()
			return out
		}
	}
	commands := ex.commands
	if len(commands) == 0 {
		if len(ex.files) == 0 {
			out.failed = "it has an allow list and nothing to allow it to"
			return out
		}
		commands = []*command{{text: tested(ex.files[0].name, ex.allow)}}
	} else if len(ex.allow) > 0 {
		out.failed = "it has an allow list and commands: a command says what is allowed with --allow"
		return out
	}
	// What is said of an example that did not run tells where to look: how
	// long each of its commands took, and what the commands left running
	// have printed, which is where a controller or a runner says that a
	// pass of its failed.
	var took []string
	fail := func(why string) outcome {
		if len(took) > 0 {
			why += "\nBefore it, in this example:\n" + inset(strings.Join(took, "\n"))
		}
		for _, l := range s.running {
			why += fmt.Sprintf("\nLeft running, %s has printed:\n%s", l.command, inset(s.w.seen(l.printed.String())))
		}
		out.failed = why
		return out
	}
	for _, c := range commands {
		started := time.Now()
		text, err := s.named(c.text)
		if err != nil {
			return fail(err.Error())
		}
		args, leave, err := words(s.w.placed(text), s.home)
		if err != nil {
			return fail(err.Error())
		}
		if leave {
			// Left running until the page has been gone through.
			l := &left{command: c.text, printed: &output{}}
			ctx, cancel := context.WithCancel(context.Background())
			var done sync.WaitGroup
			done.Go(func() { cli.Run(ctx, args[1:], strings.NewReader(""), l.printed, l.printed) })
			l.stop = func() { cancel(); done.Wait() }
			s.running = append(s.running, l)
			took = append(took, c.text+"  (left running)")
			continue
		}
		status, printed, err := do(args[1:])
		if err != nil {
			return fail(err.Error())
		}
		said := s.w.seen(printed)
		if len(args) > 2 && args[1] == "run" && args[2] == "submit" {
			s.id = runID.FindString(said)
		}
		wantStatus := 0
		for _, line := range c.shown {
			if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "refused:") {
				wantStatus = 1
			}
		}
		if status != wantStatus {
			text := fmt.Sprintf("%s ended with the status %d after %s, and printed:\n%s", c.text, status, time.Since(started).Round(time.Millisecond), inset(said))
			if strings.Contains(said, mismatch) {
				out.differs = text
				return out
			}
			return fail(text)
		}
		took = append(took, fmt.Sprintf("%s  (%s)", c.text, time.Since(started).Round(time.Millisecond)))
		// The lines the page shows are lines it printed, in that order.
		rest := strings.Split(said, "\n")
		for _, want := range c.shown {
			at := -1
			for i, line := range rest {
				if printedAs(line, want) {
					at = i
					break
				}
			}
			if at < 0 {
				out.differs = fmt.Sprintf("%s did not print the line\n  %s\nIt printed:\n%s", c.text, want, inset(said))
				return out
			}
			rest = rest[at+1:]
			out.checked++
		}
	}
	if len(ex.allow) > 0 {
		out.without, out.taken = s.without(ex), len(ex.allow)
	}
	return out
}

// tested gives the command that runs a file as corbie test does, with what
// a list allows and each secret the list names.
func tested(file string, allow []string) string {
	line := "corbie test " + file
	for _, entry := range allow {
		line += " --allow '" + entry + "'"
		if name, isSecret := secretOf(entry); isSecret {
			line += " --secret-env " + name
		}
	}
	return line
}

// secretOf gives the name of the secret an entry is for.
func secretOf(entry string) (string, bool) {
	rest, isSecret := strings.CutPrefix(entry, permission.Secret+":")
	name, _, _ := strings.Cut(rest, "@")
	return name, isSecret
}

// without runs an example that has just run once more for each entry of
// the list it is allowed, with that entry taken away, and says of each
// what was wrong with how it ended. The runs of one example are made side
// by side, where the example ran.
//
// A secret's entry does two things: it names the secret, so that the job
// is sent it, and it says where the secret may go. What is taken away is
// where it may go: the secret stays named, bound to a place no example
// sends anything.
func (s *session) without(ex example) []string {
	faults := make([]string, len(ex.allow))
	var all sync.WaitGroup
	for i, entry := range ex.allow {
		rest := slices.Clone(ex.allow)
		if name, isSecret := secretOf(entry); isSecret {
			rest[i] = permission.Secret + ":" + name + "@net:nowhere.invalid"
		} else {
			rest = slices.Delete(rest, i, i+1)
		}
		args, _, err := words(s.w.placed(tested(ex.files[0].name, rest)), s.home)
		if err != nil {
			faults[i] = err.Error()
			continue
		}
		all.Go(func() {
			status, printed, err := do(args[1:])
			if err != nil {
				faults[i] = err.Error()
				return
			}
			faults[i] = s.w.missing(entry, status, printed)
		})
	}
	all.Wait()
	return slices.DeleteFunc(faults, func(fault string) bool { return fault == "" })
}

var refusedLine = regexp.MustCompile(`(?m)^\s*refused: (.+)$`)

// missing says what is wrong with how an example ended when an entry of
// its list was taken away, or nothing. It should end denied, refused
// something the entry allowed: then the page allows the example nothing it
// does not need, and what was taken away is what the refusal speaks of.
func (w *world) missing(entry string, status int, printed string) string {
	if status == 0 {
		return fmt.Sprintf("without %s it runs to its end: the page allows it something it does not need", entry)
	}
	m := refusedLine.FindStringSubmatch(printed)
	if m == nil || !strings.Contains(printed, "failed (denied)") {
		return fmt.Sprintf("without %s it does not end denied. It printed:\n%s", entry, inset(w.seen(printed)))
	}
	allowed := entry == m[1] || entry == "all"
	if kind, value, scoped := strings.Cut(m[1], ":"); scoped {
		// An entry that names a place in the scratch directory is read as
		// the refused job read it: its directory is the one the refused path
		// is in.
		scratch := ""
		if _, in, found := strings.Cut(value, w.tmp+"/"); found {
			name, _, _ := strings.Cut(in, "/")
			scratch = filepath.Join(w.tmp, name)
		}
		taken := &permission.Set{Layers: []permission.Layer{{Allow: []string{w.placed(entry)}}}, Scratch: scratch}
		allowed = taken.Peek(kind, value)
	}
	if !allowed {
		return fmt.Sprintf("without %s it is refused %s, which is not what that entry allowed", entry, w.seen(m[1]))
	}
	return ""
}

// recordStart is what a line of a record begins with: its time and the
// number of its run.
var recordStart = regexp.MustCompile(`^\d\d:\d\d:\d\d [0-9a-f]{10}  `)

// printedAs says whether a printed line is the line a page shows.
func printedAs(line, shown string) bool {
	line = strings.TrimSpace(line)
	if strings.Contains(shown, "<id>") {
		line = runID.ReplaceAllString(line, "<id>")
	}
	return line == shown || recordStart.ReplaceAllString(line, "") == shown
}

var runID = regexp.MustCompile(`[\w.-]+/\d{8}T\d{6}\.\d{3}Z-[0-9a-f]+`)

// named gives a command with the run and the ticket it speaks of written in:
// the run the page's last corbie run submit started, and the ticket that run
// waits on, which is asked for until the run has come to wait on it. A
// command that prints the tickets waits for the same.
func (s *session) named(command string) (string, error) {
	ticket := strings.Contains(command, "<ticket>") || strings.Contains(command, "--tickets")
	if !strings.Contains(command, "<id>") && !ticket {
		return command, nil
	}
	if s.id == "" {
		return "", fmt.Errorf("the command %q speaks of a run, and the page has started none with corbie run submit", command)
	}
	command = strings.ReplaceAll(command, "<id>", s.id)
	if !ticket {
		return command, nil
	}
	// The ticket is there once the run has come to wait on it, which takes
	// a controller and a runner a moment.
	var status int
	var tickets string
	for until := time.Now().Add(limit); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
		var err error
		if status, tickets, err = do([]string{"run", "show", s.id, "--tickets"}); err != nil {
			return "", err
		}
		if held := strings.Fields(tickets); status == 0 && len(held) == 2 {
			return strings.ReplaceAll(command, "<ticket>", held[1]), nil
		}
	}
	_, stands, _ := do([]string{"run", "show", s.id})
	return "", fmt.Errorf("%s speaks of a ticket, and in %s the run did not come to wait on one ticket: corbie run show %s --tickets ended with the status %d, and printed:\n%s\nand corbie run show %s printed:\n%s",
		command, limit, s.id, status, inset(tickets), s.id, inset(s.w.seen(stands)))
}

// calls asks a script what it finds on each plugin and library: the calls
// it can make.
func (w *world) calls() (map[string][]string, error) {
	names, err := json.Marshal(plugin.Names())
	if err != nil {
		return nil, err
	}
	script := filepath.Join(w.dir, "work", "calls.js")
	if err := os.WriteFile(script, fmt.Appendf(nil, "const found = {}\nfor (const name of %s) found[name] = Object.keys(require(name)).sort()\nreturn found\n", names), 0o644); err != nil {
		return nil, err
	}
	status, printed, err := do([]string{"test", script, "--json"})
	if err != nil {
		return nil, err
	}
	var run struct {
		Output map[string][]string `json:"output"`
	}
	if err := json.Unmarshal([]byte(printed), &run); err != nil || status != 0 {
		return nil, fmt.Errorf("a script that asks what calls it can make ended with the status %d, and printed:\n%s", status, inset(printed))
	}
	return run.Output, nil
}

// The examples are run once, for every test that looks at how they went.
var (
	examplesRun sync.Once
	went        struct {
		err      error
		outcomes []outcome
		reached  []string // each place a job reached for, and what was there
		asked    []string // each kind a plugin's call asked for
		takes    []string // the options of each call on something left open
		gave     []string // the names of what each call gave back
		// calls are the calls a script finds on each plugin and library,
		// and unasked is why it could not be asked.
		calls   map[string][]string
		unasked error
	}
)

// ran runs every example of every page of the manual, in the world, and
// gives what became of each.
func ran() ([]outcome, error) {
	examplesRun.Do(func() {
		w, err := newWorld()
		if err != nil {
			went.err = err
			return
		}
		defer w.close()
		was, _ := os.Getwd()
		defer os.Chdir(was)
		// The runner's own things are the world's: where programs are
		// looked for, where scratch directories are made, and the variables
		// that hold the secrets a command reads.
		before := map[string]*string{}
		set := func(name, value string) {
			if old, was := os.LookupEnv(name); was {
				before[name] = &old
			} else {
				before[name] = nil
			}
			os.Setenv(name, value)
		}
		defer func() {
			for name, old := range before {
				if old == nil {
					os.Unsetenv(name)
				} else {
					os.Setenv(name, *old)
				}
			}
		}()
		set("PATH", filepath.Join(w.dir, "bin"))
		set("TMPDIR", w.tmp)
		set("HOME", w.dir)
		set("XDG_CONFIG_HOME", "")
		set("CORBIE_CONFIG", "")
		set("CORBIE_STORE", "")
		for name, value := range w.secrets {
			set(name, value)
		}
		carried := map[string]string{}
		for n, page := range manual.Pages() {
			s, err := w.session(n)
			if err != nil {
				went.err = err
				return
			}
			for name, text := range carried {
				if err := os.WriteFile(filepath.Join(s.dir, name), []byte(text), 0o644); err != nil {
					went.err = err
					return
				}
			}
			if page.Group == "Start" {
				for _, ex := range examplesOf(page) {
					for _, f := range ex.files {
						if _, given := carried[f.name]; !given {
							carried[f.name] = w.placed(f.text)
						}
					}
				}
			}
			for _, ex := range examplesOf(page) {
				went.outcomes = append(went.outcomes, s.run(ex))
			}
			s.end()
		}
		went.calls, went.unasked = w.calls()
		went.reached, went.asked, went.takes, went.gave = w.lines("reached"), w.lines("asked"), w.lines("takes"), w.lines("gave")
	})
	return went.outcomes, went.err
}
