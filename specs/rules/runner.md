# Runner

The detailed rules behind the [Runner overview](../runner.md).

## Taking a job

**R-run-01** A runner reads its inbox in the store of each controller it trusts.
*Accept:* a runner trusts two controllers with two stores, and each has a job waiting for it: one pass takes both.

**R-run-02** A runner acts on a job only if it is from a controller it trusts, signed by that controller, sealed to this runner and still good.
*Accept:* four jobs are left in a runner's inbox: one signed by a controller it does not trust, one altered after it was signed, one sealed to another runner, and one past the time it is good for. No script starts for any of them, and no mark is written.

**R-run-03** A runner does not start a job whose result is already overdue by the runner's own clock.
*Accept:* a job is still good as a message, and its due time has passed by the runner's clock: no script starts and no result is sent.

**R-run-04** A runner takes a job by writing its mark only-if-new, and only the copy that wrote the mark runs it, so a job is run once; a copy that finds the mark already there leaves the job alone.
*Accept:* ten copies of one runner and thirty jobs, each script adding one line to a file they share: thirty lines, one for each job, and thirty results. Copy B looks at a job that copy A has marked and is running: B starts nothing and writes no result, and the job's message is still in the inbox.

**R-run-06** A runner runs several jobs at once, up to a number its owner sets, and leaves the rest waiting.
*Accept:* with the number set to two and five jobs waiting, never more than two scripts run at once, and all five have run in the end.

**C-run-01** Whether a job is overdue is checked last, at the moment its script would start, against the clock as read then, and with no allowance in the job's favour. A serverless host freezes a copy between calls for any length of time, and a job read before the freeze and started after it runs long after its attempt was given up and another has done the work. *Test:* `run_thawed_copy_does_not_start_overdue_job`

**C-run-02** A mark says which copy wrote it, and a copy whose write of a mark got no answer finds out whether the mark is its own before it leaves the job. A write only-if-new can land while its answer is lost, and a copy that cannot know the mark as its own leaves the job to nobody. *Test:* `run_mark_whose_answer_was_lost_is_known_as_own`

**C-run-04** A copy that is at its number of jobs writes no mark for the next one. A mark written by a copy with no room holds the job away from copies that have room, and the first copy to look takes every job and runs them all itself. *Test:* `run_copy_at_its_limit_leaves_jobs_unmarked`

**C-run-05** A copy that left a job for want of room looks at its inbox again as soon as one of its own jobs ends. Left to its next interval, a waiting job sits out an interval for every place that comes free. *Test:* `run_freed_place_takes_waiting_job_at_once`

**C-run-06** A job is taken only if its whole time limit, and the writing of its result, fit in the time the host has left. A host stops a copy at its limit without warning, and a job cut off there has a mark and no result, so nothing answers for it but its due time. *Test:* `run_job_taken_only_if_limit_fits_time_left`

**C-run-55** A job whose time limit is longer than the host gives any call is failed at once, with an error that names both times. No copy could ever take it, and left in the inbox it waits out its whole time and then fails saying nothing of why. *Test:* `run_job_no_call_could_finish_fails_at_once`

**C-run-56** A copy reads from its inbox only the cancels and the jobs it has a place for and that no copy has marked, and clears old marks without reading them. Reading every waiting job at every look, and every mark at every pass, costs a request for each: ten copies before a thousand jobs spent their time reading, and jobs ran out of time in the queue. *Test:* `run_long_inbox_read_only_where_a_job_can_be_taken`

**C-run-57** A write a job makes for the controller, an emit, a note of one or its result, whose answer was lost is sent again under the same name, a few times. The name is that job's alone, so finding the value there means the first one arrived; left unsent, a job that ran and finished waits out its time and is reported as a timeout. *Test:* `run_write_whose_answer_was_lost_is_sent_again`

**C-run-07** Nothing of a job runs past the end of its copy's pass: a job still running then is stopped, and answered with a result that says why. A copy frozen with a job half run carries on with it whenever the host next thaws it. *Test:* `run_job_running_at_pass_end_is_stopped_and_answered`

## Running it

