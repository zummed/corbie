# Secrets: rules

The detailed rules behind the [Secrets overview](../secret.md). Requirements are `R-sec-nn`, constraints `C-sec-nn`.

## Where secrets come from

**R-sec-01** A controller secret is kept by the controller in the store, sealed so that only the controller can read it.
*Accept:* after a secret is set, the value under the controller's secrets in the store does not hold the secret in the clear, the controller's key opens it, and a runner's key and a client's key do not.

**R-sec-02** The controller's owner sets a controller secret, sets it again, or removes it. Only the latest value is kept.
*Accept:* the owner sets `K` to `a`, and a job that names `K` sends `a`. The owner sets `K` to `b`, and the next job sends `b`. The owner removes `K`, and the next job fails as naming a secret that is not set. No command brings `a` back.

**R-sec-10** A run secret is sent with a start, for that run and everything below it, and overrides a controller secret of the same name.
*Accept:* a root is started with the run secret `K`. A job of the root that names `K` sends it, and so does a job of a machine three levels down. A second root the same client starts without it fails as naming a secret that is not set. With the controller holding `K` as `a`, a root started with the run secret `K` as `b` sends `b`, and a root started without it sends `a`.

**R-sec-12** A run secret is kept, sealed, in the root's record for as long as the root runs.
*Accept:* a root with a keep of one hour runs for a day, and entries older than an hour are removed from its record as it goes. A job it starts at the end of the day still sends the run secret. The root's head holds no trace of the secret, and nothing in the record holds it in the clear.

**R-sec-13** A machine only ever holds a secret's name.
*Accept:* a root that uses `K` runs to its end. Where `K` is a controller secret, its step entries, opened with the controller's key, hold the name `K` and nowhere its value. A run secret's value is in the record, sealed, and nowhere else (R-sec-12). No expression in a definition can read either.

## Getting to a job

**R-sec-14** A definition names each secret a job uses, by a `secret` entry for that job with the secret's name written out. When the controller builds a job, it puts in the values of the secrets the job names and no others, and where a layer the controller knows of (the job in its definition, the start, each parent's job, the client's ceiling) does not allow one of them, the job is denied before anything is sent; being allowed a secret is not the same as being sent it.
*Accept:* the controller holds `A` and `B`, and a job's lists hold `secret:A@net:api.example`. The job, opened as its runner, holds `A`'s value and not `B`'s; the script sends `A`, and its use of `B` fails, saying the job's definition does not name it. Under a client's ceiling of `network` nothing is sent and the job is denied, naming the client's ceiling and what would allow it; the same when a start's list or a parent's job has no entry that covers `A`, or denies it everywhere; under a ceiling of `network, secret:A@*` the value is in the job. A job with `allow: [all]` and no `secret` entry is sent holding no secret's value.

**R-sec-16** An entry that covers secrets by a pattern in the name permits them and sends nothing.
*Accept:* the controller holds `GH_TOKEN`. A job whose only secret entry is `secret:GH_*@net:api.github.com` is sent with no secret's value. A client's ceiling of `secret:GH_*@*` allows `GH_TOKEN` for a job whose definition names it, and that job holds the value.

**R-sec-17** The values travel in the job, which is sealed to its runner, and nowhere else; the runner's own ceiling is applied when the script acts.
*Accept:* the job in the runner's inbox cannot be opened with another runner's key or with the client's, and after the run ends the value is in no head, step, emit, result or receipt. Every layer the controller knows of allows `A`, and the runner's ceiling has no entry that covers it: the job is sent with the value, the script's sending of `A` is refused, naming the runner's ceiling, and nothing arrives at the destination.

**R-sec-19** Each attempt at a job is built afresh, so it gets the value current at that time.
*Accept:* a job asks for two attempts and its first fails. The owner sets the secret again before the second, and the second attempt sends the new value.

