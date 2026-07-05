package cli

import (
	"os"
	"path/filepath"
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
