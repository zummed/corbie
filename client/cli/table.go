package cli

import "github.com/zummed/corbie/identity"

// Commands is the one table of commands.
var Commands []Command

// everywhere names, at the end of every command's help, the flags every
// command takes, which the overview describes.
const everywhere = "Every command takes --json, --config and --store: see corbie --help."

// starts is the help for what a start may be given, at corbie submit and at
// corbie test.
const starts = `
  --input <json>           the run's input; --input @file reads the JSON from
                           a file
  --input-file name=path   a file as a blob in the input, under that name;
                           repeatable
  --label k=v              a label on the run, readable by anyone who can
                           list runs; repeatable
  --allow <entry>          one permission the run's jobs may use; with
                           --allow or --deny they may use no other; repeatable
  --deny <entry>           one permission the run's jobs are refused,
                           whatever is allowed; repeatable
  --timeout <time>         the longest the run may take, such as 1h; it
                           cannot extend the definition's timeout
  --secret NAME            a run secret, its value typed at the terminal and
                           not shown; repeatable
  --secret-env NAME        a run secret, its value read from the environment
                           variable NAME; repeatable
  --life <time>            how long the start message is valid while no
                           controller takes it; default 5m`

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
		{Name: "init", Usage: "--store <address> [--ceiling <list>] [--deny <entry>] [--name <name>] [--serve host:port]", Summary: "create a controller, a runner and a client on this computer", run: cmdInit, Help: `
It creates the three in the config directory, makes each trust the others,
sets up the store and prints what it did. Running it again replaces the
ceiling and --serve with those then given.

  --store <address>   the store: a directory, or an S3 bucket as s3://…;
                      required unless CORBIE_STORE is set
  --ceiling <list>    the most any job on this computer may be allowed, as
                      entries separated by commas: all, or network,disk, or
                      net:api.github.com,fs.read:/srv/**; default nothing
  --deny <entry>      a permission the ceiling refuses, whatever it allows;
                      repeatable
  --name <name>       names the three <name>-controller, <name>-runner and
                      <name>-client; default this computer's host name
  --serve host:port   the host name and port where other computers connect
                      to this computer's store directory; default not served

A name that another controller has on the store is refused.

  corbie manual one-computer       this setup, and keeping it running
  corbie manual served-directory   --serve, and adding other computers
  corbie manual permissions        the entries of a ceiling`},
		{Name: "run", Usage: "<controller | runner | gateway | store>… [--interval 2s] [--jobs 4]", Summary: "run the controller, a runner, the gateway or the store's server, until stopped", Stays: true, run: cmdRun, Help: `
It runs what its arguments name, in one process.

  controller   this config's controller
  runner       this config's runner, for every controller it trusts
  gateway      an HTTP server that starts and reads runs as this config's
               client
  store        a server that gives other computers this computer's store
               directory, at the address given to corbie init --serve

A flag is refused unless the one it applies to is named.

  --interval <time>          how often a controller or a runner polls the
                             store; default 2s
  --jobs <n>                 how many jobs the runner runs at once; default 4
  --listen host:port         where the gateway listens; default 127.0.0.1:8371
  --token-file <file>        a file with the token every request to the
                             gateway must carry; by default the gateway
                             makes a token and prints it
  --exec-wrapper <command>   a command put before every program a job
                             starts, such as a sandbox or a container

A controller and a runner in one process notify each other, and do not wait
for the interval.

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
		{Name: "test", Usage: "<definition or script> [--input <json>] [--watch]", Summary: "run a definition or a script in this process, with no setup", Stays: true, run: cmdTest, Help: `
It runs a controller, a runner and a client in this process with the store in
memory, waits until the run ends and prints it. It reads no config and keeps
nothing. It fails unless the run ends with status done.

The ceilings allow everything. Each job's own allow: list still applies.
` + starts + `
  --watch                  print each entry of the run's record as it is
                           written` + scripts},
		{Name: "submit", Usage: "<definition or script> [--input <json>] [--wait | --watch]", Summary: "start a run and print its ID", run: cmdSubmit, Help: `
It reads the definition and the files it names, reports every fault in them
together, uploads the files and sends the start message. It prints the run's
ID and exits, unless --wait or --watch is given.
` + starts + `
  --wait                   wait until the run ends, then print it
  --watch                  print each entry of the run's record as it is
                           written, until the run ends

With --wait or --watch the command fails unless the run ends with status done.` + scripts},
		{Name: "ls", Usage: "[--watch] [--all] [--label k=v] [--status <status>] [--parent <id>]", Summary: "list runs", run: cmdLs, Help: `
It lists this client's runs that are in progress, child runs included. A run
with status waiting, which no controller has started, is not listed.

  --all               include runs that have ended and are still in the store
  --label k=v         only runs with this label; --label k takes any value;
                      repeatable
  --status <status>   only runs with this status: running, done, failed or
                      cancelled; the last three need --all
  --parent <id>       only the child runs that this run started
  --watch             keep running: print the runs that exist, then a line
                      that ends them, then each run as it appears, changes
                      or ends

  corbie manual runs   a run as it is printed, and with --json`},
		{Name: "show", Usage: "<id> [--record | --watch | --wait | --tickets | --step n | --definition | --save <directory>]", Summary: "show one run: its status and its result", run: cmdShow, Help: `
It prints the run's status, and its output or its error when it has ended. It
takes one of these flags at most.

  --record             print each entry of the run's record so far, child
                       runs' included, then the run
  --watch              print the same, then each entry as it is written,
                       until the run ends
  --wait               wait until the run ends, then print it
  --tickets            print each ticket the run waits on, as its name and
                       its value; corbie reply takes the value
  --step n             print step n of the run's record, as JSON
  --definition         print the definition the run was started from, with
                       every short form written out, as JSON
  --save <directory>   save the files of the run's result into a directory

With --watch or --wait the command fails unless the run ends with status done.

  corbie manual runs   the statuses, a failure, and a blob in a result`},
		{Name: "cancel", Usage: "<id> [--life 5m]", Summary: "cancel a run that this client started", run: cmdCancel, Help: `
It sends the cancel to the controller and prints the answer: taken, or refused
with the reason. It fails if the cancel is refused.

  --life <time>   how long the cancel is valid while no controller takes it,
                  and how long the command waits for the answer; default 5m`},
		{Name: "send", Usage: "<id> <event> [data] [--life 5m]", Summary: "send an event to a run that this client started", run: cmdSend, Help: `
The data is JSON, or plain text if it does not parse as JSON. The command
prints whether the machine acted on the event or had no rule for it, or
refused with the reason. It fails if the event is refused.

  --life <time>   how long the event is valid while no controller takes it,
                  and how long the command waits for the answer; default 5m`},
		{Name: "reply", Usage: "<id> <ticket> [value] [--life 5m]", Summary: "answer a ticket that a run waits on", run: cmdReply, Help: `
The ticket is the value that corbie show <id> --tickets prints, and <id> is
the run that waits on it. Any client that has the value may answer, and a
ticket is answered once. The answer is JSON, or plain text if it does not
parse as JSON.

The command prints taken, or refused with the reason. It fails if the answer
is refused.

  --life <time>   how long the answer is valid while no controller takes it,
                  and how long the command waits for the answer; default 5m`},
		{Name: "check", Usage: "[--tag <tag>]… [--wait 90s]", Summary: "start one small run, and report which part of a setup did not respond", Stays: true, run: cmdCheck, Help: `
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

		{Name: "controller card", Usage: "--store <address> [--name <name>] [--serve host:port]", Summary: "create this computer's controller and print its card", run: cmdCard(identity.Controller), Help: `
It creates the controller's identity if the config has none, sets up the
store and prints the controller's card.

  --store <address>   the controller's store: a directory, or an S3 bucket as
                      s3://…; required the first time
  --name <name>       the controller's name; default <host name>-controller
  --serve host:port   the host name and port where other computers connect
                      to this computer's store directory

  corbie manual parties            cards and trust
  corbie manual served-directory   serving the store directory`},
		{Name: "controller enrol", Usage: "<card | - | name> [--ceiling <list>] [--allow <entry>] [--deny <entry>] [--tags <list>] [--replace]", Summary: "trust a runner or a client, and print what it needs to trust this controller", run: cmdEnrol, Help: `
It takes a runner's or a client's card, writes to the store that the
controller trusts it, and prints the controller's card and the store's
address. corbie runner trust and corbie client trust take those two lines.

  <card>   the card, as corbie runner card or corbie client card prints it
  -        the card is read from standard input, so the commands join in a
           pipe
  <name>   a runner or client already enrolled: sets its ceiling or tags
           again

  --ceiling <list>   sets a client's ceiling, the most any run it starts may
                     be allowed: entries separated by commas; none removes
                     it; without the flag it stays as it is
  --allow <entry>    adds one entry to the client's ceiling; repeatable
  --deny <entry>     adds one permission the client's ceiling refuses,
                     whatever it allows; repeatable
  --tags <list>      sets a runner's tags, separated by commas; a job's tags:
                     selects a runner that has every one; none removes them;
                     without the flag they stay as they are
  --replace          accept a card with another key under a name already
                     enrolled; by default it is refused

A runner takes no ceiling here, and a client no tags. A runner sets its own
ceiling with corbie runner trust.

  corbie manual served-directory   the three commands as one pipe
  corbie manual permissions   the entries of a ceiling`},
		{Name: "controller remove", Usage: "<name>", Summary: "stop trusting a runner or a client", run: cmdRemove, Help: `
The controller keeps the name with its key, so a card with another key under
that name is refused without --replace. corbie controller parties lists it as
removed.`},
		{Name: "controller parties", Usage: "", Summary: "list the runners and clients the controller trusts", run: cmdParties, Help: `
Each line gives a runner's or a client's kind, name and card fingerprint, then
its ceiling, deny list and tags if set, then removed if it was removed.`},
		{Name: "controller secret set", Usage: "<NAME> [-]", Summary: "set a controller secret", run: cmdSecretSet, Help: `
The value is typed at the terminal and not shown. It is never given on the
command line.

  -       the value is read from standard input, to the end
  --env   the value is read from the environment variable NAME

  corbie manual using-secrets   how a job uses a secret`},
		{Name: "controller secret remove", Usage: "<NAME>", Summary: "remove a controller secret", run: cmdSecretRemove},
		{Name: "controller secret list", Usage: "", Summary: "list the names of the controller's secrets", run: cmdSecretList},
		{Name: "controller policy", Usage: "<name> | --viewer", Summary: "print the bucket policy for one controller, runner or client", run: cmdPolicy, Help: `
On an S3 bucket each controller, runner and client has its own access key.
The command prints the IAM policy that limits one key to the objects its
process reads and writes.

  <name>     the controller itself, or an enrolled runner or client
  --viewer   a key that can only list runs and read their status

  corbie manual bucket   creating the keys`},
		{Name: "controller cancel", Usage: "<id>", Summary: "cancel any run, whichever client started it", run: cmdControllerCancel, Help: `
It writes the cancel to the controller's inbox and does not wait. The
controller cancels the run the next time it polls.`},

		{Name: "runner card", Usage: "[--name <name>]", Summary: "create this computer's runner and print its card", run: cmdCard(identity.Runner), Help: `
It creates the runner's identity if the config has none, and prints its card,
which corbie controller enrol takes.

  --name <name>   the runner's name; default <host name>-runner`},
		{Name: "runner trust", Usage: "<what enrol printed | - | name> [--ceiling <list>] [--allow <entry>] [--deny <entry>] [--replace]", Summary: "trust a controller, up to a ceiling", run: cmdTrust(identity.Runner), Help: `
It takes what corbie controller enrol printed, which is the controller's card
and the store's address, and saves both in the config.

  <what enrol printed>   the card, then the address
  -                      both are read from standard input
  <name>                 a controller already trusted: sets its ceiling again

  --ceiling <list>   sets the ceiling, the most this controller's jobs may be
                     allowed on this computer: entries separated by commas;
                     none removes it; without the flag it stays as it is
  --allow <entry>    adds one entry to the ceiling; repeatable
  --deny <entry>     adds one permission the ceiling refuses, whatever it
                     allows; repeatable
  --replace          accept a card with another key under a name already
                     trusted; by default it is refused

  corbie manual served-directory   the three commands as one pipe
  corbie manual permissions   the entries of a ceiling`},
		{Name: "runner remove", Usage: "<controller>", Summary: "stop trusting a controller, and running its jobs", run: cmdUntrust(identity.Runner), Help: `
The config keeps the controller's name with its key, so a card with another
key under that name is refused without --replace.`},

		{Name: "client card", Usage: "[--name <name>]", Summary: "create this computer's client and print its card", run: cmdCard(identity.Client), Help: `
It creates the client's identity if the config has none, and prints its card,
which corbie controller enrol takes.

  --name <name>   the client's name; default <host name>-client`},
		{Name: "client trust", Usage: "<what enrol printed | -> [--replace]", Summary: "trust a controller", run: cmdTrust(identity.Client), Help: `
It takes what corbie controller enrol printed, which is the controller's card
and the store's address, and saves both in the config. A client trusts one
controller: remove the other first with corbie client remove.

  <what enrol printed>   the card, then the address
  -                      both are read from standard input

  --replace   accept a card with another key under a name already trusted;
              by default it is refused

  corbie manual served-directory   the three commands as one pipe`},
		{Name: "client remove", Usage: "<controller>", Summary: "stop trusting a controller", run: cmdUntrust(identity.Client), Help: `
The config keeps the controller's name with its key, so a card with another
key under that name is refused without --replace.`},

		{Name: "manual", Usage: "[page | --web]", Summary: "print a page of the manual, or list the pages", run: cmdManual, Help: `
With no page it lists the pages. It reads no config and no store.

  --web                serve the pages to a browser on this computer,
                       until stopped
  --listen host:port   where --web listens; default 127.0.0.1:7403`},
		{Name: "mcp", Usage: "[--allow <command>]…", Summary: "serve the manual and the commands to a model, as an MCP server", Stays: true, run: cmdMCP, Help: `
It is an MCP server on standard input and output, with two tools: manual
prints a page, and corbie runs a command. Commands run with this server's
config and store, and the model can name no other. The server needs no
controller or runner running, and keeps no state.

  --allow <command>   a command the model may run, such as test or
                      "controller parties"; repeatable; default none

  corbie mcp --allow ls --allow show --allow test --allow submit

  corbie manual first-machine   adding the server to an assistant`},
		{Name: "version", Usage: "", Summary: "print the version, the machine types and the plugins", run: cmdVersion},
	}
}
