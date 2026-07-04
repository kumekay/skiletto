package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/skill"
	"github.com/kumekay/skiletto/internal/source"
)

// copyAdapter mimics the Windows copy-link fallback on any OS: Link copies
// the canonical tree into the adapter dir, Unlink refuses a copy that no
// longer matches the canonical tree unless forced — the same contract the
// real link helper implements with the copy strategy. It lets the engine's
// unlink-before-promote ordering and force threading be tested everywhere.
type copyAdapter struct{}

func (copyAdapter) Name() string                   { return "copy" }
func (copyAdapter) SkillsDir(s scope.Scope) string { return filepath.Join(s.Root, ".copy") }

func (a copyAdapter) Link(s scope.Scope, name, target string, force bool) error {
	dst := filepath.Join(a.SkillsDir(s), name)
	if _, err := os.Lstat(dst); err == nil {
		if !force && !treesEqual(dst, target) {
			return fmt.Errorf("%s diverged from the canonical tree; re-run with --force", dst)
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	return copyTestTree(target, dst)
}

func (a copyAdapter) Unlink(s scope.Scope, name string, force bool) error {
	dst := filepath.Join(a.SkillsDir(s), name)
	if _, err := os.Lstat(dst); os.IsNotExist(err) {
		return nil
	}
	if !force && !treesEqual(dst, s.SkillDir(name)) {
		return fmt.Errorf("%s diverged from the canonical tree; re-run with --force", dst)
	}
	return os.RemoveAll(dst)
}

func treesEqual(a, b string) bool {
	ha, err := skill.Hash(a)
	if err != nil {
		return false
	}
	hb, err := skill.Hash(b)
	if err != nil {
		return false
	}
	return ha == hb
}

func copyTestTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyTestTree(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyFixture is a fixture whose only adapter copy-links.
func copyFixture(t *testing.T) (*fixture, copyAdapter) {
	t.Helper()
	src := pdfSource()
	sc := scope.Project(t.TempDir())
	f := newFixtureScope(t, src, sc)
	ad := copyAdapter{}
	f.eng.Adapters = []adapter.Adapter{ad}
	f.writeManifest(t, &manifest.Manifest{Skills: map[string]manifest.Entry{"pdf": pdfEntry()}})
	if err := f.eng.Sync(false); err != nil {
		t.Fatal(err)
	}
	return f, ad
}

func (f *fixture) copyLinkFile(ad copyAdapter) string {
	return filepath.Join(ad.SkillsDir(f.scope), "pdf", "SKILL.md")
}

// advanceUpstream moves the fake source to a new commit with new content.
func (f *fixture) advanceUpstream() {
	f.src.commit = commitB
	f.src.tree["skills/pdf/SKILL.md"] = "# pdf v2"
}

// A pristine copy-linked skill must update as smoothly as a symlinked one:
// no --force, copy refreshed, lock moved.
func TestUpdateRefreshesPristineCopyLink(t *testing.T) {
	f, ad := copyFixture(t)
	f.advanceUpstream()

	if err := f.eng.Update("", false); err != nil {
		t.Fatalf("update on pristine copy link: %v\nstderr: %s", err, f.errOut.String())
	}
	data, err := os.ReadFile(f.copyLinkFile(ad))
	if err != nil || string(data) != "# pdf v2" {
		t.Errorf("copy not refreshed: %q, %v", data, err)
	}
	if locked := f.readLock(t).Find("pdf"); locked == nil || locked.Commit != commitB {
		t.Errorf("lock not moved to new commit: %+v", locked)
	}
}

// A diverged copy must make update fail without --force and leave canonical
// and lock consistent (no phantom drift), then succeed with --force.
func TestUpdateDivergedCopyRefusedThenForced(t *testing.T) {
	f, ad := copyFixture(t)
	if err := os.WriteFile(f.copyLinkFile(ad), []byte("# user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.advanceUpstream()

	if err := f.eng.Update("", false); err == nil {
		t.Fatal("update should refuse a diverged copy without --force")
	}
	// The refusal must happen before promotion: canonical still matches the
	// (unmoved) lock, so the next sync sees no phantom drift.
	locked := f.readLock(t).Find("pdf")
	if locked == nil || locked.Commit != commitA {
		t.Fatalf("lock moved despite refused update: %+v", locked)
	}
	hash, err := skill.Hash(f.scope.SkillDir("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if hash != locked.Hash {
		t.Error("canonical no longer matches the lock after refused update (phantom drift)")
	}

	if err := f.eng.Update("", true); err != nil {
		t.Fatalf("update --force: %v\nstderr: %s", err, f.errOut.String())
	}
	data, _ := os.ReadFile(f.copyLinkFile(ad))
	if string(data) != "# pdf v2" {
		t.Errorf("copy not refreshed by --force: %q", data)
	}
}

// sync --force must repair a diverged copy back to the canonical content.
func TestSyncForceRestoresDivergedCopy(t *testing.T) {
	f, ad := copyFixture(t)
	if err := os.WriteFile(f.copyLinkFile(ad), []byte("# user edit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Sync(false); err == nil {
		t.Fatal("sync should refuse a diverged copy without --force")
	}
	if err := f.eng.Sync(true); err != nil {
		t.Fatalf("sync --force: %v\nstderr: %s", err, f.errOut.String())
	}
	data, _ := os.ReadFile(f.copyLinkFile(ad))
	if string(data) != "# pdf" {
		t.Errorf("copy not restored by sync --force: %q", data)
	}
}

// remove --force must delete a diverged copy that plain remove refuses.
func TestRemoveDivergedCopyNeedsForce(t *testing.T) {
	f, ad := copyFixture(t)
	if err := os.WriteFile(f.copyLinkFile(ad), []byte("# user edit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Remove("pdf", false); err == nil {
		t.Fatal("remove should refuse a diverged copy without --force")
	}
	if err := f.eng.Remove("pdf", true); err != nil {
		t.Fatalf("remove --force: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ad.SkillsDir(f.scope), "pdf")); !os.IsNotExist(err) {
		t.Error("diverged copy survived remove --force")
	}
	if _, err := os.Lstat(f.scope.SkillDir("pdf")); !os.IsNotExist(err) {
		t.Error("canonical tree survived remove --force")
	}
}

var _ source.Source = (*fakeSource)(nil)
var _ adapter.Adapter = copyAdapter{}
