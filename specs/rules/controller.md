# Controller

The detailed rules behind the [Controller overview](../controller.md). What the controller does for every machine is stated once: attempts, timers and timeouts are here; what a machine sees of children, tickets, limits and approval is under Machines, and what a machine may be held for is under Permissions.

## What arrives

**R-ctl-04** What the controller tells itself, a child's start, a child's ending or an event between a parent and a child, is a message in its own inbox in the store.
*Accept:* stop the controller straight after a parent's step that starts a child: the child's start is in the controller's inbox in the store, and a controller started afresh takes it.

**R-ctl-07** A client may cancel only runs at its own trees' locations, sends an event to a run by its address as a run does (R-ctl-122, R-ctl-123), and may answer any ticket it holds; the controller's owner can cancel any run with the controller's own key.
*Accept:* client B sends a cancel for a run in client A's tree, and an event to it with no `send:` in its ceiling: the run goes on, the machine receives nothing, and B's receipts say each was refused and why. B answers a ticket of that run, naming the instance and the ticket: the machine receives the answer, and B's receipt says it was taken. A run whose client has been removed, and a root with no timeout, are each cancelled with the `corbie controller` command that cancels a run: each, and everything below it, ends cancelled.

**R-ctl-11** A result or an emit is accepted only for a job the instance is still waiting on, from the runner the job was sent to.
*Accept:* a result and an emit arrive for a job whose instance has moved on without it: both are dropped, and no step entry is written. A trusted runner that was not sent the job writes a well-formed result for it: dropped, and the instance still waits for the job.

**R-ctl-13** The controller takes one sender's messages in the order they were sent.
*Accept:* a client sends the events `a`, `b`, `c` to a waiting statechart before any pass: the step entry holds them in the order `a`, `b`, `c`.

**R-ctl-14** A message that arrives more than once harms nothing, because each kind is checked against the instance's state: a result the instance is not waiting for is dropped, a start for an instance that exists is dropped, and an answer to a ticket already answered changes nothing.
*Accept:* the same result is left twice: the machine receives one `done`, and the second is gone from the inbox with no step written for it. The same start is delivered twice: there is one instance with one first step, and the receipt reads the same after the second delivery as after the first. Two different answers are sent for one ticket: the machine receives the first only, and the second's receipt says it was not the one taken.

**R-ctl-19** A message for an instance that has ended is refused and changes nothing. An event sent to `{ at }` or `{ at, key }` goes on to the run that has the key, and one sent to the run by its ID is noted as undelivered.
*Accept:* parity case 41: an answer to a ticket, sent after the run was cancelled, is refused as for an instance that has finished. `Test_ctl_event_for_a_run_that_ended_starts_anew_or_is_noted`.

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

**R-ctl-32** Some steps are the controller's own and the machine is not consulted: an attempt failed and another is due, a job is being held for approval, a result did not come in time, the run was cancelled or its timeout passed, a step of a machine made while running passed a limit. They are written in the record like any step.
*Accept:* a job with `restart: { limit: 1 }` fails its first: the record gains a step entry that shows attempt one's failure and asks for the wake for attempt two, with the machine's state unchanged. A step asks for a job marked for approval: a step entry shows the job held and the ask started for it, and when the ask approves, a further entry shows it released. A job goes to a runner that never answers: when its due time passes the record gains an entry that shows the job timed out. A cancel and a passed timeout are each written as the instance's last step.

**R-ctl-36** When a machine's last step both sends events and ends the machine, the events are sent before the ending, so a parent sees a child's last words before its result.
*Accept:* parity case 93: the parent receives the child's event before the child's result, in every run.

**C-ctl-09** For a message whose instance has no head, the controller looks for a first step entry before it treats the instance as one that does not exist. A controller stopped after writing the first step and before writing the head leaves exactly that, and the start must then finish the step, not be refused or begin another. *Test:* `ctl_first_step_without_head_is_finished`

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
*Accept:* one step entry is carried out by two copies at once and again by a third later, with three runners that qualify: the job is in one runner's inbox and in no other's. A job with `restart: { limit: 2 }` and two runners that qualify, the first picked of which never answers: attempt two is in the other runner's inbox. Giving one of two qualifying runners a hundred waiting jobs changes none of the picks.

**R-ctl-48** A job whose tags no runner has fails before anything is sent.
*Accept:* a job names a tag no enrolled runner has: the machine receives `failed` with an error that names the tags, and no runner's inbox gains anything.

