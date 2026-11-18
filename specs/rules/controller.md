# Controller

The rules behind [the Controller overview](../controller.md). Area code: `controller`.

## Runners and clients

**R-controller-01** The page lists each runner and client the controller trusts with its card's fingerprint, a runner's prefixes and tags, a client's ceiling and deny list, and whether it was removed, read as `corbie controller parties` reads them; each tree the controller recorded, with its prefix, its client, the directory last submitted there and each location's lifetime; and the secrets by name and the values with their text, each at the location it is set at, read as `corbie controller secret list` and `corbie controller value list` read them. *Test:* `controller_lists_parties_trees_secrets_and_values`

**R-controller-02** Enrol, Tags, Prefixes, Ceiling, Remove, a value's Set and a secret's or a value's Remove run their `corbie controller` commands, a runner's prefixes as `--at` once for each, a secret's and a value's at the location given, `/` by default, and Enrol shows the controller's card and the store's address that the command printed back. *Test:* `controller_actions_run_their_commands`

**C-controller-01** A removed party's line offers only Enrol. `corbie controller enrol <name>` writes a new trust for the name, which trusts a removed party again. *Test:* `controller_removed_party_offers_only_enrol`

## Secrets and values

**C-controller-02** A secret's value goes to its command on standard input, and no page and no answer carries it. On a command line it would be seen by every process on this computer. *Test:* `controller_secret_value_goes_on_standard_input_only`

## Interfaces

Controller uses Pages for the shell, the stream, its errors and the commands it runs (`page.Shell`, `page.Write`, `page.Stream`, `page.Part`, `page.Error`, `page.Call`, `page.Command`, `page.Answer`).
