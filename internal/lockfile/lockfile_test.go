package lockfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	lf, err := Load(filepath.Join(t.TempDir(), "skiletto.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Version != 1 {
		t.Errorf("Version = %d, want 1", lf.Version)
	}
	if len(lf.Skills) != 0 {
		t.Errorf("want no skills, got %v", lf.Skills)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.lock")
	lf := &Lockfile{Version: 1, Skills: []Skill{
		{
			Name:   "pdf",
			Source: "https://github.com/anthropics/skills",
			Path:   "skills/pdf",
			Ref:    "main",
			Commit: "8c1f2ab90d3e4f56a7b8c9d0e1f2a3b4c5d6e7f8",
			Hash:   "sha256:abc",
		},
		{
			Name:     "my-skill",
			Source:   "/home/me/my-skills",
			Path:     "my-skill",
			Editable: true,
		},
	}}
	if err := lf.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, lf) {
		t.Errorf("round trip mismatch:\ngot  %#v\nwant %#v", got, lf)
	}
}

func TestSaveFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiletto.lock")
	lf := &Lockfile{Version: 1, Skills: []Skill{
		{Name: "pdf", Source: "https://github.com/o/r", Commit: "abc", Hash: "sha256:x"},
	}}
	if err := lf.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "version = 1") {
		t.Errorf("missing version:\n%s", content)
	}
	if !strings.Contains(content, "[[skill]]") {
		t.Errorf("missing [[skill]] table:\n%s", content)
	}
	if strings.Contains(content, "editable") {
		t.Errorf("editable should be omitted when false:\n%s", content)
	}
}

func TestFind(t *testing.T) {
	lf := &Lockfile{Version: 1, Skills: []Skill{{Name: "a"}, {Name: "b"}}}
	if s := lf.Find("b"); s == nil || s.Name != "b" {
		t.Errorf("Find(b) = %v", s)
	}
	if s := lf.Find("zzz"); s != nil {
		t.Errorf("Find(zzz) = %v, want nil", s)
	}
}

func TestUpsertAndRemove(t *testing.T) {
	lf := &Lockfile{Version: 1}
	lf.Upsert(Skill{Name: "a", Commit: "1"})
	lf.Upsert(Skill{Name: "b", Commit: "2"})
	lf.Upsert(Skill{Name: "a", Commit: "3"})
	if len(lf.Skills) != 2 {
		t.Fatalf("want 2 skills, got %v", lf.Skills)
	}
	if lf.Find("a").Commit != "3" {
		t.Errorf("upsert did not replace: %v", lf.Find("a"))
	}
	lf.Remove("a")
	if lf.Find("a") != nil || len(lf.Skills) != 1 {
		t.Errorf("remove failed: %v", lf.Skills)
	}
}
