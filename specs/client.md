# Client

A client starts runs and reads what happened to them. This area is the client as a Go package, and the three things built on it and on nothing else: the command line, the gateway and the server for models.

## The package

The client is a public Go package in the Corbie repository. An application written in Go uses it directly. Everything else Corbie offers a client goes through it, so there is one behaviour and one set of words for all of them.

It does two kinds of thing.

It **sends**: start a run, cancel one, send an event to one, answer a ticket. To start a run it takes a definition and the files it names, fills out the short forms, checks the whole and reports everything wrong at once, uploads the files, and sends the start. A start may carry input, labels, run secrets, a list that narrows what the run may do, and a timeout. Any value in the input may be a file.

It **reads**: list runs and follow the list as it changes; show one run, where it stands and how it ended; read a run's record so far, children included, and follow it as it grows; fetch a file from a result; show the definition a run was started from.

Reading is the one way of watching described under Instances. The package owns the awkward corners so that nothing built on it meets them: a head that is a moment behind its record, a run that has been sent and not yet taken up, a start that was refused, a receipt that arrives before the head does, a head that is moving from running to finished.

To a client a run is an instance: a root it started, or one below it.

## One shape for a run

Wherever a run is shown, it has the same shape and the same words.

A run is `waiting` to start, `running`, `done`, `failed` or `cancelled`. A failed run says why: an `error`, a `timeout`, or `denied` with the refusal. A finished run has its output, and a list of the permissions it used, which for a machine are those of every job in it and below it.

A run waiting on a ticket shows the ticket's name. The ticket's value is in the record, which only the client that started the run can read. Only that client is shown the run's input, its output, the failure's text and the permissions it used.

A blob in any value is shown as its type, its size and the path to ask for it by. Its key is never shown, at the command line, over HTTP or to a model.

## The command line

```
corbie init                             create a controller, a runner and a client on this computer; by default the store is in the config, and corbie init store sets up the store's server alone
corbie start [<name> [<its flags>]]…    run the controller, a runner, the gateway, the store's server or a program, until stopped
corbie check                            start one small run, and report which part of a setup did not respond
corbie test flow.yaml                   run a definition or a script in this process, with no setup

corbie run submit flow.yaml             start a run and print its ID
corbie run ls                           list runs
corbie run show <id>                    show one run: its status and its result
corbie run cancel <id>                  cancel a run that this client started
corbie run send <id> <event> [data]     send an event to a run that this client started
corbie run reply <id> <ticket> [value]  answer a ticket that a run waits on

corbie controller …                     enrol and remove runners and clients, set ceilings and tags, manage secrets, cancel any run
corbie runner …                         print a card, trust a controller
corbie client …                         print a card, trust a controller

corbie manual [page]                    print a page of the manual, or list the pages
corbie manual web                       serve the pages to a browser on this computer, until stopped
corbie mcp                              serve the manual and the commands to a model, as an MCP server
corbie version                          print the version, the machine types and the plugins
```

The commands form a tree. A word at the top is a command, or begins several commands, as `run` begins the six that act on runs. `corbie help` prints the top, each word with its summary or the words that follow it, then the names of the programs on PATH. With `corbie-ui` and `corbie-bulldog` on PATH it prints:

```
  init         create a controller, a runner and a client on this computer
  start        run the controller, a runner, the gateway, the store's server or
               a program, until stopped
  check        start one small run, and report which part of a setup did not
               respond
  test         run a definition or a script in this process, with no setup
  run          submit, ls, show, cancel, send, reply
  controller   card, enrol, remove, parties, secret, policy, cancel
  runner       card, trust, remove
  client       card, trust, remove
  manual       print a page of the manual, or list the pages; web
  mcp          serve the manual and the commands to a model, as an MCP server
  version      print the version, the machine types and the plugins

Programs on PATH, which corbie <name> runs: bulldog, ui.
```

- `corbie run` and `corbie run --help` list the commands of `corbie run` in the same rows, and the same holds for `controller`, `controller secret`, `runner` and `client`.
- `corbie manual`'s help ends with the row of `web`.
- A command's usage line is in its own help, never in these rows.
- The rows fit 80 columns: a text too long goes on below, under itself.

`run show` prints where a run stands. `run show --record` prints what happened to it: each entry of its record so far, its children's included, and then the run as it stands.

```
$ corbie run show <id>
<id>  running  statechart door  step 1
  waiting on ticket knock

$ corbie run show <id> --record
15:56:21 4efcf50f63  step 1  <- start
<id>  running  statechart door  step 1
  waiting on ticket knock
```

Following from a terminal is one flag, `--watch`. It prints what the command prints without it, and keeps going:

- `run ls --watch` prints the listing, then each run that appears or changes.
- `run show --watch` prints the record, then each entry as it is written, until the run ends.

`run show` can also print one step, one job's input and output, or the definition, can save a result's files to a directory, and with `--wait` stays until the run ends and prints its result. `run submit` sends a start; `run submit --wait` waits for the run it started and `run submit --watch` follows it.

A secret's value is never on the command line. It is read only from the source the command names, and a value not found there is a mistake that sends nothing:

```
--secret NAME                         typed at the terminal, not shown
--secret-env NAME                     the environment variable NAME
controller secret set NAME            typed at the terminal, not shown
controller secret set NAME --env      the environment variable NAME
controller secret set NAME -          standard input, to the end
```

A script file can be submitted as it is. The package wraps it in a flow of one stage, with the permissions it may use and its limits, which the command line takes as flags.