**R-run-08** Each job runs in a process of its own, so it is stopped at its time limit, held to a memory limit if it has one, and cannot bring the runner down. A script may run for five minutes unless its job says otherwise, and has no memory limit unless one is set.
*Accept:* two jobs running at once on one runner are two processes, and neither is the runner's own. Parity case 12: a job is stopped at its time limit; a job with no time limit written is stopped at five minutes. A script whose job gives a memory limit goes on making values until it passes the limit: the job ends with a result saying it ran out of memory, and the other jobs on the runner are untouched; the same script with no limit set runs on. A job's process is killed from outside, or dies by itself: the job gets a result saying so, and the runner carries on and runs the next job.

**R-run-12** A job's process starts with nothing of the runner's: no files open and no environment.
*Accept:* a runner is started with the variable `RUNNER_ONLY` set; a job permitted to start programs starts one that prints its variables, giving it none, and lists what it has open: `RUNNER_ONLY` is not printed, and nothing of the runner's is open, not its identity file, its config or its connections to the store.

**R-run-15** The runner reads the script, the script's files and the input from the places in the instance's record that the job names.
*Accept:* a job names a script, one file beside it and an input: the script that runs is the one stored in the record, `require` of the file gives the stored file, and `input` is the stored input.

**R-run-16** Every job has a scratch directory of its own, made for it and removed when it ends, however it ends.
*Accept:* two jobs running at once on one runner each find a different directory, and each is empty when its script starts. One job writes a file into its scratch directory and returns, a second throws, a third is stopped at its time limit: all three directories are gone afterwards.

**R-run-18** A path a script writes without a leading `/`, or on Windows without a drive, is in the job's scratch directory. On Windows one with a leading `/` and no drive is refused.
*Accept:* a permitted script writes `out/report.txt`: the file is at that path under the job's scratch directory, and reading `out/report.txt` gives it back. On Windows, a script allowed `all` that reads `/srv/a` is refused, and is told to write the drive.

**R-run-19** Everything a job starts is stopped when the job ends, however it ends.
*Accept:* a script starts a program that starts another, and neither would end for an hour. The script then returns, or throws, or runs out of time, or is cancelled: in each case both programs are gone moments after the job ends.

**R-run-20** When the script returns, the runner writes any blobs in its output into the instance's record and then sends the output to the controller as the job's result, along with the permissions it used.
*Accept:* a script returns `{ a: 1, f: <a file> }` having read one file and connected to one host: when the result is in the controller's inbox, the blob is already complete in the instance's record; the machine receives `done` with `a` as 1 and `f` as a reference; and the result lists that read and that connection and nothing else.

**R-run-23** If the script throws, or is refused something, or runs out of time, the result says so.
*Accept:* `throw new Error("no")` gives the machine `failed` with an error whose text holds `no`. A script allowed nothing reads a file: the result has the status denied and carries the refusal. A script that never returns, in a job with a one-second time limit: the result has the status timeout.

**R-run-26** The runner deletes a job from its inbox only when the job's result is written.
*Accept:* stop the runner after a script has returned and before its result is written: the job is still in the inbox. Let another job finish as usual: its result is in the controller's inbox, and the job is gone from the runner's.

**R-run-27** The runner removes a job's mark only once the job's due time has passed, allowing five minutes for clocks that differ, also when the job finished early and its message is gone.
*Accept:* a job finishes well before its due time: its mark is still there after the job's message has been deleted, and is still there four minutes after the due time. Six minutes after it, and one more pass, the mark is gone.

**C-run-54** A runner that starts removes the scratch directories left by a runner that is gone, and leaves alone those of one still running. A runner that is killed removes nothing, and what its jobs wrote would lie there for the next job permitted to read the directory, until the disk was full. *Test:* `run_scratch_left_by_runner_that_is_gone_removed`

**C-run-13** Everything a job starts is in one group with the job's process, and the group is stopped when the job ends and once more after the job's process has gone. A program started in the job's last moment is missed by the first stop. *Test:* `run_job_end_stops_what_started_at_the_last_moment`

**C-run-14** The runner stops waiting for a job's output shortly after the job's process has ended, whether or not the output has been closed. A program the job started can hold the output open, and the runner would wait on it for ever. *Test:* `run_job_ends_though_started_program_holds_its_output`

