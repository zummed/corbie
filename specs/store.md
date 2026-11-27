# Store

The store is the one place Corbie keeps anything. Every party reads and writes it directly. Nothing of Corbie's runs inside it, and nothing of Corbie's stands in front of a directory or a bucket. The one exception is a served directory, described below. Its server keeps nothing of its own with one member, and a log of recent changes with more.

## Named values

The store holds one kind of thing: a value with a name. Names are paths, and the store lists the names under a path in order, each with what a read of it gives for a later only-if-unchanged write, so a reader can tell which values have changed without reading them. Everything else in Corbie, from a message to an instance's head to a job a runner has taken, is a named value used in a particular way.

There are three ways to write a value, and the rest of these specs call them by name.

- **Overwrite** writes the value whatever is there.
- **Only-if-new** writes it only if nothing has that name yet. Finding something already there is an answer, not a failure.
- **Only-if-unchanged** writes it only if the value there is still the one the writer last read.

A value is deleted by the first or the third. A value appears whole or not at all; nobody ever reads half of one.

These three are all the coordination Corbie has. There are no locks, no leases and no transactions across names.

Almost everything is written only-if-new and never changed afterwards. Very few values are ever rewritten: an instance's head, its note of how far a large step has been carried out, a key's file whose run has ended, an event sent to a key that moves on to the run with the key, a tree and the files at its locations when it is submitted again, the controller's list of who it trusts, and the secrets and values set at a location. This makes the store easy to reason about and cheap to watch: a value read once is read again only when a listing shows it has changed.

## Layout

Each party has an area named after it. This is all of it:

```
version                                  the layout's version

<controller>/
  inbox/<sender>/<message>               messages to the controller
  receipts/<sender>/<message>/           what became of each message a client sent
      receipt                            taken or refused
      result                             for a call, the run's status and its output or error
      blobs/<name>                       the blobs a call's output names
  running/<client>/<instance>            heads of instances that are running
  finished/<client>/<instance>           heads of instances that have ended
  records/<client>/<instance>/
      steps/<number>                     written by the controller
      emits/<job number>/<number>        written by the runner doing that job
      blobs/<name>                       written by whoever made the blob
      carried                            how far a large step has been carried out
      tried                              how many times a step has been taken in a process of its own
  wakes/<time>/<instance>/<name>         when an instance is next due attention
  locations/<location>/+/
      tree                               at a tree's prefix: the client that owns the tree, and the directory last submitted there
      file                               the file recorded at the location, written out
      blobs/<name>                       the scripts and machines that file names
      keys/<hash>                        which run a key names, by the SHA-256 of the key; a singleton's is the empty key, a counted transient run's its own ID
      secrets/<name>                     a secret set at the location
      values/<name>                      a value set at the location
  uploads/<client>/<message>/<name>      the files a client sends with a submit
  trust/<party>                          who the controller trusts

<runner>/
  inbox/<sender>/<message>               jobs for the runner
  taken/<due time>/<job>                 one mark for each job it has taken
```

The version is written once, when the store is first set up, and a Corbie that does not know it refuses to use the store. A store whose version is older is refused the same way: nothing is carried over from it, and `corbie init` sets up a new one. An instance's name begins with the client that owns its root's tree and carries the time it was named, so anything that names an instance says where its head and record are and how old it is.

A location's entries are under `+` after its path. No part of a location is empty, `.`, `..` or `+`, and none holds `#`. A part that starts with a dot, as `.work` does, is kept after a `#`, as `locations/#.work/`, so that a directory does not hide it. A tree's entry is at its prefix. Secrets and values may be set at a path above a location as well, and those set at `/` are under `locations/+/`. For example, these commands:

```
corbie run submit house --at /home
corbie controller secret set BOILER_PASSWORD --at /home/devices/boiler
corbie controller secret set ANTHROPIC_KEY
```

write these values, and each file's scripts and machines under `blobs/`:

```
<controller>/locations/home/+/tree
<controller>/locations/home/devices/boiler/+/file
<controller>/locations/home/devices/boiler/+/secrets/BOILER_PASSWORD
<controller>/locations/+/secrets/ANTHROPIC_KEY
```

