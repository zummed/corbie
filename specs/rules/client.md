# Client

The detailed rules behind [the Client overview](../client.md).

## The package

**R-client-01** The client is a public Go package in the Corbie repository; an application written in Go uses it directly, and the command line, the gateway and the model server do everything through it, so one act gives one answer in one set of words through all of them.
*Accept:* a Go program kept outside the Corbie repository imports the package, starts a run and reads its result, with no gateway and no `corbie` command involved. One finished run is shown through the package, by `corbie run show` as JSON, by the gateway and by the model server's command tool; the four values are the same.

**R-client-03** The package records a tree and starts a run at one of its locations, and gives back the run's name.
*Accept:* a flow and its script are submitted as a tree of one file against a running controller and runner, and a run is started at its location; the name given back is the name the run then has in a listing, and it ends `done`.

**R-client-06** To submit a tree the package reads each machine and the files it names as one set (every script the definition names by path, the files each job lists, every machine kept in its own file, and a file of tools an agent names), fills out the short forms, checks the whole and reports everything wrong at once, and uploads the files before it sends the submit.
*Accept:* a flow names a script and lists one file it loads, and a child machine kept in its own file with a script of its own; after a start at its location the run's record holds all five, the controller's first step finds them there, and the run finishes using them. A definition with a mistake of its own, a second in another stage and a third in a child's file is submitted: one report lists all three, the child's among them.

**C-client-54** A submit's uploads, and a start's, are made side by side, a bounded number at a time. One after another, each upload waits a round trip to a bucket; all at once, a bucket refuses the burst. *Test:* `client_start_uploads_side_by_side`

**R-client-09** When the check finds anything wrong, nothing is uploaded and nothing is sent.
*Accept:* after the report of the three mistakes above, a listing of the store shows no new value.

**R-client-12** A start of a location may carry input, labels, a list that narrows what the run may do, and a timeout. Any value in the input may be a file.
*Accept:* a flow whose output is its input is started with `{"n": 3}`; it ends `done` with output `{"n": 3}`. A start gives the file `report.pdf` under the input name `report`; the run's script reads `input.report` and gets the file's bytes, and the input shown has a blob there with the file's type and size. A file given at `input.attachments[0].data` is read there by the script as a blob of that file's type and size. A run started with a label shows it in a listing made by a heads-only viewer, and so does a child of that run. A definition allowing `network` and `fs.read:/srv/**` is started with a list allowing `network` and `fs.read:/srv/reports/**`; a job that reads `/srv/other` is denied with a refusal that names the start. A run started with a timeout of five seconds, whose script sleeps for a minute, ends `failed` with the reason `timeout`.

**R-client-18** The package cancels a run in one of its client's trees, sends a named event with data to a run by its address, and answers a ticket by naming the instance and the ticket and giving a value; the answer to each is its receipt: that the message was taken, or that it was refused and why, and for an event the run that took it and whether the machine acted on it or had no rule for it, and for an answer whether it was the one taken. An address is a run's ID, which is at the location its head names, a location, or a location and a key.
*Accept:* a running run is cancelled; the caller is told the cancel was taken, and the run is then shown as `cancelled`, as is every instance below it. A chart standing in a state with a rule for `go` is sent `go` by its ID and the caller is told the run took it and acted on it; sent `go` again while the chart is busy, the caller is told there was no rule for it. Two answers are sent for one ticket, the second by a client that did not start the run; the first caller is told its answer was taken and the second that it was not. A client cancels a run another client started; the caller is told the cancel was refused and the reason the receipt gives.

**R-client-23** A caller waiting for a receipt is told the message was not taken up, and stops waiting, once no controller may take the message, past its time and twice the clock allowance, and no receipt has come.
*Accept:* with no controller running, a cancel sent with a life of ten seconds is waited for until ten seconds and twice the clock allowance have passed, and then comes back saying it was not taken up.

**R-client-25** The package lists the runs it may read, those of its trees and those whose head lists it, each as its head gives it: name, type of machine, status, step, when it started, its parent, its location and key, what it is waiting for, and its labels; the listing can be narrowed and widened as Instances says (R-inst-45), and narrowed to the runs at a location.
*Accept:* with two runs in progress, one of them waiting on a ticket named `verdict`, the listing has two rows and the second names `verdict` among what it waits for. Narrowed by label, by status and by parent, and taking in finished runs, it gives what R-inst-45 says.

**R-client-30** The package follows a listing as it changes: after the runs that already exist and the mark of the end of the first listing, each run that appears or changes is given as it happens, and a run is given once more when it ends, saying how, also when the follow does not take in finished runs.
*Accept:* a follow is opened with two runs in progress; it gives two rows, then the mark, then a third row when another run is started, then a changed row when one of them moves to its next step, then that run with the status `done` when it finishes. Opened on an empty store, it gives the mark first.

