# Plugins: rules

The detailed rules behind the [Plugins overview](../plugin.md). Requirements are `R-plug-nn`, constraints `C-plug-nn`. A "stand-in plugin" is one built for the tests as an extension, on the public package alone.

## Three layers

**R-plug-01** A script can do nothing outside itself except by calling a plugin.
*Accept:* in a job whose every layer is `all`, nothing a script has apart from plugins and the libraries over them reads a file, opens a connection, starts a program or reads a variable of the runner's.

**R-plug-02** A plugin never opens anything by itself. It asks Corbie for a way to connect, a path, a program or a variable.
*Accept:* for each plugin that comes with Corbie, every call that reaches outside is refused in a job no layer allows anything, and nothing happens outside the job: a stand-in server sees no connection, no file is made, no program starts.

**R-plug-03** At a way out, Corbie checks the job's permissions, notes what was used, puts secrets in where they are bound to go, and hands the thing over or refuses. This is the same for every plugin and no plugin does any of it.
*Accept:* a stand-in plugin asks for a way to connect and sends a handle through it, and does nothing else. Its connection is refused when no layer allows the host, is in the job's list of what was used when it is allowed, and carries the secret only to a host the secret is bound to.

**R-plug-04** A library written in script needs no trust: it can do only what the job is permitted.
*Accept:* a library file makes a web request to a host the job is not allowed. The job ends denied, with the refusal the same request made from the script itself gets.

**R-plug-06** Plugins and the libraries that come with a binary share one set of names, which a script loads by bare name (see Runner, R-run-42). Two of them under one name stop the binary as it starts.
*Accept:* a binary built with an extension whose library is called `http` stops at once when started, and says which name is taken twice.

## The ways out

**R-plug-07** Connect gives a plugin a way to open connections, not one open connection, either plain or carrying named secrets. Each address opened through it is checked against `net` and against each secret's destination, like the first.
*Accept:* a job allows `net:a.example` and `net:c.example`. A stand-in plugin opens `a.example`, then `b.example`, then `c.example` through one way to connect: the first and third open, the second is refused, naming `net:b.example`. Asked for with a handle, the way to connect gives the secret's value only for a host the secret is bound to (R-sec-40).

**R-plug-09** Path gives a plugin a file's real location, to read or to write, checked against `fs.read` or `fs.write`.
*Accept:* a stand-in plugin asks to read `/srv/reports/latest`, a link to `/srv/reports/2026/q3.csv`, in a job allowed `fs.read:/srv/reports/**`. It is given `/srv/reports/2026/q3.csv`. Asked to write the same file, it is refused.

**R-plug-10** Program gives a plugin a started program, given its arguments, variables and files, checked against `exec`; the program has the variables it was given and, the runner's search path apart, no others of the runner's.
*Accept:* a stand-in plugin starts an allowed program with two arguments, one variable and one file, on a runner started with `RUNNER_ONLY` set. The program receives exactly those, and `RUNNER_ONLY` is not among its variables. Starting a program the job is not allowed is refused, and nothing starts.

**R-plug-11** Variable gives a plugin the value of one of the runner's variables, checked against `env`.
*Accept:* a stand-in plugin asks for `HOME` in a job allowed `env:HOME` and gets the runner's value. Asked for `PATH`, it is refused.

**R-plug-12** The job's own channel is its result, what it emits, its blobs and the machines it defines; it is checked against nothing, every job has it, and nothing leaves a job except by these ways.
*Accept:* in a job no layer allows anything, the `blobs` and `machines` plugins work, and `emit` and the script's return value reach the record and the result. The public package gives a plugin a way to connect, a path, a program, a variable and the job's own channel, and no other means of reaching outside the job.

**R-plug-14** A plugin that wraps someone else's library turns off whatever that library would find by itself: the computer's variables, files in the user's home, and its own lookups.
*Accept:* the runner has proxy variables set, and its user's home holds settings, known hosts and keys for the programs the wrapped libraries imitate. Requests through `http`, `websocket` and `llm` go straight to their hosts. `ssh` signs in with nothing but what the script gave and trusts no host the script did not name. Each name a script gives reaches Corbie's check as it was written.

