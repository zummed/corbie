# Client

The detailed rules behind [the Client overview](../client.md).

## The package

**R-client-01** The client is a public Go package in the Corbie repository; an application written in Go uses it directly, and the command line, the gateway and the model server do everything through it, so one act gives one answer in one set of words through all of them.
*Accept:* a Go program kept outside the Corbie repository imports the package, starts a run and reads its result, with no gateway and no `corbie` command involved. One finished run is shown through the package, by `corbie show` as JSON, by the gateway and by the model server's command tool; the four values are the same.

**R-client-03** The package starts a run from a definition and the files beside it, and gives back the run's name.
*Accept:* a flow and its script are started against a running controller and runner; the name given back is the name the run then has in a listing, and it ends `done`.

**R-client-06** To start a run the package reads the definition and the files beside it as one set (every script the definition names by path, the files each script lists, every machine kept in its own file, and a file of tools an agent names), fills out the short forms, checks the whole and reports everything wrong at once, and uploads the files before it sends the start.
*Accept:* a flow names a script that lists one file it loads, and a child machine kept in its own file with a script of its own; after the start the run's record holds all five, the controller's first step finds them there, and the run finishes using them. A definition with a mistake of its own, a second in another stage and a third in a child's file is started: one report lists all three, the child's among them.

**R-client-09** When the check finds anything wrong, nothing is uploaded and nothing is sent.
*Accept:* after the report of the three mistakes above, a listing of the store shows no new value.

**R-client-12** A start may carry input, files as input, labels, run secrets, a list that narrows what the run may do, and a timeout.
*Accept:* a flow whose output is its input is started with `{"n": 3}`; it ends `done` with output `{"n": 3}`. A start gives the file `report.pdf` under the input name `report`; the run's script reads `input.report` and gets the file's bytes, and the input shown has a blob there with the file's type and size. A run started with a label shows it in a listing made by a heads-only viewer, and so does a child of that run. A start carries a run secret named `TOKEN` while the controller holds another value under the same name; a job that names `TOKEN` sends the start's value, and the value appears in no head, no receipt and no value shown for the run. A definition allowing `network` and `fs.read:/srv/**` is started with a list allowing `network` and `fs.read:/srv/reports/**`; a job that reads `/srv/other` is denied with a refusal that names the start. A run started with a timeout of five seconds, whose script sleeps for a minute, ends `failed` with the reason `timeout`.

**R-client-18** The package cancels a run its client started, sends it a named event with data, and answers a ticket by naming the instance and the ticket and giving a value; the answer to each is its receipt: that the message was taken, or that it was refused and why, and for an event whether the machine acted on it or had no rule for it, and for an answer whether it was the one taken.
*Accept:* a running run is cancelled; the caller is told the cancel was taken, and the run is then shown as `cancelled`, as is every instance below it. A chart standing in a state with a rule for `go` is sent `go` and the caller is told it was acted on; sent `go` again while the chart is busy, the caller is told there was no rule for it. Two answers are sent for one ticket, the second by a client that did not start the run; the first caller is told its answer was taken and the second that it was not. A client cancels a run another client started; the caller is told the cancel was refused and the reason the receipt gives.

**R-client-23** A caller waiting for a receipt is told the message was not taken up, and stops waiting, once the message is no longer good and no receipt has come.
*Accept:* with no controller running, a cancel sent with a life of ten seconds comes back within about that time saying it was not taken up.

**R-client-25** The package lists the runs it may read, each as its head gives it: name, type of machine, status, step, when it started, its parent, what it is waiting for, and its labels; the listing can be narrowed and widened as Instances says (R-inst-45).
*Accept:* with two runs in progress, one of them waiting on a ticket named `verdict`, the listing has two rows and the second names `verdict` among what it waits for. Narrowed by label, by status and by parent, and taking in finished runs, it gives what R-inst-45 says.

**R-client-30** The package follows a listing as it changes: after the runs that already exist and the mark of the end of the first listing, each run that appears or changes is given as it happens, and a run is given once more when it ends, saying how, also when the follow does not take in finished runs.
*Accept:* a follow is opened with two runs in progress; it gives two rows, then the mark, then a third row when another run is started, then a changed row when one of them moves to its next step, then that run with the status `done` when it finishes. Opened on an empty store, it gives the mark first.

