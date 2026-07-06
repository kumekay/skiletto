package engine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/lockfile"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/skill"
	"github.com/kumekay/skiletto/internal/source"
)

// fakeSource serves an in-memory file tree at a fixed commit.
type fakeSource struct {
	commit   string
	tree     map[string]string // slash-relative path -> content
	resolved *int              // counts Resolve calls when non-nil
}

func (f *fakeSource) Resolve(ref string) (string, error) {
	if f.resolved != nil {
		*f.resolved++
	}
	return f.commit, nil
}

func (f *fakeSource) Fetch(commit, subpath, dest string) error {
	if commit != f.commit {
		return fmt.Errorf("unknown commit %s", commit)
	}
	found := false
	for rel, content := range f.tree {
		target := rel
		if subpath != "" {
			if !strings.HasPrefix(rel, subpath+"/") {
				continue
			}
			target = strings.TrimPrefix(rel, subpath+"/")
		}
		found = true
		p := filepath.Join(dest, filepath.FromSlash(target))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("path %q not found", subpath)
	}
	return nil
}

// fakeAdapter records link operations.
type fakeAdapter struct {
	links map[string]string
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{links: map[string]string{}}
}

func (a *fakeAdapter) Name() string                   { return "fake" }
func (a *fakeAdapter) SkillsDir(s scope.Scope) string { return filepath.Join(s.Root, ".fake") }

func (a *fakeAdapter) Link(s scope.Scope, name, target string, force bool) error {
	a.links[name] = target
	return nil
}

func (a *fakeAdapter) Unlink(s scope.Scope, name string, force bool) error {
	delete(a.links, name)
	return nil
}

func (a *fakeAdapter) Detected(s scope.Scope) bool {
	_, err := os.Lstat(a.SkillsDir(s))
	return err == nil
}

type fixture struct {
	eng     *Engine
	scope   scope.Scope
	src     *fakeSource
	adapter *fakeAdapter
	out     *bytes.Buffer
	errOut  *bytes.Buffer
}

func newFixture(t *testing.T, src *fakeSource) *fixture {
	t.Helper()
	return newFixtureScope(t, src, scope.Project(t.TempDir()))
}

func newFixtureScope(t *testing.T, src *fakeSource, sc scope.Scope) *fixture {
	t.Helper()
	ad := newFakeAdapter()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	// A machine scope with the fake harness enabled, so fixtures link by
	// default; tests exercising unconfigured scopes clear eng.Machine.
	machine := sc
	if sc.Kind != scope.KindMachine {
		home := t.TempDir()
		machine = scope.Machine(home, filepath.Join(home, ".config"))
	}
	mm := &manifest.Manifest{Harnesses: []string{"fake"}, Skills: map[string]manifest.Entry{}}
	if err := mm.Save(machine.ManifestPath); err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Scope:    sc,
		Machine:  &machine,
		Adapters: []adapter.Adapter{ad},
		NewSource: func(s string) (source.Source, error) {
			return src, nil
		},
		Out: out,
		Err: errOut,
	}
	return &fixture{eng: eng, scope: sc, src: src, adapter: ad, out: out, errOut: errOut}
}

// setMachineHarnesses rewrites the fixture's machine manifest to enable
// exactly the given harnesses.
func (f *fixture) setMachineHarnesses(t *testing.T, names ...string) {
	t.Helper()
	mm := &manifest.Manifest{Harnesses: names, Skills: map[string]manifest.Entry{}}
	if err := mm.Save(f.eng.Machine.ManifestPath); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) writeManifest(t *testing.T, m *manifest.Manifest) {
	t.Helper()
	if err := m.Save(f.scope.ManifestPath); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) readLock(t *testing.T) *lockfile.Lockfile {
	t.Helper()
	lf, err := lockfile.Load(f.scope.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

func kinds(p Plan) []ActionKind {
	var ks []ActionKind
	for _, a := range p.Actions {
		ks = append(ks, a.Kind)
	}
	return ks
}

const commitA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func pdfSource() *fakeSource {
	return &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"README.md":           "readme",
	}}
}

