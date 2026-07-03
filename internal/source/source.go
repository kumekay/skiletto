// Package source defines the Source extension point: something that can
// resolve a ref to a commit and fetch a subtree at that commit. v1 ships
// git URLs and local git paths, both backed by system git.
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kumekay/skiletto/internal/gitcli"
)

// Source resolves refs and fetches content for one skill source.
type Source interface {
	// Resolve turns ref (empty means the default branch) into a full
	// commit SHA.
	Resolve(ref string) (string, error)
	// Fetch materializes the content of subpath (repo root when empty) at
	// commit into the dest directory.
	Fetch(commit, subpath, dest string) error
}

// New returns the Source implementation for a canonical source string: a
// local git path when it looks like a filesystem path, a git URL
// otherwise.
func New(g *gitcli.Git, src string) Source {
	if IsLocalPath(src) {
		return &Path{git: g, root: ExpandHome(src)}
	}
	return &Git{git: g, url: src}
}

// Git fetches from any URL system git can clone.
type Git struct {
	git *gitcli.Git
	url string
}

// Resolve resolves ref via ls-remote against the remote.
func (s *Git) Resolve(ref string) (string, error) {
	return s.git.ResolveRemote(s.url, ref)
}

// Fetch extracts subpath at commit into dest.
func (s *Git) Fetch(commit, subpath, dest string) error {
	return s.git.Extract(s.url, commit, subpath, dest)
}

// Path fetches from a local git repository (a pinned, non-editable path
// source).
type Path struct {
	git  *gitcli.Git
	root string
}

// Resolve resolves ref against the local clone.
func (s *Path) Resolve(ref string) (string, error) {
	sha, err := s.git.ResolveLocal(s.root, ref)
	if err != nil {
		return "", fmt.Errorf("path source %s must be a git repository (or use --editable): %w", s.root, err)
	}
	return sha, nil
}

// Fetch extracts subpath at commit from the local clone into dest.
func (s *Path) Fetch(commit, subpath, dest string) error {
	return s.git.Extract(s.root, commit, subpath, dest)
}

var scpLikeRe = regexp.MustCompile(`^[^/@]+@[^/:]+:`)

// IsLocalPath reports whether a canonical source string refers to the
// local filesystem rather than a remote repository.
func IsLocalPath(src string) bool {
	if strings.Contains(src, "://") || scpLikeRe.MatchString(src) {
		return false
	}
	return strings.HasPrefix(src, "/") ||
		strings.HasPrefix(src, "./") ||
		strings.HasPrefix(src, "../") ||
		src == "." || src == ".." ||
		strings.HasPrefix(src, "~/") || src == "~"
}

// ExpandHome expands a leading ~ to the user's home directory.
func ExpandHome(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
