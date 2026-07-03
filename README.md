# skiletto

Package manager for agent skills: manifest for intent, lockfile pinned to
commit SHAs, reproducible installs on any machine.

A skill is any directory containing a `SKILL.md` inside a git repository.
skiletto records what you want in `skiletto.toml`, pins exact commits and
content hashes in `skiletto.lock` (commit both), materializes skills in
`.agents/skills/<name>`, and symlinks them into each supported harness
(currently Claude Code's `.claude/skills/`).

Early development — see the [design doc](https://github.com/kumekay/skiletto/wiki)
for where this is headed.

## Usage

Run in your project root:

```sh
# add a skill: <repo>[//subdir][@ref]
skiletto add anthropics/skills//skills/pdf            # GitHub shorthand
skiletto add https://github.com/anthropics/skills//skills/pdf@main
skiletto add ssh://gitea@git.example.com:30009/me/skills.git//deploy
skiletto add ~/p/my-skills//my-skill                  # local git repo, pinned
skiletto add --editable ~/p/my-skills//my-skill       # link the working tree

# make installed skills match the lockfile exactly
skiletto sync
skiletto sync --force   # also restore drifted skills to their locked version
```

- `add` resolves the ref (or the default branch) to a commit SHA, records
  the skill in `skiletto.toml`, pins commit and content hash in
  `skiletto.lock`, installs it, and links it into every harness. If the
  source contains several skills and no `//path` was given, it lists them
  and exits.
- `sync` installs exactly what the lock pins and resolves+locks manifest
  entries that are not locked yet. It never re-resolves already-locked
  versions. Skills with local modifications (drift) are warned about and
  skipped with a non-zero exit; `--force` restores them. Entries removed
  from the manifest are pruned (drifted ones only with `--force`).
- `--editable` (local paths only) symlinks the working tree instead of
  copying a pinned commit, so edits are live; such entries carry no
  commit/hash and are never drift-checked.

## Development

Requires Go 1.26+ and system git.

```sh
lefthook install   # gofmt + golangci-lint on commit, tests on push
go test ./...
go build .
```

## License

MIT
