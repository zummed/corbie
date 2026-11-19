# Machines that go wrong

Each machine here goes wrong on purpose, so the Runs pages can be tried on it. A controller and a runner must be running, as `corbie manual one-computer` sets them up:

    corbie init --ceiling all
    corbie start controller runner ui &

Submit the directory, start each machine, and open the address `corbie start` printed:

    corbie run submit runs/testdata/trouble --at /trouble
    corbie run start /trouble/fails
    corbie run start /trouble/denied
    corbie run start /trouble/stuck
    corbie run start /trouble/slow-child
    corbie run start /trouble/unruled
    corbie run start /trouble/logs
    corbie run start /trouble/slow

`corbie run start` prints each run's ID. Send the unruled run an event it has no rule for, by that ID; `corbie run ls` lists the IDs too:

    corbie run send <the unruled run's ID> poke

| File | What goes wrong | What its run's page shows |
|---|---|---|
| `fails.yaml` | the stage `check` throws an error | the line under the title gives the stage and the error; Steps opens on `stage check failed` |
| `denied.yaml` | the stage `read` reads a file with no `allow:` for it | what was refused, the layer that refused it, and the fix |
| `stuck.yaml` | the run waits on its ticket `verdict` | `waiting on ticket verdict`; Waiting on gives the ticket's value and when the run times out |
| `slow-child.yaml` | the stage `wait` starts `stuck.yaml` as a child, which passes the stage's timeout of 1s | the stage, its timeout, and the child that timed out |
| `unruled.yaml` | the event `poke` arrives in the state `resting`, which has no rule for it | `waiting on an event in resting`, and the row `⇠ poke from client:… · no rule in resting` |
| `logs.yaml` | the stage `count` logs a line and its progress five times as it runs | the row `stage count done · …s`, and its details' Output gives what it logged |
| `slow.yaml` | the stage `work` fails at its first attempt, then works for a minute, logging each second | Waiting on gives its runner, attempt 2, when it was sent and is due, and its logs as they grow |

On any of them, hold the pointer over a row to light its cards in the drawing, click a card to select its newest row, and press Play or Space to step through the rows.
