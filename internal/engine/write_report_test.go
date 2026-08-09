package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
)

func pdfSpec() manifest.SourceSpec {
	return manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
}

func TestAddReportsWrittenFiles(t *testing.T) {
	f := newFixture(t, pdfSource())
	if err := f.eng.Add(pdfSpec(), false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"wrote " + f.scope.ManifestPath,
		"wrote " + f.scope.LockPath,
	} {
		if !strings.Contains(f.errOut.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, f.errOut.String())
		}
	}
}

func TestSyncReportsWrittenLockOnce(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.errOut.String(), "wrote "+f.scope.LockPath) {
		t.Errorf("stderr missing lock write report:\n%s", f.errOut.String())
	}

	// A second sync changes nothing: no writes, no reports.
	f.errOut.Reset()
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.errOut.String(), "wrote ") {
		t.Errorf("idle sync reported writes:\n%s", f.errOut.String())
	}
}

func TestRemoveReportsWrittenFiles(t *testing.T) {
	f := newFixture(t, pdfSource())
	if err := f.eng.Add(pdfSpec(), false); err != nil {
		t.Fatal(err)
	}
	f.errOut.Reset()
	if err := f.eng.Remove("pdf", false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"wrote " + f.scope.ManifestPath,
		"wrote " + f.scope.LockPath,
	} {
		if !strings.Contains(f.errOut.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, f.errOut.String())
		}
	}
}

func TestHarnessEnableReportsWrittenManifest(t *testing.T) {
	f := newFixture(t, pdfSource())
	if err := f.eng.HarnessEnable([]string{"fake"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.errOut.String(), "wrote "+f.scope.ManifestPath) {
		t.Errorf("stderr missing manifest write report:\n%s", f.errOut.String())
	}
}

// Project-scope writes stay absolute even when the project lives under
// the home dir: the ~/... shorthand is reserved for the machine scope, so
// scripts can rely on the project paths they passed in.
func TestWriteReportKeepsProjectPathsAbsolute(t *testing.T) {
	f := newFixture(t, pdfSource())
	home := f.eng.Machine.Root
	f.eng.Scope = scope.Project(filepath.Join(home, "proj"))
	if err := f.eng.Add(pdfSpec(), false); err != nil {
		t.Fatal(err)
	}
	want := "wrote " + filepath.Join(home, "proj", "skiletto.toml")
	if !strings.Contains(f.errOut.String(), want) {
		t.Errorf("stderr missing %q:\n%s", want, f.errOut.String())
	}
	if strings.Contains(f.errOut.String(), "wrote ~") {
		t.Errorf("project write report was shortened to ~:\n%s", f.errOut.String())
	}
}

// Machine-scope writes show a home-relative path, matching the ~/.config
// convention users sync with dotfile managers.
func TestWriteReportShortensHomeToTilde(t *testing.T) {
	home := t.TempDir()
	sc := scope.Machine(home, filepath.Join(home, ".config"))
	f := newFixtureScope(t, pdfSource(), sc)
	if err := f.eng.Add(pdfSpec(), false); err != nil {
		t.Fatal(err)
	}
	want := "wrote " + filepath.Join("~", ".config", "skiletto", "skiletto.toml")
	if !strings.Contains(f.errOut.String(), want) {
		t.Errorf("stderr missing %q:\n%s", want, f.errOut.String())
	}
	if strings.Contains(f.errOut.String(), home) {
		t.Errorf("write report leaks the absolute home dir:\n%s", f.errOut.String())
	}
}
