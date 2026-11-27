# Agent

The detailed rules behind the [Agent overview](../agent.md). Area code: `agent`. What an agent shares with every machine type (values, jobs and their names, events, children, hooks, attempts, limits) is under Machines and is not repeated here. The parity cases for agents replace the model with a script of the definition's own, so none of them reaches a model.

## The loop

**R-agent-01** An agent's definition names its model, and may give a system text, a prompt, tools, limits and the shape of its reply.
*Accept:* an agent written without a model is refused when checked. An agent written with only a model and a prompt is accepted and runs; parity case 84 shows a system text and a prompt reaching the first turn.

**R-agent-03** Each turn is one call to the model, made by a job, and the loop repeats until the model answers without asking for a tool.
*Accept:* parity case 84: one reply, one job, named `turn`, and the agent ends after it. Parity case 85 ends at the third turn, the first that asks for no tool.

**R-agent-04** The model sees the system text, the conversation so far, and for each tool its name, its description and what arguments it takes; it never sees how a tool is run or what the tool is permitted.
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

**R-agent-19** An agent may be started with the conversation of an earlier agent's result, its turns and the blobs they name, and carries on from it. The conversation names under `opened` the list of each toolkit that was open, and under `tasks` each task that has not ended. A run started with it runs nothing for them at its start: a list it keeps is open from the first turn as the model was shown it, its instructions are not given again, and a toolkit with `open: true` that is kept is not opened again. It keeps each task whose entry it has, as a toolkit that is offered.
*Accept:* one agent ends, and a second is started with the first one's conversation and a new prompt; the second's first turn is shown the first one's turns followed by the new prompt, and the second's conversation begins with them. `TestAgentToolkitsAreOpenedByTheModel`: the conversation of an agent that opened two toolkits holds `turns`, `blobs` and `opened`, and a second run started with it has no `open` job, is offered `net_show` and `payments_pay` and no `open` at its first turn, holds `net`'s instructions once, and runs `net_show`. `TestAgentResumesACallThatHasNotEnded`: an agent that ends with `task1` working gives `tasks: { task1: { entry, tool, args, state } }`, and a second run started with it resumes `task1` to its result.

**C-agent-31** A carried conversation opens only what a call of `open` that no hook holds would open: a list whose entry the definition has as a toolkit, whose `when:` holds and that is not marked `approve: true`. Any other name in `opened` is passed over. The conversation is input, written by whoever starts the run; a kept list that misdescribes a tool only misleads the model, since the toolkit checks every call. *Test:* `agent_conversation_opens_only_what_open_would`

**R-agent-20** A turn that fails fails the agent, as a failure a machine has no rule for does (see Machines).
*Accept:* parity case 86: the second agent's first turn fails, and the agent fails with an error that names the turn.

**C-agent-01** The next turn waits for the calls held for a hook as well as for the others. A turn started while a call is held shows the model a conversation with a call unanswered. *Test:* `agent_next_turn_waits_for_open_approval`

**C-agent-02** A last reply that is data may be wrapped in one fenced block, and data followed by anything else does not fit the shape. Models wrap data in fences, and prose after the data must not be dropped without a word. *Test:* `agent_shaped_reply_accepts_one_fence_and_refuses_trailing_text`

**C-agent-03** A reply cut short in the middle of asking for a tool runs no tool. Half a set of arguments would run a tool with arguments the model did not mean. *Test:* `agent_cut_short_call_runs_nothing`

## Tools

**R-agent-22** An agent's `tools:` holds entries: a tool written in place, or a machine named by its file with `machine:` or by its location with `call:`. A tool written in place is a named job, a script, a machine, a call or a send, with a description, the shape of its arguments, and its own permissions; its job reads the agent's input and the arguments the model supplied. A tool's `description:` is plain text, and an expression in it is refused.
*Accept:* parity case 85: one tool that is a script, one that is a flow and one that is a chart. The librarian is started with the directory `/srv/library` and its model calls `shelf` with the shelf `B`; the tool's job is given that directory and that shelf as its input, and may read under `/srv/library` and nowhere else. `Test_agent_tool_that_sends_is_done_once_written`: a tool whose job is a send ends `done` with no output, its event starts the keyed run, and the model's next turn is shown an empty result.

