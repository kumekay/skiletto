package engine

import (
	"fmt"

	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/vercelimport"
)

// Import bootstraps the scope's manifest and lockfile from a Vercel
// skills-lock.json. Each entry is mapped to a canonical git source, its
// default-branch HEAD is resolved to a commit and installed exactly like
// sync, and both files are written pinned. Entries already present in the
// manifest are skipped untouched; entries that cannot be mapped or resolved
// are reported and cause a non-zero exit, but never abort the entries that
// do resolve.
func (e *Engine) Import(lockPath string) error {
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
		entry := manifest.Entry{Source: mp.Source, Path: mp.Path, Ref: mp.Ref}
		if err := e.applyFetch(mp.Name, entry, lf, false); err != nil {
			e.cleanupFailedAdd(mp.Name, false)
			installFailures++
			_, _ = fmt.Fprintf(e.Err, "error: %s: %v\n", mp.Name, err)
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