**R-client-33** The package shows one run: for one in progress, where it stands and what it is waiting for; for one that has ended, its status, and its output or the reason it failed.
*Accept:* a chart waiting in its second state on one job and a ticket named `verdict` is shown as `running`, at its current step, waiting for one job and the ticket `verdict`; the ticket's value is not in what is shown, and the client that started the run reads it from the record, where another client cannot. A finished flow is shown as `done` with its output; a flow whose script threw is shown as `failed` with the reason `error` and the script's error.

**R-client-35** A run is shown from its record, so a head that is a moment behind does not show.
*Accept:* a store is arranged so that a run's last step says it ended and its head still says `running` at an earlier step; the run is shown as `done` with its output.

**R-client-36** A run whose start has been sent, which has neither a receipt nor a head, is shown as `waiting` while its start may still be good; once it cannot be, the run is no longer `waiting`, and anything waiting on it ends, saying the start was never taken up.
*Accept:* a start is sent with no controller running and the run is asked for at once; the answer is `waiting`, and not "no such run". A start is sent with no controller running; a caller waiting for the run to end is released with that message once the life a start has by default, and the allowance for clocks that differ, have passed.

**R-client-38** A name for which no start is waiting, and which has neither a receipt nor a head, is reported as no such run.
*Accept:* a run is asked for under a name that was never sent; the caller is told at once there is no such run.

**R-client-39** A start the controller refused is shown as failed with an error, the reason taken from its receipt.
*Accept:* a start is taken up after the time it was good for; the controller's receipt says it was refused as late, no head exists, and the run is shown as `failed` with that reason.

**R-client-40** A run whose receipt says the start was taken, and whose head is not yet there, is shown from its record.
*Accept:* a store is arranged with the receipt and the first step and no head; the run is shown as `running` at its first step, and not as `waiting` or as no such run.

**R-client-42** The package reads a run's record so far and follows it as it grows: each step and each emit is given in order as it is written, an emit while its job is still running, and the records of every instance below are taken in, each entry saying which instance and which job it came from.
*Accept:* a flow with one script stage and one stage that is a child flow is followed from its start; the follow gives the first step, the first job's emits in the order they were emitted and well before the job's result, the step that took its result, and the child's steps and its job's emits beside the parent's, each carrying its instance and the name of its job. Read without following while the flow is in progress, the record gives the entries written so far and the run as `running`, and returns.

**R-client-46** A follow of a run's record ends when the run and everything below it have ended, and says how the run ended.
*Accept:* a followed flow with a child finishes; the follow gives its last entry, after the child's last, then the run as `done`, and closes.

**R-client-48** The package fetches a file from a result by its path in the result, and every path shown for a blob can be asked for and gives that blob, wherever the value was shown.
*Accept:* a run's output has a blob at `report`; asking for the run's file `report` gives the bytes the script wrote, with their type and size. Blobs stand in a run's input, in an emit, in the events of a step, in its definition and in its output; each is shown with a path, and each path fetches the right bytes.

**R-client-50** The package shows the definition a run was started from.
*Accept:* asked for the definition of a run, the package gives the full form that was sent, with each script standing as a blob of its type and size.

**R-client-51** Everything the package reads about a root it reads the same way about an instance below a root its client started.
*Accept:* the child of a flow is shown, followed, asked for its definition and asked for a file from its result by the child's own name; each answers as it would for a root.

**C-client-01** A start's receipt is found from the run's name alone. A client that has restarted knows nothing of a run but the name it noted. *Test:* `client_start_receipt_found_from_run_name`

**C-client-04** Preparing reads only the set of files it was given, and a definition that names a path outside the set is refused. Where the set arrives from another computer, anything else would read the disk of the process doing the preparing. *Test:* `client_definition_path_outside_set_refused`

**C-client-08** A listing reads the running heads before the finished ones. A head is written among the finished before it leaves the running, so read in the other order a run ending in between is in neither. *Test:* `client_listing_never_misses_a_run_that_ends`

**C-client-09** A follow of a record reads the record once more after it sees the run has ended, before it closes. An emit written just before the last step can be listed just after it. *Test:* `client_follow_reads_once_more_after_end`

**C-client-10** A wait that is cut short, by its own limit or by the caller going away, answers with the run as it stands, `waiting` or `running`, through the package and through the gateway alike. Answered as a failure, or as no such run, it makes the caller act on a run that is in fact going well. *Test:* `client_wait_cut_short_answers_as_it_stands`

