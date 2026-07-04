package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
)

// SkillStatus is the observed state of one skill for list.
type SkillStatus struct {
	Name     string
	Source   string // empty for unmanaged skills
	Commit   string // short pinned commit, empty when editable/unmanaged/unlocked
	Editable bool
	// Status is one of: ok, drifted, missing, not-locked,
	// "pruned on next sync" (locked but gone from the manifest), unmanaged.
	Status string
}

// Status observes managed skills (from the manifest, cross-checked against
// the lock and disk), lock-only orphans that the next sync will prune, and
// unmanaged skills found in the canonical skills dir or an adapter's skills
// dir but absent from the manifest. It never changes anything.
func (e *Engine) Status() ([]SkillStatus, error) {
	m, lf, err := e.load()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(m.Skills))
	for name := range m.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]SkillStatus, 0, len(names))
	claimed := make(map[string]bool, len(names))
	for _, name := range names {
		out = append(out, e.managedStatus(name, m.Skills[name], lf.Find(name)))
		claimed[name] = true
	}
	out = append(out, orphanStatuses(m, lf, claimed)...)
	out = append(out, e.unmanagedStatuses(claimed)...)
	return out, nil
}

// managedStatus classifies a manifest entry against its lock entry and the
// installed tree.
func (e *Engine) managedStatus(name string, entry manifest.Entry, locked *lockfile.Skill) SkillStatus {
	s := SkillStatus{Name: name, Source: entry.Source, Editable: entry.Editable}
	switch {
	case entry.Editable:
		// Stat follows the canonical symlink: a broken link (the linked
		// working tree was deleted) counts as missing.
		if _, err := os.Stat(e.Scope.SkillDir(name)); err == nil {
			s.Status = "ok"
		} else {
			s.Status = "missing"
		}
	case locked == nil || lockMismatch(entry, *locked):
		s.Status = "not-locked"
	default:
		s.Commit = shortCommit(locked.Commit)
		switch hash, ok := e.installedHash(name); {
		case !ok:
			s.Status = "missing"
		case hash == locked.Hash:
			s.Status = "ok"
		default:
			s.Status = "drifted"
		}
	}
	return s
}

// orphanStatuses reports lock entries gone from the manifest. Unlike truly
// unmanaged dirs they keep their lock identity and the next sync will
// prune them, so they get a distinct status. Reported names are added to
// claimed so the disk scan does not repeat them.
func orphanStatuses(m *manifest.Manifest, lf *lockfile.Lockfile, claimed map[string]bool) []SkillStatus {
	var out []SkillStatus
	for _, locked := range lf.Skills {
		if _, ok := m.Skills[locked.Name]; ok {
			continue
		}
		s := SkillStatus{
			Name:     locked.Name,
			Source:   locked.Source,
			Editable: locked.Editable,
			Status:   "pruned on next sync",
		}
		if !locked.Editable {
			s.Commit = shortCommit(locked.Commit)
		}
		out = append(out, s)
		claimed[locked.Name] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// unmanagedStatuses lists directories found in the canonical skills dir or
// an adapter's skills dir whose names are not claimed by the manifest or
// the lock. Symlinks an adapter dir holds into the canonical skills dir
// are skiletto's own links, not unmanaged skills. Everything reported here
// is listed only, never touched.
func (e *Engine) unmanagedStatuses(claimed map[string]bool) []SkillStatus {
	dirs := []string{e.Scope.SkillsDir}
	for _, a := range e.Adapters {
		dirs = append(dirs, a.SkillsDir(e.Scope))
	}
	seen := map[string]bool{}
	var out []SkillStatus
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range entries {
			name := de.Name()
			if strings.HasPrefix(name, ".") {
				continue // hidden entries and staging temp dirs
			}
			if claimed[name] || seen[name] {
				continue
			}
			if adapter.IsOwnLink(e.Scope.SkillsDir, filepath.Join(dir, name)) {
				continue
			}
			seen[name] = true
			out = append(out, SkillStatus{Name: name, Status: "unmanaged"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// List renders Status as a table on the engine's output. It always exits
// without error: listing observes, it does not act on drift.
func (e *Engine) List() error {
	statuses, err := e.Status()
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		_, _ = fmt.Fprintln(e.Out, "no skills managed")
		return nil
	}
	tw := tabwriter.NewWriter(e.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tSTATUS\tSOURCE")
	for _, s := range statuses {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, versionCell(s), s.Status, dash(s.Source))
	}
	return tw.Flush()
}

// versionCell is the VERSION column: "editable", a short commit, or "-".
func versionCell(s SkillStatus) string {
	if s.Editable {
		return "editable"
	}
	return dash(s.Commit)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// shortCommit truncates a commit SHA for display.
func shortCommit(commit string) string {
	const n = 12
	if len(commit) > n {
		return commit[:n]
	}
	return commit
}
