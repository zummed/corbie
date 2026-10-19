# Agent

The detailed rules behind the [Agent overview](../agent.md). Area code: `agent`. What an agent shares with every machine type (values, jobs and their names, events, children, approval, attempts, limits) is under Machines and is not repeated here. The parity cases for agents replace the model with a script of the definition's own, so none of them reaches a model.

## The loop

**R-agent-01** An agent's definition names its model, and may give a system text, a prompt, tools, limits and the shape of its reply.
*Accept:* an agent written without a model is refused when checked. An agent written with only a model and a prompt is accepted and runs; parity case 84 shows a system text and a prompt reaching the first turn.

**R-agent-03** Each turn is one call to the model, made by a job, and the loop repeats until the model answers without asking for a tool.
*Accept:* parity case 84: one reply, one job, named `turn`, and the agent ends after it. Parity case 85 ends at the third turn, the first that asks for no tool.

**R-agent-04** The model sees the system text, the conversation so far, and for each tool its name, what it does and what arguments it takes; it never sees how a tool is run or what the tool is permitted.
*Accept:* for the librarian, the first turn's job is given the system text, a conversation holding the prompt, and the tool `shelf` with its description and the shape of its arguments, and nothing it is given mentions the tool's script, the input its job is written with, or its allow list.

**R-agent-07** The tools the model picks in one turn all run at once, and the next turn starts when every one of them has settled.
*Accept:* parity case 85: the three jobs of the first turn are asked for in one step, and the second turn is asked for only after the third result has arrived.

**R-agent-09** Corbie supplies the script that makes a turn; a definition may put a script of its own in its place, and the supplied one can be called from it.
*Accept:* an agent written with no turn script of its own, run against a stand-in for its model service, makes its turns and ends with the stand-in's reply. Parity case 84, with a turn script of the definition's own. A definition's turn script calls the supplied one and puts the reply's text in capitals; the agent's result and its conversation hold the text in capitals.

**R-agent-12** An agent's result is the model's last reply as text, or as data in the shape the definition gives.
*Accept:* parity case 84. An agent whose shape is a title and a number of pages, and whose model's last reply is data that fits, has a result holding the title and the number as data.

**R-agent-14** If the last reply does not fit the shape, or was cut short by the model's length limit, the model is told and takes another turn.
*Accept:* a scripted model's first reply lacks a field and its second fits; the agent takes two turns, the second turn's conversation says what was wrong with the first reply, and the result is the second reply. A turn returns a reply with no tool calls that stopped for want of room; the agent does not end, the next turn's conversation says the reply was cut short, and the agent ends on the next complete reply.

**R-agent-16** An agent's result always carries the conversation and a count of the tokens its own turns used, and so does its error if it fails.
*Accept:* parity case 84: the result carries the conversation. Parity case 91: the lead's count is the sum of its own two turns and leaves out its helper's. Parity case 90: the agent that fails at its turn limit hands its parent the conversation and the count beside the error.

**R-agent-19** An agent may be started with the conversation of an earlier agent's result, its turns and the blobs they name, and carries on from it.
*Accept:* one agent ends, and a second is started with the first one's conversation and a new prompt; the second's first turn is shown the first one's turns followed by the new prompt, and the second's conversation begins with them.

**R-agent-20** A turn that fails fails the agent, as a failure a machine has no rule for does (see Machines).
*Accept:* parity case 86: the second agent's first turn fails, and the agent fails with an error that names the turn.

**C-agent-01** The next turn waits for the ask as well as for the tools. A turn started while an approval is open shows the model a conversation with a call unanswered. *Test:* `agent_next_turn_waits_for_open_approval`

**C-agent-02** A last reply that is data may be wrapped in one fenced block, and data followed by anything else does not fit the shape. Models wrap data in fences, and prose after the data must not be dropped without a word. *Test:* `agent_shaped_reply_accepts_one_fence_and_refuses_trailing_text`

**C-agent-03** A reply cut short in the middle of asking for a tool runs no tool. Half a set of arguments would run a tool with arguments the model did not mean. *Test:* `agent_cut_short_call_runs_nothing`

## Tools

