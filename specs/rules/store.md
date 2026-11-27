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

**R-store-11** Only these values are ever rewritten: the head of a running instance, the note of how far a large step has been carried out, a key's file whose run has ended, an event sent to a key that moves on to the run with the key, a tree and the files at its locations when it is submitted again, an entry in the controller's list of who it trusts, and a secret or a value set at a location. Every other value is written only-if-new.
*Accept:* run flows, statecharts and agents to their ends on a memory store that notes every write. Every write to a name outside `running/`, `trust/` and `locations/`, a record's `carried` apart, is only-if-new, and under `locations/` so is every write under a location's `blobs/`.

**R-store-12** Every party reaches the store directly. Nothing of Corbie's runs inside it, and nothing of Corbie's stands in front of a directory or a bucket; what serves a directory to other computers keeps nothing of its own with one member, and a log of recent changes with more.
*Accept:* with no controller and no runner running, a client writes a start into the controller's inbox and a viewer lists heads, each using only the store's address and its own access to the store. A served directory's server is stopped and started again in the middle of a run on another computer: the run goes on from the directory, and the server has written no value of its own.

**R-store-13** A directory, a served directory, a bucket and memory behave alike in everything under this heading, and the address alone says which kind a store is.
*Accept:* one set of checks covering R-store-01 to R-store-10 runs against a directory, against a directory served in the same process, against one with three members in the same process, against memory and against each bucket service Corbie names, and passes on every one.

**C-store-01** Only the store's own answer counts as "not there", "already there" or "changed"; no reply, a request turned away for load, or an error from anything between the party and the store is reported as "could not reach the store". A caller acts on an answer, and would fail a run or take a message for sent on something that is not true. *Test:* `store_unreachable_is_never_an_answer`

**C-store-02** A listing is read to its end before it is trusted. A bucket returns at most a thousand names at a time, and stopping at the first page silently loses the rest. *Test:* `store_list_follows_pages`

**C-store-03** A name that was listed and is gone when it is read is passed over. Another copy may delete a message or move a head between the listing and the read. *Test:* `store_listed_name_gone_on_read_is_skipped`

## Layout

**R-store-14** The layout's version is the value named `version` at the top of the store, written once when the store is set up, and a Corbie that does not know the version it finds refuses to use the store.
*Accept:* `corbie init` on an empty directory leaves a value named `version` that every party can read; setting up a second controller identity on the same store leaves it as it was. Point any command or role at a store whose `version` this Corbie does not know: it ends in failure saying the store's layout is one it does not know, having written and deleted nothing. A store with no `version` is refused the same way by everything except the commands that set a store up. A store whose `version` is older than this Corbie's is refused, and the failure says an older Corbie set it up and that `corbie init` sets up a new one.

**R-store-17** Every value is at the place the overview's drawing gives it, under the party that writes it: messages to a party at `<party>/inbox/<sender>/<message>`; the answer to a client's message at `<controller>/receipts/<sender>/<message>/receipt`, in a directory of the message's own name, with a call's result at `result` and the blobs it names at `blobs/<name>` beside it; heads at `<controller>/running/<client>/<instance>` and `finished/<client>/<instance>`; a record's steps, emits and blobs at `<controller>/records/<client>/<instance>/steps/<number>`, `emits/<job number>/<number>` and `blobs/<name>`; wakes at `<controller>/wakes/<time>/<instance>/<name>`, listed soonest first; a location's entries at `<controller>/locations/<location>/+/`: `tree` at a tree's prefix, `file`, `blobs/<name>`, `keys/<hash>`, `secrets/<name>` and `values/<name>`, those of `/` at `<controller>/locations/+/`; a client's uploads at `<controller>/uploads/<client>/<message>/<name>`; trust at `<controller>/trust/<party>`; a runner's marks at `<runner>/taken/<due time>/<job>`.
*Accept:* run a flow started by `c1` through a job on `r1` that logs, returns a file and sets a timer, then enrol a runner, set a secret at `/` and at `/home`, and set a value at `/home`. Each of the values above exists at exactly that path, and the memory store shows the party named as its writer: the controller for steps, the job's message and the receipt, `c1` for the start and an uploaded file, `r1` for the result, the emits and the returned file. Client `c2` then calls a location in `c1`'s tree whose run returns a file: the call's receipt, its `result` and the file at its `blobs/<name>` are at that path, written by the controller, and `c2`'s input file is at `records/c2/<the name c2 chose>/blobs/`, written by `c2`. Three wakes set for three times list in order of their times.

