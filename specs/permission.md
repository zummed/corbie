# Permissions

A job can do nothing outside itself unless it is permitted. This area is how a permission is written, who can give and limit one, and how it is checked.

## Writing one

A permission is a kind and a scope: `fs.read:/srv/reports/**`, `net:api.github.com`, `exec:git`.

There are eight kinds.

| Kind | Allows | Scope |
|---|---|---|
| `fs.read` | reading files | a path |
| `fs.write` | writing files | a path |
| `net` | connecting to a host | a host, with a port if it matters |
| `exec` | starting a program | a program's name or path |
| `env` | reading a variable of the runner's | a name |
| `secret` | sending a secret somewhere | a secret's name and a destination |
| `send` | sending to or calling a location | a location |
| `at` | submitting a tree, in a client's ceiling only | a prefix, starting with `/` |

A scope is one of four things, matched the same way wherever it appears: a path, a host, a name, or a location. Any of them may use `*` for a part. A host with no port means every port, a host of `*` means every host, and a host written as an address, `10.0.0.*`, matches no name. A path is checked where it really points, after links are followed.

A location is a name in the store. Its parts are matched as a path's are, but no link is followed and case counts on every system. No part is empty, `.`, `..` or `+`, and none holds `#`; a tree's prefix and a location sent to hold no `*`.

In a definition a location is read from the tree's prefix by default, and as a full path when it starts with `/`; `../` is refused. In a ceiling or a start's list it starts with `/`.

On Windows:

- A path starts with its drive or share and is written with `/`, as `C:/Users/sam/**` or `//server/share/**`.
- A path with a leading `/` and no drive is on whichever drive is current, so as a scope it matches nothing, and a script that names it is refused.
- A path and a program's name match in any case, and a program's name matches with or without `.exe` or `.com`.
- A program named without its extension is found as its `.exe` or `.com`, and never as a batch file, whose arguments cmd.exe reads again as commands.

Kinds are gathered into groups: `network`, `disk`, `exec` and `env`. A group allows every scope of its kinds. `send` and `at` are in no group. `all` allows everything, secrets, `send` and `at` included, and means the same in every layer. Being allowed a secret is not the same as being sent it: a job is sent only the secrets its definition names (see Secrets).

## Layers

What a job may do is set by several layers, each written by a different hand.

```
the job in its definition    allow: [network, fs.read:/srv/**, exec:git]
the start                    allow: [network, fs.read:/srv/reports/**]
the client's ceiling         [network, disk]
the runner's ceiling         [disk, exec]

the job may                  read under /srv/reports
```

- A **definition** gives each script it runs its permissions: a list for the whole definition and more for each job.
- A **start** may narrow the whole run with a list of its own.
- A **parent** may narrow a child with a list on the job that starts it.
- The **client's ceiling** is set by the controller's owner, and the one that applies is that of the client that owns the run's tree.
- The **runner's ceiling** is set by the runner's owner.

Every layer is the same thing, a list to allow and a list to deny, and every layer is read the same way: what its deny list matches is refused, what its allow list matches passes, and anything else is refused. A job may do something only if every layer passes it. So a layer can narrow what is above it and can never widen it.

A start or a parent's job that carries no list adds no layer. A child machine that a definition names came with that definition and is the same author's words, so it runs with what its own definition says, and its permissions are not written out again where it is used.

```
parent                       the child's job may
  job: { machine: ./child.yaml }                              what child.yaml allows
  job: { machine: ./child.yaml, allow: [fs.read:/srv/**] }    what child.yaml allows, within /srv
  job: { machine: ${stages.plan.output} }                     nothing
  job: { machine: ${stages.plan.output}, allow: [net:a.example] }    what the made machine allows, within a.example
```

A machine made while running is not its parent's author's words: a script or a model wrote it. It is always narrowed by the job that starts it, and with no list it is allowed nothing. Everything below it is made while running too, and its steps are taken in a process of their own (see Controller).

A made machine has no ask of its own, so what it holds goes to an ask its author did not write (see Machines).

The scope of a definition's allow entry may be filled from the machine's input, as in `fs.read:${input.dir}/**`. It reads nothing but the input, and since every layer above still applies, an input can choose a place within a limit and never move the limit. A value set at the run's location fills a scope the same way, as in `net:${values.HUB}` (see secret.md).

## Where a run's authority comes from

A run's authority comes from its location, the path of its file under its tree's prefix:

```
its file's allow: and ask:        what its jobs may do, and where held jobs go
its tree's client's ceiling       the most any run in that tree may do
secrets and values                those set at its location or above; the nearest wins (see secret.md)
```

- A start's list, a parent's job list and the runner's ceiling narrow it, as in Layers.
- A sender or a caller adds nothing to a run's authority and takes nothing from it. The input it gives may fill an allow scope, within the receiver's own patterns and every ceiling.
- A child started by `machine:` runs at its parent's location. A machine made while running runs at the location of the job that starts it.
- A made machine's sends would carry that job's location in `event.from`, so it sends only to its parent and its children.

