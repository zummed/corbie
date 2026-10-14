# Controller

The detailed rules behind the [Controller overview](../controller.md). What the controller does for every machine is stated once: attempts, timers and timeouts are here; what a machine sees of children, tickets, limits and approval is under Machines, and what a machine may be held for is under Permissions.

## What arrives

**R-ctl-04** What the controller tells itself, a child's start, a child's ending or an event between a parent and a child, is a message in its own inbox in the store.
*Accept:* stop the controller straight after a parent's step that starts a child: the child's start is in the controller's inbox in the store, and a controller started afresh takes it.

**R-ctl-07** A client may cancel or send events only to runs it started, and may answer any ticket it holds; the controller's owner can cancel any run with the controller's own key.
*Accept:* client B sends a cancel and an event for a run client A started: the run goes on, the machine receives nothing, and B's receipts say each was refused and why. B answers a ticket of that run, naming the instance and the ticket: the machine receives the answer, and B's receipt says it was taken. A run whose client has been removed, and a root with no timeout, are each cancelled with the `corbie controller` command that cancels a run: each, and everything below it, ends cancelled.

**R-ctl-11** A result or an emit is accepted only for a job the instance is still waiting on, from the runner the job was sent to.
*Accept:* a result and an emit arrive for a job whose instance has moved on without it: both are dropped, and no step entry is written. A trusted runner that was not sent the job writes a well-formed result for it: dropped, and the instance still waits for the job.

**R-ctl-13** The controller takes one sender's messages in the order they were sent.
*Accept:* a client sends the events `a`, `b`, `c` to a waiting statechart before any pass: the step entry holds them in the order `a`, `b`, `c`.

**R-ctl-14** A message that arrives more than once harms nothing, because each kind is checked against the instance's state: a result the instance is not waiting for is dropped, a start for an instance that exists is dropped, and an answer to a ticket already answered changes nothing.
*Accept:* the same result is left twice: the machine receives one `done`, and the second is gone from the inbox with no step written for it. The same start is delivered twice: there is one instance with one first step, and the receipt reads the same after the second delivery as after the first. Two different answers are sent for one ticket: the machine receives the first only, and the second's receipt says it was not the one taken.

**R-ctl-19** A message for an instance that has ended is refused and changes nothing.
*Accept:* parity case 41: an answer to a ticket, sent after the run was cancelled, is refused as for an instance that has finished.

**C-ctl-01** A message the controller sends itself stays good for as long as the instance it is for is kept. A controller can be away for longer than a few minutes, and a child's ending that has stopped being good by then leaves its parent waiting for something that will never come. *Test:* `ctl_message_to_self_outlives_absent_controller`

**C-ctl-02** A start is given to a step by itself, and any other message for that instance waits for the step after. Inbox order is the order of names, so a message can sort ahead of the start it depends on. *Test:* `ctl_start_taken_alone_before_other_messages`

**C-ctl-03** Within the events of one step, a cancel or a passed timeout is acted on first, and no event after it is given to the machine. An instance that is to end must not first start work on the strength of an event that happened to sort ahead. *Test:* `ctl_cancel_or_timeout_beats_events_in_one_step`

**C-ctl-06** The receipt for a refused message is written before the message is deleted. Stopped between the two the other way round, the controller leaves a client with no answer and nothing waiting that would ever produce one. *Test:* `ctl_refusal_receipt_written_before_message_deleted`

## Taking a step

**R-ctl-20** The controller finds an instance's latest step by reading step entries from the number its head gives to the last one there.
*Accept:* an instance's head says step 3 while step entries 4 and 5 exist: the next step written is 6.

**R-ctl-21** A step is given the events from every message and every due wake waiting for the instance, in order, and its result is written as the next numbered step entry, only-if-new, naming the messages and wakes it used.
*Accept:* three results from one runner are waiting for one flow when a pass begins: the record gains exactly one more step entry, numbered one above the last, holding three events in the order the results were sent and naming the three results. A step set off by a timer names that wake.

