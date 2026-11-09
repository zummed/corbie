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

A state may have a job. It starts when the state is entered and is cancelled if the state is left before it finishes. A state that holds other states has no job of its own.

A state may instead have several named jobs, which all start when it is entered. The state is done when all of them are done, its output is each one's by name, and it has failed as soon as one fails. This is how a chart does several things at once.

## Transitions

A state's `on:` says where to go on each event, by the event's name.

- `done` is the state's jobs finishing, or the states inside it reaching a final state.
- `failed` is a job failing.
- A timer's name is the timer being due.
- A ticket's name is the ticket's answer.
- Any other name is an event sent to the run, or a value a job emitted.

A transition names the state to go to, and may have a condition and may send events. Where several are listed, the first whose condition holds is taken. A transition that names no state stays where it is and only sends.

A transition to the state that has the rule, or to a state holding it, leaves that state and enters it again: its jobs and timers start afresh, and it has a new ticket.

An event is taken by at most one rule. One the current state has no rule for is offered to the state that holds it, and so on outward. A failure nobody takes fails the chart. Any other event nobody takes is dropped.

One event may set off a chain of transitions. A chain that does not come to rest fails the chart.

## Data

A chart has no variables. Each state's latest output is remembered and can be read by name from any expression, along with the chart's input and the event that caused the current move.

A state's output is set only when the state is done: by its jobs ending well, or by a final state inside it. A job that fails leaves the state's earlier output as it was.

A value built up over a loop goes through a script: in as its input and out as its output.

## Sending

The only action a chart has is to send an event: to its parent, to one of its children, named by the state or the job that started it, or to a run at an address. A send can sit on a transition, on entering a state or on leaving one.

A send made on entering a state to that state's own child is delivered once the child has started. A send on leaving a state cannot address that state's own child, which is already being stopped, and a definition that tries is refused. A chart that is cancelled takes no further step, so it sends nothing and its leaving actions do not run.

An event sent to a chart goes to the chart and to nothing below it. A child hears only what its parent chooses to send on.

`to:` is `parent`, a state or a named job, or an address as machine.md says. `to: "${event.from}"` replies to the sender of the event being taken.

```yaml
# house/thermostats/kitchen.yaml        location /home/thermostats/kitchen
statechart: kitchen
lifetime: singleton
allow: [net:kitchen-sensor.lan, send:devices/boiler]
initial: reading
states:
  reading:
    job: { script: ./read-sensor.js }
    on:
      done:
        - { go: resting, when: "${event.data.celsius < 19}", send: { to: { at: devices/boiler }, event: set, data: { target: 21 } } }
        - resting
      failed: resting
  resting:
    timers: { again: 5m }
    on: { again: reading }
```

- A send to an address needs `send:` in every layer of the chart, and the receiver's `receive:` must accept the chart (see permission.md). A send the chart's own layers refuse fails the chart with the status `denied`.
- Any other send that is not taken is noted as undelivered in the chart's record and fails nothing. machine.md lists the cases.
- Only a chart takes a sent event.
- A send action is never held by `approve:`.

## Waiting

`ticket:` makes a state issue a ticket while it is active: a random value that lets whoever holds it answer once. The answer arrives as an event with the ticket's name, and no other event is taken under that name. A ticket's name is used once in a chart, no timer takes it, and a state entered again issues a new ticket.

The client that owns the run's tree reads the ticket's value and hands it to whoever should answer. A chart run as a child hands it to its parent with a send that reads `${tickets.<name>}`.

`timers:` names timers that start when the state is entered and are cancelled when it is left. A timer that is due arrives as an event with the timer's name, from the state, and no other event is taken under that name.

A length of time counts from entry. A time of day, optionally with a weekday and a time zone, is due at its next occurrence after entry; without a zone it is UTC. A time with an expression is filled in when the state is entered, and a filled time that is not a length of time or a time of day fails the chart.

A rule under a ticket's or a timer's name belongs in the state that issues or starts it, or in a state that holds that one: anywhere else it would never be taken, and the definition is refused.

Corbie has no scheduler. Work that recurs is done by a chart that waits on a timer and starts the work as a child.

## A chart that waits for work

A chart with `lifetime: singleton` starts when its tree is submitted and runs until it ends. Senders address it by its location. On each event it moves to a state that runs the work as a child, and comes back when the child is done.