**R-store-29** The drawing in the overview is the whole layout: no name is written that it does not draw.
*Accept:* after flows, statecharts and agents have run, with children, timers, keys, receipts, cancels and housekeeping, and with secrets and values set at locations, on a memory store, every name in the store matches one line of the drawing.

**R-store-65** No name in a record holds anything a definition or a person chose: steps and emit entries are named by number, a job's emits by the number the controller gives each attempt at a script, and blobs by their time and random characters. The job's name is inside its sealed entries.
*Accept:* run a flow whose machine, stage, script file, input, emitted value and returned blob each hold a marker. No name under `records/` holds the marker, and a client following the run is shown the emits under the stage's name. *Test:* `Test_store_record_names_say_nothing_chosen`

**R-store-30** An instance's name begins with the client that owns its root's tree and carries the time it was named, so its name alone says where its head and record are and how old it is.
*Accept:* a child three levels below a root in a tree of `c1` has its head under `running/c1/` or `finished/c1/` and its record under `records/c1/`. Given only an instance's name, as a parent's head or a wake carries it, a reader forms both paths and reads the time it was named without reading anything else.

**C-store-49** A key's file is named by the SHA-256 of the key, among its location's entries, and the key is in its body, signed by the controller and sealed to the controller only. A key is any text and comes from outside, so no name holds it, and only the controller reads which run it names. *Test:* `store_a_key_is_in_no_name` and `store_key_file_is_signed_and_sealed`

**C-store-50** A location's entries are under `+` after its path. A location is `/`, or `/` and parts, and no part is empty, `.`, `..` or `+`, or holds `#`, a backslash or a control character; a part that starts with a dot is kept after a `#`. A secret's name may be `+`, and a location with a `+` part would put one location's entries on the path above another's; a directory hides a name that starts with a dot. *Test:* `store_location_part_cannot_be_plus`

**C-store-04** Step numbers and emit numbers are written so that the names sort in number order. A listing sorts names as text, and step 10 would otherwise come before step 9. *Test:* `store_numbered_names_sort_by_number`

**C-store-05** The time in a wake's name is written in one fixed form and one zone. Wakes written by copies in different zones, or with different numbers of digits, would otherwise not list soonest first. *Test:* `store_wake_names_sort_by_time`

**C-store-06** The names of one sender's messages sort in the order they were sent, also when two are sent in the same instant or the sender's clock steps back. The order of names is the order the controller takes them in, and a result must never come before the emits sent ahead of it. *Test:* `store_message_names_sort_in_order_sent`

**C-store-07** No name the layout draws is also the path above another name. In a directory a name cannot be both a file and a directory. *Test:* `store_no_name_is_a_path_above_another`

**C-store-08** Each part of a name that another party supplied, such as a sender's name, a blob's name or a job's name, is checked before it is used: not empty, not starting with a dot, and holding no slash and no control character. Such a part could otherwise step out of its place in the layout, and in a directory out of the directory; a listing is one name per line, and a newline in a name is read as two. *Test:* `store_name_part_cannot_leave_its_place`

## Who may touch what

Each rule here is checked on the memory store opened as the party named, and on a stand-in for a bucket that carries the policy Corbie prints for that party. A directory enforces none of them (R-store-48).

**R-store-31** The controller may write, rewrite, delete and read every name in its own area; it may add jobs and cancels to a runner's inbox under its own name, only-if-new, and may touch nothing else in a runner's area.
*Accept:* as the controller, write, overwrite, delete, read and list a name under each of `inbox/`, `receipts/`, `running/`, `finished/`, `records/`, `wakes/`, `locations/`, `uploads/` and `trust/` in its area: every act is allowed. Write a job and a cancel under `<runner>/inbox/<controller>/` only-if-new: allowed. Overwrite or delete one of them, write under `<runner>/inbox/<another party>/`, write under `<runner>/taken/`, or read or list anything in a runner's area: each refused, and no name is given.

