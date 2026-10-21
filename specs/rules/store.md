# Store

The detailed rules behind the [Store overview](../store.md).

## Named values

**R-store-01** A value written under a name is read back under that name exactly as written, and a name that holds nothing reads as "not there".
*Accept:* in each kind of store, write a value under a name and read it: the same bytes come back. Read a name never written: the answer is "not there" and no error.

**R-store-02** Overwrite writes the value whatever is there.
*Accept:* overwrite a name that holds nothing, then overwrite it with a second value. Both writes succeed and a read returns the second value.

**R-store-03** A value written only-if-new is not changed when the name already exists, and the writer is told it was there. "Already there" is an answer and not a failure.
*Accept:* two writers write different values to one name only-if-new; one succeeds, the other is told "already there", and a read returns the first value. A pass whose only-if-new write is answered "already there" carries on and ends without reporting an error.

**R-store-05** A value written only-if-unchanged is written only when the name still holds the value the writer last read. Otherwise nothing is written and the writer is told it has changed.
*Accept:* two writers read one value and each writes a different value only-if-unchanged; one succeeds, the other is told "changed", and a read returns the first writer's value. A writer whose value was deleted after it read is told "changed" too.

**R-store-06** A value is deleted whatever is there, or only-if-unchanged, and deleting a name that holds nothing is not a failure.
*Accept:* delete a name outright; a read then says "not there"; delete it again: no error. Read a value, let another writer change it, then delete only-if-unchanged: the answer is "changed" and the newer value is still there.

**R-store-08** The store lists every name under a path, in the order the names sort, each with what a read of it gives for a later only-if-unchanged write.
*Accept:* write `p/b`, `p/a`, `p/c/d` and `q/a` in any order. A listing under `p/` gives `p/a`, `p/b`, `p/c/d` in that order and nothing from `q/`, and gives the same in every kind of store. Each name comes with what a read of it gives, and an only-if-unchanged write of `p/a` with it succeeds; the next listing gives `p/a` with what a read of the new value gives and the other two as before.

**R-store-09** A value appears whole or not at all.
*Accept:* one writer overwrites a name with large values again and again while a reader reads it in a loop: every read gives "not there" or one whole value, never part of one. A writer killed partway through a write leaves the name holding what it held before.

**R-store-10** Every read, write and delete concerns one name, and nothing is held between calls: no lock, no lease, no set of writes that stand or fall together.
*Accept:* a party killed between any two of its writes leaves every name open at once to the others in all three ways, with nothing to wait out. When a caller makes two writes to two names and the second is refused, the first stands.

**R-store-11** Only four kinds of value are ever rewritten: the head of a running instance, the note of how far a large step has been carried out, an entry in the controller's list of who it trusts, and an entry in its list of secrets. Every other value is written only-if-new.
*Accept:* run flows, statecharts and agents to their ends on a memory store that notes every write. Every write to a name outside `running/`, `trust/` and `secrets/`, a record's `carried` apart, is only-if-new.

**R-store-12** Every party reaches the store directly. Nothing of Corbie's runs inside it, and nothing of Corbie's stands in front of a directory or a bucket; what serves a directory to other computers keeps nothing of its own.
*Accept:* with no controller and no runner running, a client writes a start into the controller's inbox and a viewer lists heads, each using only the store's address and its own access to the store. A served directory's server is stopped and started again in the middle of a run on another computer: the run goes on from the directory, and the server has written no value of its own.

**R-store-13** A directory, a served directory, a bucket and memory behave alike in everything under this heading, and the address alone says which kind a store is.
*Accept:* one set of checks covering R-store-01 to R-store-10 runs against a directory, against a directory served in the same process, against memory and against each bucket service Corbie names, and passes on every one.

**C-store-01** Only the store's own answer counts as "not there", "already there" or "changed"; no reply, a request turned away for load, or an error from anything between the party and the store is reported as "could not reach the store". A caller acts on an answer, and would fail a run or take a message for sent on something that is not true. *Test:* `store_unreachable_is_never_an_answer`

**C-store-02** A listing is read to its end before it is trusted. A bucket returns at most a thousand names at a time, and stopping at the first page silently loses the rest. *Test:* `store_list_follows_pages`