**R-ctl-50** Every job has a time by which its result is due; if none has come by then the machine receives a timeout in its place, and a result that has arrived always wins over its own due time, however late the controller is in looking.
*Accept:* a job with a one-minute time limit and no further attempt goes to a runner that never answers: after the due time the machine receives `failed` as a timeout. A result is left one second before its job's due time and the controller's next pass comes an hour later, with the result and the due wake both waiting: the machine receives `done` with the output, and the record holds no timeout.

**C-ctl-32** A due wake for a job is not acted on in a pass that has not yet read the inbox for that job's result. A pass that runs out of time between listing wakes and reading messages would otherwise answer `timeout` for a job whose result is waiting. *Test:* `ctl_due_wake_not_acted_on_before_inbox_is_read`

**C-ctl-33** The same attempt goes to the same runner also when it is carried out twice and the set of runners that qualify has changed in between; which runner it went to is settled once and found by every later copy. Two copies, or one copy repeating a carry-out, read the trusted runners at different moments, and each choosing from what it sees sends one job to two runners. *Test:* `ctl_attempt_goes_to_one_runner_when_runners_change`

**C-ctl-34** A job fails for want of a runner only when every trusted runner's tags have been read. If one cannot be read because the store is out of reach, the pass ends instead; passing it over fails a job for good because of a moment's trouble. *Test:* `ctl_unreadable_runner_ends_pass_not_run`

## What the controller does for every machine

**R-ctl-53** The controller makes the restarts a job's policy asks for and gives the machine only the final outcome. An error or a timeout restarts a job, and with `when: always` a job that ended well too; a denial and a cancel never do, and a job with no policy runs once.
*Accept:* parity case 11: the flow receives one result for each stage, whatever the restarts. A job with `restart: { limit: 2 }` whose script is refused a permission: the job is sent once, and the machine receives `failed` with the status denied; the same for a job that is cancelled. *Test:* `TestRestartAfterAFailure`, `Test_ctl_a_job_with_no_restart_runs_once`, `Test_ctl_a_cancelled_job_is_not_restarted`, `Test_ctl_no_limit_goes_on_until_it_works`

**R-ctl-105** A job past its limit gives its last outcome. A limit with a length of time counts only the restarts in that length of time, and without one it counts them all.
*Accept:* a job limited to one restart in 1h fails on its second attempt, and limited to one in 200ms, restarting every 250ms, it is never past it. *Test:* `Test_ctl_a_limit_in_a_length_of_time_forgets_old_restarts`

**R-ctl-106** A restart after a job ended well is a new job, with a new request name: it is held again if it needs approval, and counts against the limit of jobs. A restart after a failure is the same job, is not held again and is not counted. An ask that ends well is final.
*Accept:* a job held for approval with `{ limit: 2 }` that fails twice is asked about once, and with `{ when: always, limit: 2 }` that ends well three times is asked about three times. With `limits: { jobs: 3 }`, a job with `when: always` fails on its fourth run with `job limit reached`. *Test:* `Test_ctl_a_restart_is_held_only_after_a_job_ended_well`, `Test_ctl_a_new_job_counts_against_the_limit_of_jobs`, `Test_ctl_a_restart_after_a_failure_is_the_same_job`, `Test_ctl_when_always_runs_again_after_it_ends_well`, `Test_ctl_an_ask_ends_when_it_ends_well`

**R-ctl-55** A restart waits for its delay and is never made at once. A delay given as two lengths of time doubles from the first to the second, and starts again once an attempt has stayed up for twice the second.
*Accept:* after attempt one fails, no job for attempt two exists until a wake for it comes due; then it is sent. `1s to 5s` waits 1s, 2s, 4s, 5s and 5s, and 1s again after an attempt that stayed up 10s. A delay a step's process wrote as nothing or less waits the default. *Test:* `Test_mach_a_delay_doubles_and_starts_again`, `Test_mach_a_wait_takes_nothing_on_trust`

**R-ctl-56** The record shows each attempt, under the job's name with the attempt's number added, and each attempt at a job that is a machine is a new instance of the child.
*Accept:* after a job that took two attempts, the record holds the emits of both under names that differ only by the attempt's number, and a step entry shows the first failure. Parity case 62 for a child.

**R-ctl-58** Values an attempt emitted stand, and every value of the next attempt is delivered.
*Accept:* attempt one emits two values of a name the machine acts on and then throws: the machine has received both, they remain in the record, and nothing withdraws them when attempt two starts. Attempt two emits the same two values and a third: the machine receives all three, so it has five. *Test:* `Test_ctl_a_restart_delivers_all_it_emits`

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

## A machine made while running

