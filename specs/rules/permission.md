# Permissions: rules

The detailed rules behind the [Permissions overview](../permission.md). Requirements are `R-perm-nn`, constraints `C-perm-nn`.

## Writing one

**R-perm-01** A permission is written `kind:scope`, and the kinds are `fs.read`, `fs.write`, `net`, `exec`, `env`, `secret`, `send` and `at` and no others. An entry of any other kind is refused where it is written, and so is an `at:` entry anywhere but a client's ceiling.
*Accept:* a definition with `allow: [ssh:build.example]` fails its check, and the report names the entry; setting a ceiling to the same entry is refused by the command. `at:/home` fails the check in a definition's `allow:`, `deny:` and `ask_about:`, and is refused in a start's list and a runner's ceiling; a client's ceiling takes it.

**R-perm-02** Each kind allows what the overview's table says and takes the scope it gives: `fs.read` reading files at a path, `fs.write` writing files at a path, `net` connecting to a host, with a port if it matters and every port if none, `exec` starting a program by its name or its path, `env` reading a variable of the runner's by name, `send` sending to or calling a location, `at` submitting a tree at a prefix.
*Accept:* a job allowed only `fs.read:/srv/reports/**` reads `/srv/reports/q3.csv`, is refused `/srv/other/a`, and is refused writing `/srv/reports/q3.csv`. One allowed only `fs.write:/srv/out/**` writes `/srv/out/a.md` and is refused reading it. One allowed only `net:api.github.com:443` connects to that host on port 443 and is refused port 8443 and `example.com`; one allowed `net:api.github.com` connects on ports 22, 443 and 8443. One allowed only `exec:git` starts `git` and is refused `curl`; one allowed only `exec:/opt/tools/deploy` starts that file and is refused `/opt/tools/other`. One allowed only `env:HOME` reads `HOME` and is refused `PATH`. A run allowed only `send:/work/services/lookup` calls that location and is refused a send to `/work/services/other`. A client whose ceiling holds `at:/home` submits a tree at `/home` and is refused one at `/work`.

**R-perm-09** A scope is a path, a host, a name or a location, and each of the four is matched the same way wherever it appears.
*Accept:* for the path `/srv/*/log`, the host `*.example.com` and the name `APP_*`, the values matched are the same whether the scope stands in a definition's allow list, in a deny list, in a start's list, in either ceiling, or after the `@` of a secret's entry. In a tree at `/home`, `thermostats/*` matches the same locations in a `send:` entry and in a `receive:` entry, and so does `/home/thermostats/*` in a client's ceiling.

**R-perm-10** Any scope may use `*` for a part. A `*` stands for any run of characters inside one part and never crosses into the next: a part of a path lies between two `/`, a part of a host between two `.`, and a name is one part. A location's parts lie between two `/`, as a path's do. In a path and a location, `**` stands for any number of parts. A host of `*` alone matches every host, name or address.
*Accept:* `fs.read:/srv/*/log` passes `/srv/a/log` and refuses `/srv/a/b/log`; `fs.read:/srv/**` passes both; `send:/home/thermostats/*` passes `/home/thermostats/kitchen` and refuses `/home/thermostats/a/b`; `send:/work/**` passes `/work/a/b`; `at:/home` passes `/home` and refuses `/home2` and `/home/a`; `net:*.example.com` passes `api.example.com` and refuses `example.com` and `a.b.example.com`; `net:*` passes `a.b.example.com` and `10.0.0.5`; `env:AWS_*` passes `AWS_REGION`.

**R-perm-11** A path is checked where it really points, after links are followed.
*Accept:* `/srv/reports/latest` is a link to `/etc/passwd`. A job allowed `fs.read:/srv/reports/**` is refused when it reads `/srv/reports/latest`, and the refusal names `/etc/passwd`. A job allowed `fs.read:/etc/passwd` reads it through the link.

**R-perm-12** A path scope with no leading `/` names a place in the job's scratch directory, as a path a script writes with no leading `/` does (see Runner), and the scratch directory needs an entry like any other directory.
*Accept:* a job allowed only `fs.write:out/**` writes `out/a.txt`, which lands in its scratch directory; writing `/tmp/a.txt` is refused, and so is writing `other/a.txt`. A job allowed nothing is refused a write into its own scratch directory.

