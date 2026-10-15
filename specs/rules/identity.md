# Identity

The detailed rules behind the [Identity overview](../identity.md).

## Identities and cards

**R-id-01** An identity is two private keys, one for signing and one for decrypting, kept in a file only its owner can read.
*Accept:* after `corbie runner card` on an empty config, the runner's place in the config holds an identity file that the system reports as readable and writable by its owner alone; with that file and nothing else the runner signs a message and opens one sealed to it.

**R-id-02** A party's card is one line of text holding its name, whether it is a controller's, a runner's or a client's, and its two public keys, signed by itself; a card whose signature does not hold is refused wherever a card is taken.
*Accept:* `corbie runner card` puts one line on standard output; read by `corbie controller enrol`, it gives the runner's name, says it is a runner's, and gives two public keys that match the private keys in the identity file. Change one character of the name inside a printed card and give it to `enrol`: the command fails saying the card is not valid, and no trust is written. A client's card given to `corbie runner trust`, which expects a controller's, is refused for not being one.

**R-id-04** A card has a short fingerprint, the same wherever and whenever that card is shown, and both sides of an enrolment show the fingerprint of each card that passes between them.
*Accept:* in the three steps across computers, `runner card` shows the runner's fingerprint, `controller enrol` shows the runner's and the controller's, and `runner trust` shows the controller's; the two showings of each card agree, and a second runner's card shows a different one.

**R-id-06** A party's name is chosen when its identity is made, and no command changes it afterwards. `corbie runner card` and `corbie client card` make the party's identity if it has none, and print its card.
*Accept:* on an empty config `corbie client card` prints a card; run a second time, also asking for another name, it prints the same card with the same name and keys.

**C-id-01** A fingerprint is made from both of a card's public keys. Made from the signing key alone it is the same for a card whose decrypting key has been swapped, and the two people comparing fingerprints see nothing wrong. *Test:* `id_fingerprint_changes_with_either_key`

**C-id-02** An identity file that anyone but its owner can read is refused, and the refusal says so. A key others on the computer can read is no longer that party's alone, and using it quietly hides the fact. *Test:* `id_identity_file_readable_by_others_refused`

**C-id-03** An identity file is written whole or not at all. A party stopped while its identity is being made must not be left with half a key under a name it has already used. *Test:* `id_identity_file_written_whole`

**C-id-04** A private key never appears in an error, a log line or any other output, also when the identity file is damaged or cannot be read. Errors are copied into logs and reports that others read. *Test:* `id_private_key_never_printed`

**C-id-05** Names inside a card, and inside anything else that is signed, are matched exactly, letter case included, and a value that gives one name twice is refused. A reader that takes `NAME` for `name` can be handed a value in which the two disagree: signed over one, read by the other. *Test:* `id_signed_value_names_matched_exactly`

## Enrolment

**R-id-10** Two parties trust each other only by each taking the other's card, and trust must be held on both sides before anything passes between them; nothing is trusted for where it runs or for the config it shares.
*Accept:* make a controller and a runner separately in one config and one store, with no `enrol` and no `trust`: the controller sends the runner no job, and the runner acts on nothing left in its inbox. A controller enrols a runner's card and the runner never runs `trust`: a job sent to that runner is not run. A runner trusts a controller that has not enrolled it: the controller sends it no job and accepts nothing it writes.

**R-id-12** `corbie init --store <address>` makes a controller, a runner and a client in the config, has the controller trust the runner and the client, has the runner and the client trust the controller, and prints what it did.
*Accept:* on an empty config and an empty directory, `corbie init --store <directory>`, then `corbie run controller runner`, then `corbie submit` of a script that returns 1: the run ends done with the output 1, and no other set-up command is needed. The output of `init` names the three parties it made, each with its fingerprint, the store's address, and the ceiling it gave.