**R-ctl-103** A step of a machine made while running, and a step after the first of an agent with a `list:` tool, is taken in a process of its own, with a limit on its memory and its time, and a step that passes either fails that machine and nothing else.
*Accept:* `'a'.repeat(2**40)`, `1n<<(1n<<40n)`, `/(a+)+\1b/` and `new Array(2**32-1).indexOf(1)`, each in a made machine's `output:`, a `when:`, a job's input, a timer's time, its timeout, a permission and a child two levels below it: the machine fails, the controller goes on, and another run ends `done`. *Test:* `ctl_made_step_past_a_limit_fails_that_machine`, `ctl_made_step_limits_hold_at_every_place` and `ctl_made_definition_past_a_limit_fails_that_machine`

**R-ctl-104** A made machine runs as any other: only where its steps are taken differs.
*Accept:* a made statechart waits on a ticket, an event and a timer across several steps and ends `done`. A made agent calls a tool and answers, with the stand-in model. *Test:* `ctl_made_statechart_waits_across_steps` and `ctl_made_agent_works_as_any_other`

**C-ctl-50** That a machine was made while running is noted by the controller in the child's start, from what the job that starts it gave, and passes to everything below it. A definition cannot say so or deny it, and a repeated pass reads the same answer from the record. *Test:* `ctl_made_step_of_a_definition_stored_as_a_blob`

**C-ctl-51** Carrying out a step reads no definition: the step says where each job's script or machine is, and the state says how long the instance is kept and which ask it names. A definition may have gone by the time a carry-out is repeated, and a step already written must still finish. *Test:* `ctl_made_step_past_a_limit_stopped_after_any_write` and `ctl_keep_is_in_the_record`

**C-ctl-52** A step past a limit, or whose process ends without an answer, ends the machine in a step of the controller's own, and a controller stopped at any point takes the step again at most once before the machine has ended. A step that was simply repeated would end the controller's passes for ever. *Test:* `ctl_made_step_past_a_limit_stopped_after_any_write` and `ctl_made_step_past_a_limit_is_an_ordinary_failure`

**C-ctl-53** Before a step's process starts, the controller notes in the record that the step is being taken. A copy that finds the note less than a step's time and 5 seconds old leaves the step alone, and the step is taken at most twice with no answer. A step's process can bring its controller down, and copies at work on one step must not count against its machine. *Test:* `ctl_made_step_lost_twice_fails_its_machine` and `ctl_made_step_is_taken_by_one_copy`

**C-ctl-54** A pass takes at most four such steps at a time and leaves the rest for the next pass: at once on a computer, and at the next call on a serverless host. Each may hold a gigabyte, and a pass waits for them. *Test:* `ctl_made_steps_at_once_are_bounded`

**C-ctl-55** What a step's process says is read up to 4 MiB and no further, nests at most 1000 deep, sends at most 1024 events, and with the state it adds to comes to at most 4 MiB. Past any of these the machine fails. It is data: nothing in it is compiled or evaluated, and carrying it out takes time in proportion to it, so what it names of a definition is read once a step, and not once for each thing it names. A step is read on every later pass, and JSON nested past 10000 cannot be written at all, so such a step would be taken for ever. *Test:* `ctl_made_step_answer_is_bounded`, `ctl_made_step_answer_past_a_steps_limit_fails_its_machine`, `ctl_made_step_state_past_a_steps_limit_fails_its_machine`, `ctl_answer_nested_too_deep_fails_its_machine`, `ctl_asks_of_a_definition_are_read_once_a_step`, `ctl_answer_with_many_sends_fails_its_machine` and `sec_named_takes_time_in_proportion_to_the_list`

**C-ctl-56** A step of any other machine, and the first step of an agent with a `list:` tool, is taken in the controller's own process. *Test:* `ctl_owner_step_spawns_no_process` and `ctl_listed_agent_steps_after_its_first_are_taken_in_a_process`

**C-ctl-57** A step's process gives back what the machine asks for and nothing of the controller's: no name in the store, no runner, no status, no flag, no receipt, nothing of what the step used. The controller works out each of those by the code that serves every machine. That process can be taken over through a bug in what it runs, and what the controller takes from it is the only way it reaches the store. *Test:* `ctl_answer_says_nothing_of_what_a_step_used`, `ctl_answer_cannot_mark_a_job_as_from_above`, `ctl_answer_cannot_drop_a_made_machines_narrowing`, `ctl_answer_cannot_send_to_another_instance`, `ctl_answer_cannot_forge_a_receipt`, `ctl_answer_cannot_cancel_another_instances_job`, `ctl_answer_cannot_take_a_message_name`, `ctl_answer_cannot_end_its_machine_by_saying_so` and `ctl_answer_of_a_made_machine_sends_to_no_address`