**C-plug-02** The code of a plugin that comes with Corbie reaches files, connections, programs and variables only through what Corbie hands it. A check over what each one is built from fails the build when one uses the operating system's own means. Whether a plugin's reach is checked can then be answered without reading the plugin. *Test:* `plug_bundled_reach_only_through_ways_out`

**C-plug-04** A connection opened for one destination is never used for a request to another, and none is kept from one job to the next. A connection is the outcome of one check and may be signed in. Used again, it carries a request past the check that request should have had. *Test:* `plug_connection_not_reused_across_destinations`

**C-plug-05** A name is handed to Corbie unresolved. No library a plugin wraps looks a name up first. A lookup the library makes never passes the check, and it tells the outside the name. *Test:* `plug_wrapped_library_makes_no_lookup`

## One shape

**R-plug-15** A call is `module.call(first, options)`: the one obvious thing first, then named options, and an option the plugin does not know is refused.
*Accept:* every call on the page of every plugin that comes with Corbie takes one first value and, where it takes more, one map of named options after it. `http.get(url, { hedaers: {} })` fails as bad input, naming `hedaers`, and no request is sent; the same holds for an unknown option on every call of every plugin that comes with Corbie.

**R-plug-17** Calls wait for their answer.
*Accept:* `const page = http.get(input.url)` gives the answer itself, and `page.body` is there on the next line. No call takes a callback or returns something to be waited on.

**R-plug-18** What goes into a call and comes out of it is plain data: text, numbers, lists, maps, bytes, handles to secrets, references to blobs; and bytes are one type everywhere.
*Accept:* the result of a web request, returned as the job's output, arrives in the result with every value in it intact. Bytes read from a file are given as they are to `crypto` for a hash, to `compress`, to `http` as a body, to `net` to write and to `blobs`, and each takes them; bytes that any of these hand back are the same type.

**R-plug-20** Something that stays open, such as a connection, is an object with its calls and a `close`.
*Accept:* a raw connection and a `websocket` connection each have `close`. After `close`, the other end sees the connection closed and a further call on the object fails.

**R-plug-21** An error is one shape, with a short code from a short list: not found, refused by the other side, timed out, not permitted, bad input, and failed for anything else.
*Accept:* reading a file that is not there gives not found; a server that turns a connection away gives refused by the other side; a server that never answers gives timed out; a file the runner's user may not read gives not permitted; an unknown option gives bad input. Every error any plugin that comes with Corbie throws has the same fields and one of the six codes.

**C-plug-06** Every call that waits on the outside has a time by which it gives up, a default where the script gives none, and then fails as timed out. A script has one thread, so a wait with no end holds the job to its time limit and leaves nothing to say what it was waiting for. *Test:* `plug_every_wait_has_a_deadline`

**C-plug-07** What a script left open is closed by its plugin when the job ends, so nothing a job opens outlives the job. A script that throws never reaches its `close`. *Test:* `plug_left_open_closed_at_job_end`

## What comes with Corbie

**R-plug-25** The plugins that come with Corbie are `http`, `net`, `fs`, `exec`, `env`, `crypto`, `compress`, `html`, `xml`, `websocket`, `ssh`, `blobs`, `secrets`, `machines` and `llm`, and no others. Databases, message queues and mail are not among them.
*Accept:* the list of plugins the binary as it comes prints in its manual is exactly these fifteen.

**R-plug-26** Each gives a script what the overview's table says, and needs the kind its page names and nothing else: `http`, `net`, `websocket` and `ssh` need `net` for the host; `fs` needs `fs.read` or `fs.write` at the path's real location, and a path with no leading `/` is in the job's scratch directory; `exec` needs `exec` for the program; `env` needs `env` for the name; `crypto`, `compress`, `html`, `xml`, `blobs`, `secrets` and `machines` need no permission.
*Accept:* for each of the first eight, a call with the one entry its page names passes against a stand-in (a request with a header and a body reaches a server as written and its status, headers and body are read; a line and ten bytes go out and come back on a raw connection; a file is written, read back, listed and removed; `echo hello` gives `hello` and how it ended; `HOME` is read; a message of text and one of bytes go each way on a `websocket`; a command runs on another computer) and the same call without the entry ends the job denied. For the other seven, in a job no layer allows anything: a hash and a keyed hash of `abc` equal the known answers, sixteen random bytes come twice and differ, a signature checks as good and as bad once a byte changes, what was encrypted decrypts to the same bytes; bytes packed and unpacked are the same, and bytes packed by the usual tool outside Corbie unpack; `html.parse("<title>T</title>").find("title").text` gives `T` and an `xml` document gives an element's text and an attribute; a blob is made from bytes, read back whole and in part, and is in the instance's record; `secrets.get("K")` gives a handle; a flow is defined and a job in the parent names it and runs it.

