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
