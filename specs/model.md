# Models

An agent's turns are calls to a model, made through the `llm` plugin. This area is that plugin and the model services behind it. The plugin has two calls: `chat` holds a conversation with a model, and `choose` puts questions with fixed answers to a System One model.

## Chat

A script calls a model with one call: it gives the conversation so far, the tools on offer and what shape of answer is wanted, and gets back what the model said, the tools it asked for, why it stopped, and how many tokens were used. A service that has a way of its own to hold an answer to a shape is given the shape that way, and any other is told it in the system text. The model's text is emitted as it arrives, a few pieces at a time, so a watcher sees a reply being written.

Why the model stopped is always passed on. A reply that ended because it ran out of room is told apart from one that was finished.

## Choose

A System One model writes no text. It is given the text or data the questions are about, sent as `state`, and named questions, and answers every question at once with probabilities. A question has a type, its instructions, and for two of the types its criteria.

```
  noul     whether a statement is true: the probability that it is
  choice   one of the options in its criteria, with a probability for each
  score    a level of the ordered scale in its criteria, with a probability for each level
```

The questions are written in the System One format, which TypeSafe's Jev set. The answers come back as the service wrote them, with the model that answered and the tokens used. Corbie does not read the questions or the answers: what the service's documentation says they hold is what a script writes and reads.

```
const r = llm.choose("Checkout has been failing for every customer for the last hour.", {
  model: "jev-latest", key: secrets.get("TYPESAFE_KEY"),
  questions: { urgent: { type: "noul", instructions: "The customer cannot use the product." } },
})
r.answers.urgent.noul      0.99
r.model                    jev-1.13.0
```

An answer's `confidence` is how concentrated its probabilities are, not how likely it is right, so a condition reads a probability. The model that answered is a version, since `jev-latest` names a different version after each release.

The `systemone` service is at `https://api.typesafe.ai`, and a `jev-…` model is of it by default. `address:` sends the call to another host, such as a model served on the runner's computer, and a call with no `key:` sends no key.

An agent's model is of a service that has `chat`. A definition that names a model of `systemone` is refused when it is checked.

## Services

Each model service is an extension, kept apart from the rest and from each other. It says which kinds of content it accepts beside text, such as images or documents, and which levels of effort it takes. A definition that asks a service for something it does not take is refused when the definition is checked. A service also says which models it is known to offer, and a program which tools of its own it is known to have, for a person choosing. Those lists are for choosing from and hold nobody to them: a model or a tool may be named whether or not it is there.

A model's part of a definition holds what its service's models hold, and nothing else. One of a service on the web has an address and may be held to a size of reply; one of a program has neither, and has tools of its own. A field its service's models do not hold is refused when the definition is checked.

There are two kinds. A service **on the web** speaks one service's way of talking, and is reached with two things: an address and a key.

Corbie comes with four ways of talking: Anthropic's, OpenAI's two, and the System One format. Each knows where its service usually lives, so a definition that names a `claude-…` or `gpt-…` model need say nothing more. A model anywhere else that speaks one of these ways, self-hosted or offered by a cloud, is named by the way it speaks and its own address.

```
model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
model: { name: my-model, service: openai-chat, address: https://models.example, secret: MODELS_KEY }
model: { name: sonnet, service: claude-code }
```

A service that is **a program** starts that program on the runner's computer and has it write one reply. Corbie comes with one, for Claude Code. It has no address, it cannot be held to a size of reply, and its key is optional: without one, the program is signed in as the runner's user is. Its key is a key of the API, or a subscription's token (`claude setup-token` prints one), so a runner with nobody signed in on it can still use a subscription.

## Content beside text

A conversation may carry images and documents. One of a type the service takes is sent to the model as it is, under the name the model knows it by; one too large for the service, or of a type it does not take, is refused before anything is sent.

## A program's own tools

A model run as a program is started bare: its own tools, saved sessions and whatever its user has set up for it are turned off, in a fresh directory, so the only tools the model has are the ones Corbie runs and checks.

A definition may switch some of the program's own tools on by name, such as its web search. Corbie does not run such a tool and cannot hold it to a scope, so one is allowed only where the whole of what it could use is allowed, in every layer: a tool that browses needs `network`, one that reads or writes files needs `disk`, and a shell, a skill or anything else needs `all`. Each use is noted in the record, though nothing about it was checked.

```
model: { name: sonnet, service: claude-code, own: [WebSearch, WebFetch] }      needs network
model: { name: sonnet, service: claude-code, own: [Skill] }                    needs all
```

## Keys

A model service's key is an ordinary secret. When an agent's definition is sent, the client writes into each turn's permissions exactly what the turn needs: the connection to the service, and the key bound to that connection. For a model run as a program, the turn is written the start of that program in place of a connection, with the key bound to the program or, where there is none, the variable that says where the runner's user is signed in; and for each of the program's own tools that is switched on, the group it needs. A turn gets nothing else from the definition's permissions.
