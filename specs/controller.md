# Controller

The controller advances machines. It owns every instance's head and record, and it is the only party that takes a step. It has no listener and no interface of its own: it reads its inbox in the store, and everything it does is a write to the store.

It brings nothing with it but its private key. Its list of trusted parties, its trees, and the secrets and values set at locations are in the store, like everything else.

## What arrives

Everything that makes a machine move arrives as a message in the controller's inbox or as a wake coming due.

A message comes from a client submitting a tree, starting a run, cancelling one, sending an event, calling a location or answering a ticket; from a runner returning a job's result or passing on an emit the machine wants; or from the controller itself, which is how a child's start, a child's ending, events between a parent and a child, and sends and calls between runs travel. A wake is anything owed to time: a machine's timer, a machine's timeout, a job's result falling due, the next attempt at a job.

The controller accepts a message only from a party it trusts, signed by that party and sealed to itself. A client may cancel only runs at its own trees' locations, and may answer any ticket it holds. It starts a run in another client's tree only by calling a transient location: its start of a location in another client's tree is a call. A client's send or call is checked as a run's is (see Sending and calling). A result or an emit is accepted only for a job the instance is still waiting on, from the runner the job was sent to.

A sender's messages are named so that they sort in the order they were sent, and the controller takes them in that order.

Messages may arrive more than once and nothing is harmed. Each kind is checked against the instance's state: a result the instance is not waiting for is dropped, a start for an instance that exists is dropped, an answer to a ticket already answered changes nothing.

## Taking a step

For each instance with something waiting, the controller does five things in this order.

1. **Find the latest step.** The head says roughly where the instance is; the controller reads step entries from there to the last one.
2. **Write the next step.** It gives the machine the events from every message and due wake for the instance, in order, and writes the result as the next numbered step, only-if-new. The entry names the messages and wakes it used. If the number is taken, another copy got there first: read what it wrote and go back to 1.
3. **Carry out what the entry asks for.** Send script jobs, start children, send events, write wakes, copy blobs to a parent, write receipts. Every one is written only-if-new under a name the entry gives. The entry says where each script and machine is, and how long the instance is kept, so this reads no definition.
4. **Bring the head up to date,** only-if-unchanged and only forward.
5. **Delete the messages and wakes the entry names.**

A controller stopped at any point leaves the same messages and wakes waiting. The next pass starts again at 1, and everything it repeats is harmless.

A step may ask for more than one pass can carry out, such as a thousand jobs on a host that gives a pass a few seconds. The controller notes in the record how far it has got, a batch at a time, and the next pass goes on from there.

Some steps are the controller's own and the machine is not consulted: an attempt failed and another is due, a job is being held for approval, a result did not come in time, the run was cancelled or its timeout passed, a step of a machine made while running passed a limit. They are written in the record like any step, so the record shows them and nothing about an instance is remembered anywhere else.

When a machine's last step both sends events and ends the machine, the events are sent first, so a parent sees a child's last words before its result.

## A step in a process of its own

Two kinds of step are taken in a process of their own: every step of a machine made while running, and every step after the first of an agent with a `list:` tool. The machine above an agent, and a tool's machine below it, are taken as their own definitions say.

## A machine made while running

A machine is made while running when the job that starts it gave a stored definition by an expression, as `machine: ${…}` does. Everything below it is made while running too, in place or by name, however deep. A machine run as an ask for it is not, since the ask it goes to was written by a machine not made while running, unless that ask itself gave a stored definition. The controller notes this in the record when it starts the child, and nothing a definition says changes it.

A step of such a machine reads its definition in a process of its own, with a limit on its memory and its time, and a step that passes either fails that machine and nothing else.

```
  time     10 seconds    a step takes about 15 ms
  memory   1 GiB         a step holds about 25 MiB, and 60 MiB when it reads 4 MiB of state
```

1. The controller reads the definition as plain JSON and notes in the record that the step is being taken.
2. It starts the process and gives it the definition, the state before and the events.
3. The process reads the definition and gives back what the machine asks for.
4. The controller takes that as it takes what any machine asks for, writes the step and carries it out.

Everything that compiles or evaluates happens in that process: checking the definition, compiling and filling in its expressions, compiling its shapes and checking values against them, and running the type's step. The process holds no key and reaches no store, and it is given no secret.

