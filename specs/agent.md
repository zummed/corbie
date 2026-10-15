# Agent

An agent is a machine whose decisions are made by a model. The model is asked what to do, the tools it picks are run, the results go back to it, and this repeats until it answers without asking for a tool.

Corbie itself knows nothing about models. A turn is an ordinary job and so is a tool, and an agent is held to its permissions and its ask like any machine.

```yaml
agent: librarian
input: [dir, question]
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

Each turn is one call to the model, made by a job. The model sees the system text, the conversation so far, and for each tool its name, what it does and what arguments it takes. It never sees how a tool is run or what the tool is permitted. The tools it picks in one turn all run at once, and the next turn starts when every one of them has settled. A turn that fails fails the agent.

Corbie supplies the script that makes a turn. A definition may put its own in its place, and the supplied one can be called from it, which is how an agent's behaviour is wrapped or changed. There is no other hook.

An agent's result is the model's last reply as text, or as data in the shape the definition gives. If the last reply does not fit the shape, or was cut short by the model's length limit, the model is told and takes another turn. The result always carries the conversation and a count of the tokens the agent's own turns used, and so does its error if it fails.

An agent may be started with the conversation of an earlier agent's result, and carries on from it.

## Tools

A tool is a named job, a script or a machine, with a description, the shape of its arguments, and its own permissions. The job reads the agent's input and the arguments the model supplied. A tool whose job is an agent is how an agent has sub-agents: the model is shown what the sub-agent replied, and the sub-agent's conversation stays in its own record.

A definition may name files of tools shared with other agents. Each file is filled in when the definition is sent.

A tool that needs approval goes to the agent's ask like any job. The calls from one turn that need it go together, and the ask is shown the arguments, what the tool does and what the model said. A call that needs no approval runs at once, and the next turn waits for the ask as well as for the tools. A call that is not approved goes back to the model as denied, with the ask's note.

## Limits

An agent has a number of turns, twenty by default, and the model is told when few remain. At the limit the agent fails. Its error carries the conversation, so an agent started with that conversation carries on from where it stopped.

## The conversation

The conversation is everything the model has been shown and has said: the prompt, each reply and tool call, and each call's result, error or denial. It is kept as its turns, each written once as the turn ends. Nothing is rewritten as the conversation grows.

In a result the conversation is one value: its turns, and the blobs the model was shown, each under the name the model knows it by.

```
conversation: { turns: [ turn 1, turn 2, … ], blobs: { blob1: …, blob2: … } }
```

An agent started with it carries on where the other stopped. Every name goes on meaning the blob it meant, and a blob the conversation has not shown takes a name it has not used.

## Blobs and the model

A blob is shown to the model as a handle: a short name, its type and size, how many items it has, roughly what it would cost to read, and, the first time it is shown, a preview that depends on its type.

```
{ blob: "blob2", type: "text/csv", size: 48112033, items: 310224,
  cost: "about 12028008 tokens to read whole, 39 an item",
  preview: [ { date: "2026-01-02", total: "19.50" }, … ] }
```

The first time a blob is to be shown, a job looks at it, between the tool's result and the next turn: it counts the items and takes the first few as the preview. The preview is part of what the model is told in that turn, and stays in the conversation as anything it was told does. It is kept nowhere else, and a blob shown again is shown without it.

A blob of a type the model's service takes as content, an image or a document, is also sent to the model as it is, beside its handle, from the turn it is first shown. Any other blob is only its handle, and the model works on it with its tools.

The model uses the handle's name to refer to the blob. The agent turns names into references before a tool is run, and references into handles before the model sees anything. A blob's key never reaches a model by any path, including inside the contents of another blob.

Every agent has tools for working with blobs without reading them whole: one to read a range of items, one to search, and one to run a few lines of code over a blob and get back only the answer. They are ordinary scripts, allowed nothing from the definition's lists. A tool result too large to show is stored as a blob, by the job that makes its preview, and the model is given its handle.

## Events

An agent acts on the results of its turns, its tools and its ask, and on nothing else. Events sent down to it, and events its tools send up while they run, are ignored; a watcher still sees them in the record.
