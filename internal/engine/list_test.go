package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

func statusByName(ss []SkillStatus) map[string]SkillStatus {
	m := map[string]SkillStatus{}
	for _, s := range ss {
		m[s.Name] = s
	}
	return m
}

func TestStatusReportsAllStates(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"skills/web/SKILL.md": "# web",
		"skills/doc/SKILL.md": "# doc",
		"skills/new/SKILL.md": "# new",
	}}
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: "https://github.com/o/r", Path: "skills/pdf"},
		"web": {Source: "https://github.com/o/r", Path: "skills/web"},
		"doc": {Source: "https://github.com/o/r", Path: "skills/doc"},
	}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	// web drifts.
	if err := os.WriteFile(filepath.Join(f.scope.SkillDir("web"), "SKILL.md"), []byte("hacked"), 0o644); err != nil {
		t.Fatal(err)
	}
	// doc goes missing on disk.
	if err := os.RemoveAll(f.scope.SkillDir("doc")); err != nil {
		t.Fatal(err)
	}
	// new is added to the manifest but never synced (not-locked).
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: "https://github.com/o/r", Path: "skills/pdf"},
		"web": {Source: "https://github.com/o/r", Path: "skills/web"},
		"doc": {Source: "https://github.com/o/r", Path: "skills/doc"},
		"new": {Source: "https://github.com/o/r", Path: "skills/new"},
	}})
	// An unmanaged directory sits in the skills dir.
	orphan := f.scope.SkillDir("orphan")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "SKILL.md"), []byte("# orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	ss, err := f.eng.Status()
	if err != nil {
		t.Fatal(err)
	}
	byName := statusByName(ss)

	want := map[string]string{
		"pdf":    "ok",
		"web":    "drifted",
		"doc":    "missing",
		"new":    "not-locked",
		"orphan": "unmanaged",
	}
	for name, status := range want {
		got, ok := byName[name]
		if !ok {
			t.Errorf("status missing %q", name)
			continue
		}
		if got.Status != status {
			t.Errorf("%s status = %q, want %q", name, got.Status, status)
		}
	}
	// Managed pinned entries carry a short commit; unmanaged carries none.
	if c := byName["pdf"].Commit; c == "" || len(c) >= len(commitA) {
		t.Errorf("pdf commit = %q, want a shortened commit", c)
	}
	if byName["pdf"].Source != "https://github.com/o/r" {
		t.Errorf("pdf source = %q", byName["pdf"].Source)
	}
}

func TestStatusEditable(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"my-skill": {Source: worktree, Path: "my-skill", Editable: true},
	}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	ss, err := f.eng.Status()
	if err != nil {
		t.Fatal(err)
	}
	s := statusByName(ss)["my-skill"]
	if !s.Editable {
		t.Errorf("editable flag not set: %+v", s)
	}
	if s.Status != "ok" {
		t.Errorf("editable status = %q, want ok", s.Status)
	}
}

// Finding 1: a lock-only orphan (still in the lock and on disk, removed
// from the manifest) will be pruned by the next sync, unlike a truly
// unmanaged dir. It must be visibly distinct and keep its lock identity.
func TestStatusLockOnlyOrphanIsDistinct(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	// Drop pdf from the manifest without syncing: lock + disk still have it.
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{}})

	ss, err := f.eng.Status()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := statusByName(ss)["pdf"]
	if !ok {
		t.Fatal("orphan missing from status")
	}
	if s.Status == "unmanaged" {
		t.Errorf("orphan status = %q, want it distinct from unmanaged", s.Status)
	}
	if !strings.Contains(s.Status, "prune") {
		t.Errorf("orphan status = %q, want it to say it will be pruned", s.Status)
	}
	if s.Source != "https://github.com/o/r" {
		t.Errorf("orphan source = %q, want the lock entry's source", s.Source)
	}
	if s.Commit == "" || len(s.Commit) >= len(commitA) {
		t.Errorf("orphan commit = %q, want the lock entry's short commit", s.Commit)
	}
	// It appears exactly once (not repeated by the disk scan).
	count := 0
	for _, st := range ss {
		if st.Name == "pdf" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("orphan reported %d times, want 1", count)
	}
}

// Finding 2: an editable skill whose linked working tree was deleted is a
// broken symlink; it must report missing, not ok.
func TestStatusEditableBrokenLinkIsMissing(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"my-skill": {Source: worktree, Path: "my-skill", Editable: true},
	}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	// Delete the working tree: the canonical symlink is now broken.
	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatal(err)
	}
	ss, err := f.eng.Status()
	if err != nil {
		t.Fatal(err)
	}
	if s := statusByName(ss)["my-skill"]; s.Status != "missing" {
		t.Errorf("broken editable link status = %q, want missing", s.Status)
	}
}

// Finding 3: unmanaged skills in adapter dirs (the typical pre-skiletto
// install) must be listed too; our own symlinks into the canonical dir
// must not be double-reported, and a name present in several locations is
// reported once.
func TestStatusScansAdapterDirsForUnmanaged(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	adapterDir := f.adapter.SkillsDir(f.scope)
	// A pre-existing real skill dir in the adapter dir.
	legacy := filepath.Join(adapterDir, "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "SKILL.md"), []byte("# legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Our own symlink into the canonical dir (relative, like the claude
	// adapter creates them): must not be reported.
	rel, err := filepath.Rel(adapterDir, f.scope.SkillDir("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rel, filepath.Join(adapterDir, "pdf")); err != nil {
		t.Fatal(err)
	}
	// The same stray name in the canonical dir and the adapter dir: once.
	for _, dir := range []string{f.scope.SkillDir("stray"), filepath.Join(adapterDir, "stray")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ss, err := f.eng.Status()
	if err != nil {
		t.Fatal(err)
	}
	byName := statusByName(ss)
	if s, ok := byName["legacy"]; !ok || s.Status != "unmanaged" {
		t.Errorf("legacy adapter-dir skill = %+v (found=%v), want unmanaged", s, ok)
	}
	if s := byName["pdf"]; s.Status != "ok" {
		t.Errorf("managed pdf reported as %q via its adapter link", s.Status)
	}
	counts := map[string]int{}
	for _, s := range ss {
		counts[s.Name]++
	}
	if counts["pdf"] != 1 {
		t.Errorf("pdf reported %d times, want 1", counts["pdf"])
	}
	if counts["stray"] != 1 {
		t.Errorf("stray reported %d times, want 1", counts["stray"])
	}
}

func TestListWritesTableAndExitsZeroOnDrift(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.List(); err != nil {
		t.Errorf("list returned error on drift: %v", err)
	}
	out := f.out.String()
	for _, want := range []string{"pdf", "drifted"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}