**R-id-14** `--ceiling` on `corbie init` says what jobs on this computer may do: it is the ceiling the controller gives the client and the ceiling the runner gives the controller. Without it, jobs may do nothing outside themselves, and the first refusal names the command that changes that.
*Accept:* after `corbie init --ceiling disk`, a submitted script whose definition allows everything reads a file, and is refused a connection with a refusal that names a ceiling as the layer. After `corbie init` with no ceiling, a script that only computes ends done; a script that reads a file ends denied, and the fix its refusal gives is a command that sets the ceiling.

**R-id-16** `corbie controller enrol` takes a runner's or a client's card, writes the trust into the controller's store, and prints the controller's own card and the address of its store, which is everything the other side needs.
*Accept:* enrol a runner's card: the store holds a trust value for that runner, and standard output holds the controller's card and the store's address. After the three steps, `corbie run runner` on the runner's computer takes a job from that controller without the store's address or the controller's card being given to it again.

**R-id-18** Each of the three steps reads the output of the step before it from standard input when its argument is `-`, so the three join into one line.
*Accept:* `corbie runner card | corbie controller enrol - --tags laptop | corbie runner trust - --ceiling disk,exec`, run across two configs, leaves the same trust on both sides as the three commands run one at a time with their outputs carried by hand.

**R-id-20** A controller's identity is made together with the address of its store, by `corbie init` or by `corbie controller card --store <address>`; `enrol` needs the store to be reachable and nothing to be running.
*Accept:* on an empty config `corbie controller card --store <address>` prints a card, and a later `corbie controller enrol` works with no store named and no Corbie process running anywhere. With no identity yet and no store named, `corbie controller card` ends as a mistake in how it was written, saying the store's address is needed. With the store out of reach, `enrol` fails saying the store cannot be reached.

**R-id-22** A controller's trust is kept in the store; a runner's and a client's is kept in their own config.
*Accept:* a controller started on another computer with only the controller's identity file and the store's address accepts a message from a client enrolled before. `corbie runner trust` writes nothing to the store; afterwards the runner's config holds the controller's card, the store's address and the ceiling given.

**C-id-06** In a pipe, a step's standard output is exactly what the next step reads; fingerprints and notes for the person go to standard error. Anything else on standard output is read by the next step as part of a card. *Test:* `id_enrol_pipe_output_is_only_what_next_step_reads`

**C-id-07** `enrol` prints its card and the store's address only after the trust is written. Printed first, a write that then fails leaves the next step trusting a controller that does not know it. *Test:* `id_enrol_prints_nothing_when_trust_not_written`

**C-id-08** A step given empty input trusts nothing and says what it expected to read. When the step before it fails, a pipe hands on nothing, and nothing must not pass for an answer. *Test:* `id_trust_on_empty_input_trusts_nothing`

**C-id-09** The last step of the pipe reads its input to the end before it looks at its own config. The commands of a pipe start together, so the identity the first one makes may not exist yet when the last one starts. *Test:* `id_trust_reads_input_before_config`

**C-id-10** The store's address that `enrol` prints holds no credential of the controller's. Each party holds its own key to a bucket, and the controller's key gives its holder the controller's whole row of access. *Test:* `id_enrol_prints_address_without_credentials`

## What trust carries

**R-id-24** A controller gives each client a ceiling, and no run that client starts is allowed more; a runner gives each controller it serves a ceiling, and no job from that controller is allowed more on the runner's computer; a party trusted with no ceiling may do nothing outside the job itself.
*Accept:* a client enrolled with `--ceiling disk` starts a run whose definition allows `network`: the job's connection is refused, and the refusal names the client's ceiling. A runner trusts a controller with `--ceiling disk`; a job from it that every other layer allows to start a program is refused, and the refusal names the runner's ceiling. A client enrolled with no `--ceiling` starts a script that only computes: done; one that reads a file: denied, naming the client's ceiling. The same two results follow when it is the runner that trusted the controller with no `--ceiling`, naming the runner's ceiling.