**R-ctl-25** After the step is written the controller carries out what the entry asks for, each thing written only-if-new under a name the entry gives: it sends script jobs, starts children, sends events, writes wakes, copies blobs to a parent and writes receipts.
*Accept:* for each kind in turn, after a step that asks for it the thing is in the store: the job in a runner's inbox, the child's start and the event in the controller's own inbox, the wake under its time, the blob in the parent's record, the receipt under its message's name. Carry out one entry twice: the listing of every inbox, of the wakes, of the receipts and of the records is the same after the second time as after the first.

**R-ctl-27** After carrying out, the head is brought up to date with the step, only-if-unchanged and only forward.
*Accept:* after a step numbered 5 that leaves two jobs waiting, the head says step 5, running, two jobs. A copy that is slow in carrying out step 5 comes to write its head after another copy has brought the head to step 6: the head still says step 6.

**R-ctl-29** Last, the messages and wakes the entry names are deleted, and only then; every message and wake it does not name stays.
*Accept:* stop the controller at each point before the last action: the messages and wakes are in the store every time. After a pass that completes a step, none of the messages and wakes the entry names is in the store, and every one it does not name is still there.

**R-ctl-31** A controller stopped at any point leaves the same messages and wakes waiting, and the next pass, starting again from finding the latest step, repeats only what is harmless.
*Accept:* for a run with a script job, a child, a timer and a second attempt, stop the controller after each single write to the store in turn and let a fresh controller carry on: every such run ends as a run never stopped does, each machine given the same events and asking for the same things, each script job is sent once and run once, and nothing is left in the inbox or among the wakes.

**R-ctl-32** Some steps are the controller's own and the machine is not consulted: an attempt failed and another is due, a job is being held for approval, a result did not come in time, the run was cancelled or its timeout passed. They are written in the record like any step.
*Accept:* a job with two attempts fails its first: the record gains a step entry that shows attempt one's failure and asks for the wake for attempt two, with the machine's state unchanged. A step asks for a job marked for approval: a step entry shows the job held and the ask started for it, and when the ask approves, a further entry shows it released. A job goes to a runner that never answers: when its due time passes the record gains an entry that shows the job timed out. A cancel and a passed timeout are each written as the instance's last step.

**R-ctl-36** When a machine's last step both sends events and ends the machine, the events are sent before the ending, so a parent sees a child's last words before its result.
*Accept:* parity case 93: the parent receives the child's event before the child's result, in every run.

**C-ctl-09** For a message whose instance has no head, the controller looks for a first step entry before it decides the instance does not exist. A controller stopped after writing the first step and before writing the head leaves exactly that, and the start must then finish the step, not be refused or begin another. *Test:* `ctl_first_step_without_head_is_finished`

**C-ctl-11** Before it writes another step, the controller finishes the latest one: while any message or wake the latest entry names is still there, that entry's carry-out, head and deleting come first, and those messages are never given to a step again. Given to a step again they would make the machine act twice on one event, and a new step written over an unfinished one leaves jobs that were asked for and never sent. *Test:* `ctl_unfinished_step_finished_before_next_is_written`

**C-ctl-16** A copy that finds the step's number taken throws away its own result and acts only on the entry that was written. Two copies can see different messages waiting, so their results can differ, and carrying out the loser's sends jobs no entry asks for. *Test:* `ctl_loser_of_step_carries_out_winners_entry`

**C-ctl-49** A copy that read a message before another copy took its step, carried it out and deleted the message carries nothing out again; and copies begin at different places among the instances that have something waiting. A host that calls once for every message runs many copies over one inbox, and each repeating what another has finished costs a run several times its requests. *Test:* `ctl_copy_that_read_too_early_finishes_nothing`

**C-ctl-17** Whether a message is past its time is asked only when it is about to be given to a step. A message the latest entry already names is deleted by that step's last action, however old it is. A controller stopped between writing a step and carrying it out, and away for longer than the message is good for, would otherwise drop the one thing that leads it back to the unfinished step. *Test:* `ctl_message_past_its_time_named_by_step_still_finishes_it`

