# Models: rules

The detailed rules behind the [Models overview](../model.md). Requirements are `R-model-nn`, constraints `C-model-nn`. A "stand-in service" is a model service built for the tests, answering as it is told to.

## Chat

**R-model-01** A script holds a conversation with a model with the `llm` plugin's `chat`, giving the conversation so far, the tools on offer and what shape of answer is wanted.
*Accept:* a script makes the call with a conversation of two messages, one tool and a shape. The stand-in service receives all three. The `llm` page of the manual lists the call, and the standard turn of an agent uses it.

**R-model-02** The call gives back what the model said, the tools it asked for, why it stopped, and how many tokens were used.
*Accept:* the stand-in answers with a sentence and a request for one tool. The script's result holds the sentence, the tool's name and arguments, the reason the model stopped, and the tokens in and out.

**R-model-03** The model's text is emitted as it arrives, a few pieces at a time, so a watcher sees a reply being written.
*Accept:* the stand-in sends a reply in two hundred pieces over ten seconds. A watcher of the run's record sees emit entries holding the reply's text before the call has returned, the pieces put together in order are the whole reply, and the record gained far fewer than two hundred entries for them.

**R-model-05** Why the model stopped is always passed on, for every service, and a reply that ended because it ran out of room is told apart from one that was finished.
*Accept:* for each service that comes with Corbie, a reply that finished, a reply that asked for tools and a reply that ran out of room each give the script a result that says which it was. The stand-in sends the same text twice, once finished and once cut off at its length limit: the two results differ in why the model stopped, and an agent given the second tells the model and takes another turn (see Agent).

**C-model-01** A request for a tool whose arguments were cut off is never handed back as a request. Half of its arguments would run the tool with the wrong input. *Test:* `model_cut_off_tool_request_not_passed_on`

**C-model-02** A reply that holds requests for tools counts as having stopped to ask for tools, even when the service says it simply finished. Some services that speak another's way of talking say so. *Test:* `model_tool_requests_mean_stopped_for_tools`

**C-model-03** A service's running out of its whole window is passed on as running out of room. To whoever called, both mean the reply was cut short. *Test:* `model_window_full_is_out_of_room`

**C-model-04** The tokens counted as input include what the service read from and wrote to its own cache. Left out, the count is far too small. *Test:* `model_usage_counts_cached_input`

**C-model-05** Parts of a reply that a service wants back unchanged, such as its own reasoning, are kept as they came and sent back with the next call, and are dropped when the conversation is sent to another service. A service turns away, or loses its place in, a conversation whose parts were altered, and another model cannot read them. *Test:* `model_kept_parts_sent_back_unchanged`

## Decide

**R-model-21** A script puts questions to a System One model with the `llm` plugin's `decide`, giving `state`, the text or data the questions are about, and named questions in the System One format, and gets back the answers as the service wrote them, the model that answered and how many tokens were used.
*Accept:* a script makes the call with a state and three questions, one of each type. The stand-in receives them at `/v1/systemone` as they were written, with the key as a bearer token. The script's result holds each answer as the stand-in sent it, a field the plugin does not know included, the model's version and the tokens in and out.

**R-model-22** Corbie comes with the `systemone` service, at `https://api.typesafe.ai`, which `jev-…` models are of by default. `address:` sends the call to another host, and a call with no `key:` sends no `Authorization` header.
*Accept:* a call that names `jev-latest` and no service is of `systemone`, whose address is `https://api.typesafe.ai`. A call that gives a plain address on the runner's computer and no key goes there, and the request carries no `Authorization` header.

**C-model-19** An agent's model is of a service that has `chat`, and a definition whose model is of `systemone` is refused when it is checked, by the client and by the controller. A System One model writes no text and picks no tool, so no turn could be made. *Test:* `model_agent_model_has_chat`

## Services

**R-model-07** Each model service is an extension that speaks one service's way of talking and says where the service usually lives.
*Accept:* a binary built with the stand-in service as an extension runs an agent that names it and gives no address; each turn goes through it, to the service's usual address, and the connection written into each turn's permissions is to that host.

**R-model-09** A service says which kinds of content it accepts beside text, such as images or documents, and which levels of effort it takes; content of a type it takes is sent to the model as it is, under the name the model knows it by; a definition that asks a service for something it does not take is refused when the definition is checked, by the client before it is sent and by the controller again.
*Accept:* a conversation holding an image goes to a stand-in that accepts images, and the image arrives, after a line of text that gives its name; sent to a stand-in that does not, the call fails as bad input, naming the kind, and nothing is sent. An agent's tool returns a picture and a table: every call from the next turn on carries the picture beside its handle, and the table only as its handle. An agent's definition asks for a level the stand-in takes: it passes its check, and the level reaches the service with each turn. One that asks for a level the service does not list fails its check, the report naming the level and the levels the service takes. A definition that names a service the binary does not hold fails its check the same way.

**R-model-19** A service says which models it is known to offer, by the names it knows them by; a client offers them to a person choosing a model, and a model may be named whether or not it is among them.
*Accept:* a client asks what an agent's model may be and is given, for each service, the models it is known to offer; an agent that names a model of that service which is not among them passes its check, and its turns go to that model.

**R-model-20** A model's part of a definition holds what its service's models hold: one of a service on the web an address and a size of reply, one of a program tools of its own, and either a level of effort where the service takes levels; a field its service's models do not hold is refused when the definition is checked. A program says which tools of its own it is known to have, for a person choosing, and any may be named.
*Accept:* an agent names a model of a service that is a program and gives it a size of reply: the check fails, saying the program cannot be held to one. The same model with a tool of its own that is not among those the program is known to have passes its check. A client that asks what a model may hold is given, for each service, the fields its models hold.

