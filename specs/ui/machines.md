# Machines

The Machines page shows the machines the controller has recorded: each tree, the machine at each of its locations, and for the one chosen its description, its location, the secrets and values it uses, its drawing and its definition written out. The Toolkits page shows the same machines from the tools up: each toolkit, what offers it and what starts that. A UI that acts as the controller offers both, and both only read: `corbie run submit` changes a tree.

On both pages, choosing a machine, a toolkit, a tab or a card's link shows it in place, and the address names it. The list keeps its place, and Back shows what was shown before.

## What is read

The pages read the store as the controller, and no directory:

```
locations/<prefix>/+/tree       a tree: its client, the directory last submitted there, and each location's lifetime
locations/<location>/+/file     the definition recorded at a location, and the child machines it names
```

- A definition is as `corbie run submit` wrote it out: what it includes is in place, and each value has the file and line it was submitted from.
- A file with a problem is not recorded. `corbie run submit` prints its problems, and its location keeps the definition recorded before.
- The UI reads the trees at the interval, and a location's definition again only when a submit changed it. The Machines and Toolkits pages share what was read.

## The list

The list holds each tree, by prefix, and under it the machine at each of its locations:

```
/home                 laptop-client · from /home/paul/house
devices/boiler        statechart: boiler · singleton
rooms/thermostat      statechart: thermostat · keyed
tools/lookup          flow: lookup · transient
/report               laptop-client · from /home/paul/reports
report                flow: report · transient
```

- A tree has its prefix, the client that owns it, and the directory last submitted there.
- A machine has its type's icon, its location written from its tree's prefix, its type and name, and its lifetime. A location the prefix names in full keeps its last name.
- The tooltip of a machine's location gives the location whole, and the tooltip of its type and name its `description:`.
- A store with no tree shows `no machines recorded`.

## A machine

Choosing a machine shows, above its view, its `description:`, where it lives and how its runs start, as its definition says:

```
Keeps one room at its target temperature.

location   /home/rooms/thermostat · 3 running · 1 failed today
lifetime   keyed: a send to a key starts the run for that key, at most 3 running at once
update     restart: a changed file cancels its runs
receives   set from schedules/*, client:panel
           and anything from its tree's client
sends to   devices/boiler, /work/log
approve    send:devices/boiler
```

| Row | What it shows |
|---|---|
| location | the location, a link to the runs there, with how many of its runs are running and how many failed today |
| lifetime | `singleton`, `keyed` or `transient`, what starts its runs, and `limits: { runs: }` |
| update | `restart` or `drain`, and what a changed file does to its runs |
| receives | who `receive:` lets send to it and call it, by event, `none` for an empty list; with no `receive:`, its own tree and its tree's client |
| sends to | the `send:` entries of its `allow:`, with `**` read as every location in its tree, `/**` and `all` as every location; for an agent, the location each tool written `call:` calls, `people/find (by the tool who)`, a location `allow:` also names given once, with its tools; `none` where there are none |
| approve | the permissions its `approve:` lists, which a job uses only after the ask approves it; no row where there are none |

A machine with no `description:` shows no line above the rows.

The tabs Drawing and Written out choose the view, Drawing by default:

```
Drawing       the machine drawn, as Drawing says; a card that names a machine unfolds it in place
Written out   the definition recorded at the location, each value with the file and line it was submitted from
```

On the drawing, each state and each running stage shows how many of the UI's client's running runs at the machine's location are in it now; another client's runs are counted at the location only.

Unfolding a card moves the drawing in to it, and folding it moves the drawing back out to the card it was in, or to the whole machine. Back folds and unfolds as the address was. Where the machine cannot be shown at the address asked for, the view says why and keeps what it shows.

The address of a machine is `/machines?at=<location>`, with `open=<card path>` for each card unfolded, a card's path being the names of the cards down to it joined by `/`: `/machines?at=/home/schedules/away&open=pause`. A run's page links to the address of the machine recorded at its root's location (see Runs).

Written out gives the machine's type and name, then its fields by name. A value's file is written from the directory submitted:

```
agent: support
model:
  effort: high                               # family.yaml:3
  name: claude-sonnet-5-5                    # base.yaml:1
```

When a submit changes the machine shown, read at the interval, it is shown again. The counts of runs follow the store, and change at once when the UI reads a change in the list of runs.

## Secrets and values

Above the view of a machine, the page lists the secrets and the values it uses, read from its definition, its child machines included:

```
secret   each NAME of a secret:NAME@… entry in an allow: list; an entry whose name holds * names none
value    each NAME an expression reads as values.NAME
```

The UI reads where each is set, as `corbie controller secret list` and `corbie controller value list` read it, and gives the location nearest the machine's, at it or above:

```
ANTHROPIC_KEY    secret · set at /
BOILER_PASSWORD  secret · not set at /home/devices/boiler or above
HUB              value · set at /home
```

A machine that uses none shows no section.

## Toolkits

A toolkit is a group of tools under an agent's `toolkits:`. The Toolkits page lists, by tree, each toolkit an agent's `toolkits:` names, written in place or included from a file, with its `description:` and its count of tools, and shows the one chosen:

```
orders                                   toolkits/orders/toolkit.yaml

The orders app.
instructions toolkits/orders/instructions.md

Offered by                                                    Started by
support  /acme/agents/support  when: input.asker.scopes.includes(…)  conversation  transient: a call, a send, or corbie run start /acme/agents/support, starts each run

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
| instructions | the first line of the toolkit's `instructions:`, or the file they were included from |
| Offered by | each agent whose `toolkits:` holds the toolkit, with the `when:` of its entry: the agent at a location, or one a machine at a location runs with `machine:` or writes in place |
| Started by | for an agent at a location, each machine of its tree that runs its file with `machine:` or calls its location, and how its location's runs start; for any other agent, the machine that runs it or writes it in place |
| Tools | each tool's `description:`, its job, the input a call fixes, its `when:`, and `needs approval` where the job has `approve: true` |

- An agent and a machine that starts it link to the machine at their location on the Machines page. One with no location is named by its file.
- A tool written `call:` gives the location it calls, linked to the machine recorded there, in any tree.
- A tool whose job is a call, `job: { call: { at: chat/post } }`, is a call too: it gives the location, linked as above, and keeps its own `description:`. A tool whose job is a send gives its event and its address, its location linked to the machine recorded there, as the agent's drawing writes them: `send ping to chat/post#infra`.
- A call's location is read from the tree's prefix, or from `/` where it begins with `/`. A location an expression gives is known only when the call runs, and is not followed.
- A tool with `when:` has a dashed border.
- A toolkit's file is the file its tools were submitted in, as the agent's definition gives it, where that is not the agent's own file. Two agents of one tree that take a toolkit's tools from the same file offer one toolkit, whichever name each gives it.
- Opening a toolkit with `approve: true` is shown as `opening it needs approval`.