## Who may touch what

The layout is drawn so that the right to read or write is only ever a matter of paths.

| Party | May write | May read |
|---|---|---|
| Controller | everything in its own area; jobs into runners' inboxes | everything in its own area |
| Runner | results into the controller's inbox; new emits and blobs in records; its own inbox and marks | its own inbox and marks; blobs in records |
| Client | messages into the controller's inbox; its own uploads; new blobs in the records of its own runs | its own receipts; every head; every record, opening only what is sealed to it |
| A viewer given heads only | nothing | every head |

Every party can read the layout's version. A runner or a client adds to a record only-if-new: it can never change or remove what is there. A runner is not limited to the records of the jobs it holds, so a runner can add junk to any record. What it adds is signed by it and tied to a job, and readers ignore an entry from a runner that had no such job.

Blobs are encrypted and their keys travel only in sealed messages, so the right to read a blob's bytes gives nothing without its reference. A record's entries are sealed, so a client opens only those sealed to it.

A record's names say nothing a definition or a person chose:

| Name | Made of |
|---|---|
| `steps/<number>` | the step's number |
| `emits/<job number>/<number>` | the number the controller gives each attempt at a script when it asks for it, and the entry's number |
| `blobs/<name>` | the time the blob was made and random characters |

The job's name is inside its sealed entries. A client that lists a record learns that the run exists and how many entries its record has, with their sizes and times.

## Kinds of store

A store is a directory, a directory served to other computers, a bucket or memory, chosen by its address.

A **directory** is for one computer. Every value is a plain file, so a person can look at an instance with ordinary tools. A listing reads every file it lists. A change the directory has answered is on disk: a computer that loses power comes back with everything it said it had written, and nothing it said it had deleted. Windows cannot sync a directory, so there a name made or removed just before the power went may be lost, as far as NTFS keeps it. Every party on the computer can read and write all of it. A directory keeps no party out of anything, and Corbie says so.

On a file system that ignores case, as macOS's and Windows's do by default, `/Work` and `/work` are one directory. A secret or a value set at one of them is refused while the other has that name set, and so is a remove. A removal of a tree at one of them is refused while a secret or a value is set at the other or under it. A tree submitted at one of them is refused while the other is a tree. Listing the secrets or the values reads every file under `locations/`.

A **served directory** is for a few computers of one's own, with nothing rented and nothing else installed. A computer that holds the directory and the controller's identity serves it, and is a **member**. A party on another computer reaches it at `corbie://host:port`.

A store has one member by default, and roles on that computer use the directory as it is. A store with more is described under Members.

For example, the computer `desk` holds the directory and runs `corbie start controller runner store`, and a computer with a graphics card runs `corbie start runner` with the store `corbie://desk:8372`. One private connection joins them. On it `desk` proves the controller's key, and the other computer proves its runner's key.

`corbie init store` sets up the store's server alone, on a computer whose config holds the controller, and `corbie start` then runs it:

```
corbie init store                            listens on :8372; others are given corbie://<host name>:8372
corbie init store --listen 10.0.0.5:8372     listens there; others are given corbie://10.0.0.5:8372
corbie start store --listen :9000            listens there this time; others are given what init saved
corbie start store --timeout 5m              closes a connection that sends nothing for five minutes; a minute by default
```

The address other computers are given is the one `corbie init store` saved, with this computer's host name where it names every network interface. `corbie start store --listen` never changes it, `corbie init store` with no `--listen` keeps it, and `corbie init store --listen ""` serves the directory to no other computer.

`corbie init store --listen` refuses an IPv6 link-local address, `fe80::…`. Only a computer on the same link connects to it, and each writes it with the name of its own network interface.

There are no keys to a served directory: a party uses the identity it already has. A served directory's wire is private to the two computers. When a connection is made each end proves its own identity key: the party speaks only to a computer that holds its controller's key, and the server speaks only to a party whose key is the one in the controller's trust list for the name it gives, read afresh for every request, so a party that is removed is turned away from its next request on. The server holds each party to its row of the table, exactly as the table is drawn.