**R-client-33** The package shows one run, named by its ID, as the one run of its client running at a location, or as the running run of a key there: its location and key; for one in progress, where it stands and what it is waiting for; for one that has ended, its status, and its output or the reason it failed. The input, the output, the failure's text and the permissions used are shown only to the client that owns the run's tree and the clients its file's `read:` lists.
*Accept:* a chart waiting in its second state on one job and a ticket named `verdict` is shown as `running`, at its current step, waiting for one job and the ticket `verdict`; the ticket's value is not in what is shown, and the client that owns the run's tree reads it from the record, where another client cannot. A finished flow is shown as `done` with its output; a flow whose script threw is shown as `failed` with the reason `error` and the script's error. A flow started with `{"n": 3}` and a file under `report` is shown to its client with that input, the file as a blob at the path `input/report`, which fetches it; another client is shown the run without its input or its output, or, before the run has a head, told there is no such run of its own and whose tree the run is in.

**R-client-35** A run is shown from its record, so a head that is a moment behind does not show.
*Accept:* a store is arranged so that a run's last step says it ended and its head still says `running` at an earlier step; the run is shown as `done` with its output.

**R-client-36** A run whose start has been sent, which has neither a receipt nor a head, is shown as `waiting` while its start may still be good; once it cannot be, the run is no longer `waiting`, and anything waiting on it ends, saying the start was never taken up.
*Accept:* a start is sent with no controller running and the run is asked for at once; the answer is `waiting`, and not "no such run". A start is sent with no controller running; a caller waiting for the run to end is released with that message once the life a start has by default and twice the allowance for clocks that differ have passed.

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

**R-client-51** Everything the package reads about a root it reads the same way about an instance below a root in its client's trees, and a send to it by its ID reaches it at its parent's location.
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

**R-client-151** A run whose definition gives `title:` is shown with its title, and only to the client that owns the run's tree. `corbie run show` prints it as `title:`, and in JSON it is `title`. A row of a listing has none.
*Accept:* a run started with a title is shown by its client with the title, and by a viewer without it. `corbie run show` prints the line `title: #testing: give me a release card`, `run show --json` gives `"title"`, and `run ls --json` gives no title. *Test:* `Test_client_run_show_prints_a_title`, `TestTitleIsFilledInWhenTheRunStarts`

**C-client-14** The status `waiting` means only that a run has been sent and not yet taken up. A run that has started and waits on a job, a timer or a ticket is `running`, and what it waits for is given beside its status. One word with two meanings makes a filter on it wrong half the time. *Test:* `client_waiting_status_only_before_taken_up`

**C-client-15** A run's name is given under one word, its ID, in every command, in JSON and over HTTP. A second word for the same thing makes callers handle both. *Test:* `client_run_name_is_one_field_everywhere`

**C-client-16** The key is left out where the one shape is made, so a value the package hands to an application has no key in it when the application prints it or turns it into JSON, and no command, gateway answer or stream, error text or result given to a model can hold one. A surface that hides keys for itself is a surface that can forget to. *Test:* `client_blob_in_package_value_has_no_key`

## The command line

**R-client-73** A script file can be submitted as it is: the package wraps it in a flow of one stage, with the permissions it may use and its limits, which the command line takes as flags.
*Accept:* after `corbie run submit fetch.js`, `corbie run start /fetch` with an input ends `done`; the script received the input as its `input`, the run's output is what the script returned, and the definition shown is a flow with one stage. A script that fetches a web page is submitted with no permissions, and a run there ends `failed`, `denied`; submitted again with `net` for that host given on the command line, a run there ends `done`. A Go program gives the package a script with an allow list, a deny list, a timeout, a memory limit and a tag, and gets back a flow with one stage whose job names the script and carries all five; given again with no tag and started with an input, it ends `done` with what the script returned.

**R-client-78** `corbie run show <id>` prints one run, where it stands and its result if it has ended; `run show --record` prints each entry of the run's record so far, its children's included, and then the run as it stands; `run show --watch` prints the same and keeps going, each entry as it is written, until the run ends; `run show --wait` stays until the run ends and prints its result.
*Accept:* for a run in progress `run show` prints `running`, the step and what the run waits for; for the same run after it ends it prints `done`, the output and the permissions used. For a run waiting on a ticket, `run show --record` prints its first step and the run as `running`, and returns; for a run that has ended, `run show --record` and `run show --watch` print the same. For a flow with a child, `run show --watch` prints the parent's and the child's steps and emits as they are written, each marked with where it came from, and returns when the run has ended. `run show --wait` given a run in progress prints nothing on standard output until the run ends, then what `run show` prints for a finished run; given a run that has already ended it prints that at once.

