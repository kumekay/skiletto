package engine

import (
	"fmt"

	"github.com/kumekay/skiletto/internal/manifest"
)

// HookRejectedError reports a pre-install hook that exited non-zero. The
// install was aborted before anything on disk, in the manifest, or in the
// lock changed, so callers must not clean up previously installed state.
type HookRejectedError struct {
	Name string
	Err  error
}

func (e *HookRejectedError) Error() string {
	return fmt.Sprintf("pre-install hook rejected %s (%v); nothing was installed (rerun with --no-hooks to bypass)", e.Name, e.Err)
}

func (e *HookRejectedError) Unwrap() error { return e.Err }

// preInstallHook returns the pre-install hook command for this run, read
// from the machine manifest only. Hooks execute arbitrary commands, so a
// project's checked-in skiletto.toml must not supply one: a cloned
// repository would gain code execution on sync and could replace the
// user's scanner. A [hooks] table in a project manifest is reported and
// ignored. The gate fails closed: an unreadable machine manifest or an
// unknown hook name is an error, not a warning. NoHooks (the --no-hooks
// flag) disables the hook entirely.
func (e *Engine) preInstallHook(m *manifest.Manifest) (string, error) {
	if e.NoHooks {
		return "", nil
	}
	if e.Machine != nil && e.Machine.ManifestPath == e.Scope.ManifestPath {
		return knownHook(m, e.Scope.ManifestPath)
	}
	if len(m.Hooks) > 0 {
		_, _ = fmt.Fprintf(e.Err, "warning: hooks run only from the machine manifest; ignoring [hooks] in %s\n", e.Scope.ManifestPath)
	}
	if e.Machine == nil {
		return "", nil
	}
	mm, err := manifest.Load(e.Machine.ManifestPath)
	if err != nil {
		return "", fmt.Errorf("reading the machine manifest for the pre-install hook: %w", err)
	}
	return knownHook(mm, e.Machine.ManifestPath)
}

// knownHook returns the manifest's pre-install command, rejecting hook
// names this version does not know so a typo cannot silently disable the
// gate.
func knownHook(m *manifest.Manifest, path string) (string, error) {
	for name := range m.Hooks {
		if name != "pre-install" {
			return "", fmt.Errorf("unknown hook %q in %s; supported hooks: pre-install", name, path)
		}
	}
	return m.Hooks["pre-install"], nil
}

// runPreInstall executes the hook command against a staged skill directory,
// before the content is hashed, promoted, linked, or locked. The directory
// is exported as SKILETTO_SKILL_DIR — the command references it itself
// ("$SKILETTO_SKILL_DIR"; %SKILETTO_SKILL_DIR% on Windows) — alongside
// SKILETTO_SKILL_NAME, SKILETTO_SOURCE, SKILETTO_COMMIT, and SKILETTO_EVENT
// (add, update, sync, or import). The hook's output streams through; a
// non-zero exit aborts the install with a *HookRejectedError.
func (e *Engine) runPreInstall(command, name, src, commit, event, dir string) error {
	if command == "" {
		return nil
	}
	cmd := hookCmd(command)
	cmd.Env = append(cmd.Environ(),
		"SKILETTO_SKILL_DIR="+dir,
		"SKILETTO_SKILL_NAME="+name,
		"SKILETTO_SOURCE="+src,
		"SKILETTO_COMMIT="+commit,
		"SKILETTO_EVENT="+event,
	)
	cmd.Stdout = e.Out
	cmd.Stderr = e.Err
	if err := cmd.Run(); err != nil {
		return &HookRejectedError{Name: name, Err: err}
	}
	return nil
}
