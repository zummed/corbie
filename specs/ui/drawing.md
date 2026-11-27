# Drawing

A machine is drawn top to bottom from its definition in full form, and a run's state is shown on the drawing.

1. The UI reads the machine's graph, with Corbie's own types for a flow, a statechart, an agent, a decision and a toolkit: each card and what it says, the arrows, and the marks of a run.
2. The page lays the graph out and draws it as SVG, with Solid, at the size it needs.

Each holder, card, arrow and chip is an element of its own, and a card keeps its element while the graph changes. The same drawing is on a run's page and on the Machines page.

## The camera

A drawing fills its panel and is seen through a camera, which frames one part of it and scales it to fill the panel:

```
a card the page focuses   the machine unfolded inside the card, or the card       up to 1.6 times its size
the cards a run marks     the innermost machine they are in: a child's in its card  up to 1.6 times its size
otherwise                 the whole machine, from its top                           up to its size
```

Following a run into a child therefore zooms in to the child's machine, and back to the parent's step zooms out to the whole parent. Where the machine framed cannot be read at half its size or more, the camera frames the cards the run marks at half size.

When the graph or the focus changes, the camera glides to frame it again, in 0.35s. A person moves it by hand:

```
wheel            pans
ctrl-wheel       zooms round the pointer, as a pinch does
drag             pans, and a drag that began on a link does not follow it
+ - 0            zoom in, zoom out, and fit the whole machine
buttons          zoom out, zoom in, and Fit, in the panel's corner
```

Once a person moves the camera, it stays where they put it, and the corner offers Follow, which frames what the drawing is about again.

When the graph changes, every card, holder and chip that stays glides to its new place and size in 0.35s, an arrow glides along with them, and a machine unfolded inside a card fades in. A child drawn inside its card therefore reads as the card opening. With the computer's setting for reduced motion, nothing glides.

The pointer on a card lights its arrows and their chips, and the pointer on an arrow or a chip lights the two cards it joins. A page can light cards the same way, by their paths.

Every card has one width and one corner radius: an icon for what its job is, its name, and one line for each fact about the job, such as `script: ./find.js` or `flow: people-find`, with `approve: true` where the job has it. A holder is drawn round cards: a statechart's state that holds states, an agent's tools, and each of its toolkits.

A card's tooltip gives its `when:`, a tool's `description:`, and the job's limits where the definition sets them:

```
timeout: 5s
restart: { after: failure, limit: 2 in 1h }
tags: gpu
allow: network, send:devices/boiler
```

Where a page gives lines an address, a card's name links to the line of the first value written under it, such as `support.yaml:14` for a stage whose key is on line 13, in the file most of its values are in, and its tooltip gives that file and line; the rest of a card that is a link follows its own link. A card a page can pick, which is no link, is picked by a click or by Enter. A page can give a card a count, such as how many runs are in that state now, which is drawn at the card's top right.

A card whose job names a machine shows the machine's type as its icon and `type: name` as its line. The Machines page knows the machine from the definition recorded at its location, and a run's page from the definition the run was started from. A machine known by neither is written as the definition names it, `machine: ./find.yaml`.

A job that sends or calls is a card with these lines:

```
a job that sends   send: set, to: devices/boiler
a job that calls   call: services/weather, and on a run's page the called run's type: name
```

A send's card has a statechart's icon, since only a statechart takes an event. On a run's page a call's card links to the run it called. On the Machines page it links to the machine recorded at the location it calls, where the location is written out and a machine is recorded there.

An arrow goes straight down. Arrows from one card run along one line, so a branch is one line that divides; in a flow and an agent, arrows into one row run along a rail above the row. A drawing has as few arrows as say what it does.

## A flow

- A stage is a card.
- An arrow goes to a stage from each stage it waits for: each stage an expression of it reads, whether its output, its error, whether it failed or was skipped, or its emitted items; each stage it names in `follows:`; and the stage it names in `each: { emitted: … }`. No arrow is drawn from a stage it waits for through another.
- An arrow's tooltip says why the stage waits: `reads find.output`, `follows: find`, `each: { emitted: find }`.
- Stages are placed top to bottom in the order the arrows give, and stages that run at the same time are placed side by side.
- A stage with `each:` is drawn as a stack of cards, and a stage with `when:` has a dashed border.

Which stage waits for which is the flow package's rule, which it keeps to itself. The UI applies the same rule to the same expressions, read with Corbie's `machine.Reads`, `machine.ReadsIn` and `machine.ReadsOf`.

## A statechart

