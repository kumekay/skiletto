package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The `tto` entry point is the short alias binary for skiletto. It must
// build from cmd/tto and behave exactly like the skiletto binary — here we
// smoke-test that `tto --version` exits 0 and reports the version.
func TestTtoBinaryBuildsAndReportsVersion(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "tto")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/tto failed: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("tto --version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "version") {
		t.Fatalf("unexpected --version output: %q", out)
	}
}
