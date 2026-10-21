# Instances

The detailed rules behind the [Instances overview](../instance.md).

## The head

**R-inst-01** The head is signed by the controller, and anyone who can read it can check the signature with the controller's card.
*Accept:* read a head straight from the store and check it against the controller's card: it passes. Change one byte of it: the check fails and no viewer shows it.

**R-inst-02** The head says what the instance is called, what type of machine it is, when it started, which instance is its parent, whether it is running, done, failed or cancelled, and which step it has reached.
*Accept:* the head of a flow started as a child by a statechart gives the flow's name, says it is a flow, gives a start time, names the statechart's instance as its parent, says `running` and gives the step the controller last wrote; a root's head names no parent. The heads of four ended runs show `done`, `failed` and `cancelled` as each ended.

**R-inst-05** The head says what the instance is waiting for: how many jobs on runners, how many timers, and the names of any tickets.
*Accept:* a statechart in a state with one script job, one timer and `ticket: verdict` shows one job, one timer and `verdict` among its tickets. Once the job's result has been taken it shows no jobs; once the state is left it shows no ticket.

**R-inst-07** The head carries the labels its client gave at the start, and a child's head carries its parent's.
*Accept:* a run started with the labels `team=ops` and `nightly` shows both in every version of its head from the first to the last, and so do a child and a child of that child.

**R-inst-09** The head is sealed to nobody. Whoever the store lets read it can read all of it and needs no identity of their own.
*Accept:* with the controller's card, the store's address and read access to heads, and no identity file, a viewer lists the heads and reads every field of each.

**R-inst-10** Apart from its labels, the head holds nothing the instance was given or produced: no input, no output, no error text, no log line, no value of a ticket.
*Accept:* run a machine whose input, output, error text, log lines, ticket value and ticket answer each hold a marker string of their own, on a memory store that keeps every version of every head. No version of any head holds any of the markers. A label holding a marker does appear.

**R-inst-12** The head may be a moment behind the record, and where the two differ the record is right.
*Accept:* stop the controller after it has written step 4 and before it has touched the head, which says 3. A client's `run show` says the run is at step 4. The next pass brings the head to 4 and takes no new step to do it.

**R-inst-13** When an instance ends, its head moves from the running heads to the finished ones, and it is written among the finished before it is removed from the running, so an instance is never without a head.
*Accept:* after a run ends and the pass that ended it is over, its head is under `finished/` with its final status and nothing of that name is under `running/`. On a memory store that notes every write, from an instance's first head until it is removed as past keeping there is no moment when neither place holds its head. Stop the controller between the two writes: both heads exist, a listing that takes in both shows the instance once and as ended, and the next pass removes the running one.

**C-inst-02** A viewer that lists a running head and finds it gone when it reads looks for it among the finished before it says anything about that instance. A head can move between the listing and the read. *Test:* `inst_head_moves_between_list_and_read`

## The record

**R-inst-17** Nothing but its head and its record knows anything about an instance.
*Accept:* stop every Corbie process while an instance waits on a job, a timer and a ticket. Start a controller that has never run before, with only its private key and the store's address: the instance goes on to its end exactly as it would have.

**R-inst-20** The controller writes one step entry each time the instance takes a step, and steps are numbered one after another.
*Accept:* a run that took four steps has the entries `steps/1` to `steps/4` and no others, each written by the controller.

**R-inst-21** A step entry holds the events that arrived, names the messages and the wakes they came from, holds the state the machine was left in, and holds what the step asked for.
*Accept:* open the entry for a statechart's step set off by a job's result and a timer together, which moved it from `drafting` to `waiting`. It shows a `done` event and a `timer` event, the name of the result's message in the controller's inbox and the name of the wake, a state in which `waiting` is active with the output of `drafting`, and the job it asked for, under the name the message then sent to the runner carries.

