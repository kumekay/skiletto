package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLockJSON serializes a Vercel-shaped skills-lock.json into dir.
func writeLockJSON(t *testing.T, dir string, skills map[string]map[string]string) string {
	t.Helper()
	doc := map[string]any{"version": 1, "skills": skills}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "skills-lock.json")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestImportEndToEnd drives the real command against local git repos (via the
// git sourceType, which system git clones from a plain path), including a
// deliberately broken entry, then re-imports to confirm existing entries are
// skipped. No network.
func TestImportEndToEnd(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	wantHead := gitT(t, repo, "rev-parse", "HEAD")
	project := t.TempDir()
	t.Chdir(project)

	writeLockJSON(t, project, map[string]map[string]string{
		"pdf": {
			"source":     repo,
			"sourceType": "git",
			"skillPath":  "skills/pdf",
		},
		"gone": {
			"source":     filepath.Join(t.TempDir(), "does-not-exist"),
			"sourceType": "git",
		},
		"weird": {
			"source":     "whatever",
			"sourceType": "node_modules",
		},
	})

	// Two entries fail, so import exits non-zero.
	stdout, stderr, err := run(t, "import")
	if err == nil {
		t.Fatalf("want non-zero exit; stdout=%s stderr=%s", stdout, stderr)
	}

	// pdf fully installed, pinned, and linked.
	if _, statErr := os.Stat(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")); statErr != nil {
		t.Errorf("pdf not materialized: %v", statErr)
	}
	link := filepath.Join(project, ".claude", "skills", "pdf")
	if fi, lerr := os.Lstat(link); lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("pdf not linked into claude: %v", lerr)
	}
	if !lockContains(t, project, wantHead) {
		t.Errorf("lock not pinned to HEAD %s", wantHead)
	}
	if !lockContains(t, project, "pdf") {
		t.Error("lock missing pdf")
	}
	man, _ := os.ReadFile(filepath.Join(project, "skiletto.toml"))
	if !strings.Contains(string(man), "pdf") {
		t.Errorf("manifest missing pdf:\n%s", man)
	}
	// The broken entries never leaked into the manifest.
	if strings.Contains(string(man), "gone") || strings.Contains(string(man), "weird") {
		t.Errorf("broken entry leaked into manifest:\n%s", man)
	}

	// Both failures reported with a reason.
	for _, want := range []string{"gone", "weird", "node_modules"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}

	// Re-import: the resolvable entry is now already in the manifest and is
	// skipped rather than re-installed or duplicated.
	stdout2, _, _ := run(t, "import")
	if !strings.Contains(stdout2, "skipping pdf") {
		t.Errorf("re-import did not skip pdf:\n%s", stdout2)
	}
}

func TestImportGlobalScope(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	home, config := setMachineHome(t)
	project := t.TempDir()
	t.Chdir(project)

	writeLockJSON(t, project, map[string]map[string]string{
		"pdf": {"source": repo, "sourceType": "git", "skillPath": "skills/pdf"},
	})

	if _, stderr, err := run(t, "import", "--global"); err != nil {
		t.Fatalf("import --global: %v\n%s", err, stderr)
	}
	// Manifest and lock land in the machine-scope config dir.
	if _, err := os.Stat(filepath.Join(config, "skiletto", "skiletto.lock")); err != nil {
		t.Errorf("machine-scope lock missing: %v", err)
	}
	// Skill materializes under the machine home.
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("skill not materialized under home: %v", err)
	}
}

func TestImportMissingFileGuidance(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _, err := run(t, "import")
	if err == nil {
		t.Fatal("want error when skills-lock.json is absent")
	}
	if !strings.Contains(err.Error(), "skiletto import") {
		t.Errorf("error lacks guidance: %v", err)
	}
}

// The primary migration case: npx skills left real skill directories in
// .claude/skills. Import must keep refusing to replace them but tell the
// user how to migrate.
func TestImportHintsOnPreexistingClaudeSkillDir(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	old := filepath.Join(project, ".claude", "skills", "pdf")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "SKILL.md"), []byte("# npx install"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeLockJSON(t, project, map[string]map[string]string{
		"pdf": {"source": repo, "sourceType": "git", "skillPath": "skills/pdf"},
	})

	_, stderr, err := run(t, "import")
	if err == nil {
		t.Fatal("want non-zero exit when the harness dir is occupied")
	}
	// The old directory is never touched.
	data, rerr := os.ReadFile(filepath.Join(old, "SKILL.md"))
	if rerr != nil || string(data) != "# npx install" {
		t.Errorf("pre-existing skill dir modified: %q %v", data, rerr)
	}
	// The error carries migration guidance naming the directory to remove.
	for _, want := range []string{"rm -r", old} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
}

// --force is passed through to the engine: an unmanaged tree occupying the
// canonical location blocks import until --force replaces it.
func TestImportForceFlag(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	dir := filepath.Join(project, ".agents", "skills", "pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeLockJSON(t, project, map[string]map[string]string{
		"pdf": {"source": repo, "sourceType": "git", "skillPath": "skills/pdf"},
	})

	if _, _, err := run(t, "import"); err == nil {
		t.Fatal("want non-zero exit for occupied canonical location")
	}
	if _, stderr, err := run(t, "import", "--force"); err != nil {
		t.Fatalf("import --force: %v\n%s", err, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(data) != "# pdf" {
		t.Errorf("content after --force = %q", data)
	}
}