**C-ctl-19** A wake's time is fixed by the step entry and not by the clock at the moment of carrying out. The time is part of a wake's name, so a carry-out repeated later with a time of its own leaves two wakes for one thing. *Test:* `ctl_wake_time_fixed_by_entry`

**C-ctl-20** A job written again by a repeated carry-out is the same job: the same name, the same due time, the same time until which it is good. A runner's mark goes once the due time has passed, and after that only the job's own due time stops it being run a second time. *Test:* `ctl_job_sent_again_keeps_name_and_due_time`

**C-ctl-21** A carry-out too large for one pass still finishes: how far it has got is noted in the record a batch at a time, and a later pass goes on from the note and does not spend its time doing again what an earlier one did. A pass on a serverless host has a fixed time, and one that begins each time from the first job of a thousand never reaches the last. *Test:* `ctl_carry_out_larger_than_a_pass_finishes`

**C-ctl-26** The names of a last step's events sort ahead of the name of its ending, and the events are written first. The parent takes its messages in the order of their names, also when it finds the events and the ending waiting together. *Test:* `ctl_stopped_between_last_events_and_ending_parent_sees_events_first`

**C-ctl-28** A wake that comes due for something the instance no longer waits on is deleted and changes nothing: a job whose result has come, a timer whose state was left, an attempt that was overtaken. *Test:* `ctl_wake_for_something_no_longer_waited_on_does_nothing`

**C-ctl-29** Deleting a wake a step used removes that wake only: a wake of the same name that the same step set again stays. A timer started afresh, or the next attempt, carries the name of the one that has just come due, and clearing by name alone removes the new with the old. *Test:* `ctl_wake_set_again_survives_delete_of_used_one`

## Sending a job

**R-ctl-37** A script job is sent as a message to one runner, signed by the controller and sealed to that runner, carrying everything its runner needs: with the message and the store, and nothing else from the controller, the runner can run the job.
*Accept:* the job in a runner's inbox checks against the controller's card and opens with that runner's key and with no other party's. A runner that has never seen the definition runs the job from the message alone, reading the script, its files and the input from the places in the record that the job names.

**R-ctl-43** A job goes to a runner that has every tag it names, and a job that names no tag may go to any runner.
*Accept:* runner A has the tag `gpu`, runner B has `gpu` and `eu`: a job that names `gpu` and `eu` goes to B every time. Over many jobs that name no tag, with one runner that has tags and one that has none, both receive jobs.

**R-ctl-45** Where several runners qualify the controller picks one from the request and the attempt, taking no account of load, so the same attempt always goes to the same runner and a later attempt goes to a different one.
*Accept:* one step entry is carried out by two copies at once and again by a third later, with three runners that qualify: the job is in one runner's inbox and in no other's. A job with three attempts and two runners that qualify, the first picked of which never answers: attempt two is in the other runner's inbox. Giving one of two qualifying runners a hundred waiting jobs changes none of the picks.

**R-ctl-48** A job whose tags no runner has fails before anything is sent.
*Accept:* a job names a tag no enrolled runner has: the machine receives `failed` with an error that names the tags, and no runner's inbox gains anything.

**R-ctl-50** Every job has a time by which its result is due; if none has come by then the machine receives a timeout in its place, and a result that has arrived always wins over its own due time, however late the controller is in looking.
*Accept:* a job with a one-minute time limit and no further attempt goes to a runner that never answers: after the due time the machine receives `failed` as a timeout. A result is left one second before its job's due time and the controller's next pass comes an hour later, with the result and the due wake both waiting: the machine receives `done` with the output, and the record holds no timeout.

**C-ctl-32** A due wake for a job is not acted on in a pass that has not yet read the inbox for that job's result. A pass that runs out of time between listing wakes and reading messages would otherwise answer `timeout` for a job whose result is waiting. *Test:* `ctl_due_wake_not_acted_on_before_inbox_is_read`

**C-ctl-33** The same attempt goes to the same runner also when it is carried out twice and the set of runners that qualify has changed in between; which runner it went to is settled once and found by every later copy. Two copies, or one copy repeating a carry-out, read the trusted runners at different moments, and each choosing from what it sees sends one job to two runners. *Test:* `ctl_attempt_goes_to_one_runner_when_runners_change`

