# Runner

A runner does the work. It takes jobs from its inbox, runs each one's script with the permissions the job was given, and sends back the result. This area is what a runner does with a job and the world a script runs in.

## Taking a job

A runner reads its inbox in the store of each controller it trusts. It acts on a job only if the job is from such a controller, signed by it, sealed to this runner, and still good. It does not start a job whose result is already overdue.

A runner takes a job by writing its mark, only-if-new. Only one copy of a runner can write that mark, so a job is run once. A copy that finds the mark there leaves the job alone.

A copy reads only what it can act on. It reads every cancel, and it reads a job only when it has a place for one and no copy has marked it, which the names of the marks tell it. So a copy among many, before an inbox of a thousand jobs, reads a few and not all of them. Among the jobs that are free it picks at random, so copies that look at the same moment seldom reach for the same one. A mark's name carries its job's due time, and marks whose time has passed are cleared from the listing alone.

A runner runs several jobs at once, up to a number its owner sets, and leaves the rest waiting. It does not take a job it could not finish in the time its host has left, and leaves it for a copy with more time. A job that may take longer than the host gives any call can never be taken, so the runner fails it at once and says which of the two to change: the job's time limit or the host's.

## Running it

Each job runs in a process of its own, so it can be stopped at its time limit, held to a memory limit if it has one, and cannot bring the runner down. A script may run for five minutes unless its job says otherwise. The process starts with nothing of the runner's: no files, no environment. What it then reaches is what the job is permitted, and nothing else is kept from it.

The job names its script, the script's files and its input, and the runner reads them from the instance's record.

A job may use kept values (see Store). The runner holds the key the job was sent and reads and writes the values for it. The job's process never holds the key: it asks its runner for each value.

Every job has a scratch directory of its own, made for it and removed when it ends. A runner that starts removes the scratch directories left by one that was killed before it could. A path a script writes without a leading `/`, or on Windows without a drive, is in that directory; on Windows one with a leading `/` and no drive is refused. Everything a job starts is stopped when the job ends, however it ends.

When the script returns, the runner writes any blobs in the output into the instance's record and sends the output to the controller as the job's result, along with the permissions it used. If the script throws, or is refused something, or runs out of time, the result says so. Everything a job writes for the controller is under a name that is that job's alone, so a write whose answer was lost is sent again. Only when the result is written does the runner delete the job from its inbox. It removes the mark once the job's due time has passed.

## Cancelling

The controller cancels a job by leaving a cancel for it in the runner's inbox. The copy of the runner that is running the job looks for one every few seconds, stops the job and everything it started, and sends a result saying it was cancelled. A copy that is not running that job leaves the cancel where it is, and it goes when it is no longer good.

## What a script is

A script is JavaScript, and is the body of a function. Its `input` is the job's input, and what it returns is the job's output.

```js
const page = http.get(input.url)
emit("fetched", { url: input.url, size: page.body.length })
return { title: html.parse(page.body).find("title").text }
```

A script starts with nothing: no files, no network, no environment. Everything it can do is a call to a plugin, and every call that reaches outside is checked (see Permissions). Calls wait for their answer, so a script reads from top to bottom with no callbacks.

Besides plugins, a script has a few things of its own:

- `emit(name, value)` sends a named value up while the script runs. `log(text)` and `progress(done, total)` are emits for a watcher's benefit. Emits are written to the record a few at a time, not one write each.
- `job` says which request this is. Every attempt at the same job has the same request name, so a script can use it to avoid doing something twice.
- `require` loads something by name or by path. A bare name is a plugin or a library that comes with the binary. A path starting `./` or `../` is read from the directory of the file that calls it, and is one of the files the job lists.

A script cannot start jobs, whether scripts or machines. Deciding what runs is a machine's work.

## Programs a job starts

A script that is permitted may start programs. Corbie checks the start: which program it is, and which secrets go to it. What the program then does is the program's own business. It runs as the runner's user and can read, write and connect as that user can.

A runner's owner can change that with one setting, the **exec wrapper**: a command put in front of every program a job starts. It is given when the runner is started.

```
corbie start runner --exec-wrapper "bwrap --ro-bind /usr /usr … --"

the script asks for      git status
the runner starts        bwrap --ro-bind /usr /usr … -- /usr/bin/git status
```

The wrapper can be a sandbox tool or a container, and so the owner sets what a started program can reach. Corbie passes it the job's scratch directory and permissions, in case it wants them, and claims nothing about what it does. A program finds its files in the scratch directory, so a wrapper must show that directory, and the programs, at the paths they have outside.

Nothing a script chooses reaches the wrapper itself. It is started with the scratch directory, the permissions and the runner's own search path, and no other variable. The variables a script gives a program are handed to the program inside, by the standard `env` tool, which the wrapper must be able to find; they then show in the computer's list of processes, as a program's arguments do.

## One runner is one box

Everything on a runner shares that runner: its user, its key, its computer. A job that is permitted to read the runner's files can read its key, and a runner keeps the key to the kept values of every job sent to it. Scratch directories are removed, but jobs on one runner are not walled off from each other, and on a serverless host the jobs that land on one warm copy share it in the same way.

Jobs that must be kept apart go to different runners. A runner is cheap to set apart: a container, a user of its own, or a function of its own, with tags to send the right jobs to it.
