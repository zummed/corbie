package cli

import "github.com/zummed/corbie/identity"

// Commands is the one table of commands.
var Commands []Command

// everywhere names, at the end of every command's help, the flags every
// command takes, which the overview describes.
const everywhere = "Every command takes --json, --config and --store: see corbie --help."

// starts is the help for what a start may be given, at corbie run start and
// at corbie test.
const starts = `
  --input <json>           the run's input; --input @file reads the JSON from
                           a file
  --input-file place=path  a file as a blob at that place in the input, such
                           as report or 'files[0].file'; repeatable
  --label k=v              a label on the run, k=v or k, readable by anyone
                           who can list runs; repeatable, each name once
  --allow <entry>          one permission the run's jobs may use; with
                           --allow or --deny they may use no other; repeatable
  --deny <entry>           one permission the run's jobs are refused,
                           whatever is allowed; repeatable
  --timeout <time>         the longest the run may take, such as 1h; it
                           cannot extend the definition's timeout
  --life <time>            how long the start message is valid while no
                           controller takes it; default 5m. The command
                           waits that long and 10m more for the answer,
                           for clocks that differ, and says so while it
                           waits`

// scripts is the help for a script file given where a definition is.
const scripts = `

A script file (.js) can be given in place of a definition. Corbie wraps it in
a flow of one stage, and these flags set that stage's job. With a definition,
--memory and --tag are refused.

  --allow, --deny    the job's allow and deny lists
  --timeout <time>   the job's timeout; default 5m
  --memory <size>    the most memory the job may use, such as 256MB; default
                     no limit
  --tag <tag>        a tag the job's runner must have; repeatable

  corbie manual permissions   the entries --allow and --deny take
  corbie manual runs          a run as JSON, and one run's file as another
                              run's input`