A bug in what the process runs gives a made definition that process, so its answer is a definition's word and no more. The answer holds what the machine asks for and nothing of the controller's. The same code then runs for a made machine as for any other.

```
  the definition, as plain JSON   the machine's name and type, ask_about, keep, limit on jobs, deny list, scripts and machines
  the answer                      the machine's own state, its requests, its input with defaults, its time limit, its filled-in allow list
  the controller                  every name in the store, every runner, every status, every flag, every receipt, what a step used
```

- A made machine's `keep` is no longer than its parent's, and its time limit no longer than the job that starts it allows.
- Whether a job is held is worked out by the controller, from the lists above the machine.
- A made machine has no ask of its own, and one that writes an ask fails as it starts. Nothing it wrote of one is kept, so a process that skips that check gains nothing. A held job goes to the nearest ask above it that was not made while running, and with none it is denied.
- Every ask is that owner's words, and the controller fills it in from the input of the machine that declares it, in its own process. No process fills in an ask, and an answer cannot name one.
- A name in an answer goes into names in the store, so it is one a definition could hold, of at most 100 characters. Any other fails the machine.
- A job named `ask` in an answer fails the machine too.
- A machine made while running that asks to send to a run other than its parent or its children, or to call a location, fails. The controller knows which machines those are from its own record.

An answer is data of at most 4 MiB, nested at most 1000 deep, of at most 1024 sends, and with the state it adds to at most 4 MiB. Past any of these the machine fails, since a step is read on every later pass. The controller compiles and evaluates none of it, and no step an answer leads to can end the controller or stop its passes.

A step past a limit, or whose process ends without an answer, ends the machine as failed, with the reason, in a step of the controller's own. Its parent receives an ordinary failure.

A controller that stops ends the step's process too, and leaves the note. A pass that finds a note less than 15 seconds old, a step's time and 5 seconds, leaves the step alone, since another copy may be taking it. After that time it takes the step once more. A step taken twice with no answer ends the machine, so a step that ends its controller does so twice at most.

## An agent with a list

A `list:` job gives shapes from outside the definition, which the agent's steps compile and check values against. So every step after the agent's first is taken as a made machine's is, with the same limits and the same bounds on its answer. Its first step holds nothing from outside, and is taken in the controller.

- The controller notes this in the state at the first step, from the definition read as plain JSON. An answer has no say in it.
- The agent stays the owner's: nothing narrows it, and its asks are its definition's. A step names where its job's ask is written, and the controller reads what is written there from the definition and fills it in.
- A step takes about 16 ms where it took 5 ms, with 20 listed tools, and one more write to the store.
- The step's whole time and 5 seconds must fit in what the host leaves of a call, so a host whose calls are shorter than 15 seconds fails such an agent and says so.

A pass takes four such steps at a time, and the rest wait for the next pass: at once on a computer, and at the next call on a serverless host, within a minute. A pass waits for them, and an owner's run pays that wait in each pass it needs. Beside eight machines that each used a step's whole time, a run of 20 ms took 19 to 20 seconds.

A pass on a serverless host takes a step only if the step's whole time and 5 seconds fit in the time the host has left. Otherwise the step waits for the next call and spends no try. A host whose calls are too short for that fails the machine, and the failure says so.

Any other machine is an owner's words, and its steps are taken in the controller's own process.

## Sending a job

A script job is sent as a message to a runner, signed by the controller and sealed to that runner. The message carries everything the runner needs to know: where in the record to read the script, its files and the input; each layer of permissions that applies, by name, so a refusal can say which layer refused; the values of the secrets it may use, set at its location or above; its limits; where in the record the job writes what it emits; and which emitted names the machine wants to hear about.

A job's tags choose its runner. The job goes to a runner that has every tag it names, and a job that names none may go to any runner. Where several qualify the controller picks one from the request and the attempt, so the same attempt always goes to the same runner, and a later attempt goes to a different one where more than one qualifies. It takes no account of load. A job whose tags no runner has, or which names a value or secret not set at its location or above, fails before anything is sent, naming the name and the location.

Every job has a time by which its result is due. If none has come by then, the machine receives a timeout in its place. A result that has arrived always wins over its own due time, however late the controller is in looking.

## What the controller does for every machine

Some things are the same whatever the machine's type, and the controller does them so that no type has to.