**C-client-11** A follow given a name that is no run ends at once saying so, and does not wait for the run to appear. A follower of a mistyped name otherwise waits for ever. *Test:* `client_follow_of_unknown_run_ends`

**C-client-12** A receipt is believed only if the controller signed it for the message it is filed under. Whoever keeps the store can move a value, and a receipt for another message put in its place would tell a client its start was refused or its cancel taken. *Test:* `client_receipt_for_another_message_not_believed`

**C-client-13** A step is believed only for the instance and the number the controller signed it for. A step moved in from another record would show a run an ending it never had. *Test:* `client_step_moved_between_records_not_believed`

## One shape for a run

**R-client-57** A run's status is one of five words, `waiting`, `running`, `done`, `failed`, `cancelled`, and a run has the same status word wherever it is shown.
*Accept:* a run asked for before it is taken up, while it works, after it finishes, after its script throws and after it is cancelled gives those five words in turn, and no surface gives any other. A finished run is `done` in a listing, in a follow of the listing, when shown by itself and at the end of a follow of its record.

**R-client-59** A failed run says why with one of three words, `error`, `timeout` or `denied`, and a run that failed as `denied` carries the refusal: what was tried, which layer refused it and what would fix it.
*Accept:* a run whose script throws gives `error`; one that passes its timeout gives `timeout`; one whose script reads a file outside what its definition allows gives `denied`, with the path tried, the definition named as the layer, and the line to add.

**R-client-61** A finished run has its output and a list of the permissions it used; for a machine run those are the permissions of every job in it and below it.
*Accept:* a flow whose output is `{"title": "x"}` is shown with that output after it ends, and a run whose one job read a file and connected to one host is shown with exactly those two permissions; a flow with a child whose job started a program is shown with those two and the program; a failed agent is shown with its error and with the output it still gave.

**R-client-63** A row of a listing is the same shape with only what a head holds, under the same names and words.
*Accept:* every field of a listing's row for a run is found, with the same name and value, in the same run shown by itself.

**C-client-14** The status `waiting` means only that a run has been sent and not yet taken up. A run that has started and waits on a job, a timer or a ticket is `running`, and what it waits for is given beside its status. One word with two meanings makes a filter on it wrong half the time. *Test:* `client_waiting_status_only_before_taken_up`

**C-client-15** A run's name is given under one word, its ID, in every command, in JSON and over HTTP. A second word for the same thing makes callers handle both. *Test:* `client_run_name_is_one_field_everywhere`

**C-client-16** The key is left out where the one shape is made, so a value the package hands to an application has no key in it when the application prints it or turns it into JSON, and no command, gateway answer or stream, error text or result given to a model can hold one. A surface that hides keys for itself is a surface that can forget to. *Test:* `client_blob_in_package_value_has_no_key`

## The command line

**R-client-73** A script file can be submitted as it is: it is wrapped in a flow of one stage, and the permissions it may use and its limits are given on the command line.
*Accept:* `corbie submit fetch.js` with an input ends `done`; the script received the input as its `input`, the run's output is what the script returned, and the definition shown is a flow with one stage. A script that fetches a web page is submitted with no permissions and ends `failed`, `denied`; submitted again with `net` for that host given on the command line, it ends `done`.

**R-client-78** `corbie show <id>` prints one run, where it stands and its result if it has ended; `show --record` prints each entry of the run's record so far, its children's included, and then the run as it stands; `show --watch` prints the same and keeps going, each entry as it is written, until the run ends; `show --wait` stays until the run ends and prints its result.
*Accept:* for a run in progress `show` prints `running`, the step and what the run waits for; for the same run after it ends it prints `done`, the output and the permissions used. For a run waiting on a ticket, `show --record` prints its first step and the run as `running`, and returns; for a run that has ended, `show --record` and `show --watch` print the same. For a flow with a child, `show --watch` prints the parent's and the child's steps and emits as they are written, each marked with where it came from, and returns when the run has ended. `show --wait` given a run in progress prints nothing on standard output until the run ends, then what `show` prints for a finished run; given a run that has already ended it prints that at once.

**R-client-81** `corbie show` can also print one step or the definition, and can save a result's files to a directory.
*Accept:* asked for step 2 of a run, it prints the events that arrived, the state the machine was left in and what the step asked for. Asked for the definition, it prints the full form, readable as a definition, with each script shown as a blob. For a run whose output has blobs at `report` and `chart`, saving to a directory leaves two files with those names and the bytes the run produced.

