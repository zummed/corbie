# Machines

A machine is a definition that Corbie runs. There are three types, flow, statechart and agent, and each has its own area. This area is what they all share: how a machine runs, how a definition is written, where it lives, and what a machine can receive and ask for.

## Steps

A machine runs as a series of steps and never as one long process. A step is given the definition, the state the machine was left in, and the events that have arrived since. It returns the new state and a list of requests. That is all a step does. The work it asks for is done by its jobs, which are scripts and other machines. Steps of one instance happen one at a time and in order.

Because a step depends on nothing but what it is given, taking the same step twice gives the same answer, and everything a machine has ever done can be read back from its record.

A machine runs until it reaches an end of its own, is cancelled, or passes its timeout if it has one. One with no timeout may run for ever. Waiting costs nothing: a machine that is waiting is just a record in the store.

## Definitions

A definition is data, written in YAML. Exactly one of its fields is `flow:`, `statechart:` or `agent:`, which gives its type and its name. That field may be written anywhere in the mapping, or come from a file the definition includes, and a definition with none or with two is refused. It may give the shape of its input, how long its instances are kept, and its limits.

A definition may carry `meta:`, with any value. Corbie keeps it with the stored definition, never reads it, and fills in no expression in it. An application keeps its own data there, such as when to start the machine.

A definition may give `description:`, plain text that says what the machine is for. Corbie keeps it with the stored definition, and refuses an expression in it.

A definition may give `title:`, which names a run to a person. Its expressions read `input`, `values` and `run`, and the controller fills it in when the run starts and keeps it in the run's record, never in its head, so only the client that owns the run's tree sees it. A title is one line of at most 200 characters: each run of white space, line breaks among it, becomes one space, and a longer title is cut.

```yaml
title: "${input.channel}: ${input.said}"     # a run's title: "#testing: give me a release card for 4.2"
```

A title that cannot be filled in fails the run at its first step, as an input that does not fit does.

Definitions are meant to be written by people and by models, so each type offers short forms where they read better. Each short form has one meaning and one place it can be written. When a client sends a definition, every short form is filled out into one full form, and only the full form is stored and run. The same filling-out is how a type offers a convenience over something Corbie already does: where a feature can be expressed as a short form over an existing mechanism it is, and the machine that runs never learns of it.

What a definition may hold is said once. Each mapping in it has a shape: the definition itself, a job, an agent's model, a state. A shape is the mapping's fields, with the kind of value each takes and what each is for.

```
 a model      name     a word     the model's name, as its service knows it        any name; these are known: …
              effort   a word     how much effort the model is to take             one of its service's: low, medium, high
              reply    a number   the most a reply may hold, in tokens
```

A definition is read by its shapes and by nothing else. A field is read only if its shape names it, as the kind of value its shape gives it, and a field its shape names is always read. A client is given the same shapes, to offer a definition's fields to a person, so what it offers is exactly what will be read. Where a field takes one of a few values the shape gives them, and where the values depend on a field beside it, such as the efforts a model's service takes, it gives them for each.

A mapping may be one of several sorts of thing that hold different fields. Its shape then says which field tells the sorts apart, and which fields each sort holds.

```
 a model, by its service     anthropic, openai     name  secret  service  effort  address  reply_tokens
                             claude-code           name  secret  service  effort  own
```

A field its sort does not hold is not offered to a person, and is refused if it is written.

A definition may take values from other files with `include:`, which names a file or a list of them. A name may end in `#path` to take one part of a file: `base.yaml#allow`, `base.yaml#tools.read_file`.

```yaml
# family.yaml
agent: family
include: ./base.yaml
model: { effort: high }
system:
  - { include: ./style.md }
  - You look after one house.
allow:
  - { include: ./base.yaml#allow }
  - send:devices/*
toolkits:
  home: { include: toolkits/home.yaml, approve: true }
```

```yaml
# base.yaml
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.said}
allow: ["fs.read:${input.dir}/**"]
```

