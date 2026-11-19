# Machines

The Machines page shows the machines in one directory as the tree `corbie run submit` records: a list of its machines and of the files that are no machine, the problems Corbie finds in them, and for the one chosen its description, its location, the secrets and values it uses, its drawing, its text and its definition written out. The Toolkits page shows the same directory from the tools up: each toolkit, what offers it and what starts that. Every UI but `none` offers both, and both only read: the files are what `corbie run submit` reads and a model edits.

On both pages, choosing a file, a toolkit, a tab or a card's link shows it in place, and the address names it. The list keeps its place, and Back shows what was shown before.

## The directory

The directory is the one `--dir` names, by default the one the UI was started in. It is read with Corbie's `client.ReadTree`, as `corbie run submit` reads it, at a prefix:

```
acting as the controller, the directory last submitted at a tree   that tree's prefix, such as /home
any other                                                         the prefix a submit gives by default: / and the directory's name
```

Its locations, and where its secrets and values are set, are then those of the runs the tree starts.

```
house/.corbieignore             no location                 lists shared/
house/devices/boiler.yaml       /house/devices/boiler       a machine
house/shared/boiler-job.yaml    no location                 a listed file
house/.kit/review.yaml          /house/.kit/review          a machine in a directory whose name begins with a dot
house/style.md                  no location                 a file a machine includes
```

- Every `.yaml` file is a machine but those the `.corbieignore` at the top of the directory lists. Its location is its path under the prefix, without `.yaml`.
- A listed file has no location. A machine may still include it, run it with `machine:` or read it with `files:`.
- A `.yaml` file with no type, or with two, or whose YAML does not read, is a machine with a problem until `.corbieignore` lists it.
- A path in a machine may name any file inside the directory. Every path a request names is inside it, and a path that leaves it is refused.
- The UI reads the directory again only when a file in it changes, by name, size or time of change, or its prefix does, and the Machines and Toolkits pages share what it read.

The page gives the prefix above the list, `at /house`.

## The list

The list holds the machines by directory, then the files that are no machine:

```
house                                    at /house
answer.yaml        flow: answer · transient           2
devices/
boiler.yaml        statechart: boiler · singleton
rooms/
thermostat.yaml    statechart: thermostat · keyed
FILES                                         .corbieignore
.rook/                    listed in .corbieignore
people/scopes.yaml        listed in .corbieignore · included by none
shared/                   listed in .corbieignore
shared/boiler-job.yaml    listed in .corbieignore · included by 1 machine
style.md                  included by 1 machine
```

- A machine has its type's icon, its file's name, its type and name, its lifetime, and the count of the problems shown at its text. Its name's tooltip gives its location, and the tooltip of its type and name its `description:`.
- A machine with problems has the type and name its file writes at its top level, where its YAML reads and writes exactly one type, and the type alone where the name is empty. Otherwise it has the file icon and no type.
- Files lists each file and directory `.corbieignore` lists, and each other file a machine takes a value from, such as `style.md`. A file has its path and how many machines include it: those whose definition written out takes a value from it.
- A listed directory is one row. A file below it is a row of its own only where a machine includes it, such as `shared/boiler-job.yaml`: Rook's files under `.rook/` show as the one row `.rook/`.
- The heading's `.corbieignore` opens that file.
- A file beside a directory of the same name, and the files below it, have no location. Each is listed with Corbie's problem: `it is beside a directory of the same name, which holds the machine at /house/boiler/x: a location has nothing below it`.
- When a directory cannot be read, Corbie gives no tree. The page then lists no machine, file or toolkit, only Corbie's error.

## Problems

Above the list of Machines and of Toolkits, each problem of the directory's machines is listed once, at the file Corbie names, with how many machines it breaks:

```
2 PROBLEMS
toolkits/messages/toolkit.yaml:5
toolkits.messages.tools.tell.description  a tool says what it is for, for the model to choose by
breaks 3 machines
toolkits/messages/toolkit.yaml:6
toolkits.messages.tools.tell.does  "does" is not a field that belongs here: a tool takes description, args, job, list, call, input, approve, when
breaks 3 machines
```

- The file, the line and the place are Corbie's. The place is read from the top of the machine that includes the file.
- A problem at the same file and line with the same text is one problem. Machines that include a file under different names give it different places, and the first machine's is shown.
- Paths in a problem's text are written from the directory.
- A problem is drawn the same way above a list, above a file's text and above a toolkit: a box with a rowan border, holding each problem's place, its text and the machines it breaks.
- A file an `include:` names that cannot be read is one problem at that file, breaking each machine that includes it, however each writes its path. The other problems of those machines are not listed. Where that file is a machine, its own problem is the one problem, and it breaks that machine too.
- A machine with problems that reads without any from a directory below this one, read as a tree with that directory's own `.corbieignore`, is listed under one problem for that directory: `grouse/ is the top of a tree, and its machines read their paths from there: start the UI in grouse/`. The machine's own problems are not listed, here or at its text.

## A machine

Choosing a machine shows, above its view, its `description:`, where it lives and how its runs start, as its file says:

```
Keeps one room at its target temperature.

location   /house/rooms/thermostat
lifetime   keyed: a send to a key starts the run for that key, at most 3 running at once
update     restart: a changed file cancels its runs
receives   set from schedules/*, client:panel
           and anything from its tree's client
sends to   devices/boiler, /work/log
approve    send:devices/boiler
```