**R-store-34** A runner may add messages to the controller's inbox under its own name, only-if-new; may add emit entries and blobs to any record, only-if-new, and can never change or remove what is in a record or write a step entry; may write, rewrite, delete and read its own inbox and its own marks; may read the blobs in any record; and may do nothing else.
*Accept:* as runner `r1`: write a result under `<controller>/inbox/r1/` only-if-new: allowed; overwrite or delete it, or write under `<controller>/inbox/c1/`: refused. Write a new name under `emits/` and `blobs/` in the record of an instance it holds a job for and in one it holds no job for: all allowed; overwrite, write only-if-unchanged or delete one it wrote, or write any name under `steps/`: refused and the value is as it was. Delete a job from `r1/inbox/`, write a mark under `r1/taken/` only-if-new and delete it, list both, and read a blob in any record: allowed; the same in another runner's area: refused. Read or list a step entry, an emit entry, a head, a receipt or its blobs, a wake, a trust entry, anything under `locations/` or `uploads/`, or the controller's inbox: no value and no name is given; add a blob under a location, an upload or a receipt's blob: refused.

**R-store-39** A client may add messages to the controller's inbox under its own name, only-if-new; may add the files it sends with a message to its own uploads, only-if-new; may add blobs to the records of its own runs, only-if-new, and may write nothing else in any record; may read its own receipts and their blobs, and every head and record; and may do nothing else.
*Accept:* as client `c1`: write a start under `<controller>/inbox/c1/` only-if-new: allowed; overwrite or delete it, or write under `<controller>/inbox/c2/`: refused. Write a new name under `uploads/c1/<message>/`: allowed; overwrite or delete it, or write under `uploads/c2/`: refused. Write a new name under `records/c1/<instance>/blobs/`: allowed; overwrite or delete it, write under `steps/` or `emits/` of the same record, or write a blob under `records/c2/`: refused. Read and list `receipts/c1/`, `running/`, `finished/` and `records/`, `c2`'s included, and read a receipt's blob: allowed; `receipts/c2/`, the controller's inbox (the messages `c1` itself wrote included), `wakes/`, `trust/`, `uploads/`, any name under `locations/` or any runner's area: no value and no name is given.

**R-store-42** A viewer given heads only may read every head and may write nothing.
*Accept:* as such a viewer, list and read everything under `running/` and `finished/`: allowed. Read or list any other path except `version`: no value and no name is given. Any write or delete anywhere: refused.

**R-store-44** What a runner adds to a record is signed by it and tied to a job, and readers ignore an entry from a runner that had no such job.
*Accept:* an emit entry opened by the controller or by the root's client shows which runner signed it and which job it is for, and the signature checks against that runner's card. Runner `r2`, which was sent no job for instance `i`, writes a well-formed emit entry into `i`'s record under the number of a job that was sent to `r1`: a client following `i` is not shown it, and the machine receives nothing from it.

**C-store-09** A read of one name that the store refuses counts as "not there", while a write, a delete or a listing that the store refuses is reported as refused, never as done and never as "already there". A bucket answers a limited key "refused" for a name that holds nothing exactly as for one it may not read, so the two cannot be told apart; but taking a refused write for an answer would mark a message as sent that was never written. *Test:* `store_refused_read_is_not_there_refused_write_stays_refused`

**C-store-11** The policy printed for a runner or a client allows each write only when it is only-if-new. That one condition is the whole wall between parties in a bucket; without it a party could replace another's entry. *Test:* `store_policy_limited_party_cannot_replace`

## Kinds of store

**R-store-48** In a directory every value is one plain file, at a path that follows from its name, and every party on the computer can read and write all of it.
*Accept:* after a run on a directory store, find the run's head and its step entries with a file listing and print them with ordinary tools, using nothing of Corbie's. A process acting as a runner reads a head, a step entry and a trust entry, and deletes a message in the controller's inbox: none is refused.

**R-store-50** Corbie says that a directory keeps no party out of anything.
*Accept:* `corbie init` with a directory store prints, among what it did, that a directory keeps no party out. The manual's page on the store says the same.