**R-sec-20** A job that names a secret that is not set fails before anything is sent.
*Accept:* a job names `MISSING`, which is neither a controller secret nor a run secret. The job fails with an error naming `MISSING`, and no runner's inbox ever holds it.

## Binding

**R-sec-21** A secret's entry says where the secret may go, and the destination is written as another permission: `secret:NAME@net:host` lets it go into a connection to that host, `secret:NAME@exec:program` to that program, `secret:NAME@fs.write:path` into that file.
*Accept:* a job allowed `net:api.github.com`, `net:other.example` and `secret:GITHUB_TOKEN@net:api.github.com` sends the secret in a header to `api.github.com`, and the server receives the value; the same request to `other.example` is refused, the fix naming `secret:GITHUB_TOKEN@net:other.example`. A job allowed `exec:deploy`, `exec:other` and `secret:DEPLOY_TOKEN@exec:deploy` starts `deploy` with the secret as an argument and as a variable, and the program receives the value; starting `other` with it is refused. A job allowed `fs.write:/etc/app/**` and `secret:APP_KEY@fs.write:/etc/app/key` writes the secret to `/etc/app/key`, and the file holds the value; writing it to `/etc/app/other` is refused.

**R-sec-24** `secret:NAME@*` lets the secret go anywhere the job may go. `net`, `net.unencrypted`, `exec`, `fs.write` and `*` are the only destinations.
*Accept:* a job allowed `net:api.telegram.org`, `exec:notify` and `secret:TG_BOT@*` sends the secret to that host and hands it to that program. A destination of any kind but `net`, `net.unencrypted`, `exec`, `fs.write` or `*` fails the check of the definition that writes it.

**R-sec-25** A secret's entry opens nothing by itself. The connection, the program or the file needs its own entry.
*Accept:* a job allowed only `secret:GITHUB_TOKEN@net:api.github.com` is refused a request to `api.github.com`, with the secret or without, and the fix names `net:api.github.com`. A job allowed only `secret:TG_BOT@*` is refused a connection to anywhere.

**R-sec-26** A secret's name is always written out; its destination is a scope like any other and may be filled from the machine's input.
*Accept:* a definition with the entry `secret:${input.which}@*` fails its check, the report naming the entry. A job allows `net:${input.board}` and `secret:BOARD_TOKEN@net:${input.board}`; in a run started with `board: board.example`, the job sends the secret to `board.example`, and nowhere else.

**R-sec-29** A secret bound to a host goes only over an encrypted connection. `secret:NAME@net.unencrypted:host` lets it go to that host over a connection that is not encrypted as well, and to no other host; one bound to anywhere goes over either.
*Accept:* a job allowed `net:api.example` and `secret:K@net:api.example` sends `K` in a request to `https://api.example/`. The same request to `http://api.example/` is refused, and the refusal says the connection is not encrypted and gives `secret:K@net.unencrypted:api.example:80` as the line that would let it go. With `secret:K@net.unencrypted:api.example` instead, the requests to `http://api.example/` and to `https://api.example/` are both sent, and the server receives `K`; a request with `K` to `http://other.example/`, which the job may also reach, is refused. With `secret:K@*`, the request to `http://api.example/` is sent.

**C-sec-04** A secret's name is not empty and holds no `@`, no `:` and no blank. `NAME@destination` must split in exactly one place. *Test:* `sec_name_splits_one_way`

**C-sec-05** Whether a connection is encrypted is judged at the moment a value goes into it, not when it was opened. A connection that starts plain and is turned encrypted partway carries a host-bound secret only afterwards. *Test:* `sec_encrypted_judged_when_the_value_goes`

**C-sec-06** A connection Corbie did not encrypt itself counts as encrypted only on the plugin's word, and a plugin that gives its word leaves the library it wraps no way to fall back to a plain connection. Without the word a secret goes only when its entry says `net.unencrypted` for that host, or anywhere. A library that falls back sends a password in the clear on a connection Corbie was told was safe. *Test:* `sec_plugins_word_on_encryption`

