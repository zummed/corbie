# Flow

The detailed rules behind the [Flow overview](../flow.md). Area code: `flow`. What a flow shares with every machine type (values, jobs and their names, events, children, approval, attempts, limits) is under Machines and is not repeated here.

## Stages and order

**R-flow-01** Each stage has a job, and each stage runs once.
*Accept:* a stage written with no job is refused when the definition is checked. Parity case 01: each of the four stages is asked for once.

**R-flow-03** A stage that uses another stage's output runs after it, and `follows:` orders two stages that pass nothing.
*Accept:* parity case 01, where `last` names `third` in `follows:`, runs after it and takes no data from it.

**R-flow-05** Stages that do not depend on each other run at the same time.
*Accept:* a flow of three stages with no arrows between them asks for all three jobs in its first step.

**R-flow-06** A flow with a cycle is refused.
*Accept:* stage `a` reads `b`'s output and `b` has `follows: a`; the definition is refused when checked, and the refusal names both stages.

**R-flow-07** A flow's `output:` is its result, filled in once every stage has settled; it reads the flow's input and its stages, and a flow with no `output:` ends well with no output.
*Accept:* parity case 54. The digest flow's `output: ${stages.summarise.output}` gives the summary, and an `output:` of `${input.urls.length}` gives the number of addresses. Parity case 15.

**C-flow-01** Two ways of writing one reference to a stage draw the same arrow. An arrow the check misses lets a stage run before the one it reads. *Test:* `flow_reference_spellings_draw_the_same_arrow`

**C-flow-02** A reference to a stage the flow does not have is reported when the definition is checked. The arrows come from the data, and an arrow to nothing leaves a stage waiting for ever. *Test:* `flow_reference_to_missing_stage_is_refused`

## Skipping

**R-flow-10** A stage with `when:` runs only if its condition holds.
*Accept:* parity case 02: `gate` is skipped and no job is asked for it.

**R-flow-11** A skip spreads along the arrows, `follows:` included: a stage that needs something from a skipped stage is skipped as well, unless its own condition asks whether that stage was skipped, in which case it decides for itself.
*Accept:* parity case 02: the stage that reads `gate`'s output is skipped, so is the stage that reads that stage's, and the stage whose condition asks whether `gate` was skipped runs. The same written with `follows:`.

**R-flow-14** A skipped stage has settled, and a flow whose stages have all run or been skipped ends well.
*Accept:* parity case 02: the flow ends well, and its output says which stages were skipped.

## Failure

**R-flow-16** A stage handles another's failure by reading it: its condition or its input mentions whether that stage failed, or its error.
*Accept:* parity case 04. Parity case 05's flow is run; the stage that reads the error runs with the error's text, and the flow ends well.

**R-flow-18** A failure counts as handled only when a stage that reads it actually runs: if no stage reads it, the flow fails the moment it arrives, and if one does but is then skipped, the flow fails then.
*Accept:* parity case 03: the flow fails at once with an error that names the stage, and the child still running is cancelled. Parity case 29's flow is run and fails, with the failed stage's error. `bad` fails while `slow` is still running, and the only reader of the failure also needs the output of a stage that is skipped once `slow` has ended: the flow is still running until that skip is decided and fails then, with `bad`'s error.

**R-flow-21** A stage that depends on a failed stage for its output, and does not read the failure, is skipped.
*Accept:* parity cases 04 and 56.

**R-flow-22** A stage that reads a failure can read the failed stage's error and whatever output it still gave.
*Accept:* parity case 48's flow is run; the handling stage reads `reached gave_up` as the error and the failed final state's output as the output.

## Once per item

**R-flow-24** `each:` runs a stage once for every item of a list, and the stage's output is a list in the order of the items, holding for each the item and its output, or the item and its error.
*Accept:* parity case 06: `bump` runs three times, once with each number, and its output has three entries in item order, whichever job finished first. Parity case 09: the failed item appears with its error in its place.

**R-flow-26** A stage that reads a per-item stage's output runs once and is given the whole list; nothing in a flow has a second meaning for the per-item case.
*Accept:* `each:` over three numbers, and a second stage whose input is the first stage's output; the second stage runs once and its input is the list of three entries.

**R-flow-28** Items run at the same time unless the stage says they go one at a time.
*Accept:* `each:` over three items asks for three jobs in one step; the same stage told to go one at a time asks for the second job only after the first has settled, in item order.

