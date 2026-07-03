# Skiletto

Package manager for agent skills: manifest (`skiletto.toml`) for user intent,
lockfile (`skiletto.lock`) pinned to commit SHAs, reproducible installs.

**The design doc lives in the GitHub wiki and is the source of truth** for
behavior, file formats, CLI semantics, and architecture:
https://github.com/kumekay/skiletto/wiki. Read it before changing anything
substantial.

## Development rules

- **Red-green TDD, no exceptions.** Write a failing test first, run it and
  watch it fail, then write the minimum code to make it pass, then refactor.
  No production code without a failing test that demanded it.
- Hooks: run `lefthook install` once after cloning. Pre-commit runs gofmt and
  golangci-lint; pre-push runs `go test ./...`. CI runs the same hooks plus
  tests, so if hooks fail locally, CI fails too.
- Thin CLI layer: `internal/cli` only parses flags/args, calls
  `internal/engine`, and formats output. Business logic never lives in
  command handlers.
- Commit messages and PR bodies: imperative, describe the change, nothing
  else. No tool attribution, no generated-by lines, no co-author trailers.

## Easy to miss

- **Keep the wiki in sync.** Any change to file formats, CLI behavior, the
  install model, or architecture must be reflected in the wiki design doc as
  part of the same piece of work
  (`git clone git@github.com:kumekay/skiletto.wiki.git`, edit `Home.md`, push).
- **Keep README.md honest.** It must describe the actually-implemented CLI —
  never planned-but-unbuilt features.
- `sync` never moves locked versions; only `update` re-resolves refs. `sync`
  prunes only entries it itself removes from the lock, and never deletes a
  drifted skill without `--force`.
- Drifted skills (content hash mismatch): warn and skip, never overwrite
  silently.
- `skiletto.toml` and `skiletto.lock` contain only canonical URLs —
  shorthands like `owner/repo` are expanded at `add` time, in the CLI layer.
- Editable (path) skills have no commit/hash in the lock and are excluded
  from drift checks.
- Every interactive prompt must have a flag equivalent. No TTY + ambiguous
  invocation = actionable error listing choices; never hang waiting for input.
