package scope

import (
	"os"
	"path/filepath"
	"testing"
)

// envOf builds a getenv stand-in from a map.
func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// writeMachineManifest creates dir/skiletto.toml.
func writeMachineManifest(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skiletto.toml"), []byte("[skills]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveMachineEnvOverrideWins(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	res, err := ResolveMachine(home,
		envOf(map[string]string{"SKILETTO_CONFIG_DIR": dir, "XDG_CONFIG_HOME": "/xdg"}),
		func() (string, error) { return "/osdefault", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join(dir, "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if res.Source != "SKILETTO_CONFIG_DIR" {
		t.Errorf("Source = %q, want SKILETTO_CONFIG_DIR", res.Source)
	}
}

func TestResolveMachineXDG(t *testing.T) {
	home := t.TempDir()
	res, err := ResolveMachine(home,
		envOf(map[string]string{"XDG_CONFIG_HOME": "/xdg"}),
		func() (string, error) { return "/osdefault", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join("/xdg", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if res.Source != "XDG_CONFIG_HOME" {
		t.Errorf("Source = %q, want XDG_CONFIG_HOME", res.Source)
	}
}

func TestResolveMachineDotConfigFallback(t *testing.T) {
	home := t.TempDir()
	writeMachineManifest(t, filepath.Join(home, ".config", "skiletto"))
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return "/osdefault", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join(home, ".config", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if res.Source != "~/.config/skiletto" {
		t.Errorf("Source = %q, want ~/.config/skiletto", res.Source)
	}
}

// A ~/.config/skiletto directory without skiletto.toml does not count: the
// platform default wins, so a fresh machine install lands in the OS dir.
func TestResolveMachineDotConfigDirAloneIsNotEnough(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "skiletto"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return "/osdefault", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join("/osdefault", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if res.Source != "platform default" {
		t.Errorf("Source = %q, want platform default", res.Source)
	}
}

func TestResolveMachinePlatformDefault(t *testing.T) {
	home := t.TempDir()
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return "/osdefault", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join("/osdefault", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if res.Source != "platform default" {
		t.Errorf("Source = %q, want platform default", res.Source)
	}
	if len(res.Shadowed) != 0 {
		t.Errorf("Shadowed = %v, want none", res.Shadowed)
	}
}

// The fallback wins over the platform default, and a manifest left in the
// platform default is reported as shadowed.
func TestResolveMachineShadowedPlatformDefault(t *testing.T) {
	home := t.TempDir()
	writeMachineManifest(t, filepath.Join(home, ".config", "skiletto"))
	osDefault := t.TempDir()
	writeMachineManifest(t, filepath.Join(osDefault, "skiletto"))
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return osDefault, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join(home, ".config", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	want := filepath.Join(osDefault, "skiletto", "skiletto.toml")
	if len(res.Shadowed) != 1 || res.Shadowed[0] != want {
		t.Errorf("Shadowed = %v, want [%s]", res.Shadowed, want)
	}
}

// XDG pointing at ~/.config while a stray platform-default manifest exists:
// the shadow is still reported.
func TestResolveMachineShadowedWhenXDGIsDotConfig(t *testing.T) {
	home := t.TempDir()
	writeMachineManifest(t, filepath.Join(home, ".config", "skiletto"))
	osDefault := t.TempDir()
	writeMachineManifest(t, filepath.Join(osDefault, "skiletto"))
	res, err := ResolveMachine(home,
		envOf(map[string]string{"XDG_CONFIG_HOME": filepath.Join(home, ".config")}),
		func() (string, error) { return osDefault, nil })
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "XDG_CONFIG_HOME" {
		t.Errorf("Source = %q, want XDG_CONFIG_HOME", res.Source)
	}
	want := filepath.Join(osDefault, "skiletto", "skiletto.toml")
	if len(res.Shadowed) != 1 || res.Shadowed[0] != want {
		t.Errorf("Shadowed = %v, want [%s]", res.Shadowed, want)
	}
}

// On Linux the platform default IS ~/.config: the candidate list must not
// report the resolved manifest as its own shadow.
func TestResolveMachineNoSelfShadowOnLinux(t *testing.T) {
	home := t.TempDir()
	writeMachineManifest(t, filepath.Join(home, ".config", "skiletto"))
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return filepath.Join(home, ".config"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join(home, ".config", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if len(res.Shadowed) != 0 {
		t.Errorf("Shadowed = %v, want none", res.Shadowed)
	}
}

func TestResolveMachineDeduplicatesShadowedPaths(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	shadowed := filepath.Join(home, ".config", "skiletto")
	writeMachineManifest(t, shadowed)

	res, err := ResolveMachine(home,
		envOf(map[string]string{"SKILETTO_CONFIG_DIR": dir}),
		func() (string, error) { return filepath.Join(home, ".config"), nil })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(shadowed, "skiletto.toml")
	if len(res.Shadowed) != 1 || res.Shadowed[0] != want {
		t.Errorf("Shadowed = %v, want [%s]", res.Shadowed, want)
	}
}

// SKILETTO_CONFIG_DIR overriding while stray configs exist elsewhere reports
// both as shadowed.
func TestResolveMachineEnvOverrideShadowsBoth(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	writeMachineManifest(t, filepath.Join(home, ".config", "skiletto"))
	osDefault := t.TempDir()
	writeMachineManifest(t, filepath.Join(osDefault, "skiletto"))
	res, err := ResolveMachine(home,
		envOf(map[string]string{"SKILETTO_CONFIG_DIR": dir}),
		func() (string, error) { return osDefault, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Shadowed) != 2 {
		t.Fatalf("Shadowed = %v, want two entries", res.Shadowed)
	}
}

func TestResolveMachineUserConfigDirError(t *testing.T) {
	home := t.TempDir()
	_, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return "", os.ErrNotExist })
	if err == nil {
		t.Fatal("expected an error when the platform config dir is unavailable")
	}
}

// The ~/.config fallback rescues a platform default that cannot be
// resolved: a manifest already synced into ~/.config stays usable.
func TestResolveMachineFallbackWhenPlatformDefaultUnavailable(t *testing.T) {
	home := t.TempDir()
	writeMachineManifest(t, filepath.Join(home, ".config", "skiletto"))
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return "", os.ErrNotExist })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.ManifestPath, filepath.Join(home, ".config", "skiletto", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
}

func TestResolveMachineSkillsDir(t *testing.T) {
	home := t.TempDir()
	res, err := ResolveMachine(home, envOf(nil),
		func() (string, error) { return "/osdefault", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Scope.SkillsDir, filepath.Join(home, ".agents", "skills"); got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
	if res.Scope.Kind != KindMachine {
		t.Errorf("Kind = %v, want KindMachine", res.Scope.Kind)
	}
}