func pdfEntry() manifest.Entry {
	return manifest.Entry{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
}

func TestPlanSyncUnlockedEntryIsFetched(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})

	plan, err := f.eng.PlanSync(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != ActionFetch || plan.Actions[0].Name != "pdf" {
		t.Errorf("plan = %+v, want one fetch(pdf)", plan.Actions)
	}
}

func TestSyncInstallsLocksAndLinks(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# pdf" {
		t.Errorf("installed content = %q", data)
	}
	lf := f.readLock(t)
	s := lf.Find("pdf")
	if s == nil {
		t.Fatal("no lock entry")
	}
	if s.Commit != commitA {
		t.Errorf("lock commit = %q", s.Commit)
	}
	wantHash, _ := skill.Hash(f.scope.SkillDir("pdf"))
	if s.Hash != wantHash {
		t.Errorf("lock hash = %q, want %q", s.Hash, wantHash)
	}
	if got := f.adapter.links["pdf"]; got != f.scope.SkillDir("pdf") {
		t.Errorf("adapter link = %q", got)
	}
}

func TestSyncNeverReResolvesLockedEntries(t *testing.T) {
	src := pdfSource()
	count := 0
	src.resolved = &count
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("first sync resolved %d times", count)
	}
	// Delete the installed copy: the second sync reinstalls from the
	// locked commit without resolving again.
	if err := os.RemoveAll(f.scope.SkillDir("pdf")); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("locked entry re-resolved (%d calls)", count)
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("skill not reinstalled: %v", err)
	}
}

func TestSyncDriftWarnsSkipsAndFails(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	// Local modification.
	mod := filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")
	if err := os.WriteFile(mod, []byte("# hacked"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := f.eng.Sync(false)
	if err == nil {
		t.Fatal("want drift error")
	}
	data, _ := os.ReadFile(mod)
	if string(data) != "# hacked" {
		t.Error("drifted file was overwritten without --force")
	}
	if !strings.Contains(f.errOut.String(), "pdf") {
		t.Errorf("no drift warning printed:\n%s", f.errOut.String())
	}

	// --force restores the locked content.
	if err := f.eng.Sync(true); err != nil {
		t.Fatalf("forced sync: %v", err)
	}
	data, _ = os.ReadFile(mod)
	if string(data) != "# pdf" {
		t.Errorf("content after force = %q", data)
	}
}

func TestSyncDriftContinuesWithOtherSkills(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"skills/web/SKILL.md": "# web",
	}}
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: "https://github.com/o/r", Path: "skills/pdf"},
	}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	// Drift pdf, then add web to the manifest: sync must still install web.
	if err := os.WriteFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: "https://github.com/o/r", Path: "skills/pdf"},
		"web": {Source: "https://github.com/o/r", Path: "skills/web"},
	}})
	if err := f.eng.Sync(false); err == nil {
		t.Fatal("want drift error")
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("web"), "SKILL.md")); err != nil {
		t.Errorf("web not installed despite pdf drift: %v", err)
	}
}

func TestSyncPrunesRemovedEntries(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	// Remove from manifest.
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("canonical dir not pruned")
	}
	if f.readLock(t).Find("pdf") != nil {
		t.Error("lock entry not pruned")
	}
	if _, ok := f.adapter.links["pdf"]; ok {
		t.Error("adapter link not removed")
	}
}

func TestSyncRefusesToPruneDriftedSkill(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"), []byte("precious edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{}})

	if err := f.eng.Sync(false); err == nil {
		t.Fatal("want error when pruning drifted skill")
	}
	if _, err := os.Stat(f.scope.SkillDir("pdf")); err != nil {
		t.Error("drifted skill deleted without --force")
	}
	if f.readLock(t).Find("pdf") == nil {
		t.Error("lock entry removed despite refused prune")
	}

	// --force prunes it.
	if err := f.eng.Sync(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("drifted skill not pruned with --force")
	}
}

