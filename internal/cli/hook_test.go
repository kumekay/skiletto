package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

// failHook exits non-zero on every platform.
const failHook = "git --skiletto-no-such-flag"

// setMachineHook writes a pre-install hook into the machine manifest (the
// only place hooks are honored) and restores the previous state afterwards,
// so the shared test HOME stays clean for other tests.
func setMachineHook(t *testing.T, command string) {
	t.Helper()
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "skiletto", "skiletto.toml")
	prev, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.WriteFile(path, prev, 0o644)
		} else {
			_ = os.Remove(path)
		}
	})
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m.Hooks = map[string]string{"pre-install": command}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
}

func TestAddNoHooksFlagBypassesFailingHook(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)
	setMachineHook(t, failHook)

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
	setMachineHook(t, failHook)

	if _, stderr, err := run(t, "update", "--no-hooks"); err != nil {
		t.Fatalf("update --no-hooks: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "sync", "--no-hooks"); err != nil {
		t.Fatalf("sync --no-hooks: %v\n%s", err, stderr)
	}
}