**R-store-67** A process waiting on a path of a store is told at once of each name its own process writes or deletes under the path. In a directory it is told as well of a message in an inbox and of a running head that any other process on the computer writes or deletes under the path; the operating system reports them, and Corbie writes nothing for it. Any other write is found at the process's interval. Where the system refuses to report them, `corbie start` prints a warning that says why.
*Accept:* a process waits on a controller's inbox, on the controller's area and on its records, in a directory that holds nothing yet. Another process writes a message, deletes it, writes a second, and writes a running head twice: the first process is told each time, within a moment. The other process writes a step and a finished head: the first is told nothing. Two stores opened on one bucket by one process: a write through one is told to a wait on the other, and a write the bucket answers "already there" is not. On Linux with `fs.inotify.max_user_watches` at 0, `corbie start controller` prints a warning with the system's error and the names of the two limits, and takes a message another process writes at its interval.

**C-store-53** A directory has the operating system watch the inboxes and the running heads alone, and makes the watch again each time a delete has removed a directory and a write has made it anew. Those two hold only what is in flight. A watch on the records, the receipts or the finished heads costs one for each directory of every run the store has kept, and on macOS an open file for each name, until the system refuses more. *Test:* `store_directory_tells_of_another_processs_write`, `store_directory_says_why_it_is_not_watched`, `store_bucket_tells_the_process_that_wrote`

**R-store-51** In a bucket Corbie uses only what the overview names: reading a value or a part of one, ordered listing with each value's ETag, the three ways of writing, the two ways of deleting, and the bucket's own copy for a blob (see Blobs).
*Accept:* run the whole of Corbie's tests against a bucket stand-in that refuses every other kind of request, and whose listing gives each name's ETag as S3's does. Nothing fails.

**R-store-53** In a bucket each party uses a key of its own, and Corbie prints the policy for each party, which limits that party's key to its row of the table.
*Accept:* a controller, a runner and a client work on one bucket with three different keys; taking one party's key away stops that party and no other. Print the policy for a runner, for a client and for a heads-only viewer, and put each on a bucket stand-in: every act in that party's row is allowed and every act outside it is refused.

**R-store-60** A computer that holds a directory and its controller's identity can serve the directory to other computers, where a party reaches it at `corbie://host:port`. Every connection to it is made by the party with its own identity key, and there is no other key to the store. The server finds the party in the controller's list of who it trusts, read afresh for each request, and holds it to its row of the table as the table is drawn.
*Accept:* a runner on a second config is enrolled with one line, as on any store, and what `enrol` prints for its store is the `corbie://` address; it takes a job and returns the result with nothing else set up. A client there reads every head and record and no other client's receipt, and a runner reads no step; a name deeper than the layout draws is refused. The controller removes the runner: its next request is turned away.

**R-store-62** A party waiting on a path of a served directory is told the moment a name there that it may read is written, and is told nothing of any other name.
*Accept:* a runner on another computer takes a job within a moment of the controller sending it, with no look at an interval in between. A second runner waiting on the first one's inbox is told nothing when a job is written there.

**R-store-63** Only a computer that holds the controller's identity serves, and only a directory; someone with no identity is not shown its heads.
*Accept:* `corbie start store` on a config whose store is a bucket, or which has no controller, is refused and says why; `corbie init store` on a config with no controller is refused, makes nothing, and says to set the controller up first or use a bucket. `corbie init store --listen` with an IPv6 link-local address is refused and says why. With the serving computer stopped, a runner elsewhere reports that the store cannot be reached and carries on when it is back, having lost nothing. A config with no identity, given the controller's card and a `corbie://` address, is told to make a client.

**R-store-64** A served directory's wire is private to the two computers. A party speaks only to a computer that holds its controller's key, and the server only to a party whose key is the one in the controller's trust list for the name it gives. Someone between two computers can see that they talk and how much, and hold work up or cut it off, and nothing else.
*Accept:* a party given the address of a computer that does not hold its controller's key sends it nothing, and says the store cannot be reached. A request made in the plain, or over a connection that proves no key, reaches no store. A party's name given over a connection made with another key, a stranger's or another party's, is turned away.

