# Agent

An agent is a machine whose tools a model picks. The model is asked what to do, the tools it picks are run, the results go back to it, and this repeats until it answers without asking for a tool.

Corbie itself knows nothing about models. A turn is an ordinary job and so is a tool, and an agent is held to its permissions and its ask like any machine.

```yaml
agent: librarian
input: { dir: string, question: string }
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.question}

tools:
  shelf:
    description: Give back every file on one shelf
    args: { shelf: string }
    job:
      script: ./shelf.js
      input: { dir: "${input.dir}", shelf: "${args.shelf}" }
      allow: ["fs.read:${input.dir}/**"]
```

## The loop

An agent's definition names its model and may give a system text, a prompt, tools, limits and the shape of its reply.

Each turn is one call to the model, made by a job named `turn`, which emits the reply under `text` (see Models). The model sees the system text, the conversation so far, and for each tool its name, its description and what arguments it takes. It never sees how a tool is run or what the tool is permitted.

The tools the model picks in one turn all run at once, and the next turn starts when every one of them has settled. A turn that fails fails the agent.

Corbie supplies the script that makes a turn. A definition may put its own in its place, and the supplied one can be called from it, which is how an agent's behaviour is wrapped or changed. There is no other hook.

An agent's result is the model's last reply as text, or as data in the shape the definition gives. If the last reply does not fit the shape, or was cut short by the model's length limit, the model is told and takes another turn. The result always carries the conversation and a count of the tokens the agent's own turns used, and so does its error if it fails.

An agent may be started with the conversation of an earlier agent's result, and carries on from it.

## Tools

A tool is a named job, a script, a machine, a call or a send, with a description, the shape of its arguments, and its own permissions. The job reads the agent's input and the arguments the model supplied. A tool whose job is an agent is how an agent has sub-agents: the model is shown what the sub-agent replied, and the sub-agent's conversation stays in its own record.

A tool's `description:` is plain text, and Corbie refuses an expression in it, as in a definition's. A call or a send names a location read from the agent's tree, and needs `send:` for it (see [Permissions](permission.md)).

```yaml
tools:
  customer:
    description: Look a customer up by their number.
    args: { customer: string }
    job:
      call: { at: services/lookup }
      input: { customer: "${args.customer}" }
      allow: [send:services/lookup]
  heating:
    description: Set the temperature of one room.
    args: { room: string, celsius: number }
    job:
      send: { to: { at: rooms/thermostat, key: "${args.room}" }, event: set, data: { celsius: "${args.celsius}" } }
      allow: [send:rooms/thermostat]
```

- A call starts a run of the transient location and waits. The model is shown that run's result, or its error.
- The called run's record belongs to its own tree's client. Cancelling the call cancels the run.
- A send ends `done` once the message is written. A send the receiver does not take is noted in the agent's record, and the model is not told.

A tool written `call:` names a location in the same tree and takes its description and arguments from the machine there. The client reads that machine's file when it reads the tree. `input:` fixes part of the machine's input, and the model gives the rest.

```yaml
# toolkits/orders.yaml
description: The orders app.
tools:
  read_file:   { call: gitlab/files/read,     input: { project: group/orders } }
  change_file: { call: gitlab/commits/create, input: { project: group/orders }, approve: true }

# gitlab/files/read.yaml
flow: read
description: Read one file of a project.
input: { project: string, path: string, ref: "string?" }
```

- The model is offered `read_file` as `Read one file of a project.`, with the arguments `path` and `ref`. A fixed name, or a name the machine's input does not give, does not fit.
- A call `{ path: a.txt }` starts a run of `gitlab/files/read` with `{ project: group/orders, path: a.txt }`. An argument the model leaves out is absent.
- `input:` may read the agent's input. `approve: true` holds the call for the ask.
- The tool has `send:` for the location, and nothing else is written for it. It has no `description:`, `args:` or `job:`.
- A blob in the machine's input is an argument the model gives by the name it was shown. An input written as JSON Schema gives arguments only through `properties`.
- A machine that is an agent is shown to the model as its reply.

