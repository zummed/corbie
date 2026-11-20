# Packs

A pack is a directory of ready-made machines, in a git repository or on this computer. `corbie add` writes a pack into a project, which is a directory of machines. The files stay in the project, so a submit, a controller and a runner read them as they read any file, and need no network and no git.

## What a pack holds

```
slack/chat/post.yaml                      a machine, written to .packs/slack/chat/post.yaml
slack/slack.js                            a script its machines use
slack/AGENTS.md                           what a model is told about the pack
slack/starter/BOT/slack/BOT.yaml          a starting file, written to grouse/slack/grouse.yaml
slack/starter/BOT/slack/chat/post.yaml    a starting file of one line: include: .packs/slack/chat/post.yaml
```

- Every file of a pack outside `starter/` is written under `.packs/<pack>/`. `.corbieignore` lists `.packs/`, so none of them has a location.
- A starting file is a file of the project. It places a machine of the pack at a location with `include:`, or holds what a person changes.
- A pack holds no file that Corbie reads as configuration.

## Naming a pack

```
github.com/owner/repository/slack            the directory slack of a repository
github.com/owner/repository/slack@v0.4.0     at a tag
github.com/owner/repository/slack@try/this   at a branch
github.com/owner/repository/slack@3f2a9c1e   at a commit
host/a/b/c.git/slack                         a repository whose address has more than three parts
./packs/slack                                a directory on this computer, copied as it is
./packs/slack@try/this                       that directory, at a branch of the git repository it is in
```

The repository is the first three parts, and the pack's directory the rest. A repository holds any number of packs, each a directory. Without a ref, the pack is read at the repository's default branch.

## What corbie add does

```
corbie add github.com/zummed/corbie-rook/slack@v0.4.0 --set BOT=grouse
```

1. It fetches the repository at the ref, with `git`, over HTTPS.
2. It removes `.packs/slack/` and writes the pack's files there.
3. It does the same for each pack beside `slack` in the repository that the files of `slack` name, as `.packs/shared/`.
4. It writes each starting file at its path in the project, where no file is there.
5. It adds `.packs/` to the project's `.corbieignore` when it lacks it.

`corbie add` changes no other file. It prints each pack with its commit, each starting file it wrote, and each starting file that is there and is not as the pack starts it.

- `--set BOT=grouse` writes `grouse` in place of `BOT` in each starting file's path and text. A word is written in capitals.
- A starting directory named in capitals is a word the pack takes, and `corbie add` refuses without it.
- A pack that another needs gets no starting files, and does not replace a pack a person added.
- `--in <directory>` names the project. By default it is the directory `corbie add` was started in.

## Updating

Each pack's directory holds a file `.pack`: where the pack came from, its commit, and the words it was added with.

```
corbie add                    adds each pack the project holds again, as its .pack records
corbie add <pack>@<ref>       adds one pack again, at another ref
```

A pack added from a branch is read at the branch's newest commit each time. A pack added at a tag or a commit stays as it is.

## What is trusted

A pack's scripts run with the permissions its machines give them, and with the secrets set at the locations its starting files place them at. A pack is trusted as any file a person copies into the project is, and Corbie checks nothing more. Its files are in the project to be read, and `git diff` shows what an update changed.

`corbie mcp` gives a model `corbie add` only when the server was started with `--allow add`.