**R-store-66** A served directory's server closes a connection that sends nothing for its timeout, between two requests or in the middle of a value. The timeout is a minute by default, and `corbie start store --timeout` sets it. A party that keeps sending is not cut off, however long its value takes, and a value sent slowly delays no other party's write.
*Accept:* with a timeout of 300 ms, a client that begins a value and sends no more is answered within seconds, and nothing of the value is in the directory; one that sends a byte every 100 ms for 800 ms has its value written; a connection that asks once and sends no more is closed. With twenty values begun and left unfinished, the controller writes a head at once. `corbie start store --timeout 0` and `corbie start runner --timeout 5m` are mistakes.

**R-store-55** The memory store enforces the table as a bucket does, so under `corbie test` each role works under its own row.
*Accept:* every acceptance check under "Who may touch what" passes on the memory store with no bucket. Under `corbie test`, a build in which the runner tries to read a step entry is given nothing, and one in which a client writes into another client's record is refused, exactly as each would be on a bucket.

**C-store-37** A served directory holds each party to its row itself, by the one table the memory store and a bucket's policy are made from, and takes a request only from a name in the controller's trust list, over a connection made with that card's key. Anything looser lets one computer on the network read or remove what is another's. *Test:* `store_served_directory_holds_each_party_to_its_row`

**C-store-39** A party is told of a write to a served directory only if it may read the name written. Told of every write, a client learns when other clients' runs move. *Test:* `store_served_directory_tells_a_waiting_party`

**C-store-40** A request to a served directory that was never sent, because nothing answered at its address, is tried again for about ten seconds, whatever kind of request it is. The serving computer's process is stopped and started when it is updated, and a job then in flight elsewhere otherwise finishes, cannot say so, and is reported minutes later as a timeout. *Test:* `store_served_directory_tries_outlast_a_restart`

**C-store-67** A served directory's server sends an HTTP 102 response each second until it answers a request, and a party stops waiting for a server that sends nothing for three seconds: while the connection is made, while each end proves its key, or after the request is sent. A request that was not sent is sent again to the next member, and one that was sent is sent again only where twice is the same as once. A computer that loses power or its link sends nothing, the operating system waits minutes for it, and a runner that waits as long takes no job meanwhile; a fixed limit on an answer gives up a request that takes the server long. *Test:* `store_served_directory_a_silent_member_is_passed_over`

**C-store-41** A server that is stopped answers every request it has in hand, those that wait among them, and keeps no connection open for another. A request cut off leaves its party not knowing whether it was done, and a connection kept is one a party writes to after the server has gone. *Test:* `store_served_directory_stopping_answers_what_it_has_in_hand`

**C-store-42** What goes wrong on the serving computer is told to that computer; the party is told only that something did, at once, and does not ask again. An error's own text names the serving computer's paths, and a failure that is asked for again for ten seconds and then called a store out of reach hides what it was. *Test:* `store_served_directory_failure_names_nothing_of_the_machine`

**C-store-43** Each end of a served directory's wire proves its identity key as the connection is made, and goes by nothing else: a party sends nothing to a computer that proves any key but its controller's, a request that comes by no such connection reaches no store, and whose key a name's is, is asked at every request, not once for a connection. The server keeps nothing from one request to the next; a list of what it had taken would be lost each time it stopped. *Test:* `store_served_directory_speaks_only_to_its_parties`

**C-store-51** A value streamed into a directory takes a turn to change it for each piece written, and gives it back while the next piece is read. A process changes sixteen names at once, so with a turn kept for the whole value, sixteen values begun from another computer and left unfinished stop every write of the controller's own. *Test:* `store_served_directory_slow_value_holds_up_no_other_write`

**C-store-52** The server's timeout starts again at each read of a value, and is the time a connection may wait between two requests. A connection left open and silent keeps a file handle and a temporary file on the serving computer for as long as the other end chooses, and a limit on a whole value would refuse a large one sent in good order. *Test:* `store_served_directory_cuts_off_a_silent_party`

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

## Members

**R-store-68** A served directory has one member by default, which keeps no log, and roles on that computer use the directory as it is. With more members, each holds a copy of the directory and the controller's identity, and the members keep the copies in step with Raft: the leader answers a change once a majority of the members have its value and its entry on disk, and each member applies the log to its copy in order.
*Accept:* a directory with one member holds no `.log`. Two more computers join it: each copy then holds what the first held, a large value among them. A party writes, copies and deletes: every copy holds the same values, as plain files. *Test:* `Test_store_members_joiner_copies_the_directory`

