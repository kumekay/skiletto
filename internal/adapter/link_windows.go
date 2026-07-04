//go:build windows

package adapter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kumekay/skiletto/internal/skill"
)

// noSymlinkEnv and noJunctionEnv, when set, make the corresponding strategy
// fail so each fallback is exercised deterministically. GitHub's windows
// runners can run elevated and would otherwise always succeed at symlinks,
// hiding the fallbacks the CI canary is meant to cover; setting both forces
// the copy strategy.
const (
	noSymlinkEnv  = "SKILETTO_NO_SYMLINK"
	noJunctionEnv = "SKILETTO_NO_JUNCTION"
)

// linkSteps on Windows is the full chain: symlink (needs Developer Mode),
// then a directory junction (no privilege required), then a copy when copies
// are allowed (pinned installs; editable installs pass allowCopy=false so a
// copy can never silently break liveness).
func linkSteps(allowCopy bool) []linkStep {
	steps := []linkStep{
		{StrategySymlink, trySymlink},
		{StrategyJunction, makeJunction},
	}
	if allowCopy {
		steps = append(steps, linkStep{StrategyCopy, copyTree})
	}
	return steps
}

func trySymlink(link, target string) error {
	if os.Getenv(noSymlinkEnv) != "" {
		return fmt.Errorf("symlink disabled via %s", noSymlinkEnv)
	}
	return os.Symlink(target, link)
}

// makeJunction creates a directory junction at link pointing to target.
// mklink /J needs an absolute target, so a relative target is resolved
// against link's parent directory.
func makeJunction(link, target string) error {
	if os.Getenv(noJunctionEnv) != "" {
		return fmt.Errorf("junction disabled via %s", noJunctionEnv)
	}
	target = absTarget(link, target)
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J %q %q: %w: %s", link, target, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// copyTree recursively copies the canonical tree at target into link.
func copyTree(link, target string) error {
	return copyDir(absTarget(link, target), link)
}

func absTarget(link, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Clean(filepath.Join(filepath.Dir(link), target))
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// reparseLink reports whether fi describes a symlink or any other reparse
// point (directory junctions among them). Since Go 1.23 (winsymlink=1)
// os.Lstat no longer sets ModeSymlink for junctions, so the reparse-point
// file attribute is inspected directly.
func reparseLink(fi os.FileInfo) (bool, error) {
	if fi.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
	}
	return false, nil
}

// platformReclaimDir on Windows reclaims a real directory at a link
// location when forced, or when its contents hash-match the canonical tree
// (proof the directory is skiletto's own copy-link).
func platformReclaimDir(link, canonical string, force bool) bool {
	return force || dirsMatch(link, canonical)
}

// reclaimHint explains how to recover a copy-linked skill that no longer
// matches its canonical tree.
const reclaimHint = " (a copy-linked skill that has diverged counts as a local modification; re-run with --force to replace or delete it)"

// wrapNoLiveLink explains why the no-copy chain exists when it fails: the
// caller needed a live link (an editable install) and a copy would not do.
func wrapNoLiveLink(err error) error {
	return fmt.Errorf("%w (editable skills need a live link: enable Developer Mode for symlinks, or use a filesystem that supports directory junctions; a copy cannot stay live)", err)
}

// dirsMatch reports whether the directory at link has the same content hash
// as canonical (resolved to an absolute path against link's parent when
// relative). It is how a copied link proves it is ours.
func dirsMatch(link, canonical string) bool {
	canonical = absTarget(link, canonical)
	got, err := skill.Hash(link)
	if err != nil {
		return false
	}
	want, err := skill.Hash(canonical)
	if err != nil {
		return false
	}
	return got == want
}

// IsOwnLink reports whether path is a link skiletto created into skillsDir:
// a symlink or junction resolving to the matching canonical skill directory,
// or a copy whose contents match it.
func IsOwnLink(skillsDir, path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	canonical := filepath.Join(skillsDir, filepath.Base(path))
	isLink, _ := reparseLink(fi)
	if isLink {
		// A junction cannot be read with os.Readlink on Go 1.23+, but
		// os.Stat traverses it, so identity against canonical proves it is
		// ours. A genuine symlink is also handled by the readlink fallback.
		if li, err1 := os.Stat(path); err1 == nil {
			if ci, err2 := os.Stat(canonical); err2 == nil && os.SameFile(li, ci) {
				return true
			}
		}
		if target, err := os.Readlink(path); err == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			rel, err := filepath.Rel(skillsDir, filepath.Clean(target))
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	if fi.IsDir() {
		// A real directory sitting directly in the canonical skills dir is a
		// materialized skill (or a stray), never an adapter copy-link.
		if filepath.Clean(filepath.Dir(path)) == filepath.Clean(skillsDir) {
			return false
		}
		return dirsMatch(path, canonical)
	}
	return false
}
