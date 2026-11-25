# Roles

A Corbie process runs as a controller, as a runner, or as both. This area is about how each goes about its work, wherever it is hosted. (`corbie start` can also start a gateway, which is a client and is described there, and can serve the controller's directory to other computers, which is described under Store.) What a controller does with a message and what a runner does with a job are in their own areas.

## Passes

A controller or runner works in passes. A pass looks at the store, does everything that is owed, and stops. A controller's pass handles every message in its inbox and every wake that has come due. A runner's pass takes every job in its inbox that it can finish and runs it. A pass ends by saying when the next thing is due, if it knows.

Nothing is carried in memory from one pass to the next. Whatever a pass needs it reads from the store, and its result it writes there before acting on it.

## Hosts

How passes are started depends on the host.

On a **serverless host**, for a role that `corbie start` runs, the host calls for a pass when something is written to the role's inbox, and calls the controller once a minute so that wakes are noticed. Both are services the host and the bucket already offer. Corbie has no signalling of its own. A pass knows how long the host will let it run and finishes before that. Whatever it has not reached waits for the next pass, and a runner does not take a job it could not finish in the time left. A gateway does no passes: the host calls it once for each HTTP request, and it answers the request before the host's limit (see client.md).

`corbie up` on a serverless host runs the controller, the runner and the gateway in the function's one process. The config and the store are in the copy's temporary directory by default, and `--store` and `CORBIE_STORE` are refused. The host calls the function for each request of the gateway. The controller and the runner start their passes as on a computer, while a request is being answered, and know no limit of the host's.

- Each copy of the function has its own store, and reads nothing another copy recorded.
- A copy submits the directory when it starts.
- The process runs only while a request is being answered, so a run proceeds only then.
- A copy's store is deleted when the host discards the copy.
- The copy prints where its config and store are, and that they are deleted with it.

Use it for a run that ends within the request that started it. A run that goes on after its request needs a store in a bucket, with a function for each role.

On a **computer** `corbie start` runs passes in a loop, sleeping until the next thing is due or for a short interval, whichever is sooner. A write to a role's inbox wakes the role at once, when it is made by:

```
  a directory          any process on the computer
  a served directory   any process on the serving computer, for a role on another computer too
  a bucket, memory     the role's own process
```

That shortcut is the only thing a computer adds. It changes when a pass happens and never what a pass does.

**`corbie test`** is the same loop with every role in one process and the store in memory. It is not a separate way of running a machine.

## A process of its own

A runner's job, and a controller's step taken apart from it (see Controller), each run in a process of their own. It is the binary, started with nothing of the role's environment, in a group of its own, with a channel to the role that nothing it prints can reach. The process ends when that channel closes.

```
  on Linux          the system refuses a step's process memory past its limit
  on Windows        the job object the process makes of itself does the same
  on every system   the process ends itself past that limit, read every 100 ms
  on every system   the role kills the process at its time limit
```

A serverless host runs a process as a computer does. A made step there costs a pass about 15 ms for the process and, beside what any step costs, two or three reads and one write of the store. It reads its definition, its note and, when a hook from above runs for a job, the definition that declares that hook, and it writes its note. The list of runners is read once in a pass, for a pass that sends a job. A host can stop a whole copy for its memory. The step is then taken once more, and its machine fails (see Controller).

## Several copies

Any number of copies of a role may run against one store, and any of them may be stopped at any instant. Three rules make that safe, and they are the whole of it.

1. Every write is only-if-new or only-if-unchanged. When two copies do the same work, one of them finds it already done and moves on.
2. A message or a wake is deleted only after the step it led to has been written and acted on. A pass that dies halfway leaves its work to be picked up again, and the first rule makes doing it again harmless.
3. A step's outcome depends only on what went into it: the definition, the state before, and the events. Two copies that take the same step write the same thing, down to the names of the jobs it asks for.

A runner takes a job by writing a mark only-if-new, so a job is run once. Two copies of a controller may take the same step at the same time. One of them wastes the effort and no harm is done. Two things keep that waste small when many copies run. Each copy starts at a different place in the list of what is waiting, so copies seldom work on the same instance at once. And a copy that finds a step already written, the head up to date and the messages it used gone knows the step was carried out, and does none of it again.

## What is not promised

A job sent to a runner that has gone away waits until its time is up. Its next attempt, if its definition gives it one, goes to another runner. Corbie does not watch for runners that have stopped.

A timer fires within a minute of its time on a serverless host, and within the loop's interval on a computer.
