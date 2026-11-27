# Agent

This area is the two types of machine that call a model: an agent, and a decision, which the last section gives.

An agent is a machine whose tools a model picks. The model is asked what to do, the tools it picks are run, the results go back to it, and this repeats until it answers without asking for a tool.

Corbie itself knows nothing about models. A turn is an ordinary job and so is a tool, and an agent is held to its permissions and its hooks like any machine.

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

An agent's `tools:` holds entries. An entry is a tool written in place, or a machine named by its file or by its location.

```
  description:, args:, job:    a tool written in place
  machine: ./file.yaml         a machine named by its file
  call: location               a machine named by its location
```

A machine that is a toolkit makes the entry a toolkit, which the model opens (see [Toolkits](#toolkits)). Any other machine makes the entry one tool.

A tool written in place is a named job, written with `script:`, `machine:`, `call:` or `send:`, with a description, the shape of its arguments, and its own permissions. The job reads the agent's input and the arguments the model supplied. A tool whose job is an agent is how an agent has sub-agents: the model is shown what the sub-agent replied, and the sub-agent's conversation stays in its own record.

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

## An entry that names a machine

An entry written `machine:` or `call:` takes its description from the machine it names, and has no `description:`, `args:` or `job:`.

| Field | What it is |
|---|---|
| `machine:` | the machine's file, which runs as a child run of the agent |
| `call:` | the machine's location, where it runs as a run of its own |
| `input:` | the names of the machine's input the entry fixes; it reads the agent's input |
| `approve:` | `true` marks every job the entry starts for approval |
| `meta:` | the `meta:` of every job the entry starts |
| `when:` | a condition on the agent's input: the entry is offered only if it holds |
| `open:` | for a toolkit, `true` opens it before the first turn |

```yaml
tools:
  read_file:   { call: gitlab/files/read,     input: { project: group/orders } }
  change_file: { call: gitlab/commits/create, input: { project: group/orders }, approve: true }
  summary:     { machine: ./summary.yaml }

# gitlab/files/read.yaml
flow: read
description: Read one file of a project.
input: { project: string, path: string, ref: "string?" }
```

A machine that is no toolkit is one tool, whose arguments are the machine's input without the names the entry fixes:

- The model is offered `read_file` as `Read one file of a project.`, with the arguments `path` and `ref`. A fixed name, or a name the machine's input does not give, does not fit.
- A call `{ path: a.txt }` starts a run of `gitlab/files/read` with `{ project: group/orders, path: a.txt }`. An argument the model leaves out is absent.
- A blob in the machine's input is an argument the model gives by the name it was shown. An input written as JSON Schema gives arguments only through `properties`.
- A machine that is an agent is shown to the model as its reply.

An entry written `call:` has `send:` for its location, and nothing else is written for it. The machine is read at one of two moments, and a leading `/` tells them apart:

```
call: gitlab/files/read     a location in the same tree, read by the client when it reads the tree
call: /netops/tools/show    a location in full, in any tree, read by the controller when the run starts
```

A file, or a location in the same tree, is read when the definition is. These are refused then:

- a location with no machine, or a file `.corbieignore` lists;
- a machine with no `description:`, or at a location, one that is not `transient`;
- a fixed name the machine's input does not give, or `request`, which a toolkit is given by the agent;
- `open: true` on an entry whose machine is no toolkit;
- for one tool, a machine whose input is written as JSON Schema with a keyword other than `type`, `properties`, `required`, `additionalProperties`, `description` and `title`, such as `allOf` or `$ref`;
- a location in the same tree in a file read alone, which has no tree;
- an entry written `call:` in a machine made while running, which names no address.

A location in full is read as the run starts:

1. The stored definition holds the entry with its location, the input it fixes, `approve:`, `when:` and `send:` for the location, and no description or arguments.
2. At the run's first step the controller reads the machine recorded at the location: its `description:`, the shape of its `input:`, its type and its lifetime.
3. It gives them to the agent only where a call from the run would pass: every layer of the agent holds `send:` for the location, its tree's client's ceiling included, and the location's `receive:` accepts the agent's location.
4. The run keeps what it read, and the model is offered the entry as one in the same tree.

```yaml
# x/agents/support.yaml            location /x/agents/support
tools:
  show: { call: /netops/tools/show, input: { device: fw1 } }

# netops/tools/show.yaml           location /netops/tools/show, in the client netops's tree
flow: show
description: Run one show command on a device.
input: { device: string, command: string }
receive: [/x/agents/support]
```

- The model is offered `show` as `Run one show command on a device.`, with the argument `command`.
- When netops submits a changed `show.yaml`, a run that starts after it is offered the change, and a run already started is not.
- A run that carries a conversation on reads at its own start, and so does an agent that is a child or a tool's machine.
- A call the model makes is checked again, as any call is.

An entry that is offered, as its `when:` says, fails the agent at its start, naming the entry's place and the reason, where:

- nothing is recorded at the location;
- a layer of the agent refuses `send:` to it, with the refusal a call would get;
- its `receive:` does not accept the agent's location;
- the machine there has what is refused above for a location in the same tree.

An entry that is not offered fails nothing.

## Hooks and a tool's call

A tool's call takes the agent's hooks like any job (see [Machines](machine.md#hooks)), and one marked for approval starts only after a `before` hook. A `call:` or `send:` job is marked when `approve:` names its `send:` entry. A hook reads the call as `job.tool`:

| Field | What it is |
|---|---|
| `name` | the tool's own name; an open has none |
| `args` | the arguments the model gave |
| `said` | what the model said in the turn that made the call |
| `toolkit` | for an open and for a call of a toolkit's tool, the name of the agent's entry |
| `open` | `true` for an open of a toolkit |
| `path` | for a toolkit inside a toolkit, the names that lead to it |
| `resume` | for a resume, the task's name, with `requests`, `responses` and `cancel` |

A hook runs once for each call. A call no hook holds runs at once, and the next turn waits for the held calls as well as for the others. A call a hook refuses goes back to the model as denied, with the hook's note.

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

## Toolkits

A toolkit is a machine that holds tools (see [Toolkit](toolkit.md)). An entry that names one gives the model its tools once the model opens it, so an agent with many tools shows the model the ones it has opened.

```yaml
agent: helper
input: { said: string, asker: any }
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
prompt: ${input.said}
tools:
  net:    { machine: ./toolkits/net.yaml, input: { asker: "${input.asker}" } }
  github: { call: tools/github, open: true }
  orders: { call: /orders/toolkits/orders, approve: true, when: "${input.asker.scopes.includes('orders')}" }
```

```yaml
# toolkits/net.yaml
toolkit: net
description: The firewall and the switch, and the network's docs.
input: { asker: any }
instructions: |
  Run show commands only. ${input.asker.name} is asking.
tools:
  show_status:
    description: Run one read-only status command.
    args: { device: string, command: string }
    job: { script: ./show-status.js, input: { device: "${args.device}", command: "${args.command}" }, allow: [net:192.0.2.1:22] }
```

1. The model is shown one more tool, `open`, with the list, the name and the description of each toolkit it can open.
2. The model calls `open` with `{ list: toolkits, name: net }`, and a job named `open` runs: the entry's job, with the request `{ do: open }`.
3. The model is shown the toolkit's instructions as the result of `open`.
4. From the next turn, the model is shown the toolkit's tools, each under the entry's name and its own: `net_show_status`.
5. A call of `net_show_status` is the entry's job, with the request `{ do: call, tool: show_status, args }`. The model is shown the `result` of the toolkit's answer.

| The entry is written | Its open and its calls run | With whose hooks and secrets |
|---|---|---|
| `machine: ./file.yaml` | as child runs of the agent, at the agent's location | the agent's, and the file's own hooks |
| `call: location` | as runs at that location | that location's |

- The entry's `input:` is the toolkit's input, with `request` added by the agent. A toolkit at another location is recorded in that tree, so the agent passes on only what its entry writes.
- The toolkit's `description:` is the machine's, which the model reads before it opens it.
- The toolkit checks a call's arguments against the tool's shape. Arguments that do not fit fail the toolkit's run, and go back to the model as an error. The agent gives the arguments as the model gave them, and turns no blob's name into the blob.
- In a name the model is shown, a character other than a letter, a digit, `_` or `-` is written `_`.
- An open whose list gives a tool that would be shown under the name of another tool, or under a name longer than 64 characters, goes back to the model as an error, and opens nothing.
- The result of `open` is shown to the model whole, and is never stored as a blob. With no instructions, it is `Opened net.`.
- `open` and `resume` are supplied by Corbie, and no entry may take the name `open`, `toolkits` or `resume`. `open` is not shown while no toolkit can be opened.
- An entry with `open: true` is opened before the first turn, by a job named `open`. The first turn is offered its tools and is shown its instructions. A toolkit that fails to open then fails the agent.
- `approve: true` marks the open and every call through the entry for approval. A call a hook refuses goes back to the model as denied, with the hook's note, and a refused open opens nothing.
- The agent's hooks run for the open and for each call, which are the agent's jobs. A toolkit's own hooks run for its tools' jobs, in its run (see [Toolkit](toolkit.md#authority)).
- An answer with `working` or `requests` makes the call a task (see [A call that has not ended](#a-call-that-has-not-ended)). Any other answer with no `result` goes back to the model as an error.

### Lists

Every list the model chooses from is a list of entries: the agent's own toolkits, in the list `toolkits`, and the tools and toolkits an open toolkit gives, in a list named as the toolkit is. A list small enough to show is shown: `open` names each toolkit of it, and its tools are offered. A list whose JSON is larger than 32 KiB, the limit for a tool's result, is a blob.

1. A job named `preview` stores the list as a blob of entries, `{ kind, name, description, args }`.
2. The model is shown the blob's handle: for the list `toolkits` before the first turn, and for a toolkit's list in the result of its `open`, with its instructions.
3. The model searches the blob with the blob tools.
4. The model calls `open` with the list and an entry's name. A toolkit is opened as any is. A tool is offered from the next turn, and no job runs for it.

- `open` names a list that is a blob with its blob, and not its entries.
- A tool of such a list that the model has not opened is not offered, and a call of it goes back to the model as an error.
- A list that could not be stored is shown as a short one is.

A toolkit's list may hold a toolkit, as well as tools. The model opens it from the list of the toolkit that gave it:

```
open { list: github, name: review }     the request is { do: open, toolkit: [review] }
github_review_read                      the request is { do: call, toolkit: [review], tool: read, args }
```

## A call that has not ended

A toolkit answers a call with `{ working, state }` where the work goes on, such as a task of an MCP server, or with `{ requests, state }` where it needs input. The call is then a task.

1. The agent keeps the state under a name, `task1`, and the model is shown `{ task: task1, working }` or `{ task: task1, requests }` as the call's result.
2. The model is offered one more tool, `resume`, which names each task. It goes on with other tools.
3. The model calls `resume` with `{ task: task1 }`, and with `responses`, by the names of the requests, where the call asked for input.
4. A job named `resume` runs: the entry's job, with the request `{ do: resume, tool, args, state, responses }`. The toolkit waits for the work (see [Toolkit](toolkit.md#a-resume-waits)).
5. The model is shown the result, and the task is gone. An answer of `working` or `requests` again keeps the task, with its new state.

- A resume is the call carried on. Its job has the entry's `approve:` and `meta:`, and its `job.tool` is the call's `{ toolkit, name, args }`, with what the model `said` in the turn that resumed it.
- A hook tells a resume by `job.tool.resume`, the task's name, and reads `job.tool.requests` and `job.tool.responses`. It gives a person's answer in place of the model's with `{ ok: true, input }`, the job's input with other `responses` in its `request`.
- `resume` with `cancel: true` asks the toolkit to cancel, with `{ do: cancel }`, and the task is gone. `job.tool.cancel` is `true`.
- A resume a hook refuses goes back to the model as denied, and keeps the task. A resume that fails for any other reason ends the task.
- Two calls of `resume` in one turn for one task run one job, and the second goes back to the model as an error.
- `resume` is not shown while the run has no task.
- Cancelling an agent cancels no work a toolkit started elsewhere.

```yaml
hooks:
  person:
    when: "${job.tool && job.tool.resume && job.tool.requests}"
    before: { machine: ./ask-a-person.yaml, input: { job: "${job}" } }
```

## Who is offered what

A `when:` on an entry is a condition on the agent's input. It is checked once, when the run starts, and the run keeps what it found: every turn is offered the same entries.

- `open` does not name a toolkit whose `when:` does not hold. A call of `open` with its name goes back to the model as an error.
- A tool whose `when:` does not hold is not shown to the model. A call of it runs nothing and goes back to the model as an error.
- A toolkit with `open: true` whose `when:` does not hold is not opened.
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

In a result the conversation is one value: its turns, the blobs the model was shown, each under the name the model knows it by, the lists of the toolkits that are open, and the tasks that have not ended. With none, `opened` or `tasks` is left out.

```
conversation: { turns: [ turn 1, turn 2, … ], blobs: { blob1: …, blob2: … },
                opened: { net: { entry, entries, blob, loaded } }, tasks: { task1: { entry, tool, args, state } } }
```

An agent started with it carries on where the other stopped. Every name goes on meaning the blob it meant, and a blob the conversation has not shown takes a name it has not used.

A run that carries a conversation on runs nothing for it at its start. The conversation is input, written by whoever starts the run, so the run checks each list in `opened` against its own definition, and keeps one only where a call of `open` that no hook holds would open it:

- the list's `entry` is an entry the definition has, as a toolkit;
- that entry's `when:` holds on the run's own input;
- that entry is not marked `approve: true`;
- a list that is a blob names a blob the conversation holds.

A list that is kept is open from the first turn, as the model was shown it: its tools are offered, `open` does not name it, and its instructions are in the conversation and are not given again. A toolkit with `open: true` that is kept is not opened again. Any other list is passed over, and the model opens that toolkit with `open`.

- Every call is the entry's job, which its hooks hold and the toolkit checks at its location. A kept list that misdescribes a tool misleads the model, and the toolkit refuses the call.
- The run keeps each task whose entry it has, as a toolkit that is offered, and the model resumes it by its name. A task's state is the claim of whoever wrote the conversation.
- The list of the agent's own toolkits is made again in each run.

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

An agent acts on the results of its turns and its tools, and on nothing else. A send to an agent run by its address is noted as undelivered in the sender's record, and one to a transient agent's location starts a new run of it, with the event's data as its input. Events sent down to it, and events its tools send up while they run, are ignored; a watcher still sees them in the record.

## The record

An agent's record holds these entries, under these names:

| Entry | What it is |
|---|---|
| job `turn`, `turn#2`, … | one turn, whatever script makes it; its input names the model as `model.name` |
| emit `text` of a turn | a piece of the model's reply, as `llm.chat` gives it |
| `done` from a turn | `{ turn, text, calls: [{ id, name, args }], stop, usage: { input, output, cached } }` |
| job named after a tool, `shelf`, `shelf.1` | one call of the tool the model named; its `done` or `failed` goes back to the model |
| job named after a toolkit's tool, `net_show_status` | one call of that tool: the entry's job, with the request |
| job `open`, `open.1` | an open of a toolkit: the entry's job, with the request |
| job `resume`, `resume.1` | a resume or a cancel of a task: the entry's job, with the request |
| job `preview`, `preview.1` | the preview of a blob before the model is first shown it, or a list stored as a blob |

`stop` is `end`, `tools` or `length`.

## Decisions

A decision is the second type of machine that calls a model. It puts named questions with fixed answers to a decision model, and its output is the probability of each answer (see [Models](model.md#decide)). It has no conversation, no tools and no loop.

```yaml
# urgent.yaml
decision: urgent
input: { text: string }
model: { name: jev-latest, secret: TYPESAFE_KEY }
state: ${input.text}
questions:
  urgent: { type: noul, instructions: The customer cannot use the product. }
```

1. At its start the decision fills in `state:` and `questions:` from its input.
2. One job, named `decide`, calls `llm.decide` with them. Corbie supplies its script.
3. The job's output is the decision's output, `{ answers, model, usage }`, and the job's failure is the decision's.

```
  model       the model, written as an agent's is, of a service that has decide; it holds name, secret, service and address
  state       the text or data the questions are about
  questions   each question under its name, as the model's service takes it
```

A flow or a statechart runs a decision as a job and reads a probability in a condition:

```yaml
stages:
  triage:
    job: { machine: ./urgent.yaml, input: { text: "${input.text}" } }
  page:
    when: ${stages.triage.output.answers.urgent.noul > 0.8}
    job: { script: ./page.js }
```

- The client writes into the `decide` job's permissions the connection to the model's service and the key bound to that connection, as it does for an agent's turn. The job gets nothing else from the definition's permissions.
- Corbie does not read the questions or the answers.
- A decision ignores every event but its start and its job's end. A send to a decision's run is noted as undelivered, as one to an agent's is.