**C-ctl-58** A made machine's name, type, `ask_about`, `keep`, limit on jobs, deny list, scripts and machines are read by the controller from its definition as plain JSON, in which nothing is compiled or evaluated. Its `keep` is no longer than its parent's, and its time limit, which an expression may give and so comes from the answer, no longer than the job that starts it allows. An answer that gave these could clear what approval holds or outlive what the record above it may. *Test:* `ctl_answer_cannot_change_what_the_controller_holds`, `ctl_answer_cannot_outlast_its_job`, `ctl_keep_of_a_made_machine_is_no_longer_than_its_parents` and `ctl_answer_cannot_pass_what_its_definition_limits`

**C-ctl-59** Whether a job of a made machine is held is worked out by the controller from the job as it will be sent and the lists it holds. Every ask is read from a definition and filled in by the controller from the input of the machine that declares it, and takes nothing from an answer but the place it is written. It runs with the layers of that machine, so a script an answer chose would run with those layers, and an expression an answer wrote would run in the controller. *Test:* `ctl_answer_cannot_clear_what_approval_holds`, `ctl_answer_cannot_fill_an_owners_ask` and `ctl_answer_cannot_write_a_listed_agents_ask`

**C-ctl-60** A name a step's answer gives for a job, a timer or a ticket, and a made definition's own, is words of letters, digits, `_` and `-` with `.` between them, of at most 100 characters; a job is not named `ask` and its name does not end in `.cancel`. Any other fails that machine. These go into names in the store and into a head, a name past what the store holds would stop every pass, and a job named `ask` decides what its machine holds. *Test:* `ctl_answer_with_a_name_no_definition_holds_fails_its_machine`, `ctl_made_definition_with_a_name_past_the_limit_fails_its_machine` and `ctl_names_of_the_types_pass_the_check_of_an_answer`

**C-ctl-61** No step entry an answer leads to can end a controller, or stop its passes, when it is written, carried out, read after a restart or taken further. A step's process is a boundary, and a step that ended the controller would be found again by the next one. *Test:* `ctl_answer_holding_nothing_ends_no_controller`, `ctl_answer_with_strange_values_ends_no_controller`, `ctl_answer_ending_with_a_reference_to_nothing_ends_as_any_other`, `ctl_answer_of_a_listed_agent_cannot_send_to_an_address` and `ctl_stored_step_with_hostile_content_ends_no_controller`

**C-ctl-62** A pass takes a step in a process of its own only if the step's whole time and 5 seconds fit in the time the host has left, and otherwise the step waits for the next call with no try spent. A host whose calls are too short for that fails the machine and says so. A host stops a pass at its limit, and a step cut off twice for that would fail a healthy machine. *Test:* `ctl_made_step_waits_for_a_call_with_time_for_it` and `ctl_made_step_too_long_for_any_call_fails_its_machine`

**C-ctl-63** A message the controller sends itself that finds its name holding another message fails what it was for, with the reason. A carry-out done again finds the same message there, and that is no fault. A runner's inbox cannot be read, and only one job has a job's name. A write that goes unnoticed leaves a machine waiting for something that will never come. *Test:* `ctl_message_finds_its_name_taken` and `ctl_carry_out_done_again_finds_only_its_own_messages`

**C-ctl-64** A machine made while running has no ask of its own. What it holds goes to the nearest ask above it that was not made while running, and it hands that ask down to the machines below it. With none, the job is denied with a note that says a machine made while running uses only an ask above it. A made machine that writes an `ask:`, or a part's `ask:`, fails as it starts, with the reason. Nothing it wrote of an ask is kept, so a step's process that skips that check gains nothing. A machine run as an ask for it is not made while running, unless that ask gave a stored definition, so its own `ask:` and what it holds stand. A definition produced at run time would otherwise replace the decider the owner named, and one written into what a machine hands down would do so a level below. *Test:* `ctl_made_machine_has_no_ask_of_its_own`, `ctl_made_machine_hands_down_the_ask_above_it`, `ctl_made_machine_that_writes_an_ask_fails`, `ctl_made_machine_with_no_ask_above_is_denied`, `ctl_ask_that_is_a_machine_runs_for_a_made_machine`, `ctl_owners_ask_machine_for_a_made_machine_keeps_its_own_ask`, `agent_made_while_running_has_no_ask_of_its_own` and `agent_part_ask_is_reached_from_a_made_machine_below_its_tool`

