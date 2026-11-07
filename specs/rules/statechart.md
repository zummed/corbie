# Statechart

The detailed rules behind the [Statechart overview](../statechart.md). Area code: `chart`. What a chart shares with every machine type (values, jobs and their names, events and their order, children, tickets and their answers, approval, attempts, limits) is under Machines and is not repeated here.

## States

**R-chart-01** A chart is always in some state: a chart that is started enters its initial state, and entering a state that holds states enters that state's initial state.
*Accept:* parity case 16: the first step enters `working` and asks for its job. Parity case 18: entering `outer` enters `inner` and starts `inner`'s job.

**R-chart-03** Where there is more than one state to choose from, `initial:` names the one entered first, and a chart or a state that leaves it out is refused; where there is only one, none is needed.
*Accept:* parity case 28. Parity case 27's chart is accepted, though `spinning` holds one state and names no initial.

**R-chart-05** State names are unique across a chart, a state is referred to by its name alone wherever it sits, and the order states are written in means nothing.
*Accept:* a chart with two states named `waiting`, held by different states, is refused when checked. A rule on one state names a state held inside another, by its bare name, and the chart goes there. A chart, and the same chart with its states written in the reverse order, produce the same record.

**R-chart-08** A final state ends the state that holds it, and a final state at the top ends the chart with the output it gives.
*Accept:* parity case 18: reaching `inner_final` takes `second_outer`'s rule for `done`. Parity case 30.

**R-chart-10** A final state with `end: failed` fails the chart at once when it is reached, and its output still goes to whatever started the chart.
*Accept:* started by a client: parity case 24. Started by a parent: the parent's `failed` event carries the output.

**C-chart-01** A final state with `end: failed` fails the chart from any depth, not only the state that holds it. A failure that merely ended its holder would be reported onward as an ordinary finish. *Test:* `chart_failed_final_fails_chart_from_any_depth`

## Jobs

**R-chart-12** A state's job starts when the state is entered, afresh each time, and is cancelled if the state is left before it finishes, with everything below it.
*Accept:* parity case 16: `working` is entered three times and its job is asked for three times. Parity case 19. With machines below: parity cases 39, 46 and 88.

**R-chart-15** A state that holds other states has no job of its own, and a state has one job or several named jobs, not both.
*Accept:* a state written with both inner states and a job, and a state written with a job and with named jobs, are each refused when checked.

**R-chart-16** A state may instead have several named jobs, which all start when it is entered; the state is done when all of them are done, and its output is each one's by name.
*Accept:* parity case 25 with two named jobs in one state: both jobs are asked for in that step, the rule for `done` is taken only after the second job's result, and the `done` event's output holds `left`'s output under `left` and `right`'s under `right`.

**R-chart-19** A state with several named jobs has failed as soon as one of them fails, and a failure nobody takes fails the chart and cancels the others.
*Accept:* a state has a job that fails at once, a job that takes six seconds and a rule for `failed`; the rule is taken while the slow job is still going, and the slow job is cancelled as the state is left. Parity case 92 with two named jobs in one state.

## Transitions

**R-chart-22** A state's rule for `done` is taken when the state's jobs finish, or when the states inside it reach a final state, and the event carries the output; its rule for `failed` is taken when a job fails, and the event carries the error.
*Accept:* parity case 16: the rules for `done` and `failed` each taken, the state entered reading the event's error and its name. Parity case 18: the holder's rule for `done` is given the final state's output.

**R-chart-25** A state's rules are under `on:`, by the name of the event: `done`, `failed`, a timer's name, an answer to a ticket, an event sent by the parent, by a child or to the run by its address, a value a job emitted.
*Accept:* an answer to a ticket: parity case 21. An event from the parent and one from a child: parity case 33. An emitted value: parity case 26. An event sent by address: parity case 114.

**R-chart-27** A transition names the state to go to, may have a condition and may send events; where several are listed, the first whose condition holds is taken.
*Accept:* parity case 16: the rule for `done` goes to `finished` once its condition holds, and to `working` until then. Parity case 35's parent sends five events from one transition, and the child receives all five.

**R-chart-29** A transition that names no state stays where it is and only sends.
*Accept:* parity case 26: in `listening` the chart stays for all three emitted values and the state's job keeps going.

