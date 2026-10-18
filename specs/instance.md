# Instances

An instance is one run of a machine. It is two things in the store: a head, which says where it stands, and a record, which is everything that happened to it. Nothing else knows anything about an instance. To watch one is to read these.

## The head

The head is a small signed summary that the controller rewrites as the instance moves. It says what the instance is called and what type of machine it is, whether it is running, done, failed or cancelled, which step it has reached, when it started, which instance is its parent, and what it is waiting for: how many jobs on runners, how many timers, and the names of any tickets. It also carries the labels its client gave it at the start.

The head is sealed to nobody. Anyone the store lets read it can read it and can check the controller's signature. For that reason it holds nothing the instance was given or produced: no input, no output, no error text, no log line, no value of a ticket. Labels are the one exception. They are the client's own words about the run, chosen knowing they are public, and a child carries its parent's.

The head is a summary and may be a moment behind. The record is the truth, and the controller brings the head up to date after each step.

When an instance ends, its head moves from the running heads to the finished ones. It is written among the finished before it is removed from the running, so an instance is never without a head. A watcher of what is in progress therefore reads the heads of the work in progress and of the runs that have just ended, however much history is kept. The price is one more write at the end of every run.

## The record

The record is the instance's history, kept as entries that are each written once and never changed. The record stays where it is for as long as it exists.

A **step** entry is written by the controller each time the instance takes a step. It holds the events that arrived and which messages and wakes they came from, the state the machine was left in, and what the step asked for. Steps are numbered. The first copy of the controller to write a given number has taken that step; any other copy finds the number taken, reads that entry and carries on from there. The latest step is the instance's whole state, and its last step holds its result.

An **emit** entry is written by a job while it runs: a log line, a note of progress, a piece of a model's reply, a named value. The job's runner writes it straight into the record, in order, and a watcher sees it as soon as it is there. If the machine has said it wants to act on values of that name, the runner also tells the controller, which costs one more message; otherwise the controller plays no part.

The record also holds the instance's **blobs**: the larger values its entries refer to.

Everything in a record is sealed so that only the controller and the client that started the root can read it.

## How long it is kept

A definition says how long its instances are kept after they end; a week if it says nothing. When that time has passed the controller deletes the record and then the head, and the run is gone. A child's record goes no later than its parent's.

An instance that runs for a long time would grow without limit, so its definition's keep also bounds its own history: step and emit entries older than that are removed while it runs. A blob in the record of a running instance stays for as long as the current state of that instance, or of any instance running below it, refers to it. The definition, its scripts and the run's secrets always stay.

## Watching

There is one way to see what is happening, and every viewer uses it: the command line, a terminal screen, a web page, the gateway's streams, and an application built on Corbie.

A watcher lists the running heads it may read and notices which are new or changed. The end of the first listing is marked, so a viewer knows when it has seen everything that already existed. A listing can be narrowed by label, by status and by parent, and can take in the finished heads as well.

A run is reported once more when it ends, saying how. For a listing that does not take in the finished heads, the watcher lists the finished heads by name at each look and reads only those it has not listed before, so a run that started and ended between two looks is reported too, once, as new and ended.

For an instance whose record the watcher may read, it then reads the entries it has not seen. Because entries never change, each is read once.

What a watcher sees depends only on what the store lets it read, and there are three levels. With no access it sees nothing. With access to heads it sees where every instance stands and nothing inside any of them, which is what a dashboard or a metrics exporter needs. A client sees where its own runs stand and everything inside them.

Watching holds no state anywhere and asks nothing of the controller. In a directory or in memory, a watcher in the same process as the writer is told at once that something was written. Any other watcher looks again at an interval, which is the one setting watching has.

## Sending

A client acts on an instance by writing a message into the controller's inbox: start a run, cancel one, send it an event, answer a ticket. A client has no inbox of its own.

For every message a client sends, the controller writes a receipt under the same name, saying that the message was taken or why it was refused. This is how a client learns that a start was refused before any instance existed, and how a client that answers a ticket for a run it did not start learns that its answer was taken. The receipt for an event also says whether the machine acted on it or had no rule for it. A receipt is sealed to the client that sent the message and is kept for a day. A message sent twice gets the same answer twice. Everything else a client learns, it learns from heads and records.

A client chooses a run's name before starting it, because files it sends along go into the run's record first. An application that must survive its own restart notes the name before it sends.

A message is good for as long as its sender says, a few minutes unless it says otherwise. Until the controller has taken it there is no receipt and no head, and a client that asks is told the run is waiting to start. Once a start is past its time with no receipt, the run is no longer waiting: it was never taken up.
