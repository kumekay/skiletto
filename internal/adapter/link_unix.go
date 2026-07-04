//go:build !windows

package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

// linkSteps on unix is symlink only: no junction, no copy. The allowCopy
// flag is irrelevant here, keeping unix behavior byte-for-byte identical to
// the original os.Symlink path.
func linkSteps(bool) []linkStep {
	return []linkStep{{StrategySymlink, func(link, target string) error {
		return os.Symlink(target, link)
	}}}
}

// reparseLink on unix treats only symlinks as links.
func reparseLink(fi os.FileInfo) (bool, error) {
	return fi.Mode()&os.ModeSymlink != 0, nil
}

// ourCopy is always false on unix: skiletto never produces copies here, so a
// real directory at a link path is always foreign and never reclaimed.
func ourCopy(string, string) bool { return false }

// IsOwnLink reports whether path is a symlink skiletto created: one that
// points into skillsDir. This is the original list.ownLink logic, unchanged.
func IsOwnLink(skillsDir, path string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false // not a symlink
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	rel, err := filepath.Rel(skillsDir, filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