- **Restarts.** It makes the restarts a job asks for and gives the machine only the final outcome. Values an attempt emitted stand, and the next attempt's are delivered too, so a job that repeats itself delivers its values again.
- **Children.** It starts child machines, passes events between parent and child in order, copies what a child hands up into the parent's record, and cancels everything below a machine that ends or is cancelled. A script job is cancelled by a message to its runner.
- **Time.** It keeps timers and delivers them, and ends a machine whose timeout has passed. That machine fails as out of time and everything below it is cancelled.
- **Tickets.** It makes tickets and takes their answers.
- **Limits.** It counts depth and jobs against their limits, and a keyed location's running runs against `limits: { runs }`.
- **Approval.** It holds the jobs that need approval, starts each ask once for the jobs of a step that go to it, and releases or refuses them when the ask has ended.

## Trees and locations

A client submits a directory at a prefix, and the controller records it as a tree.

```
$ corbie run submit house --at /home
```

| File | Location |
|---|---|
| `house/devices/boiler.yaml` | `/home/devices/boiler` |
| `house/rooms/room.yaml` | `/home/rooms/room` |
| `house/tools/lookup.yaml` | `/home/tools/lookup` |
| `house/shared/base.yaml` | none: it has no type line, so it is a piece |

1. The controller refuses a prefix that the client's ceiling's `at:` entries do not match.
2. It refuses a prefix inside or above another tree, and a tree another client owns, naming that tree or its owner.
3. It refuses a file whose location would be below another's, such as `boiler/x.yaml` beside `boiler.yaml`.
4. It records each file, written out with its includes, at its location, and acts on what changed (see A changed or removed file).
5. The receipt names the directory that last submitted there, and the values and secrets the files use that are not set at their locations or above.

A submit changes no other tree. Only the tree's client reads and cancels the runs at its locations. Another client starts a run there only by calling a transient location (see Sending and calling).

## Starting runs

| `lifetime:` | The controller starts a run |
|---|---|
| `singleton` | at a submit, when the location has no running run |
| `keyed` | at a send to a key with no running run, with the event's data as input |
| `transient` (default) | at a call, or at `corbie run submit <location>` from the tree's client |

- Every run keeps the definition it started with, in its own record.
- A run's authority comes from its location: its file's `allow:` and `ask:`, its tree client's ceiling, and the secrets and values set at its location or above (see permission.md).
- The controller reads the values at a run's first step and keeps them in its record.
- A child started by `machine:` runs at its parent's location.
- A run started by a send or a call counts one level below its sender, and fails as it starts if that passes its own file's depth limit.

## A changed or removed file

| The submit finds | The controller |
|---|---|
| a new `singleton` file | starts its run |
| a changed file with `update: restart` | cancels its runs, and starts a singleton again on the new file |
| a changed file with `update: drain` | leaves its runs to finish on the definition they started with |
| a file gone from the tree | cancels every run at its location |

- `drain` is the default for keyed and transient files. A singleton is always `restart`, and its new run starts once every run it replaces has ended.
- A keyed run cancelled by a restart starts fresh at the next send to its key.
- Events a cancelled run had kept under `defer:` are noted as undelivered in each sender's record.
- The controller finds a location's runs by listing its tree client's running heads, each of which names its location.

## Sending and calling

Addresses and `receive:` are written as machine.md says.

1. While the sender's step is worked out, the controller checks `send:` in every permission layer of the sender. A refusal is `denied` (see permission.md).
2. It names the receiving run, and writes the message when it carries out the step.
3. The receiving run takes the message only if its `receive:` lists the sender, by default every location and the client of its own tree. A run checks the definition it started with, and a run about to start checks its location's file.
4. The controller writes the sender into `event.from`: `{ at, run }`, with `key` for a keyed run, or `{ client }`, whatever a client wrote.

The sender's record names the run the step named, or the run the send started. A run that ended first passes the event on, and the receiving run's record names the sender. A client's send is routed the same way, and its receipt names the run that took it. The tree's client names a run of its own by its ID alone.

These sends are noted as undelivered in the sender's record and fail nothing:

- The receiver's `receive:` does not list the sender.
- No run at the address is running, or the run is a flow or an agent.
- The key has no running run and its location has `limits: { runs }` running already.
- The event is named `start`, `done`, `failed`, a timer's name or a ticket's name.
- The address names a run by its ID that is not at the location the address gives, or a client.
- A run that had kept the event under `defer:` was cancelled.

