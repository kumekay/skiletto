// Package engine orchestrates skiletto: it diffs manifest, lockfile, and
// disk state into a Plan of actions, then applies them through sources and
// adapters. Command handlers stay thin; all behavior lives here.
package engine

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/gitcli"
	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/skill"
	"github.com/kumekay/skiletto/internal/source"
)

// Engine wires one scope to sources and adapters.
type Engine struct {
	Scope     scope.Scope
	Adapters  []adapter.Adapter
	NewSource func(src string) (source.Source, error)
	Out       io.Writer
	Err       io.Writer
}

// New returns a production engine for the scope: system git sources and
// all registered adapters.
func New(sc scope.Scope) (*Engine, error) {
	g, err := gitcli.New()
	if err != nil {
		return nil, err
	}
	return &Engine{
		Scope:    sc,
		Adapters: adapter.All(),
		NewSource: func(src string) (source.Source, error) {
			return source.New(g, src), nil
		},
		Out: os.Stdout,
		Err: os.Stderr,
	}, nil
}

// ActionKind classifies plan actions.
type ActionKind string

// Plan action kinds.
const (
	// ActionFetch resolves and installs a manifest entry that has no
	// (matching) lock entry, then locks it.
	ActionFetch ActionKind = "fetch"
	// ActionMaterialize reinstalls the exact locked commit (missing from
	// disk, or a forced drift restore). It never re-resolves refs.
	ActionMaterialize ActionKind = "materialize"
	// ActionLink ensures the canonical location (for editable skills) and
	// the harness links are in place.
	ActionLink ActionKind = "link"
	// ActionPrune removes a skill that is locked but no longer in the
	// manifest: lock entry, harness links, and materialized copy.
	ActionPrune ActionKind = "prune"
	// ActionWarnDrift reports an installed tree whose content hash does
	// not match the lock; the files are left alone.
	ActionWarnDrift ActionKind = "warn-drift"
)

// Action is one step of a plan.
type Action struct {
	Kind    ActionKind
	Name    string
	Message string // set for warn-drift
}

// Plan is an ordered list of actions.
type Plan struct {
	Actions []Action
}

// PlanSync computes the sync plan for the current manifest, lockfile, and
// disk state.
func (e *Engine) PlanSync(force bool) (Plan, error) {
	m, lf, err := e.load()
	if err != nil {
		return Plan{}, err
	}
	return e.planSync(m, lf, force), nil
}

// Sync makes the installed state match the lock: it installs what the
// lock pins, resolves and locks manifest entries missing from the lock,
// prunes lock entries gone from the manifest, and warns about drifted
// skills (restoring them only with force). It returns a non-nil error if
// any skill drifted or failed.
func (e *Engine) Sync(force bool) error {
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	plan := e.planSync(m, lf, force)
	return e.apply(m, lf, plan)
}

func (e *Engine) load() (*manifest.Manifest, *lockfile.Lockfile, error) {
	m, err := manifest.Load(e.Scope.ManifestPath)
	if err != nil {
		return nil, nil, err
	}
	lf, err := lockfile.Load(e.Scope.LockPath)
	if err != nil {
		return nil, nil, err
	}
	return m, lf, nil
}

func (e *Engine) planSync(m *manifest.Manifest, lf *lockfile.Lockfile, force bool) Plan {
	var p Plan
	names := make([]string, 0, len(m.Skills))
	for name := range m.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := m.Skills[name]
		locked := lf.Find(name)
		switch {
		case locked == nil || lockMismatch(entry, *locked):
			p.Actions = append(p.Actions, Action{Kind: ActionFetch, Name: name})
		case entry.Editable:
			p.Actions = append(p.Actions, Action{Kind: ActionLink, Name: name})
		default:
			hash, ok := e.installedHash(name)
			switch {
			case !ok:
				p.Actions = append(p.Actions, Action{Kind: ActionMaterialize, Name: name})
			case hash == locked.Hash:
				p.Actions = append(p.Actions, Action{Kind: ActionLink, Name: name})
			case force:
				p.Actions = append(p.Actions, Action{Kind: ActionMaterialize, Name: name})
			default:
				p.Actions = append(p.Actions, Action{
					Kind: ActionWarnDrift, Name: name,
					Message: fmt.Sprintf("skill %q has local modifications; skipping (run 'skiletto sync --force' to restore the locked version)", name),
				})
			}
		}
	}

	var gone []string
	for _, locked := range lf.Skills {
		if _, ok := m.Skills[locked.Name]; !ok {
			gone = append(gone, locked.Name)
		}
	}
	sort.Strings(gone)
	for _, name := range gone {
		locked := lf.Find(name)
		if !locked.Editable && !force {
			if hash, ok := e.installedHash(name); ok && hash != locked.Hash {
				p.Actions = append(p.Actions, Action{
					Kind: ActionWarnDrift, Name: name,
					Message: fmt.Sprintf("skill %q was removed from the manifest but has local modifications; refusing to delete it (run 'skiletto sync --force' to remove)", name),
				})
				continue
			}
		}
		p.Actions = append(p.Actions, Action{Kind: ActionPrune, Name: name})
	}
	return p
}

