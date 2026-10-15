# Controller

The controller advances machines. It owns every instance's head and record, and it is the only party that takes a step. It has no listener and no interface of its own: it reads its inbox in the store, and everything it does is a write to the store.

It brings nothing with it but its private key. Its lists of trusted parties and of secrets, like everything else, are in the store.

## What arrives

Everything that makes a machine move arrives as a message in the controller's inbox or as a wake coming due.

A message comes from a client starting a run, cancelling one, sending it an event or answering a ticket; from a runner returning a job's result or passing on an emit the machine wants; or from the controller itself, which is how a child's start, a child's ending and events between a parent and a child travel. A wake is anything owed to time: a machine's timer, a machine's timeout, a job's result falling due, the next attempt at a job.

The controller accepts a message only from a party it trusts, signed by that party and sealed to itself. A client may cancel or send events only to runs it started, and may answer any ticket it holds. A result or an emit is accepted only for a job the instance is still waiting on, from the runner the job was sent to.

A sender's messages are named so that they sort in the order they were sent, and the controller takes them in that order.

Messages may arrive more than once and nothing is harmed. Each kind is checked against the instance's state: a result the instance is not waiting for is dropped, a start for an instance that exists is dropped, an answer to a ticket already answered changes nothing.

## Taking a step

For each instance with something waiting, the controller does five things in this order.

1. **Find the latest step.** The head says roughly where the instance is; the controller reads step entries from there to the last one.
2. **Write the next step.** It gives the machine the events from every message and due wake for the instance, in order, and writes the result as the next numbered step, only-if-new. The entry names the messages and wakes it used. If the number is taken, another copy got there first: read what it wrote and go back to 1.
3. **Carry out what the entry asks for.** Send script jobs, start children, send events, write wakes, copy blobs to a parent, write receipts. Every one is written only-if-new under a name the entry gives.
4. **Bring the head up to date,** only-if-unchanged and only forward.
5. **Delete the messages and wakes the entry names.**

A controller stopped at any point leaves the same messages and wakes waiting. The next pass starts again at 1, and everything it repeats is harmless.

A step may ask for more than one pass can carry out, such as a thousand jobs on a host that gives a pass a few seconds. The controller notes in the record how far it has got, a batch at a time, and the next pass goes on from there.

Some steps are the controller's own and the machine is not consulted: an attempt failed and another is due, a job is being held for approval, a result did not come in time, the run was cancelled or its timeout passed. They are written in the record like any step, so the record shows them and nothing about an instance is remembered anywhere else.

When a machine's last step both sends events and ends the machine, the events are sent first, so a parent sees a child's last words before its result.

## Sending a job

A script job is sent as a message to a runner, signed by the controller and sealed to that runner. The message carries everything the runner needs to know: where in the record to read the script, its files and the input; each layer of permissions that applies, by name, so a refusal can say which layer refused; the values of the secrets it may use; its limits; where in the record the job writes what it emits; and which emitted names the machine wants to hear about.

A job's tags choose its runner. The job goes to a runner that has every tag it names, and a job that names none may go to any runner. Where several qualify the controller picks one from the request and the attempt, so the same attempt always goes to the same runner, and a later attempt goes to a different one where more than one qualifies. It takes no account of load. A job whose tags no runner has, or which names a secret that is not set, fails before anything is sent.

Every job has a time by which its result is due. If none has come by then, the machine receives a timeout in its place. A result that has arrived always wins over its own due time, however late the controller is in looking.

## What the controller does for every machine

Some things are the same whatever the machine's type, and the controller does them so that no type has to.

- **Attempts.** It makes the attempts a job asks for and gives the machine only the final outcome. Values a failed attempt emitted stand, and the same values from a later attempt are not delivered twice.
- **Children.** It starts child machines, passes events between parent and child in order, copies what a child hands up into the parent's record, and cancels everything below a machine that ends or is cancelled. A script job is cancelled by a message to its runner.
- **Time.** It keeps timers and delivers them, and ends a machine whose timeout has passed. That machine fails as out of time and everything below it is cancelled.
- **Tickets.** It makes tickets and takes their answers.
- **Limits.** It counts depth and jobs against their limits.
- **Approval.** It holds the jobs that need approval, starts the ask, and releases or refuses them when the ask has ended.

## Housekeeping

The controller removes what is past keeping: finished instances after the time their definition gives, the older history of instances still running, receipts, and uploads for runs that never started. It does this a little at a time as part of its passes, and a pass that is busy with machines does not put it first. What is due to go is brought up by a wake, as everything owed to time is: each instance has its own for its history and for its end, and the controller sets one for itself, once an hour, for receipts and uploads.

## When things go wrong

If the store cannot be reached, nothing fails. The pass ends, and what was waiting is still waiting for the next one.

If a message is too large to send, or a result too large to deliver, the job it belongs to fails with a reason, and nothing is left waiting for something that will never come.