**C-ctl-65** That an agent has a `list:` tool is read by the controller from the definition, as plain JSON, and every step after its first is taken in a process of its own. Nothing a step's process gives back has a say in it. A list's shapes come from outside the definition, and a step compiles them: in the controller, one would stop every run. A step past a limit fails that agent, with the reason. *Test:* `ctl_listed_agent_steps_after_its_first_are_taken_in_a_process`, `ctl_listed_step_past_a_limit_fails_that_agent` and `ctl_listed_shape_past_a_limit_fails_that_agent`

**C-ctl-66** A client's start names a location, and its run begins from the file recorded there, which the controller copies into the run's record; a definition the start names is not read, and a start that names no location is refused. A definition a client wrote would be read in the controller as an owner's is, and one that ends the process would end the controller on every pass. *Test:* `ctl_client_start_names_a_location`

## Trees and locations

**R-ctl-107** A submit records a directory as a tree at a prefix, and each of its files with a type line at its location: the file's path under the prefix, without `.yaml`. A submit changes no other tree.
*Accept:* `house/devices/boiler.yaml` submitted at `/home` is recorded at `/home/devices/boiler`, and a file with no type line is at no location. A second tree submitted at `/work` leaves `/home`'s locations and runs as they were.

**R-ctl-108** A submit is refused, with its receipt saying why, at a prefix no `at:` entry of the client's ceiling matches, inside or above another tree, at a tree another client owns, at `/`, or with a location below another's or outside the prefix. The refusal names the tree or the owner, and for the ceiling the line that allows it.
*Accept:* `Test_ctl_submit_needs_an_at_entry` and `Test_ctl_submit_is_refused_naming_the_tree`.

**R-ctl-109** The receipt of a submit says what was done at each location, the directory last submitted there when it was another, and the secrets and values the files use that are not set at their locations or above, which refuses nothing.
*Accept:* `Test_ctl_submit_receipt_names_what_it_did`.

**R-ctl-110** A file with a problem keeps what was recorded at its location before, and its runs, and the rest of the submit is done.
*Accept:* a tree's singleton is running; its file is changed to one that does not read and another file is added: the new file's singleton starts, and the first runs on.

**C-ctl-78** A submit can be taken again after a stop at any write and leaves what one never stopped leaves. The tree's entry is written first, naming the submit and what it does, and a submit taken again does exactly that, however late it is; every later write is only-if-new, writes what is there already, or deletes. Its receipt names as cancelled the runs still running when it is taken again, so a cancel another copy has carried out meanwhile is not named. Worked out again after its first writes, or refused as too late, a submit would find its own files recorded and do nothing to the runs it was to cancel or start. *Test:* `ctl_submit_stopped_after_every_write`

**C-ctl-79** The controller reads a submit's files, and every blob they name, only from that submit's own uploads, and keeps a tree's entry and each location's file sealed to itself. A file named elsewhere could be another client's upload, and a location's file holds the keys of its blobs. *Test:* `ctl_submit_reads_only_its_own_uploads`

## Starting runs

**R-ctl-111** A submit starts a singleton when its location has no running run, and starts nothing for a singleton that runs. A run of a transient location is started by its tree's client naming the location; a start of a singleton or keyed location by a client, or of a location in another client's tree, is refused.
*Accept:* `Test_ctl_singleton_is_started_once` and `Test_ctl_client_starts_its_own_transient_locations`.

**R-ctl-112** A run at a location starts from the file recorded there, which the controller copies into the run's record at its first step, and keeps that definition whatever a later submit records.
*Accept:* a transient file is changed with `update: drain` while a run of it waits: that run ends on the old definition, and the next run started there on the new one.

**R-ctl-113** A run reads the values set at its location or above once, at its first step, the one set nearest winning, and keeps them; a job's secrets are those set nearest its run's location. A child runs at its parent's location, with its parent's values and context. A value a run reads that is not set fails it, naming the value and the location.
*Accept:* `Test_ctl_values_are_read_at_the_first_step` and `Test_ctl_values_and_secrets_by_location`.

**R-ctl-114** A root a client or a submit starts counts its depth limit from its own file's `limits: { depth }`, sixteen by default.
*Accept:* parity case 79.

## A changed or removed file

**R-ctl-115** A changed file with `update: restart` has its root runs cancelled, and a singleton is started again on the new file. A file whose lifetime changed counts as removed and recorded anew. With `update: drain` the runs finish on the definition they started with.
*Accept:* `Test_ctl_singleton_restart_has_no_overlap`.

**R-ctl-116** A file gone from the tree has every root run at its location cancelled, and its location removed. A child running at the location ends with its parent.
*Accept:* `Test_ctl_removal_cancels_only_root_runs`.