## Handles

**R-sec-31** A script never has a secret's value. It has a handle, which stands for the value and can be put wherever a value goes out: a header, a part of an address, a body, a password, a program's argument or variable, a file.
*Accept:* `secrets.get("GITHUB_TOKEN")` in a job that was sent the secret returns something that is not text, and no statement of the script yields the value. With the entries each needs, a script puts the handle in each of the seven places in turn; a stand-in server, a stand-in program and the file each receive the real value.

**R-sec-33** Text and handles are joined into a new handle, and a handle can be told how its value is to be encoded or escaped.
*Accept:* ``secrets.text`Bearer ${gh}` `` is sent as a header, and the server receives `Bearer ` followed by the value; what was joined is a handle and is held to R-sec-35. A secret whose value holds a quote and a backslash, told to be escaped for the text it is joined into, arrives inside a JSON body that parses and yields the exact value.

**R-sec-35** A handle cannot be turned into a string, compared, measured or read a piece at a time.
*Accept:* `String(h)`, `` `${h}` ``, `h + ""` and `JSON.stringify({ h })` do not yield the value or any part of it; `h == "guess"` does not say whether the guess is right, even when it is; `h.length` gives nothing; `h[0]`, spreading `h` and looping over `h` give nothing of the value; and no call on a handle or on `secrets` says whether two values are equal, gives the size of a value, or returns a part of one.

**R-sec-40** A plugin can get the real value only together with the way out it asked Corbie for: the connection, the program or the file the value is going into. A plugin that is given a way to connect for a secret cannot use it to reach an address the secret is not bound to.
*Accept:* a stand-in plugin built on the public package gets a handle's value only by naming where it is going, and is refused where the secret is not bound to that place; that the value then goes there and nowhere else is the plugin's part. A job is allowed `net:a.example`, `net:b.example` and `secret:K@net:a.example`; the stand-in plugin asks for a way to connect carrying `K` and opens `b.example` with it: the opening is refused, and the plugin gets no value for it.

**R-sec-41** Corbie checks that every secret in a handle may go to the destination, and if one may not, nothing goes.
*Accept:* a handle joined from `A`, bound to `api.example`, and `B`, bound to `other.example`, is sent to `api.example`. The call is refused, naming `B`, and the server receives no request at all.

**R-sec-43** A secret may be used as a key, to sign something or to make a keyed hash; what comes out is ordinary data, so only a secret bound to anywhere may be used this way.
*Accept:* with `secret:K@*`, a keyed hash of `abc` under `K` equals the known answer for `K`'s value, and the script reads, compares, emits and returns the bytes like any others. With only `secret:K@net:api.example`, the same call is refused, and the fix names `secret:K@*`.

**C-sec-08** Every way of turning a handle into text throws. It never gives a stand-in text. A stand-in sent in place of a value is a fault found only at the destination. *Test:* `sec_every_conversion_throws`

**C-sec-09** Every secret in a call is checked before anything is made for the call: a connection, a file, a started program. A call half carried out leaves a value where it was not allowed. *Test:* `sec_all_checked_before_any_taken`

**C-sec-12** A value is escaped by what wrote the text around it and by nothing else. Whatever sends the text never reads its stated type to guess. A body's stated type does not say how the text around the value was written. *Test:* `sec_escaped_by_what_wrote_the_text`

**C-sec-13** A handle never stands in the part of an address that says where a request goes: its scheme, host, port or user. The check was made for one destination, and a value there would pick another. *Test:* `sec_value_cannot_choose_where_it_goes`

**C-sec-14** A handle is refused in any part of a request that is not sent as written: the headers that say where a request goes and how it is framed, a header's name, the method. A value there is rewritten or dropped on the way. *Test:* `sec_handle_only_where_sent_as_written`

**C-sec-15** A value that holds a line break or a zero byte does not go into a header. It would start a header of its own. *Test:* `sec_no_line_break_into_a_header`