An undelivered message is deleted, and then a note goes to the sender's record. A controller stopped between the two writes no note. Events from one run to one run arrive once each, in the order sent, and nothing is promised between two senders.

A call is a job the controller carries out itself, and a client's start of a location in another client's tree is one too:

```yaml
job:
  call: { at: tools/lookup }
  input: { room: "${input.room}" }
```

1. The controller starts a run of the `transient` location with the input, as a root of that location's tree client.
2. The job's output is that run's result, and a client's call has the result written beside its receipt (see instance.md).
3. The job fails if `receive:` refuses the caller, the location is not `transient`, or the run fails.
4. Cancelling the calling job, or ending the run that holds it, cancels the run.

- The caller's record names the run it called, and that run's head names the caller.
- A call is checked against a `receive:` list. A mapping names events, and takes a call only from the tree's client.
- A called run's held jobs go only to its own file's `ask:`, and are denied when it has none.

## Keyed runs

A key's file is at its location (see store.md), named by the SHA-256 of the key. It holds the key, the run and the message that starts the run, signed by the controller and sealed to it. A singleton's run is named the same way, by the file of the empty key, which the submit writes as it starts the run. A send to `{ at }` never starts a run: with no running singleton it is noted as undelivered.

1. A send reads the file once, while the step is worked out, and the step names the run the event goes to: the run the file names or, when it names none at a keyed location, a new run. A carry-out done again goes to the same run.
2. A message to a key carries the event, and what would start a run there. The new run's first step takes both, and the run with the key takes the event alone.
3. A run checks `receive:`, then takes its key, only-if-new, after its definition has been read and before anything of the run is written. It then copies its location's file into its record. A run that is refused or loses has done nothing.
4. A run's last step is written, then its key's file is deleted, then the step is carried out.

The message of an event sent to a key is deleted only when a step of a running run has used it. A run that cannot take it, because it has ended, never began or lost the key, leaves it in the inbox, and the controller rewrites it in place, one write, to name the run that has the key or, with none at a keyed location, the run it starts. A message rewritten in place keeps its name, so it sorts as it did. A run that ended with the event in its record has taken it, and the message is dropped.

A message that would start a run that exists is that run's own start written again by a carry-out, and is deleted.

| The file names | What it means |
|---|---|
| a run that is running | the key is in use |
| a run whose start is still in the inbox | the key is in use: a controller stopped before writing that first step |
| a run that has ended | the file is replaced, only-if-unchanged |
| a run with no step and no start | the file is replaced, only-if-unchanged |

An event sent to a key is the first event of its step. The events of a step after one that ends the machine are not acted on, and this one has to be. An event sent to a run by its ID is taken in such a step and not acted on, as an event with no rule is. It waits for the next pass while an earlier event from the same sender to the same key is on its way to another run, so no event is taken before one sent ahead of it. An event sent to a run by its ID waits the same way behind an earlier one from the same sender to that location.

An instance remembers the last message taken from each of its 64 most recent senders, and one that does not sort after it is a copy from a carry-out done again.

`limits: { runs }` is checked while the sender's step is worked out, by listing the key files at the location, and again when a message moves on to start a run. Sends from several runs at once may each find room, so the count can pass the cap by a few.

A send costs one read of the file, and one of the location's file when it starts a run. It lists nothing when the location has no `limits: { runs }`. A step's files are read side by side, since a step that does not fit in a pass is never written. Taking a key costs a read and a write, and reading the run it names when another has it. Freeing one costs a read and a delete.

## Housekeeping

The controller removes what is past keeping: finished instances after the time their definition gives, the older history of instances still running, receipts with the results of calls and their blobs beside them, and uploads for runs that never started and for submits never taken. It does this a little at a time as part of its passes, and a pass that is busy with machines does not put it first. What is due to go is brought up by a wake, as everything owed to time is: each instance has its own for its history and for its end, and the controller sets one for itself, once an hour, for receipts, results and uploads.

## When things go wrong

If the store cannot be reached, nothing fails. The pass ends, and what was waiting is still waiting for the next one.

If a message is too large to send, or a result too large to deliver, the job it belongs to fails with a reason, and nothing is left waiting for something that will never come. The same goes for a message to the controller itself that finds its name holding another message. A carry-out done again finds the same message there, and that is no fault.
