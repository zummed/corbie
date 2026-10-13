# Machines

The detailed rules behind the [Machines overview](../machine.md). Area code: `mach`. What is shared by every machine type is stated here once and is not repeated under Flow, Statechart or Agent. Attempts, timers and deadlines are the controller's and are under Controller; what a job may be held for is under Permissions.

## Steps

**R-mach-01** A step is given the definition, the state the machine was left in and the events that have arrived since, and returns the new state and a list of requests; it does no work itself.
*Accept:* a flow of one script stage and one child machine is started; the first step entry in its record holds the `start` event, the state after it and only the two requests, and the script's output and the child's first step appear in the record after it.

**R-mach-03** Steps of one instance happen one at a time and in order.
*Accept:* three results for one flow arrive together while two copies of the controller are running; the record's steps are numbered with no gap and no repeat, and each step's state follows from the step before it.

**R-mach-04** Taking the same step twice gives the same answer.
*Accept:* a step is taken twice from the same definition, state, events and step time; the two results are identical, including the names of the jobs, timers and keys asked for.

**R-mach-05** Everything a machine has done can be read back from its record.
*Accept:* for a finished run, giving each step's recorded events to the machine in order, starting from nothing, reproduces every recorded state and every recorded request.

**R-mach-06** A machine runs until it reaches an end of its own, is cancelled, or passes its deadline; one that is cancelled ends as cancelled and takes no further step, and one that is waiting costs nothing.
*Accept:* parity case 41: the root ends `cancelled` and its record holds no step after the cancel. A chart with no deadline waits on a key over a long stretch of simulated time with nothing arriving: no step is written for it and no job exists for it, and an answer at the end is taken as usual.

## Definitions

**R-mach-09** A definition's first line gives its type and its name.
*Accept:* a definition beginning `flow: digest` is a flow named `digest`; one whose first line names no known type is refused, and the refusal names the line.

**R-mach-10** A definition may list the inputs it needs, and an instance started without one of them fails at its first step, saying which, and starts nothing.
*Accept:* parity case 14.

**R-mach-12** When a client sends a definition, every short form in it is filled out into the one full form, and only the full form is stored and run; each short form has one meaning and one place it can be written.
*Accept:* a statechart sent with a transition written as a bare state name, and the same chart with that transition written out, give the same stored definition, and the definition shown for the run contains no short form. A short form written in a place other than its own is reported as a problem naming that place.

**R-mach-16** A definition is checked as a whole, and everything wrong with it is reported together.
*Accept:* a definition with an unknown field, an expression that does not compile and a reference to a stage that does not exist is refused once, with three problems, each naming its place.

**R-mach-17** The client checks a definition before sending it, and the controller checks it again with the same code and refuses what fails.
*Accept:* a definition with a mistake is submitted; the command fails listing the problems, and nothing has been written to the store. A trusted client writes a start carrying a full-form definition with two mistakes straight into the controller's inbox; no instance is made, and the receipt lists the same two problems, in the same words, that the client's own check gives for that definition.

**R-mach-19** An instance never changes its definition; a machine changes how it works by changing a child's definition and starting the child again.
*Accept:* the definition shown for a long-running chart is the same after its hundredth step as after its first, and no message a client can send alters it. A chart runs a child whose machine is given by an expression; a script hands back a newly made definition and the chart enters the state again; the new child runs the new definition and the chart's own definition is unchanged.

**R-mach-132** Each mapping a definition may hold has one shape, which gives its fields, the kind of value each takes, what each is for, the values it takes where they are few, and, where the mapping is one of several sorts, which fields each sort holds; a definition is read by its shapes, and a client is given the same shapes for a type.
*Accept:* a client asks what an agent's definition may hold and is given its first line and what every type takes, the agent's own fields, and the shapes of a model, a tool and a job, each once. A field added to a model's shape and to its reader is given to the client with nothing more written; added to one and not the other, it is the fault C-mach-23 refuses.

**C-mach-23** A reader that reads a field its shape does not name, reads one as another kind of value than its shape gives it, or leaves a field its shape names unread, is refused as a fault of Corbie's whenever a definition is read. A shape its reader could stray from would offer a person a field that is then refused, or leave one that is read unsaid. *Test:* `mach_reader_reads_exactly_what_its_shape_names`