Someone on the network between them can see that the two computers talk and how much, and can hold work up or cut it off; they can read nothing, write nothing, change nothing and answer nothing. Nothing of a connection is kept once it ends.

The server closes a connection that sends nothing for a minute, between two requests or in the middle of a value. `corbie start store --timeout 5m` sets another time. A party that keeps sending is not cut off, and a value sent slowly delays no other party's write.

A party the controller trusts can fill every member's disk with values, and a client's listing of the records reads every file in them. Corbie limits neither.

A party waiting on its inbox is told the moment something is written there, as a process on the serving computer is, and is told nothing of names it may not read.

With one member, that computer is the store. While it is off nothing moves, and what was waiting waits. Its process can be stopped and started again, as it is to update it, with work in flight on the other computers: they wait about ten seconds for it and carry on.

A computer that loses power, or its network link, sends nothing, and the operating system waits minutes for it. So a server sends an HTTP 102 response each second until it answers a request, and a party stops waiting for a server that sends nothing for three seconds:

```
  while the connection is made      the request was not sent, and is sent again
  while each end proves its key     the request was not sent, and is sent again
  after the request is sent         a read, a listing or an overwrite is sent again; the outcome of any other change is reported as not known
```

A party sends a request again to the next member of the address, for about ten seconds. A request that takes the server long is waited for, however long.

Off for longer, a job that finished meanwhile cannot say so, and is a timeout when its time is up. A store that must outlive a computer has three members, or is a bucket. A directory with one member copies into a bucket unchanged.

Only a computer that holds the controller's identity serves, and only a directory. Someone with no identity is not shown a served directory's heads; a watcher there is a client.

A **bucket** is for several computers and for serverless hosts. It needs a service that offers the three ways of writing, ordered listing with each value's ETag, reading part of a value, and copying a value within the store. Corbie names the services it is tested against, says which of them can enforce the table above, and claims nothing more. In a bucket each party holds its own key, and the bucket's policy limits that key to its row of the table. Corbie prints the policy for each party. A policy cannot hold a name to one part, so it lets a runner or a client add names deeper in a record than the layout draws; it never lets one replace or remove anything.

**Memory** is for `corbie test` and Corbie's own tests. It enforces the table as a bucket does, so access rules can be tried without a bucket.

A process that waits on a path of a store is told when a name under it is written or deleted. Which writes it is told of depends on the kind of store:

```
  a directory          its own process's writes; from any process on the computer, a message in an inbox and a running head
  a served directory   as a directory; through a server, each write of a name the party may read
  a bucket, memory     its own process's writes
```

A process that is not told looks again at its interval.

- The operating system reports the writes of other processes to a directory. Corbie writes nothing for it.
- A directory watches the inboxes and the running heads alone. They hold only what is in flight, so the watch costs the same however many runs the store has kept.
- A controller takes a message from its inbox once it has acted on it, so a client that waits for a receipt or for a run is told when its message is taken, and when the run's head is written or removed.
- An emit of a job still running, written by another process, is found at the interval.
- Where the operating system refuses the watch, a write by another process is found at the interval, and `corbie start` prints a warning that says why.

## Members

A served directory with three members loses one and carries on. Each member holds a copy of the directory and the controller's identity, and the members keep their copies in step with Raft.

1. The members elect one of themselves leader.
2. A party sends each request to the leader. Another member answers with the leader's address.
3. The leader sends a value to the other members, then appends to a log what the change does: put this value under this name, or delete this name.
4. The leader answers a change once a majority of the members have the value and the entry on disk.
5. Each member applies the log to its copy, in order.

The log has one order, so of two writes only-if-new to one name, one is done and the other is told "already there". The leader answers a read once a majority has confirmed that it is still the leader, so no member gives an old value in place of a newer one.

For example, three switches share a store. `sw1` is set up as for one member, and a person copies `controller/identity` from its config to the other two.

```
sw1$   corbie init store
sw2$   corbie init store --join corbie://sw1:8372
sw3$   corbie init store --join corbie://sw1:8372
```

