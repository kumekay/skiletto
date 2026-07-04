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

// platformReclaimDir is always false on unix: skiletto never produces
// copies here, so a real directory at a link path is always foreign and
// never reclaimed — not even with force.
func platformReclaimDir(string, string, bool) bool { return false }

// reclaimHint is empty on unix: refusal messages stay unchanged.
const reclaimHint = ""

// wrapNoLiveLink is the identity on unix: symlinks are the only strategy,
// and their errors are reported verbatim.
func wrapNoLiveLink(err error) error { return err }

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
