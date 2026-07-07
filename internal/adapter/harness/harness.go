// Package harness holds the compiled-in harness adapters. Every adapter is a
// data row: harnesses differ only in where they keep their skills and how
// they are detected, never in how a skill directory is linked, so a single
// Config type driven by a table replaces one hand-written adapter per
// harness. The shared link helper in package adapter owns the link mechanism.
package harness

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/scope"
)

// Config describes one harness. projectDir and userDir are slash-relative path
// templates: projectDir hangs off the repo root, userDir off the user's home
// directory. filepath.Join rewrites separators, so Windows is handled.
// userDirEnv, when set, names an environment variable that overrides userDir
// with an absolute path (e.g. CLAUDE_CONFIG_DIR). At machine scope the skills
// dir is always <userDir>/skills and the harness is detected when <userDir>
// exists.
type Config struct {
	name       string
	projectDir string
	userDir    string
	userDirEnv string
}

// Name returns the adapter name (e.g. "claude").
func (c Config) Name() string { return c.name }

// userDirFor resolves the harness's user-level directory: the userDirEnv
// override if set to a non-empty value, else home joined with userDir. home is
// the machine scope root, injected so tests never touch the real home.
func (c Config) userDirFor(home string) string {
	if c.userDirEnv != "" {
		if v := strings.TrimSpace(os.Getenv(c.userDirEnv)); v != "" {
			return v
		}
	}
	return filepath.Join(home, filepath.FromSlash(c.userDir))
}

// SkillsDir is the project skills dir under the repo root, or the user-level
// skills dir under the harness's user directory, depending on the scope kind.
func (c Config) SkillsDir(s scope.Scope) string {
	if s.Kind == scope.KindMachine {
		return filepath.Join(c.userDirFor(s.Root), "skills")
	}
	return filepath.Join(s.Root, filepath.FromSlash(c.projectDir))
}

// Link makes target visible as <skills dir>/<name>, preferring a relative
// target so the repository can move. It uses the shared link helper's full
// fallback chain (symlink, then a junction and finally a copy on Windows);
// force replaces a copy that has diverged.
func (c Config) Link(s scope.Scope, name, target string, force bool) error {
	link := filepath.Join(c.SkillsDir(s), name)
	// A harness that reads the canonical .agents/skills directory directly
	// (its skills dir is that dir) needs no link: the skill already lives at
	// the link path, so linking would clobber the canonical tree — including
	// an editable symlink.
	if link == s.SkillDir(name) {
		return nil
	}
	if rel, err := filepath.Rel(filepath.Dir(link), target); err == nil {
		target = rel
	}
	_, err := adapter.LinkDir(link, target, force)
	return err
}

// Unlink removes the link for name: a symlink, a junction, or a copy that
// matches the canonical skill directory (with force, also a diverged copy).
// A foreign directory is left alone.
func (c Config) Unlink(s scope.Scope, name string, force bool) error {
	link := filepath.Join(c.SkillsDir(s), name)
	// Mirror Link: when the link path is the canonical skill dir itself there
	// is nothing of ours to remove, and doing so would delete the skill.
	if link == s.SkillDir(name) {
		return nil
	}
	return adapter.RemoveLinkOrCopy(link, s.SkillDir(name), force)
}

// Detected reports whether the harness appears installed for the user: its
// user directory exists. It is only ever consulted with the machine scope,
// which seeds the one-time harness picker; enablement stays explicit.
func (c Config) Detected(s scope.Scope) bool {
	fi, err := os.Stat(c.userDirFor(s.Root))
	return err == nil && fi.IsDir()
}

// builtins is the harness table. Every harness materializes skills under
// <userDir>/skills at machine scope; codex, cursor, and antigravity read the
// shared .agents/skills convention at project scope, where linking is an
// in-place no-op because skiletto already writes there.
func builtins() []Config {
	return []Config{
		{name: "claude", projectDir: ".claude/skills", userDir: ".claude", userDirEnv: "CLAUDE_CONFIG_DIR"},
		{name: "codex", projectDir: ".agents/skills", userDir: ".codex", userDirEnv: "CODEX_HOME"},
		{name: "vibe", projectDir: ".vibe/skills", userDir: ".vibe", userDirEnv: "VIBE_HOME"},
		{name: "hermes", projectDir: ".hermes/skills", userDir: ".hermes", userDirEnv: "HERMES_HOME"},
		{name: "antigravity", projectDir: ".agents/skills", userDir: ".gemini/antigravity"},
		{name: "cursor", projectDir: ".agents/skills", userDir: ".cursor"},
		{name: "pi", projectDir: ".pi/skills", userDir: ".pi/agent"},
	}
}

func init() {
	for _, c := range builtins() {
		adapter.Register(c)
	}
}
