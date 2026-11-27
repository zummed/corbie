# Serving

`corbie ui` serves the UI to a browser until it is stopped. This area is how it is started, what it acts as, and who may use it.

## Starting it

The UI acts as the parties named after the command, each from the config that `--config` names, as for every command. With nothing named, it acts as every party the config holds; `none` names no party, and beside a party it is refused. It runs no role: `corbie ui controller` uses the controller's keys, and the controller itself runs with `corbie start controller`.

```
corbie ui                      acts as every party the config holds
corbie ui none                 acts as no party
corbie ui client               acts as the config's client
corbie ui controller           acts as the config's controller
corbie ui client controller    acts as both; the client must trust the config's controller
```

`corbie start controller runner ui` starts it beside the roles, with `CORBIE_CONFIG` and `CORBIE_STORE` in its environment as `corbie start` was given `--config` and `--store`.

Acting as no party, it is a viewer, which needs no identity: only the controller's card and the store's address. A config holds them after `corbie client trust`, or where the controller is in it.

- A store another computer serves has no viewers: a UI there is started as the client.
- On a bucket, the key in the environment, not the config, says what the UI can read.

```
--listen host:port    where it listens; default 127.0.0.1:8373, or a free port when that one is in use
--token-file <file>   a file holding the token every request must show; by default it makes one and prints it
--interval <time>     how often it reads the store again; default 1s, or 10s for a bucket
```

It is a command in Corbie's table, so its help, mistakes and exit statuses are Corbie's: `--help` and `-h` print its help, a mistake prints its usage line and ends with status 2, and `--json` is a mistake. It takes `--config` and `--store` as every command does, and prints where it is, with the token when it made one:

```
$ corbie ui
ui for laptop-client and laptop-controller, at http://127.0.0.1:8373/?token=6f1e0c…
```

A UI listens on this computer only, and refuses a `--listen` address of any other; `none` makes it a viewer of runs alone, which may listen on any address. On a serverless host it refuses to start, as the gateway does: it needs a process that stays up.

## What each party opens

```
no party        Runs, with every run's status; /metrics
client          each of its own runs in full, with Reply, Send and Cancel; Reply on any run, with a ticket's value
controller      Machines, to read; Controller, with its parties, secrets and values; cancelling any run
```

The UI needs the store, and refuses to start where it cannot open it.

A page or an action the UI's parties may not use is not offered, and a request for one is refused. What the store lets the parties read limits the rest.

An action runs its command in the UI's process with `--json`, and with `--config` and `--store` as the UI was given them, as `corbie mcp` runs a command. The page shows the command's answer, or its error, as the command gave it.

- A value from a page that begins with `-` is refused before any command runs, since the command line reads it as a flag, as it does when a person types it.
- While a command waits for its receipt, the page shows it waiting. Leaving the page stops the wait, and the message stays sent.

## The token

Every request shows the token, and a request without it is refused and does nothing.

```
a browser     once, in the address it was given, then a cookie named for the port in its place
Prometheus    as Authorization: Bearer <token>
```

A request that a page of another origin makes is refused, whatever site it says it is from. A browser sends the cookie to every server on this computer, whatever its port.

Anyone with the token can do what the UI's parties can do: with a client, reply to, send to and cancel its runs, and with the controller, change what it trusts. A UI that a team shares is therefore started as no party, with `corbie ui none`.

A UI started again without `--token-file` makes a new token. A page open from before turns its dot rowan, and loading it again is refused.

## What it keeps

The UI keeps nothing on disk. It reads the list of runs at every interval, once for every page and scraper it serves, and a run that has left the store has left the list. Each open run's page follows that run's record.

Reading the list reads every kept run's status: on a bucket, that is one request for each run at each interval.