The client writes the definition out before it sends it:

1. It reads each file the definition names. A file holds any fields. One may give the type, which the definition takes, and a definition that writes a type too writes the same one. `base.yaml` gives none, so in a tree it is listed in `.corbieignore` (see Locations).
2. In a mapping, it merges in the included fields at every depth. `family.yaml`'s `model:` gets `name` and `secret` from `base.yaml`, and keeps its own `effort`.
3. In a list or a text, it replaces an item `{ include: <file> }` with that file's list or text, and `{ include: <file>#<path> }` with the value at that path.
4. It sends the result, which holds no `include:` and keeps each value's file and line.

- A value the including file writes wins over an included one, at every depth. A file's values win over those of the files it includes, and an earlier file's over a later one's. A file merged in whole by two routes is merged once; a `#path` item is always replaced.
- A list or a text the including file writes replaces the included one whole. `family.yaml`'s `allow:` replaces `base.yaml`'s, so it takes the base's items back with `#allow`.
- Nothing an include gives can be taken away. A file takes only part of another by naming each part with `#path`: `tools: { read: { include: ./tools.yaml#read } }` takes one tool, and a tool added to `tools.yaml` later does not reach it.
- A text may be a list of pieces, each a text or `{ include: … }`, joined by one empty line. A single `{ include: file.md }` in a text field is a text of one piece.
- `${…}` in an included text is an expression, filled in at run time like any text. A literal `${` is written `${'${'}`.
- An included file is part of the machine that includes it. Its expressions read that machine's `input`, `values` and `run`, its secret names are looked up at that machine's location, and that machine's `allow:`, `deny:` and `approve:` cover its jobs.
- The stored definition is the one written out, so changing a file changes nothing for a run already started.

`corbie test family.yaml --definition` prints the machine written out, each value with its file and line. Part of it:

```
model:
  name: claude-sonnet-5-5                    # base.yaml:1
  secret: ANTHROPIC_KEY                      # base.yaml:1
  effort: high                               # family.yaml:3
allow:
  - fs.read:${input.dir}/**                  # base.yaml:3
  - send:devices/*                           # family.yaml:9
```

- A file is read as a definition is: a number is a number, and a value YAML would read as a date stays the text written. An empty file is refused.
- A path in a file reads as any path does, and `./` from that file's own directory. What a shape gives as a value, such as `input:` or `meta:`, stays as it is written, whatever it holds.
- A problem in an included file names that file: `toolkits/home.yaml: toolkits.home.colour: …`.
- More than 1,000 includes written out by one definition, and what is below it, are refused. A file reached by two routes counts each time, though its values are merged once.

A definition is checked as a whole, and everything wrong with it is reported together. Each problem names its place, and its file when the problem is in a child's file, as a path from the definition's directory: `kids/kid.yaml: value: …`. The client checks it before sending, so mistakes are found at once, and the controller checks it again with the same code, because it does not take a client's word.

An instance never changes its definition. A machine that wants to change how it works changes a child's definition and starts the child again.

## Locations

A client submits a directory at a prefix, and the directory becomes a tree (see client.md). Each `*.yaml` in it is a machine with a location: its path under the prefix, without `.yaml`. A file the tree's `.corbieignore` lists has none: nothing runs there and nothing can be sent to it. A machine still includes it, runs it as a child and reads it with `files:`.

`lifetime:` says how runs start at a location:

| `lifetime:` | Runs at the location | Started by |
|---|---|---|
| `singleton` | one | the submit |
| `keyed` | one per key, statecharts only | a send to a key with no running run (see statechart.md) |
| `transient` | any number; the default | a call, or `corbie run submit <location>` by the tree's client |

```
$ corbie run submit house --at /home
/home/devices/boiler     singleton   started <id>
/home/rooms/thermostat   keyed       recorded
/home/services/weather   transient   recorded
```

The submit starts the boiler's run. The thermostat starts a run for each room a send names, and the weather service one for each call. `house/.corbieignore` lists `shared/`, so `house/shared/base.yaml` has no location and is not printed.

