# Client

A client starts runs and reads what happened to them. This area is the client as a Go package, and the three things built on it and on nothing else: the command line, the gateway and the server for models.

## The package

The client is a public Go package in the Corbie repository. An application written in Go uses it directly. Everything else Corbie offers a client goes through it, so there is one behaviour and one set of words for all of them.

It does two kinds of thing.

It **sends**: submit a tree, remove a tree, start a run of a location, cancel runs, send an event, answer a ticket, set, remove and list its secrets and values. To submit a tree it reads every file, writes each machine out with its includes resolved and its short forms filled out, checks the whole and reports everything wrong at once, and uploads the definitions and the files they name side by side, a bounded number at a time. A start names a location and may carry input, labels, a list that narrows what the run may do, and a timeout. Any value in the input may be a file, and the start is sent once every file is in the store.

It **reads**: list runs and follow the list as it changes; show one run, where it stands and how it ended; read a run's record so far, children included, and follow it as it grows; fetch a file from a result; show the definition a run was started from.

Reading is the one way of watching described under Instances. The package owns the awkward corners so that nothing built on it meets them: a head that is a moment behind its record, a run that has been sent and not yet taken up, a start that was refused, a receipt that arrives before the head does, a head that is moving from running to finished.

To a client a run is an instance: a root at a location in one of its trees, or one below it.

## One shape for a run

Wherever a run is shown, it has the same shape and the same words.

A run shows its location, its key at a keyed location, and its cause: the step of another run, or the client's message, that started it. A run is `waiting` to start, `running`, `done`, `failed` or `cancelled`. A failed run says why: an `error`, a `timeout`, or `denied` with the refusal. A finished run has its output, and a list of the permissions it used, which for a machine are those of every job in it and below it.

A run waiting on a ticket shows the ticket's name. The ticket's value is in the record, which only the client of the run's tree and the clients its file's `read:` lists can read. Only those clients are shown the run's title, its input, its output, the failure's text and the permissions it used, and a client's `run ls` lists only the runs of its trees and those whose head lists it. Another client that asks for more than the run's status is told there is no such run of its own, and which client's tree the run is in. A client that called the run from outside the tree is shown its status and its output or error, from the call's result, and the run its call started (see instance.md).

A blob in any value is shown as its type, its size and the path to ask for it by. The path is the blob's place in the value, and `file` for a value that is itself a blob. Its key is never shown, at the command line, over HTTP or to a model.

## The command line

```
corbie init                             create a controller, a runner and a client on this computer; by default the store is in the config, and corbie init store sets up the store's server alone
corbie start [<name> [<its flags>]]…    run the controller, a runner, the gateway, the store's server or a program, until stopped
corbie up [<name> [<its flags>]]…       run a controller, a runner and a gateway on a directory of machines, set up in the directory
corbie check                            start one small run, and report which part of a setup did not respond
corbie test flow.yaml                   run a definition or a script in this process, with no setup; given directories, read every machine in them

corbie run submit flow.yaml             record one file as a tree, and start it if it is a singleton
corbie run submit <dir> --at <prefix>   record a directory as a tree, and start its singletons
corbie run remove <prefix>              remove a tree, with its runs and the secrets and values set in it
corbie run start <location>             start a run of a transient location and print its ID
corbie run ls                           list runs
corbie run show <id>                    show one run: its status and its result
corbie run cancel <id>                  cancel a run in one of this client's trees
corbie run send <id> <event> [data]     send an event to a run
corbie run reply <id> <ticket> [value]  answer a ticket that a run waits on

corbie controller …                     enrol and remove runners and clients, set ceilings, tags and prefixes, manage secrets and values, cancel any run, remove any tree
corbie runner …                         print a card, trust a controller
corbie client …                         print a card, trust a controller, show which client a command is sent as, manage secrets and values in this client's trees

corbie add <pack>                       write a pack of ready-made machines into a project
corbie ui                               serve the UI to a browser, until stopped

corbie manual [page]                    print a page of the manual, or list the pages
corbie manual web                       serve the pages to a browser on this computer, until stopped
corbie mcp                              serve the manual and the commands to a model, as an MCP server
corbie version                          print the version, the machine types and the plugins
```