**C-store-03** A name that was listed and is gone when it is read is passed over. Another copy may delete a message or move a head between the listing and the read. *Test:* `store_listed_name_gone_on_read_is_skipped`

## Layout

**R-store-14** The layout's version is the value named `version` at the top of the store, written once when the store is set up, and a Corbie that does not know the version it finds refuses to use the store.
*Accept:* `corbie init` on an empty directory leaves a value named `version` that every party can read; setting up a second controller identity on the same store leaves it as it was. Point any command or role at a store whose `version` this Corbie does not know: it ends in failure saying the store's layout is one it does not know, having written and deleted nothing. A store with no `version` is refused the same way by everything except the commands that set a store up.

**R-store-17** Every value is at the place the overview's drawing gives it, under the party that writes it: messages to a party at `<party>/inbox/<sender>/<message>`; a client's receipts at `<controller>/receipts/<sender>/<message>` under the message's own name; heads at `<controller>/running/<client>/<instance>` and `finished/<client>/<instance>`; a record's steps, emits and blobs at `<controller>/records/<client>/<instance>/steps/<number>`, `emits/<job>/<number>` and `blobs/<name>`; wakes at `<controller>/wakes/<time>/<instance>/<name>`, listed soonest first; trust at `<controller>/trust/<party>`; secrets at `<controller>/secrets/<name>`; a runner's marks at `<runner>/taken/<due time>/<job>`.
*Accept:* run a flow started by `c1` through a job on `r1` that logs, returns a file and sets a timer, then enrol a runner and set a secret. Each of the values above exists at exactly that path, and the memory store shows the party named as its writer: the controller for steps and the job's message, `c1` for the start and an uploaded file, `r1` for the result, the emits and the returned file. Three wakes set for three times list in order of their times.

**R-store-29** The drawing in the overview is the whole layout: no name is written that it does not draw.
*Accept:* after flows, statecharts and agents have run, with children, timers, keys, receipts, cancels and housekeeping, on a memory store, every name in the store matches one line of the drawing.

**R-store-30** An instance's name begins with the client that started its root and carries the time it was named, so its name alone says where its head and record are and how old it is.
*Accept:* a child three levels below a root started by `c1` has its head under `running/c1/` or `finished/c1/` and its record under `records/c1/`. Given only an instance's name, as a parent's head or a wake carries it, a reader forms both paths and reads the time it was named without reading anything else.

**C-store-04** Step numbers and emit numbers are written so that the names sort in number order. A listing sorts names as text, and step 10 would otherwise come before step 9. *Test:* `store_numbered_names_sort_by_number`

**C-store-05** The time in a wake's name is written in one fixed form and one zone. Wakes written by copies in different zones, or with different numbers of digits, would otherwise not list soonest first. *Test:* `store_wake_names_sort_by_time`

**C-store-06** The names of one sender's messages sort in the order they were sent, also when two are sent in the same instant or the sender's clock steps back. The order of names is the order the controller takes them in, and a result must never come before the emits sent ahead of it. *Test:* `store_message_names_sort_in_order_sent`

**C-store-07** No name the layout draws is also the path above another name. In a directory a name cannot be both a file and a directory. *Test:* `store_no_name_is_a_path_above_another`

**C-store-08** Each part of a name that another party supplied, such as a sender's name, a blob's name or a job's name, is checked before it is used: not empty, not starting with a dot, and holding no slash and no control character. Such a part could otherwise step out of its place in the layout, and in a directory out of the directory; a listing is one name per line, and a newline in a name is read as two. *Test:* `store_name_part_cannot_leave_its_place`

## Who may touch what

Each rule here is checked on the memory store opened as the party named, and on a stand-in for a bucket that carries the policy Corbie prints for that party. A directory enforces none of them (R-store-48).

**R-store-31** The controller may write, rewrite, delete and read every name in its own area; it may add jobs and cancels to a runner's inbox under its own name, only-if-new, and may touch nothing else in a runner's area.
*Accept:* as the controller, write, overwrite, delete, read and list a name under each of `inbox/`, `receipts/`, `running/`, `finished/`, `records/`, `wakes/`, `trust/` and `secrets/` in its area: every act is allowed. Write a job and a cancel under `<runner>/inbox/<controller>/` only-if-new: allowed. Overwrite or delete one of them, write under `<runner>/inbox/<another party>/`, write under `<runner>/taken/`, or read or list anything in a runner's area: each refused, and no name is given.