**R-perm-13** The groups are `network` (`net`), `disk` (`fs.read` and `fs.write`), `exec` (`exec`) and `env` (`env`), and a group allows every scope of its kinds; `send` and `at` are in no group; `all` allows everything, secrets, `send` and `at` included, and means the same in every layer.
*Accept:* a job allowed `network` connects to any host on any port; one allowed `disk` reads and writes any path; one allowed `exec` starts any program; one allowed `env` reads any variable. Under a ceiling of all four groups, a job whose definition names `secret:K@*` has the sending of `K` refused by the ceiling; under a ceiling of `all` it sends `K`. Written in a definition, a start's list, a parent's job, the client's ceiling or the runner's ceiling, `all` passes the same calls: any read, write, connection, program, variable, and any secret to any destination. No group but `all` passes a send to a location or a submit at a prefix.

**R-perm-61** A location in a definition is read from the tree's prefix by default, and in full when it starts with `/`; `../` is refused. In a ceiling and a start's list a location starts with `/`.
*Accept:* in a tree at `/home`, `send:thermostats/*` passes a send to `/home/thermostats/kitchen` and refuses one to `/work/thermostats/kitchen`, and `send:/work/**` passes a send to `/work/a`. `send:../work/a` fails the definition's check. `--ceiling send:thermostats/*` is refused by the command, and `--ceiling send:/home/thermostats/*` is taken.

**C-perm-01** The fixed leading part of a path scope is resolved through links as the value is, and the part with `*` in it is not. A scope written through a link otherwise never matches the real location of anything under it, and a link that a `*` happens to match must not carry the scope somewhere else. *Test:* `perm_scope_leading_part_resolved`

**C-perm-02** Every kind that names a path resolves it the same one way: `fs.read`, `fs.write`, `exec` by path, and a secret's file or program destination. Two ways of resolving disagree at the edges, and a path then passes one check and not the other. *Test:* `perm_one_path_rule_for_every_kind`

**C-perm-03** The characters `*` and the like in a directory's real name are taken as they are when a scope is resolved. A directory called `x*` would otherwise widen the scope it is part of. *Test:* `perm_resolved_name_is_not_a_pattern`

**C-perm-25** A link to a place that does not exist is followed to that place. Writing the link makes the file there, so the check is made there. *Test:* `perm_dangling_link_followed`

**C-perm-05** A host is matched in one spelling: lower case, no trailing dot, an IPv4 address written inside an IPv6 one taken as the IPv4 address, and an address's zone dropped. Another spelling of a denied host otherwise slips past the deny. *Test:* `perm_host_one_spelling_before_match`

**C-perm-06** A host name with characters other than letters, digits, `.`, `-` and `_` is refused before anything is looked up. What is not a host name must not be handed to a resolver. *Test:* `perm_odd_host_name_refused_before_lookup`

**C-perm-07** A host scope written as a whole address, with a scheme or a path, is refused where it is written, with the advice to write the host alone. Matched as a host it matches nothing, and the entry silently allows nothing. *Test:* `perm_host_scope_not_an_address`

**C-perm-09** A program's name and a program's path never pass for each other: `exec:git` does not allow `/tmp/x/git`, and `exec:/usr/bin/git` does not allow the bare name `git`. A file with an allowed program's name could otherwise be started from anywhere. *Test:* `perm_program_name_is_not_a_path`

**C-perm-26** On Windows a path scope has a drive or a share. One with a leading `/` and no drive matches nothing: a definition written for Linux allows less on Windows, never more. *Test:* `perm_windows_path_has_a_drive`

**C-perm-27** On Windows a path and a program's name match in any case, and a program's name matches with or without `.exe` or `.com`. Windows opens and starts the same file by every one of those spellings, so a deny entry written one way would let the others through. *Test:* `perm_windows_name_in_any_case`

**C-perm-30** A location is matched as a name in the store, never as a path on the disk: no link is followed, and case counts on every system. Two spellings a disk takes for one place are two locations, and a location matched through the disk would match differently on each computer. *Test:* `perm_location_is_not_a_path`

