# Secrets

A secret is a value a job must be able to use and must not be able to read. Corbie keeps secrets, gives a job only the ones it names, and lets each one go only to the place it was bound to.

## Where secrets come from

A **controller secret** is kept by the controller in the store, sealed so only the controller can read it. Its owner sets it, sets it again, or removes it.

A **run secret** is sent with a start, for that run and everything below it. It overrides a controller secret of the same name. It is kept, sealed, in the root's record for as long as the root runs.

A machine only ever holds a secret's name.

## Getting to a job

A definition names each secret a job uses. When the controller builds a job, it puts in the values of the secrets the job names, and no others. If a layer the controller knows of does not allow one of them, the job is denied before anything is sent, and the refusal names the layer. An entry that covers secrets by a pattern permits and sends nothing. The job is sealed to its runner, whose own ceiling is applied when the script acts.

Each attempt at a job is built afresh, so it gets the value current at that time. A job that names a secret that is not set fails before anything is sent.

## Binding

A secret's permission says where it may go.

```yaml
allow:
  - net:api.github.com
  - secret:GITHUB_TOKEN@net:api.github.com       # into a connection to that host
  - net:10.0.30.8:8123
  - secret:HOME_TOKEN@net.unencrypted:10.0.30.8:8123   # to that host, encrypted or not
  - exec:deploy
  - secret:DEPLOY_TOKEN@exec:deploy              # to that program
  - secret:APP_KEY@fs.write:/etc/app/key         # into that file
  - secret:TG_BOT@*                              # anywhere the job may go
```

The destination is written as another permission, and the secret's entry opens nothing by itself. It lets the value travel there, and the connection, the program or the file needs its own entry. A secret's name is always written out. It holds no `@`, `:`, `=`, `*`, `$`, `{`, `}`, slash, backslash or blank, and does not start with `-` or `.`; in an entry, a `*` stands for any part of a name. Its destination is a scope like any other and may be filled from the machine's input.

A secret bound to a host goes only over an encrypted connection. Many things on an owner's own network speak no other way than plain, and for one of those the entry says so: `net.unencrypted` in place of `net` lets the secret go to that host over a connection that is not encrypted as well, and to no other host. Anybody on the way there can read it. A secret bound to anywhere goes over either. The host is the one the script names: a secret bound to a name does not go to a connection asked for by address, nor the other way.

Whatever is at the destination is trusted with the secret.

## Handles

A script never has a secret's value. It has a handle, which stands for the value and can be put wherever a value goes out: a header, a part of an address, a body, a password, a program's argument or variable, a file.

```js
const gh = secrets.get("GITHUB_TOKEN")

http.get("https://api.github.com/user", {
  headers: { Authorization: secrets.text`Bearer ${gh}` }
})
```

Text and handles are joined into a new handle, which is how a secret is put inside other text. A handle can be told how its value is to be encoded or escaped. It cannot be turned into a string, compared, measured or read a piece at a time. A library written in script can hold a handle, join it with text and pass it to a plugin, and nothing more.

A plugin can get the real value only together with the way out it asked Corbie for: the connection, program or file it is going into. Corbie checks that every secret in the handle may go to that destination, and if one may not, nothing goes. A plugin that is given a way to connect for a secret cannot use it to reach an address the secret is not bound to.

A secret may also be used as a key, to sign something or make a keyed hash. What comes out is ordinary data and Corbie cannot say where it then goes, so only a secret bound to anywhere may be used this way.

## Masking

Text a plugin hands back to a script is searched for the values of the job's secrets, as they are and in the usual encodings, and each is replaced with its name before the script sees it. A value of one or two characters is not searched for: it turns up in ordinary text. This is done once, by Corbie, at the line between plugin and script, for every plugin. It catches accidents, such as a server that echoes a header. It does not stop a script that is set on learning a value from a destination it is allowed to reach.

## Tokens bought with a secret

A token a service issues while a job runs, such as one from a sign-in, is ordinary data to the script. Binding protects the long-lived secret and not what it buys. The token is plain in the script and in anything the script emits or returns, so a library that signs in should not emit it.
