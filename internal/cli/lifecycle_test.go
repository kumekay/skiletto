package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// advanceRepo adds a new commit to the pdf skill in repo and returns the new HEAD.
func advanceRepo(t *testing.T, repo string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "skills", "pdf", "SKILL.md"), []byte("# pdf v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "advance")
	return gitT(t, repo, "rev-parse", "HEAD")
}

func lockContains(t *testing.T, project, want string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, "skiletto.lock"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(data), want)
}

func TestUpdateReResolvesWhereSyncDoesNot(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	newHead := advanceRepo(t, repo)

	// sync alone never moves an already-locked version.
	if _, stderr, err := run(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	if lockContains(t, project, newHead) {
		t.Error("sync re-resolved to the new commit")
	}

	// update re-resolves to the new commit.
	if _, stderr, err := run(t, "update"); err != nil {
		t.Fatalf("update: %v\n%s", err, stderr)
	}
	if !lockContains(t, project, newHead) {
		t.Error("update did not re-resolve to the new commit")
	}
	data, _ := os.ReadFile(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md"))
	if string(data) != "# pdf v2" {
		t.Errorf("installed content after update = %q", data)
	}
}

func TestRemoveCommand(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "remove", "pdf"); err != nil {
		t.Fatalf("remove: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "pdf")); !os.IsNotExist(err) {
		t.Error("materialized copy not deleted")
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "skills", "pdf")); !os.IsNotExist(err) {
		t.Error("claude link not removed")
	}
	if lockContains(t, project, "pdf") {
		t.Error("lock still references pdf")
	}
}

func TestListCommand(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	stdout, stderr, err := run(t, "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, stderr)
	}
	for _, want := range []string{"pdf", "ok", "NAME"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output missing %q:\n%s", want, stdout)
		}
	}
}

func TestUpdateGlobal(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	_, config := setMachineHome(t)
	t.Chdir(t.TempDir())

	if _, stderr, err := run(t, "add", "--global", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add --global: %v\n%s", err, stderr)
	}
	newHead := advanceRepo(t, repo)
	if _, stderr, err := run(t, "update", "--global"); err != nil {
		t.Fatalf("update --global: %v\n%s", err, stderr)
	}
	data, err := os.ReadFile(filepath.Join(config, "skiletto", "skiletto.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), newHead) {
		t.Error("update --global did not re-resolve in the machine-scope lock")
	}
}
