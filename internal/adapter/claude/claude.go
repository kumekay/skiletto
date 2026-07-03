// Package claude adapts skiletto to Claude Code: individual skill
// directories are symlinked into .claude/skills/.
package claude

import (
	"path/filepath"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/scope"
)

func init() {
	adapter.Register(New())
}

// Claude is the Claude Code adapter.
type Claude struct{}

// New returns the Claude Code adapter.
func New() Claude {
	return Claude{}
}

// Name returns "claude".
func (Claude) Name() string {
	return "claude"
}

// SkillsDir is .claude/skills under the scope root.
func (Claude) SkillsDir(s scope.Scope) string {
	return filepath.Join(s.Root, ".claude", "skills")
}

// Link symlinks target as <skills dir>/<name>, preferring a relative
// target so the repository can be moved.
func (c Claude) Link(s scope.Scope, name, target string) error {
	link := filepath.Join(c.SkillsDir(s), name)
	if rel, err := filepath.Rel(filepath.Dir(link), target); err == nil {
		target = rel
	}
	return adapter.Symlink(link, target)
}

// Unlink removes the symlink for name.
func (c Claude) Unlink(s scope.Scope, name string) error {
	return adapter.RemoveLink(filepath.Join(c.SkillsDir(s), name))
}