**C-ctl-80** A singleton started again starts once the run it replaces has ended: the start names the run running at its location, or with none the run its key's file names, and waits while that run is running. A later submit's start of the same singleton takes its key's file, and an earlier start that finds it so is left. Two runs of one singleton would act on the same thing at once. *Test:* `ctl_singleton_restart_has_no_overlap`

**C-ctl-81** The runs a submit cancels are found among its tree client's running heads, by their location, and only roots are sent a cancel. A child runs at its parent's location, and a cancel of its own would end it while its parent waits on it. *Test:* `ctl_removal_cancels_only_root_runs`

## Sending and calling

**R-ctl-119** A send to an address names the run it goes to while the sender's step is worked out, and the controller writes the event when it carries out the step, with the sender's address in `event.from`: `{ at, run }`, with `key` for a keyed run, or `{ client }`. The sender's record names the run the step named, or the run it started, and the receiving run's record names the sender.
*Accept:* parity cases 108 and 114. `Test_chart_send_to_a_key_starts_a_run_then_delivers` and `Test_ctl_event_from_is_the_senders_address`.

**R-ctl-120** A run takes an event sent to it by address only if its `receive:` lists the sender, by default every location and the client of its own tree. The tree's own client is always listed. A run checks the definition it started with, and a run about to start checks its location's file.
*Accept:* parity cases 116 and 117. `Test_ctl_receive_default_is_own_tree_and_owner`, `Test_ctl_receive_list_names_who_may_send` and `Test_ctl_receive_mapping_is_by_event`.

**R-ctl-121** A send that no run takes is noted as undelivered in the sender's record, or refused in a client's receipt, and fails nothing: the receiver's `receive:` does not list the sender; no singleton is running at `{ at }`; the run named by its ID has ended or is not at the location the address names; the run is a flow or an agent; the key is empty or past 256 characters; the location has nothing recorded, is not keyed, or is at its `limits: { runs }`; the event has a name of Corbie's or a timer's or a ticket's name of the receiver; the address is a client's.
*Accept:* parity cases 109, 119 and 120. `Test_chart_send_to_a_singleton_with_no_run_is_undelivered`, `Test_ctl_send_to_a_run_not_at_its_address_is_undelivered`, `Test_ctl_send_to_a_flow_or_agent_run_is_undelivered`, `Test_ctl_send_to_a_location_that_was_removed_is_undelivered` and `Test_ctl_send_to_client_is_undelivered`.

**R-ctl-122** A run's send to an address needs `send:` for the location in every permission layer of the run: its definition's lists, the lists it was started under and its tree client's ceiling. The runner's ceiling takes no part. A send a layer refuses ends the run as `denied`, naming the layer and the line that allows it.
*Accept:* parity case 118. `Test_ctl_send_denied_by_a_layer_fails_the_chart`.

**R-ctl-123** A client's send to an address is checked as a run's is: against its ceiling's `send:` entries unless the client owns the location's tree, and against the receiver's `receive:`. The controller writes the client's name as the sender, whatever the client wrote. The receipt names the run that took the event and whether the machine acted on it, or says why it was refused.
*Accept:* parity case 121. `Test_ctl_owner_client_needs_no_send_entry`, `Test_ctl_other_client_needs_send_and_receive`, `Test_ctl_client_send_receipt_names_the_run_and_whether_it_acted` and `Test_ctl_client_cannot_write_its_sender`.

**R-ctl-124** `limits: { runs }` is checked while the sender's step is worked out, by listing the key files at the location, and again when a message moves on to start a run. A send to a new key past it is undelivered, and sends to running keys still go through.
*Accept:* parity case 113. `Test_ctl_limit_runs_caps_new_keys` and `Test_ctl_limit_runs_checked_when_a_message_moves_on`.

**R-ctl-125** A run started by a send is a root of its location's tree client, started from the file recorded there. It counts one level below its sender, and fails as it starts if that passes its own file's depth limit, and a run that fails as it starts adds the reason to its sender's record.
*Accept:* `Test_ctl_chain_of_sends_ends_at_the_depth_limit` and `Test_ctl_send_whose_data_does_not_fit_fails_in_the_senders_record`.

## Keyed runs

**R-ctl-126** A send to a key at a keyed location with no running run starts one, with the event's data as its input, and then gives it the event. Further sends to the key go to that run. When it ends, the next send to the key starts a fresh one.
*Accept:* parity case 115. `Test_chart_send_to_a_key_starts_a_run_then_delivers`, `Test_ctl_key_is_free_once_its_run_ends` and `Test_ctl_event_for_a_run_that_ended_starts_anew_or_is_noted`.