The commands form a tree. A word at the top is a command, or begins several commands, as `run` begins the seven that act on runs. `corbie help` prints the top, each word with its summary or the words that follow it, then the names of the programs on PATH. With `corbie-console` on PATH it prints:

```
  init         create a controller, a runner and a client on this computer
  start        run the controller, a runner, the gateway, the store's server or
               a program, until stopped
  check        start one small run, and report which part of a setup did not
               respond
  test         run a definition or a script in this process, with no setup
  run          submit, remove, start, ls, show, cancel, send, reply
  controller   card, enrol, remove, parties, secret, value, policy, cancel, tree
  runner       card, trust, remove
  client       card, trust, remove, show, secret, value
  manual       print a page of the manual, or list the pages; web
  mcp          serve the manual and the commands to a model, as an MCP server
  add          write a pack of ready-made machines into a project
  version      print the version, the machine types and the plugins
  ui           serve the UI to a browser until stopped

Programs on PATH, which corbie <name> runs: console.
```

- `corbie run` and `corbie run --help` list the commands of `corbie run` in the same rows, and the same holds for `controller`, `controller secret`, `controller value`, `runner`, `client`, `client secret` and `client value`.
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

A time Corbie prints is in UTC.

Following from a terminal is one flag, `--watch`. It prints what the command prints without it, and keeps going:

- `run ls --watch` prints the listing, then each run that appears or changes.
- `run show --watch` prints the record, then each entry as it is written, until the run ends.

`run show`, `run cancel` and `run send` name a run in one of three ways:

```
<id>                  one run, by its ID
/home/devices/boiler  the singleton at a location; for run show, the one run of this client's running there; for run cancel, every run there
/home/chats#alice     the running run of key alice at a keyed location
```

`run send` to a key with no running run starts one, as the file's `lifetime: keyed` says, and `run send` to a transient location starts a new run of it. A client sends to the runs of its own trees with no `send:` entry, and only it cancels them. A send to another tree needs `send:` in the client's ceiling and `client:<its name>` in the receiver's `receive:` (see permission.md).

`run show` prints a run's title, and `run ls` does not, since a listing reads only the heads. For a run another run started, `run show` prints `cause: <id> step <n>`. For a run a client's message started for a principal, it prints `principal:`, and `--record` writes the principal after each event a client sent for one. In `--json`, a listing's row and `run show` give `cause`, and each step of a record gives the cause of each event and the run's `state` (see instance.md). `run show` can also print one step, one job's input and output, or the definition, can save a result's files to a directory, and with `--wait` stays until the run ends and prints its result.

`run start <location>` sends a start; `run start --wait` waits for the run it started and `run start --watch` follows it. The tree's client starts a transient location with no `send:` entry. Another client's start is a call: its ceiling needs `send:` for the location, the file's `receive:` must list `client:<its name>`, and the start takes no labels or `--allow`. Under the ID it printed, it is shown only the call's result: the run's status and its output or error, and the result's files. A cancel of that ID is refused, naming the run the call started and its tree's client, which alone cancels it.

A command that sends a message waits for its receipt, and `run start --wait` and `--watch` for the run to be taken, while a controller may still take the message: for the message's life, five minutes by default or the time given with `--life`, and twice the clock allowance. When neither has come after two seconds the command prints one line to standard error, `waiting up to 15m for a controller to take it`, and when neither comes at all it fails, saying the message was not taken up.

A secret's value is never on the command line. It is read only from the source the command names, and a value not found there is a mistake that sends nothing:

```
controller secret set NAME            typed at the terminal, not shown
controller secret set NAME --env      the environment variable NAME
controller secret set NAME -          standard input, to the end
```

`client secret set` reads it from the same three sources.

`--at <location>` sets a secret for the runs at that location and below. With no `--at`, `controller secret set` sets it at `/`, for every tree, and `client secret set` is a mistake. A client sets only where it could submit a tree, and its command prints `taken`, or the refusal and fails. A plain value is not secret, so it is given on the line (see secret.md):

```
corbie controller value set DB_HOST db.prod.lan --at /prod
corbie controller value list
```

