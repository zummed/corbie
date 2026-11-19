# Runs

The Runs page lists runs, and a run's page shows one. A run has Corbie's one shape for a run and the words `corbie run show` prints, wherever the UI shows it.

## Names

Every run has a location: the location of the file it was started from, or, for a child that `machine:` started, its parent's. A run at a keyed location has a key. The UI names a run by the first of these it has:

```
title     the definition's title, read from the record of a run in a tree of the UI's client
address   its location, with #key at a keyed location: /home/rooms/thermostat#kitchen
machine   its machine's name, for a child that machine: started
```

- A run at a keyed location named by its title has its key after the title, as `thermostat#kitchen`, so runs with one title are told apart wherever they are listed.
- A run in another client's tree, and every run of a UI that acts as no client, is named by its address, or by its machine where it is a child.
- On a page narrowed to a prefix, an address is written from that prefix: at `/home`, `rooms/thermostat#kitchen`. A location the prefix names in full keeps its last name: at `/home/rooms/thermostat`, `thermostat#kitchen`.
- Holding the pointer over a name shows the run's title, its full address and its ID.
- The UI reads each title once, from the step the run's head gives, while the list holds the run. A run's title, tree and lifetime never change once it has started.

## The list

The list reads the status of every run, as `corbie run ls --all` does with no client. The store lets every client and a viewer read every run's status.

### Locations

On the left, Locations lists every location at which the list holds a run, under the client whose tree holds it. A location the UI knows only from the runs listed: a location with no run is not shown.

```
lab-client
  /home                    tree
    devices/boiler         singleton   running
    rooms/thermostat       keyed       3 running
    scenes/away            transient
ci-client
  /work/digest             keyed       1 running
```

- A location is written whole, on as many lines as it needs, since two locations may differ only at their end.
- A tree of the UI's client is a row of its own, its prefix read from the record of a run there, with its locations written from the prefix under it.
- A location in another client's tree is written in full under that client, since only a tree's client reads which tree a run is in.
- A location's lifetime is read from the record of a run there, where its tree is the UI's client's. Elsewhere a location with a keyed run is `keyed`, and any other shows none.
- A singleton shows its run's status. Any other location shows how many of its runs are running.
- A click on a tree or a location narrows the page to it, as `?at=<prefix>`. The first row, every location, takes the narrowing away.

### Sections

A root is a running run whose parent is not running. Each root is one line in Busy, Idle or Singletons, and Ended lists the runs with no parent that ended:

```
Busy         a root with a running child, a job on a runner or a ticket, or with status waiting
Idle         any other root: no running child, no job on a runner and no ticket
Singletons   a root at a singleton location in a tree of the UI's client, whatever it waits on
Ended        a run with no parent that ended in the window, newest first
```

A location's lifetime is read only in a tree of the UI's client, so a singleton in another client's tree is in Busy or Idle. The window is 1h, 6h, 24h or 7d, and 6h by default. A line of Busy, Idle or Singletons reads:

```
running  slack dm: Please restart…#slack:D1:U1:support  conversation › turn [answer]                                       20:29
running  support wants to run restart.                  slack-chat-ask [open, waiting]  timer unanswered due 10 Nov 20:29  20:29
```

- The line begins with the root's status and its name. A long address is cut in its middle, since keys differ at their end, and a title at its end, before its key.
- The chain runs from the root to the deepest running run, through the newest running child at each level. Each machine has its type's icon and opens its run's page.
- A chain of more than three machines shows the first, `…` and the last, and the `…` gives the whole chain in its tooltip.
- The brackets after the deepest machine hold the states or stages it is in, or the turn an agent is on: `turn 2`.
- Then come what the deepest run waits on and when the root started.

An ended line holds its status, name, machine, the number of runs under it where there are any, when it ended and how long it took.

A click on a line opens its root's page, whose Steps follow the newest row. The `▸` at the line's start unfolds it to its root's own row, where the root runs, and then the runs the root started, newest first: those running and those that ended in the window. This is how a person finds one message in a long conversation:

