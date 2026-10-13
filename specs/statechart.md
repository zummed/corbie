# Statechart

A statechart is a machine that is always in some state and moves between states when events arrive. It is for anything that waits, loops, branches or reacts: a process with people in it, a schedule, a conversation with an outside system.

```yaml
statechart: review
input: [document]
initial: drafting

states:
  drafting:
    job: { machine: ./writer.yaml, input: { document: "${input.document}" } }
    done: waiting
    failed: gave_up
  waiting:
    wait: verdict
    entry: { send: { to: parent, event: review_wanted, data: { key: "${keys.verdict}" } } }
    on:
      verdict:
        - { to: approved, when: event.data.ok }
        - { to: drafting }
    after: { 48h: gave_up }
  approved: { final: true, output: "${states.drafting.output}" }
  gave_up:  { final: true, failed: true }
```

## States

A chart has states, and a state may hold states of its own. Where there is more than one to choose from, `initial:` names the one entered first. State names are unique across a chart, so a state is always referred to by its name alone. The order states are written in means nothing.

A final state ends the state that holds it, and a final state at the top ends the chart with the output it gives. A final state may be marked as a failure. Reaching one fails the chart at once, and its output still goes to whatever started the chart.

## Jobs

A state may have a job, a script or a machine. It starts when the state is entered and is cancelled if the state is left before it finishes. A state that holds other states has no job of its own.

A state may instead have several named jobs, which all start when it is entered. The state is done when all of them are done, its output is each one's by name, and it has failed as soon as one fails. This is how a chart does several things at once.

## Transitions

A state says where to go when something happens.

- `done:` is taken when the state's jobs finish, or when the states inside it reach a final state.
- `failed:` is taken when a job fails.
- `on:` names any other event: an answer to a key, an event sent by the parent or by a child, a value a job emitted.
- `after:` names a length of time or a time of day.

A transition names the state to go to, and may have a condition and may send events. Where several are listed, the first whose condition holds is taken. A transition that names no state stays where it is and only sends.

An event is taken by at most one rule. One the current state has no rule for is offered to the state that holds it, and so on outward. A failure nobody takes fails the chart. Any other event nobody takes is dropped.

One event may set off a chain of transitions. A chain that does not come to rest fails the chart.

## Data

A chart has no variables. Each state's latest output is remembered and can be read by name from any expression, along with the chart's input and the event that caused the current move. A value built up over a loop goes through a script: in as its input and out as its output.

## Sending

The only action a chart has is to send an event, to its parent or to one of its children, named by the state or the job that started it. A send can sit on a transition, on entering a state or on leaving one.

A send made on entering a state to that state's own child is delivered once the child has started. A send on leaving a state cannot address that state's own child, which is already being stopped, and a definition that tries is refused. A chart that is cancelled takes no further step, so it sends nothing and its leaving actions do not run.

An event sent to a chart goes to the chart and to nothing below it. A child hears only what its parent chooses to send on.

## Waiting

`wait:` gives a state a key for as long as the state is active, and the answer arrives as an event with the key's name. Every waiting state has its own key, and an answer goes to the state whose key it carries. Two states may wait under the same name, and an expression that reads the key gets its own state's.

`after:` starts a timer when the state is entered and cancels it when the state is left. A length of time counts from entry. A time of day, optionally with a weekday and a time zone, is due at its next occurrence after entry; without a zone it is UTC.

Corbie has no scheduler. Work that recurs is done by a chart that waits with `after:` and starts the work as a child.

## A chart that is started once and used many times

A chart can stand ready for as long as it runs and do a piece of work each time it is told to. Its client finds it by its label and sends it an event; the chart moves to a state that runs the work as a child, and comes back when the child is done.

```yaml
statechart: nightly-report
initial: ready

states:
  ready:
    on: { go: working }
  working:
    job: { machine: ./report.yaml, input: "${event.data}" }
    done: ready
    failed: ready
```

Such a chart does one piece of work at a time. An event that arrives while it is busy finds no rule and is dropped, and the sender's receipt says so. Work that must go on many at once is started by a client, one run each.
