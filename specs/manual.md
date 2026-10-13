# Manual

The binary carries its own manual, so the documentation always matches the binary in hand. It is written for two readers: a person, and a model that has been asked to write definitions and scripts.

## Pages

The manual is pages of plain text with examples. A page says in its first line what it is for, then what the thing is, why a reader wants it and how to do it. There is a page for each idea a user needs, drawn from these overviews; a page for each command; a page for each machine type; and a page for each plugin and each library that comes with the binary, extensions included.

A plugin's or a library's page is the full account of how to call it: every call, every option, what it returns, and the permissions it needs. A model writing a script is expected to read the page first, so the page is written to be enough.

The examples in every page are run as tests. A page whose example no longer works fails the build.

The pages for commands are made from the one table of commands, and the list of plugins and their permission kinds is made from what the binary actually holds.

## Reading it

`corbie manual` lists the pages and `corbie manual <page>` prints one. `corbie mcp` gives a model the same two things.

`corbie manual --web` serves the same pages to a browser on this computer, each as the text that is printed. A page that another page names is shown there as a link, by its title. It serves until it is stopped. It reads no config and no store, and is no part of a running system.
