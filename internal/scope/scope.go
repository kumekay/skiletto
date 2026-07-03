// Package scope resolves where a scope keeps its manifest, lockfile, and
// canonical skills directory. v1 implements the project scope; the machine
// scope slots in as another constructor.
package scope

import "path/filepath"

// Kind identifies a scope flavor.
type Kind string

// KindProject is the project scope: files live in the repo root.
const KindProject Kind = "project"

// Scope holds the resolved paths for one scope.
type Scope struct {
	Kind         Kind
	Root         string
	ManifestPath string
	LockPath     string
	SkillsDir    string
}

// Project returns the project scope rooted at root: manifest and lock in
// the root, skills materialized under .agents/skills.
func Project(root string) Scope {
	return Scope{
		Kind:         KindProject,
		Root:         root,
		ManifestPath: filepath.Join(root, "skiletto.toml"),
		LockPath:     filepath.Join(root, "skiletto.lock"),
		SkillsDir:    filepath.Join(root, ".agents", "skills"),
	}
}

// SkillDir returns the canonical directory for the named skill.
func (s Scope) SkillDir(name string) string {
	return filepath.Join(s.SkillsDir, name)
}
