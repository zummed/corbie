# Corbie

Corbie runs machines (flows, statecharts and agents) whose work is done by JavaScript jobs on runners. Every party reads and writes one store directly, and nothing important is held in memory.

It is one Go module, one package per area of the design, and the binary in `cmd/corbie`.

## Where to start

- `specs/README.md`: the whole system in a page, with one drawing, and the map of areas.
- `specs/<area>.md`: the overview of an area, in prose. These are the authority, and what the author reads.
- `specs/rules/<area>.md`: the requirements (`R-…`) and constraints (`C-…`) behind an overview. Code is built and tested against these.
- `parity/machines/`: machines alone and inside one another, as cases, each with how its run should end. A rule names the case that is its check.

## What the code is for

In this order:

1. Simple. Drop anything that can be dropped.
2. One path to do each thing. Flag anything that adds a second.
3. Obvious logic. Prefer the plain way to the clever one.
4. Simple error handling. The layer that notices passes the error up untouched; one outermost place per step, job or command reports everything that went wrong together. Nothing in between catches, retries or rewords.
5. Data flowing through functions, where that is sensible: a step is a function of its inputs.

And these hold everywhere:

- The core holds only generic mechanisms. Models are not special: a model call is a plugin and its key is a secret.
- Nothing important lives in memory. Everything is in the store, and any process can stop at any moment.
- Security rests on a few rules that can be said in a sentence. Do not add a check where there is no real boundary, and do not add fiddly mechanism for a special case. What Corbie does not defend against is said plainly in `specs/permission.md`.
- If a normal person would need a special config, package or key to set something up, the design is wrong.
- Corbie's ideals come before any one application's convenience.

## Working on the specs

- An overview describes how things are now. No history, no "previously", no alternatives. Something removed is simply gone.
- A rule earns its place. It states a behaviour that a definition, a script or a client relies on, or it guards a secret, a record or the owner's computer, or it settles that there is one way. Wording, layout, small edges and what another rule already says are left to the overview and the tests.
- A rule says nothing its overview does not. If a behaviour is needed and no overview gives it, the overview changes first, and only after the author agrees.
- A behaviour change updates its overview and its rules in the same commit as the code.
- Rule IDs are stable and never reused. A cut rule's number disappears.
- The author thinks visually. Worked examples and concrete runs help more than abstract description. Write as the next section says.

## Writing

These rules hold for everything a reader sees: the manual, the specs, the README, what Corbie prints, and reports to the author.

Words:

- Use the precise technical word. The developer's word is the plain word: process, file, directory, S3 bucket, network connection, permission, environment variable, default, timeout, poll, encrypted, signed.
- Name the real thing: the field (`allow:`, `job:`), the command, the status (`denied`), the path.
- No metaphor and no soft verbs. Not "decides", "reaches", "touches", "meets", "holds on to", "is owed", "stands", "says so". Not "unless it says": write "by default".
- One word for one thing, the same in the specs, the code, the manual and what is printed. A word with two meanings is a fault, and so is a second word for the same thing. A word is changed everywhere or not at all.
- These words are settled:
  - **machine**: a YAML definition of type flow, statechart or agent. Never a computer.
  - **job**: the `job:` block of a stage, a state, a tool or an ask. A job is a script, which a runner executes, or another machine, which the controller starts as a child run.
  - **run**: one execution of a machine, which a client starts, lists and shows.
  - **directory**, never "folder". **computer**, never "machine".
  - **controller**, **runner**, **client**: the three kinds of process. **store**: the directory or bucket they share.
- The manual does not use words that only the code needs: pass, wake, mark, head, receipt, party. It says what happens instead.

Sentences:

- Few words. Every sentence states a fact the reader needs. If a sentence can be deleted with nothing lost, delete it. Say a thing once.
- Cut every word that carries no fact: "also", "that is", "which is", "in order to", a subject said again, a reason the reader does not need. After writing a paragraph, cut it by a third and check that no fact went.
- A paragraph is at most three sentences. Facts of the same kind go in a list, not in a paragraph.
- Choose the list by what its items are. A term and a few words about it (a field, a flag, a command, a path) is a row of two columns. Steps in order are a numbered list. Anything else is a dash list, and each item of it is a whole sentence with a subject and a verb: "When the script sends the handle to another host, Corbie refuses", never "Sent to another host: Corbie refuses". Do not set sentences in two columns.
- For example, this paragraph:

  "The bucket calls a function when a file is written to that function's inbox directory: the controller's function for the controller's inbox, and a runner's function for that runner's inbox. The function processes everything that is waiting and returns. A schedule also calls the controller's function once a minute."

  is written as:

  ```
    a file written to the controller's inbox   the controller's function
    a file written to a runner's inbox         that runner's function
    a schedule, once a minute                  the controller's function, for timers

  A function handles everything that is waiting, then exits.
  ```
- Say who does what: "The controller writes the step, then sends the job."
- Whole sentences. No slogans, no two-part titles, no comma fragments. A title is a short name.
- A row in a list is one line. A fact added to a row is a few words in that line, not another sentence.
- Never claim more than is true. Check a statement against the code, or run it.
- No diagram drawn in text: no boxes, lines or arrows made of characters. They confuse the author every time. Say the same thing as a short table or a numbered list of one-line steps, and do not let the replacement grow longer than the diagram was.

A page of the manual:

- It answers three things: what the thing is, why a reader wants it, and how to do it.
- Its first line is `Title: one sentence that says what the page is for.`
- It gives the definition first, then a real example: the real files, the real command and the output Corbie printed. Every example is run as a test.
- A mechanism is stated first, in three or four numbered one-line steps, before any example. A simple mechanism must read as simple: one example, and the detail left to the reference page. `manual/pages/approval.txt` is the model.
- Headings are short and say what the section does. No numbered captions at the top of a page.
- Another page is named as `corbie manual <page>`. A page ends with what to read next, or with a `Reference` list of pages.
- A reader should come away with Corbie's few ideas and how they fit: a machine, its jobs, permissions, the three processes and the store, and state kept in the store. `manual/pages/corbie.txt` states them, `manual/pages/first-machine.txt` is the model for a page with an example, and `manual/pages/how-a-run-works.txt` is the model for how few words a page needs.
- Do not add a page or a figure to fill a gap without asking the author. The aim is fewer and shorter pages.

## Working on the code

- Run Go through mise, which pins the version: `mise exec -- go …`. `mise run check` vets and tests.
- Pure Go, no cgo. Tests need no network, no container, no key and no model.
- One package per area, named as the area is. An area uses another only as its rules' `Interfaces` section says.
- Each constraint names its test. Write the test under that name.
- No references to the specs in code comments. The link between a rule and the code is the commit message.
- Match what is there: the same words the specs use for the same things.

## Commits

- Straight to `main`.
- The message starts with the area, and names the rule when one applies: `store: listing reads to the end (C-store-02)`.
- Refactors and tooling need only the area.