**C-mach-04** A field its type does not know, a repeated key or a name in the wrong case is refused, in a written definition and in a stored one alike. A field quietly ignored is a mistake nobody is told about, and the controller does not take a client's word for what it stores. *Test:* `mach_unknown_or_repeated_field_is_refused`

**C-mach-06** YAML aliases may not grow a definition far beyond what was written, nor contain themselves. A few lines of YAML can otherwise fill memory. *Test:* `mach_alias_growth_is_refused`

**C-mach-08** A plain value that YAML would read as a date or a time stays the text that was written. An unquoted `2026-10-01` is otherwise changed before any expression sees it. *Test:* `mach_date_like_value_stays_text`

**C-mach-10** An expression is compiled when the definition is checked, not only parsed. Some mistakes pass a parse and would otherwise first show when a step runs. *Test:* `mach_check_compiles_every_expression`

**C-mach-11** A definition whose machine files lead back to itself is refused, and the refusal names the chain. Writing such a definition out in full never ends. *Test:* `mach_definition_including_itself_is_refused`

## Values

**R-mach-21** A value that is exactly one expression takes the expression's value, whatever its type; text with expressions inside it is text with the values filled in; everything else is literal.
*Accept:* `count: "${input.n}"` with the input `{ n: 3 }` gives the number 3, and with `{ n: [1, 2] }` gives the list. `"Look into ${input.topic}."` with the topic `the disk` gives `Look into the disk.` `n: 3`, `title: hello` and a nested map reach a job's input exactly as written.

**R-mach-24** Expressions are JavaScript, and an expression reads `input` and what its machine's type adds, and nothing else.
*Accept:* `${input.urls.filter(u => u.startsWith("https")).length}` gives the number of matching items. A flow's expression that reads `states`, and an expression that names something outside the language itself such as `require` or `process`, are each refused when the definition is checked.

**R-mach-27** An expression in a field that is a name is refused: a thing's name, a type, a permission's kind, a deny entry, a secret's name, a tag, a timer's key. The path of a script or a machine is a name too, except that it may be exactly one expression that gives a stored definition or script.
*Accept:* seven definitions, each with `${input.x}` in one of these fields, are each refused when checked, and each refusal names the field. `machine: "${input.definition}"`, given a stored definition, runs it as a child; `script: "./${input.name}.js"` is refused when checked.

**R-mach-29** An expression may run for one second.
*Accept:* a stage's input holds an expression that loops for ever; the instance fails after about a second with an error that names the expression's place.

**R-mach-30** The time an expression sees is the time of the step, taken from the step's inputs; random numbers differ from one expression to the next but are the same if the step is taken again.
*Accept:* two expressions in one step read the time and get the same value, and each asks for a random number and gets a different one. The step taken again, by another copy of the controller at a later moment, gives the same time and the same two numbers.

**C-mach-12** One expression cannot change what the next one sees. Anything shared that an expression could alter would make a step's answer depend on the order of evaluation. *Test:* `mach_expression_cannot_alter_shared_names`

**C-mach-13** A value an expression throws is turned into text within the expression's one second. Turning it into text runs the definition's own code, which could otherwise run for ever. *Test:* `mach_thrown_value_is_shown_within_the_limit`

**C-mach-14** Running out of time stops only the expression that ran out. A stop that arrives late must not end the next expression. *Test:* `mach_expression_timeout_stops_one_expression`

**C-mach-15** An expression that calls itself without end fails with an error. Unbounded depth ends the process, not the expression. *Test:* `mach_expression_depth_is_bounded`

## Jobs

**R-mach-33** A job that is a script is executed by a runner; a job that is a machine is started by the controller as a child run.
*Accept:* a flow with a script stage and a machine stage: the record shows one job sent to a runner and its result arriving as the first stage's `done`, and a new instance whose head names the flow as its parent, with nothing sent to a runner for the second stage itself.

**R-mach-35** A job of either kind has an input, the permissions it may use, a limit on time, and optionally a number of attempts, written the same way for both; a script also has a limit on memory and tags that choose its runners.
*Accept:* a script job written with all six is sent, carrying that input, those permissions and both limits, to a runner holding the tags; a machine job written with an input, a time limit and attempts behaves as parity cases 30, 98 and 62 show, and a machine job written with memory or tags is refused when checked.