**R-inst-25** The latest step entry is the instance's whole state, and the last step entry of an instance that has ended holds its result.
*Accept:* remove every step and emit entry of a waiting instance except its latest step; when its next message arrives it takes the same step, with the same result, as an untouched copy of the same instance. The output `run show` prints for a finished run is the output its last step entry holds, and a failed run's last step entry holds its error.

**R-inst-27** A job's log lines, notes of progress, pieces of a model's reply and named values are written as emit entries in the record of the job's instance, by the job's runner, in the order the job made them, and they reach the record and a watcher without any act of the controller.
*Accept:* send a job whose script logs `a`, emits the value `x`, reports progress and logs `b`, then stop the controller. While the job runs, a client following the record is shown those four in that order, under the job's name.

**R-inst-29** When the machine has said it wants to act on values of a name, the runner also tells the controller of each such value by a message; for values of any other name the controller plays no part.
*Accept:* a flow has a stage that takes its items from what another stage emits: each item emitted reaches the machine as an event, and a message for it was written into the controller's inbox. A job that logs a hundred lines and reports progress, in a machine that has asked for none of them, causes one message from its runner to the controller: its result.

**R-inst-32** Everything in a record is sealed so that the controller and the client that started the root can read it, and nobody else can.
*Accept:* run a root with a child, with a marker string in each input, emit and output, and read the record's values raw: no marker shows. The client that started the root opens every entry of both records. Another enrolled client handed the same raw values opens none. A runner opens no step entry and no emit entry, the emits it wrote itself included.

## How long it is kept

**R-inst-33** A definition says how long its instances are kept after they end, and an instance whose definition says nothing is kept a week.
*Accept:* an instance whose definition keeps it a day can be read twenty-three hours after it ended, and is gone after a day and a housekeeping pass. One whose definition says nothing can be read six days after it ended, and is gone after a week and a housekeeping pass.

**R-inst-35** When the time has passed, the controller deletes the record and then the head, and the run is then gone.
*Accept:* stop the controller partway through removing an instance: its finished head is there for as long as any entry of its record is, and a later pass removes what is left of the record and then the head. Afterwards no name under `running/`, `finished/` or `records/` carries the instance's name, and a client that asks for it is told there is no such run.

**R-inst-37** A child's record goes no later than its parent's.
*Accept:* a child whose definition keeps it a month runs under a parent kept a day. From the parent's end to a day and a housekeeping pass later, there is no moment when the child's record or head exists and the parent's does not.

**R-inst-38** While an instance runs, its step and emit entries older than its definition's keep are removed, its latest step excepted.
*Accept:* a statechart whose definition keeps a day takes a step every hour for three days. At the end no step or emit entry older than a day (and a housekeeping pass) remains. A statechart that has waited a month on one ticket still has its latest step, and takes its next step correctly.

**R-inst-39** A blob in the record of a running instance stays for as long as the current state of that instance, or of any instance running below it, refers to it.
*Accept:* a statechart's first state produced a file that its current state reads as `states.<name>.output`; three keeps later the file is still fetched. A parent hands a file down to a child, moves on so that its own state no longer mentions the file, and runs for three keeps while the child keeps running: the child's jobs still read the file, and once the child has ended and a keep has passed, the file is removed.

**R-inst-41** The definition, its scripts and the run's secrets are never trimmed.
*Accept:* after the trimming above, `run show` prints the instance's definition, a new job reads its script and the files it lists, and a job that names a run secret is sent its value.

**C-inst-06** A blob is trimmed only when it is older than the keep as well as referred to by no current state. Step and emit entries younger than the keep refer to blobs too, and a watcher reading them would find nothing behind the reference. *Test:* `inst_trim_spares_blob_of_a_recent_entry`

**C-inst-07** Before a blob is trimmed, the current state of every instance running below, to any depth, is read for a reference to it. A value handed down is read where it is, and a grandchild can hold the only reference left. *Test:* `inst_trim_keeps_blob_a_grandchild_uses`

## Watching