**R-store-34** A runner may add messages to the controller's inbox under its own name, only-if-new; may add emit entries and blobs to any record, only-if-new, and can never change or remove what is in a record or write a step entry; may write, rewrite, delete and read its own inbox and its own marks; may read the blobs in any record; and may do nothing else.
*Accept:* as runner `r1`: write a result under `<controller>/inbox/r1/` only-if-new: allowed; overwrite or delete it, or write under `<controller>/inbox/c1/`: refused. Write a new name under `emits/` and `blobs/` in the record of an instance it holds a job for and in one it holds no job for: all allowed; overwrite, write only-if-unchanged or delete one it wrote, or write any name under `steps/`: refused and the value is as it was. Delete a job from `r1/inbox/`, write a mark under `r1/taken/` only-if-new and delete it, list both, and read a blob in any record: allowed; the same in another runner's area: refused. Read or list a step entry, an emit entry, a head, a receipt, a wake, a trust entry, a secret or the controller's inbox: no value and no name is given.

**R-store-39** A client may add messages to the controller's inbox under its own name, only-if-new; may add blobs to the records of its own runs, only-if-new, and may write nothing else in any record; may read its own receipts, heads and records; and may do nothing else.
*Accept:* as client `c1`: write a start under `<controller>/inbox/c1/` only-if-new: allowed; overwrite or delete it, or write under `<controller>/inbox/c2/`: refused. Write a new name under `records/c1/<instance>/blobs/`: allowed; overwrite or delete it, write under `steps/` or `emits/` of the same record, or write a blob under `records/c2/`: refused. Read and list `receipts/c1/`, `running/c1/`, `finished/c1/` and `records/c1/`: allowed; the same paths for `c2`, the controller's inbox (the messages `c1` itself wrote included), `wakes/`, `trust/`, `secrets/` or any runner's area: no value and no name is given.

**R-store-42** A viewer given heads only may read every head and may write nothing.
*Accept:* as such a viewer, list and read everything under `running/` and `finished/`: allowed. Read or list any other path except `version`: no value and no name is given. Any write or delete anywhere: refused.

**R-store-44** What a runner adds to a record is signed by it and tied to a job, and readers ignore an entry from a runner that had no such job.
*Accept:* an emit entry opened by the controller or by the root's client shows which runner signed it and which job it is for, and the signature checks against that runner's card. Runner `r2`, which was sent no job for instance `i`, writes a well-formed emit entry into `i`'s record under the name of a job that was sent to `r1`: a client following `i` is not shown it, and the machine receives nothing from it.

**C-store-09** A read of one name that the store refuses counts as "not there", while a write, a delete or a listing that the store refuses is reported as refused, never as done and never as "already there". A bucket answers a limited key "refused" for a name that holds nothing exactly as for one it may not read, so the two cannot be told apart; but taking a refused write for an answer would mark a message as sent that was never written. *Test:* `store_refused_read_is_not_there_refused_write_stays_refused`

**C-store-11** The policy printed for a runner or a client allows each write only when it is only-if-new. That one condition is the whole wall between parties in a bucket; without it a party could replace another's entry. *Test:* `store_policy_limited_party_cannot_replace`

## Kinds of store

**R-store-48** In a directory every value is one plain file, at a path that follows from its name, and every party on the computer can read and write all of it.
*Accept:* after a run on a directory store, find the run's head and its step entries with a file listing and print them with ordinary tools, using nothing of Corbie's. A process acting as a runner reads a head, a step entry and a trust entry, and deletes a message in the controller's inbox: none is refused.

**R-store-50** Corbie says that a directory keeps no party out of anything.
*Accept:* `corbie init` with a directory store prints, among what it did, that a directory keeps no party out. The manual's page on the store says the same.

**R-store-51** In a bucket Corbie uses only what the overview names: reading a value or a part of one, ordered listing with each value's ETag, the three ways of writing, the two ways of deleting, and the bucket's own copy for a blob (see Blobs).
*Accept:* run the whole of Corbie's tests against a bucket stand-in that refuses every other kind of request, and whose listing gives each name's ETag as S3's does. Nothing fails.

