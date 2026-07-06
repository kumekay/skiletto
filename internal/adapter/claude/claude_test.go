package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kumekay/skiletto/internal/scope"
)

func TestSkillsDirProject(t *testing.T) {
	a := New()
	s := scope.Project("/repo")
	want := filepath.Join("/repo", ".claude", "skills")
	if got := a.SkillsDir(s); got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
}

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

	a := New()
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
	// The link must resolve to the canonical skill content.
	data, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf" {
		t.Errorf("linked content = %q", data)
	}

	if err := a.Unlink(s, "pdf", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("link still exists after Unlink")
	}
}

// When .claude/skills is itself a symlink onto the canonical .agents/skills
// dir, the per-skill link path resolves straight to the canonical tree.
// Link and Unlink must both treat that as already-in-place and leave the
// canonical skill directory untouched.
func TestLinkAliasedSkillsDir(t *testing.T) {
	root := t.TempDir()
	s := scope.Project(root)
	canonical := s.SkillDir("pdf")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .claude/skills -> ../.agents/skills
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", ".agents", "skills"), filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}

	a := New()
	if err := a.Link(s, "pdf", canonical, false); err != nil {
		t.Fatalf("Link aliased: %v", err)
	}
	fi, err := os.Lstat(canonical)
	if err != nil {
		t.Fatalf("canonical vanished after Link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("canonical was replaced by a symlink")
	}
	if data, _ := os.ReadFile(filepath.Join(canonical, "SKILL.md")); string(data) != "# pdf" {
		t.Errorf("canonical content = %q, want intact", data)
	}

	if err := a.Unlink(s, "pdf", false); err != nil {
		t.Fatalf("Unlink aliased: %v", err)
	}
	if _, err := os.Lstat(canonical); err != nil {
		t.Errorf("canonical removed after Unlink: %v", err)
	}
}

// Regression: a foreign real directory at the per-skill path (no parent-alias
// illusion) is still refused by both Link and Unlink.
func TestLinkForeignDirStillRefused(t *testing.T) {
	root := t.TempDir()
	s := scope.Project(root)
	canonical := s.SkillDir("pdf")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".claude", "skills", "pdf")
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(link, "SKILL.md"), []byte("# foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	if err := a.Link(s, "pdf", canonical, false); err == nil {
		t.Error("Link should refuse a foreign real directory")
	}
	if err := a.Unlink(s, "pdf", false); err == nil {
		t.Error("Unlink should refuse a foreign real directory")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("foreign directory removed: %v", err)
	}
}

func TestLinkIsRelativeWithinRepo(t *testing.T) {
	root := t.TempDir()
	s := scope.Project(root)
	canonical := s.SkillDir("pdf")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	a := New()
	if err := a.Link(s, "pdf", canonical, false); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(root, ".claude", "skills", "pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("link target %q should be relative so the repo can move", target)
	}
}
