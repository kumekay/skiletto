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
