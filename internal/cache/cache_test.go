package cache

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirEnvOverride(t *testing.T) {
	dir, err := Dir(func(name string) string {
		if name == "SKILETTO_CACHE_DIR" {
			return "/custom/cache"
		}
		return ""
	}, func() (string, error) {
		t.Fatal("platform cache dir must not be consulted when SKILETTO_CACHE_DIR is set")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/custom/cache" {
		t.Errorf("Dir = %q, want /custom/cache", dir)
	}
}

func TestDirPlatformDefault(t *testing.T) {
	dir, err := Dir(func(string) string { return "" }, func() (string, error) {
		return "/platform/cache", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/platform/cache", "skiletto"); dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
}

func TestDirPlatformError(t *testing.T) {
	boom := errors.New("no home")
	_, err := Dir(func(string) string { return "" }, func() (string, error) {
		return "", boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("Dir error = %v, want %v", err, boom)
	}
}

// The same repository reached through different spellings of its URL must
// share one cache entry; different repositories must not.
func TestRepoDirEquivalentURLsShareEntry(t *testing.T) {
	root := "/cache"
	spellings := []string{
		"https://github.com/kumekay/skiletto",
		"https://github.com/kumekay/skiletto/",
		"https://github.com/kumekay/skiletto.git",
		"http://github.com/kumekay/skiletto",
		"git@github.com:kumekay/skiletto.git",
		"ssh://git@github.com/kumekay/skiletto.git",
	}
	first := RepoDir(root, spellings[0])
	for _, url := range spellings[1:] {
		if got := RepoDir(root, url); got != first {
			t.Errorf("RepoDir(%q) = %q, want %q", url, got, first)
		}
	}

	other := RepoDir(root, "https://github.com/kumekay/other")
	if other == first {
		t.Errorf("different repos share cache entry %q", first)
	}
	otherHost := RepoDir(root, "https://example.com/kumekay/skiletto")
	if otherHost == first {
		t.Errorf("different hosts share cache entry %q", first)
	}
}

// Cache entry names must be filesystem-safe: only a conservative character
// set may appear in any path component under the root.
func TestRepoDirSafeName(t *testing.T) {
	urls := []string{
		"https://github.com/kumekay/skiletto",
		"https://user:token@my-host.example.com:8443/owner/repo.git",
		"git@weird.host:deeply/nested/repo.git",
		"https://github.com/owner/repo with spaces",
	}
	for _, url := range urls {
		dir := RepoDir("/cache", url)
		rel, err := filepath.Rel("/cache", dir)
		if err != nil {
			t.Fatalf("RepoDir(%q) escapes the root: %v", url, err)
		}
		for _, comp := range strings.Split(rel, string(filepath.Separator)) {
			if comp == "" {
				t.Errorf("RepoDir(%q) has an empty path component", url)
				continue
			}
			for _, r := range comp {
				ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
					r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
				if !ok {
					t.Errorf("RepoDir(%q): component %q contains unsafe rune %q", url, comp, r)
				}
			}
		}
	}
}

// RepoDir must stay under the cache root.
func TestRepoDirUnderRoot(t *testing.T) {
	root := t.TempDir()
	url := "https://github.com/kumekay/skiletto"
	rel, err := filepath.Rel(root, RepoDir(root, url))
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("RepoDir escapes the cache root: %v", err)
	}
}