**C-ctl-34** A job fails for want of a runner only when every trusted runner's tags have been read. If one cannot be read because the store is out of reach, the pass ends instead; passing it over fails a job for good because of a moment's trouble. *Test:* `ctl_unreadable_runner_ends_pass_not_run`

## What the controller does for every machine

**R-ctl-53** The controller makes the attempts a job asks for and gives the machine only the final outcome. Only an error or a timeout is tried again; a denial and a cancel are not, and a job that asks for no attempts is tried once.
*Accept:* parity case 11: the flow receives one result for each stage, whatever the attempts. A job with three attempts whose script is refused a permission: the job is sent once, and the machine receives `failed` with the status denied; the same for a job that is cancelled.

**R-ctl-55** Each attempt after the first is made a little later than the last, and not at once.
*Accept:* after attempt one fails, no job for attempt two exists until a wake for it comes due; then it is sent. With three attempts that all fail, the wait before the third is longer than the wait before the second.

**R-ctl-56** The record shows each attempt, under the job's name with the attempt's number added, and each attempt at a job that is a machine is a new instance of the child.
*Accept:* after a job that took two attempts, the record holds the emits of both under names that differ only by the attempt's number, and a step entry shows the first failure. Parity case 62 for a child.

**R-ctl-58** Values a failed attempt emitted stand, and the same values from a later attempt are not delivered twice.
*Accept:* attempt one emits two values of a name the machine acts on and then throws: the machine has received both, they remain in the record, and nothing withdraws them when attempt two starts. Attempt two then emits the same two values and a third, and returns: the machine receives the third only, so three events of that name in all, and one `done`.

**R-ctl-68** A script job is cancelled by a message to the runner it was sent to.
*Accept:* an instance with a job running is cancelled: a cancel for that job is in the inbox of the runner the job went to, and in no other's.

**R-ctl-69** A timer a machine asks for is delivered to it as a `timer` event in the first pass at or after its time; one the machine has cancelled is not delivered.
*Accept:* parity case 19: the eight-second timer never arrives. A state with a one-second and a two-second timer, each leaving for a different state, under `corbie test`: the one-second timer arrives alone about a second after the start and the other is never delivered; parity case 22's two timers arrive in one step only when the controller first looks after both are due, and the chart then does as R-chart-64 says.

**R-ctl-71** A machine whose timeout has passed is ended by the controller, fails as out of time, and everything below it is cancelled.
*Accept:* parity case 98: the bottom chart ends with the status timeout and an error saying its timeout passed. Parity case 13's flow is run: the flow ends as out of time, and its child is listed as `cancelled`.

**C-ctl-35** The n-th value of a name that a later attempt emits is taken to be the n-th an earlier attempt emitted, and is passed to the machine only if no attempt has delivered an n-th before. A script that is run again emits its values again from the first, and a machine that starts work for each one would do every item's work twice. *Test:* `ctl_later_attempt_emits_not_delivered_twice`

**C-ctl-36** The names of events sent on one link sort in the order the step asked for them, and after the names of every earlier step's events on that link. The receiver takes messages in the order of their names and has nothing else to order them by. *Test:* `ctl_events_on_one_link_arrive_in_order_sent`

**C-ctl-37** A cancel sent to a runner stays good until its job's due time. A job can wait in an inbox for a free place longer than a few minutes, and a cancel that has stopped being good by the time the job is taken leaves it to run in full for a machine that has ended. *Test:* `ctl_cancel_good_until_jobs_due_time`

**C-ctl-38** A cancel for a child whose start has not yet been taken is not dropped as being for no instance. A parent can end in the pass after it asked for a child, and a cancel dropped then lets the child start afterwards and run under nothing. *Test:* `ctl_cancel_reaches_child_not_yet_started`

## Housekeeping

