package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

// freshHome isolates a test from the TestMain-wide home: HOME and
// XDG_CONFIG_HOME both move to a throwaway directory.
func freshHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILETTO_CONFIG_DIR", "")
	return home
}

// withPlatformConfigDir pins the platform config dir (os.UserConfigDir)
// for the test, so macOS resolution behavior is testable on any OS.
func withPlatformConfigDir(t *testing.T, dir string) {
	t.Helper()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = prev })
}

// writeToml writes content to dir/skiletto.toml, creating dir.
func writeToml(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skiletto.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A chezmoi-synced ~/.config/skiletto/skiletto.toml is picked up on every
// platform, for reads and writes alike, even when XDG_CONFIG_HOME is not
// exported.
func TestMachineConfigDotConfigFallback(t *testing.T) {
	home := freshHome(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	cfgDir := filepath.Join(home, ".config", "skiletto")
	writeToml(t, cfgDir, "harnesses = [\"claude\"]\n\n[skills]\n")
	withPlatformConfigDir(t, filepath.Join(home, "Library", "Application Support"))

	repo := makeSkillRepo(t, "pdf")
	t.Chdir(t.TempDir())
	if _, stderr, err := run(t, "add", "-g", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}

	m, err := manifest.Load(filepath.Join(cfgDir, "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Skills["pdf"]; !ok {
		t.Errorf("pdf was not written to ~/.config/skiletto/skiletto.toml:\n%v", m.Skills)
	}
	stray := filepath.Join(home, "Library", "Application Support", "skiletto", "skiletto.toml")
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("fallback active, yet the platform default %s was created", stray)
	}
}

// No ~/.config/skiletto manifest: a fresh install lands in the platform
// default, never auto-creating ~/.config/skiletto.
func TestMachineConfigFreshInstallPlatformDefault(t *testing.T) {
	home := freshHome(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	platform := filepath.Join(home, "Library", "Application Support")
	withPlatformConfigDir(t, platform)

	repo := makeSkillRepo(t, "pdf")
	t.Chdir(t.TempDir())
	if _, stderr, err := run(t, "add", "-g", "--no-input", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}

	if _, err := os.Stat(filepath.Join(platform, "skiletto", "skiletto.toml")); err != nil {
		t.Errorf("fresh install did not create the platform default manifest: %v", err)
	}
	dotConfig := filepath.Join(home, ".config", "skiletto", "skiletto.toml")
	if _, err := os.Stat(dotConfig); err == nil {
		t.Errorf("fresh install must not create %s", dotConfig)
	}
}

// Configs in both locations: the winner reads and writes, the shadowed one
// gets a warning naming it.
func TestMachineConfigShadowWarning(t *testing.T) {
	home := freshHome(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	platform := filepath.Join(home, "Library", "Application Support")
	withPlatformConfigDir(t, platform)
	writeToml(t, filepath.Join(home, ".config", "skiletto"), "harnesses = []\n\n[skills]\n")
	writeToml(t, filepath.Join(platform, "skiletto"), "harnesses = []\n\n[skills]\n")

	t.Chdir(t.TempDir())
	_, stderr, err := run(t, "list", "-g")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, filepath.Join(platform, "skiletto", "skiletto.toml")) {
		t.Errorf("warning does not name the shadowed manifest:\n%s", stderr)
	}
	if !strings.Contains(stderr, "shadowed") {
		t.Errorf("warning does not say 'shadowed':\n%s", stderr)
	}
}

func TestDoctorMachineSection(t *testing.T) {
	home := freshHome(t)
	cfgDir := filepath.Join(home, ".config", "skiletto")
	writeToml(t, cfgDir, "harnesses = [\"claude\"]\n\n[skills]\n")
	t.Chdir(t.TempDir())

	stdout, stderr, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, stderr)
	}
	for _, want := range []string{
		version,
		filepath.Join(cfgDir, "skiletto.toml") + " (exists)",
		filepath.Join(cfgDir, "skiletto.lock") + " (missing)",
		"XDG_CONFIG_HOME",
		filepath.Join(home, ".agents", "skills"),
		"claude",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor output missing %q:\n%s", want, stdout)
		}
	}
}

func TestDoctorShadowWarning(t *testing.T) {
	home := freshHome(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	platform := filepath.Join(home, "Library", "Application Support")
	withPlatformConfigDir(t, platform)
	writeToml(t, filepath.Join(home, ".config", "skiletto"), "harnesses = []\n\n[skills]\n")
	writeToml(t, filepath.Join(platform, "skiletto"), "harnesses = []\n\n[skills]\n")
	t.Chdir(t.TempDir())

	stdout, stderr, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, stderr)
	}
	out := stdout + stderr
	if !strings.Contains(out, filepath.Join(platform, "skiletto", "skiletto.toml")) {
		t.Errorf("doctor does not name the shadowed manifest:\n%s", out)
	}
	if !strings.Contains(out, "shadowed") {
		t.Errorf("doctor does not say 'shadowed':\n%s", out)
	}
}

func TestDoctorProjectSectionAndSkillHealth(t *testing.T) {
	freshHome(t)
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)
	if _, stderr, err := run(t, "harness", "enable", "claude"); err != nil {
		t.Fatalf("harness enable: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	// Drift the installed skill so the health report has something to show.
	if err := os.WriteFile(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md"), []byte("hacked"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, stderr)
	}
	for _, want := range []string{
		project,
		filepath.Join(project, "skiletto.toml") + " (exists)",
		"drifted",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor output missing %q:\n%s", want, stdout)
		}
	}
}

func TestDoctorNoProject(t *testing.T) {
	freshHome(t)
	t.Chdir(t.TempDir())
	stdout, stderr, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "no skiletto.toml") {
		t.Errorf("doctor output does not report the missing project manifest:\n%s", stdout)
	}
}
