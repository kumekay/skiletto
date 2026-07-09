package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

// failHook exits non-zero on every platform, even with the staged dir
// appended as an argument.
const failHook = "git --skiletto-no-such-flag"

func TestAddNoHooksFlagBypassesFailingHook(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	m := &manifest.Manifest{
		Harnesses: []string{},
		Hooks:     map[string]string{"pre-install": failHook},
		Skills:    map[string]manifest.Entry{},
	}
	if err := m.Save(filepath.Join(project, "skiletto.toml")); err != nil {
		t.Fatal(err)
	}

	_, _, err := run(t, "add", repo+"//skills/pdf")
	if err == nil || !strings.Contains(err.Error(), "pre-install hook") {
		t.Fatalf("want pre-install hook error, got %v", err)
	}
	if _, stderr, err := run(t, "add", "--no-hooks", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add --no-hooks: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("skill not materialized: %v", err)
	}
}

func TestSyncAndUpdateAcceptNoHooksFlag(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	m, err := manifest.Load(filepath.Join(project, "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	m.Hooks = map[string]string{"pre-install": failHook}
	if err := m.Save(filepath.Join(project, "skiletto.toml")); err != nil {
		t.Fatal(err)
	}

	if _, stderr, err := run(t, "update", "--no-hooks"); err != nil {
		t.Fatalf("update --no-hooks: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "sync", "--no-hooks"); err != nil {
		t.Fatalf("sync --no-hooks: %v\n%s", err, stderr)
	}
}
