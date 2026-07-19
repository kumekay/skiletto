package gitcli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// gitT runs git with test-safe config in dir and fails the test on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{
		"-c", "init.defaultBranch=main",
		"-c", "user.name=test",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
	}
	if dir != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Env = Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// makeRepo creates a git repo with two skills and two commits and returns
// its path plus the first (old) and second (tip) commit SHAs.
func makeRepo(t *testing.T) (repo, oldSHA, tipSHA string) {
	t.Helper()
	repo = t.TempDir()
	gitT(t, "", "init", "-q", repo)
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/pdf/SKILL.md", "# pdf v1")
	write("skills/web/SKILL.md", "# web v1")
	write("README.md", "readme")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "first")
	oldSHA = gitT(t, repo, "rev-parse", "HEAD")

	write("skills/pdf/SKILL.md", "# pdf v2")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "second")
	gitT(t, repo, "tag", "v1.0")
	tipSHA = gitT(t, repo, "rev-parse", "HEAD")
	return repo, oldSHA, tipSHA
}

// TestRunIgnoresInheritedGitDir proves the production exec path scrubs an
// inherited GIT_DIR (as exported by git hooks). With it set to a decoy repo,
// ResolveLocal must still target its intended repo, not the decoy.
func TestRunIgnoresInheritedGitDir(t *testing.T) {
	repo, _, tip := makeRepo(t)

	decoy := t.TempDir()
	gitT(t, "", "init", "-q", decoy)
	if err := os.WriteFile(filepath.Join(decoy, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, decoy, "add", ".")
	gitT(t, decoy, "commit", "-q", "-m", "decoy")

	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	g, _ := New()
	sha, err := g.ResolveLocal(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if sha != tip {
		t.Errorf("ResolveLocal honored inherited GIT_DIR: got %s, want %s", sha, tip)
	}
}

// TestFixtureHelperIgnoresInheritedGitDir proves the test fixture helper
// (gitT/makeRepo) scrubs an inherited GIT_DIR, so building fixtures under a
// git hook never writes into the enclosing repository.
func TestFixtureHelperIgnoresInheritedGitDir(t *testing.T) {
	decoy := t.TempDir()
	gitT(t, "", "init", "-q", decoy)
	gitT(t, decoy, "commit", "-q", "--allow-empty", "-m", "decoy")
	before := gitT(t, decoy, "rev-parse", "HEAD")

	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)

	if _, _, tip := makeRepo(t); tip == "" {
		t.Fatal("makeRepo produced no tip commit")
	}

	if after := gitT(t, decoy, "rev-parse", "HEAD"); after != before {
		t.Errorf("fixture git leaked into inherited GIT_DIR: %s -> %s", before, after)
	}
}

func TestNewDetectsVersion(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if g.Version() == "" {
		t.Error("empty git version")
	}
}

func TestResolveRemoteDefaultBranch(t *testing.T) {
	repo, _, tip := makeRepo(t)
	g, _ := New()
	sha, err := g.ResolveRemote(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if sha != tip {
		t.Errorf("ResolveRemote HEAD = %s, want %s", sha, tip)
	}
}

func TestResolveRemoteBranchAndTag(t *testing.T) {
	repo, _, tip := makeRepo(t)
	g, _ := New()
	for _, ref := range []string{"main", "v1.0"} {
		sha, err := g.ResolveRemote(repo, ref)
		if err != nil {
			t.Fatalf("resolve %s: %v", ref, err)
		}
		if sha != tip {
			t.Errorf("ResolveRemote(%s) = %s, want %s", ref, sha, tip)
		}
	}
}

func TestResolveRemoteFullSHA(t *testing.T) {
	repo, old, _ := makeRepo(t)
	g, _ := New()
	sha, err := g.ResolveRemote(repo, old)
	if err != nil {
		t.Fatal(err)
	}
	if sha != old {
		t.Errorf("ResolveRemote(sha) = %s, want %s", sha, old)
	}
}

func TestResolveRemoteUnknownRef(t *testing.T) {
	repo, _, _ := makeRepo(t)
	g, _ := New()
	_, err := g.ResolveRemote(repo, "no-such-ref")
	if err == nil {
		t.Fatal("want error for unknown ref")
	}
	if !errors.Is(err, ErrRefNotFound) {
		t.Errorf("error %v does not wrap ErrRefNotFound", err)
	}
}

func TestResolveLocal(t *testing.T) {
	repo, old, tip := makeRepo(t)
	g, _ := New()
	sha, err := g.ResolveLocal(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if sha != tip {
		t.Errorf("ResolveLocal HEAD = %s, want %s", sha, tip)
	}
	sha, err = g.ResolveLocal(repo, old[:10])
	if err != nil {
		t.Fatal(err)
	}
	if sha != old {
		t.Errorf("ResolveLocal(short sha) = %s, want %s", sha, old)
	}
}

func TestExtractSubdirAtCommit(t *testing.T) {
	repo, old, tip := makeRepo(t)
	g, _ := New()

	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "skills/pdf", dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf v2" {
		t.Errorf("content = %q, want %q", data, "# pdf v2")
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Error("README.md leaked into extracted subdir")
	}

	// Non-tip commit must also be fetchable.
	destOld := filepath.Join(t.TempDir(), "out-old")
	if err := g.Extract(repo, old, "skills/pdf", destOld); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(destOld, "SKILL.md"))
	if string(data) != "# pdf v1" {
		t.Errorf("old content = %q, want %q", data, "# pdf v1")
	}
}

func TestExtractWholeRepo(t *testing.T) {
	repo, _, tip := makeRepo(t)
	g, _ := New()
	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "", dest); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"README.md", "skills/pdf/SKILL.md", "skills/web/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); !os.IsNotExist(err) {
		t.Error(".git leaked into extracted tree")
	}
}