**C-ctl-67** A run takes its key's file only-if-new after its definition has been read and its location's `receive:` takes the event, and before anything of the run is written; a file that names it already is its own. A run that loses has done nothing, and its start moves on to the run that won, which takes it as an event. A claim made after the step would have a machine act on an event that a second run also acts on. *Test:* `ctl_two_senders_race_for_one_key`, `ctl_two_copies_race_for_one_key` and `ctl_two_senders_stopped_after_every_write`

**C-ctl-68** A run's last step is written, then its key's file is deleted, then the step is carried out. An event sent to `{ at }` or `{ at, key }` leaves the inbox only when a step of a running run used it, or when the run's last step shows it was taken. A run that cannot take it leaves it, and the controller rewrites it in place, in one write, for the run that has the key or, with none at a keyed location, the run it starts: every event sent to a key carries what would start a run. A send reads the file once and names the run in its step, so a carry-out done again goes to that run and finds the event taken. An event dropped at a run that had just ended would be lost, and a second message for it would give a gap between two writes. *Test:* `ctl_key_stopped_after_every_write`, `ctl_event_for_a_run_that_ended_starts_anew_or_is_noted`, `ctl_event_in_flight_goes_to_the_run_that_has_the_key_now` and `ctl_singleton_restart_passes_a_send_in_flight_to_the_new_run`

**C-ctl-69** A key's file naming a run that has ended, or one with no step and no start waiting, is replaced only-if-unchanged by the next run that takes the key. A file whose start is still waiting is kept. A controller can be stopped between any two writes, and a file left behind must not block its key for ever. *Test:* `ctl_stale_key_file_blocks_no_key` and `ctl_key_file_of_a_start_still_waiting_is_kept`

**C-ctl-70** A run started by a send holds a copy, in its own record, of its location's file, of what that names and of the blobs the event names, made after it takes its key and before its first step. It is a root and outlives the record of its sender and a submit that changes its file. A run that is refused or loses leaves nothing in the store. *Test:* `ctl_keyed_root_outlives_its_sender`

**C-ctl-71** Events from one sender to one run arrive once each, in the order sent. Each is written one after another; one sent to `{ at }` or `{ at, key }` is the first event of its step, and waits while an earlier one from the same sender to the same key is on its way to another run; one sent to a run by its ID waits while an earlier one from the same sender to that location is on its way to another run. A run remembers the last message of each of its 64 most recent senders, a client among them, and takes as a copy one that does not sort after it. A step's later events are not acted on once an event ends the machine, and a second message overtaking a first would be taken for a copy of it. *Test:* `ctl_keyed_events_arrive_once_in_order`, `ctl_keyed_event_is_never_given_after_the_run_ends`, `ctl_carry_out_done_again_gives_a_key_no_event_twice`, `ctl_event_on_its_way_is_not_overtaken`, `ctl_direct_send_waits_behind_a_moved_send`, `ctl_client_sends_arrive_in_order` and `ctl_senders_with_names_that_begin_alike_are_apart`

**C-ctl-72** A run started by a send takes nothing from its sender but how far down it is: no layer, no label, no ask and no value, and a seed of its own. The controller works these out from the location and never from an answer. A sender's layers would otherwise follow the event into a tree that is not its own, and runs that shared a seed would issue one ticket. *Test:* `ctl_sent_root_takes_nothing_from_its_sender` and `ctl_keyed_runs_of_one_sender_share_no_ticket`

**C-ctl-73** A run started by a send counts one level below its sender, so a chain of sends that each start a run ends where a file's depth limit is passed. Nothing else would bound a chain, since no run in it is a child of another. *Test:* `ctl_chain_of_sends_ends_at_the_depth_limit`

**C-ctl-74** A send to a key costs one read of its key's file, read side by side with the step's others, and one read of the location's file when it starts a run. It lists the location's key files only where the location has `limits: { runs }`, once a step. A machine takes all the events waiting in one step, so a step sends to as many keys as there are events to send on, and a step that does not fit in a pass is never written. *Test:* `ctl_send_to_a_key_lists_nothing` and `ctl_step_sends_to_any_number_of_keys`

**C-ctl-75** A step of a machine whose type cannot send to an address cannot say it does: the machine fails. A listed agent's answer is the agent's word, and no agent's definition sends. *Test:* `ctl_answer_of_a_listed_agent_cannot_send_to_an_address`

**C-ctl-76** A machine made while running that asks to send to an address fails, whatever the job above it lists and whatever its answer says of itself. It may have been written by a model, and a forged event sent to a keyed run looks like any other to that run. *Test:* `ctl_made_machine_sends_to_no_address` and `ctl_answer_of_a_made_machine_sends_to_no_address`