**R-model-12** Corbie comes with services for Anthropic, for OpenAI in its two forms, and for a model run as a local program, each an extension kept apart from the others: a service on the web is reached with an address and a key, and one that is a program has neither an address nor need of a key.
*Accept:* in the binary as it comes, an agent naming each of the four runs a turn, the first three against stand-in servers that speak as those services do and the fourth against a stand-in program. A definition that gives the program's service an address fails its check.

**R-model-13** A self-hosted model gives its own address.
*Accept:* a definition names a service and an address. Its turns go to that address in that service's way of talking, and the connection written into each turn's permissions is to the given host and not the usual one.

**C-model-07** OpenAI's two forms are its Responses way and its Chat Completions way. In the first, nothing is kept at the service: its reasoning is asked for sealed, kept with the conversation and sent back. Some models refuse tools together with reasoning in the second way, and every agent has tools. *Test:* `model_openai_two_forms`

**C-model-08** For OpenAI in both forms, an image or a document in a tool's result is sent in a message of the user's that follows the result. These services take only text in a tool's result. *Test:* `model_openai_tool_result_content_follows`

**C-model-09** For OpenAI, tools are offered without the service's strict reading of the shapes of their arguments. Its strict reading refuses most shapes a tool's arguments take. *Test:* `model_openai_tools_not_strict`

**C-model-10** Where a service must be told the most a reply may hold, Corbie sends a figure of its own when the definition gives none. Such a service refuses a call without it. *Test:* `model_reply_size_always_sent_where_needed`

**C-model-11** Content too large for the service is refused before anything is sent. A service answers an oversized call with an error that says little, after the whole of it has been sent. *Test:* `model_too_large_content_refused_before_sending`

**C-model-12** A model run as a local program is started with the program's own tools, saved sessions and settings turned off, in a fresh directory, unless the definition switches a tool of its own on by name. A tool the program runs for itself is one Corbie does not run and cannot hold to a permission, and what it saves would pass from one job to the next. *Test:* `model_local_program_starts_bare`

**C-model-18** A tool of the program's own is allowed only where the whole of what it could use is allowed, in every layer: `network` for one that browses, `disk` for one that reads or writes files, `all` for a shell, a skill or one that is not known; and it is refused before the program is started. Corbie cannot check what such a tool does, so the only honest leave is leave for everything it could do. *Test:* `model_programs_own_tool_needs_its_whole_group`

**C-model-13** Names made up for a local program's tool requests never repeat within a conversation, even when earlier parts of it have been dropped. Two requests under one name cannot be matched to their results. *Test:* `model_made_up_request_names_never_repeat`

**C-model-14** A local program that does not read what it is sent, or does not end when stopped, does not hold the call. The call has a time limit like any other wait. *Test:* `model_local_program_cannot_hold_the_call`

**C-model-15** A service's address that holds a user name or a password is refused, and the refusal does not repeat the address. The address is kept with the definition and shown in errors. *Test:* `model_address_with_login_refused`

## Keys

**R-model-14** A model service's key is an ordinary secret, named in the agent's definition.
*Accept:* an agent's definition gives its `secret:` as `ANTHROPIC_KEY`. With that set as a controller secret, the stand-in server receives the value with each turn. With it sent as a run secret instead, the same.

**R-model-15** When an agent's definition is sent, the client writes into each turn's permissions exactly what the turn needs, the connection to the service and the key bound to that connection, and a turn gets nothing else from the definition's permissions.
*Accept:* the stored definition of that agent, shown by the client, gives its turn `net:` for the service's host and `secret:ANTHROPIC_KEY@net:` for the same host, and nothing else. The agent's definition allows `fs.read:/srv/**` for the whole definition: a turn script of the agent's own that reads `/srv/a` ends denied, and a tool of the same agent reads it.

**R-model-18** For a model run as a local program, what the turn needs is the start of that program, written into its turn's permissions in place of a connection, with the key bound to the program or, where the definition names none, the variable that says where the runner's user is signed in; and the group each of its own tools that is switched on needs.
*Accept:* the stored definition of an agent that names the local-program service gives its turn `exec:` for the program and `env:HOME`, and nothing else; with a key, `exec:` and the key bound to the program; with its web search switched on, `network` as well.

**C-model-16** A call to a model service follows no redirect. The key is bound to the service's host, and an answer from anywhere else is not the service's. *Test:* `model_no_redirect_followed`

**C-model-17** For a self-hosted model at a plain address, the client writes the key as bound to that host over a connection that is not encrypted, beside the connection. A key bound with `net` never goes over a plain connection, so every turn would otherwise be refused. *Test:* `model_plain_address_key_bound_to_it`

## Interfaces

- **Plugins** hold `llm` as a plugin of the one shape, with its two calls, and a model service is an extension like any other.
- **Agent** makes each turn as a job that calls the model through the standard turn, and acts on why the model stopped.
- **Client** writes each turn's permissions when it fills out an agent's definition, and checks the definition against what the service says it takes.
- **Machines** have the controller check the definition again, with the same code.
- **Secrets** hold the key: a controller or run secret, bound to the service's connection and masked where it comes back (R-sec-45).
- **Permissions** apply every layer to a turn's connection and key as to any job's.
- **Runner** writes the pieces of a reply to the record a few at a time, as it does all emits (C-run-33).
- **Instances** hold those pieces as emit entries, where a watcher reads them.
- **Blobs** supply the images and documents a conversation holds.
- **Manual** holds the `llm` page, and the pages a service's extension carries.
