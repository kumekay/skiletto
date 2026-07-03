package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootShowsUsage(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "skiletto") {
		t.Errorf("help output does not mention skiletto:\n%s", out.String())
	}
}

func TestRootShowsVersion(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), version) {
		t.Errorf("version output %q does not contain version %q", out.String(), version)
	}
	if !strings.Contains(out.String(), "skiletto") {
		t.Errorf("version output %q does not mention skiletto", out.String())
	}
}
