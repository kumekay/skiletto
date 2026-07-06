package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
)

// namedAdapter is a fakeAdapter with a configurable name.
type namedAdapter struct {
	fakeAdapter
	name string
}

func newNamedAdapter(name string) *namedAdapter {
	return &namedAdapter{fakeAdapter: *newFakeAdapter(), name: name}
}

func (a *namedAdapter) Name() string { return a.name }

func oneSkillSource() *fakeSource {
	return &fakeSource{commit: commitA, tree: map[string]string{
		"SKILL.md": "# pdf",
	}}
}

func oneSkillManifest(harnesses []string) *manifest.Manifest {
	return &manifest.Manifest{
		Harnesses: harnesses,
		Skills: map[string]manifest.Entry{
			"pdf": {Source: "https://example.com/r", Ref: "main"},
		},
	}
}

// noMachine strips the fixture's machine scope so the project manifest is
// the only harness configuration in play.
func noMachine(f *fixture) *fixture {
	f.eng.Machine = nil
	return f
}

func TestSyncUnconfiguredLinksNothingAndNotes(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest(nil))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(f.adapter.links) != 0 {
		t.Errorf("unconfigured scope must not link, got %v", f.adapter.links)
	}
	if fi, err := os.Stat(f.scope.SkillDir("pdf")); err != nil || !fi.IsDir() {
		t.Errorf("canonical install must still happen: %v", err)
	}
	if !strings.Contains(f.out.String(), "no harnesses configured") {
		t.Errorf("want canonical-only note, got out=%q", f.out.String())
	}
}