- A state is a card, and a state that holds states is a holder drawn around them. A state with no job has no icon.
- States are placed top to bottom by how many transitions each is from the state its level starts in. In a row, states joined by transitions are placed side by side, so that a chain reads left to right.
- States in different holders are in the rows of the states that hold them, at the level that holds both.
- The state each level starts in has a short arrow into it.
- A final state is a narrow card with a double border, drawn in rowan for `end: failed`.
- An arrow goes from a state to each state its `on:` names in a row below or in its own row, one arrow for all the rules between the two, with their events and conditions as written on a chip, shortened to fit, and whole in its tooltip: `verdict, when event.data.ok`. A timer's arrow is dashed and its chip gives its time: `too_long, 48h`.
- A rule to a state in a row above, or to the same state, has no arrow. It is written in the card: `done, failed ↩ waiting`, or `done ↻`.
- A state's ticket and timers are written in its card.
- Each arrow has a line of its own, and its chip is on that line, clear of every other chip and card, and 12 pixels from every other line. The arrows from one state share the line they leave it by.
- An arrow to a row below runs along a track in the gap above that row. Tracks that would overlap are drawn one below another.
- An arrow to the state beside it runs straight across, one line each way. An arrow to a state further along the row runs on a track below the row and up into the state.
- An arrow leaving a holder from a state with states below it runs down the side of the holder. An arrow into a state with states above it runs down the holder's right side.
- A `send:` to an address is a card of its own, below the state, named by the address, such as `rooms/thermostat#kitchen`, with a dashed arrow whose chip gives the event.

## An agent

An agent is three things, one below another, joined by two arrows.

1. The model is a card, holding the model's name.
2. Its entries are in one holder: each is a card, in the order of their names, six to a column, and each of the agent's hooks is a card below them, named as the hook is, with a dashed border where it has a `when:`. The chip on the arrow into the holder gives how many turns the agent has.
3. `reply` is a narrow card.

- An entry with `when:` has a dashed border, and its tooltip gives the condition: `when: input.asker.scopes.includes('orders:read')`.
- An entry written `machine:` or `call:` is the card of a job that runs or calls a machine: `machine: ./toolkits/net.yaml`, `call: people/find`. One that names a toolkit has the toolkit's icon, and its tooltip gives the toolkit's `description:`.
- On a run, an entry that names a toolkit is marked by the jobs the run asked for through it: its opens and the calls of its tools.
- Tools that are Corbie's own, whose script begins `corbie:`, are not drawn.

## A decision

A decision is two things, one below the other, joined by one arrow: its model, by its name, and its answers, with the name of each question.

## A toolkit

A toolkit is three things, one below another, joined by two arrows.

1. `request` is a narrow card.
2. Its tools are in one holder: each tool is a card, in the order of their names, six to a column, and each of the toolkit's hooks is a card below them, as an agent's are. A toolkit with `job:` has one card, `job`.
3. `answer` is a narrow card.

A tool's tooltip gives its `description:`.

## A machine inside a machine

On a run's page, a card whose job started or called a run links to that run's page. The page can give the drawing of that run as of a step, which is drawn inside the card with that run's own marks. A run followed into its children is then a camera moving into the card, and out again.

On the Machines page, a card whose job names a machine is a link. The link unfolds that machine's drawing inside the card, and the card then has the link that folds it again. Any number can be unfolded at once, at any depth, and the address names them, so a person can send the address to another.

A job whose machine is an expression is known only when it runs, and has no link.

## A run on the drawing

Each step of the record gives the run's state after it, in one shape for every type, and the drawing marks it. A run is therefore drawn only when the UI may read its record.

A run is drawn as of one step: the step selected in Steps. Its marks come from its record up to that step, and the jobs going and how the run stood come from the state that step left, so a later step changes nothing on it.

```
flow         each stage's status: waiting, running, held for a hook, done, failed or skipped
statechart   the states it is in, and whether a state's job is held for a hook; states it was in before are outlined in moss, and states never entered faintly
decision     the model going now while its job runs
toolkit      the tool or the job the request started, going now, held for a hook, or done; each hook going now; the other tools as never used
agent        the turn it is on; the model going now while its turn runs; each tool and each hook going now, a tool held for a hook, used at an earlier step, or never used
```

A run that ended is marked by how it ended. A stage still running when it ended is outlined in rowan where the run failed, and grey where it was cancelled. In a failed run, the states it ended in are outlined in rowan, and so is the card of the job whose failure ended it, such as the model of an agent whose turn failed.

A statechart's run is followed through its steps: from the states it was in before a step, by the events the step took, to the states it is in after. A state it passed through within one step was entered, such as the state it started in and left at its first step. Where an event has rules to several states, the rule taken is the one to a state the run is in after the step.

The transition the step drawn took is drawn in petrol, its chip's words too, heavier than the other arrows and over any arrow it shares a line with, or written in petrol in its card. Its arrow is drawn along its line from the state it leaves, in 0.7s, so a person sees it taken. A card's mark changes colour over 0.4s. The states a run that did not fail ended in are outlined in moss, as states it was in.

A flow's stage and a statechart's state whose job waits for a hook are outlined in amber, as held. An agent's hook is going while a tool waits for it: the tool is outlined in amber, as held, and the hook as going now. The run a hook's job started is linked from the hook's card.
