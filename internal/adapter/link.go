package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LinkStrategy names how a skill directory is materialized into a location.
// It is never stored in the lockfile: symlinks and directory junctions
// self-identify as reparse points, and a copy is recognized as ours when its
// contents hash equal to the canonical tree. The strategy is derived at
// inspection time so a lockfile stays portable across machines.
type LinkStrategy string

// Link strategies, in fallback order.
const (
	// StrategySymlink is a filesystem symlink (needs Developer Mode on Windows).
	StrategySymlink LinkStrategy = "symlink"
	// StrategyJunction is a Windows directory junction (no privilege required).
	StrategyJunction LinkStrategy = "junction"
	// StrategyCopy is a plain recursive copy, the last resort on Windows.
	StrategyCopy LinkStrategy = "copy"
)

// linkStep is one strategy in the platform fallback chain.
type linkStep struct {
	strategy LinkStrategy
	link     func(link, target string) error
}

// reclaimDir decides whether a real directory occupying a link location may
// be removed (and replaced or deleted). Platform-specific: never on unix,
// where skiletto produces no copies; on Windows when forced or when the
// contents hash-match the canonical tree, which is proof the copy is ours.
// A variable so the Windows rule is testable on any OS.
var reclaimDir = platformReclaimDir

// runLinkChain tries steps in order and returns the strategy of the first
// that succeeds. When only one strategy exists (the unix chain, symlink
// only) its error is returned verbatim, so the unix path is
// indistinguishable from a direct os.Symlink; with several, every cause is
// joined so a Windows failure is diagnosable.
func runLinkChain(link, target string, steps []linkStep) (LinkStrategy, error) {
	var errs []error
	for _, s := range steps {
		err := s.link(link, target)
		if err == nil {
			return s.strategy, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", s.strategy, err))
	}
	if len(errs) == 1 {
		return "", errors.Unwrap(errs[0])
	}
	return "", fmt.Errorf("no link strategy succeeded: %w", errors.Join(errs...))
}

// Symlink creates a link at link pointing to target, creating parent
// directories as needed, using the platform fallback chain that stops
// before copying (a symlink, then a directory junction on Windows). Editable
// installs rely on it because a copy cannot stay live; on a platform where
// only copying would work the failure carries that explanation. It refuses
// to replace anything that is not one of our links.
func Symlink(link, target string) error {
	if _, err := createLink(link, target, false, false); err != nil {
		return wrapNoLiveLink(err)
	}
	return nil
}

// LinkDir creates a link at link pointing to target using the full platform
// fallback chain (a symlink, a directory junction, then a copy as a last
// resort on Windows) and reports the strategy used. force additionally
// replaces a copy at link that has diverged from the canonical tree.
func LinkDir(link, target string, force bool) (LinkStrategy, error) {
	return createLink(link, target, true, force)
}

func createLink(link, target string, allowCopy, force bool) (LinkStrategy, error) {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return "", err
	}
	if err := clearExisting(link, target, allowCopy, force); err != nil {
		return "", err
	}
	return runLinkChain(link, target, linkSteps(allowCopy))
}

// clearExisting removes whatever already sits at link if it is provably
// ours: a symlink or junction, or — only on the copy-capable path — a real
// directory the platform reclaim rule accepts (a copy matching target, the
// canonical tree, or any real directory under force on Windows). Anything
// else yields NotASymlinkError and is never touched. A missing entry is
// fine.
func clearExisting(link, target string, allowCopy, force bool) error {
	fi, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	isLink, err := reparseLink(fi)
	if err != nil {
		return err
	}
	if isLink {
		return os.Remove(link)
	}
	if allowCopy && fi.IsDir() && reclaimDir(link, target, force) {
		return os.RemoveAll(link)
	}
	return &NotASymlinkError{Path: link, Hint: reclaimHint}
}

// RemoveLink removes a link (a symlink or, on Windows, a directory junction)
// at link. A missing link is a no-op; a real directory is refused.
func RemoveLink(link string) error {
	fi, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	isLink, err := reparseLink(fi)
	if err != nil {
		return err
	}
	if !isLink {
		return fmt.Errorf("%s is not a symlink; refusing to remove it", link)
	}
	return os.Remove(link)
}

// RemoveLinkOrCopy removes the link at link, or, when link is a real
// directory the platform reclaim rule accepts (a copy matching canonical,
// or any real directory under force on Windows), the copied directory.
// Anything else is refused. A missing link is a no-op.
func RemoveLinkOrCopy(link, canonical string, force bool) error {
	fi, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	isLink, err := reparseLink(fi)
	if err != nil {
		return err
	}
	if isLink {
		return os.Remove(link)
	}
	if fi.IsDir() && reclaimDir(link, canonical, force) {
		return os.RemoveAll(link)
	}
	return &NotOurLinkError{Path: link, Hint: reclaimHint}
}

// IsLink reports whether path is one of skiletto's links: a symlink or, on
// Windows, a directory junction. It follows nothing and never treats a
// materialized copy (a real directory) as a link.
func IsLink(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return reparseLink(fi)
}
