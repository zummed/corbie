# Machines

A machine is a definition that Corbie runs. There are three types, flow, statechart and agent, and each has its own area. This area is what they all share: how a machine runs, how a definition is written, and what a machine can receive and ask for.

## Steps

A machine runs as a series of steps and never as one long process. A step is given the definition, the state the machine was left in, and the events that have arrived since. It returns the new state and a list of requests. That is all a step does. The work it asks for is done by its jobs, which are scripts and other machines. Steps of one instance happen one at a time and in order.

Because a step depends on nothing but what it is given, taking the same step twice gives the same answer, and everything a machine has ever done can be read back from its record.

A machine runs until it reaches an end of its own, is cancelled, or passes its timeout if it has one. One with no timeout may run for ever. Waiting costs nothing: a machine that is waiting is just a record in the store.

## Definitions

A definition is data, written in YAML. Its first line gives its type and its name. It may give the shape of its input, how long its instances are kept, and its limits.

Definitions are meant to be written by people and by models, so each type offers short forms where they read better. Each short form has one meaning and one place it can be written. When a client sends a definition, every short form is filled out into one full form, and only the full form is stored and run. The same filling-out is how a type offers a convenience over something Corbie already does: where a feature can be expressed as a short form over an existing mechanism it is, and the machine that runs never learns of it.

What a definition may hold is said once. Each mapping in it has a shape: the definition itself, a job, an agent's model, a state. A shape is the mapping's fields, with the kind of value each takes and what each is for.

```
 a model      name     a word     the model's name, as its service knows it        any name; these are known: …
              effort   a word     how much effort the model is to take             one of its service's: low, medium, high
              reply    a number   the most a reply may hold, in tokens
```

A definition is read by its shapes and by nothing else. A field is read only if its shape names it, as the kind of value its shape gives it, and a field its shape names is always read. A client is given the same shapes, to offer a definition's fields to a person, so what it offers is exactly what will be read. Where a field takes one of a few values the shape gives them, and where the values depend on a field beside it, such as the efforts a model's service takes, it gives them for each.

A mapping may be one of several sorts of thing that hold different fields. Its shape then says which field tells the sorts apart, and which fields each sort holds.

```
 a model, by its service     anthropic, openai     name  secret  service  effort  address  reply_tokens
                             claude-code           name  secret  service  effort  own
```

A field its sort does not hold is not offered to a person, and is refused if it is written.

A definition is checked as a whole, and everything wrong with it is reported together. Each problem names its place, and its file when the problem is in a child's file, as a path from the definition's directory: `kids/kid.yaml: value: …`. The client checks it before sending, so mistakes are found at once, and the controller checks it again with the same code, because it does not take a client's word.

An instance never changes its definition. A machine that wants to change how it works changes a child's definition and starts the child again.

## Values

Any value in a definition may be an expression, written `${…}`. A value that is exactly one expression takes the expression's value, whatever its type. Text with expressions inside it is text with the values filled in. Everything else is literal.

A condition, such as `when:`, is exactly one expression. It holds unless its value is false, null, zero or empty text.

Expressions are JavaScript. They read `input`, which is what the machine was started with, and whatever its type adds: a flow's stages, a statechart's states and current event, an agent tool's arguments. Everything that varies from one run of a definition to the next comes in through `input`, and there is no other way to vary it.

Some fields are names, not values, and an expression in them is refused: a thing's name, a type, a permission's kind, a deny entry, a secret's name, a tag, a ticket's name. The path of a script or a machine is a name too, with one exception: it may be exactly one expression that gives a stored definition or script.

An expression may run for one second. The time it sees is the time of the step, and random numbers differ from one expression to the next but are the same if the step is taken again.

## Shapes

A shape is a JSON Schema of an object, 2020-12 unless its `$schema` names another dialect. A definition's `input:`, a tool's `args:` and an agent's `reply:` are shapes. A definition writes one in full, or in a short form that gives each name a type:

```yaml
input: { urls: list, depth: number? }
```

```
  string, number, boolean   that JSON type
  list, map                 an array, an object
  blob                      a blob: in input:, the blob; in args: and reply:, the name the model was shown
  any                       any value
  ? after the type          the name may be left out
```

A shape with `type: object` is the full form, and any other is the short form. The client fills out the short form when the definition is sent. The short form of `args:` and `reply:` refuses a name it does not give, and that of `input:` takes it.

A `$ref` resolves only within its own shape. One that names a file or an address is refused when the definition is checked.

## Input

A definition's `input:` is the shape of what an instance is started with. A definition without one takes any input.

1. The controller fills in each missing name that has a `default`, in the input and in each object that `properties` and `items` lead to. A default filled in is filled the same way.
2. The controller checks the filled input against `input:`.
3. An input that does not fit fails the instance at its first step, naming each place, and nothing starts.
4. The filled input is the data of the `start` event, and what every expression reads as `input`.

```yaml
input:
  type: object
  properties:
    urls:  { type: array }
    depth: { type: number, default: 2 }
    fetch: { type: object, default: {}, properties: { retries: { type: number, default: 3 } } }
  required: [urls]
```

```
  started with                         input reads
  { urls: [a] }                        { urls: [a], depth: 2, fetch: { retries: 3 } }
  { urls: [a], fetch: { wait: 5 } }    { urls: [a], depth: 2, fetch: { wait: 5, retries: 3 } }
  { depth: 2 }                         the instance fails: missing property 'urls'
```

A `default` that `properties` and `items` do not lead to from the top, such as one under `anyOf` or behind a `$ref`, is refused when the definition is checked, and so is a default that does not fit its own place.

