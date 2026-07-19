// Package gitcli wraps the system git binary. All exec calls live behind
// this boundary so capability fallbacks (or a future go-git swap) stay
// local to one package.
package gitcli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	versionRe = regexp.MustCompile(`git version (\d+)\.(\d+)`)
	shaRe     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// ErrRefNotFound marks a ref that ls-remote could not find, so callers can
// distinguish a bad ref from transport failures.
var ErrRefNotFound = errors.New("ref not found")

// gitEnvBlocklist names GIT_* variables that pin git to a specific
// repository, object store, or index. Git exports these into hook
// environments (pre-push, pre-commit, ...), so when skiletto is invoked from
// a hook an inherited value would silently retarget our git subprocesses at
// the enclosing repository instead of the temp clone we set up via cmd.Dir.
// We strip exactly the repo-targeting variables; auth and identity variables
// (GIT_SSH*, GIT_ASKPASS, GIT_TERMINAL_PROMPT, GIT_CONFIG_*, GIT_AUTHOR_*,
// GIT_COMMITTER_*) do not retarget the repo, so they are left untouched.
var gitEnvBlocklist = map[string]struct{}{
	"GIT_DIR":                          {},
	"GIT_WORK_TREE":                    {},
	"GIT_INDEX_FILE":                   {},
	"GIT_COMMON_DIR":                   {},
	"GIT_PREFIX":                       {},
	"GIT_OBJECT_DIRECTORY":             {},
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
}

// Environ returns a copy of the current process environment with the
// repo-targeting GIT_* variables (see gitEnvBlocklist) removed, suitable as
// the Env of a git subprocess. It is exported so test fixtures that spawn
// git directly can apply the same scrubbing.
func Environ() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if _, blocked := gitEnvBlocklist[name]; blocked {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Git runs system git commands. Capabilities are detected once in New.
type Git struct {
	version string
	// sparse: git sparse-checkout exists (git >= 2.25).
	sparse bool
	// shaFetch: attempt shallow fetches of exact SHAs before falling back
	// to a full clone.
	shaFetch bool
}

// New locates system git and detects its version and capabilities.
func New() (*Git, error) {
	cmd := exec.Command("git", "version")
	cmd.Env = Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("system git not found: %w", err)
	}
	version := strings.TrimSpace(string(out))
	g := &Git{version: version, shaFetch: true}
	if m := versionRe.FindStringSubmatch(version); m != nil {
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		g.sparse = major > 2 || major == 2 && minor >= 25
	}
	return g, nil
}

// Version returns the detected `git version` string.
func (g *Git) Version() string {
	return g.version
}

// run executes git with args and returns stdout.
func (g *Git) run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = Environ()
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return string(out), nil
}

// ResolveRemote resolves ref against the repository at url (any URL or
// path git can reach) to a full commit SHA using ls-remote. An empty ref
// means the remote's default branch (HEAD). A 40-hex ref that matches no
// remote ref is returned as-is.
func (g *Git) ResolveRemote(url, ref string) (string, error) {
	if ref == "" {
		out, err := g.run("", "ls-remote", url, "HEAD")
		if err != nil {
			return "", err
		}
		if sha := firstSHA(out); sha != "" {
			return sha, nil
		}
		return "", fmt.Errorf("no HEAD found at %s", url)
	}

	out, err := g.run("", "ls-remote", url, ref, ref+"^{}")
	if err != nil {
		return "", err
	}
	byRef := map[string]string{}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			byRef[parts[1]] = parts[0]
			lines = append(lines, parts[1])
		}
	}
	// Prefer the peeled tag (the commit an annotated tag points at),
	// then branch, then the tag object itself.
	for _, name := range []string{
		"refs/tags/" + ref + "^{}",
		"refs/heads/" + ref,
		"refs/tags/" + ref,
	} {
		if sha, ok := byRef[name]; ok {
			return sha, nil
		}
	}
	if len(lines) > 0 {
		return byRef[lines[0]], nil
	}
	if shaRe.MatchString(ref) {
		return ref, nil
	}
	return "", fmt.Errorf("%w: %q at %s", ErrRefNotFound, ref, url)
}

// ResolveLocal resolves ref (default HEAD) to a full commit SHA against a
// local repository working tree.
func (g *Git) ResolveLocal(repo, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := g.run(repo, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Extract materializes the content of subdir (repo root when empty) at
// commit from the repository at url into dest. It tries a shallow fetch of
// the exact SHA with a sparse checkout, and falls back to a full clone for
// servers or gits that do not support that.
func (g *Git) Extract(url, commit, subdir, dest string) error {
	tmp, err := os.MkdirTemp("", "skiletto-git-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if _, err := g.run(tmp, "init", "-q"); err != nil {
		return err
	}
	if _, err := g.run(tmp, "remote", "add", "origin", url); err != nil {
		return err
	}
	if g.sparse && subdir != "" {
		if _, err := g.run(tmp, "sparse-checkout", "set", subdir); err != nil {
			return err
		}
	}

	fetched := false
	if g.shaFetch {
		// Shallow fetch of the exact commit. Needs
		// uploadpack.allowAnySHA1InWant server-side; -c propagates it to
		// local upload-packs so path remotes work too.
		_, err := g.run(tmp, "-c", "uploadpack.allowAnySHA1InWant=true",
			"fetch", "-q", "--depth", "1", "origin", commit)
		fetched = err == nil
	}
	if !fetched {
		// Full-clone fallback: fetch everything, then check the commit out.
		if _, err := g.run(tmp, "fetch", "-q", "--tags", "origin"); err != nil {
			return err
		}
	}
	if _, err := g.run(tmp, "checkout", "-q", commit); err != nil {
		return err
	}

	src := tmp
	if subdir != "" {
		src = filepath.Join(tmp, filepath.FromSlash(subdir))
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("path %q not found in %s at %s", subdir, url, commit)
		}
	}
	return copyTree(src, dest)
}

// firstSHA returns the SHA of the first ls-remote output line.
func firstSHA(out string) string {
	fields := strings.Fields(out)
	if len(fields) > 0 && shaRe.MatchString(fields[0]) {
		return fields[0]
	}
	return ""
}

// copyTree copies the directory tree at src to dest (which must not
// exist), skipping any .git directory. Symlinks are recreated as symlinks
// (never followed): a link may point at a directory or even dangle, and
// dereferencing would either fail or silently duplicate content.
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.Type()&fs.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(p, target, info.Mode())
	})
}

func copyFile(src, dest string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
