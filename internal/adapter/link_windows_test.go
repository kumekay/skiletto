//go:build windows

package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

// mkSkill creates a directory with a SKILL.md whose body is content.
func mkSkill(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestJunctionRoundTrip exercises the whole junction machinery: creation via
// the fallback chain with symlinks disabled, detection as one of our links,
// ownership recognition, and removal that never touches the target.
func TestJunctionRoundTrip(t *testing.T) {
	t.Setenv(noSymlinkEnv, "1") // force symlink failure -> junction path

	root := t.TempDir()
	canonical := filepath.Join(root, ".agents", "skills", "demo")
	mkSkill(t, canonical, "# demo")
	link := filepath.Join(root, ".claude", "skills", "demo")

	strategy, err := LinkDir(link, canonical)
	if err != nil {
		t.Fatalf("LinkDir: %v", err)
	}
	if strategy != StrategyJunction {
		t.Fatalf("strategy = %q, want junction (symlink disabled)", strategy)
	}

	// The junction resolves to the canonical content.
	data, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil || string(data) != "# demo" {
		t.Fatalf("read through junction = %q, %v", data, err)
	}

	// It is recognized as a link and as ours.
	if ok, err := IsLink(link); err != nil || !ok {
		t.Errorf("IsLink(junction) = %v, %v; want true", ok, err)
	}
	if !IsOwnLink(filepath.Dir(canonical), link) {
		t.Error("IsOwnLink did not recognize the junction")
	}

	// Removing the junction must not delete the canonical tree behind it.
	if err := RemoveLinkOrCopy(link, canonical); err != nil {
		t.Fatalf("RemoveLinkOrCopy: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("junction still present after removal")
	}
	if _, err := os.Stat(filepath.Join(canonical, "SKILL.md")); err != nil {
		t.Errorf("canonical tree was damaged by junction removal: %v", err)
	}
}

// TestJunctionReLinkIsIdempotent mirrors a second `sync`: re-linking over an
// existing junction must succeed, not trip the not-a-symlink guard.
func TestJunctionReLinkIsIdempotent(t *testing.T) {
	t.Setenv(noSymlinkEnv, "1")

	root := t.TempDir()
	canonical := filepath.Join(root, ".agents", "skills", "demo")
	mkSkill(t, canonical, "# demo")
	link := filepath.Join(root, ".claude", "skills", "demo")

	if _, err := LinkDir(link, canonical); err != nil {
		t.Fatalf("first LinkDir: %v", err)
	}
	if s, err := LinkDir(link, canonical); err != nil || s != StrategyJunction {
		t.Fatalf("second LinkDir = %q, %v; want junction, nil", s, err)
	}
}

// TestEditableSymlinkFallsBackToJunction covers editable installs: the
// no-copy Symlink helper must still produce a working junction on Windows
// when symlinks are unavailable.
func TestEditableSymlinkFallsBackToJunction(t *testing.T) {
	t.Setenv(noSymlinkEnv, "1")

	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	mkSkill(t, worktree, "# live")
	canonical := filepath.Join(root, ".agents", "skills", "demo")

	if err := Symlink(canonical, worktree); err != nil {
		t.Fatalf("Symlink (editable): %v", err)
	}
	if ok, _ := IsLink(canonical); !ok {
		t.Error("editable canonical is not detected as a link")
	}
	// Edits to the worktree are visible through the junction (liveness).
	if err := os.WriteFile(filepath.Join(worktree, "SKILL.md"), []byte("# edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(canonical, "SKILL.md"))
	if err != nil || string(data) != "# edited" {
		t.Errorf("edit not live through junction: %q, %v", data, err)
	}
}

// TestCopyStrategy covers the last-resort copy: recognized as ours by
// content hash, removable, and refused once it diverges.
func TestCopyStrategy(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, ".agents", "skills", "demo")
	mkSkill(t, canonical, "# demo")
	link := filepath.Join(root, ".claude", "skills", "demo")

	if err := copyTree(link, canonical); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(link, "SKILL.md")); err != nil || string(data) != "# demo" {
		t.Fatalf("copy content = %q, %v", data, err)
	}
	// A copy is a real directory, not a reparse point.
	if ok, _ := IsLink(link); ok {
		t.Error("copy should not be reported as a link")
	}
	// But it is recognized as ours (matches canonical) and removable.
	if !IsOwnLink(filepath.Dir(canonical), link) {
		t.Error("matching copy not recognized as our own link")
	}
	if err := RemoveLinkOrCopy(link, canonical); err != nil {
		t.Fatalf("RemoveLinkOrCopy(copy): %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("copy not removed")
	}

	// A diverged copy is neither ours nor removable.
	if err := copyTree(link, canonical); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(link, "SKILL.md"), []byte("# user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsOwnLink(filepath.Dir(canonical), link) {
		t.Error("diverged copy wrongly recognized as ours")
	}
	if err := RemoveLinkOrCopy(link, canonical); err == nil {
		t.Error("diverged copy should be refused")
	}
}

// TestForeignDirRefused keeps the sacred rule: a real directory that is not
// our copy is never replaced or removed.
func TestForeignDirRefused(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, ".agents", "skills", "demo")
	mkSkill(t, canonical, "# demo")
	link := filepath.Join(root, ".claude", "skills", "demo")
	mkSkill(t, link, "# someone else's skill")

	if _, err := LinkDir(link, canonical); err == nil {
		t.Error("LinkDir should refuse to overwrite a foreign directory")
	}
	if err := RemoveLinkOrCopy(link, canonical); err == nil {
		t.Error("RemoveLinkOrCopy should refuse a foreign directory")
	}
}
