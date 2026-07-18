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
	Scope scope.Scope
	// Machine is the resolved machine scope, set even when Scope is a
	// project: its manifest's harnesses key applies in every scope (union
	// semantics) and it anchors harness detection. nil means no machine
	// configuration exists (tests).
	Machine  *scope.Scope
	Adapters []adapter.Adapter
	// PromptHarnesses, when set, is the interactive one-time harness
	// picker resolveHarnesses uses for a scope with no harnesses key. nil
	// means non-interactive: install to the canonical dir only, with a
	// note.
	PromptHarnesses func([]HarnessOption) ([]string, error)
	NewSource       func(src string) (source.Source, error)
	// NoHooks disables the pre-install hook for this run (--no-hooks).
	NoHooks bool
	// Verbose emits extra diagnostics to Err, such as a line for each
	// pre-install hook run (--verbose).
	Verbose bool
	Out     io.Writer
	Err     io.Writer
}

// New returns a production engine for the scope: system git sources and
// all registered adapters. machine is the resolved machine scope, whose
// manifest supplies machine-wide harnesses in any scope.
func New(sc scope.Scope, machine scope.Scope) (*Engine, error) {
	g, err := gitcli.New()
	if err != nil {
		return nil, err
	}
	return &Engine{
		Scope:    sc,
		Machine:  &machine,
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
	// ActionNote reports an informational message (e.g. an editable entry
	// skipped by update) without counting as a failure.
	ActionNote ActionKind = "note"
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
	enabled, err := e.resolveHarnesses(m, true)
	if err != nil {
		return err
	}
	plan := e.planSync(m, lf, force)
	return e.apply(m, lf, plan, force, enabled, "sync")
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
			// Replacing what the old lock entry installed must not destroy
			// local modifications: drift-check against the old hash first.
			if locked != nil && !locked.Editable && !force {
				if hash, ok := e.installedHash(name); ok && hash != locked.Hash {
					p.Actions = append(p.Actions, Action{
						Kind: ActionWarnDrift, Name: name,
						Message: fmt.Sprintf("skill %q has local modifications; skipping the manifest change (run 'skiletto sync --force' to overwrite)", name),
					})
					continue
				}
			}
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

// apply executes a plan. Every failure is reported once, on e.Err as it
// happens; the returned error only summarizes how many skills had
// problems. event names the invoking command for the pre-install hook.
func (e *Engine) apply(m *manifest.Manifest, lf *lockfile.Lockfile, plan Plan, force bool, enabled []adapter.Adapter, event string) error {
	hook, err := e.preInstallHook(m)
	if err != nil {
		return err
	}
	failures := 0
	lockChanged := false
	for _, act := range plan.Actions {
		var err error
		switch act.Kind {
		case ActionFetch:
			if err = e.applyFetch(act.Name, m.Skills[act.Name], lf, force, enabled, hook, event); err == nil {
				lockChanged = true
			}
		case ActionMaterialize:
			err = e.applyMaterialize(act.Name, *lf.Find(act.Name), force, enabled)
		case ActionLink:
			err = e.applyLink(act.Name, m.Skills[act.Name], force, enabled)
		case ActionPrune:
			if err = e.applyPrune(act.Name, force, enabled); err == nil {
				lf.Remove(act.Name)
				lockChanged = true
			}
		case ActionWarnDrift:
			_, _ = fmt.Fprintf(e.Err, "warning: %s\n", act.Message)
			failures++
		case ActionNote:
			_, _ = fmt.Fprintf(e.Out, "%s\n", act.Message)
		}
		if err != nil {
			failures++
			_, _ = fmt.Fprintf(e.Err, "error: %s: %v\n", act.Name, err)
		}
	}
	if lockChanged {
		if err := lf.Save(e.Scope.LockPath); err != nil {
			failures++
			_, _ = fmt.Fprintf(e.Err, "error: %v\n", err)
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d skill(s) drifted or failed; see warnings above", failures)
	}
	return nil
}

// applyFetch resolves a manifest entry, installs it, and locks it.
func (e *Engine) applyFetch(name string, entry manifest.Entry, lf *lockfile.Lockfile, force bool, enabled []adapter.Adapter, hook, event string) error {
	if entry.Editable {
		if err := e.ensureEditable(name, entry, force, enabled); err != nil {
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
	preInstall := func(staged string) error {
		return e.runPreInstall(hook, name, entry.Source, commit, event, staged)
	}
	hash, _, err := e.install(name, src, commit, entry.Path, force, enabled, preInstall)
	if err != nil {
		// An ambiguous source reached through the manifest is fixed by
		// setting path on the entry, not by re-running add.
		var multi *MultipleSkillsError
		if errors.As(err, &multi) {
			multi.ManifestName = name
		}
		return err
	}
	if err := e.linkAll(name, force, enabled); err != nil {
		return err
	}
	lf.Upsert(lockfile.Skill{
		Name: name, Source: entry.Source, Path: entry.Path, Ref: entry.Ref,
		Commit: commit, Hash: hash,
	})
	return nil
}

// applyMaterialize reinstalls the exact locked commit.
func (e *Engine) applyMaterialize(name string, locked lockfile.Skill, force bool, enabled []adapter.Adapter) error {
	src, err := e.NewSource(locked.Source)
	if err != nil {
		return err
	}
	hash, _, err := e.install(name, src, locked.Commit, locked.Path, force, enabled, nil)
	if err != nil {
		return err
	}
	if hash != locked.Hash {
		return fmt.Errorf("content at commit %s does not match the locked hash", locked.Commit)
	}
	return e.linkAll(name, force, enabled)
}

// applyLink ensures the canonical location (editable) and harness links.
func (e *Engine) applyLink(name string, entry manifest.Entry, force bool, enabled []adapter.Adapter) error {
	if entry.Editable {
		return e.ensureEditable(name, entry, force, enabled)
	}
	return e.linkAll(name, force, enabled)
}

// applyPrune unlinks a skill from every adapter and deletes its
// materialized copy.
func (e *Engine) applyPrune(name string, force bool, enabled []adapter.Adapter) error {
	if err := e.unlinkAll(name, force, enabled); err != nil {
		return err
	}
	return removeInstalled(e.Scope.SkillDir(name))
}

// ensureEditable points the canonical location at the working tree and
// links it into every adapter. A materialized copy left by a previous
// pinned install is only replaced with force.
func (e *Engine) ensureEditable(name string, entry manifest.Entry, force bool, enabled []adapter.Adapter) error {
	worktree := filepath.Join(source.ExpandHome(entry.Source), filepath.FromSlash(entry.Path))
	if fi, err := os.Stat(worktree); err != nil || !fi.IsDir() {
		return fmt.Errorf("editable source %s is not a directory", worktree)
	}
	canonical := e.Scope.SkillDir(name)
	if link, err := adapter.IsLink(canonical); err == nil && !link {
		if !force {
			return fmt.Errorf("%s contains a materialized copy; run 'skiletto sync --force' to replace it with the editable link", canonical)
		}
		if err := removeInstalled(canonical); err != nil {
			return err
		}
	}
	if err := adapter.Symlink(canonical, worktree); err != nil {
		return err
	}
	return e.linkAll(name, force, enabled)
}

// install fetches subpath at commit into a staging area, requires it to
// contain exactly one skill, and promotes that skill directory to the
// canonical location. It returns the content hash and the skill's
// effective subpath within the source. hook, when non-nil, runs against
// the staged content before it is hashed and before anything installed is
// touched; its error aborts the install with the previous content still in
// place.
//
// Adapter links are removed after staging succeeds and before promotion,
// while the canonical tree still has its pre-update content: a copy-linked
// install proves itself ours by matching that tree, so unlinking any later
// would refuse its own pristine copies. A diverged copy makes the unlink
// fail here (unless force), before the canonical tree or the lock move —
// nothing is left half-updated.
func (e *Engine) install(name string, src source.Source, commit, subpath string, force bool, enabled []adapter.Adapter, hook func(staged string) error) (hash, effPath string, err error) {
	staged, effPath, cleanup, err := e.stage(src, commit, subpath)
	if err != nil {
		return "", "", err
	}
	defer cleanup()
	if hook != nil {
		if err := hook(staged); err != nil {
			return "", "", err
		}
	}
	// Hash after the hook: a hook that rewrites staged content must not
	// lock a hash the promoted tree no longer matches.
	hash, err = skill.Hash(staged)
	if err != nil {
		return "", "", err
	}
	if err := e.unlinkAll(name, force, enabled); err != nil {
		return "", "", err
	}
	if err := e.promote(staged, name); err != nil {
		return "", "", err
	}
	return hash, effPath, nil
}

// promote moves a staged skill tree into the canonical location for name,
// replacing whatever was installed there.
func (e *Engine) promote(staged, name string) error {
	canonical := e.Scope.SkillDir(name)
	if err := removeInstalled(canonical); err != nil {
		return err
	}
	if err := os.Rename(staged, canonical); err != nil {
		return err
	}
	// Staging directories are created 0700 by MkdirTemp.
	return os.Chmod(canonical, 0o755)
}

// stage fetches subpath at commit into a temporary directory under the
// skills dir and locates the single skill within it. An explicit "."
// subpath pins the skill to the source root itself: SKILL.md must exist
// there, and nested skills elsewhere in the source are not discovered.
func (e *Engine) stage(src source.Source, commit, subpath string) (staged, effPath string, cleanup func(), err error) {
	if err := os.MkdirAll(e.Scope.SkillsDir, 0o755); err != nil {
		return "", "", nil, err
	}
	staging, err := os.MkdirTemp(e.Scope.SkillsDir, ".staging-")
	if err != nil {
		return "", "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(staging) }
	fetchPath := subpath
	if subpath == "." {
		fetchPath = ""
	}
	if err := src.Fetch(commit, fetchPath, staging); err != nil {
		cleanup()
		return "", "", nil, err
	}
	if subpath == "." {
		if _, err := os.Stat(filepath.Join(staging, "SKILL.md")); err != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("path is \".\" but the source root has no SKILL.md")
		}
		return staging, ".", cleanup, nil
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
			skills[i] = skillSubpath(subpath, d)
		}
		return "", "", nil, &MultipleSkillsError{Skills: skills}
	}
}

// linkAll reconciles the skill's harness links: enabled adapters get a
// link to the canonical directory, every other registered adapter has its
// link removed (so disabling a harness converges on the next sync). force
// lets a link replace a copy-linked install that has diverged.
func (e *Engine) linkAll(name string, force bool, enabled []adapter.Adapter) error {
	target := e.Scope.SkillDir(name)
	on := enabledSet(enabled)
	for _, a := range e.Adapters {
		if on[a.Name()] {
			if err := a.Link(e.Scope, name, target, force); err != nil {
				return fmt.Errorf("adapter %s: %w", a.Name(), err)
			}
			continue
		}
		if err := unlinkTolerant(a, e.Scope, name, force); err != nil {
			return fmt.Errorf("adapter %s: %w", a.Name(), err)
		}
	}
	return nil
}

// unlinkAll removes the skill's link from every registered adapter. For
// adapters that are not enabled, a location skiletto cannot prove it owns
// is silently left alone — the user never asked for that harness, so a
// foreign directory there is none of our business. For enabled adapters
// the refusal surfaces (with force it removes a diverged copy).
func (e *Engine) unlinkAll(name string, force bool, enabled []adapter.Adapter) error {
	on := enabledSet(enabled)
	for _, a := range e.Adapters {
		if on[a.Name()] {
			if err := a.Unlink(e.Scope, name, force); err != nil {
				return fmt.Errorf("adapter %s: %w", a.Name(), err)
			}
			continue
		}
		if err := unlinkTolerant(a, e.Scope, name, force); err != nil {
			return fmt.Errorf("adapter %s: %w", a.Name(), err)
		}
	}
	return nil
}

// unlinkTolerant unlinks from a harness the user has not enabled,
// swallowing the not-ours refusal: a foreign directory in a disabled
// harness's dir must not block skill operations.
func unlinkTolerant(a adapter.Adapter, sc scope.Scope, name string, force bool) error {
	err := a.Unlink(sc, name, force)
	var notOurs *adapter.NotOurLinkError
	if errors.As(err, &notOurs) {
		return nil
	}
	return err
}

// enabledSet indexes enabled adapters by name.
func enabledSet(enabled []adapter.Adapter) map[string]bool {
	on := make(map[string]bool, len(enabled))
	for _, a := range enabled {
		on[a.Name()] = true
	}
	return on
}

// removeInstalled deletes whatever occupies a canonical skill location: a
// link (a symlink for editable installs, or a directory junction on Windows)
// is removed without following it into its target; a materialized copy is
// removed recursively.
func removeInstalled(dir string) error {
	if _, err := os.Lstat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	link, err := adapter.IsLink(dir)
	if err != nil {
		return err
	}
	if link {
		return os.Remove(dir)
	}
	return os.RemoveAll(dir)
}

// skillSubpath is joinSubpath for a skill listed in an ambiguity: an empty
// join (the source root) becomes "." so the skill stays addressable as
// <src>//. rather than an unusable bare <src>//.
func skillSubpath(base, rel string) string {
	if sub := joinSubpath(base, rel); sub != "" {
		return sub
	}
	return "."
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
