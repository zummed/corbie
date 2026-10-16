# Manual

The detailed rules behind [the Manual overview](../manual.md).

## Pages

**R-man-01** The binary carries its own manual, so the manual read is always the one for the binary in hand.
*Accept:* a binary copied alone to a computer with no network, no config and no other file of Corbie's lists and prints every page; two binaries built from different sources each print their own text for a page that differs between them.

**R-man-03** There is a page for each idea a user needs, drawn from the overviews; a page for each command; a page for each machine type the binary holds; and a page for each plugin and each library written in script that comes with the binary, extensions included, listed and printed like any others.
*Accept:* for each area the overviews' map lists, the list of pages has at least one page that explains it. Every command in the table of commands has a page, and no command's page is for a command the table does not have. The list has one page each for flow, statechart and agent, one under the name of every module a script can call, and one for every library a script can load by a bare name, the agent's standard turn among them. A binary built with an extension that adds a type, a plugin and a library has their pages beside Corbie's own, printed the same way, and a binary built without it does not.

**R-man-09** A plugin's or a library's page is the full account of how to call it, every call, every option, what it returns and the permissions it needs, and is enough to write a script that uses it.
*Accept:* for each plugin and library, the calls a script can make on it and the calls its page gives are the same set; for each call, the options it accepts and the options the page gives are the same set, an option the page gives is accepted and one it does not give is refused; each call has a statement of what it returns, and the value the page's example gets back has that shape; a script that makes a call with exactly the permissions the page gives is not refused, and with one taken away is `denied`. Every example on the page runs given only the permissions the page names, with no call or option the page does not give.

**R-man-14** The examples in every page are run as tests, and a page whose example no longer works fails the build.
*Accept:* the build's tests include every example of every page in the binary, a script as a job, a definition as a run and a command as a command; a page added with an example gains a test with no other change. A plugin is changed so that an option an example uses is refused: the build fails and names the page and the example.

**R-man-16** The pages for commands are made from the one table of commands, and the list of plugins, with the kinds of permission each needs, is made from what the binary holds.
*Accept:* each command's page gives the arguments, the flags and the line saying what it does that the command's help gives, and a flag added to the table appears on the page with no other change. The list names every plugin in this binary and no other, each with the kinds its calls ask for; a binary built with one more plugin lists it with its kinds, with no page edited by hand.

**C-man-01** Every example is run, and a page has no way to mark one as not run. An example that is not run goes out of date without anyone being told, which is the fault running them exists to stop. *Test:* `man_no_example_is_skipped`

**C-man-06** An example that shows what it returns or prints is checked against what it does return or print. An example that still runs and now gives something else teaches the wrong thing. *Test:* `man_example_output_matches_the_page`

## Reading it

**R-man-18** `corbie manual` lists the pages and `corbie manual <page>` prints one; given a name that is no page it says so and ends as a mistake in how the command was written.
*Accept:* `corbie manual` prints the name of every page in the binary once, the extensions' among them, and every name it prints is accepted by `corbie manual <page>`, which prints that page's whole text on standard output and ends with success. `corbie manual nosuchpage` prints that there is no such page and how to list the pages, prints no page, and ends with the status of a mistake.

**R-man-21** `corbie mcp` gives a model the same two things: the list of pages, and one page as the same text a person reads.
*Accept:* the model server's manual tool, asked for the list, gives the same names `corbie manual` prints; for every page, what the tool gives and what `corbie manual <page>` prints are the same text.

**R-man-24** `corbie manual web` serves the same pages to a browser on this computer until it is stopped, each as the text that is printed, with a page that another names shown as a link by its title. It reads no config and no store.
*Accept:* with nothing set up and nothing running, `corbie manual web` says where the manual is; the list at that address leads to every page `corbie manual` lists, each page's text there is the text `corbie manual <page>` prints, but for the pages it names, each shown by its title and leading to that page; and every file a page asks for is served from the same address. It listens on this computer alone unless `--listen` names another address. Stopped, it ends with success and has written nothing. The model server's manual tool does not start it.

## Interfaces

Uses:

- **Client.** The table of commands, from which each command's page is made (R-client-97); the `corbie manual` command and the model server's manual tool, which are how the pages are read.
- **Plugins.** The plugins, libraries and extensions the binary holds, each carrying its pages; the calls and options of each, against which its page is checked.
- **Permissions.** The kinds of permission, and what each call asked for when its example ran.
- **Machines, Flow, Statechart, Agent.** Each type supplies its own page.
- **Runner, Roles.** A page's example is run as a job or as a run, in one process, as `corbie test` runs one.

Used by:

- **Client.** `corbie manual` and `corbie mcp` list and print the pages, and `corbie manual web` serves them to a browser.
- **Plugins.** A plugin's page is its specification, and the tests of its examples are the plugin's own tests.
- **Store, Permissions, Secrets.** Each carries a statement the manual must make: the bucket services tested and that a directory keeps no party out (R-store-50); what is not defended against (R-perm-60); where a secret's protection ends, as the Secrets overview says it.
