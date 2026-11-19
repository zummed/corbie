# Drawing

A machine is drawn top to bottom from its definition in full form, and a run's state is shown on the drawing.

1. The UI reads the machine's graph, with Corbie's own types for a flow, a statechart, an agent and a decision: each card and what it says, the arrows, and the marks of a run.
2. The page lays the graph out and draws it as SVG, with Solid, at the size it needs.

Each holder, card, arrow and chip is an element of its own, and a card keeps its element while the graph changes.

The Machines page makes a drawing narrower to fit its panel, and never larger; a run's page draws it at its size, in a panel that scrolls. The same drawing is on a run's page and on the Machines page.

Every card has one width and one corner radius: an icon for what its job is, its name, and one line for each fact about the job, such as `script: ./find.js` or `flow: people-find`, with `approve: true` where the job has it. A card's tooltip gives its `when:` and a tool's `description:`. A holder is drawn round cards: a statechart's state that holds states, an agent's tools, and each of its toolkits.

A card whose job names a machine shows the machine's type as its icon and `type: name` as its line. The Machines page knows the machine from its file, and a run's page from the definition the run was started from. A machine known by neither is written as the definition names it, `machine: ./find.yaml`.

A job that sends or calls is a card with these lines:

```
a job that sends   send: set, to: devices/boiler
a job that calls   call: services/weather, and on a run's page the called run's type: name
```

A send's card has a statechart's icon, since only a statechart takes an event. On a run's page a call's card links to the run it called. On the Machines page it links to the file of the machine at the location it calls, where the location is written out and the directory has a machine there.

An arrow goes straight down. Arrows from one card run along one line, so a branch is one line that divides; in a flow and an agent, arrows into one row run along a rail above the row. A drawing has as few arrows as say what it does.

## A flow

- A stage is a card.
- An arrow goes to a stage from each stage it waits for: each stage an expression of it reads, whether its output, its error, whether it failed or was skipped, or its emitted items; each stage it names in `follows:`; and the stage it names in `emits:`. No arrow is drawn from a stage it waits for through another.
- Stages are placed top to bottom in the order the arrows give, and stages that run at the same time are placed side by side.
- A stage with `each:`, `emits:` or `blob:` is drawn as a stack of cards, and a stage with `when:` has a dashed border.

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
2. Its tools are in one holder. Each toolkit is a holder inside it, with its name and its tools. Its tooltip gives its `when:` and the file its tools are written in, or its `description:` where it has no tools, as the definition written out gives each value's file, where that is not the agent's own file. Tools in no toolkit are cards beside the toolkits, and the agent's ask is a card below them. The chip on the arrow into the holder gives how many turns the agent has.
3. `reply` is a narrow card.

- A toolkit or a tool with `when:` has a dashed border, and its tooltip gives the condition: `when: input.asker.scopes.includes('orders:read')`.
- A tool written `call:` is the card of a job that calls: `call: people/find`.
- On a run's page the file the agent's type is written in is taken as the agent's own file.
- Tools that are Corbie's own, whose script begins `corbie:`, are not drawn.

## A decision

A decision is two things, one below the other, joined by one arrow: its model, by its name, and its answers, with the name of each question.

## A machine inside a machine

On a run's page, a card whose job started or called a run links to that run's page.

On the Machines page, a card whose job names a machine is a link. The link unfolds that machine's drawing inside the card, and the card then has the link that folds it again. Any number can be unfolded at once, at any depth, and the address names them, so a person can send the address to another.

A job whose machine is an expression is known only when it runs, and has no link.

## A run on the drawing

Each step of the record gives the run's state after it, in one shape for every type, and the drawing marks it. A run is therefore drawn only when the UI may read its record.

A run is drawn as of one step: the step selected in Steps. Its marks come from its record up to that step, and the jobs going and how the run stood come from the state that step left, so a later step changes nothing on it.

```
flow         each stage's status: waiting, running, held for the ask, done, failed or skipped
statechart   the states it is in, and whether a state's job is held for the ask; states it was in before are outlined in moss, and states never entered faintly
decision     the model going now while its job runs
agent        the turn it is on; the model going now while its turn runs; each tool and the ask going now, held for the ask, used at an earlier step, or never used
```

A run that ended is marked by how it ended. A stage still running when it ended is outlined in rowan where the run failed, and grey where it was cancelled. In a failed run, the states it ended in are outlined in rowan, and so is the card of the job whose failure ended it, such as the model of an agent whose turn failed.

A statechart's run is followed through its steps: from the states it was in before a step, by the events the step took, to the states it is in after. A state it passed through within one step was entered, such as the state it started in and left at its first step. Where an event has rules to several states, the rule taken is the one to a state the run is in after the step.

The transition the step drawn took is drawn in petrol, its chip's words too, heavier than the other arrows and over any arrow it shares a line with, or written in petrol in its card. The states a run that did not fail ended in are outlined in moss, as states it was in.

A toolkit of an agent is folded where the run has not asked for its tools by the step drawn. A folded toolkit is a small holder with its name and how many tools it has, `2 tools`, and its tooltip names them. The folded toolkits are one below another, in one column after the others, so the toolkits the run uses are drawn side by side and in view together.

A flow's stage and a statechart's state whose job waits for approval are outlined in amber, as held. An agent's ask is going while a tool waits for its approval: the tool is outlined in amber, as held, and the ask as going now. The run the ask started is linked from the ask's card.
