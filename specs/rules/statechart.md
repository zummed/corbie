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

**R-chart-25** A state's rules are under `on:`, by the name of the event: `done`, `failed`, a timer's name, an answer to a ticket, an event sent by the parent or by a child, a value a job emitted.
*Accept:* an answer to a ticket: parity case 21. An event from the parent and one from a child: parity case 33. An emitted value: parity case 26.

**R-chart-27** A transition names the state to go to, may have a condition and may send events; where several are listed, the first whose condition holds is taken, and a transition that names its own state leaves the state and enters it again.
*Accept:* parity case 16: the rule for `done` goes to `finished` once its condition holds, and to `working` until then, leaving and entering `working` each round. Parity case 35's parent sends five events from one transition, and the child receives all five.

**R-chart-29** A transition that names no state stays where it is and only sends.
*Accept:* parity case 26: in `listening` the chart stays for all three emitted values and the state's job keeps going.

**R-chart-31** A rule may be written as a bare state name, as one transition, or as a list of transitions.
*Accept:* `on: { done: waiting }`, `on: { done: { go: waiting } }` and `on: { done: [{ go: waiting }] }` give the same stored definition.

**R-chart-32** An event the current state has no rule for is offered to the state that holds it, and so on outward; a failure nobody takes fails the chart, and any other event nobody takes is dropped.
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

**R-chart-42** An expression reads the chart's input and the event that caused the current move.
*Accept:* the review chart's writer is started with the document the chart was started with. Parity case 16: `recovering`'s input is filled from the error and the name of the event that entered it.

## Sending

**R-chart-45** The only action a chart has is to send an event, to its parent or to one of its children, named by the state or the named job that started it; a send to anything else is refused when the definition is checked.
*Accept:* a chart whose entry, exit or transition holds anything but sends is refused when checked. Parity case 33: the child's send reaches its parent, and the parent's send reaches the child named by its state. A state has the named jobs `a` and `b`, both charts; a send addressed to `b` arrives at `b` and not at `a`. A send to a state whose job is a script, a send to a state with no job and a send to a name the chart does not have are each refused.

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
*Accept:* a state issues `verdict`, has the timer `too_long` and a job, and has a rule for each; an event named `verdict` or `too_long` sent by the client, sent by the parent or emitted by the state's own job finds no rule, and the client's receipt says so. The ticket's answer and the timer each take their rule. A chart with a rule for `verdict` in a state beside the one that issues it is refused, and the refusal names both states and says where the rule belongs.

**R-chart-64** Each of a state's `timers:` starts when the state is entered and is cancelled when the state is left, and a timer that is due arrives as an event of its name; a timer that had come due when its state was left does nothing, and a timer on a state that holds states stands while any state inside it is active.
*Accept:* parity case 19: the eight-second timer never arrives. Parity case 22's chart is run with both timers due before one step; `first` is entered and `second` never is. Parity case 20.

**R-chart-69** A time of day is due at its next occurrence after entry; it may be given a weekday and a time zone, and without a zone it is UTC.
*Accept:* a state with a timer at `18:00` is entered at 17:00 UTC and its timer is due at 18:00 that day; entered at 19:00, it is due at 18:00 the next day, whatever zone the controller's host is set to. One naming Monday at 09:00 is entered on a Tuesday; its timer is due at 09:00 the following Monday. One naming 09:00 in Europe/London is entered in July; its timer is due at 08:00 UTC.

**C-chart-07** A time of day is due strictly after entry: a state entered at exactly its time waits for the next occurrence. A chart that returns to its waiting state at the very moment it fired would otherwise fire again at once. *Test:* `chart_time_of_day_is_strictly_after_entry`

**C-chart-08** A time of day that a zone's clock skips on some day is due at the moment the clock jumps past it. Daily work set inside the skipped hour would otherwise miss that day. *Test:* `chart_time_of_day_in_skipped_hour_fires_at_the_jump`

**C-chart-09** Time zones are known to the binary itself, and a zone that means "wherever the controller is running" is refused. A host with no zone files would otherwise refuse or misread a zone that another host accepts, and the same chart would be due at different moments on different hosts. *Test:* `chart_zone_known_without_host_zone_files`

## A chart that is started once and used many times

**C-chart-12** A chart that is used many times does not grow with use: its state after the thousandth piece of work is no larger than after the first. The latest step holds the whole state, so a state that kept every past job would make each step slower than the last. *Test:* `chart_state_does_not_grow_with_use`

## Interfaces

Uses:

- **Machines.** Values and expressions, jobs and their names, events and their order, children, tickets and their answers, approval, failure and attempts, and limits. A chart adds `states`, `event` and `tickets` to what an expression reads.
- **Controller.** Takes the chart's steps, keeps and delivers its timers (R-ctl-69), makes its tickets and takes their answers, starts and cancels its children, and carries its sends up and down.
- **Instances.** The head counts the chart's timers and names the tickets it waits on; a root's sends to its parent are written in its record; a client's event is answered by a receipt saying whether the chart acted on it.
- **Client.** Finds a standing chart by its label, sends it events and answers its tickets.

Used by:

- **Machines.** A statechart is one of the three types, and supplies the filling-out of its short forms, its check, its step, what a watcher is shown of its state, and its manual page.
- **Flow, Agent.** Either may run a chart as a child, and a chart may run either.
- **Machines, for approval.** An ask that waits for a person is typically a chart that hands out a ticket and ends with the decision.
