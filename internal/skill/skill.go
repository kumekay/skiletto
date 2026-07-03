// Package skill locates skill directories (any directory containing
// SKILL.md), derives default skill names, and hashes installed trees for
// drift detection.
package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Discover walks root and returns the slash-separated paths, relative to
// root, of every directory containing a SKILL.md file, sorted. The root
// itself is reported as ".". Directories named .git are skipped.
func Discover(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(p))
		if err != nil {
			return err
		}
		dirs = append(dirs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	return dirs, nil
}

// DefaultName derives a skill name from its source and subdirectory path:
// the base name of the subdirectory, or of the source when the skill lives
// at the repo root. A trailing .git suffix on the source is dropped.
func DefaultName(source, subpath string) string {
	if subpath != "" && subpath != "." {
		return path.Base(filepath.ToSlash(subpath))
	}
	s := strings.TrimSuffix(strings.TrimRight(filepath.ToSlash(source), "/"), ".git")
	return path.Base(s)
}

// Hash returns a deterministic content hash ("sha256:<hex>") of the tree
// rooted at dir: sha256 over the sorted relative file paths and their
// contents.
func Hash(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "%s\x00", rel)
		_, err = io.Copy(h, f)
		_ = f.Close()
		if err != nil {
			return "", err
		}
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
