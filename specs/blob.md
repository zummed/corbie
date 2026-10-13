# Blobs

A blob is a value too large to sit inside the entry that mentions it: a file, a long text, an image. Small values are simply written inline, and nothing about them is a blob.

## References

Where a blob is used, a reference stands in for it. The reference says where the blob is, what type and size it is, and holds the key that opens it. Holding the reference is the whole right to read the blob, so references only ever travel inside things that are sealed.

A reference's key is never shown. Wherever a value is displayed to a person or handed to a model, at the command line, through the gateway or on a screen, a blob appears as its type and size and a name to ask for it by. The display can fetch the contents for someone entitled to them.

## Storage

A blob is one value in the store, encrypted, written once and complete before anyone is given its reference. It is encrypted in pieces so that a reader can fetch part of a large blob without fetching all of it. Each piece is checked as it is read, so a blob that has been altered or cut short is refused.

A blob's contents are bytes to Corbie. A reference written inside a blob is not followed, kept or copied. The one exception is a stored definition, which Corbie writes itself: it names its scripts and child definitions, and wherever a definition is copied, what it names is copied with it.

## Where a blob lives and when it goes

Every blob lives inside the record of one instance and goes when that record goes. Nothing counts references and nothing extends a blob's life.

A blob made by a job is written into the record of the instance the job is for. A blob a client supplies with a start is written into the new instance's record.

A value handed **down** to a child is read where it is. A child never outlives its parent, and a parent keeps what its running children still use.

A value handed **up**, from a child to its parent, is copied into the parent's record before the parent sees it. The controller does this once, for every machine type, at the moment it passes the value on. A value a client carries from one run to another is copied the same way by the client. In a directory the copy is a link and costs nothing. In a bucket the bucket makes the copy itself and no data passes through Corbie.

A reference that names no blob, because a script made it up or the blob has gone, has nothing to copy. Handed up to a parent, it goes on as it is and opens nothing, wherever it is read, and nothing waits or stops on its account. A client that gives one with a start is told at once.

The price of owning by place is storage. The same script is stored again for each run that uses it, and a large result passed up three levels is stored once at each.

An upload for a run that has not started within a day is removed.

## Large blobs in a job

A job never has to hold a blob whole. The job's scratch directory is its local copy of whatever it is working on, and there is no other: no cache, and nothing kept on a runner between jobs.

A blob written to a file is read from the store a part at a time. A blob made from a file is read where the file lies, and sent to the store only if it leaves the job. Something large that a plugin receives, a download or a file unpacked from an archive, goes into a file in the scratch directory as it arrives when the script asks for it as a blob. So a job needs as much disk as the files it works on, and a runner short of disk fails the job as a full disk fails any write.

What a script is handed whole is held in the job's memory: the text of a file, a body, what a program printed, one item. There is one largest size for all of these, 64 MiB, and it is the only such size. Past it the call fails and says the way out, which is the same everywhere: ask for it as a blob, which has no largest size.

## Items

Some blobs have items: the lines of a text, the rows of a table, the entries of a list. A job walks them one after another and holds only the part of the blob it is reading, so a table of any size can be gone through. Reaching an item means reading up to it.

```
for (const row of blobs.items(input.table)) total += Number(row.amount)
```

A blob of any other type has no items as far as Corbie knows. A document, an image or an archive is a format, and taking one apart is work for a job that knows the format: a program run in the scratch directory, or a plugin for it.

## Limits

A step may read and write only small blobs. Work on large ones, such as splitting a file into items or scaling an image, is done by jobs.