**R-flow-29** An item that fails does not fail the flow by itself; the flow fails only if nothing reads the stage's output.
*Accept:* read: parity case 09, and with a child per item parity cases 53 and 58. Not read: parity cases 10 and 59.

**R-flow-33** The items may be what another stage emits, said by naming that stage: each item starts as soon as it is emitted, without waiting for the emitting stage to finish, and the output is in the order the items were emitted.
*Accept:* parity case 07, which names the stage: the first item's job is asked for before `source`'s result arrives, and the gathered outputs are in emitted order.

**R-flow-35** If the emitting stage fails, no more items start and those already started finish.
*Accept:* `source` emits two items and then fails, and another stage reads the failure; the two items' jobs finish and appear in the output, and no third job is asked for.

**R-flow-36** The items may come from a blob, and what an item is depends on the blob's type: a line of text, a row of a table, an entry of a list. This is a short form: the flow runs a job that reads the blob and emits its items, and follows that job's emits.
*Accept:* `each:` over a text of three lines, a table of three rows and a list of three entries each asks for three jobs, whose items are the lines, the rows and the entries. `each:` over a blob of a type that has no items, such as a document or an image, fails the reading job, which says what has items. The stored definition holds the reading stage and the stage that follows it; the record shows the reading job, and for a large blob the first item's job is asked for before the reading job has finished.

**R-flow-38** A flow keeps every item's result in its state, and `each:` works over lists of hundreds.
*Accept:* `each:` over five hundred items ends well, with five hundred entries in item order.

**C-flow-03** A flow's own `output:` counts as a reader of a per-item stage's output. A flow whose result is the list would otherwise fail on the first bad item, though the list with its errors is exactly what was asked for. *Test:* `flow_output_counts_as_reader_of_item_failures`

**C-flow-04** A stage that follows another's emits waits for its other inputs before it takes its first item. Items otherwise start with inputs that are not ready. *Test:* `flow_following_stage_waits_for_other_inputs`

**C-flow-05** `each:` given a value that is neither a list nor a blob fails the flow with an error saying what it needs. A text or a number quietly treated as one item, or as none, hides a mistake in the expression. *Test:* `flow_each_refuses_what_is_not_a_list`

## Events

**R-flow-39** What a stage's job sends up while it works, a script's emits or a child machine's events, is available to later stages as that stage's emitted items, as they arrive.
*Accept:* parity case 07. Parity case 50: the per-item stage runs once, for the event the chart sent, and nothing arrives for the values the chart's own script emitted.

**R-flow-41** A stage that reads another stage's emitted items in an expression runs after that stage has settled, and is given them all in the order emitted.
*Accept:* parity case 08: its `each:` is given by an expression over `source`'s emitted items, and every item's job is asked for only after `source` has finished.

**R-flow-42** A flow can name one stage whose emitted items it passes up to its own parent as they arrive, which is how a flow gives partial results; a flow that names no such stage passes nothing up.
*Accept:* a child flow names its emitting stage, which emits three values; the parent flow's per-item stage that follows the child's stage starts its first item before the child has finished, and runs three items in all. A root flow names a stage that emits three values a second apart; a client following the record sees each value as an event sent up, before the flow has ended. Parity cases 57 and 45, where nothing is named and nothing is passed up.

**R-flow-45** A flow does not wait on keys or timers, and ignores events sent down to it.
*Accept:* a flow written with a `wait:` or an `after:` is refused when checked; a running flow's head never names a key or counts a timer. Parity case 45: the flow receives the event, carries on and finishes. For a root flow, the receipt of the client that sent the event says the flow had no rule for it.

## Interfaces

Uses:

- **Machines.** Values and expressions, jobs and their names, events, children, approval, failure and attempts, and limits. A flow adds `stages` and, in a per-item stage, `item` to what an expression reads.
- **Controller.** Takes the flow's steps, makes each stage's attempts, starts and cancels its children, and passes on the emitted values the flow has said it wants.
- **Runner.** Executes each script stage's job, including the job that reads a blob into items.
- **Blobs.** A blob's type decides what its items are; the reading is done by a job and not by a step.

Used by:

- **Machines.** A flow is one of the three types, and supplies the shape of its definitions, the filling-out of its short forms, its check, its step, what a watcher is shown of its state, and its manual page.
- **Statechart, Agent.** Either may run a flow as a child, and a flow may run either.
- **Client.** Wraps a script submitted by itself in a flow of one stage.
