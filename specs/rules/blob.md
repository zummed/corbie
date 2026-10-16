# Blobs

The detailed rules behind the [Blobs overview](../blob.md).

## References

**R-blob-01** A value that is not a blob is written whole inside the entry that mentions it; where a blob is used, a reference stands in for it.
*Accept:* a job returns a short text, a small list and a file. Its result holds the text and the list as they are, and no name is written under any `blobs/` for them; the result, the step entry that records it and the input of the next job each hold a reference in the file's place, and none holds the file's bytes.

**R-blob-03** A reference says where the blob is, what type it is and how large it is, and holds the key that opens it; holding the reference is the whole right to read the blob.
*Accept:* a script handed a blob in its input reads the blob's type and size, and the store shows no read of the blob's value. A party that holds a reference and can read the stored bytes reads the blob's contents with nothing else; a party that can read the stored bytes and holds no reference reads nothing of the contents, and the bytes hold nothing of them in the clear.

**R-blob-05** References travel only inside things that are sealed.
*Accept:* run machines that pass blobs through inputs, outputs, emits, events and children, then read the whole store raw. No blob's key appears in the clear in any value or any name, and no head holds a reference.

**R-blob-06** Wherever a value is displayed to a person or handed to a model, a blob in it appears as its type, its size and a name to ask for it by, and its key is never shown.
*Accept:* a result holding a file is shown by `corbie run show` as text and as JSON, by the gateway's route and its streams, and by `corbie mcp`, and the same result is handed to an agent's model. Each shows the file's type, its size and the path to ask for it by, and the blob's key appears nowhere, in any encoding.

**C-blob-01** A reference written into a log line or an error shows no key. Logs and errors are read by people and passed on by tools that were never given the right to read the blob. *Test:* `blob_reference_in_log_or_error_hides_key`

**C-blob-02** Where a blob's contents are displayed or handed to a model, a reference found inside those contents, written as Corbie writes one, is shown as a type, a size and a name, like any other. A value too large for an entry can itself hold references, and a key shown from inside one is a key shown. *Test:* `blob_reference_inside_contents_hides_key`

**C-blob-03** The type of a blob made from a file comes from the file's name by one table that is part of Corbie. The operating system's own table differs from computer to computer, and the same file would be two types on two runners. *Test:* `blob_type_from_name_same_everywhere`

## Storage

**R-blob-09** A blob is one value in the store, encrypted, written once and never changed.
*Accept:* a job returns a file of several hundred megabytes whose contents repeat a marker string. Exactly one name is added under the `blobs/` of the instance's record; read raw, its value holds the marker nowhere. On a memory store that notes every write, every write under any `blobs/` is only-if-new, and nothing but removal ever happens to a blob afterwards.

**R-blob-12** A blob is complete in the store before anyone is given its reference.
*Accept:* kill a runner at every write it makes between a script's return and its result, and a client at every write of a start with files. Whenever a result, an emit entry or a start that holds a reference exists in the store, the blob it refers to can be read whole.

**R-blob-13** A blob is encrypted in pieces, so that a reader can fetch part of a large blob without fetching all of it.
*Accept:* read a hundred bytes from the middle of a blob of several hundred megabytes, on a store that counts the bytes it hands over. The bytes read are right, and the store handed over a small part of the blob.

**R-blob-14** Each piece is checked as it is read, and a blob that has been altered or cut short is refused.
*Accept:* change one byte of a stored blob: a read that touches that piece fails, saying the blob has been altered, and hands over nothing from the piece; swapping two pieces, or putting in a piece from another blob, is refused the same way. Remove the tail of a stored blob: a read that reaches the missing part fails, saying the blob has been cut short, and does not return a shorter blob as if it were whole.

**C-blob-05** The size a reference gives is checked against what is stored. Whole pieces removed from the end leave every piece that remains passing its own check. *Test:* `blob_whole_pieces_removed_from_end_refused`

**C-blob-06** A blob a job makes is sent to the store only when its reference leaves the job, in an emit or in the result; one made and used only inside the job is never sent. A job that packs a directory would otherwise send and fetch every file once more than it needs, hundreds of megabytes for nothing. *Test:* `blob_used_only_inside_a_job_never_reaches_the_store`

**C-blob-18** A job is never made to hold a blob whole. One it is given is read a part at a time, one made from a file is read where the file lies, and what a plugin receives to make a blob of is written into the job's scratch directory as it arrives. A job has only its own memory, and one large blob held whole takes all of it. *Test:* `blob_large_never_held_whole`

## Where a blob lives and when it goes

**R-blob-16** Every blob lives inside the record of one instance and goes when that record goes; nothing counts references and nothing extends a blob's life.
*Accept:* after flows, statecharts and agents have run with uploads, job results and children, every blob in the store is under the `blobs/` of some instance's record. After an instance is removed as past keeping, none of its blobs is in the store, and a reference to one of them kept by an application outside Corbie opens nothing. No value in the store records how many references a blob has, or a time to keep it until.