func TestSyncEditableSkippedByDriftCheck(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"my-skill": {Source: worktree, Path: "my-skill", Editable: true},
	}})

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	lf := f.readLock(t)
	s := lf.Find("my-skill")
	if s == nil || !s.Editable {
		t.Fatalf("lock entry = %+v", s)
	}
	if s.Commit != "" || s.Hash != "" {
		t.Errorf("editable entry must have no commit/hash: %+v", s)
	}
	// Canonical location is a symlink to the working tree.
	target, err := os.Readlink(f.scope.SkillDir("my-skill"))
	if err != nil {
		t.Fatal(err)
	}
	if target != skillDir {
		t.Errorf("canonical symlink -> %q, want %q", target, skillDir)
	}

	// Edits in the working tree never count as drift.
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# v2 live"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Sync(false); err != nil {
		t.Errorf("editable skill flagged as drift: %v", err)
	}
}

func TestPlanSyncKinds(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	// Everything in place: only a link ensure.
	plan, err := f.eng.PlanSync(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != ActionLink {
		t.Errorf("plan = %v, want [link]", kinds(plan))
	}

	// Missing on disk: materialize from lock.
	if err := os.RemoveAll(f.scope.SkillDir("pdf")); err != nil {
		t.Fatal(err)
	}
	plan, _ = f.eng.PlanSync(false)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != ActionMaterialize {
		t.Errorf("plan = %v, want [materialize]", kinds(plan))
	}
}

func TestAddGitSourceWithPath(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}

	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := m.Skills["pdf"]
	if !ok {
		t.Fatalf("manifest missing pdf: %+v", m.Skills)
	}
	if e.Source != "https://github.com/o/r" || e.Path != "skills/pdf" || e.Ref != "main" {
		t.Errorf("manifest entry = %+v", e)
	}
	s := f.readLock(t).Find("pdf")
	if s == nil || s.Commit != commitA || s.Hash == "" {
		t.Errorf("lock entry = %+v", s)
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")); err != nil {
		t.Errorf("not installed: %v", err)
	}
	if f.adapter.links["pdf"] == "" {
		t.Error("not linked")
	}
}

func TestAddSingleSkillRepoAutoDiscovers(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r"}

	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	m, _ := manifest.Load(f.scope.ManifestPath)
	e, ok := m.Skills["pdf"]
	if !ok {
		t.Fatalf("manifest = %+v", m.Skills)
	}
	if e.Path != "skills/pdf" {
		t.Errorf("discovered path = %q, want skills/pdf", e.Path)
	}
}

func TestAddMultiSkillRepoWithoutPathFails(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"skills/web/SKILL.md": "# web",
	}}
	f := newFixture(t, src)
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "main"}

	err := f.eng.Add(spec, false)
	if err == nil {
		t.Fatal("want error for multi-skill repo without //path")
	}
	var multi *MultipleSkillsError
	if !errors.As(err, &multi) {
		t.Fatalf("error type = %T: %v", err, err)
	}
	msg := err.Error()
	for _, want := range []string{
		"https://github.com/o/r//skills/pdf@main",
		"https://github.com/o/r//skills/web@main",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
	// Nothing was written.
	if _, err := os.Stat(f.scope.ManifestPath); !os.IsNotExist(err) {
		t.Error("manifest written despite error")
	}
}

// Issue #22: a source with a root skill and a nested one must suggest the
// root skill as `<src>//.`, never an unusable bare `<src>//`, and its
// Skills entry must be "." rather than "".
func TestAddMultiSkillRootSkillSuggestsDot(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"SKILL.md":            "# root",
		"skills/pdf/SKILL.md": "# pdf",
	}}
	f := newFixture(t, src)
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "main"}

	err := f.eng.Add(spec, false)
	var multi *MultipleSkillsError
	if !errors.As(err, &multi) {
		t.Fatalf("error type = %T: %v", err, err)
	}
	hasDot := false
	for _, s := range multi.Skills {
		if s == "" {
			t.Errorf("Skills contains an empty subpath: %q", multi.Skills)
		}
		if s == "." {
			hasDot = true
		}
	}
	if !hasDot {
		t.Errorf("Skills missing the root skill %q: %q", ".", multi.Skills)
	}
	msg := err.Error()
	if !strings.Contains(msg, "https://github.com/o/r//.@main") {
		t.Errorf("error missing root suggestion %q:\n%s", "https://github.com/o/r//.@main", msg)
	}
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "//") || strings.Contains(line, "//@") {
			t.Errorf("malformed empty-path suggestion:\n%s", msg)
		}
	}
}