**R-agent-54** A tool whose job is an agent is shown to the model as what that agent replied. The sub-agent's conversation and its count of tokens stay in its own record.
*Accept:* an agent calls a tool that is another agent; its next turn is shown that agent's reply as the tool's result, and neither its conversation nor its tokens.

**R-agent-64** A tool whose job is a call is shown to the model as the whole of the called run's result, or as its error when the run fails. The called run's record belongs to its own tree's client.
*Accept:* `Test_agent_tool_that_calls_shows_the_whole_result`: the model is shown the called chart's whole output, and with the chart ending failed, its error with that output.

**R-agent-65** An entry written `machine:` or `call:` takes its description from the machine it names. A machine that is no toolkit makes it one tool, whose arguments are that machine's input without the names the entry's `input:` fixes; the machine is started with the fixed input and the model's arguments. A location with no machine, a file `.corbieignore` lists, a machine with no `description:`, a machine at a location that is not transient, a fixed name the machine's input does not give, `open: true` where the machine is no toolkit, and, for one tool, a machine whose input is written as JSON Schema with a keyword other than `type`, `properties`, `required`, `additionalProperties`, `description` and `title` (such as `allOf` or `$ref`) are refused when the definition is read.
*Accept:* `TestAgentToolCallsAMachineOfTheTree`: the model is offered `read_file` with the flow's description and its input without `project`, and the flow is given `project` with the model's `path`. `TestAgentCallToolIsRefused`: each of the refusals names the entry's place. `TestAgentToolCallsAnAgent`: a machine that is an agent is shown as its reply. `TestAgentEntryNamesAMachineByItsFile`: an entry `sum: { machine: ./sum.yaml, input: { by: 2 } }` is offered with the flow's description and the argument `n`, and its call runs the flow as a child run with `{ by: 2, n: 5 }`. `TestAgentToolkitToolNamesAreTheirOwn`: `open: true` on an entry that names a flow is refused at `tools.sum.open`.

**R-agent-66** An entry written `machine:` or `call:` has no `description:`, `args:` or `job:`, and names its machine one way. One written `call:` has `send:` for its location and nothing else is written for it. It is refused in a machine made while running, and one with a location in the same tree is refused in a file read alone, which has no tree.
*Accept:* `TestAgentToolCallsAMachineOfTheTree`: an agent with no `allow:` calls its machines, and the call marked `approve: true` waits for the agent's hook, which reads the tool's name and the model's arguments. `Test_agent_call_tool_needs_its_tree`: both forms are refused in a machine made while running; a file read alone is refused its entry in the same tree and not its entry with a location in full; and `corbie test` names a location in full in the tree where nothing is. `TestAgentToolkitToolNamesAreTheirOwn`: an entry with `machine:` and `description:`, and one with `machine:` and `call:`, are refused.

**R-agent-92** An entry written `call:` with a location in full, from `/`, takes its description, its type and its arguments, as R-agent-65 gives them, from the machine recorded at that location when the run starts, and the run keeps them. They are given only where a call from the run would pass: every layer of the agent, its tree's client's ceiling included, holds `send:` for the location, and its `receive:` accepts the agent's location. An entry that is offered and cannot be read fails the agent at its start, naming the entry's place and the reason: nothing recorded there, the refusal a call would get, or what R-agent-65 refuses when the definition is read. An entry that is not offered fails nothing.
*Accept:* `TestAgentToolCallsAMachineOfAnotherTree`: with a second client's tree at `/netops`, the model is offered its machine's description and its input without the fixed `device`, and the call runs there with `device` given; the machine changed and submitted again is offered changed to the next run and unchanged to a run already started. `TestAgentToolOfAnotherTreeThatCannotBeRead`: a `receive:` that does not list the agent, a ceiling without `send:`, nothing recorded and a fixed name the input does not give each fail the agent at its start with the reason; a tool whose `when:` does not hold names a location with nothing recorded, and the agent ends `done`.