**R-client-81** `corbie run show` can also print one step or the definition, and can save a result's files to a directory.
*Accept:* asked for step 2 of a run, it prints the events that arrived, the state the machine was left in and what the step asked for. Asked for the definition, it prints the full form, readable as a definition, with each script shown as a blob. For a run whose output has blobs at `report` and `chart`, saving to a directory leaves two files with those names and the bytes the run produced.

**R-client-92** Every command can print its result as JSON, in the one shape for a run.
*Accept:* each command in the table, asked for JSON, prints JSON and nothing else on standard output; where the result is a run or a list of runs it is the shape the package gives, and a command that keeps going prints one JSON value on each line, the mark of the end of `run ls --watch`'s first listing among them.

**R-client-93** A command ends in one of three ways, each with its own exit status, whether it prints text or JSON: success with 0, a mistake in how it was written with 2, and failure with 1. A run named by its location that is not running there is a failure, not a mistake. Only a command that waits for a run fails on a run that ended badly; a plain `run show` of a failed run succeeds.
*Accept:* `corbie run ls` ends 0; `corbie run show` with no ID, and `corbie run submit` with a flag it does not have, end 2 and name the mistake; `corbie run show` of an ID that is no run ends 1. `corbie run start --wait`, `corbie run show --wait` and `corbie test` each end 1 for a run that ends `failed` or `cancelled`, and 0 for one that ends `done`; `corbie run show` of a run that ended `failed` ends 0.

**R-client-95** The config and the store may each be named by an environment variable, and no other flag is read from the environment.
*Accept:* with both variables set and neither flag given, `corbie run ls` lists the runs of that config in that store. With a variable set for every other flag of `corbie run submit` in the same style, `corbie run submit` behaves exactly as with none of them set.

**R-client-97** The commands, their help, the manual's pages about them and the names by which a model is allowed to run them come from one table.
*Accept:* for every command in the table there is help, a manual page and a name to allow it by, each giving the same arguments; there is none of the three for anything not in the table; and every flag a command takes is named in its help.

**R-client-98** `corbie test` runs a definition with everything in one process and nothing set up, and is an ordinary run: the same roles, on a store in memory. It records the file as a tree of one file and starts it: a transient file with the start's flags, and a singleton by the submit. A keyed file is refused, since its runs start at a send. `--value NAME=VALUE`, `--secret NAME` and `--secret-env NAME` set each name at `/` of its controller.
*Accept:* on a computer with no config and no store, `corbie test flow.yaml` runs the flow to its end and prints its result, and leaves no config, no store and no process behind. One definition and input run by `corbie test`, and by `corbie run submit flow.yaml` then `corbie run start /flow --wait` against an initialised computer, give the same output, the same status and a record with the same entries under the same names. `corbie test uses.yaml --value HOST=db.lan --secret-env TOKEN` runs a job that reads `${values.HOST}` and the secret `TOKEN`; without `--value` the run fails naming `HOST` and `/uses`. `TestTestRunsEachLifetime`: a transient file runs with its input, a singleton runs from its submit, and a keyed file is refused naming why.

**R-client-100** Under `corbie test` the ceilings allow everything, the definition's own permissions still apply, and a job is carried out whatever tags it names.
*Accept:* a definition that allows `network` reaches a host under `corbie test` with no ceiling given; a script whose definition allows nothing is `denied` when it tries. A flow whose stage names the tag `gpu` runs to its end under `corbie test` on a computer where nothing was set up.

**R-client-132** `corbie check` submits a small machine of its own as a tree at `/corbie-check/<client>`, so several clients can check one controller, and starts a run there, which asks for one job and sets one timer, follows it as any client does, and reports each thing that had to happen, with how long it took: the controller took the start, a runner took the job, the controller took the result, the timer fired. A line that never came, or came late, says what leaves it so on a computer and on a serverless host; a run that ended for a reason of its own shows that reason; and the command fails unless every line is `ok`. A submit the controller refuses is printed with its reason, and the check fails.
*Accept:* on a computer with a controller and a runner running, `corbie check` prints the four lines, each `ok`, and succeeds. With nothing running it prints the first line as `never`, naming the controller's inbox, and fails. With a controller and no runner it prints the first line `ok` and the second `never`, naming the runner the job went to and that runner's inbox, and fails, having waited no longer than it was told to. Sent to a tag no runner has, it prints the first line and the run's own error, and fails. For a client whose ceiling has no `at:` entry for `/corbie-check/<client>`, it prints `refused:` and the reason, with the line that allows it, and fails.