**R-client-92** Every command can print its result as JSON, in the one shape for a run.
*Accept:* each command in the table, asked for JSON, prints JSON and nothing else on standard output; where the result is a run or a list of runs it is the shape the package gives, and a command that keeps going prints one JSON value on each line, the mark of the end of `ls --watch`'s first listing among them.

**R-client-93** A command ends in one of three ways, each with its own exit status: success with 0, a mistake in how it was written with 2, and failure with 1. Only a command that waits for a run fails on a run that ended badly; a plain `show` of a failed run succeeds.
*Accept:* `corbie ls` ends 0; `corbie show` with no ID, and `corbie submit` with a flag it does not have, end 2 and name the mistake; `corbie show` of an ID that is no run ends 1. `corbie submit --wait`, `corbie show --wait` and `corbie test` each end 1 for a run that ends `failed` or `cancelled`, and 0 for one that ends `done`; `corbie show` of a run that ended `failed` ends 0.

**R-client-95** The config and the store may each be named by an environment variable, and no other flag is read from the environment.
*Accept:* with both variables set and neither flag given, `corbie ls` lists the runs of that config in that store. With a variable set for every other flag of `corbie submit` in the same style, `corbie submit` behaves exactly as with none of them set.

**R-client-97** The commands, their help, the manual's pages about them and the names by which a model is allowed to run them come from one table.
*Accept:* for every command in the table there is help, a manual page and a name to allow it by, each giving the same arguments; there is none of the three for anything not in the table; and every flag a command takes is named in its help.

**R-client-98** `corbie test` runs a definition with everything in one process and nothing set up, and is an ordinary run: the same roles, on a store in memory.
*Accept:* on a computer with no config and no store, `corbie test flow.yaml` runs the flow to its end and prints its result, and leaves no config, no store and no process behind. One definition and input run by `corbie test` and by `corbie submit --wait` against an initialised computer give the same output, the same status and a record with the same entries under the same names.

**R-client-100** Under `corbie test` the ceilings allow everything, the definition's own permissions still apply, and a job is carried out whatever tags it names.
*Accept:* a definition that allows `network` reaches a host under `corbie test` with no ceiling given; a script whose definition allows nothing is `denied` when it tries. A flow whose stage names the tag `gpu` runs to its end under `corbie test` on a computer where nothing was set up.

**R-client-132** `corbie check` starts a small run of its own, which asks for one job and sets one timer, follows it as any client does, and reports each thing that had to happen, with how long it took: the controller took the start, a runner took the job, the controller took the result, the timer fired. A line that never came, or came late, says what leaves it so on a computer and on a serverless host; a run that ended for a reason of its own shows that reason; and the command fails unless every line is `ok`.
*Accept:* on a computer with a controller and a runner running, `corbie check` prints the four lines, each `ok`, and succeeds. With nothing running it prints the first line as `never`, naming the controller's inbox, and fails. With a controller and no runner it prints the first line `ok` and the second `never`, naming the runner the job went to and that runner's inbox, and fails, having waited no longer than it was told to. Sent to a tag no runner has, it prints the first line and the run's own error, and fails.

**C-client-24** `corbie submit` with neither `--wait` nor `--watch` prints the run's ID and nothing else on standard output. The ID is taken by other commands, and anything beside it breaks them. *Test:* `client_submit_prints_only_the_id`

**C-client-25** A command's result goes to standard output and everything said along the way to standard error, the same for every command, and help that was asked for goes to standard output. A result that moves between the two from one command to the next cannot be piped, and help on standard error shows nothing when it is piped to a pager. *Test:* `client_results_on_stdout_progress_on_stderr`

**C-client-28** A flag that has no effect on what was given is refused as a mistake: one that applies only to a script file is refused with a definition. A flag taken and ignored leaves a person believing a limit is in force. *Test:* `client_flag_without_effect_refused`

**C-client-31** A run secret's value is never given on the command line, and a secret named whose variable is not set is a mistake that says which variable is missing and sends nothing. A command line can be read by every user and many jobs on the computer. *Test:* `client_run_secret_only_from_environment`

**C-client-32** Saving a result's files checks every name before writing any file: a name that is `.` or `..`, holds a slash, or is shared by two blobs is refused. Names come from a run's output, and a run must not choose where on the caller's disk a file lands. *Test:* `client_saved_file_names_checked_first`