| Row | What it shows |
|---|---|
| location | the prefix and the file's path, without `.yaml`; where the UI offers Runs, a link to the runs there |
| lifetime | `singleton`, `keyed` or `transient`, what starts its runs, and `limits: { runs: }` |
| update | `restart` or `drain`, and what a changed file does to its runs |
| receives | who `receive:` lets send to it and call it, by event, `none` for an empty list; with no `receive:`, its own tree and its tree's client |
| sends to | the `send:` entries of its `allow:`, with `**` read as every location in its tree, `/**` and `all` as every location; for an agent, the location each tool written `call:` calls, `people/find (by the tool who)`, a location `allow:` also names given once, with its tools; `none` where there are none |
| approve | the permissions its `approve:` lists, which a job uses only after the ask approves it; no row where there are none |

A machine with no `description:` shows no line above the rows.

A machine with problems shows only its text, with no location. One whose YAML does not read, or writes no type or two, is headed `a machine`.

The tabs Drawing, Text and Written out choose the view, Drawing by default:

```
Drawing       the machine drawn, as Drawing says; a card that names a machine unfolds it in place
Text          the file as it is written
Written out   the definition Corbie stores: what it includes in place, each value with its file and line
```

Unfolding a card moves the drawing in to it, and folding it moves the drawing back out to the card it was in, or to the whole machine. Back folds and unfolds as the address was. Where the file cannot be shown at the address asked for, the view says why and keeps what it shows.

The address of a machine is `/machines?file=<file>`, with `open=<card path>` for each card unfolded, a card's path being the names of the cards down to it joined by `/`: `/machines?file=schedules/away.yaml&open=pause`. A run's page links to its machine's address where the run's tree was submitted from this directory at the prefix the page reads it at (see Runs).

Written out is what `corbie test <file> --definition` prints. Each value's file and line links to that line of that file's text:

```
model:
  name: claude-sonnet-5-5                    # base.yaml:1
  effort: high                               # family.yaml:3
```

When the directory changes on disk, read at the interval, the file shown is shown again.

## A file that is no machine

Choosing a file from Files shows its text, and above it the machines that include it, each a link. A file no machine takes a value from says so. A listed file, or a file in a listed directory, is headed `listed in .corbieignore`, and any other file `a file`.

Any other file a link opens, such as a tool's script, is headed `a file` and shows its text alone.

## The text

A file is shown as text, read-only, with its lines numbered, and its keys, expressions and comments set apart. Above the text are the problems found in it, and the lines they are found at are marked.

```
a machine        the problems Corbie gives for it, wherever each is
any other file   the problems found in it by the machines that read it
```

A file with no problems shows no problems section.

## Secrets and values

Above the view of a machine, the page lists the secrets and the values it uses, read from the machine written out, its child machines included:

```
secret   each NAME of a secret:NAME@… entry in an allow: list; an entry whose name holds * names none
value    each NAME an expression reads as values.NAME
```

Acting as the controller, the UI reads where each is set, as `corbie controller secret list` and `corbie controller value list` read it, and gives the location nearest the machine's, at it or above:

```
ANTHROPIC_KEY    secret · set at /
BOILER_PASSWORD  secret · not set at /house/devices/boiler or above
HUB              value · set at /house
```

Acting as a client alone, it lists the names with no state, and one line gives the commands that set them at the machine's location. A machine that uses none shows no section.

## Toolkits

A toolkit is a group of tools under an agent's `toolkits:`. The Toolkits page lists each toolkit an agent's `toolkits:` names, written in place or included from a file, with its `description:` and its count of tools, and shows the one chosen:

```
orders                                   toolkits/orders/toolkit.yaml
Drawing   Text

The orders app.
instructions toolkits/orders/instructions.md

Offered by                                                         Started by
support  agents/support.yaml  when: input.asker.scopes.includes(…)  conversation  transient: a call, a send, or corbie run start /acme/agents/support, starts each run

read_file
Read one file of a project.
call gitlab/files/read
input: project

change_file                                                        needs approval
Change a file of a project.
call gitlab/commits/create
input: project
when: input.asker.scopes.includes('orders:write')
```

| Field | What it shows |
|---|---|
| instructions | the first line of the toolkit's `instructions:`, or the file they are included from |
| Offered by | each agent whose `toolkits:` holds the toolkit, in its own file or written in place in another machine, with the `when:` of its entry |
| Started by | for an agent in its own file, each machine that runs it with `machine:` or calls its location, and how its location's runs start; for one written in place, the machine it is written in |
| Tools | each tool's `description:`, its job, the input a call fixes, its `when:`, and `needs approval` where the job has `approve: true` |

- A tool written `call:` gives the location it calls, linked to the file of the machine there, and that machine's `description:`.
- A tool whose job is a call, `job: { call: { at: chat/post } }`, is a call too: it gives the location, linked as above, and keeps its own `description:`. A tool whose job is a send gives its event and its address, its location linked to the file of the machine there, as the agent's drawing writes them: `send ping to chat/post#infra`.
- A call's location is read from the tree's prefix, or from `/` where it begins with `/`. A location an expression gives is known only when the call runs, and is not followed.
- A tool with `when:` has a dashed border.
- A toolkit's file is the file its tools are written in, as the agent written out gives it, where that is not the agent's own file. Two agents that take a toolkit's tools from the same file offer one toolkit, whichever name each gives it.
- Opening a toolkit with `approve: true` is shown as `opening it needs approval`.
- A file no agent's `toolkits:` includes is not on the Toolkits page, whatever it holds. It is a file like any other: in the Machines page's Files it has a row of its own where `.corbieignore` lists it or a machine includes it, and below a listed directory, with nothing including it, it is under its directory's row and has none of its own.
- A machine with problems offers no toolkit, since Corbie does not write it out. A toolkit whose file has problems is marked `has problems`, and the problems above it name the machines it breaks.
- A toolkit's file with problems shows them above the toolkit, as the text does.
- The tabs Drawing and Text choose the view, as on the Machines page, and Text shows the toolkit's file as the text view does. A toolkit written in place in a machine has no tabs.
