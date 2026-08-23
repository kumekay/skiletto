package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestGlobalFlagIsPersistentAndInterspersed(t *testing.T) {
	for _, args := range [][]string{
		{"-g", "probe", "value"},
		{"probe", "-g", "value"},
		{"probe", "value", "-g"},
		{"group", "-g", "probe", "value"},
		{"group", "probe", "value", "--global"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newRootCmd()
			var gotGlobal bool
			probe := &cobra.Command{
				Use:  "probe <value>",
				Args: cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, _ []string) error {
					var err error
					gotGlobal, err = cmd.Flags().GetBool("global")
					return err
				},
			}
			if args[0] == "group" {
				group := &cobra.Command{Use: "group"}
				group.AddCommand(probe)
				cmd.AddCommand(group)
			} else {
				cmd.AddCommand(probe)
			}
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute %v: %v", args, err)
			}
			if !gotGlobal {
				t.Errorf("global = false for %v", args)
			}
		})
	}
}

func TestFindProjectRootWalksUpToNearestManifest(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "src", "project")
	nested := filepath.Join(project, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeToml(t, project, "[skills]\n")

	root, found, err := findProjectRoot(nested, home)
	if err != nil {
		t.Fatal(err)
	}
	if !found || root != project {
		t.Errorf("findProjectRoot() = %q, %v; want %q, true", root, found, project)
	}
}

func TestFindProjectRootStopsBeforeHome(t *testing.T) {
	home := t.TempDir()
	nested := filepath.Join(home, "work", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeToml(t, home, "[skills]\n")

	root, found, err := findProjectRoot(nested, home)
	if err != nil {
		t.Fatal(err)
	}
	if found || root != nested {
		t.Errorf("findProjectRoot() = %q, %v; want %q, false", root, found, nested)
	}
}

func TestProjectCommandUsesAncestorManifest(t *testing.T) {
	home := freshHome(t)
	project := filepath.Join(home, "src", "project")
	nested := filepath.Join(project, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeToml(t, project, "harnesses = []\n\n[skills]\n")
	t.Chdir(nested)

	stdout, stderr, err := run(t, "harness", "list")
	if err != nil {
		t.Fatalf("harness list: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, filepath.Join(project, ".claude", "skills")) {
		t.Errorf("harness list does not use ancestor project root:\n%s", stdout)
	}
	if strings.Contains(stdout, filepath.Join(nested, ".claude", "skills")) {
		t.Errorf("harness list invented a nested project root:\n%s", stdout)
	}
}

func TestProjectCommandOutsideProjectFailsWithGuidance(t *testing.T) {
	freshHome(t)
	start := t.TempDir()
	t.Chdir(start)

	_, _, err := run(t, "list")
	if err == nil {
		t.Fatal("list outside a project succeeded")
	}
	for _, want := range []string{start, "skiletto.toml", "skiletto add", "skiletto import", "--global"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
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
