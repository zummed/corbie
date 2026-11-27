# Pages

Every page of the UI shares a header, a look, and a way of following the store. This area is those.

## The header

The header is at the top of every page. It holds Corbie's logo, the pages the UI offers, the parties it acts as, and a dot that pulses while the page follows the store and turns rowan when the store cannot be read.

```
Runs         the list of runs, and each run's page
Machines     the machines the controller recorded, when the UI acts as the controller
Controller   runners, clients and secrets, when the UI acts as the controller
Manual       Corbie's manual, as corbie manual web serves it, in Corbie's own look
```

## The look

The UI is a dashboard. It uses the whole window, with a 20px margin and no column of fixed width. The page fills the window under the header: the Machines page and a run's page fit in it, and their panels scroll inside.

- Data is set in IBM Plex Mono at 13px, secondary data at 11.5px, and labels and buttons in IBM Plex Sans. Both are in the binary, and no page of the UI fetches anything from another site.
- A page carries labels and data. It writes a sentence only where it cannot show a thing, or where a person must act to fix one.
- A status is a dot and a word, and a failure adds its kind in brackets: `failed (denied)`. The dot is petrol for running, moss for done and rowan for failed.
- A time is its time of day when it is today, and has its date on any other day: `20:29`, `10 Nov 20:29`.
- A machine's type is an icon beside its name, drawn as its drawing draws the type: stages for a flow, rounded states for a statechart, and the model's star for an agent.
- The status colours mark status: rowan also marks the buttons that cancel or remove, and petrol what a run is doing now. A blue marks the field with the keyboard, a link under the pointer, and the keys of a machine's text.
- Corbie's gradient, from petrol through blue to violet, colours the icons of the types. No panel has a gradient or a shadow.
- A page follows the computer's light or dark setting, with the manual's colours.

Each page is built from these:

```
title line   one line, 46px, at the top of a page or a panel: icon, name, dim facts cut short with "…", buttons at the right
panel        the plate colour with a thin border, corners of 10px
list         one line for each thing, 31px high, with a thin rule between lines, cut short with "…", and an optional dim second line
tabs         an underlined bar of names, to choose a view
handle       the gap between two panels, which resizes them when dragged
pills        a small group of choices, such as a time window
button       30px high, corners of 6px; one that starts or adds something is filled in ink, one that cancels or removes has rowan text
label        a section's name, in capitals
```

A handle is between Steps and the drawing on a run's page, between the drawing and the details, and between the list and the view on the Machines page. The browser keeps each size in its local storage, for every page of the UI.

## Commands

Every button that changes the store runs one of Corbie's commands. Holding the pointer over it, or moving to it with the keyboard, shows the command:

```
Cancel    $ corbie run cancel laptop-client/20261015T091204.118Z-3be1c07a2f
```

## Following the store

A page is drawn in the browser by its script, with Solid, from its parts. A part is the data of one thing on the page that changes, such as the busy runs or a run's Steps, sent as JSON. Each page's parts are one Go type, whose fields are its parts.

1. The page carries every part as it is when it loads, and its script draws them.
2. The script opens a WebSocket to the UI.
3. The UI sends a message for each part that changed, and when the store is lost or read again.
4. The script draws the part again, and keeps what a person typed, unfolded and chose.

```
{"type": "part", "id": "busy", "data": {...}}    a part, as it is now
{"type": "store", "store": "lost"}               the store cannot be read; "live" when it can again
```

`types.d.ts` gives the type of every message and part, written from the Go types by the tests. The tests of each page draw it from the parts Go sends, so a part whose shape changes fails them. A part is drawn by one component, on the first load and on every change. When the connection is cut, the page opens it again and is sent every part as it is then. A browser holds no more than six requests open to one address, and a WebSocket is not one of them, so any number of pages can be open.

## Motion

Motion shows a change, and nothing else moves.

```
a run that is running         its dot pulses, every 2.4s
a line that appears           it fades in, lit in blue that fades over 1.6s
a line that changes           it is lit for 1.2s
a run that ends               its line is lit in its status's colour, and moves to the ended runs
a row of Steps that appears   it slides in from below, in 0.35s
the state a run is in         its outline pulses, every 2.6s
a count that changes          it lifts and settles, in 0.5s
another machine on Machines     the views cross-fade in place, in 0.18s
another page                  the page shown stays until the next is drawn
```

With the computer's setting for reduced motion, nothing moves.

## Errors

When an action fails, the error its command printed is shown beside its button, untouched. A page that cannot read the store keeps what it shows, turns the header's dot rowan, and reads again at the interval.
