# Roles

The detailed rules behind the [Roles overview](../role.md).

## Passes

**R-role-01** A Corbie process runs as a controller, as a runner, or as both, and what it does is set by the roles it is started with and by nothing else.
*Accept:* one binary with one config is started as `corbie start controller`, as `corbie start runner` and as `corbie start controller runner`. The first takes steps and runs no job, the second runs jobs and takes no step, the third does both.

**R-role-02** A pass looks at the store, does everything that is owed, and stops: a controller's pass handles every message in its inbox and every wake that has come due, and no wake that has not; a runner's pass takes every job in its inbox that it can finish, and runs it.
*Accept:* messages from two clients and two runners, for five instances, are waiting, with one wake due a minute ago and another due in an hour. After one controller's pass each message has led to its step or been refused with a receipt, none is left in the inbox, the first wake has been delivered and is gone, the second is untouched, and the pass returned of its own accord. Four short jobs wait for a runner set to run two at once: one pass runs all four, two at a time, writes four results and leaves the inbox empty.

**R-role-07** Nothing is carried in memory from one pass to the next: whatever a pass needs, it reads from the store.
*Accept:* run a workload once with every pass in a process of its own and once with all the passes in one process: the names and the opened contents of everything written are the same. A controller process started for the first time, holding only its key and the store's address, carries on an instance that another copy began: who it trusts, its secrets, the instance's state and what it waits for all come from the store.

**C-role-05** However much is owed, a pass has a bounded number of store requests open at once. A process has a limit of open files and connections, and a pass that works on everything side by side reaches it. *Test:* `role_pass_bounded_side_by_side`

## Hosts

**R-role-10** How passes are started depends on the host and on nothing else, and a pass does the same on every host.
*Accept:* the same store contents are given to a pass under the stand-in for a serverless host and to a pass of `corbie start`. The two make the same writes.

**R-role-11** On a serverless host, the host calls for a pass when something is written to the role's inbox, and calls the controller once a minute so that wakes are noticed.
*Accept:* under the stand-in host, write a start into the controller's inbox: a controller's pass is called and takes it, and the job that pass writes into a runner's inbox calls a runner's pass, which runs it. A statechart waiting on a timer with no message arriving is moved by the call that falls in the minute after the timer's time.

**R-role-13** Corbie has no signalling of its own: the calls are services the host and the bucket already offer, and parties write to each other only through the store.
*Accept:* with every network path closed except those to the store, runs go from start to end. No controller and no runner opens a listening port or connects to another party.

**R-role-14** A pass knows how long the host will let it run and finishes before that; whatever it has not reached waits for the next pass.
*Accept:* under a stand-in host that kills a copy at thirty seconds, a controller's pass is given more messages than fit. The pass returns before thirty seconds, the host kills nothing, the messages not reached are in the inbox untouched, the passes that follow take them, and every run ends as it would have with one long pass.

**R-role-17** On a computer `corbie start` runs passes in a loop, sleeping until the next thing is due or for a short interval, whichever is sooner.
*Accept:* with the interval set to two seconds, a message written by another process is taken within about two seconds, and a wake due in a fifth of a second is delivered then and not at two seconds.

**R-role-18** When one role in a process writes something that another role in the same process reads, the second is woken at once; that shortcut changes when a pass happens and never what a pass does, and it is the only thing a computer adds.
*Accept:* `corbie start controller runner` with the interval set to a minute: once the controller has taken a start, the job it sends is run and its result is taken without any wait for the interval. Run one workload with both roles in one process and again with each role in a process of its own: everything written is the same, and only the times differ.

**R-role-20** `corbie test` is the same loop, with every role in one process and the store in memory.
*Accept:* a definition run by `corbie test`, and the same definition submitted with the same input to `corbie start controller runner` on a directory store, go through the same steps to the same result. `corbie test` needs nothing set up beforehand.

**C-role-08** Nothing of a pass goes on after it has answered its host: no delete, no write, no look for the next wake. A host freezes a copy once it has answered, and work in flight is cut off or carried out minutes later when the copy thaws. *Test:* `role_nothing_runs_after_the_pass_answers`

**C-role-10** A copy frozen at any write of its pass and thawed after another copy has done the same work changes nothing: it reads the store afresh, and every write it still makes is answered "already there" or "changed", or lands as something later passes drop. A thawed copy that trusts what it held before the freeze does the work a second time. *Test:* `role_copy_frozen_at_every_write_and_thawed_harms_nothing`

**C-role-12** The controller's call once a minute takes the messages that are waiting as well as the wakes that are due. A bucket's notice of a write can be lost, and one newly set up takes minutes to come into force. *Test:* `role_minute_call_takes_waiting_messages`