Every run has a location, and its authority comes from it: its file's `allow:` and `ask:`, its tree client's ceiling, and the secrets and values set at its location or above (see permission.md and secret.md).

- A child started by `machine:` runs at its parent's location.
- A machine made while running has no file of its own. It runs at the location of the job that starts it, narrowed by that job's list.
- `limits: { runs: N }` on a keyed file caps how many of its runs are running at once (see statechart.md). N is at least 1, and `runs` on a file that is not keyed is refused.
- `lifetime:`, `update:`, `receive:` and `limits: { runs: }` belong to a file at a location, and are refused in a machine written in place or made while running.

A file of one line places a machine that a listed file holds at its own location:

```yaml
# project/.corbieignore
.rook/

# project/.rook/slack/chat/post.yaml        listed: no location
flow: post
input: { in: string, text: string }
receive: [gromit/gromit]
stages:
  post: { job: { script: ./post.js, input: "${input}", allow: [net:slack.com] } }

# project/gromit/chat/post.yaml
include: .rook/slack/chat/post.yaml

# project/ops/chat/post.yaml
include: .rook/slack/chat/post.yaml
receive: [ops/ops]
```

```
$ corbie run submit project --at /x
/x/gromit/chat/post   transient   recorded
/x/ops/chat/post      transient   recorded
```

- A placed file takes its type, its name and every field from the file it includes, `lifetime:` and `receive:` among them. Placing a singleton gives a singleton.
- A field the placed file writes wins: `/x/ops/chat/post` receives only from `ops/ops`.
- Two placements of one file are two locations, and the runs of both are named `post`.
- A `./` path in the included file is read from that file's directory, so both run `.rook/slack/chat/post.js`.
- A change to the included file changes every placement, and the next submit acts on each as on any changed file.

A run keeps the definition it started with, and a submit changes what new runs get. `update:` says what a changed file does to the runs already running at its location:

| `update:` | Runs already running |
|---|---|
| `restart` | are cancelled; a singleton starts again on the new definition, and a keyed run starts fresh on the next send to its key |
| `drain` | finish on the definition they started with; the default for keyed and transient |

- A file is changed when the machine written out from it differs. Where its values are written takes no part, so moving a line changes nothing.
- A singleton always restarts, and `update: drain` on one is refused.
- Removing a file from a tree, by deleting or renaming it and submitting, cancels every run at its location.
- After a submit, `corbie run cancel <location>` ends a `drain` file's runs, so the next ones start on the new definition.

## Values

Any value in a definition may be an expression, written `${…}`. A value that is exactly one expression takes the expression's value, whatever its type. Text with expressions inside it is text with the values filled in. Everything else is literal.

A condition, such as `when:`, is exactly one expression. It holds unless its value is false, null, zero or empty text.

Expressions are JavaScript. They read `input`, which is what the machine was started with, `values`, the plain values set at the run's location or above (see secret.md), `run`, which is the run they are in, and whatever its type adds: a flow's stages, a statechart's states and current event, an agent tool's arguments.

```
  run.id         the run's ID
  run.location   the run's location, such as /home/devices/boiler
  run.context    the absolute path of the directory last submitted at the run's tree
  run.tree       the prefix of the run's tree, such as /home
```

Everything else that varies from one run of a definition to the next comes in through `input`, and from one tree to the next through `values`.

A value is read by its name, as `values.HUB`. `values[name]` and `values` alone are refused, so the values a file reads can be listed. An expression that reads a value not set at the run's location or above fails, naming the value and the location.

Some fields are names, not values, and an expression in them is refused: a thing's name, a type, a permission's kind, a deny entry, a secret's name, a tag, a ticket's name. The path of a script or a machine is a name too, with one exception: it may be exactly one expression that gives a stored definition or script.