**C-client-34** A store's address is not repeated in an error. The address of a bucket may hold a password. *Test:* `client_store_address_not_in_errors`

**C-client-35** A store named on the command line is used in place of the one the environment names, and that in place of the one the config recorded. A config made on one computer does not always know where its store is on another. *Test:* `client_store_flag_beats_environment_beats_config`

**C-client-36** The name a command is allowed by and the command that then runs are found by one reading of the command line. Two readings that disagree let a caller run one command under another's name. *Test:* `client_allowed_name_and_command_from_one_reading`

**C-client-37** `corbie test` uses the same code for every role as `corbie run`, and has no way of running a machine of its own. A second way passes tests the real one fails. *Test:* `client_test_and_run_share_roles`

**C-client-50** The check's run is an ordinary run, and the check reports only what it reads of that run in the store: it asks the host nothing. A check that asked a host what was set up would need a way of asking for every host, and would pass a setup that is all there and does not work. *Test:* `client_check_is_an_ordinary_run`

**C-client-51** A line that took more than twenty seconds is `late` and fails the check, the timer excepted. On a serverless host the once-a-minute call does whatever the bucket's own call should have, so a call that is missing shows only as slowness, and a check that passed a slow line would pass it; a timer there waits for that call and is not at fault for it. *Test:* `client_check_slow_leg_is_late`

## The gateway

**R-client-109** The gateway has a route for each thing the package reads: a listing, narrowed and widened as a listing can be; one run; a run's record so far; a run's definition; and a listing and a record are each served with `?watch` as a stream that follows them, a listing's with the end of the first listing marked, a record's with the children's entries included.
*Accept:* each narrowing, asked of the listing route, gives the rows the package gives for it. Asked for a finished run, the route gives the status, the output and the permissions used, in the same JSON `corbie show` prints; asked for its definition, the full form with each script as a blob of its type and size, and no key. A caller opens the listing stream with two runs in progress and receives two runs, then the mark, then a run each time one appears or changes, for as long as it stays connected, and a stream narrowed to one label carries only runs with that label, before the mark and after it. A caller opens the record stream for a flow with a child and receives the parent's and the child's steps and emits, each saying where it came from. Asked for the record of the same flow without following, the route gives those entries and the run as it stands, and returns.

**R-client-115** A stream of a record says when the run has ended and how, so its end is told apart from a connection that was lost.
*Accept:* the stream of a run that finishes carries a last message giving the run as `done` before it closes; a stream cut by a failing network carries no such message.

**R-client-116** Files go in as uploads, whatever their bytes, and come out by their path in a result.
*Accept:* a start is made with a definition, two scripts and an image as input, uploaded with the request; the run's script reads the image's bytes unchanged. A run's output shows a blob with a path; asking the gateway for that path of that run gives the bytes with their type.

**R-client-119** A gateway is one client with one ceiling: every caller's run is started by that client, all callers act as it, and nothing in a request raises the ceiling.
*Accept:* two callers start one run each; both runs carry the gateway's client in their names, each caller sees both in a listing, and either can cancel the other's. With the gateway's client given the ceiling `network`, a run started through the gateway whose definition allows `disk` is `denied` when it writes a file, whatever the request said.

**R-client-121** Callers show the gateway a token it was given when it was started, and a request without the right one is refused and does nothing.
*Accept:* a gateway is started with its tokens; a start sent with no token, and one sent with a wrong token, are refused, and the store holds no upload and no message from either.

**R-client-122** The gateway keeps nothing itself, runs wherever a process can stay up, and is not offered on serverless hosts.
*Accept:* a gateway is stopped and another started on the same config between a start and a show; the show answers as it would have, and after a start's request has ended nothing of its uploads remains on the gateway's computer. A process started on a serverless host and asked to be a gateway refuses to start and says a gateway needs a process that stays up.

**R-client-125** A request holding a field the gateway does not know is refused, and the answer names the field.
*Accept:* a start whose body has a misspelt field beside valid ones is refused naming that field, and no run is started.

**C-client-38** A token is compared in a way that takes the same time whether it is nearly right or wholly wrong. A comparison that stops at the first wrong character lets a caller find the token one character at a time. *Test:* none. How long a comparison takes cannot be tested without a test that fails by chance; both sides are hashed and compared in constant time, and that is checked by reading.

