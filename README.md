# Corbie UI

Corbie UI shows [Corbie](https://github.com/zummed/corbie)'s runs in a browser, and does to them what Corbie's commands do. It also draws a directory of machines, read as the tree `corbie run submit` records, and the toolkits their agents can open, and needs no store for that.

## Start

```sh
go install github.com/zummed/corbie-ui/cmd/corbie-ui@latest
corbie ui --dir my-project
```

`corbie ui` runs the program, and prints an address to open. With no store it shows the directory's machines. `specs/README.md` says what each way of starting it shows.

| Page | What it shows |
|---|---|
| Runs | every run, its record and what waits on a person |
| Machines | each machine in a directory, drawn from top to bottom |
| Toolkits | each toolkit, what offers it and what starts that |
| Controller | the runners and clients the controller trusts, its trees, and the secrets and values set at each location |

## License

Copyright 2026 Paul Young. Licensed under the Apache License, Version 2.0: see `LICENSE` and `NOTICE`. The IBM Plex fonts are under the SIL Open Font License 1.1, and Solid under the MIT License.

Corbie UI was written with Claude Code, designed and directed by Paul Young.
