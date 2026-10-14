// Package exec is the plugin for starting a program and getting what it
// printed.
package exec

import (
	"context"
	"errors"
	"io"
	osexec "os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "exec", Page: page, Calls: map[string]*plugin.Call{
		"run": {Options: []string{"args", "input", "env", "dir", "timeout", "as"}, Do: run},
	}})
}

func run(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	name, err := plugin.Text(first, "the program")
	if err != nil {
		return nil, err
	}
	to := "exec:" + name
	var secrets []*plugin.Secret
	reveal := func(v any) (string, error) {
		if s, ok := v.(*plugin.Secret); ok {
			secrets = append(secrets, s)
		}
		return job.Reveal(v, to)
	}
	given, err := opt.List("args")
	if err != nil {
		return nil, err
	}
	args := make([]string, len(given))
	for i, arg := range given {
		if n, ok := arg.(float64); ok {
			arg = formatNumber(n)
		}
		if args[i], err = reveal(arg); err != nil {
			return nil, err
		}
	}
	vars, err := opt.Map("env")
	if err != nil {
		return nil, err
	}
	var env []string
	for name, value := range vars {
		text, err := reveal(value)
		if err != nil {
			return nil, err
		}
		env = append(env, name+"="+text)
	}
	var input io.Reader
	if s, ok := opt["input"].(*plugin.Secret); ok {
		text, err := reveal(s)
		if err != nil {
			return nil, err
		}
		input = strings.NewReader(text)
	} else if input, _, _, err = job.Reading(opt["input"]); err != nil {
		// A blob is given to the program as it is read, a part at a time.
		return nil, err
	}
	dir, err := opt.Text("dir", "")
	if err != nil {
		return nil, err
	}
	if dir != "" {
		if dir, err = job.Path(dir, false); err != nil {
			return nil, err
		}
	}
	limit, err := opt.Duration("timeout", time.Minute)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(job.Context, limit)
	defer cancel()
	cmd, err := job.Program(ctx, name, plugin.ProgramOptions{Args: args, Env: env, Dir: dir, Stdin: input, Secrets: secrets})
	if err != nil {
		return nil, err
	}
	form, err := opt.Text("as", "text")
	if err != nil {
		return nil, err
	}
	// What the program prints is handed back whole, or goes into a blob as
	// it is printed. Either way the program is never held up.
	errOut := &plugin.Whole{}
	cmd.WaitDelay = 2 * time.Second
	out, printed := job.Give(form, "text/plain", "what the program printed", func(w io.Writer) error {
		stdout := &plugin.Draining{To: w}
		cmd.Stdout, cmd.Stderr = stdout, &plugin.Draining{To: errOut}
		err = cmd.Run()
		return stdout.Err
	})
	var exit *osexec.ExitError
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return nil, plugin.Errorf(plugin.TimedOut, "the program %q ran past the %s its call gave it", name, limit)
	case errors.As(err, &exit):
		// One that ran and ended badly is an answer, with its status.
	case err != nil:
		return nil, plugin.Errorf(plugin.NotFound, "the program %q could not be started: %v", name, err)
	}
	if printed != nil {
		return nil, printed
	}
	if errOut.Over {
		return nil, plugin.TooLarge("what the program printed as errors")
	}
	return map[string]any{"status": float64(cmd.ProcessState.ExitCode()), "out": out, "err": errOut.Text()}, nil
}

func formatNumber(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

const page = `exec: this plugin runs a program on the runner.

  exec.run(program, { args, input, env, dir, timeout, as })   gives { status, out, err }

program is a name on the runner's PATH, or a path. A path with no leading / is
in the job's scratch directory (` + "`corbie manual scripts`" + `).

Options:
  args      a list of text, numbers and handles to secrets (` + "`corbie manual secrets`" + `)
  input     stdin: text, bytes, a blob or a handle
  env       a map of text and handles; the program gets these variables and PATH, and no others
  dir       the working directory; default the job's scratch directory
  timeout   seconds, or text such as "30s"; default 1m
  as        the form of out: "text", "bytes", "json" or "blob"; default "text"

Returns:
  status   the exit code; a code other than 0 is not an error
  out      stdout: at most 64 MiB, or a blob of any size (` + "`corbie manual blobs`" + `)
  err      stderr, as text: at most 64 MiB

Errors:
  not_found   the program is not found, or cannot be started
  timeout     the program ran past timeout
  bad_input   out or err is over 64 MiB

Needs these permissions:
  exec:program               the program
  fs.read:path               the directory given as dir
  secret:NAME@exec:program   a secret sent to the program

The program runs as the runner's user, and Corbie does not limit what it does.

This script reads stdout as a blob, and writes a blob to a file for a program.

    allow: [exec:pg_dump, exec:pdftotext, fs.write:in.pdf, fs.read:out.txt]

    const dump = exec.run("pg_dump", { args: ["shop"], as: "blob" }).out

    const document = blobs.make("%PDF A short document.", { type: "application/pdf" })
    fs.write("in.pdf", { blob: document })                   // a blob into the scratch directory
    exec.run("pdftotext", { args: ["in.pdf", "out.txt"] })
    return { dump, text: blobs.file("out.txt") }             // and a file out of it

This script runs git in a directory and checks its exit code.

    allow: [exec:git, fs.read:/srv/repo]

    const made = exec.run("git", { args: ["log", "--oneline", "-5"], dir: "/srv/repo" })
    if (made.status !== 0) throw new Error(made.err)
    return made.out.trim().split("\n").length                // gives 5

This script sends a secret to a program in an environment variable.

    allow: [exec:deploy, "secret:DEPLOY_TOKEN@exec:deploy"]

    const sent = exec.run("deploy", { args: ["site"], env: { DEPLOY_TOKEN: secrets.get("DEPLOY_TOKEN") } })
    return sent.out                                          // gives "deployed site\n"
`