**R-store-53** In a bucket each party uses a key of its own, and Corbie prints the policy for each party, which limits that party's key to its row of the table.
*Accept:* a controller, a runner and a client work on one bucket with three different keys; taking one party's key away stops that party and no other. Print the policy for a runner, for a client and for a heads-only viewer, and put each on a bucket stand-in: every act in that party's row is allowed and every act outside it is refused.

**R-store-60** The computer that holds a directory and its controller can serve the directory to other computers, where a party reaches it at `corbie://host:port`. Every connection to it is made by the party with its own identity key, and there is no other key to the store. The server finds the party in the controller's list of who it trusts, read afresh for each request, and holds it to its row of the table as the table is drawn.
*Accept:* a runner on a second config is enrolled with one line, as on any store, and what `enrol` prints for its store is the `corbie://` address; it takes a job and returns the result with nothing else set up. A client there reads its own heads and records and no other client's, and a runner reads no step; a name deeper than the layout draws is refused. The controller removes the runner: its next request is turned away.

**R-store-62** A party waiting on a path of a served directory is told the moment a name there that it may read is written, and is told nothing of any other name.
*Accept:* a runner on another computer takes a job within a moment of the controller sending it, with no look at an interval in between. A second runner waiting on the first one's inbox is told nothing when a job is written there.

**R-store-63** Only the controller's computer serves, and only a directory. Corbie keeps no second copy of a served directory and has no computer take over from the one that serves it; someone with no identity is not shown its heads.
*Accept:* `corbie start store` on a config whose store is a bucket, or which has no controller, is refused and says why; `corbie init store` on a config with no controller is refused, makes nothing, and says to set the controller up first or use a bucket. With the serving computer stopped, a runner elsewhere reports that the store cannot be reached and carries on when it is back, having lost nothing. A config with no identity, given the controller's card and a `corbie://` address, is told to make a client.

**R-store-64** A served directory's wire is private to the two computers. A party speaks only to the computer that holds its controller's key, and the server only to a party whose key is the one in the controller's trust list for the name it gives. Someone between two computers can see that they talk and how much, and hold work up or cut it off, and nothing else.
*Accept:* a party given the address of a computer that does not hold its controller's key sends it nothing, and says the store cannot be reached. A request made in the plain, or over a connection that proves no key, reaches no store. A party's name given over a connection made with another key, a stranger's or another party's, is turned away.

**R-store-55** The memory store enforces the table as a bucket does, so under `corbie test` each role works under its own row.
*Accept:* every acceptance check under "Who may touch what" passes on the memory store with no bucket. Under `corbie test`, a build in which the runner tries to read a step entry is given nothing, and one in which a client writes into another client's record is refused, exactly as each would be on a bucket.

**C-store-37** A served directory holds each party to its row itself, by the one table the memory store and a bucket's policy are made from, and takes a request only from a name in the controller's trust list, over a connection made with that card's key. Anything looser lets one computer on the network read or remove what is another's. *Test:* `store_served_directory_holds_each_party_to_its_row`

**C-store-39** A party is told of a write to a served directory only if it may read the name written. Told of every write, a client learns when other clients' runs move. *Test:* `store_served_directory_tells_a_waiting_party`

**C-store-40** A request to a served directory that was never sent, because nothing answered at its address, is tried again for about ten seconds, whatever kind of request it is. The serving computer's process is stopped and started when it is updated, and a job then in flight elsewhere otherwise finishes, cannot say so, and is reported minutes later as a timeout. *Test:* `store_served_directory_tries_outlast_a_restart`

**C-store-41** A server that is stopped answers every request it has in hand, those that wait among them, and keeps no connection open for another. A request cut off leaves its party not knowing whether it was done, and a connection kept is one a party writes to after the server has gone. *Test:* `store_served_directory_stopping_answers_what_it_has_in_hand`

**C-store-42** What goes wrong on the serving computer is told to that computer; the party is told only that something did, at once, and does not ask again. An error's own text names the serving computer's paths, and a failure that is asked for again for ten seconds and then called a store out of reach hides what it was. *Test:* `store_served_directory_failure_names_nothing_of_the_machine`

