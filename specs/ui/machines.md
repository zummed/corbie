# Machines

The Machines page shows the machines the controller has recorded: each tree, the machine at each of its locations, and for the one chosen its description, its location, the secrets and values it uses, its drawing and its definition. A UI that acts as the controller offers it, and it only reads: `corbie run submit` changes a tree. A toolkit is a machine, and is shown as any machine is.

Choosing a machine, a tab or a card's link shows it in place, and the address names it. The list keeps its place, and Back shows what was shown before.

## What is read

The pages read the store as the controller, and no directory:

```
locations/<prefix>/+/tree       a tree: its client, the directory last submitted there, and each location's lifetime
locations/<location>/+/file     the definition recorded at a location, and the child machines it names
```

- A definition is as `corbie run submit` wrote it out: what it includes is in place, and each value has the file and line it was submitted from.
- A file with a problem is not recorded. `corbie run submit` prints its problems, and its location keeps the definition recorded before.
- The UI reads the trees at the interval, and a location's definition again only when a submit changed it.

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
| approve | the permissions its `approve:` lists, which mark a job for approval; no row where there are none |

A machine with no `description:` shows no line above the rows.

The tabs Drawing and Definition choose the view, Drawing by default:

```
Drawing       the machine drawn, as Drawing says; a card that names a machine unfolds it in place
Definition    the definition recorded at the location, each value with the file and line it was submitted from
```

On the drawing, each state and each running stage shows how many of the UI's client's running runs at the machine's location are in it now; another client's runs are counted at the location only.

A card's name links to the Definition tab, which lights the lines written at the card's file and line. A card of a machine unfolded inside another has no such link, since the tab holds only the machine at the location.

Unfolding a card moves the drawing in to it, and folding it moves the drawing back out to the card it was in, or to the whole machine. Back folds and unfolds as the address was. Where the machine cannot be shown at the address asked for, the view says why and keeps what it shows.

The address of a machine is `/machines?at=<location>`, with `open=<card path>` for each card unfolded, a card's path being the names of the cards down to it joined by `/`: `/machines?at=/home/schedules/away&open=pause`. A run's page links to the address of the machine recorded at its root's location (see Runs).

Definition gives the machine's type and name, then its fields by name. A value's file is written from the directory submitted:

```
agent: support
model:
  effort: high                               # family.yaml:3
  name: claude-sonnet-5-5                    # base.yaml:1
```

When a submit changes the machine shown, read at the interval, it is shown again. The counts of runs follow the store, and change at once when the UI reads a change in the list of runs.

A run's page gives how the definition now recorded at its location differs from the definition the run started with:

```
- description: "a room"                 a value the run has and the location has not, with its path
+ states.open.on.shut.0.go: "closed"    a value the location has and the run has not
a script changed: ./clean.js            a file a job runs, whose contents differ
a child changed: ./inner.yaml           a child machine that differs in any of these
```

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