**C-sec-16** A handle is refused wherever the result comes back to the script: as the thing to be hashed, signed, encrypted or packed. The value would return in a form nothing masks. *Test:* `sec_handle_never_the_data`

**C-sec-17** A file a secret is written into is readable by the runner's user alone. Left readable by others, it hands the value to every user of the computer. *Test:* `sec_secret_file_private`

**C-sec-19** Whatever Corbie logs or prints about a secret or a handle shows its name and never its value, in every way of printing. Logs and errors are how a value usually gets out. *Test:* `sec_nothing_prints_a_value`

## Masking

**R-sec-45** Text a plugin hands back to a script is searched for the values of the job's secrets, as they are and in the usual encodings, and each is replaced with its name before the script sees it.
*Accept:* a server echoes the request's `Authorization` header in its answer, once as it is, once as base64, once escaped as for an address and once escaped as for JSON. The body the script reads holds the name `GITHUB_TOKEN` in each place, and the value nowhere.

**R-sec-47** Masking is done once, by Corbie, at the line between plugin and script, for every plugin.
*Accept:* the value comes back through a web answer, a program's output, a line read from a raw connection, a command run on another computer, a message on a `websocket`, a file read as text, and text unpacked from a compressed body. The script sees the name in each. A stand-in plugin built on the public package that returns the value as text, and does nothing about it, has it replaced too.

**R-sec-48** Only text is searched. Bytes a plugin hands back, and bytes it writes into a blob, reach the script as they are.
*Accept:* a file whose bytes happen to hold a secret's value is read as bytes, and the script gets every byte unchanged. Read as text, the same file comes back with the name in the value's place.

**C-sec-20** The forms searched for are the value as it is, the value escaped for an address in its two ways, as base64 in its usual alphabets, escaped for JSON, each line of a value of several lines, and every form a handle can be told to send. A server echoes a value in the form it arrived in, and a log quotes one line of a key. *Test:* `sec_mask_knows_every_form`

**C-sec-21** A very short value is searched for only as it is, and one of one or two characters is not searched for at all. Its encoded forms are a few letters that turn up in ordinary text, and replacing those spoils what the script reads. *Test:* `sec_mask_short_value_only_as_it_is`

**C-sec-22** The longest form is replaced first. A shorter form found inside a longer one leaves the rest of the value showing. *Test:* `sec_mask_longest_form_first`

**C-sec-23** Text that arrives in pieces is searched across the joins. A value cut in two by the reading matches in neither piece. *Test:* `sec_mask_across_pieces`

**C-sec-24** A body with no stated type is searched as text. Servers echo values in bodies they give no type. *Test:* `sec_mask_untyped_body_as_text`

**C-sec-25** The text of an error, of a refusal, and of what a failed job's process printed is searched like any other before it leaves the job, and before any of it is cut. An error from the other side often quotes what it was sent, and a value cut first can be split so that it is no longer found. *Test:* `sec_mask_errors_and_refusals`

## Interfaces

- **Store** keeps controller secrets under the controller's area, sealed.
- **Identity** supplies the sealing: a controller secret to the controller alone, a job to its runner.
- **Controller** chooses the values when it builds a job, and fails a job that names a secret that is not set.
- **Instances** keep a root's run secrets in its record for as long as the root runs, through any trimming of its history.
- **Permissions** supply the `secret` kind and how its scope is matched (R-perm-09), the layers a value must pass, the check at the moment of acting, and the refusal.
- **Plugins** get a value only with a way out. The `secrets` plugin makes handles, and masking is done at the line every plugin's results cross.
- **Runner** holds a job's values for the life of the job, applies its own ceiling when the script acts, and has what its job's process prints masked before it is kept.
- **Client** sends run secrets with a start, and the controller's commands set, set again and remove controller secrets.
- **Models** use a secret as a model service's key, bound to the service's connection (R-model-15).
- **Manual** says where a secret's protection ends, as the overview does.