A file given alone is a tree of one file, at `--at`, by default `/` and the file's name without its extension. `run submit` reads its argument as a file or a directory, and `run start` as a location. `run show`, `run cancel` and `run send` read every argument that starts with `/` as a location.

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

Every command can print its result as JSON, in the one shape above. A command ends with success, with a mistake in how it was written, or with failure, which includes a run that did not end well, and ends the same way whichever it prints.

`--json`, `--config` and `--store` may be written before the command as well as after it. The config and the store may be named by environment variables. No other flag is read from the environment.

A command finds its config in this order:

```
--config <directory>         the directory named
CORBIE_CONFIG                the directory named
.corbie                      in the current directory, or the nearest directory above it; corbie up makes it
the user's config directory  ~/.config/corbie on Linux
```

The commands, their help, the manual's pages about them and the names by which a model is allowed to run them all come from one table.

`corbie <word>`, where the word begins no command, runs the program `corbie-<word>` from PATH with the rest of the line, this process's standard input, output, error and environment, and ends with the program's exit status. A command in the table always wins. A model through `corbie mcp` runs a program only for a command the server's `--allow` names.

When no program `corbie-<word>` is on PATH, `corbie` names the program it looked for, prints `corbie --help lists the commands.` and ends with status 2.

`corbie help` lists the programs `corbie <word>` runs by their names, and starts none of them. A command, the words that begin commands, and a program each print their help with `--help`.

A program written in Go reads its command line with the package's `cli.Program`, from a table of commands as Corbie's own is made. It then reads as one of Corbie's commands: the same `--help`, `--json`, `--config` and `--store`, flags anywhere among its arguments, and the same mistakes, failures and exit statuses. Its help says which of the three flags it takes. The command named `""` is the one run when no other is named. A program `corbie-board` with one command is written:

```go
os.Exit(cli.Program("board", []cli.Command{
	{Name: "new", Usage: "<board>", Summary: "make a board", Run: cmdNew},
}, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
```

```
$ corbie board new
corbie board new: name one board

usage: corbie board new <board>
```

The program is given `manual` and `version` beside its own commands, and no `mcp`. `version` prints the program's version and the version of the Corbie it is built with. Its manual is Corbie's, with a page for each of its commands and any pages it adds to the manual. A model runs its commands through `corbie mcp`, as For a model says:

```
corbie board manual                       Corbie's pages, the program's pages and its commands
corbie board manual board-cards           a page the program added
corbie mcp --allow "board new"            a model may run corbie board new, and no other command
```