**R-agent-22** A tool is a named job, a script or a machine, with a description, the shape of its arguments, and its own permissions; its job reads the agent's input and the arguments the model supplied.
*Accept:* parity case 85: one tool that is a script, one that is a flow and one that is a chart. The librarian is started with the directory `/srv/library` and its model calls `shelf` with the shelf `B`; the tool's job is given that directory and that shelf as its input, and may read under `/srv/library` and nowhere else.

**R-agent-25** A definition may name files of tools shared with other agents, one or several, and each file is filled in when the definition is sent.
*Accept:* two agents name one file of tools; the model of each is offered the file's tools beside the agent's own. The stored definition of such an agent holds the file's tools written out, and changing the file afterwards changes nothing for a run already started. The package starts an agent from the directory `a/` that names `../shared/tools.yaml`, whose tools name `./wait.yaml` and `./run.js`: its stored definition names them by their paths from `a/`, `../shared/wait.yaml` and `../shared/run.js`. Two files of tools in different directories that each name `./run.js` each run their own.

**R-agent-54** A tool whose job is an agent is shown to the model as what that agent replied. The sub-agent's conversation and its count of tokens stay in its own record.
*Accept:* an agent calls a tool that is another agent; its next turn is shown that agent's reply as the tool's result, and neither its conversation nor its tokens.

**R-agent-27** A tool that fails goes back to the model as an error, with whatever output it still gave, and the agent carries on.
*Accept:* parity case 85: the third turn is shown both errors, and the agent ends well; in the result for the chart's call the model sees `reached gave_up` and the failed final state's output.

**R-agent-29** A tool that needs approval goes to the agent's ask like any job; the calls from one turn that need it go together, the ask is shown the arguments, what the tool does and what the model said, and for a listed tool the entry's name as `entry` and the tool's own name as `tool`; a call that is not approved goes back to the model as denied, with the ask's note.
*Accept:* parity case 89: the ask is run once, with both calls, is shown each call's arguments, description and the model's words, and the refused call reaches the model as denied with the note. An entry `github` lists `create_issue`; the ask for a call of `github_create_issue` is shown `entry: github` and `tool: create_issue`, and the ask for a call of a tool of the definition's own is shown neither.

**R-agent-33** A call that needs no approval is not held by one in the same turn that does.
*Accept:* a turn asks for one tool that is marked for approval and one that is not; the second's job is asked for in the same step as the ask.

**R-agent-34** A call to a tool the agent does not have, or with arguments that do not fit the tool's shape, runs nothing and goes back to the model as an error.
*Accept:* a scripted model calls `shelf` with a number for the shelf and calls a tool named `burn`; no job is sent for either, and the next turn is shown an error for each.

**C-agent-06** A tool's job that reads an argument that is not a key of its shape's `properties` is refused when the definition is checked, whatever else the shape allows. The model is never told of that argument, so the value would always be missing. *Test:* `agent_tool_reading_unnamed_argument_is_refused`

**R-agent-57** A tool entry may give a `list:` job in place of `does:` and `args:`. The job runs once, before the first turn, and returns the tools, each with a name, what it does and the shape of its arguments; the list is kept in the agent's record and is the same for every turn. The model is shown each tool under the entry's name and its own joined by `_`, and a call to one runs the entry's `job:` with `tool` the tool's own name.
*Accept:* an agent's entry `files` has a list script that returns `read` and `write`. The first turn is offered `files_read` and `files_write`, with the descriptions and shapes the list gave. The model calls `files_write`; the entry's job is given `write` as `${tool}` and the arguments sent, and has the entry's permissions. The agent takes three turns, and the record shows one job named `files`, before the first turn. A second run started with the first one's conversation runs the list again.

**R-agent-58** A list that fails fails the agent, and so does a list in which a tool has no name, a shape that is not a JSON Schema of an object, or a name the model would be shown that another tool has or that is longer than 64 characters. In that name, a character other than a letter, a digit, `_` or `-` is written `_`.
*Accept:* an agent whose list script throws fails, with an error that names the entry, and takes no turn. Lists that give a tool with no name, a tool whose shape has `type: string`, and a tool `read` under an entry `blob`, beside the agent's own `blob_read`, each fail the agent, naming the tool. A list giving `issues.create` under `github` is shown as `github_issues_create`, and a call to it gives the job `issues.create` as `${tool}`.