**R-client-133** `corbie <word>`, where the word begins no command, runs the program `corbie-<word>` found on PATH with the rest of the line, this process's standard input, output, error and environment, and ends with the program's exit status. A command in the table is found first, so a program under a command's name, or under a word that begins commands, is never run. `corbie help` lists, after the commands, the names of the programs on PATH that `corbie <word>` runs, and starts none of them.
*Accept:* with a program `corbie-ui` on PATH, `corbie ui --help` starts it with `--help`; what it reads is `corbie`'s standard input, what it prints is `corbie`'s standard output, and `corbie` ends with the status it ended with. With programs `corbie-start` and `corbie-controller` on PATH, `corbie start` is the command, `corbie controller frob` is a mistake, and neither program is run. `corbie help` lists `ui` and starts no program; it lists neither `start` nor `controller`, and no programs when none is on PATH. `corbie frob`, with no program `corbie-frob` on PATH, is a mistake that names the program it looked for and ends with `corbie --help lists the commands.`

**R-client-134** The words after each name on `corbie start`'s line, up to the next name, are that name's: a role's flags or a program's arguments; `--config`, `--store`, `--json` and `--help` are `corbie start`'s wherever they are written. `corbie start` takes, beside the roles, the name of a program `corbie-<name>` on PATH, and starts it once every role named is set up: with the words written after its name, standard input empty, its output and errors on `corbie start`'s own with no prefix, and this process's environment with `CORBIE_CONFIG` and `CORBIE_STORE` set as `--config` and `--store` were given. When a program stops, `corbie start` stops the roles and the other programs, waits for them, and fails naming the program; when `corbie start` is stopped, each program is interrupted and waited for, for up to ten seconds before it is killed, before `corbie start` returns. On a serverless host a program is refused before anything starts.
*Accept:* with a program `corbie-ui` on PATH, `corbie start controller runner ui --config /tmp/c` sets the controller and the runner up, then starts `corbie-ui` with `CORBIE_CONFIG=/tmp/c` in its environment and `CORBIE_STORE` as the environment had it, and what it prints is `corbie start`'s output, line for line. When `corbie-ui` exits 1, `corbie start` ends with status 1 and `the program ui stopped (exit status 1)`, and nothing of it is left running. `corbie start controller runner ui --listen 127.0.0.1:8383` starts `corbie-ui` with `--listen 127.0.0.1:8383`, and `corbie start ui --listen 127.0.0.1:8383 runner --jobs 2` starts it with the same and runs the runner with two jobs at once. `corbie start controller --jobs 2` and `corbie start --interval 1s controller` are mistakes that say where the flag is written. `corbie start controller runner ui`, stopped, returns after `corbie-ui` has ended. `corbie start controller ui ui`, and `corbie start controller frob` with no program `corbie-frob` on PATH, are mistakes.

**R-client-136** A program written in Go reads its command line with `cli.Program`, from its own table of commands, and is given the same help, flags, mistakes, failures and exit statuses as Corbie's own commands, and the same client and store.
*Accept:* a program with the commands `""` and `check` is run through `cli.Program`. `--help` prints the usage line of `""`, its summary and the row of `check`; `check --help` prints the help of `check`. `check` with a flag it does not have ends 2 with `corbie <name> check: `, what is wrong and its usage line; a command that fails ends 1 with `corbie <name>: ` and the error. Its flags may be written anywhere among its arguments. With `--store`, with `CORBIE_STORE` and with neither, a command that lists runs through its client ends as `corbie run ls` ends, and prints and says the same.

**R-client-138** A program read with `cli.Program` is given `manual` and `version` beside its own commands, and no `mcp`: a model runs its commands through `corbie mcp`. `version` prints the program's version and the version of the Corbie it is built with. Its manual is Corbie's with its commands' pages and the pages it adds.
*Accept:* a program `frob` with the commands `""`, `check` and `ls` adds a page `frob` to the manual. `corbie frob manual` lists `frob` first, Corbie's pages and `corbie frob check`; `corbie frob manual frob check` prints the help of `check`. `corbie frob --help` lists `manual` and no `mcp`. With the version `1.2.3` given, `corbie frob version` prints `corbie-frob 1.2.3` and `corbie 1.2.3`.

**R-client-137** `corbie start` with nothing named runs the roles the config holds: its controller, its runner, and the store's server when the config records where its directory is served. Anything named means exactly what is named.
*Accept:* after `corbie init`, `corbie start` runs the controller and the runner, and a run submitted to it ends `done`; after `corbie init store --listen 127.0.0.1:<port>`, it serves the directory at that port as well. On a config that holds only a runner it runs the runner alone, and on a config that holds neither a controller nor a runner it fails, naming `corbie init`. After `corbie init`, `corbie start runner` runs no controller.