**C-agent-23** The arguments of an entry that is one tool are closed: a name the entry fixes, or one the machine's input does not give, from the model does not fit, and runs nothing. An argument the model leaves out is absent from the machine's input. A model could otherwise set what the entry fixes, which is how a tool is limited. *Test:* `agent_call_tool_arguments_are_closed`

**R-agent-27** A tool that fails goes back to the model as an error, with whatever output it still gave, and the agent carries on.
*Accept:* parity case 85: the third turn is shown both errors, and the agent ends well; in the result for the chart's call the model sees `reached gave_up` and the failed final state's output.

**R-agent-29** A tool's call takes the agent's hooks like any job, and a hook runs once for each call. A hook reads the call as `job.tool`: the tool's own name as `name`, the arguments as `args` and what the model `said`; for an open and for a call of a toolkit's tool, the entry's name as `toolkit`; for an open, `open: true` and no `name`; and for a toolkit inside a toolkit, the names that lead to it as `path`. A call a hook refuses goes back to the model as denied, with the hook's note.
*Accept:* parity case 89: the hook runs once for each of the two calls, reads each call's tool, arguments and the model's words, and the refused call reaches the model as denied with the note. `TestAgentHookReadsTheToolOfEachCall`: the hook for a call of `github_create_issue` reads `toolkit: github` and `name: create_issue`, the hook for the open reads `toolkit: github` and `open: true`, and the hook for a call of the definition's own tool `add` reads `name: add` and no toolkit. Parity case 158: the agent's hook reads `toolkit`, `name`, `args` and `said` for a call of a toolkit at a location.

**R-agent-33** A call no hook holds is not held by one in the same turn that is.
*Accept:* a turn asks for one tool that is marked for approval and one that is not; the second's job is asked for in the same step as the hook's job for the first.

**R-agent-34** A call to a tool the agent does not have, or with arguments that do not fit the tool's shape, runs nothing and goes back to the model as an error.
*Accept:* a scripted model calls `shelf` with a number for the shelf and calls a tool named `burn`; no job is sent for either, and the next turn is shown an error for each.

**C-agent-06** A tool's job that reads an argument that is not a key of its shape's `properties`, in its input, a call's location or a send's address or data, is refused when the definition is checked, whatever else the shape allows. The model is never told of that argument, so the value would always be missing. *Test:* `agent_tool_reading_unnamed_argument_is_refused`

**C-agent-07** An entry may not take the name of a job the agent itself makes, `turn`, `preview`, `open`, `resume` or one of the blob tools, or `toolkits`, the list `open` names the agent's toolkits by. The record names a job by what asked for it, and two askers under one name cannot be told apart. *Test:* `agent_tool_cannot_take_an_agents_own_job_name`

## Toolkits

**R-agent-59** An entry whose machine is a toolkit is a toolkit. The model is shown a tool `open`, with the list, the name and the description of each toolkit it can open; the agent's entries are in the list `toolkits`. A call of it runs a job named `open`, the entry's job with the request `{ do: open }`, and from the next turn the model is shown the toolkit's tools, each under the entry's name and its own joined by `_`. A call of one is the entry's job with the request `{ do: call, tool, args }`, and the model is shown the `result` of the answer. `open` is not shown while no toolkit can be opened.
*Accept:* `TestAgentToolkitsAreOpenedByTheModel`: an agent with the toolkits `net` and `payments` is first offered its own tool, the blob tools and `open`, whose description names both with their descriptions. The model opens `net`; the next turn is offered `net_show`, and `open` names only `payments`. The record shows the jobs `turn open turn#2 net_show turn#3 open#2 turn#4`, the call of `net_show` gives the tool's output, and with both open the model is offered no `open`.