**C-agent-16** A listed tool's shape may not mark a blob, and a list with one that does fails the agent. A blob's reference holds its key, and the entry's job would send it to whoever wrote the list. *Test:* `agent_listed_shape_marking_a_blob_fails`

**C-agent-17** The `job:` of an entry with a `list:` reads `args` only whole; one that reads a field of it is refused when the definition is checked. The fields are not known until the run, so the value could be missing every time. *Test:* `agent_listed_job_reads_args_whole`

**C-agent-07** A tool may not take the name of a job the agent itself makes: `turn`, `preview` or one of the blob tools. The record names a job by what asked for it, and two askers under one name cannot be told apart. *Test:* `agent_tool_cannot_take_an_agents_own_job_name`

## Shapes

**R-agent-55** A tool's `args:` and an agent's `reply:` are shapes, whose short form refuses a name it does not give; the model is shown the full form as it is written, and what the model sends is checked against it.
*Accept:* an agent with `args: { shelf: string, from: number? }` and the same agent with that shape written in full give the same stored definition. A tool whose shape gives `labels` an `enum` and `assign` properties of its own is offered to the model with both, as written; the model calls it with a label outside the `enum`, no job is sent, and the next turn is shown an error naming `labels`. A shape with `$schema` naming draft-07 is checked by draft-07.

**R-agent-56** `format: blob` marks a blob at any depth of a shape.
*Accept:* a tool's shape takes `pages`, a list of objects each with a `scan` marked `format: blob`; the model calls it with two handles' names, and the tool's job is given a reference in each `scan`.

**C-agent-15** A `$ref` in a shape resolves only within that shape; one that names a file, an address or another document is refused when the definition is checked. The check runs on the client and again on the controller, and a `$ref` followed outward would make either one read a file or connect to an address that a definition chose. *Test:* `agent_shape_ref_outside_the_shape_is_refused`

## Limits

**R-agent-35** An agent has a number of turns, twenty by default, the model is told when few remain, and at the limit the agent fails.
*Accept:* an agent with no number of its own, whose scripted model asks for a tool every time, fails after its twentieth turn with `turn limit reached`, and is shown nothing about turns left at its first turn; an ask above it is not asked anything. Parity case 90: the agent is stopped at two with `turn limit reached`, its last turn is shown how many turns are left, and a second run started with its conversation carries on to the end.

## The conversation

**R-agent-40** The conversation is everything the model has been shown and has said: the prompt, each reply and tool call, and each call's result, error or denial.
*Accept:* parity case 89: the conversation in the agent's result holds the prompt, the reply with its two calls, the first call's result, the second call's denial with the ask's note, and the last reply.

**R-agent-41** The conversation is kept as its turns, each written once as the turn ends, and nothing is rewritten as it grows.
*Accept:* after an agent's third turn its record holds three pieces of conversation, one for each turn, and a watcher that has read the first two reads only the third. After turn ten, the pieces for turns one and two are byte for byte what they were when written.

**C-agent-09** The results of one turn's calls are shown to the model in the order the calls were made, whichever finished first. A step taken again must show the model the same conversation, and the order of finishing is not fixed. *Test:* `agent_results_shown_in_call_order`

## Blobs and the model

**R-agent-43** A blob is shown to the model as a handle: a short name, its type and size, how many items it has, roughly what it would cost to read, and, the first time it is shown, a preview that depends on its type; a job makes the preview and counts the items, between the tool's result and the next turn, and a blob shown again is shown without its preview.
*Accept:* a tool returns a reference to a table of three hundred rows; the next turn is shown a short name for it, that it is a table, its size, three hundred items, a rough cost, and its first rows, and the record shows a job that made the preview, between the tool's result and the next turn. A later tool returns the same table: it is shown under the same name with its three hundred items and no preview, and no second job is run for it.

**R-agent-45** The model refers to a blob by its handle's name; the agent turns names into references before a tool is run, and references into handles before the model sees anything.
*Accept:* the model calls a tool with the handle's name as an argument that the tool's shape says is a blob; the tool's job is given the reference to that blob. An agent is started with a blob in its input and a prompt that mentions it, and a tool returns another; the first turn's prompt and the later tool result each show a handle where the reference was.

