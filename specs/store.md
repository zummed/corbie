# Store

The store is the one place Corbie keeps anything. Every party reads and writes it directly. Nothing of Corbie's runs inside it, and nothing of Corbie's stands in front of a directory or a bucket. The one exception is a directory that its computer serves to others, described below, and what serves it keeps nothing of its own.

## Named values

The store holds one kind of thing: a value with a name. Names are paths, and the store can list the names under a path in order. Everything else in Corbie, from a message to an instance's head to a job a runner has taken, is a named value used in a particular way.

There are three ways to write a value, and the rest of these specs call them by name.

- **Overwrite** writes the value whatever is there.
- **Only-if-new** writes it only if nothing has that name yet. Finding something already there is an answer, not a failure.
- **Only-if-unchanged** writes it only if the value there is still the one the writer last read.

A value is deleted by the first or the third. A value appears whole or not at all; nobody ever reads half of one.

These three are all the coordination Corbie has. There are no locks, no leases and no transactions across names.

Almost everything is written only-if-new and never changed afterwards. Very few values are ever rewritten: an instance's head, its note of how far a large step has been carried out, and the controller's lists of who it trusts and which secrets it holds. This makes the store easy to reason about and cheap to watch, since a value read once need never be read again unless it is one of those few.

## Layout

Each party has an area named after it. This is all of it:

```
version                                  the layout's version

<controller>/
  inbox/<sender>/<message>               messages to the controller
  receipts/<sender>/<message>            what became of each message a client sent
  running/<client>/<instance>            heads of instances that are running
  finished/<client>/<instance>           heads of instances that have ended
  records/<client>/<instance>/
      steps/<number>                     written by the controller
      emits/<job>/<number>               written by the runner doing that job
      blobs/<name>                       written by whoever made the blob
      carried                            how far a large step has been carried out
  wakes/<time>/<instance>/<name>         when an instance is next due attention
  trust/<party>                          who the controller trusts
  secrets/<name>                         the secrets it holds

<runner>/
  inbox/<sender>/<message>               jobs for the runner
  taken/<due time>/<job>                 one mark for each job it has taken
```

The version is written once, when the store is first set up, and a Corbie that does not know it refuses to use the store. An instance's name begins with the client that started its root and carries the time it was named, so anything that names an instance says where its head and record are and how old it is.

## Who may touch what

The layout is drawn so that the right to read or write is only ever a matter of paths.

| Party | May write | May read |
|---|---|---|
| Controller | everything in its own area; jobs into runners' inboxes | everything in its own area |
| Runner | results into the controller's inbox; new emits and blobs in records; its own inbox and marks | its own inbox and marks; blobs in records |
| Client | messages into the controller's inbox; new blobs in the records of its own runs | its own receipts, heads and records |
| A viewer given heads only | nothing | every head |

Every party can read the layout's version. A runner or a client adds to a record only-if-new: it can never change or remove what is there. A runner is not limited to the records of the jobs it holds, so a runner can add junk to any record. What it adds is signed by it and tied to a job, and readers ignore an entry from a runner that had no such job.

Blobs are encrypted and their keys travel only in sealed messages, so the right to read a blob's bytes gives nothing without its reference.

## Kinds of store

A store is a directory, a directory served to other computers, a bucket or memory, chosen by its address.

A **directory** is for one computer. Every value is a plain file, so a person can look at an instance with ordinary tools. A change the directory has answered is on disk: a computer that loses power comes back with everything it said it had written, and nothing it said it had deleted. Every party on the computer can read and write all of it. A directory keeps no party out of anything, and Corbie says so.

A **served directory** is for a few computers of one's own, with nothing rented and nothing else installed. The computer that holds the directory and the controller serves it, and a party on another computer reaches it at `corbie://host:port`. Roles on the serving computer use the directory as it is.

For example, the computer `desk` holds the directory and runs `corbie run controller runner store`, and a computer with a graphics card runs `corbie run runner` with the store `corbie://desk:8372`. One private connection joins them. On it `desk` proves the controller's key, and the other computer proves its runner's key.

There are no keys to a served directory: a party uses the identity it already has. A served directory's wire is private to the two computers. When a connection is made each end proves its own identity key: the party speaks only to the computer that holds its controller's key, and the server speaks only to a party whose key is the one in the controller's trust list for the name it gives, read afresh for every request, so a party that is removed is turned away from its next request on. The server holds each party to its row of the table, exactly as the table is drawn.

Someone on the network between them can see that the two computers talk and how much, and can hold work up or cut it off; they can read nothing, write nothing, change nothing and answer nothing. Nothing of a connection is kept once it ends, and the server keeps nothing of its own.

A party waiting on its inbox is told the moment something is written there, as roles in one process are, and is told nothing of names it may not read.

The serving computer is the store. While it is off nothing moves, and what was waiting waits. Its process can be stopped and started again, as it is to update it, with work in flight on the other computers: they wait about ten seconds for it and carry on. Off for longer, a job that finished meanwhile cannot say so, and is a timeout when its time is up. Corbie offers no second computer to take over and no copies kept in step: two computers that each believe they hold the store break only-if-new, on which everything rests. A store that must outlive a computer is a bucket, and a served directory copies into one unchanged. Only the controller's computer serves, and only a directory. Someone with no identity is not shown a served directory's heads; a watcher there is a client.

A **bucket** is for several computers and for serverless hosts. It needs a service that offers the three ways of writing, ordered listing, reading part of a value, and copying a value within the store. Corbie names the services it is tested against, says which of them can enforce the table above, and claims nothing more. In a bucket each party holds its own key, and the bucket's policy limits that key to its row of the table. Corbie prints the policy for each party. A policy cannot hold a name to one part, so it lets a runner or a client add names deeper in a record than the layout draws; it never lets one replace or remove anything.

**Memory** is for `corbie test` and Corbie's own tests. It enforces the table as a bucket does, so access rules can be tried without a bucket.

## What the store is trusted with

Whoever runs the store can delete anything and can see every name. They cannot read what is sealed, and cannot alter a value without it showing, because everything that matters is signed by its writer or named in something that is. Corbie trusts the store not to hand back an old value in place of a newer one.

A store that has gone back in time, as a directory restored from a backup has, is still whole and signed, and simply older. Runs that moved since move again and their jobs run again, what was started since is gone, a party enrolled since is unknown, and a party removed since is trusted again. A backup is for a lost disk, not for undoing. A directory is copied with nothing running on it, or from a snapshot, so that the copy is one moment and not several.

Parties' clocks are trusted to agree within five minutes. Anything decided by time allows for that much.
