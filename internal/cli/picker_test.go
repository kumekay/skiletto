package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/ui"
)

// recordingPrompter is a fake Prompter: it records what it was asked and
// returns a canned selection, so the multi-select add flow can be tested
// without a real terminal.
type recordingPrompter struct {
	called  bool
	title   string
	options []ui.Option
	ret     []string
	err     error
}

func (p *recordingPrompter) MultiSelect(title string, options []ui.Option) ([]string, error) {
	p.called = true
	p.title = title
	p.options = options
	return p.ret, p.err
}

// setPrompter installs p as the prompter the add command uses and restores
// the default afterwards.
func setPrompter(t *testing.T, p ui.Prompter) {
	t.Helper()
	old := promptSelector
	promptSelector = func(bool) ui.Prompter { return p }
	t.Cleanup(func() { promptSelector = old })
}

func skillInstalled(project, name string) bool {
	_, err := os.Stat(filepath.Join(project, ".agents", "skills", name, "SKILL.md"))
	return err == nil
}

func TestAddMultiSelectInstallsChosen(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web", "img")
	project := t.TempDir()
	t.Chdir(project)

	fake := &recordingPrompter{ret: []string{"skills/pdf", "skills/web"}}
	setPrompter(t, fake)

	if _, stderr, err := run(t, "add", repo); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	if !fake.called {
		t.Fatal("prompter was not invoked for an ambiguous add")
	}
	if len(fake.options) != 3 {
		t.Errorf("prompter got %d options, want 3", len(fake.options))
	}
	for _, name := range []string{"pdf", "web"} {
		if !skillInstalled(project, name) {
			t.Errorf("selected skill %q not installed", name)
		}
	}
	if skillInstalled(project, "img") {
		t.Error("unselected skill img was installed")
	}
	data, _ := os.ReadFile(filepath.Join(project, "skiletto.toml"))
	for _, want := range []string{"pdf", "web"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("manifest missing %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "img") {
		t.Errorf("manifest should not contain img:\n%s", data)
	}
}

// Issue #22: the picker option for a root skill must carry Value "." and a
// hint ending in `//.`, so selecting it re-drives the add addressably
// instead of hitting the same ambiguity through an empty subpath.
func TestAddMultiSelectRootSkillOptionUsesDot(t *testing.T) {
	repo := makeRootAndNestedRepo(t)
	project := t.TempDir()
	t.Chdir(project)

	fake := &recordingPrompter{ret: []string{"."}}
	setPrompter(t, fake)

	if _, stderr, err := run(t, "add", repo); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	if !fake.called {
		t.Fatal("prompter was not invoked for an ambiguous add")
	}
	var root *ui.Option
	for i := range fake.options {
		if fake.options[i].Value == "." {
			root = &fake.options[i]
		}
		if fake.options[i].Value == "" {
			t.Errorf("picker option with empty Value: %+v", fake.options)
		}
	}
	if root == nil {
		t.Fatalf("no picker option for the root skill (Value \".\"): %+v", fake.options)
	}
	if !strings.HasSuffix(root.Hint, "//.") {
		t.Errorf("root option hint = %q, want it to end in //.", root.Hint)
	}
}

func TestAddMultiSelectWarnsPortabilityOnce(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	project := t.TempDir()
	t.Chdir(project)

	setPrompter(t, &recordingPrompter{ret: []string{"skills/pdf", "skills/web"}})

	_, stderr, err := run(t, "add", repo)
	if err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	if n := strings.Count(stderr, "machine-specific path"); n != 1 {
		t.Errorf("portability warning appeared %d times, want 1:\n%s", n, stderr)
	}
}

func TestAddMultiSelectNothingChosenErrors(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	project := t.TempDir()
	t.Chdir(project)

	setPrompter(t, &recordingPrompter{ret: nil})

	_, _, err := run(t, "add", repo)
	if err == nil {
		t.Fatal("want an error when nothing is selected")
	}
	if !strings.Contains(err.Error(), "no skills selected") {
		t.Errorf("error = %q, want it to mention no skills selected", err)
	}
	if skillInstalled(project, "pdf") || skillInstalled(project, "web") {
		t.Error("nothing should have been installed")
	}
}

func TestAddAllInstallsEverySkill(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web", "img")
	project := t.TempDir()
	t.Chdir(project)

	// --all must never consult the prompter.
	setPrompter(t, &recordingPrompter{err: os.ErrInvalid})

	if _, stderr, err := run(t, "add", "--all", repo); err != nil {
		t.Fatalf("add --all: %v\n%s", err, stderr)
	}
	for _, name := range []string{"pdf", "web", "img"} {
		if !skillInstalled(project, name) {
			t.Errorf("skill %q not installed by --all", name)
		}
	}
}

func TestAddAllRejectsExplicitPath(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--all", repo+"//skills/pdf")
	if err == nil {
		t.Fatal("want an error combining --all with an explicit //path")
	}
	if !strings.Contains(err.Error(), "--all") || !strings.Contains(err.Error(), "//path") {
		t.Errorf("error = %q, want it to explain the --all / //path conflict", err)
	}
}

func TestAddSkillFlagInstallsNamed(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web", "img")
	project := t.TempDir()
	t.Chdir(project)

	// --skill must never consult the prompter.
	setPrompter(t, &recordingPrompter{err: os.ErrInvalid})

	if _, stderr, err := run(t, "add", "--skill", "pdf", "--skill", "web", repo); err != nil {
		t.Fatalf("add --skill: %v\n%s", err, stderr)
	}
	for _, name := range []string{"pdf", "web"} {
		if !skillInstalled(project, name) {
			t.Errorf("skill %q not installed by --skill", name)
		}
	}
	if skillInstalled(project, "img") {
		t.Error("unrequested skill img was installed")
	}
}

func TestAddSkillFlagUnknownNameListsAvailable(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--skill", "nope", repo)
	if err == nil {
		t.Fatal("want an error for an unknown skill name")
	}
	msg := err.Error()
	for _, want := range []string{"nope", "pdf", "web"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestAddSkillFlagConflictsWithAll(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--all", "--skill", "pdf", repo)
	if err == nil {
		t.Fatal("want an error combining --all with --skill")
	}
	if !strings.Contains(err.Error(), "all") || !strings.Contains(err.Error(), "skill") {
		t.Errorf("error = %q, want it to name both flags", err)
	}
}

func TestAddSkillFlagComposesWithPath(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", "--skill", "pdf", repo+"//skills"); err != nil {
		t.Fatalf("add //skills --skill pdf: %v\n%s", err, stderr)
	}
	if !skillInstalled(project, "pdf") {
		t.Error("pdf not installed")
	}
	if skillInstalled(project, "web") {
		t.Error("web installed despite --skill pdf")
	}
}

func TestAddSkillFlagAmbiguousNameSuggestsPath(t *testing.T) {
	repo := makeSkillRepo(t, "pdf")
	dir := filepath.Join(repo, "extra", "pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# other pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "duplicate name")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--skill", "pdf", repo)
	if err == nil {
		t.Fatal("want an error for an ambiguous skill name")
	}
	msg := err.Error()
	for _, want := range []string{"pdf", "//path", "skills/pdf", "extra/pdf"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestAddSkillFlagUnknownNameDedupesAvailable(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	dir := filepath.Join(repo, "extra", "pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# other pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "duplicate name")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--skill", "nope", repo)
	if err == nil {
		t.Fatal("want an error for an unknown skill name")
	}
	msg := err.Error()
	avail := msg[strings.Index(msg, "available"):]
	if n := strings.Count(avail, "pdf"); n != 1 {
		t.Errorf("duplicate name listed %d times, want once:\n%s", n, msg)
	}
}

// The ambiguous-name suggestions must be complete commands: an --editable
// add keeps the flag, or the suggested command would do a pinned install.
func TestAddSkillFlagAmbiguousEditableKeepsFlag(t *testing.T) {
	src := t.TempDir()
	for _, sub := range []string{"a/pdf", "b/pdf"} {
		dir := filepath.Join(src, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# pdf"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--editable", "--skill", "pdf", src)
	if err == nil {
		t.Fatal("want an error for an ambiguous skill name")
	}
	if !strings.Contains(err.Error(), "--editable") {
		t.Errorf("suggestions dropped --editable:\n%s", err)
	}
}

func TestAddSkillFlagEditableInstallsNamed(t *testing.T) {
	src := t.TempDir()
	for _, name := range []string{"pdf", "web"} {
		dir := filepath.Join(src, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", "--editable", "--skill", "pdf", src); err != nil {
		t.Fatalf("add --editable --skill: %v\n%s", err, stderr)
	}
	if !skillInstalled(project, "pdf") {
		t.Error("pdf not installed")
	}
	if skillInstalled(project, "web") {
		t.Error("web installed despite --skill pdf")
	}
	data, _ := os.ReadFile(filepath.Join(project, "skiletto.toml"))
	if !strings.Contains(string(data), "editable = true") {
		t.Errorf("manifest entry not editable:\n%s", data)
	}
}

// The root skill of a source is addressed by the source's base name (the
// same name the availability list and the picker print for it).
func TestAddSkillFlagSelectsRootSkill(t *testing.T) {
	repo := makeRootAndNestedRepo(t)
	project := t.TempDir()
	t.Chdir(project)

	name := filepath.Base(repo)
	if _, stderr, err := run(t, "add", "--skill", name, repo); err != nil {
		t.Fatalf("add --skill %s: %v\n%s", name, err, stderr)
	}
	if !skillInstalled(project, name) {
		t.Errorf("root skill %q not installed", name)
	}
	if skillInstalled(project, "pdf") {
		t.Error("nested skill installed despite --skill naming the root")
	}
	data, _ := os.ReadFile(filepath.Join(project, "skiletto.toml"))
	if !strings.Contains(string(data), `path = "."`) {
		t.Errorf("root skill entry missing path \".\":\n%s", data)
	}
}

func TestAddNoInputMentionsSkillFlag(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--no-input", repo)
	if err == nil {
		t.Fatal("want an actionable error under --no-input")
	}
	if !strings.Contains(err.Error(), "--skill") {
		t.Errorf("no-input error should mention --skill:\n%s", err)
	}
}

// A skill containing a symlink whose target lies outside the skill subtree
// must add cleanly via //path and stay converged on sync: the lock hash is
// computed over the link target string, not the content behind it, so the
// staged and installed copies always agree (reviewer scenario for the
// symlink-preserving fetch).
func TestAddSkillWithOutOfTreeSymlinkConverges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	repo := makeSkillRepo(t, "pdf")
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("MIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../LICENSE", filepath.Join(repo, "skills", "pdf", "LICENSE")); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "symlinked license")
	project := t.TempDir()
	t.Chdir(project)

	if _, stderr, err := run(t, "add", repo+"//skills/pdf"); err != nil {
		t.Fatalf("add: %v\n%s", err, stderr)
	}
	link := filepath.Join(project, ".agents", "skills", "pdf", "LICENSE")
	if target, err := os.Readlink(link); err != nil || target != "../../LICENSE" {
		t.Errorf("installed LICENSE not a symlink to ../../LICENSE: %q, %v", target, err)
	}
	for i := range 2 {
		if _, stderr, err := run(t, "sync"); err != nil {
			t.Fatalf("sync #%d: %v\n%s", i+1, err, stderr)
		}
	}
	if _, _, err := run(t, "list"); err != nil {
		t.Errorf("list: %v", err)
	}
}

// --all with a /tree/ URL that carries a path must not blame a //path the
// user never typed; the error names the URL's path instead.
func TestAddAllWithTreeURLExplainsConflict(t *testing.T) {
	t.Chdir(t.TempDir())

	_, _, err := run(t, "add", "--all", "https://github.com/o/r/tree/main/skills")
	if err == nil {
		t.Fatal("want an error combining --all with a /tree/ URL path")
	}
	if !strings.Contains(err.Error(), "/tree/") {
		t.Errorf("error should mention the /tree/ URL:\n%s", err)
	}
	if strings.Contains(err.Error(), "//path") {
		t.Errorf("error blames //path the user never typed:\n%s", err)
	}
}

func TestAddNoInputListsChoices(t *testing.T) {
	repo := makeSkillRepo(t, "pdf", "web")
	t.Chdir(t.TempDir())

	// --no-input forces the non-interactive path regardless of any TTY.
	_, _, err := run(t, "add", "--no-input", repo)
	if err == nil {
		t.Fatal("want an actionable error under --no-input")
	}
	msg := err.Error()
	for _, want := range []string{"skills/pdf", "skills/web", "skiletto add", "--all"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}