```
running  conversation [answering]                         child turn         3 Nov 20:34
running    Please restart web-1   turn [answer]                              3 Nov 20:36
done       Done: web-1 restarted  reply          done from slack             3 Nov 20:35  0.4s
done       What runs on web-1?    turn           done from answer            3 Nov 20:34  9.2s
```

```
name         the run's name
chain        the runs below it as a line's chain: the running ones, or for an ended run the newest at each level
ended with   the error it failed with or the last event it took, as its record writes it; for a running run, what the deepest waits on
runs         how many runs are under it, where there are any
when         when it started, and how long it took
```

A line shows at most 20 such rows, then `N more`, which opens the root's page. A run in another client's tree shows nothing it ended with.

What a run waits on is written as:

```
ticket <name>               a ticket
timer <name> due <time>     a timer; a run in another client's tree shows the number of timers
job <name>                  a job on a runner; a run in another client's tree shows the number of jobs
held <name>                 a job held for the ask
child <machine>             a running child
```

The names and times come from the latest step of a run in a tree of the UI's client, read once for each step.

### Narrowing

The page is narrowed by location prefix, `?at=/home`, and by label, as `--label` narrows `corbie run ls`. A prefix keeps the runs at it and below it: `/home` keeps `/home/rooms/thermostat` and not `/homes/x`. A root or ended run the narrowing leaves is listed with every run under it.

### One location

Narrowed to one location, the page has a head above the sections with the location, its lifetime, and its tree and client where the UI knows them. With a client among the UI's parties, the head offers:

```
Cancel every run here   every run of the location in the client's tree     corbie run cancel <location>
Send                    an event to a key, with data; keyed locations       corbie run send <location>#<key> <event> <data>
Start                   a run, with input; transient locations             corbie run start <location> --input <input>
```

- Cancel is offered at a location in a tree of the UI's client. The command cancels each root running there, one after another, and a run that starts in between is not cancelled.
- Send is offered at any keyed location, and Start at a transient location in a tree of the UI's client. The page shows the answer: for Send the receipt, whether the controller took it; for Start the ID of the run it started, a link to its page, or why it was refused.

## One run

A run's page opens from its line in the list. Its title line holds the run: its machine, type, status and title, then its address, which links to the page narrowed to its location, then its machine's file, then its ID, when it started, and when it ended and how long it took. Under it is the run's path, each run a link:

```
turn ⇢ support › approval
```

Under the title line, one line says why the run is where it is, and a click on it selects the row it is at:

```
waiting 12m on job fetch on runner-a                      the job the deepest running run waits on, how long since it was sent, and its runner
waiting on ticket verdict in statechart review            a ticket or a timer, where no job is going
failed (denied) at stage read in flow lookup: <refused>, <layer>, <fix>    the deepest row that failed, and what refused it
```

Where the UI offers Machines and the run's file on disk now differs from the definition the run started with, a line says `the file changed since this run started`, and opens to the lines that differ.

The path goes up from the run, by parent, to its root, and down to the deepest running run, by the newest running child at each level. Where a send or a call started the root, the run that sent or called comes first, with `⇢`; its tooltip says which, where the root is in a tree of the UI's client. The run itself is in bold, and the run of the step selected is lit. Holding the pointer over a run shows its title, its address and its ID, as over a name.

On the left are four tabs: Steps, Waiting on, Output and Children. On the right are the drawing and the details of the step selected.

### Steps

Steps lists what happened in the run and in every run under it, one row each, read from the record as `corbie run show --record` reads it. A row holds its time to the second, with its date where that is not the date the run started, its run's machine in the colour of the machine's type, and what happened:

```
statechart   entered answering · left asking, entered open and waiting · timer unanswered set, due 10 Nov 20:29 · timer unanswered fired · state answering started turn
flow         stage asker done · stage files skipped · stage answer failed
agent        turn 2 to claude-sonnet-5-5 · turn 2 called restart(vm: “web-1”) · open done
any run      started · started by /acme/grouse/slack/grouse · held restart for approval · ask started approval · done · failed (denied) · cancelled
```

Sends and calls between runs read:

