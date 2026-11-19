# Machines that go wrong

Each machine here goes wrong on purpose, so the Runs pages can be tried on it.
Submit the directory, start each machine, and open the runs in `corbie ui`.

    corbie run submit runs/testdata/trouble --at /trouble
    corbie run start /trouble/fails
    corbie run start /trouble/denied
    corbie run start /trouble/stuck
    corbie run start /trouble/slow-child
    corbie run start /trouble/unruled
    corbie run send /trouble/unruled poke
    corbie run start /trouble/logs
    corbie ui

| File | What goes wrong |
|---|---|
| `fails.yaml` | the stage `check` throws an error, and the run fails |
| `denied.yaml` | the stage `read` reads a file with no `allow:` for it, and is denied |
| `stuck.yaml` | the run waits on its ticket `verdict` until its timeout of 1h |
| `slow-child.yaml` | the stage `wait` starts `stuck.yaml` as a child, which passes the stage's timeout of 1s |
| `unruled.yaml` | the event `poke` arrives in the state `resting`, which has no rule for it |
| `logs.yaml` | the stage `count` logs a line and its progress five times as it runs |