**C-run-15** What a job's process prints as errors is read to its end, with over-long lines cut. Left unread it fills, and the job stalls on its next line. *Test:* `run_long_error_output_does_not_stall_job`

**C-run-17** Nothing a script, a plugin or a library prints can reach the channel between the job's process and the runner. A stray line there is read as part of a result. *Test:* `run_stray_print_cannot_corrupt_result`

**C-run-19** A script past its time limit is interrupted inside its process first, and the process is killed from outside if it has not ended shortly after. An interrupt reaches only script code, and a script waiting inside a call or on a started program never sees it. *Test:* `run_time_limit_stops_script_stuck_in_a_call`

**C-run-21** A memory limit is enforced twice: inside the process, by interrupting the script at the limit, and from outside, on Linux, by killing the process when the memory it really holds goes well past it. The script engine counts only its own values, and what a plugin holds is not seen from inside. *Test:* `run_memory_limit_enforced_from_outside`

**C-run-22** A result too large to send is sent as an error that says so and says to put large output in a blob. With no result at all the controller waits out the due time, and the machine learns only of a timeout. *Test:* `run_result_too_large_is_an_error_result`

**C-run-23** A job's result is written only after every emit of that job has been written. A result that overtakes its emits shows the machine a finished job before its values, and the values then arrive for a job nobody is waiting on. *Test:* `run_result_written_after_all_emits`

**C-run-24** A result, and an emit told to the controller, stay good while the instance they are for exists. Finished work has to wait for a controller that is away, and must not lapse into a timeout. *Test:* `run_result_stays_good_while_controller_is_away`

**C-run-28** A job's process ends when its runner goes away for any reason, also when the runner is killed outright. An orphaned job runs on holding the job's secrets, with nothing to stop it at its limit and nothing to answer for it. *Test:* `run_job_process_does_not_outlive_killed_runner`

## Cancelling

**R-run-28** The copy of the runner that is running a job looks for a cancel for it every few seconds; on finding one it stops the job and everything the job started, and sends a result saying the job was cancelled.
*Accept:* a cancel is left for a running job whose script had started a program: within ten seconds the script's process and the program are both gone, and the controller's inbox gains a result for that job with the status cancelled.

**R-run-31** A copy that is not running the job leaves the cancel where it is while the job is still in the inbox, and a cancel that no copy acts on goes when it is no longer good.
*Accept:* of two copies, A runs the job and B reads the inbox first: the cancel is still there after B's pass, and A then stops the job. A cancel for a job that is answered and gone from the inbox is removed by the next pass of any copy.

**C-run-30** A job whose cancel is already waiting is not started: the copy that would take it marks it and answers it as cancelled. Inbox order is the order of names, so a cancel can sort ahead of the job it is for, and a job started and then stopped has already begun what it does to the outside. *Test:* `run_job_with_cancel_waiting_is_not_started`

**C-run-32** A cancel stops a script that is waiting inside a call. Calls wait for their answer, and a cancel that is only noticed between calls waits with them. *Test:* `run_cancel_stops_script_waiting_in_a_call`

## What a script is

**R-run-33** A script is JavaScript and is the body of a function: its `input` is the job's input, and what it returns is the job's output.
*Accept:* the script `return input.a + 1` with the input `{ "a": 1 }` gives the output 2.

**R-run-37** `emit(name, value)` sends a named value up while the script runs; `log(text)` and `progress(done, total)` are emits for a watcher's benefit.
*Accept:* a script emits `a`, logs `half way`, reports `progress(5, 10)` and then waits a minute inside a call: a watcher of the record sees the value, the log line and the note of progress in that order while the job is still running, and no message goes to the controller for the log or the progress.

**R-run-41** `job` says which request this is, and every attempt at one job is told the same request name.
*Accept:* a script returns what `job` says, in a job that takes two attempts: both attempts see the same request name, and another job sees a different one.

