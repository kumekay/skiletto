package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/skill"
)

// recorderHook returns a hook command that checks the staged skill (via
// SKILETTO_SKILL_DIR) contains SKILL.md and records "<event> <name>" into
// outFile. When the check fails the file is not written.
func recorderHook(outFile string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`if exist "%%SKILETTO_SKILL_DIR%%\SKILL.md" echo %%SKILETTO_EVENT%% %%SKILETTO_SKILL_NAME%%> "%s"`, outFile)
	}
	return fmt.Sprintf(`test -f "$SKILETTO_SKILL_DIR/SKILL.md" && echo "$SKILETTO_EVENT $SKILETTO_SKILL_NAME" > '%s'`, outFile)
}

// mutatorHook returns a hook command that appends to the staged SKILL.md,
// like a sanitizer or formatter would.
func mutatorHook() string {
	if runtime.GOOS == "windows" {
		return `echo extra>> "%SKILETTO_SKILL_DIR%\SKILL.md"`
	}
	return `echo extra >> "$SKILETTO_SKILL_DIR/SKILL.md"`
}

// failHook is a command that exits non-zero on every platform.
const failHook = "git --skiletto-no-such-flag"

// setMachineHooks rewrites the fixture's machine manifest with the given
// hooks table, keeping the fake harness enabled.
func (f *fixture) setMachineHooks(t *testing.T, hooks map[string]string) {
	t.Helper()
	mm := &manifest.Manifest{
		Harnesses: []string{"fake"},
		Hooks:     hooks,
		Skills:    map[string]manifest.Entry{},
	}
	if err := mm.Save(f.eng.Machine.ManifestPath); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) setMachineHook(t *testing.T, command string) {
	t.Helper()
	f.setMachineHooks(t, map[string]string{"pre-install": command})
}

// recorded returns the recorder output, or "" when the hook never ran.
func recorded(t *testing.T, outFile string) string {
	t.Helper()
	data, err := os.ReadFile(outFile)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestAddRunsPreInstallHook(t *testing.T) {
	f := newFixture(t, pdfSource())
	outFile := filepath.Join(t.TempDir(), "hook.out")
	f.setMachineHook(t, recorderHook(outFile))

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); got != "add pdf" {
		t.Errorf("hook record = %q, want %q", got, "add pdf")
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("not installed: %v", err)
	}
}

func TestAddFailingHookAbortsInstall(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.setMachineHook(t, failHook)

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	err := f.eng.Add(spec, false)
	if err == nil || !strings.Contains(err.Error(), "pre-install hook") {
		t.Fatalf("want pre-install hook error, got %v", err)
	}
	if _, err := os.Lstat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Errorf("canonical dir exists after aborted add (err=%v)", err)
	}
	m, _ := manifest.Load(f.scope.ManifestPath)
	if _, ok := m.Skills["pdf"]; ok {
		t.Error("manifest gained an entry despite hook failure")
	}
	if f.readLock(t).Find("pdf") != nil {
		t.Error("lock gained an entry despite hook failure")
	}
	if len(f.adapter.links) != 0 {
		t.Errorf("adapter links = %v, want none", f.adapter.links)
	}
}

func TestUpdateRunsHookWithUpdateEvent(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(t.TempDir(), "hook.out")
	f.setMachineHook(t, recorderHook(outFile))

	if err := f.eng.Update("", false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); got != "update pdf" {
		t.Errorf("hook record = %q, want %q", got, "update pdf")
	}
}

func TestUpdateFailingHookKeepsInstalledVersion(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	f.setMachineHook(t, failHook)
	const commitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	f.src.commit = commitB
	f.src.tree = map[string]string{"skills/pdf/SKILL.md": "# pdf v2"}

	if err := f.eng.Update("", false); err == nil {
		t.Fatal("update succeeded despite failing hook")
	}
	data, err := os.ReadFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf" {
		t.Errorf("installed content = %q, want the pre-update version", data)
	}
	if s := f.readLock(t).Find("pdf"); s == nil || s.Commit != commitA {
		t.Errorf("lock entry = %+v, want commit %s", s, commitA)
	}
	if f.adapter.links["pdf"] == "" {
		t.Error("skill was unlinked despite failing hook")
	}
}

func TestSyncMaterializeSkipsHook(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(t.TempDir(), "hook.out")
	f.setMachineHook(t, recorderHook(outFile))
	if err := os.RemoveAll(f.scope.SkillDir("pdf")); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); got != "" {
		t.Errorf("hook ran on materialize: %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("not restored: %v", err)
	}
}

func TestSyncFetchRunsHookWithSyncEvent(t *testing.T) {
	f := newFixture(t, pdfSource())
	outFile := filepath.Join(t.TempDir(), "hook.out")
	f.setMachineHook(t, recorderHook(outFile))
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); got != "sync pdf" {
		t.Errorf("hook record = %q, want %q", got, "sync pdf")
	}
}

func TestNoHooksSkipsHook(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.setMachineHook(t, failHook)
	f.eng.NoHooks = true

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("not installed: %v", err)
	}
}