**R-agent-60** An open whose list gives a tool that would be shown under the name of another tool, or under a name longer than 64 characters, goes back to the model as an error and opens nothing. In a name the model is shown, a character other than a letter, a digit, `_` or `-` is written `_`.
*Accept:* `TestAgentToolkitToolNamesAreTheirOwn`: an agent with a tool `net_show` of its own opens the toolkit `net`, whose tool is `show`; the open is shown as an error naming `net_show`, and `open` still names `net`.

**R-agent-62** An entry marked `approve: true` marks every job it starts for approval: the open of its toolkit and each call. An open a hook refuses goes back to the model as denied, with the hook's note, and opens nothing. An entry marked for approval needs a `before` hook, as a job marked for it does.
*Accept:* `TestAgentToolkitOpenWaitsForAHook`: with `net` marked, the hook says yes to the open only when it reads `toolkit: net` and `open: true`, and to the call of `net_show` only when it reads `name: show` and the arguments; the model is shown neither as denied. A hook that says no: the open goes back to the model as denied with the hook's note, and the next turn is not offered `net_show`. The same agent with no hook is refused when checked.

**R-agent-67** An entry may give `when:`, a condition on the agent's input, checked once when the run starts. An entry whose `when:` does not hold is not shown, its toolkit is not opened before the first turn, and a call or an open of it runs nothing and goes back to the model as an error. A `when:` that cannot be evaluated fails the agent at its start.
*Accept:* `TestAgentWhenSaysWhatIsOffered`: with neither `when:` holding, the record holds no `open` job, `open` names `net` and not `payments`, an open of `payments` goes back as an error, and a call of the tool `add` is answered `there is no tool named`. With the `when:` of `payments` holding, it is opened before the first turn. A `when:` that reads a field the input lacks fails the agent before its first turn, naming `tools.payments.when`.

**R-agent-68** A toolkit's instructions are the result of `open`, shown to the model whole. The toolkit fills them from the input its entry gives. With none, the result says the toolkit was opened.
*Accept:* `TestAgentOpenGivesTheInstructions`: instructions of 40 KiB, filled from the entry's `input:`, are shown as text and not as a blob's handle. `TestAgentToolkitsAreOpenedByTheModel`: `net`'s instructions read the input the entry gives, and `payments`, with none, gives `Opened payments.`.

**R-agent-93** A toolkit named `machine:` is opened and called as child runs of the agent, at the agent's location, and one named `call:` as runs at its location, which need `send:` for it and its `receive:`. The entry's `input:` is the toolkit's input, to which the agent adds `request`. A hook of the agent's runs for the open and for each call, and for no job in a run at another location.
*Accept:* parity case 158: an agent opens and calls a toolkit at a location with no `allow:` of its own; its hook holds the call of a refund over 500 and no other, and the toolkit's hook holds every refund in the toolkit's run, where it is given nothing the model said. `TestAgentToolCallsAMachineOfTheTree`: the toolkit at `kits/orders` is given the `team` its entry fixes. `TestAgentToolkitHooksAreItsOwn`: a hook written in a toolkit's file refuses its tool's job, and the model is shown the call as denied with the hook's note.

**R-agent-94** The toolkit checks a call's arguments. The agent gives them as the model gave them, with no name of a blob turned into the blob, and arguments that do not fit go back to the model as an error, as does an answer that holds neither a `result`, nor `working` or `requests` with a `state`.
*Accept:* `TestAgentToolkitChecksTheArguments`: a call of `net_show` with a number for `command` is shown as an error that names the tool and `command`, and the next call, which fits, gives the tool's output.