The location is written out, from the tree's prefix. These are refused when the tree is read:

- a location with no machine, or a file `.corbieignore` lists;
- a machine with no `description:`, or one that is not `transient`;
- a fixed name the machine's input does not give;
- a machine whose input is written as JSON Schema with a keyword other than `type`, `properties`, `required`, `additionalProperties`, `description` and `title`, such as `allOf` or `$ref`;
- a tool written `call:` in a file read alone or in a machine made while running, which have no tree.

A tool may give `when:`, a condition on the agent's input. It is checked once, when the run starts, and a tool whose `when:` does not hold is not offered (see [Who is offered what](#who-is-offered-what)).

A tool that needs approval goes to the agent's ask like any job. A `call:` or `send:` job is held when `approve:` names its `send:` entry. The tool calls from one turn that need it go together, and the ask is shown the arguments, the tool's description and what the model said. A tool call that needs no approval runs at once, and the next turn waits for the ask as well as for the tools. A tool call that is not approved goes back to the model as denied, with the ask's note.

## Shared definitions

Agents share a model, tools and text with `include:`, as any machine does (see [Machines](machine.md#definitions)). A base file gives no type, so `.corbieignore` lists its directory: it has no location and runs nothing.

```yaml
# .corbieignore
agents/shared/

# agents/shared/base.yaml
input: { dir: string, said: string }
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
system: { include: ./base.md }
prompt: ${input.said}
tools:
  read_file: { include: ./tools/read_file.yaml }

# agents/operator.yaml
agent: operator
include: ./shared/base.yaml
tools:
  run_tests: { include: ./shared/tools/run_tests.yaml }

# agents/teacher.yaml
agent: teacher
include: ./shared/base.yaml
system:
  - { include: ./shared/base.md }
  - The person asking is new to this code. Explain each step before you give the change.
```

`agents/shared/base.md` holds the shared text:

```
You answer questions about the Go project in ${input.dir}.
Read before you answer, and name the file and the line you mean.
```

- `tools:` is a mapping, so it merges by name: the operator has `read_file` and `run_tests`.
- `system:` is a text, so the teacher's replaces the base's whole. Its pieces are joined by one empty line.
- `${input.dir}` in `base.md` is an expression, and reads the input of the agent that includes it.
- `corbie test agents/operator.yaml --definition` prints the operator written out, each value with its file and line.

## Listed tools

A tool entry may give a `list:` job in place of `description:` and `args:`. The job returns the tools, and the entry's `job:` runs each of them. This is how an agent is given the tools of an MCP server, which change without the definition changing.

```yaml
tools:
  github:
    list:
      script: ./github.js
      allow: [net:api.githubcopilot.com, "secret:GITHUB_TOKEN@net:api.githubcopilot.com"]
    job:
      script: ./github.js
      input: { tool: "${tool}", args: "${args}" }
      allow: [net:api.githubcopilot.com, "secret:GITHUB_TOKEN@net:api.githubcopilot.com"]
```

```js
// github.js
const server = mcp.connect("https://api.githubcopilot.com/mcp/", {
  headers: { Authorization: secrets.text`Bearer ${secrets.get("GITHUB_TOKEN")}` },
})
return input?.tool ? server.call(input.tool, { args: input.args }) : server.tools()
```

1. Before the first turn, the `list:` job runs and returns `[{ name, description, args }, …]`.
2. The model is shown each tool under the entry's name and its own, joined by `_`: `github_create_issue`.
3. A call runs the entry's `job:`, where `${tool}` is the tool's own name, `create_issue`.

- The list is made once in a run, kept in the agent's record, and is the same for every turn.
- Every step of the agent after its first is taken in a process of its own, since a list's shapes come from outside the definition (see Controller).
- Every listed tool has the entry's permissions and, if the entry's job is marked for approval, its approval. The ask is shown the name the model used, the arguments, the entry's name as `entry` and the tool's own name as `tool`.
- The model can call every tool the list gives. The `list:` script narrows that by returning fewer.
- A tool's description and its shape are shown to the model as the list gives them.
- In a name the model is shown, a character other than a letter, a digit, `_` or `-` is written `_`.
- The entry's `job:` reads `${args}` only whole, since its fields are not known until the run.

A list that fails fails the agent, and so does a list in which a tool has no name, a shape that is not a JSON Schema of an object, a shape that marks a blob, or a name the model is shown that another tool has or that is longer than 64 characters.

## Toolkits

A toolkit is a group of tools the model is not shown until it opens the toolkit. An agent with many tools shows the model the ones it has opened.

```yaml
agent: helper
input: { said: string, asker: any }
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.said}
toolkits:
  net: { include: toolkits/net.yaml }
  payments:
    include: toolkits/payments.yaml
    approve: true
    when: ${input.asker.scopes.includes('payments')}
```

```yaml
# toolkits/net.yaml
description: The firewall and the switch, and the network's docs.
instructions: |
  Run show commands only. ${input.asker.name} is asking.
ask: { machine: ./approval.yaml }
tools:
  show_status:
    description: Run one read-only status command.
    args: { device: string, command: string }
    job: { script: ./show-status.js, input: { device: "${args.device}", command: "${args.command}" }, allow: [net:192.0.2.1:22] }
```

1. The model is shown one more tool, `open`, with the name and `description:` of each toolkit not yet open.
2. The model calls `open` with a toolkit's name, and a job named `open` runs.
3. The model is shown the toolkit's `instructions:` as the result of `open`.
4. From the next turn, the model is shown that toolkit's tools as well.

| Field | What it is |
|---|---|
| `description:` | what the toolkit is for, in plain text with no expression, which the model reads before it opens it |
| `instructions:` | the text `open` gives the model, written as `system:` is |
| `ask:` | the ask for the toolkit's tools and for opening it |
| `approve:` | `true` holds the toolkit's open for the ask |
| `when:` | a condition on the agent's input: the toolkit is offered only if it holds |
| `tools:` | the toolkit's tools, written as an agent's are |
| `meta:` | data Corbie keeps and never reads |

- A toolkit's tools have their own permissions, as every tool has. Opening a toolkit gives the model names and instructions, and nothing else.
- The instructions are filled from the agent's input when the toolkit is opened. With none, `open` gives `Opened net.`. A run that carries the conversation on and opens the toolkit again is given them again.
- The result of `open` is shown to the model whole, and is never stored as a blob.
- A tool of a toolkit may not take the name of another tool of the agent.
- `open` is supplied by Corbie, as the blob tools are, and no tool may take its name. It is not shown while no toolkit can be opened.
- The list of an entry with `list:` in a toolkit is made before the first turn, as any list is. An error names it by where it is written, as `toolkits.net.tools.files.list`.
- A toolkit marked `approve: true` holds its `open` job for the ask, which is shown the toolkit's name as `toolkit` and the toolkit's `description:`. An open that is not approved goes back to the model as denied, with the ask's note, and opens nothing. Only an approved open opens such a toolkit.
- For a call of a toolkit's tool, and for the list of an entry in a toolkit, the ask is shown the toolkit's name as `toolkit`.
- A toolkit of an agent made while running cannot name an ask (see Machines).
- A toolkit may name an ask. It is the ask for the jobs and lists of the toolkit's tools and for opening the toolkit, before the agent's own, and the nearest ask above every machine below those tools' jobs. Its permissions are its own, as any ask's are, and its expressions read the agent's input. Like any ask, it is filled in and started once for the held calls of one turn, and one that cannot be filled in denies them.

## Who is offered what

A `when:` on a toolkit or on a tool is a condition on the agent's input. It is checked once, when the run starts, and the run keeps what it found: every turn is offered the same tools. A `when:` is written on the agent's entry, beside `include:`, so the agent says who is offered a toolkit.

- `open` does not name a toolkit whose `when:` does not hold. A call of `open` with its name does not fit the arguments, and goes back to the model as an error.
- A tool whose `when:` does not hold, or that is in such a toolkit, is not shown to the model. Its `list:` does not run, and a call of it runs nothing and goes back to the model as an error.
- A `when:` that cannot be evaluated fails the agent at its start.
- An agent started with a conversation checks its own input.
- `open` and the blob tools take no `when:`.

## Shapes

A tool's `args:` and an agent's `reply:` are shapes, as [Machines](machine.md#shapes) gives them:

```yaml
args: { shelf: string, from: number? }
```

The full form can say what the short form cannot:

```yaml
args:
  type: object
  properties:
    repo:   { type: string, description: "owner/name, as zummed/corbie" }
    labels: { type: array, items: { type: string, enum: [bug, docs, spec] } }
  required: [repo]
```

- The model is shown the shape as it is written.
- What the model sends is checked against the shape: arguments that do not fit go back to the model as an error, and so does a reply.
- `format: blob` marks a blob, at any depth of the shape.

## Limits

An agent has a number of turns, twenty by default, and the model is told when few remain. At the limit the agent fails. Its error carries the conversation, so an agent started with that conversation carries on from where it stopped.

## The conversation

The conversation is everything the model has been shown and has said: the prompt, each reply and tool call, and each call's result, error or denial. It is kept as its turns, each written once as the turn ends. Nothing is rewritten as the conversation grows.

In a result the conversation is one value: its turns and the blobs the model was shown, each under the name the model knows it by.

```
conversation: { turns: [ turn 1, turn 2, … ], blobs: { blob1: …, blob2: … } }
```

An agent started with it carries on where the other stopped. Every name goes on meaning the blob it meant, and a blob the conversation has not shown takes a name it has not used. It opens no toolkit: the model calls `open` for each toolkit it needs, and a toolkit marked `approve: true` waits for the ask again.

## Blobs and the model

A blob is shown to the model as a handle: a short name, its type and size, how many items it has, roughly what it would cost to read, and, the first time it is shown, a preview that depends on its type.

```
{ blob: "blob2", type: "text/csv", size: 48112033, items: 310224,
  cost: "about 12028008 tokens to read whole, 39 an item",
  preview: [ { date: "2026-01-02", total: "19.50" }, … ] }
```

The first time a blob is to be shown, a job named `preview` looks at it, between the tool's result and the next turn: it counts the items and takes the first few as the preview. The preview is part of what the model is told in that turn, and stays in the conversation as anything it was told does. It is kept nowhere else, and a blob shown again is shown without it.

A blob of a type the model's service takes as content, an image or a document, is also sent to the model as it is, beside its handle, from the turn it is first shown. Any other blob is only its handle, and the model works on it with its tools.

The model uses the handle's name to refer to the blob. The agent turns names into references before a tool is run, and references into handles before the model sees anything. A blob's key never reaches a model by any path, including inside the contents of another blob.

Every agent has tools for working with blobs without reading them whole: one to read a range of items, one to search, and one to run a few lines of code over a blob and get back only the answer. They are ordinary scripts, allowed nothing from the definition's lists. A tool result too large to show is stored as a blob, by the job that makes its preview, and the model is given its handle.

## Events

An agent acts on the results of its turns, its tools and its ask, and on nothing else. A send to an agent run by its address is noted as undelivered in the sender's record. Events sent down to it, and events its tools send up while they run, are ignored; a watcher still sees them in the record.

## The record

An agent's record holds these entries, under these names:

| Entry | What it is |
|---|---|
| job `turn`, `turn#2`, … | one turn, whatever script makes it; its input names the model as `model.name` |
| emit `text` of a turn | a piece of the model's reply, as `llm.chat` gives it |
| `done` from a turn | `{ turn, text, calls: [{ id, name, args }], stop, usage: { input, output } }` |
| job named after a tool, `shelf`, `shelf.1` | one call of the tool the model named; its `done` or `failed` goes back to the model |
| job `open` | an open of a toolkit |
| job `preview`, `preview.1` | the preview of a blob before the model is first shown it |
| job named after an entry with `list:` | the list of that entry's tools, made before the first turn |

`stop` is `end`, `tools` or `length`.