// lockMismatch reports whether the manifest entry no longer matches what
// the lock entry was resolved from, meaning it must be re-locked.
func lockMismatch(entry manifest.Entry, locked lockfile.Skill) bool {
	return entry.Source != locked.Source ||
		entry.Path != locked.Path ||
		entry.Ref != locked.Ref ||
		entry.Editable != locked.Editable
}

// installedHash hashes the canonical directory for name. ok is false when
// nothing usable is installed (missing, or not a real directory).
func (e *Engine) installedHash(name string) (string, bool) {
	dir := e.Scope.SkillDir(name)
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	hash, err := skill.Hash(dir)
	if err != nil {
		return "", false
	}
	return hash, true
}

func (e *Engine) apply(m *manifest.Manifest, lf *lockfile.Lockfile, plan Plan) error {
	var errs []error
	lockChanged := false
	for _, act := range plan.Actions {
		var err error
		switch act.Kind {
		case ActionFetch:
			if err = e.applyFetch(act.Name, m.Skills[act.Name], lf); err == nil {
				lockChanged = true
			}
		case ActionMaterialize:
			err = e.applyMaterialize(act.Name, *lf.Find(act.Name))
		case ActionLink:
			err = e.applyLink(act.Name, m.Skills[act.Name])
		case ActionPrune:
			if err = e.applyPrune(act.Name); err == nil {
				lf.Remove(act.Name)
				lockChanged = true
			}
		case ActionWarnDrift:
			_, _ = fmt.Fprintf(e.Err, "warning: %s\n", act.Message)
			err = fmt.Errorf("%s: local modifications", act.Name)
		}
		if err != nil {
			errs = append(errs, err)
			if act.Kind != ActionWarnDrift {
				_, _ = fmt.Fprintf(e.Err, "error: %s: %v\n", act.Name, err)
			}
		}
	}
	if lockChanged {
		if err := lf.Save(e.Scope.LockPath); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// applyFetch resolves a manifest entry, installs it, and locks it.
func (e *Engine) applyFetch(name string, entry manifest.Entry, lf *lockfile.Lockfile) error {
	if entry.Editable {
		if err := e.ensureEditable(name, entry); err != nil {
			return err
		}
		lf.Upsert(lockfile.Skill{
			Name: name, Source: entry.Source, Path: entry.Path, Editable: true,
		})
		return nil
	}
	src, err := e.NewSource(entry.Source)
	if err != nil {
		return err
	}
	commit, err := src.Resolve(entry.Ref)
	if err != nil {
		return err
	}
	hash, _, err := e.install(name, src, commit, entry.Path)
	if err != nil {
		return err
	}
	if err := e.linkAll(name); err != nil {
		return err
	}
	lf.Upsert(lockfile.Skill{
		Name: name, Source: entry.Source, Path: entry.Path, Ref: entry.Ref,
		Commit: commit, Hash: hash,
	})
	return nil
}

// applyMaterialize reinstalls the exact locked commit.
func (e *Engine) applyMaterialize(name string, locked lockfile.Skill) error {
	src, err := e.NewSource(locked.Source)
	if err != nil {
		return err
	}
	hash, _, err := e.install(name, src, locked.Commit, locked.Path)
	if err != nil {
		return err
	}
	if hash != locked.Hash {
		return fmt.Errorf("content at commit %s does not match the locked hash", locked.Commit)
	}
	return e.linkAll(name)
}

// applyLink ensures the canonical location (editable) and harness links.
func (e *Engine) applyLink(name string, entry manifest.Entry) error {
	if entry.Editable {
		return e.ensureEditable(name, entry)
	}
	return e.linkAll(name)
}

// applyPrune unlinks a skill from every adapter and deletes its
// materialized copy.
func (e *Engine) applyPrune(name string) error {
	for _, a := range e.Adapters {
		if err := a.Unlink(e.Scope, name); err != nil {
			return err
		}
	}
	return removeInstalled(e.Scope.SkillDir(name))
}

// ensureEditable points the canonical location at the working tree and
// links it into every adapter.
func (e *Engine) ensureEditable(name string, entry manifest.Entry) error {
	worktree := filepath.Join(source.ExpandHome(entry.Source), filepath.FromSlash(entry.Path))
	if fi, err := os.Stat(worktree); err != nil || !fi.IsDir() {
		return fmt.Errorf("editable source %s is not a directory", worktree)
	}
	if err := adapter.Symlink(e.Scope.SkillDir(name), worktree); err != nil {
		return err
	}
	return e.linkAll(name)
}

// install fetches subpath at commit into a staging area, requires it to
// contain exactly one skill, and promotes that skill directory to the
// canonical location. It returns the content hash and the skill's
// effective subpath within the source.
func (e *Engine) install(name string, src source.Source, commit, subpath string) (hash, effPath string, err error) {
	staged, effPath, cleanup, err := e.stage(src, commit, subpath)
	if err != nil {
		return "", "", err
	}
	defer cleanup()
	hash, err = skill.Hash(staged)
	if err != nil {
		return "", "", err
	}
	canonical := e.Scope.SkillDir(name)
	if err := removeInstalled(canonical); err != nil {
		return "", "", err
	}
	if err := os.Rename(staged, canonical); err != nil {
		return "", "", err
	}
	return hash, effPath, nil
}

// stage fetches subpath at commit into a temporary directory under the
// skills dir and locates the single skill within it.
func (e *Engine) stage(src source.Source, commit, subpath string) (staged, effPath string, cleanup func(), err error) {
	if err := os.MkdirAll(e.Scope.SkillsDir, 0o755); err != nil {
		return "", "", nil, err
	}
	staging, err := os.MkdirTemp(e.Scope.SkillsDir, ".staging-")
	if err != nil {
		return "", "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(staging) }
	if err := src.Fetch(commit, subpath, staging); err != nil {
		cleanup()
		return "", "", nil, err
	}
	dirs, err := skill.Discover(staging)
	if err != nil {
		cleanup()
		return "", "", nil, err
	}
	switch len(dirs) {
	case 0:
		cleanup()
		return "", "", nil, fmt.Errorf("no SKILL.md found under %q", path.Join("/", subpath))
	case 1:
		return filepath.Join(staging, filepath.FromSlash(dirs[0])), joinSubpath(subpath, dirs[0]), cleanup, nil
	default:
		cleanup()
		skills := make([]string, len(dirs))
		for i, d := range dirs {
			skills[i] = joinSubpath(subpath, d)
		}
		return "", "", nil, &MultipleSkillsError{Skills: skills}
	}
}

// linkAll links the canonical skill directory into every adapter.
func (e *Engine) linkAll(name string) error {
	target := e.Scope.SkillDir(name)
	for _, a := range e.Adapters {
		if err := a.Link(e.Scope, name, target); err != nil {
			return fmt.Errorf("adapter %s: %w", a.Name(), err)
		}
	}
	return nil
}

// removeInstalled deletes whatever occupies a canonical skill location: a
// symlink (editable installs) is removed without following it.
func removeInstalled(dir string) error {
	fi, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return os.Remove(dir)
	}
	return os.RemoveAll(dir)
}

// joinSubpath joins a source subpath with a slash-relative discovered
// directory ("." meaning the subpath itself).
func joinSubpath(base, rel string) string {
	if rel == "." || rel == "" {
		return base
	}
	if base == "" {
		return rel
	}
	return base + "/" + rel
}
