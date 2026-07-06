package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/scope"
)

// byName indexes the registered adapters so the tests exercise the real
// instances wired up in init(), which also proves registration.
func byName(t *testing.T) map[string]adapter.Adapter {
	t.Helper()
	m := map[string]adapter.Adapter{}
	for _, a := range adapter.All() {
		m[a.Name()] = a
	}
	return m
}

func get(t *testing.T, name string) adapter.Adapter {
	t.Helper()
	a, ok := byName(t)[name]
	if !ok {
		t.Fatalf("adapter %q not registered", name)
	}
	return a
}

func TestSkillsDirProject(t *testing.T) {
	cases := map[string]string{
		"claude":      ".claude/skills",
		"codex":       ".agents/skills",
		"vibe":        ".vibe/skills",
		"hermes":      ".hermes/skills",
		"antigravity": ".agents/skills",
		"cursor":      ".agents/skills",
		"pi":          ".pi/skills",
	}
	s := scope.Project("/repo")
	for name, rel := range cases {
		want := filepath.Join("/repo", filepath.FromSlash(rel))
		if got := get(t, name).SkillsDir(s); got != want {
			t.Errorf("%s SkillsDir(project) = %q, want %q", name, got, want)
		}
	}
}

func TestSkillsDirMachine(t *testing.T) {
	// Clear every override so the defaults are exercised.
	for _, env := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "VIBE_HOME", "HERMES_HOME"} {
		t.Setenv(env, "")
	}
	cases := map[string]string{
		"claude":      ".claude/skills",
		"codex":       ".codex/skills",
		"vibe":        ".vibe/skills",
		"hermes":      ".hermes/skills",
		"antigravity": ".gemini/antigravity/skills",
		"cursor":      ".cursor/skills",
		"pi":          ".pi/agent/skills",
	}
	s := scope.Machine("/home/u", "/home/u/.config")
	for name, rel := range cases {
		want := filepath.Join("/home/u", filepath.FromSlash(rel))
		if got := get(t, name).SkillsDir(s); got != want {
			t.Errorf("%s SkillsDir(machine) = %q, want %q", name, got, want)
		}
	}
}

func TestSkillsDirMachineEnvOverride(t *testing.T) {
	cases := map[string]string{
		"claude": "CLAUDE_CONFIG_DIR",
		"codex":  "CODEX_HOME",
		"vibe":   "VIBE_HOME",
		"hermes": "HERMES_HOME",
	}
	s := scope.Machine("/home/u", "/home/u/.config")
	for name, env := range cases {
		t.Setenv(env, "/custom/base")
		want := filepath.Join("/custom/base", "skills")
		if got := get(t, name).SkillsDir(s); got != want {
			t.Errorf("%s SkillsDir with %s set = %q, want %q", name, env, got, want)
		}
		t.Setenv(env, "")
	}
}

func TestDetectedMachine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", "")
	s := scope.Machine(home, filepath.Join(home, ".config"))

	codex := get(t, "codex")
	if codex.Detected(s) {
		t.Fatal("codex detected before its base dir exists")
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !codex.Detected(s) {
		t.Error("codex not detected after ~/.codex created")
	}

	// A nested base (antigravity) must key off its own dir, not a bare
	// ~/.gemini that could belong to another tool.
	anti := get(t, "antigravity")
	if anti.Detected(s) {
		t.Fatal("antigravity detected before ~/.gemini/antigravity exists")
	}
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !anti.Detected(s) {
		t.Error("antigravity not detected after ~/.gemini/antigravity created")
	}
}

func TestDetectedMachineEnvOverride(t *testing.T) {
	home := t.TempDir()
	base := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	s := scope.Machine(home, filepath.Join(home, ".config"))
	// ~/.claude absent, but the override points at an existing dir.
	if !get(t, "claude").Detected(s) {
		t.Error("claude not detected via CLAUDE_CONFIG_DIR override")
	}
}

// Link/Unlink is shared behavior; exercise it through one config (claude).
func TestLinkAndUnlink(t *testing.T) {
	root := t.TempDir()
	s := scope.Project(root)
	canonical := s.SkillDir("pdf")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# pdf"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := get(t, "claude")
	if err := a.Link(s, "pdf", canonical, false); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".claude", "skills", "pdf")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("link is not a symlink")
	}
	data, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf" {
		t.Errorf("linked content = %q", data)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("link target %q should be relative so the repo can move", target)
	}

	if err := a.Unlink(s, "pdf", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("link still exists after Unlink")
	}
}

// When the canonical skill at .agents/skills is an editable symlink (its
// directory points at a live worktree), a harness that reads .agents/skills
// directly (codex) must not remove or replace it — not when enabled, and not
// during the tolerant unlink of a disabled harness. Regression: RemoveLinkOrCopy
// would delete the symlink because the link path is the canonical path itself.
func TestEditableCanonicalUntouched(t *testing.T) {
	root := t.TempDir()
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scope.Project(root)
	canonical := s.SkillDir("mine")
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(worktree, canonical); err != nil {
		t.Fatal(err)
	}

	a := get(t, "codex")
	if err := a.Unlink(s, "mine", false); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, err := os.Lstat(canonical); err != nil {
		t.Fatalf("editable canonical removed by Unlink: %v", err)
	}
	if err := a.Link(s, "mine", canonical, false); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(canonical, "SKILL.md")); err != nil || string(data) != "# mine" {
		t.Fatalf("editable canonical broken after Link: data=%q err=%v", data, err)
	}
}

// A harness whose project skills dir is the canonical .agents/skills dir
// itself (codex) must treat linking as an in-place no-op: the skill already
// lives there, and neither Link nor Unlink may disturb the canonical tree.
func TestLinkCanonicalDirIsNoOp(t *testing.T) {
	root := t.TempDir()
	s := scope.Project(root)
	canonical := s.SkillDir("pdf")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# pdf"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := get(t, "codex")
	if a.SkillsDir(s) != s.SkillsDir {
		t.Fatalf("precondition: codex project dir %q != canonical %q", a.SkillsDir(s), s.SkillsDir)
	}
	if err := a.Link(s, "pdf", canonical, false); err != nil {
		t.Fatalf("Link: %v", err)
	}
	fi, err := os.Lstat(canonical)
	if err != nil {
		t.Fatalf("canonical vanished after Link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("canonical was replaced by a symlink")
	}
	if err := a.Unlink(s, "pdf", false); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, err := os.Lstat(canonical); err != nil {
		t.Errorf("canonical removed after Unlink: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(canonical, "SKILL.md")); string(data) != "# pdf" {
		t.Errorf("canonical content = %q, want intact", data)
	}
}