```
⇠ set from /home/rooms/thermostat#kitchen                                        an event a run sent; links to that run
⇠ set from client:panel                                                          an event a client sent
⇠ poke from client:panel · no rule in resting                                    the machine had no rule for it
⇢ sent set to /home/devices/boiler · boiler                                      links to the run that took it
⇢ sent message to /acme/conversation#slack:C1:U1:support · started conversation  the send started a run at a keyed location
⇢ sent set to /home/rooms/thermostat#hall · undelivered                          its details say why
⇢ sent ack to /home/rooms/thermostat#hall · thermostat                           a reply to a keyed run gives its key
⇢ sent ack to client:panel · undelivered                                         a reply to a client, which has no inbox
⇢ called /home/services/weather · weather                                        a call job; links to the run it started
```

- A row that ends a job says how long the job took, from the step that asked for it: `stage asker done · 3.2s`.
- An event from a run that the machine had no rule for is not marked, since its run's record does not say so.
- Holding the pointer over a row's machine shows its run's title, its address and its ID.
- A child's rows are under the row that started it, one level in, with a faint line at each level. Rows deeper than four levels are drawn at the fourth, and the path gives their depth.
- A send and an event from a run have a dashed mark.
- An undelivered send's row is in the colour of a failure.
- The controller notes in the sender's record a send that was not delivered, or whose event went to another run, as when another send took the key first. The send's row says it, and the note has no row of its own.
- An undelivered send gives the reason in its details: the receiver's `receive:` does not list the sender, no run is running there, the location's limit of runs is reached, the run is a flow or an agent, which takes no events, the run was cancelled while it kept the event under `defer:`, or the address is a client, which has no inbox.
- A send a layer of the sender refused is `denied`: a send job's row says so, and a statechart's send ends its run `failed (denied)`, whose details give what was refused, the layer and the fix.
- The row that started a child ends with the child's title where it has one, its status, its steps and those of the runs under it, and how long it took: `state answering started turn · running · 22 steps`. The child's rows fold into that row.
- An ended child is folded, and a running one open. Open all and Fold all fold every group.
- On the page of a run in another client's tree, there are no Steps.

### Following a request

A send or a call to a run in a tree of the UI's client folds the rows it caused in that run, so one request can be followed from run to run on one page. Its row ends with the run's title, where it has one other than its machine's name, and its status, and its group is folded:

```
call    every row of the run the call started
send    the rows of the steps the send caused in the run that took the event, with the runs those steps started
```

Corbie names the cause of each event a step takes. A step the send caused is one of these:

- The step took the event, whose cause is the sender's step.
- The step took the end of a job, a timer, a child or a call that a step the send caused asked for.
- The step took an event whose cause is a run started by a step the send caused, or a run started by such a run.

- The rows are read when the group is first opened, and read again as the page follows the run.
- A group whose rows cannot be read shows the error, and its rows are read again when it is next opened.
- The rows of a send's group can hold another send or call, which folds in the same way.
- A send or a call to a run in another client's tree is a link to its page, which shows only its status.

In the house example, the desk at `/house/desk` sends `knock` and then `leave` to the room at `/house/room#1`, and the trip at `/lab/trip` calls the weather at `/lab/weather`. On the desk's page:

1. `⇢ sent knock to /house/room#1 · started room` unfolds to the room's rows `started by` and `⇠ knock from /house/desk`.
2. `⇢ sent leave to /house/room#1 · room` unfolds to `⇠ leave from /house/desk` and the room entering `closed`.

On the trip's page, `⇢ called /lab/weather · weather` unfolds to every row of the weather run: `started by`, then `stage look done`.

### Selecting a row

One row is selected. On a running run it is the newest row, or the first row of the folded group it is in, until the person selects another; on a failed run it is the deepest row that failed, and on any other ended run the last. While a running run's selection is not the newest row, the head of Steps offers Follow, which selects the newest row and follows each new row until the person selects another. Folding the group of the selected row selects the group's first row. Rows of a send's or a call's group are never the newest row.

The keys act when the list has the focus, or nothing else on the page has it:

```
↑ ↓      selects the row above or below, of those shown
←        folds the group the row starts, or the group it is in
→        opens the group the row starts
Space    plays or stops
click    selects the row and shows its details
```

Play, in the head of Steps, selects each row shown after the selected one in turn, a row a second, and the drawing follows. A second press plays three rows a second, and a third stops it; selecting a row, Follow, or the last row stops it too.

