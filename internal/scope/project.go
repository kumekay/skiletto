package scope

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindProjectRoot walks from start toward the filesystem root and returns the
// nearest directory containing skiletto.toml. stop is excluded from the search
// so a manifest in the home directory can never become a project manifest.
func FindProjectRoot(start, stop string) (string, bool, error) {
	start = filepath.Clean(start)
	root := start
	info, err := os.Stat(root)
	if err != nil {
		return root, false, fmt.Errorf("resolving project root: %w", err)
	}
	if !info.IsDir() {
		return root, false, fmt.Errorf("resolving project root: %s is not a directory", root)
	}
	for {
		if sameDir(root, stop) {
			return start, false, nil
		}
		_, err := os.Stat(filepath.Join(root, "skiletto.toml"))
		if err == nil {
			return root, true, nil
		}
		if !os.IsNotExist(err) {
			return start, false, fmt.Errorf("resolving project root: %w", err)
		}
		parent := filepath.Dir(root)
		if parent == root {
			return start, false, nil
		}
		root = parent
	}
}

func sameDir(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}