An expression may run for one second. That limit stops its own code, and not a call to something built in that is under way; a step of a machine made while running has limits that stop everything (see Controller). The time it sees is the time of the step, and random numbers differ from one expression to the next but are the same if the step is taken again.

## Shapes

A shape is a JSON Schema of an object, 2020-12 unless its `$schema` names another dialect. A definition's `input:`, a tool's `args:` and an agent's `reply:` are shapes. A definition writes one in full, or in a short form that gives each name a type:

```yaml
input: { urls: list, depth: number? }
```

```
  string, number, boolean   that JSON type
  list, map                 an array, an object
  blob                      a blob: in input:, the blob; in args: and reply:, the name the model was shown
  any                       any value
  ? after the type          the name may be left out
```

A shape with `type: object` is the full form, and any other is the short form. The client fills out the short form when the definition is submitted. The short form of `args:` and `reply:` refuses a name it does not give, and that of `input:` takes it.

A `$ref` resolves only within its own shape. One that names a file or an address is refused when the definition is checked.

A shape may lead back to itself, and so describe a tree:

```yaml
args:
  type: object
  properties:
    name: { type: string }
    kids: { type: array, items: { $ref: "#" } }
```

A `$ref` that leads back with no part of the value between has no end, and a value checked against it fails.

An error names each place in a value that does not fit.

## Input

A definition's `input:` is the shape of what an instance is started with. A definition without one takes any input.

1. The controller fills in each missing name that has a `default`, in the input and in each object that `properties` and `items` lead to. A default filled in is filled the same way.
2. The controller checks the filled input against `input:`.
3. An input that does not fit fails the instance at its first step, naming each place, and nothing starts.
4. The filled input is the data of the `start` event, and what every expression reads as `input`.

```yaml
input:
  type: object
  properties:
    urls:  { type: array }
    depth: { type: number, default: 2 }
    fetch: { type: object, default: {}, properties: { retries: { type: number, default: 3 } } }
  required: [urls]
```

```
  started with                         input reads
  { urls: [a] }                        { urls: [a], depth: 2, fetch: { retries: 3 } }
  { urls: [a], fetch: { wait: 5 } }    { urls: [a], depth: 2, fetch: { wait: 5, retries: 3 } }
  { depth: 2 }                         the instance fails: missing property 'urls'
```

A `default` that `properties` and `items` do not lead to from the top, such as one under `anyOf` or behind a `$ref`, is refused when the definition is checked, and so is a default that does not fit its own place. An entry of `$defs` that no `$ref` names is not read, and neither is a default in it.

## Jobs

A machine contains jobs, which do its work. A job is a script, which a runner executes, or a machine:

| Job | What runs |
|---|---|
| `script: ./fetch.js` | the script, on a runner |
| `machine: ./child.yaml` | the machine of that file, as a child run |
| `call: { at: services/weather }` | the machine at that location, as a run of its own whose result comes back (see Sending and calling) |
| `send: { to, event, data }` | nothing: the event is delivered to a run, and the job gives back nothing |

A script or a machine has an input, the permissions it may use, a limit on time, and optionally a restart policy. A script also has a limit on memory and tags that choose which runners may take it.

A program using Corbie as a library can list a definition's jobs, each with its place, such as `stages.a.job`.

A `timeout:` with an expression is filled in when its job is asked for, and `limits: { timeout: }` when the run starts; a filled value that is not a length of time fails the machine.

A job may be written where it is used or kept in its own file. A script is a file named by its path, and its job lists the files the script loads; a client sends them along with the definition.

The context is the directory a client submits as a tree. For a single file it is the directory the client submits from, or the file's own directory for a file outside it. Every file a definition names is in it or below it, and nothing outside it is read. A machine made while running has the files it was given as its context.

| A path written as | Is read from |
|---|---|
| `toolkits/net.yaml` | the context |
| `./tools/pve.js` | the directory of the file it is written in |

`../` and a path from the top of the disk are refused in a definition. A script that loads another with `require` names it from its own directory, with `./` or `../`, since a bare name there is a plugin.