**R-chart-82** A transition to the state that has the rule, or to a state holding it, leaves that state and enters it again: its jobs and timers start afresh, and it has a new ticket.
*Accept:* a state `open` with `timers: { idle: 30m }` and `on: { poke: open }` is poked at 10m, and its timer is due at 40m; a rule of `open` with no `go:` taken at 30m leaves it due at 40m. Parity case 16 leaves and enters `working` each round.

**R-chart-31** A rule may be written as a bare state name, as one transition, or as a list of transitions.
*Accept:* `on: { done: waiting }`, `on: { done: { go: waiting } }` and `on: { done: [{ go: waiting }] }` give the same stored definition.

**R-chart-32** An event the current state has no rule for is offered to the state that holds it, and so on outward; a failure nobody takes fails the chart, and any other event nobody takes is dropped, unless an active state names it under `defer:`.
*Accept:* offered outward: parity case 18 for a failure and parity case 21 for an answer. Parity case 17: the failure nobody takes fails the chart. Parity case 33: the event the parent has no rule for is received and changes nothing.

**R-chart-35** An event is taken by one rule at most.
*Accept:* parity case 34 with two named jobs in the child's state: the parent sends one event, the rule on that state stays and sends `poked`, and the parent receives `poked` once.

**R-chart-36** One event may set off a chain of transitions, and a chain that does not come to rest fails the chart.
*Accept:* parity case 18: one failure moves the chart to `second_outer`, enters `step_one` and asks for its job, all in the step that took the event. Parity case 27.

**C-chart-02** Finishes are acted on innermost first, and each once. A holder's rule for `done` otherwise runs before that of a state inside it, or runs twice. *Test:* `chart_finishes_innermost_first_and_once`

**C-chart-03** An event is not offered to a state that was left, or left and entered again, while that same event was being handled. One event otherwise causes two moves. *Test:* `chart_event_not_offered_to_reentered_state`

**C-chart-04** A move to a state inside the one that holds the rule does not leave the holder: its timers and its ticket stand. Leaving and entering the holder would restart timers that are meant to keep counting. *Test:* `chart_move_inward_keeps_holder`

## Data

**R-chart-38** A chart has no variables.
*Accept:* a chart written with a field that sets or stores a value is refused when checked.

**R-chart-39** Each state's latest output is remembered and can be read by name from any expression; a job that fails leaves it as it was, and a state that has produced no output reads as nothing.
*Accept:* parity case 20: the final state reads the output of `inner`, a state held by another and left earlier. Parity case 16: after the third round's job fails, the output read for `working` is the second round's. Parity case 26: the output read for `leaving_on_emit`, whose job was cancelled, is null.

**R-chart-42** An expression reads the chart's input and the event that caused the current move: its name, where it came from and its data.
*Accept:* the review chart's writer is started with the document the chart was started with. Parity case 16: `recovering`'s input is filled from the error and the name of the event that entered it.

## Sending

**R-chart-45** The only action a chart has is to send an event, to its parent, to one of its children, named by the state or the named job that started it, or to a run by its address; a send to anything else is refused when the definition is checked.
*Accept:* a chart whose entry, exit or transition holds anything but sends is refused when checked. Parity case 33: the child's send reaches its parent, and the parent's send reaches the child named by its state. A state has the named jobs `a` and `b`, both charts; a send addressed to `b` arrives at `b` and not at `a`. A send to a state whose job is a script, a send to a state with no job and a send to a name the chart does not have are each refused.

**R-chart-76** `to:` may be an address, `{ at }`, `{ at, key }` or `{ at, run }`, or exactly one expression that gives one, filled in as the send is made. A key with no location, a key and a run both, a location with `..` or an empty name, and an expression that is not the whole address are refused when the definition is checked.
*Accept:* parity case 108. *Test:* `chart_send_fills_each_address_form` and `chart_send_to_is_checked`

**R-chart-77** `event.from` of an event sent to the run by its address is the sender's address, as the controller wrote it: `{ at, run }`, with `key` for a keyed run, or `{ client }`. Of any other event it is the parent, the job or the state it came from. `to: "${event.from}"` replies to the sender.
*Accept:* parity cases 108, 114 and 121. *Test:* `chart_reply_to_event_from_gives_the_senders_address` and `chart_event_from_is_unchanged_for_jobs_timers_tickets_and_parent`

