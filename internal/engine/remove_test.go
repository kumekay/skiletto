package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

func TestRemoveCleansManifestLockLinkAndCopy(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Remove("pdf", false); err != nil {
		t.Fatalf("remove: %v", err)
	}
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Skills["pdf"]; ok {
		t.Error("manifest entry not removed")
	}
	if f.readLock(t).Find("pdf") != nil {
		t.Error("lock entry not removed")
	}
	if _, err := os.Stat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("materialized copy not deleted")
	}
	if _, ok := f.adapter.links["pdf"]; ok {
		t.Error("adapter link not removed")
	}
}

func TestRemoveEditableLeavesWorktreeIntact(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(file, []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"my-skill": {Source: worktree, Path: "my-skill", Editable: true},
	}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Remove("my-skill", false); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// Canonical symlink gone.
	if _, err := os.Lstat(f.scope.SkillDir("my-skill")); !os.IsNotExist(err) {
		t.Error("canonical symlink not removed")
	}
	// Working tree untouched.
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("working tree deleted: %v", err)
	}
	if string(data) != "# mine" {
		t.Errorf("working tree content changed: %q", data)
	}
	if f.readLock(t).Find("my-skill") != nil {
		t.Error("lock entry not removed")
	}
}

func TestRemoveUnknownNameErrors(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	err := f.eng.Remove("nope", false)
	if err == nil {
		t.Fatal("want error for unknown skill")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error missing name: %v", err)
	}
}

func TestRemoveDriftedRefusesWithoutForce(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"), []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Remove("pdf", false); err == nil {
		t.Fatal("want error removing drifted skill")
	}
	if _, err := os.Stat(f.scope.SkillDir("pdf")); err != nil {
		t.Error("drifted skill deleted without --force")
	}
	if f.readLock(t).Find("pdf") == nil {
		t.Error("lock entry removed despite refused remove")
	}

	// --force removes it.
	if err := f.eng.Remove("pdf", true); err != nil {
		t.Fatalf("remove --force: %v", err)
	}
	if _, err := os.Stat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("drifted skill not removed with --force")
	}
}
