package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "skiletto.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Skills) != 0 {
		t.Errorf("want empty manifest, got %v", m.Skills)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{Skills: map[string]Entry{
		"pdf":      {Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main"},
		"deploy":   {Source: "ssh://gitea@git.kumekay.com:30009/ku/skills.git", Path: "deploy"},
		"my-skill": {Source: "~/p/my-skills", Path: "my-skill", Editable: true},
	}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Skills, m.Skills) {
		t.Errorf("round trip mismatch:\ngot  %#v\nwant %#v", got.Skills, m.Skills)
	}
}

func TestSaveWritesInlineTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{Skills: map[string]Entry{
		"pdf": {Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main"},
	}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "[skills]") {
		t.Errorf("missing [skills] table:\n%s", content)
	}
	want := `pdf = { source = "https://github.com/anthropics/skills", path = "skills/pdf", ref = "main" }`
	if !strings.Contains(content, want) {
		t.Errorf("missing inline entry %q:\n%s", want, content)
	}
}

func TestSaveOmitsEmptyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{Skills: map[string]Entry{
		"whole-repo": {Source: "https://github.com/o/r"},
	}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	for _, forbidden := range []string{"path", "ref", "editable"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("unexpected %q in output:\n%s", forbidden, content)
		}
	}
}

func TestLoadHarnessesAbsentIsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	if err := os.WriteFile(path, []byte("[skills]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Harnesses != nil {
		t.Errorf("want nil Harnesses for absent key, got %#v", m.Harnesses)
	}
}

func TestLoadHarnessesEmptyIsSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	if err := os.WriteFile(path, []byte("harnesses = []\n\n[skills]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Harnesses == nil {
		t.Error("want non-nil Harnesses for explicit empty list")
	}
	if len(m.Harnesses) != 0 {
		t.Errorf("want empty Harnesses, got %#v", m.Harnesses)
	}
}

func TestHarnessesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{
		Harnesses: []string{"claude", "goose"},
		Skills: map[string]Entry{
			"pdf": {Source: "https://github.com/anthropics/skills", Path: "skills/pdf"},
		},
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Harnesses, m.Harnesses) {
		t.Errorf("Harnesses = %#v, want %#v", got.Harnesses, m.Harnesses)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	want := `harnesses = ["claude", "goose"]`
	if !strings.Contains(content, want) {
		t.Errorf("missing %q in output:\n%s", want, content)
	}
	if strings.Index(content, "harnesses") > strings.Index(content, "[skills]") {
		t.Errorf("harnesses key must precede the [skills] table:\n%s", content)
	}
}

func TestSaveOmitsNilHarnesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{Skills: map[string]Entry{}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "harnesses") {
		t.Errorf("nil Harnesses must not be written:\n%s", data)
	}
}

func TestSaveKeepsExplicitEmptyHarnesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{Harnesses: []string{}, Skills: map[string]Entry{}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Harnesses == nil || len(got.Harnesses) != 0 {
		t.Errorf("want explicit empty Harnesses to survive a round trip, got %#v", got.Harnesses)
	}
}

