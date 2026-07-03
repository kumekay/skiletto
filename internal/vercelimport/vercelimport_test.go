package vercelimport

import (
	"path/filepath"
	"testing"
)

func TestMapGithubEntries(t *testing.T) {
	lk := &Lock{Skills: map[string]Entry{
		"pdf": {Source: "anthropics/skills", SourceType: "github", SkillPath: "skills/pdf"},
		"web": {Source: "vercel/ai", SourceType: "github", Ref: "v2"},
	}}
	mapped, failures := lk.Map()
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %+v", failures)
	}
	// Sorted by name: pdf, web.
	want := []Mapped{
		{Name: "pdf", Source: "https://github.com/anthropics/skills", Path: "skills/pdf"},
		{Name: "web", Source: "https://github.com/vercel/ai", Ref: "v2"},
	}
	if len(mapped) != len(want) {
		t.Fatalf("mapped = %+v, want %+v", mapped, want)
	}
	for i := range want {
		if mapped[i] != want[i] {
			t.Errorf("mapped[%d] = %+v, want %+v", i, mapped[i], want[i])
		}
	}
}

func TestMapGitEntryUsesRawSource(t *testing.T) {
	lk := &Lock{Skills: map[string]Entry{
		"a": {Source: "https://gitea.example.com/me/skills.git", SourceType: "git", SkillPath: "deploy"},
		"b": {SourceType: "git", SourceURL: "ssh://git@host/x.git"},
	}}
	mapped, failures := lk.Map()
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %+v", failures)
	}
	if mapped[0].Source != "https://gitea.example.com/me/skills.git" || mapped[0].Path != "deploy" {
		t.Errorf("git entry a = %+v", mapped[0])
	}
	// source empty falls back to sourceUrl.
	if mapped[1].Source != "ssh://git@host/x.git" {
		t.Errorf("git entry b source = %q, want the sourceUrl", mapped[1].Source)
	}
}

func TestMapUnsupportedSourceTypesFail(t *testing.T) {
	lk := &Lock{Skills: map[string]Entry{
		"local-one": {Source: "/x", SourceType: "local"},
		"nm":        {Source: "pkg", SourceType: "node_modules"},
		"wk":        {Source: "x", SourceType: "well-known"},
		"empty":     {Source: "x"},
		"bad-gh":    {Source: "not-owner-repo", SourceType: "github"},
		"git-nourl": {SourceType: "git"},
	}}
	mapped, failures := lk.Map()
	if len(mapped) != 0 {
		t.Fatalf("nothing should map: %+v", mapped)
	}
	if len(failures) != 6 {
		t.Fatalf("want 6 failures, got %d: %+v", len(failures), failures)
	}
	// Failures carry the skill name and a non-empty reason, sorted by name.
	for _, f := range failures {
		if f.Reason == "" {
			t.Errorf("failure %q has no reason", f.Name)
		}
	}
	if failures[0].Name != "bad-gh" {
		t.Errorf("failures not sorted by name: %+v", failures)
	}
}

func TestReadMissingFileGuidance(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "skills-lock.json"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
	if got := err.Error(); !containsAll(got, "not found", "skiletto import") {
		t.Errorf("error lacks guidance: %q", got)
	}
}

func TestReadRealFixture(t *testing.T) {
	lk, err := Read(filepath.Join("testdata", "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	mapped, failures := lk.Map()
	if len(failures) != 0 {
		t.Fatalf("real fixture produced failures: %+v", failures)
	}
	// The real fixture is all github owner/repo entries.
	byName := map[string]Mapped{}
	for _, m := range mapped {
		byName[m.Name] = m
	}
	got, ok := byName["ai-sdk"]
	if !ok {
		t.Fatalf("ai-sdk missing from %+v", mapped)
	}
	if got.Source != "https://github.com/vercel/ai" {
		t.Errorf("ai-sdk source = %q", got.Source)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
