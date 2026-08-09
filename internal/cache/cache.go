// Package cache resolves the user-wide cache directory that holds cloned
// git repositories, so skill sources are reused across installs and
// projects instead of re-cloning from the network every time. The cache is
// not tied to any single skiletto.toml; it is safe to delete entirely.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// Dir resolves the cache root: SKILETTO_CACHE_DIR when set, otherwise
// <platform cache dir>/skiletto ($XDG_CACHE_HOME/skiletto on Linux,
// ~/Library/Caches/skiletto on macOS, %LocalAppData%\skiletto on Windows).
func Dir(getenv func(string) string, userCacheDir func() (string, error)) (string, error) {
	if env := getenv("SKILETTO_CACHE_DIR"); env != "" {
		return env, nil
	}
	base, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "skiletto"), nil
}

// RepoDir returns the cache directory for one canonical source URL.
// Equivalent spellings of the same repository (scheme, trailing .git or /,
// scp-style vs URL form, userinfo) map to the same directory; different
// repositories never collide, guaranteed by a short hash of the normalized
// URL in the leaf name.
func RepoDir(root, url string) string {
	host, segments := splitURL(url)
	normalized := strings.ToLower(host) + "/" + strings.Join(segments, "/")
	sum := sha256.Sum256([]byte(normalized))
	short := hex.EncodeToString(sum[:4])

	name := "repo"
	if len(segments) > 0 {
		name = segments[len(segments)-1]
	} else if host != "" {
		name = host
	}

	parts := make([]string, 0, len(segments)+2)
	parts = append(parts, root)
	if host != "" && len(segments) > 0 {
		parts = append(parts, sanitize(host))
		for _, segment := range segments[:len(segments)-1] {
			parts = append(parts, sanitize(segment))
		}
	}
	parts = append(parts, sanitize(name)+"-"+short)
	return filepath.Join(parts...)
}

// splitURL splits a repository URL into host and path segments, ignoring
// scheme and userinfo and stripping a trailing .git suffix, so that
// https/http/ssh spellings of one repository normalize identically.
func splitURL(url string) (host string, segments []string) {
	rest := strings.TrimSpace(url)
	pathPart := ""
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			if slash := strings.Index(rest, "/"); slash < 0 || at < slash {
				rest = rest[at+1:]
			}
		}
		host, pathPart, _ = strings.Cut(rest, "/")
	} else if i := strings.Index(rest, ":"); i > 0 && !strings.Contains(rest[:i], "/") {
		// scp-style user@host:path.
		host, pathPart, _ = strings.Cut(rest, ":")
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
	} else {
		host, pathPart, _ = strings.Cut(rest, "/")
	}
	for _, segment := range strings.Split(pathPart, "/") {
		if segment == "" {
			continue
		}
		segments = append(segments, segment)
	}
	if len(segments) > 0 {
		segments[len(segments)-1] = strings.TrimSuffix(segments[len(segments)-1], ".git")
	}
	return host, segments
}

// sanitize maps a path component onto a conservative filesystem-safe
// character set.
func sanitize(s string) string {
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
