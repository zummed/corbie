# Packs

The detailed rules behind the [Packs overview](../pack.md).

## Adding

**R-pack-01** `corbie add <pack>` removes `.packs/<pack>/` in the project and writes there every file of the pack outside its `starter/` directory. It does the same for each pack beside it in its source that its `.yaml` and `.js` files name as `.packs/<name>/`, and for those that pack names. A pack that is needed and is not in the source is a failure that names both, and nothing is written.
*Accept:* a source holds `notes`, whose file names `.packs/shared/format.js`, and `shared`. After `corbie add <source>/notes --set TEAM=ops` the project holds `.packs/notes/` and `.packs/shared/` as the source has them, without `starter/`. A file added to `.packs/notes/` by hand is gone after the command is run again.

**R-pack-02** `corbie add` writes each file under the pack's `starter/` at the same path in the project where no file is there and it has written none before, with each word given as `--set WORD=<name>` written as the name in the file's path and text. It leaves a file that is there as it is, and names it when it is not as the pack starts it. A pack that another needs gets no starting files.
*Accept:* `notes/starter/TEAM/notes/TEAM.yaml` holds `statechart: TEAM`. `corbie add <source>/notes --set TEAM=ops` writes `ops/notes/ops.yaml` holding `statechart: ops`. With that file changed, the command run again leaves it and names it. With that file removed, the command run again does not write it.

**R-pack-03** `corbie add` changes no file of the project outside `.packs/`, but for the starting files it writes and the line `.packs/` it adds to a `.corbieignore` that lacks it.
*Accept:* a project's `.corbieignore` holds `people.yaml`. After `corbie add` it holds that line and `.packs/`, and after a second `corbie add` it is the same.

**R-pack-04** A starting directory named in capitals is a word the pack takes: `corbie add` without `--set` for it is refused, naming the word. A word set that no starting file of the pack holds is refused, and so is a name that is not letters, digits, `_` and `-` starting with a letter, so no starting file is written outside the project.
*Accept:* `corbie add <source>/notes` is refused with `the pack notes takes TEAM: add --set TEAM=<name>`, and `--set OTHER=x` with it is refused naming `OTHER`. `--set TEAM=../up` is refused, and nothing is written.

## Sources

**R-pack-05** A pack is named `host/owner/repository/path`, with `@<ref>` for a tag, a branch or a commit, or as a directory on this computer starting with `./` or `/`. `corbie add` fetches a repository with the `git` on PATH, and a failure of `git` is reported as git said it. A directory with no ref is copied as it is.
*Accept:* `github.com/owner/packs/notes@claude/try-packs` is the directory `notes` of `https://github.com/owner/packs` at the branch `claude/try-packs`. A directory in a git repository, written with `@try/branch`, is read as that branch has it while the directory holds another.

**R-pack-06** Each pack's directory holds `.pack`: its source or the pack that needs it, the commit it was read at, the words it was added with, and the starting files written. `corbie add` with no pack adds each pack a person added again, from that record.
*Accept:* after `corbie add <repository>/notes@try/branch --set TEAM=ops`, `.packs/notes/.pack` holds the source, the commit of the branch and `set TEAM=ops`.

## Interfaces

- **Client.** The command line reads `corbie add`'s line and prints what `pack.Add` reports. The package uses no other area.
