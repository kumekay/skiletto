package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kumekay/skiletto/internal/scope"
)

func TestSymlinkCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	target1 := filepath.Join(dir, "t1")
	target2 := filepath.Join(dir, "t2")
	for _, d := range []string{target1, target2} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "nested", "link")

	if err := Symlink(link, target1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != target1 {
		t.Errorf("link points at %q, want %q", got, target1)
	}

	// Replacing an existing symlink is fine.
	if err := Symlink(link, target2); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != target2 {
		t.Errorf("link points at %q, want %q", got, target2)
	}
}

func TestSymlinkRefusesRealDir(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "existing")
	if err := os.Mkdir(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Symlink(link, dir); err == nil {
		t.Error("want error when link path is a real directory")
	}
}

func TestRemoveLink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := Symlink(link, dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLink(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("link still exists")
	}
	// Missing link is a no-op.
	if err := RemoveLink(link); err != nil {
		t.Fatal(err)
	}
	// A real directory is never removed.
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLink(real); err == nil {
		t.Error("want error when removing a non-symlink")
	}
}

func TestRegistry(t *testing.T) {
	reset := swapRegistry(t)
	defer reset()

	Register(fake{name: "b"})
	Register(fake{name: "a"})
	all := All()
	if len(all) != 2 || all[0].Name() != "a" || all[1].Name() != "b" {
		t.Errorf("All() = %v", all)
	}
}

type fake struct{ name string }

func (f fake) Name() string                                              { return f.name }
func (f fake) SkillsDir(s scope.Scope) string                            { return s.Root }
func (f fake) Link(s scope.Scope, name, target string, force bool) error { return nil }
func (f fake) Unlink(s scope.Scope, name string, force bool) error       { return nil }

func swapRegistry(t *testing.T) func() {
	t.Helper()
	old := registry
	registry = map[string]Adapter{}
	return func() { registry = old }
}
