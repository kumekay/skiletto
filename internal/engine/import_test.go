package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

// writeVercelLock writes a skills-lock.json into dir and returns its path.
func writeVercelLock(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "skills-lock.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportInstallsLocksAndLinks(t *testing.T) {
	f := newFixture(t, pdfSource())
	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 1,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf"}
		}
	}`)

	if err := f.eng.Import(lock); err != nil {
		t.Fatalf("import: %v\n%s", err, f.errOut.String())
	}

	// Manifest records the mapped github source and skill path, no ref
	// (default branch) — that is the whole point of import.
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := m.Skills["pdf"]
	if !ok {
		t.Fatalf("manifest missing pdf: %+v", m.Skills)
	}
	if e.Source != "https://github.com/o/r" || e.Path != "skills/pdf" || e.Ref != "" {
		t.Errorf("manifest entry = %+v", e)
	}

	// Lock is fully pinned to a commit and content hash.
	s := f.readLock(t).Find("pdf")
	if s == nil || s.Commit != commitA || s.Hash == "" {
		t.Errorf("lock entry = %+v", s)
	}

	// Installed and linked.
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("skill not installed: %v", err)
	}
	if f.adapter.links["pdf"] == "" {
		t.Error("skill not linked")
	}
}

func TestImportPartialReportsFailuresAndExitsNonZero(t *testing.T) {
	f := newFixture(t, pdfSource())
	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 1,
		"skills": {
			"pdf":   {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf"},
			"local": {"source": "/somewhere", "sourceType": "local"},
			"nm":    {"source": "pkg", "sourceType": "node_modules"}
		}
	}`)

	err := f.eng.Import(lock)
	if err == nil {
		t.Fatal("want non-zero exit when an entry cannot be imported")
	}

	// The resolvable entry still imported.
	m, _ := manifest.Load(f.scope.ManifestPath)
	if _, ok := m.Skills["pdf"]; !ok {
		t.Errorf("resolvable entry not imported: %+v", m.Skills)
	}
	// The unmappable entries did not leak into the manifest.
	if _, ok := m.Skills["local"]; ok {
		t.Error("unmappable entry leaked into manifest")
	}

	// Each failure reported with its name and reason.
	errText := f.errOut.String()
	for _, want := range []string{"local", "node_modules", "nm"} {
		if !strings.Contains(errText, want) {
			t.Errorf("stderr missing %q:\n%s", want, errText)
		}
	}
}

func TestImportSkipsEntriesAlreadyInManifest(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: "https://github.com/existing/repo", Path: "skills/pdf"},
	}})
	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 1,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf"}
		}
	}`)

	if err := f.eng.Import(lock); err != nil {
		t.Fatalf("import: %v\n%s", err, f.errOut.String())
	}

	// The pre-existing entry is left untouched, not overwritten.
	m, _ := manifest.Load(f.scope.ManifestPath)
	if m.Skills["pdf"].Source != "https://github.com/existing/repo" {
		t.Errorf("existing entry overwritten: %+v", m.Skills["pdf"])
	}
	if !strings.Contains(f.out.String(), "pdf") || !strings.Contains(f.out.String(), "skip") {
		t.Errorf("no skip note printed:\n%s", f.out.String())
	}
}

func TestImportMissingFileErrors(t *testing.T) {
	f := newFixture(t, pdfSource())
	err := f.eng.Import(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("want error for missing skills-lock.json")
	}
}