**C-store-43** Each end of a served directory's wire proves its identity key as the connection is made, and goes by nothing else: a party sends nothing to a computer that proves any key but its controller's, a request that comes by no such connection reaches no store, and whose key a name's is, is asked at every request, not once for a connection. The server keeps nothing from one request to the next; a list of what it had taken would be lost each time it stopped. *Test:* `store_served_directory_speaks_only_to_its_parties`

**C-store-12** A party's key for a bucket is never part of the store's address, and no error or log line shows it. The address is printed at enrolment and handed to every party. *Test:* `store_address_and_errors_carry_no_key`

**C-store-14** In a directory a value's bytes are safely on disk before it is moved onto its name. A computer that loses power just after the move can otherwise show the name holding an empty or partial value. *Test:* none; losing power cannot be staged in a test.

**C-store-36** In a directory a write, a copy or a delete answers only once the directory holding the name is safely on disk as well, and every directory above it in the directory, any of which may have been made for the name. The system keeps a new name in memory for seconds, up to half a minute, and a computer that loses power in that time comes back without writes it answered: a runner on another computer that took a job and finished it finds the job back and its mark gone, and runs it again with nothing to say it is the second time. *Test:* `store_directory_answered_change_is_on_disk`

**C-store-15** In a directory only the final act of putting a value onto its name may answer "already there". A directory that comes or goes at that moment gives errors that read like "exists", and "already there" for a message means it was sent. *Test:* `store_directory_only_if_new_while_directory_comes_and_goes`

**C-store-16** A directory write whose directory or temporary file is taken away under it is tried again, a few times, and not failed at once. Another party's delete or clearing-up can remove either between the write's two steps. *Test:* `store_directory_write_survives_vanishing_directory_and_temporary_file`

**C-store-17** Leftover temporary files in a directory are cleared by their age on the computer's own clock, never by a time a caller passes in. A caller's time can be far ahead, and a write in progress would lose its file. *Test:* `store_directory_clearing_spares_temporary_file_in_use`

**C-store-18** A directory listing returns every name that was there from its start to its end, once each, while other names are being written and replaced. Reading a directory during a move onto a name can skip that name or give it twice. *Test:* `store_directory_list_while_names_replaced`

**C-store-19** A directory write gets its turn however many listings overlap. Watchers list all the time, and an operating system may keep letting new readers in while a writer waits. *Test:* `store_directory_write_not_starved_by_listings`

**C-store-20** Only-if-unchanged in a directory keeps every other writer of that name out, in this process and in every other, from its read to its write. A directory has no conditional write of its own, and two writers that each compare and then write both succeed. *Test:* `store_directory_only_if_unchanged_across_processes`

**C-store-21** Every wait for another writer in a directory ends when the caller's deadline does, and a call whose deadline has already passed waits for nothing. A pass must end before its host's limit whatever another process is doing. *Test:* `store_directory_wait_ends_with_deadline`

**C-store-22** Any number of directory writes may wait at once in one process without each holding a thread of the operating system. Thousands of waiting writes otherwise end the process at its limit of threads. *Test:* `store_directory_many_waiting_writes_few_threads`

**C-store-23** A party killed at any instant leaves nothing in a directory that keeps another party waiting. Whatever marks a name as busy has to go when its holder dies, or one killed listing holds every head for good. *Test:* `store_directory_killed_party_blocks_nobody`

**C-store-24** Listing and reading work in a directory the party can only read. A viewer may have no right to write anything there, not even a file that marks a name as busy. *Test:* `store_directory_list_when_read_only`

**C-store-25** Only-if-new and only-if-unchanged in a bucket are each one request that carries its condition, never a look followed by a write, and a bucket write never reads or lists the name first. Two parties that each look and then write both succeed; and a party that may add but not read is refused the look, which says nothing about whether the name is free. *Test:* `store_bucket_conditional_write_is_one_request`

**C-store-27** An only-if-new or only-if-unchanged write whose answer was lost is never sent again, and the caller is told the outcome is not known. If the first try landed, a second meets its own value and reports "already there" or "changed" for a write that succeeded. *Test:* `store_bucket_conditional_write_not_resent_after_lost_answer`

**C-store-28** A request the bucket certainly did not act on, because it answered "slow down", is sent again whatever kind it is; a request that may have been acted on is sent again only when twice is the same as once: a read, a listing, an overwrite, an outright delete. Nothing happened in the first case, so nothing can happen twice; any other request sent twice can report a result that is not true. *Test:* `store_bucket_resends_only_what_is_safe_twice`