**R-store-69** With three members the store loses one, the leader included, and carries on: the others elect a leader within seconds, and a party waits for it as for a restart. A member that comes back follows the log from where it stopped, or copies the directory again where the others no longer keep the log that far back.
*Accept:* stop the leader of three members: a party's next write only-if-new is done, a value answered before reads back, and a name written before is "already there". Start it again: its copy holds what the others hold. Stop a member, make more changes than the others keep in their log, start it: its copy holds what the others hold, with what was deleted gone. *Test:* `Test_store_members_carry_on_without_one`, `Test_store_members_follow_by_copying_when_the_log_is_cut`

**R-store-70** A member that cannot reach a majority of the members answers no request, and a party that asks it reports that the store cannot be reached. While a majority is off nothing moves, and what was waiting waits.
*Accept:* cut the leader of three members off from the other two. A write and a read asked of it alone end in "could not reach the store", or for a write it had taken in "the outcome is not known", and never in "already there" or "not there". The other two do a write only-if-new to the same name. With the cut ended, all three copies hold the value the two wrote. *Test:* `Test_store_members_minority_answers_nothing`

**R-store-71** A party sends each request to the leader, and a member that does not lead answers with the leader's address. Enrolment gives a party every member's address, as `corbie://host:port,host:port,host:port`, and the party reaches the store while one computer of that address is a member and running.
*Accept:* a party given the address of one member that does not lead writes a value and reads it back. `corbie controller enrol` on a member prints the addresses of all three, and a client enrolled with them starts a run and reads its output with the first member stopped. *Test:* `Test_store_members_any_member_is_asked`, `Test_store_members_join_carry_on_and_leave`

**R-store-72** A computer becomes a member with its own card and no private key copied by hand: `corbie init store --card` prints the card, `--admit <card>` on a member prints the controller's identity encrypted to it with the members' addresses, and `--join` with that sets the computer up, with an empty directory. `corbie start` there asks the members to take it, copies the directory and follows the log, and the computer counts as a member once its copy is whole. `--remove <name>`, asked of any other member, removes a member, and the others turn it away. `--alone` makes this computer's copy the whole store, with one member and no log. Members are added and removed one at a time; `corbie init store` prints each member's name and address and whether it leads, follows the leader, knows of no leader, does not answer or is joining; and `corbie start` prints that a store with two members stops when either is off.
*Accept:* two computers each run `corbie init store --card`, give the card to `--admit` on the first, give what it prints to `--join`, and run `corbie start`: `corbie init store` on the first then prints three members by name and none joining, one that leads and two that follow. What was admitted for one card is refused by `--join` with another, which then holds nothing of the controller. A run goes on with the first stopped, and `corbie init store` on the second prints that the first does not answer. `--remove` of the first by its name, on the second, leaves two members and a run goes on; the first joins again with its card and an empty directory. `--remove` of a name that is no member's, and `--listen ""` on a member, are refused. `corbie start store --join` is a mistake that names `corbie init store`. With the third stopped, `--alone` there leaves a directory with no `.log`, and `--alone` again is refused as a store with one member. A member that is removed stops and says it was removed. *Test:* `Test_store_members_join_carry_on_and_leave`, `Test_store_members_removed_member_is_turned_away`, `Test_store_members_alone_is_a_directory_again`

**R-store-74** A member is known to the others by its card's name, and its address is a setting. A member sends its address with each request to another member, and a member that receives another address than it has saved saves it: `corbie init store --listen` with another address, then `corbie start`, moves a member. A party keeps the addresses it was given, and `corbie controller enrol <name>` prints them as they are now.
*Accept:* a member of three is stopped and started at another address: the others list it there under its name, and a write is in every copy. On a member, `corbie init store --listen` with another address prints that the other members are told it; after `corbie start`, another member's `corbie init store` prints the member at the new address, `corbie controller enrol` of a runner already enrolled prints the three addresses with the new one, and a run goes on. *Test:* `Test_store_members_a_member_at_another_address_is_found_there`, `Test_store_members_join_carry_on_and_leave`