**R-chart-78** A send to an address that a layer of the chart does not allow with `send:` ends the chart as failed with the status `denied`, and nothing after the send in that step is done.
*Accept:* parity case 118. `Test_ctl_send_denied_by_a_layer_fails_the_chart`.

**R-chart-79** The names of a chart's timers and tickets are its own: an event sent to the chart by its address under one is not delivered, and its sender is told.
*Accept:* parity case 120. *Test:* `chart_owns_its_timer_and_ticket_names`

**R-chart-80** A send to an address that no run takes is noted as undelivered in the chart's record and fails nothing.
*Accept:* parity cases 109, 116 and 119.

**C-chart-14** No send can be named `start`, `done`, `failed` or `timer`. Those are the events Corbie gives a machine, and `start` sent by another run would enter a chart's first state again. *Test:* `chart_send_cannot_take_an_events_own_name`

**R-chart-51** A send made on entering a state to that state's own child is delivered once the child has started.
*Accept:* parity case 36's chart is run with its send on leaving removed; the child receives `hello` after its `start` and finishes on it.

**R-chart-52** A send on leaving a state cannot address that state's own child, and a definition that tries is refused.
*Accept:* parity case 36's chart as written is refused when checked, and the refusal names the send on leaving.

**R-chart-53** A chart that is cancelled sends nothing, and its leaving actions do not run.
*Accept:* parity cases 39 and 41.

**C-chart-05** On entering a state its ticket exists before its entry sends are filled in. The usual entry send hands the ticket to whoever should answer. *Test:* `chart_entry_send_reads_own_ticket`

**C-chart-06** The sends made in one move go out in a fixed order: those on leaving, innermost state first, then the transition's, then those on entering, outermost state first, and each list as written. Events arrive in the order sent, so the order of sending has to be fixed for the order of arrival to mean anything. *Test:* `chart_sends_in_one_move_go_in_fixed_order`

## Waiting

**R-chart-57** `ticket:` gives a state a ticket for as long as the state is active, and the answer arrives as an event with the ticket's name; the ticket ends when the state is left, and a state entered again has a new ticket.
*Accept:* parity case 21: the head names `answer` while `asking` is active and names no ticket after it is left. The review chart leaves `waiting` by its timer; an answer sent afterwards with that ticket gets a receipt saying the instance is not waiting on it. The chart goes from `waiting` to `drafting` and back; the ticket issued the second time differs from the first, and an answer with the first is refused.

**R-chart-60** Every waiting state has its own ticket, a ticket's name is used once in a statechart and is not a timer's, and an answer goes to the state that issued the ticket.
*Accept:* a statechart in which two states issue tickets named `answer` is refused when checked, and so is one in which a state issues the ticket `late` and another has a timer named `late`. A state issues `withdraw` and a state inside it issues `answer`; each entry send reads its own ticket and sends it up, and the two values differ. An answer to `withdraw` is offered to the outer state, and the inner state's rule for `answer` is not taken.

**R-chart-70** An event with a ticket's name is taken only as that ticket's answer, and one with a timer's name only as that timer being due; a rule under such a name in a state that neither issues or starts it nor holds that state is refused when the definition is checked.
*Accept:* a state issues `verdict`, has the timer `too_long` and a job, and has a rule for each; an event named `verdict` or `too_long` sent to the run by its address is not delivered and its sender is told, and one sent by the parent or emitted by the state's own job finds no rule. The ticket's answer and the timer each take their rule. A chart with a rule for `verdict` in a state beside the one that issues it is refused, and the refusal names both states and says where the rule belongs.

**R-chart-64** Each of a state's `timers:` starts when the state is entered and is cancelled when the state is left, and a timer that is due arrives as an event of its name; a timer that had come due when its state was left does nothing, and a timer on a state that holds states stands while any state inside it is active.
*Accept:* parity case 19: the eight-second timer never arrives. Parity case 22's chart is run with both timers due before one step; `first` is entered and `second` never is. Parity case 20.

**R-chart-69** A time of day is due at its next occurrence after entry; it may be given a weekday and a time zone, and without a zone it is UTC.
*Accept:* a state with a timer at `18:00` is entered at 17:00 UTC and its timer is due at 18:00 that day; entered at 19:00, it is due at 18:00 the next day, whatever zone the controller's host is set to. One naming Monday at 09:00 is entered on a Tuesday; its timer is due at 09:00 the following Monday. One naming 09:00 in Europe/London is entered in July; its timer is due at 08:00 UTC.