**C-client-24** `corbie run start <location>`, with neither `--wait` nor `--watch`, prints the run's ID and nothing else on standard output. A submit that records a tree prints its rows instead (R-client-147). The ID is taken by other commands, and anything beside it breaks them. *Test:* `client_submit_prints_only_the_id` and `TestSubmitRecordsAFileAndStartsALocation`

**C-client-25** A command's result goes to standard output and everything said along the way to standard error, the same for every command, and help that was asked for goes to standard output. A result that moves between the two from one command to the next cannot be piped, and help on standard error shows nothing when it is piped to a pager. *Test:* `client_results_on_stdout_progress_on_stderr`

**C-client-28** A flag that has no effect on what was given is refused as a mistake: one that applies only to a script file is refused with a definition, a script file's flag with a directory, and a flag of a start with a singleton under `corbie test`, whose submit starts it, and a flag of a start beside `--definition`, which runs nothing. A flag taken and ignored leaves a person believing a limit is in force. *Test:* `client_flag_without_effect_refused`

**C-client-31** A secret's value, for `corbie test` or for `controller secret set`, is never given on the command line, and is read only from the source the command names: the terminal, the environment variable of its name, or standard input. A value not found there is a mistake that sends nothing. A command line can be read by every user and many jobs on the computer, and a value picked up from an unnamed source is not the one meant. *Test:* `client_secret_value_never_on_command_line`

**C-client-32** Saving a result's files checks every name before writing any file: a name that is `.` or `..`, holds a slash, or is shared by two blobs is refused. Names come from a run's output, and a run must not choose where on the caller's disk a file lands. *Test:* `client_saved_file_names_checked_first`

**C-client-34** A store's address is not repeated in an error. The address of a bucket may hold a password. *Test:* `client_store_address_not_in_errors`

**C-client-35** A store named on the command line is used in place of the one the environment names, and that in place of the one the config recorded. A config made on one computer does not always know where its store is on another. *Test:* `client_store_flag_beats_environment_beats_config`

**C-client-36** The name a command is allowed by and the command that then runs are found by one reading of the command line. Two readings that disagree let a caller run one command under another's name. *Test:* `client_allowed_name_and_command_from_one_reading`

**C-client-37** `corbie test` uses the same code for every role as `corbie start`, and has no way of running a machine of its own. A second way passes tests the real one fails. *Test:* `client_test_and_start_share_roles`

**C-client-50** The check's run is an ordinary run, and the check reports only what it reads of that run in the store: it asks the host nothing. A check that asked a host what was set up would need a way of asking for every host, and would pass a setup that is all there and does not work. *Test:* `client_check_is_an_ordinary_run`

**C-client-51** A line that took more than twenty seconds is `late` and fails the check, the timer excepted. On a serverless host the once-a-minute call does whatever the bucket's own call should have, so a call that is missing shows only as slowness, and a check that passed a slow line would pass it; a timer there waits for that call and is not at fault for it. *Test:* `client_check_slow_leg_is_late`

**C-client-53** A model runs a program on PATH only for a command the server's `--allow` names by its whole name, and `corbie start` never starts a program for a model. Anything on the owner's PATH would otherwise be a model's to run. *Test:* `client_model_runs_a_program_only_as_allowed`

## A tree

**R-client-146** `corbie run submit <dir> --at <prefix>` records one directory as a tree: it reads every `*.yaml` in it and below it as a machine, but for `.git` and what the `.corbieignore` at its top lists, and each has a location, its path under the prefix without `.yaml`. `--at` is `/` and the directory's real name by default. A file that is no machine, a file beside a directory of the same name, the files below it, and a file whose location has a part a location cannot have are reported among the files with problems, and nothing of them is recorded. *Test:* `TestTreeStartsItsSingletons`, `TestTreeIsAtItsNameByDefault`, `TestTreeLocationsAreLeaves` and `TestEveryYAMLIsAMachine`

**R-client-165** Each line of the `.corbieignore` at the top of a tree's directory lists a path from the directory: a file, or a directory and everything below it. A listed file has no location, so nothing runs there and a send or a call to it is to no location, and a machine still includes it, runs it with `machine:` and reads it with `files:`. A line that names the directory itself, such as `.`, or has a `..` segment, a `/` or `!` at its start, a `\`, `*`, `?` or `[`, or a control character fails the read, naming the line.
*Accept:* `.corbieignore` lists `shared/`, `child.yaml` and `rules.yaml`. `top.yaml` includes `shared/base.yaml`, whose script reads `rules.yaml` through `files:`, and runs `child.yaml` with `machine:`; it ends `done` with what both gave. `shared/broken.yaml`, which is not YAML, is no problem. A start at `child`'s path fails, saying nothing is recorded there, and `corbie test` prints a send to it as to a location with no file. `corbie test` prints each listed path as `listed`. A line `../up` fails the read, naming line 2, and so do `*.yaml`, `a?.yaml`, `[ab].yaml` and `!keep`. A singleton's file listed between two submits: the second submit removes its location and cancels its run. *Test:* `TestListedFileHasNoLocation`, `TestBadIgnoreLineFailsTheRead`, `TestTestReadsADirectory`, `TestRemovedFileCancelsItsRuns`

