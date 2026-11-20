# Packs

A pack is a directory of ready-made machines, in a git repository or on this computer. `corbie add` writes a pack into a project, which is a directory of machines. The files stay in the project, so a submit, a controller and a runner read them as they read any file, and need no network and no git.

## What a pack holds

```
notes/chat/post.yaml                       a machine, written to .packs/notes/chat/post.yaml
notes/notes.js                             a script its machines use
notes/AGENTS.md                            what a model is told about the pack
notes/starter/TEAM/notes/TEAM.yaml         a starting file, written to ops/notes/ops.yaml
notes/starter/TEAM/notes/chat/post.yaml    a starting file of one line: include: .packs/notes/chat/post.yaml
```

- Every file of a pack outside `starter/` is written under `.packs/<pack>/`. `.corbieignore` lists `.packs/`, so none of them has a location.
- A starting file is a file of the project. It places a machine of the pack at a location with `include:`, or holds what a person changes.
- A pack holds no file that Corbie reads as configuration.

## Naming a pack

```
github.com/owner/repository/notes            the directory notes of a repository
github.com/owner/repository/notes@v0.4.0     at a tag
github.com/owner/repository/notes@try/this   at a branch
github.com/owner/repository/notes@3f2a9c1e   at a commit
host/a/b/c.git/notes                         a repository whose address has more than three parts
./packs/notes                                a directory on this computer, copied as it is
./packs/notes@try/this                       that directory, at a branch of the git repository it is in
```

The repository is the first three parts, and the pack's directory the rest. A repository holds any number of packs, each a directory. Without a ref, the pack is read at the repository's default branch.

## What corbie add does

```
corbie add github.com/owner/repository/notes@v0.4.0 --set TEAM=ops
```

1. It fetches the repository at the ref, with `git`, over HTTPS.
2. It removes `.packs/notes/` and writes the pack's files there.
3. It does the same for each pack beside `notes` in the repository that a `.yaml` or `.js` file of `notes` names, as `.packs/shared/`.
4. It writes each starting file at its path in the project, where no file is there and it has written none before.
5. It adds `.packs/` to the project's `.corbieignore` when it lacks it.

`corbie add` changes no other file. It prints each pack with its commit, each starting file it wrote, and each starting file that is there and is not as the pack starts it.

- `--set TEAM=ops` writes `ops` in place of `TEAM` in each starting file's path and text. A word is written in capitals, and its name in letters, digits, `_` and `-`.
- A starting file a person removed stays removed: removing a file that places a machine removes its location.
- A starting directory named in capitals is a word the pack takes, and `corbie add` refuses without it.
- A pack that another needs gets no starting files, and does not replace a pack a person added.
- `--in <directory>` names the project. By default it is the directory `corbie add` was started in.

## Updating

Each pack's directory holds a file `.pack`: where the pack came from, its commit, the words it was added with, and the starting files written.

```
corbie add                    adds each pack the project holds again, as its .pack records
corbie add <pack>@<ref>       adds one pack again, at another ref
```

A pack added from a branch is read at the branch's newest commit each time. A pack added at a tag or a commit stays as it is.

## What is trusted

A pack's scripts run with the permissions its machines give them, and with the secrets set at the locations its starting files place them at. A pack is trusted as any file a person copies into the project is, and Corbie checks nothing more. Its files are in the project to be read, and `git diff` shows what an update changed.

`corbie mcp` gives a model `corbie add` only when the server was started with `--allow add`.