**R-store-73** Only a member's server writes its copy, and a person reads the files as in any directory. From the moment a second member joins, a party on a member that opens the directory uses the members' servers in its place, its own computer's first, with no config changed. `corbie start` starts the store's server first, and the other roles once a majority of the members reach each other.
*Accept:* a party holds the directory of a store with one member open and writes a value as the directory is. Two computers join. The party's next write is in every copy, a name it wrote before is "already there", and a write by a process that opens the copy as a plain directory is refused. A party on each member asks that member first. *Test:* `Test_store_members_a_party_on_a_member_uses_its_server`, `Test_store_members_joiner_copies_the_directory`

**C-store-68** A member is counted by its card's key, and a name is one member's: the members take no computer whose card is a member's already, and none under a name that a member with another card has. `--admit` refuses a card with a member's name, and the refusal names `--remove`. A member that lost its directory and joined as it was would vote without the entries it had answered for, and `--remove` takes a name, which two members would share. *Test:* `Test_store_members_a_name_is_one_members`, `Test_store_members_join_carry_on_and_leave`

**C-store-54** The log holds what a change does, put this value under this name or delete this name, and never the change's condition. The leader checks the condition against its own copy before it makes the entry, with the name held until the entry is applied, also where the party that asked has gone, so that nothing it has put in the log for that name is still to be applied. A member applies some entries twice, after it stops, and applies entries to a copy made later than they are, after it copies the directory; a condition checked again then gives another answer, and the copies part. A name let go before its entry is applied is found free by the next write only-if-new, and both are done. *Test:* `Test_store_members_apply_twice_and_hold_the_same` and `Test_store_members_a_name_is_held_until_its_entry_is_applied`

**C-store-55** A leader answers a party only once it has applied every entry of the terms before its own, and answers a read, or that a change's condition does not hold, only after a majority has confirmed that it still leads. A new leader that has not applied what the last one put in the log finds a name free that is taken; a leader cut off from the others does not know it was replaced, and gives an old value or answers "already there" for a name the others have deleted. *Test:* `Test_store_members_minority_answers_nothing`

**C-store-56** A value is on the disks of a majority of the members before the entry that names it is made, and a member keeps a value sent ahead until it has written down that the entry is applied. A leader can be lost the moment it answers; and members that all stop with entries applied and not written down apply them again, and each must find the values on its own disk, since the others are doing the same. *Test:* `Test_store_members_apply_twice_and_hold_the_same`

**C-store-57** What a member that must copy the directory is sent names the members as they are after the last change of members. Raft has a member take a copy only if the copy's members name it, so a computer that joins is otherwise never given the directory. *Test:* `Test_store_members_joiner_copies_the_directory`

**C-store-58** A directory with members refuses a change made by anything but its member's server. A change that is not in the log is in one copy and not in the others. *Test:* `Test_store_members_joiner_copies_the_directory`

**C-store-59** A removed member learns that it is no member from the others turning it away, not from the log alone. The leader stops sending to a member once it is removed, so the member is not always sent the entry that removes it. *Test:* `Test_store_members_removed_member_is_turned_away`

**C-store-60** In a directory with members, `corbie start` starts the store's server before the other roles. The other roles read the store through the members' servers, and three computers that each wait for the store before they serve it never have one. *Test:* `Test_store_members_join_carry_on_and_leave`

**C-store-61** A change to a name that can hold no value does nothing on any member, and the party that asked is told it failed. Where the name is under a value or names are under it, each member finds that as it applies the entry. A name that is not UTF-8, is over 512 bytes, or has a part that is over 255 bytes or is no file name on Windows is refused before it is in the log, and a computer whose file system or directory's path does not take the longest such name is not made a member. A member stops over an entry it cannot apply and applies it again at each start, so one such write by any party would stop every member for good. *Test:* `Test_store_members_a_name_with_no_room_stops_no_member`, `Test_store_members_a_name_no_member_takes_is_refused` and `Test_store_members_a_path_too_long_for_a_members_names_is_no_member`

**C-store-62** A party that sends a value as it reads it waits for the member to ask for the value, which a member does once it has found that it leads; and once any of the value is read the party does not send the change again. A value sent again from where the reading stopped is stored cut short, and answered as done. *Test:* `Test_store_members_any_member_is_asked` and `Test_store_members_a_value_read_once_is_not_sent_again`

