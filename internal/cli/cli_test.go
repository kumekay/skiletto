package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/gitcli"
	"github.com/kumekay/skiletto/internal/manifest"
)

// TestMain points HOME and XDG_CONFIG_HOME at a throwaway directory so no
// test ever reads the developer's real machine-scope manifest (its
// harnesses key would change behavior) or writes near their home.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "skiletto-cli-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	_ = os.Unsetenv("SKILETTO_CONFIG_DIR")
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

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

// makeRootAndNestedRepo creates a git repo holding a skill at the root and
// another under skills/, so an add without a //path is ambiguous.
func makeRootAndNestedRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitT(t, "", "init", "-q", repo)
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("# root"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, "skills", "pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# pdf"), 0o644); err != nil {
		t.Fatal(err)
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

	if _, stderr, err := run(t, "harness", "enable", "claude"); err != nil {
		t.Fatalf("harness enable: %v\n%s", err, stderr)
	}
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

	_, _, err := run(t, "add", "--no-input", repo)
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

func TestAddEditableRelativePathResolves(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)

	if _, stderr, err := run(t, "add", "--editable", "./my-skill"); err != nil {
		t.Fatalf("add --editable ./my-skill: %v\n%s", err, stderr)
	}

	canonical := filepath.Join(project, ".agents", "skills", "my-skill")
	// A broken symlink fails Stat: this asserts the link resolves.
	if _, err := os.Stat(filepath.Join(canonical, "SKILL.md")); err != nil {
		t.Fatalf("editable symlink does not resolve: %v", err)
	}
	target, err := os.Readlink(canonical)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if !filepath.IsAbs(target) {
		t.Errorf("symlink target %q is not absolute", target)
	}
	m, err := manifest.Load(filepath.Join(project, "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if src := m.Skills["my-skill"].Source; !filepath.IsAbs(src) {
		t.Errorf("manifest source %q is not absolute", src)
	}
}

func TestAddPinnedRelativePathAbsolutized(t *testing.T) {
	project := t.TempDir()
	repo := filepath.Join(project, "srcrepo")
	gitT(t, "", "init", "-q", repo)
	dir := filepath.Join(repo, "skills", "pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "skills")
	t.Chdir(project)

	if _, stderr, err := run(t, "add", "./srcrepo//skills/pdf"); err != nil {
		t.Fatalf("add ./srcrepo//skills/pdf: %v\n%s", err, stderr)
	}

	m, err := manifest.Load(filepath.Join(project, "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if src := m.Skills["pdf"].Source; !filepath.IsAbs(src) {
		t.Errorf("manifest source %q is not absolute", src)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("skill not materialized: %v", err)
	}
}

func TestAddEditableRelativePathGlobal(t *testing.T) {
	home, _ := setMachineHome(t)
	project := t.TempDir()
	dir := filepath.Join(project, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)

	if _, stderr, err := run(t, "add", "--global", "--editable", "./my-skill"); err != nil {
		t.Fatalf("add --global --editable ./my-skill: %v\n%s", err, stderr)
	}

	// The relative source resolves against the invocation cwd (project),
	// not the machine scope root (home).
	canonical := filepath.Join(home, ".agents", "skills", "my-skill")
	if _, err := os.Stat(filepath.Join(canonical, "SKILL.md")); err != nil {
		t.Fatalf("editable symlink does not resolve: %v", err)
	}
	target, err := os.Readlink(canonical)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if !filepath.IsAbs(target) {
		t.Errorf("symlink target %q is not absolute", target)
	}
}

// setMachineHome points the machine scope at a temp home and config dir so
// tests never touch the developer's real ~/.claude, ~/.agents, or ~/.config.
func setMachineHome(t *testing.T) (home, config string) {
	t.Helper()
	home = t.TempDir()
	config = filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	return home, config
}

// SKILETTO_CONFIG_DIR points directly at the directory holding the
// machine-scope manifest and lock, taking precedence over XDG_CONFIG_HOME
// (which gets a "skiletto" subdirectory appended).
func TestConfigDirEnvOverridesXDG(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	_, config := setMachineHome(t)
	cfgDir := filepath.Join(t.TempDir(), "custom-config")
	t.Setenv("SKILETTO_CONFIG_DIR", cfgDir)
	t.Chdir(t.TempDir())

	if _, stderr, err := run(t, "add", "--global", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add --global: %v\n%s", err, stderr)
	}
	for _, f := range []string{"skiletto.toml", "skiletto.lock"} {
		if _, err := os.Stat(filepath.Join(cfgDir, f)); err != nil {
			t.Errorf("missing %s in SKILETTO_CONFIG_DIR: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(config, "skiletto")); !os.IsNotExist(err) {
		t.Errorf("XDG config dir was used despite SKILETTO_CONFIG_DIR (stat err: %v)", err)
	}
}

func TestAddAndSyncGlobalRoundTrip(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	home, config := setMachineHome(t)
	t.Chdir(t.TempDir())

	if _, stderr, err := run(t, "harness", "enable", "claude", "--global"); err != nil {
		t.Fatalf("harness enable --global: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "add", "--global", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add --global: %v\n%s", err, stderr)
	}
	for _, f := range []string{"skiletto.toml", "skiletto.lock"} {
		if _, err := os.Stat(filepath.Join(config, "skiletto", f)); err != nil {
			t.Errorf("missing %s in config dir: %v", f, err)
		}
	}
	skillFile := filepath.Join(home, ".agents", "skills", "pdf", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Errorf("skill not materialized under home: %v", err)
	}
	link := filepath.Join(home, ".claude", "skills", "pdf")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("claude link missing or not a symlink: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(home, ".agents")); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := run(t, "sync", "--global"); err != nil {
		t.Fatalf("sync --global: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(skillFile); err != nil {
		t.Errorf("sync --global did not restore skill: %v", err)
	}
}

func TestAddEditableGlobalNoWarning(t *testing.T) {
	worktree := t.TempDir()
	dir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, _ := setMachineHome(t)
	t.Chdir(t.TempDir())

	_, stderr, err := run(t, "add", "--global", "--editable", worktree)
	if err != nil {
		t.Fatalf("add --global --editable: %v\n%s", err, stderr)
	}
	if strings.Contains(stderr, "warning") {
		t.Errorf("machine scope must not warn about portability:\n%s", stderr)
	}
	canonical := filepath.Join(home, ".agents", "skills", "my-skill")
	if target, err := os.Readlink(canonical); err != nil || target != dir {
		t.Errorf("canonical symlink -> %q (%v), want %q", target, err, dir)
	}
}

// Running without --global in the home directory would create a "project"
// scope whose skills dir is the machine scope's ~/.agents/skills. The
// machine scope must always be explicit.
func TestHomeDirRequiresGlobal(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	home, _ := setMachineHome(t)
	t.Chdir(home)

	for _, args := range [][]string{
		{"add", repo + "//skills/pdf"},
		{"sync"},
		{"list"},
		{"harness", "enable", "claude"},
	} {
		_, _, err := run(t, args...)
		if err == nil {
			t.Errorf("%v: want error in home dir without --global", args)
			continue
		}
		if !strings.Contains(err.Error(), "--global") {
			t.Errorf("%v: error should point at --global, got %v", args, err)
		}
	}

	// With --global (and its -g shorthand) the same invocations work.
	if _, stderr, err := run(t, "harness", "enable", "claude", "-g"); err != nil {
		t.Fatalf("harness enable -g: %v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "add", "-g", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add -g: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("skill not materialized under home: %v", err)
	}
}
