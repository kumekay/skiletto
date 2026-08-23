package scope

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindProjectRootWalksUpToNearestManifest(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "src", "project")
	nested := filepath.Join(project, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "skiletto.toml"), []byte("[skills]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, found, err := FindProjectRoot(nested, home)
	if err != nil {
		t.Fatal(err)
	}
	if !found || root != project {
		t.Errorf("FindProjectRoot() = %q, %v; want %q, true", root, found, project)
	}
}

func TestFindProjectRootStopsBeforeHome(t *testing.T) {
	home := t.TempDir()
	nested := filepath.Join(home, "work", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skiletto.toml"), []byte("[skills]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, found, err := FindProjectRoot(nested, home)
	if err != nil {
		t.Fatal(err)
	}
	if found || root != nested {
		t.Errorf("FindProjectRoot() = %q, %v; want %q, false", root, found, nested)
	}
}

func TestFindProjectRootWrapsStatErrors(t *testing.T) {
	start := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(start, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := FindProjectRoot(start, t.TempDir())
	if err == nil {
		t.Fatal("FindProjectRoot() succeeded for a non-directory start")
	}
	if !strings.Contains(err.Error(), "resolving project root") {
		t.Errorf("error lacks project-root context: %v", err)
	}
}