**R-plug-30** `http` gives a script uploads, redirects it follows or not, and large bodies into blobs.
*Accept:* A form with one field and one file, given as bytes and again as a blob, reaches the server as an upload with the field, the file's name and every byte. A server answers with a redirect: told to follow, the script gets the answer at the place redirected to; told not to, it gets the redirect itself, with its status and the place it points to. An answer larger than the job's memory limit is received into a blob, the script gets the reference, and the blob holds every byte.

**R-plug-32** A `net` connection is plain or encrypted.
*Accept:* a script opens a plain connection to one port and an encrypted one to another. On the encrypted one the server's certificate is checked against the name asked for, and a server showing a certificate for another name is turned down.

**R-plug-41** A `websocket` connection is kept alive while the script works.
*Accept:* the server asks every second whether the client is still there, and sends three messages, while the script spends ten seconds in other calls. Afterwards the connection is open and the script reads the three messages in order.

**R-plug-42** `ssh` has no kind of its own: what it signs in with is a secret bound to that computer.
*Accept:* against a stand-in server, a script allowed `net:build.example` and `secret:BUILD_KEY@net:build.example` runs a command and gets what it printed and how it ended. Without either entry the job ends denied, the refusal naming the missing one.

**C-plug-09** `http` follows redirects itself, one at a time, and takes each secret's value afresh for each place it is redirected to, so a request carrying a secret is followed only to a host the secret is also bound to. The server would otherwise choose where the secret goes next. *Test:* `plug_http_secret_not_carried_to_another_host`

**C-plug-10** `http` follows a redirect only to another web address, and twenty at most. A redirect to another kind of address reaches for something no web request was allowed, and two servers can redirect to each other for ever. *Test:* `plug_http_redirect_kinds_and_limit`

**C-plug-11** A host that has not answered an `http` request in ten seconds fails as timed out, unless the script gives another time, and the limit covers the whole of the request, the body included. A request with no limit of its own waits until the job's, and a server that sends its headers at once and its body a byte a minute would hold the job. *Test:* `plug_http_default_time_limit`

**C-plug-13** Where a script gives `http` the certificates to trust, only those are trusted, at every place the call is redirected to, the name on a certificate is still checked, and a certificate that cannot be read is refused, not passed over. A redirect that fell back to the computer's own certificates would undo the script's choice, and a call that went ahead trusting fewer certificates than were written would hide the mistake. *Test:* `plug_http_given_certificates_every_hop`

**C-plug-17** A body on a request that only fetches (`GET`, `HEAD`) is refused. Servers and what stands in front of them drop it or turn the request away, and nothing tells the script. *Test:* `plug_http_no_body_on_fetch`

**C-plug-18** An address that holds a user name or a password is refused, and neither the refusal nor any later error repeats the address. A login in an address shows in every error and in the record. *Test:* `plug_http_address_with_login_refused`

**C-plug-19** `http` sends a script's headers as written, with a secret or without. A header dropped or changed out of sight gives the script an answer to a request it did not make. *Test:* `plug_http_headers_sent_as_written`

**C-plug-20** A body that has to be held whole, to be handed to the script, has a largest size, and one past it fails with the advice to send it into a blob. A body going into a blob is passed on as it arrives, whatever its size. A job has only its own memory, and one large answer held whole takes all of it. *Test:* `plug_http_held_body_bounded`

**C-plug-21** In an upload, a line break or a quote in a field's name or a file's name is escaped, and one in a part's type is refused. Written as given, it would start a header of its own inside the upload. *Test:* `plug_http_upload_names_escaped`

**C-plug-22** A `net` connection is not turned from plain to encrypted while bytes that arrived in the clear are still unread. They would be taken for part of the protected conversation. *Test:* `plug_net_no_encrypting_over_unread_bytes`

**C-plug-23** A read on a `net` connection that runs out of time ends the wait and not the connection, and loses nothing that had arrived. A script that looks in on a quiet connection must not have to open it again. *Test:* `plug_net_read_time_limit_keeps_connection`