**C-perm-31** No part of a location, or of a pattern of locations, is empty, `.`, `..` or `+` or holds `#`, where it is written, when it is read from a prefix and when it is sent to; a location itself (a tree's prefix, a location sent to, one a secret or value is set at) holds no `*`. `..` would let a location read from one tree's prefix name another tree's, `+` names a location's own entries in the store and `#` separates a key from its location, a prefix holding `*` would make every location read from it a pattern over other trees, and a secret or value set at a location holding `*` would never apply. *Test:* `perm_location_climbs_nowhere`

**C-perm-10** Everything is allowed in one way, by `all` in a list. No flag or setting does the same. A second way is a second place to look when asking what a job may do. *Test:* `perm_all_only_in_a_list`

## Layers

**R-perm-16** A definition gives each script it runs its permissions: the lists written for the whole definition, and with them the lists written for that job.
*Accept:* a definition allows `net:a.example` for the whole and one of its two jobs allows `fs.read:/srv/**` besides. That job connects to `a.example` and reads `/srv/x`. The other job connects and is refused the read.

**R-perm-17** A start may carry a list of its own, which narrows the root and everything below it. A start that carries none adds no layer.
*Accept:* a definition allows `network`, `fs.read:/srv/**` and `exec:git`. Started with `allow: [network, fs.read:/srv/reports/**]`, its job reads `/srv/reports/a`, is refused `/srv/other/a`, and is refused `git`, each refusal naming the start. A job of a child machine three levels down is held to the same list. Started with no list, the job reads `/srv/other/a` and starts `git`.

**R-perm-18** A parent may narrow a child with a list on the job that starts it, and so may every parent above. A job that carries none adds no layer: a child the definition names runs with what its own definition says.
*Accept:* a parent's job `{ machine: ./child.yaml, allow: [fs.read:/srv/reports/2026/**] }` starts a child whose own definition allows `fs.read:/srv/**`. The child's job reads `/srv/reports/2026/a` and is refused `/srv/reports/2025/a`, the refusal naming the parent's job. A grandchild is held to that list and to its own parent's. With no list on the parent's job, the child's job reads `/srv/reports/2025/a`, and it does so too when the parent's definition allows only `env:HOME` for the whole.

**R-perm-22** Every layer is the same thing, a list to allow and a list to deny, and that holds for a definition, a start, a parent's job, the ceiling of the client that owns the run's tree and the runner's ceiling; the ceilings are given with trust (see Identity, R-id-24).
*Accept:* the lists `allow: [disk]`, `deny: [fs.read:/srv/private/**]` are written in turn as the definition's, the start's, the parent job's, the client's ceiling and the runner's ceiling, with every other layer `all`. In each case reading `/srv/a` passes and reading `/srv/private/a` is refused by that layer.

**R-perm-23** In a layer, what the deny list matches is refused whatever the allow list says, what the allow list matches passes, and anything neither list matches is refused; neither the order of entries nor how exact an entry is changes that.
*Accept:* a layer with `allow: [fs.read:/srv/private/key]`, `deny: [fs.read:/srv/**]` refuses reading `/srv/private/key`, with the two lists written in either order. A layer with `allow: [fs.read:/srv/**]` and no deny passes reading `/srv/a` and refuses reading `/etc/hosts` and connecting anywhere.

**R-perm-25** A job may do something only if every layer passes it, so a layer can narrow what is above it and can never widen it.
*Accept:* the definition's job allows `network`, `fs.read:/srv/**`, `exec:git`; the start `network`, `fs.read:/srv/reports/**`; the client's ceiling `network`, `disk`; the runner's ceiling `disk`, `exec`. The job reads `/srv/reports/a`. Reading `/srv/a` is refused by the start, a connection by the runner's ceiling, and `git` by the start and the client's ceiling. A child's definition allows `all` under a parent's job that allows `fs.read:/srv/**`: the child's job is refused a write to `/srv/a` by the parent's layer.

**R-perm-27** The scope of a definition's allow entry may be filled from the machine's input or from a value set at the run's location; an input can choose a place within a limit and never move the limit, since every layer above still applies.
*Accept:* a job allows `fs.read:${input.dir}/**` and the machine is started with `dir: /srv/reports`: the job reads `/srv/reports/a` and is refused `/srv/other/a`. A job allows `net:${values.HUB}` and `HUB` is set to `hub.local` at the run's location: the job connects to `hub.local` and is refused `other.local`. With the start narrowed to `fs.read:/srv/reports/**` and `dir: /etc`, the job is refused reading `/etc/hosts`, the refusal naming the start.

**R-perm-28** A filled scope reads nothing but the input of the machine whose definition holds the entry and the values at its location; an expression anywhere else in a permission is refused when the definition is checked.
*Accept:* `fs.read:${stages.a.output.dir}/**` in a flow, `fs.read:${args.dir}/**` in an agent's tool and `deny: ["fs.read:${input.dir}/**"]` each fail the check, the report naming the entry; so does an expression in a start's list or a ceiling. `fs.read:${input.dir}/**` in an agent's tool passes and is filled from the agent's input, and `net:${values.HUB}` passes and is filled from the value.

**C-perm-12** A job carries its layers apart, each under its name, and the runner adds its own. Nothing is merged or narrowed before the script runs. A merged list cannot say which layer refused, and narrowing one list of patterns by another is where mistakes are made. *Test:* `perm_layers_travel_apart`

**C-perm-23** A machine made while running is always narrowed by the lists on the job that starts it, and with none it is allowed nothing. A script or a model wrote it, and a job allowed nothing could otherwise write a definition that allows everything and have it run. *Test:* `perm_made_machine_gets_only_its_jobs_list` and `perm_definition_stored_as_a_blob_is_checked_and_narrowed`

**C-perm-13** A deny entry that cannot be read refuses everything in its layer. It could have been meant to deny anything. *Test:* `perm_unreadable_deny_refuses_all`

**C-perm-14** A scope is not filled from an input or a value that is missing, empty, not text or a number, or itself holds `${`. The job fails before anything is sent, naming the entry. An empty value turns `${input.dir}/**` into `/**`, the whole disk, and a value holding an expression would be read a second time. *Test:* `perm_filled_scope_refuses_missing_empty_nontext_nested`

## Where a run's authority comes from

**R-perm-62** A run's authority comes from its location: its file's `allow:` and `ask:`, the ceiling of the client that owns its tree, and the secrets and values set at its location or above, the nearest winning. A start's list, a parent's job list and the runner's ceiling narrow it.
*Accept:* `/work/services/lookup` allows `net:db.internal:5432`, the work client's ceiling holds `network`, and `DB_PASSWORD` is set at `/work`. A run of it started by the work client and one called from `/shop/helper` each connect to `db.internal:5432` with the same `DB_PASSWORD`, and each is refused a connection the work client's ceiling does not allow, whatever the shop client's ceiling holds.

**R-perm-63** A sender or a caller adds nothing to a run's authority and takes nothing from it; the input it gives may fill an allow scope, within the receiver's own patterns and every ceiling.
*Accept:* a caller whose own layers allow `all` calls a location whose file allows `fs.read:${input.dir}/**`, with `dir: /srv/reports`: the called run reads `/srv/reports/a` and is refused `/etc/hosts`. A caller allowed only `send:` entries calls the same location, and the called run reads `/srv/reports/a`.

**R-perm-64** A child started by `machine:` runs at its parent's location. A machine made while running runs at the location of the job that starts it, and sends only to its parent and its children.
*Accept:* a child of `/home/devices/boiler` reads the secrets set at `/home/devices`. A made machine whose layers allow `send:/home/**` has a send to `/home/devices/boiler` refused, and its send to its parent is delivered. Parity case 133.

**R-perm-65** A tree belongs to the client that first submitted it: only that client submits there, reads the runs at its locations and cancels them, and a client submits trees only at prefixes its ceiling's `at:` entries match.
*Accept:* the work client, with `at:/work` in its ceiling, submits a tree at `/work`. The shop client's submit at `/work` is refused, naming the work client, and its cancel of a run at `/work/services/lookup` is refused. The work client's submit at `/shop` is refused, naming its ceiling and the line `at:/shop`.

## Sending and calling

**R-perm-66** A send or a call needs a `send:` entry that matches the receiver's location in every layer of the sender, the ceiling of the client that owns its tree included and the runner's ceiling not. A send the sender's layers refuse is `denied`.
*Accept:* a flow at `/shop/helper` whose call job allows `send:/work/services/lookup`, under a shop client's ceiling without it, fails `denied`, the refusal naming the ceiling and the line `send:/work/services/lookup`. With the line added to the ceiling the call is made, though no runner's ceiling holds a `send:` entry.

**R-perm-67** The receiver's `receive:` says who may send to it or call it. With none, it accepts every location of its own tree and the client that owns the tree. A written list replaces that whole; a mapping gives each event its own list, and an event it does not name, and a call, are accepted from no other sender. An entry is a location, read as in a definition, or `client:<name>`, and matches the sender's location or the client that sent, never a run's ID. A send the receiver refuses is noted as undelivered in the sender's record, and nothing fails.
*Accept:* `/work/services/lookup` writes `receive: [agents/*, /shop/helper]`. A call from `/work/agents/support` and one from `/shop/helper` are taken; a call from `/shop/other` is refused, and its call job fails while the lookup is unchanged. A chart with `receive: { set: [thermostats/*], stop: [client:panel] }` takes `set` from `/home/thermostats/kitchen` and `stop` from the client `panel`, and `stop` from `/home/thermostats/kitchen` is noted as undelivered in that run's record, which goes on. Parity case 124.

**R-perm-68** The client that owns a tree sends to and calls the runs at its locations with no `send:` entry, whatever their `receive:` says. A parent run and its children send to each other with no `send:` or `receive:` entry.
*Accept:* the work client, with no `send:` entry in its ceiling, sends to a chart at `/work/a` whose `receive:` lists only `client:panel`, and the event is delivered. A child sends to its parent, whose `receive:` lists nobody, and its parent sends back, with no `send:` entry in either's layers.

## Checking

**R-perm-30** The check is made at the moment a script acts, and not before.
*Accept:* a job no layer allows anything runs a script that only computes, and ends done. A script that emits and then reads a file it is not allowed has its emit in the record and then ends denied. A job whose definition allows something its ceilings do not is sent and runs until the script tries that thing.

**R-perm-31** The check is made in one place, by Corbie, and never by a plugin: when a plugin asks for a way to connect, a path, a program or a variable, Corbie checks every layer, notes what was used, and hands the thing over or refuses, so the same thing tried through any plugin gets the same answer.
*Accept:* with no entry for `host.example`, a script reaches for it through `http`, `net`, `websocket` and `ssh` in four jobs: all four are refused, and each refusal has the same three parts and names the same line, `net:host.example`. A job allowed `fs.read:/srv/a` and `env:HOME` reads both and tries to start `git`: its result lists the two as used, and the refusal for `exec:git` is its failure.

**R-perm-33** A connection is checked by the name asked for and again by the address the name resolves to, and only an address that was checked is connected to. In each layer a deny that matches either refuses it, and an allow that matches either passes it.
*Accept:* a layer allows `net:*.internal.example` and denies `net:169.254.169.254`. A connection to `meta.internal.example`, which resolves to `169.254.169.254`, is refused. A layer that allows only `net:10.0.0.5` passes a connection asked for by a name that resolves to `10.0.0.5`. A name resolves to two addresses, one of which a layer denies: the job is never connected to the denied one, whatever order the addresses come in.

**R-perm-35** Where a service hands out further addresses, each is checked the same way as the first.
*Accept:* a job allowed `net:a.example` asks for a page at `a.example`, which answers with a redirect to `b.example`. Following it is refused, and the refusal names `net:b.example`.

**C-perm-15** No name is looked up for a connection no list could allow, and a denied name is refused before it is looked up. A lookup is itself a message that leaves the runner, and it carries the name. *Test:* `perm_no_lookup_before_check`

**C-perm-16** A name that passes only because one of its addresses is allowed is connected to at that address and no other. Allowing one address must not open every address the name has. *Test:* `perm_address_allow_reaches_only_that_address`

**C-perm-17** Every address a name resolves to is checked, and the name is not looked up a second time between the check and the connection. A name can resolve differently the second time. *Test:* `perm_connects_only_to_what_was_checked`

**C-perm-24** A host scope written as an address, whole or with `*` for parts of it, matches an address and never a name. `*` alone is not written as an address. `10.x.example.com` has the parts of `10.*.*.*`, and a name that passes is connected to at whatever it resolves to. *Test:* `perm_address_scope_matches_only_an_address`

## Refusals

**R-perm-36** A refused call stops the job with the status denied. Nothing in the script after the call runs, and the script cannot catch the refusal and carry on.
*Accept:* a script wraps a read it is not allowed in `try` and `catch` and emits afterwards. The emit is never written, and the job's result is denied.

**R-perm-37** Every refusal says three things: what was tried, as the kind and the value that was checked; which layer refused it, every one where more than one does; and what would fix it, down to the line to add and where, or the command to run, naming the deny entry where one is what refused.
*Accept:* a job reads `/srv/reports/latest`, a link to `/etc/passwd`, without leave: the refusal says `fs.read:/etc/passwd`. A refused send says the location in full, as `send:/home/thermostats/kitchen`, a line that pastes into a definition and a ceiling alike. The same read is refused in five jobs, each by a different one of the five kinds of layer (the job in its definition, the start, a parent's job and which, the ceiling of the client that owns the tree, the runner's ceiling), and each refusal names that layer; in a sixth job the start and the client's ceiling both refuse, and the refusal names both. A read refused by a job's definition gives the line `fs.read:/srv/a` and names the job whose list it goes in, and pasting the line there and starting again, the read passes. A read refused by a definition's `deny: [fs.read:/srv/**]` names that entry.

**R-perm-40** The fix for a ceiling names the command and the one line, and never shows the rest of the ceiling.
*Accept:* a client's ceiling holds ten entries and none for `net:b.example`. The refusal of a connection there holds the command that sets that client's ceiling and the entry `net:b.example`, and none of the ten. The same holds for the runner's ceiling.

**R-perm-41** A denial passes up through parent machines unchanged.
*Accept:* a job three machines down is refused, and no machine has a rule for the failure. The root fails as denied, and its refusal is word for word the one the runner wrote.

## Asking

**R-perm-42** A definition may list permissions that must be asked about. Before a job in that machine starts, the permissions the job declares, its scopes filled, are read against the list, and a job that declares one of them is held for the ask. A job declares a listed permission when one of its allow entries has the same kind and either scope covers the other, or is a group containing it, or is `all`; a listed group stands for every scope of its kinds.
*Accept:* a machine lists `fs.write:/srv/out/**`. A job that allows `fs.write:/srv/out/a.md` is held, and so is one that allows `fs.write:/srv/**`, one that allows `disk`, and one that allows `all`. A job that allows only `fs.read:/srv/**` starts with no ask. A machine lists `exec`, and a job that allows `exec:git` is held.

**R-perm-44** The list applies to every job in the machine that writes it and in every machine below it, and a job held this way is held exactly as a job marked for approval is (see Machines, R-mach-91).
*Accept:* a root lists `exec:deploy`. A job two machines down that allows `exec:deploy` is held, and it goes to the nearest ask above it, together with a job marked for approval from the same step.

**R-perm-46** Whether a job is held depends on what it declares alone, before anything is sent for it.
*Accept:* a job that allows `network` is held although its script never connects, and while it is held no runner's inbox holds it. A job that declares nothing listed is not held, and when its script tries the listed thing it is refused at the act like anything else it did not declare.

**R-perm-47** Asking only ever holds: an approved job gets exactly what its layers already allowed, nothing a layer refuses can be had by asking, and a job never stops halfway to wait for a person.
*Accept:* a machine lists `fs.write:/srv/out/**`, a job allows it, and the runner's ceiling does not. The job is held, the ask approves it, the job is sent, and its write is refused by the runner's ceiling. A running job that tries something no layer allows is refused at once and ends denied; no ask is started for it, and it is never seen waiting.

**R-perm-69** A run started by a send or a call has its held jobs go only to its own file's `ask:`, and they are denied if it has none. A call and a `job: { send }` are jobs, so the sender's `ask_about: [send:…]` holds them. A statechart's send action is never held.
*Accept:* parity cases 127 and 128. A called location with no `ask:` whose job is marked for approval is refused as it starts, and the call job fails, though its caller has an ask (`Test_ctl_called_runs_held_jobs_go_to_its_own_ask`). `Test_ctl_ask_about_send_holds_a_call` and `Test_ctl_ask_about_send_holds_a_send_job`. A flow lists `ask_about: [send:/work/**]`, and its job `{ call: { at: /work/services/lookup } }` is held for its ask; a statechart with the same list sends to `/work/a` with no ask started.

## What a job used

**R-perm-50** Every job's result lists the permissions the job used, however the job ended: what was used and nothing else, not what was allowed and never tried, and not what was refused.
*Accept:* a script reads `/srv/a`, connects to `api.github.com` on port 443, is refused a program and then throws. Its result is an error, and it lists an `fs.read` entry for `/srv/a` and a `net` entry for that host, and no `exec` entry. A job allowed `disk` and `network` that reads one file has a list holding the one read and no `net` entry.

**R-perm-52** What a job used can be written into the definition in place of something looser.
*Accept:* a job with `allow: [all, secret:K@*]` ends done. The entries its result lists are pasted in place of both, and the job started again with the same input ends done.

## A job allowed nothing

**R-perm-53** A job with no permissions still has its input and its result, plain computation, making and reading blobs in its own instance's record, what it emits, and defining machines.
*Accept:* a job no layer allows anything reads `input`, calls `emit`, `log` and `progress`, makes a hash, packs and unpacks bytes, reads a page and a document as trees, makes a blob, reads it back, reads a blob given in its input, defines a machine, and returns the blob's reference and the machine. It ends done with that output, the emits are in the record, and the blob is in its instance's record.

**R-perm-56** A job with no permissions has nothing else.
*Accept:* five jobs no layer allows anything each try one thing: read a file, connect, start a program, read a variable, send a secret. Each ends denied.

## What Corbie does not defend against

**R-perm-58** A job cannot listen for connections.
*Accept:* no call on any plugin's page waits for a connection to come in, and the public package gives an extension's plugin no way out that accepts one.

**R-perm-60** Corbie says what it does not defend against: the manual states each of the six. A program a job starts does its own reading, writing and connecting as the runner's user, and the permissions govern the start and nothing after. A job that may read the runner's own files can read the runner's key, and with it every job sent to that runner. A script allowed to reach a secret's destination can learn the secret from the other side. A plugin is trusted, and only review keeps one from opening something by itself. Whether a connection is encrypted is the plugin's word where a service's own protocol does it. Jobs on one runner are not kept apart, and jobs that must be are sent to different runners.
*Accept:* the manual's pages on permissions hold a sentence for each of the six.

## Interfaces

- **Machines** supply each job's lists, the filling of scopes from input and values, the list of permissions to be asked about, the ask a held job goes to, and each file's `receive:`; what happens to a held job from then on is R-mach-91 to R-mach-115. Permissions tell them which entries are names and may hold no expression, and read and match the locations in addresses and `receive:` (`ReadLocation`, `MatchLocation`).
- **Controller** builds a job's layers from the definition, the start, each parent's job and the ceiling of the client that owns the run's tree, sends them apart and by name, and holds the jobs that declare a listed permission. It checks `send:` against the sender's layers for a send action and for send and call jobs, with the tree's prefix and never the runner's ceiling, checks the receiver's `receive:`, and checks `at:` against the client's ceiling at a submit.
- **Identity** is where a ceiling is given with trust and changed: a client's by the controller's owner, a controller's by the runner's owner; a party with no ceiling may do nothing outside the job (R-id-24). A client's ceiling carries its `at:` entries and is checked as one (`CheckCeiling`).
- **Runner** adds its own ceiling to the layers a job carries, gives the job its scratch directory, writes the refusal or the list of what was used into the job's result, keeps nothing from a job on any ground but these layers, and stops what a job started when it ends (R-run-19).
- **Plugins** reach outside only through the ways out, and each way out is where a check is made and a use is noted (R-plug-02); what a script leaves open is closed at the job's end (C-plug-07).
- **Secrets** supply the values that may fill a scope, and use the `secret` kind and its rule for a secret's name: its destination, which values a job is sent (R-sec-14), that being allowed a secret is not being sent it, and the check that every secret in a handle may go where it is going.
- **Client** carries a start's narrowing list, and shows a run's `denied` with its refusal and the permissions a finished run used.
- **Manual** carries the statements of what is not defended, and each plugin's page says which kinds its calls need.
