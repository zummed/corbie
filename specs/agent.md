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
    does: Give back every file on one shelf
    args: { shelf: string }
    job:
      script: ./shelf.js
      input: { dir: "${input.dir}", shelf: "${args.shelf}" }
      allow: ["fs.read:${input.dir}/**"]
```

## The loop

An agent's definition names its model and may give a system text, a prompt, tools, limits and the shape of its reply.

Each turn is one call to the model, made by a job named `turn`, which emits the reply under `text` (see Models). The model sees the system text, the conversation so far, and for each tool its name, what it does and what arguments it takes. It never sees how a tool is run or what the tool is permitted.

The tools the model picks in one turn all run at once, and the next turn starts when every one of them has settled. A turn that fails fails the agent.

Corbie supplies the script that makes a turn. A definition may put its own in its place, and the supplied one can be called from it, which is how an agent's behaviour is wrapped or changed. There is no other hook.

An agent's result is the model's last reply as text, or as data in the shape the definition gives. If the last reply does not fit the shape, or was cut short by the model's length limit, the model is told and takes another turn. The result always carries the conversation and a count of the tokens the agent's own turns used, and so does its error if it fails.

An agent may be started with the conversation of an earlier agent's result, and carries on from it.

## Tools

A tool is a named job, a script or a machine, with a description, the shape of its arguments, and its own permissions. The job reads the agent's input and the arguments the model supplied. A tool whose job is an agent is how an agent has sub-agents: the model is shown what the sub-agent replied, and the sub-agent's conversation stays in its own record.

Agents share tools with `include:` under `tools:`, as any mapping takes fields from a file (see [Machines](machine.md#definitions)).

A tool that needs approval goes to the agent's ask like any job. The calls from one turn that need it go together, and the ask is shown the arguments, what the tool does and what the model said. A call that needs no approval runs at once, and the next turn waits for the ask as well as for the tools. A call that is not approved goes back to the model as denied, with the ask's note.

## Listed tools

A tool entry may give a `list:` job in place of `does:` and `args:`. The job returns the tools, and the entry's `job:` runs each of them. This is how an agent is given the tools of an MCP server, which change without the definition changing.

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

1. Before the first turn, the `list:` job runs and returns `[{ name, does, args }, …]`.
2. The model is shown each tool under the entry's name and its own, joined by `_`: `github_create_issue`.
3. A call runs the entry's `job:`, where `${tool}` is the tool's own name, `create_issue`.

- The list is made once in a run, kept in the agent's record, and is the same for every turn.
- Every step of the agent after its first is taken in a process of its own, since a list's shapes come from outside the definition (see Controller).
- Every listed tool has the entry's permissions and, if the entry's job is marked for approval, its approval. The ask is shown the name the model used, the arguments, the entry's name as `entry` and the tool's own name as `tool`.
- The model can call every tool the list gives. The `list:` script narrows that by returning fewer.
- What a tool does and its shape are shown to the model as the list gives them.
- In a name the model is shown, a character other than a letter, a digit, `_` or `-` is written `_`.
- The entry's `job:` reads `${args}` only whole, since its fields are not known until the run.

A list that fails fails the agent, and so does a list in which a tool has no name, a shape that is not a JSON Schema of an object, a shape that marks a blob, or a name the model is shown that another tool has or that is longer than 64 characters.

## Parts

A part is a group of tools the model is not shown until it opens the part. An agent with many tools shows the model the ones it has opened.

```yaml
agent: helper
input: { said: string }
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.said}
open:
  net: { include: parts/net/part.yaml }
  payments: { include: parts/payments/part.yaml, approve: true }
```

```yaml
# parts/net/part.yaml
does: The firewall and the switch, and the network's docs.
ask: { machine: ./approval.yaml }
tools:
  show_status:
    does: Run one read-only status command.
    args: { device: string, command: string }
    job: { script: ./show-status.js, input: { device: "${args.device}", command: "${args.command}" }, allow: [net:192.0.2.1:22] }
```

1. The model is shown one more tool, `open`, with the name and `does:` of each part not yet open.
2. The model calls `open` with a part's name, and a job named `open` runs.
3. From the next turn, the model is shown that part's tools as well.

| Field | What it is |
|---|---|
| `does:` | what the part is for, which the model reads before it opens it |
| `tools:` | the part's tools, written as an agent's are |
| `ask:` | the ask for the part's tools and for opening it |
| `approve:` | `true` holds the part's open for the ask |
| `meta:` | data Corbie keeps and never reads |

- A part's tools have their own permissions, as every tool has. Opening a part gives the model names and nothing else.
- A tool of a part may not take the name of another tool of the agent.
- `open` is supplied by Corbie, as the blob tools are, and no tool may take its name. It is not shown while no part can be opened.
- The list of an entry with `list:` in a part is made before the first turn, as any list is. An error names it by where it is written, as `open.net.tools.files.list`.
- A part marked `approve: true` holds its `open` job for the ask, which is shown the part's name as `part` and the part's `does:`. An open that is not approved goes back to the model as denied, with the ask's note, and opens nothing. Only an approved open opens such a part.
- For a call of a part's tool, and for the list of an entry in a part, the ask is shown the part's name as `part`.
- A part of an agent made while running cannot name an ask (see Machines).
- A part may name an ask. It is the ask for the jobs and lists of the part's tools and for opening the part, before the agent's own, and the nearest ask above every machine below those tools' jobs. Its permissions are its own, as any ask's are, and its expressions read the agent's input. Like any ask, it is filled in and started once for the held calls of one turn, and one that cannot be filled in denies them.

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

An agent started with it carries on where the other stopped. Every name goes on meaning the blob it meant, and a blob the conversation has not shown takes a name it has not used. It opens no part: the model calls `open` for each part it needs, and a part marked `approve: true` waits for the ask again.

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

An agent acts on the results of its turns, its tools and its ask, and on nothing else. Events sent down to it, and events its tools send up while they run, are ignored; a watcher still sees them in the record.