**R-agent-47** A blob's key never reaches a model by any path, including inside the contents of another blob.
*Accept:* an agent works with several blobs through its prompt, its tools' results, previews and its blob tools; the text of no blob's key appears in anything any turn's job was given. Blob A's contents hold a reference to blob B; the model reads A with the range tool, and again with the code tool returning A's contents as text; each time B is shown as a handle and its key is absent.

**R-agent-49** Every agent has tools for working with blobs without reading them whole: one to read a range of items, one to search, and one to run a few lines of code over a blob and get back only the answer; they are ordinary scripts, allowed nothing from the definition's lists.
*Accept:* an agent written with no tools; the first turn's job is offered those three tools. A call to the range tool is asked for as a job named for the tool and executed by a runner, like any other tool's.

**R-agent-51** A tool result too large to show is stored as a blob, by the job that makes its preview, and the model is given its handle.
*Accept:* a tool returns a hundred kilobytes of text; the next turn is shown a handle for it and not the text.

**R-agent-53** A blob of a type the model's service takes as content is sent to the model as it is, beside its handle and under its name, in every call from the turn it is first shown; a blob of any other type is only its handle.
*Accept:* a tool returns a picture and a table to an agent whose service takes pictures: the next call to the model, and each after it, carries the picture after a line giving its handle's name, and the table only as its handle; the conversation that is kept holds neither's contents.

**C-agent-10** A handle's name always means the same blob and is never used for another, also in an agent that carries the conversation on. The model refers back to names from earlier turns. *Test:* `agent_handle_name_is_stable_and_never_reused`

**C-agent-11** A name is turned into a reference only where a tool's shape asks for a blob. Ordinary text from a model may happen to contain a handle's name. *Test:* `agent_name_becomes_reference_only_where_shape_asks`

**C-agent-12** The code the model writes for the code tool cannot reach the blob's reference or the agent's input. Either holds a key, and whatever the code returns goes to the model. *Test:* `agent_code_tool_hides_reference_from_model_code`

**C-agent-14** The blob tools get none of the lists written for the whole definition. The code tool runs what the model wrote, and the definition's list could let it read a file, connect, or send a secret, past what its own tools are allowed. *Test:* `agent_blob_tools_get_none_of_the_definitions_permissions`

**C-agent-13** A preview is searched for references like any other value shown to the model. A preview is made from a blob's contents, which may hold another blob's reference. *Test:* `agent_preview_shows_no_key`

## Events

**R-agent-52** An agent acts on the results of its turns, its tools and its ask, and on nothing else: events sent down to it, and events its tools send up while they run, are ignored, and a watcher still sees them in the record.
*Accept:* an agent's record shows a step asking for a new job only after a result from a turn, a tool or the ask. Parity case 87 (sent down) and parity case 103 (sent up): the event is in the agent's record, from its sender, and the agent did nothing with it.

## Interfaces

Uses:

- **Machines.** Values and expressions, jobs and their names, events, children, approval, failure and attempts, and limits. An agent adds `args` to what an expression in a tool's job reads, and `tool` in the job of an entry with a list.
- **Models.** A turn is a call to a model through the `llm` plugin: the conversation, the tools on offer and the shape wanted go in, every shape in full form; what the model said, the tools it asked for, why it stopped and the tokens used come back. Why it stopped is how a reply cut short is told from one that was finished. What a turn is permitted, the connection and the key, is written by the client when the definition is sent (R-model-15).
- **Controller.** Takes the agent's steps, starts each turn and each tool as a job, and holds the calls that need approval for the ask.
- **Runner.** Executes the jobs of turns, lists, tools, the blob tools and previews.
- **Blobs.** A reference stands for a blob everywhere but in front of the model, where a handle stands for it; a large result is a blob in the agent's record, and a preview is in the turn that first shows its handle.
- **Plugins.** The supplied turn script and the blob tools are libraries that come with Corbie.
- **Client.** Fills in a shared file of tools when the definition is sent.

Used by:

- **Machines.** An agent is one of the three types, and supplies the filling-out of its short forms, its check, its step, what a watcher is shown of its state, and its manual page.
- **Flow, Statechart.** Either may run an agent as a child, and an agent may run either, or another agent, as a tool.
