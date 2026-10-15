# Roles

A Corbie process runs as a controller, as a runner, or as both. This area is about how each goes about its work, wherever it is hosted. (`corbie run` can also start a gateway, which is a client and is described there, and can serve the controller's directory to other computers, which is described under Store.) What a controller does with a message and what a runner does with a job are in their own areas.

## Passes

A controller or runner works in passes. A pass looks at the store, does everything that is owed, and stops. A controller's pass handles every message in its inbox and every wake that has come due. A runner's pass takes every job in its inbox that it can finish and runs it. A pass ends by saying when the next thing is due, if it knows.

Nothing is carried in memory from one pass to the next. Whatever a pass needs it reads from the store, and its result it writes there before acting on it.

## Hosts

How passes are started depends on the host, and on nothing else.

On a **serverless host** the host calls for a pass when something is written to the role's inbox, and calls the controller once a minute so that wakes are noticed. Both are services the host and the bucket already offer. Corbie has no signalling of its own. A pass knows how long the host will let it run and finishes before that. Whatever it has not reached waits for the next pass, and a runner does not take a job it could not finish in the time left.

On a **computer** `corbie run` runs passes in a loop, sleeping until the next thing is due or for a short interval, whichever is sooner. When one role in the process writes something that another role in the same process reads, the second is woken at once. A served directory does the same for a role on another computer: it tells the role when its inbox is written. That shortcut is the only thing a computer adds. It changes when a pass happens and never what a pass does.

**`corbie test`** is the same loop with every role in one process and the store in memory. It is not a separate way of running a machine.

## Several copies

Any number of copies of a role may run against one store, and any of them may be stopped at any instant. Three rules make that safe, and they are the whole of it.

1. Every write is only-if-new or only-if-unchanged. When two copies do the same work, one of them finds it already done and moves on.
2. A message or a wake is deleted only after the step it led to has been written and acted on. A pass that dies halfway leaves its work to be picked up again, and the first rule makes doing it again harmless.
3. A step's outcome depends only on what went into it: the definition, the state before, and the events. Two copies that take the same step write the same thing, down to the names of the jobs it asks for.

A runner takes a job by writing a mark only-if-new, so a job is run once. Two copies of a controller may take the same step at the same time. One of them wastes the effort and no harm is done. Two things keep that waste small when many copies run. Each copy starts at a different place in the list of what is waiting, so copies seldom work on the same instance at once. And a copy that finds a step already written, the head up to date and the messages it used gone knows the step was carried out, and does none of it again.

## What is not promised

A job sent to a runner that has gone away waits until its time is up. Its next attempt, if its definition gives it one, goes to another runner. Corbie does not watch for runners that have stopped.

A timer fires within a minute of its time on a serverless host, and within the loop's interval on a computer.
