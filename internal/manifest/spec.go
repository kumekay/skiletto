package manifest

import (
	"fmt"
	"regexp"
	"strings"
)

// SourceSpec is a parsed CLI source argument: <repo>[//subdir][@ref].
// Source is the canonical form written to files: shorthands like
// owner/repo are already expanded, and IsPath marks local filesystem
// sources.
type SourceSpec struct {
	Source string
	Path   string
	Ref    string
	IsPath bool
}

var shorthandRe = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// ParseSourceSpec parses a CLI source spec, splitting off the //subdir and
// @ref parts and expanding the owner/repo shorthand to a full GitHub URL.
func ParseSourceSpec(spec string) (SourceSpec, error) {
	if spec == "" {
		return SourceSpec{}, fmt.Errorf("empty source spec")
	}

	scheme := ""
	rest := spec
	if i := strings.Index(rest, "://"); i >= 0 {
		scheme, rest = rest[:i+3], rest[i+3:]
	}

	// A ref separator is the last "@" that comes after the first "/"
	// (an earlier "@" belongs to the user info of ssh/scp-style sources).
	ref := ""
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		slash := strings.Index(rest, "/")
		if slash >= 0 && at > slash || slash < 0 && !strings.Contains(rest, ":") {
			ref = rest[at+1:]
			rest = rest[:at]
		}
	}

	subdir := ""
	if i := strings.Index(rest, "//"); i >= 0 {
		subdir = strings.Trim(rest[i+2:], "/")
		rest = rest[:i]
	}

	source := scheme + rest
	if source == "" {
		return SourceSpec{}, fmt.Errorf("source spec %q has no repository", spec)
	}

	s := SourceSpec{Path: subdir, Ref: ref}
	switch {
	case isPathSource(source):
		s.IsPath = true
		s.Source = source
	case scheme == "" && shorthandRe.MatchString(source):
		s.Source = "https://github.com/" + source
	case scheme == "" && !strings.Contains(source, "@") && !strings.Contains(strings.SplitN(source, "/", 2)[0], ":"):
		// Host-based shorthand like github.com/owner/repo.
		s.Source = "https://" + source
	default:
		s.Source = source
	}
	return s, nil
}

// isPathSource reports whether source is a local filesystem path. Relative
// paths must be written with an explicit ./ or ../ prefix.
func isPathSource(source string) bool {
	return strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		source == "." || source == ".." ||
		strings.HasPrefix(source, "~/") || source == "~"
}