**R-agent-97** A call a toolkit answers with `{ working, state }` or `{ requests, state }` is a task: the agent keeps the state under a name, and the model is shown `{ task, working }` or `{ task, requests }`. The model is offered `resume`, which names each task, while there is one. A call of it runs a job named `resume`, the entry's job with the request `{ do: resume, tool, args, state, responses }`, or `{ do: cancel }` with `cancel: true`. A result ends the task, a cancel ends it, and `working` or `requests` again keeps it with its new state.
*Accept:* `TestAgentResumesACallThatHasNotEnded`: the call of `net_export` is shown as `task1` working, the next turn is offered `resume` naming `task1`, the resume gives the result of the third ask, the turn after is offered no `resume`, and a second resume of `task1` is an error; with `cancel: true` the tool's job is asked to cancel. `TestAgentResumesAnMCPServersTask`: a call an MCP server makes a task is shown as `task1` working, and the resume gives what the task exported.

**R-agent-98** A resume is the call carried on: its job has the entry's `approve:` and `meta:`, and its `job.tool` holds the call's `toolkit`, `name` and `args`, the task's name as `resume`, the call's `requests`, the `responses` the model gave, and `cancel`. A hook gives other responses with `{ ok: true, input }`. A resume a hook refuses keeps its task, and one that fails for any other reason ends it.
*Accept:* `TestAgentHookHoldsAResume`: the hook that holds the entry's jobs holds the resume of `net_choose`, reads `resume: task1`, the request's message and the model's `red`, and gives `blue` in the job's input; the model is shown `chose blue`.

**R-agent-95** A toolkit whose entry has `open: true` is opened before the first turn, by a job named `open`: the first turn is offered its tools and is shown its instructions. One that fails to open fails the agent, naming the entry.
*Accept:* `TestAgentToolkitOpenBeforeTheFirstTurn`: the record shows the jobs `open turn net_show turn#2`, the first turn is offered `net_show` and holds the instructions, and a toolkit whose job throws fails the agent at `tools.net` before any turn. `TestAgentWithTheToolsOfAnMCPServer`: a toolkit whose job is `server.toolkit(input.request)` gives the model the server's tools at its first turn.

**R-agent-96** A toolkit's list may hold a toolkit. `open` names it in the list of the toolkit that gave it, an open or a call for it is the same entry's job with the names that lead to it as the request's `toolkit`, and its tools are shown under that list's name and their own.
*Accept:* `TestAgentOpensAToolkitInsideAToolkit`: after the open of `net`, `open` names `{ list: net, name: review }`; the open of it gives its instructions, `net_review_read` is answered by the job given `toolkit: [review]`, and `net_find` by the job given none. A field of an entry other than `kind`, `name`, `description` and `args` is not offered to the model.

**R-agent-99** A list of entries whose JSON is larger than 32 KiB is a blob: a job named `preview` stores it, and the model is shown its handle, for the agent's own toolkits before the first turn and for a toolkit's list in the result of its `open`. `open` names the list with its blob. A call of `open` with the list and a tool's name offers that tool from the next turn and runs no job; a tool of such a list that was not opened is not offered.
*Accept:* `TestAgentListTooLargeToShowIsABlob`: the open of a toolkit with 80 tools is shown its instructions and the handle of a blob of 80 items, and the next turn is offered none of its tools; a search of the blob gives the entry `t42`, the open of it answers `Opened net_t42`, the turn after is offered `net_t42`, its call runs, and a call of `net_t7` is an error. With two toolkits whose descriptions are 20 KiB each, the first turn is shown the list `toolkits` as a blob of 2 items, `open` names the blob and no description, and the open of `net` by its name opens it.

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
*Accept:* parity case 89: the conversation in the agent's result holds the prompt, the reply with its two calls, the first call's result, the second call's denial with the hook's note, and the last reply.

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

