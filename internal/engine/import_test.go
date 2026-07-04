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
		"version": 3,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf/SKILL.md"}
		}
	}`)

	if err := f.eng.Import(lock, false); err != nil {
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
		"version": 3,
		"skills": {
			"pdf":   {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf/SKILL.md"},
			"local": {"source": "/somewhere", "sourceType": "local"},
			"nm":    {"source": "pkg", "sourceType": "node_modules"}
		}
	}`)

	err := f.eng.Import(lock, false)
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
		"version": 3,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf/SKILL.md"}
		}
	}`)

	if err := f.eng.Import(lock, false); err != nil {
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
	err := f.eng.Import(filepath.Join(t.TempDir(), "nope.json"), false)
	if err == nil {
		t.Fatal("want error for missing skills-lock.json")
	}
}

// A lock-only orphan (in the lock and on disk, removed from the manifest)
// with local edits must not be silently destroyed by import.
func TestImportRefusesToOverwriteDriftedOrphan(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	// Orphan the entry (still locked, still installed) and drift it.
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{}})
	mod := filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")
	if err := os.WriteFile(mod, []byte("# precious local edits"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 3,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf/SKILL.md"}
		}
	}`)
	err := f.eng.Import(lock, false)
	if err == nil {
		t.Fatal("want non-zero exit for drifted orphan")
	}
	data, _ := os.ReadFile(mod)
	if string(data) != "# precious local edits" {
		t.Errorf("local edits destroyed: content = %q", data)
	}
	if !strings.Contains(f.errOut.String(), "--force") {
		t.Errorf("error does not hint at --force:\n%s", f.errOut.String())
	}
	// The entry was not written to the manifest.
	m, _ := manifest.Load(f.scope.ManifestPath)
	if _, ok := m.Skills["pdf"]; ok {
		t.Error("drifted entry written to manifest without --force")
	}

	// --force overwrites and imports.
	if err := f.eng.Import(lock, true); err != nil {
		t.Fatalf("import --force: %v\n%s", err, f.errOut.String())
	}
	data, _ = os.ReadFile(mod)
	if string(data) != "# pdf" {
		t.Errorf("content after --force = %q", data)
	}
	m, _ = manifest.Load(f.scope.ManifestPath)
	if _, ok := m.Skills["pdf"]; !ok {
		t.Error("entry not imported with --force")
	}
}

// An installed tree with no lock entry at all (unmanaged) is just as
// unverifiable: refuse without --force.
func TestImportRefusesToOverwriteUnmanagedTree(t *testing.T) {
	f := newFixture(t, pdfSource())
	dir := f.scope.SkillDir("pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 3,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf/SKILL.md"}
		}
	}`)
	if err := f.eng.Import(lock, false); err == nil {
		t.Fatal("want non-zero exit for unmanaged installed tree")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(data) != "# unmanaged" {
		t.Errorf("unmanaged tree destroyed: content = %q", data)
	}

	if err := f.eng.Import(lock, true); err != nil {
		t.Fatalf("import --force: %v\n%s", err, f.errOut.String())
	}
	data, _ = os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(data) != "# pdf" {
		t.Errorf("content after --force = %q", data)
	}
}

// A multi-skill source without skillPath must point at skills-lock.json
// (or skiletto add), not at a skiletto.toml entry import never wrote.
func TestImportMultiSkillEntryPointsAtSkillsLock(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"skills/web/SKILL.md": "# web",
	}}
	f := newFixture(t, src)
	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 3,
		"skills": {
			"tools": {"source": "o/r", "sourceType": "github"}
		}
	}`)

	if err := f.eng.Import(lock, false); err == nil {
		t.Fatal("want error for ambiguous entry")
	}
	out := f.errOut.String()
	for _, want := range []string{
		"skillPath",
		"skills-lock.json",
		"skiletto add https://github.com/o/r//skills/pdf",
		"skiletto add https://github.com/o/r//skills/web",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "skiletto.toml") {
		t.Errorf("stderr points at a skiletto.toml entry import never wrote:\n%s", out)
	}
}

// A v3 lock names skills by their SKILL.md file. A root skill (skillPath
// "SKILL.md") must import as the source root itself — even when the repo
// also contains nested skills — and a nested entry must import from its
// directory.
func TestImportV3RootSkillAlongsideNestedSkill(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"SKILL.md":               "# root",
		"extras/helper/SKILL.md": "# helper",
	}}
	f := newFixture(t, src)
	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 3,
		"skills": {
			"root-skill": {
				"source": "o/r", "sourceType": "github",
				"sourceUrl": "https://github.com/o/r.git",
				"skillPath": "SKILL.md", "skillFolderHash": "abc",
				"installedAt": "2026-03-16T21:08:10.962Z",
				"updatedAt": "2026-05-12T19:31:07.260Z"
			},
			"helper": {
				"source": "o/r", "sourceType": "github",
				"sourceUrl": "https://github.com/o/r.git",
				"skillPath": "extras/helper/SKILL.md", "skillFolderHash": ""
			}
		},
		"dismissed": { "findSkillsPrompt": true },
		"lastSelectedAgents": ["amp"]
	}`)

	if err := f.eng.Import(lock, false); err != nil {
		t.Fatalf("import: %v\n%s", err, f.errOut.String())
	}

	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	// The root skill pins the source root explicitly ("."), so the nested
	// skill in the same repo cannot make it ambiguous.
	if e := m.Skills["root-skill"]; e.Source != "https://github.com/o/r" || e.Path != "." {
		t.Errorf("root-skill entry = %+v", e)
	}
	if e := m.Skills["helper"]; e.Path != "extras/helper" {
		t.Errorf("helper entry = %+v", e)
	}

	// Both installed with the right content and pinned in the lock.
	data, err := os.ReadFile(filepath.Join(f.scope.SkillDir("root-skill"), "SKILL.md"))
	if err != nil || string(data) != "# root" {
		t.Errorf("root-skill SKILL.md = %q, err %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(f.scope.SkillDir("helper"), "SKILL.md"))
	if err != nil || string(data) != "# helper" {
		t.Errorf("helper SKILL.md = %q, err %v", data, err)
	}
	for _, name := range []string{"root-skill", "helper"} {
		if s := f.readLock(t).Find(name); s == nil || s.Commit != commitA || s.Hash == "" {
			t.Errorf("lock entry %s = %+v", name, s)
		}
	}
}