**C-plug-24** Removing a link with `fs` removes the link and not what it points to. Removing through a link takes away a directory nobody named. *Test:* `plug_fs_remove_link_not_target`

**C-plug-25** Writing over a file with `fs` leaves the file's mode as it was. A file only its owner could read would otherwise become readable by everyone. *Test:* `plug_fs_write_keeps_mode`

**C-plug-26** Removing a directory and all inside it checks `fs.write` for every entry, and removes nothing if one is refused. Removing a parent must not reach past a deny inside it. *Test:* `plug_fs_remove_checks_every_entry`

**C-plug-28** A program named by a path with no leading `/` is found from the job's scratch directory and never from the root of the disk. A path joined to an empty directory name becomes a path from the root, and `bin/x` then means `/bin/x`. *Test:* `plug_exec_relative_program_from_scratch`

**C-plug-29** `ssh` checks the other computer's key against one the script gives; a call that gives none fails, unless it says outright that the key is not to be checked, and when the key shown is not the one given, the failure shows the fingerprint of the key that was shown. Trusting whatever key is shown first hands the sign-in to whoever answers, and the reader needs the fingerprint to tell a changed key from a wrong computer. *Test:* `plug_ssh_host_key_given_or_waived_outright`

**C-plug-31** `ssh` takes a key or a password only as a handle, and refuses one given as plain text; a password is offered both as a password and as the answer to a question the server asks. A sign-in written into a script is in the record for every reader of it, and many servers take a password only the second way. *Test:* `plug_ssh_sign_in_as_handle_both_ways`

## Extensions

**R-plug-48** An extension is a Go module compiled into the binary. It can offer any of: plugins, libraries written in script, a machine type, a model service.
*Accept:* for each of the four, a binary is built with an extension that offers that one thing, and the thing is used by the ordinary path: a script calls the plugin, a script loads the library by its bare name, a definition of the type runs, and an agent names the service.

**R-plug-49** Every plugin that comes with Corbie is built the same way, through the same public package, and uses nothing an extension could not.
*Accept:* a check over Corbie's source finds that each plugin that comes with Corbie uses the public package and no part of Corbie outside it. The check fails when one does.

**R-plug-50** A binary with extensions is built with Go from Corbie's source plus the modules wanted, and each extension carries its pages of the manual.
*Accept:* a Go build that names Corbie and one extension kept in a repository of its own gives a binary; its `corbie version` lists the extension, `corbie manual` lists the extension's pages, and `corbie manual <page>` prints one.

**R-plug-52** Runners need not all be alike: a job that calls a plugin its runner does not have fails, saying which, and tags are how such a job is sent to a runner that has it. Corbie does no matching of its own between jobs and what runners hold.
*Accept:* of two runners, the one whose binary has `pg` is enrolled with the tag `pg`. A job tagged `pg` always goes to it and runs. A job with no tag may go to either, and on the other fails with an error that names `pg`.

**R-plug-54** An extension cannot add a kind of permission.
*Accept:* the public package gives an extension no way to name a seventh kind, and in a binary with extensions an entry of a kind outside the six is still refused (R-perm-01).

## Interfaces

- **Permissions** check every way out but the job's own channel, and note what was used. Plugins make no check and add no kind.
- **Secrets** go into a way out where they are bound to go, and what a plugin hands back as text is masked on its way to the script.
- **Runner** runs the script, gives it `require`, `emit` and its scratch directory, starts and stops what a job starts, puts the exec wrapper in front of every program (C-run-49), and fails a job that calls a plugin its binary does not have.
- **Machines** fill out, check and store a definition the `machines` plugin is given.
- **Blobs** are made and read by the `blobs` plugin, in the job's own instance's record, and are where `http` sends a large body.
- **Models** are the `llm` plugin and the model services behind it.
- **Agent** uses the bundled libraries: the standard turn and the tools for blobs.
- **Controller** sends a tagged job to a runner with that tag.
- **Store**, **Roles** and **Machines** take a kind of store, a serverless host and a machine type from an extension.
- **Client** lists a binary's extensions with `corbie version`.
- **Manual** holds each plugin's and each library's page, makes the list of plugins from what the binary holds, and runs every page's examples as tests.