**R-inst-42** Every viewer sees what is happening the same way, by reading heads and records from the store: the command line, a terminal screen, a web page, the gateway's streams and an application built on Corbie.
*Accept:* against one store, `corbie run ls`, the gateway's list stream and the client package's list give the same runs with the same statuses, and `corbie run show`, the gateway's record stream and the package's follow give the same entries in the same order.

**R-inst-43** A watcher lists the running heads it may read, reads only those that are new or have changed since it last read them, and reports which are new and which have changed; the end of the first listing is marked.
*Accept:* with five runs in progress, a new watcher is given five heads, then the mark, then only what appears or changes afterwards; with none in progress the mark comes first. Start a run and let it take two steps: the watcher reports it once as new and reports it as changed after its head is rewritten, and a head that has not changed is not reported again. On a memory store that notes every read, a head that has not changed is not read again.

**R-inst-45** A listing can be narrowed by label, by status and by parent, and can take in the finished heads as well.
*Accept:* with runs labelled `team=ops` and `team=web`, a listing narrowed to `team=ops` gives only the first kind, and a child of one of them is given too. A listing narrowed to one instance as parent gives its children and no others. With two runs going and three ended and kept, the plain listing gives two and the wider one gives five, and the wider one narrowed to failed gives only instances whose head says failed.

**R-inst-49** For an instance whose record it may read, a watcher reads the entries it has not seen, and reads each entry once.
*Accept:* a client follows a run from its start to its end on a memory store that notes every read. Every step and emit entry of the record was shown, and each entry's value was read exactly once.

**R-inst-53** Watching writes nothing to the store and asks nothing of the controller.
*Accept:* on a memory store that notes every write, a heads-only viewer and a client each watch a run from start to end and make no write and no delete. With the controller stopped, both still list and follow everything already written.

**R-inst-54** In a directory or in memory, a watcher in the same process as the writer is told at once that something was written; any other watcher looks again at an interval, and that interval is the one setting watching has.
*Accept:* under `corbie test`, with the watching interval set to an hour, a follower is shown each entry within a moment of its being written. A watcher in another process, with its interval set to two seconds, is shown a new entry no later than about two seconds after it was written. Watching offers no other setting.

**C-inst-09** A look that cannot reach the store is made again at the next interval, and nothing is reported as changed or gone on its account. A listing turned away for load would otherwise read as every run having vanished. *Test:* `inst_watch_store_unreachable_reports_nothing`

**C-inst-10** A watcher following a run's children finds a child from any step entry that started one, not only from the latest. A child started several steps back is still running when the parent's latest step no longer mentions it. *Test:* `inst_watch_finds_child_from_earlier_step`

**C-inst-11** A watcher stops waiting on a child whose record it cannot read once the child's parent has ended. A follow that waits for every child to end would otherwise never return. *Test:* `inst_watch_ends_despite_unreadable_child`

**C-inst-12** A watcher of a listing that does not take in the finished heads lists the finished heads by name at each look and reads only those it has not listed before. A run read there that it has not reported, and that the listing keeps, is reported once, as new and ended. A run that starts and ends between two looks is never among the running heads, and a watcher that gets no notice of writes would never be told of it. *Test:* `inst_watch_reports_run_ended_between_looks`

**C-inst-13** A watcher reads a head only when it is new or a listing shows it has changed since the watcher last read it. A watcher looks every two seconds, and on a bucket a look that reads a few hundred heads takes seconds. *Test:* `inst_watch_reads_only_heads_that_changed`

## Sending

**R-inst-56** A client acts on an instance by writing a message into the controller's inbox, and there are four: start a run, cancel one, send it an event, answer a ticket. A client has no inbox of its own.
*Accept:* `corbie run submit`, `run cancel`, `run send` and `run reply` each write exactly one message under `<controller>/inbox/<client>/` and write to no other place in the controller's or a runner's area, the uploads of a start apart. After starts, refusals, cancels, events, answers and finished runs, no name in the store is addressed to a client other than its receipts.

