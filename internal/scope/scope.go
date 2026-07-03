// Package scope resolves where a scope keeps its manifest, lockfile, and
// canonical skills directory. v1 implements the project scope; the machine
// scope slots in as another constructor.
package scope

import "path/filepath"

// Kind identifies a scope flavor.
type Kind string

// Kinds of scope.
const (
	// KindProject scopes files to a repo root.
	KindProject Kind = "project"
	// KindMachine scopes files to the user's home and config dirs.
	KindMachine Kind = "machine"
)

// Scope holds the resolved paths for one scope.
type Scope struct {
	Kind Kind
	// Root is the base the scope's harness link dirs hang off of: the repo
	// root for project scope, the user's home directory for machine scope.
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

// Machine returns the machine scope: manifest and lock under
// configDir/skiletto, skills materialized under home/.agents/skills, and
// harness link dirs hanging off home. home and configDir are injected so
// path resolution stays testable and never assumes the real user home.
func Machine(home, configDir string) Scope {
	return Scope{
		Kind:         KindMachine,
		Root:         home,
		ManifestPath: filepath.Join(configDir, "skiletto", "skiletto.toml"),
		LockPath:     filepath.Join(configDir, "skiletto", "skiletto.lock"),
		SkillsDir:    filepath.Join(home, ".agents", "skills"),
	}
}

// SkillDir returns the canonical directory for the named skill.
func (s Scope) SkillDir(name string) string {
	return filepath.Join(s.SkillsDir, name)
}
