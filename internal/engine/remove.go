package engine

import "fmt"

// Remove drops a skill from the manifest and lock, unlinks it from every
// adapter, and deletes its materialized copy. For an editable skill only
// the canonical symlink is removed; the linked working tree is never
// touched. A drifted skill (local modifications) is refused without force,
// since removal would destroy those edits. An unknown name is an error.
func (e *Engine) Remove(name string, force bool) error {
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	enabled, err := e.resolveHarnesses(m, false)
	if err != nil {
		return err
	}
	entry, ok := m.Skills[name]
	if !ok {
		return unknownSkillError(m, name)
	}
	if !entry.Editable && !force {
		if locked := lf.Find(name); locked != nil {
			if hash, ok := e.installedHash(name); ok && hash != locked.Hash {
				return fmt.Errorf("skill %q has local modifications; refusing to remove it (run 'skiletto remove --force %s' to delete it)", name, name)
			}
		}
	}
	if err := e.applyPrune(name, force, enabled); err != nil {
		return err
	}
	delete(m.Skills, name)
	lf.Remove(name)
	if err := m.Save(e.Scope.ManifestPath); err != nil {
		return err
	}
	if err := lf.Save(e.Scope.LockPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "removed %s\n", name)
	return nil
}