func TestParseSourceSpec(t *testing.T) {
	cases := []struct {
		spec string
		want SourceSpec
	}{
		{
			"github.com/anthropics/skills//skills/pdf@main",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main"},
		},
		{
			"anthropics/skills",
			SourceSpec{Source: "https://github.com/anthropics/skills"},
		},
		{
			"anthropics/skills//skills/pdf",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf"},
		},
		{
			"anthropics/skills@v1.2",
			SourceSpec{Source: "https://github.com/anthropics/skills", Ref: "v1.2"},
		},
		{
			"https://github.com/anthropics/skills//skills/pdf@main",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main"},
		},
		{
			"ssh://gitea@git.kumekay.com:30009/ku/skills.git//deploy@main",
			SourceSpec{Source: "ssh://gitea@git.kumekay.com:30009/ku/skills.git", Path: "deploy", Ref: "main"},
		},
		{
			"ssh://gitea@git.kumekay.com:30009/ku/skills.git",
			SourceSpec{Source: "ssh://gitea@git.kumekay.com:30009/ku/skills.git"},
		},
		{
			"git@github.com:anthropics/skills.git//skills/pdf",
			SourceSpec{Source: "git@github.com:anthropics/skills.git", Path: "skills/pdf"},
		},
		{
			"https://github.com/anthropics/skills/tree/main/skills/pdf",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main", TreeURL: true},
		},
		{
			"https://github.com/anthropics/skills/tree/main",
			SourceSpec{Source: "https://github.com/anthropics/skills", Ref: "main", TreeURL: true},
		},
		{
			"https://github.com/anthropics/skills/tree/main/skills/pdf/",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main", TreeURL: true},
		},
		{
			"github.com/anthropics/skills/tree/main/skills/pdf",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main", TreeURL: true},
		},
		{
			// Only github.com gets /tree/ normalization; other hosts keep
			// their URL untouched.
			"https://example.com/anthropics/skills/tree/main/skills/pdf",
			SourceSpec{Source: "https://example.com/anthropics/skills/tree/main/skills/pdf"},
		},
		{
			// A repo path that merely ends in /tree is not a browser URL.
			"https://github.com/anthropics/tree",
			SourceSpec{Source: "https://github.com/anthropics/tree"},
		},
		{
			// @ inside a path segment is not a ref separator in browser URLs.
			"https://github.com/anthropics/skills/tree/main/skills/@scope/pkg",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/@scope/pkg", Ref: "main", TreeURL: true},
		},
		{
			// Nor is a final segment that starts with @.
			"https://github.com/anthropics/skills/tree/main/skills/@scope",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/@scope", Ref: "main", TreeURL: true},
		},
		{
			// A /blob/ URL (a pasted SKILL.md link) maps to the file's directory.
			"https://github.com/anthropics/skills/blob/main/skills/pdf/SKILL.md",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main", TreeURL: true},
		},
		{
			// A root-level /blob/ file pins the repo root: explicit ".",
			// since "" would mean whole-source discovery.
			"https://github.com/anthropics/skills/blob/main/SKILL.md",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: ".", Ref: "main", TreeURL: true},
		},
		{
			// Fragment and query residue from the browser is dropped.
			"https://github.com/anthropics/skills/tree/main/skills/pdf#readme",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main", TreeURL: true},
		},
		{
			"https://github.com/anthropics/skills/blob/main/skills/pdf/SKILL.md?plain=1",
			SourceSpec{Source: "https://github.com/anthropics/skills", Path: "skills/pdf", Ref: "main", TreeURL: true},
		},
		{
			"./my-skills//my-skill",
			SourceSpec{Source: "./my-skills", Path: "my-skill", IsPath: true},
		},
		{
			"/abs/path/skills@main",
			SourceSpec{Source: "/abs/path/skills", Ref: "main", IsPath: true},
		},
		{
			"~/p/my-skills",
			SourceSpec{Source: "~/p/my-skills", IsPath: true},
		},
		{
			"../relative",
			SourceSpec{Source: "../relative", IsPath: true},
		},
		{
			`C:\Users\me\skills//my-skill`,
			SourceSpec{Source: `C:\Users\me\skills`, Path: "my-skill", IsPath: true},
		},
		{
			"C:/Users/me/skills//my-skill",
			SourceSpec{Source: "C:/Users/me/skills", Path: "my-skill", IsPath: true},
		},
	}
	for _, c := range cases {
		got, err := ParseSourceSpec(c.spec)
		if err != nil {
			t.Errorf("ParseSourceSpec(%q): %v", c.spec, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSourceSpec(%q) = %+v, want %+v", c.spec, got, c.want)
		}
	}
}

func TestParseSourceSpecRejectsEmpty(t *testing.T) {
	if _, err := ParseSourceSpec(""); err == nil {
		t.Error("want error for empty spec")
	}
}

// A /tree/ URL already carries a ref and a path; combining it with an
// explicit @ref or //path is contradictory and must be rejected, not
// guessed at.
func TestParseSourceSpecTreeURLConflicts(t *testing.T) {
	for _, spec := range []string{
		"https://github.com/anthropics/skills/tree/main/skills/pdf@v2",
		"https://github.com/anthropics/skills/tree/main//skills/pdf",
	} {
		if _, err := ParseSourceSpec(spec); err == nil {
			t.Errorf("ParseSourceSpec(%q): want error for /tree/ URL combined with @ref or //path", spec)
		}
	}
}

func TestLoadHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	content := "[hooks]\npre-install = \"skillspector scan --no-llm\"\n\n[skills]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Hooks["pre-install"]; got != "skillspector scan --no-llm" {
		t.Errorf("pre-install hook = %q, want %q", got, "skillspector scan --no-llm")
	}
}

// Unknown hook names must not fail parsing: a manifest written by a newer
// skiletto would otherwise brick every command in the scope. They are
// validated where hooks are consulted (the engine), so installs still fail
// on a typo.
func TestLoadKeepsUnknownHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	content := "[hooks]\npost-install = \"echo done\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Hooks["post-install"]; got != "echo done" {
		t.Errorf("post-install hook = %q, want %q", got, "echo done")
	}
}

func TestHooksRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{
		Hooks:  map[string]string{"pre-install": "skillspector scan"},
		Skills: map[string]Entry{"pdf": {Source: "https://github.com/anthropics/skills", Path: "skills/pdf"}},
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Hooks, m.Hooks) {
		t.Errorf("hooks round trip mismatch:\ngot  %#v\nwant %#v", got.Hooks, m.Hooks)
	}
	if !reflect.DeepEqual(got.Skills, m.Skills) {
		t.Errorf("skills round trip mismatch:\ngot  %#v\nwant %#v", got.Skills, m.Skills)
	}
}

func TestSaveOmitsEmptyHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.toml")
	m := &Manifest{Skills: map[string]Entry{}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "[hooks]") {
		t.Errorf("empty hooks table written:\n%s", data)
	}
}