**R-mach-36** A job may be written where it is used or kept in its own file; a script is a file named by its path, with the files it loads listed beside it, and a client sends them along with the definition.
*Accept:* a stage whose child flow is written in place, and one that names the same flow in a file, produce records of the same shape and the same output. A job names `./fetch.js` and lists `./lib.js` beside it: both are in the run's record before the first step is taken, and the script loads `./lib.js` and runs.

**R-mach-39** Every job has a name that says what asked for it: a stage, a state, a tool, an agent's `turn`, or the `ask`; an item of a list adds its position and a later attempt adds its number.
*Accept:* parity cases 01, 16, 85, 84 and 63, one for each: the record names the job after the stage, the state, the tool, `turn` and `ask`. `each:` over three items gives three jobs named for the stage and the positions 0, 1 and 2, and parity case 10's error names the second item as item 1. A stage given two attempts whose first fails has its second attempt's job named with the number 2.

**R-mach-42** When one name asks for a job more than once, the jobs are told apart in the order they were asked for.
*Accept:* parity case 16 (a state entered three times) and parity case 90 (an agent's turns one to four).

## Events

**R-mach-44** Every event has the same shape: a name, where it came from, and its data.
*Accept:* a chart receives a start, a job's `done`, a job's `failed`, a timer, an answer to a key, an emitted value and an event from its parent; each is written in the record with those three parts and nothing else.

**R-mach-45** A machine is started by a `start` event carrying its input; a job that ends well arrives as `done` with its output, and one that does not arrives as `failed` with its error.
*Accept:* parity cases 30, 42 and 43.

**R-mach-48** A `failed` event also carries whatever output the job still gave.
*Accept:* a child chart ends in a final state marked as a failure that has an output; its parent's `failed` event carries the error and that output, whether the parent is a chart or a flow.

**R-mach-49** A timer arrives as `timer`, and an answer to a key arrives as an event under the key's name.
*Accept:* parity cases 20 and 21.

**R-mach-51** A script sends an event up while it runs by emitting a named value, and a child machine by sending to its parent; the two arrive the same way, as an event with that name from that job.
*Accept:* parity case 26 with the emitted value's own name in `on:`. A parent chart has a rule for `page` on a state with a job; once with the job a script that emits `page`, and once with the job a child chart that sends `page` to its parent, the parent's record shows an event of the same shape and the same transition taken.

**R-mach-53** A parent can send a named event down to a child machine, and a machine talks only to its parent and its own children; siblings go through the parent.
*Accept:* parity case 33. Parity case 38: the bottom chart's event reaches the top only because the middle chart passes it on, and the answer comes down the same way.

**R-mach-55** A value a script emits goes to the machine that asked for the script, and to no machine above it unless that machine sends it on.
*Accept:* parity case 37: the child chart's script emits two values, and the parent, which has a rule for them on the state that runs the child, never takes it.

**R-mach-56** The client that started a root stands where a parent would: it can send the root named events, and what the root sends up is written in its record, where the client and any watcher can see it.
*Accept:* a client sends `go` with data to a root chart that has a rule for it; the chart receives an event named `go`, from the client, with that data, and takes the rule. Parity case 23's chart is run; its record holds the three events it sent to its parent, in the order sent, and a client following the record sees each.

**R-mach-58** Events sent between a parent and a child arrive once each and in the order they were sent, across steps as well as within one.
*Accept:* a controller is stopped after carrying out half of a step that sends an event to a child, and the next pass repeats the step's work; the child's record shows the event once. Parity case 35's charts are run twenty times; each time the parent receives `up1` to `up5` in that order, and the child receives the five sent down in the order written. A child sends `begin` in one step and `end` in its next; the parent receives `begin` first, however the controller's passes fall.

**R-mach-61** An event sent to a child the sender knows is not running is noted in the sender's record as undelivered and fails nothing; one sent to a child that has ended before the sender has heard so is dropped by the child.
*Accept:* parity case 94: the parent's record notes the event `hello` as undelivered, and the parent carries on.

**R-mach-63** An event sent up reaches the parent whatever the two types are, and the parent's type decides what is done with it.
*Accept:* a chart under a chart: parity case 33. A chart under a flow: parity case 50, where the event is one of that stage's emitted items. A chart under an agent: parity case 103, received and ignored. A flow that names a stage to pass up, under a flow, under a chart and under an agent: the parent receives each item as the stage emits it. An agent child sends nothing up under any parent.

**R-mach-64** An event sent down reaches the child whatever its type, and the child's type decides what is done with it.
*Accept:* chart to chart: parity case 33. Chart to flow: parity case 45. Chart to agent: parity case 87. A flow and an agent have nothing to send with, so there are no other pairings.

**C-mach-17** An event sent down before the child has taken its first step waits for the child; it is not counted as undelivered. A parent may send in the very step that starts the child, and the start and the event travel separately. *Test:* `mach_event_sent_before_child_starts_is_kept`

**C-mach-18** An emitted value the machine has been given is not given to it again, however many times the message about it arrives. Messages may arrive more than once, and an item started twice is work done twice. *Test:* `mach_emit_delivered_twice_is_taken_once`

## Children

**R-mach-65** A child's ending well is one `done` event to its parent carrying the child's output, whatever the two types are.
*Accept:* a flow under a flow: parity case 54. A chart under a flow: parity case 47. An agent under a flow: parity case 86. A flow under a chart: parity case 42. A chart under a chart: parity case 30. An agent under a chart: parity case 87. A flow and a chart under an agent: parity case 85. An agent under an agent: parity case 91.

**R-mach-66** A child's failure is one `failed` event to its parent, which the parent may handle, whatever the two types are.
*Accept:* a flow under a flow: parity case 56. A chart under a flow: parity case 48, with the child's output readable as well. An agent under a flow: parity case 86. A flow under a chart: parity case 43. A chart under a chart: parity case 31, with the child's output in the event as well. A flow and a chart under an agent: parity case 85, where the model is told. An agent under a chart: a state whose agent child fails at its first turn takes its `failed:` rule with the agent's error. An agent under an agent: the lead's model is shown the helper's error and takes another turn.

**R-mach-67** A child's failure that its parent has no rule for fails the parent, whose error names the job and then gives the child's error.
*Accept:* a flow under a flow: parity case 55. A chart under a flow: parity case 49. A flow under a chart: parity case 44. A chart under a chart: parity case 32. An agent under a flow and an agent under a chart: the parent fails with an error that names the stage or the state and then gives the agent's error. An agent parent has no such case, because every tool's failure goes to its model.

**R-mach-68** A child never outlives its parent: when a machine is cancelled, every script job and every machine below it is cancelled too.
*Accept:* a flow over a flow: parity case 61. A flow over a chart: parity case 52. A chart over a flow: parity case 102. A chart over a chart: parity case 41. An agent over a chart: parity case 104. A flow over an agent, a chart over an agent, an agent over a flow and an agent over an agent: the root is cancelled while the child is busy for six seconds; root and child both end `cancelled`, and so does any script job either had running.

**R-mach-69** When a machine ends, by finishing or by failing, everything still running below it is cancelled.
*Accept:* parity cases 03, 10, 55 and 59, where a flow fails while a child runs; parity case 92 with two named jobs in one state.

**R-mach-72** Where a machine starts many children at once and one of them fails, the others are not disturbed unless the failure fails the parent.
*Accept:* a flow's `each:` over flows: parity case 58. Over charts: parity case 53. Over agents: three agents started by `each:`, the second failing at its first turn, give a list with two outputs and one error. A chart state with several named jobs: parity case 92, where the failure nobody takes fails the chart and the other child is cancelled. An agent's calls in one turn: parity case 85.

**R-mach-73** A definition made while a machine runs, by a script or by an agent, is filled out, checked and stored exactly as one sent by a client is, and can be run as a child.
*Accept:* a script defines a machine written with short forms and two mistakes, and is refused with both problems, worded as the client's check words them; with the mistakes mended, the same call gives back a stored definition in full form. A flow's first stage defines a machine and returns it; the second stage's job names it by exactly one expression, and the child runs to its end.

**R-mach-75** Whether a child's approvals have an ask to go to is checked when the child is started.
*Accept:* a definition made at run time marks a job for approval and has no ask; started under a machine with no ask anywhere above it, it fails at its start saying that no ask applies, and starts nothing; started under a machine with an ask, it runs.

**R-mach-76** A root may have eight levels of machines below it unless it says otherwise, an ask that is a machine uses a level like any child, and a job that would go deeper fails.
*Accept:* parity cases 79, 78 and 100.

**R-mach-79** A definition may limit how many jobs its instance starts, and a job past the limit fails. The limit counts the jobs the instance itself starts, not the tree below it.
*Accept:* a flow limited to three jobs has four stages in a chain; the first three run and the fourth stage's job fails with `job limit reached`. Parity cases 80, 81, 82, 83, 99 and 101: every job they ask for starts and none fails for the limit.

**R-mach-81** A job past the depth limit or the job limit is not tried again.
*Accept:* a stage with three attempts whose child would be a ninth level; the record shows the job refused once with `depth limit reached` and no later attempt.

## Waiting on the outside

**R-mach-82** A machine waits for a person or an outside system by waiting on a key, which its step creates as a random value and hands to whoever should answer.
*Accept:* the review chart enters `waiting`; its head gives the key's name and not its value, the value is in its record, two runs of the chart get different values, and the parent receives in `review_wanted` the value the chart's record holds.

**R-mach-84** A client answers by naming the instance and the key and giving a value; the value arrives as the event's data, any client that holds the key may answer, and what the answer says about who gave it reaches the machine as given, unchecked.
*Accept:* parity case 21. A second client, enrolled with the controller but not the one that started the run, answers with the key `{ by: "anyone", ok: true }`: the answer is taken, the machine receives an event with exactly that data, and that client's receipt says it was taken.

**R-mach-87** A key is answered once, and the receipt for an answer says whether it was the one taken.
*Accept:* two answers are sent for one key; the machine receives one event, with the first one's value; the first sender's receipt says taken and the second's says the key was already answered. In parity case 21 the answer with a key the instance is not waiting on, and the answer sent after the chart has finished, each get a receipt saying so.

**R-mach-89** A key belongs to the instance that made it, and a child's key is answered by naming the child.
*Accept:* a chart under a flow: parity cases 51 and 95. A chart under a chart: parity case 40. A chart under an agent: parity case 103.

**C-mach-20** A key cannot be worked out from anything outside the instance's record: not from the head, which names what is being waited on, and not from the names of the jobs, wakes or blobs the same step made, which whoever runs the store can list. The key is the whole right to answer. *Test:* `mach_key_not_derivable_from_public_names`

## Approval

**R-mach-91** A job marked for approval does not start until its machine's ask has approved it; a job that declares a permission listed as needing to be asked about is held the same way (see Permissions, R-perm-42).
*Accept:* parity case 63.

**R-mach-95** What gives the yes is the machine's ask: a job of its own, a script or a machine, named at the top of the definition. That job is called `ask`, and nothing else in a definition may take the name.
*Accept:* parity case 63 (a script) and parity case 74 (a machine). A flow with a stage named `ask`, a chart with a state named `ask` and an agent with a tool named `ask` are each refused when checked.

**R-mach-97** A machine with an ask of its own uses it; a machine with none uses the nearest one above it.
*Accept:* parity case 73 (own ask wins); parity case 70 (one level up, approved), parity case 71 (one level up, refused) and parity case 72 (two levels up, through a machine that marks nothing).

**R-mach-99** The jobs from one step that need approval go to the ask together, and the ask is started once for them.
*Accept:* parity case 66.

**R-mach-100** The ask is told which machine is asking and, for each job, what it is and what input it would get.
*Accept:* parity case 70: each child is named as the machine asking. Parity case 63: the ask is given the job and its input.

**R-mach-102** The ask's result names the jobs it approves and may add a note. Approved jobs start; a job not approved never starts, and its machine sees it as a failed job with the status denied and the note.
*Accept:* parity case 66: the two approved items run, the other two appear with an error, and nothing was ever sent to a runner for them. Parity case 65: the note is in the error.

**R-mach-106** An ask that fails, or ends without deciding, approves nothing.
*Accept:* parity case 76: the first child's ask fails, the second's gives no decision, and neither held job starts.

**R-mach-109** An approval has no time limit of its own; the ask's deadline is how long it may take.
*Accept:* an ask that is a chart with a deadline of one second waits on a key nobody answers; it fails as out of time and the held job is denied. The same ask with no deadline holds the job for as long as the test waits.

**R-mach-112** A held job that is no longer wanted is dropped without starting, and an ask left with nothing to decide is cancelled.
*Accept:* parity case 69.

**R-mach-113** A job that starts a child is held like any other, before the child exists.
*Accept:* parity case 96.

**R-mach-114** A definition sent by a client whose approvals have no ask to go to is refused when checked.
*Accept:* a root flow that marks a stage for approval and has no ask is refused when submitted, and nothing is started.

**R-mach-115** A machine run as an ask takes no ask from above it: one that needs approval and has no ask of its own is refused.
*Accept:* parity case 77.

**C-mach-21** An ask's input and permissions are filled from the input of the machine that declares the ask, never from the input of a machine that inherits it, and the permissions and secrets of an inherited ask go into the ask's own job and into no other job of that machine. A child's input could otherwise widen what the ask may reach, and any job in a child could name the secrets its parent gave the ask. *Test:* `mach_inherited_ask_filled_from_declaring_machine`

**C-mach-22** When an ask, started once, refuses several jobs, every one of them is given to its machine as denied and written in the record, including those that arrive after the first has failed the machine. Telling only the first leaves the rest held for a decision that has already been made, and whoever reads the record must see every job that was turned down. *Test:* `mach_every_refusal_is_recorded`

## Failure

**R-mach-118** A job fails in one of four ways, and its `failed` event says which: with an error, by running out of time, by being denied, or by being cancelled.
*Accept:* an error: parity case 03. Out of time: parity case 12. Denied: parity case 64. Cancelled: the controller's owner cancels a child while its parent runs, and the parent receives `failed` saying cancelled.

**R-mach-128** A machine reacts to a failure as to any event; a failure it has no rule for fails the machine, and its own failure is the event its parent receives.
*Accept:* parity cases 17 and 44.

**R-mach-130** If a step itself fails, the instance fails, and everything below it is cancelled.
*Accept:* a stage's input holds an expression that throws while a child runs; the instance ends `failed` with an error that names the expression's place and what it threw, and the child ends cancelled.

## Machine types

**R-mach-131** A type is an extension that supplies six things, the shape of its definitions, the filling-out of its short forms, the check of its definitions, its step, what a watcher is shown of its state, and its page of the manual; everything in this area is supplied by Corbie, the same for every type.
*Accept:* a binary is built with a test extension that adds a fourth type; a definition of that type is submitted, checked, run to its end and watched, and `corbie manual` lists and prints its page. The checks of this area for values, jobs, events, keys, approval and limits pass unchanged with a flow, a statechart, an agent and the test extension's type as the machine.

## Interfaces

Uses:

- **Controller.** Takes every step and carries out what it asks for. It gives a machine its definition, its state and its events, and takes back the new state and the requests. It makes attempts and gives the machine only the final outcome (R-ctl-53 to R-ctl-58), keeps timers and ends a machine at its deadline (R-ctl-69 to R-ctl-71), and carries out what this area states about children, keys, limits and approval.
- **Instances.** A step is written as a step entry in the instance's record; the head says what the instance waits for; a client's message is answered by a receipt.
- **Runner.** A job that is a script is sent to a runner; what the script emits comes back as events, and its result as `done` or `failed`.
- **Permissions.** A job names the permissions it may use, and a definition names those to be asked about; which jobs that holds is R-perm-42 to R-perm-47; a refusal comes back as a denial with its three parts, and passes up unchanged (R-perm-37 to R-perm-41).
- **Secrets.** A definition names the secrets a job uses; a machine holds only the names.
- **Blobs.** A value may hold a reference to a blob; a value handed up from a child is copied into the parent's record before the parent sees it.
- **Plugins.** A script defines a machine at run time through the `machines` plugin; a machine type is added as an extension.

Used by:

- **Flow, Statechart, Agent.** Each supplies the six things a type supplies, and takes from this area values and expressions, jobs and their names, events, children, keys, approval, failure and limits.
- **Client.** Fills out and checks a definition with this area's code before sending it; sends events and answers to keys.
- **Controller.** Checks a definition with this area's code before starting it, and calls a type's step.
- **Manual.** Prints each type's page.