**R-agent-52** An agent acts on the results of its turns and its tools, and on nothing else: events sent down to it, and events its tools send up while they run, are ignored, and a watcher still sees them in the record.
*Accept:* an agent's record shows a step asking for a new job only after a result from a turn or a tool. Parity case 87 (sent down) and parity case 103 (sent up): the event is in the agent's record, from its sender, and the agent did nothing with it.

## The record

**R-agent-70** An agent's record names its entries as the overview's table gives them: a turn is the job `turn`, whatever script makes it; `llm.chat` emits the reply's pieces under `text`; a turn's `done` holds `turn`, `text`, `calls`, `stop` and `usage`; and a tool call is a job named after the tool the model named.
*Accept:* an agent whose model calls `add` once and then answers is followed: its script jobs are `turn`, `add` and `turn#2`; `text` is emitted under `turn`; the turn's input names the model; the first turn's `done` holds one call, a `stop`, a `turn` and a `usage`, and the last holds the answer as `text` with `stop` `end`. *Test:* `Test_agent_record_names_its_entries`

## Decisions

**R-agent-71** A decision gives `model:`, `state:` and `questions:`. At its start it fills in `state:` and `questions:` from its input and asks for one job, named `decide`, which calls `llm.decide` with them by a script Corbie supplies. The job's output is the decision's output, and the job's failure fails the decision. A decision acts on no other event.
*Accept:* the manual page `decision`: a decision run alone ends `done` with `{ answers, model, usage }`, and a flow that runs it as a job reads `answers.urgent.noul` in a `when:`. A decision with no `state:` or no `questions:` is refused when checked. *Test:* `TestDecisionAsksOneJobAndEndsWithIt`

**R-agent-72** A decision's model is of a service that has `decide`, and holds `name`, `secret`, `service` and `address` and no other field. A model of a service that has only `chat` is refused when the definition is checked.
*Accept:* `model: { name: jev-latest }` is of `systemone` by its name. `model: { name: claude-sonnet-5-5 }` is refused, naming the services that have decide, and so is a model that writes `effort:`. *Test:* `TestDecisionModelHasDecide`

**R-agent-73** The `decide` job is written the connection to the model's service and the key bound to that connection, and gets nothing else from the definition's permissions.
*Accept:* a decision whose definition allows `disk` and names `secret: TYPESAFE_KEY` has a `decide` job that allows `net:api.typesafe.ai:443` and `secret:TYPESAFE_KEY@net:api.typesafe.ai:443` and no other entry. *Test:* `TestDecisionModelHasDecide`

## Interfaces

Uses:

- **Machines.** Values and expressions, jobs and their names, events, children, hooks, failure and attempts, and limits. An agent adds `args` to what an expression in a tool's job reads.
- **Toolkit.** An entry that names a toolkit puts its requests to it, `open`, `call`, `resume` and `cancel`, as jobs, and reads its answers.
- **Models.** A turn is a call to a model through the `llm` plugin: the conversation, the tools on offer and the shape wanted go in, every shape in full form; what the model said, the tools it asked for, why it stopped and the tokens used come back. Why it stopped is how a reply cut short is told from one that was finished. What a turn is permitted, the connection and the key, is written by the client when the definition is submitted (R-model-15).
- **Controller.** Takes the agent's steps, starts each turn and each tool as a job, a tool's send and call included, and runs the hooks before and after each tool's call.
- **Runner.** Executes the jobs of turns, tools, the blob tools and previews.
- **Blobs.** A reference stands for a blob everywhere but in front of the model, where a handle stands for it; a large result is a blob in the agent's record, and a preview is in the turn that first shows its handle.
- **Plugins.** The supplied turn script and the blob tools are libraries that come with Corbie.

Used by:

- **Machines.** An agent and a decision are two of the five types, and each supplies the filling-out of its short forms, its check, its step, the state a watcher is shown (R-inst-72), and its manual page.
- **Flow, Statechart.** Either may run an agent as a child, and an agent may run either, or another agent, as a tool.