```yaml
# work/reports/nightly.yaml             location /work/reports/nightly
statechart: nightly-report
lifetime: singleton
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

Such a chart does one piece of work at a time. An event that arrives while it is busy finds no rule and is dropped, unless an active state names it under `defer:`. A client sender's receipt says so; a run sender is told nothing.

1. An event that no rule takes is kept when an active state names it under `defer:`, and the receipt says the chart acted on it.
2. Kept events are offered again in the order they arrived, each once no active state defers it, and before any newer event.
3. A kept event that no rule takes then is dropped.

- Kept events are offered in the step in which the last state keeping them is left, so a state entered in that step is given a kept event before its own job's result or its own timer.
- A rule of a state that holds the deferring state is taken before the event is kept.
- A state cannot defer `done`, `failed`, a ticket's name, a timer's name, or an event no state has a rule for, and a definition that tries is refused.
- A chart that ends drops the events it kept.
- When a chart is cancelled, by a client or by `update: restart`, each event it kept is noted as undelivered in its sender's record.
- A chart keeps at most 100 events. One more is dropped, as an event with no rule is; the newest is dropped because a receipt already written for an older one cannot change.

## Handing over to the next run

A singleton starts again on every change to its file, and by default the new run starts empty. `handover:` names one state of a singleton chart: when a submit replaces the run, the new run's input is that state's latest output in the run replaced, or, if it gave none, what that run was given.

```yaml
# feeds/watch.yaml                      location /feeds/watch
statechart: watch
lifetime: singleton
handover: checking
initial: checking

states:
  checking:
    job: { script: ./check.js, input: "${states.checking.output ?? input}" }
    on: { done: waiting, failed: waiting }
  waiting:
    timers: { again: 10m }
    on: { again: checking }
```

```js
// check.js: what this run has seen, from its last check or from the run before
const seen = input?.seen ?? []
// ... fetch the feed, add the new ids ...
return { seen }
```

1. A submit finds the file changed and cancels the running chart.
2. Once that run and its children have ended, a new run starts on the new file.
3. Its input is the latest output of the state the cancelled run's `handover:` names, or, if that state gave none there, what the cancelled run was given.

```
  what happens                                          the new run's input   submit prints
  first submit                                          null                  started
  check.js changed, submitted                           { seen: [a, b] }      restarted
  watch.yaml changed again before checking finished     { seen: [a, b] }      restarted
  corbie run cancel /feeds/watch, then a submit         null                  started
```

The third row is the fallback: the second run had not finished `checking`, so it hands over what it was given.

- The run replaced follows its own definition, as every run does. A chart that gains `handover:` in a change hands nothing over on that submit, and one that loses it hands over on that submit for the last time.
- The new run's `input:` checks what it is handed, as it checks any input. A value that does not fit fails the new run at its first step, and the next submit starts the chart with no input.
- With no `input:` nothing checks the value, so the script turns an older shape into the new one.
- Only a run a submit cancels hands over. A chart cancelled, failed or ended before the submit hands nothing to the run the submit starts.
- Work the cancelled run had under way is not in the output, and may be done again.
- When the cancelled run hands over nothing by its `handover:`, or its record can no longer be read, the new run's record has a note naming that run and why.

## One run per key

`lifetime: keyed` is for statecharts only. A location with it has one run for each key a sender names.

```yaml
# bot/agents/chat.yaml                  location /bot/agents/chat
statechart: chat
lifetime: keyed
limits: { runs: 50 }
receive: [bridges/slack]
allow: [send:bridges/slack]
initial: listening
states:
  listening:
    on: { message: answering }
  answering:
    job: { machine: ./answer.yaml, input: "${event.data}" }
    defer: [message]
    on:
      done: { go: listening, send: { to: { at: bridges/slack }, event: reply, data: "${event.data}" } }
      failed: listening
```

The bridge sends `send: { to: { at: agents/chat, key: "${event.data.thread}" }, event: message, data: "${event.data}" }`.

1. A send to a key with no running run starts one. The event's data is its input, then the event is delivered to it.
2. Further sends to that key go to that run.
3. When the run ends, the next send to the key starts a fresh one.

- `limits: { runs: N }` caps how many of the location's runs are running at once, as it does at a transient location (see machine.md). With the cap reached, a send to a new key is not delivered and is noted in the sender's record; sends to running keys still go through. There is no default cap.
- A run started by a send counts one level below its sender, and fails as it starts if that passes its own file's depth limit.
- The run's authority comes from its location. The sender adds nothing to it and takes nothing away (see permission.md).