**R-client-147** The submit prints, for each location, its lifetime and what was done there, `started <id>`, `restarted <id>`, `recorded`, `removed` or `problem`, then the directory last submitted there when it was another, the secrets and values the files use that are not set at their locations or above, and the clients the files' `read:` lists that the controller does not trust. It exits when each run it cancelled has ended and each run it started has been taken up, waiting for the second with no limit. A refused submit prints the refusal and fails. *Test:* `TestTreeStartsItsSingletons`, `TestSecondSubmitStartsNothing`, `TestChangedSingletonStartedAgain` and `TestRemovedFileCancelsItsRuns`

**R-client-148** A file with a problem is reported, its location keeps what was recorded before and its runs, and the rest of the tree is done. The command then fails, whether it prints text or JSON, and its JSON gives each problem's text by location. *Test:* `TestFileWithAProblemKeepsItsLocation` and `client_submit_problems_in_json`

**R-client-149** A run reads the tree's directory as `run.context` and its location as `run.location`, and so does what it starts. A singleton is given no input unless a submit starts it again in place of a run with `handover:`, and a singleton file whose `input:` needs a value is a file with a problem. *Test:* `TestContextIsReadByARunAndItsChildren` and `mach_singleton_needing_input_refused`

**R-client-164** `corbie test <file> --definition` prints the machine as the client writes it out, in the order it is written, each value with its file and line, and runs nothing. *Test:* `Test_client_test_definition_prints_file_and_line`

**R-client-150** `corbie test` given directories reads every machine in them as `corbie run submit` does, prints each one ok or its problems, takes no flag of a start, reads no config and no store, and starts and cancels nothing. It fails when a machine has a problem. Each directory is read as a tree, and a send or a call to one of its locations, its `at:` written out, is a problem of the sender's file where the file there has a lifetime the address does not fit, a `receive:` that does not list the sender, or an event of its own by that name, or where the sender's own lists do not hold `send:` for it; a location with no file is printed and fails nothing. *Test:* `TestTestReadsADirectory`, `TestTestChecksTheSendsOfATree`

**R-client-159** `corbie run submit <file>` given a definition or a script file alone, or with `--at`, records it as a tree of one file, at `--at`, by default `/` and the file's name without `.yaml` or `.js`, and prints its row as a directory's submit does. A file with a problem is reported and nothing is sent, so its location keeps what was recorded before. `run submit` takes no flag of a start, and a script file's own flags only with a script file (R-client-73).
*Accept:* `corbie run submit hello.yaml` prints `/hello   transient   recorded`. A singleton given with `--at /standing/steady` prints `/standing/steady   singleton   started <id>`. A file whose `stages:` is a number fails, printing nothing on standard output. *Test:* `TestSubmitRecordsAFileAndStartsALocation`

**R-client-160** `corbie run start <location>` starts a run at the location, and reads no file or directory of this computer for it: `run submit` alone records one. An argument that does not start with `/` is refused as a mistake. It takes no `--at`, `--memory` or `--tag`: the file recorded there gives its jobs their own. A start carries no secret: secrets are set at the location.
*Accept:* after `corbie run submit hello.yaml`, `corbie run start /hello --input '{"name": "world"}'` prints a run's ID, and the run ends with the output `"hello, world"`. `corbie run start /hello --secret-env HOME` is refused as a mistake, and so is `run start hello.yaml`. A file recorded at a location that is the path of a directory on this computer is started there, and the directory is not read. `corbie run start /broken --wait`, where nothing is recorded, fails naming `/broken`. *Test:* `TestSubmitRecordsAFileAndStartsALocation`

**R-client-161** A client that starts a location in a tree another client owns calls it: under the ID it chose, `run show`, `--wait` and the package's show give the call's status and its output or error from the call's result, the run the call started, and the files the output names.
*Accept:* `Test_ctl_taken_call_receipt_waits_for_its_result`: `c2`'s start of `c1`'s location is shown `running` with the run it started, then `done`, and its output's file is fetched by its path. `Test_ctl_client_call_is_refused`.

**C-client-55** A path in a machine may name a file of the context and none outside it, by a link or otherwise, so a directory's machines are what its files say. Tests: `TestPathsReadFromTheContext`, `TestLinksStayInTheContext`.

