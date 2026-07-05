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
