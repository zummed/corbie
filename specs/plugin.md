# Plugins

A script can do nothing outside itself except by calling a plugin. This area is how that is layered, the one shape every plugin has, and what comes with Corbie.

## Three layers

A library, written in script, calls plugins: a Graph helper, a paging loop, an API wrapper. A plugin, written in Go, asks Corbie for a way out: http, net, fs, exec, crypto and the rest. The ways out are kept by Corbie, and there are four: open a connection, open a path, start a program, read a variable.

**The ways out** are few and Corbie keeps them. A plugin never opens anything by itself, except that `time` reads the runner's zone files. It asks Corbie for a way to connect, a path, a program or a variable, and Corbie checks the job's permissions, notes what was used, puts secrets in where they are bound to go, and hands the thing over or refuses. Because there is one place, the checks, the record of what was used and the handling of secrets are the same for every plugin and cannot be forgotten by one.

**Plugins** are thin and low-level. They give a script the plain building blocks: a web request, a raw connection, a file, a program, a hash. With them anything can be done.

**Libraries** make things easy, and are written in script. A helper for a particular service, with its sign-in, paging and retries, is a script file that uses the plugins below it. A library needs no trust: it can do only what the job is permitted, and it holds only handles to secrets. So the part of Corbie that has to be right stays small.

## The ways out

| Way out | What the plugin gets | Checked against |
|---|---|---|
| Connect | a way to open connections, for a plain connection or for one carrying named secrets | `net`, and each secret's destination |
| Path | a file's real location, to read or to write | `fs.read`, `fs.write` |
| Program | a started program, given its arguments, variables and files | `exec` |
| Variable | the value of one of the runner's variables | `env` |
| The job's own channel | its result, what it emits, its blobs, the machines it defines | nothing; every job has it |

A plugin that connects is given a way to open connections, not one open connection, because many services hand out further addresses to connect to. Each address is checked like the first, and a secret can be had only for an address it is bound to.

A plugin that wraps someone else's library turns off whatever that library would find by itself, such as the computer's variables, files in the user's home and its own lookups, so that nothing leaves except by the ways above.

## One shape

Every plugin is written the same way and reads the same way to a script.

- A call is `module.call(first, options)`: the one obvious thing first, then named options. An option the plugin does not know is refused.
- Calls wait for their answer.
- What goes in and comes out is plain data: text, numbers, lists, maps, bytes, handles to secrets, references to blobs. Bytes are one type everywhere.
- A map read from YAML, JSON or a table keeps its keys in the order they were written, as Node's `JSON.parse` keeps them.
- Something that stays open, such as a connection, is an object with its calls and a `close`.
- An error is one shape, with a short code from a short list: not found, refused by the other side, timed out, not permitted, bad input, and failed for anything else.

Corbie's names are its own. Where a script's standard language already has a thing, such as `URL` or `JSON`, a script has that.

Scripts have neither `Intl` nor `Temporal`, so a time zone comes from the `time` plugin.

A plugin's page in the manual is its specification. It lists each call, its options, and the permissions it needs, and its examples are run as tests.

## What comes with Corbie

| Plugin | Gives a script |
|---|---|
| `http` | web requests: headers, bodies, uploads, redirects it follows or not, large bodies into blobs |
| `net` | a raw connection, plain or encrypted, to read and write bytes and lines |
| `fs` | files and directories |
| `exec` | starting a program and getting what it printed |
| `env` | the runner's variables |
| `crypto` | hashes, keyed hashes, random bytes, signing and checking, encryption |
| `compress` | packing and unpacking |
| `html`, `xml` | reading a page or a document as a tree |
| `yaml` | reading a YAML document as data, and writing data as YAML |
| `time` | the date and time of day an instant has in a time zone, and the instant of a date and time in one |
| `websocket` | a connection that carries messages both ways, kept alive while the script works |
| `ssh` | running a command on another computer |
| `blobs` | making and reading blobs |
| `secrets` | handles to secrets |
| `machines` | defining a machine while a job runs |
| `llm` | calling a model |
| `mcp` | the tools of an MCP server, over the web or from a program it starts, calls to them, and calls that end later |

`mcp` is a plugin so that a script uses any MCP server with one call: it speaks the protocol and its versions, signs in to a server on the web, and keeps a server it starts as a program open for several calls, which `exec` does not give a script.

`server.toolkit(request)` answers one of four requests, so that one script serves a server's tools to any caller:

| Request | Answer |
|---|---|
| `{ do: open }` | `{ instructions, entries }`: the server's instructions and its tools |
| `{ do: call, tool, args }` | `{ result }`, `{ working, state }` or `{ requests, state }` |
| `{ do: resume, tool, state, responses }` | the same three |
| `{ do: cancel, tool, state }` | `{ result }` |

- A server on the web may answer a call with a task, which ends later. The call gives `{ working, state }`, and a resume with that state, in the same job or another, asks the server once.
- A call or a task that needs input gives `{ requests, state }`, and a resume with the responses answers it.
- A program stops when its job ends, so the plugin waits in the job for a program's task.

A server that has the Skills extension, `io.modelcontextprotocol/skills`, gives its skills as toolkits inside its own:

1. An open lists each skill after the tools, as `{ kind: toolkit, name, description }`, from the skill's frontmatter.
2. An open with `toolkit: [code-review]` reads the skill's `SKILL.md` and gives its text as the instructions, with one tool, `read`.
3. A call of `read` with `toolkit: [code-review]` and `{ path }` gives one of the skill's other files: a text, or a blob.

- `read` takes only a path the skill's list of files gives. The plugin reads no other address of the server.
- The plugin checks the SHA-256 of each file it reads against the digest in the skill's list, and fails with both where they differ. That finds a file changed or cut short since the list was made. It is no control: the server writes both.
- A skill whose files the server makes as they are asked for has no list of them, and is left out.
- The plugin reads a file only when it is asked for, and runs nothing a skill holds.

Corbie also comes with libraries written in script, among them the standard turn of an agent.

Databases, message queues, mail and the like are not in this list. Each is an extension, added by whoever needs it.

## Extensions

An extension is a Go module compiled into the binary. It can offer any of: plugins, libraries written in script, a machine type, a model service. Every plugin that comes with Corbie is built the same way, through the same public package, and uses nothing an extension could not.

A binary with extensions is built with Go from Corbie's source plus the modules wanted. Each extension carries its pages of the manual.

Runners need not all be alike. A job that calls a plugin its runner does not have fails, saying which, and tags are how such a job is sent to a runner that has it.
