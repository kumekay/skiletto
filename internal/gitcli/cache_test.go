package gitcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kumekay/skiletto/internal/cache"
)

// The first extraction populates the cache, and later extractions of the
// same commit are served from it: once the remote is gone, the extract
// must still succeed — no network involved.
func TestExtractReusesCacheWithoutNetwork(t *testing.T) {
	repo, _, tip := makeRepo(t)
	root := t.TempDir()
	g, _ := New()
	g.Cache = root

	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "skills/pdf", dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache.RepoDir(root, repo)); err != nil {
		t.Fatalf("cache repo was not created: %v", err)
	}

	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	dest2 := filepath.Join(t.TempDir(), "out2")
	if err := g.Extract(repo, tip, "skills/pdf", dest2); err != nil {
		t.Fatalf("extract from cache failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest2, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf v2" {
		t.Errorf("content = %q, want %q", data, "# pdf v2")
	}
}

// Whole-repo and subdir extractions alike must be servable from a cache
// entry populated by a different extraction shape.
func TestExtractCacheAcrossShapes(t *testing.T) {
	repo, _, tip := makeRepo(t)
	g, _ := New()
	g.Cache = t.TempDir()

	if err := g.Extract(repo, tip, "skills/pdf", filepath.Join(t.TempDir(), "a")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "whole")
	if err := g.Extract(repo, tip, "", dest); err != nil {
		t.Fatalf("whole-repo extract from subdir-populated cache failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Errorf("missing README.md: %v", err)
	}
}

// A commit that is not in the cache yet is fetched into the existing cache
// repo instead of failing or bypassing the cache.
func TestExtractFetchesMissingCommitIntoCache(t *testing.T) {
	repo, old, tip := makeRepo(t)
	root := t.TempDir()
	g, _ := New()
	g.Cache = root

	if err := g.Extract(repo, old, "skills/pdf", filepath.Join(t.TempDir(), "a")); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "b")
	if err := g.Extract(repo, tip, "skills/pdf", dest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if string(data) != "# pdf v2" {
		t.Errorf("content = %q, want %q", data, "# pdf v2")
	}

	// Both commits are now cached: the remote can disappear.
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	for commit, want := range map[string]string{old: "# pdf v1", tip: "# pdf v2"} {
		out := filepath.Join(t.TempDir(), commit[:8])
		if err := g.Extract(repo, commit, "skills/pdf", out); err != nil {
			t.Fatalf("extract %s from cache: %v", commit[:8], err)
		}
		data, _ := os.ReadFile(filepath.Join(out, "SKILL.md"))
		if string(data) != want {
			t.Errorf("%s: content = %q, want %q", commit[:8], data, want)
		}
	}
}

// A cache entry that is not a repository at all is rebuilt, never fatal.
func TestExtractRecoversFromForeignCacheEntry(t *testing.T) {
	repo, _, tip := makeRepo(t)
	root := t.TempDir()
	g, _ := New()
	g.Cache = root

	repoDir := cache.RepoDir(root, repo)
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoDir, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "skills/pdf", dest); err != nil {
		t.Fatalf("foreign cache entry broke extract: %v", err)
	}
	if fi, err := os.Stat(repoDir); err != nil || !fi.IsDir() {
		t.Errorf("cache entry was not rebuilt as a directory: %v", err)
	}
}

// A cache entry whose .git is garbage is rebuilt, never fatal.
func TestExtractRecoversFromBrokenGitDir(t *testing.T) {
	repo, _, tip := makeRepo(t)
	root := t.TempDir()
	g, _ := New()
	g.Cache = root

	repoDir := cache.RepoDir(root, repo)
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "skills/pdf", dest); err != nil {
		t.Fatalf("broken .git broke extract: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if string(data) != "# pdf v2" {
		t.Errorf("content = %q, want %q", data, "# pdf v2")
	}
}

// Without a cache dir the behavior is unchanged: nothing is created under
// a cache root and extraction still works.
func TestExtractWithoutCache(t *testing.T) {
	repo, _, tip := makeRepo(t)
	g, _ := New()
	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "skills/pdf", dest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if string(data) != "# pdf v2" {
		t.Errorf("content = %q, want %q", data, "# pdf v2")
	}
}
