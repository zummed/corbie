# Permissions

A job can do nothing outside itself unless it is permitted. This area is how a permission is written, who can give and limit one, and how it is checked.

## Writing one

A permission is a kind and a scope: `fs.read:/srv/reports/**`, `net:api.github.com`, `exec:git`.

There are six kinds.

| Kind | Allows | Scope |
|---|---|---|
| `fs.read` | reading files | a path |
| `fs.write` | writing files | a path |
| `net` | connecting to a host | a host, with a port if it matters |
| `exec` | starting a program | a program's name or path |
| `env` | reading a variable of the runner's | a name |
| `secret` | sending a secret somewhere | a secret's name and a destination |

A scope is one of three things, matched the same way wherever it appears: a path, a host, or a name. Any of them may use `*` for a part. A host with no port means every port, a host of `*` means every host, and a host written as an address, `10.0.0.*`, matches no name. A path is checked where it really points, after links are followed.

On Windows:

- A path starts with its drive or share and is written with `/`, as `C:/Users/sam/**` or `//server/share/**`.
- A path with a leading `/` and no drive is on whichever drive is current, so as a scope it matches nothing, and a script that names it is refused.
- A path and a program's name match in any case, and a program's name matches with or without `.exe` or `.com`.
- A program named without its extension is found as its `.exe` or `.com`, and never as a batch file, whose arguments cmd.exe reads again as commands.

Kinds are gathered into groups: `network`, `disk`, `exec` and `env`. A group allows every scope of its kinds. `all` allows everything, secrets included, and means the same in every layer. Being allowed a secret is not the same as being sent it: a job is sent only the secrets its definition names (see Secrets).

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
- The **client's ceiling** is set by the controller's owner.
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

A machine made while running is not its parent's author's words: a script or a model wrote it. It is always narrowed by the job that starts it, and with no list it is allowed nothing. Everything below it is made while running too, and its steps are taken in a process of their own (see Controller). It has no ask of its own, so what it holds goes to an ask its author did not write (see Machines). It sends to no key and gives no job a key, so it talks only to its parent and its children: a forged event sent to a keyed run would look like any other. A job above it may start it with a key.

The scope of a definition's allow entry may be filled from the machine's input, as in `fs.read:${input.dir}/**`. It reads nothing but the input, and since every layer above still applies, an input can choose a place within a limit and never move the limit.

## Checking

The check is made at the moment a script acts, and it is made in one place. A plugin never checks a permission and never opens anything by itself. It asks Corbie for a way to connect, a path, a program or a variable, and Corbie checks every layer, notes what was used, and hands over the thing or refuses (see Plugins).

A connection is checked by the name asked for and again by the address it resolves to, and only a checked address is connected to. Where a service hands out further addresses, each is checked the same way.

## Refusals

A refused call stops the job with the status denied. Every refusal says three things: what was tried, which layer refused it, and what would fix it, down to the line to add or the command to run. The fix for a ceiling names the command and the one line, and never shows the rest of the ceiling. A denial passes up through parent machines unchanged, so the fix reaches whoever can make it.

## Asking

A definition may list permissions that must be asked about. Before any job in that machine, or in a machine below it, starts, the permissions it declares are read against the list. A job that declares one of them, or a group that contains one, is held for the ask, exactly as a job marked for approval is (see Machines). Whether a job is held depends on what it declares alone, before anything is sent for it.

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

A script cannot reach outside itself except through the six kinds above, and those are checked. Beyond that, these are true and Corbie says so.

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
- A step of an agent with a `list:` tool runs in a process of its own and its answer is the agent's word, so a bug in what that process runs can ask for any job of the agent's definition, with any lists and no approval the definition's jobs would have marked, and can send a job to another ask that definition writes. It can also give those jobs any key that is free. A permission the layers above refuse stays refused, `ask_about:` still holds what it lists, and an ask stays the definition's. Such an agent's step cannot send to a key: no agent's definition can.
- An owner's expression runs in the controller's own process. Nothing limits its memory, and its one second stops its own code and not a call to something built in, so `'a'.repeat(2**40)` ends the controller and a regular expression that backtracks holds a step without end. An owner can do this to a controller that takes their definitions. A client starts a run only from a definition in that run's own record, so a definition a job stored reaches this process only if a client copies it there.
- An owner's expression is read and compiled in the controller's own process, and nothing stops that. Reading `=>` nested in what stands before another takes twice as long with each, and so does compiling a chain of `&&`, `||` or `??` whose left sides are fixed, so forty of either never finish. 4 MB of brackets ended a client for want of stack in 2.4 seconds, and the controller reads an expression with the same code.
- Nothing limits what reading a definition costs the process that reads it. An owner's definition costs this to their own client, gateway and controller. A job's costs that job, and past its limits the job fails and the runner goes on: 4 MB of brackets in a define ended the job's process, and 20 levels of files that each name the next twice ran to its time limit. These were measured reading and checking each:

```
  400 KB of brackets in one expression               2 seconds     1 GiB
  4 MiB of YAML that writes [1,1,…]                  2.3 seconds   660 MiB
  4 MiB of YAML that writes [{a},{a},…]              2 seconds     1 GiB
  14 levels of files that each name the next twice   3.9 seconds   230 MiB, twice that with each level
```

- A gateway takes a definition of up to 64 MiB from whoever holds its token, and reads it as a client does.
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