**R-run-42** `require` of a bare name gives a plugin or a library that comes with the binary; of a path starting `./`, one of the files the job lists beside the script; and nothing else.
*Accept:* `require("http").get` is the same call as `http.get`, and the bare name of a library that comes with the binary gives that library. A job lists `./lib/util.js`: `require("./lib/util.js")` gives what that file offers. A `./` path the job does not list, a path that starts with `/`, a path that starts with `../` and a bare name that is neither a plugin nor a bundled library each fail the call with an error that says what was asked for, and none reads the runner's disk.

**R-run-46** A script cannot start jobs, whether scripts or machines.
*Accept:* a script uses `machines` to define a machine and returns: no job and no instance has been made by it, and the definition runs only when a machine asks for it as a job.

**C-run-33** A batch of emits is written when it is full or a short time after its first emit, whichever comes first. Written one at a time, a model's reply is hundreds of writes; held until full, a single line of progress is not seen until the job ends. *Test:* `run_emits_batched_by_count_or_short_time`

**C-run-34** A job's emit entries are numbered from one with none left out, an entry may carry several emits in the order they were made, and each attempt, being sent to a runner separately, begins again at one. A watcher and the controller tell what they have seen, and what they have missed, by the number alone. *Test:* `run_emit_entries_numbered_from_one_without_gaps`

**C-run-35** `require` looks a path up only among the files the job lists, and never on the runner's disk. A path that reaches the disk is a file read outside every permission check. *Test:* `run_require_reads_only_listed_files`

**C-run-36** No plugin or library can take the name of one of a script's own things: `input`, `job`, `emit`, `log`, `progress`, `require`. An extension added later would silently stand in for one of them in every script. *Test:* `run_plugin_cannot_take_a_scripts_own_name`

**C-run-38** A script's error gives the line and column in the script as it was written. The script is run inside a function Corbie adds, and positions that count the added part point at the wrong line. *Test:* `run_error_position_matches_script_as_written`

**C-run-39** A source map named in a script is never read. Reading one opens whatever file the script names, outside every permission check. *Test:* `run_source_map_named_in_script_is_never_read`

**C-run-40** What Corbie itself uses of the script's language to pass values in and out is taken before the script runs. A script can replace those things, and with them change what Corbie reads as its output. *Test:* `run_script_cannot_swap_what_corbie_uses_of_the_language`

**C-run-41** A value a plugin hands a script shows only the calls the plugin's page lists; nothing else of the object behind it can be reached. Whatever else shows is a call nobody checked. *Test:* `run_plugin_values_show_only_listed_calls`

**C-run-42** To learn what kind of bytes a script's value is, the value is asked its type; it is never first converted out of the script engine to find out. For an empty view at the very end of its buffer the conversion makes a pointer one past the buffer, and the process is stopped. *Test:* `run_empty_bytes_view_at_end_of_buffer_is_safe`, run with pointer checks on, which find it every time and not one run in several.

**C-run-43** Turning a script's value into data checks the job's time limit as it goes. A value of enormous length costs a script nothing to make, and the conversion happens where an interrupt of the script does not reach. *Test:* `run_enormous_sparse_value_stopped_at_time_limit`

**C-run-46** Such a file is read to the size it had when it was opened; if it has been cut short the read fails, and if it has gone, or another file or a link is in its place, what is read is still the file that was opened. The file that is read must be the file that was checked. *Test:* `run_file_changed_after_opening_fails_the_read`

**C-run-47** A file opened this way is held open until the job ends. A system gives a deleted file's identity to the next file made, so a file known only by its identity can be another file passing for the one that was checked. *Test:* `run_opened_file_held_until_job_ends`

## Programs a job starts

**R-run-48** Corbie checks which program is started and which secrets go to it; arguments and variables are matched against nothing.
*Accept:* a secret bound to `exec:deploy` is given as an argument to `deploy`: the program gets the value. Given to another program the job may start: refused, and nothing is started. `deploy` started with any arguments and any variables the script chooses, secrets apart, is allowed by `exec:deploy` alone.

**R-run-49** What a started program then does is its own business: it runs as the runner's user and reads, writes and connects as that user can, and Corbie claims nothing about what it can reach.
*Accept:* a job allowed `exec:cat` and no `fs.read` starts `cat` on a file the runner's user can read: the program prints the file. No refusal, result or page of the manual says a started program is confined.