```
$ corbie board version
corbie-board 0.7.0
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

`corbie start controller runner console` starts `corbie-console` beside the roles, once they are set up.

- `corbie start` starts the program with the words written after its name and an empty standard input, and what it prints is `corbie start`'s own output, with no prefix.
- Its environment is this process's, with `CORBIE_CONFIG` and `CORBIE_STORE` set as `--config` and `--store` were given.
- Beside a gateway in the same process, its environment has the gateway's address in `CORBIE_GATEWAY` and its first token in `CORBIE_GATEWAY_TOKEN`.
- When the program stops, `corbie start` stops the roles and the other programs, waits for them, and fails naming the program.
- When `corbie start` is stopped, it interrupts each program and waits for it, for up to ten seconds before killing it.

`ui` is a command of the binary, and `corbie start` takes it as it takes a program's name. It starts `corbie ui` as a process of its own, from this binary, with the words written after `ui`. A binary built with the tag `noui` has no `ui` command.
- On a serverless host, `corbie start` refuses a program before anything starts.

`corbie up` does what `corbie init`, `corbie start` and `corbie run submit` do, in one process:

1. It sets up a controller, a runner and a client in the config `.corbie` of the current directory, as `corbie init --ceiling all` does, with the store in `.corbie/store`, unless they are set up.
2. It runs the controller, the runner and a gateway.
3. It submits the directory, as `corbie run submit` does, at `/` and the directory's name.
4. It starts each program named, as `corbie start` does.

```
corbie up                          the machines of the current directory
corbie up ui console               and the UI, and the program corbie-console
corbie up --in boards --at /b      another directory, at another prefix
corbie up --store s3://…           a bucket as the store, when .corbie is set up
```

- Every command run in the same directory, or below it, finds `.corbie` and uses it, as it uses any config.
- `.corbie` stays when `corbie up` stops, and `corbie up` run again continues with its runs, secrets and values. Removing `.corbie` removes all of it.
- `.corbie` holds a `.gitignore` that lists everything in it.
- On a serverless host the config is `.corbie` in the copy's temporary directory, since a function's own directory is read-only (see Roles).
- A file with a problem is reported before anything starts, and nothing is started.
- The gateway listens at a free port, and `gateway --listen` names another. A program is given `CORBIE_CONFIG`, `CORBIE_GATEWAY` and `CORBIE_GATEWAY_TOKEN`.
- `--value`, `--value-env`, `--secret` and `--secret-env` set values and secrets at `/` as it starts, as `corbie test` takes them.
- `--store` and `--ceiling` apply when the config is set up. `--config` names another config than `.corbie`.

`corbie test` is an ordinary run: the same roles, in one process, on a store in memory, with ceilings that allow everything. It records the file as a tree of one file and starts it. A singleton is started by that submit, so a flag of a start given with one is a mistake. A keyed file is refused: its runs start when an event is sent to a key. `corbie test <file> --in <dir>` records the directory as `corbie run submit <dir>` does, which starts its singletons, and runs the file at its location there, so its `call:` and `send:` reach the tree's machines. `corbie test <file> --definition` prints the machine as the client writes it out, each value with its file and line, runs nothing, and takes no flag of a start.

`corbie test` sets values and secrets at `/` of the controller in its process, read from the sources `controller value set` and `controller secret set` read:

```
--value NAME=VALUE    a value
--value-env NAME      a value, read from the environment variable of its name
--secret NAME         a secret, typed at the terminal, not shown
--secret-env NAME     a secret, from the environment variable NAME
```

## A tree

`corbie run submit <dir> --at <prefix>` records a directory as a tree. Each `*.yaml` in it is a machine at a location: its path under the prefix, without `.yaml`. A file the directory's `.corbieignore` lists has no location. `--at` defaults to `/` and the directory's own name, and `.` uses the directory's real name.

```yaml
# house/.corbieignore
shared/

# house/devices/boiler.yaml
statechart: boiler
lifetime: singleton
initial: idle
states:
  idle:
    on: { set: applying }
  applying:
    job: { include: shared/boiler-job.yaml }
    on: { done: idle, failed: idle }

# house/shared/boiler-job.yaml            listed: no location
script: ./boiler.js
input: { target: "${event.data.target}" }
allow: ["secret:BOILER_PASSWORD@net:boiler.lan:443"]

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