func TestAddEditableSkipsHook(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pdfSource())
	f.setMachineHook(t, failHook)

	if err := f.eng.Add(manifest.SourceSpec{Source: worktree, IsPath: true}, true); err != nil {
		t.Fatal(err)
	}
}

// Hooks execute arbitrary commands, so a project's checked-in skiletto.toml
// must never supply one: a cloned repository would gain code execution on
// sync and could replace the user's scanner. Machine-manifest hooks are the
// only ones honored.
func TestProjectManifestHooksIgnoredWithWarning(t *testing.T) {
	f := newFixture(t, pdfSource())
	outFile := filepath.Join(t.TempDir(), "hook.out")
	f.setMachineHook(t, recorderHook(outFile))
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": failHook},
		Skills: map[string]manifest.Entry{},
	})

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); got != "add pdf" {
		t.Errorf("machine hook record = %q, want %q", got, "add pdf")
	}
	if !strings.Contains(f.errOut.String(), "machine") {
		t.Errorf("no warning about ignored project hooks:\n%s", f.errOut.String())
	}
}

// In the machine scope itself, the scope manifest is the machine manifest,
// so its hook runs.
func TestMachineScopeHookRuns(t *testing.T) {
	home := t.TempDir()
	sc := scope.Machine(home, filepath.Join(home, ".config"))
	f := newFixtureScope(t, pdfSource(), sc)
	outFile := filepath.Join(t.TempDir(), "hook.out")
	mm := &manifest.Manifest{
		Harnesses: []string{"fake"},
		Hooks:     map[string]string{"pre-install": recorderHook(outFile)},
		Skills:    map[string]manifest.Entry{},
	}
	if err := mm.Save(sc.ManifestPath); err != nil {
		t.Fatal(err)
	}

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); got != "add pdf" {
		t.Errorf("hook record = %q, want %q", got, "add pdf")
	}
}

// An unknown hook name in the machine manifest must fail installs (a typo
// cannot silently disable the gate) without bricking read-only commands —
// the manifest still parses.
func TestUnknownHookNameFailsInstall(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.setMachineHooks(t, map[string]string{"post-install": "echo done"})

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	err := f.eng.Add(spec, false)
	if err == nil || !strings.Contains(err.Error(), "unknown hook") {
		t.Fatalf("want unknown hook error, got %v", err)
	}
}

// The gate fails closed: an unreadable machine manifest aborts the install
// instead of proceeding without the hook.
func TestUnreadableMachineManifestFailsInstall(t *testing.T) {
	f := newFixture(t, pdfSource())
	if err := os.WriteFile(f.eng.Machine.ManifestPath, []byte("not [ toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err == nil {
		t.Fatal("add succeeded with an unreadable machine manifest and hooks enabled")
	}
	if _, err := os.Lstat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("skill installed despite unreadable machine manifest")
	}
}

// The lock hash must describe what is actually promoted: a hook that
// rewrites staged content (sanitizer, formatter) must not cause permanent
// drift.
func TestMutatingHookDoesNotCauseDrift(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.setMachineHook(t, mutatorHook())

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	installed, err := skill.Hash(f.scope.SkillDir("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if locked := f.readLock(t).Find("pdf"); locked == nil || locked.Hash != installed {
		t.Errorf("lock hash = %+v, want installed hash %q", locked, installed)
	}
}

// Verbose mode announces each hook run, naming the skill and the event, so
// an otherwise-silent hook still confirms it was picked up and ran.
func TestVerboseAnnouncesHook(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.setMachineHook(t, "cd .")
	f.eng.Verbose = true

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	got := f.errOut.String()
	if !strings.Contains(got, "pre-install hook") || !strings.Contains(got, "pdf") || !strings.Contains(got, "add") {
		t.Errorf("verbose did not announce the hook run:\n%s", got)
	}
}

// Without verbose, a successful hook run stays silent: no diagnostic line is
// written for it.
func TestHookRunSilentWithoutVerbose(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.setMachineHook(t, "cd .")

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.errOut.String(), "pre-install hook") {
		t.Errorf("hook run announced without --verbose:\n%s", f.errOut.String())
	}
}

// A hook rejection during import must not destroy a pre-existing installed
// tree: install() aborts before touching it, so import has nothing to
// clean up.
func TestImportHookRejectionKeepsExistingTree(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	// Orphan the entry: still locked, still installed, gone from manifest.
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{}})
	f.setMachineHook(t, failHook)

	lock := writeVercelLock(t, t.TempDir(), `{
		"version": 3,
		"skills": {
			"pdf": {"source": "o/r", "sourceType": "github", "skillPath": "skills/pdf/SKILL.md"}
		}
	}`)
	if err := f.eng.Import(lock, false); err == nil {
		t.Fatal("want non-zero exit when the hook rejects the import")
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("pre-existing installed tree deleted after hook rejection: %v", err)
	}
	if f.adapter.links["pdf"] == "" {
		t.Error("pre-existing skill unlinked after hook rejection")
	}
}
