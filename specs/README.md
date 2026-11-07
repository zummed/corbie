# Corbie

Corbie runs machines. A machine is a flow, a statechart or an agent, written as a definition in YAML, and it contains jobs, which do its work: a job is a script, which a runner executes, or another machine. One binary does everything. What a process does is set by the roles it is started with and by nothing else.

## How it holds together

Everything Corbie knows is in the store, which is a directory or a bucket. Every party reads and writes the store directly and no party talks to another in any other way: a message is a value written into the receiver's inbox. No process holds anything that matters in memory, so any of them can stop at any moment and any number of copies can run side by side.

There are three kinds of party. A **controller** advances machines. It takes each message addressed to it, works out which instance the message is for, runs one step of that instance and writes down the result. A step only asks for work; the work is done elsewhere. A **runner** does the work: it takes jobs from its inbox, runs each script with the permissions the job was given, and writes the result back. A **client** submits machine files, starts runs and reads what happened to them. The command line, the gateway and an application using Corbie as a library are all clients.

Each instance of a machine has a head and a record. The head is a small public card saying where the instance stands. The record is everything that happened to it, in order, readable only by the controller and by the client that owns the run's tree. Watching a run, listing runs, reading a result and streaming a run's progress are all the same act: reading heads and records from the store.

One script job's trip looks like this. Every step of it is a write to the store.

1. The client writes a start into the controller's inbox.
2. The controller takes step 1, which asks for the job, and writes the job into the runner's inbox.
3. The runner takes the job and runs the script. Logs and progress go into the record as emits while it runs.
4. The runner writes the result into the controller's inbox.
5. The controller takes step 2, which ends the run.
6. The client reads the head and the record.

Parties know each other by exchanging cards, which hold public keys. Messages are signed by their sender and sealed to their receiver. What a job may touch is set by permissions, written the same way at every level, and a secret reaches a script only as a handle that works at the destination it was bound to.

Corbie is built for hosts that run a function when something happens and then stop it. On such a host each role runs as one pass over the store per call. On a computer the same pass runs in a loop.

When something goes wrong, the layer that notices passes the error up untouched, and one outermost place for each step, job or command reports everything that went wrong together. Nothing in between catches, restarts or rewords.

## Trees and locations

A client submits a directory of machine files at a prefix, and the directory becomes a **tree**. Each `*.yaml` file is a machine with a **location**: its path under the prefix, without `.yaml`. A file the directory's `.corbieignore` lists has no location, and machines include it.

```
corbie run submit house --at /home

house/devices/boiler.yaml        /home/devices/boiler
house/thermostats/kitchen.yaml   /home/thermostats/kitchen
house/shared/base.yaml           listed in .corbieignore: no location
```

- A tree belongs to the client that first submitted it. Only that client submits there, and reads or cancels the runs at its locations.
- Trees do not nest. A client submits only at prefixes its ceiling's `at:` entries match.
- A run's authority comes from its location: its file's `allow:` and `ask:`, its tree client's ceiling, and the secrets and values set at its location or above.
- A sender or a caller, machine or client, adds nothing to the run's authority and takes nothing away. Its input may fill an allow scope, within the run's own patterns and every ceiling.

## How runs start

A file's `lifetime:` says how runs start at its location.

```
singleton   the submit starts the one run
keyed       one run per key, started by a send to a key with no running run; statecharts only
transient   a new run for each call or corbie run submit <location>; the default
```

The gateway starts a run by calling a location (see client.md).

A run keeps the definition it started with. A file's `update:` says what a changed file does to runs already running:

```
restart     cancels them, and starts a singleton again; a singleton always restarts
drain       lets them finish on the definition they started with; the default for the others
```

Removing a file cancels every run at its location.

## How machines talk

A statechart sends an event with a `send:` action, and a flow stage or an agent tool with `job: { send: … }`. `job: { call: { at }, input }` starts a run of a transient location and waits for its result. Only a statechart takes a sent event. An address is `{ at }` for a singleton, `{ at, key }` for a keyed run, or `{ at, run }` for one run.

Two lists must agree:

- Every permission layer of the sender, its tree client's ceiling included, holds `send:<location pattern>`.
- The receiver's `receive:` lists the sender. With no `receive:`, a file accepts from every location of its own tree and from its tree's client.

A tree's client sends to and calls runs in its own tree with no `send:` entry.

```yaml
# house/devices/boiler.yaml            location /home/devices/boiler
statechart: boiler
lifetime: singleton
receive: [thermostats/*]
initial: idle
states:
  idle:
    on: { set: applying }
  applying:
    job:
      script: ./boiler.js
      input: { target: "${event.data.target}" }
      allow: [net:boiler.lan:443, "secret:BOILER_PASSWORD@net:boiler.lan:443"]
    on: { done: idle, failed: idle }

# house/thermostats/kitchen.yaml       location /home/thermostats/kitchen
statechart: kitchen
lifetime: singleton
allow: [net:kitchen-sensor.lan, send:devices/boiler]
initial: reading
states:
  reading:
    job: { script: ./read-sensor.js }
    on:
      done:
        - { go: resting, when: "${event.data.celsius < 19}", send: { to: { at: devices/boiler }, event: set, data: { target: 21 } } }
        - { go: resting }
      failed: resting
  resting:
    timers: { again: 5m }
    on: { again: reading }
```

```
corbie controller secret set BOILER_PASSWORD --at /home/devices/boiler
corbie run submit house --at /home
```

- Only jobs of runs at `/home/devices/boiler` get `BOILER_PASSWORD`.
- The controller writes the sender into `event.from`, here `{ at: /home/thermostats/kitchen, run: <ID> }`. A reply is a send to that address.
- A send the sender's own layers refuse is `denied`. A send the receiver refuses is noted in the sender's record and fails nothing.

## Reading these specs

Each area below has an overview in this directory, written to be read. The detailed rules behind an overview are in `rules/`, under the same headings, and say nothing the overview does not. Each area is one module of the code.

## Areas

**[Store](store.md).** The one place everything is kept: named values, the three ways of writing one, and the directory, bucket and memory stores.

**[Instances](instance.md).** The head and the record of an instance, how long they are kept, and how anyone with the right to read them watches a run.

**[Blobs](blob.md).** Values too large to sit inside an entry: where they live, how they are protected, and when they go.

**[Roles](role.md).** How a controller or a runner does its work in passes, on a serverless host and on a computer, and why several copies are safe.

**[Identity](identity.md).** Parties, cards, signing and sealing, and how two parties come to trust each other.

**[Controller](controller.md).** What a controller's pass does with a message: finding the instance, running the step, carrying out what the step asked for, and the things it owns on every machine's behalf: child runs, timers, tickets, restarts, depth and approval.

**[Machines](machine.md).** What every machine type shares: the definition, values filled in from input, the events a machine receives, the requests it makes, and the contract a type keeps with the controller.

**[Flow](flow.md), [Statechart](statechart.md), [Agent](agent.md).** One area for each machine type.

**[Runner](runner.md).** What a runner's pass does with a job, and the world a script runs in.

**[Permissions](permission.md).** How what a job may do is written, layered and checked.

**[Secrets](secret.md).** How a secret is kept, bound to a destination, handed to a script and kept out of what comes back.

**[Plugins](plugin.md).** The modules a script calls, the few ways out they all go through, the one shape they all have, the ones that come with Corbie, and how more are added.

**[Models](model.md).** The services an agent's turns are sent to.

**[Client](client.md).** Submitting trees, starting, stopping and answering runs, and reading them, as a library; the command line, the gateway and the model server built on it.

**[Manual](manual.md).** The documentation carried in the binary, for people and for models.
