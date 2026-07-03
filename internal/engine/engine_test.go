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

func (a *fakeAdapter) Link(s scope.Scope, name, target string) error {
	a.links[name] = target
	return nil
}

func (a *fakeAdapter) Unlink(s scope.Scope, name string) error {
	delete(a.links, name)
	return nil
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
	sc := scope.Project(t.TempDir())
	ad := newFakeAdapter()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	eng := &Engine{
		Scope:    sc,
		Adapters: []adapter.Adapter{ad},
		NewSource: func(s string) (source.Source, error) {
			return src, nil
		},
		Out: out,
		Err: errOut,
	}
	return &fixture{eng: eng, scope: sc, src: src, adapter: ad, out: out, errOut: errOut}
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