**C-store-63** A change that was in the log of a leader that stopped leading is the next leader's to apply or drop. The party sends it to the next leader where sending it twice is the same as sending it once, and otherwise reports that the outcome is not known. *Test:* `Test_store_members_a_change_in_flight_goes_to_the_next_leader`

**C-store-64** A member that applies its own removal stays for three election timeouts, gives its vote and takes entries meanwhile, and then stops. A leader that stops at once ends what it was sending, and the other of two members then holds an entry it can never count as in the log for good, and never leads. *Test:* `Test_store_members_the_leader_is_removed`

**C-store-65** A change is put in the log only in the term in which the party was let in and the change's condition was checked. A member that led, was replaced and leads again does not know what the leader between did to the name, and a write only-if-new would be done over that leader's value. *Test:* `Test_store_members_a_change_let_in_in_one_term_is_not_made_in_another`

**C-store-66** A member answers another member that a name holds nothing only where nothing is there, and a member that was given a value and could not put it under the name has not applied the entry. The member that asks deletes a name it is told holds nothing, and an entry counted as applied is not applied again: either way its copy lacks a value the others hold. *Test:* `Test_store_members_a_file_not_opened_is_not_answered_as_none` and `Test_store_members_a_value_not_put_under_its_name_is_not_applied`

## What the store is trusted with

**R-store-57** Whoever runs the store cannot read what is sealed.
*Accept:* start a run whose input, output, emits and receipts each hold a marker string of their own, and set a secret and a value that hold one, then read every value in the store raw. No marker appears in any value under `inbox/`, `receipts/`, `records/` or `locations/`.

**R-store-58** A value that matters cannot be altered without it showing: it is signed by its writer, or named in something that is.
*Accept:* change one byte of, in turn, a message in an inbox, a head, a step entry, an emit entry, a blob, a receipt, a trust entry, a key's file, a secret and a value. Each time, the party that next reads it refuses it as altered and does nothing it says.

**R-store-59** Anything a party does by setting its own clock against a time another party wrote allows five minutes; the allowance is for treating another party's time as passed, never for delaying a timer.
*Accept:* a receiver whose clock is five minutes ahead of the sender's takes a message sent a moment ago with the usual life, and does not drop it as past its time. A runner whose clock is five minutes ahead does not remove a mark until five minutes after the job's due time by its own clock. A timer set for a time is delivered at that time by the controller's clock, not five minutes later.

**C-store-35** A signed value says which name it belongs under, and a reader refuses one found under any other name. Whoever runs the store can otherwise put a true value in a false place: one instance's head under another's name, or an earlier step in place of a later one. *Test:* `store_value_moved_to_another_name_refused`

## Interfaces

Uses:

- Nothing of Corbie's. The store is storage and has no identity.

Used by:

- **Identity**: keeps the controller's trust entries under `trust/`; every message in an inbox is signed and sealed by it; each end of a served directory's wire is known by its identity key; enrolment hands out the store's address with the controller's card.
- **Instances**: heads under `running/` and `finished/`, records under `records/`, receipts, the results of calls and their blobs under `receipts/`; watching is listing and reading these, and a store tells a watcher of what was written as R-store-67 says.
- **Blobs**: each blob is one value under the `blobs/` of a record, a location or a receipt, or under a message's `uploads/`, of which a reader may fetch a part; a copy is a link in a directory and the bucket's own copy in a bucket; the right to read a blob's bytes gives nothing without its reference (R-blob-03).
- **Roles**: a pass lists an inbox and the due wakes and does all its work through the three ways of writing; a store wakes a role when its inbox is written (R-store-67).
- **Controller**: owns every name in its area, trees, locations, keys and uploads among them; writes jobs and cancels into runners' inboxes.
- **Runner**: reads its inbox, writes marks, emits, blobs and results, each within its row of the table.
- **Client**: writes messages, a submit's files under `uploads/` and a start's input into the record of the run it names, and reads its receipts, the results of its calls, heads and records.
- **Machines**: a draft's files and definitions are written side by side, under the store's bound, among a submit's uploads or into a record.
- **Secrets**: secrets and values are under `locations/<location>/+/secrets/` and `values/`, sealed to the controller.
- **Plugins**: an extension may add a kind of store, which must pass the checks of R-store-13.
- **Manual**: carries the list of bucket services Corbie is tested against and the statement that a directory keeps no party out.
