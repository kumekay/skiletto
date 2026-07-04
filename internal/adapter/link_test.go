package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/skill"
)

// swapReclaim installs a Windows-like copy-reclaim rule (force or a content
// hash match against the canonical tree) so the copy-link clear/refuse/force
// logic is testable on any OS.
func swapReclaim(t *testing.T) {
	t.Helper()
	old := reclaimDir
	reclaimDir = func(link, canonical string, force bool) bool {
		if force {
			return true
		}
		if !filepath.IsAbs(canonical) {
			canonical = filepath.Clean(filepath.Join(filepath.Dir(link), canonical))
		}
		got, err := skill.Hash(link)
		if err != nil {
			return false
		}
		want, err := skill.Hash(canonical)
		if err != nil {
			return false
		}
		return got == want
	}
	t.Cleanup(func() { reclaimDir = old })
}

// mkTree writes a directory containing a SKILL.md with the given body.
func mkTree(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A copy that still matches the canonical tree is ours: LinkDir replaces it
// without force, exactly like re-linking over a symlink.
func TestLinkDirReplacesPristineCopy(t *testing.T) {
	swapReclaim(t)
	dir := t.TempDir()
	canonical := filepath.Join(dir, "canonical")
	link := filepath.Join(dir, "link")
	mkTree(t, canonical, "# same")
	mkTree(t, link, "# same")

	if _, err := LinkDir(link, canonical, false); err != nil {
		t.Fatalf("LinkDir over pristine copy: %v", err)
	}
	if ok, err := IsLink(link); err != nil || !ok {
		t.Errorf("link not replaced by a real link: %v, %v", ok, err)
	}
}

// A diverged copy is a local modification: refused without force, replaced
// with it.
func TestLinkDirDivergedCopyNeedsForce(t *testing.T) {
	swapReclaim(t)
	dir := t.TempDir()
	canonical := filepath.Join(dir, "canonical")
	link := filepath.Join(dir, "link")
	mkTree(t, canonical, "# upstream")
	mkTree(t, link, "# user edit")

	if _, err := LinkDir(link, canonical, false); err == nil {
		t.Fatal("LinkDir should refuse a diverged copy without force")
	}
	if data, _ := os.ReadFile(filepath.Join(link, "SKILL.md")); string(data) != "# user edit" {
		t.Fatalf("refused LinkDir modified the diverged copy: %q", data)
	}
	if _, err := LinkDir(link, canonical, true); err != nil {
		t.Fatalf("LinkDir --force: %v", err)
	}
	if ok, _ := IsLink(link); !ok {
		t.Error("diverged copy not replaced by a link with force")
	}
}

// RemoveLinkOrCopy: a matching copy goes without force, a diverged one only
// with force.
func TestRemoveLinkOrCopyForceMatrix(t *testing.T) {
	swapReclaim(t)
	dir := t.TempDir()
	canonical := filepath.Join(dir, "canonical")
	link := filepath.Join(dir, "link")
	mkTree(t, canonical, "# same")
	mkTree(t, link, "# same")

	if err := RemoveLinkOrCopy(link, canonical, false); err != nil {
		t.Fatalf("RemoveLinkOrCopy(pristine copy): %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("pristine copy not removed")
	}

	mkTree(t, link, "# user edit")
	if err := RemoveLinkOrCopy(link, canonical, false); err == nil {
		t.Fatal("RemoveLinkOrCopy should refuse a diverged copy without force")
	}
	if err := RemoveLinkOrCopy(link, canonical, true); err != nil {
		t.Fatalf("RemoveLinkOrCopy --force: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("diverged copy not removed with force")
	}
}

// On unix skiletto never produces copies, so a real directory is never
// reclaimed — not even with force. Windows CI covers its own rule.
func TestUnixNeverReclaimsRealDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only guarantee")
	}
	dir := t.TempDir()
	canonical := filepath.Join(dir, "canonical")
	link := filepath.Join(dir, "link")
	mkTree(t, canonical, "# same")
	mkTree(t, link, "# same")

	if _, err := LinkDir(link, canonical, true); err == nil {
		t.Error("LinkDir must refuse a real directory on unix even with force")
	}
	if err := RemoveLinkOrCopy(link, canonical, true); err == nil {
		t.Error("RemoveLinkOrCopy must refuse a real directory on unix even with force")
	}
}

// TestRunLinkChainFallsBack verifies the fallback chain tries strategies in
// order, stops at the first success, and reports which strategy won. This is
// the platform-independent decision logic; the Windows chain (symlink →
// junction → copy) is exercised in CI, but the ordering and short-circuit
// behavior are testable anywhere by injecting a failing first step.
func TestRunLinkChainFallsBack(t *testing.T) {
	var attempted []LinkStrategy
	steps := []linkStep{
		{StrategySymlink, func(string, string) error {
			attempted = append(attempted, StrategySymlink)
			return errors.New("symlink privilege denied")
		}},
		{StrategyJunction, func(string, string) error {
			attempted = append(attempted, StrategyJunction)
			return nil
		}},
		{StrategyCopy, func(string, string) error {
			attempted = append(attempted, StrategyCopy)
			return errors.New("should not reach copy")
		}},
	}
	got, err := runLinkChain("link", "target", steps)
	if err != nil {
		t.Fatalf("runLinkChain: %v", err)
	}
	if got != StrategyJunction {
		t.Errorf("strategy = %q, want %q", got, StrategyJunction)
	}
	if len(attempted) != 2 || attempted[0] != StrategySymlink || attempted[1] != StrategyJunction {
		t.Errorf("attempted = %v, want [symlink junction] and no copy", attempted)
	}
}

// TestRunLinkChainAllFail surfaces every underlying cause when no strategy
// succeeds, so a Windows failure is diagnosable.
func TestRunLinkChainAllFail(t *testing.T) {
	steps := []linkStep{
		{StrategySymlink, func(string, string) error { return errors.New("cause-symlink") }},
		{StrategyJunction, func(string, string) error { return errors.New("cause-junction") }},
	}
	_, err := runLinkChain("link", "target", steps)
	if err == nil {
		t.Fatal("want error when all strategies fail")
	}
	if !strings.Contains(err.Error(), "cause-symlink") || !strings.Contains(err.Error(), "cause-junction") {
		t.Errorf("err = %v, want both causes surfaced", err)
	}
}

// TestRunLinkChainSingleStepVerbatim keeps the unix path (symlink only)
// byte-for-byte: a lone step's error is returned unwrapped, exactly as a
// direct os.Symlink would.
func TestRunLinkChainSingleStepVerbatim(t *testing.T) {
	sentinel := errors.New("sentinel")
	steps := []linkStep{{StrategySymlink, func(string, string) error { return sentinel }}}
	_, err := runLinkChain("link", "target", steps)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the verbatim sentinel error", err)
	}
}