No file leads back to itself. A machine file or an included file that names a file already being read is refused, with machines written in place between them as well, and the refusal names the chain: `b.yaml: toolkits.s.include: the definition leads back to itself: s.yaml -> b.yaml -> s.yaml`.

A machine as it is stored is at most 4 MiB, as much as a step may read of one stored definition, with a reference for each of its files and child definitions. One past it is refused at its place, by the client and by a job's `define`, and nothing is stored.

A YAML alias may not contain itself, and a file whose aliases make more than about 100 times what it writes, past 1000 values, is refused. That is the YAML reader's own check. Nothing else counts what a definition reads: what reading one costs falls on the process that reads it (see Permissions).

Every job has a name that says what asked for it: a stage, a state, a tool, an agent's `turn`, the `ask`. A job's name has a suffix when other jobs have the same name:

```
get_title.0   one of several jobs asked for in one step
turn#2        the second time the name asked for a job in the run
get_title@2   the second attempt
```

- An item of a list adds its position from 0: `get_title.0`, `get_title.1`.
- A tool called twice in one agent turn has jobs named `shelf` and `shelf.1`, and a second preview before one turn is `preview.1`.

The record and every watcher use these names.

## Events

Everything that reaches a machine is an event, and every event has the same shape: a name, where it came from, and its data. A machine is started by a `start` event carrying its input. A job that ends well arrives as `done`, and its data is its output. One that does not arrives as `failed`, and its data is its error together with whatever output it still gave. A timer arrives under the timer's name. An answer to a ticket arrives under the ticket's name, and is marked as an answer, which no other event can be.

A job can also send events up while it is still going. A script does it by emitting a named value, and a child machine by sending an event to its parent. The two are the same thing and arrive the same way, as an event with that name from that job.

A parent can send a named event down to a child machine. Any other run is sent to by its address (see Sending and calling).

The client that owns a root's tree takes the place of its parent. It can send the root named events, and what the root sends up is written in its record, where the client and any watcher can see it.

Events from one run to another arrive once each, in the order they were sent. Nothing is promised between two senders. An event sent to a child that is not running is noted in the record as undelivered and fails nothing.

Each type handles an event in its own way, and its own area says which it acts on and which it ignores.

## Sending and calling

A machine sends an event to a run by its address, or calls a location and waits for the result.

| Address | Sends to |
|---|---|
| `{ at: devices/boiler }` | the run of a `singleton` location |
| `{ at: rooms/thermostat, key: kitchen }` | the run for that key at a `keyed` location, started if none is running |
| `{ at: devices/boiler, run: <ID> }` | one run by its ID, a child run included |

```yaml
# a statechart's send action
send: { to: { at: rooms/thermostat, key: "${event.data.room}" }, event: set, data: { celsius: 21 } }
send: { to: "${event.from}", event: applied, data: "${event.data}" }    # a reply to the sender
# a job of a flow stage or an agent tool
job: { send: { to: { at: devices/boiler }, event: set, data: { target: 12 } } }
job: { call: { at: services/weather }, input: { days: 2 } }
```

- A location in an address, a `send:` entry or a `receive:` entry is read from the tree's prefix, unless it starts with `/`, which is a full path into any tree. A name in it is not empty, `.`, `..` or `+`, and holds no `#`, backslash or control character.
- `job: { send: … }` ends `done`, with no output, once the message is written. An event too large to write, past 256 KiB, fails the job, as any job's message does. Beside `send:` it takes `approve:`, `allow:` and `deny:`, and no other field of a job.
- `at`, `key` and `run` may hold expressions, and `to` may be one expression that gives an address. An address is checked once it is filled in.
- `job: { call: { at }, input }` starts a run of a `transient` location and waits. The job's output is that run's result. The job fails if the receiver refuses it, if the location is not `transient`, or if the run fails.
- A key is any text of 1 to 256 characters. It is written in the run's head, as a label is, so anyone who can list runs can read it, and it holds no secret.