## The gateway

**R-client-109** The gateway has a route for each thing the package reads: a listing, narrowed and widened as a listing can be; one run; a run's record so far; a run's definition; and a listing and a record are each served with `?watch` as a stream that follows them, a listing's with the end of the first listing marked, a record's with the children's entries included.
*Accept:* each narrowing, asked of the listing route, gives the rows the package gives for it. Asked for a finished run, the route gives the status, the output and the permissions used, in the same JSON `corbie run show` prints; asked for its definition, the full form with each script as a blob of its type and size, and no key. A caller opens the listing stream with two runs in progress and receives two runs, then the mark, then a run each time one appears or changes, for as long as it stays connected, and a stream narrowed to one label carries only runs with that label, before the mark and after it. A caller opens the record stream for a flow with a child and receives the parent's and the child's steps and emits, each saying where it came from. Asked for the record of the same flow without following, the route gives those entries and the run as it stands, and returns.

**R-client-152** The gateway starts a run at a location a request names, with an input, as `corbie run start <location>` does. The request is JSON, or an upload form whose part `start` holds that JSON when the input has files. A request carries no definition, no file of one and no secret: a part or a field for any of them is refused, naming it.
*Accept:* after the gateway's client submits `door.yaml` at `/door-1`, a start whose part `start` is `{"at": "/door-1"}` starts a run of `door`. A start with no `at`, one with `at` that is no location, one with a part `definition` or `file:./a.js`, and one whose `start` holds `secrets` are each refused naming what is wrong, and nothing is uploaded and nothing sent. *Test:* `TestGateway`, `TestStartIsJSONWhenItHasNoFile` and `TestStartThatIsNoUploadFormIsRefused`

**R-client-162** A gateway's start of a location in a tree its client does not own is a call, and the run is answered from the call's result: its status, its output or error, and the files the output names, with a record that holds no entry.
*Accept:* `Test_client_gateway_call_is_answered_from_its_result`: the gateway's client `gw` starts a location of another client's tree whose `receive:` lists `client:gw`, with a file in its input; `GET /runs/<id>?wait` gives `done`, the output and the run the call started, the file comes from `files/file`, and the record has no entries.

**R-client-163** A gateway route names a run by its ID, and a listing narrowed with `?at=<location>` and `&key=<key>` gives the runs at a location, and with a key there.
*Accept:* after a start at `/door-1`, `GET /runs?all&at=/door-1` lists the run under the ID the start gave; `?at=/elsewhere` and `?at=/door-1&key=alice` list nothing. *Test:* `TestGateway`

**R-client-115** A stream of a record says when the run has ended and how, and a stream the gateway closes because it could not read the store says why, so each is told apart from a connection that was lost.
*Accept:* the stream of a run that finishes carries a last message `end` giving the run as `done` before it closes; a stream of a listing whose store then fails carries a last message `error` giving the failure; a stream cut by a failing network carries neither. *Test:* `TestStreamSaysWhyItStopped`

**R-client-116** Files go in as uploads, whatever their bytes, and come out by their path in a result.
*Accept:* a start of a location is made with an image as input, uploaded with the request; the run's script reads the image's bytes unchanged. A run's output shows a blob with a path; asking the gateway for that path of that run gives the bytes with their type.

**R-client-119** A gateway is one client with one ceiling: every caller's run is started by that client, all callers act as it, and nothing in a request raises the ceiling.
*Accept:* two callers start one run each; both runs carry the gateway's client in their names, each caller sees both in a listing, and either can cancel the other's. With the gateway's client given the ceiling `network`, a run started through the gateway whose definition allows `disk` is `denied` when it writes a file, whatever the request said.

**R-client-121** Callers show the gateway a token it was given when it was started, and a request without the right one is refused and does nothing.
*Accept:* a gateway is started with its tokens; a start sent with no token, and one sent with a wrong token, are refused, and the store holds no upload and no message from either.

**R-client-122** The gateway keeps nothing itself, runs wherever a process can stay up, and is not offered on serverless hosts.
*Accept:* a gateway is stopped and another started on the same config between a start and a show; the show answers as it would have, and after a start's request has ended nothing of its uploads remains on the gateway's computer. A process started on a serverless host and asked to be a gateway refuses to start and says a gateway needs a process that stays up.

**R-client-125** A request holding a field the gateway does not know is refused, and the answer names the field.
*Accept:* a start whose body has a misspelt field beside valid ones is refused naming that field, and no run is started.

**C-client-38** A token is compared in a way that takes the same time whether it is nearly right or wholly wrong. A comparison that stops at the first wrong character lets a caller find the token one character at a time. *Test:* none. How long a comparison takes cannot be tested without a test that fails by chance; both sides are hashed and compared in constant time, and that is checked by reading.