Steps and the drawing point at each other:

- Holding the pointer over a row lights the cards of the drawing it is about: a state entered or left, a stage, the job that ended or failed.
- A click on a card that is no link selects the newest row about it.
- Where the run links to its machine's file, a card's name links to the line of the file that writes it, on the Machines page.

The drawing shows the selected row's run as it was at the row's step, from its record up to that step and the state that step left (see Drawing). It is the page's run, with the card of each job that started a run on the way to the selected row's run unfolded, that run drawn inside it:

```
the page's run      as of its latest step at or before the row
each run below it   as of its latest step at or before the row, inside the card of the job that started it
the row's run       as of the row's step
```

A row of a send's or a call's group is of a run the page's run did not start, and its run is drawn alone. The drawing's view moves to the cards the step marks, the deepest first, so following rows into a child moves the view into its card, and back out (see Drawing). Its head names the selected row's run and its machine's file, as the title line does.

- The UI sends the drawing and the details of the newest row as the run's page follows the run, so the page asks for nothing while it follows.
- The page asks the UI for a row the person selects, and keeps each answer while the page is open: a row's drawing and details as of its step do not change.
- A selection is shown at once. While its answer is on its way, the drawing before it stays, dimmed after a moment.

### The machine's file

The file links to the Machines page, opened on the machine of a run in a tree of the UI's client:

```
a run at its own location   the file at that location: devices/boiler.yaml
a child                     its root's file, unfolded down the cards of the jobs that started each child: schedules/away.yaml › pause
```

- The Machines page shows the file as it is on disk now, which may differ from the definition the run started with. The run's drawing is the definition it runs, and the link's tooltip says so. Where they differ, the line under the title says so.
- There is no link where the UI offers no Machines page, where the run's tree was submitted from another directory or at another prefix than the page reads, where a machine on the way was made while running, and on a run in another client's tree.

The details of a row have four tabs, and Open opens the row's run's page:

```
What happened   the row's facts: the job and its machine or script, the toolkit a tool is from (the `toolkit` Corbie shows on the job), a tool's arguments, the run a tool is held for, the turn, the error, the address sent to and the run that took it
Input           what the row's job, call or event was given
Output          what it gave back, and what the row's job logged, each value with its time
Record          the values the run's jobs emitted since the step before, and the row's step, in the words corbie run show --record prints, each named by its run's machine
```

### Waiting on, Output and Children

- What the run waits on is listed as in the list, with each running child linked and each ticket's value, and its labels below.
- A job going gives its runner, its attempt after the first, when it was sent and how long ago, when its result is due, when it restarts and how many times it has, and what it has logged so far.
- A run with a timeout of its own says when it times out.
- Output shows the output, or the error; a denial shows what was refused, the layer that refused it, and the fix. The files of its result are listed below, each a download, as `corbie run show --save` fetches it, and never shown in a page.
- Children lists the runs it started, with each one's status, name, machine where that is not its name, and when it started or ended.

A run in another client's tree shows its status, its path, its children and its labels, and nothing from its record. Its jobs and timers are counted and its tickets named.

## Acting on a run

```
Reply    answers a ticket, by its value, with an answer   corbie run reply <id> <ticket> <answer>
Send     sends the run an event, with data                corbie run send <id> <event> <data>
Cancel   cancels the run                                  corbie run cancel <id>
```

With a client among the UI's parties, Send is on the page of every running statechart. The run's `receive:` decides whether a run in another client's tree takes it, and the receipt says.

On a run in a tree of the UI's client, each ticket's value is filled in. On any other running run, Reply takes a ticket's value typed in, as a client that was handed one answers it. An answer and an event's data are typed as JSON.

Cancel is on every running run in a tree of the UI's client. With the controller among the UI's parties, it is on every running run's page, and on a run in another client's tree it is `corbie controller cancel <id>`.

The page shows the answer the command prints. For Start it is the ID of the run it started. For Reply, Send and Cancel it is the receipt: taken, or refused with the reason, for an event whether the machine acted on it, and for a send the run that took it. `corbie controller cancel` does not wait for a receipt, and the page says the cancel is in the controller's inbox.
