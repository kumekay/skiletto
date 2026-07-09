package engine

import (
	"fmt"

	"github.com/kumekay/skiletto/internal/manifest"
)

// preInstallHook returns the pre-install hook command for this run: the
// scope manifest's, or the machine manifest's when the scope has none
// (personal hooks apply in every project, like harnesses). Empty means no
// hook. NoHooks (the --no-hooks flag) disables it entirely.
func (e *Engine) preInstallHook(m *manifest.Manifest) string {
	if e.NoHooks {
		return ""
	}
	if cmd, ok := m.Hooks["pre-install"]; ok {
		return cmd
	}
	if e.Machine == nil || e.Machine.ManifestPath == e.Scope.ManifestPath {
		return ""
	}
	mm, err := manifest.Load(e.Machine.ManifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(e.Err, "warning: %v\n", err)
		return ""
	}
	return mm.Hooks["pre-install"]
}

// runPreInstall executes the hook command against a staged skill directory,
// before the content is promoted or locked. The directory is appended to
// the command as its last argument and also exported as SKILETTO_SKILL_DIR,
// alongside SKILETTO_SKILL_NAME, SKILETTO_SOURCE, SKILETTO_COMMIT, and
// SKILETTO_EVENT (add, update, sync, or import). The hook's output streams
// through; a non-zero exit aborts the install.
func (e *Engine) runPreInstall(command, name, src, commit, event, dir string) error {
	if command == "" {
		return nil
	}
	cmd := hookCmd(command, dir)
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
		return fmt.Errorf("pre-install hook rejected %s (%v); nothing was installed (rerun with --no-hooks to bypass)", name, err)
	}
	return nil
}