A send or a call needs two lists to agree:

1. Every permission layer of the sender, its tree client's ceiling included, holds `send:<location pattern>` (see permission.md). A tree's own client needs none for its own tree.
2. The receiver's `receive:` accepts the sender.

```yaml
receive: [thermostats/*, client:panel]                                # for every event
receive: { set: [thermostats/*, client:panel], stop: [client:panel] }  # by event name
```

- With no `receive:`, a file accepts from every location of its own tree and from its tree's client.
- A written `receive:` replaces that with exactly its list. The tree's own client is accepted whatever the list says.
- A call is checked against a list. A mapping names events, and takes a call only from the tree's client.
- An entry is a location pattern or `client:<name>`.

The controller writes the sender into `event.from`: `{ at, run }`, with `key` for a keyed run, or `{ client }`. A send to it is a reply, and an address that names both a run and a key goes to the run. Anything about the sender in the data is only the sender's claim.

| What happens to a send | Result |
|---|---|
| the sender's own layers refuse it | `denied` |
| the receiver's `receive:` refuses it | noted as undelivered in the sender's record |
| no running statechart can take it: it names a flow or an agent run, a run that has ended, or a key that is empty or past 256 characters | noted as undelivered |
| a keyed location is at its `limits: { runs: }` and no run has the key | noted as undelivered |
| a run kept it under `defer:` and was cancelled or restarted | noted as undelivered |
| it is named `start`, `done`, `failed`, `timer`, a timer's name or a ticket's name | noted as undelivered |

An undelivered send fails nothing, and the sender goes on. Its note is written in the sender's record even after the sender has ended.

A run started by a send or a call:

- It is a root of its own tree's client and no child of its sender. Ending the sender does not cancel it, but cancelling a calling job cancels the run it called.
- It counts one level below its sender, and fails as it starts if that passes its own file's depth limit. A run that fails as it starts, such as one given data that does not fit its input, adds the reason to the sender's record.
- Its authority is its location's. The sender adds nothing to it and takes nothing away, but the input it gives may fill an allow scope within the receiver's own patterns and every ceiling.
- Its held jobs go only to its own file's `ask:` (see Approval).
- It holds a copy of the blobs the event names, so it needs nothing of the sender's record.

A machine made while running sends only to its parent and its children. A send to an address or a call in it fails that machine.

## Children

Larger machines are built from smaller ones. A child is started by a job and its ending is one event to its parent. A child never outlives its parent: when a machine ends or is cancelled, every script job and every machine below it is cancelled too. A run started by a send or a call is a root and no child of its sender.

A definition can be made while a machine runs, by a script or by an agent, and run as a child. It is filled out, checked and stored exactly as one sent by a client is, in the process of the job that defines it (see Runner). It is checked again when a job starts it, as any definition is, and whether its approvals have an ask to go to, since that depends on what is above it. Each of its steps is taken in a process of its own (see Controller).

A root may have sixteen levels of machines below it unless it says otherwise, and an ask that is a machine uses a level like any child. A definition may also limit how many jobs an instance starts. A job past either limit fails and is not restarted.

## Waiting on the outside

A machine waits for a person or an outside system with a ticket: a random value that gives the right to answer one question, once.

1. A state issues the ticket, and the step makes its value.
2. The machine, or the client that owns the run's tree, hands the value to whoever should answer.
3. That person or system answers with any client, naming the instance and the ticket and giving a value.
4. The answer arrives as an event with the ticket's name. No other event is taken under that name.

The ticket is the authority: any client that holds it may answer, and Corbie does not check who did. What an answer says about who gave it is the answerer's word, which the machine's definition checks or does not. The receipt for an answer says whether it was the one taken.

Services that carry questions to people, such as a chat program, are clients at the edge and no part of Corbie.

## Approval