**R-id-27** A controller gives each runner its tags, and they are the tags by which jobs choose that runner; a card carries none.
*Accept:* enrol one runner with `--tags laptop` and another with no tags: a job that names `laptop` goes to the first every time. `corbie runner card` takes no tags.

**R-id-29** A runner can serve several controllers, each with its own ceiling and its own store.
*Accept:* a runner trusts controller A with the ceiling `disk` and store 1, and controller B with the ceiling `exec` and store 2. A job from A that starts a program is refused by the runner's ceiling; the same job from B runs; each result is written to its own controller's store.

**R-id-30** A client has one controller.
*Accept:* a client that trusts one controller is given another controller's card with `corbie client trust`: refused, naming the controller it has. After the first is removed, the second is taken.

**C-id-11** The ceiling a runner applies to a job is the one it gave the controller that signed that job. A runner serving several controllers that applies one ceiling to all lets the least trusted use what was given to the most. *Test:* `id_runner_ceiling_chosen_by_jobs_signer`

## Changing and ending trust

**R-id-32** A ceiling or a runner's tags can be set again at any time by the command that gave them. A command that names neither leaves both as they are, and `none` takes them away.
*Accept:* a client enrolled with `--ceiling disk` is enrolled again with the same card and `--ceiling disk,network`: a run it starts afterwards may connect. A runner that runs `trust` again for the same controller with another ceiling applies the new one to the next job. A runner enrolled with `--tags a` is enrolled again with `--tags b`: a job started afterwards that names `a` finds no runner, and one that names `b` goes to it. Enrolled again by name with no flag, the runner keeps `b` and a client keeps its ceiling; with `--tags none` the runner has no tags, and with `--ceiling none` the client has no ceiling.

**R-id-34** Either side can remove the other's card, and from then on nothing from that party is accepted.
*Accept:* after a client is removed, a start it sends creates no instance. After a runner is removed, no job is sent to it and a result it writes is not accepted. After a runner removes a controller, a job that controller leaves in the runner's inbox is not run.

**R-id-36** A name stays tied to the key it was enrolled with, also after the party has been removed: a card with a different key under a name already used is refused, and the same key under its own name is taken again without being called a replacement.
*Accept:* enrol a runner named `laptop`; make another identity named `laptop` elsewhere and enrol its card: refused, saying the name is already used with another key, and the first runner's trust is unchanged. Remove `laptop`, then enrol the different key: refused the same way; enrol the same card again: accepted.

**C-id-20** `enrol` refuses a runner's or a client's card that bears the controller's own name. The name is the controller's area in the store, and a party under it would be given the controller's row of access to a served directory. *Test:* `id_enrol_refuses_the_controllers_own_name`

**R-id-39** A different key is taken under a used name when the person enrolling says it replaces the old one, and from then on nothing signed with the old key is accepted; the tie holds in a runner's and a client's config as it does at a controller.
*Accept:* enrol a new key named `laptop`, saying it replaces: accepted. A message signed with the old key is not accepted afterwards; one signed with the new key is. A runner trusts controller `ctl`; `corbie runner trust` is given a card named `ctl` with other keys: refused. Said to replace: accepted, and a job signed with the old key is not run.

**R-id-41** A replacement takes over the old key's runs: it can cancel them and send them events, though it cannot read what was sealed to the old key.
*Accept:* a client starts a run that waits; the client's key is replaced; a cancel signed with the new key ends that run as cancelled, and an event signed with the new key is taken. With its new key the client reads the record of a run its old key started: the entries written before the replacement cannot be opened.

**C-id-12** A removed party's name stays on record with its key, marked as removed; removing does not erase it. An erased name is free to the next card that arrives under it, and since an instance's name begins with its client's name, the newcomer would stand as the starter of the old client's runs. *Test:* `id_removed_name_stays_reserved`