// Issue #22 follow-through: picking the root skill (subpath ".") from an
// ambiguous source installs it.
func TestAddSelectedRootSubpathInstalls(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"SKILL.md":            "# root",
		"skills/pdf/SKILL.md": "# pdf",
	}}
	f := newFixture(t, src)
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "main"}

	if err := f.eng.AddSelected(spec, []string{"."}, false); err != nil {
		t.Fatal(err)
	}
	m, _ := manifest.Load(f.scope.ManifestPath)
	e, ok := m.Skills["r"]
	if !ok {
		t.Fatalf("manifest missing root skill: %+v", m.Skills)
	}
	if e.Path != "." {
		t.Errorf("path = %q, want .", e.Path)
	}
	if _, err := os.Stat(filepath.Join(f.scope.SkillDir("r"), "SKILL.md")); err != nil {
		t.Errorf("root skill not installed: %v", err)
	}
}

func TestAddEditablePathSource(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: worktree, IsPath: true}

	if err := f.eng.Add(spec, true); err != nil {
		t.Fatal(err)
	}
	m, _ := manifest.Load(f.scope.ManifestPath)
	e, ok := m.Skills["my-skill"]
	if !ok || !e.Editable || e.Path != "my-skill" {
		t.Fatalf("manifest entry = %+v ok=%v", e, ok)
	}
	s := f.readLock(t).Find("my-skill")
	if s == nil || !s.Editable || s.Commit != "" || s.Hash != "" {
		t.Errorf("lock entry = %+v", s)
	}
	target, err := os.Readlink(f.scope.SkillDir("my-skill"))
	if err != nil {
		t.Fatal(err)
	}
	if target != skillDir {
		t.Errorf("canonical -> %q, want %q", target, skillDir)
	}
	// Project-scope path source warns about portability.
	if !strings.Contains(f.errOut.String(), "warning") {
		t.Errorf("no portability warning:\n%s", f.errOut.String())
	}
}

func TestAddEditableMachineScopeNoWarning(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	f := newFixtureScope(t, pdfSource(), scope.Machine(home, filepath.Join(home, ".config")))
	spec := manifest.SourceSpec{Source: worktree, IsPath: true}

	if err := f.eng.Add(spec, true); err != nil {
		t.Fatal(err)
	}
	// Path sources are the expected case in machine scope: no portability warning.
	if strings.Contains(f.errOut.String(), "warning") {
		t.Errorf("machine scope must not warn about portability:\n%s", f.errOut.String())
	}
}

func TestAddEditableRequiresPathSource(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r"}
	if err := f.eng.Add(spec, true); err == nil {
		t.Error("want error: --editable with git source")
	}
}

func TestAddDuplicateNameFails(t *testing.T) {
	f := newFixture(t, pdfSource())
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Add(spec, false); err == nil {
		t.Error("want error for duplicate skill name")
	}
}

// Finding 1: a manifest edit (e.g. ref change) on a drifted skill must not
// silently overwrite the local modifications.
func TestSyncManifestChangeDoesNotClobberDrift(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md")
	if err := os.WriteFile(mod, []byte("# hacked"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Change the entry's ref: the entry no longer matches the lock.
	entry := pdfEntry()
	entry.Ref = "v2"
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": entry}})

	err := f.eng.Sync(false)
	if err == nil {
		t.Fatal("want drift error, got nil")
	}
	data, _ := os.ReadFile(mod)
	if string(data) != "# hacked" {
		t.Errorf("local modifications destroyed: content = %q", data)
	}
	if !strings.Contains(f.errOut.String(), "pdf") {
		t.Errorf("no drift warning printed:\n%s", f.errOut.String())
	}

	// --force re-resolves and installs the new entry.
	if err := f.eng.Sync(true); err != nil {
		t.Fatalf("forced sync: %v", err)
	}
	data, _ = os.ReadFile(mod)
	if string(data) != "# pdf" {
		t.Errorf("content after force = %q", data)
	}
	if s := f.readLock(t).Find("pdf"); s == nil || s.Ref != "v2" {
		t.Errorf("lock not updated: %+v", s)
	}
}

// Finding 2: turning a pinned entry into an editable one must be possible
// with --force (and refused, with a hint, without it).
func TestSyncPinnedToEditableTransition(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}

	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, "pdf")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# live"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"pdf": {Source: worktree, Path: "pdf", Editable: true},
	}})

	err := f.eng.Sync(false)
	if err == nil {
		t.Fatal("want error without --force")
	}
	if !strings.Contains(f.errOut.String(), "--force") {
		t.Errorf("error does not mention --force:\n%s", f.errOut.String())
	}
	if fi, statErr := os.Lstat(f.scope.SkillDir("pdf")); statErr != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Error("materialized copy replaced without --force")
	}

	if err := f.eng.Sync(true); err != nil {
		t.Fatalf("sync --force: %v", err)
	}
	target, linkErr := os.Readlink(f.scope.SkillDir("pdf"))
	if linkErr != nil {
		t.Fatalf("canonical is not a symlink after --force: %v", linkErr)
	}
	if target != skillDir {
		t.Errorf("canonical -> %q, want %q", target, skillDir)
	}
}

