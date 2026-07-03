package skill

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFindsSkillDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "pdf", "SKILL.md"), "# pdf")
	writeFile(t, filepath.Join(root, "skills", "web", "SKILL.md"), "# web")
	writeFile(t, filepath.Join(root, "skills", "web", "helper.py"), "pass")
	writeFile(t, filepath.Join(root, "README.md"), "readme")

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"skills/pdf", "skills/web"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v, want %v", got, want)
	}
}

func TestDiscoverRootSkill(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), "# root skill")

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v, want %v", got, want)
	}
}

func TestDiscoverSkipsGitDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "sub", "SKILL.md"), "not a skill")
	writeFile(t, filepath.Join(root, "real", "SKILL.md"), "# real")

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"real"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v, want %v", got, want)
	}
}

func TestDiscoverNoSkills(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "nothing here")

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("Discover = %v, want empty", got)
	}
}

func TestHashDeterministic(t *testing.T) {
	a := t.TempDir()
	writeFile(t, filepath.Join(a, "SKILL.md"), "# s")
	writeFile(t, filepath.Join(a, "sub", "x.py"), "print(1)")

	b := t.TempDir()
	writeFile(t, filepath.Join(b, "sub", "x.py"), "print(1)")
	writeFile(t, filepath.Join(b, "SKILL.md"), "# s")

	ha, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := Hash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("hashes differ for identical trees: %s vs %s", ha, hb)
	}
	if !strings.HasPrefix(ha, "sha256:") {
		t.Errorf("hash %q missing sha256: prefix", ha)
	}
}

func TestHashSensitiveToContentAndPath(t *testing.T) {
	a := t.TempDir()
	writeFile(t, filepath.Join(a, "SKILL.md"), "# s")

	b := t.TempDir()
	writeFile(t, filepath.Join(b, "SKILL.md"), "# t")

	c := t.TempDir()
	writeFile(t, filepath.Join(c, "OTHER.md"), "# s")

	ha, _ := Hash(a)
	hb, _ := Hash(b)
	hc, _ := Hash(c)
	if ha == hb {
		t.Error("hash ignores file content")
	}
	if ha == hc {
		t.Error("hash ignores file path")
	}
}

func TestDefaultName(t *testing.T) {
	cases := []struct {
		source, path, want string
	}{
		{"https://github.com/anthropics/skills", "skills/pdf", "pdf"},
		{"https://github.com/anthropics/skills.git", "", "skills"},
		{"https://github.com/anthropics/skills", ".", "skills"},
		{"/home/me/my-skills", "my-skill", "my-skill"},
	}
	for _, c := range cases {
		if got := DefaultName(c.source, c.path); got != c.want {
			t.Errorf("DefaultName(%q, %q) = %q, want %q", c.source, c.path, got, c.want)
		}
	}
}