`--join` takes an empty directory: the members' directory is copied into it. `corbie start` on a computer set up with `--join` asks the members to take it, copies the directory and then follows the log. The computer counts as a member once its copy is whole.

Other parties are given `corbie://sw1:8372,sw2:8372,sw3:8372`, and reach the store while one computer of that address is a member and running.

```
  the leader stops                   the others elect a leader within seconds, and a party asks them at once
  the leader loses power or a link   the same, once it has sent the party nothing for three seconds
  a member cannot reach a majority   it answers no request, and a party that asks it reports that the store cannot be reached
  a member comes back                it follows the log from where it stopped, or copies the directory again
  a majority is off                  nothing moves, and what was waiting waits
```

A party that no member answers for about ten seconds reports what each member it asked said:

```
could not reach the store: dial tcp 10.0.0.1:8372: connect: no route to host; 10.0.0.2:8372: this member knows of no leader; 10.0.0.3:8372: this member knows of no leader
```

The members are changed with `corbie init store`:

```
  --join <address>       makes this computer a member
  --remove <host:port>   removes a member; asked of any other member
  --alone                makes this computer's copy the whole store, with one member
```

- Members are added and removed one at a time, and the others turn a removed member away. Its copy is not used again.
- `--remove` of an address that is not a member is refused.
- A member's address is not changed. `corbie init store --listen` with another address is refused there: the member is removed, and joins again with an empty directory.
- A store with two members stops when either is off, and `corbie start` prints that.
- `corbie init store` asks each member who leads, and prints what each answers: `leads`, `follows the leader`, `knows of no leader`, `does not answer`, or `joining, and not yet counted`.
- `--alone` is for a store that has lost a majority of its members for good. The copy may lack the last changes the store answered, so the store has gone back in time, as the next section describes.
- `--alone` is refused while a process listens at the address `corbie init store` saved, as `corbie start` does there: a running member writes its log again.
- A computer that was a member before `--alone` is never started again as one. Two computers that each believe they hold the store break only-if-new, on which everything rests.

On a member:

- Only the member's server writes its copy. A person reads the files with ordinary tools, as in any directory.
- From the moment a second member joins, a party on a member that opens the directory uses the members' servers in its place, and asks the member on its own computer first. No config changes. A controller there is held to the controller's row of the table.
- `corbie start` starts the store's server first, and the other roles once a majority of the members reach each other.
- The log is in `.log` at the top of the copy, with the values sent ahead of their entries.
- A member that cannot apply an entry stops, and stops again at each start. So a change to a name that can hold no value is refused and stops no member: a name under a value, or a name with names under it.
- With members, a name is UTF-8 and at most 512 bytes, and each of its parts at most 255 bytes and a file name that Windows takes. A change to any other name is refused before it is in the log.
- Windows takes no file name that contains one of `< > : " | ? *`, that ends in a dot or a space, or that is a device's name, such as `CON` or `NUL`, alone or before a dot. A member on any computer refuses such a part, so a member on Windows applies every entry that a member on Linux does.
- A computer is not made a member where its file system takes no file name of 255 bytes, or its directory's path is too long for a name of 512 bytes under it. `corbie start` prints which.
- A member holds the controller's identity, so it can do everything the controller can.

Members speak to each other on the wire parties use, and each proves the controller's key. A change the store has answered is on the disks of a majority of the members. The members' clocks need not agree for any of this.

## What the store is trusted with

Whoever runs the store can delete anything and can see every name. They cannot read what is sealed, and cannot alter a value without it showing, because everything that matters is signed by its writer or named in something that is. Corbie trusts the store not to hand back an old value in place of a newer one.

A store that has gone back in time, as a directory restored from a backup has, is still whole and signed, and simply older. Runs that moved since move again and their jobs run again, what was started since is gone, a party enrolled since is unknown, and a party removed since is trusted again. A backup is for a lost disk, not for undoing. A directory is copied with nothing running on it, or from a snapshot, so that the copy is one moment and not several.

Parties' clocks are trusted to agree within five minutes. Anything that depends on time allows for that much.
