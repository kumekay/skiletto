package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
)

// SkillStatus is the observed state of one skill for list.
type SkillStatus struct {
	Name     string
	Source   string // empty for unmanaged skills
	Commit   string // short pinned commit, empty when editable/unmanaged/unlocked
	Editable bool
	// Status is one of: ok, drifted, missing, not-locked, unmanaged.
	Status string
}

// Status observes managed skills (from the manifest, cross-checked against
// the lock and disk) and unmanaged skills (present in the canonical skills
// dir but absent from the manifest). It never changes anything.
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
	for _, name := range names {
		out = append(out, e.managedStatus(name, m.Skills[name], lf.Find(name)))
	}
	out = append(out, e.unmanagedStatuses(m)...)
	return out, nil
}

// managedStatus classifies a manifest entry against its lock entry and the
// installed tree.
func (e *Engine) managedStatus(name string, entry manifest.Entry, locked *lockfile.Skill) SkillStatus {
	s := SkillStatus{Name: name, Source: entry.Source, Editable: entry.Editable}
	switch {
	case entry.Editable:
		if _, err := os.Lstat(e.Scope.SkillDir(name)); err == nil {
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

// unmanagedStatuses lists directories in the canonical skills dir that are
// not in the manifest. They are reported, never touched.
func (e *Engine) unmanagedStatuses(m *manifest.Manifest) []SkillStatus {
	entries, err := os.ReadDir(e.Scope.SkillsDir)
	if err != nil {
		return nil
	}
	var out []SkillStatus
	for _, de := range entries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue // hidden entries and staging temp dirs
		}
		if _, ok := m.Skills[name]; ok {
			continue
		}
		out = append(out, SkillStatus{Name: name, Status: "unmanaged"})
	}
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
