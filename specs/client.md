# Client

A client starts runs and reads what happened to them. This area is the client as a Go package, and the three things built on it and on nothing else: the command line, the gateway and the server for models.

## The package

The client is a public Go package in the Corbie repository. An application written in Go uses it directly. Everything else Corbie offers a client goes through it, so there is one behaviour and one set of words for all of them.

It does two kinds of thing.

It **sends**: submit a tree, start a run of a location, cancel runs, send an event, answer a ticket. To submit a tree it reads every file, writes each machine out with its includes resolved and its short forms filled out, checks the whole and reports everything wrong at once, and uploads the definitions and the files they name side by side, a bounded number at a time. A start names a location and may carry input, labels, a list that narrows what the run may do, and a timeout. Any value in the input may be a file, and the start is sent once every file is in the store.

It **reads**: list runs and follow the list as it changes; show one run, where it stands and how it ended; read a run's record so far, children included, and follow it as it grows; fetch a file from a result; show the definition a run was started from.

Reading is the one way of watching described under Instances. The package owns the awkward corners so that nothing built on it meets them: a head that is a moment behind its record, a run that has been sent and not yet taken up, a start that was refused, a receipt that arrives before the head does, a head that is moving from running to finished.

To a client a run is an instance: a root at a location in one of its trees, or one below it.

## One shape for a run

Wherever a run is shown, it has the same shape and the same words.

A run shows its location, and its key at a keyed location. A run is `waiting` to start, `running`, `done`, `failed` or `cancelled`. A failed run says why: an `error`, a `timeout`, or `denied` with the refusal. A finished run has its output, and a list of the permissions it used, which for a machine are those of every job in it and below it.

A run waiting on a ticket shows the ticket's name. The ticket's value is in the record, which only the client of the run's tree can read. Only that client is shown the run's title, its input, its output, the failure's text and the permissions it used. A client that called the run from outside the tree is shown its status and its output or error, from the call's result, and the run its call started (see instance.md).

A blob in any value is shown as its type, its size and the path to ask for it by. Its key is never shown, at the command line, over HTTP or to a model.

## The command line

```
corbie init                             create a controller, a runner and a client on this computer; by default the store is in the config, and corbie init store sets up the store's server alone
corbie start [<name> [<its flags>]]…    run the controller, a runner, the gateway, the store's server or a program, until stopped
corbie check                            start one small run, and report which part of a setup did not respond
corbie test flow.yaml                   run a definition or a script in this process, with no setup; given directories, read every machine in them

corbie run submit flow.yaml             record one file as a tree, and start it if it is a singleton
corbie run submit <dir> --at <prefix>   record a directory as a tree, and start its singletons
corbie run submit <location>            start a run of a transient location and print its ID
corbie run ls                           list runs
corbie run show <id>                    show one run: its status and its result
corbie run cancel <id>                  cancel a run in one of this client's trees
corbie run send <id> <event> [data]     send an event to a run
corbie run reply <id> <ticket> [value]  answer a ticket that a run waits on

corbie controller …                     enrol and remove runners and clients, set ceilings and tags, manage secrets and values, cancel any run
corbie runner …                         print a card, trust a controller
corbie client …                         print a card, trust a controller

corbie manual [page]                    print a page of the manual, or list the pages
corbie manual web                       serve the pages to a browser on this computer, until stopped
corbie mcp                              serve the manual and the commands to a model, as an MCP server
corbie version                          print the version, the machine types and the plugins
```

The commands form a tree. A word at the top is a command, or begins several commands, as `run` begins the six that act on runs. `corbie help` prints the top, each word with its summary or the words that follow it, then the names of the programs on PATH. With `corbie-ui` and `corbie-rook` on PATH it prints:

```
  init         create a controller, a runner and a client on this computer
  start        run the controller, a runner, the gateway, the store's server or
               a program, until stopped
  check        start one small run, and report which part of a setup did not
               respond
  test         run a definition or a script in this process, with no setup
  run          submit, ls, show, cancel, send, reply
  controller   card, enrol, remove, parties, secret, value, policy, cancel
  runner       card, trust, remove
  client       card, trust, remove
  manual       print a page of the manual, or list the pages; web
  mcp          serve the manual and the commands to a model, as an MCP server
  version      print the version, the machine types and the plugins

Programs on PATH, which corbie <name> runs: rook, ui.
```

- `corbie run` and `corbie run --help` list the commands of `corbie run` in the same rows, and the same holds for `controller`, `controller secret`, `controller value`, `runner` and `client`.
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

`run show`, `run cancel` and `run send` name a run in one of three ways:

```
<id>                  one run, by its ID
/home/devices/boiler  the singleton at a location; for run show, the one run of this client's running there; for run cancel, every run there
/home/chats#alice     the running run of key alice at a keyed location
```