A tree belongs to the client that first submitted it. Only that client submits there, and reads and cancels the runs at its locations. A client submits trees only at prefixes its ceiling's `at:` entries match.

```
work$ corbie client card | ssh ctl corbie controller enrol - --ceiling "at:/work,send:/work/**,network,secret:*@*"
shop$ corbie client card | ssh ctl corbie controller enrol - --ceiling "at:/shop,send:/shop/**,send:/work/services/lookup,network,secret:ANTHROPIC_KEY@*"
```

- The shop client can call `/work/services/lookup`, and cannot submit at `/work` or cancel the runs there.
- A run of `/work/services/lookup` has the work client's ceiling and the secrets set at its location, whoever called it.

## Sending and calling

A send or a call is checked on both sides:

1. Every layer of the sender holds a `send:` entry that matches the receiver's location, its client's ceiling included. Otherwise the send is `denied`.
2. The receiver's `receive:` accepts the sender. Otherwise the send is noted as undelivered in the sender's record, and nothing fails.

- A statechart's send action is checked against the chart's layers.
- The runner's ceiling takes no part, since no runner sends.
- A client sends to and calls the runs of a tree it owns with no `send:` entry, whatever their `receive:` says, since it can cancel and submit them already.
- A parent run and its children send to each other with no `send:` or `receive:` entry.

`receive:` is written in the receiver's file:

| `receive:` | Accepts |
|---|---|
| none | every location of its own tree, and the client that owns the tree |
| `[thermostats/*, client:panel]` | those senders, for every event |
| `{ set: [thermostats/*, client:panel], stop: [client:panel] }` | each event from its own list only |

An entry is a location, read as in a definition, or `client:<name>`. It matches the sender's location or the client that sent, never a run's ID. A written `receive:` replaces the default whole, so an event a mapping does not name, and a call, are accepted from no sender but the tree's client.

```yaml
# work/services/lookup.yaml             location /work/services/lookup
flow: lookup
receive: [agents/*, /shop/helper]
input: { customer: string }
stages:
  find:
    job:
      script: ./lookup.js
      input: { customer: "${input.customer}" }
      allow: [net:db.internal:5432, "secret:DB_PASSWORD@net:db.internal:5432"]
output: ${stages.find.output}

# shop/helper.yaml                      location /shop/helper
agent: shop-helper
input: { question: string }
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.question}
tools:
  customer:
    does: Look a customer up by their number.
    args: { customer: string }
    job: { call: { at: /work/services/lookup }, input: { customer: "${args.customer}" }, allow: [send:/work/services/lookup] }
```

- The call from `/shop/helper` passes the tool's `allow:`, the shop client's ceiling and the lookup's `receive:`.
- A call from `/shop/other` is refused by the lookup's `receive:`, and the call job fails.
- The controller writes the sender into `event.from`. Anything about the sender in the data is only the sender's claim.

A gateway is a client, and a request to it is a call to a location by the gateway's client, checked as above (see client.md). The file's `receive:` lists `client:<name>` for the gateway's client, or that client owns the tree.

## Checking

The check is made at the moment a script acts, and it is made in one place. A plugin never checks a permission and never opens anything by itself. It asks Corbie for a way to connect, a path, a program or a variable, and Corbie checks every layer, notes what was used, and hands over the thing or refuses (see Plugins).

A connection is checked by the name asked for and again by the address it resolves to, and only a checked address is connected to. Where a service hands out further addresses, each is checked the same way.

## Refusals

A refused call stops the job with the status denied. Every refusal says three things: what was tried, which layer refused it, and what would fix it, down to the line to add or the command to run. The fix for a ceiling names the command and the one line, and never shows the rest of the ceiling. A denial passes up through parent machines unchanged, so the fix reaches whoever can make it.

## Asking

A definition may list permissions that must be asked about. Before any job in that machine, or in a machine below it, starts, the permissions it declares are read against the list. A job that declares one of them, or a group that contains one, is held for the ask, exactly as a job marked for approval is (see Machines). Whether a job is held depends on what it declares alone, before anything is sent for it.

A called run's held jobs go only to its own file's `ask:`, and are denied if it has none. A call and a `job: { send: … }` are jobs, so the caller's `ask_about: [send:…]` holds them. A statechart's send action is never held.

Asking only ever holds. An approved job gets exactly what its layers already allowed, and nothing can be had by asking that a layer refuses. A job never stops halfway to wait for a person.

## What a job used

Every job's result lists the permissions it used. That gives a way to go from loose to strict: run with everything allowed, read what was used, and write that into the definition.

## A job allowed nothing

A job with no permissions still has its input and its result, plain computation, making and reading blobs in its own instance's record, what it emits, and defining machines. Its own process reads a machine it defines, under the job's time and memory limits, so what that costs falls on that job alone (see Runner). The limits every definition has also apply (see Machines):

- No file leads back to itself.
- A machine as it is stored is at most 4 MiB.
- A YAML alias may not contain itself, and a file whose aliases make more than about 100 times what it writes, past 1000 values, is refused.