**C-ctl-77** A step's events are written one after another in the order sent, after what the step's other asks write. A run takes a message that sorts before one it has heard as a copy, so an event written out of order by a controller stopped between two writes would be lost. *Test:* `ctl_events_of_a_step_stopped_after_every_write`

## Housekeeping

**R-ctl-98** The controller removes what is past keeping (finished instances, the older history of instances still running, receipts, and uploads for runs that never started) a little at a time as part of its passes, and a pass that has machines to advance does that first.
*Accept:* with ten thousand finished instances past keeping and messages waiting, every waiting message is handled in the pass before anything is removed, each pass ends within its time having removed some, and after enough passes all are gone. What is removed and when is as the Instances rules say (R-inst-33 to R-inst-41, R-inst-62, R-blob-29).

**C-ctl-44** How far housekeeping has got is not kept in memory: a controller started afresh for each pass still gets through all of it. On a serverless host every pass is a fresh start, and housekeeping that begins again from the first name each time never reaches the last. *Test:* `ctl_housekeeping_gets_through_all_across_fresh_starts`

**C-ctl-45** A message or a wake left for an instance that has been removed is deleted. Nothing else will ever take it, and every pass would list it. *Test:* `ctl_message_for_removed_instance_dropped`

## When things go wrong

**R-ctl-100** If the store cannot be reached nothing fails: the pass ends, and what was waiting is still waiting for the next one.
*Accept:* the store refuses every request partway through a step and then answers again: no instance is marked failed, no start is refused, no sender is treated as a stranger, no receipt says refused, and the run ends as it would have.

**R-ctl-101** A message too large to send, or a result too large to deliver, fails the job it belongs to with a reason, and nothing is left waiting for something that will never come.
*Accept:* a job whose input, written inline, is larger than a message may be: the machine receives `failed` with an error saying the job could not be sent because it is too large, and no runner's inbox gains anything. A child ends with an inline output too large to pass to its parent: the parent receives `failed` for that job with an error saying the result is too large. After either, the instance's head no longer counts that job, and no wake for it changes anything later.

**C-ctl-47** A step that could not finish because the store was out of reach, or because its pass was ending, is not a step that failed: nothing is written for it, and it is taken again in the next pass. The two look alike from where the step is called and must be told apart there. *Test:* `ctl_step_that_cannot_reach_store_is_not_a_failed_step`

**C-ctl-48** A job that fails before anything is sent reaches its machine as a result does: as a message in the controller's own inbox, written during the carry-out under a name the entry gives. A failure found during a carry-out and held in memory is lost when the controller stops, and the machine waits for ever. *Test:* `ctl_job_that_cannot_be_sent_fails_though_controller_stops`

## Interfaces

Uses:

- **Store**: reads and writes everything in its own area with the three ways of writing, lists its inbox and its wakes in order, and writes jobs and cancels into runners' inboxes.
- **Role**: is run as passes, and is told how long a pass may take; starts the process of a step of a machine made while running.
- **Identity**: asks whether a sender is trusted, reads a client's ceiling and a runner's tags, opens messages sealed to it, signs heads, and seals jobs, cancels and receipts; a message it does not accept is deleted without a receipt (R-id-47).
- **Instance**: writes heads, step entries and receipts in the form that area gives, moves a head from running to finished, and removes what is past keeping by that area's rules.
- **Machine**: gives a type's step the definition, the state before and the events, and takes back the new state and the requests; checks a definition with the same code as the client (R-mach-17); starts children, counts depth and jobs, makes tickets and takes their answers, and holds jobs for the ask, as the Machines rules state (R-mach-65 to R-mach-115).
- **Blob**: copies what a child hands up into the parent's record, once and for every type (R-blob-23), and reads the small blobs a step needs.
- **Permission**: puts each layer that applies into a job by name, and reads the permissions a job declares against the lists of those to be asked about (R-perm-42).
- **Secret**: puts into a job the values of the secrets it names and every layer allows, set nearest its run's location (R-sec-14); fails a job that names a secret that is not set (R-sec-20), and denies one that names a secret a layer does not allow; reads a run's values at its first step, and the secrets and values a submit's files use that are not set.

Used by:

- **Client**: leaves submits, starts, cancels, events and answers in the controller's inbox, with a submit's files among its uploads, and reads the receipts, heads and records the controller writes.
- **Runner**: is sent jobs and cancels; returns results and the emits a machine wants.
- **Flow, Statechart, Agent**: each supplies the step the controller runs, and relies on the controller for attempts, children, timers, tickets, limits and approval.
- **Role**: a controller's pass is the unit that area starts and bounds.