**R-inst-58** For every message a client sends, the controller writes a receipt under the same name, saying that the message was taken or why it was refused. A start that is refused has a receipt saying why and no instance exists for it; an answer to a ticket for a run the client did not start has a receipt saying whether it was the one taken.
*Accept:* a start, a cancel, an event and an answer that the controller takes each get a receipt saying taken. A cancel for a run the client did not start gets a receipt saying why it was refused. A start whose definition does not pass its check gets a receipt listing what is wrong, and no head and no record exist under the name the client chose. Client `c2`, which can read nothing of `c1`'s run, answers a ticket of that run: its receipt says the answer was the one taken, and a second answer to the same ticket gets a receipt saying it was not.

**R-inst-61** The receipt for an event says whether the machine acted on it or had no rule for it.
*Accept:* send `go` to a statechart in a state with a rule for `go`: the receipt says it was acted on. Send `go` again while the chart is busy in a state with no such rule: the receipt says there was no rule for it.

**R-inst-62** A receipt is sealed to the client that sent the message and is kept for a day.
*Accept:* a receipt for a refused start, whose text quotes the definition, shows none of that text when read raw from the store, and another client handed the raw value cannot open it. A receipt can be read twenty-three hours after it was written, and is gone after a day and a housekeeping pass.

**R-inst-64** A message sent twice gets the same answer twice.
*Accept:* a client writes the same answer message again after the first was taken and deleted. The receipt reads as it did, the ticket is not answered a second time, and no second step is taken on its account. The same holds for a start: no second instance is made.

**R-inst-65** A client chooses a run's name before it starts the run, so an application can note the name before anything is sent, and the files it sends along are written into that run's record before the start is sent.
*Accept:* on a memory store that notes every write, a `run submit` with two files writes two blobs under `records/<client>/<the chosen name>/blobs/` and only then the start message, and the run that results has the chosen name. An application using the client package obtains the name, saves it, sends the start and is killed; a new process holding only the saved name reads the run's status and result.

**R-inst-67** A message is good for as long as its sender says, and for a few minutes when the sender says nothing. Until the controller has taken a start there is no receipt and no head, and a client that asks is told the run is waiting to start.
*Accept:* a start sent with a life of an hour is taken by a controller that first looks fifty minutes later; a start sent with no life given, found by a controller that first looks an hour later, is not taken, and its receipt says it came too late. With the controller stopped, `corbie run submit` prints the run's name and `corbie run show` on it says `waiting`, and the store holds the start message and its uploads and nothing else of the run.

## Interfaces

Uses:

- **Store**: heads under `running/` and `finished/`, records under `records/`, receipts under `receipts/`, each within the table of who may touch what; ordered listing, each name with what a read of it gives; the notice a directory or memory store gives a watcher in the same process.
- **Identity**: the controller's signature on a head; the sealing of record entries to the controller and the root's client, and of a receipt to its client; the signature of a runner on what it emits.
- **Blobs**: the larger values a record's entries refer to, where they live in the record, and the rules for when a blob in a record goes.

Used by:

- **Controller**: writes step entries, heads and receipts; brings the head up to date after each step (R-ctl-27) and moves it when an instance ends; accepts or refuses each of a client's messages; trims and removes what is past keeping, a little at a time (R-ctl-98).
- **Runner**: writes a job's emit entries and blobs into the instance's record, in batches (C-run-33), every one before the job's result (C-run-23), and a message to the controller for each emitted value the machine wants.
- **Client**: writes the four messages and a start's uploads; lists, shows and follows runs by the one way of watching; owns what a viewer is told at the awkward moments: a receipt before a head, a start never taken (R-client-36 to R-client-40), and C-inst-02.
- **Machines** and each machine type: the state a step entry holds, and what a watcher is shown of it.
- **Roles**: a pass that is killed leaves the head and the record as R-inst-12, R-inst-13 and C-ctl-16 describe.
- **Secrets**: a run's secrets are kept, sealed, in the root's record and are never trimmed (R-inst-41).
