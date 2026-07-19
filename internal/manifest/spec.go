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
	// TreeURL marks a spec parsed from a pasted github.com/.../tree/<ref>/...
	// browser URL. It is parse-time metadata only (never persisted), used to
	// improve the error when the single-segment ref guess does not resolve.
	TreeURL bool
}

var shorthandRe = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// winPathRe matches a Windows drive-letter path (C:\... or C:/...).
var winPathRe = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

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
	if err := normalizeTreeURL(&s); err != nil {
		return SourceSpec{}, err
	}
	return s, nil
}

// normalizeTreeURL rewrites a pasted github.com browser URL
// (https://github.com/owner/repo/tree/<ref>[/<path>]) into the canonical
// repo URL plus Ref and Path. The ref is assumed to be a single segment:
// a ref containing "/" cannot be split from the path without asking the
// remote, so it fails later at resolve time with a hint (see TreeURL).
func normalizeTreeURL(s *SourceSpec) error {
	const prefix = "https://github.com/"
	if s.IsPath || !strings.HasPrefix(s.Source, prefix) {
		return nil
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(s.Source, prefix), "/"), "/")
	if len(seg) < 4 || seg[2] != "tree" {
		return nil
	}
	if s.Ref != "" {
		return fmt.Errorf("%s already names ref %q in /tree/; an extra @%s is contradictory", s.Source, seg[3], s.Ref)
	}
	if s.Path != "" {
		return fmt.Errorf("%s already contains the path after /tree/<ref>/; an extra //%s is contradictory", s.Source, s.Path)
	}
	s.Source = prefix + seg[0] + "/" + seg[1]
	s.Ref = seg[3]
	s.Path = strings.Join(seg[4:], "/")
	s.TreeURL = true
	return nil
}

// isPathSource reports whether source is a local filesystem path. Relative
// paths must be written with an explicit ./ or ../ prefix.
func isPathSource(source string) bool {
	return strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		source == "." || source == ".." ||
		strings.HasPrefix(source, "~/") || source == "~" ||
		strings.HasPrefix(source, `\\`) || winPathRe.MatchString(source)
}
