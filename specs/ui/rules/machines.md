# Machines

The rules behind [the Machines overview](../machines.md). Area code: `machines`.

## What is read

**R-machines-10** The pages read the trees the controller recorded and the definition recorded at each location, as the controller, and no directory. A location's definition is read again only when a submit changed it, and the Machines and Toolkits pages share what was read. *Test:* `machines_definition_is_read_once_per_submit`

**C-machines-06** The Machines and Toolkits pages have no address that writes to the store or runs a command, and show no form. `corbie run submit` changes a tree, and a second way to change one would be a second path. *Test:* `machines_pages_only_read`

## The list

**R-machines-01** The list holds each tree by its prefix, with its client and the directory last submitted there, and under it the machine at each of its locations: its type's icon, its location written from the prefix, its type and name, its lifetime, its location whole in its location's tooltip, and its `description:` in the tooltip of its type and name. *Test:* `machines_list_shows_what_each_item_is`

## A machine

**R-machines-13** A machine shows its `description:`, its location, a link to the Runs page narrowed to it, its lifetime and what starts its runs, `limits: { runs: }`, its `update:` and what a changed file does to its runs, who its `receive:` lets send to it and call it, the `send:` entries of its `allow:`, or `all`, and for an agent the location each tool written `call:` calls, with the tools that call it, given once where `allow:` also names it, and the permissions its `approve:` lists. *Test:* `machines_a_machine_shows_its_location`

**R-machines-05** Choosing a machine shows its drawing, unfolded as the address says. *Test:* `machines_drawing_unfolds_as_the_address_says`

**R-machines-14** Machines gives Runs the address of the machine recorded at a location, `/machines?at=<location>` with `open=<card path>` for each card unfolded down to a child, and none where no machine is recorded there. *Test:* `machines_a_runs_machine_is_found_by_its_location`

**R-machines-16** A machine's location gives how many root runs there are running and how many failed today, and each state and running stage of its drawing gives how many of the UI's client's running runs at the location are in it, from each run's latest step; another client's runs are counted at the location only. The counts follow the store. *Test:* `machines_a_location_counts_its_runs`, `machines_location_row_says_its_counts`, `machines_drawing_counts_the_runs_in_each_state`, `machines_counts_follow_the_store`

**R-machines-17** A card's name on a drawing links to the Definition tab, which shows the lines written at the card's file and line lit, in place; a card of a machine unfolded inside another has no such link. *Test:* `machines_card_opens_its_line`

**R-machines-18** Machines gives Runs the values of the definition now recorded at a run's location that differ from the definition the run started with, and each script and child machine whose contents differ; none where they are the same, and none where no machine is recorded there. *Test:* `machines_a_run_differs_from_the_definition_recorded`

**R-machines-12** Definition shows the definition recorded at the location: its type and name, then its fields by name, each value with the file and line it was submitted from. *Test:* `machines_definition_shows_where_each_value_is_from`

## Secrets and values

**R-machines-04** The page lists each secret and each value a machine uses, read from its definition, its child machines included, and where each is set: the location nearest the machine's, at it or above, or `not set` at its location or above. *Test:* `machines_secrets_and_values_used_are_listed`

## Toolkits

**R-machines-06** The Toolkits page lists, by tree, each toolkit an agent offers: an agent at a location, or one a machine at a location runs with `machine:` or writes in place. A toolkit's file is the one its tools were submitted in, as the agent's definition gives it, where that is not the agent's own file, and two agents of one tree that take its tools from the same file offer one toolkit. A toolkit shows the first line of its `instructions:` or the file they were included from; each tool's `description:`, its job, for a call, written `call:` or as a job's `call:`, the location with a link to the machine recorded there and the input it fixes, for a job's `send:` the event and the address with a link to the machine recorded at its location, its `when:`, and whether the job has `approve: true`; each agent that offers it, with its entry's `when:`; and for an agent at a location each machine of its tree that runs its file with `machine:` or calls its location, and how its location's runs start, or for any other agent the machine that runs it or writes it in place. *Test:* `machines_toolkits_show_what_offers_each`, `machines_toolkits_call_machines_by_their_location`, `machines_toolkits_written_in_a_placed_agent_are_in_its_machine`

## Interfaces

Machines uses Pages for the shell, the stream and its errors (`page.Shell`, `page.Write`, `page.Stream`, `page.Error`), and its tests use `pagetest.Run`, and Drawing to draw a definition and to find a toolkit's file (`drawing.Unfolded`, `drawing.Unfold`, `drawing.ToolkitFile`), and to write an address as a drawing does (`drawing.Address`). Serve gives it the controller and its store (`Config.Controller`, `Config.Store`), the list of runs the UI reads (`Config.List`), the client it acts as (`Config.Client`), and the latest step of a run as the Runs page reads it (`Config.Latest`), and Runs asks it for the address of the machine at a location (`Pages.Href`), and how the definition a run started with, by its reference, differs from the one recorded at its location (`Pages.Differs`).