`run send` to a key with no running run starts one, as the file's `lifetime: keyed` says. A client sends to the runs of its own trees with no `send:` entry, and only it cancels them. A send to another tree needs `send:` in the client's ceiling and `client:<its name>` in the receiver's `receive:` (see permission.md).

`run show` prints a run's title, and `run ls` does not, since a listing reads only the heads. `run show` can also print one step, one job's input and output, or the definition, can save a result's files to a directory, and with `--wait` stays until the run ends and prints its result.

`run submit <location>` sends a start; `run submit --wait` waits for the run it started and `run submit --watch` follows it. The tree's client starts a transient location with no `send:` entry. Another client's start is a call: its ceiling needs `send:` for the location, the file's `receive:` must list `client:<its name>`, and the start takes no labels or `--allow`. Under the ID it printed, it is shown only the call's result: the run's status and its output or error, and the result's files.

A secret's value is never on the command line. It is read only from the source the command names, and a value not found there is a mistake that sends nothing:

```
controller secret set NAME            typed at the terminal, not shown
controller secret set NAME --env      the environment variable NAME
controller secret set NAME -          standard input, to the end
```

`--at <location>` sets a secret for the runs at that location and below. With no `--at` it is set at `/`, for every tree. A plain value is not secret, so it is given on the line (see secret.md):

```
corbie controller value set DB_HOST db.prod.lan --at /prod
corbie controller value list
```

A file given alone is a tree of one file, at `--at`, by default `/` and the file's name without its extension. An argument that starts with `/`, does not end in `.yaml` or `.js`, and is no file or directory on this computer is read as a location. A file's name that is not there is reported as not found.

A script file can be submitted as it is. The package wraps it in a flow of one stage, with the permissions it may use and its limits, which the command line takes as flags.

`corbie check` is the last step of setting anything up. It submits a small transient machine of its own as a tree at `/corbie-check/<client>`, so several clients can check one controller, starts a run of it, which asks for one job and sets one timer, follows it as any client does, and reports each thing that had to happen, with how long it took. While it waits it says what for.

The client's ceiling needs an `at:` entry that matches `/corbie-check/<client>`. Without one, the check's first line is the refused submit, with the line to add.

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

`corbie <word>`, where the word begins no command, runs the program `corbie-<word>` from PATH with the rest of the line, this process's standard input, output, error and environment, and ends with the program's exit status. A command in the table always wins. A model through `corbie mcp` runs a program only for a command the server was started with permission for.

When no program `corbie-<word>` is on PATH, `corbie` names the program it looked for, prints `corbie --help lists the commands.` and ends with status 2.

`corbie help` lists the programs `corbie <word>` runs by their names, and starts none of them. A command, the words that begin commands, and a program each print their help with `--help`.

A program written in Go reads its command line with the package's `cli.Program`, from a table of commands as Corbie's own is made. It then reads as one of Corbie's commands: the same `--help`, `--json`, `--config` and `--store`, flags anywhere among its arguments, and the same mistakes, failures and exit statuses. Its help says which of the three flags it takes. The command named `""` is the one run when no other is named. Rook has one command:

```go
os.Exit(cli.Program("rook", []cli.Command{
	{Name: "init", Usage: "<directory> [--bridges <names>]", Summary: "make a directory a project, or bring its ready-made machines up to date", Run: cmdInit},
}, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
```

```
$ corbie rook init
corbie rook init: name one directory

usage: corbie rook init <directory> [--bridges <names>]
```

The program is given `manual` and `version` beside its own commands, and no `mcp`. `version` prints the program's version and the version of the Corbie it is built with. Its manual is Corbie's, with a page for each of its commands and any pages it adds to the manual. A model runs its commands through `corbie mcp`, as For a model says:

```
corbie rook manual                       Corbie's pages, the program's pages and its commands
corbie rook manual rook-project          a page the program added
corbie mcp --allow "rook init"           a model may run corbie rook init, and no other
```

```
$ corbie rook version
corbie-rook 0.7.0
corbie 0.15.0
```

`corbie start` with nothing named runs the roles the config holds: its controller, its runner, and the store's server when `corbie init store` set it up. A name means exactly what is named:

```
corbie start                      the controller and the runner, after corbie init
corbie start runner               the runner alone
corbie start ui                   the UI alone, beside roles that systemd runs
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

`corbie test` is an ordinary run: the same roles, in one process, on a store in memory, with ceilings that allow everything. It records the file as a tree of one file and starts it. A singleton is started by that submit, so a flag of a start given with one is a mistake. A keyed file is refused: its runs start when an event is sent to a key. `corbie test <file> --definition` prints the machine as the client writes it out, each value with its file and line, runs nothing, and takes no flag of a start.

`corbie test` sets values and secrets at `/` of the controller in its process, read from the sources `controller value set` and `controller secret set` read:

```
--value NAME=VALUE    a value
--secret NAME         a secret, typed at the terminal, not shown
--secret-env NAME     a secret, from the environment variable NAME
```

## A tree

`corbie run submit <dir> --at <prefix>` records a directory as a tree. Each file with a type line gets a location: its path under the prefix, without `.yaml`. A file without one is a piece, read only by including it. `--at` defaults to `/` and the directory's own name, and `.` uses the directory's real name.

```yaml
# house/devices/boiler.yaml
statechart: boiler
lifetime: singleton
initial: idle
states:
  idle:
    on: { set: applying }
  applying:
    job: { include: ./boiler-job.yaml }
    on: { done: idle, failed: idle }

