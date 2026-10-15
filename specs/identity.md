# Identity

Every controller, runner and client is a party with an identity of its own. This area is about what an identity is, how two parties come to trust each other, and how they then write to each other.

## Identities and cards

An identity is a pair of private keys, one for signing and one for decrypting, kept in a file only its owner can read. Its public side is a **card**: one line of text holding the party's name, whether it is a controller, a runner or a client, and its two public keys, signed by itself. A card has a short fingerprint, and both sides of an enrolment show it, so two people can check they mean the same card. A party's signing key is also how it proves itself to a served directory, and how the computer that serves the directory proves it holds the controller's.

A party's name is chosen when its identity is made and never changes. The store has no identity. It is storage.

A computer's parties live in one config directory, with a place each for a controller, a runner and a client. One computer can be all three.

## Enrolment

Two parties trust each other by swapping cards, and there is no other way.

On one computer it is one command:

```
corbie init --store ~/corbie --ceiling all
```

This makes a controller, a runner and a client in the config, has each trust the others, and prints what it did. `--ceiling` says what jobs on this computer may do. Without it they may do nothing outside themselves, and the first refusal names the command that changes that.

Across computers it is three steps, which join into one line because each reads the last one's output:

```
laptop$  corbie runner card
ctl$     corbie controller enrol <card> --tags laptop
laptop$  corbie runner trust <what enrol printed> --ceiling disk,exec

corbie runner card | ssh ctl corbie controller enrol - --tags laptop | corbie runner trust - --ceiling disk,exec
```

The controller's `enrol` takes a runner's or a client's card and prints back its own card and the address of its store, which is everything the other side needs. Where the controller's computer serves its directory, the address printed is the one other computers reach it at. A client enrols the same way.

A controller's identity is made together with the address of its store, by `corbie init` or by `corbie controller card --store <address>`. `enrol` writes what it learns into that store, so the store must be reachable and nothing needs to be running. A controller's trust is kept in the store. A runner's and a client's is kept in their own config.

## What trust carries

Trust is always given with a limit.

A controller gives each **client** a ceiling: the most any run that client starts may be allowed. A runner gives each **controller** it serves a ceiling: the most it will let that controller's jobs do on its computer. A party trusted with no ceiling may do nothing outside the job itself.

A controller also gives each runner its **tags**, which are how jobs choose runners.

A runner can serve several controllers, each with its own ceiling and its own store. A client has one controller.

Someone who only wants to see where runs stand needs no identity at all: the controller's card and the store's address, and whatever access to heads the store gives them.

## Changing and ending trust

The same commands change what was given: a ceiling or tags can be set again at any time. A command that names neither leaves both as they are, and `none` in place of a ceiling or of tags takes them away. Either side can remove the other's card, and from then on nothing from that party is accepted.

A name stays tied to the key it was enrolled with, and the controller's name to its own. A different key under a name already used is refused unless the person enrolling says it replaces the old one, and this holds after the party has been removed too. A replacement takes over the old key's runs: it can cancel them and send them events, though it cannot read what was sealed to the old key.

## Messages

Parties write to each other by leaving messages in each other's inboxes. A message is signed by its sender and sealed to its receiver, so only the receiver can read it and it cannot be altered or passed off as another's. A message says when it stops being good, and a receiver drops one that is past that time. A controller answers a client's late message with a receipt saying so.

A receiver accepts a message only from a card it trusts. What it then does with the message depends on its kind and on who sent it: a runner acts only on jobs from a controller it trusts, and a controller's rules are in its own area.
