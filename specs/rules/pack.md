# Packs

The detailed rules behind the [Packs overview](../pack.md).

## Adding

**R-pack-01** `corbie add <pack>` removes `.packs/<pack>/` in the project and writes there every file of the pack outside its `starter/` directory. It does the same for each pack beside it in its source that its files name as `.packs/<name>/`, and for those that pack names. A pack that is needed and is not in the source is a failure that names both, and nothing is written.
*Accept:* a source holds `slack`, whose file names `.packs/shared/format.js`, and `shared`. After `corbie add <source>/slack --set BOT=grouse` the project holds `.packs/slack/` and `.packs/shared/` as the source has them, without `starter/`. A file added to `.packs/slack/` by hand is gone after the command is run again.

**R-pack-02** `corbie add` writes each file under the pack's `starter/` at the same path in the project where no file is there, with each word given as `--set WORD=<name>` written as the name in the file's path and text. It leaves a file that is there as it is, and names it when it is not as the pack starts it. A pack that another needs gets no starting files.
*Accept:* `slack/starter/BOT/slack/BOT.yaml` holds `statechart: BOT`. `corbie add <source>/slack --set BOT=grouse` writes `grouse/slack/grouse.yaml` holding `statechart: grouse`. With that file changed, the command run again leaves it and names it.

**R-pack-03** `corbie add` changes no file of the project outside `.packs/`, but for the starting files it writes and the line `.packs/` it adds to a `.corbieignore` that lacks it.
*Accept:* a project's `.corbieignore` holds `people.yaml`. After `corbie add` it holds that line and `.packs/`, and after a second `corbie add` it is the same.

**R-pack-04** A starting directory named in capitals is a word the pack takes: `corbie add` without `--set` for it is refused, naming the word. A word set that no starting file of the pack holds is refused.
*Accept:* `corbie add <source>/slack` is refused with `the pack slack takes BOT: add --set BOT=<name>`, and `--set TEAM=x` with it is refused naming `TEAM`.

## Sources

**R-pack-05** A pack is named `host/owner/repository/path`, with `@<ref>` for a tag, a branch or a commit, or as a directory on this computer starting with `./` or `/`. `corbie add` fetches a repository with the `git` on PATH, and a failure of `git` is reported as git said it. A directory with no ref is copied as it is.
*Accept:* `github.com/zummed/packs/slack@claude/try-packs` is the directory `slack` of `https://github.com/zummed/packs` at the branch `claude/try-packs`. A directory in a git repository, written with `@try/branch`, is read as that branch has it while the directory holds another.

**R-pack-06** Each pack's directory holds `.pack`: its source or the pack that needs it, the commit it was read at, and the words it was added with. `corbie add` with no pack adds each pack a person added again, from that record.
*Accept:* after `corbie add <repository>/slack@try/branch --set BOT=grouse`, `.packs/slack/.pack` holds the source, the commit of the branch and `set BOT=grouse`.

## Interfaces

- **Client.** The command line reads `corbie add`'s line and prints what `pack.Add` reports. The package uses no other area.