**R-blob-19** A blob made by a job is written into the record of the instance the job is for, and a blob a client supplies with a start into the new instance's record.
*Accept:* a job of a child instance returns a file: the blob is under the child's record, not its parent's and not the root's. `corbie run submit` with a file as input leaves the blob under `records/<client>/<the new run's name>/blobs/`.

**R-blob-21** A value handed down to a child is read where it is, and a parent keeps what its running children still use.
*Accept:* a parent passes a file from its own record as input to a child machine, which passes it on to a job: no blob is written for either hand-down, the child's record holds no copy, and the job reads the bytes from the parent's record. A parent that has run for longer than its keep, and whose own state no longer mentions the file, still holds it while the child is running, and the child's next job reads it (R-inst-39).

**R-blob-23** A value handed up from a child to its parent is copied into the parent's record before the parent sees it. The controller makes the copy, once, at the moment it passes the value on, and the same way for every machine type.
*Accept:* a child ends with a file in its output: the `done` event in the parent's step entry refers to a blob in the parent's own record, that blob was written before the step entry was, and after the child's record has been removed the parent still reads the file; the same holds for an event a child sends up while it runs. A flow, a statechart and an agent each run a child machine that hands up a file: each time, the memory store shows that the controller wrote one blob into the parent's record and no job did, and a second copy of the controller carrying out the same step finds the blob there and writes no other.

**R-blob-25** A value a client carries from one run to another is copied by the client into the new run's record.
*Accept:* a client gives a file from the result of run A as input to a start of run B. Before the start is sent, the client has written a blob under B's record, and B's input refers to that one. After A has been removed, B's jobs still read the file.

**R-blob-26** In a directory the copy is a link and costs nothing; in a bucket the bucket makes the copy itself and no data passes through Corbie.
*Accept:* copy a large file up in a directory store: the two names are one file on disk, and the directory's use of disk has not grown by the file's size. Copy one up on a bucket stand-in that counts bytes: one copy request is made, and Corbie neither fetched nor sent the blob's bytes.

**R-blob-29** An upload for a run that has not started within a day is removed.
*Accept:* a client uploads two files under a new run's name and never sends the start. Twenty-three hours later they are there; after a day and a housekeeping pass they are gone. The uploads of a run that did start are untouched.

**C-blob-10** A value handed up is copied whole: every blob it refers to, those a stored definition names included. The child's record may go before the parent's, and a reference left pointing into it stops opening. *Test:* `blob_copy_up_takes_blobs_referred_to_inside_blobs`

**C-blob-20** A value handed up that holds a reference to nothing is still handed up: every blob that is there is copied, and the reference that names none, or names something that is not a blob, goes on as it is. A script can write anything shaped like a reference into its result, and a controller that stopped at one would stop at it on every pass, with the parent never told. *Test:* `blob_reference_to_nothing_handed_up_opens_nothing`

**C-blob-11** When blobs are trimmed from a running instance, a blob named by a stored definition that the current state refers to counts as referred to. A definition made while a machine runs names its scripts, and they would otherwise go while it stays. *Test:* `blob_trim_follows_references_inside_blobs`

**C-blob-12** A search for references inside a blob passes over content that only looks like a reference and never fails on it; a reference is followed, kept or copied only in a stored definition, whose scripts and child definitions go with it. A blob holds whatever a job put there, and trimming and copying are not where content is judged. *Test:* `blob_reference_search_passes_over_lookalikes`

**C-blob-14** How old an upload or a blob is is never taken from a file's modification time; it is read from a name that carries the time it was given: an upload's from its instance's name, a blob's from its own. Backups and copies change file times. *Test:* `blob_age_not_from_file_times`

## Limits

**R-blob-30** A step reads and writes only small blobs; work on large ones is done by jobs.
*Accept:* a flow whose `each:` names a blob of several hundred megabytes runs on a store that counts bytes by party: the controller fetched none of that blob, a job read it and emitted its items, and the stage started once for each item. A step that could go on only by reading or writing a blob over the limit fails its instance, saying the blob is too large for a step.

**C-blob-15** A job that walks the items of a large blob holds only the part it is reading, and a file inside an archive is read out of the archive when it is asked for. Unpacking a whole archive to reach one file, or reading a whole table to reach its first rows, takes several times its size in memory. *Test:* `blob_item_read_on_demand`

**C-blob-16** An item of an archive whose name starts at the root or has `..` in it fails, read or written, and only files are items. Nothing may be unpacked outside the directory it is unpacked into. *Test:* `blob_archive_item_name_cannot_leave_its_directory`

**C-blob-17** A row of a table that has another number of fields than its header fails the read. A row quietly padded or cut puts values under the wrong names. *Test:* `blob_table_row_of_wrong_length_fails`

**C-blob-19** One item may be no larger than anything else that is handed to a script whole, and a larger one fails the walk, saying so. A text with no line ends, or a list with one enormous entry, would otherwise be held whole after all. *Test:* `blob_item_too_large_fails_the_walk`

## Interfaces

Uses:

- **Store**: one value for each blob, written only-if-new under a record's `blobs/`; reading a part of a value; a link for a copy in a directory and the bucket's own copy in a bucket; a value sent in parts appears whole or not at all (R-store-09).
- **Instances**: a blob lives in an instance's record, is trimmed while the instance runs (R-inst-39) and goes when the record goes.
- **Identity**: the sealed messages, entries and receipts that are the only things a reference travels in.

Used by:

- **Controller**: copies up what a child hands to its parent, before the parent is told (R-blob-23); removes uploads that never started as part of housekeeping; reads and writes the small blobs a step needs.
- **Runner**: reads a job's script, files and input by reference from records; writes the blobs in a job's emits and output into the instance's record.
- **Client**: uploads the files of a start, copies a value from one run to another, shows a blob as type, size and path in the one shape (C-client-16), and fetches contents for whoever may have them.
- **Machines**: a value handed down is a reference into the parent's record; a value handed up is a reference into the receiver's own.
- **Flow**: the items of a blob, read by a job.
- **Agent**: handles shown to a model in place of references (R-agent-47), and the tools that read blobs in part.
- **Plugins**: `blobs` makes and reads blobs for a script; `http` puts large bodies into blobs.
