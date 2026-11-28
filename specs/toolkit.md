# Toolkit

A toolkit is a machine that holds tools. Another machine opens it and calls its tools with a job, as it runs any machine: by its file with `machine:`, or by its location with `call:`. An agent's entry that names a toolkit does both for its model (see [Agent](agent.md#toolkits)).

```yaml
# kits/notes.yaml          location /x/kits/notes
toolkit: notes
description: Notes kept on shelves.
input: { shelf: string }
instructions: Notes are on the shelf ${input.shelf}.
tools:
  add_note:
    description: Add a note.
    args: { text: string }
    job: { script: ./add.js, input: { shelf: "${input.shelf}", text: "${args.text}" }, allow: ["secret:NOTES_TOKEN@net:notes.example.com"] }
```

```yaml
# a stage of a flow at /x/shop
job:
  call: { at: kits/notes }
  input: { shelf: a, request: { do: call, tool: add_note, args: { text: milk } } }
```

```
output: {"result":"added to a: milk"}
```

A toolkit's definition has a `description:`, and either `tools:` with `instructions:`, or one `job:`. A toolkit is never a singleton: each request is a run.

## Requests

A run of a toolkit is given `request:` in its input, beside what the toolkit's `input:` names. `do:` says what is asked.

| Request | Answer |
|---|---|
| `{ do: open }` | `{ instructions, entries }` |
| `{ do: call, tool, args }` | `{ result }`, `{ working, state }` or `{ requests, state }` |
| `{ do: resume, tool, args, state, responses }` | the same three |
| `{ do: cancel, tool, args, state }` | `{ result }` |

- The answer is the run's result.
- An entry is `{ kind, name, description, args }`. `kind` is `tool` or `toolkit`, and `args` is the shape of a tool's arguments.
- `{ working, state }` says the work has not ended, and `{ requests, state }` that it needs input. A resume gives `state` back, with `responses` to the requests.
- `toolkit:` in a request is a list of names that lead to a toolkit inside this one.
- `said:` in a call, a resume or a cancel is a text: what the caller says of the call. A hook reads it as `job.tool.said`.
- A request of any other form fails the run, and the error names the field.

## Tools written in the toolkit

A toolkit with `tools:` answers a request itself.

1. An open fills `instructions:` from the input, and gives one entry of kind `tool` for each tool, in the order of their names.
2. A call checks `args` against the tool's `args:`.
3. The call starts the tool's job, which reads `input` and `args`.
4. The run ends with `{ result }`, the job's output. A job that fails fails the run.
5. A job whose output is `{ working, state }` or `{ requests, state }` ends the run with that answer in place of a result.
6. A resume or a cancel is a call again: the same check, and the same job, which reads the request as `input.request`.

| A request that | Ends the run |
|---|---|
| names a tool the toolkit does not have | failed, naming the tools it has |
| gives arguments that do not fit | failed, naming each place |
| gives `toolkit:` | failed: the toolkit has none inside it |

No job starts for any of these.

A tool is written in place, as an agent's tool is, or names a machine by its file or by its location.

```
  description:, args:, job:    a tool written in place
  machine: ./file.yaml         a machine named by its file
  call: location               a machine named by its location
```

A tool written in place has `description:`, `args:` in the short form or in full, and `job:`. A `blob` in the short form is the blob itself, as in a machine's input; an agent offers it to the model as a name.

## A tool that names a machine

A tool written `machine:` or `call:` takes its description and its arguments from the machine it names, and has no `description:`, `args:` or `job:`.

| Field | What it is |
|---|---|
| `machine:` | the machine's file, which runs as a child run of the toolkit's run |
| `call:` | the machine's location in the same tree, where it runs as a run of its own |
| `input:` | the names of the machine's input the tool fixes; it reads the toolkit's input |
| `approve:` | `true` marks the tool's job for approval |
| `meta:` | the `meta:` of the tool's job |

```yaml
# toolkits/orders.yaml
toolkit: orders
description: The orders app, and its settings.
input: { team: string }
tools:
  read_file:   { call: gitlab/files/read,     input: { project: "${input.team}/orders" } }
  change_file: { call: gitlab/commits/create, input: { project: "${input.team}/orders" }, approve: true, meta: { channel: "#orders-team" } }

# gitlab/files/read.yaml
flow: read
description: Read one file of a project.
input: { project: string, path: string, ref: "string?" }
```

1. An open gives the entry `read_file` with the description `Read one file of a project.` and the arguments `path` and `ref`.
2. A call `{ path: a.txt }` is checked against those arguments. A fixed name, or a name the machine's input does not give, does not fit.
3. The call starts a run of `gitlab/files/read` with `{ project: group/orders, path: a.txt }`, for a toolkit whose input gives the team `group`. The toolkit's run ends with `{ result }`, that run's result.