`corbie check` is the last step of setting anything up. It starts a small run of its own, which asks for one job and sets one timer, follows it as any client does, and reports each thing that had to happen, with how long it took. While it waits it says what for.

```
corbie check
  the controller took the start    ok     0.6s
  a runner took the job            never
    The job went to cloud-runner. … On a serverless host, the bucket does not
    call the runner's function when something is written under cloud-runner/inbox/ …
```

A line that is not `ok` says what leaves it so, on a computer and on a serverless host. `never` is something that did not happen in the time waited. `late` is something that took more than twenty seconds: on a serverless host the once-a-minute call does whatever the bucket's own call should have, so a call that is missing shows only as slowness. A timer is never late, since on a serverless host it waits for that call. If the run ended for a reason of its own, such as no runner having a tag it was sent to, the check shows that reason.

The check asks the host nothing. What it reports is what turned up in the store, so it is the same check on every host, and it fails unless every line is `ok`.

Every command can print its result as JSON, in the one shape above. A command ends with success, with a mistake in how it was written, or with failure, which includes a run that did not end well.

The config and the store may be named by environment variables. No other flag is read from the environment.

The commands, their help, the manual's pages about them and the names by which a model is allowed to run them all come from one table.

`corbie <word>`, where the word begins no command, runs the program `corbie-<word>` from PATH with the rest of the line, this process's standard input, output, error and environment, and ends with the program's exit status. A command in the table always wins. A model through `corbie mcp` runs no program.

When no program `corbie-<word>` is on PATH, `corbie` names the program it looked for, prints `corbie --help lists the commands.` and ends with status 2.

`corbie help` lists the programs `corbie <word>` runs by their names, and starts none of them. A command, the words that begin commands, and a program each print their help with `--help`.

A program written in Go reads its command line with the package's `cli.Program`, from a table of commands as Corbie's own is made. It then reads as one of Corbie's commands: the same `--help`, `--json`, `--config` and `--store`, flags anywhere among its arguments, and the same mistakes, failures and exit statuses. Its help says which of the three flags it takes. The command named `""` is the one run when no other is named:

```go
os.Exit(cli.Program("bulldog", []cli.Command{
	{Name: "", Usage: "[<folder>]", Summary: "serve an office until stopped", Run: serve},
	{Name: "check", Usage: "<folder>", Summary: "read an office, and print everything wrong with it", Run: check},
}, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
```

```
$ corbie bulldog check
corbie bulldog check: it takes one folder, and 0 are named

usage: corbie bulldog check <folder>
```

The program is given `manual` and `mcp` beside its own commands, as Corbie has them. Its manual is Corbie's, with a page for each of its commands and any pages it adds to the manual. Its model server runs its commands, under a tool named after the program, each giving back what it prints on the command line, and tells a model the manual's page named after the program when it connects:

```
corbie bulldog manual                    Corbie's pages, the program's pages and its commands
corbie bulldog manual office             a page the program added
corbie bulldog mcp --allow check         a model may run corbie bulldog check, and no other
```

`corbie start` with nothing named runs the roles the config holds: its controller, its runner, and the store's server when `corbie init store` set it up. A name means exactly what is named:

```
corbie start                      the controller and the runner, after corbie init
corbie start runner               the runner alone
corbie start bulldog              Bulldog alone, beside roles that systemd runs
```

The words after a name, up to the next name, are its own: a role's flags, or a program's arguments. `--config`, `--store`, `--json` and `--help` are `corbie start`'s wherever they are written.

```
corbie start controller --interval 1s runner --jobs 8 ui --listen 127.0.0.1:8383
```

A program's word that is itself a role's or a program's name begins the next name. `corbie start runner ui controller` runs the runner, the UI and the controller, and never the UI acting as the controller; that UI is run on its own, as `corbie ui controller`.

`corbie start controller runner ui` starts `corbie-ui` beside the roles, once they are set up.

- `corbie start` starts the program with the words written after its name and an empty standard input, and what it prints is `corbie start`'s own output, with no prefix.
- Its environment is this process's, with `CORBIE_CONFIG` and `CORBIE_STORE` set as `--config` and `--store` were given.
- When the program stops, `corbie start` stops the roles and the other programs, waits for them, and fails naming the program.
- When `corbie start` is stopped, it interrupts each program and waits for it, for up to ten seconds before killing it.
- On a serverless host, `corbie start` refuses a program before anything starts.

`corbie test` is an ordinary run: the same roles, in one process, on a store in memory, with ceilings that allow everything.

## The gateway

The gateway offers the package over HTTP for applications not written in Go. Its routes start, list, show and cancel runs, read a run's record, definition and files, and send events and answers to a run. A listing and a record are each given as they stand, and with `?watch` as a stream that follows them. Files go in as uploads and come out by their path in a result.

A gateway is started with `corbie start gateway`, alone or beside a controller and a runner. It is one client with one ceiling. Callers show it a token it was given when it was started, and all of them act as that client. Different ceilings need different gateways. A request with a field the gateway does not know is refused.

It keeps nothing itself. It runs wherever a process can stay up, and is not offered on serverless hosts.

## For a model

`corbie mcp` serves two tools to a model: the manual, and the command line. The model may run only the commands it was started with permission for, each named by its whole name, as in `--allow "run submit"`, with the config the server was started on. It needs nothing running and keeps nothing. A program read with `cli.Program` serves the same two tools with its own `mcp`, and its model runs only the program's commands.

Each call runs by itself. A call that stays until a run ends keeps no other call from being answered, and it ends when the model's host stops it.