A job can be made to wait for a yes before it starts. A job is held if it is marked for approval, or if a permission it asks for is one its machine, or a machine above it, has listed in `approve:`. Being approved gives a job nothing it was not already allowed; approval only ever holds. What gives the yes is the machine's ask: a job of its own, a script or a machine, named at the top of the definition. That job is called `ask`, and nothing else in a definition may take the name. A machine with no ask uses the nearest one above it. A type may give some of its jobs an ask nearer than the machine's, as an agent's toolkit does, and that ask is the nearest above every machine below those jobs.

A machine made while running has no ask of its own. What it holds goes to the nearest ask above it that was not made while running, and with none the job is denied. An ask it writes, at the top of its definition or in an agent's toolkit, fails that machine as it starts, with the reason.

A run started by a send or a call has no machine above it, so its held jobs go only to its own file's `ask:`, and are denied if it has none. A call or a `job: { send }` is a job, and declares only `send:` to the location it names, so the sender's `approve: [send:…]` holds it when the list names that location. A statechart's send action is never held.

The controller does the holding. It gathers the jobs from one step that need approval, fills in each ask they go to once, and starts it once with them. The ask is told which machine is asking and, for each job, what it is and what input it would get. Its result, `{ approved: [names of jobs], note }`, names the jobs it approves and may add a note. Approved jobs start. A job not approved never starts, and its machine sees it as a failed job with the status denied, which it may handle like any other failure. An ask that cannot be filled in, that fails, or whose result does not list the jobs it approves, approves nothing, and the error of each job it denies says why.

Waiting for a person happens inside the ask, typically a statechart that issues a ticket and ends with the jobs it approves. An ask that waits for nobody, such as a model reviewing the jobs, is an ask all the same. An approval has no time limit of its own; the ask's timeout is how long it may take.

An ask's job takes input like any other. The permissions an ask needs are its own, used by the ask's job and by nothing else in a machine that inherits it.

## Failure

A job fails with an error, by running out of time, by being denied, or by being cancelled. A denial says what was refused, which layer refused it and what would fix it, and passes through a machine to its parent unchanged.

Nothing runs again unless its job says `restart:`. The policy is on the job and not on what it runs, so a definition says only what a machine does, and a machine is restarted by wrapping it in a job.

```yaml
job: { script: ./fetch.js, restart: { limit: 2 } }

job:
  machine: ./listen.yaml
  restart: { after: always, limit: 5 in 1h, delay: 1s to 5m }
```

| Field | What it says | Default |
|---|---|---|
| `after` | `failure` restarts after an error or a timeout, and `always` after the job ends well too | `failure` |
| `limit` | how many restarts: a number, or a number in a length of time | no limit |
| `delay` | the wait: `1s to 5m` doubles from the first to the second, and `30s` is the same each time | `1s to 5m` |

- The controller restarts a job with the input it was first given. A machine job restarts as a new child run, from its start.
- A denial and a cancel are never restarted, so a job cancelled by hand stays stopped.
- A delay that doubles starts again once an attempt has stayed up for twice its longest wait.
- The machine sees only the final outcome, which is the last attempt's once the limit is passed. The record shows each attempt.
- Every value an attempt emits is delivered, so a job that repeats itself delivers its values again.
- A job with `after: always` and no limit gives its machine no outcome, so a stage that waits for it never ends.
- A restart after a failure is the same job, under a yes it was already given, and is not counted against `limits: { jobs: }`.
- A restart after a job ended well is a new job. It is held again if it needs approval, has a new request name, and counts against `limits: { jobs: }`.
- An ask that ends well is final, whatever its policy says.

A machine reacts to a failure as to any event. A failure it has no rule for fails the machine, and its own failure is the event its parent receives. If a step itself fails, the instance fails.

## Machine types

A type is an extension like any other. It supplies the shape of its definitions, the filling-out of its short forms, the check of its definitions, its step, what a watcher is shown of its state, and its page of the manual. A type with asks of its own, or that takes shapes from outside as it runs, says so from a definition read as plain JSON. Everything in this area is supplied by Corbie, the same for every type, and no type has its own version of any of it.
