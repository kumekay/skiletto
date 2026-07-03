package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

const commitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestUpdateReResolvesAndRelocks(t *testing.T) {
	src := pdfSource()
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if s := f.readLock(t).Find("pdf"); s == nil || s.Commit != commitA {
		t.Fatalf("initial lock = %+v", s)
	}

	// Upstream advances to a new commit with new content.
	src.commit = commitB
	src.tree["skills/pdf/SKILL.md"] = "# pdf v2"

	if err := f.eng.Update("", false); err != nil {
		t.Fatalf("update: %v", err)
	}
	s := f.readLock(t).Find("pdf")
	if s == nil || s.Commit != commitB {
		t.Errorf("lock commit after update = %+v, want %s", s, commitB)
	}
	data, err := os.ReadFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf v2" {
		t.Errorf("installed content after update = %q", data)
	}
}

func TestUpdateNameOnlyReResolvesThatSkill(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"skills/web/SKILL.md": "# web",
	}}
	count := 0
	src.resolved = &count
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: "https://github.com/o/r", Path: "skills/pdf"},
		"web": {Source: "https://github.com/o/r", Path: "skills/web"},
	}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("sync resolved %d times, want 2", count)
	}

	count = 0
	if err := f.eng.Update("pdf", false); err != nil {
		t.Fatalf("update pdf: %v", err)
	}
	if count != 1 {
		t.Errorf("update pdf re-resolved %d entries, want 1", count)
	}
}

func TestUpdateUnknownNameErrors(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	err := f.eng.Update("nope", false)
	if err == nil {
		t.Fatal("want error for unknown skill")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error missing name: %v", err)
	}
}

func TestUpdateEditableSkippedWithNote(t *testing.T) {
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

	if err := f.eng.Update("", false); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(f.out.String(), "my-skill") {
		t.Errorf("no editable note printed:\n%s", f.out.String())
	}
	s := f.readLock(t).Find("my-skill")
	if s == nil || !s.Editable || s.Commit != "" {
		t.Errorf("editable lock entry changed: %+v", s)
	}
}

func TestUpdateDriftBlockedWithoutForce(t *testing.T) {
	src := pdfSource()
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")
	if err := os.WriteFile(mod, []byte("# hacked"), 0o644); err != nil {
		t.Fatal(err)
	}
	src.commit = commitB
	src.tree["skills/pdf/SKILL.md"] = "# pdf v2"

	err := f.eng.Update("", false)
	if err == nil {
		t.Fatal("want drift error")
	}
	data, _ := os.ReadFile(mod)
	if string(data) != "# hacked" {
		t.Errorf("drifted file overwritten without --force: %q", data)
	}
	if !strings.Contains(f.errOut.String(), "pdf") {
		t.Errorf("no drift warning:\n%s", f.errOut.String())
	}
	// Lock not moved.
	if s := f.readLock(t).Find("pdf"); s == nil || s.Commit != commitA {
		t.Errorf("lock moved despite drift: %+v", s)
	}

	// --force re-resolves and overwrites.
	if err := f.eng.Update("", true); err != nil {
		t.Fatalf("update --force: %v", err)
	}
	data, _ = os.ReadFile(mod)
	if string(data) != "# pdf v2" {
		t.Errorf("content after force = %q", data)
	}
	if s := f.readLock(t).Find("pdf"); s == nil || s.Commit != commitB {
		t.Errorf("lock not updated after force: %+v", s)
	}
}
