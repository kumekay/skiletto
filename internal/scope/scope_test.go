package scope

import (
	"path/filepath"
	"testing"
)

func TestProjectPaths(t *testing.T) {
	s := Project("/repo")
	if s.Kind != KindProject {
		t.Errorf("Kind = %v, want KindProject", s.Kind)
	}
	if s.Root != "/repo" {
		t.Errorf("Root = %q", s.Root)
	}
	if got, want := s.ManifestPath, filepath.Join("/repo", "skiletto.toml"); got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
	if got, want := s.LockPath, filepath.Join("/repo", "skiletto.lock"); got != want {
		t.Errorf("LockPath = %q, want %q", got, want)
	}
	if got, want := s.SkillsDir, filepath.Join("/repo", ".agents", "skills"); got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
	if got, want := s.SkillDir("pdf"), filepath.Join("/repo", ".agents", "skills", "pdf"); got != want {
		t.Errorf("SkillDir = %q, want %q", got, want)
	}
}
