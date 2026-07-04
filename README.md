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

## Install

With a Go toolchain (works today):

```sh
go install github.com/kumekay/skiletto@latest
```

Prebuilt binaries and the `npm` / `pip` wrappers land with the first tagged
release:

```sh
npm install -g skiletto     # coming with the first release
pip install skiletto        # coming with the first release
```

Or download a binary for your platform from the
[releases page](https://github.com/kumekay/skiletto/releases) (coming with the
first release).

## Usage

Run in your project root:

```sh
# add a skill: <repo>[//subdir][@ref]
skiletto add anthropics/skills//skills/pdf            # GitHub shorthand
skiletto add https://github.com/anthropics/skills//skills/pdf@main
skiletto add ssh://gitea@git.example.com:30009/me/skills.git//deploy
skiletto add ~/p/my-skills//my-skill                  # local git repo, pinned
skiletto add --editable ~/p/my-skills//my-skill       # link the working tree

# a source with several skills and no //path: pick interactively, or
skiletto add --all anthropics/skills                  # install every skill in it

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
  source contains several skills and no `//path` picks one, `add` shows a
  multi-select picker in a terminal and installs everything you check.
  `--all` installs every skill without prompting. With no TTY — or with
  `--no-input`, or when the `CI` env var is set — it instead prints the
  skills and the exact `//path` (or `--all`) commands to script the choice,
  and exits non-zero, so scripts and CI never hang on a prompt.
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
  Both the legacy version 1 and the current version 3 lock formats are read
  (version 3 records `skillPath` as the `SKILL.md` file, whose directory
  import recovers). It maps each entry to a canonical git source (`github`
  and `git` sourceTypes; `local` and `well-known` entries are reported with
  guidance instead), resolves the default-branch HEAD to a commit — Vercel's lock
  stores no ref or SHA, so HEAD is the best available pin — and installs and
  links each skill. Entries already in the manifest are skipped; entries that
  cannot be mapped or resolved are reported and cause a non-zero exit without
  aborting the ones that do resolve. Import is one-way. `--global` writes the
  machine scope. Installed trees that import cannot prove pristine (drifted
  lock orphans, unmanaged skill dirs) are refused unless `--force` replaces
  them. Migrating: real skill directories that `npx skills` left in
  `.claude/skills/` are never touched — import refuses each one and names it;
  remove the old copy (`rm -r .claude/skills/<name>`) and re-run import.
- `--editable` (local paths only) symlinks the working tree instead of
  copying a pinned commit, so edits are live; such entries carry no
  commit/hash and are never drift-checked.
- `--global` (on `add` and `sync`) switches to the machine scope: the
  manifest and lock live in the platform config dir (`~/.config/skiletto/`
  on Linux), skills materialize in `~/.agents/skills/`, and the Claude
  adapter links into `~/.claude/skills/`. Local path and editable sources
  are the normal case here, so `add` skips the portability warning.
- `--no-input` (on any command) forces the non-interactive path: instead of
  prompting, skiletto fails with an actionable error listing the flags that
  script the choice. A set `CI` env var implies it.

## Windows

Linking a skill into a harness dir tries three strategies in order:

1. **symlink** — works when Developer Mode is on (or the shell is elevated);
2. **directory junction** — the normal Windows path, no privilege required;
3. **copy** — a last resort when neither reparse point can be created.

skiletto records nothing about which strategy it used: symlinks and junctions
identify themselves, and a copy is recognized as skiletto's own only when its
contents still hash-match the canonical `.agents/skills/<name>` tree. A copied
link that has diverged is treated like any other local modification — `sync`,
`update`, and `remove` refuse to overwrite or delete it without `--force`.

Two consequences of the copy fallback:

- A **pristine** copy — one that still matches the canonical tree — behaves
  exactly like a symlink: `sync` re-links over it, `update` refreshes it in
  place, `remove` deletes it, none of them need `--force`. Only a copy you
  have edited by hand is refused without `--force`.
- **Editable installs need a symlink or a junction** — a copy cannot stay
  live. On a filesystem where only copying works, `skiletto add --editable`
  fails with a clear message instead of silently installing a stale snapshot.

On Linux and macOS the behavior is unchanged: skiletto symlinks, or fails.

## Development

Requires Go 1.26+ and system git.

```sh
lefthook install   # gofmt + golangci-lint on commit, tests on push
go test ./...
go build .
```

## License

MIT