// TestExtractRecreatesSymlinks proves symlinks in the fetched tree are
// recreated as symlinks: a directory symlink (harness mirror layout like
// .agents/skills/x -> ../../skills/x) must not be opened as a file, and a
// dangling link must not fail the copy.
func TestExtractRecreatesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	repo, _, _ := makeRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		filepath.Join(".agents", "skills", "pdf"):      "../../skills/pdf",
		filepath.Join(".agents", "skills", "dangling"): "../../skills/missing",
		"README-link": "README.md",
	} {
		if err := os.Symlink(target, filepath.Join(repo, link)); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "symlinks")
	tip := gitT(t, repo, "rev-parse", "HEAD")

	g, _ := New()
	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, tip, "", dest); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		filepath.Join(dest, ".agents", "skills", "pdf"):      "../../skills/pdf",
		filepath.Join(dest, ".agents", "skills", "dangling"): "../../skills/missing",
		filepath.Join(dest, "README-link"):                   "README.md",
	} {
		got, err := os.Readlink(link)
		if err != nil {
			t.Errorf("%s not recreated as a symlink: %v", link, err)
			continue
		}
		if got != target {
			t.Errorf("%s -> %q, want %q", link, got, target)
		}
	}
}

// TestExtractSymlinkedSubdir proves a subdir that is itself a symlink
// (skiletto add repo//.agents/skills/pdf against a mirror layout) resolves
// to its target instead of failing, in both sparse and full-clone modes.
func TestExtractSymlinkedSubdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	repo, _, _ := makeRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../skills/pdf", filepath.Join(repo, ".agents", "skills", "pdf")); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "mirror")
	tip := gitT(t, repo, "rev-parse", "HEAD")

	for name, mutate := range map[string]func(*Git){
		"sparse": func(*Git) {},
		"full":   func(g *Git) { g.sparse = false; g.shaFetch = false },
	} {
		g, _ := New()
		mutate(g)
		dest := filepath.Join(t.TempDir(), "out-"+name)
		if err := g.Extract(repo, tip, ".agents/skills/pdf", dest); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(data) != "# pdf v2" {
			t.Errorf("%s: content = %q, want %q", name, data, "# pdf v2")
		}
	}
}

func TestExtractFullCloneFallback(t *testing.T) {
	repo, old, _ := makeRepo(t)
	g, _ := New()
	g.sparse = false
	g.shaFetch = false
	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Extract(repo, old, "skills/pdf", dest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if string(data) != "# pdf v1" {
		t.Errorf("content = %q, want %q", data, "# pdf v1")
	}
}
