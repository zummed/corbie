# Plugins

A script can do nothing outside itself except by calling a plugin. This area is how that is layered, the one shape every plugin has, and what comes with Corbie.

## Three layers

A library, written in script, calls plugins: a Graph helper, a paging loop, an API wrapper. A plugin, written in Go, asks Corbie for a way out: http, net, fs, exec, crypto and the rest. The ways out are kept by Corbie, and there are five: open a connection, open a path, start a program, read a variable, read or write a kept value.

**The ways out** are few and Corbie keeps them. A plugin never opens anything by itself. It asks Corbie for a way to connect, a path, a program, a variable or a kept value, and Corbie checks the job's permissions, notes what was used, puts secrets in where they are bound to go, and hands the thing over or refuses. Because there is one place, the checks, the record of what was used and the handling of secrets are the same for every plugin and cannot be forgotten by one.

**Plugins** are thin and low-level. They give a script the plain building blocks: a web request, a raw connection, a file, a program, a hash. With them anything can be done.

**Libraries** make things easy, and are written in script. A helper for a particular service, with its sign-in, paging and retries, is a script file that uses the plugins below it. A library needs no trust: it can do only what the job is permitted, and it holds only handles to secrets. So the part of Corbie that has to be right stays small.

## The ways out

| Way out | What the plugin gets | Checked against |
|---|---|---|
| Connect | a way to open connections, for a plain connection or for one carrying named secrets | `net`, and each secret's destination |
| Path | a file's real location, to read or to write | `fs.read`, `fs.write` |
| Program | a started program, given its arguments, variables and files | `exec` |
| Variable | the value of one of the runner's variables | `env` |
| Kept value | the value kept at a kept path, to read, keep, list or remove | `kept.read`, `kept.write` |
| The job's own channel | its result, what it emits, its blobs, the machines it defines | nothing; every job has it |

A plugin that connects is given a way to open connections, not one open connection, because many services hand out further addresses to connect to. Each address is checked like the first, and a secret can be had only for an address it is bound to.

A plugin that wraps someone else's library turns off whatever that library would find by itself, such as the computer's variables, files in the user's home and its own lookups, so that nothing leaves except by the ways above.

## One shape

Every plugin is written the same way and reads the same way to a script.

- A call is `module.call(first, options)`: the one obvious thing first, then named options. An option the plugin does not know is refused.
- Calls wait for their answer.
- What goes in and comes out is plain data: text, numbers, lists, maps, bytes, handles to secrets, references to blobs. Bytes are one type everywhere.
- Something that stays open, such as a connection, is an object with its calls and a `close`.
- An error is one shape, with a short code from a short list: not found, refused by the other side, timed out, not permitted, bad input, and failed for anything else.

Corbie's names are its own. Where a script's standard language already has a thing, such as `URL` or `JSON`, a script has that.

A plugin's page in the manual is its specification. It lists each call, its options, and the permissions it needs, and its examples are run as tests.

## What comes with Corbie

| Plugin | Gives a script |
|---|---|
| `http` | web requests: headers, bodies, uploads, redirects it follows or not, large bodies into blobs |
| `net` | a raw connection, plain or encrypted, to read and write bytes and lines |
| `fs` | files and directories |
| `exec` | starting a program and getting what it printed |
| `env` | the runner's variables |
| `kept` | values kept in the store, which outlive every run |
| `crypto` | hashes, keyed hashes, random bytes, signing and checking, encryption |
| `compress` | packing and unpacking |
| `html`, `xml` | reading a page or a document as a tree |
| `websocket` | a connection that carries messages both ways, kept alive while the script works |
| `ssh` | running a command on another computer |
| `blobs` | making and reading blobs |
| `secrets` | handles to secrets |
| `machines` | defining a machine while a job runs |
| `llm` | calling a model |
| `mcp` | the tools of an MCP server, over the web or from a program it starts, and calls to them |

`mcp` is a plugin and not a library because a server it starts is a program kept open, reading and writing, for several calls, which `exec` does not give a script.

Corbie also comes with libraries written in script, among them the standard turn of an agent.

Databases, message queues, mail and the like are not in this list. Each is an extension, added by whoever needs it.

## Extensions

An extension is a Go module compiled into the binary. It can offer any of: plugins, libraries written in script, a machine type, a model service. Every plugin that comes with Corbie is built the same way, through the same public package, and uses nothing an extension could not.

A binary with extensions is built with Go from Corbie's source plus the modules wanted. Each extension carries its pages of the manual.

Runners need not all be alike. A job that calls a plugin its runner does not have fails, saying which, and tags are how such a job is sent to a runner that has it.