- A tool written `call:` has `send:` for its location, and nothing else is written for it.
- A hook reads the call as `job.tool`, as for a tool written in place.
- The machine is read when the toolkit's definition is. What an agent's entry that names a machine in the same tree is refused for then, a tool is refused for (see [Agent](agent.md#an-entry-that-names-a-machine)).
- A tool that names a toolkit is refused, as is a location written from `/`.

## A resume waits

A resume whose job answers `{ working, state }` does not end. The toolkit waits and asks again:

1. It sets a timer for `working.wait` seconds, 5 by default and 1 at the least. No job is going, so no runner is in use.
2. At the timer it starts the job again, with the request's `state` replaced by the state the last answer gave.
3. It ends with the first answer that is not `working`.
4. An answer of `working` whose wait would end after `limits: { resume: }`, ten minutes by default, ends the run with `{ working, state }`.

```yaml
toolkit: exports
description: Exports that take a while.
limits: { resume: 30m }
tools:
  export:
    description: Export something.
    args: { what: string }
    job: { script: ./export.js, input: { what: "${args.what}", request: "${input.request}" } }
```

```js
// export.js
const r = input.request
if (r.do === "call") return { working: { said: "started", wait: 30 }, state: { id: start(input.what) } }
if (r.do === "cancel") return stop(r.state.id)
return ended(r.state.id) ? fetch(r.state.id) : { working: { wait: 30 }, state: r.state }
```

A call ends working at once, so its caller goes on. A toolkit with `job:` waits the same way.

## A job that answers

A toolkit with `job:` has no tools of its own. The job answers every request.

```yaml
# tools/github.yaml          location /x/tools/github
toolkit: github
description: GitHub issues, pull requests and code.
job:
  script: ./github.js
  allow: ["secret:GITHUB_TOKEN@net:api.githubcopilot.com"]
```

```js
// github.js
const server = mcp.connect("https://api.githubcopilot.com/mcp/", {
  headers: { Authorization: secrets.text`Bearer ${secrets.get("GITHUB_TOKEN")}` },
})
return server.toolkit(input.request)
```

- The job is given the toolkit's input, `request` in it, or the `input:` it writes.
- The job's output is the answer. The toolkit keeps only what the request's row above gives, and fails the run for any other form.
- An entry keeps `kind`, `name`, `description` and `args`. Any other field is dropped, so an entry names nothing to call.
- Two entries of one name fail the run.
- The toolkit does not compile a tool's `args` that a job gives. The job that answers a call checks its arguments.
- `server.toolkit` of the `mcp` plugin answers a request for an MCP server (see plugin.md).

## A skill

A skill is a toolkit whose instructions are its `SKILL.md`, with one tool, `read`, that gives one of its other files.

```yaml
# review/toolkit.yaml
toolkit: review
description: Review code using the team's checklist.
instructions: { include: ./SKILL.md }
files: [./references/checklist.md]
```

1. An open gives the text of `SKILL.md` as the instructions, and the entry of the tool `read`.
2. A call of `read` with `{ path: references/checklist.md }` gives that file's text.

- `files:` lists text files, each written as a path is. With it the toolkit has the tool `read`, and no tool of the toolkit may take that name.
- `read` takes `path`, one of the files as it is written from the toolkit's directory. Any other path does not fit.
- `read` is a script of Corbie's own, which is sent the files and is allowed nothing.
- Corbie runs no script that a skill holds: a script among the files is text `read` gives.
- A skill of an MCP server is a toolkit inside the server's toolkit, which the server's job answers for (see plugin.md).

## Authority

A toolkit's run is a run like any other, so the machine area gives all of this:

| The toolkit is named by | Its run | Hooks and secrets |
|---|---|---|
| `machine: ./notes.yaml` | a child run, at the caller's location | the caller's, and the file's own hooks |
| `call: { at: kits/notes }` | a run of its own, at the toolkit's location | that location's |

- A call needs `send:` for the location in the caller's layers, and the toolkit's `receive:` to accept the caller.
- A secret set at the toolkit's location is sent to its jobs, and not to its caller's.
- A toolkit writes `hooks:`, as any machine does. A hook reads the call as `job.tool`: `{ toolkit, name, args }`, with the toolkit's own name, and `said` where the request gives it. It is set on a tool's job, and on the job that answers a call, a resume or a cancel.
- A caller's hooks run for its call job, and for nothing in a run started by `call:`.

```yaml
# kits/orders.yaml
toolkit: orders
description: Orders, refunds and returns.
receive: [support/*]
hooks:
  team:
    when: "${job.approve}"
    before: { script: ./ask-in.js, input: { channel: "#orders-team", tool: "${job.tool.name}", args: "${job.tool.args}" } }
tools:
  refund:
    description: Refund an order.
    args: { order: number, amount: number }
    job: { script: ./refund.js, input: "${args}", approve: true }
```

Every refund waits for `#orders-team`, whichever machine calls it.