**C-id-13** A replacement gets only the ceiling and tags given with it, and nothing the old key was given. Whoever holds the new key is not known to be whoever held the old. *Test:* `id_replacement_does_not_inherit_ceiling`

**C-id-14** Whether a party is trusted is read from where trust is kept, in the pass that handles its message; an answer remembered from an earlier pass or by another copy is not used. A removal has to hold in every copy from its next pass, not from whenever a remembered answer lapses. *Test:* `id_removal_holds_in_every_copy_at_next_pass`

## Messages

**R-id-44** A message is signed by its sender and sealed to its receiver: only the receiver can read it, one that has been altered is not accepted, and it cannot be passed off as another party's.
*Accept:* a third party with an identity of its own and the right to read every name in the store cannot read what a job waiting in a runner's inbox carries. Change one byte of a message waiting in an inbox: the receiver does not act on it. Runner B copies a result runner A wrote and files it as its own: the controller does not accept it as B's. A job sealed to runner A is copied into runner B's inbox: B does not run it.

**R-id-47** A receiver accepts a message only from a card it trusts; a message it does not accept, from a stranger, altered, or sealed to another, is deleted and gets no receipt.
*Accept:* a party with a well-formed identity that was never enrolled writes a start into a controller's inbox in a directory: no instance appears, the message is gone after the next pass, and no receipt is written for it.

**R-id-48** A message says when it stops being good, and a receiver drops one that is past that time, allowing five minutes for clocks that differ.
*Accept:* a start good for one minute is first looked at by its controller an hour later: it is not acted on, and it is gone from the inbox. A receiver whose clock is four minutes ahead of the sender's reads a message good for one minute as soon as it is sent: accepted. A message six minutes past its time by the receiver's clock: dropped.

**R-id-50** What a receiver does with an accepted message is decided by its kind and by who sent it; a runner acts only on jobs, and on cancels of jobs, from a controller it trusts.
*Accept:* a controller that a runner trusts leaves a message of another kind, a start, in the runner's inbox: the runner runs nothing.

**C-id-16** A message is checked in this order: signed by a card the receiver trusts, sealed to this receiver, still good. Nothing it carries is read before the first two hold, and nothing is acted on before all three do, because until then its contents are whatever the writer chose. *Test:* `id_message_checked_before_contents_read`

**C-id-17** Who sent a message, whom it is for, what kind it is and when it stops being good are taken from the part its sender signed, never from its name in the store or from anything beside the signed part. Anyone who can write into an inbox can write a wrapper around another's message. *Test:* `id_unsigned_parts_of_message_not_believed`

**C-id-19** Something in an inbox that the receiver does not accept, junk included, is deleted, and does not hold up what comes after it in the same pass. Every pass lists the inbox, and what is left there is listed and refused again for ever; and any party that may write into an inbox can leave anything there. *Test:* `id_refused_message_removed_from_inbox`

## Interfaces

Uses:

- **Store**: a controller's trust values are named values in its own area, read and rewritten there; every message is a named value in an inbox, under a name the store's rules order by sending.
- **Permission**: a ceiling is a layer of permissions, written and read as any other layer.

Used by:

- **Controller**: asks whether a sender is trusted, reads a client's ceiling and a runner's tags, opens messages sealed to it, signs heads, and seals jobs and receipts; which of a client's accepted messages it then acts on, and how a late one is answered, are its own (R-ctl-07, R-inst-67).
- **Runner**: opens jobs and cancels, applies the ceiling it gave the controller that signed a job, signs what it adds to a record, and seals results.
- **Client**: signs and seals what it sends, opens its receipts and records, and checks heads against the controller's card.
- **Instance**: a head carries the controller's signature; record entries are sealed to the controller and to the client that started the root.
- **Blob**: a reference travels only inside something sealed.
- **Secret**: a controller secret is sealed so that only the controller's key opens it.
- **Role**: `corbie run` loads the identity of each role it is started with.
