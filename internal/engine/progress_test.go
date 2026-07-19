package engine

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/kumekay/skiletto/internal/manifest"
)

// recordingProgress records progress events in order.
type recordingProgress struct {
	events []string
}

func (r *recordingProgress) Step(name, stage string) {
	r.events = append(r.events, fmt.Sprintf("step %s %s", name, stage))
}

func (r *recordingProgress) Done(name, result string) {
	r.events = append(r.events, fmt.Sprintf("done %s %s", name, result))
}

func (r *recordingProgress) Clear() {
	r.events = append(r.events, "clear")
}

func TestSyncFetchReportsProgress(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	rec := &recordingProgress{}
	f.eng.Progress = rec

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	want := []string{"step pdf resolving", "step pdf fetching", "done pdf installed", "clear"}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %v, want %v", rec.events, want)
	}
}

func TestSyncMaterializeReportsProgress(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(f.scope.SkillDir("pdf")); err != nil {
		t.Fatal(err)
	}
	rec := &recordingProgress{}
	f.eng.Progress = rec

	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	want := []string{"step pdf fetching", "done pdf installed", "clear"}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %v, want %v", rec.events, want)
	}
}

func TestUpdateReportsProgress(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	rec := &recordingProgress{}
	f.eng.Progress = rec

	if err := f.eng.Update("", false); err != nil {
		t.Fatal(err)
	}
	want := []string{"step pdf resolving", "step pdf fetching", "done pdf installed", "clear"}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %v, want %v", rec.events, want)
	}
}

func TestAddReportsProgress(t *testing.T) {
	f := newFixture(t, pdfSource())
	rec := &recordingProgress{}
	f.eng.Progress = rec

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "skills/pdf", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"step skills/pdf resolving", "step skills/pdf fetching", "clear"}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %v, want %v", rec.events, want)
	}
}

func TestAddWithoutPathLabelsProgressWithSource(t *testing.T) {
	f := newFixture(t, pdfSource())
	rec := &recordingProgress{}
	f.eng.Progress = rec

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "main"}
	if err := f.eng.Add(spec, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"step https://github.com/o/r resolving", "step https://github.com/o/r fetching", "clear"}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %v, want %v", rec.events, want)
	}
}

func TestAddAllReportsDiscoveryProgress(t *testing.T) {
	f := newFixture(t, pdfSource())
	rec := &recordingProgress{}
	f.eng.Progress = rec

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "main"}
	if err := f.eng.AddAll(spec, false); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"step https://github.com/o/r resolving",
		"step https://github.com/o/r fetching",
		"step skills/pdf resolving",
		"step skills/pdf fetching",
		"clear",
	}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events = %v, want %v", rec.events, want)
	}
}

func TestAddAllClearsProgressBeforeHarnessPrompt(t *testing.T) {
	f := newFixture(t, pdfSource())
	// No harnesses key anywhere: the one-time interactive picker fires.
	mm := &manifest.Manifest{Skills: map[string]manifest.Entry{}}
	if err := mm.Save(f.eng.Machine.ManifestPath); err != nil {
		t.Fatal(err)
	}
	rec := &recordingProgress{}
	f.eng.Progress = rec
	var atPrompt []string
	f.eng.PromptHarnesses = func([]HarnessOption) ([]string, error) {
		atPrompt = append([]string(nil), rec.events...)
		return []string{"fake"}, nil
	}

	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "main"}
	if err := f.eng.AddAll(spec, false); err != nil {
		t.Fatal(err)
	}
	if len(atPrompt) == 0 || atPrompt[len(atPrompt)-1] != "clear" {
		t.Errorf("progress events when the picker opened = %v, want a trailing clear", atPrompt)
	}
}

func TestNilProgressIsSilent(t *testing.T) {
	f := newFixture(t, pdfSource())
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	if got := f.errOut.String(); got != "" {
		t.Errorf("stderr = %q, want empty without a progress renderer", got)
	}
}
