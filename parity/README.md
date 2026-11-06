# Parity cases

Each directory of `machines/` is a case: a machine, alone or with machines inside it, and how its run should end. `parity_test.go` runs every case, with every role in one process and the store in memory.

    mise exec -- go test ./parity/
    mise exec -- go test ./parity/ -v -run 'TestParity/36'

There are 134 cases. Cases 36 and 92 are written a second way, in a directory inside the case, so the test has 136 runs.

Each case's directory is submitted as a tree at `/case`, and the run checked is started at `/case/root`, the location of `root.yaml`, with `input.json`. A singleton's run is the one the submit starts.

## What is in a case

| File | What it is |
|---|---|
| `root.yaml` | the machine whose run is checked; other `.yaml` files are the machines it runs |
| `*.js` | the scripts |
| `input.json` | the run's input |
| `expect.json` | how the run should end and what its record should hold; a `note` names the rule the case checks |
| `drive.json` | what a client does while the case runs: an answer, an event or a cancel, another directory submitted, a file removed, a value or a secret set |

The top of `parity_test.go` describes each field of `expect.json` and `drive.json`.

The agent cases call no model. Each agent's `turn:` is a script in the case, which gives the next reply from a fixed list.
