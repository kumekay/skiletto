package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/gitcli"
)

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
	cmd.Env = gitcli.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func makeRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	gitT(t, "", "init", "-q", repo)
	p := filepath.Join(repo, "pdf", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "first")
	return repo, gitT(t, repo, "rev-parse", "HEAD")
}

func TestIsLocalPath(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/o/r":                 false,
		"ssh://gitea@git.kumekay.com:30009/ku/r": false,
		"git@github.com:o/r.git":                 false,
		"/abs/path":                              true,
		"./rel":                                  true,
		"../rel":                                 true,
		"~/p/my-skills":                          true,
		`C:\Users\me\skills`:                     true,
		"C:/Users/me/skills":                     true,
		`\\host\share\skills`:                    true,
	}
	for src, want := range cases {
		if got := IsLocalPath(src); got != want {
			t.Errorf("IsLocalPath(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := ExpandHome("~/x"); got != filepath.Join(home, "x") {
		t.Errorf("ExpandHome(~/x) = %q", got)
	}
	if got := ExpandHome("/abs"); got != "/abs" {
		t.Errorf("ExpandHome(/abs) = %q", got)
	}
}

func TestGitSourceAgainstLocalRepo(t *testing.T) {
	repo, tip := makeRepo(t)
	g, err := gitcli.New()
	if err != nil {
		t.Fatal(err)
	}
	src := New(g, repo) // a path is a valid git URL too; use Path via New
	commit, err := src.Resolve("main")
	if err != nil {
		t.Fatal(err)
	}
	if commit != tip {
		t.Errorf("Resolve = %s, want %s", commit, tip)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := src.Fetch(commit, "pdf", dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("missing SKILL.md: %v", err)
	}
}

func TestPathSourceNotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	g, _ := gitcli.New()
	src := New(g, dir)
	if _, err := src.Resolve(""); err == nil {
		t.Error("want error for non-git path source")
	}
}

// TestPathSourceNotAGitRepoInheritedGitDir reproduces the git-hook false
// failure: with GIT_DIR inherited from an enclosing repo, a non-git path
// source must still report an error rather than resolving the hook's repo.
func TestPathSourceNotAGitRepoInheritedGitDir(t *testing.T) {
	real := t.TempDir()
	gitT(t, "", "init", "-q", real)
	gitT(t, real, "commit", "-q", "--allow-empty", "-m", "real")
	t.Setenv("GIT_DIR", filepath.Join(real, ".git"))

	dir := t.TempDir()
	g, _ := gitcli.New()
	src := New(g, dir)
	if _, err := src.Resolve(""); err == nil {
		t.Error("want error for non-git path source under inherited GIT_DIR")
	}
}
