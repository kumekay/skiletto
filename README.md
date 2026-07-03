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

# re-resolve refs to the current commit and rewrite the lock
skiletto update         # every skill
skiletto update pdf     # just one
skiletto update --force # overwrite drifted skills too

# drop a skill from the manifest, lock, links, and disk
skiletto remove pdf
skiletto remove --force pdf   # remove even if it has local modifications

# show managed skills (with drift status) and unmanaged ones
skiletto list

# bootstrap from a Vercel `npx skills` skills-lock.json (one-way)
skiletto import                 # reads ./skills-lock.json
skiletto import path/to/skills-lock.json

# --global installs machine-wide instead of into the current project
skiletto add --global --editable ~/p/my-skills//my-skill
skiletto sync --global
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
- `update` is the only command that moves already-locked versions: it
  re-resolves each entry's ref (or default branch) to the current commit,
  re-materializes and re-links it, and rewrites the lock. With no argument it
  updates every skill; with a name, only that one. Editable entries have
  nothing to re-resolve and are skipped; drifted skills are skipped with a
  non-zero exit unless `--force` overwrites them.
- `remove` drops a skill from the manifest and lock, unlinks it from every
  harness, and deletes its materialized copy. An editable skill loses only its
  canonical link — the working tree is left untouched. A drifted skill is
  refused unless `--force`, since removal discards local edits.
- `list` shows each managed skill with its pinned commit (or `editable`) and
  status (`ok`, `drifted`, `missing`, `not-locked`). Skills still in the lock
  but removed from the manifest show `pruned on next sync`; skills found in
  the skills dir or a harness dir but absent from the manifest show
  `unmanaged`. It only observes.
- `import` bootstraps `skiletto.toml` and `skiletto.lock` from a Vercel
  `npx skills` `skills-lock.json` (default: one in the current directory).
  It maps each entry to a canonical git source (`github` and `git`
  sourceTypes), resolves the default-branch HEAD to a commit — Vercel's lock
  stores no ref or SHA, so HEAD is the best available pin — and installs and
  links each skill. Entries already in the manifest are skipped; entries that
  cannot be mapped or resolved are reported and cause a non-zero exit without
  aborting the ones that do resolve. Import is one-way. `--global` writes the
  machine scope.
- `--editable` (local paths only) symlinks the working tree instead of
  copying a pinned commit, so edits are live; such entries carry no
  commit/hash and are never drift-checked.
- `--global` (on `add` and `sync`) switches to the machine scope: the
  manifest and lock live in the platform config dir (`~/.config/skiletto/`
  on Linux), skills materialize in `~/.agents/skills/`, and the Claude
  adapter links into `~/.claude/skills/`. Local path and editable sources
  are the normal case here, so `add` skips the portability warning.

## Development

Requires Go 1.26+ and system git.

```sh
lefthook install   # gofmt + golangci-lint on commit, tests on push
go test ./...
go build .
```

## License

MIT
