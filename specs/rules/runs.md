# Runs

The rules behind [the Runs overview](../runs.md). Area code: `runs`.

## Names

**R-runs-09** A run in a tree of the UI's client is named by its title where it has one, read from its record once while the list holds it; any other run, and a run with no title, by its address, its location with `#key` at a keyed location, written from the prefix the page is narrowed to; and a child that `machine:` started, by its machine. A run at a keyed location named by its title has `#key` after the title. Its line, its row and Children show its name. The row that started it shows its title, and its page's title line shows its title beside its address. The tooltip of its name gives its title, its full address and its ID. *Test:* `runs_a_run_is_named_by_its_title`, `runs_a_child_is_named_by_its_title`, `runs_keyed_runs_are_listed_by_key`

## The list

**R-runs-10** Locations lists each location at which the list holds a run, under the client whose tree holds it, and a tree of the UI's client under its prefix, read from the record of a run there. A location shows its lifetime, read from the record of a run there in a tree of the UI's client, or `keyed` where a run there has a key; and the status of a singleton's run, or how many of its runs are running. *Test:* `runs_locations_are_listed_by_tree`

**R-runs-01** Each root, a running run whose parent is not running, is one line in Busy, Idle or Singletons by its lifetime, read only in a tree of the UI's client so that a singleton in another client's tree is in Busy or Idle, its status, its jobs, its tickets and its running children, with the chain of running runs from it to the deepest through the newest running child at each level, and what the deepest waits on, by name from its latest step where it is in a tree of the UI's client; below them, each run with no parent that ended in the window. *Test:* `runs_list_roots_by_section`, `runs_own_runs_wait_by_name`

**R-runs-07** A click on a line opens its root's page, and the button at its start unfolds it. A line unfolds to the runs its root started that run or ended in the window, newest first, at most 20 and the number of the rest; each with the chain below it, what it ended with from its last step or what its deepest waits on, where it is in a tree of the UI's client, and how many runs are under it, where there are any. *Test:* `runs_line_unfolds_to_the_runs_its_root_started`

**R-runs-02** The page is narrowed by label as `corbie run ls --label` reads it, and by location prefix, which keeps the runs at it and below it; a root or ended run the narrowing leaves is listed with the runs under it. *Test:* `runs_narrowing_holds_for_the_whole_page`

**R-runs-11** Narrowed to one location, the page heads the list with the location and its lifetime. With a client, the head offers Cancel every run here at a location in the client's tree, which runs `corbie run cancel <location>`; Send at a keyed location, which runs `corbie run send <location>#<key> <event> <data>`; and Start at a transient location in the client's tree, which runs `corbie run start <location>  --input <input>`, the data and input written out again from the JSON typed. The page shows the answer: for Send the receipt, and for Start the ID of the run it started. *Test:* `runs_keyed_runs_are_listed_by_key`, `runs_location_actions_run_their_commands`

## One run

**R-runs-04** A run in a tree of the UI's client is shown in full: what it waits on with each ticket's value and each timer's due time from its latest step, and Steps, one row for each thing that happened in it and in the runs under it, read from its record, each child's rows under the row that started it, with an ended child folded and a running one open. *Test:* `runs_own_run_shown_in_full`

**R-runs-08** The drawing beside Steps shows the selected row's run as of the row's step, from its record up to that step and the state that step left, inside the page's run with the cards unfolded down to it, each run on the way as of its latest step at or before the row's; a row of a run the page's run did not start is drawn alone. The row's details give the entries of that step, with the values emitted since the step before, in the words `corbie run show --record` prints. A send names the address or the client it went to and links to the run that took it or that it started, an undelivered send gives the reason, from the send or from the controller's note of it, a send whose event went to another run links to that run, a send to a keyed run by its ID gives the run's key, a call job links to the run it started, an event a run sent links to that run, and the path begins with the run whose send or call started the root. *Test:* `runs_a_step_shows_its_run_as_it_was`

**R-runs-12** A send or a call to a run in a tree of the UI's client is a group, folded, whose row ends with that run's title, where it is not its machine's name, and its status, and whose rows are read when it is opened, one level below the row: for a call every row of the run it started, and for a send the rows of the steps in the run that took the event whose causes, as Corbie names them, lead back to the send: the step that took the event, and each step that took the end of a job, a timer, a child or a call those steps asked for, or an event from a run they started. *Test:* `runs_a_send_shows_the_rows_it_caused`, `runs_a_call_shows_the_run_it_called`

**R-runs-13** On a running run's page the selected row is the newest row, which it follows as rows are added, until the person selects another row; then the head of Steps offers Follow, which selects the newest row and follows again. An ended run's page has no Follow. *Test:* `runs_follow_returns_to_the_newest_row`

**R-runs-14** Where the UI offers Machines, a run in a tree of the UI's client links from its title line and the head of its drawing to the Machines page opened on its machine: the file at its root's location, unfolded down the cards of the jobs that started each child, where the root's tree was submitted from the directory the page reads at the prefix it reads it at and no machine on the way was made while running; any other run has no link. The tooltip says the page shows the file as it is now, and the drawing the definition the run started with. *Test:* `runs_a_run_links_to_its_machine`

**C-runs-01** A file of a run's result is sent as a download, with `Content-Disposition: attachment` and `X-Content-Type-Options: nosniff`. A result can hold HTML or SVG a script fetched from anywhere, and shown in a page it would run with the UI's cookie. *Test:* `runs_result_file_is_a_download`

**C-runs-02** A run in another client's tree is read only through the list and Corbie's client package as a viewer, which gives its status and nothing from its record, on its page and in a send's or a call's group. The UI holds the controller's keys too, and must not read a run another way. *Test:* `runs_other_clients_run_shows_no_record`

## Acting on a run

**R-runs-05** Reply, Send and Cancel run `corbie run reply`, `corbie run send` and `corbie run cancel`, an answer and an event's data written out again from the JSON typed. Reply runs on any run with a ticket's value, Send on any running statechart, and Cancel on the runs in the client's trees; with the controller, Cancel on a run in another client's tree runs `corbie controller cancel`. *Test:* `runs_actions_run_their_commands`

## Interfaces

Runs uses Pages for the shell, the stream, its errors, the commands its buttons run and how a time is shown (`page.Shell`, `page.Write`, `page.Stream`, `page.Part`, `page.Error`, `page.Call`, `page.Command`, `page.Answer`, `page.When`, `page.Clock`, `page.Dated`), Drawing to draw a run's machine (`drawing.Draw`, `drawing.Run`, `drawing.Step`, `drawing.Child`, `drawing.Machine`), and Machines, as Serving gives it, for the address of a run's machine (`machines.Pages.Href`).
