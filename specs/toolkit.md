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
| `{ do: resume, tool, state, responses }` | the same three |
| `{ do: cancel, tool, state }` | `{ result }` |

- The answer is the run's result.
- An entry is `{ kind, name, description, args }`. `kind` is `tool` or `toolkit`, and `args` is the shape of a tool's arguments.
- `{ working, state }` says the work has not ended, and `{ requests, state }` that it needs input. A resume gives `state` back, with `responses` to the requests.
- `toolkit:` in a request is a list of names that lead to a toolkit inside this one.
- A request of any other form fails the run, and the error names the field.

## Tools written in the toolkit

A toolkit with `tools:` answers a request itself.

1. An open fills `instructions:` from the input, and gives one entry of kind `tool` for each tool, in the order of their names.
2. A call checks `args` against the tool's `args:`.
3. The call starts the tool's job, which reads `input` and `args`.
4. The run ends with `{ result }`, the job's output. A job that fails fails the run.

| A request that | Ends the run |
|---|---|
| names a tool the toolkit does not have | failed, naming the tools it has |
| gives arguments that do not fit | failed, naming each place |
| resumes or cancels | failed: a tool's call ends with its result |
| gives `toolkit:` | failed: the toolkit has none inside it |

No job starts for any of these.

A tool is written as an agent's tool is: `description:`, `args:` in the short form or in full, and `job:`. A `blob` in the short form is the blob itself, as in a machine's input.

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

## Authority

A toolkit's run is a run like any other, so the machine area gives all of this:

| The toolkit is named by | Its run | Hooks and secrets |
|---|---|---|
| `machine: ./notes.yaml` | a child run, at the caller's location | the caller's, and the file's own hooks |
| `call: { at: kits/notes }` | a run of its own, at the toolkit's location | that location's |

- A call needs `send:` for the location in the caller's layers, and the toolkit's `receive:` to accept the caller.
- A secret set at the toolkit's location is sent to its jobs, and not to its caller's.
- A toolkit writes `hooks:`, as any machine does. A hook reads the call as `job.tool`: `{ toolkit, name, args }`, with the toolkit's own name. It is set on a tool's job, and on the job that answers a call, a resume or a cancel.
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