// Finding 3: an ambiguous manifest entry hit during sync must point at
// skiletto.toml, not print a malformed `skiletto add //...` suggestion.
func TestSyncMultiSkillEntryPointsAtManifest(t *testing.T) {
	src := &fakeSource{commit: commitA, tree: map[string]string{
		"skills/pdf/SKILL.md": "# pdf",
		"skills/web/SKILL.md": "# web",
	}}
	f := newFixture(t, src)
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{
		"tools": {Source: "https://github.com/o/r"},
	}})

	if err := f.eng.Sync(false); err == nil {
		t.Fatal("want error for ambiguous manifest entry")
	}
	out := f.errOut.String()
	for _, want := range []string{"skiletto.toml", `path = "skills/pdf"`, `path = "skills/web"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "skiletto add //") {
		t.Errorf("malformed add suggestion in stderr:\n%s", out)
	}
}

// Finding 4: a failed add must not leave an orphan materialized copy.
func TestAddFailureLeavesNoOrphan(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.eng.Adapters = []adapter.Adapter{failingAdapter{}}
	f.setMachineHarnesses(t, "failing")
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf"}

	if err := f.eng.Add(spec, false); err == nil {
		t.Fatal("want error from failing adapter")
	}
	if _, err := os.Lstat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("orphan materialized copy left at canonical location")
	}
	if _, err := os.Stat(f.scope.ManifestPath); !os.IsNotExist(err) {
		t.Error("manifest written despite failed add")
	}
}

func TestAddEditableFailureLeavesNoOrphan(t *testing.T) {
	worktree := t.TempDir()
	dir := filepath.Join(worktree, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pdfSource())
	f.eng.Adapters = []adapter.Adapter{failingAdapter{}}
	f.setMachineHarnesses(t, "failing")
	spec := manifest.SourceSpec{Source: worktree, IsPath: true}

	if err := f.eng.Add(spec, true); err == nil {
		t.Fatal("want error from failing adapter")
	}
	if _, err := os.Lstat(f.scope.SkillDir("my-skill")); !os.IsNotExist(err) {
		t.Error("orphan canonical symlink left behind")
	}
}

type failingAdapter struct{}

func (failingAdapter) Name() string                   { return "failing" }
func (failingAdapter) SkillsDir(s scope.Scope) string { return filepath.Join(s.Root, ".failing") }
func (failingAdapter) Link(s scope.Scope, name, target string, force bool) error {
	return fmt.Errorf("link refused")
}
func (failingAdapter) Unlink(s scope.Scope, name string, force bool) error { return nil }
func (failingAdapter) Detected(s scope.Scope) bool                         { return false }

// Finding 5: each sync failure is reported exactly once (in the streamed
// warnings, not repeated in the returned error).
func TestSyncFailurePrintedOnce(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.scope.SkillDir("pdf"), "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := f.eng.Sync(false)
	if err == nil {
		t.Fatal("want drift error")
	}
	if got := strings.Count(f.errOut.String(), "local modifications"); got != 1 {
		t.Errorf("drift message printed %d times on stderr:\n%s", got, f.errOut.String())
	}
	if strings.Contains(err.Error(), "local modifications") {
		t.Errorf("returned error repeats the streamed message: %q", err.Error())
	}
}
