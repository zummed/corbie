# Flow

A flow is a set of stages that run once each, in an order that follows from the data they pass. It is for work that goes one way. Branching back and looping belong in a statechart, and a flow can run one as a child.

```yaml
flow: digest
input: { urls: list }

stages:
  fetch:
    each: ${input.urls}
    job: { script: ./fetch.js, input: { url: "${item}" } }
  summarise:
    job: { script: ./summarise.js, input: { pages: "${stages.fetch.output}" } }
  tell_someone:
    when: ${stages.summarise.failed}
    job: { script: ./notify.js, input: { why: "${stages.summarise.error}" } }

output: ${stages.summarise.output}
```

## Stages and order

Each stage has a job. A stage that uses another stage's output runs after it, so the arrows of a flow come from its data and are not written separately. `follows:` orders two stages that pass nothing. Stages that do not depend on each other run at the same time. A flow with a cycle is refused.

A flow's `output:` is its result, filled in once every stage has settled. It reads the flow's input and its stages.

## Skipping

`when:` makes a stage optional: it runs only if its condition holds. A skip spreads along the arrows, `follows:` included. A stage that needs something from a skipped stage is skipped as well, unless its own condition asks whether that stage was skipped, in which case it runs if its condition holds.

## Failure

A stage fails when its job fails after whatever attempts it was given.

A stage handles another's failure when its `when:` reads whether that stage failed, or its error. A failure counts as handled only when such a stage actually runs, or when the flow's `output:` reads it. If neither reads a failure, the flow fails the moment it arrives, and if only a stage does and it is then skipped, the flow fails then.

A stage that depends on a failed stage for its output, and whose `when:` does not read the failure, is skipped.

## Once per item

`each:` runs a stage once for every item of a list. The stage's output is then a list in the order of the items: for each, the item and its output, or the item and its error. Items run at the same time unless `each:` says `serial: true`.

An item that fails does not fail the flow by itself. It appears in the stage's output with its error, and the flow fails only if nothing reads that output.

To do several things for each item, the stage's job is a machine, usually another flow, which may be written in place. Each item is then an instance of its own, with its own stages, skips and failures. Nothing in a flow has a second meaning for the per-item case.

`each:` names where the items come from, one of three places, and may add `serial: true`.

- **A list,** written as `{ items: … }`, in the definition or given by an expression. A value that is neither a list nor a blob fails the flow. A list or an expression written as `each:` itself is short for this.
- **What another stage emits,** written as `{ emitted: <stage> }`. Each item starts as soon as it is emitted, without waiting for the emitting stage to finish. If the emitting stage fails, no more items start and those already started finish.
- **A blob,** whose items depend on its type: lines of text, rows of a table, entries of a list. A job of the stage's own name reads the blob and emits its items, and each item starts as it arrives. If the reading job fails, the stage fails.

A flow keeps every item's result in its state, so `each:` suits lists of hundreds. Work on many thousands of items belongs in a job that reads and writes blobs.

## Events

What a stage's job sends up while it works, a script's emits or a child machine's events, is available to later stages as that stage's emitted items, as they arrive. A flow's `emit:` can name one stage whose emitted items it emits to its own parent as they arrive, which is how a flow gives partial results.

A flow does not wait on tickets or timers, and ignores events sent down to it.