**R-chart-71** A timer's time with an expression is filled in when the state is entered, and a filled time that is not a length of time or a time of day fails the chart.
*Accept:* a chart with `timers: { t: "${input.due}" }` started with `{due: "18:00"}` has its timer due at 18:00. Started with `{due: "soon"}`, it fails on entry, and the error names `states.<state>.timers.t`.

**C-chart-07** A time of day is due strictly after entry: a state entered at exactly its time waits for the next occurrence. A chart that returns to its waiting state at the very moment it fired would otherwise fire again at once. *Test:* `chart_time_of_day_is_strictly_after_entry`

**C-chart-08** A time of day that a zone's clock skips on some day is due at the moment the clock jumps past it. Daily work set inside the skipped hour would otherwise miss that day. *Test:* `chart_time_of_day_in_skipped_hour_fires_at_the_jump`

**C-chart-09** Time zones are known to the binary itself, and a zone that means "wherever the controller is running" is refused. A host with no zone files would otherwise refuse or misread a zone that another host accepts, and the same chart would be due at different moments on different hosts. *Test:* `chart_zone_known_without_host_zone_files`

## A chart that is started once and used many times

**C-chart-12** A chart that is used many times does not grow with use: its state after the thousandth piece of work is no larger than after the first. The latest step holds the whole state, so a state that kept every past job would make each step slower than the last. *Test:* `chart_state_does_not_grow_with_use`

**R-chart-72** An event that no rule takes is kept when an active state names it under `defer:`, and the receipt for it says the chart acted on it; kept events are offered again in the order they arrived, each once no active state defers it and before any newer event, and one that no rule takes then is dropped.
*Accept:* parity case 106: the parent sends `said` a, b and c and then `finish` from one entry, and the chat answers a, b and c in that order and then finishes. `TestKeptEventsGoBeforeLaterOnes`: with `said b` kept, the answer to a and `said c` arrive in one step; b is answered and c is kept. A kept `work` and a kept `stop`, and a state is entered that defers `work` and not `stop`: `stop` is taken and `work` stays kept. A kept event offered again that no rule takes is dropped, and the chart no longer keeps it. A holder's rule for `said` is taken though the state it holds defers `said`.

**R-chart-73** `defer:` cannot name `done`, `failed`, a ticket's name, a timer's name or an event no state of the chart has a rule for, and a definition that does is refused when checked.
*Accept:* charts deferring `done`, a ticket's name, a timer's name, an expression and `said` with no rule for it are each refused with a problem at the `defer:` entry. A final state with `defer:` is refused.

**R-chart-81** A chart that ends drops the events it kept. One cancelled, by a client or by `update: restart`, notes each event it kept as undelivered in the record of the run that sent it.
*Accept:* parity case 130. `Test_ctl_kept_events_of_a_cancelled_chart_are_noted`, `Test_ctl_kept_events_a_parent_sent_are_noted_to_it` and `Test_ctl_restart_starts_a_singleton_again_and_notes_kept_events`.

**C-chart-13** A chart keeps at most 100 events; one more is dropped, and the receipt for it says the chart did not act on it. Every step reads the chart's state whole, so a sender could otherwise make each step slower than the last. *Test:* `chart_kept_events_are_bounded`

## Interfaces

Uses:

- **Machines.** Values and expressions, jobs and their names, events and their order, children, tickets and their answers, approval, failure and attempts, and limits. A chart adds `states`, `event` and `tickets` to what an expression reads.
- **Controller.** Takes the chart's steps, keeps and delivers its timers (R-ctl-69), makes its tickets and takes their answers, starts and cancels its children, and carries its sends to its parent, its children and runs by their address.
- **Instances.** The head counts the chart's timers and names the tickets it waits on; a root's sends to its parent, and each send to an address that was not delivered, are written in its record; a client's event is answered by a receipt saying whether the chart acted on it.
- **Client.** Sends a chart events by its address and answers its tickets.

Used by:

- **Machines.** A statechart is one of the three types, and supplies the filling-out of its short forms, its check, its step, what a watcher is shown of its state, and its manual page.
- **Flow, Agent.** Either may run a chart as a child, and a chart may run either.
- **Machines, for approval.** An ask that waits for a person is typically a chart that hands out a ticket and ends with the jobs it approves.