**C-client-42** An ID that is no run is answered as not found by the answer's status, and at once, on every route that names a run, the streams included; a run started a moment ago is answered as `waiting`, never as not found. A caller made to match the words of an error breaks when the words change, and one that reads not-found as "gone" starts the run again in a loop. *Test:* `client_gateway_unknown_run_is_not_found`

## For a model

**R-client-126** `corbie mcp` serves two tools to a model: the manual, which lists the pages and gives one; and the command line, which runs a command and gives back its result in the one shape and which of the three ways it ended.
*Accept:* a model's host connected to `corbie mcp` is offered exactly those two tools. The manual tool answers as R-man-21 says. A model allowed `run show` runs it for a finished run; the result is the JSON `corbie run show` prints for that run, with the ending given as success.

**R-client-129** The model may run only the commands the server was started with permission for, each named by its whole name, with the config the server was started on; only a model is allowed commands by name, and a script has no way to run them.
*Accept:* a server started with `--allow "run ls" --allow "run show"` runs both; asked for `run submit` it refuses, runs nothing, and names the command that was not allowed. A call that gives a config or a store among the command's arguments is refused; a call that gives neither lists the runs of the server's own config. No plugin or library a script can load runs a command of Corbie's.

**R-client-131** The model server needs nothing running and keeps nothing.
*Accept:* with no controller, runner or gateway running, `corbie mcp` starts, serves a manual page, and runs `corbie test` for a model allowed it. A server is stopped and started again between a model's `run submit` and its `run show`; the `run show` answers as it would have, and the server has written nothing of its own to the config or the store.

**C-client-47** What a model is told about a command that stays until a run ends says that it stays. A model that follows a run waiting on a ticket waits with it, and cannot answer the ticket from inside the same call. *Test:* `client_model_told_which_commands_stay`

**C-client-52** A call that stays keeps no other call from being answered, and ends when the model's host stops it. A model that waits for a run that never ends would otherwise leave the server answering nothing until it is started again. *Test:* `client_model_server_answers_beside_a_call_that_stays`

**C-client-49** A command refused to a model is refused before any part of it runs. A check made after the command has read its files or its store has already done what the model was not allowed. *Test:* `client_model_refused_command_does_nothing`

## Interfaces

Uses:

- **Store.** Reads heads, records, its own receipts and the results of its calls; writes messages into the controller's inbox, a submit's files under `uploads/` and a start's input into the record of the run it names, a bounded number at a time; is told at once of a write made in the same process, and otherwise looks again at the one interval (R-inst-54).
- **Instances.** The head and the entries of a record are what it reads, by the one way of watching, with its narrowings (R-inst-43 to R-inst-49); a receipt is what it gets back for a message; the three levels of watching set what it shows, and a head that is not the controller's is not shown (R-inst-01); the run's name is chosen, and its input written into its record, before the start is sent (R-inst-65); a call's result is read beside its receipt.
- **Blobs.** Makes a blob for each uploaded file, reads a blob through its reference, copies a blob from one record to another (R-blob-25), and shows a blob as its type, size and path with no key (R-blob-06).
- **Identity.** The client's own identity and its controller's card: it signs and seals each message, opens receipts and records sealed to it, and checks the controller's signature on a head. `corbie init` and the `controller`, `runner` and `client` commands that enrol and trust are Identity's (R-id-12 to R-id-22).
- **Machines, Flow, Statechart, Agent.** The writing out of includes, the filling-out of short forms and the check of a definition, which the controller runs again with the same code (R-mach-12, R-mach-17, C-mach-25, C-mach-26); the state a watcher is shown of each type's run (R-inst-72).
- **Models.** What an agent's turn needs, the connection and the key, written into the definition as it is submitted (R-model-15).
- **Permissions.** The form of a narrowing list and of the permissions given with a script file; the three parts of a refusal.
- **Secrets.** `corbie test` sets secrets and values at `/` of the controller in its process.
- **Controller.** Takes the client's submits, starts, cancels, events and answers, and writes their receipts, the results of calls, heads and records. Nothing passes between them except through the store.
- **Roles, Runner.** `corbie start` starts the roles named, and `corbie test` runs them all in one process.
- **Plugins.** The extensions the binary holds, for `corbie version`.
- **Manual.** The pages `corbie manual` and the model server's manual tool give (R-man-18, R-man-21).

Used by:

- **Manual.** Takes the table of commands and makes each command's page from it.
- **Applications.** A Go application uses the package; any other uses the gateway; a model uses `corbie mcp`; a program on PATH named `corbie-<name>` is run by `corbie <name>` and started by `corbie start`, and a program written in Go reads its command line with `cli.Program`.
