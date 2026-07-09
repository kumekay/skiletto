package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/skill"
	"github.com/kumekay/skiletto/internal/source"
)

// MultipleSkillsError reports an ambiguous source: it contains several
// skills and nothing picks one. From add it suggests //path invocations;
// when the ambiguity comes from a manifest entry (ManifestName set) it
// tells the user to set path on that entry instead; when it comes from a
// skills-lock.json entry (FromImport set) it points at skillPath there.
type MultipleSkillsError struct {
	Source       string // CLI source to embed in add suggestions
	Ref          string
	ManifestName string   // manifest entry the source was reached through
	FromImport   bool     // the source was reached through a skills-lock.json entry
	Skills       []string // skill subpaths within the source
}

func (e *MultipleSkillsError) Error() string {
	var b strings.Builder
	if e.FromImport {
		fmt.Fprintf(&b, "source contains %d skills; set skillPath on this entry in skills-lock.json, or add one directly:", len(e.Skills))
		for _, s := range e.Skills {
			fmt.Fprintf(&b, "\n  skiletto add %s//%s", e.Source, s)
		}
		return b.String()
	}
	if e.ManifestName != "" {
		fmt.Fprintf(&b, "source contains %d skills; set path on the %q entry in skiletto.toml to pick one:", len(e.Skills), e.ManifestName)
		for _, s := range e.Skills {
			fmt.Fprintf(&b, "\n  path = %q", s)
		}
		return b.String()
	}
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
// it into every adapter, and records it in the manifest and lockfile. When
// the source contains several skills and the spec picks none, it returns a
// *MultipleSkillsError so the caller can run a picker and re-drive the add
// with AddSelected.
func (e *Engine) Add(spec manifest.SourceSpec, editable bool) error {
	if err := validateAdd(spec, editable); err != nil {
		return err
	}
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	enabled, err := e.resolveHarnesses(m, true)
	if err != nil {
		return err
	}
	e.warnPathSource(spec)
	if err := e.addOne(spec, editable, m, lf, enabled); err != nil {
		return err
	}
	return e.saveBoth(m, lf)
}

// AddSelected installs a known set of skill subpaths from one source (the
// choices a picker returned), recording and saving them together. Each
// subpath is a skill directory relative to the source root.
func (e *Engine) AddSelected(spec manifest.SourceSpec, subpaths []string, editable bool) error {
	if err := validateAdd(spec, editable); err != nil {
		return err
	}
	return e.addSubpaths(spec, subpaths, editable)
}

// AddAll discovers every skill in the source and installs them all, without
// prompting. It is the engine side of the --all flag.
func (e *Engine) AddAll(spec manifest.SourceSpec, editable bool) error {
	if err := validateAdd(spec, editable); err != nil {
		return err
	}
	e.warnPathSource(spec)
	subpaths, err := e.discover(spec, editable)
	if err != nil {
		return err
	}
	return e.addSubpaths(spec, subpaths, editable)
}

// addSubpaths installs each subpath as its own skill, then writes the
// manifest and lock once. Per-skill failures are reported as they happen
// and summarized; skills that succeed are still saved.
func (e *Engine) addSubpaths(spec manifest.SourceSpec, subpaths []string, editable bool) error {
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	enabled, err := e.resolveHarnesses(m, true)
	if err != nil {
		return err
	}
	added, failures := 0, 0
	for _, sub := range subpaths {
		s := spec
		s.Path = sub
		if err := e.addOne(s, editable, m, lf, enabled); err != nil {
			failures++
			_, _ = fmt.Fprintf(e.Err, "error: %s: %v\n", sub, err)
			continue
		}
		added++
	}
	if added > 0 {
		if err := e.saveBoth(m, lf); err != nil {
			return err
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d skill(s) failed to add; see errors above", failures)
	}
	return nil
}

// addOne dispatches a single-skill install to the editable or pinned path.
func (e *Engine) addOne(spec manifest.SourceSpec, editable bool, m *manifest.Manifest, lf *lockfile.Lockfile, enabled []adapter.Adapter) error {
	if editable {
		return e.addEditable(spec, m, lf, enabled)
	}
	return e.addPinned(spec, m, lf, enabled)
}

// discover lists the skill subpaths of a source without installing them,
// used by AddAll.
func (e *Engine) discover(spec manifest.SourceSpec, editable bool) ([]string, error) {
	if editable {
		return e.discoverEditable(spec)
	}
	return e.discoverPinned(spec)
}

// discoverPinned resolves the ref and stages the source to enumerate its
// skills. A single skill is returned as a one-element list; several come
// back from the MultipleSkillsError the stage raises.
func (e *Engine) discoverPinned(spec manifest.SourceSpec) ([]string, error) {
	src, err := e.NewSource(spec.Source)
	if err != nil {
		return nil, err
	}
	commit, err := src.Resolve(spec.Ref)
	if err != nil {
		return nil, err
	}
	_, effPath, cleanup, err := e.stage(src, commit, spec.Path)
	if err != nil {
		var multi *MultipleSkillsError
		if errors.As(err, &multi) {
			return multi.Skills, nil
		}
		return nil, err
	}
	cleanup()
	return []string{effPath}, nil
}

// discoverEditable enumerates the skills under a local path source.
func (e *Engine) discoverEditable(spec manifest.SourceSpec) ([]string, error) {
	searchDir := filepath.Join(source.ExpandHome(spec.Source), filepath.FromSlash(spec.Path))
	if fi, err := os.Stat(searchDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", searchDir)
	}
	dirs, err := skill.Discover(searchDir)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no SKILL.md found in %s", spec.Source)
	}
	subs := make([]string, len(dirs))
	for i, d := range dirs {
		subs[i] = joinSubpath(spec.Path, d)
	}
	return subs, nil
}

// validateAdd rejects flag combinations that cannot install anything.
func validateAdd(spec manifest.SourceSpec, editable bool) error {
	if editable && !spec.IsPath {
		return fmt.Errorf("--editable requires a local path source, got %q", spec.Source)
	}
	if editable && spec.Ref != "" {
		return fmt.Errorf("--editable installs track the working tree; a ref (@%s) cannot be used", spec.Ref)
	}
	return nil
}

// warnPathSource warns that a machine-specific path source will break a
// teammate's sync, but only in the project scope.
func (e *Engine) warnPathSource(spec manifest.SourceSpec) {
	if spec.IsPath && e.Scope.Kind == scope.KindProject {
		_, _ = fmt.Fprintf(e.Err, "warning: %q is a machine-specific path; 'skiletto sync' will fail for anyone without it\n", spec.Source)
	}
}

// addEditable symlinks the canonical location straight at the working
// tree; the lock entry carries no commit and no hash.
func (e *Engine) addEditable(spec manifest.SourceSpec, m *manifest.Manifest, lf *lockfile.Lockfile, enabled []adapter.Adapter) error {
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
	if err := e.ensureEditable(name, entry, false, enabled); err != nil {
		e.cleanupFailedAdd(name, true)
		return err
	}
	m.Skills[name] = entry
	lf.Upsert(lockfile.Skill{Name: name, Source: spec.Source, Path: effPath, Editable: true})
	_, _ = fmt.Fprintf(e.Out, "added %s\n", name)
	return nil
}

// addPinned resolves the spec's ref to a commit (via ls-remote for URLs,
// locally for path sources, which must be git repositories), installs the
// pinned content, and locks commit and hash.
func (e *Engine) addPinned(spec manifest.SourceSpec, m *manifest.Manifest, lf *lockfile.Lockfile, enabled []adapter.Adapter) error {
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
	if err := e.runPreInstall(e.preInstallHook(m), name, spec.Source, commit, "add", staged); err != nil {
		return err
	}
	if err := e.promote(staged, name); err != nil {
		return err
	}
	if err := e.linkAll(name, false, enabled); err != nil {
		e.cleanupFailedAdd(name, false)
		return err
	}
	m.Skills[name] = manifest.Entry{Source: spec.Source, Path: effPath, Ref: spec.Ref}
	lf.Upsert(lockfile.Skill{
		Name: name, Source: spec.Source, Path: effPath, Ref: spec.Ref,
		Commit: commit, Hash: hash,
	})
	_, _ = fmt.Fprintf(e.Out, "added %s\n", name)
	return nil
}

// cleanupFailedAdd removes what a failed add left behind so no orphan
// materialized copy or link survives without a manifest entry. With
// symlinkOnly (the editable path) the canonical location is removed only
// when it is a symlink, never a pre-existing real directory.
func (e *Engine) cleanupFailedAdd(name string, symlinkOnly bool) {
	for _, a := range e.Adapters {
		_ = a.Unlink(e.Scope, name, false)
	}
	canonical := e.Scope.SkillDir(name)
	if symlinkOnly {
		if link, err := adapter.IsLink(canonical); err != nil || !link {
			return
		}
	}
	_ = removeInstalled(canonical)
}

func (e *Engine) saveBoth(m *manifest.Manifest, lf *lockfile.Lockfile) error {
	if err := m.Save(e.Scope.ManifestPath); err != nil {
		return err
	}
	return lf.Save(e.Scope.LockPath)
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
			skills[i] = skillSubpath(spec.Path, d)
		}
		return "", &MultipleSkillsError{Source: spec.Source, Ref: spec.Ref, Skills: skills}
	}
}
