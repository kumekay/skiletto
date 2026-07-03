package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// makeSkillRepo creates a local git repo with skills under skills/.
func makeSkillRepo(t *testing.T, skills ...string) string {
	t.Helper()
	repo := t.TempDir()
	gitT(t, "", "init", "-q", repo)
	for _, name := range skills {
		dir := filepath.Join(repo, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "skills")
	return repo
}

func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestAddAndSyncRoundTrip(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	_, stderr, err := run(t, "add", repo+"//skills/pdf")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	for _, f := range []string{"skiletto.toml", "skiletto.lock"} {
		if _, err := os.Stat(filepath.Join(project, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("skill not materialized: %v", err)
	}
	link := filepath.Join(project, ".claude", "skills", "pdf")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("claude link missing or not a symlink: %v", err)
	}

	// Wipe installed state; sync must restore it from the lock.
	if err := os.RemoveAll(filepath.Join(project, ".agents")); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := run(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("sync did not restore skill: %v", err)
	}
}

func TestAddMultiSkillListsChoices(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", repo)
	if err == nil {
		t.Fatal("want error for ambiguous source")
	}
	msg := err.Error()
	for _, want := range []string{"skills/pdf", "skills/web", "skiletto add"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestSyncForceFlag(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	skillFile := filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "sync"); err == nil {
		t.Fatal("want drift error from sync")
	}
	if _, stderr, err := run(t, "sync", "--force"); err != nil {
		t.Fatalf("sync --force: %v\n%s", err, stderr)
	}
	data, _ := os.ReadFile(skillFile)
	if string(data) != "# pdf" {
		t.Errorf("content after force = %q", data)
	}
}

func TestAddEditableFlag(t *testing.T) {
	worktree := t.TempDir()
	dir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	t.Chdir(project)

	_, stderr, err := run(t, "add", "--editable", worktree)
	if err != nil {
		t.Fatalf("add --editable: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("no portability warning:\n%s", stderr)
	}
	canonical := filepath.Join(project, ".agents", "skills", "my-skill")
	if target, err := os.Readlink(canonical); err != nil || target != dir {
		t.Errorf("canonical symlink -> %q (%v), want %q", target, err, dir)
	}
}
