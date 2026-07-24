package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The `tto` entry point is the short alias binary for skiletto. It must
// build from cmd/tto and behave exactly like the skiletto binary — here we
// smoke-test that `tto --version` exits 0 and reports the version.
func TestTtoBinaryBuildsAndReportsVersion(t *testing.T) {
	// Build into a directory (not an explicit file name) so Go emits the
	// platform-correct executable name — `tto.exe` on Windows, `tto`
	// elsewhere. `go build -o <file>` would keep the bare name on Windows,
	// which the OS cannot execute.
	dir := t.TempDir()
	build := exec.Command("go", "build", "-o", dir, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/tto failed: %v\n%s", err, out)
	}

	name := "tto"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)

	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("tto --version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "version") {
		t.Fatalf("unexpected --version output: %q", out)
	}
}