$ corbie run start /home/schedules/away --wait
```

A submit does this, and exits when each run it cancelled has ended and each start has been taken. The controller's start of a singleton is good until it is taken, so the submit waits for it with no limit:

1. It reads every `*.yaml` in the directory and below it, but for `.git`, `.corbie` and what `.corbieignore` lists, and writes out and checks each machine.
2. It records every file at its location in the store, and removes the locations whose files have gone, cancelling every run there.
3. For a changed file, the runs already running at its location do as its `update:` says.
4. It starts each singleton with no running run, and nothing else.
5. It prints the names of values and secrets the files use that are not set at their locations or above. This is a reminder, not a refusal.

`.corbieignore`, at the top of the directory, lists what in it is no machine. Only the one at the top is read.

- Each line is a path from the directory. A directory, with or without `/` at its end, lists everything below it.
- A line starting with `#` and an empty line are passed over.
- A line that names the directory itself, such as `.`, or has a `..` segment, a `/` at its start, a `\` or a control character fails the submit, naming the line. A line naming nothing in the directory is no fault.
- There are no patterns, so a line holding `*`, `?` or `[`, or starting with `!`, fails the submit too.
- A listed file has no location: nothing runs there, and a send or a call to its path is to no location. A machine still includes it, runs it with `machine:` and reads it with `files:`.
- A file listed after a submit is gone from the tree, so the next submit removes its location and cancels its runs.
- A `*.yaml` that is no machine, such as `.github/workflows/test.yaml` or `docker-compose.yaml`, is a file with a problem until it is listed. Its problem gives the line to add.
- A file given alone, to `corbie run submit <file>` or `corbie test <file>`, is a tree of its own, and no list applies.

A tree belongs to the client that first submitted it. Every run keeps the definition it started with, and a submit changes no other tree.

`corbie run remove <prefix>` removes a tree, and exits when each run it cancelled has ended.

```
$ corbie run remove /home
/home/devices/boiler   singleton   removed
/home/schedules/away   transient   removed
cancelled: <id>
removed: secret BOILER_PASSWORD at /home/devices/boiler
```

1. The controller cancels every run at the tree's locations.
2. It removes every location of the tree.
3. It removes every secret and value set at the prefix or under it.
4. It removes the tree. The prefix then holds no tree, and a client whose ceiling matches it submits there.

- A secret or a value set above the prefix stays.
- A run that has ended is kept for the time its definition gives.
- Where the prefix holds no tree, the command removes the secrets and values set at it or under it.
- A prefix inside or above a tree, and another client's tree, are refused as a submit there is.

The owner removes any client's tree, also one whose client was removed:

```
$ corbie controller tree remove /home
the removal is in the controller's inbox; the controller acts on it at its next pass
```

- The command fails when no tree is recorded at the prefix.
- It does not wait, as `corbie controller cancel` does not.

- A submit at a prefix inside or above another tree is refused, naming that tree. `/` is never a tree.
- A submit at a tree another client owns is refused, naming the owner.
- A submit at a prefix that no `at:` entry of the client's ceiling matches is refused.
- A file under a directory whose name starts with a dot is read as any other: `.work/review.yaml` submitted at `/project` is at `/project/.work/review`.
- A file beside a directory of the same name, as `boiler.yaml` beside `boiler/x.yaml`, is refused with the files below it, because a location has nothing below it. They are reported among the files with problems.
- A file with a problem is reported, and its location keeps what was recorded before. A problem with one file leaves the rest done. In JSON, `problems` gives each problem's text by location.
- A file whose approvals have no ask is recorded, since it may run as another machine's child under that machine's ask. A run started at its location is refused.
- A singleton is given no input. One whose `input:` needs some is refused, and reported. It reads the context's absolute path as `${run.context}`.
- When a client submits a different directory at a prefix it holds, the submit prints `/home: last submitted from /srv/old-house`.
- `corbie run cancel <location>` after a submit cancels the runs there, so the next of each starts on the new definition.
- `corbie test` given directories reads their machines as this command does, prints each one ok or its problems and each listed path, and starts and cancels nothing. It reads no config and no store, so it changes nothing wherever it runs.
- It reads each directory as a tree at / and the directory's name, as a submit with no `--at` does, and checks each send and call whose `at:` is written out and is in the tree as the controller would: that the lifetime of the file there fits the address, that its `receive:` lists the sender, that the event is not one of the file's own, such as a timer's name, and that the sender's own `allow:` and `deny:` hold `send:` for it. A machine is checked with the machines it starts, which send from its location, narrowed by the lists of the jobs that start them. A list with an expression in it is left out, as only a run fills it in. What fails is a problem of the sender's file.
- A location of the tree that a send or a call names and that has no file is printed with where it is named, and fails nothing: the send may be one a condition leaves out.

## The gateway

The gateway offers the package over HTTP for applications not written in Go. Its routes do everything a client does: submit a tree, remove a tree, start a run of a location with an input, list, show and cancel runs, read a run's record, one step of it, its definition, its tickets and its files, send events and answers to a run, send an event to a location or to a key at one, and set, remove and list secrets and values. One route says which client and principal the caller's token names. A listing and a record are each given as they stand, and with `?watch` as a stream that follows them. A stream the gateway closes ends with a message: `end` with the run when the run has ended, or `error` with the failure when the gateway could not read the store. A start is JSON, or an upload form when its input has files, which come out by their path in a result.

A submit is an upload form: the prefix, and each file of the directory by its path from the directory.

1. The gateway writes the files into a directory of its own computer.
2. It reads them as `corbie run submit` reads a directory, and sends the submit as the caller's client.
3. It answers with the receipt, and what is wrong with each file that is no machine, by its location.
4. The directory is gone when it answers.

A start is the caller's client starting a transient location, as `corbie run start <location>` does. The location's file lists `client:<that client>` in `receive:`, or that client owns the tree. A start in a tree another client owns is a call, and the run is shown from the call's result. A route names a run by its ID. A listing narrowed with `?at=<location>`, and `&key=<key>` at a keyed location, gives the IDs of the runs there.

A gateway is started with `corbie start gateway`, alone or beside a controller and a runner. A request with a field the gateway does not know is refused.

A gateway acts as one or more clients, and each of its tokens names one of them:

1. A caller shows a token.
2. The token names a client and the principal the token was given to.
3. The gateway sends the request as that client, and writes the principal into each message it sends.
4. The controller checks the message as it checks any client's, and writes the principal into the cause (see instance.md).

```
corbie start gateway                                                       this config's client, and a new token
corbie start gateway --client viewer --client editor                       the clients of two configs, and a new token for each
corbie start gateway --client viewer --client editor --token-file tokens   the same clients, and the file's tokens
```

`--client <config>` names a config whose client the gateway acts as, and is written once for each client. With none, the gateway acts as the client of its own config.

`--token-file <file>` names a file with a line for each token: the token, the client's name, and the principal the token was given to.

```
179bd50585d0   editor   alice@example.com
93ab77c0e1f2   editor   bob@example.com
5c11d2a09b3e   viewer
```

- A line with no principal gives its requests none.
- An empty line and a line that starts with `#` are passed over.
- A line with a token alone names the client of a gateway that has one client.
- With no token file, the gateway makes a token for each client and prints it, and no request has a principal.
- A token on two lines stops the gateway from starting, and so does a line whose client the gateway does not have. Each names the line.
- The gateway reads the file when it starts, and says how many tokens it gave each client, and for which principals.

A gateway also takes the tokens of an OIDC issuer, such as Microsoft Entra ID:

```
corbie start gateway --client editor --client viewer --oidc-issuer https://login.example.com/tenant/v2.0 --oidc-audience api://corbie
```

1. A caller shows a token the issuer signed.
2. The gateway checks the signature with the keys the issuer publishes, and that the token names the issuer and the audience, and that its time has come and has not passed.
3. The token's `roles` name the client: the first of the gateway's clients, in the order of `--client`, whose name is among them. The client that may do most is written first.
4. The token's `sub` is the principal.

| Flag | Default | |
|---|---|---|
| `--oidc-issuer <address>` | none | the issuer, as its tokens name it: `https`, or `http` on this computer |
| `--oidc-audience <text>` | none | who a token must be for; given with the issuer |
| `--oidc-client-claim <claim>` | `roles` | the claim that names the client: a text, or a list of texts |
| `--oidc-principal-claim <claim>` | `sub` | the claim that is the principal |

- The gateway reads the issuer's keys when it starts, from the address the issuer's `/.well-known/openid-configuration` gives. An issuer it cannot read, or that names itself otherwise, stops it from starting.
- A token signed with a key the gateway does not have makes it read the keys again, once a minute at most.
- A token signed with a key the gateway has makes it read the keys again when it last read them an hour ago or more. A key the issuer no longer publishes is then refused.
- A token is signed with RS256 by an RSA key of 2048 bits or more, or with ES256 by a key on the curve P-256. The key sets the algorithm, and a token that names another is refused.
- A token that is not taken is answered 401, saying why, with what the token has and what the gateway takes. The gateway prints nothing for a refused request: the caller is told.
- The gateway says when it starts which issuer and audience it takes, the order of its clients and the two claims.
- With an issuer and no token file, the gateway makes no token of its own.
- The gateway has no page to sign in at. A caller gets its token from the issuer.

A caller asks which client and principal its token names, and `corbie client show --gateway <address>` prints them:

```
client     editor
principal  "alice@example.com"
```

One request starts a run and is answered with the run:

- `POST /runs?wait` starts the run and answers when it has ended, with the run, as `GET /runs/<id>?wait` does. `?wait=30s` answers after 30 s at most.
- A `wait` that is no length of time is answered 400, and no run is started.

`?files` puts the bytes of each blob of the run's output in the answer:

- A blob is then `{ "$blob": "<path>", "type", "size", "data": "<base64>" }`. Without `?files` it has no `data`, and `GET /runs/<id>/files/<path>` gives its bytes.
- `POST /runs?wait&files` and `GET /runs/<id>?files` take it. On a start `?files` needs `?wait`: alone it is answered 400, and no run is started.
- A blob written out inside a text has its `data` there too.
- A value that has a `$blob` key and is not a blob is unchanged.
- The gateway gives at most 64 MiB of blobs in one answer, counted together. Past that the run is answered with no `data`, and its `withheld` says why and where each blob is read.

A page in a browser calls the gateway from any origin:

- The gateway answers the `OPTIONS` request a browser sends before a page's call, with no token.
- Every answer permits a page of any origin to read it.
- A page shows its token in `Authorization`, as every caller does. The gateway reads no cookie, so a page without a token is refused.
- A page reads a stream with `fetch`. A browser's `EventSource` sends no `Authorization`, and is refused.

Whoever has a token is the client it names. Nothing in a request raises that client's ceiling, names another client or names a principal. The gateway's computer holds the key of every client it acts as.

It keeps nothing itself, so it runs as a process that listens, or on a serverless host as a function of its own:

1. The host calls the function once for each request that comes to the function's URL.
2. The gateway answers the request as it answers any, as the client its token names.
3. The host sends the answer on as the gateway writes it, so a stream is a stream there too.

- A request's time ends ten seconds before the host's limit for a call. `?wait` then answers with the run as it stands, a stream closes, and a request that waits for the controller is answered 504, saying its time ended.
- On a serverless host a stream goes on until its run ends or the request's time does, also after its caller has left.
- The gateway takes its tokens from `--token-file` or from an issuer. With neither it does not start there: each copy of the function would make a token of its own.
- `corbie up` there runs the gateway in one function with the controller and the runner (see Roles).
- It is started beside no controller and no runner there: the host calls those for a pass, and the gateway for a request.
- The host limits the size of a request, and so of the tree a submit uploads.

## Through a gateway

The package sends to a store or to a gateway:

| A client made with | Does each thing |
|---|---|
| `client.New`, from an identity, a controller's card and a store | on the store, signed and sealed with the client's own key |
| `client.Remote`, from a gateway's address and a token | as one request to the gateway, which does it as the token's client |

A command a client sends takes `--gateway <address>`, and sends the token in `CORBIE_GATEWAY_TOKEN`:

```
export CORBIE_GATEWAY_TOKEN=179bd50585d0
corbie run submit house --gateway https://corbie.example.com
```

- `corbie run …`, `corbie client show`, `corbie client secret …` and `corbie client value …` take `--gateway`. Any other command refuses it.
- The command uses no config and no store, and holds no key.
- A command prints the same through a gateway as with the client's own config.
- A submit sends each file the directory's machines read: the machines, their scripts, the files they include and `.corbieignore`.
- A file given to a start is a value of the input's top level.
- The gateway names the run a start starts.

## For a model

`corbie mcp` serves two tools to a model: the manual, and the command line. A command runs with the config the server was started on. The server needs nothing running and keeps nothing.

```
corbie mcp                                        the model may run every command of Corbie's
corbie mcp --allow "run ls" --allow "run show"    the model may run those two only
```

`--allow` names a command by its whole name. A host that gives the model a shell lets it run every command there, so `--allow` limits a model only where the host gives it no shell and the commands named run no script.

`--allow` may name a program's command, as in `--allow "board new"`. When the server starts, it runs `corbie board manual --json` once:

1. The program's pages join the manual, and its commands' pages name its commands.
2. The model is told the program's page named after it, `board`, after Corbie's briefing.
3. The model runs `board new` with the command tool, as `corbie board new` runs it, with the server's config and store, and gets back what the program printed.

A model never runs a program the server's line does not name, and `corbie start` never starts a program for a model.

Each call runs by itself. A call that stays until a run ends keeps no other call from being answered, and it ends when the model's host stops it.
