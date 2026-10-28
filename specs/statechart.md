# Statechart

A statechart is a machine that is always in some state and moves between states when events arrive. It is for anything that waits, loops, branches or reacts: a process with people in it, a schedule, a conversation with an outside system.

```yaml
statechart: review
input: { document: string }
initial: drafting

states:
  drafting:
    job: { machine: ./writer.yaml, input: { document: "${input.document}" } }
    on:
      done: waiting
      failed: gave_up
  waiting:
    ticket: verdict
    timers: { too_long: 48h }
    on:
      verdict:
        - { go: approved, when: "${event.data.ok}" }
        - { go: drafting }
      too_long: gave_up
  approved: { end: done, output: "${states.drafting.output}" }
  gave_up:  { end: failed }
```

## States

A chart has states, and a state may hold states of its own. Where there is more than one to choose from, `initial:` names the one entered first. State names are unique across a chart, so a state is always referred to by its name alone. The order states are written in means nothing.

A final state is one with `end:`. It ends the state that holds it, and a final state at the top ends the chart with the output it gives. A final state with `end: failed` fails the chart at once, from any depth, and its output still goes to whatever started the chart.

## Jobs

A state may have a job, a script or a machine. It starts when the state is entered and is cancelled if the state is left before it finishes. A state that holds other states has no job of its own.

A state may instead have several named jobs, which all start when it is entered. The state is done when all of them are done, its output is each one's by name, and it has failed as soon as one fails. This is how a chart does several things at once.

## Transitions

A state's `on:` says where to go on each event, by the event's name.

- `done` is the state's jobs finishing, or the states inside it reaching a final state.
- `failed` is a job failing.
- A timer's name is the timer being due.
- A ticket's name is the ticket's answer.
- Any other name is an event sent by the parent or by a child, or a value a job emitted.

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

`ticket:` makes a state issue a ticket while it is active: a random value that lets whoever holds it answer once. The answer arrives as an event with the ticket's name, and no other event is taken under that name. A ticket's name is used once in a chart, no timer takes it, and a state entered again issues a new ticket.

The client that started the run reads the ticket's value and hands it to whoever should answer. A chart run as a child hands it to its parent with a send that reads `${tickets.<name>}`.

`timers:` names timers that start when the state is entered and are cancelled when it is left. A timer that is due arrives as an event with the timer's name, from the state, and no other event is taken under that name.

A length of time counts from entry. A time of day, optionally with a weekday and a time zone, is due at its next occurrence after entry; without a zone it is UTC. A time with an expression is filled in when the state is entered, and a filled time that is not a length of time or a time of day fails the chart.

A rule under a ticket's or a timer's name belongs in the state that issues or starts it, or in a state that holds that one: anywhere else it would never be taken, and the definition is refused.

Corbie has no scheduler. Work that recurs is done by a chart that waits on a timer and starts the work as a child.

## A chart that is started once and used many times

A chart can stand ready for as long as it runs and do a piece of work each time it is told to. Its client finds it by its label and sends it an event; the chart moves to a state that runs the work as a child, and comes back when the child is done.

```yaml
statechart: nightly-report
initial: ready

states:
  ready:
    on: { work: working }
  working:
    job: { machine: ./report.yaml, input: "${event.data}" }
    defer: [work]
    on:
      done: ready
      failed: ready
```

Such a chart does one piece of work at a time. An event that arrives while it is busy finds no rule and is dropped, and the sender's receipt says so, unless an active state names it under `defer:`.

1. An event that no rule takes is kept when an active state names it under `defer:`, and the receipt says the chart acted on it.
2. Kept events are offered again in the order they arrived, each once no active state defers it, and before any newer event.
3. A kept event that no rule takes then is dropped.

- A rule of a state that holds the deferring state is taken before the event is kept.
- A state cannot defer `done`, `failed`, a ticket's name, a timer's name, or an event no state has a rule for, and a definition that tries is refused.
- A chart that ends or is cancelled drops the events it kept.
- A chart keeps at most 100 events. One more is dropped, and the sender's receipt says so; the newest is dropped because a receipt already written for an older one cannot change.

Work that must go on many at once is started by a client, one run each.
