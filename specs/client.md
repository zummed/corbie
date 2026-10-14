# Client

A client starts runs and reads what happened to them. This area is the client as a Go package, and the three things built on it and on nothing else: the command line, the gateway and the server for models.

## The package

The client is a public Go package in the Corbie repository. An application written in Go uses it directly. Everything else Corbie offers a client goes through it, so there is one behaviour and one set of words for all of them.

It does two kinds of thing.

It **sends**: start a run, cancel one, send an event to one, answer a ticket. To start a run it takes a definition and the files beside it, fills out the short forms, checks the whole and reports everything wrong at once, uploads the files, and sends the start. A start may carry input, files as input, labels, run secrets, a list that narrows what the run may do, and a timeout.

It **reads**: list runs and follow the list as it changes; show one run, where it stands and how it ended; read a run's record so far, children included, and follow it as it grows; fetch a file from a result; show the definition a run was started from.

Reading is the one way of watching described under Instances. The package owns the awkward corners so that nothing built on it meets them: a head that is a moment behind its record, a run that has been sent and not yet taken up, a start that was refused, a receipt that arrives before the head does, a head that is moving from running to finished.

To a client a run is an instance: a root it started, or one below it.

## One shape for a run

Wherever a run is shown, it has the same shape and the same words.

A run is `waiting` to start, `running`, `done`, `failed` or `cancelled`. A failed run says why: an `error`, a `timeout`, or `denied` with the refusal. A finished run has its output, and a list of the permissions it used, which for a machine are those of every job in it and below it. A run waiting on a ticket shows the ticket's name. The ticket's value is in the record, which only the client that started the run can read.

A blob in any value is shown as its type, its size and the path to ask for it by. Its key is never shown, at the command line, over HTTP or to a model.

## The command line

```
corbie init                         create a controller, a runner and a client on this computer; --serve says where others reach its directory
corbie run controller runner        run the controller, a runner, the gateway or the store's server, until stopped
corbie test flow.yaml               run a definition or a script in this process, with no setup

corbie submit flow.yaml             start a run and print its ID
corbie ls                           list runs
corbie show <id>                    show one run: its status and its result
corbie cancel <id>                  cancel a run that this client started
corbie send <id> <event> [data]     send an event to a run that this client started
corbie reply <id> <ticket> [value]  answer a ticket that a run waits on
corbie check                        start one small run, and report which part of a setup did not respond

corbie controller …                 enrol and remove runners and clients, set ceilings and tags, manage secrets, cancel any run
corbie runner …                     print a card, trust a controller
corbie client …                     print a card, trust a controller

corbie manual                       print a page of the manual, or list the pages; --web serves them to a browser
corbie mcp                          serve the manual and the commands to a model, as an MCP server
corbie version                      print the version, the machine types and the plugins
```

`show` prints where a run stands. `show --record` prints what happened to it: each entry of its record so far, its children's included, and then the run as it stands.

```
$ corbie show <id>
<id>  running  statechart door  step 1
  waiting on ticket knock

$ corbie show <id> --record
15:56:21 4efcf50f63  step 1  <- start
<id>  running  statechart door  step 1
  waiting on ticket knock
```

Following from a terminal is one flag, `--watch`. It prints what the command prints without it, and keeps going:

- `ls --watch` prints the listing, then each run that appears or changes.
- `show --watch` prints the record, then each entry as it is written, until the run ends.

`show` can also print one step, one job's input and output, or the definition, can save a result's files to a directory, and with `--wait` stays until the run ends and prints its result. `submit` sends a start; `submit --wait` waits for the run it started and `submit --watch` follows it. `submit` and `test` take run secrets by name from the environment.

A script file can be submitted as it is. It is wrapped in a flow of one stage, and the permissions it may use and its limits are given on the command line.

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

`corbie test` is an ordinary run: the same roles, in one process, on a store in memory, with ceilings that allow everything.

## The gateway

The gateway offers the package over HTTP for applications not written in Go. It has a route for each thing the package does. A listing and a record are each given as they stand, and with `?watch` as a stream that follows them. Files go in as uploads and come out by their path in a result.

A gateway is started with `corbie run gateway`, alone or beside a controller and a runner. It is one client with one ceiling. Callers show it a token it was given when it was started, and all of them act as that client. Different ceilings need different gateways. A request with a field the gateway does not know is refused.

It keeps nothing itself. It runs wherever a process can stay up, and is not offered on serverless hosts.

## For a model

`corbie mcp` serves two tools to a model: the manual, and the command line. The model may run only the commands it was started with permission for, with the config the server was started on. It needs nothing running and keeps nothing.

Each call runs by itself. A call that stays until a run ends keeps no other call from being answered, and it ends when the model's host stops it.
