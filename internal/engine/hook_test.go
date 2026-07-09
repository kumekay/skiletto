package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

// recorderHook returns a hook command that checks the staged skill (via
// SKILETTO_SKILL_DIR) contains SKILL.md and records "<event> <name>" (plus
// the appended staged-dir argument) into outFile. When the check fails the
// file is not written.
func recorderHook(outFile string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`if exist "%%SKILETTO_SKILL_DIR%%\SKILL.md" echo %%SKILETTO_EVENT%% %%SKILETTO_SKILL_NAME%% > "%s"`, outFile)
	}
	return fmt.Sprintf(`test -f "$SKILETTO_SKILL_DIR/SKILL.md" && echo "$SKILETTO_EVENT" "$SKILETTO_SKILL_NAME" > '%s'`, outFile)
}

// failHook is a command that exits non-zero on every platform, even with
// the staged dir appended as an argument.
const failHook = "git --skiletto-no-such-flag"

// setHook rewrites the scope manifest in place with the hook set, keeping
// the recorded skills.
func (f *fixture) setHook(t *testing.T, command string) {
	t.Helper()
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	m.Hooks = map[string]string{"pre-install": command}
	f.writeManifest(t, m)
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
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": recorderHook(outFile)},
		Skills: map[string]manifest.Entry{},
	})

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	got := recorded(t, outFile)
	if !strings.HasPrefix(got, "add pdf") {
		t.Errorf("hook record = %q, want prefix %q", got, "add pdf")
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("not installed: %v", err)
	}
}

func TestAddFailingHookAbortsInstall(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": failHook},
		Skills: map[string]manifest.Entry{},
	})

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
	f.setHook(t, recorderHook(outFile))

	if err := f.eng.Update("", false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); !strings.HasPrefix(got, "update pdf") {
		t.Errorf("hook record = %q, want prefix %q", got, "update pdf")
	}
}

func TestUpdateFailingHookKeepsInstalledVersion(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	f.setHook(t, failHook)
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
	f.setHook(t, recorderHook(outFile))
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
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": recorderHook(outFile)},
		Skills: map[string]manifest.Entry{"pdf": pdfEntry()},
	})

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); !strings.HasPrefix(got, "sync pdf") {
		t.Errorf("hook record = %q, want prefix %q", got, "sync pdf")
	}
}

func TestNoHooksSkipsHook(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": failHook},
		Skills: map[string]manifest.Entry{},
	})
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
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": failHook},
		Skills: map[string]manifest.Entry{},
	})

	if err := f.eng.Add(manifest.SourceSpec{Source: worktree, IsPath: true}, true); err != nil {
		t.Fatal(err)
	}
}

func TestHookFallsBackToMachineManifest(t *testing.T) {
	f := newFixture(t, pdfSource())
	outFile := filepath.Join(t.TempDir(), "hook.out")
	mm := &manifest.Manifest{
		Harnesses: []string{"fake"},
		Hooks:     map[string]string{"pre-install": recorderHook(outFile)},
		Skills:    map[string]manifest.Entry{},
	}
	if err := mm.Save(f.eng.Machine.ManifestPath); err != nil {
		t.Fatal(err)
	}

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); !strings.HasPrefix(got, "add pdf") {
		t.Errorf("hook record = %q, want prefix %q", got, "add pdf")
	}
}

func TestScopeHookOverridesMachineHook(t *testing.T) {
	f := newFixture(t, pdfSource())
	outFile := filepath.Join(t.TempDir(), "hook.out")
	mm := &manifest.Manifest{
		Harnesses: []string{"fake"},
		Hooks:     map[string]string{"pre-install": failHook},
		Skills:    map[string]manifest.Entry{},
	}
	if err := mm.Save(f.eng.Machine.ManifestPath); err != nil {
		t.Fatal(err)
	}
	f.writeManifest(t, &manifest.Manifest{
		Hooks:  map[string]string{"pre-install": recorderHook(outFile)},
		Skills: map[string]manifest.Entry{},
	})

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, outFile); !strings.HasPrefix(got, "add pdf") {
		t.Errorf("hook record = %q, want prefix %q", got, "add pdf")
	}
}