**C-role-14** On a serverless host a role refuses a store that is a directory or memory. Copies on such a host share no disk and no memory, so each would see a store of its own. *Test:* `role_serverless_host_refuses_directory_and_memory`

**C-role-15** On a host that unpacks its package read-only and readable by everyone, a role copies its config to a place of its own and makes it its owner's alone before using it. An identity that others can read is not accepted, and the role would otherwise not start. *Test:* `role_host_package_readable_by_all_still_starts`

**C-role-16** In the loop, a pass that could not reach the store is followed by another after the interval, and the process does not stop. A store out of reach for a moment must not need a person to start the role again. *Test:* `role_loop_survives_store_out_of_reach`

**C-role-17** A copy of a newer build works beside a copy of an older one on one store, for as long as both know the layout's version. A host replaces a role's code while calls of the earlier code are still running. *Test:* none; it needs two builds, and is checked from the second release on against a store written by the release before.

## Several copies

**R-role-21** Any number of copies of a role may run against one store, and any copy may be stopped at any instant.
*Accept:* five copies of the controller and three copies of each of two runners carry two hundred runs of flows, statecharts and agents, with copies killed at random instants and new ones started throughout. Every run ends with the result that one copy of each role gives, every job's script ran once for each attempt, and at the end no message and no due wake is left. The only runs that differ from an unbroken run are those whose job was in the hands of a runner's copy when it was killed: each of those sees a timeout at the job's due time.

**R-role-23** Every write a role makes is only-if-new or only-if-unchanged.
*Accept:* on a memory store that notes every write, across that workload, neither a controller nor a runner ever overwrites a value.

**R-role-29** A runner's pass that dies halfway does no harm.
*Accept:* kill a runner's pass at each of these points and then run fresh passes. Before the mark is written: a later pass takes the job and its script runs once. After the mark and before the script starts, while the script runs, or after it returns and before the result is written: no copy runs the script again, the machine receives a timeout at the job's due time, and what the job had emitted stands. After the result is written and before the job is deleted: the result reaches the machine once and the script is not run again. In every case, once the job's due time has passed, the job and its mark are gone from the runner's area.

**R-role-30** A step's outcome depends only on what went into it: the definition, the state before, and the events. Two copies that take the same step write the same thing, down to the names of the jobs it asks for.
*Accept:* give the same definition, state and events to two copies of the controller, on two computers, at two different times, each on its own copy of the store. The step entries they write say the same thing once opened: the same state, the same requests, the same time and random values, and the same names for the jobs, wakes, children, keys and blobs.

## What is not promised

**R-role-34** A job sent to a runner that has gone away waits until its time is up; the next attempt, if the job's definition gives it one, goes to another runner; Corbie does not watch for runners that have stopped.
*Accept:* send a job to a runner none of whose copies is running: nothing sends it anywhere else and nothing fails early, and at the job's due time the machine receives a timeout. Two runners have the tag a job names, and the one its first attempt goes to is stopped: with two attempts, the second goes to the other runner and the job ends well; with one attempt, the job fails with a timeout. Nothing in the store records whether a runner is up, and a first attempt is sent to a stopped runner as readily as to a running one.

**R-role-37** On a serverless host a timer fires within a minute of its time; on a computer within the loop's interval of its time; and never before it.
*Accept:* under a stand-in host that calls the controller once a minute, timers set for times spread across a minute are each delivered no later than a minute after their time, and none before it. Two copies of `corbie start controller` share a store, with the interval set to two seconds: a timer set by a step one copy took is delivered, by whichever copy, no later than about two seconds after its time, and not before it.

## Interfaces

Uses:

- **Store**: a pass lists an inbox and the due wakes and works only through the three ways of writing; a store in the same process wakes a role when another role writes what it reads.
- **Controller**: what a controller's pass does with a message or a wake: the five things done for each instance, in their order, and what a controller stopped at any point leaves (R-ctl-31).
- **Runner**: what a runner's pass does with a job: the mark that makes a job run once (R-run-04), the script's own process, the result, the number of jobs at once, which jobs it leaves for want of time (C-run-06), and how a cancel reaches the copy that runs a job.
- **Identity**: the role's key, and who it trusts, read afresh in every pass (C-id-14).
- **Instances**: the head and the record a killed pass leaves behind (R-inst-12, R-inst-13, C-ctl-16).
- **Plugins**: a serverless host is an extension; it tells a pass how long it may run and calls for passes.

Used by:

- **Client**: `corbie start` starts the loop for the roles named, and `corbie test` starts it with every role in one process on a store in memory.
- **Controller** and **Runner**: each does its work only inside a pass, and relies on the three rules for what happens when a pass is killed or doubled.
- **Machines**: a step is taken once however many copies take it (R-mach-04), and a timer is as punctual as its host allows (R-role-37).