## Jobs

A machine contains jobs, which do its work. A job is a script, which a runner executes, or a machine, which the controller starts as a child run. Either way it has an input, the permissions it may use, a limit on time, and optionally a number of attempts. A script also has a limit on memory and tags that choose which runners may take it.

A program using Corbie as a library can list a definition's jobs, each with its place, such as `stages.a.job`.

A `timeout:` with an expression is filled in when its job is asked for, and `limits: { timeout: }` when the run starts; a filled value that is not a length of time fails the machine.

A job may be written where it is used or kept in its own file. A script is a file named by its path, and its job lists the files the script loads; a client sends them along with the definition. A relative path is read from the directory of the file it is written in: a definition, a machine kept in its own file, a file of tools, or a script that loads another with `require`.

Every job has a name that says what asked for it: a stage, a state, a tool, an agent's `turn`, the `ask`. A job's name has a suffix when other jobs have the same name:

```
get_title.0   one of several jobs asked for in one step
turn#2        the second time the name asked for a job in the run
get_title@2   the second attempt
```

- An item of a list adds its position from 0: `get_title.0`, `get_title.1`.
- A tool called twice in one agent turn has jobs named `shelf` and `shelf.1`, and a second preview before one turn is `preview.1`.

The record and every watcher use these names.

## Events

Everything that reaches a machine is an event, and every event has the same shape: a name, where it came from, and its data. A machine is started by a `start` event carrying its input. A job that ends well arrives as `done`, and its data is its output. One that does not arrives as `failed`, and its data is its error together with whatever output it still gave. A timer arrives under the timer's name. An answer to a ticket arrives under the ticket's name, and is marked as an answer, which no other event can be.

A job can also send events up while it is still going. A script does it by emitting a named value, and a child machine by sending an event to its parent. The two are the same thing and arrive the same way, as an event with that name from that job. A parent can send a named event down to a child machine. Machines talk only to their parent and their own children; siblings go through the parent.

The client that started a root stands where a parent would. It can send the root named events, and what the root sends up is written in its record, where the client and any watcher can see it.

Events sent between a parent and a child arrive once each and in the order they were sent. An event sent to a child that is not running is noted in the record as undelivered and fails nothing.

Each type handles an event in its own way, and its own area says which it acts on and which it ignores.

## Children

Larger machines are built from smaller ones. A child is started by a job and its ending is one event to its parent. A child never outlives its parent: when a machine ends or is cancelled, every script job and every machine below it is cancelled too.

A definition can be made while a machine runs, by a script or by an agent, and run as a child. It is filled out, checked and stored exactly as one sent by a client is. Whether its approvals have an ask to go to is checked when it is started, since that depends on what is above it.

A root may have eight levels of machines below it unless it says otherwise, and an ask that is a machine uses a level like any child. A definition may also limit how many jobs an instance starts. A job past either limit fails and is not retried.

## Waiting on the outside

A machine waits for a person or an outside system with a ticket: a random value that gives the right to answer one question, once.

1. A state issues the ticket, and the step makes its value.
2. The machine, or the client that started the run, hands the value to whoever should answer.
3. That person or system answers with any client, naming the instance and the ticket and giving a value.
4. The answer arrives as an event with the ticket's name. No other event is taken under that name.

The ticket is the authority: any client that holds it may answer, and Corbie does not check who did. What an answer says about who gave it is the answerer's word, which the machine's definition checks or does not. The receipt for an answer says whether it was the one taken.

Services that carry questions to people, such as a chat program, are clients at the edge and no part of Corbie.

## Approval

A job can be made to wait for a yes before it starts. A job is held if it is marked for approval, or if a permission it asks for is one its machine, or a machine above it, has listed as needing to be asked about. Being approved gives a job nothing it was not already allowed; approval only ever holds. What gives the yes is the machine's ask: a job of its own, a script or a machine, named at the top of the definition. That job is called `ask`, and nothing else in a definition may take the name. A machine with no ask uses the nearest one above it.

The controller does the holding. It gathers the jobs from one step that need approval and starts the ask with them. The ask is told which machine is asking and, for each job, what it is and what input it would get. Its result names the jobs it approves and may add a note. Approved jobs start. A job not approved never starts, and its machine sees it as a failed job with the status denied, which it may handle like any other failure. An ask that fails, or whose result does not list the jobs it approves, approves nothing.

Waiting for a person happens inside the ask, typically a statechart that issues a ticket and ends with the jobs it approves. An ask that waits for nobody, such as a model reviewing the jobs, is an ask all the same. An approval has no time limit of its own; the ask's timeout is how long it may take.

An ask's job takes input like any other. The permissions an ask needs are its own, used by the ask's job and by nothing else in a machine that inherits it.

## Failure

A job fails with an error, by running out of time, by being denied, or by being cancelled. A denial says what was refused, which layer refused it and what would fix it, and passes through a machine to its parent unchanged.

Nothing is tried again unless its job asks for attempts, and only errors and timeouts are. The controller makes the attempts, each a little later than the last, and the machine sees only the final outcome; the record shows each attempt.

A machine reacts to a failure as to any event. A failure it has no rule for fails the machine, and its own failure is the event its parent receives. If a step itself fails, the instance fails.

## Machine types

A type is an extension like any other. It supplies the shape of its definitions, the filling-out of its short forms, the check of its definitions, its step, what a watcher is shown of its state, and its page of the manual. Everything in this area is supplied by Corbie, the same for every type, and no type has its own version of any of it.
