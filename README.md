# skiletto

Package manager for agent skills: manifest for intent, lockfile pinned to
commit SHAs, reproducible installs on any machine.

A skill is any directory containing a `SKILL.md` inside a git repository.
skiletto records what you want in `skiletto.toml`, pins exact commits and
content hashes in `skiletto.lock` (commit both), materializes skills in
`.agents/skills/<name>`, and symlinks them into each enabled harness (e.g.
Claude Code's `.claude/skills/`; run `skiletto harness list` for the full
set). The canonical `.agents/skills/` dir is always populated — harnesses
that read it directly need no links at all; the rest are opt-in via
`skiletto harness`.

Early development — see the [design doc](https://github.com/kumekay/skiletto/wiki)
for where this is headed.

## Install

```sh
npm install -g skiletto
# or
pip install skiletto
```

Or download a prebuilt binary for your platform from the
[releases page](https://github.com/kumekay/skiletto/releases), or build from
source with a Go toolchain:

```sh
go install github.com/kumekay/skiletto@latest
```

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

# choose which harnesses get per-skill links (saved in skiletto.toml)
skiletto harness list                     # registered harnesses + where enabled
skiletto harness enable claude            # this project
skiletto harness enable claude --global   # machine-wide, applies in every project
skiletto harness disable claude

# --global (-g) installs machine-wide instead of into the current project
skiletto add --global --editable ~/p/my-skills//my-skill
skiletto sync -g
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
- `harness` controls which harnesses get per-skill links, via a `harnesses`
  key in the manifest. The effective set is the union of the project
  manifest's key and the machine manifest's, so personal harnesses apply
  everywhere without touching a shared `skiletto.toml`. `enable` links every
  installed skill immediately; `disable` unlinks (warning when the harness
  stays enabled machine-wide); `sync` keeps links reconciled with the key.
  A scope with no key anywhere gets a one-time picker on the first
  interactive `add`/`sync`/`import` (the answer is saved); without a TTY the
  command installs to the canonical dir only and prints a note — never an
  error, since canonical-only is always safe. If a harness link path already
  resolves to the canonical directory (say `.claude/skills` is your own
  symlink to `.agents/skills`), skiletto treats it as linked and touches
  nothing.
- `import` bootstraps `skiletto.toml` and `skiletto.lock` from a Vercel
  `npx skills` `skills-lock.json` (default: one in the current directory).
  Only the current version 3 lock format is read; older versions are
  rejected, since Vercel wipes them and starts fresh, so none survive on
  disk. The lock records `skillPath` as the `SKILL.md` file, whose directory
  import recovers; a repo-root skill becomes path `.`, which pins the source
  root itself even when the repo also contains nested skills. It maps each
  entry to a canonical git source (`github`
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
- `--global` / `-g` (on any command) switches to the machine scope: the
  manifest and lock live in the platform config dir (`~/.config/skiletto/`
  on Linux), skills materialize in `~/.agents/skills/`, and the Claude
  adapter links into `~/.claude/skills/`. Local path and editable sources
  are the normal case here, so `add` skips the portability warning. The
  machine scope is always explicit: running in your home directory without
  `--global` is an error, not a project rooted at `~` — a home-rooted
  "project" would silently share `~/.agents/skills` with the machine scope.
- `--no-input` (on any command) forces the non-interactive path: instead of
  prompting, skiletto fails with an actionable error listing the flags that
  script the choice. A set `CI` env var implies it.
- `--verbose` / `-v` (on any command) prints extra diagnostics to stderr,
  including a line for each pre-install hook run naming the skill and the
  event. Because `-v` means verbose, `--version` has no shorthand.

## Pre-install hook

skiletto can run a security scanner — or any command — over a skill's
content before it is installed. Configure it under `[hooks]` in the
**machine-scope** manifest (`~/.config/skiletto/skiletto.toml` on Linux):

```toml
[hooks]
pre-install = 'skillspector scan --no-llm "$SKILETTO_SKILL_DIR"'
```

- The command runs after the skill's content is fetched into a staging
  directory and before anything is installed or locked. The staged directory
  is exported as `SKILETTO_SKILL_DIR` — reference it in the command
  (`"$SKILETTO_SKILL_DIR"`; `%SKILETTO_SKILL_DIR%` on Windows) — alongside
  `SKILETTO_SKILL_NAME`, `SKILETTO_SOURCE`, `SKILETTO_COMMIT`, and
  `SKILETTO_EVENT` (`add`, `update`, `sync`, or `import`). Exit 0 lets the
  install proceed; any other exit aborts it with nothing changed — on disk,
  in the manifest, or in the lock.
- The hook gates content entering the lock: it runs on `add`, `update`,
  `import`, and on `sync` only for manifest entries that are not locked yet.
  Re-installing already-locked content (`sync` materializing from the lock)
  skips it — the lock's content hash guarantees those are byte-for-byte the
  contents that were scanned when the entry was locked. Editable skills are
  never scanned: their working tree changes after any scan.
- Hooks run only from the machine manifest, and it applies in every
  project. A `[hooks]` table in a project's `skiletto.toml` is ignored with
  a warning: hooks execute arbitrary commands, so a cloned repository must
  not be able to supply one — or to replace your scanner. `--no-hooks` (on
  `add`/`sync`/`update`/`import`) skips the hook for one run.
- The gate fails closed: an unreadable machine manifest or an unknown name
  under `[hooks]` makes installs fail rather than silently skipping the
  hook.
- The command runs through `sh` (`cmd.exe` on Windows), so it can carry its
  own flags and environment variables.

### Example: scanning skills with SkillSpector

[SkillSpector](https://github.com/NVIDIA/skillspector) is NVIDIA's security
scanner for agent skills. Its exit codes fit the hook contract directly:
0 (safe or caution), 1 (do not install), 2 (scan error) — so a risky skill
blocks the install. Install it once with uv:

```sh
uv tool install git+https://github.com/NVIDIA/skillspector.git
```

The `[hooks]` entry above runs its static analysis: fast, offline, and the
skill's contents never leave your machine. For the full scan with LLM
semantic analysis, use the `claude_cli` provider — it drives your locally
installed, already-authenticated `claude` CLI, so no API key is configured
anywhere:

```toml
[hooks]
pre-install = 'SKILLSPECTOR_PROVIDER=claude_cli skillspector scan "$SKILETTO_SKILL_DIR"'
```

Note that LLM analysis sends the scanned skill's file contents to the model
provider; keep `--no-llm` if they must stay local.

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
