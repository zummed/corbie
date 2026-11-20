# Serving

The rules behind [the Serving overview](../serve.md). Area code: `serve`.

## Starting it

**R-serve-01** `corbie ui` acts as the parties named after it, each from the config; with nothing named, as every party the config holds; and as `none`, or on a config that holds no party, it is a viewer. `none` beside a party is a mistake. *Test:* `serve_acts_as_the_parties_named`

**R-serve-03** With nothing named, a store that cannot be opened leaves the directory alone: the UI prints why, and offers Machines and Toolkits to read. A party or `none` named needs the store, and the UI refuses to start without it. *Test:* `serve_directory_alone_needs_no_store`

**C-serve-01** A UI refuses to listen on any address but this computer's, unless `none` is named. The UI has no TLS, so on another address the token, a secret's value and every definition in the directory would cross the network in clear text. *Test:* `serve_party_listens_on_this_computer_only`

**C-serve-02** With both parties, the client must trust the config's controller, or the UI refuses to start. A client may trust another controller, and the list and the Controller page would then act on two stores. *Test:* `serve_client_must_trust_the_configs_controller`

## What each party opens

**R-serve-02** Every UI but `none` opens Machines and Toolkits to read; no party opens Runs and /metrics where there is a store; the controller opens the Controller page; a page the UI may not offer answers that there is no such page. *Test:* `serve_each_party_opens_its_pages`

**C-serve-04** A value from a page that begins with `-` is refused before any command runs. Corbie's command line reads flags anywhere among a command's arguments, so a value such as `--config=/tmp/other` would run the command as another config's party. *Test:* `serve_page_value_cannot_become_a_flag`

## The token

**C-serve-05** A request that does not show the token is refused, and does nothing. *Test:* `serve_request_without_the_token_is_refused`

**C-serve-06** A token in an address is taken once: the browser is sent back to the address without it, holding a cookie named for the port, which scripts cannot read and other sites cannot send. Left in the address, the token would stay in the browser's history. *Test:* `serve_token_in_the_address_becomes_a_cookie`

**C-serve-07** A request that a page of another origin makes is refused. A page on another port of this computer is the same site to a browser, and its requests carry the cookie. *Test:* `serve_request_from_another_origin_is_refused`

## What it keeps

**C-serve-08** The list is read again at every interval, and a run no longer listed leaves it. Corbie's watch reports no run that leaves the store after its keep, and no error after its first look. *Test:* `serve_run_that_leaves_the_store_leaves_the_list`

## Interfaces

Serving uses every other area: it gives Runs and Metrics the list of runs, Runs, Machines and Controller the parties, Runs and Controller the way a command runs, Machines the directory, Runs the address Machines gives of a run's machine where the UI offers Machines, and serves the assets of Pages.
