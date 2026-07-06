package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
)

// Update re-resolves manifest entries to the current commit for their ref
// (or default branch) and rewrites their lock entries, re-materializing and
// re-linking the installed content. It is the only command that moves
// already-locked versions. An empty name updates every manifest entry;
// otherwise only the named one (an unknown name is an error). Editable
// entries have nothing to re-resolve and are skipped with a note. A drifted
// installed tree is not overwritten without force.
func (e *Engine) Update(name string, force bool) error {
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	names, err := e.updateTargets(m, name)
	if err != nil {
		return err
	}
	enabled, err := e.resolveHarnesses(m, false)
	if err != nil {
		return err
	}
	plan := e.planUpdate(m, lf, names, force)
	return e.apply(m, lf, plan, force, enabled)
}

// updateTargets returns the manifest names to update: all of them (sorted)
// for an empty name, or just the named one, which must exist.
func (e *Engine) updateTargets(m *manifest.Manifest, name string) ([]string, error) {
	if name != "" {
		if _, ok := m.Skills[name]; !ok {
			return nil, unknownSkillError(m, name)
		}
		return []string{name}, nil
	}
	names := make([]string, 0, len(m.Skills))
	for n := range m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// planUpdate emits a re-resolve (fetch) for each pinned target, a note for
// editable ones, and a drift warning for a target whose installed tree was
// modified locally (unless force).
func (e *Engine) planUpdate(m *manifest.Manifest, lf *lockfile.Lockfile, names []string, force bool) Plan {
	var p Plan
	for _, name := range names {
		entry := m.Skills[name]
		if entry.Editable {
			p.Actions = append(p.Actions, Action{
				Kind: ActionNote, Name: name,
				Message: fmt.Sprintf("skill %q is editable; nothing to re-resolve", name),
			})
			continue
		}
		if locked := lf.Find(name); locked != nil && !force {
			if hash, ok := e.installedHash(name); ok && hash != locked.Hash {
				p.Actions = append(p.Actions, Action{
					Kind: ActionWarnDrift, Name: name,
					Message: fmt.Sprintf("skill %q has local modifications; skipping update (run 'skiletto update --force' to overwrite)", name),
				})
				continue
			}
		}
		p.Actions = append(p.Actions, Action{Kind: ActionFetch, Name: name})
	}
	return p
}

// unknownSkillError reports that name is not in the manifest, listing the
// skills that are.
func unknownSkillError(m *manifest.Manifest, name string) error {
	if len(m.Skills) == 0 {
		return fmt.Errorf("skill %q is not in the manifest (no skills are managed)", name)
	}
	known := make([]string, 0, len(m.Skills))
	for n := range m.Skills {
		known = append(known, n)
	}
	sort.Strings(known)
	return fmt.Errorf("skill %q is not in the manifest; known skills: %s", name, strings.Join(known, ", "))
}
