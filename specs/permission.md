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

A scope is one of three things, matched the same way wherever it appears: a path, a host, or a name. Any of them may use `*` for a part. A host with no port means every port, and a host written as an address, `10.0.0.*`, matches no name. A path is checked where it really points, after links are followed.

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

A machine made while running is not its parent's author's words: a script or a model wrote it. It is always narrowed by the job that starts it, and with no list it is allowed nothing.

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

A job with no permissions still has its input and its result, plain computation, making and reading blobs in its own instance's record, what it emits, and defining machines.

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

Jobs that must be kept apart go to different runners.