func TestSyncConfiguredEmptyLinksNothingWithoutNote(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest([]string{}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(f.adapter.links) != 0 {
		t.Errorf("explicit empty harnesses must not link, got %v", f.adapter.links)
	}
	if strings.Contains(f.out.String(), "no harnesses configured") {
		t.Errorf("explicit empty list is configured; no note expected, got %q", f.out.String())
	}
}

func TestMachineHarnessesApplyToProjectScope(t *testing.T) {
	// The default fixture machine scope enables "fake"; the project
	// manifest leaves the key absent.
	f := newFixture(t, oneSkillSource())
	f.writeManifest(t, oneSkillManifest(nil))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, ok := f.adapter.links["pdf"]; !ok {
		t.Errorf("machine-enabled harness must link in project scope, links=%v", f.adapter.links)
	}
}

func TestProjectAndMachineHarnessesUnion(t *testing.T) {
	f := newFixture(t, oneSkillSource())
	other := newNamedAdapter("other")
	f.eng.Adapters = append(f.eng.Adapters, other)
	// machine enables "fake" (fixture default); project enables "other".
	f.writeManifest(t, oneSkillManifest([]string{"other"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, ok := f.adapter.links["pdf"]; !ok {
		t.Errorf("machine-enabled harness missing from union, links=%v", f.adapter.links)
	}
	if _, ok := other.links["pdf"]; !ok {
		t.Errorf("project-enabled harness missing from union, links=%v", other.links)
	}
}

func TestUnknownHarnessWarnsAndContinues(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest([]string{"fake", "zed"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, ok := f.adapter.links["pdf"]; !ok {
		t.Errorf("known harness must still link, links=%v", f.adapter.links)
	}
	if !strings.Contains(f.errOut.String(), `unknown harness "zed"`) {
		t.Errorf("want unknown-harness warning, got %q", f.errOut.String())
	}
}

func TestSyncPrunesLinksOfDisabledHarness(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	other := newNamedAdapter("other")
	f.eng.Adapters = append(f.eng.Adapters, other)
	f.writeManifest(t, oneSkillManifest([]string{"fake", "other"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if _, ok := other.links["pdf"]; !ok {
		t.Fatalf("setup: other not linked")
	}
	f.writeManifest(t, oneSkillManifest([]string{"fake"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if _, ok := other.links["pdf"]; ok {
		t.Errorf("disabled harness link must be pruned, links=%v", other.links)
	}
	if _, ok := f.adapter.links["pdf"]; !ok {
		t.Errorf("enabled harness must stay linked, links=%v", f.adapter.links)
	}
}

func TestPromptOncePersistsChoice(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest(nil))
	calls := 0
	f.eng.PromptHarnesses = func(opts []HarnessOption) ([]string, error) {
		calls++
		names := make([]string, len(opts))
		for i, o := range opts {
			names[i] = o.Name
		}
		if len(names) != 1 || names[0] != "fake" {
			t.Errorf("prompt options = %v, want [fake]", names)
		}
		return []string{"fake"}, nil
	}
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if calls != 1 {
		t.Fatalf("prompt calls = %d, want 1", calls)
	}
	if _, ok := f.adapter.links["pdf"]; !ok {
		t.Errorf("chosen harness must link, links=%v", f.adapter.links)
	}
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Harnesses) != 1 || m.Harnesses[0] != "fake" {
		t.Errorf("choice must persist to the manifest, got %#v", m.Harnesses)
	}
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if calls != 1 {
		t.Errorf("configured scope must not prompt again, calls = %d", calls)
	}
}

func TestPromptNoneSelectedPersistsExplicitEmpty(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest(nil))
	f.eng.PromptHarnesses = func([]HarnessOption) ([]string, error) {
		return nil, nil
	}
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(f.adapter.links) != 0 {
		t.Errorf("none selected must not link, got %v", f.adapter.links)
	}
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.Harnesses == nil || len(m.Harnesses) != 0 {
		t.Errorf("empty choice must persist as harnesses = [], got %#v", m.Harnesses)
	}
}

func TestHarnessEnableLinksInstalledSkills(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	other := newNamedAdapter("other")
	f.eng.Adapters = append(f.eng.Adapters, other)
	f.writeManifest(t, oneSkillManifest([]string{"fake"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := f.eng.HarnessEnable([]string{"other"}, false); err != nil {
		t.Fatalf("HarnessEnable: %v", err)
	}
	if _, ok := other.links["pdf"]; !ok {
		t.Errorf("enable must link installed skills, links=%v", other.links)
	}
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Harnesses) != 2 || m.Harnesses[0] != "fake" || m.Harnesses[1] != "other" {
		t.Errorf("manifest harnesses = %#v, want [fake other]", m.Harnesses)
	}
}

func TestHarnessEnableUnknownNameFails(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	err := f.eng.HarnessEnable([]string{"nope"}, false)
	if err == nil || !strings.Contains(err.Error(), `unknown harness "nope"`) {
		t.Errorf("want unknown-harness error, got %v", err)
	}
}

func TestHarnessDisableUnlinksAndUpdatesManifest(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest([]string{"fake"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := f.eng.HarnessDisable([]string{"fake"}, false); err != nil {
		t.Fatalf("HarnessDisable: %v", err)
	}
	if len(f.adapter.links) != 0 {
		t.Errorf("disable must unlink, got %v", f.adapter.links)
	}
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.Harnesses == nil || len(m.Harnesses) != 0 {
		t.Errorf("manifest harnesses = %#v, want explicit empty", m.Harnesses)
	}
}

func TestHarnessDisableNotEnabledInScope(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	f.writeManifest(t, oneSkillManifest([]string{}))
	err := f.eng.HarnessDisable([]string{"fake"}, false)
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("want not-enabled error, got %v", err)
	}
}

func TestHarnessDisableWarnsWhenStillMachineEnabled(t *testing.T) {
	f := newFixture(t, oneSkillSource()) // machine enables "fake"
	f.writeManifest(t, oneSkillManifest([]string{"fake"}))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := f.eng.HarnessDisable([]string{"fake"}, false); err != nil {
		t.Fatalf("HarnessDisable: %v", err)
	}
	if !strings.Contains(f.errOut.String(), "still enabled machine-wide") {
		t.Errorf("want machine-wide warning, got %q", f.errOut.String())
	}
}

func TestHarnessListShowsState(t *testing.T) {
	f := newFixture(t, oneSkillSource()) // machine enables "fake"
	other := newNamedAdapter("other")
	f.eng.Adapters = append(f.eng.Adapters, other)
	f.writeManifest(t, oneSkillManifest([]string{"other"}))
	if err := f.eng.HarnessList(); err != nil {
		t.Fatalf("HarnessList: %v", err)
	}
	out := f.out.String()
	for _, want := range []string{"fake", "machine", "other", "project"} {
		if !strings.Contains(out, want) {
			t.Errorf("HarnessList output missing %q:\n%s", want, out)
		}
	}
}

func TestAddUnconfiguredInstallsCanonicalOnly(t *testing.T) {
	f := noMachine(newFixture(t, oneSkillSource()))
	spec := manifest.SourceSpec{Source: "https://example.com/r", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(f.adapter.links) != 0 {
		t.Errorf("unconfigured add must not link, got %v", f.adapter.links)
	}
	if !strings.Contains(f.out.String(), "no harnesses configured") {
		t.Errorf("want canonical-only note, got %q", f.out.String())
	}
	// The fallback must not write a harnesses key: a later interactive
	// run should still get the one-time prompt.
	m, err := manifest.Load(f.scope.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.Harnesses != nil {
		t.Errorf("fallback must not persist harnesses, got %#v", m.Harnesses)
	}
}

func TestHarnessOptionsReportDetection(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	msc := scope.Machine(home, filepath.Join(home, ".config"))
	f := newFixture(t, oneSkillSource())
	f.eng.Machine = &msc
	var got []HarnessOption
	f.eng.PromptHarnesses = func(opts []HarnessOption) ([]string, error) {
		got = opts
		return []string{"fake"}, nil
	}
	f.writeManifest(t, oneSkillManifest(nil))
	if err := f.eng.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(got) != 1 || got[0].Name != "fake" || !got[0].Detected {
		t.Errorf("options = %#v, want fake detected", got)
	}
}

var _ adapter.Adapter = (*namedAdapter)(nil)