func init() {
	Commands = []Command{
		{Name: "init", Usage: "[--store <address>] [--identity <address>] [--ceiling <list>] [--deny <entry>] [--name <name>] | store [--listen host:port] [--card [--name <name>] | --admit <card> | --join <admitted> | --remove <name> | --alone]", Summary: "create a controller, a runner and a client on this computer", Run: cmdInit, Help: `
It creates the three in the config directory, makes each trust the others,
sets up the store and prints what it did. Running it again replaces the
ceiling with the one then given.

  --store <address>      the store: a directory, or an S3 bucket as s3://…;
                         default the one the config records, or CORBIE_STORE,
                         or on a new config the directory store in it
  --identity <address>   keeps the three identities in an S3 bucket, as
                         s3://…, and not in the config; only on a new config
  --ceiling <list>       the most any job on this computer may be allowed, as
                         entries separated by commas: all, or network,disk,
                         or net:api.github.com,fs.read:/srv/**; default
                         nothing
  --deny <entry>         a permission the ceiling refuses, whatever it
                         allows; repeatable
  --name <name>          names the three <name>-controller, <name>-runner and
                         <name>-client; default this computer's host name;
                         only on a new config, or one with those names

corbie init store sets up only the store's server, which gives other
computers this computer's store directory, on a config that holds the
controller; corbie start then runs it.

  store --listen host:port   where it listens; default the address saved
                             before, or else :8372, every network interface,
                             with this computer's host name given to other
                             computers
  store --listen ""          serves the directory to no other computer

A store has members: computers that each hold a copy of the directory. A
computer joins with one line, run on it:

  corbie init store --card | ssh sw1 corbie init store --admit - \
      | corbie init store --join -

  store --card               prints this computer's card, named by its host
                             name, or by --name where the card is first made
  store --admit <card>       on a member, prints the controller's identity
                             encrypted to that card, and the members'
                             address; - reads the card from standard input
  store --join <admitted>    takes what --admit printed, and makes this
                             computer a member: corbie start then copies the
                             members' directory into this computer's, which
                             must be empty; - reads standard input
  store --remove <name>      removes a member of the store, from another
                             member
  store --alone              makes this computer's copy the whole store, with
                             one member, where the other members are lost;
                             corbie start is stopped on this computer first

corbie init store on a member prints the store's members: each one's name,
its address, and whether it leads. --listen with another address there
changes the member's address, and the other members are told it.

A name that another controller has on the store is refused.

  corbie manual one-computer       this setup, and keeping it running
  corbie manual served-directory   store, and adding other computers
  corbie manual permissions        the entries of a ceiling`},
		{Name: "start", Usage: "[<name> [<its flags or words>]…]…", Summary: "run the controller, a runner, the gateway, the store's server or a program, until stopped", Stays: true, Run: cmdStart, Help: `
It runs what its arguments name, in one process. With nothing named, it runs
the controller, the runner and the store's server that the config holds:
what corbie init set up.

  controller   this config's controller
  runner       this config's runner, for every controller it trusts
  gateway      an HTTP server that starts and reads runs as this config's
               client, or as the clients of the configs --client names
  store        a server that gives other computers this computer's store
               directory, set up by corbie init store
  ui           the UI, started as corbie ui, as a program is
  <name>       the program corbie-<name> on PATH, started with CORBIE_CONFIG
               and CORBIE_STORE in its environment as this command was given
               them; when it stops, everything stops

The words after a name, up to the next name, are its own: a role's flags, or
a program's arguments. A program's word that is itself a name begins the
next name, so a program that needs such a word is run on its own, as
corbie <name> ….

  corbie start controller --interval 1s runner --jobs 8 ui --listen 127.0.0.1:8383

  controller --interval <time>     how often it polls the store; default 2s
  runner --interval <time>         how often it polls the store; default 2s
  runner --jobs <n>                how many jobs it runs at once; default 4
  runner --exec-wrapper <command>  a command put before every program a job
                                   starts, such as a sandbox or a container
  gateway --listen host:port       where it listens; default 127.0.0.1:8371
  gateway --client <config>        a config whose client the gateway acts as;
                                   written once for each client
  gateway --token-file <file>      a file with a line for each token a request
                                   may carry: the token, then the client it
                                   names and the principal it was given to; by
                                   default it makes a token for each client
                                   and prints it
  gateway --oidc-issuer <address>  an OIDC issuer whose tokens the gateway
                                   takes too
  gateway --oidc-audience <text>   who the issuer's tokens must be for; given
                                   with the issuer
  gateway --oidc-client-claim <c>  the claim that names the client; default
                                   roles
  gateway --oidc-principal-claim <c>
                                   the claim that is the principal: the
                                   person or the service a token is for;
                                   default sub
  store --listen host:port         where it listens this time; default where
                                   corbie init store --listen said
  store --timeout <time>           closes a connection that sends nothing for
                                   this long; default 60s

In a directory, a controller or a runner starts work when a message is
written to its inbox by any process on the computer, and does not wait for
the interval. Where the operating system does not report those writes,
corbie start prints a warning that says why, and polls. In a bucket, only a
controller and a runner in one process notify each other.

The wrapper is started with these environment variables and with none that a
script set. The variables a script gives a program are passed to it by the
program env, which the wrapper must be able to start.

  CORBIE_SCRATCH   the job's scratch directory
  CORBIE_ALLOW     the job's permissions, one to a line

  corbie manual one-computer       keeping the process running
  corbie manual served-directory   serving the store directory
  corbie manual gateway            the gateway's requests and its token
  corbie manual functions          the controller and runners as AWS Lambda
                                   functions
  corbie manual limits             what a started program can do`},
		{Name: "up", Usage: "[--in <directory>] [--at <prefix>] [--store <address>] [<name> [<its flags or words>]…]…", Summary: "run a controller, a runner and a gateway on a directory of machines, set up in the directory", Stays: true, Run: cmdUp, Help: `
It does what corbie init, corbie start and corbie run submit do, in one
process, until stopped.

  1. It sets up a controller, a runner and a client in the config .corbie of
     the current directory, with the store in it, unless they are set up.
  2. It runs the controller, the runner and a gateway.
  3. It submits the directory, as corbie run submit does.
  4. It starts each program named, as corbie start does.

Every command run in the same directory, or below it, uses .corbie: corbie
run start, corbie controller secret set, corbie ui. Run again, corbie up
continues with the runs, the secrets and the values in it. Remove .corbie to
start with nothing.

  --in <directory>     the directory of machines; default the current one
  --at <prefix>        the prefix it is submitted at; default / and the
                       directory's name
  --store <address>    the store, when .corbie is set up: a directory, or an
                       S3 bucket as s3://…; default .corbie/store
  --ceiling <list>     the most any job may be allowed, when .corbie is set
                       up, as corbie init takes it; default all
  --value NAME=VALUE   a value set at / of the controller; repeatable
  --value-env NAME     a value set at / of the controller, read from the
                       environment variable of its name; repeatable
  --secret NAME        a secret set at / of the controller, its value typed
                       at the terminal and not shown; repeatable
  --secret-env NAME    a secret set at / of the controller, its value read
                       from the environment variable of its name; repeatable
  --config <directory> another config than .corbie of the current directory

The names and the words after them are corbie start's. The gateway listens
at a free port, which gateway --listen host:port changes. A program is
started with CORBIE_CONFIG, CORBIE_GATEWAY and CORBIE_GATEWAY_TOKEN in its
environment.

  corbie up        the machines of this directory
  corbie up ui     and the UI

  corbie manual a-directory   a directory of machines
  corbie manual gateway       the gateway's requests and its token`},
		{Name: "check", Usage: "[--tag <tag>]… [--wait 90s]", Summary: "start one small run, and report which part of a setup did not respond", Stays: true, Run: cmdCheck, Help: `
Run it last when you set anything up. It starts a run with one script job and
one timer, and prints four lines, each with a status and the time it took. It
fails unless every line is ok.

  the controller took the start    ok     0.6s
  a runner took the job            ok     0.3s
  the controller took the result   ok     0.4s
  the timer fired                  ok     41.0s

  ok      it happened
  late    it took more than 20s: on a serverless host, the bucket did not
          call the function and the once-a-minute call did the work
  never   it did not happen within --wait

A line that is not ok is followed by its causes, on a computer and on a
serverless host. The timer is never late: on a serverless host it waits for
the once-a-minute call. While the command waits, it prints what it waits for.

The check reads only the store, so it is the same wherever Corbie runs.

  --tag <tag>     send the job to a runner that has the tag; repeatable; by
                  default any runner takes it
  --wait <time>   how long to wait for each line, from the one before it;
                  default 90s

  corbie manual functions   the calls a serverless host must make`},
		{Name: "test", Usage: "<definition or script> [--input <json>] [--value NAME=VALUE] [--watch] | <definition> --in <directory> | <definition> [--in <directory>] --definition | <directory>...", Summary: "run a definition or a script in this process, with no setup", Stays: true, Run: cmdTest, Help: `
It runs a controller, a runner and a client in this process with the store in
memory, records the file as a tree of one file, starts it, waits until the run
ends and prints it. It reads no config and keeps nothing. It fails unless the
run ends with status done.

In this process only the runs send events, and nothing answers a ticket. When
no run has a job, a timer, a timeout or a ticket outstanding, it prints each
run still running and fails. Beside the run, it prints each other run that failed, and
each run the submit or a send started at a singleton or keyed location.

A singleton is started by the submit, and takes no flag of a start. A keyed
file is refused: its runs start when an event is sent to a key.

With --in, it records the directory as corbie run submit does, which starts
its singletons, and runs the definition at its location in that tree, where
its call: and send: name the tree's machines. A file of the tree with a
problem is printed, and nothing runs.

The ceilings allow everything. Each job's own allow: list still applies.
` + starts + `
  --value NAME=VALUE       a value, set at / of the controller in this
                           process; repeatable
  --value-env NAME         a value set at / of the controller in this
                           process, read from the environment variable of
                           its name; repeatable
  --secret NAME            a secret set at / of the controller in this
                           process, its value typed at the terminal and not
                           shown; repeatable
  --secret-env NAME        a secret set at / of the controller in this
                           process, its value read from the environment
                           variable NAME; repeatable
  --watch                  print each entry of the run's record as it is
                           written
  --in <directory>         the directory whose tree the definition is in

With --definition, it prints the definition as the client writes it out,
each value with its file and line, and runs nothing. With --in as well, it
reads the definition at its location in that directory's tree.

Given directories, it reads every machine in them as corbie run submit does,
prints each one ok or its problems and each path .corbieignore lists, and
starts nothing. It fails when a machine has a problem. Each directory is a
tree: a send or a call to one of its locations is checked against the file
there and the sender's own allow: and deny:, and a location with no machine
is printed, which fails nothing.
` + scripts},
		{Name: "run submit", Usage: "<directory or file> [--at <prefix>]", Summary: "record a directory or a file as a tree", Run: cmdSubmit, Help: `
Given a directory, it records it as a tree. Each *.yaml in it and below it, but
for .git and what the directory's .corbieignore lists, is a machine at a
location: its path under the prefix, without .yaml. It starts each singleton with no running run,
and cancels the runs at a location whose file has gone. A changed file's
running runs do as its update: says. A file with a problem is reported, and its
location keeps what was recorded before. It prints what it did at each
location, each run it cancelled, and the secrets and values the files use
that are not set.

Given a file, it records it as a tree of one file.

  --at <prefix>   where the tree is recorded; by default / and the
                  directory's or the file's name, without .yaml or .js

  corbie run start <location>   start a run of a transient location
  corbie manual a-directory     a tree of machines
` + scripts},
		{Name: "run remove", Usage: "<prefix>", Summary: "remove a tree of this client's, with its runs, secrets and values", Run: cmdRemoveTree, Help: `
It removes the tree recorded at the prefix. The controller does this:

  1. It cancels every run at the tree's locations.
  2. It removes every location of the tree.
  3. It removes every secret and value set at the prefix or under it.
  4. It removes the tree, so the prefix holds none.

The command prints each location, each run it cancelled and each secret and
value it removed, and exits when the cancelled runs have ended. A secret or a
value set above the prefix stays. A run that has ended is kept as its
definition's keep: says.

Where the prefix holds no tree, it removes the secrets and values set there.

  corbie run submit <directory>   record a tree
  corbie manual a-directory       a tree of machines
`},
		{Name: "run start", Usage: "<location> [--input <json>] [--wait | --watch]", Summary: "start a run of a transient location and print its ID", Run: cmdRunStart, Help: `
It starts a run of the file recorded at the location: it uploads the input's
files and sends the start message. It prints the run's ID once a controller
has taken the start, or the reason it was refused. It fails if the start is
refused.
` + starts + `
  --wait                   wait until the run ends, then print it
  --watch                  print each entry of the run's record as it is
                           written, until the run ends

With --wait or --watch the command fails unless the run ends with status done.

  corbie manual permissions   the entries --allow and --deny take
  corbie manual runs          a run as JSON, and one run's file as another
                              run's input
`},
		{Name: "run ls", Usage: "[--watch] [--all] [--label k=v] [--status <status>] [--parent <id>]", Summary: "list runs", Run: cmdLs, Help: `
It lists the runs in progress of this client's trees and of the files whose
read: lists it, child runs included. A run with status waiting, which no controller has started, is not
listed.

  --all               include runs that have ended and are still in the store
  --label k=v         only runs with this label; --label k takes any value
                      and --label k= the empty value; repeatable
  --status <status>   only runs with this status: running, done, failed or
                      cancelled; the last three are of runs that have ended
  --parent <id>       only the child runs that this run started
  --watch             keep running: print the runs that exist, then a line
                      that ends them, then each run as it appears, changes
                      or ends

  corbie manual runs   a run as it is printed, and with --json`},
		{Name: "run show", Usage: "<id> | <location>[#key] [--record | --watch | --wait | --tickets | --step n | --definition | --save <directory>]", Summary: "show one run: its status and its result", Run: cmdShow, Help: `
It prints the run's status and input; while it runs, its states, stages or
turn and the outputs so far, from its latest step; and its output or its error
when it has ended. A location, or a location and a key, names the one run of this
client's running there. It takes one of these flags at most.

  --record             print each entry of the run's record so far, child
                       runs' included, then the run
  --watch              print the same, then each entry as it is written,
                       until the run ends
  --wait               wait until the run ends, then print it
  --tickets            print each ticket the run waits on, as its name and
                       its value; corbie run reply takes the value
  --step n             print step n of the run's record, as JSON, with the
                       outputs so far of the run's stages or states
  --definition         print the definition the run was started from, with
                       every short form written out, as JSON
  --save <directory>   save the files of the run's result into a directory

With --watch or --wait the command fails unless the run ends with status done.

  corbie manual runs   the statuses, a failure, and a blob in a result`},
		{Name: "run cancel", Usage: "<id> | <location>[#key] [--life 5m]", Summary: "cancel a run in a tree of this client's", Run: cmdCancel, Help: `
It sends the cancel to the controller and prints the answer: taken, or refused
with the reason. It fails if the cancel is refused. A location cancels every
run of this client's running there, each with a cancel of its own; a run that
starts there meanwhile is not cancelled. A keyed location and a key, as
/location#key, cancels the running run of that key, and fails when none is
running.

  --life <time>   how long the cancel is valid while no controller takes it;
                  default 5m. The command waits that long and 10m more for
                  the answer, for clocks that differ, and says so while it
                  waits`},
		{Name: "run send", Usage: "<id> | <location>[#key] <event> [data] [--life 5m]", Summary: "send an event to a run by its address", Run: cmdSend, Help: `
The address is the run's ID, a singleton's location, or a keyed location and
a key as /location#key, which starts a run when none has the key. The data is
JSON, or plain text if it does not parse as JSON. The command prints the run
that took the event and whether the machine acted on it or had no rule for
it, or the reason it was not delivered. It fails if the event is not
delivered.

A run in another client's tree needs send:<location> in this client's
ceiling and client:<name> in the file's receive:.

  --life <time>   how long the event is valid while no controller takes it;
                  default 5m. The command waits that long and 10m more for
                  the answer, for clocks that differ, and says so while it
                  waits`},
		{Name: "run reply", Usage: "<id> <ticket> [value] [--life 5m]", Summary: "answer a ticket that a run waits on", Run: cmdReply, Help: `
The ticket is the value that corbie run show <id> --tickets prints, and <id>
is the run that waits on it. Any client that has the value may answer, and a
ticket is answered once. The answer is JSON, or plain text if it does not
parse as JSON.

The command prints taken, or refused with the reason. It fails if the answer
is refused.

  --life <time>   how long the answer is valid while no controller takes it;
                  default 5m. The command waits that long and 10m more for
                  the answer, for clocks that differ, and says so while it
                  waits`},

		{Name: "controller card", Usage: "--store <address> [--name <name>] [--identity <address>]", Summary: "create this computer's controller and print its card", Run: cmdCard(identity.Controller), Help: `
It creates the controller's identity if the config has none, sets up the
store and prints the controller's card.

  --store <address>      the controller's store: a directory, or an S3
                         bucket as s3://…; required the first time
  --name <name>          the controller's name; default
                         <host name>-controller
  --identity <address>   keeps the identity in an S3 bucket, as s3://…, and
                         not in the config; only where none is made yet

  corbie manual parties            cards and trust
  corbie manual served-directory   serving the store directory`},
		{Name: "controller enrol", Usage: "<card | - | name> [--ceiling <list>] [--allow <entry>] [--deny <entry>] [--tags <list>] [--at <prefix>] [--replace]", Summary: "trust a runner or a client, and print what it needs to trust this controller", Run: cmdEnrol, Help: `
It takes a runner's or a client's card, writes to the store that the
controller trusts it, and prints the controller's card and the store's
address. corbie runner trust and corbie client trust take those two lines.

  <card>   the card, as corbie runner card or corbie client card prints it
  -        the card is read from standard input, so the commands join in a
           pipe
  <name>   a runner or client already enrolled: sets its ceiling, tags or
           prefixes again

  --ceiling <list>   sets a client's ceiling, the most any run in its trees
                     may be allowed, with at:<prefix> for where it may
                     submit trees: entries separated by commas; none
                     removes it; without the flag it stays as it is
  --allow <entry>    adds one entry to the client's ceiling; repeatable
  --deny <entry>     adds one permission the client's ceiling refuses,
                     whatever it allows; repeatable
  --tags <list>      sets a runner's tags, separated by commas; a job's tags:
                     selects a runner that has every one; none removes them;
                     without the flag they stay as they are
  --at <prefix>      the runner is sent only the jobs of runs at or under the
                     prefix; repeatable; / is every tree, the default;
                     without the flag the prefixes stay as they are
  --replace          accept a card with another key under a name already
                     enrolled; by default it is refused

A runner takes no ceiling here, and a client no tags or --at. A runner sets
its own ceiling, and the same prefixes, with corbie runner trust.

  corbie manual served-directory   the three commands as one pipe
  corbie manual permissions   the entries of a ceiling`},
		{Name: "controller remove", Usage: "<name>", Summary: "stop trusting a runner or a client", Run: cmdRemove, Help: `
The controller keeps the name with its key, so a card with another key under
that name is refused without --replace. corbie controller parties lists it as
removed.`},
		{Name: "controller parties", Usage: "", Summary: "list the runners and clients the controller trusts", Run: cmdParties, Help: `
Each line gives a runner's or a client's kind, name and card fingerprint, then
its ceiling, deny list and tags if set, a runner's prefixes, then removed if
it was removed.`},
		{Name: "controller secret set", Usage: "<NAME> [- | --env] [--at <location>]", Summary: "set a secret for the runs at a location and below", Run: cmdSecretSet, Help: `
The value is typed at the terminal and not shown. It is never given on the
command line. A run uses the secret set nearest its location.

  -                the value is read from standard input, to the end
  --env            the value is read from the environment variable NAME
  --at <location>  where the secret is set; by default /, for every tree

  corbie manual using-secrets   how a job uses a secret`},
		{Name: "controller secret remove", Usage: "<NAME> [--at <location>]", Summary: "remove a secret set at a location", Run: cmdSecretRemove, Help: `
A secret of the name set above the location is then the one used.

  --at <location>  where the secret is set; by default /`},
		{Name: "controller secret list", Usage: "", Summary: "list the names of the secrets and where each is set", Run: cmdSecretList},
		{Name: "controller value set", Usage: "<NAME> <VALUE> [--at <location>]", Summary: "set a value for the runs at a location and below", Run: cmdValueSet, Help: `
A value is plain text, not secret. An expression reads it as ${values.NAME},
and a run uses the value set nearest its location.

  --at <location>  where the value is set; by default /, for every tree
  --               a value that starts with - follows it, as in OFFSET -- -5

  corbie manual using-secrets   secrets and values`},
		{Name: "controller value remove", Usage: "<NAME> [--at <location>]", Summary: "remove a value set at a location", Run: cmdValueRemove, Help: `
A value of the name set above the location is then the one used.

  --at <location>  where the value is set; by default /`},
		{Name: "controller value list", Usage: "", Summary: "list the values and where each is set", Run: cmdValueList},
		{Name: "controller policy", Usage: "<name> | --viewer", Summary: "print the bucket policy for one controller, runner or client", Run: cmdPolicy, Help: `
On an S3 bucket each controller, runner and client has its own access key.
The command prints the IAM policy that limits one key to the objects its
process reads and writes.

  <name>     the controller itself, or an enrolled runner or client
  --viewer   a key that can only list runs and read their status

Where this config keeps a party's identity in a bucket, that party's policy
also allows reading it.

  corbie manual bucket   creating the keys`},
		{Name: "controller cancel", Usage: "<id>", Summary: "cancel any run, in any client's tree", Run: cmdControllerCancel, Help: `
It writes the cancel to the controller's inbox and does not wait. The
controller cancels the run the next time it polls.`},
		{Name: "controller tree remove", Usage: "<prefix>", Summary: "remove any client's tree, with its runs, secrets and values", Run: cmdControllerTreeRemove, Help: `
It removes the tree recorded at the prefix as corbie run remove does, whichever
client the tree belongs to, also one the controller no longer trusts. It
writes the removal to the controller's inbox and does not wait. The
controller removes the tree the next time it polls.

  corbie run remove <prefix>   what a removal removes`},

		{Name: "runner card", Usage: "[--name <name>] [--identity <address>]", Summary: "create this computer's runner and print its card", Run: cmdCard(identity.Runner), Help: `
It creates the runner's identity if the config has none, and prints its card,
which corbie controller enrol takes.

  --name <name>          the runner's name; default <host name>-runner
  --identity <address>   keeps the identity in an S3 bucket, as s3://…, and
                         not in the config; only where none is made yet`},
		{Name: "runner trust", Usage: "<what enrol printed | - | name> [--ceiling <list>] [--allow <entry>] [--deny <entry>] [--at <prefix>] [--replace]", Summary: "trust a controller, up to a ceiling", Run: cmdTrust(identity.Runner), Help: `
It takes what corbie controller enrol printed, which is the controller's card
and the store's address, and saves both in the config.

  <what enrol printed>   the card, then the address
  -                      both are read from standard input
  <name>                 a controller already trusted: sets its ceiling or
                         prefixes again

  --ceiling <list>   sets the ceiling, the most this controller's jobs may be
                     allowed on this computer: entries separated by commas;
                     none removes it; without the flag it stays as it is
  --allow <entry>    adds one entry to the ceiling; repeatable
  --deny <entry>     adds one permission the ceiling refuses, whatever it
                     allows; repeatable
  --at <prefix>      takes only the jobs of runs at or under the prefix, as
                     the controller's enrol --at gives; repeatable; / is
                     every tree, the default; without the flag the prefixes
                     stay as they are
  --replace          accept a card with another key under a name already
                     trusted; by default it is refused

  corbie manual served-directory   the three commands as one pipe
  corbie manual permissions   the entries of a ceiling`},
		{Name: "runner remove", Usage: "<controller>", Summary: "stop trusting a controller, and running its jobs", Run: cmdUntrust(identity.Runner), Help: `
The config keeps the controller's name with its key, so a card with another
key under that name is refused without --replace.`},

		{Name: "client card", Usage: "[--name <name>] [--identity <address>]", Summary: "create this computer's client and print its card", Run: cmdCard(identity.Client), Help: `
It creates the client's identity if the config has none, and prints its card,
which corbie controller enrol takes.

  --name <name>          the client's name; default <host name>-client
  --identity <address>   keeps the identity in an S3 bucket, as s3://…, and
                         not in the config; only where none is made yet`},
		{Name: "client trust", Usage: "<what enrol printed | -> [--replace]", Summary: "trust a controller", Run: cmdTrust(identity.Client), Help: `
It takes what corbie controller enrol printed, which is the controller's card
and the store's address, and saves both in the config. A client trusts one
controller: remove the other first with corbie client remove.

  <what enrol printed>   the card, then the address
  -                      both are read from standard input

  --replace   accept a card with another key under a name already trusted;
              by default it is refused

  corbie manual served-directory   the three commands as one pipe`},
		{Name: "client remove", Usage: "<controller>", Summary: "stop trusting a controller", Run: cmdUntrust(identity.Client), Help: `
The config keeps the controller's name with its key, so a card with another
key under that name is refused without --replace.`},

		{Name: "client show", Summary: "print which client a command is sent as, and for which principal", Run: cmdClientShow, Help: `
It prints the name of this config's client. With --gateway it prints the
client and the principal the gateway's token names: what a command with that
token is sent as.

  corbie client show --gateway http://127.0.0.1:8371

  corbie manual gateway-clients   several clients at one gateway`},
		{Name: "client secret set", Usage: "<NAME> [- | --env] --at <location>", Summary: "set a secret for the runs at a location and below, in this client's trees", Run: cmdClientSecretSet, Help: `
The controller takes it at a location where this client could submit a tree,
or in one of its trees. The value is typed at the terminal and not shown. It
is never given on the command line. A run uses the secret set nearest its
location.

  -                the value is read from standard input, to the end
  --env            the value is read from the environment variable NAME
  --at <location>  where the secret is set; required

  corbie manual using-secrets   how a job uses a secret`},
		{Name: "client secret remove", Usage: "<NAME> --at <location>", Summary: "remove a secret set at a location in this client's trees", Run: cmdClientSecretRemove, Help: `
A secret of the name set above the location is then the one used.

  --at <location>  where the secret is set; required`},
		{Name: "client secret list", Usage: "", Summary: "list the names of the secrets set where this client may set them, and where each is set", Run: cmdClientSecretList},
		{Name: "client value set", Usage: "<NAME> <VALUE> --at <location>", Summary: "set a value for the runs at a location and below, in this client's trees", Run: cmdClientValueSet, Help: `
The controller takes it at a location where this client could submit a tree,
or in one of its trees. A value is plain text, not secret. An expression
reads it as ${values.NAME}, and a run uses the value set nearest its
location.

  --at <location>  where the value is set; required
  --               a value that starts with - follows it, as in OFFSET -- -5

  corbie manual using-secrets   secrets and values`},
		{Name: "client value remove", Usage: "<NAME> --at <location>", Summary: "remove a value set at a location in this client's trees", Run: cmdClientValueRemove, Help: `
A value of the name set above the location is then the one used.

  --at <location>  where the value is set; required`},
		{Name: "client value list", Usage: "", Summary: "list the values set where this client may set them, and where each is set", Run: cmdClientValueList},

		{Name: "manual", Usage: "[page]", Summary: "print a page of the manual, or list the pages", Run: cmdManual, Help: `
With no page it lists the pages. It reads no config and no store.`},
		{Name: "manual web", Usage: "[--listen host:port]", Summary: "serve the pages to a browser on this computer, until stopped", Stays: true, Run: cmdManualWeb, Help: `
It reads no config and no store.

  --listen host:port   where it listens; default 127.0.0.1:7403`},
		{Name: "mcp", Usage: "[--allow <command>]…", Summary: "serve the manual and the commands to a model, as an MCP server", Stays: true, Run: cmdMCP, Help: `
It is an MCP server on standard input and output, with two tools: manual,
which prints a page, and corbie, which runs a command. By default the model
may run every command of Corbie's. Commands run with this server's config and
store, and the model can name no other. The server needs no controller or
runner running, and keeps no state.

  --allow <command>   the model may run only the commands named, each by its
                      whole name, such as test, "run show" or a program's
                      "board new"; repeatable; default every command of
                      Corbie's and no program's

A program's command adds the program's pages to the manual, and the model is
told the program's page named after it.

  corbie mcp                                      every command
  corbie mcp --allow "run ls" --allow "run show"  those two only

  corbie manual first-machine   adding the server to an assistant`},
		{Name: "add", Usage: "[<pack>[@<ref>]]… [--set WORD=<name>]… [--in <directory>]", Summary: "write a pack of ready-made machines into a project", Run: cmdAdd, Help: `
A pack is a directory of ready-made machines, in a git repository or on this
computer.

  github.com/owner/repository/notes          the directory notes of a repository
  github.com/owner/repository/notes@v0.4.0   at a tag, a branch or a commit
  host/a/b/c.git/notes                       a repository whose address has
                                             more than three parts
  ./packs/notes                              a directory on this computer

1. It fetches the repository with git, at the ref, or at its default branch.
2. It rewrites .packs/<pack>/ in the project with the pack's files, and the
   same for each pack beside it that its .yaml and .js files name as
   .packs/<name>/.
3. It writes each file of the pack's starter/ directory at the same path in
   the project, where no file is there and it has written none before.
4. It adds .packs/ to the project's .corbieignore when it lacks it.

It changes no other file, and no file of yours. It names each starting file
that is there and is not as the pack starts it.

  --set WORD=<name>   writes <name> in place of WORD in each starting file's
                      path and text, as --set TEAM=ops
  --in <directory>    the project; default the directory it was started in

With no pack named it adds each pack the project holds again, from where it
came and with the words it was added with. A pack added from a branch is
read at the branch's newest commit.

It reads no config and no store. A pack's scripts run with the permissions
its machines give them and the secrets set at their locations: read a pack
before a run of it.`},
		{Name: "version", Usage: "", Summary: "print the version, the machine types and the plugins", Run: cmdVersion},
	}
}