**R-ctl-98** The controller removes what is past keeping (finished instances, the older history of instances still running, receipts, and uploads for runs that never started) a little at a time as part of its passes, and a pass that has machines to advance does that first.
*Accept:* with ten thousand finished instances past keeping and messages waiting, every waiting message is handled in the pass before anything is removed, each pass ends within its time having removed some, and after enough passes all are gone. What is removed and when is as the Instances rules say (R-inst-33 to R-inst-41, R-inst-62, R-blob-29).

**C-ctl-44** How far housekeeping has got is not kept in memory: a controller started afresh for each pass still gets through all of it. On a serverless host every pass is a fresh start, and housekeeping that begins again from the first name each time never reaches the last. *Test:* `ctl_housekeeping_gets_through_all_across_fresh_starts`

**C-ctl-45** A message or a wake left for an instance that has been removed is deleted. Nothing else will ever take it, and every pass would list it. *Test:* `ctl_message_for_removed_instance_dropped`

## When things go wrong

**R-ctl-100** If the store cannot be reached nothing fails: the pass ends, and what was waiting is still waiting for the next one.
*Accept:* the store refuses every request partway through a step and then answers again: no instance is marked failed, no start is refused, no sender is treated as a stranger, no receipt says refused, and the run ends as it would have.

**R-ctl-101** A message too large to send, or a result too large to deliver, fails the job it belongs to with a reason, and nothing is left waiting for something that will never come.
*Accept:* a job whose input, written inline, is larger than a message may be: the machine receives `failed` with an error saying the job was too large to send, and no runner's inbox gains anything. A child ends with an inline output too large to pass to its parent: the parent receives `failed` for that job with an error saying the result was too large. After either, the instance's head no longer counts that job, and no wake for it changes anything later.

**C-ctl-47** A step that could not finish because the store was out of reach, or because its pass was ending, is not a step that failed: nothing is written for it, and it is taken again in the next pass. The two look alike from where the step is called and must be told apart there. *Test:* `ctl_step_that_cannot_reach_store_is_not_a_failed_step`

**C-ctl-48** A job that fails before anything is sent reaches its machine as a result does: as a message in the controller's own inbox, written during the carry-out under a name the entry gives. A failure decided during a carry-out and held in memory is lost when the controller stops, and the machine waits for ever. *Test:* `ctl_job_that_cannot_be_sent_fails_though_controller_stops`

## Interfaces

Uses:

- **Store**: reads and writes everything in its own area with the three ways of writing, lists its inbox and its wakes in order, and writes jobs and cancels into runners' inboxes.
- **Role**: is run as passes, and is told how long a pass may take.
- **Identity**: asks whether a sender is trusted, reads a client's ceiling and a runner's tags, opens messages sealed to it, signs heads, and seals jobs, cancels and receipts; a message it does not accept is deleted without a receipt (R-id-47).
- **Instance**: writes heads, step entries and receipts in the form that area gives, moves a head from running to finished, and removes what is past keeping by that area's rules.
- **Machine**: gives a type's step the definition, the state before and the events, and takes back the new state and the requests; checks a definition with the same code as the client (R-mach-17); starts children, counts depth and jobs, makes tickets and takes their answers, and holds jobs for the ask, as the Machines rules state (R-mach-65 to R-mach-115).
- **Blob**: copies what a child hands up into the parent's record, once and for every type (R-blob-23), and reads the small blobs a step needs.
- **Permission**: puts each layer that applies into a job by name, and reads the permissions a job declares against the lists of those to be asked about (R-perm-42).
- **Secret**: puts into a job the values of the secrets it names and every layer allows (R-sec-14); fails a job that names a secret that is not set (R-sec-20), and denies one that names a secret a layer does not allow.

Used by:

- **Client**: leaves starts, cancels, events and answers in the controller's inbox, and reads the receipts, heads and records the controller writes.
- **Runner**: is sent jobs and cancels; returns results and the emits a machine wants.
- **Flow, Statechart, Agent**: each supplies the step the controller runs, and relies on the controller for attempts, children, timers, tickets, limits and approval.
- **Role**: a controller's pass is the unit that area starts and bounds.
