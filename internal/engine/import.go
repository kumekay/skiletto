package engine

import (
	"errors"
	"fmt"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/vercelimport"
)

// Import bootstraps the scope's manifest and lockfile from a Vercel
// skills-lock.json. Each entry is mapped to a canonical git source, its
// default-branch HEAD is resolved to a commit and installed exactly like
// sync, and both files are written pinned. Entries already present in the
// manifest are skipped untouched; an entry whose canonical location holds a
// drifted or unmanaged tree is refused unless force overwrites it; entries
// that cannot be mapped or resolved are reported and cause a non-zero exit,
// but never abort the entries that do resolve.
func (e *Engine) Import(lockPath string, force bool) error {
	lk, err := vercelimport.Read(lockPath)
	if err != nil {
		return err
	}
	mapped, failures := lk.Map()

	m, lf, err := e.load()
	if err != nil {
		return err
	}

	imported := 0
	installFailures := 0
	for _, mp := range mapped {
		if _, exists := m.Skills[mp.Name]; exists {
			_, _ = fmt.Fprintf(e.Out, "skipping %s: already in the manifest\n", mp.Name)
			continue
		}
		if msg := e.importOverwriteGuard(mp.Name, lf, force); msg != "" {
			installFailures++
			_, _ = fmt.Fprintf(e.Err, "error: %s: %s\n", mp.Name, msg)
			continue
		}
		entry := manifest.Entry{Source: mp.Source, Path: mp.Path, Ref: mp.Ref}
		if err := e.applyFetch(mp.Name, entry, lf, force); err != nil {
			e.cleanupFailedAdd(mp.Name, false)
			installFailures++
			_, _ = fmt.Fprintf(e.Err, "error: %s: %v\n", mp.Name, importError(err, mp.Source))
			continue
		}
		m.Skills[mp.Name] = entry
		imported++
		_, _ = fmt.Fprintf(e.Out, "imported %s\n", mp.Name)
	}

	for _, fail := range failures {
		_, _ = fmt.Fprintf(e.Err, "error: %s: %s\n", fail.Name, fail.Reason)
	}

	if imported > 0 {
		if err := m.Save(e.Scope.ManifestPath); err != nil {
			return err
		}
		if err := lf.Save(e.Scope.LockPath); err != nil {
			return err
		}
	}

	if failed := installFailures + len(failures); failed > 0 {
		noun := "entries"
		if failed == 1 {
			noun = "entry"
		}
		return fmt.Errorf("%d %s could not be imported; see errors above", failed, noun)
	}
	return nil
}

// importOverwriteGuard refuses to replace an installed tree that import
// cannot prove pristine: a lock-only orphan with local edits, or a tree
// with no lock entry at all. It returns a failure message, or "" when the
// install may proceed.
func (e *Engine) importOverwriteGuard(name string, lf *lockfile.Lockfile, force bool) string {
	if force {
		return ""
	}
	hash, ok := e.installedHash(name)
	if !ok {
		return ""
	}
	locked := lf.Find(name)
	switch {
	case locked == nil:
		return fmt.Sprintf("skill %q is already installed but not managed by skiletto; refusing to overwrite it (run 'skiletto import --force' to replace)", name)
	case !locked.Editable && hash != locked.Hash:
		return fmt.Sprintf("skill %q is already installed with local modifications; refusing to overwrite it (run 'skiletto import --force' to replace)", name)
	default:
		return ""
	}
}

// importError rewrites errors whose stock guidance points at state import
// never wrote: an ambiguous source is fixed in skills-lock.json (not in a
// skiletto.toml entry), and a harness location occupied by a real
// directory — the normal leftover of an npx skills install — needs the old
// copy removed before re-running.
func importError(err error, source string) error {
	var multi *MultipleSkillsError
	if errors.As(err, &multi) {
		multi.FromImport = true
		multi.Source = source
		return err
	}
	var notLink *adapter.NotASymlinkError
	if errors.As(err, &notLink) {
		return fmt.Errorf("%w (likely installed by npx skills; remove it with 'rm -r %s' and re-run import)", err, notLink.Path)
	}
	return err
}
