package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/skill"
	"github.com/kumekay/skiletto/internal/source"
)

// MultipleSkillsError reports an ambiguous source: it contains several
// skills and no //path was given to pick one.
type MultipleSkillsError struct {
	Source string // CLI source to embed in suggestions
	Ref    string
	Skills []string // skill subpaths within the source
}

func (e *MultipleSkillsError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "source contains %d skills; pick one with //path:", len(e.Skills))
	for _, s := range e.Skills {
		fmt.Fprintf(&b, "\n  skiletto add %s//%s", e.Source, s)
		if e.Ref != "" {
			fmt.Fprintf(&b, "@%s", e.Ref)
		}
	}
	return b.String()
}

// Add resolves a parsed source spec, installs the skill it names, links
// it into every adapter, and records it in the manifest and lockfile.
func (e *Engine) Add(spec manifest.SourceSpec, editable bool) error {
	if editable && !spec.IsPath {
		return fmt.Errorf("--editable requires a local path source, got %q", spec.Source)
	}
	if editable && spec.Ref != "" {
		return fmt.Errorf("--editable installs track the working tree; a ref (@%s) cannot be used", spec.Ref)
	}
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	if spec.IsPath && e.Scope.Kind == scope.KindProject {
		_, _ = fmt.Fprintf(e.Err, "warning: %q is a machine-specific path; 'skiletto sync' will fail for anyone without it\n", spec.Source)
	}

	if editable {
		return e.addEditable(spec, m, lf)
	}
	return e.addPinned(spec, m, lf)
}

// addEditable symlinks the canonical location straight at the working
// tree; the lock entry carries no commit and no hash.
func (e *Engine) addEditable(spec manifest.SourceSpec, m *manifest.Manifest, lf *lockfile.Lockfile) error {
	root := source.ExpandHome(spec.Source)
	searchDir := filepath.Join(root, filepath.FromSlash(spec.Path))
	if fi, err := os.Stat(searchDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", searchDir)
	}
	dirs, err := skill.Discover(searchDir)
	if err != nil {
		return err
	}
	effPath, err := singleSkill(spec, dirs)
	if err != nil {
		return err
	}
	name := skill.DefaultName(spec.Source, effPath)
	if _, exists := m.Skills[name]; exists {
		return fmt.Errorf("skill %q is already in the manifest; edit skiletto.toml to rename or remove it first", name)
	}

	entry := manifest.Entry{Source: spec.Source, Path: effPath, Editable: true}
	if err := e.ensureEditable(name, entry); err != nil {
		return err
	}
	m.Skills[name] = entry
	lf.Upsert(lockfile.Skill{Name: name, Source: spec.Source, Path: effPath, Editable: true})
	return e.saveBoth(m, lf, name)
}

// addPinned resolves the spec's ref to a commit (via ls-remote for URLs,
// locally for path sources, which must be git repositories), installs the
// pinned content, and locks commit and hash.
func (e *Engine) addPinned(spec manifest.SourceSpec, m *manifest.Manifest, lf *lockfile.Lockfile) error {
	src, err := e.NewSource(spec.Source)
	if err != nil {
		return err
	}
	commit, err := src.Resolve(spec.Ref)
	if err != nil {
		return err
	}
	staged, effPath, cleanup, err := e.stage(src, commit, spec.Path)
	if err != nil {
		var multi *MultipleSkillsError
		if errors.As(err, &multi) {
			multi.Source = spec.Source
			multi.Ref = spec.Ref
		}
		return err
	}
	defer cleanup()

	name := skill.DefaultName(spec.Source, effPath)
	if _, exists := m.Skills[name]; exists {
		return fmt.Errorf("skill %q is already in the manifest; edit skiletto.toml to rename or remove it first", name)
	}
	hash, err := skill.Hash(staged)
	if err != nil {
		return err
	}
	if err := e.promote(staged, name); err != nil {
		return err
	}
	if err := e.linkAll(name); err != nil {
		return err
	}
	m.Skills[name] = manifest.Entry{Source: spec.Source, Path: effPath, Ref: spec.Ref}
	lf.Upsert(lockfile.Skill{
		Name: name, Source: spec.Source, Path: effPath, Ref: spec.Ref,
		Commit: commit, Hash: hash,
	})
	return e.saveBoth(m, lf, name)
}

func (e *Engine) saveBoth(m *manifest.Manifest, lf *lockfile.Lockfile, name string) error {
	if err := m.Save(e.Scope.ManifestPath); err != nil {
		return err
	}
	if err := lf.Save(e.Scope.LockPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "added %s\n", name)
	return nil
}

// singleSkill maps discovery results to the one skill the spec means, or
// returns an actionable error.
func singleSkill(spec manifest.SourceSpec, dirs []string) (string, error) {
	switch len(dirs) {
	case 0:
		return "", fmt.Errorf("no SKILL.md found in %s", spec.Source)
	case 1:
		return joinSubpath(spec.Path, dirs[0]), nil
	default:
		skills := make([]string, len(dirs))
		for i, d := range dirs {
			skills[i] = joinSubpath(spec.Path, d)
		}
		return "", &MultipleSkillsError{Source: spec.Source, Ref: spec.Ref, Skills: skills}
	}
}
