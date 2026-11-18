# Controller

The Controller page lists the runners and clients the controller trusts, the trees it has recorded, and the secrets and values set at each location, and changes the parties, secrets and values. It is offered when the UI acts as the controller.

The page reads the store through Corbie's packages: the parties as `corbie controller parties` reads them, the secrets and values as `corbie controller secret list` and `corbie controller value list` read them, and each tree as the controller recorded it at its prefix. Each action runs its `corbie controller` command, with the controller's config, which is why the page needs the controller.

## Runners and clients

Each is one line: its name, its card's fingerprint, and a runner's prefixes and tags or a client's ceiling and deny list. A runner enrolled with no `--at` is listed at `/`. One that was removed is listed as removed.

```
Enrol      corbie controller enrol <card> --tags <list> --at <prefix>   trusts a runner, with its tags and prefixes
Enrol      corbie controller enrol <card> --ceiling <list>              trusts a client, with its ceiling
Tags       corbie controller enrol <name> --tags <list>                 sets a runner's tags again
Prefixes   corbie controller enrol <name> --at <prefix>                 sets a runner's prefixes again
Ceiling    corbie controller enrol <name> --ceiling <list>              sets a client's ceiling again
Remove     corbie controller remove <name>                              stops trusting it
```

Enrol shows what the command prints back, the controller's card and the store's address, for the other side's `corbie runner trust` or `corbie client trust`. A card with another key under a name already used is refused, unless Replace is ticked, as `--replace` allows it.

A runner's prefixes are typed as a list, such as `/home,/netops/tools`, and go to the command as `--at` once for each. Tags leaves a runner's prefixes as they are, and Prefixes leaves its tags. A runner set to the prefix `/` is sent every tree's jobs.

Remove keeps the name with its key, marked as removed and signed by the controller. A removed party's line offers only Enrol, since setting its tags, prefixes or ceiling again would trust it again.

## Trees

Each tree is a line with its prefix, the client that owns it, and the directory last submitted there. Its locations follow, each with its lifetime:

```
/home                 tree of laptop-client, from /home/paul/house
  devices/boiler      singleton
  rooms/thermostat    keyed
  tools/lookup        transient
```

- A location is written from its tree's prefix. A location the prefix names in full keeps its last name.
- A tree and each location link to the Runs page narrowed to them.
- The page changes no tree: only a tree's client submits there.

## Secrets and values

Secrets and values are listed by the location they are set at, `/` first, then by name. A secret is listed by name; a value with its text.

```
/       ANTHROPIC_KEY   secret
/home   HUB             value    192.0.2.8:8123
```

```
Set      corbie controller secret set <NAME> - --at <location>          keeps a secret at the location, in place of any kept there before
Set      corbie controller value set <NAME> <VALUE> --at <location>     keeps a value at the location, in place of any kept there before
Remove   corbie controller secret remove <NAME> --at <location>         deletes the secret set at the location
Remove   corbie controller value remove <NAME> --at <location>          deletes the value set at the location
```

- The location field is `/` by default, which every tree reads.
- A secret is typed into a field that does not show it, and goes to the command on its standard input, never on its command line. No page shows a secret, and none is sent back.
- A value is typed in plain text and goes on the command line, as `corbie controller value set` takes it.
