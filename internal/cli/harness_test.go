package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/manifest"
)

// Without a TTY and with no harnesses key anywhere, add installs to the
// canonical dir only and says so; enabling the harness later links the
// already-installed skills.
func TestAddUnconfiguredNotesThenEnableLinks(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	setMachineHome(t)
	t.Chdir(project)

	stdout, stderr, err := run(t, "add", repo+"//skills/pdf")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "no harnesses configured") {
		t.Errorf("want canonical-only note, got stdout=%q", stdout)
	}
	link := filepath.Join(project, ".claude", "skills", "pdf")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("unconfigured add must not create harness links: %v", err)
	}
	m, err := manifest.Load(filepath.Join(project, "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Harnesses != nil {
		t.Errorf("fallback must not persist a harnesses key, got %#v", m.Harnesses)
	}

	if _, stderr, err := run(t, "harness", "enable", "claude"); err != nil {
		t.Fatalf("harness enable: %v\n%s", err, stderr)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("enable must link installed skills: %v", err)
	}
}

// An interactive first run asks once and persists the answer.
func TestAddPromptsForHarnessesOnce(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	setMachineHome(t)
	t.Chdir(project)

	calls := 0
	restore := harnessPrompter
	harnessPrompter = func(noInput bool) func([]engine.HarnessOption) ([]string, error) {
		return func(opts []engine.HarnessOption) ([]string, error) {
			calls++
			return []string{"claude"}, nil
		}
	}
	defer func() { harnessPrompter = restore }()

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	if calls != 1 {
		t.Fatalf("prompt calls = %d, want 1", calls)
	}
	link := filepath.Join(project, ".claude", "skills", "pdf")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("chosen harness must be linked: %v", err)
	}
	m, err := manifest.Load(filepath.Join(project, "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Harnesses) != 1 || m.Harnesses[0] != "claude" {
		t.Errorf("choice must persist, got %#v", m.Harnesses)
	}

	if _, stderr, err := run(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	if calls != 1 {
		t.Errorf("configured scope must not prompt again, calls = %d", calls)
	}
}

// harness disable removes the links and flips the manifest key.
func TestHarnessDisableEndToEnd(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	setMachineHome(t)
	t.Chdir(project)

	if _, stderr, err := run(t, "harness", "enable", "claude"); err != nil {
		t.Fatalf("harness enable: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	link := filepath.Join(project, ".claude", "skills", "pdf")
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("setup: claude not linked: %v", err)
	}

	if _, stderr, err := run(t, "harness", "disable", "claude"); err != nil {
		t.Fatalf("harness disable: %v\n%s", err, stderr)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("disable must remove the link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("canonical copy must survive disable: %v", err)
	}

	stdout, _, err := run(t, "harness", "list")
	if err != nil {
		t.Fatalf("harness list: %v", err)
	}
	if !strings.Contains(stdout, "claude") || !strings.Contains(stdout, "disabled") {
		t.Errorf("harness list should report claude disabled:\n%s", stdout)
	}
}