# house/devices/boiler-job.yaml           no type line: a piece
script: ./boiler.js
input: { target: "${event.data.target}" }
allow: [net:boiler.lan:443, "secret:BOILER_PASSWORD@net:boiler.lan:443"]

# house/schedules/away.yaml
flow: away
allow: [send:devices/boiler]
stages:
  lower:
    job: { send: { to: { at: devices/boiler }, event: set, data: { target: 12 } } }
```

```
$ corbie run submit house --at /home
/home/devices/boiler   singleton   started <id>
/home/schedules/away   transient   recorded
not set at its location or above: secret BOILER_PASSWORD for /home/devices/boiler

$ corbie run submit /home/schedules/away --wait
```

A submit does this, and exits when each run it cancelled has ended and each start has been taken:

1. It reads every `*.yaml` in the directory and below it, but for `.git`, and writes out and checks each machine.
2. It records every file at its location in the store, and removes the locations whose files have gone, cancelling every run there.
3. For a changed file, the runs already running at its location do as its `update:` says.
4. It starts each singleton with no running run, and nothing else.
5. It prints the names of values and secrets the files use that are not set at their locations or above. This is a reminder, not a refusal.

A tree belongs to the client that first submitted it. Every run keeps the definition it started with, and a submit changes no other tree.

- A submit at a prefix inside or above another tree is refused, naming that tree. `/` is never a tree.
- A submit at a tree another client owns is refused, naming the owner.
- A submit at a prefix that no `at:` entry of the client's ceiling matches is refused.
- A file under a directory whose name starts with a dot is read as any other: `.rook/review.yaml` submitted at `/project` is at `/project/.rook/review`.
- A file beside a directory of the same name, as `boiler.yaml` beside `boiler/x.yaml`, is refused, because a location has nothing below it.
- A file with a problem is reported, and its location keeps what was recorded before. A problem with one file leaves the rest done.
- A file whose approvals have no ask is recorded, since it may run as another machine's child under that machine's ask. A run started at its location is refused.
- A singleton is given no input, and one that needs some is refused, and reported. It reads the context's absolute path as `${run.context}`.
- When a client submits a different directory at a prefix it holds, the submit prints `/home: last submitted from /srv/old-house`.
- `corbie run cancel <location>` after a submit cancels the runs there, so the next of each starts on the new definition.
- `corbie test` given directories reads their machines as this command does, prints each one ok or its problems, and starts and cancels nothing. It reads no config and no store, so it changes nothing wherever it runs.

## The gateway

The gateway offers the package over HTTP for applications not written in Go. Its routes start a run of a location with an input, list, show and cancel runs, read a run's record, definition and files, and send events and answers to a run. A listing and a record are each given as they stand, and with `?watch` as a stream that follows them. Files in an input go in as uploads and come out by their path in a result.

A start is the gateway's client starting a transient location, as `corbie run submit <location>` does. The location's file lists `client:<the gateway's client>` in `receive:`, or the gateway's client owns the tree. A start in a tree another client owns is a call, and the run is shown from the call's result. A route names a run by its ID. A listing narrowed with `?at=<location>`, and `&key=<key>` at a keyed location, gives the IDs of the runs there.

A gateway is started with `corbie start gateway`, alone or beside a controller and a runner. It is one client with one ceiling. Callers show it a token it was given when it was started, and all of them act as that client. Different ceilings need different gateways. A request with a field the gateway does not know is refused.

It keeps nothing itself. It runs wherever a process can stay up, and is not offered on serverless hosts.

## For a model

`corbie mcp` serves two tools to a model: the manual, and the command line. The model may run only the commands it was started with permission for, each named by its whole name, as in `--allow "run submit"`, with the config the server was started on. It needs nothing running and keeps nothing.

A permission may name a program's command, as in `--allow "rook init"`. When the server starts, it runs `corbie rook manual --json` once:

1. The program's pages join the manual, and its commands' pages name its commands.
2. The model is told the program's page named after it, `rook`, after Corbie's briefing.
3. The model runs `rook init` with the command tool, as `corbie rook init` runs it, with the server's config and store, and gets back what the program printed.

A model never runs a program the server's line does not name, and `corbie start` never starts a program for a model.

Each call runs by itself. A call that stays until a run ends keeps no other call from being answered, and it ends when the model's host stops it.