**C-store-30** Tries at a bucket request go on for a few seconds, with growing waits, before the store is reported as out of reach. A bucket turns requests away during a burst, and a few quick tries do not outlast it. *Test:* `store_bucket_retries_outlast_a_burst`

**C-store-31** A connection to a bucket that has sat unused for a few seconds is not used again. A bucket closes kept connections while a serverless copy is frozen, and a conditional write that meets a closed connection cannot be sent again. *Test:* `store_bucket_idle_connection_not_reused`

**C-store-32** A value in a bucket is never changed by copying it onto itself. A bucket counts that as replacing the value, which a party limited to only-if-new may not do. *Test:* `store_bucket_never_copies_a_value_onto_itself`

**C-store-34** Values named by a listing are read a bounded number at a time. A process has a limit of open connections and files, and a bucket turns away a burst. *Test:* `store_bucket_reads_bounded_at_once`

**C-store-44** In a bucket, what a listing gives for each name is the ETag in the bucket's own listing, and no value is read to give it. A watcher lists at every look, and a read for each name is the cost the listing is there to save. *Test:* `store_bucket_listing_reads_no_value`

**C-store-45** In a directory, what a listing gives for each name is made from the file's contents, as a read's is, never from its size or its time. A value rewritten at the same size within the resolution of the file's time keeps both, and a watcher would never read it again. *Test:* `store_directory_listing_follows_contents`

## What the store is trusted with

**R-store-57** Whoever runs the store cannot read what is sealed.
*Accept:* start a run whose input, output, emits, run secrets and receipts each hold a marker string of their own, then read every value in the store raw. No marker appears in any value under `inbox/`, `receipts/`, `records/` or `secrets/`.

**R-store-58** A value that matters cannot be altered without it showing: it is signed by its writer, or named in something that is.
*Accept:* change one byte of, in turn, a message in an inbox, a head, a step entry, an emit entry, a blob, a receipt, a trust entry and a secret. Each time, the party that next reads it refuses it as altered and does nothing it says.

**R-store-59** Anything a party does by setting its own clock against a time another party wrote allows five minutes; the allowance is for treating another party's time as passed, never for delaying a timer.
*Accept:* a receiver whose clock is five minutes ahead of the sender's takes a message sent a moment ago with the usual life, and does not drop it as past its time. A runner whose clock is five minutes ahead does not remove a mark until five minutes after the job's due time by its own clock. A timer set for a time is delivered at that time by the controller's clock, not five minutes later.

**C-store-35** A signed value says which name it belongs under, and a reader refuses one found under any other name. Whoever runs the store can otherwise put a true value in a false place: one instance's head under another's name, or an earlier step in place of a later one. *Test:* `store_value_moved_to_another_name_refused`

## Interfaces

Uses:

- Nothing of Corbie's. The store is storage and has no identity.

Used by:

- **Identity**: keeps the controller's trust entries under `trust/`; every message in an inbox is signed and sealed by it; each end of a served directory's wire is known by its identity key; enrolment hands out the store's address with the controller's card.
- **Instances**: heads under `running/` and `finished/`, records under `records/`, receipts under `receipts/`; watching is listing and reading these, and a directory or memory store tells a watcher in the same process at once that something was written.
- **Blobs**: each blob is one value under a record's `blobs/`, of which a reader may fetch a part; a copy is a link in a directory and the bucket's own copy in a bucket; the right to read a blob's bytes gives nothing without its reference (R-blob-03).
- **Roles**: a pass lists an inbox and the due wakes and does all its work through the three ways of writing; a store in the same process wakes a role when another role writes what it reads.
- **Controller**: owns every name in its area; writes jobs and cancels into runners' inboxes.
- **Runner**: reads its inbox, writes marks, emits, blobs and results, each within its row of the table.
- **Client**: writes messages and uploads, and reads its receipts, heads and records.
- **Machines**: a draft's files and definitions are written into a record side by side, under the store's bound.
- **Secrets**: the controller's secrets are the values under `secrets/`, sealed to the controller.
- **Plugins**: an extension may add a kind of store, which must pass the checks of R-store-13.
- **Manual**: carries the list of bucket services Corbie is tested against and the statement that a directory keeps no party out.