**R-run-50** A runner's owner can set an exec wrapper, a command put in front of every program a job starts, when the runner is started, and Corbie passes the wrapper the job's scratch directory and the job's permissions.
*Accept:* the setting names a wrapper that notes its arguments and then runs them; a job starts `git status`: the wrapper ran, with `git status` at the end of its arguments and the path of the job's scratch directory and the job's permissions in what it was started with. With the setting unset, `git` runs directly.

**C-run-49** The exec wrapper goes in front of every program started for a job, whichever plugin starts it, and one named with no path is found where the runner looks for programs. Programs are started in one place, and a box with one way round it is no box. *Test:* `run_exec_wrapper_fronts_programs_a_plugin_starts`

**C-run-50** Nothing in a job, a definition or a call can change the exec wrapper or leave it out: it is started with the scratch directory, the permissions and the runner's search path and no other variable, and the variables a call gives go to the program inside. The wrapper is the owner's, for the owner's computer, and a variable can make any program load code of the script's choosing. *Test:* `run_job_cannot_change_or_skip_exec_wrapper`

**C-run-51** The permission check is made on the program the script named, before the exec wrapper is added, and that program is what the result lists as used. Checked after, every job needs leave to start the wrapper and none needs leave for what it really runs. *Test:* `run_exec_checked_against_program_the_script_named`

**C-run-52** Stopping one program a job started stops that program only; the job and its other programs go on. The job's programs share one group, and a stop aimed at the group ends the job. *Test:* `run_stopping_one_program_leaves_the_others`

**C-run-53** A program that cannot be started is an error like any other (not found); one that ran and ended badly is an answer, with its exit status; one that runs past the time its call gave it is a timeout error. A script that starts programs has to be able to tell the three apart. *Test:* `run_program_that_cannot_start_is_in_the_calls_answer`

## One runner is one box

**R-run-52** Everything on a runner shares the runner's user, key and computer: a job that is permitted to read the runner's files can read its key, jobs on one runner are not walled off from each other, and on a serverless host the jobs that land on one warm copy share it in the same way.
*Accept:* a job whose every layer allows reading the runner's identity file reads it. A job permitted to write `/tmp/shared` writes it; a later job on the same runner, permitted to read it, reads what the first wrote, and only the first job's scratch directory was removed. Two calls of one function land on the same warm copy: the second job, permitted to, reads a file the first left outside its scratch directory.

## Interfaces

Uses:

- **Store**: lists and reads its own inbox, writes and removes its marks, deletes jobs and cancels it has dealt with, adds emits and blobs to records only-if-new, reads blobs in records, and writes results into the controller's inbox.
- **Role**: is run as passes, and is told how long its host will let a pass run.
- **Identity**: opens jobs and cancels from controllers it trusts, signs what it adds to a record, seals results, and takes the ceiling it gave the controller that signed a job.
- **Permission**: checks every call that reaches outside against each layer in the job and the runner's own ceiling, and notes what was used for the result (R-perm-50); the scratch directory needs an entry like any path (R-perm-12).
- **Secret**: gets the values a job carries, hands scripts handles to them, and has what comes back to a script, and what a job's process prints, searched for them (C-sec-25).
- **Plugin**: supplies the calls a script makes and the libraries a bare name loads; a program is started through the one way out that the exec wrapper is put in front of; what a script leaves open is closed by its plugin when the job ends (C-plug-07).
- **Blob**: reads the script, its files and the input, and writes blobs that leave a job into the instance's record; a blob stays on the runner until its reference leaves the job (C-blob-06).
- **Instance**: an emit entry is signed by the runner, tied to its job and sealed to the controller and the root's client (R-store-44, R-inst-32); an emit the machine wants is also told to the controller (R-inst-29).

Used by:

- **Controller**: sends jobs and cancels; receives results and the emits a machine wants.
- **Instance**: emit entries and blobs in a record are written by the runner doing the job.
- **Flow, Statechart, Agent**: every script job a machine asks for is carried out here.
- **Model**: a turn's call to a model is made by a job, and its pieces are emits.
- **Client**: a watcher reads the emits a job writes as they appear.