**C-client-39** A definition uploaded to the gateway may name only the files uploaded with it, and an upload holding a file and a directory of the same name is refused. A path that leaves the upload would be read from the gateway's own disk, and only one of the two can be the thing a definition names. *Test:* `client_gateway_definition_cannot_read_gateway_disk`

**C-client-42** An ID that is no run is answered as not found by the answer's status, and at once, on every route that names a run, the streams included; a run started a moment ago is answered as `waiting`, never as not found. A caller made to match the words of an error breaks when the words change, and one that reads not-found as "gone" starts the run again in a loop. *Test:* `client_gateway_unknown_run_is_not_found`

## For a model

**R-client-126** `corbie mcp` serves two tools to a model: the manual, which lists the pages and gives one; and the command line, which runs a command and gives back its result in the one shape and which of the three ways it ended.
*Accept:* a model's host connected to `corbie mcp` is offered exactly those two tools. The manual tool answers as R-man-21 says. A model allowed `show` runs it for a finished run; the result is the JSON `corbie show` prints for that run, with the ending given as success.

**R-client-129** The model may run only the commands the server was started with permission for, with the config the server was started on; only a model is allowed commands by name, and a script has no way to run them.
*Accept:* a server started allowing `ls` and `show` runs both; asked for `submit` it refuses, runs nothing, and names the command that was not allowed. A call that gives a config or a store among the command's arguments is refused; a call that gives neither lists the runs of the server's own config. No plugin or library a script can load runs a command of Corbie's.

**R-client-131** The model server needs nothing running and keeps nothing.
*Accept:* with no controller, runner or gateway running, `corbie mcp` starts, serves a manual page, and runs `corbie test` for a model allowed it. A server is stopped and started again between a model's `submit` and its `show`; the `show` answers as it would have, and the server has written nothing of its own to the config or the store.

**C-client-47** What a model is told about a command that stays until a run ends says that it stays. A model that follows a run waiting on a ticket waits with it, and cannot answer the ticket from inside the same call. *Test:* `client_model_told_which_commands_stay`

**C-client-52** A call that stays keeps no other call from being answered, and ends when the model's host stops it. A model that waits for a run that never ends would otherwise leave the server answering nothing until it is started again. *Test:* `client_model_server_answers_beside_a_call_that_stays`

**C-client-49** A command refused to a model is refused before any part of it runs. A check made after the command has read its files or its store has already done what the model was not allowed. *Test:* `client_model_refused_command_does_nothing`

## Interfaces

Uses:

- **Store.** Reads heads, records and its own receipts; writes messages into the controller's inbox and new blobs into the records of its own runs; is told at once of a write made in the same process, and otherwise looks again at the one interval (R-inst-54).
- **Instances.** The head and the entries of a record are what it reads, by the one way of watching, with its narrowings (R-inst-43 to R-inst-49); a receipt is what it gets back for a message; the three levels of watching decide what it shows, and a head that is not the controller's is not shown (R-inst-01); the run's name is chosen before anything is sent (R-inst-65).
- **Blobs.** Makes a blob for each uploaded file, reads a blob through its reference, copies a blob from one record to another (R-blob-25), and shows a blob as its type, size and path with no key (R-blob-06).
- **Identity.** The client's own identity and its controller's card: it signs and seals each message, opens receipts and records sealed to it, and checks the controller's signature on a head. `corbie init` and the `controller`, `runner` and `client` commands that enrol and trust are Identity's (R-id-12 to R-id-22).
- **Machines, Flow, Statechart, Agent.** The filling-out of short forms and the check of a definition, which the controller runs again with the same code (R-mach-12, R-mach-17, C-mach-11); what a watcher is shown of each type's state.
- **Models.** What an agent's turn needs, the connection and the key, written into the definition as it is sent (R-model-15).
- **Permissions.** The form of a narrowing list and of the permissions given with a script file; the three parts of a refusal.
- **Secrets.** Run secrets, sealed into the start.
- **Controller.** Takes the client's messages and writes their receipts, heads and records. Nothing passes between them except through the store.
- **Roles, Runner.** `corbie run` starts the roles named, and `corbie test` runs them all in one process.
- **Plugins.** The extensions the binary holds, for `corbie version`.
- **Manual.** The pages `corbie manual` and the model server's manual tool give (R-man-18, R-man-21).

Used by:

- **Manual.** Takes the table of commands and makes each command's page from it.
- **Applications.** A Go application uses the package; any other uses the gateway; a model uses `corbie mcp`.