A machine a job made runs only when a job starts it, which takes `machine: ${…}` in a running machine's definition. A script cannot start a machine. Each of its steps is then taken in a process of its own, with a limit on its memory and its time, and a step past either fails that machine and nothing else (see Controller). That process checks the definition again and takes nothing a job's process said on trust. A bug in what it runs gets that process and no more: the controller takes from it only what a definition could say, and works out every name, runner and flag itself.

A shape a `list:` job gives is read the same way: every step of that agent after its first is taken in a process of its own, with the same limits, and past either the agent fails with the reason and nothing else does.

## What Corbie does not defend against

A script cannot reach outside itself except through the kinds above, and those are checked. Beyond that, these are true and Corbie says so.

- A program a job starts does its own reading, writing and connecting, as the runner's user. The permissions govern the start and nothing after. A runner's owner can put a box around started programs (see Runner); Corbie does not build one.
- A permission to connect to a host governs the connection and nothing sent over it. A job allowed an MCP server can call any of its tools; what a call can do is set by the credentials the server is given.
- An agent's listed tools are described by whoever wrote the list, and the model reads that text as it reads a tool's result.
- A job that may read the runner's own files can read the runner's key, and with it every job sent to that runner.
- A script allowed to reach a secret's destination can learn the secret from the other side.
- A tool that a model's own program runs for itself, where a definition has switched one on, is not checked by Corbie: it has everything its group allows, and only its use is noted (see Models).
- A plugin is trusted. One that opens something by itself, or uses a library that reads the computer's settings unasked, is a hole, and only review prevents it.
- Whether a connection is encrypted is known to Corbie when Corbie does the encrypting. Where a service's own protocol does it, that is the plugin's word.
- A job cannot listen for connections. Nothing a job opens outlives the job.
- A served directory's wire is private to the two computers. Someone on the network between them can see that they talk and how much, and can hold work up or cut it off, and nothing else (see Store).
- A step of an agent with a `list:` tool runs in a process of its own and its answer is the agent's word, so a bug in what that process runs can ask for any job of the agent's definition, with any lists and no approval the definition's jobs would have marked, and can send a job to another ask that definition writes. A permission the layers above refuse stays refused, `ask_about:` still holds what it lists, and an ask stays the definition's.
- An owner's expression runs in the controller's own process. Nothing limits its memory, and its one second stops its own code and not a call to something built in, so `'a'.repeat(2**40)` ends the controller and a regular expression that backtracks holds a step without end. An owner can do this to a controller that takes their definitions. Only a definition a client submitted has its expressions run in the controller's own process; a definition a job stored runs in a process of its own (see Controller).
- An owner's expression is read and compiled in the controller's own process, and nothing stops that. Reading `=>` nested in what stands before another takes twice as long with each, and so does compiling a chain of `&&`, `||` or `??` whose left sides are fixed, so forty of either never finish. 4 MB of brackets ended a client for want of stack in 2.4 seconds, and the controller reads an expression with the same code.
- Nothing limits what reading a definition costs the process that reads it. An owner's definition costs this to their own client, gateway and controller. A job's costs that job, and past its limits the job fails and the runner goes on: 4 MB of brackets in a define ended the job's process, and 20 levels of files that each name the next twice ran to its time limit. These were measured reading and checking each:

```
  400 KB of brackets in one expression               2 seconds     1 GiB
  4 MiB of YAML that writes [1,1,…]                  2.3 seconds   660 MiB
  4 MiB of YAML that writes [{a},{a},…]              2 seconds     1 GiB
  14 levels of files that each name the next twice   3.9 seconds   230 MiB, twice that with each level
```

- Whoever holds a gateway's token calls every location that accepts the gateway's client, with any input.
- Machines made while running that each use a step's whole time delay other machines: a pass waits for four of those steps at a time, up to 10 seconds, and every other run pays that in each pass it needs. Beside four such machines, a run of 20 ms took 10 seconds.
- A step that asks for as much as fits in its 4 MiB delays other machines for as long as carrying it out takes.
- A host with less memory than four steps' 4 GiB stops a step's process or the whole controller. The step is then taken once more, and its machine fails.
- A stored definition is read as plain JSON by the controller, to keep or copy what it names and, for a made machine, for what its answer may not say. Each is at most 4 MiB, is read once in a pass, and nothing in it is compiled or evaluated.
- A value is checked against a shape in time and memory that grow with the size of both, and nothing limits a shape. An owner's shape costs this to the controller. A made machine's shape, and a shape a list gives, stop at the step's limits. These were measured, with a value of 256 KiB unless it says otherwise:

```
  a list of texts against items: { type: string }    0.07 µs for each character
  the same list with every item wrong                0.3 seconds, and an error of 4.8 MiB
  numbers against nine $refs of 200 types each       119 seconds
  3000 such numbers, every one wrong                 ended a process at 2.8 GiB
  forty $refs that each name the next twice          never ends
```

Jobs that must be kept apart go to different runners.
