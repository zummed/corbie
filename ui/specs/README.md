# Corbie UI

The UI shows machines and their runs in a browser, and does to runs what Corbie's commands do. It is the program `corbie-ui`, which `corbie ui` runs. It reads a directory of machines and the store through Corbie's packages, acts with Corbie's commands as the parties it is started as, and keeps nothing of its own.

## What it acts as

What the UI shows and does follows from the parties it is started as, and from what the store lets them read.

```
corbie ui                      the machines in a directory, and every party the config holds
corbie ui none                 no party: the status of every run, and /metrics
corbie ui client               the config's client: its runs in full, and the machines in a directory
corbie ui client controller    the controller as well: its runners, clients, trees, secrets and values
```

Three uses follow from this.

- A person with a directory of machines and no store runs `corbie ui` there, and reads the machines.
- A UI a team shares is started as no party, with `corbie ui none` on the controller's computer or with a viewer's key to the bucket. Prometheus reads its `/metrics`, and Grafana shows what Prometheus keeps.
- A person's own UI is started as their client, and as the controller where they own it, on their own computer.

## How it holds together

The UI is a client of Corbie and adds nothing to Corbie. Every action on the store runs one of Corbie's commands in the UI's process, as `corbie mcp` runs one, and the page shows the command and what it answered. The command line, a model through `corbie mcp` and the UI can therefore do the same things to the store, and the UI can do nothing more.

Pages read the store through Corbie's packages. The UI reads the list of runs at every interval, once for every page and scraper it serves, and each open run's page follows that run's record. It keeps nothing on disk, and what it holds in memory it has read from the store, so any number of copies can run against one store, and any of them can be stopped at any moment.

A page is drawn in the browser by Solid, from the data of its parts. It shows what changes as it changes: the UI sends each page the data of the parts that changed.

## The program

This repository builds the program `corbie-ui`. `corbie ui` runs it from PATH, and `corbie start controller runner ui` starts it beside the roles, with `CORBIE_CONFIG` and `CORBIE_STORE` in its environment as `corbie start` was given `--config` and `--store`.

The program links Corbie's packages, and carries the machine types and the plugins, as Corbie's does. It carries Solid 2.0 for the browser, with no build step.

Corbie itself is not changed, so the UI repeats what Corbie keeps to itself. Each goes when Corbie gives it out.

- Corbie's command line opens a config's parties to read the store.
- An `include:` names a file from the file that writes it when it begins `./`, and from the top of the tree otherwise.
- `corbie run show` prints the words of a run and of its record.
- The flow package has the rule by which a stage waits for others (see Drawing).
- The names of the secrets a machine written out uses, which Corbie reads for a submit's reminder.
- The controller's record of a tree: its client, the directory last submitted there, and each location's lifetime.
- The name of the controller's note of a send in the sender's record: the send's message, then `.note` where it was not delivered, or `.moved.` and the end of the run's ID where its event went to another run.

## Reading these specs

Each area below has an overview here, and the overviews are the authority. Its rules are in `rules/`, under the same headings, and say nothing the overview does not. Each area is one package of the code, and Corbie's own specs are at `../corbie/specs/`.

## Areas

**[Serving](serve.md).** `corbie ui`: the parties it acts as, its address and its token, and which pages each party opens.

**[Pages](page.md).** What every page shares: the header, the look, motion, and how a page follows the store.

**[Runs](runs.md).** The runs in progress, one line for each root run and the runs under it, and one run: its steps, its drawing as of each step, what it waits on, and what can be sent to it.

**[Machines](machines.md).** The machines in a directory, read as a tree, and its files that are no machine, read only: the list, the problems Corbie finds in them, and each machine's description, location, lifetime, who may send to it, the secrets and values it uses, its drawing, its text and its definition written out. The toolkits in the directory, and what offers and starts each.

**[Drawing](drawing.md).** How a flow, a statechart, an agent and a decision are drawn, and how a run's state is shown on the drawing.

**[Controller](controller.md).** The runners and clients the controller trusts, its trees, and the secrets and values set at each location.

**[Metrics](metrics.md).** The counts Prometheus reads, from the status of every run.
