# Parity cases

Each directory of `machines/` is a case: a machine, alone or with machines inside it, and how its run should end. `parity_test.go` runs every case, with every role in one process and the store in memory.

    mise exec -- go test ./parity/
    mise exec -- go test ./parity/ -v -run 'TestParity/36'

There are 104 cases. Cases 36 and 92 are written a second way, in a directory inside the case, so the test has 106 runs.

## What is in a case

| File | What it is |
|---|---|
| `root.yaml` | the definition that is started; other `.yaml` files are the machines it runs |
| `*.js` | the scripts |
| `input.json` | the run's input |
| `expect.json` | how the run should end and what its record should hold; a `note` names the rule the case checks |
| `drive.json` | what a client sends while the case runs: an answer or a cancel |

The top of `parity_test.go` describes each field of `expect.json` and `drive.json`.

The agent cases call no model. Each agent's `turn:` is a script in the case, which gives the next reply from a fixed list.
