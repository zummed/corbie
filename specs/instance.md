# Instances

An instance is one run of a machine. It is two things in the store: a head, which says where it stands, and a record, which is everything that happened to it. Nothing else knows anything about an instance. To watch one is to read these.

## The head

The head is a small signed summary that the controller rewrites as the instance moves. It says what the instance is called and what type of machine it is, whether it is running, done, failed or cancelled, which step it has reached, when it started, which instance is its parent, and what it is waiting for: how many jobs on runners, how many timers, and the names of any tickets. It also carries its location, its key at a keyed location, the labels its client gave it at the start, the clients its file's `read:` lists, and its cause: what started it.

The head is sealed to nobody. Anyone the store lets read it can read it and can check the controller's signature. For that reason it holds nothing the instance was given or produced: no input, no output, no error text, no log line, no value of a ticket. A location, a key, labels and the clients `read:` lists are the exceptions. A label is the client's own words about the run, chosen knowing they are public, and a child carries its parent's. A location and its `read:` are chosen by its tree's client, and a key by the definition or the client that sends to it, on the same terms.

The head is a summary and may be a moment behind. The record is the truth, and the controller brings the head up to date after each step.

When an instance ends, its head moves from the running heads to the finished ones. It is written among the finished before it is removed from the running, so an instance is never without a head. A watcher of what is in progress therefore reads the heads of the work in progress and of the runs that have just ended, however much history is kept. The price is one more write at the end of every run.

## The record

The record is the instance's history, kept as entries that are each written once and never changed. The record stays where it is for as long as it exists.

A **step** entry is written by the controller each time the instance takes a step. It holds the events that arrived and which messages and wakes they came from, the state the machine was left in, and what the step asked for. Steps are numbered. The first copy of the controller to write a given number has taken that step; any other copy finds the number taken, reads that entry and carries on from there. The latest step is the instance's whole state, and its last step holds its result.

Each event in a step names its `cause`, which the controller writes:

| Event | `cause` |
|---|---|
| asked for by a step of a run: a send, a call, a start, a cancel, an event to or from a child, an end, a tell; or the note or failure the controller writes when one of these cannot be delivered | `{ run, step }`: that run and step |
| from a client: a start, an event, a call, a submit that starts a singleton | `{ client, message }`: the client and its message |
| from the run's own script job or timer | none; `from` names the job or the timer, and the step that asked for it lists it among its asks |

A viewer follows a request from a client's message through every send, call and child to its end by these causes. A run's cause is the cause of its start.

A watcher is shown, for each step, the run's `state`, in one shape for every type. It holds nothing the head says.

| Field | Type | What it holds |
|---|---|---|
| `stages` | flow | each stage and its status: `waiting`, `running`, `done`, `failed` or `skipped` |
| `states` | statechart | the active states, outermost first |
| `turn` | agent | how many turns the agent has taken |

An **emit** entry is written by a job while it runs: a log line, a note of progress, a piece of a model's reply, a named value. The job's runner writes it straight into the record, in order, and a watcher sees it as soon as it is there. If the machine has said it wants to act on values of that name, the runner also tells the controller, which costs one more message; otherwise the controller plays no part.

The record also holds the instance's **blobs**: the larger values its entries refer to. Among them is the definition the instance started with, which a later submit at its location does not change.

Everything in a record is sealed so that only the controller, the client that owns the root's tree and the clients the root's file's `read:` lists can read it (see permission.md). A client `read:` lists that the controller does not trust when an entry is written cannot read that entry.

## How long it is kept

A definition says how long its instances are kept after they end; a week if it says nothing. When that time has passed the controller deletes the record and then the head, and the run is gone. A child's record goes no later than its parent's.

An instance that runs for a long time would grow without limit, so its definition's keep also bounds its own history: step and emit entries older than that are removed while it runs. A blob in the record of a running instance stays for as long as the current state of that instance, or of any instance running below it, refers to it. The definition and its scripts always stay.

## Watching

There is one way to see what is happening, and every viewer uses it: the command line, a terminal screen, a web page, the gateway's streams, and an application built on Corbie.

A watcher lists the running heads it may read, and reads a head only when it is new or has changed since it last read it. The end of the first listing is marked, so a viewer knows when it has seen everything that already existed. A listing can be narrowed by label, by status and by parent, and can take in the finished heads as well.

A run is reported once more when it ends, saying how. For a listing that does not take in the finished heads, the watcher lists the finished heads by name at each look and reads only those it has not listed before, so a run that started and ended between two looks is reported too, once, as new and ended.

For an instance whose record the watcher may read, it then reads the entries it has not seen. Because entries never change, each is read once.

What a watcher sees depends only on what the store lets it read, and there are three levels. With no access it sees nothing. With access to heads it sees where every instance stands and nothing inside any of them, which is what a dashboard or a metrics exporter needs. A client sees where the runs of its trees, and of files whose `read:` lists it, stand and everything inside them.

Watching holds no state anywhere and asks nothing of the controller. In a directory or in memory, a watcher in the same process as the writer is told at once that something was written. Any other watcher looks again at an interval, which is the one setting watching has.

## Sending

A client acts by writing a message into the controller's inbox: submit a tree, start a run, cancel one, send it an event, answer a ticket. A client has no inbox of its own.

For every message a client sends, the controller writes a receipt under the same name, saying that the message was taken or why it was refused. This is how a client learns that a start was refused before any instance existed, and how a client that answers a ticket for a run it did not start learns that its answer was taken. The receipt for an event also names the run that took it and says whether the machine acted on it or had no rule for it. A receipt is sealed to the client that sent the message and is kept for a day, a taken call's for longer, as below. A message sent twice gets the same answer twice. Everything else a client learns, it learns from heads, records and the results of its calls.

A client chooses a run's name before starting it, because a start's input goes into the run's record before the start is sent: the input's files, then the input itself, which the start names and the controller reads. A run whose record holds nothing was never sent. An application that must survive its own restart notes the name before it sends.

A client's start of a location in a tree it does not own is a call. The client cannot read the run's record:

1. It writes the input under the name it chose, as for any start, and the controller copies the input into the new run's record, named under the tree's client.
2. The controller writes the call's receipt at once: taken, naming the run, or refused and why.
3. When the run ends, the controller writes the call's result beside the receipt: the run's status, and its output or error, with the blobs the output names.

The result is sealed to the client that called. The receipt of a taken call is kept while its run is running, and the receipt and the result are deleted together a day after the result is written. When the controller holds no card for the client as the run ends, it writes no result and deletes the receipt.

A message is good for as long as its sender says, a few minutes unless it says otherwise. A controller refuses a message past its time and the clock allowance of five minutes, by its own clock. Until the controller has taken a start there is no receipt and no head, and a client that asks is told the run is waiting to start.

A client waits for a receipt until its message is past its time and twice the clock allowance, since the controller's clock may be five minutes behind its own. A start with no receipt by then was never taken up, and no controller takes it later.
